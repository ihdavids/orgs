package ical

import (
	"fmt"
	"html"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// OrgOptions says how a calendar becomes org.
type OrgOptions struct {
	// #+TITLE: of a new file; the calendar's own name when empty, nothing
	// when that is empty too.
	Title string
	// Where times are written in; time.Local when nil.
	Location *time.Location
	// Leave out events that ended before this (a repeating one is always
	// kept). Zero keeps everything.
	Since time.Time
	// Tag every entry with this (e.g. the calendar's name); nothing when empty.
	Tag string
	// Keep cancelled events and tasks.
	Cancelled bool
	// Keywords for tasks: open, in progress, done, cancelled.
	Todo, Doing, Done, Cancel string
}

// Stats says what a conversion did.
type Stats struct {
	Events, Todos, Skipped int
	// Repeats that say more than org can (an end date, several weekdays):
	// written as a plain repeater with the rule kept in :RRULE:.
	PartialRules int
}

func (o *OrgOptions) fill() {
	if o.Location == nil {
		o.Location = time.Local
	}
	if o.Todo == "" {
		o.Todo = "TODO"
	}
	if o.Doing == "" {
		o.Doing = "NEXT"
	}
	if o.Done == "" {
		o.Done = "DONE"
	}
	if o.Cancel == "" {
		o.Cancel = "CANCELLED"
	}
}

func stamp(t time.Time, allDay bool) string {
	if allDay {
		return t.Format("2006-01-02 Mon")
	}
	return t.Format("2006-01-02 Mon 15:04")
}

// OrgRange writes start to end the way org spells it: one stamp for a moment
// or a day, `<d 10:00-11:00>` within a day, `<a>--<b>` across days. The
// repeater goes in each stamp, as org writes it.
func OrgRange(start, end time.Time, allDay bool, cookie string) string {
	rep := ""
	if cookie != "" {
		rep = " " + cookie
	}
	if allDay {
		if end.IsZero() || !end.After(start) {
			return "<" + stamp(start, true) + rep + ">"
		}
		return "<" + stamp(start, true) + rep + ">--<" + stamp(end, true) + rep + ">"
	}
	if end.IsZero() || !end.After(start) {
		return "<" + stamp(start, false) + rep + ">"
	}
	if sameDay(start, end) {
		return "<" + stamp(start, false) + "-" + end.Format("15:04") + rep + ">"
	}
	return "<" + stamp(start, false) + rep + ">--<" + stamp(end, false) + rep + ">"
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

var badTag = regexp.MustCompile(`[^\p{L}\p{N}_@#%]+`)

// OrgTag makes a category into an org tag.
func OrgTag(s string) string {
	return strings.Trim(badTag.ReplaceAllString(strings.TrimSpace(s), "_"), "_")
}

var (
	brRe   = regexp.MustCompile(`(?i)<\s*br\s*/?\s*>|</\s*p\s*>|</\s*li\s*>`)
	liRe   = regexp.MustCompile(`(?i)<\s*li[^>]*>`)
	aRe    = regexp.MustCompile(`(?is)<a\s[^>]*href\s*=\s*"([^"]+)"[^>]*>(.*?)</a>`)
	tagRe  = regexp.MustCompile(`(?s)<[^>]+>`)
	isHTML = regexp.MustCompile(`(?i)</?(p|br|div|span|a|b|i|ul|li|html-blob)\b`)
)

// plainDescription turns a description into org text. Google and Outlook
// put html in it; links become org links and the rest loses its tags.
func plainDescription(s string) string {
	s = strings.TrimSpace(s)
	if isHTML.MatchString(s) {
		s = aRe.ReplaceAllString(s, "[[$1][$2]]")
		s = liRe.ReplaceAllString(s, "- ")
		s = brRe.ReplaceAllString(s, "\n")
		s = tagRe.ReplaceAllString(s, "")
		s = html.UnescapeString(s)
	}
	lines := []string{}
	blank := false
	for _, l := range strings.Split(s, "\n") {
		l = strings.TrimRight(l, " \t ")
		if strings.TrimSpace(l) == "" {
			if !blank && len(lines) > 0 {
				lines = append(lines, "")
			}
			blank = true
			continue
		}
		blank = false
		lines = append(lines, l)
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}

// body indents text under a heading. A line that would read as a heading, a
// keyword or a drawer is indented anyway, which is what keeps it text.
func body(b *strings.Builder, indent, text string) {
	for _, l := range strings.Split(text, "\n") {
		if l == "" {
			b.WriteString("\n")
			continue
		}
		b.WriteString(indent + l + "\n")
	}
}

func cn(p *Prop) string {
	if p == nil {
		return ""
	}
	name := p.Params["CN"]
	mail := strings.TrimPrefix(strings.TrimPrefix(p.Value, "mailto:"), "MAILTO:")
	switch {
	case name != "" && mail != "":
		return fmt.Sprintf("%s <%s>", name, mail)
	case name != "":
		return name
	}
	return mail
}

type entry struct {
	start time.Time
	text  string
}

// ToOrg writes a calendar as org: an event is a heading with its time as a
// plain active stamp (so it lands in the agenda on its day), a task a heading
// with a keyword and DEADLINE/SCHEDULED. Each carries :ICAL_UID:, which is
// how importing the same calendar again knows what it already has.
func ToOrg(cal *Calendar, o OrgOptions) (string, Stats) {
	o.fill()
	st := Stats{}
	entries := []entry{}
	usedDoing, usedCancel := false, false
	for i := range cal.Components {
		c := &cal.Components[i]
		status := strings.ToUpper(c.Text("STATUS"))
		if status == "CANCELLED" && !o.Cancelled {
			st.Skipped++
			continue
		}
		summary := strings.Join(strings.Fields(c.Text("SUMMARY")), " ")
		if summary == "" {
			summary = "(untitled)"
		}
		tags := []string{}
		if o.Tag != "" {
			tags = append(tags, OrgTag(o.Tag))
		}
		for _, p := range c.Props {
			if p.Name == "CATEGORIES" {
				for _, cat := range splitUnquoted(p.Value, ',') {
					if t := OrgTag(Unescape(cat)); t != "" && !contains(tags, t) {
						tags = append(tags, t)
					}
				}
			}
		}
		props := [][2]string{}
		if uid := c.Text("UID"); uid != "" {
			props = append(props, [2]string{"ICAL_UID", uid})
		}
		for _, k := range []string{"LOCATION", "URL"} {
			if v := strings.Join(strings.Fields(c.Text(k)), " "); v != "" {
				props = append(props, [2]string{k, v})
			}
		}
		if who := cn(c.Get("ORGANIZER")); who != "" {
			props = append(props, [2]string{"ORGANIZER", who})
		}
		b := strings.Builder{}
		var when time.Time
		start, hasStart := ParseWhen(c.Get("DTSTART"), o.Location)
		cookie := ""
		if rr := c.Get("RRULE"); rr != nil {
			if r, ok := ParseRule(rr.Value, start.Time); ok {
				cookie = r.Cookie()
				if r.Partial {
					st.PartialRules++
					props = append(props, [2]string{"RRULE", rr.Value})
				}
			} else {
				props = append(props, [2]string{"RRULE", rr.Value})
			}
		}
		tagStr := ""
		if len(tags) > 0 {
			tagStr = " :" + strings.Join(tags, ":") + ":"
		}
		switch c.Kind {
		case "VEVENT":
			if !hasStart {
				st.Skipped++
				continue
			}
			end, hasEnd := ParseWhen(c.Get("DTEND"), o.Location)
			if !hasEnd {
				if d, ok := ParseDuration(c.Text("DURATION")); ok {
					end = When{Time: start.Time.Add(d), AllDay: start.AllDay}
					hasEnd = true
				}
			}
			endT := time.Time{}
			if hasEnd {
				endT = end.Time
				// An all-day DTEND is the day after the last one.
				if start.AllDay {
					endT = endT.AddDate(0, 0, -1)
				}
			}
			last := start.Time
			if endT.After(last) {
				last = endT
			}
			if !o.Since.IsZero() && cookie == "" && last.Before(o.Since) {
				st.Skipped++
				continue
			}
			when = start.Time
			st.Events++
			b.WriteString("* " + summary + tagStr + "\n")
			writeDrawer(&b, props)
			b.WriteString("  " + OrgRange(start.Time, endT, start.AllDay, cookie) + "\n")
		case "VTODO":
			kw := o.Todo
			switch status {
			case "IN-PROCESS":
				kw, usedDoing = o.Doing, true
			case "COMPLETED":
				kw = o.Done
			case "CANCELLED":
				kw, usedCancel = o.Cancel, true
			}
			if c.Get("COMPLETED") != nil && status == "" {
				kw = o.Done
			}
			pri := ""
			// 1-4 high, 5 middling, 6-9 low; 0 is none (RFC 5545 3.8.1.9).
			if n, err := strconv.Atoi(strings.TrimSpace(c.Text("PRIORITY"))); err == nil && n > 0 {
				switch {
				case n <= 4:
					pri = " [#A]"
				case n == 5:
					pri = " [#B]"
				default:
					pri = " [#C]"
				}
			}
			st.Todos++
			b.WriteString("* " + kw + pri + " " + summary + tagStr + "\n")
			plan := []string{}
			if done, ok := ParseWhen(c.Get("COMPLETED"), o.Location); ok && (kw == o.Done || kw == o.Cancel) {
				plan = append(plan, "CLOSED: ["+stamp(done.Time, false)+"]")
			}
			if due, ok := ParseWhen(c.Get("DUE"), o.Location); ok {
				plan = append(plan, "DEADLINE: "+OrgRange(due.Time, time.Time{}, due.AllDay, cookie))
				cookie = ""
				when = due.Time
			}
			if hasStart {
				plan = append(plan, "SCHEDULED: "+OrgRange(start.Time, time.Time{}, start.AllDay, cookie))
				if when.IsZero() {
					when = start.Time
				}
			}
			if len(plan) > 0 {
				b.WriteString("  " + strings.Join(plan, " ") + "\n")
			}
			if pc := strings.TrimSpace(c.Text("PERCENT-COMPLETE")); pc != "" && pc != "0" && kw != o.Done {
				props = append(props, [2]string{"PERCENT_COMPLETE", pc})
			}
			writeDrawer(&b, props)
		default:
			continue
		}
		if d := plainDescription(c.Text("DESCRIPTION")); d != "" {
			body(&b, "  ", d)
		}
		entries = append(entries, entry{when, b.String()})
	}
	// In date order, which is how a calendar is read; undated tasks last.
	sort.SliceStable(entries, func(i, j int) bool {
		a, b := entries[i].start, entries[j].start
		if a.IsZero() != b.IsZero() {
			return !a.IsZero()
		}
		return a.Before(b)
	})
	out := strings.Builder{}
	title := o.Title
	if title == "" {
		title = cal.Name
	}
	if title != "" {
		out.WriteString("#+TITLE: " + title + "\n")
	}
	if st.Todos > 0 {
		open := o.Todo
		if usedDoing {
			open += " " + o.Doing
		}
		done := o.Done
		if usedCancel {
			done += " " + o.Cancel
		}
		out.WriteString("#+TODO: " + open + " | " + done + "\n")
	}
	if out.Len() > 0 {
		out.WriteString("\n")
	}
	for _, e := range entries {
		out.WriteString(e.text)
	}
	return out.String(), st
}

func writeDrawer(b *strings.Builder, props [][2]string) {
	if len(props) == 0 {
		return
	}
	b.WriteString("  :PROPERTIES:\n")
	for _, kv := range props {
		b.WriteString("  :" + kv[0] + ": " + kv[1] + "\n")
	}
	b.WriteString("  :END:\n")
}

func contains(vs []string, v string) bool {
	for _, x := range vs {
		if x == v {
			return true
		}
	}
	return false
}
