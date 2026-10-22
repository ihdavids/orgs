package orgs

import (
	"strings"
	"testing"
)

func TestJournalInheritsTheMap(t *testing.T) {
	lines := strings.Split(`#+TITLE: Notes
#+DND_MAP: ../maps/crypt.org

* The door
:PROPERTIES:
:DND_AT: 3 4.5
:END:
Old runes.
** Behind it
:PROPERTIES:
:DND_AT: 5 6
:END:
* Elsewhere
:PROPERTIES:
:DND_MAP: other.org
:END:
** On the other map
:PROPERTIES:
:DND_AT: 1 1
:END:
* Not placed
#+BEGIN_SRC org
* In a block
:PROPERTIES:
:DND_AT: 9 9
:END:
#+END_SRC`, "\n")
	got := journalIn("/org/notes/crypt-notes.org", lines, "/org/maps/crypt.org")
	if len(got) != 2 {
		t.Fatalf("want 2 entries, got %d: %+v", len(got), got)
	}
	if got[0].Title != "The door" || got[0].X != 3 || got[0].Y != 4.5 || got[0].Line != 3 || got[0].End != 12 {
		t.Errorf("first entry wrong: %+v", got[0])
	}
	if !strings.Contains(got[0].Text, "** Behind it") {
		t.Errorf("a note's text should carry its sub headings: %q", got[0].Text)
	}
	if got[1].Title != "Behind it" || got[1].Level != 2 {
		t.Errorf("second entry wrong: %+v", got[1])
	}
}

func TestPlaceNamesTheMapOnlyWhenNeeded(t *testing.T) {
	lines := strings.Split("#+DND_MAP: crypt.org\n* A\n* B\n:PROPERTIES:\n:DND_MAP: other.org\n:END:\n** C", "\n")
	set := placeProps("/org/m/notes.org", lines, 1, "/org/m/crypt.org", 2, 3)
	if set["DND_AT"] != "2 3" {
		t.Errorf("DND_AT = %q", set["DND_AT"])
	}
	if _, has := set["DND_MAP"]; has {
		t.Errorf("A gets the map from the file: %+v", set)
	}
	set = placeProps("/org/m/notes.org", lines, 6, "/org/m/crypt.org", 1.5, 0)
	if set["DND_MAP"] != "crypt.org" {
		t.Errorf("C sits under another map and needs its own DND_MAP: %+v", set)
	}
	if mapLink("/org/notes/x.org", "/org/maps/crypt.org") != "../maps/crypt.org" {
		t.Errorf("mapLink")
	}
}
