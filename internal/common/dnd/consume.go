//lint:file-ignore ST1006 allow the use of self
package dnd

/* SDOC: DnD
* Drinking A Potion

  Using something up is already an inventory change - one fewer potion, a line
  in the Inventory History - but for the things that heal it should also be a
  roll and a change to the hit points, or the player has to drink the potion on
  one part of the sheet and then type the number into another.

  So an item's own text is read for what using it does:

  #+BEGIN_SRC yaml
  - id: "keoghtoms-ointment"
    text: "The creature that receives it regains 2d8+2 hit points..."
  #+END_SRC

  gives =2d8+2= of healing, and the html sheet's Drink button throws those dice
  on the table and posts the total to the hit points along with the potion
  being used up.

  As with the use limits in [[uses.go]] this is prose parsing and so is best
  effort by design: an effect the text does not plainly state is never guessed
  at, and an item with no readable effect simply keeps the plain Use button it
  always had.

  A Potion of Healing is the awkward case, because the SRD prints one entry for
  all four strengths and puts the dice in a table:

  #+BEGIN_SRC org
  | Potion of ...    | Rarity    | HP Regained |
  | Healing          | Common    | 2d4+2       |
  | Greater healing  | Uncommon  | 4d4+4       |
  #+END_SRC

  A table like that is read as variants, and the sheet asks which one is being
  drunk rather than picking for you.
EDOC */

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// The effects a use can have, which is what the sheet does with the answer.
const (
	UseHeal = "heal"
	UseTemp = "temp"
)

// UseVariant is one row of an item's own table of strengths: a Potion of
// Healing and a Potion of Superior Healing are one entry in the SRD with four
// rows under it.
type UseVariant struct {
	Name   string `json:"name"`
	Rarity string `json:"rarity"`
	// Heal is the dice that row restores, written as the rules write them.
	Heal string `json:"heal"`
}

// UseEffect is what using an item up does to the character, as far as the
// text plainly says. Every field is empty or zero when the text does not say,
// and Has reports whether anything at all was found.
type UseEffect struct {
	// Heal is the dice rolled for hit points restored, "2d4+2". HealFlat is
	// the same thing when the text gives a plain number instead of dice.
	Heal     string `json:"heal,omitempty"`
	HealFlat int    `json:"healFlat,omitempty"`
	// Temp is temporary hit points gained, which are always a flat number in
	// the rules.
	Temp int `json:"temp,omitempty"`
	// Variants are the strengths an item comes in, when its text prints a
	// table of them instead of one set of dice.
	Variants []UseVariant `json:"variants,omitempty"`
	// Verb is what the sheet's button says: "Drink" for a potion, "Use" for
	// everything else.
	Verb string `json:"verb,omitempty"`
}

// Has reports whether the effect says anything worth acting on.
func (e UseEffect) Has() bool {
	return e.Heal != "" || e.HealFlat > 0 || e.Temp > 0 || len(e.Variants) > 0
}

// Kind is the effect the sheet applies once the dice are in: healing, or
// temporary hit points. An effect with variants is healing whichever row is
// picked, so it answers heal too.
func (e UseEffect) Kind() string {
	if e.Heal != "" || e.HealFlat > 0 || len(e.Variants) > 0 {
		return UseHeal
	}
	if e.Temp > 0 {
		return UseTemp
	}
	return ""
}

// useSubject is who has to be doing the regaining for it to be the character.
// Plenty of items have hit points of their own - a rope of climbing regains one
// every five minutes - and healing the item is not healing its owner. The gap
// allows for the words a sentence puts between the two ("the creature that
// receives it regains", "you swallow the dose and regain") but cannot cross
// into the next sentence, so one claim never lends its subject to another.
const useSubject = `(?:you|creature|target|wearer|drinker)\b[^.!?\n]{0,40}?\b`

var (
	// "You regain 2d4 + 2 hit points", "the creature regains 2d8+2 hit points"
	useHealDice = regexp.MustCompile(
		`(?i)\b` + useSubject + `regains?\s+(?:up\s+to\s+)?(\d*d\d+(?:\s*[+-]\s*\d+)?)\s+hit\s+points`)
	// The same with a plain number in place of the dice.
	useHealFlat = regexp.MustCompile(`(?i)\b` + useSubject + `regains?\s+(\d+)\s+hit\s+points`)
	// "you gain 10 temporary hit points"
	useTempFlat = regexp.MustCompile(`(?i)\bgains?\s+(\d+)\s+temporary\s+hit\s+points`)
	// A dice expression on its own, which is all a table cell holds.
	useDiceOnly = regexp.MustCompile(`^\d*d\d+(?:\s*[+-]\s*\d+)?$`)
	// What marks healing as something that keeps happening rather than
	// something a dose does once. A ring of regeneration and an ioun stone
	// both say they restore hit points, but they say it of every hour you
	// wear them: there is no dose of either, so there is nothing for a Use
	// button to spend. Looking for this in the sentence the healing was found
	// in is more reliable than looking for the word "drink" somewhere in the
	// item - an ioun stone of sustenance mentions drinking too.
	useContinuous = regexp.MustCompile(
		`(?i)\b(?:each|every|per)\s+(?:\d+\s+)?(?:hour|minute|round|turn|day|dawn|dusk)|` +
			`\bwhile\s+(?:you|this|it|wearing|worn)|\bat\s+the\s+end\s+of\s+each\b`)
)

// ItemUse reads what using an item up does out of its own text. Everything
// here is best effort: a text that does not plainly say is left saying nothing,
// and the sheet falls back to the plain Use button.
func ItemUse(it *Item) UseEffect {
	e := UseEffect{}
	if it == nil {
		return e
	}
	text := it.Text
	if strings.TrimSpace(text) == "" {
		return e
	}
	if isPotion(it) {
		e.Verb = "Drink"
	}
	// A table of strengths comes first: when there is one, the dice in the
	// prose above it are the table's business rather than a single answer.
	e.Variants = useVariants(text)
	if len(e.Variants) == 0 {
		if m := findOnce(useHealDice, text); m != nil {
			e.Heal = tidyDice(m[1])
		} else if m := findOnce(useHealFlat, text); m != nil {
			if n, err := strconv.Atoi(m[1]); err == nil && n > 0 {
				e.HealFlat = n
			}
		}
	}
	if m := findOnce(useTempFlat, text); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil && n > 0 {
			e.Temp = n
		}
	}
	return e
}

// findOnce is a match that is about using something up rather than about
// wearing it: the sentence it was found in must not say the effect keeps
// happening. Anything continuous belongs to the rules engine, not to a button.
func findOnce(re *regexp.Regexp, text string) []string {
	for _, at := range re.FindAllStringSubmatchIndex(text, -1) {
		if useContinuous.MatchString(sentenceAt(text, at[0], at[1])) {
			continue
		}
		out := make([]string, 0, len(at)/2)
		for i := 0; i < len(at); i += 2 {
			if at[i] < 0 {
				out = append(out, "")
				continue
			}
			out = append(out, text[at[i]:at[i+1]])
		}
		return out
	}
	return nil
}

// sentenceAt is the sentence a match fell in, which is as much context as a
// single claim about hit points ever needs.
func sentenceAt(text string, from, to int) string {
	start := strings.LastIndexAny(text[:from], ".!?\n")
	if start < 0 {
		start = 0
	} else {
		start++
	}
	end := strings.IndexAny(text[to:], ".!?\n")
	if end < 0 {
		end = len(text)
	} else {
		end += to
	}
	return text[start:end]
}

// isPotion reports whether an item is drunk rather than merely used, which is
// only ever about the word on the button.
func isPotion(it *Item) bool {
	if strings.EqualFold(strings.TrimSpace(it.Kind), "potion") ||
		strings.EqualFold(strings.TrimSpace(it.Category), "potion") {
		return true
	}
	return strings.Contains(strings.ToLower(it.Name), "potion")
}

// useVariants reads a table of strengths out of an item's text. The column
// wanted is the one headed with what is regained; the first column names the
// row and the second, when there is one, is its rarity.
func useVariants(text string) []UseVariant {
	lines := strings.Split(text, "\n")
	col, nameCol, rarityCol := -1, 0, -1
	out := []UseVariant{}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "|") {
			// A blank line or prose after the table ends it; a table that has
			// not started yet is simply not there yet.
			if col >= 0 && line == "" {
				break
			}
			continue
		}
		cells := tableCells(line)
		if len(cells) == 0 {
			continue
		}
		if col < 0 {
			// Looking for the header. "HP Regained" is what the SRD calls the
			// column; anything that says regained will do.
			for i, cell := range cells {
				low := strings.ToLower(cell)
				if strings.Contains(low, "regained") || strings.Contains(low, "hp restored") {
					col = i
				}
				if strings.Contains(low, "rarity") {
					rarityCol = i
				}
			}
			continue
		}
		if col >= len(cells) {
			continue
		}
		dice := tidyDice(cells[col])
		if !useDiceOnly.MatchString(dice) {
			continue
		}
		v := UseVariant{Name: strings.TrimSpace(cells[nameCol]), Heal: dice}
		if rarityCol >= 0 && rarityCol < len(cells) {
			v.Rarity = strings.TrimSpace(cells[rarityCol])
		}
		if v.Name == "" {
			continue
		}
		out = append(out, v)
	}
	if len(out) < 2 {
		// One row is not a table of strengths, it is a fact about the item,
		// and the prose above it says the same thing better.
		return nil
	}
	return out
}

// tableCells splits one row of a markdown table, dropping the rules between
// the columns and the separator row made of dashes.
func tableCells(line string) []string {
	line = strings.Trim(line, "|")
	if strings.Trim(line, "-|: ") == "" {
		return nil
	}
	parts := strings.Split(line, "|")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		out = append(out, strings.TrimSpace(p))
	}
	return out
}

// tidyDice closes up the spaces the rules put around a plus sign, so "2d4 + 2"
// and "2d4+2" are the one string the dice roller reads.
func tidyDice(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, " + ", "+")
	s = strings.ReplaceAll(s, " - ", "-")
	s = strings.ReplaceAll(s, "+ ", "+")
	s = strings.ReplaceAll(s, " +", "+")
	return strings.TrimSpace(s)
}

// UseVariantHeal is the dice for one named strength of an item, for a use that
// says which one is being drunk. A name the item does not offer is an error
// rather than the first row, so a stale page cannot drink the wrong potion.
func UseVariantHeal(e UseEffect, name string) (string, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", false
	}
	for _, v := range e.Variants {
		if strings.EqualFold(v.Name, name) {
			return v.Heal, true
		}
	}
	return "", false
}

// ----------------------------------------------------------------------------
// Using something up
// ----------------------------------------------------------------------------

// UseResult is what using one of something did: the line it wrote in the
// Inventory History, and the change to the hit points when the item was one of
// the few that heal.
type UseResult struct {
	Item InventoryEvent `json:"item"`
	// Health is nil unless the use moved the hit points.
	Health *HealthEvent `json:"health,omitempty"`
	Effect UseEffect    `json:"effect"`
	// Applied is the effect that was actually applied - "heal", "temp" or ""
	// for an item that was simply used up - and Amount how much of it.
	Applied string `json:"applied"`
	Amount  int    `json:"amount"`
	Msg     string `json:"msg"`
}

// UseItem takes one of something out of the bag and does what using it does.
//
// The dice are the sheet's to roll, not the engine's: a potion drunk on the
// html sheet is thrown on the dice table like every other roll, so the total
// arrives here already rolled. An amount of zero uses the item up and touches
// nothing else, which is what using something has always done.
//
// The item goes first. A potion that cannot be drunk because it is not there
// must not heal anybody.
func UseItem(c *Character, rs *Ruleset, req InventoryRequest) (UseResult, error) {
	out := UseResult{}
	if c == nil {
		return out, fmt.Errorf("no character")
	}
	qty := maxInt(req.Qty, 1)
	it := shopItem(rs, req.Item, req.Name)
	out.Effect = ItemUse(it)

	// What the caller says it is applying has to be something the item
	// actually does, or a stale page could heal out of a rope.
	effect := strings.ToLower(strings.TrimSpace(req.Effect))
	amount := req.Amount
	if amount < 0 {
		amount = 0
	}
	if effect != "" && amount > 0 {
		if !out.Effect.Has() {
			return out, fmt.Errorf("%s does not do that", useItemName(it, req))
		}
		if effect != out.Effect.Kind() {
			return out, fmt.Errorf("%s does not %s anyone", useItemName(it, req), effect)
		}
		if req.Variant != "" {
			if _, ok := UseVariantHeal(out.Effect, req.Variant); !ok {
				return out, fmt.Errorf("%s does not come in %s",
					useItemName(it, req), req.Variant)
			}
		}
	}

	event, err := InventoryRemove(c, rs, req.Item, qty, req.Container, InvUsed, req.Notes)
	if err != nil {
		return out, err
	}
	out.Item = event
	out.Msg = "used " + event.Item

	if effect == "" || amount == 0 {
		return out, nil
	}
	action := Heal
	if effect == UseTemp {
		action = SetTemp
	}
	health, err := ApplyHealth(c, rs, HealthRequest{
		Action: action, Amount: amount, Notes: useNote(event.Item, req.Variant),
	})
	if err != nil {
		// The item is gone either way - it was drunk - so a refusal here is
		// worth saying out loud rather than hiding, and the sheet shows both.
		out.Msg = "used " + event.Item + ", but " + err.Error()
		return out, nil
	}
	out.Health = &health
	out.Applied, out.Amount = effect, amount
	out.Msg = "used " + event.Item + ": " + HealthEventMsg(health)
	return out, nil
}

// useItemName is what to call the thing in a refusal, before the bag has been
// asked whether it is even there.
func useItemName(it *Item, req InventoryRequest) string {
	if it != nil {
		return it.Name
	}
	return shopName(req.Item, req.Name)
}

// useNote is the line the Health History carries, so the healing says what it
// came out of rather than appearing from nowhere.
func useNote(item, variant string) string {
	if strings.TrimSpace(variant) != "" {
		return item + " (" + variant + ")"
	}
	return item
}
