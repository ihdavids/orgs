package ical

import (
	"fmt"
	"strings"
	"time"
)

// Event is a VEVENT to write.
type Event struct {
	UID, Summary, Description, Location, URL string
	Categories                                []string
	Start, End                                time.Time
	// A DATE rather than a DATE-TIME: Start is the first day, End the last.
	AllDay bool
	RRule  string // the RRULE value, without the name
}

// Todo is a VTODO to write.
type Todo struct {
	UID, Summary, Description, Location, URL string
	Categories                                []string
	Due, Start                                time.Time // zero = none
	DueAllDay, StartAllDay                    bool
	Completed                                 time.Time
	Status                                    string // NEEDS-ACTION, IN-PROCESS, COMPLETED, CANCELLED
	Priority                                  int    // 0 none, 1 high .. 9 low
	RRule                                     string
}

// Writer builds a VCALENDAR.
type Writer struct {
	Name   string
	Now    time.Time
	Events []Event
	Todos  []Todo
}

// fold writes a content line, folded at 75 octets without splitting a
// character, with CRLF endings.
func fold(b *strings.Builder, line string) {
	for len(line) > 75 {
		cut := 75
		for cut > 0 && line[cut]&0xC0 == 0x80 {
			cut--
		}
		b.WriteString(line[:cut] + "\r\n")
		line = " " + line[cut:]
	}
	b.WriteString(line + "\r\n")
}

func utc(t time.Time) string { return t.UTC().Format("20060102T150405Z") }
func day(t time.Time) string { return t.Format("20060102") }

func (w *Writer) common(b *strings.Builder, uid, summary, desc, loc, url string, cats []string, rrule string) {
	fold(b, "UID:"+Escape(uid))
	fold(b, "DTSTAMP:"+utc(w.Now))
	fold(b, "SUMMARY:"+Escape(summary))
	if desc != "" {
		fold(b, "DESCRIPTION:"+Escape(desc))
	}
	if loc != "" {
		fold(b, "LOCATION:"+Escape(loc))
	}
	if url != "" {
		fold(b, "URL:"+url)
	}
	if len(cats) > 0 {
		esc := []string{}
		for _, c := range cats {
			esc = append(esc, Escape(c))
		}
		fold(b, "CATEGORIES:"+strings.Join(esc, ","))
	}
	if rrule != "" {
		fold(b, "RRULE:"+rrule)
	}
}

// String is the calendar as an .ics file.
func (w *Writer) String() string {
	if w.Now.IsZero() {
		w.Now = time.Now()
	}
	b := &strings.Builder{}
	fold(b, "BEGIN:VCALENDAR")
	fold(b, "VERSION:2.0")
	fold(b, "PRODID:-//orgs//org-mode export//EN")
	fold(b, "CALSCALE:GREGORIAN")
	if w.Name != "" {
		fold(b, "X-WR-CALNAME:"+Escape(w.Name))
	}
	for _, e := range w.Events {
		fold(b, "BEGIN:VEVENT")
		w.common(b, e.UID, e.Summary, e.Description, e.Location, e.URL, e.Categories, e.RRule)
		if e.AllDay {
			end := e.End
			if end.IsZero() || end.Before(e.Start) {
				end = e.Start
			}
			fold(b, "DTSTART;VALUE=DATE:"+day(e.Start))
			// The day after the last one: DTEND is exclusive.
			fold(b, "DTEND;VALUE=DATE:"+day(end.AddDate(0, 0, 1)))
		} else {
			fold(b, "DTSTART:"+utc(e.Start))
			if !e.End.IsZero() && e.End.After(e.Start) {
				fold(b, "DTEND:"+utc(e.End))
			}
		}
		fold(b, "END:VEVENT")
	}
	for _, t := range w.Todos {
		fold(b, "BEGIN:VTODO")
		w.common(b, t.UID, t.Summary, t.Description, t.Location, t.URL, t.Categories, t.RRule)
		// RFC 5545: DUE and DTSTART must be the same kind of value. A due
		// date is the more important of the two, so a timed start beside a
		// due day is written as its day.
		if !t.Due.IsZero() && t.DueAllDay {
			t.StartAllDay = true
		}
		if !t.Start.IsZero() {
			if t.StartAllDay {
				fold(b, "DTSTART;VALUE=DATE:"+day(t.Start))
			} else {
				fold(b, "DTSTART:"+utc(t.Start))
			}
		}
		if !t.Due.IsZero() {
			if t.DueAllDay {
				fold(b, "DUE;VALUE=DATE:"+day(t.Due))
			} else {
				fold(b, "DUE:"+utc(t.Due))
			}
		}
		if t.Status != "" {
			fold(b, "STATUS:"+t.Status)
		}
		if !t.Completed.IsZero() {
			fold(b, "COMPLETED:"+utc(t.Completed))
			if t.Status == "COMPLETED" {
				fold(b, "PERCENT-COMPLETE:100")
			}
		}
		if t.Priority > 0 {
			fold(b, fmt.Sprintf("PRIORITY:%d", t.Priority))
		}
		fold(b, "END:VTODO")
	}
	fold(b, "END:VCALENDAR")
	return b.String()
}
