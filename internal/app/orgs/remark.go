package orgs

// org-remark: highlights on any file (or web page) with a note on each, kept
// in a marginalia file rather than in the file highlighted. The marginalia
// file is org-remark's own shape, so Emacs and orgs share one:
//
//	* notes.txt
//	:PROPERTIES:
//	:org-remark-file: notes.txt
//	:END:
//	** the highlighted words
//	:PROPERTIES:
//	:org-remark-beg: 120
//	:org-remark-end: 141
//	:org-remark-id: 9f2c41aa
//	:org-remark-label: nil
//	:org-remark-link: [[file:notes.txt::7]]
//	:org-remark-original-text: the highlighted words
//	:END:
//	The note on them.
//
// A web page is a source heading with :org-remark-url: instead (orgs's own
// spelling; org-remark's eww support files pages differently).
//
// Positions are Emacs's: characters, counted from 1, end exclusive. A file
// that changed after it was highlighted moves the words, so a highlight is
// found again by its original text, nearest to where it was (`Moved`), and
// the marginalia file is only written when a highlight is made, noted or
// removed - never by reading. Only with features.remark on.

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/ihdavids/orgs/internal/common"
)

// RemarkHighlight is one highlight, where it is now.
type RemarkHighlight struct {
	Id    string
	Beg   int
	End   int
	Label string
	Text  string
	Note  string
	// 0 based line and character column of Beg and End, for an editor.
	Line, Col       int
	EndLine, EndCol int
	// Found somewhere other than where it was written; Lost when its words
	// are not in the file at all any more.
	Moved bool `json:",omitempty"`
	Lost  bool `json:",omitempty"`
	// Where it is in the marginalia file.
	NotesLine int
}

// RemarkSource is a file or page with highlights.
type RemarkSource struct {
	Source string // the absolute path, or the url
	Title  string
	Url    bool `json:",omitempty"`
	Count  int
}

type remarkHL struct {
	RemarkHighlight
	row, end int
	original string
}

type remarkSrc struct {
	title, written string
	url            bool
	row, end       int
	hls            []remarkHL
}

func remarkNotesFile() string {
	name := "marginalia.org"
	if s := Conf().Server; s != nil && strings.TrimSpace(s.Remark.NotesFile) != "" {
		name = strings.TrimSpace(s.Remark.NotesFile)
	}
	if strings.HasPrefix(name, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			name = filepath.Join(home, name[2:])
		}
	}
	if !filepath.IsAbs(name) {
		if dirs := orgDirsOf(); len(dirs) > 0 {
			root, _ := filepath.Abs(dirs[0])
			name = filepath.Join(root, name)
		}
	}
	return filepath.Clean(name)
}

// remarkAbs is a source as written in the marginalia file, made absolute.
func remarkAbs(notes, written string) string {
	if strings.HasPrefix(written, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, written[2:])
		}
	}
	if !filepath.IsAbs(written) {
		return filepath.Clean(filepath.Join(filepath.Dir(notes), written))
	}
	return filepath.Clean(written)
}

// remarkWritten is how a source is written: relative to the marginalia file
// when it is under its folder, as org-remark does.
func remarkWritten(notes, abs string) string {
	if rel, err := filepath.Rel(filepath.Dir(notes), abs); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(abs, home+string(filepath.Separator)) {
		return "~/" + filepath.ToSlash(abs[len(home)+1:])
	}
	return abs
}

func isUrl(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

// remarkSourceKey is what a client's Source is compared by.
func remarkSourceKey(notes, src string) string {
	src = strings.TrimSpace(src)
	if isUrl(src) {
		return src
	}
	src = strings.TrimPrefix(src, "file://")
	if !filepath.IsAbs(src) && !strings.HasPrefix(src, "~/") {
		// A file as the server lists it: with relative orgDirs, relative to
		// where the server runs.
		if _, err := os.Stat(src); err == nil {
			if a, err := filepath.Abs(src); err == nil {
				return a
			}
		}
		// Else relative to the first org directory.
		if dirs := orgDirsOf(); len(dirs) > 0 {
			root, _ := filepath.Abs(dirs[0])
			return filepath.Clean(filepath.Join(root, src))
		}
	}
	return remarkAbs(notes, src)
}

// parseMarginalia reads the marginalia file's lines.
func parseMarginalia(notes string, lines []string) []remarkSrc {
	out := []remarkSrc{}
	for i := 0; i < len(lines); i++ {
		if headingLevel(lines[i]) != 1 {
			continue
		}
		end := subtreeEndRow(lines, i, 1, i)
		if end >= len(lines) {
			end = len(lines) - 1
		}
		props := lowerKeys(propsFromLines(lines, i, ownLinesEnd(lines, i, end)))
		src := remarkSrc{title: headingText(lines[i]), row: i, end: end}
		if v := props["org-remark-url"]; v != "" {
			src.written, src.url = v, true
		} else if v := props["org-remark-file"]; v != "" {
			src.written = v
		} else {
			continue
		}
		for j := i + 1; j <= end; j++ {
			if headingLevel(lines[j]) != 2 {
				continue
			}
			hend := subtreeEndRow(lines, j, 2, j)
			if hend > end {
				hend = end
			}
			own := ownLinesEnd(lines, j, hend)
			hp := lowerKeys(propsFromLines(lines, j, own))
			if hp["org-remark-id"] == "" {
				continue
			}
			beg, _ := strconv.Atoi(hp["org-remark-beg"])
			en, _ := strconv.Atoi(hp["org-remark-end"])
			label := hp["org-remark-label"]
			if label == "nil" {
				label = ""
			}
			h := remarkHL{RemarkHighlight: RemarkHighlight{Id: hp["org-remark-id"], Beg: beg, End: en, Label: label,
				Text: headingText(lines[j]), NotesLine: j}, row: j, end: hend, original: hp["org-remark-original-text"]}
			if h.original != "" {
				h.Text = h.original
			}
			_, h.Note = noterBody(lines, j)
			src.hls = append(src.hls, h)
		}
		out = append(out, src)
	}
	return out
}

func lowerKeys(m map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range m {
		out[strings.ToLower(k)] = v
	}
	return out
}

// headingText is a heading line without its stars.
func headingText(line string) string {
	return strings.TrimSpace(strings.TrimLeft(line, "*"))
}

// anchor finds a highlight in the file's text: where it was if its words are
// still there, else the occurrence of them nearest to where it was. Words are
// matched with any run of white space between them, because the marginalia
// file keeps the original text on one line and a highlight can span several.
func anchor(text []rune, h *RemarkHighlight) {
	words := strings.Fields(h.Text)
	if len(words) == 0 {
		return
	}
	for i := range words {
		words[i] = regexp.QuoteMeta(words[i])
	}
	re := regexp.MustCompile(strings.Join(words, `\s+`))
	s := string(text)
	at := h.Beg - 1
	best, bestEnd := -1, -1
	for _, m := range re.FindAllStringIndex(s, -1) {
		b := utf8.RuneCountInString(s[:m[0]])
		if best < 0 || abs(b-at) < abs(best-at) {
			best, bestEnd = b, b+utf8.RuneCountInString(s[m[0]:m[1]])
		}
	}
	if best < 0 {
		h.Lost = true
		return
	}
	h.Moved = best != at
	h.Beg, h.End = best+1, bestEnd+1
	h.Text = string(text[best:bestEnd])
	h.Line, h.Col = runeLineCol(text, h.Beg-1)
	h.EndLine, h.EndCol = runeLineCol(text, h.End-1)
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func runeLineCol(text []rune, at int) (int, int) {
	line, col := 0, 0
	for i := 0; i < at && i < len(text); i++ {
		if text[i] == '\n' {
			line, col = line+1, 0
		} else {
			col++
		}
	}
	return line, col
}

func readMarginalia() (string, []string, error) {
	notes := remarkNotesFile()
	lines, err := readFileLines(notes)
	if err != nil {
		if os.IsNotExist(err) {
			return notes, []string{}, nil
		}
		return notes, nil, err
	}
	return notes, lines, nil
}

/* SDOC: API
* GET /remark/sources — Every File And Page With Highlights
* GET /remark — One Source's Highlights
* POST /remark/highlight — Highlight A Passage
* POST /remark/note — Change A Highlight's Note
* POST /remark/delete — Take A Highlight Away

	#+BEGIN_EXAMPLE
	GET  /remark/sources              {"Ok": true, "NotesFile": "...", "Sources": [{Source, Title, Url, Count}]}
	GET  /remark?source=<path or url> {"Ok": true, "Highlights": [{Id, Beg, End, Label, Text, Note, Line, Col, EndLine, EndCol, Moved, Lost}]}
	POST /remark/highlight  {"Source": "...", "Beg": 120, "End": 141, "Text": "the words", "Label": "", "Note": ""}
	POST /remark/note       {"Id": "9f2c41aa", "Note": "..."}
	POST /remark/delete     {"Id": "9f2c41aa"}
	#+END_EXAMPLE

	org-remark's marginalia file (=remark: notesFile:=, default
	=marginalia.org= in the first org directory). Positions are characters
	counted from 1 with the end exclusive, as Emacs counts them. A highlight
	whose file has changed is found again by its words (=Moved=); one whose
	words have gone is =Lost=. A highlight with no =Beg= is placed at the
	first occurrence of =Text= at or after =Line= (0 based). Needs
	=features: remark:=.
EDOC */
func RequestRemarkSources(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "remark") {
		return
	}
	notes, lines, err := readMarginalia()
	if err != nil {
		dynJson(w, common.Result{Msg: err.Error()})
		return
	}
	out := []RemarkSource{}
	for _, s := range parseMarginalia(notes, lines) {
		src := s.written
		if !s.url {
			src = remarkAbs(notes, s.written)
		}
		out = append(out, RemarkSource{Source: src, Title: s.title, Url: s.url, Count: len(s.hls)})
	}
	dynJson(w, map[string]interface{}{"Ok": true, "NotesFile": notes, "Sources": out})
}

func RequestRemark(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "remark") {
		return
	}
	notes, lines, err := readMarginalia()
	if err != nil {
		dynJson(w, common.Result{Msg: err.Error()})
		return
	}
	key := remarkSourceKey(notes, r.URL.Query().Get("source"))
	out := []RemarkHighlight{}
	for _, s := range parseMarginalia(notes, lines) {
		if remarkSourceKey(notes, s.written) != key {
			continue
		}
		var text []rune
		if !s.url {
			if b, err := os.ReadFile(key); err == nil {
				text = []rune(strings.ReplaceAll(string(b), "\r\n", "\n"))
			}
		}
		for _, h := range s.hls {
			hl := h.RemarkHighlight
			if text != nil {
				anchor(text, &hl)
			}
			out = append(out, hl)
		}
	}
	dynJson(w, map[string]interface{}{"Ok": true, "NotesFile": notes, "Highlights": out})
}

func newRemarkId() string {
	b := make([]byte, 4)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func PostRemarkHighlight(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "remark") {
		return
	}
	body, _ := io.ReadAll(r.Body)
	var req struct {
		Source            string
		Beg, End, Line    int
		Text, Label, Note string
	}
	if err := json.Unmarshal(body, &req); err != nil {
		dynJson(w, common.Result{Msg: err.Error()})
		return
	}
	if strings.TrimSpace(req.Text) == "" || strings.TrimSpace(req.Source) == "" {
		dynJson(w, common.Result{Msg: "a highlight needs a source and the words highlighted"})
		return
	}
	notes, lines, err := readMarginalia()
	if err != nil {
		dynJson(w, common.Result{Msg: err.Error()})
		return
	}
	key := remarkSourceKey(notes, req.Source)
	url := isUrl(key)
	text := strings.ReplaceAll(req.Text, "\r\n", "\n")
	hl := RemarkHighlight{Beg: req.Beg, End: req.End, Text: text}
	if !url {
		b, err := os.ReadFile(key)
		if err != nil {
			dynJson(w, common.Result{Msg: fmt.Sprintf("cannot read %s: %v", key, err)})
			return
		}
		src := []rune(strings.ReplaceAll(string(b), "\r\n", "\n"))
		if hl.Beg <= 0 {
			// Start looking at the line the client was on.
			at, line := 0, 0
			for at < len(src) && line < req.Line {
				if src[at] == '\n' {
					line++
				}
				at++
			}
			hl.Beg = at + 1
		}
		anchor(src, &hl)
		if hl.Lost {
			dynJson(w, common.Result{Msg: "those words are not in " + filepath.Base(key)})
			return
		}
	} else if hl.End <= hl.Beg {
		hl.End = hl.Beg + len([]rune(text))
	}
	hl.Id = newRemarkId()

	label := strings.TrimSpace(req.Label)
	if label == "" {
		label = "nil"
	}
	title := strings.Join(strings.Fields(text), " ")
	if rs := []rune(title); len(rs) > 80 {
		title = strings.TrimSpace(string(rs[:77])) + "…"
	}
	add := []string{"** " + title, ":PROPERTIES:",
		":org-remark-beg: " + strconv.Itoa(hl.Beg),
		":org-remark-end: " + strconv.Itoa(hl.End),
		":org-remark-id: " + hl.Id,
		":org-remark-label: " + label}
	written := key
	if !url {
		written = remarkWritten(notes, key)
		add = append(add, fmt.Sprintf(":org-remark-link: [[file:%s::%d]]", written, hl.Line+1))
	}
	add = append(add, ":org-remark-original-text: "+strings.Join(strings.Fields(text), " "), ":END:")
	if n := strings.TrimSpace(req.Note); n != "" {
		add = append(add, strings.Split(n, "\n")...)
	}
	// Under the source's heading, in position order; the heading made when
	// this is the source's first highlight.
	var at int
	found := false
	for _, s := range parseMarginalia(notes, lines) {
		if remarkSourceKey(notes, s.written) != key {
			continue
		}
		found = true
		at = s.end + 1
		for at-1 > s.row && strings.TrimSpace(lines[at-1]) == "" {
			at-- // before the blank lines that end the file, not after them
		}
		for _, h := range s.hls {
			if h.Beg > hl.Beg {
				at = h.row
				break
			}
		}
		break
	}
	if !found {
		for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
			lines = lines[:len(lines)-1]
		}
		prop := ":org-remark-file: " + written
		title := filepath.Base(key)
		if url {
			prop, title = ":org-remark-url: "+key, key
		}
		lines = append(lines, "* "+title, ":PROPERTIES:", prop, ":END:")
		at = len(lines)
	}
	lines = splice(lines, at, add)
	if err := writeRemark(notes, lines); err != nil {
		dynJson(w, common.Result{Msg: err.Error()})
		return
	}
	dynJson(w, map[string]interface{}{"Ok": true, "Highlight": hl})
}

// writeRemark writes the marginalia file, and has the server read it the
// first time it is made, so it shows up with every other org file.
func writeRemark(notes string, lines []string) error {
	_, statErr := os.Stat(notes)
	if err := os.MkdirAll(filepath.Dir(notes), 0755); err != nil {
		return err
	}
	if err := writeLines(notes, lines); err != nil {
		return err
	}
	if os.IsNotExist(statErr) && GetDb().FindByFile(notes) == nil {
		GetDb().LoadFile(notes, true)
	}
	return nil
}

func remarkFind(id string) (string, []string, *remarkHL, error) {
	notes, lines, err := readMarginalia()
	if err != nil {
		return notes, nil, nil, err
	}
	for _, s := range parseMarginalia(notes, lines) {
		for i := range s.hls {
			if s.hls[i].Id == id {
				return notes, lines, &s.hls[i], nil
			}
		}
	}
	return notes, lines, nil, fmt.Errorf("no highlight %q", id)
}

func PostRemarkNote(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "remark") {
		return
	}
	body, _ := io.ReadAll(r.Body)
	var req struct{ Id, Note string }
	if err := json.Unmarshal(body, &req); err != nil {
		dynJson(w, common.Result{Msg: err.Error()})
		return
	}
	notes, lines, h, err := remarkFind(req.Id)
	if err != nil {
		dynJson(w, common.Result{Msg: err.Error()})
		return
	}
	// The note is the heading's own text after its drawer.
	own := ownLinesEnd(lines, h.row, h.end)
	_, pe, ok := drawerAt(lines, h.row, own, "PROPERTIES")
	if !ok {
		dynJson(w, common.Result{Msg: "that highlight has no property drawer"})
		return
	}
	keep := append([]string{}, lines[:pe+1]...)
	if n := strings.TrimSpace(req.Note); n != "" {
		keep = append(keep, strings.Split(n, "\n")...)
	}
	lines = append(keep, lines[own+1:]...)
	if err := writeRemark(notes, lines); err != nil {
		dynJson(w, common.Result{Msg: err.Error()})
		return
	}
	dynJson(w, common.Result{Ok: true})
}

func PostRemarkDelete(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "remark") {
		return
	}
	body, _ := io.ReadAll(r.Body)
	var req struct{ Id string }
	if err := json.Unmarshal(body, &req); err != nil {
		dynJson(w, common.Result{Msg: err.Error()})
		return
	}
	notes, lines, h, err := remarkFind(req.Id)
	if err != nil {
		dynJson(w, common.Result{Msg: err.Error()})
		return
	}
	lines = append(lines[:h.row:h.row], lines[h.end+1:]...)
	if err := writeRemark(notes, lines); err != nil {
		dynJson(w, common.Result{Msg: err.Error()})
		return
	}
	dynJson(w, common.Result{Ok: true})
}
