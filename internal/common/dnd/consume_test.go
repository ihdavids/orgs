package dnd

import (
	"strings"
	"testing"
)

// The effects here are read out of the item text the generator produces, so
// the wordings that must keep working are pinned against the real SRD rather
// than against a hand written fixture.
func TestItemUseFromSRD(t *testing.T) {
	rs := NewLibrary(nil).Get("srd")
	if rs == nil {
		t.Fatal("no srd ruleset")
	}

	// A potion whose strengths are printed as a table comes back as variants,
	// not as one set of dice: which one is being drunk is the player's to say.
	heal := ItemUse(rs.Item("potion-of-healing"))
	if heal.Verb != "Drink" {
		t.Errorf("potion of healing verb = %q, want Drink", heal.Verb)
	}
	if heal.Heal != "" {
		t.Errorf("potion of healing heal = %q, want the variants to carry it", heal.Heal)
	}
	want := []UseVariant{
		{Name: "Healing", Rarity: "Common", Heal: "2d4+2"},
		{Name: "Greater healing", Rarity: "Uncommon", Heal: "4d4+4"},
		{Name: "Superior healing", Rarity: "Rare", Heal: "8d4+8"},
		{Name: "Supreme healing", Rarity: "Very rare", Heal: "10d4+20"},
	}
	if len(heal.Variants) != len(want) {
		t.Fatalf("variants = %v", heal.Variants)
	}
	for i, v := range want {
		if heal.Variants[i] != v {
			t.Errorf("variant %d = %+v, want %+v", i, heal.Variants[i], v)
		}
	}
	if heal.Kind() != UseHeal || !heal.Has() {
		t.Errorf("kind = %q, has = %v", heal.Kind(), heal.Has())
	}
	if dice, ok := UseVariantHeal(heal, "superior healing"); !ok || dice != "8d4+8" {
		t.Errorf("superior healing = %q, %v", dice, ok)
	}
	if _, ok := UseVariantHeal(heal, "mediocre healing"); ok {
		t.Error("a strength the potion does not come in was accepted")
	}

	// Dice said in the prose are read straight off it.
	oint := ItemUse(rs.Item("restorative-ointment"))
	if oint.Heal != "2d8+2" {
		t.Errorf("restorative ointment heal = %q, want 2d8+2", oint.Heal)
	}

	// Temporary hit points are always a flat number in the rules.
	hero := ItemUse(rs.Item("potion-of-heroism"))
	if hero.Temp != 10 || hero.Heal != "" {
		t.Errorf("potion of heroism temp = %d, heal = %q", hero.Temp, hero.Heal)
	}
	if hero.Kind() != UseTemp {
		t.Errorf("potion of heroism kind = %q, want temp", hero.Kind())
	}

	// A potion that does something the sheet cannot apply says nothing, and a
	// sword is not drunk at all.
	if e := ItemUse(rs.Item("potion-of-climbing")); e.Has() {
		t.Errorf("potion of climbing = %+v, want no effect the sheet can apply", e)
	}
	if e := ItemUse(rs.Item("longsword")); e.Has() || e.Verb != "" {
		t.Errorf("longsword = %+v, want nothing", e)
	}

	// Healing that keeps happening while an item is worn is not a dose. Both
	// of these say they restore hit points, and neither has anything a Use
	// button could spend.
	for _, id := range []string{"ring-of-regeneration", "ioun-stone"} {
		if e := ItemUse(rs.Item(id)); e.Has() {
			t.Errorf("%s = %+v, want nothing: it heals while worn", id, e)
		}
	}

	// And nothing else in the whole SRD offers one, so a new hit here is a
	// change to be looked at rather than a free win.
	found := []string{}
	for i := range rs.Items {
		if ItemUse(&rs.Items[i]).Has() {
			found = append(found, rs.Items[i].Id)
		}
	}
	only := []string{"potion-of-healing", "potion-of-heroism", "restorative-ointment"}
	if strings.Join(found, ",") != strings.Join(only, ",") {
		t.Errorf("items with an effect = %v, want %v", found, only)
	}
}

// The wordings the parser is allowed to read, and the ones it must leave
// alone. An effect the text does not plainly state is never guessed at.
func TestItemUseWordings(t *testing.T) {
	cases := []struct {
		text string
		heal string
		flat int
		temp int
	}{
		{"You regain 2d4 + 2 hit points when you drink this potion.", "2d4+2", 0, 0},
		{"The creature that receives it regains 2d8+2 hit points.", "2d8+2", 0, 0},
		{"You regain up to 4d4+4 hit points.", "4d4+4", 0, 0},
		{"The creature regains 10 hit points.", "", 10, 0},
		{"For 1 hour after drinking it, you gain 10 temporary hit points.", "", 0, 10},
		{"You swallow the dose and regain 3d4 hit points, and gain 5 " +
			"temporary hit points.", "3d4", 0, 5},
		// Nothing about hit points at all, and hit points that belong to the
		// item rather than to whoever uses it.
		{"When you drink this potion, you become invisible for 1 hour.", "", 0, 0},
		{"The rope has AC 20 and 20 hit points. It regains 1 hit point every " +
			"5 minutes as long as it has at least 1 hit point.", "", 0, 0},
		// Healing that keeps coming is not a dose, however it is worded.
		{"You regain 1d6 hit points every 10 minutes.", "", 0, 0},
		{"You regain 15 hit points at the end of each hour this spindle " +
			"orbits your head.", "", 0, 0},
		{"While you wear this ring you regain 2d4 hit points.", "", 0, 0},
	}
	for _, c := range cases {
		e := ItemUse(&Item{Id: "x", Name: "Test Potion", Text: c.text})
		if e.Heal != c.heal || e.HealFlat != c.flat || e.Temp != c.temp {
			t.Errorf("%q\n  got  heal=%q flat=%d temp=%d\n  want heal=%q flat=%d temp=%d",
				c.text, e.Heal, e.HealFlat, e.Temp, c.heal, c.flat, c.temp)
		}
	}
}

// ----------------------------------------------------------------------------
// Using something up
// ----------------------------------------------------------------------------

func drinker() (*Character, *Ruleset) {
	rs := NewLibrary(nil).Get("srd")
	c := &Character{
		Name:      "Thirsty",
		Ruleset:   "srd",
		Abilities: map[string]int{"str": 10, "dex": 10, "con": 12, "int": 10, "wis": 10, "cha": 10},
		Classes:   []ClassLevel{{Class: "fighter", Level: 3}},
		HPMax:     28,
		HPCurrent: 10,
		Equipment: []Gear{
			{Id: "potion-of-healing", Name: "Potion of Healing", Qty: 2},
			{Id: "potion-of-heroism", Name: "Potion of Heroism", Qty: 1},
			{Id: "rope-hempen-50-feet", Name: "Rope, hempen (50 feet)", Qty: 1},
		},
	}
	return c, rs
}

// Drinking a healing potion is one call: the potion goes and the hit points
// move, with the dice the sheet threw.
func TestUseItemHeals(t *testing.T) {
	c, rs := drinker()
	out, err := UseItem(c, rs, InventoryRequest{
		Item: "potion-of-healing", Qty: 1, Effect: UseHeal, Amount: 7, Variant: "Healing",
	})
	if err != nil {
		t.Fatal(err)
	}
	if c.HPCurrent != 17 {
		t.Errorf("hp = %d, want 17", c.HPCurrent)
	}
	if out.Health == nil || out.Applied != UseHeal || out.Amount != 7 {
		t.Errorf("result = %+v", out)
	}
	// One left of the two, and both histories say what happened.
	if n := countGear(c, "potion-of-healing"); n != 1 {
		t.Errorf("potions left = %d, want 1", n)
	}
	if len(c.InventoryLog) != 1 || c.InventoryLog[0].Action != InvUsed {
		t.Errorf("inventory log = %+v", c.InventoryLog)
	}
	if len(c.HealthLog) != 1 || c.HealthLog[0].Notes == "" {
		t.Errorf("health log = %+v", c.HealthLog)
	}
}

func TestUseItemTempHitPoints(t *testing.T) {
	c, rs := drinker()
	if _, err := UseItem(c, rs, InventoryRequest{
		Item: "potion-of-heroism", Qty: 1, Effect: UseTemp, Amount: 10,
	}); err != nil {
		t.Fatal(err)
	}
	if c.HPTemp != 10 {
		t.Errorf("temp = %d, want 10", c.HPTemp)
	}
	if c.HPCurrent != 10 {
		t.Errorf("real hit points moved to %d", c.HPCurrent)
	}
}

// Using something up with no effect to apply is what it always was.
func TestUseItemPlain(t *testing.T) {
	c, rs := drinker()
	out, err := UseItem(c, rs, InventoryRequest{Item: "rope-hempen-50-feet", Qty: 1})
	if err != nil {
		t.Fatal(err)
	}
	if out.Health != nil || out.Applied != "" {
		t.Errorf("result = %+v", out)
	}
	if c.HPCurrent != 10 {
		t.Errorf("hp moved to %d", c.HPCurrent)
	}
}

// A page that has been left open must not be able to heal out of a rope, drink
// a strength the potion does not come in, or heal with the wrong effect.
func TestUseItemRefusesWhatTheItemDoesNotDo(t *testing.T) {
	c, rs := drinker()
	bad := []InventoryRequest{
		{Item: "rope-hempen-50-feet", Qty: 1, Effect: UseHeal, Amount: 20},
		{Item: "potion-of-healing", Qty: 1, Effect: UseTemp, Amount: 20},
		{Item: "potion-of-healing", Qty: 1, Effect: UseHeal, Amount: 20, Variant: "Mediocre"},
	}
	for _, req := range bad {
		if _, err := UseItem(c, rs, req); err == nil {
			t.Errorf("%+v was allowed", req)
		}
	}
	// Nothing was drunk and nobody was healed.
	if c.HPCurrent != 10 || c.HPTemp != 0 {
		t.Errorf("hp %d, temp %d", c.HPCurrent, c.HPTemp)
	}
	if n := countGear(c, "potion-of-healing"); n != 2 {
		t.Errorf("potions = %d, want 2", n)
	}
}

// A potion that is not there heals nobody.
func TestUseItemNotCarried(t *testing.T) {
	c, rs := drinker()
	if _, err := UseItem(c, rs, InventoryRequest{
		Item: "restorative-ointment", Qty: 1, Effect: UseHeal, Amount: 9,
	}); err == nil {
		t.Fatal("used an ointment nobody had")
	}
	if c.HPCurrent != 10 {
		t.Errorf("hp = %d", c.HPCurrent)
	}
}

// Healing at full hit points is refused by the hit points, and the potion is
// still gone: it was drunk. The sheet is told both halves.
func TestUseItemAtFullHealth(t *testing.T) {
	c, rs := drinker()
	c.HPCurrent = c.HPMax
	out, err := UseItem(c, rs, InventoryRequest{
		Item: "potion-of-healing", Qty: 1, Effect: UseHeal, Amount: 7, Variant: "Healing",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Health != nil {
		t.Error("healed someone who was already full")
	}
	if n := countGear(c, "potion-of-healing"); n != 1 {
		t.Errorf("potions = %d, want the drunk one gone", n)
	}
	if !strings.Contains(out.Msg, "but") {
		t.Errorf("message = %q, want it to say the healing went nowhere", out.Msg)
	}
}
