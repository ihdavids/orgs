package dnd

import "testing"

func lvlRuleset(t *testing.T) *Ruleset {
	t.Helper()
	l := NewLibrary([]string{"../../../templates/dnd"})
	l.Resolve()
	rs := l.Get("srd")
	if rs == nil {
		t.Fatal("no srd ruleset")
	}
	return rs
}

func rogue(level int, sub string) *Character {
	return &Character{Name: "Lyra", Race: "human",
		Abilities: map[string]int{"str": 10, "dex": 16, "con": 14, "int": 12, "wis": 10, "cha": 8},
		Classes:   []ClassLevel{{Class: "rogue", Subclass: sub, Level: level}},
		Skills:    []string{"stealth", "perception", "acrobatics", "investigation"},
		HPMax:     17, HPCurrent: 17, Choices: map[string][]string{}}
}

// walk answers a whole climb, the way a client does: the same starting
// character every time, with one more answer each pass. It returns the plan it
// finished on and the levelled character.
func walk(t *testing.T, start *Character, rs *Ruleset, req *LevelUpRequest,
	answer func(p *Prompt) []string) (*LevelUpPlan, *Character) {
	t.Helper()
	if req.Answers == nil {
		req.Answers = map[string][]string{}
	}
	for i := 0; i < 40; i++ {
		plan, c, err := LevelUp(start, rs, req)
		if err != nil {
			t.Fatalf("level up: %s", err)
		}
		if plan.Done {
			return plan, c
		}
		if plan.Next == nil {
			t.Fatal("neither done nor asking")
		}
		if plan.Next.Error != "" {
			t.Fatalf("answer to %s refused: %s", plan.Next.Step, plan.Next.Error)
		}
		req.Answers[plan.Next.Step] = answer(plan.Next)
	}
	t.Fatal("the climb never finished")
	return nil, nil
}

// firstValid is the naive client: take whatever is offered.
func firstValid(p *Prompt) []string {
	n := p.Min
	if n < 1 {
		n = 1
	}
	out := []string{}
	for _, o := range p.Options {
		if o.Disabled {
			continue
		}
		out = append(out, o.Id)
		if len(out) == n {
			break
		}
	}
	return out
}

// The character the flow is run against must be the one that comes out, and
// hit points are the one thing every level touches.
func TestLevelUpHitPoints(t *testing.T) {
	rs := lvlRuleset(t)
	start := rogue(2, "")
	// A d8 averages 5, and Constitution 14 is +2, so a level is worth 7.
	_, c := walk(t, start, rs, &LevelUpRequest{Levels: 1, Seed: 1},
		func(p *Prompt) []string {
			if p.Step == "hp-rogue-3" {
				return []string{"average"}
			}
			return firstValid(p)
		})
	if c.HPMax != 24 {
		t.Errorf("hit points = %d, want 24 (17 plus 5 average plus 2 con)", c.HPMax)
	}
	if c.HPCurrent != 24 {
		t.Errorf("current hit points = %d, a level should raise them too", c.HPCurrent)
	}
	if classLevelOf(c, "rogue") != 3 {
		t.Errorf("rogue level = %d, want 3", classLevelOf(c, "rogue"))
	}
}

// A number typed in is the hit points rolled at the table, and has to be a
// number the die could actually have rolled.
func TestLevelUpAcceptsARolledNumber(t *testing.T) {
	rs := lvlRuleset(t)
	_, c := walk(t, rogue(2, "thief"), rs, &LevelUpRequest{Levels: 1, Seed: 1},
		func(p *Prompt) []string { return []string{"8"} })
	if c.HPMax != 27 {
		t.Errorf("hit points = %d, want 27 (17 plus a rolled 8 plus 2 con)", c.HPMax)
	}

	// A number no d8 could roll is refused rather than written down.
	c2 := rogue(2, "thief")
	req := &LevelUpRequest{Levels: 1, Seed: 1,
		Answers: map[string][]string{"hp-rogue-3": {"19"}}}
	plan, out, err := LevelUp(c2, rs, req)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Next == nil || plan.Next.Error == "" {
		t.Fatal("a 19 on a d8 was accepted")
	}
	if out.HPMax != 17 {
		t.Errorf("a refused answer still changed the hit points: %d", out.HPMax)
	}
	if c2.HPMax != 17 {
		t.Errorf("the character handed in was modified: %d", c2.HPMax)
	}
}

// Several levels at once are taken in order, and a choice that exists only
// because of an earlier level is asked at the right moment. A rogue going
// 2 to 4 picks their archetype at 3 and their improvement at 4.
func TestLevelUpSeveralLevelsInOrder(t *testing.T) {
	rs := lvlRuleset(t)
	asked := []string{}
	plan, c := walk(t, rogue(2, ""), rs, &LevelUpRequest{Levels: 2, Seed: 1},
		func(p *Prompt) []string {
			asked = append(asked, p.Step)
			if p.Step == "subclass-rogue" {
				return []string{"thief"}
			}
			if p.Step == "asi-rogue-4" {
				return []string{"dex", "con"}
			}
			return []string{"average"}
		})
	want := []string{"hp-rogue-3", "subclass-rogue", "hp-rogue-4", "asi-rogue-4"}
	if len(asked) != len(want) {
		t.Fatalf("asked %v, want %v", asked, want)
	}
	for i := range want {
		if asked[i] != want[i] {
			t.Fatalf("asked %v, want %v", asked, want)
		}
	}
	if got := subclassOf(c, "rogue"); got != "thief" {
		t.Errorf("subclass = %q, want thief", got)
	}
	if c.Abilities["dex"] != 17 || c.Abilities["con"] != 15 {
		t.Errorf("the improvement did not land: dex %d con %d",
			c.Abilities["dex"], c.Abilities["con"])
	}
	if plan.ToLevel != 4 {
		t.Errorf("finished at level %d, want 4", plan.ToLevel)
	}
}

// An improvement cannot take a score past 20, and a 19 cannot take the +2 on
// its own - which the option says before it is picked, not after.
func TestLevelUpImprovementRespectsTheCap(t *testing.T) {
	rs := lvlRuleset(t)
	c := rogue(3, "thief")
	c.Abilities["dex"] = 19
	req := &LevelUpRequest{Levels: 1, Seed: 1,
		Answers: map[string][]string{"hp-rogue-4": {"average"}, "asi-rogue-4": {"dex"}}}
	plan, out, err := LevelUp(c, rs, req)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Next == nil || plan.Next.Error == "" {
		t.Fatal("+2 onto a 19 was allowed")
	}
	if out.Abilities["dex"] != 19 {
		t.Errorf("a refused improvement still changed the score: %d", c.Abilities["dex"])
	}
	// And the option warned about it in advance.
	for _, o := range plan.Next.Options {
		if o.Id == "dex" && o.Reason == "" {
			t.Error("a 19 was offered with nothing said about the cap")
		}
	}
}

// A feat is the alternative to the numbers, and takes the whole improvement.
func TestLevelUpFeatInsteadOfImprovement(t *testing.T) {
	rs := lvlRuleset(t)
	feat := ""
	_, c := walk(t, rogue(3, "thief"), rs, &LevelUpRequest{Levels: 1, Seed: 1},
		func(p *Prompt) []string {
			if p.Step != "asi-rogue-4" {
				return []string{"average"}
			}
			for _, o := range p.Options {
				if len(o.Id) > len(featOptionPrefix) && o.Id[:len(featOptionPrefix)] == featOptionPrefix {
					feat = o.Id[len(featOptionPrefix):]
					return []string{o.Id}
				}
			}
			t.Fatal("no feat was offered as an alternative")
			return nil
		})
	if feat == "" || !containsStr(c.Feats, feat) {
		t.Errorf("the feat %q was not taken: %v", feat, c.Feats)
	}
}

// Levelling a class the character does not have is multiclassing into it.
func TestLevelUpIntoANewClass(t *testing.T) {
	rs := lvlRuleset(t)
	_, c := walk(t, rogue(3, "thief"), rs, &LevelUpRequest{Levels: 1, Class: "fighter", Seed: 1},
		func(p *Prompt) []string { return firstValid(p) })
	if len(c.Classes) != 2 {
		t.Fatalf("classes = %v, want the fighter added", c.Classes)
	}
	if classLevelOf(c, "fighter") != 1 || classLevelOf(c, "rogue") != 3 {
		t.Errorf("levels went wrong: %v", c.Classes)
	}
	if c.TotalLevel() != 4 {
		t.Errorf("total level = %d, want 4", c.TotalLevel())
	}
}

// Past 20th there is nothing to gain, and the request is refused rather than
// quietly capped.
func TestLevelUpStopsAtTwenty(t *testing.T) {
	rs := lvlRuleset(t)
	if _, _, err := LevelUp(rogue(20, "thief"), rs, &LevelUpRequest{Levels: 1}); err == nil {
		t.Fatal("levelling past 20 was allowed")
	}
	if _, _, err := LevelUp(rogue(18, "thief"), rs, &LevelUpRequest{Levels: 5}); err == nil {
		t.Fatal("a climb that overshoots 20 was allowed")
	}
}

// The flow is replayed from the answers every call, so the same answers have
// to produce the same character however many times they are sent.
func TestLevelUpIsDeterministic(t *testing.T) {
	rs := lvlRuleset(t)
	answers := map[string][]string{
		"hp-rogue-3":     {"roll"},
		"subclass-rogue": {"thief"},
		"hp-rogue-4":     {"roll"},
		"asi-rogue-4":    {"dex", "con"},
	}
	_, first, err := LevelUp(rogue(2, ""), rs, &LevelUpRequest{Levels: 2, Seed: 12345, Answers: answers})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		_, again, err := LevelUp(rogue(2, ""), rs, &LevelUpRequest{Levels: 2, Seed: 12345, Answers: answers})
		if err != nil {
			t.Fatal(err)
		}
		if again.HPMax != first.HPMax {
			t.Fatalf("replay %d rolled different hit points: %d then %d",
				i, first.HPMax, again.HPMax)
		}
	}
}

// A choice from a level the character already had, which the sheet never
// recorded, is reported rather than asked - levelling up is not the moment to
// interrogate somebody about their first level.
func TestLevelUpReportsRatherThanAsksAboutOldChoices(t *testing.T) {
	rs := lvlRuleset(t)
	start := rogue(3, "thief")
	// Rogues choose expertise at 1st, and this sheet has not recorded it.
	delete(start.Choices, "rogue-expertise")
	asked := []string{}
	plan, _ := walk(t, start, rs, &LevelUpRequest{Levels: 1, Seed: 1},
		func(p *Prompt) []string {
			asked = append(asked, p.Step)
			if p.Step == "asi-rogue-4" {
				return []string{"dex"}
			}
			return []string{"average"}
		})
	for _, step := range asked {
		if step == "choice-rogue-expertise" {
			t.Error("levelling up asked about a choice from 1st level")
		}
	}
	if len(plan.Unrecorded) == 0 {
		t.Error("the unrecorded expertise was not reported either")
	}
}

// Levelling one class must not ask about another one's choices. A rogue
// taking their first level of wizard was being asked for the rogue's
// expertise, because that choice opens at 1st level and the wizard level was
// also a 1st - so the two levels were being confused for each other.
func TestLevelUpAsksOnlyTheLevelledClassChoices(t *testing.T) {
	rs := lvlRuleset(t)
	start := rogue(5, "thief")
	delete(start.Choices, "rogue-expertise")
	asked := []string{}
	_, c := walk(t, start, rs, &LevelUpRequest{Levels: 1, Class: "wizard", Seed: 1},
		func(p *Prompt) []string {
			asked = append(asked, p.Step)
			return firstValid(p)
		})
	for _, step := range asked {
		if step == "choice-rogue-expertise" {
			t.Errorf("a wizard level asked for the rogue's expertise: %v", asked)
		}
	}
	if classLevelIn(c, "wizard") != 1 {
		t.Errorf("wizard level = %d, want 1", classLevelIn(c, "wizard"))
	}
	if classLevelIn(c, "rogue") != 5 {
		t.Errorf("the rogue levels moved: %d", classLevelIn(c, "rogue"))
	}
}
