package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

// client drives a server over pipes, as an editor would.
type client struct {
	t   *testing.T
	in  *io.PipeWriter
	out *bufio.Reader
	c   *conn
	id  int
	// Notifications and server requests that arrived while waiting.
	seen []message
	// What to answer the server's own requests with, by method.
	answer map[string]interface{}
}

func start(t *testing.T, url string, opt options) *client {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	core := commands.NewCore(url, nil)
	core.Rest.Timeout = 2 * time.Second
	if opt.PathMap == nil {
		opt.PathMap = map[string]string{}
	}
	if opt.Lint == "" {
		opt.Lint = "off"
	}
	s := newServer(core, newConn(inR, outW), opt)
	go s.run()
	cl := &client{t: t, in: inW, c: newConn(outR, inW), answer: map[string]interface{}{}}
	t.Cleanup(func() { inW.Close() })
	cl.call("initialize", map[string]interface{}{"capabilities": map[string]interface{}{
		"workspace": map[string]interface{}{"workspaceEdit": map[string]bool{"documentChanges": true}}}}, nil)
	cl.notify("initialized", map[string]interface{}{})
	return cl
}

func (cl *client) notify(method string, params interface{}) { cl.c.notify(method, params) }

func (cl *client) call(method string, params interface{}, result interface{}) {
	cl.t.Helper()
	cl.id++
	raw, _ := json.Marshal(params)
	id := json.RawMessage(fmt.Sprint(cl.id))
	cl.c.write(message{Id: &id, Method: method, Params: raw})
	for {
		m, err := cl.c.read()
		if err != nil {
			cl.t.Fatalf("%s: %v", method, err)
		}
		if m.Method != "" {
			if m.Id != nil {
				// The server asking: answer it.
				cl.c.reply(m.Id, cl.answer[m.Method], nil)
			}
			cl.seen = append(cl.seen, m)
			continue
		}
		if string(*m.Id) != fmt.Sprint(cl.id) {
			continue
		}
		if m.Error != nil {
			cl.t.Fatalf("%s: %s", method, m.Error.Message)
		}
		if result != nil {
			if err := json.Unmarshal(m.Result, result); err != nil {
				cl.t.Fatalf("%s: %v in %s", method, err, m.Result)
			}
		}
		return
	}
}

func (cl *client) open(uri, text string) {
	cl.notify("textDocument/didOpen", map[string]interface{}{"textDocument": map[string]interface{}{
		"uri": uri, "languageId": "org", "version": 1, "text": text}})
}

const notes = `#+TODO: TODO NEXT | DONE
* NEXT Write the report :work:
  SCHEDULED: <2026-11-02 Mon>
  :PROPERTIES:
  :CUSTOM_ID: report
  :END:
See [[*Budget]] and [[#report]] and [[*Nowhere]].
#+NAME: square
#+BEGIN_SRC python
return 4
#+END_SRC
** Budget
| a | b |
* DONE Old
  <<there>>
#+BEGIN_QUOTE
never closed
`

func TestSymbolsFoldingAndDefinitions(t *testing.T) {
	cl := start(t, "http://127.0.0.1:1", options{})
	uri := "file:///tmp/orgs-lsp-test/notes.org"
	cl.open(uri, notes)

	var syms []DocumentSymbol
	cl.call("textDocument/documentSymbol", map[string]interface{}{"textDocument": map[string]string{"uri": uri}}, &syms)
	if len(syms) != 2 || syms[0].Name != "Write the report" || syms[1].Name != "Old" {
		t.Fatalf("symbols: %+v", syms)
	}
	if syms[0].Detail != "NEXT :work: SCHEDULED <2026-11-02 Mon>" {
		t.Errorf("detail: %q", syms[0].Detail)
	}
	kids := syms[0].Children
	if len(kids) != 2 || kids[0].Name != "square" || kids[1].Name != "Budget" {
		t.Errorf("children: %+v", kids)
	}
	if syms[0].Range.End.Line != 12 {
		t.Errorf("the first heading ends on line %d, not 12", syms[0].Range.End.Line)
	}

	var folds []FoldingRange
	cl.call("textDocument/foldingRange", map[string]interface{}{"textDocument": map[string]string{"uri": uri}}, &folds)
	want := map[string]bool{"1-12": true, "8-10": true, "3-5": true}
	for _, f := range folds {
		delete(want, fmt.Sprintf("%d-%d", f.StartLine, f.EndLine))
	}
	if len(want) > 0 {
		t.Errorf("folds %+v miss %v", folds, want)
	}

	def := func(line, char int) []Location {
		var locs []Location
		cl.call("textDocument/definition", map[string]interface{}{"textDocument": map[string]string{"uri": uri},
			"position": Position{Line: line, Character: char}}, &locs)
		return locs
	}
	if l := def(6, 8); len(l) != 1 || l[0].Range.Start.Line != 11 {
		t.Errorf("[[*Budget]]: %+v", l)
	}
	if l := def(6, 25); len(l) != 1 || l[0].Range.Start.Line != 1 {
		t.Errorf("[[#report]]: %+v", l)
	}
	if l := def(6, 42); len(l) != 0 {
		t.Errorf("[[*Nowhere]] went somewhere: %+v", l)
	}
}

func TestDiagnostics(t *testing.T) {
	cl := start(t, "http://127.0.0.1:1", options{})
	uri := "file:///tmp/orgs-lsp-test/diag.org"
	cl.open(uri, notes)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		for _, m := range cl.seen {
			if m.Method != "textDocument/publishDiagnostics" {
				continue
			}
			var p struct{ Diagnostics []Diagnostic }
			json.Unmarshal(m.Params, &p)
			codes := []string{}
			for _, d := range p.Diagnostics {
				codes = append(codes, fmt.Sprintf("%s@%d", d.Code, d.Range.Start.Line))
			}
			got := strings.Join(codes, " ")
			if got != "unclosed-block@15 broken-link@6" {
				t.Fatalf("diagnostics: %s", got)
			}
			return
		}
		// Anything, to read what the server has sent meanwhile.
		cl.call("textDocument/foldingRange", map[string]interface{}{"textDocument": map[string]string{"uri": uri}}, nil)
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("no diagnostics were published")
}

func TestCompletion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/alltags":
			json.NewEncoder(w).Encode([]string{"work", "home", "errand"})
		case "/search":
			json.NewEncoder(w).Encode([]common.Todo{{Headline: "Plan the garden", Filename: "/srv/org/home.org", LineNum: 4,
				Props: map[string]string{"ID": "abc-123"}}})
		case "/status":
			json.NewEncoder(w).Encode(common.TodoStatesResult{Active: []string{"TODO"}, Done: []string{"DONE"}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	cl := start(t, srv.URL, options{PathMap: map[string]string{"/srv/org": "/Users/me/org"}})
	uri := "file:///tmp/orgs-lsp-test/c.org"
	text := "* NEXT Write :work:\n** N\n* Plan :ho\nSee [[id:\nand [[*W\n"
	cl.open(uri, text)
	labels := func(line, char int) []CompletionItem {
		var list CompletionList
		cl.call("textDocument/completion", map[string]interface{}{"textDocument": map[string]string{"uri": uri},
			"position": Position{Line: line, Character: char}}, &list)
		return list.Items
	}
	names := func(items []CompletionItem) string {
		out := []string{}
		for _, it := range items {
			out = append(out, it.Label)
		}
		return strings.Join(out, ",")
	}
	if got := names(labels(1, 4)); got != "TODO,DONE" {
		t.Errorf("keywords: %s", got)
	}
	items := labels(2, 10)
	got := map[string]CompletionItem{}
	for _, it := range items {
		got[it.Label] = it
	}
	if _, ok := got["home"]; !ok || got["work"].Detail != "1 in this file" {
		t.Errorf("tags: %+v", items)
	}
	if it := got["home"]; it.TextEdit == nil || it.TextEdit.NewText != "home:" || it.TextEdit.Range.Start.Character != 8 {
		t.Errorf("tag edit: %+v", it.TextEdit)
	}
	ids := labels(3, 9)
	if len(ids) != 1 || ids[0].TextEdit.NewText != "id:abc-123][Plan the garden]]" || ids[0].Detail != "home.org" {
		t.Errorf("ids: %+v", ids)
	}
	heads := labels(4, 8)
	if !strings.Contains(names(heads), "*NEXT Write") {
		t.Errorf("headings: %s", names(heads))
	}
}

// Schedule goes through /lsp/date with the buffer, and comes back as an edit
// the editor is asked to apply.
func TestScheduleIsAnEditToTheBuffer(t *testing.T) {
	var sent common.TextDateChange
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/lsp/date":
			json.NewDecoder(r.Body).Decode(&sent)
			json.NewEncoder(w).Encode(common.TextEditResult{Ok: true, Changed: true,
				Text: "* TODO Call\n  SCHEDULED: <2026-11-02 Mon>\nnotes\n"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	cl := start(t, srv.URL, options{})
	cl.answer["workspace/applyEdit"] = map[string]bool{"applied": true}
	uri := "file:///tmp/orgs-lsp-test/s.org"
	cl.open(uri, "* TODO Call\nnotes\n")

	var actions []CodeAction
	cl.call("textDocument/codeAction", map[string]interface{}{"textDocument": map[string]string{"uri": uri},
		"range": Range{Start: Position{Line: 1}, End: Position{Line: 1}}, "context": map[string]interface{}{"diagnostics": []interface{}{}}}, &actions)
	var today *Command
	titles := []string{}
	for _, a := range actions {
		titles = append(titles, a.Title)
		if a.Title == "Schedule for today" {
			today = a.Command
		}
	}
	if today == nil {
		t.Fatalf("no schedule action in %v", titles)
	}
	cl.call("workspace/executeCommand", map[string]interface{}{"command": today.Command, "arguments": today.Arguments}, nil)
	if sent.Name != "SCHEDULED" || sent.Line != 0 || !strings.HasPrefix(sent.Value, "<") || sent.Text != "* TODO Call\nnotes\n" {
		t.Errorf("sent: %+v", sent)
	}
	var edit *WorkspaceEdit
	for _, m := range cl.seen {
		if m.Method == "workspace/applyEdit" {
			var p struct{ Edit WorkspaceEdit }
			json.Unmarshal(m.Params, &p)
			edit = &p.Edit
		}
	}
	if edit == nil || len(edit.DocumentChanges) != 1 {
		t.Fatalf("no edit was applied: %+v", cl.seen)
	}
	e := edit.DocumentChanges[0].Edits[0]
	if e.NewText != "  SCHEDULED: <2026-11-02 Mon>\n" || e.Range.Start != (Position{Line: 1}) || e.Range.End != (Position{Line: 1}) {
		t.Errorf("edit: %+v", e)
	}
}

func TestPositionsAndDiffs(t *testing.T) {
	line := "é😀x"
	if c := utf16Enc.col(line, len(line)); c != 4 {
		t.Errorf("utf-16 width %d", c)
	}
	if b := utf16Enc.byteAt(line, 3); b != len("é😀") {
		t.Errorf("byteAt %d", b)
	}
	text := "one\ntwo é\nthree\n"
	e, ok := utf16Enc.diffEdit(text, "one\ntwo è\nthree\n")
	if !ok || e.NewText != "è" || e.Range.Start != (Position{1, 4}) || e.Range.End != (Position{1, 5}) {
		t.Errorf("diff: %+v", e)
	}
	if got := utf16Enc.apply(text, &Range{Start: Position{1, 0}, End: Position{1, 3}}, "2"); got != "one\n2 é\nthree\n" {
		t.Errorf("apply: %q", got)
	}
}

// An editor that logs in itself hands the token over, and a renewed one with
// didChangeConfiguration.
func TestTheEditorsTokenIsSent(t *testing.T) {
	seen := make(chan string, 8)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen <- r.URL.Path + " " + r.Header.Get("Authorization")
		json.NewEncoder(w).Encode(common.TodoStatesResult{Active: []string{"TODO"}, Done: []string{"DONE"}})
	}))
	defer srv.Close()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	core := commands.NewCore(srv.URL, nil)
	s := newServer(core, newConn(inR, outW), options{Lint: "off", PathMap: map[string]string{}})
	go s.run()
	cl := &client{t: t, in: inW, c: newConn(outR, inW), answer: map[string]interface{}{}}
	defer inW.Close()
	cl.call("initialize", map[string]interface{}{"capabilities": map[string]interface{}{},
		"initializationOptions": map[string]string{"token": "first"}}, nil)
	if got := <-seen; got != "/status Bearer first" {
		t.Errorf("initialize asked %q", got)
	}
	cl.notify("workspace/didChangeConfiguration", map[string]interface{}{"settings": map[string]string{"token": "second"}})
	var syms []SymbolInformation
	cl.call("workspace/symbol", map[string]string{"query": "x"}, &syms)
	if got := <-seen; got != "/search Bearer second" {
		t.Errorf("after the change %q", got)
	}
}
