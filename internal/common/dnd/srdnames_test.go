package dnd

import "testing"

// srdRuleset is the built in SRD with no modules loaded on top, because the
// renamed spells have to be found by a ruleset that is nothing but the SRD.
func srdRuleset(t *testing.T) *Ruleset {
	t.Helper()
	lib := NewLibrary(nil)
	lib.Resolve()
	rs := lib.Get("srd")
	if rs == nil {
		t.Fatal("no built in srd ruleset")
	}
	return rs
}

// Every Player's Handbook spell the SRD renamed has to be findable under the
// name the books use, because that is the name on every character sheet that
// came from anywhere else. A sheet saying Tasha's Hideous Laughter used to
// render a row with no casting time, no range and nothing to roll.
func TestBookNamesFindTheRenamedSrdSpells(t *testing.T) {
	rs := srdRuleset(t)
	for id, names := range srdRenamedSpells {
		if rs.Spell(id) == nil {
			t.Errorf("the srd has no spell %q for the alias table to name", id)
			continue
		}
		for _, name := range names {
			got := rs.Spell(name)
			if got == nil {
				t.Errorf("%q found nothing, want %s", name, id)
				continue
			}
			if got.Id != id {
				t.Errorf("%q found %s, want %s", name, got.Id, id)
			}
		}
	}
}

// D&D Beyond sends a typographic apostrophe, so both spellings have to land.
func TestBookNameMatchesEitherApostrophe(t *testing.T) {
	rs := srdRuleset(t)
	for _, name := range []string{"Tasha's Hideous Laughter", "Tasha’s Hideous Laughter"} {
		if s := rs.Spell(name); s == nil || s.Id != "hideous-laughter" {
			t.Errorf("%q = %v, want hideous-laughter", name, s)
		}
	}
}

// An alias is a last resort, so it must not answer for a name that is not
// there at all - a spell the ruleset has never heard of has to stay unmatched
// rather than be quietly turned into the nearest thing with a wizard's name.
func TestAliasesDoNotInventMatches(t *testing.T) {
	rs := srdRuleset(t)
	for _, name := range []string{"Tasha's Caustic Brew", "Aganazzar's Scorcher",
		"Bigby's Utterly Made Up Spell"} {
		if s := rs.Spell(name); s != nil {
			t.Errorf("%q matched %s, it is not in the srd at all", name, s.Id)
		}
	}
	// And a name a spell owns outright still wins it.
	if s := rs.Spell("Acid Arrow"); s == nil || s.Id != "acid-arrow" {
		t.Errorf("Acid Arrow = %v", s)
	}
}

// A module can alias anything it adds, and the ruleset is still indexed by
// the real names first.
func TestModuleDeclaredAliases(t *testing.T) {
	lib := NewLibrary(nil)
	lib.LoadBytes([]byte(`
id: srd
spells:
  - id: "witch-bolt"
    name: "Witch Bolt"
    level: 1
    aliases: ["Lightning Tether"]
`), "test")
	lib.Resolve()
	rs := lib.Get("srd")
	if s := rs.Spell("Lightning Tether"); s == nil || s.Id != "witch-bolt" {
		t.Fatalf("declared alias did not resolve: %v", s)
	}
	if s := rs.Spell("Witch Bolt"); s == nil || s.Id != "witch-bolt" {
		t.Fatalf("the real name stopped working: %v", s)
	}
}

// The import path has to agree with the sheet path, or a character imported
// from D&D Beyond arrives carrying spells with no rules on them.
func TestImportMatchesBookNames(t *testing.T) {
	rs := srdRuleset(t)
	idx := newNameIndex(rs)
	for id, names := range srdRenamedSpells {
		for _, name := range names {
			got := idx.spell(name)
			if got == nil || got.Id != id {
				t.Errorf("import of %q = %v, want %s", name, got, id)
			}
		}
	}
	if got := idx.spell("Tasha's Caustic Brew"); got != nil {
		t.Errorf("import invented a match for a spell not in the srd: %s", got.Id)
	}
}
