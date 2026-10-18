package mapgen

import (
	"math"
	"testing"
)

// Towns land on land, as many as asked for; roads join them and never cross
// the sea; the same seed is the same map.
func TestSettle(t *testing.T) {
	p := Defaults()
	p.Width, p.Height, p.Seed = 768, 512, 7
	none := Settle(p, nil, SettleOptions{Density: 0, Roads: true})
	if len(none.Places) != 0 || len(none.Roads) != 0 {
		t.Fatalf("density 0 made %d places", len(none.Places))
	}
	s := Settle(p, nil, SettleOptions{Density: 0.6, Roads: true})
	if len(s.Places) < 8 {
		t.Fatalf("only %d places", len(s.Places))
	}
	tr := Generate(p, nil)
	cities := 0
	for _, pl := range s.Places {
		if tr.sample(pl.X*float64(p.Width), pl.Y*float64(p.Height)) < 0 {
			t.Errorf("%s (%s) is in the sea", pl.Name, pl.Kind)
		}
		if pl.Kind == "city" {
			cities++
		}
	}
	if cities < 1 {
		t.Errorf("no city among %d places", len(s.Places))
	}
	if len(s.Roads) < len(s.Places)/2 {
		t.Errorf("%d roads for %d places", len(s.Roads), len(s.Places))
	}
	wet := 0
	total := 0
	for _, r := range s.Roads {
		for i := 0; i+3 < len(r.Pts); i += 2 {
			for k := 0.0; k <= 1; k += 0.25 {
				x := (r.Pts[i] + (r.Pts[i+2]-r.Pts[i])*k) * float64(p.Width)
				y := (r.Pts[i+1] + (r.Pts[i+3]-r.Pts[i+1])*k) * float64(p.Height)
				total++
				if tr.sample(x, y) < -0.05 {
					wet++
				}
			}
		}
	}
	if float64(wet) > math.Max(2, float64(total)*0.01) {
		t.Errorf("roads run through the sea at %d of %d points", wet, total)
	}
	names := map[string]bool{}
	for _, pl := range s.Places {
		if names[pl.Name] {
			t.Errorf("two places called %s", pl.Name)
		}
		names[pl.Name] = true
	}
	again := Settle(p, nil, SettleOptions{Density: 0.6, Roads: true})
	if len(again.Places) != len(s.Places) || again.Places[0] != s.Places[0] {
		t.Error("the same seed settled differently")
	}
	h := Defaults()
	h.Generator, h.Hexes, h.Seed = "hex", 48, 3
	hs := Settle(h, nil, SettleOptions{Density: 0.5, Roads: true})
	if len(hs.Places) == 0 || len(hs.Roads) == 0 {
		t.Errorf("hex world: %d places, %d roads", len(hs.Places), len(hs.Roads))
	}
}
