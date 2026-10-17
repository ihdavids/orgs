package randgen

import (
	"reflect"
	"strings"
	"testing"
)

func TestDungeonsAreWhole(t *testing.T) {
	rs := srd(t)
	for _, f := range DungeonFields() {
		for _, ch := range f.Choices {
			for seed := uint64(1); seed <= 4; seed++ {
				o := map[string]string{f.Id: ch.Value}
				d := GenerateDungeon(rs, DungeonRequest{Seed: seed, Options: o})
				checkDungeon(t, d, o)
			}
		}
	}
}

func checkDungeon(t *testing.T, d *Dungeon, o map[string]string) {
	t.Helper()
	if len(d.Grid) != d.Rows || len(d.Room) != d.Rows*d.Cols {
		t.Fatalf("%v: grid is %d rows for %d, rooms %d", o, len(d.Grid), d.Rows, len(d.Room))
	}
	open := func(r, c int) bool { return r >= 0 && c >= 0 && r < d.Rows && c < d.Cols && d.Grid[r][c] != '#' }
	// Every open cell can be walked to from every other.
	start, total := -1, 0
	for r := 0; r < d.Rows; r++ {
		for c := 0; c < d.Cols; c++ {
			if open(r, c) {
				total++
				if start < 0 {
					start = r*d.Cols + c
				}
			}
		}
	}
	if total == 0 {
		t.Fatalf("%v: no floor at all", o)
	}
	seen := map[int]bool{start: true}
	q := []int{start}
	for len(q) > 0 {
		j := q[0]
		q = q[1:]
		for _, dd := range dirs4 {
			r, c := j/d.Cols+dd[0], j%d.Cols+dd[1]
			if open(r, c) && !seen[r*d.Cols+c] {
				seen[r*d.Cols+c] = true
				q = append(q, r*d.Cols+c)
			}
		}
	}
	if len(seen) != total {
		t.Errorf("%v seed %d: %d of %d floor cells reachable", o, d.Seed, len(seen), total)
	}
	for _, x := range d.Doors {
		if !open(x.R, x.C) {
			t.Errorf("%v: door at %d,%d is in rock", o, x.R, x.C)
		}
	}
	for _, x := range d.Stairs {
		if !open(x.R, x.C) {
			t.Errorf("%v: stairs at %d,%d are in rock", o, x.R, x.C)
		}
	}
	// The key and the map agree about which rooms there are.
	onMap := map[int]bool{}
	for _, id := range d.Room {
		if id != 0 {
			onMap[id] = true
		}
	}
	if len(onMap) != len(d.Rooms) {
		t.Errorf("%v: %d rooms on the map, %d in the key", o, len(onMap), len(d.Rooms))
	}
	for i, r := range d.Rooms {
		if r.Id != i+1 || !onMap[r.Id] {
			t.Errorf("%v: room %d in the key is not numbered in order or not on the map", o, r.Id)
		}
	}
	org := d.OrgFile(nil, "")
	for _, line := range strings.Split(org, "\n") {
		if strings.HasPrefix(line, "*") && !strings.HasPrefix(line, "* ") && !strings.HasPrefix(line, "** ") {
			t.Errorf("%v: unexpected heading %q", o, line)
		}
	}
}

func TestSameSeedSameDungeon(t *testing.T) {
	rs := srd(t)
	o := map[string]string{"layout": "keep", "polymorph": "many", "motif": "undead"}
	a := GenerateDungeon(rs, DungeonRequest{Seed: 99, Options: o})
	b := GenerateDungeon(rs, DungeonRequest{Seed: 99, Options: o})
	if !reflect.DeepEqual(a, b) {
		t.Error("two dungeons from seed 99 differ")
	}
}

// Text somebody typed into a room in the editor is filed under the room's
// heading; a line of stars in it must not become a heading of its own.
func TestRoomTextCannotStartAHeading(t *testing.T) {
	d := &Dungeon{Name: "x", Rows: 1, Cols: 1, Grid: []string{"."}, Room: []int{1},
		Rooms: []DRoom{{Id: 1, Name: "Hall", Text: "* not a heading\n** nor this"}}}
	org := d.OrgFile(nil, "")
	if strings.Contains(org, "\n* not") || strings.Contains(org, "\n** nor") {
		t.Errorf("room text became headings:\n%s", org)
	}
}

func TestExitsFollowTheMap(t *testing.T) {
	// Two rooms joined by a corridor through a door, stairs down off it.
	d := &Dungeon{Rows: 3, Cols: 9,
		Grid: []string{"#########", "#..._...#", "#########"},
		Room: []int{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 1, 0, 0, 0, 2, 2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		Doors:  []DDoor{{R: 1, C: 3, Kind: "locked"}},
		Stairs: []DStair{{R: 1, C: 4, Dir: "down"}},
	}
	d.Grid[1] = "#.......#"
	ex := d.Exits()
	if len(ex[1]) != 1 || ex[1][0].Label != "East" || !strings.Contains(ex[1][0].Text, "locked door") ||
		!strings.Contains(ex[1][0].Text, "stairs down") || !strings.Contains(ex[1][0].Text, "room 2") {
		t.Errorf("room 1 exits: %+v", ex[1])
	}
	if len(ex[2]) != 1 || ex[2][0].Label != "West" || !strings.Contains(ex[2][0].Text, "room 1") {
		t.Errorf("room 2 exits: %+v", ex[2])
	}
}

// The same cases as worg's dungeon.test.ts "room shapes touch the middle of
// every side": the editor's room tool and the generator draw one set of shapes.
func TestRoomShapesTouchEverySide(t *testing.T) {
	for _, shape := range []string{"ellipse", "octagon", "cross"} {
		h, w := 7, 9
		for _, p := range [][2]int{{0, 4}, {6, 4}, {3, 0}, {3, 8}} {
			if !inShape(shape, p[0], p[1], h, w) {
				t.Errorf("%s: %v is not floor", shape, p)
			}
		}
		if inShape(shape, 0, 0, h, w) {
			t.Errorf("%s: the corner is floor", shape)
		}
	}
}

func TestMonstersHaveStatBlocks(t *testing.T) {
	for _, m := range Monsters() {
		if len(m.Abilities) != 6 {
			t.Errorf("%s has %d ability scores", m.Name, len(m.Abilities))
		}
		if m.Speed == "" {
			t.Errorf("%s has no speed", m.Name)
		}
	}
	g := FindMonster("goblins")
	if g == nil || g.Name != "Goblin" || g.Init() != 2 || len(g.Actions) < 2 {
		t.Errorf("goblin: %+v", g)
	}
	if a := FindMonster("ABOLETH"); a == nil || len(a.Legendary) != 3 || a.Init() != -1 {
		t.Errorf("aboleth: %+v", a)
	}
	if FindMonster("Snarglebeast") != nil {
		t.Error("found a monster that is not there")
	}
	if got := SearchMonsters("dragon red", 5); len(got) == 0 || !strings.Contains(got[0].Name, "Red Dragon") {
		t.Errorf("search: %v", got)
	}
}

// Rooms marked on a picture are filled in like a generated dungeon's: room 1
// as the way in, a size only when one is given, the same rooms for the same
// seed, and a table and a story when asked for.
func TestStockDungeon(t *testing.T) {
	rs := srd(t)
	req := StockRequest{Seed: 9, Options: map[string]string{"level": "3"}, Rooms: []StockRoom{{Id: 1}, {Id: 2, W: 4, H: 3}, {Id: 7}}, Wandering: true, Story: true}
	a := StockDungeon(rs, req)
	if len(a.Rooms) != 3 || a.Rooms[0].Id != 1 || a.Rooms[2].Id != 7 {
		t.Fatalf("rooms: %+v", a.Rooms)
	}
	for _, r := range a.Rooms {
		if r.Name == "" || r.Note == "" || !strings.Contains(r.Text, "Here") {
			t.Errorf("room %d is not filled in: %+v", r.Id, r)
		}
		if strings.Contains(r.Text, "\n*") || strings.HasPrefix(r.Text, "*") {
			t.Errorf("room %d has a line of stars", r.Id)
		}
	}
	if strings.Contains(a.Rooms[0].Text, "Size") || !strings.Contains(a.Rooms[1].Text, "40 × 30 ft") {
		t.Errorf("sizes: %q / %q", a.Rooms[0].Text, a.Rooms[1].Text)
	}
	if !strings.Contains(a.Wandering, "Encounter") || a.Name == "" || a.Story == "" {
		t.Errorf("no table or story: %+v", a)
	}
	if b := StockDungeon(rs, req); !reflect.DeepEqual(a, b) {
		t.Errorf("the same seed gave different rooms")
	}
}

// A dungeon on a picture of one's own has no plan to read exits off.
func TestNoExitsOnOwnPicture(t *testing.T) {
	d := GenerateDungeon(srd(t), DungeonRequest{Seed: 3})
	if len(d.Exits()) == 0 {
		t.Fatal("a generated dungeon should have exits")
	}
	d.Backdrop = &Backdrop{Own: true}
	if len(d.Exits()) != 0 {
		t.Error("exits read off a picture's grid")
	}
	d.Backdrop.Own = false
	if len(d.Exits()) == 0 {
		t.Error("a picture over a generated dungeon keeps its exits")
	}
}

// A room off a picture says what its lights are, and an irregular one its
// area; a room with none is dark.
func TestStockWithHints(t *testing.T) {
	rs := srd(t)
	a := StockDungeon(rs, StockRequest{Seed: 4, Rooms: []StockRoom{
		{Id: 1, W: 5, H: 4, Hints: &RoomHints{Area: 12, Lights: 3, Colour: "warm"}},
		{Id: 2, W: 2, H: 2, Hints: &RoomHints{Area: 4}},
		{Id: 3, W: 3, H: 3},
	}})
	if !strings.Contains(a.Rooms[0].Text, "Light") || !strings.Contains(a.Rooms[0].Text, "three ") || !strings.Contains(a.Rooms[0].Text, "about 300 sq ft, irregular (50 × 40 ft across)") {
		t.Errorf("room 1: %s", a.Rooms[0].Text)
	}
	if !strings.Contains(a.Rooms[1].Text, "dark") || strings.Contains(a.Rooms[1].Text, "irregular") {
		t.Errorf("room 2: %s", a.Rooms[1].Text)
	}
	if strings.Contains(a.Rooms[2].Text, "Light") {
		t.Errorf("a room with no hints gets no light line: %s", a.Rooms[2].Text)
	}
}

// On a picture with doors, a door between two rooms (or a room and the
// passages) is a way out of each.
func TestPictureExits(t *testing.T) {
	// 6 × 3 squares: room 1 on the left (columns 0-1), passage in the middle,
	// room 2 on the right (columns 4-5). A shut door at x = 2, an open way at x = 4.
	d := &Dungeon{Rows: 3, Cols: 6, Room: []int{1, 1, 0, 0, 2, 2, 1, 1, 0, 0, 2, 2, 1, 1, 0, 0, 2, 2}}
	d.Backdrop = &Backdrop{Own: true, Ppc: 100, Doors: [][]float64{{2, 1, 2, 2, 1}, {4, 1, 4, 2, 0}}}
	ex := d.Exits()
	if len(ex[1]) != 1 || ex[1][0] != (DLine{"East", "door, to the passages"}) {
		t.Errorf("room 1: %+v", ex[1])
	}
	if len(ex[2]) != 1 || ex[2][0] != (DLine{"West", "open way, to the passages"}) {
		t.Errorf("room 2: %+v", ex[2])
	}
}

// A dwelling is a house: household rooms written up as a home's, every room
// reached through a door, a front door to the outside, happenings rather
// than wandering monsters.
func TestDwelling(t *testing.T) {
	rs := srd(t)
	for seed := uint64(1); seed <= 6; seed++ {
		d := GenerateDungeon(rs, DungeonRequest{Seed: seed, Options: map[string]string{"kind": "dwelling", "size": "medium"}})
		checkDungeon(t, d, d.Options)
		if len(d.Rooms) < 3 {
			t.Fatalf("seed %d: %d rooms", seed, len(d.Rooms))
		}
		household := map[string]bool{}
		for _, n := range dwellingRooms {
			household[n] = true
		}
		for _, r := range d.Rooms {
			if !household[r.Name] {
				t.Errorf("seed %d: a %q in a house", seed, r.Name)
			}
			if strings.Contains(r.Text, "Monsters") || strings.Contains(r.Text, "Trap") {
				t.Errorf("seed %d: room %d is written up as a dungeon's: %s", seed, r.Id, r.Text)
			}
		}
		if !strings.Contains(d.Wandering, "Happening") {
			t.Errorf("seed %d: %s", seed, d.Wandering)
		}
		if len(d.Doors) < len(d.Rooms) {
			t.Errorf("seed %d: %d doors for %d rooms", seed, len(d.Doors), len(d.Rooms))
		}
		edge := false
		for r := 0; r < d.Rows; r++ {
			for c := 0; c < d.Cols; c++ {
				if (r == 0 || c == 0 || r == d.Rows-1 || c == d.Cols-1) && d.Grid[r][c] != '#' {
					edge = true
				}
			}
		}
		if !edge {
			t.Errorf("seed %d: no way in from outside", seed)
		}
	}
}

// A cave is a cavern, dressed as a cave, with dripstone and the like.
func TestCave(t *testing.T) {
	d := GenerateDungeon(srd(t), DungeonRequest{Seed: 5, Options: map[string]string{"kind": "cave"}})
	if d.Options["layout"] != "cavernous" {
		t.Errorf("layout %q", d.Options["layout"])
	}
	cavey := 0
	for _, p := range d.Props {
		if p.Kind == "stalagmite" || p.Kind == "mushrooms" || p.Kind == "crystals" {
			cavey++
		}
	}
	if cavey == 0 {
		t.Error("a cave with no dripstone, fungus or crystals")
	}
}

// A stair's link is kept relative to its map's folder, written as an org
// link under "Ways on", and resolved again for a screen.
func TestStairLinks(t *testing.T) {
	d := GenerateDungeon(srd(t), DungeonRequest{Seed: 3, Options: map[string]string{"stairs": "many"}})
	if len(d.Stairs) == 0 {
		t.Fatal("no stairs")
	}
	d.Stairs[0].Link = &StairLink{Abs: "/notes/dnd/cellar.org", R: 4, C: 6, Label: "The Cellar"}
	d.RelinkStairs("/notes/dnd/keep", true)
	if l := d.Stairs[0].Link; l.File != "../cellar.org" || l.Abs != "" {
		t.Fatalf("kept as %+v", l)
	}
	org := d.OrgFile(nil, "x.dungeon.json")
	if !strings.Contains(org, "* Ways on") || !strings.Contains(org, "[[file:../cellar.org][The Cellar]] (arriving at row 4, column 6)") {
		t.Errorf("org file:\n%s", org)
	}
	d.RelinkStairs("/notes/dnd/keep", false)
	if d.Stairs[0].Link.Abs != "/notes/dnd/cellar.org" {
		t.Errorf("resolved to %q", d.Stairs[0].Link.Abs)
	}
}
