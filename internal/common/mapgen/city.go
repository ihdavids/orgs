package mapgen

import (
	"container/heap"
	"fmt"
	"math"
	"math/rand/v2"
	"sort"
	"strings"
)

// A medieval town or city, in the manner of watabou's Medieval Fantasy City
// Generator (https://watabou.itch.io/medieval-fantasy-city-generator - the
// idea, as its author has described it; none of its code): a spiral of
// points cut into Voronoi patches, the inner ones the wards of the town and
// the rest its farmland; a wall round the wards with towers and gates; main
// streets from the gates to the market, roads out through the fields; every
// ward cut into lots for its buildings, more or fewer and bigger or smaller
// by what the ward is; and a citadel, a temple, a river or a coast if asked.
//
// It is drawn as SVG - a town is all edges, and a picture of one has to stay
// sharp however close it is looked at.

type cpt = [2]float64

// A patch: a Voronoi cell and what it is.
type patch struct {
	site  cpt
	poly  []cpt
	inner bool
	water bool
	ward  string
}

type cityEdge struct {
	a, b    string // vertex keys
	cells   []int
	main    bool // a main street
	road    bool // a road out of town
	wall    bool
	coast   bool
	pa, pb  cpt
}

// City is a town laid out: its patches, buildings and the lines between.
type City struct {
	Size      float64 // the picture's side, px
	scale     float64
	patches   []*patch
	buildings [][]cpt
	keeps     [][]cpt
	trees     []cpt
	fields    [][]cpt
	furrows   [][2]cpt
	walls     [][2]cpt
	towers    []cpt
	gates     []cpt
	castle    [][2]cpt
	mains     [][]cpt
	roads     [][]cpt
	river     []cpt
	riverW    float64
	bridges   [][2]cpt
	fountain  *cpt

	// What makes it lived in.
	cropOf    []int // the crop of each field
	hedges    [][]cpt
	farmSpots []cpt
	windmills []cpt
	granaries []cpt
	haystacks []cpt
	carts     []placed
	boats     []placed
	docks     []placed
	wheels    []placed
	stalls    []placed
	graves    []placed
	plazaPoly []cpt
	rounds    []placed // round roofs: an apse, a tower
	river2    float64 // the river's bank half-width, for things on it
}

// placed is a thing put somewhere, turned, of a size.
type placed struct {
	c    cpt
	turn float64
	size float64
	flag bool
	tag  string
}

func cityKey(p cpt) string { return fmt.Sprintf("%.4f,%.4f", p[0], p[1]) }

func polyArea(p []cpt) float64 {
	a := 0.0
	for i := range p {
		q := p[(i+1)%len(p)]
		a += p[i][0]*q[1] - q[0]*p[i][1]
	}
	return a / 2
}

func centroid(p []cpt) cpt {
	var cx, cy, a float64
	for i := range p {
		q := p[(i+1)%len(p)]
		c := p[i][0]*q[1] - q[0]*p[i][1]
		cx += (p[i][0] + q[0]) * c
		cy += (p[i][1] + q[1]) * c
		a += c
	}
	if math.Abs(a) < 1e-12 {
		var s cpt
		for _, v := range p {
			s[0] += v[0]
			s[1] += v[1]
		}
		return cpt{s[0] / float64(len(p)), s[1] / float64(len(p))}
	}
	return cpt{cx / (3 * a), cy / (3 * a)}
}

// clipHalf keeps the part of a convex polygon where (x - m)·n <= 0.
func clipHalf(poly []cpt, m, n cpt) []cpt {
	side := func(v cpt) float64 { return (v[0]-m[0])*n[0] + (v[1]-m[1])*n[1] }
	var out []cpt
	for i := range poly {
		a, b := poly[i], poly[(i+1)%len(poly)]
		sa, sb := side(a), side(b)
		if sa <= 0 {
			out = append(out, a)
		}
		if (sa < 0) != (sb < 0) && sa != sb {
			t := sa / (sa - sb)
			out = append(out, cpt{a[0] + (b[0]-a[0])*t, a[1] + (b[1]-a[1])*t})
		}
	}
	return out
}

// splitPoly cuts a convex polygon by the line through p along d.
func splitPoly(poly []cpt, p, d cpt) ([]cpt, []cpt) {
	n := cpt{-d[1], d[0]}
	return clipHalf(poly, p, n), clipHalf(poly, p, cpt{-n[0], -n[1]})
}

// insetPoly moves each edge of a convex polygon in by its own distance: the
// polygon clipped by each edge's line moved inward, so an edge too short to
// survive simply goes. Nil if nothing is left.
func insetPoly(poly []cpt, by []float64) []cpt {
	n := len(poly)
	if n < 3 {
		return nil
	}
	sign := 1.0
	if polyArea(poly) < 0 {
		sign = -1
	}
	out := poly
	for i := 0; i < n && len(out) >= 3; i++ {
		a, b := poly[i], poly[(i+1)%n]
		d := cpt{b[0] - a[0], b[1] - a[1]}
		l := math.Hypot(d[0], d[1])
		if l < 1e-9 {
			continue
		}
		in := cpt{-d[1] / l * sign, d[0] / l * sign}
		m := cpt{a[0] + in[0]*by[i], a[1] + in[1]*by[i]}
		out = clipHalf(out, m, cpt{-in[0], -in[1]})
	}
	if len(out) < 3 || math.Abs(polyArea(out)) < math.Abs(polyArea(poly))*0.03 {
		return nil
	}
	return out
}

func shrink(poly []cpt, by float64) []cpt {
	b := make([]float64, len(poly))
	for i := range b {
		b[i] = by
	}
	return insetPoly(poly, b)
}

// The wards: how big their lots, how many are left empty, how much smaller
// than its lot a building is.
type wardKind struct {
	lot, empty, gap float64
}

var wardKinds = map[string]wardKind{
	"merchant":       {0.14, 0.02, 0.035},
	"craftsmen":      {0.08, 0.03, 0.025},
	"slum":           {0.045, 0.08, 0.02},
	"administration": {0.26, 0.12, 0.06},
	"military":       {0.18, 0.1, 0.045},
	"patriciate":     {0.24, 0.1, 0.07},
	"market":         {0.12, 0.05, 0.04},
	"outskirts":      {0.07, 0.5, 0.03},
}

// GenerateCity lays out a town.
func GenerateCity(p Params) *City {
	p = p.Normal()
	r := rand.New(rand.NewPCG(p.Seed, p.Seed^0x9fb21c651e98df25))
	N := p.CitySize
	total := N * 3
	if !p.Farms {
		total = N*2 + 6
	}
	// The spiral: points spread as sunflower seeds, a little shaken.
	golden := math.Pi * (3 - math.Sqrt(5))
	pts := make([]cpt, 0, total+32)
	for i := 0; i < total; i++ {
		rad := math.Sqrt(float64(i)+0.5) * (1 + (r.Float64()-0.5)*0.25)
		a := float64(i)*golden + (r.Float64()-0.5)*0.4
		pts = append(pts, cpt{math.Cos(a) * rad, math.Sin(a) * rad})
	}
	outer := math.Sqrt(float64(total)) * 1.05
	// A ring beyond, to close the outer patches.
	for i := 0; i < 32; i++ {
		a := float64(i) / 32 * 2 * math.Pi
		pts = append(pts, cpt{math.Cos(a) * outer * 1.25, math.Sin(a) * outer * 1.25})
	}
	box := outer * 1.6
	c := &City{Size: float64(p.CityPx)}
	c.scale = c.Size / 2 / (outer * 0.97)
	for i := 0; i < total; i++ {
		poly := []cpt{{-box, -box}, {box, -box}, {box, box}, {-box, box}}
		for j, q := range pts {
			if j == i {
				continue
			}
			m := cpt{(pts[i][0] + q[0]) / 2, (pts[i][1] + q[1]) / 2}
			poly = clipHalf(poly, m, cpt{q[0] - pts[i][0], q[1] - pts[i][1]})
		}
		c.patches = append(c.patches, &patch{site: pts[i], poly: poly, inner: i < N})
	}
	innerR := math.Sqrt(float64(N)) * 1.0

	// The sea: everything beyond a ragged line on one side.
	noise := newNoise(p.Seed*7 + 3)
	if p.Seaside {
		a := r.Float64() * 2 * math.Pi
		dir := cpt{math.Cos(a), math.Sin(a)}
		off := innerR * (0.45 + r.Float64()*0.3)
		for _, pa := range c.patches {
			d := pa.site[0]*dir[0] + pa.site[1]*dir[1]
			side := -pa.site[0]*dir[1] + pa.site[1]*dir[0]
			if d > off+noise.at(side*0.35, 3.3)*innerR*0.35 {
				pa.water, pa.inner = true, false
			}
		}
	}

	// The wards, by how far from the middle.
	inner := []int{}
	for i, pa := range c.patches {
		if pa.inner {
			inner = append(inner, i)
		}
	}
	sort.Slice(inner, func(a, b int) bool {
		return math.Hypot(c.patches[inner[a]].site[0], c.patches[inner[a]].site[1]) < math.Hypot(c.patches[inner[b]].site[0], c.patches[inner[b]].site[1])
	})
	plaza := -1
	if len(inner) > 0 && p.Plaza {
		plaza = inner[0]
		c.patches[plaza].ward = "plaza"
	}
	if p.Temple && len(inner) > 2 {
		c.patches[inner[1]].ward = "temple"
	}
	if p.Citadel && len(inner) > 5 {
		c.patches[inner[len(inner)-1-r.IntN(2)]].ward = "castle"
	}
	for k, i := range inner {
		pa := c.patches[i]
		if pa.ward != "" {
			continue
		}
		f := float64(k) / float64(max(1, len(inner)-1))
		roll := r.Float64()
		switch {
		case f < 0.3 && roll < 0.35:
			pa.ward = "administration"
		case f < 0.3 && roll < 0.55:
			pa.ward = "patriciate"
		case f < 0.45:
			pa.ward = "merchant"
		case roll < 0.08:
			pa.ward = "park"
		case f > 0.75 && roll < 0.45:
			pa.ward = "slum"
		case f > 0.6 && roll < 0.6:
			pa.ward = "military"
		default:
			pa.ward = "craftsmen"
		}
	}
	if plaza < 0 && len(inner) > 0 {
		plaza = inner[0]
	}

	// The edges between patches.
	edges := map[string]*cityEdge{}
	edgeKey := func(a, b string) string {
		if a < b {
			return a + "|" + b
		}
		return b + "|" + a
	}
	verts := map[string]cpt{}
	for i, pa := range c.patches {
		for k := range pa.poly {
			a, b := pa.poly[k], pa.poly[(k+1)%len(pa.poly)]
			ka, kb := cityKey(a), cityKey(b)
			if ka == kb {
				continue
			}
			verts[ka], verts[kb] = a, b
			e := edges[edgeKey(ka, kb)]
			if e == nil {
				e = &cityEdge{a: ka, b: kb, pa: a, pb: b}
				edges[edgeKey(ka, kb)] = e
			}
			e.cells = append(e.cells, i)
		}
	}
	isInner := func(i int) bool { return c.patches[i].inner }
	adj := map[string][]*cityEdge{}
	for _, e := range edges {
		adj[e.a] = append(adj[e.a], e)
		adj[e.b] = append(adj[e.b], e)
		in, wet, out := 0, 0, 0
		for _, ci := range e.cells {
			switch {
			case c.patches[ci].water:
				wet++
			case isInner(ci):
				in++
			default:
				out++
			}
		}
		if in == 1 && out > 0 && p.Walls {
			e.wall = true
		}
		if in == 1 && wet > 0 {
			e.coast = true
		}
	}

	// The wall's corners, and the gates: as many as the town is big, as far
	// apart round the wall as they can be.
	wallV := map[string]bool{}
	for _, e := range edges {
		if e.wall {
			wallV[e.a], wallV[e.b] = true, true
		}
	}
	gateN := 2 + N/12
	var gates []string
	if p.Walls {
		cands := make([]string, 0, len(wallV))
		for k := range wallV {
			cands = append(cands, k)
		}
		sort.Strings(cands)
		ang := func(k string) float64 { v := verts[k]; return math.Atan2(v[1], v[0]) }
		if len(cands) > 0 {
			gates = append(gates, cands[r.IntN(len(cands))])
		}
		for len(gates) < gateN && len(gates) < len(cands) {
			best, bd := "", -1.0
			for _, k := range cands {
				near := math.Inf(1)
				for _, g := range gates {
					d := math.Abs(math.Remainder(ang(k)-ang(g), 2*math.Pi))
					near = math.Min(near, d)
				}
				if near > bd {
					bd, best = near, k
				}
			}
			gates = append(gates, best)
		}
	} else {
		// No wall: the streets still leave town at the edge of the wards.
		for _, i := range inner {
			for _, v := range c.patches[i].poly {
				k := cityKey(v)
				for _, e := range adj[k] {
					for _, ci := range e.cells {
						if !isInner(ci) && !c.patches[ci].water {
							wallV[k] = true
						}
					}
				}
			}
		}
		cands := make([]string, 0, len(wallV))
		for k := range wallV {
			cands = append(cands, k)
		}
		sort.Strings(cands)
		r.Shuffle(len(cands), func(a, b int) { cands[a], cands[b] = cands[b], cands[a] })
		for _, k := range cands {
			if len(gates) >= gateN {
				break
			}
			ok := true
			for _, g := range gates {
				if math.Hypot(verts[k][0]-verts[g][0], verts[k][1]-verts[g][1]) < innerR*0.8 {
					ok = false
				}
			}
			if ok {
				gates = append(gates, k)
			}
		}
	}

	// Main streets: from each gate to the plaza along the patches' edges,
	// keeping to the town (an edge on the wall costs more, one on the water
	// is no street at all).
	plazaV := map[string]bool{}
	if plaza >= 0 {
		for _, v := range c.patches[plaza].poly {
			plazaV[cityKey(v)] = true
		}
	}
	path := func(from string, ok func(e *cityEdge) (float64, bool), done func(k string) bool) []string {
		dist := map[string]float64{from: 0}
		prev := map[string]string{}
		q := &strHeap{{from, 0}}
		for q.Len() > 0 {
			cur := heap.Pop(q).(strAt)
			if cur.d > dist[cur.k] {
				continue
			}
			if done(cur.k) {
				out := []string{cur.k}
				for k := cur.k; k != from; {
					k = prev[k]
					out = append(out, k)
				}
				return out
			}
			for _, e := range adj[cur.k] {
				w, okE := ok(e)
				if !okE {
					continue
				}
				nk := e.a
				if nk == cur.k {
					nk = e.b
				}
				nd := cur.d + math.Hypot(verts[nk][0]-verts[cur.k][0], verts[nk][1]-verts[cur.k][1])*w
				if old, seen := dist[nk]; !seen || nd < old {
					dist[nk] = nd
					prev[nk] = cur.k
					heap.Push(q, strAt{nk, nd})
				}
			}
		}
		return nil
	}
	mark := func(ks []string, set func(e *cityEdge)) []cpt {
		var line []cpt
		for i := len(ks) - 1; i >= 0; i-- {
			line = append(line, verts[ks[i]])
			if i > 0 {
				set(edges[edgeKey(ks[i], ks[i-1])])
			}
		}
		return line
	}
	for _, g := range gates {
		ks := path(g, func(e *cityEdge) (float64, bool) {
			in, wet := false, false
			for _, ci := range e.cells {
				in = in || isInner(ci)
				wet = wet || c.patches[ci].water
			}
			if !in || (wet && len(e.cells) == 1) {
				return 0, false
			}
			if e.wall || e.coast {
				return 4, true
			}
			return 1, true
		}, func(k string) bool { return plazaV[k] })
		if ks != nil {
			c.mains = append(c.mains, mark(ks, func(e *cityEdge) { e.main = true }))
		}
		// And the road out, through the fields to the edge of the map.
		ro := path(g, func(e *cityEdge) (float64, bool) {
			for _, ci := range e.cells {
				if isInner(ci) || c.patches[ci].water {
					return 0, false
				}
			}
			return 1, true
		}, func(k string) bool { v := verts[k]; return math.Hypot(v[0], v[1]) > outer*0.92 })
		if ro != nil {
			line := mark(ro, func(e *cityEdge) { e.road = true })
			// On to the edge of the picture, the way it was going.
			n := len(line)
			if n >= 2 {
				a, b := line[n-2], line[n-1]
				d := cpt{b[0] - a[0], b[1] - a[1]}
				l := math.Hypot(d[0], d[1])
				end := cpt{b[0] + d[0]/l*outer, b[1] + d[1]/l*outer}
				// Not out to sea.
				wet := false
				for _, pa := range c.patches {
					if pa.water && (pointIn(end, pa.poly) || pointIn(cpt{(b[0] + end[0]) / 2, (b[1] + end[1]) / 2}, pa.poly)) {
						wet = true
					}
				}
				if !wet {
					line = append(line, end)
				}
			}
			c.roads = append(c.roads, line)
		}
	}

	// The river: in at one side, out at the other, bending.
	if p.River {
		a := r.Float64() * math.Pi
		dir := cpt{math.Cos(a), math.Sin(a)}
		side := cpt{-dir[1], dir[0]}
		for t := -1.3; t <= 1.3001; t += 0.05 {
			s := t * outer
			bend := noise.fbm(t*2.2+7, 1.7, 3) * innerR * 0.9
			c.river = append(c.river, cpt{dir[0]*s + side[0]*bend, dir[1]*s + side[1]*bend})
		}
		c.riverW = 0.45 + float64(N)*0.004
	}
	nearRiver := func(v cpt, w float64) bool {
		for i := 0; i+1 < len(c.river); i++ {
			if segDist2(v, c.river[i], c.river[i+1]) < w*w {
				return true
			}
		}
		return false
	}

	// The wards' blocks: each patch moved in from its edges by half the
	// street there, then cut into lots.
	const mainHalf, alleyHalf, wallIn = 0.16, 0.07, 0.22
	for i, pa := range c.patches {
		if pa.water {
			continue
		}
		by := make([]float64, len(pa.poly))
		for k := range pa.poly {
			e := edges[edgeKey(cityKey(pa.poly[k]), cityKey(pa.poly[(k+1)%len(pa.poly)]))]
			switch {
			case e == nil:
				by[k] = alleyHalf
			case e.main || e.road:
				by[k] = mainHalf
			case e.wall:
				by[k] = wallIn
			case e.coast:
				by[k] = 0.12
			default:
				by[k] = alleyHalf
			}
		}
		block := insetPoly(pa.poly, by)
		if block == nil {
			continue
		}
		switch {
		case !pa.inner:
			// Outside the walls: fields, a wood, or houses along a road.
			d := math.Hypot(pa.site[0], pa.site[1])
			byRoad := false
			for k := range pa.poly {
				if e := edges[edgeKey(cityKey(pa.poly[k]), cityKey(pa.poly[(k+1)%len(pa.poly)]))]; e != nil && e.road {
					byRoad = true
				}
			}
			switch {
			case byRoad && d < innerR*1.6 && r.Float64() < 0.7:
				c.lots(block, wardKinds["outskirts"], r, nearRiver)
			case !p.Farms || r.Float64() < 0.22:
				c.wood(block, r, 0.4+r.Float64()*0.4)
			default:
				c.farm(block, r)
			}
		case pa.ward == "plaza":
			cc := centroid(block)
			if !nearRiver(cc, c.riverW*0.5+0.35) {
				c.fountain = &cc
			}
			c.plazaPoly = block
			// Stalls round the fountain, turned to face it.
			for ring := 0.32; ring < 1.2; ring += 0.24 {
				n := int(ring * 14)
				for k := 0; k < n; k++ {
					t := float64(k)/float64(n)*2*math.Pi + ring
					v := cpt{cc[0] + math.Cos(t)*ring, cc[1] + math.Sin(t)*ring}
					if pointIn(v, shrink(block, 0.12)) && !nearRiver(v, c.riverW*0.5+0.12) && r.Float64() < 0.6 {
						c.stalls = append(c.stalls, placed{c: v, turn: t + math.Pi/2, size: 0.1, tag: stallColours[r.IntN(len(stallColours))]})
					}
				}
			}
		case pa.ward == "park":
			c.wood(block, r, 0.25)
		case pa.ward == "temple":
			cc := centroid(block)
			church := c.cathedral(cc, math.Sqrt(math.Abs(polyArea(block)))*0.62, longAxis(block))
			inChurch := func(v cpt) bool {
				for _, sh := range church {
					if pointIn(v, scalePoly(sh, centroid(sh), 1.35)) {
						return true
					}
				}
				return false
			}
			// The churchyard: graves on the side away from the market.
			away := cpt{cc[0], cc[1]}
			l := math.Hypot(away[0], away[1])
			if l > 0 {
				away = cpt{away[0] / l, away[1] / l}
			}
			yard := func(v cpt) bool { return (v[0]-cc[0])*away[0]+(v[1]-cc[1])*away[1] > 0.05 }
			ring := shrink(block, 0.06)
			if ring != nil {
				lo, hi := cpt{math.Inf(1), math.Inf(1)}, cpt{math.Inf(-1), math.Inf(-1)}
				for _, v := range ring {
					lo = cpt{math.Min(lo[0], v[0]), math.Min(lo[1], v[1])}
					hi = cpt{math.Max(hi[0], v[0]), math.Max(hi[1], v[1])}
				}
				inner := scalePoly(block, cc, 0.6)
				for y := lo[1] + 0.05; y < hi[1]; y += 0.085 {
					for x := lo[0] + 0.05; x < hi[0]; x += 0.07 {
						v := cpt{x + (r.Float64()-0.5)*0.02, y}
						if pointIn(v, ring) && !pointIn(v, inner) && !inChurch(v) && yard(v) && r.Float64() < 0.85 {
							c.graves = append(c.graves, placed{c: v, size: 0.035, flag: r.Float64() < 0.3})
						}
					}
				}
				for k := 0; k < 4; k++ {
					v := cpt{lo[0] + r.Float64()*(hi[0]-lo[0]), lo[1] + r.Float64()*(hi[1]-lo[1])}
					if pointIn(v, ring) && !pointIn(v, inner) && !inChurch(v) && yard(v) {
						c.trees = append(c.trees, v)
					}
				}
				c.lots(ring, wardKind{0.1, 0.3, 0.04}, r, func(v cpt, w float64) bool {
					return pointIn(v, scalePoly(block, cc, 0.62)) || inChurch(v) || yard(v) || nearRiver(v, w)
				})
			}
		case pa.ward == "castle":
			wall := shrink(block, 0.02)
			if wall != nil {
				for k := range wall {
					c.castle = append(c.castle, [2]cpt{wall[k], wall[(k+1)%len(wall)]})
					c.towers = append(c.towers, wall[k])
				}
				cc := centroid(wall)
				s := math.Sqrt(math.Abs(polyArea(wall))) * 0.22
				c.keeps = append(c.keeps, []cpt{{cc[0] - s, cc[1] - s}, {cc[0] + s, cc[1] - s}, {cc[0] + s, cc[1] + s}, {cc[0] - s, cc[1] + s}})
				c.lots(shrink(wall, 0.25), wardKind{0.12, 0.45, 0.05}, r, func(v cpt, w float64) bool {
					return math.Abs(v[0]-cc[0]) < s*1.6 && math.Abs(v[1]-cc[1]) < s*1.6
				})
			}
		default:
			wk, ok := wardKinds[pa.ward]
			if !ok {
				wk = wardKinds["craftsmen"]
			}
			c.lots(block, wk, r, nearRiver)
		}
		_ = i
	}

	// The wall, its towers and gates.
	gateSet := map[string]bool{}
	for _, g := range gates {
		gateSet[g] = true
		if p.Walls {
			c.gates = append(c.gates, verts[g])
		}
	}
	keys := make([]string, 0, len(edges))
	for k := range edges {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		e := edges[k]
		if !e.wall {
			continue
		}
		a, b := e.pa, e.pb
		// A gate is a gap in the wall.
		if gateSet[e.a] {
			a = towardBy(a, b, 0.32)
		}
		if gateSet[e.b] {
			b = towardBy(b, a, 0.32)
		}
		// Where the river goes through the wall, the wall stops either
		// side of it, a tower on each bank.
		cut := false
		for i := 0; i+1 < len(c.river) && !cut; i++ {
			if hit, ok := segCross(a, b, c.river[i], c.river[i+1]); ok {
				gap := c.riverW*0.5 + 0.12
				a1, b1 := towardBy(hit, a, gap), towardBy(hit, b, gap)
				if math.Hypot(a1[0]-hit[0], a1[1]-hit[1]) < math.Hypot(a[0]-hit[0], a[1]-hit[1]) {
					c.walls = append(c.walls, [2]cpt{a, a1})
					c.towers = append(c.towers, a1)
				}
				if math.Hypot(b1[0]-hit[0], b1[1]-hit[1]) < math.Hypot(b[0]-hit[0], b[1]-hit[1]) {
					c.walls = append(c.walls, [2]cpt{b1, b})
					c.towers = append(c.towers, b1)
				}
				cut = true
			}
		}
		if !cut {
			c.walls = append(c.walls, [2]cpt{a, b})
		}
	}
	for k := range wallV {
		if p.Walls && !gateSet[k] {
			c.towers = append(c.towers, verts[k])
		}
	}
	sort.Slice(c.towers, func(a, b int) bool { return c.towers[a][0] < c.towers[b][0] })

	// Bridges, where a main street or a road crosses the river, and where
	// a street in town does, not too close to another.
	if p.River {
		for _, k := range keys {
			e := edges[k]
			inTown := false
			for _, ci := range e.cells {
				inTown = inTown || c.patches[ci].inner
			}
			if !(e.main || e.road || (inTown && !e.wall)) {
				continue
			}
			for i := 0; i+1 < len(c.river); i++ {
				if hit, ok := segCross(e.pa, e.pb, c.river[i], c.river[i+1]); ok {
					d := cpt{c.river[i+1][0] - c.river[i][0], c.river[i+1][1] - c.river[i][1]}
					l := math.Hypot(d[0], d[1])
					n := cpt{-d[1] / l * (c.riverW*0.5 + 0.2), d[0] / l * (c.riverW*0.5 + 0.2)}
					near := false
					for _, br := range c.bridges {
						mid := cpt{(br[0][0] + br[1][0]) / 2, (br[0][1] + br[1][1]) / 2}
						if math.Hypot(mid[0]-hit[0], mid[1]-hit[1]) < 1.1 {
							near = true
						}
					}
					if !near || e.main || e.road {
						c.bridges = append(c.bridges, [2]cpt{{hit[0] - n[0], hit[1] - n[1]}, {hit[0] + n[0], hit[1] + n[1]}})
					}
				}
			}
		}
	}
	c.decorate(p, r, innerR, nearRiver, edges, keys)
	return c
}

var stallColours = []string{"#b5473a", "#3f6f9f", "#c79a2c", "#4f8a4a", "#8a4f8a", "#c46a2d"}

// decorate adds what makes a town lived in: windmills on the farmland,
// carts on the roads, mills and boats on the river, docks and ships on the
// coast.
func (c *City) decorate(p Params, r *rand.Rand, innerR float64, nearRiver func(cpt, float64) bool, edges map[string]*cityEdge, keys []string) {
	// Windmills on the open farmland, away from the walls.
	spots := append([]cpt(nil), c.farmSpots...)
	r.Shuffle(len(spots), func(a, b int) { spots[a], spots[b] = spots[b], spots[a] })
	want := 1 + p.CitySize/15
	for _, s := range spots {
		if len(c.windmills) >= want {
			break
		}
		if math.Hypot(s[0], s[1]) < innerR*1.25 || (len(c.river) > 0 && nearRiver(s, c.riverW+0.4)) {
			continue
		}
		far := true
		for _, w := range c.windmills {
			if math.Hypot(w[0]-s[0], w[1]-s[1]) < 2.5 {
				far = false
			}
		}
		if far {
			c.windmills = append(c.windmills, s)
		}
	}
	// Carts on the roads and the main streets.
	along := func(line []cpt, f float64) (cpt, float64) {
		total := 0.0
		for i := 0; i+1 < len(line); i++ {
			total += math.Hypot(line[i+1][0]-line[i][0], line[i+1][1]-line[i][1])
		}
		want := total * f
		for i := 0; i+1 < len(line); i++ {
			seg := math.Hypot(line[i+1][0]-line[i][0], line[i+1][1]-line[i][1])
			if want <= seg && seg > 0 {
				t := want / seg
				return cpt{line[i][0] + (line[i+1][0]-line[i][0])*t, line[i][1] + (line[i+1][1]-line[i][1])*t},
					math.Atan2(line[i+1][1]-line[i][1], line[i+1][0]-line[i][0])
			}
			want -= seg
		}
		return line[len(line)-1], 0
	}
	loads := []string{"", c.hayColour(p), "#8a6845", "#b9a27a"}
	for _, rd := range c.roads {
		for k := 0; k < 2; k++ {
			if r.Float64() < 0.65 {
				v, t := along(rd, 0.15+r.Float64()*0.5)
				if r.IntN(2) == 0 {
					t += math.Pi
				}
				c.carts = append(c.carts, placed{c: v, turn: t, size: 0.11, tag: loads[r.IntN(len(loads))]})
			}
		}
	}
	for _, m := range c.mains {
		if r.Float64() < 0.5 {
			v, t := along(m, 0.2+r.Float64()*0.6)
			c.carts = append(c.carts, placed{c: v, turn: t, size: 0.1, tag: loads[r.IntN(len(loads))]})
		}
	}
	// The river: mills where buildings stand on its bank, boats moored.
	if len(c.river) > 1 {
		bank := func(v cpt) (cpt, float64, cpt) {
			best, bd, turn := v, math.Inf(1), 0.0
			var normal cpt
			for i := 0; i+1 < len(c.river); i++ {
				a, b := c.river[i], c.river[i+1]
				dx, dy := b[0]-a[0], b[1]-a[1]
				L := dx*dx + dy*dy
				t := clamp(((v[0]-a[0])*dx+(v[1]-a[1])*dy)/L, 0, 1)
				q := cpt{a[0] + dx*t, a[1] + dy*t}
				if d := math.Hypot(v[0]-q[0], v[1]-q[1]); d < bd {
					bd, best, turn = d, q, math.Atan2(dy, dx)
					l := math.Sqrt(L)
					normal = cpt{-dy / l, dx / l}
					if (v[0]-q[0])*normal[0]+(v[1]-q[1])*normal[1] < 0 {
						normal = cpt{-normal[0], -normal[1]}
					}
				}
			}
			_ = bd
			return best, turn, normal
		}
		mills := 0
		for _, b := range c.buildings {
			if mills >= 2+p.CitySize/25 {
				break
			}
			cc := centroid(b)
			if !nearRiver(cc, c.riverW*0.5+0.35) {
				continue
			}
			q, t, n := bank(cc)
			spot := cpt{q[0] + n[0]*(c.riverW*0.5-0.05), q[1] + n[1]*(c.riverW*0.5-0.05)}
			ok := true
			for _, w := range c.wheels {
				if math.Hypot(w.c[0]-spot[0], w.c[1]-spot[1]) < 1.2 {
					ok = false
				}
			}
			if ok {
				c.wheels = append(c.wheels, placed{c: spot, turn: t, size: 0.13})
				mills++
			}
		}
		for k := 0; k < 3+p.CitySize/20; k++ {
			i := 2 + r.IntN(len(c.river)-4)
			a, b := c.river[i], c.river[i+1]
			t := math.Atan2(b[1]-a[1], b[0]-a[0])
			n := cpt{-math.Sin(t), math.Cos(t)}
			side := c.riverW * 0.28
			if r.IntN(2) == 0 {
				side = -side
			}
			c.boats = append(c.boats, placed{c: cpt{a[0] + n[0]*side, a[1] + n[1]*side}, turn: t + float64(r.IntN(2))*math.Pi, size: 0.2})
		}
	}
	// The coast: docks out from the town's shore, boats at them and ships
	// out at sea.
	if p.Seaside {
		type shore struct {
			mid, out cpt
		}
		var shores []shore
		for _, k := range keys {
			e := edges[k]
			if !e.coast {
				continue
			}
			mid := cpt{(e.pa[0] + e.pb[0]) / 2, (e.pa[1] + e.pb[1]) / 2}
			var in cpt
			for _, ci := range e.cells {
				if c.patches[ci].inner {
					in = c.patches[ci].site
				}
			}
			d := cpt{mid[0] - in[0], mid[1] - in[1]}
			l := math.Hypot(d[0], d[1])
			if l == 0 {
				continue
			}
			shores = append(shores, shore{mid, cpt{d[0] / l, d[1] / l}})
		}
		r.Shuffle(len(shores), func(a, b int) { shores[a], shores[b] = shores[b], shores[a] })
		for i, sh := range shores {
			if i >= 3+p.CitySize/10 {
				break
			}
			turn := math.Atan2(sh.out[1], sh.out[0])
			length := 0.45 + r.Float64()*0.5
			c.docks = append(c.docks, placed{c: sh.mid, turn: turn, size: length})
			n := cpt{-sh.out[1], sh.out[0]}
			for k := 0; k < 2; k++ {
				if r.Float64() < 0.7 {
					f := 0.3 + r.Float64()*0.6
					side := 0.12
					if k == 1 {
						side = -0.12
					}
					v := cpt{sh.mid[0] + sh.out[0]*length*f + n[0]*side, sh.mid[1] + sh.out[1]*length*f + n[1]*side}
					c.boats = append(c.boats, placed{c: v, turn: turn, size: 0.24})
				}
			}
		}
		for k := 0; k < 4+p.CitySize/12; k++ {
			pa := c.patches[r.IntN(len(c.patches))]
			if !pa.water {
				continue
			}
			c.boats = append(c.boats, placed{c: pa.site, turn: r.Float64() * 2 * math.Pi, size: 0.5 + r.Float64()*0.25, flag: true})
		}
	}
}

func (c *City) hayColour(p Params) string {
	lk, ok := looks[p.CityStyle]
	if !ok {
		lk = looks["light"]
	}
	return lk.hay
}

func towardBy(a, b cpt, by float64) cpt {
	d := math.Hypot(b[0]-a[0], b[1]-a[1])
	if d < 1e-9 {
		return a
	}
	t := math.Min(0.45, by/d)
	return cpt{a[0] + (b[0]-a[0])*t, a[1] + (b[1]-a[1])*t}
}

func scalePoly(p []cpt, c cpt, k float64) []cpt {
	out := make([]cpt, len(p))
	for i, v := range p {
		out[i] = cpt{c[0] + (v[0]-c[0])*k, c[1] + (v[1]-c[1])*k}
	}
	return out
}

func pointIn(v cpt, poly []cpt) bool {
	in := false
	for i, j := 0, len(poly)-1; i < len(poly); j, i = i, i+1 {
		a, b := poly[i], poly[j]
		if (a[1] > v[1]) != (b[1] > v[1]) && v[0] < (b[0]-a[0])*(v[1]-a[1])/(b[1]-a[1])+a[0] {
			in = !in
		}
	}
	return in
}

func segDist2(v, a, b cpt) float64 {
	dx, dy := b[0]-a[0], b[1]-a[1]
	L := dx*dx + dy*dy
	t := 0.0
	if L > 0 {
		t = clamp(((v[0]-a[0])*dx+(v[1]-a[1])*dy)/L, 0, 1)
	}
	ex, ey := v[0]-(a[0]+dx*t), v[1]-(a[1]+dy*t)
	return ex*ex + ey*ey
}

func segCross(a, b, c, d cpt) (cpt, bool) {
	r := cpt{b[0] - a[0], b[1] - a[1]}
	s := cpt{d[0] - c[0], d[1] - c[1]}
	den := r[0]*s[1] - r[1]*s[0]
	if math.Abs(den) < 1e-12 {
		return cpt{}, false
	}
	t := ((c[0]-a[0])*s[1] - (c[1]-a[1])*s[0]) / den
	u := ((c[0]-a[0])*r[1] - (c[1]-a[1])*r[0]) / den
	if t < 0 || t > 1 || u < 0 || u > 1 {
		return cpt{}, false
	}
	return cpt{a[0] + r[0]*t, a[1] + r[1]*t}, true
}

// lots cuts a block into lots and puts a building on most: each cut across
// the block's longest side, a little off its middle, so the lots come out in
// rows along the streets.
func (c *City) lots(block []cpt, wk wardKind, r *rand.Rand, skip func(v cpt, w float64) bool) {
	if block == nil {
		return
	}
	var cut func(poly []cpt, depth int)
	cut = func(poly []cpt, depth int) {
		a := math.Abs(polyArea(poly))
		if len(poly) < 3 || a < 1e-4 {
			return
		}
		if a < wk.lot*(0.6+r.Float64()*0.9) || depth > 14 {
			if r.Float64() < wk.empty {
				return
			}
			// A triangle is a corner's leftover, not a house.
			if len(poly) == 3 && r.Float64() < 0.85 {
				return
			}
			cc := centroid(poly)
			if skip != nil && skip(cc, c.riverW*0.5+0.12) {
				return
			}
			if b := shrink(poly, wk.gap*(0.6+r.Float64()*0.8)); b != nil {
				c.buildings = append(c.buildings, b)
			}
			return
		}
		long, li := -1.0, 0
		for i := range poly {
			q := poly[(i+1)%len(poly)]
			if l := math.Hypot(q[0]-poly[i][0], q[1]-poly[i][1]); l > long {
				long, li = l, i
			}
		}
		pa, pb := poly[li], poly[(li+1)%len(poly)]
		t := 0.5 + (r.Float64()-0.5)*0.35
		m := cpt{pa[0] + (pb[0]-pa[0])*t, pa[1] + (pb[1]-pa[1])*t}
		d := cpt{pb[0] - pa[0], pb[1] - pa[1]}
		// Across the long side, give or take a few degrees.
		ang := (r.Float64() - 0.5) * 0.2
		perp := cpt{-d[1]*math.Cos(ang) - d[0]*math.Sin(ang), d[0]*math.Cos(ang) - d[1]*math.Sin(ang)}
		x, y := splitPoly(poly, m, perp)
		cut(x, depth+1)
		cut(y, depth+1)
	}
	cut(block, 0)
}

// farm cuts a field into strips with furrows, and puts a farmhouse by one.
func (c *City) farm(block []cpt, r *rand.Rand) {
	a := r.Float64() * math.Pi
	d := cpt{math.Cos(a), math.Sin(a)}
	n := cpt{-d[1], d[0]}
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, v := range block {
		s := v[0]*n[0] + v[1]*n[1]
		lo, hi = math.Min(lo, s), math.Max(hi, s)
	}
	rest := block
	for s := lo + 0.6 + r.Float64()*0.6; s < hi-0.3 && rest != nil; s += 0.6 + r.Float64()*0.7 {
		x, y := splitPoly(rest, cpt{n[0] * s, n[1] * s}, d)
		if len(x) >= 3 {
			c.field(x, d)
		}
		rest = y
		if len(rest) < 3 {
			rest = nil
		}
	}
	if rest != nil {
		c.field(rest, d)
	}
	c.hedges = append(c.hedges, block)
	cc := centroid(block)
	c.farmSpots = append(c.farmSpots, cc)
	if r.Float64() < 0.6 {
		// A farmhouse and a barn by it, and maybe a round granary.
		h := 0.09 + r.Float64()*0.05
		a := r.Float64() * math.Pi
		c.buildings = append(c.buildings, rect(cc, h*3.2, h*2, a))
		c.buildings = append(c.buildings, rect(cpt{cc[0] + math.Cos(a+1.6)*h*3, cc[1] + math.Sin(a+1.6)*h*3}, h*2.4, h*1.7, a))
		if r.Float64() < 0.45 {
			c.granaries = append(c.granaries, cpt{cc[0] + math.Cos(a-1.4)*h*2.8, cc[1] + math.Sin(a-1.4)*h*2.8})
		}
	}
	// Haystacks in a field or two.
	if r.Float64() < 0.4 {
		for k := 0; k < 2+r.IntN(4); k++ {
			v := cpt{cc[0] + (r.Float64()-0.5)*0.8, cc[1] + (r.Float64()-0.5)*0.8}
			if pointIn(v, block) {
				c.haystacks = append(c.haystacks, v)
			}
		}
	}
}

// longAxis is the way a polygon's longest side runs.
func longAxis(poly []cpt) float64 {
	long, ang := -1.0, 0.0
	for i := range poly {
		q := poly[(i+1)%len(poly)]
		if l := math.Hypot(q[0]-poly[i][0], q[1]-poly[i][1]); l > long {
			long, ang = l, math.Atan2(q[1]-poly[i][1], q[0]-poly[i][0])
		}
	}
	return ang
}

// cathedral: a long nave, a transept across it, a round apse at the east
// end and a square tower at the west.
func (c *City) cathedral(at cpt, L, turn float64) [][]cpt {
	d := cpt{math.Cos(turn), math.Sin(turn)}
	off := func(f float64) cpt { return cpt{at[0] + d[0]*L*f, at[1] + d[1]*L*f} }
	nave, transept := rect(off(0), L, L*0.3, turn), rect(off(0.18), L*0.24, L*0.66, turn)
	tower := rect(off(-0.52), L*0.26, L*0.26, turn)
	c.buildings = append(c.buildings, nave, transept)
	c.rounds = append(c.rounds, placed{c: off(0.5), size: L * 0.15})
	c.keeps = append(c.keeps, tower)
	return [][]cpt{nave, transept, tower, rect(off(0.5), L*0.3, L*0.3, turn)}
}

// rect is a w × h rectangle about c, turned.
func rect(c cpt, w, h, turn float64) []cpt {
	d := cpt{math.Cos(turn) * w / 2, math.Sin(turn) * w / 2}
	n := cpt{-math.Sin(turn) * h / 2, math.Cos(turn) * h / 2}
	return []cpt{{c[0] - d[0] - n[0], c[1] - d[1] - n[1]}, {c[0] + d[0] - n[0], c[1] + d[1] - n[1]}, {c[0] + d[0] + n[0], c[1] + d[1] + n[1]}, {c[0] - d[0] + n[0], c[1] - d[1] + n[1]}}
}

func (c *City) field(poly []cpt, d cpt) {
	f := shrink(poly, 0.05)
	if f == nil {
		return
	}
	c.fields = append(c.fields, f)
	c.cropOf = append(c.cropOf, len(c.fields)*7919%13)
	// Furrows along the strip.
	n := cpt{-d[1], d[0]}
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, v := range f {
		s := v[0]*n[0] + v[1]*n[1]
		lo, hi = math.Min(lo, s), math.Max(hi, s)
	}
	for s := lo + 0.09; s < hi; s += 0.09 {
		var hits []cpt
		for i := range f {
			a, b := f[i], f[(i+1)%len(f)]
			sa, sb := a[0]*n[0]+a[1]*n[1], b[0]*n[0]+b[1]*n[1]
			if (sa-s)*(sb-s) < 0 {
				t := (s - sa) / (sb - sa)
				hits = append(hits, cpt{a[0] + (b[0]-a[0])*t, a[1] + (b[1]-a[1])*t})
			}
		}
		if len(hits) == 2 {
			c.furrows = append(c.furrows, [2]cpt{hits[0], hits[1]})
		}
	}
}

// wood fills a block with trees, thicker or thinner.
func (c *City) wood(block []cpt, r *rand.Rand, density float64) {
	lo, hi := cpt{math.Inf(1), math.Inf(1)}, cpt{math.Inf(-1), math.Inf(-1)}
	for _, v := range block {
		lo = cpt{math.Min(lo[0], v[0]), math.Min(lo[1], v[1])}
		hi = cpt{math.Max(hi[0], v[0]), math.Max(hi[1], v[1])}
	}
	step := 0.2
	for y := lo[1]; y < hi[1]; y += step {
		for x := lo[0]; x < hi[0]; x += step {
			if r.Float64() > density {
				continue
			}
			v := cpt{x + r.Float64()*step, y + r.Float64()*step}
			if pointIn(v, block) {
				c.trees = append(c.trees, v)
			}
		}
	}
}

// ---- drawing -----------------------------------------------------------

// SVG draws a town, size × size pixels, in one of the looks (decor.go).
func (c *City) SVG(style string) []byte {
	lk, ok := looks[style]
	if !ok {
		lk = looks["light"]
	}
	half := c.Size / 2
	a := &art{pen: pen{S: c.scale, ox: half, oy: half}, lk: lk}
	var b strings.Builder
	sz := fmt.Sprintf("%.0f", c.Size)
	b.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" width="` + sz + `" height="` + sz + `" viewBox="0 0 ` + sz + ` ` + sz + `">`)
	b.WriteString(`<rect width="100%" height="100%" fill="` + lk.grass + `"/>`)
	w := a.w

	// The sea: deep water, a lighter band along the shore, foam on it.
	var sea strings.Builder
	for _, pa := range c.patches {
		if pa.water {
			sea.WriteString(a.poly(pa.poly))
		}
	}
	if sea.Len() > 0 {
		b.WriteString(`<path d="` + sea.String() + `" fill="` + lk.waterDeep + `" stroke="` + lk.waterDeep + `" stroke-width="` + w(0.04) + `"/>`)
		var shore strings.Builder
		for _, pa := range c.patches {
			if !pa.water {
				continue
			}
			for k := range pa.poly {
				a0, b0 := pa.poly[k], pa.poly[(k+1)%len(pa.poly)]
				shore.WriteString("M" + a.pt(a0) + "L" + a.pt(b0))
			}
		}
		b.WriteString(`<g clip-path="url(#mg-sea)"><clipPath id="mg-sea"><path d="` + sea.String() + `"/></clipPath>`)
		for _, band := range []struct {
			wd float64
			col string
			op  string
		}{{1.1, lk.water, "1"}, {0.5, lk.foam, "0.55"}, {0.12, lk.foam, "0.9"}} {
			b.WriteString(`<path d="` + c.landEdge(a) + `" fill="none" stroke="` + band.col + `" stroke-opacity="` + band.op + `" stroke-width="` + w(band.wd) + `" stroke-linejoin="round"/>`)
		}
		b.WriteString(`</g>`)
		_ = shore
	}
	// Fields in their crops, furrows, and hedges round the farms.
	byCrop := map[int]*strings.Builder{}
	for i, f := range c.fields {
		k := 0
		if i < len(c.cropOf) {
			k = c.cropOf[i] % len(lk.crops)
		}
		if byCrop[k] == nil {
			byCrop[k] = &strings.Builder{}
		}
		byCrop[k].WriteString(a.poly(f))
	}
	for k := 0; k < len(lk.crops); k++ {
		if byCrop[k] != nil {
			b.WriteString(`<path d="` + byCrop[k].String() + `" fill="` + lk.crops[k] + `" stroke="` + lk.furrow + `" stroke-width="` + w(0.015) + `"/>`)
		}
	}
	var furrows strings.Builder
	for _, f := range c.furrows {
		furrows.WriteString("M" + a.pt(f[0]) + "L" + a.pt(f[1]))
	}
	b.WriteString(`<path d="` + furrows.String() + `" stroke="` + lk.furrow + `" stroke-width="` + w(0.01) + `" stroke-opacity="0.65"/>`)
	var hedges strings.Builder
	for _, h := range c.hedges {
		hedges.WriteString(a.poly(h))
	}
	b.WriteString(`<path d="` + hedges.String() + `" fill="none" stroke="` + lk.hedge + `" stroke-width="` + w(0.07) + `" stroke-dasharray="0 ` + w(0.09) + `" stroke-linecap="round" stroke-opacity="0.85"/>`)
	// The wards' ground, and the plaza paved.
	var ground strings.Builder
	for _, pa := range c.patches {
		if pa.inner && pa.ward != "plaza" {
			ground.WriteString(a.poly(pa.poly))
		}
	}
	b.WriteString(`<path d="` + ground.String() + `" fill="` + lk.ground + `" stroke="none"/>`)
	if c.plazaPoly != nil {
		b.WriteString(`<path d="` + a.poly(c.plazaPoly) + `" fill="` + lk.plaza + `" stroke="` + lk.roadEdge + `" stroke-width="` + w(0.02) + `"/>`)
	}
	// The river: banks, water, a light run down its middle.
	if len(c.river) > 1 {
		rv := a.smooth(c.river)
		b.WriteString(`<path d="` + rv + `" fill="none" stroke="` + lk.waterInk + `" stroke-width="` + w(c.riverW+0.1) + `" stroke-linecap="round" stroke-linejoin="round"/>`)
		b.WriteString(`<path d="` + rv + `" fill="none" stroke="` + lk.water + `" stroke-width="` + w(c.riverW) + `" stroke-linecap="round" stroke-linejoin="round"/>`)
		b.WriteString(`<path d="` + rv + `" fill="none" stroke="` + lk.foam + `" stroke-opacity="0.5" stroke-width="` + w(c.riverW*0.18) + `" stroke-dasharray="` + w(0.5) + ` ` + w(0.9) + `" stroke-linecap="round"/>`)
	}
	// Roads out of town: an edge, a surface, ruts.
	for _, rd := range c.roads {
		d := a.smooth(rd)
		b.WriteString(`<path d="` + d + `" fill="none" stroke="` + lk.roadEdge + `" stroke-width="` + w(0.3) + `" stroke-linecap="round" stroke-linejoin="round"/>`)
		b.WriteString(`<path d="` + d + `" fill="none" stroke="` + lk.road + `" stroke-width="` + w(0.24) + `" stroke-linecap="round" stroke-linejoin="round"/>`)
		b.WriteString(`<path d="` + d + `" fill="none" stroke="` + lk.roadEdge + `" stroke-opacity="0.45" stroke-width="` + w(0.012) + `" stroke-dasharray="` + w(0.25) + ` ` + w(0.12) + `"/>`)
	}
	for _, br := range c.bridges {
		b.WriteString(`<path d="M` + a.pt(br[0]) + `L` + a.pt(br[1]) + `" stroke="` + lk.shadow + `" stroke-opacity="0.3" stroke-width="` + w(0.4) + `" transform="translate(` + w(0.04) + ` ` + w(0.05) + `)"/>`)
		b.WriteString(`<path d="M` + a.pt(br[0]) + `L` + a.pt(br[1]) + `" stroke="` + lk.ink + `" stroke-width="` + w(0.34) + `"/>`)
		b.WriteString(`<path d="M` + a.pt(br[0]) + `L` + a.pt(br[1]) + `" stroke="` + lk.stone + `" stroke-width="` + w(0.26) + `"/>`)
	}

	// The things on the ground: docks, haystacks; then the buildings.
	for _, d := range c.docks {
		a.dock(d.c, d.turn, d.size, 0.1)
	}
	for _, h := range c.haystacks {
		a.haystack(h, 0.05)
	}
	for _, p := range c.buildings {
		a.building(p, 0.035)
	}
	for _, p := range c.keeps {
		a.building(p, 0.07)
	}
	for _, g := range c.granaries {
		a.round(g, 0.09, 0.03)
	}
	for _, rr := range c.rounds {
		a.round(rr.c, rr.size, 0.04)
	}
	for i, t := range c.trees {
		a.treeAt(t, 0.085, float64(i)*1.7)
	}
	a.write(&b, 0.018)

	// The castle's wall and the town's: dark stone with a lighter walk.
	var cw, wl strings.Builder
	for _, e := range c.castle {
		cw.WriteString("M" + a.pt(e[0]) + "L" + a.pt(e[1]))
	}
	for _, e := range c.walls {
		wl.WriteString("M" + a.pt(e[0]) + "L" + a.pt(e[1]))
	}
	for _, wall := range []struct {
		d     string
		width float64
	}{{cw.String(), 0.11}, {wl.String(), 0.17}} {
		if wall.d == "" {
			continue
		}
		b.WriteString(`<path d="` + wall.d + `" stroke="` + lk.shadow + `" stroke-opacity="` + fmt.Sprintf("%.2f", lk.shadowAlpha) + `" stroke-width="` + w(wall.width) + `" stroke-linecap="round" transform="translate(` + w(0.05) + ` ` + w(0.06) + `)"/>`)
		b.WriteString(`<path d="` + wall.d + `" stroke="` + lk.ink + `" stroke-width="` + w(wall.width) + `" stroke-linecap="round"/>`)
		b.WriteString(`<path d="` + wall.d + `" stroke="` + lk.stone + `" stroke-width="` + w(wall.width*0.45) + `" stroke-linecap="round"/>`)
	}
	var tw, tws strings.Builder
	for _, t := range c.towers {
		tws.WriteString(a.circle(cpt{t[0] + 0.05, t[1] + 0.06}, 0.14))
		tw.WriteString(a.circle(t, 0.14))
	}
	b.WriteString(`<path d="` + tws.String() + `" fill="` + lk.shadow + `" fill-opacity="` + fmt.Sprintf("%.2f", lk.shadowAlpha) + `"/>`)
	b.WriteString(`<path d="` + tw.String() + `" fill="` + lk.stone + `" stroke="` + lk.ink + `" stroke-width="` + w(0.045) + `"/>`)
	for _, g := range c.gates {
		b.WriteString(`<path d="` + a.circle(g, 0.2) + `" fill="` + lk.stone + `" stroke="` + lk.ink + `" stroke-width="` + w(0.05) + `"/>`)
		b.WriteString(`<path d="` + a.circle(g, 0.11) + `" fill="` + lk.road + `" stroke="` + lk.ink + `" stroke-width="` + w(0.03) + `"/>`)
	}

	// What stands over it all: windmills, wheels, boats, carts, stalls,
	// graves, the fountain.
	top := &art{pen: a.pen, lk: lk}
	for i, wm := range c.windmills {
		top.windmill(wm, 0.38, 0.35+float64(i)*0.9)
	}
	for _, wh := range c.wheels {
		top.waterwheel(wh.c, wh.size, wh.turn)
	}
	for _, bt := range c.boats {
		top.boat(bt.c, bt.size, bt.turn, bt.flag)
	}
	for _, ct := range c.carts {
		top.cart(ct.c, ct.size, ct.turn, ct.tag)
	}
	for _, st := range c.stalls {
		top.stall(st.c, st.size, st.turn, st.tag)
	}
	for _, g := range c.graves {
		top.grave(g.c, g.size, g.flag)
	}
	if c.fountain != nil {
		top.well(*c.fountain, 0.16)
	}
	top.write(&b, 0.018)
	b.WriteString(paperGrain(c.Size, 0.06))
	b.WriteString(`</svg>`)
	return []byte(b.String())
}

// landEdge is the coast as path data: every edge between land and sea.
func (c *City) landEdge(a *art) string {
	var b strings.Builder
	for _, pa := range c.patches {
		if pa.water {
			continue
		}
		for k := range pa.poly {
			a0, b0 := pa.poly[k], pa.poly[(k+1)%len(pa.poly)]
			b.WriteString("M" + a.pt(a0) + "L" + a.pt(b0))
		}
	}
	return b.String()
}

type strAt struct {
	k string
	d float64
}

type strHeap []strAt

func (h strHeap) Len() int            { return len(h) }
func (h strHeap) Less(a, b int) bool  { return h[a].d < h[b].d }
func (h strHeap) Swap(a, b int)       { h[a], h[b] = h[b], h[a] }
func (h *strHeap) Push(x interface{}) { *h = append(*h, x.(strAt)) }
func (h *strHeap) Pop() interface{} {
	old := *h
	x := old[len(old)-1]
	*h = old[:len(old)-1]
	return x
}
