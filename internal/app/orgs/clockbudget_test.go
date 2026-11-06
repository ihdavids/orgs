package orgs

import (
	"strings"
	"testing"
	"time"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

func TestParseBudget(t *testing.T) {
	cases := map[string]float64{"5:00": 300, "0:45": 45, "300": 300, "5h": 300, "90m": 90, "1h30m": 90, "1.5h": 90, "1h 30m": 90, " 2:05 ": 125}
	for in, want := range cases {
		if got, ok := parseBudget(in); !ok || got != want {
			t.Errorf("parseBudget(%q) = %v, %v; want %v", in, got, ok, want)
		}
	}
	for _, bad := range []string{"", "five", "5:75", "h", "1d"} {
		if _, ok := parseBudget(bad); ok {
			t.Errorf("parseBudget(%q) read", bad)
		}
	}
}

func TestBudgetRangeAndPace(t *testing.T) {
	ref := time.Date(2026, 11, 4, 12, 0, 0, 0, time.Local) // a Wednesday
	want := map[string][2]string{
		"day":     {"2026-11-04", "2026-11-05"},
		"week":    {"2026-11-02", "2026-11-09"},
		"month":   {"2026-11-01", "2026-12-01"},
		"quarter": {"2026-10-01", "2027-01-01"},
		"year":    {"2026-01-01", "2027-01-01"},
	}
	for p, w := range want {
		r := budgetRange(p, ref)
		if r.From.Format("2006-01-02") != w[0] || r.To.Format("2006-01-02") != w[1] {
			t.Errorf("%s: %s..%s, want %v", p, r.From.Format("2006-01-02"), r.To.Format("2006-01-02"), w)
		}
	}
	if got := budgetPace(budgetRange("day", ref), ref); got != 0.5 {
		t.Errorf("pace at noon = %v", got)
	}
	r := budgetRange("week", ref)
	if budgetPace(r, r.From.Add(-time.Hour)) != 0 || budgetPace(r, r.To.Add(time.Hour)) != 1 {
		t.Error("pace outside the period")
	}
	for in, want := range map[string]string{"w": "week", "BUDGET_MONTH": "month", "quarterly": "quarter", "today": "day"} {
		if got, _ := budgetPeriodOf(in); got != want {
			t.Errorf("budgetPeriodOf(%q) = %q", in, got)
		}
	}
}

func TestBudgetUsedOverSubtree(t *testing.T) {
	text := `* Writing
:PROPERTIES:
:BUDGET_WEEK: 5:00
:BUDGET_MONTH: 600
:END:
:LOGBOOK:
CLOCK: [2026-11-02 Mon 09:00]--[2026-11-02 Mon 11:00] =>  2:00
CLOCK: [2026-10-30 Fri 09:00]--[2026-10-30 Fri 10:00] =>  1:00
:END:
** Chapter one
:LOGBOOK:
CLOCK: [2026-11-03 Tue 09:00]--[2026-11-03 Tue 11:30] =>  2:30
:END:
** Chapter two
:LOGBOOK:
CLOCK: [2026-11-04 Wed 10:00]
:END:
* Reading
:PROPERTIES:
:BUDGET_WEEK: nonsense
:END:
`
	withFeatures(t, func(f *common.FeatureSettings) { f.ClockBudget = true })
	old := recapActiveClock
	recapActiveClock = func() (*org.Section, time.Time) { return nil, time.Time{} }
	t.Cleanup(func() { recapActiveClock = old })
	paths := loadOrg(t, map[string]string{"b.org": text})
	f, sec := secAt(t, paths["b.org"], "Writing")
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.Local)
	u := &budgetUsage{cache: lineCache{}, now: now}

	week, ok := u.item(f, sec, "week", now)
	if !ok {
		t.Fatal("no week budget")
	}
	// 2:00 Monday + 2:30 Tuesday + the open clock since 10:00 = 6:30.
	if week.Used != 390 || !week.Over || !week.Running || week.Remaining != -90 {
		t.Errorf("week = %+v", week)
	}
	month, _ := u.item(f, sec, "month", now)
	// October's hour is not November's.
	if month.Used != 390 || month.Over || month.Budget != 600 || !month.Ahead {
		t.Errorf("month = %+v", month)
	}
	if _, ok := u.item(f, sec, "year", now); ok {
		t.Error("a year budget nobody gave")
	}
	_, reading := secAt(t, paths["b.org"], "Reading")
	if hasAnyBudget(reading) {
		t.Error("an unreadable budget counted")
	}
	if ok, _ := queryHasBudget(sec, []interface{}{"week"}); !ok {
		t.Error(`HasBudget("week") false`)
	}
	if ok, _ := queryHasBudget(sec, []interface{}{"day"}); ok {
		t.Error(`HasBudget("day") true`)
	}

	withFeatures(t, func(f *common.FeatureSettings) { f.ClockBudget = false })
	if ok, _ := queryHasBudget(sec, nil); ok {
		t.Error("HasBudget with the feature off")
	}
}

func TestBudgetSetWritesTheProperty(t *testing.T) {
	withFeatures(t, func(f *common.FeatureSettings) { f.ClockBudget = true })
	paths := loadOrg(t, map[string]string{"s.org": "* Project\nSome text\n"})
	f, sec := secAt(t, paths["s.org"], "Project")
	if err := setHeadingProperty(f, sec, budgetProperty("week"), clockHM(90)); err != nil {
		t.Fatal(err)
	}
	if got := readOrg(t, paths["s.org"]); !strings.Contains(got, ":BUDGET_WEEK: 1:30") {
		t.Fatalf("got\n%s", got)
	}
}
