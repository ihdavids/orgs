package common

// org-recur's rules: a recurrence written in a heading's title between bars,
// `* TODO Water plants |+2|`. Pure - title in, dates out - so the server's
// DONE path, `orgs recur explain` (no server needed) and the query function
// all read a rule the same way. The TypeScript twin is worg's src/recur.ts
// and vscode-org-everywhere's src/recur-model.ts; the cases in recur_test.go
// are the cases in recur.test.ts and recur-model.test.ts.
//
// What is between the bars is a comma separated list of options; the rule's
// next date is the earliest of them. org-recur hands each option to
// org-read-date, so these are org-read-date's spellings:
//
//	+2  +2d  +w  +2w  +1m  +1y  ++3   that long from today (+w is +1w)
//	+0  .                             today
//	+tue  +2tue  -fri                 the next (second) Tuesday after today; last Friday
//	mon  monday  thu ...              the next such weekday
//	1  15  31                         the next such day of a month
//	wkdy                              every weekday (mon,tue,wed,thu,fri, configurable)
//	12-25  dec 25  25 dec  12/25      the next such day of a year
//	2026-12-25                        that date
//	10:00  3pm  9:30-10:00            a time, alone (today, else tomorrow) or with any of the above
//
// One deliberate difference from org-read-date: a weekday, day of the month
// or day of the year that is today counts as next time, not today. Done on a
// Thursday, `|Thu|` is next Thursday - the rule runs when the task is
// finished, and a task finished today is not due again today.

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// DefaultRecurWeekdays is what `wkdy` stands for unless recur.weekdays says
// otherwise (org-recur-weekday-recurrence).
const DefaultRecurWeekdays = "mon,tue,wed,thu,fri"

// RecurOption is one comma separated option of a rule.
type RecurOption struct {
	Text string // as written (after wkdy is expanded)
	// delta, weekday, monthday, yearday, date or time
	Kind string
	// delta: N units from today; Unit d, w, m, y, or "" with DeltaDay a weekday
	N        int
	Unit     string
	DeltaDay int // 0-6 for +2tue, else -1
	Weekday  int // weekday: 0 Sunday .. 6 Saturday
	Day      int // monthday, yearday, date
	Month    int // yearday, date
	Year     int // date
	HasTime  bool
	Hour     int
	Min      int
	HasEnd   bool
	EndHour  int
	EndMin   int
	Wkdy     bool // came from wkdy
}

// RecurRule is a whole |rule|.
type RecurRule struct {
	Text    string // between the bars, as written
	Options []RecurOption
}

// RecurDate is where a rule goes next.
type RecurDate struct {
	Date    time.Time // with the time of day when HasTime
	HasTime bool
	HasEnd  bool
	EndHour int
	EndMin  int
}

// FindRecur finds the first |...| in a title or headline line and says what
// is between the bars and where the bars are (byte offsets, the first bar and
// one past the second). `||` is no rule, as in org-recur.
func FindRecur(title string) (inner string, start, end int, ok bool) {
	a := strings.Index(title, "|")
	if a < 0 {
		return "", 0, 0, false
	}
	b := strings.Index(title[a+1:], "|")
	if b < 0 {
		return "", 0, 0, false
	}
	b += a + 1
	inner = title[a+1 : b]
	if strings.TrimSpace(inner) == "" {
		return "", 0, 0, false
	}
	return inner, a, b + 1, true
}

// RecurOf is the rule a title carries, if it carries one that reads. A title
// with bars that are not a rule (`a | b | c`) has none.
func RecurOf(title, weekdays string) (RecurRule, bool) {
	inner, _, _, ok := FindRecur(title)
	if !ok {
		return RecurRule{}, false
	}
	r, err := ParseRecur(inner, weekdays)
	return r, err == nil
}

// StripRecur is the title without its |rule| (and the space it leaves).
func StripRecur(title string) string {
	_, a, b, ok := FindRecur(title)
	if !ok {
		return title
	}
	left := strings.TrimRight(title[:a], " \t")
	right := strings.TrimLeft(title[b:], " \t")
	switch {
	case left == "":
		return right
	case right == "":
		return left
	}
	return left + " " + right
}

// SetRecurIn puts a rule into a title: in place of the one there, else at
// the end. An empty rule takes the one there off. The rule may be written
// with or without its bars.
func SetRecurIn(title, rule string) string {
	rule = strings.Trim(strings.TrimSpace(rule), "|")
	rule = strings.TrimSpace(rule)
	_, a, b, ok := FindRecur(title)
	if rule == "" {
		if !ok {
			return title
		}
		return StripRecur(title)
	}
	if ok {
		return title[:a] + "|" + rule + "|" + title[b:]
	}
	t := strings.TrimRight(title, " \t")
	if t == "" {
		return "|" + rule + "|"
	}
	return t + " |" + rule + "|"
}

var (
	recurTimeRe  = regexp.MustCompile(`(?i)(?:^|\s)(\d{1,2}):(\d{2})(?:\s*-\s*(\d{1,2}):(\d{2}))?(?:\s|$)`)
	recurAmPmRe  = regexp.MustCompile(`(?i)(?:^|\s)(\d{1,2})(?::(\d{2}))?\s*(am|pm)(?:\s|$)`)
	recurDeltaRe = regexp.MustCompile(`^(\+\+|\+|-)(\d*)\s*([a-z]*)$`)
	recurIsoRe   = regexp.MustCompile(`^(\d{4})-(\d{1,2})-(\d{1,2})$`)
	recurMDRe    = regexp.MustCompile(`^(\d{1,2})[-/](\d{1,2})$`)
	recurNumRe   = regexp.MustCompile(`^\d{1,2}$`)
	recurMonDRe  = regexp.MustCompile(`^([a-z]+)\.?\s+(\d{1,2})$`)
	recurDMonRe  = regexp.MustCompile(`^(\d{1,2})\s+([a-z]+)\.?$`)
	recurWkdyRe  = regexp.MustCompile(`(?i)\bwkdy\b`)
)

var recurWeekdayNames = map[string]int{
	"sun": 0, "sunday": 0,
	"mon": 1, "monday": 1,
	"tue": 2, "tues": 2, "tuesday": 2,
	"wed": 3, "wednesday": 3,
	"thu": 4, "thur": 4, "thurs": 4, "thursday": 4,
	"fri": 5, "friday": 5,
	"sat": 6, "saturday": 6,
}

var recurMonthNames = map[string]int{
	"jan": 1, "january": 1, "feb": 2, "february": 2, "mar": 3, "march": 3,
	"apr": 4, "april": 4, "may": 5, "jun": 6, "june": 6, "jul": 7, "july": 7,
	"aug": 8, "august": 8, "sep": 9, "sept": 9, "september": 9,
	"oct": 10, "october": 10, "nov": 11, "november": 11, "dec": 12, "december": 12,
}

// ParseRecur reads a rule, with or without its bars. weekdays is what wkdy
// stands for ("" for org-recur's own mon,tue,wed,thu,fri).
func ParseRecur(rule, weekdays string) (RecurRule, error) {
	s := strings.TrimSpace(rule)
	if strings.HasPrefix(s, "|") && strings.HasSuffix(s, "|") && len(s) >= 2 {
		s = s[1 : len(s)-1]
	}
	out := RecurRule{Text: s}
	if strings.TrimSpace(weekdays) == "" {
		weekdays = DefaultRecurWeekdays
	}
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		// wkdy stands for each weekday, keeping what was written round it
		// (+wkdy, wkdy 10:00), as org-recur--recurrence-to-options does.
		if recurWkdyRe.MatchString(part) {
			for _, d := range strings.Split(weekdays, ",") {
				d = strings.TrimSpace(d)
				if d == "" {
					continue
				}
				o, err := parseRecurOption(recurWkdyRe.ReplaceAllLiteralString(part, d))
				if err != nil {
					return RecurRule{}, err
				}
				o.Wkdy = true
				out.Options = append(out.Options, o)
			}
			continue
		}
		o, err := parseRecurOption(part)
		if err != nil {
			return RecurRule{}, err
		}
		out.Options = append(out.Options, o)
	}
	if len(out.Options) == 0 {
		return RecurRule{}, fmt.Errorf("an empty rule: write a date between the bars, e.g. |+2| or |Mon,Thu|")
	}
	return out, nil
}

func parseRecurOption(text string) (RecurOption, error) {
	o := RecurOption{Text: text, DeltaDay: -1}
	s := strings.ToLower(strings.TrimSpace(text))
	// The time of day first, then whatever is left is the date.
	if m := recurTimeRe.FindStringSubmatchIndex(s); m != nil {
		o.HasTime = true
		o.Hour, _ = strconv.Atoi(s[m[2]:m[3]])
		o.Min, _ = strconv.Atoi(s[m[4]:m[5]])
		if m[6] >= 0 {
			o.HasEnd = true
			o.EndHour, _ = strconv.Atoi(s[m[6]:m[7]])
			o.EndMin, _ = strconv.Atoi(s[m[8]:m[9]])
		}
		s = strings.TrimSpace(s[:m[0]] + " " + s[m[1]:])
	} else if m := recurAmPmRe.FindStringSubmatchIndex(s); m != nil {
		o.HasTime = true
		o.Hour, _ = strconv.Atoi(s[m[2]:m[3]])
		if m[4] >= 0 {
			o.Min, _ = strconv.Atoi(s[m[4]:m[5]])
		}
		if o.Hour < 1 || o.Hour > 12 {
			return o, fmt.Errorf("%q: not a time", text)
		}
		pm := s[m[6]:m[7]] == "pm"
		if o.Hour == 12 {
			o.Hour = 0
		}
		if pm {
			o.Hour += 12
		}
		s = strings.TrimSpace(s[:m[0]] + " " + s[m[1]:])
	}
	if o.HasTime && (o.Hour > 23 || o.Min > 59 || o.EndHour > 23 || o.EndMin > 59) {
		return o, fmt.Errorf("%q: not a time", text)
	}
	switch {
	case s == "":
		if !o.HasTime {
			return o, fmt.Errorf("an empty option")
		}
		o.Kind = "time"
	case s == ".":
		o.Kind, o.N, o.Unit = "delta", 0, "d"
	case recurDeltaRe.MatchString(s):
		m := recurDeltaRe.FindStringSubmatch(s)
		o.Kind = "delta"
		o.N = 1
		if m[2] != "" {
			o.N, _ = strconv.Atoi(m[2])
		}
		if m[1] == "-" {
			o.N = -o.N
		}
		switch u := m[3]; u {
		case "", "d":
			o.Unit = "d"
			if m[2] == "" && u == "" {
				return o, fmt.Errorf("%q: a number of days, e.g. +2", text)
			}
		case "w", "m", "y":
			o.Unit = u
		default:
			wd, ok := recurWeekdayNames[u]
			if !ok {
				return o, fmt.Errorf("%q: %q is not d, w, m, y or a weekday", text, u)
			}
			if o.N == 0 {
				return o, fmt.Errorf("%q: +0 of a weekday", text)
			}
			o.DeltaDay = wd
		}
	case recurIsoRe.MatchString(s):
		m := recurIsoRe.FindStringSubmatch(s)
		o.Kind = "date"
		o.Year, _ = strconv.Atoi(m[1])
		o.Month, _ = strconv.Atoi(m[2])
		o.Day, _ = strconv.Atoi(m[3])
		if !validDay(o.Year, o.Month, o.Day) {
			return o, fmt.Errorf("%q: no such date", text)
		}
	case recurMDRe.MatchString(s):
		m := recurMDRe.FindStringSubmatch(s)
		o.Kind = "yearday"
		o.Month, _ = strconv.Atoi(m[1])
		o.Day, _ = strconv.Atoi(m[2])
		if !validDay(2000, o.Month, o.Day) {
			return o, fmt.Errorf("%q: no such day of the year", text)
		}
	case recurMonDRe.MatchString(s) || recurDMonRe.MatchString(s):
		var name, day string
		if m := recurMonDRe.FindStringSubmatch(s); m != nil {
			name, day = m[1], m[2]
		} else {
			m := recurDMonRe.FindStringSubmatch(s)
			name, day = m[2], m[1]
		}
		mo, ok := recurMonthNames[name]
		if !ok {
			return o, fmt.Errorf("%q: %q is not a month", text, name)
		}
		o.Kind, o.Month = "yearday", mo
		o.Day, _ = strconv.Atoi(day)
		if !validDay(2000, o.Month, o.Day) {
			return o, fmt.Errorf("%q: no such day of the year", text)
		}
	case recurNumRe.MatchString(s):
		o.Kind = "monthday"
		o.Day, _ = strconv.Atoi(s)
		if o.Day < 1 || o.Day > 31 {
			return o, fmt.Errorf("%q: a day of the month is 1 to 31", text)
		}
	default:
		wd, ok := recurWeekdayNames[s]
		if !ok {
			return o, fmt.Errorf("%q is not a date org-recur reads: try +2, +w, Mon, 15 or wkdy", text)
		}
		o.Kind, o.Weekday = "weekday", wd
	}
	return o, nil
}

// validDay: 2000 is a leap year, so Feb 29 passes as a day of the year.
func validDay(y, m, d int) bool {
	if m < 1 || m > 12 || d < 1 {
		return false
	}
	t := time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
	return t.Day() == d && int(t.Month()) == m
}

func dayOf(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// next is where one option lands, counted from now. Every date is a day
// after today except a delta (+0 is today, -1 yesterday), a fixed date and
// a time alone (today if that time is still to come).
func (o RecurOption) next(now time.Time) time.Time {
	today := dayOf(now)
	var d time.Time
	switch o.Kind {
	case "delta":
		switch {
		case o.DeltaDay >= 0:
			if o.N > 0 {
				k := (o.DeltaDay - int(today.Weekday()) + 7) % 7
				if k == 0 {
					k = 7
				}
				d = today.AddDate(0, 0, k+7*(o.N-1))
			} else {
				k := (int(today.Weekday()) - o.DeltaDay + 7) % 7
				if k == 0 {
					k = 7
				}
				d = today.AddDate(0, 0, -k+7*(o.N+1))
			}
		case o.Unit == "w":
			d = today.AddDate(0, 0, 7*o.N)
		case o.Unit == "m":
			d = today.AddDate(0, o.N, 0)
		case o.Unit == "y":
			d = today.AddDate(o.N, 0, 0)
		default:
			d = today.AddDate(0, 0, o.N)
		}
	case "weekday":
		k := (o.Weekday - int(today.Weekday()) + 7) % 7
		if k == 0 {
			k = 7
		}
		d = today.AddDate(0, 0, k)
	case "monthday":
		// The next month that has that day: |31| skips the short months
		// rather than spilling into the next one.
		for i := 0; i < 48; i++ {
			c := time.Date(today.Year(), today.Month()+time.Month(i), o.Day, 0, 0, 0, 0, today.Location())
			if c.Day() == o.Day && c.After(today) {
				d = c
				break
			}
		}
	case "yearday":
		for i := 0; i < 9; i++ {
			c := time.Date(today.Year()+i, time.Month(o.Month), o.Day, 0, 0, 0, 0, today.Location())
			if c.Day() == o.Day && c.After(today) {
				d = c
				break
			}
		}
	case "date":
		d = time.Date(o.Year, time.Month(o.Month), o.Day, 0, 0, 0, 0, today.Location())
	case "time":
		d = today
		if !time.Date(d.Year(), d.Month(), d.Day(), o.Hour, o.Min, 0, 0, d.Location()).After(now) {
			d = d.AddDate(0, 0, 1)
		}
	}
	if o.HasTime {
		d = time.Date(d.Year(), d.Month(), d.Day(), o.Hour, o.Min, 0, 0, d.Location())
	}
	return d
}

// Next is where the rule goes when the task is done at now: the earliest of
// its options.
func (r RecurRule) Next(now time.Time) RecurDate {
	var best RecurDate
	for i, o := range r.Options {
		d := o.next(now)
		if i == 0 || d.Before(best.Date) {
			best = RecurDate{Date: d, HasTime: o.HasTime, HasEnd: o.HasEnd, EndHour: o.EndHour, EndMin: o.EndMin}
		}
	}
	return best
}

// NextN is the next n dates, each one where the rule goes when the task is
// done on the date before it. Stops early when a rule cannot move on (a
// fixed date, +0).
func (r RecurRule) NextN(now time.Time, n int) []RecurDate {
	out := []RecurDate{}
	at := now
	for len(out) < n {
		d := r.Next(at)
		if len(out) > 0 && !dayOf(d.Date).After(dayOf(out[len(out)-1].Date)) {
			break
		}
		out = append(out, d)
		at = time.Date(d.Date.Year(), d.Date.Month(), d.Date.Day(), 23, 59, 59, 0, d.Date.Location())
	}
	return out
}

// Day is the date part as org writes it, 2026-11-05 Thu.
func (d RecurDate) Day() string { return d.Date.Format("2006-01-02 Mon") }

// Clock is the time of day, 10:00 or 10:00-11:00, or "".
func (d RecurDate) Clock() string {
	if !d.HasTime {
		return ""
	}
	s := d.Date.Format("15:04")
	if d.HasEnd {
		s += fmt.Sprintf("-%02d:%02d", d.EndHour, d.EndMin)
	}
	return s
}

// String is the date and time, 2026-11-05 Thu 10:00.
func (d RecurDate) String() string {
	if c := d.Clock(); c != "" {
		return d.Day() + " " + c
	}
	return d.Day()
}

var recurDayWords = []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}
var recurMonthWords = []string{"", "January", "February", "March", "April", "May", "June", "July",
	"August", "September", "October", "November", "December"}

func ordinal(n int) string {
	suf := "th"
	if n%100 < 11 || n%100 > 13 {
		switch n % 10 {
		case 1:
			suf = "st"
		case 2:
			suf = "nd"
		case 3:
			suf = "rd"
		}
	}
	return strconv.Itoa(n) + suf
}

func plural(n int, one string) string {
	if n == 1 {
		return one
	}
	return strconv.Itoa(n) + " " + one + "s"
}

func andList(xs []string) string {
	switch len(xs) {
	case 0:
		return ""
	case 1:
		return xs[0]
	}
	return strings.Join(xs[:len(xs)-1], ", ") + " and " + xs[len(xs)-1]
}

func (o RecurOption) clockWords() string {
	if !o.HasTime {
		return ""
	}
	s := fmt.Sprintf(" at %02d:%02d", o.Hour, o.Min)
	if o.HasEnd {
		s += fmt.Sprintf("-%02d:%02d", o.EndHour, o.EndMin)
	}
	return s
}

func (o RecurOption) words() string {
	switch o.Kind {
	case "delta":
		if o.DeltaDay >= 0 {
			if o.N > 0 {
				if o.N == 1 {
					return "the next " + recurDayWords[o.DeltaDay]
				}
				return "the " + ordinal(o.N) + " " + recurDayWords[o.DeltaDay] + " from then"
			}
			if o.N == -1 {
				return "the " + recurDayWords[o.DeltaDay] + " before"
			}
			return ordinal(-o.N) + " " + recurDayWords[o.DeltaDay] + " before"
		}
		unit := map[string]string{"d": "day", "w": "week", "m": "month", "y": "year"}[o.Unit]
		switch {
		case o.N == 0:
			return "the same day"
		case o.N < 0:
			return plural(-o.N, unit) + " before"
		case o.N == 1:
			return "every " + unit
		}
		return "every " + plural(o.N, unit)
	case "weekday":
		return "every " + recurDayWords[o.Weekday]
	case "monthday":
		return "the " + ordinal(o.Day) + " of each month"
	case "yearday":
		return "every " + strconv.Itoa(o.Day) + " " + recurMonthWords[o.Month]
	case "date":
		return fmt.Sprintf("on %04d-%02d-%02d", o.Year, o.Month, o.Day)
	case "time":
		return "every day"
	}
	return o.Text
}

// Words is the rule said in English: |Mon,Thu| is "every Monday and
// Thursday", |1,15| "the 1st and 15th of each month", |wkdy| "every weekday".
func (r RecurRule) Words() string {
	type group struct {
		clock string
		kind  string
		items []string
	}
	groups := []*group{}
	find := func(kind, clock string) *group {
		for _, g := range groups {
			if g.kind == kind && g.clock == clock {
				return g
			}
		}
		g := &group{kind: kind, clock: clock}
		groups = append(groups, g)
		return g
	}
	seenWkdy := map[string]bool{}
	for _, o := range r.Options {
		c := o.clockWords()
		switch {
		case o.Wkdy && o.Kind == "weekday":
			if !seenWkdy[c] {
				seenWkdy[c] = true
				find("other", c).items = append(find("other", c).items, "every weekday")
			}
		case o.Kind == "weekday":
			g := find("weekday", c)
			g.items = append(g.items, recurDayWords[o.Weekday])
		case o.Kind == "monthday":
			g := find("monthday", c)
			g.items = append(g.items, ordinal(o.Day))
		default:
			g := find("other"+strconv.Itoa(len(groups)), c)
			g.items = append(g.items, o.words())
		}
	}
	parts := []string{}
	for _, g := range groups {
		switch g.kind {
		case "weekday":
			parts = append(parts, "every "+andList(g.items)+g.clock)
		case "monthday":
			parts = append(parts, "the "+andList(g.items)+" of each month"+g.clock)
		default:
			for _, it := range g.items {
				parts = append(parts, it+g.clock)
			}
		}
	}
	return strings.Join(parts, ", or ")
}

// RecurExamples are rules a client can offer to pick from.
var RecurExamples = []string{"+1", "+2", "+w", "+2w", "+1m", "+1y", "wkdy", "Mon", "Mon,Thu", "Sat,Sun", "1", "1,15", "Mon 09:00"}
