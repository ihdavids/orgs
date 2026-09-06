package dnd

/* SDOC: DnD
* The Org Character Sheet

  A character is stored as a perfectly ordinary org file. Everything the
  engine needs lives in the property drawer of the top level heading, plus two
  hand editable tables (equipment and spells). Every other section is derived
  and regenerated whenever the sheet is written, so you can level up by
  editing one property.

  #+BEGIN_SRC org
  ,#+TITLE: Lyra Silverleaf
  ,#+DND_ID: lyra-silverleaf-4c1f2a
  ,#+DND_RULESET: srd
  ,#+LATEX_CLASS: dndcharacter
  ,#+LATEX_TEMPLATE: dnd_character.tpl

  ,* Lyra Silverleaf                                          :dnd:character:
  ,   :PROPERTIES:
  ,   :DND_ID:         lyra-silverleaf-4c1f2a
  ,   :DND_RACE:       elf
  ,   :DND_SUBRACE:    high-elf
  ,   :DND_CLASSES:    wizard:evocation:3
  ,   :DND_BACKGROUND: sage
  ,   :DND_STR:        8
  ,   :END:
  #+END_SRC

  The =DND_CLASSES= property is a comma separated list of
  =class:subclass:level= entries, which is how multiclassing is stored.
EDOC */

import (
	"crypto/sha1"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// InventoryHistoryHeading is the section of the sheet the inventory log is
// written to and read back from.
const InventoryHistoryHeading = "Inventory History"

// CoinHistoryHeading is the same for the coin log: what has been spent,
// earned or changed up.
const CoinHistoryHeading = "Coin History"

// Property names used in the character property drawer.
const (
	PropId         = "DND_ID"
	PropName       = "DND_NAME"
	PropPlayer     = "DND_PLAYER"
	PropRuleset    = "DND_RULESET"
	PropRace       = "DND_RACE"
	PropSubrace    = "DND_SUBRACE"
	PropClasses    = "DND_CLASSES"
	PropLevel      = "DND_LEVEL"
	PropBackground = "DND_BACKGROUND"
	PropAlignment  = "DND_ALIGNMENT"
	PropXP         = "DND_XP"
	PropSkills     = "DND_SKILLS"
	PropExpertise  = "DND_EXPERTISE"
	PropLanguages  = "DND_LANGUAGES"
	PropTools      = "DND_TOOLS"
	PropToolExpert = "DND_TOOL_EXPERTISE"
	PropFeats      = "DND_FEATS"
	PropChoices    = "DND_CHOICES"
	PropHPMax      = "DND_HP_MAX"
	PropHPCur      = "DND_HP_CURRENT"
	PropHPTemp     = "DND_HP_TEMP"
	PropHitDice    = "DND_HIT_DICE_USED"
	PropDeath      = "DND_DEATH_SAVES"
	PropInspire    = "DND_INSPIRATION"
	PropSlotsUsed  = "DND_SLOTS_USED"
	PropUsesSpent  = "DND_USES_SPENT"
	PropImage      = "DND_IMAGE"
	PropImageFocus = "DND_IMAGE_FOCUS"
	PropImageZoom  = "DND_IMAGE_ZOOM"
)

// RenderOrg writes a complete org mode character sheet.
func RenderOrg(c *Character, rs *Ruleset) string {
	s := Compute(c, rs)
	var b strings.Builder
	w := func(format string, args ...interface{}) {
		fmt.Fprintf(&b, format, args...)
	}

	title := c.Name
	if title == "" {
		title = "Unnamed Adventurer"
	}
	w("#+TITLE: %s\n", title)
	if c.Player != "" {
		w("#+AUTHOR: %s\n", c.Player)
	}
	w("#+DND_ID: %s\n", CharacterId(c))
	w("#+DND_RULESET: %s\n", orDefault(c.Ruleset, DefaultRuleset))
	w("#+LATEX_CLASS: dndcharacter\n")
	w("#+LATEX_TEMPLATE: dnd_character.tpl\n")
	w("#+HTML_THEME: dndcharacter\n")
	w("#+STARTUP: showeverything\n")
	w("#+FILETAGS: :dnd:character:\n")
	w("\n")

	// ---- identity ---------------------------------------------------------
	w("* %s\n", title)
	// The drawer has to be indented: go-org only attaches a property drawer to
	// its headline when it is, and an unattached drawer is invisible to every
	// org query, which is the whole point of storing DND_ID here.
	w("   :PROPERTIES:\n")
	props := [][2]string{
		{PropId, CharacterId(c)},
		{PropName, c.Name},
		{PropPlayer, c.Player},
		{PropRuleset, orDefault(c.Ruleset, DefaultRuleset)},
		{PropRace, c.Race},
		{PropSubrace, c.Subrace},
		{PropClasses, encodeClasses(c.Classes)},
		{PropLevel, itoa(c.TotalLevel())},
		{PropBackground, c.Background},
		{PropAlignment, c.Alignment},
		{PropXP, itoa(c.XP)},
	}
	for _, a := range AbilityOrder {
		props = append(props, [2]string{"DND_" + strings.ToUpper(a), itoa(c.Abilities[a])})
	}
	props = append(props,
		[2]string{PropSkills, joinList(c.Skills)},
		[2]string{PropExpertise, joinList(c.Expertise)},
		[2]string{PropLanguages, joinList(c.Languages)},
		[2]string{PropTools, joinList(c.Tools)},
		[2]string{PropToolExpert, joinList(c.ToolExpertise)},
		[2]string{PropFeats, joinList(c.Feats)},
		[2]string{PropChoices, encodeChoices(c.Choices)},
		[2]string{PropHPMax, itoa(s.HPMax)},
		[2]string{PropHPCur, itoa(s.HPCurrent)},
		[2]string{PropHPTemp, itoa(c.HPTemp)},
		[2]string{PropHitDice, itoa(c.HitDiceUsed)},
		[2]string{PropDeath, c.DeathSaves},
		[2]string{PropInspire, boolStr(c.Inspiration)},
		[2]string{PropSlotsUsed, encodeInts(c.SlotsUsed)},
		[2]string{PropUsesSpent, encodeUses(c.UsesSpent)},
		[2]string{"DND_CP", itoa(c.Money.CP)},
		[2]string{"DND_SP", itoa(c.Money.SP)},
		[2]string{"DND_EP", itoa(c.Money.EP)},
		[2]string{"DND_GP", itoa(c.Money.GP)},
		[2]string{"DND_PP", itoa(c.Money.PP)},
		[2]string{"DND_AGE", c.Age},
		[2]string{"DND_HEIGHT", c.Height},
		[2]string{"DND_WEIGHT", c.Weight},
		[2]string{"DND_EYES", c.Eyes},
		[2]string{"DND_SKIN", c.Skin},
		[2]string{"DND_HAIR", c.Hair},
		[2]string{PropImage, c.Image},
		[2]string{PropImageFocus, c.ImageFocus},
		[2]string{PropImageZoom, FormatZoom(c.ImageZoom)},
	)
	width := 0
	for _, p := range props {
		if len(p[0]) > width {
			width = len(p[0])
		}
	}
	for _, p := range props {
		w("   :%s:%s %s\n", p[0], strings.Repeat(" ", width-len(p[0])), p[1])
	}
	w("   :END:\n\n")

	w("%s, %s. %s. %s.\n\n", s.RaceName, s.ClassLine, orDefault(s.Background, "No background"),
		orDefault(s.Alignment, "Unaligned"))

	// ---- derived: abilities ----------------------------------------------
	w("** Ability Scores\n")
	rows := [][]string{{"Ability", "Score", "Mod", "Save"}}
	for _, a := range s.Abilities {
		save := a.SaveStr
		if a.SaveProf {
			save += " *"
		}
		rows = append(rows, []string{a.Name, itoa(a.Score), a.Mod, save})
	}
	w("%s\n", orgTable(rows, 1))
	w("Proficiency bonus %s. Passive Perception %d. Passive Insight %d. Passive Investigation %d.\n\n",
		s.ProficiencyStr, s.PassivePerception, s.PassiveInsight, s.PassiveInvestigation)

	// ---- derived: skills --------------------------------------------------
	w("** Skills\n")
	rows = [][]string{{"Prof", "Skill", "Ability", "Mod"}}
	for _, sk := range s.Skills {
		mark := " "
		if sk.Expertise {
			mark = "E"
		} else if sk.Proficent {
			mark = "X"
		}
		rows = append(rows, []string{mark, sk.Name, sk.Short, sk.Mod})
	}
	w("%s\n", orgTable(rows, 1))
	w("X = proficient, E = expertise.\n\n")

	// ---- derived: combat --------------------------------------------------
	w("** Combat\n")
	rows = [][]string{
		{"Armor Class", itoa(s.AC), s.ACSource},
		{"Initiative", s.InitiativeStr, ""},
		{"Speed", fmt.Sprintf("%d ft.", s.Speed), s.Size},
		{"Hit Points", itoa(s.HPMax), fmt.Sprintf("current %d, temp %d", s.HPCurrent, s.HPTemp)},
		{"Hit Dice", s.HitDice, fmt.Sprintf("%d used", s.HitDiceUsed)},
		{"Proficiency", s.ProficiencyStr, ""},
	}
	if s.Darkvision > 0 {
		rows = append(rows, []string{"Darkvision", fmt.Sprintf("%d ft.", s.Darkvision), ""})
	}
	w("%s\n\n", orgTable(rows, 0))

	w("*** Attacks\n")
	rows = [][]string{{"Attack", "Bonus", "Damage", "Type", "Range", "Notes"}}
	for _, a := range s.Attacks {
		rows = append(rows, []string{a.Name, a.Bonus, a.Damage, a.Type, a.Range, a.Notes})
	}
	w("%s\n\n", orgTable(rows, 1))

	// ---- equipment (parsed back) -----------------------------------------
	w("** Equipment\n")
	w("Edit this table freely, it is read back in when the sheet is loaded.\n")
	w("Container is where a line is kept: blank for on your person, otherwise a\n")
	w("container you own, such as backpack or pouch.\n")
	rows = [][]string{{"Item", "Qty", "Equipped", "Attuned", "Weight", "Container", "Notes"}}
	for _, g := range s.Equipment {
		rows = append(rows, []string{g.Name, itoa(g.Qty), yesNo(g.Equipped),
			yesNo(g.Attuned), trimFloat(g.Weight), g.Container, g.Notes})
	}
	w("%s\n", orgTable(rows, 1))
	w("Carrying %.1f lb of %d lb capacity (push/drag/lift %d lb). %s.\n",
		s.Weight, s.CarryCapacity, s.PushDragLift, s.Inventory.Label)
	if s.Inventory.Stored > 0 {
		w("Another %.1f lb is stowed in extradimensional space and is not carried.\n",
			s.Inventory.Stored)
	}
	w("Coins: %d cp, %d sp, %d ep, %d gp, %d pp - %s in all, weighing %s lb.\n\n",
		s.Money.CP, s.Money.SP, s.Money.EP, s.Money.GP, s.Money.PP,
		ValueString(s.Money.Copper()), trimFloat(s.Money.Weight()))

	// ---- inventory history (parsed back) ----------------------------------
	//
	// Written by the character sheet every time something is picked up, used,
	// dropped or packed away. It is the file's own record: nothing is derived
	// from it, so an entry corrected by hand stays corrected.
	if len(c.InventoryLog) > 0 {
		w("** %s\n", InventoryHistoryHeading)
		w("What has come and gone, newest last.\n")
		rows = [][]string{{"Date", "Time", "Action", "Item", "Qty", "From", "To", "Notes"}}
		for _, e := range c.InventoryLog {
			rows = append(rows, []string{e.Date, e.Time, e.Action, e.Item,
				itoa(e.Qty), e.From, e.To, e.Notes})
		}
		w("%s\n\n", orgTable(rows, 1))
	}

	// ---- coin history (parsed back) ---------------------------------------
	//
	// The purse's own record, written every time coin is spent, earned,
	// changed up or set by hand. Like the inventory history it is read
	// straight back rather than derived from anything, so a line corrected by
	// hand stays corrected.
	if len(c.MoneyLog) > 0 {
		w("** %s\n", CoinHistoryHeading)
		w("What has been spent and earned, newest last.\n")
		rows = [][]string{{"Date", "Time", "Action", "Amount", "Change", "Balance", "Notes"}}
		for _, e := range c.MoneyLog {
			rows = append(rows, []string{e.Date, e.Time, e.Action, e.Amount.String(),
				moneyCell(e.Change), e.Balance.String(), e.Notes})
		}
		w("%s\n\n", orgTable(rows, 1))
	}

	// ---- magic items (derived from the equipment table above) -------------
	if len(s.MagicItems) > 0 {
		w("** Magic Items\n")
		w("Derived from the equipment table, edit the Attuned column there.\n")
		rows = [][]string{{"Item", "Rarity", "Attunement", "Effect"}}
		for _, m := range s.MagicItems {
			att := "-"
			switch m.Attunement {
			case "attuned":
				att = "attuned"
			case "required":
				att = "needs attunement"
			}
			if m.Note != "" {
				att += " " + m.Note
			}
			rows = append(rows, []string{m.Name, m.Rarity, att, m.Effect})
		}
		w("%s\n", orgTable(rows, 1))
		w("Attunement: %d of %d slots used.\n\n", s.AttunementUsed, s.AttunementSlots)
	}

	// ---- proficiencies ----------------------------------------------------
	w("** Proficiencies & Languages\n")
	w("- Armor :: %s\n", orDefault(joinList(s.ArmorProficiencies), "none"))
	w("- Weapons :: %s\n", orDefault(joinList(s.WeaponProficiencies), "none"))
	w("- Tools :: %s\n", orDefault(joinList(s.ToolProficiencies), "none"))
	w("- Languages :: %s\n\n", orDefault(joinList(s.Languages), "none"))

	// ---- features ---------------------------------------------------------
	w("** Features & Traits\n")
	if len(s.Traits) == 0 && len(s.Features) == 0 {
		w("None yet.\n")
	}
	for _, t := range append(append([]Trait{}, s.Traits...), s.Features...) {
		src := ""
		if t.Source != "" {
			src = fmt.Sprintf(" (%s)", t.Source)
		}
		w("*** %s%s\n", t.Name, src)
		// A feature with a use limit says so, and says how many are gone. The
		// count itself lives in the property drawer - this line is derived
		// and is rewritten every time the sheet is.
		if t.UsesMax > 0 {
			w("Uses: %d of %d spent (%s).\n", t.UsesSpent, t.UsesMax,
				orDefault(t.UsesNote, restWords(t.Recharge)))
		}
		if t.Text != "" {
			w("%s\n", wrapText(t.Text, 78))
		}
	}
	w("\n")

	// ---- spells (parsed back) --------------------------------------------
	if s.IsCaster {
		w("** Spellcasting\n")
		w("Spellcasting ability %s. Spell save DC %d. Spell attack bonus %s.\n",
			orDefault(s.CastingAbilityName, "-"), s.SpellSaveDC, s.SpellAttackStr)
		if s.CantripsKnown > 0 {
			w("Cantrips known %d. ", s.CantripsKnown)
		}
		if s.SpellsKnown > 0 {
			w("Spells known %d. ", s.SpellsKnown)
		}
		if s.PreparedMax > 0 {
			w("Spells prepared %d of %d. ", s.SpellsPrepared, s.PreparedMax)
		}
		if s.SpellNotes != "" {
			w("%s.", s.SpellNotes)
		}
		w("\n\n")
		if len(s.Slots) > 0 {
			rows = [][]string{{"Level", "Slots", "Used"}}
			for _, sl := range s.Slots {
				rows = append(rows, []string{Ordinal(sl.Level), itoa(sl.Total), itoa(sl.Used)})
			}
			w("%s\n\n", orgTable(rows, 1))
		}
		for _, lvl := range s.SpellLevels {
			if len(lvl.Spells) == 0 {
				continue
			}
			w("*** %s\n", lvl.Name)
			rows = [][]string{{"Prep", "Spell", "Time", "Range", "Comp", "Duration", "Notes"}}
			for _, sp := range lvl.Spells {
				notes := []string{}
				if sp.Concentration {
					notes = append(notes, "concentration")
				}
				if sp.Ritual {
					notes = append(notes, "ritual")
				}
				if sp.Source != "" {
					notes = append(notes, sp.Source)
				}
				prep := " "
				if sp.Prepared {
					prep = "X"
				}
				if lvl.Level == 0 {
					prep = "-"
				}
				rows = append(rows, []string{prep, sp.Name, sp.CastingTime, sp.Range,
					sp.Components, sp.Duration, strings.Join(notes, ", ")})
			}
			w("%s\n\n", orgTable(rows, 1))
		}
	}

	// ---- roleplaying ------------------------------------------------------
	w("** Personality\n")
	writeSection(&b, "*** Personality Traits", c.Personality)
	writeSection(&b, "*** Ideals", c.Ideals)
	writeSection(&b, "*** Bonds", c.Bonds)
	writeSection(&b, "*** Flaws", c.Flaws)

	w("** Appearance\n")
	if c.Age != "" || c.Height != "" || c.Weight != "" {
		w("%s\n", strings.TrimSpace(strings.Join(nonEmpty([]string{
			labelled("Age", c.Age), labelled("Height", c.Height), labelled("Weight", c.Weight),
			labelled("Eyes", c.Eyes), labelled("Skin", c.Skin), labelled("Hair", c.Hair),
		}), ", ")))
	}
	if c.Appearance != "" {
		w("%s\n", c.Appearance)
	}
	w("\n")

	writeSection(&b, "** Backstory", c.Backstory)
	writeSection(&b, "** Allies & Organizations", c.Allies)
	writeSection(&b, "** Treasure", c.Treasure)
	writeSection(&b, "** Notes", c.Notes)

	return b.String()
}

// CharacterId reports the character's id, deriving a stable one from the name
// when the sheet has never been given one. Deriving rather than generating
// matters: a sheet written before ids existed keeps the same id every time it
// is read, so the sessions it already appears in still line up. Once the sheet
// is next written the id is stored in DND_ID and a later rename cannot move it.
func CharacterId(c *Character) string {
	if c == nil {
		return ""
	}
	if strings.TrimSpace(c.Id) != "" {
		return strings.TrimSpace(c.Id)
	}
	return DeriveCharacterId(c.Name)
}

// DeriveCharacterId builds a readable, stable id from a character name.
func DeriveCharacterId(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "unnamed adventurer"
	}
	sum := sha1.Sum([]byte(strings.ToLower(name)))
	slug := idStrip.ReplaceAllString(strings.ToLower(name), "-")
	slug = strings.Trim(idStrip.ReplaceAllString(slug, "-"), "-")
	if slug == "" {
		slug = "character"
	}
	if len(slug) > 32 {
		slug = strings.Trim(slug[:32], "-")
	}
	return fmt.Sprintf("%s-%x", slug, sum[:3])
}

var idStrip = regexp.MustCompile(`[^a-z0-9]+`)

func writeSection(b *strings.Builder, heading, text string) {
	fmt.Fprintf(b, "%s\n", heading)
	if strings.TrimSpace(text) != "" {
		fmt.Fprintf(b, "%s\n", strings.TrimRight(text, "\n"))
	}
	fmt.Fprintf(b, "\n")
}

func labelled(name, val string) string {
	if strings.TrimSpace(val) == "" {
		return ""
	}
	return name + " " + val
}

func nonEmpty(list []string) []string {
	out := []string{}
	for _, v := range list {
		if strings.TrimSpace(v) != "" {
			out = append(out, v)
		}
	}
	return out
}

// ----------------------------------------------------------------------------
// Parsing
// ----------------------------------------------------------------------------

// ParseOrg reads a character back out of an org character sheet. The ruleset
// is optional and is only used to resolve item and spell names onto ids.
func ParseOrg(text string, rs *Ruleset) (*Character, error) {
	c := &Character{Abilities: map[string]int{}, Choices: map[string][]string{}}
	lines := strings.Split(text, "\n")

	inDrawer := false
	foundRoot := false
	section := ""
	subsection := ""
	body := map[string]*strings.Builder{}
	tables := map[string][][]string{}
	sectionKey := func() string {
		if subsection != "" {
			return section + "/" + subsection
		}
		return section
	}
	appendBody := func(line string) {
		k := sectionKey()
		if k == "" {
			return
		}
		if body[k] == nil {
			body[k] = &strings.Builder{}
		}
		body[k].WriteString(line)
		body[k].WriteString("\n")
	}

	for _, raw := range lines {
		line := strings.TrimRight(raw, " \t\r")
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "#+DND_ID:") {
			c.Id = strings.TrimSpace(strings.TrimPrefix(trimmed, "#+DND_ID:"))
			continue
		}
		if strings.HasPrefix(trimmed, "#+DND_RULESET:") {
			c.Ruleset = strings.TrimSpace(strings.TrimPrefix(trimmed, "#+DND_RULESET:"))
			continue
		}
		if strings.HasPrefix(trimmed, "#+TITLE:") && c.Name == "" {
			c.Name = strings.TrimSpace(strings.TrimPrefix(trimmed, "#+TITLE:"))
			continue
		}
		if strings.HasPrefix(trimmed, "#+AUTHOR:") && c.Player == "" {
			c.Player = strings.TrimSpace(strings.TrimPrefix(trimmed, "#+AUTHOR:"))
			continue
		}
		if strings.HasPrefix(trimmed, "#+") {
			continue
		}

		// headings
		if strings.HasPrefix(line, "*") {
			stars := 0
			for stars < len(line) && line[stars] == '*' {
				stars++
			}
			if stars < len(line) && line[stars] == ' ' {
				heading := stripTags(strings.TrimSpace(line[stars:]))
				switch {
				case stars == 1:
					foundRoot = true
					section, subsection = "", ""
				case stars == 2:
					section, subsection = normalizeHeading(heading), ""
				default:
					subsection = normalizeHeading(heading)
				}
				inDrawer = false
				continue
			}
		}

		if trimmed == ":PROPERTIES:" {
			inDrawer = true
			continue
		}
		if trimmed == ":END:" {
			inDrawer = false
			continue
		}
		if inDrawer {
			if !strings.HasPrefix(trimmed, ":") {
				continue
			}
			rest := trimmed[1:]
			idx := strings.Index(rest, ":")
			if idx < 0 {
				continue
			}
			key := strings.ToUpper(strings.TrimSpace(rest[:idx]))
			val := strings.TrimSpace(rest[idx+1:])
			applyProperty(c, key, val)
			continue
		}

		// tables
		if strings.HasPrefix(trimmed, "|") {
			if strings.HasPrefix(trimmed, "|-") || strings.HasPrefix(trimmed, "|+") {
				continue
			}
			cells := parseTableRow(trimmed)
			k := sectionKey()
			tables[k] = append(tables[k], cells)
			continue
		}
		appendBody(line)
	}

	if !foundRoot {
		return nil, fmt.Errorf("no character heading found, is this a dnd character sheet?")
	}

	// equipment table
	//
	// Columns are located by their header rather than by position, so a sheet
	// written before the Attuned column existed still reads correctly, and a
	// hand edited table may reorder or drop columns. Without a header row the
	// original order is assumed.
	col := map[string]int{"item": 0, "qty": 1, "equipped": 2, "weight": 3, "notes": 4}
	for _, row := range tables["equipment"] {
		if len(row) == 0 || !strings.EqualFold(strings.TrimSpace(row[0]), "Item") {
			continue
		}
		found := map[string]int{}
		for i, h := range row {
			found[strings.ToLower(strings.TrimSpace(h))] = i
		}
		col = found
		break
	}
	cell := func(row []string, name string) string {
		i, ok := col[name]
		if !ok || i >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[i])
	}
	for _, row := range tables["equipment"] {
		if len(row) == 0 {
			continue
		}
		name := cell(row, "item")
		if name == "" || strings.EqualFold(name, "Item") {
			continue
		}
		g := Gear{Name: name, Qty: 1}
		if rs != nil {
			if it := rs.Item(name); it != nil {
				g.Id = it.Id
				g.Name = it.Name
				g.Weight = it.Weight
			}
		}
		if v := cell(row, "qty"); v != "" {
			if q := atoi(v); q > 0 {
				g.Qty = q
			}
		}
		g.Equipped = isYes(cell(row, "equipped"))
		g.Attuned = isYes(cell(row, "attuned"))
		if v := cell(row, "weight"); v != "" {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				g.Weight = f
			}
		}
		g.Notes = cell(row, "notes")
		g.Container = ContainerKey(cell(row, "container"))
		c.Equipment = append(c.Equipment, g)
	}

	// inventory history
	//
	// Read straight back the way it was written so that the log lives in the
	// file rather than anywhere else. Columns are located by header here too,
	// so a sheet written before a column existed still reads.
	hcol := map[string]int{}
	hrows := tables[normalizeHeading(InventoryHistoryHeading)]
	for _, row := range hrows {
		if len(row) == 0 || !strings.EqualFold(strings.TrimSpace(row[0]), "Date") {
			continue
		}
		for i, h := range row {
			hcol[strings.ToLower(strings.TrimSpace(h))] = i
		}
		break
	}
	hcell := func(row []string, name string) string {
		i, ok := hcol[name]
		if !ok || i >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[i])
	}
	for _, row := range hrows {
		if len(row) == 0 || strings.EqualFold(strings.TrimSpace(row[0]), "Date") {
			continue
		}
		e := InventoryEvent{
			Date: hcell(row, "date"), Time: hcell(row, "time"),
			Action: hcell(row, "action"), Item: hcell(row, "item"),
			Qty: atoi(hcell(row, "qty")), From: hcell(row, "from"),
			To: hcell(row, "to"), Notes: hcell(row, "notes"),
		}
		if e.Item == "" && e.Action == "" {
			continue
		}
		if e.Qty <= 0 {
			e.Qty = 1
		}
		c.InventoryLog = append(c.InventoryLog, e)
	}

	// coin history, read back the same way the inventory history is
	ccol := map[string]int{}
	crows := tables[normalizeHeading(CoinHistoryHeading)]
	for _, row := range crows {
		if len(row) == 0 || !strings.EqualFold(strings.TrimSpace(row[0]), "Date") {
			continue
		}
		for i, h := range row {
			ccol[strings.ToLower(strings.TrimSpace(h))] = i
		}
		break
	}
	ccell := func(row []string, name string) string {
		i, ok := ccol[name]
		if !ok || i >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[i])
	}
	for _, row := range crows {
		if len(row) == 0 || strings.EqualFold(strings.TrimSpace(row[0]), "Date") {
			continue
		}
		e := MoneyEvent{
			Date: ccell(row, "date"), Time: ccell(row, "time"),
			Action: ccell(row, "action"), Notes: ccell(row, "notes"),
		}
		if e.Action == "" {
			continue
		}
		// An amount that has been mistyped by hand is worth keeping the line
		// for: the action and the note still say what happened.
		e.Amount, _ = ParseMoney(ccell(row, "amount"))
		e.Change, _ = ParseMoney(ccell(row, "change"))
		e.Balance, _ = ParseMoney(ccell(row, "balance"))
		c.MoneyLog = append(c.MoneyLog, e)
	}

	// spell tables live under spellcasting/<level name>
	for key, rows := range tables {
		if !strings.HasPrefix(key, "spellcasting/") {
			continue
		}
		levelName := strings.TrimPrefix(key, "spellcasting/")
		level := spellLevelFromName(levelName)
		if level < 0 {
			continue
		}
		for _, row := range rows {
			if len(row) < 2 {
				continue
			}
			prep := strings.TrimSpace(row[0])
			name := strings.TrimSpace(row[1])
			if name == "" || strings.EqualFold(name, "Spell") {
				continue
			}
			ks := KnownSpell{Name: name, Level: level, Prepared: isYes(prep) || level == 0}
			ks.Id = Slugify(name)
			if rs != nil {
				if sp := rs.Spell(name); sp != nil {
					ks.Id = sp.Id
					ks.Name = sp.Name
					ks.Level = sp.Level
				}
			}
			if len(row) > 6 {
				// The notes column holds "concentration, ritual, <source>",
				// the derived keywords are dropped and the rest is the source.
				rest := []string{}
				for _, n := range splitList(row[6]) {
					if n == "concentration" || n == "ritual" {
						continue
					}
					rest = append(rest, n)
				}
				ks.Source = joinList(rest)
			}
			c.Spells = append(c.Spells, ks)
		}
	}
	sort.SliceStable(c.Spells, func(i, j int) bool { return c.Spells[i].Level < c.Spells[j].Level })

	// free text sections
	get := func(keys ...string) string {
		for _, k := range keys {
			if b, ok := body[k]; ok {
				if t := strings.TrimSpace(b.String()); t != "" {
					return t
				}
			}
		}
		return ""
	}
	c.Personality = get("personality/personality traits", "personality/traits")
	c.Ideals = get("personality/ideals")
	c.Bonds = get("personality/bonds")
	c.Flaws = get("personality/flaws")
	c.Backstory = get("backstory")
	c.Allies = get("allies & organizations", "allies and organizations", "allies")
	c.Treasure = get("treasure")
	c.Notes = get("notes")
	if app := get("appearance"); app != "" {
		// The first line is the generated age/height/... summary, drop it.
		parts := strings.SplitN(app, "\n", 2)
		if len(parts) == 2 && strings.HasPrefix(parts[0], "Age ") {
			c.Appearance = strings.TrimSpace(parts[1])
		} else if !strings.HasPrefix(parts[0], "Age ") {
			c.Appearance = app
		}
	}

	if len(c.Classes) == 0 {
		c.Classes = []ClassLevel{{Class: "", Level: 1}}
	}
	// Only the spell table is read back out of the sheet, so a character
	// written before its subclass granted spells - or hand edited - picks the
	// free ones up here. appendSpell makes this a no-op when they are present.
	GrantSubclassSpells(c, rs)
	return c, nil
}

func applyProperty(c *Character, key, val string) {
	switch key {
	case PropId:
		if val != "" {
			c.Id = val
		}
	case PropName:
		if val != "" {
			c.Name = val
		}
	case PropPlayer:
		c.Player = val
	case PropRuleset:
		if val != "" {
			c.Ruleset = val
		}
	case PropRace:
		c.Race = val
	case PropSubrace:
		c.Subrace = val
	case PropClasses:
		c.Classes = decodeClasses(val)
	case PropLevel:
		if len(c.Classes) == 1 && c.Classes[0].Level == 0 {
			c.Classes[0].Level = atoi(val)
		}
	case PropBackground:
		c.Background = val
	case PropAlignment:
		c.Alignment = val
	case PropXP:
		c.XP = atoi(val)
	case "DND_STR", "DND_DEX", "DND_CON", "DND_INT", "DND_WIS", "DND_CHA":
		c.Abilities[strings.ToLower(strings.TrimPrefix(key, "DND_"))] = atoi(val)
	case PropSkills:
		c.Skills = splitList(val)
	case PropExpertise:
		c.Expertise = splitList(val)
	case PropLanguages:
		c.Languages = splitList(val)
	case PropTools:
		c.Tools = splitList(val)
	case PropToolExpert:
		c.ToolExpertise = splitList(val)
	case PropFeats:
		c.Feats = splitList(val)
	case PropChoices:
		c.Choices = decodeChoices(val)
	case PropHPMax:
		c.HPMax = atoi(val)
	case PropHPCur:
		c.HPCurrent = atoi(val)
	case PropHPTemp:
		c.HPTemp = atoi(val)
	case PropHitDice:
		c.HitDiceUsed = atoi(val)
	case PropDeath:
		c.DeathSaves = val
	case PropInspire:
		c.Inspiration = isYes(val)
	case PropSlotsUsed:
		c.SlotsUsed = decodeInts(val)
	case PropUsesSpent:
		c.UsesSpent = decodeUses(val)
	case "DND_CP":
		c.Money.CP = atoi(val)
	case "DND_SP":
		c.Money.SP = atoi(val)
	case "DND_EP":
		c.Money.EP = atoi(val)
	case "DND_GP":
		c.Money.GP = atoi(val)
	case "DND_PP":
		c.Money.PP = atoi(val)
	case "DND_AGE":
		c.Age = val
	case "DND_HEIGHT":
		c.Height = val
	case "DND_WEIGHT":
		c.Weight = val
	case "DND_EYES":
		c.Eyes = val
	case "DND_SKIN":
		c.Skin = val
	case "DND_HAIR":
		c.Hair = val
	case PropImage:
		c.Image = val
	case PropImageFocus:
		c.ImageFocus = val
	case PropImageZoom:
		c.ImageZoom = ParseZoom(val)
	}
}

// ----------------------------------------------------------------------------
// encoding helpers
// ----------------------------------------------------------------------------

func encodeClasses(list []ClassLevel) string {
	parts := []string{}
	for _, cl := range list {
		parts = append(parts, fmt.Sprintf("%s:%s:%d", cl.Class, cl.Subclass, cl.Level))
	}
	return strings.Join(parts, ", ")
}

func decodeClasses(val string) []ClassLevel {
	out := []ClassLevel{}
	for _, part := range splitList(val) {
		bits := strings.Split(part, ":")
		cl := ClassLevel{Level: 1}
		if len(bits) > 0 {
			cl.Class = strings.TrimSpace(bits[0])
		}
		if len(bits) > 1 {
			cl.Subclass = strings.TrimSpace(bits[1])
		}
		if len(bits) > 2 {
			if l := atoi(bits[2]); l > 0 {
				cl.Level = l
			}
		}
		if cl.Class != "" {
			out = append(out, cl)
		}
	}
	return out
}

func encodeChoices(m map[string][]string) string {
	if len(m) == 0 {
		return ""
	}
	parts := []string{}
	for _, k := range sortedKeys(m) {
		if len(m[k]) == 0 {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%s", k, strings.Join(m[k], "|")))
	}
	return strings.Join(parts, "; ")
}

func decodeChoices(val string) map[string][]string {
	out := map[string][]string{}
	for _, part := range strings.Split(val, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		idx := strings.Index(part, "=")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(part[:idx])
		vals := []string{}
		for _, v := range strings.Split(part[idx+1:], "|") {
			if v = strings.TrimSpace(v); v != "" {
				vals = append(vals, v)
			}
		}
		out[key] = vals
	}
	return out
}

// encodeUses writes the spent uses of limited features as "second-wind=1;
// channel-divinity=2", keyed by the slug of the feature's name. Nothing is
// written for a feature with nothing spent, so a character who has rested
// carries no line at all.
func encodeUses(m map[string]int) string {
	if len(m) == 0 {
		return ""
	}
	parts := []string{}
	for _, k := range sortedKeys(m) {
		if m[k] <= 0 {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%d", k, m[k]))
	}
	return strings.Join(parts, "; ")
}

func decodeUses(val string) map[string]int {
	out := map[string]int{}
	for _, part := range strings.Split(val, ";") {
		part = strings.TrimSpace(part)
		idx := strings.Index(part, "=")
		if part == "" || idx < 0 {
			continue
		}
		key := strings.TrimSpace(part[:idx])
		if n := atoi(part[idx+1:]); key != "" && n > 0 {
			out[key] = n
		}
	}
	return out
}

func encodeInts(list []int) string {
	parts := []string{}
	for _, v := range list {
		parts = append(parts, itoa(v))
	}
	return strings.Join(parts, ", ")
}

func decodeInts(val string) []int {
	out := []int{}
	for _, p := range splitList(val) {
		out = append(out, atoi(p))
	}
	return out
}

func parseTableRow(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "|")
	line = strings.TrimSuffix(line, "|")
	cells := strings.Split(line, "|")
	for i := range cells {
		cells[i] = strings.TrimSpace(cells[i])
	}
	return cells
}

// orgTable renders an aligned org table. headerRows is the number of leading
// rows to separate with a rule.
func orgTable(rows [][]string, headerRows int) string {
	if len(rows) == 0 {
		return ""
	}
	cols := 0
	for _, r := range rows {
		if len(r) > cols {
			cols = len(r)
		}
	}
	widths := make([]int, cols)
	for _, r := range rows {
		for i, cell := range r {
			if len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}
	var b strings.Builder
	writeRow := func(r []string) {
		b.WriteString("|")
		for i := 0; i < cols; i++ {
			cell := ""
			if i < len(r) {
				cell = r[i]
			}
			b.WriteString(" ")
			b.WriteString(cell)
			b.WriteString(strings.Repeat(" ", widths[i]-len(cell)))
			b.WriteString(" |")
		}
		b.WriteString("\n")
	}
	writeRule := func() {
		b.WriteString("|")
		for i := 0; i < cols; i++ {
			b.WriteString(strings.Repeat("-", widths[i]+2))
			if i < cols-1 {
				b.WriteString("+")
			}
		}
		b.WriteString("|\n")
	}
	for i, r := range rows {
		writeRow(r)
		if headerRows > 0 && i == headerRows-1 {
			writeRule()
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func stripTags(heading string) string {
	idx := strings.LastIndex(heading, " :")
	if idx > 0 && strings.HasSuffix(heading, ":") {
		return strings.TrimSpace(heading[:idx])
	}
	return heading
}

func normalizeHeading(h string) string {
	return strings.ToLower(strings.TrimSpace(h))
}

func spellLevelFromName(name string) int {
	name = strings.ToLower(strings.TrimSpace(name))
	if strings.HasPrefix(name, "cantrip") {
		return 0
	}
	for i := 1; i <= 9; i++ {
		if strings.HasPrefix(name, strings.ToLower(Ordinal(i))) {
			return i
		}
	}
	return -1
}

func isYes(v string) bool {
	v = strings.ToLower(strings.TrimSpace(v))
	return v == "yes" || v == "y" || v == "x" || v == "t" || v == "true" || v == "*"
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func boolStr(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func itoa(v int) string { return strconv.Itoa(v) }

func orDefault(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

func trimFloat(f float64) string {
	if f == 0 {
		return ""
	}
	s := strconv.FormatFloat(f, 'f', -1, 64)
	return s
}

// wrapText soft wraps rules text so the org file stays readable.
func wrapText(text string, width int) string {
	out := []string{}
	for _, para := range strings.Split(text, "\n") {
		words := strings.Fields(para)
		if len(words) == 0 {
			out = append(out, "")
			continue
		}
		line := ""
		for _, wd := range words {
			if line == "" {
				line = wd
			} else if len(line)+1+len(wd) <= width {
				line += " " + wd
			} else {
				out = append(out, line)
				line = wd
			}
		}
		if line != "" {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}
