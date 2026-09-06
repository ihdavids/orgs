package dnd

/* SDOC: DnD
* Managing Spells

  The spell list on a character sheet is not a free text field: a class knows
  or prepares a fixed number of spells, drawn from its own list, and none of
  them above the level it has slots for. The html sheet's Manage Spells panel
  edits that list live - and this is the engine behind it, so the terminal
  client and anything else asking the server get the same answer about what
  may be taken and what is already taken.

  How a class holds its spells decides what the panel can do:

  - a *known* caster (bard, sorcerer, ranger, warlock) has one budget. Every
    spell it knows is always available, so taking a spell is the whole of it.
  - a *list* preparer (cleric, druid, paladin) prepares from the entire class
    list every morning. Its budget is the prepared count, and a spell on the
    sheet is a spell prepared today.
  - a *spellbook* caster (wizard) has two budgets: the spells written in the
    book, and the smaller number prepared out of it. Taking a spell writes it
    into the book, and preparing it is a second, separate step.

  Cantrips are their own budget in all three cases and are always prepared.

  Spells a subclass hands out - cleric domain spells, oath spells - carry the
  subclass as their =Source=. They are always prepared, never count against a
  budget, and cannot be given back, because the character did not choose them
  in the first place.
EDOC */

import (
	"fmt"
	"sort"
	"strings"
)

// The actions a spellbook change is asked for under.
const (
	SpellLearn     = "learn"
	SpellForget    = "forget"
	SpellPrepare   = "prepare"
	SpellUnprepare = "unprepare"
)

// How a class holds its spells, which is what decides whether preparing is a
// separate step from learning.
const (
	// BookKnown is a fixed list of known spells, all of them always available.
	BookKnown = "known"
	// BookList prepares from the whole class list: on the sheet is prepared.
	BookList = "list"
	// BookSpellbook writes spells into a book and prepares a subset of it.
	BookSpellbook = "spellbook"
)

// SpellbookRequest is one change to a character's spells, posted by the html
// sheet. Spell is a spell id, or a name for one the ruleset does not know.
type SpellbookRequest struct {
	Filename string `json:"filename"`
	Id       string `json:"id"`
	Action   string `json:"action"`
	Spell    string `json:"spell"`
	Name     string `json:"name"`
	Notes    string `json:"notes"`
}

// SpellbookState is the answer to every spellbook call: the character's
// spells as they now stand, and the budgets they are held against.
type SpellbookState struct {
	Id       string        `json:"id"`
	Name     string        `json:"name"`
	Filename string        `json:"filename"`
	Ruleset  string        `json:"ruleset"`
	Book     SpellbookView `json:"book"`
	Msg      string        `json:"msg"`
}

// SpellbookView is everything the manage spells panel draws: the budgets, the
// spells held against them, and the spell page as the sheet shows it, so one
// answer redraws both the panel and the sheet behind it.
type SpellbookView struct {
	IsCaster bool `json:"isCaster"`
	// Mode is known, list or spellbook - how this class holds its spells.
	Mode string `json:"mode"`
	// TwoStage is set when preparing is a step of its own, which is what a
	// spellbook caster does and nobody else.
	TwoStage bool `json:"twoStage"`

	ClassId   string `json:"classId"`
	ClassName string `json:"className"`
	// Lists are the spell lists the character draws from, one per casting
	// class. A multiclassed caster picks from all of them.
	Lists []string `json:"lists"`

	CastingAbility string `json:"castingAbility"`
	SpellSaveDC    int    `json:"spellSaveDc"`
	SpellAttack    string `json:"spellAttack"`
	// MaxLevel is the highest spell level the character has slots for, and so
	// the highest one that may be taken.
	MaxLevel int    `json:"maxLevel"`
	Notes    string `json:"notes"`

	Allotments []SpellAllotment `json:"allotments"`
	Slots      []SpellSlotView  `json:"slots"`
	// Levels is the character's own spells, grouped as the sheet groups them.
	Levels []SpellLevelView `json:"levels"`
}

// SpellAllotment is one budget: how many of something the class gets at this
// level and how many are spoken for.
type SpellAllotment struct {
	// Kind is cantrips, known or prepared.
	Kind string `json:"kind"`
	Name string `json:"name"`
	Used int    `json:"used"`
	Max  int    `json:"max"`
	Left int    `json:"left"`
	Full bool   `json:"full"`
	Note string `json:"note"`
}

// SpellMatch is one spell offered by the picker, with what the character has
// already done about it and whether they may do any more.
type SpellMatch struct {
	Id            string   `json:"id"`
	Name          string   `json:"name"`
	Level         int      `json:"level"`
	LevelName     string   `json:"levelName"`
	School        string   `json:"school"`
	CastingTime   string   `json:"castingTime"`
	Range         string   `json:"range"`
	Components    string   `json:"components"`
	Duration      string   `json:"duration"`
	Concentration bool     `json:"concentration"`
	Ritual        bool     `json:"ritual"`
	Classes       []string `json:"classes"`
	Text          string   `json:"text"`
	HigherLevel   string   `json:"higherLevel"`
	Summary       string   `json:"summary"`

	// Known is set when the character has the spell at all, Prepared when it
	// is ready to cast, and Source names the subclass that granted it.
	Known    bool   `json:"known"`
	Prepared bool   `json:"prepared"`
	Source   string `json:"source"`
	// Granted marks a spell the character was given and cannot give back.
	Granted bool `json:"granted"`
	// Expanded marks a spell a subclass added to the list to choose from.
	Expanded bool `json:"expanded"`

	// CanLearn, CanForget and CanPrepare are what the picker may offer right
	// now. Why says what stops it when it may not.
	CanLearn   bool   `json:"canLearn"`
	CanForget  bool   `json:"canForget"`
	CanPrepare bool   `json:"canPrepare"`
	Why        string `json:"why"`
}

// ----------------------------------------------------------------------------
// The view
// ----------------------------------------------------------------------------

// Spellbook works out how this character holds its spells, what it is allowed
// and what it has spent, and hands back the spell page along with it.
func Spellbook(c *Character, rs *Ruleset) SpellbookView {
	v := SpellbookView{Allotments: []SpellAllotment{}, Lists: []string{},
		Slots: []SpellSlotView{}, Levels: []SpellLevelView{}}
	if c == nil || rs == nil {
		return v
	}
	sheet := Compute(c, rs)
	v.IsCaster = sheet.IsCaster
	v.CastingAbility = sheet.CastingAbility
	v.SpellSaveDC = sheet.SpellSaveDC
	v.SpellAttack = sheet.SpellAttackStr
	v.Notes = sheet.SpellNotes
	v.Slots = sheet.Slots
	v.Levels = sheet.SpellLevels
	v.Lists = SpellLists(c, rs)

	sc := castingInfo(rs, c)
	if sc == nil {
		// A character with no spellcasting class can still know a spell or
		// two - a racial cantrip - and there is no budget to hold them to.
		return v
	}
	if cls := castingClass(rs, c); cls != nil {
		v.ClassId = cls.Id
		v.ClassName = cls.Name
	}
	v.Mode = bookMode(sc)
	v.TwoStage = v.Mode == BookSpellbook
	v.MaxLevel = maxSpellLevel(rs, c)
	v.Allotments = spellAllotments(c, rs, sc, sheet)
	return v
}

// bookMode is which of the three ways this class holds its spells.
func bookMode(sc *Spellcasting) string {
	if !sc.Prepares {
		return BookKnown
	}
	if sc.PreparedFrom == "spellbook" {
		return BookSpellbook
	}
	return BookList
}

// spellAllotments is every budget the class has at this level. Cantrips are
// always one; the other depends on whether the class knows a fixed list or
// prepares one, and a spellbook caster has both.
func spellAllotments(c *Character, rs *Ruleset, sc *Spellcasting, sheet *Sheet) []SpellAllotment {
	out := []SpellAllotment{}
	add := func(kind, name string, used, max int, note string) {
		if max <= 0 {
			return
		}
		left := max - used
		if left < 0 {
			left = 0
		}
		out = append(out, SpellAllotment{Kind: kind, Name: name, Used: used,
			Max: max, Left: left, Full: used >= max, Note: note})
	}
	add("cantrips", "Cantrips", CountSpells(c, 0, false), cantripCount(rs, c, sc),
		"cast at will, never using a slot")

	mode := bookMode(sc)
	if len(sc.SpellsKnown) > 0 {
		name, note := "Spells known", "always ready to cast"
		if mode == BookSpellbook {
			name, note = "Spellbook", "written in your book, prepare them below"
		}
		add("known", name, CountSpells(c, -1, false), spellsKnownMax(rs, c, sc), note)
	}
	if sc.Prepares && sheet.PreparedMax > 0 {
		note := "prepared after a long rest"
		if mode == BookList {
			note = "prepared from the whole " + orDefault(sc.SpellList, "class") + " list"
		}
		add("prepared", "Prepared", CountSpells(c, -1, true), sheet.PreparedMax, note)
	}
	return out
}

// spellsKnownMax reads the class's spells known table at this class level.
func spellsKnownMax(rs *Ruleset, c *Character, sc *Spellcasting) int {
	if len(sc.SpellsKnown) == 0 {
		return 0
	}
	lvl := 0
	if cls := castingClass(rs, c); cls != nil {
		lvl = classLevelOf(c, cls.Id)
	}
	if len(sc.SpellsKnown) > lvl {
		return sc.SpellsKnown[lvl]
	}
	return sc.SpellsKnown[len(sc.SpellsKnown)-1]
}

// CountSpells counts what the character has chosen for itself. level 0 counts
// cantrips and -1 counts every levelled spell; preparedOnly counts only the
// ones ready to cast. Spells a subclass granted are never counted: they were
// not chosen and do not come out of an allowance.
func CountSpells(c *Character, level int, preparedOnly bool) int {
	n := 0
	for _, ks := range c.Spells {
		if ks.Source != "" {
			continue
		}
		if level == 0 && ks.Level != 0 {
			continue
		}
		if level < 0 && ks.Level == 0 {
			continue
		}
		if preparedOnly && !ks.Prepared {
			continue
		}
		n++
	}
	return n
}

// SpellLists is every spell list the character may draw from, one per casting
// class. A bard who took a level of cleric picks from both.
func SpellLists(c *Character, rs *Ruleset) []string {
	out := []string{}
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
		list := cls.Id
		if info.SpellList != "" {
			list = info.SpellList
		}
		if !containsStr(out, list) {
			out = append(out, list)
		}
	}
	return out
}

// ----------------------------------------------------------------------------
// The picker
// ----------------------------------------------------------------------------

// SearchSpells is every spell this character could have, answered for the
// manage spells panel: the class lists, plus anything a subclass widened them
// with, plus whatever the character already knows plus any spell already on the
// sheet from somewhere else. Each one says what the character has done about
// it and what it is still allowed to do.
//
// An empty query returns the whole list, because the panel groups and filters
// it locally - by level, by school, by whether it is already taken - and
// regrouping should not need another round trip.
func SearchSpells(c *Character, rs *Ruleset, query string, limit int) []SpellMatch {
	if rs == nil {
		return []SpellMatch{}
	}
	if limit <= 0 {
		limit = 500
	}
	pool, expanded := spellPool(c, rs)
	labels, names := make([]string, len(pool)), make([]string, len(pool))
	for i, sp := range pool {
		names[i] = sp.Name
		labels[i] = sp.Name + " " + sp.LevelString() + " " + sp.School + " " +
			sp.CastingTime + " " + sp.Duration + " " + ritualWords(sp)
	}
	book := Spellbook(c, rs)
	known := map[string]*KnownSpell{}
	if c != nil {
		for i := range c.Spells {
			known[c.Spells[i].Id] = &c.Spells[i]
		}
	}
	out := []SpellMatch{}
	for _, idx := range FuzzyMatches(query, labels, names) {
		if len(out) >= limit {
			break
		}
		out = append(out, spellMatch(pool[idx], known[pool[idx].Id], expanded[pool[idx].Id], &book))
	}
	if strings.TrimSpace(query) == "" {
		sort.SliceStable(out, func(a, b int) bool {
			if out[a].Level != out[b].Level {
				return out[a].Level < out[b].Level
			}
			return out[a].Name < out[b].Name
		})
	}
	return out
}

// ritualWords puts the tags into the text the filter searches, so typing
// "ritual" or "concentration" narrows the list to them.
func ritualWords(sp *Spell) string {
	out := []string{}
	if sp.Ritual {
		out = append(out, "ritual")
	}
	if sp.Concentration {
		out = append(out, "concentration")
	}
	return strings.Join(out, " ")
}

// spellPool is every spell the character may choose from, and which of them a
// subclass added to the list rather than the class itself.
func spellPool(c *Character, rs *Ruleset) ([]*Spell, map[string]bool) {
	expanded := map[string]bool{}
	seen := map[string]bool{}
	pool := []*Spell{}
	push := func(sp *Spell) {
		if sp == nil || seen[sp.Id] {
			return
		}
		seen[sp.Id] = true
		pool = append(pool, sp)
	}
	if c != nil {
		for _, list := range SpellLists(c, rs) {
			for _, sp := range rs.SpellsForClass(list, -1) {
				push(sp)
			}
		}
		for _, sp := range expandedSpellsFor(rs, c) {
			expanded[sp.Id] = true
			push(sp)
		}
		// Whatever is already on the sheet belongs in the picker even when it
		// is on nobody's list - a racial cantrip, a scroll copied into a
		// spellbook - or it could never be given back.
		for _, ks := range c.Spells {
			push(rs.Spell(ks.Id))
		}
	}
	sort.SliceStable(pool, func(a, b int) bool {
		if pool[a].Level != pool[b].Level {
			return pool[a].Level < pool[b].Level
		}
		return pool[a].Name < pool[b].Name
	})
	return pool, expanded
}

// spellMatch says what the character has done about one spell and what the
// picker may still offer for it.
func spellMatch(sp *Spell, ks *KnownSpell, expanded bool, book *SpellbookView) SpellMatch {
	m := SpellMatch{
		Id: sp.Id, Name: sp.Name, Level: sp.Level, LevelName: SpellLevelName(sp.Level),
		School: sp.School, CastingTime: sp.CastingTime, Range: sp.Range,
		Components: sp.Components, Duration: sp.Duration,
		Concentration: sp.Concentration, Ritual: sp.Ritual, Classes: sp.Classes,
		Text: sp.Text, HigherLevel: sp.HigherLevel, Summary: firstSentence(sp.Text),
		Expanded: expanded,
	}
	if ks != nil {
		m.Known = true
		m.Prepared = ks.Prepared
		m.Source = ks.Source
		m.Granted = ks.Source != ""
	}
	if m.Granted {
		m.Why = "granted by " + m.Source
		return m
	}
	if !m.Known {
		if err := canLearn(sp, book); err != nil {
			m.Why = err.Error()
		} else {
			m.CanLearn = true
		}
		return m
	}
	m.CanForget = true
	// Only a spellbook caster prepares as a step of its own; for everyone
	// else a spell on the sheet is a spell ready to cast.
	if book.TwoStage && sp.Level > 0 {
		if m.Prepared {
			m.CanPrepare = true
		} else if err := canPrepare(book); err != nil {
			m.Why = err.Error()
		} else {
			m.CanPrepare = true
		}
	}
	return m
}

// SpellLevelName is the heading a spell level is written under, matching what
// the org sheet uses so the two agree.
func SpellLevelName(level int) string {
	if level <= 0 {
		return "Cantrips"
	}
	return Ordinal(level) + " Level"
}

// ----------------------------------------------------------------------------
// Changing what the character holds
// ----------------------------------------------------------------------------

// LearnSpell adds a spell to the character's list, after checking that it is
// on a list they can draw from, of a level they have slots for, and that they
// have room left in the budget it comes out of.
func LearnSpell(c *Character, rs *Ruleset, id string) (string, error) {
	sp, err := findSpell(c, rs, id)
	if err != nil {
		return "", err
	}
	if hasSpell(c.Spells, sp.Id) {
		return "", fmt.Errorf("%s is already on your sheet", sp.Name)
	}
	book := Spellbook(c, rs)
	if err := canLearn(sp, &book); err != nil {
		return "", err
	}
	ks := KnownSpell{Id: sp.Id, Name: sp.Name, Level: sp.Level}
	// Written into a spellbook is not the same as prepared; anywhere else,
	// having the spell is having it ready.
	ks.Prepared = sp.Level == 0 || !book.TwoStage
	c.Spells = append(c.Spells, ks)
	sortSpells(c)
	return learnedMsg(sp, &book), nil
}

func learnedMsg(sp *Spell, book *SpellbookView) string {
	switch {
	case sp.Level == 0:
		return "learned " + sp.Name
	case book.Mode == BookSpellbook:
		return "wrote " + sp.Name + " into your spellbook"
	case book.Mode == BookList:
		return "prepared " + sp.Name
	}
	return "learned " + sp.Name
}

// canLearn is the whole of what stops a spell being taken: the level it is,
// and the budget it would come out of.
func canLearn(sp *Spell, book *SpellbookView) error {
	if sp.Level > 0 && book.MaxLevel > 0 && sp.Level > book.MaxLevel {
		return fmt.Errorf("you have no %s slots yet", Ordinal(sp.Level))
	}
	kind := "known"
	if sp.Level == 0 {
		kind = "cantrips"
	} else if book.Mode == BookList {
		kind = "prepared"
	}
	a := book.Allotment(kind)
	if a == nil {
		// No budget of that kind is no reason to refuse: a class the ruleset
		// says nothing about is the ruleset's business, not ours.
		return nil
	}
	if a.Full {
		return fmt.Errorf("your %d %s are all spoken for - give one back first",
			a.Max, strings.ToLower(a.Name))
	}
	return nil
}

// canPrepare is whether there is room to prepare one more spell.
func canPrepare(book *SpellbookView) error {
	a := book.Allotment("prepared")
	if a == nil || !a.Full {
		return nil
	}
	return fmt.Errorf("you have %d spells prepared already - unprepare one first", a.Max)
}

// Allotment finds one budget by kind, or nil when the class has no such thing.
func (v *SpellbookView) Allotment(kind string) *SpellAllotment {
	for i := range v.Allotments {
		if v.Allotments[i].Kind == kind {
			return &v.Allotments[i]
		}
	}
	return nil
}

// ForgetSpell takes a spell off the character's sheet. A spell a subclass
// granted is not the character's to give back.
func ForgetSpell(c *Character, rs *Ruleset, id string) (string, error) {
	idx := knownSpellIndex(c, rs, id)
	if idx < 0 {
		return "", fmt.Errorf("%s is not on your sheet", spellLabel(rs, id))
	}
	ks := c.Spells[idx]
	if ks.Source != "" {
		return "", fmt.Errorf("%s was granted by %s and cannot be given back",
			ks.Name, ks.Source)
	}
	c.Spells = append(c.Spells[:idx], c.Spells[idx+1:]...)
	return "gave back " + ks.Name, nil
}

// PrepareSpell readies a spell already written in the spellbook, or puts it
// back. This is only a step of its own for a class that prepares from a book;
// anywhere else a spell on the sheet is already prepared, and the way to stop
// having it is to give it back.
func PrepareSpell(c *Character, rs *Ruleset, id string, prepare bool) (string, error) {
	idx := knownSpellIndex(c, rs, id)
	if idx < 0 {
		return "", fmt.Errorf("%s is not on your sheet", spellLabel(rs, id))
	}
	ks := &c.Spells[idx]
	if ks.Source != "" {
		return "", fmt.Errorf("%s is granted by %s and is always prepared", ks.Name, ks.Source)
	}
	if ks.Level == 0 {
		return "", fmt.Errorf("cantrips are always ready, there is nothing to prepare")
	}
	book := Spellbook(c, rs)
	if !book.TwoStage {
		return "", fmt.Errorf(
			"your spells are ready as soon as you have them - give one back to make room")
	}
	if prepare {
		if ks.Prepared {
			return "", fmt.Errorf("%s is already prepared", ks.Name)
		}
		if err := canPrepare(&book); err != nil {
			return "", err
		}
		ks.Prepared = true
		return "prepared " + ks.Name, nil
	}
	if !ks.Prepared {
		return "", fmt.Errorf("%s is not prepared", ks.Name)
	}
	ks.Prepared = false
	return "unprepared " + ks.Name, nil
}

// ApplySpellChange runs one request against a character, and is the single
// place the actions are named.
func ApplySpellChange(c *Character, rs *Ruleset, action, spell string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case SpellLearn, "add", "equip":
		return LearnSpell(c, rs, spell)
	case SpellForget, "remove", "unequip", "drop":
		return ForgetSpell(c, rs, spell)
	case SpellPrepare:
		return PrepareSpell(c, rs, spell, true)
	case SpellUnprepare:
		return PrepareSpell(c, rs, spell, false)
	}
	return "", fmt.Errorf("unknown action %q, expected learn, forget, prepare or unprepare", action)
}

// ----------------------------------------------------------------------------
// helpers
// ----------------------------------------------------------------------------

// findSpell resolves an id or a name against the ruleset, and refuses a spell
// no list of the character's offers.
func findSpell(c *Character, rs *Ruleset, id string) (*Spell, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("which spell?")
	}
	sp := rs.Spell(id)
	if sp == nil {
		sp = rs.Spell(Slugify(id))
	}
	if sp == nil {
		return nil, fmt.Errorf("no spell called %q in this ruleset", id)
	}
	pool, _ := spellPool(c, rs)
	for _, p := range pool {
		if p.Id == sp.Id {
			return sp, nil
		}
	}
	return nil, fmt.Errorf("%s is not on any spell list you can draw from", sp.Name)
}

// knownSpellIndex finds a spell on the character's own list, by id or by name.
func knownSpellIndex(c *Character, rs *Ruleset, id string) int {
	id = strings.TrimSpace(id)
	if id == "" || c == nil {
		return -1
	}
	want := id
	if rs != nil {
		if sp := rs.Spell(id); sp != nil {
			want = sp.Id
		}
	}
	slug := Slugify(id)
	for i, ks := range c.Spells {
		if ks.Id == want || ks.Id == id || ks.Id == slug || strings.EqualFold(ks.Name, id) {
			return i
		}
	}
	return -1
}

// spellLabel is the best name we have for a spell in an error message.
func spellLabel(rs *Ruleset, id string) string {
	if rs != nil {
		if sp := rs.Spell(id); sp != nil {
			return sp.Name
		}
	}
	return id
}

// sortSpells keeps the list in the order the sheet writes it, so a spell
// added now lands in its own level's table rather than at the end.
func sortSpells(c *Character) {
	sort.SliceStable(c.Spells, func(i, j int) bool { return c.Spells[i].Level < c.Spells[j].Level })
}
