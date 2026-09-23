package dnd

import (
	"strings"
	"testing"
)

const markSession = `#+TITLE: Goblin Ambush
#+DATE: [2025-09-05 Fri]
#+SUMMARY: Ambushed on the road.
#+FILETAGS: :dnd:session:

* Characters
** Lyra
   :PROPERTIES:
   :DND_ID: lyra-4c1f
   :END:

* Notes
** 19:32
   The wagon tracks leave the road here.

** 19:41
   Kael is down.

* Rolls
#+NAME: rolls
| Time  | Character | Roll       | Formula | Result | Dice   | Notes |
|-------+-----------+------------+---------+--------+--------+-------|
| 19:33 | Lyra      | Perception | d20 +5  | 18     | d20 13 |       |
| 19:34 | Lyra      | Longsword  | d20 +7  | 21     | d20 14 |       |
| 19:36 | Lyra      | Longsword  | 1d8 +4  | 9      | d8 5   |       |
`

func TestSetMarkWritesATimelineSection(t *testing.T) {
	out, err := SetMark(markSession, SessionMark{
		Time: "19:32", Kind: "fight", Title: "Ambush at the ford",
		Note: "They came out of the reeds.\nKael went down.",
	})
	if err != nil {
		t.Fatalf("SetMark: %s", err)
	}
	if !strings.Contains(out, "* Timeline") {
		t.Fatalf("no Timeline section written:\n%s", out)
	}
	if !strings.Contains(out, "** 19:32 Ambush at the ford :fight:") {
		t.Fatalf("heading not as expected:\n%s", out)
	}
	// It goes above the notes, so the file reads who, shape, notes, rolls.
	if strings.Index(out, "* Timeline") > strings.Index(out, "* Notes") {
		t.Fatalf("Timeline written below Notes:\n%s", out)
	}
	// And nothing that was already there moved.
	for _, want := range []string{"** 19:32\n   The wagon tracks", "| 19:36 | Lyra"} {
		if !strings.Contains(out, want) {
			t.Fatalf("lost %q:\n%s", want, out)
		}
	}

	got := ParseMarks(out)
	if len(got) != 1 {
		t.Fatalf("read back %d marks, want 1: %#v", len(got), got)
	}
	if got[0].Time != "19:32" || got[0].Kind != "fight" ||
		got[0].Title != "Ambush at the ford" {
		t.Fatalf("anchor did not round trip: %#v", got[0])
	}
	if got[0].Note != "They came out of the reeds.\nKael went down." {
		t.Fatalf("note did not round trip: %q", got[0].Note)
	}
}

func TestSetMarkReplacesTheOneOnThatAnchor(t *testing.T) {
	out, err := SetMark(markSession, SessionMark{
		Time: "19:32", Kind: "fight", Title: "The ambush"})
	if err != nil {
		t.Fatalf("SetMark: %s", err)
	}
	out, err = SetMark(out, SessionMark{
		Time: "21:10", Kind: "note", Title: "The library"})
	if err != nil {
		t.Fatalf("SetMark: %s", err)
	}
	// Same anchor again: a rename, not a second entry.
	out, err = SetMark(out, SessionMark{
		Time: "19:32", Kind: "fight", Title: "Ambush at the ford",
		Note: "On both sides."})
	if err != nil {
		t.Fatalf("SetMark: %s", err)
	}
	marks := ParseMarks(out)
	if len(marks) != 2 {
		t.Fatalf("want 2 marks, got %d: %#v", len(marks), marks)
	}
	if marks[0].Title != "Ambush at the ford" || marks[0].Note != "On both sides." {
		t.Fatalf("the rewrite did not land: %#v", marks[0])
	}
	if marks[1].Title != "The library" {
		t.Fatalf("the other mark moved or changed: %#v", marks[1])
	}
	// A block on one anchor and a block on another of the same time are two
	// different things.
	out, err = SetMark(out, SessionMark{Time: "19:32", Kind: "note", Title: "A whisper"})
	if err != nil {
		t.Fatalf("SetMark: %s", err)
	}
	if got := len(ParseMarks(out)); got != 3 {
		t.Fatalf("kind is part of the anchor: want 3 marks, got %d", got)
	}
}

func TestSetMarkNeedsSomethingToSay(t *testing.T) {
	if _, err := SetMark(markSession, SessionMark{Time: "19:32", Kind: "fight"}); err == nil {
		t.Fatalf("an empty annotation should be refused - deleting is DeleteMark")
	}
	if _, err := SetMark(markSession, SessionMark{Title: "Nowhere"}); err == nil {
		t.Fatalf("an annotation with no anchor should be refused")
	}
}

func TestMarkTitleThatLooksLikeATag(t *testing.T) {
	out, err := SetMark(markSession, SessionMark{
		Time: "19:32", Kind: "fight", Title: "the :trap:"})
	if err != nil {
		t.Fatalf("SetMark: %s", err)
	}
	got := ParseMarks(out)
	if len(got) != 1 {
		t.Fatalf("want 1 mark, got %d", len(got))
	}
	if got[0].Kind != "fight" {
		t.Fatalf("a tag-shaped title stole the kind: %#v", got[0])
	}
	if got[0].Title != "the :trap" {
		t.Fatalf("title should lose its last colon and nothing else: %q", got[0].Title)
	}
}

func TestDeleteMarkTakesOneOffAndLeavesTheRest(t *testing.T) {
	out, _ := SetMark(markSession, SessionMark{Time: "19:32", Kind: "fight", Title: "One"})
	out, _ = SetMark(out, SessionMark{Time: "21:10", Kind: "note", Title: "Two"})
	out, err := DeleteMark(out, "19:32", "fight")
	if err != nil {
		t.Fatalf("DeleteMark: %s", err)
	}
	marks := ParseMarks(out)
	if len(marks) != 1 || marks[0].Title != "Two" {
		t.Fatalf("wrong one deleted: %#v", marks)
	}
	if _, err := DeleteMark(out, "19:32", "fight"); err == nil {
		t.Fatalf("deleting an annotation that is not there should be refused")
	}
	// And DropMark does not mind, because a block being deleted may never
	// have been annotated at all.
	if again := DropMark(out, "19:32", "fight"); again != out {
		t.Fatalf("DropMark changed a file with nothing to drop")
	}
	out, err = DeleteMark(out, "21:10", "note")
	if err != nil {
		t.Fatalf("DeleteMark: %s", err)
	}
	if got := ParseMarks(out); len(got) != 0 {
		t.Fatalf("want no marks left, got %#v", got)
	}
	// The last one out takes the section with it, the way the last row out
	// takes the roll table.
	if strings.Contains(out, "* Timeline") {
		t.Fatalf("an empty Timeline section was left behind:\n%s", out)
	}
	if !strings.Contains(out, ":END:\n\n* Notes") {
		t.Fatalf("the blank line between the sections went with it:\n%s", out)
	}
	// And the file is back to exactly what it was before anything was
	// annotated, which is what makes an annotation cost nothing to try.
	if out != markSession {
		t.Fatalf("annotating and unannotating did not round trip:\n%s", out)
	}
}

// A section somebody has written in themselves is not this code's to tidy.
func TestDeleteMarkLeavesASectionSomebodyElseIsUsing(t *testing.T) {
	out, _ := SetMark(markSession, SessionMark{Time: "19:33", Kind: "fight", Title: "One"})
	out = strings.Replace(out, "* Timeline\n",
		"* Timeline\n** Things to remember\n   Not an annotation.\n", 1)
	out, err := DeleteMark(out, "19:33", "fight")
	if err != nil {
		t.Fatalf("DeleteMark: %s", err)
	}
	if !strings.Contains(out, "* Timeline") ||
		!strings.Contains(out, "** Things to remember") {
		t.Fatalf("somebody's own section was thrown away:\n%s", out)
	}
}

func TestDeleteEntriesTakesAWholeBlock(t *testing.T) {
	// A combat block: the last two rolls, and the note written during it.
	out, err := DeleteEntries(markSession,
		[]SessionRef{{Index: 1, Was: "Longsword"}, {Index: 2, Was: "Longsword"}},
		[]SessionRef{{Index: 1, Was: "19:41"}})
	if err != nil {
		t.Fatalf("DeleteEntries: %s", err)
	}
	rolls := ParseRolls(out)
	if len(rolls) != 1 || rolls[0].Label != "Perception" {
		t.Fatalf("want only the Perception roll left, got %#v", rolls)
	}
	notes := ParseNotes(out)
	if len(notes) != 1 || notes[0].Time != "19:32" {
		t.Fatalf("want only the first note left, got %#v", notes)
	}
}

func TestDeleteEntriesRefusesTheLotOrNoneOfIt(t *testing.T) {
	// The second roll named is not what the page thought it was, so nothing
	// goes - half a fight deleted would be worse than none of it.
	out, err := DeleteEntries(markSession,
		[]SessionRef{{Index: 0, Was: "Perception"}, {Index: 1, Was: "Dagger"}}, nil)
	if err == nil {
		t.Fatalf("a stale reading should refuse the whole block")
	}
	if out != markSession {
		t.Fatalf("a refused delete must leave the file alone")
	}
	if _, err := DeleteEntries(markSession, nil, nil); err == nil {
		t.Fatalf("naming nothing should be refused")
	}
}

func TestDeleteEntriesCountsFromTheBack(t *testing.T) {
	// Rolls handed over out of order and with a repeat in them: deleting
	// front to back would shift the later indexes out from under it.
	out, err := DeleteEntries(markSession,
		[]SessionRef{{Index: 0}, {Index: 2}, {Index: 0}}, nil)
	if err != nil {
		t.Fatalf("DeleteEntries: %s", err)
	}
	rolls := ParseRolls(out)
	if len(rolls) != 1 || rolls[0].Formula != "d20 +7" {
		t.Fatalf("want the middle roll left, got %#v", rolls)
	}
}

func TestParseMarksLeavesProseAlone(t *testing.T) {
	text := markSession + "\n* Timeline\n** Things to remember\n   Not an annotation.\n" +
		"** 22:00 The road home :note:\n   Quiet all the way back.\n"
	marks := ParseMarks(text)
	if len(marks) != 1 || marks[0].Title != "The road home" {
		t.Fatalf("want just the one annotation, got %#v", marks)
	}
	out, err := SetMark(text, SessionMark{Time: "22:00", Kind: "note", Title: "Home"})
	if err != nil {
		t.Fatalf("SetMark: %s", err)
	}
	if !strings.Contains(out, "** Things to remember") {
		t.Fatalf("somebody's own heading was thrown away:\n%s", out)
	}
}
