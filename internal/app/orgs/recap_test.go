package orgs

import (
	"strings"
	"testing"
	"time"

	"github.com/ihdavids/go-org/org"
)

const recapSample = `#+TODO: TODO NEXT WAITING | DONE CANCELLED
#+NEXT: NEXT
* Website :project:
** DONE Ship the [[https://example.com][landing page]]
   CLOSED: [2026-10-12 Mon 16:00]
   :LOGBOOK:
   - State "DONE"       from "NEXT"       [2026-10-12 Mon 16:00]
   CLOCK: [2026-10-07 Wed 23:00]--[2026-10-08 Thu 01:00] =>  2:00
   CLOCK: [2026-10-12 Mon 09:00]--[2026-10-12 Mon 10:30] =>  1:30
   :END:
** NEXT Write the *pricing* copy [1/3]
   :LOGBOOK:
   CLOCK: [2026-10-14 Wed 11:00]
   :END:
** WAITING Logo from the designer
   :LOGBOOK:
   - State "WAITING"    from "NEXT"       [2026-10-10 Sat 09:00]
   :END:
** CANCELLED Animated hero
   CLOSED: [2026-10-09 Fri 12:00]
* TODO Water the plants
  SCHEDULED: <2026-10-17 Sat +1w>
  :LOGBOOK:
  - State "DONE"       from "TODO"       [2026-10-10 Sat 10:00]
  - State "DONE"       from "TODO"       [2026-10-03 Sat 10:00]
  :END:
* DONE Old thing
  CLOSED: [2026-10-01 Thu 10:00]
* TODO Exercise
  SCHEDULED: <2026-10-15 Thu .+1d>
  :PROPERTIES:
  :STYLE: habit
  :END:
  :LOGBOOK:
  - State "DONE"       from "TODO"       [2026-10-13 Tue 07:00]
  - State "DONE"       from "TODO"       [2026-10-12 Mon 07:00]
  :END:
* TODO Dentist <2026-10-14 Wed 15:00>
* TODO Tax return
  SCHEDULED: <2026-10-11 Sun>
* COMMENT Drafts
** DONE Hidden
   CLOSED: [2026-10-12 Mon 10:00]
* Old work :ARCHIVE:
** DONE Archived
   CLOSED: [2026-10-12 Mon 10:00]
`

// One walk answers every part, by the rules of the views each part comes
// from: CLOSED and the logbook's DONE lines both count (a repeating task never
// keeps a CLOSED), clocks are cut to the range, a clock with no end is the
// running one, waiting is counted from its logbook line, and nothing archived
// or under a COMMENT heading is news.
func TestRecapOneWalk(t *testing.T) {
	loadOrg(t, map[string]string{"work.org": recapSample})
	old := recapActiveClock
	recapActiveClock = func() (*org.Section, time.Time) { return nil, time.Time{} }
	defer func() { recapActiveClock = old }()
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.Local)
	from := time.Date(2026, 10, 8, 0, 0, 0, 0, time.Local)
	r := buildRecap(from, now, now, "nobody")

	closed := map[string]int{}
	for _, c := range r.Closed {
		closed[c.Headline] = c.Count
		if c.Headline == "Ship the landing page" && c.Project != "Website" {
			t.Errorf("landing page project %q, want Website", c.Project)
		}
		if c.Headline == "Water the plants" && !c.Repeat {
			t.Errorf("the plants are a repeating task")
		}
		if c.Headline == "Animated hero" && !c.Cancelled {
			t.Errorf("the hero was cancelled, not done")
		}
	}
	want := map[string]int{"Ship the landing page": 1, "Water the plants": 1, "Animated hero": 1}
	for k, v := range want {
		if closed[k] != v {
			t.Errorf("closed %q count %d, want %d (all: %v)", k, closed[k], v, closed)
		}
	}
	for _, gone := range []string{"Old thing", "Hidden", "Archived", "Exercise"} {
		if _, ok := closed[gone]; ok {
			t.Errorf("%q should not be in Closed", gone)
		}
	}

	if len(r.Habits) != 1 || r.Habits[0].Headline != "Exercise" || r.Habits[0].Done != 2 || r.Habits[0].Days != 7 {
		t.Errorf("habits %+v, want Exercise done 2 of 7", r.Habits)
	}

	// 1:00 of the clock across midnight falls in the range, then 1:30, then
	// the running hour: 3:30 in all, against the project.
	if r.ClockedMins != 210 {
		t.Errorf("clocked %v minutes, want 210", r.ClockedMins)
	}
	if len(r.Clocked) != 1 || r.Clocked[0].Name != "Website" || r.Clocked[0].Kind != "project" {
		t.Errorf("clocked groups %+v, want one: Website", r.Clocked)
	}
	if r.Running == nil || r.Running.Headline != "Write the pricing copy" || r.Running.Mins != 60 || r.Running.When != "11:00" {
		t.Errorf("running %+v, want the pricing copy, 60 minutes since 11:00", r.Running)
	}

	kinds := map[string]string{}
	for _, a := range r.Agenda {
		kinds[a.Headline] = a.Kind
	}
	if kinds["Dentist"] != "timestamp" || kinds["Tax return"] != "overdue" || kinds["Write the pricing copy"] != "next" {
		t.Errorf("today %v", kinds)
	}
	if _, ok := kinds["Exercise"]; ok {
		t.Errorf("habits are the tracker's, not today's list")
	}
	for _, a := range r.Agenda {
		if a.Headline == "Dentist" && a.When != "15:00" {
			t.Errorf("dentist at %q, want 15:00", a.When)
		}
		if a.Headline == "Tax return" && a.Late != 3 {
			t.Errorf("tax return %d days late, want 3", a.Late)
		}
	}

	if len(r.Waiting) != 1 || r.Waiting[0].Headline != "Logo from the designer" || r.Waiting[0].Days != 4 || r.Waiting[0].SinceFrom != "logbook" {
		t.Errorf("waiting %+v, want the logo, 4 days from the logbook", r.Waiting)
	}
	if r.From != "2026-10-08" || r.To != "2026-10-14" {
		t.Errorf("range %s..%s", r.From, r.To)
	}
}

func TestRecapTitleIsPlain(t *testing.T) {
	loadOrg(t, map[string]string{"t.org": "* TODO Read [[file:x.org][the /spec/]] <2026-10-14 Wed> [2/5]\n"})
	f := GetDb().GetFile(GetDb().GetFiles()[0])
	got := recapTitle(f.Doc.Outline.Children[0].Headline)
	if got != "Read the spec" || strings.Contains(got, "<") {
		t.Errorf("title %q, want %q", got, "Read the spec")
	}
}
