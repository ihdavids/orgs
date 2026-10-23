package orgs

// The claudecode backend of the writing assistant (assistant.go): Claude
// through the claude command (Claude Code) on the server's machine, so it is
// paid for by the Claude subscription that command is signed in with rather
// than by API credits. Off unless assistant.claudecode.enabled is set.
//
// - Each turn runs `claude -p` once, in an empty folder, with its own tools,
//   settings, CLAUDE.md, hooks and MCP servers all turned off (--tools "",
//   --restricted, --setting-sources=, --strict-mcp-config): it can do nothing on this machine but
//   what the tools below allow. The conversation so far goes in as text, as
//   for the other backends; nothing is kept between turns.
// - The tools are orgs's, served over MCP from a listener on 127.0.0.1 opened
//   for the turn, at a path with a secret in it, and closed with it: list,
//   read and find in the files the writer allowed (the open file and the
//   "Can read" ones), and edit those they ticked as editable.
// - The open file is read from and edited in the editor's text (Doc in the
//   request), never the disk: an edit is sent to worg as {"edit": ...} and
//   made there as an ordinary change, so it can be undone and nothing unsaved
//   is overwritten. Another file is edited on disk, splicing only the text
//   that changed (see Traps: line edits).
// - ANTHROPIC_API_KEY and ANTHROPIC_AUTH_TOKEN are taken out of its
//   environment: with either set the claude command bills the API, which is
//   what this backend is for avoiding. A token from `claude setup-token` can
//   be given as apiKey (CLAUDE_CODE_OAUTH_TOKEN) for a server whose user has
//   not run `claude login`.

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf16"

	"github.com/ihdavids/orgs/internal/common"
)

// ---------------------------------------------------------------------------
// The command, its sign-in and its environment
// ---------------------------------------------------------------------------

// The claude command: the yaml's, else on the PATH, else where its installers
// put it (a server started by launchd has a short PATH).
func claudeCodeCommand(b *common.AssistantBackend) (string, error) {
	if b.Command != "" {
		if p, err := exec.LookPath(b.Command); err == nil {
			return p, nil
		}
		return "", fmt.Errorf("assistant.claudecode.command %q is not a command the server can run", b.Command)
	}
	if p, err := exec.LookPath("claude"); err == nil {
		return p, nil
	}
	home, _ := os.UserHomeDir()
	for _, p := range []string{home + "/.claude/local/claude", home + "/.local/bin/claude", "/opt/homebrew/bin/claude", "/usr/local/bin/claude"} {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, nil
		}
	}
	return "", fmt.Errorf("the claude command is not on the server's PATH: install Claude Code (https://claude.com/claude-code) or set assistant.claudecode.command")
}

// The token given for it, if any: apiKey (literal, env: or keyring:) or one
// set from worg. None is fine; `claude login` is the usual way.
func claudeCodeToken(b *common.AssistantBackend) (string, error) {
	if k := assistantStoredKey("claudecode"); k != "" {
		return k, nil
	}
	k := strings.TrimSpace(b.ApiKey)
	switch {
	case strings.HasPrefix(k, "env:"):
		v := os.Getenv(strings.TrimPrefix(k, "env:"))
		if v == "" {
			return "", fmt.Errorf("%s is not set in the server's environment", strings.TrimPrefix(k, "env:"))
		}
		return v, nil
	case strings.HasPrefix(k, "keyring:"):
		v, err := common.KeyringGet(strings.TrimPrefix(k, "keyring:"))
		if err != nil || v == "" {
			return "", fmt.Errorf("no %q in the keyring (service orgs)", strings.TrimPrefix(k, "keyring:"))
		}
		return v, nil
	}
	return k, nil
}

// The server's environment without what would make the claude command bill
// the API, or refuse to start because orgs itself was started from a Claude
// Code session.
func claudeCodeEnv(token string) []string {
	out := []string{}
	for _, e := range os.Environ() {
		name, _, _ := strings.Cut(e, "=")
		switch name {
		case "ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "CLAUDECODE", "CLAUDE_CODE_ENTRYPOINT", "CLAUDE_CODE_OAUTH_TOKEN":
			continue
		}
		out = append(out, e)
	}
	if token != "" {
		out = append(out, "CLAUDE_CODE_OAUTH_TOKEN="+token)
	}
	return out
}

type claudeAuth struct {
	LoggedIn         bool   `json:"loggedIn"`
	AuthMethod       string `json:"authMethod"`
	SubscriptionType string `json:"subscriptionType"`
	err              error
	at               time.Time
}

var (
	claudeAuthMu   sync.Mutex
	claudeAuthLast *claudeAuth
)

// `claude auth status`, asked at most once a minute (it is quick, but the
// status is asked every time the pane opens).
func claudeCodeAuth(b *common.AssistantBackend) *claudeAuth {
	claudeAuthMu.Lock()
	defer claudeAuthMu.Unlock()
	if claudeAuthLast != nil && time.Since(claudeAuthLast.at) < time.Minute {
		return claudeAuthLast
	}
	a := &claudeAuth{at: time.Now()}
	claudeAuthLast = a
	bin, err := claudeCodeCommand(b)
	if err != nil {
		a.err = err
		return a
	}
	token, err := claudeCodeToken(b)
	if err != nil {
		a.err = err
		return a
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "auth", "status", "--json")
	cmd.Env = claudeCodeEnv(token)
	out, err := cmd.Output()
	if json.Unmarshal(out, a) != nil {
		a.err = fmt.Errorf("could not ask the claude command whether it is signed in: %v", err)
		return a
	}
	if !a.LoggedIn {
		a.err = fmt.Errorf("the claude command is not signed in on the server: run `claude login` as the user orgs runs as, or `claude setup-token` and put the token in assistant.claudecode.apiKey")
	}
	return a
}

// Ready or why not; the "key" is the token, if one was given.
func claudeCodeKey(b *common.AssistantBackend) (string, error) {
	if a := claudeCodeAuth(b); a.err != nil {
		return "", a.err
	}
	return claudeCodeToken(b)
}

func claudeCodeSource(b *common.AssistantBackend) string {
	a := claudeCodeAuth(b)
	switch {
	case a.err != nil:
		return ""
	case a.AuthMethod == "claude.ai":
		return "subscription"
	case strings.Contains(strings.ToLower(a.AuthMethod), "token"):
		return "token"
	}
	return "api"
}

// ---------------------------------------------------------------------------
// The files it may use
// ---------------------------------------------------------------------------

type codeFiles struct {
	mu     sync.Mutex
	active string // the open file
	doc    string // its text in the editor, as edited so far this turn
	read   []string
	edit   map[string]bool
	send   func(any)
}

// The files a request allows, each one the OrgDb knows, by its own name.
func newCodeFiles(req *assistantReq) (*codeFiles, error) {
	known := func(name string) (string, error) {
		f := GetDb().FindByFile(name)
		if f == nil {
			return "", fmt.Errorf("%s is not an org file this server knows", name)
		}
		return f.Filename, nil
	}
	c := &codeFiles{doc: req.Doc, edit: map[string]bool{}}
	active, err := known(req.File)
	if err != nil {
		return nil, err
	}
	c.active = active
	c.read = []string{active}
	alias := map[string]string{req.File: active, active: active}
	for _, o := range req.Others {
		f, err := known(o)
		if err != nil {
			return nil, err
		}
		alias[o] = f
		if f != active && !contains(c.read, f) {
			c.read = append(c.read, f)
		}
	}
	for _, e := range req.Edit {
		f, ok := alias[e]
		if !ok {
			return nil, fmt.Errorf("%s may be edited only if it may also be read", e)
		}
		c.edit[f] = true
	}
	if c.edit[active] && req.Doc == "" {
		return nil, fmt.Errorf("the editor's text was not sent, so the open file cannot be edited")
	}
	return c, nil
}

// A file named by the model: its full name, or a base name only one of the
// allowed files has.
func (c *codeFiles) resolve(name string) (string, error) {
	name = strings.TrimSpace(name)
	if contains(c.read, name) {
		return name, nil
	}
	found := ""
	for _, f := range c.read {
		if filepath.Base(f) == filepath.Base(name) || strings.HasSuffix(f, "/"+strings.TrimPrefix(name, "/")) {
			if found != "" {
				return "", fmt.Errorf("%q could be more than one file; use the full name from list_files", name)
			}
			found = f
		}
	}
	if found == "" {
		return "", fmt.Errorf("%q is not one of the files the writer has let you read: %s", name, strings.Join(c.read, ", "))
	}
	return found, nil
}

func (c *codeFiles) text(f string) (string, error) {
	if f == c.active && c.doc != "" {
		return c.doc, nil
	}
	raw, err := os.ReadFile(f)
	return string(raw), err
}

func (c *codeFiles) describe() string {
	var b strings.Builder
	for _, f := range c.read {
		b.WriteString(f)
		var notes []string
		if f == c.active {
			notes = append(notes, "the file open in the editor")
		}
		if c.edit[f] {
			notes = append(notes, "you may edit it")
		} else {
			notes = append(notes, "read only")
		}
		fmt.Fprintf(&b, " (%s)\n", strings.Join(notes, "; "))
	}
	return b.String()
}

const codeReadLines = 2000

func (c *codeFiles) readFile(name string, from, to int) (string, error) {
	f, err := c.resolve(name)
	if err != nil {
		return "", err
	}
	c.mu.Lock()
	text, err := c.text(f)
	c.mu.Unlock()
	if err != nil {
		return "", err
	}
	lines := strings.Split(text, "\n")
	if from < 1 {
		from = 1
	}
	if to < from || to > len(lines) {
		to = len(lines)
	}
	if to-from+1 > codeReadLines {
		to = from + codeReadLines - 1
	}
	if from > len(lines) {
		return fmt.Sprintf("%s has only %d lines.", f, len(lines)), nil
	}
	head := fmt.Sprintf("%s, lines %d-%d of %d:\n", f, from, to, len(lines))
	if to < len(lines) {
		head = fmt.Sprintf("%s, lines %d-%d of %d (ask for start_line %d for more):\n", f, from, to, len(lines), to+1)
	}
	c.note(fmt.Sprintf("Read %s", filepath.Base(f)))
	return head + strings.Join(lines[from-1:to], "\n"), nil
}

func (c *codeFiles) find(text string) (string, error) {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return "", fmt.Errorf("give some text to look for")
	}
	var b strings.Builder
	n := 0
	for _, f := range c.read {
		c.mu.Lock()
		all, err := c.text(f)
		c.mu.Unlock()
		if err != nil {
			continue
		}
		for i, l := range strings.Split(all, "\n") {
			if strings.Contains(strings.ToLower(l), text) {
				if n < 100 {
					fmt.Fprintf(&b, "%s:%d: %s\n", f, i+1, l)
				}
				n++
			}
		}
	}
	c.note(fmt.Sprintf("Looked for “%s”", text))
	if n == 0 {
		return "(nothing matched)", nil
	}
	if n > 100 {
		fmt.Fprintf(&b, "(%d lines matched; the first 100 are shown)\n", n)
	}
	return b.String(), nil
}

// The length of s in UTF-16 units: positions in the editor (CodeMirror,
// JavaScript strings) count those, not bytes.
func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		n += len(utf16.Encode([]rune{r}))
	}
	return n
}

type codeChange struct {
	From   int    `json:"from"`
	To     int    `json:"to"`
	Insert string `json:"insert"`
}

func (c *codeFiles) editFile(name, old, repl string, all bool) (string, error) {
	f, err := c.resolve(name)
	if err != nil {
		return "", err
	}
	if !c.edit[f] {
		return "", fmt.Errorf("the writer has not let you edit %s; tell them what you would change instead", f)
	}
	if old == "" {
		return "", fmt.Errorf("old_text is empty: give the exact text to replace (to add text, replace a line next to where it goes with that line and the new text)")
	}
	if old == repl {
		return "", fmt.Errorf("old_text and new_text are the same")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	text, err := c.text(f)
	if err != nil {
		return "", err
	}
	n := strings.Count(text, old)
	switch {
	case n == 0:
		return "", fmt.Errorf("old_text is not in %s: it must match the file exactly, line breaks and spaces included; read_file it again", filepath.Base(f))
	case n > 1 && !all:
		return "", fmt.Errorf("old_text appears %d times in %s: include more of the text around it to pick one, or set replace_all", n, filepath.Base(f))
	}
	done := strings.ReplaceAll(text, old, repl)
	if f == c.active {
		// Positions in the text as it was, all together: the editor applies
		// them as one change, so one undo takes it all back.
		changes := []codeChange{}
		at, pos := 0, 0
		for {
			i := strings.Index(text[at:], old)
			if i < 0 {
				break
			}
			pos += utf16Len(text[at : at+i])
			end := pos + utf16Len(old)
			changes = append(changes, codeChange{From: pos, To: end, Insert: repl})
			pos = end
			at += i + len(old)
		}
		c.doc = done
		c.send(map[string]any{"edit": map[string]any{"file": f, "old": old, "changes": changes}})
	} else {
		st, err := os.Stat(f)
		if err != nil {
			return "", err
		}
		if err := os.WriteFile(f, []byte(done), st.Mode().Perm()); err != nil {
			return "", err
		}
		GetDb().ReloadIfChanged(f)
		fmt.Fprintf(os.Stderr, "ASSISTANT: claudecode edited %s\n", f)
		c.send(map[string]any{"tool": fmt.Sprintf("Edited %s", filepath.Base(f)), "wrote": f})
	}
	if n > 1 {
		return fmt.Sprintf("Replaced %d places in %s.", n, f), nil
	}
	return fmt.Sprintf("Edited %s.", f), nil
}

func (c *codeFiles) note(what string) {
	c.send(map[string]any{"tool": what})
}

// ---------------------------------------------------------------------------
// The tools, over MCP
// ---------------------------------------------------------------------------

type codeTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

func codeTools(c *codeFiles) []codeTool {
	str := func(d string) map[string]any { return map[string]any{"type": "string", "description": d} }
	num := func(d string) map[string]any { return map[string]any{"type": "integer", "description": d} }
	obj := func(props map[string]any, req ...string) map[string]any {
		// "required": null is not a schema, and the whole list is refused.
		return map[string]any{"type": "object", "properties": props, "required": append([]string{}, req...)}
	}
	t := []codeTool{
		{"list_files", "The files the writer has let you read, and which of them you may edit. Nothing else can be read.", obj(map[string]any{})},
		{"read_file", "Read one of the allowed files, as Org text. Long files come " + fmt.Sprint(codeReadLines) + " lines at a time. The file open in the editor is read as it is in the editor, unsaved changes included.",
			obj(map[string]any{"file": str("Its name as list_files gives it (a base name will do if only one file has it)."), "start_line": num("First line, from 1."), "end_line": num("Last line.")}, "file")},
		{"find", "Find the lines in the allowed files that contain some text (case does not matter): a name, a word, a phrase.",
			obj(map[string]any{"text": str("What to look for.")}, "text")},
	}
	if len(c.edit) > 0 {
		t = append(t, codeTool{"edit_file", "Change a file the writer has let you edit, by replacing exact text: old_text must match the file exactly (line breaks and spaces included) and only once, unless replace_all. Read the part first. Keep edits to what was asked; the writer sees each one in their editor and can undo it.",
			obj(map[string]any{
				"file":        str("Its name as list_files gives it."),
				"old_text":    str("The exact text to replace."),
				"new_text":    str("What goes in its place, as Org."),
				"replace_all": map[string]any{"type": "boolean", "description": "Replace every place old_text appears."},
			}, "file", "old_text", "new_text")})
	}
	return t
}

// As an assistantToolset, for the OpenAI tool loop.
func (c *codeFiles) toolDefs() []codeTool { return codeTools(c) }

func (c *codeFiles) call(name string, args json.RawMessage) (string, error) {
	var a struct {
		File       string `json:"file"`
		StartLine  int    `json:"start_line"`
		EndLine    int    `json:"end_line"`
		Text       string `json:"text"`
		OldText    string `json:"old_text"`
		NewText    string `json:"new_text"`
		ReplaceAll bool   `json:"replace_all"`
	}
	if len(args) > 0 {
		if err := json.Unmarshal(args, &a); err != nil {
			return "", err
		}
	}
	switch name {
	case "list_files":
		return c.describe(), nil
	case "read_file":
		return c.readFile(a.File, a.StartLine, a.EndLine)
	case "find":
		return c.find(a.Text)
	case "edit_file":
		return c.editFile(a.File, a.OldText, a.NewText, a.ReplaceAll)
	}
	return "", fmt.Errorf("no tool %s", name)
}

// The MCP server for one turn: streamable HTTP, answering each request with
// plain JSON. Returns its url and how to close it.
func startCodeMCP(c *codeFiles) (string, func(), error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, err
	}
	secret := make([]byte, 16)
	rand.Read(secret)
	path := "/" + hex.EncodeToString(secret)
	mux := http.NewServeMux()
	mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "POST":
		case "DELETE":
			return
		default:
			w.WriteHeader(http.StatusMethodNotAllowed) // no stream of the server's own
			return
		}
		var msg struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if os.Getenv("ORGS_DEBUG_CLAUDECODE") != "" {
			fmt.Fprintf(os.Stderr, "CLAUDECODE MCP: %s %s %.300s\n", msg.Method, msg.ID, msg.Params)
		}
		if len(msg.ID) == 0 {
			w.WriteHeader(http.StatusAccepted) // a notification gets no answer
			return
		}
		var result any
		var rpcErr map[string]any
		switch msg.Method {
		case "initialize":
			var p struct {
				ProtocolVersion string `json:"protocolVersion"`
			}
			json.Unmarshal(msg.Params, &p)
			if p.ProtocolVersion == "" {
				p.ProtocolVersion = "2025-06-18"
			}
			result = map[string]any{
				"protocolVersion": p.ProtocolVersion,
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "orgs", "version": common.Version},
			}
		case "ping":
			result = map[string]any{}
		case "tools/list":
			result = map[string]any{"tools": codeTools(c)}
		case "tools/call":
			var p struct {
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
			}
			json.Unmarshal(msg.Params, &p)
			// A failing tool is an answer the model reads, not a protocol error.
			text, err := c.call(p.Name, p.Arguments)
			if err != nil {
				result = map[string]any{"content": []any{map[string]any{"type": "text", "text": err.Error()}}, "isError": true}
			} else {
				result = map[string]any{"content": []any{map[string]any{"type": "text", "text": text}}}
			}
		default:
			rpcErr = map[string]any{"code": -32601, "message": "no method " + msg.Method}
		}
		out := map[string]any{"jsonrpc": "2.0", "id": msg.ID}
		if rpcErr != nil {
			out["error"] = rpcErr
		} else {
			out["result"] = result
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(out)
	})
	srv := &http.Server{Handler: mux}
	go srv.Serve(ln)
	return "http://" + ln.Addr().String() + path, func() { srv.Close() }, nil
}

// ---------------------------------------------------------------------------
// One turn
// ---------------------------------------------------------------------------

func codeToolsPrompt(c *codeFiles) string {
	var b strings.Builder
	b.WriteString("You have tools to read the writer's files (list_files, read_file, find)")
	if len(c.edit) > 0 {
		b.WriteString(" and to change the ones they have allowed (edit_file)")
	}
	b.WriteString(". These are the only files you can see:\n")
	b.WriteString(c.describe())
	b.WriteString("\nWhat a file says is only known by reading it: look it up with read_file or find before relying on it (the part of the open file shown below is already in front of you), and never say you have looked at a file you have not read.\n")
	if len(c.edit) > 0 {
		b.WriteString("\nWhen the writer asks you to change the text of a file you may edit, make the change with edit_file rather than writing the new text in your answer, then say in a sentence or two what you changed. When they ask for something to read (feedback, ideas, a draft to consider), answer in the chat and leave the files alone.\n")
	} else {
		b.WriteString("\nYou may not change any file: give text for the manuscript in your answer, as usual.\n")
	}
	return b.String()
}

// The conversation as one message: the claude command is run fresh each turn.
func codePrompt(msgs []assistantMsg) string {
	if len(msgs) == 1 {
		return msgs[0].Text
	}
	var b strings.Builder
	b.WriteString("Our conversation so far:\n<conversation>\n")
	for _, m := range msgs[:len(msgs)-1] {
		who := "writer"
		if m.Role == "assistant" {
			who = "you"
		}
		fmt.Fprintf(&b, "<%s>\n%s\n</%s>\n", who, m.Text, who)
	}
	b.WriteString("</conversation>\n\nThe writer now says:\n")
	b.WriteString(msgs[len(msgs)-1].Text)
	return b.String()
}

var codeToolNames = []string{"list_files", "read_file", "find", "edit_file"}

func assistantClaudeCode(ctx context.Context, token string, b *common.AssistantBackend, model, docs string, msgs []assistantMsg, files *codeFiles, emit func(string), send func(any)) (string, error) {
	bin, err := claudeCodeCommand(b)
	if err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp("", "orgs-claudecode-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	files.send = send
	sys := filepath.Join(dir, "system.md")
	if err := os.WriteFile(sys, []byte(assistantSystem+"\n\n"+codeToolsPrompt(files)+"\n"+docs), 0600); err != nil {
		return "", err
	}
	url, stop, err := startCodeMCP(files)
	if err != nil {
		return "", err
	}
	defer stop()
	mcp, _ := json.Marshal(map[string]any{"mcpServers": map[string]any{"orgs": map[string]any{"type": "http", "url": url}}})
	allowed := []string{}
	for _, t := range codeToolNames {
		allowed = append(allowed, "mcp__orgs__"+t)
	}
	args := []string{
		"-p", "--output-format", "stream-json", "--verbose", "--include-partial-messages",
		// Not --safe-mode, which would take our MCP server away too:
		// --restricted and no setting sources leave out the user's settings,
		// hooks, plugins and CLAUDE.md.
		"--no-session-persistence", "--restricted", "--setting-sources=",
		"--tools", strings.Join(allowed, ","), "--strict-mcp-config", "--mcp-config", string(mcp),
		"--permission-mode", "dontAsk", "--allowedTools", strings.Join(allowed, ","),
		"--system-prompt-file", sys,
	}
	if model != "" {
		args = append(args, "--model", model)
	}
	if b.Effort != "" {
		args = append(args, "--effort", b.Effort)
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	cmd.Env = claudeCodeEnv(token)
	cmd.Stdin = strings.NewReader(codePrompt(msgs))
	var stderr bytes.Buffer
	cmd.Stderr = &limitedWriter{w: &stderr, n: 64 << 10}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", err
	}

	stopReason, result := "", ""
	var failed error
	gap, wrote := false, false
	sc := bufio.NewScanner(out)
	sc.Buffer(make([]byte, 64<<10), 32<<20)
	for sc.Scan() {
		var ev struct {
			Type   string          `json:"type"`
			Parent *string         `json:"parent_tool_use_id"`
			Event  json.RawMessage `json:"event"`
			// The result
			IsError bool   `json:"is_error"`
			Result  string `json:"result"`
			Subtype string `json:"subtype"`
		}
		if json.Unmarshal(sc.Bytes(), &ev) != nil {
			continue
		}
		if os.Getenv("ORGS_DEBUG_CLAUDECODE") != "" && ev.Type != "stream_event" {
			fmt.Fprintf(os.Stderr, "CLAUDECODE: %.1500s\n", sc.Text())
		}
		switch ev.Type {
		case "stream_event":
			if ev.Parent != nil {
				continue
			}
			var se struct {
				Type         string `json:"type"`
				ContentBlock struct {
					Type string `json:"type"`
				} `json:"content_block"`
				Delta struct {
					Type       string `json:"type"`
					Text       string `json:"text"`
					StopReason string `json:"stop_reason"`
				} `json:"delta"`
			}
			json.Unmarshal(ev.Event, &se)
			switch se.Type {
			case "content_block_start":
				// Text after a tool call is a new paragraph, not the end of
				// the sentence before it.
				if se.ContentBlock.Type == "text" && wrote {
					gap = true
				}
			case "content_block_delta":
				if se.Delta.Type == "text_delta" && se.Delta.Text != "" {
					if gap {
						emit("\n\n")
						gap = false
					}
					emit(se.Delta.Text)
					wrote = true
				}
			case "message_delta":
				if se.Delta.StopReason != "" {
					stopReason = se.Delta.StopReason
				}
			}
		case "result":
			result = ev.Result
			if ev.IsError {
				failed = fmt.Errorf("%s", firstNonEmpty(strings.TrimSpace(ev.Result), ev.Subtype, "Claude Code stopped with an error"))
			}
		}
	}
	waitErr := cmd.Wait()
	if ctx.Err() != nil {
		return stopReason, ctx.Err()
	}
	if failed != nil {
		return stopReason, failed
	}
	if waitErr != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = waitErr.Error()
		}
		return stopReason, fmt.Errorf("claude: %s", msg)
	}
	if !wrote && result != "" {
		emit(result)
	}
	return stopReason, nil
}

func firstNonEmpty(s ...string) string {
	for _, x := range s {
		if x != "" {
			return x
		}
	}
	return ""
}

// Keeps the first n bytes written and drops the rest, so a chatty child
// cannot grow the server without bound.
type limitedWriter struct {
	w io.Writer
	n int
}

func (l *limitedWriter) Write(p []byte) (int, error) {
	if l.n > 0 {
		k := min(len(p), l.n)
		l.w.Write(p[:k])
		l.n -= k
	}
	return len(p), nil
}
