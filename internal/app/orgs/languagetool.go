//lint:file-ignore ST1006 allow the use of self
package orgs

// Running LanguageTool
//
// worg's writing mode can hand its spelling and grammar to LanguageTool. With
// `languageTool: {enabled: true}` in orgs.yaml orgs runs one itself - an
// unzipped LanguageTool under java, or a docker image - for as long as the
// server is up, the way it runs whisper (whisperd.go): a server already
// answering on the port is adopted rather than started again, and the child
// is stopped on a signal as well as on a clean exit.
//
// The browser never talks to it directly: /languagetool/check passes the form
// through, behind orgs' own login, so LanguageTool can listen on localhost
// and a phone reaching orgs reaches it too.

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ihdavids/orgs/internal/common"
)

const (
	LtOff      = "off"
	LtStarting = "starting"
	LtReady    = "ready"
	LtAdopted  = "adopted"
	LtFailed   = "failed"
)

type ltd struct {
	mu       sync.Mutex
	cmd      *exec.Cmd
	docker   bool
	state    string
	msg      string
	lines    []string
	stopping bool
	starts   int
	sigOnce  sync.Once
}

var languageTool = &ltd{state: LtOff}

func ltSettings() common.LanguageToolSettings {
	if Conf().Server == nil {
		return common.LanguageToolSettings{}
	}
	return Conf().Server.LanguageTool
}

func ltPort() int {
	if p := ltSettings().Port; p > 0 {
		return p
	}
	return 8082
}

// Whether orgs starts it, rather than only passing requests on to a url.
func ltManaged() bool {
	s := ltSettings()
	return s.Enabled && strings.TrimSpace(s.Url) == ""
}

func ltBase() string {
	if u := strings.TrimSpace(ltSettings().Url); u != "" {
		return strings.TrimRight(strings.TrimSuffix(strings.TrimRight(u, "/"), "/v2"), "/")
	}
	return fmt.Sprintf("http://localhost:%d", ltPort())
}

func ltConfigured() bool {
	s := ltSettings()
	return s.Enabled || strings.TrimSpace(s.Url) != ""
}

func expandHome(p string) string {
	p = strings.TrimSpace(p)
	if strings.HasPrefix(p, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(p, "~"), "/"))
		}
	}
	if abs, err := filepath.Abs(p); err == nil && p != "" {
		return abs
	}
	return p
}

func (self *ltd) set(state, msg string) {
	self.mu.Lock()
	self.state = state
	self.msg = msg
	self.mu.Unlock()
}

func (self *ltd) say(format string, a ...any) {
	self.mu.Lock()
	self.lines = append(self.lines, fmt.Sprintf(format, a...))
	if len(self.lines) > 40 {
		self.lines = self.lines[len(self.lines)-40:]
	}
	self.mu.Unlock()
}

// LanguageTool answering: its language list, which only it has.
func ltAnswering(timeout time.Duration) (bool, string) {
	c := http.Client{Timeout: timeout}
	res, err := c.Get(ltBase() + "/v2/languages")
	if err != nil {
		return false, ""
	}
	defer res.Body.Close()
	var langs []map[string]any
	if res.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&langs) != nil {
		return false, ""
	}
	return true, ltVersion(timeout)
}

// The server's version, from a check of one word (it is only in a check's
// answer).
func ltVersion(timeout time.Duration) string {
	c := http.Client{Timeout: timeout}
	res, err := c.PostForm(ltBase()+"/v2/check", map[string][]string{"text": {"ok"}, "language": {"en-US"}})
	if err != nil {
		return ""
	}
	defer res.Body.Close()
	var out struct {
		Software struct{ Version string }
	}
	json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&out)
	return out.Software.Version
}

// StartLanguageTool starts LanguageTool if orgs.yaml asks for it. Never blocks:
// it takes a while to load and nothing else waits for it.
func StartLanguageTool() {
	if !ltManaged() {
		if ltConfigured() {
			languageTool.set(LtOff, "using the LanguageTool at "+ltBase())
		}
		return
	}
	go languageTool.start()
	languageTool.sigOnce.Do(func() {
		// As for whisper: StartServer usually ends in log.Fatal, which does
		// not unwind, so a signal is the reliable moment to stop the child.
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
		go func() {
			<-ch
			fmt.Fprintf(os.Stderr, "\n[languagetool] stopping\n")
			StopLanguageTool()
			StopWhisper()
			os.Exit(0)
		}()
	})
}

// The command line: java on an unzipped LanguageTool, else docker.
func ltCommand() (*exec.Cmd, bool, error) {
	s := ltSettings()
	port := fmt.Sprint(ltPort())
	if p := strings.TrimSpace(s.Path); p != "" {
		dir := expandHome(p)
		jar := filepath.Join(dir, "languagetool-server.jar")
		if _, err := os.Stat(jar); err != nil {
			return nil, false, fmt.Errorf("no languagetool-server.jar in %s", dir)
		}
		java := strings.TrimSpace(s.Java)
		if java == "" {
			java = "java"
		}
		if _, err := exec.LookPath(java); err != nil {
			return nil, false, fmt.Errorf("%s is not installed (LanguageTool needs java 17 or later)", java)
		}
		args := append([]string{}, s.JavaArgs...)
		args = append(args, "-cp", jar, "org.languagetool.server.HTTPServer", "--port", port, "--allow-origin", "*")
		if ng := strings.TrimSpace(s.Ngrams); ng != "" {
			args = append(args, "--languageModel", expandHome(ng))
		}
		args = append(args, s.ExtraArgs...)
		cmd := exec.Command(java, args...)
		cmd.Dir = dir
		return cmd, false, nil
	}
	if _, err := exec.LookPath("docker"); err != nil {
		return nil, true, fmt.Errorf("languageTool.path is not set and docker is not installed")
	}
	image := strings.TrimSpace(s.Docker)
	if image == "" {
		image = "erikvl87/languagetool"
	}
	args := []string{"run", "--rm", "--name", "orgs-languagetool", "-p", "127.0.0.1:" + port + ":8010"}
	if ng := strings.TrimSpace(s.Ngrams); ng != "" {
		args = append(args, "-v", expandHome(ng)+":/ngrams:ro", "-e", "langtool_languageModel=/ngrams")
	}
	args = append(args, s.ExtraArgs...)
	args = append(args, image)
	return exec.Command("docker", args...), true, nil
}

func (self *ltd) start() {
	if ok, v := ltAnswering(2 * time.Second); ok {
		self.set(LtAdopted, "using the LanguageTool already on "+ltBase()+ltV(v))
		fmt.Fprintf(os.Stderr, "[languagetool] adopted the server already on %s\n", ltBase())
		return
	}
	cmd, docker, err := ltCommand()
	if err != nil {
		self.set(LtFailed, err.Error())
		fmt.Fprintf(os.Stderr, "[languagetool] %s\n", err)
		return
	}
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	self.mu.Lock()
	self.cmd = cmd
	self.docker = docker
	self.stopping = false
	self.starts++
	attempt := self.starts
	self.mu.Unlock()
	self.set(LtStarting, fmt.Sprintf("starting on port %d", ltPort()))
	if err := cmd.Start(); err != nil {
		self.set(LtFailed, fmt.Sprintf("could not run %s: %s", cmd.Path, err))
		fmt.Fprintf(os.Stderr, "[languagetool] could not run %s: %s\n", cmd.Path, err)
		return
	}
	fmt.Fprintf(os.Stderr, "[languagetool] %s\n", strings.Join(cmd.Args, " "))
	go self.drain(stdout)
	go self.drain(stderr)
	go self.wait(cmd, attempt)
	go self.awaitReady()
}

func ltV(v string) string {
	if v == "" {
		return ""
	}
	return " (LanguageTool " + v + ")"
}

func (self *ltd) drain(r io.ReadCloser) {
	if r == nil {
		return
	}
	defer r.Close()
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 16*1024), 512*1024)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r\n")
		if line == "" {
			continue
		}
		self.say("%s", line)
		fmt.Fprintf(os.Stderr, "[languagetool] %s\n", line)
	}
}

func (self *ltd) wait(cmd *exec.Cmd, attempt int) {
	err := cmd.Wait()
	self.mu.Lock()
	stopping, starts := self.stopping, self.starts
	self.mu.Unlock()
	if stopping || attempt != starts {
		return
	}
	why := "LanguageTool exited"
	if err != nil {
		why += ": " + err.Error()
	}
	fmt.Fprintf(os.Stderr, "[languagetool] %s\n", why)
	if starts >= 3 {
		self.set(LtFailed, why+" - not starting it again")
		return
	}
	self.set(LtStarting, why+" - starting it again")
	time.Sleep(time.Duration(starts) * 2 * time.Second)
	self.start()
}

// LanguageTool takes several seconds to load; a first docker pull far longer.
func (self *ltd) awaitReady() {
	deadline := time.Now().Add(10 * time.Minute)
	for time.Now().Before(deadline) {
		self.mu.Lock()
		stopping, state := self.stopping, self.state
		self.mu.Unlock()
		if stopping || state == LtFailed {
			return
		}
		if ok, v := ltAnswering(2 * time.Second); ok {
			self.set(LtReady, ltV(v))
			fmt.Fprintf(os.Stderr, "[languagetool] ready on %s%s\n", ltBase(), ltV(v))
			return
		}
		time.Sleep(time.Second)
	}
	self.set(LtFailed, "LanguageTool did not answer on "+ltBase()+" after ten minutes")
}

// StopLanguageTool takes the child down; one that was adopted is left alone.
func StopLanguageTool() {
	languageTool.mu.Lock()
	cmd := languageTool.cmd
	docker := languageTool.docker
	languageTool.stopping = true
	languageTool.cmd = nil
	languageTool.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return
	}
	languageTool.set(LtOff, "stopped")
	if docker {
		// The docker client passes the signal on, but a container left
		// running would be adopted next time rather than restarted with new
		// settings.
		exec.Command("docker", "stop", "-t", "5", "orgs-languagetool").Run()
	}
	_ = cmd.Process.Signal(syscall.SIGTERM)
	done := make(chan struct{})
	go func() { cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Kill()
	}
}

/* SDOC: API
* GET /languagetool/status — The LanguageTool Orgs Runs
	={Ok, Running, Url, State, Version, Msg, Lines}=: whether the LanguageTool
	configured by =languageTool:= in orgs.yaml is answering, where, its state
	(off, starting, ready, adopted, failed) and the last lines it printed.
	=Ok= is false when none is configured.
EDOC */
func RequestLanguageToolStatus(w http.ResponseWriter, r *http.Request) {
	if !ltConfigured() {
		writingJson(w, map[string]any{"Ok": false, "Running": false, "Msg": "no languageTool block in orgs.yaml (enabled: true, or url:)"})
		return
	}
	ok, v := ltAnswering(3 * time.Second)
	languageTool.mu.Lock()
	state, msg := languageTool.state, languageTool.msg
	lines := append([]string{}, languageTool.lines...)
	languageTool.mu.Unlock()
	if !ok && msg == "" {
		msg = "nothing answers at " + ltBase()
	}
	writingJson(w, map[string]any{"Ok": true, "Running": ok, "Url": ltBase(), "State": state, "Version": v, "Msg": msg, "Lines": lines})
}

var ltClient = &http.Client{Timeout: 90 * time.Second}

func ltProxy(w http.ResponseWriter, r *http.Request, method, path string) {
	if !ltConfigured() {
		http.Error(w, "no languageTool block in orgs.yaml (enabled: true, or url:)", http.StatusServiceUnavailable)
		return
	}
	var body io.Reader
	if method == http.MethodPost {
		b, _ := io.ReadAll(io.LimitReader(r.Body, 2<<20))
		// Something before the handler that parsed the form has the body.
		if len(b) == 0 && len(r.PostForm) > 0 {
			b = []byte(r.PostForm.Encode())
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(r.Context(), method, ltBase()+path, body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	ct := r.Header.Get("Content-Type")
	if ct == "" {
		ct = "application/x-www-form-urlencoded"
	}
	if method == http.MethodPost {
		req.Header.Set("Content-Type", ct)
	}
	req.Header.Set("Accept", "application/json")
	res, err := ltClient.Do(req)
	if err != nil {
		languageTool.mu.Lock()
		state := languageTool.state
		languageTool.mu.Unlock()
		msg := "LanguageTool is not answering at " + ltBase()
		if state == LtStarting {
			msg = "LanguageTool is still starting"
		}
		http.Error(w, msg, http.StatusBadGateway)
		return
	}
	defer res.Body.Close()
	w.Header().Set("Content-Type", res.Header.Get("Content-Type"))
	w.WriteHeader(res.StatusCode)
	io.Copy(w, res.Body)
}

/* SDOC: API
* POST /languagetool/check — Check Text with LanguageTool
	The form LanguageTool's own =/v2/check= takes (=text=, =language=,
	=motherTongue=, =level=, =disabledRules=...), passed to the LanguageTool
	configured by =languageTool:= in orgs.yaml, and its answer passed back as
	it is. Behind orgs' login, so LanguageTool itself can listen on localhost.
EDOC */
func PostLanguageToolCheck(w http.ResponseWriter, r *http.Request) {
	ltProxy(w, r, http.MethodPost, "/v2/check")
}

/* SDOC: API
* GET /languagetool/languages — LanguageTool's Languages
	LanguageTool's own =/v2/languages=: ={name, code, longCode}= for each
	language the configured server checks.
EDOC */
func RequestLanguageToolLanguages(w http.ResponseWriter, r *http.Request) {
	ltProxy(w, r, http.MethodGet, "/v2/languages")
}
