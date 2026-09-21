package dnd

import "testing"

// spellClasses is the class list a spell is carrying, for the assertions
// below to read against.
func spellClasses(t *testing.T, rs *Ruleset, name string) []string {
	t.Helper()
	s := rs.Spell(name)
	if s == nil {
		t.Fatalf("no spell %q", name)
	}
	return s.Classes
}

func hasClass(list []string, want string) bool {
	for _, c := range list {
		if c == want {
			return true
		}
	}
	return false
}

// A widened list hands the spell to the class without touching anything else
// about it. This is the whole point of the mechanism: an entry under spells:
// would replace the SRD's copy, so the rules text has to survive.
func TestSpellListsWidenWithoutReplacing(t *testing.T) {
	l := NewLibrary(nil)
	l.LoadBytes([]byte(`
id: srd
spellLists:
  druid: ["revivify", "Cone of Cold"]
`), "test")
	l.Resolve()
	rs := l.Get("srd")

	for _, name := range []string{"revivify", "Cone of Cold"} {
		s := rs.Spell(name)
		if !hasClass(s.Classes, "druid") {
			t.Errorf("%s was not handed to the druid: %v", name, s.Classes)
		}
		// Everything that made it that spell is still there.
		if s.Name == "" || s.Text == "" || s.School == "" || s.Level == 0 {
			t.Errorf("%s was clobbered: name=%q level=%d school=%q text=%v",
				name, s.Name, s.Level, s.School, s.Text != "")
		}
	}
	// The classes it already had are kept, not replaced.
	if c := spellClasses(t, rs, "revivify"); !hasClass(c, "cleric") || !hasClass(c, "paladin") {
		t.Errorf("revivify lost the classes it already had: %v", c)
	}
}

// Index() runs whenever the ruleset is re-indexed, so applying the additions
// there has to be safe to do twice.
func TestSpellListsAreIdempotent(t *testing.T) {
	l := NewLibrary(nil)
	l.LoadBytes([]byte(`
id: srd
spellLists:
  druid: ["augury"]
`), "test")
	l.Resolve()
	rs := l.Get("srd")
	first := len(spellClasses(t, rs, "augury"))
	rs.Index()
	rs.Index()
	if got := len(spellClasses(t, rs, "augury")); got != first {
		t.Fatalf("re-indexing added the class again: %d then %d", first, got)
	}
}

// Two modules that each widen the same list both get their way.
func TestSpellListsFromSeveralModulesAccumulate(t *testing.T) {
	l := NewLibrary(nil)
	l.LoadBytes([]byte("id: srd\nspellLists:\n  druid: [\"augury\"]\n"), "a")
	l.LoadBytes([]byte("id: srd\nspellLists:\n  druid: [\"divination\"]\n"), "b")
	l.Resolve()
	rs := l.Get("srd")
	for _, name := range []string{"augury", "divination"} {
		if !hasClass(spellClasses(t, rs, name), "druid") {
			t.Errorf("%s was lost when a second module widened the same list", name)
		}
	}
}

// A module written against a fuller ruleset must still load against a thinner
// one, so a spell that is not there is passed over rather than being an error.
func TestSpellListsIgnoreUnknownSpells(t *testing.T) {
	l := NewLibrary(nil)
	l.LoadBytes([]byte(`
id: srd
spellLists:
  druid: ["no-such-spell-at-all", "augury"]
`), "test")
	l.Resolve()
	if len(l.Errors) > 0 {
		t.Fatalf("an unknown spell was an error: %v", l.Errors)
	}
	rs := l.Get("srd")
	if !hasClass(spellClasses(t, rs, "augury"), "druid") {
		t.Error("the spell that did exist was skipped too")
	}
}

// The shipped druid module has to hold the whole druid list together: the
// spells it adds outright and the ones it only widens the list with.
func TestShippedDruidListIsComplete(t *testing.T) {
	l := NewLibrary([]string{"../../../templates/dnd"})
	l.Resolve()
	if len(l.Errors) > 0 {
		t.Fatalf("module load errors: %v", l.Errors)
	}
	rs := l.Get("srd")

	// Written out in druid-spells.yaml.
	added := []string{"Thorn Whip", "Primal Savagery", "Absorb Elements",
		"Ice Knife", "Healing Spirit", "Summon Beast", "Erupting Earth",
		"Guardian of Nature", "Maelstrom", "Investiture of Flame",
		"Investiture of Ice", "Investiture of Stone", "Investiture of Wind",
		"Whirlwind", "Draconic Transformation"}
	// Only widened onto the list - these must keep their SRD rules text.
	widened := []string{"Revivify", "Divination", "Fire Shield", "Cone of Cold",
		"Flesh to Stone", "Symbol", "Incendiary Cloud", "Augury",
		"Continual Flame", "Enlarge/Reduce", "Protection from Evil and Good"}

	for _, name := range append(append([]string{}, added...), widened...) {
		s := rs.Spell(name)
		if s == nil {
			t.Errorf("%q is not in the ruleset", name)
			continue
		}
		if !hasClass(s.Classes, "druid") {
			t.Errorf("%q is not a druid spell: %v", name, s.Classes)
		}
		if s.Text == "" || s.School == "" || s.CastingTime == "" ||
			s.Range == "" || s.Duration == "" {
			t.Errorf("%q is missing rules: school=%q time=%q range=%q dur=%q text=%v",
				name, s.School, s.CastingTime, s.Range, s.Duration, s.Text != "")
		}
	}

	// Thorn Whip is the one PHB cantrip the SRD leaves out, so it is the one
	// worth pinning by its numbers rather than just its presence.
	whip := rs.Spell("Thorn Whip")
	if whip.Level != 0 || !whip.Attack || whip.Damage != "1d6 piercing" {
		t.Errorf("thorn whip = level %d attack=%v damage=%q",
			whip.Level, whip.Attack, whip.Damage)
	}
}

// A concentration spell has to say so in both places, or the sheet shows a
// duration the concentration marker disagrees with.
func TestDruidSpellsAgreeAboutConcentration(t *testing.T) {
	l := NewLibrary([]string{"../../../templates/dnd"})
	l.Resolve()
	rs := l.Get("srd")
	for i := range rs.Spells {
		s := &rs.Spells[i]
		says := len(s.Duration) >= 13 && s.Duration[:13] == "Concentration"
		if says != s.Concentration {
			t.Errorf("%s: duration %q but concentration=%v",
				s.Id, s.Duration, s.Concentration)
		}
	}
}

// spellcast.go reads a spell's prose to work out what a successful save gets
// you, and it recognises the books' phrasing rather than a paraphrase. Getting
// that wrong is quiet and it matters: the sheet tells the player a successful
// save avoided all the damage when it only halved it. These are the spells in
// the shipped modules that halve, checked through the same path the sheet uses.
func TestHalvingSpellsSayTheyHalve(t *testing.T) {
	l := NewLibrary([]string{"../../../templates/dnd"})
	l.Resolve()
	rs := l.Get("srd")
	c := &Character{Name: "T", Race: "human",
		Abilities: map[string]int{"str": 10, "dex": 14, "con": 14, "int": 10, "wis": 20, "cha": 10},
		Classes:   []ClassLevel{{Class: "druid", Level: 17}}}

	halves := []string{"wither-and-bloom", "erupting-earth", "tidal-wave",
		"investiture-of-flame", "investiture-of-ice", "investiture-of-wind",
		"whirlwind", "draconic-transformation", "summon-draconic-spirit"}
	allOrNothing := []string{"thunderclap", "infestation", "earth-tremor",
		"ice-knife", "dust-devil", "maelstrom"}

	check := func(ids []string, want string) {
		for _, id := range ids {
			sp := rs.Spell(id)
			if sp == nil {
				t.Errorf("no spell %s", id)
				continue
			}
			c.Spells = []KnownSpell{{Id: sp.Id, Name: sp.Name, Level: sp.Level, Prepared: true}}
			s := Compute(c, rs)
			got := ""
			for _, lv := range s.SpellLevels {
				for _, e := range lv.Spells {
					if e.Id == id {
						got = e.Cast.SaveFor
					}
				}
			}
			if got != want {
				t.Errorf("%s: the sheet says %q, want %q - check the wording of its text",
					id, got, want)
			}
		}
	}
	check(halves, "half as much damage on a success")
	check(allOrNothing, "no damage on a success")
}
