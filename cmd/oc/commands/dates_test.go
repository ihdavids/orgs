package commands

import (
	"testing"
	"time"
)

// A date read wrong goes into a file and is not noticed for a year, so the
// readings are pinned rather than trusted. Sunday the 27th of September 2026 is
// "now" throughout, which makes the weekday cases readable.
var now = time.Date(2026, 9, 27, 11, 30, 0, 0, time.UTC)

func TestParseDate(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"today", "<2026-09-27 Sun>"},
		{"tomorrow", "<2026-09-28 Mon>"},
		{"yesterday", "<2026-09-26 Sat>"},
		{"+3d", "<2026-09-30 Wed>"},
		{"-1w", "<2026-09-20 Sun>"},
		{"+2w", "<2026-10-11 Sun>"},
		{"+1m", "<2026-10-27 Tue>"},
		{"+1y", "<2027-09-27 Mon>"},
		{"2026-10-01", "<2026-10-01 Thu>"},
		// The coming weekday, today counting as one - which is org's reading.
		{"sun", "<2026-09-27 Sun>"},
		{"mon", "<2026-09-28 Mon>"},
		{"fri", "<2026-10-02 Fri>"},
		{"eow", "<2026-10-02 Fri>"},
		{"eom", "<2026-09-30 Wed>"},
		{"eoy", "<2026-12-31 Thu>"},
		// A time, either way round, and a range.
		{"tomorrow 14:00", "<2026-09-28 Mon 14:00>"},
		{"14:00 tomorrow", "<2026-09-28 Mon 14:00>"},
		{"fri 2pm", "<2026-10-02 Fri 14:00>"},
		{"mon 09:30-10:45", "<2026-09-28 Mon 09:30-10:45>"},
		// A time with no day is today.
		{"16:00", "<2026-09-27 Sun 16:00>"},
		// A repeater needs a day in front of it, and `+1w` after one is the
		// repeater rather than a second offset.
		{"mon +1w", "<2026-09-28 Mon +1w>"},
		{"tomorrow .+2d", "<2026-09-28 Mon .+2d>"},
		{"fri 14:00 ++1m", "<2026-10-02 Fri 14:00 ++1m>"},
		// A day and month with no year is the coming one.
		{"12-25", "<2026-12-25 Fri>"},
		{"01-05", "<2027-01-05 Tue>"},
		// A bare day of the month.
		{"30", "<2026-09-30 Wed>"},
		{"15", "<2026-10-15 Thu>"},
		// Already written: passed through untouched, brackets and all.
		{"<2026-10-01 Thu 09:00>", "<2026-10-01 Thu 09:00>"},
		{"[2026-10-01 Thu]", "[2026-10-01 Thu]"},
		// Clearing.
		{"", ""},
		{"clear", ""},
		{"none", ""},
	} {
		got, err := ParseDateToOrg(c.in, now)
		if err != nil {
			t.Errorf("ParseDateToOrg(%q): %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseDateToOrg(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// An ambiguous date is refused rather than guessed at: it goes into a file and
// is read a year later, by which time nobody remembers which way round it was
// meant.
func TestAmbiguousDateIsRefused(t *testing.T) {
	for _, in := range []string{"1/10", "3/4", "not-a-date", "+2q"} {
		if _, err := ParseDateToOrg(in, now); err == nil {
			t.Errorf("ParseDateToOrg(%q) should have refused", in)
		}
	}
}

// The hash test is here because everything downstream of it - the stdin form,
// the picker's address fields, the "is this a query or a heading" question -
// turns on getting it right, and a false positive sends a write somewhere
// nobody asked for.
func TestLooksLikeHash(t *testing.T) {
	yes := []string{"hOpOB7vIg6oiYz5sMVSlzGiJXic=", "2jmj7l5rSw0yVb/vlWAYkK/YBwk="}
	no := []string{
		"IsTask()",                      // a query
		`IsStatus("NEXT")`,              // a query with a string in it
		"{{ WorkTasks }}",               // a filter
		"",                              // nothing
		"hOpOB7vIg6oiYz5sMVSlzGiJXic",   // a hash with the padding lost
		"hOpOB7vIg6oiYz5sMVSlzGiJXic==", // one character too long
		"notbase64!!notbase64!!notb=",   // the right shape, not base64
	}
	for _, s := range yes {
		if !LooksLikeHash(s) {
			t.Errorf("LooksLikeHash(%q) = false, want true", s)
		}
	}
	for _, s := range no {
		if LooksLikeHash(s) {
			t.Errorf("LooksLikeHash(%q) = true, want false", s)
		}
	}
}
