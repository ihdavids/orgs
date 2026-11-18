package orgs

import (
	"strings"
	"testing"
)

// Two edits to one heading where the first adds lines: the second has to land
// at the end of the same heading, not two lines into the next one.
func TestHeadingLinesMeasuresAfterEverySet(t *testing.T) {
	lines := strings.Split(`* One
body
** Child
* Two
two's body`, "\n")
	h := newHeadingLines("x.org", lines, 0, 1)
	if h.End != 2 {
		t.Fatalf("End = %d, want 2", h.End)
	}
	h.Set(splice(h.Lines, 1, []string{":PROPERTIES:", ":ID: abc", ":END:"}))
	if h.End != 5 {
		t.Fatalf("End after adding three lines = %d, want 5", h.End)
	}
	h.Set(splice(h.Lines, h.End+1, []string{"appended"}))
	want := `* One
:PROPERTIES:
:ID: abc
:END:
body
** Child
appended
* Two
two's body`
	if got := strings.Join(h.Lines, "\n"); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}

	// Lines taken out: the end comes back up with them.
	h.Set(append(append([]string{}, h.Lines[:1]...), h.Lines[4:]...))
	if h.End != 3 || h.Lines[h.End] != "appended" {
		t.Errorf("End after removing the drawer = %d (%q)", h.End, h.Lines[h.End])
	}

	// The last heading in a file runs to the last line.
	last := newHeadingLines("x.org", lines, 3, 1)
	if last.End != 4 {
		t.Errorf("last heading End = %d, want 4", last.End)
	}
	if newHeadingLines("x.org", lines, 9, 1) != nil {
		t.Error("a row past the end measured")
	}
}
