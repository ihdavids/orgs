package notionorg

import (
	"regexp"
	"strings"
	"time"
)

// Notion writes a date the way its date property shows it: "October 3, 2026",
// with a time when the property has one ("October 3, 2026 10:00 AM", or
// 24-hour "October 3, 2026 14:00"), and a range with an arrow between the
// two ends, the second end sometimes just a time on the same day.
var (
	dateLayouts = []string{"January 2, 2006 3:04 PM", "January 2, 2006 15:04", "January 2, 2006",
		"2006-01-02 15:04", "2006-01-02T15:04:05Z07:00", "2006-01-02T15:04:05.000Z07:00", "2006-01-02",
		"2006/01/02 15:04", "2006/01/02", "Jan 2, 2006 3:04 PM", "Jan 2, 2006"}
	timeLayouts = []string{"3:04 PM", "15:04"}
	// A trailing zone Notion adds to some times: "(GMT-7)", "(PDT)".
	zoneTail = regexp.MustCompile(`\s*\([A-Za-z]{2,5}(?:[+-]\d{1,2}(?::\d\d)?)?\)\s*$`)
)

// parseDate reads one end of a Notion date; hasTime says whether it named a
// time of day.
func parseDate(s string) (time.Time, bool, bool) {
	s = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s), "@"))
	s = zoneTail.ReplaceAllString(s, "")
	for _, l := range dateLayouts {
		if t, err := time.Parse(l, s); err == nil {
			return t, strings.Contains(l, "15") || strings.Contains(l, "3:04"), true
		}
	}
	return time.Time{}, false, false
}

func parseClock(s string) (time.Time, bool) {
	s = zoneTail.ReplaceAllString(strings.TrimSpace(s), "")
	for _, l := range timeLayouts {
		if t, err := time.Parse(l, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// OrgDate turns a Notion date or range into an org timestamp: active, or
// inactive ([…]) for when something happened rather than is to (a Created
// or Last edited column). ok is false for anything that is not a date.
func OrgDate(s string, inactive bool) (string, bool) {
	open, close := "<", ">"
	if inactive {
		open, close = "[", "]"
	}
	stamp := func(t time.Time, withTime bool, end string) string {
		f := "2006-01-02 Mon"
		if withTime {
			f += " 15:04"
		}
		return open + t.Format(f) + end + close
	}
	parts := strings.Split(s, "→")
	if len(parts) == 1 {
		parts = strings.Split(s, " -> ")
	}
	switch len(parts) {
	case 1:
		t, tm, ok := parseDate(parts[0])
		if !ok {
			return "", false
		}
		return stamp(t, tm, ""), true
	case 2:
		a, atm, ok := parseDate(parts[0])
		if !ok {
			return "", false
		}
		// The far end on the same day, as only a time: <d 10:00-11:00>.
		if c, ok := parseClock(parts[1]); ok && atm {
			return stamp(a, true, "-"+c.Format("15:04")), true
		}
		b, btm, ok := parseDate(parts[1])
		if !ok {
			return "", false
		}
		if atm && btm && a.Format("2006-01-02") == b.Format("2006-01-02") {
			return stamp(a, true, "-"+b.Format("15:04")), true
		}
		return stamp(a, atm, "") + "--" + stamp(b, btm, ""), true
	}
	return "", false
}

// Whether a column is when a row was made or last touched, which is history:
// its stamps are inactive and stay off the agenda.
func historyColumn(name string) bool {
	n := strings.ToLower(name)
	return strings.Contains(n, "created") || strings.Contains(n, "edited") || strings.Contains(n, "updated")
}

// Status values that clearly say a task's state. A Status column becomes the
// heading's keyword only when every value in it is one of these: a Status
// column of "Draft"/"Published" is a property, not a todo list.
var (
	doneWords = map[string]bool{"done": true, "complete": true, "completed": true, "finished": true, "closed": true}
	openWords = map[string]bool{"not started": true, "to do": true, "todo": true, "to-do": true, "in progress": true,
		"in-progress": true, "doing": true, "started": true, "next": true, "waiting": true, "blocked": true,
		"backlog": true, "planned": true, "open": true}
)

func statusColumn(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	return n == "status" || n == "state" || n == "stage"
}

func doneColumn(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "done", "completed", "complete", "finished":
		return true
	}
	return false
}

func dueColumn(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "due", "due date", "deadline":
		return true
	}
	return false
}

func tagsColumn(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "tags", "tag", "labels":
		return true
	}
	return false
}

var propBad = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

// PropName is a column's name as an org property: "Due Date" → DUE_DATE,
// which records then read as a date by its suffix.
func PropName(k string) string {
	p := strings.ToUpper(strings.Trim(propBad.ReplaceAllString(strings.TrimSpace(k), "_"), "_-"))
	if p == "" {
		return "FIELD"
	}
	return p
}

// Collection is a database's title as a record collection: "Reading List" →
// reading-list.
func Collection(title string) string {
	b := strings.Builder{}
	dash := false
	for _, r := range strings.ToLower(title) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r > 127 {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	s := strings.TrimSuffix(b.String(), "-")
	if s == "" {
		return "notion"
	}
	return s
}
