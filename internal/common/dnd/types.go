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

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

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
	// Bonuses are the numbers this trait actually moves on the sheet. Most
	// traits leave it empty and are pure prose.
	Bonuses Bonuses `yaml:"bonuses" json:"bonuses"`
}

// Bonuses are the mechanical adjustments a trait, feature or feat hands out.
//
// Every field is a formula in the same style as a class's unarmoredAc: terms
// joined with "+", where a term is a number, an ability id whose modifier is
// added, or the proficiency bonus as "prof", "prof/2" (rounded down) or
// "prof/2up" (rounded up). This keeps the interesting cases in data rather
// than in go, so a homebrew module can grant a bonus without new code:
//
//	bonuses:
//	  initiative: "5"                       # Alert
//	  checks: "prof/2"                      # Jack of All Trades
//	  checks: "prof/2up"                    # Remarkable Athlete...
//	  abilities: ["str", "dex", "con"]      # ...but only these three
//	  saves: "cha"                          # Aura of Protection
//	  minimum: 1                            # "with a minimum bonus of +1"
type Bonuses struct {
	// Checks is added to ability checks, and to skill checks that do not
	// already include the proficiency bonus, which is how both Jack of All
	// Trades and Remarkable Athlete are worded. Initiative is a dexterity
	// check, so it picks this up too.
	Checks string `yaml:"checks" json:"checks"`
	// Initiative is added to initiative on top of whatever Checks grants it.
	Initiative string `yaml:"initiative" json:"initiative"`
	// Saves is added to every saving throw, death saving throws included.
	Saves string `yaml:"saves" json:"saves"`
	// DeathSave is added to death saving throws only.
	DeathSave string `yaml:"deathSave" json:"deathSave"`
	// Abilities restricts Checks to checks based on these abilities. Empty,
	// the usual case, means all six.
	Abilities []string `yaml:"abilities" json:"abilities"`
	// Minimum floors each evaluated bonus. It exists for the "minimum bonus
	// of +1" wording on a paladin's aura.
	Minimum int `yaml:"minimum" json:"minimum"`
}

// Proficiencies is the set of things a race/class/background can make you good at.
type Proficiencies struct {
	Armor   []string `yaml:"armor" json:"armor"`
	Weapons []string `yaml:"weapons" json:"weapons"`
	Tools   []string `yaml:"tools" json:"tools"`
	Skills  []string `yaml:"skills" json:"skills"`
	Saves   []string `yaml:"saves" json:"saves"`
	// Level is the class level at which this block is granted. Zero, the
	// usual case, means it arrives as soon as its source applies - which for
	// a race, background or feat is always, and for a subclass is the level
	// the subclass itself is chosen at. It is only meaningful on a source
	// that is read through a ProficiencySet.
	Level int `yaml:"level" json:"level"`
}

// ProficiencySet is what one source grants. It accepts either a single block,
// which is what almost everything wants:
//
//	proficiencies:
//	  skills: ["insight", "medicine"]
//	  tools: ["herbalism-kit"]
//
// or a list of blocks, when some of them arrive later than the source itself.
// A monk of the Way of Mercy gets its skills at 3rd level with the tradition,
// but a Gloom Stalker's Wisdom save comes at 7th:
//
//	proficiencies:
//	  - skills: ["deception"]
//	  - level: 7
//	    saves: ["wis"]
//
// The two forms are interchangeable, so existing modules that write a single
// block keep working untouched.
type ProficiencySet []Proficiencies

// UnmarshalYAML accepts either shape described above. A null is an empty set.
func (p *ProficiencySet) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.SequenceNode:
		var many []Proficiencies
		if err := value.Decode(&many); err != nil {
			return err
		}
		*p = ProficiencySet(many)
		return nil
	case yaml.MappingNode:
		var one Proficiencies
		if err := value.Decode(&one); err != nil {
			return err
		}
		*p = ProficiencySet{one}
		return nil
	case yaml.ScalarNode:
		if value.Tag == "!!null" {
			*p = nil
			return nil
		}
	}
	return fmt.Errorf("line %d: proficiencies must be a block or a list of blocks",
		value.Line)
}

// At returns the blocks a character of the given class level has reached.
func (p ProficiencySet) At(level int) []Proficiencies {
	out := make([]Proficiencies, 0, len(p))
	for _, one := range p {
		if one.Level <= level {
			out = append(out, one)
		}
	}
	return out
}

// Choice is a generic "pick N of these" definition. It powers class skill
// picks, fighting styles, expertise, extra languages, favoured enemies and
// anything a module author dreams up, without needing new code.
type Choice struct {
	Id     string `yaml:"id" json:"id"`
	Name   string `yaml:"name" json:"name"`
	Prompt string `yaml:"prompt" json:"prompt"`
	Help   string `yaml:"help" json:"help"`
	Level  int    `yaml:"level" json:"level"`
	Count  int    `yaml:"count" json:"count"`
	// CountByLevel, when set, overrides Count using the class level as index
	// (warlock invocations and sorcerer metamagic both grow with level).
	CountByLevel []int    `yaml:"countByLevel" json:"countByLevel"`
	Kind         string   `yaml:"kind" json:"kind"` // skills|languages|tools|options|spells|expertise
	From         []string `yaml:"from" json:"from"`
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
	// Appearance replaces the built in colour and size table for this race.
	// A subrace extends its parent rather than replacing it.
	Appearance *Appearance `yaml:"appearance" json:"appearance,omitempty"`
	Subraces   []Race      `yaml:"subraces" json:"subraces"`
	// Parent is filled in by the loader for subraces.
	Parent string `yaml:"-" json:"parent"`
}

// AbilityChoice models "increase two different scores of your choice by 1".
type AbilityChoice struct {
	Count  int      `yaml:"count" json:"count"`
	Amount int      `yaml:"amount" json:"amount"`
	From   []string `yaml:"from" json:"from"`
	// GrantsSave makes the chosen ability a saving throw proficiency as well,
	// which is what the Resilient feat does.
	GrantsSave bool `yaml:"grantsSave" json:"grantsSave"`
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
	Id       string   `yaml:"id" json:"id"`
	Name     string   `yaml:"name" json:"name"`
	Source   string   `yaml:"source" json:"source"`
	Summary  string   `yaml:"summary" json:"summary"`
	Text     string   `yaml:"text" json:"text"`
	Features []Trait  `yaml:"features" json:"features"`
	Choices  []Choice `yaml:"choices" json:"choices"`
	// Spells are granted outright and are always prepared - cleric domain
	// spells, paladin oath spells, druid circle spells.
	Spells []SubSpell `yaml:"spells" json:"spells"`
	// ExpandedSpells are added to the class's spell list to choose from, but
	// are not handed to you. This is what a warlock patron's Expanded Spell
	// List does, and it is a different thing from Spells above: the warlock
	// still has to spend one of their few spells known on it.
	ExpandedSpells []SubSpell     `yaml:"expandedSpells" json:"expandedSpells"`
	Proficiency    ProficiencySet `yaml:"proficiencies" json:"proficiencies"`
	Spellcasting   *Spellcasting  `yaml:"spellcasting" json:"spellcasting"`
}

// SubSpell is an always prepared subclass spell (domain spells, oath spells...)
//
// Group is optional. When it is empty the spell is granted as soon as the
// character reaches Level. When it is set the subclass offers several
// alternative lists and the spell is only granted if the character picked that
// group - the druid's Circle of the Land, where the circle spells depend on
// the terrain you were initiated in, is the case this exists for. The pick
// itself is an ordinary Choice of kind "spellgroup" on the subclass.
type SubSpell struct {
	Id    string `yaml:"id" json:"id"`
	Level int    `yaml:"level" json:"level"`
	Group string `yaml:"group" json:"group"`
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
	Id       string  `yaml:"id" json:"id"`
	Name     string  `yaml:"name" json:"name"`
	Kind     string  `yaml:"kind" json:"kind"` // weapon|armor|shield|gear|pack|tool
	Category string  `yaml:"category" json:"category"`
	Cost     string  `yaml:"cost" json:"cost"`
	Weight   float64 `yaml:"weight" json:"weight"`
	Text     string  `yaml:"text" json:"text"`

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

	// ---- magic item fields ------------------------------------------------
	//
	// Rarity is what makes an item magical as far as the engine is concerned:
	// anything with a rarity set answers true to IsMagic. The SRD's own
	// vocabulary is used - common, uncommon, rare, very rare, legendary,
	// artifact - plus "varies" for the handful of items whose rarity depends
	// on a variant, such as a belt of giant strength.
	Rarity string `yaml:"rarity" json:"rarity"`
	// Attunement is set when the item requires attunement, and
	// AttunementNote carries the restriction when there is one, e.g.
	// "by a spellcaster" or "by a creature of good alignment".
	Attunement     bool   `yaml:"attunement" json:"attunement"`
	AttunementNote string `yaml:"attunementNote" json:"attunementNote"`
	// Base is the id of the mundane item this one is built on. A +1 longsword
	// sets base: longsword and inherits its damage, properties and weight, so
	// that a variant only has to state what it changes. Resolved once at load
	// time by resolveItemBases.
	Base string `yaml:"base" json:"base"`
	// The mechanical bonuses the engine can actually apply. AttackBonus and
	// DamageBonus land on the item's own attack; ACBonus and SaveBonus apply
	// while the item is equipped, whether it is armour, a shield, or a ring
	// or cloak of protection. An item that requires attunement contributes
	// none of them until it is attuned.
	AttackBonus int `yaml:"attackBonus" json:"attackBonus"`
	DamageBonus int `yaml:"damageBonus" json:"damageBonus"`
	ACBonus     int `yaml:"acBonus" json:"acBonus"`
	SaveBonus   int `yaml:"saveBonus" json:"saveBonus"`
	// Charges is the item's maximum charges, 0 for items that have none.
	Charges int `yaml:"charges" json:"charges"`
	// Clears names base fields the variant drops rather than inherits, for
	// the cases where the value it wants is the zero value and so cannot be
	// told apart from saying nothing. Mithral plate is the example: it is
	// plate with no Strength requirement and no stealth penalty, and
	// "strengthReq: 0" on its own would simply be overwritten from the base.
	//
	//	base: "plate"
	//	clears: ["strengthReq", "stealthDisadvantage"]
	//
	// The names are the yaml field names of Item.
	Clears []string `yaml:"clears" json:"clears"`
}

// IsMagic reports whether the item is a magic item. Rarity is the marker: an
// ordinary longsword has none, every magic item has one.
func (i *Item) IsMagic() bool { return i != nil && i.Rarity != "" }

// RarityName is the rarity capitalised for display, empty for mundane items.
func (i *Item) RarityName() string {
	if i == nil || i.Rarity == "" {
		return ""
	}
	return Titleize(i.Rarity)
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
	// Choices are the picks a feat asks you to make - the three proficiencies
	// from Skilled, the ability Resilient makes you proficient in. They are
	// collected alongside race, class and subclass choices, so a feat can ask
	// anything those can ask without new code.
	Choices []Choice `yaml:"choices" json:"choices"`
	// Bonuses are the numbers the feat moves, the same as on a Trait.
	Bonuses Bonuses `yaml:"bonuses" json:"bonuses"`
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
	Id       string `yaml:"id" json:"id"`
	Name     string `yaml:"name" json:"name"`
	Qty      int    `yaml:"qty" json:"qty"`
	Equipped bool   `yaml:"equipped" json:"equipped"`
	// Attuned marks one of the three items a character has attuned to. An
	// item that requires attunement gives no bonuses until this is set.
	Attuned bool    `yaml:"attuned" json:"attuned"`
	Weight  float64 `yaml:"weight" json:"weight"`
	Notes   string  `yaml:"notes" json:"notes"`
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
	// Id is the stable identity of this character, written into the sheet as
	// DND_ID and stamped onto every play session the character appears in so
	// that a query can gather everything about them.
	Id      string `yaml:"id" json:"id"`
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
	// Check is what you actually add to an ability check. It is the plain
	// modifier unless something like Jack of All Trades is in play.
	Check    int    `json:"check"`
	CheckStr string `json:"checkStr"`
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
	Name   string `json:"name"`
	Bonus  string `json:"bonus"`
	Damage string `json:"damage"`
	// Versatile is the two handed damage of a versatile weapon, already
	// carrying the same ability and magic modifiers as Damage. It is a
	// separate field rather than a note so a sheet can roll it on its own.
	Versatile string `json:"versatile"`
	Type      string `json:"type"`
	Range     string `json:"range"`
	Notes     string `json:"notes"`
	Proficent bool   `json:"proficient"`
}

// MagicItemView is one magic item as displayed on the sheet.
type MagicItemView struct {
	Id       string `json:"id"`
	Name     string `json:"name"`
	Rarity   string `json:"rarity"`
	Equipped bool   `json:"equipped"`
	// Attunement is "" when none is needed, "required" when the item wants it
	// and has not got it, or "attuned" when it has.
	Attunement string `json:"attunement"`
	Note       string `json:"note"`
	Effect     string `json:"effect"`
}

// SpellSlotView is the slot line for one spell level.
type SpellSlotView struct {
	Level int    `json:"level"`
	Label string `json:"label"`
	Total int    `json:"total"`
	Used  int    `json:"used"`
	Left  int    `json:"left"`
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

	Id           string `json:"id"`
	Name         string `json:"name"`
	Player       string `json:"player"`
	RaceName     string `json:"raceName"`
	ClassLine    string `json:"classLine"`
	ClassName    string `json:"className"`
	SubclassName string `json:"subclassName"`
	Background   string `json:"background"`
	Alignment    string `json:"alignment"`
	Level        int    `json:"level"`
	XP           int    `json:"xp"`
	NextLevelXP  int    `json:"nextLevelXp"`
	RulesetName  string `json:"rulesetName"`

	Abilities      []AbilityView          `json:"abilities"`
	AbilityMap     map[string]AbilityView `json:"abilityMap"`
	Proficiency    int                    `json:"proficiency"`
	ProficiencyStr string                 `json:"proficiencyStr"`

	Saves                []AbilityView `json:"saves"`
	Skills               []SkillView   `json:"skills"`
	PassivePerception    int           `json:"passivePerception"`
	PassiveInsight       int           `json:"passiveInsight"`
	PassiveInvestigation int           `json:"passiveInvestigation"`

	AC            int    `json:"ac"`
	ACSource      string `json:"acSource"`
	Initiative    int    `json:"initiative"`
	InitiativeStr string `json:"initiativeStr"`
	Speed         int    `json:"speed"`
	Size          string `json:"size"`
	Darkvision    int    `json:"darkvision"`

	HPMax       int    `json:"hpMax"`
	HPCurrent   int    `json:"hpCurrent"`
	HPTemp      int    `json:"hpTemp"`
	HPPercent   int    `json:"hpPercent"`
	HitDice     string `json:"hitDice"`
	HitDiceUsed int    `json:"hitDiceUsed"`
	DeathSaves  string `json:"deathSaves"`
	// DeathSaveBonus is what a death saving throw adds. It is normally zero -
	// a death save is a flat d20 - but a ring of protection or a paladin's
	// aura are saving throw bonuses and so apply to it.
	DeathSaveBonus int    `json:"deathSaveBonus"`
	DeathSaveStr   string `json:"deathSaveStr"`
	Inspiration    bool   `json:"inspiration"`

	Attacks       []AttackView `json:"attacks"`
	Equipment     []Gear       `json:"equipment"`
	Money         Money        `json:"money"`
	Weight        float64      `json:"weight"`
	CarryCapacity int          `json:"carryCapacity"`
	PushDragLift  int          `json:"pushDragLift"`

	// Magic items carried, and the attunement slots they take up. Attuned
	// lists the items actually attuned to; AttunementSlots is the limit,
	// which is three for everyone.
	MagicItems      []MagicItemView `json:"magicItems"`
	Attuned         []string        `json:"attuned"`
	AttunementUsed  int             `json:"attunementUsed"`
	AttunementSlots int             `json:"attunementSlots"`

	ArmorProficiencies  []string `json:"armorProficiencies"`
	WeaponProficiencies []string `json:"weaponProficiencies"`
	ToolProficiencies   []string `json:"toolProficiencies"`
	Languages           []string `json:"languages"`

	Features []Trait `json:"features"`
	Traits   []Trait `json:"traits"`

	IsCaster           bool             `json:"isCaster"`
	CastingAbility     string           `json:"castingAbility"`
	CastingAbilityName string           `json:"castingAbilityName"`
	SpellSaveDC        int              `json:"spellSaveDc"`
	SpellAttack        int              `json:"spellAttack"`
	SpellAttackStr     string           `json:"spellAttackStr"`
	CantripsKnown      int              `json:"cantripsKnown"`
	SpellsKnown        int              `json:"spellsKnown"`
	SpellsPrepared     int              `json:"spellsPrepared"`
	PreparedMax        int              `json:"preparedMax"`
	Slots              []SpellSlotView  `json:"slots"`
	SpellLevels        []SpellLevelView `json:"spellLevels"`
	SpellNotes         string           `json:"spellNotes"`
	PactMagic          bool             `json:"pactMagic"`

	Personality string `json:"personality"`
	Ideals      string `json:"ideals"`
	Bonds       string `json:"bonds"`
	Flaws       string `json:"flaws"`

	Age     string `json:"age"`
	Height  string `json:"height"`
	Weight_ string `json:"weightStr"`
	Eyes    string `json:"eyes"`
	Skin    string `json:"skin"`
	Hair    string `json:"hair"`

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
	Session  string `json:"session"`
	Step     string `json:"step"`
	Title    string `json:"title"`
	Question string `json:"question"`
	Help     string `json:"help"`
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
	Summary  string `json:"summary"`
	Sheet    *Sheet `json:"sheet,omitempty"`
	Org      string `json:"org,omitempty"`
	Filename string `json:"filename,omitempty"`
}

// Field is one input of a multi field prompt: an ability score to assign, or
// one line of the appearance step.
type Field struct {
	Id   string `json:"id"`
	Name string `json:"name"`
	Hint string `json:"hint"`
	// Kind names what the field holds, which is what ValidateField checks
	// against. Empty means free text with no rules.
	Kind  string `json:"kind,omitempty"`
	Value int    `json:"value"`
	// Min and Max bound the answer. For the appearance fields they are in
	// years, inches and pounds.
	Min   int `json:"min"`
	Max   int `json:"max"`
	Bonus int `json:"bonus"`
	Cost  int `json:"cost"`
	// Text is the value the field starts with.
	Text string `json:"text,omitempty"`
	// Options are suggested values. AllowCustom says whether the player may
	// type something that is not on the list.
	Options     []Option `json:"options,omitempty"`
	AllowCustom bool     `json:"allowCustom,omitempty"`
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
