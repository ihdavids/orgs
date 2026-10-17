package randgen

import (
	"fmt"
	"sort"
	"strings"
)

// Two other kinds of place the dungeon generator makes.
//
// A dwelling, in the manner of watabou's Dwellings
// (https://watabou.itch.io/dwellings - the idea, not its code): a house's
// floor cut into rooms by walls, each split leaving a door between its two
// halves, the front door on the outside; rooms given a household's purposes
// (hall, kitchen, bedchambers...) and written up as rooms of a home - what
// furnishes them, what is to be noticed, who is usually there - rather than
// as a dungeon's.
//
// A cave, in the manner of watabou's Cave Generator: the cavern layout with
// more water, and chambers dressed and furnished as caves are - dripstone,
// fungus, crystals, pools.

// ---- the dwelling's plan ----------------------------------------------------

type rectArea struct{ r, c, h, w int }

// dwelling builds the floor plan: a footprint (an oblong, or an L), split
// into rooms with one-cell walls, a door through every split, a front door.
func (c *ctx) dwelling(o map[string]string) (*grid, []roomBox) {
	sizes := map[string][2]int{
		"fine": {8, 6}, "diminutive": {9, 7}, "tiny": {11, 8}, "small": {14, 10}, "medium": {18, 13},
		"large": {22, 16}, "huge": {27, 19}, "gargantuan": {32, 22}, "colossal": {38, 26},
	}
	sz, ok := sizes[o["size"]]
	if !ok {
		sz = sizes["medium"]
	}
	w, h := sz[0]+c.between(-1, 2), sz[1]+c.between(-1, 1)
	m := 2
	g := newGrid(h+2*m, w+2*m)
	foot := []rectArea{{m, m, h, w}}
	// An L: a corner left out, on some layouts.
	if strings.Contains("cross keep dagger saltire box", o["layout"]) && w >= 12 && h >= 9 {
		cw, ch := w/3+c.between(0, 2), h/3+c.between(0, 1)
		if c.chance(50) {
			foot = []rectArea{{m, m, h, w - cw}, {m + ch, m + w - cw, h - ch, cw}}
		} else {
			foot = []rectArea{{m + ch, m, h - ch, w}, {m, m, ch, w - cw}}
		}
	}
	for _, a := range foot {
		for r := a.r; r < a.r+a.h; r++ {
			for col := a.c; col < a.c+a.w; col++ {
				g.cell[g.at(r, col)] = floor
			}
		}
	}
	target := map[string]int{"small": 10, "medium": 16, "large": 26, "huge": 40, "gargantuan": 55, "colossal": 70}[o["roomSize"]]
	if target == 0 {
		target = 16
	}
	var leaves []rectArea
	var split func(a rectArea, depth int)
	split = func(a rectArea, depth int) {
		if a.w*a.h <= target*(1+c.r.IntN(2)) || (a.w < 7 && a.h < 7) || depth > 8 {
			leaves = append(leaves, a)
			return
		}
		vertical := a.w > a.h || (a.w == a.h && c.chance(50))
		if vertical && a.w < 7 {
			vertical = false
		}
		if !vertical && a.h < 7 {
			vertical = true
		}
		if vertical && a.w < 7 {
			leaves = append(leaves, a)
			return
		}
		// A wall across, not where it would block a door already made.
		for tries := 0; tries < 12; tries++ {
			if vertical {
				col := a.c + 3 + c.r.IntN(a.w-6)
				if g.door[g.at(a.r-1, col)] != "" || g.door[g.at(a.r+a.h, col)] != "" {
					continue
				}
				for r := a.r; r < a.r+a.h; r++ {
					g.cell[g.at(r, col)] = rock
				}
				dr := a.r + c.r.IntN(a.h)
				g.cell[g.at(dr, col)] = floor
				g.door[g.at(dr, col)] = "door"
				split(rectArea{a.r, a.c, a.h, col - a.c}, depth+1)
				split(rectArea{a.r, col + 1, a.h, a.c + a.w - col - 1}, depth+1)
				return
			}
			row := a.r + 3 + c.r.IntN(a.h-6)
			if g.door[g.at(row, a.c-1)] != "" || g.door[g.at(row, a.c+a.w)] != "" {
				continue
			}
			for col := a.c; col < a.c+a.w; col++ {
				g.cell[g.at(row, col)] = rock
			}
			dc := a.c + c.r.IntN(a.w)
			g.cell[g.at(row, dc)] = floor
			g.door[g.at(row, dc)] = "door"
			split(rectArea{a.r, a.c, row - a.r, a.w}, depth+1)
			split(rectArea{row + 1, a.c, a.r + a.h - row - 1, a.w}, depth+1)
			return
		}
		leaves = append(leaves, a)
	}
	for _, a := range foot {
		split(a, 0)
	}
	// An L's two wings are joined by a door where they meet.
	if len(foot) == 2 {
		for tries := 0; tries < 40; tries++ {
			i := c.r.IntN(len(g.cell))
			r, col := i/g.cols, i%g.cols
			if g.cell[i] != rock || r < m || col < m || r >= g.rows-m || col >= g.cols-m {
				continue
			}
			if (g.in(r-1, col) && g.open(g.at(r-1, col)) && g.in(r+1, col) && g.open(g.at(r+1, col))) ||
				(g.in(r, col-1) && g.open(g.at(r, col-1)) && g.in(r, col+1) && g.open(g.at(r, col+1))) {
				g.cell[i] = floor
				g.door[i] = "door"
				break
			}
		}
	}
	// The rooms, biggest first.
	sort.Slice(leaves, func(a, b int) bool { return leaves[a].w*leaves[a].h > leaves[b].w*leaves[b].h })
	var rooms []roomBox
	for k, a := range leaves {
		if a.w < 1 || a.h < 1 {
			continue
		}
		id := k + 1
		for r := a.r; r < a.r+a.h; r++ {
			for col := a.c; col < a.c+a.w; col++ {
				if g.cell[g.at(r, col)] == floor && g.door[g.at(r, col)] == "" {
					g.room[g.at(r, col)] = id
				}
			}
		}
		rooms = append(rooms, roomBox{id: id, r: a.r, c: a.c, h: a.h, w: a.w})
	}
	// The front door: on the bottom wall of the biggest room, if it has one
	// on the outside, else wherever a room meets the outside.
	front := false
	for _, rb := range rooms {
		for _, side := range [][3]int{{rb.r + rb.h, 1, 0}, {rb.r - 1, -1, 0}} {
			row := side[0]
			if !g.in(row, rb.c) || !g.in(row+side[1], rb.c) {
				continue
			}
			for tries := 0; tries < 6 && !front; tries++ {
				col := rb.c + c.r.IntN(rb.w)
				out := g.at(row, col)
				if g.cell[out] == rock && g.in(row+side[1], col) && g.cell[g.at(row+side[1], col)] == rock && !g.inside(row+side[1], col, foot) {
					g.cell[out] = floor
					g.door[out] = c.pick([]string{"door", "door", "locked"})
					// A step outside, and a path to the edge.
					for rr := row + side[1]; g.in(rr, col); rr += side[1] {
						g.cell[g.at(rr, col)] = floor
					}
					front = true
				}
			}
			if front {
				break
			}
		}
		if front {
			break
		}
	}
	// Stairs: up from the biggest room, down from a small one.
	if o["stairs"] != "no" && len(rooms) > 2 {
		c.stairIn(g, rooms[0], "up")
		c.stairIn(g, rooms[len(rooms)-1], "down")
	}
	return g, rooms
}

func (g *grid) inside(r, c int, foot []rectArea) bool {
	for _, a := range foot {
		if r >= a.r && r < a.r+a.h && c >= a.c && c < a.c+a.w {
			return true
		}
	}
	return false
}

func (c *ctx) stairIn(g *grid, rb roomBox, dir string) {
	for tries := 0; tries < 20; tries++ {
		r, col := rb.r+c.r.IntN(rb.h), rb.c+c.r.IntN(rb.w)
		i := g.at(r, col)
		if g.cell[i] == floor && g.door[i] == "" && g.floorNeighbours(r, col) >= 2 {
			g.stairs[i] = dir
			return
		}
	}
}

// ---- the household ------------------------------------------------------------

var dwellingKinds = []string{"townhouse", "farmhouse", "manor", "merchant's house", "wizard's house", "cottage", "hunting lodge", "vicarage"}

// The rooms of a household, in the order a house needs them.
var dwellingRooms = []string{"Hall", "Kitchen", "Bedchamber", "Parlour", "Pantry", "Bedchamber", "Study", "Dining room",
	"Storeroom", "Servants' quarters", "Privy", "Nursery", "Library", "Workshop", "Guest room", "Chapel", "Wardrobe",
	"Scullery", "Bedchamber", "Music room", "Trophy room", "Strongroom"}

var dwellingProps = map[string][]string{
	"Hall": {"table", "brazier"}, "Kitchen": {"table", "brazier", "barrel"}, "Bedchamber": {"bed", "chest"},
	"Parlour": {"table"}, "Pantry": {"barrel", "barrel"}, "Study": {"table", "chest"}, "Dining room": {"table"},
	"Storeroom": {"barrel", "chest", "barrel"}, "Servants' quarters": {"bed", "bed"}, "Nursery": {"bed"},
	"Library": {"table"}, "Workshop": {"table", "barrel"}, "Guest room": {"bed"}, "Chapel": {"altar", "statue"},
	"Wardrobe": {"chest", "chest"}, "Scullery": {"barrel", "well"}, "Music room": {"table"}, "Trophy room": {"statue"},
	"Strongroom": {"chest", "chest"},
}

var dwellingFurnish = map[string][]string{
	"Hall":               {"a long table with benches", "a great hearth with a spit", "antlers mounted over the fire", "a worn rug before the hearth"},
	"Kitchen":            {"a scrubbed oak table", "pots hanging from hooks", "a bread oven built into the wall", "strings of onions and herbs"},
	"Bedchamber":         {"a curtained bed", "a washstand and basin", "a clothes chest at the bed's foot", "a chamber pot under the bed"},
	"Parlour":            {"cushioned chairs by the window", "a small writing desk", "a cabinet of curiosities", "a game board set out mid-play"},
	"Pantry":             {"shelves of preserves", "sacks of flour and grain", "a hanging ham", "a cheese under a cloth"},
	"Study":              {"a desk piled with papers", "shelves of ledgers", "a globe on a stand", "maps pinned to the wall"},
	"Dining room":        {"a polished table set for six", "a sideboard of pewter", "candlesticks gone green", "a portrait over the fireplace"},
	"Storeroom":          {"crates and barrels", "broken furniture stacked high", "old tools", "rolled carpets"},
	"Servants' quarters": {"narrow cots", "a shared washbasin", "a row of pegs with aprons", "a small locked box under a bed"},
	"Privy":              {"a wooden seat over a shaft", "a jug of water", "old almanac pages on a nail"},
	"Nursery":            {"a cradle", "a rocking horse", "wooden soldiers", "a nurse's chair"},
	"Library":            {"floor-to-ceiling shelves", "a reading lectern", "a ladder on a rail", "a locked glass case"},
	"Workshop":           {"a workbench of tools", "half-finished work in a vice", "shavings on the floor", "jars of nails and glue"},
	"Guest room":         {"a made bed nobody sleeps in", "fresh linen", "a vase of dried flowers"},
	"Chapel":             {"a small altar", "a kneeler", "a stained-glass window", "a reliquary"},
	"Wardrobe":           {"racks of clothes", "a full-length mirror", "hatboxes", "a dressmaker's dummy"},
	"Scullery":           {"a stone sink", "a pump", "stacked crockery"},
	"Music room":         {"a harpsichord", "lutes on the wall", "a music stand", "a stack of songbooks"},
	"Trophy room":        {"mounted heads", "a stuffed bear", "crossed spears", "a case of medals"},
	"Strongroom":         {"an iron-bound door", "a heavy chest chained to the floor", "a ledger of holdings"},
}

var dwellingDressing = []string{
	"a fire crackles in the hearth", "the room smells of beeswax", "dust sheets cover the furniture", "sunlight falls through a dirty window",
	"the floorboards creak", "a draught rattles the shutters", "it smells of bread and woodsmoke", "a clock ticks somewhere",
	"everything is neat as a pin", "the room has not been aired in weeks", "the curtains are drawn", "a candle has burned down to a stub",
	"children's drawings are tacked to the wall", "a cat sleeps on a chair", "there are muddy boot prints on the floor", "the walls are freshly whitewashed",
}

var dwellingNotes = []string{
	"a letter, half written and abandoned", "a loose floorboard with a hollow beneath", "a portrait whose eyes have been scratched out",
	"a key hidden under a vase", "a ledger with one page torn out", "a locked drawer", "a smell of something burned in the grate",
	"a hidden door behind a bookcase", "a purse of coins forgotten under a cushion", "a map with a place circled", "a bloodstain scrubbed but not gone",
	"a strange symbol carved under the table", "a bottle of poison labelled as cordial", "a diary with a broken lock",
}

func (c *ctx) dwellingStock(d *Dungeon, g *grid, rooms []roomBox) {
	o := d.Options
	band := levelBand(optNum(o["level"], 1, 1, 20))
	purposes := append([]string(nil), dwellingRooms...)
	for k, b := range rooms {
		name := purposes[min(k, len(purposes)-1)]
		if k >= len(purposes) {
			name = c.pick(dwellingRooms)
		}
		look := c.pick(dwellingDressing)
		cells := []int{}
		for i, id := range g.room {
			if id == b.id {
				cells = append(cells, i)
			}
		}
		lines := []DLine{{"Size", fmt.Sprintf("%d × %d ft", b.w*5, b.h*5)}, {"Here", look}}
		if f := dwellingFurnish[name]; len(f) > 0 {
			picks := []string{}
			for _, x := range f {
				if c.chance(60) {
					picks = append(picks, x)
				}
			}
			if len(picks) == 0 {
				picks = append(picks, f[0])
			}
			lines = append(lines, DLine{"Furnishings", strings.Join(picks, ", ")})
		}
		note := capital(look) + "."
		if c.chance(45) {
			n := c.pick(dwellingNotes)
			lines = append(lines, DLine{"Of note", n})
			if strings.Contains(n, "purse") {
				lines = append(lines, DLine{"Treasure", c.purse(band).String()})
			}
		}
		if c.chance(40) {
			who, _, _, summary := c.npc()
			lines = append(lines, DLine{"Usually here", who + ", " + strings.ToLower(summary)})
			note += " " + strings.Split(who, " ")[0] + " is often here."
		}
		if name == "Strongroom" || (name == "Study" && c.chance(50)) {
			cs, vals, items := c.hoard(band)
			lines = append(lines, DLine{"Valuables", c.hoardLine(cs, vals, items)})
		}
		for _, pk := range dwellingProps[name] {
			c.propIn(d, g, cells, pk)
		}
		d.Rooms = append(d.Rooms, DRoom{Id: b.id, Name: name, Note: note, Text: nestedFields(lines)})
	}
}

var dwellingHappenings = []string{
	"a servant comes in to light the lamps", "the dog starts barking at the door", "the owner comes home early",
	"a cat knocks something off a shelf", "a messenger knocks with a sealed letter", "a guard patrol passes outside",
	"the cook shouts for help in the kitchen", "a child wanders in looking for a toy", "rain begins to drum on the roof",
	"a visitor is shown into the parlour", "someone is heard on the stairs", "a fire flares up in a hearth",
}

func (c *ctx) dwellingHappens() string {
	rows := [][]string{}
	picked := map[string]bool{}
	for i := 1; i <= 6; i++ {
		h := c.pick(dwellingHappenings)
		for picked[h] {
			h = c.pick(dwellingHappenings)
		}
		picked[h] = true
		rows = append(rows, []string{fmt.Sprint(i), capital(h)})
	}
	return orgTable([]string{"d6", "Happening"}, rows)
}

func (c *ctx) dwellingStory() (string, string) {
	kind := c.pick(dwellingKinds)
	who := c.npcName("")
	first := strings.Split(who, " ")[0]
	var name string
	switch c.between(1, 3) {
	case 1:
		name = fmt.Sprintf("%s's %s", first, capital(kind))
	case 2:
		name = fmt.Sprintf("The %s %s", capital(c.pick(storyAdjectives)), capital(kind))
	default:
		name = fmt.Sprintf("The %s at %s", capital(kind), c.pick([]string{"Mill Lane", "the Crossroads", "Hob's End", "the Old Bridge", "Raven Hill", "the Market"}))
	}
	hooks := []string{
		fmt.Sprintf("%s has not been seen for three days, and the shutters stay closed.", who),
		fmt.Sprintf("%s wants something fetched from the house, and will not say why.", who),
		fmt.Sprintf("Lights move in the windows at night, though %s swears the house is empty.", who),
		fmt.Sprintf("%s is hiding something in the house, and someone means to find it.", who),
		fmt.Sprintf("A will names a stranger heir to everything %s owned.", who),
	}
	return name, fmt.Sprintf("The %s of %s. %s", kind, who, c.pick(hooks))
}

// ---- the cave -------------------------------------------------------------------

var caveDressing = []string{
	"water drips from a forest of stalactites", "the floor is slick with mud", "pale fungus glows along the walls",
	"the air tastes of minerals", "bats stir overhead", "a cold wind moans through a crack", "crystals glitter in the lamplight",
	"the ceiling is low enough to stoop", "an underground stream gurgles out of sight", "the walls are streaked red with iron",
	"bones are scattered among the rocks", "the echo of every step goes on and on", "roots hang from the roof",
	"a sheet of flowstone covers one wall", "the floor drops away in terraces",
}

var caveProps = map[string][]string{
	"Dripping grotto": {"stalagmite", "stalagmite"}, "Fungus cave": {"mushrooms", "mushrooms"}, "Bat roost": {"bones"},
	"Crystal hollow": {"crystals", "crystals"}, "Echoing cavern": {"stalagmite", "stalagmite", "stalagmite"},
	"Mud pit": {"rubble"}, "Collapsed passage": {"rubble", "rubble"}, "Underground pool": {"stalagmite"},
	"Bone-strewn den": {"bones", "bones"}, "Glowworm gallery": {"mushrooms"}, "Mineral terraces": {"crystals", "stalagmite"},
	"Root-choked hollow": {"rubble"}, "Chasm edge": {"rubble"},
}
