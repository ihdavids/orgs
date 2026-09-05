// Tests for the appearance step of the character builder: the suggestions it
// offers, the answers it accepts and the ones it turns away.
package dnd

import (
	"strings"
	"testing"
)

func fieldById(fs []Field, id string) Field {
	for _, f := range fs {
		if f.Id == id {
			return f
		}
	}
	return Field{}
}

func TestAppearanceValidation(t *testing.T) {
	rs := &Ruleset{}
	c := &Character{Race: "human", Classes: []ClassLevel{{Class: "wizard", Level: 1}}}
	fs := AppearanceFields(rs, c)
	cases := []struct {
		id, in, want string
		bad          bool
	}{
		{id: "age", in: "44", want: "44"},
		{id: "age", in: "44 years", want: "44"},
		{id: "age", in: "hazel", bad: true},
		{id: "age", in: "900", bad: true},
		{id: "height", in: "33", bad: true},
		{id: "height", in: `5'6"`, want: `5'6"`},
		{id: "height", in: "5'6", want: `5'6"`},
		{id: "height", in: "5 ft 6 in", want: `5'6"`},
		{id: "height", in: "168 cm", want: "168 cm"},
		{id: "height", in: "1.7m", want: "170 cm"},
		{id: "height", in: "66", want: `5'6"`},
		{id: "height", in: "tall", bad: true},
		{id: "weight", in: "22", bad: true},
		{id: "weight", in: "160", want: "160 lb."},
		{id: "weight", in: "160 lb.", want: "160 lb."},
		{id: "weight", in: "73 kg", want: "73 kg"},
		{id: "eyes", in: "11", bad: true},
		{id: "eyes", in: "hazel", want: "hazel"},
		{id: "skin", in: "66", bad: true},
		{id: "hair", in: "77", bad: true},
		{id: "hair", in: "salt and pepper", want: "salt and pepper"},
		{id: "hair", in: "", want: ""},
	}
	for _, tc := range cases {
		got, err := ValidateField(fieldById(fs, tc.id), tc.in)
		if tc.bad {
			if err == nil {
				t.Errorf("%s %q: expected an error, got %q", tc.id, tc.in, got)
			} else {
				t.Logf("%s %q rejected: %s", tc.id, tc.in, err)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s %q: unexpected error %s", tc.id, tc.in, err)
		} else if got != tc.want {
			t.Errorf("%s %q: got %q want %q", tc.id, tc.in, got, tc.want)
		}
	}
}

func TestSuggestionsAreRaceAndClassAware(t *testing.T) {
	rs := &Ruleset{}
	for _, c := range []*Character{
		{Race: "dwarf", Subrace: "hill-dwarf", Classes: []ClassLevel{{Class: "druid", Level: 3}}},
		{Race: "tiefling", Classes: []ClassLevel{{Class: "warlock", Level: 3}}},
		{Race: "dragonborn", Classes: []ClassLevel{{Class: "fighter", Level: 3}}},
		{},
	} {
		fs := AppearanceFields(rs, c)
		for _, f := range fs {
			if len(f.Options) == 0 {
				t.Errorf("%s %s: no suggestions", c.Race, f.Id)
			}
			for _, o := range f.Options {
				if _, err := ValidateField(f, o.Name); err != nil {
					t.Errorf("%s %s: suggestion %q does not validate: %s", c.Race, f.Id, o.Name, err)
				}
			}
		}
		t.Logf("%s/%v age=%v", c.Race, c.Classes, optNames(fieldById(fs, "age").Options))
		t.Logf("%s/%v height=%v", c.Race, c.Classes, optNames(fieldById(fs, "height").Options))
		t.Logf("%s/%v weight=%v", c.Race, c.Classes, optNames(fieldById(fs, "weight").Options))
		t.Logf("%s/%v eyes=%v", c.Race, c.Classes, optNames(fieldById(fs, "eyes").Options))
		t.Logf("%s/%v hair=%v", c.Race, c.Classes, optNames(fieldById(fs, "hair").Options))
		t.Logf("%s/%v skin=%v", c.Race, c.Classes, optNames(fieldById(fs, "skin").Options))
	}
}

func optNames(os []Option) []string {
	out := []string{}
	for _, o := range os {
		out = append(out, o.Name)
	}
	return out
}

func srd(t *testing.T) *Ruleset {
	t.Helper()
	lib := NewLibrary(nil)
	rs := lib.Get(DefaultRuleset)
	if rs == nil {
		t.Fatalf("no srd ruleset, errors: %v", lib.Errors)
	}
	return rs
}

// run a whole session by answering every prompt randomly, which is what
// "orgs dnd random" does.
func TestRandomCharacterGetsAnAppearance(t *testing.T) {
	rs := srd(t)
	s := NewSession("t1", &NewSessionRequest{Ruleset: "srd", Name: "Test", Level: 3}, nil)
	p := s.Next(rs)
	for i := 0; i < 200 && !p.Done; i++ {
		ans := &Answer{Session: s.Id, Step: p.Step, Random: true}
		next, err := s.Apply(rs, ans)
		if err != nil {
			t.Fatalf("step %s: %s", p.Step, err)
		}
		p = next
	}
	if !p.Done {
		t.Fatalf("never finished")
	}
	c := s.Finish(rs)
	t.Logf("%s the %s %s: age %s, %s, %s, eyes %s, skin %s, hair %s",
		c.Name, c.Race, c.PrimaryClass().Class, c.Age, c.Height, c.Weight, c.Eyes, c.Skin, c.Hair)
	for _, v := range []string{c.Age, c.Height, c.Weight, c.Eyes, c.Skin, c.Hair} {
		if strings.TrimSpace(v) == "" {
			t.Errorf("a random character should be fully described, got %+v", c)
		}
	}
	if len(s.ReplayErrors) > 0 {
		t.Errorf("replay errors: %v", s.ReplayErrors)
	}
}

func TestBadAppearanceIsRejectedAndGoodOneSticks(t *testing.T) {
	rs := srd(t)
	s := NewSession("t2", &NewSessionRequest{Ruleset: "srd", Name: "Test", Level: 1}, nil)
	p := s.Next(rs)
	for i := 0; i < 200 && !p.Done && p.Step != "details"; i++ {
		next, err := s.Apply(rs, &Answer{Session: s.Id, Step: p.Step, Random: true})
		if err != nil {
			t.Fatalf("step %s: %s", p.Step, err)
		}
		p = next
	}
	if p.Step != "details" {
		t.Fatalf("never reached the appearance step (at %q)", p.Step)
	}
	if len(p.Fields) != 6 {
		t.Fatalf("expected 6 fields, got %d", len(p.Fields))
	}
	for _, f := range p.Fields {
		if len(f.Options) == 0 {
			t.Errorf("field %s has no suggestions", f.Id)
		}
	}
	t.Logf("race %s/%s help=%q", s.Char.Race, s.Char.Subrace, p.Help)

	// the values from the bug report
	_, err := s.Apply(rs, &Answer{Session: s.Id, Step: "details",
		Values: []string{"44", "33", "22", "11", "66", "77"}})
	if err == nil {
		t.Fatalf("nonsense appearance was accepted")
	}
	t.Logf("rejected: %s", err)
	if !strings.Contains(err.Error(), "eye colour") {
		t.Errorf("the eye colour should have been called out: %s", err)
	}
	if _, done := s.Answers["details"]; done {
		t.Errorf("a rejected answer should not be recorded")
	}

	// and a sensible one - taken from what this race is actually offered - is
	// accepted, and written down in the form the sheet wants
	height := p.Fields[1].Options[2].Name // the average height for this race
	inches, _, _ := ParseHeight(height)
	weight := p.Fields[2].Options[2].Name
	pounds, _, _ := ParseWeight(weight)
	next, err := s.Apply(rs, &Answer{Session: s.Id, Step: "details",
		Values: []string{"44 years",
			sprintf("%d ft %d in", int(inches)/12, int(inches)%12),
			sprintf("%d", int(pounds)),
			"hazel", "olive", "black"}})
	if err != nil {
		t.Fatalf("good appearance rejected: %s", err)
	}
	c := s.Char
	if c.Age != "44" || c.Height != height || c.Weight != weight || c.Eyes != "hazel" {
		t.Errorf("not normalised: age=%q height=%q (want %q) weight=%q (want %q) eyes=%q",
			c.Age, c.Height, height, c.Weight, weight, c.Eyes)
	}
	_ = next

	// going back and forward again must not change anything (replay is
	// deterministic, which is the rule the builder lives by)
	before := *s.Char
	s.Apply(rs, &Answer{Back: true})
	s.Apply(rs, &Answer{Session: s.Id, Step: "details",
		Values: []string{"44", height, weight, "hazel", "olive", "black"}})
	if s.Char.Height != before.Height || s.Char.Age != before.Age {
		t.Errorf("replay changed the appearance: %q/%q vs %q/%q",
			s.Char.Age, s.Char.Height, before.Age, before.Height)
	}
}
