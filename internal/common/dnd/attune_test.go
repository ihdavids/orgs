package dnd

import "testing"

func attuneChar() (*Character, *Ruleset) {
	rs := NewLibrary(nil).Get("srd")
	c := &Character{
		Name:      "Magpie",
		Ruleset:   "srd",
		Abilities: map[string]int{"str": 10, "dex": 10, "con": 10, "int": 10, "wis": 10, "cha": 10},
		Classes:   []ClassLevel{{Class: "fighter", Level: 1}},
		Equipment: []Gear{
			{Id: "ring-of-protection", Name: "Ring of Protection", Qty: 1, Equipped: true},
			{Id: "amulet-of-health", Name: "Amulet of Health", Qty: 1, Equipped: true},
			{Id: "boots-of-levitation", Name: "Boots of Levitation", Qty: 1, Equipped: true},
			{Id: "bracers-of-archery", Name: "Bracers of Archery", Qty: 1, Equipped: true},
			{Id: "longsword", Name: "Longsword", Qty: 1},
		},
	}
	return c, rs
}

func TestAttuneAndGiveUp(t *testing.T) {
	c, rs := attuneChar()
	if _, err := InventoryAttune(c, rs, "ring-of-protection", "", true); err != nil {
		t.Fatal(err)
	}
	if !c.Equipment[0].Attuned {
		t.Error("the ring is not marked attuned")
	}
	// An attuned item's bonuses are live, which is the whole point of doing it.
	if s := Compute(c, rs); s.AttunementUsed != 1 {
		t.Errorf("attunement used = %d, want 1", s.AttunementUsed)
	}
	if _, err := InventoryAttune(c, rs, "ring-of-protection", "", false); err != nil {
		t.Fatal(err)
	}
	if c.Equipment[0].Attuned {
		t.Error("the ring is still attuned after giving it up")
	}
}

// Three is all anyone gets, and the button refuses the fourth rather than
// letting the sheet carry an item that silently does nothing.
func TestAttuneStopsAtThree(t *testing.T) {
	c, rs := attuneChar()
	for _, id := range []string{"ring-of-protection", "amulet-of-health", "boots-of-levitation"} {
		if _, err := InventoryAttune(c, rs, id, "", true); err != nil {
			t.Fatalf("%s: %s", id, err)
		}
	}
	if _, err := InventoryAttune(c, rs, "bracers-of-archery", "", true); err == nil {
		t.Fatal("attuned to a fourth item")
	}
	if c.Equipment[3].Attuned {
		t.Error("the bracers were attuned anyway")
	}
	s := Compute(c, rs)
	if s.AttunementUsed != AttunementSlots || s.AttunementOver != 0 {
		t.Errorf("used = %d, over = %d", s.AttunementUsed, s.AttunementOver)
	}
	// Setting one that is already attuned again is not a fourth attunement.
	if _, err := InventoryAttune(c, rs, "amulet-of-health", "", true); err != nil {
		t.Errorf("re-attuning an attuned item: %s", err)
	}
}

// Something that does not ask for attunement cannot be attuned to.
func TestAttuneOnlyWhatAsksForIt(t *testing.T) {
	c, rs := attuneChar()
	if _, err := InventoryAttune(c, rs, "longsword", "", true); err == nil {
		t.Fatal("attuned to an ordinary longsword")
	}
	if _, err := InventoryAttune(c, rs, "shortbow", "", true); err == nil {
		t.Fatal("attuned to something not carried at all")
	}
}

// A sheet edited by hand can claim four attunements. The rules engine leaves
// the fourth inert and the counter says so, so the player can see why their
// boots are doing nothing.
func TestAttunementCounterReportsOverflow(t *testing.T) {
	c, rs := attuneChar()
	for i := 0; i < 4; i++ {
		c.Equipment[i].Attuned = true
	}
	s := Compute(c, rs)
	view := ComputeAttunement(s)
	if view.Used != AttunementSlots || view.Slots != AttunementSlots {
		t.Errorf("used %d of %d", view.Used, view.Slots)
	}
	if !view.Over {
		t.Error("four attunements did not read as over the limit")
	}
	if len(view.Items) != AttunementSlots {
		t.Errorf("items = %v", view.Items)
	}

	// And with three, nothing is over.
	c.Equipment[3].Attuned = false
	if view := ComputeAttunement(Compute(c, rs)); view.Over {
		t.Error("three attunements read as over the limit")
	}
}

// The inventory entry has to say which lines are worth offering the mark on.
func TestInventoryEntryCarriesAttunement(t *testing.T) {
	c, rs := attuneChar()
	inv := Compute(c, rs).Inventory
	box := inv.Container("")
	if box == nil {
		t.Fatal("nothing carried on the person")
	}
	seen := map[string]bool{}
	for _, e := range box.Entries {
		seen[e.Name] = e.Attunement
	}
	if !seen["Ring of Protection"] {
		t.Error("the ring does not say it needs attunement")
	}
	if seen["Longsword"] {
		t.Error("a longsword says it needs attunement")
	}
}
