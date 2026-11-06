package orgs

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ihdavids/orgs/internal/common"
)

var trackerNow = time.Date(2026, 11, 4, 15, 0, 0, 0, time.Local) // a Wednesday

func trackerSetup(t *testing.T) map[string]string {
	t.Helper()
	withFeatures(t, func(f *common.FeatureSettings) { f.Tracker = true })
	return loadOrg(t, map[string]string{
		// A day page with its own drawer, one with a #+PROPERTY: line.
		"daily/Wed_2026_11_04.org": ":PROPERTIES:\n:MOOD: 4\n:END:\n#+TITLE: Wed\n* Notes\n",
		"daily/2026-11-03.org":     "#+PROPERTY: MOOD 2\n* Notes\n",
		"journal.org": `* 2026
** 2026-11
*** 2026-11-02 Monday
:PROPERTIES:
:MOOD: ★★★☆☆
:END:
**** Lunch
:PROPERTIES:
:MOOD: 5
:END:
* Created note
:PROPERTIES:
:CREATED: [2026-11-01 Sun 10:00]
:MOOD: 1
:END:
* Long ago
:PROPERTIES:
:CREATED: [2020-01-01 Wed]
:MOOD: 3
:END:
`,
		"tasks.org": `* DONE One :gym:
CLOSED: [2026-11-03 Tue 10:00]
* DONE Two :gym:
CLOSED: [2026-11-03 Tue 18:00]
* DONE Three
CLOSED: [2026-11-01 Sun 09:00]
* TODO Open :gym:
* Run
:PROPERTIES:
:STYLE: habit
:END:
:LOGBOOK:
- State "DONE"       from "TODO"       [2026-11-03 Tue 07:00]
- State "DONE"       from "TODO"       [2026-11-02 Mon 07:00]
- State "DONE"       from "TODO"       [2026-11-02 Mon 19:00]
- State "DONE"       from "TODO"       [2026-10-31 Sat 07:00]
:END:
* Writing
:LOGBOOK:
CLOCK: [2026-11-03 Tue 23:00]--[2026-11-04 Wed 01:30] =>  2:30
CLOCK: [2026-11-02 Mon 09:00]--[2026-11-02 Mon 10:00] =>  1:00
:END:
** Chapter
:LOGBOOK:
CLOCK: [2026-11-02 Mon 14:00]--[2026-11-02 Mon 14:45] =>  0:45
:END:
* Weight
#+NAME: weight
| Day              | Kg   |
|------------------+------|
| [2026-11-01 Sun] | 80.5 |
| [2026-11-03 Tue] | 80.1 |
| 2026-11-04       | 79.8 |

#+TRACKER: source:property:MOOD type:heatmap from:-1y
#+TRACKER: source:table:weight type:line title:"Weight"

#+begin_example
#+TRACKER: source:tag:nothing
#+end_example
`,
	})
}

func trackerDays(t *testing.T, line string) map[string]common.TrackerDay {
	t.Helper()
	s, _, err := trackerFor(line, "", trackerNow)
	if err != nil {
		t.Fatalf("%s: %v", line, err)
	}
	out := map[string]common.TrackerDay{}
	for _, d := range s.Days {
		if d.N > 0 {
			out[d.Date] = d
		}
	}
	return out
}

func TestTrackerProperty(t *testing.T) {
	trackerSetup(t)
	got := trackerDays(t, "source:property:MOOD from:2026-10-01 agg:mean")
	want := map[string]float64{
		"2026-11-04": 4,             // the day page's own drawer
		"2026-11-03": 2,             // #+PROPERTY: on an org-roam style day page
		"2026-11-02": (3 + 5) / 2.0, // the datetree day, and a heading under it
		"2026-11-01": 1,             // CREATED
	}
	if len(got) != len(want) {
		t.Errorf("days = %+v", got)
	}
	for d, v := range want {
		if got[d].Value != v {
			t.Errorf("%s = %+v, want %v", d, got[d], v)
		}
	}
	if got["2026-11-04"].File == "" {
		t.Error("no file to open for the day")
	}
}

func TestTrackerCountsHabitsClockAndTables(t *testing.T) {
	trackerSetup(t)
	tags := trackerDays(t, "source:tag:gym from:-1w")
	if len(tags) != 1 || tags["2026-11-03"].Value != 2 {
		t.Errorf("tag = %+v", tags)
	}
	q := trackerDays(t, `source:query:"IsDone()" from:-1w`)
	if q["2026-11-03"].Value != 2 || q["2026-11-01"].Value != 1 {
		t.Errorf("query = %+v", q)
	}
	h := trackerDays(t, "source:habit:Run from:-1w")
	if len(h) != 3 || h["2026-11-02"].Value != 1 {
		t.Errorf("habit = %+v (a day ticked twice is one day)", h)
	}
	c := trackerDays(t, "source:clock:Writing from:-1w")
	// Cut at midnight; the child counts.
	if c["2026-11-03"].Value != 60 || c["2026-11-04"].Value != 90 || c["2026-11-02"].Value != 105 {
		t.Errorf("clock = %+v", c)
	}
	w := trackerDays(t, "source:table:weight from:-1w")
	if w["2026-11-01"].Value != 80.5 || w["2026-11-04"].Value != 79.8 || len(w) != 3 {
		t.Errorf("table = %+v", w)
	}
	s, _, _ := trackerFor("source:habit:Run from:-1w", "", trackerNow)
	// 31st, (1st missed), 2nd, 3rd: a run of two up to yesterday, today not over.
	if s.Summary.Current != 2 || s.Summary.Longest != 2 || s.Kind != "count" {
		t.Errorf("habit summary = %+v kind %s", s.Summary, s.Kind)
	}
}

func TestTrackerSpecErrors(t *testing.T) {
	trackerSetup(t)
	for _, bad := range []string{"type:heatmap", "source:mood:x", "source:property:", "source:tag:x type:pie", "source:tag:x agg:median", "source:tag:x from:someday"} {
		if _, err := parseTrackerSpec(bad, trackerNow); err == nil {
			t.Errorf("%q read", bad)
		}
	}
	s, err := parseTrackerSpec("source:tag:x", trackerNow)
	if err != nil || s.From.Format("2006-01-02") != "2025-11-05" || s.To.Format("2006-01-02") != "2026-11-04" {
		t.Errorf("heatmap default range %v..%v %v", s.From, s.To, err)
	}
	if _, _, err := trackerFor("source:habit:Nobody", "", trackerNow); err == nil {
		t.Error("a habit nobody has")
	}
}

func TestTrackerListSkipsExamples(t *testing.T) {
	trackerSetup(t)
	ls := trackerList()
	if len(ls) != 2 || ls[1].Title != "Weight" || ls[0].Title != "MOOD" || ls[0].Headline != "Weight" {
		t.Errorf("list = %+v", ls)
	}
}

func TestTrackerExportCopy(t *testing.T) {
	paths := trackerSetup(t)
	lines := strings.Split(readOrg(t, paths["journal.org"]), "\n")
	lines = append(lines, "#+TRACKER: source:property:MOOD from:-1w")
	out, changed := trackerExportLines(paths["journal.org"], lines, "html")
	text := strings.Join(out, "\n")
	if !changed || !strings.Contains(text, "#+BEGIN_EXPORT html") || !strings.Contains(text, `data-date="2026-11-04"`) || strings.Contains(text, "#+TRACKER:") {
		t.Errorf("html copy:\n%s", text)
	}
	out, _ = trackerExportLines(paths["journal.org"], lines, "txt")
	if !strings.Contains(strings.Join(out, "\n"), "MOOD: =") {
		t.Errorf("text copy: %v", out)
	}
	out, _ = trackerExportLines(paths["journal.org"], lines, "latex")
	if out[0] != `#+LATEX_HEADER: \usepackage{tikz}` || !strings.Contains(strings.Join(out, "\n"), `\begin{tikzpicture}`) {
		t.Errorf("latex copy: %v", out[:3])
	}
	withFeatures(t, func(f *common.FeatureSettings) { f.Tracker = false })
	if trackerMayApply(strings.Join(lines, "\n")) {
		t.Error("drawn with the feature off")
	}
}

func TestTrackerBlock(t *testing.T) {
	paths := trackerSetup(t)
	f := GetDb().FindByFile(paths["tasks.org"])
	c := &dynCtx{File: f, Now: trackerNow, Block: dynBlock{Name: "tracker", Header: "source:habit:Run from:-1w"}}
	out, err := dynTracker(c)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(out, "\n")
	for _, want := range []string{"| Date ", "[2026-11-03 Tue]", "| Total ", "| Streak ", "2 (longest 2)"} {
		if !strings.Contains(text, want) {
			t.Errorf("no %q in\n%s", want, text)
		}
	}
}

func TestTrackerLog(t *testing.T) {
	paths := trackerSetup(t)
	dir := filepath.Dir(paths["daily/Wed_2026_11_04.org"])
	old := *Conf().Server
	t.Cleanup(func() { *Conf().Server = old })
	Conf().Server.DayPagePath = dir
	Conf().Server.DayPageMode = ""
	Conf().Server.DayPageTemplate = ""
	Conf().Server.OrgDirs = []string{filepath.Dir(dir)}
	// An existing page: the drawer gets the property beside MOOD.
	res, err := logTracker("SLEEP", "7:30", time.Date(2026, 11, 4, 0, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatal(err)
	}
	got := readOrg(t, res.Filename)
	if !strings.HasPrefix(got, ":PROPERTIES:\n:MOOD:     4\n:SLEEP:    7:30\n:END:\n#+TITLE: Wed") {
		t.Errorf("got\n%s", got)
	}
	// A day with no page yet: one is made.
	res, err = logTracker("MOOD", "3", time.Date(2026, 11, 5, 0, 0, 0, 0, time.Local))
	if err != nil || !res.Created {
		t.Fatal(res, err)
	}
	if got := readOrg(t, res.Filename); got != ":PROPERTIES:\n  :MOOD: 3\n:END:\n#+TITLE: Thu_2026_11_05" {
		t.Errorf("new page\n%q", got)
	}
	// A week page: under the day's heading.
	Conf().Server.DayPageMode = "week"
	Conf().Server.DayPageModeWeekDay = "mon"
	res, err = logTracker("MOOD", "5", time.Date(2026, 11, 4, 0, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatal(err)
	}
	if got := readOrg(t, res.Filename); !strings.HasSuffix(got, "* Wed\n:PROPERTIES:\n  :MOOD: 5\n:END:") {
		t.Errorf("week page\n%s", got)
	}
	if _, err := logTracker("bad name", "1", trackerNow); err == nil {
		t.Error("a property name with a space")
	}
}

func TestTrackerTypstAndInsert(t *testing.T) {
	paths := trackerSetup(t)
	lines := []string{"* A", "#+TRACKER: source:tag:gym from:-1w"}
	out, _ := trackerExportLines(paths["tasks.org"], lines, "typst")
	if !strings.Contains(strings.Join(out, "\n"), `#figure(image(bytes("<svg`) {
		t.Errorf("typst copy: %v", out)
	}
	old := *Conf().Server
	t.Cleanup(func() { *Conf().Server = old })
	Conf().Server.OrgDirs = []string{filepath.Dir(paths["tasks.org"])}
	if _, err := insertTrackerLine(paths["tasks.org"], "source:nonsense:x", -1, trackerNow); err == nil {
		t.Error("a line that is not a tracker went in")
	}
	row, err := insertTrackerLine(paths["tasks.org"], "#+TRACKER: source:tag:gym", 0, trackerNow)
	if err != nil || row != 1 {
		t.Fatal(row, err)
	}
	if got := strings.Split(readOrg(t, paths["tasks.org"]), "\n")[1]; got != "#+TRACKER: source:tag:gym" {
		t.Errorf("line 2 = %q", got)
	}
}
