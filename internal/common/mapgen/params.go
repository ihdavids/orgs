// Package mapgen makes maps of a region - coasts, mountains, rain, rivers,
// forests and deserts - as pictures, in the manner of Red Blob Games'
// mapgen4 (https://www.redblobgames.com/maps/mapgen4/): elevation from noise
// shaped into land and sea, rain carried on the wind and wrung out by
// mountains (so their lee is dry), rivers gathered downhill from the rain,
// and colour from height and wetness, lit from one side.
//
// The whole of a map is its Params and the terrain Strokes painted on it, so
// a map is kept as those (orgs keeps them in the map's org file) and drawn
// again whenever it is wanted. The code is our own; only the idea is
// mapgen4's.
package mapgen

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// The generators: "terrain" is a region in the manner of mapgen4 (Generate
// and Render); "hex" is a whole world on a hexagon grid, in the manner of
// hexgen (https://github.com/eranimo/hexgen; see hexworld.go); "city" is a
// town in the manner of watabou's Medieval Fantasy City Generator (city.go). Each has its
// own params in Params, told apart in Fields by Gen.
var Generators = []string{"terrain", "shores", "hex", "city", "village"}

type Params struct {
	Generator string `json:"generator"`
	Seed      uint64 `json:"seed"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	// Shape: how much is land, how much the edges fall away into sea (0 a
	// continent running off the map, 1 an island), how ragged the coasts are.
	Land   float64 `json:"land"`
	Island float64 `json:"island"`
	Coast  float64 `json:"coast"`
	// Mountains: how high, how sharp their ridges; how deep the sea.
	Mountains float64 `json:"mountains"`
	Jagged    float64 `json:"jagged"`
	Ocean     float64 `json:"ocean"`
	// Climate: the way the wind blows (degrees, 0 = toward the east, 90 =
	// toward the south), how wet it is, how much mountains wring the rain
	// out, how much the sea gives back.
	Wind       float64 `json:"wind"`
	Rain       float64 `json:"rain"`
	Shadow     float64 `json:"shadow"`
	Evaporate  float64 `json:"evaporate"`
	Rivers     float64 `json:"rivers"`
	RiverWidth float64 `json:"riverWidth"`
	// The look: where the light comes from (degrees, as Wind), how strong the
	// relief, how dark the coastline, and a palette.
	Light   float64 `json:"light"`
	Relief  float64 `json:"relief"`
	Outline float64 `json:"outline"`
	Style   string  `json:"style"`

	// The hex world: how many hexes across, how wide a hex is drawn (px),
	// the world's average temperature (°C) and axial tilt (degrees), how
	// rough the land, what share of it is land (0..1), and how many rivers,
	// craters, volcanoes and territories. Lakes form where rivers meet.
	Hexes       int     `json:"hexes"`
	HexPx       float64 `json:"hexPx"`
	AvgTemp     float64 `json:"avgTemp"`
	Tilt        float64 `json:"tilt"`
	Roughness   float64 `json:"roughness"`
	LandShare   float64 `json:"landShare"`
	RiverCount  int     `json:"riverCount"`
	Lakes       bool    `json:"lakes"`
	Craters     int     `json:"craters"`
	Volcanoes   int     `json:"volcanoes"`
	Territories int     `json:"territories"`
	HexStyle    string  `json:"hexStyle"`

	// The town: how many wards, and whether it has walls, a citadel, a
	// market plaza, a temple, a river, a coast and farmland round it; the
	// picture's side in pixels, and its look.
	CitySize  int    `json:"citySize"`
	Walls     bool   `json:"walls"`
	Citadel   bool   `json:"citadel"`
	Plaza     bool   `json:"plaza"`
	Temple    bool   `json:"temple"`
	River     bool   `json:"river"`
	Seaside   bool   `json:"seaside"`
	Farms     bool   `json:"farms"`
	CityPx    int    `json:"cityPx"`
	CityStyle string `json:"cityStyle"`

	// The village (village.go): how many houses, a church, how many
	// windmills, a pond; and the town's river, coast, farmland, size and look.
	Houses    int  `json:"houses"`
	Church    bool `json:"church"`
	Windmills int  `json:"windmills"`
	Pond      bool `json:"pond"`

	// Perilous Shores (shores.go): the terrain's params, inked in a style.
	ShoresStyle string `json:"shoresStyle"`
}

var CityStyles = []string{"light", "parchment", "ink", "dark"}

// The hex world's looks: true colour from orbit, biomes, or territories.
var HexStyles = []string{"satellite", "biomes", "political"}

// Stroke is terrain painted by hand, in fractions of the map's width and
// height (the radius in fractions of its width), so it stays put when the
// map is drawn at another size.
type Stroke struct {
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	R    float64 `json:"r"`
	Kind string  `json:"kind"` // mountain, hill, valley, land, shallow, ocean
}

var StrokeKinds = []string{"mountain", "hill", "valley", "land", "shallow", "ocean"}

// The looks: smooth colour by height and wetness lit from one side (atlas,
// as mapgen4), polygons coloured by biome with ragged edges (biomes, as
// mapgen2 - see biome in render.go), an aged sheet, and pen and wash.
var Styles = []string{"atlas", "biomes", "parchment", "ink"}

func Defaults() Params {
	return Params{ShoresStyle: "sepia", Houses: 32, Church: true, Windmills: 1, Pond: true,
		CitySize: 30, Walls: true, Citadel: true, Plaza: true, Temple: true, Farms: true, CityPx: 1600, CityStyle: "light",
		Generator: "terrain", Hexes: 96, HexPx: 16, AvgTemp: 14, Tilt: 23, Roughness: 0.5, LandShare: 0.35,
		RiverCount: 30, Lakes: true, Craters: 0, Volcanoes: 4, Territories: 12, HexStyle: "satellite",
		Seed: 1, Width: 1536, Height: 1024, Land: 0.5, Island: 0.6, Coast: 0.5,
		Mountains: 0.55, Jagged: 0.5, Ocean: 0.5, Wind: 200, Rain: 0.5, Shadow: 0.5, Evaporate: 0.5,
		Rivers: 0.45, RiverWidth: 0.5, Light: 225, Relief: 0.55, Outline: 0.5, Style: "atlas"}
}

// Normal is p with everything inside its range: a map can be asked for with
// any numbers (they come from an org file a person edits), and the worst it
// gets is the nearest one that makes sense.
func (p Params) Normal() Params {
	d := Defaults()
	if p.Seed == 0 {
		p.Seed = d.Seed
	}
	if p.Width <= 0 {
		p.Width = d.Width
	}
	if p.Height <= 0 {
		p.Height = d.Height
	}
	p.Width = int(clamp(float64(p.Width), 128, 4096))
	p.Height = int(clamp(float64(p.Height), 128, 4096))
	for _, f := range []*float64{&p.Land, &p.Island, &p.Coast, &p.Mountains, &p.Jagged, &p.Ocean, &p.Rain,
		&p.Shadow, &p.Evaporate, &p.Rivers, &p.RiverWidth, &p.Relief, &p.Outline} {
		if math.IsNaN(*f) {
			*f = 0.5
		}
		*f = clamp(*f, 0, 1)
	}
	p.Wind = math.Mod(math.Mod(p.Wind, 360)+360, 360)
	okg := false
	for _, g := range Generators {
		okg = okg || g == p.Generator
	}
	if !okg {
		p.Generator = "terrain"
	}
	if p.CitySize <= 0 {
		p.CitySize = d.CitySize
	}
	p.CitySize = int(clamp(float64(p.CitySize), 6, 120))
	if p.Houses <= 0 {
		p.Houses = d.Houses
	}
	p.Houses = int(clamp(float64(p.Houses), 4, 160))
	p.Windmills = int(clamp(float64(p.Windmills), 0, 6))
	if p.CityPx <= 0 {
		p.CityPx = d.CityPx
	}
	p.CityPx = int(clamp(float64(p.CityPx), 400, 4096))
	oks := false
	for _, s := range ShoresStyles {
		oks = oks || s == p.ShoresStyle
	}
	if !oks {
		p.ShoresStyle = d.ShoresStyle
	}
	okc := false
	for _, s := range CityStyles {
		okc = okc || s == p.CityStyle
	}
	if !okc {
		p.CityStyle = d.CityStyle
	}
	if p.Hexes <= 0 {
		p.Hexes = d.Hexes
	}
	p.Hexes = int(clamp(float64(p.Hexes), 16, 400))
	if p.HexPx <= 0 {
		p.HexPx = d.HexPx
	}
	p.HexPx = clamp(p.HexPx, 4, 64)
	p.AvgTemp = clamp(p.AvgTemp, -40, 50)
	p.Tilt = clamp(p.Tilt, -90, 90)
	p.Roughness = clamp(p.Roughness, 0, 1)
	p.LandShare = clamp(p.LandShare, 0, 1)
	p.RiverCount = int(clamp(float64(p.RiverCount), 0, 500))
	p.Craters = int(clamp(float64(p.Craters), 0, 200))
	p.Volcanoes = int(clamp(float64(p.Volcanoes), 0, 200))
	p.Territories = int(clamp(float64(p.Territories), 0, 100))
	okh := false
	for _, s := range HexStyles {
		okh = okh || s == p.HexStyle
	}
	if !okh {
		p.HexStyle = d.HexStyle
	}
	p.Light = math.Mod(math.Mod(p.Light, 360)+360, 360)
	ok := false
	for _, s := range Styles {
		ok = ok || s == p.Style
	}
	if !ok {
		p.Style = d.Style
	}
	return p
}

// Hash names a map's picture: the same params and strokes, the same hash.
func Hash(p Params, strokes []Stroke) string {
	b, _ := json.Marshal(struct {
		P Params
		S []Stroke
	}{p.Normal(), strokes})
	h := sha1.Sum(b)
	return hex.EncodeToString(h[:8])
}

// Field describes a parameter for a form, and for the org file it is kept in
// (Prop is its property name).
type Field struct {
	Id      string   `json:"id"`
	Prop    string   `json:"prop"`
	Label   string   `json:"label"`
	Group   string   `json:"group"`
	Kind    string   `json:"kind"` // range, number, angle, select
	Min     float64  `json:"min"`
	Max     float64  `json:"max"`
	Step    float64  `json:"step"`
	Choices []string `json:"choices,omitempty"`
	Hint    string   `json:"hint,omitempty"`
	// Which generators it belongs to: "terrain", "hex", "city", "village",
	// several with commas, or "" for all.
	Gen string `json:"gen,omitempty"`
}

func Fields() []Field {
	r := func(id, label, group, hint string) Field {
		g := "terrain,shores"
		if group == "Look" {
			g = "terrain"
		}
		return Field{Id: id, Prop: strings.ToUpper(id), Label: label, Group: group, Kind: "range", Min: 0, Max: 1, Step: 0.01, Hint: hint, Gen: g}
	}
	c := func(f Field) Field {
		f.Gen = "city"
		if f.Prop == "" {
			f.Prop = strings.ToUpper(f.Id)
		}
		return f
	}
	cv := func(f Field) Field {
		f = c(f)
		f.Gen = "city,village"
		return f
	}
	vl := func(f Field) Field {
		f = c(f)
		f.Gen = "village"
		return f
	}
	yesNo := func(id, label, group string) Field {
		return Field{Id: id, Label: label, Group: group, Kind: "select", Choices: []string{"true", "false"}}
	}
	h := func(f Field) Field {
		f.Gen = "hex"
		if f.Prop == "" {
			f.Prop = strings.ToUpper(f.Id)
		}
		return f
	}
	return []Field{
		{Id: "generator", Prop: "GENERATOR", Label: "Generator", Group: "Map", Kind: "select", Choices: Generators,
			Hint: "terrain: a region, mapgen4-style. shores: a region inked, Perilous Shores-style. hex: a whole world on hexes, hexgen-style. city: a walled town. village: a village and its fields"},
		{Id: "seed", Prop: "SEED", Label: "Seed", Group: "Map", Kind: "number", Min: 1, Max: 1e15, Step: 1},
		{Id: "width", Prop: "WIDTH", Label: "Width (px)", Group: "Map", Kind: "number", Min: 128, Max: 4096, Step: 64, Gen: "terrain,shores"},
		{Id: "height", Prop: "HEIGHT", Label: "Height (px)", Group: "Map", Kind: "number", Min: 128, Max: 4096, Step: 64, Gen: "terrain,shores"},
		h(Field{Id: "hexes", Label: "Hexes across", Group: "Map", Kind: "number", Min: 16, Max: 400, Step: 4}),
		h(Field{Id: "hexPx", Label: "Hex width (px)", Group: "Map", Kind: "number", Min: 4, Max: 64, Step: 1}),
		h(Field{Id: "landShare", Label: "Land", Group: "World", Kind: "range", Min: 0, Max: 1, Step: 0.01, Hint: "The share of the world that is land"}),
		h(Field{Id: "roughness", Label: "Roughness", Group: "World", Kind: "range", Min: 0, Max: 1, Step: 0.01, Hint: "Smooth continents, or broken land"}),
		h(Field{Id: "avgTemp", Label: "Average temperature (°C)", Group: "World", Kind: "number", Min: -40, Max: 50, Step: 1}),
		h(Field{Id: "tilt", Label: "Axial tilt (°)", Group: "World", Kind: "number", Min: -90, Max: 90, Step: 1, Hint: "More tilt, harder seasons: colder poles, wider deserts"}),
		h(Field{Id: "riverCount", Label: "Rivers", Group: "Features", Kind: "number", Min: 0, Max: 500, Step: 1}),
		h(Field{Id: "lakes", Label: "Lakes where rivers meet", Group: "Features", Kind: "select", Choices: []string{"true", "false"}}),
		h(Field{Id: "volcanoes", Label: "Volcanoes", Group: "Features", Kind: "number", Min: 0, Max: 200, Step: 1}),
		h(Field{Id: "craters", Label: "Craters", Group: "Features", Kind: "number", Min: 0, Max: 200, Step: 1}),
		h(Field{Id: "territories", Label: "Territories", Group: "Features", Kind: "number", Min: 0, Max: 100, Step: 1, Hint: "Realms grown across the land; 0 for none"}),
		h(Field{Id: "hexStyle", Label: "Style", Group: "Look", Kind: "select", Choices: HexStyles}),
		c(Field{Id: "citySize", Label: "Wards", Group: "Town", Kind: "number", Min: 6, Max: 120, Step: 1, Hint: "A village is 8 or so, a town 25, a city 60"}),
		cv(Field{Id: "cityPx", Label: "Size (px)", Group: "Look", Kind: "number", Min: 400, Max: 4096, Step: 100}),
		vl(Field{Id: "houses", Label: "Houses", Group: "Village", Kind: "number", Min: 4, Max: 160, Step: 1, Hint: "A hamlet is 8, a village 30, a large village 80"}),
		vl(yesNo("church", "Church", "Village")),
		vl(Field{Id: "windmills", Label: "Windmills", Group: "Village", Kind: "number", Min: 0, Max: 6, Step: 1}),
		vl(yesNo("pond", "Pond", "Village")),
		c(yesNo("walls", "Walls", "Town")),
		c(yesNo("citadel", "Citadel", "Town")),
		c(yesNo("plaza", "Market plaza", "Town")),
		c(yesNo("temple", "Temple", "Town")),
		cv(yesNo("river", "River", "Land")),
		cv(yesNo("seaside", "Coast", "Land")),
		cv(yesNo("farms", "Farmland", "Land")),
		cv(Field{Id: "cityStyle", Label: "Style", Group: "Look", Kind: "select", Choices: CityStyles}),
		r("land", "Land", "Shape", "More land, or more sea"),
		r("island", "Island", "Shape", "0 runs off the map's edges, 1 is an island"),
		r("coast", "Ragged coast", "Shape", "How broken up the coastline is"),
		r("mountains", "Mountains", "Shape", "How high"),
		r("jagged", "Jagged ridges", "Shape", "Sharp ridge lines, or rounded hills"),
		r("ocean", "Ocean depth", "Shape", ""),
		{Id: "wind", Prop: "WIND", Label: "Wind toward", Group: "Climate", Kind: "angle", Min: 0, Max: 359, Step: 1, Hint: "The way the wind blows: its rain falls on the windward side of mountains", Gen: "terrain,shores"},
		r("rain", "Raininess", "Climate", ""),
		r("shadow", "Rain shadow", "Climate", "How much mountains dry the land beyond them"),
		r("evaporate", "Evaporation", "Climate", "How much the sea gives back to the wind"),
		r("rivers", "Rivers", "Rivers", "Fewer, bigger rivers or more, smaller ones"),
		r("riverWidth", "River width", "Rivers", ""),
		{Id: "light", Prop: "LIGHT", Label: "Light from", Group: "Look", Kind: "angle", Min: 0, Max: 359, Step: 1, Gen: "terrain"},
		r("relief", "Relief", "Look", "How strongly the hills are shaded"),
		r("outline", "Coastline", "Look", "How dark the coast is drawn"),
		{Id: "style", Prop: "STYLE", Label: "Style", Group: "Look", Kind: "select", Choices: Styles, Gen: "terrain"},
		{Id: "shoresStyle", Prop: "SHORESSTYLE", Label: "Style", Group: "Look", Kind: "select", Choices: ShoresStyles, Gen: "shores"},
	}
}

// For says whether a field belongs to a generator: Gen is empty (every
// generator), one name, or a list of them with commas.
func (f Field) For(gen string) bool {
	if f.Gen == "" {
		return true
	}
	for _, g := range strings.Split(f.Gen, ",") {
		if g == gen {
			return true
		}
	}
	return false
}

// Props is p as org properties, in the order of Fields.
func (p Params) Props() [][2]string {
	p = p.Normal()
	m := map[string]interface{}{}
	b, _ := json.Marshal(p)
	json.Unmarshal(b, &m)
	out := [][2]string{}
	for _, f := range Fields() {
		v := m[f.Id]
		s := ""
		switch x := v.(type) {
		case float64:
			s = strconv.FormatFloat(x, 'f', -1, 64)
		case string:
			s = x
		case bool:
			s = strconv.FormatBool(x)
		}
		// Only the generator's own params (and those all use) are kept.
		if !f.For(p.Generator) {
			continue
		}
		out = append(out, [2]string{f.Prop, s})
	}
	return out
}

// FromProps reads params from org properties (by Field.Prop, any case) on
// top of base: what the file does not say keeps base's value.
func FromProps(base Params, props map[string]string) (Params, error) {
	m := map[string]interface{}{}
	b, _ := json.Marshal(base)
	json.Unmarshal(b, &m)
	upper := map[string]string{}
	for k, v := range props {
		upper[strings.ToUpper(k)] = strings.TrimSpace(v)
	}
	var bad []string
	for _, f := range Fields() {
		v, ok := upper[f.Prop]
		if !ok || v == "" {
			continue
		}
		if f.Kind == "select" {
			if _, isBool := m[f.Id].(bool); isBool {
				m[f.Id] = strings.EqualFold(v, "true") || v == "t" || strings.EqualFold(v, "yes")
			} else {
				m[f.Id] = strings.ToLower(v)
			}
			continue
		}
		n, err := strconv.ParseFloat(v, 64)
		if err != nil {
			bad = append(bad, f.Prop)
			continue
		}
		m[f.Id] = n
	}
	b, _ = json.Marshal(m)
	var p Params
	if err := json.Unmarshal(b, &p); err != nil {
		return base, err
	}
	if len(bad) > 0 {
		sort.Strings(bad)
		return p.Normal(), fmt.Errorf("not numbers: %s", strings.Join(bad, ", "))
	}
	return p.Normal(), nil
}
