package orgs

// The local backend's runtimes (assistant.local): llama.cpp, LM Studio and
// MLX, each an OpenAI-compatible server on this machine that orgs can
// install, run, and fill with models from worg's Local runtimes dialog
// (components/LocalRuntimes.tsx) through the /local/* endpoints below. The
// chat itself is assistantlocal.go, over the OpenAI tool loop (openAIChat).
//
// - Each runtime is a localRuntime (locallamacpp.go, locallmstudio.go,
//   localmlx.go) and one localRT supervising it: the same states as Ollama's
//   (missing, installing, stopped, starting, running, adopted, failed,
//   unreachable), the same rules - something already answering at the url is
//   adopted, orgs starts one only on a local url, Stop keeps it down until
//   Start, changing anything is for an administrator (mayChangeKeys).
// - A child orgs starts is recorded in a pid file beside its install, so a
//   server killed without its signal handler running (another handler exited
//   first) finds its own child next time and can still stop and restart it,
//   rather than adopting it as somebody else's.
// - Asking for a model is enough: a pull of a runtime that is not installed
//   installs it first, then fetches the model (the status shows both).
// - Model files come only in forms that hold no code: .gguf for llama.cpp,
//   safetensors and tokenizer files for MLX (localhf.go); LM Studio fetches
//   its own. No runtime is ever given --trust-remote-code.

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ihdavids/orgs/internal/common"
)

// localModel is one model a runtime has, as worg lists it.
type localModel struct {
	Name       string // the runtime's own name for it
	Size       int64
	Params     string
	Quant      string
	Tools      bool // can use tools...
	ToolsKnown bool // ...if this is true; else it is tried and asked again without
	ContextMax int
	Loaded     bool
}

type localPull struct {
	Model    string
	Status   string
	Done     int64
	Total    int64
	Error    string
	Finished bool
	at       time.Time
	cancel   context.CancelFunc
}

type localSuggestion struct {
	Name  string // what pull takes
	About string
	Size  int64
	Gives string // the model's name once fetched, when not Name
	Have  bool   // fetched already
}

// localRuntime is what each runtime knows of itself; localRT does the rest.
type localRuntime interface {
	id() string
	label() string
	conf() *common.LocalRuntimeSettings
	defaultURL() string
	// Why it cannot run on this machine, or nil.
	supported() error
	// Its program: assistant.local.<id>.command, the PATH, or orgs's copy.
	binary() (string, error)
	// What installing does, said before an administrator presses it.
	installNote() string
	install(ctx context.Context, rt *localRT) error
	// Starts it serving on host:port. A long-running child is returned; nil
	// when the program puts itself in the background (LM Studio's daemon),
	// which halt then stops.
	launch(bin, host, port string) (*exec.Cmd, error)
	halt(bin string) error
	// Whether listing its models needs it running (else they are files).
	listsFromServer() bool
	models(ctx context.Context, rt *localRT) ([]localModel, error)
	pull(ctx context.Context, rt *localRT, p *localPull) error
	canCancel() bool
	remove(ctx context.Context, rt *localRT, name string) error
	// Gets the model ready to answer (loads it, swaps another out) and gives
	// the name to send in the request. note tells the writer about a wait.
	prepare(ctx context.Context, rt *localRT, name string, numCtx int, note func(string)) (string, error)
	suggested() []localSuggestion
	// A name worth sending to pull.
	validName(n string) bool
}

type localRT struct {
	impl        localRuntime
	mu          sync.Mutex
	stopMu      sync.Mutex // a second stop waits for the first, so an exit after it is clean
	state       string
	msg         string
	cmd         *exec.Cmd
	pid         int  // a child of an earlier orgs, found by its pid file
	detached    bool // orgs started it, and it went into the background
	userStopped bool
	log         []string
	instDone    int64
	instTotal   int64
	pulls       map[string]*localPull
	models      []localModel
	modelsAt    time.Time
	// A model came or went since orgs started it: llama.cpp's router reads
	// its folder only at start, so it is restarted before it is next used.
	stale bool
}

var localRuntimes = []*localRT{
	{impl: llamaCpp{}, pulls: map[string]*localPull{}},
	{impl: lmStudio{}, pulls: map[string]*localPull{}},
	{impl: mlxLM{}, pulls: map[string]*localPull{}},
}

var localSigOnce sync.Once

func localConf() *common.LocalSettings { return &Conf().Server.Assistant.Local }

// The runtimes this server offers: not disabled, and able to run here.
func localOffered() []*localRT {
	out := []*localRT{}
	for _, rt := range localRuntimes {
		if !rt.impl.conf().Disabled && rt.impl.supported() == nil {
			out = append(out, rt)
		}
	}
	return out
}

func localRuntimeNamed(id string) *localRT {
	for _, rt := range localRuntimes {
		if rt.impl.id() == id {
			return rt
		}
	}
	return nil
}

func (rt *localRT) url() string {
	if u := strings.TrimRight(rt.impl.conf().Url, "/"); u != "" {
		return strings.TrimSuffix(u, "/v1")
	}
	return rt.impl.defaultURL()
}

func localURLIsLocal(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	h := u.Hostname()
	return h == "localhost" || h == "127.0.0.1" || h == "::1"
}

func (rt *localRT) set(state, msg string) {
	rt.mu.Lock()
	rt.state, rt.msg = state, msg
	rt.mu.Unlock()
}

func (rt *localRT) logLine(l string) {
	rt.mu.Lock()
	rt.log = append(rt.log, l)
	if len(rt.log) > 60 {
		rt.log = rt.log[len(rt.log)-60:]
	}
	rt.mu.Unlock()
}

// Where orgs keeps a runtime's install, pid file and (by default) models.
func localDir(id string) (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cache, "orgs", id), nil
}

func (rt *localRT) modelDir() (string, error) {
	if d := rt.impl.conf().ModelDir; d != "" {
		return expandHome(d), nil
	}
	d, err := localDir(rt.impl.id())
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "models"), nil
}

func localContext() int {
	if n := localConf().ContextLength; n > 0 {
		return n
	}
	return 16384
}

func localMaxTokens() int {
	if n := localConf().MaxTokens; n > 0 {
		return n
	}
	return 8192
}

// ---------------------------------------------------------------------------
// Talking to it
// ---------------------------------------------------------------------------

var localClient = &http.Client{Timeout: 30 * time.Second}

func localCall(ctx context.Context, method, full string, body, out any) error {
	return localCallAuth(ctx, method, full, "", body, out)
}

func localCallAuth(ctx context.Context, method, full, key string, body, out any) error {
	var rd io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		rd = strings.NewReader(string(raw))
	}
	req, err := http.NewRequestWithContext(ctx, method, full, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	res, err := localClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 16<<20))
	if res.StatusCode >= 300 {
		var e struct {
			Error any `json:"error"`
		}
		if json.Unmarshal(raw, &e) == nil && e.Error != nil {
			switch v := e.Error.(type) {
			case string:
				return fmt.Errorf("%s", v)
			case map[string]any:
				if m, ok := v["message"].(string); ok {
					return fmt.Errorf("%s", m)
				}
			}
		}
		return fmt.Errorf("%s: %s", res.Status, strings.TrimSpace(string(raw)))
	}
	if out != nil {
		return json.Unmarshal(raw, out)
	}
	return nil
}

// Whether it answers: every one of them serves /v1/models.
func (rt *localRT) alive(timeout time.Duration) bool {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return localCall(ctx, "GET", rt.url()+"/v1/models", nil, nil) == nil
}

// ---------------------------------------------------------------------------
// Running it
// ---------------------------------------------------------------------------

func (rt *localRT) pidFile() string {
	d, err := localDir(rt.impl.id())
	if err != nil {
		return ""
	}
	return filepath.Join(d, "server.pid")
}

// The child an earlier orgs started, if its pid file names a process that is
// still that program; or detached, when it says orgs started one that put
// itself in the background (LM Studio).
func (rt *localRT) earlierChild() (int, bool) {
	raw, err := os.ReadFile(rt.pidFile())
	if err != nil {
		return 0, false
	}
	if strings.TrimSpace(string(raw)) == "detached" {
		return 0, true
	}
	if runtime.GOOS == "windows" {
		return 0, false
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(raw)))
	if pid <= 0 {
		return 0, false
	}
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "command=").Output()
	if err != nil {
		return 0, false
	}
	bin, _ := rt.impl.binary()
	if bin == "" || !strings.Contains(string(out), filepath.Base(bin)) {
		return 0, false
	}
	return pid, false
}

// Takes up what an earlier orgs left running, as its own. Under rt.mu.
func (rt *localRT) claimEarlier() {
	if rt.cmd == nil && !rt.detached && rt.pid == 0 {
		rt.pid, rt.detached = rt.earlierChild()
	}
}

// Makes sure it answers at its url, starting it if it may, and waits up to
// wait. The error says why not.
func (rt *localRT) ensure(wait time.Duration) error {
	rt.mu.Lock()
	stale := rt.stale && (rt.cmd != nil || rt.pid != 0)
	rt.mu.Unlock()
	if stale {
		rt.stop()
		rt.mu.Lock()
		rt.stale = false
		rt.mu.Unlock()
	}
	if rt.alive(2 * time.Second) {
		rt.mu.Lock()
		rt.claimEarlier()
		if rt.cmd != nil || rt.detached || rt.pid != 0 {
			rt.state, rt.msg = "running", rt.impl.label()+" at "+rt.url()
		} else {
			rt.state, rt.msg = "adopted", "using the "+rt.impl.label()+" already running at "+rt.url()
		}
		rt.mu.Unlock()
		return nil
	}
	rt.mu.Lock()
	state, stopped, cmd := rt.state, rt.userStopped, rt.cmd
	rt.mu.Unlock()
	if state == "installing" {
		return fmt.Errorf("%s is being installed", rt.impl.label())
	}
	if !localURLIsLocal(rt.url()) {
		rt.set("unreachable", "nothing answers at "+rt.url())
		return fmt.Errorf("%s does not answer at %s", rt.impl.label(), rt.url())
	}
	if s := localConf().Serve; s != nil && !*s {
		rt.set("stopped", rt.impl.label()+" is not running at "+rt.url()+" (assistant.local.serve is off)")
		return fmt.Errorf("%s is not running at %s", rt.impl.label(), rt.url())
	}
	bin, err := rt.impl.binary()
	if err != nil {
		rt.set("missing", err.Error())
		return err
	}
	if stopped {
		rt.set("stopped", "stopped from worg")
		return fmt.Errorf("%s was stopped: start it in Local runtimes", rt.impl.label())
	}
	if cmd == nil {
		if err := rt.start(bin); err != nil {
			rt.set("failed", err.Error())
			return err
		}
	}
	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		if rt.alive(time.Second) {
			rt.set("running", rt.impl.label()+" at "+rt.url())
			return nil
		}
		rt.mu.Lock()
		gone := rt.cmd == nil && !rt.detached
		rt.mu.Unlock()
		if gone {
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.state == "starting" {
		return fmt.Errorf("%s is starting", rt.impl.label())
	}
	return fmt.Errorf("%s", rt.msg)
}

func (rt *localRT) start(bin string) error {
	u, _ := url.Parse(rt.url())
	port := u.Port()
	if port == "" {
		port = "80"
	}
	cmd, err := rt.impl.launch(bin, u.Hostname(), port)
	if err != nil {
		return err
	}
	localSigOnce.Do(func() {
		// As for Ollama: StartServer usually ends in log.Fatal, which does not
		// unwind, so a signal is the moment to stop the children. One missed
		// (another handler exited first) is found by its pid file next time.
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
		go func() {
			<-ch
			StopLocalRuntimes()
			StopOllama()
			StopLanguageTool()
			StopWhisper()
			os.Exit(0)
		}()
	})
	if cmd == nil {
		if f := rt.pidFile(); f != "" {
			os.MkdirAll(filepath.Dir(f), 0755)
			os.WriteFile(f, []byte("detached"), 0644)
		}
		rt.mu.Lock()
		rt.detached = true
		rt.state, rt.msg = "starting", "starting "+rt.impl.label()
		rt.mu.Unlock()
		return nil
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "LOCAL: started %s %s\n", rt.impl.id(), strings.Join(cmd.Args[1:], " "))
	if f := rt.pidFile(); f != "" {
		os.MkdirAll(filepath.Dir(f), 0755)
		os.WriteFile(f, []byte(strconv.Itoa(cmd.Process.Pid)), 0644)
	}
	rt.mu.Lock()
	rt.cmd, rt.pid = cmd, 0
	rt.state, rt.msg = "starting", "starting "+rt.impl.label()
	rt.mu.Unlock()
	go func() {
		sc := bufio.NewScanner(out)
		sc.Buffer(make([]byte, 64<<10), 1<<20)
		for sc.Scan() {
			rt.logLine(sc.Text())
		}
	}()
	go func() {
		err := cmd.Wait()
		rt.mu.Lock()
		if rt.cmd == cmd {
			rt.cmd = nil
			if !rt.userStopped && !rt.stale {
				rt.state = "failed"
				rt.msg = fmt.Sprintf("%s stopped (%v)", rt.impl.label(), err)
				if n := len(rt.log); n > 0 {
					rt.msg += ": " + rt.log[n-1]
				}
			}
		}
		rt.mu.Unlock()
	}()
	return nil
}

// stop takes down what orgs started; an adopted one is left alone.
func (rt *localRT) stop() {
	rt.stopMu.Lock()
	defer rt.stopMu.Unlock()
	rt.mu.Lock()
	rt.claimEarlier()
	cmd, pid, detached := rt.cmd, rt.pid, rt.detached
	rt.cmd, rt.pid, rt.detached = nil, 0, false
	rt.mu.Unlock()
	switch {
	case cmd != nil && cmd.Process != nil:
		_ = cmd.Process.Signal(syscall.SIGTERM)
		done := make(chan struct{})
		go func() { cmd.Process.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			_ = cmd.Process.Kill()
		}
	case pid != 0:
		if p, err := os.FindProcess(pid); err == nil {
			_ = p.Signal(syscall.SIGTERM)
			for i := 0; i < 50 && p.Signal(syscall.Signal(0)) == nil; i++ {
				time.Sleep(200 * time.Millisecond)
			}
		}
	case detached:
		if bin, err := rt.impl.binary(); err == nil {
			_ = rt.impl.halt(bin)
		}
	default:
		return
	}
	if f := rt.pidFile(); f != "" {
		os.Remove(f)
	}
}

// StopLocalRuntimes takes down every runtime orgs started.
func StopLocalRuntimes() {
	for _, rt := range localRuntimes {
		rt.stop()
	}
}

// ---------------------------------------------------------------------------
// Installing it
// ---------------------------------------------------------------------------

// Installs in the background; progress and failure go into the status.
func (rt *localRT) beginInstall() bool {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.state == "installing" {
		return false
	}
	rt.state, rt.msg = "installing", "starting the install"
	rt.instDone, rt.instTotal = 0, 0
	return true
}

func (rt *localRT) installNow() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()
	err := rt.impl.install(ctx, rt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "LOCAL: installing %s failed: %v\n", rt.impl.id(), err)
		rt.set("failed", "installing "+rt.impl.label()+" failed: "+err.Error())
		return err
	}
	rt.mu.Lock()
	rt.userStopped = false
	rt.state, rt.msg = "stopped", "installed"
	rt.mu.Unlock()
	return nil
}

func (rt *localRT) canInstall() bool {
	if d := localConf().Download; d != nil && !*d {
		return false
	}
	bin, _ := rt.impl.binary()
	return bin == "" && localURLIsLocal(rt.url())
}

// Downloads src into a file in dir, counting into the install progress.
func (rt *localRT) download(ctx context.Context, src, dir string) (string, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", src, nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return "", fmt.Errorf("%s from %s", res.Status, src)
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	part, err := os.CreateTemp(dir, ".download-*")
	if err != nil {
		return "", err
	}
	if res.ContentLength > 0 {
		rt.mu.Lock()
		rt.instTotal += res.ContentLength
		rt.mu.Unlock()
	}
	_, err = io.Copy(part, &countingReader{r: res.Body, n: &rt.instDone, mu: &rt.mu})
	part.Close()
	if err != nil {
		os.Remove(part.Name())
		return "", err
	}
	return part.Name(), nil
}

// Runs an installer's command, its output into the log.
func (rt *localRT) runLogged(ctx context.Context, env []string, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return err
	}
	sc := bufio.NewScanner(out)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	sc.Split(scanLinesOrReturns)
	last := ""
	rt.mu.Lock()
	base := rt.msg
	rt.mu.Unlock()
	for sc.Scan() {
		l := strings.TrimSpace(ansiRe.ReplaceAllString(sc.Text(), ""))
		if m := progressBarRe.FindStringSubmatch(l); m != nil {
			// A progress bar redrawn with \r: the percentage, not a log line.
			rt.set("installing", base+" ("+m[1]+"%)")
			continue
		}
		if l != "" {
			rt.logLine(l)
			last = l
		}
	}
	if err := cmd.Wait(); err != nil {
		if last != "" {
			return fmt.Errorf("%v: %s", err, last)
		}
		return err
	}
	return nil
}

var progressBarRe = regexp.MustCompile(`^[#=>.\s-]*?(\d{1,3}(?:\.\d)?)\s*%$`)

// bufio.ScanLines, but a carriage return ends a line too: installers redraw
// their progress bars with one.
func scanLinesOrReturns(data []byte, atEOF bool) (int, []byte, error) {
	for i, b := range data {
		if b == '\n' || b == '\r' {
			return i + 1, data[:i], nil
		}
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}

// ---------------------------------------------------------------------------
// Models
// ---------------------------------------------------------------------------

// Its models, asked afresh at most every few seconds. One that lists from
// its server lists nothing while it is down.
func (rt *localRT) listModels(fresh bool) ([]localModel, error) {
	rt.mu.Lock()
	if !fresh && rt.models != nil && time.Since(rt.modelsAt) < 5*time.Second {
		m := rt.models
		rt.mu.Unlock()
		return m, nil
	}
	rt.mu.Unlock()
	if rt.impl.listsFromServer() && !rt.alive(time.Second) {
		return []localModel{}, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	models, err := rt.impl.models(ctx, rt)
	if err != nil {
		return nil, err
	}
	sort.Slice(models, func(i, j int) bool { return models[i].Name < models[j].Name })
	rt.mu.Lock()
	rt.models, rt.modelsAt = models, time.Now()
	rt.mu.Unlock()
	return models, nil
}

// A model came or went: one orgs is running is restarted before it is next
// used (one not running reads the folder when it starts; an adopted one is
// not orgs's to restart).
func (rt *localRT) markStale() {
	rt.mu.Lock()
	rt.stale = rt.cmd != nil || rt.pid != 0
	rt.modelsAt = time.Time{}
	rt.mu.Unlock()
}

func (rt *localRT) forget() {
	rt.mu.Lock()
	rt.modelsAt = time.Time{}
	rt.mu.Unlock()
}

// Fetches a model in the background, installing the runtime first when it
// is missing.
func (rt *localRT) pull(name string) error {
	if !rt.impl.validName(name) {
		return fmt.Errorf("%q is not a model name %s takes", name, rt.impl.label())
	}
	rt.mu.Lock()
	if p := rt.pulls[name]; p != nil && !p.Finished {
		rt.mu.Unlock()
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	p := &localPull{Model: name, Status: "starting", cancel: cancel, at: time.Now()}
	rt.pulls[name] = p
	rt.mu.Unlock()
	go func() {
		err := rt.pullAfterInstall(ctx, p)
		rt.mu.Lock()
		p.Finished, p.at = true, time.Now()
		switch {
		case ctx.Err() != nil:
			p.Status, p.Error = "cancelled", ""
		case err != nil:
			p.Status, p.Error = "failed", err.Error()
		default:
			p.Status = "done"
		}
		rt.modelsAt = time.Time{}
		rt.mu.Unlock()
		cancel()
	}()
	return nil
}

func (rt *localRT) pullAfterInstall(ctx context.Context, p *localPull) error {
	if bin, _ := rt.impl.binary(); bin == "" {
		if !rt.canInstall() {
			return fmt.Errorf("%s is not installed, and orgs may not install it", rt.impl.label())
		}
		rt.mu.Lock()
		p.Status = "installing " + rt.impl.label()
		rt.mu.Unlock()
		if rt.beginInstall() {
			if err := rt.installNow(); err != nil {
				return err
			}
		} else {
			for {
				rt.mu.Lock()
				st := rt.state
				rt.mu.Unlock()
				if st != "installing" {
					break
				}
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(time.Second):
				}
			}
		}
	}
	if rt.impl.listsFromServer() {
		if err := rt.ensure(60 * time.Second); err != nil {
			return err
		}
	}
	return rt.impl.pull(ctx, rt, p)
}

func (rt *localRT) progress(p *localPull, status string, done, total int64) {
	rt.mu.Lock()
	p.Status, p.Done, p.Total = status, done, total
	rt.mu.Unlock()
}

// ---------------------------------------------------------------------------
// Endpoints
// ---------------------------------------------------------------------------

type localRuntimeStatus struct {
	Id, Label   string
	Supported   bool
	Why         string // why not supported
	State, Msg  string
	Url         string
	Binary      string
	CanInstall  bool
	InstallNote string
	Install     map[string]int64
	Models      []localModel
	Pulls       []localPull
	Suggested   []localSuggestion
	Log         []string
	CanDelete   bool
	CanCancel   bool
}

/* SDOC: API
* GET /local/status — Local Runtimes for the Writing Assistant
	The local backend's runtimes (llama.cpp, LM Studio, MLX) and servers:
	={Ok, Enabled, Default, CanManage, ContextLength, Runtimes: [{Id, Label,
	Supported, Why, State, Msg, Url, Binary, CanInstall, InstallNote,
	Install: {Done, Total}, Models: [{Name, Size, Params, Quant, Tools,
	ToolsKnown, ContextMax, Loaded}], Pulls: [{Model, Status, Done, Total,
	Error, Finished}], Suggested: [{Name, About, Size}], Log, CanDelete,
	CanCancel}], Servers: [{Name, Url, Ready, Msg, Models}]}=. =State= is
	Ollama's: =missing=, =installing=, =stopped=, =starting=, =running=,
	=adopted=, =failed= or =unreachable=. Asking starts nothing; a runtime is
	started when a model of it is used, or with =start=.
EDOC */
func RequestLocalStatus(w http.ResponseWriter, r *http.Request) {
	c := localConf()
	out := map[string]any{"Ok": true, "Enabled": c.Enabled, "CanManage": mayChangeKeys(r) == nil,
		"ContextLength": localContext(), "Default": c.Runtime}
	list := []localRuntimeStatus{}
	for _, rt := range localRuntimes {
		if rt.impl.conf().Disabled {
			continue
		}
		s := localRuntimeStatus{Id: rt.impl.id(), Label: rt.impl.label(), Url: rt.url(), Supported: true,
			Suggested: rt.impl.suggested(), InstallNote: rt.impl.installNote(),
			CanDelete: rt.impl.id() != "lmstudio", CanCancel: rt.impl.canCancel()}
		if err := rt.impl.supported(); err != nil {
			s.Supported, s.Why = false, err.Error()
			list = append(list, s)
			continue
		}
		if c.Enabled {
			rt.refreshState()
		}
		s.Binary, _ = rt.impl.binary()
		s.CanInstall = rt.canInstall()
		rt.mu.Lock()
		s.State, s.Msg = rt.state, rt.msg
		s.Install = map[string]int64{"Done": rt.instDone, "Total": rt.instTotal}
		s.Log = append([]string{}, rt.log...)
		for k, p := range rt.pulls {
			if p.Finished && time.Since(p.at) > 2*time.Minute {
				delete(rt.pulls, k)
				continue
			}
			s.Pulls = append(s.Pulls, *p)
		}
		rt.mu.Unlock()
		sort.Slice(s.Pulls, func(i, j int) bool { return s.Pulls[i].Model < s.Pulls[j].Model })
		s.Models = []localModel{}
		if s.Binary != "" || s.State == "adopted" {
			if m, err := rt.listModels(false); err == nil {
				s.Models = m
			}
		}
		for i, g := range s.Suggested {
			name := g.Gives
			if name == "" {
				name = g.Name
			}
			for _, m := range s.Models {
				s.Suggested[i].Have = s.Suggested[i].Have || strings.EqualFold(m.Name, name)
			}
		}
		list = append(list, s)
	}
	out["Runtimes"] = list
	out["Servers"] = localServerStatus()
	writingJson(w, out)
}

// The state as it is now, without starting anything: up (ours or not),
// else down for a reason.
func (rt *localRT) refreshState() {
	rt.mu.Lock()
	busy := rt.state == "installing" || rt.state == "starting"
	rt.mu.Unlock()
	if busy && !rt.alive(time.Second) {
		return
	}
	if rt.alive(time.Second) {
		rt.mu.Lock()
		rt.claimEarlier()
		mine := rt.cmd != nil || rt.detached || rt.pid != 0
		rt.mu.Unlock()
		if mine {
			rt.set("running", rt.impl.label()+" at "+rt.url())
		} else {
			rt.set("adopted", "using the "+rt.impl.label()+" already running at "+rt.url())
		}
		return
	}
	bin, err := rt.impl.binary()
	rt.mu.Lock()
	defer rt.mu.Unlock()
	switch {
	case bin == "" && rt.state != "failed":
		rt.state, rt.msg = "missing", err.Error()
	case bin != "" && (rt.state == "" || rt.state == "missing" || rt.state == "running" || rt.state == "adopted"):
		rt.state, rt.msg = "stopped", "not running: it starts when one of its models is used"
		if !localURLIsLocal(rt.url()) {
			rt.state, rt.msg = "unreachable", "nothing answers at "+rt.url()
		}
	}
}

/* SDOC: API
* POST /local/{runtime}/{install,start,stop,pull,cancel,delete} — Managing Local Runtimes
	For an administrator (anyone with =noAuth=). =runtime= is =llamacpp=,
	=lmstudio= or =mlx=. =install= installs it on the server's machine
	(llama.cpp's GitHub release; LM Studio's own installer, llmster; MLX's
	mlx-lm into a python environment of its own); =start= and =stop= the
	server orgs runs (stop keeps it down until start); =pull= ={Model}=
	fetches a model in the background, installing the runtime first when it
	is missing (llama.cpp: =owner/repo:QUANT= or =owner/repo/file.gguf= from
	Hugging Face; MLX: =owner/repo=; LM Studio: a catalogue key such as
	=qwen/qwen3-8b=); =cancel= ={Model}= stops a pull, =delete= ={Model}=
	removes one. *Response:* ={Ok, Msg}=.
EDOC */
func PostLocalAction(w http.ResponseWriter, r *http.Request) {
	var req struct{ Model string }
	json.NewDecoder(r.Body).Decode(&req)
	fail := func(msg string) { writingJson(w, common.ResultMsg{Ok: false, Msg: msg}) }
	if !localConf().Enabled {
		fail("the local backend is off: set assistant.local.enabled in orgs.yaml")
		return
	}
	if err := mayChangeKeys(r); err != nil {
		fail(strings.Replace(err.Error(), "its AI keys", "its local runtimes", 1))
		return
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 2 {
		fail("no runtime named")
		return
	}
	id, action := parts[len(parts)-2], parts[len(parts)-1]
	rt := localRuntimeNamed(id)
	if rt == nil || rt.impl.conf().Disabled {
		fail("no runtime " + id)
		return
	}
	if err := rt.impl.supported(); err != nil {
		fail(err.Error())
		return
	}
	model := strings.TrimSpace(req.Model)
	switch action {
	case "install":
		if d := localConf().Download; d != nil && !*d {
			fail("installing from worg is off (assistant.local.download)")
			return
		}
		if bin, _ := rt.impl.binary(); bin != "" {
			fail(rt.impl.label() + " is already installed: " + bin)
			return
		}
		if !localURLIsLocal(rt.url()) {
			fail(rt.impl.label() + " is set to answer at " + rt.url() + ", which is not this machine")
			return
		}
		if rt.beginInstall() {
			go rt.installNow()
		}
	case "start":
		rt.mu.Lock()
		rt.userStopped = false
		rt.mu.Unlock()
		if err := rt.ensure(30 * time.Second); err != nil {
			fail(err.Error())
			return
		}
	case "stop":
		rt.mu.Lock()
		rt.userStopped = true
		mine := rt.cmd != nil || rt.pid != 0 || rt.detached
		rt.mu.Unlock()
		if !mine {
			fail("this " + rt.impl.label() + " was not started by orgs; stop it where it was started")
			return
		}
		rt.stop()
		rt.set("stopped", "stopped from worg")
	case "pull":
		if err := rt.pull(model); err != nil {
			fail(err.Error())
			return
		}
	case "cancel":
		rt.mu.Lock()
		p := rt.pulls[model]
		rt.mu.Unlock()
		if p == nil || p.Finished {
			fail("no download of " + model + " is running")
			return
		}
		p.cancel()
	case "delete":
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		if err := rt.impl.remove(ctx, rt, model); err != nil {
			fail(err.Error())
			return
		}
		rt.forget()
	default:
		fail("no action " + action)
		return
	}
	writingJson(w, common.ResultMsg{Ok: true})
}
