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

	s.Name = c.Name
	s.Player = c.Player
	s.Alignment = c.Alignment
	s.Level = level
	s.XP = c.XP
	s.NextLevelXP = XPForLevel(level + 1)
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

	// ---- abilities and saves ---------------------------------------------
	saveProf := map[string]bool{}
	if primaryClass != nil {
		for _, sv := range primaryClass.SavingThrows {
			saveProf[sv] = true
		}
	}
	s.AbilityMap = map[string]AbilityView{}
	for _, a := range AbilityOrder {
		score := 10
		if c.Abilities != nil {
			if v, ok := c.Abilities[a]; ok && v > 0 {
				score = v
			}
		}
		mod := AbilityMod(score)
		save := mod
		if saveProf[a] {
			save += s.Proficiency
		}
		av := AbilityView{
			Id: a, Name: AbilityNames[a], Short: AbilityShort[a],
			Score: score, Modifier: mod, Mod: Signed(mod),
			Save: save, SaveStr: Signed(save), SaveProf: saveProf[a],
		}
		s.Abilities = append(s.Abilities, av)
		s.AbilityMap[a] = av
		s.Saves = append(s.Saves, av)
	}
	mod := func(a string) int { return s.AbilityMap[a].Modifier }

	// ---- skills -----------------------------------------------------------
	skills := rs.Skills
	if len(skills) == 0 {
		skills = DefaultSkills
	}
	for _, sk := range skills {
		m := mod(sk.Ability)
		prof := c.HasSkill(sk.Id)
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

	// ---- proficiencies and languages -------------------------------------
	armorProf := []string{}
	weaponProf := []string{}
	toolProf := append([]string{}, c.Tools...)
	langs := append([]string{}, c.Languages...)
	collect := func(p Proficiencies) {
		armorProf = addUnique(armorProf, p.Armor...)
		weaponProf = addUnique(weaponProf, p.Weapons...)
		toolProf = addUnique(toolProf, p.Tools...)
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
		if sc := rs.Subclass(cl.Class, cl.Subclass); sc != nil {
			collect(sc.Proficiency)
		}
	}
	if bg != nil {
		collect(bg.Proficiencies)
	}
	s.ArmorProficiencies = prettyProficiencies(rs, armorProf, "armor")
	s.WeaponProficiencies = prettyProficiencies(rs, weaponProf, "weapon")
	s.ToolProficiencies = prettyProficiencies(rs, toolProf, "tool")
	s.Languages = langs

	// ---- features ---------------------------------------------------------
	s.Traits = collectTraits(rs, c, race, sub)
	s.Features = collectFeatures(rs, c, bg)

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
	s.AC, s.ACSource = computeAC(c, rs, s)
	s.Initiative = mod(DEX)
	s.InitiativeStr = Signed(s.Initiative)
	str := s.AbilityMap[STR].Score
	s.CarryCapacity = str * 15
	s.PushDragLift = str * 30

	// ---- equipment --------------------------------------------------------
	total := 0.0
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
		total += g.Weight * float64(g.Qty)
		equip = append(equip, g)
	}
	s.Equipment = equip
	s.Weight = total
	// Heavy armour you are too weak for slows you down.
	if armor := equippedArmor(c, rs); armor != nil && armor.StrengthReq > 0 && str < armor.StrengthReq {
		s.Speed -= 10
		if s.Speed < 0 {
			s.Speed = 0
		}
	}

	// ---- attacks ----------------------------------------------------------
	s.Attacks = computeAttacks(c, rs, s)

	// ---- spellcasting -----------------------------------------------------
	computeSpellcasting(c, rs, s)

	return s
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

func collectTraits(rs *Ruleset, c *Character, race, sub *Race) []Trait {
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
	return out
}

func collectFeatures(rs *Ruleset, c *Character, bg *Background) []Trait {
	out := []Trait{}
	for _, cl := range c.Classes {
		cls := rs.Class(cl.Class)
		if cls == nil {
			continue
		}
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
	}
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
		if it := rs.Item(gearKey(g)); it != nil && it.Kind == "armor" && it.ArmorType != "shield" {
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

func computeAC(c *Character, rs *Ruleset, s *Sheet) (int, string) {
	dex := s.AbilityMap[DEX].Modifier
	ac := 10 + dex
	source := "Unarmored"
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
		ac = base
		source = armor.Name
	} else {
		// unarmored defence styles, take the best one available
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
			if v > ac {
				ac = v
				source = cls.Name + " unarmored defense"
			}
		}
	}
	if shield := equippedShield(c, rs); shield != nil {
		bonus := shield.AC
		if bonus == 0 {
			bonus = 2
		}
		ac += bonus
		source += " + " + shield.Name
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
		}
	}
	return false
}

func computeAttacks(c *Character, rs *Ruleset, s *Sheet) []AttackView {
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
		bonus := abilityMod
		if prof {
			bonus += s.Proficiency
		}
		dmg := it.Damage
		if dmg != "" && abilityMod != 0 {
			dmg = fmt.Sprintf("%s%s", dmg, Signed(abilityMod))
		}
		notes := strings.ToLower(ability) + " based"
		if len(it.Properties) > 0 {
			notes += ", " + strings.Join(it.Properties, ", ")
		}
		if it.Versatile != "" {
			v := it.Versatile
			if abilityMod != 0 {
				v = fmt.Sprintf("%s%s", v, Signed(abilityMod))
			}
			notes = strings.TrimSpace(notes + " (" + v + " two handed)")
		}
		if !prof {
			notes = strings.TrimSpace(notes + ", not proficient")
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
			Name: it.Name, Bonus: Signed(bonus), Damage: dmg, Type: it.DamageType,
			Range: rng, Notes: strings.TrimSpace(notes), Proficent: prof,
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
			s.SpellLevels = groupSpells(c, rs, nil)
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
	for _, sp := range c.Spells {
		if sp.Prepared && sp.Level > 0 {
			s.SpellsPrepared++
		}
	}
	s.SpellLevels = groupSpells(c, rs, s.Slots)
}

// MarkPrepared sets the prepared flag on known spells. Classes that know a
// fixed list (bard, sorcerer, ranger, warlock) always have their spells
// available, classes that prepare from a list or spellbook get the first
// N marked, where N is their prepared spell allowance.
func MarkPrepared(c *Character, rs *Ruleset) {
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

func groupSpells(c *Character, rs *Ruleset, slots []SpellSlotView) []SpellLevelView {
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
