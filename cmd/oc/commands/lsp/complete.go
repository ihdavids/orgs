package lsp

// Completion: what the line up to the cursor says is being typed.
//
//	**  TO|                 a keyword: the file's #+TODO, else the server's
//	** Title :wo|           a tag: this file's first (by use), then the server's
//	[[|  [[*|  [[#|         a link: protocols, this file's headings, custom ids, targets
//	[[id:|                  every heading with an ID, by title (the server's)
//	[[file:|                every org file the server has, relative to this one
//	[[file:x.org::*|        that file's headings
//	#+|                     keywords and blocks
//	#+begin_src |           languages
//	  :|  in a drawer       property names
//	:TRIGGER: next-s|       org-edna's finders, actions and conditions (edna on)

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/ihdavids/orgs/internal/common"
	"github.com/ihdavids/orgs/internal/common/biblio"
)

var (
	keywordCtxRe  = regexp.MustCompile(`^(\*+\s+)(\S*)$`)
	tagCtxRe      = regexp.MustCompile(`\s(:(?:[\p{L}\p{N}_@#%]+:)*)([\p{L}\p{N}_@#%]*)$`)
	linkCtxRe     = regexp.MustCompile(`\[\[([^\[\]]*)$`)
	hashPlusRe    = regexp.MustCompile(`^(\s*)#\+(\w*)$`)
	srcLangCtxRe  = regexp.MustCompile(`(?i)^\s*#\+begin_src\s+(\S*)$`)
	propNameCtxRe = regexp.MustCompile(`^(\s*):([\w-]*)$`)
	ednaCtxRe     = regexp.MustCompile(`(?i)^\s*:(?:TRIGGER|BLOCKER):[ \t]+(?:.*[ \t])?!?([\w-]*[!?]?)$`)
)

func (s *server) complete(p positionParams) CompletionList {
	d := s.doc(p.TextDocument.URI)
	list := CompletionList{Items: []CompletionItem{}}
	if d == nil {
		return list
	}
	line := d.line(p.Position.Line)
	cur := s.enc.byteAt(line, p.Position.Character)
	before := line[:cur]
	row := p.Position.Line
	// replace is the edit that puts text in place of the last n bytes typed.
	replace := func(n int, text string) *TextEdit {
		return &TextEdit{Range: Range{Start: s.enc.pos(d.lines, row, cur-n), End: p.Position}, NewText: text}
	}
	bs := blocks(d.lines)
	if b := inBlock(bs, row); b != nil && row != b.begin {
		return list
	}

	if m := linkCtxRe.FindStringSubmatch(before); m != nil {
		list.Items = s.linkItems(d, m[1], line[cur:], replace)
		return list
	}
	if m := citeCtxRe.FindStringSubmatch(before); m != nil {
		list.Items = s.citeItems(d, m[1], replace)
		return list
	}
	if starsRe.MatchString(line) {
		if m := keywordCtxRe.FindStringSubmatch(before); m != nil {
			todo, done := d.parsed(s.orgConfig).TodoKeywords()
			for i, k := range append(append([]string{}, todo...), done...) {
				list.Items = append(list.Items, CompletionItem{Label: k, Kind: compKeyword, SortText: pad(i),
					TextEdit: replace(len(m[2]), k+" ")})
			}
			return list
		}
		if m := tagCtxRe.FindStringSubmatch(before); m != nil {
			list.Items = s.tagItems(d, m[1], m[2], replace)
		}
		return list
	}
	if m := srcLangCtxRe.FindStringSubmatch(before); m != nil {
		list.Items = s.langItems(d, m[1], replace)
		return list
	}
	if m := hashPlusRe.FindStringSubmatch(before); m != nil {
		list.Items = s.keywordItems(m[2], m[1], replace)
		return list
	}
	if m := ednaCtxRe.FindStringSubmatch(before); m != nil && s.featureOn("edna") {
		for _, k := range []map[string]string{common.EdnaFinders, common.EdnaActions, common.EdnaConditions} {
			for name, about := range k {
				if strings.HasPrefix(name, strings.ToLower(m[1])) {
					list.Items = append(list.Items, CompletionItem{Label: name, Kind: compKeyword, Detail: about,
						TextEdit: replace(len(m[1]), name)})
				}
			}
		}
		return list
	}
	if m := propNameCtxRe.FindStringSubmatch(before); m != nil {
		for _, dr := range drawers(d.lines, bs) {
			if dr.begin < row && (dr.end < 0 || row < dr.end) && strings.EqualFold(dr.name, "properties") {
				list.Items = s.propertyItems(m[2], replace)
				return list
			}
		}
	}
	return list
}

func pad(i int) string {
	s := strconv.Itoa(i)
	if len(s) >= 5 {
		return s
	}
	return strings.Repeat("0", 5-len(s)) + s
}

func (s *server) tagItems(d *document, have, typed string, replace func(int, string) *TextEdit) []CompletionItem {
	on := map[string]bool{}
	for _, t := range strings.Split(have, ":") {
		on[t] = true
	}
	counts := map[string]int{}
	for _, h := range s.headings(d) {
		for _, t := range h.tags {
			counts[t]++
		}
	}
	if ft := d.parsed(s.orgConfig).Get("FILETAGS"); ft != "" {
		for _, t := range strings.FieldsFunc(ft, func(r rune) bool { return r == ':' || r == ' ' }) {
			counts[t]++
		}
	}
	all := map[string]bool{}
	for t := range counts {
		all[t] = true
	}
	server, _ := getCached[[]string](s.api, "tags", "alltags", nil)
	for _, t := range server {
		all[t] = true
	}
	out := []CompletionItem{}
	for t := range all {
		if on[t] {
			continue
		}
		it := CompletionItem{Label: t, Kind: compEnum, TextEdit: replace(len(typed), t+":")}
		if n := counts[t]; n > 0 {
			it.Detail = strconv.Itoa(n) + " in this file"
			it.SortText = "0" + pad(99999-n) + t
		} else {
			it.Detail = "used elsewhere"
			it.SortText = "1" + t
		}
		out = append(out, it)
	}
	return out
}

func (s *server) keywordItems(typed, indent string, replace func(int, string) *TextEdit) []CompletionItem {
	words := []string{"TITLE:", "AUTHOR:", "DATE:", "FILETAGS:", "TODO:", "STARTUP:", "OPTIONS:", "PROPERTY:",
		"SETUPFILE:", "INCLUDE:", "NAME:", "CAPTION:", "RESULTS:", "CALL:", "TBLFM:", "COLUMNS:", "ARCHIVE:",
		"CATEGORY:", "DESCRIPTION:", "LANGUAGE:", "PRIORITIES:", "TAGS:", "SEQ_TODO:", "ATTR_HTML:", "ATTR_LATEX:"}
	out := []CompletionItem{}
	for i, w := range words {
		out = append(out, CompletionItem{Label: "#+" + w, Kind: compKeyword, FilterText: w, SortText: "1" + pad(i),
			TextEdit: replace(len(typed), w+" ")})
	}
	for i, b := range []string{"SRC", "EXAMPLE", "QUOTE", "VERSE", "CENTER", "EXPORT", "COMMENT"} {
		it := CompletionItem{Label: "#+BEGIN_" + b, Kind: compSnippet, FilterText: "BEGIN_" + b, SortText: "0" + pad(i)}
		if s.caps.snippets {
			arg := ""
			if b == "SRC" || b == "EXPORT" {
				arg = " ${1}"
			}
			it.InsertTextFormat = 2
			it.TextEdit = replace(len(typed), "BEGIN_"+b+arg+"\n"+indent+"$0\n"+indent+"#+END_"+b)
		} else {
			it.TextEdit = replace(len(typed), "BEGIN_"+b)
		}
		out = append(out, it)
	}
	// Callout types, while the server draws them.
	if s.featureOn("callouts") {
		for i, t := range common.CalloutTypes {
			if t.Name == "example" || t.Name == "quote" {
				continue // org's own blocks; #+begin_callout :type example
			}
			b := strings.ToUpper(t.Name)
			it := CompletionItem{Label: "#+BEGIN_" + b, Kind: compSnippet, FilterText: "BEGIN_" + b, SortText: "2" + pad(i),
				Detail: t.Glyph + " callout"}
			if s.caps.snippets {
				it.InsertTextFormat = 2
				it.TextEdit = replace(len(typed), "BEGIN_"+b+" ${1:"+t.Title+"}\n"+indent+"$0\n"+indent+"#+END_"+b)
			} else {
				it.TextEdit = replace(len(typed), "BEGIN_"+b)
			}
			out = append(out, it)
		}
	}
	return out
}

func (s *server) langItems(d *document, typed string, replace func(int, string) *TextEdit) []CompletionItem {
	used := map[string]int{}
	for _, b := range blocks(d.lines) {
		if b.lang != "" {
			used[b.lang]++
		}
	}
	for _, l := range []string{"sh", "bash", "python", "js", "go", "emacs-lisp", "sql", "sqlite", "ruby",
		"lua", "R", "C", "dot", "plantuml", "mermaid", "org", "json", "yaml", "elixir", "rust", "typescript"} {
		if _, ok := used[l]; !ok {
			used[l] = 0
		}
	}
	out := []CompletionItem{}
	for l, n := range used {
		it := CompletionItem{Label: l, Kind: compConstant, TextEdit: replace(len(typed), l), SortText: "1" + l}
		if n > 0 {
			it.Detail = strconv.Itoa(n) + " in this file"
			it.SortText = "0" + pad(99999-n) + l
		}
		out = append(out, it)
	}
	return out
}

func (s *server) propertyItems(typed string, replace func(int, string) *TextEdit) []CompletionItem {
	names := map[string]string{}
	for _, n := range []string{"ID", "CUSTOM_ID", "CATEGORY", "EFFORT", "ARCHIVE", "ORDERED", "COOKIE_DATA",
		"LOGGING", "STYLE", "COLUMNS", "NOBLOCKING", "VISIBILITY", "EXPORT_FILE_NAME", "HEADER-ARGS"} {
		names[n] = "org"
	}
	type propUse struct {
		Name  string
		Count int
	}
	if used, err := getCached[[]propUse](s.api, "props", "properties", nil); err == nil {
		for _, u := range used {
			names[strings.ToUpper(u.Name)] = strconv.Itoa(u.Count) + " in use"
		}
	}
	out := []CompletionItem{}
	for n, detail := range names {
		out = append(out, CompletionItem{Label: n, Kind: compProperty, Detail: detail, TextEdit: replace(len(typed), n+": ")})
	}
	return out
}

// ---------------------------------------------------------------------------
// Link targets
// ---------------------------------------------------------------------------

type idHeading struct {
	Id, Title, File string
	Line            int
}

// ids is every heading with an ID or CUSTOM_ID, from the server.
func (s *server) ids() []idHeading {
	todos, err := getCached[[]common.Todo](s.api, "ids", "search",
		map[string]string{"query": `HasProperty("ID") || HasProperty("CUSTOM_ID")`})
	if err != nil {
		return nil
	}
	out := []idHeading{}
	for _, t := range todos {
		for pk, v := range t.Props {
			if (strings.EqualFold(pk, "ID") || strings.EqualFold(pk, "CUSTOM_ID")) && v != "" {
				out = append(out, idHeading{Id: v, Title: t.Headline, File: s.local(t.Filename), Line: t.LineNum})
			}
		}
	}
	return out
}

// files is every org file the server has, as paths here.
func (s *server) files() []string {
	fs, _ := getCached[common.FileList](s.api, "files", "files", nil)
	out := []string{}
	for _, f := range fs {
		out = append(out, s.local(f))
	}
	return out
}

func (s *server) local(p string) string  { return pathMap(s.opt.PathMap).toLocal(p) }
func (s *server) remote(p string) string { return pathMap(s.opt.PathMap).toServer(p) }

func (s *server) linkItems(d *document, typed, after string, replace func(int, string) *TextEdit) []CompletionItem {
	out := []CompletionItem{}
	closeWith := "]]"
	if strings.HasPrefix(after, "]") {
		closeWith = ""
	}
	n := len(typed)
	add := func(label, detail, insert, filter string, kind int, sort string) {
		out = append(out, CompletionItem{Label: label, Detail: detail, Kind: kind, FilterText: filter, SortText: sort,
			TextEdit: replace(n, insert)})
	}
	proto, rest := splitLink(typed)
	switch {
	case proto == "id":
		for _, h := range s.ids() {
			add(h.Title, filepath.Base(h.File), "id:"+h.Id+"]["+h.Title+closeWith, "id:"+h.Title+" "+h.Id, compRefer, "")
		}
		return out
	case proto == "file":
		file, _, has := strings.Cut(rest, "::")
		if has {
			path := s.resolveFile(d, file)
			lines := s.linesOf(path)
			if lines == nil {
				return out
			}
			for i, h := range s.headings(newDocument(pathToURI(path), 0, strings.Join(lines, "\n"))) {
				ins := "file:" + file + "::*" + h.title + "][" + h.title + closeWith
				add(h.title, filepath.Base(path), ins, "file:"+file+"::*"+h.title, compRefer, pad(i))
			}
			return out
		}
		dir := filepath.Dir(d.path)
		for _, f := range s.files() {
			if f == d.path {
				continue
			}
			rel, err := filepath.Rel(dir, f)
			if err != nil {
				rel = f
			}
			add(rel, "", "file:"+rel, "file:"+rel, compFile, pad(strings.Count(rel, string(filepath.Separator)))+rel)
		}
		return out
	case proto != "":
		return out
	}
	hs := s.headings(d)
	if strings.HasPrefix(typed, "#") {
		for _, h := range hs {
			if h.customId != "" {
				add("#"+h.customId, h.title, "#"+h.customId+"]["+h.title+closeWith, "#"+h.customId+" "+h.title, compRefer, "")
			}
		}
		return out
	}
	for i, h := range hs {
		if h.title != "" {
			add("*"+h.title, "heading", "*"+h.title+closeWith, "*"+h.title, compRefer, "1"+pad(i))
		}
	}
	for i, l := range d.lines {
		for _, m := range targetDefRe.FindAllStringSubmatch(l, -1) {
			add(m[1], "target", m[1]+closeWith, m[1], compRefer, "2"+pad(i))
		}
	}
	if !strings.HasPrefix(typed, "*") {
		for i, p := range []string{"file:", "id:", "https://", "http://", "mailto:", "attachment:", "doi:"} {
			add(p, "link", p, p, compKeyword, "0"+pad(i))
		}
	}
	return out
}

// resolveFile is a file: link's path, as a path here.
func (s *server) resolveFile(d *document, p string) string {
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, p[2:])
		}
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(filepath.Dir(d.path), p)
	}
	return filepath.Clean(p)
}

// linesOf is a file's lines: the editor's copy when it is open, the disk's
// when it is here, the server's otherwise. nil when none has it.
func (s *server) linesOf(path string) []string {
	if d := s.docByPath(path); d != nil {
		return d.lines
	}
	if b, err := os.ReadFile(path); err == nil {
		return strings.Split(string(b), "\n")
	}
	res, err := get[common.ResultMsg](s.api, "orgfile", map[string]string{"filename": s.remote(path)})
	if err == nil && res.Ok {
		return strings.Split(res.Msg, "\n")
	}
	return nil
}

// Inside [cite:...] after an @: the works the file's bibliographies have.
var citeCtxRe = regexp.MustCompile(`\[cite(?:/[^:\]\s]*)?:[^\]]*@([^\s;\]@,]*)$`)

func (s *server) citeItems(d *document, typed string, replace func(int, string) *TextEdit) []CompletionItem {
	type answer struct {
		Ok   bool
		Keys []biblio.Key
	}
	file := s.remote(d.path)
	res, err := getCached[answer](s.api, "cite:"+file, "cite/keys", map[string]string{"file": file})
	if err != nil {
		return nil
	}
	out := []CompletionItem{}
	for i, k := range res.Keys {
		if typed != "" && !strings.Contains(strings.ToLower(k.Key+" "+k.Author+" "+k.Title), strings.ToLower(typed)) {
			continue
		}
		out = append(out, CompletionItem{Label: k.Key, Kind: compRefer, SortText: pad(i),
			Detail:     strings.TrimSpace(k.Author + " " + k.Year + " · " + k.Title),
			FilterText: k.Key + " " + k.Author + " " + k.Title, TextEdit: replace(len(typed), k.Key)})
	}
	return out
}
