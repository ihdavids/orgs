package ledger

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ihdavids/orgs/internal/orgdate"
)

// Period is a half open range of days: Begin included, End not. A zero end
// is open.
type Period struct {
	Begin time.Time
	End   time.Time
}

func (p Period) Contains(d time.Time) bool {
	if !p.Begin.IsZero() && d.Before(p.Begin) {
		return false
	}
	if !p.End.IsZero() && !d.Before(p.End) {
		return false
	}
	return true
}

// String is the period in ledger's words, for a heading.
func (p Period) String() string {
	switch {
	case p.Begin.IsZero() && p.End.IsZero():
		return ""
	case p.End.IsZero():
		return "from " + p.Begin.Format("2006-01-02")
	case p.Begin.IsZero():
		return "before " + p.End.Format("2006-01-02")
	}
	return p.Begin.Format("2006-01-02") + " to " + p.End.AddDate(0, 0, -1).Format("2006-01-02")
}

var monthNames = map[string]time.Month{
	"jan": 1, "january": 1, "feb": 2, "february": 2, "mar": 3, "march": 3, "apr": 4, "april": 4,
	"may": 5, "jun": 6, "june": 6, "jul": 7, "july": 7, "aug": 8, "august": 8,
	"sep": 9, "sept": 9, "september": 9, "oct": 10, "october": 10, "nov": 11, "november": 11,
	"dec": 12, "december": 12,
}

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

// ParsePeriod reads a period the way ledger's -p does, in the words people
// type: "this month", "last year", "2026", "2026-11", "nov", "q3", "from
// 2026-01-01 to 2026-04-01", "jan..mar", "since last month", "until today".
// A single day (through internal/orgdate: "today", "fri", "2026-11-04") is
// that day. An end is exclusive, as ledger's is: "to mar" stops before March.
func ParsePeriod(s string, now time.Time) (Period, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" || s == "all" {
		return Period{}, nil
	}
	for _, sep := range []string{"..", " to ", " until "} {
		if at := strings.Index(s, sep); at > 0 {
			a, err := ParsePeriod(strings.TrimPrefix(s[:at], "from "), now)
			if err != nil {
				return Period{}, err
			}
			b, err := ParsePeriod(s[at+len(sep):], now)
			if err != nil {
				return Period{}, err
			}
			return Period{Begin: a.Begin, End: b.Begin}, nil
		}
	}
	for _, pre := range []string{"from ", "since ", "after "} {
		if strings.HasPrefix(s, pre) {
			a, err := ParsePeriod(s[len(pre):], now)
			return Period{Begin: a.Begin}, err
		}
	}
	for _, pre := range []string{"to ", "until ", "before "} {
		if strings.HasPrefix(s, pre) {
			a, err := ParsePeriod(s[len(pre):], now)
			return Period{End: a.Begin}, err
		}
	}
	today := day(now.Year(), now.Month(), now.Day())
	monthOf := func(t time.Time) Period {
		b := day(t.Year(), t.Month(), 1)
		return Period{b, b.AddDate(0, 1, 0)}
	}
	yearOf := func(y int) Period { return Period{day(y, 1, 1), day(y+1, 1, 1)} }
	weekOf := func(t time.Time) Period {
		back := (int(t.Weekday()) + 6) % 7 // weeks start on Monday
		b := t.AddDate(0, 0, -back)
		return Period{b, b.AddDate(0, 0, 7)}
	}
	quarterOf := func(y, q int) Period {
		b := day(y, time.Month(3*(q-1)+1), 1)
		return Period{b, b.AddDate(0, 3, 0)}
	}
	words := strings.Fields(s)
	if len(words) == 2 && (words[0] == "this" || words[0] == "last" || words[0] == "next" || words[0] == "current") {
		step := map[string]int{"this": 0, "current": 0, "last": -1, "next": 1}[words[0]]
		switch words[1] {
		case "day":
			b := today.AddDate(0, 0, step)
			return Period{b, b.AddDate(0, 0, 1)}, nil
		case "week":
			return weekOf(today.AddDate(0, 0, 7*step)), nil
		case "month":
			return monthOf(day(today.Year(), today.Month()+time.Month(step), 1)), nil
		case "quarter":
			q := (int(today.Month())-1)/3 + 1 + step
			y := today.Year()
			for q < 1 {
				q, y = q+4, y-1
			}
			for q > 4 {
				q, y = q-4, y+1
			}
			return quarterOf(y, q), nil
		case "year":
			return yearOf(today.Year() + step), nil
		}
	}
	switch s {
	case "today":
		return Period{today, today.AddDate(0, 0, 1)}, nil
	case "yesterday":
		return Period{today.AddDate(0, 0, -1), today}, nil
	case "monthly", "month":
		return monthOf(today), nil
	case "year", "yearly":
		return yearOf(today.Year()), nil
	}
	// 2026
	if len(s) == 4 {
		if y, err := strconv.Atoi(s); err == nil {
			return yearOf(y), nil
		}
	}
	// 2026-11, 2026/11
	if len(words) == 1 && (strings.Count(s, "-") == 1 || strings.Count(s, "/") == 1) && len(s) >= 6 && len(s) <= 7 {
		sep := "-"
		if strings.Contains(s, "/") {
			sep = "/"
		}
		parts := strings.Split(s, sep)
		y, e1 := strconv.Atoi(parts[0])
		m, e2 := strconv.Atoi(parts[1])
		if e1 == nil && e2 == nil && len(parts[0]) == 4 && m >= 1 && m <= 12 {
			return monthOf(day(y, time.Month(m), 1)), nil
		}
	}
	// q3, 2026q3, q3 2026
	if q, y, ok := quarterWords(words, today.Year()); ok {
		return quarterOf(y, q), nil
	}
	// nov, november 2025
	if m, ok := monthNames[words[0]]; ok && len(words) <= 2 {
		y := today.Year()
		if len(words) == 2 {
			n, err := strconv.Atoi(words[1])
			if err != nil {
				return Period{}, fmt.Errorf("%q is not a period", s)
			}
			y = n
		}
		return monthOf(day(y, m, 1)), nil
	}
	// One day, read the way every date in orgs is read.
	d, ok, err := orgdate.ParseDate(s, now)
	if err != nil || !ok {
		return Period{}, fmt.Errorf("%q is not a period I can read - try this month, last year, 2026, 2026-11, q3, or from 2026-01-01 to 2026-04-01", s)
	}
	b := day(d.Day.Year(), d.Day.Month(), d.Day.Day())
	return Period{b, b.AddDate(0, 0, 1)}, nil
}

func quarterWords(words []string, year int) (int, int, bool) {
	j := strings.Join(words, "")
	at := strings.Index(j, "q")
	if at < 0 || at+1 >= len(j) {
		return 0, 0, false
	}
	q := int(j[at+1] - '0')
	if q < 1 || q > 4 {
		return 0, 0, false
	}
	rest := j[:at] + j[at+2:]
	if rest == "" {
		return q, year, true
	}
	y, err := strconv.Atoi(rest)
	if err != nil || len(rest) != 4 {
		return 0, 0, false
	}
	return q, y, true
}

// Within combines a period with -b/-e style bounds; either may be empty.
func Within(period, begin, end string, now time.Time) (Period, error) {
	p, err := ParsePeriod(period, now)
	if err != nil {
		return p, err
	}
	if begin != "" {
		b, err := ParsePeriod(begin, now)
		if err != nil {
			return p, err
		}
		p.Begin = b.Begin
	}
	if end != "" {
		e, err := ParsePeriod(end, now)
		if err != nil {
			return p, err
		}
		p.End = e.Begin
	}
	return p, nil
}
