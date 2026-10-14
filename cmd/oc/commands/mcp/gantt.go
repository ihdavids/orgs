package mcp

// The gantt tools: building a plan out of a pile of tickets.
//
// The workflow they are described for is gantt_tasks (what the chart holds)
// -> gantt_suggest (what the tickets' own words and past work suggest) ->
// showing the suggestions to the person -> gantt_apply_suggestions, or
// gantt_link / gantt_set_effort one at a time for the ones they picked.
//
// gantt_apply_suggestions writes many headings in a row. A hash is made from
// a heading's title and those before it in its file, so a property write does
// not move one - but a heading renamed by anyone else between two writes
// would, and so would anything that changes a title. Rather than trust hashes
// fetched before the first write, every write looks its headings up again in a
// fresh /gantt/tasks, by ID or CUSTOM_ID when they have one and otherwise by
// file, level, headline and which of its twins it is; a heading that cannot be
// found again is skipped and said so, never written by a stale hash.

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

func init() {
	allTools = append(allTools, ganttTools...)
}

func ganttFetch(core *commands.Core, query string) (common.GanttData, error) {
	d, err := commands.SendReceiveGetErr[common.GanttData](core, "gantt/tasks", map[string]string{"query": query})
	if err != nil {
		return d, err
	}
	if !d.Ok {
		return d, fmt.Errorf("%s", d.Msg)
	}
	return d, nil
}

// /gantt/suggest is a POST that writes nothing, so it goes straight to the
// rest client: SendReceivePostErr would refuse it under -dry-run.
func ganttSuggestFetch(core *commands.Core, req common.GanttSuggestReq) (common.GanttSuggest, error) {
	s, err := common.RestPost[common.GanttSuggest](&core.Rest, "gantt/suggest", &req)
	if err != nil {
		return s, err
	}
	if !s.Ok {
		return s, fmt.Errorf("%s", s.Msg)
	}
	return s, nil
}

func ganttFloat(args map[string]interface{}, key string, def float64) float64 {
	switch v := args[key].(type) {
	case float64:
		return v
	case string:
		if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			return f
		}
	}
	return def
}

func ganttKinds(args map[string]interface{}) []string {
	out := []string{}
	switch v := args["kinds"].(type) {
	case []interface{}:
		for _, k := range v {
			if s, ok := k.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
	case string:
		for _, k := range strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == ' ' }) {
			out = append(out, k)
		}
	}
	return out
}

// One task as a line a model can read and act on.
func ganttRow(t common.GanttTask, titles map[string]string) string {
	b := strings.Builder{}
	b.WriteString(t.Hash + "  ")
	if t.Status != "" {
		b.WriteString(t.Status + " ")
	}
	b.WriteString(strconv.Quote(t.Headline))
	parts := []string{}
	if t.Start != "" {
		s := "start " + t.Start
		if t.End != "" {
			s += " to " + t.End
		}
		parts = append(parts, s)
	}
	if t.Effort != "" {
		parts = append(parts, "effort "+t.Effort)
	} else {
		parts = append(parts, "no effort")
	}
	if t.After != "" {
		how := "AFTER"
		if t.AfterId == "" {
			how = "ORDERED"
		}
		parts = append(parts, fmt.Sprintf("after %s %q (%s)", t.After, titles[t.After], how))
	}
	if t.Resource != "" {
		parts = append(parts, "who "+t.Resource)
	}
	if t.Section != "" {
		parts = append(parts, "lane "+t.Section)
	}
	if t.Percent > 0 {
		parts = append(parts, fmt.Sprintf("%d%%", t.Percent))
	}
	if t.Implied {
		parts = append(parts, "implied (not found by the query; something found waits on it)")
	}
	parts = append(parts, fmt.Sprintf("%s:%d", filepath.Base(t.Filename), t.LineNum+1))
	b.WriteString("  | " + strings.Join(parts, " | "))
	return b.String()
}

func ganttTitles(d common.GanttData) map[string]string {
	m := map[string]string{}
	for _, t := range d.Tasks {
		m[t.Hash] = t.Headline
	}
	return m
}

func capNote(shown, total int) string {
	if shown < total {
		return fmt.Sprintf("  (%d more not shown; raise limit to see them)\n", total-shown)
	}
	return ""
}

// The suggestions as text, every heading named with its title and hash.
func renderSuggest(s common.GanttSuggest, titles map[string]string, limit int) string {
	if limit <= 0 {
		limit = 50
	}
	name := func(h string) string {
		if t, ok := titles[h]; ok {
			return fmt.Sprintf("%q [%s]", t, h)
		}
		return "[" + h + "]"
	}
	b := strings.Builder{}
	section := func(title string, n int) bool {
		if n == 0 {
			return false
		}
		b.WriteString(fmt.Sprintf("%s (%d):\n", title, n))
		return true
	}
	if section("Dependencies suggested, most certain first (Task waits on After)", len(s.Deps)) {
		for i, d := range s.Deps {
			if i >= limit {
				break
			}
			b.WriteString(fmt.Sprintf("- %.2f %s: %s waits on %s\n    evidence: %s\n", d.Confidence, d.Source, name(d.Task), name(d.After), d.Evidence))
		}
		b.WriteString(capNote(min(limit, len(s.Deps)), len(s.Deps)))
	}
	if section("Redundant links (the chart keeps the wait without them)", len(s.Redundant)) {
		for i, r := range s.Redundant {
			if i >= limit {
				break
			}
			via := "the ORDERED chain it falls back to says the same"
			if len(r.Via) > 0 {
				vs := []string{}
				for _, v := range r.Via {
					vs = append(vs, name(v))
				}
				via = "already implied through " + strings.Join(vs, " -> ")
			}
			rm := ""
			if r.Removable {
				rm = " (remove with gantt_link remove=true)"
			}
			b.WriteString(fmt.Sprintf("- %s AFTER %s: %s%s\n", name(r.Task), name(r.After), via, rm))
		}
		b.WriteString(capNote(min(limit, len(s.Redundant)), len(s.Redundant)))
	}
	if section("Existing cycles (none of these can ever start)", len(s.Cycles)) {
		for _, c := range s.Cycles {
			vs := []string{}
			for _, v := range c {
				vs = append(vs, name(v))
			}
			b.WriteString("- " + strings.Join(vs, " waits on ") + " waits on the first\n")
		}
	}
	if section("Estimates for open tasks with no EFFORT", len(s.Estimates)) {
		for i, e := range s.Estimates {
			if i >= limit {
				break
			}
			b.WriteString(fmt.Sprintf("- %s: %s (confidence %.2f) - %s\n", name(e.Task), e.Effort, e.Confidence, e.Basis))
		}
		b.WriteString(capNote(min(limit, len(s.Estimates)), len(s.Estimates)))
	}
	if section("Ways to split the chart into lanes, best first", len(s.Lanes)) {
		for _, l := range s.Lanes {
			key := "by=" + l.By
			if l.Prop != "" {
				key += " prop=" + l.Prop
			}
			gs := []string{}
			for _, g := range l.Groups {
				gs = append(gs, fmt.Sprintf("%s (%d)", g.Name, len(g.Tasks)))
			}
			b.WriteString(fmt.Sprintf("- %.2f %s [%s]: %s; %d dependency link(s) cross lanes\n", l.Score, l.Label, key, strings.Join(gs, ", "), l.CrossLinks))
		}
	}
	if len(s.Ratios) > 0 {
		r := append([]float64(nil), s.Ratios...)
		sort.Float64s(r)
		b.WriteString(fmt.Sprintf("Past estimates: %d finished task(s) with EFFORT and clocked time; actual/estimate median %.2f (range %.2f to %.2f).\n",
			len(r), r[len(r)/2], r[0], r[len(r)-1]))
	}
	if b.Len() == 0 {
		return "(nothing to suggest: no dependencies in the text, no open task without an estimate, no useful lanes)"
	}
	return b.String()
}

// ---------------------------------------------------------------------------
// Finding a heading again between writes
// ---------------------------------------------------------------------------

type ganttKey struct {
	ID    string // "id:..." or "cid:...", when it has one
	Pos   string // file, level, headline and which twin
	Title string
}

func ganttKeysOf(d common.GanttData) map[string]ganttKey {
	out := map[string]ganttKey{}
	twins := map[string]int{}
	for _, t := range d.Tasks {
		pos := fmt.Sprintf("%s\x00%d\x00%s", t.Filename, t.Level, strings.TrimSpace(t.Headline))
		twins[pos]++
		k := ganttKey{Pos: fmt.Sprintf("%s\x00%d", pos, twins[pos]), Title: t.Headline}
		if v := strings.TrimSpace(t.Props["ID"]); v != "" {
			k.ID = "id:" + v
		} else if v := strings.TrimSpace(t.Props["CUSTOM_ID"]); v != "" {
			k.ID = "cid:" + v
		}
		out[t.Hash] = k
	}
	return out
}

// The current hash of a heading, by the key it had when the plan was made.
func ganttResolve(d common.GanttData, k ganttKey) (common.GanttTask, bool) {
	keys := ganttKeysOf(d)
	if k.ID != "" {
		for _, t := range d.Tasks {
			id := ""
			if v := strings.TrimSpace(t.Props["ID"]); v != "" {
				id = "id:" + v
			}
			cid := ""
			if v := strings.TrimSpace(t.Props["CUSTOM_ID"]); v != "" {
				cid = "cid:" + v
			}
			if k.ID == id || k.ID == cid {
				return t, true
			}
		}
	}
	for _, t := range d.Tasks {
		if keys[t.Hash].Pos == k.Pos {
			return t, true
		}
	}
	return common.GanttTask{}, false
}

type ganttOp struct {
	kind  string // link, effort, lane
	task  ganttKey
	after ganttKey
	value string
	prop  string
	label string
}

func applySuggestions(core *commands.Core, args map[string]interface{}) (interface{}, error) {
	q, err := need(args, "query")
	if err != nil {
		return nil, err
	}
	doDeps, doEst := yes(args, "deps"), yes(args, "estimates")
	laneBy, laneProp := "", ""
	if l, ok := args["lane"].(map[string]interface{}); ok {
		laneBy, laneProp = strings.ToLower(str(l, "by")), strings.ToUpper(str(l, "prop"))
	}
	laneProperty := strings.ToUpper(str(args, "laneProperty"))
	if laneProperty == "" {
		laneProperty = "SECTION"
	}
	if !doDeps && !doEst && laneBy == "" {
		return nil, fmt.Errorf("nothing asked for: set deps, estimates, or lane")
	}
	minConf := ganttFloat(args, "minConfidence", 0.5)

	kinds := []string{}
	if doDeps {
		kinds = append(kinds, "deps")
	}
	if doEst {
		kinds = append(kinds, "estimates")
	}
	if laneBy != "" {
		kinds = append(kinds, "lanes")
	}
	sug, err := ganttSuggestFetch(core, common.GanttSuggestReq{
		Query: q, Kinds: kinds, DayHours: ganttFloat(args, "dayHours", 0), From: str(args, "from"),
	})
	if err != nil {
		return nil, err
	}
	data, err := ganttFetch(core, q)
	if err != nil {
		return nil, err
	}
	keys := ganttKeysOf(data)
	byHash := map[string]common.GanttTask{}
	for _, t := range data.Tasks {
		byHash[t.Hash] = t
	}

	// The plan, made once from one read: deps, then efforts, then lanes.
	ops := []ganttOp{}
	skipped := []string{}
	if doDeps {
		for _, d := range sug.Deps {
			if d.Confidence < minConf {
				skipped = append(skipped, fmt.Sprintf("below minConfidence %.2f: %q waits on %q (%.2f %s)", minConf, keys[d.Task].Title, keys[d.After].Title, d.Confidence, d.Source))
				continue
			}
			ops = append(ops, ganttOp{kind: "link", task: keys[d.Task], after: keys[d.After],
				label: fmt.Sprintf("%q waits on %q (%.2f %s)", keys[d.Task].Title, keys[d.After].Title, d.Confidence, d.Source)})
		}
	}
	if doEst {
		for _, e := range sug.Estimates {
			if e.Confidence < minConf {
				skipped = append(skipped, fmt.Sprintf("below minConfidence %.2f: EFFORT %s on %q (%.2f)", minConf, e.Effort, keys[e.Task].Title, e.Confidence))
				continue
			}
			ops = append(ops, ganttOp{kind: "effort", task: keys[e.Task], value: e.Effort,
				label: fmt.Sprintf("EFFORT %s on %q (%s)", e.Effort, keys[e.Task].Title, e.Basis)})
		}
	}
	if laneBy != "" {
		var lane *common.GanttLaneSuggestion
		avail := []string{}
		for i, l := range sug.Lanes {
			avail = append(avail, strings.TrimSpace("by="+l.By+" "+map[bool]string{true: "prop=" + l.Prop, false: ""}[l.Prop != ""]))
			if l.By == laneBy && (laneBy != "property" || strings.EqualFold(l.Prop, laneProp)) {
				lane = &sug.Lanes[i]
			}
		}
		if lane == nil {
			return nil, fmt.Errorf("no lane suggestion by=%s prop=%s for this query; gantt_suggest offers: %s", laneBy, laneProp, strings.Join(avail, "; "))
		}
		if lane.By == "property" && strings.EqualFold(lane.Prop, laneProperty) {
			skipped = append(skipped, fmt.Sprintf("lanes: the tasks already carry %s; nothing to write", laneProperty))
		} else {
			for _, g := range lane.Groups {
				if g.Name == "(none)" {
					continue
				}
				for _, h := range g.Tasks {
					if strings.TrimSpace(byHash[h].Props[laneProperty]) == g.Name {
						continue
					}
					ops = append(ops, ganttOp{kind: "lane", task: keys[h], prop: laneProperty, value: g.Name,
						label: fmt.Sprintf("%s %s on %q", laneProperty, g.Name, keys[h].Title)})
				}
			}
		}
	}

	b := strings.Builder{}
	if commands.DryRun {
		b.WriteString(fmt.Sprintf("-dry-run: nothing was written. These %d write(s) would be made:\n", len(ops)))
		for _, op := range ops {
			b.WriteString("- " + op.label + "\n")
		}
		for _, s := range skipped {
			b.WriteString("- not applied, " + s + "\n")
		}
		return b.String(), nil
	}

	done, failed := []string{}, []string{}
	current := data
	for i, op := range ops {
		if i > 0 {
			// Found again after every write: see the top of this file.
			if current, err = ganttFetch(core, q); err != nil {
				failed = append(failed, "stopped: could not read the chart again: "+err.Error())
				break
			}
		}
		t, ok := ganttResolve(current, op.task)
		if !ok {
			skipped = append(skipped, "could not find "+strconv.Quote(op.task.Title)+" again; "+op.label)
			continue
		}
		switch op.kind {
		case "link":
			a, ok := ganttResolve(current, op.after)
			if !ok {
				skipped = append(skipped, "could not find "+strconv.Quote(op.after.Title)+" again; "+op.label)
				continue
			}
			res, err := commands.SendReceivePostErr[common.GanttLinkReq, common.ResultMsg](core, "gantt/link", &common.GanttLinkReq{Task: t.Hash, After: a.Hash})
			if err != nil {
				failed = append(failed, op.label+": "+err.Error())
			} else if !res.Ok {
				failed = append(failed, op.label+": "+res.Msg)
			} else {
				done = append(done, res.Msg)
			}
		case "effort", "lane":
			name := "EFFORT"
			if op.kind == "lane" {
				name = op.prop
			}
			if op.kind == "effort" && strings.TrimSpace(t.Effort) != "" {
				skipped = append(skipped, fmt.Sprintf("%q has an EFFORT (%s) now; left alone", t.Headline, t.Effort))
				continue
			}
			res, err := commands.SendReceivePostErr[common.TodoPropertyChange, common.Result](core, "property",
				&common.TodoPropertyChange{Hash: t.Hash, Name: name, Value: op.value})
			if err != nil {
				failed = append(failed, op.label+": "+err.Error())
			} else if !res.Ok {
				failed = append(failed, op.label+": "+res.Msg)
			} else {
				done = append(done, "wrote "+op.label)
			}
		}
	}
	b.WriteString(fmt.Sprintf("Wrote %d, failed %d, not applied %d.\n", len(done), len(failed), len(skipped)))
	for _, s := range done {
		b.WriteString("- " + s + "\n")
	}
	for _, s := range failed {
		b.WriteString("- FAILED " + s + "\n")
	}
	for _, s := range skipped {
		b.WriteString("- not applied, " + s + "\n")
	}
	if len(done) == 0 && len(failed) == 0 && len(skipped) == 0 {
		b.WriteString("(gantt_suggest had nothing to apply for this query)\n")
	}
	if len(failed) > 0 && len(done) == 0 {
		return nil, fmt.Errorf("%s", b.String())
	}
	return b.String(), nil
}

var ganttTools = []Tool{
	{
		Name:  "gantt_tasks",
		Short: "the schedule behind a gantt chart",
		Description: "The tasks a gantt chart draws for a query (an org_search expression, e.g. IsTask() && InFile(\"project.org\")), one per line: " +
			"Hash, keyword, title, start/end date, EFFORT, what it waits on (the Hash and title, by AFTER or an ORDERED parent), who, lane, percent done, file:line. " +
			"A task the query did not find but something found waits on is included and marked implied. " +
			"Step one of planning from tickets: gantt_tasks to see the chart, gantt_suggest for what the tickets imply, show the person, then gantt_apply_suggestions or gantt_link / gantt_set_effort for what they choose.",
		Schema: obj(map[string]interface{}{
			"query": sprop("the chart's query, an org_search expression"),
			"json":  bprop("answer with the task objects rather than one line each"),
			"limit": nprop("most tasks to return (default 100)"),
		}, "query"),
		Run: func(core *commands.Core, args map[string]interface{}) (interface{}, error) {
			q, err := need(args, "query")
			if err != nil {
				return nil, err
			}
			d, err := ganttFetch(core, q)
			if err != nil {
				return nil, err
			}
			if len(d.Tasks) == 0 {
				return "(no tasks: the query found nothing)", nil
			}
			limit := num(args, "limit", 100)
			if yes(args, "json") {
				rows, trimmed := capped(d.Tasks, limit)
				return withNote(rows, trimmed, len(d.Tasks)), nil
			}
			titles := ganttTitles(d)
			b := strings.Builder{}
			for i, t := range d.Tasks {
				if limit > 0 && i >= limit {
					break
				}
				b.WriteString(ganttRow(t, titles) + "\n")
			}
			if limit > 0 {
				b.WriteString(capNote(min(limit, len(d.Tasks)), len(d.Tasks)))
			}
			return b.String(), nil
		},
	},
	{
		Name:  "gantt_suggest",
		Short: "dependencies, estimates and lanes a chart's tickets imply",
		Description: "Suggestions for setting up a gantt chart from a pile of tickets, read from the tasks the query finds; writes nothing. " +
			"deps: what each task's own text says it waits on (\"blocked by ABC-12\", \"depends on [[*Heading]]\", \"needs the login page\", \"blocks X\"), numbered sibling steps, " +
			"plus existing AFTER links that are redundant and existing cycles. A task gets at most one, since AFTER holds one heading, and none if it already waits on something. " +
			"estimates: an EFFORT for each open task without one, from the clocked time of similar finished work. lanes: ways to split the chart into swim lanes (by tag, parent, file, dependency chain, or a property such as ASSIGNED). " +
			"ratios: how past estimates compared with the clock. Every suggestion carries hashes and a confidence. " +
			"Show them to the person before writing; then gantt_apply_suggestions applies them in bulk, or gantt_link / gantt_set_effort one at a time.",
		Schema: obj(map[string]interface{}{
			"query":    sprop("the chart's query, as for gantt_tasks"),
			"kinds":    map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string", "enum": []string{"deps", "estimates", "lanes", "ratios"}}, "description": "which suggestions; omit for all"},
			"dayHours": nprop("hours in a day of effort (default 8)"),
			"from":     sprop("start of the history estimates are read from, e.g. -180d (the default) or 2026-01-01"),
			"limit":    nprop("most entries per section (default 50)"),
		}, "query"),
		Run: func(core *commands.Core, args map[string]interface{}) (interface{}, error) {
			q, err := need(args, "query")
			if err != nil {
				return nil, err
			}
			s, err := ganttSuggestFetch(core, common.GanttSuggestReq{
				Query: q, Kinds: ganttKinds(args), DayHours: ganttFloat(args, "dayHours", 0), From: str(args, "from"),
			})
			if err != nil {
				return nil, err
			}
			d, err := ganttFetch(core, q)
			if err != nil {
				return nil, err
			}
			return renderSuggest(s, ganttTitles(d), num(args, "limit", 50)), nil
		},
	},
	{
		Name:   "gantt_link",
		Short:  "make one task wait on another",
		Writes: true,
		Description: "Make the task with Hash task wait on the task with Hash after (writes AFTER, giving the other an ID first if it has none), or with remove=true stop it waiting. " +
			"AFTER holds one heading: a task whose AFTER already names another is refused - remove that one first (remove=true) if the person wants it replaced. " +
			"Refused when it would make a circle or a task wait on its own parent. Hashes come from gantt_tasks or gantt_suggest.",
		Schema: obj(map[string]interface{}{
			"task":   sprop("Hash of the task that waits"),
			"after":  sprop("Hash of the task it waits on"),
			"remove": bprop("take the dependency away instead"),
		}, "task", "after"),
		Run: func(core *commands.Core, args map[string]interface{}) (interface{}, error) {
			t, err := need(args, "task")
			if err != nil {
				return nil, err
			}
			a, err := need(args, "after")
			if err != nil {
				return nil, err
			}
			res, err := commands.SendReceivePostErr[common.GanttLinkReq, common.ResultMsg](core, "gantt/link",
				&common.GanttLinkReq{Task: t, After: a, Remove: yes(args, "remove")})
			if err == commands.ErrDryRun {
				return "this server is being run with -dry-run: nothing was written, and nothing will be", nil
			}
			if err != nil {
				return nil, err
			}
			if !res.Ok {
				return nil, fmt.Errorf("%s", res.Msg)
			}
			return res.Msg, nil
		},
	},
	{
		Name:        "gantt_set_effort",
		Short:       "set a task's EFFORT",
		Writes:      true,
		Description: "Set the EFFORT of one task, an org duration such as 30min, 4h, 1.5d or 2w (a gantt bar's length). To clear it, use org_set_property with an empty value.",
		Schema: obj(map[string]interface{}{
			"hash":   sprop("the task's Hash, from gantt_tasks"),
			"effort": sprop("the duration, e.g. 4h or 2d"),
		}, "hash", "effort"),
		Run: func(core *commands.Core, args map[string]interface{}) (interface{}, error) {
			h, err := need(args, "hash")
			if err != nil {
				return nil, err
			}
			e, err := need(args, "effort")
			if err != nil {
				return nil, err
			}
			req := common.TodoPropertyChange{Hash: h, Name: "EFFORT", Value: e}
			return post[common.TodoPropertyChange, common.Result](core, "property", &req)
		},
	},
	{
		Name:   "gantt_apply_suggestions",
		Short:  "write gantt_suggest's suggestions",
		Writes: true,
		Description: "Fetch gantt_suggest for a query and write what is asked for: deps (each as gantt_link would), estimates (EFFORT on open tasks without one), " +
			"and/or a lane split (writes laneProperty, default SECTION - the chart's lane - on every task, from the gantt_suggest lane with that by/prop). " +
			"Only suggestions at or above minConfidence (default 0.5) are written. Every heading is looked up again before each write, so nothing is written by a stale hash. " +
			"Use it after the person has seen gantt_suggest's list and agreed; for a hand-picked few use gantt_link and gantt_set_effort instead. Answers with what it wrote, what failed and what it left alone.",
		Schema: obj(map[string]interface{}{
			"query":         sprop("the chart's query, as for gantt_suggest"),
			"deps":          bprop("write the suggested dependencies"),
			"estimates":     bprop("write the suggested EFFORTs"),
			"minConfidence": nprop("only write suggestions at least this confident, 0 to 1 (default 0.5)"),
			"lane": map[string]interface{}{"type": "object", "description": "a lane split from gantt_suggest to write, e.g. {\"by\":\"property\",\"prop\":\"COMPONENT\"} or {\"by\":\"tag\"}",
				"properties": map[string]interface{}{"by": sprop("tag, parent, file, chain or property"), "prop": sprop("the property, when by is property")}},
			"laneProperty": sprop("the property the lane is written to (default SECTION, which is what the chart draws lanes by)"),
			"dayHours":     nprop("hours in a day of effort (default 8)"),
			"from":         sprop("start of the history estimates are read from (default -180d)"),
		}, "query"),
		Run: applySuggestions,
	},
}
