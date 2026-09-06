package dnd

import (
	"strings"
	"testing"
)

const emptySession = `#+TITLE: Goblin Ambush
#+DATE: [2025-09-05 Fri]
#+SUMMARY:
#+FILETAGS: :dnd:session:

* Notes

* Rolls
#+NAME: rolls
`

func TestAppendRollsCreatesTableUnderTheName(t *testing.T) {
	out := AppendRolls(emptySession, []SessionRoll{
		{Time: "19:33", Character: "Lyra", Label: "Perception", Formula: "d20 +5",
			Result: "18", Dice: "d20 13", Notes: "adv 22, dis 18"},
	})
	if !strings.Contains(out, "| Time  | Character | Roll") {
		t.Fatalf("no header row written:\n%s", out)
	}
	rolls := ParseRolls(out)
	if len(rolls) != 1 || rolls[0].Label != "Perception" || rolls[0].Result != "18" {
		t.Fatalf("round trip lost the roll: %#v", rolls)
	}
	// A second roll must land in the same table, not start a new one.
	out = AppendRolls(out, []SessionRoll{{Time: "19:40", Character: "Lyra", Label: "Firebolt",
		Formula: "1d10", Result: "7", Dice: "d10 7"}})
	if strings.Count(out, "#+NAME: rolls") != 1 {
		t.Fatalf("table was duplicated:\n%s", out)
	}
	if rolls = ParseRolls(out); len(rolls) != 2 || rolls[1].Label != "Firebolt" {
		t.Fatalf("second roll missing: %#v", rolls)
	}
}

func TestAppendRollsWithoutAnAnchorAddsASection(t *testing.T) {
	out := AppendRolls("#+TITLE: Bare\n", []SessionRoll{{Time: "10:00", Label: "Stealth", Result: "12"}})
	if !strings.Contains(out, "* Rolls") || !strings.Contains(out, "#+NAME: rolls") {
		t.Fatalf("no rolls section created:\n%s", out)
	}
	if rolls := ParseRolls(out); len(rolls) != 1 {
		t.Fatalf("expected one roll, got %#v", rolls)
	}
}

func TestAppendNotesNestsHeadingsUnderTheEntry(t *testing.T) {
	out := AppendNotes(emptySession, []SessionNote{
		{Time: "19:32", Text: "* The Ambush\nThe wagon tracks leave the road here.\n- two dead horses"},
	})
	if !strings.Contains(out, "** 19:32") {
		t.Fatalf("no timestamped entry:\n%s", out)
	}
	if !strings.Contains(out, "*** The Ambush") {
		t.Fatalf("note heading was not pushed down:\n%s", out)
	}
	// The rolls section must survive an append to the notes section.
	if !strings.Contains(out, "* Rolls") {
		t.Fatalf("rolls section lost:\n%s", out)
	}
	notes := ParseNotes(out)
	if len(notes) != 1 || notes[0].Time != "19:32" {
		t.Fatalf("round trip lost the note: %#v", notes)
	}
	if !strings.Contains(notes[0].Text, "* The Ambush") {
		t.Fatalf("heading was not promoted back: %q", notes[0].Text)
	}
	if !strings.Contains(notes[0].Text, "- two dead horses") {
		t.Fatalf("body lost: %q", notes[0].Text)
	}
}

func TestNotesAndRollsCoexist(t *testing.T) {
	out := AppendNotes(emptySession, []SessionNote{{Time: "19:00", Text: "we set out"}})
	out = AppendRolls(out, []SessionRoll{{Time: "19:05", Label: "Survival", Result: "9"}})
	out = AppendNotes(out, []SessionNote{{Time: "19:10", Text: "lost the trail"}})
	if n := len(ParseNotes(out)); n != 2 {
		t.Fatalf("expected 2 notes, got %d:\n%s", n, out)
	}
	if r := len(ParseRolls(out)); r != 1 {
		t.Fatalf("expected 1 roll, got %d:\n%s", r, out)
	}
}

func TestSummaryFallsBackToTheFirstNote(t *testing.T) {
	out := AppendNotes(emptySession, []SessionNote{{Time: "19:00", Text: "We left town. Then it rained."}})
	if got := SessionSummary(out); got != "We left town." {
		t.Fatalf("summary was %q", got)
	}
	out = SetSessionSummary(out, "A wet start")
	if got := SessionSummary(out); got != "A wet start" {
		t.Fatalf("summary was %q", got)
	}
	if strings.Count(out, "#+SUMMARY:") != 1 {
		t.Fatalf("summary keyword duplicated:\n%s", out)
	}
}

func TestSummaryIsInsertedWhenTheFileHasNone(t *testing.T) {
	out := SetSessionSummary("#+TITLE: Bare\n\n* Notes\n", "one line")
	if !strings.Contains(out, "#+SUMMARY: one line") {
		t.Fatalf("summary not inserted:\n%s", out)
	}
	if strings.Index(out, "#+SUMMARY:") > strings.Index(out, "* Notes") {
		t.Fatalf("summary landed after the body:\n%s", out)
	}
}

func TestSearchReportsTheHeadingItFoundThingsUnder(t *testing.T) {
	text := AppendNotes(emptySession, []SessionNote{{Time: "19:00", Text: "Sildar is missing"}})
	info := SessionInfoFromText("2025_09_05_Goblin_Ambush", "x.org", text)
	hits := SearchSession(info, text, "sildar", 10)
	if len(hits) != 1 {
		t.Fatalf("expected one hit, got %#v", hits)
	}
	if hits[0].Context != "19:00" || hits[0].Kind != "note" {
		t.Fatalf("unexpected hit %#v", hits[0])
	}
}

func TestSessionFileNaming(t *testing.T) {
	if got := SessionSlug("Goblin  Ambush!"); got != "Goblin_Ambush" {
		t.Fatalf("slug was %q", got)
	}
	if !ValidSessionId("2025_09_05_Goblin_Ambush") {
		t.Fatal("valid id rejected")
	}
	for _, bad := range []string{"", "../../etc/passwd", "a/b", ".hidden"} {
		if ValidSessionId(bad) {
			t.Fatalf("%q should not be a valid id", bad)
		}
	}
}

func TestAddSessionCharacterStampsTheId(t *testing.T) {
	ch := SessionCharacter{Id: "lyra-silverleaf-4c1f2a", Name: "Lyra Silverleaf"}
	out := AddSessionCharacter(emptySession, ch)
	if !strings.Contains(out, "* Characters") {
		t.Fatalf("no characters section:\n%s", out)
	}
	if !strings.Contains(out, ":DND_ID: lyra-silverleaf-4c1f2a") {
		t.Fatalf("id not stamped:\n%s", out)
	}
	if !strings.Contains(out, "#+CHARACTERS: Lyra Silverleaf") {
		t.Fatalf("character keyword missing:\n%s", out)
	}
	// The same character twice must not double up.
	again := AddSessionCharacter(out, ch)
	if strings.Count(again, ":DND_ID:") != 1 {
		t.Fatalf("character was added twice:\n%s", again)
	}
	// A second character joins the list.
	two := AddSessionCharacter(again, SessionCharacter{Id: "grish-99aa11", Name: "Grish"})
	people := ParseSessionCharacters(two)
	if len(people) != 2 || people[0].Name != "Lyra Silverleaf" || people[1].Id != "grish-99aa11" {
		t.Fatalf("characters round tripped as %#v\n%s", people, two)
	}
	if !strings.Contains(two, "#+CHARACTERS: Lyra Silverleaf, Grish") {
		t.Fatalf("character keyword not updated:\n%s", two)
	}
	// Notes and rolls still go where they belong.
	two = AppendNotes(two, []SessionNote{{Time: "19:00", Text: "we begin"}})
	two = AppendRolls(two, []SessionRoll{{Time: "19:01", Label: "Stealth", Result: "14"}})
	if len(ParseNotes(two)) != 1 || len(ParseRolls(two)) != 1 || len(ParseSessionCharacters(two)) != 2 {
		t.Fatalf("sections interfered with each other:\n%s", two)
	}
}

func TestCharacterIdIsStableAndDerivable(t *testing.T) {
	a := DeriveCharacterId("Gooey McFart")
	if a != DeriveCharacterId("gooey mcfart") {
		t.Fatalf("id is not stable across case: %s", a)
	}
	if !strings.HasPrefix(a, "gooey-mcfart-") {
		t.Fatalf("id is not readable: %s", a)
	}
	if a == DeriveCharacterId("Lyra Silverleaf") {
		t.Fatal("two characters share an id")
	}
	c := &Character{Name: "Gooey McFart"}
	if CharacterId(c) != a {
		t.Fatalf("character without an id did not derive one")
	}
	c.Id = "kept-by-hand"
	if CharacterId(c) != "kept-by-hand" {
		t.Fatal("an explicit id was overwritten")
	}
}

// A sheet lists its own games, not the whole table's, so the id embedded in a
// session file has to be what decides.
func TestSessionPlayedBy(t *testing.T) {
	s := SessionInfo{Characters: []SessionCharacter{
		{Id: "lyra-silverleaf-4f2a", Name: "Lyra Silverleaf"},
		{Id: "gooey-mcfart-91bd", Name: "Gooey McFart"},
	}}
	if !s.PlayedBy("gooey-mcfart-91bd", "Gooey McFart") {
		t.Error("a character who played was filtered out")
	}
	if s.PlayedBy("brand-new-7c11", "Someone Else") {
		t.Error("a character who did not play was let through")
	}
	// A name that happens to match must not beat a mismatched id.
	if s.PlayedBy("someone-else-0000", "Lyra Silverleaf") {
		t.Error("a mismatched id fell back to the name")
	}
}

// Session files written before ids were recorded carry a name and nothing
// else. Those must stay visible on the sheet that played them.
func TestSessionPlayedByFallsBackToName(t *testing.T) {
	s := SessionInfo{Characters: []SessionCharacter{{Name: "Lyra Silverleaf"}}}
	if !s.PlayedBy("lyra-silverleaf-4f2a", "lyra silverleaf") {
		t.Error("an entry with no id was not matched by name")
	}
	if s.PlayedBy("gooey-mcfart-91bd", "Gooey McFart") {
		t.Error("an entry with no id matched the wrong character")
	}
}
