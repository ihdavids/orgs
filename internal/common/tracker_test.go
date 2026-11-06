package common

import (
	"testing"
	"time"
)

// The same cases are in worg's src/tracker.test.ts (trackerLine) and
// vscode-org-everywhere's test/tracker-model.test.ts: a builder writes the
// line the server reads.
var trackerLineCases = []struct {
	opts []TrackerOption
	line string
}{
	{[]TrackerOption{{"source", "property:MOOD"}, {"type", "heatmap"}, {"from", "-1y"}}, `source:property:MOOD type:heatmap from:-1y`},
	{[]TrackerOption{{"source", "query:IsDone()"}, {"type", "bar"}}, `source:query:"IsDone()" type:bar`},
	{[]TrackerOption{{"source", "habit:Morning run"}, {"title", "My runs"}}, `source:habit:"Morning run" title:"My runs"`},
	{[]TrackerOption{{"source", "words"}, {"goal", "500"}}, `source:words goal:500`},
	{[]TrackerOption{{"source", "clock:Writing"}, {"color", "#2a78d6"}, {"agg", "sum"}}, `source:clock:Writing color:#2a78d6 agg:sum`},
	{[]TrackerOption{{"title", `say "hi"`}, {"source", "tag:gym"}}, `title:"say \"hi\"" source:tag:gym`},
}

func TestTrackerLineRoundTrip(t *testing.T) {
	for _, c := range trackerLineCases {
		if got := FormatTrackerOptions(c.opts); got != c.line {
			t.Errorf("format %v = %s, want %s", c.opts, got, c.line)
		}
		back := ParseTrackerOptions(c.line)
		if len(back) != len(c.opts) {
			t.Fatalf("parse %s = %v", c.line, back)
		}
		for i := range back {
			if back[i] != c.opts[i] {
				t.Errorf("parse %s [%d] = %v, want %v", c.line, i, back[i], c.opts[i])
			}
		}
	}
}

func TestTrackerOptionSpellings(t *testing.T) {
	// A dynamic block's org spelling, and the kind given as its own key.
	o := ParseTrackerOptions(`:source property:MOOD :from -30d stray words habit:Run`)
	if TrackerGet(o, "source", "") != "property:MOOD" || TrackerGet(o, "from", "") != "-30d" {
		t.Errorf("block spelling: %v", o)
	}
	if k, a, ok := TrackerSourceOf(ParseTrackerOptions(`habit:"Morning run" type:line`)); !ok || k != "habit" || a != "Morning run" {
		t.Errorf("habit: = %s %s %v", k, a, ok)
	}
	if k, _, ok := TrackerSourceOf(ParseTrackerOptions(`source:nonsense:x`)); ok || k != "nonsense" {
		t.Errorf("unknown source read")
	}
	if v := TrackerGet(ParseTrackerOptions(`deps:(2 3) title:x`), "deps", ""); v != "(2 3)" {
		t.Errorf("list = %q", v)
	}
}

func TestTrackerValue(t *testing.T) {
	cases := []struct {
		in   string
		v    float64
		kind string
	}{
		{"4", 4, "number"}, {"72.5kg", 72.5, "number"}, {"8,000", 8000, "number"}, {"-3", -3, "number"},
		{"1:30", 90, "duration"}, {"1h30m", 90, "duration"}, {"45m", 45, "duration"}, {"2h", 120, "duration"},
		{"yes", 1, "bool"}, {"t", 1, "bool"}, {"[X]", 1, "bool"}, {"no", 0, "bool"}, {"[ ]", 0, "bool"},
		{"★★★☆☆", 3, "rating"}, {"⭐⭐", 2, "rating"}, {"4/5", 4, "rating"},
	}
	for _, c := range cases {
		v, k, ok := TrackerValue(c.in)
		if !ok || v != c.v || k != c.kind {
			t.Errorf("TrackerValue(%q) = %v %s %v, want %v %s", c.in, v, k, ok, c.v, c.kind)
		}
	}
	for _, bad := range []string{"", "happy", "1:75", "abc 4"} {
		if _, _, ok := TrackerValue(bad); ok {
			t.Errorf("TrackerValue(%q) read", bad)
		}
	}
}

func TestTrackerSummaryAndStreaks(t *testing.T) {
	from := time.Date(2026, 10, 25, 0, 0, 0, 0, time.Local)
	to := time.Date(2026, 11, 4, 0, 0, 0, 0, time.Local)
	vals := map[string][]float64{
		"2026-10-25": {1}, "2026-10-26": {2}, "2026-10-27": {1, 3}, "2026-10-28": {4},
		// a gap on the 29th
		"2026-10-30": {1}, "2026-10-31": {1}, "2026-11-01": {2}, "2026-11-02": {5}, "2026-11-03": {1},
		// nothing yet today, the 4th
	}
	days := TrackerDays(from, to, vals, "sum")
	if len(days) != 11 || days[2].Value != 4 || days[2].N != 2 {
		t.Fatalf("days = %+v", days)
	}
	s := SummarizeTracker(days, nil, "2026-11-04")
	if s.Days != 9 || s.Total != 21 || s.Max != 5 || s.Min != 1 || s.Current != 5 || s.Longest != 5 || s.LongestEnd != "2026-11-03" {
		t.Errorf("summary = %+v", s)
	}
	goal := 2.0
	g := SummarizeTracker(days, &goal, "2026-11-04")
	// 26th, 27th, 28th meet it (3); 1st and 2nd (2), then the 3rd breaks it.
	if g.Longest != 3 || g.Current != 0 {
		t.Errorf("with a goal = %+v", g)
	}
	if mean := TrackerDays(from, to, vals, "mean")[2].Value; mean != 2 {
		t.Errorf("mean = %v", mean)
	}
	if TrackerAggregate([]float64{3, 1, 2}, "last") != 2 || TrackerAggregate([]float64{3, 1, 2}, "count") != 3 {
		t.Error("last/count")
	}
}

func TestTrackerLevelAndFormat(t *testing.T) {
	if TrackerLevel(0, 0, 0, 10) != 0 || TrackerLevel(0, 1, 0, 10) != 0 || TrackerLevel(1, 1, 0, 10) != 1 ||
		TrackerLevel(5, 1, 0, 10) != 3 || TrackerLevel(10, 1, 0, 10) != 4 || TrackerLevel(3, 1, 3, 3) != 4 {
		t.Error("levels")
	}
	for v, want := range map[float64]string{90: "1:30", 5: "0:05"} {
		if got := TrackerFormat(v, "duration"); got != want {
			t.Errorf("duration %v = %s", v, got)
		}
	}
	if TrackerFormat(3, "rating") != "★★★" || TrackerFormat(2.345, "number") != "2.35" || TrackerFormat(1, "bool") != "yes" {
		t.Error("format")
	}
	if got := Sparkline([]TrackerDay{{N: 1, Value: 0}, {N: 0}, {N: 1, Value: 7}}); got != "▁ █" {
		t.Errorf("sparkline %q", got)
	}
}
