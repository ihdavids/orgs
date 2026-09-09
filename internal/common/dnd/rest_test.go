// Tests for the use limits read out of feature text, and for what a short
// and a long rest give back.
package dnd

import (
	"strings"
	"testing"
)

func rester(class string, level int) *Character {
	return &Character{
		Name:      "Test",
		Race:      "human",
		Classes:   []ClassLevel{{Class: class, Level: level}},
		Abilities: map[string]int{"str": 14, "dex": 12, "con": 14, "int": 10, "wis": 10, "cha": 18},
	}
}

// limited finds a feature on a computed sheet by name.
func limited(t *testing.T, s *Sheet, name string) Trait {
	t.Helper()
	tr, ok := FindLimitedTrait(s, name)
	if !ok {
		t.Fatalf("%q is not a limited feature on this sheet", name)
	}
	return tr
}

func TestUsesReadOutOfFeatureText(t *testing.T) {
	rs := srd(t)
	for _, tc := range []struct {
		class    string
		level    int
		feature  string
		max      int
		recharge string
	}{
		// "Once you use this feature, you must finish a short or long rest."
		{"fighter", 1, "Second Wind", 1, RechargeShort},
		// "Starting at 17th level, you can use it twice before a rest."
		{"fighter", 2, "Action Surge", 1, RechargeShort},
		{"fighter", 17, "Action Surge", 2, RechargeShort},
		// "twice between long rests starting at 13th ... three times at 17th"
		{"fighter", 9, "Indomitable", 1, RechargeLong},
		{"fighter", 13, "Indomitable", 2, RechargeLong},
		{"fighter", 17, "Indomitable", 3, RechargeLong},
		// "twice between rests at 6th, three times at 18th"
		{"cleric", 2, "Channel Divinity", 1, RechargeShort},
		{"cleric", 6, "Channel Divinity", 2, RechargeShort},
		{"cleric", 18, "Channel Divinity", 3, RechargeShort},
		// "You can use this feature twice."
		{"druid", 2, "Wild Shape", 2, RechargeShort},
		// "a number of times equal to your Charisma modifier (minimum once)"
		{"bard", 1, "Bardic Inspiration", 4, RechargeLong},
		// "a number of times equal to 1 + your Charisma modifier"
		{"paladin", 1, "Divine Sense", 5, RechargeLong},
		// "You can't use this feature again until you finish a long rest."
		{"wizard", 1, "Arcane Recovery", 1, RechargeLong},
	} {
		s := Compute(rester(tc.class, tc.level), rs)
		got := limited(t, s, tc.feature)
		if got.UsesMax != tc.max || got.Recharge != tc.recharge {
			t.Errorf("%s %d %s: want %d uses per %s rest, got %d per %s",
				tc.class, tc.level, tc.feature, tc.max, tc.recharge,
				got.UsesMax, got.Recharge)
		}
	}
}

// A resource whose size is printed as a column of the class level table gets
// its limit from that column, because its prose never states one.
func TestUsesReadOutOfTheClassLevelTable(t *testing.T) {
	rs := srd(t)
	for _, tc := range []struct {
		class    string
		level    int
		feature  string
		max      int
		recharge string
	}{
		{"barbarian", 1, "Rage", 2, RechargeLong},
		{"barbarian", 10, "Rage", 4, RechargeLong},
		{"barbarian", 17, "Rage", 6, RechargeLong},
		{"monk", 2, "Ki", 2, RechargeShort},
		{"monk", 10, "Ki", 10, RechargeShort},
		{"sorcerer", 2, "Font of Magic", 2, RechargeLong},
		{"sorcerer", 10, "Font of Magic", 10, RechargeLong},
	} {
		s := Compute(rester(tc.class, tc.level), rs)
		got := limited(t, s, tc.feature)
		if got.UsesMax != tc.max || got.Recharge != tc.recharge {
			t.Errorf("%s %d %s: want %d uses per %s rest, got %d per %s",
				tc.class, tc.level, tc.feature, tc.max, tc.recharge,
				got.UsesMax, got.Recharge)
		}
	}
}

// The table is the whole answer, so a level it gives nothing at carries no
// limit: a 20th level barbarian's rages are unlimited and get no slots, and a
// 1st level monk has no ki to spend at all.
func TestUsesTableLevelsWithNothingInThemCarryNoLimit(t *testing.T) {
	rs := srd(t)
	for _, tc := range []struct {
		class   string
		level   int
		feature string
	}{
		{"barbarian", 20, "Rage"},
		{"monk", 1, "Ki"},
	} {
		s := Compute(rester(tc.class, tc.level), rs)
		if tr, ok := FindLimitedTrait(s, tc.feature); ok {
			t.Errorf("%s %d: %s should carry no limit, got %d uses",
				tc.class, tc.level, tc.feature, tr.UsesMax)
		}
	}
}

// A table resource is spent, written and given back like any other: the whole
// point of reading the column is that the sheet can tick it off.
func TestTableResourceSpendsAndRestsLikeAnyOther(t *testing.T) {
	rs := srd(t)
	c := rester("barbarian", 10)
	for i := 0; i < 2; i++ {
		if _, err := ApplyUseChange(c, rs, "spend", "Rage"); err != nil {
			t.Fatalf("raging: %v", err)
		}
	}
	back, err := ParseOrg(RenderOrg(c, rs), rs)
	if err != nil {
		t.Fatalf("reading the sheet back: %v", err)
	}
	if got := limited(t, Compute(back, rs), "Rage"); got.UsesSpent != 2 || got.UsesMax != 4 {
		t.Fatalf("want 2 of 4 rages spent, got %d of %d", got.UsesSpent, got.UsesMax)
	}
	// Rages come back on a long rest and not on a short one.
	if _, err := ApplyRest(back, rs, RestRequest{Kind: ShortRest}); err != nil {
		t.Fatalf("short rest: %v", err)
	}
	if got := limited(t, Compute(back, rs), "Rage"); got.UsesSpent != 2 {
		t.Fatalf("a short rest should leave rages spent, got %d", got.UsesSpent)
	}
	if _, err := ApplyRest(back, rs, RestRequest{Kind: LongRest}); err != nil {
		t.Fatalf("long rest: %v", err)
	}
	if got := limited(t, Compute(back, rs), "Rage"); got.UsesSpent != 0 {
		t.Fatalf("a long rest should give every rage back, got %d spent", got.UsesSpent)
	}
}

// A module may write a level table of its own, and it is believed over the
// prose the same way a formula is. A level past the end of the table gets the
// last row it has.
func TestUsesByLevelDeclaredByAModuleWins(t *testing.T) {
	tr := Trait{Name: "Starfall", UsesByLevel: []int{0, 1, 1, 2},
		Text: "You can do so twice. You regain expended uses on a short rest."}
	for _, tc := range []struct {
		level, max int
	}{{1, 1}, {3, 2}, {9, 2}} {
		max, recharge, _ := DetectUses(tr, tc.level, nil, 3)
		if max != tc.max || recharge != RechargeShort {
			t.Errorf("level %d: want %d uses per short rest, got %d per %s",
				tc.level, tc.max, max, recharge)
		}
	}
}

// A resource whose size is neither in its prose nor in a column is still left
// alone rather than given an invented limit.
func TestUsesLeavesUnstatedResourcesAlone(t *testing.T) {
	rs := srd(t)
	s := Compute(rester("warlock", 10), rs)
	if _, ok := FindLimitedTrait(s, "Pact Magic"); ok {
		t.Error("warlock: Pact Magic has no limit to read and should carry none")
	}
	max, _, _ := DetectUses(Trait{Name: "Starfall",
		Text: "As an action you call down a star. You regain your uses when " +
			"you finish a long rest."}, 5, nil, 3)
	if max != 0 {
		t.Errorf("a count nobody stated should not be invented, got %d", max)
	}
}

// A module that writes the limit down is believed over the prose.
func TestUsesDeclaredByAModuleWin(t *testing.T) {
	mods := map[string]int{"cha": 4}
	tr := Trait{Name: "Starfall", Uses: "1 + cha", Recharge: "short",
		Text: "Once you use this feature you must finish a long rest."}
	max, recharge, _ := DetectUses(tr, 5, mods, 3)
	if max != 5 || recharge != RechargeShort {
		t.Fatalf("want 5 uses per short rest, got %d per %s", max, recharge)
	}
}

// The srd names the same resource several times as it grows - "Bardic
// Inspiration (d6)" and "(d10)" - and they all draw on one pool.
func TestUsesShareAPoolAcrossTheirVariants(t *testing.T) {
	if a, b := UsesKey(Trait{Name: "Bardic Inspiration (d6)"}),
		UsesKey(Trait{Name: "Bardic Inspiration (d10)"}); a != b {
		t.Fatalf("bardic inspiration should share a pool, got %q and %q", a, b)
	}
	if a, b := UsesKey(Trait{Name: "Action Surge (one use)"}),
		UsesKey(Trait{Name: "Action Surge (two uses)"}); a != b {
		t.Fatalf("action surge should share a pool, got %q and %q", a, b)
	}
	// Two different arcanum spells are two different resources, though.
	if a, b := UsesKey(Trait{Name: "Mystic Arcanum (6th level)"}),
		UsesKey(Trait{Name: "Mystic Arcanum (7th level)"}); a == b {
		t.Fatalf("mystic arcanum spells are separate, both keyed %q", a)
	}
}

func TestSpendingAndRecoveringOneUse(t *testing.T) {
	rs := srd(t)
	c := rester("fighter", 1)
	if _, err := ApplyUseChange(c, rs, "spend", "Second Wind"); err != nil {
		t.Fatalf("spending second wind: %v", err)
	}
	if c.UsesSpent[UsesKey(Trait{Name: "Second Wind"})] != 1 {
		t.Fatalf("second wind should be spent, got %v", c.UsesSpent)
	}
	if _, err := ApplyUseChange(c, rs, "spend", "Second Wind"); err == nil {
		t.Fatal("second wind has one use; spending it twice should be refused")
	}
	if _, err := ApplyUseChange(c, rs, "recover", "Second Wind"); err != nil {
		t.Fatalf("recovering second wind: %v", err)
	}
	if len(c.UsesSpent) != 0 {
		t.Fatalf("nothing should be spent now, got %v", c.UsesSpent)
	}
	if _, err := ApplyUseChange(c, rs, "spend", "Extra Attack"); err == nil {
		t.Fatal("a feature with no limit has nothing to spend")
	}
}

func TestShortRestSpendsHitDiceAndRechargesShortFeatures(t *testing.T) {
	rs := srd(t)
	c := rester("fighter", 9)
	s := Compute(c, rs)
	c.HPCurrent = 10
	c.HPMax = s.HPMax
	// Second Wind comes back on a short rest, Indomitable does not.
	mustSpend(t, c, rs, "Second Wind")
	mustSpend(t, c, rs, "Indomitable")

	res, err := ApplyRest(c, rs, RestRequest{Kind: "short",
		HitDiceSpent: 2, HitPointsHealed: 13})
	if err != nil {
		t.Fatalf("short rest: %v", err)
	}
	if c.HitDiceUsed != 2 {
		t.Fatalf("want 2 hit dice spent, got %d", c.HitDiceUsed)
	}
	if c.HPCurrent != 23 || res.Healed != 13 {
		t.Fatalf("want 23 hp after healing 13, got %d hp / %d healed",
			c.HPCurrent, res.Healed)
	}
	if _, ok := c.UsesSpent[UsesKey(Trait{Name: "Second Wind"})]; ok {
		t.Fatal("second wind recharges on a short rest")
	}
	if c.UsesSpent[UsesKey(Trait{Name: "Indomitable"})] != 1 {
		t.Fatalf("indomitable is a long rest feature, got %v", c.UsesSpent)
	}
	if len(res.FeaturesBack) != 1 || !strings.Contains(res.FeaturesBack[0], "Second Wind") {
		t.Fatalf("want second wind back and nothing else, got %v", res.FeaturesBack)
	}
}

// Healing never runs past the maximum, and a rest with no dice spent heals
// nothing at all.
func TestShortRestCapsHealingAndRefusesDiceYouDoNotHave(t *testing.T) {
	rs := srd(t)
	c := rester("fighter", 3)
	c.HPMax = Compute(c, rs).HPMax
	c.HPCurrent = c.HPMax - 2
	if _, err := ApplyRest(c, rs, RestRequest{Kind: "short", HitDiceSpent: 4}); err == nil {
		t.Fatal("a level 3 fighter has 3 hit dice, not 4")
	}
	res, err := ApplyRest(c, rs, RestRequest{Kind: "short",
		HitDiceSpent: 1, HitPointsHealed: 40})
	if err != nil {
		t.Fatalf("short rest: %v", err)
	}
	if c.HPCurrent != c.HPMax || res.Healed != 2 {
		t.Fatalf("want capped at %d having healed 2, got %d hp / %d healed",
			c.HPMax, c.HPCurrent, res.Healed)
	}
}

func TestLongRestGivesBackEverything(t *testing.T) {
	rs := srd(t)
	c := rester("fighter", 9)
	c.HPMax = Compute(c, rs).HPMax
	c.HPCurrent = 4
	c.HPTemp = 3
	c.HitDiceUsed = 9
	c.DeathSaves = "1/1"
	c.SlotsUsed = []int{2, 1}
	mustSpend(t, c, rs, "Second Wind")
	mustSpend(t, c, rs, "Indomitable")

	res, err := ApplyRest(c, rs, RestRequest{Kind: "long"})
	if err != nil {
		t.Fatalf("long rest: %v", err)
	}
	if c.HPCurrent != c.HPMax || c.HPTemp != 0 || c.DeathSaves != "" {
		t.Fatalf("a long rest returns every hit point: %d/%d temp %d saves %q",
			c.HPCurrent, c.HPMax, c.HPTemp, c.DeathSaves)
	}
	// Half of nine hit dice, rounded down, is four.
	if c.HitDiceUsed != 5 || res.DiceBack != 4 {
		t.Fatalf("want 4 of 9 hit dice back, got %d back leaving %d spent",
			res.DiceBack, c.HitDiceUsed)
	}
	if len(c.SlotsUsed) != 0 || res.SlotsBack != 3 {
		t.Fatalf("want all 3 slots back, got %d back leaving %v",
			res.SlotsBack, c.SlotsUsed)
	}
	if len(c.UsesSpent) != 0 {
		t.Fatalf("a long rest recharges everything, got %v", c.UsesSpent)
	}
	if len(res.FeaturesBack) != 2 {
		t.Fatalf("want both features back, got %v", res.FeaturesBack)
	}
}

// A warlock's slots all come back on a short rest, and only when every slot
// they have is a warlock's. Everyone else gets the house rule instead: one
// slot of each level they have spent one at.
func TestShortRestRefillsPactSlotsOnly(t *testing.T) {
	rs := srd(t)
	warlock := rester("warlock", 5)
	warlock.HPMax = Compute(warlock, rs).HPMax
	warlock.HPCurrent = warlock.HPMax
	warlock.SlotsUsed = []int{0, 0, 2}
	if _, err := ApplyRest(warlock, rs, RestRequest{Kind: "short"}); err != nil {
		t.Fatalf("short rest: %v", err)
	}
	if len(warlock.SlotsUsed) != 0 {
		t.Fatalf("pact slots come back on a short rest, got %v", warlock.SlotsUsed)
	}

	both := rester("warlock", 3)
	both.Classes = append(both.Classes, ClassLevel{Class: "wizard", Level: 3})
	both.HPMax = Compute(both, rs).HPMax
	both.HPCurrent = both.HPMax
	both.SlotsUsed = []int{2, 2}
	res, err := ApplyRest(both, rs, RestRequest{Kind: "short"})
	if err != nil {
		t.Fatalf("short rest: %v", err)
	}
	// Mixed slots are not all a warlock's, so they do not all come back: one
	// of each level does, and the rest wait for the long rest.
	if res.SlotsBack != 2 || len(both.SlotsUsed) != 2 ||
		both.SlotsUsed[0] != 1 || both.SlotsUsed[1] != 1 {
		t.Fatalf("want one slot of each level back, got %d back leaving %v",
			res.SlotsBack, both.SlotsUsed)
	}
}

// The house rule: an hour is worth one slot of every level you have spent one
// at, and nothing at a level you have not.
func TestShortRestGivesBackOneSlotOfEachLevel(t *testing.T) {
	rs := srd(t)
	c := rester("wizard", 5)
	c.HPMax = Compute(c, rs).HPMax
	c.HPCurrent = c.HPMax
	c.SlotsUsed = []int{2, 0, 2}

	res, err := ApplyRest(c, rs, RestRequest{Kind: "short"})
	if err != nil {
		t.Fatalf("short rest: %v", err)
	}
	if res.SlotsBack != 2 || len(c.SlotsUsed) != 3 ||
		c.SlotsUsed[0] != 1 || c.SlotsUsed[1] != 0 || c.SlotsUsed[2] != 1 {
		t.Fatalf("want one 1st and one 3rd back, got %d back leaving %v",
			res.SlotsBack, c.SlotsUsed)
	}
	found := false
	for _, line := range res.Lines {
		if strings.Contains(line, "Recovered 2 spell slots") {
			found = true
		}
	}
	if !found {
		t.Fatalf("the log should say what came back, got %v", res.Lines)
	}

	res, err = ApplyRest(c, rs, RestRequest{Kind: "short"})
	if err != nil {
		t.Fatalf("short rest: %v", err)
	}
	if res.SlotsBack != 2 || len(c.SlotsUsed) != 0 {
		t.Fatalf("want the other two back, got %d leaving %v",
			res.SlotsBack, c.SlotsUsed)
	}

	// Nothing spent, nothing to give back.
	res, err = ApplyRest(c, rs, RestRequest{Kind: "short"})
	if err != nil {
		t.Fatalf("short rest: %v", err)
	}
	if res.SlotsBack != 0 {
		t.Fatalf("want nothing back, got %d", res.SlotsBack)
	}
}

func TestRestPlanDescribesWhatIsOnOffer(t *testing.T) {
	rs := srd(t)
	c := rester("fighter", 5)
	c.HPMax = Compute(c, rs).HPMax
	c.HPCurrent = 1
	c.HitDiceUsed = 1
	mustSpend(t, c, rs, "Second Wind")

	short := RestPlan(c, rs, "short")
	if short.HitDiceLeft != 4 || short.HitDieFaces != 10 {
		t.Fatalf("a level 5 fighter has 4 of 5 d10 hit dice left, got %d d%d",
			short.HitDiceLeft, short.HitDieFaces)
	}
	if step := planStep(short, "hitdice"); step == nil || step.Max != 4 || step.Kind != "hitdice" {
		t.Fatalf("the hit dice step should offer 4 dice, got %+v", step)
	}
	if len(short.Recharges) != 1 || short.Recharges[0].Name != "Second Wind" {
		t.Fatalf("want second wind coming back, got %+v", short.Recharges)
	}

	long := RestPlan(c, rs, "long")
	if planStep(long, "hitdice").Kind != "info" {
		t.Fatal("a long rest gives hit dice back rather than asking for them")
	}
	if long.Name != "Long Rest" || !strings.Contains(long.Duration, "8 hours") {
		t.Fatalf("bad long rest plan: %+v", long)
	}
}

func planStep(p *RestPlanView, id string) *RestStep {
	for i := range p.Steps {
		if p.Steps[i].Id == id {
			return &p.Steps[i]
		}
	}
	return nil
}

func mustSpend(t *testing.T, c *Character, rs *Ruleset, name string) {
	t.Helper()
	if _, err := ApplyUseChange(c, rs, "spend", name); err != nil {
		t.Fatalf("spending %s: %v", name, err)
	}
}

// Spent uses survive a trip through the org file, which is the only place
// they are stored.
func TestUsesSurviveTheOrgFile(t *testing.T) {
	rs := srd(t)
	c := rester("cleric", 6)
	c.HPMax = Compute(c, rs).HPMax
	c.HPCurrent = c.HPMax
	mustSpend(t, c, rs, "Channel Divinity")

	back, err := ParseOrg(RenderOrg(c, rs), rs)
	if err != nil {
		t.Fatalf("reading the sheet back: %v", err)
	}
	if back.UsesSpent[UsesKey(Trait{Name: "Channel Divinity"})] != 1 {
		t.Fatalf("want channel divinity spent once, got %v", back.UsesSpent)
	}
	s := Compute(back, rs)
	got := limited(t, s, "Channel Divinity")
	if got.UsesSpent != 1 || got.UsesMax != 2 {
		t.Fatalf("want 1 of 2 spent, got %d of %d", got.UsesSpent, got.UsesMax)
	}
}
