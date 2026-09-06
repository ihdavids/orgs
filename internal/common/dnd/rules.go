package dnd

/* SDOC: DnD
* The Rules Engine

  =dnd.Compute(character, ruleset)= turns the small amount of state stored in
  an org character sheet into a fully derived =Sheet=: ability modifiers,
  saving throws, every skill, armour class, initiative, hit dice, attacks,
  spell save DC, spell slots, carrying capacity and the collected feature
  text from race, subrace, class, subclass and background.

  Nothing derived is stored in the org file, which means levelling up or
  changing a single ability score in the property drawer immediately produces
  a correct sheet on the next export.
EDOC */

import (
	"fmt"
	"sort"
	"strings"
)

// FullCasterSlots is the standard spell slot progression, indexed by caster
// level. Each row lists slots for spell levels 1..9.
var FullCasterSlots = [][]int{
	{0, 0, 0, 0, 0, 0, 0, 0, 0}, // 0
	{2, 0, 0, 0, 0, 0, 0, 0, 0}, // 1
	{3, 0, 0, 0, 0, 0, 0, 0, 0},
	{4, 2, 0, 0, 0, 0, 0, 0, 0},
	{4, 3, 0, 0, 0, 0, 0, 0, 0},
	{4, 3, 2, 0, 0, 0, 0, 0, 0}, // 5
	{4, 3, 3, 0, 0, 0, 0, 0, 0},
	{4, 3, 3, 1, 0, 0, 0, 0, 0},
	{4, 3, 3, 2, 0, 0, 0, 0, 0},
	{4, 3, 3, 3, 1, 0, 0, 0, 0},
	{4, 3, 3, 3, 2, 0, 0, 0, 0}, // 10
	{4, 3, 3, 3, 2, 1, 0, 0, 0},
	{4, 3, 3, 3, 2, 1, 0, 0, 0},
	{4, 3, 3, 3, 2, 1, 1, 0, 0},
	{4, 3, 3, 3, 2, 1, 1, 0, 0},
	{4, 3, 3, 3, 2, 1, 1, 1, 0}, // 15
	{4, 3, 3, 3, 2, 1, 1, 1, 0},
	{4, 3, 3, 3, 2, 1, 1, 1, 1},
	{4, 3, 3, 3, 3, 1, 1, 1, 1},
	{4, 3, 3, 3, 3, 2, 1, 1, 1},
	{4, 3, 3, 3, 3, 2, 2, 1, 1}, // 20
}

// PactSlots is the warlock pact magic table: {slots, slot level} by class level.
var PactSlots = [][]int{
	{0, 0},
	{1, 1}, {2, 1}, {2, 2}, {2, 2}, {2, 3},
	{2, 3}, {2, 4}, {2, 4}, {2, 5}, {2, 5},
	{3, 5}, {3, 5}, {3, 5}, {3, 5}, {3, 5},
	{3, 5}, {4, 5}, {4, 5}, {4, 5}, {4, 5},
}

// Compute derives a full character sheet from stored character state.
func Compute(c *Character, rs *Ruleset) *Sheet {
	s := &Sheet{Character: c, Warnings: []string{}}
	if c == nil {
		return s
	}
	if rs == nil {
		s.Warnings = append(s.Warnings, "no ruleset available, sheet is incomplete")
		rs = &Ruleset{}
		rs.Index()
	}
	level := c.TotalLevel()

	s.Id = CharacterId(c)
	s.Name = c.Name
	s.Player = c.Player
	s.Alignment = c.Alignment
	s.Level = level
	s.XP = c.XP
	s.NextLevelXP = XPForLevel(level + 1)
	s.XPPercent = xpProgress(c.XP, level)
	s.Image = c.Image
	s.ImageFocusX, s.ImageFocusY = ParseFocus(c.ImageFocus)
	s.ImageZoom = c.ImageZoom
	if s.ImageZoom <= 0 {
		s.ImageZoom = 1
	}
	s.RulesetName = rs.Name
	s.Proficiency = ProficiencyBonus(level)
	s.ProficiencyStr = Signed(s.Proficiency)
	s.Personality = c.Personality
	s.Ideals = c.Ideals
	s.Bonds = c.Bonds
	s.Flaws = c.Flaws
	s.Age = c.Age
	s.Height = c.Height
	s.Weight_ = c.Weight
	s.Eyes = c.Eyes
	s.Skin = c.Skin
	s.Hair = c.Hair
	s.Appearance = c.Appearance
	s.Backstory = c.Backstory
	s.Allies = c.Allies
	s.Treasure = c.Treasure
	s.Notes = c.Notes
	s.Money = c.Money
	s.Purse = ComputeMoney(c.Money)
	s.Inspiration = c.Inspiration
	s.DeathSaves = c.DeathSaves

	// ---- race -------------------------------------------------------------
	race := rs.Race(c.Race)
	sub := (*Race)(nil)
	if c.Subrace != "" {
		sub = rs.Race(c.Subrace)
	}
	raceName := ""
	if race != nil {
		raceName = race.Name
		s.Speed = race.Speed
		s.Size = race.Size
		s.Darkvision = race.Darkvision
	} else if c.Race != "" {
		raceName = Titleize(c.Race)
		s.Warnings = append(s.Warnings, fmt.Sprintf("unknown race %q", c.Race))
	}
	if sub != nil {
		raceName = sub.Name
		if sub.Speed != 0 {
			s.Speed = sub.Speed
		}
		if sub.Darkvision != 0 {
			s.Darkvision = sub.Darkvision
		}
	}
	if s.Speed == 0 {
		s.Speed = 30
	}
	if s.Size == "" {
		s.Size = "Medium"
	}
	s.RaceName = raceName

	// ---- classes ----------------------------------------------------------
	classNames := []string{}
	primaryClass := (*Class)(nil)
	for i, cl := range c.Classes {
		cls := rs.Class(cl.Class)
		name := Titleize(cl.Class)
		if cls != nil {
			name = cls.Name
			if i == 0 {
				primaryClass = cls
			}
		} else if cl.Class != "" {
			s.Warnings = append(s.Warnings, fmt.Sprintf("unknown class %q", cl.Class))
		}
		if cl.Subclass != "" {
			if sc := rs.Subclass(cl.Class, cl.Subclass); sc != nil {
				if i == 0 {
					s.SubclassName = sc.Name
				}
				name = fmt.Sprintf("%s (%s)", name, sc.Name)
			} else {
				name = fmt.Sprintf("%s (%s)", name, Titleize(cl.Subclass))
			}
		}
		if i == 0 {
			s.ClassName = name
		}
		classNames = append(classNames, fmt.Sprintf("%s %d", name, cl.Level))
	}
	s.ClassLine = strings.Join(classNames, " / ")

	// ---- background -------------------------------------------------------
	bg := rs.Background(c.Background)
	if bg != nil {
		s.Background = bg.Name
	} else if c.Background != "" {
		s.Background = Titleize(c.Background)
	}

	// ---- proficiencies and languages -------------------------------------
	//
	// This runs before abilities and skills below, because a race, subclass,
	// background or feat can grant saving throw and skill proficiency as
	// readily as it grants armour, weapons or tools, and both of those
	// sections need the answer. Everything a source hands out goes through
	// collect, so there is one place to look and one place to extend.
	armorProf := []string{}
	weaponProf := []string{}
	toolProf := append([]string{}, c.Tools...)
	// Expertise in a tool implies proficiency with it, the same way skill
	// expertise does, so a sheet that only records the doubling still lists
	// the tool.
	toolProf = addUnique(toolProf, c.ToolExpertise...)
	langs := append([]string{}, c.Languages...)
	// Skills and saves granted by a source are derived here rather than
	// written onto the character, so that reading a sheet back in cannot
	// double count them. The character's own picks stay in c.Skills.
	grantedSkills := map[string]bool{}
	saveProf := map[string]bool{}
	collect := func(p Proficiencies) {
		armorProf = addUnique(armorProf, p.Armor...)
		weaponProf = addUnique(weaponProf, p.Weapons...)
		toolProf = addUnique(toolProf, p.Tools...)
		for _, sk := range p.Skills {
			grantedSkills[sk] = true
		}
		for _, sv := range p.Saves {
			saveProf[sv] = true
		}
	}
	if primaryClass != nil {
		for _, sv := range primaryClass.SavingThrows {
			saveProf[sv] = true
		}
	}
	if race != nil {
		collect(race.Proficiencies)
		langs = addUnique(langs, race.Languages...)
	}
	if sub != nil {
		collect(sub.Proficiencies)
		langs = addUnique(langs, sub.Languages...)
	}
	for _, cl := range c.Classes {
		if cls := rs.Class(cl.Class); cls != nil {
			collect(cls.Proficiencies)
		}
		// A subclass may hold several blocks, each with the class level it
		// arrives at, so that a grant like the Gloom Stalker's Wisdom save
		// lands at 7th rather than when the subclass is chosen at 3rd.
		if sc := rs.Subclass(cl.Class, cl.Subclass); sc != nil {
			for _, p := range sc.Proficiency.At(cl.Level) {
				collect(p)
			}
		}
	}
	if bg != nil {
		collect(bg.Proficiencies)
	}
	// Feats such as Heavily Armored, Weapon Master, Skilled and Resilient.
	for _, id := range c.Feats {
		ft := rs.Feat(id)
		if ft == nil {
			continue
		}
		collect(ft.Proficiencies)
		// Resilient makes you proficient in whichever save its abilityChoice
		// landed on, which is not something Proficiencies can express.
		if ft.AbilityChoice != nil && ft.AbilityChoice.GrantsSave {
			for _, ab := range c.Choices[featAbilityChoiceId(id)] {
				saveProf[ab] = true
			}
		}
	}
	s.ArmorProficiencies = prettyProficiencies(rs, armorProf, "armor")
	s.WeaponProficiencies = prettyProficiencies(rs, weaponProf, "weapon")
	s.ToolProficiencies = markToolExpertise(
		prettyProficiencies(rs, toolProf, "tool"),
		prettyProficiencies(rs, c.ToolExpertise, "tool"))
	s.Languages = langs

	// ---- magic items ------------------------------------------------------
	// Worked out here because a ring or cloak of protection raises every
	// saving throw, and the saves are computed immediately below.
	magic := magicItems(c, rs, s)

	// ---- traits, features and the bonuses they carry ----------------------
	// Collected before the saves because a paladin's aura raises every one of
	// them, and before initiative because a bard's Jack of All Trades does.
	// The plain ability modifiers come first, because a feature's use limit
	// is often a modifier - "a number of times equal to your Charisma
	// modifier" - and the collectors work that out as they go.
	scores := map[string]int{}
	rawMods := map[string]int{}
	for _, a := range AbilityOrder {
		score := 10
		if c.Abilities != nil {
			if v, ok := c.Abilities[a]; ok && v > 0 {
				score = v
			}
		}
		scores[a] = score
		rawMods[a] = AbilityMod(score)
	}
	s.Traits = collectTraits(rs, c, race, sub, rawMods, s.Proficiency)
	s.Features = collectFeatures(rs, c, bg, rawMods, s.Proficiency)
	bonus := collectBonuses(rs, c, s.Traits, s.Features, rawMods, s.Proficiency, armorWorn(c, rs))

	// ---- abilities and saves ---------------------------------------------
	s.AbilityMap = map[string]AbilityView{}
	for _, a := range AbilityOrder {
		score := scores[a]
		mod := rawMods[a]
		save := mod + magic.save + bonus.saves
		if saveProf[a] {
			save += s.Proficiency
		}
		check := mod + bonus.checks[a]
		av := AbilityView{
			Id: a, Name: AbilityNames[a], Short: AbilityShort[a],
			Score: score, Modifier: mod, Mod: Signed(mod),
			Check: check, CheckStr: Signed(check),
			Save: save, SaveStr: Signed(save), SaveProf: saveProf[a],
		}
		s.Abilities = append(s.Abilities, av)
		s.AbilityMap[a] = av
		s.Saves = append(s.Saves, av)
	}
	mod := func(a string) int { return s.AbilityMap[a].Modifier }

	// A death save is a flat d20, but it is still a saving throw, so a ring of
	// protection or a paladin's aura applies to it.
	s.DeathSaveBonus = magic.save + bonus.saves + bonus.deathSave
	s.DeathSaveStr = Signed(s.DeathSaveBonus)

	// ---- skills -----------------------------------------------------------
	skills := rs.Skills
	if len(skills) == 0 {
		skills = DefaultSkills
	}
	for _, sk := range skills {
		m := mod(sk.Ability)
		prof := c.HasSkill(sk.Id) || grantedSkills[sk.Id]
		exp := c.HasExpertise(sk.Id)
		if prof {
			m += s.Proficiency
		}
		if exp {
			m += s.Proficiency
			if !prof {
				m += s.Proficiency // expertise implies proficiency
			}
		}
		// Jack of All Trades and Remarkable Athlete are both worded as
		// applying only to checks that do not already add proficiency.
		if !prof && !exp {
			m += bonus.checks[sk.Ability]
		}
		sv := SkillView{
			Id: sk.Id, Name: sk.Name, Ability: sk.Ability, Short: AbilityShort[sk.Ability],
			Modifier: m, Mod: Signed(m), Proficent: prof, Expertise: exp, Passive: 10 + m,
		}
		s.Skills = append(s.Skills, sv)
		switch sk.Id {
		case "perception":
			s.PassivePerception = 10 + m
		case "insight":
			s.PassiveInsight = 10 + m
		case "investigation":
			s.PassiveInvestigation = 10 + m
		}
	}

	// ---- hit points -------------------------------------------------------
	s.HPMax = c.HPMax
	if s.HPMax <= 0 {
		s.HPMax = DefaultHitPoints(c, rs)
	}
	s.HPCurrent = c.HPCurrent
	if s.HPCurrent == 0 && c.HPMax == 0 {
		s.HPCurrent = s.HPMax
	}
	s.HPTemp = c.HPTemp
	s.HitDiceUsed = c.HitDiceUsed
	s.HitDice = hitDiceString(c, rs)
	if s.HPMax > 0 {
		s.HPPercent = s.HPCurrent * 100 / s.HPMax
		if s.HPPercent < 0 {
			s.HPPercent = 0
		} else if s.HPPercent > 100 {
			s.HPPercent = 100
		}
	}

	// ---- armour class, initiative, movement -------------------------------
	s.AC, s.ACSource = computeAC(c, rs, s, magic)
	// Initiative is a dexterity check, so it takes the check bonus as well as
	// anything pointed at initiative specifically (the Alert feat).
	s.Initiative = mod(DEX) + bonus.checks[DEX] + bonus.initiative
	s.InitiativeStr = Signed(s.Initiative)
	str := s.AbilityMap[STR].Score
	s.CarryCapacity = str * 15
	s.PushDragLift = str * 30

	// ---- equipment --------------------------------------------------------
	equip := []Gear{}
	for _, g := range c.Equipment {
		if g.Qty <= 0 {
			g.Qty = 1
		}
		if g.Weight == 0 {
			if it := rs.Item(gearKey(g)); it != nil {
				g.Weight = it.Weight
			}
		}
		if g.Name == "" {
			if it := rs.Item(g.Id); it != nil {
				g.Name = it.Name
			} else {
				g.Name = Titleize(g.Id)
			}
		}
		g.Container = ContainerKey(g.Container)
		equip = append(equip, g)
	}
	s.Equipment = equip
	// The inventory stacks that same list into its containers and works out
	// the encumbrance. What is inside an extradimensional container is not on
	// the character's back, so the weight carried comes from there rather
	// than from a plain sum of the table.
	s.Inventory = ComputeInventory(equip, rs, str)
	s.Weight = s.Inventory.Weight
	// A barbarian's Fast Movement and a monk's Unarmored Movement move the
	// walking speed. Both are conditional on what is worn, which
	// collectBonuses has already tested, so by here the total is either in or
	// out. It is applied before the encumbrance penalty so that a slowed
	// character is slowed from the speed they actually have.
	s.Speed += bonus.speed
	// Heavy armour you are too weak for slows you down.
	if armor := equippedArmor(c, rs); armor != nil && armor.StrengthReq > 0 && str < armor.StrengthReq {
		s.Speed -= 10
	}
	if s.Speed < 0 {
		s.Speed = 0
	}

	// ---- attacks ----------------------------------------------------------
	s.Attacks = computeAttacks(c, rs, s, magic)

	// ---- spellcasting -----------------------------------------------------
	computeSpellcasting(c, rs, s)

	return s
}

// markToolExpertise notes which tools double the proficiency bonus.
//
// A skill's expertise gets a column of its own because a skill has one ability
// behind it and so one number to print. A tool does not - picking a lock is a
// Dexterity check and identifying a poison an Intelligence one, both with the
// same kit - so there is no row to double. Saying it beside the tool's name is
// what the sheet can honestly do, and it is enough: the player already knows
// which ability the DM asked for.
//
// Both lists arrive already prettified, so the names are compared as the
// reader sees them.
func markToolExpertise(names, expert []string) []string {
	if len(expert) == 0 {
		return names
	}
	out := make([]string, 0, len(names))
	for _, name := range names {
		for _, e := range expert {
			if strings.EqualFold(name, e) {
				name += " (expertise)"
				break
			}
		}
		out = append(out, name)
	}
	return out
}

// prettyProficiencies turns stored proficiency ids into something a player
// wants to read: "simple" becomes "simple weapons", item ids become names.
func prettyProficiencies(rs *Ruleset, list []string, kind string) []string {
	out := []string{}
	for _, p := range list {
		name := p
		switch {
		case kind == "armor" && (p == "light" || p == "medium" || p == "heavy"):
			name = p + " armor"
		case kind == "armor" && p == "shields":
			name = "shields"
		case kind == "weapon" && (p == "simple" || p == "martial"):
			name = p + " weapons"
		default:
			if it := rs.Item(p); it != nil {
				name = it.Name
			} else if !strings.ContainsAny(p, " '") {
				name = Titleize(p)
			}
		}
		out = addUnique(out, name)
	}
	return out
}

func gearKey(g Gear) string {
	if g.Id != "" {
		return g.Id
	}
	return g.Name
}

// DefaultSkills is used when a ruleset does not define its own skill list.
var DefaultSkills = []Skill{
	{Id: "acrobatics", Name: "Acrobatics", Ability: DEX},
	{Id: "animal-handling", Name: "Animal Handling", Ability: WIS},
	{Id: "arcana", Name: "Arcana", Ability: INT},
	{Id: "athletics", Name: "Athletics", Ability: STR},
	{Id: "deception", Name: "Deception", Ability: CHA},
	{Id: "history", Name: "History", Ability: INT},
	{Id: "insight", Name: "Insight", Ability: WIS},
	{Id: "intimidation", Name: "Intimidation", Ability: CHA},
	{Id: "investigation", Name: "Investigation", Ability: INT},
	{Id: "medicine", Name: "Medicine", Ability: WIS},
	{Id: "nature", Name: "Nature", Ability: INT},
	{Id: "perception", Name: "Perception", Ability: WIS},
	{Id: "performance", Name: "Performance", Ability: CHA},
	{Id: "persuasion", Name: "Persuasion", Ability: CHA},
	{Id: "religion", Name: "Religion", Ability: INT},
	{Id: "sleight-of-hand", Name: "Sleight of Hand", Ability: DEX},
	{Id: "stealth", Name: "Stealth", Ability: DEX},
	{Id: "survival", Name: "Survival", Ability: WIS},
}

// bonusTotals is everything the data driven Bonuses blocks added up.
type bonusTotals struct {
	// checks is per ability, because Remarkable Athlete only covers three.
	checks     map[string]int
	initiative int
	saves      int
	deathSave  int
	speed      int
}

// wornArmor is what a Bonuses block's Unless clause is tested against. It is
// worked out from the equipment before the bonuses are collected, because a
// barbarian's Fast Movement and a monk's Unarmored Movement both depend on it.
type wornArmor struct {
	any    bool
	heavy  bool
	shield bool
}

func armorWorn(c *Character, rs *Ruleset) wornArmor {
	w := wornArmor{}
	if a := equippedArmor(c, rs); a != nil {
		w.any = true
		w.heavy = a.ArmorType == "heavy"
	}
	if equippedShield(c, rs) != nil {
		w.shield = true
	}
	return w
}

// blocks reports whether the named condition is currently true, and so whether
// a Bonuses block naming it is switched off. An unrecognised name never
// blocks: a module that invents one keeps its bonus rather than losing it to a
// condition nothing here knows how to test.
func (w wornArmor) blocks(unless string) bool {
	switch strings.ToLower(strings.TrimSpace(unless)) {
	case "":
		return false
	case "heavyarmor", "heavyarmour", "heavy-armor", "heavy armor":
		return w.heavy
	case "armor", "armour":
		return w.any || w.shield
	}
	return false
}

// collectBonuses adds up the Bonuses blocks on everything the character has:
// racial traits, class and subclass features, the options they picked, their
// background feature and their feats.
//
// The traits and features are passed in already collected and already level
// gated, so a feature the character has not reached yet cannot pay out early.
func collectBonuses(rs *Ruleset, c *Character, traits, features []Trait,
	mods map[string]int, prof int, worn wornArmor) bonusTotals {
	b := bonusTotals{checks: map[string]int{}}
	eval := func(formula string, minimum int) int {
		if formula == "" {
			return 0
		}
		v := EvalBonus(formula, mods, prof)
		if v < minimum {
			v = minimum
		}
		return v
	}
	apply := func(bs Bonuses) {
		if worn.blocks(bs.Unless) {
			return
		}
		if bs.Checks != "" {
			v := eval(bs.Checks, bs.Minimum)
			abilities := bs.Abilities
			if len(abilities) == 0 {
				abilities = AbilityOrder
			}
			for _, a := range abilities {
				b.checks[strings.ToLower(strings.TrimSpace(a))] += v
			}
		}
		b.initiative += eval(bs.Initiative, bs.Minimum)
		b.saves += eval(bs.Saves, bs.Minimum)
		b.deathSave += eval(bs.DeathSave, bs.Minimum)
		b.speed += eval(bs.Speed, bs.Minimum)
	}
	for _, t := range traits {
		apply(t.Bonuses)
	}
	for _, f := range features {
		apply(f.Bonuses)
	}
	for _, id := range c.Feats {
		if ft := rs.Feat(id); ft != nil {
			apply(ft.Bonuses)
		}
	}
	return b
}

func collectTraits(rs *Ruleset, c *Character, race, sub *Race, mods map[string]int, proficiency int) []Trait {
	out := []Trait{}
	add := func(list []Trait, source string) {
		for _, t := range list {
			if t.Level > 0 && t.Level > c.TotalLevel() {
				continue
			}
			t.Source = source
			out = append(out, t)
		}
	}
	if race != nil {
		add(race.Traits, race.Name)
	}
	if sub != nil {
		add(sub.Traits, sub.Name)
	}
	// A racial trait that scales - "twice starting at 11th level" - scales
	// against the whole character, not against any one class.
	AnnotateUses(c, out, c.TotalLevel(), mods, proficiency)
	return out
}

func collectFeatures(rs *Ruleset, c *Character, bg *Background, mods map[string]int, proficiency int) []Trait {
	out := []Trait{}
	// A class feature's limit scales against the level in the class that
	// granted it, so each batch is annotated with that class's level before
	// it joins the list.
	annotate := func(from int, level int) {
		AnnotateUses(c, out[from:], level, mods, proficiency)
	}
	for _, cl := range c.Classes {
		cls := rs.Class(cl.Class)
		if cls == nil {
			continue
		}
		at := len(out)
		for _, f := range cls.Features {
			if f.Level > cl.Level {
				continue
			}
			f.Source = fmt.Sprintf("%s %d", cls.Name, maxInt(f.Level, 1))
			out = append(out, f)
		}
		if sc := rs.Subclass(cl.Class, cl.Subclass); sc != nil {
			for _, f := range sc.Features {
				if f.Level > cl.Level {
					continue
				}
				f.Source = fmt.Sprintf("%s %d", sc.Name, maxInt(f.Level, 1))
				out = append(out, f)
			}
		}
		annotate(at, cl.Level)
	}
	// Options the player picked (fighting styles, invocations, ...)
	for _, cl := range c.Classes {
		cls := rs.Class(cl.Class)
		if cls == nil {
			continue
		}
		choices := append([]Choice{}, cls.Choices...)
		if sc := rs.Subclass(cl.Class, cl.Subclass); sc != nil {
			choices = append(choices, sc.Choices...)
		}
		at := len(out)
		for _, ch := range choices {
			if ch.Kind != "options" {
				continue
			}
			picked := c.Choices[ch.Id]
			for _, p := range picked {
				for _, o := range ch.Options {
					if o.Name == p || Slugify(o.Name) == p {
						o.Source = ch.Name
						out = append(out, o)
					}
				}
			}
		}
		annotate(at, cl.Level)
	}
	at := len(out)
	if bg != nil && bg.Feature.Name != "" {
		f := bg.Feature
		f.Source = bg.Name
		out = append(out, f)
	}
	for _, id := range c.Feats {
		if ft := rs.Feat(id); ft != nil {
			out = append(out, Trait{Name: ft.Name, Text: ft.Text, Source: "Feat"})
		}
	}
	// A background feature or a feat belongs to the character rather than to
	// any one class, so both scale against the total level.
	annotate(at, c.TotalLevel())
	sort.SliceStable(out, func(i, j int) bool { return out[i].Level < out[j].Level })
	return out
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// DefaultHitPoints computes hit points using the fixed "average" rule.
func DefaultHitPoints(c *Character, rs *Ruleset) int {
	conMod := 0
	if c.Abilities != nil {
		conMod = AbilityMod(c.Abilities[CON])
	}
	hp := 0
	first := true
	extraPerLevel := 0
	if race := rs.Race(c.Race); race != nil {
		extraPerLevel += race.HpPerLevel
	}
	if c.Subrace != "" {
		if sr := rs.Race(c.Subrace); sr != nil {
			extraPerLevel += sr.HpPerLevel
		}
	}
	for _, cl := range c.Classes {
		die := 8
		if cls := rs.Class(cl.Class); cls != nil && cls.HitDie > 0 {
			die = cls.HitDie
		}
		for i := 0; i < cl.Level; i++ {
			if first {
				hp += die + conMod
				first = false
			} else {
				hp += AverageRoll(die) + conMod
			}
		}
	}
	hp += extraPerLevel * c.TotalLevel()
	if hp < 1 {
		hp = 1
	}
	return hp
}

func hitDiceString(c *Character, rs *Ruleset) string {
	dice := map[int]int{}
	for _, cl := range c.Classes {
		die := 8
		if cls := rs.Class(cl.Class); cls != nil && cls.HitDie > 0 {
			die = cls.HitDie
		}
		dice[die] += cl.Level
	}
	keys := []int{}
	for k := range dice {
		keys = append(keys, k)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(keys)))
	parts := []string{}
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%dd%d", dice[k], k))
	}
	return strings.Join(parts, " + ")
}

func equippedArmor(c *Character, rs *Ruleset) *Item {
	for _, g := range c.Equipment {
		if !g.Equipped {
			continue
		}
		// AC 0 means the entry is a template rather than a wearable suit -
		// the SRD writes "Armor of Resistance" as "Armor (light, medium, or
		// heavy)" without saying which. Wearing one of those would otherwise
		// drop the character to AC 0; the way to use it is to declare a
		// variant with base: set to the armour you actually wear.
		if it := rs.Item(gearKey(g)); it != nil && it.Kind == "armor" &&
			it.ArmorType != "shield" && it.AC > 0 {
			return it
		}
	}
	return nil
}

func equippedShield(c *Character, rs *Ruleset) *Item {
	for _, g := range c.Equipment {
		if !g.Equipped {
			continue
		}
		if it := rs.Item(gearKey(g)); it != nil && (it.ArmorType == "shield" || it.Kind == "shield") {
			return it
		}
	}
	return nil
}

// AttunementSlots is how many items a character can be attuned to at once.
const AttunementSlots = 3

// itemBonuses is what the equipped magic items add up to.
type itemBonuses struct {
	ac     int
	save   int
	attack map[string]int // item id -> attack bonus
	damage map[string]int // item id -> damage bonus
	// active reports whether a given equipment line's bonuses count. An item
	// that requires attunement and has not got it contributes nothing.
	active map[string]bool
}

// magicItems walks the character's equipment and works out what the magic
// among it actually does: the flat AC and saving throw bonuses that apply
// while it is worn, the per weapon attack and damage bonuses, the attunement
// slots in use, and the display rows for the sheet.
//
// The rules it enforces are the two general ones. An item that requires
// attunement does nothing until it is attuned, and a character has only
// AttunementSlots of those to give. Attuning to more is not silently allowed:
// the items past the limit stay inert and the sheet carries a warning.
func magicItems(c *Character, rs *Ruleset, s *Sheet) itemBonuses {
	b := itemBonuses{
		attack: map[string]int{},
		damage: map[string]int{},
		active: map[string]bool{},
	}
	s.AttunementSlots = AttunementSlots
	used := 0
	for _, g := range c.Equipment {
		it := rs.Item(gearKey(g))
		if !it.IsMagic() {
			continue
		}
		state := ""
		live := g.Equipped
		if it.Attunement {
			switch {
			case !g.Attuned:
				state = "required"
				live = false
			case used >= AttunementSlots:
				state = "required"
				live = false
				s.Warnings = append(s.Warnings, fmt.Sprintf(
					"%s is marked attuned but you are already attuned to %d items, the maximum",
					it.Name, AttunementSlots))
			default:
				state = "attuned"
				used++
				s.Attuned = append(s.Attuned, it.Name)
			}
		}
		effect := magicItemEffect(it)
		s.MagicItems = append(s.MagicItems, MagicItemView{
			Id: it.Id, Name: it.Name, Rarity: it.RarityName(),
			Equipped: g.Equipped, Attunement: state,
			Note: it.AttunementNote, Effect: effect,
		})
		if !live {
			continue
		}
		b.active[it.Id] = true
		// Armour and shields fold their bonus into the armour calculation in
		// computeAC, so only the free floating ones are summed here.
		if it.Kind != "armor" && it.Kind != "shield" {
			b.ac += it.ACBonus
		}
		b.save += it.SaveBonus
		if it.AttackBonus != 0 {
			b.attack[it.Id] = it.AttackBonus
		}
		if it.DamageBonus != 0 {
			b.damage[it.Id] = it.DamageBonus
		}
	}
	s.AttunementUsed = used
	return b
}

// magicItemEffect is the short "what does it do" line on the sheet, built from
// whatever bonuses the item declares. Items whose effect is prose only get an
// empty string and are shown by name alone.
func magicItemEffect(it *Item) string {
	parts := []string{}
	if it.AttackBonus != 0 || it.DamageBonus != 0 {
		if it.AttackBonus == it.DamageBonus {
			parts = append(parts, Signed(it.AttackBonus)+" attack and damage")
		} else {
			if it.AttackBonus != 0 {
				parts = append(parts, Signed(it.AttackBonus)+" attack")
			}
			if it.DamageBonus != 0 {
				parts = append(parts, Signed(it.DamageBonus)+" damage")
			}
		}
	}
	if it.ACBonus != 0 {
		parts = append(parts, Signed(it.ACBonus)+" AC")
	}
	if it.SaveBonus != 0 {
		parts = append(parts, Signed(it.SaveBonus)+" saving throws")
	}
	if it.Charges > 0 {
		parts = append(parts, fmt.Sprintf("%d charges", it.Charges))
	}
	return strings.Join(parts, ", ")
}

// acOption is one way a character's armour class could be worked out: what it
// comes to, what to call it, and whether a shield may be carried with it.
type acOption struct {
	ac     int
	source string
	shield bool
}

// computeAC works the armour class out the way D&D Beyond does: every
// calculation the character has available is worked out and the best one is
// what the sheet shows. A barbarian who keeps a leather jerkin in their pack
// is not made worse off by owning it, and a monk who picks up a breastplate
// still gets their own defence if it is better.
//
// That is deliberately more generous than the letter of the rules, where an
// unarmored defence applies only while wearing no armour. It is what the
// character could have with a moment's undressing, and it is the number
// players see on their D&D Beyond sheet, so it is the one to agree with.
func computeAC(c *Character, rs *Ruleset, s *Sheet, magic itemBonuses) (int, string) {
	dex := s.AbilityMap[DEX].Modifier
	options := []acOption{}

	// What is actually being worn, first, so it wins a tie: a character in
	// armour should be told the armour's name.
	if armor := equippedArmor(c, rs); armor != nil {
		base := armor.AC
		switch armor.ArmorType {
		case "light":
			base += dex
		case "medium":
			d := dex
			cap := armor.DexMax
			if cap == 0 {
				cap = 2
			}
			if d > cap {
				d = cap
			}
			base += d
		case "heavy":
			// no dex
		default:
			if !armor.NoDex {
				base += dex
			}
		}
		if magic.active[armor.Id] {
			base += armor.ACBonus
		}
		options = append(options, acOption{ac: base, source: armor.Name, shield: true})
	}

	// Wearing nothing at all, and then any unarmored defence style the
	// character's classes grant.
	options = append(options, acOption{ac: 10 + dex, source: "Unarmored", shield: true})
	for _, cl := range c.Classes {
		cls := rs.Class(cl.Class)
		if cls == nil || cls.UnarmoredAC == "" {
			continue
		}
		v := 10
		for _, part := range strings.Split(cls.UnarmoredAC, "+") {
			part = strings.TrimSpace(strings.ToLower(part))
			if part == "" {
				continue
			}
			if n := atoi(part); n > 0 {
				v = n
				continue
			}
			if av, ok := s.AbilityMap[part]; ok {
				v += av.Modifier
			}
		}
		options = append(options, acOption{
			ac: v, source: cls.Name + " unarmored defense", shield: cls.UnarmoredShield})
	}

	// A shield is worth a couple of points to most of these but not to all of
	// them, so it is counted before they are compared rather than after: a
	// monk's defence has to beat armour and shield together to be the better
	// choice.
	shieldBonus, shieldName := 0, ""
	if shield := equippedShield(c, rs); shield != nil {
		shieldBonus = shield.AC
		if shieldBonus == 0 {
			shieldBonus = 2
		}
		if magic.active[shield.Id] {
			shieldBonus += shield.ACBonus
		}
		shieldName = shield.Name
	}

	ac, source, withShield := 0, "Unarmored", false
	for i, o := range options {
		total := o.ac
		carries := shieldName != "" && o.shield
		if carries {
			total += shieldBonus
		}
		if i == 0 || total > ac {
			ac, source, withShield = total, o.source, carries
		}
	}
	if withShield {
		source += " + " + shieldName
	}

	// Rings, cloaks and anything else that raises AC without being worn as
	// armour. Armour and shields are already counted above.
	if magic.ac != 0 {
		ac += magic.ac
		source += fmt.Sprintf(" %s magic", Signed(magic.ac))
	}
	return ac, source
}

// IsProficientWith reports whether a proficiency list covers an item.
func IsProficientWith(profs []string, it *Item) bool {
	if it == nil {
		return false
	}
	for _, p := range profs {
		p = strings.ToLower(strings.TrimSpace(p))
		switch {
		case p == "all":
			return true
		case p == it.Id || strings.EqualFold(p, it.Name):
			return true
		case p == strings.ToLower(it.Category):
			return true
		case it.Category != "" && strings.HasPrefix(strings.ToLower(it.Category), p):
			// "simple" matches "simple-melee" and "simple-ranged"
			return true
		case it.ArmorType != "" && p == it.ArmorType:
			return true
		case p == "light armor" && it.ArmorType == "light",
			p == "medium armor" && it.ArmorType == "medium",
			p == "heavy armor" && it.ArmorType == "heavy",
			p == "shields" && it.ArmorType == "shield":
			return true
		// The sheet holds display names, not raw ids - prettyProficiencies
		// turns "martial" into "martial weapons" - so the pluralised forms
		// have to match too, the same way the armour ones above do.
		case p == "simple weapons" && strings.HasPrefix(strings.ToLower(it.Category), "simple"),
			p == "martial weapons" && strings.HasPrefix(strings.ToLower(it.Category), "martial"):
			return true
		}
	}
	return false
}

func computeAttacks(c *Character, rs *Ruleset, s *Sheet, magic itemBonuses) []AttackView {
	out := []AttackView{}
	strMod := s.AbilityMap[STR].Modifier
	dexMod := s.AbilityMap[DEX].Modifier
	for _, g := range c.Equipment {
		if !g.Equipped {
			continue
		}
		it := rs.Item(gearKey(g))
		if it == nil || !it.IsWeapon() {
			continue
		}
		// A weapon with no damage is a template, not something you can swing:
		// the SRD types a flame tongue as "Weapon (any sword)" and leaves the
		// choice to the player. Give it a base: and it becomes a real weapon.
		if it.Damage == "" {
			continue
		}
		abilityMod := strMod
		ability := "STR"
		ranged := strings.Contains(strings.ToLower(it.Category), "ranged")
		if ranged {
			abilityMod = dexMod
			ability = "DEX"
		}
		if it.HasProperty("finesse") && dexMod > strMod {
			abilityMod = dexMod
			ability = "DEX"
		}
		prof := IsProficientWith(s.WeaponProficiencies, it)
		bonus := abilityMod + magic.attack[it.Id]
		if prof {
			bonus += s.Proficiency
		}
		// A magic weapon's damage bonus rides along with the ability modifier
		// rather than being printed separately, so "1d8+5" stays readable.
		dmgMod := abilityMod + magic.damage[it.Id]
		dmg := it.Damage
		if dmg != "" && dmgMod != 0 {
			dmg = fmt.Sprintf("%s%s", dmg, Signed(dmgMod))
		}
		notes := strings.ToLower(ability) + " based"
		if len(it.Properties) > 0 {
			notes += ", " + strings.Join(it.Properties, ", ")
		}
		// Versatile damage is carried as its own field rather than folded into
		// the notes, so that a sheet can roll the two handed damage on its own.
		versatile := ""
		if it.Versatile != "" {
			versatile = it.Versatile
			if dmgMod != 0 {
				versatile = fmt.Sprintf("%s%s", versatile, Signed(dmgMod))
			}
		}
		if !prof {
			notes = strings.TrimSpace(notes + ", not proficient")
		}
		if it.IsMagic() {
			notes = strings.TrimSpace(notes + ", magical")
			if it.Attunement && !magic.active[it.Id] {
				notes = strings.TrimSpace(notes + " (not attuned)")
			}
		}
		rng := it.Range
		if rng == "" {
			if ranged {
				rng = "ranged"
			} else {
				rng = "5 ft."
			}
		}
		out = append(out, AttackView{
			Name: it.Name, Bonus: Signed(bonus), Damage: dmg, Versatile: versatile,
			Type: it.DamageType, Range: rng, Notes: strings.TrimSpace(notes),
			Proficent: prof,
		})
	}
	// Unarmed strike is always available.
	out = append(out, AttackView{
		Name: "Unarmed Strike", Bonus: Signed(strMod + s.Proficiency),
		Damage: fmt.Sprintf("%d", maxInt(1+strMod, 0)), Type: "bludgeoning",
		Range: "5 ft.", Proficent: true,
	})
	return out
}

func computeSpellcasting(c *Character, rs *Ruleset, s *Sheet) {
	casterLevel := 0.0
	pactLevel := 0
	var sc *Spellcasting
	var scClass *Class
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
		if sc == nil {
			sc = info
			scClass = cls
		}
		multi := len(c.Classes) > 1
		switch info.Progression {
		case "full":
			casterLevel += float64(cl.Level)
		case "half":
			if multi {
				casterLevel += float64(cl.Level / 2)
			} else {
				casterLevel += float64((cl.Level + 1) / 2)
			}
		case "third":
			if multi {
				casterLevel += float64(cl.Level / 3)
			} else {
				casterLevel += float64((cl.Level + 2) / 3)
			}
		case "pact":
			pactLevel += cl.Level
		}
	}
	if sc == nil && pactLevel == 0 {
		// still show any spells the character knows (racial spells etc)
		if len(c.Spells) > 0 {
			s.IsCaster = true
			s.SpellLevels = groupSpells(c, rs, s, nil)
		}
		return
	}
	s.IsCaster = true
	ability := WIS
	if sc != nil && sc.Ability != "" {
		ability = sc.Ability
	}
	s.CastingAbility = AbilityShort[ability]
	s.CastingAbilityName = AbilityNames[ability]
	am := s.AbilityMap[ability].Modifier
	s.SpellSaveDC = 8 + s.Proficiency + am
	s.SpellAttack = s.Proficiency + am
	s.SpellAttackStr = Signed(s.SpellAttack)

	slots := make([]int, 9)
	cl := int(casterLevel)
	if cl > 0 {
		if cl >= len(FullCasterSlots) {
			cl = len(FullCasterSlots) - 1
		}
		table := FullCasterSlots
		if rs.SlotTables != nil {
			if t, ok := rs.SlotTables["full"]; ok && len(t) > cl {
				table = t
			}
		}
		copy(slots, table[cl])
	}
	if pactLevel > 0 {
		s.PactMagic = true
		pl := pactLevel
		if pl >= len(PactSlots) {
			pl = len(PactSlots) - 1
		}
		n, lvl := PactSlots[pl][0], PactSlots[pl][1]
		if lvl >= 1 && lvl <= 9 {
			slots[lvl-1] += n
		}
	}
	for i, n := range slots {
		if n == 0 {
			continue
		}
		used := 0
		if i < len(c.SlotsUsed) {
			used = c.SlotsUsed[i]
		}
		pips := []int{}
		for p := 1; p <= n; p++ {
			pips = append(pips, p)
		}
		s.Slots = append(s.Slots, SpellSlotView{Level: i + 1, Label: Ordinal(i + 1),
			Total: n, Used: used, Left: n - used, Pips: pips})
	}

	// known / prepared counts
	if sc != nil {
		lvl := 0
		for _, cle := range c.Classes {
			if scClass != nil && cle.Class == scClass.Id {
				lvl = cle.Level
			}
		}
		if len(sc.CantripsKnown) > lvl {
			s.CantripsKnown = sc.CantripsKnown[lvl]
		} else if len(sc.CantripsKnown) > 0 {
			s.CantripsKnown = sc.CantripsKnown[len(sc.CantripsKnown)-1]
		}
		if len(sc.SpellsKnown) > lvl {
			s.SpellsKnown = sc.SpellsKnown[lvl]
		} else if len(sc.SpellsKnown) > 0 {
			s.SpellsKnown = sc.SpellsKnown[len(sc.SpellsKnown)-1]
		}
		if sc.Prepares {
			s.PreparedMax = PreparedCount(sc, lvl, am)
		}
		notes := []string{}
		if sc.Ritual {
			notes = append(notes, "ritual casting")
		}
		if sc.Focus != "" {
			notes = append(notes, "focus: "+sc.Focus)
		}
		if sc.PreparedFrom == "spellbook" {
			notes = append(notes, "prepares from spellbook")
		}
		if sc.Notes != "" {
			notes = append(notes, sc.Notes)
		}
		s.SpellNotes = strings.Join(notes, ", ")
	}
	// What is prepared out of the allowance. Spells a subclass granted - a
	// cleric's domain spells, a paladin's oath spells - are always prepared
	// and do not come out of it, so counting them would read as over budget
	// on a sheet that is not.
	s.SpellsPrepared = CountSpells(c, -1, true)
	s.SpellLevels = groupSpells(c, rs, s, s.Slots)
}

// GrantSubclassSpells adds the spells a subclass hands out for free - cleric
// domain spells, paladin oath spells, the druid's circle spells - to the
// character's known spell list.
//
// A subclass entry with no Group is granted as soon as the class level reaches
// its Level. A subclass that offers alternative lists tags each spell with a
// Group and carries a Choice of kind "spellgroup"; only the groups the
// character actually picked are granted. A druid of the Circle of the Land
// initiated on the coast gets the coast list and nothing else.
//
// Granted spells are marked with the subclass name as their Source, which is
// what makes MarkPrepared treat them as always prepared and keeps them out of
// the character's prepared spell allowance.
//
// The function is idempotent: appendSpell skips ids the character already
// knows, so calling it twice, or after a level up, adds only what is new. That
// matters because the builder replays every answer onto a fresh character.
func GrantSubclassSpells(c *Character, rs *Ruleset) {
	if c == nil || rs == nil {
		return
	}
	for _, cl := range c.Classes {
		sc := rs.Subclass(cl.Class, cl.Subclass)
		if sc == nil || len(sc.Spells) == 0 {
			continue
		}
		picked := pickedSpellGroups(c, sc)
		for _, sp := range sc.Spells {
			if sp.Level > cl.Level {
				continue
			}
			if sp.Group != "" && !containsStr(picked, sp.Group) {
				continue
			}
			c.Spells = appendSpell(c.Spells, rs, sp.Id, sc.Name)
		}
	}
}

// pickedSpellGroups returns the spell groups the character chose, reading the
// answers stored against every "spellgroup" choice the subclass declares.
func pickedSpellGroups(c *Character, sc *Subclass) []string {
	out := []string{}
	for _, ch := range sc.Choices {
		if ch.Kind != "spellgroup" {
			continue
		}
		out = append(out, c.Choices[ch.Id]...)
	}
	return out
}

// MarkPrepared sets the prepared flag on known spells. Classes that know a
// fixed list (bard, sorcerer, ranger, warlock) always have their spells
// available, classes that prepare from a list or spellbook get the first
// N marked, where N is their prepared spell allowance.
func MarkPrepared(c *Character, rs *Ruleset) {
	GrantSubclassSpells(c, rs)
	sheet := Compute(c, rs)
	prepares := sheet.PreparedMax > 0
	used := 0
	for i := range c.Spells {
		if c.Spells[i].Level == 0 {
			c.Spells[i].Prepared = true
			continue
		}
		if !prepares {
			c.Spells[i].Prepared = true
			continue
		}
		// spells granted by a subclass or race are always prepared
		if c.Spells[i].Source != "" {
			c.Spells[i].Prepared = true
			continue
		}
		if used < sheet.PreparedMax {
			c.Spells[i].Prepared = true
			used++
		} else {
			c.Spells[i].Prepared = false
		}
	}
}

// PreparedCount evaluates the prepared spell formula for a class.
func PreparedCount(sc *Spellcasting, level, abilityMod int) int {
	formula := sc.PreparedFormula
	if formula == "" {
		if sc.Progression == "half" {
			formula = "mod+level/2"
		} else {
			formula = "mod+level"
		}
	}
	n := abilityMod
	switch strings.ReplaceAll(formula, " ", "") {
	case "mod+level":
		n += level
	case "mod+level/2":
		n += level / 2
	case "mod+level/3":
		n += level / 3
	}
	if n < 1 {
		n = 1
	}
	return n
}

// groupSpells collects the character's spells into the levels the sheet shows
// them in. The sheet is passed in because a spell entry carries what casting
// it does - the attack bonus, the save DC - which are the sheet's numbers.
func groupSpells(c *Character, rs *Ruleset, s *Sheet, slots []SpellSlotView) []SpellLevelView {
	abilityMod := 0
	if s != nil && s.CastingAbility != "" {
		abilityMod = s.AbilityMap[s.CastingAbility].Modifier
	}
	byLevel := map[int][]SpellEntry{}
	for _, ks := range c.Spells {
		sp := rs.Spell(ks.Id)
		e := SpellEntry{Id: ks.Id, Prepared: ks.Prepared, Source: ks.Source, Level: ks.Level}
		if sp != nil {
			e.Name = sp.Name
			e.Level = sp.Level
			e.School = sp.School
			e.CastingTime = sp.CastingTime
			e.Range = sp.Range
			e.Components = sp.Components
			e.Duration = sp.Duration
			e.Concentration = sp.Concentration
			e.Ritual = sp.Ritual
			e.Text = sp.Text
			e.HigherLevel = sp.HigherLevel
			e.Save = sp.Save
			if s != nil {
				e.Cast = ComputeCast(sp, s.Level, s.SpellAttack, s.SpellSaveDC, abilityMod)
			}
		} else {
			e.Name = ks.Name
			if e.Name == "" {
				e.Name = Titleize(ks.Id)
			}
		}
		byLevel[e.Level] = append(byLevel[e.Level], e)
	}
	out := []SpellLevelView{}
	for lvl := 0; lvl <= 9; lvl++ {
		list := byLevel[lvl]
		slotTotal, slotUsed := 0, 0
		for _, sl := range slots {
			if sl.Level == lvl {
				slotTotal, slotUsed = sl.Total, sl.Used
			}
		}
		if len(list) == 0 && slotTotal == 0 {
			continue
		}
		sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
		name := "Cantrips"
		if lvl > 0 {
			name = fmt.Sprintf("%s Level", Ordinal(lvl))
		}
		out = append(out, SpellLevelView{
			Level: lvl, Name: name, Slots: slotTotal, Used: slotUsed,
			Spells: list, HasSlots: slotTotal > 0,
		})
	}
	return out
}
