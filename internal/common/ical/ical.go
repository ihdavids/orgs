// Package ical reads and writes iCalendar (RFC 5545): the events and tasks
// of a .ics file, and a calendar of them to write out.
//
// Shared by `orgs import` (a .ics into org, on the client) and the `ics`
// exporter (headings into VEVENTs and VTODOs, on the server), so the two
// agree on what a repeat, an all-day event or a priority is.
//
// Only what an org file can hold is read: summary, description, place, url,
// categories, times, a simple repeat, and for a task its due date, start,
// completion, status and priority. Alarms, attendees and time zone
// definitions are skipped; a TZID is looked up by name in the system's zone
// database (with the common Windows names mapped), which is what every
// calendar that exports one also means by it.
package ical

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Prop is one content line: NAME;PARAM=VALUE:value.
type Prop struct {
	Name   string
	Params map[string]string
	Value  string
}

// Component is a VEVENT or VTODO, its properties by name in order.
type Component struct {
	Kind  string
	Props []Prop
}

// Get is the first property called name, or nil.
func (c *Component) Get(name string) *Prop {
	for i := range c.Props {
		if c.Props[i].Name == name {
			return &c.Props[i]
		}
	}
	return nil
}

// Text is a text property's value with its escapes undone.
func (c *Component) Text(name string) string {
	if p := c.Get(name); p != nil {
		return Unescape(p.Value)
	}
	return ""
}

// Calendar is what a file held.
type Calendar struct {
	Name       string // X-WR-CALNAME
	Components []Component
}

// unfold joins continuation lines (a line starting with a space or tab
// continues the one before it, minus that one character).
func unfold(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	out := []string{}
	for _, l := range strings.Split(text, "\n") {
		if len(l) > 0 && (l[0] == ' ' || l[0] == '\t') && len(out) > 0 {
			out[len(out)-1] += l[1:]
			continue
		}
		out = append(out, l)
	}
	return out
}

// parseLine splits NAME;A=1;B="x:y":value at the first colon outside quotes.
func parseLine(l string) (Prop, bool) {
	inq := false
	colon := -1
	for i, r := range l {
		if r == '"' {
			inq = !inq
		} else if r == ':' && !inq {
			colon = i
			break
		}
	}
	if colon <= 0 {
		return Prop{}, false
	}
	head, val := l[:colon], l[colon+1:]
	p := Prop{Params: map[string]string{}, Value: val}
	parts := splitUnquoted(head, ';')
	p.Name = strings.ToUpper(strings.TrimSpace(parts[0]))
	for _, kv := range parts[1:] {
		if i := strings.IndexByte(kv, '='); i > 0 {
			p.Params[strings.ToUpper(kv[:i])] = strings.Trim(kv[i+1:], `"`)
		}
	}
	return p, true
}

func splitUnquoted(s string, sep rune) []string {
	out := []string{}
	inq := false
	cur := strings.Builder{}
	for _, r := range s {
		switch {
		case r == '"':
			inq = !inq
			cur.WriteRune(r)
		case r == sep && !inq:
			out = append(out, cur.String())
			cur.Reset()
		default:
			cur.WriteRune(r)
		}
	}
	return append(out, cur.String())
}

// Parse reads a calendar's events and tasks.
func Parse(text string) (*Calendar, error) {
	cal := &Calendar{}
	var stack []string
	var cur *Component
	seen := false
	for _, l := range unfold(strings.TrimPrefix(text, "\ufeff")) {
		if strings.TrimSpace(l) == "" {
			continue
		}
		p, ok := parseLine(l)
		if !ok {
			continue
		}
		switch p.Name {
		case "BEGIN":
			kind := strings.ToUpper(strings.TrimSpace(p.Value))
			if kind == "VCALENDAR" {
				seen = true
			}
			stack = append(stack, kind)
			if (kind == "VEVENT" || kind == "VTODO") && cur == nil {
				cur = &Component{Kind: kind}
			}
			continue
		case "END":
			kind := strings.ToUpper(strings.TrimSpace(p.Value))
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			if cur != nil && kind == cur.Kind && !inside(stack, "VEVENT", "VTODO") {
				cal.Components = append(cal.Components, *cur)
				cur = nil
			}
			continue
		}
		top := ""
		if len(stack) > 0 {
			top = stack[len(stack)-1]
		}
		switch {
		case cur != nil && top == cur.Kind:
			cur.Props = append(cur.Props, p)
		case top == "VCALENDAR" && p.Name == "X-WR-CALNAME":
			cal.Name = Unescape(p.Value)
		}
	}
	if !seen {
		return nil, fmt.Errorf("not an iCalendar file (no BEGIN:VCALENDAR)")
	}
	return cal, nil
}

func inside(stack []string, kinds ...string) bool {
	for _, s := range stack {
		for _, k := range kinds {
			if s == k {
				return true
			}
		}
	}
	return false
}

// Unescape undoes TEXT escaping: \n \, \; \\.
func Unescape(s string) string {
	b := strings.Builder{}
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
			switch s[i] {
			case 'n', 'N':
				b.WriteByte('\n')
			default:
				b.WriteByte(s[i])
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// Escape is Unescape the other way about.
func Escape(s string) string {
	r := strings.NewReplacer(`\`, `\\`, ";", `\;`, ",", `\,`, "\r\n", `\n`, "\n", `\n`)
	return r.Replace(s)
}

// ---------------------------------------------------------------------------
// Times
// ---------------------------------------------------------------------------

// The Windows zone names Outlook and Exchange write, for the zones people
// are most often in. Anything else is looked up as written.
var windowsZones = map[string]string{
	"Pacific Standard Time":          "America/Los_Angeles",
	"Mountain Standard Time":         "America/Denver",
	"Central Standard Time":          "America/Chicago",
	"Eastern Standard Time":          "America/New_York",
	"Atlantic Standard Time":         "America/Halifax",
	"Alaskan Standard Time":          "America/Anchorage",
	"Hawaiian Standard Time":         "Pacific/Honolulu",
	"GMT Standard Time":              "Europe/London",
	"Greenwich Standard Time":        "Atlantic/Reykjavik",
	"W. Europe Standard Time":        "Europe/Berlin",
	"Romance Standard Time":          "Europe/Paris",
	"Central Europe Standard Time":   "Europe/Budapest",
	"Central European Standard Time": "Europe/Warsaw",
	"E. Europe Standard Time":        "Europe/Chisinau",
	"FLE Standard Time":              "Europe/Kiev",
	"Russian Standard Time":          "Europe/Moscow",
	"India Standard Time":            "Asia/Kolkata",
	"China Standard Time":            "Asia/Shanghai",
	"Tokyo Standard Time":            "Asia/Tokyo",
	"AUS Eastern Standard Time":      "Australia/Sydney",
	"New Zealand Standard Time":      "Pacific/Auckland",
	"UTC":                            "UTC",
}

func zone(tzid string, local *time.Location) *time.Location {
	tzid = strings.Trim(strings.TrimSpace(tzid), `"/`)
	if tzid == "" {
		return local
	}
	if w, ok := windowsZones[tzid]; ok {
		tzid = w
	}
	if loc, err := time.LoadLocation(tzid); err == nil {
		return loc
	}
	// Some exporters write /mozilla.org/20050126_1/Europe/Berlin and the like.
	if i := strings.Index(tzid, "/"); i >= 0 {
		parts := strings.Split(tzid, "/")
		for j := range parts {
			if loc, err := time.LoadLocation(strings.Join(parts[j:], "/")); err == nil {
				return loc
			}
		}
	}
	return local
}

// When is a DATE or DATE-TIME value.
type When struct {
	Time   time.Time // in the caller's zone
	AllDay bool
}

// ParseWhen reads a DTSTART/DTEND/DUE value into local time; local is where
// a floating time (no Z, no TZID) is taken to be.
func ParseWhen(p *Prop, local *time.Location) (When, bool) {
	if p == nil {
		return When{}, false
	}
	v := strings.TrimSpace(p.Value)
	if strings.EqualFold(p.Params["VALUE"], "DATE") || len(v) == 8 {
		t, err := time.ParseInLocation("20060102", v[:min(8, len(v))], local)
		return When{Time: t, AllDay: true}, err == nil
	}
	if strings.HasSuffix(v, "Z") {
		t, err := time.Parse("20060102T150405Z", v)
		return When{Time: t.In(local)}, err == nil
	}
	t, err := time.ParseInLocation("20060102T150405", v, zone(p.Params["TZID"], local))
	if err != nil {
		t, err = time.ParseInLocation("20060102T1504", v, zone(p.Params["TZID"], local))
	}
	return When{Time: t.In(local)}, err == nil
}

var durRe = regexp.MustCompile(`^([+-])?P(?:(\d+)W)?(?:(\d+)D)?(?:T(?:(\d+)H)?(?:(\d+)M)?(?:(\d+)S)?)?$`)

// ParseDuration reads P1DT2H30M, PT45M, P2W.
func ParseDuration(s string) (time.Duration, bool) {
	m := durRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0, false
	}
	n := func(i int) time.Duration {
		v, _ := strconv.Atoi(m[i])
		return time.Duration(v)
	}
	d := n(2)*7*24*time.Hour + n(3)*24*time.Hour + n(4)*time.Hour + n(5)*time.Minute + n(6)*time.Second
	if m[1] == "-" {
		d = -d
	}
	return d, true
}

// Rule is the part of an RRULE org can say.
type Rule struct {
	Freq     string // DAILY WEEKLY MONTHLY YEARLY HOURLY
	Interval int
	// Whether the rule says more than "every N units" (an end, several days a
	// week, the second Tuesday): org's repeater cannot, so the rule is kept
	// beside it as written.
	Partial bool
}

// ParseRule reads an RRULE value.
func ParseRule(v string, start time.Time) (Rule, bool) {
	r := Rule{Interval: 1}
	for _, kv := range strings.Split(v, ";") {
		i := strings.IndexByte(kv, '=')
		if i < 0 {
			continue
		}
		k, val := strings.ToUpper(kv[:i]), strings.ToUpper(kv[i+1:])
		switch k {
		case "FREQ":
			r.Freq = val
		case "INTERVAL":
			if n, err := strconv.Atoi(val); err == nil && n > 0 {
				r.Interval = n
			}
		case "BYDAY":
			// One day, the day it starts on, is what a weekly repeat means
			// anyway; anything else org cannot say.
			days := strings.Split(val, ",")
			if !(len(days) == 1 && r.Freq != "MONTHLY" && r.Freq != "YEARLY" &&
				strings.EqualFold(days[0], strings.ToUpper(start.Weekday().String()[:2]))) {
				r.Partial = true
			}
		case "WKST":
		default:
			r.Partial = true
		}
	}
	switch r.Freq {
	case "DAILY", "WEEKLY", "MONTHLY", "YEARLY", "HOURLY":
		return r, true
	}
	return r, false
}

// Cookie is the rule as org's repeater: +1w.
func (r Rule) Cookie() string {
	unit := map[string]string{"DAILY": "d", "WEEKLY": "w", "MONTHLY": "m", "YEARLY": "y", "HOURLY": "h"}[r.Freq]
	if unit == "" {
		return ""
	}
	return fmt.Sprintf("+%d%s", r.Interval, unit)
}

var cookieRe = regexp.MustCompile(`^(?:\.\+|\+\+|\+)(\d+)([hdwmy])`)

// RuleFromCookie is an org repeater (+1w, .+2d, ++1m) as an RRULE value.
func RuleFromCookie(cookie string) string {
	m := cookieRe.FindStringSubmatch(strings.TrimSpace(cookie))
	if m == nil {
		return ""
	}
	freq := map[string]string{"h": "HOURLY", "d": "DAILY", "w": "WEEKLY", "m": "MONTHLY", "y": "YEARLY"}[m[2]]
	if m[1] == "1" {
		return "FREQ=" + freq
	}
	return "FREQ=" + freq + ";INTERVAL=" + m[1]
}
