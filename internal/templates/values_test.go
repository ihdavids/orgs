package templates

import (
	"strings"
	"testing"
	"time"

	"github.com/flosch/pongo2/v5"
)

// Saturday the 3rd of October 2026: the day and month differ, so a swapped
// layout shows, and the ISO week runs back to Monday the 28th of September.
var valuesNow = time.Date(2026, 10, 3, 9, 7, 0, 0, time.UTC)

func TestAutoNamesAllAnswer(t *testing.T) {
	for _, n := range AutoNames {
		if _, ok := AutoValue(n, valuesNow); !ok {
			t.Errorf("%q is in AutoNames but AutoValue has no case for it", n)
		}
	}
	for alias, n := range Aliases {
		if n == "username" {
			continue // answered by whoever asks, not by AutoValue
		}
		if _, ok := AutoValue(n, valuesNow); !ok {
			t.Errorf("alias %q points at %q, which has no value", alias, n)
		}
	}
}

func TestWeekBounds(t *testing.T) {
	cases := []struct {
		at         time.Time
		start, end string
	}{
		{valuesNow, "<2026-09-28 Mon>", "<2026-10-04 Sun>"},
		// Sunday is the end of its ISO week, not the start of the next.
		{time.Date(2026, 10, 4, 23, 0, 0, 0, time.UTC), "<2026-09-28 Mon>", "<2026-10-04 Sun>"},
		{time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC), "<2026-09-28 Mon>", "<2026-10-04 Sun>"},
		// Across a year end.
		{time.Date(2027, 1, 1, 12, 0, 0, 0, time.UTC), "<2026-12-28 Mon>", "<2027-01-03 Sun>"},
	}
	for _, c := range cases {
		s, _ := AutoValue("week_start", c.at)
		e, _ := AutoValue("week_end", c.at)
		if s != c.start || e != c.end {
			t.Errorf("%s: got %s - %s, want %s - %s", c.at.Format("2006-01-02"), s, e, c.start, c.end)
		}
	}
}

func TestWhen(t *testing.T) {
	w := when(valuesNow)
	cases := []struct {
		spec []string
		want string
	}{
		{[]string{"tomorrow"}, "<2026-10-04 Sun>"},
		{[]string{"+1w"}, "<2026-10-10 Sat>"},
		{[]string{"fri 14:00"}, "<2026-10-09 Fri 14:00>"},
		{[]string{"mon", "date"}, "2026-10-05"},
		{[]string{"+2d", "inactive"}, "[2026-10-05 Mon 00:00]"},
		{[]string{"2026-12-25", "weekday"}, "Friday"},
	}
	for _, c := range cases {
		got := w(c.spec[0], c.spec[1:]...)
		if got != c.want {
			t.Errorf("when%q = %q, want %q", c.spec, got, c.want)
		}
	}
	// Said in the output, not failed: a failed template is an empty file.
	for _, bad := range [][]string{{"someday"}, {"+1w", "uuid"}, {"+1w", "nonsense"}} {
		if got := w(bad[0], bad[1:]...); !strings.HasPrefix(got, "when(") {
			t.Errorf("when%q = %q, want a complaint", bad, got)
		}
	}
}

// The context a file template gets, rendered the way RenderTemplate renders
// it. The clock here is the real one, so this pins shapes, not values.
func TestStandardContext(t *testing.T) {
	tm := &TemplateManager{Author: "A. Writer", Email: "a@example.com", OrgDir: "/notes"}
	render := func(src string) string {
		tpl, err := pongo2.FromString(src)
		if err != nil {
			t.Fatalf("%s: %v", src, err)
		}
		out, err := tpl.Execute(*tm.GetStandardContext())
		if err != nil {
			t.Fatalf("%s: %v", src, err)
		}
		return out
	}
	now := time.Now()
	if got, want := render("{{date}}"), now.Format("2006-01-02"); got != want {
		t.Errorf("date = %q, want %q (year-month-day)", got, want)
	}
	if got := render("{{author}}|{{email}}|{{orgdir}}"); got != "A. Writer|a@example.com|/notes" {
		t.Errorf("identity = %q", got)
	}
	if render("{{username}}") == "" || render("{{user}}") != render("{{username}}") {
		t.Errorf("username %q / user %q", render("{{username}}"), render("{{user}}"))
	}
	if got := render(`{{ when("+0d", "date") }}`); got != now.Format("2006-01-02") {
		t.Errorf(`when("+0d", "date") = %q`, got)
	}
	// Org timestamps come through as written, not html escaped.
	if got := render(`{{today}} {{ when("+1d") }}`); strings.Contains(got, "&lt;") || !strings.HasPrefix(got, "<") {
		t.Errorf("timestamps escaped: %q", got)
	}
	if got := render("{{inactive}}"); got != render("{{now}}") {
		t.Errorf("inactive %q != now %q", got, render("{{now}}"))
	}
	// No configured author: the account's name stands in.
	tm.Author = ""
	if render("{{author}}") == "" {
		t.Errorf("author with nothing configured should fall back to the account")
	}
}
