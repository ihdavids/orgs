package mapgen

import (
	"container/heap"
	"math"
	"math/rand/v2"
	"sort"
	"strings"
)

// Settling a generated map: towns and cities put where people would live -
// flat, watered land by rivers and coasts, not up mountains or in bogs - as
// many as a density asks for, and roads between them that go the way a road
// would: across open land and through forest rather than over mountains or
// through swamp, round lakes, over rivers only where they must (a bridge
// costs), and joining one another so a network grows rather than a tangle of
// parallel lines.
//
// The ground is read off the map's own terrain (Generate for a region,
// GenerateHex for a hex world), so the places and roads agree with the
// picture. Everything comes back in fractions of the picture, as worg keeps
// a map's overlays (randgen.World). Deterministic per seed.

type SettlePlace struct {
	Kind string  `json:"kind"`
	Name string  `json:"name"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
}

type SettleRoad struct {
	Kind string    `json:"kind"` // highway, road, trail
	Pts  []float64 `json:"pts"`  // x0, y0, x1, y1, ... in fractions
}

type SettleOptions struct {
	// 0 none .. 1 quite a few.
	Density float64 `json:"density"`
	Roads   bool    `json:"roads"`
	Seed    uint64  `json:"seed"`
	// Places already on the map (put there by hand) that roads should reach
	// too, and that new places keep their distance from.
	Keep []SettlePlace `json:"keep,omitempty"`
}

type Settlement struct {
	Places []SettlePlace `json:"places"`
	Roads  []SettleRoad  `json:"roads"`
}

// ground is the map as a graph of squares (a region) or hexes (a hex world):
// where each is, whether it is water, what it costs to cross, and how good a
// place it is to live.
type ground struct {
	n     int
	x, y  []float64 // fractions of the picture
	water []bool
	cost  []float64
	good  []float64
	coast []bool
	near  func(i int) []int
	// aspect is the picture's width over its height, so distances are true.
	aspect float64
}

func (g *ground) dist(a, b int) float64 {
	return math.Hypot((g.x[a]-g.x[b])*g.aspect, g.y[a]-g.y[b])
}

// regionGround reads a terrain (terrain and shores maps).
func regionGround(p Params, strokes []Stroke) *ground {
	t := Generate(p, strokes)
	cw, ch, cell := t.CW, t.CH, float64(t.Cell)
	N := cw * ch
	g := &ground{n: N, x: make([]float64, N), y: make([]float64, N), water: make([]bool, N),
		cost: make([]float64, N), good: make([]float64, N), coast: make([]bool, N), aspect: float64(t.W) / float64(t.H)}
	river := make([]float64, N)
	for _, r := range t.Rivers {
		for k, pt := range r.Pts {
			cx, cy := int(pt[0]/cell), int(pt[1]/cell)
			if cx >= 0 && cy >= 0 && cx < cw && cy < ch {
				river[cy*cw+cx] = math.Max(river[cy*cw+cx], r.Width[min(k, len(r.Width)-1)])
			}
		}
	}
	elev := make([]float64, N)
	for i := 0; i < N; i++ {
		px, py := (float64(i%cw)+0.5)*cell, (float64(i/cw)+0.5)*cell
		g.x[i], g.y[i] = px/float64(t.W), py/float64(t.H)
		elev[i] = t.sample(px, py)
		g.water[i] = elev[i] < 0 || t.Lake[i] > 0.002
	}
	at := func(c, r int) int { return min(max(r, 0), ch-1)*cw + min(max(c, 0), cw-1) }
	for i := 0; i < N; i++ {
		if g.water[i] {
			g.cost[i] = math.Inf(1)
			continue
		}
		c, r := i%cw, i/cw
		e, m := elev[i], float64(t.Moist[i])
		slope := math.Hypot(elev[at(c+1, r)]-elev[at(c-1, r)], elev[at(c, r+1)]-elev[at(c, r-1)]) * 20
		cost := 1.0
		b := Biome(e, m)
		switch {
		case strings.Contains(b, "forest") || b == "taiga":
			cost = 1.45 // a road goes through a wood well enough
		case strings.Contains(b, "desert"):
			cost = 1.8
		case b == "snow":
			cost = 7
		case b == "tundra" || b == "bare" || b == "scorched":
			cost = 2.6
		}
		swamp := e < 0.13 && m > 0.72
		if swamp {
			cost += 6
		}
		if e > 0.55 {
			cost += 3 + (e-0.55)*30 // round the mountains, or through a pass
		}
		cost += slope * slope * 3
		if river[i] > 0 {
			cost += 4 + river[i]*0.8 // a bridge, where one is needed
		}
		g.cost[i] = cost
		for _, j := range []int{at(c+1, r), at(c-1, r), at(c, r+1), at(c, r-1)} {
			if g.water[j] && elev[j] < 0 {
				g.coast[i] = true
			}
		}
		// Where people live: low, flat, watered, mild.
		good := 1.0 - smoothstep(0.25, 0.6, e)*0.9 - math.Min(1, slope)*0.6
		good -= math.Abs(m-0.55) * 0.6
		if swamp || b == "snow" || strings.Contains(b, "desert") {
			good -= 0.7
		}
		g.good[i] = good
	}
	// Near water is better: a river bank most of all, then a coast.
	for i := 0; i < N; i++ {
		if g.water[i] {
			continue
		}
		c, r := i%cw, i/cw
		best := 0.0
		for dr := -2; dr <= 2; dr++ {
			for dc := -2; dc <= 2; dc++ {
				j := at(c+dc, r+dr)
				if river[j] > 0 {
					best = math.Max(best, 0.55)
				}
				if g.water[j] && elev[j] < 0 {
					best = math.Max(best, 0.4)
				}
				if t.Lake[j] > 0.002 {
					best = math.Max(best, 0.35)
				}
			}
		}
		g.good[i] += best
	}
	g.near = func(i int) []int {
		c, r := i%cw, i/cw
		out := make([]int, 0, 8)
		for dr := -1; dr <= 1; dr++ {
			for dc := -1; dc <= 1; dc++ {
				if (dr != 0 || dc != 0) && c+dc >= 0 && r+dr >= 0 && c+dc < cw && r+dr < ch {
					out = append(out, (r+dr)*cw+c+dc)
				}
			}
		}
		return out
	}
	return g
}

// hexGround reads a hex world: a hex is a node, its neighbours (not across
// the date line, which a road on a flat picture cannot cross) its edges.
func hexGround(p Params) *ground {
	h := GenerateHex(p)
	N := h.Cols * h.Rows
	iw, ih := HexImageSize(h.Cols, h.Rows, h.W)
	g := &ground{n: N, x: make([]float64, N), y: make([]float64, N), water: make([]bool, N),
		cost: make([]float64, N), good: make([]float64, N), coast: make([]bool, N), aspect: float64(iw) / float64(ih)}
	for i := 0; i < N; i++ {
		cx, cy := HexCenter(i%h.Cols, i/h.Cols, h.W)
		g.x[i], g.y[i] = cx/float64(iw), cy/float64(ih)
		b := h.Biome[i]
		g.water[i] = b == "ocean" || b == "lake" || b == "sea ice"
	}
	for i := 0; i < N; i++ {
		if g.water[i] {
			g.cost[i] = math.Inf(1)
			continue
		}
		b := h.Biome[i]
		cost, good := 1.0, 1.0
		switch {
		case strings.Contains(b, "forest") || b == "taiga":
			cost, good = 1.45, 0.8
		case strings.Contains(b, "desert"):
			cost, good = 1.8, 0.2
		case b == "mountain":
			cost, good = 7, 0.05
		case b == "snow" || b == "ice":
			cost, good = 9, 0
		case b == "tundra":
			cost, good = 2.6, 0.2
		case b == "savanna" || b == "shrubland":
			cost, good = 1.2, 0.75
		}
		if h.River[i] > 0 {
			cost += 3
			good += 0.5
		}
		for _, j := range h.neighbours(i) {
			if g.water[j] {
				g.coast[i] = true
				good += 0.08
			}
		}
		g.cost[i], g.good[i] = cost, good
	}
	g.near = func(i int) []int {
		out := []int{}
		for _, j := range h.neighbours(i) {
			if d := j%h.Cols - i%h.Cols; d > 1 || d < -1 {
				continue
			}
			out = append(out, j)
		}
		return out
	}
	return g
}

func groundOf(p Params, strokes []Stroke) *ground {
	switch p.Normal().Generator {
	case "hex":
		return hexGround(p)
	case "terrain", "shores":
		return regionGround(p, strokes)
	}
	return nil
}

// Settle places towns and routes roads on a generated region or world.
// Towns and villages (city and village maps) have no land to settle.
func Settle(p Params, strokes []Stroke, o SettleOptions) Settlement {
	out := Settlement{Places: []SettlePlace{}, Roads: []SettleRoad{}}
	g := groundOf(p, strokes)
	if g == nil {
		return out
	}
	seed := o.Seed
	if seed == 0 {
		seed = p.Seed
	}
	r := rand.New(rand.NewPCG(seed, seed^0x5e771e))
	density := clamp(o.Density, 0, 1)

	// Where the kept places stand.
	nodeAt := func(x, y float64) int {
		best, bd := -1, math.Inf(1)
		for i := 0; i < g.n; i++ {
			if g.water[i] {
				continue
			}
			d := math.Hypot((g.x[i]-x)*g.aspect, g.y[i]-y)
			if d < bd {
				best, bd = i, d
			}
		}
		return best
	}
	type site struct {
		node  int
		place SettlePlace
		kept  bool
		score float64
	}
	var sites []site
	for _, k := range o.Keep {
		if n := nodeAt(k.X, k.Y); n >= 0 {
			sites = append(sites, site{node: n, place: k, kept: true, score: 9})
		}
	}

	want := int(math.Round(math.Pow(density, 1.25) * 34))
	if want > 0 {
		// Far enough apart that a map of many is still a map of places, not
		// a rash: fewer places, more room between them.
		sep := 0.05 + (1-density)*0.12
		cand := make([]int, 0, g.n)
		jit := make([]float64, g.n)
		for i := 0; i < g.n; i++ {
			if !g.water[i] && g.good[i] > 0.15 {
				cand = append(cand, i)
				jit[i] = g.good[i] + r.Float64()*0.45
			}
		}
		sort.Slice(cand, func(a, b int) bool { return jit[cand[a]] > jit[cand[b]] })
		var made []site
		for _, i := range cand {
			if len(made) >= want {
				break
			}
			ok := true
			for _, s := range sites {
				if g.dist(i, s.node) < sep {
					ok = false
					break
				}
			}
			if !ok {
				continue
			}
			s := site{node: i, score: g.good[i], place: SettlePlace{X: g.x[i], Y: g.y[i]}}
			sites = append(sites, s)
			made = append(made, s)
		}
		// The best placed grow biggest: a city or two, some towns, the rest
		// villages; a town on the coast is a port.
		sort.Slice(made, func(a, b int) bool { return made[a].score > made[b].score })
		// No two places of one name, nor one of a place already there.
		used := map[string]bool{}
		for _, k := range o.Keep {
			used[strings.ToLower(k.Name)] = true
		}
		cities := max(1, len(made)/8)
		towns := max(1, len(made)*3/8)
		for k := range made {
			kind := "village"
			switch {
			case k < cities:
				kind = "city"
			case k < cities+towns:
				kind = "town"
				if g.coast[made[k].node] && r.IntN(2) == 0 {
					kind = "port"
				}
			}
			made[k].place.Kind = kind
			name := ""
			for try := uint64(0); try < 12; try++ {
				name = PlaceName(seed*977+uint64(k)*131+uint64(made[k].node)+try*7919, kind)
				if !used[strings.ToLower(name)] {
					break
				}
			}
			used[strings.ToLower(name)] = true
			made[k].place.Name = name
			out.Places = append(out.Places, made[k].place)
		}
		// The sites list holds them too, with their kinds, for the roads.
		for k := range sites {
			for _, m := range made {
				if sites[k].node == m.node && !sites[k].kept {
					sites[k].place = m.place
				}
			}
		}
	}
	if !o.Roads || len(sites) < 2 {
		return out
	}

	// Which places a road joins: the relative neighbourhood graph - a pair
	// is joined unless some third place is nearer to both - which joins
	// neighbours, never makes a long way round a short one, and leaves no
	// place alone. The longest pairs of all go unbuilt.
	type pair struct {
		a, b int
		d    float64
	}
	var pairs []pair
	for a := range sites {
		for b := a + 1; b < len(sites); b++ {
			d := g.dist(sites[a].node, sites[b].node)
			if d > 0.5 {
				continue
			}
			ok := true
			for c := range sites {
				if c == a || c == b {
					continue
				}
				if math.Max(g.dist(sites[a].node, sites[c].node), g.dist(sites[b].node, sites[c].node)) < d {
					ok = false
					break
				}
			}
			if ok {
				pairs = append(pairs, pair{a, b, d})
			}
		}
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].d < pairs[j].d })
	onRoad := make([]bool, g.n)
	rank := map[string]int{"city": 3, "port": 2, "town": 2, "castle": 2, "village": 1}
	for _, pr := range pairs {
		path := g.route(sites[pr.a].node, sites[pr.b].node, onRoad)
		if len(path) < 2 {
			continue
		}
		for _, i := range path {
			onRoad[i] = true
		}
		ka, kb := rank[sites[pr.a].place.Kind], rank[sites[pr.b].place.Kind]
		kind := "road"
		switch {
		case ka+kb >= 5:
			kind = "highway"
		case ka == 1 && kb == 1, ka == 0 || kb == 0:
			kind = "trail"
		}
		pts := []float64{sites[pr.a].place.X, sites[pr.a].place.Y}
		for _, q := range simplifyPath(g, path[1:len(path)-1], 1.4) {
			pts = append(pts, g.x[q], g.y[q])
		}
		pts = append(pts, sites[pr.b].place.X, sites[pr.b].place.Y)
		out.Roads = append(out.Roads, SettleRoad{Kind: kind, Pts: pts})
	}
	return out
}

// route is the cheapest way from a to b (A*), a road already built being
// much cheaper to follow, so roads share their way where they can.
func (g *ground) route(a, b int, onRoad []bool) []int {
	minCost := 0.3
	h := func(i int) float64 { return g.dist(i, b) * minCost }
	dist := map[int]float64{a: 0}
	from := map[int]int{}
	q := &routeHeap{{a, h(a)}}
	done := map[int]bool{}
	for q.Len() > 0 {
		cur := heap.Pop(q).(routeAt)
		if done[cur.i] {
			continue
		}
		done[cur.i] = true
		if cur.i == b {
			break
		}
		for _, j := range g.near(cur.i) {
			if g.water[j] || done[j] {
				continue
			}
			c := (g.cost[cur.i] + g.cost[j]) / 2
			if onRoad[j] && onRoad[cur.i] {
				c = minCost
			}
			nd := dist[cur.i] + c*g.dist(cur.i, j)
			if old, ok := dist[j]; !ok || nd < old {
				dist[j] = nd
				from[j] = cur.i
				heap.Push(q, routeAt{j, nd + h(j)})
			}
		}
	}
	if !done[b] {
		return nil
	}
	path := []int{b}
	for at := b; at != a; {
		at = from[at]
		path = append(path, at)
	}
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	return path
}

type routeAt struct {
	i int
	f float64
}
type routeHeap []routeAt

func (h routeHeap) Len() int            { return len(h) }
func (h routeHeap) Less(a, b int) bool  { return h[a].f < h[b].f }
func (h routeHeap) Swap(a, b int)       { h[a], h[b] = h[b], h[a] }
func (h *routeHeap) Push(x interface{}) { *h = append(*h, x.(routeAt)) }
func (h *routeHeap) Pop() interface{} {
	old := *h
	v := old[len(old)-1]
	*h = old[:len(old)-1]
	return v
}

// simplifyPath drops the squares a line through its neighbours passes
// within tol squares of (Ramer-Douglas-Peucker), so a road is a few smooth
// bends rather than a staircase.
func simplifyPath(g *ground, path []int, tol float64) []int {
	if len(path) < 3 {
		return path
	}
	unit := 0.0
	if len(path) > 1 {
		unit = g.dist(path[0], path[1])
	}
	lim := tol * unit
	var rdp func(lo, hi int) []int
	rdp = func(lo, hi int) []int {
		ax, ay := g.x[path[lo]]*g.aspect, g.y[path[lo]]
		bx, by := g.x[path[hi]]*g.aspect, g.y[path[hi]]
		far, at := 0.0, -1
		for k := lo + 1; k < hi; k++ {
			px, py := g.x[path[k]]*g.aspect, g.y[path[k]]
			d := math.Sqrt(segDist2(cpt{px, py}, cpt{ax, ay}, cpt{bx, by}))
			if d > far {
				far, at = d, k
			}
		}
		if at < 0 || far <= lim {
			return []int{path[lo]}
		}
		return append(rdp(lo, at), rdp(at, hi)...)
	}
	return append(rdp(0, len(path)-1), path[len(path)-1])
}
