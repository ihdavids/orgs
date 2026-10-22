package orgs

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// The DM's journal on a map: ordinary org headings, in any org file, tied to a
// spot on a map by two properties -
//
//	:DND_MAP: ../dungeons/crypt.org   the map (relative to the heading's file)
//	:DND_AT:  12.5 8.25                where, in squares of the plan (x y)
//
// DND_MAP is inherited the way org inherits properties: from a parent heading,
// or from the file's own #+DND_MAP: keyword, so a file of notes for one map
// names it once. A heading's subtree is its note, sub headings and all; a sub
// heading with a DND_AT of its own is a pin of its own as well.
//
// Never the map's own org file: that one is written again from the dungeon's
// data on every save (rewriteDungeonOrg), and a note in it would be lost. New
// notes go to <stem>-journal.org beside the map.
//
// Every write is a line edit (see Traps: line edits), and a heading is named by
// its file, line and title together, the title checked so a stale line from a
// page drawn before somebody else's edit cannot write into the wrong heading.

type journalEntry struct {
	File  string  `json:"file"`
	Rel   string  `json:"rel"`
	Line  int     `json:"line"` // zero based, the heading's own line
	End   int     `json:"end"`  // the line after its subtree
	Level int     `json:"level"`
	Title string  `json:"title"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	// Its colour on the map (DND_COLOUR), when it has one.
	Colour string `json:"colour,omitempty"`
	// The subtree, its heading line first.
	Text string `json:"text"`
}

var (
	jHeadRe    = regexp.MustCompile(`^(\*+)\s+(.*?)\s*$`)
	jKeywordRe = regexp.MustCompile(`(?i)^#\+DND_MAP:\s*(.*?)\s*$`)
)

// journalTarget reads a DND_MAP value as a file, relative to the file it is
// written in: a bare path, file:path, or an org link [[file:path][words]].
func journalTarget(from, val string) string {
	v := strings.TrimSpace(val)
	if strings.HasPrefix(v, "[[") {
		v = strings.TrimPrefix(v, "[[")
		if i := strings.Index(v, "]"); i >= 0 {
			v = v[:i]
		}
	}
	v = strings.TrimPrefix(v, "file:")
	if i := strings.Index(v, "::"); i >= 0 {
		v = v[:i]
	}
	if v == "" {
		return ""
	}
	if strings.HasPrefix(v, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			v = filepath.Join(home, v[2:])
		}
	}
	if !filepath.IsAbs(v) {
		v = filepath.Join(filepath.Dir(from), filepath.FromSlash(v))
	}
	return filepath.Clean(v)
}

// headingLevel is how many stars a heading line has, or 0 for any other line.
func headingLevel(line string) int {
	m := jHeadRe.FindStringSubmatch(line)
	if m == nil {
		return 0
	}
	return len(m[1])
}

// noteOwnEnd is the last line of a heading's own text: the line before the next
// heading of any level.
func noteOwnEnd(lines []string, at int) int {
	for i := at + 1; i < len(lines); i++ {
		if headingLevel(lines[i]) > 0 {
			return i - 1
		}
	}
	return len(lines) - 1
}

// subtreeEnd is the line after a heading's subtree.
func subtreeEnd(lines []string, at int) int {
	lvl := headingLevel(lines[at])
	for i := at + 1; i < len(lines); i++ {
		if l := headingLevel(lines[i]); l > 0 && l <= lvl {
			return i
		}
	}
	return len(lines)
}

// ownProps reads a heading's own property drawer.
func ownProps(lines []string, at int) map[string]string {
	out := map[string]string{}
	ps, pe, ok := drawerAt(lines, at, noteOwnEnd(lines, at), "PROPERTIES")
	if !ok {
		return out
	}
	for i := ps + 1; i < pe; i++ {
		if m := propLineRe.FindStringSubmatch(lines[i]); m != nil {
			out[strings.ToUpper(m[2])] = strings.TrimSpace(m[4])
		}
	}
	return out
}

// parseAt reads "x y" (or "x,y").
func parseAt(v string) (float64, float64, bool) {
	f := strings.FieldsFunc(v, func(r rune) bool { return r == ' ' || r == ',' || r == '\t' })
	if len(f) != 2 {
		return 0, 0, false
	}
	x, e1 := strconv.ParseFloat(f[0], 64)
	y, e2 := strconv.ParseFloat(f[1], 64)
	return x, y, e1 == nil && e2 == nil
}

func formatAt(x, y float64) string {
	return strconv.FormatFloat(x, 'f', -1, 64) + " " + strconv.FormatFloat(y, 'f', -1, 64)
}

// journalIn finds the entries in one file's lines that are on the map.
func journalIn(file string, lines []string, mapFile string) []journalEntry {
	out := []journalEntry{}
	fileMap := ""
	// Each level's DND_MAP as the walk goes down the outline.
	stack := []string{}
	inBlock := false
	for i, line := range lines {
		t := strings.ToUpper(strings.TrimSpace(line))
		if strings.HasPrefix(t, "#+BEGIN_") {
			inBlock = true
		} else if strings.HasPrefix(t, "#+END_") {
			inBlock = false
		}
		if inBlock {
			continue
		}
		lvl := headingLevel(line)
		if lvl == 0 {
			if len(stack) == 0 {
				if m := jKeywordRe.FindStringSubmatch(line); m != nil {
					fileMap = journalTarget(file, m[1])
				}
			}
			continue
		}
		for len(stack) >= lvl {
			stack = stack[:len(stack)-1]
		}
		for len(stack) < lvl-1 {
			stack = append(stack, "")
		}
		props := ownProps(lines, i)
		here := ""
		if v, ok := props["DND_MAP"]; ok && v != "" {
			here = journalTarget(file, v)
		}
		stack = append(stack, here)
		at, ok := props["DND_AT"]
		if !ok {
			continue
		}
		x, y, ok := parseAt(at)
		if !ok {
			continue
		}
		target := fileMap
		for _, s := range stack {
			if s != "" {
				target = s
			}
		}
		if target != mapFile {
			continue
		}
		end := subtreeEnd(lines, i)
		m := jHeadRe.FindStringSubmatch(line)
		out = append(out, journalEntry{
			File: file, Rel: orgRel(file), Line: i, End: end, Level: lvl, Title: m[2],
			X: x, Y: y, Colour: props["DND_COLOUR"], Text: strings.Join(lines[i:end], "\n"),
		})
	}
	return out
}

// journalOf is every entry on a map, from every org file.
func journalOf(mapFile string) []journalEntry {
	out := []journalEntry{}
	walkOrg(func(p string) {
		if filepath.Clean(p) == mapFile {
			return
		}
		b, err := os.ReadFile(p)
		if err != nil || !bytes.Contains(b, []byte("DND_AT")) {
			return
		}
		out = append(out, journalIn(p, fileLinesFresh(p), mapFile)...)
	})
	return out
}

// The file new notes for a map go into, beside it.
func journalFileFor(mapFile string) string {
	return strings.TrimSuffix(mapFile, ".org") + "-journal.org"
}

// mapOrgFile reads the map's file from a request.
func mapOrgFile(w http.ResponseWriter, file string) (string, bool) {
	fname, ok := insideOrgDirs(strings.TrimSpace(file))
	if !ok || filepath.Ext(fname) != ".org" {
		dndError(w, http.StatusBadRequest, "the map has to be an org file inside one of the org directories")
		return "", false
	}
	if _, err := os.Stat(strings.TrimSuffix(fname, ".org") + ".dungeon.json"); err != nil {
		dndError(w, http.StatusNotFound, "%s is not a saved map", filepath.Base(fname))
		return "", false
	}
	return fname, true
}

func answerJournal(w http.ResponseWriter, mapFile, msg string, made *journalEntry) {
	res := map[string]interface{}{"Ok": true, "Msg": msg, "Entries": journalOf(mapFile), "Journal": journalFileFor(mapFile)}
	if made != nil {
		res["Made"] = made
	}
	dndJson(w, res)
}

/* SDOC: API
* GET /dnd/journal — The DM's Notes on a Map
	Every org heading tied to a spot on a map: a heading with =:DND_AT: x y= (squares of the plan)
	whose =DND_MAP= - its own, a parent's, or the file's =#+DND_MAP:= keyword - names the map
	(relative to the file it is written in; a bare path, =file:path= or an org link). Any org file
	may hold them, but not the map's own, which is written again from the map's data on every save.

	*Method:* =GET=

	*Query:* =file= the map's org file.

	*Response:* ={Ok, Entries: [{file, rel, line, end, level, title, x, y, colour, text}], Journal}=:
	=line= the heading's line (zero based), =end= the line after its subtree, =text= the subtree,
	=Journal= the file new notes go into (=<map>-journal.org= beside the map).
	EDOC */
func RequestDndJournal(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	mapFile, ok := mapOrgFile(w, r.URL.Query().Get("file"))
	if !ok {
		return
	}
	answerJournal(w, mapFile, "", nil)
}

type journalRequest struct {
	Op    string  `json:"op"`
	Map   string  `json:"map"`
	File  string  `json:"file"`
	Line  int     `json:"line"`
	Title string  `json:"title"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	// new: under this heading (file, line, title), else at the end of File.
	Under *struct {
		File  string `json:"file"`
		Line  int    `json:"line"`
		Title string `json:"title"`
	} `json:"under"`
	Colour *string `json:"colour"`
	// place: the heading by its hash instead of file, line and title (a
	// heading picked from the refile targets).
	Hash string `json:"hash"`
}

// journalHeading reads a file and finds the heading a request names.
func journalHeading(file string, line int, title string) (string, []string, error) {
	fname, ok := insideOrgDirs(strings.TrimSpace(file))
	if !ok || filepath.Ext(fname) != ".org" {
		return "", nil, fmt.Errorf("the note has to be in an org file inside one of the org directories")
	}
	lines := fileLinesFresh(fname)
	if lines == nil {
		return "", nil, fmt.Errorf("could not read %s", filepath.Base(fname))
	}
	if line < 0 || line >= len(lines) {
		return "", nil, fmt.Errorf("%s has changed: open the note again", filepath.Base(fname))
	}
	m := jHeadRe.FindStringSubmatch(lines[line])
	if m == nil || m[2] != title {
		return "", nil, fmt.Errorf("%s has changed: open the note again", filepath.Base(fname))
	}
	return fname, lines, nil
}

// inheritedMap is the map a heading would be on without a DND_MAP of its own.
func inheritedMap(file string, lines []string, at int) string {
	lvl := headingLevel(lines[at])
	for i := at - 1; i >= 0 && lvl > 1; i-- {
		l := headingLevel(lines[i])
		if l == 0 || l >= lvl {
			continue
		}
		lvl = l
		if v := ownProps(lines, i)["DND_MAP"]; v != "" {
			return journalTarget(file, v)
		}
	}
	for _, line := range lines {
		if headingLevel(line) > 0 {
			break
		}
		if m := jKeywordRe.FindStringSubmatch(line); m != nil {
			return journalTarget(file, m[1])
		}
	}
	return ""
}

// mapLink is how a note names its map: relative to the note's file.
func mapLink(noteFile, mapFile string) string {
	if rel, err := filepath.Rel(filepath.Dir(noteFile), mapFile); err == nil {
		return filepath.ToSlash(rel)
	}
	return mapFile
}

// placeProps is what a heading needs to be on the map at x, y: DND_AT, and
// DND_MAP unless it would get the map from a parent or the file already.
func placeProps(file string, lines []string, at int, mapFile string, x, y float64) map[string]string {
	set := map[string]string{"DND_AT": formatAt(x, y)}
	own := ownProps(lines, at)["DND_MAP"]
	if own != "" && journalTarget(file, own) == mapFile {
		return set
	}
	if inheritedMap(file, lines, at) != mapFile {
		set["DND_MAP"] = mapLink(file, mapFile)
	} else if own != "" {
		set["DND_MAP"] = ""
	}
	return set
}

func writeJournalLines(fname string, lines []string) error {
	if err := writeLines(fname, lines); err != nil {
		return err
	}
	PublishReload(fname)
	return nil
}

/* SDOC: API
* POST /dnd/journal — Put Notes on a Map
	Ties org headings to spots on a map, or makes new ones there, as line edits of the files.

	*Method:* =POST=

	*Request Body:* ={op, map, file, line, title, x, y, under, colour}=, =map= the map's org file;
	a heading is named by =file=, =line= (zero based) and =title= (the heading line without its
	stars, checked, so a stale line is refused rather than written into another heading).
	- =place= :: put the heading at =x y=: sets =DND_AT=, and =DND_MAP= when it would not get the
	  map from a parent or the file; also moves a note already there. =colour= sets =DND_COLOUR=.
	  =hash= may name the heading instead of =file=, =line= and =title=.
	- =unplace= :: takes =DND_AT= (and its own =DND_MAP=, =DND_COLOUR=) off: the heading stays.
	- =new= :: a new heading called =title= at =x y=: under the heading =under= ={file, line, title}=
	  as its last child, else at the end of =file=, else of =<map>-journal.org= (made with
	  =#+DND_MAP:= naming the map, so its headings need only =DND_AT=).

	*Response:* as =GET /dnd/journal=, with =Made= the entry made or placed (=new=, =place=).
	EDOC */
func PostDndJournal(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	var req journalRequest
	if !dndBody(w, r, &req) {
		return
	}
	mapFile, ok := mapOrgFile(w, req.Map)
	if !ok {
		return
	}
	switch req.Op {
	case "place", "unplace":
		if req.Hash != "" {
			sec := GetDb().FindByHash(req.Hash)
			if sec == nil || sec.Headline == nil {
				dndError(w, http.StatusNotFound, "no heading with that hash: pick it again")
				return
			}
			f := GetDb().FileFromSection(sec)
			if f == nil || f.Doc == nil {
				dndError(w, http.StatusNotFound, "that heading has no file")
				return
			}
			req.File, req.Line = f.Doc.Path, sec.Headline.Pos.Row
			if lines := fileLinesFresh(req.File); req.Line >= 0 && req.Line < len(lines) {
				if m := jHeadRe.FindStringSubmatch(lines[req.Line]); m != nil {
					req.Title = m[2]
				}
			}
		}
		fname, lines, err := journalHeading(req.File, req.Line, req.Title)
		if err != nil {
			dndError(w, http.StatusConflict, "%s", err)
			return
		}
		if fname == mapFile {
			dndError(w, http.StatusBadRequest, "the map's own file is written again on every save: keep notes in another file")
			return
		}
		var set map[string]string
		if req.Op == "place" {
			set = placeProps(fname, lines, req.Line, mapFile, req.X, req.Y)
			if req.Colour != nil {
				set["DND_COLOUR"] = strings.TrimSpace(*req.Colour)
			}
		} else {
			set = map[string]string{"DND_AT": "", "DND_COLOUR": ""}
			if own := ownProps(lines, req.Line)["DND_MAP"]; own != "" && journalTarget(fname, own) == mapFile {
				set["DND_MAP"] = ""
			}
		}
		lines, err = setPropsIn(lines, req.Line, noteOwnEnd(lines, req.Line), "", set)
		if err == nil {
			// An emptied drawer is taken away with its last property.
			if ps, pe, has := drawerAt(lines, req.Line, noteOwnEnd(lines, req.Line), "PROPERTIES"); has && pe == ps+1 {
				lines = append(lines[:ps], lines[pe+1:]...)
			}
			err = writeJournalLines(fname, lines)
		}
		if err != nil {
			dndError(w, http.StatusInternalServerError, "could not write %s: %s", filepath.Base(fname), err)
			return
		}
		var made *journalEntry
		if req.Op == "place" {
			// The properties go under the heading line, so it has not moved.
			for _, e := range journalIn(fname, fileLinesFresh(fname), mapFile) {
				if e.Line == req.Line {
					e := e
					made = &e
				}
			}
		}
		answerJournal(w, mapFile, "", made)
	case "new":
		title := strings.Join(strings.Fields(req.Title), " ")
		if title == "" {
			dndError(w, http.StatusBadRequest, "a note needs a title")
			return
		}
		var fname string
		var lines []string
		at, level := -1, 1
		if req.Under != nil && req.Under.File != "" {
			var err error
			fname, lines, err = journalHeading(req.Under.File, req.Under.Line, req.Under.Title)
			if err != nil {
				dndError(w, http.StatusConflict, "%s", err)
				return
			}
			at = subtreeEnd(lines, req.Under.Line)
			level = headingLevel(lines[req.Under.Line]) + 1
		} else {
			fname = journalFileFor(mapFile)
			if strings.TrimSpace(req.File) != "" {
				var ok bool
				if fname, ok = insideOrgDirs(strings.TrimSpace(req.File)); !ok || filepath.Ext(fname) != ".org" {
					dndError(w, http.StatusBadRequest, "notes go in an org file inside one of the org directories")
					return
				}
			}
			if _, err := os.Stat(fname); err == nil {
				lines = fileLinesFresh(fname)
			} else {
				lines = []string{"#+TITLE: " + mapTitleOf(mapFile) + " - journal", "#+DND_MAP: " + mapLink(fname, mapFile), ""}
				if err := os.MkdirAll(filepath.Dir(fname), 0755); err != nil {
					dndError(w, http.StatusInternalServerError, "could not make %s: %s", filepath.Dir(fname), err)
					return
				}
			}
			for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
				lines = lines[:len(lines)-1]
			}
			// A blank line between it and what is above, as people write them.
			if len(lines) > 0 {
				lines = append(lines, "")
			}
			at = len(lines)
		}
		if fname == mapFile {
			dndError(w, http.StatusBadRequest, "the map's own file is written again on every save: keep notes in another file")
			return
		}
		lines = splice(lines, at, []string{strings.Repeat("*", level) + " " + title})
		set := placeProps(fname, lines, at, mapFile, req.X, req.Y)
		if req.Colour != nil && strings.TrimSpace(*req.Colour) != "" {
			set["DND_COLOUR"] = strings.TrimSpace(*req.Colour)
		}
		lines, err := setPropsIn(lines, at, noteOwnEnd(lines, at), "", set)
		if err == nil {
			err = writeJournalLines(fname, lines)
		}
		if err != nil {
			dndError(w, http.StatusInternalServerError, "could not write %s: %s", filepath.Base(fname), err)
			return
		}
		var made *journalEntry
		for _, e := range journalIn(fname, fileLinesFresh(fname), mapFile) {
			if e.Line == at {
				e := e
				made = &e
			}
		}
		answerJournal(w, mapFile, "", made)
	default:
		dndError(w, http.StatusBadRequest, "op is place, unplace or new")
	}
}
