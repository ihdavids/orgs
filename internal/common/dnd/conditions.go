//lint:file-ignore ST1006 allow the use of self
package dnd

// ----------------------------------------------------------------------------
// Conditions and defenses
//
// What is currently wrong with a character - frightened, poisoned, three
// levels into exhaustion - and what they shrug off: the damage they take less
// of, none of, or more of.
//
// Both live on the character rather than being derived, because neither can be
// worked out from the rules alone. A condition is something the table decided
// happened, and a resistance may come from a race, a spell that is running, a
// cloak that is being worn or a boon the DM handed out; the sheet cannot tell
// which, so it keeps what it is told and writes it into the org file.
//
// There is no server side state here either. ApplyConditionChange moves the
// character on and the caller writes the sheet back out, exactly the way the
// purse and the inventory work.
// ----------------------------------------------------------------------------

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// The actions a conditions call may ask for.
const (
	CondAdd       = "add"
	CondRemove    = "remove"
	CondToggle    = "toggle"
	CondLevel     = "level"
	CondClear     = "clear"
	CondResist    = "resist"
	CondImmune    = "immune"
	CondVulnerabl = "vulnerable"
	CondUnprotect = "unprotect"
)

// The three kinds of defense, which are also the words the history uses.
const (
	Resistance    = "resistance"
	Immunity      = "immunity"
	Vulnerability = "vulnerability"
)

// Condition is one condition a creature can be under. Text is the rules, and
// Levels is how many degrees the condition has - only exhaustion has any, and
// then Effects is what each level does, worst last.
type Condition struct {
	Id   string `yaml:"id" json:"id"`
	Name string `yaml:"name" json:"name"`
	Text string `yaml:"text" json:"text"`
	// Icon names the drawing the sheet puts on the condition's button. It
	// defaults to the id, so a module only sets it when it wants to borrow
	// another condition's icon for one the sheet has never heard of.
	Icon    string   `yaml:"icon" json:"icon"`
	Levels  int      `yaml:"levels" json:"levels"`
	Effects []string `yaml:"effects" json:"effects"`
}

// DamageType is one kind of damage, which is what a resistance, an immunity or
// a vulnerability is usually against.
type DamageType struct {
	Id   string `yaml:"id" json:"id"`
	Name string `yaml:"name" json:"name"`
	// Physical marks the three types a mundane weapon deals. Resistance to
	// them is common enough - and qualified often enough ("from nonmagical
	// attacks") - to be worth telling apart on the sheet.
	Physical bool `yaml:"physical" json:"physical"`
}

// SRDConditions are the conditions of the basic rules. A ruleset module may
// add to them or replace one by id, but it does not have to say anything at
// all: these are what a sheet gets when nothing does.
var SRDConditions = []Condition{
	{Id: "blinded", Name: "Blinded",
		Text: "A blinded creature can't see and automatically fails any ability " +
			"check that requires sight. Attack rolls against the creature have " +
			"advantage, and the creature's attack rolls have disadvantage."},
	{Id: "charmed", Name: "Charmed",
		Text: "A charmed creature can't attack the charmer or target the charmer " +
			"with harmful abilities or magical effects. The charmer has advantage " +
			"on any ability check to interact socially with the creature."},
	{Id: "deafened", Name: "Deafened",
		Text: "A deafened creature can't hear and automatically fails any ability " +
			"check that requires hearing."},
	{Id: "exhaustion", Name: "Exhaustion", Levels: 6,
		Text: "Exhaustion is measured in six levels, and its effects are " +
			"cumulative: a creature suffering level 2 also suffers level 1. " +
			"Finishing a long rest reduces it by one level, as long as the " +
			"creature has eaten and drunk something.",
		Effects: []string{
			"Disadvantage on ability checks",
			"Speed halved",
			"Disadvantage on attack rolls and saving throws",
			"Hit point maximum halved",
			"Speed reduced to 0",
			"Death",
		}},
	{Id: "frightened", Name: "Frightened",
		Text: "A frightened creature has disadvantage on ability checks and attack " +
			"rolls while the source of its fear is within line of sight, and can't " +
			"willingly move closer to the source of its fear."},
	{Id: "grappled", Name: "Grappled",
		Text: "A grappled creature's speed becomes 0, and it can't benefit from any " +
			"bonus to its speed. The condition ends if the grappler is incapacitated, " +
			"or if the creature is removed from the reach of the grapple."},
	{Id: "incapacitated", Name: "Incapacitated",
		Text: "An incapacitated creature can't take actions or reactions."},
	{Id: "invisible", Name: "Invisible",
		Text: "An invisible creature is impossible to see without the aid of magic " +
			"or a special sense, and counts as heavily obscured for hiding. Attack " +
			"rolls against the creature have disadvantage, and its own have advantage."},
	{Id: "paralyzed", Name: "Paralyzed",
		Text: "A paralyzed creature is incapacitated and can't move or speak. It " +
			"automatically fails Strength and Dexterity saving throws, attack rolls " +
			"against it have advantage, and any attack that hits it from within 5 " +
			"feet is a critical hit."},
	{Id: "petrified", Name: "Petrified",
		Text: "A petrified creature is turned to stone along with everything it is " +
			"wearing or carrying. It is incapacitated, unaware of its surroundings, " +
			"weighs ten times as much and stops aging. It automatically fails " +
			"Strength and Dexterity saving throws, has resistance to all damage, and " +
			"is immune to poison and disease."},
	{Id: "poisoned", Name: "Poisoned",
		Text: "A poisoned creature has disadvantage on attack rolls and ability checks."},
	{Id: "prone", Name: "Prone",
		Text: "A prone creature's only movement is to crawl, unless it stands up. It " +
			"has disadvantage on attack rolls. An attack against it has advantage " +
			"from within 5 feet and disadvantage from further away."},
	{Id: "restrained", Name: "Restrained",
		Text: "A restrained creature's speed becomes 0. Attack rolls against it have " +
			"advantage, its own have disadvantage, and it has disadvantage on " +
			"Dexterity saving throws."},
	{Id: "stunned", Name: "Stunned",
		Text: "A stunned creature is incapacitated, can't move, and can speak only " +
			"falteringly. It automatically fails Strength and Dexterity saving " +
			"throws, and attack rolls against it have advantage."},
	{Id: "unconscious", Name: "Unconscious",
		Text: "An unconscious creature is incapacitated, unaware of its surroundings, " +
			"drops what it is holding and falls prone. It automatically fails " +
			"Strength and Dexterity saving throws, attack rolls against it have " +
			"advantage, and any attack that hits it from within 5 feet is a critical hit."},
}

// SRDDamageTypes are the thirteen kinds of damage in the basic rules.
var SRDDamageTypes = []DamageType{
	{Id: "acid", Name: "Acid"},
	{Id: "bludgeoning", Name: "Bludgeoning", Physical: true},
	{Id: "cold", Name: "Cold"},
	{Id: "fire", Name: "Fire"},
	{Id: "force", Name: "Force"},
	{Id: "lightning", Name: "Lightning"},
	{Id: "necrotic", Name: "Necrotic"},
	{Id: "piercing", Name: "Piercing", Physical: true},
	{Id: "poison", Name: "Poison"},
	{Id: "psychic", Name: "Psychic"},
	{Id: "radiant", Name: "Radiant"},
	{Id: "slashing", Name: "Slashing", Physical: true},
	{Id: "thunder", Name: "Thunder"},
}

// ConditionList is every condition this ruleset knows, which is the basic
// rules' fifteen unless a module has said otherwise.
func (self *Ruleset) ConditionList() []Condition {
	list := SRDConditions
	if self != nil && len(self.Conditions) > 0 {
		list = self.Conditions
	}
	out := make([]Condition, 0, len(list))
	for _, cd := range list {
		if cd.Icon == "" {
			cd.Icon = cd.Id
		}
		out = append(out, cd)
	}
	return out
}

// Condition finds one by id or by name, case insensitively.
func (self *Ruleset) Condition(nameOrId string) *Condition {
	key := strings.TrimSpace(nameOrId)
	if key == "" {
		return nil
	}
	slug := Slugify(key)
	list := self.ConditionList()
	for i := range list {
		if strings.EqualFold(list[i].Id, key) || strings.EqualFold(list[i].Name, key) ||
			list[i].Id == slug {
			return &list[i]
		}
	}
	return nil
}

// DamageTypeList is every kind of damage this ruleset knows.
func (self *Ruleset) DamageTypeList() []DamageType {
	if self != nil && len(self.DamageTypes) > 0 {
		return self.DamageTypes
	}
	return SRDDamageTypes
}

// DamageType finds one by id or by name, case insensitively.
func (self *Ruleset) DamageType(nameOrId string) *DamageType {
	key := strings.TrimSpace(nameOrId)
	if key == "" {
		return nil
	}
	slug := Slugify(key)
	list := self.DamageTypeList()
	for i := range list {
		if strings.EqualFold(list[i].Id, key) || strings.EqualFold(list[i].Name, key) ||
			list[i].Id == slug {
			return &list[i]
		}
	}
	return nil
}

// ----------------------------------------------------------------------------
// What is stored on the character
// ----------------------------------------------------------------------------

// ConditionRef is one condition a character is currently under. Level is only
// meaningful for a condition that has levels - exhaustion - where it is 1..6.
type ConditionRef struct {
	Id    string `yaml:"id" json:"id"`
	Level int    `yaml:"level" json:"level"`
}

// String writes a reference the way the property drawer stores it: the id on
// its own, or "exhaustion:3" when the condition has levels.
func (self ConditionRef) String() string {
	if self.Level > 0 {
		return self.Id + ":" + strconv.Itoa(self.Level)
	}
	return self.Id
}

// ConditionEvent is one line of the sheet's Condition History: a condition
// coming or going, or a defense being gained or lost.
type ConditionEvent struct {
	Date string `json:"date"`
	Time string `json:"time"`
	// Action is "gained", "ended", "worsened", "eased", "cleared", or one of
	// the three defense words, "resistance", "immunity", "vulnerability", or
	// "unprotected" when one of those is dropped.
	Action string `json:"action"`
	// What the action was about: a condition name, or a damage type.
	Name  string `json:"name"`
	Level int    `json:"level"`
	Notes string `json:"notes"`
}

// HasCondition reports whether a condition is currently on the character.
func (self *Character) HasCondition(id string) bool {
	for _, cd := range self.Conditions {
		if strings.EqualFold(cd.Id, id) {
			return true
		}
	}
	return false
}

// ConditionLevel is what level a levelled condition is at, 0 when it is off.
func (self *Character) ConditionLevel(id string) int {
	for _, cd := range self.Conditions {
		if strings.EqualFold(cd.Id, id) {
			if cd.Level > 0 {
				return cd.Level
			}
			return 1
		}
	}
	return 0
}

// ----------------------------------------------------------------------------
// What the sheet draws
// ----------------------------------------------------------------------------

// ConditionView is one condition as the character sheet shows it: the rules,
// whether it is on, and - for exhaustion - which level it is at.
type ConditionView struct {
	Id   string `json:"id"`
	Name string `json:"name"`
	Icon string `json:"icon"`
	Text string `json:"text"`
	On   bool   `json:"on"`
	// Level and Levels are 0 for every condition but exhaustion: Levels is
	// how many degrees it has, Level which one is in force.
	Level  int `json:"level"`
	Levels int `json:"levels"`
	// Effects are the level lines, worst last, and Note is the one in force.
	Effects []string `json:"effects"`
	Note    string   `json:"note"`
	// Label is the name with the level on it, "Exhaustion 3".
	Label string `json:"label"`
}

// DefenseView is one thing a character takes less, no, or more damage from.
// Name is what to print: the damage type's proper name when it is one, and
// otherwise whatever was written down, since a defense may be a condition
// ("immune to being frightened") or a phrase the rules have no id for.
type DefenseView struct {
	Id   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
	// Damage is true when this is one of the ruleset's damage types, which
	// is what lets the sheet colour it.
	Damage bool `json:"damage"`
}

// DefensesView is the three lists together.
type DefensesView struct {
	Resistances     []DefenseView `json:"resistances"`
	Immunities      []DefenseView `json:"immunities"`
	Vulnerabilities []DefenseView `json:"vulnerabilities"`
	// Any is false when there is nothing in any of the three, which is the
	// usual case at first level and is worth saying rather than drawing
	// three empty lists.
	Any bool `json:"any"`
	// Summary is the whole lot in one line, for a printed sheet.
	Summary string `json:"summary"`
}

// ConditionsView is what is wrong with the character, and everything that
// could be: Active is what is on, All is the whole catalog with On set, which
// is what the sheet's condition picker is drawn from.
type ConditionsView struct {
	Active []ConditionView `json:"active"`
	All    []ConditionView `json:"all"`
	Count  int             `json:"count"`
	// Summary is the active conditions in one line, "Frightened, Exhaustion 2".
	Summary string `json:"summary"`
}

// ComputeDefenses resolves the three defense lists into what the sheet draws.
func ComputeDefenses(c *Character, rs *Ruleset) DefensesView {
	out := DefensesView{
		Resistances:     []DefenseView{},
		Immunities:      []DefenseView{},
		Vulnerabilities: []DefenseView{},
	}
	if c == nil {
		return out
	}
	one := func(entry, kind string) DefenseView {
		v := DefenseView{Id: Slugify(entry), Name: entry, Kind: kind}
		if dt := rs.DamageType(entry); dt != nil {
			v.Id = dt.Id
			v.Name = dt.Name
			v.Damage = true
		} else if cd := rs.Condition(entry); cd != nil {
			v.Id = cd.Id
			v.Name = cd.Name
		}
		return v
	}
	for _, e := range c.Resistances {
		out.Resistances = append(out.Resistances, one(e, Resistance))
	}
	for _, e := range c.Immunities {
		out.Immunities = append(out.Immunities, one(e, Immunity))
	}
	for _, e := range c.Vulnerabilities {
		out.Vulnerabilities = append(out.Vulnerabilities, one(e, Vulnerability))
	}
	out.Any = len(out.Resistances)+len(out.Immunities)+len(out.Vulnerabilities) > 0
	parts := []string{}
	say := func(label string, list []DefenseView) {
		if len(list) == 0 {
			return
		}
		names := []string{}
		for _, v := range list {
			names = append(names, strings.ToLower(v.Name))
		}
		parts = append(parts, label+" "+joinList(names))
	}
	say("resistant to", out.Resistances)
	say("immune to", out.Immunities)
	say("vulnerable to", out.Vulnerabilities)
	out.Summary = strings.Join(parts, "; ")
	return out
}

// ComputeConditions resolves what the character is under against the ruleset's
// catalog. A condition the catalog has never heard of is still shown - the
// table's word beats the book's - it simply has no rules text behind it.
func ComputeConditions(c *Character, rs *Ruleset) ConditionsView {
	out := ConditionsView{Active: []ConditionView{}, All: []ConditionView{}}
	if c == nil {
		return out
	}
	view := func(cd Condition, level int, on bool) ConditionView {
		v := ConditionView{
			Id: cd.Id, Name: cd.Name, Icon: orDefault(cd.Icon, cd.Id),
			Text: cd.Text, On: on, Levels: cd.Levels, Effects: cd.Effects,
			Label: cd.Name,
		}
		if cd.Levels > 0 && on {
			if level < 1 {
				level = 1
			}
			if level > cd.Levels {
				level = cd.Levels
			}
			v.Level = level
			v.Label = fmt.Sprintf("%s %d", cd.Name, level)
			if level-1 < len(cd.Effects) {
				v.Note = cd.Effects[level-1]
			}
		}
		return v
	}

	known := map[string]bool{}
	for _, cd := range rs.ConditionList() {
		level := c.ConditionLevel(cd.Id)
		v := view(cd, level, c.HasCondition(cd.Id))
		known[cd.Id] = true
		out.All = append(out.All, v)
		if v.On {
			out.Active = append(out.Active, v)
		}
	}
	// Anything on the character the catalog does not carry, kept so a
	// homebrew condition written into the sheet by hand is not silently lost.
	for _, ref := range c.Conditions {
		if known[strings.ToLower(ref.Id)] {
			continue
		}
		v := view(Condition{Id: ref.Id, Name: Titleize(ref.Id)}, ref.Level, true)
		out.All = append(out.All, v)
		out.Active = append(out.Active, v)
	}

	out.Count = len(out.Active)
	labels := []string{}
	for _, v := range out.Active {
		labels = append(labels, v.Label)
	}
	out.Summary = joinList(labels)
	return out
}

// ----------------------------------------------------------------------------
// Changing them
// ----------------------------------------------------------------------------

// ConditionsRequest is one change to what a character is under or shrugs off,
// posted by the html character sheet.
type ConditionsRequest struct {
	Filename string `json:"filename"`
	Id       string `json:"id"`
	// Action is one of add, remove, toggle, level, clear, resist, immune,
	// vulnerable or unprotect.
	Action string `json:"action"`
	// Name is what the action is about: the condition for a condition action,
	// the damage type - or condition, or free text - for a defense one.
	Name string `json:"name"`
	// Level sets a levelled condition, which today means exhaustion. Zero on
	// a "level" action takes it off entirely.
	Level int    `json:"level"`
	Notes string `json:"notes"`
}

// ConditionsState is the answer to every conditions call: what the character
// is under, what they shrug off, and the history behind both.
type ConditionsState struct {
	Id         string           `json:"id"`
	Name       string           `json:"name"`
	Filename   string           `json:"filename"`
	Ruleset    string           `json:"ruleset"`
	Conditions ConditionsView   `json:"conditions"`
	Defenses   DefensesView     `json:"defenses"`
	History    []ConditionEvent `json:"history"`
	// DamageTypes is the ruleset's catalog, so the sheet can offer them
	// rather than carrying its own copy of a list that lives in the data.
	DamageTypes []DamageType `json:"damageTypes"`
	Msg         string       `json:"msg"`
}

// ApplyConditionChange applies one change and returns what it did. The
// character is moved on in place; writing the sheet back out is the caller's
// business, the same way it is for coin and for the inventory.
func ApplyConditionChange(c *Character, rs *Ruleset, req ConditionsRequest) (ConditionEvent, error) {
	action := strings.ToLower(strings.TrimSpace(req.Action))
	e := ConditionEvent{Action: action, Notes: strings.TrimSpace(req.Notes)}
	if c == nil {
		return e, fmt.Errorf("no character")
	}

	switch action {
	case CondAdd, CondRemove, CondToggle, CondLevel:
		return applyConditionAction(c, rs, action, req, e)

	case CondClear:
		if len(c.Conditions) == 0 {
			return e, fmt.Errorf("nothing is on you")
		}
		names := []string{}
		for _, ref := range c.Conditions {
			names = append(names, conditionName(rs, ref.Id))
		}
		c.Conditions = nil
		e.Action = "cleared"
		e.Name = joinList(names)
		return logCondition(c, e), nil

	case CondResist, CondImmune, CondVulnerabl:
		return applyDefenseAction(c, rs, action, req, e)

	case CondUnprotect:
		name, err := defenseName(rs, req.Name)
		if err != nil {
			return e, err
		}
		kind := dropDefense(c, name)
		if kind == "" {
			return e, fmt.Errorf("%s is not one of your defenses", name)
		}
		e.Action = "unprotected"
		e.Name = name
		return logCondition(c, e), nil
	}

	return e, fmt.Errorf(
		"unknown action %q, expected add, remove, toggle, level, clear, resist, "+
			"immune, vulnerable or unprotect", req.Action)
}

// applyConditionAction is the four actions that move a condition on or off.
func applyConditionAction(c *Character, rs *Ruleset, action string,
	req ConditionsRequest, e ConditionEvent) (ConditionEvent, error) {
	key := strings.TrimSpace(req.Name)
	if key == "" {
		return e, fmt.Errorf("which condition?")
	}
	id := Slugify(key)
	levels := 0
	name := Titleize(id)
	if cd := rs.Condition(key); cd != nil {
		id, name, levels = cd.Id, cd.Name, cd.Levels
	}
	on := c.HasCondition(id)
	e.Name = name

	// Toggle is add or remove, decided by what is already there. It is what
	// the sheet's picker sends, so one tap on an icon does the obvious thing.
	if action == CondToggle {
		if on {
			action = CondRemove
		} else {
			action = CondAdd
		}
	}

	switch action {
	case CondAdd:
		level := req.Level
		if levels > 0 {
			if level < 1 {
				level = 1
			}
			if level > levels {
				level = levels
			}
		} else {
			level = 0
		}
		if on {
			was := c.ConditionLevel(id)
			if levels == 0 {
				return e, fmt.Errorf("you are already %s", strings.ToLower(name))
			}
			if level == was {
				return e, fmt.Errorf("you are already at %s %d", name, was)
			}
			setCondition(c, id, level)
			e.Action = "worsened"
			if level < was {
				e.Action = "eased"
			}
			e.Level = level
			return logCondition(c, e), nil
		}
		c.Conditions = append(c.Conditions, ConditionRef{Id: id, Level: level})
		sortConditions(c)
		e.Action = "gained"
		e.Level = level
		return logCondition(c, e), nil

	case CondRemove:
		if !on {
			return e, fmt.Errorf("you are not %s", strings.ToLower(name))
		}
		dropCondition(c, id)
		e.Action = "ended"
		return logCondition(c, e), nil

	case CondLevel:
		if levels == 0 {
			return e, fmt.Errorf("%s has no levels", name)
		}
		level := req.Level
		if level > levels {
			level = levels
		}
		was := c.ConditionLevel(id)
		if level <= 0 {
			if !on {
				return e, fmt.Errorf("you are not %s", strings.ToLower(name))
			}
			dropCondition(c, id)
			e.Action = "ended"
			return logCondition(c, e), nil
		}
		if level == was {
			return e, fmt.Errorf("you are already at %s %d", name, was)
		}
		if on {
			setCondition(c, id, level)
			e.Action = "worsened"
			if level < was {
				e.Action = "eased"
			}
		} else {
			c.Conditions = append(c.Conditions, ConditionRef{Id: id, Level: level})
			sortConditions(c)
			e.Action = "gained"
		}
		e.Level = level
		return logCondition(c, e), nil
	}
	return e, fmt.Errorf("unknown action %q", action)
}

// applyDefenseAction moves one entry into one of the three defense lists. A
// damage type only belongs to one of them at a time - being both resistant and
// immune to fire says nothing the immunity does not - so it is moved rather
// than copied.
func applyDefenseAction(c *Character, rs *Ruleset, action string,
	req ConditionsRequest, e ConditionEvent) (ConditionEvent, error) {
	name, err := defenseName(rs, req.Name)
	if err != nil {
		return e, err
	}
	kind := Resistance
	switch action {
	case CondImmune:
		kind = Immunity
	case CondVulnerabl:
		kind = Vulnerability
	}
	if had := defenseKind(c, name); had == kind {
		return e, fmt.Errorf("you already have %s to %s", kind, strings.ToLower(name))
	}
	dropDefense(c, name)
	switch kind {
	case Immunity:
		c.Immunities = addUnique(c.Immunities, name)
	case Vulnerability:
		c.Vulnerabilities = addUnique(c.Vulnerabilities, name)
	default:
		c.Resistances = addUnique(c.Resistances, name)
	}
	e.Action = kind
	e.Name = name
	return logCondition(c, e), nil
}

// defenseName resolves what was asked for onto the name that gets stored: a
// damage type or a condition keeps the book's spelling, and anything else is
// kept as it was typed so a phrase the rules have no id for still works.
func defenseName(rs *Ruleset, asked string) (string, error) {
	name := strings.TrimSpace(asked)
	if name == "" {
		return "", fmt.Errorf("against what?")
	}
	if dt := rs.DamageType(name); dt != nil {
		return dt.Name, nil
	}
	if cd := rs.Condition(name); cd != nil {
		return cd.Name, nil
	}
	return name, nil
}

// defenseKind says which list an entry is already in, empty for none.
func defenseKind(c *Character, name string) string {
	if containsStr(c.Resistances, name) {
		return Resistance
	}
	if containsStr(c.Immunities, name) {
		return Immunity
	}
	if containsStr(c.Vulnerabilities, name) {
		return Vulnerability
	}
	return ""
}

// dropDefense takes an entry out of all three lists and says which one it was
// actually in.
func dropDefense(c *Character, name string) string {
	kind := defenseKind(c, name)
	without := func(list []string) []string {
		out := []string{}
		for _, v := range list {
			if !strings.EqualFold(strings.TrimSpace(v), strings.TrimSpace(name)) {
				out = append(out, v)
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	}
	c.Resistances = without(c.Resistances)
	c.Immunities = without(c.Immunities)
	c.Vulnerabilities = without(c.Vulnerabilities)
	return kind
}

func setCondition(c *Character, id string, level int) {
	for i := range c.Conditions {
		if strings.EqualFold(c.Conditions[i].Id, id) {
			c.Conditions[i].Level = level
			return
		}
	}
}

func dropCondition(c *Character, id string) {
	out := []ConditionRef{}
	for _, ref := range c.Conditions {
		if !strings.EqualFold(ref.Id, id) {
			out = append(out, ref)
		}
	}
	if len(out) == 0 {
		out = nil
	}
	c.Conditions = out
}

// sortConditions keeps the list alphabetical so the property drawer does not
// churn every time something is added and taken off again.
func sortConditions(c *Character) {
	sort.SliceStable(c.Conditions, func(i, j int) bool {
		return c.Conditions[i].Id < c.Conditions[j].Id
	})
}

func conditionName(rs *Ruleset, id string) string {
	if cd := rs.Condition(id); cd != nil {
		return cd.Name
	}
	return Titleize(id)
}

// logCondition stamps an event with the time and appends it to the character's
// condition history, which is what gets written into the Condition History
// section of the org sheet.
func logCondition(c *Character, e ConditionEvent) ConditionEvent {
	now := time.Now()
	e.Date = now.Format("2006-01-02")
	e.Time = now.Format("15:04")
	c.ConditionLog = append(c.ConditionLog, e)
	return e
}

// ConditionEventMsg is the one line the sheet shows after a change, and the
// line the play session log gets.
func ConditionEventMsg(e ConditionEvent) string {
	what := strings.ToLower(e.Name)
	switch e.Action {
	case "gained":
		if e.Level > 0 {
			return fmt.Sprintf("%s %d", e.Name, e.Level)
		}
		return what
	case "ended":
		return "no longer " + what
	case "worsened":
		return fmt.Sprintf("%s worsened to %d", e.Name, e.Level)
	case "eased":
		return fmt.Sprintf("%s eased to %d", e.Name, e.Level)
	case "cleared":
		return "clear of " + what
	case Resistance, Immunity, Vulnerability:
		return e.Action + " to " + what
	case "unprotected":
		return "no longer protected against " + what
	}
	return strings.TrimSpace(e.Action + " " + what)
}

// ConditionEventLine is the same thing as org markup, for the session notes.
func ConditionEventLine(e ConditionEvent) string {
	msg := ConditionEventMsg(e)
	if e.Notes != "" {
		msg += " - " + e.Notes
	}
	switch e.Action {
	case "gained", "worsened":
		return "*Condition.* Now " + msg + "."
	case "ended", "cleared":
		return "*Condition.* " + strings.ToUpper(msg[:1]) + msg[1:] + "."
	}
	return "*Defenses.* " + strings.ToUpper(msg[:1]) + msg[1:] + "."
}
