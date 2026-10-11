package plan

import (
	"strings"
	"testing"
	"time"

	"github.com/ihdavids/orgs/internal/common"
)

func TestParseSlot(t *testing.T) {
	cases := map[string]Slot{
		"10:00":       {600, -1, false},
		"9":           {540, -1, false},
		"2pm":         {840, -1, false},
		"12am":        {0, -1, false},
		"9:30am":      {570, -1, false},
		"10:00-11:30": {600, 690, false},
		"14-15:30":    {840, 930, false},
		"2-4pm":       {840, 960, false},
		"next":        {0, -1, true},
	}
	for in, want := range cases {
		got, ok := ParseSlot(in)
		if !ok || got != want {
			t.Errorf("%s: got %+v %v want %+v", in, got, ok, want)
		}
	}
	for _, bad := range []string{"report", "2026", "25:00", "10:75", "13pm", "fri"} {
		if _, ok := ParseSlot(bad); ok {
			t.Errorf("%s read as a time", bad)
		}
	}
}

func TestMins(t *testing.T) {
	for m, want := range map[int]string{0: "0m", 45: "45m", 60: "1h", 90: "1h30", 125: "2h05"} {
		if got := Mins(m); got != want {
			t.Errorf("%d: %s", m, got)
		}
	}
}

func TestDraw(t *testing.T) {
	pd := common.PlanDay{
		Date: "2026-10-12", DayStart: "09:00", DayEnd: "12:00",
		Blocks: []common.PlanBlock{
			{Headline: "Standup", Kind: "timestamp", Start: "09:00", End: "09:15", Mins: 15},
			{Headline: "Write report", Status: "TODO", Kind: "scheduled", Start: "10:00", End: "11:00", Mins: 60, Movable: true},
			{Headline: "Review", Status: "TODO", Kind: "scheduled", Start: "10:30", End: "11:00", Mins: 30, Movable: true},
		},
		Overlaps: [][2]int{{1, 2}},
		Tasks:    []common.PlanTask{{Headline: "Plan sprint", Status: "TODO", EffortMins: 120, Late: -1, Filename: "/x/work.org"}},
		Load:     common.PlanLoad{WindowMins: 180, PlannedMins: 75, FreeMins: 105, DoubleMins: 30, BacklogMins: 120},
	}
	out := Draw(pd, time.Date(2026, 10, 11, 0, 0, 0, 0, time.Local), func(string) string { return "" })
	if bar := Bar(pd, time.Time{}); bar != "█···██▓▓····" {
		t.Errorf("bar %q", bar)
	}
	for _, want := range []string{
		"Mon 12 Oct", "1h15 of 3h planned · 1h45 free · 30m double-booked",
		"09:00-09:15  Standup  appointment",
		"free 09:15-10:00  45m",
		"10:30-11:00  TODO Review  overlaps",
		"free 11:00-12:00  1h",
		"To place (1 · 2h)",
		"2h  TODO Plan sprint  work.org",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
}
