package dnd

/* SDOC: DnD
* Casting A Spell

  A spell on the sheet knows what happens when it is cast: whether it needs a
  spell attack roll, what the target saves against, what it does for damage or
  healing, and how much of that a cantrip has grown by at this level. That is
  worked out once by the rules engine, into =SpellEntry.Cast=, so the html
  sheet can cast a spell with one click and the same answer would come out of
  any other client.

  Most of it comes straight from the ruleset - =attack=, =save= and =damage=
  are fields on a spell - and the rest is read out of the spell's own text:

  - a *ranged* or *melee* spell attack, from the phrase the spell uses,
  - the saving throw's consequence ("half as much damage on a successful one"),
  - a cantrip's damage at this character level, from the sentence that lists
    it: "increases by 1d10 when you reach 5th level (2d10), 11th level
    (3d10)...",
  - healing, from "regains a number of hit points equal to 1d8 + your
    spellcasting ability modifier".

  A spell with nothing to roll still says what it does to whoever is on the
  other end of it, which is the line the sheet shows in place of a result:
  "Dexterity saving throw against DC 15, half as much damage on a success".

  A cantrip that grows with the character's level carries those levels as
  well, in =SpellCast.Tiers= - one entry per level its own text names, up to
  the one this character has reached, each with the dice it rolls there. The
  last of them is what the cast button already rolls; the sheet offers the
  rest beside it the way a levelled spell offers its slots, so the
  progression can be seen and a lower tier rolled for someone further down
  the table. A tier is not a slot and spends nothing.
EDOC */

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// SpellCast is everything the table needs when a spell is cast: what to roll,
// and what the other side has to roll back.
type SpellCast struct {
	// Attack is set when the spell calls for a spell attack roll, and Kind is
	// "ranged" or "melee" when the text says which.
	Attack      bool   `json:"attack"`
	Kind        string `json:"kind"`
	AttackBonus int    `json:"attackBonus"`

	// Save is the ability the target rolls, SaveDC what it has to beat, and
	// SaveFor what a success gets them.
	Save     string `json:"save"`
	SaveName string `json:"saveName"`
	SaveDC   int    `json:"saveDc"`
	SaveFor  string `json:"saveFor"`

	// Damage and Heal are dice expressions ready to roll, already grown to
	// this character's level for a cantrip.
	Damage     string `json:"damage"`
	DamageType string `json:"damageType"`
	Heal       string `json:"heal"`

	// Damage2 is a second, separate lot of damage - ice knife's cold burst
	// after the piercing shard, hellfire's necrotic beside its fire. It is
	// rolled on its own because the two are different types and adding them
	// together would lose which half a resistance applies to. Label is what
	// the button that rolls it says.
	Damage2      string `json:"damage2,omitempty"`
	Damage2Type  string `json:"damage2Type,omitempty"`
	Damage2Label string `json:"damage2Label,omitempty"`

	// Rolls says whether casting this rolls anything at all. When it does
	// not, Line is what to read out instead.
	Rolls bool   `json:"rolls"`
	Line  string `json:"line"`
	// Detail is the one line of salient play detail that goes in the roll
	// history beside the spell's name.
	Detail string `json:"detail"`
	// Short is the same thing in as few words as fit on one line of roll
	// history: "DEX save DC 15".
	Short string `json:"short"`

	// Upcast is this spell cast from a bigger slot than it needs, one entry
	// per level above its own, when its text says what that buys. See
	// upcast.go.
	Upcast []SpellUpcast `json:"upcast,omitempty"`

	// Tiers is a cantrip's damage at each of the character levels it grows
	// at, up to the one this character has reached. A cantrip has no slot to
	// pick, so the last entry is what casting it rolls; the earlier ones are
	// there to be seen, and to be rolled for someone lower down the table.
	Tiers []SpellTier `json:"tiers,omitempty"`
	// TierLevel is the level of the tier this character is at, which is the
	// one the cast button rolls and the one the picker starts on.
	TierLevel int `json:"tierLevel,omitempty"`
}

// SpellTier is a cantrip at one of the character levels its dice grow at.
type SpellTier struct {
	// Level is the character level the step lands on - 1 for the cantrip as
	// it starts out, then the levels its own text names.
	Level int    `json:"level"`
	Label string `json:"label"`

	// Damage, Damage2 and Heal are what it rolls at that level.
	Damage  string `json:"damage"`
	Damage2 string `json:"damage2,omitempty"`
	Heal    string `json:"heal,omitempty"`

	// Note is the dice at this tier in the words the sheet has room for:
	// "2d4 fire + 2d4 necrotic".
	Note string `json:"note"`
	// Current marks the tier this character is actually at, which is the one
	// the cast button already rolls.
	Current bool `json:"current"`

	// Detail and Short are the two lines the roll history shows, said again
	// for this tier.
	Detail string `json:"detail"`
	Short  string `json:"short"`
}

var (
	// "increases by 1d10 when you reach 5th level (2d10), 11th level (3d10)"
	reCantripStep = regexp.MustCompile(`(?i)(\d+)(?:st|nd|rd|th)\s+level\s*\((\d+d\d+)\)`)
	// The same sentence read for its levels alone, for the cantrips that do
	// not spell the dice out in a way reCantripStep can read - "(2d4 each)".
	reCantripLevel = regexp.MustCompile(`(?i)(\d+)(?:st|nd|rd|th)\s+level`)
	// a bare dice expression, for growing one by a step
	reDiceOnly = regexp.MustCompile(`^\s*(\d*)d(\d+)\s*$`)
	// a dice expression with a flat bonus, as magic missile's "1d4 + 1"
	reDicePlus = regexp.MustCompile(`(\d+d\d+\s*\+\s*\d+)`)
	// "regains a number of hit points equal to 1d8 + your spellcasting ability modifier"
	reHeal = regexp.MustCompile(`(?i)(?:regains?|restores?)[^.]*?hit points[^.]*?(\d+d\d+)`)
	// the dice at the front of a damage field: "8d6 fire" -> "8d6", "fire"
	reDamage = regexp.MustCompile(`^\s*(\d*d\d+(?:\s*[+-]\s*\d+)?)\s*(.*)$`)
	// "must succeed on a Dexterity saving throw" - the fallback for spells a
	// ruleset never got round to tagging with a save.
	reSaveText = regexp.MustCompile(
		`(?i)(strength|dexterity|constitution|intelligence|wisdom|charisma)\s+saving throw`)
)

// ComputeCast works out what casting this spell does for this character.
// attackBonus and saveDC come from the sheet, abilityMod is the spellcasting
// ability modifier, and level is the character's total level, which is what a
// cantrip's damage grows with.
func ComputeCast(sp *Spell, level, attackBonus, saveDC, abilityMod int) SpellCast {
	cast := SpellCast{}
	if sp == nil {
		return cast
	}
	text := sp.Text

	// ---- attack -----------------------------------------------------------
	low := strings.ToLower(text)
	cast.Attack = sp.Attack
	switch {
	case strings.Contains(low, "ranged spell attack"):
		cast.Attack, cast.Kind = true, "ranged"
	case strings.Contains(low, "melee spell attack"):
		cast.Attack, cast.Kind = true, "melee"
	}
	if cast.Attack {
		cast.AttackBonus = attackBonus
	}

	// ---- saving throw -----------------------------------------------------
	//
	// The ruleset names the save on the spells it has been told about; every
	// other one says so in its own text, which is where the rest come from.
	save := sp.Save
	if save == "" {
		if m := reSaveText.FindStringSubmatch(text); m != nil {
			save = abilityIdFor(m[1])
		}
	}
	if save != "" {
		cast.Save = strings.ToLower(save)
		cast.SaveName = AbilityNames[cast.Save]
		if cast.SaveName == "" {
			cast.SaveName = Titleize(cast.Save)
		}
		cast.SaveDC = saveDC
		switch {
		case strings.Contains(low, "half as much damage on a successful"):
			cast.SaveFor = "half as much damage on a success"
		case strings.Contains(low, "saving throw or take"),
			strings.Contains(low, "saving throw, taking"):
			cast.SaveFor = "no damage on a success"
		}
	}

	// ---- damage -----------------------------------------------------------
	if sp.Damage != "" {
		if m := reDamage.FindStringSubmatch(sp.Damage); m != nil {
			cast.Damage = strings.TrimSpace(m[1])
			cast.DamageType = strings.TrimSpace(m[2])
		} else {
			cast.Damage = strings.TrimSpace(sp.Damage)
		}
	}
	// A cantrip's damage is whatever the last step it has reached says, or
	// failing that whatever counting the steps makes of it. The dice it
	// started out with are kept, because the tiers below this level are
	// counted from them and not from what this level grew them to.
	baseDamage := cast.Damage
	if sp.Level == 0 && cast.Damage != "" {
		cast.Damage = cantripDamageAt(text, level, cast.Damage)
	}

	// ---- the second damage ------------------------------------------------
	baseDamage2 := ""
	if sp.Damage2 != "" {
		if m := reDamage.FindStringSubmatch(sp.Damage2); m != nil {
			cast.Damage2 = strings.TrimSpace(m[1])
			cast.Damage2Type = strings.TrimSpace(m[2])
		} else {
			cast.Damage2 = strings.TrimSpace(sp.Damage2)
		}
		cast.Damage2Label = strings.TrimSpace(sp.Damage2Label)
		if cast.Damage2Label == "" {
			cast.Damage2Label = cast.Damage2Type
		}
		if cast.Damage2Label == "" {
			cast.Damage2Label = "Also"
		}
		// A cantrip that rolls two damage types grows both of them, and the
		// sentence that says so states one step for the pair of them:
		// hellfire's "both of this spell's damage types increase by 1d4".
		// So the same step is applied to each, scaled from its own base.
		if sp.Level == 0 && cast.Damage2 != "" {
			baseDamage2 = cast.Damage2
			if grown := cantripDamageFrom(text, level, cast.Damage2); grown != "" {
				cast.Damage2 = grown
			}
		}
	}
	// The ruleset records the dice, the text sometimes adds a flat bonus to
	// them - magic missile's "1d4 + 1 force damage" - which is worth keeping.
	if cast.Damage != "" && !strings.ContainsAny(cast.Damage, "+-") {
		if m := reDicePlus.FindStringSubmatch(text); m != nil {
			full := strings.Join(strings.Fields(m[1]), " ")
			if strings.HasPrefix(full, cast.Damage+" ") {
				cast.Damage = full
			}
		}
	}

	// ---- healing ----------------------------------------------------------
	if m := reHeal.FindStringSubmatch(text); m != nil {
		cast.Heal = m[1]
		if strings.Contains(low, "spellcasting ability modifier") && abilityMod != 0 {
			cast.Heal += " " + Signed(abilityMod)
		}
	}

	cast.Rolls = cast.Attack || cast.Damage != "" || cast.Heal != ""
	cast.Line = castLine(&cast, sp)
	cast.Detail = castDetailAt(&cast, sp, sp.Level)
	cast.Short = castShort(&cast)
	cast.Upcast = ComputeUpcast(sp, &cast)
	cast.Tiers = cantripTiers(sp, &cast, level, baseDamage, baseDamage2)
	if n := len(cast.Tiers); n > 0 {
		cast.TierLevel = cast.Tiers[n-1].Level
	}
	return cast
}

// castShort is what fits beside a spell's name in the roll history, where
// there is room for three or four words and no more.
func castShort(cast *SpellCast) string {
	if cast.Save != "" {
		save := AbilityShort[cast.Save]
		if save == "" {
			save = strings.ToUpper(cast.Save)
		}
		if cast.SaveDC > 0 {
			return fmt.Sprintf("%s save DC %d", save, cast.SaveDC)
		}
		return save + " save"
	}
	if !cast.Rolls {
		return "no roll"
	}
	if cast.Heal != "" && cast.Damage == "" {
		return "heals " + cast.Heal
	}
	if cast.Damage != "" {
		return cast.Damage + " " + cast.DamageType
	}
	return ""
}

// abilityIdFor turns "Dexterity" into the ability id the rest of the engine
// uses.
func abilityIdFor(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	for id, full := range AbilityNames {
		if strings.ToLower(full) == name {
			return id
		}
	}
	return ""
}

// cantripDamage is a cantrip's damage at this character level, from the
// sentence listing what it grows to and when.
func cantripDamage(text string, level int) string {
	best, at := "", 0
	for _, m := range reCantripStep.FindAllStringSubmatch(text, -1) {
		lvl, err := strconv.Atoi(m[1])
		if err != nil || lvl > level || lvl < at {
			continue
		}
		best, at = m[2], lvl
	}
	return best
}

// cantripDamageAt is one damage line of a cantrip at a character level: the
// dice the text spells out for the last step reached, or failing that what
// counting the steps makes of the dice it started with. Below the first step
// it is those dice unchanged.
//
// It reads the stated dice, so it is for the line the sentence is written
// about - the first one. A second damage line has to be counted; see
// cantripDamageFrom.
func cantripDamageAt(text string, level int, base string) string {
	if base == "" {
		return base
	}
	if grown := cantripDamage(text, level); grown != "" {
		return grown
	}
	if grown := cantripDamageFrom(text, level, base); grown != "" {
		return grown
	}
	return base
}

// cantripSteps is the character levels a cantrip's text says its dice grow at,
// in order: "5th level (2d10), 11th level (3d10), and 17th level (4d10)" is
// 5, 11, 17.
func cantripSteps(text string) []int {
	out := []int{}
	seen := map[int]bool{}
	for _, lm := range reCantripLevel.FindAllStringSubmatch(cantripSentence(text), -1) {
		lvl, err := strconv.Atoi(lm[1])
		// A cantrip's steps are levels a character reaches, so a "1st level"
		// in the sentence is talking about a slot and not a step.
		if err != nil || lvl <= 1 || seen[lvl] {
			continue
		}
		seen[lvl] = true
		out = append(out, lvl)
	}
	sort.Ints(out)
	return out
}

// cantripTiers is a scaling cantrip at each of the levels its dice grow at,
// up to the one this character has reached. A cantrip has no slot to choose,
// so the tiers are not choices in the way a levelled spell's are - the last
// one is what this character rolls - but the sheet offers them beside the
// cast button all the same, to show the progression and to roll a lower tier
// for someone else at the table.
//
// damage and damage2 are the dice before this character's level grew them,
// which is what every tier is counted from.
func cantripTiers(sp *Spell, cast *SpellCast, level int, damage, damage2 string) []SpellTier {
	if sp == nil || cast == nil || sp.Level != 0 {
		return nil
	}
	if damage == "" && damage2 == "" {
		return nil
	}
	levels := []int{1}
	for _, step := range cantripSteps(sp.Text) {
		if step > level {
			break
		}
		levels = append(levels, step)
	}
	// A cantrip that has grown nothing yet has one tier, which is no
	// progression to show and nothing but the cast button already says.
	if len(levels) < 2 {
		return nil
	}
	out := []SpellTier{}
	for i, lvl := range levels {
		tier := SpellTier{Level: lvl, Label: Ordinal(lvl), Heal: cast.Heal,
			Current: i == len(levels)-1}
		tier.Damage = cantripDamageAt(sp.Text, lvl, damage)
		if damage2 != "" {
			tier.Damage2 = damage2
			if grown := cantripDamageFrom(sp.Text, lvl, damage2); grown != "" {
				tier.Damage2 = grown
			}
		}
		tier.Note = damageNote(tier.Damage, cast.DamageType, tier.Damage2, cast.Damage2Type)
		// The two lines the roll history shows, said for this tier.
		at := *cast
		at.Damage, at.Damage2 = tier.Damage, tier.Damage2
		tier.Detail = castDetailAt(&at, sp, sp.Level)
		if !tier.Current {
			tier.Detail = "at " + tier.Label + " level · " + tier.Detail
		}
		tier.Short = castShort(&at)
		out = append(out, tier)
	}
	return out
}

// damageNote is what a tier rolls, in the few words an option has room for:
// "2d4 fire + 2d4 necrotic".
func damageNote(damage, damageType, damage2, damage2Type string) string {
	parts := []string{}
	for _, d := range [][2]string{{damage, damageType}, {damage2, damage2Type}} {
		if d[0] == "" {
			continue
		}
		if d[1] != "" {
			parts = append(parts, d[0]+" "+d[1])
			continue
		}
		parts = append(parts, d[0])
	}
	return strings.Join(parts, " + ")
}

// cantripDamageFrom grows one damage line of a cantrip by counting the steps
// it has reached rather than reading the dice off the text.
//
// cantripDamage reads the dice the sentence spells out - "(2d10)", "(3d10)" -
// which is exact and is what most cantrips give you. A cantrip that rolls two
// damage types states the step once for both of them instead, and writes the
// dice in a form that belongs to neither line on its own: hellfire's "both of
// this spell's damage types increase by 1d4 ... 5th level (2d4 each)". There
// is nothing there to read for the second line.
//
// The levels themselves are always stated, so this counts them and adds a die
// of the line's own base for each one reached. That is the shape every
// scaling cantrip has - one more die per step - so counting gets the same
// answer as reading wherever both can be had.
func cantripDamageFrom(text string, level int, base string) string {
	m := reDiceOnly.FindStringSubmatch(base)
	if m == nil {
		return ""
	}
	count := 1
	if m[1] != "" {
		n, err := strconv.Atoi(m[1])
		if err != nil || n < 1 {
			return ""
		}
		count = n
	}
	steps := 0
	for _, step := range cantripSteps(text) {
		if step <= level {
			steps++
		}
	}
	if steps == 0 {
		return ""
	}
	return fmt.Sprintf("%dd%s", count+steps, m[2])
}

// cantripSentence is the sentence that says how a cantrip grows, so that the
// levels counted out of it are the steps and not some other number of levels
// the spell happens to mention.
func cantripSentence(text string) string {
	for _, sentence := range splitSentences(text) {
		low := strings.ToLower(sentence)
		if strings.Contains(low, "when you reach") &&
			(strings.Contains(low, "increase") || strings.Contains(low, "damage")) {
			return sentence
		}
	}
	return ""
}

// castLine is what the other side of the spell has to do about it: the saving
// throw, in the words you would say across the table. A spell that rolls its
// own dice and asks nothing of the target has no line - the roll is the
// answer - and a spell that rolls nothing at all describes itself instead.
func castLine(cast *SpellCast, sp *Spell) string {
	if cast.Save != "" {
		line := fmt.Sprintf("%s saving throw against DC %d", cast.SaveName, cast.SaveDC)
		if cast.SaveDC <= 0 {
			line = cast.SaveName + " saving throw"
		}
		if cast.SaveFor != "" {
			line += ", " + cast.SaveFor
		}
		return line
	}
	if cast.Rolls {
		return ""
	}
	// Nothing is rolled and nobody saves: say what the spell costs to cast
	// and how long it lasts, which is what actually gets tracked in play.
	parts := []string{}
	if sp.CastingTime != "" {
		parts = append(parts, sp.CastingTime)
	}
	if sp.Range != "" {
		parts = append(parts, sp.Range)
	}
	if sp.Duration != "" {
		parts = append(parts, sp.Duration)
	}
	if sp.Concentration {
		parts = append(parts, "concentration")
	}
	if len(parts) == 0 {
		return "No roll"
	}
	return "No roll - " + strings.Join(parts, ", ")
}

// castDetailAt is the salient play detail written beside the spell's name in
// the roll history: level, what it does, and whether it holds concentration.
// at is the slot level it is being cast from, which is only worth saying when
// it is above the spell's own.
func castDetailAt(cast *SpellCast, sp *Spell, at int) string {
	parts := []string{sp.LevelString()}
	if at > sp.Level {
		parts = []string{fmt.Sprintf("cast at %s level", Ordinal(at))}
	}
	if cast.Damage != "" {
		d := cast.Damage
		if cast.DamageType != "" {
			d += " " + cast.DamageType
		}
		parts = append(parts, d)
	}
	if cast.Heal != "" {
		parts = append(parts, "heals "+cast.Heal)
	}
	if cast.Save != "" {
		dc := ""
		if cast.SaveDC > 0 {
			dc = fmt.Sprintf(" DC %d", cast.SaveDC)
		}
		save := AbilityShort[cast.Save]
		if save == "" {
			save = strings.ToUpper(cast.Save)
		}
		parts = append(parts, save+" save"+dc)
		if cast.SaveFor != "" {
			parts = append(parts, cast.SaveFor)
		}
	}
	if sp.Range != "" {
		parts = append(parts, sp.Range)
	}
	if sp.Concentration {
		parts = append(parts, "concentration")
	}
	return strings.Join(parts, " · ")
}
