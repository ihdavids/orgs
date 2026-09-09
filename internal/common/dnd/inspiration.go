//lint:file-ignore ST1006 allow the use of self
package dnd

/* SDOC: DnD
* Inspiration

  Inspiration is the one thing on a character sheet that is neither earned by
  the rules nor spent by them: the DM hands it out for playing your character
  well, and you spend it on advantage when you like. It is a single bit, held
  in the =DND_INSPIRATION= property of the org character sheet.

  The html sheet's inspiration marker is a button. Pressing it posts to
  =/dnd/inspiration=, the server rewrites the character's own org file, and the
  answer is what the sheet redraws from - so the page and the file never
  disagree about whether you are holding inspiration, the same way hit points,
  coin and inventory work.
EDOC */

import "fmt"

// The actions an inspiration call may ask for. Toggle is what the button on
// the sheet sends: whoever is pressing it can see the current state, so there
// is nothing to be gained by making them say which way it should go.
const (
	InspirationToggle = "toggle"
	InspirationGain   = "gain"
	InspirationSpend  = "spend"
)

// InspirationRequest is one change to whether a character holds inspiration.
type InspirationRequest struct {
	Filename string `json:"filename"`
	Id       string `json:"id"`
	// Action is "toggle", "gain" or "spend". An empty action toggles.
	Action string `json:"action"`
}

// InspirationState is the answer to every inspiration call.
type InspirationState struct {
	Id          string `json:"id"`
	Name        string `json:"name"`
	Filename    string `json:"filename"`
	Ruleset     string `json:"ruleset"`
	Inspiration bool   `json:"inspiration"`
	// Msg says what the call did in one line, for the sheet to show and for
	// the night's log to record.
	Msg string `json:"msg"`
}

// ApplyInspiration moves a character's inspiration on and says what it did.
// The character is changed in place; writing the sheet back out is the
// caller's business.
//
// Gaining inspiration you already hold, or spending what you have not got, is
// not an error - it is two people at the table pressing the same button - so
// it simply leaves the sheet where it was and says so.
func ApplyInspiration(c *Character, action string) (string, error) {
	if c == nil {
		return "", fmt.Errorf("no character")
	}
	switch action {
	case "", InspirationToggle:
		c.Inspiration = !c.Inspiration
	case InspirationGain:
		if c.Inspiration {
			return "already holding inspiration", nil
		}
		c.Inspiration = true
	case InspirationSpend:
		if !c.Inspiration {
			return "no inspiration to spend", nil
		}
		c.Inspiration = false
	default:
		return "", fmt.Errorf("unknown inspiration action %q", action)
	}
	if c.Inspiration {
		return "inspired", nil
	}
	return "inspiration spent", nil
}
