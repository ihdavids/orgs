package ics

/* SDOC: Exporters

* iCalendar (.ics)
	Writes headings as an iCalendar file: appointments as events (VEVENT)
	and tasks as to-dos (VTODO), for Apple Calendar and Reminders, Google
	Calendar, Outlook, Thunderbird and anything else that reads .ics.

	#+BEGIN_SRC bash
	orgs export -f ics meetings.org -out meetings.ics
	orgs export -f ics 'IsTodo() && HasDeadline()' -out due.ics
	#+END_SRC

	The query is a file (every heading in it) or an expression (the headings
	it finds). It needs no settings and need not be listed in the yaml; list
	it to change these:

	#+BEGIN_SRC yaml
  - name: "ics"
    events: true            # each plain active stamp is an event
    todos: true             # each heading with a keyword is a to-do
    done: false             # finished tasks too, as COMPLETED
    deadlinesAsEvents: false   # a task's DEADLINE also as an event on its day
    scheduledAsEvents: false   # and its SCHEDULED
    duration: 60            # minutes, for a timed stamp with no end (EFFORT wins)
	#+END_SRC

	- *An event* is a heading's own active timestamp (=<2026-10-12 Mon 10:00>=,
	  a range, a repeater as RRULE), as the agenda reads it. Inactive stamps
	  and those in drawers are not events.
	- *A to-do* is a heading with a keyword: =DEADLINE= is DUE, =SCHEDULED= is
	  DTSTART, =[#A]/[#B]/[#C]= are priority 1/5/9, a done keyword is COMPLETED
	  with its =CLOSED= time, and an active keyword other than the first (NEXT,
	  STARTED...) is IN-PROCESS.
	- The title, body text, tags (CATEGORIES), =:LOCATION:= and =:URL:= go
	  across. The UID is =:ICAL_UID:= when the heading has one (so a calendar
	  imported with =orgs import= goes back out as the same events), else
	  =:ID:=, else one made from the file and the outline path, so exporting
	  again updates rather than duplicates.
EDOC */

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/app/orgs/plugs"
	"github.com/ihdavids/orgs/internal/common"
	"github.com/ihdavids/orgs/internal/common/ical"
)

type Exporter struct {
	Events            *bool `yaml:"events"`
	Todos             *bool `yaml:"todos"`
	Done              bool  `yaml:"done"`
	DeadlinesAsEvents bool  `yaml:"deadlinesAsEvents"`
	ScheduledAsEvents bool  `yaml:"scheduledAsEvents"`
	Duration          int   `yaml:"duration"`
	pm                *common.PluginManager
}

func (self *Exporter) Unmarshal(unmarshal func(interface{}) error) error { return unmarshal(self) }
func (self *Exporter) Startup(manager *common.PluginManager, opts *common.PluginOpts) {
	self.pm = manager
}
func (self *Exporter) Standalone() bool { return true }

func (self *Exporter) Export(db common.ODb, query string, to string, opts string, props map[string]string) error {
	err, txt := self.ExportToString(db, query, opts, props)
	if err != nil {
		return err
	}
	return plugs.WriteExportFile(to, txt)
}

func on(b *bool) bool { return b == nil || *b }

func (self *Exporter) ExportToString(db common.ODb, query string, opts string, props map[string]string) (error, string) {
	secs, err := plugs.Headings(db, query)
	if err != nil {
		return err, ""
	}
	w := self.Calendar(secs, time.Now())
	w.Name = query
	if d := db.FindByFile(query); d != nil {
		if t := d.Get("TITLE"); t != "" {
			w.Name = t
		} else {
			w.Name = strings.TrimSuffix(filepath.Base(query), filepath.Ext(query))
		}
	}
	if len(w.Events) == 0 && len(w.Todos) == 0 {
		return fmt.Errorf("nothing to put in a calendar: no dated headings or tasks in %q", query), ""
	}
	return nil, w.String()
}

// Calendar is the events and to-dos of these headings.
func (self *Exporter) Calendar(secs []*org.Section, now time.Time) *ical.Writer {
	w := &ical.Writer{Now: now}
	for _, s := range secs {
		h := s.Headline
		if h == nil || h.IsComment || h.IsArchived() {
			continue
		}
		title := plugs.Title(h)
		// A stamp in the headline is part of its title as written; in a
		// calendar it is the time, said once.
		title = strings.TrimSpace(stampRe.ReplaceAllString(title, ""))
		if title == "" {
			title = "(untitled)"
		}
		desc := Description(h)
		loc, url := prop(h, "LOCATION"), linkTarget(prop(h, "URL"))
		uid := self.uid(s)
		if on(self.Events) && !(h.Status != "" && h.IsDone()) {
			for i, ts := range h.Timestamps {
				if ts == nil || ts.Time == nil {
					continue
				}
				e := self.event(h, ts.Time)
				e.UID, e.Summary, e.Description, e.Location, e.URL, e.Categories = fmt.Sprintf("%s-%d", uid, i), title, desc, loc, url, h.Tags
				w.Events = append(w.Events, e)
			}
			for _, x := range []struct {
				want bool
				sdc  *org.SDC
				kind string
			}{{self.DeadlinesAsEvents, h.Deadline, "deadline"}, {self.ScheduledAsEvents, h.Scheduled, "scheduled"}} {
				if x.want && x.sdc != nil && x.sdc.Date != nil {
					e := self.event(h, x.sdc.Date)
					e.UID, e.Summary, e.Description, e.Location, e.URL, e.Categories = uid+"-"+x.kind, title, desc, loc, url, h.Tags
					if x.kind == "deadline" {
						e.Summary = "Due: " + title
					}
					w.Events = append(w.Events, e)
				}
			}
		}
		if on(self.Todos) && h.Status != "" {
			done := h.IsDone()
			if done && !self.Done {
				continue
			}
			t := ical.Todo{UID: uid, Summary: title, Description: desc, Location: loc, URL: url, Categories: h.Tags}
			t.Status = "NEEDS-ACTION"
			if done {
				t.Status = "COMPLETED"
				if strings.HasPrefix(strings.ToUpper(h.Status), "CANCEL") {
					t.Status = "CANCELLED"
				}
				if h.Closed != nil && h.Closed.Date != nil {
					t.Completed = h.Closed.Date.Start
				}
			} else if h.Doc != nil {
				if todo, _ := h.Doc.TodoKeywords(); len(todo) > 1 && todo[0] != h.Status {
					t.Status = "IN-PROCESS"
				}
			}
			switch strings.ToUpper(h.Priority) {
			case "A":
				t.Priority = 1
			case "B":
				t.Priority = 5
			case "C":
				t.Priority = 9
			}
			if h.Deadline != nil && h.Deadline.Date != nil {
				d := h.Deadline.Date
				t.Due, t.DueAllDay = d.Start, !d.HaveTime
				t.RRule = ical.RuleFromCookie(d.RepeatString())
			}
			if h.Scheduled != nil && h.Scheduled.Date != nil {
				d := h.Scheduled.Date
				t.Start, t.StartAllDay = d.Start, !d.HaveTime
				if t.RRule == "" {
					t.RRule = ical.RuleFromCookie(d.RepeatString())
				}
			}
			if done {
				t.RRule = ""
			}
			w.Todos = append(w.Todos, t)
		}
	}
	return w
}

var stampRe = regexp.MustCompile(`<\d{4}-\d{2}-\d{2}[^>]*>(--<\d{4}-\d{2}-\d{2}[^>]*>)?`)

func (self *Exporter) event(h *org.Headline, d *org.OrgDate) ical.Event {
	e := ical.Event{Start: d.Start, AllDay: !d.HaveTime}
	if d.HasEnd() {
		e.End = d.End
	} else if d.HaveTime {
		mins := self.Duration
		if m := effortMinutes(prop(h, "EFFORT")); m > 0 {
			mins = m
		}
		if mins <= 0 {
			mins = 60
		}
		e.End = d.Start.Add(time.Duration(mins) * time.Minute)
	}
	e.RRule = ical.RuleFromCookie(d.RepeatString())
	return e
}

var effortRe = regexp.MustCompile(`^(?:(\d+):)?(\d+)$`)

// EFFORT as minutes when it is written H:MM or M; anything fancier (2d, 1w)
// is not an appointment's length, and the default is used.
func effortMinutes(s string) int {
	m := effortRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0
	}
	var h, mm int
	fmt.Sscan(m[2], &mm)
	if m[1] != "" {
		fmt.Sscan(m[1], &h)
	}
	return h*60 + mm
}

func prop(h *org.Headline, k string) string {
	if h.Properties == nil {
		return ""
	}
	v, _ := h.Properties.Get(k)
	return strings.TrimSpace(v)
}

var linkRe = regexp.MustCompile(`^\[\[([^\]]+)\](?:\[[^\]]*\])?\]$`)

func linkTarget(s string) string {
	if m := linkRe.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	return s
}

// The uid an export keeps from one run to the next: one the heading carries,
// or one made from where it is (file and outline path), which survives
// everything but a rename or a move.
func (self *Exporter) uid(s *org.Section) string {
	for _, k := range []string{"ICAL_UID", "ID", "CUSTOM_ID"} {
		if v := prop(s.Headline, k); v != "" {
			return v
		}
	}
	path := []string{}
	for p := s; p != nil && p.Headline != nil; p = p.Parent {
		path = append([]string{plugs.Title(p.Headline)}, path...)
	}
	file := ""
	if s.Headline.Doc != nil {
		file = filepath.Base(s.Headline.Doc.Path)
	}
	sum := sha1.Sum([]byte(file + "\x00" + strings.Join(path, "\x00")))
	return hex.EncodeToString(sum[:10]) + "@orgs"
}

// Description is a heading's own text as plain text: its paragraphs, lists
// and blocks, not its drawers, planning line or children.
func Description(h *org.Headline) string {
	parts := []string{}
	for _, n := range h.Children {
		switch v := n.(type) {
		case org.Paragraph:
			// The heading's stamps are its times, already said.
			if t := strings.Join(strings.Fields(stampRe.ReplaceAllString(plugs.PlainText(v.Children), "")), " "); t != "" {
				parts = append(parts, t)
			}
		case org.List:
			for _, it := range v.Items {
				if li, ok := it.(org.ListItem); ok {
					item := []string{}
					for _, c := range li.Children {
						if p, ok := c.(org.Paragraph); ok {
							item = append(item, plugs.PlainText(p.Children))
						}
					}
					parts = append(parts, strings.TrimSpace(li.Bullet+" "+strings.Join(item, " ")))
				}
			}
		case org.Block:
			if v.Name == "SRC" || v.Name == "EXAMPLE" || v.Name == "QUOTE" {
				parts = append(parts, strings.TrimRight(org.String(v.Children...), "\n"))
			}
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func init() {
	common.AddExporter("ics", func() common.Exporter { return &Exporter{} })
}
