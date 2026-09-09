//lint:file-ignore ST1006 allow the use of self
package dnd

/* SDOC: DnD
* Spending A Spell Slot

  Casting a spell of 1st level or higher costs a slot of that level, and the
  html character sheet marks one off as it rolls the spell. The count lives in
  the character's own file, in the =DND_SLOTS_USED= property - one number per
  spell level, in order - so what the sheet shows and what the org file says
  are the same thing.

  A spell may always be cast from a higher slot than its own. When the level a
  spell asks for is empty, casting reaches for the lowest slot above it that
  still has one left and says so; when nothing at all is left, the cast is
  refused and nothing is written.

  Which levels a character has slots for is never decided here - that is the
  rules engine's answer for their classes and level - so the sheet cannot
  spend a slot the character does not have.
EDOC */

import (
	"fmt"
	"strings"
)

// SlotRequest is one change to the slots a character has spent.
type SlotRequest struct {
	Filename string `json:"filename"`
	Id       string `json:"id"`
	// Action is "cast" (spend one, reaching upward if that level is empty),
	// "spend" (spend one of exactly this level) or "recover" (hand one back).
	Action string `json:"action"`
	// Level is the spell level of the slot, 1 to 9.
	Level int `json:"level"`
	// Spell is what is being cast. It only ever appears in the message.
	Spell string `json:"spell"`
}

// SlotChange is what one such change did: the level it actually landed on -
// which is not the level asked for when a cast had to reach upward - and how
// that level stands now.
type SlotChange struct {
	Level int `json:"level"`
	// Label is that level written the way the sheet says it: "2nd".
	Label string `json:"label"`
	// Asked is the level the caller wanted, when casting had to reach past it.
	Asked int    `json:"asked,omitempty"`
	Total int    `json:"total"`
	Used  int    `json:"used"`
	Left  int    `json:"left"`
	Up    bool   `json:"up"`
	Msg   string `json:"msg"`
}

// SlotUsed is how many slots of a level the character has spent.
func SlotUsed(c *Character, level int) int {
	if c == nil || level < 1 || level > len(c.SlotsUsed) {
		return 0
	}
	n := c.SlotsUsed[level-1]
	if n < 0 {
		return 0
	}
	return n
}

// SetSlotUsed writes the count back, growing the list to reach the level and
// trimming it again when the tail is all zeroes. An empty list is stored as
// nothing at all, which is what an untouched character has.
func SetSlotUsed(c *Character, level, used int) {
	if c == nil || level < 1 {
		return
	}
	if used < 0 {
		used = 0
	}
	for len(c.SlotsUsed) < level {
		c.SlotsUsed = append(c.SlotsUsed, 0)
	}
	c.SlotsUsed[level-1] = used
	end := len(c.SlotsUsed)
	for end > 0 && c.SlotsUsed[end-1] == 0 {
		end--
	}
	if end == 0 {
		c.SlotsUsed = nil
		return
	}
	c.SlotsUsed = c.SlotsUsed[:end]
}

// findSlot is the slot row for one level on a computed sheet.
func findSlot(s *Sheet, level int) (SpellSlotView, bool) {
	for _, row := range s.Slots {
		if row.Level == level {
			return row, true
		}
	}
	return SpellSlotView{}, false
}

// highestSlotLevel is the highest level this character has any slots at.
func highestSlotLevel(s *Sheet) int {
	top := 0
	for _, row := range s.Slots {
		if row.Total > 0 && row.Level > top {
			top = row.Level
		}
	}
	return top
}

// ApplySlotChange spends or hands back one spell slot and says what it did.
// The character is moved on; writing the sheet back out is the caller's job.
func ApplySlotChange(c *Character, rs *Ruleset, action string, level int,
	spell string) (*SlotChange, error) {
	s := Compute(c, rs)
	if len(s.Slots) == 0 {
		return nil, fmt.Errorf("%s has no spell slots", c.Name)
	}
	if level < 1 {
		return nil, fmt.Errorf("a cantrip costs no spell slot")
	}
	if level > 9 {
		return nil, fmt.Errorf("there is no such thing as a %s level slot",
			Ordinal(level))
	}
	act := strings.ToLower(strings.TrimSpace(action))
	switch act {
	case "cast", "spend", "use":
	case "recover", "unspend", "restore":
	default:
		return nil, fmt.Errorf("a slot is either %q, %q or %q, not %q",
			"cast", "spend", "recover", action)
	}

	if act == "recover" || act == "unspend" || act == "restore" {
		row, ok := findSlot(s, level)
		if !ok {
			return nil, fmt.Errorf("you have no %s level slots", Ordinal(level))
		}
		if row.Used <= 0 {
			return nil, fmt.Errorf("you have spent no %s level slots",
				Ordinal(level))
		}
		SetSlotUsed(c, level, row.Used-1)
		left := row.Total - (row.Used - 1)
		return &SlotChange{Level: level, Label: Ordinal(level),
			Total: row.Total, Used: row.Used - 1, Left: left,
			Msg: fmt.Sprintf("%s level slot recovered, %d of %d left",
				Ordinal(level), left, row.Total)}, nil
	}

	// Casting reaches upward when the level asked for is empty; spending a
	// named slot - a pip clicked on the sheet - means that one and no other.
	asked := level
	at := level
	if act == "cast" {
		top := highestSlotLevel(s)
		for ; at <= top; at++ {
			if row, ok := findSlot(s, at); ok && row.Left > 0 {
				break
			}
		}
		if at > top {
			if _, ok := findSlot(s, asked); !ok {
				return nil, fmt.Errorf("you have no %s level slots",
					Ordinal(asked))
			}
			if asked < top {
				return nil, fmt.Errorf("you have no %s level slots left, "+
					"and none above it either", Ordinal(asked))
			}
			return nil, fmt.Errorf("you have no %s level slots left; "+
				"take a rest", Ordinal(asked))
		}
	}
	row, ok := findSlot(s, at)
	if !ok {
		return nil, fmt.Errorf("you have no %s level slots", Ordinal(at))
	}
	if row.Left <= 0 {
		return nil, fmt.Errorf("you have no %s level slots left; take a long rest",
			Ordinal(at))
	}
	SetSlotUsed(c, at, row.Used+1)
	left := row.Total - (row.Used + 1)
	ch := &SlotChange{Level: at, Label: Ordinal(at), Total: row.Total,
		Used: row.Used + 1, Left: left, Up: at != asked}
	if ch.Up {
		ch.Asked = asked
	}
	what := strings.TrimSpace(spell)
	switch {
	case what != "" && ch.Up:
		ch.Msg = fmt.Sprintf("%s cast at %s level, %d of %d left",
			what, Ordinal(at), left, row.Total)
	case what != "":
		ch.Msg = fmt.Sprintf("%s cast, %d of %d %s level slots left",
			what, left, row.Total, Ordinal(at))
	default:
		ch.Msg = fmt.Sprintf("%s level slot spent, %d of %d left",
			Ordinal(at), left, row.Total)
	}
	return ch, nil
}

// RecoverOneSlotEachLevel hands back a single spent slot at every level that
// has one spent, which is what a short rest gives back here. It answers with
// how many slots that came to.
func RecoverOneSlotEachLevel(c *Character) int {
	back := 0
	for lvl := 1; lvl <= len(c.SlotsUsed); lvl++ {
		if used := SlotUsed(c, lvl); used > 0 {
			SetSlotUsed(c, lvl, used-1)
			back++
		}
	}
	return back
}
