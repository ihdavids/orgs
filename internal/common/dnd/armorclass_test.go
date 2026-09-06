package dnd

import (
	"strings"
	"testing"
)

// acCase builds a character and asks the rules engine what their armour class
// comes to, which is the only thing any of these tests care about.
func acCase(t *testing.T, classes []ClassLevel, abilities map[string]int, gear ...Gear) (int, string) {
	t.Helper()
	lib := NewLibrary(nil)
	lib.Resolve()
	rs := lib.Get("srd")
	if rs == nil {
		t.Skip("no srd ruleset")
	}
	c := &Character{
		Name: "Test", Race: "human", Background: "sage",
		Classes: classes, Abilities: abilities, Equipment: gear,
	}
	s := Compute(c, rs)
	return s.AC, s.ACSource
}

func worn(id string) Gear { return Gear{Id: id, Qty: 1, Equipped: true} }

func abilities(str, dex, con, intel, wis, cha int) map[string]int {
	return map[string]int{STR: str, DEX: dex, CON: con, INT: intel, WIS: wis, CHA: cha}
}

func TestACPlainUnarmored(t *testing.T) {
	ac, src := acCase(t, []ClassLevel{{Class: "fighter", Level: 1}},
		abilities(10, 14, 12, 10, 10, 10))
	if ac != 12 || src != "Unarmored" {
		t.Errorf("AC = %d (%s), want 12 Unarmored", ac, src)
	}
}

func TestACWornArmor(t *testing.T) {
	// Leather is 11 plus the whole dexterity modifier.
	ac, src := acCase(t, []ClassLevel{{Class: "rogue", Level: 5}},
		abilities(10, 18, 12, 10, 10, 10), worn("leather"))
	if ac != 15 || src != "Leather" {
		t.Errorf("AC = %d (%s), want 15 Leather", ac, src)
	}
}

func TestACMediumArmorCapsDex(t *testing.T) {
	// A breastplate takes at most +2 of dexterity, however nimble you are.
	ac, _ := acCase(t, []ClassLevel{{Class: "fighter", Level: 5}},
		abilities(14, 18, 14, 10, 10, 10), worn("breastplate"))
	if ac != 16 {
		t.Errorf("AC = %d, want 16 (14 + 2 capped dex)", ac)
	}
}

func TestACHeavyArmorIgnoresDex(t *testing.T) {
	ac, _ := acCase(t, []ClassLevel{{Class: "fighter", Level: 5}},
		abilities(16, 14, 14, 10, 10, 10), worn("chain-mail"))
	if ac != 16 {
		t.Errorf("AC = %d, want 16 flat", ac)
	}
}

func TestACUnarmoredDefenseBeatsWornArmor(t *testing.T) {
	// The case this was written for. K is a rogue/barbarian in a leather
	// jerkin: the leather comes to 15, but their own unarmored defence comes
	// to 18, and that is the number D&D Beyond shows, so it is the one the
	// sheet has to show too.
	ac, src := acCase(t,
		[]ClassLevel{{Class: "rogue", Level: 5}, {Class: "barbarian", Level: 5}},
		abilities(19, 18, 19, 15, 13, 10), worn("leather"))
	if ac != 18 {
		t.Errorf("AC = %d (%s), want 18", ac, src)
	}
	if !strings.Contains(strings.ToLower(src), "barbarian") {
		t.Errorf("AC source = %q, want the barbarian's unarmored defense", src)
	}
}

func TestACWornArmorStillWinsWhenItIsBetter(t *testing.T) {
	// The same barbarian in plate: the armour is the better deal and the
	// sheet should say so by name.
	ac, src := acCase(t, []ClassLevel{{Class: "barbarian", Level: 5}},
		abilities(19, 14, 16, 10, 10, 10), worn("plate"))
	if ac != 18 || src != "Plate" {
		t.Errorf("AC = %d (%s), want 18 Plate", ac, src)
	}
}

func TestACBarbarianKeepsShield(t *testing.T) {
	// "You can use a shield and still gain this benefit."
	ac, src := acCase(t, []ClassLevel{{Class: "barbarian", Level: 5}},
		abilities(16, 16, 16, 10, 10, 10), worn("shield"))
	if ac != 18 {
		t.Errorf("AC = %d (%s), want 10 + 3 + 3 + 2 = 18", ac, src)
	}
	if !strings.Contains(src, "Shield") {
		t.Errorf("AC source = %q, want the shield counted", src)
	}
}

func TestACMonkLosesShield(t *testing.T) {
	// A monk's defence asks for no shield, so a monk who picks one up gets
	// nothing from it: their own 10 + 3 + 3 = 16 still beats carrying it,
	// which as an ordinary character would be 10 + 3 + 2 = 15.
	ac, src := acCase(t, []ClassLevel{{Class: "monk", Level: 5}},
		abilities(12, 16, 12, 10, 16, 10), worn("shield"))
	if ac != 16 {
		t.Errorf("AC = %d (%s), want 16 with the shield doing nothing", ac, src)
	}
	if strings.Contains(src, "Shield") {
		t.Errorf("AC source = %q, the monk's defence does not allow a shield", src)
	}
}

func TestACShieldCountedBeforeComparing(t *testing.T) {
	// A monk in armour with a shield: their own defence is 10 + 2 + 1 = 13,
	// but scale mail and a shield come to 14 + 2 + 2 = 18, so the armour wins
	// - which it only can if the shield is counted before the two are
	// compared rather than added to whichever won.
	ac, src := acCase(t, []ClassLevel{{Class: "monk", Level: 5}},
		abilities(14, 14, 14, 10, 12, 10), worn("scale-mail"), worn("shield"))
	if ac != 18 {
		t.Errorf("AC = %d (%s), want 18", ac, src)
	}
	if !strings.Contains(src, "Shield") {
		t.Errorf("AC source = %q, want the shield counted", src)
	}
}

func TestACMulticlassTakesTheBestDefense(t *testing.T) {
	// Barbarian and monk at once: whichever of their two defences is higher.
	ac, src := acCase(t,
		[]ClassLevel{{Class: "barbarian", Level: 3}, {Class: "monk", Level: 3}},
		abilities(14, 16, 12, 10, 18, 10))
	if ac != 17 {
		t.Errorf("AC = %d (%s), want 17 from the monk's 10 + 3 + 4", ac, src)
	}
	if !strings.Contains(strings.ToLower(src), "monk") {
		t.Errorf("AC source = %q, want the monk's", src)
	}
}
