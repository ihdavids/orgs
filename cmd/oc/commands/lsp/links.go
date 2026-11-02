package lsp

// Where a link goes: go-to-definition, document links, and the headings of
// every file for workspace symbols.
//
// A link is followed the way org follows it: *Title is a heading of this file,
// #id a CUSTOM_ID, a bare word a <<target>>, a #+NAME: or else a heading;
// file:x.org::search the same inside that file (or ::42, a line); id:X is
// asked of the server, which knows every file's ids.

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/ihdavids/orgs/internal/common"
)

func (s *server) linkAt(d *document, p Position) *link {
	cur := s.enc.byteAt(d.line(p.Line), p.Character)
	for _, l := range linksIn(d.lines, blocks(d.lines)) {
		if l.line == p.Line && cur >= l.start && cur <= l.end {
			lk := l
			return &lk
		}
	}
	return nil
}

func (s *server) definition(p positionParams) []Location {
	d := s.doc(p.TextDocument.URI)
	out := []Location{}
	if d == nil {
		return out
	}
	l := s.linkAt(d, p.Position)
	if l == nil {
		return out
	}
	if loc, ok := s.follow(d, l.target); ok {
		out = append(out, loc)
	}
	return out
}

// follow is where a link target goes, when it goes somewhere in a file.
func (s *server) follow(d *document, target string) (Location, bool) {
	proto, rest := splitLink(target)
	switch proto {
	case "id":
		return s.followId(rest)
	case "file":
		file, search, _ := strings.Cut(rest, "::")
		path := s.resolveFile(d, file)
		if search == "" {
			return Location{URI: pathToURI(path)}, true
		}
		lines := s.linesOf(path)
		if lines == nil {
			return Location{URI: pathToURI(path)}, true
		}
		other := newDocument(pathToURI(path), 0, strings.Join(lines, "\n"))
		if row, col, ok := s.search(other, search); ok {
			return Location{URI: other.uri, Range: s.point(other, row, col)}, true
		}
		return Location{URI: other.uri}, true
	case "":
		if row, col, ok := s.search(d, rest); ok {
			return Location{URI: d.uri, Range: s.point(d, row, col)}, true
		}
	}
	return Location{}, false
}

func (s *server) point(d *document, row, col int) Range {
	p := s.enc.pos(d.lines, row, col)
	return Range{Start: p, End: p}
}

func (s *server) followId(id string) (Location, bool) {
	// The search for completion already has every id; a link written a
	// minute ago is not in it yet, so ask outright when it misses.
	for _, h := range s.ids() {
		if h.Id == id {
			return Location{URI: pathToURI(h.File), Range: Range{Start: Position{Line: h.Line}, End: Position{Line: h.Line}}}, true
		}
	}
	t, err := get[common.Todo](s.api, "id/"+commandsPathEscape(id), nil)
	if err != nil || t.Filename == "" {
		return Location{}, false
	}
	return Location{URI: pathToURI(s.local(t.Filename)), Range: Range{Start: Position{Line: t.LineNum}, End: Position{Line: t.LineNum}}}, true
}

var lineSearchRe = regexp.MustCompile(`^\d+$`)

// search finds what a link's search part names in d: the row and the byte
// column to put the cursor at.
func (s *server) search(d *document, q string) (int, int, bool) {
	q = strings.TrimSpace(q)
	if q == "" {
		return 0, 0, false
	}
	if lineSearchRe.MatchString(q) {
		n, _ := strconv.Atoi(q)
		if n < 1 {
			n = 1
		}
		if n > len(d.lines) {
			n = len(d.lines)
		}
		return n - 1, 0, true
	}
	hs := s.headings(d)
	if strings.HasPrefix(q, "*") {
		t := strings.TrimSpace(q[1:])
		for _, h := range hs {
			if sameTitle(h.title, t) {
				return h.line, h.titleCol, true
			}
		}
		return 0, 0, false
	}
	if strings.HasPrefix(q, "#") {
		for _, h := range hs {
			if h.customId == q[1:] {
				return h.line, h.titleCol, true
			}
		}
		return 0, 0, false
	}
	// A dedicated target, then a name, then a heading: org's order.
	for i := range d.lines {
		for _, m := range targetDefRe.FindAllStringSubmatchIndex(d.line(i), -1) {
			if strings.EqualFold(d.line(i)[m[2]:m[3]], q) {
				return i, m[0], true
			}
		}
	}
	for i := range d.lines {
		if m := nameRe.FindStringSubmatch(d.line(i)); m != nil && strings.EqualFold(m[1], q) {
			return i, 0, true
		}
	}
	for _, h := range hs {
		if sameTitle(h.title, q) {
			return h.line, h.titleCol, true
		}
	}
	return 0, 0, false
}

// commandsPathEscape is a path segment: an id is a uuid in practice, but a
// CUSTOM_ID can be anything.
func commandsPathEscape(s string) string {
	r := strings.NewReplacer("%", "%25", "/", "%2F", "?", "%3F", "#", "%23", " ", "%20")
	return r.Replace(s)
}

// documentLinks makes urls and files clickable. A link with a protocol the
// server's yaml defines (jira:X-1) is resolved when the editor asks
// (documentLink/resolve), not for every link on every keystroke.
func (s *server) documentLinks(uri string) []DocumentLink {
	d := s.doc(uri)
	out := []DocumentLink{}
	if d == nil {
		return out
	}
	for _, l := range linksIn(d.lines, blocks(d.lines)) {
		r := Range{Start: s.enc.pos(d.lines, l.line, l.start), End: s.enc.pos(d.lines, l.line, l.end)}
		proto, rest := splitLink(l.target)
		switch proto {
		case "http", "https", "ftp", "mailto":
			out = append(out, DocumentLink{Range: r, Target: l.target})
		case "file":
			file, search, _ := strings.Cut(rest, "::")
			path := s.resolveFile(d, file)
			t := pathToURI(path)
			if lineSearchRe.MatchString(search) {
				t += "#L" + search
			}
			out = append(out, DocumentLink{Range: r, Target: t})
		case "", "id":
			// Definition goes there; a document link would be a second way
			// to the same place that editors draw as an underline.
		default:
			out = append(out, DocumentLink{Range: r, Data: l.target})
		}
	}
	return out
}

func (s *server) resolveLink(l DocumentLink) DocumentLink {
	raw, _ := l.Data.(string)
	if raw == "" {
		return l
	}
	res, err := get[common.LinkResolution](s.api, "links/resolve", map[string]string{"link": raw})
	if err == nil && res.Ok && res.Url != "" {
		l.Target = res.Url
	}
	return l
}

// workspaceSymbols is every heading whose title has the words in it, from the
// server's search.
func (s *server) workspaceSymbols(q string) []SymbolInformation {
	out := []SymbolInformation{}
	q = strings.TrimSpace(q)
	if q == "" {
		return out
	}
	// Anything that means something to a regexp or ends the query's string
	// matches any one character instead: no escaping to get wrong twice.
	safe := strings.Map(func(r rune) rune {
		if strings.ContainsRune(`\.+*?()|[]{}^$"`, r) {
			return '.'
		}
		return r
	}, q)
	expr := `MatchHeadline("(?i)` + safe + `")`
	todos, err := get[[]common.Todo](s.api, "search", map[string]string{"query": expr})
	if err != nil {
		return out
	}
	for i, t := range todos {
		if i == 200 {
			break
		}
		kind := symNamespace
		if t.Status != "" {
			kind = symEvent
		}
		path := s.local(t.Filename)
		p := Position{Line: t.LineNum}
		out = append(out, SymbolInformation{Name: t.Headline, Kind: kind, ContainerName: filepath.Base(path),
			Location: Location{URI: pathToURI(path), Range: Range{Start: p, End: p}}})
	}
	return out
}
