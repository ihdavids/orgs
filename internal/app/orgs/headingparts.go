//lint:file-ignore ST1006 allow the use of self
package orgs

import (
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/ihdavids/orgs/internal/common"
)

// Setting a heading's tags or priority: the parts of its headline line other
// than the title. A line edit of that one line (see the root CLAUDE.md, Traps:
// line edits) - the old tag toggle wrote the whole document back out, which
// re-indents every drawer in the file to change one tag.

var (
	headlineTagsRe = regexp.MustCompile(`[ \t]+(:[^\s]+:)[ \t]*$`)
	headlinePrioRe = regexp.MustCompile(`^\[#([A-Za-z0-9])\][ \t]*`)
	headlineHeadRe = regexp.MustCompile(`^(\*+)[ \t]+`)
	priorityRe     = regexp.MustCompile(`^[A-Za-z0-9]$`)
)

// HeadingParts is the request: the heading, and the parts to set. A nil
// field is left as it is.
type HeadingParts struct {
	Hash     string
	Tags     *[]string
	Priority *string
}

// The headline line with new tags and/or a new priority. status is the
// heading's todo keyword as parsed, so a first word that is the keyword is
// kept as the keyword rather than read as title. Tags that were right-aligned
// stay ending at the column they ended at.
func setHeadlineParts(line, status string, prio *string, tags *[]string) string {
	m := headlineHeadRe.FindStringSubmatch(line)
	if m == nil {
		return line
	}
	stars := m[1]
	rest := line[len(m[0]):]

	oldEnd := 0
	gap := 2
	if loc := headlineTagsRe.FindStringSubmatchIndex(rest); loc != nil {
		// The space before the tags says how they were laid out: a run of
		// it is org's tags column (right-aligned, kept ending where it ended),
		// a space or two is just a space or two.
		gap = loc[2] - loc[0]
		if gap > 2 {
			oldEnd = len(strings.TrimRight(line, " \t"))
		}
		if tags == nil {
			// Keep the tags exactly as written.
			t := rest[loc[2]:loc[3]]
			ts := strings.Split(strings.Trim(t, ":"), ":")
			tags = &ts
		}
		rest = rest[:loc[0]]
	}
	kw := ""
	if status != "" && (rest == status || strings.HasPrefix(rest, status+" ") || strings.HasPrefix(rest, status+"\t")) {
		kw = status
		rest = strings.TrimLeft(rest[len(status):], " \t")
	}
	p := ""
	if pm := headlinePrioRe.FindStringSubmatch(rest); pm != nil {
		p = pm[1]
		rest = rest[len(pm[0]):]
	}
	if prio != nil {
		p = strings.ToUpper(strings.TrimSpace(*prio))
	}
	title := strings.TrimRight(rest, " \t")

	out := stars
	if kw != "" {
		out += " " + kw
	}
	if p != "" {
		out += " [#" + p + "]"
	}
	if title != "" {
		out += " " + title
	}
	var clean []string
	if tags != nil {
		for _, t := range *tags {
			t = strings.Trim(strings.TrimSpace(t), ":")
			if t != "" && !strings.ContainsAny(t, " \t:") {
				clean = append(clean, t)
			}
		}
	}
	if len(clean) > 0 {
		tagText := ":" + strings.Join(clean, ":") + ":"
		pad := gap
		if oldEnd > 0 {
			// Right-align to where the tags used to end, the way org's own
			// tags column keeps them; never closer than one space.
			pad = 1
			if n := oldEnd - len(out) - len(tagText); n > 1 {
				pad = n
			}
		}
		out += strings.Repeat(" ", pad) + tagText
	}
	return out
}

/* SDOC: API
* POST /heading/parts — Set a Heading's Tags or Priority

	Rewrites the heading's headline line with new tags, a new priority, or
	both, keeping the stars, keyword and title as written and right-aligned
	tags at the column they ended at. Only that one line changes.

	*Method:* =POST=

	*Request Body:* ={Hash, Tags, Priority}=. =Tags= is the whole list (empty
	takes them all off); =Priority= is one letter, or empty to take it off.
	A field left out is left as it is.

	*Response:* A =ResultMsg=.
	EDOC */
func PostHeadingParts(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	res := common.ResultMsg{Ok: false}
	body, _ := io.ReadAll(r.Body)
	var req HeadingParts
	if err := json.Unmarshal(body, &req); err != nil {
		res.Msg = "heading parts: could not read request: " + err.Error()
		json.NewEncoder(w).Encode(res)
		return
	}
	if req.Priority != nil && *req.Priority != "" && !priorityRe.MatchString(strings.TrimSpace(*req.Priority)) {
		res.Msg = "a priority is one letter or digit, e.g. A"
		json.NewEncoder(w).Encode(res)
		return
	}
	sec := GetDb().FindByHash(req.Hash)
	if sec == nil || sec.Headline == nil {
		res.Msg = "no heading with that hash - it may have moved; refresh and try again"
		json.NewEncoder(w).Encode(res)
		return
	}
	f := GetDb().FileFromSection(sec)
	if f == nil {
		res.Msg = "could not find the file that heading is in"
		json.NewEncoder(w).Encode(res)
		return
	}
	lines, from, _, ok := recordLines(f.Doc.Path, sec)
	if !ok {
		res.Msg = "could not read " + f.Doc.Path
		json.NewEncoder(w).Encode(res)
		return
	}
	lines[from] = setHeadlineParts(lines[from], sec.Headline.Status, req.Priority, req.Tags)
	if err := writeLines(f.Doc.Path, lines); err != nil {
		res.Msg = err.Error()
		json.NewEncoder(w).Encode(res)
		return
	}
	res.Ok, res.Msg = true, "updated"
	json.NewEncoder(w).Encode(res)
}
