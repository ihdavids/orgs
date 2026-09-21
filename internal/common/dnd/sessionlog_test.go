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

func TestUpdateNoteRewritesOneEntryInPlace(t *testing.T) {
	out := AppendNotes(emptySession, []SessionNote{
		{Time: "19:32", Text: "The wagon tracks leave the road here."},
		{Time: "19:45", Text: "Sildar is missing."},
		{Time: "20:01", Text: "Camped by the stream."},
	})
	out, err := UpdateNote(out, 1, SessionNote{
		Text: "* Sildar\nSildar is missing, and so is the wagon.\n- goblin arrows"}, "19:45")
	if err != nil {
		t.Fatalf("editing the middle note: %s", err)
	}
	notes := ParseNotes(out)
	if len(notes) != 3 {
		t.Fatalf("editing a note changed how many there are: %#v", notes)
	}
	if notes[1].Time != "19:45" {
		t.Fatalf("an edited note keeps the time it was taken, got %q", notes[1].Time)
	}
	if !strings.Contains(notes[1].Text, "and so is the wagon") {
		t.Fatalf("the new text did not land: %q", notes[1].Text)
	}
	if !strings.Contains(notes[1].Text, "* Sildar") {
		t.Fatalf("a heading inside the note should survive the round trip: %q", notes[1].Text)
	}
	if notes[0].Text != "The wagon tracks leave the road here." ||
		notes[2].Text != "Camped by the stream." {
		t.Fatalf("the notes either side moved: %#v", notes)
	}
	// The rolls section is somebody else's, and stays exactly where it was.
	if !strings.Contains(out, "* Rolls") {
		t.Fatalf("the rest of the file was disturbed:\n%s", out)
	}
}

func TestUpdateNoteCanRestampANote(t *testing.T) {
	out := AppendNotes(emptySession, []SessionNote{{Time: "19:32", Text: "First light."}})
	out, err := UpdateNote(out, 0, SessionNote{Time: "19:35", Text: "First light."}, "19:32")
	if err != nil {
		t.Fatal(err)
	}
	if notes := ParseNotes(out); len(notes) != 1 || notes[0].Time != "19:35" {
		t.Fatalf("want the note restamped, got %#v", notes)
	}
}

func TestUpdateNoteRefusesWhatItCannotBeSureOf(t *testing.T) {
	out := AppendNotes(emptySession, []SessionNote{{Time: "19:32", Text: "First light."}})
	if _, err := UpdateNote(out, 3, SessionNote{Text: "Nope."}, ""); err == nil {
		t.Fatal("there is no fourth note to edit")
	}
	// The page thought this note was taken at some other time, so the file
	// has moved under it.
	if _, err := UpdateNote(out, 0, SessionNote{Text: "Nope."}, "21:00"); err == nil {
		t.Fatal("a stale edit should be refused")
	}
	if _, err := UpdateNote(out, 0, SessionNote{Text: "   "}, "19:32"); err == nil {
		t.Fatal("a note cannot be emptied")
	}
	// A file with no notes at all has nothing to edit.
	if _, err := UpdateNote("#+TITLE: Bare\n", 0, SessionNote{Text: "Hi"}, ""); err == nil {
		t.Fatal("there is no notes section here")
	}
}

func TestNoteKeepsTheLinesItWasTypedWith(t *testing.T) {
	typed := "They came out of the trees.\nSildar went down first.\nWe ran."
	out := AppendNotes(emptySession, []SessionNote{{Time: "19:32", Text: typed}})
	// Org runs consecutive prose together, so every line but the last needs
	// a hard break or the note comes back as one run-on sentence.
	if strings.Count(out, `\\`) != 2 {
		t.Fatalf("want a break after the first two lines:\n%s", out)
	}
	if strings.Contains(out, `We ran. \\`) {
		t.Fatalf("the last line of a paragraph needs no break:\n%s", out)
	}
	// And what the editor gets back is what was typed, backslashes and all
	// taken off again.
	if got := ParseNotes(out)[0].Text; got != typed {
		t.Fatalf("the round trip changed the note:\nwant %q\ngot  %q", typed, got)
	}
}

func TestNoteBreaksOnlyWhereOrgWouldRunLinesTogether(t *testing.T) {
	typed := "* Loot\n- 12 gp\n- a map\nfolded twice\n\n| a | b |\n| 1 | 2 |"
	out := AppendNotes(emptySession, []SessionNote{{Time: "19:32", Text: typed}})
	// A bullet after a bullet starts something new, so it needs no break;
	// a line continuing a bullet does. A heading and a table row never do.
	if strings.Contains(out, `12 gp \\`) {
		t.Fatalf("a bullet before another bullet needs no break:\n%s", out)
	}
	if !strings.Contains(out, `a map \\`) {
		t.Fatalf("a bullet continued on the next line does:\n%s", out)
	}
	if strings.Contains(out, `| a | b | \\`) || strings.Contains(out, `Loot \\`) {
		t.Fatalf("a table row and a heading never carry one:\n%s", out)
	}
	if got := ParseNotes(out)[0].Text; got != typed {
		t.Fatalf("the round trip changed the note:\nwant %q\ngot  %q", typed, got)
	}
}

func TestNoteLeavesVerbatimBlocksAlone(t *testing.T) {
	typed := "#+BEGIN_SRC go\na := 1\nb := 2\n#+END_SRC"
	out := AppendNotes(emptySession, []SessionNote{{Time: "19:32", Text: typed}})
	if strings.Contains(out, `\\`) {
		t.Fatalf("nothing inside a block should be touched:\n%s", out)
	}
	if got := ParseNotes(out)[0].Text; got != typed {
		t.Fatalf("the round trip changed the block:\nwant %q\ngot  %q", typed, got)
	}
}

func TestDeleteRollTakesOneRowOut(t *testing.T) {
	out := AppendRolls(emptySession, []SessionRoll{
		{Time: "19:32", Character: "Lyra", Label: "Stealth", Formula: "d20 +5", Result: "18"},
		{Time: "19:40", Character: "Lyra", Label: "Longsword", Formula: "d20 +6", Result: "12"},
		{Time: "19:41", Character: "Lyra", Label: "Perception", Formula: "d20 +3", Result: "9"},
	})
	out, err := DeleteRoll(out, 1, "Longsword")
	if err != nil {
		t.Fatalf("deleting the middle roll: %s", err)
	}
	rolls := ParseRolls(out)
	if len(rolls) != 2 {
		t.Fatalf("want two rolls left, got %#v", rolls)
	}
	if rolls[0].Label != "Stealth" || rolls[1].Label != "Perception" {
		t.Fatalf("the wrong roll went: %#v", rolls)
	}
	if strings.Contains(out, "Longsword") {
		t.Fatalf("the roll is still in the file:\n%s", out)
	}
}

func TestDeletingTheLastRollTakesTheTableWithIt(t *testing.T) {
	out := AppendRolls(emptySession, []SessionRoll{
		{Time: "19:32", Character: "Lyra", Label: "Stealth", Formula: "d20 +5", Result: "18"},
	})
	out, err := DeleteRoll(out, 0, "Stealth")
	if err != nil {
		t.Fatalf("deleting the only roll: %s", err)
	}
	if rolls := ParseRolls(out); len(rolls) != 0 {
		t.Fatalf("want no rolls left, got %#v", rolls)
	}
	// Back to the state a session with nothing rolled in it starts in: the
	// anchor, and no table under it.
	if strings.Contains(out, "| Time") {
		t.Fatalf("an emptied table should go altogether:\n%s", out)
	}
	if !strings.Contains(out, "#+NAME: "+SessionTableName) {
		t.Fatalf("the next roll has nowhere to go:\n%s", out)
	}
	// And the next roll writes it back exactly as it was the first time.
	again := AppendRolls(out, []SessionRoll{
		{Time: "20:00", Character: "Lyra", Label: "Athletics", Formula: "d20 +4", Result: "15"},
	})
	if rolls := ParseRolls(again); len(rolls) != 1 || rolls[0].Label != "Athletics" {
		t.Fatalf("the table did not come back: %#v", rolls)
	}
}

func TestDeleteRollRefusesWhatItCannotBeSureOf(t *testing.T) {
	out := AppendRolls(emptySession, []SessionRoll{
		{Time: "19:32", Character: "Lyra", Label: "Stealth", Formula: "d20 +5", Result: "18"},
		{Time: "19:40", Character: "Lyra", Label: "Longsword", Formula: "d20 +6", Result: "12"},
	})
	if _, err := DeleteRoll(out, 7, ""); err == nil {
		t.Fatal("there is no eighth roll to delete")
	}
	// Deleting shifts every roll after it up by one, so a stale number
	// would otherwise throw away the wrong row.
	if _, err := DeleteRoll(out, 0, "Longsword"); err == nil {
		t.Fatal("a stale delete should be refused")
	}
	if _, err := DeleteRoll("#+TITLE: Bare\n", 0, ""); err == nil {
		t.Fatal("there are no rolls here to delete")
	}
	if rolls := ParseRolls(out); len(rolls) != 2 {
		t.Fatalf("a refused delete moved something: %#v", rolls)
	}
}

func TestDeleteNoteTakesOneOutAndLeavesTheRest(t *testing.T) {
	out := AppendNotes(emptySession, []SessionNote{
		{Time: "19:32", Text: "The wagon tracks leave the road here."},
		{Time: "19:45", Text: "Sildar is missing."},
		{Time: "20:01", Text: "Camped by the stream."},
	})
	out, err := DeleteNote(out, 1, "19:45")
	if err != nil {
		t.Fatalf("deleting the middle note: %s", err)
	}
	notes := ParseNotes(out)
	if len(notes) != 2 {
		t.Fatalf("want two notes left, got %#v", notes)
	}
	if notes[0].Text != "The wagon tracks leave the road here." ||
		notes[1].Text != "Camped by the stream." {
		t.Fatalf("the wrong note went: %#v", notes)
	}
	if strings.Contains(out, "Sildar") {
		t.Fatalf("the note is still in the file:\n%s", out)
	}
	// The rolls section is somebody else's, and stays exactly where it was.
	if !strings.Contains(out, "* Rolls") {
		t.Fatalf("the rest of the file was disturbed:\n%s", out)
	}
}

func TestDeleteNoteTakesTheLastOneAndTheOnlyOne(t *testing.T) {
	out := AppendNotes(emptySession, []SessionNote{
		{Time: "19:32", Text: "First light."},
		{Time: "19:45", Text: "Last light."},
	})
	out, err := DeleteNote(out, 1, "19:45")
	if err != nil {
		t.Fatalf("deleting the last note: %s", err)
	}
	if notes := ParseNotes(out); len(notes) != 1 || notes[0].Text != "First light." {
		t.Fatalf("want only the first left, got %#v", notes)
	}
	out, err = DeleteNote(out, 0, "19:32")
	if err != nil {
		t.Fatalf("deleting the only note: %s", err)
	}
	if notes := ParseNotes(out); len(notes) != 0 {
		t.Fatalf("want no notes left, got %#v", notes)
	}
	// An empty notes section is still a notes section: the next note has
	// somewhere to go.
	if !strings.Contains(out, "* Notes") {
		t.Fatalf("the notes heading should stay:\n%s", out)
	}
}

func TestDeleteNoteRefusesWhatItCannotBeSureOf(t *testing.T) {
	out := AppendNotes(emptySession, []SessionNote{
		{Time: "19:32", Text: "First light."},
		{Time: "19:45", Text: "Last light."},
	})
	if _, err := DeleteNote(out, 5, ""); err == nil {
		t.Fatal("there is no sixth note to delete")
	}
	// Deleting shifts every note after it up by one, so a page holding a
	// stale number would otherwise throw away the wrong note.
	if _, err := DeleteNote(out, 0, "19:45"); err == nil {
		t.Fatal("a stale delete should be refused")
	}
	if _, err := DeleteNote("#+TITLE: Bare\n", 0, ""); err == nil {
		t.Fatal("there are no notes here to delete")
	}
	// A refusal leaves the file exactly as it was.
	if notes := ParseNotes(out); len(notes) != 2 {
		t.Fatalf("a refused delete moved something: %#v", notes)
	}
}

func TestSessionCharacterLinksBackToTheSheet(t *testing.T) {
	out := AddSessionCharacter(emptySession, SessionCharacter{
		Id: "lyra-silverleaf-4c1f2a", Name: "Lyra", File: "/gtd/dnd/lyra.org"})
	if !strings.Contains(out, "[[file:/gtd/dnd/lyra.org][Lyra]]") {
		t.Fatalf("no link back to the character sheet:\n%s", out)
	}
	cast := ParseSessionCharacters(out)
	if len(cast) != 1 || cast[0].File != "/gtd/dnd/lyra.org" {
		t.Fatalf("the link did not survive the round trip: %#v", cast)
	}
	if cast[0].Id != "lyra-silverleaf-4c1f2a" || cast[0].Name != "Lyra" {
		t.Fatalf("the link cost us the rest of the entry: %#v", cast)
	}
	// Listing the same character again changes nothing.
	again := AddSessionCharacter(out, SessionCharacter{
		Id: "lyra-silverleaf-4c1f2a", Name: "Lyra", File: "/gtd/dnd/lyra.org"})
	if again != out {
		t.Fatalf("a character was listed twice:\n%s", again)
	}
}

func TestSessionCharacterGetsALinkItWasWrittenWithout(t *testing.T) {
	// A session from before the sheet knew where the character lived.
	out := AddSessionCharacter(emptySession, SessionCharacter{
		Id: "lyra-silverleaf-4c1f2a", Name: "Lyra"})
	if strings.Contains(out, "[[file:") {
		t.Fatalf("there was no file to link to:\n%s", out)
	}
	out = AddSessionCharacter(out, SessionCharacter{
		Id: "lyra-silverleaf-4c1f2a", Name: "Lyra", File: "/gtd/dnd/lyra.org"})
	cast := ParseSessionCharacters(out)
	if len(cast) != 1 {
		t.Fatalf("topping up the link listed the character twice: %#v", cast)
	}
	if cast[0].File != "/gtd/dnd/lyra.org" {
		t.Fatalf("the link was not added: %#v\n%s", cast, out)
	}
	// The second character in a file gets their own link, and the first one
	// keeps theirs.
	out = AddSessionCharacter(out, SessionCharacter{
		Id: "durnan-7b21", Name: "Durnan", File: "/gtd/dnd/durnan.org"})
	cast = ParseSessionCharacters(out)
	if len(cast) != 2 || cast[0].File != "/gtd/dnd/lyra.org" ||
		cast[1].File != "/gtd/dnd/durnan.org" {
		t.Fatalf("two characters, two links: %#v\n%s", cast, out)
	}
}

// The sheet rolls two d20 and shows all three readings of them; settling on
// one afterwards must rewrite the row it already wrote rather than add a
// second roll, and must leave the rolls either side of it alone.
func TestUpdateRollSettlesOneRowInPlace(t *testing.T) {
	out := AppendRolls(emptySession, []SessionRoll{
		{Time: "19:33", Character: "Lyra", Label: "Perception", Formula: "d20 +5",
			Result: "18", Dice: "d20 13, d20 17", Notes: "adv 22, dis 18"},
		{Time: "19:40", Character: "Lyra", Label: "Stealth", Formula: "d20 +7",
			Result: "12", Dice: "d20 5, d20 9", Notes: "adv 16, dis 12"},
	})
	out, err := UpdateRoll(out, 0, SessionRoll{Result: "22",
		Notes: "advantage chosen; normal 18, adv 22, dis 18"}, "Perception")
	if err != nil {
		t.Fatalf("settling the roll was refused: %s", err)
	}
	rolls := ParseRolls(out)
	if len(rolls) != 2 {
		t.Fatalf("the edit changed how many rolls there are: %#v", rolls)
	}
	if rolls[0].Result != "22" || !strings.Contains(rolls[0].Notes, "advantage chosen") {
		t.Fatalf("the roll was not settled: %#v", rolls[0])
	}
	// Cells the edit said nothing about keep what the row was written with.
	if rolls[0].Time != "19:33" || rolls[0].Character != "Lyra" ||
		rolls[0].Label != "Perception" || rolls[0].Formula != "d20 +5" {
		t.Fatalf("the edit lost part of the row: %#v", rolls[0])
	}
	if rolls[1].Result != "12" || rolls[1].Label != "Stealth" {
		t.Fatalf("the roll after it was disturbed: %#v", rolls[1])
	}
	if strings.Count(out, "#+NAME: rolls") != 1 {
		t.Fatalf("the table was duplicated:\n%s", out)
	}
}

func TestUpdateRollRefusesWhatItCannotBeSureOf(t *testing.T) {
	out := AppendRolls(emptySession, []SessionRoll{
		{Time: "19:33", Character: "Lyra", Label: "Perception", Result: "18"},
	})
	if _, err := UpdateRoll(out, 1, SessionRoll{Result: "22"}, ""); err == nil {
		t.Fatalf("a roll that is not there was edited anyway")
	}
	if _, err := UpdateRoll(out, 0, SessionRoll{Result: "22"}, "Stealth"); err == nil {
		t.Fatalf("a stale edit was written over the wrong roll")
	}
	if _, err := UpdateRoll(emptySession, 0, SessionRoll{Result: "22"}, ""); err == nil {
		t.Fatalf("a session with no rolls accepted an edit")
	}
}
