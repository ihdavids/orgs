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
	// Where the character stood before and after.
	HPBefore   int `json:"hpBefore"`
	HPAfter    int `json:"hpAfter"`
	TempBefore int `json:"tempBefore"`
	TempAfter  int `json:"tempAfter"`
	HPMax      int `json:"hpMax"`
	// Down is true when the hit put the character on nothing at all.
	Down  bool   `json:"down"`
	Notes string `json:"notes"`
}

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
	Amount int    `json:"amount"`
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
	return v
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

	default:
		return e, fmt.Errorf(
			"unknown action %q, expected hurt, heal, temp, cleartemp or set", req.Action)
	}

	c.HPCurrent = cur
	c.HPTemp = temp
	e.HPAfter, e.TempAfter = cur, temp
	e.Down = cur <= 0
	return logHealth(c, e), nil
}

// logHealth stamps an event with the time and appends it to the character's
// health history, which is what gets written into the Health History section.
func logHealth(c *Character, e HealthEvent) HealthEvent {
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
		msg := fmt.Sprintf("took %d damage", e.Amount)
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
	}
	return "*Hit Points.* " + strings.ToUpper(msg[:1]) + msg[1:] + "."
}
