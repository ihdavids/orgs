package dnd

/* SDOC: DnD
* The Interactive Builder

  Character creation is a server side state machine. The client never needs to
  know any rules: it asks for the next =Prompt=, shows the options (each of
  which carries a summary, a detail blurb and a "recommended" flag based on
  what has been chosen so far) and posts an =Answer= back.

  Because every answer is stored, going *back* simply drops the later answers
  and replays the remaining ones onto a fresh character, so changing your
  class half way through cannot leave stale proficiencies behind.

  The step list itself is generated from the ruleset, so a module that adds a
  class with a new "choose one of" block automatically gains a prompt for it.
EDOC */

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Session is one in progress character build.
type Session struct {
	Id        string             `json:"id"`
	Owner     string             `json:"owner"`
	Ruleset   string             `json:"ruleset"`
	Created   time.Time          `json:"created"`
	Updated   time.Time          `json:"updated"`
	Char      *Character         `json:"character"`
	Answers   map[string]*Answer `json:"answers"`
	Order     []string           `json:"order"`
	Pool      []int              `json:"pool"`
	Method    string             `json:"method"`
	Filename  string             `json:"filename"`
	Done      bool               `json:"done"`
	Rulesets  []Option           `json:"rulesets"`
	StartName string             `json:"startName"`
	Player    string             `json:"player"`
	Level     int                `json:"level"`
	// RulesetChosen is set when the caller named a ruleset up front, which
	// means the ruleset prompt is skipped.
	RulesetChosen bool `json:"rulesetChosen"`
	// ReplayErrors records answers that stopped applying cleanly after an
	// earlier answer changed (for example skills that no longer exist after
	// you go back and switch class).
	ReplayErrors []string `json:"replayErrors,omitempty"`
}

// NewSession creates a build session.
func NewSession(id string, req *NewSessionRequest, rulesets []Option) *Session {
	s := &Session{
		Id: id, Ruleset: orDefault(req.Ruleset, DefaultRuleset),
		Created: time.Now(), Updated: time.Now(),
		Answers: map[string]*Answer{}, Rulesets: rulesets,
		StartName: req.Name, Player: req.Player, Level: req.Level,
		Filename: req.Filename, RulesetChosen: strings.TrimSpace(req.Ruleset) != "",
	}
	s.reset()
	return s
}

func (s *Session) reset() {
	s.Char = &Character{
		Ruleset:   s.Ruleset,
		Abilities: map[string]int{},
		Choices:   map[string][]string{},
		Name:      s.StartName,
		Player:    s.Player,
	}
	if s.Level > 0 {
		s.Char.Classes = []ClassLevel{{Level: s.Level}}
	}
}

// stepDef is one question in the flow.
type stepDef struct {
	id    string
	build func(*Session, *Ruleset) *Prompt
	apply func(*Session, *Ruleset, *Answer) error
	// optional indicates the step may be skipped
	optional bool
}

// ----------------------------------------------------------------------------
// Public API
// ----------------------------------------------------------------------------

// Next returns the next unanswered prompt, or the final review prompt.
func (s *Session) Next(rs *Ruleset) *Prompt {
	steps := s.steps(rs)
	for i, st := range steps {
		if _, ok := s.Answers[st.id]; ok {
			continue
		}
		p := st.build(s, rs)
		if p == nil {
			// nothing to ask (e.g. no valid options), auto answer and continue
			s.Answers[st.id] = &Answer{Step: st.id, Skip: true}
			s.Order = append(s.Order, st.id)
			continue
		}
		p.Session = s.Id
		p.Step = st.id
		p.AllowSkip = p.AllowSkip || st.optional
		p.Progress = Progress{Step: i + 1, Total: len(steps)}
		for _, prev := range s.Order {
			p.Progress.Completed = append(p.Progress.Completed, prev)
		}
		p.Summary = s.summary(rs)
		return p
	}
	return s.donePrompt(rs)
}

// Apply validates and applies an answer, then returns the following prompt.
func (s *Session) Apply(rs *Ruleset, ans *Answer) (*Prompt, error) {
	if ans.Back {
		s.back()
		return s.Next(rs), nil
	}
	steps := s.steps(rs)
	var target *stepDef
	for i := range steps {
		if steps[i].id == ans.Step {
			target = &steps[i]
			break
		}
	}
	if target == nil {
		return nil, fmt.Errorf("unknown step %q", ans.Step)
	}
	// Answering an earlier step throws away everything after it.
	if _, done := s.Answers[target.id]; done {
		s.truncateAt(target.id)
	}
	if ans.Random {
		s.randomize(rs, target, ans)
	}
	if !ans.Skip {
		if err := target.apply(s, rs, ans); err != nil {
			return nil, err
		}
	} else if !target.optional {
		return nil, fmt.Errorf("step %q cannot be skipped", ans.Step)
	}
	s.Answers[target.id] = ans
	s.Order = append(s.Order, target.id)
	s.Updated = time.Now()
	s.replay(rs)
	return s.Next(rs), nil
}

// Finish completes the character (hit points, currency, defaults).
func (s *Session) Finish(rs *Ruleset) *Character {
	c := s.Char
	if len(c.Classes) == 0 {
		c.Classes = []ClassLevel{{Class: "", Level: 1}}
	}
	if c.HPMax <= 0 {
		c.HPMax = DefaultHitPoints(c, rs)
	}
	c.HPCurrent = c.HPMax
	if c.XP == 0 {
		c.XP = XPForLevel(c.TotalLevel())
	}
	if c.DeathSaves == "" {
		c.DeathSaves = "0/0"
	}
	if c.Name == "" {
		c.Name = "Unnamed Adventurer"
	}
	MarkPrepared(c, rs)
	s.Done = true
	return c
}

func (s *Session) back() {
	if len(s.Order) == 0 {
		return
	}
	last := s.Order[len(s.Order)-1]
	s.truncateAt(last)
}

func (s *Session) truncateAt(id string) {
	idx := -1
	for i, v := range s.Order {
		if v == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return
	}
	for _, v := range s.Order[idx:] {
		delete(s.Answers, v)
	}
	s.Order = s.Order[:idx]
	s.Done = false
}

// replay rebuilds the character from scratch out of the stored answers.
func (s *Session) replay(rs *Ruleset) {
	answers := s.Answers
	order := append([]string{}, s.Order...)
	s.reset()
	s.Answers = map[string]*Answer{}
	s.Order = []string{}
	s.ReplayErrors = nil
	for _, id := range order {
		ans := answers[id]
		if ans == nil {
			continue
		}
		steps := s.steps(rs)
		var target *stepDef
		for i := range steps {
			if steps[i].id == id {
				target = &steps[i]
				break
			}
		}
		if target == nil {
			continue
		}
		if !ans.Skip {
			if err := target.apply(s, rs, ans); err != nil {
				s.ReplayErrors = append(s.ReplayErrors,
					fmt.Sprintf("%s: %s", id, err.Error()))
			}
		}
		s.Answers[id] = ans
		s.Order = append(s.Order, id)
	}
}

func (s *Session) summary(rs *Ruleset) string {
	c := s.Char
	parts := []string{}
	if c.Name != "" {
		parts = append(parts, c.Name)
	}
	if c.Race != "" {
		n := c.Race
		if c.Subrace != "" {
			n = c.Subrace
		}
		if r := rs.Race(n); r != nil {
			n = r.Name
		}
		parts = append(parts, n)
	}
	for _, cl := range c.Classes {
		if cl.Class == "" {
			continue
		}
		n := cl.Class
		if cls := rs.Class(cl.Class); cls != nil {
			n = cls.Name
		}
		parts = append(parts, fmt.Sprintf("%s %d", n, cl.Level))
	}
	if c.Background != "" {
		if bg := rs.Background(c.Background); bg != nil {
			parts = append(parts, bg.Name)
		}
	}
	return strings.Join(parts, " · ")
}

func (s *Session) donePrompt(rs *Ruleset) *Prompt {
	c := s.Finish(rs)
	sheet := Compute(c, rs)
	return &Prompt{
		Session: s.Id, Step: "done", Kind: "done", Done: true,
		Title:    "Character Complete",
		Question: fmt.Sprintf("%s is ready to play.", c.Name),
		Summary:  s.summary(rs),
		Sheet:    sheet,
		Org:      RenderOrg(c, rs),
		Filename: s.Filename,
		Advice:   finishAdvice(sheet),
	}
}

func finishAdvice(s *Sheet) []string {
	out := []string{
		fmt.Sprintf("AC %d, %d hit points, initiative %s, speed %d ft.",
			s.AC, s.HPMax, s.InitiativeStr, s.Speed),
	}
	if s.IsCaster {
		out = append(out, fmt.Sprintf("Spell save DC %d, spell attack %s.", s.SpellSaveDC, s.SpellAttackStr))
	}
	out = append(out, s.Warnings...)
	return out
}

// ----------------------------------------------------------------------------
// Step list
// ----------------------------------------------------------------------------

func (s *Session) steps(rs *Ruleset) []stepDef {
	steps := []stepDef{}
	if len(s.Rulesets) > 1 && !s.RulesetChosen {
		steps = append(steps, stepRuleset())
	}
	steps = append(steps, stepName(), stepClass(), stepLevel())

	c := s.Char
	primary := c.PrimaryClass()
	cls := rs.Class(primary.Class)
	if cls != nil && len(cls.Subclasses) > 0 && primary.Level >= maxInt(cls.SubclassLevel, 1) {
		steps = append(steps, stepSubclass())
	}
	steps = append(steps, stepRace())
	if race := rs.Race(c.Race); race != nil && len(race.Subraces) > 0 {
		steps = append(steps, stepSubrace())
	}
	if race := activeRace(rs, c); race != nil && race.AbilityChoice != nil {
		steps = append(steps, stepRaceAbilityChoice(race))
	}
	steps = append(steps, stepBackground(), stepAbilityMethod(), stepAbilities())
	// Ability score improvements, one prompt per class level that grants one.
	for _, cl := range c.Classes {
		acls := rs.Class(cl.Class)
		if acls == nil {
			continue
		}
		for _, lvl := range acls.ASILevels {
			if lvl <= cl.Level {
				steps = append(steps, stepASI(cl.Class, lvl))
			}
		}
	}
	if cls != nil {
		steps = append(steps, stepSkills(cls))
	}
	// generic choices coming from race, class, subclass and background
	for _, ch := range collectChoices(rs, c) {
		steps = append(steps, stepChoice(ch))
	}
	if extra := extraLanguageCount(rs, c); extra > 0 {
		steps = append(steps, stepLanguages(extra))
	}
	if cls != nil {
		for i := range cls.Equipment {
			steps = append(steps, stepEquipment(cls, i))
		}
	}
	if sc := castingInfo(rs, c); sc != nil {
		if n := cantripCount(rs, c, sc); n > 0 {
			steps = append(steps, stepSpells(0, n, "cantrips"))
		}
		if n, kind := spellCount(rs, c, sc); n > 0 {
			steps = append(steps, stepSpells(-1, n, kind))
		}
	}
	steps = append(steps, stepAlignment(), stepPersonality("personality", "Personality Trait"),
		stepPersonality("ideals", "Ideal"), stepPersonality("bonds", "Bond"),
		stepPersonality("flaws", "Flaw"), stepDetails(), stepBackstory())
	return steps
}

func activeRace(rs *Ruleset, c *Character) *Race {
	if c.Subrace != "" {
		if r := rs.Race(c.Subrace); r != nil {
			return r
		}
	}
	return rs.Race(c.Race)
}

func collectChoices(rs *Ruleset, c *Character) []Choice {
	out := []Choice{}
	level := c.TotalLevel()
	if race := rs.Race(c.Race); race != nil {
		out = append(out, race.Choices...)
	}
	if c.Subrace != "" {
		if sr := rs.Race(c.Subrace); sr != nil {
			out = append(out, sr.Choices...)
		}
	}
	for _, cl := range c.Classes {
		if cls := rs.Class(cl.Class); cls != nil {
			for _, ch := range cls.Choices {
				if ch.Level <= cl.Level {
					out = append(out, ch)
				}
			}
		}
		if sc := rs.Subclass(cl.Class, cl.Subclass); sc != nil {
			for _, ch := range sc.Choices {
				if ch.Level <= cl.Level {
					out = append(out, ch)
				}
			}
		}
	}
	if bg := rs.Background(c.Background); bg != nil {
		out = append(out, bg.Choices...)
	}
	_ = level
	return out
}

func extraLanguageCount(rs *Ruleset, c *Character) int {
	n := 0
	if race := rs.Race(c.Race); race != nil {
		n += race.ExtraLanguages
	}
	if c.Subrace != "" {
		if sr := rs.Race(c.Subrace); sr != nil {
			n += sr.ExtraLanguages
		}
	}
	if bg := rs.Background(c.Background); bg != nil {
		n += bg.Languages
	}
	return n
}

func castingInfo(rs *Ruleset, c *Character) *Spellcasting {
	for _, cl := range c.Classes {
		cls := rs.Class(cl.Class)
		if cls == nil {
			continue
		}
		info := cls.Spellcasting
		if sc := rs.Subclass(cl.Class, cl.Subclass); sc != nil && sc.Spellcasting != nil {
			info = sc.Spellcasting
		}
		if info == nil || info.Progression == "" || info.Progression == "none" {
			continue
		}
		if info.StartLevel > 0 && cl.Level < info.StartLevel {
			continue
		}
		return info
	}
	return nil
}

func castingClass(rs *Ruleset, c *Character) *Class {
	for _, cl := range c.Classes {
		cls := rs.Class(cl.Class)
		if cls == nil {
			continue
		}
		if cls.Spellcasting != nil && cls.Spellcasting.Progression != "" {
			return cls
		}
		if sc := rs.Subclass(cl.Class, cl.Subclass); sc != nil && sc.Spellcasting != nil {
			return cls
		}
	}
	return nil
}

func classLevelOf(c *Character, id string) int {
	for _, cl := range c.Classes {
		if cl.Class == id {
			return cl.Level
		}
	}
	return c.TotalLevel()
}

func cantripCount(rs *Ruleset, c *Character, sc *Spellcasting) int {
	cls := castingClass(rs, c)
	if cls == nil {
		return 0
	}
	lvl := classLevelOf(c, cls.Id)
	if len(sc.CantripsKnown) > lvl {
		return sc.CantripsKnown[lvl]
	}
	if len(sc.CantripsKnown) > 0 {
		return sc.CantripsKnown[len(sc.CantripsKnown)-1]
	}
	return 0
}

// spellCount returns how many levelled spells to pick and what to call them.
func spellCount(rs *Ruleset, c *Character, sc *Spellcasting) (int, string) {
	cls := castingClass(rs, c)
	if cls == nil {
		return 0, ""
	}
	lvl := classLevelOf(c, cls.Id)
	if len(sc.SpellsKnown) > 0 {
		n := 0
		if len(sc.SpellsKnown) > lvl {
			n = sc.SpellsKnown[lvl]
		} else {
			n = sc.SpellsKnown[len(sc.SpellsKnown)-1]
		}
		kind := "spells known"
		if sc.PreparedFrom == "spellbook" {
			kind = "spellbook spells"
		}
		return n, kind
	}
	if sc.Prepares {
		am := AbilityMod(c.Abilities[sc.Ability])
		return PreparedCount(sc, lvl, am), "prepared spells"
	}
	return 0, ""
}

// maxSpellLevel is the highest spell level the character can cast.
func maxSpellLevel(rs *Ruleset, c *Character) int {
	sheet := Compute(c, rs)
	maxLvl := 0
	for _, sl := range sheet.Slots {
		if sl.Total > 0 && sl.Level > maxLvl {
			maxLvl = sl.Level
		}
	}
	if maxLvl == 0 {
		maxLvl = 1
	}
	return maxLvl
}

// ----------------------------------------------------------------------------
// Individual steps
// ----------------------------------------------------------------------------

func stepRuleset() stepDef {
	return stepDef{
		id: "ruleset",
		build: func(s *Session, rs *Ruleset) *Prompt {
			return &Prompt{
				Kind: "select", Title: "Ruleset",
				Question: "Which ruleset are you playing with?",
				Help:     "Rulesets are yaml modules, homebrew content shows up here too.",
				Options:  s.Rulesets, Default: s.Ruleset,
			}
		},
		apply: func(s *Session, rs *Ruleset, a *Answer) error {
			if v := firstValue(a); v != "" {
				s.Ruleset = v
				s.Char.Ruleset = v
			}
			return nil
		},
	}
}

func stepName() stepDef {
	return stepDef{
		id: "name",
		build: func(s *Session, rs *Ruleset) *Prompt {
			opts := []Option{}
			if race := activeRace(rs, s.Char); race != nil {
				for _, n := range race.Names {
					opts = append(opts, Option{Id: n, Name: n})
				}
			}
			return &Prompt{
				Kind: "text", Title: "Name",
				Question:    "What is your character called?",
				Help:        "You can change this at any time by editing the org file.",
				Default:     s.Char.Name,
				Options:     opts,
				AllowCustom: true, AllowRandom: len(opts) > 0,
			}
		},
		apply: func(s *Session, rs *Ruleset, a *Answer) error {
			name := strings.TrimSpace(a.Text)
			if name == "" {
				name = firstValue(a)
			}
			if name == "" {
				return fmt.Errorf("a character needs a name")
			}
			s.Char.Name = name
			return nil
		},
	}
}

func stepClass() stepDef {
	return stepDef{
		id: "class",
		build: func(s *Session, rs *Ruleset) *Prompt {
			opts := []Option{}
			for i := range rs.Classes {
				cls := &rs.Classes[i]
				meta := map[string]string{
					"hitDie":         fmt.Sprintf("d%d", cls.HitDie),
					"primaryAbility": abilityList(cls.PrimaryAbility),
					"saves":          abilityList(cls.SavingThrows),
				}
				caster := "no"
				if cls.Spellcasting != nil && cls.Spellcasting.Progression != "" {
					caster = cls.Spellcasting.Progression
				}
				meta["spellcasting"] = caster
				opts = append(opts, Option{
					Id: cls.Id, Name: cls.Name, Summary: cls.Summary,
					Detail: cls.Text, Meta: meta,
					Tags: []string{"d" + itoa(cls.HitDie), abilityList(cls.PrimaryAbility)},
				})
			}
			sort.Slice(opts, func(i, j int) bool { return opts[i].Name < opts[j].Name })
			return &Prompt{
				Kind: "select", Title: "Class",
				Question: "Choose your class.",
				Help:     "Your class decides your hit die, saving throws, proficiencies and features.",
				Options:  opts, AllowRandom: true,
				Advice: []string{"Fighter, Rogue, Cleric and Wizard cover the four classic party roles.",
					"The hit die (d6 to d12) is how tough you are, the primary ability is what you will want your best score in."},
			}
		},
		apply: func(s *Session, rs *Ruleset, a *Answer) error {
			id := firstValue(a)
			if rs.Class(id) == nil {
				return fmt.Errorf("unknown class %q", id)
			}
			lvl := 1
			if len(s.Char.Classes) > 0 && s.Char.Classes[0].Level > 0 {
				lvl = s.Char.Classes[0].Level
			}
			s.Char.Classes = []ClassLevel{{Class: id, Level: lvl}}
			return nil
		},
	}
}

func stepLevel() stepDef {
	return stepDef{
		id: "level",
		build: func(s *Session, rs *Ruleset) *Prompt {
			def := 1
			if len(s.Char.Classes) > 0 && s.Char.Classes[0].Level > 0 {
				def = s.Char.Classes[0].Level
			}
			if s.Level > 0 {
				def = s.Level
			}
			return &Prompt{
				Kind: "number", Title: "Level",
				Question: "What level is this character?",
				Help:     "Level 1 is a fresh adventurer, most campaigns start at 1 or 3.",
				Min:      1, Max: 20, Default: itoa(def),
			}
		},
		apply: func(s *Session, rs *Ruleset, a *Answer) error {
			lvl := 1
			if a.Numbers != nil {
				if v, ok := a.Numbers["level"]; ok {
					lvl = v
				}
			}
			if v := atoi(firstValue(a)); v > 0 {
				lvl = v
			}
			if v := atoi(a.Text); v > 0 {
				lvl = v
			}
			if lvl < 1 || lvl > 20 {
				return fmt.Errorf("level must be between 1 and 20")
			}
			if len(s.Char.Classes) == 0 {
				s.Char.Classes = []ClassLevel{{Level: lvl}}
			} else {
				s.Char.Classes[0].Level = lvl
			}
			return nil
		},
	}
}

func stepSubclass() stepDef {
	return stepDef{
		id: "subclass",
		build: func(s *Session, rs *Ruleset) *Prompt {
			primary := s.Char.PrimaryClass()
			cls := rs.Class(primary.Class)
			if cls == nil || len(cls.Subclasses) == 0 {
				return nil
			}
			opts := []Option{}
			for i := range cls.Subclasses {
				sc := &cls.Subclasses[i]
				detail := sc.Summary
				for _, f := range sc.Features {
					if f.Level <= primary.Level {
						detail += fmt.Sprintf("\n%s (level %d): %s", f.Name, maxInt(f.Level, 1), f.Text)
					}
				}
				opts = append(opts, Option{Id: sc.Id, Name: sc.Name, Summary: sc.Summary, Detail: detail})
			}
			label := orDefault(cls.SubclassLabel, "Subclass")
			return &Prompt{
				Kind: "select", Title: label,
				Question: fmt.Sprintf("Choose your %s.", strings.ToLower(label)),
				Help:     fmt.Sprintf("%ss gain their %s at level %d.", cls.Name, strings.ToLower(label), maxInt(cls.SubclassLevel, 1)),
				Options:  opts, AllowRandom: true,
			}
		},
		apply: func(s *Session, rs *Ruleset, a *Answer) error {
			id := firstValue(a)
			primary := s.Char.PrimaryClass()
			if rs.Subclass(primary.Class, id) == nil {
				return fmt.Errorf("unknown subclass %q", id)
			}
			s.Char.Classes[0].Subclass = id
			return nil
		},
	}
}

func stepRace() stepDef {
	return stepDef{
		id: "race",
		build: func(s *Session, rs *Ruleset) *Prompt {
			cls := rs.Class(s.Char.PrimaryClass().Class)
			opts := []Option{}
			for i := range rs.Races {
				r := &rs.Races[i]
				opts = append(opts, raceOption(r, cls))
			}
			sort.Slice(opts, func(i, j int) bool { return opts[i].Name < opts[j].Name })
			advice := []string{}
			if cls != nil && len(cls.PrimaryAbility) > 0 {
				advice = append(advice, fmt.Sprintf(
					"A %s wants a high %s, races that boost it are marked as recommended.",
					cls.Name, AbilityNames[cls.PrimaryAbility[0]]))
			}
			return &Prompt{
				Kind: "select", Title: "Race",
				Question: "Choose your race.",
				Help:     "Race sets your ability bonuses, speed, size and a handful of traits.",
				Options:  opts, AllowRandom: true, Advice: advice,
			}
		},
		apply: func(s *Session, rs *Ruleset, a *Answer) error {
			id := firstValue(a)
			r := rs.Race(id)
			if r == nil {
				return fmt.Errorf("unknown race %q", id)
			}
			s.Char.Race = id
			s.Char.Subrace = ""
			s.Char.Languages = addUnique(s.Char.Languages, r.Languages...)
			s.Char.Skills = addUnique(s.Char.Skills, r.Proficiencies.Skills...)
			s.Char.Tools = addUnique(s.Char.Tools, r.Proficiencies.Tools...)
			for _, rsp := range r.Spells {
				if rsp.Level <= s.Char.TotalLevel() {
					s.Char.Spells = appendSpell(s.Char.Spells, rs, rsp.Id, r.Name)
				}
			}
			return nil
		},
	}
}

func raceOption(r *Race, cls *Class) Option {
	bonuses := []string{}
	for _, a := range AbilityOrder {
		if v, ok := r.AbilityBonuses[a]; ok && v != 0 {
			bonuses = append(bonuses, fmt.Sprintf("%s %s", AbilityShort[a], Signed(v)))
		}
	}
	if r.AbilityChoice != nil {
		bonuses = append(bonuses, fmt.Sprintf("%s to %d of your choice",
			Signed(r.AbilityChoice.Amount), r.AbilityChoice.Count))
	}
	summary := r.Summary
	if summary == "" {
		summary = strings.Join(bonuses, ", ")
	}
	detail := r.Text
	for _, t := range r.Traits {
		detail += fmt.Sprintf("\n%s: %s", t.Name, t.Text)
	}
	if len(r.Subraces) > 0 {
		names := []string{}
		for _, sr := range r.Subraces {
			names = append(names, sr.Name)
		}
		detail += "\nSubraces: " + strings.Join(names, ", ")
	}
	o := Option{
		Id: r.Id, Name: r.Name, Summary: summary, Detail: detail,
		Tags: bonuses,
		Meta: map[string]string{
			"speed":  fmt.Sprintf("%d ft.", r.Speed),
			"size":   r.Size,
			"bonus":  strings.Join(bonuses, ", "),
			"traits": itoa(len(r.Traits)),
		},
	}
	if cls != nil && len(cls.PrimaryAbility) > 0 {
		want := cls.PrimaryAbility[0]
		if v, ok := r.AbilityBonuses[want]; ok && v > 0 {
			o.Recommended = true
			o.Reason = fmt.Sprintf("%s %s suits a %s", AbilityShort[want], Signed(v), cls.Name)
		}
		for _, sr := range r.Subraces {
			if v, ok := sr.AbilityBonuses[want]; ok && v > 0 {
				o.Recommended = true
				o.Reason = fmt.Sprintf("the %s subrace boosts %s", sr.Name, AbilityShort[want])
			}
		}
	}
	return o
}

func stepSubrace() stepDef {
	return stepDef{
		id: "subrace",
		build: func(s *Session, rs *Ruleset) *Prompt {
			race := rs.Race(s.Char.Race)
			if race == nil || len(race.Subraces) == 0 {
				return nil
			}
			cls := rs.Class(s.Char.PrimaryClass().Class)
			opts := []Option{}
			for i := range race.Subraces {
				opts = append(opts, raceOption(&race.Subraces[i], cls))
			}
			return &Prompt{
				Kind: "select", Title: "Subrace",
				Question: fmt.Sprintf("Which kind of %s are you?", race.Name),
				Options:  opts, AllowRandom: true,
			}
		},
		apply: func(s *Session, rs *Ruleset, a *Answer) error {
			id := firstValue(a)
			sr := rs.Race(id)
			if sr == nil {
				return fmt.Errorf("unknown subrace %q", id)
			}
			s.Char.Subrace = id
			s.Char.Languages = addUnique(s.Char.Languages, sr.Languages...)
			s.Char.Skills = addUnique(s.Char.Skills, sr.Proficiencies.Skills...)
			s.Char.Tools = addUnique(s.Char.Tools, sr.Proficiencies.Tools...)
			for _, rsp := range sr.Spells {
				if rsp.Level <= s.Char.TotalLevel() {
					s.Char.Spells = appendSpell(s.Char.Spells, rs, rsp.Id, sr.Name)
				}
			}
			return nil
		},
	}
}

func stepRaceAbilityChoice(race *Race) stepDef {
	return stepDef{
		id: "race-ability-choice",
		build: func(s *Session, rs *Ruleset) *Prompt {
			r := activeRace(rs, s.Char)
			if r == nil || r.AbilityChoice == nil {
				return nil
			}
			ac := r.AbilityChoice
			from := ac.From
			if len(from) == 0 {
				from = AbilityOrder
			}
			opts := []Option{}
			for _, a := range from {
				opts = append(opts, Option{Id: a, Name: AbilityNames[a],
					Summary: fmt.Sprintf("%s %s", AbilityShort[a], Signed(ac.Amount))})
			}
			return &Prompt{
				Kind: "multiselect", Title: "Racial Ability Bonus",
				Question: fmt.Sprintf("Choose %d abilities to increase by %d.", ac.Count, ac.Amount),
				Options:  opts, Min: ac.Count, Max: ac.Count, AllowRandom: true,
			}
		},
		apply: func(s *Session, rs *Ruleset, a *Answer) error {
			r := activeRace(rs, s.Char)
			if r == nil || r.AbilityChoice == nil {
				return nil
			}
			if len(a.Values) != r.AbilityChoice.Count {
				return fmt.Errorf("choose exactly %d abilities", r.AbilityChoice.Count)
			}
			s.Char.Choices["race-ability-choice"] = a.Values
			return nil
		},
	}
}

func stepBackground() stepDef {
	return stepDef{
		id: "background",
		build: func(s *Session, rs *Ruleset) *Prompt {
			opts := []Option{}
			for i := range rs.Backgrounds {
				bg := &rs.Backgrounds[i]
				skills := []string{}
				for _, sk := range bg.Proficiencies.Skills {
					skills = append(skills, rs.SkillName(sk))
				}
				detail := bg.Text
				if bg.Feature.Name != "" {
					detail += fmt.Sprintf("\nFeature - %s: %s", bg.Feature.Name, bg.Feature.Text)
				}
				opts = append(opts, Option{
					Id: bg.Id, Name: bg.Name, Summary: orDefault(bg.Summary, joinList(skills)),
					Detail: detail, Tags: skills,
					Meta: map[string]string{"skills": joinList(skills), "feature": bg.Feature.Name},
				})
			}
			sort.Slice(opts, func(i, j int) bool { return opts[i].Name < opts[j].Name })
			return &Prompt{
				Kind: "select", Title: "Background",
				Question: "Choose a background.",
				Help:     "Backgrounds grant two skills, some tools or languages, gear and a roleplaying feature.",
				Options:  opts, AllowRandom: true,
			}
		},
		apply: func(s *Session, rs *Ruleset, a *Answer) error {
			id := firstValue(a)
			bg := rs.Background(id)
			if bg == nil {
				return fmt.Errorf("unknown background %q", id)
			}
			s.Char.Background = id
			s.Char.Skills = addUnique(s.Char.Skills, bg.Proficiencies.Skills...)
			s.Char.Tools = addUnique(s.Char.Tools, bg.Proficiencies.Tools...)
			for _, it := range bg.Equipment {
				s.Char.Equipment = appendGear(s.Char.Equipment, rs, it, false)
			}
			s.Char.Money.GP += bg.Gold
			return nil
		},
	}
}

func stepAbilityMethod() stepDef {
	return stepDef{
		id: "ability-method",
		build: func(s *Session, rs *Ruleset) *Prompt {
			return &Prompt{
				Kind: "select", Title: "Ability Scores",
				Question: "How do you want to generate ability scores?",
				Options: []Option{
					{Id: "standard", Name: "Standard array", Summary: "15, 14, 13, 12, 10, 8",
						Detail: "The default spread, balanced and quick.", Recommended: true},
					{Id: "pointbuy", Name: "Point buy", Summary: "27 points, scores 8-15",
						Detail: "Spend 27 points, every score starts at 8. 14 costs 7 points, 15 costs 9."},
					{Id: "roll", Name: "Roll 4d6 drop lowest", Summary: "Six random scores",
						Detail: "The classic random method, swingier than the other two."},
					{Id: "manual", Name: "Enter scores by hand", Summary: "Type six numbers",
						Detail: "Use this if your DM gave you scores or you are copying an existing character."},
				},
				Default: "standard",
			}
		},
		apply: func(s *Session, rs *Ruleset, a *Answer) error {
			m := firstValue(a)
			switch m {
			case "standard":
				s.Pool = append([]int{}, StandardArray...)
			case "roll":
				// The roll is stored on the answer so that replaying the
				// session (used by the back button) keeps the same dice.
				if len(a.Pool) != 6 {
					pool := []int{}
					for i := 0; i < 6; i++ {
						pool = append(pool, RollAbility())
					}
					sort.Sort(sort.Reverse(sort.IntSlice(pool)))
					a.Pool = pool
				}
				s.Pool = append([]int{}, a.Pool...)
			case "pointbuy", "manual":
				s.Pool = nil
			default:
				return fmt.Errorf("unknown method %q", m)
			}
			s.Method = m
			return nil
		},
	}
}

func stepAbilities() stepDef {
	return stepDef{
		id: "abilities",
		build: func(s *Session, rs *Ruleset) *Prompt {
			cls := rs.Class(s.Char.PrimaryClass().Class)
			race := activeRace(rs, s.Char)
			fields := []Field{}
			for _, a := range AbilityOrder {
				hint := ""
				if cls != nil {
					if containsStr(cls.PrimaryAbility, a) {
						hint = "primary ability"
					} else if containsStr(cls.SavingThrows, a) {
						hint = "saving throw proficiency"
					}
				}
				bonus := racialBonus(rs, s.Char, a)
				f := Field{Id: a, Name: AbilityNames[a], Hint: hint, Bonus: bonus, Min: 3, Max: 20}
				switch s.Method {
				case "pointbuy":
					f.Min, f.Max, f.Value = 8, 15, 8
				case "manual":
					f.Value = 10
				}
				fields = append(fields, f)
			}
			advice := []string{}
			if cls != nil && len(cls.PrimaryAbility) > 0 {
				names := []string{}
				for _, a := range cls.PrimaryAbility {
					names = append(names, AbilityNames[a])
				}
				advice = append(advice, fmt.Sprintf("Put your highest score in %s.", joinList(names)))
				advice = append(advice, "Constitution is never wasted, it is hit points for everyone.")
			}
			if race != nil {
				bs := []string{}
				for _, a := range AbilityOrder {
					if b := racialBonus(rs, s.Char, a); b != 0 {
						bs = append(bs, fmt.Sprintf("%s %s", AbilityShort[a], Signed(b)))
					}
				}
				if len(bs) > 0 {
					advice = append(advice, fmt.Sprintf("%s adds %s after assignment.", race.Name, joinList(bs)))
				}
			}
			q := "Assign your six scores."
			help := ""
			switch s.Method {
			case "standard":
				q = "Assign the standard array to your abilities."
				help = "Each value can be used once."
			case "roll":
				q = "Assign your rolled scores to your abilities."
				help = "Each value can be used once."
			case "pointbuy":
				q = "Spend 27 points across your abilities."
				help = "8 is free. 9-13 cost 1 point each step, 14 costs 2 more, 15 costs 2 more again."
			case "manual":
				q = "Type each ability score."
			}
			return &Prompt{
				Kind: "abilities", Title: "Ability Scores", Question: q, Help: help,
				Fields: fields, Pool: s.Pool, Advice: advice, AllowRandom: true,
			}
		},
		apply: func(s *Session, rs *Ruleset, a *Answer) error {
			if len(a.Numbers) == 0 {
				return fmt.Errorf("no ability scores supplied")
			}
			base := map[string]int{}
			for _, ab := range AbilityOrder {
				v, ok := a.Numbers[ab]
				if !ok {
					return fmt.Errorf("missing score for %s", AbilityNames[ab])
				}
				base[ab] = v
			}
			switch s.Method {
			case "standard", "roll":
				want := append([]int{}, s.Pool...)
				got := []int{}
				for _, ab := range AbilityOrder {
					got = append(got, base[ab])
				}
				sort.Ints(want)
				sort.Ints(got)
				for i := range want {
					if want[i] != got[i] {
						return fmt.Errorf("scores must use each of %v exactly once", s.Pool)
					}
				}
			case "pointbuy":
				spent := 0
				for _, ab := range AbilityOrder {
					cost, ok := PointBuyCost[base[ab]]
					if !ok {
						return fmt.Errorf("%s must be between 8 and 15 in point buy", AbilityNames[ab])
					}
					spent += cost
				}
				if spent > PointBuyBudget {
					return fmt.Errorf("that costs %d points, you only have %d", spent, PointBuyBudget)
				}
			}
			s.Char.Choices["ability-base"] = encodeAbilityBase(base)
			for _, ab := range AbilityOrder {
				s.Char.Abilities[ab] = base[ab] + racialBonus(rs, s.Char, ab)
			}
			return nil
		},
	}
}

func encodeAbilityBase(base map[string]int) []string {
	out := []string{}
	for _, a := range AbilityOrder {
		out = append(out, fmt.Sprintf("%s=%d", a, base[a]))
	}
	return out
}

// racialBonus is the total racial ability bonus including chosen ones.
func racialBonus(rs *Ruleset, c *Character, ability string) int {
	n := 0
	if race := rs.Race(c.Race); race != nil {
		n += race.AbilityBonuses[ability]
	}
	if c.Subrace != "" {
		if sr := rs.Race(c.Subrace); sr != nil {
			n += sr.AbilityBonuses[ability]
		}
	}
	if picks, ok := c.Choices["race-ability-choice"]; ok {
		r := activeRace(rs, c)
		amount := 1
		if r != nil && r.AbilityChoice != nil && r.AbilityChoice.Amount != 0 {
			amount = r.AbilityChoice.Amount
		}
		for _, p := range picks {
			if p == ability {
				n += amount
			}
		}
	}
	return n
}

// stepASI is the "increase one score by 2 or two scores by 1" prompt that
// every class gets at 4th level and beyond.
func stepASI(classId string, level int) stepDef {
	id := fmt.Sprintf("asi-%s-%d", classId, level)
	return stepDef{
		id: id,
		build: func(s *Session, rs *Ruleset) *Prompt {
			opts := []Option{}
			for _, a := range AbilityOrder {
				score := s.Char.Abilities[a]
				o := Option{
					Id: a, Name: AbilityNames[a],
					Summary: fmt.Sprintf("currently %d (%s)", score, Signed(AbilityMod(score))),
					Meta:    map[string]string{"score": itoa(score)},
				}
				if score >= 20 {
					o.Disabled = true
					o.Reason = "already at the maximum of 20"
				}
				if cls := rs.Class(classId); cls != nil && containsStr(cls.PrimaryAbility, a) && score < 20 {
					o.Recommended = true
					o.Reason = "your primary ability"
				}
				opts = append(opts, o)
			}
			name := Titleize(classId)
			if cls := rs.Class(classId); cls != nil {
				name = cls.Name
			}
			return &Prompt{
				Kind: "multiselect", Title: "Ability Score Improvement",
				Question: fmt.Sprintf("%s level %d: raise one ability by 2, or two abilities by 1.",
					name, level),
				Help: "Pick one ability for +2, or two different abilities for +1 each. " +
					"No score can go above 20. If you would rather take a feat, skip this " +
					"and add it to the DND_FEATS property.",
				Options: opts, Min: 1, Max: 2, AllowRandom: true, AllowSkip: true,
			}
		},
		apply: func(s *Session, rs *Ruleset, a *Answer) error {
			if len(a.Values) == 0 || len(a.Values) > 2 {
				return fmt.Errorf("choose one ability (+2) or two abilities (+1 each)")
			}
			bump := 2
			if len(a.Values) == 2 {
				bump = 1
			}
			for _, ab := range a.Values {
				if _, ok := AbilityNames[ab]; !ok {
					return fmt.Errorf("unknown ability %q", ab)
				}
				if s.Char.Abilities[ab]+bump > 20 {
					return fmt.Errorf("%s cannot go above 20", AbilityNames[ab])
				}
			}
			for _, ab := range a.Values {
				s.Char.Abilities[ab] += bump
			}
			s.Char.Choices[id] = a.Values
			return nil
		},
	}
}

func stepSkills(cls *Class) stepDef {
	return stepDef{
		id: "skills",
		build: func(s *Session, rs *Ruleset) *Prompt {
			cls := rs.Class(s.Char.PrimaryClass().Class)
			if cls == nil || cls.SkillCount == 0 {
				return nil
			}
			from := cls.SkillsFrom
			if len(from) == 0 {
				for _, sk := range rs.Skills {
					from = append(from, sk.Id)
				}
			}
			opts := []Option{}
			for _, id := range from {
				if s.Char.HasSkill(id) {
					continue // already proficient from race or background
				}
				sk := rs.Skill(id)
				name := rs.SkillName(id)
				ability := ""
				if sk != nil {
					ability = sk.Ability
				}
				mod := AbilityMod(s.Char.Abilities[ability])
				o := Option{
					Id: id, Name: name,
					Summary: fmt.Sprintf("%s (%s %s)", ability2Name(ability), AbilityShort[ability], Signed(mod)),
					Meta:    map[string]string{"ability": ability, "mod": Signed(mod)},
				}
				if sk != nil {
					o.Detail = sk.Text
				}
				if mod >= 2 {
					o.Recommended = true
					o.Reason = fmt.Sprintf("your %s is good", AbilityNames[ability])
				}
				opts = append(opts, o)
			}
			sort.Slice(opts, func(i, j int) bool { return opts[i].Name < opts[j].Name })
			have := []string{}
			for _, sk := range s.Char.Skills {
				have = append(have, rs.SkillName(sk))
			}
			advice := []string{}
			if len(have) > 0 {
				advice = append(advice, "Already proficient from race/background: "+joinList(have))
			}
			return &Prompt{
				Kind: "multiselect", Title: "Skills",
				Question: fmt.Sprintf("Choose %d skill proficiencies.", cls.SkillCount),
				Help:     "Proficiency adds your proficiency bonus to checks with that skill.",
				Options:  opts, Min: cls.SkillCount, Max: cls.SkillCount,
				AllowRandom: true, Advice: advice,
			}
		},
		apply: func(s *Session, rs *Ruleset, a *Answer) error {
			cls := rs.Class(s.Char.PrimaryClass().Class)
			if cls == nil {
				return nil
			}
			if len(a.Values) != cls.SkillCount {
				return fmt.Errorf("choose exactly %d skills", cls.SkillCount)
			}
			s.Char.Skills = addUnique(s.Char.Skills, a.Values...)
			return nil
		},
	}
}

// choiceCount is how many picks a choice allows at a given class level.
func choiceCount(ch Choice, level int) int {
	if len(ch.CountByLevel) > 0 {
		if level < len(ch.CountByLevel) && ch.CountByLevel[level] > 0 {
			return ch.CountByLevel[level]
		}
		if level >= len(ch.CountByLevel) {
			return ch.CountByLevel[len(ch.CountByLevel)-1]
		}
	}
	return maxInt(ch.Count, 1)
}

func stepChoice(ch Choice) stepDef {
	id := "choice-" + ch.Id
	return stepDef{
		id: id,
		build: func(s *Session, rs *Ruleset) *Prompt {
			opts := []Option{}
			switch ch.Kind {
			case "options":
				for _, o := range ch.Options {
					opts = append(opts, Option{Id: orDefault(Slugify(o.Name), o.Name), Name: o.Name,
						Summary: firstSentence(o.Text), Detail: o.Text})
				}
			case "skills":
				src := ch.From
				if len(src) == 0 {
					for _, sk := range rs.Skills {
						src = append(src, sk.Id)
					}
				}
				for _, sid := range src {
					if s.Char.HasSkill(sid) && ch.Kind != "expertise" {
						continue
					}
					opts = append(opts, Option{Id: sid, Name: rs.SkillName(sid)})
				}
			case "expertise":
				for _, sid := range s.Char.Skills {
					if s.Char.HasExpertise(sid) {
						continue
					}
					opts = append(opts, Option{Id: sid, Name: rs.SkillName(sid),
						Summary: "double proficiency bonus"})
				}
			case "tools":
				for _, t := range ch.From {
					opts = append(opts, Option{Id: t, Name: Titleize(t)})
				}
			case "languages":
				for _, l := range rs.AllLanguages() {
					if containsStr(s.Char.Languages, l) {
						continue
					}
					opts = append(opts, Option{Id: l, Name: l})
				}
			case "spells":
				list := ""
				if len(ch.From) > 0 {
					list = ch.From[0]
				}
				for _, sp := range rs.SpellsForClass(list, -1) {
					if len(ch.SpellLevels) > 0 && !containsInt(ch.SpellLevels, sp.Level) {
						continue
					}
					if hasSpell(s.Char.Spells, sp.Id) {
						continue
					}
					opts = append(opts, spellOption(sp))
				}
			default:
				for _, f := range ch.From {
					opts = append(opts, Option{Id: f, Name: Titleize(f)})
				}
			}
			if len(opts) == 0 {
				return nil
			}
			count := choiceCount(ch, s.Char.TotalLevel())
			kind := "multiselect"
			if count == 1 {
				kind = "select"
			}
			return &Prompt{
				Kind: kind, Title: orDefault(ch.Name, Titleize(ch.Id)),
				Question: orDefault(ch.Prompt, fmt.Sprintf("Choose %d.", count)),
				Help:     ch.Help, Options: opts, Min: count, Max: count, AllowRandom: true,
			}
		},
		apply: func(s *Session, rs *Ruleset, a *Answer) error {
			count := choiceCount(ch, s.Char.TotalLevel())
			if len(a.Values) == 0 {
				return fmt.Errorf("choose %d", count)
			}
			if len(a.Values) > count {
				return fmt.Errorf("choose at most %d", count)
			}
			s.Char.Choices[ch.Id] = a.Values
			switch ch.Kind {
			case "skills":
				s.Char.Skills = addUnique(s.Char.Skills, a.Values...)
			case "expertise":
				s.Char.Expertise = addUnique(s.Char.Expertise, a.Values...)
			case "tools":
				s.Char.Tools = addUnique(s.Char.Tools, a.Values...)
			case "languages":
				s.Char.Languages = addUnique(s.Char.Languages, a.Values...)
			case "spells":
				for _, v := range a.Values {
					s.Char.Spells = appendSpell(s.Char.Spells, rs, v, ch.Name)
				}
			}
			return nil
		},
	}
}

func stepLanguages(count int) stepDef {
	return stepDef{
		id: "languages",
		build: func(s *Session, rs *Ruleset) *Prompt {
			opts := []Option{}
			for _, l := range rs.AllLanguages() {
				if containsStr(s.Char.Languages, l) {
					continue
				}
				opts = append(opts, Option{Id: l, Name: l})
			}
			if len(opts) == 0 {
				return nil
			}
			return &Prompt{
				Kind: "multiselect", Title: "Languages",
				Question: fmt.Sprintf("Choose %d additional languages.", count),
				Help:     "You already speak: " + joinList(s.Char.Languages),
				Options:  opts, Min: count, Max: count, AllowRandom: true,
			}
		},
		apply: func(s *Session, rs *Ruleset, a *Answer) error {
			if len(a.Values) != count {
				return fmt.Errorf("choose exactly %d languages", count)
			}
			s.Char.Languages = addUnique(s.Char.Languages, a.Values...)
			return nil
		},
	}
}

func stepEquipment(cls *Class, idx int) stepDef {
	group := cls.Equipment[idx]
	id := fmt.Sprintf("equipment-%d", idx)
	if group.Id != "" {
		id = "equipment-" + group.Id
	}
	return stepDef{
		id: id,
		build: func(s *Session, rs *Ruleset) *Prompt {
			c := rs.Class(s.Char.PrimaryClass().Class)
			if c == nil || idx >= len(c.Equipment) {
				return nil
			}
			grp := c.Equipment[idx]
			opts := []Option{}
			for i, o := range grp.Options {
				detail := []string{}
				for _, it := range o.Items {
					name := it.Name
					if item := rs.Item(it.Id); item != nil {
						name = item.Name
						if item.Damage != "" {
							name += fmt.Sprintf(" (%s %s)", item.Damage, item.DamageType)
						} else if item.AC > 0 {
							name += fmt.Sprintf(" (AC %d)", item.AC)
						}
					}
					if it.Qty > 1 {
						name = fmt.Sprintf("%d x %s", it.Qty, name)
					}
					detail = append(detail, name)
				}
				opts = append(opts, Option{
					Id: itoa(i), Name: o.Label, Summary: joinList(detail),
					Detail: joinList(detail),
				})
			}
			if len(opts) == 0 {
				return nil
			}
			return &Prompt{
				Kind: "select", Title: "Starting Equipment",
				Question: orDefault(grp.Prompt, "Choose your starting equipment."),
				Options:  opts, AllowRandom: true,
			}
		},
		apply: func(s *Session, rs *Ruleset, a *Answer) error {
			c := rs.Class(s.Char.PrimaryClass().Class)
			if c == nil || idx >= len(c.Equipment) {
				return nil
			}
			grp := c.Equipment[idx]
			pick := atoi(firstValue(a))
			if pick < 0 || pick >= len(grp.Options) {
				return fmt.Errorf("invalid equipment choice")
			}
			for _, it := range grp.Options[pick].Items {
				s.Char.Equipment = appendGear(s.Char.Equipment, rs, it, true)
			}
			return nil
		},
	}
}

func stepSpells(level, count int, kind string) stepDef {
	id := "spells-known"
	if level == 0 {
		id = "spells-cantrips"
	}
	return stepDef{
		id: id,
		build: func(s *Session, rs *Ruleset) *Prompt {
			cls := castingClass(rs, s.Char)
			if cls == nil {
				return nil
			}
			sc := castingInfo(rs, s.Char)
			listId := cls.Id
			if sc != nil && sc.SpellList != "" {
				listId = sc.SpellList
			}
			maxLvl := maxSpellLevel(rs, s.Char)
			opts := []Option{}
			for _, sp := range rs.SpellsForClass(listId, -1) {
				if level == 0 && sp.Level != 0 {
					continue
				}
				if level != 0 && (sp.Level == 0 || sp.Level > maxLvl) {
					continue
				}
				if hasSpell(s.Char.Spells, sp.Id) {
					continue
				}
				opts = append(opts, spellOption(sp))
			}
			if len(opts) == 0 {
				return nil
			}
			title := "Cantrips"
			q := fmt.Sprintf("Choose %d cantrips.", count)
			help := "Cantrips are cast at will and never use a spell slot."
			if level != 0 {
				title = "Spells"
				q = fmt.Sprintf("Choose %d %s.", count, kind)
				help = fmt.Sprintf("You can cast spells up to level %d.", maxLvl)
				if sc != nil && sc.Prepares {
					help += " Prepared spells can be swapped after a long rest."
				}
			}
			return &Prompt{
				Kind: "multiselect", Title: title, Question: q, Help: help,
				Options: opts, Min: count, Max: count, AllowRandom: true,
			}
		},
		apply: func(s *Session, rs *Ruleset, a *Answer) error {
			if len(a.Values) > count {
				return fmt.Errorf("choose at most %d", count)
			}
			for _, v := range a.Values {
				s.Char.Spells = appendSpell(s.Char.Spells, rs, v, "")
			}
			return nil
		},
	}
}

func spellOption(sp *Spell) Option {
	tags := []string{sp.School}
	if sp.Concentration {
		tags = append(tags, "concentration")
	}
	if sp.Ritual {
		tags = append(tags, "ritual")
	}
	detail := fmt.Sprintf("%s\nCasting time %s, range %s, components %s, duration %s.\n%s",
		sp.LevelString(), sp.CastingTime, sp.Range, sp.Components, sp.Duration, sp.Text)
	return Option{
		Id: sp.Id, Name: sp.Name, Summary: fmt.Sprintf("%s - %s", sp.LevelString(), firstSentence(sp.Text)),
		Detail: detail, Tags: tags,
		Meta: map[string]string{"level": itoa(sp.Level), "school": sp.School,
			"time": sp.CastingTime, "range": sp.Range, "duration": sp.Duration},
	}
}

func stepAlignment() stepDef {
	return stepDef{
		id: "alignment",
		build: func(s *Session, rs *Ruleset) *Prompt {
			descs := map[string]string{
				"Lawful Good":     "Does the right thing as society expects. Paladins and dutiful knights.",
				"Neutral Good":    "Does the best they can to help others. The most common good alignment.",
				"Chaotic Good":    "Follows their conscience, with little regard for rules.",
				"Lawful Neutral":  "Acts in accordance with law, tradition or a personal code.",
				"True Neutral":    "Avoids moral questions and does what seems best at the time.",
				"Chaotic Neutral": "Follows their whims, prizing their own freedom above all.",
				"Lawful Evil":     "Takes what they want within the limits of a code or hierarchy.",
				"Neutral Evil":    "Does whatever they can get away with.",
				"Chaotic Evil":    "Acts with arbitrary violence and destruction.",
			}
			opts := []Option{}
			for _, al := range rs.AllAlignments() {
				opts = append(opts, Option{Id: al, Name: al, Summary: descs[al]})
			}
			return &Prompt{
				Kind: "select", Title: "Alignment",
				Question: "Choose an alignment.",
				Help:     "Alignment is a roleplaying signpost, not a straitjacket.",
				Options:  opts, AllowRandom: true, Default: "Neutral Good",
			}
		},
		apply: func(s *Session, rs *Ruleset, a *Answer) error {
			v := firstValue(a)
			if v == "" {
				v = strings.TrimSpace(a.Text)
			}
			s.Char.Alignment = v
			return nil
		},
	}
}

func stepPersonality(field, label string) stepDef {
	return stepDef{
		id:       field,
		optional: true,
		build: func(s *Session, rs *Ruleset) *Prompt {
			bg := rs.Background(s.Char.Background)
			list := []string{}
			if bg != nil {
				switch field {
				case "personality":
					list = bg.Traits
				case "ideals":
					list = bg.Ideals
				case "bonds":
					list = bg.Bonds
				case "flaws":
					list = bg.Flaws
				}
			}
			opts := []Option{}
			for _, t := range list {
				opts = append(opts, Option{Id: t, Name: t})
			}
			article := "a"
			if strings.ContainsRune("aeiou", rune(strings.ToLower(label)[0])) {
				article = "an"
			}
			return &Prompt{
				Kind: "text", Title: label,
				Question:    fmt.Sprintf("Pick or write %s %s.", article, strings.ToLower(label)),
				Help:        "Suggestions come from your background, you can type your own.",
				Options:     opts, AllowCustom: true, AllowRandom: len(opts) > 0, AllowSkip: true,
			}
		},
		apply: func(s *Session, rs *Ruleset, a *Answer) error {
			v := strings.TrimSpace(a.Text)
			if v == "" {
				v = firstValue(a)
			}
			switch field {
			case "personality":
				s.Char.Personality = v
			case "ideals":
				s.Char.Ideals = v
			case "bonds":
				s.Char.Bonds = v
			case "flaws":
				s.Char.Flaws = v
			}
			return nil
		},
	}
}

func stepDetails() stepDef {
	return stepDef{
		id:       "details",
		optional: true,
		build: func(s *Session, rs *Ruleset) *Prompt {
			race := activeRace(rs, s.Char)
			help := ""
			if race != nil && race.Age != "" {
				help = race.Age
			}
			return &Prompt{
				Kind: "fields", Title: "Appearance",
				Question: "Physical details (all optional).",
				Help:     help,
				Fields: []Field{
					{Id: "age", Name: "Age"},
					{Id: "height", Name: "Height"},
					{Id: "weight", Name: "Weight"},
					{Id: "eyes", Name: "Eyes"},
					{Id: "skin", Name: "Skin"},
					{Id: "hair", Name: "Hair"},
				},
				AllowSkip: true,
			}
		},
		apply: func(s *Session, rs *Ruleset, a *Answer) error {
			get := func(i int) string {
				if i < len(a.Values) {
					return strings.TrimSpace(a.Values[i])
				}
				return ""
			}
			s.Char.Age = get(0)
			s.Char.Height = get(1)
			s.Char.Weight = get(2)
			s.Char.Eyes = get(3)
			s.Char.Skin = get(4)
			s.Char.Hair = get(5)
			return nil
		},
	}
}

func stepBackstory() stepDef {
	return stepDef{
		id:       "backstory",
		optional: true,
		build: func(s *Session, rs *Ruleset) *Prompt {
			return &Prompt{
				Kind: "longtext", Title: "Backstory",
				Question:  "Write a short backstory (optional).",
				Help:      "This lands in the Backstory section of the org file where you can keep editing it.",
				AllowSkip: true,
			}
		},
		apply: func(s *Session, rs *Ruleset, a *Answer) error {
			s.Char.Backstory = strings.TrimSpace(a.Text)
			return nil
		},
	}
}

// ----------------------------------------------------------------------------
// helpers
// ----------------------------------------------------------------------------

func firstValue(a *Answer) string {
	if a == nil {
		return ""
	}
	if len(a.Values) > 0 {
		return a.Values[0]
	}
	return strings.TrimSpace(a.Text)
}

// abilityList renders a list of ability ids as "STR, CON".
func abilityList(list []string) string {
	out := []string{}
	for _, a := range list {
		if s, ok := AbilityShort[a]; ok {
			out = append(out, s)
		} else {
			out = append(out, strings.ToUpper(a))
		}
	}
	return strings.Join(out, ", ")
}

func ability2Name(a string) string {
	if n, ok := AbilityNames[a]; ok {
		return n
	}
	return Titleize(a)
}

func firstSentence(text string) string {
	text = strings.TrimSpace(strings.ReplaceAll(text, "\n", " "))
	if idx := strings.Index(text, ". "); idx > 0 {
		return text[:idx+1]
	}
	if len(text) > 140 {
		return text[:137] + "..."
	}
	return text
}

func hasSpell(list []KnownSpell, id string) bool {
	for _, s := range list {
		if s.Id == id {
			return true
		}
	}
	return false
}

func appendSpell(list []KnownSpell, rs *Ruleset, id, source string) []KnownSpell {
	if id == "" || hasSpell(list, id) {
		return list
	}
	ks := KnownSpell{Id: id, Name: Titleize(id), Prepared: true, Source: source}
	if sp := rs.Spell(id); sp != nil {
		ks.Id = sp.Id
		ks.Name = sp.Name
		ks.Level = sp.Level
	}
	return append(list, ks)
}

func appendGear(list []Gear, rs *Ruleset, ref ItemRef, equip bool) []Gear {
	name := ref.Name
	weight := 0.0
	id := ref.Id
	if it := rs.Item(ref.Id); it != nil {
		name = it.Name
		weight = it.Weight
		id = it.Id
		// packs explode into their contents
		if it.Kind == "pack" && len(it.Contents) > 0 {
			list = append(list, Gear{Id: it.Id, Name: it.Name, Qty: 1, Weight: it.Weight,
				Notes: "pack"})
			for _, ct := range it.Contents {
				list = appendGear(list, rs, ct, false)
			}
			return list
		}
	}
	if name == "" {
		name = Titleize(ref.Id)
	}
	qty := ref.Qty
	if qty <= 0 {
		qty = 1
	}
	// merge duplicates
	for i := range list {
		if list[i].Id == id && id != "" {
			list[i].Qty += qty
			return list
		}
	}
	equipped := false
	if equip {
		if it := rs.Item(id); it != nil && (it.Kind == "weapon" || it.Kind == "armor" || it.Kind == "shield") {
			equipped = true
		}
	}
	return append(list, Gear{Id: id, Name: name, Qty: qty, Weight: weight, Equipped: equipped})
}

// randomize fills an answer in with a valid random choice.
func (s *Session) randomize(rs *Ruleset, st *stepDef, a *Answer) {
	p := st.build(s, rs)
	if p == nil {
		a.Skip = true
		return
	}
	switch p.Kind {
	case "select", "text":
		if len(p.Options) > 0 {
			opts := enabledOptions(p.Options)
			pick := opts[randIndex(len(opts))]
			a.Values = []string{pick.Id}
			a.Text = pick.Name
		} else if p.Kind == "text" {
			a.Text = p.Default
		}
	case "multiselect":
		opts := enabledOptions(p.Options)
		n := p.Min
		if n <= 0 {
			n = 1
		}
		if n > len(opts) {
			n = len(opts)
		}
		perm := randPerm(len(opts))
		vals := []string{}
		for i := 0; i < n; i++ {
			vals = append(vals, opts[perm[i]].Id)
		}
		a.Values = vals
	case "number":
		a.Text = p.Default
	case "abilities":
		pool := append([]int{}, s.Pool...)
		if len(pool) == 0 {
			pool = append([]int{}, StandardArray...)
		}
		// put the best scores where the class wants them
		order := abilityPriority(rs, s.Char)
		sort.Sort(sort.Reverse(sort.IntSlice(pool)))
		nums := map[string]int{}
		for i, ab := range order {
			if i < len(pool) {
				nums[ab] = pool[i]
			} else {
				nums[ab] = 10
			}
		}
		a.Numbers = nums
	case "fields":
		a.Skip = true
	case "longtext":
		a.Skip = true
	}
}

// abilityPriority orders the abilities by how much this class wants them.
func abilityPriority(rs *Ruleset, c *Character) []string {
	cls := rs.Class(c.PrimaryClass().Class)
	order := []string{}
	if cls != nil {
		order = append(order, cls.PrimaryAbility...)
		order = addUnique(order, CON)
		for _, sv := range cls.SavingThrows {
			order = addUnique(order, sv)
		}
	}
	for _, a := range AbilityOrder {
		order = addUnique(order, a)
	}
	return order
}

func enabledOptions(opts []Option) []Option {
	out := []Option{}
	for _, o := range opts {
		if !o.Disabled {
			out = append(out, o)
		}
	}
	if len(out) == 0 {
		return opts
	}
	return out
}

// RandomCharacter builds a complete character without any interaction.
func RandomCharacter(rs *Ruleset, req *NewSessionRequest) (*Character, error) {
	sess := NewSession("random", req, nil)
	guard := 0
	for {
		guard++
		if guard > 200 {
			return nil, fmt.Errorf("character generation did not converge")
		}
		p := sess.Next(rs)
		if p == nil || p.Done {
			break
		}
		ans := &Answer{Session: sess.Id, Step: p.Step, Random: true}
		if p.Step == "name" && req.Name != "" {
			ans.Random = false
			ans.Text = req.Name
		}
		if p.Step == "level" {
			ans.Random = false
			lvl := req.Level
			if lvl <= 0 {
				lvl = 1
			}
			ans.Text = itoa(lvl)
		}
		if _, err := sess.Apply(rs, ans); err != nil {
			// A failed random answer would loop forever, skip the step instead.
			ans.Random = false
			ans.Skip = true
			sess.Answers[p.Step] = ans
			sess.Order = append(sess.Order, p.Step)
		}
	}
	c := sess.Finish(rs)
	if c.Name == "" || c.Name == "Unnamed Adventurer" {
		if race := activeRace(rs, c); race != nil && len(race.Names) > 0 {
			c.Name = PickRandom(race.Names)
		}
	}
	return c, nil
}
