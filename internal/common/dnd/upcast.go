package dnd

/* SDOC: DnD
* Casting A Spell At A Higher Level

  Most spells say what happens when they are cast from a bigger slot than
  they need: "when you cast this spell using a spell slot of 2nd level or
  higher, the healing increases by 1d4 for each slot level above 1st". That
  sentence is read the same way the rest of a cast is read - once, by the
  rules engine - into =SpellCast.Upcast=, one entry per slot level the
  character could cast it from.

  Each entry carries the whole cast at that level: the damage or healing
  already grown by the extra dice, the line of detail the roll history shows,
  and a note saying what the bigger slot bought. So the html sheet offers the
  levels beside the cast button, rolls the dice the chosen level asks for and
  spends a slot of exactly that level.

  What can be worked out is what the text actually states: dice or a flat
  amount added to damage or to healing, either for each slot level above the
  spell's own or for every two of them. A spell that spends its higher levels
  on something else - another target, a longer duration, one more dart - still
  offers the levels, because spending the bigger slot is a real choice, but it
  changes no dice and says so by quoting the spell instead.
EDOC */

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// SpellUpcast is one spell cast from one slot, at a level above its own.
type SpellUpcast struct {
	Level int    `json:"level"`
	Label string `json:"label"`

	// Damage and Heal are the dice at this level, already grown by whatever
	// the at-higher-levels text adds. They are empty when the spell has none.
	Damage string `json:"damage"`
	Heal   string `json:"heal"`
	// Damage2 is the spell's second damage line at this level, grown when it
	// is the one the bigger slot buys more of. See SpellCast.Damage2.
	Damage2 string `json:"damage2,omitempty"`

	// Note is what the bigger slot bought, in the few words the sheet has
	// room for: "+2d6 damage", or the spell's own words when the engine
	// cannot work it out.
	Note string `json:"note"`
	// Scales says whether the dice actually changed at this level. When it is
	// false the level is still worth offering - another target, a longer
	// duration - but nothing extra is rolled.
	Scales bool `json:"scales"`

	// Detail and Short are the same two lines a cast writes into the roll
	// history, said again for this level.
	Detail string `json:"detail"`
	Short  string `json:"short"`
}

// upcastRule is the scaling one at-higher-levels sentence states.
type upcastRule struct {
	// Dice is what is added per step ("1d6"), or Flat when it is a number.
	Dice string
	Flat int
	// Per is how many slot levels one step costs: 1 normally, 2 for "every
	// two slot levels above".
	Per int
	// Above is the level the steps are counted from.
	Above int
	// Heals marks a rule that grows healing rather than damage.
	Heals bool
	// Second marks a rule that grows the spell's second damage line rather
	// than its first. A spell with two damage types usually grows only one of
	// them, and says which by naming it: ice knife throws a shard for
	// piercing and bursts for cold, and it is the cold that a bigger slot
	// buys more of.
	Second bool
}

var (
	// "for each slot level above 1st", "for every two slot levels above 2nd"
	reUpStep = regexp.MustCompile(
		`(?i)for\s+(each|every\s+two)\s+slot\s+levels?\s+(?:above|beyond)\s+(\d+)(?:st|nd|rd|th)`)
	// the dice a sentence adds: "increases by 1d6", "roll an additional 2d10"
	reUpDice = regexp.MustCompile(`(?i)(?:by|additional)\s+(?:an\s+additional\s+)?(\d+d\d+)`)
	// the same thing as a flat number: "increases by 10", "an additional 5"
	reUpFlat = regexp.MustCompile(`(?i)(?:by|gain)\s+(?:an\s+additional\s+)?(\d+)\b`)
)

// ComputeUpcast works out what this spell does cast from each slot level above
// its own, up to 9th. cast is the spell at its own level, which is what the
// extra dice are added to.
func ComputeUpcast(sp *Spell, cast *SpellCast) []SpellUpcast {
	if sp == nil || cast == nil || sp.Level < 1 || sp.Level >= 9 {
		return nil
	}
	if strings.TrimSpace(sp.HigherLevel) == "" {
		return nil
	}
	rule := parseUpcastRule(sp.HigherLevel, cast)
	out := []SpellUpcast{}
	for lvl := sp.Level + 1; lvl <= 9; lvl++ {
		out = append(out, upcastAt(sp, cast, rule, lvl))
	}
	return out
}

// parseUpcastRule reads the scaling out of an at-higher-levels text. It
// answers nil when the text says nothing this engine can add up - which is
// most of the ways a spell can grow, and no less true for it.
func parseUpcastRule(text string, cast *SpellCast) *upcastRule {
	for _, sentence := range splitSentences(text) {
		m := reUpStep.FindStringSubmatch(sentence)
		if m == nil {
			continue
		}
		above, err := strconv.Atoi(m[2])
		if err != nil {
			continue
		}
		rule := &upcastRule{Per: 1, Above: above}
		if strings.Contains(strings.ToLower(m[1]), "two") {
			rule.Per = 2
		}
		low := strings.ToLower(sentence)
		// Which of the two numbers on the cast the sentence is talking
		// about. Healing is named as healing or as hit points regained; a
		// sentence that mentions neither, and no damage either, is growing
		// something this cannot roll.
		switch {
		case strings.Contains(low, "damage"):
			rule.Heals = false
		case strings.Contains(low, "healing"), strings.Contains(low, "hit points"):
			rule.Heals = true
		default:
			continue
		}
		// Which damage line, when the spell has two of them. The sentence
		// names the type it grows - "the cold damage increases by 1d6" - so
		// it goes to whichever line carries that type. Naming neither leaves
		// it on the first, which is what a spell with one line always meant.
		if !rule.Heals && cast.Damage2 != "" {
			switch {
			case cast.Damage2Type != "" && strings.Contains(low, strings.ToLower(cast.Damage2Type)):
				rule.Second = true
			case cast.DamageType != "" && strings.Contains(low, strings.ToLower(cast.DamageType)):
				rule.Second = false
			}
		}
		// Only a number that lands on something already rolled is used: a
		// spell that heals nothing at its own level is not made to heal by a
		// sentence about hit point maximums.
		if rule.Heals && cast.Heal == "" {
			continue
		}
		if !rule.Heals && !rule.Second && cast.Damage == "" {
			continue
		}
		if rule.Second && cast.Damage2 == "" {
			continue
		}
		if d := reUpDice.FindStringSubmatch(sentence); d != nil {
			rule.Dice = d[1]
			return rule
		}
		if f := reUpFlat.FindStringSubmatch(sentence); f != nil {
			if n, err := strconv.Atoi(f[1]); err == nil && n > 0 {
				rule.Flat = n
				return rule
			}
		}
	}
	return nil
}

// upcastAt is the spell cast from one particular slot level.
func upcastAt(sp *Spell, cast *SpellCast, rule *upcastRule, level int) SpellUpcast {
	up := SpellUpcast{Level: level, Label: Ordinal(level),
		Damage: cast.Damage, Damage2: cast.Damage2, Heal: cast.Heal}
	steps := 0
	if rule != nil {
		steps = (level - rule.Above) / rule.Per
	}
	if steps > 0 {
		added := ""
		switch {
		case rule.Dice != "":
			added = repeatDice(rule.Dice, steps)
		case rule.Flat != 0:
			added = Signed(rule.Flat * steps)
		}
		if added != "" {
			switch {
			case rule.Heals:
				up.Heal = addToDice(cast.Heal, rule, steps)
				up.Note = "+" + strings.TrimPrefix(added, "+") + " healing"
			case rule.Second:
				up.Damage2 = addToDice(cast.Damage2, rule, steps)
				up.Note = "+" + strings.TrimPrefix(added, "+")
				if cast.Damage2Type != "" {
					up.Note += " " + cast.Damage2Type
				}
			default:
				up.Damage = addToDice(cast.Damage, rule, steps)
				up.Note = "+" + strings.TrimPrefix(added, "+")
				if cast.DamageType != "" {
					up.Note += " " + cast.DamageType
				}
			}
			up.Scales = true
		}
	}
	if !up.Scales {
		up.Note = firstSentence(sp.HigherLevel)
	}
	// The two lines the roll history shows, said for this level: the same
	// words a cast at the spell's own level uses, with these dice.
	at := *cast
	at.Damage, at.Heal, at.Damage2 = up.Damage, up.Heal, up.Damage2
	up.Detail = castDetailAt(&at, sp, level)
	up.Short = castShort(&at)
	return up
}

// addToDice grows a dice expression by what one rule adds, taken steps times.
func addToDice(expr string, rule *upcastRule, steps int) string {
	d := parseDiceExpr(expr)
	if d == nil {
		return expr
	}
	if rule.Dice != "" {
		add := parseDiceExpr(rule.Dice)
		if add == nil {
			return expr
		}
		for _, sides := range add.Order {
			d.add(sides, add.Counts[sides]*steps)
		}
	}
	d.Flat += rule.Flat * steps
	return d.String()
}

// repeatDice is a dice expression taken n times over: 1d6 three times is 3d6.
func repeatDice(dice string, n int) string {
	d := parseDiceExpr(dice)
	if d == nil || n <= 0 {
		return ""
	}
	for _, sides := range d.Order {
		d.Counts[sides] *= n
	}
	d.Flat *= n
	return d.String()
}

// diceExpr is a dice expression pulled apart: how many of each size, plus a
// flat bonus. "2d8 +4" is two d8s and a four.
type diceExpr struct {
	Counts map[int]int
	Order  []int
	Flat   int
}

var reDiceTerm = regexp.MustCompile(`([+-]?)\s*(\d*)d(\d+)|([+-])\s*(\d+)`)

// parseDiceExpr reads "1d8 +4", "8d6" or "1d4 + 1". It answers nil for
// anything with no dice in it at all, which is not something to grow.
func parseDiceExpr(expr string) *diceExpr {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil
	}
	d := &diceExpr{Counts: map[int]int{}}
	found := false
	for _, m := range reDiceTerm.FindAllStringSubmatch(expr, -1) {
		if m[3] != "" {
			sides, err := strconv.Atoi(m[3])
			if err != nil || sides < 2 {
				continue
			}
			count := 1
			if m[2] != "" {
				if n, err := strconv.Atoi(m[2]); err == nil {
					count = n
				}
			}
			if m[1] == "-" {
				count = -count
			}
			d.add(sides, count)
			found = true
			continue
		}
		if m[5] != "" {
			n, err := strconv.Atoi(m[5])
			if err != nil {
				continue
			}
			if m[4] == "-" {
				n = -n
			}
			d.Flat += n
		}
	}
	if !found {
		return nil
	}
	return d
}

func (d *diceExpr) add(sides, count int) {
	if _, ok := d.Counts[sides]; !ok {
		d.Order = append(d.Order, sides)
	}
	d.Counts[sides] += count
}

// String writes the expression back out the way the sheet says it: the dice
// biggest first, then the flat bonus - "3d8 +4".
func (d *diceExpr) String() string {
	sides := append([]int{}, d.Order...)
	sort.Slice(sides, func(i, j int) bool { return sides[i] > sides[j] })
	parts := []string{}
	for _, s := range sides {
		if n := d.Counts[s]; n != 0 {
			parts = append(parts, fmt.Sprintf("%dd%d", n, s))
		}
	}
	out := strings.Join(parts, " + ")
	if d.Flat != 0 {
		if out == "" {
			return Signed(d.Flat)
		}
		out += " " + Signed(d.Flat)
	}
	return out
}

// splitSentences is enough sentence splitting to read one rule at a time out
// of a paragraph of rules text.
//
// A line break ends a sentence as surely as a full stop does, and spell text
// is written in paragraphs: without that, eldritch blast's "1d10 force
// damage" runs into the paragraph about its beams and the two together read
// as a damage that grows at 5th level, which is not what the spell says.
func splitSentences(text string) []string {
	out := []string{}
	for _, line := range strings.Split(text, "\n") {
		for _, part := range strings.Split(line, ". ") {
			if p := strings.TrimSpace(part); p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}
