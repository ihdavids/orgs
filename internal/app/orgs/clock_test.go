package orgs

import (
	"strings"
	"testing"
	"time"
)

// A finished clock goes where org puts it: after the planning line and the
// property drawer, at the top of an existing drawer, once - and at the
// heading's own indentation.
func TestClockLinePlacement(t *testing.T) {
	start := time.Date(2026, 10, 10, 9, 0, 0, 0, time.Local)
	end := start.Add(90 * time.Minute)
	if got := clockLine("  ", start, end); got != "  CLOCK: [2026-10-10 Sat 09:00]--[2026-10-10 Sat 10:30] =>  1:30" {
		t.Fatalf("clock line: %q", got)
	}

	lines := strings.Split(`* TODO Due
  DEADLINE: <2026-10-20 Tue>
  :PROPERTIES:
  :EFFORT: 2d
  :END:
  Body.`, "\n")
	out := logInto(lines, 0, len(lines)-1, ownBodyIndent(lines, 0, len(lines)-1, 1), "LOGBOOK", []string{clockLine("  ", start, end)})
	want := []string{"* TODO Due", "  DEADLINE: <2026-10-20 Tue>", "  :PROPERTIES:", "  :EFFORT: 2d", "  :END:", "  :LOGBOOK:"}
	for i, w := range want {
		if out[i] != w {
			t.Fatalf("line %d: want %q, got %q\n%s", i, w, out[i], strings.Join(out, "\n"))
		}
	}

	again := logInto(out, 0, len(out)-1, "  ", "LOGBOOK", []string{"  CLOCK: second"})
	if strings.Count(strings.Join(again, "\n"), ":LOGBOOK:") != 1 || again[6] != "  CLOCK: second" {
		t.Fatalf("second clock should go at the top of the one drawer:\n%s", strings.Join(again, "\n"))
	}

	zero := strings.Split("* TODO Col\nDEADLINE: <2026-10-21 Wed>\nBody.\n** Child", "\n")
	if ind := ownBodyIndent(zero, 0, len(zero)-1, 1); ind != "" {
		t.Fatalf("column-zero body should give no indent, got %q", ind)
	}
	if ind := ownBodyIndent([]string{"** Bare", "*** Child"}, 0, 1, 2); ind != "   " {
		t.Fatalf("a heading with no body of its own should get two past the stars, got %q", ind)
	}
}
