package ics

import (
	"strings"
	"testing"
	"time"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
	"github.com/ihdavids/orgs/internal/common/ical"
)

type oneFile struct {
	common.ODb
	doc *org.Document
}

func (d oneFile) FindByFile(string) *org.Document { return d.doc }

const sample = `#+TITLE: Plans
#+TODO: TODO NEXT | DONE CANCELLED
* Team sync :work:
  <2026-10-12 Mon 10:00-10:30 +1w>
  Bring the numbers.
* Offsite
  :PROPERTIES:
  :LOCATION: The lodge
  :ICAL_UID: offsite@corp
  :END:
  <2026-10-20 Tue>--<2026-10-22 Thu>
* NEXT [#A] Write report
  DEADLINE: <2026-10-15 Thu> SCHEDULED: <2026-10-13 Tue 09:00>
  :PROPERTIES:
  :ID: rep-1
  :END:
* DONE Book flights
  CLOSED: [2026-10-01 Thu 12:00]
* Lunch with Sam <2026-10-14 Wed 12:30>
  :PROPERTIES:
  :EFFORT: 1:30
  :END:
* Notes
  Just text, no dates.
`

func export(t *testing.T, e *Exporter) string {
	t.Helper()
	conf := org.New()
	doc := conf.Parse(strings.NewReader(sample), "plans.org")
	err, out := e.ExportToString(oneFile{doc: doc}, "plans.org", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestCalendar(t *testing.T) {
	out := export(t, &Exporter{})
	cal, err := ical.Parse(out)
	if err != nil {
		t.Fatal(err)
	}
	if cal.Name != "Plans" {
		t.Errorf("name %q", cal.Name)
	}
	byName := map[string]ical.Component{}
	for _, c := range cal.Components {
		byName[c.Kind+" "+c.Text("SUMMARY")] = c
	}
	sync, ok := byName["VEVENT Team sync"]
	if !ok {
		t.Fatalf("no team sync in\n%s", out)
	}
	if sync.Text("RRULE") != "FREQ=WEEKLY" || sync.Text("DESCRIPTION") != "Bring the numbers." || sync.Text("CATEGORIES") != "work" {
		t.Errorf("sync %+v", sync)
	}
	start, _ := ical.ParseWhen(sync.Get("DTSTART"), time.Local)
	end, _ := ical.ParseWhen(sync.Get("DTEND"), time.Local)
	if end.Time.Sub(start.Time) != 30*time.Minute {
		t.Errorf("sync lasts %v", end.Time.Sub(start.Time))
	}
	off := byName["VEVENT Offsite"]
	if off.Text("UID") != "offsite@corp-0" || off.Text("LOCATION") != "The lodge" || off.Get("DTEND").Value != "20261023" {
		t.Errorf("offsite %+v", off)
	}
	lunch := byName["VEVENT Lunch with Sam"]
	ls, _ := ical.ParseWhen(lunch.Get("DTSTART"), time.Local)
	le, _ := ical.ParseWhen(lunch.Get("DTEND"), time.Local)
	if le.Time.Sub(ls.Time) != 90*time.Minute {
		t.Errorf("lunch should last its EFFORT, lasts %v", le.Time.Sub(ls.Time))
	}
	rep := byName["VTODO Write report"]
	if rep.Text("UID") != "rep-1" || rep.Text("PRIORITY") != "1" || rep.Text("STATUS") != "IN-PROCESS" || rep.Get("DUE").Value != "20261015" {
		t.Errorf("report %+v", rep)
	}
	if _, ok := byName["VTODO Book flights"]; ok {
		t.Errorf("done task exported without done: true")
	}
	if strings.Contains(out, "Notes") {
		t.Errorf("undated heading exported")
	}
}

func TestDoneAndDeadlineEvents(t *testing.T) {
	out := export(t, &Exporter{Done: true, DeadlinesAsEvents: true})
	if !strings.Contains(out, "SUMMARY:Book flights") || !strings.Contains(out, "STATUS:COMPLETED") {
		t.Errorf("done task missing:\n%s", out)
	}
	if !strings.Contains(out, "SUMMARY:Due: Write report") {
		t.Errorf("deadline event missing:\n%s", out)
	}
}

// The uid made for a heading with none is the same on every export.
func TestStableUID(t *testing.T) {
	a, b := export(t, &Exporter{}), export(t, &Exporter{})
	uid := func(s string) string {
		i := strings.Index(s, "UID:")
		return s[i : i+30]
	}
	if uid(a) != uid(b) {
		t.Errorf("uid changed between exports")
	}
}
