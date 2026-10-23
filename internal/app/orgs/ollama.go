package orgs

// Ollama for the writing assistant's ollama backend (assistantollama.go):
// finding it, installing it, running it, and its models. worg's Ollama
// manager (components/OllamaManager.tsx) drives all of it through the
// /ollama/* endpoints below.
//
// - Ollama is found at assistant.ollama.command, on the PATH, in Ollama.app,
//   or as the copy orgs installed under <user cache>/orgs/ollama/<version>.
//   Installing is the latest GitHub release for this machine (a .tgz on
//   macOS, .tar.zst on Linux, .zip on Windows), unpacked whole: the binary
//   needs the runner libraries beside it.
// - Something already answering at the url is adopted and never stopped.
//   Otherwise orgs starts `ollama serve` itself when it is first needed
//   (the assistant's status, a pull), on a local url only, and stops it with
//   the server. Stop in worg keeps it down until Start.
// - Pulls run on the server, so closing worg does not stop one; their
//   progress is part of the status, which worg polls (no held stream: see
//   Traps: six connections).
// - Changing anything (install, start, stop, pull, delete) is for an
//   administrator, as setting a key is (mayChangeKeys).

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ihdavids/orgs/internal/common"
	"github.com/klauspost/compress/zstd"
)

type ollamaModel struct {
	Name     string
	Size     int64
	Family   string
	Params   string
	Quant    string
	Modified string
	// From /api/show: what the model can do, and the most context it takes.
	Tools      bool
	Thinking   bool
	Vision     bool
	ContextMax int
	Loaded     bool // in memory now
}

type ollamaPull struct {
	Model    string
	Status   string
	Done     int64
	Total    int64
	Error    string
	Finished bool
	at       time.Time
	cancel   context.CancelFunc
	layers   map[string][2]int64
}

type ollamaCaps struct {
	tools, thinking, vision bool
	ctx                     int
}

type ollamaRun struct {
	mu          sync.Mutex
	state       string // missing, installing, stopped, starting, running, adopted, failed, unreachable
	msg         string
	version     string
	cmd         *exec.Cmd
	userStopped bool
	log         []string
	instDone    int64
	instTotal   int64
	pulls       map[string]*ollamaPull
	caps        map[string]ollamaCaps // by digest
	models      []ollamaModel
	modelsAt    time.Time
	sigOnce     sync.Once
}

var ollama = &ollamaRun{pulls: map[string]*ollamaPull{}, caps: map[string]ollamaCaps{}}

func ollamaConf() *common.OllamaSettings { return &Conf().Server.Assistant.Ollama }

func ollamaURL() string {
	if u := strings.TrimRight(ollamaConf().Url, "/"); u != "" {
		return u
	}
	return "http://127.0.0.1:11434"
}

// A url on this machine: the only kind orgs starts a server for.
func ollamaLocal() bool {
	u, err := url.Parse(ollamaURL())
	if err != nil {
		return false
	}
	h := u.Hostname()
	return h == "localhost" || h == "127.0.0.1" || h == "::1"
}

func (o *ollamaRun) set(state, msg string) {
	o.mu.Lock()
	o.state, o.msg = state, msg
	o.mu.Unlock()
}

func (o *ollamaRun) logLine(l string) {
	o.mu.Lock()
	o.log = append(o.log, l)
	if len(o.log) > 60 {
		o.log = o.log[len(o.log)-60:]
	}
	o.mu.Unlock()
}

// ---------------------------------------------------------------------------
// Talking to it
// ---------------------------------------------------------------------------

var ollamaClient = &http.Client{Timeout: 30 * time.Second}

func ollamaCall(ctx context.Context, method, path string, body, out any) error {
	var rd io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		rd = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, ollamaURL()+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := ollamaClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 16<<20))
	if res.StatusCode >= 300 {
		return ollamaError(res.Status, raw)
	}
	if out != nil {
		return json.Unmarshal(raw, out)
	}
	return nil
}

func ollamaError(status string, raw []byte) error {
	var e struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(raw, &e) == nil && e.Error != "" {
		return fmt.Errorf("%s", e.Error)
	}
	return fmt.Errorf("ollama: %s %s", status, strings.TrimSpace(string(raw)))
}

// Its version, if it answers.
func ollamaAlive(timeout time.Duration) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	var v struct {
		Version string `json:"version"`
	}
	if err := ollamaCall(ctx, "GET", "/api/version", nil, &v); err != nil {
		return "", false
	}
	return v.Version, true
}

// ---------------------------------------------------------------------------
// Finding and installing it
// ---------------------------------------------------------------------------

func ollamaExe() string {
	if runtime.GOOS == "windows" {
		return "ollama.exe"
	}
	return "ollama"
}

func ollamaCacheDir() (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cache, "orgs", "ollama"), nil
}

// The newest copy orgs installed, if any.
func ollamaInstalled() string {
	dir, err := ollamaCacheDir()
	if err != nil {
		return ""
	}
	vers, _ := os.ReadDir(dir)
	sort.Slice(vers, func(i, j int) bool { return versionLess(vers[j].Name(), vers[i].Name()) })
	for _, v := range vers {
		if !v.IsDir() || strings.HasPrefix(v.Name(), ".") {
			continue
		}
		for _, p := range []string{ollamaExe(), filepath.Join("bin", ollamaExe())} {
			full := filepath.Join(dir, v.Name(), p)
			if st, err := os.Stat(full); err == nil && !st.IsDir() {
				return full
			}
		}
	}
	return ""
}

// v0.40.9 against v0.9.1, number by number.
func versionLess(a, b string) bool {
	pa := strings.Split(strings.TrimPrefix(a, "v"), ".")
	pb := strings.Split(strings.TrimPrefix(b, "v"), ".")
	for i := 0; i < len(pa) && i < len(pb); i++ {
		var x, y int
		fmt.Sscan(pa[i], &x)
		fmt.Sscan(pb[i], &y)
		if x != y {
			return x < y
		}
	}
	return len(pa) < len(pb)
}

func ollamaBinary() (string, error) {
	c := ollamaConf()
	if c.Command != "" {
		p, err := exec.LookPath(expandHome(c.Command))
		if err != nil {
			return "", fmt.Errorf("assistant.ollama.command %q is not a command the server can run", c.Command)
		}
		return p, nil
	}
	if p, err := exec.LookPath("ollama"); err == nil {
		return p, nil
	}
	home, _ := os.UserHomeDir()
	for _, p := range []string{"/Applications/Ollama.app/Contents/Resources/ollama", home + "/Applications/Ollama.app/Contents/Resources/ollama"} {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, nil
		}
	}
	if p := ollamaInstalled(); p != "" {
		return p, nil
	}
	return "", fmt.Errorf("Ollama is not installed on the server")
}

// The release file for this machine.
func ollamaAsset() (string, error) {
	switch runtime.GOOS + "/" + runtime.GOARCH {
	case "darwin/arm64", "darwin/amd64":
		return "ollama-darwin.tgz", nil
	case "linux/amd64":
		return "ollama-linux-amd64.tar.zst", nil
	case "linux/arm64":
		return "ollama-linux-arm64.tar.zst", nil
	case "windows/amd64":
		return "ollama-windows-amd64.zip", nil
	case "windows/arm64":
		return "ollama-windows-arm64.zip", nil
	}
	return "", fmt.Errorf("Ollama publishes no build for %s/%s", runtime.GOOS, runtime.GOARCH)
}

// Downloads and unpacks the latest release, then starts it. Run in its own
// goroutine; progress and failure go into the status.
func (o *ollamaRun) install() {
	err := o.installOnce()
	if err != nil {
		fmt.Fprintf(os.Stderr, "OLLAMA: install failed: %v\n", err)
		o.set("failed", "installing Ollama failed: "+err.Error())
		return
	}
	o.mu.Lock()
	o.userStopped = false
	o.state, o.msg = "stopped", "installed"
	o.mu.Unlock()
	ensureOllama(30 * time.Second)
}

// The latest release's tag (v0.40.9), or "" if GitHub will not say.
func ollamaLatestTag(ctx context.Context) string {
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://api.github.com/repos/ollama/ollama/releases/latest", nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return ""
	}
	defer res.Body.Close()
	var rel struct {
		Tag string `json:"tag_name"`
	}
	if res.StatusCode != 200 || json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&rel) != nil {
		return ""
	}
	if !regexp.MustCompile(`^v[0-9][0-9.]*$`).MatchString(rel.Tag) {
		return ""
	}
	return rel.Tag
}

func (o *ollamaRun) installOnce() error {
	asset, err := ollamaAsset()
	if err != nil {
		return err
	}
	dir, err := ollamaCacheDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()
	// The tag names the folder, so a later install is a newer folder beside
	// it; without one, the latest download under "latest".
	ver := ollamaLatestTag(ctx)
	src := "https://github.com/ollama/ollama/releases/download/" + ver + "/" + asset
	if ver == "" {
		ver = "latest"
		src = "https://github.com/ollama/ollama/releases/latest/download/" + asset
	}
	fmt.Fprintf(os.Stderr, "OLLAMA: downloading %s\n", src)
	req, _ := http.NewRequestWithContext(ctx, "GET", src, nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("%s from %s", res.Status, src)
	}
	o.mu.Lock()
	o.instTotal, o.instDone = res.ContentLength, 0
	o.state, o.msg = "installing", "downloading Ollama "+ver
	o.mu.Unlock()
	part, err := os.CreateTemp(dir, ".download-*")
	if err != nil {
		return err
	}
	defer os.Remove(part.Name())
	if _, err := io.Copy(part, &countingReader{r: res.Body, n: &o.instDone, mu: &o.mu}); err != nil {
		part.Close()
		return err
	}
	part.Close()
	o.set("installing", "unpacking Ollama "+ver)
	dest := filepath.Join(dir, ver)
	tmp := dest + ".part"
	os.RemoveAll(tmp)
	if err := unpackOllama(part.Name(), asset, tmp); err != nil {
		os.RemoveAll(tmp)
		return err
	}
	os.RemoveAll(dest)
	if err := os.Rename(tmp, dest); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "OLLAMA: installed %s in %s\n", ver, dest)
	if ollamaInstalled() == "" {
		return fmt.Errorf("no %s in the release", ollamaExe())
	}
	return nil
}

type countingReader struct {
	r  io.Reader
	n  *int64
	mu *sync.Mutex
}

func (c *countingReader) Read(p []byte) (int, error) {
	k, err := c.r.Read(p)
	c.mu.Lock()
	*c.n += int64(k)
	c.mu.Unlock()
	return k, err
}

// Unpacks a release archive into dest, refusing any entry that would land
// outside it.
func unpackOllama(file, name, dest string) error {
	inside := func(p string) (string, error) {
		full := filepath.Join(dest, filepath.FromSlash(p))
		if full != dest && !strings.HasPrefix(full, dest+string(os.PathSeparator)) {
			return "", fmt.Errorf("%s would unpack outside its folder", p)
		}
		return full, nil
	}
	if strings.HasSuffix(name, ".zip") {
		zr, err := zip.OpenReader(file)
		if err != nil {
			return err
		}
		defer zr.Close()
		for _, f := range zr.File {
			full, err := inside(f.Name)
			if err != nil {
				return err
			}
			if f.FileInfo().IsDir() {
				os.MkdirAll(full, 0755)
				continue
			}
			rc, err := f.Open()
			if err != nil {
				return err
			}
			err = writeUnpacked(full, rc, f.Mode())
			rc.Close()
			if err != nil {
				return err
			}
		}
		return nil
	}
	fh, err := os.Open(file)
	if err != nil {
		return err
	}
	defer fh.Close()
	var r io.Reader
	switch {
	case strings.HasSuffix(name, ".tgz"), strings.HasSuffix(name, ".tar.gz"):
		gz, err := gzip.NewReader(fh)
		if err != nil {
			return err
		}
		r = gz
	case strings.HasSuffix(name, ".tar.zst"):
		zr, err := zstd.NewReader(fh)
		if err != nil {
			return err
		}
		defer zr.Close()
		r = zr
	default:
		return fmt.Errorf("no way to unpack %s", name)
	}
	tr := tar.NewReader(r)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		full, err := inside(h.Name)
		if err != nil {
			return err
		}
		switch h.Typeflag {
		case tar.TypeDir:
			os.MkdirAll(full, 0755)
		case tar.TypeReg:
			if err := writeUnpacked(full, tr, os.FileMode(h.Mode)); err != nil {
				return err
			}
		case tar.TypeSymlink:
			// Only links to a neighbour (libggml.dylib -> libggml.0.dylib).
			if filepath.IsAbs(h.Linkname) {
				return fmt.Errorf("%s links outside its folder", h.Name)
			}
			if _, err := inside(filepath.Join(filepath.Dir(h.Name), h.Linkname)); err != nil {
				return err
			}
			os.MkdirAll(filepath.Dir(full), 0755)
			os.Remove(full)
			if err := os.Symlink(h.Linkname, full); err != nil {
				return err
			}
		}
	}
}

func writeUnpacked(full string, r io.Reader, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		return err
	}
	if mode&0111 != 0 {
		mode = 0755
	} else {
		mode = 0644
	}
	f, err := os.OpenFile(full, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// ---------------------------------------------------------------------------
// Running it
// ---------------------------------------------------------------------------

// Makes sure something answers at the url, starting `ollama serve` if it
// may, and waits up to wait for it. The error says why not.
func ensureOllama(wait time.Duration) error {
	o := ollama
	if v, ok := ollamaAlive(2 * time.Second); ok {
		o.mu.Lock()
		o.version = v
		if o.cmd == nil {
			o.state, o.msg = "adopted", "using the Ollama already running at "+ollamaURL()
		} else {
			o.state, o.msg = "running", "Ollama "+v+" at "+ollamaURL()
		}
		o.mu.Unlock()
		return nil
	}
	o.mu.Lock()
	state, stopped, cmd := o.state, o.userStopped, o.cmd
	o.mu.Unlock()
	if state == "installing" {
		return fmt.Errorf("Ollama is being installed")
	}
	if !ollamaLocal() {
		o.set("unreachable", "nothing answers at "+ollamaURL())
		return fmt.Errorf("Ollama does not answer at %s", ollamaURL())
	}
	if c := ollamaConf(); c.Serve != nil && !*c.Serve {
		o.set("stopped", "Ollama is not running at "+ollamaURL()+" (assistant.ollama.serve is off)")
		return fmt.Errorf("Ollama is not running at %s", ollamaURL())
	}
	bin, err := ollamaBinary()
	if err != nil {
		o.set("missing", err.Error())
		return err
	}
	if stopped {
		o.set("stopped", "stopped from worg")
		return fmt.Errorf("Ollama was stopped: start it in the Ollama manager")
	}
	if cmd == nil {
		if err := o.start(bin); err != nil {
			o.set("failed", err.Error())
			return err
		}
	}
	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		if v, ok := ollamaAlive(time.Second); ok {
			o.mu.Lock()
			o.version = v
			o.state, o.msg = "running", "Ollama "+v+" at "+ollamaURL()
			o.mu.Unlock()
			return nil
		}
		o.mu.Lock()
		gone := o.cmd == nil
		o.mu.Unlock()
		if gone {
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.state == "starting" {
		return fmt.Errorf("Ollama is starting")
	}
	return fmt.Errorf("%s", o.msg)
}

func (o *ollamaRun) start(bin string) error {
	u, _ := url.Parse(ollamaURL())
	host := u.Host
	if u.Port() == "" {
		host = net.JoinHostPort(u.Hostname(), "11434")
	}
	c := ollamaConf()
	cmd := exec.Command(bin, "serve")
	cmd.Env = append(os.Environ(), "OLLAMA_HOST="+host)
	if n := ollamaContextDefault(); n > 0 {
		cmd.Env = append(cmd.Env, fmt.Sprintf("OLLAMA_CONTEXT_LENGTH=%d", n))
	}
	if c.ModelDir != "" {
		cmd.Env = append(cmd.Env, "OLLAMA_MODELS="+expandHome(c.ModelDir))
	}
	if c.KeepAlive != "" {
		cmd.Env = append(cmd.Env, "OLLAMA_KEEP_ALIVE="+c.KeepAlive)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "OLLAMA: started %s serve on %s\n", bin, host)
	o.mu.Lock()
	o.cmd = cmd
	o.state, o.msg = "starting", "starting Ollama"
	o.mu.Unlock()
	go func() {
		sc := bufio.NewScanner(out)
		sc.Buffer(make([]byte, 64<<10), 1<<20)
		for sc.Scan() {
			o.logLine(sc.Text())
		}
	}()
	go func() {
		err := cmd.Wait()
		o.mu.Lock()
		if o.cmd == cmd {
			o.cmd = nil
			if !o.userStopped {
				o.state = "failed"
				o.msg = fmt.Sprintf("Ollama stopped (%v)", err)
				if n := len(o.log); n > 0 {
					o.msg += ": " + o.log[n-1]
				}
			}
		}
		o.mu.Unlock()
	}()
	o.sigOnce.Do(func() {
		// As for whisper and LanguageTool: StartServer usually ends in
		// log.Fatal, which does not unwind, so a signal is the moment to stop
		// the child. A child missed by a handler that exits first is adopted
		// next time.
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
		go func() {
			<-ch
			StopOllama()
			StopLanguageTool()
			StopWhisper()
			os.Exit(0)
		}()
	})
	return nil
}

// StopOllama takes down the Ollama orgs started; an adopted one is left
// alone.
func StopOllama() {
	o := ollama
	o.mu.Lock()
	cmd := o.cmd
	o.cmd = nil
	o.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Signal(syscall.SIGTERM)
	done := make(chan struct{})
	go func() { cmd.Process.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Kill()
	}
}

func ollamaContextDefault() int {
	if n := ollamaConf().ContextLength; n > 0 {
		return n
	}
	return 16384
}

// ---------------------------------------------------------------------------
// Models
// ---------------------------------------------------------------------------

// The installed models with what each can do, asked afresh at most every
// few seconds (the assistant's status and chat both want them).
func ollamaModels(fresh bool) ([]ollamaModel, error) {
	o := ollama
	o.mu.Lock()
	if !fresh && o.models != nil && time.Since(o.modelsAt) < 5*time.Second {
		m := o.models
		o.mu.Unlock()
		return m, nil
	}
	o.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var tags struct {
		Models []struct {
			Name       string `json:"name"`
			Size       int64  `json:"size"`
			Digest     string `json:"digest"`
			ModifiedAt string `json:"modified_at"`
			Details    struct {
				Family string `json:"family"`
				Params string `json:"parameter_size"`
				Quant  string `json:"quantization_level"`
			} `json:"details"`
		} `json:"models"`
	}
	if err := ollamaCall(ctx, "GET", "/api/tags", nil, &tags); err != nil {
		return nil, err
	}
	var ps struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	ollamaCall(ctx, "GET", "/api/ps", nil, &ps)
	loaded := map[string]bool{}
	for _, m := range ps.Models {
		loaded[m.Name] = true
	}
	out := []ollamaModel{}
	for _, t := range tags.Models {
		c := ollamaCapsOf(ctx, t.Name, t.Digest)
		out = append(out, ollamaModel{
			Name: t.Name, Size: t.Size, Family: t.Details.Family, Params: t.Details.Params, Quant: t.Details.Quant,
			Modified: t.ModifiedAt, Tools: c.tools, Thinking: c.thinking, Vision: c.vision, ContextMax: c.ctx, Loaded: loaded[t.Name],
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	o.mu.Lock()
	o.models, o.modelsAt = out, time.Now()
	o.mu.Unlock()
	return out, nil
}

// /api/show's capabilities, once per digest.
func ollamaCapsOf(ctx context.Context, name, digest string) ollamaCaps {
	o := ollama
	o.mu.Lock()
	c, ok := o.caps[digest]
	o.mu.Unlock()
	if ok {
		return c
	}
	var show struct {
		Capabilities []string       `json:"capabilities"`
		ModelInfo    map[string]any `json:"model_info"`
	}
	if err := ollamaCall(ctx, "POST", "/api/show", map[string]any{"model": name}, &show); err != nil {
		return c
	}
	for _, k := range show.Capabilities {
		switch k {
		case "tools":
			c.tools = true
		case "thinking":
			c.thinking = true
		case "vision":
			c.vision = true
		}
	}
	for k, v := range show.ModelInfo {
		if strings.HasSuffix(k, ".context_length") {
			if f, ok := v.(float64); ok {
				c.ctx = int(f)
			}
		}
	}
	o.mu.Lock()
	o.caps[digest] = c
	o.mu.Unlock()
	return c
}

var ollamaNameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._/:-]{0,199}$`)

// Fetches a model in the background; its progress is in the status.
func (o *ollamaRun) pull(name string) error {
	if !ollamaNameRe.MatchString(name) {
		return fmt.Errorf("%q is not a model name (like qwen3:8b)", name)
	}
	o.mu.Lock()
	if p := o.pulls[name]; p != nil && !p.Finished {
		o.mu.Unlock()
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	p := &ollamaPull{Model: name, Status: "starting", cancel: cancel, at: time.Now(), layers: map[string][2]int64{}}
	o.pulls[name] = p
	o.mu.Unlock()
	go func() {
		err := o.pullOnce(ctx, p)
		o.mu.Lock()
		p.Finished, p.at = true, time.Now()
		switch {
		case ctx.Err() != nil:
			p.Status, p.Error = "cancelled", ""
		case err != nil:
			p.Status, p.Error = "failed", err.Error()
		default:
			p.Status = "done"
		}
		o.modelsAt = time.Time{}
		o.mu.Unlock()
		cancel()
	}()
	return nil
}

func (o *ollamaRun) pullOnce(ctx context.Context, p *ollamaPull) error {
	raw, _ := json.Marshal(map[string]any{"model": p.Model, "stream": true})
	req, _ := http.NewRequestWithContext(ctx, "POST", ollamaURL()+"/api/pull", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 64<<10))
		return ollamaError(res.Status, b)
	}
	sc := bufio.NewScanner(res.Body)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		var ev struct {
			Status    string `json:"status"`
			Digest    string `json:"digest"`
			Total     int64  `json:"total"`
			Completed int64  `json:"completed"`
			Error     string `json:"error"`
		}
		if json.Unmarshal(sc.Bytes(), &ev) != nil {
			continue
		}
		if ev.Error != "" {
			return fmt.Errorf("%s", ev.Error)
		}
		o.mu.Lock()
		p.Status = ev.Status
		if ev.Digest != "" && ev.Total > 0 {
			p.layers[ev.Digest] = [2]int64{ev.Completed, ev.Total}
			p.Done, p.Total = 0, 0
			for _, l := range p.layers {
				p.Done += l[0]
				p.Total += l[1]
			}
		}
		o.mu.Unlock()
	}
	return sc.Err()
}

// A few models that use tools well, for the manager to offer. Sizes are
// the download, roughly.
var ollamaSuggested = []map[string]any{
	{"Name": "qwen3:4b", "About": "Small and quick; fine for a laptop.", "Size": 2_600_000_000},
	{"Name": "qwen3:8b", "About": "A good default: careful with tools, decent prose.", "Size": 5_200_000_000},
	{"Name": "qwen3:14b", "About": "Better writing; wants 16GB of memory or more.", "Size": 9_300_000_000},
	{"Name": "gpt-oss:20b", "About": "OpenAI's open model; strong, wants 16GB or more.", "Size": 14_000_000_000},
	{"Name": "llama3.1:8b", "About": "Meta's; plain and reliable.", "Size": 4_900_000_000},
	{"Name": "mistral-nemo:12b", "About": "Mistral's; good with long text.", "Size": 7_100_000_000},
}

// ---------------------------------------------------------------------------
// Endpoints
// ---------------------------------------------------------------------------

/* SDOC: API
* GET /ollama/status — Ollama for the Writing Assistant
	Whether Ollama is installed and running for the writing assistant's
	ollama backend, and its models: ={Ok, Enabled, State, Msg, Version, Url,
	Binary, CanManage, CanInstall, Install: {Done, Total}, ContextLength,
	Models: [{Name, Size, Family, Params, Quant, Tools, Thinking, Vision,
	ContextMax, Loaded}], Pulls: [{Model, Status, Done, Total, Error,
	Finished}], Suggested: [{Name, About, Size}], Log}=. =State= is
	=missing=, =installing=, =stopped=, =starting=, =running= (started by
	orgs), =adopted= (already running), =failed= or =unreachable=. Asking
	starts Ollama when it is installed and not running (a local url only).
EDOC */
func RequestOllamaStatus(w http.ResponseWriter, r *http.Request) {
	c := ollamaConf()
	out := map[string]any{"Ok": true, "Enabled": c.Enabled, "Url": ollamaURL(), "CanManage": mayChangeKeys(r) == nil,
		"ContextLength": ollamaContextDefault(), "Suggested": ollamaSuggested}
	if c.Enabled {
		ensureOllama(5 * time.Second)
	}
	bin, _ := ollamaBinary()
	out["Binary"] = bin
	_, err := ollamaAsset()
	out["CanInstall"] = bin == "" && err == nil && (c.Download == nil || *c.Download) && ollamaLocal()
	o := ollama
	o.mu.Lock()
	state := o.state
	out["State"], out["Msg"], out["Version"] = o.state, o.msg, o.version
	out["Install"] = map[string]int64{"Done": o.instDone, "Total": o.instTotal}
	out["Log"] = append([]string{}, o.log...)
	pulls := []ollamaPull{}
	for k, p := range o.pulls {
		if p.Finished && time.Since(p.at) > 2*time.Minute {
			delete(o.pulls, k)
			continue
		}
		pulls = append(pulls, *p)
	}
	o.mu.Unlock()
	if state == "" && bin == "" {
		out["State"], out["Msg"] = "missing", "Ollama is not installed on the server"
	}
	sort.Slice(pulls, func(i, j int) bool { return pulls[i].Model < pulls[j].Model })
	out["Pulls"] = pulls
	models := []ollamaModel{}
	if state == "running" || state == "adopted" {
		if m, err := ollamaModels(false); err == nil {
			models = m
		}
	}
	out["Models"] = models
	writingJson(w, out)
}

/* SDOC: API
* POST /ollama/{install,start,stop,pull,cancel,delete} — Managing Ollama
	For an administrator (anyone with =noAuth=). =install= downloads the
	latest Ollama release for the server's machine into the user cache and
	starts it; =start= and =stop= the =ollama serve= orgs runs (stop keeps it
	down until start); =pull= ={Model}= fetches a model in the background
	(progress in =GET /ollama/status=), =cancel= ={Model}= stops a pull,
	=delete= ={Model}= removes one. *Response:* ={Ok, Msg}=.
EDOC */
func PostOllamaAction(w http.ResponseWriter, r *http.Request) {
	var req struct{ Model string }
	json.NewDecoder(r.Body).Decode(&req)
	fail := func(msg string) { writingJson(w, common.ResultMsg{Ok: false, Msg: msg}) }
	if !ollamaConf().Enabled {
		fail("the ollama backend is off: set assistant.ollama.enabled in orgs.yaml")
		return
	}
	if err := mayChangeKeys(r); err != nil {
		fail(strings.Replace(err.Error(), "its AI keys", "its Ollama", 1))
		return
	}
	o := ollama
	model := strings.TrimSpace(req.Model)
	switch action := filepath.Base(r.URL.Path); action {
	case "install":
		c := ollamaConf()
		if c.Download != nil && !*c.Download {
			fail("installing from worg is off (assistant.ollama.download)")
			return
		}
		if bin, _ := ollamaBinary(); bin != "" {
			fail("Ollama is already installed: " + bin)
			return
		}
		o.mu.Lock()
		busy := o.state == "installing"
		if !busy {
			o.state, o.msg = "installing", "starting the download"
		}
		o.mu.Unlock()
		if !busy {
			go o.install()
		}
	case "start":
		o.mu.Lock()
		o.userStopped = false
		o.mu.Unlock()
		if err := ensureOllama(20 * time.Second); err != nil {
			fail(err.Error())
			return
		}
	case "stop":
		o.mu.Lock()
		o.userStopped = true
		mine := o.cmd != nil
		o.mu.Unlock()
		if !mine {
			fail("this Ollama was not started by orgs; stop it where it was started")
			return
		}
		StopOllama()
		o.set("stopped", "stopped from worg")
	case "pull":
		if err := ensureOllama(20 * time.Second); err != nil {
			fail(err.Error())
			return
		}
		if err := o.pull(model); err != nil {
			fail(err.Error())
			return
		}
	case "cancel":
		o.mu.Lock()
		p := o.pulls[model]
		o.mu.Unlock()
		if p == nil || p.Finished {
			fail("no pull of " + model + " is running")
			return
		}
		p.cancel()
	case "delete":
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		if err := ollamaCall(ctx, "DELETE", "/api/delete", map[string]any{"model": model}, nil); err != nil {
			fail(err.Error())
			return
		}
		o.mu.Lock()
		o.modelsAt = time.Time{}
		o.mu.Unlock()
	default:
		fail("no action " + action)
		return
	}
	writingJson(w, common.ResultMsg{Ok: true})
}
