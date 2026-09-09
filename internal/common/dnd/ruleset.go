package dnd

/* SDOC: DnD
* Ruleset Modules

  The dnd module is entirely data driven. Everything the engine knows about
  races, classes, backgrounds, items, spells and feats comes out of yaml
  "ruleset modules". A module is a single yaml file that looks like this:

  #+BEGIN_SRC yaml
  id: srd
  name: "SRD 5.1 Core Rules"
  version: "1.0"
  races:
    - id: dwarf
      name: Dwarf
      ...
  #+END_SRC

  Modules are merged together by =id=. Two files that both declare =id: srd=
  produce one combined =srd= ruleset, so dropping a new file into your dnd
  path is all that is required to add content:

  #+BEGIN_SRC yaml
  # ~/dnd/my-homebrew.yaml
  id: srd
  races:
    - id: aasimar
      name: Aasimar
      speed: 30
      abilityBonuses: {cha: 2}
  #+END_SRC

  A module can also declare a brand new ruleset that inherits from another
  one, which is how you build a variant game without copying the core data:

  #+BEGIN_SRC yaml
  id: grimdark
  name: "Grimdark 5e"
  extends: srd
  classes:
    - id: witcher
      name: Witcher
      hitDie: 10
  #+END_SRC

  Within a merge, an entry whose id already exists *extends* the existing
  entry: scalar fields overwrite when they are set, lists are appended and
  subraces / subclasses / features merge by id. That means a module can add a
  single subclass to an existing class without restating the class.

  Modules are searched for in (in order):
  1. The built in SRD data compiled into the binary.
  2. Every directory listed in =dndPaths= in your server settings.
  3. =<templatePath>/dnd= and =./templates/dnd=.

** Attaching numbers to a feature

  The SRD data carries the rules /text/ of every feature but no mechanics -
  it is prose, and the generator does not try to read numbers out of it. A
  =bonuses= block attaches the numbers, so a value the sheet computes (and a
  roll made from it) agrees with the character:

  #+BEGIN_SRC yaml
  id: srd
  classes:
    - id: bard
      features:
        - name: "Jack of All Trades"
          level: 2
          bonuses:
            checks: "prof/2"
  #+END_SRC

  Features merge by name and level, field by field, so only the block above
  is needed - the srd rules text is left alone. The fields are:

  | field      | effect                                                     |
  |------------+------------------------------------------------------------|
  | checks     | ability checks, and skills not already adding proficiency  |
  | abilities  | restricts =checks= to these abilities (default: all six)   |
  | initiative | initiative, on top of whatever =checks= gives it           |
  | saves      | every saving throw, death saving throws included           |
  | deathSave  | death saving throws only                                   |
  | speed      | the walking speed, in feet                                 |
  | minimum    | floors the evaluated bonus                                 |
  | unless     | switches the block off while a condition holds             |

  Each value is a formula: terms joined with "+", where a term is a number,
  an ability id whose modifier is added, or the proficiency bonus as =prof=,
  =prof/2= (rounded down) or =prof/2up= (rounded up). A feat takes the same
  block. See =templates/dnd/bonuses.yaml= for worked examples.

  =unless= is the one field that is not a formula. Every feature in the rules
  that moves a speed is qualified by what you are wearing, so those two
  qualifications are what it understands:

  | value        | the block is off while...                    |
  |--------------+----------------------------------------------|
  | =heavyArmor= | you are wearing heavy armour                 |
  | =armor=      | you are wearing any armour or carrying a shield |

  A barbarian's Fast Movement is =speed: "10"= with =unless: "heavyArmor"=.
  An unrecognised value never fires, so a module that invents one keeps its
  bonus rather than quietly losing it. Conditions the sheet cannot check -
  "while you are raging" - are deliberately left in the rules text instead.
EDOC */

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

//go:embed data/*.yaml
var builtinData embed.FS

// Library is the set of all known rulesets.
//
// Rulesets holds the modules exactly as they were loaded, resolved holds the
// same rulesets with "extends" inheritance applied. Keeping the two apart
// means Resolve can be re-run any number of times without a ruleset
// accumulating its parent's content over and over.
type Library struct {
	Rulesets map[string]*Ruleset
	Order    []string
	Errors   []string
	resolved map[string]*Ruleset
	lock     sync.RWMutex
}

// DefaultRuleset is used when a character does not name one.
const DefaultRuleset = "srd"

// NewLibrary loads the built in data plus every yaml module found in paths.
func NewLibrary(paths []string) *Library {
	lib := &Library{Rulesets: map[string]*Ruleset{}}
	lib.loadBuiltin()
	for _, p := range paths {
		lib.LoadDir(p)
	}
	lib.Resolve()
	return lib
}

func (l *Library) loadBuiltin() {
	entries, err := builtinData.ReadDir("data")
	if err != nil {
		l.Errors = append(l.Errors, fmt.Sprintf("dnd: no built in data: %v", err))
		return
	}
	names := []string{}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".yaml") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, n := range names {
		data, err := builtinData.ReadFile("data/" + n)
		if err != nil {
			l.Errors = append(l.Errors, fmt.Sprintf("dnd: %s: %v", n, err))
			continue
		}
		l.LoadBytes(data, "builtin:"+n)
	}
}

// LoadDir loads every .yaml/.yml file in a directory (recursively).
func (l *Library) LoadDir(dir string) {
	if strings.TrimSpace(dir) == "" {
		return
	}
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return
	}
	files := []string{}
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".yaml" || ext == ".yml" {
			files = append(files, path)
		}
		return nil
	})
	sort.Strings(files)
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			l.Errors = append(l.Errors, fmt.Sprintf("dnd: %s: %v", f, err))
			continue
		}
		l.LoadBytes(data, f)
	}
}

// LoadBytes parses one module and merges it into the library.
func (l *Library) LoadBytes(data []byte, source string) {
	var mod Ruleset
	if err := yaml.Unmarshal(data, &mod); err != nil {
		l.Errors = append(l.Errors, fmt.Sprintf("dnd: %s: %v", source, err))
		return
	}
	if strings.TrimSpace(mod.Id) == "" {
		l.Errors = append(l.Errors, fmt.Sprintf("dnd: %s: module has no id, skipped", source))
		return
	}
	mod.Modules = []string{source}
	l.lock.Lock()
	defer l.lock.Unlock()
	if existing, ok := l.Rulesets[mod.Id]; ok {
		mergeRuleset(existing, &mod)
	} else {
		l.Rulesets[mod.Id] = &mod
		l.Order = append(l.Order, mod.Id)
	}
}

// Resolve applies "extends" inheritance and rebuilds the lookup indexes.
// It is safe to call repeatedly.
func (l *Library) Resolve() {
	l.lock.Lock()
	defer l.lock.Unlock()
	sort.Strings(l.Order)
	l.resolved = map[string]*Ruleset{}
	for _, id := range l.Order {
		l.resolved[id] = l.resolveOne(id, 0)
	}
}

// resolveOne merges a ruleset onto its (already resolved) parent.
func (l *Library) resolveOne(id string, depth int) *Ruleset {
	rs, ok := l.Rulesets[id]
	if !ok {
		return nil
	}
	out := (*Ruleset)(nil)
	parentId := rs.Extends
	if parentId == "" || parentId == id || depth > 8 {
		out = cloneRuleset(rs)
	} else if parent := l.resolveOne(parentId, depth+1); parent != nil {
		out = cloneRuleset(parent)
		mergeRuleset(out, rs)
		out.Id = rs.Id
		out.Name = rs.Name
		out.Version = rs.Version
		out.Description = rs.Description
		out.Extends = rs.Extends
	} else {
		l.Errors = append(l.Errors,
			fmt.Sprintf("dnd: ruleset %q extends unknown ruleset %q", id, parentId))
		out = cloneRuleset(rs)
	}
	out.Modules = uniqueStrings(out.Modules)
	out.Index()
	return out
}

func uniqueStrings(list []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, v := range list {
		if seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

// Get returns a ruleset by id, falling back to the default one.
func (l *Library) Get(id string) *Ruleset {
	l.lock.RLock()
	defer l.lock.RUnlock()
	if l.resolved == nil {
		return nil
	}
	if id != "" {
		if rs, ok := l.resolved[id]; ok {
			return rs
		}
	}
	if rs, ok := l.resolved[DefaultRuleset]; ok {
		return rs
	}
	for _, k := range l.Order {
		return l.resolved[k]
	}
	return nil
}

// List returns summary information for every loaded ruleset.
func (l *Library) List() []RulesetInfo {
	l.lock.RLock()
	defer l.lock.RUnlock()
	out := []RulesetInfo{}
	for _, id := range l.Order {
		rs := l.resolved[id]
		if rs == nil {
			continue
		}
		out = append(out, RulesetInfo{
			Id: rs.Id, Name: rs.Name, Version: rs.Version, Description: rs.Description,
			Extends: rs.Extends, Modules: rs.Modules,
			Races: len(rs.Races), Classes: len(rs.Classes), Backgrounds: len(rs.Backgrounds),
			Spells: len(rs.Spells), Items: len(rs.Items), Feats: len(rs.Feats),
		})
	}
	return out
}

// ----------------------------------------------------------------------------
// Merging
// ----------------------------------------------------------------------------

func mergeRuleset(dst, src *Ruleset) {
	if src.Name != "" {
		dst.Name = src.Name
	}
	if src.Version != "" {
		dst.Version = src.Version
	}
	if src.Description != "" {
		dst.Description = src.Description
	}
	if src.Extends != "" {
		dst.Extends = src.Extends
	}
	dst.Modules = append(dst.Modules, src.Modules...)
	dst.Languages = addUnique(dst.Languages, src.Languages...)
	dst.Alignments = addUnique(dst.Alignments, src.Alignments...)

	for _, s := range src.Skills {
		if i := indexOfSkill(dst.Skills, s.Id); i >= 0 {
			dst.Skills[i] = s
		} else {
			dst.Skills = append(dst.Skills, s)
		}
	}
	for _, r := range src.Races {
		if i := indexOfRace(dst.Races, r.Id); i >= 0 {
			mergeRace(&dst.Races[i], &r)
		} else {
			dst.Races = append(dst.Races, r)
		}
	}
	for _, c := range src.Classes {
		if i := indexOfClass(dst.Classes, c.Id); i >= 0 {
			mergeClass(&dst.Classes[i], &c)
		} else {
			dst.Classes = append(dst.Classes, c)
		}
	}
	for _, b := range src.Backgrounds {
		if i := indexOfBackground(dst.Backgrounds, b.Id); i >= 0 {
			dst.Backgrounds[i] = b
		} else {
			dst.Backgrounds = append(dst.Backgrounds, b)
		}
	}
	for _, it := range src.Items {
		if i := indexOfItem(dst.Items, it.Id); i >= 0 {
			dst.Items[i] = it
		} else {
			dst.Items = append(dst.Items, it)
		}
	}
	for _, sp := range src.Spells {
		if i := indexOfSpell(dst.Spells, sp.Id); i >= 0 {
			dst.Spells[i] = sp
		} else {
			dst.Spells = append(dst.Spells, sp)
		}
	}
	for _, f := range src.Feats {
		if i := indexOfFeat(dst.Feats, f.Id); i >= 0 {
			dst.Feats[i] = f
		} else {
			dst.Feats = append(dst.Feats, f)
		}
	}
	// Conditions and damage types are keyed by id like everything else, so a
	// module may add one or restate one without repeating the whole set. An
	// empty list on both sides leaves the built in ones in play.
	for _, cd := range src.Conditions {
		if i := indexOfCondition(dst.Conditions, cd.Id); i >= 0 {
			dst.Conditions[i] = cd
		} else {
			dst.Conditions = append(dst.Conditions, cd)
		}
	}
	for _, dt := range src.DamageTypes {
		if i := indexOfDamageType(dst.DamageTypes, dt.Id); i >= 0 {
			dst.DamageTypes[i] = dt
		} else {
			dst.DamageTypes = append(dst.DamageTypes, dt)
		}
	}
	if len(src.SlotTables) > 0 {
		if dst.SlotTables == nil {
			dst.SlotTables = map[string][][]int{}
		}
		for k, v := range src.SlotTables {
			dst.SlotTables[k] = v
		}
	}
}

func mergeRace(dst, src *Race) {
	if src.Name != "" {
		dst.Name = src.Name
	}
	if src.Source != "" {
		dst.Source = src.Source
	}
	if src.Summary != "" {
		dst.Summary = src.Summary
	}
	if src.Text != "" {
		dst.Text = src.Text
	}
	if src.Size != "" {
		dst.Size = src.Size
	}
	if src.Speed != 0 {
		dst.Speed = src.Speed
	}
	if src.Darkvision != 0 {
		dst.Darkvision = src.Darkvision
	}
	if src.HpPerLevel != 0 {
		dst.HpPerLevel = src.HpPerLevel
	}
	if src.Age != "" {
		dst.Age = src.Age
	}
	if src.Alignment != "" {
		dst.Alignment = src.Alignment
	}
	if src.ExtraLanguages != 0 {
		dst.ExtraLanguages = src.ExtraLanguages
	}
	if src.AbilityChoice != nil {
		dst.AbilityChoice = src.AbilityChoice
	}
	if len(src.AbilityBonuses) > 0 {
		if dst.AbilityBonuses == nil {
			dst.AbilityBonuses = map[string]int{}
		}
		for k, v := range src.AbilityBonuses {
			dst.AbilityBonuses[k] = v
		}
	}
	dst.Languages = addUnique(dst.Languages, src.Languages...)
	dst.Names = addUnique(dst.Names, src.Names...)
	dst.Traits = mergeTraits(dst.Traits, src.Traits)
	dst.Proficiencies = mergeProficiencies(dst.Proficiencies, src.Proficiencies)
	dst.Choices = mergeChoices(dst.Choices, src.Choices)
	dst.Spells = append(dst.Spells, src.Spells...)
	for _, sr := range src.Subraces {
		if i := indexOfRace(dst.Subraces, sr.Id); i >= 0 {
			mergeRace(&dst.Subraces[i], &sr)
		} else {
			dst.Subraces = append(dst.Subraces, sr)
		}
	}
}

func mergeClass(dst, src *Class) {
	if src.Name != "" {
		dst.Name = src.Name
	}
	if src.Source != "" {
		dst.Source = src.Source
	}
	if src.Summary != "" {
		dst.Summary = src.Summary
	}
	if src.Text != "" {
		dst.Text = src.Text
	}
	if src.HitDie != 0 {
		dst.HitDie = src.HitDie
	}
	if len(src.PrimaryAbility) > 0 {
		dst.PrimaryAbility = src.PrimaryAbility
	}
	if len(src.SavingThrows) > 0 {
		dst.SavingThrows = src.SavingThrows
	}
	if src.SkillCount != 0 {
		dst.SkillCount = src.SkillCount
	}
	if len(src.SkillsFrom) > 0 {
		dst.SkillsFrom = src.SkillsFrom
	}
	if len(src.Equipment) > 0 {
		dst.Equipment = append(dst.Equipment, src.Equipment...)
	}
	if len(src.FixedEquipment) > 0 {
		dst.FixedEquipment = append(dst.FixedEquipment, src.FixedEquipment...)
	}
	if src.SubclassLevel != 0 {
		dst.SubclassLevel = src.SubclassLevel
	}
	if src.SubclassLabel != "" {
		dst.SubclassLabel = src.SubclassLabel
	}
	if src.Spellcasting != nil {
		dst.Spellcasting = src.Spellcasting
	}
	if len(src.ASILevels) > 0 {
		dst.ASILevels = src.ASILevels
	}
	if len(src.MulticlassReq) > 0 {
		dst.MulticlassReq = src.MulticlassReq
	}
	if src.UnarmoredAC != "" {
		// The two go together: whoever states the formula also states whether
		// a shield may be carried with it.
		dst.UnarmoredAC = src.UnarmoredAC
		dst.UnarmoredShield = src.UnarmoredShield
	}
	dst.Proficiencies = mergeProficiencies(dst.Proficiencies, src.Proficiencies)
	dst.Features = mergeTraits(dst.Features, src.Features)
	dst.Choices = mergeChoices(dst.Choices, src.Choices)
	for _, sc := range src.Subclasses {
		if i := indexOfSubclass(dst.Subclasses, sc.Id); i >= 0 {
			d := &dst.Subclasses[i]
			if sc.Name != "" {
				d.Name = sc.Name
			}
			if sc.Summary != "" {
				d.Summary = sc.Summary
			}
			if sc.Spellcasting != nil {
				d.Spellcasting = sc.Spellcasting
			}
			d.Features = mergeTraits(d.Features, sc.Features)
			d.Choices = mergeChoices(d.Choices, sc.Choices)
			d.Spells = append(d.Spells, sc.Spells...)
			d.ExpandedSpells = append(d.ExpandedSpells, sc.ExpandedSpells...)
			d.Proficiency = mergeProficiencySets(d.Proficiency, sc.Proficiency)
		} else {
			dst.Subclasses = append(dst.Subclasses, sc)
		}
	}
}

// mergeTraits merges by name and level, field by field, so that an add on
// module can attach a bonuses block to a feature the srd already defines
// without having to restate its rules text:
//
//	classes:
//	  - id: "bard"
//	    features:
//	      - name: "Jack of All Trades"
//	        level: 2
//	        bonuses:
//	          checks: "prof/2"
//
// A =uses= / =recharge= pair is attached the same way, for a feature whose use
// limit the engine cannot read out of its prose.
//
// A module that does supply text still replaces the text, as before.
func mergeTraits(dst, src []Trait) []Trait {
	for _, t := range src {
		found := false
		for i := range dst {
			if dst[i].Name == t.Name && dst[i].Level == t.Level {
				if t.Text != "" {
					dst[i].Text = t.Text
				}
				if t.Source != "" {
					dst[i].Source = t.Source
				}
				if t.Uses != "" {
					dst[i].Uses = t.Uses
				}
				if t.Recharge != "" {
					dst[i].Recharge = t.Recharge
				}
				dst[i].Bonuses = mergeBonuses(dst[i].Bonuses, t.Bonuses)
				found = true
				break
			}
		}
		if !found {
			dst = append(dst, t)
		}
	}
	return dst
}

// mergeBonuses lets a module set the fields it cares about and leave the rest
// of an existing block alone.
func mergeBonuses(dst, src Bonuses) Bonuses {
	if src.Checks != "" {
		dst.Checks = src.Checks
	}
	if src.Initiative != "" {
		dst.Initiative = src.Initiative
	}
	if src.Saves != "" {
		dst.Saves = src.Saves
	}
	if src.DeathSave != "" {
		dst.DeathSave = src.DeathSave
	}
	if len(src.Abilities) > 0 {
		dst.Abilities = src.Abilities
	}
	if src.Minimum != 0 {
		dst.Minimum = src.Minimum
	}
	if src.Speed != "" {
		dst.Speed = src.Speed
	}
	if src.Unless != "" {
		dst.Unless = src.Unless
	}
	return dst
}

func mergeChoices(dst, src []Choice) []Choice {
	for _, c := range src {
		found := false
		for i := range dst {
			if dst[i].Id == c.Id {
				mergeChoice(&dst[i], &c)
				found = true
				break
			}
		}
		if !found {
			dst = append(dst, c)
		}
	}
	return dst
}

// mergeChoice extends an existing choice rather than replacing it, which is
// what lets a module add two metamagic options or five fighting styles
// without restating the ones already there. Scalars overwrite when the
// incoming value is set, Options merge by name and From merges by value.
func mergeChoice(dst, src *Choice) {
	if src.Name != "" {
		dst.Name = src.Name
	}
	if src.Prompt != "" {
		dst.Prompt = src.Prompt
	}
	if src.Help != "" {
		dst.Help = src.Help
	}
	if src.Kind != "" {
		dst.Kind = src.Kind
	}
	if src.Grants != "" {
		dst.Grants = src.Grants
	}
	if src.Level != 0 {
		dst.Level = src.Level
	}
	if src.Count != 0 {
		dst.Count = src.Count
	}
	if len(src.CountByLevel) > 0 {
		dst.CountByLevel = src.CountByLevel
	}
	if len(src.SpellLevels) > 0 {
		dst.SpellLevels = src.SpellLevels
	}
	dst.From = addUnique(dst.From, src.From...)
	for _, o := range src.Options {
		replaced := false
		for i := range dst.Options {
			if dst.Options[i].Name == o.Name {
				dst.Options[i] = o
				replaced = true
				break
			}
		}
		if !replaced {
			dst.Options = append(dst.Options, o)
		}
	}
}

func mergeProficiencies(dst, src Proficiencies) Proficiencies {
	dst.Armor = addUnique(dst.Armor, src.Armor...)
	dst.Weapons = addUnique(dst.Weapons, src.Weapons...)
	dst.Tools = addUnique(dst.Tools, src.Tools...)
	dst.Skills = addUnique(dst.Skills, src.Skills...)
	dst.Saves = addUnique(dst.Saves, src.Saves...)
	return dst
}

// mergeProficiencySets merges two sets block by block. Blocks that arrive at
// the same level are merged into one; a level the destination does not have
// yet is appended. That keeps "extend the subclass" modules additive without
// letting a 7th level grant collapse into a 3rd level one.
func mergeProficiencySets(dst, src ProficiencySet) ProficiencySet {
	for _, sp := range src {
		merged := false
		for i := range dst {
			if dst[i].Level == sp.Level {
				dst[i] = mergeProficiencies(dst[i], sp)
				merged = true
				break
			}
		}
		if !merged {
			dst = append(dst, sp)
		}
	}
	return dst
}

func cloneRuleset(src *Ruleset) *Ruleset {
	data, err := yaml.Marshal(src)
	if err != nil {
		return src
	}
	var out Ruleset
	if err := yaml.Unmarshal(data, &out); err != nil {
		return src
	}
	out.Modules = append([]string{}, src.Modules...)
	return &out
}

func indexOfSkill(l []Skill, id string) int {
	for i := range l {
		if l[i].Id == id {
			return i
		}
	}
	return -1
}
func indexOfRace(l []Race, id string) int {
	for i := range l {
		if l[i].Id == id {
			return i
		}
	}
	return -1
}
func indexOfClass(l []Class, id string) int {
	for i := range l {
		if l[i].Id == id {
			return i
		}
	}
	return -1
}
func indexOfSubclass(l []Subclass, id string) int {
	for i := range l {
		if l[i].Id == id {
			return i
		}
	}
	return -1
}
func indexOfBackground(l []Background, id string) int {
	for i := range l {
		if l[i].Id == id {
			return i
		}
	}
	return -1
}
func indexOfItem(l []Item, id string) int {
	for i := range l {
		if l[i].Id == id {
			return i
		}
	}
	return -1
}
func indexOfSpell(l []Spell, id string) int {
	for i := range l {
		if l[i].Id == id {
			return i
		}
	}
	return -1
}
func indexOfFeat(l []Feat, id string) int {
	for i := range l {
		if l[i].Id == id {
			return i
		}
	}
	return -1
}

func indexOfCondition(l []Condition, id string) int {
	for i := range l {
		if l[i].Id == id {
			return i
		}
	}
	return -1
}

func indexOfDamageType(l []DamageType, id string) int {
	for i := range l {
		if l[i].Id == id {
			return i
		}
	}
	return -1
}

// ----------------------------------------------------------------------------
// Ruleset lookups
// ----------------------------------------------------------------------------

// Index rebuilds the internal lookup tables. Call after mutating a ruleset.
func (r *Ruleset) Index() {
	r.skillIdx = map[string]*Skill{}
	r.raceIdx = map[string]*Race{}
	r.classIdx = map[string]*Class{}
	r.bgIdx = map[string]*Background{}
	r.itemIdx = map[string]*Item{}
	r.spellIdx = map[string]*Spell{}
	r.featIdx = map[string]*Feat{}
	for i := range r.Skills {
		r.skillIdx[r.Skills[i].Id] = &r.Skills[i]
	}
	for i := range r.Races {
		r.raceIdx[r.Races[i].Id] = &r.Races[i]
		for j := range r.Races[i].Subraces {
			r.Races[i].Subraces[j].Parent = r.Races[i].Id
			r.raceIdx[r.Races[i].Subraces[j].Id] = &r.Races[i].Subraces[j]
		}
	}
	for i := range r.Classes {
		r.classIdx[r.Classes[i].Id] = &r.Classes[i]
	}
	for i := range r.Backgrounds {
		r.bgIdx[r.Backgrounds[i].Id] = &r.Backgrounds[i]
	}
	for i := range r.Items {
		r.itemIdx[r.Items[i].Id] = &r.Items[i]
	}
	for i := range r.Spells {
		r.spellIdx[r.Spells[i].Id] = &r.Spells[i]
	}
	for i := range r.Feats {
		r.featIdx[r.Feats[i].Id] = &r.Feats[i]
	}
	r.resolveItemBases()
}

// resolveItemBases fills in the fields a magic item inherits from the mundane
// item it is built on. A "+1 longsword" only has to say base: longsword and
// what it changes; everything it leaves blank - damage, properties, armour
// class, weight, category - comes from the base.
//
// Chains are followed, so a variant may be built on another variant, and a
// cycle or a missing base is simply left alone rather than looping forever.
func (r *Ruleset) resolveItemBases() {
	var fill func(it *Item, depth int)
	fill = func(it *Item, depth int) {
		if it == nil || it.Base == "" || depth > 8 {
			return
		}
		base := r.itemIdx[it.Base]
		if base == nil || base == it {
			return
		}
		fill(base, depth+1)
		cleared := map[string]bool{}
		for _, f := range it.Clears {
			cleared[f] = true
		}
		keep := func(field string, empty bool) bool { return empty && !cleared[field] }
		if keep("kind", it.Kind == "") {
			it.Kind = base.Kind
		}
		if keep("category", it.Category == "") {
			it.Category = base.Category
		}
		if keep("weight", it.Weight == 0) {
			it.Weight = base.Weight
		}
		if keep("damage", it.Damage == "") {
			it.Damage = base.Damage
		}
		if keep("damageType", it.DamageType == "") {
			it.DamageType = base.DamageType
		}
		if keep("versatile", it.Versatile == "") {
			it.Versatile = base.Versatile
		}
		if keep("properties", len(it.Properties) == 0) {
			it.Properties = base.Properties
		}
		if keep("range", it.Range == "") {
			it.Range = base.Range
		}
		if keep("ac", it.AC == 0) {
			it.AC = base.AC
		}
		if keep("armorType", it.ArmorType == "") {
			it.ArmorType = base.ArmorType
		}
		if keep("dexMax", it.DexMax == 0) {
			it.DexMax = base.DexMax
		}
		if keep("noDex", !it.NoDex) {
			it.NoDex = base.NoDex
		}
		if keep("strengthReq", it.StrengthReq == 0) {
			it.StrengthReq = base.StrengthReq
		}
		if keep("stealthDisadvantage", !it.StealthDisadvantage) {
			it.StealthDisadvantage = base.StealthDisadvantage
		}
	}
	for i := range r.Items {
		fill(&r.Items[i], 0)
	}
}

func (r *Ruleset) ensure() {
	if r.raceIdx == nil {
		r.Index()
	}
}

// Skill looks up a skill by id.
func (r *Ruleset) Skill(id string) *Skill {
	r.ensure()
	return r.skillIdx[id]
}

// SkillName returns a display name for a skill id.
func (r *Ruleset) SkillName(id string) string {
	if s := r.Skill(id); s != nil {
		return s.Name
	}
	return Titleize(id)
}

// Race looks up a race or subrace by id.
func (r *Ruleset) Race(id string) *Race {
	r.ensure()
	return r.raceIdx[id]
}

// Class looks up a class by id.
func (r *Ruleset) Class(id string) *Class {
	r.ensure()
	return r.classIdx[id]
}

// Subclass looks up a subclass within a class.
func (r *Ruleset) Subclass(classId, id string) *Subclass {
	c := r.Class(classId)
	if c == nil {
		return nil
	}
	for i := range c.Subclasses {
		if c.Subclasses[i].Id == id {
			return &c.Subclasses[i]
		}
	}
	return nil
}

// Background looks up a background by id.
func (r *Ruleset) Background(id string) *Background {
	r.ensure()
	return r.bgIdx[id]
}

// Item looks up an item by id, or by name for hand written sheets.
func (r *Ruleset) Item(id string) *Item {
	r.ensure()
	if it, ok := r.itemIdx[id]; ok {
		return it
	}
	if it, ok := r.itemIdx[Slugify(id)]; ok {
		return it
	}
	for i := range r.Items {
		if strings.EqualFold(r.Items[i].Name, id) {
			return &r.Items[i]
		}
	}
	return nil
}

// Spell looks up a spell by id or name.
func (r *Ruleset) Spell(id string) *Spell {
	r.ensure()
	if s, ok := r.spellIdx[id]; ok {
		return s
	}
	if s, ok := r.spellIdx[Slugify(id)]; ok {
		return s
	}
	for i := range r.Spells {
		if strings.EqualFold(r.Spells[i].Name, id) {
			return &r.Spells[i]
		}
	}
	return nil
}

// Feat looks up a feat by id.
func (r *Ruleset) Feat(id string) *Feat {
	r.ensure()
	return r.featIdx[id]
}

// SpellsForClass returns every spell of a level available to a class, sorted
// by name. Level -1 means "all levels".
func (r *Ruleset) SpellsForClass(classId string, level int) []*Spell {
	out := []*Spell{}
	for i := range r.Spells {
		s := &r.Spells[i]
		if level >= 0 && s.Level != level {
			continue
		}
		if classId != "" && !containsStr(s.Classes, classId) {
			continue
		}
		out = append(out, s)
	}
	sort.Slice(out, func(a, b int) bool {
		if out[a].Level != out[b].Level {
			return out[a].Level < out[b].Level
		}
		return out[a].Name < out[b].Name
	})
	return out
}

// ItemsOfKind returns every item of a kind (weapon, armor, gear...).
func (r *Ruleset) ItemsOfKind(kind string, category string) []*Item {
	out := []*Item{}
	for i := range r.Items {
		it := &r.Items[i]
		if kind != "" && it.Kind != kind {
			continue
		}
		if category != "" && !strings.EqualFold(it.Category, category) {
			continue
		}
		out = append(out, it)
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Name < out[b].Name })
	return out
}

// AllLanguages is the language list with a sane fallback.
func (r *Ruleset) AllLanguages() []string {
	if len(r.Languages) > 0 {
		return r.Languages
	}
	return []string{"Common", "Dwarvish", "Elvish", "Giant", "Gnomish", "Goblin",
		"Halfling", "Orc", "Abyssal", "Celestial", "Draconic", "Deep Speech",
		"Infernal", "Primordial", "Sylvan", "Undercommon"}
}

// AllAlignments is the alignment list with a sane fallback.
func (r *Ruleset) AllAlignments() []string {
	if len(r.Alignments) > 0 {
		return r.Alignments
	}
	return []string{"Lawful Good", "Neutral Good", "Chaotic Good",
		"Lawful Neutral", "True Neutral", "Chaotic Neutral",
		"Lawful Evil", "Neutral Evil", "Chaotic Evil"}
}
