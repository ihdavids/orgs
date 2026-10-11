package orgs

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ihdavids/orgs/internal/common"
)

func planDate(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func planEdit(t *testing.T, text string, day string, start, end int) string {
	t.Helper()
	out, _, err := planBlockLines(strings.Split(text, "\n"), 1, 2, planDate(t, day), start, end)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Join(out, "\n")
}

const plHead = "* P\n** TODO A\n"
const plNext = "** TODO B\n   SCHEDULED: <2026-10-08 Thu>"

func TestPlanBlockWritesARangeAndKeepsTheRest(t *testing.T) {
	// No planning line: one is made, above the drawer.
	in := plHead + ":PROPERTIES:\n:EFFORT:   1:30\n:END:\n" + plNext
	want := plHead + "SCHEDULED: <2026-10-12 Mon 10:00-11:30>\n:PROPERTIES:\n:EFFORT:   1:30\n:END:\n" + plNext
	if got := planEdit(t, in, "2026-10-12", 600, 690); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	// A deadline on the line stays; the repeater and warning on SCHEDULED
	// stay as written.
	in = plHead + "   DEADLINE: <2026-10-20 Tue> SCHEDULED: <2026-10-05 Mon .+1w -2d>\n" + plNext
	want = plHead + "   DEADLINE: <2026-10-20 Tue> SCHEDULED: <2026-10-12 Mon 14:15-15:00 .+1w -2d>\n" + plNext
	if got := planEdit(t, in, "2026-10-12", 855, 900); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	// Moving a block replaces its time.
	in = plHead + "   SCHEDULED: <2026-10-12 Mon 10:00-11:00 +1d>\n" + plNext
	want = plHead + "   SCHEDULED: <2026-10-13 Tue 09:00-09:30 +1d>\n" + plNext
	if got := planEdit(t, in, "2026-10-13", 540, 570); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestPlanBlockRefusesABackwardsBlock(t *testing.T) {
	for _, c := range [][2]int{{600, 600}, {660, 600}, {1400, 1440}} {
		if _, _, err := planBlockLines(strings.Split(plHead+plNext, "\n"), 1, 2, planDate(t, "2026-10-12"), c[0], c[1]); err == nil {
			t.Errorf("%v: no error", c)
		}
	}
}

func TestPlanUnblock(t *testing.T) {
	run := func(in string, keep bool) (string, string) {
		t.Helper()
		out, stamp, err := planUnblockLines(strings.Split(in, "\n"), 1, 2, keep)
		if err != nil {
			t.Fatal(err)
		}
		return strings.Join(out, "\n"), stamp
	}
	// Plain: SCHEDULED goes, the deadline stays.
	got, stamp := run(plHead+"   SCHEDULED: <2026-10-12 Mon 10:00-11:00> DEADLINE: <2026-10-20 Tue>\n"+plNext, false)
	if got != plHead+"   DEADLINE: <2026-10-20 Tue>\n"+plNext || stamp != "" {
		t.Errorf("got\n%s\n(%q)", got, stamp)
	}
	// Alone on its line: the line goes.
	got, _ = run(plHead+"   SCHEDULED: <2026-10-12 Mon 10:00-11:00>\n"+plNext, false)
	if got != plHead+plNext {
		t.Errorf("got\n%s", got)
	}
	// A repeater is never lost: only the time comes off.
	got, stamp = run(plHead+"   SCHEDULED: <2026-10-12 Mon 10:00-11:00 +1w>\n"+plNext, false)
	if got != plHead+"   SCHEDULED: <2026-10-12 Mon +1w>\n"+plNext || stamp != "<2026-10-12 Mon +1w>" {
		t.Errorf("got\n%s\n(%q)", got, stamp)
	}
	// Asked to keep the date.
	got, _ = run(plHead+"   SCHEDULED: <2026-10-12 Mon 10:00-11:00>\n"+plNext, true)
	if got != plHead+"   SCHEDULED: <2026-10-12 Mon>\n"+plNext {
		t.Errorf("got\n%s", got)
	}
	if _, _, err := planUnblockLines(strings.Split(plHead+plNext, "\n"), 1, 2, false); err == nil {
		t.Error("unscheduled: no error")
	}
}

// The same cases are in worg/src/planner.test.ts.
func TestPlanLoadOverlapsAndFreeSlot(t *testing.T) {
	spans := []planSpan{{540, 600}, {570, 630}, {720, 780}, {1020, 1080}, {900, 900}}
	planned, double, outside := planLoad(spans, 540, 1020)
	if planned != 150 || double != 30 || outside != 60 {
		t.Errorf("load: planned %d double %d outside %d", planned, double, outside)
	}
	if got := planOverlaps(spans); !reflect.DeepEqual(got, [][2]int{{0, 1}}) {
		t.Errorf("overlaps: %v", got)
	}
	cases := []struct{ from, length, want int }{
		{540, 60, 630},   // 9:00-10:30 taken, 10:30 has 90 free
		{540, 120, 780},  // the first two hours free come after lunch
		{660, 30, 660},   // from 11:00
		{540, 300, -1},   // nothing that long
		{1000, 20, 1000}, // right up to the end of the window
		{1001, 20, -1},
	}
	for _, c := range cases {
		got, ok := planFreeSlot(spans, 540, 1020, c.from, c.length)
		if !ok {
			got = -1
		}
		if got != c.want {
			t.Errorf("free from %d for %d: got %d want %d", c.from, c.length, got, c.want)
		}
	}
}

func TestPlanCookies(t *testing.T) {
	cases := map[string][]string{
		"<2026-10-05 Mon>":                  {},
		"<2026-10-05 Mon 10:00 +1w>":        {"+1w"},
		"<2026-10-05 Mon .+2d/4d -1d>":      {".+2d/4d", "-1d"},
		"<2026-10-05 Mon ++1m>--<2026-10-07 Wed>": {"++1m"},
	}
	for in, want := range cases {
		if got := planCookies(in); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: got %v want %v", in, got, want)
		}
	}
}

func planTitles(blocks []common.PlanBlock) string {
	out := []string{}
	for _, b := range blocks {
		s := b.Headline + " " + b.Kind + " " + b.Start + "-" + b.End
		if b.Implied {
			s += " implied"
		}
		if b.Movable {
			s += " movable"
		}
		out = append(out, s)
	}
	return strings.Join(out, "\n")
}

func TestPlanDay(t *testing.T) {
	text := "* Work\n" +
		"** TODO Write report\n   SCHEDULED: <2026-10-12 Mon 10:00-11:30>\n" +
		"** TODO Review PR\n   SCHEDULED: <2026-10-12 Mon 11:00>\n   :PROPERTIES:\n   :EFFORT:   0:45\n   :END:\n" +
		"** Standup <2026-10-12 Mon 09:00-09:15>\n" +
		"** TODO Ship it\n   DEADLINE: <2026-10-12 Mon 17:00>\n" +
		"** TODO Plan sprint\n   :PROPERTIES:\n   :EFFORT:   2h\n   :END:\n" +
		"** TODO Overdue thing\n   SCHEDULED: <2026-10-09 Fri>\n" +
		"** TODO Later thing\n   SCHEDULED: <2026-10-20 Tue>\n" +
		"** DONE Finished\n" +
		"** TODO Weekly review\n   SCHEDULED: <2026-10-05 Mon 15:00-16:00 +1w>\n"
	loadOrg(t, map[string]string{"plan.org": text})
	day := agDay(t, "2026-10-12")
	res, err := buildPlanDay(planDayArgs{Date: day, Today: agDay(t, "2026-10-10"), DayStart: 540, DayEnd: 1020, Now: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Join([]string{
		"Standup timestamp 09:00-09:15",
		"Write report scheduled 10:00-11:30 movable",
		"Review PR scheduled 11:00-11:45 implied movable",
		"Weekly review scheduled 15:00-16:00 movable",
		"Ship it deadline 17:00-17:00",
	}, "\n")
	if got := planTitles(res.Blocks); got != want {
		t.Errorf("blocks\n%s\nwant\n%s", got, want)
	}
	titles := []string{}
	for _, task := range res.Tasks {
		titles = append(titles, task.Headline)
	}
	// The overdue one first; no later, done or already placed ones.
	if got := strings.Join(titles, ","); got != "Overdue thing,Plan sprint" {
		t.Errorf("tasks: %s", got)
	}
	if res.Tasks[0].Late != 3 || res.Tasks[1].EffortMins != 120 || res.Tasks[1].Late != -1 {
		t.Errorf("task details: %+v", res.Tasks)
	}
	// 15 + 90 + 45 + 60 = 210 minutes, 30 of them twice (11:00-11:30).
	if res.Load.PlannedMins != 180 || res.Load.DoubleMins != 30 || res.Load.FreeMins != 300 {
		t.Errorf("load: %+v", res.Load)
	}
	if !reflect.DeepEqual(res.Overlaps, [][2]int{{1, 2}}) {
		t.Errorf("overlaps: %v", res.Overlaps)
	}
	if res.Load.BacklogMins != 120 || res.Load.BacklogNoEst != 1 {
		t.Errorf("backlog: %+v", res.Load)
	}
}

func TestPlanBlockAndUnblockOnDisk(t *testing.T) {
	text := "* Work\n** TODO Plan sprint\n   :PROPERTIES:\n   :EFFORT:   2h\n   :END:\n" +
		"** TODO Busy\n   SCHEDULED: <2026-10-12 Mon 09:00-10:00>\n** TODO After\n"
	paths := loadOrg(t, map[string]string{"plan.org": text})
	registerAllSections()
	hash := ""
	for _, sec := range flattenSections(GetDb().FindByFile(paths["plan.org"])) {
		if sec.Headline != nil && strings.Contains(sec.Headline.Title[0].String(), "Plan sprint") {
			hash = sec.Hash
		}
	}
	if hash == "" {
		t.Fatal("no hash")
	}
	now := time.Date(2026, 10, 10, 8, 0, 0, 0, time.Local)
	// No start: the first free two hours on the day, after the busy hour.
	res, err := planBlock(common.PlanBlockReq{Hash: hash, Date: "2026-10-12"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if res.Start != "10:00" || res.End != "12:00" || res.Hash != hash {
		t.Errorf("placed %+v", res)
	}
	want := "* Work\n** TODO Plan sprint\n   SCHEDULED: <2026-10-12 Mon 10:00-12:00>\n   :PROPERTIES:\n   :EFFORT:   2h\n   :END:\n" +
		"** TODO Busy\n   SCHEDULED: <2026-10-12 Mon 09:00-10:00>\n** TODO After\n"
	if got := readOrg(t, paths["plan.org"]); got != strings.TrimRight(want, "\n") {
		t.Errorf("file\n%s\nwant\n%s", got, want)
	}
	if _, err := planUnblock(common.PlanUnblockReq{Hash: hash}); err != nil {
		t.Fatal(err)
	}
	if got := readOrg(t, paths["plan.org"]); got != strings.TrimRight(text, "\n") {
		t.Errorf("unblock left\n%s", got)
	}
}
