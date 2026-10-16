package orgs

import (
	"strings"
	"testing"
)

// A new property goes in at the indent of the drawer it joins. A drawer in
// column zero (what Emacs writes since org 9.5) under a level-2 heading used
// to get its new line indented by heading depth, three spaces, as a stray.
func TestSetPropInMatchesTheDrawer(t *testing.T) {
	lines := []string{
		"** TODO Docs",
		":PROPERTIES:",
		":EFFORT: 2d",
		":END:",
	}
	out, _ := setPropIn(lines, 1, 3, "   ", "GANTT_ORDER", "20")
	got := strings.Join(out, "\n")
	for _, l := range out {
		if strings.Contains(l, "GANTT_ORDER") && strings.HasPrefix(l, " ") {
			t.Errorf("new property indented inside a column-zero drawer:\n%s", got)
		}
	}
	// An indented drawer keeps its own indent for the new line.
	lines = []string{
		"** TODO Docs",
		"   :PROPERTIES:",
		"   :EFFORT: 2d",
		"   :END:",
	}
	out, _ = setPropIn(lines, 1, 3, "", "GANTT_ORDER", "20")
	found := false
	for _, l := range out {
		if strings.Contains(l, "GANTT_ORDER") {
			found = true
			if !strings.HasPrefix(l, "   :GANTT_ORDER:") {
				t.Errorf("new property lost the drawer's indent: %q", l)
			}
		}
	}
	if !found {
		t.Errorf("property not written:\n%s", strings.Join(out, "\n"))
	}
}

// Properties two in from their drawer (what the gantt files are written with)
// get a new one at the same depth, and an empty drawer gets two spaces in.
func TestSetPropInMatchesTheSiblings(t *testing.T) {
	lines := []string{
		"*** TODO Docs",
		"    :PROPERTIES:",
		"      :EFFORT: 2d",
		"    :END:",
	}
	out, _ := setPropIn(lines, 1, 3, "", "ASSIGNEE", "ann")
	if !strings.HasPrefix(out[3], "      :ASSIGNEE:") {
		t.Errorf("new property not lined up with its siblings:\n%s", strings.Join(out, "\n"))
	}
	lines = []string{
		"*** TODO Docs",
		"    :PROPERTIES:",
		"    :END:",
	}
	out, _ = setPropIn(lines, 1, 2, "", "EFFORT", "1d")
	if !strings.HasPrefix(out[2], "      :EFFORT:") {
		t.Errorf("property in an empty drawer not two in from it:\n%s", strings.Join(out, "\n"))
	}
}
