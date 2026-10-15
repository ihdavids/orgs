package ical

import (
	"strings"
	"testing"
	"time"
)

const sample = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nX-WR-CALNAME:Work\r\n" +
	"BEGIN:VTIMEZONE\r\nTZID:America/New_York\r\nEND:VTIMEZONE\r\n" +
	"BEGIN:VEVENT\r\nUID:ev1@x\r\nSUMMARY:Stand\\, up\r\nDTSTART;TZID=America/New_York:20261012T090000\r\n" +
	"DTEND;TZID=America/New_York:20261012T091500\r\nRRULE:FREQ=WEEKLY;BYDAY=MO\r\n" +
	"LOCATION:Room 4\r\nCATEGORIES:Team,Daily Sync\r\n" +
	"DESCRIPTION:Line one\\nLine two that is quite long so that it has to be fol\r\n ded across lines\r\n" +
	"BEGIN:VALARM\r\nTRIGGER:-PT10M\r\nACTION:DISPLAY\r\nEND:VALARM\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nUID:ev2@x\r\nSUMMARY:Offsite\r\nDTSTART;VALUE=DATE:20261020\r\nDTEND;VALUE=DATE:20261023\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nUID:ev3@x\r\nSUMMARY:Old thing\r\nDTSTART:20200101T100000Z\r\nDURATION:PT1H\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nUID:ev4@x\r\nSUMMARY:Called off\r\nSTATUS:CANCELLED\r\nDTSTART:20261101T100000Z\r\nEND:VEVENT\r\n" +
	"BEGIN:VTODO\r\nUID:td1@x\r\nSUMMARY:File report\r\nDUE;VALUE=DATE:20261015\r\nPRIORITY:1\r\nSTATUS:NEEDS-ACTION\r\nEND:VTODO\r\n" +
	"BEGIN:VTODO\r\nUID:td2@x\r\nSUMMARY:Book flights\r\nSTATUS:COMPLETED\r\nCOMPLETED:20261001T120000Z\r\nEND:VTODO\r\n" +
	"END:VCALENDAR\r\n"

func TestParse(t *testing.T) {
	cal, err := Parse(sample)
	if err != nil {
		t.Fatal(err)
	}
	if cal.Name != "Work" || len(cal.Components) != 6 {
		t.Fatalf("name %q, %d components", cal.Name, len(cal.Components))
	}
	ev := cal.Components[0]
	if ev.Text("SUMMARY") != "Stand, up" {
		t.Errorf("summary %q", ev.Text("SUMMARY"))
	}
	if !strings.Contains(ev.Text("DESCRIPTION"), "folded across") || !strings.Contains(ev.Text("DESCRIPTION"), "one\nLine") {
		t.Errorf("description %q", ev.Text("DESCRIPTION"))
	}
	if ev.Get("TRIGGER") != nil {
		t.Errorf("an alarm's properties leaked into the event")
	}
}

func TestToOrg(t *testing.T) {
	cal, _ := Parse(sample)
	ny, _ := time.LoadLocation("America/New_York")
	since := time.Date(2026, 1, 1, 0, 0, 0, 0, ny)
	org, st := ToOrg(cal, OrgOptions{Location: ny, Since: since})
	if st.Events != 2 || st.Todos != 2 || st.Skipped != 2 {
		t.Errorf("stats %+v", st)
	}
	for _, want := range []string{
		"#+TITLE: Work\n#+TODO: TODO | DONE\n",
		"* Stand, up :Team:Daily_Sync:\n  :PROPERTIES:\n  :ICAL_UID: ev1@x\n  :LOCATION: Room 4\n  :END:\n  <2026-10-12 Mon 09:00-09:15 +1w>\n  Line one\n",
		"* Offsite\n  :PROPERTIES:\n  :ICAL_UID: ev2@x\n  :END:\n  <2026-10-20 Tue>--<2026-10-22 Thu>\n",
		"* TODO [#A] File report\n  DEADLINE: <2026-10-15 Thu>\n",
		"* DONE Book flights\n  CLOSED: [2026-10-01 Thu 08:00]\n",
	} {
		if !strings.Contains(org, want) {
			t.Errorf("missing\n%s\nin\n%s", want, org)
		}
	}
	if strings.Contains(org, "Old thing") || strings.Contains(org, "Called off") {
		t.Errorf("old or cancelled event kept:\n%s", org)
	}
	// Date order: the stand-up before the offsite.
	if strings.Index(org, "Stand, up") > strings.Index(org, "Offsite") {
		t.Errorf("not in date order:\n%s", org)
	}
}

func TestRules(t *testing.T) {
	mon := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	r, ok := ParseRule("FREQ=WEEKLY;INTERVAL=2;BYDAY=MO", mon)
	if !ok || r.Cookie() != "+2w" || r.Partial {
		t.Errorf("%+v", r)
	}
	r, _ = ParseRule("FREQ=WEEKLY;BYDAY=MO,WE", mon)
	if !r.Partial {
		t.Errorf("two weekdays should be partial")
	}
	r, _ = ParseRule("FREQ=DAILY;COUNT=5", mon)
	if !r.Partial || r.Cookie() != "+1d" {
		t.Errorf("%+v", r)
	}
	if RuleFromCookie(".+2w") != "FREQ=WEEKLY;INTERVAL=2" || RuleFromCookie("+1d") != "FREQ=DAILY" {
		t.Errorf("RuleFromCookie")
	}
}

func TestDuration(t *testing.T) {
	d, ok := ParseDuration("P1DT2H30M")
	if !ok || d != 26*time.Hour+30*time.Minute {
		t.Errorf("%v", d)
	}
}

// What the writer writes, the reader reads back.
func TestWriteRoundTrip(t *testing.T) {
	start := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	w := Writer{Name: "Mine", Now: start,
		Events: []Event{{UID: "a", Summary: "Long, with; specials\nand a newline " + strings.Repeat("é", 60),
			Start: start, End: start.Add(time.Hour), RRule: "FREQ=WEEKLY"},
			{UID: "b", Summary: "Holiday", Start: start, End: start.AddDate(0, 0, 2), AllDay: true}},
		Todos: []Todo{{UID: "c", Summary: "Task", Due: start, DueAllDay: true, Priority: 1, Status: "NEEDS-ACTION"}},
	}
	text := w.String()
	for _, l := range strings.Split(text, "\r\n") {
		if len(l) > 75 {
			t.Errorf("line longer than 75 octets: %q", l)
		}
	}
	cal, err := Parse(text)
	if err != nil {
		t.Fatal(err)
	}
	if cal.Components[0].Text("SUMMARY") != w.Events[0].Summary {
		t.Errorf("summary came back as %q", cal.Components[0].Text("SUMMARY"))
	}
	end, _ := ParseWhen(cal.Components[1].Get("DTEND"), time.UTC)
	if end.Time.Format("2006-01-02") != "2026-10-15" || !end.AllDay {
		t.Errorf("all-day end %v", end)
	}
	if cal.Components[2].Kind != "VTODO" || cal.Components[2].Text("PRIORITY") != "1" {
		t.Errorf("todo %+v", cal.Components[2])
	}
}
