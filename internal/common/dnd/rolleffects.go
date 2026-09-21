//lint:file-ignore ST1006 allow the use of self
package dnd

/* SDOC: DnD
* What A Condition Does To A Roll

  A character sheet that knows you are poisoned and then rolls your attack
  flat is not much of a character sheet. Every condition in the book is
  already written down on the character - it has a catalog, a picker, a
  history and a colour it paints the page - and until now none of that
  reached the dice.

  So the conditions are asked before every d20. Poisoned and prone take the
  attack roll down, exhaustion takes the checks down and then, at the third
  level, the attacks and saves with them, restrained takes Dexterity saves
  down in particular, invisible puts the attack roll up, and the four that
  leave you helpless - paralyzed, petrified, stunned, unconscious - fail your
  Strength and Dexterity saves outright.

  Three things are worth knowing about how it is done:

  1. *The rules live here, not in the page.* The sheet is handed a small map
     of what each kind of roll is owed and draws it; it never knows what
     "poisoned" means. A ruleset module that adds a condition adds its rules
     to [[conditionRules]] and the sheet needs no changes at all.

  2. *Advantage and disadvantage cancel.* That is the rule as written - one
     of each and you roll flat, however many of each you have - and it is
     surprisingly easy to get wrong by counting. The card says so when it
     happens, because a player who knows they are poisoned and sees a flat
     roll deserves to be told why.

  3. *Nothing is forced.* The sheet rolls two d20 for every check and shows
     the flat roll, the better and the worse; all a condition does is decide
     which of the three stands. The player can still click another, because
     the caveats are real - frightened only bites while the thing you are
     frightened of is in sight, and no character sheet can know that.

  What the rules text says but this cannot judge is carried as a note rather
  than applied: blinded fails checks that need sight, and only the table
  knows whether this one did.
EDOC */

import (
	"sort"
	"strings"
)

// The kinds of d20 roll a condition can have an opinion about. A damage roll
// is not among them: no condition in the book touches one.
const (
	RollAttack = "attack"
	RollCheck  = "check"
	RollSave   = "save"
)

// Which way an effect leans a roll. A note neither helps nor hurts - it is
// something the rules say that only the table can decide.
const (
	LeanAdvantage    = "advantage"
	LeanDisadvantage = "disadvantage"
	LeanFail         = "fail"
	LeanNote         = "note"
)

// conditionRule is one thing one condition does to one kind of roll.
type conditionRule struct {
	Roll string
	Lean string
	// Abilities narrows the rule to those abilities, and is empty for a rule
	// that applies to every roll of its kind.
	Abilities []string
	// Says is the rule in the book's own words, which is what the card shows.
	Says string
	// When is the caveat the rules put on it, and is why nothing is forced.
	When string
	// Level is the exhaustion level this rule starts at. Exhaustion is
	// cumulative, so a rule at level 3 is in force at 4, 5 and 6 as well.
	Level int
}

// conditionRules is what each condition does, by its id in the catalog - see
// SRDConditions in conditions.go. A condition that does nothing to a d20 is
// simply absent: grappled sets your speed to zero and has no opinion about
// dice.
var conditionRules = map[string][]conditionRule{
	"blinded": {
		{Roll: RollAttack, Lean: LeanDisadvantage, Says: "attack rolls have disadvantage"},
		{Roll: RollCheck, Lean: LeanNote, Says: "a check that needs sight fails automatically"},
	},
	"charmed": {
		{Roll: RollAttack, Lean: LeanNote, Says: "you can't attack the charmer"},
	},
	"deafened": {
		{Roll: RollCheck, Lean: LeanNote, Says: "a check that needs hearing fails automatically"},
	},
	"exhaustion": {
		{Roll: RollCheck, Lean: LeanDisadvantage, Level: 1,
			Says: "ability checks have disadvantage"},
		{Roll: RollAttack, Lean: LeanDisadvantage, Level: 3,
			Says: "attack rolls have disadvantage"},
		{Roll: RollSave, Lean: LeanDisadvantage, Level: 3,
			Says: "saving throws have disadvantage"},
	},
	"frightened": {
		{Roll: RollAttack, Lean: LeanDisadvantage, Says: "attack rolls have disadvantage",
			When: "while the source of your fear is in sight"},
		{Roll: RollCheck, Lean: LeanDisadvantage, Says: "ability checks have disadvantage",
			When: "while the source of your fear is in sight"},
	},
	"incapacitated": {
		{Roll: RollAttack, Lean: LeanNote, Says: "you can't take actions or reactions"},
	},
	"invisible": {
		{Roll: RollAttack, Lean: LeanAdvantage, Says: "attack rolls have advantage"},
	},
	"paralyzed": {
		{Roll: RollAttack, Lean: LeanNote, Says: "you can't take actions or reactions"},
		{Roll: RollSave, Lean: LeanFail, Abilities: []string{STR, DEX},
			Says: "Strength and Dexterity saves fail automatically"},
	},
	"petrified": {
		{Roll: RollAttack, Lean: LeanNote, Says: "you can't take actions or reactions"},
		{Roll: RollSave, Lean: LeanFail, Abilities: []string{STR, DEX},
			Says: "Strength and Dexterity saves fail automatically"},
	},
	"poisoned": {
		{Roll: RollAttack, Lean: LeanDisadvantage, Says: "attack rolls have disadvantage"},
		{Roll: RollCheck, Lean: LeanDisadvantage, Says: "ability checks have disadvantage"},
	},
	"prone": {
		{Roll: RollAttack, Lean: LeanDisadvantage, Says: "attack rolls have disadvantage"},
	},
	"restrained": {
		{Roll: RollAttack, Lean: LeanDisadvantage, Says: "attack rolls have disadvantage"},
		{Roll: RollSave, Lean: LeanDisadvantage, Abilities: []string{DEX},
			Says: "Dexterity saves have disadvantage"},
	},
	"stunned": {
		{Roll: RollAttack, Lean: LeanNote, Says: "you can't take actions or reactions"},
		{Roll: RollSave, Lean: LeanFail, Abilities: []string{STR, DEX},
			Says: "Strength and Dexterity saves fail automatically"},
	},
	"unconscious": {
		{Roll: RollAttack, Lean: LeanNote, Says: "you can't take actions or reactions"},
		{Roll: RollSave, Lean: LeanFail, Abilities: []string{STR, DEX},
			Says: "Strength and Dexterity saves fail automatically"},
	},
}

// RollEffect is one condition's say in one roll, as the card prints it.
type RollEffect struct {
	Id        string `json:"id"`
	Condition string `json:"condition"`
	Lean      string `json:"lean"`
	Says      string `json:"says"`
	When      string `json:"when,omitempty"`
}

// RollAdvice is everything the conditions have to say about one roll.
type RollAdvice struct {
	// Lean is which of the three readings stands: "advantage",
	// "disadvantage", or empty for the flat roll.
	Lean string `json:"lean"`
	// Cancels is true when there was one of each. The rules say that is a
	// flat roll rather than a count, and a player looking at a flat roll
	// while poisoned is owed the explanation.
	Cancels bool `json:"cancels"`
	// Fails is true when the roll fails whatever it comes up.
	Fails bool `json:"fails"`
	// Effects is every condition that had something to say, notes included.
	Effects []RollEffect `json:"effects"`
	// Why is the one line the roll card carries.
	Why string `json:"why"`
	// Short is the same thing at a glance, for the mark that follows the
	// mouse over a rollable: "Poisoned", or "Poisoned +1".
	Short string `json:"short"`
}

// Any reports whether there is anything at all to say.
func (a RollAdvice) Any() bool { return len(a.Effects) > 0 }

// RollAdviceView is every roll the sheet can make, worked out once. Checks and
// saves are keyed by ability id, plus an empty key for a roll that names no
// ability - the sheet looks up what it has and falls back to that.
type RollAdviceView struct {
	Any     bool                  `json:"any"`
	Attack  RollAdvice            `json:"attack"`
	Check   map[string]RollAdvice `json:"check"`
	Save    map[string]RollAdvice `json:"save"`
	Summary string                `json:"summary"`
}

// ComputeRollAdvice works out what the character's conditions do to every kind
// of roll they can make. It is a handful of map lookups over at most fifteen
// conditions, and is recomputed with the rest of the sheet rather than cached.
func ComputeRollAdvice(c *Character, rs *Ruleset) RollAdviceView {
	out := RollAdviceView{
		Check: map[string]RollAdvice{},
		Save:  map[string]RollAdvice{},
	}
	out.Attack = adviceFor(c, rs, RollAttack, "")
	for _, ab := range append([]string{""}, AbilityOrder...) {
		out.Check[ab] = adviceFor(c, rs, RollCheck, ab)
		out.Save[ab] = adviceFor(c, rs, RollSave, ab)
	}
	out.Any = out.Attack.Any()
	for _, m := range []map[string]RollAdvice{out.Check, out.Save} {
		for _, a := range m {
			if a.Any() {
				out.Any = true
			}
		}
	}
	if out.Any {
		out.Summary = strings.TrimSpace(ComputeConditions(c, rs).Summary)
	}
	return out
}

// adviceFor is the advice for one kind of roll, optionally of one ability.
func adviceFor(c *Character, rs *Ruleset, kind, ability string) RollAdvice {
	a := RollAdvice{Effects: []RollEffect{}}
	if c == nil {
		return a
	}
	// Walk the conditions the character is actually under, in the order they
	// are stored, so the card reads the same way twice running.
	ids := make([]string, 0, len(c.Conditions))
	for _, ref := range c.Conditions {
		ids = append(ids, ref.Id)
	}
	sort.Strings(ids)
	up, down := false, false
	for _, id := range ids {
		rules, ok := conditionRules[id]
		if !ok {
			continue
		}
		level := c.ConditionLevel(id)
		for _, r := range rules {
			if r.Roll != kind {
				continue
			}
			// A levelled rule only bites from its level upwards, and
			// exhaustion is cumulative so everything below it bites too.
			if r.Level > 0 && level < r.Level {
				continue
			}
			// A rule about particular abilities says nothing about the
			// others - and says nothing at all about a roll that has not
			// named one, since it might be either.
			if len(r.Abilities) > 0 && ability != "" && !hasAbility(r.Abilities, ability) {
				continue
			}
			a.Effects = append(a.Effects, RollEffect{
				Id: id, Condition: conditionLabel(rs, id, level),
				Lean: r.Lean, Says: r.Says, When: r.When,
			})
			switch r.Lean {
			case LeanAdvantage:
				up = true
			case LeanDisadvantage:
				down = true
			case LeanFail:
				// Only a roll that has named the ability actually fails;
				// otherwise this is a warning that it might.
				if ability != "" {
					a.Fails = true
				}
			}
		}
	}
	switch {
	case up && down:
		// One of each is neither, however many of each there are.
		a.Cancels = true
	case up:
		a.Lean = LeanAdvantage
	case down:
		a.Lean = LeanDisadvantage
	}
	a.Why, a.Short = adviceWords(a)
	return a
}

func hasAbility(list []string, ability string) bool {
	for _, a := range list {
		if a == ability {
			return true
		}
	}
	return false
}

// conditionLabel is what to call a condition on the card. Only a condition
// the catalog says has levels wears one - ConditionLevel answers 1 for
// anything that is merely on, and "Poisoned 1" would be nonsense.
func conditionLabel(rs *Ruleset, id string, level int) string {
	name := conditionName(rs, id)
	if cd := rs.Condition(id); cd != nil && cd.Levels > 0 && level > 0 {
		return name + " " + itoa(level)
	}
	return name
}

// adviceWords writes the line the roll card carries and the shorter one the
// mark by the mouse does.
func adviceWords(a RollAdvice) (string, string) {
	if len(a.Effects) == 0 {
		return "", ""
	}
	// The conditions that actually moved the roll get named first and get
	// the short line to themselves; a note is worth printing but is not what
	// the mark by the mouse is warning about.
	named := []string{}
	seen := map[string]bool{}
	for _, e := range a.Effects {
		if e.Lean == LeanNote || seen[e.Condition] {
			continue
		}
		seen[e.Condition] = true
		named = append(named, e.Condition)
	}
	short := ""
	if len(named) > 0 {
		short = named[0]
		if len(named) > 1 {
			short += " +" + itoa(len(named)-1)
		}
	}

	head := ""
	switch {
	case a.Fails:
		head = "Fails automatically"
	case a.Cancels:
		head = "Advantage and disadvantage cancel"
	case a.Lean == LeanAdvantage:
		head = "Advantage"
	case a.Lean == LeanDisadvantage:
		head = "Disadvantage"
	}
	// Every condition that had something to say, with its caveat, since the
	// caveat is the whole reason the player is allowed to overrule this.
	parts := []string{}
	for _, e := range a.Effects {
		part := e.Condition + ": " + e.Says
		if e.When != "" {
			part += " " + e.When
		}
		parts = append(parts, part)
	}
	line := strings.Join(parts, " · ")
	if head == "" {
		return line, short
	}
	return head + " — " + line, short
}
