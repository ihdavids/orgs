//lint:file-ignore ST1006 allow the use of self
package dnd

/* SDOC: DnD
* Experience

  A character's experience is the =DND_XP= property of the org character
  sheet. The html sheet draws it as the ring round the portrait (and the
  Experience fact on a phone); clicking either opens a small box that adds
  experience, takes it away, or sets it to a number, posting to =/dnd/xp=.
  The server rewrites the character's own org file and the sheet redraws the
  ring and the level-up tray's bar from its answer.

  Experience never changes the level by itself: crossing a threshold says the
  character is ready, and the level-up tray does the levelling. A table that
  levels by milestone can ignore experience altogether.
EDOC */

import "fmt"

// The actions an experience call may ask for.
const (
	XPAdd    = "add"
	XPRemove = "remove"
	XPSet    = "set"
)

// XPRequest is one change to a character's experience.
type XPRequest struct {
	Filename string `json:"filename"`
	Id       string `json:"id"`
	// Action is "add", "remove" or "set". An empty action adds.
	Action string `json:"action"`
	Amount int    `json:"amount"`
}

// XPState is the answer to every experience call: the numbers the sheet
// draws the ring and the level-up bar from.
type XPState struct {
	Id          string `json:"id"`
	Name        string `json:"name"`
	Filename    string `json:"filename"`
	Ruleset     string `json:"ruleset"`
	XP          int    `json:"xp"`
	Level       int    `json:"level"`
	NextLevelXP int    `json:"nextLevelXp"`
	XPPercent   int    `json:"xpPercent"`
	// Ready is true when the experience has bought a level the character has
	// not taken yet.
	Ready bool `json:"ready"`
	// Msg says what the call did in one line.
	Msg string `json:"msg"`
}

// NewXPState reads the experience numbers off a character.
func NewXPState(c *Character) XPState {
	level := c.TotalLevel()
	next := 0
	if level < len(XPThresholds)-1 {
		next = XPForLevel(level + 1)
	}
	return XPState{
		Id: CharacterId(c), Name: c.Name, XP: c.XP, Level: level,
		NextLevelXP: next, XPPercent: xpProgress(c.XP, level),
		Ready: next > 0 && c.XP >= next,
	}
}

// ApplyXP changes a character's experience and says what it did. The
// character is changed in place; writing the sheet back out is the caller's
// business. Taking away more than the character has leaves them at nought
// rather than refusing: the table meant "all of it".
func ApplyXP(c *Character, action string, amount int) (string, error) {
	if c == nil {
		return "", fmt.Errorf("no character")
	}
	if amount < 0 {
		return "", fmt.Errorf("experience amounts are not negative; use remove")
	}
	was := c.XP
	switch action {
	case "", XPAdd:
		c.XP += amount
	case XPRemove:
		c.XP -= amount
		if c.XP < 0 {
			c.XP = 0
		}
	case XPSet:
		c.XP = amount
	default:
		return "", fmt.Errorf("unknown experience action %q", action)
	}
	if c.XP == was {
		return fmt.Sprintf("experience stays at %d", c.XP), nil
	}
	msg := fmt.Sprintf("%+d XP, now %d", c.XP-was, c.XP)
	if s := NewXPState(c); s.Ready {
		msg += fmt.Sprintf(" - ready for level %d", s.Level+1)
	}
	return msg, nil
}
