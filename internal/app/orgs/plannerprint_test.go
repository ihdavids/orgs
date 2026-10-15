package orgs

import (
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/ihdavids/orgs/cmd/oc/commands"
)

func plDate(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.ParseInLocation("2006-01-02", s, time.Local)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestPlannerWeekStart(t *testing.T) {
	cases := []struct {
		day   string
		start time.Weekday
		want  string
	}{
		{"2026-10-14", time.Monday, "2026-10-12"}, // a Wednesday
		{"2026-10-12", time.Monday, "2026-10-12"}, // Monday is its own start
		{"2026-10-18", time.Monday, "2026-10-12"}, // Sunday ends a Monday week
		{"2026-10-18", time.Sunday, "2026-10-18"},
		{"2026-10-14", time.Sunday, "2026-10-11"},
		{"2026-11-01", time.Monday, "2026-10-26"}, // across a month
	}
	for _, c := range cases {
		if got := weekStart(plDate(t, c.day), c.start).Format("2006-01-02"); got != c.want {
			t.Errorf("weekStart(%s, %v) = %s, want %s", c.day, c.start, got, c.want)
		}
	}
}

func TestPlannerMonthGrid(t *testing.T) {
	// October 2026 starts on a Thursday and ends on a Saturday.
	g := monthGrid(plDate(t, "2026-10-20"), time.Monday)
	if len(g) != 5 {
		t.Fatalf("%d weeks, want 5", len(g))
	}
	if first, last := g[0][0].Format("2006-01-02"), g[4][6].Format("2006-01-02"); first != "2026-09-28" || last != "2026-11-01" {
		t.Errorf("grid runs %s to %s", first, last)
	}
	// A month starting on a Sunday needs six Monday rows.
	if g := monthGrid(plDate(t, "2026-03-01"), time.Monday); len(g) != 6 {
		t.Errorf("March 2026 from Monday: %d weeks, want 6", len(g))
	}
	if g := monthGrid(plDate(t, "2026-03-01"), time.Sunday); len(g) != 5 || g[0][0].Format("2006-01-02") != "2026-03-01" {
		t.Errorf("March 2026 from Sunday: %d weeks starting %s", len(g), g[0][0].Format("2006-01-02"))
	}
}

func TestPlannerGridHours(t *testing.T) {
	if lo, hi := gridHours(nil, 7, 21); lo != 7 || hi != 21 {
		t.Errorf("empty: %d-%d", lo, hi)
	}
	es := []plannerEntry{{Start: 6*60 + 30, Stop: 7 * 60}, {Start: 21*60 + 15}}
	if lo, hi := gridHours(es, 7, 21); lo != 6 || hi != 22 {
		t.Errorf("widened: %d-%d, want 6-22", lo, hi)
	}
}

func TestPlannerPlaceTimed(t *testing.T) {
	es := []plannerEntry{
		{Title: "a", Start: 9 * 60, Stop: 10 * 60},
		{Title: "b", Start: 9*60 + 30, Stop: 11 * 60},
		{Title: "c", Start: 10 * 60, Stop: 10*60 + 30}, // a's lane is free again
		{Title: "d", Start: 12 * 60},                   // no end: half an hour, alone
		{Title: "e", Start: 5 * 60, Stop: 6 * 60},      // off the grid
	}
	slots := placeTimed(es, 8, 18)
	if len(slots) != 4 {
		t.Fatalf("%d slots, want 4 (e is before the grid)", len(slots))
	}
	by := map[string]plannerSlot{}
	for _, s := range slots {
		by[s.Entry.Title] = s
	}
	near := func(a, b float64) bool { return a-b < 0.01 && b-a < 0.01 }
	if s := by["a"]; !near(s.Top, 10) || !near(s.Height, 10) || !near(s.Width, 50) || !near(s.Left, 0) {
		t.Errorf("a: %+v", s)
	}
	if s := by["b"]; !near(s.Left, 50) || !near(s.Width, 50) || !near(s.Height, 15) {
		t.Errorf("b: %+v", s)
	}
	if s := by["c"]; !near(s.Left, 0) || !near(s.Width, 50) {
		t.Errorf("c should reuse a's lane: %+v", s)
	}
	if s := by["d"]; !near(s.Width, 100) || !near(s.Height, 5) || !near(s.Top, 40) {
		t.Errorf("d: %+v", s)
	}
	if !strings.Contains(string(by["a"].Style), "top:10.000%") {
		t.Errorf("style %q", by["a"].Style)
	}
}

func TestPlannerTitle(t *testing.T) {
	cases := map[string]string{
		"Meeting <2026-10-12 Mon 10:00>":             "Meeting",
		"Read [[https://x.org][the paper]] [1/3]":    "Read the paper",
		"Trip <2026-10-12 Mon>--<2026-10-14 Wed> ok": "Trip ok",
		"See [[file:notes.org]]":                     "See notes.org",
	}
	for in, want := range cases {
		if got := plannerTitle(in); got != want {
			t.Errorf("plannerTitle(%q) = %q, want %q", in, got, want)
		}
	}
}

const plannerSample = `#+TODO: TODO NEXT | DONE CANCELLED
* Work
** NEXT Write the quarterly report
** TODO [#A] Fix the build
** TODO Weekly sync
   SCHEDULED: <2026-10-12 Mon 10:00 +1w>
** TODO Dentist
   <2026-10-14 Wed 09:00-10:00>
** TODO Overlapping call
   <2026-10-14 Wed 09:30-11:00>
** TODO Send the invoice
   DEADLINE: <2026-10-16 Fri>
** TODO Renew the passport
   SCHEDULED: <2026-10-05 Mon>
** DONE Book the venue
   CLOSED: [2026-10-13 Tue 09:00] SCHEDULED: <2026-10-13 Tue>
* Habits
** TODO Run
   SCHEDULED: <2026-10-14 Wed .+1d>
   :PROPERTIES:
   :STYLE:    habit
   :END:
   :LOGBOOK:
   - State "DONE"       from "TODO"       [2026-10-12 Mon 07:00]
   :END:
`

func plannerGet(t *testing.T, query string) (int, string) {
	t.Helper()
	r := httptest.NewRequest("GET", "/agenda/print?"+query, nil)
	w := httptest.NewRecorder()
	RequestAgendaPrint(w, r)
	return w.Code, w.Body.String()
}

func TestPlannerPrintWeek(t *testing.T) {
	loadOrg(t, map[string]string{"plan.org": plannerSample})
	code, page := plannerGet(t, "view=week&from=2026-10-14&today=2026-10-14")
	if code != 200 {
		t.Fatalf("%d: %s", code, page)
	}
	for _, want := range []string{
		"@page { size: A4 landscape;",
		"Week 42", "October 2026",
		"Weekly sync", "Dentist", "Send the invoice",
		`class="mark due">due</span>`,      // the deadline on its own day
		`class="mark warn">in 2d</span>`,   // and its warning on today
		`class="mark late">late 9d</span>`, // the passport, carried to today, in Overdue
		"<h3>Tasks</h3>", "Write the quarterly report", "Fix the build",
		"<h3>Habits</h3>", `<span class="tick done">`, `<span class="tick miss">`,
		`item kind-scheduled done`, // Book the venue
	} {
		if !strings.Contains(page, want) {
			t.Errorf("week page lacks %q", want)
		}
	}
	// The overdue item is in the panel, not also in today's column.
	if n := strings.Count(page, "Renew the passport"); n != 1 {
		t.Errorf("Renew the passport appears %d times, want once (Overdue)", n)
	}
	// Active tasks first: NEXT before the [#A] TODO.
	if strings.Index(page, "Write the quarterly report") > strings.Index(page, "Fix the build") {
		t.Errorf("NEXT should come before the other tasks")
	}
	plannerSave(t, "week-classic.html", page)
}

func TestPlannerPrintDayAndMonth(t *testing.T) {
	loadOrg(t, map[string]string{"plan.org": plannerSample})
	code, page := plannerGet(t, "view=day&from=2026-10-14&today=2026-10-14&paper=letter&theme=modern")
	if code != 200 {
		t.Fatalf("%d: %s", code, page)
	}
	for _, want := range []string{"@page { size: Letter portrait;", "Wednesday", "14 October 2026", `class="event kind-timestamp`, "09:00–10:00", "<h3>Overdue</h3>"} {
		if !strings.Contains(page, want) {
			t.Errorf("day page lacks %q", want)
		}
	}
	// Dentist and the overlapping call share the width.
	if !strings.Contains(page, "width:50.000%") {
		t.Errorf("overlapping events should be side by side")
	}
	plannerSave(t, "day-modern.html", page)

	code, page = plannerGet(t, "view=month&from=2026-10-14&today=2026-10-14&theme=bullet&weekstart=sun")
	if code != 200 {
		t.Fatalf("%d: %s", code, page)
	}
	for _, want := range []string{"<div class=\"wd\">Sunday</div>", "October", `class="cell today`, "Send the invoice"} {
		if !strings.Contains(page, want) {
			t.Errorf("month page lacks %q", want)
		}
	}
	plannerSave(t, "month-bullet.html", page)
}

func TestPlannerPrintPagesAndRefusals(t *testing.T) {
	loadOrg(t, map[string]string{"plan.org": plannerSample})
	_, page := plannerGet(t, "view=week&from=2026-10-14&days=14&theme=minimal&today=2026-10-14")
	if n := strings.Count(page, `<section class="sheet week">`); n != 2 {
		t.Errorf("days=14: %d week sheets, want 2", n)
	}
	plannerSave(t, "weeks-minimal.html", page)
	for _, q := range []string{"view=year", "paper=a3", "theme=nope", "from=tomorrow", "pages=0", "weekstart=wed"} {
		if code, _ := plannerGet(t, q); code != 400 {
			t.Errorf("%s: %d, want 400", q, code)
		}
	}
	_, list := plannerGet(t, "list=t")
	for _, th := range []string{"bullet", "classic", "minimal", "modern"} {
		if !strings.Contains(list, th) {
			t.Errorf("themes list lacks %s: %q", th, list)
		}
	}
}

// PLANNER_OUT=dir go test ... keeps the pages, to look at in a browser.
func plannerSave(t *testing.T, name, page string) {
	dir := os.Getenv("PLANNER_OUT")
	if dir == "" {
		return
	}
	os.MkdirAll(dir, 0o755)
	if err := os.WriteFile(filepath.Join(dir, name), []byte(page), 0o644); err != nil {
		t.Error(err)
	}
}

// Every theme in every view, for looking at: PLANNER_OUT=dir only.
func TestPlannerGallery(t *testing.T) {
	if os.Getenv("PLANNER_OUT") == "" {
		t.Skip("PLANNER_OUT not set")
	}
	loadOrg(t, map[string]string{"plan.org": plannerSample + `* Calendar
** Standup
   <2026-10-12 Mon 09:15 +1d>
** Lunch with Sam
   <2026-10-15 Thu 12:30-13:30>
** Conference
   <2026-10-16 Fri>--<2026-10-17 Sat>
** TODO Call the bank about the mortgage renewal paperwork
   SCHEDULED: <2026-10-15 Thu 15:00>
** TODO [#B] Review pull requests
   SCHEDULED: <2026-10-14 Wed 14:00-15:30>
** TODO Plan the offsite
   DEADLINE: <2026-10-28 Wed>
`})
	for _, th := range PlannerThemes() {
		for _, v := range []string{"week", "day", "month"} {
			code, page := plannerGet(t, "view="+v+"&theme="+th+"&from=2026-10-14&today=2026-10-14")
			if code != 200 {
				t.Fatalf("%s %s: %d %s", th, v, code, page)
			}
			plannerSave(t, "g-"+v+"-"+th+".html", page)
		}
	}
}

// Printed through a headless Chrome, a sheet is exactly one page: PLANNER_OUT
// and a Chrome on this machine only.
func TestPlannerPrintsOnePagePerSheet(t *testing.T) {
	dir := os.Getenv("PLANNER_OUT")
	if dir == "" || commands.FindBrowser() == "" {
		t.Skip("PLANNER_OUT not set, or no Chrome here")
	}
	loadOrg(t, map[string]string{"plan.org": plannerSample})
	cases := []struct {
		query string
		pages int
	}{
		{"view=week&pages=2&theme=classic", 2},
		{"view=day&pages=3&theme=minimal&paper=letter", 3},
		{"view=month&theme=bullet", 1},
		{"view=week&theme=modern&paper=letter", 1},
	}
	for i, c := range cases {
		code, page := plannerGet(t, c.query+"&from=2026-10-14&today=2026-10-14")
		if code != 200 {
			t.Fatalf("%s: %d %s", c.query, code, page)
		}
		pdf, err := commands.PrintPDF([]byte(page))
		if err != nil {
			t.Fatal(err)
		}
		plannerSave(t, fmt.Sprintf("print-%d.pdf", i), string(pdf))
		if n := pdfPages(pdf); n != c.pages {
			t.Errorf("%s: %d pages, want %d", c.query, n, c.pages)
		}
	}
}

var pdfPageRe = regexp.MustCompile(`/Type\s*/Page[^s]`)

func pdfPages(b []byte) int { return len(pdfPageRe.FindAll(b, -1)) }
