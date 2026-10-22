package orgs

// Harper (https://github.com/automattic/harper) on the server, for worg's
// writing-mode checker on devices that cannot spare the memory to run it in
// the page (worg's harperlinter.ts picks browser or server).
//
// Harper is Rust: a few hundred rules over a curated dictionary and a
// part-of-speech tagger. Rather than re-write it (and drift from it), orgs
// runs Harper's own language server, harper-ls, and talks to it in Go over
// LSP on stdin/stdout - so the server and the browser give the same answers
// from the same release. The WebAssembly build was the other way in, but it
// is made for JavaScript (51 host functions whose names change each release).
//
// - Started on the first check, not with the server, and kept running; a
//   crash is noticed and the next check starts it again. It needs nothing
//   stopped on exit: it reads stdin, which closes when orgs goes.
// - Each paragraph is opened as a document, its diagnostics waited for, the
//   fixes and the lint's kind read from a code action per diagnostic, and it
//   is closed again. One batch at a time; a batch with another dialect or
//   other rules switched off first pushes that configuration.
// - Offsets are UTF-16 (LSP's default), the same as the browser's.

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
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf16"

	"github.com/ihdavids/orgs/internal/common"
)

const harperDefaultVersion = "2.13.0"

// What a lint is to worg (harper.ts' HarperLint).
type HarperLint struct {
	Rule  string   `json:"rule"`
	Kind  string   `json:"kind"`
	Start int      `json:"start"`
	End   int      `json:"end"`
	Msg   string   `json:"msg"`
	Fixes []string `json:"fixes"`
}

type harperConfig struct {
	Dialect string
	Off     []string
}

func (c harperConfig) settings() map[string]any {
	linters := map[string]bool{}
	for _, r := range c.Off {
		linters[r] = false
	}
	return map[string]any{"harper-ls": map[string]any{"dialect": c.Dialect, "linters": linters}}
}

type harperServer struct {
	batch sync.Mutex // one batch at a time

	mu      sync.Mutex
	cmd     *exec.Cmd
	in      io.WriteCloser
	nextID  int
	waiting map[int]chan json.RawMessage
	diags   map[string]chan json.RawMessage
	config  harperConfig
	applied string
	state   string // off, starting, ready, failed
	msg     string
	bin     string
}

var harper = &harperServer{state: "off"}

// ---------------------------------------------------------------------------
// Finding harper-ls
// ---------------------------------------------------------------------------

func harperTriple() (string, string, error) {
	switch runtime.GOOS + "/" + runtime.GOARCH {
	case "darwin/arm64":
		return "aarch64-apple-darwin", ".tar.gz", nil
	case "darwin/amd64":
		return "x86_64-apple-darwin", ".tar.gz", nil
	case "linux/arm64":
		return "aarch64-unknown-linux-gnu", ".tar.gz", nil
	case "linux/amd64":
		return "x86_64-unknown-linux-gnu", ".tar.gz", nil
	case "windows/amd64":
		return "x86_64-pc-windows-msvc", ".zip", nil
	}
	return "", "", fmt.Errorf("Harper publishes no harper-ls for %s/%s: install it (cargo install harper-ls) and set harper.path", runtime.GOOS, runtime.GOARCH)
}

func harperBinary() (string, error) {
	s := Conf().Server.Harper
	if s.Path != "" {
		p := expandHome(s.Path)
		if _, err := os.Stat(p); err != nil {
			return "", fmt.Errorf("harper.path: %v", err)
		}
		return p, nil
	}
	if p, err := exec.LookPath("harper-ls"); err == nil {
		return p, nil
	}
	ver := s.Version
	if ver == "" {
		ver = harperDefaultVersion
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	name := "harper-ls"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	dest := filepath.Join(cache, "orgs", "harper-ls-"+ver, name)
	if _, err := os.Stat(dest); err == nil {
		return dest, nil
	}
	if s.Download != nil && !*s.Download {
		return "", fmt.Errorf("no harper-ls: install it (brew install harper, or cargo install harper-ls), or set harper.path, or let orgs download it (harper.download)")
	}
	return dest, harperDownload(ver, dest)
}


// Fetch the release's harper-ls for this machine and unpack the one binary.
func harperDownload(ver, dest string) error {
	triple, ext, err := harperTriple()
	if err != nil {
		return err
	}
	url := fmt.Sprintf("https://github.com/Automattic/harper/releases/download/v%s/harper-ls-%s%s", ver, triple, ext)
	harper.set("starting", "downloading harper-ls "+ver)
	fmt.Fprintf(os.Stderr, "HARPER: downloading %s\n", url)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("downloading harper-ls: %s from %s", res.Status, url)
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, 256<<20))
	if err != nil {
		return err
	}
	var bin []byte
	want := filepath.Base(dest)
	if ext == ".zip" {
		zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
		if err != nil {
			return err
		}
		for _, f := range zr.File {
			if filepath.Base(f.Name) == want {
				rc, err := f.Open()
				if err != nil {
					return err
				}
				bin, err = io.ReadAll(rc)
				rc.Close()
				if err != nil {
					return err
				}
			}
		}
	} else {
		gz, err := gzip.NewReader(bytes.NewReader(raw))
		if err != nil {
			return err
		}
		tr := tar.NewReader(gz)
		for {
			h, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}
			if filepath.Base(h.Name) == want && h.Typeflag == tar.TypeReg {
				if bin, err = io.ReadAll(tr); err != nil {
					return err
				}
			}
		}
	}
	if len(bin) == 0 {
		return fmt.Errorf("no %s in %s", want, url)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	tmp := dest + ".part"
	if err := os.WriteFile(tmp, bin, 0755); err != nil {
		return err
	}
	return os.Rename(tmp, dest)
}

// ---------------------------------------------------------------------------
// The language server
// ---------------------------------------------------------------------------

func (h *harperServer) set(state, msg string) {
	h.mu.Lock()
	h.state, h.msg = state, msg
	h.mu.Unlock()
}

func (h *harperServer) running() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.cmd != nil
}

func (h *harperServer) send(m map[string]any) error {
	m["jsonrpc"] = "2.0"
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	h.mu.Lock()
	in := h.in
	h.mu.Unlock()
	if in == nil {
		return fmt.Errorf("harper-ls is not running")
	}
	_, err = fmt.Fprintf(in, "Content-Length: %d\r\n\r\n%s", len(b), b)
	return err
}

func (h *harperServer) request(method string, params any, timeout time.Duration) (json.RawMessage, error) {
	h.mu.Lock()
	h.nextID++
	id := h.nextID
	ch := make(chan json.RawMessage, 1)
	h.waiting[id] = ch
	h.mu.Unlock()
	defer func() { h.mu.Lock(); delete(h.waiting, id); h.mu.Unlock() }()
	if err := h.send(map[string]any{"id": id, "method": method, "params": params}); err != nil {
		return nil, err
	}
	select {
	case r, ok := <-ch:
		if !ok {
			return nil, fmt.Errorf("harper-ls stopped")
		}
		return r, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("harper-ls did not answer %s", method)
	}
}

func (h *harperServer) start() error {
	bin, err := harperBinary()
	if err != nil {
		h.set("failed", err.Error())
		return err
	}
	cmd := exec.Command(bin, "--stdio")
	in, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		h.set("failed", err.Error())
		return err
	}
	h.mu.Lock()
	h.cmd, h.in, h.bin = cmd, in, bin
	h.waiting = map[int]chan json.RawMessage{}
	h.diags = map[string]chan json.RawMessage{}
	h.applied = ""
	h.state, h.msg = "starting", ""
	h.mu.Unlock()
	go h.read(bufio.NewReader(out))
	go func() {
		err := cmd.Wait()
		h.mu.Lock()
		if h.cmd == cmd {
			h.cmd, h.in = nil, nil
			for _, ch := range h.waiting {
				close(ch)
			}
			h.waiting = map[int]chan json.RawMessage{}
			if h.state != "off" {
				h.state, h.msg = "failed", fmt.Sprintf("harper-ls stopped: %v", err)
			}
		}
		h.mu.Unlock()
	}()
	_, err = h.request("initialize", map[string]any{
		"processId":             os.Getpid(),
		"rootUri":               nil,
		"capabilities":          map[string]any{"workspace": map[string]any{"configuration": true}},
		"initializationOptions": h.config.settings()["harper-ls"],
	}, 30*time.Second)
	if err != nil {
		h.stop()
		h.set("failed", err.Error())
		return err
	}
	h.send(map[string]any{"method": "initialized", "params": map[string]any{}})
	h.set("ready", "")
	fmt.Fprintf(os.Stderr, "HARPER: harper-ls running (%s)\n", bin)
	return nil
}

func (h *harperServer) stop() {
	h.mu.Lock()
	cmd, in := h.cmd, h.in
	h.cmd, h.in = nil, nil
	h.state = "off"
	h.mu.Unlock()
	if in != nil {
		in.Close()
	}
	if cmd != nil && cmd.Process != nil {
		cmd.Process.Kill()
	}
}

// StopHarper ends harper-ls with the server.
func StopHarper() { harper.stop() }

func (h *harperServer) read(r *bufio.Reader) {
	for {
		n := -1
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimSpace(line)
			if line == "" {
				break
			}
			if k, v, ok := strings.Cut(line, ":"); ok && strings.EqualFold(k, "Content-Length") {
				n, _ = strconv.Atoi(strings.TrimSpace(v))
			}
		}
		if n < 0 {
			continue
		}
		body := make([]byte, n)
		if _, err := io.ReadFull(r, body); err != nil {
			return
		}
		var m struct {
			ID     *json.RawMessage `json:"id"`
			Method string           `json:"method"`
			Params json.RawMessage  `json:"params"`
			Result json.RawMessage  `json:"result"`
		}
		if json.Unmarshal(body, &m) != nil {
			continue
		}
		switch {
		case m.Method == "textDocument/publishDiagnostics":
			var p struct {
				URI string `json:"uri"`
			}
			json.Unmarshal(m.Params, &p)
			h.mu.Lock()
			ch := h.diags[p.URI]
			h.mu.Unlock()
			if ch != nil {
				select {
				case ch <- m.Params:
				default:
				}
			}
		case m.Method != "" && m.ID != nil:
			// A request from the server: its configuration, or something
			// orgs has no use for, answered with nothing.
			var result any
			if m.Method == "workspace/configuration" {
				var p struct {
					Items []json.RawMessage `json:"items"`
				}
				json.Unmarshal(m.Params, &p)
				h.mu.Lock()
				cfg := h.config.settings()
				h.mu.Unlock()
				out := []any{}
				for range p.Items {
					out = append(out, cfg)
				}
				result = out
			}
			h.send(map[string]any{"id": m.ID, "result": result})
		case m.ID != nil:
			id, _ := strconv.Atoi(string(*m.ID))
			h.mu.Lock()
			ch := h.waiting[id]
			h.mu.Unlock()
			if ch != nil {
				ch <- m.Result
			}
		}
	}
}

type lspPos struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}
type lspRange struct {
	Start lspPos `json:"start"`
	End   lspPos `json:"end"`
}
type lspDiag struct {
	Range   lspRange        `json:"range"`
	Code    json.RawMessage `json:"code"`
	Message string          `json:"message"`
}

var harperDocs int

// Lints for each text, in order.
func (h *harperServer) Lint(texts []string, cfg harperConfig) ([][]HarperLint, error) {
	h.batch.Lock()
	defer h.batch.Unlock()
	if !h.running() {
		h.mu.Lock()
		h.config = cfg
		h.mu.Unlock()
		if err := h.start(); err != nil {
			return nil, err
		}
	}
	sig := fmt.Sprintf("%s|%v", cfg.Dialect, cfg.Off)
	h.mu.Lock()
	changed := h.applied != sig
	h.config = cfg
	h.applied = sig
	h.mu.Unlock()
	if changed {
		h.send(map[string]any{"method": "workspace/didChangeConfiguration", "params": map[string]any{"settings": cfg.settings()}})
	}
	out := make([][]HarperLint, len(texts))
	for i, text := range texts {
		lints, err := h.lintOne(text)
		if err != nil {
			return nil, err
		}
		out[i] = lints
	}
	return out, nil
}

func (h *harperServer) lintOne(text string) ([]HarperLint, error) {
	h.mu.Lock()
	harperDocs++
	uri := fmt.Sprintf("file:///orgs-harper/%d.txt", harperDocs)
	ch := make(chan json.RawMessage, 4)
	h.diags[uri] = ch
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		delete(h.diags, uri)
		h.mu.Unlock()
		h.send(map[string]any{"method": "textDocument/didClose", "params": map[string]any{"textDocument": map[string]any{"uri": uri}}})
	}()
	if err := h.send(map[string]any{"method": "textDocument/didOpen", "params": map[string]any{
		"textDocument": map[string]any{"uri": uri, "languageId": "plaintext", "version": 1, "text": text},
	}}); err != nil {
		return nil, err
	}
	var raw json.RawMessage
	select {
	case raw = <-ch:
	case <-time.After(20 * time.Second):
		return nil, fmt.Errorf("harper-ls sent no diagnostics")
	}
	var p struct {
		Diagnostics []lspDiag `json:"diagnostics"`
	}
	json.Unmarshal(raw, &p)
	lineStarts := utf16LineStarts(text)
	off := func(q lspPos) int {
		if q.Line < len(lineStarts) {
			return lineStarts[q.Line] + q.Character
		}
		return 0
	}
	out := []HarperLint{}
	for _, d := range p.Diagnostics {
		rule := strings.Trim(string(d.Code), `"`)
		l := HarperLint{Rule: rule, Start: off(d.Range.Start), End: off(d.Range.End), Msg: d.Message, Fixes: []string{}}
		res, err := h.request("textDocument/codeAction", map[string]any{
			"textDocument": map[string]any{"uri": uri},
			"range":        d.Range,
			"context":      map[string]any{"diagnostics": []lspDiag{d}},
		}, 10*time.Second)
		if err == nil {
			readActions(res, d.Range, &l)
		}
		out = append(out, l)
	}
	return out, nil
}

// The fixes for one diagnostic (edits over exactly its range), and its kind
// from the lint harper-ls attaches to them.
func readActions(res json.RawMessage, r lspRange, l *HarperLint) {
	// Each entry is a CodeAction ({title, edit, command: {command,
	// arguments}}) or a bare Command ({title, command: "name", arguments}).
	var acts []struct {
		Edit *struct {
			Changes map[string][]struct {
				Range   lspRange `json:"range"`
				NewText string   `json:"newText"`
			} `json:"changes"`
		} `json:"edit"`
		Command   json.RawMessage   `json:"command"`
		Arguments []json.RawMessage `json:"arguments"`
	}
	if json.Unmarshal(res, &acts) != nil {
		return
	}
	seen := map[string]bool{}
	for _, a := range acts {
		if a.Edit != nil {
			for _, edits := range a.Edit.Changes {
				if len(edits) == 1 && edits[0].Range == r && !seen[edits[0].NewText] {
					seen[edits[0].NewText] = true
					l.Fixes = append(l.Fixes, edits[0].NewText)
				}
			}
		}
		args := a.Arguments
		var cmd struct {
			Arguments []json.RawMessage `json:"arguments"`
		}
		if json.Unmarshal(a.Command, &cmd) == nil && len(cmd.Arguments) > 0 {
			args = cmd.Arguments
		}
		if l.Kind == "" && len(args) > 0 {
			// The lint is a JSON string inside the argument list; other
			// commands carry other things there.
			var arg string
			var lint struct {
				Lint struct {
					Kind string `json:"kind"`
				}
			}
			if json.Unmarshal(args[0], &arg) == nil && json.Unmarshal([]byte(arg), &lint) == nil {
				l.Kind = lint.Lint.Kind
			}
		}
	}
}

// Where each line starts, counted in UTF-16 units as LSP counts characters.
func utf16LineStarts(text string) []int {
	starts := []int{0}
	n := 0
	for _, c := range text {
		if c == '\n' {
			starts = append(starts, n+1)
		}
		n += len(utf16.Encode([]rune{c}))
	}
	return starts
}

// ---------------------------------------------------------------------------
// Endpoints
// ---------------------------------------------------------------------------

type harperLintReq struct {
	Texts   []string
	Dialect string
	Off     []string
}

var harperDialects = map[string]string{
	"american": "American", "british": "British", "australian": "Australian", "canadian": "Canadian", "indian": "Indian",
}

/* SDOC: API
* POST /harper/lint — Check Prose with Harper on the Server
	Body ={Texts, Dialect, Off}=: paragraphs of plain prose, the English
	(american, british, australian, canadian, indian) and Harper rule names
	switched off. Needs =harper: enabled: true=; harper-ls is started on the
	first call.

	*Response:* ={Ok, Msg, Lints}=, =Lints= a list per text of
	={rule, kind, start, end, msg, fixes}=, offsets in UTF-16 units.
EDOC */
func PostHarperLint(w http.ResponseWriter, r *http.Request) {
	var req harperLintReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writingJson(w, common.ResultMsg{Ok: false, Msg: err.Error()})
		return
	}
	if !Conf().Server.Harper.Enabled {
		writingJson(w, common.ResultMsg{Ok: false, Msg: "Harper is not set up on this server (harper: enabled: true in orgs.yaml)"})
		return
	}
	if len(req.Texts) > 200 {
		writingJson(w, common.ResultMsg{Ok: false, Msg: "too many paragraphs at once"})
		return
	}
	d := harperDialects[strings.ToLower(req.Dialect)]
	if d == "" {
		d = "American"
	}
	lints, err := harper.Lint(req.Texts, harperConfig{Dialect: d, Off: req.Off})
	if err != nil {
		writingJson(w, common.ResultMsg{Ok: false, Msg: err.Error()})
		return
	}
	writingJson(w, map[string]any{"Ok": true, "Lints": lints})
}

/* SDOC: API
* GET /harper/status — Harper on the Server
	={Ok, Enabled, State, Msg}=: whether the server offers Harper, and whether
	harper-ls is off (not yet asked for), starting, ready or failed, and why.
EDOC */
func RequestHarperStatus(w http.ResponseWriter, r *http.Request) {
	harper.mu.Lock()
	state, msg := harper.state, harper.msg
	harper.mu.Unlock()
	writingJson(w, map[string]any{"Ok": true, "Enabled": Conf().Server.Harper.Enabled, "State": state, "Msg": msg})
}
