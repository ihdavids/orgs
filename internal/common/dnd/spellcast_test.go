// Tests for what casting a spell works out to: the attack, the save the
// target makes, damage grown to level, healing, and the spells that roll
// nothing at all.
package dnd

import (
	"strings"
	"testing"
)

func castOf(t *testing.T, rs *Ruleset, id string, level, atk, dc, mod int) SpellCast {
	t.Helper()
	sp := rs.Spell(id)
	if sp == nil {
		t.Fatalf("no spell %q in the srd", id)
	}
	return ComputeCast(sp, level, atk, dc, mod)
}

func TestCastAttackSpellScalesWithLevel(t *testing.T) {
	rs := srd(t)
	c := castOf(t, rs, "fire-bolt", 1, 7, 15, 4)
	if !c.Attack || c.Kind != "ranged" {
		t.Fatalf("fire bolt is a ranged spell attack, got %+v", c)
	}
	if c.AttackBonus != 7 || c.Damage != "1d10" || c.DamageType != "fire" {
		t.Fatalf("bad cast at level 1: %+v", c)
	}
	if !c.Rolls {
		t.Fatal("fire bolt rolls dice")
	}
	for _, tc := range []struct {
		level int
		want  string
	}{{4, "1d10"}, {5, "2d10"}, {10, "2d10"}, {11, "3d10"}, {17, "4d10"}, {20, "4d10"}} {
		got := castOf(t, rs, "fire-bolt", tc.level, 7, 15, 4).Damage
		if got != tc.want {
			t.Fatalf("fire bolt at level %d: want %s, got %s", tc.level, tc.want, got)
		}
	}
}

func TestCastSaveSpellSaysWhatTheTargetRolls(t *testing.T) {
	rs := srd(t)
	c := castOf(t, rs, "fireball", 5, 7, 15, 4)
	if c.Attack {
		t.Fatal("fireball has no attack roll")
	}
	if c.Save != "dex" || c.SaveName != "Dexterity" || c.SaveDC != 15 {
		t.Fatalf("bad save: %+v", c)
	}
	if c.SaveFor != "half as much damage on a success" {
		t.Fatalf("fireball is half on a save, got %q", c.SaveFor)
	}
	if c.Damage != "8d6" || c.DamageType != "fire" {
		t.Fatalf("bad damage: %+v", c)
	}
	want := "Dexterity saving throw against DC 15, half as much damage on a success"
	if c.Line != want {
		t.Fatalf("want line %q, got %q", want, c.Line)
	}
}

func TestCastHealingPicksUpTheAbilityModifier(t *testing.T) {
	rs := srd(t)
	c := castOf(t, rs, "cure-wounds", 5, 7, 15, 4)
	if c.Heal != "1d8 +4" {
		t.Fatalf("want cure wounds to heal 1d8 +4, got %q", c.Heal)
	}
	if !c.Rolls {
		t.Fatal("healing is something to roll")
	}
}

func TestCastKeepsAFlatDamageBonus(t *testing.T) {
	rs := srd(t)
	c := castOf(t, rs, "magic-missile", 5, 7, 15, 4)
	if c.Damage != "1d4 + 1" {
		t.Fatalf("magic missile does 1d4 + 1, got %q", c.Damage)
	}
}

func TestCastWithNothingToRollStillSaysSomething(t *testing.T) {
	rs := srd(t)
	c := castOf(t, rs, "shield", 5, 7, 15, 4)
	if c.Rolls {
		t.Fatalf("shield rolls nothing, got %+v", c)
	}
	if c.Line == "" || c.Line == "No roll" {
		t.Fatalf("shield should still describe itself, got %q", c.Line)
	}
}

func TestCastReadsASaveOutOfTheSpellTextWhenTheRulesetIsSilent(t *testing.T) {
	rs := srd(t)
	// Neither of these carries a save: field in the srd, both say so in
	// their own text.
	c := castOf(t, rs, "acid-splash", 5, 7, 15, 4)
	if c.Save != "dex" || c.SaveDC != 15 {
		t.Fatalf("acid splash is a dexterity save: %+v", c)
	}
	if c.SaveFor != "no damage on a success" {
		t.Fatalf("acid splash does nothing on a save, got %q", c.SaveFor)
	}
	if c.Damage != "2d6" {
		t.Fatalf("acid splash does 2d6 at level 5, got %q", c.Damage)
	}
	g := castOf(t, rs, "hideous-laughter", 5, 7, 15, 4)
	if g.Save != "wis" {
		t.Fatalf("hideous laughter is a wisdom save: %+v", g)
	}
	if g.Rolls {
		t.Fatal("hideous laughter rolls nothing of its own")
	}
	if g.Line != "Wisdom saving throw against DC 15" {
		t.Fatalf("want the save read out, got %q", g.Line)
	}
}

func TestCastThatRollsItsOwnDiceHasNoLineToReadOut(t *testing.T) {
	rs := srd(t)
	if c := castOf(t, rs, "fire-bolt", 5, 7, 15, 4); c.Line != "" {
		t.Fatalf("an attack spell asks the target for nothing, got %q", c.Line)
	}
	if c := castOf(t, rs, "cure-wounds", 5, 7, 15, 4); c.Line != "" {
		t.Fatalf("healing asks the target for nothing, got %q", c.Line)
	}
}

// hellfire is not an SRD spell - it is the community cantrip the third party
// module carries - and it is the one that says its step once for two damage
// lines, so it is written out here rather than looked up.
func hellfire() *Spell {
	return &Spell{
		Id: "hellfire", Name: "Hellfire", Level: 0, School: "evocation",
		Range: "60 feet", Save: "cha",
		Damage: "1d4 fire", Damage2: "1d4 necrotic", Damage2Label: "Necrotic",
		Text: "You call up a gout of infernal fire around a creature you can " +
			"see within range. The target must succeed on a Charisma saving " +
			"throw or take 1d4 fire damage and 1d4 necrotic damage.\n\n" +
			"Both of this spell's damage types increase by 1d4 when you reach " +
			"5th level (2d4 each), 11th level (3d4 each) and 17th level " +
			"(4d4 each).",
	}
}

func TestCantripTiersAreTheLevelsReached(t *testing.T) {
	sp := hellfire()
	for _, tc := range []struct {
		level int
		want  []string
		dice  []string
	}{
		// Below the first step there is one tier, which is no progression to
		// show, so the sheet gets no picker at all.
		{4, nil, nil},
		{5, []string{"1st", "5th"}, []string{"1d4", "2d4"}},
		{12, []string{"1st", "5th", "11th"}, []string{"1d4", "2d4", "3d4"}},
		{17, []string{"1st", "5th", "11th", "17th"},
			[]string{"1d4", "2d4", "3d4", "4d4"}},
	} {
		c := ComputeCast(sp, tc.level, 7, 15, 3)
		if len(c.Tiers) != len(tc.want) {
			t.Fatalf("hellfire at level %d: want %d tiers, got %d (%+v)",
				tc.level, len(tc.want), len(c.Tiers), c.Tiers)
		}
		for i, tier := range c.Tiers {
			if tier.Label != tc.want[i] {
				t.Fatalf("hellfire at level %d tier %d: want %s, got %s",
					tc.level, i, tc.want[i], tier.Label)
			}
			// Both of hellfire's damage types grow together, and each tier
			// counts from the dice the cantrip started with.
			if tier.Damage != tc.dice[i] || tier.Damage2 != tc.dice[i] {
				t.Fatalf("hellfire at level %d tier %s: want %s each, got %s and %s",
					tc.level, tier.Label, tc.dice[i], tier.Damage, tier.Damage2)
			}
			if want := i == len(c.Tiers)-1; tier.Current != want {
				t.Fatalf("hellfire at level %d: tier %s current=%v",
					tc.level, tier.Label, tier.Current)
			}
		}
		if len(c.Tiers) > 0 {
			last := c.Tiers[len(c.Tiers)-1]
			// The tier this character is at is what the cast button rolls.
			if last.Damage != c.Damage || last.Damage2 != c.Damage2 {
				t.Fatalf("hellfire at level %d: the last tier is not the cast: %+v vs %s/%s",
					tc.level, last, c.Damage, c.Damage2)
			}
			if c.TierLevel != last.Level {
				t.Fatalf("hellfire at level %d: tierLevel %d, last tier %d",
					tc.level, c.TierLevel, last.Level)
			}
		}
	}
}

func TestCantripTierNotesSayTheDice(t *testing.T) {
	c := ComputeCast(hellfire(), 11, 7, 15, 3)
	want := []string{
		"1d4 fire + 1d4 necrotic",
		"2d4 fire + 2d4 necrotic",
		"3d4 fire + 3d4 necrotic",
	}
	for i, tier := range c.Tiers {
		if tier.Note != want[i] {
			t.Fatalf("tier %s note: want %q, got %q", tier.Label, want[i], tier.Note)
		}
	}
	// A tier below the one this character is at says so in the roll history,
	// where it would otherwise read as the cantrip they actually cast.
	if !strings.HasPrefix(c.Tiers[0].Detail, "at 1st level · ") {
		t.Fatalf("a lower tier should say which level it is: %q", c.Tiers[0].Detail)
	}
	if strings.HasPrefix(c.Tiers[2].Detail, "at ") {
		t.Fatalf("the current tier is just the cast: %q", c.Tiers[2].Detail)
	}
}

func TestCantripTiersFromTheSrd(t *testing.T) {
	rs := srd(t)
	c := castOf(t, rs, "fire-bolt", 11, 7, 15, 4)
	want := []struct {
		label  string
		damage string
	}{{"1st", "1d10"}, {"5th", "2d10"}, {"11th", "3d10"}}
	if len(c.Tiers) != len(want) {
		t.Fatalf("fire bolt at 11th: want %d tiers, got %+v", len(want), c.Tiers)
	}
	for i, tier := range c.Tiers {
		if tier.Label != want[i].label || tier.Damage != want[i].damage {
			t.Fatalf("fire bolt tier %d: want %s/%s, got %s/%s",
				i, want[i].label, want[i].damage, tier.Label, tier.Damage)
		}
		if tier.Note != want[i].damage+" fire" {
			t.Fatalf("fire bolt tier %s note: %q", tier.Label, tier.Note)
		}
	}
	// A levelled spell grows by the slot it is cast from and not by the
	// caster's level, so it has no tiers at all.
	if c := castOf(t, rs, "guiding-bolt", 11, 7, 15, 4); len(c.Tiers) != 0 {
		t.Fatalf("guiding bolt is not a cantrip: %+v", c.Tiers)
	}
}

// Eldritch blast grows by throwing more beams, not by rolling bigger dice,
// and its damage is a paragraph above the one that says so. Reading the two
// as one sentence used to make it a 3d10 cantrip at 11th level.
func TestCantripThatGrowsBeamsRollsTheSameDice(t *testing.T) {
	rs := srd(t)
	for _, level := range []int{1, 5, 11, 17} {
		c := castOf(t, rs, "eldritch-blast", level, 7, 15, 4)
		if c.Damage != "1d10" {
			t.Fatalf("eldritch blast at level %d rolls %s", level, c.Damage)
		}
		if len(c.Tiers) != 0 {
			t.Fatalf("eldritch blast at level %d offers tiers: %+v", level, c.Tiers)
		}
	}
}
