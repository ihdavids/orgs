// Tests for the inventory: stacking, containers, encumbrance, the changes the
// character sheet posts, and the round trip through the org file.
package dnd

import (
	"strings"
	"testing"
)

func testChar() *Character {
	return &Character{
		Name:    "Lyra",
		Ruleset: DefaultRuleset,
		Race:    "human",
		Classes: []ClassLevel{{Class: "fighter", Level: 3}},
		Abilities: map[string]int{
			STR: 10, DEX: 14, CON: 14, INT: 12, WIS: 10, CHA: 8,
		},
	}
}

func entryFor(v InventoryView, container, name string) *InventoryEntry {
	box := v.Container(container)
	if box == nil {
		return nil
	}
	for i := range box.Entries {
		if strings.EqualFold(box.Entries[i].Name, name) {
			return &box.Entries[i]
		}
	}
	return nil
}

func TestItemsStackAndCount(t *testing.T) {
	rs := srd(t)
	c := testChar()
	for i := 0; i < 3; i++ {
		if _, err := InventoryAdd(c, rs, "potion-of-healing", "", 1, "", ""); err != nil {
			t.Fatalf("add: %v", err)
		}
	}
	if len(c.Equipment) != 1 {
		t.Fatalf("want one stacked line, got %d", len(c.Equipment))
	}
	v := ComputeInventory(c.Equipment, rs, 10)
	e := entryFor(v, "", "Potion of Healing")
	if e == nil || e.Qty != 3 {
		t.Fatalf("want a stack of 3, got %+v", e)
	}
	if len(c.InventoryLog) != 3 {
		t.Fatalf("want 3 history entries, got %d", len(c.InventoryLog))
	}
	if c.InventoryLog[0].Action != InvAdded || c.InventoryLog[0].To != CarriedLabel {
		t.Fatalf("bad history entry %+v", c.InventoryLog[0])
	}
}

func TestConsumeTakesOneAndEmptiesTheLine(t *testing.T) {
	rs := srd(t)
	c := testChar()
	InventoryAdd(c, rs, "potion-of-healing", "", 2, "", "")
	if _, err := InventoryRemove(c, rs, "potion-of-healing", 1, "", InvUsed, ""); err != nil {
		t.Fatalf("use: %v", err)
	}
	if c.Equipment[0].Qty != 1 {
		t.Fatalf("want 1 left, got %d", c.Equipment[0].Qty)
	}
	if _, err := InventoryRemove(c, rs, "potion-of-healing", 1, "", InvUsed, ""); err != nil {
		t.Fatalf("use: %v", err)
	}
	if len(c.Equipment) != 0 {
		t.Fatalf("want the line gone, got %+v", c.Equipment)
	}
	last := c.InventoryLog[len(c.InventoryLog)-1]
	if last.Action != InvUsed || last.From != CarriedLabel || last.Qty != 1 {
		t.Fatalf("bad history entry %+v", last)
	}
	if _, err := InventoryRemove(c, rs, "potion-of-healing", 1, "", InvUsed, ""); err == nil {
		t.Fatal("using something you do not have should fail")
	}
}

func TestContainersMustBeOwned(t *testing.T) {
	rs := srd(t)
	c := testChar()
	if _, err := InventoryAdd(c, rs, "rations-1-day", "", 5, "backpack", ""); err == nil {
		t.Fatal("storing into a backpack you do not own should fail")
	}
	InventoryAdd(c, rs, "backpack", "", 1, "", "")
	if _, err := InventoryAdd(c, rs, "rations-1-day", "", 5, "backpack", ""); err != nil {
		t.Fatalf("add to backpack: %v", err)
	}
	v := ComputeInventory(c.Equipment, rs, 10)
	if box := v.Container("backpack"); box == nil || box.Items != 5 {
		t.Fatalf("want 5 rations in the backpack, got %+v", box)
	}
	if v.Container("") == nil || len(v.Container("").Entries) != 1 {
		t.Fatal("the backpack itself should still be carried on the person")
	}
}

func TestMoveBetweenContainers(t *testing.T) {
	rs := srd(t)
	c := testChar()
	InventoryAdd(c, rs, "backpack", "", 1, "", "")
	InventoryAdd(c, rs, "pouch", "", 1, "", "")
	InventoryAdd(c, rs, "rations-1-day", "", 4, "backpack", "")
	if _, err := InventoryMove(c, rs, "rations-1-day", 2, "backpack", "pouch"); err != nil {
		t.Fatalf("move: %v", err)
	}
	v := ComputeInventory(c.Equipment, rs, 10)
	if e := entryFor(v, "backpack", "Rations (1 day)"); e == nil || e.Qty != 2 {
		t.Fatalf("want 2 left in the backpack, got %+v", e)
	}
	if e := entryFor(v, "pouch", "Rations (1 day)"); e == nil || e.Qty != 2 {
		t.Fatalf("want 2 in the pouch, got %+v", e)
	}
	if _, err := InventoryMove(c, rs, "backpack", 1, "", "backpack"); err == nil {
		t.Fatal("a backpack should not fit inside itself")
	}
	last := c.InventoryLog[len(c.InventoryLog)-1]
	if last.Action != InvMoved || last.From != "Backpack" || last.To != "Pouch" {
		t.Fatalf("bad history entry %+v", last)
	}
}

func TestDroppingAContainerKeepsItsContents(t *testing.T) {
	rs := srd(t)
	c := testChar()
	InventoryAdd(c, rs, "backpack", "", 1, "", "")
	InventoryAdd(c, rs, "rations-1-day", "", 3, "backpack", "")
	if _, err := InventoryRemove(c, rs, "backpack", 1, "", InvDropped, ""); err != nil {
		t.Fatalf("drop: %v", err)
	}
	v := ComputeInventory(c.Equipment, rs, 10)
	if e := entryFor(v, "", "Rations (1 day)"); e == nil || e.Qty != 3 {
		t.Fatalf("rations should have come out onto the character, got %+v", e)
	}
}

func TestExtradimensionalWeightIsNotCarried(t *testing.T) {
	rs := srd(t)
	c := testChar()
	InventoryAdd(c, rs, "bag-of-holding", "", 1, "", "")
	InventoryAdd(c, rs, "chain-mail", "", 2, "bag-of-holding", "")
	v := ComputeInventory(c.Equipment, rs, 10)
	if v.Stored < 100 {
		t.Fatalf("want the mail stowed in the bag, got %v", v.Stored)
	}
	bag := entryFor(v, "", "Bag of Holding")
	if bag == nil {
		t.Fatal("no bag of holding on the person")
	}
	if v.Weight != bag.Total {
		t.Fatalf("only the bag itself should be carried, got %v", v.Weight)
	}
}

func TestEncumbranceScale(t *testing.T) {
	rs := srd(t)
	if v := ComputeInventory(testChar().Equipment, rs, 10); v.Level != "" || v.Label != "Unencumbered" {
		t.Fatalf("an empty pack should be unencumbered, got %q", v.Label)
	}
	// Chain mail is 55 lb, so with Strength 10 - encumbered over 50, heavily
	// over 100, capacity 150 - each suit steps one rung up the scale.
	cases := []struct {
		qty   int
		level string
	}{{qty: 1, level: "encumbered"}, {qty: 2, level: "heavy"}, {qty: 3, level: "over"}}
	for _, tc := range cases {
		c := testChar()
		InventoryAdd(c, rs, "chain-mail", "", tc.qty, "", "")
		v := ComputeInventory(c.Equipment, rs, 10)
		if v.Level != tc.level {
			t.Fatalf("%d chain mail: want level %q, got %q (%v lb)",
				tc.qty, tc.level, v.Level, v.Weight)
		}
	}
}

func TestInventoryRoundTripsThroughTheOrgFile(t *testing.T) {
	rs := srd(t)
	c := testChar()
	InventoryAdd(c, rs, "backpack", "", 1, "", "")
	InventoryAdd(c, rs, "rations-1-day", "", 5, "backpack", "")
	InventoryRemove(c, rs, "rations-1-day", 1, "backpack", InvUsed, "breakfast")

	org := RenderOrg(c, rs)
	if !strings.Contains(org, "** "+InventoryHistoryHeading) {
		t.Fatal("no inventory history section written")
	}
	back, err := ParseOrg(org, rs)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	v := ComputeInventory(Compute(back, rs).Equipment, rs, 10)
	e := entryFor(v, "backpack", "Rations (1 day)")
	if e == nil || e.Qty != 4 {
		t.Fatalf("want 4 rations still in the backpack after a round trip, got %+v", e)
	}
	if len(back.InventoryLog) != len(c.InventoryLog) {
		t.Fatalf("want %d history entries back, got %d",
			len(c.InventoryLog), len(back.InventoryLog))
	}
	last := back.InventoryLog[len(back.InventoryLog)-1]
	if last.Action != InvUsed || last.Item != "Rations (1 day)" || last.Notes != "breakfast" {
		t.Fatalf("history did not survive the round trip: %+v", last)
	}
}

// ----------------------------------------------------------------------------
// The groups the add item box searches in
// ----------------------------------------------------------------------------

func filterFor(t *testing.T, id string) ItemFilter {
	t.Helper()
	f, ok := FindItemFilter(id)
	if !ok {
		t.Fatalf("no item filter %q", id)
	}
	return f
}

func TestEveryFilterKeepsOnlyItsOwn(t *testing.T) {
	rs := srd(t)
	// One thing every group has to hold, and what it must not let through.
	cases := []struct{ filter, wanted, unwanted string }{
		{"weapon", "Longsword", "Backpack"},
		{"armor", "Plate", "Longsword"},
		{"potion", "Potion of Healing", "Longsword"},
		{"focus", "Component pouch", "Longsword"},
		{"tool", "Thieves' tools", "Longsword"},
		{"pack", "Explorer's Pack", "Longsword"},
		{"container", "Backpack", "Longsword"},
		{"magic", "Bag of Holding", "Backpack"},
	}
	for _, tc := range cases {
		hits := SearchItems(rs, "", 0xffff, filterFor(t, tc.filter), false, nil)
		if len(hits) == 0 {
			t.Fatalf("%s: no items at all", tc.filter)
		}
		found, wrong := false, false
		for _, h := range hits {
			if strings.EqualFold(h.Name, tc.wanted) {
				found = true
			}
			if strings.EqualFold(h.Name, tc.unwanted) {
				wrong = true
			}
		}
		if !found {
			t.Errorf("%s: expected it to hold %q", tc.filter, tc.wanted)
		}
		if wrong {
			t.Errorf("%s: %q does not belong in it", tc.filter, tc.unwanted)
		}
	}
}

func TestFilteringNarrowsASearchWithoutReorderingIt(t *testing.T) {
	rs := srd(t)
	all := SearchItems(rs, "sword", 0xffff, filterFor(t, "all"), false, nil)
	magic := SearchItems(rs, "sword", 0xffff, filterFor(t, "magic"), false, nil)
	if len(magic) == 0 || len(magic) >= len(all) {
		t.Fatalf("magic swords (%d) should be some but not all of %d", len(magic), len(all))
	}
	// The same hits in the same order, with the mundane ones taken out.
	at := 0
	for _, h := range all {
		if at < len(magic) && h.Id == magic[at].Id {
			at++
		}
	}
	if at != len(magic) {
		t.Fatalf("filtering reordered the search: matched %d of %d", at, len(magic))
	}
	for _, h := range magic {
		if h.Rarity == "" {
			t.Fatalf("%q is not magic", h.Name)
		}
	}
}

func TestUnknownFilterIsAnErrorAndEmptyIsAll(t *testing.T) {
	if _, ok := FindItemFilter("cheese"); ok {
		t.Fatal("a name no filter goes by should not be found")
	}
	for _, id := range []string{"", "all", "ALL", " magic "} {
		if _, ok := FindItemFilter(id); !ok {
			t.Fatalf("%q should name a filter", id)
		}
	}
	if f, _ := FindItemFilter(""); f.Match != nil {
		t.Fatal("the default filter should let everything through")
	}
}

func TestMagicItemsCarryTheirRarityIntoTheBag(t *testing.T) {
	rs := srd(t)
	c := testChar()
	if _, err := InventoryAdd(c, rs, "bag-of-holding", "", 1, "", ""); err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, err := InventoryAdd(c, rs, "backpack", "", 1, "", ""); err != nil {
		t.Fatalf("add: %v", err)
	}
	v := ComputeInventory(c.Equipment, rs, 10)
	if e := entryFor(v, "", "Bag of Holding"); e == nil || e.Rarity != "Uncommon" {
		t.Fatalf("want an uncommon bag of holding, got %+v", e)
	}
	if e := entryFor(v, "", "Backpack"); e == nil || e.Rarity != "" {
		t.Fatalf("a backpack has no rarity, got %+v", e)
	}
}

// ----------------------------------------------------------------------------
// Wearing and wielding
// ----------------------------------------------------------------------------

func TestWhatCanBeWornOrWielded(t *testing.T) {
	rs := srd(t)
	for _, tc := range []struct {
		id   string
		want bool
	}{
		{"chain-mail", true},         // armour
		{"shield", true},             // a shield
		{"longsword", true},          // a weapon
		{"ring-of-protection", true}, // a ring
		{"cloak-of-protection", true},
		{"wand-of-magic-missiles", true},
		{"rope-hempen-50-feet", false}, // carried, never worn
		{"rations-1-day", false},
		{"potion-of-healing", false},
		{"thieves-tools", false},
		{"backpack", false},       // a container is carried
		{"bag-of-holding", false}, // wondrous, but still a container
	} {
		it := rs.Item(tc.id)
		if it == nil {
			t.Fatalf("the srd has no %s to test with", tc.id)
		}
		if got := CanEquip(it); got != tc.want {
			t.Errorf("CanEquip(%s) = %v, want %v (kind %q)", tc.id, got, tc.want, it.Kind)
		}
	}
	// Homebrew the ruleset has never heard of is the player's business.
	if !CanEquip(nil) {
		t.Error("an item the ruleset does not know should still be wearable")
	}
}

func TestEquippingWearsTheWholeStackAndChangesAC(t *testing.T) {
	rs := srd(t)
	c := testChar()
	InventoryAdd(c, rs, "chain-mail", "", 1, "", "")
	InventoryAdd(c, rs, "dagger", "", 2, "", "")

	bare := Compute(c, rs).AC
	if _, err := InventoryEquip(c, rs, "chain-mail", "", true); err != nil {
		t.Fatalf("equip: %v", err)
	}
	if got := Compute(c, rs).AC; got <= bare {
		t.Fatalf("wearing chain mail should raise AC above %d, got %d", bare, got)
	}

	// One line of two daggers is drawn as a pair, not half a pair.
	ev, err := InventoryEquip(c, rs, "dagger", "", true)
	if err != nil {
		t.Fatalf("equip daggers: %v", err)
	}
	if ev.Action != InvWorn || ev.Qty != 2 {
		t.Fatalf("want both daggers worn, got %+v", ev)
	}
	v := ComputeInventory(Compute(c, rs).Equipment, rs, 10)
	if e := entryFor(v, "", "Chain Mail"); e == nil || !e.Equipped || !e.Wearable {
		t.Fatalf("chain mail should read as worn and wearable, got %+v", e)
	}
	if e := entryFor(v, "", "Rope, Hempen (50 feet)"); e != nil {
		t.Fatal("test does not carry rope")
	}

	// And taking it off puts the AC back where it started.
	if _, err := InventoryEquip(c, rs, "chain-mail", "", false); err != nil {
		t.Fatalf("unequip: %v", err)
	}
	if got := Compute(c, rs).AC; got != bare {
		t.Fatalf("taking the armour off should return AC to %d, got %d", bare, got)
	}
	// None of it belongs in the history: it is not gaining or losing anything.
	for _, e := range c.InventoryLog {
		if e.Action == InvWorn || e.Action == InvRemoved {
			t.Fatalf("wearing should not be logged, found %+v", e)
		}
	}
}

func TestWhatCannotBeWorn(t *testing.T) {
	rs := srd(t)
	c := testChar()
	InventoryAdd(c, rs, "backpack", "", 1, "", "")
	InventoryAdd(c, rs, "rope-hempen-50-feet", "", 1, "", "")
	InventoryAdd(c, rs, "chain-mail", "", 1, "backpack", "")

	if _, err := InventoryEquip(c, rs, "rope-hempen-50-feet", "", true); err == nil {
		t.Fatal("rope is carried, not worn")
	}
	// Armour in a backpack has to come out before it goes on, which is the
	// same rule that unequips anything packed away.
	if _, err := InventoryEquip(c, rs, "chain-mail", "backpack", true); err == nil {
		t.Fatal("armour stowed in a backpack should not be wearable there")
	}
	// Taking something off is always allowed, whatever it is: a line already
	// marked equipped has to be clearable.
	if _, err := InventoryEquip(c, rs, "chain-mail", "backpack", false); err != nil {
		t.Fatalf("unequip should always be allowed: %v", err)
	}
	if _, err := InventoryEquip(c, rs, "plate", "", true); err == nil {
		t.Fatal("you cannot wear armour you do not have")
	}
}

func TestWornSurvivesTheOrgFile(t *testing.T) {
	rs := srd(t)
	c := testChar()
	InventoryAdd(c, rs, "leather-armor", "", 1, "", "")
	if _, err := InventoryEquip(c, rs, "leather-armor", "", true); err != nil {
		t.Fatalf("equip: %v", err)
	}
	back, err := ParseOrg(RenderOrg(c, rs), rs)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	v := ComputeInventory(Compute(back, rs).Equipment, rs, 10)
	if e := entryFor(v, "", "Leather Armor"); e == nil || !e.Equipped {
		t.Fatalf("worn armour did not survive the round trip: %+v", e)
	}
}
