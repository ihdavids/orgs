package dnd

import "testing"

func modules(t *testing.T) *Ruleset {
	t.Helper()
	l := NewLibrary([]string{"../../../templates/dnd"})
	l.Resolve()
	if len(l.Errors) > 0 {
		t.Fatalf("module load errors: %v", l.Errors)
	}
	rs := l.Get("srd")
	if rs == nil {
		t.Fatal("no srd ruleset")
	}
	return rs
}

func artificerAt(level int, sub string) *Character {
	return &Character{Name: "A", Race: "human",
		Abilities: map[string]int{"str": 10, "dex": 14, "con": 14, "int": 18, "wis": 12, "cha": 10},
		Classes:   []ClassLevel{{Class: "artificer", Subclass: sub, Level: level}}}
}

func slotString(s *Sheet) string {
	out := ""
	for _, sl := range s.Slots {
		if sl.Total > 0 {
			out += string(rune('0'+sl.Level)) + ":" + string(rune('0'+sl.Total)) + " "
		}
	}
	return out
}

// The artificer is the only half caster with spell slots at 1st level. Every
// other one waits until 2nd, so getting this wrong is silent: the class simply
// has no magic at the level it is most defined by having some.
func TestArtificerCastsFromFirstLevel(t *testing.T) {
	rs := modules(t)
	s := Compute(artificerAt(1, "alchemist"), rs)
	if !s.IsCaster {
		t.Fatal("an artificer of 1st level is not a caster")
	}
	if got := slotString(s); got != "1:2 " {
		t.Errorf("1st level slots = %q, want two 1st level slots", got)
	}
	// Intelligence, and prepared is the modifier plus half the level at least 1.
	if s.CastingAbility != "INT" {
		t.Errorf("casting ability = %q, want INT", s.CastingAbility)
	}
	if s.PreparedMax != 4 {
		t.Errorf("prepared = %d, want 4 (int +4, half of level 1 rounds to 0)", s.PreparedMax)
	}
}

// The slot table, at the levels where it changes shape.
func TestArtificerSlotProgression(t *testing.T) {
	rs := modules(t)
	for _, tc := range []struct {
		level int
		want  string
	}{
		{1, "1:2 "},
		{5, "1:4 2:2 "},
		{9, "1:4 2:3 3:2 "},
		{13, "1:4 2:3 3:3 4:1 "},
		{20, "1:4 2:3 3:3 4:3 5:2 "},
	} {
		if got := slotString(Compute(artificerAt(tc.level, "artillerist"), rs)); got != tc.want {
			t.Errorf("level %d slots = %q, want %q", tc.level, got, tc.want)
		}
	}
}

// Half casters round their contribution to a multiclass caster level down;
// the artificer is the one the rules say rounds up, and it is the reason
// Spellcasting.MulticlassRoundUp exists.
func TestArtificerRoundsUpWhenMulticlassed(t *testing.T) {
	rs := modules(t)
	c := artificerAt(3, "battle-smith")
	c.Classes = append(c.Classes, ClassLevel{Class: "paladin", Level: 3})
	c.Abilities["cha"] = 14
	// artificer 3 rounds up to 2, paladin 3 rounds down to 1: caster level 3.
	if got := slotString(Compute(c, rs)); got != "1:4 2:2 " {
		t.Errorf("artificer 3 / paladin 3 slots = %q, want caster level 3", got)
	}
	// A paladin on their own still rounds down, which is what everyone else does.
	only := &Character{Name: "P", Race: "human",
		Abilities: map[string]int{"str": 14, "dex": 10, "con": 14, "int": 10, "wis": 12, "cha": 16},
		Classes:   []ClassLevel{{Class: "paladin", Level: 3}, {Class: "fighter", Level: 1}}}
	if got := slotString(Compute(only, rs)); got != "1:2 " {
		t.Errorf("paladin 3 / fighter 1 slots = %q, want caster level 1", got)
	}
}

// The class is no use without its list, and most of the list is spells other
// classes already have - so both halves of artificer-spells.yaml have to land.
func TestArtificerSpellList(t *testing.T) {
	rs := modules(t)
	if n := len(rs.SpellsForClass("artificer", -1)); n < 90 {
		t.Errorf("the artificer list has %d spells, want the whole list", n)
	}
	// Written out in artificer-spells.yaml because nothing carried them.
	for _, name := range []string{"Green-Flame Blade", "Lightning Lure", "Sword Burst",
		"Catapult", "Tasha's Caustic Brew", "Pyrotechnics", "Blinding Smite",
		"Catnap", "Intellect Fortress", "Tiny Servant", "Skill Empowerment"} {
		s := rs.Spell(name)
		if s == nil {
			t.Errorf("%q was not added", name)
			continue
		}
		if s.Text == "" || s.School == "" || s.CastingTime == "" || s.Duration == "" {
			t.Errorf("%q is missing rules", name)
		}
	}
	// Only widened onto the list, so they must keep their own rules text.
	// Fireball is deliberately not here: it reaches an artificer through the
	// Artillerist subclass, not through the class list.
	for _, name := range []string{"Cure Wounds", "Haste", "Fabricate",
		"Bigby's Hand", "Animate Objects", "Revivify", "Snare"} {
		s := rs.Spell(name)
		if s == nil {
			t.Fatalf("%q vanished", name)
		}
		found := false
		for _, c := range s.Classes {
			if c == "artificer" {
				found = true
			}
		}
		if !found {
			t.Errorf("%q is not on the artificer list: %v", name, s.Classes)
		}
		if s.Text == "" {
			t.Errorf("%q lost its rules text", name)
		}
	}
}

// The subclasses hand out spells, and a spell id that does not resolve would
// leave a character quietly missing what their subclass promised them.
func TestArtificerSubclassesResolve(t *testing.T) {
	rs := modules(t)
	cl := rs.Class("artificer")
	if cl == nil {
		t.Fatal("no artificer")
	}
	if len(cl.Subclasses) != 4 {
		t.Fatalf("%d subclasses, want 4", len(cl.Subclasses))
	}
	for _, sc := range cl.Subclasses {
		if len(sc.Spells) == 0 {
			t.Errorf("%s hands out no spells", sc.Id)
		}
		for _, sp := range sc.Spells {
			if rs.Spell(sp.Id) == nil {
				t.Errorf("%s: subclass spell %q does not exist", sc.Id, sp.Id)
			}
		}
		for _, blk := range sc.Proficiency {
			for _, tl := range blk.Tools {
				if rs.Item(tl) == nil {
					t.Errorf("%s: tool %q does not exist", sc.Id, tl)
				}
			}
		}
	}
	// Everything the class itself points at has to exist too.
	for _, id := range []string{"thieves-tools", "tinkers-tools", "studded-leather",
		"scale-mail", "crossbow-light", "crossbow-bolts-20", "dungeoneers-pack"} {
		if rs.Item(id) == nil {
			t.Errorf("item %q referenced by the artificer does not exist", id)
		}
	}
}
