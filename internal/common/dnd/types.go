// Package dnd implements the orgs Dungeons & Dragons character module.
//
// It is shared between the server (REST endpoints + exporters) and the
// command line client, so it may not import anything from
// internal/app/orgs (that would create an import cycle).
//
// The package is split into:
//
//	types.go   - the data model (rulesets, characters, wire types)
//	ruleset.go - loading and merging of ruleset modules (the add on system)
//	rules.go   - the rules engine, turns a Character into a computed Sheet
//	builder.go - the interactive character creation state machine
//	orgfile.go - org mode character sheet reader / writer
package dnd

import "fmt"

// ----------------------------------------------------------------------------
// Abilities and skills
// ----------------------------------------------------------------------------

const (
	STR = "str"
	DEX = "dex"
	CON = "con"
	INT = "int"
	WIS = "wis"
	CHA = "cha"
)

// AbilityOrder is the canonical order abilities are displayed in on a sheet.
var AbilityOrder = []string{STR, DEX, CON, INT, WIS, CHA}

// AbilityNames maps an ability id onto its full name.
var AbilityNames = map[string]string{
	STR: "Strength",
	DEX: "Dexterity",
	CON: "Constitution",
	INT: "Intelligence",
	WIS: "Wisdom",
	CHA: "Charisma",
}

// AbilityShort maps an ability id onto the 3 letter form used in boxes.
var AbilityShort = map[string]string{
	STR: "STR", DEX: "DEX", CON: "CON", INT: "INT", WIS: "WIS", CHA: "CHA",
}

// ----------------------------------------------------------------------------
// Ruleset data model. Everything here is loaded from yaml modules so that
// new races, classes, spells, items and even whole rulesets can be added
// without touching any go code.
// ----------------------------------------------------------------------------

// Skill is a single skill definition, skills are keyed off an ability.
type Skill struct {
	Id      string `yaml:"id" json:"id"`
	Name    string `yaml:"name" json:"name"`
	Ability string `yaml:"ability" json:"ability"`
	Text    string `yaml:"text" json:"text"`
}

// Trait is a named chunk of rules text. Races, subraces, classes, subclasses,
// backgrounds and feats are all largely built out of these.
type Trait struct {
	Name  string `yaml:"name" json:"name"`
	Text  string `yaml:"text" json:"text"`
	Level int    `yaml:"level" json:"level"`
	// Source is filled in by the engine so a sheet can say where a trait came from.
	Source string `yaml:"source" json:"source"`
}

// Proficiencies is the set of things a race/class/background can make you good at.
type Proficiencies struct {
	Armor   []string `yaml:"armor" json:"armor"`
	Weapons []string `yaml:"weapons" json:"weapons"`
	Tools   []string `yaml:"tools" json:"tools"`
	Skills  []string `yaml:"skills" json:"skills"`
	Saves   []string `yaml:"saves" json:"saves"`
}

// Choice is a generic "pick N of these" definition. It powers class skill
// picks, fighting styles, expertise, extra languages, favoured enemies and
// anything a module author dreams up, without needing new code.
type Choice struct {
	Id     string   `yaml:"id" json:"id"`
	Name   string   `yaml:"name" json:"name"`
	Prompt string   `yaml:"prompt" json:"prompt"`
	Help   string   `yaml:"help" json:"help"`
	Level  int      `yaml:"level" json:"level"`
	Count  int      `yaml:"count" json:"count"`
	// CountByLevel, when set, overrides Count using the class level as index
	// (warlock invocations and sorcerer metamagic both grow with level).
	CountByLevel []int `yaml:"countByLevel" json:"countByLevel"`
	Kind   string   `yaml:"kind" json:"kind"` // skills|languages|tools|options|spells|expertise
	From   []string `yaml:"from" json:"from"`
	// SpellLevels restricts a spell choice to certain levels ([0] = cantrips).
	SpellLevels []int `yaml:"spellLevels" json:"spellLevels"`
	// Options are used when Kind is "options" (fighting styles, invocations...)
	Options []Trait `yaml:"options" json:"options"`
	// Grants records what picking an option gives you (proficiency ids)
	Grants string `yaml:"grants" json:"grants"`
}

// EquipmentOption is one line of a class/background starting equipment choice.
type EquipmentOption struct {
	Label string     `yaml:"label" json:"label"`
	Items []ItemRef  `yaml:"items" json:"items"`
	Gold  string     `yaml:"gold" json:"gold"`
	Any   []string   `yaml:"any" json:"any"` // any weapon of these categories
	Meta  [][]string `yaml:"-" json:"-"`
}

// ItemRef is a quantity of an item id (or a free text item for homebrew).
type ItemRef struct {
	Id   string `yaml:"id" json:"id"`
	Name string `yaml:"name" json:"name"`
	Qty  int    `yaml:"qty" json:"qty"`
}

// EquipmentChoice is a single "choose one of" starting equipment group.
type EquipmentChoice struct {
	Id      string            `yaml:"id" json:"id"`
	Prompt  string            `yaml:"prompt" json:"prompt"`
	Options []EquipmentOption `yaml:"options" json:"options"`
}

// Race is a playable race. Subraces inherit everything the parent race has.
type Race struct {
	Id             string         `yaml:"id" json:"id"`
	Name           string         `yaml:"name" json:"name"`
	Source         string         `yaml:"source" json:"source"`
	Summary        string         `yaml:"summary" json:"summary"`
	Text           string         `yaml:"text" json:"text"`
	Size           string         `yaml:"size" json:"size"`
	Speed          int            `yaml:"speed" json:"speed"`
	AbilityBonuses map[string]int `yaml:"abilityBonuses" json:"abilityBonuses"`
	// AbilityChoice lets a race hand out "+1 to two abilities of your choice"
	AbilityChoice  *AbilityChoice `yaml:"abilityChoice" json:"abilityChoice"`
	Age            string         `yaml:"age" json:"age"`
	Alignment      string         `yaml:"alignment" json:"alignment"`
	Languages      []string       `yaml:"languages" json:"languages"`
	ExtraLanguages int            `yaml:"extraLanguages" json:"extraLanguages"`
	Darkvision     int            `yaml:"darkvision" json:"darkvision"`
	HpPerLevel     int            `yaml:"hpPerLevel" json:"hpPerLevel"`
	Traits         []Trait        `yaml:"traits" json:"traits"`
	Proficiencies  Proficiencies  `yaml:"proficiencies" json:"proficiencies"`
	Choices        []Choice       `yaml:"choices" json:"choices"`
	Spells         []RaceSpell    `yaml:"spells" json:"spells"`
	Names          []string       `yaml:"names" json:"names"`
	Subraces       []Race         `yaml:"subraces" json:"subraces"`
	// Parent is filled in by the loader for subraces.
	Parent string `yaml:"-" json:"parent"`
}

// AbilityChoice models "increase two different scores of your choice by 1".
type AbilityChoice struct {
	Count  int      `yaml:"count" json:"count"`
	Amount int      `yaml:"amount" json:"amount"`
	From   []string `yaml:"from" json:"from"`
}

// RaceSpell is an innate spell granted by a race at a given level.
type RaceSpell struct {
	Id      string `yaml:"id" json:"id"`
	Level   int    `yaml:"level" json:"level"`
	Ability string `yaml:"ability" json:"ability"`
	Notes   string `yaml:"notes" json:"notes"`
}

// Spellcasting describes how a class casts spells.
type Spellcasting struct {
	// Progression: full, half, third, pact or "" for none
	Progression string `yaml:"progression" json:"progression"`
	Ability     string `yaml:"ability" json:"ability"`
	// Prepares means the class prepares from the whole list (cleric, druid,
	// paladin, wizard) rather than having a fixed known list.
	Prepares bool `yaml:"prepares" json:"prepares"`
	// PreparedFrom is "list" (cleric/druid/paladin) or "spellbook" (wizard)
	PreparedFrom string `yaml:"preparedFrom" json:"preparedFrom"`
	Ritual       bool   `yaml:"ritual" json:"ritual"`
	Focus        string `yaml:"focus" json:"focus"`
	StartLevel   int    `yaml:"startLevel" json:"startLevel"`
	// PreparedFormula is "mod+level" or "mod+level/2", used when Prepares is set.
	PreparedFormula string `yaml:"preparedFormula" json:"preparedFormula"`
	// CantripsKnown / SpellsKnown are indexed by class level (1 based, index 0 unused)
	CantripsKnown []int `yaml:"cantripsKnown" json:"cantripsKnown"`
	SpellsKnown   []int `yaml:"spellsKnown" json:"spellsKnown"`
	// SpellList is the id of the spell list to draw from, defaults to the class id
	SpellList string `yaml:"spellList" json:"spellList"`
	Notes     string `yaml:"notes" json:"notes"`
}

// Subclass is an archetype/domain/circle/oath etc.
type Subclass struct {
	Id           string        `yaml:"id" json:"id"`
	Name         string        `yaml:"name" json:"name"`
	Source       string        `yaml:"source" json:"source"`
	Summary      string        `yaml:"summary" json:"summary"`
	Text         string        `yaml:"text" json:"text"`
	Features     []Trait       `yaml:"features" json:"features"`
	Choices      []Choice      `yaml:"choices" json:"choices"`
	Spells       []SubSpell    `yaml:"spells" json:"spells"`
	Proficiency  Proficiencies `yaml:"proficiencies" json:"proficiencies"`
	Spellcasting *Spellcasting `yaml:"spellcasting" json:"spellcasting"`
}

// SubSpell is an always prepared subclass spell (domain spells, oath spells...)
type SubSpell struct {
	Id    string `yaml:"id" json:"id"`
	Level int    `yaml:"level" json:"level"`
}

// Class is a playable class.
type Class struct {
	Id             string            `yaml:"id" json:"id"`
	Name           string            `yaml:"name" json:"name"`
	Source         string            `yaml:"source" json:"source"`
	Summary        string            `yaml:"summary" json:"summary"`
	Text           string            `yaml:"text" json:"text"`
	HitDie         int               `yaml:"hitDie" json:"hitDie"`
	PrimaryAbility []string          `yaml:"primaryAbility" json:"primaryAbility"`
	SavingThrows   []string          `yaml:"savingThrows" json:"savingThrows"`
	Proficiencies  Proficiencies     `yaml:"proficiencies" json:"proficiencies"`
	SkillCount     int               `yaml:"skillCount" json:"skillCount"`
	SkillsFrom     []string          `yaml:"skillsFrom" json:"skillsFrom"`
	Equipment      []EquipmentChoice `yaml:"equipment" json:"equipment"`
	FixedEquipment []ItemRef         `yaml:"fixedEquipment" json:"fixedEquipment"`
	Features       []Trait           `yaml:"features" json:"features"`
	Choices        []Choice          `yaml:"choices" json:"choices"`
	SubclassLevel  int               `yaml:"subclassLevel" json:"subclassLevel"`
	SubclassLabel  string            `yaml:"subclassLabel" json:"subclassLabel"`
	Subclasses     []Subclass        `yaml:"subclasses" json:"subclasses"`
	Spellcasting   *Spellcasting     `yaml:"spellcasting" json:"spellcasting"`
	// ASILevels are the levels where you gain an ability score improvement.
	ASILevels []int `yaml:"asiLevels" json:"asiLevels"`
	// MulticlassReq is the ability minimum required to multiclass into this class.
	MulticlassReq map[string]int `yaml:"multiclassReq" json:"multiclassReq"`
	// Unarmored describes an unarmored defence style AC (barbarian/monk).
	UnarmoredAC string `yaml:"unarmoredAc" json:"unarmoredAc"`
}

// Background is a character background.
type Background struct {
	Id            string        `yaml:"id" json:"id"`
	Name          string        `yaml:"name" json:"name"`
	Source        string        `yaml:"source" json:"source"`
	Summary       string        `yaml:"summary" json:"summary"`
	Text          string        `yaml:"text" json:"text"`
	Proficiencies Proficiencies `yaml:"proficiencies" json:"proficiencies"`
	Languages     int           `yaml:"languages" json:"languages"`
	Equipment     []ItemRef     `yaml:"equipment" json:"equipment"`
	Gold          int           `yaml:"gold" json:"gold"`
	Feature       Trait         `yaml:"feature" json:"feature"`
	Choices       []Choice      `yaml:"choices" json:"choices"`
	Traits        []string      `yaml:"traits" json:"traits"`
	Ideals        []string      `yaml:"ideals" json:"ideals"`
	Bonds         []string      `yaml:"bonds" json:"bonds"`
	Flaws         []string      `yaml:"flaws" json:"flaws"`
}

// Item covers weapons, armor, gear and packs. A single struct keeps the yaml
// simple and lets homebrew modules add anything they like.
type Item struct {
	Id       string `yaml:"id" json:"id"`
	Name     string `yaml:"name" json:"name"`
	Kind     string `yaml:"kind" json:"kind"` // weapon|armor|shield|gear|pack|tool
	Category string `yaml:"category" json:"category"`
	Cost     string `yaml:"cost" json:"cost"`
	Weight   float64 `yaml:"weight" json:"weight"`
	Text     string `yaml:"text" json:"text"`

	// Weapon fields
	Damage     string   `yaml:"damage" json:"damage"`
	DamageType string   `yaml:"damageType" json:"damageType"`
	Versatile  string   `yaml:"versatile" json:"versatile"`
	Properties []string `yaml:"properties" json:"properties"`
	Range      string   `yaml:"range" json:"range"`

	// Armor fields
	AC                  int    `yaml:"ac" json:"ac"`
	ArmorType           string `yaml:"armorType" json:"armorType"` // light|medium|heavy|shield
	DexMax              int    `yaml:"dexMax" json:"dexMax"`
	NoDex               bool   `yaml:"noDex" json:"noDex"`
	StrengthReq         int    `yaml:"strengthReq" json:"strengthReq"`
	StealthDisadvantage bool   `yaml:"stealthDisadvantage" json:"stealthDisadvantage"`

	// Pack contents
	Contents []ItemRef `yaml:"contents" json:"contents"`
}

// IsWeapon is true for anything that can be attacked with.
func (i *Item) IsWeapon() bool { return i.Kind == "weapon" }

// HasProperty reports whether a weapon has the named property (finesse etc).
func (i *Item) HasProperty(p string) bool {
	for _, v := range i.Properties {
		if v == p {
			return true
		}
	}
	return false
}

// Spell is a single spell definition.
type Spell struct {
	Id            string   `yaml:"id" json:"id"`
	Name          string   `yaml:"name" json:"name"`
	Source        string   `yaml:"source" json:"source"`
	Level         int      `yaml:"level" json:"level"`
	School        string   `yaml:"school" json:"school"`
	CastingTime   string   `yaml:"castingTime" json:"castingTime"`
	Range         string   `yaml:"range" json:"range"`
	Components    string   `yaml:"components" json:"components"`
	Materials     string   `yaml:"materials" json:"materials"`
	Duration      string   `yaml:"duration" json:"duration"`
	Concentration bool     `yaml:"concentration" json:"concentration"`
	Ritual        bool     `yaml:"ritual" json:"ritual"`
	Classes       []string `yaml:"classes" json:"classes"`
	Text          string   `yaml:"text" json:"text"`
	HigherLevel   string   `yaml:"higherLevel" json:"higherLevel"`
	// Attack marks a spell that uses a spell attack roll, Save names the
	// ability a target saves with. Both feed the attacks table on the sheet.
	Attack bool   `yaml:"attack" json:"attack"`
	Save   string `yaml:"save" json:"save"`
	Damage string `yaml:"damage" json:"damage"`
}

// LevelString renders "Cantrip" or "3rd level evocation".
func (s *Spell) LevelString() string {
	if s.Level == 0 {
		return s.School + " cantrip"
	}
	return fmt.Sprintf("%s level %s", Ordinal(s.Level), s.School)
}

// Feat is an optional feat definition.
type Feat struct {
	Id             string         `yaml:"id" json:"id"`
	Name           string         `yaml:"name" json:"name"`
	Source         string         `yaml:"source" json:"source"`
	Text           string         `yaml:"text" json:"text"`
	Prerequisite   string         `yaml:"prerequisite" json:"prerequisite"`
	AbilityBonuses map[string]int `yaml:"abilityBonuses" json:"abilityBonuses"`
	AbilityChoice  *AbilityChoice `yaml:"abilityChoice" json:"abilityChoice"`
	Proficiencies  Proficiencies  `yaml:"proficiencies" json:"proficiencies"`
}

// Ruleset is a complete (possibly merged) set of rules data.
type Ruleset struct {
	Id          string       `yaml:"id" json:"id"`
	Name        string       `yaml:"name" json:"name"`
	Version     string       `yaml:"version" json:"version"`
	Description string       `yaml:"description" json:"description"`
	Extends     string       `yaml:"extends" json:"extends"`
	Skills      []Skill      `yaml:"skills" json:"skills"`
	Races       []Race       `yaml:"races" json:"races"`
	Classes     []Class      `yaml:"classes" json:"classes"`
	Backgrounds []Background `yaml:"backgrounds" json:"backgrounds"`
	Items       []Item       `yaml:"items" json:"items"`
	Spells      []Spell      `yaml:"spells" json:"spells"`
	Feats       []Feat       `yaml:"feats" json:"feats"`
	Languages   []string     `yaml:"languages" json:"languages"`
	Alignments  []string     `yaml:"alignments" json:"alignments"`
	// SlotTables lets a module replace the built in spell slot progressions.
	SlotTables map[string][][]int `yaml:"slotTables" json:"slotTables"`
	// Modules records which yaml files contributed to this ruleset.
	Modules []string `yaml:"-" json:"modules"`

	// lookup indexes, built by Index()
	skillIdx map[string]*Skill
	raceIdx  map[string]*Race
	classIdx map[string]*Class
	bgIdx    map[string]*Background
	itemIdx  map[string]*Item
	spellIdx map[string]*Spell
	featIdx  map[string]*Feat
}

// ----------------------------------------------------------------------------
// Character - what actually gets stored in the org file
// ----------------------------------------------------------------------------

// ClassLevel is one class a character has levels in.
type ClassLevel struct {
	Class    string `yaml:"class" json:"class"`
	Subclass string `yaml:"subclass" json:"subclass"`
	Level    int    `yaml:"level" json:"level"`
}

// Gear is one line of the equipment table.
type Gear struct {
	Id       string  `yaml:"id" json:"id"`
	Name     string  `yaml:"name" json:"name"`
	Qty      int     `yaml:"qty" json:"qty"`
	Equipped bool    `yaml:"equipped" json:"equipped"`
	Weight   float64 `yaml:"weight" json:"weight"`
	Notes    string  `yaml:"notes" json:"notes"`
}

// KnownSpell is a spell the character knows, and whether it is prepared.
type KnownSpell struct {
	Id       string `yaml:"id" json:"id"`
	Name     string `yaml:"name" json:"name"`
	Level    int    `yaml:"level" json:"level"`
	Prepared bool   `yaml:"prepared" json:"prepared"`
	Source   string `yaml:"source" json:"source"`
	Notes    string `yaml:"notes" json:"notes"`
}

// Money is the coin purse.
type Money struct {
	CP int `yaml:"cp" json:"cp"`
	SP int `yaml:"sp" json:"sp"`
	EP int `yaml:"ep" json:"ep"`
	GP int `yaml:"gp" json:"gp"`
	PP int `yaml:"pp" json:"pp"`
}

// Character is the persistent state of a player character. This is exactly
// what round trips through the org file.
type Character struct {
	Name    string `yaml:"name" json:"name"`
	Player  string `yaml:"player" json:"player"`
	Ruleset string `yaml:"ruleset" json:"ruleset"`

	Race       string       `yaml:"race" json:"race"`
	Subrace    string       `yaml:"subrace" json:"subrace"`
	Classes    []ClassLevel `yaml:"classes" json:"classes"`
	Background string       `yaml:"background" json:"background"`
	Alignment  string       `yaml:"alignment" json:"alignment"`
	XP         int          `yaml:"xp" json:"xp"`

	// Abilities are the final scores including racial bonuses.
	Abilities map[string]int `yaml:"abilities" json:"abilities"`

	Skills    []string `yaml:"skills" json:"skills"`
	Expertise []string `yaml:"expertise" json:"expertise"`
	Languages []string `yaml:"languages" json:"languages"`
	Tools     []string `yaml:"tools" json:"tools"`
	Feats     []string `yaml:"feats" json:"feats"`

	// Choices records generic class/race choice answers keyed by choice id.
	Choices map[string][]string `yaml:"choices" json:"choices"`

	HPMax       int    `yaml:"hpMax" json:"hpMax"`
	HPCurrent   int    `yaml:"hpCurrent" json:"hpCurrent"`
	HPTemp      int    `yaml:"hpTemp" json:"hpTemp"`
	HitDiceUsed int    `yaml:"hitDiceUsed" json:"hitDiceUsed"`
	DeathSaves  string `yaml:"deathSaves" json:"deathSaves"`
	Inspiration bool   `yaml:"inspiration" json:"inspiration"`
	SlotsUsed   []int  `yaml:"slotsUsed" json:"slotsUsed"`

	Equipment []Gear       `yaml:"equipment" json:"equipment"`
	Spells    []KnownSpell `yaml:"spells" json:"spells"`
	Money     Money        `yaml:"money" json:"money"`

	Personality string `yaml:"personality" json:"personality"`
	Ideals      string `yaml:"ideals" json:"ideals"`
	Bonds       string `yaml:"bonds" json:"bonds"`
	Flaws       string `yaml:"flaws" json:"flaws"`

	Age    string `yaml:"age" json:"age"`
	Height string `yaml:"height" json:"height"`
	Weight string `yaml:"weight" json:"weight"`
	Eyes   string `yaml:"eyes" json:"eyes"`
	Skin   string `yaml:"skin" json:"skin"`
	Hair   string `yaml:"hair" json:"hair"`

	Appearance string `yaml:"appearance" json:"appearance"`
	Backstory  string `yaml:"backstory" json:"backstory"`
	Allies     string `yaml:"allies" json:"allies"`
	Treasure   string `yaml:"treasure" json:"treasure"`
	Notes      string `yaml:"notes" json:"notes"`

	// Filename is where the sheet lives, it is not written into the file.
	Filename string `yaml:"-" json:"filename"`
}

// TotalLevel is the sum of all class levels.
func (c *Character) TotalLevel() int {
	t := 0
	for _, cl := range c.Classes {
		t += cl.Level
	}
	if t <= 0 {
		return 1
	}
	return t
}

// PrimaryClass returns the first (highest priority) class entry.
func (c *Character) PrimaryClass() ClassLevel {
	if len(c.Classes) > 0 {
		return c.Classes[0]
	}
	return ClassLevel{}
}

// HasSkill reports skill proficiency.
func (c *Character) HasSkill(id string) bool { return containsStr(c.Skills, id) }

// HasExpertise reports skill expertise.
func (c *Character) HasExpertise(id string) bool { return containsStr(c.Expertise, id) }

// ----------------------------------------------------------------------------
// Computed sheet - everything derived from Character + Ruleset. This is what
// the html/latex templates and the terminal renderer consume.
// ----------------------------------------------------------------------------

// AbilityView is one ability score box on the sheet.
type AbilityView struct {
	Id       string `json:"id"`
	Name     string `json:"name"`
	Short    string `json:"short"`
	Score    int    `json:"score"`
	Modifier int    `json:"modifier"`
	Mod      string `json:"mod"`
	Save     int    `json:"save"`
	SaveStr  string `json:"saveStr"`
	SaveProf bool   `json:"saveProf"`
}

// SkillView is one row of the skills box.
type SkillView struct {
	Id        string `json:"id"`
	Name      string `json:"name"`
	Ability   string `json:"ability"`
	Short     string `json:"short"`
	Modifier  int    `json:"modifier"`
	Mod       string `json:"mod"`
	Proficent bool   `json:"proficient"`
	Expertise bool   `json:"expertise"`
	Passive   int    `json:"passive"`
}

// AttackView is one row of the attacks table.
type AttackView struct {
	Name      string `json:"name"`
	Bonus     string `json:"bonus"`
	Damage    string `json:"damage"`
	Type      string `json:"type"`
	Range     string `json:"range"`
	Notes     string `json:"notes"`
	Proficent bool   `json:"proficient"`
}

// SpellSlotView is the slot line for one spell level.
type SpellSlotView struct {
	Level int    `json:"level"`
	Label string `json:"label"`
	Total int `json:"total"`
	Used  int `json:"used"`
	Left  int `json:"left"`
	// Pips is 1..Total, so a template can draw one box per slot.
	Pips []int `json:"pips"`
}

// SpellLevelView groups the known spells of one level for display.
type SpellLevelView struct {
	Level    int          `json:"level"`
	Name     string       `json:"name"`
	Slots    int          `json:"slots"`
	Used     int          `json:"used"`
	Spells   []SpellEntry `json:"spells"`
	HasSlots bool         `json:"hasSlots"`
}

// SpellEntry is one spell as displayed on the spell page.
type SpellEntry struct {
	Id            string `json:"id"`
	Name          string `json:"name"`
	Level         int    `json:"level"`
	School        string `json:"school"`
	CastingTime   string `json:"castingTime"`
	Range         string `json:"range"`
	Components    string `json:"components"`
	Duration      string `json:"duration"`
	Concentration bool   `json:"concentration"`
	Ritual        bool   `json:"ritual"`
	Prepared      bool   `json:"prepared"`
	Text          string `json:"text"`
	HigherLevel   string `json:"higherLevel"`
	Source        string `json:"source"`
	Save          string `json:"save"`
}

// Sheet is the fully computed character sheet.
type Sheet struct {
	Character *Character `json:"character"`

	Name        string `json:"name"`
	Player      string `json:"player"`
	RaceName    string `json:"raceName"`
	ClassLine   string `json:"classLine"`
	ClassName   string `json:"className"`
	SubclassName string `json:"subclassName"`
	Background  string `json:"background"`
	Alignment   string `json:"alignment"`
	Level       int    `json:"level"`
	XP          int    `json:"xp"`
	NextLevelXP int    `json:"nextLevelXp"`
	RulesetName string `json:"rulesetName"`

	Abilities  []AbilityView `json:"abilities"`
	AbilityMap map[string]AbilityView `json:"abilityMap"`
	Proficiency int          `json:"proficiency"`
	ProficiencyStr string    `json:"proficiencyStr"`

	Saves     []AbilityView `json:"saves"`
	Skills    []SkillView   `json:"skills"`
	PassivePerception int   `json:"passivePerception"`
	PassiveInsight    int   `json:"passiveInsight"`
	PassiveInvestigation int `json:"passiveInvestigation"`

	AC           int    `json:"ac"`
	ACSource     string `json:"acSource"`
	Initiative   int    `json:"initiative"`
	InitiativeStr string `json:"initiativeStr"`
	Speed        int    `json:"speed"`
	Size         string `json:"size"`
	Darkvision   int    `json:"darkvision"`

	HPMax       int    `json:"hpMax"`
	HPCurrent   int    `json:"hpCurrent"`
	HPTemp      int    `json:"hpTemp"`
	HPPercent   int    `json:"hpPercent"`
	HitDice     string `json:"hitDice"`
	HitDiceUsed int    `json:"hitDiceUsed"`
	DeathSaves  string `json:"deathSaves"`
	Inspiration bool   `json:"inspiration"`

	Attacks   []AttackView `json:"attacks"`
	Equipment []Gear       `json:"equipment"`
	Money     Money        `json:"money"`
	Weight    float64      `json:"weight"`
	CarryCapacity int      `json:"carryCapacity"`
	PushDragLift  int      `json:"pushDragLift"`

	ArmorProficiencies  []string `json:"armorProficiencies"`
	WeaponProficiencies []string `json:"weaponProficiencies"`
	ToolProficiencies   []string `json:"toolProficiencies"`
	Languages           []string `json:"languages"`

	Features []Trait `json:"features"`
	Traits   []Trait `json:"traits"`

	IsCaster        bool             `json:"isCaster"`
	CastingAbility  string           `json:"castingAbility"`
	CastingAbilityName string        `json:"castingAbilityName"`
	SpellSaveDC     int              `json:"spellSaveDc"`
	SpellAttack     int              `json:"spellAttack"`
	SpellAttackStr  string           `json:"spellAttackStr"`
	CantripsKnown   int              `json:"cantripsKnown"`
	SpellsKnown     int              `json:"spellsKnown"`
	SpellsPrepared  int              `json:"spellsPrepared"`
	PreparedMax     int              `json:"preparedMax"`
	Slots           []SpellSlotView  `json:"slots"`
	SpellLevels     []SpellLevelView `json:"spellLevels"`
	SpellNotes      string           `json:"spellNotes"`
	PactMagic       bool             `json:"pactMagic"`

	Personality string `json:"personality"`
	Ideals      string `json:"ideals"`
	Bonds       string `json:"bonds"`
	Flaws       string `json:"flaws"`

	Age    string `json:"age"`
	Height string `json:"height"`
	Weight_ string `json:"weightStr"`
	Eyes   string `json:"eyes"`
	Skin   string `json:"skin"`
	Hair   string `json:"hair"`

	Appearance string `json:"appearance"`
	Backstory  string `json:"backstory"`
	Allies     string `json:"allies"`
	Treasure   string `json:"treasure"`
	Notes      string `json:"notes"`

	// Warnings are rules problems detected while computing (bad ids etc).
	Warnings []string `json:"warnings"`
}

// ----------------------------------------------------------------------------
// Wire types for the interactive builder
// ----------------------------------------------------------------------------

// Option is a single valid answer offered to the player, along with enough
// information for a client to explain the choice the way D&D Beyond does.
type Option struct {
	Id          string            `json:"id"`
	Name        string            `json:"name"`
	Summary     string            `json:"summary"`
	Detail      string            `json:"detail"`
	Tags        []string          `json:"tags"`
	Meta        map[string]string `json:"meta"`
	Recommended bool              `json:"recommended"`
	Disabled    bool              `json:"disabled"`
	Reason      string            `json:"reason"`
}

// Prompt is one question in the character creation flow.
type Prompt struct {
	Session  string   `json:"session"`
	Step     string   `json:"step"`
	Title    string   `json:"title"`
	Question string   `json:"question"`
	Help     string   `json:"help"`
	// Kind is one of: select, multiselect, text, longtext, number, abilities, confirm, done
	Kind        string   `json:"kind"`
	Options     []Option `json:"options"`
	Min         int      `json:"min"`
	Max         int      `json:"max"`
	Default     string   `json:"default"`
	Defaults    []string `json:"defaults"`
	AllowCustom bool     `json:"allowCustom"`
	AllowRandom bool     `json:"allowRandom"`
	AllowSkip   bool     `json:"allowSkip"`
	// Fields is used by the "abilities" kind, one entry per ability to fill in.
	Fields []Field `json:"fields"`
	// Pool are the values available to assign for the abilities step.
	Pool     []int    `json:"pool"`
	Advice   []string `json:"advice"`
	Progress Progress `json:"progress"`
	Done     bool     `json:"done"`
	Error    string   `json:"error"`
	// Summary is a running description of the character so far.
	Summary  string     `json:"summary"`
	Sheet    *Sheet     `json:"sheet,omitempty"`
	Org      string     `json:"org,omitempty"`
	Filename string     `json:"filename,omitempty"`
}

// Field is one input of a multi field prompt (ability score assignment).
type Field struct {
	Id      string `json:"id"`
	Name    string `json:"name"`
	Hint    string `json:"hint"`
	Value   int    `json:"value"`
	Min     int    `json:"min"`
	Max     int    `json:"max"`
	Bonus   int    `json:"bonus"`
	Cost    int    `json:"cost"`
}

// Progress tells the client how far through creation we are.
type Progress struct {
	Step      int      `json:"step"`
	Total     int      `json:"total"`
	Completed []string `json:"completed"`
	Remaining []string `json:"remaining"`
}

// Answer is the client reply to a Prompt.
type Answer struct {
	Session string         `json:"session"`
	Step    string         `json:"step"`
	Values  []string       `json:"values"`
	Text    string         `json:"text"`
	Numbers map[string]int `json:"numbers"`
	Random  bool           `json:"random"`
	Back    bool           `json:"back"`
	Skip    bool           `json:"skip"`
	// Pool carries dice results back to the server so that replaying an
	// answer (which happens whenever you step backwards) does not reroll.
	Pool []int `json:"pool"`
}

// NewSessionRequest starts a character build.
type NewSessionRequest struct {
	Ruleset  string `json:"ruleset"`
	Name     string `json:"name"`
	Player   string `json:"player"`
	Level    int    `json:"level"`
	Random   bool   `json:"random"`
	Filename string `json:"filename"`
}

// SaveRequest writes a finished character out to an org file.
type SaveRequest struct {
	Session   string     `json:"session"`
	Filename  string     `json:"filename"`
	Character *Character `json:"character"`
	Overwrite bool       `json:"overwrite"`
}

// SaveResponse is the result of a save.
type SaveResponse struct {
	Ok       bool   `json:"ok"`
	Filename string `json:"filename"`
	Msg      string `json:"msg"`
	Org      string `json:"org"`
}

// CatalogRequest asks for part of a ruleset (used to browse choices).
type CatalogRequest struct {
	Ruleset string `json:"ruleset"`
	Kind    string `json:"kind"`
	Filter  string `json:"filter"`
	Id      string `json:"id"`
}

// CatalogResponse is a list of options for browsing.
type CatalogResponse struct {
	Ruleset string   `json:"ruleset"`
	Kind    string   `json:"kind"`
	Options []Option `json:"options"`
	Detail  string   `json:"detail"`
}

// RulesetInfo is the summary of a loaded ruleset.
type RulesetInfo struct {
	Id          string   `json:"id"`
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Description string   `json:"description"`
	Extends     string   `json:"extends"`
	Modules     []string `json:"modules"`
	Races       int      `json:"races"`
	Classes     int      `json:"classes"`
	Backgrounds int      `json:"backgrounds"`
	Spells      int      `json:"spells"`
	Items       int      `json:"items"`
	Feats       int      `json:"feats"`
}
