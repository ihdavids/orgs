package dnd

import (
	"strings"
	"testing"
)

func concentrator() (*Character, *Ruleset) {
	rs := NewLibrary(nil).Get("srd")
	c := &Character{
		Name:      "Holder",
		Ruleset:   "srd",
		Abilities: map[string]int{"str": 10, "dex": 12, "con": 14, "int": 16, "wis": 10, "cha": 10},
		Classes:   []ClassLevel{{Class: "wizard", Level: 5}},
		HPMax:     28,
		HPCurrent: 28,
	}
	return c, rs
}

func TestConcentrationProperty(t *testing.T) {
	// The property round trips, with and without the slot level.
	for _, tc := range []struct {
		val   string
		spell string
		level int
	}{
		{"hex", "hex", 0},
		{"hex @2", "hex", 2},
		{"Hold Person @3", "Hold Person", 3},
		{"hex@9", "hex", 9},
	} {
		got := ParseConcentration(tc.val)
		if got == nil || got.Spell != tc.spell || got.Level != tc.level {
			t.Errorf("%q = %+v, want %s @%d", tc.val, got, tc.spell, tc.level)
			continue
		}
		if back := ConcentrationProp(got); ParseConcentration(back) == nil {
			t.Errorf("%q wrote back as %q, which does not read", tc.val, back)
		}
	}
	// Nothing at all, said several ways.
	for _, val := range []string{"", "  ", "none"} {
		if got := ParseConcentration(val); got != nil {
			t.Errorf("%q = %+v, want nothing", val, got)
		}
	}
	if ConcentrationProp(nil) != "" {
		t.Error("nothing held wrote something")
	}
	// A level nobody has is dropped rather than kept: the spell is still held.
	if got := ParseConcentration("hex @99"); got == nil || got.Level != 0 {
		t.Errorf("hex @99 = %+v", got)
	}
}

func TestConcentrationDCIsTenOrHalf(t *testing.T) {
	for _, tc := range []struct{ damage, dc int }{
		{1, 10}, {9, 10}, {20, 10}, {21, 10}, {22, 11}, {45, 22}, {0, 10},
	} {
		if got := ConcentrationDC(tc.damage); got != tc.dc {
			t.Errorf("%d damage = DC %d, want %d", tc.damage, got, tc.dc)
		}
	}
}

// Only a spell that asks for concentration can be concentrated on, and a second
// one displaces the first rather than being refused.
func TestStartConcentrationHoldsOneSpell(t *testing.T) {
	c, rs := concentrator()
	if _, err := StartConcentration(c, rs, "haste", 3); err != nil {
		t.Fatal(err)
	}
	if c.Concentration == nil || c.Concentration.Spell != "haste" || c.Concentration.Level != 3 {
		t.Fatalf("holding %+v", c.Concentration)
	}
	msg, err := StartConcentration(c, rs, "fly", 3)
	if err != nil {
		t.Fatal(err)
	}
	if c.Concentration.Spell != "fly" {
		t.Errorf("holding %+v, want fly", c.Concentration)
	}
	if msg == "" || !strings.Contains(msg, "Haste") {
		t.Errorf("message = %q, want it to say what was let go of", msg)
	}
	if _, err := StartConcentration(c, rs, "fireball", 3); err == nil {
		t.Error("concentrated on a fireball")
	}
	if c.Concentration.Spell != "fly" {
		t.Errorf("the refused spell displaced %+v anyway", c.Concentration)
	}
}

func TestDropConcentration(t *testing.T) {
	c, rs := concentrator()
	if _, err := StartConcentration(c, rs, "haste", 3); err != nil {
		t.Fatal(err)
	}
	if msg := DropConcentration(c, rs); !strings.Contains(msg, "Haste") {
		t.Errorf("message = %q", msg)
	}
	if c.Concentration != nil {
		t.Error("still holding something")
	}
	// Letting go of nothing is not an error: a reloaded sheet and a sheet that
	// never had a spell up look the same, and both can press the button.
	if msg := DropConcentration(c, rs); msg == "" {
		t.Error("no answer at all")
	}
}

// Taking damage asks for the save; making it keeps the spell and missing it
// does not.
func TestConcentrationSaveOnDamage(t *testing.T) {
	c, rs := concentrator()
	if _, err := StartConcentration(c, rs, "haste", 3); err != nil {
		t.Fatal(err)
	}
	e, err := ApplyHealth(c, rs, HealthRequest{Action: Hurt, Amount: 7})
	if err != nil {
		t.Fatal(err)
	}
	save := ConcentrationSaveFor(c, rs, Compute(c, rs), e.Amount)
	if save == nil {
		t.Fatal("no save called for")
	}
	if save.DC != 10 || save.Name != "Haste" {
		t.Errorf("save = %+v", save)
	}
	// Constitution 14 with no proficiency is +2.
	if save.Mod != 2 || save.ModStr != "+2" {
		t.Errorf("modifier = %d (%q)", save.Mod, save.ModStr)
	}

	kept, msg, err := ApplyConcentrationSave(c, rs, save.DC, 12)
	if err != nil || !kept || c.Concentration == nil {
		t.Errorf("made the save but lost the spell: kept=%v err=%v msg=%q", kept, err, msg)
	}
	kept, _, err = ApplyConcentrationSave(c, rs, save.DC, 9)
	if err != nil {
		t.Fatal(err)
	}
	if kept || c.Concentration != nil {
		t.Error("missed the save and kept the spell")
	}
	// With nothing held there is nothing to save for.
	if _, _, err := ApplyConcentrationSave(c, rs, 10, 20); err == nil {
		t.Error("saved for a spell that was not up")
	}
}

// A big hit asks for more than DC 10, and a hit soaked up by temporary hit
// points still asks: you took the damage either way.
func TestConcentrationSaveScalesAndCountsSoakedDamage(t *testing.T) {
	c, rs := concentrator()
	c.HPTemp = 30
	if _, err := StartConcentration(c, rs, "haste", 3); err != nil {
		t.Fatal(err)
	}
	e, err := ApplyHealth(c, rs, HealthRequest{Action: Hurt, Amount: 26})
	if err != nil {
		t.Fatal(err)
	}
	if e.Absorbed != 26 || e.HPAfter != c.HPMax {
		t.Fatalf("event = %+v", e)
	}
	save := ConcentrationSaveFor(c, rs, Compute(c, rs), e.Amount)
	if save == nil || save.DC != 13 {
		t.Errorf("save = %+v, want DC 13", save)
	}
}

// Being knocked out ends it with no save at all, and so does a night's sleep.
func TestConcentrationBreaksWhenYouGoDown(t *testing.T) {
	c, rs := concentrator()
	if _, err := StartConcentration(c, rs, "haste", 3); err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyHealth(c, rs, HealthRequest{Action: Hurt, Amount: 40}); err != nil {
		t.Fatal(err)
	}
	if c.HPCurrent != 0 {
		t.Fatalf("hp = %d", c.HPCurrent)
	}
	if c.Concentration != nil {
		t.Error("an unconscious caster is still holding a spell")
	}

	c.HPCurrent = c.HPMax
	if _, err := StartConcentration(c, rs, "haste", 3); err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyRest(c, rs, RestRequest{Kind: LongRest}); err != nil {
		t.Fatal(err)
	}
	if c.Concentration != nil {
		t.Error("a long rest did not end it")
	}
}

func TestConcentrationView(t *testing.T) {
	c, rs := concentrator()
	if v := ComputeConcentration(Compute(c, rs), rs); v.On {
		t.Errorf("nothing held reads as %+v", v)
	}
	if _, err := StartConcentration(c, rs, "haste", 3); err != nil {
		t.Fatal(err)
	}
	v := ComputeConcentration(Compute(c, rs), rs)
	if !v.On || v.Name != "Haste" || v.Level != 3 {
		t.Errorf("view = %+v", v)
	}
	if v.Label != "Haste (3rd level)" {
		t.Errorf("label = %q", v.Label)
	}
	if v.SaveStr != "+2" || v.Duration == "" {
		t.Errorf("save %q, duration %q", v.SaveStr, v.Duration)
	}
}
