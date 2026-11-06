package lsp

// Code actions, the run lens, and the commands they call.
//
//	orgs.date    [uri, line, name, value?]   SCHEDULED/DEADLINE; value as `orgs sched` reads one
//	                                         ("tomorrow", "fri", "+2w", "remove"); asked for when missing
//	orgs.run     [uri, line]                 the block, #+CALL: or inline code at the line
//	orgs.refile  [uri, line, target?]        target {Filename, Olp}; asked for when missing
//	orgs.archive [uri, line]
//
// An editor can bind a key to any of them with workspace/executeCommand, which
// is how a date nobody offered gets typed in.
//
// Date and run go through the buffer (see the header of lsp.go); refile and
// archive through the files, which is why they refuse an unsaved buffer.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

var commandNames = []string{"orgs.date", "orgs.run", "orgs.refile", "orgs.archive"}

type codeActionParams struct {
	TextDocument textDocumentId
	Range        Range
	Context      struct {
		Only []string
	}
}

type refileTarget struct {
	Filename string
	Olp      []string
}

func (t refileTarget) label() string {
	if len(t.Olp) == 0 {
		return filepath.Base(t.Filename)
	}
	return filepath.Base(t.Filename) + " › " + strings.Join(t.Olp, " › ")
}

func (s *server) codeActions(p codeActionParams) []CodeAction {
	out := []CodeAction{}
	d := s.doc(p.TextDocument.URI)
	if d == nil {
		return out
	}
	want := func(kind string) bool {
		if len(p.Context.Only) == 0 {
			return true
		}
		for _, o := range p.Context.Only {
			if kind == o || strings.HasPrefix(kind, o+".") {
				return true
			}
		}
		return false
	}
	cmd := func(title, kind, name string, args ...interface{}) {
		if want(kind) {
			out = append(out, CodeAction{Title: title, Kind: kind, Command: &Command{Title: title, Command: name, Arguments: append([]interface{}{d.uri}, args...)}})
		}
	}

	// The checkers' suggestions.
	if want("quickfix") {
		for _, it := range s.diagnosticsAt(d.uri, d.version, p.Range) {
			for i, fix := range it.fixes {
				if i == 3 {
					break
				}
				title := fmt.Sprintf("Replace with “%s”", fix)
				if fix == "" {
					title = "Remove it"
				}
				out = append(out, CodeAction{Title: title, Kind: "quickfix", Diagnostics: []Diagnostic{it.Diagnostic},
					IsPreferred: i == 0, Edit: s.edit(d, []TextEdit{{Range: it.Range, NewText: fix}})})
			}
		}
	}

	row := p.Range.Start.Line
	if what, lang := runnableAt(d, row); what != "" {
		why := ""
		if lang != "" {
			why = s.whyNot(lang)
		}
		title := "Run " + what
		switch {
		case why == "":
			cmd(title, "", "orgs.run", row)
		case s.caps.disabledActions && len(p.Context.Only) == 0:
			a := CodeAction{Title: title}
			a.Disabled = &struct {
				Reason string `json:"reason"`
			}{firstLine(why)}
			out = append(out, a)
		}
	}

	hs := s.headings(d)
	if i := headingAt(hs, row); i >= 0 {
		h := hs[i]
		cmd("Schedule for today", "refactor.rewrite", "orgs.date", h.line, "SCHEDULED", "today")
		cmd("Schedule…", "refactor.rewrite", "orgs.date", h.line, "SCHEDULED")
		if h.sched != "" {
			cmd("Remove the schedule", "refactor.rewrite", "orgs.date", h.line, "SCHEDULED", "remove")
		}
		cmd("Deadline…", "refactor.rewrite", "orgs.date", h.line, "DEADLINE")
		if h.deadline != "" {
			cmd("Remove the deadline", "refactor.rewrite", "orgs.date", h.line, "DEADLINE", "remove")
		}
		s.rmu.Lock()
		recent := append([]refileTarget{}, s.recent...)
		s.rmu.Unlock()
		for k, t := range recent {
			if k == 3 {
				break
			}
			cmd("Refile to "+t.label(), "refactor.move", "orgs.refile", h.line, t)
		}
		cmd("Refile…", "refactor.move", "orgs.refile", h.line)
		cmd("Archive the subtree", "refactor.move", "orgs.archive", h.line)
	}
	return out
}

// runnableAt is what at row could run - "the python block", "the #+CALL:",
// "the inline code" - and the block's language when it is a block.
func runnableAt(d *document, row int) (string, string) {
	bs := blocks(d.lines)
	if b := inBlock(bs, row); b != nil {
		if b.Kind != "src" || b.End < 0 {
			return "", ""
		}
		what := "the " + b.Lang + " block"
		if b.Name != "" {
			what += " " + b.Name
		}
		return what, b.Lang
	}
	l := d.line(row)
	if callRe.MatchString(l) {
		return "the #+CALL:", ""
	}
	if inlineRe.MatchString(l) {
		return "the inline code", ""
	}
	return "", ""
}

// whyNot is why a block in lang would not run here, "" when it would. A
// server that cannot be asked counts as no.
func (s *server) whyNot(lang string) string {
	res, err := getCached[map[string]string](s.api, "run:"+lang, "lsp/runnable", map[string]string{"lang": lang})
	if err != nil {
		return "the orgs server is not answering: " + err.Error()
	}
	return res[lang]
}

func firstLine(s string) string {
	l, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return l
}

func (s *server) codeLenses(uri string) []CodeLens {
	out := []CodeLens{}
	d := s.doc(uri)
	if d == nil {
		return out
	}
	for _, b := range blocks(d.lines) {
		if b.Kind != "src" || b.End < 0 || b.Lang == "" || s.whyNot(b.Lang) != "" {
			continue
		}
		r := Range{Start: Position{Line: b.Begin}, End: Position{Line: b.Begin}}
		out = append(out, CodeLens{Range: r, Command: &Command{Title: "▶ Run", Command: "orgs.run", Arguments: []interface{}{uri, b.Begin}}})
	}
	return out
}

// edit is a workspace edit to d as the editor has it now: versioned when the
// client can check one, so an edit worked out from text the person has since
// typed over is refused rather than applied in the wrong place.
func (s *server) edit(d *document, edits []TextEdit) *WorkspaceEdit {
	if s.caps.documentChanges {
		v := d.version
		return &WorkspaceEdit{DocumentChanges: []TextDocumentEdit{{TextDocument: VersionedTextDocumentIdentifier{URI: d.uri, Version: &v}, Edits: edits}}}
	}
	return &WorkspaceEdit{Changes: map[string][]TextEdit{d.uri: edits}}
}

// applyText asks the editor to make d read text, as one edit.
func (s *server) applyText(d *document, label, text string) error {
	e, ok := s.enc.diffEdit(d.text, text)
	if !ok {
		return nil
	}
	var res struct {
		Applied       bool
		FailureReason string
	}
	if err := s.c.call("workspace/applyEdit", map[string]interface{}{"label": label, "edit": s.edit(d, []TextEdit{e})}, &res); err != nil {
		return err
	}
	if !res.Applied {
		if res.FailureReason != "" {
			return fmt.Errorf("the editor did not apply it: %s", res.FailureReason)
		}
		return fmt.Errorf("the editor did not apply it (the text changed in the meantime?)")
	}
	return nil
}

// pick asks the person to choose one of options, and says which, or -1.
func (s *server) pick(prompt string, options []string) int {
	items := []MessageActionItem{}
	seen := map[string]int{}
	for _, o := range options {
		t := o
		if n := seen[o]; n > 0 {
			t = fmt.Sprintf("%s (%d)", o, n+1)
		}
		seen[o]++
		items = append(items, MessageActionItem{Title: t})
	}
	var chose *MessageActionItem
	if err := s.c.call("window/showMessageRequest", map[string]interface{}{"type": sevInfo, "message": prompt, "actions": items}, &chose); err != nil || chose == nil {
		return -1
	}
	for i, it := range items {
		if it.Title == chose.Title {
			return i
		}
	}
	return -1
}

func (s *server) execute(name string, raw []json.RawMessage) (interface{}, *rpcError) {
	str := func(i int) string {
		var v string
		if i < len(raw) {
			json.Unmarshal(raw[i], &v)
		}
		return v
	}
	num := func(i int) int {
		var v int
		if i < len(raw) {
			json.Unmarshal(raw[i], &v)
		}
		return v
	}
	d := s.doc(str(0))
	if d == nil {
		return nil, &rpcError{Code: errInvalidParams, Message: "that document is not open"}
	}
	line := num(1)
	var err error
	switch name {
	case "orgs.date":
		err = s.setDate(d, line, strings.ToUpper(str(2)), len(raw) > 3, str(3))
	case "orgs.run":
		err = s.runAt(d, line)
	case "orgs.refile":
		var t *refileTarget
		if len(raw) > 2 {
			t = &refileTarget{}
			if json.Unmarshal(raw[2], t) != nil || t.Filename == "" {
				t = nil
			}
		}
		err = s.move(d, line, "refile", t)
	case "orgs.archive":
		err = s.move(d, line, "archive", nil)
	default:
		return nil, &rpcError{Code: errInvalidParams, Message: "no command " + name}
	}
	if err != nil {
		s.say(sevError, err.Error())
	}
	return nil, nil
}

var datePicks = []struct{ title, value string }{
	{"Today", "today"}, {"Tomorrow", "tomorrow"}, {"In two days", "+2d"}, {"Next Monday", "mon"},
	{"In a week", "+1w"}, {"In two weeks", "+2w"}, {"In a month", "+1m"}, {"Remove", "remove"},
}

func (s *server) setDate(d *document, line int, name string, given bool, value string) error {
	if name != "SCHEDULED" && name != "DEADLINE" && name != "CLOSED" && name != "TIMESTAMP" {
		return fmt.Errorf("%q is not a date a heading has: SCHEDULED, DEADLINE, CLOSED or TIMESTAMP", name)
	}
	if !given {
		opts := []string{}
		for _, p := range datePicks {
			opts = append(opts, p.title)
		}
		i := s.pick(name[:1]+strings.ToLower(name[1:])+" for when?", opts)
		if i < 0 {
			return nil
		}
		value = datePicks[i].value
	}
	stamp, err := commands.ParseDateToOrg(value, time.Now())
	if err != nil {
		return err
	}
	res, err := post[common.TextEditResult](s.api, "lsp/date",
		common.TextDateChange{Filename: s.remote(d.path), Text: d.text, Line: line, Name: name, Value: stamp}, false)
	if err != nil {
		return fmt.Errorf("could not ask the orgs server: %v", err)
	}
	if !res.Ok {
		return fmt.Errorf("%s", res.Msg)
	}
	if !res.Changed {
		return nil
	}
	return s.applyText(d, strings.ToLower(name), res.Text)
}

func (s *server) runAt(d *document, line int) error {
	res, err := post[common.TextEditResult](s.api, "lsp/run",
		common.TextRun{Filename: s.remote(d.path), Text: d.text, Line: line}, true)
	if err != nil {
		return fmt.Errorf("could not ask the orgs server: %v", err)
	}
	if res.Changed {
		// The run may have taken a while; the edit is checked against the
		// version it was worked out from.
		if err := s.applyText(d, "run", res.Text); err != nil {
			return err
		}
	}
	if !res.Ok {
		return fmt.Errorf("%s", firstLine(res.Msg))
	}
	if !res.Changed {
		s.say(sevInfo, "ran: the result is what was there already")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Refile and archive
// ---------------------------------------------------------------------------

// disk is a file's text where the server reads it: here when it is here,
// asked of the server otherwise.
func (s *server) disk(path string) (string, bool) {
	if b, err := os.ReadFile(path); err == nil {
		return string(b), true
	}
	res, err := get[common.ResultMsg](s.api, "orgfile", map[string]string{"filename": s.remote(path)})
	if err == nil && res.Ok {
		return res.Msg, true
	}
	return "", false
}

func same(a, b string) bool { return strings.TrimRight(a, "\n") == strings.TrimRight(b, "\n") }

func (s *server) move(d *document, line int, op string, target *refileTarget) error {
	if on, ok := s.disk(d.path); !ok || !same(on, d.text) {
		return fmt.Errorf("save %s first: %s works on the file, and this one has changes the file does not", filepath.Base(d.path), op)
	}
	// The heading, by the hash the server gives it: the buffer is the file,
	// so parsed under the server's name for it, it hashes as it does there.
	parsed := s.orgConfig().Silent().Parse(strings.NewReader(d.text), s.remote(d.path))
	hs := s.headings(d)
	i := headingAt(hs, line)
	if i < 0 {
		return fmt.Errorf("that line is not under a heading")
	}
	h := hs[i]
	hash := hashAt(parsed, h.line)
	if hash == "" {
		return fmt.Errorf("could not find the heading at line %d", h.line+1)
	}
	if t, err := get[common.Todo](s.api, "hash/"+commands.HashPath(hash), nil); err != nil || t.Filename == "" || t.LineNum != h.line {
		return fmt.Errorf("the orgs server has not read %s since it was saved; try again in a moment", filepath.Base(d.path))
	}

	req := common.MoveRequest{Op: op, From: []common.Target{{Type: "hash", Id: hash}}}
	if op == "refile" {
		t, to, err := s.refileTo(d, hs, i, target)
		if err != nil || t == nil {
			return err
		}
		target = t
		req.To = to
	}

	// Every open file the editor has saved, so that what the server writes
	// can be brought into the buffers it changed.
	clean := s.cleanDocs()
	res, err := write[common.MoveRequest, common.MoveResponse](s.api, "move", &req)
	if err == commands.ErrDryRun {
		s.say(sevInfo, "dry run: nothing was moved")
		return nil
	}
	if err != nil {
		return fmt.Errorf("could not ask the orgs server: %v", err)
	}
	s.api.forget()
	if !res.Ok {
		msg := res.Msg
		for _, r := range res.Results {
			if r.Msg != "" {
				msg = r.Msg
			}
		}
		return fmt.Errorf("%s: %s", op, msg)
	}
	if target != nil {
		s.rmu.Lock()
		keep := []refileTarget{*target}
		for _, t := range s.recent {
			if t.label() != target.label() && len(keep) < 5 {
				keep = append(keep, t)
			}
		}
		s.recent = keep
		s.rmu.Unlock()
	}
	s.catchUp(clean, op)
	return nil
}

// hashAt is the hash of the heading on row.
func hashAt(doc *org.Document, row int) string {
	var find func(secs []*org.Section) string
	find = func(secs []*org.Section) string {
		for _, sec := range secs {
			if sec.Headline != nil && sec.Headline.Pos.Row == row {
				return sec.Hash
			}
			if h := find(sec.Children); h != "" {
				return h
			}
		}
		return ""
	}
	if doc.Outline.Section == nil {
		return ""
	}
	return find(doc.Outline.Children)
}

// refileTo settles where the heading goes: the target named (looked up again,
// since hashes move), or one picked from the server's list.
func (s *server) refileTo(d *document, hs []heading, i int, named *refileTarget) (*refileTarget, common.Target, error) {
	depth := s.opt.RefileDepth
	if depth <= 0 {
		depth = 3
	}
	ps := map[string]string{"depth": fmt.Sprint(depth)}
	if named != nil {
		ps["file"] = s.remote(named.Filename)
	}
	list, err := get[common.RefileTargetList](s.api, "refile/targets", ps)
	if err != nil {
		return nil, common.Target{}, fmt.Errorf("could not ask the orgs server where things go: %v", err)
	}
	// Not into itself or under itself.
	self := []string{}
	for k := 0; k <= i; k++ {
		if hs[k].end >= hs[i].line {
			self = append(self, hs[k].title)
		}
	}
	inside := func(t common.RefileTarget) bool {
		if s.local(t.Filename) != d.path || len(t.Olp) < len(self) {
			return false
		}
		for k := range self {
			if !sameTitle(t.Olp[k], self[k]) {
				return false
			}
		}
		return true
	}
	cands := []common.RefileTarget{}
	for _, t := range list.Targets {
		if !inside(t) {
			cands = append(cands, t)
		}
	}
	as := func(t common.RefileTarget) refileTarget { return refileTarget{Filename: s.local(t.Filename), Olp: t.Olp} }
	to := func(t common.RefileTarget) common.Target {
		if t.Hash == "" {
			return common.Target{Type: "file", Filename: t.Filename}
		}
		return common.Target{Type: "hash", Id: t.Hash}
	}
	if named != nil {
		for _, t := range cands {
			if as(t).label() == named.label() && s.local(t.Filename) == named.Filename {
				r := as(t)
				return &r, to(t), nil
			}
		}
		return nil, common.Target{}, fmt.Errorf("%s is not a place to refile to any more", named.label())
	}
	if len(cands) == 0 {
		return nil, common.Target{}, fmt.Errorf("the orgs server knows nowhere to refile to")
	}
	// Recent first, then the server's order.
	s.rmu.Lock()
	recent := map[string]int{}
	for k, t := range s.recent {
		recent[t.label()] = k + 1
	}
	s.rmu.Unlock()
	ordered := []common.RefileTarget{}
	for k := 1; k <= len(recent); k++ {
		for _, t := range cands {
			if recent[as(t).label()] == k {
				ordered = append(ordered, t)
			}
		}
	}
	for _, t := range cands {
		if recent[as(t).label()] == 0 {
			ordered = append(ordered, t)
		}
	}
	labels := []string{}
	for _, t := range ordered {
		labels = append(labels, as(t).label())
	}
	k := s.pick("Refile “"+hs[i].title+"” to:", labels)
	if k < 0 {
		return nil, common.Target{}, nil
	}
	r := as(ordered[k])
	return &r, to(ordered[k]), nil
}

// cleanDocs is every open document whose buffer is its file.
func (s *server) cleanDocs() map[string]string {
	s.mu.Lock()
	docs := []*document{}
	for _, d := range s.docs {
		docs = append(docs, d)
	}
	s.mu.Unlock()
	out := map[string]string{}
	for _, d := range docs {
		if on, ok := s.disk(d.path); ok && same(on, d.text) {
			out[d.uri] = on
		}
	}
	return out
}

// catchUp brings the editor's buffers up to files the server just wrote.
// Most editors reload a file that changed under a saved buffer by
// themselves, and a moment is given for that; a buffer still behind after it
// is edited to match, so an editor that does not (Helix, Vim without
// autoread) does not save the old text back over the move.
func (s *server) catchUp(clean map[string]string, op string) {
	behind := func() map[string]string {
		out := map[string]string{}
		for uri, was := range clean {
			d := s.doc(uri)
			if d == nil {
				continue
			}
			now, ok := s.disk(d.path)
			if ok && !same(now, was) && !same(now, d.text) {
				out[uri] = now
			}
		}
		return out
	}
	deadline := time.Now().Add(800 * time.Millisecond)
	left := behind()
	for len(left) > 0 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
		left = behind()
	}
	for uri, text := range left {
		if d := s.doc(uri); d != nil {
			if err := s.applyText(d, op, text); err != nil {
				s.say(sevWarning, fmt.Sprintf("%s changed on disk and the editor still shows the old text: reload it before saving (%v)", filepath.Base(d.path), err))
			}
		}
	}
}
