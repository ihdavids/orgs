//lint:file-ignore ST1006 allow the use of self
package dnd

/* SDOC: DnD
* Taking The Last Thing Back

  Every change the html character sheet makes is written straight into the org
  file, which is what makes the sheet trustworthy - and what makes a misplaced
  click expensive. Pressing Drink on the wrong line spends a Potion of Healing
  and heals you for it, and putting that right by hand means adding the potion
  back, taking the hit points off again and deleting two lines of history.

  So the last thing that happened can be taken back. Undo reverses one
  operation - the item that moved, and the hit points or the coin that moved
  with it - and takes its lines out of the history, so an accident leaves no
  trace rather than leaving a correction.

  It is deliberately only ever the *last* thing, and only when it really is the
  last thing. The character sheet is a file anyone may edit and a rest moves
  more than any one log records, so before offering to undo anything the plan
  checks that the character still stands exactly where the line it means to
  reverse left them. Anything else - a blow taken since, a night's sleep, a
  hand edit - and there is nothing safe to undo, which the sheet says rather
  than guessing.

  What can be taken back:

  | Last operation           | What comes back                          |
  |--------------------------+------------------------------------------|
  | Picked something up      | It goes away again                       |
  | Used or dropped          | It comes back to the container it left   |
  | Packed away              | It goes back where it was                |
  | Bought                   | The item goes, the coin comes back       |
  | Sold                     | The item comes back, the coin goes       |
  | Drank a potion           | The potion and the hit points both       |
  | Hurt, healed, temporary  | The hit points as they stood             |

  A death saving throw is not on the list: the marks a save left are written
  to the sheet but the marks it found are not, so there is nothing to put
  back. The same goes for a rest, which moves hit points, hit dice, slots and
  every feature at once - undoing that is what the rest walkthrough's own
  cancel is for, before it is taken.
EDOC */

import (
	"fmt"
	"strings"
)

// UndoPlan is what the last operation was and whether it can be taken back.
// It is the answer to GET /dnd/undo, so the sheet can label its Undo button
// with what pressing it will actually do.
type UndoPlan struct {
	// Can is whether there is something to undo. When it is false Why says
	// what is in the way, in the words the sheet shows.
	Can  bool   `json:"can"`
	What string `json:"what"`
	Why  string `json:"why"`
	// When is the timestamp of the operation, "2026-09-18 19:32".
	When string `json:"when"`
	// Kind is the operation being undone - the inventory action, or the
	// health action for a change that was only to the hit points.
	Kind string `json:"kind"`
}

// undoOp is the plan with the events it is built from, which is private
// because nothing outside undo wants the log indices.
type undoOp struct {
	plan UndoPlan
	// inv, health and money are indices into the character's logs, or -1 for
	// a log this operation did not write to.
	inv    int
	health int
	money  int
}

// The inventory actions that can be reversed. Wearing and attuning are not
// here because they are never written to the history in the first place.
func undoableInv(action string) bool {
	switch action {
	case InvAdded, InvUsed, InvDropped, InvMoved, InvBought, InvSold:
		return true
	}
	return false
}

// The health actions that can be reversed. The three that settle a death
// saving throw are not among them: the marks the save found are not recorded,
// so there is nothing to put back.
func undoableHealth(action string) bool {
	switch action {
	case "hurt", "healed", "temp", "temp lost", "set":
		return true
	}
	return false
}

// LastUndo works out what taking the last thing back would do, without doing
// any of it. A plan that cannot be carried out says why in the same words the
// sheet shows, so there is one wording rather than two.
func LastUndo(c *Character, rs *Ruleset) UndoPlan {
	return lastUndo(c, rs).plan
}

func lastUndo(c *Character, rs *Ruleset) undoOp {
	op := undoOp{inv: -1, health: -1, money: -1}
	if c == nil {
		op.plan.Why = "there is no character"
		return op
	}
	iv, ivAt := lastOf(len(c.InventoryLog), func(i int) string { return stamp(c.InventoryLog[i].Date, c.InventoryLog[i].Time) })
	hl, hlAt := lastOf(len(c.HealthLog), func(i int) string { return stamp(c.HealthLog[i].Date, c.HealthLog[i].Time) })
	mn, mnAt := lastOf(len(c.MoneyLog), func(i int) string { return stamp(c.MoneyLog[i].Date, c.MoneyLog[i].Time) })
	cd, cdAt := lastOf(len(c.ConditionLog), func(i int) string { return stamp(c.ConditionLog[i].Date, c.ConditionLog[i].Time) })

	if iv < 0 && hl < 0 && mn < 0 {
		op.plan.Why = "nothing has happened yet"
		return op
	}
	// The newest line in any log is where the operation is. A condition
	// coming or going is not something undo reverses, but it is something
	// that has happened since, so it counts here.
	newest := latest(ivAt, hlAt, mnAt, cdAt)
	if cd >= 0 && cdAt == newest &&
		ivAt != newest && hlAt != newest && mnAt != newest {
		op.plan.Why = "the last thing that happened was a condition, which undo does not take back"
		return op
	}

	// Which line is the operation. A potion drunk writes two lines at the
	// same moment - one in the bag, one in the hit points - and those are one
	// operation with the bag as its name. Anything else at the same moment is
	// two operations that happened inside the same minute, and since the logs
	// are stamped to the minute there is no telling which came second. The hit
	// points and the purse go first there, because undoing either of those on
	// its own is always safe and the bag is still there to be undone next.
	invLive := iv >= 0 && ivAt == newest
	healthLive := hl >= 0 && hlAt == newest
	moneyLive := mn >= 0 && mnAt == newest
	if invLive {
		e := c.InventoryLog[iv]
		if healthLive && e.Action == InvUsed && healthFrom(c.HealthLog[hl], e.Item) {
			return undoInvOp(c, rs, iv, hl, -1, newest)
		}
		if moneyLive && (e.Action == InvBought || e.Action == InvSold) {
			return undoInvOp(c, rs, iv, -1, mn, newest)
		}
	}
	if healthLive {
		return undoHealthOp(c, hl, newest)
	}
	if moneyLive {
		return undoMoneyOp(c, mn, newest)
	}
	if invLive {
		return undoInvOp(c, rs, iv, -1, -1, newest)
	}
	op.plan.Why = "there is nothing to undo"
	return op
}

// undoInvOp builds the plan for an operation the bag was part of, gathering up
// the hit points a potion moved or the coin a purchase cost.
func undoInvOp(c *Character, rs *Ruleset, iv, hl, mn int, at string) undoOp {
	op := undoOp{inv: iv, health: -1, money: -1}
	e := c.InventoryLog[iv]
	op.plan.When, op.plan.Kind = at, e.Action
	if !undoableInv(e.Action) {
		op.plan.Why = fmt.Sprintf("%q is not something undo takes back", e.Action)
		return op
	}

	// The healing a potion gave comes off with the potion. Where the hit
	// points have moved since, putting them back would undo more than this
	// one operation, so nothing is touched at all.
	if hl >= 0 {
		h := c.HealthLog[hl]
		if !undoableHealth(h.Action) {
			op.plan.Why = fmt.Sprintf("%q is not something undo takes back", h.Action)
			return op
		}
		if !h.hasBefore {
			op.plan.Why = "this sheet does not record where the hit points stood before that"
			return op
		}
		if c.HPCurrent != h.HPAfter || c.HPTemp != h.TempAfter {
			op.plan.Why = "the hit points have moved since, so putting them back would undo more than this"
			return op
		}
		op.health = hl
	}

	// Buying and selling move the purse at the same moment for the same
	// reason, and are the only inventory lines that do.
	if mn >= 0 {
		if c.Money != c.MoneyLog[mn].Balance {
			op.plan.Why = "the purse has moved since, so putting it back would undo more than this"
			return op
		}
		op.money = mn
	}
	if (e.Action == InvBought || e.Action == InvSold) && op.money < 0 {
		op.plan.Why = "the coin that paid for this is no longer the last line in the purse"
		return op
	}

	op.plan.Can = true
	op.plan.What = undoWhat(c, e, op.health >= 0)
	return op
}

// undoHealthOp builds the plan for a change that was only to the hit points -
// a blow taken, healing given, temporary hit points set or cleared.
func undoHealthOp(c *Character, hl int, at string) undoOp {
	op := undoOp{inv: -1, health: hl, money: -1}
	h := c.HealthLog[hl]
	op.plan.When, op.plan.Kind = at, h.Action
	if !undoableHealth(h.Action) {
		op.plan.Why = fmt.Sprintf("%q is not something undo takes back", h.Action)
		return op
	}
	if !h.hasBefore {
		op.plan.Why = "this sheet does not record where the hit points stood before that"
		return op
	}
	if c.HPCurrent != h.HPAfter || c.HPTemp != h.TempAfter {
		op.plan.Why = "the hit points have moved since"
		return op
	}
	op.plan.Can = true
	op.plan.What = undoHealthWhat(h)
	return op
}

// undoMoneyOp builds the plan for coin that moved on its own, which is the
// coin tab rather than a purchase.
func undoMoneyOp(c *Character, mn int, at string) undoOp {
	op := undoOp{inv: -1, health: -1, money: mn}
	m := c.MoneyLog[mn]
	op.plan.When, op.plan.Kind = at, m.Action
	if c.Money != m.Balance {
		op.plan.Why = "the purse has moved since"
		return op
	}
	if _, ok := moneyBefore(c, mn); !ok {
		op.plan.Why = "there is no record of what the purse held before that"
		return op
	}
	op.plan.Can = true
	op.plan.What = strings.TrimSpace(m.Action + " " + m.Amount.String())
	return op
}

// healthFrom reports whether a health line was written by using the named item
// up. UseItem notes the item on the healing it causes - and the strength, for
// something that comes in more than one - which is the only thread tying the
// two logs together.
func healthFrom(h HealthEvent, item string) bool {
	note := strings.TrimSpace(h.Notes)
	item = strings.TrimSpace(item)
	if note == "" || item == "" {
		return false
	}
	return strings.EqualFold(note, item) ||
		strings.HasPrefix(strings.ToLower(note), strings.ToLower(item)+" (")
}

// undoWhat is what the Undo button says it will do, in the words the log used.
func undoWhat(c *Character, e InventoryEvent, withHealth bool) string {
	what := ""
	switch e.Action {
	case InvAdded:
		what = fmt.Sprintf("picking up %s", qtyName(e.Qty, e.Item))
	case InvUsed:
		what = fmt.Sprintf("using %s", qtyName(e.Qty, e.Item))
	case InvDropped:
		what = fmt.Sprintf("dropping %s", qtyName(e.Qty, e.Item))
	case InvMoved:
		what = fmt.Sprintf("packing %s into %s", qtyName(e.Qty, e.Item), e.To)
	case InvBought:
		what = fmt.Sprintf("buying %s", qtyName(e.Qty, e.Item))
	case InvSold:
		what = fmt.Sprintf("selling %s", qtyName(e.Qty, e.Item))
	default:
		what = e.Action + " " + e.Item
	}
	if withHealth {
		what += " and the hit points it moved"
	}
	return what
}

func undoHealthWhat(h HealthEvent) string {
	switch h.Action {
	case "hurt":
		return fmt.Sprintf("taking %d damage", h.Amount)
	case "healed":
		return fmt.Sprintf("healing %d", h.Amount)
	case "temp":
		return fmt.Sprintf("setting %d temporary hit points", h.TempAfter)
	case "temp lost":
		return "clearing the temporary hit points"
	case "set":
		return fmt.Sprintf("setting the hit points to %d", h.HPAfter)
	}
	return h.Action
}

func qtyName(qty int, item string) string {
	if qty > 1 {
		return fmt.Sprintf("%d %s", qty, item)
	}
	return item
}

// stamp is a log line's moment, comparable as a string because both halves are
// fixed width. A line with no date at all sorts before everything, which is
// what a hand written line with the columns left blank deserves.
func stamp(date, tm string) string {
	return strings.TrimSpace(date) + " " + strings.TrimSpace(tm)
}

// lastOf is the index and stamp of the final entry of a log, or -1 and the
// empty stamp for a log with nothing in it.
func lastOf(n int, at func(int) string) (int, string) {
	if n == 0 {
		return -1, ""
	}
	return n - 1, at(n - 1)
}

func latest(stamps ...string) string {
	out := ""
	for _, s := range stamps {
		if s > out {
			out = s
		}
	}
	return out
}

// moneyBefore is the purse as it stood before a coin line. The line carries
// it, because paying may have broken a gold piece into silver and the value
// alone could not put that back; the line before it says the same thing and
// stands in for a sheet written before the column existed.
func moneyBefore(c *Character, at int) (Money, bool) {
	if c.MoneyLog[at].hasBefore {
		return c.MoneyLog[at].Before, true
	}
	if at > 0 {
		return c.MoneyLog[at-1].Balance, true
	}
	return Money{}, false
}

// ----------------------------------------------------------------------------
// Doing it
// ----------------------------------------------------------------------------

// UndoResult is what taking the last thing back actually did.
type UndoResult struct {
	Plan UndoPlan `json:"plan"`
	Msg  string   `json:"msg"`
}

// ApplyUndo reverses the last operation and takes its lines out of the
// character's history. The character is moved in place; writing the sheet back
// out is the caller's business.
//
// The reversal goes through the same InventoryAdd, InventoryRemove and
// InventoryMove the original change did, so a stack that was merged comes
// apart the way it went together and a container that was emptied is refilled
// by the same rules. Those log what they do, which undo does not want: the
// lines they write are trimmed off along with the ones being undone, because
// an accident should leave no trace rather than leaving a correction.
func ApplyUndo(c *Character, rs *Ruleset) (UndoResult, error) {
	out := UndoResult{}
	op := lastUndo(c, rs)
	out.Plan = op.plan
	if !op.plan.Can {
		why := op.plan.Why
		if why == "" {
			why = "there is nothing to undo"
		}
		return out, fmt.Errorf("%s", why)
	}

	// The bag first. It is the half that can still refuse - a container that
	// has since been dropped has nowhere to put anything back - and nothing
	// else should move if it does.
	if op.inv >= 0 {
		if err := undoInventory(c, rs, c.InventoryLog[op.inv]); err != nil {
			return out, err
		}
	}
	if op.health >= 0 {
		h := c.HealthLog[op.health]
		c.HPCurrent, c.HPTemp = h.HPBefore, h.TempBefore
	}
	if op.money >= 0 {
		before, ok := moneyBefore(c, op.money)
		if !ok {
			return out, fmt.Errorf("there is no record of what the purse held before that")
		}
		c.Money = before
	}

	// Now the history. Each log is trimmed to the line being undone, which
	// also takes off whatever the reversal above wrote.
	if op.inv >= 0 {
		c.InventoryLog = c.InventoryLog[:op.inv]
	}
	if op.health >= 0 {
		c.HealthLog = c.HealthLog[:op.health]
	}
	if op.money >= 0 {
		c.MoneyLog = c.MoneyLog[:op.money]
	}
	out.Msg = "undid " + op.plan.What
	return out, nil
}

// undoInventory puts the bag back the way it was before one line of history.
func undoInventory(c *Character, rs *Ruleset, e InventoryEvent) error {
	qty := maxInt(e.Qty, 1)
	switch e.Action {
	case InvAdded, InvBought:
		to := containerKeyOf(c, rs, e.To)
		if findGear(c, itemIdOf(rs, e.Item), e.Item, to) < 0 {
			return fmt.Errorf("there is no %s in %s to take back", e.Item,
				strings.ToLower(containerLabel(c, rs, to)))
		}
		_, err := InventoryRemove(c, rs, e.Item, qty, to, InvDropped, "")
		return err
	case InvUsed, InvDropped, InvSold:
		from := containerKeyOf(c, rs, e.From)
		_, err := InventoryAdd(c, rs, e.Item, e.Item, qty, from, "")
		return err
	case InvMoved:
		from, to := containerKeyOf(c, rs, e.From), containerKeyOf(c, rs, e.To)
		_, err := InventoryMove(c, rs, e.Item, qty, to, from)
		return err
	}
	return fmt.Errorf("%q is not something undo takes back", e.Action)
}

// itemIdOf is the ruleset id behind a name written in the history, or the
// empty string for homebrew the rules have never heard of.
func itemIdOf(rs *Ruleset, name string) string {
	if rs == nil {
		return ""
	}
	if it := rs.Item(name); it != nil {
		return it.Id
	}
	return ""
}

// containerKeyOf turns the container name a history line carries back into the
// key the equipment table uses. The log names containers for the reader - "the
// Backpack" rather than "backpack" - so undo has to look the name up again.
func containerKeyOf(c *Character, rs *Ruleset, label string) string {
	label = strings.TrimSpace(label)
	if label == "" || strings.EqualFold(label, CarriedLabel) {
		return ""
	}
	for _, g := range c.Equipment {
		it := ruleItem(rs, g)
		if _, isBox := ItemContainer(it); !isBox {
			continue
		}
		if strings.EqualFold(gearName(g, it), label) {
			return gearContainerKey(g, it)
		}
	}
	return ContainerKey(label)
}
