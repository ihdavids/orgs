// Tests for what a condition does to a d20, and for what a character's own
// defenses do to a blow before it lands.
package dnd

import (
	"strings"
	"testing"
)

func under(t *testing.T, c *Character, rs *Ruleset, id string, level int) {
	t.Helper()
	if _, err := ApplyConditionChange(c, rs, ConditionsRequest{
		Action: "add", Name: id, Level: level,
	}); err != nil {
		t.Fatalf("add %s: %v", id, err)
	}
}

func TestPoisonedTakesTheAttackAndTheCheckDown(t *testing.T) {
	rs := srd(t)
	c := testChar()
	under(t, c, rs, "poisoned", 0)
	a := ComputeRollAdvice(c, rs)
	if !a.Any {
		t.Fatalf("being poisoned should be worth saying")
	}
	if a.Attack.Lean != LeanDisadvantage {
		t.Fatalf("want disadvantage on attacks, got %q", a.Attack.Lean)
	}
	if a.Check[DEX].Lean != LeanDisadvantage {
		t.Fatalf("want disadvantage on checks, got %q", a.Check[DEX].Lean)
	}
	// Saves are not ability checks, and poison says nothing about them.
	if a.Save[CON].Lean != "" {
		t.Fatalf("poison should not touch saving throws, got %q", a.Save[CON].Lean)
	}
	if !strings.Contains(a.Attack.Why, "Poisoned") {
		t.Fatalf("the card should say why: %q", a.Attack.Why)
	}
	if a.Attack.Short != "Poisoned" {
		t.Fatalf("the mark should say Poisoned, got %q", a.Attack.Short)
	}
}

func TestRestrainedOnlyTouchesDexteritySaves(t *testing.T) {
	rs := srd(t)
	c := testChar()
	under(t, c, rs, "restrained", 0)
	a := ComputeRollAdvice(c, rs)
	if a.Save[DEX].Lean != LeanDisadvantage {
		t.Fatalf("want disadvantage on Dexterity saves, got %q", a.Save[DEX].Lean)
	}
	for _, ab := range []string{STR, CON, INT, WIS, CHA} {
		if a.Save[ab].Lean != "" {
			t.Fatalf("restrained should leave %s saves alone, got %q", ab, a.Save[ab].Lean)
		}
	}
	if a.Attack.Lean != LeanDisadvantage {
		t.Fatalf("restrained should still take the attack down")
	}
}

func TestExhaustionBitesHarderAsItDeepens(t *testing.T) {
	rs := srd(t)
	c := testChar()
	under(t, c, rs, "exhaustion", 1)
	a := ComputeRollAdvice(c, rs)
	if a.Check[STR].Lean != LeanDisadvantage {
		t.Fatalf("level 1 should take the checks down")
	}
	if a.Attack.Lean != "" || a.Save[STR].Lean != "" {
		t.Fatalf("level 1 should leave attacks and saves alone")
	}
	under(t, c, rs, "exhaustion", 3)
	a = ComputeRollAdvice(c, rs)
	if a.Attack.Lean != LeanDisadvantage || a.Save[WIS].Lean != LeanDisadvantage {
		t.Fatalf("level 3 should take attacks and saves down too")
	}
	// Cumulative: the level 1 line is still in force at level 3.
	if a.Check[STR].Lean != LeanDisadvantage {
		t.Fatalf("level 3 should still have the level 1 effect")
	}
	if !strings.Contains(a.Attack.Short, "Exhaustion 3") {
		t.Fatalf("the mark should carry the level, got %q", a.Attack.Short)
	}
}

func TestAdvantageAndDisadvantageCancel(t *testing.T) {
	rs := srd(t)
	c := testChar()
	under(t, c, rs, "invisible", 0)
	under(t, c, rs, "poisoned", 0)
	a := ComputeRollAdvice(c, rs)
	if a.Attack.Lean != "" {
		t.Fatalf("one of each is neither, got %q", a.Attack.Lean)
	}
	if !a.Attack.Cancels {
		t.Fatalf("the card has to be able to say they cancelled")
	}
	if !strings.Contains(a.Attack.Why, "cancel") {
		t.Fatalf("and say so in words: %q", a.Attack.Why)
	}
}

func TestBeingHelplessFailsStrengthAndDexteritySaves(t *testing.T) {
	rs := srd(t)
	c := testChar()
	under(t, c, rs, "paralyzed", 0)
	a := ComputeRollAdvice(c, rs)
	if !a.Save[STR].Fails || !a.Save[DEX].Fails {
		t.Fatalf("paralyzed fails Strength and Dexterity saves")
	}
	if a.Save[WIS].Fails {
		t.Fatalf("it says nothing about Wisdom saves")
	}
	if !strings.Contains(a.Save[STR].Why, "Fails automatically") {
		t.Fatalf("the card should say so: %q", a.Save[STR].Why)
	}
}

func TestACleanCharacterIsToldNothing(t *testing.T) {
	rs := srd(t)
	c := testChar()
	a := ComputeRollAdvice(c, rs)
	if a.Any || a.Attack.Any() || a.Attack.Why != "" {
		t.Fatalf("nothing wrong with you is nothing to say, got %+v", a.Attack)
	}
}

// ---- and what the character shrugs off ---------------------------------

func TestResistanceHalvesTheBlowBeforeTheTemporaryHitPoints(t *testing.T) {
	rs := srd(t)
	c := testChar()
	c.Resistances = []string{"fire"}
	c.HPTemp = 4
	before := Compute(c, rs).HPCurrent
	e, err := ApplyHealth(c, rs, HealthRequest{Action: Hurt, Amount: 13, Type: "fire"})
	if err != nil {
		t.Fatalf("hurt: %v", err)
	}
	// Thirteen halves to six, four of which the buffer eats.
	if e.Amount != 6 || e.Raw != 13 || e.Defense != Resistance {
		t.Fatalf("want 6 of 13 resisted, got amount %d raw %d defense %q",
			e.Amount, e.Raw, e.Defense)
	}
	if e.Absorbed != 4 || c.HPTemp != 0 {
		t.Fatalf("the buffer should have eaten 4, got %d leaving %d", e.Absorbed, c.HPTemp)
	}
	if c.HPCurrent != before-2 {
		t.Fatalf("want %d hit points left, got %d", before-2, c.HPCurrent)
	}
	if !strings.Contains(HealthEventMsg(e), "resisted from 13") {
		t.Fatalf("the line should say it was resisted: %q", HealthEventMsg(e))
	}
}

func TestImmunityStopsItDead(t *testing.T) {
	rs := srd(t)
	c := testChar()
	c.Immunities = []string{"Poison"}
	before := Compute(c, rs).HPCurrent
	e, err := ApplyHealth(c, rs, HealthRequest{Action: Hurt, Amount: 20, Type: "poison"})
	if err != nil {
		t.Fatalf("hurt: %v", err)
	}
	if c.HPCurrent != before {
		t.Fatalf("immunity should leave the hit points alone, got %d", c.HPCurrent)
	}
	if e.Defense != Immunity || e.Raw != 20 {
		t.Fatalf("the history should still record the blow, got %+v", e)
	}
	if !strings.Contains(HealthEventMsg(e), "immune") {
		t.Fatalf("and say so: %q", HealthEventMsg(e))
	}
}

func TestVulnerabilityDoublesAndUntypedDamageResistsNothing(t *testing.T) {
	rs := srd(t)
	c := testChar()
	c.Vulnerabilities = []string{"cold"}
	c.Resistances = []string{"fire"}
	before := Compute(c, rs).HPCurrent
	if _, err := ApplyHealth(c, rs, HealthRequest{Action: Hurt, Amount: 5, Type: "cold"}); err != nil {
		t.Fatalf("hurt: %v", err)
	}
	if c.HPCurrent != before-10 {
		t.Fatalf("want 10 taken, got %d", before-c.HPCurrent)
	}
	// No type at all is what most of the damage on a sheet is, and it
	// resists nothing even where the character has resistances.
	at := c.HPCurrent
	if _, err := ApplyHealth(c, rs, HealthRequest{Action: Hurt, Amount: 6}); err != nil {
		t.Fatalf("hurt: %v", err)
	}
	if c.HPCurrent != at-6 {
		t.Fatalf("untyped damage should land in full, got %d", at-c.HPCurrent)
	}
}

func TestDamageTypeSurvivesTheRoundTripThroughTheOrgFile(t *testing.T) {
	rs := srd(t)
	c := testChar()
	c.Resistances = []string{"fire"}
	if _, err := ApplyHealth(c, rs, HealthRequest{Action: Hurt, Amount: 11, Type: "fire"}); err != nil {
		t.Fatalf("hurt: %v", err)
	}
	back, err := ParseOrg(RenderOrg(c, rs), rs)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(back.HealthLog) != 1 {
		t.Fatalf("want one line back, got %d", len(back.HealthLog))
	}
	e := back.HealthLog[0]
	if e.Type != "Fire" || e.Defense != Resistance || e.Raw != 11 || e.Amount != 5 {
		t.Fatalf("the line did not survive the round trip: %+v", e)
	}
}
