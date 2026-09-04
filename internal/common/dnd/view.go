package dnd

/* SDOC: DnD
* Sheet Templates

  Both the html and the latex character sheets are ordinary pongo2 templates
  that receive one variable, =sheet=, which is the computed sheet converted
  into plain maps and lists. That means a template can reach anything the
  engine knows:

  #+BEGIN_SRC django
  {{ sheet.name }} - {{ sheet.classLine }}
  {% for a in sheet.abilities %}{{ a.short }} {{ a.score }} ({{ a.mod }}){% endfor %}
  {% for s in sheet.skills %}{% if s.proficient %}* {% endif %}{{ s.name }} {{ s.mod }}
  {% endfor %}
  #+END_SRC

  The latex renderer passes every string through a tex escaper first, so
  ampersands, underscores and the rest of the special characters in a player's
  backstory cannot break the build.
EDOC */

import (
	"encoding/json"
	"strconv"
	"strings"
)

// SheetMap converts a computed sheet into the nested map/slice form the
// templates consume. When escape is non nil it is applied to every string,
// which is how the latex template gets tex safe values.
func SheetMap(s *Sheet, escape func(string) string) map[string]interface{} {
	data, err := json.Marshal(s)
	if err != nil {
		return map[string]interface{}{"name": s.Name}
	}
	var out map[string]interface{}
	if err := json.Unmarshal(data, &out); err != nil {
		return map[string]interface{}{"name": s.Name}
	}
	if escape == nil {
		escape = func(s string) string { return s }
	}
	return walkValues(out, escape).(map[string]interface{})
}

// walkValues escapes every string and turns whole float64 values back into
// ints. Without that second step pongo2 renders an armour class of 16 as
// "16.000000", because json decoding makes every number a float.
func walkValues(v interface{}, fn func(string) string) interface{} {
	switch t := v.(type) {
	case string:
		return fn(t)
	case float64:
		if t == float64(int64(t)) {
			return int(t)
		}
		// A fractional weight would otherwise render as "52.500000".
		return strconv.FormatFloat(t, 'f', -1, 64)
	case []interface{}:
		for i, item := range t {
			t[i] = walkValues(item, fn)
		}
		return t
	case map[string]interface{}:
		for k, item := range t {
			t[k] = walkValues(item, fn)
		}
		return t
	}
	return v
}

var latexReplacer = strings.NewReplacer(
	"\\", "\\textbackslash{}",
	"&", "\\&",
	"%", "\\%",
	"$", "\\$",
	"#", "\\#",
	"_", "\\_",
	"{", "\\{",
	"}", "\\}",
	"~", "\\textasciitilde{}",
	"^", "\\textasciicircum{}",
	"’", "'",
	"‘", "`",
	"“", "``",
	"”", "''",
	"—", "---",
	"–", "--",
	"…", "\\ldots{}",
	"°", "\\textdegree{}",
	"×", "x",
	"−", "-",
	" ", " ",
)

// LatexEscape makes a string safe to drop into a tex document.
//
// As well as the tex special characters it drops runes that a pdflatex T1
// font cannot typeset. Accented latin letters are fine, but a stray greek or
// CJK character in a player's backstory would otherwise fail the whole build.
func LatexEscape(s string) string {
	s = latexReplacer.Replace(s)
	var b strings.Builder
	for _, r := range s {
		switch {
		case r < 0x20 && r != '\n' && r != '\t':
			// control characters, drop
		case r <= 0x017F:
			// ascii, latin-1 supplement and latin extended-a are all in T1
			b.WriteRune(r)
		default:
			b.WriteRune('?')
		}
	}
	return b.String()
}

// TerminalSheet renders a compact character sheet for the command line.
func TerminalSheet(s *Sheet) string {
	var b strings.Builder
	line := func(format string, args ...interface{}) {
		b.WriteString(sprintf(format, args...))
		b.WriteString("\n")
	}
	rule := strings.Repeat("=", 78)
	line("%s", rule)
	line(" %s", strings.ToUpper(s.Name))
	line(" %s | %s | %s | %s", orDefault(s.RaceName, "?"), orDefault(s.ClassLine, "?"),
		orDefault(s.Background, "no background"), orDefault(s.Alignment, "unaligned"))
	line("%s", rule)
	line(" AC %-3d  HP %-4d  Hit Dice %-8s  Init %-3s  Speed %d ft  Prof %s",
		s.AC, s.HPMax, s.HitDice, s.InitiativeStr, s.Speed, s.ProficiencyStr)
	if s.IsCaster {
		line(" Spell DC %-3d  Spell Attack %-3s  %s",
			s.SpellSaveDC, s.SpellAttackStr, s.CastingAbilityName)
	}
	line("")
	head := " "
	scores := " "
	mods := " "
	saves := " "
	for _, a := range s.Abilities {
		head += sprintf("%-8s", a.Short)
		scores += sprintf("%-8d", a.Score)
		mods += sprintf("%-8s", a.Mod)
		mark := " "
		if a.SaveProf {
			mark = "*"
		}
		saves += sprintf("%-8s", a.SaveStr+mark)
	}
	line("%s", head)
	line("%s   scores", scores)
	line("%s   modifiers", mods)
	line("%s   saving throws (* = proficient)", saves)
	line("")
	line(" Skills:")
	for _, sk := range s.Skills {
		if !sk.Proficent && !sk.Expertise {
			continue
		}
		mark := "*"
		if sk.Expertise {
			mark = "E"
		}
		line("   %s %-18s %s", mark, sk.Name, sk.Mod)
	}
	line(" Passive perception %d, insight %d, investigation %d",
		s.PassivePerception, s.PassiveInsight, s.PassiveInvestigation)
	if len(s.Attacks) > 0 {
		line("")
		line(" Attacks:")
		for _, a := range s.Attacks {
			line("   %-20s %-4s %-10s %-12s %s", a.Name, a.Bonus, a.Damage, a.Type, a.Range)
		}
	}
	if len(s.Slots) > 0 {
		line("")
		slots := []string{}
		for _, sl := range s.Slots {
			slots = append(slots, sprintf("%s:%d", Ordinal(sl.Level), sl.Total))
		}
		line(" Spell slots: %s", strings.Join(slots, "  "))
	}
	if len(s.Equipment) > 0 {
		line("")
		line(" Equipment: %s", equipmentLine(s.Equipment))
	}
	line(" Carrying %.1f of %d lb.  Coins: %dcp %dsp %dep %dgp %dpp",
		s.Weight, s.CarryCapacity, s.Money.CP, s.Money.SP, s.Money.EP, s.Money.GP, s.Money.PP)
	if len(s.Warnings) > 0 {
		line("")
		for _, w := range s.Warnings {
			line(" ! %s", w)
		}
	}
	line("%s", rule)
	return b.String()
}

func equipmentLine(gear []Gear) string {
	parts := []string{}
	for _, g := range gear {
		name := g.Name
		if g.Qty > 1 {
			name = sprintf("%s x%d", name, g.Qty)
		}
		if g.Equipped {
			name += " (worn)"
		}
		parts = append(parts, name)
	}
	return strings.Join(parts, ", ")
}
