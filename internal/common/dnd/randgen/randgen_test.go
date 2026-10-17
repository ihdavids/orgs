package randgen

import (
	"reflect"
	"strings"
	"testing"

	"github.com/ihdavids/orgs/internal/common/dnd"
)

func srd(t *testing.T) *dnd.Ruleset {
	t.Helper()
	rs := dnd.NewLibrary(nil).Get("")
	if rs == nil {
		t.Fatal("no built in ruleset")
	}
	return rs
}

// Every generator, every choice of its first select, a run of seeds: a result
// has a title, and its body never starts a line with a heading - the body is
// filed under one heading, and a line of stars in it would split the save in
// two.
func TestEveryGeneratorMakesSomethingFileable(t *testing.T) {
	rs := srd(t)
	for _, info := range Generators(rs) {
		opts := []map[string]string{{}}
		for _, f := range info.Fields {
			for _, ch := range f.Choices {
				opts = append(opts, map[string]string{f.Id: ch.Value})
			}
		}
		for _, o := range opts {
			for seed := uint64(1); seed <= 12; seed++ {
				res, err := Generate(rs, Request{Type: info.Id, Options: o, Seed: seed, Count: 2})
				if err != nil {
					t.Fatalf("%s %v: %v", info.Id, o, err)
				}
				for _, it := range res.Items {
					if strings.TrimSpace(it.Title) == "" {
						t.Errorf("%s %v seed %d: empty title", info.Id, o, seed)
					}
					for _, line := range strings.Split(it.Body, "\n") {
						s := strings.TrimLeft(line, "*")
						if len(s) < len(line) && strings.HasPrefix(s, " ") {
							t.Errorf("%s %v seed %d: body has a heading line %q", info.Id, o, seed, line)
						}
					}
					if it.Tags[0] != "dnd" {
						t.Errorf("%s: tags %v do not start with dnd", info.Id, it.Tags)
					}
				}
			}
		}
	}
}

func TestSameSeedSameResult(t *testing.T) {
	rs := srd(t)
	for _, info := range Generators(rs) {
		a, _ := Generate(rs, Request{Type: info.Id, Seed: 42})
		b, _ := Generate(rs, Request{Type: info.Id, Seed: 42})
		if !reflect.DeepEqual(a, b) {
			t.Errorf("%s: two runs with seed 42 differ", info.Id)
		}
	}
}

// An encounter asked for at a difficulty comes out at that difficulty for
// nearly every party: the search can miss for a tiny party in a sparse
// environment, but not often.
func TestEncountersLandInTheirBand(t *testing.T) {
	rs := srd(t)
	miss, total := 0, 0
	for _, diff := range difficulties {
		for _, lvl := range []string{"1", "3", "5", "9", "13", "17", "20"} {
			for _, env := range []string{"any", "forest", "underdark", "urban"} {
				res, err := Generate(rs, Request{Type: "encounter", Seed: 7, Count: 4,
					Options: map[string]string{"difficulty": diff, "level": lvl, "party": "4", "environment": env}})
				if err != nil {
					t.Fatal(err)
				}
				for _, it := range res.Items {
					total++
					if it.Props["DIFFICULTY"] != diff {
						miss++
					}
				}
			}
		}
	}
	if miss*10 > total {
		t.Errorf("%d of %d encounters missed their difficulty", miss, total)
	}
}

func TestMonstersLoad(t *testing.T) {
	ms := Monsters()
	if len(ms) < 300 {
		t.Fatalf("only %d monsters", len(ms))
	}
	for _, m := range ms {
		if m.Name == "" || len(m.Environments) == 0 {
			t.Errorf("monster %+v is missing a name or a home", m)
		}
	}
}

func TestMultiplier(t *testing.T) {
	cases := []struct {
		n, party int
		want     float64
	}{{1, 4, 1}, {2, 4, 1.5}, {4, 4, 2}, {8, 4, 2.5}, {12, 4, 3}, {20, 4, 4}, {1, 2, 1.5}, {1, 6, 0.5}, {20, 2, 5}}
	for _, c := range cases {
		if got := multiplier(c.n, c.party); got != c.want {
			t.Errorf("multiplier(%d, %d) = %v, want %v", c.n, c.party, got, c.want)
		}
	}
}

func TestOrgTableLinesUp(t *testing.T) {
	got := orgTable([]string{"a", "bb"}, [][]string{{"ccc", "d"}})
	want := "| a   | bb |\n|-----+----|\n| ccc | d  |\n"
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}
