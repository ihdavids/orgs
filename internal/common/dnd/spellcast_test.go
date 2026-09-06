// Tests for what casting a spell works out to: the attack, the save the
// target makes, damage grown to level, healing, and the spells that roll
// nothing at all.
package dnd

import "testing"

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
