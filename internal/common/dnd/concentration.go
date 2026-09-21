//lint:file-ignore ST1006 allow the use of self
package dnd

/* SDOC: DnD
* Concentration

  A spell that says =Concentration= on it lasts only as long as the caster
  keeps their attention on it, and a caster may hold only one such spell at a
  time. What breaks it is the part that gets forgotten at the table: taking
  damage calls for a Constitution saving throw at DC 10, or half the damage
  taken, whichever is higher.

  So the sheet remembers what is being concentrated on, in the character's own
  file:

  #+BEGIN_SRC org
  ,   :DND_CONCENTRATION: hex @2
  #+END_SRC

  The name before the =@= is a spell id or a spell name - whichever was
  written, so the property stays editable by hand - and the number after it is
  the slot level it was cast at, which is dropped when it is the spell's own.

  Casting a second concentration spell drops the first rather than refusing:
  that is what the rules do, and a sheet that argued about it would be wrong.
  Going to zero hit points drops it too, and so does a long rest.
EDOC */

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// The actions a concentration call may ask for.
const (
	ConcStart = "start"
	ConcDrop  = "drop"
	ConcSave  = "save"
)

// ConcentrationBaseDC is the floor on a concentration save. Half the damage
// taken beats it once a single hit is worth more than twenty.
const ConcentrationBaseDC = 10

// Concentration is the spell a character is currently holding, stored on the
// character because it is a thing that happened at the table: nothing on the
// sheet can work out that a spell is still up.
type Concentration struct {
	// Spell is what was written - a spell id or its name - and Name is that
	// resolved against the ruleset for display. Only Spell is stored.
	Spell string `yaml:"spell" json:"spell"`
	Name  string `yaml:"-" json:"name"`
	// Level is the slot level it was cast at, 0 for a cantrip or when the
	// level is simply the spell's own.
	Level int `yaml:"level" json:"level"`
	// Since is the time of day it went up, so the sheet can say how long it
	// has been held. It is not written to the file - a duration that survived
	// a night's sleep would be a lie - so it is only ever this session's.
	Since string `yaml:"-" json:"since"`
}

// ConcentrationView is the banner on the sheet: what is being held, and the
// saving throw the character makes to keep it.
type ConcentrationView struct {
	// On is false when nothing is being concentrated on, which is the usual
	// state and the reason this is a view rather than a bare pointer: the
	// banner has to be able to draw its own absence.
	On    bool   `json:"on"`
	Spell string `json:"spell"`
	Name  string `json:"name"`
	Level int    `json:"level"`
	Label string `json:"label"`
	Since string `json:"since"`
	// SaveStr is the Constitution save the character rolls to keep it, as a
	// modifier they can read off the banner.
	SaveStr string `json:"saveStr"`
	SaveMod int    `json:"saveMod"`
	// Duration is what the spell's own text says it lasts, when the ruleset
	// knows the spell, so the banner can say "up to 1 minute".
	Duration string `json:"duration"`
}

// ConcentrationSave is the saving throw a hit has just called for. It rides
// back on the answer to the blow that caused it, so the sheet can ask for the
// roll without having to work out the DC itself.
type ConcentrationSave struct {
	Spell string `json:"spell"`
	Name  string `json:"name"`
	// DC is 10, or half the damage taken when that is more.
	DC     int `json:"dc"`
	Damage int `json:"damage"`
	// Mod is the character's Constitution saving throw modifier.
	Mod     int    `json:"mod"`
	ModStr  string `json:"modStr"`
	Prompt  string `json:"prompt"`
	Ability string `json:"ability"`
}

// ConcentrationRequest is one change to what a character is concentrating on.
type ConcentrationRequest struct {
	Filename string `json:"filename"`
	Id       string `json:"id"`
	// Action is start, drop or save.
	Action string `json:"action"`
	// Spell is what is being concentrated on, for start.
	Spell string `json:"spell"`
	Level int    `json:"level"`
	// For save: the DC that was called for and what the d20 came to. A roll
	// at or above the DC keeps the spell, anything under it drops the spell.
	DC    int `json:"dc"`
	Roll  int `json:"roll"`
	Notes string `json:"notes"`
}

// ConcentrationState is the answer to every concentration call.
type ConcentrationState struct {
	Id            string            `json:"id"`
	Name          string            `json:"name"`
	Filename      string            `json:"filename"`
	Ruleset       string            `json:"ruleset"`
	Concentration ConcentrationView `json:"concentration"`
	// Kept says whether a save kept the spell up, for the one action where
	// that is the question being asked.
	Kept bool   `json:"kept"`
	Msg  string `json:"msg"`
}

// ParseConcentration reads the DND_CONCENTRATION property: a spell id or name,
// optionally followed by "@" and the slot level it was cast at. An empty or
// unreadable value is nothing being concentrated on rather than an error - the
// property is meant to be editable by hand, and a sheet that refused to load
// over a stray character in it would be the worse answer.
func ParseConcentration(val string) *Concentration {
	val = strings.TrimSpace(val)
	if val == "" || strings.EqualFold(val, "none") {
		return nil
	}
	c := &Concentration{Spell: val}
	if at := strings.LastIndex(val, "@"); at >= 0 {
		c.Spell = strings.TrimSpace(val[:at])
		if n, err := strconv.Atoi(strings.TrimSpace(val[at+1:])); err == nil && n > 0 && n <= 9 {
			c.Level = n
		}
	}
	if c.Spell == "" {
		return nil
	}
	return c
}

// ConcentrationProp is the value written back into the property, which is
// ParseConcentration's input again.
func ConcentrationProp(c *Concentration) string {
	if c == nil || strings.TrimSpace(c.Spell) == "" {
		return ""
	}
	if c.Level > 0 {
		return fmt.Sprintf("%s @%d", c.Spell, c.Level)
	}
	return c.Spell
}

// ComputeConcentration is the banner worked out from a computed sheet: what is
// being held, said the way the ruleset names it, and the save that keeps it.
func ComputeConcentration(s *Sheet, rs *Ruleset) ConcentrationView {
	v := ConcentrationView{}
	if s == nil || s.Character == nil || s.Character.Concentration == nil {
		return v
	}
	held := s.Character.Concentration
	v.On = true
	v.Spell = held.Spell
	v.Level = held.Level
	v.Since = held.Since
	v.Name = held.Name
	v.SaveMod = conSaveMod(s)
	v.SaveStr = Signed(v.SaveMod)
	if rs != nil {
		if sp := rs.Spell(held.Spell); sp != nil {
			v.Name = sp.Name
			v.Duration = sp.Duration
		}
	}
	if v.Name == "" {
		v.Name = Titleize(held.Spell)
	}
	v.Label = v.Name
	if held.Level > 0 {
		v.Label = fmt.Sprintf("%s (%s level)", v.Name, Ordinal(held.Level))
	}
	return v
}

// conSaveMod is what the character adds to a Constitution saving throw, which
// is the only roll concentration ever asks for.
func conSaveMod(s *Sheet) int {
	if s == nil {
		return 0
	}
	return s.AbilityMap["con"].Save
}

// ConcentrationDC is the saving throw a hit calls for: DC 10, or half the
// damage taken when that is higher. The half rounds down, which is what every
// "half the damage" in the rules does.
func ConcentrationDC(damage int) int {
	if damage < 0 {
		damage = -damage
	}
	if half := damage / 2; half > ConcentrationBaseDC {
		return half
	}
	return ConcentrationBaseDC
}

// StartConcentration puts a spell up. A caster holds only one at a time, so
// this replaces whatever was there and says what it displaced - the rules do
// not offer a choice about it, and a sheet that refused would be wrong.
func StartConcentration(c *Character, rs *Ruleset, spell string, level int) (string, error) {
	if c == nil {
		return "", fmt.Errorf("no character")
	}
	spell = strings.TrimSpace(spell)
	if spell == "" {
		return "", fmt.Errorf("concentrating on what?")
	}
	name := spell
	if rs != nil {
		if sp := rs.Spell(spell); sp != nil {
			name = sp.Name
			if !sp.Concentration {
				return "", fmt.Errorf("%s does not ask for concentration", sp.Name)
			}
		}
	}
	dropped := ""
	if c.Concentration != nil && !strings.EqualFold(c.Concentration.Spell, spell) {
		dropped = concentrationName(c.Concentration, rs)
	}
	if level < 0 || level > 9 {
		level = 0
	}
	c.Concentration = &Concentration{
		Spell: spell, Name: name, Level: level, Since: time.Now().Format("15:04"),
	}
	if dropped != "" {
		return fmt.Sprintf("concentrating on %s, letting %s go", name, dropped), nil
	}
	return "concentrating on " + name, nil
}

// DropConcentration lets go of whatever is being held and says what it was.
// Letting go of nothing is not an error: a sheet that has just been reloaded
// and one that never had a spell up look the same, and both should be able to
// press the button.
func DropConcentration(c *Character, rs *Ruleset) string {
	if c == nil || c.Concentration == nil {
		return "nothing to let go of"
	}
	name := concentrationName(c.Concentration, rs)
	c.Concentration = nil
	return "let " + name + " go"
}

// concentrationName is the display name of a held spell.
func concentrationName(held *Concentration, rs *Ruleset) string {
	if held == nil {
		return ""
	}
	if rs != nil {
		if sp := rs.Spell(held.Spell); sp != nil {
			return sp.Name
		}
	}
	if held.Name != "" {
		return held.Name
	}
	return Titleize(held.Spell)
}

// ConcentrationSaveFor is the saving throw a blow calls for, or nil when the
// character is holding nothing. The sheet asks for the roll; whether it was
// made is answered by ApplyConcentrationSave.
func ConcentrationSaveFor(c *Character, rs *Ruleset, s *Sheet, damage int) *ConcentrationSave {
	if c == nil || c.Concentration == nil || damage <= 0 {
		return nil
	}
	name := concentrationName(c.Concentration, rs)
	mod := conSaveMod(s)
	dc := ConcentrationDC(damage)
	return &ConcentrationSave{
		Spell: c.Concentration.Spell, Name: name, DC: dc, Damage: damage,
		Mod: mod, ModStr: Signed(mod), Ability: "con",
		Prompt: fmt.Sprintf("Constitution save DC %d to keep %s", dc, name),
	}
}

// ApplyConcentrationSave settles a save that has been rolled: at or above the
// DC the spell stays up, under it the spell goes. It reports whether the spell
// was kept along with the line the sheet shows.
func ApplyConcentrationSave(c *Character, rs *Ruleset, dc, roll int) (bool, string, error) {
	if c == nil {
		return false, "", fmt.Errorf("no character")
	}
	if c.Concentration == nil {
		return false, "", fmt.Errorf("you are not concentrating on anything")
	}
	if dc <= 0 {
		dc = ConcentrationBaseDC
	}
	name := concentrationName(c.Concentration, rs)
	if roll >= dc {
		return true, fmt.Sprintf("%d against DC %d, %s holds", roll, dc, name), nil
	}
	c.Concentration = nil
	return false, fmt.Sprintf("%d against DC %d, %s is lost", roll, dc, name), nil
}

// ConcentrationLine is one change as org markup, for the session notes.
func ConcentrationLine(msg string) string {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return ""
	}
	return "*Concentration.* " + strings.ToUpper(msg[:1]) + msg[1:] + "."
}
