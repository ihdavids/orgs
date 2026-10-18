package randgen

import (
	"fmt"
	"path/filepath"
	"math"
	"math/rand/v2"
	"sort"
	"strings"
	"time"

	"github.com/ihdavids/orgs/internal/common/dnd"
)

// A random dungeon: a map, a key, and a story to hang them on.
//
// Two inspirations. donjon's dungeon generator for the settings (size,
// layout, rooms, corridors, doors, the dungeon level and party it is stocked
// for); watabou's one page dungeon for what a dungeon is *for* at the table -
// a title, a few lines of backstory, and one short evocative note per room
// before the details. Every table here is our own.
//
// The Dungeon is the editable form. worg draws it and edits it (the map is
// drawn in the browser, never here), so everything in it is plain data: the
// grid as strings, rooms per cell, doors, stairs, props and labels as lists.
// What is derived from the geometry - each room's exits - is worked out here
// only (Exits), so the editor asks the server again after a change rather than
// keeping a second copy of the rule.

const DungeonVersion = 1

type DDoor struct {
	R    int    `json:"r"`
	C    int    `json:"c"`
	Kind string `json:"kind"` // arch door locked trapped secret portcullis
}

type DStair struct {
	R   int    `json:"r"`
	C   int    `json:"c"`
	Dir string `json:"dir"` // up down
	// Where the stairs lead: another map's org file, and the square the
	// party arrives on there.
	Link *StairLink `json:"link,omitempty"`
}

// StairLink is a way from one map to another. File is relative to the org
// file of the map the stairs are on (so a folder of maps can move); Abs is
// that resolved, filled in when a map is read for a screen.
type StairLink struct {
	File  string `json:"file"`
	Abs   string `json:"abs,omitempty"`
	R     int    `json:"r"`
	C     int    `json:"c"`
	Label string `json:"label,omitempty"`
}

type DProp struct {
	R    int    `json:"r"`
	C    int    `json:"c"`
	Kind string `json:"kind"`
}

// DLabel is free text on the map, placed in cell units.
type DLabel struct {
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	Text string  `json:"text"`
}

type DRoom struct {
	Id int `json:"id"`
	// Name is what the room is ("Guard room"); Note is the one line a
	// one-page dungeon prints beside its number; Text is the rest, org body
	// text the editor lets you change.
	Name string `json:"name"`
	Note string `json:"note"`
	Text string `json:"text"`
}

type Dungeon struct {
	Version int               `json:"version"`
	Name    string            `json:"name"`
	Story   string            `json:"story"`
	Seed    uint64            `json:"seed"`
	Options map[string]string `json:"options"`
	Rows    int               `json:"rows"`
	Cols    int               `json:"cols"`
	// Grid is one string per row: '#' rock, '.' floor, '~' water.
	Grid []string `json:"grid"`
	// Room is the room id of every cell, row by row; 0 is a corridor.
	Room      []int    `json:"room"`
	Doors     []DDoor  `json:"doors"`
	Stairs    []DStair `json:"stairs"`
	Props     []DProp  `json:"props"`
	Labels    []DLabel `json:"labels"`
	Rooms     []DRoom  `json:"rooms"`
	Wandering string   `json:"wandering"`
	// Lights are what holds a fire or a glow (lights.go).
	Lights []DLight `json:"lights,omitempty"`
	// Style is how the editor last drew it (worg's MapStyle: theme, view,
	// angle...), kept so the map viewer draws it the same way. The server
	// never reads it.
	Style map[string]interface{} `json:"style,omitempty"`
	// Backdrop is a picture drawn instead of the map: a Dungeon Alchemist
	// (Universal VTT) export or any image, laid over the grid.
	Backdrop *Backdrop `json:"backdrop,omitempty"`
	// World is a generated map's makings (world.go), when the dungeon is one.
	World *World `json:"world,omitempty"`
}

// Backdrop is the picture a dungeon is shown as. Two ways to have one: a
// generated dungeon remade in another tool, whose picture replaces the
// drawing (its grid is still the real plan: exits, fog and sight come from
// it), or a dungeon made on a picture of somebody's own (Own), where the grid
// only says which squares belong to which room.
type Backdrop struct {
	// Src is the picture beside the org file ("maps/name-map.jpg"). While
	// being edited it is a blob: url the server never sees saved.
	Src    string  `json:"src"`
	Width  int     `json:"width"`
	Height int     `json:"height"`
	// Ppc is the picture's pixels per grid square; X and Y the picture
	// pixel where the grid's first square starts.
	Ppc  float64 `json:"ppc"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	Own  bool    `json:"own"`
	Kind string  `json:"kind,omitempty"` // "uvtt" or "image"
	Name string  `json:"name,omitempty"` // the file it came from
	// From a Universal VTT export: line of sight walls and doors, as
	// polylines [x0, y0, x1, y1, ...] in the picture's squares.
	Walls [][]float64 `json:"walls,omitempty"`
	// Doors are [x0, y0, x1, y1, shut]; lights [x, y, range, intensity, r, g, b].
	Doors  [][]float64 `json:"doors,omitempty"`
	Lights [][]float64 `json:"lights,omitempty"`
}

type DungeonRequest struct {
	Name    string            `json:"name"`
	Seed    uint64            `json:"seed"`
	Options map[string]string `json:"options"`
}

// The props a room can hold, in the order the editor offers them.
var PropKinds = []string{"column", "statue", "altar", "chest", "barrel", "table", "bed", "well",
	"fountain", "brazier", "sarcophagus", "rubble", "trapdoor", "trap", "bones", "throne",
	"stalagmite", "mushrooms", "crystals"}

// DungeonFields is the settings form.
func DungeonFields() []Field {
	return []Field{
		{Id: "kind", Label: "Kind of place", Kind: "select", Default: "dungeon", Choices: choices("dungeon=A dungeon", "cave=A cave", "dwelling=A dwelling")},
		{Id: "level", Label: "Dungeon level", Kind: "select", Choices: levelChoices(), Default: "1"},
		{Id: "party", Label: "Party size", Kind: "number", Default: "4", Min: 1, Max: 12},
		{Id: "motif", Label: "Motif", Kind: "select", Default: "none", Choices: choices(
			"none", "abandoned", "aberrant", "giant", "undead", "vermin", "aquatic", "desert",
			"underdark", "arcane", "fire", "cold", "abyssal", "infernal")},
		{Id: "size", Label: "Size", Kind: "select", Default: "medium", Choices: choices(
			"fine", "diminutive", "tiny", "small", "medium", "large", "huge", "gargantuan", "colossal")},
		{Id: "layout", Label: "Layout", Kind: "select", Default: "rectangle", Choices: choices(
			"square", "rectangle", "box", "cross", "dagger", "saltire", "keep", "hexagon", "round", "cavernous")},
		{Id: "roomLayout", Label: "Room layout", Kind: "select", Default: "scattered", Choices: choices(
			"sparse", "scattered", "dense", "symmetric")},
		{Id: "roomSize", Label: "Room size", Kind: "select", Default: "medium", Choices: choices(
			"small", "medium", "large", "huge", "gargantuan", "colossal")},
		{Id: "polymorph", Label: "Room shapes", Kind: "select", Default: "yes", Choices: choices("no=Rectangles only", "yes=Some shaped", "many=Mostly shaped")},
		{Id: "doors", Label: "Doors", Kind: "select", Default: "standard", Choices: choices(
			"none=No doors", "basic", "secure", "standard", "deathtrap")},
		{Id: "corridors", Label: "Corridors", Kind: "select", Default: "errant", Choices: choices(
			"labyrinth", "errant", "straight")},
		{Id: "deadends", Label: "Remove dead ends", Kind: "select", Default: "some", Choices: choices("none", "some", "all")},
		{Id: "stairs", Label: "Stairs", Kind: "select", Default: "yes", Choices: choices("no", "yes", "many")},
		{Id: "egress", Label: "Ways out at the edge", Kind: "select", Default: "yes", Choices: choices("no", "yes", "many", "tiling")},
	}
}

// GenerateDungeon builds a dungeon. The same seed and options give the same
// dungeon, which is what lets a save be checked against what was shown.
func GenerateDungeon(rs *dnd.Ruleset, req DungeonRequest) *Dungeon {
	seed := req.Seed
	if seed == 0 {
		seed = uint64(time.Now().UnixNano())
	}
	o := map[string]string{}
	for _, f := range DungeonFields() {
		o[f.Id] = f.Default
	}
	for k, v := range req.Options {
		if strings.TrimSpace(v) != "" {
			o[k] = v
		}
	}
	// A cave is the cavern layout, whatever the layout says.
	if o["kind"] == "cave" {
		o["layout"] = "cavernous"
	}
	c := &ctx{r: rand.New(rand.NewPCG(seed, seed^0x51a7e5d)), rs: rs, opts: o}
	var g *grid
	var boxes []roomBox
	if o["kind"] == "dwelling" {
		g, boxes = c.dwelling(o)
	} else {
		g, boxes = c.build(o)
	}

	d := &Dungeon{Version: DungeonVersion, Seed: seed, Options: o, Rows: g.rows, Cols: g.cols,
		Room: g.room, Doors: []DDoor{}, Stairs: []DStair{}, Props: []DProp{}, Labels: []DLabel{}}
	if o["kind"] == "dwelling" {
		d.Name, d.Story = c.dwellingStory()
	} else {
		d.Name, d.Story = c.story(o["motif"], len(boxes))
	}
	if strings.TrimSpace(req.Name) != "" {
		d.Name = strings.TrimSpace(req.Name)
	}
	for _, i := range sortedInts(g.door) {
		d.Doors = append(d.Doors, DDoor{R: i / g.cols, C: i % g.cols, Kind: g.door[i]})
	}
	for _, i := range sortedInts(g.stairs) {
		d.Stairs = append(d.Stairs, DStair{R: i / g.cols, C: i % g.cols, Dir: g.stairs[i]})
	}
	if o["kind"] == "dwelling" {
		c.dwellingStock(d, g, boxes)
	} else {
		c.stock(d, g, boxes)
	}
	d.Grid = make([]string, g.rows)
	for r := 0; r < g.rows; r++ {
		d.Grid[r] = string(g.cell[r*g.cols : (r+1)*g.cols])
	}
	c.lightRooms(d, o["kind"])
	if o["kind"] == "dwelling" {
		d.Wandering = c.dwellingHappens()
	} else {
		d.Wandering = c.wandering(o)
	}
	return d
}

func sortedInts[V any](m map[int]V) []int {
	out := make([]int, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}

// ---- monsters by motif -----------------------------------------------------

func motifKeep(motif string) func(*Monster) bool {
	has := func(m *Monster, words ...string) bool {
		s := strings.ToLower(m.Name + " " + m.Subtype)
		for _, w := range words {
			if strings.Contains(s, w) {
				return true
			}
		}
		return false
	}
	switch motif {
	case "undead":
		return func(m *Monster) bool { return m.Type == "undead" }
	case "aberrant":
		return func(m *Monster) bool { return m.Type == "aberration" || m.Type == "ooze" }
	case "giant":
		return func(m *Monster) bool { return m.Type == "giant" || has(m, "ogre", "troll", "ettin") }
	case "vermin":
		return func(m *Monster) bool {
			return m.Type == "swarm" || (m.Type == "beast" && has(m, "rat", "spider", "centipede", "beetle", "bat", "snake", "wasp", "scorpion", "toad", "frog", "lizard", "weasel", "crab", "stirge")) || has(m, "stirge")
		}
	case "aquatic":
		return func(m *Monster) bool { return m.livesIn("underwater") || m.livesIn("coastal") || m.livesIn("swamp") }
	case "desert":
		return func(m *Monster) bool { return m.livesIn("desert") }
	case "underdark":
		return func(m *Monster) bool { return m.livesIn("underdark") }
	case "arcane":
		return func(m *Monster) bool {
			return m.Type == "construct" || m.Type == "elemental" || has(m, "mage", "golem", "homunculus", "mephit", "naga")
		}
	case "fire":
		return func(m *Monster) bool {
			return has(m, "fire", "red dragon", "magma", "salamander", "hell hound", "efreeti", "azer", "magmin", "brass")
		}
	case "cold":
		return func(m *Monster) bool {
			return has(m, "ice", "frost", "winter", "white dragon", "polar", "remorhaz", "silver dragon") || m.livesIn("arctic")
		}
	case "abyssal":
		return func(m *Monster) bool { return has(m, "demon") || (m.Type == "fiend" && strings.Contains(m.Alignment, "chaotic")) }
	case "infernal":
		return func(m *Monster) bool { return has(m, "devil") || (m.Type == "fiend" && strings.Contains(m.Alignment, "lawful")) }
	}
	// No motif, or an abandoned place: anything that lives indoors or
	// underground, not the sea's or the sky's.
	return func(m *Monster) bool {
		return !(len(m.Environments) > 0 && (m.livesIn("underwater") && !m.livesIn("urban") && !m.livesIn("underdark")))
	}
}

func (c *ctx) dungeonEncounter(o map[string]string, diff string) ([]group, int, string) {
	party := optNum(o["party"], 4, 1, 12)
	level := optNum(o["level"], 1, 1, 20)
	keep := motifKeep(o["motif"])
	gs, raw, _, actual := c.encounter(party, level, diff, keep)
	if gs == nil {
		gs, raw, _, actual = c.encounter(party, level, diff, motifKeep("none"))
	}
	return gs, raw, actual
}

func optNum(s string, def, lo, hi int) int {
	n := def
	fmt.Sscanf(s, "%d", &n)
	return max(lo, min(hi, n))
}

func groupNames(gs []group) string {
	names := []string{}
	for _, g := range gs {
		if g.n > 1 {
			names = append(names, fmt.Sprintf("%d × %s", g.n, g.m.Name))
		} else {
			names = append(names, g.m.Name)
		}
	}
	return strings.Join(names, " and ")
}

func groupTable(gs []group) string {
	rows := [][]string{}
	for _, g := range gs {
		rows = append(rows, []string{fmt.Sprint(g.n), g.m.Name, g.m.CR, commas(g.m.XP * g.n), fmt.Sprint(g.m.AC), fmt.Sprint(g.m.HP)})
	}
	return orgTable([]string{"#", "Monster", "CR", "XP", "AC", "HP"}, rows)
}

// ---- stocking --------------------------------------------------------------

func (c *ctx) stock(d *Dungeon, g *grid, boxes []roomBox) {
	o := d.Options
	motif := o["motif"]
	cave := o["layout"] == "cavernous"
	waterPct := 8
	if o["kind"] == "cave" {
		waterPct = 30
	}
	if motif == "aquatic" {
		waterPct = 45
	}

	for _, b := range boxes {
		cells := []int{}
		for i, id := range g.room {
			if id == b.id {
				cells = append(cells, i)
			}
		}
		if len(cells) == 0 {
			continue
		}
		rc := c.roomContents(o, b.id, b.w, b.h, b.shape, cave, nil)
		name := rc.Name
		for _, k := range rc.props {
			c.propIn(d, g, cells, k)
		}
		// Furniture the purpose calls for, and columns down a big hall; a
		// cave's dripstone, fungus and crystals.
		for _, p := range caveProps[name] {
			if o["kind"] == "cave" && c.chance(80) {
				c.propIn(d, g, cells, p)
			}
		}
		if o["kind"] == "cave" && len(cells) > 12 {
			for k := 0; k < len(cells)/18; k++ {
				c.propIn(d, g, cells, c.pick([]string{"stalagmite", "stalagmite", "rubble", "mushrooms"}))
			}
		}
		for _, p := range purposeProps[name] {
			if c.chance(60) {
				c.propIn(d, g, cells, p)
			}
		}
		if !cave && b.shape == "" && b.w >= 7 && b.h >= 5 && c.chance(35) {
			for y := b.r + 1; y < b.r+b.h-1; y += 2 {
				d.Props = append(d.Props, DProp{R: y, C: b.c + 1, Kind: "column"}, DProp{R: y, C: b.c + b.w - 2, Kind: "column"})
			}
		}
		if c.chance(waterPct) {
			c.pool(g, cells)
		}
		if c.chance(15) {
			c.propIn(d, g, cells, "rubble")
		}

		d.Rooms = append(d.Rooms, rc.DRoom)
	}
}

// StockRoom is one room to fill in: its id, and its size in squares (0 for
// a room whose size means nothing, like one drawn on a picture).
type StockRoom struct {
	Id    int        `json:"id"`
	W     int        `json:"w"`
	H     int        `json:"h"`
	Shape string     `json:"shape"`
	Hints *RoomHints `json:"hints,omitempty"`
}

// RoomHints is what a map from elsewhere says about a room, for its key: its
// area in squares (when it is no rectangle), and its lights - how many, and
// their colour by name (warm, white, blue, green, red, violet).
type RoomHints struct {
	Area   int    `json:"area"`
	Lights int    `json:"lights"`
	Colour string `json:"colour"`
}

type StockRequest struct {
	Seed    uint64            `json:"seed"`
	Options map[string]string `json:"options"`
	Rooms   []StockRoom       `json:"rooms"`
	// Also a wandering monster table, and a name and backstory.
	Wandering bool `json:"wandering"`
	Story     bool `json:"story"`
}

type StockResult struct {
	Rooms     []DRoom `json:"rooms"`
	Wandering string  `json:"wandering,omitempty"`
	Name      string  `json:"name,omitempty"`
	Story     string  `json:"story,omitempty"`
}

// StockDungeon fills in rooms the way a generated dungeon's are filled, for
// a map that came from somewhere else: a picture with rooms marked on it.
func StockDungeon(rs *dnd.Ruleset, req StockRequest) StockResult {
	seed := req.Seed
	if seed == 0 {
		seed = uint64(time.Now().UnixNano())
	}
	o := map[string]string{}
	for _, f := range DungeonFields() {
		o[f.Id] = f.Default
	}
	for k, v := range req.Options {
		if strings.TrimSpace(v) != "" {
			o[k] = v
		}
	}
	c := &ctx{r: rand.New(rand.NewPCG(seed, seed^0x51a7e5d)), rs: rs, opts: o}
	out := StockResult{Rooms: []DRoom{}}
	cave := o["layout"] == "cavernous"
	for _, r := range req.Rooms {
		out.Rooms = append(out.Rooms, c.roomContents(o, r.Id, r.W, r.H, r.Shape, cave, r.Hints).DRoom)
	}
	if req.Wandering {
		out.Wandering = c.wandering(o)
	}
	if req.Story {
		out.Name, out.Story = c.story(o["motif"], len(req.Rooms))
	}
	return out
}

// The lights of a room, as the key says them, and a sentence of what that is
// like to stand in.
var lightSources = map[string][][2]string{
	"warm": {{"a torch in an iron sconce", "torches in iron sconces"}, {"an oil lamp on a chain", "oil lamps on chains"},
		{"a guttering candle", "guttering candles"}, {"a hooded lantern", "hooded lanterns"}, {"a brazier of glowing coals", "braziers of glowing coals"}},
	"white":  {{"a pale lantern", "pale lanterns"}, {"a lamp of cold white flame", "lamps of cold white flame"}},
	"blue":   {{"a witchlight burning blue", "witchlights burning blue"}, {"a blue-flamed lamp", "blue-flamed lamps"}},
	"green":  {{"a lamp guttering sickly green", "lamps guttering sickly green"}, {"a cluster of glowing green fungus", "clusters of glowing green fungus"}},
	"red":    {{"a brazier of smouldering red coals", "braziers of smouldering red coals"}},
	"violet": {{"a mote of violet arcane light", "motes of violet arcane light"}},
}

var lightMoods = map[string][]string{
	"warm":   {"Firelight flickers on the walls.", "The air is smoky and warm.", "Shadows jump at the edge of the light."},
	"white":  {"A cold, even light fills the room.", "The light is steady and pale, and casts no warmth."},
	"blue":   {"Everything is washed in a cold blue light.", "The blue light makes the shadows look deep as water."},
	"green":  {"A sickly green light makes every face look ill."},
	"red":    {"A dull red glow hangs over everything."},
	"violet": {"The light hums faintly, like a held note."},
}

var darkMoods = []string{"It is pitch dark.", "No light burns here.", "Darkness presses in from every side."}

var countWords = []string{"no", "one", "two", "three", "four", "five", "six", "seven", "eight", "nine", "ten", "eleven", "twelve"}

func (c *ctx) lightWords(n int, colour string) (string, string) {
	if n <= 0 {
		return "none - dark, unless the party brings light", c.pick(darkMoods)
	}
	srcs, ok := lightSources[colour]
	if !ok {
		srcs, colour = lightSources["warm"], "warm"
	}
	src := srcs[c.r.IntN(len(srcs))]
	mood := c.pick(lightMoods[colour])
	if n == 1 {
		return src[0], mood
	}
	count := fmt.Sprint(n)
	if n < len(countWords) {
		count = countWords[n]
	}
	return count + " " + src[1], mood
}

type roomStock struct {
	DRoom
	look  string
	props []string // what to put in it, for a map that draws them
}

// roomContents rolls what is in one room: what it was for, how it looks, and
// a monster, trap, treasure or feature (or nothing). Room 1 is the way in, and
// usually quiet. w and h are its size in squares, for the key; 0 leaves it out.
func (c *ctx) roomContents(o map[string]string, id, w, h int, shape string, cave bool, hints *RoomHints) roomStock {
	level := optNum(o["level"], 1, 1, 20)
	motif := o["motif"]
	band := levelBand(level)
	name := c.pick(roomPurposes)
	if cave {
		name = c.pick(cavePurposes)
	}
	if id == 1 {
		name = c.pick(entrancePurposes)
	}
	look := c.pick(roomDressing)
	if c.opts["kind"] == "cave" {
		look = c.pick(caveDressing)
	}
	if m, ok := motifDressing[motif]; ok && c.chance(50) {
		look = c.pick(m)
	}
	lines := []DLine{}
	props := []string{}
	if !cave && w > 0 && h > 0 {
		sh := map[string]string{"ellipse": ", round", "octagon": ", eight-sided", "cross": ", cross-shaped"}[shape]
		size := fmt.Sprintf("%d × %d ft%s", w*10, h*10, sh)
		// A room off a picture is whatever shape its walls make.
		if hints != nil && hints.Area > 0 && hints.Area*10 < w*h*8 {
			size = fmt.Sprintf("about %s sq ft, irregular (%d × %d ft across)", commas(hints.Area*25), w*10, h*10)
		}
		lines = append(lines, DLine{"Size", size})
	}
	lines = append(lines, DLine{"Here", look})
	mood := ""
	if hints != nil {
		var light string
		light, mood = c.lightWords(hints.Lights, hints.Colour)
		lines = append(lines, DLine{"Light", light})
	}
	extra := ""

	roll := c.r.IntN(100)
	note := capital(look) + "."
	switch {
	case id == 1 && roll < 60:
		// The way in is usually quiet: a first room that kills is a bad
		// first room.
	case roll < 38:
		// empty
	case roll < 66:
		gs, raw, diff := c.dungeonEncounter(o, "")
		if gs != nil {
			act := c.pick(encounterActivities)
			lines = append(lines, DLine{"Monsters", fmt.Sprintf("%s, %s (%s, %s XP)", groupNames(gs), act, diff, commas(raw))})
			note = fmt.Sprintf("%s %s, %s.", capital(look)+".", groupNames(gs), act)
			extra = groupTable(gs)
		}
		if c.chance(50) {
			cs, vals, items := c.hoard(band)
			lines = append(lines, DLine{"Treasure", c.hoardLine(cs, vals, items)})
			props = append(props, "chest")
		}
	case roll < 76:
		t := c.trap(level, "", "")
		lines = append(lines, DLine{"Trap", t.title + " — " + t.summary})
		for _, l := range t.lines {
			lines = append(lines, DLine{"  " + l.Label, l.Text})
		}
		note = capital(look) + ". " + c.pick(trapNotes)
		props = append(props, "trap")
		if c.chance(30) {
			lines = append(lines, DLine{"Treasure", c.purse(band).String()})
		}
	case roll < 86:
		cs, vals, items := c.hoard(band)
		lines = append(lines, DLine{"Treasure", c.hoardLine(cs, vals, items) + ", " + c.pick(hiddenPlaces)})
		note = capital(look) + ". " + c.pick(treasureNotes)
		props = append(props, "chest")
	default:
		f := roomFeatures[c.r.IntN(len(roomFeatures))]
		lines = append(lines, DLine{"Feature", f.text})
		note = capital(f.note) + "."
		props = append(props, f.prop)
	}

	if mood != "" {
		note += " " + mood
	}
	body := nestedFields(lines)
	if extra != "" {
	body += "\n" + extra
	}
	return roomStock{DRoom: DRoom{Id: id, Name: name, Note: note, Text: body}, look: look, props: props}
}

func (c *ctx) hoardLine(cs coins, vals []string, items []*dnd.Item) string {
	parts := []string{cs.String()}
	parts = append(parts, vals...)
	for _, it := range items {
		parts = append(parts, fmt.Sprintf("%s (%s)", it.Name, strings.ToLower(it.RarityName())))
	}
	return strings.Join(parts, "; ")
}

// propIn puts a prop on a free floor cell of the room, away from its edge
// when the room is big enough to have a middle.
func (c *ctx) propIn(d *Dungeon, g *grid, cells []int, kind string) {
	if kind == "" {
		return
	}
	taken := map[int]bool{}
	for _, p := range d.Props {
		taken[p.R*g.cols+p.C] = true
	}
	inner := []int{}
	for _, i := range cells {
		if !taken[i] && g.cell[i] == floor && g.floorNeighbours(i/g.cols, i%g.cols) == 4 {
			inner = append(inner, i)
		}
	}
	if len(inner) == 0 {
		for _, i := range cells {
			if !taken[i] && g.cell[i] == floor {
				inner = append(inner, i)
			}
		}
	}
	if len(inner) == 0 {
		return
	}
	i := inner[c.r.IntN(len(inner))]
	d.Props = append(d.Props, DProp{R: i / g.cols, C: i % g.cols, Kind: kind})
}

// pool floods a blob of a room's middle.
func (c *ctx) pool(g *grid, cells []int) {
	if len(cells) < 9 {
		return
	}
	start := cells[c.r.IntN(len(cells))]
	want := len(cells) / 4
	q := []int{start}
	seen := map[int]bool{start: true}
	for len(q) > 0 && want > 0 {
		i := q[0]
		q = q[1:]
		r, col := i/g.cols, i%g.cols
		if g.floorNeighbours(r, col) < 4 {
			continue
		}
		g.cell[i] = water
		want--
		for _, d := range dirs4 {
			j := g.at(r+d[0], col+d[1])
			if !seen[j] && g.room[j] == g.room[i] && c.chance(75) {
				seen[j] = true
				q = append(q, j)
			}
		}
	}
}

func (c *ctx) wandering(o map[string]string) string {
	rows := [][]string{}
	for i := 1; i <= 6; i++ {
		gs, raw, _ := c.dungeonEncounter(o, c.weighted([]string{"easy", "medium"}, []int{2, 1}))
		if gs == nil {
			continue
		}
		rows = append(rows, []string{fmt.Sprint(i), groupNames(gs), c.pick(encounterActivities), commas(raw)})
	}
	return orgTable([]string{"d6", "Encounter", "Doing", "XP"}, rows)
}

// ---- the story -------------------------------------------------------------

func (c *ctx) story(motif string, rooms int) (string, string) {
	place := c.pick(storyPlaces["none"])
	if p, ok := storyPlaces[motif]; ok && c.chance(70) {
		place = c.pick(p)
	}
	who := c.npcName("")
	var name string
	switch c.between(1, 3) {
	case 1:
		name = fmt.Sprintf("The %s %s of %s", capital(c.pick(storyAdjectives)), capital(place), who)
	case 2:
		name = fmt.Sprintf("%s's %s", strings.Fields(who)[0], capital(place))
	default:
		name = fmt.Sprintf("The %s of the %s %s", capital(place), capital(c.pick(storyAdjectives)), capital(c.pick(storyNouns)))
	}
	past := strings.ReplaceAll(c.pick(storyPast), "{who}", who)
	past = strings.ReplaceAll(past, "{place}", place)
	now := c.pick(storyNow)
	if n, ok := motifNow[motif]; ok && c.chance(60) {
		now = c.pick(n)
	}
	hook := c.pick(storyHooks)
	return name, fmt.Sprintf("%s %s %s", past, now, hook)
}

// ---- exits -----------------------------------------------------------------

// Exits says, for each room, which ways lead out of it and where they go,
// read off the map as it is now: a door or an opening on each side, and the
// rooms, stairs or ways out that the corridor beyond reaches.
func (d *Dungeon) Exits() map[int][]DLine {
	R, C := d.Rows, d.Cols
	// On a picture of one's own the grid is not a plan: the ways out are the
	// picture's doors, if it came with any.
	if d.Backdrop != nil && d.Backdrop.Own {
		return d.pictureExits()
	}
	if R == 0 || C == 0 || len(d.Grid) != R || len(d.Room) != R*C {
		return map[int][]DLine{}
	}
	open := func(r, c int) bool {
		return r >= 0 && c >= 0 && r < R && c < C && len(d.Grid[r]) == C && d.Grid[r][c] != rock
	}
	roomAt := func(r, c int) int { return d.Room[r*C+c] }
	doors := map[int]string{}
	for _, x := range d.Doors {
		doors[x.R*C+x.C] = x.Kind
	}
	stairs := map[int]string{}
	for _, s := range d.Stairs {
		stairs[s.R*C+s.C] = s.Dir
	}

	// What lies beyond a step out of a room: walk the corridors from there
	// and note the nearest rooms, stairs and ways out. Only the nearest, or
	// every exit of a well joined dungeon would list the whole dungeon.
	beyond := func(start, self int) string {
		dist := map[int]int{start: 0}
		q := []int{start}
		type hit struct {
			what string
			d    int
		}
		var hits []hit
		got := map[string]bool{}
		note := func(w string, dd int) {
			if !got[w] {
				got[w] = true
				hits = append(hits, hit{w, dd})
			}
		}
		for len(q) > 0 {
			j := q[0]
			q = q[1:]
			y, x := j/C, j%C
			if y == 0 || x == 0 || y == R-1 || x == C-1 {
				note("the way out", dist[j])
			}
			switch stairs[j] {
			case "up":
				note("stairs up", dist[j])
			case "down":
				note("stairs down", dist[j])
			}
			for _, dd := range dirs4 {
				yy, xx := y+dd[0], x+dd[1]
				if !open(yy, xx) {
					continue
				}
				k := yy*C + xx
				if id2 := roomAt(yy, xx); id2 != 0 {
					if id2 != self {
						note(fmt.Sprintf("room %d", id2), dist[j]+1)
					}
					continue
				}
				if _, ok := dist[k]; !ok {
					dist[k] = dist[j] + 1
					q = append(q, k)
				}
			}
		}
		if len(hits) == 0 {
			return "a dead end"
		}
		sort.SliceStable(hits, func(a, b int) bool { return hits[a].d < hits[b].d })
		parts := []string{}
		for _, h := range hits {
			if len(parts) >= 3 || h.d > hits[0].d+10 {
				break
			}
			parts = append(parts, h.what)
		}
		return "to " + strings.Join(parts, ", ")
	}
	cache := map[[2]int]string{}

	type key struct {
		side string
		kind string
		to   string
	}
	out := map[int][]DLine{}
	seen := map[int]map[key]bool{}
	bounds := map[int][4]int{} // minR, minC, maxR, maxC
	for r := 0; r < R; r++ {
		for c := 0; c < C; c++ {
			if id := roomAt(r, c); id != 0 && open(r, c) {
				b, ok := bounds[id]
				if !ok {
					b = [4]int{r, c, r, c}
				}
				b[0], b[1], b[2], b[3] = min(b[0], r), min(b[1], c), max(b[2], r), max(b[3], c)
				bounds[id] = b
			}
		}
	}
	sides := []string{"North", "East", "South", "West"}
	for r := 0; r < R; r++ {
		for c := 0; c < C; c++ {
			id := roomAt(r, c)
			if id == 0 || !open(r, c) {
				continue
			}
			for di, dd := range dirs4 {
				yy, xx := r+dd[0], c+dd[1]
				if !open(yy, xx) || roomAt(yy, xx) == id {
					continue
				}
				// Which side of the room: by the direction of the step,
				// unless the cell is plainly on another face of a round room.
				side := sides[di]
				k := yy*C + xx
				kind := doors[k]
				if kind == "" {
					kind = "opening"
				}
				to := ""
				if other := roomAt(yy, xx); other != 0 {
					to = fmt.Sprintf("to room %d", other)
				} else {
					ck := [2]int{k, id}
					if _, ok := cache[ck]; !ok {
						cache[ck] = beyond(k, id)
					}
					to = cache[ck]
				}
				kk := key{side, kind, to}
				if seen[id] == nil {
					seen[id] = map[key]bool{}
				}
				if seen[id][kk] {
					continue
				}
				seen[id][kk] = true
				out[id] = append(out[id], DLine{Label: side, Text: doorWords[kind] + ", " + to})
			}
		}
	}
	for id := range out {
		order := map[string]int{"North": 0, "East": 1, "South": 2, "West": 3}
		sort.SliceStable(out[id], func(a, b int) bool { return order[out[id][a].Label] < order[out[id][b].Label] })
	}
	return out
}

// pictureExits are the ways out of rooms marked on a picture with doors (a
// Universal VTT export): a door whose two sides, a little way off it, are in
// two different rooms - or a room and the passages - joins them. The side of
// the room it is on is the way it lies from the room's middle.
func (d *Dungeon) pictureExits() map[int][]DLine {
	out := map[int][]DLine{}
	b := d.Backdrop
	if b == nil || b.Ppc <= 0 || len(d.Room) != d.Rows*d.Cols {
		return out
	}
	ox, oy := b.X/b.Ppc, b.Y/b.Ppc
	at := func(x, y float64) (int, bool) {
		c, r := int(math.Floor(x)), int(math.Floor(y))
		if r < 0 || c < 0 || r >= d.Rows || c >= d.Cols {
			return 0, false
		}
		return d.Room[r*d.Cols+c], true
	}
	sum := map[int][3]float64{}
	for i, id := range d.Room {
		if id != 0 {
			s := sum[id]
			sum[id] = [3]float64{s[0] + float64(i%d.Cols) + 0.5, s[1] + float64(i/d.Cols) + 0.5, s[2] + 1}
		}
	}
	name := func(id int) string {
		if id == 0 {
			return "the passages"
		}
		return fmt.Sprintf("room %d", id)
	}
	seen := map[int]map[DLine]bool{}
	add := func(id int, mx, my float64, text string) {
		s := sum[id]
		if s[2] == 0 {
			return
		}
		dx, dy := mx-s[0]/s[2], my-s[1]/s[2]
		side := "North"
		switch {
		case math.Abs(dx) > math.Abs(dy) && dx > 0:
			side = "East"
		case math.Abs(dx) > math.Abs(dy):
			side = "West"
		case dy > 0:
			side = "South"
		}
		l := DLine{Label: side, Text: text}
		if seen[id] == nil {
			seen[id] = map[DLine]bool{}
		}
		if !seen[id][l] {
			seen[id][l] = true
			out[id] = append(out[id], l)
		}
	}
	for _, dr := range b.Doors {
		if len(dr) < 4 {
			continue
		}
		x0, y0, x1, y1 := dr[0]-ox, dr[1]-oy, dr[2]-ox, dr[3]-oy
		mx, my := (x0+x1)/2, (y0+y1)/2
		nx, ny := -(y1 - y0), x1-x0
		l := math.Hypot(nx, ny)
		if l == 0 {
			continue
		}
		nx, ny = nx/l*0.6, ny/l*0.6
		a, okA := at(mx+nx, my+ny)
		c, okC := at(mx-nx, my-ny)
		if !okA || !okC || a == c {
			continue
		}
		kind := "open way"
		if len(dr) > 4 && dr[4] == 1 {
			kind = "door"
		}
		if a != 0 {
			add(a, mx, my, kind+", to "+name(c))
		}
		if c != 0 {
			add(c, mx, my, kind+", to "+name(a))
		}
	}
	order := map[string]int{"North": 0, "East": 1, "South": 2, "West": 3}
	for id := range out {
		sort.SliceStable(out[id], func(a, b int) bool { return order[out[id][a].Label] < order[out[id][b].Label] })
	}
	return out
}

var doorWords = map[string]string{
	"arch": "archway", "door": "wooden door", "locked": "locked door", "trapped": "trapped door",
	"secret": "secret door", "portcullis": "portcullis", "opening": "open way",
}

// ---- org -------------------------------------------------------------------

// roomOf is the room a square is in, 0 for none (or off the map).
func (d *Dungeon) roomOf(r, c int) int {
	if r < 0 || c < 0 || r >= d.Rows || c >= d.Cols || len(d.Room) != d.Rows*d.Cols {
		return 0
	}
	return d.Room[r*d.Cols+c]
}

// RelinkStairs makes the stairs' links relative to dir, the folder of the
// map's org file, for keeping (Rel), or resolves them from it for a screen
// (Abs).
func (d *Dungeon) RelinkStairs(dir string, forKeeping bool) {
	for i := range d.Stairs {
		l := d.Stairs[i].Link
		if l == nil {
			continue
		}
		if forKeeping {
			src := l.File
			if l.Abs != "" {
				src = l.Abs
			}
			if filepath.IsAbs(src) {
				if rel, err := filepath.Rel(dir, src); err == nil {
					src = rel
				}
			}
			l.File, l.Abs = filepath.ToSlash(src), ""
			continue
		}
		if filepath.IsAbs(l.File) {
			l.Abs = l.File
		} else {
			l.Abs = filepath.Clean(filepath.Join(dir, filepath.FromSlash(l.File)))
		}
	}
}

// OrgFile is the dungeon as an org file. maps names the pictures saved beside
// it - the GM's map, the players' map and the one-page sheet - by the link
// text to write for each.
func (d *Dungeon) OrgFile(maps [][2]string, data string) string {
	var b strings.Builder
	b.WriteString("#+TITLE: " + oneLine(d.Name) + "\n")
	b.WriteString("#+STARTUP: overview\n")
	if data != "" {
		b.WriteString("#+DND_DUNGEON: " + data + "\n")
	}
	b.WriteString("\n" + oneLine(d.Story) + "\n\n")
	b.WriteString("* Map\n")
	b.WriteString("  :PROPERTIES:\n")
	keys := []string{}
	for k := range d.Options {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if d.Seed != 0 {
		b.WriteString(fmt.Sprintf("  :%-10s %d\n", "SEED:", d.Seed))
	}
	// A dungeon made on a picture has no seed; it says where it came from
	// (and its drawer is never empty, or the first property the map viewer
	// adds is indented a level in).
	if d.Backdrop != nil && d.Backdrop.Own {
		from := d.Backdrop.Name
		if from == "" {
			from = d.Backdrop.Src
		}
		b.WriteString(fmt.Sprintf("  :%-10s %s\n", "PICTURE:", oneLine(from)))
	}
	for _, k := range keys {
		b.WriteString(fmt.Sprintf("  :%-10s %s\n", strings.ToUpper(k)+":", d.Options[k]))
	}
	b.WriteString("  :END:\n")
	for _, m := range maps {
		b.WriteString("  - " + m[0] + " :: [[file:" + m[1] + "]]\n")
	}
	if strings.TrimSpace(d.Wandering) != "" {
		b.WriteString("* Wandering monsters\n")
		b.WriteString(indentLines(d.Wandering, "  "))
	}
	// Stairs that lead to other maps, as links to their org files.
	var ways []string
	for _, st := range d.Stairs {
		if st.Link == nil || st.Link.File == "" {
			continue
		}
		where := "the stairs"
		if id := d.roomOf(st.R, st.C); id != 0 {
			where = fmt.Sprintf("the stairs in room %d", id)
		}
		name := st.Link.Label
		if name == "" {
			name = strings.TrimSuffix(filepath.Base(st.Link.File), ".org")
		}
		ways = append(ways, fmt.Sprintf("  - %s %s :: [[file:%s][%s]] (arriving at row %d, column %d)\n",
			capital(where), st.Dir, filepath.ToSlash(st.Link.File), oneLine(name), st.Link.R, st.Link.C))
	}
	if len(ways) > 0 {
		b.WriteString("* Ways on\n")
		for _, w := range ways {
			b.WriteString(w)
		}
	}
	b.WriteString("* Rooms\n")
	exits := d.Exits()
	rooms := append([]DRoom(nil), d.Rooms...)
	sort.Slice(rooms, func(a, b int) bool { return rooms[a].Id < rooms[b].Id })
	for _, r := range rooms {
		b.WriteString(fmt.Sprintf("** %d. %s\n", r.Id, oneLine(r.Name)))
		b.WriteString(fmt.Sprintf("   :PROPERTIES:\n   :ROOM:     %d\n   :END:\n", r.Id))
		if n := strings.TrimSpace(r.Note); n != "" {
			b.WriteString("   /" + oneLine(n) + "/\n\n")
		}
		if ex := exits[r.Id]; len(ex) > 0 {
			b.WriteString("   - Exits ::\n")
			for _, e := range ex {
				b.WriteString("     - " + e.Label + " :: " + e.Text + "\n")
			}
		}
		if t := strings.TrimRight(r.Text, "\n"); strings.TrimSpace(t) != "" {
			b.WriteString(indentLines(noHeadings(t), "   "))
		}
	}
	return b.String()
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

func indentLines(s, ind string) string {
	var b strings.Builder
	for _, l := range strings.Split(strings.TrimRight(s, "\n"), "\n") {
		if strings.TrimSpace(l) == "" {
			b.WriteString("\n")
			continue
		}
		b.WriteString(ind + strings.TrimRight(l, " \t") + "\n")
	}
	return b.String()
}

// nestedFields is dfields where a label starting with two spaces is a line of
// the item above it.
func nestedFields(lines []DLine) string {
	var b strings.Builder
	for _, l := range lines {
		if strings.TrimSpace(l.Text) == "" {
			continue
		}
		if strings.HasPrefix(l.Label, "  ") {
			b.WriteString("  - " + strings.TrimSpace(l.Label) + " :: " + l.Text + "\n")
		} else {
			b.WriteString("- " + l.Label + " :: " + l.Text + "\n")
		}
	}
	return b.String()
}
