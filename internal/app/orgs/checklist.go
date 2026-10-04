package orgs

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gorilla/mux"
	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

// What is written under a heading, and the checkboxes in it.
//
// Two things the kanban cards need and nothing else served. `/todohtml/{hash}`
// answers with the body already rendered, which is right for reading and no
// use at all for a checklist: an `- [ ]` that has become `<input disabled>`
// cannot be ticked, and there is no way back from the html to the line it came
// from.
//
// So the body is served as it is written, and a tick is a **line edit** rather
// than a rewrite. `/body/change` exists and would work, but it parses the new
// text and writes the whole document back through go-org - a lot of file to
// put at risk for one character, and every drawer and table in it reformatted
// on the way past. Ticking a box changes one character on one line, and that
// is all this does.

type ChecklistToggle struct {
	Hash  string
	Index int
	// What the client believes that item says, without the box. A tick that
	// lands on the wrong line is worse than one that does not land at all, and
	// an index goes stale the moment the heading is edited anywhere else.
	Text string
	// The state to write. The client says what it wants rather than asking for
	// a flip, so a double click cannot land as two flips and come back where
	// it started.
	Done bool
}

type BodyResult struct {
	Ok   bool
	Msg  string
	Text string
	// The recording and the picture this heading points at, already resolved
	// to something a browser can fetch.
	//
	// Resolved here rather than in the client because the link is written
	// relative to the org file and the file server is rooted at the first org
	// directory - and the client knows neither. It is the same sum the html
	// exporter does for an audio player, and doing it in one place is what
	// stops a card and an exported page disagreeing about where a file is.
	Audio string
	Image string
	// Every picture the heading points at, resolved the same way, in writing
	// order and with `Image` first. A card shows one; a reader looking at the
	// whole heading wants all of them, and which folder each link is relative
	// to is a sum only this end can do.
	Images []string
	// The files this heading owns - its org-attach folder, already listed and
	// resolved. A card that can be dragged a pdf onto has to be able to show
	// that the pdf is there.
	//
	// Listed rather than left to a second request: the card is already asking
	// for the body, and a card per request is what makes a board of forty
	// cards forty requests.
	Attachments []common.Attachment
}

var audioExt = map[string]bool{
	".mp3": true, ".wav": true, ".ogg": true, ".oga": true, ".opus": true,
	".webm": true, ".weba": true, ".m4a": true, ".aac": true, ".flac": true,
}

var imageExt = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true,
	".svg": true, ".avif": true, ".bmp": true,
}

// The target out of whatever shape a link was written in.
var orgLinkRe = regexp.MustCompile(`\[\[([^\]]+)\](?:\[[^\]]*\])?\]`)

func linkTarget(v string) string {
	v = strings.TrimSpace(v)
	if strings.HasPrefix(v, "[[") {
		end := strings.Index(v, "]]")
		if end < 0 {
			return ""
		}
		v = v[2:end]
		if i := strings.Index(v, "]["); i >= 0 {
			v = v[:i]
		}
	}
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "file://")
	v = strings.TrimPrefix(v, "file:")
	return strings.TrimSpace(v)
}

// Where a browser can fetch a file the heading points at, or "" when there is
// nowhere to fetch it from. The file server is rooted at the first org
// directory; anything outside it cannot be served, and is refused rather than
// turned into a url that would answer 404.
func mediaURL(target, fromFile string) string {
	if target == "" {
		return ""
	}
	if strings.Contains(target, "://") || strings.HasPrefix(target, "//") {
		return target
	}
	dirs := Conf().Server.OrgDirs
	if len(dirs) == 0 {
		return ""
	}
	root, err := filepath.Abs(dirs[0])
	if err != nil {
		return ""
	}
	abs := filepath.FromSlash(target)
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(filepath.Dir(fromFile), abs)
		if _, err := os.Stat(abs); err != nil {
			// Plenty of files are written relative to the org root instead.
			if alt := filepath.Join(root, filepath.FromSlash(target)); alt != "" {
				if _, err := os.Stat(alt); err == nil {
					abs = alt
				}
			}
		}
	}
	abs, err = filepath.Abs(abs)
	if err != nil {
		return ""
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil || strings.HasPrefix(rel, "..") {
		return ""
	}
	return "/images/" + filepath.ToSlash(rel)
}

// The first recording the heading points at, and every picture it points at.
// Properties are read first, because that is where a voice note writes its
// audio, and the first picture found is the one a card draws.
//
// One recording rather than all of them because a heading with two recordings in
// it is a heading whose player is ambiguous; pictures stack up without asking
// anything of the reader, so all of them come back.
func mediaIn(sectionProps map[string]string, text, fromFile string) (audio string, images []string) {
	seen := map[string]bool{}
	take := func(target string) {
		if target == "" {
			return
		}
		ext := strings.ToLower(filepath.Ext(strings.SplitN(target, "?", 2)[0]))
		if audio == "" && audioExt[ext] {
			audio = mediaURL(target, fromFile)
		}
		if imageExt[ext] {
			// The same picture linked twice is one picture.
			if url := mediaURL(target, fromFile); url != "" && !seen[url] {
				seen[url] = true
				images = append(images, url)
			}
		}
	}
	for _, v := range sectionProps {
		take(linkTarget(v))
	}
	for _, m := range orgLinkRe.FindAllStringSubmatch(text, -1) {
		take(linkTarget("[[" + m[1] + "]]"))
	}
	return audio, images
}

// An org checklist item: optional indent, a bullet, a box, and the rest.
//
// `-`, `+` and `*` are all bullets, but `*` only at an indent - at column zero
// it is a heading, and `* [ ] something` is a heading whose text happens to
// start with a box. Org also takes `1.` and `1)`, which is why the number form
// is here rather than just the dash everybody writes.
//
// The client matches the same shape and counts the same items, because it
// counts the boxes to number them and this counts them again to find the line.
var checkItemRe = regexp.MustCompile(`^(\s*)([-+*]|\d+[.)])\s+\[([ xX-])\]\s?(.*)$`)

func checkItem(line string) []string {
	m := checkItemRe.FindStringSubmatch(line)
	if m == nil {
		return nil
	}
	if m[2] == "*" && m[1] == "" {
		return nil
	}
	return m
}

// The lines of one heading's own body: everything under the headline, stopping
// at the first heading at the same level or above. Child headings are not part
// of it - a card shows what is written on its own heading.
func headingBodyLines(hash string) (filename string, lines []string, from int, to int, err error) {
	// FindByHash rather than the raw map: the registries are filled in lazily,
	// so on a server that has not answered a query yet the map is empty - and
	// after a file has been written the entries for it have been dropped, which
	// is what stops this handing back a heading three lines from where it is.
	sec := GetDb().FindByHash(hash)
	if sec == nil || sec.Headline == nil {
		return "", nil, 0, 0, fmt.Errorf("no heading with that hash")
	}
	f := GetDb().FileFromSection(sec)
	if f == nil || f.Doc == nil {
		return "", nil, 0, 0, fmt.Errorf("that heading has no file")
	}
	filename = f.Doc.Path
	b, rerr := os.ReadFile(filename)
	if rerr != nil {
		return "", nil, 0, 0, rerr
	}
	lines = strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")

	start := sec.Headline.Pos.Row
	if start < 0 || start >= len(lines) {
		return "", nil, 0, 0, fmt.Errorf("that heading is not where the database says")
	}
	end := subtreeEndRow(lines, start, sec.Headline.Lvl, start)
	// Only this heading's own body: stop at the first child heading.
	for i := start + 1; i <= end && i < len(lines); i++ {
		stars := 0
		for stars < len(lines[i]) && lines[i][stars] == '*' {
			stars++
		}
		if stars > 0 && stars < len(lines[i]) && lines[i][stars] == ' ' {
			end = i - 1
			break
		}
	}
	return filename, lines, start + 1, end, nil
}

// The body as it is written, with the property drawer and the planning line
// left in. The client knows what to ignore and would rather have the file's
// own text than a version somebody has already decided things about.
func RequestTodoBody(w http.ResponseWriter, r *http.Request) {
	var res BodyResult
	vars := mux.Vars(r)
	h, err := GetHash(vars, "hash")
	if err != nil {
		res.Msg = err.Error()
		bodyJson(w, res)
		return
	}
	_, lines, from, to, err := headingBodyLines(string(h))
	if err != nil {
		res.Msg = err.Error()
		bodyJson(w, res)
		return
	}
	if from > to || from >= len(lines) {
		res.Ok = true
		bodyJson(w, res)
		return
	}
	if to >= len(lines) {
		to = len(lines) - 1
	}
	res.Ok = true
	res.Text = strings.Join(lines[from:to+1], "\n")

	props := map[string]string{}
	if sec := GetDb().FindByHash(string(h)); sec != nil && sec != nil && sec.Headline != nil &&
		sec.Headline.Properties != nil {
		for _, p := range sec.Headline.Properties.Properties {
			if len(p) >= 2 {
				props[p[0]] = p[1]
			}
		}
	}
	filename := ""
	if f, ok := GetDb().ByHashToFile[string(h)]; ok && f != nil && f.Doc != nil {
		filename = f.Doc.Path
	}
	res.Audio, res.Images = mediaIn(props, res.Text, filename)
	// Image is what a card reads, and is the first of them.
	if len(res.Images) > 0 {
		res.Image = res.Images[0]
	}
	// What the heading owns, and - when it owns a picture and points at no
	// other - the picture the card draws. Attaching a screenshot to a card and
	// having the card not show it would be a surprise.
	if sec := GetDb().FindByHash(string(h)); sec != nil {
		dir, from, _ := attachDirOf(sec, GetDb().FileFromSection(sec))
		if dir != "" && from != "" {
			res.Attachments = attachmentsIn(dir, string(h))
			for _, a := range res.Attachments {
				if a.Media == "image" {
					if res.Image == "" {
						res.Image = a.Url
					}
					res.Images = append(res.Images, a.Url)
				}
				if a.Media == "audio" && res.Audio == "" {
					res.Audio = a.Url
				}
			}
		}
	}
	if res.Attachments == nil {
		res.Attachments = []common.Attachment{}
	}
	bodyJson(w, res)
}

// Tick or untick one box.
//
// The item is found by counting checkbox lines in the heading's own body, and
// then checked against the text the client was looking at. Both have to agree:
// the count alone goes stale the moment a line is added above, and the text
// alone cannot tell two identical items apart.
func PostChecklistToggle(w http.ResponseWriter, r *http.Request) {
	var res common.ResultMsg
	body, _ := io.ReadAll(r.Body)
	var args ChecklistToggle
	if err := json.Unmarshal(body, &args); err != nil {
		res.Msg = err.Error()
		checklistJson(w, res)
		return
	}

	filename, lines, from, to, err := headingBodyLines(args.Hash)
	if err != nil {
		res.Msg = err.Error()
		checklistJson(w, res)
		return
	}

	seen := 0
	row := -1
	var m []string
	for i := from; i <= to && i < len(lines); i++ {
		hit := checkItem(lines[i])
		if hit == nil {
			continue
		}
		if seen == args.Index {
			row = i
			m = hit
			break
		}
		seen++
	}
	if row < 0 {
		res.Msg = "that heading has no checklist item there any more"
		checklistJson(w, res)
		return
	}
	if strings.TrimSpace(m[4]) != strings.TrimSpace(args.Text) {
		res.Msg = "that item has changed since it was read; the board will refresh"
		checklistJson(w, res)
		return
	}

	box := " "
	if args.Done {
		box = "X"
	}
	lines[row] = fmt.Sprintf("%s%s [%s] %s", m[1], m[2], box, m[4])

	if err := os.WriteFile(filename, []byte(strings.Join(lines, "\n")+"\n"), 0644); err != nil {
		res.Msg = err.Error()
		checklistJson(w, res)
		return
	}
	res.Ok = true
	res.Msg = "ok"
	checklistJson(w, res)
}

func bodyJson(w http.ResponseWriter, res BodyResult) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

func checklistJson(w http.ResponseWriter, res common.ResultMsg) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

// ---------------------------------------------------------------------------
// The query language's side of a checklist
// ---------------------------------------------------------------------------

// HasChecklist reports whether a heading's own body holds checkboxes, and
// ChecklistCounts says how many of them are ticked. They are here rather than
// in todo.go because they have to count boxes exactly the way the *write* counts
// them - `orgs check 3` and `HasChecklist()` disagreeing about what a box is
// would put a tick on the wrong line.
//
// Counted off the file's own lines rather than out of the parse tree, for the
// same reason the toggle is a line edit: what org calls a list item and what
// somebody looking at the file calls a checkbox are not quite the same set, and
// the file is the thing being written to.
func HasChecklist(p *org.Section, f *common.OrgFile) bool {
	_, total := ChecklistCounts(p, f)
	return total > 0
}

// ChecklistCounts is how many boxes are ticked and how many there are. A
// heading with no boxes answers 0, 0 - which is what lets `ChecklistDone()` be
// false for one rather than vacuously true.
func ChecklistCounts(p *org.Section, f *common.OrgFile) (done int, total int) {
	if p == nil || p.Headline == nil || f == nil || f.Doc == nil {
		return 0, 0
	}
	b, err := os.ReadFile(f.Doc.Path)
	if err != nil {
		return 0, 0
	}
	lines := strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
	start := p.Headline.Pos.Row
	if start < 0 || start >= len(lines) {
		return 0, 0
	}
	end := subtreeEndRow(lines, start, p.Headline.Lvl, start)
	for i := start + 1; i <= end && i < len(lines); i++ {
		// This heading's own body, so stop at the first child heading - the
		// same boundary headingBodyLines draws.
		stars := 0
		for stars < len(lines[i]) && lines[i][stars] == '*' {
			stars++
		}
		if stars > 0 && stars < len(lines[i]) && lines[i][stars] == ' ' {
			break
		}
		m := checkItem(lines[i])
		if m == nil {
			continue
		}
		total++
		if m[3] != " " {
			done++
		}
	}
	return done, total
}
