package orgs

import (
	"strings"
	"testing"
	"time"

	"github.com/ihdavids/orgs/internal/common"
)

// The cells of an org table's lines, trimmed, with a rule as "-" - so a test
// says what a table holds without spelling out its padding.
func tableCells(lines []string) [][]string {
	out := [][]string{}
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if !strings.HasPrefix(l, "|") {
			continue
		}
		if strings.HasPrefix(l, "|-") {
			out = append(out, []string{"-"})
			continue
		}
		row := []string{}
		for _, c := range strings.Split(strings.Trim(l, "|"), "|") {
			row = append(row, strings.TrimSpace(c))
		}
		out = append(out, row)
	}
	return out
}

func wantCells(t *testing.T, got []string, want [][]string) {
	t.Helper()
	cells := tableCells(got)
	if len(cells) != len(want) {
		t.Fatalf("got %d rows, want %d:\n%s", len(cells), len(want), strings.Join(got, "\n"))
	}
	for i := range want {
		if strings.Join(cells[i], "|") != strings.Join(want[i], "|") {
			t.Errorf("row %d: got %q, want %q\n%s", i, cells[i], want[i], strings.Join(got, "\n"))
		}
	}
}

// The lines between a file's first BEGIN and END.
func blockBody(t *testing.T, text string) []string {
	t.Helper()
	lines := strings.Split(text, "\n")
	b := findDynBlocks(lines)
	if len(b) == 0 {
		t.Fatalf("no block in:\n%s", text)
	}
	return lines[b[0].Begin+1 : b[0].End]
}

func TestDynParamsReadTheWayOrgWritesThem(t *testing.T) {
	p := parseDynParams(` :scope file :maxlevel 2 :match "+work-boss" :exclude-tags (a "b c") :indent :q "IsStatus(\"NEXT\")"`)
	want := map[string]string{
		"scope": "file", "maxlevel": "2", "match": "+work-boss",
		"exclude-tags": `(a "b c")`, "indent": "t", "q": `IsStatus("NEXT")`,
	}
	for k, v := range want {
		if p[k] != v {
			t.Errorf("%s: got %q, want %q", k, p[k], v)
		}
	}
	if l := dynList(p["exclude-tags"]); len(l) != 2 || l[1] != "b c" {
		t.Errorf("list: %q", l)
	}
}

// A `#+BEGIN:` in an example is an example: refreshing the guide that shows
// how to write a block must not run it.
func TestABlockInsideASourceBlockIsNotABlock(t *testing.T) {
	lines := strings.Split(`#+BEGIN_SRC org
#+BEGIN: clocktable
#+END:
#+END_SRC
#+BEGIN: insertdatetime
#+END:
#+BEGIN: columnview
no end here`, "\n")
	b := findDynBlocks(lines)
	if len(b) != 1 || b[0].Name != "insertdatetime" || b[0].Begin != 4 || b[0].End != 5 {
		t.Fatalf("got %+v", b)
	}
}

func TestClockRangesAreOrgs(t *testing.T) {
	// A Wednesday.
	now := time.Date(2026, 10, 7, 15, 30, 0, 0, time.Local)
	day := func(s string) time.Time {
		d, _ := time.ParseInLocation("2006-01-02", s, time.Local)
		return d
	}
	cases := []struct{ spec, from, to string }{
		{"today", "2026-10-07", "2026-10-08"},
		{"yesterday", "2026-10-06", "2026-10-07"},
		{"today-3", "2026-10-04", "2026-10-05"},
		// Weeks start on Monday.
		{"thisweek", "2026-10-05", "2026-10-12"},
		{"lastweek", "2026-09-28", "2026-10-05"},
		// A month starts on its first, not the last day of the one before.
		{"thismonth", "2026-10-01", "2026-11-01"},
		{"lastmonth", "2026-09-01", "2026-10-01"},
		{"thisyear", "2026-01-01", "2027-01-01"},
		{"2026-03-15", "2026-03-15", "2026-03-16"},
		{"2026-02", "2026-02-01", "2026-03-01"},
		{"2026-W01", "2025-12-29", "2026-01-05"},
		{"2026-W41", "2026-10-05", "2026-10-12"},
		{"2026-Q3", "2026-07-01", "2026-10-01"},
		{"2025", "2025-01-01", "2026-01-01"},
	}
	for _, c := range cases {
		r, err := clockBlockRange(c.spec, now)
		if err != nil {
			t.Errorf("%s: %v", c.spec, err)
			continue
		}
		if !r.From.Equal(day(c.from)) || !r.To.Equal(day(c.to)) {
			t.Errorf("%s: got %s..%s, want %s..%s", c.spec, r.From.Format("2006-01-02"), r.To.Format("2006-01-02"), c.from, c.to)
		}
	}
	jan := time.Date(2026, 1, 10, 0, 0, 0, 0, time.Local)
	if r, _ := clockBlockRange("lastmonth", jan); !r.From.Equal(day("2025-12-01")) {
		t.Errorf("last month from January: %s", r.From)
	}
	if _, err := clockBlockRange("thisfortnight", now); err == nil {
		t.Errorf("a range nobody knows should be refused, not read as everything")
	}
}

const clockFixture = `#+TITLE: Time
* Project
:LOGBOOK:
CLOCK: [2026-10-06 Tue 09:00]--[2026-10-06 Tue 10:30] =>  1:30
:END:
** Task A                                                              :work:
:LOGBOOK:
CLOCK: [2026-09-28 Mon 23:00]--[2026-09-29 Tue 01:00] =>  2:00
:END:
** Task B
*** Deep
:LOGBOOK:
CLOCK: [2026-10-05 Mon 09:00]--[2026-10-05 Mon 09:15] =>  0:15
:END:
** Nothing clocked
* Report
#+BEGIN: clocktable :scope file :maxlevel 2
old text
#+END:
Text after the block stays.
`

func TestAClockTableIsWrittenBetweenItsLines(t *testing.T) {
	paths := loadOrg(t, map[string]string{"time.org": clockFixture})
	res := UpdateDynBlocks(&common.DynBlockRequest{Filename: paths["time.org"], All: true})
	if !res.Ok {
		t.Fatalf("update: %s", res.Msg)
	}
	text := readOrg(t, paths["time.org"])
	body := blockBody(t, text)
	if !strings.HasPrefix(body[0], "#+CAPTION: Clock summary at [") {
		t.Errorf("caption: %q", body[0])
	}
	// Deep is past maxlevel 2, so it is not listed but its quarter hour still
	// counts towards Task B. Nothing clocked is not listed at all.
	wantCells(t, body, [][]string{
		{"Headline", "Time", ""},
		{"-"},
		{"*Total time*", "*3:45*", ""},
		{"-"},
		{"Project", "3:45", ""},
		{`\_  Task A`, "", "2:00"},
		{`\_  Task B`, "", "0:15"},
	})
	if !strings.HasSuffix(text, "#+END:\nText after the block stays.") {
		t.Errorf("the lines after the block moved:\n%s", text)
	}
	if !strings.Contains(text, "** Task A                                                              :work:") {
		t.Errorf("a heading outside the block was rewritten:\n%s", text)
	}
}

// A clock across the edge of the range counts the part inside it: Task A's
// clock runs from 23:00 on the 28th to 01:00 on the 29th.
func TestAClockTableRangeCountsOnlyWhatIsInside(t *testing.T) {
	text := strings.Replace(clockFixture, ":scope file :maxlevel 2", ":scope file :block 2026-09-29", 1)
	paths := loadOrg(t, map[string]string{"time.org": text})
	if res := UpdateDynBlocks(&common.DynBlockRequest{Filename: paths["time.org"], All: true}); !res.Ok {
		t.Fatalf("update: %s", res.Msg)
	}
	wantCells(t, blockBody(t, readOrg(t, paths["time.org"])), [][]string{
		{"Headline", "Time", ""},
		{"-"},
		{"*Total time*", "*1:00*", ""},
		{"-"},
		{"Project", "1:00", ""},
		{`\_  Task A`, "", "1:00"},
	})
}

func TestAClockTableMatchCountsOnlyMatchingHeadings(t *testing.T) {
	text := strings.Replace(clockFixture, ":scope file :maxlevel 2", `:scope file :match "+work"`, 1)
	paths := loadOrg(t, map[string]string{"time.org": text})
	if res := UpdateDynBlocks(&common.DynBlockRequest{Filename: paths["time.org"], All: true}); !res.Ok {
		t.Fatalf("update: %s", res.Msg)
	}
	wantCells(t, blockBody(t, readOrg(t, paths["time.org"])), [][]string{
		{"Headline", "Time", ""},
		{"-"},
		{"*Total time*", "*2:00*", ""},
		{"-"},
		{"Project", "2:00", ""},
		{`\_  Task A`, "", "2:00"},
	})
}

func TestASubtreeClockTableReportsItsOwnHeading(t *testing.T) {
	text := `* Project
** Task A
:LOGBOOK:
CLOCK: [2026-09-28 Mon 09:00]--[2026-09-28 Mon 10:00] =>  1:00
:END:
#+BEGIN: clocktable :scope subtree
#+END:
* Elsewhere
:LOGBOOK:
CLOCK: [2026-09-28 Mon 11:00]--[2026-09-28 Mon 12:00] =>  1:00
:END:
`
	paths := loadOrg(t, map[string]string{"s.org": text})
	if res := UpdateDynBlocks(&common.DynBlockRequest{Filename: paths["s.org"], All: true}); !res.Ok {
		t.Fatalf("update: %s", res.Msg)
	}
	wantCells(t, blockBody(t, readOrg(t, paths["s.org"])), [][]string{
		{"Headline", "Time"},
		{"-"},
		{"*Total time*", "*1:00*"},
		{"-"},
		{"Task A", "1:00"},
	})
}

const columnsFixture = `#+COLUMNS: %ITEM %EFFORT{:}
* Plan
#+BEGIN: columnview :id local :indent t
#+END:
** Design
:PROPERTIES:
:EFFORT:   2:00
:END:
** Build
:PROPERTIES:
:EFFORT:   3:30
:END:
* Unrelated
:PROPERTIES:
:EFFORT:   9:00
:END:
`

func TestAColumnViewBlockShowsItsSubtreeWithTheRollup(t *testing.T) {
	paths := loadOrg(t, map[string]string{"c.org": columnsFixture})
	if res := UpdateDynBlocks(&common.DynBlockRequest{Filename: paths["c.org"], All: true}); !res.Ok {
		t.Fatalf("update: %s", res.Msg)
	}
	wantCells(t, blockBody(t, readOrg(t, paths["c.org"])), [][]string{
		{"Item", "Effort"},
		{"-"},
		{"Plan", "5:30"},
		{`\_  Design`, "2:00"},
		{`\_  Build`, "3:30"},
	})
}

func TestAColumnViewKeepsAndRunsItsFormulas(t *testing.T) {
	text := strings.Replace(columnsFixture, "#+BEGIN: columnview :id local :indent t\n#+END:",
		"#+BEGIN: columnview :id global :format \"%ITEM %N %DOUBLE\"\n#+TBLFM: $3=$2*2\n#+END:", 1)
	text = strings.Replace(text, ":EFFORT:   2:00", ":EFFORT:   2:00\n:N: 2", 1)
	text = strings.Replace(text, ":EFFORT:   3:30", ":EFFORT:   3:30\n:N: 5", 1)
	paths := loadOrg(t, map[string]string{"c.org": text})
	if res := UpdateDynBlocks(&common.DynBlockRequest{Filename: paths["c.org"], All: true}); !res.Ok {
		t.Fatalf("update: %s", res.Msg)
	}
	body := blockBody(t, readOrg(t, paths["c.org"]))
	if last := body[len(body)-1]; strings.TrimSpace(last) != "#+TBLFM: $3=$2*2" {
		t.Errorf("the formula line went missing:\n%s", strings.Join(body, "\n"))
	}
	cells := tableCells(body)
	got := map[string]string{}
	for _, r := range cells {
		if len(r) == 3 {
			got[r[0]] = r[2]
		}
	}
	if got["Design"] != "4" || got["Build"] != "10" {
		t.Errorf("the formula did not run over the new rows:\n%s", strings.Join(body, "\n"))
	}
}

func TestAQueryBlockListsWhatTheQueryFinds(t *testing.T) {
	paths := loadOrg(t, map[string]string{"q.org": `* TODO Write it
* DONE Shipped it
* TODO Test it
#+BEGIN: query :q "IsStatus(\"TODO\")" :columns "TODO ITEM" :scope file
#+END:
`})
	if res := UpdateDynBlocks(&common.DynBlockRequest{Filename: paths["q.org"], All: true}); !res.Ok {
		t.Fatalf("update: %s", res.Msg)
	}
	wantCells(t, blockBody(t, readOrg(t, paths["q.org"])), [][]string{
		{"Todo", "Item"},
		{"-"},
		{"TODO", "Write it"},
		{"TODO", "Test it"},
	})
}

// A block with a name nobody knows is reported and left as it was, and the
// blocks beside it are still refreshed.
func TestAnUnknownBlockIsReportedNotEmptied(t *testing.T) {
	paths := loadOrg(t, map[string]string{"u.org": `* H
#+BEGIN: mystery :x 1
keep me
#+END:
#+BEGIN: insertdatetime :format "%Y"
#+END:
`})
	res := UpdateDynBlocks(&common.DynBlockRequest{Filename: paths["u.org"], All: true})
	if res.Ok || !strings.Contains(res.Msg, "mystery") {
		t.Errorf("the failure should name the block: %+v", res)
	}
	text := readOrg(t, paths["u.org"])
	if !strings.Contains(text, "keep me") {
		t.Errorf("the unknown block was emptied:\n%s", text)
	}
	if !strings.Contains(text, "#+BEGIN: insertdatetime :format \"%Y\"\n"+time.Now().Format("2006")+"\n#+END:") {
		t.Errorf("the block beside it was not refreshed:\n%s", text)
	}
}

// Asking for one block by a line inside it refreshes that block only.
func TestRefreshingByLineTouchesOneBlock(t *testing.T) {
	paths := loadOrg(t, map[string]string{"l.org": `* H
#+BEGIN: insertdatetime :format "first"
#+END:
#+BEGIN: insertdatetime :format "second"
#+END:
`})
	res := UpdateDynBlocks(&common.DynBlockRequest{Filename: paths["l.org"], Line: 4})
	if !res.Ok {
		t.Fatalf("update: %s", res.Msg)
	}
	text := readOrg(t, paths["l.org"])
	if strings.Contains(text, "\nfirst\n") || !strings.Contains(text, "\nsecond\n") {
		t.Errorf("wrong block refreshed:\n%s", text)
	}
}
