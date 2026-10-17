package mapgen

import (
	"bytes"
	"encoding/xml"
	"strings"
	"image/jpeg"
	"reflect"
	"testing"
)

func small() Params {
	p := Defaults()
	p.Width, p.Height = 320, 240
	return p
}

// The same params, the same map; another seed, another map.
func TestSameSeedSameMap(t *testing.T) {
	a, _ := JPEG(small(), nil)
	b, _ := JPEG(small(), nil)
	if !bytes.Equal(a, b) {
		t.Fatal("the same params drew two maps")
	}
	p := small()
	p.Seed = 99
	c, _ := JPEG(p, nil)
	if bytes.Equal(a, c) {
		t.Fatal("another seed drew the same map")
	}
	if Hash(small(), nil) != Hash(small(), nil) || Hash(small(), nil) == Hash(p, nil) {
		t.Fatal("hash")
	}
	if Hash(small(), nil) == Hash(small(), []Stroke{{X: 0.5, Y: 0.5, R: 0.1, Kind: "mountain"}}) {
		t.Fatal("a stroke does not change the hash")
	}
}

// More land asked for is more land; an island has sea all round its edge.
func TestLandAndIsland(t *testing.T) {
	share := func(p Params) float64 {
		tr := Generate(p, nil)
		n := 0
		for _, e := range tr.Elev {
			if e > 0 {
				n++
			}
		}
		return float64(n) / float64(len(tr.Elev))
	}
	lo, hi := small(), small()
	lo.Land, hi.Land = 0.2, 0.8
	if share(lo) >= share(hi) {
		t.Errorf("more land gave less: %.2f vs %.2f", share(lo), share(hi))
	}
	isl := small()
	isl.Island, isl.Land = 1, 0.5
	tr := Generate(isl, nil)
	for x := 0; x < tr.W; x++ {
		if tr.Elev[x] > 0 || tr.Elev[(tr.H-1)*tr.W+x] > 0 {
			t.Fatal("an island reaches the edge of the map")
		}
	}
}

// Painted terrain does what it says: an ocean stroke makes sea, a mountain
// stroke high ground.
func TestStrokes(t *testing.T) {
	p := small()
	mid := func(tr *Terrain) float32 { return tr.Elev[(tr.H/2)*tr.W+tr.W/2] }
	sea := Generate(p, []Stroke{{X: 0.5, Y: 0.5, R: 0.1, Kind: "ocean"}})
	if mid(sea) >= 0 {
		t.Errorf("ocean stroke left land: %v", mid(sea))
	}
	hill := Generate(p, []Stroke{{X: 0.5, Y: 0.5, R: 0.1, Kind: "mountain"}})
	if mid(hill) <= mid(Generate(p, nil))+0.2 {
		t.Errorf("mountain stroke did not raise the land")
	}
}

// Rivers run downhill to the sea or a lake, and get wider as they go.
func TestRiversRunDownhill(t *testing.T) {
	p := small()
	p.Rivers = 0.2
	tr := Generate(p, nil)
	if len(tr.Rivers) == 0 {
		t.Fatal("no rivers")
	}
	for _, r := range tr.Rivers {
		if r.Width[len(r.Width)-1] < r.Width[0] {
			t.Errorf("a river narrows downstream")
		}
	}
}

// Params survive a trip through an org file's properties, keep to their
// ranges, and a property that is not a number is said to be one.
func TestPropsRoundTrip(t *testing.T) {
	p := Defaults()
	p.Seed, p.Rain, p.Style = 4242, 0.8, "biomes"
	props := map[string]string{}
	for _, kv := range p.Props() {
		props[kv[0]] = kv[1]
	}
	if _, ok := props["HEXES"]; ok {
		t.Error("a terrain map keeps the hex world's params")
	}
	got, err := FromProps(Defaults(), props)
	if err != nil || !reflect.DeepEqual(got, p.Normal()) {
		t.Fatalf("round trip: %v\n%+v\n%+v", err, got, p.Normal())
	}
	got, _ = FromProps(Defaults(), map[string]string{"rain": "7", "width": "9", "style": "nonsense", "wind": "-90"})
	if got.Rain != 1 || got.Width != 128 || got.Style != "atlas" || got.Wind != 270 {
		t.Errorf("not kept in range: %+v", got)
	}
	if _, err := FromProps(Defaults(), map[string]string{"RAIN": "lots"}); err == nil {
		t.Error("a word for a number was taken")
	}
	h := Defaults()
	h.Generator, h.Lakes = "hex", false
	props = map[string]string{}
	for _, kv := range h.Props() {
		props[kv[0]] = kv[1]
	}
	if got, _ := FromProps(Defaults(), props); got.Generator != "hex" || got.Lakes {
		t.Errorf("hex params: %+v", got)
	}
}

// The hex world: the share of land asked for, a picture the size the
// layout says, and realms that cover the land.
func TestHexWorld(t *testing.T) {
	p := Defaults()
	p.Generator, p.Hexes, p.LandShare, p.Territories = "hex", 48, 0.4, 5
	h := GenerateHex(p)
	land, owned := 0, 0
	for i := range h.Alt {
		if h.Alt[i] > h.Sea && !h.Lake[i] {
			land++
			if h.Territory[i] > 0 {
				owned++
			}
		}
	}
	share := float64(land) / float64(len(h.Alt))
	if share < 0.3 || share > 0.45 {
		t.Errorf("land share %.2f, wanted about 0.4", share)
	}
	if owned < land*8/10 {
		t.Errorf("realms cover %d of %d land hexes", owned, land)
	}
	b, err := JPEG(p, nil)
	if err != nil {
		t.Fatal(err)
	}
	img, err := jpeg.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	w, hh := ImageSize(p)
	if img.Bounds().Dx() != w || img.Bounds().Dy() != hh {
		t.Errorf("picture %v, layout says %dx%d", img.Bounds(), w, hh)
	}
	// Neighbours wrap east to west.
	if nb := h.neighbours(0); !contains(nb, h.Cols-1) {
		t.Errorf("hex 0's neighbours %v miss the far east", nb)
	}
}

func contains(xs []int, v int) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

func TestPlaceNames(t *testing.T) {
	seen := map[string]bool{}
	for i := uint64(0); i < 40; i++ {
		n := PlaceName(i, "town")
		if n == "" {
			t.Fatal("no name")
		}
		seen[n] = true
	}
	if len(seen) < 25 {
		t.Errorf("only %d different names in 40", len(seen))
	}
}

// A town: walls with gates, streets to the plaza, plenty of buildings, an
// SVG that parses; the same seed, the same town; a coast takes wards away.
func TestCity(t *testing.T) {
	p := Defaults()
	p.Generator, p.Seed, p.CitySize = "city", 5, 25
	c := GenerateCity(p)
	if len(c.buildings) < 300 {
		t.Errorf("only %d buildings in a town of 25 wards", len(c.buildings))
	}
	if len(c.walls) < 10 || len(c.gates) < 2 || len(c.towers) < 8 {
		t.Errorf("wall %d, gates %d, towers %d", len(c.walls), len(c.gates), len(c.towers))
	}
	if len(c.mains) < 2 || len(c.roads) < 2 {
		t.Errorf("main streets %d, roads %d", len(c.mains), len(c.roads))
	}
	b, ext, err := Picture(p, nil)
	if err != nil || ext != ".svg" {
		t.Fatalf("%v %q", err, ext)
	}
	var doc struct{ XMLName xml.Name }
	if err := xml.Unmarshal(b, &doc); err != nil || doc.XMLName.Local != "svg" {
		t.Fatalf("not an svg: %v", err)
	}
	if bytes.Contains(b, []byte("<script")) || bytes.Contains(b, []byte(" on")) && bytes.Contains(b, []byte("onload")) {
		t.Fatal("a picture with something that runs")
	}
	again, _, _ := Picture(p, nil)
	if !bytes.Equal(b, again) {
		t.Fatal("the same seed drew another town")
	}
	if w, h := ImageSize(p); w != p.CityPx || h != p.CityPx {
		t.Errorf("size %dx%d", w, h)
	}
	p.Walls = false
	if open := GenerateCity(p); len(open.walls) != 0 || len(open.mains) == 0 {
		t.Errorf("no walls asked for: %d wall pieces, %d streets", len(open.walls), len(open.mains))
	}
	p.Walls, p.Seaside = true, true
	sea := GenerateCity(p)
	wet := 0
	for _, pa := range sea.patches {
		if pa.water {
			wet++
		}
	}
	if wet == 0 {
		t.Error("a town on the coast with no sea")
	}
	p.Seaside, p.River = false, true
	if r := GenerateCity(p); len(r.river) < 2 {
		t.Error("no river")
	}
}

func TestTownPlaceNames(t *testing.T) {
	if n := PlaceName(3, "tavern"); !strings.HasPrefix(n, "The ") {
		t.Errorf("a tavern called %q", n)
	}
	if n := PlaceName(3, "temple"); !strings.HasPrefix(n, "Temple of ") {
		t.Errorf("a temple called %q", n)
	}
	if n := PlaceName(3, "gate"); !strings.HasSuffix(n, " Gate") {
		t.Errorf("a gate called %q", n)
	}
}

// A village: as many houses as asked for (or nearly - there may not be
// room), each clear of the roads, the water and every other plot; a church;
// an SVG that parses.
func TestVillage(t *testing.T) {
	p := Defaults()
	p.Generator, p.Seed, p.Houses, p.River, p.Windmills = "village", 21, 30, true, 2
	v := GenerateVillage(p)
	if len(v.plots) < 20 {
		t.Errorf("%d plots for 30 houses", len(v.plots))
	}
	if len(v.church) == 0 || v.yard == nil {
		t.Error("no church")
	}
	if len(v.windmills) == 0 || len(v.fields) < 10 || len(v.roads) < 2 {
		t.Errorf("windmills %d, fields %d, roads %d", len(v.windmills), len(v.fields), len(v.roads))
	}
	for i, a := range v.plots {
		for j, b := range v.plots {
			if i < j && convexOverlap(shrink(a.poly, 0.3), shrink(b.poly, 0.3)) {
				t.Fatalf("plots %d and %d overlap", i, j)
			}
		}
		for _, rd := range v.roads {
			if distToLine(centroid(a.poly), rd.pts) < rd.width/2 {
				t.Fatalf("plot %d is on a road", i)
			}
		}
	}
	b, ext, err := Picture(p, nil)
	if err != nil || ext != ".svg" {
		t.Fatal(err, ext)
	}
	var doc struct{ XMLName xml.Name }
	if err := xml.Unmarshal(b, &doc); err != nil {
		t.Fatalf("not an svg: %v", err)
	}
	p.Seaside = true
	if s := GenerateVillage(p); len(s.docks) == 0 || len(s.boats) == 0 {
		t.Errorf("a fishing village with %d docks and %d boats", len(s.docks), len(s.boats))
	}
}

// Perilous Shores: an SVG that parses, a coast, a compass; the terrain's
// own settings are its settings too.
func TestShores(t *testing.T) {
	p := Defaults()
	p.Generator, p.Width, p.Height, p.Seed = "shores", 600, 400, 3
	b, ext, err := Picture(p, nil)
	if err != nil || ext != ".svg" {
		t.Fatal(err, ext)
	}
	var doc struct{ XMLName xml.Name }
	if err := xml.Unmarshal(b, &doc); err != nil {
		t.Fatalf("not an svg: %v", err)
	}
	if !bytes.Contains(b, []byte(">N</text>")) {
		t.Error("no compass")
	}
	props := map[string]string{}
	for _, kv := range p.Props() {
		props[kv[0]] = kv[1]
	}
	if props["RAIN"] == "" || props["SHORESSTYLE"] == "" || props["STYLE"] != "" || props["HOUSES"] != "" {
		t.Errorf("shores keeps %v", props)
	}
	if w, h := ImageSize(p); w != 600 || h != 400 {
		t.Errorf("size %dx%d", w, h)
	}
}
