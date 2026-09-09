//lint:file-ignore ST1006 allow the use of self
package dnd

// ----------------------------------------------------------------------------
// Short and long rests
//
// A rest is the one moment in play where most of a character's bookkeeping
// happens at once: hit dice are spent and hit points come back, spell slots
// are refilled, and every feature that recharges hands its uses back. This
// file works out what a given character has to do to take a rest - what the
// prompts are, and what each one may answer - and then applies the answers.
//
// There is no server side state here either. RestPlan reads a character and
// says what is on offer; ApplyRest takes what the player chose and moves the
// character on, and the caller writes the sheet back out. A rest taken twice
// by two windows is two rests, exactly as it would be at the table.
// ----------------------------------------------------------------------------

import (
	"fmt"
	"strings"
)

// The two kinds of rest.
const (
	ShortRest = "short"
	LongRest  = "long"
)

// RestStep is one thing the player is walked through while resting. The
// character sheet renders the steps in order and collects the answers the
// ones that ask for something need.
type RestStep struct {
	// Id names the step. The only ones that carry an answer are "hitdice"
	// (how many hit dice were spent, and what they rolled) and "note" (a
	// line for the session log); the rest are read and acknowledged.
	Id    string `json:"id"`
	Title string `json:"title"`
	Text  string `json:"text"`
	// Kind is how the sheet should draw the step: "info" is prose to read,
	// "hitdice" is the hit dice spender, "note" is a free text line.
	Kind string `json:"kind"`
	// Max, Die and Mod describe the hit dice step: how many are left, what
	// each one rolls, and what constitution adds to each.
	Max int    `json:"max,omitempty"`
	Die string `json:"die,omitempty"`
	Mod int    `json:"mod,omitempty"`
}

// RestRecharge is one thing this rest will give back, listed so the player
// can see what they are about to get before they take it.
type RestRecharge struct {
	Name  string `json:"name"`
	Spent int    `json:"spent"`
	Max   int    `json:"max"`
	Kind  string `json:"kind"` // "feature", "slots", "hitdice", "hp"
}

// RestPlanView is everything the sheet needs to run one rest.
type RestPlanView struct {
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	Duration string `json:"duration"`
	Summary  string `json:"summary"`

	HPCurrent int `json:"hpCurrent"`
	HPMax     int `json:"hpMax"`
	HPTemp    int `json:"hpTemp"`

	// HitDice is the pool as it is written on the sheet ("5d10"), HitDiceMax
	// how many there are in total and HitDiceLeft how many are unspent.
	HitDice     string `json:"hitDice"`
	HitDiceMax  int    `json:"hitDiceMax"`
	HitDiceLeft int    `json:"hitDiceLeft"`
	// HitDieFaces is the die a single hit die rolls. A multiclass character
	// has more than one size of hit die; the sheet spends the largest, and
	// HitDiceNote says so.
	HitDieFaces int    `json:"hitDieFaces"`
	HitDiceNote string `json:"hitDiceNote"`
	ConMod      int    `json:"conMod"`

	SlotsUsed int            `json:"slotsUsed"`
	Steps     []RestStep     `json:"steps"`
	Recharges []RestRecharge `json:"recharges"`
}

// RestRequest is what the sheet sends back when the player finishes resting.
type RestRequest struct {
	Filename string `json:"filename"`
	Id       string `json:"id"`
	// Kind is "short" or "long".
	Kind string `json:"kind"`
	// HitDiceSpent is how many hit dice were rolled on a short rest, and
	// HitPointsHealed the total those dice came to. Both are what the sheet
	// actually rolled, so the dice on screen and the sheet agree.
	HitDiceSpent    int `json:"hitDiceSpent"`
	HitPointsHealed int `json:"hitPointsHealed"`
	// Note is an optional line for the session log.
	Note string `json:"note"`
}

// RestResult is what the rest did, in the order it did it. Lines is written
// straight into the session log; the rest is what the sheet redraws from.
type RestResult struct {
	Kind      string   `json:"kind"`
	Name      string   `json:"name"`
	Lines     []string `json:"lines"`
	Summary   string   `json:"summary"`
	HPBefore  int      `json:"hpBefore"`
	HPAfter   int      `json:"hpAfter"`
	HPMax     int      `json:"hpMax"`
	Healed    int      `json:"healed"`
	DiceSpent int      `json:"diceSpent"`
	// DiceBack is how many hit dice a long rest gave back.
	DiceBack     int      `json:"diceBack"`
	SlotsBack    int      `json:"slotsBack"`
	FeaturesBack []string `json:"featuresBack"`
}

// RestState is what a rest call answers with: the character, the plan for
// next time, and - after a rest was taken - what it did.
type RestState struct {
	Id       string        `json:"id"`
	Name     string        `json:"name"`
	Filename string        `json:"filename"`
	Ruleset  string        `json:"ruleset"`
	Short    *RestPlanView `json:"short"`
	Long     *RestPlanView `json:"long"`
	Result   *RestResult   `json:"result,omitempty"`
	Msg      string        `json:"msg,omitempty"`
}

// RestName is the rest written the way it is spoken about.
func RestName(kind string) string {
	if kind == ShortRest {
		return "Short Rest"
	}
	return "Long Rest"
}

// NormalizeRestKind reads whatever the caller sent as one of the two rests.
func NormalizeRestKind(kind string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case ShortRest, "s", "short rest":
		return ShortRest, nil
	case LongRest, "l", "long rest", "":
		return LongRest, nil
	}
	return "", fmt.Errorf("a rest is either %q or %q, not %q", ShortRest, LongRest, kind)
}

// hitDicePool is the whole pool of hit dice a character has: how many there
// are, the largest die among them, and the pool written out.
func hitDicePool(c *Character, rs *Ruleset) (total int, faces int, mixed bool) {
	sizes := map[int]bool{}
	for _, cl := range c.Classes {
		die := 8
		if cls := rs.Class(cl.Class); cls != nil && cls.HitDie > 0 {
			die = cls.HitDie
		}
		total += cl.Level
		sizes[die] = true
		if die > faces {
			faces = die
		}
	}
	if faces == 0 {
		faces = 8
	}
	return total, faces, len(sizes) > 1
}

// limitedTraits is every trait and feature on the sheet that has a use limit,
// with the duplicates that share a pool folded together.
func limitedTraits(s *Sheet) []Trait {
	out := []Trait{}
	at := map[string]int{}
	for _, t := range append(append([]Trait{}, s.Traits...), s.Features...) {
		if t.UsesMax <= 0 {
			continue
		}
		key := UsesKey(t)
		if i, ok := at[key]; ok {
			// The higher level wording of the same feature is the one that
			// describes the pool as it now stands.
			if t.UsesMax > out[i].UsesMax {
				out[i] = t
			}
			continue
		}
		at[key] = len(out)
		out = append(out, t)
	}
	return out
}

// RestPlan works out what one kind of rest is going to ask of this character
// and what it will give back.
func RestPlan(c *Character, rs *Ruleset, kind string) *RestPlanView {
	kind, err := NormalizeRestKind(kind)
	if err != nil {
		kind = LongRest
	}
	s := Compute(c, rs)
	total, faces, mixed := hitDicePool(c, rs)
	left := total - c.HitDiceUsed
	if left < 0 {
		left = 0
	}
	conMod := s.AbilityMap[CON].Modifier

	plan := &RestPlanView{
		Kind: kind, Name: RestName(kind),
		HPCurrent: s.HPCurrent, HPMax: s.HPMax, HPTemp: c.HPTemp,
		HitDice: s.HitDice, HitDiceMax: total, HitDiceLeft: left,
		HitDieFaces: faces, ConMod: conMod,
	}
	if mixed {
		plan.HitDiceNote = "You have more than one size of hit die; " +
			"the largest is spent first."
	}
	for _, n := range c.SlotsUsed {
		plan.SlotsUsed += n
	}

	limited := limitedTraits(s)
	for _, t := range limited {
		if t.UsesSpent <= 0 {
			continue
		}
		if kind == ShortRest && t.Recharge != RechargeShort {
			continue
		}
		plan.Recharges = append(plan.Recharges, RestRecharge{
			Name: t.Name, Spent: t.UsesSpent, Max: t.UsesMax, Kind: "feature",
		})
	}

	if kind == ShortRest {
		plan.Duration = "at least 1 hour"
		plan.Summary = "An hour of doing nothing more strenuous than eating, " +
			"drinking, reading and tending to wounds."
		plan.Steps = []RestStep{
			{Id: "settle", Kind: "info", Title: "Take an hour",
				Text: "A short rest is a period of at least 1 hour spent doing " +
					"nothing more strenuous than eating, drinking, reading and " +
					"tending to wounds. If the rest is interrupted by anything " +
					"needing at least an hour of your attention, you gain none " +
					"of its benefits."},
			{Id: "hitdice", Kind: "hitdice",
				Title: "Spend hit dice", Max: left, Die: fmt.Sprintf("d%d", faces),
				Mod: conMod,
				Text: fmt.Sprintf("You may spend any number of your %d remaining "+
					"hit dice. Each one rolls d%d %s and gives you back that "+
					"many hit points, to a minimum of 0. You choose after each "+
					"roll whether to spend another.", left, faces, Signed(conMod))},
		}
		if plan.SlotsUsed > 0 && isPactOnlyCaster(c, rs) {
			plan.Steps = append(plan.Steps, RestStep{
				Id: "pact", Kind: "info", Title: "Pact magic",
				Text: "Your pact magic slots come back on a short rest, so all " +
					"of them are refilled.",
			})
			plan.Recharges = append(plan.Recharges, RestRecharge{
				Name: "Pact magic slots", Spent: plan.SlotsUsed, Kind: "slots",
			})
		} else if back := slotsShortRestReturns(c); back > 0 {
			// An hour with your book open is worth one slot of every level
			// you have spent one at. This is a house rule: by the book a
			// short rest returns no slots at all to anyone but a warlock.
			plan.Steps = append(plan.Steps, RestStep{
				Id: "slots", Kind: "info", Title: "Spell slots",
				Text: fmt.Sprintf("You get back one spell slot of each level "+
					"you have spent one at: %d slot%s in all.",
					back, plural(back, "", "s")),
			})
			plan.Recharges = append(plan.Recharges, RestRecharge{
				Name: "Spell slots", Spent: back, Kind: "slots",
			})
		}
		plan.Steps = append(plan.Steps, restFeatureStep(plan.Recharges))
		return plan
	}

	plan.Duration = "at least 8 hours"
	plan.Summary = "Eight hours of sleep or light activity: hit points, half " +
		"your hit dice, every spell slot and every feature come back."
	// Half the total pool, rounded down, and never fewer than one.
	back := total / 2
	if back < 1 {
		back = 1
	}
	if back > c.HitDiceUsed {
		back = c.HitDiceUsed
	}
	plan.Steps = []RestStep{
		{Id: "settle", Kind: "info", Title: "Take eight hours",
			Text: "A long rest is a period of at least 8 hours of sleep or light " +
				"activity - reading, talking, eating, standing watch - of which " +
				"no more than 2 hours may be the light activity. Interrupt it " +
				"with an hour of walking, fighting, casting spells or any other " +
				"strenuous activity and you must begin the rest again. You " +
				"cannot benefit from more than one long rest in a 24 hour period."},
		{Id: "hp", Kind: "info", Title: "Hit points",
			Text: fmt.Sprintf("You regain all your hit points: %d of %d.",
				s.HPMax, s.HPMax)},
		{Id: "hitdice", Kind: "info", Title: "Hit dice",
			Text: fmt.Sprintf("You regain %d spent hit die%s, half your total of "+
				"%d rounded down, to a minimum of one. You have spent %d.",
				back, plural(back, "", "s"), total, c.HitDiceUsed)},
	}
	if plan.SlotsUsed > 0 {
		plan.Steps = append(plan.Steps, RestStep{
			Id: "slots", Kind: "info", Title: "Spell slots",
			Text: fmt.Sprintf("All %d expended spell slot%s come back.",
				plan.SlotsUsed, plural(plan.SlotsUsed, "", "s")),
		})
		plan.Recharges = append(plan.Recharges, RestRecharge{
			Name: "Spell slots", Spent: plan.SlotsUsed, Kind: "slots",
		})
	}
	if c.HitDiceUsed > 0 {
		plan.Recharges = append(plan.Recharges, RestRecharge{
			Name: "Hit dice", Spent: back, Max: total, Kind: "hitdice",
		})
	}
	plan.Steps = append(plan.Steps, restFeatureStep(plan.Recharges))
	return plan
}

// restFeatureStep is the closing page of the walk through: what is coming
// back, written out.
func restFeatureStep(list []RestRecharge) RestStep {
	if len(list) == 0 {
		return RestStep{Id: "recharge", Kind: "info", Title: "Nothing to recover",
			Text: "You have nothing spent that this rest gives back."}
	}
	parts := []string{}
	for _, r := range list {
		if r.Kind == "feature" {
			parts = append(parts, fmt.Sprintf("%s (%d of %d)", r.Name, r.Spent, r.Max))
			continue
		}
		parts = append(parts, fmt.Sprintf("%s (%d)", r.Name, r.Spent))
	}
	return RestStep{Id: "recharge", Kind: "info", Title: "What comes back",
		Text: strings.Join(parts, ", ") + "."}
}

// isPactOnlyCaster says whether every spell slot this character has is a
// warlock's, which is what decides if a short rest refills them. A warlock
// who is also a wizard has both kinds in one row of pips and there is no
// telling them apart, so their slots are left for the long rest.
func isPactOnlyCaster(c *Character, rs *Ruleset) bool {
	pact, other := false, false
	for _, cl := range c.Classes {
		cls := rs.Class(cl.Class)
		if cls == nil {
			continue
		}
		info := cls.Spellcasting
		if sub := rs.Subclass(cl.Class, cl.Subclass); sub != nil && sub.Spellcasting != nil {
			info = sub.Spellcasting
		}
		if info == nil || info.Progression == "" || info.Progression == "none" {
			continue
		}
		if info.StartLevel > 0 && cl.Level < info.StartLevel {
			continue
		}
		if info.Progression == "pact" {
			pact = true
		} else {
			other = true
		}
	}
	return pact && !other
}

// ApplyRest takes a rest and moves the character on. It answers with what it
// did, which is what the session log records and the sheet redraws from.
//
// Nothing about it is random: the dice a short rest rolls are rolled by the
// sheet, in the open, and arrive here as a total. That keeps the roll on the
// table where the player can see it and keeps this function replayable.
func ApplyRest(c *Character, rs *Ruleset, req RestRequest) (*RestResult, error) {
	kind, err := NormalizeRestKind(req.Kind)
	if err != nil {
		return nil, err
	}
	before := Compute(c, rs)
	total, _, _ := hitDicePool(c, rs)
	res := &RestResult{
		Kind: kind, Name: RestName(kind),
		HPBefore: before.HPCurrent, HPMax: before.HPMax,
	}
	if c.HPMax <= 0 {
		c.HPMax = before.HPMax
	}

	if kind == ShortRest {
		spent := req.HitDiceSpent
		if spent < 0 {
			spent = 0
		}
		if left := total - c.HitDiceUsed; spent > left {
			return nil, fmt.Errorf("you have %d hit dice left, not %d", left, spent)
		}
		healed := req.HitPointsHealed
		if healed < 0 {
			healed = 0
		}
		if spent == 0 {
			healed = 0
		}
		c.HitDiceUsed += spent
		hp := before.HPCurrent + healed
		if hp > before.HPMax {
			hp = before.HPMax
		}
		c.HPCurrent = hp
		res.DiceSpent = spent
		res.Healed = hp - before.HPCurrent
		res.HPAfter = hp
		if isPactOnlyCaster(c, rs) {
			for _, n := range c.SlotsUsed {
				res.SlotsBack += n
			}
			c.SlotsUsed = nil
		} else {
			// One slot of each level you have spent one at, which is the
			// house rule this table plays with.
			res.SlotsBack = RecoverOneSlotEachLevel(c)
		}
	} else {
		c.HPCurrent = before.HPMax
		c.HPTemp = 0
		c.DeathSaves = ""
		res.HPAfter = before.HPMax
		res.Healed = before.HPMax - before.HPCurrent
		// Half the total pool, rounded down, and never fewer than one.
		back := total / 2
		if back < 1 {
			back = 1
		}
		if back > c.HitDiceUsed {
			back = c.HitDiceUsed
		}
		c.HitDiceUsed -= back
		res.DiceBack = back
		for _, n := range c.SlotsUsed {
			res.SlotsBack += n
		}
		c.SlotsUsed = nil
	}

	// Every feature this rest recharges hands its uses back. A short rest
	// gives back only what recharges on one; a long rest gives back the lot.
	for _, t := range limitedTraits(before) {
		if t.UsesSpent <= 0 {
			continue
		}
		if kind == ShortRest && t.Recharge != RechargeShort {
			continue
		}
		res.FeaturesBack = append(res.FeaturesBack,
			fmt.Sprintf("%s (%d of %d)", t.Name, t.UsesSpent, t.UsesMax))
		delete(c.UsesSpent, UsesKey(t))
	}
	if len(c.UsesSpent) == 0 {
		c.UsesSpent = nil
	}

	res.Lines = restLines(res, req.Note)
	res.Summary = res.Lines[0]
	return res, nil
}

// restLines writes the rest out for the session log, in org markup.
func restLines(res *RestResult, note string) []string {
	head := fmt.Sprintf("*%s.* Hit points %d of %d", res.Name, res.HPAfter, res.HPMax)
	if res.Healed > 0 {
		head += fmt.Sprintf(", %d regained", res.Healed)
	}
	head += "."
	lines := []string{head}
	if res.DiceSpent > 0 {
		lines = append(lines, fmt.Sprintf("- Spent %d hit di%s.",
			res.DiceSpent, plural(res.DiceSpent, "e", "ce")))
	}
	if res.DiceBack > 0 {
		lines = append(lines, fmt.Sprintf("- Recovered %d hit di%s.",
			res.DiceBack, plural(res.DiceBack, "e", "ce")))
	}
	if res.SlotsBack > 0 {
		lines = append(lines, fmt.Sprintf("- Recovered %d spell slot%s.",
			res.SlotsBack, plural(res.SlotsBack, "", "s")))
	}
	for _, f := range res.FeaturesBack {
		lines = append(lines, "- Recovered "+f+".")
	}
	if strings.TrimSpace(note) != "" {
		lines = append(lines, strings.TrimSpace(note))
	}
	return lines
}

// slotsShortRestReturns is how many slots a short rest hands back under the
// one-of-each-level rule: one for every level with something spent at it.
func slotsShortRestReturns(c *Character) int {
	n := 0
	for _, used := range c.SlotsUsed {
		if used > 0 {
			n++
		}
	}
	return n
}
