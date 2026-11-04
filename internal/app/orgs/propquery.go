package orgs

// Property searches as org's tags/property match has them (org manual 7.3,
// 7.4): a property's value compared as a date, a number, a duration or text,
// and properties inherited from the headings above and the file's
// #+PROPERTY: lines.
//
// Inheritance follows propertyInheritance in the yaml (org's
// org-use-property-inheritance): a list of names, or "*" for every one;
// unset, nothing is inherited but CATEGORY, which org always takes from
// above (org-get-category). HasProperty and MatchProperty follow it;
// InheritedProperty always inherits.

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
	"github.com/ihdavids/orgs/internal/orgdate"
)

// propInherits is whether a property's value is taken from above.
func propInherits(name string) bool {
	name = strings.ToUpper(strings.TrimSpace(name))
	if name == "CATEGORY" {
		return true
	}
	s := Conf().Server
	if s == nil {
		return false
	}
	for _, p := range s.PropertyInheritance {
		if p == "*" || strings.EqualFold(p, name) {
			return true
		}
	}
	return false
}

// fileProperty is a #+PROPERTY: NAME value line's value (NAME+ lines add to
// it, with a space).
func fileProperty(f *common.OrgFile, name string) (string, bool) {
	if f == nil || f.Doc == nil {
		return "", false
	}
	val, found := "", false
	for _, l := range strings.Split(f.Doc.BufferSettings["PROPERTY"], "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(l), " ")
		if !ok {
			continue
		}
		switch {
		case strings.EqualFold(k, name):
			val, found = strings.TrimSpace(v), true
		case strings.EqualFold(k, name+"+"):
			val, found = strings.TrimSpace(val+" "+strings.TrimSpace(v)), true
		}
	}
	return val, found
}

func sdcText(s *org.SDC) (string, bool) {
	if s == nil || s.Date == nil {
		return "", false
	}
	return s.Date.ToString(), true
}

// propValueOf is a property's value on a heading: org's special properties
// (TODO, PRIORITY, LEVEL, ITEM, CATEGORY, SCHEDULED, DEADLINE, CLOSED, TAGS),
// else its own drawer, else - when inherit - the headings above, outermost
// last, then the file's #+PROPERTY: lines.
func propValueOf(f *common.OrgFile, sec *org.Section, name string, inherit bool) (string, bool) {
	if sec == nil || sec.Headline == nil {
		return "", false
	}
	h := sec.Headline
	switch strings.ToUpper(name) {
	case "TODO":
		return h.Status, h.Status != ""
	case "PRIORITY":
		if h.Priority != "" {
			return h.Priority, true
		}
		return "B", true
	case "LEVEL":
		return strconv.Itoa(h.Lvl), true
	case "ITEM":
		return strings.TrimSpace(headlineTitle(sec)), true
	case "CATEGORY":
		return categoryOf(f, sec), true
	case "TAGS":
		if len(h.Tags) == 0 {
			return "", false
		}
		return ":" + strings.Join(h.Tags, ":") + ":", true
	case "SCHEDULED":
		return sdcText(h.Scheduled)
	case "DEADLINE":
		return sdcText(h.Deadline)
	case "CLOSED":
		return sdcText(h.Closed)
	}
	for s := sec; s != nil; s = s.Parent {
		if s.Headline != nil && s.Headline.Properties != nil {
			if v, ok := s.Headline.Properties.Get(name); ok {
				return v, true
			}
		}
		if !inherit {
			return "", false
		}
	}
	return fileProperty(f, name)
}

var (
	stampDayRe = regexp.MustCompile(`(\d{4}-\d{2}-\d{2})(?:\s+\w+)?(?:\s+(\d{1,2}:\d{2}))?`)
	numberRe   = regexp.MustCompile(`^[-+]?(\d+\.?\d*|\.\d+)$`)
	durationRe = regexp.MustCompile(`^\s*(\d+:\d{2}|(\d+(\.\d+)?\s*(y|w|d|h|min)\s*)+)\s*$`)
)

// propTime reads a value as a moment: a timestamp written in the file, or a
// date as typed in a query - <2026-11-01>, 2026-11-01, <today>, <+1w>, fri.
func propTime(s string, now time.Time) (time.Time, bool) {
	t := strings.TrimSpace(s)
	inner := strings.Trim(t, "<>[]")
	if m := stampDayRe.FindStringSubmatch(inner); m != nil && strings.HasPrefix(inner, m[1]) {
		layout, v := "2006-01-02", m[1]
		if m[2] != "" {
			layout, v = "2006-01-02 15:04", m[1]+" "+m[2]
		}
		if d, err := time.ParseInLocation(layout, v, time.Local); err == nil {
			return d, true
		}
	}
	switch strings.ToLower(inner) {
	case "now":
		return now, true
	case "today":
		y, mo, d := now.Date()
		return time.Date(y, mo, d, 0, 0, 0, 0, time.Local), true
	}
	if od, ok, err := orgdate.ParseDate(inner, now); ok && err == nil {
		return od.Day, true
	}
	return time.Time{}, false
}

// looksLikeDate is whether a query's value asks for a date comparison: in
// angle or square brackets, or an ISO day.
func looksLikeDate(s string) bool {
	s = strings.TrimSpace(s)
	return (strings.HasPrefix(s, "<") && strings.HasSuffix(s, ">")) || (strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]")) ||
		regexp.MustCompile(`^\d{4}-\d{2}-\d{2}`).MatchString(s)
}

func cmpResult(c int, op string) bool {
	switch op {
	case "<":
		return c < 0
	case "<=", "=<":
		return c <= 0
	case ">":
		return c > 0
	case ">=", "=>":
		return c >= 0
	case "=", "==":
		return c == 0
	case "!=", "<>":
		return c != 0
	}
	return false
}

func sign(x float64) int {
	switch {
	case x < 0:
		return -1
	case x > 0:
		return 1
	}
	return 0
}

// propCompare compares a property's value with want, the way org's match
// does: as moments when want is a date, as numbers when both are numbers, as
// minutes when both are durations (2:30, 2h, 1d 4h), else as text. A missing
// value is unequal to everything and nothing else.
func propCompare(val string, found bool, op, want string, now time.Time) bool {
	if !found {
		return op == "!=" || op == "<>"
	}
	if looksLikeDate(want) {
		a, okA := propTime(val, now)
		b, okB := propTime(want, now)
		if okA && okB {
			return cmpResult(a.Compare(b), op)
		}
		return op == "!=" || op == "<>"
	}
	if numberRe.MatchString(strings.TrimSpace(val)) && numberRe.MatchString(strings.TrimSpace(want)) {
		a, _ := strconv.ParseFloat(strings.TrimSpace(val), 64)
		b, _ := strconv.ParseFloat(strings.TrimSpace(want), 64)
		return cmpResult(sign(a-b), op)
	}
	if durationRe.MatchString(val) && durationRe.MatchString(want) {
		a, b := common.ParseDuration(val), common.ParseDuration(want)
		if a != nil && b != nil {
			return cmpResult(sign(a.Mins-b.Mins), op)
		}
	}
	return cmpResult(strings.Compare(val, want), op)
}
