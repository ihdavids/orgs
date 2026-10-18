package randgen

import (
	"strings"
	"testing"
)

// A few rooms are lit, not most; every light stands on floor or on a wall
// beside its room's floor; a lit room says so in its key.
func TestSomeRoomsAreLit(t *testing.T) {
	rooms, lit, lights := 0, 0, 0
	for seed := uint64(1); seed <= 40; seed++ {
		d := GenerateDungeon(nil, DungeonRequest{Seed: seed, Options: map[string]string{"size": "medium"}})
		rooms += len(d.Rooms)
		for _, r := range d.Rooms {
			if strings.Contains(r.Text, "- Light ::") {
				lit++
			}
		}
		for _, l := range d.Lights {
			lights++
			if _, ok := LightKinds[l.Kind]; !ok {
				t.Errorf("seed %d: unknown light %q", seed, l.Kind)
			}
			// The square it stands in, or one beside the wall it is on, is floor.
			ok := false
			for _, dx := range []float64{-0.25, 0.25} {
				for _, dy := range []float64{-0.25, 0.25} {
					r, c := int(l.Y+dy), int(l.X+dx)
					if r >= 0 && c >= 0 && r < d.Rows && c < d.Cols && d.Grid[r][c] == '.' {
						ok = true
					}
				}
			}
			if !ok {
				t.Errorf("seed %d: %s at %.1f,%.1f is in rock", seed, l.Kind, l.X, l.Y)
			}
		}
	}
	if lit == 0 || lights == 0 {
		t.Fatalf("no room lit in %d rooms", rooms)
	}
	if share := float64(lit) / float64(rooms); share > 0.25 {
		t.Errorf("%.0f%% of rooms lit; it should be very few", share*100)
	}
}
