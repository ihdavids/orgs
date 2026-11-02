package lsp

// `orgs lsp`: a language server for org files, built on the parser the orgs
// server uses.
//
//	orgs lsp                        speak LSP on stdin and stdout
//	orgs lsp -lint off              without the spelling and grammar checkers
//	orgs lsp -lint all -lang en-GB  both checkers, British English
//	orgs -url https://box:8010 lsp -path-map /srv/org=/Users/me/org
//
// Like `orgs mcp` it is a client of a running server, not a second one: the
// outline, folding, completion and links within a file are worked out here
// from the buffer (go-org, as the server reads it), and everything that needs
// the other files or the server's rules - tags and ids in use, refile targets,
// running a block, the checkers - is one request to it. With no server the
// in-file features still work.
//
// Features: completion (TODO keywords, tags, link targets, id: links, #+
// keywords, block languages, property names), go-to-definition on links,
// document links, document and workspace symbols, folding, diagnostics
// (broken links, unclosed blocks and drawers, Harper/LanguageTool prose
// lints with their fixes as quick fixes), code actions (schedule, deadline,
// refile, archive, run a block) and a run lens over source blocks.
//
// ---------------------------------------------------------------------------
// The buffer is the truth
//
// An editor holds a file the server has not seen. Schedule, deadline and run
// therefore send the buffer to the server (/lsp/date, /lsp/run), which makes
// the edit it would make to the file and answers with the text; the
// difference comes back to the editor as one undoable edit and the file on
// disk is left alone. Refile and archive change two files on the server's
// disk, so they refuse unless the buffer matches the file (save first), and
// then wait for the editor to reload what changed, editing the buffer to
// match only if it does not.
//
// Stdout is the protocol (see Traps: stdout is the answer).

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/cmd/oc/commands/lint"
	"github.com/ihdavids/orgs/internal/common"
)

type Lsp struct {
	Lint    string
	Lang    string
	Dialect string
	Ignore  string
	PathMap string
	Depth   int
}

func (self *Lsp) Unmarshal(unmarshal func(interface{}) error) error { return unmarshal(self) }
func (self *Lsp) StartPlugin(manager *common.PluginManager)       {}
func (self *Lsp) HelpGroup() string                                 { return "Editors" }

// No server is needed to start: an editor that launches this before the
// server is up still gets the outline, folding and the links within a file,
// and the token is renewed from here (core.Refresh) rather than checked once.
func (self *Lsp) NeedsNoServer() bool { return true }

func (self *Lsp) SetupParameters(fset *flag.FlagSet) {
	fset.StringVar(&self.Lint, "lint", "auto", "prose checkers: auto (the first the server offers), harper, languagetool, all or off")
	fset.StringVar(&self.Lang, "lang", "en-US", "LanguageTool's language")
	fset.StringVar(&self.Dialect, "dialect", "american", "Harper's English: american, british, australian, canadian, indian")
	fset.StringVar(&self.Ignore, "ignore", "", "lint rules to leave out, comma separated")
	fset.StringVar(&self.PathMap, "path-map", "", "SERVER=LOCAL path prefixes, comma separated, for a server on another machine")
	fset.IntVar(&self.Depth, "refile-depth", 3, "how deep refile targets go")
}

// options are the flags, which an editor can also send as initializationOptions
// (the same names: lint, lang, dialect, ignore, pathMap, refileDepth).
type options struct {
	Lint        string            `json:"lint"`
	Lang        string            `json:"lang"`
	Dialect     string            `json:"dialect"`
	Ignore      []string          `json:"ignore"`
	PathMap     map[string]string `json:"pathMap"`
	RefileDepth int               `json:"refileDepth"`
	// The token to send, from an editor that logs in itself (VS Code's
	// extension). Never a flag: a flag shows in ps. Renewed by the editor,
	// which sends the new one with workspace/didChangeConfiguration.
	Token string `json:"token"`
}

type server struct {
	core *commands.Core
	c    *conn
	api  *api
	opt  options
	enc  enc
	caps clientCaps

	mu   sync.Mutex
	docs map[string]*document

	// The server's keywords, for a file without #+TODO.
	todo string

	// Prose checkers to run, settled at startup.
	checkers []string

	dmu     sync.Mutex
	timers  map[string]*time.Timer
	diagFor map[string]diagSet

	// Refile targets picked this session, newest first.
	rmu    sync.Mutex
	recent []refileTarget

	initialized bool
	shutdown    bool
	// The token came from the editor (initializationOptions).
	clientToken bool
}

// setToken uses a token the editor sent, under the lock a renewal takes.
func (s *server) setToken(t string) {
	s.api.mu.Lock()
	s.core.Rest.Header.Set("Authorization", "Bearer "+t)
	s.clientToken = true
	s.api.mu.Unlock()
	s.api.forget()
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}

type clientCaps struct {
	documentChanges bool
	disabledActions bool
	snippets        bool
}

func (self *Lsp) Exec(core *commands.Core) {
	opt := options{Lint: self.Lint, Lang: self.Lang, Dialect: self.Dialect, RefileDepth: self.Depth, PathMap: map[string]string{}}
	for _, r := range strings.Split(self.Ignore, ",") {
		if r = strings.TrimSpace(r); r != "" {
			opt.Ignore = append(opt.Ignore, r)
		}
	}
	for _, pm := range strings.Split(self.PathMap, ",") {
		if k, v, ok := strings.Cut(strings.TrimSpace(pm), "="); ok {
			opt.PathMap[k] = v
		}
	}
	core.Rest.Timeout = 10 * time.Second
	// The protocol keeps the real stdout; everything else that prints - a
	// library's fmt.Println, go-org's recovered-panic stack trace - goes to
	// stderr, where the editor logs it, instead of into the stream, where the
	// editor reads it as a broken message and stops the server.
	out := bufio.NewWriter(os.Stdout)
	os.Stdout = os.Stderr
	s := newServer(core, newConn(os.Stdin, out), opt)
	os.Exit(s.run())
}

func newServer(core *commands.Core, c *conn, opt options) *server {
	return &server{core: core, c: c, api: newApi(core), opt: opt, enc: utf16Enc,
		docs: map[string]*document{}, timers: map[string]*time.Timer{}, diagFor: map[string]diagSet{},
		todo: "TODO | DONE"}
}

// run reads until the client goes, and is the exit code: 0 after a shutdown,
// 1 when the client left without one (the protocol's rule).
func (s *server) run() int {
	for {
		m, err := s.c.read()
		if err == io.EOF {
			if s.shutdown {
				return 0
			}
			return 1
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "orgs lsp:", err)
			return 1
		}
		if m.Error != nil && m.Method == "" && m.Id == nil {
			s.c.reply(nil, nil, m.Error)
			continue
		}
		if s.c.answered(m) {
			continue
		}
		if m.Method == "exit" {
			if s.shutdown {
				return 0
			}
			return 1
		}
		if m.Method == "initialize" {
			// In order, before anything that depends on the encoding it
			// settles.
			s.request(m)
			continue
		}
		if m.Id == nil {
			// Notifications are handled in order, here, so a request that
			// follows an edit sees the edit.
			s.notification(m)
			continue
		}
		go s.request(m)
	}
}

func (s *server) request(m message) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "orgs lsp: %s failed: %v\n", m.Method, r)
			s.c.reply(m.Id, nil, &rpcError{Code: errInternal, Message: fmt.Sprint(r)})
		}
	}()
	if !s.initialized && m.Method != "initialize" {
		s.c.reply(m.Id, nil, &rpcError{Code: errNotInitialized, Message: "initialize first"})
		return
	}
	var res interface{}
	var rerr *rpcError
	bad := func(err error) { rerr = &rpcError{Code: errInvalidParams, Message: err.Error()} }
	switch m.Method {
	case "initialize":
		res = s.initialize(m.Params)
	case "shutdown":
		s.shutdown = true
		res = nil
	case "textDocument/documentSymbol":
		var p struct{ TextDocument textDocumentId }
		if err := json.Unmarshal(m.Params, &p); err != nil {
			bad(err)
			break
		}
		if d := s.doc(p.TextDocument.URI); d != nil {
			res = s.symbols(d)
		} else {
			res = []DocumentSymbol{}
		}
	case "textDocument/foldingRange":
		var p struct{ TextDocument textDocumentId }
		if err := json.Unmarshal(m.Params, &p); err != nil {
			bad(err)
			break
		}
		if d := s.doc(p.TextDocument.URI); d != nil {
			res = s.folding(d)
		} else {
			res = []FoldingRange{}
		}
	case "textDocument/completion":
		var p positionParams
		if err := json.Unmarshal(m.Params, &p); err != nil {
			bad(err)
			break
		}
		res = s.complete(p)
	case "textDocument/definition":
		var p positionParams
		if err := json.Unmarshal(m.Params, &p); err != nil {
			bad(err)
			break
		}
		res = s.definition(p)
	case "textDocument/documentLink":
		var p struct{ TextDocument textDocumentId }
		if err := json.Unmarshal(m.Params, &p); err != nil {
			bad(err)
			break
		}
		res = s.documentLinks(p.TextDocument.URI)
	case "documentLink/resolve":
		var l DocumentLink
		if err := json.Unmarshal(m.Params, &l); err != nil {
			bad(err)
			break
		}
		res = s.resolveLink(l)
	case "workspace/symbol":
		var p struct{ Query string }
		if err := json.Unmarshal(m.Params, &p); err != nil {
			bad(err)
			break
		}
		res = s.workspaceSymbols(p.Query)
	case "textDocument/codeAction":
		var p codeActionParams
		if err := json.Unmarshal(m.Params, &p); err != nil {
			bad(err)
			break
		}
		res = s.codeActions(p)
	case "textDocument/codeLens":
		var p struct{ TextDocument textDocumentId }
		if err := json.Unmarshal(m.Params, &p); err != nil {
			bad(err)
			break
		}
		res = s.codeLenses(p.TextDocument.URI)
	case "workspace/executeCommand":
		var p struct {
			Command   string
			Arguments []json.RawMessage
		}
		if err := json.Unmarshal(m.Params, &p); err != nil {
			bad(err)
			break
		}
		res, rerr = s.execute(p.Command, p.Arguments)
	default:
		rerr = &rpcError{Code: errMethodNotFound, Message: "orgs lsp does not do " + m.Method}
	}
	s.c.reply(m.Id, res, rerr)
}

func (s *server) notification(m message) {
	switch m.Method {
	case "initialized":
		go s.started()
	case "textDocument/didOpen":
		var p struct {
			TextDocument struct {
				URI     string
				Version int
				Text    string
			}
		}
		if json.Unmarshal(m.Params, &p) != nil {
			return
		}
		d := newDocument(p.TextDocument.URI, p.TextDocument.Version, p.TextDocument.Text)
		s.mu.Lock()
		s.docs[d.uri] = d
		s.mu.Unlock()
		s.diagnoseSoon(d.uri, 0)
	case "textDocument/didChange":
		var p struct {
			TextDocument struct {
				URI     string
				Version int
			}
			ContentChanges []struct {
				Range *Range
				Text  string
			}
		}
		if json.Unmarshal(m.Params, &p) != nil {
			return
		}
		s.mu.Lock()
		old := s.docs[p.TextDocument.URI]
		if old == nil {
			s.mu.Unlock()
			return
		}
		text := old.text
		for _, ch := range p.ContentChanges {
			text = s.enc.apply(text, ch.Range, ch.Text)
		}
		s.docs[old.uri] = newDocument(old.uri, p.TextDocument.Version, text)
		s.mu.Unlock()
		s.diagnoseSoon(old.uri, 700*time.Millisecond)
	case "workspace/didChangeConfiguration":
		// {"settings": {"token": "..."}}, or under "orgs": a renewed login.
		var p struct {
			Settings struct {
				Token string
				Orgs  struct{ Token string }
			}
		}
		if json.Unmarshal(m.Params, &p) == nil {
			if t := firstNonEmpty(p.Settings.Token, p.Settings.Orgs.Token); t != "" {
				s.setToken(t)
			}
		}
	case "textDocument/didSave":
		var p struct{ TextDocument textDocumentId }
		if json.Unmarshal(m.Params, &p) == nil {
			s.diagnoseSoon(p.TextDocument.URI, 0)
		}
	case "textDocument/didClose":
		var p struct{ TextDocument textDocumentId }
		if json.Unmarshal(m.Params, &p) != nil {
			return
		}
		s.mu.Lock()
		delete(s.docs, p.TextDocument.URI)
		s.mu.Unlock()
		s.dmu.Lock()
		if t := s.timers[p.TextDocument.URI]; t != nil {
			t.Stop()
		}
		delete(s.diagFor, p.TextDocument.URI)
		s.dmu.Unlock()
		s.c.notify("textDocument/publishDiagnostics", map[string]interface{}{"uri": p.TextDocument.URI, "diagnostics": []Diagnostic{}})
	}
}

func (s *server) doc(uri string) *document {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.docs[uri]
}

func (s *server) docByPath(path string) *document {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, d := range s.docs {
		if d.path == path {
			return d
		}
	}
	return nil
}

// orgConfig is go-org's configuration as the server has it (GetConfig in
// internal/app/orgs/orgdb.go), with the server's keywords.
func (s *server) orgConfig() *org.Configuration {
	c := org.New()
	c.AutoLink = true
	c.MaxEmphasisNewLines = 1
	c.DefaultSettings["TODO"] = s.todo
	return c
}

func (s *server) initialize(raw json.RawMessage) interface{} {
	var p struct {
		Capabilities struct {
			General struct {
				PositionEncodings []string
			}
			Workspace struct {
				WorkspaceEdit struct {
					DocumentChanges bool
				}
			}
			TextDocument struct {
				CodeAction struct {
					DisabledSupport bool
				}
				Completion struct {
					CompletionItem struct {
						SnippetSupport bool
					}
				}
			}
			Window struct {
				ShowMessage struct {
					MessageActionItem *struct{} `json:"messageActionItem"`
				}
			}
		}
		InitializationOptions *options
	}
	json.Unmarshal(raw, &p)
	for _, e := range p.Capabilities.General.PositionEncodings {
		if e == string(utf8Enc) {
			s.enc = utf8Enc
		}
	}
	s.caps = clientCaps{
		documentChanges: p.Capabilities.Workspace.WorkspaceEdit.DocumentChanges,
		disabledActions: p.Capabilities.TextDocument.CodeAction.DisabledSupport,
		snippets:        p.Capabilities.TextDocument.Completion.CompletionItem.SnippetSupport,
	}
	if o := p.InitializationOptions; o != nil {
		if o.Lint != "" {
			s.opt.Lint = o.Lint
		}
		if o.Lang != "" {
			s.opt.Lang = o.Lang
		}
		if o.Dialect != "" {
			s.opt.Dialect = o.Dialect
		}
		if len(o.Ignore) > 0 {
			s.opt.Ignore = o.Ignore
		}
		for k, v := range o.PathMap {
			s.opt.PathMap[k] = v
		}
		if o.RefileDepth > 0 {
			s.opt.RefileDepth = o.RefileDepth
		}
		if o.Token != "" {
			s.setToken(o.Token)
		}
	}
	// The keywords before anything is parsed, with a short wait: a server
	// that is not up leaves the default, and the rest still works.
	quick := *s.core
	quick.Rest.Timeout = 2 * time.Second
	if st, err := commands.SendReceiveGetErr[common.TodoStatesResult](&quick, "status", nil); err == nil && len(st.Active)+len(st.Done) > 0 {
		s.todo = strings.Join(st.Active, " ") + " | " + strings.Join(st.Done, " ")
	}
	s.initialized = true
	return map[string]interface{}{
		"capabilities": map[string]interface{}{
			"positionEncoding": string(s.enc),
			"textDocumentSync": map[string]interface{}{"openClose": true, "change": 2, "save": map[string]bool{"includeText": false}},
			"completionProvider": map[string]interface{}{
				"triggerCharacters": []string{":", "[", "*", "#", "+"},
			},
			"definitionProvider":      true,
			"documentSymbolProvider":  true,
			"workspaceSymbolProvider": true,
			"foldingRangeProvider":    true,
			"documentLinkProvider":    map[string]bool{"resolveProvider": true},
			"codeActionProvider": map[string]interface{}{
				"codeActionKinds": []string{"quickfix", "refactor.rewrite", "refactor.move", "source"},
			},
			"codeLensProvider":       map[string]bool{"resolveProvider": false},
			"executeCommandProvider": map[string]interface{}{"commands": commandNames},
		},
		"serverInfo": map[string]string{"name": "orgs", "version": common.Version},
	}
}

// started is the work after the handshake: which checkers to run, and a
// token renewal now and then for as long as the editor is open.
func (s *server) started() {
	if s.opt.Lint != "off" && s.opt.Lint != "none" {
		with := s.opt.Lint
		if with == "auto" {
			with = ""
		}
		if names, notes, err := lint.Checkers(s.core, with); err != nil {
			if s.opt.Lint != "auto" {
				s.log(sevWarning, "orgs lsp: no prose checking: "+err.Error())
			}
		} else {
			for _, n := range notes {
				s.log(sevInfo, n)
			}
			s.dmu.Lock()
			s.checkers = names
			s.dmu.Unlock()
			s.mu.Lock()
			uris := []string{}
			for u := range s.docs {
				uris = append(uris, u)
			}
			s.mu.Unlock()
			for _, u := range uris {
				s.diagnoseSoon(u, 0)
			}
		}
	}
	// An editor that handed over the token renews it itself.
	if s.core.Refresh != nil && !s.clientToken {
		s.api.renew()
		for range time.Tick(20 * time.Minute) {
			s.api.renew()
		}
	}
}

func (s *server) log(sev int, msg string) {
	fmt.Fprintln(os.Stderr, msg)
	s.c.notify("window/logMessage", map[string]interface{}{"type": sev, "message": msg})
}

func (s *server) say(sev int, msg string) {
	s.c.notify("window/showMessage", map[string]interface{}{"type": sev, "message": msg})
}

func init() {
	commands.AddCmd("lsp", "a language server for org files (completion, links, outline, lint, code actions) for any editor",
		func() commands.Cmd { return &Lsp{} })
}
