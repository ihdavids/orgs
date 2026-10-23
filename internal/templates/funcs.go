package templates

// The functions and filters a template has beyond pongo2's own, for the things
// org files are made of: a run of days, a date moved on, an id made from a
// title, a list of tags.
//
//	{% for d in days("mon", 5) %}* {{ d.weekday }} {{ d }}{% endfor %}
//	DEADLINE: {{ today|plus:"2w" }}       <2026-11-05 Thu>
//	{{ week_start|plus:"6d"|format:"date" }}  2026-10-25
//	:CUSTOM_ID: {{ title|slug }}             weekly-review
//	#+FILETAGS: {{ "work, q4 plans"|tags }}  :work:q4_plans:
//
// Dates are read the way they are written in a file - <2026-10-22 Thu>,
// [2026-10-22 Thu 09:00], 2026-10-22 - and written back the same way, so a
// filter changes the day and nothing else.

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/flosch/pongo2/v5"
	"github.com/ihdavids/orgs/internal/orgdate"
)

// Day is one day of days(): every clock value at that day, read as
// {{ d.weekday }}, and the day's active timestamp when written on its own.
type Day map[string]string

func (d Day) String() string { return d["today"] }

// dayValue is every clock value for a day, at that day's start.
func dayValue(t time.Time) Day {
	out := Day{}
	for _, n := range AutoNames {
		switch n {
		case "uuid", "shortuid", "hostname":
			continue
		}
		if v, ok := AutoValue(n, t); ok {
			out[n] = v
		}
	}
	for alias, n := range Aliases {
		if v, ok := out[n]; ok {
			out[alias] = v
		}
	}
	return out
}

// The most days() will make: a year and a bit, so a typo of 5000 does not
// write a file nobody can open.
const maxDays = 400

func days(now time.Time) func(spec string, n ...int) []Day {
	return func(spec string, n ...int) []Day {
		count := 7
		if len(n) > 0 {
			count = n[0]
		}
		if count < 1 {
			return nil
		}
		if count > maxDays {
			count = maxDays
		}
		start := now
		if st, ok := readStamp(spec); ok {
			// A timestamp ({{week_start}}): orgdate passes one through as
			// written, without reading its day.
			start = st.t
		} else if strings.TrimSpace(spec) != "" {
			d, set, err := orgdate.ParseDate(spec, now)
			if err != nil || !set || d.Day.IsZero() {
				return nil
			}
			start = d.Day
		}
		start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location())
		out := make([]Day, 0, count)
		for i := 0; i < count; i++ {
			out = append(out, dayValue(start.AddDate(0, 0, i)))
		}
		return out
	}
}

// A date as a template writes one: the moment, whether it had a time, and the
// brackets it came in ("" for a bare date).
type stamp struct {
	t       time.Time
	hasTime bool
	open    string
	rest    string // a repeater or warning after the time, kept as written
}

var stampRe = regexp.MustCompile(`^([<\[]?)(\d{4}-\d{2}-\d{2})(?:\s+[A-Za-z]{2,3}\.?)?(?:\s+(\d{1,2}:\d{2}))?([^>\]]*?)\s*[>\]]?$`)

func readStamp(s string) (stamp, bool) {
	m := stampRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return stamp{}, false
	}
	layout, val := "2006-01-02", m[2]
	if m[3] != "" {
		layout, val = "2006-01-02 15:04", m[2]+" "+m[3]
	}
	t, err := time.ParseInLocation(layout, val, time.Local)
	if err != nil {
		return stamp{}, false
	}
	return stamp{t: t, hasTime: m[3] != "", open: m[1], rest: strings.TrimSpace(m[4])}, true
}

func (s stamp) String() string {
	body := s.t.Format("2006-01-02")
	if s.open != "" {
		body = s.t.Format("2006-01-02 Mon")
	}
	if s.hasTime {
		body += s.t.Format(" 15:04")
	}
	if s.rest != "" {
		body += " " + s.rest
	}
	switch s.open {
	case "<":
		return "<" + body + ">"
	case "[":
		return "[" + body + "]"
	}
	return body
}

var spanRe = regexp.MustCompile(`^([+-]?)(\d+)\s*(min|h|d|w|m|y)$`)

// moved is t moved on by a span: 3d, +2w, -1m, 1y, 2h, 30min.
func moved(t time.Time, span string) (time.Time, error) {
	m := spanRe.FindStringSubmatch(strings.TrimSpace(strings.ToLower(span)))
	if m == nil {
		return t, fmt.Errorf("%q is not a span - try 3d, 2w, -1m, 1y or 2h", span)
	}
	var n int
	fmt.Sscan(m[2], &n)
	if m[1] == "-" {
		n = -n
	}
	switch m[3] {
	case "min":
		return t.Add(time.Duration(n) * time.Minute), nil
	case "h":
		return t.Add(time.Duration(n) * time.Hour), nil
	case "d":
		return t.AddDate(0, 0, n), nil
	case "w":
		return t.AddDate(0, 0, 7*n), nil
	case "m":
		return t.AddDate(0, n, 0), nil
	}
	return t.AddDate(n, 0, 0), nil
}

func filterErr(name string, err error) *pongo2.Error {
	return &pongo2.Error{Sender: "filter:" + name, OrigError: err}
}

// |plus:"2w" - the date moved on, written the way it came.
func plusFilter(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	s, ok := readStamp(in.String())
	if !ok {
		return nil, filterErr("plus", fmt.Errorf("%q is not a date", in.String()))
	}
	t, err := moved(s.t, param.String())
	if err != nil {
		return nil, filterErr("plus", err)
	}
	s.t = t
	return pongo2.AsSafeValue(s.String()), nil
}

// |format:"weekday" - the date written as one of the clock values.
func formatFilter(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	s, ok := readStamp(in.String())
	if !ok {
		return nil, filterErr("format", fmt.Errorf("%q is not a date", in.String()))
	}
	k := CanonName(param.String())
	switch k {
	case "uuid", "shortuid", "hostname", "username":
		return nil, filterErr("format", fmt.Errorf("%q is not a way of writing a date", param.String()))
	}
	v, ok := AutoValue(k, s.t)
	if !ok {
		return nil, filterErr("format", fmt.Errorf("%q is not a way of writing a date - try date, today, now, weekday, week, month", param.String()))
	}
	return pongo2.AsSafeValue(v), nil
}

// Slug is text as an id or a file name: lower case, letters and digits, a
// dash between words. "Q4 Plans: Draft!" is q4-plans-draft.
func Slug(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			dash = false
			b.WriteRune(r)
		} else {
			dash = true
		}
	}
	return b.String()
}

func slugFilter(in *pongo2.Value, _ *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	return pongo2.AsSafeValue(Slug(in.String())), nil
}

// Tags is a list, or text split at commas, spaces or colons, written as org
// writes tags: ":work:q4_plans:". Spaces inside a comma-separated tag become
// _, as org has no spaces in a tag; "" stays "".
func Tags(items []string) string {
	out := []string{}
	seen := map[string]bool{}
	for _, it := range items {
		t := strings.Trim(strings.Join(strings.Fields(it), "_"), ":")
		t = strings.Map(func(r rune) rune {
			if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '@' || r == '#' || r == '%' {
				return r
			}
			return -1
		}, t)
		if t != "" && !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	if len(out) == 0 {
		return ""
	}
	return ":" + strings.Join(out, ":") + ":"
}

func tagsFilter(in *pongo2.Value, _ *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	var items []string
	if in.CanSlice() && !in.IsString() {
		in.Iterate(func(_, _ int, key, _ *pongo2.Value) bool {
			items = append(items, key.String())
			return true
		}, func() {})
	} else {
		s := in.String()
		sep := func(r rune) bool { return r == ':' || r == ' ' || r == '\t' }
		if strings.Contains(s, ",") {
			sep = func(r rune) bool { return r == ',' || r == ':' }
		}
		items = strings.FieldsFunc(s, sep)
	}
	return pongo2.AsSafeValue(Tags(items)), nil
}

func registerOrgFilters() {
	for name, f := range map[string]pongo2.FilterFunction{
		"plus": plusFilter, "format": formatFilter, "slug": slugFilter, "tags": tagsFilter,
	} {
		if !pongo2.FilterExists(name) {
			pongo2.RegisterFilter(name, f)
		}
	}
}
