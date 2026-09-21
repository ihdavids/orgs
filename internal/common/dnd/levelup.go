package dnd

/* SDOC: DnD
* Levelling Up

  A character who has earned a level does not need building again. =orgs dnd
  levelup= takes the sheet you already have, works out what the new level
  actually asks you to decide, asks only that, and writes the answers back
  into the same org file.

  #+BEGIN_SRC sh
  orgs dnd levelup -file lyra.org              # one level
  orgs dnd levelup -file lyra.org -levels 3    # three at once
  orgs dnd levelup -file lyra.org -class rogue # multiclass, or take a new class
  #+END_SRC

  What it asks about, and nothing else:

  - *hit points*, every level - the average, a roll, or a number you type in
    because you rolled it at the table,
  - *a subclass*, when the level you are entering is the one your class picks
    at and you have not picked one,
  - *an ability score improvement*, at the levels your class grants one, with
    every feat offered as the alternative,
  - *anything your class or subclass chooses* that has come open at the new
    level - another invocation, a metamagic, a fighting style, expertise,
  - *new spells and cantrips*, when the level buys more of them.

  Several levels at once are taken one at a time, in order, so a choice that
  only exists because of an earlier level is asked at the right moment: a
  rogue going 2 to 4 is asked for their subclass at 3 and their improvement at
  4, in that order.

  Nothing else on the sheet is touched. Your equipment, your coin, your notes
  and your backstory are exactly where you left them - only the property
  drawer and the sections the rules engine regenerates change.

  The flow is stateless: each answer is sent back with the ones before it, so
  there is no session to expire and stopping halfway costs you nothing. Any
  dice it rolls for you come off the seed in the request, so the same answers
  always produce the same character.
EDOC */

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"sort"
	"strconv"
	"strings"
)

// LevelUpRequest is one pass at levelling a character. Answers accumulate
// between calls, so the caller resends everything it has answered so far and
// the engine replays them - there is no session state anywhere.
type LevelUpRequest struct {
	// Filename is the character's org sheet.
	Filename string `json:"filename"`
	// Class is the class gaining the levels. Empty means the character's
	// primary class, which is what levelling usually means. A class the
	// character does not have yet is a multiclass and is added.
	Class string `json:"class"`
	// Levels is how many levels to gain, at least 1.
	Levels int `json:"levels"`
	// Answers are the answers given so far, keyed by the step id they answer.
	Answers map[string][]string `json:"answers"`
	// Seed makes any dice the flow rolls repeatable, so replaying the same
	// answers rebuilds the same character. Zero asks for a fresh one, which
	// the response reports back so the caller can keep sending it.
	Seed int64 `json:"seed"`
}

// LevelUpPlan is what the engine says about a request: what it still wants to
// know, and what the levels have come to once it knows everything.
type LevelUpPlan struct {
	Character string `json:"character"`
	Class     string `json:"class"`
	ClassName string `json:"className"`
	// FromLevel and ToLevel are the character's total level either side.
	FromLevel int `json:"fromLevel"`
	ToLevel   int `json:"toLevel"`
	// Next is the question still to be answered, or nil when there are none.
	Next *Prompt `json:"next,omitempty"`
	// Asked is every step the climb needs, answered or not, so a caller can
	// show how far along it is.
	Asked []string `json:"asked"`
	Done  bool     `json:"done"`
	// Gained is what the levels came to, in the words a player wants read
	// back: "hit points 38 to 45", "Path of the Totem Warrior".
	Gained []string `json:"gained"`
	// Unrecorded are choices from levels the character already had that the
	// sheet never wrote down - common on an imported character. They are not
	// asked about, because they are not what this level earned, but they are
	// said out loud so they are not lost.
	Unrecorded []string `json:"unrecorded,omitempty"`
	Seed       int64    `json:"seed"`
	Error      string   `json:"error,omitempty"`
}

// levelUpStep is one question in the climb, and what answering it does.
type levelUpStep struct {
	id    string
	build func() *Prompt
	apply func(values []string) error
	// auto answers the step without asking when there is only one sane
	// answer. It returns nil to ask after all.
	auto func() []string
}

// LevelUp walks a character up the levels the request asks for, applying
// every answer it has been given and stopping at the first question it has
// not. It returns the plan and the levelled character.
//
// The character handed in is never modified. That matters because the flow is
// stateless: a client answers one question at a time and resends every answer
// each call, so LevelUp is called over and over with the same starting
// character and a longer list of answers. Levelling in place would add the
// levels again on every call.
//
// The walk is one level at a time on purpose. A choice can exist only because
// of an earlier level - a subclass picked at 3rd opens choices of its own -
// so the steps for level N+1 are worked out against the character as it
// stands after level N, never against the character that started.
func LevelUp(start *Character, rs *Ruleset, req *LevelUpRequest) (*LevelUpPlan, *Character, error) {
	if start == nil || rs == nil {
		return nil, nil, fmt.Errorf("no character or ruleset to level up")
	}
	c, err := copyCharacter(start)
	if err != nil {
		return nil, nil, err
	}
	levels := req.Levels
	if levels < 1 {
		levels = 1
	}
	seed := req.Seed
	if seed == 0 {
		seed = rand.Int63()
	}
	answers := req.Answers
	if answers == nil {
		answers = map[string][]string{}
	}

	classId := strings.TrimSpace(req.Class)
	if classId == "" {
		classId = c.PrimaryClass().Class
	}
	classId = Slugify(classId)
	cls := rs.Class(classId)
	if cls == nil {
		return nil, nil, fmt.Errorf("unknown class %q", req.Class)
	}

	plan := &LevelUpPlan{
		Character: c.Name, Class: classId, ClassName: cls.Name,
		FromLevel: c.TotalLevel(), Seed: seed, Gained: []string{},
		Asked: []string{},
	}
	if c.TotalLevel()+levels > 20 {
		return nil, nil, fmt.Errorf(
			"%s is level %d, and %d more would pass 20", c.Name, c.TotalLevel(), levels)
	}
	if c.Choices == nil {
		c.Choices = map[string][]string{}
	}

	hpBefore := c.HPMax
	for i := 0; i < levels; i++ {
		newLevel := classLevelIn(c, classId) + 1
		// The class level goes up first: everything the level grants is
		// worked out against the character who already has it.
		raiseClassLevel(c, classId, newLevel)
		plan.ToLevel = c.TotalLevel()

		for _, st := range levelUpSteps(c, rs, cls, newLevel, seed) {
			plan.Asked = append(plan.Asked, st.id)
			values, answered := answers[st.id]
			if !answered && st.auto != nil {
				values, answered = st.auto(), true
			}
			if !answered {
				p := st.build()
				p.Step = st.id
				p.Progress = Progress{Step: len(plan.Asked), Total: 0}
				plan.Next = p
				return plan, c, nil
			}
			if err := st.apply(values); err != nil {
				p := st.build()
				p.Step = st.id
				p.Error = err.Error()
				plan.Next = p
				return plan, c, nil
			}
		}
	}
	plan.Done = true
	if c.HPMax != hpBefore {
		plan.Gained = append(plan.Gained,
			fmt.Sprintf("hit points %d to %d", hpBefore, c.HPMax))
	}
	plan.Gained = append(plan.Gained, levelUpGains(c, rs, classId)...)
	plan.Unrecorded = unrecordedChoices(c, rs, classId)
	return plan, c, nil
}

// copyCharacter is a character that can be levelled without touching the one
// it came from. It goes through json because the character is entirely json
// tagged already, which means a field added later is copied without anybody
// having to remember to add it here.
func copyCharacter(c *Character) (*Character, error) {
	data, err := json.Marshal(c)
	if err != nil {
		return nil, fmt.Errorf("could not read the character: %s", err)
	}
	out := &Character{}
	if err := json.Unmarshal(data, out); err != nil {
		return nil, fmt.Errorf("could not copy the character: %s", err)
	}
	if out.Choices == nil {
		out.Choices = map[string][]string{}
	}
	if out.Abilities == nil {
		out.Abilities = map[string]int{}
	}
	return out, nil
}

// classChoices are the choices that belong to one class and the subclass of
// it this character took - and nothing else.
//
// It is deliberately narrower than collectChoices, which gathers everything a
// character chooses from every source they have. Levelling one class must not
// ask about another one's: a rogue taking a level of wizard was being asked
// for the rogue's expertise, because that choice opens at 1st level and the
// wizard level was also a 1st. Counting it against the wrong class's level
// would have been wrong even when it did not ask.
func classChoices(rs *Ruleset, c *Character, cls *Class) []Choice {
	out := append([]Choice{}, cls.Choices...)
	if sub := subclassOf(c, cls.Id); sub != "" {
		if sc := rs.Subclass(cls.Id, sub); sc != nil {
			out = append(out, sc.Choices...)
		}
	}
	return out
}

// classLevelIn is how many levels the character has in one class, and zero
// when they have none.
//
// It is deliberately not classLevelOf, which answers with the character's
// whole level when they do not have the class at all. That is the right answer
// while building a character, where the class being asked about is the one
// they are - and the wrong one here, where a class they have never taken is
// exactly the case that means multiclassing into it at level 1.
func classLevelIn(c *Character, classId string) int {
	for _, cl := range c.Classes {
		if cl.Class == classId {
			return cl.Level
		}
	}
	return 0
}

// raiseClassLevel sets a class's level, adding the class when the character
// does not have it yet - which is what multiclassing into something new is.
func raiseClassLevel(c *Character, classId string, level int) {
	for i := range c.Classes {
		if c.Classes[i].Class == classId {
			c.Classes[i].Level = level
			return
		}
	}
	c.Classes = append(c.Classes, ClassLevel{Class: classId, Level: level})
}

// levelUpSteps is everything one new level asks, in the order it asks it.
func levelUpSteps(c *Character, rs *Ruleset, cls *Class, level int, seed int64) []levelUpStep {
	steps := []levelUpStep{stepLevelHP(c, rs, cls, level, seed)}

	// The subclass, at the level the class picks one and not before.
	if len(cls.Subclasses) > 0 && level >= maxInt(cls.SubclassLevel, 1) &&
		subclassOf(c, cls.Id) == "" {
		steps = append(steps, stepLevelSubclass(c, cls, level))
	}
	// The improvement, at the levels that grant one.
	for _, l := range cls.ASILevels {
		if l == level {
			steps = append(steps, stepLevelASI(c, rs, cls, level))
		}
	}
	// Anything the class or subclass chooses that this level has opened, or
	// bought more of.
	//
	// Only this level's worth. A choice from an earlier level that was never
	// written down - a feat's pick on an imported sheet, say - is a gap in
	// the sheet rather than something the new level asks for, and levelling
	// up is not the moment to interrogate somebody about their first level.
	// Those are reported at the end instead, so they are not lost either.
	for _, ch := range classChoices(rs, c, cls) {
		if ch.Level > level {
			continue
		}
		want := choiceCount(ch, level)
		if want <= len(c.Choices[ch.Id]) {
			continue
		}
		opened := ch.Level == level
		grew := want > choiceCount(ch, level-1)
		if !opened && !grew {
			continue
		}
		// A choice with nothing left to offer is not a question. Expertise on
		// a character with no skills left to double is the usual case, and
		// asking it would be a prompt with an empty list that no answer can
		// satisfy.
		if len(availableChoices(rs, c, ch)) == 0 {
			continue
		}
		steps = append(steps, stepLevelChoice(c, rs, ch, want))
	}
	// More spells, and more cantrips.
	if sc := castingInfo(rs, c); sc != nil {
		if n := cantripCount(rs, c, sc); n > countKnownSpells(c, 0) {
			steps = append(steps, stepLevelSpells(c, rs, 0, n, "cantrip"))
		}
		if n, kind := spellCount(rs, c, sc); n > 0 && kind == "known" &&
			n > countKnownSpells(c, -1) {
			steps = append(steps, stepLevelSpells(c, rs, -1, n, "spell"))
		}
	}
	return steps
}

// ---------------------------------------------------------------- the steps

// stepLevelHP is the one question every level asks. The average is offered
// first because it is what most tables use and it is what the rules suggest
// for a character who would rather not risk a 1.
func stepLevelHP(c *Character, rs *Ruleset, cls *Class, level int, seed int64) levelUpStep {
	id := fmt.Sprintf("hp-%s-%d", cls.Id, level)
	die := cls.HitDie
	if die <= 0 {
		die = 8
	}
	// The average a hit die is taken at is half of it rounded up, which for a
	// d8 is 5 and for a d10 is 6.
	avg := die/2 + 1
	conMod := AbilityMod(c.Abilities["con"])
	gain := func(roll int) int {
		n := roll + conMod
		// A level never costs you hit points, however bad your Constitution.
		if n < 1 {
			n = 1
		}
		return n
	}
	return levelUpStep{
		id: id,
		build: func() *Prompt {
			return &Prompt{
				Kind:  "select",
				Title: fmt.Sprintf("Hit Points - %s level %d", cls.Name, level),
				Question: fmt.Sprintf(
					"Your hit die is a d%d and your Constitution modifier is %s. How do you gain hit points?",
					die, Signed(conMod)),
				Help: "The average is what most tables use and never costs you a level. " +
					"Rolling can do better or worse. If you rolled at the table, type the " +
					"number you rolled on the die itself.",
				Options: []Option{
					{Id: "average", Name: fmt.Sprintf("Average (+%d)", gain(avg)),
						Summary:     fmt.Sprintf("%d from the die %s from Constitution", avg, Signed(conMod)),
						Recommended: true},
					{Id: "roll", Name: fmt.Sprintf("Roll 1d%d", die),
						Summary: "let the dice decide, between " +
							strconv.Itoa(gain(1)) + " and " + strconv.Itoa(gain(die))},
					{Id: "max", Name: fmt.Sprintf("Maximum (+%d)", gain(die)),
						Summary: "some tables give the maximum"},
				},
				Default: "average", Min: 1, Max: 1, AllowCustom: true,
			}
		},
		apply: func(values []string) error {
			if len(values) == 0 {
				return fmt.Errorf("choose average, roll, maximum, or type the number you rolled")
			}
			v := strings.ToLower(strings.TrimSpace(values[0]))
			roll := 0
			switch v {
			case "average", "avg", "":
				roll = avg
			case "max", "maximum":
				roll = die
			case "roll":
				// Seeded off the step id, so the same request always rolls the
				// same level the same way however many times it is replayed.
				r := rand.New(rand.NewSource(seed + int64(len(id)) + int64(level)*7919))
				roll = r.Intn(die) + 1
			default:
				n, err := strconv.Atoi(strings.TrimSpace(values[0]))
				if err != nil {
					return fmt.Errorf(
						"%q is not average, roll, maximum, or a number rolled on the die", values[0])
				}
				if n < 1 || n > die {
					return fmt.Errorf("a d%d rolls between 1 and %d, not %d", die, die, n)
				}
				roll = n
			}
			c.HPMax += gain(roll)
			c.HPCurrent += gain(roll)
			return nil
		},
	}
}

func stepLevelSubclass(c *Character, cls *Class, level int) levelUpStep {
	id := fmt.Sprintf("subclass-%s", cls.Id)
	name := cls.SubclassLabel
	if name == "" {
		name = "Subclass"
	}
	return levelUpStep{
		id: id,
		build: func() *Prompt {
			opts := []Option{}
			for i := range cls.Subclasses {
				sc := &cls.Subclasses[i]
				opts = append(opts, Option{Id: sc.Id, Name: sc.Name,
					Summary: sc.Summary, Detail: sc.Text})
			}
			return &Prompt{
				Kind: "select", Title: name,
				Question: fmt.Sprintf("%s level %d: choose your %s.",
					cls.Name, level, strings.ToLower(name)),
				Help:    "This is the one choice you cannot change later without your DM's say so.",
				Options: opts, Min: 1, Max: 1, AllowRandom: true,
			}
		},
		apply: func(values []string) error {
			if len(values) != 1 {
				return fmt.Errorf("choose one %s", strings.ToLower(name))
			}
			want := Slugify(values[0])
			for i := range cls.Subclasses {
				if cls.Subclasses[i].Id == want {
					setSubclass(c, cls.Id, want)
					return nil
				}
			}
			return fmt.Errorf("%q is not a %s of the %s", values[0],
				strings.ToLower(name), cls.Name)
		},
	}
}

// stepLevelASI is the improvement, offered exactly as the builder offers it so
// that a character levelled here and one built from scratch record the same
// answer in the same place.
func stepLevelASI(c *Character, rs *Ruleset, cls *Class, level int) levelUpStep {
	id := fmt.Sprintf("asi-%s-%d", cls.Id, level)
	return levelUpStep{
		id: id,
		build: func() *Prompt {
			opts := []Option{}
			for _, a := range AbilityOrder {
				score := c.Abilities[a]
				o := Option{Id: a, Name: AbilityNames[a],
					Summary: fmt.Sprintf("currently %d (%s)", score, Signed(AbilityMod(score)))}
				switch {
				case score >= 20:
					o.Disabled = true
					o.Reason = "already at the maximum of 20"
				case score == 19:
					// It can still take the +1 half of a pair, so it is not
					// disabled - but on its own it is +2 and would overshoot,
					// which is worth saying before it is picked rather than
					// after.
					o.Reason = "at 19: pair it with another for +1, it cannot take +2"
				case containsStr(cls.PrimaryAbility, a):
					o.Recommended = true
					o.Reason = "your primary ability"
				}
				opts = append(opts, o)
			}
			for i := range rs.Feats {
				ft := &rs.Feats[i]
				if containsStr(c.Feats, ft.Id) {
					continue
				}
				o := Option{Id: featOptionPrefix + ft.Id, Name: ft.Name,
					Summary: firstSentence(ft.Text), Detail: ft.Text}
				if ft.Prerequisite != "" {
					o.Summary = "Requires " + ft.Prerequisite + ". " + o.Summary
				}
				opts = append(opts, o)
			}
			return &Prompt{
				Kind: "multiselect", Title: "Ability Score Improvement",
				Question: fmt.Sprintf(
					"%s level %d: raise one ability by 2, raise two abilities by 1, or take a feat.",
					cls.Name, level),
				Help: "Pick one ability for +2, or two different abilities for +1 each, or a " +
					"single feat instead. No score can go above 20.",
				Options: opts, Min: 1, Max: 2, AllowRandom: true,
			}
		},
		apply: func(values []string) error {
			if len(values) == 0 || len(values) > 2 {
				return fmt.Errorf("choose one ability (+2), two abilities (+1 each), or one feat")
			}
			if strings.HasPrefix(values[0], featOptionPrefix) {
				if len(values) != 1 {
					return fmt.Errorf("a feat takes the whole improvement, choose it on its own")
				}
				featId := strings.TrimPrefix(values[0], featOptionPrefix)
				ft := rs.Feat(featId)
				if ft == nil {
					return fmt.Errorf("unknown feat %q", featId)
				}
				for ab, v := range ft.AbilityBonuses {
					if _, ok := AbilityNames[ab]; !ok {
						return fmt.Errorf("feat %q has unknown ability %q", featId, ab)
					}
					if c.Abilities[ab]+v > 20 {
						return fmt.Errorf("%s cannot go above 20", AbilityNames[ab])
					}
				}
				for ab, v := range ft.AbilityBonuses {
					c.Abilities[ab] += v
				}
				c.Feats = addUnique(c.Feats, featId)
				c.Choices[id] = values
				return nil
			}
			bump := 2
			if len(values) == 2 {
				bump = 1
			}
			for _, ab := range values {
				if _, ok := AbilityNames[ab]; !ok {
					return fmt.Errorf("unknown ability %q", ab)
				}
				if c.Abilities[ab]+bump > 20 {
					return fmt.Errorf("%s cannot go above 20", AbilityNames[ab])
				}
			}
			for _, ab := range values {
				c.Abilities[ab] += bump
			}
			c.Choices[id] = values
			return nil
		},
	}
}

// stepLevelChoice asks for whatever a class or subclass chooses that has come
// open at this level, or grown: another invocation, another metamagic, the
// fighting style a subclass hands out at 3rd.
func stepLevelChoice(c *Character, rs *Ruleset, ch Choice, want int) levelUpStep {
	had := append([]string{}, c.Choices[ch.Id]...)
	need := want - len(had)
	return levelUpStep{
		id: "choice-" + ch.Id,
		build: func() *Prompt {
			opts := availableChoices(rs, c, ch)
			// Never ask for more than there is to pick from.
			if need > len(opts) {
				need = len(opts)
			}
			q := ch.Prompt
			if q == "" {
				q = fmt.Sprintf("Choose %d %s.", need, ch.Name)
			}
			if len(had) > 0 {
				q += fmt.Sprintf(" You already have %s.", strings.Join(had, ", "))
			}
			kind := "multiselect"
			if need == 1 {
				kind = "select"
			}
			return &Prompt{
				Kind: kind, Title: ch.Name, Question: q, Help: ch.Help,
				Options: opts, Min: need, Max: need, AllowRandom: true,
			}
		},
		apply: func(values []string) error {
			if len(values) != need {
				return fmt.Errorf("choose exactly %d", need)
			}
			for _, v := range values {
				if containsStr(had, v) {
					return fmt.Errorf("you already have %q", v)
				}
			}
			c.Choices[ch.Id] = append(had, values...)
			return nil
		},
	}
}

// unrecordedChoices are choices the character is owed from levels they
// already had, which the sheet does not record. Levelling up does not ask
// about them - see levelUpSteps - but it does say they are there.
func unrecordedChoices(c *Character, rs *Ruleset, classId string) []string {
	cls := rs.Class(classId)
	if cls == nil {
		return nil
	}
	out := []string{}
	for _, ch := range classChoices(rs, c, cls) {
		want := choiceCount(ch, classLevelIn(c, classId))
		short := want - len(c.Choices[ch.Id])
		if short <= 0 || len(availableChoices(rs, c, ch)) == 0 {
			continue
		}
		out = append(out, fmt.Sprintf("%s: %d still to choose",
			orDefault(ch.Name, Titleize(ch.Id)), short))
	}
	sort.Strings(out)
	return out
}

// availableChoices is what a choice still has to offer this character, which
// is its options less whatever has already been taken.
func availableChoices(rs *Ruleset, c *Character, ch Choice) []Option {
	had := c.Choices[ch.Id]
	opts := []Option{}
	for _, o := range choiceOptions(rs, c, ch) {
		if containsStr(had, o.Id) || o.Disabled {
			continue
		}
		opts = append(opts, o)
	}
	return opts
}

// stepLevelSpells asks for the spells a level buys, for a class that knows a
// fixed list. A class that prepares from the whole list is not asked: its new
// allowance is simply larger, and what it prepares is a daily decision rather
// than a thing to write down at level up.
func stepLevelSpells(c *Character, rs *Ruleset, level, want int, what string) levelUpStep {
	id := fmt.Sprintf("spells-%d", level)
	known := []string{}
	for _, ks := range c.Spells {
		if level == 0 && ks.Level == 0 {
			known = append(known, ks.Id)
		}
		if level < 0 && ks.Level > 0 {
			known = append(known, ks.Id)
		}
	}
	need := want - len(known)
	return levelUpStep{
		id: id,
		build: func() *Prompt {
			// The same list the creation flow offers, so a spell that could
			// not be learned while building cannot be learned by levelling.
			cls := castingClass(rs, c)
			listId := ""
			if cls != nil {
				listId = cls.Id
			}
			if sc := castingInfo(rs, c); sc != nil && sc.SpellList != "" {
				listId = sc.SpellList
			}
			maxLvl := maxSpellLevel(rs, c)
			opts := []Option{}
			for _, sp := range rs.SpellsForClass(listId, -1) {
				if level == 0 && sp.Level != 0 {
					continue
				}
				if level != 0 && (sp.Level == 0 || sp.Level > maxLvl) {
					continue
				}
				if hasSpell(c.Spells, sp.Id) {
					continue
				}
				opts = append(opts, spellOption(sp))
			}
			title := "New Cantrips"
			if level != 0 {
				title = "New Spells"
			}
			return &Prompt{
				Kind: "multiselect", Title: title,
				Question: fmt.Sprintf("You learn %d new %s.",
					need, plural(need, what, what+"s")),
				Help:    "Only what your class can learn at this level is offered.",
				Options: opts, Min: need, Max: need, AllowRandom: true,
			}
		},
		apply: func(values []string) error {
			if len(values) != need {
				return fmt.Errorf("choose exactly %d %s", need, plural(need, what, what+"s"))
			}
			for _, v := range values {
				sp := rs.Spell(v)
				if sp == nil {
					return fmt.Errorf("unknown spell %q", v)
				}
				c.Spells = appendSpell(c.Spells, rs, sp.Id, "class")
			}
			sort.SliceStable(c.Spells, func(i, j int) bool {
				if c.Spells[i].Level != c.Spells[j].Level {
					return c.Spells[i].Level < c.Spells[j].Level
				}
				return c.Spells[i].Name < c.Spells[j].Name
			})
			return nil
		},
	}
}

// ---------------------------------------------------------------- helpers

func subclassOf(c *Character, classId string) string {
	for _, cl := range c.Classes {
		if cl.Class == classId {
			return cl.Subclass
		}
	}
	return ""
}

func setSubclass(c *Character, classId, subclass string) {
	for i := range c.Classes {
		if c.Classes[i].Class == classId {
			c.Classes[i].Subclass = subclass
			return
		}
	}
}

func countKnownSpells(c *Character, level int) int {
	n := 0
	for _, ks := range c.Spells {
		if level == 0 && ks.Level == 0 {
			n++
		}
		if level < 0 && ks.Level > 0 {
			n++
		}
	}
	return n
}

// levelUpGains is what to read back to the player once the climb is done.
func levelUpGains(c *Character, rs *Ruleset, classId string) []string {
	out := []string{}
	cls := rs.Class(classId)
	if cls == nil {
		return out
	}
	lvl := classLevelIn(c, classId)
	if sub := subclassOf(c, classId); sub != "" {
		for i := range cls.Subclasses {
			if cls.Subclasses[i].Id == sub {
				out = append(out, cls.Subclasses[i].Name)
			}
		}
	}
	// The features the new level turned on, which are the thing a player most
	// wants to be told about and never has to choose.
	for _, f := range cls.Features {
		if f.Level == lvl {
			out = append(out, f.Name)
		}
	}
	if sub := subclassOf(c, classId); sub != "" {
		for i := range cls.Subclasses {
			if cls.Subclasses[i].Id != sub {
				continue
			}
			for _, f := range cls.Subclasses[i].Features {
				if f.Level == lvl {
					out = append(out, f.Name)
				}
			}
		}
	}
	return out
}
