// Tests for managing a character's spells: what each kind of caster is
// allowed, what the picker offers, and what the rules refuse.
package dnd

import "testing"

// wizard, cleric and bard stand in for the three ways a class holds its
// spells: a spellbook, the whole class list, and a fixed known list.
func caster(class string, level int, abilities map[string]int) *Character {
	c := &Character{
		Name:      "Test",
		Race:      "human",
		Classes:   []ClassLevel{{Class: class, Level: level}},
		Abilities: abilities,
	}
	return c
}

func stdAbilities(casting string, score int) map[string]int {
	a := map[string]int{"str": 10, "dex": 10, "con": 10, "int": 10, "wis": 10, "cha": 10}
	a[casting] = score
	return a
}

func allot(t *testing.T, v SpellbookView, kind string) SpellAllotment {
	t.Helper()
	a := v.Allotment(kind)
	if a == nil {
		t.Fatalf("no %q allotment, got %+v", kind, v.Allotments)
	}
	return *a
}

func learn(t *testing.T, c *Character, rs *Ruleset, id string) {
	t.Helper()
	if _, err := LearnSpell(c, rs, id); err != nil {
		t.Fatalf("learning %s: %v", id, err)
	}
}

// ---------------------------------------------------------------- allotments

func TestWizardHasASpellbookAndASmallerPreparedList(t *testing.T) {
	rs := srd(t)
	// int 16 at level 3: 6 + 2 more per level = 10 in the book, mod+level
	// prepared, so 3 + 3 = 6.
	c := caster("wizard", 3, stdAbilities("int", 16))
	v := Spellbook(c, rs)
	if v.Mode != BookSpellbook || !v.TwoStage {
		t.Fatalf("a wizard prepares from a spellbook, got mode %q twoStage %v", v.Mode, v.TwoStage)
	}
	if got := allot(t, v, "known").Max; got != 10 {
		t.Fatalf("a level 3 wizard has 10 spells in the book, got %d", got)
	}
	if got := allot(t, v, "prepared").Max; got != 6 {
		t.Fatalf("int 16 at level 3 prepares 6, got %d", got)
	}
	if got := allot(t, v, "cantrips").Max; got != 3 {
		t.Fatalf("a level 3 wizard knows 3 cantrips, got %d", got)
	}
	if v.MaxLevel != 2 {
		t.Fatalf("a level 3 wizard casts up to 2nd level, got %d", v.MaxLevel)
	}
}

func TestClericPreparesFromTheWholeListWithNoSeparateStep(t *testing.T) {
	rs := srd(t)
	c := caster("cleric", 5, stdAbilities("wis", 16))
	v := Spellbook(c, rs)
	if v.Mode != BookList || v.TwoStage {
		t.Fatalf("a cleric prepares from the list, got mode %q twoStage %v", v.Mode, v.TwoStage)
	}
	if v.Allotment("known") != nil {
		t.Fatal("a cleric has no spells known budget, it prepares from the whole list")
	}
	if got := allot(t, v, "prepared").Max; got != 8 {
		t.Fatalf("wis 16 at level 5 prepares 8, got %d", got)
	}
}

func TestBardKnowsAFixedListAndPreparesNothing(t *testing.T) {
	rs := srd(t)
	c := caster("bard", 4, stdAbilities("cha", 16))
	v := Spellbook(c, rs)
	if v.Mode != BookKnown || v.TwoStage {
		t.Fatalf("a bard knows a fixed list, got mode %q twoStage %v", v.Mode, v.TwoStage)
	}
	if v.Allotment("prepared") != nil {
		t.Fatal("a bard prepares nothing, its spells are always ready")
	}
	if got := allot(t, v, "known").Max; got != 7 {
		t.Fatalf("a level 4 bard knows 7 spells, got %d", got)
	}
}

// ------------------------------------------------------------------ learning

func TestLearningCountsAgainstTheRightBudget(t *testing.T) {
	rs := srd(t)
	c := caster("wizard", 3, stdAbilities("int", 16))
	learn(t, c, rs, "fire-bolt")
	learn(t, c, rs, "magic-missile")
	v := Spellbook(c, rs)
	if got := allot(t, v, "cantrips").Used; got != 1 {
		t.Fatalf("one cantrip taken, got %d", got)
	}
	if got := allot(t, v, "known").Used; got != 1 {
		t.Fatalf("one spell in the book, got %d", got)
	}
	// A spellbook caster writes a spell down without preparing it.
	if got := allot(t, v, "prepared").Used; got != 0 {
		t.Fatalf("nothing is prepared yet, got %d", got)
	}
}

func TestALearnedSpellIsReadyForEveryoneButASpellbookCaster(t *testing.T) {
	rs := srd(t)
	for _, tc := range []struct{ class, spell string }{
		{"bard", "charm-person"}, {"cleric", "bless"},
	} {
		c := caster(tc.class, 3, stdAbilities("cha", 16))
		if tc.class == "cleric" {
			c.Abilities = stdAbilities("wis", 16)
		}
		learn(t, c, rs, tc.spell)
		if !c.Spells[len(c.Spells)-1].Prepared {
			t.Fatalf("a %s has %s ready as soon as it is taken", tc.class, tc.spell)
		}
	}
}

func TestCannotLearnASpellAboveYourSlots(t *testing.T) {
	rs := srd(t)
	c := caster("wizard", 3, stdAbilities("int", 16))
	// A level 3 wizard has 2nd level slots and no more.
	if _, err := LearnSpell(c, rs, "fireball"); err == nil {
		t.Fatal("a level 3 wizard cannot write fireball into the book")
	}
	if _, err := LearnSpell(c, rs, "misty-step"); err != nil {
		t.Fatalf("a level 3 wizard can take a 2nd level spell: %v", err)
	}
}

func TestCannotLearnOffAnotherClassList(t *testing.T) {
	rs := srd(t)
	c := caster("wizard", 5, stdAbilities("int", 16))
	// Cure wounds is a cleric, bard, druid, paladin and ranger spell.
	if _, err := LearnSpell(c, rs, "cure-wounds"); err == nil {
		t.Fatal("a wizard cannot learn cure wounds")
	}
}

func TestCannotLearnPastTheBudget(t *testing.T) {
	rs := srd(t)
	c := caster("bard", 1, stdAbilities("cha", 16))
	// A level 1 bard knows 2 cantrips and 4 spells.
	learn(t, c, rs, "vicious-mockery")
	learn(t, c, rs, "minor-illusion")
	if _, err := LearnSpell(c, rs, "dancing-lights"); err == nil {
		t.Fatal("a third cantrip is one too many for a level 1 bard")
	}
	for _, id := range []string{"charm-person", "healing-word", "faerie-fire", "sleep"} {
		learn(t, c, rs, id)
	}
	if _, err := LearnSpell(c, rs, "thunderwave"); err == nil {
		t.Fatal("a fifth spell is one too many for a level 1 bard")
	}
	// Giving one back makes room again.
	if _, err := ForgetSpell(c, rs, "sleep"); err != nil {
		t.Fatalf("giving sleep back: %v", err)
	}
	learn(t, c, rs, "thunderwave")
}

func TestLearningTheSameSpellTwiceIsRefused(t *testing.T) {
	rs := srd(t)
	c := caster("bard", 3, stdAbilities("cha", 16))
	learn(t, c, rs, "charm-person")
	if _, err := LearnSpell(c, rs, "charm-person"); err == nil {
		t.Fatal("charm person is already on the sheet")
	}
}

// ----------------------------------------------------------------- preparing

func TestSpellbookCasterPreparesOutOfTheBookAgainstItsOwnBudget(t *testing.T) {
	rs := srd(t)
	// int 12 at level 1: mod 1 + level 1 = 2 prepared, 6 in the book.
	c := caster("wizard", 1, stdAbilities("int", 12))
	for _, id := range []string{"magic-missile", "shield", "sleep"} {
		learn(t, c, rs, id)
	}
	if _, err := PrepareSpell(c, rs, "magic-missile", true); err != nil {
		t.Fatalf("preparing magic missile: %v", err)
	}
	if _, err := PrepareSpell(c, rs, "shield", true); err != nil {
		t.Fatalf("preparing shield: %v", err)
	}
	if _, err := PrepareSpell(c, rs, "sleep", true); err == nil {
		t.Fatal("a third prepared spell is one too many at int 12 level 1")
	}
	if _, err := PrepareSpell(c, rs, "shield", false); err != nil {
		t.Fatalf("unpreparing shield: %v", err)
	}
	if _, err := PrepareSpell(c, rs, "sleep", true); err != nil {
		t.Fatalf("there is room now: %v", err)
	}
}

func TestPreparingIsNotAStepForAClassThatDoesNotDoIt(t *testing.T) {
	rs := srd(t)
	c := caster("bard", 3, stdAbilities("cha", 16))
	learn(t, c, rs, "charm-person")
	if _, err := PrepareSpell(c, rs, "charm-person", false); err == nil {
		t.Fatal("a bard's spells are always ready, there is nothing to unprepare")
	}
}

func TestCantripsAreAlwaysReady(t *testing.T) {
	rs := srd(t)
	c := caster("wizard", 3, stdAbilities("int", 16))
	learn(t, c, rs, "fire-bolt")
	if !c.Spells[0].Prepared {
		t.Fatal("a cantrip is prepared the moment it is learned")
	}
	if _, err := PrepareSpell(c, rs, "fire-bolt", false); err == nil {
		t.Fatal("a cantrip cannot be unprepared")
	}
}

// ------------------------------------------------------------------- granted

func TestGrantedSpellsAreFreeAndCannotBeGivenBack(t *testing.T) {
	rs := srd(t)
	c := caster("cleric", 3, stdAbilities("wis", 16))
	c.Classes[0].Subclass = "life-domain"
	GrantSubclassSpells(c, rs)
	granted := 0
	for _, ks := range c.Spells {
		if ks.Source == "" {
			continue
		}
		granted++
		if _, err := ForgetSpell(c, rs, ks.Id); err == nil {
			t.Fatalf("%s was granted by %s and is not the cleric's to give back",
				ks.Name, ks.Source)
		}
	}
	if granted == 0 {
		t.Fatal("the life domain grants spells")
	}
	// None of them come out of the prepared allowance.
	if got := allot(t, Spellbook(c, rs), "prepared").Used; got != 0 {
		t.Fatalf("domain spells are free, got %d used", got)
	}
}

// -------------------------------------------------------------------- picker

func TestPickerOffersTheClassListAndSaysWhatIsTaken(t *testing.T) {
	rs := srd(t)
	c := caster("wizard", 3, stdAbilities("int", 16))
	learn(t, c, rs, "magic-missile")
	hits := SearchSpells(c, rs, "", 0)
	if len(hits) < 20 {
		t.Fatalf("the wizard list is longer than %d spells", len(hits))
	}
	var mm, fb, cw *SpellMatch
	for i := range hits {
		switch hits[i].Id {
		case "magic-missile":
			mm = &hits[i]
		case "fireball":
			fb = &hits[i]
		case "cure-wounds":
			cw = &hits[i]
		}
	}
	if cw != nil {
		t.Fatal("cure wounds is not on the wizard list and should not be offered")
	}
	if mm == nil || !mm.Known || !mm.CanForget || mm.CanLearn {
		t.Fatalf("magic missile is already in the book: %+v", mm)
	}
	// It is in the book but not prepared, so preparing is what is on offer.
	if !mm.CanPrepare || mm.Prepared {
		t.Fatalf("magic missile can be prepared: %+v", mm)
	}
	if fb == nil || fb.CanLearn || fb.Why == "" {
		t.Fatalf("fireball is out of reach at level 3 and should say why: %+v", fb)
	}
}

func TestPickerFiltersFuzzily(t *testing.T) {
	rs := srd(t)
	c := caster("wizard", 5, stdAbilities("int", 16))
	hits := SearchSpells(c, rs, "mag mis", 0)
	if len(hits) == 0 || hits[0].Id != "magic-missile" {
		t.Fatalf("mag mis is magic missile, got %+v", firstIds(hits))
	}
	for _, h := range SearchSpells(c, rs, "ritual", 0) {
		if !h.Ritual {
			t.Fatalf("%s is not a ritual", h.Name)
		}
	}
}

func firstIds(hits []SpellMatch) []string {
	out := []string{}
	for i, h := range hits {
		if i >= 5 {
			break
		}
		out = append(out, h.Id)
	}
	return out
}

// --------------------------------------------------------------- round trip

func TestSpellsSurviveTheOrgFile(t *testing.T) {
	rs := srd(t)
	c := caster("wizard", 3, stdAbilities("int", 16))
	c.Name = "Lyra Silverleaf"
	for _, id := range []string{"fire-bolt", "magic-missile", "shield", "misty-step"} {
		learn(t, c, rs, id)
	}
	if _, err := PrepareSpell(c, rs, "magic-missile", true); err != nil {
		t.Fatalf("preparing: %v", err)
	}
	org := RenderOrg(c, rs)
	back, err := ParseOrg(org, rs)
	if err != nil {
		t.Fatalf("parsing the sheet back: %v", err)
	}
	if len(back.Spells) != len(c.Spells) {
		t.Fatalf("want %d spells back, got %d", len(c.Spells), len(back.Spells))
	}
	prepared := map[string]bool{}
	for _, ks := range back.Spells {
		prepared[ks.Id] = ks.Prepared
	}
	if !prepared["magic-missile"] {
		t.Fatal("magic missile was prepared and should have come back prepared")
	}
	if prepared["shield"] {
		t.Fatal("shield was never prepared and should not have come back prepared")
	}
	if !prepared["fire-bolt"] {
		t.Fatal("a cantrip always comes back ready")
	}
}
