package randgen

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/ihdavids/orgs/internal/common/mapgen"
)

// A map of a region or a world, made by mapgen. It is kept as a Dungeon
// whose picture is the generated map (Backdrop, Own) with the map's makings
// in World, so the map viewer runs a game on it - the party's pin, the fog
// of war, the players' screen - as on a dungeon on a picture. Its squares
// are only for the fog (FogCell pixels a side); there are no rooms.
//
// Everything drawn over the picture - places, roads, pins, words - is in
// fractions of the picture's width and height, so it stays put when the map
// is drawn bigger or smaller.
type World struct {
	Params  mapgen.Params   `json:"params"`
	Strokes []mapgen.Stroke `json:"strokes,omitempty"`
	// Hash is mapgen.Hash of the params the picture was drawn from.
	Hash   string       `json:"hash,omitempty"`
	Hex    WorldHex     `json:"hex"`
	Sight  float64      `json:"sight,omitempty"` // how far the party sees, in fog squares
	Roads  []WorldRoad  `json:"roads,omitempty"`
	Places []WorldPlace `json:"places,omitempty"`
	Pins   []WorldPin   `json:"pins,omitempty"`
	Labels []WorldLabel `json:"labels,omitempty"`
}

// WorldHex is the dotted hex grid over the map: a hex's width in pixels of
// the picture (pointy-topped, rows of hexes, odd rows pushed half a hex
// east - mapgen.HexCenter's layout).
type WorldHex struct {
	Show    bool    `json:"show"`
	Size    float64 `json:"size"`
	Opacity float64 `json:"opacity"`
}

type WorldRoad struct {
	Kind string    `json:"kind"` // road, highway, trail, sea
	Name string    `json:"name,omitempty"`
	Pts  []float64 `json:"pts"` // x0, y0, x1, y1, ...
}

type WorldPlace struct {
	Id     int     `json:"id"`
	Kind   string  `json:"kind"` // city, town, village, castle, ruin, port, camp...
	Name   string  `json:"name"`
	Note   string  `json:"note,omitempty"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Secret bool    `json:"secret,omitempty"` // the GM's only
}

type WorldPin struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Label  string  `json:"label,omitempty"`
	Colour string  `json:"colour,omitempty"`
	Secret bool    `json:"secret,omitempty"`
}

type WorldLabel struct {
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	Text  string  `json:"text"`
	Size  float64 `json:"size,omitempty"`  // in fractions of the picture's width
	Angle float64 `json:"angle,omitempty"` // degrees
	Style string  `json:"style,omitempty"` // region, sea, note
}

// FogCell is the size of a fog square on a map w × h pixels: about forty
// across the shorter side, never under 16 pixels.
func FogCell(w, h int) float64 {
	return math.Max(16, math.Round(float64(min(w, h))/40))
}

// FitWorld shapes a map's dungeon to its picture (w × h, at src): the
// backdrop, and a grid of fog squares over it, all floor and no rooms.
func (d *Dungeon) FitWorld(src string, w, h int) {
	ppc := FogCell(w, h)
	rows, cols := int(math.Ceil(float64(h)/ppc)), int(math.Ceil(float64(w)/ppc))
	name := "generated map"
	if d.Backdrop != nil && d.Backdrop.Name != "" {
		name = d.Backdrop.Name
	}
	d.Backdrop = &Backdrop{Src: src, Width: w, Height: h, Ppc: ppc, Own: true, Kind: "world", Name: name}
	if d.Rows != rows || d.Cols != cols || len(d.Grid) != rows || len(d.Room) != rows*cols {
		d.Rows, d.Cols = rows, cols
		d.Grid = make([]string, rows)
		for r := range d.Grid {
			d.Grid[r] = strings.Repeat(".", cols)
		}
		d.Room = make([]int, rows*cols)
	}
	d.Rooms = []DRoom{}
	if d.Doors == nil {
		d.Doors = []DDoor{}
	}
	if d.Stairs == nil {
		d.Stairs = []DStair{}
	}
	if d.Props == nil {
		d.Props = []DProp{}
	}
	if d.Labels == nil {
		d.Labels = []DLabel{}
	}
	if d.World.Hex.Size <= 0 {
		d.World.Hex.Size = math.Max(24, math.Round(float64(min(w, h))/24))
		if d.World.Params.Generator == "hex" {
			d.World.Hex.Size = d.World.Params.HexPx
		}
	}
	if d.World.Hex.Opacity <= 0 {
		d.World.Hex.Opacity = 0.35
	}
	if d.World.Sight <= 0 {
		d.World.Sight = 4
	}
}

// WorldOrgFile is a map as an org file: its params as properties of its Map
// heading (edit one and the map is drawn again when next opened), the
// picture, and a heading for each place, to write about.
func (d *Dungeon) WorldOrgFile(picture, data string) string {
	var b strings.Builder
	b.WriteString("#+TITLE: " + oneLine(d.Name) + "\n")
	b.WriteString("#+STARTUP: overview\n")
	if data != "" {
		b.WriteString("#+DND_DUNGEON: " + data + "\n")
	}
	b.WriteString("#+DND_WORLD: t\n")
	if s := strings.TrimSpace(d.Story); s != "" {
		b.WriteString("\n" + s + "\n")
	}
	b.WriteString("\n* Map\n")
	b.WriteString("  :PROPERTIES:\n")
	for _, kv := range d.World.Params.Props() {
		b.WriteString(fmt.Sprintf("  :%-12s %s\n", kv[0]+":", kv[1]))
	}
	b.WriteString("  :END:\n")
	b.WriteString("  The map is drawn from these: change one and it is drawn again the next time the map is opened.\n")
	if len(d.World.Strokes) > 0 {
		b.WriteString(fmt.Sprintf("  (%d strokes of terrain painted by hand are kept with the map's data.)\n", len(d.World.Strokes)))
	}
	if picture != "" {
		b.WriteString("  - Map :: [[file:" + picture + "]]\n")
	}
	if len(d.World.Places) > 0 {
		b.WriteString("* Places\n")
		places := append([]WorldPlace(nil), d.World.Places...)
		sort.SliceStable(places, func(a, c int) bool { return places[a].Id < places[c].Id })
		for _, p := range places {
			b.WriteString("** " + oneLine(nonEmpty(p.Name, "Unnamed")) + "\n")
			b.WriteString("   :PROPERTIES:\n")
			b.WriteString(fmt.Sprintf("   :%-8s %s\n", "KIND:", oneLine(nonEmpty(p.Kind, "town"))))
			if p.Secret {
				b.WriteString(fmt.Sprintf("   :%-8s t\n", "SECRET:"))
			}
			b.WriteString("   :END:\n")
			if n := strings.TrimSpace(p.Note); n != "" {
				b.WriteString(indentLines(n, "   "))
			}
		}
	}
	return b.String()
}

func nonEmpty(s, or string) string {
	if strings.TrimSpace(s) == "" {
		return or
	}
	return s
}
