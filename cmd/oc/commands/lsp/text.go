package lsp

// Open documents, and the arithmetic between them and the protocol: file
// urls, positions in the encoding the client counts in, and the smallest edit
// that turns one text into another.

import (
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/ihdavids/go-org/org"
)

type document struct {
	uri     string
	path    string
	version int
	text    string
	lines   []string

	// Parsed on first use and dropped by every edit: completion and symbols
	// both want it, and a parse per keystroke is wasted on everything else.
	once sync.Once
	doc  *org.Document
}

func newDocument(uri string, version int, text string) *document {
	return &document{uri: uri, path: uriToPath(uri), version: version, text: text, lines: strings.Split(text, "\n")}
}

func (d *document) line(n int) string {
	if n < 0 || n >= len(d.lines) {
		return ""
	}
	return strings.TrimSuffix(d.lines[n], "\r")
}

// parsed is the document as the server would read it: the server's default
// TODO keywords, unless the file says its own.
func (d *document) parsed(conf func() *org.Configuration) *org.Document {
	d.once.Do(func() {
		d.doc = conf().Silent().Parse(strings.NewReader(d.text), d.path)
	})
	return d.doc
}

// ---------------------------------------------------------------------------
// Positions
// ---------------------------------------------------------------------------

// enc is how the client counts characters in a line: "utf-16" (the default
// every client has to accept) or "utf-8", which lets a column be a byte
// offset as it is everywhere else in this code.
type enc string

const (
	utf16Enc enc = "utf-16"
	utf8Enc  enc = "utf-8"
)

// col is a byte offset into a line, as the client counts it.
func (e enc) col(line string, b int) int {
	if b > len(line) {
		b = len(line)
	}
	if e == utf8Enc {
		return b
	}
	n := 0
	for _, r := range line[:b] {
		n += len(utf16.Encode([]rune{r}))
	}
	return n
}

// byteAt is the client's column as a byte offset into the line.
func (e enc) byteAt(line string, c int) int {
	if c <= 0 {
		return 0
	}
	if e == utf8Enc {
		if c > len(line) {
			return len(line)
		}
		for c > 0 && c < len(line) && !utf8.RuneStart(line[c]) {
			c--
		}
		return c
	}
	n := 0
	for i, r := range line {
		if n >= c {
			return i
		}
		n += len(utf16.Encode([]rune{r}))
	}
	return len(line)
}

func (e enc) pos(lines []string, line, b int) Position {
	l := ""
	if line >= 0 && line < len(lines) {
		l = strings.TrimSuffix(lines[line], "\r")
	}
	return Position{Line: line, Character: e.col(l, b)}
}

// offset is a position as a byte offset into the whole text.
func (e enc) offset(text string, p Position) int {
	off := 0
	for i := 0; i < p.Line; i++ {
		nl := strings.IndexByte(text[off:], '\n')
		if nl < 0 {
			return len(text)
		}
		off += nl + 1
	}
	end := strings.IndexByte(text[off:], '\n')
	line := text[off:]
	if end >= 0 {
		line = text[off : off+end]
	}
	return off + e.byteAt(line, p.Character)
}

// posOf is a byte offset into the whole text as a position.
func (e enc) posOf(text string, off int) Position {
	if off > len(text) {
		off = len(text)
	}
	line := strings.Count(text[:off], "\n")
	start := strings.LastIndexByte(text[:off], '\n') + 1
	end := strings.IndexByte(text[start:], '\n')
	l := text[start:]
	if end >= 0 {
		l = text[start : start+end]
	}
	return Position{Line: line, Character: e.col(l, off-start)}
}

// ---------------------------------------------------------------------------
// Edits
// ---------------------------------------------------------------------------

// diffEdit is one edit that turns old into new: everything between the
// longest common prefix and suffix. One edit rather than a real diff, because
// what the server does is one change in one place (a planning line, a
// result) and an editor applies one edit as one undo step.
func (e enc) diffEdit(old, new string) (TextEdit, bool) {
	if old == new {
		return TextEdit{}, false
	}
	p := 0
	for p < len(old) && p < len(new) && old[p] == new[p] {
		p++
	}
	for p > 0 && ((p < len(old) && !utf8.RuneStart(old[p])) || (p < len(new) && !utf8.RuneStart(new[p]))) {
		p--
	}
	s := 0
	for s < len(old)-p && s < len(new)-p && old[len(old)-1-s] == new[len(new)-1-s] {
		s++
	}
	for s > 0 && ((len(old)-s < len(old) && !utf8.RuneStart(old[len(old)-s])) || (len(new)-s < len(new) && !utf8.RuneStart(new[len(new)-s]))) {
		s--
	}
	return TextEdit{
		Range:   Range{Start: e.posOf(old, p), End: e.posOf(old, len(old)-s)},
		NewText: new[p : len(new)-s],
	}, true
}

// apply is one content change from didChange.
func (e enc) apply(text string, r *Range, newText string) string {
	if r == nil {
		return newText
	}
	a, b := e.offset(text, r.Start), e.offset(text, r.End)
	if b < a {
		a, b = b, a
	}
	return text[:a] + newText + text[b:]
}

// ---------------------------------------------------------------------------
// Files
// ---------------------------------------------------------------------------

func uriToPath(uri string) string {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "file" {
		return uri
	}
	p := u.Path
	if runtime.GOOS == "windows" {
		p = strings.TrimPrefix(p, "/")
		return filepath.FromSlash(p)
	}
	return p
}

func pathToURI(path string) string {
	p := filepath.ToSlash(path)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return (&url.URL{Scheme: "file", Path: p}).String()
}

// pathMap is where the server's files are on this machine, for a server on
// another one: its path prefix to ours.
type pathMap map[string]string

func (m pathMap) toLocal(p string) string {
	for server, local := range m {
		if strings.HasPrefix(p, server) {
			return local + strings.TrimPrefix(p, server)
		}
	}
	return p
}

func (m pathMap) toServer(p string) string {
	for server, local := range m {
		if strings.HasPrefix(p, local) {
			return server + strings.TrimPrefix(p, local)
		}
	}
	return p
}
