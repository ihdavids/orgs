// Tests for spending a spell slot: what a cast costs, when it reaches for a
// higher slot, and what happens when there is nothing left to spend.
package dnd

import (
	"strings"
	"testing"
)

func TestCastSpendsASlotOfItsOwnLevel(t *testing.T) {
	rs := srd(t)
	c := rester("wizard", 5)

	ch, err := ApplySlotChange(c, rs, "cast", 2, "Misty Step")
	if err != nil {
		t.Fatalf("cast: %v", err)
	}
	// A 5th level wizard has three 2nd level slots.
	if ch.Level != 2 || ch.Up || ch.Left != 2 || ch.Total != 3 {
		t.Fatalf("want one of three 2nd level slots spent, got %+v", ch)
	}
	if len(c.SlotsUsed) != 2 || c.SlotsUsed[1] != 1 {
		t.Fatalf("slotsUsed = %v, want a single 2nd level slot spent", c.SlotsUsed)
	}
	if !strings.Contains(ch.Msg, "Misty Step") {
		t.Fatalf("the message should name the spell, got %q", ch.Msg)
	}
}

// A spell may always be cast from a higher slot, so an empty level reaches up
// rather than refusing.
func TestCastReachesUpWhenTheLevelIsEmpty(t *testing.T) {
	rs := srd(t)
	c := rester("wizard", 5)
	// Every 1st and 2nd level slot spent, and one of the two 3rd level ones.
	c.SlotsUsed = []int{4, 3, 1}

	ch, err := ApplySlotChange(c, rs, "cast", 1, "Magic Missile")
	if err != nil {
		t.Fatalf("cast: %v", err)
	}
	if ch.Level != 3 || ch.Asked != 1 || !ch.Up {
		t.Fatalf("want a 3rd level slot spent for a 1st level spell, got %+v", ch)
	}
	if !strings.Contains(ch.Msg, "3rd level") {
		t.Fatalf("the message should say where it came from, got %q", ch.Msg)
	}

	// That was the last one, so now there is nothing left at all.
	_, err = ApplySlotChange(c, rs, "cast", 1, "Magic Missile")
	if err == nil {
		t.Fatal("casting with every slot spent should be refused")
	}
	if len(c.SlotsUsed) != 3 || c.SlotsUsed[2] != 2 {
		t.Fatalf("a refused cast must write nothing, got %v", c.SlotsUsed)
	}
}

// Clicking a slot on the sheet means that slot and no other.
func TestSpendAndRecoverOneLevel(t *testing.T) {
	rs := srd(t)
	c := rester("wizard", 5)
	c.SlotsUsed = []int{4}

	if _, err := ApplySlotChange(c, rs, "spend", 1, ""); err == nil {
		t.Fatal("spending a level with nothing left should be refused, not raised")
	}
	ch, err := ApplySlotChange(c, rs, "recover", 1, "")
	if err != nil {
		t.Fatalf("recover: %v", err)
	}
	if ch.Left != 1 || c.SlotsUsed[0] != 3 {
		t.Fatalf("want one 1st level slot back, got %+v leaving %v", ch, c.SlotsUsed)
	}
	for i := 0; i < 3; i++ {
		if _, err := ApplySlotChange(c, rs, "recover", 1, ""); err != nil {
			t.Fatalf("recover %d: %v", i, err)
		}
	}
	if len(c.SlotsUsed) != 0 {
		t.Fatalf("a character with nothing spent stores nothing, got %v", c.SlotsUsed)
	}
	if _, err := ApplySlotChange(c, rs, "recover", 1, ""); err == nil {
		t.Fatal("there was nothing left to hand back")
	}
}

func TestSlotChangesTheRulesRefuse(t *testing.T) {
	rs := srd(t)
	wiz := rester("wizard", 5)
	for _, tc := range []struct {
		name   string
		c      *Character
		action string
		level  int
		want   string
	}{
		{"cantrip", wiz, "cast", 0, "no spell slot"},
		{"tenth level", wiz, "cast", 10, "no such thing"},
		{"a level too high for them", wiz, "cast", 5, "no 5th level slots"},
		{"not a caster at all", rester("fighter", 5), "cast", 1, "no spell slots"},
		{"nonsense", wiz, "burn", 1, "not \"burn\""},
	} {
		_, err := ApplySlotChange(tc.c, rs, tc.action, tc.level, "")
		if err == nil {
			t.Fatalf("%s: should have been refused", tc.name)
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%s: want %q in %q", tc.name, tc.want, err)
		}
	}
}
