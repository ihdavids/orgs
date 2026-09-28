package commands

// The dates a person types, and the timestamps org writes.
//
// `orgs sched tomorrow`, `orgs sched +2w`, `orgs sched fri 14:00`, `orgs sched
// clear`. Nobody types `<2026-09-28 Mon>` at a prompt, and a tool that asks
// them to is a tool they keep a date calculator open beside. So this is the one
// place a date is read, shared by `sched`, `deadline`, `close` and the clock
// report's range - which is the point of it being here rather than in a verb:
// three verbs that disagree about what "+2w" means are worse than one that is
// wrong about it.
//
// What it will not do is guess at an ambiguous date. `1/10` is the first of
// October to most of the world and the tenth of January to some of it, and the
// answer goes silently into a file to be read a year later; it is refused and
// the refusal says to write the month first. Everything else here has exactly
// one reading.

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// OrgDate is a timestamp on the way to a file: the day, optionally a time or a
// range, optionally a repeater, and whether it is active (angle brackets, which
// the agenda reads) or inactive (square, which it does not).
type OrgDate struct {
	Day      time.Time
	HasTime  bool
	End      time.Time // the far end of a time range; zero when there is none
	HasEnd   bool
	Repeater string // "+1w", ".+2d", "++1m" - written as typed
	Inactive bool
	// A timestamp that arrived already written, to be passed through as it
	// stands. Somebody pasting one from a file means it, and re-deriving it
	// would only be a chance to get it wrong.
	Raw string
}

// String is the timestamp as org writes one.
func (d OrgDate) String() string {
	if d.Raw != "" {
		return d.Raw
	}
	lo, hi := "<", ">"
	if d.Inactive {
		lo, hi = "[", "]"
	}
	s := d.Day.Format("2006-01-02 Mon")
	if d.HasTime {
		s += " " + d.Day.Format("15:04")
		if d.HasEnd {
			s += "-" + d.End.Format("15:04")
		}
	}
	if d.Repeater != "" {
		s += " " + d.Repeater
	}
	return lo + s + hi
}

// ParseDate reads what somebody typed as a date. The bool is whether a date was
// asked for at all: false with no error is "clear this date", which is what the
// server reads an empty Value as - so `orgs sched clear` and `orgs sched ”` are
// the same write and neither needs a flag of its own.
//
// now is passed in rather than read, so the parser is a function of its
// arguments and can be tested without waiting for midnight.
func ParseDate(s string, now time.Time) (OrgDate, bool, error) {
	s = strings.TrimSpace(s)
	low := strings.ToLower(s)

	switch low {
	case "", "clear", "none", "off", "remove", "nil", "-":
		return OrgDate{}, false, nil
	}

	if (strings.HasPrefix(s, "<") && strings.HasSuffix(s, ">")) ||
		(strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]")) {
		return OrgDate{Raw: s}, true, nil
	}

	d := OrgDate{Day: dayOf(now)}

	// The time is picked out wherever it was written, because "fri 14:00" and
	// "14:00 fri" are the same sentence and arguing about which is correct is
	// not a service to anybody.
	rest := []string{}
	for _, w := range strings.Fields(low) {
		if looksLikeTime(w) {
			if err := applyTime(&d, w); err != nil {
				return d, true, err
			}
			continue
		}
		rest = append(rest, w)
	}

	// Then the day, then the repeater. In that order and not the other way
	// round: `+2w` is both an offset and a repeater in org's spelling, and the
	// only thing that tells them apart is that a repeater needs a day to
	// repeat from. So the first word that reads as a day *is* the day, and a
	// `+2w` after one is the repeater.
	day := d.Day
	haveDay := false
	for _, w := range rest {
		if !haveDay {
			if t, err := parseDay(w, now); err == nil {
				day, haveDay = t, true
				continue
			}
		}
		if isRepeater(w) {
			d.Repeater = w
			continue
		}
		// Not a day and not a repeater: say what is wrong with it as a day,
		// which is what it was most likely meant to be.
		if _, err := parseDay(w, now); err != nil {
			return d, true, err
		}
	}

	if !haveDay {
		if !d.HasTime {
			return d, true, fmt.Errorf("no date in %q", s)
		}
		// A time with no day is today, which is what "14:00" means when
		// somebody says it out loud.
		return d, true, nil
	}

	// Move the parsed time onto the day that was found.
	if d.HasTime {
		d.Day = withDay(day, d.Day)
		if d.HasEnd {
			d.End = withDay(day, d.End)
		}
	} else {
		d.Day = day
	}
	return d, true, nil
}

// ParseDateToOrg is ParseDate for the common case: hand back the string to put
// in the file, or the empty string to clear it.
func ParseDateToOrg(s string, now time.Time) (string, error) {
	d, set, err := ParseDate(s, now)
	if err != nil {
		return "", err
	}
	if !set {
		return "", nil
	}
	return d.String(), nil
}

// withDay is clock from one time, calendar from another.
func withDay(day, clock time.Time) time.Time {
	return time.Date(day.Year(), day.Month(), day.Day(), clock.Hour(), clock.Minute(), 0, 0,
		day.Location())
}

func dayOf(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// parseDay reads the day half: a name, an offset, or a written date.
func parseDay(s string, now time.Time) (time.Time, error) {
	today := dayOf(now)
	switch s {
	case "today", "tod":
		return today, nil
	case "now":
		return today, nil
	case "tomorrow", "tom", "tmr":
		return today.AddDate(0, 0, 1), nil
	case "yesterday", "yest":
		return today.AddDate(0, 0, -1), nil
	case "eow", "endofweek":
		// Friday of this week, which is what "end of the week" means about work.
		return forward(today, time.Friday), nil
	case "eom", "endofmonth":
		return time.Date(today.Year(), today.Month()+1, 1, 0, 0, 0, 0, today.Location()).
			AddDate(0, 0, -1), nil
	case "eoy", "endofyear":
		return time.Date(today.Year(), 12, 31, 0, 0, 0, 0, today.Location()), nil
	}

	if wd, ok := weekdays[s]; ok {
		// Org's reading: the coming one, today included when today is that day.
		return forward(today, wd), nil
	}

	// An offset: +3d, -1w, +2m, +1y. The sign is required, which is what tells
	// an offset from a date.
	if len(s) > 1 && (s[0] == '+' || s[0] == '-') {
		return offset(today, s)
	}

	// A written date. Only the unambiguous forms, month first.
	for _, layout := range []string{"2006-01-02", "2006/01/02", "01-02", "01/02"} {
		if t, err := time.ParseInLocation(layout, s, now.Location()); err == nil {
			if t.Year() == 0 {
				// A day and month with no year is this year, or next year when
				// this year's has already gone - which is what somebody
				// writing "12-25" in January and in December both mean.
				t = time.Date(now.Year(), t.Month(), t.Day(), 0, 0, 0, 0, now.Location())
				if t.Before(today) {
					t = t.AddDate(1, 0, 0)
				}
			}
			return t, nil
		}
	}
	// A bare day of the month: "15" is the 15th, this month or next.
	if n, err := strconv.Atoi(s); err == nil && n >= 1 && n <= 31 {
		t := time.Date(now.Year(), now.Month(), n, 0, 0, 0, 0, now.Location())
		if t.Before(today) {
			t = t.AddDate(0, 1, 0)
		}
		return t, nil
	}

	if strings.Contains(s, "/") || (strings.Count(s, "-") == 1 && len(s) <= 5) {
		return time.Time{}, fmt.Errorf("%q could be two dates - write the month first, 2026-10-01 or 10-01", s)
	}
	return time.Time{}, fmt.Errorf("%q is not a date I can read - try today, tomorrow, fri, +2w or 2026-10-01", s)
}

var weekdays = map[string]time.Weekday{
	"mon": time.Monday, "monday": time.Monday,
	"tue": time.Tuesday, "tues": time.Tuesday, "tuesday": time.Tuesday,
	"wed": time.Wednesday, "weds": time.Wednesday, "wednesday": time.Wednesday,
	"thu": time.Thursday, "thur": time.Thursday, "thurs": time.Thursday, "thursday": time.Thursday,
	"fri": time.Friday, "friday": time.Friday,
	"sat": time.Saturday, "saturday": time.Saturday,
	"sun": time.Sunday, "sunday": time.Sunday,
}

// forward is the next day with this weekday, today counting as one.
func forward(from time.Time, wd time.Weekday) time.Time {
	delta := (int(wd) - int(from.Weekday()) + 7) % 7
	return from.AddDate(0, 0, delta)
}

func offset(from time.Time, s string) (time.Time, error) {
	sign := 1
	if s[0] == '-' {
		sign = -1
	}
	body := s[1:]
	unit := byte('d')
	if len(body) > 0 {
		last := body[len(body)-1]
		if last < '0' || last > '9' {
			unit = last
			body = body[:len(body)-1]
		}
	}
	n, err := strconv.Atoi(body)
	if err != nil {
		return time.Time{}, fmt.Errorf("%q is not an offset - try +3d, +2w, +1m or +1y", s)
	}
	n *= sign
	switch unit {
	case 'd':
		return from.AddDate(0, 0, n), nil
	case 'w':
		return from.AddDate(0, 0, n*7), nil
	case 'm':
		return from.AddDate(0, n, 0), nil
	case 'y':
		return from.AddDate(n, 0, 0), nil
	}
	return time.Time{}, fmt.Errorf("%q: d, w, m or y", s)
}

// A repeater as org writes one: +1w every week from the last one, ++1w from
// today, .+1w from when it was done. Passed through as typed - org's three
// kinds mean different things and none of them is this program's to choose.
func isRepeater(w string) bool {
	if strings.HasPrefix(w, "++") || strings.HasPrefix(w, ".+") {
		return len(w) > 2
	}
	if strings.HasPrefix(w, "+") && len(w) > 1 {
		last := w[len(w)-1]
		return last == 'd' || last == 'w' || last == 'm' || last == 'y' || last == 'h'
	}
	return false
}

func looksLikeTime(w string) bool {
	if strings.Contains(w, ":") {
		return true
	}
	return strings.HasSuffix(w, "am") || strings.HasSuffix(w, "pm")
}

func applyTime(d *OrgDate, w string) error {
	parts := strings.SplitN(w, "-", 2)
	t, err := oneTime(parts[0])
	if err != nil {
		return err
	}
	d.Day = time.Date(d.Day.Year(), d.Day.Month(), d.Day.Day(), t.Hour(), t.Minute(), 0, 0,
		d.Day.Location())
	d.HasTime = true
	if len(parts) == 2 && parts[1] != "" {
		e, err := oneTime(parts[1])
		if err != nil {
			return err
		}
		d.End = time.Date(d.Day.Year(), d.Day.Month(), d.Day.Day(), e.Hour(), e.Minute(), 0, 0,
			d.Day.Location())
		d.HasEnd = true
	}
	return nil
}

func oneTime(s string) (time.Time, error) {
	for _, layout := range []string{"15:04", "3:04pm", "3pm", "15"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("%q is not a time - try 14:00, 2pm or 09:30", s)
}
