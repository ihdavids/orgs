// Tests for casting a spell from a bigger slot than it needs: the dice the
// at-higher-levels text adds, and the spells whose higher levels buy
// something this cannot roll.
package dnd

import "testing"

// upcastAtLevel is the entry for one slot level, or a failure when the spell
// does not offer it.
func upcastAtLevel(t *testing.T, rs *Ruleset, id string, level int) SpellUpcast {
	t.Helper()
	c := castOf(t, rs, id, 10, 7, 15, 4)
	for _, up := range c.Upcast {
		if up.Level == level {
			return up
		}
	}
	t.Fatalf("%s offers no %s level cast, got %+v", id, Ordinal(level), c.Upcast)
	return SpellUpcast{}
}

func TestUpcastGrowsHealing(t *testing.T) {
	rs := srd(t)
	// "the healing increases by 1d8 for each slot level above 1st"
	for _, tc := range []struct {
		level int
		heal  string
		note  string
	}{
		{2, "2d8 +4", "+1d8 healing"},
		{3, "3d8 +4", "+2d8 healing"},
		{9, "9d8 +4", "+8d8 healing"},
	} {
		up := upcastAtLevel(t, rs, "cure-wounds", tc.level)
		if up.Heal != tc.heal || up.Note != tc.note {
			t.Fatalf("cure wounds at %s: want %q %q, got %q %q",
				up.Label, tc.heal, tc.note, up.Heal, up.Note)
		}
		if !up.Scales {
			t.Fatalf("cure wounds scales at %s", up.Label)
		}
	}
}

func TestUpcastGrowsDamage(t *testing.T) {
	rs := srd(t)
	// "the damage increases by 1d6 for each slot level above 3rd"
	up := upcastAtLevel(t, rs, "fireball", 5)
	if up.Damage != "10d6" || up.Note != "+2d6 fire" {
		t.Fatalf("fireball at 5th: want 10d6 and +2d6 fire, got %q %q",
			up.Damage, up.Note)
	}
	if up.Detail == "" || up.Detail[:len("cast at 5th level")] != "cast at 5th level" {
		t.Fatalf("an upcast says the level it was cast at, got %q", up.Detail)
	}
	if up.Short != "DEX save DC 15" {
		t.Fatalf("the save is unchanged by the slot, got %q", up.Short)
	}
}

func TestUpcastCountsEveryTwoSlotLevels(t *testing.T) {
	rs := srd(t)
	// "the damage increases by 1d6 for every two slot levels above 2nd"
	for _, tc := range []struct {
		level  int
		damage string
	}{{3, "3d6"}, {4, "4d6"}, {5, "4d6"}, {6, "5d6"}} {
		up := upcastAtLevel(t, rs, "flame-blade", tc.level)
		if up.Damage != tc.damage {
			t.Fatalf("flame blade at %s: want %s, got %s",
				up.Label, tc.damage, up.Damage)
		}
	}
}

func TestUpcastStillOffersALevelItCannotRoll(t *testing.T) {
	rs := srd(t)
	// Magic missile's higher levels buy another dart, not bigger dice, and
	// hold person another target. Both are worth a bigger slot; neither
	// changes what is rolled.
	for _, id := range []string{"magic-missile", "hold-person"} {
		up := upcastAtLevel(t, rs, id, 3)
		if up.Scales {
			t.Fatalf("%s does not roll extra dice at 3rd, got %+v", id, up)
		}
		if up.Note == "" {
			t.Fatalf("%s should say what the bigger slot buys", id)
		}
	}
	if d := upcastAtLevel(t, rs, "magic-missile", 3).Damage; d != "1d4 + 1" {
		t.Fatalf("magic missile rolls the same dice at 3rd, got %q", d)
	}
}

func TestUpcastIsOnlyOfferedWhereItMeansSomething(t *testing.T) {
	rs := srd(t)
	// A cantrip has no slot to spend, and a spell whose text says nothing
	// about higher levels has nothing to offer for one.
	if up := castOf(t, rs, "fire-bolt", 10, 7, 15, 4).Upcast; len(up) != 0 {
		t.Fatalf("a cantrip is never upcast, got %+v", up)
	}
	if up := castOf(t, rs, "shield", 10, 7, 15, 4).Upcast; len(up) != 0 {
		t.Fatalf("shield says nothing about higher levels, got %+v", up)
	}
	// Every offered level is above the spell's own, and none is past 9th.
	ups := castOf(t, rs, "fireball", 10, 7, 15, 4).Upcast
	if len(ups) != 6 {
		t.Fatalf("fireball can be cast from 4th to 9th, got %d levels", len(ups))
	}
	for i, up := range ups {
		if up.Level != 4+i || up.Label != Ordinal(4+i) {
			t.Fatalf("bad level %d: %+v", i, up)
		}
	}
}

func TestDiceExpressionsSurviveTheRoundTrip(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"1d8 +4", "1d8 +4"},
		{"1d4 + 1", "1d4 +1"},
		{"8d6", "8d6"},
		{"2d8 + 1d6", "2d8 + 1d6"},
		{"nothing at all", ""},
	} {
		d := parseDiceExpr(tc.in)
		got := ""
		if d != nil {
			got = d.String()
		}
		if got != tc.want {
			t.Fatalf("parse %q: want %q, got %q", tc.in, tc.want, got)
		}
	}
}
