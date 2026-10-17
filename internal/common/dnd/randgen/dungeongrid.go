package randgen

import (
	"math"
	"sort"
)

// The shape of a dungeon: which cells are floor, which room each belongs to,
// and where the doors and stairs are. Stocking the rooms is dungeon.go.
//
// The usual rooms-and-maze construction, written for this: rooms sit on odd
// rows and columns with odd sizes, so every wall is an even row or column and
// a room's wall is one cell thick; a maze fills the odd cells left over; then
// the separate pieces are joined through single wall cells (connectors) by a
// spanning tree, with a few extra joins for loops, and the dead ends are
// trimmed. A cavern is a cellular automaton instead, cut into areas.

const (
	rock  = '#'
	floor = '.'
	water = '~'
)

// grid is the working form of a map while it is built.
type grid struct {
	rows, cols int
	cell       []byte // rock, floor or water
	room       []int  // room id per cell, 0 = corridor or none
	blocked    []bool // outside the dungeon's outline
	reserved   []bool // a room's box, which the maze stays out of
	region     []int  // connected piece, while joining
	door       map[int]string
	stairs     map[int]string
	keep       map[int]bool // never trimmed as a dead end
}

func newGrid(rows, cols int) *grid {
	n := rows * cols
	g := &grid{rows: rows, cols: cols, cell: make([]byte, n), room: make([]int, n),
		blocked: make([]bool, n), reserved: make([]bool, n), region: make([]int, n),
		door: map[int]string{}, stairs: map[int]string{}, keep: map[int]bool{}}
	for i := range g.cell {
		g.cell[i] = rock
	}
	return g
}

func (g *grid) at(r, c int) int { return r*g.cols + c }
func (g *grid) in(r, c int) bool {
	return r >= 0 && c >= 0 && r < g.rows && c < g.cols
}
func (g *grid) open(i int) bool { return g.cell[i] != rock }

var dirs4 = [4][2]int{{-1, 0}, {0, 1}, {1, 0}, {0, -1}}

func (g *grid) floorNeighbours(r, c int) int {
	n := 0
	for _, d := range dirs4 {
		rr, cc := r+d[0], c+d[1]
		if g.in(rr, cc) && g.open(g.at(rr, cc)) {
			n++
		}
	}
	return n
}

// ---- sizes and outlines ----------------------------------------------------

var dungeonSizes = map[string]int{
	"fine": 11, "diminutive": 13, "tiny": 17, "small": 21, "medium": 27,
	"large": 35, "huge": 43, "gargantuan": 55, "colossal": 71,
}

func odd(n int) int {
	if n%2 == 0 {
		return n + 1
	}
	return n
}

func dungeonDims(size, layout string) (rows, cols int) {
	n, ok := dungeonSizes[size]
	if !ok {
		n = dungeonSizes["medium"]
	}
	rows = odd(n)
	cols = rows
	switch layout {
	case "rectangle", "box", "cavernous", "keep":
		cols = odd(int(float64(n) * 1.4))
	}
	return
}

// outline blocks the cells outside a layout's shape. The border ring is
// always blocked so nothing runs off the map except on purpose.
func (g *grid) outline(layout string) {
	R, C := g.rows, g.cols
	cy, cx := float64(R-1)/2, float64(C-1)/2
	for r := 0; r < R; r++ {
		for c := 0; c < C; c++ {
			b := r == 0 || c == 0 || r == R-1 || c == C-1
			fy, fx := (float64(r)-cy)/cy, (float64(c)-cx)/cx // -1..1
			switch layout {
			case "box":
				b = b || (math.Abs(fy) < 0.34 && math.Abs(fx) < 0.34)
			case "cross":
				b = b || (math.Abs(fy) > 0.36 && math.Abs(fx) > 0.36)
			case "keep":
				// A square body with a tower at each corner.
				body := math.Abs(fy) < 0.7 && math.Abs(fx) < 0.7
				tower := math.Abs(fy) > 0.45 && math.Abs(fx) > 0.45
				b = b || !(body || tower)
			case "round":
				b = b || fx*fx+fy*fy > 1.0
			case "hexagon":
				b = b || math.Abs(fy) > 0.98 || math.Abs(fy)*0.5+math.Abs(fx)*0.87 > 0.87
			case "dagger":
				blade := math.Abs(fx) < 0.22 && fy < 0.55
				guard := math.Abs(fy-0.45) < 0.16
				hilt := math.Abs(fx) < 0.14
				b = b || !(blade || guard || hilt)
			case "saltire":
				d1 := math.Abs(fx - fy)
				d2 := math.Abs(fx + fy)
				b = b || (d1 > 0.42 && d2 > 0.42)
			}
			g.blocked[g.at(r, c)] = b
		}
	}
}

// ---- rooms -----------------------------------------------------------------

var roomSizes = map[string][2]int{
	"small": {3, 5}, "medium": {3, 7}, "large": {5, 9}, "huge": {5, 11},
	"gargantuan": {7, 13}, "colossal": {9, 15},
}

type roomBox struct {
	id, r, c, h, w int
	shape          string // "" for a rectangle; ellipse, octagon, cross
}

// inShape says whether a cell of a room's box is floor, for a shape. Every
// shape reaches the middle of each side of its box, so the joining always
// has somewhere to put a door. worg's inShape (dungeon.ts) draws the same
// shapes for the editor's room tool.
func inShape(shape string, r, c, h, w int) bool {
	if shape == "" || h < 3 || w < 3 {
		return true
	}
	cy, cx := float64(h-1)/2, float64(w-1)/2
	switch shape {
	case "ellipse":
		fy, fx := (float64(r)-cy)/(cy+0.5), (float64(c)-cx)/(cx+0.5)
		return fy*fy+fx*fx <= 1.0
	case "octagon":
		cut := max(1, int(float64(min(h, w))/3.4))
		dy, dx := min(r, h-1-r), min(c, w-1-c)
		return dy+dx >= cut
	case "cross":
		ty, tx := max(1, h/3), max(1, w/3)
		return (r >= ty && r <= h-1-ty) || (c >= tx && c <= w-1-tx)
	}
	return true
}

func (c *ctx) oddBetween(lo, hi int) int {
	lo, hi = odd(lo), odd(hi)
	if hi < lo {
		hi = lo
	}
	return lo + 2*c.r.IntN((hi-lo)/2+1)
}

func (g *grid) fits(r, c, h, w int) bool {
	if r < 1 || c < 1 || r+h > g.rows-1 || c+w > g.cols-1 {
		return false
	}
	for y := r - 1; y <= r+h; y++ {
		for x := c - 1; x <= c+w; x++ {
			i := g.at(y, x)
			if g.blocked[i] && !(y == r-1 || y == r+h || x == c-1 || x == c+w) {
				return false
			}
			if g.reserved[i] {
				return false
			}
		}
	}
	return true
}

func (g *grid) carveRoom(b roomBox) {
	for y := b.r; y < b.r+b.h; y++ {
		for x := b.c; x < b.c+b.w; x++ {
			i := g.at(y, x)
			g.reserved[i] = true
			if !inShape(b.shape, y-b.r, x-b.c, b.h, b.w) {
				continue
			}
			g.cell[i] = floor
			g.room[i] = b.id
		}
	}
}

func (c *ctx) placeRooms(g *grid, sizeOpt, layoutOpt, polyOpt string) []roomBox {
	sz, ok := roomSizes[sizeOpt]
	if !ok {
		sz = roomSizes["medium"]
	}
	maxDim := min(g.rows, g.cols) - 2
	if sz[1] > maxDim {
		sz[1] = odd(maxDim - 2)
	}
	if sz[0] > sz[1] {
		sz[0] = sz[1]
	}
	avg := float64((sz[0] + sz[1]) / 2 + 1)
	area := float64(g.rows * g.cols)
	per := map[string]float64{"sparse": 3.2, "scattered": 2.0, "dense": 1.1, "symmetric": 2.0}[layoutOpt]
	if per == 0 {
		per = 4.5
	}
	want := int(area / (avg * avg * per))
	if want < 2 {
		want = 2
	}
	roundPct := map[string]int{"yes": 25, "many": 60}[polyOpt]
	symmetric := layoutOpt == "symmetric"

	rooms := []roomBox{}
	next := 1
	for try := 0; try < want*40 && len(rooms) < want; try++ {
		h := c.oddBetween(sz[0], sz[1])
		w := c.oddBetween(sz[0], sz[1])
		r := 1 + 2*c.r.IntN(max(1, (g.rows-h-1)/2))
		col := 1 + 2*c.r.IntN(max(1, (g.cols-w-1)/2))
		if symmetric && col+w/2 > g.cols/2 {
			continue // the left half is placed, the right half mirrors it
		}
		if !g.fits(r, col, h, w) {
			continue
		}
		shape := ""
		if h >= 5 && w >= 5 && c.chance(roundPct) {
			shape = c.weighted([]string{"ellipse", "octagon", "cross"}, []int{4, 4, 2})
		}
		b := roomBox{id: next, r: r, c: col, h: h, w: w, shape: shape}
		g.carveRoom(b)
		rooms = append(rooms, b)
		next++
		if symmetric {
			mc := g.cols - col - w
			if mc != col && g.fits(r, mc, h, w) {
				m := roomBox{id: next, r: r, c: mc, h: h, w: w, shape: shape}
				g.carveRoom(m)
				rooms = append(rooms, m)
				next++
			}
		}
	}
	return rooms
}

// ---- the maze --------------------------------------------------------------

func (g *grid) mazeCell(r, c int) bool {
	if !g.in(r, c) || r%2 == 0 || c%2 == 0 {
		return false
	}
	i := g.at(r, c)
	return !g.blocked[i] && !g.reserved[i] && g.cell[i] == rock
}

// maze fills the odd cells left over, one region per run. straightness is
// how often a passage keeps going the way it was going.
func (c *ctx) maze(g *grid, corridor string, regionStart int) int {
	straight := map[string]int{"labyrinth": 0, "errant": 50, "straight": 90}[corridor]
	region := regionStart
	for r := 1; r < g.rows; r += 2 {
		for col := 1; col < g.cols; col += 2 {
			if !g.mazeCell(r, col) {
				continue
			}
			region++
			type step struct{ r, c, d int }
			stack := []step{{r, col, -1}}
			i := g.at(r, col)
			g.cell[i] = floor
			g.region[i] = region
			for len(stack) > 0 {
				cur := stack[len(stack)-1]
				var can []int
				for d, v := range dirs4 {
					if g.mazeCell(cur.r+2*v[0], cur.c+2*v[1]) && !g.blocked[g.at(cur.r+v[0], cur.c+v[1])] {
						can = append(can, d)
					}
				}
				if len(can) == 0 {
					stack = stack[:len(stack)-1]
					continue
				}
				d := can[c.r.IntN(len(can))]
				for _, x := range can {
					if x == cur.d && c.chance(straight) {
						d = x
					}
				}
				v := dirs4[d]
				for k := 1; k <= 2; k++ {
					j := g.at(cur.r+k*v[0], cur.c+k*v[1])
					g.cell[j] = floor
					g.region[j] = region
				}
				stack = append(stack, step{cur.r + 2*v[0], cur.c + 2*v[1], d})
			}
		}
	}
	return region
}

// ---- joining ---------------------------------------------------------------

type unionFind map[int]int

func (u unionFind) find(x int) int {
	for u[x] != 0 && u[x] != x {
		x = u[x]
	}
	return x
}
func (u unionFind) join(a, b int) { u[u.find(a)] = u.find(b) }

var doorWeights = map[string][]int{
	// arch, door, locked, trapped, secret, portcullis
	"none":      {1, 0, 0, 0, 0, 0},
	"basic":     {60, 40, 0, 0, 0, 0},
	"secure":    {20, 40, 30, 0, 0, 10},
	"standard":  {30, 40, 15, 5, 5, 5},
	"deathtrap": {10, 25, 25, 20, 15, 5},
}
var doorKinds = []string{"arch", "door", "locked", "trapped", "secret", "portcullis"}

func (c *ctx) doorKind(set string) string {
	w, ok := doorWeights[set]
	if !ok {
		w = doorWeights["standard"]
	}
	return c.weighted(doorKinds, w)
}

// connect joins every piece through single wall cells: a spanning tree over
// the pieces, then a few more for loops. A join that touches a room is a door.
func (c *ctx) connect(g *grid, doorSet string) {
	type conn struct{ i, a, b int }
	var conns []conn
	for r := 1; r < g.rows-1; r++ {
		for col := 1; col < g.cols-1; col++ {
			i := g.at(r, col)
			if g.cell[i] != rock {
				continue
			}
			for _, pair := range [][2][2]int{{{-1, 0}, {1, 0}}, {{0, -1}, {0, 1}}} {
				a := g.at(r+pair[0][0], col+pair[0][1])
				b := g.at(r+pair[1][0], col+pair[1][1])
				if g.open(a) && g.open(b) && g.region[a] != g.region[b] {
					conns = append(conns, conn{i, g.region[a], g.region[b]})
				}
			}
		}
	}
	c.r.Shuffle(len(conns), func(x, y int) { conns[x], conns[y] = conns[y], conns[x] })
	uf := unionFind{}
	opened := map[int]bool{}
	near := func(i int) bool {
		r, col := i/g.cols, i%g.cols
		for dy := -2; dy <= 2; dy++ {
			for dx := -2; dx <= 2; dx++ {
				if g.in(r+dy, col+dx) && opened[g.at(r+dy, col+dx)] {
					return true
				}
			}
		}
		return false
	}
	openIt := func(cn conn) {
		opened[cn.i] = true
		g.cell[cn.i] = floor
		g.region[cn.i] = cn.a
		g.keep[cn.i] = true
		r, col := cn.i/g.cols, cn.i%g.cols
		touchesRoom := false
		for _, d := range dirs4 {
			if g.room[g.at(r+d[0], col+d[1])] != 0 {
				touchesRoom = true
			}
		}
		if touchesRoom {
			g.door[cn.i] = c.doorKind(doorSet)
		}
	}
	for _, cn := range conns {
		if uf.find(cn.a) != uf.find(cn.b) {
			uf.join(cn.a, cn.b)
			openIt(cn)
		}
	}
	for _, cn := range conns {
		if !opened[cn.i] && !near(cn.i) && c.chance(6) {
			openIt(cn)
		}
	}
}

// ---- dead ends, stairs, ways out -------------------------------------------

func (g *grid) deadEnd(i int) bool {
	if g.cell[i] == rock || g.room[i] != 0 || g.keep[i] || g.door[i] != "" || g.stairs[i] != "" {
		return false
	}
	return g.floorNeighbours(i/g.cols, i%g.cols) <= 1
}

func (c *ctx) trim(g *grid, how string) {
	pct := map[string]int{"none": 0, "some": 50, "all": 100}[how]
	if pct == 0 {
		return
	}
	for i := range g.cell {
		if !g.deadEnd(i) || !c.chance(pct) {
			continue
		}
		j := i
		for j >= 0 && g.deadEnd(j) {
			g.cell[j] = rock
			r, col := j/g.cols, j%g.cols
			j = -1
			for _, d := range dirs4 {
				k := g.at(r+d[0], col+d[1])
				if g.in(r+d[0], col+d[1]) && g.open(k) {
					j = k
				}
			}
		}
	}
}

// stairsAt puts stairs at the ends of passages, where they are found, else
// anywhere on a corridor, else in a room.
func (c *ctx) placeStairs(g *grid, how string) {
	n := map[string]int{"yes": 2, "many": c.between(3, 5)}[how]
	if n == 0 {
		return
	}
	var ends, corr, rooms []int
	for i := range g.cell {
		if g.cell[i] != floor || g.door[i] != "" || g.keep[i] {
			continue
		}
		switch {
		case g.room[i] != 0:
			rooms = append(rooms, i)
		case g.floorNeighbours(i/g.cols, i%g.cols) == 1:
			ends = append(ends, i)
		default:
			corr = append(corr, i)
		}
	}
	for k := 0; k < n; k++ {
		var from *[]int
		switch {
		case len(ends) > 0:
			from = &ends
		case len(corr) > 0:
			from = &corr
		case len(rooms) > 0:
			from = &rooms
		default:
			return
		}
		x := c.r.IntN(len(*from))
		i := (*from)[x]
		*from = append((*from)[:x], (*from)[x+1:]...)
		dir := "down"
		if k%2 == 1 {
			dir = "up"
		}
		g.stairs[i] = dir
		g.keep[i] = true
	}
}

// egress cuts passages out to the edge of the map.
func (c *ctx) egress(g *grid, how string) {
	n := map[string]int{"yes": c.between(1, 2), "many": c.between(3, 5), "tiling": 4}[how]
	for k := 0; k < n; k++ {
		side := c.r.IntN(4)
		if how == "tiling" {
			side = k
		}
		for try := 0; try < 30; try++ {
			var r, col int
			pos := 1 + 2*c.r.IntN(max(1, ((side%2)*g.rows+(1-side%2)*g.cols-2)/2))
			if how == "tiling" {
				pos = odd(((side%2)*g.rows + (1-side%2)*g.cols) / 2)
				if pos >= ((side%2)*g.rows+(1-side%2)*g.cols)-1 {
					pos -= 2
				}
			}
			d := dirs4[(side+2)%4] // inward
			switch side {
			case 0:
				r, col = 0, pos
			case 1:
				r, col = pos, g.cols-1
			case 2:
				r, col = g.rows-1, pos
			default:
				r, col = pos, 0
			}
			path := []int{}
			hit := -1
			for y, x := r, col; g.in(y, x); y, x = y+d[0], x+d[1] {
				i := g.at(y, x)
				if g.open(i) {
					hit = i
					break
				}
				path = append(path, i)
			}
			if hit < 0 || len(path) > max(g.rows, g.cols)/2 {
				continue
			}
			for _, i := range path {
				g.cell[i] = floor
				g.keep[i] = true
			}
			if g.room[hit] != 0 && len(path) > 0 {
				g.door[path[len(path)-1]] = "arch"
			}
			break
		}
	}
}

// ---- caverns ---------------------------------------------------------------

func (c *ctx) cavern(g *grid) []roomBox {
	usable := 0
	for i := range g.blocked {
		if !g.blocked[i] {
			usable++
		}
	}
	// Start sparse and open up until the biggest cave is a fair share of the
	// map: too open and it is one hall with pillars, too closed and it is a
	// few pockets. Winding passages live between the two.
	var best []bool
	bestSize := 0
	for pct := 44; pct <= 60 && bestSize*100 < usable*38; pct += 2 {
		piece, size := g.biggestPiece(c.caveField(g, pct))
		if size > bestSize {
			best, bestSize = piece, size
		}
	}
	cells := []int{}
	for i := range best {
		if best[i] {
			g.cell[i] = floor
			cells = append(cells, i)
		}
	}
	if len(cells) == 0 {
		return nil
	}
	// Areas: seeds spread out by farthest-point picking, then every cell
	// goes to the seed it is fewest steps from.
	k := max(3, len(cells)/45)
	seeds := []int{cells[c.r.IntN(len(cells))]}
	dist := g.bfsFrom(seeds)
	for len(seeds) < k {
		far, fd := -1, -1
		for _, i := range cells {
			if dist[i] > fd {
				far, fd = i, dist[i]
			}
		}
		if fd < 4 {
			break
		}
		seeds = append(seeds, far)
		nd := g.bfsFrom([]int{far})
		for i := range dist {
			if nd[i] < dist[i] {
				dist[i] = nd[i]
			}
		}
	}
	owner := g.bfsOwner(seeds)
	rooms := make([]roomBox, len(seeds))
	for s := range seeds {
		rooms[s] = roomBox{id: s + 1, r: g.rows, c: g.cols}
	}
	for _, i := range cells {
		s := owner[i]
		if s < 0 {
			continue
		}
		g.room[i] = s + 1
		r, col := i/g.cols, i%g.cols
		b := &rooms[s]
		if r < b.r {
			b.h += b.r - r
			b.r = r
		}
		if col < b.c {
			b.w += b.c - col
			b.c = col
		}
		if r >= b.r+b.h {
			b.h = r - b.r + 1
		}
		if col >= b.c+b.w {
			b.w = col - b.c + 1
		}
	}
	return rooms
}

func (g *grid) bfsFrom(from []int) []int {
	dist := make([]int, len(g.cell))
	for i := range dist {
		dist[i] = math.MaxInt32
	}
	q := []int{}
	for _, f := range from {
		dist[f] = 0
		q = append(q, f)
	}
	for len(q) > 0 {
		j := q[0]
		q = q[1:]
		for _, d := range dirs4 {
			rr, cc := j/g.cols+d[0], j%g.cols+d[1]
			if !g.in(rr, cc) {
				continue
			}
			k := g.at(rr, cc)
			if g.open(k) && dist[k] > dist[j]+1 {
				dist[k] = dist[j] + 1
				q = append(q, k)
			}
		}
	}
	return dist
}

func (g *grid) bfsOwner(seeds []int) []int {
	owner := make([]int, len(g.cell))
	for i := range owner {
		owner[i] = -1
	}
	q := []int{}
	for s, f := range seeds {
		owner[f] = s
		q = append(q, f)
	}
	for len(q) > 0 {
		j := q[0]
		q = q[1:]
		for _, d := range dirs4 {
			rr, cc := j/g.cols+d[0], j%g.cols+d[1]
			if !g.in(rr, cc) {
				continue
			}
			k := g.at(rr, cc)
			if g.open(k) && owner[k] < 0 {
				owner[k] = owner[j]
				q = append(q, k)
			}
		}
	}
	return owner
}

// caveField is a cellular automaton's cave: pct of cells open to start, then
// a few rounds of "open where most around it is open".
func (c *ctx) caveField(g *grid, pct int) []bool {
	n := len(g.cell)
	open := make([]bool, n)
	for i := range open {
		open[i] = !g.blocked[i] && c.chance(pct)
	}
	for it := 0; it < 5; it++ {
		next := make([]bool, n)
		for r := 1; r < g.rows-1; r++ {
			for col := 1; col < g.cols-1; col++ {
				i := g.at(r, col)
				if g.blocked[i] {
					continue
				}
				walls := 0
				for dy := -1; dy <= 1; dy++ {
					for dx := -1; dx <= 1; dx++ {
						if !open[g.at(r+dy, col+dx)] {
							walls++
						}
					}
				}
				// Two rules, as the classic cave automaton has: walls grow
				// where most around is wall, and open ground far from any
				// wall gets a pillar early on, so halls break into passages.
				far := 0
				if it < 3 {
					for dy := -2; dy <= 2; dy++ {
						for dx := -2; dx <= 2; dx++ {
							rr, cc := r+dy, col+dx
							if g.in(rr, cc) && !open[g.at(rr, cc)] {
								far++
							}
						}
					}
					next[i] = walls < 5 && far > 2
				} else {
					next[i] = walls < 5
				}
			}
		}
		open = next
	}
	return open
}

// biggestPiece is the largest connected run of open cells, and its size.
func (g *grid) biggestPiece(open []bool) ([]bool, int) {
	n := len(open)
	seen := make([]int, n)
	best, bestSize, label := 0, 0, 0
	for i := range open {
		if !open[i] || seen[i] != 0 {
			continue
		}
		label++
		size := 0
		q := []int{i}
		seen[i] = label
		for len(q) > 0 {
			j := q[0]
			q = q[1:]
			size++
			for _, d := range dirs4 {
				rr, cc := j/g.cols+d[0], j%g.cols+d[1]
				k := g.at(rr, cc)
				if g.in(rr, cc) && open[k] && seen[k] == 0 {
					seen[k] = label
					q = append(q, k)
				}
			}
		}
		if size > bestSize {
			best, bestSize = label, size
		}
	}
	out := make([]bool, n)
	for i := range seen {
		out[i] = best != 0 && seen[i] == best
	}
	return out, bestSize
}

// ---- the whole shape -------------------------------------------------------

// build makes the shape of a dungeon from its options and answers it with its
// rooms renumbered from the way in, the way a key is read.
func (c *ctx) build(o map[string]string) (*grid, []roomBox) {
	layout := o["layout"]
	rows, cols := dungeonDims(o["size"], layout)
	g := newGrid(rows, cols)
	g.outline(layout)

	var rooms []roomBox
	if layout == "cavernous" {
		rooms = c.cavern(g)
		c.placeStairs(g, o["stairs"])
		c.egress(g, o["egress"])
	} else {
		rooms = c.placeRooms(g, o["roomSize"], o["roomLayout"], o["polymorph"])
		for i := range g.cell {
			if g.room[i] != 0 {
				g.region[i] = -g.room[i] // rooms are their own regions
			}
		}
		c.maze(g, o["corridors"], 0)
		c.connect(g, o["doors"])
		c.placeStairs(g, o["stairs"])
		c.egress(g, o["egress"])
		c.trim(g, o["deadends"])
	}
	rooms = g.prune(rooms)
	return g, c.renumber(g, rooms)
}

// prune drops whatever cannot be walked to from the piece holding the most
// room floor - a tower the outline cut off, a scrap of cave - and the rooms
// that went with it.
func (g *grid) prune(rooms []roomBox) []roomBox {
	n := len(g.cell)
	comp := make([]int, n)
	best, bestScore, label := 0, -1, 0
	for i := range g.cell {
		if !g.open(i) || comp[i] != 0 {
			continue
		}
		label++
		score := 0
		q := []int{i}
		comp[i] = label
		for len(q) > 0 {
			j := q[0]
			q = q[1:]
			score++
			if g.room[j] != 0 {
				score += 4
			}
			for _, d := range dirs4 {
				rr, cc := j/g.cols+d[0], j%g.cols+d[1]
				if !g.in(rr, cc) {
					continue
				}
				k := g.at(rr, cc)
				if g.open(k) && comp[k] == 0 {
					comp[k] = label
					q = append(q, k)
				}
			}
		}
		if score > bestScore {
			best, bestScore = label, score
		}
	}
	alive := map[int]bool{}
	for i := range g.cell {
		if g.open(i) && comp[i] != best {
			g.cell[i] = rock
			g.room[i] = 0
			delete(g.door, i)
			delete(g.stairs, i)
		}
		if g.room[i] != 0 {
			alive[g.room[i]] = true
		}
	}
	out := rooms[:0]
	for _, b := range rooms {
		if alive[b.id] {
			out = append(out, b)
		}
	}
	return out
}

// renumber orders rooms by how far they are from the way in - an exit at the
// edge, else up stairs, else the first room - so room 1 is where the party
// arrives and the numbers walk inward.
func (c *ctx) renumber(g *grid, rooms []roomBox) []roomBox {
	if len(rooms) == 0 {
		return rooms
	}
	var start []int
	for r := 0; r < g.rows; r++ {
		for col := 0; col < g.cols; col++ {
			i := g.at(r, col)
			if g.open(i) && (r == 0 || col == 0 || r == g.rows-1 || col == g.cols-1) {
				start = append(start, i)
			}
		}
	}
	if len(start) == 0 {
		for i, s := range g.stairs {
			if s == "up" {
				start = append(start, i)
			}
		}
	}
	if len(start) == 0 {
		b := rooms[0]
		start = []int{g.at(b.r+b.h/2, b.c+b.w/2)}
	}
	sort.Ints(start)
	dist := g.bfsFrom(start)
	near := map[int]int{}
	for i, id := range g.room {
		if id == 0 {
			continue
		}
		if d, ok := near[id]; !ok || dist[i] < d {
			near[id] = dist[i]
		}
	}
	sort.SliceStable(rooms, func(a, b int) bool { return near[rooms[a].id] < near[rooms[b].id] })
	newId := map[int]int{}
	for k := range rooms {
		newId[rooms[k].id] = k + 1
		rooms[k].id = k + 1
	}
	for i, id := range g.room {
		if id != 0 {
			g.room[i] = newId[id]
		}
	}
	return rooms
}
