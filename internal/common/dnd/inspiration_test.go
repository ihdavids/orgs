// Tests for inspiration: the one bit on a sheet the rules neither earn nor
// spend for you, and that it survives a trip out to an org character sheet
// and back, which is where it actually lives.
package dnd

import "testing"

func inspired() *Character {
	return &Character{
		Name:      "Test",
		Race:      "human",
		Classes:   []ClassLevel{{Class: "fighter", Level: 3}},
		Abilities: map[string]int{"str": 16, "dex": 12, "con": 14, "int": 10, "wis": 10, "cha": 8},
	}
}

func TestInspirationToggle(t *testing.T) {
	c := inspired()
	// The marker on the sheet sends toggle and nothing else, so one press has
	// to do both jobs.
	if msg, err := ApplyInspiration(c, InspirationToggle); err != nil || !c.Inspiration {
		t.Fatalf("first toggle = %q, %v, inspiration %v", msg, err, c.Inspiration)
	}
	if msg, err := ApplyInspiration(c, ""); err != nil || c.Inspiration {
		t.Fatalf("empty action should toggle back: %q, %v, inspiration %v",
			msg, err, c.Inspiration)
	}
}

func TestInspirationGainAndSpend(t *testing.T) {
	c := inspired()
	if _, err := ApplyInspiration(c, InspirationGain); err != nil || !c.Inspiration {
		t.Fatalf("gain did not take")
	}
	// Two people at the table pressing the same button is not an error.
	msg, err := ApplyInspiration(c, InspirationGain)
	if err != nil || !c.Inspiration || msg != "already holding inspiration" {
		t.Errorf("gaining twice = %q, %v", msg, err)
	}
	if _, err := ApplyInspiration(c, InspirationSpend); err != nil || c.Inspiration {
		t.Fatalf("spend did not take")
	}
	msg, err = ApplyInspiration(c, InspirationSpend)
	if err != nil || c.Inspiration || msg != "no inspiration to spend" {
		t.Errorf("spending twice = %q, %v", msg, err)
	}
	if _, err := ApplyInspiration(c, "brood"); err == nil {
		t.Errorf("an action nobody has heard of should be refused")
	}
	if _, err := ApplyInspiration(nil, InspirationGain); err == nil {
		t.Errorf("no character should be refused")
	}
}

// The point of the endpoint is that the sheet and the file agree, so what the
// server writes has to come back out of the org file it wrote.
func TestInspirationSurvivesTheOrgFile(t *testing.T) {
	rs := srd(t)
	c := inspired()
	if _, err := ApplyInspiration(c, InspirationGain); err != nil {
		t.Fatal(err)
	}
	back, err := ParseOrg(RenderOrg(c, rs), rs)
	if err != nil {
		t.Fatal(err)
	}
	if !back.Inspiration {
		t.Errorf("inspiration did not come back out of the org file")
	}
	if _, err := ApplyInspiration(back, InspirationSpend); err != nil {
		t.Fatal(err)
	}
	again, err := ParseOrg(RenderOrg(back, rs), rs)
	if err != nil {
		t.Fatal(err)
	}
	if again.Inspiration {
		t.Errorf("spent inspiration came back held")
	}
}
