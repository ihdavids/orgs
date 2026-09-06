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
EDOC */

import (
	"fmt"
	"regexp"
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
}

var (
	// "increases by 1d10 when you reach 5th level (2d10), 11th level (3d10)"
	reCantripStep = regexp.MustCompile(`(?i)(\d+)(?:st|nd|rd|th)\s+level\s*\((\d+d\d+)\)`)
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
	// A cantrip's damage is whatever the last step it has reached says.
	if sp.Level == 0 && cast.Damage != "" {
		if grown := cantripDamage(text, level); grown != "" {
			cast.Damage = grown
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
	cast.Detail = castDetail(&cast, sp)
	cast.Short = castShort(&cast)
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

// castDetail is the salient play detail written beside the spell's name in
// the roll history: level, what it does, and whether it holds concentration.
func castDetail(cast *SpellCast, sp *Spell) string {
	parts := []string{sp.LevelString()}
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
