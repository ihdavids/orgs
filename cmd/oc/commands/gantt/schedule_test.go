package gantt

// The same scenarios as worg/src/gantt.test.ts, because the two schedulers have
// to put every bar on the same day: a difference between them is a chart that
// is wrong in one place and right in the other, and only a test notices.

import (
	"sort"
	"testing"
	"time"

	"github.com/ihdavids/orgs/internal/common"
)

var now = time.Date(2026, 1, 5, 9, 0, 0, 0, time.Local) // Monday 2026-01-05

func srv(hash string, props map[string]string, f func(*common.GanttTask)) common.GanttTask {
	t := common.GanttTask{Hash: hash, Headline: hash, Props: props, Section: "main"}
	if f != nil {
		f(&t)
	}
	return t
}

func find(m *Model, name string) *Task {
	for _, t := range m.Tasks {
		if t.Name == name {
			return t
		}
	}
	return nil
}

func TestAfterChainsAndWeekends(t *testing.T) {
	m := Build([]common.GanttTask{
		srv("a", nil, func(x *common.GanttTask) { x.Start = "2026-01-08"; x.EffortDays = 2 }),
		srv("b", nil, func(x *common.GanttTask) { x.After = "a"; x.EffortDays = 3 }),
	}, now, true, "")
	// Two days from Thursday ends Saturday midnight; the weekend is skipped,
	// so b starts Monday and three working days end Thursday.
	if got := dayKey(find(m, "a").End); got != "2026-01-10" {
		t.Errorf("a ends %s", got)
	}
	b := find(m, "b")
	if dayKey(b.Start) != "2026-01-12" || dayKey(b.End) != "2026-01-15" {
		t.Errorf("b runs %s to %s", dayKey(b.Start), dayKey(b.End))
	}
	if b.PlannedDays != 3 || b.Days != 3 {
		t.Errorf("b planned %v wide %v", b.PlannedDays, b.Days)
	}
	if len(m.Errors) != 0 {
		t.Errorf("errors %v", m.Errors)
	}
}

func TestPlannedIsWhatWasAskedNotTheWidth(t *testing.T) {
	m := Build([]common.GanttTask{
		srv("a", nil, func(x *common.GanttTask) { x.Start = "2026-01-08"; x.EffortDays = 5 }),
	}, now, true, "")
	a := find(m, "a")
	if a.PlannedDays != 5 || a.Days != 7 || dayKey(a.End) != "2026-01-15" {
		t.Errorf("planned %v wide %v ends %s", a.PlannedDays, a.Days, dayKey(a.End))
	}
}

func TestUndatedFollowsTheOneAboveAndFirstStartsToday(t *testing.T) {
	m := Build([]common.GanttTask{
		srv("a", nil, nil),
		srv("b", nil, nil),
	}, now, false, "")
	if dayKey(find(m, "a").Start) != "2026-01-05" || dayKey(find(m, "b").Start) != "2026-01-06" {
		t.Errorf("a %s b %s", dayKey(find(m, "a").Start), dayKey(find(m, "b").Start))
	}
}

func TestMissingDependencyIsReportedAndCyclesTerminate(t *testing.T) {
	m := Build([]common.GanttTask{
		srv("a", nil, func(x *common.GanttTask) { x.After = "nowhere" }),
	}, now, false, "")
	if len(m.Errors) != 1 {
		t.Errorf("errors %v", m.Errors)
	}
	m = Build([]common.GanttTask{
		srv("a", nil, func(x *common.GanttTask) { x.After = "b" }),
		srv("b", nil, func(x *common.GanttTask) { x.After = "a" }),
	}, now, false, "")
	if len(m.Errors) == 0 || len(m.Tasks) != 2 {
		t.Errorf("a cycle should be reported and still drawn: %v", m.Errors)
	}
}

func TestMilestonesKeepTheirDay(t *testing.T) {
	m := Build([]common.GanttTask{
		srv("m", nil, func(x *common.GanttTask) { x.Start = "2026-01-10"; x.Milestone = true }),
	}, now, true, "")
	ms := find(m, "m")
	if dayKey(ms.Start) != "2026-01-10" || !ms.End.Equal(ms.Start) || ms.Days != 0 {
		t.Errorf("milestone at %s days %v", dayKey(ms.Start), ms.Days)
	}
}

func TestLanesByPropertySortedWithTheEmptyOneLast(t *testing.T) {
	m := Build([]common.GanttTask{
		srv("a", map[string]string{"OWNER": "bob"}, nil),
		srv("b", nil, nil),
		srv("c", map[string]string{"OWNER": "alice"}, nil),
	}, now, false, "OWNER")
	got := []string{}
	for _, s := range m.Sections {
		got = append(got, s.Name)
	}
	want := []string{"alice", "bob", "(no OWNER)"}
	for i := range want {
		if i >= len(got) || got[i] != want[i] {
			t.Fatalf("lanes %v, want %v", got, want)
		}
	}
}

func TestCriticalPath(t *testing.T) {
	// a -> b is five days; c runs alongside for one and has room.
	m := Build([]common.GanttTask{
		srv("a", nil, func(x *common.GanttTask) { x.Start = "2026-01-05"; x.EffortDays = 2 }),
		srv("b", nil, func(x *common.GanttTask) { x.After = "a"; x.EffortDays = 3 }),
		srv("c", nil, func(x *common.GanttTask) { x.Start = "2026-01-05"; x.EffortDays = 1 }),
	}, now, false, "")
	got := []string{}
	for id := range CriticalPath(m) {
		got = append(got, id)
	}
	sort.Strings(got)
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("critical %v", got)
	}
}

func TestHealth(t *testing.T) {
	m := Build([]common.GanttTask{
		srv("late", nil, func(x *common.GanttTask) { x.Start = "2025-12-29"; x.EffortDays = 2 }),
		srv("today", nil, func(x *common.GanttTask) { x.Start = "2026-01-05"; x.EffortDays = 1 }),
		srv("untracked", nil, func(x *common.GanttTask) { x.Start = "2026-01-01"; x.EffortDays = 10 }),
		srv("behind", map[string]string{"PERCENTDONE": "0"}, func(x *common.GanttTask) { x.Start = "2026-01-01"; x.EffortDays = 10 }),
		srv("finished", nil, func(x *common.GanttTask) { x.Start = "2025-12-01"; x.Done = true }),
	}, now, false, "")
	want := map[string]string{"late": "overdue", "today": "", "untracked": "", "behind": "slipping", "finished": ""}
	for name, kind := range want {
		if got, _ := Health(find(m, name), now); got != kind {
			t.Errorf("%s: %q, want %q", name, got, kind)
		}
	}
}

func TestNaturalOrder(t *testing.T) {
	if !naturalLess("sprint 9", "sprint 10") || naturalLess("Sprint 10", "sprint 9") {
		t.Error("numbers inside lane names should sort as numbers")
	}
}
