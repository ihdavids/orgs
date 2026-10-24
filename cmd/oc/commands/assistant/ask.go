package assistant

// orgs ask - worg's writing assistant, from a shell.
//
//	orgs ask "what is weak about this scene" -on 'the ferry'   a heading in scope
//	orgs ask "summarise it" -file novel.org                     the whole file
//	cat draft.txt | orgs ask "tighten this"                     stdin is the passage
//	orgs ask -c "and shorter still"                             carry on the last talk
//	orgs ask -view search -q 'IsStatus("NEXT")' "which are stale?"
//	orgs ask -view search -q 'IsTask()' -edit "tag the stale ones :stale:"
//	orgs ask undo                                               put back what it changed
//	orgs ask status                                             which backends there are
//	orgs ask use ollama qwen3:8b                                 the default backend (and its model), kept in orgs.yaml
//
// It is one request to `POST /assistant/chat`, the endpoint worg's assistant
// pane and its view assistant already use. The prompt, the backends, the keys
// and the tools all live on the server; this side only says what is in scope,
// the way the pane does when it sends the file, the section the cursor is in
// or the selection.
//
// The answer is newline-delimited json, a piece of text at a time, so it is
// written to stdout as it arrives rather than after a local model has spent a
// minute thinking. Everything that is not the answer - "Read notes.org", a
// warning that the text will not fit in the context, a heading the view
// assistant changed - goes to stderr (see **Traps: stdout is the answer**),
// so `orgs ask … > reply.org` holds the reply and nothing else.
//
// A view turn with -edit is the only one that writes, and it can write a lot:
// the server keeps an undo for it and ends the stream with its token. That
// token is kept beside the last conversation in the user cache, which is what
// lets `orgs ask undo` take no argument.

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

type Ask struct {
	tf commands.TargetFlags

	On       string
	File     string
	Read     string
	Backend  string
	Model    string
	NumCtx   int
	Continue bool
	View     string
	Query    string
	Edit     bool
}

func (self *Ask) Unmarshal(u func(interface{}) error) error { return u(self) }
func (self *Ask) StartPlugin(m *common.PluginManager)       {}
func (self *Ask) HelpGroup() string                         { return "Writing" }

func (self *Ask) SetupParameters(fset *flag.FlagSet) {
	fset.StringVar(&self.On, "on", "", "the heading in scope: a query, a hash, or words of its title")
	fset.StringVar(&self.File, "file", "", "the org file in scope (the whole of it)")
	fset.StringVar(&self.Read, "read", "", "other org files it may read, comma separated, or all for every org file the server knows")
	fset.StringVar(&self.Backend, "backend", "", "claude, chatgpt, gemini, claudecode, ollama or local (else the server's default)")
	fset.StringVar(&self.Model, "model", "", "a model the backend offers (orgs ask status lists them)")
	fset.IntVar(&self.NumCtx, "ctx", 0, "ollama and local: the context window to ask for, in tokens")
	fset.BoolVar(&self.Continue, "c", false, "carry on the last conversation")
	fset.StringVar(&self.View, "view", "", "talk about a view instead: search, gantt, kanban or columns (with -file)")
	fset.StringVar(&self.Query, "q", "", "the query the view shows")
	fset.BoolVar(&self.Edit, "edit", false, "with -view: let it change headings (orgs ask undo puts them back)")
	commands.AddTargetFlags(fset, &self.tf)
}

// The request, as the server's assistantReq spells it.
type msg struct {
	Role string
	Text string
}

type view struct {
	Kind     string
	Title    string   `json:",omitempty"`
	Query    string   `json:",omitempty"`
	File     string   `json:",omitempty"`
	Edit     bool     `json:",omitempty"`
	Selected []string `json:",omitempty"`
}

type chatReq struct {
	Backend   string   `json:",omitempty"`
	Model     string   `json:",omitempty"`
	Messages  []msg
	File      string   `json:",omitempty"`
	Title     string   `json:",omitempty"`
	Scope     string   `json:",omitempty"`
	Context   string   `json:",omitempty"`
	Section   string   `json:",omitempty"`
	Selection string   `json:",omitempty"`
	Others    []string `json:",omitempty"`
	NumCtx    int      `json:",omitempty"`
	View      *view    `json:",omitempty"`
}

// The whole of an answer, for -json and -format, and what is kept for -c.
type Answer struct {
	Ok      bool
	Text    string
	Model   string   `json:",omitempty"`
	Stop    string   `json:",omitempty"`
	Error   string   `json:",omitempty"`
	Tools   []string `json:",omitempty"`
	Wrote   []string `json:",omitempty"`
	Queries []Query  `json:",omitempty"`
	Undo    string   `json:",omitempty"`
}

type Query struct {
	Query string
	Why   string
	Count int
}

func (self *Ask) Exec(core *commands.Core) {
	words := commands.FreeArgs(commands.Find("ask").Flags)
	// A subcommand only when it is the whole line: `orgs ask status of the
	// launch` is a question.
	if len(words) == 1 && words[0] == "status" {
		status(core)
		return
	}
	if len(words) >= 1 && words[0] == "undo" && len(words) <= 2 {
		tok := ""
		if len(words) == 2 {
			tok = words[1]
		}
		undo(core, tok)
		return
	}
	// Only when the next word is a backend: `orgs ask use active voice?` is a question.
	if len(words) >= 2 && len(words) <= 3 && words[0] == "use" && backends[strings.ToLower(words[1])] {
		model := ""
		if len(words) == 3 {
			model = words[2]
		}
		use(core, words[1], model)
		return
	}

	question := strings.TrimSpace(strings.Join(words, " "))
	if question == "" {
		commands.Fail("ask what? orgs ask \"your question\" [-on heading] [-file f.org]")
	}

	var req chatReq
	last := loadLast()
	if self.Continue {
		if last == nil {
			commands.Fail("no conversation to carry on")
		}
		req = last.Req
		req.Messages = append([]msg{}, last.Req.Messages...)
	}
	if self.Backend != "" {
		req.Backend = self.Backend
	}
	if self.Model != "" {
		req.Model = self.Model
	}
	if self.NumCtx > 0 {
		req.NumCtx = self.NumCtx
	}
	// A new scope on a carried-on conversation replaces the old one: the
	// writer moved, and the next answer should be about where they are.
	if !self.Continue || self.On != "" || self.File != "" || self.View != "" {
		self.scope(core, &req)
	}
	readAll := false
	if self.Read != "" {
		names := []string{}
		for _, n := range splitList(self.Read) {
			if n == "all" || n == "*" {
				readAll = true
				continue
			}
			names = append(names, n)
		}
		req.Others = commands.ResolveOrgFiles(core, names)
		if readAll {
			req.Others = append(req.Others, commands.SendReceiveGetOr[common.FileList](core, "files", nil)...)
		}
		req.Others = distinct(req.Others)
	}
	// Stdin is the passage, the way a selection is in the pane. It is read
	// only when it is a pipe, so a terminal is never waited on.
	if commands.StdinIsPipe() {
		if b, err := io.ReadAll(os.Stdin); err == nil && len(bytes.TrimSpace(b)) > 0 {
			req.Selection = string(b)
			if req.Scope == "" {
				req.Scope = "none"
			}
		}
	}
	if req.Scope == "" && req.View == nil {
		req.Scope = "none"
	}
	if req.File == "" && req.View == nil {
		req.File = anchorFile(core, req.Backend)
	}
	// The file in scope is already the turn's own; named again among the
	// others, a backend without tools would be sent its text twice.
	if req.File != "" && len(req.Others) > 0 {
		kept := []string{}
		for _, o := range req.Others {
			if o != req.File {
				kept = append(kept, o)
			}
		}
		req.Others = kept
	}
	if readAll {
		warnReadAll(core, &req)
	}
	req.Messages = append(req.Messages, msg{Role: "user", Text: question})

	if commands.Wrote("POST /assistant/chat", shortened(req)) {
		return
	}
	ans := chat(core, &req)
	if ans.Text != "" {
		req.Messages = append(req.Messages, msg{Role: "assistant", Text: ans.Text})
	}
	saveLast(&lastTalk{Req: req, Undo: ans.Undo})

	if commands.RenderOne(ans, nil) {
		if !ans.Ok {
			os.Exit(1)
		}
		return
	}
	// Said before a failure too: a turn that broke off part way may already
	// have changed headings, and the server's undo holds those as well.
	if len(ans.Wrote) > 0 {
		hint := " - orgs ask undo puts it back"
		if ans.Undo == "" {
			hint = ""
		}
		fmt.Fprintf(os.Stderr, "%schanged %s%s%s\n",
			commands.C(commands.AnsiGold), strings.Join(uniq(ans.Wrote), ", "), hint, commands.C(commands.AnsiReset))
	}
	if !ans.Ok {
		commands.Fail("%s", ans.Error)
	}
}

// scope fills in what the writer is working on: a heading (the pane's
// "section"), a file, or a view.
func (self *Ask) scope(core *commands.Core, req *chatReq) {
	req.File, req.Title, req.Scope, req.Context, req.Section, req.View = "", "", "", "", "", nil
	if self.View != "" {
		v := &view{Kind: self.View, Query: self.Query, Edit: self.Edit}
		switch self.View {
		case "columns":
			if self.File == "" {
				commands.Fail("-view columns needs -file")
			}
			v.File = commands.ResolveOrgFile(core, self.File)
			v.Title = filepath.Base(v.File)
		case "search", "gantt", "kanban":
			if v.Query == "" {
				v.Query = self.On
			}
			if v.Query == "" {
				commands.Fail("-view %s needs -q, the query it shows", self.View)
			}
			v.Query = asQuery(v.Query)
			v.Title = v.Query
		default:
			commands.Fail("no view %s: search, gantt, kanban or columns", self.View)
		}
		req.View = v
		return
	}
	if self.Edit {
		commands.Fail("-edit is for -view; in writing mode the answer is only text")
	}
	switch {
	case self.On != "":
		var target []string
		if commands.LooksLikeHash(self.On) {
			target = []string{self.On}
		} else {
			target = []string{asQuery(self.On)}
		}
		t := commands.Resolve(core, &self.tf, target, commands.TargetOpts{
			Prompt: "Ask about> ", Multi: false, Quiet: true,
		})[0]
		text := fileText(core, t.Filename)
		req.File = t.Filename
		req.Title = filepath.Base(t.Filename)
		req.Scope = "section"
		req.Context = subtree(text, t)
		req.Section = t.Headline
		if t.Parent != "" {
			req.Section = t.Parent + " / " + t.Headline
		}
	case self.File != "":
		f := commands.ResolveOrgFile(core, self.File)
		req.File = f
		req.Title = filepath.Base(f)
		req.Scope = "file"
		req.Context = fileText(core, f)
	}
}

// warnReadAll says what `-read all` will cost, on stderr. A backend with
// tools (list_files, read_file, find) is only told the names and reads what
// it needs, so every org file is cheap; one without has each file's text put
// in the prompt of every turn, cut off at the server's assistant.maxContext,
// which is slow, costly on a paid backend, and drops whatever comes after the
// cut. Said rather than refused: a small set of files is fine either way.
func warnReadAll(core *commands.Core, req *chatReq) {
	if commands.JsonOut {
		return
	}
	b, ok := backendOf(core, req.Backend)
	n := len(req.Others)
	switch {
	case !ok:
		fmt.Fprintf(os.Stderr, "%sreading %d org files%s\n", commands.C(commands.AnsiDim), n, commands.C(commands.AnsiReset))
	case b.Tools:
		fmt.Fprintf(os.Stderr, "%s%s may read %d org files, as it needs them%s\n", commands.C(commands.AnsiDim), b.Name, n, commands.C(commands.AnsiReset))
	default:
		fmt.Fprintf(os.Stderr, "%s%s has no tools, so the text of %d org files goes in the prompt of every turn, cut at the server's assistant.maxContext; a backend with tools reads only what it needs%s\n",
			commands.C(commands.AnsiGold), b.Name, n, commands.C(commands.AnsiReset))
	}
}

// backendOf is the status of the backend a turn will go to: the one named,
// else the server's default.
func backendOf(core *commands.Core, backend string) (backendStatus, bool) {
	st, err := commands.SendReceiveGetErr[struct {
		Default  string
		Backends []backendStatus
	}](core, "assistant/status", nil)
	if err != nil {
		return backendStatus{}, false
	}
	if backend == "" {
		backend = st.Default
	}
	for _, b := range st.Backends {
		if b.Name == backend {
			return b, true
		}
	}
	return backendStatus{}, false
}

// anchorFile is a file for a turn about nothing in particular - stdin, or a
// bare question - when the backend is one with tools. The server opens a
// tool backend's file set on the request's File and refuses an empty one,
// since worg always has a file open; so a turn with none names the first
// file the server knows. Scope stays "none", so none of its text goes in the
// prompt, and Edit stays empty, so nothing can be written.
func anchorFile(core *commands.Core, backend string) string {
	if b, ok := backendOf(core, backend); ok && b.Tools {
		if files := commands.SendReceiveGetOr[common.FileList](core, "files", nil); len(files) > 0 {
			return files[0]
		}
	}
	return ""
}

// Plain words become a title match, the way `orgs mentions` reads them; a
// real query (it has a bracket or an operator) goes as it is.
func asQuery(s string) string {
	if strings.ContainsAny(s, "()&|!") {
		return s
	}
	return fmt.Sprintf("MatchHeadline(%q)", "(?i)"+regexp.QuoteMeta(s))
}

func fileText(core *commands.Core, file string) string {
	res, err := commands.SendReceiveGetErr[common.ResultMsg](core, "orgfile", map[string]string{"filename": file})
	if err != nil || !res.Ok {
		commands.Fail("could not read %s: %v%s", file, err, res.Msg)
	}
	return res.Msg
}

var starsRe = regexp.MustCompile(`^(\*+)\s`)

// subtree is the heading's own lines and everything under it. The heading is
// looked for near the line the server gave, by its title, rather than trusted
// to be exactly there: a file edited since the last index moves it a line or
// two, and a section that starts on the wrong heading is a wrong answer that
// looks right.
func subtree(text string, t common.Todo) string {
	lines := strings.Split(text, "\n")
	start := -1
	for d := 0; d < 50 && start < 0; d++ {
		for _, i := range []int{t.LineNum - 1 + d, t.LineNum - 1 - d, t.LineNum + d} {
			if i >= 0 && i < len(lines) && starsRe.MatchString(lines[i]) && strings.Contains(lines[i], t.Headline) {
				start = i
				break
			}
		}
	}
	if start < 0 {
		commands.Fail("could not find %q in %s; has it moved?", t.Headline, t.Filename)
	}
	lvl := len(starsRe.FindStringSubmatch(lines[start])[1])
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if m := starsRe.FindStringSubmatch(lines[i]); m != nil && len(m[1]) <= lvl {
			end = i
			break
		}
	}
	return strings.Join(lines[start:end], "\n")
}

// chat sends the turn and reads the answer as it comes.
func chat(core *commands.Core, req *chatReq) Answer {
	ans := Answer{}
	body, _ := json.Marshal(req)
	hr, err := http.NewRequest("POST", core.Rest.Url+"/assistant/chat", bytes.NewReader(body))
	if err != nil {
		commands.Fail("%v", err)
	}
	hr.Header = core.Rest.Header.Clone()
	hr.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(hr)
	if err != nil {
		commands.Fail("could not reach the server: %v", err)
	}
	defer resp.Body.Close()
	// A refusal before the stream starts (no key, no such model, the
	// assistant turned off) is a plain {Ok, Msg}.
	if !strings.Contains(resp.Header.Get("Content-Type"), "ndjson") {
		var rm common.ResultMsg
		raw, _ := io.ReadAll(resp.Body)
		if err := json.Unmarshal(raw, &rm); err != nil || rm.Msg == "" {
			ans.Error = strings.TrimSpace(fmt.Sprintf("%s %s", resp.Status, string(raw)))
		} else {
			ans.Error = rm.Msg
		}
		return ans
	}
	live := !commands.Machine()
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
	ended := false
	for sc.Scan() {
		var ev struct {
			T     string `json:"t"`
			Done  bool   `json:"done"`
			Model string `json:"model"`
			Stop  string `json:"stop"`
			Error string `json:"error"`
			Tool  string `json:"tool"`
			Warn  bool   `json:"warn"`
			Wrote string `json:"wrote"`
			Query string `json:"query"`
			Why   string `json:"why"`
			Count int    `json:"count"`
			Undo  string `json:"undo"`
		}
		if json.Unmarshal(sc.Bytes(), &ev) != nil {
			continue
		}
		if ev.T != "" {
			ans.Text += ev.T
			if live {
				fmt.Print(ev.T)
			}
		}
		if ev.Tool != "" {
			ans.Tools = append(ans.Tools, ev.Tool)
			if live {
				col := commands.AnsiDim
				if ev.Warn {
					col = commands.AnsiGold
				}
				fmt.Fprintf(os.Stderr, "%s· %s%s\n", commands.C(col), ev.Tool, commands.C(commands.AnsiReset))
			}
		}
		if ev.Wrote != "" {
			ans.Wrote = append(ans.Wrote, ev.Wrote)
		}
		if ev.Query != "" {
			ans.Queries = append(ans.Queries, Query{Query: ev.Query, Why: ev.Why, Count: ev.Count})
			if live {
				fmt.Fprintf(os.Stderr, "%s· query (%d): %s%s  %s\n", commands.C(commands.AnsiCyan), ev.Count,
					ev.Query, commands.C(commands.AnsiReset), ev.Why)
			}
		}
		if ev.Undo != "" {
			ans.Undo = ev.Undo
		}
		if ev.Error != "" {
			ans.Error = ev.Error
			ended = true
		}
		if ev.Done {
			ans.Ok, ans.Model, ans.Stop = true, ev.Model, ev.Stop
			ended = true
		}
	}
	if live && ans.Text != "" && !strings.HasSuffix(ans.Text, "\n") {
		fmt.Println()
	}
	if !ended && ans.Error == "" {
		ans.Error = "the answer stopped part way"
		if err := sc.Err(); err != nil {
			ans.Error += ": " + err.Error()
		}
	}
	if ans.Stop == "max_tokens" || ans.Stop == "length" {
		fmt.Fprintf(os.Stderr, "%s(cut short: the model ran out of room)%s\n", commands.C(commands.AnsiGold), commands.C(commands.AnsiReset))
	}
	return ans
}

var backends = map[string]bool{"claude": true, "chatgpt": true, "gemini": true, "claudecode": true, "ollama": true, "local": true}

func use(core *commands.Core, backend, model string) {
	req := struct{ Backend, Model string }{backend, model}
	res, err := commands.SendReceivePostErr[struct{ Backend, Model string }, common.ResultMsg](core, "assistant/default", &req)
	if err == commands.ErrDryRun {
		return
	}
	if err != nil {
		commands.Fail("%v", err)
	}
	if commands.RenderOne(res, nil) {
		if !res.Ok {
			os.Exit(1)
		}
		return
	}
	if !res.Ok {
		commands.Fail("%s", res.Msg)
	}
	fmt.Println(res.Msg)
}

func undo(core *commands.Core, tok string) {
	last := loadLast()
	if tok == "" && last != nil {
		tok = last.Undo
	}
	if tok == "" {
		commands.Fail("nothing to undo: the last answer changed nothing")
	}
	res, err := commands.SendReceivePostErr[struct{ Token string }, common.ResultMsg](core, "assistant/undo", &struct{ Token string }{tok})
	if err == commands.ErrDryRun {
		return
	}
	if err != nil {
		commands.Fail("%v", err)
	}
	if res.Ok && last != nil && last.Undo == tok {
		last.Undo = ""
		saveLast(last)
	}
	if commands.RenderOne(res, nil) {
		if !res.Ok {
			os.Exit(1)
		}
		return
	}
	if !res.Ok {
		commands.Fail("%s", res.Msg)
	}
	fmt.Println(res.Msg)
}

type backendStatus struct {
	Name, Label, Model string
	Models             []string
	Ready              bool
	Msg, Source, Where string
	Tools, Views       bool
	ViewTools          bool
	Default            bool
}

func status(core *commands.Core) {
	st, err := commands.SendReceiveGetErr[struct {
		Ok       bool
		Off      bool
		Default  string
		Backends []backendStatus
	}](core, "assistant/status", nil)
	if err != nil {
		commands.Fail("could not ask the server: %v", err)
	}
	for i := range st.Backends {
		st.Backends[i].Default = st.Backends[i].Name == st.Default
	}
	if commands.Render(st.Backends, nil) {
		return
	}
	if st.Off {
		fmt.Println("the writing assistant is turned off on this server (assistant.backend: off)")
		return
	}
	if len(st.Backends) == 0 {
		fmt.Println("no backends: set one up under assistant: in orgs.yaml")
		return
	}
	for _, b := range st.Backends {
		mark := commands.C(commands.AnsiRed) + "✗" + commands.C(commands.AnsiReset)
		if b.Ready {
			mark = commands.C(commands.AnsiGreen) + "✓" + commands.C(commands.AnsiReset)
		}
		def := " "
		if b.Default {
			def = "*"
		}
		can := []string{}
		if b.Tools {
			can = append(can, "files")
		}
		if b.Views {
			v := "views"
			if b.ViewTools {
				v += "+edit"
			}
			can = append(can, v)
		}
		fmt.Printf("%s %s %-10s %-28s %s%-8s %-12s %s%s\n", mark, def, b.Name, commands.Ellipsis(b.Model, 28),
			commands.C(commands.AnsiDim), b.Where, b.Source, strings.Join(can, ","), commands.C(commands.AnsiReset))
		if !b.Ready && b.Msg != "" {
			fmt.Printf("      %s%s%s\n", commands.C(commands.AnsiGold), b.Msg, commands.C(commands.AnsiReset))
		}
		if len(b.Models) > 1 {
			fmt.Printf("      %salso: %s%s\n", commands.C(commands.AnsiDim), strings.Join(b.Models[1:], ", "), commands.C(commands.AnsiReset))
		}
	}
}

// The last conversation, kept so -c can carry it on and undo can find its
// token. One file, overwritten each turn: this is a scratch pad, not a log.
type lastTalk struct {
	Req  chatReq
	Undo string `json:",omitempty"`
}

func lastPath() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "orgs", "ask-last.json")
}

func loadLast() *lastTalk {
	p := lastPath()
	if p == "" {
		return nil
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	var l lastTalk
	if json.Unmarshal(raw, &l) != nil {
		return nil
	}
	return &l
}

func saveLast(l *lastTalk) {
	p := lastPath()
	if p == "" {
		return
	}
	os.MkdirAll(filepath.Dir(p), 0700)
	if raw, err := json.Marshal(l); err == nil {
		os.WriteFile(p, raw, 0600)
	}
}

// What -dry-run shows: the request with the long text cut down, since the
// point is to see what would be asked, not to read the file back.
func shortened(r chatReq) chatReq {
	cut := func(s string) string {
		if len(s) <= 200 {
			return s
		}
		return fmt.Sprintf("%s… (%d characters)", s[:200], len(s))
	}
	r.Context, r.Selection = cut(r.Context), cut(r.Selection)
	ms := []msg{}
	for _, m := range r.Messages {
		ms = append(ms, msg{Role: m.Role, Text: cut(m.Text)})
	}
	r.Messages = ms
	return r
}

func splitList(s string) []string {
	out := []string{}
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// distinct drops repeats and keeps the full paths: the server finds a bare
// name by its first file with that base name, so two notes.org in different
// folders would read as one.
func distinct(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func uniq(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, filepath.Base(s))
		}
	}
	return out
}

func init() {
	commands.AddCmd("ask", "ask the writing assistant about a heading, a file, stdin or a view; undo, status, use",
		func() commands.Cmd { return &Ask{} })
	commands.AddCmd("models", "the assistant's local models: list, pull, start, stop, delete",
		func() commands.Cmd { return &Models{} })
}
