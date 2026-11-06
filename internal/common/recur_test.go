package common

import (
	"strings"
	"testing"
	"time"
)

// The same cases are in worg's src/recur.test.ts and vscode-org-everywhere's
// test/recur-model.test.ts: change all three together.
//
// now is Wednesday 2026-11-04 12:00.
var recurCases = []struct {
	rule  string
	next  string // "" for a rule that does not read
	words string
}{
	{"+2", "2026-11-06 Fri", "every 2 days"},
	{"+1", "2026-11-05 Thu", "every day"},
	{"+w", "2026-11-11 Wed", "every week"},
	{"+2w", "2026-11-18 Wed", "every 2 weeks"},
	{"+1m", "2026-12-04 Fri", "every month"},
	{"+1y", "2027-11-04 Thu", "every year"},
	{"++3", "2026-11-07 Sat", "every 3 days"},
	{"+0", "2026-11-04 Wed", "the same day"},
	{".", "2026-11-04 Wed", "the same day"},
	{"Mon,Thu", "2026-11-05 Thu", "every Monday and Thursday"},
	{"Wed", "2026-11-11 Wed", "every Wednesday"},
	{"sunday", "2026-11-08 Sun", "every Sunday"},
	{"1,15", "2026-11-15 Sun", "the 1st and 15th of each month"},
	{"4", "2026-12-04 Fri", "the 4th of each month"},
	{"31", "2026-12-31 Thu", "the 31st of each month"},
	{"wkdy", "2026-11-05 Thu", "every weekday"},
	{"Wkdy 09:00", "2026-11-05 Thu 09:00", "every weekday at 09:00"},
	{"+tue", "2026-11-10 Tue", "the next Tuesday"},
	{"+2tue", "2026-11-17 Tue", "the 2nd Tuesday from then"},
	{"+wed", "2026-11-11 Wed", "the next Wednesday"},
	{"-fri", "2026-10-30 Fri", "the Friday before"},
	{"Mon 09:00", "2026-11-09 Mon 09:00", "every Monday at 09:00"},
	{"Thu 10:00-11:30", "2026-11-05 Thu 10:00-11:30", "every Thursday at 10:00-11:30"},
	{"3pm", "2026-11-04 Wed 15:00", "every day at 15:00"},
	{"11:00", "2026-11-05 Thu 11:00", "every day at 11:00"},
	{"12-25", "2026-12-25 Fri", "every 25 December"},
	{"dec 25", "2026-12-25 Fri", "every 25 December"},
	{"25 Dec", "2026-12-25 Fri", "every 25 December"},
	{"2/29", "2028-02-29 Tue", "every 29 February"},
	{"2027-01-01", "2027-01-01 Fri", "on 2027-01-01"},
	{"+2,Mon", "2026-11-06 Fri", "every 2 days, or every Monday"},
	{"Sat, Sun", "2026-11-07 Sat", "every Saturday and Sunday"},
	{"|Mon|", "2026-11-09 Mon", "every Monday"},
	{"", "", ""},
	{"bogus", "", ""},
	{"32", "", ""},
	{"+", "", ""},
	{"13-01", "", ""},
	{"25:00", "", ""},
	{"+2x", "", ""},
}

var recurNow = time.Date(2026, 11, 4, 12, 0, 0, 0, time.Local)

func TestRecurCases(t *testing.T) {
	for _, c := range recurCases {
		r, err := ParseRecur(c.rule, "")
		if c.next == "" {
			if err == nil {
				t.Errorf("%q: read as %+v, want an error", c.rule, r)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q: %v", c.rule, err)
			continue
		}
		if got := r.Next(recurNow).String(); got != c.next {
			t.Errorf("%q: next %s, want %s", c.rule, got, c.next)
		}
		if got := r.Words(); got != c.words {
			t.Errorf("%q: words %q, want %q", c.rule, got, c.words)
		}
	}
}

func TestRecurNextN(t *testing.T) {
	cases := []struct {
		rule string
		n    int
		want string
	}{
		{"+2", 3, "2026-11-06 Fri, 2026-11-08 Sun, 2026-11-10 Tue"},
		{"1,15", 4, "2026-11-15 Sun, 2026-12-01 Tue, 2026-12-15 Tue, 2027-01-01 Fri"},
		{"31", 3, "2026-12-31 Thu, 2027-01-31 Sun, 2027-03-31 Wed"},
		{"wkdy", 3, "2026-11-05 Thu, 2026-11-06 Fri, 2026-11-09 Mon"},
		{"2027-01-01", 3, "2027-01-01 Fri"},
		{"+0", 3, "2026-11-04 Wed"},
	}
	for _, c := range cases {
		r, err := ParseRecur(c.rule, "")
		if err != nil {
			t.Fatalf("%q: %v", c.rule, err)
		}
		got := []string{}
		for _, d := range r.NextN(recurNow, c.n) {
			got = append(got, d.String())
		}
		if strings.Join(got, ", ") != c.want {
			t.Errorf("%q: %s, want %s", c.rule, strings.Join(got, ", "), c.want)
		}
	}
}

func TestRecurWeekdaysSetting(t *testing.T) {
	r, err := ParseRecur("wkdy", "sun,mon,tue,wed,thu")
	if err != nil {
		t.Fatal(err)
	}
	if got := r.Next(time.Date(2026, 11, 5, 9, 0, 0, 0, time.Local)).String(); got != "2026-11-08 Sun" {
		t.Errorf("wkdy with a Sunday-Thursday week from Thursday: %s", got)
	}
}

func TestRecurTitles(t *testing.T) {
	strips := map[string]string{
		"Water plants |+2|":   "Water plants",
		"a |Mon| b":           "a b",
		"|1,15| Pay rent":     "Pay rent",
		"a || b":              "a || b",
		"no rule":             "no rule",
	}
	for in, want := range strips {
		if got := StripRecur(in); got != want {
			t.Errorf("StripRecur(%q) = %q, want %q", in, got, want)
		}
	}
	sets := []struct{ title, rule, want string }{
		{"Water", "Mon,Fri", "Water |Mon,Fri|"},
		{"Water", "|Mon,Fri|", "Water |Mon,Fri|"},
		{"x |+2| y", "|+w|", "x |+w| y"},
		{"x |+2| y", "", "x y"},
		{"x", "", "x"},
	}
	for _, c := range sets {
		if got := SetRecurIn(c.title, c.rule); got != c.want {
			t.Errorf("SetRecurIn(%q, %q) = %q, want %q", c.title, c.rule, got, c.want)
		}
	}
	if _, ok := RecurOf("a | b | c", ""); ok {
		t.Errorf("a | b | c is a table-ish title, not a rule")
	}
	if r, ok := RecurOf("TODO Water plants |+2|", ""); !ok || r.Text != "+2" {
		t.Errorf("RecurOf: %+v %v", r, ok)
	}
}
