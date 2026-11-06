package lsp

// Diagnostics: what is wrong with the file, published after the editor has
// been quiet for a moment.
//
//   - a block with no #+END_ (an error: the rest of the file is in it) and a
//     drawer with no :END:
//   - a link to nothing: no heading, custom id, target or name in the file,
//     no such file, no heading of that name in it, an id no file has
//   - Harper's or LanguageTool's prose lints (`orgs lint`), with their
//     suggestions kept for the quick fixes
//
// A run that finishes after the buffer has changed again is thrown away: its
// lines are the old ones, and a lint drawn on the wrong word is worse than
// none for a moment.

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ihdavids/orgs/cmd/oc/commands/lint"
	"github.com/ihdavids/orgs/internal/common/orgcheck"
)

type diagItem struct {
	Diagnostic
	fixes []string
}

type diagSet struct {
	version int
	items   []diagItem
}

func (s *server) diagnoseSoon(uri string, after time.Duration) {
	s.dmu.Lock()
	defer s.dmu.Unlock()
	if t := s.timers[uri]; t != nil {
		t.Stop()
	}
	s.timers[uri] = time.AfterFunc(after, func() { s.diagnose(uri) })
}

func (s *server) diagnose(uri string) {
	d := s.doc(uri)
	if d == nil {
		return
	}
	items := s.structural(d)
	s.dmu.Lock()
	checkers := s.checkers
	s.dmu.Unlock()
	if len(checkers) > 0 {
		lines := make([]string, len(d.lines))
		for i := range d.lines {
			lines[i] = d.line(i)
		}
		s.api.mu.RLock()
		probs, err := lint.Check(s.core, lines, checkers, lint.Options{Lang: s.opt.Lang, Dialect: s.opt.Dialect, Ignore: s.opt.Ignore})
		s.api.mu.RUnlock()
		if err != nil {
			s.log(sevWarning, "orgs lsp: prose check: "+err.Error())
		}
		for _, p := range probs {
			sev := sevWarning
			if p.Severity == "info" {
				sev = sevInfo
			}
			items = append(items, diagItem{Diagnostic: Diagnostic{
				Range:    Range{Start: s.enc.pos(d.lines, p.Line, p.Col), End: s.enc.pos(d.lines, p.EndLine, p.EndCol)},
				Severity: sev, Code: p.Rule, Source: p.Checker, Message: p.Msg}, fixes: p.Fix})
		}
	}
	if now := s.doc(uri); now == nil || now.version != d.version {
		return
	}
	s.dmu.Lock()
	s.diagFor[uri] = diagSet{version: d.version, items: items}
	s.dmu.Unlock()
	out := make([]Diagnostic, len(items))
	for i := range items {
		out[i] = items[i].Diagnostic
	}
	s.c.notify("textDocument/publishDiagnostics", map[string]interface{}{"uri": uri, "version": d.version, "diagnostics": out})
}

// structural is what is wrong with the file as org, with no checker: the
// rules are orgcheck's (the server's /lsp/diagnose runs the same ones), the
// questions the text cannot answer are put to lspEnv.
func (s *server) structural(d *document) []diagItem {
	out := []diagItem{}
	sevs := map[string]int{"error": sevError, "warning": sevWarning, "info": sevInfo}
	for _, p := range orgcheck.Structural(d.path, d.lines, checkHeadings(s.headings(d)), &lspEnv{s: s}) {
		out = append(out, diagItem{Diagnostic: Diagnostic{
			Range:    Range{Start: s.enc.pos(d.lines, p.Line, p.From), End: s.enc.pos(d.lines, p.Line, p.To)},
			Severity: sevs[p.Severity], Code: p.Code, Source: "orgs", Message: p.Message}})
	}
	return out
}

// lspEnv answers orgcheck from here: the disk and open buffers for files,
// the server for ids and features.
type lspEnv struct {
	s     *server
	known map[string]bool
}

func (e *lspEnv) FeatureOn(name string) bool { return e.s.featureOn(name) }

func (e *lspEnv) File(path string) (bool, bool) {
	if _, err := os.Stat(path); err == nil {
		return true, false
	}
	// Only said when the folder is here: otherwise this is not the machine
	// the file is on, and its absence means nothing.
	_, derr := os.Stat(filepath.Dir(path))
	return false, derr == nil && e.s.docByPath(path) == nil
}

func (e *lspEnv) LinesOf(path string) []string { return e.s.linesOf(path) }

func (e *lspEnv) Headings(path string, lines []string) []orgcheck.Heading {
	return checkHeadings(e.s.headings(newDocument(pathToURI(path), 0, strings.Join(lines, "\n"))))
}

func (e *lspEnv) Id(id string) (bool, bool) {
	if e.known == nil {
		e.known = map[string]bool{}
		ids := e.s.ids()
		if ids == nil {
			// No server to ask: say nothing rather than call every id
			// broken.
			e.known["\x00offline"] = true
		}
		for _, h := range ids {
			e.known[h.Id] = true
		}
	}
	if e.known["\x00offline"] {
		return false, false
	}
	if e.known[id] {
		return true, true
	}
	_, ok := e.s.followId(id)
	return ok, true
}

// featureOn is whether the server has an optional feature on; false with
// no server to ask.
func (s *server) featureOn(name string) bool {
	res, err := getCached[struct {
		Features []struct {
			Name   string
			Enable bool
		}
	}](s.api, "features", "features", nil)
	if err != nil {
		return false
	}
	for _, f := range res.Features {
		if f.Name == name {
			return f.Enable
		}
	}
	return false
}

// diagnosticsAt are the published diagnostics touching row, as of the
// document's current version.
func (s *server) diagnosticsAt(uri string, version int, r Range) []diagItem {
	s.dmu.Lock()
	defer s.dmu.Unlock()
	set, ok := s.diagFor[uri]
	if !ok || set.version != version {
		return nil
	}
	out := []diagItem{}
	for _, it := range set.items {
		if it.Range.End.Line < r.Start.Line || it.Range.Start.Line > r.End.Line {
			continue
		}
		if it.Range.Start.Line == r.End.Line && it.Range.Start.Character > r.End.Character {
			continue
		}
		if it.Range.End.Line == r.Start.Line && it.Range.End.Character < r.Start.Character {
			continue
		}
		out = append(out, it)
	}
	return out
}
