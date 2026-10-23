package templates

// A template that is not saved yet, rendered as if it were: for the editor's
// preview, and to refuse a save that would not parse. Includes resolve the way
// they will once it is on disk - relative to the folder it is going to be
// saved in - because the draft is served under the path it will have.

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/flosch/pongo2/v5"
)

// DraftError is where a template went wrong, as an editor wants it: a line
// and column (1-based, 0 when pongo2 did not say) and what was wrong.
type DraftError struct {
	Line int
	Col  int
	Msg  string
}

func (e *DraftError) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("line %d: %s", e.Line, e.Msg)
	}
	return e.Msg
}

// draftLoader serves the draft under its future path and every other file
// from disk, relative to the template naming it.
type draftLoader struct {
	path string
	text string
}

func (l *draftLoader) Abs(base, name string) string {
	if filepath.IsAbs(name) {
		return name
	}
	if base == "" {
		return filepath.Join(filepath.Dir(l.path), name)
	}
	return filepath.Join(filepath.Dir(base), name)
}

func (l *draftLoader) Get(path string) (io.Reader, error) {
	if path == l.path {
		return strings.NewReader(l.text), nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(b), nil
}

// Tags that read a file. ssi reads any path at all, and a template can be
// written by anybody logged in to the server, so it is not allowed; the
// others must name a plain relative path inside the template folders.
var fileTag = regexp.MustCompile(`\{%-?\s*(ssi|include|extends|import)\b\s*([^%]*?)\s*-?%\}`)
var plainPath = regexp.MustCompile(`^"([^"]*)"|^'([^']*)'`)

// UnsafeTags is why text may not be saved as a template, or "" when it may.
func UnsafeTags(text string) *DraftError {
	for _, m := range fileTag.FindAllStringSubmatchIndex(text, -1) {
		tag := text[m[2]:m[3]]
		arg := text[m[4]:m[5]]
		line := strings.Count(text[:m[0]], "\n") + 1
		col := m[0] - strings.LastIndex(text[:m[0]], "\n")
		if tag == "ssi" {
			return &DraftError{Line: line, Col: col, Msg: "{% ssi %} reads any file on the server, so a template here may not use it; {% include %} a template beside it instead"}
		}
		pm := plainPath.FindStringSubmatch(arg)
		if pm == nil {
			return &DraftError{Line: line, Col: col, Msg: fmt.Sprintf("{%% %s %%} has to name its file in quotes", tag)}
		}
		p := pm[1] + pm[2]
		if filepath.IsAbs(p) || strings.HasPrefix(p, "~") || strings.Contains(filepath.ToSlash(p), "..") {
			return &DraftError{Line: line, Col: col, Msg: fmt.Sprintf("{%% %s %%} can only name a file inside the template folders, not %s", tag, p)}
		}
	}
	return nil
}

func draftErr(err error) *DraftError {
	if err == nil {
		return nil
	}
	if pe, ok := err.(*pongo2.Error); ok {
		msg := err.Error()
		if pe.OrigError != nil {
			msg = pe.OrigError.Error()
		}
		return &DraftError{Line: pe.Line, Col: pe.Column, Msg: msg}
	}
	return &DraftError{Msg: err.Error()}
}

// RenderDraft is text, a template that will be saved at path, rendered with
// the standard context and context on top. The error says where it broke,
// parsing or running; the text is "" then.
func (self *TemplateManager) RenderDraft(path, text string, context map[string]any) (string, *DraftError) {
	if e := UnsafeTags(text); e != nil {
		return "", e
	}
	set := pongo2.NewSet("draft", &draftLoader{path: path, text: text})
	set.BanTag("ssi")
	tpl, err := set.FromFile(path)
	if err != nil {
		return "", draftErr(err)
	}
	ctx := self.AugmentContext(self.GetStandardContext(), context, false)
	out, err := tpl.Execute(*ctx)
	if err != nil {
		return "", draftErr(err)
	}
	return out, nil
}

// AskField is one ask("Client", "Acme") a template makes: the question, and
// what its box starts with.
type AskField struct {
	Name    string
	Default string `json:",omitempty"`
}

var askRe = regexp.MustCompile(`\bask\(\s*(?:"((?:[^"\\]|\\.)*)"|'((?:[^'\\]|\\.)*)')\s*(?:,\s*(?:"((?:[^"\\]|\\.)*)"|'((?:[^'\\]|\\.)*)'))?\s*\)`)
var includeRe = regexp.MustCompile(`\{%-?\s*include\s+(?:"([^"]+)"|'([^']+)')`)

// AskFields is what a template asks for, in the order it first asks, each
// question once, the included templates' questions in their place. path is
// where the template is (or will be), for the includes; an include that is
// not there is skipped, as rendering will say so anyway.
func AskFields(text, path string) []AskField {
	out := []AskField{}
	seen := map[string]bool{}
	var walk func(text, path string, depth int)
	walk = func(text, path string, depth int) {
		type hit struct {
			at   int
			ask  *AskField
			file string
		}
		hits := []hit{}
		for _, m := range askRe.FindAllStringSubmatchIndex(text, -1) {
			part := func(i int) string {
				if m[2*i] < 0 {
					return ""
				}
				return text[m[2*i]:m[2*i+1]]
			}
			hits = append(hits, hit{at: m[0], ask: &AskField{Name: part(1) + part(2), Default: part(3) + part(4)}})
		}
		if depth < 4 && path != "" {
			for _, m := range includeRe.FindAllStringSubmatchIndex(text, -1) {
				name := ""
				if m[2] >= 0 {
					name = text[m[2]:m[3]]
				} else {
					name = text[m[4]:m[5]]
				}
				if filepath.IsAbs(name) || strings.Contains(name, "..") {
					continue
				}
				hits = append(hits, hit{at: m[0], file: filepath.Join(filepath.Dir(path), name)})
			}
		}
		sort.Slice(hits, func(i, j int) bool { return hits[i].at < hits[j].at })
		for _, h := range hits {
			if h.ask != nil {
				if h.ask.Name != "" && !seen[h.ask.Name] {
					seen[h.ask.Name] = true
					out = append(out, *h.ask)
				}
				continue
			}
			if b, err := os.ReadFile(h.file); err == nil {
				walk(string(b), h.file, depth+1)
			}
		}
	}
	walk(text, path, 0)
	return out
}
