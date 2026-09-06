package dnd

import "testing"

// The add on modules shipped in templates/dnd have to load and index cleanly.
// They are data, so nothing else would catch a typo in them.
func TestShippedModulesLoad(t *testing.T) {
	lib := NewLibrary([]string{"../../../templates/dnd"})
	if len(lib.Errors) > 0 {
		t.Fatalf("module load errors: %v", lib.Errors)
	}
	rs := lib.Get("srd")
	if rs == nil {
		t.Fatal("no srd ruleset")
	}
	for _, id := range []string{"booming-blade", "frostbite", "silvery-barbs"} {
		if rs.Spell(id) == nil {
			t.Errorf("spell %s did not load", id)
		}
	}
	for _, id := range []string{"string", "bath-potion", "bottled-slime"} {
		it := rs.Item(id)
		if it == nil {
			t.Fatalf("item %s did not load", id)
		}
		t.Logf("%s: kind=%s rarity=%q magic=%v", id, it.Kind, it.Rarity, it.IsMagic())
	}
	found := map[string]bool{}
	for _, f := range rs.Feats {
		found[f.Id] = true
	}
	for _, id := range []string{"arcane-artist", "heros-journey-boon", "dark-bargain"} {
		if !found[id] {
			t.Errorf("feat %s did not load", id)
		}
	}
	ok := false
	for _, b := range rs.Backgrounds {
		if b.Id == "phantasmic-circus-trouper" {
			ok = true
			t.Logf("bg skills=%v tools=%v equip=%d gold=%d", b.Proficiencies.Skills,
				b.Proficiencies.Tools, len(b.Equipment), b.Gold)
			for _, e := range b.Equipment {
				if rs.Item(e.Id) == nil {
					t.Errorf("background equipment %q is not an item", e.Id)
				}
			}
		}
	}
	if !ok {
		t.Error("phantasmic circus trouper did not load")
	}
	// The importer must now find all of these by their D&D Beyond names.
	idx := newNameIndex(rs)
	for name, want := range map[string]string{
		"Booming Blade":            "booming-blade",
		"Frostbite":                "frostbite",
		"Silvery Barbs":            "silvery-barbs",
		"Tasha's Hideous Laughter": "hideous-laughter",
	} {
		if got := idx.spells.find(name); got != want {
			t.Errorf("spell %q matched %q, want %q", name, got, want)
		}
	}
	for name, want := range map[string]string{
		"Bottled Slime": "bottled-slime",
		"Bath Potion":   "bath-potion",
		"String":        "string",
	} {
		if got := idx.items.find(name); got != want {
			t.Errorf("item %q matched %q, want %q", name, got, want)
		}
	}
	for name, want := range map[string]string{
		"Hero's Journey Boon": "heros-journey-boon",
		"Dark Bargain":        "dark-bargain",
		"Arcane Artist":       "arcane-artist",
	} {
		if got := idx.feats.find(name); got != want {
			t.Errorf("feat %q matched %q, want %q", name, got, want)
		}
	}
	if got := idx.bgs.find("Phantasmic Circus Trouper"); got != "phantasmic-circus-trouper" {
		t.Errorf("background matched %q", got)
	}
}

// Fast Movement is the case that showed the engine had no way to move a
// walking speed at all: a 5th level barbarian reads 30 without it.
func TestFastMovementRaisesSpeed(t *testing.T) {
	lib := NewLibrary([]string{"../../../templates/dnd"})
	rs := lib.Get("srd")
	if rs == nil {
		t.Fatal("no srd ruleset")
	}
	barb := func(level int, gear ...Gear) *Sheet {
		return Compute(&Character{
			Name:      "K",
			Race:      "human",
			Classes:   []ClassLevel{{Class: "barbarian", Level: level}},
			Abilities: map[string]int{STR: 16, DEX: 14, CON: 16, INT: 10, WIS: 12, CHA: 10},
			Equipment: gear,
		}, rs)
	}
	if got := barb(4).Speed; got != 30 {
		t.Errorf("speed at 4th = %d, want 30 - Fast Movement has not arrived yet", got)
	}
	if got := barb(5).Speed; got != 40 {
		t.Errorf("speed at 5th = %d, want 40 - Fast Movement did not apply", got)
	}
	// "while you aren't wearing heavy armor" is the whole of the condition:
	// medium armour leaves it alone.
	if got := barb(5, Gear{Id: "chain-mail", Equipped: true}).Speed; got != 30 {
		t.Errorf("speed in heavy armour = %d, want 30 - the unless clause did not fire", got)
	}
	if got := barb(5, Gear{Id: "scale-mail", Equipped: true}).Speed; got != 40 {
		t.Errorf("speed in medium armour = %d, want 40 - the unless clause fired too widely", got)
	}
}
