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
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ihdavids/orgs/cmd/oc/commands/lint"
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

// structural is what is wrong with the file as org, with no checker.
func (s *server) structural(d *document) []diagItem {
	out := []diagItem{}
	add := func(row, from, to, sev int, code, msg string) {
		out = append(out, diagItem{Diagnostic: Diagnostic{
			Range:    Range{Start: s.enc.pos(d.lines, row, from), End: s.enc.pos(d.lines, row, to)},
			Severity: sev, Code: code, Source: "orgs", Message: msg}})
	}
	bs := blocks(d.lines)
	for _, b := range bs {
		if b.end < 0 {
			l := d.line(b.begin)
			add(b.begin, len(l)-len(strings.TrimLeft(l, " \t")), len(l), sevError, "unclosed-block",
				fmt.Sprintf("#+BEGIN_%s with no #+END_%s", strings.ToUpper(b.kind), strings.ToUpper(b.kind)))
		}
	}
	for _, dr := range drawers(d.lines, bs) {
		if dr.end < 0 {
			l := d.line(dr.begin)
			add(dr.begin, len(l)-len(strings.TrimLeft(l, " \t")), len(l), sevWarning, "unclosed-drawer",
				fmt.Sprintf(":%s: drawer with no :END:", dr.name))
		}
	}
	var known map[string]bool
	for _, l := range linksIn(d.lines, bs) {
		proto, rest := splitLink(l.target)
		switch proto {
		case "":
			if _, _, ok := s.search(d, rest); !ok {
				what := "no heading, target or #+NAME: called %q in this file"
				name := rest
				if strings.HasPrefix(rest, "*") {
					what, name = "no heading called %q in this file", strings.TrimSpace(rest[1:])
				} else if strings.HasPrefix(rest, "#") {
					what, name = "no heading with CUSTOM_ID %q in this file", rest[1:]
				}
				add(l.line, l.start, l.end, sevWarning, "broken-link", fmt.Sprintf(what, name))
			}
		case "file":
			file, search, _ := strings.Cut(rest, "::")
			path := s.resolveFile(d, file)
			if _, err := os.Stat(path); err != nil {
				// Only said when the folder is here: otherwise this is not the
				// machine the file is on, and its absence means nothing.
				if _, derr := os.Stat(filepath.Dir(path)); derr == nil && s.docByPath(path) == nil {
					add(l.line, l.start, l.end, sevWarning, "broken-link", "no file "+file)
				}
				continue
			}
			if search != "" && strings.EqualFold(filepath.Ext(path), ".org") {
				if lines := s.linesOf(path); lines != nil {
					other := newDocument(pathToURI(path), 0, strings.Join(lines, "\n"))
					if _, _, ok := s.search(other, search); !ok {
						add(l.line, l.start, l.end, sevWarning, "broken-link", fmt.Sprintf("%s has nothing called %q", filepath.Base(path), search))
					}
				}
			}
		case "id":
			if known == nil {
				known = map[string]bool{}
				ids := s.ids()
				if ids == nil {
					// No server to ask: say nothing rather than call every
					// id broken.
					known["\x00offline"] = true
				}
				for _, h := range ids {
					known[h.Id] = true
				}
			}
			if known["\x00offline"] || known[rest] {
				continue
			}
			if _, ok := s.followId(rest); !ok {
				add(l.line, l.start, l.end, sevWarning, "broken-link", "no heading has the id "+rest)
			}
		}
	}
	return out
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
