// Package biblio reads bibliographies - BibTeX/BibLaTeX (.bib) and CSL-JSON
// (.json) - into entries, and formats them the way org-cite's basic
// processor does (oc-basic.el): author-year or numeric citations, and an
// author-year bibliography. Pure: text in, entries and strings out, so the
// server's export pass and the language server's completion read a .bib the
// same way.
package biblio

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// Name is one author or editor.
type Name struct {
	Given  string
	Family string
	// Literal is a name that is not split (an organisation: {World Health
	// Organization}).
	Literal string
}

func (n Name) family() string {
	if n.Literal != "" {
		return n.Literal
	}
	return n.Family
}

// Full is "Given Family", or the literal name.
func (n Name) Full() string {
	if n.Literal != "" {
		return n.Literal
	}
	return strings.TrimSpace(n.Given + " " + n.Family)
}

// Entry is one work.
type Entry struct {
	Key     string
	Type    string
	Fields  map[string]string
	Authors []Name
	Editors []Name
}

func (e *Entry) Field(name string) string { return e.Fields[strings.ToLower(name)] }

// Year is the year of the work: year, else the start of date, else "n.d.".
func (e *Entry) Year() string {
	if y := e.Field("year"); y != "" {
		return y
	}
	if d := e.Field("date"); len(d) >= 4 {
		return d[:4]
	}
	return "n.d."
}

// Title is the work's title.
func (e *Entry) Title() string { return e.Field("title") }

// names are who a citation names: the authors, else the editors.
func (e *Entry) names() []Name {
	if len(e.Authors) > 0 {
		return e.Authors
	}
	return e.Editors
}

// AuthorShort is how a citation names the authors: "Doe", "Doe and Roe",
// "Doe et al."; full names every family name.
func (e *Entry) AuthorShort(full bool) string {
	ns := e.names()
	fams := []string{}
	for _, n := range ns {
		fams = append(fams, n.family())
	}
	switch {
	case len(fams) == 0:
		if t := e.Title(); t != "" {
			return t
		}
		return e.Key
	case len(fams) == 1:
		return fams[0]
	case len(fams) == 2:
		return fams[0] + " and " + fams[1]
	case full:
		return strings.Join(fams[:len(fams)-1], ", ") + " and " + fams[len(fams)-1]
	}
	return fams[0] + " et al."
}

// ---------------------------------------------------------------------------
// Reading
// ---------------------------------------------------------------------------

// Parse reads a bibliography by its file name's extension: .json is
// CSL-JSON, anything else BibTeX.
func Parse(name string, data []byte) ([]*Entry, error) {
	if strings.HasSuffix(strings.ToLower(name), ".json") {
		return ParseCSLJSON(data)
	}
	return ParseBibTeX(string(data))
}

// ParseBibTeX reads @type{key, field = {value} | "value" | 1999 | macro # ...}
// entries. @string macros are expanded, @comment and @preamble skipped;
// LaTeX accents and braces are taken off the values.
func ParseBibTeX(text string) ([]*Entry, error) {
	p := &bibParser{s: text, macros: map[string]string{
		"jan": "January", "feb": "February", "mar": "March", "apr": "April", "may": "May", "jun": "June",
		"jul": "July", "aug": "August", "sep": "September", "oct": "October", "nov": "November", "dec": "December",
	}}
	out := []*Entry{}
	for {
		at := strings.IndexByte(p.s[p.i:], '@')
		if at < 0 {
			break
		}
		p.i += at + 1
		kind := strings.ToLower(p.word())
		p.space()
		if p.i >= len(p.s) || (p.s[p.i] != '{' && p.s[p.i] != '(') {
			continue
		}
		closeCh := byte('}')
		if p.s[p.i] == '(' {
			closeCh = ')'
		}
		p.i++
		switch kind {
		case "comment", "preamble":
			p.skipGroup(closeCh)
			continue
		case "string":
			for _, f := range p.fields(closeCh) {
				p.macros[f[0]] = f[1]
			}
			continue
		}
		p.space()
		start := p.i
		for p.i < len(p.s) && p.s[p.i] != ',' && p.s[p.i] != closeCh {
			p.i++
		}
		key := strings.TrimSpace(p.s[start:p.i])
		if p.i < len(p.s) && p.s[p.i] == ',' {
			p.i++
		}
		e := &Entry{Key: key, Type: kind, Fields: map[string]string{}}
		raw := map[string]string{}
		for _, f := range p.fields(closeCh) {
			raw[f[0]] = f[1]
			e.Fields[f[0]] = cleanTeX(f[1])
		}
		// Names from the raw value: {World Health Organization} is one name
		// because of its braces.
		e.Authors = splitNames(raw["author"])
		e.Editors = splitNames(raw["editor"])
		if key != "" {
			out = append(out, e)
		}
	}
	return out, nil
}

type bibParser struct {
	s      string
	i      int
	macros map[string]string
}

func (p *bibParser) space() {
	for p.i < len(p.s) && unicode.IsSpace(rune(p.s[p.i])) {
		p.i++
	}
}

func (p *bibParser) word() string {
	start := p.i
	for p.i < len(p.s) && (unicode.IsLetter(rune(p.s[p.i])) || unicode.IsDigit(rune(p.s[p.i])) || strings.IndexByte("_-:.+/", p.s[p.i]) >= 0) {
		p.i++
	}
	return p.s[start:p.i]
}

func (p *bibParser) skipGroup(closeCh byte) {
	open := byte('{')
	if closeCh == ')' {
		open = '('
	}
	for depth := 1; p.i < len(p.s) && depth > 0; p.i++ {
		switch p.s[p.i] {
		case open:
			depth++
		case closeCh:
			depth--
		}
	}
}

// fields reads name = value pairs up to the entry's closing character.
func (p *bibParser) fields(closeCh byte) [][2]string {
	out := [][2]string{}
	for {
		p.space()
		if p.i >= len(p.s) {
			return out
		}
		if p.s[p.i] == closeCh {
			p.i++
			return out
		}
		if p.s[p.i] == ',' {
			p.i++
			continue
		}
		name := strings.ToLower(p.word())
		if name == "" {
			p.i++
			continue
		}
		p.space()
		if p.i >= len(p.s) || p.s[p.i] != '=' {
			continue
		}
		p.i++
		out = append(out, [2]string{name, p.value(closeCh)})
	}
}

// value is one field's value: pieces joined by #.
func (p *bibParser) value(closeCh byte) string {
	var b strings.Builder
	for {
		p.space()
		if p.i >= len(p.s) {
			break
		}
		switch c := p.s[p.i]; {
		case c == '{':
			depth := 0
			start := p.i + 1
			for ; p.i < len(p.s); p.i++ {
				if p.s[p.i] == '{' {
					depth++
				} else if p.s[p.i] == '}' {
					depth--
					if depth == 0 {
						break
					}
				}
			}
			b.WriteString(p.s[start:min(p.i, len(p.s))])
			p.i++
		case c == '"':
			p.i++
			start := p.i
			depth := 0
			for ; p.i < len(p.s); p.i++ {
				if p.s[p.i] == '{' {
					depth++
				} else if p.s[p.i] == '}' {
					depth--
				} else if p.s[p.i] == '"' && depth == 0 {
					break
				}
			}
			b.WriteString(p.s[start:min(p.i, len(p.s))])
			p.i++
		default:
			w := p.word()
			if w == "" {
				p.i++
				break
			}
			if v, ok := p.macros[strings.ToLower(w)]; ok {
				b.WriteString(v)
			} else {
				b.WriteString(w)
			}
		}
		p.space()
		if p.i < len(p.s) && p.s[p.i] == '#' {
			p.i++
			continue
		}
		break
	}
	_ = closeCh
	return strings.Join(strings.Fields(b.String()), " ")
}

var texAccents = strings.NewReplacer(
	`\"a`, "ä", `\"o`, "ö", `\"u`, "ü", `\"A`, "Ä", `\"O`, "Ö", `\"U`, "Ü",
	`\'a`, "á", `\'e`, "é", `\'i`, "í", `\'o`, "ó", `\'u`, "ú", `\'E`, "É",
	"\\`a", "à", "\\`e", "è", "\\`o", "ò", `\^e`, "ê", `\^o`, "ô", `\^a`, "â",
	`\~n`, "ñ", `\~a`, "ã", `\~o`, "õ", `\c{c}`, "ç", `\c c`, "ç", `\ss`, "ß", `\o`, "ø", `\O`, "Ø",
	`\aa`, "å", `\AA`, "Å", `\ae`, "æ", `\AE`, "Æ", `\&`, "&", `\%`, "%", `\_`, "_", `\$`, "$", `--`, "–", `~`, " ",
)

// cleanTeX takes accents, braces and the commonest commands off a value.
func cleanTeX(s string) string {
	s = texAccents.Replace(s)
	// {\"o} and \"{o} forms.
	for _, a := range []string{`"`, `'`, "`", `^`, `~`} {
		for _, v := range "aeiouAEIOUn" {
			s = strings.ReplaceAll(s, `\`+a+`{`+string(v)+`}`, texAccents.Replace(`\`+a+string(v)))
		}
	}
	for _, cmd := range []string{`\emph`, `\textit`, `\textbf`, `\textsc`, `\mkbibquote`} {
		s = strings.ReplaceAll(s, cmd, "")
	}
	s = strings.NewReplacer("{", "", "}", "").Replace(s)
	return strings.TrimSpace(s)
}

// splitNames reads "Last, First and First Last and {Org Name}".
func splitNames(s string) []Name {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	out := []Name{}
	for _, part := range splitAnd(s) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.EqualFold(part, "others") {
			out = append(out, Name{Literal: "others"})
			continue
		}
		if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") {
			out = append(out, Name{Literal: cleanTeX(part)})
			continue
		}
		part = cleanTeX(part)
		if fam, given, ok := strings.Cut(part, ","); ok {
			out = append(out, Name{Family: strings.TrimSpace(fam), Given: strings.TrimSpace(given)})
			continue
		}
		words := strings.Fields(part)
		// "Ludwig van Beethoven": the lower case particle goes with the family name.
		i := len(words) - 1
		for i > 0 && words[i-1] != "" && unicode.IsLower(rune(words[i-1][0])) {
			i--
		}
		out = append(out, Name{Given: strings.Join(words[:i], " "), Family: strings.Join(words[i:], " ")})
	}
	return out
}

// splitAnd splits at " and " outside braces.
func splitAnd(s string) []string {
	out := []string{}
	depth, start := 0, 0
	low := strings.ToLower(s)
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
		}
		if depth == 0 && strings.HasPrefix(low[i:], " and ") {
			out = append(out, s[start:i])
			i += 4
			start = i + 1
		}
	}
	return append(out, s[start:])
}

// ParseCSLJSON reads a CSL-JSON array (what Zotero's Better BibTeX and
// citeproc read).
func ParseCSLJSON(data []byte) ([]*Entry, error) {
	var items []map[string]any
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("not CSL-JSON: %v", err)
	}
	out := []*Entry{}
	for _, it := range items {
		key, _ := it["id"].(string)
		if key == "" {
			if n, ok := it["id"].(float64); ok {
				key = strconv.Itoa(int(n))
			}
		}
		if key == "" {
			continue
		}
		e := &Entry{Key: key, Fields: map[string]string{}}
		e.Type, _ = it["type"].(string)
		for k, v := range it {
			if s, ok := v.(string); ok {
				e.Fields[strings.ToLower(k)] = s
			}
		}
		if s := e.Fields["container-title"]; s != "" {
			e.Fields["journal"] = s
		}
		if s := e.Fields["page"]; s != "" {
			e.Fields["pages"] = s
		}
		if s := e.Fields["issue"]; s != "" {
			e.Fields["number"] = s
		}
		if s := e.Fields["url"]; s != "" {
			e.Fields["url"] = s
		}
		if iss, ok := it["issued"].(map[string]any); ok {
			if parts, ok := iss["date-parts"].([]any); ok && len(parts) > 0 {
				if first, ok := parts[0].([]any); ok && len(first) > 0 {
					if y, ok := first[0].(float64); ok {
						e.Fields["year"] = strconv.Itoa(int(y))
					} else if y, ok := first[0].(string); ok {
						e.Fields["year"] = y
					}
				}
			}
			if lit, ok := iss["literal"].(string); ok && e.Fields["year"] == "" {
				e.Fields["year"] = lit
			}
		}
		e.Authors = cslNames(it["author"])
		e.Editors = cslNames(it["editor"])
		out = append(out, e)
	}
	return out, nil
}

func cslNames(v any) []Name {
	list, ok := v.([]any)
	if !ok {
		return nil
	}
	out := []Name{}
	for _, x := range list {
		m, ok := x.(map[string]any)
		if !ok {
			continue
		}
		n := Name{}
		n.Family, _ = m["family"].(string)
		n.Given, _ = m["given"].(string)
		n.Literal, _ = m["literal"].(string)
		out = append(out, n)
	}
	return out
}

// ---------------------------------------------------------------------------
// Formatting (org-cite basic)
// ---------------------------------------------------------------------------

// Reference is one cited work inside a citation, with its own prefix and
// suffix ("see @doe p. 3").
type Reference struct {
	Key    string
	Prefix string
	Suffix string
}

// Citation is one [cite/style:...] object.
type Citation struct {
	Style  string // "", "t", "a", "na", "n", "nb", "ft"...
	Bare   bool
	Caps   bool
	Full   bool
	Prefix string // the common prefix, before the first reference
	Suffix string // the common suffix, after the last
	Refs   []Reference
}

// StyleName is a style's long name: org's short names (t, a, na, n, nb, ft,
// l) read as text, author, noauthor, nocite, numeric, note, locators.
func StyleName(s string) string {
	switch s {
	case "t":
		return "text"
	case "a":
		return "author"
	case "na":
		return "noauthor"
	case "n":
		return "nocite"
	case "nb":
		return "numeric"
	case "ft":
		return "note"
	case "l":
		return "locators"
	case "", "default", "nil":
		return ""
	}
	return s
}

// ParseCitation reads the inside of [cite/style:...]: the style with its
// variants (/t/b = text, bare), then ;-separated parts, the ones with @key
// references, any before them the common prefix and any after the common
// suffix.
func ParseCitation(styleSpec, body string) Citation {
	c := Citation{}
	// "/t/b": style t, variant b; "//b": the default style, bare.
	parts := strings.Split(strings.TrimPrefix(styleSpec, "/"), "/")
	if len(parts) > 0 {
		c.Style = StyleName(parts[0])
	}
	for _, v := range parts[1:] {
		switch v {
		case "b", "bare":
			c.Bare = true
		case "c", "caps":
			c.Caps = true
		case "f", "full":
			c.Full = true
		case "bc", "bare-caps":
			c.Bare, c.Caps = true, true
		case "bf", "bare-full":
			c.Bare, c.Full = true, true
		case "cf", "caps-full":
			c.Caps, c.Full = true, true
		case "bcf", "bare-caps-full":
			c.Bare, c.Caps, c.Full = true, true, true
		}
	}
	segs := strings.Split(body, ";")
	firstRef, lastRef := -1, -1
	for i, s := range segs {
		if keyAt(s) >= 0 {
			if firstRef < 0 {
				firstRef = i
			}
			lastRef = i
		}
	}
	if firstRef < 0 {
		return c
	}
	c.Prefix = strings.TrimSpace(strings.Join(segs[:firstRef], ";"))
	c.Suffix = strings.TrimSpace(strings.Join(segs[lastRef+1:], ";"))
	for _, s := range segs[firstRef : lastRef+1] {
		at := keyAt(s)
		if at < 0 {
			continue
		}
		end := at + 1
		for end < len(s) && isKeyChar(s[end]) {
			end++
		}
		// A key does not end in punctuation that belongs to the sentence.
		for end > at+1 && strings.IndexByte(".,:;!?", s[end-1]) >= 0 {
			end--
		}
		c.Refs = append(c.Refs, Reference{Key: s[at+1 : end], Prefix: strings.TrimSpace(s[:at]), Suffix: strings.TrimSpace(s[end:])})
	}
	return c
}

func keyAt(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] == '@' && (i == 0 || s[i-1] == ' ' || s[i-1] == '\t' || s[i-1] == '-' || s[i-1] == '[') && i+1 < len(s) && isKeyChar(s[i+1]) {
			return i
		}
	}
	return -1
}

func isKeyChar(c byte) bool {
	return c > ' ' && c != ';' && c != ']' && c != '[' && c != '@' && c != ','
}

// Format is a citation in org-cite basic's author-year form, or numeric
// ([1], [1, 3]) when numbers is given. missing is called for a key the
// bibliography does not have, and the key is written in its place. link,
// when given, wraps each work's part (not its prefix or suffix) - a link to
// its bibliography entry.
func Format(c Citation, entries map[string]*Entry, numbers map[string]int, missing func(string), link func(key, text string) string) string {
	style := c.Style
	if style == "nocite" {
		return ""
	}
	pieces := []string{}
	for _, r := range c.Refs {
		e := entries[r.Key]
		if e == nil {
			if missing != nil {
				missing(r.Key)
			}
		}
		one := ""
		switch {
		case e == nil:
			one = r.Key
		case style == "numeric" || (numbers != nil && style == ""):
			one = strconv.Itoa(numbers[r.Key])
		case style == "author":
			one = e.AuthorShort(c.Full)
		case style == "noauthor":
			one = e.Year()
		case style == "text":
			one = e.AuthorShort(c.Full) + " (" + e.Year() + ")"
			if c.Bare {
				one = e.AuthorShort(c.Full) + " " + e.Year()
			}
		default:
			one = e.AuthorShort(c.Full) + ", " + e.Year()
		}
		if c.Caps && one != "" {
			one = strings.ToUpper(one[:1]) + one[1:]
		}
		if link != nil && e != nil && one != "" {
			one = link(r.Key, one)
		}
		if r.Prefix != "" {
			one = r.Prefix + " " + one
		}
		if r.Suffix != "" {
			if strings.HasPrefix(r.Suffix, ",") {
				one += r.Suffix
			} else {
				one += ", " + r.Suffix
			}
		}
		pieces = append(pieces, one)
	}
	sep := "; "
	if style == "numeric" || (numbers != nil && style == "") {
		sep = ", "
	}
	body := strings.Join(pieces, sep)
	if c.Prefix != "" {
		body = c.Prefix + " " + body
	}
	if c.Suffix != "" {
		body += ", " + c.Suffix
	}
	switch {
	case style == "text" || style == "author" || c.Bare:
		return body
	case style == "numeric" || (numbers != nil && style == ""):
		return "[" + body + "]"
	}
	return "(" + body + ")"
}

// Sort puts entries in a bibliography's order: by first author's family
// name, then year, then title.
func Sort(es []*Entry) {
	sort.SliceStable(es, func(a, b int) bool {
		x, y := strings.ToLower(es[a].AuthorShort(true)), strings.ToLower(es[b].AuthorShort(true))
		if x != y {
			return x < y
		}
		if es[a].Year() != es[b].Year() {
			return es[a].Year() < es[b].Year()
		}
		return strings.ToLower(es[a].Title()) < strings.ToLower(es[b].Title())
	})
}

// Reference is one bibliography entry as org text (org-cite basic's
// author-year layout): "Doe, Jane, and Richard Roe. 2020. /Title/. Journal
// 12 (3): 45–67. doi:…". Its pieces are org markup, so every exporter draws
// it its own way.
func (e *Entry) Reference() string {
	parts := []string{}
	ns := e.names()
	if len(ns) > 0 {
		names := []string{}
		for i, n := range ns {
			switch {
			case n.Literal != "":
				names = append(names, n.Literal)
			case i == 0 && n.Given != "":
				names = append(names, n.Family+", "+n.Given)
			default:
				names = append(names, n.Full())
			}
		}
		who := names[0]
		if len(names) > 1 {
			who = strings.Join(names[:len(names)-1], ", ") + ", and " + names[len(names)-1]
		}
		if len(e.Authors) == 0 {
			who += " (ed.)"
		}
		parts = append(parts, strings.TrimSuffix(who, ".")+".")
	}
	parts = append(parts, e.Year()+".")
	if t := e.Title(); t != "" {
		if e.Field("journal") != "" || e.Field("booktitle") != "" {
			parts = append(parts, "“"+strings.TrimSuffix(t, ".")+".”")
		} else {
			parts = append(parts, "/"+strings.TrimSuffix(t, ".")+"/.")
		}
	}
	where := e.Field("journal")
	if where == "" {
		where = e.Field("journaltitle")
	}
	if where == "" {
		if b := e.Field("booktitle"); b != "" {
			where = "In /" + b + "/"
		}
	} else {
		where = "/" + where + "/"
	}
	if where != "" {
		if v := e.Field("volume"); v != "" {
			where += " " + v
			if n := e.Field("number"); n != "" {
				where += " (" + n + ")"
			}
		}
		if p := e.Field("pages"); p != "" {
			where += ": " + strings.ReplaceAll(p, "--", "–")
		}
		parts = append(parts, where+".")
	}
	if pub := e.Field("publisher"); pub != "" {
		if a := e.Field("address"); a != "" {
			pub = a + ": " + pub
		}
		parts = append(parts, pub+".")
	}
	if doi := e.Field("doi"); doi != "" {
		parts = append(parts, "[[https://doi.org/"+doi+"][doi:"+doi+"]].")
	} else if u := e.Field("url"); u != "" {
		parts = append(parts, "[["+u+"]].")
	}
	return strings.Join(parts, " ")
}

// Key is one citable work as a completion lists it.
type Key struct {
	Key    string
	Author string
	Year   string
	Title  string
}
