//lint:file-ignore ST1006 allow the use of self
package orgs

// The gantt chart's own endpoints.
//
// Everything a gantt client changes - a headline, a date, an effort, a property
// - already has an endpoint, so the only two things missing were a way to *read*
// a schedule with hashes on it (/gantt/tasks, because a picture of a plan cannot
// be edited) and a way to put a new heading into one (/gantt/add, because
// /capture needs a template and a chart knows its parent by hash).
//
// The scheduling rules are not restated here. Which heading comes after which,
// what swim lane it is in and whose it is are read with the mermaid exporter's
// own functions, so the four charts worg draws and the mermaid page the server
// exports can never drift apart.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/app/orgs/plugs"
	"github.com/ihdavids/orgs/internal/app/orgs/plugs/mermaid"
	"github.com/ihdavids/orgs/internal/common"
)

func ganttDateStr(t org.OrgDate) string {
	return fmt.Sprintf("%d-%02d-%02d", t.Start.Year(), t.Start.Month(), t.Start.Day())
}

// Which of a heading's dates the chart is reading, and so which one a client
// dragging the bar should write back to. A heading with neither gets
// "SCHEDULED", because that is what org means by "this is when I will do it".
func ganttDateKind(hash string) string {
	if sec, ok := GetDb().ByHash[hash]; ok && sec != nil && sec.Headline != nil {
		if sec.Headline.Timestamp != nil {
			return "TIMESTAMP"
		}
		if sec.Headline.Scheduled != nil {
			return "SCHEDULED"
		}
	}
	return ""
}

func ganttTaskFrom(d common.ODb, have map[string]*common.Todo, td *common.Todo, implied bool) common.GanttTask {
	t := common.GanttTask{
		Hash: td.Hash, Headline: strings.TrimSpace(td.Headline), Filename: td.Filename,
		LineNum: td.LineNum, Level: td.Level, Parent: td.Parent, Status: td.Status,
		Tags: td.Tags, Props: td.Props, IsActive: td.IsActive, Implied: implied,
	}
	if td.Date != nil && td.Date.TimestampType == org.Active {
		t.Start = ganttDateStr(*td.Date)
		// A plain stamp carries a zero End rather than the same day twice, and
		// "1-01-01" on the wire reads as a date rather than as "no end".
		if !td.Date.End.IsZero() && td.Date.End.After(td.Date.Start) {
			t.End = fmt.Sprintf("%d-%02d-%02d", td.Date.End.Year(), td.Date.End.Month(), td.Date.End.Day())
		}
	}
	t.DateKind = ganttDateKind(td.Hash)

	t.Section = mermaid.GetSection(d, td)
	t.Resource = mermaid.GetResource(d, "", td)

	if e, ok := td.Props["EFFORT"]; ok && e != "" {
		t.Effort = e
		if dur := common.ParseDuration(e); dur != nil {
			t.EffortDays = dur.Days()
		}
	}

	if dep := mermaid.After(have, d, td); dep != nil && dep.Hash != td.Hash {
		t.After = dep.Hash
	}
	t.AfterId = td.Props["AFTER"]
	_, t.Ordered = td.Props["ORDERED"]

	t.Crit = plugs.HasP(td, "CRIT") || td.Status == "BLOCKED"
	t.Active = plugs.HasP(td, "ACT") || td.Status == "IN-PROGRESS" || td.Status == "INPROGRESS"
	t.Done = plugs.HasP(td, "DONE") || td.Status == "DONE" || td.Status == "COMPLETED"
	t.Milestone = plugs.HasP(td, "MILESTONE") || td.Status == "MILESTONE"
	t.Mark = plugs.HasP(td, "MARK") || td.Status == "MARK"

	if !td.IsActive {
		t.Percent = 100
	} else if p, ok := td.Props["PERCENTDONE"]; ok {
		if n, err := strconv.Atoi(strings.TrimSpace(p)); err == nil {
			t.Percent = n
		}
	}
	if c, ok := td.Props["GANTT_COLOR"]; ok && c != "" {
		t.Color = c
	} else if c, ok := td.Props["COLOR"]; ok && c != "" {
		t.Color = c
	}
	if o, ok := td.Props["GANTT_ORDER"]; ok && o != "" {
		if n, err := strconv.ParseFloat(strings.TrimSpace(o), 64); err == nil {
			t.Order = n
			t.HasOrder = true
		}
	}
	return t
}

/* SDOC: API
* GET /gantt/tasks — The Schedule Behind a Gantt Chart

	Every heading a query finds, with the things a gantt chart needs to draw it
	and the hash it needs to change it. Unlike =/file/mermaid=, which answers
	with a rendered diagram, this answers with the plan: what each task is, when
	it starts, how long it is meant to take, what it comes after, and what lane
	it belongs in.

	Dates are *not* laid out. A task carries either a start date or the hash of
	the task it comes after, and the client resolves the chain - the same
	division of labour the mermaid exporter has with mermaid's renderer.

	*Method:* =GET=

	*Query Parameters:*
	| Parameter | Type   | Required | Description                        |
	|-----------+--------+----------+------------------------------------|
	| =query=   | string | yes      | The query expression to schedule.  |

	*Response:* A =GanttData= object: ={"Ok": true, "Query": "...", "Tasks": [...]}=

	A task the query did not find but something it did find comes =after= is
	included with ="Implied": true=, so the chain reads end to end.
EDOC */
func RequestGanttTasks(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("query")
	res := common.GanttData{Ok: false, Query: query, Tasks: []common.GanttTask{}}

	tds, err := db.QueryTodosExpr(query)
	if err != nil {
		res.Msg = fmt.Sprintf("gantt: failed to query expression, %v [%s]", err, query)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(res)
		return
	}

	// `have` is what the query found, and is what the mermaid helpers use to
	// decide whether a dependency needs pulling in behind it.
	have := map[string]*common.Todo{}
	for i := range tds {
		have[tds[i].Hash] = &tds[i]
	}
	seen := map[string]bool{}
	for i := range tds {
		td := &tds[i]
		if seen[td.Hash] {
			continue
		}
		seen[td.Hash] = true
		t := ganttTaskFrom(db, have, td, false)
		res.Tasks = append(res.Tasks, t)
		// Walk the chain back so a task that waits on something outside the
		// query still has something to be drawn after.
		for dep := t.After; dep != "" && !seen[dep]; {
			d := db.FindByHash(dep)
			if d == nil || d.Hash == "" {
				break
			}
			seen[dep] = true
			dt := ganttTaskFrom(db, have, d, true)
			res.Tasks = append(res.Tasks, dt)
			dep = dt.After
		}
	}
	res.Ok = true
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

// "2026-01-05" as org writes a date. A date the client sent that org cannot
// read is written through as-is rather than silently turned into today.
func ganttStamp(day string) string {
	t, err := time.Parse("2006-01-02", strings.TrimSpace(day))
	if err != nil {
		return day
	}
	return t.Format("<2006-01-02 Mon>")
}

// The lines of a new heading: the stars, the keyword and the title, then the
// date under it and a property drawer for what a chart knows about it.
func ganttNewLines(lvl int, a *common.GanttAdd) []string {
	head := strings.Repeat("*", lvl)
	if a.Status != "" {
		head += " " + a.Status
	}
	head += " " + strings.TrimSpace(a.Headline)
	if len(a.Tags) > 0 {
		head += "  :" + strings.Join(a.Tags, ":") + ":"
	}
	lines := []string{head}
	indent := strings.Repeat(" ", lvl+1)
	if a.Start != "" {
		kind := a.DateKind
		if kind == "" {
			kind = "SCHEDULED"
		}
		stamp := ganttStamp(a.Start)
		if kind == "TIMESTAMP" {
			lines = append(lines, indent+stamp)
		} else {
			lines = append(lines, indent+kind+": "+stamp)
		}
	}
	props := [][2]string{}
	if a.Effort != "" {
		props = append(props, [2]string{"EFFORT", a.Effort})
	}
	for k, v := range a.Props {
		if v != "" {
			props = append(props, [2]string{strings.ToUpper(k), v})
		}
	}
	if len(props) > 0 {
		lines = append(lines, indent+":PROPERTIES:")
		for _, p := range props {
			lines = append(lines, indent+":"+p[0]+": "+p[1])
		}
		lines = append(lines, indent+":END:")
	}
	return lines
}

/* SDOC: API
* POST /gantt/add — Add a Task to a Schedule

	Writes a new heading, either under a heading given by hash or at the end of
	a file, with the date, effort and properties a chart needs to draw it. It is
	=/capture= without a template: a gantt client knows where a task goes by the
	bar it was dropped beside, not by a name in the config.

	*Method:* =POST=

	*Request Body:* A =GanttAdd= object.
	| Field        | Type   | Description                                            |
	|--------------+--------+--------------------------------------------------------|
	| =ParentHash= | string | Put the new heading under this one, as its last child.  |
	| =Filename=   | string | Or: put it at the end of this file, at level one.       |
	| =Headline=   | string | The title.                                             |
	| =Status=     | string | Todo keyword, e.g. ="TODO"=.                            |
	| =Start=      | string | ="YYYY-MM-DD"=, written as =DateKind=.                  |
	| =DateKind=   | string | ="SCHEDULED"= (default) or ="TIMESTAMP"=.               |
	| =Effort=     | string | An org duration, e.g. ="3d"=.                           |
	| =Tags=       | list   | Tags for the heading.                                   |
	| =Props=      | map    | Extra properties, written into the drawer.              |

	*Response:* A =ResultMsg=.
EDOC */
func PostGanttAdd(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var args common.GanttAdd
	res := common.ResultMsg{Ok: false}
	if err := json.Unmarshal(body, &args); err != nil {
		res.Msg = "gantt add: could not read request: " + err.Error()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(res)
		return
	}
	if strings.TrimSpace(args.Headline) == "" {
		res.Msg = "gantt add: a task needs a headline"
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(res)
		return
	}

	var filename string
	var row int
	var lvl int
	if args.ParentHash != "" {
		sec, ok := GetDb().ByHash[args.ParentHash]
		if !ok || sec == nil || sec.Headline == nil {
			res.Msg = "gantt add: no heading with that hash"
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(res)
			return
		}
		f := GetDb().FileFromSection(sec)
		if f == nil {
			res.Msg = "gantt add: could not find the file that heading is in"
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(res)
			return
		}
		filename = f.Doc.Path
		lvl = sec.Headline.Lvl + 1
		// The end of the parent's whole subtree, read off the file's own lines
		// rather than from the parsed heading: go-org's end position stops at
		// the planning line for a heading whose body is only a SCHEDULED and a
		// property drawer, and inserting there writes the new task *into* the
		// last child, between its date and its drawer.
		lines, lerr := ganttFileLines(filename)
		if lerr != nil {
			res.Msg = "gantt add: " + lerr.Error()
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(res)
			return
		}
		row = subtreeEndRow(lines, sec.Headline.Pos.Row, sec.Headline.Lvl, sec.Headline.Pos.Row)
	} else if args.Filename != "" {
		target := common.Target{Type: "file", Filename: args.Filename}
		f, _ := GetDb().GetFromTarget(&target, false)
		if f == nil {
			res.Msg = fmt.Sprintf("gantt add: no file [%s]", args.Filename)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(res)
			return
		}
		filename = f.Doc.Path
		lvl = 1
		row = ganttLastRow(filename)
	} else {
		res.Msg = "gantt add: need a ParentHash or a Filename to add to"
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(res)
		return
	}

	if _, err := insertLinesAt(filename, row, ganttNewLines(lvl, &args)); err != nil {
		res.Msg = "gantt add: " + err.Error()
	} else {
		res.Ok = true
		res.Msg = "added"
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

func ganttFileLines(filename string) ([]string, error) {
	b, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	text := strings.TrimSuffix(string(b), "\n")
	return strings.Split(text, "\n"), nil
}

func ganttLastRow(filename string) int {
	b, err := os.ReadFile(filename)
	if err != nil {
		return 0
	}
	n := strings.Count(string(b), "\n")
	if n > 0 {
		return n - 1
	}
	return 0
}
