package randgen

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// DLight is a light in a dungeon: something holding a fire (a brazier, a
// torch in a sconce, a fire pit, a hearth, candles, a lantern) or a glow of
// magic, in the dungeon's squares (x across, y down; a torch stands on the
// line of the wall it hangs on). worg's map viewer lights the room the party
// is in with them, the way it lights a Dungeon Alchemist export's lights, and
// draws what holds them; lights are put there by the generator (a few rooms,
// lightRooms) or by hand on the map viewer (POST /dnd/dungeon/lights).
type DLight struct {
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	Kind string  `json:"kind"`
	// How far it reaches, in squares; 0 is the kind's own.
	Range float64 `json:"range,omitempty"`
	// warm (the default), white, blue, green, red or violet.
	Colour string `json:"colour,omitempty"`
}

// LightKinds are the kinds worg can draw, with how far each reaches.
var LightKinds = map[string]float64{
	"torch": 4, "brazier": 5, "firepit": 6, "hearth": 5, "candles": 2.5, "lantern": 4, "magic": 4,
}

// lightRooms lights a few rooms, very few: most of a dungeon is dark, and a
// lit room says somebody keeps it lit. A dwelling's hearth is the exception,
// lit in most houses. Each lit room says so in its key.
func (c *ctx) lightRooms(d *Dungeon, kind string) {
	cells := map[int][]int{}
	for i, id := range d.Room {
		if id == 0 || d.Grid[i/d.Cols][i%d.Cols] != '.' {
			continue
		}
		cells[id] = append(cells[id], i)
	}
	busy := map[int]bool{}
	for _, p := range d.Props {
		busy[p.R*d.Cols+p.C] = true
	}
	for _, s := range d.Stairs {
		busy[s.R*d.Cols+s.C] = true
	}
	for _, dr := range d.Doors {
		busy[dr.R*d.Cols+dr.C] = true
	}
	ids := make([]int, 0, len(cells))
	for id := range cells {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	motif := c.opts["motif"]
	rock := func(r, cc int) bool {
		return r < 0 || cc < 0 || r >= d.Rows || cc >= d.Cols || d.Grid[r][cc] == '#'
	}
	// The room's own square nearest its middle.
	middle := func(cs []int) (int, bool) {
		var sr, sc float64
		for _, i := range cs {
			sr += float64(i / d.Cols)
			sc += float64(i % d.Cols)
		}
		sr, sc = sr/float64(len(cs)), sc/float64(len(cs))
		best, bd := -1, math.Inf(1)
		for _, i := range cs {
			if busy[i] {
				continue
			}
			dd := math.Hypot(float64(i/d.Cols)-sr, float64(i%d.Cols)-sc)
			if dd < bd {
				best, bd = i, dd
			}
		}
		return best, best >= 0
	}
	// Places on the room's walls: a floor square's side that faces rock.
	type wallAt struct{ x, y float64 }
	walls := func(cs []int) []wallAt {
		var out []wallAt
		for _, i := range cs {
			r, cc := i/d.Cols, i%d.Cols
			for _, v := range [][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
				if rock(r+v[0], cc+v[1]) {
					out = append(out, wallAt{float64(cc) + 0.5 + float64(v[1])*0.5, float64(r) + 0.5 + float64(v[0])*0.5})
				}
			}
		}
		return out
	}
	colour := ""
	switch motif {
	case "undead", "necromantic":
		colour = "green"
	case "arcane":
		colour = "blue"
	case "infernal", "abyssal":
		colour = "red"
	}
	for _, id := range ids {
		cs := cells[id]
		chance := 14
		if kind == "dwelling" {
			chance = 30
		}
		if kind == "cave" {
			chance = 10
		}
		if !c.chance(chance) || len(cs) < 4 {
			continue
		}
		var lit []DLight
		say := ""
		switch {
		case kind == "dwelling":
			if ws := walls(cs); len(ws) > 0 && c.chance(60) {
				w := ws[c.r.IntN(len(ws))]
				lit = append(lit, DLight{X: w.x, Y: w.y, Kind: "hearth"})
				say = "a fire is banked in the hearth"
			} else if at, ok := middle(cs); ok {
				lit = append(lit, DLight{X: float64(at%d.Cols) + 0.5, Y: float64(at/d.Cols) + 0.5, Kind: "candles"})
				say = "candles burn low on the table"
			}
		case kind == "cave":
			if at, ok := middle(cs); ok {
				lit = append(lit, DLight{X: float64(at%d.Cols) + 0.5, Y: float64(at/d.Cols) + 0.5, Kind: "firepit"})
				say = "a fire pit smoulders, ringed with stones"
			}
		default:
			ws := walls(cs)
			roll := c.r.IntN(100)
			switch {
			case colour == "blue" && roll < 50:
				if at, ok := middle(cs); ok {
					lit = append(lit, DLight{X: float64(at%d.Cols) + 0.5, Y: float64(at/d.Cols) + 0.5, Kind: "magic", Colour: colour})
					say = "a globe of cold light hangs in the air"
				}
			case roll < 45 && len(ws) >= 2:
				a := ws[c.r.IntN(len(ws))]
				b, far := a, 0.0
				for _, w := range ws {
					if dd := math.Hypot(w.x-a.x, w.y-a.y); dd > far {
						b, far = w, dd
					}
				}
				lit = append(lit, DLight{X: a.x, Y: a.y, Kind: "torch", Colour: colour})
				if far > 2 {
					lit = append(lit, DLight{X: b.x, Y: b.y, Kind: "torch", Colour: colour})
					say = "two torches gutter in iron sconces"
				} else {
					say = "a torch gutters in an iron sconce"
				}
			case roll < 80 && len(cs) >= 9:
				if at, ok := middle(cs); ok {
					lit = append(lit, DLight{X: float64(at%d.Cols) + 0.5, Y: float64(at/d.Cols) + 0.5, Kind: "brazier", Colour: colour})
					say = "a brazier of coals glows in the middle of the room"
				}
			default:
				if at, ok := middle(cs); ok {
					lit = append(lit, DLight{X: float64(at%d.Cols) + 0.5, Y: float64(at/d.Cols) + 0.5, Kind: "firepit", Colour: colour})
					say = "a fire pit burns, the ceiling black with its soot"
				}
			}
		}
		if len(lit) == 0 {
			continue
		}
		if colour != "" && kind != "dwelling" && kind != "cave" {
			say += fmt.Sprintf(", its flame %s", colourWord(colour))
		}
		d.Lights = append(d.Lights, lit...)
		for k := range d.Rooms {
			if d.Rooms[k].Id == id {
				t := strings.TrimRight(d.Rooms[k].Text, "\n")
				if t != "" {
					t += "\n"
				}
				d.Rooms[k].Text = t + "- Light :: " + say + "\n"
			}
		}
	}
}

func colourWord(c string) string {
	switch c {
	case "green":
		return "a sickly green"
	case "blue":
		return "a cold blue"
	case "red":
		return "a deep red"
	}
	return c
}
