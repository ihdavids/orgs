//lint:file-ignore ST1006 allow the use of self
package dnd

// ----------------------------------------------------------------------------
// Hit points
//
// Taking damage, being healed, and the temporary hit points that sit in front
// of both. A rest is the tidy, once a night version of this (see rest.go);
// this is the rest of the evening, one blow at a time.
//
// As everywhere else in this module there is no server side state: the org
// character sheet is where the hit points live, so the page, the file and
// anything else reading it never disagree about how badly hurt someone is.
// ----------------------------------------------------------------------------

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// The actions a health call may ask for.
const (
	Hurt      = "hurt"
	Heal      = "heal"
	SetTemp   = "temp"
	ClearTemp = "cleartemp"
	SetHP     = "set"
	// DeathSave marks one of the three successes or three failures a dying
	// character collects, Stabilize ends the dying outright, and ClearDeath
	// wipes the marks without changing the hit points.
	DeathSave  = "deathsave"
	Stabilize  = "stabilize"
	ClearDeath = "cleardeath"
)

// The readings of a death saving throw. A natural 20 brings you round on one
// hit point, a natural 1 counts as two failures, and DC 10 decides the rest.
const (
	SaveSuccess  = "success"
	SaveFailure  = "failure"
	SaveCritical = "critical"
	SaveFumble   = "fumble"
)

// DeathSaveDC is the flat DC of a death saving throw, and DeathSaveMarks how
// many of either kind it takes to settle the matter.
const (
	DeathSaveDC    = 10
	DeathSaveMarks = 3
)

// HealthEvent is one line of the sheet's Health History: a hit taken, healing
// received, or the temporary hit points being changed.
type HealthEvent struct {
	Date string `json:"date"`
	Time string `json:"time"`
	// Action is "hurt", "healed", "temp", "temp lost" or "set".
	Action string `json:"action"`
	// Amount is how much was asked for. Absorbed is how much of a hit the
	// temporary hit points soaked up, so the history can say a 9 point hit
	// cost 5 temporary and 4 real ones.
	Amount   int `json:"amount"`
	Absorbed int `json:"absorbed"`
	// Type is the kind of damage a blow was, Raw what it would have been
	// before the character's defenses were applied, and Defense which of
	// them applied - "resistance", "immunity" or "vulnerability". Raw is
	// only set when a defense actually changed the number.
	Type    string `json:"type,omitempty"`
	Raw     int    `json:"raw,omitempty"`
	Defense string `json:"defense,omitempty"`
	// Where the character stood before and after.
	HPBefore   int `json:"hpBefore"`
	HPAfter    int `json:"hpAfter"`
	TempBefore int `json:"tempBefore"`
	TempAfter  int `json:"tempAfter"`
	HPMax      int `json:"hpMax"`
	// Down is true when the hit put the character on nothing at all.
	Down bool `json:"down"`
	// DeathSaves is the marks as they stood after the change, so a line of
	// history about a death saving throw can say what it came to.
	DeathSaves string `json:"deathSaves,omitempty"`
	Notes      string `json:"notes"`
	// hasBefore says whether HPBefore and TempBefore are a real reading or
	// merely zero. A line just applied always has them; one read back off a
	// sheet written before the Was column existed does not, and undo must not
	// mistake an absent reading for a character who was on nothing at all.
	hasBefore bool
}

// HasBefore reports whether this line knows where the character stood before
// it, which is what decides whether it can be taken back.
func (e HealthEvent) HasBefore() bool { return e.hasBefore }

// HealthView is the hit point line as the sheet draws it.
type HealthView struct {
	Current int `json:"current"`
	Max     int `json:"max"`
	Temp    int `json:"temp"`
	// Percent is how full the bar is, and TempPercent how far past the end of
	// it the temporary hit points reach.
	Percent     int `json:"percent"`
	TempPercent int `json:"tempPercent"`
	// Level is how the bar is coloured: "hale", "hurt", "bloodied" or
	// "dying", worked out here rather than in the sheet so the page and the
	// exported html always band the same hit points the same way.
	Level string `json:"level"`
	// Down is true at zero hit points, Bloodied at half or less.
	Down     bool `json:"down"`
	Bloodied bool `json:"bloodied"`
	// DeathSaves is the marks on the sheet, kept here so one answer redraws
	// the whole line.
	DeathSaves string `json:"deathSaves"`
	// The same marks counted out, which is what the sheet draws as pips:
	// three successes and you are stable, three failures and you are dead.
	DeathSuccesses int `json:"deathSuccesses"`
	DeathFailures  int `json:"deathFailures"`
	// Dying is true while the character is down and the matter is unsettled,
	// which is when the pips are worth drawing at all.
	Dying  bool `json:"dying"`
	Stable bool `json:"stable"`
	Dead   bool `json:"dead"`
	// DeathPips is 1..DeathSaveMarks, so a template that cannot count to three
	// can still draw one mark per save. The same list does both rows: a mark is
	// filled when its number is at or under the count.
	DeathPips []int `json:"deathPips"`
}

// HealthRequest is one change to a character's hit points, posted by the html
// character sheet.
type HealthRequest struct {
	Filename string `json:"filename"`
	Id       string `json:"id"`
	// Action is hurt, heal, temp, cleartemp or set.
	Action string `json:"action"`
	// Amount is how many hit points, and is never negative: which way they
	// go is the action's business.
	Amount int `json:"amount"`
	// Type is what kind of damage it was - "fire", "necrotic" - for a hurt.
	// It is what lets the character's own resistances and immunities be
	// applied rather than left as decoration under Defenses, and what the
	// page colours the blow with. Empty is untyped damage, which nothing
	// resists.
	Type string `json:"type"`
	// Result is what a death saving throw came to, said in words:
	// "success", "failure", "critical" or "fumble". Roll is the same thing
	// said as the number on the d20, which the sheet has to hand because it
	// threw it - either will do, and Result wins when both are given.
	Result string `json:"result"`
	Roll   int    `json:"roll"`
	Notes  string `json:"notes"`
}

// HealthState is the answer to every health call.
type HealthState struct {
	Id       string        `json:"id"`
	Name     string        `json:"name"`
	Filename string        `json:"filename"`
	Ruleset  string        `json:"ruleset"`
	HP       HealthView    `json:"hp"`
	History  []HealthEvent `json:"history"`
	Msg      string        `json:"msg"`
	// Concentration is what the character is still holding, which a blow can
	// cost them - so the hit point answer carries it rather than making the
	// sheet ask a second question after every hit.
	Concentration ConcentrationView `json:"concentration"`
	// Save is the concentration check the blow just called for, nil when
	// there was nothing to hold or nothing landed.
	Save *ConcentrationSave `json:"save,omitempty"`
}

// ComputeHealth is the hit point line worked out from a computed sheet.
func ComputeHealth(s *Sheet) HealthView {
	v := HealthView{}
	if s == nil {
		return v
	}
	v.Current = s.HPCurrent
	v.Max = s.HPMax
	v.Temp = s.HPTemp
	v.DeathSaves = s.DeathSaves
	if v.Max > 0 {
		v.Percent = clampPercent(v.Current * 100 / v.Max)
		v.TempPercent = clampPercent(v.Temp * 100 / v.Max)
		v.Bloodied = v.Current*2 <= v.Max
	}
	v.Down = v.Current <= 0
	v.Level = HealthLevel(v.Percent)
	v.DeathSuccesses, v.DeathFailures = ParseDeathSaves(v.DeathSaves)
	v.Stable = v.Down && v.DeathSuccesses >= DeathSaveMarks
	v.Dead = v.DeathFailures >= DeathSaveMarks
	v.Dying = v.Down && !v.Stable && !v.Dead
	for i := 1; i <= DeathSaveMarks; i++ {
		v.DeathPips = append(v.DeathPips, i)
	}
	return v
}

// ParseDeathSaves counts the marks out of the DND_DEATH_SAVES property, which
// is written "successes/failures" - the form the D&D Beyond import and the
// builder both already use. A value in any other shape counts as no marks at
// all rather than as an error: the property is editable by hand, and the first
// death save rolled writes it back in the canonical form.
func ParseDeathSaves(val string) (int, int) {
	val = strings.TrimSpace(val)
	if val == "" {
		return 0, 0
	}
	parts := strings.SplitN(val, "/", 2)
	succ, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0
	}
	fail := 0
	if len(parts) == 2 {
		if n, err := strconv.Atoi(strings.TrimSpace(parts[1])); err == nil {
			fail = n
		}
	}
	return clampMarks(succ), clampMarks(fail)
}

// FormatDeathSaves writes the marks back. No marks at all is written as
// nothing rather than as "0/0", so a character who has never been down and one
// who has been brought round read the same.
func FormatDeathSaves(succ, fail int) string {
	succ, fail = clampMarks(succ), clampMarks(fail)
	if succ == 0 && fail == 0 {
		return ""
	}
	return fmt.Sprintf("%d/%d", succ, fail)
}

func clampMarks(n int) int {
	if n < 0 {
		return 0
	}
	if n > DeathSaveMarks {
		return DeathSaveMarks
	}
	return n
}

// HealthLevel is the band a hit point bar falls in, which is what colours it:
// green while you are hale, yellow once you have been hit, orange at half and
// red when you are nearly out.
func HealthLevel(percent int) string {
	switch {
	case percent < 25:
		return "dying"
	case percent < 50:
		return "bloodied"
	case percent < 75:
		return "hurt"
	}
	return "hale"
}

func clampPercent(n int) int {
	if n < 0 {
		return 0
	}
	if n > 100 {
		return 100
	}
	return n
}

// ApplyHealth applies one change to a character's hit points and returns what
// it did. The character is moved on in place; writing the sheet back out is
// the caller's business.
//
// Damage comes off the temporary hit points first and only then off the real
// ones, which is the order the rules put them in. Healing never touches the
// temporary ones - they are not hit points that can be restored, they are a
// buffer that is used up - and never carries a character past their maximum.
func ApplyHealth(c *Character, rs *Ruleset, req HealthRequest) (HealthEvent, error) {
	action := strings.ToLower(strings.TrimSpace(req.Action))
	e := HealthEvent{Action: action, Notes: strings.TrimSpace(req.Notes)}
	if c == nil {
		return e, fmt.Errorf("no character")
	}
	before := Compute(c, rs)
	max := before.HPMax
	if c.HPMax <= 0 {
		c.HPMax = max
	}
	cur := before.HPCurrent
	temp := c.HPTemp
	e.HPBefore, e.TempBefore, e.HPMax = cur, temp, max

	amount := req.Amount
	if amount < 0 {
		amount = -amount
	}

	switch action {
	case Hurt, "damage", "hit":
		if amount == 0 {
			return e, fmt.Errorf("how much damage?")
		}
		e.Action = "hurt"
		// What the character shrugs off comes first: resistance halves the
		// blow, vulnerability doubles it and immunity stops it, all before
		// the temporary hit points get a look at what is left. That is the
		// order the rules put them in, and it matters - resistance applied
		// after the buffer would let a resistant character soak twice.
		raw := amount
		e.Type, e.Defense, amount = DefendDamage(c, rs, req.Type, amount)
		if amount != raw {
			e.Raw = raw
		}
		if amount == 0 {
			// Immunity is not a refusal. It is a hit that did nothing, and
			// is worth a line in the history saying so.
			e.Amount = 0
			break
		}
		e.Amount = amount
		// The buffer goes first, and only what is left over draws blood.
		soaked := temp
		if soaked > amount {
			soaked = amount
		}
		e.Absorbed = soaked
		temp -= soaked
		cur -= amount - soaked
		if cur < 0 {
			cur = 0
		}

	case Heal, "healed", "cure":
		if amount == 0 {
			return e, fmt.Errorf("how much healing?")
		}
		if cur >= max {
			return e, fmt.Errorf("you are already at full hit points")
		}
		e.Action = "healed"
		cur += amount
		if cur > max {
			cur = max
		}
		e.Amount = cur - e.HPBefore
		// Healing off zero is being brought round, so the death saves go.
		if e.HPBefore <= 0 && cur > 0 {
			c.DeathSaves = ""
		}

	case SetTemp, "temporary":
		if amount == temp {
			return e, fmt.Errorf("you already have %d temporary hit points", temp)
		}
		e.Action = "temp"
		if amount < temp {
			e.Action = "temp lost"
		}
		e.Amount = amount
		temp = amount

	case ClearTemp, "clear":
		if temp == 0 {
			return e, fmt.Errorf("you have no temporary hit points")
		}
		e.Action = "temp lost"
		e.Amount = 0
		temp = 0

	case SetHP:
		if amount > max {
			amount = max
		}
		if amount == cur {
			return e, fmt.Errorf("you are already on %d hit points", cur)
		}
		e.Action = "set"
		e.Amount = amount
		if amount > 0 && cur <= 0 {
			c.DeathSaves = ""
		}
		cur = amount

	case DeathSave, "save", "death":
		// A death save is only a question while the character is down. Above
		// nothing at all there is nothing to save against.
		if cur > 0 {
			return e, fmt.Errorf("you are on %d hit points, not dying", cur)
		}
		succ, fail := ParseDeathSaves(c.DeathSaves)
		if succ >= DeathSaveMarks || fail >= DeathSaveMarks {
			return e, fmt.Errorf("the death saves are already settled")
		}
		result := deathSaveResult(req)
		switch result {
		case SaveCritical:
			// A natural 20 is not a success, it is standing back up.
			e.Action = "revived"
			e.Amount = 1
			cur = 1
			c.DeathSaves = ""
		case SaveFumble:
			e.Action = "death save"
			fail += 2
			c.DeathSaves = FormatDeathSaves(succ, fail)
		case SaveSuccess:
			e.Action = "death save"
			succ++
			c.DeathSaves = FormatDeathSaves(succ, fail)
		case SaveFailure:
			e.Action = "death save"
			fail++
			c.DeathSaves = FormatDeathSaves(succ, fail)
		default:
			return e, fmt.Errorf(
				"unknown death save %q, expected success, failure, critical or fumble",
				req.Result)
		}
		e.Notes = deathSaveNote(e.Notes, result, req.Roll)

	case Stabilize, "stable":
		if cur > 0 {
			return e, fmt.Errorf("you are on %d hit points, not dying", cur)
		}
		succ, fail := ParseDeathSaves(c.DeathSaves)
		if fail >= DeathSaveMarks {
			return e, fmt.Errorf("it is too late to stabilise")
		}
		if succ >= DeathSaveMarks {
			return e, fmt.Errorf("you are already stable")
		}
		e.Action = "stabilized"
		c.DeathSaves = FormatDeathSaves(DeathSaveMarks, fail)

	case ClearDeath, "cleardeathsaves":
		if c.DeathSaves == "" {
			return e, fmt.Errorf("there are no death saves to clear")
		}
		e.Action = "death saves cleared"
		c.DeathSaves = ""

	default:
		return e, fmt.Errorf(
			"unknown action %q, expected hurt, heal, temp, cleartemp, set, "+
				"deathsave, stabilize or cleardeath", req.Action)
	}

	c.HPCurrent = cur
	c.HPTemp = temp
	e.HPAfter, e.TempAfter = cur, temp
	e.Down = cur <= 0
	e.DeathSaves = c.DeathSaves
	// Being knocked out ends concentration: an unconscious caster is holding
	// nothing. This is the one break that needs no saving throw.
	if e.Down && c.Concentration != nil {
		c.Concentration = nil
	}
	return logHealth(c, e), nil
}

// deathSaveResult is what the sheet said the save came to. A reading in words
// is taken as given; a bare d20 is read against the flat DC of 10, with the
// natural 20 and the natural 1 doing what the rules say they do.
func deathSaveResult(req HealthRequest) string {
	switch strings.ToLower(strings.TrimSpace(req.Result)) {
	case SaveSuccess, "succeeded", "made":
		return SaveSuccess
	case SaveFailure, "failed", "fail":
		return SaveFailure
	case SaveCritical, "crit", "nat20":
		return SaveCritical
	case SaveFumble, "nat1", "critical failure":
		return SaveFumble
	case "":
		// Nothing said in words, so read the die.
	default:
		return ""
	}
	if req.Roll <= 0 {
		return ""
	}
	switch {
	case req.Roll >= 20:
		return SaveCritical
	case req.Roll <= 1:
		return SaveFumble
	case req.Roll >= DeathSaveDC:
		return SaveSuccess
	}
	return SaveFailure
}

// deathSaveNote keeps the number the die landed on in the history, since that
// is the one thing the marks themselves cannot say afterwards.
func deathSaveNote(notes, result string, roll int) string {
	if roll <= 0 {
		return notes
	}
	said := "rolled " + strconv.Itoa(roll)
	switch result {
	case SaveCritical:
		said += ", a natural 20"
	case SaveFumble:
		said += ", a natural 1"
	}
	if notes == "" {
		return said
	}
	return notes + " - " + said
}

// logHealth stamps an event with the time and appends it to the character's
// health history, which is what gets written into the Health History section.
func logHealth(c *Character, e HealthEvent) HealthEvent {
	// A line being written now always knows where it started from.
	e.hasBefore = true
	now := time.Now()
	e.Date = now.Format("2006-01-02")
	e.Time = now.Format("15:04")
	c.HealthLog = append(c.HealthLog, e)
	return e
}

// HealthEventMsg is the one line the sheet shows after a change.
func HealthEventMsg(e HealthEvent) string {
	switch e.Action {
	case "hurt":
		what := "damage"
		if e.Type != "" {
			what = strings.ToLower(e.Type)
		}
		// A blow the character shrugs off says so: the number that landed is
		// not the number that was rolled, and the history should not look
		// like the arithmetic went wrong.
		if e.Defense == Immunity {
			return fmt.Sprintf("immune to %s - %d shrugged off", what, e.Raw)
		}
		msg := fmt.Sprintf("took %d %s", e.Amount, what)
		switch e.Defense {
		case Resistance:
			msg += fmt.Sprintf(", resisted from %d", e.Raw)
		case Vulnerability:
			msg += fmt.Sprintf(", doubled from %d", e.Raw)
		}
		if e.Absorbed > 0 {
			msg += fmt.Sprintf(", %d soaked up", e.Absorbed)
		}
		if e.Down {
			return msg + " and went down"
		}
		return msg + fmt.Sprintf(", %d of %d left", e.HPAfter, e.HPMax)
	case "healed":
		return fmt.Sprintf("healed %d, %d of %d", e.Amount, e.HPAfter, e.HPMax)
	case "temp":
		return fmt.Sprintf("%d temporary hit points", e.TempAfter)
	case "temp lost":
		if e.TempAfter == 0 {
			return "temporary hit points gone"
		}
		return fmt.Sprintf("temporary hit points down to %d", e.TempAfter)
	case "set":
		return fmt.Sprintf("hit points set to %d of %d", e.HPAfter, e.HPMax)
	case "death save":
		succ, fail := ParseDeathSaves(e.DeathSaves)
		if fail >= DeathSaveMarks {
			return fmt.Sprintf("%d of 3 failures - you are gone", fail)
		}
		if succ >= DeathSaveMarks {
			return "three successes, you are stable"
		}
		return fmt.Sprintf("death saves %d success%s, %d failure%s",
			succ, plural(succ, "", "es"), fail, plural(fail, "", "s"))
	case "revived":
		return "a natural 20 - back up on 1 hit point"
	case "stabilized":
		return "stabilised at 0 hit points"
	case "death saves cleared":
		return "death saves cleared"
	}
	return e.Action
}

// HealthEventLine is the same thing as org markup, for the session notes.
func HealthEventLine(e HealthEvent) string {
	msg := HealthEventMsg(e)
	if e.Notes != "" {
		msg += " - " + e.Notes
	}
	switch e.Action {
	case "hurt":
		return "*Damage.* " + strings.ToUpper(msg[:1]) + msg[1:] + "."
	case "healed":
		return "*Healing.* " + strings.ToUpper(msg[:1]) + msg[1:] + "."
	case "death save", "revived", "stabilized", "death saves cleared":
		return "*Death Save.* " + strings.ToUpper(msg[:1]) + msg[1:] + "."
	}
	return "*Hit Points.* " + strings.ToUpper(msg[:1]) + msg[1:] + "."
}
