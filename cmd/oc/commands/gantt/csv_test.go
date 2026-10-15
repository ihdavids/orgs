package gantt

import (
	"bytes"
	"encoding/csv"
	"testing"

	"github.com/ihdavids/orgs/internal/common"
)

func exportPlan() *Model {
	return Build([]common.GanttTask{
		srv("proj", nil, func(x *common.GanttTask) { x.LineNum = 1; x.Tags = []string{"project"} }),
		srv("design", map[string]string{"ASSIGNED": "[ian]"}, func(x *common.GanttTask) {
			x.LineNum = 2; x.Parent = "proj"; x.Start = "2026-01-05"; x.EffortDays = 2; x.Priority = "A"
		}),
		srv("build", nil, func(x *common.GanttTask) {
			x.LineNum = 5; x.Parent = "proj"; x.After = "design"; x.EffortDays = 3; x.Percent = 50; x.Status = "NEXT"; x.Active = true
		}),
		srv("step", nil, func(x *common.GanttTask) { x.LineNum = 6; x.Parent = "build"; x.Start = "2026-01-07" }),
		srv("deeper", nil, func(x *common.GanttTask) { x.LineNum = 7; x.Parent = "step"; x.Start = "2026-01-07" }),
	}, now, true, "")
}

func readCSV(t *testing.T, b []byte) []map[string]string {
	rows, err := csv.NewReader(bytes.NewReader(b)).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	out := []map[string]string{}
	for _, r := range rows[1:] {
		m := map[string]string{}
		for i, h := range rows[0] {
			if m[h] == "" { // the first of a repeated column
				m[h] = r[i]
			}
		}
		out = append(out, m)
	}
	return out
}

func TestCSVExport(t *testing.T) {
	var buf bytes.Buffer
	if err := writeCSV(&buf, exportPlan(), now, nil); err != nil {
		t.Fatal(err)
	}
	rows := readCSV(t, buf.Bytes())
	want := []struct{ name, wbs, start, finish, preds string }{
		{"proj", "1", "2026-01-05", "2026-01-09", ""}, // its children's span
		{"design", "1.1", "2026-01-05", "2026-01-06", ""},
		{"build", "1.2", "2026-01-07", "2026-01-09", "2"},
		{"step", "1.2.1", "", "", ""},
		{"deeper", "1.2.1.1", "", "", ""},
	}
	for i, w := range want {
		r := rows[i]
		if r["Name"] != w.name || r["WBS"] != w.wbs || r["Predecessors"] != w.preds {
			t.Errorf("row %d: %v", i, r)
		}
		if w.start != "" && (r["Start"] != w.start || r["Finish"] != w.finish) {
			t.Errorf("%s runs %s to %s", w.name, r["Start"], r["Finish"])
		}
	}
}

func TestJiraExport(t *testing.T) {
	var buf bytes.Buffer
	if err := writeJira(&buf, exportPlan()); err != nil {
		t.Fatal(err)
	}
	rows := readCSV(t, buf.Bytes())
	want := []struct{ id, parent, typ, blocks string }{
		{"1", "", "Epic", ""},
		{"2", "1", "Task", "3"},
		{"3", "1", "Task", ""},
		{"4", "3", "Sub-task", ""},
		{"5", "3", "Sub-task", ""}, // a sub-task cannot have children: under its task
	}
	for i, w := range want {
		r := rows[i]
		if r["Issue Id"] != w.id || r["Parent"] != w.parent || r["Issue Type"] != w.typ || r["Blocks"] != w.blocks {
			t.Errorf("row %d: %v", i, r)
		}
	}
	d, b := rows[1], rows[2]
	if d["Priority"] != "High" || d["Assignee"] != "ian" || d["Original Estimate"] != "57600" {
		t.Errorf("design: %v", d)
	}
	if d["Start date"] != "05/Jan/26 12:00 AM" || d["Due date"] != "06/Jan/26 12:00 AM" {
		t.Errorf("design dates %q %q", d["Start date"], d["Due date"])
	}
	if rows[3]["Status"] != "To Do" {
		t.Errorf("step: %v", rows[3])
	}
	if b["Status"] != "In Progress" || b["Remaining Estimate"] != "43200" {
		t.Errorf("build: %v", b)
	}
}
