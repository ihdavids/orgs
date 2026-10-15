package gantt

// The plan as a spreadsheet: orgs gantt <query> -csv, or -jira for a file
// Jira's "Import issues from CSV" takes as it is.
//
// Both are written from the scheduled Model, not from /gantt/tasks, because
// the dates are the point and only the scheduler knows them. Rows are in
// outline order (file, then line) rather than the chart's lane order, so a
// parent always comes before its children: the WBS numbers read down the
// page, and Jira resolves a Parent it has already seen.

import (
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

// A working day of EFFORT, as Jira's time tracking counts one by default.
const jiraDayHours = 8

// The importer's own default date format (dd/MMM/yy h:mm a), so the date step
// of the wizard can be left as it is.
const jiraDate = "02/Jan/06 3:04 PM"

// csvPlan is the export's view of the plan: tasks in outline order, each with
// its row number, and parents worked out within what was exported.
type csvPlan struct {
	tasks  []*Task
	num    map[string]int    // hash -> 1-based row number
	parent map[string]string // hash -> parent's hash, only when the parent is exported too
	kids   map[string][]string
	// Who waits on whom, by hash: AFTER and ORDERED chains both.
	blocks map[string][]string
	// A parent with no date or effort of its own runs from its first child's
	// start to its last child's end, not the default one-day bar.
	spans map[string][2]time.Time
}

func newCsvPlan(m *Model) *csvPlan {
	p := &csvPlan{
		num: map[string]int{}, parent: map[string]string{},
		kids: map[string][]string{}, blocks: map[string][]string{},
	}
	p.tasks = append(p.tasks, m.Tasks...)
	sort.SliceStable(p.tasks, func(a, b int) bool {
		x, y := p.tasks[a].Src, p.tasks[b].Src
		if x.Filename != y.Filename {
			return x.Filename < y.Filename
		}
		return x.LineNum < y.LineNum
	})
	for i, t := range p.tasks {
		p.num[t.ID] = i + 1
	}
	p.spans = map[string][2]time.Time{}
	for _, t := range p.tasks {
		if par := t.Src.Parent; par != "" && par != t.ID {
			if _, ok := p.num[par]; ok {
				p.parent[t.ID] = par
				p.kids[par] = append(p.kids[par], t.ID)
			}
		}
		for _, a := range t.After {
			if _, ok := p.num[a]; ok {
				p.blocks[a] = append(p.blocks[a], t.ID)
			}
		}
	}
	for i := len(p.tasks) - 1; i >= 0; i-- { // children come after parents
		t := p.tasks[i]
		if len(p.kids[t.ID]) == 0 || t.Dated || t.PlannedDays > 0 || t.Milestone || t.Mark {
			continue
		}
		var from, to time.Time
		for j, k := range p.kids[t.ID] {
			s, e := p.span(p.task(k))
			if j == 0 || s.Before(from) {
				from = s
			}
			if j == 0 || e.After(to) {
				to = e
			}
		}
		p.spans[t.ID] = [2]time.Time{from, to}
	}
	return p
}

func (p *csvPlan) task(id string) *Task { return p.tasks[p.num[id]-1] }

// span is when a task starts, and the midnight after the day it finishes.
func (p *csvPlan) span(t *Task) (time.Time, time.Time) {
	if s, ok := p.spans[t.ID]; ok {
		return s[0], s[1]
	}
	return t.Start, t.End
}

// days is the bar's length in calendar days, as Task.Days counts it.
func (p *csvPlan) days(t *Task) float64 {
	if _, ok := p.spans[t.ID]; !ok {
		return t.Days
	}
	s, e := p.span(t)
	return e.Sub(s).Hours() / 24
}

// wbs is the outline number of a task among the exported ones: 1, 1.2, 1.2.3.
func (p *csvPlan) wbs() map[string]string {
	out := map[string]string{}
	counts := map[string]int{} // parent hash ("" for the top) -> children numbered so far
	for _, t := range p.tasks {
		par := p.parent[t.ID]
		counts[par]++
		n := strconv.Itoa(counts[par])
		if par != "" {
			n = out[par] + "." + n
		}
		out[t.ID] = n
	}
	return out
}

func (p *csvPlan) depth(id string) int {
	d := 0
	for cur := p.parent[id]; cur != "" && d < 100; cur = p.parent[cur] {
		d++
	}
	return d
}

// lastDay is the day a span finishes on: its end is the midnight after it.
func lastDay(start, end time.Time) time.Time {
	if !end.After(start) {
		return start
	}
	return end.Add(-time.Nanosecond)
}

func fmtDays(d float64) string {
	return strconv.FormatFloat(math.Round(d*100)/100, 'f', -1, 64)
}

// writeCSV is the plan for a spreadsheet or another planning tool: one row a
// task, Predecessors by row number the way MS Project and ProjectLibre read
// them, and the hash and file:line to find the heading again.
func writeCSV(w io.Writer, m *Model, now time.Time, crit map[string]bool) error {
	p := newCsvPlan(m)
	wbs := p.wbs()
	cw := csv.NewWriter(w)
	cw.Write([]string{"ID", "WBS", "Outline Level", "Name", "Section", "Resource", "Status", "Priority",
		"Start", "Finish", "Duration (days)", "Effort", "Percent Complete", "Predecessors",
		"Milestone", "Critical", "Health", "Tags", "Implied", "Hash", "File", "Line"})
	for _, t := range p.tasks {
		preds := []string{}
		for _, a := range t.After {
			if n, ok := p.num[a]; ok {
				preds = append(preds, strconv.Itoa(n))
			}
		}
		_, health := Health(t, now)
		start, end := p.span(t)
		cw.Write([]string{
			strconv.Itoa(p.num[t.ID]), wbs[t.ID], strconv.Itoa(p.depth(t.ID) + 1),
			t.Name, t.Section, t.Src.Resource, t.Src.Status, t.Src.Priority,
			start.Format("2006-01-02"), lastDay(start, end).Format("2006-01-02"),
			fmtDays(p.days(t)), t.Src.Effort, strconv.Itoa(t.Src.Percent), strings.Join(preds, ","),
			yesNo(t.Milestone || t.Mark), yesNo(crit[t.ID]), health,
			strings.Join(t.Src.Tags, " "), yesNo(t.Src.Implied),
			t.ID, t.Src.Filename, strconv.Itoa(t.Src.LineNum + 1), // 1-based, as an editor counts
		})
	}
	cw.Flush()
	return cw.Error()
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// The properties the jira plugin already reads and writes on a heading, so a
// plan that has been near the sync says the same thing here.
func jiraType(p *csvPlan, t *Task) string {
	if v := propOf(t.Src.Props, "ISSUETYPE"); v != "" {
		return v
	}
	// Jira's hierarchy is three deep: an epic, its tasks, their sub-tasks. A
	// heading with work under it at the top of the export is the epic; deeper
	// than a sub-task flattens onto the sub-task's task (see jiraParent).
	switch d := p.depth(t.ID); {
	case d == 0 && len(p.kids[t.ID]) > 0:
		return "Epic"
	case d == 0, d == 1 && p.isEpic(p.parent[t.ID]):
		return "Task"
	default:
		return "Sub-task"
	}
}

func (p *csvPlan) isEpic(id string) bool {
	return p.depth(id) == 0 && len(p.kids[id]) > 0
}

// jiraParent is the row a task sits under in Jira: its own parent, except that
// a sub-task's children go under the same task the sub-task does, because a
// sub-task cannot have any.
func (p *csvPlan) jiraParent(t *Task, types map[string]string) string {
	par := p.parent[t.ID]
	for par != "" && types[par] == "Sub-task" {
		par = p.parent[par]
	}
	return par
}

func jiraStatus(t *Task) string {
	if v := propOf(t.Src.Props, "STATUS"); v != "" {
		return v
	}
	switch {
	case t.Src.Done || t.State == "done":
		return "Done"
	}
	// The keyword, not the chart's Active flag, which marks anything current.
	switch strings.ToUpper(strings.ReplaceAll(t.Src.Status, "-", "")) {
	case "NEXT", "STARTED", "INPROGRESS", "DOING", "WIP", "ACTIVE":
		return "In Progress"
	}
	return "To Do"
}

func jiraPriority(t *Task) string {
	if v := propOf(t.Src.Props, "PRIORITY"); v != "" {
		return v
	}
	switch t.Src.Priority {
	case "A":
		return "High"
	case "B":
		return "Medium"
	case "C":
		return "Low"
	}
	return ""
}

func jiraAssignee(t *Task) string {
	for _, k := range []string{"ASSIGNEE", "ASSIGNED", "RID", "RESOURCEID"} {
		if v := strings.Trim(strings.TrimSpace(propOf(t.Src.Props, k)), "[]"); v != "" {
			return v
		}
	}
	return ""
}

// Jira labels are single words: tags already are, LABELS is split the way the
// jira plugin writes it, and a SECTION goes in so the chart's grouping
// survives. Only one written down: the lane otherwise falls back to the
// assignee, which has its own column.
func jiraLabels(t *Task) []string {
	seen := map[string]bool{}
	out := []string{}
	add := func(s string) {
		s = strings.Join(strings.Fields(s), "_")
		if s != "" && !seen[strings.ToLower(s)] {
			seen[strings.ToLower(s)] = true
			out = append(out, s)
		}
	}
	for _, tag := range t.Src.Tags {
		add(tag)
	}
	for _, l := range strings.FieldsFunc(propOf(t.Src.Props, "LABELS"), func(r rune) bool { return r == ',' || r == ' ' }) {
		add(l)
	}
	add(propOf(t.Src.Props, "SECTION"))
	if t.Milestone || t.Mark {
		add("milestone")
	}
	return out
}

// writeJira is a file for Jira's CSV importer. Columns named as Jira names its
// fields map themselves; "Issue Id"/"Parent" carry the hierarchy, and a
// "Blocks" column on a task names the rows that wait on it (map it to Linked
// Issues: Blocks). Repeated columns (Labels, Blocks) are how the importer
// takes more than one value. Estimates are in seconds, at 8h a day.
func writeJira(w io.Writer, m *Model) error {
	p := newCsvPlan(m)
	types := map[string]string{}
	for _, t := range p.tasks {
		types[t.ID] = jiraType(p, t)
	}

	maxLabels, maxBlocks, project := 0, 0, false
	for _, t := range p.tasks {
		maxLabels = max(maxLabels, len(jiraLabels(t)))
		maxBlocks = max(maxBlocks, len(p.blocks[t.ID]))
		if propOf(t.Src.Props, "PROJECTKEY") != "" {
			project = true
		}
	}

	head := []string{"Issue Id", "Parent", "Issue Type", "Summary", "Status", "Priority", "Assignee",
		"Start date", "Due date", "Original Estimate", "Remaining Estimate"}
	if project {
		head = append(head, "Project Key")
	}
	for i := 0; i < maxLabels; i++ {
		head = append(head, "Labels")
	}
	for i := 0; i < maxBlocks; i++ {
		head = append(head, "Blocks")
	}
	head = append(head, "Description")

	cw := csv.NewWriter(w)
	cw.Write(head)
	for _, t := range p.tasks {
		parent := ""
		if par := p.jiraParent(t, types); par != "" {
			parent = strconv.Itoa(p.num[par])
		}
		est, left := "", ""
		if days := t.PlannedDays; days > 0 {
			secs := days * jiraDayHours * 3600
			est = strconv.Itoa(int(math.Round(secs)))
			left = strconv.Itoa(int(math.Round(secs * float64(100-min(max(t.Src.Percent, 0), 100)) / 100)))
		}
		start, end := p.span(t)
		row := []string{
			strconv.Itoa(p.num[t.ID]), parent, types[t.ID], t.Name, jiraStatus(t), jiraPriority(t),
			jiraAssignee(t), startOfDay(start).Format(jiraDate), startOfDay(lastDay(start, end)).Format(jiraDate), est, left,
		}
		if project {
			row = append(row, propOf(t.Src.Props, "PROJECTKEY"))
		}
		labels := jiraLabels(t)
		for i := 0; i < maxLabels; i++ {
			row = append(row, at(labels, i))
		}
		blocks := []string{}
		for _, b := range p.blocks[t.ID] {
			blocks = append(blocks, strconv.Itoa(p.num[b]))
		}
		for i := 0; i < maxBlocks; i++ {
			row = append(row, at(blocks, i))
		}
		row = append(row, fmt.Sprintf("From %s:%d", t.Src.Filename, t.Src.LineNum+1))
		cw.Write(row)
	}
	cw.Flush()
	return cw.Error()
}

func at(xs []string, i int) string {
	if i < len(xs) {
		return xs[i]
	}
	return ""
}
