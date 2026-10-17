package orgs

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/ihdavids/orgs/internal/common"
	"github.com/ihdavids/orgs/internal/common/dnd/randgen"
)

// The table's random generator (internal/common/dnd/randgen) and the one
// thing it needs of the server beyond the ruleset: filing what it made.

/* SDOC: API
* GET /dnd/generate — List the Random Generators
	The generators the random generator offers (encounters, traps, treasure, magic items,
	spellbooks, NPCs, taverns, trinkets and so on), each with the options its form takes.
	A race option's choices are the ruleset's races.

	*Method:* =GET=

	*Query Parameters:*
	| Parameter | Type   | Required | Description                                  |
	|-----------+--------+----------+----------------------------------------------|
	| =ruleset= | string | no       | Ruleset id; the default ruleset when absent. |

	*Response:* ={ok, generators: [{id, name, group, summary, count, fields: [{id, label, kind, choices, default}]}]}=
	EDOC */
func RequestDndGenerators(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	rs := dndRuleset(r.URL.Query().Get("ruleset"))
	dndJson(w, map[string]interface{}{"ok": true, "generators": randgen.Generators(rs)})
}

/* SDOC: API
* POST /dnd/generate — Run a Random Generator
	Makes =count= results of one generator. Each result is a title, a one line summary and
	an org body (tables and lists, never a heading), so what is shown is what gets saved.
	The same =seed= gives the same results; a request without one is answered with the
	seed it was given.

	*Method:* =POST=

	*Request Body:* ={type, options: {name: value}, count, seed, ruleset}=

	*Response:* ={ok, type, seed, items: [{kind, title, summary, body, tags, props}]}=
	EDOC */
func PostDndGenerate(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	var req randgen.Request
	if !dndBody(w, r, &req) {
		return
	}
	res, err := randgen.Generate(dndRuleset(req.Ruleset), req)
	if err != nil {
		dndError(w, http.StatusBadRequest, "%s", err)
		return
	}
	dndJson(w, res)
}

type dndGenSaveRequest struct {
	// Where to file them, as /refile/targets answers: a heading by hash, or
	// a file (the end of it).
	Target common.Target  `json:"target"`
	Items  []randgen.Item `json:"items"`
}

/* SDOC: API
* POST /dnd/generate/save — File Generated Results
	Writes generated results into an org file, one heading each, with the result's tags
	and properties and its body underneath. A heading target files them as its last
	children; a file target puts them at the end of the file as top level headings. All
	of them go in one write, so the target's position cannot go stale between two.

	*Method:* =POST=

	*Request Body:* ={target: {Type: "hash"|"file", Id, Filename}, items: [...]}=, items as
	=POST /dnd/generate= answers them.

	*Response:* ={Ok, Msg}=
	EDOC */
func PostDndGenerateSave(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	var req dndGenSaveRequest
	if !dndBody(w, r, &req) {
		return
	}
	if len(req.Items) == 0 {
		dndError(w, http.StatusBadRequest, "nothing to save")
		return
	}
	filename, row, lvl, err := dndGenTarget(&req.Target)
	if err != nil {
		dndError(w, http.StatusBadRequest, "%s", err)
		return
	}
	add := []string{}
	for i := range req.Items {
		add = append(add, dndGenLines(lvl, &req.Items[i])...)
	}
	if _, err := insertLinesAt(filename, row, add); err != nil {
		dndError(w, http.StatusInternalServerError, "%s", err)
		return
	}
	what := "1 result"
	if len(req.Items) > 1 {
		what = fmt.Sprintf("%d results", len(req.Items))
	}
	dndJson(w, common.ResultMsg{Ok: true, Msg: fmt.Sprintf("Saved %s to %s", what, baseOf(filename))})
}

// dndGenTarget finds the file, the row to insert after and the level of the
// new headings. The row is read off the file's own lines, as the gantt's add
// does, because go-org's end position for a heading whose body is a planning
// line and a drawer stops short of its last child.
func dndGenTarget(t *common.Target) (string, int, int, error) {
	db := GetDb()
	switch t.Type {
	case "hash", "":
		sec := db.FindByHash(t.Id)
		if sec == nil || sec.Headline == nil {
			return "", 0, 0, fmt.Errorf("no heading with that hash; it may have moved")
		}
		f := db.FileFromSection(sec)
		if f == nil {
			return "", 0, 0, fmt.Errorf("could not find the file that heading is in")
		}
		lines, err := ganttFileLines(f.Doc.Path)
		if err != nil {
			return "", 0, 0, err
		}
		row := subtreeEndRow(lines, sec.Headline.Pos.Row, sec.Headline.Lvl, sec.Headline.Pos.Row)
		return f.Doc.Path, row, sec.Headline.Lvl + 1, nil
	case "file":
		f, _ := db.GetFromTarget(t, false)
		if f == nil {
			return "", 0, 0, fmt.Errorf("no file [%s]", t.Filename)
		}
		lines, err := ganttFileLines(f.Doc.Path)
		if err != nil {
			return "", 0, 0, err
		}
		return f.Doc.Path, len(lines) - 1, 1, nil
	}
	return "", 0, 0, fmt.Errorf("cannot save to a %q target", t.Type)
}

func dndGenLines(lvl int, it *randgen.Item) []string {
	title := strings.Join(strings.Fields(it.Title), " ")
	if title == "" {
		title = "Untitled"
	}
	head := strings.Repeat("*", lvl) + " " + title
	if t := captureTags(dndGenTags(it.Tags)); t != "" {
		head += "  " + t
	}
	props := map[string]string{"GENERATOR": it.Kind}
	for k, v := range it.Props {
		if strings.TrimSpace(v) != "" {
			props[strings.ToUpper(k)] = v
		}
	}
	body := strings.TrimRight(it.Body, "\n")
	if it.Summary != "" {
		if body != "" {
			body = it.Summary + "\n\n" + body
		} else {
			body = it.Summary
		}
	}
	content := withProperties(body, props)
	lines := []string{head}
	for _, l := range strings.Split(strings.TrimRight(indentEachLine(content, strings.Repeat(" ", lvl+1)), "\n"), "\n") {
		lines = append(lines, l)
	}
	return lines
}

// A tag is a word: org will not read one with a space or a colon in it.
func dndGenTags(tags []string) []string {
	out := []string{}
	for _, t := range tags {
		t = strings.Map(func(r rune) rune {
			if r == ' ' || r == ':' || r == '\t' {
				return '_'
			}
			return r
		}, strings.TrimSpace(t))
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

func baseOf(path string) string {
	if i := strings.LastIndexAny(path, `/\`); i >= 0 {
		return path[i+1:]
	}
	return path
}
