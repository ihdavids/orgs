// Tests for the conditions a character is under, the defenses they shrug off,
// and the hit points in between - including that all three survive a trip out
// to an org character sheet and back, which is where they actually live.
package dnd

import (
	"strings"
	"testing"
)

func sufferer() *Character {
	return &Character{
		Name:      "Test",
		Race:      "human",
		Classes:   []ClassLevel{{Class: "fighter", Level: 3}},
		Abilities: map[string]int{"str": 16, "dex": 12, "con": 14, "int": 10, "wis": 10, "cha": 8},
	}
}

func change(t *testing.T, c *Character, rs *Ruleset, req ConditionsRequest) ConditionEvent {
	t.Helper()
	e, err := ApplyConditionChange(c, rs, req)
	if err != nil {
		t.Fatalf("%s %q: %s", req.Action, req.Name, err)
	}
	return e
}

func TestConditionToggle(t *testing.T) {
	rs := srd(t)
	c := sufferer()
	// The picker sends toggle and nothing else, so one tap has to do both.
	e := change(t, c, rs, ConditionsRequest{Action: CondToggle, Name: "frightened"})
	if e.Action != "gained" || e.Name != "Frightened" {
		t.Errorf("first toggle = %+v", e)
	}
	if !c.HasCondition("frightened") {
		t.Errorf("not frightened after the first toggle")
	}
	e = change(t, c, rs, ConditionsRequest{Action: CondToggle, Name: "frightened"})
	if e.Action != "ended" {
		t.Errorf("second toggle = %+v", e)
	}
	if c.HasCondition("frightened") {
		t.Errorf("still frightened after the second toggle")
	}
	if len(c.ConditionLog) != 2 {
		t.Errorf("history has %d lines, want 2", len(c.ConditionLog))
	}
}

func TestConditionByNameOrId(t *testing.T) {
	rs := srd(t)
	for _, name := range []string{"poisoned", "Poisoned", "POISONED"} {
		c := sufferer()
		change(t, c, rs, ConditionsRequest{Action: CondAdd, Name: name})
		if !c.HasCondition("poisoned") {
			t.Errorf("%q did not take", name)
		}
	}
}

func TestConditionAddedTwiceIsRefused(t *testing.T) {
	rs := srd(t)
	c := sufferer()
	change(t, c, rs, ConditionsRequest{Action: CondAdd, Name: "prone"})
	if _, err := ApplyConditionChange(c, rs, ConditionsRequest{
		Action: CondAdd, Name: "prone"}); err == nil {
		t.Errorf("being knocked prone twice was allowed")
	}
	if len(c.Conditions) != 1 {
		t.Errorf("prone is on the sheet %d times", len(c.Conditions))
	}
}

func TestExhaustionHasLevels(t *testing.T) {
	rs := srd(t)
	c := sufferer()
	// Exhaustion arrives at level 1 whether or not one was asked for.
	change(t, c, rs, ConditionsRequest{Action: CondAdd, Name: "exhaustion"})
	if got := c.ConditionLevel("exhaustion"); got != 1 {
		t.Errorf("exhaustion arrived at %d, want 1", got)
	}
	e := change(t, c, rs, ConditionsRequest{Action: CondLevel, Name: "exhaustion", Level: 3})
	if e.Action != "worsened" || e.Level != 3 {
		t.Errorf("worsening = %+v", e)
	}
	e = change(t, c, rs, ConditionsRequest{Action: CondLevel, Name: "exhaustion", Level: 1})
	if e.Action != "eased" {
		t.Errorf("easing = %+v", e)
	}
	// Level 0 is how it is shaken off entirely.
	e = change(t, c, rs, ConditionsRequest{Action: CondLevel, Name: "exhaustion", Level: 0})
	if e.Action != "ended" || c.HasCondition("exhaustion") {
		t.Errorf("level 0 = %+v, still on: %v", e, c.HasCondition("exhaustion"))
	}
	// And it cannot be driven past the sixth level, which is death.
	change(t, c, rs, ConditionsRequest{Action: CondLevel, Name: "exhaustion", Level: 99})
	if got := c.ConditionLevel("exhaustion"); got != 6 {
		t.Errorf("exhaustion 99 came to %d, want 6", got)
	}
}

func TestConditionViewLabelsTheLevel(t *testing.T) {
	rs := srd(t)
	c := sufferer()
	change(t, c, rs, ConditionsRequest{Action: CondAdd, Name: "exhaustion", Level: 2})
	change(t, c, rs, ConditionsRequest{Action: CondAdd, Name: "frightened"})
	v := ComputeConditions(c, rs)
	if v.Count != 2 {
		t.Fatalf("%d conditions active, want 2", v.Count)
	}
	if len(v.All) != len(rs.ConditionList()) {
		t.Errorf("the picker offers %d conditions, the ruleset has %d",
			len(v.All), len(rs.ConditionList()))
	}
	var ex ConditionView
	for _, cd := range v.Active {
		if cd.Id == "exhaustion" {
			ex = cd
		}
	}
	if ex.Label != "Exhaustion 2" {
		t.Errorf("label = %q, want %q", ex.Label, "Exhaustion 2")
	}
	if ex.Note != "Speed halved" {
		t.Errorf("level 2 note = %q", ex.Note)
	}
	if !strings.Contains(v.Summary, "Exhaustion 2") ||
		!strings.Contains(v.Summary, "Frightened") {
		t.Errorf("summary = %q", v.Summary)
	}
}

func TestDefenseMovesRatherThanDuplicates(t *testing.T) {
	rs := srd(t)
	c := sufferer()
	change(t, c, rs, ConditionsRequest{Action: CondResist, Name: "fire"})
	if len(c.Resistances) != 1 || c.Resistances[0] != "Fire" {
		t.Fatalf("resistances = %v", c.Resistances)
	}
	// Immunity to the same damage says everything the resistance did, so the
	// resistance goes rather than sitting there saying it too.
	change(t, c, rs, ConditionsRequest{Action: CondImmune, Name: "fire"})
	if len(c.Resistances) != 0 || len(c.Immunities) != 1 {
		t.Errorf("resistances = %v, immunities = %v", c.Resistances, c.Immunities)
	}
	if _, err := ApplyConditionChange(c, rs, ConditionsRequest{
		Action: CondImmune, Name: "Fire"}); err == nil {
		t.Errorf("the same immunity was taken twice")
	}
	change(t, c, rs, ConditionsRequest{Action: CondUnprotect, Name: "fire"})
	if len(c.Immunities) != 0 {
		t.Errorf("immunities = %v after unprotect", c.Immunities)
	}
}

func TestDefenseKeepsWhatItCannotName(t *testing.T) {
	rs := srd(t)
	c := sufferer()
	// A phrase the rules have no id for is kept as it was written, rather
	// than being refused or slugified into something unreadable.
	change(t, c, rs, ConditionsRequest{
		Action: CondResist, Name: "bludgeoning from nonmagical attacks"})
	if c.Resistances[0] != "bludgeoning from nonmagical attacks" {
		t.Errorf("resistances = %v", c.Resistances)
	}
	// A condition names itself properly, so "immune to being frightened" is
	// stored the way the book spells it.
	change(t, c, rs, ConditionsRequest{Action: CondImmune, Name: "frightened"})
	if c.Immunities[0] != "Frightened" {
		t.Errorf("immunities = %v", c.Immunities)
	}
	d := ComputeDefenses(c, rs)
	if !d.Any || len(d.Resistances) != 1 || len(d.Immunities) != 1 {
		t.Errorf("defenses = %+v", d)
	}
	if d.Resistances[0].Damage {
		t.Errorf("a free text resistance was taken for a damage type")
	}
}

// ---------------------------------------------------------------- hit points

func hurt(t *testing.T, c *Character, rs *Ruleset, req HealthRequest) HealthEvent {
	t.Helper()
	e, err := ApplyHealth(c, rs, req)
	if err != nil {
		t.Fatalf("%s %d: %s", req.Action, req.Amount, err)
	}
	return e
}

// The bar on the html sheet is coloured from HealthLevel, and the page says
// the same bands again in javascript so a rest can colour the bar without
// asking the server. If these move, that copy has to move with them.
func TestHealthBands(t *testing.T) {
	for _, tc := range []struct {
		percent int
		want    string
	}{
		{100, "hale"}, {75, "hale"}, {74, "hurt"}, {50, "hurt"},
		{49, "bloodied"}, {25, "bloodied"}, {24, "dying"}, {0, "dying"},
	} {
		if got := HealthLevel(tc.percent); got != tc.want {
			t.Errorf("HealthLevel(%d) = %q, want %q", tc.percent, got, tc.want)
		}
	}
}

func TestDamageComesOffTemporaryHitPointsFirst(t *testing.T) {
	rs := srd(t)
	c := sufferer()
	c.HPMax, c.HPCurrent = 27, 27
	hurt(t, c, rs, HealthRequest{Action: SetTemp, Amount: 5})
	if c.HPTemp != 5 {
		t.Fatalf("temp = %d", c.HPTemp)
	}
	// Nine points against five of buffer: five soaked, four drawn.
	e := hurt(t, c, rs, HealthRequest{Action: Hurt, Amount: 9})
	if e.Absorbed != 5 {
		t.Errorf("soaked %d, want 5", e.Absorbed)
	}
	if c.HPTemp != 0 || c.HPCurrent != 23 {
		t.Errorf("hp %d, temp %d, want 23 and 0", c.HPCurrent, c.HPTemp)
	}
}

func TestHealingStopsAtTheMaximum(t *testing.T) {
	rs := srd(t)
	c := sufferer()
	c.HPMax, c.HPCurrent = 27, 20
	e := hurt(t, c, rs, HealthRequest{Action: Heal, Amount: 100})
	if c.HPCurrent != 27 {
		t.Errorf("healed to %d, want 27", c.HPCurrent)
	}
	// The amount recorded is what was actually gained, not what was offered.
	if e.Amount != 7 {
		t.Errorf("recorded %d healed, want 7", e.Amount)
	}
	if _, err := ApplyHealth(c, rs, HealthRequest{Action: Heal, Amount: 5}); err == nil {
		t.Errorf("healing at full hit points was allowed")
	}
}

func TestHealingDoesNotRefillTheBuffer(t *testing.T) {
	rs := srd(t)
	c := sufferer()
	c.HPMax, c.HPCurrent, c.HPTemp = 27, 20, 4
	hurt(t, c, rs, HealthRequest{Action: Heal, Amount: 5})
	if c.HPTemp != 4 {
		t.Errorf("temp = %d, healing should leave it alone", c.HPTemp)
	}
}

func TestGoingDownAndBeingBroughtRound(t *testing.T) {
	rs := srd(t)
	c := sufferer()
	c.HPMax, c.HPCurrent = 27, 6
	c.DeathSaves = "s1 f1"
	e := hurt(t, c, rs, HealthRequest{Action: Hurt, Amount: 40})
	if c.HPCurrent != 0 || !e.Down {
		t.Errorf("hp %d, down %v - hit points do not go negative", c.HPCurrent, e.Down)
	}
	if c.DeathSaves != "s1 f1" {
		t.Errorf("death saves were cleared by going down")
	}
	// A single point of healing brings someone round, and the marks against
	// them go with it.
	hurt(t, c, rs, HealthRequest{Action: Heal, Amount: 1})
	if c.HPCurrent != 1 || c.DeathSaves != "" {
		t.Errorf("hp %d, death saves %q", c.HPCurrent, c.DeathSaves)
	}
}

// ------------------------------------------------------------- the org sheet

func TestConditionsSurviveTheOrgSheet(t *testing.T) {
	rs := srd(t)
	c := sufferer()
	c.HPMax, c.HPCurrent = 27, 27
	change(t, c, rs, ConditionsRequest{Action: CondAdd, Name: "exhaustion", Level: 3})
	change(t, c, rs, ConditionsRequest{
		Action: CondAdd, Name: "frightened", Notes: "the wraith's shriek"})
	change(t, c, rs, ConditionsRequest{Action: CondResist, Name: "fire"})
	change(t, c, rs, ConditionsRequest{Action: CondImmune, Name: "poison"})
	change(t, c, rs, ConditionsRequest{Action: CondVulnerabl, Name: "cold"})
	hurt(t, c, rs, HealthRequest{Action: SetTemp, Amount: 5})
	hurt(t, c, rs, HealthRequest{Action: Hurt, Amount: 9, Notes: "wraith's touch"})

	org := RenderOrg(c, rs)
	back, err := ParseOrg(org, rs)
	if err != nil {
		t.Fatal(err)
	}

	if got := back.ConditionLevel("exhaustion"); got != 3 {
		t.Errorf("exhaustion came back at %d, want 3", got)
	}
	if !back.HasCondition("frightened") {
		t.Errorf("frightened did not come back")
	}
	if len(back.Resistances) != 1 || back.Resistances[0] != "Fire" {
		t.Errorf("resistances came back as %v", back.Resistances)
	}
	if len(back.Immunities) != 1 || len(back.Vulnerabilities) != 1 {
		t.Errorf("immunities %v, vulnerabilities %v",
			back.Immunities, back.Vulnerabilities)
	}
	if back.HPCurrent != 23 || back.HPTemp != 0 {
		t.Errorf("hp %d, temp %d, want 23 and 0", back.HPCurrent, back.HPTemp)
	}

	// The two logs are the file's own record, so they have to read back whole.
	if len(back.ConditionLog) != len(c.ConditionLog) {
		t.Fatalf("condition history came back with %d of %d lines",
			len(back.ConditionLog), len(c.ConditionLog))
	}
	first := back.ConditionLog[0]
	if first.Action != "gained" || first.Name != "Exhaustion" || first.Level != 3 {
		t.Errorf("first condition line came back as %+v", first)
	}
	if got := back.ConditionLog[1].Notes; got != "the wraith's shriek" {
		t.Errorf("notes came back as %q", got)
	}
	if len(back.HealthLog) != len(c.HealthLog) {
		t.Fatalf("health history came back with %d of %d lines",
			len(back.HealthLog), len(c.HealthLog))
	}
	blow := back.HealthLog[len(back.HealthLog)-1]
	if blow.Action != "hurt" || blow.Amount != 9 || blow.Absorbed != 5 ||
		blow.HPAfter != 23 || blow.HPMax != 27 {
		t.Errorf("the blow came back as %+v", blow)
	}

	// And the derived block says what is going on, for anyone reading the
	// file rather than the sheet.
	if !strings.Contains(org, "Exhaustion 3") {
		t.Errorf("the org sheet does not mention the exhaustion:\n%s", org)
	}
	if !strings.Contains(org, "Disadvantage on attack rolls and saving throws") {
		t.Errorf("the org sheet does not say what exhaustion 3 costs")
	}
}

func TestEmptyConditionsWriteNothingBack(t *testing.T) {
	rs := srd(t)
	c := sufferer()
	org := RenderOrg(c, rs)
	if strings.Contains(org, ConditionHistoryHeading) ||
		strings.Contains(org, HealthHistoryHeading) {
		t.Errorf("a character nothing has happened to got history sections")
	}
	back, err := ParseOrg(org, rs)
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Conditions) != 0 || len(back.Resistances) != 0 {
		t.Errorf("conditions %v, resistances %v", back.Conditions, back.Resistances)
	}
}
