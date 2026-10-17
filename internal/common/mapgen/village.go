package mapgen

import (
	"fmt"
	"math"
	"math/rand/v2"
	"sort"
	"strings"
)

// A village, in the manner of watabou's Village Generator
// (https://watabou.itch.io/village-generator - the idea, not its code): a
// road winding through and lanes off it; houses along them, each on its
// plot with a fenced garden behind; a green with a well, the church and its
// churchyard beside it; farms with barns, granaries and haystacks at the
// edges; fields in strips with hedges, woods, orchards, pasture with sheep;
// a stream with a mill and bridges, a pond, or the sea with piers and boats;
// windmills on the open ground, and carts on the roads.
//
// Measured in metres; the picture shows `villageSpan` metres a side.

const villageSpan = 340.0

type Village struct {
	Size  float64
	half  float64
	roads []vroad
	sea   []cpt
	shore []cpt
	river []cpt
	riverW float64
	pond  []cpt
	green []cpt
	yard  []cpt // the churchyard

	houses  [][]cpt
	barns   [][]cpt
	church  [][]cpt
	apse    *placed
	plots   []vplot
	fields  []vfield
	pasture [][]cpt
	woods   []cpt
	orchard []cpt
	hedgeTrees []cpt
	trees   []cpt
	sheep   []cpt

	bridges   []placed
	windmills []placed
	wheels    []placed
	granaries []cpt
	haystacks []cpt
	graves    []placed
	boats     []placed
	docks     []placed
	carts     []placed
	well      *cpt
}

type vroad struct {
	pts   []cpt
	width float64
}

type vplot struct {
	poly   []cpt
	front  int // which side faces the road (not fenced)
	garden string
	rows   [][2]cpt
}

type vfield struct {
	poly    []cpt
	crop    int
	furrows [][2]cpt
}

// convexOverlap: do two convex polygons overlap (separating axes)?
func convexOverlap(a, b []cpt) bool {
	for _, poly := range [][]cpt{a, b} {
		for i := range poly {
			p, q := poly[i], poly[(i+1)%len(poly)]
			n := cpt{p[1] - q[1], q[0] - p[0]}
			minA, maxA, minB, maxB := math.Inf(1), math.Inf(-1), math.Inf(1), math.Inf(-1)
			for _, v := range a {
				d := v[0]*n[0] + v[1]*n[1]
				minA, maxA = math.Min(minA, d), math.Max(maxA, d)
			}
			for _, v := range b {
				d := v[0]*n[0] + v[1]*n[1]
				minB, maxB = math.Min(minB, d), math.Max(maxB, d)
			}
			if maxA < minB || maxB < minA {
				return false
			}
		}
	}
	return true
}

func distToLine(v cpt, line []cpt) float64 {
	best := math.Inf(1)
	for i := 0; i+1 < len(line); i++ {
		best = math.Min(best, segDist2(v, line[i], line[i+1]))
	}
	return math.Sqrt(best)
}

// GenerateVillage lays out a village.
func GenerateVillage(p Params) *Village {
	p = p.Normal()
	r := rand.New(rand.NewPCG(p.Seed, p.Seed^0x7a3c5e1f9b2d4c6a))
	noise := newNoise(p.Seed*13 + 7)
	v := &Village{Size: float64(p.CityPx)}
	v.half = villageSpan / 2
	H := v.half

	// The sea, on one side if asked: everything beyond a ragged shore.
	seaDir := r.Float64() * 2 * math.Pi
	sd := cpt{math.Cos(seaDir), math.Sin(seaDir)}
	sideOf := cpt{-sd[1], sd[0]}
	shoreOff := 55 + r.Float64()*30
	if p.Seaside {
		for t := -H * 1.6; t <= H*1.6; t += 6 {
			off := shoreOff + noise.fbm(t*0.012+3, 1.1, 4)*28
			v.shore = append(v.shore, cpt{sd[0]*off + sideOf[0]*t, sd[1]*off + sideOf[1]*t})
		}
		v.sea = append(append([]cpt{}, v.shore...),
			cpt{sd[0]*H*3 + sideOf[0]*H*1.6, sd[1]*H*3 + sideOf[1]*H*1.6},
			cpt{sd[0]*H*3 - sideOf[0]*H*1.6, sd[1]*H*3 - sideOf[1]*H*1.6})
		// The polygon's vertices in order: the shore one way, back the far side.
		v.sea = append(append([]cpt{}, v.shore...), cpt{sd[0]*H*3 + sideOf[0]*H*1.6, sd[1]*H*3 + sideOf[1]*H*1.6}, cpt{sd[0]*H*3 - sideOf[0]*H*1.6, sd[1]*H*3 - sideOf[1]*H*1.6})
	}
	inSea := func(q cpt) bool {
		if !p.Seaside {
			return false
		}
		return q[0]*sd[0]+q[1]*sd[1] > shoreOff+noise.fbm((q[0]*sideOf[0]+q[1]*sideOf[1])*0.012+3, 1.1, 4)*28
	}

	// The stream: in at one edge, out at another (to the sea, if there is
	// one), passing near the middle.
	if p.River {
		a := r.Float64() * 2 * math.Pi
		if p.Seaside {
			a = seaDir + math.Pi + (r.Float64()-0.5)*0.8
		}
		from := cpt{math.Cos(a) * H * 1.3, math.Sin(a) * H * 1.3}
		to := cpt{-from[0], -from[1]}
		if p.Seaside {
			to = cpt{sd[0] * (shoreOff + 40), sd[1] * (shoreOff + 40)}
		}
		d := cpt{to[0] - from[0], to[1] - from[1]}
		n := cpt{-d[1], d[0]}
		nl := math.Hypot(n[0], n[1])
		n = cpt{n[0] / nl, n[1] / nl}
		pass := 18 + r.Float64()*25
		for t := 0.0; t <= 1.0001; t += 0.02 {
			bend := noise.fbm(t*3.5+11, 4.2, 3)*45 + math.Sin(t*math.Pi)*pass
			q := cpt{from[0] + d[0]*t + n[0]*bend, from[1] + d[1]*t + n[1]*bend}
			v.river = append(v.river, q)
			if inSea(q) {
				break
			}
		}
		v.riverW = 7 + r.Float64()*4
	}
	nearWater := func(q cpt, m float64) bool {
		if inSea(cpt{q[0] - sd[0]*m, q[1] - sd[1]*m}) {
			return true
		}
		if len(v.river) > 1 && distToLine(q, v.river) < v.riverW/2+m {
			return true
		}
		if v.pond != nil && pointIn(q, scalePoly(v.pond, centroid(v.pond), 1+m/12)) {
			return true
		}
		return false
	}

	// The roads: one winding through, along the shore if there is one, and
	// a road off it away from the sea; lanes branching from them.
	mainAng := r.Float64() * math.Pi
	if p.Seaside {
		mainAng = seaDir + math.Pi/2
	}
	walk := func(start cpt, ang float64, length, width float64, wander float64) vroad {
		rd := vroad{width: width}
		q := start
		for s := 0.0; s <= length; s += 6 {
			rd.pts = append(rd.pts, q)
			ang += noise.at(q[0]*0.02+float64(len(v.roads))*9, q[1]*0.02) * wander
			nq := cpt{q[0] + math.Cos(ang)*6, q[1] + math.Sin(ang)*6}
			if nearWater(nq, width) && !(len(v.river) > 1 && distToLine(nq, v.river) < v.riverW/2+width && !inSea(nq)) {
				// The sea or the pond stops a road; a stream it bridges.
				if inSea(nq) || (v.pond != nil && pointIn(nq, v.pond)) {
					break
				}
			}
			if math.Abs(nq[0]) > H*1.15 || math.Abs(nq[1]) > H*1.15 {
				rd.pts = append(rd.pts, nq)
				break
			}
			q = nq
		}
		return rd
	}
	centre := cpt{(r.Float64() - 0.5) * 30, (r.Float64() - 0.5) * 30}
	if p.Seaside {
		centre = cpt{-sd[0] * (shoreOff * 0.15), -sd[1] * (shoreOff * 0.15)}
	}
	// The main road, out from the middle both ways.
	one := walk(centre, mainAng, H*1.5, 7, 0.22)
	two := walk(centre, mainAng+math.Pi, H*1.5, 7, 0.22)
	mainPts := append([]cpt{}, two.pts...)
	for i, j := 0, len(mainPts)-1; i < j; i, j = i+1, j-1 {
		mainPts[i], mainPts[j] = mainPts[j], mainPts[i]
	}
	mainPts = append(mainPts, one.pts[1:]...)
	v.roads = append(v.roads, vroad{pts: mainPts, width: 7})
	crossAng := mainAng + math.Pi/2 + (r.Float64()-0.5)*0.6
	if p.Seaside {
		crossAng = seaDir + math.Pi
	}
	v.roads = append(v.roads, walk(centre, crossAng, H*1.4, 6, 0.25))
	if !p.Seaside && r.Float64() < 0.6 {
		v.roads = append(v.roads, walk(centre, crossAng+math.Pi, H*1.4, 6, 0.25))
	}
	lanes := 1 + p.Houses/22
	for k := 0; k < lanes; k++ {
		base := v.roads[r.IntN(len(v.roads))]
		if len(base.pts) < 8 {
			continue
		}
		i := 3 + r.IntN(len(base.pts)-6)
		q := base.pts[i]
		if math.Hypot(q[0]-centre[0], q[1]-centre[1]) > H*0.7 {
			continue
		}
		dir := math.Atan2(base.pts[i+1][1]-base.pts[i][1], base.pts[i+1][0]-base.pts[i][0])
		side := math.Pi / 2
		if r.IntN(2) == 0 {
			side = -side
		}
		v.roads = append(v.roads, walk(q, dir+side+(r.Float64()-0.5)*0.5, 40+r.Float64()*80, 4.5, 0.3))
	}
	onRoad := func(q cpt, m float64) bool {
		for _, rd := range v.roads {
			if distToLine(q, rd.pts) < rd.width/2+m {
				return true
			}
		}
		return false
	}

	// The pond, somewhere near but not on the roads.
	if p.Pond {
		for tries := 0; tries < 40; tries++ {
			a := r.Float64() * 2 * math.Pi
			d := 35 + r.Float64()*55
			c := cpt{centre[0] + math.Cos(a)*d, centre[1] + math.Sin(a)*d}
			rad := 9 + r.Float64()*7
			if onRoad(c, rad+4) || nearWater(c, rad+6) {
				continue
			}
			for i := 0; i < 24; i++ {
				t := float64(i) / 24 * 2 * math.Pi
				rr := rad * (1 + noise.at(math.Cos(t)*1.3+c[0]*0.1, math.Sin(t)*1.3)*0.35)
				v.pond = append(v.pond, cpt{c[0] + math.Cos(t)*rr*1.25, c[1] + math.Sin(t)*rr})
			}
			break
		}
	}

	// The green, the well on it, and the church beside it.
	gr := 13 + r.Float64()*6
	for i := 0; i < 20; i++ {
		t := float64(i) / 20 * 2 * math.Pi
		rr := gr * (1 + noise.at(math.Cos(t)*1.1+40, math.Sin(t)*1.1)*0.25)
		v.green = append(v.green, cpt{centre[0] + math.Cos(t)*rr, centre[1] + math.Sin(t)*rr})
	}
	w := cpt{centre[0] + 4, centre[1] - 3}
	v.well = &w
	taken := [][]cpt{v.green}
	free := func(poly []cpt, m float64) bool {
		// Every corner, points along every edge and across the middle: a
		// road can cross a plot without coming near a corner.
		var test []cpt
		for i, q := range poly {
			nq := poly[(i+1)%len(poly)]
			l := math.Hypot(nq[0]-q[0], nq[1]-q[1])
			for f := 0.0; f < l; f += 2.5 {
				test = append(test, cpt{q[0] + (nq[0]-q[0])*f/l, q[1] + (nq[1]-q[1])*f/l})
			}
		}
		cc := centroid(poly)
		for _, q := range poly {
			test = append(test, cpt{(q[0] + cc[0]) / 2, (q[1] + cc[1]) / 2})
		}
		test = append(test, cc)
		for _, q := range test {
			if nearWater(q, m) || onRoad(q, m) || math.Abs(q[0]) > H*1.2 || math.Abs(q[1]) > H*1.2 {
				return false
			}
		}
		for _, t := range taken {
			if convexOverlap(poly, t) {
				return false
			}
		}
		return true
	}
	if p.Church {
		for tries := 0; tries < 60; tries++ {
			a := r.Float64() * 2 * math.Pi
			d := gr + 16 + r.Float64()*10
			c := cpt{centre[0] + math.Cos(a)*d, centre[1] + math.Sin(a)*d}
			yard := rect(c, 40, 30, 0)
			if !free(yard, 2) {
				continue
			}
			v.yard = yard
			taken = append(taken, yard)
			nave := rect(cpt{c[0] - 3, c[1] - 2}, 22, 8.5, 0)
			tower := rect(cpt{c[0] - 16, c[1] - 2}, 7, 7, 0)
			v.church = [][]cpt{nave, tower}
			v.apse = &placed{c: cpt{c[0] + 8, c[1] - 2}, size: 4.2}
			for gy := c[1] + 6; gy < c[1]+13.5; gy += 2.4 {
				for gx := c[0] - 17; gx < c[0]+17; gx += 2.2 {
					if r.Float64() < 0.75 {
						v.graves = append(v.graves, placed{c: cpt{gx + (r.Float64()-0.5)*0.6, gy}, size: 0.75, flag: r.Float64() < 0.3})
					}
				}
			}
			v.trees = append(v.trees, cpt{c[0] + 16, c[1] - 11}, cpt{c[0] - 17, c[1] + 11}, cpt{c[0] + 17, c[1] + 10})
			break
		}
	}

	// Houses along the roads, the nearest the green first, each on its plot.
	type spot struct {
		at, dir cpt
		side    float64
		d       float64
		width   float64
	}
	var spots []spot
	for _, rd := range v.roads {
		for i := 1; i+1 < len(rd.pts); i++ {
			a, b := rd.pts[i-1], rd.pts[i+1]
			d := cpt{b[0] - a[0], b[1] - a[1]}
			l := math.Hypot(d[0], d[1])
			if l == 0 {
				continue
			}
			d = cpt{d[0] / l, d[1] / l}
			dist := math.Hypot(rd.pts[i][0]-centre[0], rd.pts[i][1]-centre[1])
			for _, s := range []float64{-1, 1} {
				spots = append(spots, spot{rd.pts[i], d, s, dist + r.Float64()*25, rd.width})
			}
		}
	}
	sort.Slice(spots, func(a, b int) bool { return spots[a].d < spots[b].d })
	built := 0
	for _, s := range spots {
		if built >= p.Houses {
			break
		}
		n := cpt{-s.dir[1] * s.side, s.dir[0] * s.side}
		setback := s.width/2 + 2.5 + r.Float64()*3
		hw, hd := 9+r.Float64()*5, 6.5+r.Float64()*2.5
		turn := math.Atan2(s.dir[1], s.dir[0])
		long := r.Float64() < 0.75 // the long side to the road, mostly
		if !long {
			hw, hd = hd, hw
		}
		hc := cpt{s.at[0] + n[0]*(setback+hd/2), s.at[1] + n[1]*(setback+hd/2)}
		house := rect(hc, hw, hd, turn)
		farm := s.d > 80 && r.Float64() < 0.35
		depth := 18 + r.Float64()*16
		pw := hw + 2.5 + r.Float64()*5
		if farm {
			depth, pw = 32+r.Float64()*14, hw+16+r.Float64()*8
		}
		pc := cpt{s.at[0] + n[0]*(setback+depth/2-1), s.at[1] + n[1]*(setback+depth/2-1)}
		plot := rect(pc, pw, depth+1, turn)
		if !free(plot, 0.6) {
			continue
		}
		taken = append(taken, plot)
		v.houses = append(v.houses, house)
		// A wing at the back, now and then: an L-shaped house.
		if r.Float64() < 0.4 {
			ws := 1.0
			if r.IntN(2) == 0 {
				ws = -1
			}
			wing := rect(cpt{hc[0] + s.dir[0]*ws*(hw/2-2.2) + n[0]*(hd/2+2.2), hc[1] + s.dir[1]*ws*(hw/2-2.2) + n[1]*(hd/2+2.2)}, 4.4, 5, turn)
			v.houses = append(v.houses, wing)
		}
		vp := vplot{poly: plot, front: 0}
		// Which edge of the plot faces the road: the one nearest it.
		best := math.Inf(1)
		for k := range plot {
			m := cpt{(plot[k][0] + plot[(k+1)%4][0]) / 2, (plot[k][1] + plot[(k+1)%4][1]) / 2}
			if d := math.Hypot(m[0]-s.at[0], m[1]-s.at[1]); d < best {
				best, vp.front = d, k
			}
		}
		back := cpt{s.at[0] + n[0]*(setback+hd+3), s.at[1] + n[1]*(setback+hd+3)}
		switch {
		case farm:
			barn := rect(cpt{back[0] + s.dir[0]*(pw*0.22) + n[0]*6, back[1] + s.dir[1]*(pw*0.22) + n[1]*6}, 12, 7, turn)
			v.barns = append(v.barns, barn)
			if r.Float64() < 0.6 {
				v.granaries = append(v.granaries, cpt{back[0] - s.dir[0]*(pw*0.28) + n[0]*8, back[1] - s.dir[1]*(pw*0.28) + n[1]*8})
			}
			for k := 0; k < 2+r.IntN(3); k++ {
				v.haystacks = append(v.haystacks, cpt{back[0] - s.dir[0]*(pw*0.1) + n[0]*(16+float64(k)*3.5), back[1] - s.dir[1]*(pw*0.1) + n[1]*(16+float64(k)*3.5)})
			}
			vp.garden = "yard"
		case r.Float64() < 0.55:
			// Vegetable rows across the garden.
			vp.garden = "veg"
			for f := 2.0; f < depth-hd-setback+3; f += 1.6 {
				c := cpt{back[0] + n[0]*f, back[1] + n[1]*f}
				vp.rows = append(vp.rows, [2]cpt{{c[0] - s.dir[0]*(pw/2-1.5), c[1] - s.dir[1]*(pw/2-1.5)}, {c[0] + s.dir[0]*(pw/2-1.5), c[1] + s.dir[1]*(pw/2-1.5)}})
			}
		case r.Float64() < 0.6:
			vp.garden = "orchard"
			for f := 3.0; f < depth-hd-setback+2; f += 5 {
				for g := -pw/2 + 2.5; g < pw/2-1.5; g += 5 {
					v.orchard = append(v.orchard, cpt{back[0] + n[0]*f + s.dir[0]*g, back[1] + n[1]*f + s.dir[1]*g})
				}
			}
		default:
			vp.garden = "grass"
			if r.Float64() < 0.5 {
				v.barns = append(v.barns, rect(cpt{back[0] + n[0]*(depth*0.4) + s.dir[0]*(pw*0.2), back[1] + n[1]*(depth*0.4) + s.dir[1]*(pw*0.2)}, 4, 3.4, turn))
			}
		}
		v.plots = append(v.plots, vp)
		built++
	}

	// The mill: on the stream's bank by a road, its wheel in the water.
	for _, rd := range v.roads {
		for i := 0; i+1 < len(rd.pts) && len(v.river) > 1; i++ {
			for j := 0; j+1 < len(v.river); j++ {
				if hit, ok := segCross(rd.pts[i], rd.pts[i+1], v.river[j], v.river[j+1]); ok {
					rt := math.Atan2(v.river[j+1][1]-v.river[j][1], v.river[j+1][0]-v.river[j][0])
					v.bridges = append(v.bridges, placed{c: hit, turn: math.Atan2(rd.pts[i+1][1]-rd.pts[i][1], rd.pts[i+1][0]-rd.pts[i][0]), size: v.riverW + 6, flag: rd.width > 5})
					if len(v.wheels) == 0 {
						// The mill a little downstream of the first bridge.
						k := min(j+3, len(v.river)-2)
						q := v.river[k]
						t := math.Atan2(v.river[k+1][1]-q[1], v.river[k+1][0]-q[0])
						nn := cpt{-math.Sin(t), math.Cos(t)}
						mill := rect(cpt{q[0] + nn[0]*(v.riverW/2+5.5), q[1] + nn[1]*(v.riverW/2+5.5)}, 11, 8, t)
						if free(mill, 0.3) {
							v.houses = append(v.houses, mill)
							taken = append(taken, mill)
							v.wheels = append(v.wheels, placed{c: cpt{q[0] + nn[0]*(v.riverW/2-1.2), q[1] + nn[1]*(v.riverW/2-1.2)}, turn: t, size: 3.2})
						}
					}
					_ = rt
				}
			}
		}
	}

	// The fields: Voronoi cells over the land, cut into strips; the ones
	// near the village left as grass, some woods, some pasture.
	step := 46.0
	var sites []cpt
	for y := -H * 1.25; y <= H*1.25; y += step {
		for x := -H * 1.25; x <= H*1.25; x += step {
			sites = append(sites, cpt{x + (r.Float64()-0.5)*step*0.8, y + (r.Float64()-0.5)*step*0.8})
		}
	}
	villageR := 30 + float64(p.Houses)*1.6
	for i, s := range sites {
		poly := []cpt{{-H * 1.4, -H * 1.4}, {H * 1.4, -H * 1.4}, {H * 1.4, H * 1.4}, {-H * 1.4, H * 1.4}}
		for j, q := range sites {
			if j != i {
				poly = clipHalf(poly, cpt{(s[0] + q[0]) / 2, (s[1] + q[1]) / 2}, cpt{q[0] - s[0], q[1] - s[1]})
			}
		}
		cell := shrink(poly, 2.2)
		if cell == nil {
			continue
		}
		cc := centroid(cell)
		dist := math.Hypot(cc[0]-centre[0], cc[1]-centre[1])
		if dist < villageR*0.8 || inSea(cc) {
			continue
		}
		blocked := false
		for _, t := range taken {
			if convexOverlap(cell, t) {
				blocked = true
				break
			}
		}
		if blocked {
			continue
		}
		roll := r.Float64()
		switch {
		case !p.Farms || (roll < 0.17 && dist > villageR*1.4):
			v.wood(cell, r, 0.75, nearWater, onRoad)
		case roll < 0.32:
			v.pasture = append(v.pasture, cell)
			for k := 0; k < 3+r.IntN(9); k++ {
				q := cpt{cc[0] + (r.Float64()-0.5)*step*0.6, cc[1] + (r.Float64()-0.5)*step*0.6}
				if pointIn(q, cell) && !onRoad(q, 2) && !nearWater(q, 2) {
					v.sheep = append(v.sheep, q)
				}
			}
		default:
			v.strips(cell, r)
			if r.Float64() < 0.2 {
				for k := 0; k < 2+r.IntN(4); k++ {
					q := cpt{cc[0] + (r.Float64()-0.5)*step*0.5, cc[1] + (r.Float64()-0.5)*step*0.5}
					if pointIn(q, cell) && !onRoad(q, 2) && !nearWater(q, 2) {
						v.haystacks = append(v.haystacks, q)
					}
				}
			}
		}
		// Hedge trees along the edges now and then.
		for k := range cell {
			if r.Float64() < 0.35 {
				a, b := cell[k], cell[(k+1)%len(cell)]
				t := r.Float64()
				q := cpt{a[0] + (b[0]-a[0])*t, a[1] + (b[1]-a[1])*t}
				if !onRoad(q, 2) && !nearWater(q, 2) {
					v.hedgeTrees = append(v.hedgeTrees, q)
				}
			}
		}
	}

	// Windmills on the open ground, on a field away from the houses.
	for k := 0; k < p.Windmills; k++ {
		for tries := 0; tries < 80; tries++ {
			a := r.Float64() * 2 * math.Pi
			d := villageR + 15 + r.Float64()*60
			c := cpt{centre[0] + math.Cos(a)*d, centre[1] + math.Sin(a)*d}
			if nearWater(c, 12) || onRoad(c, 10) || math.Abs(c[0]) > H*0.9 || math.Abs(c[1]) > H*0.9 {
				continue
			}
			ok := true
			for _, t := range taken {
				if pointIn(c, t) {
					ok = false
				}
			}
			for _, wm := range v.windmills {
				if math.Hypot(wm.c[0]-c[0], wm.c[1]-c[1]) < 50 {
					ok = false
				}
			}
			if !ok {
				continue
			}
			v.windmills = append(v.windmills, placed{c: c, turn: r.Float64() * math.Pi, size: 10})
			taken = append(taken, rect(c, 22, 22, 0))
			break
		}
	}

	// The sea: piers from the shore near the village, boats at them and out.
	if p.Seaside && len(v.shore) > 2 {
		var near []cpt
		for _, q := range v.shore {
			if math.Abs(q[0]) < H && math.Abs(q[1]) < H && math.Hypot(q[0]-centre[0], q[1]-centre[1]) < villageR*1.6 {
				near = append(near, q)
			}
		}
		r.Shuffle(len(near), func(a, b int) { near[a], near[b] = near[b], near[a] })
		for i, q := range near {
			if i >= 2+p.Houses/30 {
				break
			}
			ok := true
			for _, d := range v.docks {
				if math.Hypot(d.c[0]-q[0], d.c[1]-q[1]) < 25 {
					ok = false
				}
			}
			if !ok {
				continue
			}
			from := cpt{q[0] - sd[0]*2, q[1] - sd[1]*2}
			length := 18 + r.Float64()*14
			v.docks = append(v.docks, placed{c: from, turn: seaDir, size: length})
			for _, side := range []float64{-3, 3} {
				if r.Float64() < 0.75 {
					f := length * (0.4 + r.Float64()*0.45)
					v.boats = append(v.boats, placed{c: cpt{from[0] + sd[0]*f + sideOf[0]*side, from[1] + sd[1]*f + sideOf[1]*side}, turn: seaDir + (r.Float64()-0.5)*0.3, size: 6.5})
				}
			}
		}
		for k := 0; k < 3+r.IntN(4); k++ {
			t := (r.Float64() - 0.5) * H * 1.6
			off := shoreOff + 35 + r.Float64()*60
			q := cpt{sd[0]*off + sideOf[0]*t, sd[1]*off + sideOf[1]*t}
			if math.Abs(q[0]) < H && math.Abs(q[1]) < H {
				v.boats = append(v.boats, placed{c: q, turn: r.Float64() * 2 * math.Pi, size: 9 + r.Float64()*4, flag: true})
			}
		}
	}
	if v.pond != nil && r.Float64() < 0.7 {
		pc := centroid(v.pond)
		v.boats = append(v.boats, placed{c: cpt{pc[0] + 3, pc[1] - 2}, turn: r.Float64() * 3, size: 4.5})
	}
	if len(v.river) > 6 {
		k := len(v.river) / 3
		q := v.river[k]
		t := math.Atan2(v.river[k+1][1]-q[1], v.river[k+1][0]-q[0])
		v.boats = append(v.boats, placed{c: cpt{q[0] - math.Sin(t)*v.riverW*0.25, q[1] + math.Cos(t)*v.riverW*0.25}, turn: t, size: 4.5})
	}

	// Carts on the roads.
	for i, rd := range v.roads {
		if len(rd.pts) < 6 || (i > 1 && r.Float64() < 0.5) {
			continue
		}
		for k := 0; k < 1+r.IntN(2); k++ {
			j := 2 + r.IntN(len(rd.pts)-4)
			t := math.Atan2(rd.pts[j+1][1]-rd.pts[j][1], rd.pts[j+1][0]-rd.pts[j][0])
			if r.IntN(2) == 0 {
				t += math.Pi
			}
			nn := cpt{-math.Sin(t) * rd.width * 0.18, math.Cos(t) * rd.width * 0.18}
			if !nearWater(rd.pts[j], 3) {
				v.carts = append(v.carts, placed{c: cpt{rd.pts[j][0] + nn[0], rd.pts[j][1] + nn[1]}, turn: t, size: 3.2, tag: []string{"", "hay", "wood", ""}[r.IntN(4)]})
			}
		}
	}
	// Willows along the stream.
	for i := 2; i+2 < len(v.river); i += 2 {
		if r.Float64() < 0.45 {
			a, b := v.river[i], v.river[i+1]
			t := math.Atan2(b[1]-a[1], b[0]-a[0])
			side := 1.0
			if r.IntN(2) == 0 {
				side = -1
			}
			q := cpt{a[0] - math.Sin(t)*side*(v.riverW/2+3), a[1] + math.Cos(t)*side*(v.riverW/2+3)}
			ok := !onRoad(q, 2) && !inSea(q)
			for _, tk := range taken {
				if pointIn(q, tk) {
					ok = false
				}
			}
			if ok {
				v.trees = append(v.trees, q)
			}
		}
	}
	// Trees about the village, where there is room.
	for k := 0; k < 30+p.Houses/2; k++ {
		a := r.Float64() * 2 * math.Pi
		d := r.Float64() * villageR * 1.1
		q := cpt{centre[0] + math.Cos(a)*d, centre[1] + math.Sin(a)*d}
		if onRoad(q, 3) || nearWater(q, 2) {
			continue
		}
		ok := true
		for _, t := range taken {
			if pointIn(q, scalePoly(t, centroid(t), 1.05)) {
				ok = false
			}
		}
		if ok {
			v.trees = append(v.trees, q)
		}
	}
	return v
}

func (v *Village) wood(cell []cpt, r *rand.Rand, density float64, nearWater func(cpt, float64) bool, onRoad func(cpt, float64) bool) {
	lo, hi := cpt{math.Inf(1), math.Inf(1)}, cpt{math.Inf(-1), math.Inf(-1)}
	for _, q := range cell {
		lo = cpt{math.Min(lo[0], q[0]), math.Min(lo[1], q[1])}
		hi = cpt{math.Max(hi[0], q[0]), math.Max(hi[1], q[1])}
	}
	for y := lo[1]; y < hi[1]; y += 5.5 {
		for x := lo[0]; x < hi[0]; x += 5.5 {
			q := cpt{x + r.Float64()*5.5, y + r.Float64()*5.5}
			if r.Float64() < density && pointIn(q, cell) && !nearWater(q, 1.5) && !onRoad(q, 2) {
				v.woods = append(v.woods, q)
			}
		}
	}
}

// strips cuts a field into a few strips, each its own crop, ploughed.
func (v *Village) strips(cell []cpt, r *rand.Rand) {
	a := longAxis(cell) + (r.Float64()-0.5)*0.4
	d := cpt{math.Cos(a), math.Sin(a)}
	n := cpt{-d[1], d[0]}
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, q := range cell {
		s := q[0]*n[0] + q[1]*n[1]
		lo, hi = math.Min(lo, s), math.Max(hi, s)
	}
	rest := cell
	cut := func(poly []cpt) {
		f := shrink(poly, 0.8)
		if f == nil {
			return
		}
		vf := vfield{poly: f, crop: r.IntN(16)}
		flo, fhi := math.Inf(1), math.Inf(-1)
		for _, q := range f {
			s := q[0]*n[0] + q[1]*n[1]
			flo, fhi = math.Min(flo, s), math.Max(fhi, s)
		}
		for s := flo + 1.6; s < fhi; s += 1.6 {
			var hits []cpt
			for i := range f {
				p0, p1 := f[i], f[(i+1)%len(f)]
				s0, s1 := p0[0]*n[0]+p0[1]*n[1], p1[0]*n[0]+p1[1]*n[1]
				if (s0-s)*(s1-s) < 0 {
					t := (s - s0) / (s1 - s0)
					hits = append(hits, cpt{p0[0] + (p1[0]-p0[0])*t, p0[1] + (p1[1]-p0[1])*t})
				}
			}
			if len(hits) == 2 {
				vf.furrows = append(vf.furrows, [2]cpt{hits[0], hits[1]})
			}
		}
		v.fields = append(v.fields, vf)
	}
	for s := lo + 12 + r.Float64()*10; s < hi-8 && rest != nil; s += 12 + r.Float64()*12 {
		x, y := splitPoly(rest, cpt{n[0] * s, n[1] * s}, d)
		if len(x) >= 3 {
			cut(x)
		}
		rest = y
		if len(rest) < 3 {
			rest = nil
		}
	}
	if rest != nil {
		cut(rest)
	}
}

// SVG draws the village.
func (v *Village) SVG(style string) []byte {
	lk, ok := looks[style]
	if !ok {
		lk = looks["light"]
	}
	S := v.Size / villageSpan
	a := &art{pen: pen{S: S, ox: v.Size / 2, oy: v.Size / 2}, lk: lk}
	w := a.w
	var b strings.Builder
	sz := fmt.Sprintf("%.0f", v.Size)
	b.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" width="` + sz + `" height="` + sz + `" viewBox="0 0 ` + sz + ` ` + sz + `">`)
	b.WriteString(`<rect width="100%" height="100%" fill="` + lk.grass + `"/>`)

	// Fields and pasture.
	for i := 0; i < 16; i++ {
		var fb strings.Builder
		for _, f := range v.fields {
			if f.crop == i {
				fb.WriteString(a.poly(f.poly))
			}
		}
		if fb.Len() > 0 {
			b.WriteString(`<path d="` + fb.String() + `" fill="` + lk.crops[i%len(lk.crops)] + `" stroke="` + lk.furrow + `" stroke-width="` + w(0.25) + `"/>`)
		}
	}
	var furrows strings.Builder
	for _, f := range v.fields {
		for _, l := range f.furrows {
			furrows.WriteString("M" + a.pt(l[0]) + "L" + a.pt(l[1]))
		}
	}
	b.WriteString(`<path d="` + furrows.String() + `" stroke="` + lk.furrow + `" stroke-width="` + w(0.18) + `" stroke-opacity="0.6"/>`)
	var past strings.Builder
	for _, p := range v.pasture {
		past.WriteString(a.poly(p))
	}
	b.WriteString(`<path d="` + past.String() + `" fill="` + lk.tree + `" fill-opacity="0.18" stroke="` + lk.hedge + `" stroke-width="` + w(0.9) + `" stroke-dasharray="0 ` + w(1.3) + `" stroke-linecap="round"/>`)
	var hedges strings.Builder
	for _, f := range v.fields {
		hedges.WriteString(a.poly(f.poly))
	}
	b.WriteString(`<path d="` + hedges.String() + `" fill="none" stroke="` + lk.hedge + `" stroke-opacity="0.5" stroke-width="` + w(0.6) + `" stroke-dasharray="0 ` + w(1.1) + `" stroke-linecap="round"/>`)

	// The sea, with bands of shallows and foam along the shore.
	if len(v.sea) > 2 {
		b.WriteString(`<path d="` + a.poly(v.sea) + `" fill="` + lk.waterDeep + `"/>`)
		b.WriteString(`<clipPath id="mg-vsea"><path d="` + a.poly(v.sea) + `"/></clipPath><g clip-path="url(#mg-vsea)">`)
		for _, band := range []struct {
			wd  float64
			col string
			op  string
		}{{30, lk.water, "1"}, {12, lk.foam, "0.5"}, {3, lk.foam, "0.9"}} {
			b.WriteString(`<path d="` + a.smooth(v.shore) + `" fill="none" stroke="` + band.col + `" stroke-opacity="` + band.op + `" stroke-width="` + w(band.wd) + `"/>`)
		}
		b.WriteString(`</g>`)
		b.WriteString(`<path d="` + a.smooth(v.shore) + `" fill="none" stroke="` + lk.waterInk + `" stroke-width="` + w(0.6) + `"/>`)
	}
	// The stream and the pond.
	if len(v.river) > 1 {
		rv := a.smooth(v.river)
		b.WriteString(`<path d="` + rv + `" fill="none" stroke="` + lk.waterInk + `" stroke-width="` + w(v.riverW+1.2) + `" stroke-linecap="round" stroke-linejoin="round"/>`)
		b.WriteString(`<path d="` + rv + `" fill="none" stroke="` + lk.water + `" stroke-width="` + w(v.riverW) + `" stroke-linecap="round" stroke-linejoin="round"/>`)
		b.WriteString(`<path d="` + rv + `" fill="none" stroke="` + lk.foam + `" stroke-opacity="0.55" stroke-width="` + w(v.riverW*0.15) + `" stroke-dasharray="` + w(6) + ` ` + w(10) + `" stroke-linecap="round"/>`)
	}
	if v.pond != nil {
		pd := a.smoothClosed(v.pond)
		b.WriteString(`<path d="` + pd + `" fill="` + lk.water + `" stroke="` + lk.waterInk + `" stroke-width="` + w(0.8) + `"/>`)
		b.WriteString(`<path d="` + a.smoothClosed(scalePoly(v.pond, centroid(v.pond), 0.6)) + `" fill="` + lk.waterDeep + `" fill-opacity="0.6"/>`)
	}
	// The green, the churchyard and the plots.
	b.WriteString(`<path d="` + a.smoothClosed(v.green) + `" fill="` + lk.tree + `" fill-opacity="0.3" stroke="` + lk.hedge + `" stroke-opacity="0.5" stroke-width="` + w(0.4) + `"/>`)
	if v.yard != nil {
		b.WriteString(`<path d="` + a.poly(v.yard) + `" fill="` + lk.grass + `" stroke="` + lk.stone + `" stroke-width="` + w(1.1) + `"/>`)
		b.WriteString(`<path d="` + a.poly(v.yard) + `" fill="none" stroke="` + lk.ink + `" stroke-width="` + w(0.25) + `"/>`)
	}
	var plots, fences, rows strings.Builder
	for _, p := range v.plots {
		plots.WriteString(a.poly(p.poly))
		for k := range p.poly {
			if k == p.front {
				continue
			}
			fences.WriteString("M" + a.pt(p.poly[k]) + "L" + a.pt(p.poly[(k+1)%len(p.poly)]))
		}
		for _, l := range p.rows {
			rows.WriteString("M" + a.pt(l[0]) + "L" + a.pt(l[1]))
		}
	}
	b.WriteString(`<path d="` + plots.String() + `" fill="` + lk.ground + `" fill-opacity="0.7"/>`)
	b.WriteString(`<path d="` + rows.String() + `" stroke="` + lk.hedge + `" stroke-width="` + w(0.55) + `" stroke-dasharray="` + w(0.6) + ` ` + w(0.5) + `"/>`)
	b.WriteString(`<path d="` + fences.String() + `" fill="none" stroke="` + lk.wood + `" stroke-width="` + w(0.3) + `"/>`)
	b.WriteString(`<path d="` + fences.String() + `" fill="none" stroke="` + lk.ink + `" stroke-width="` + w(0.5) + `" stroke-dasharray="0 ` + w(2.2) + `" stroke-linecap="round"/>`)
	// The roads: edge, surface, ruts.
	for _, rd := range v.roads {
		d := a.smooth(rd.pts)
		b.WriteString(`<path d="` + d + `" fill="none" stroke="` + lk.roadEdge + `" stroke-width="` + w(rd.width+1) + `" stroke-linecap="round" stroke-linejoin="round"/>`)
		b.WriteString(`<path d="` + d + `" fill="none" stroke="` + lk.road + `" stroke-width="` + w(rd.width) + `" stroke-linecap="round" stroke-linejoin="round"/>`)
	}
	for _, rd := range v.roads {
		for _, off := range []float64{-rd.width * 0.22, rd.width * 0.22} {
			b.WriteString(`<path d="` + a.smooth(offsetLine(rd.pts, off)) + `" fill="none" stroke="` + lk.roadEdge + `" stroke-opacity="0.5" stroke-width="` + w(0.25) + `" stroke-dasharray="` + w(4) + ` ` + w(2) + `"/>`)
		}
	}
	for _, br := range v.bridges {
		d := cpt{math.Cos(br.turn), math.Sin(br.turn)}
		n := cpt{-d[1], d[0]}
		wd := 4.5
		if br.flag {
			wd = 6.5
		}
		deck := []cpt{{br.c[0] - d[0]*br.size/2 - n[0]*wd/2, br.c[1] - d[1]*br.size/2 - n[1]*wd/2}, {br.c[0] + d[0]*br.size/2 - n[0]*wd/2, br.c[1] + d[1]*br.size/2 - n[1]*wd/2}, {br.c[0] + d[0]*br.size/2 + n[0]*wd/2, br.c[1] + d[1]*br.size/2 + n[1]*wd/2}, {br.c[0] - d[0]*br.size/2 + n[0]*wd/2, br.c[1] - d[1]*br.size/2 + n[1]*wd/2}}
		sh := make([]cpt, 4)
		for i, q := range deck {
			sh[i] = cpt{q[0] + 0.8, q[1] + 1}
		}
		b.WriteString(`<path d="` + a.poly(sh) + `" fill="` + lk.shadow + `" fill-opacity="` + fmt.Sprintf("%.2f", lk.shadowAlpha) + `"/>`)
		b.WriteString(`<path d="` + a.poly(deck) + `" fill="` + lk.stone + `" stroke="` + lk.ink + `" stroke-width="` + w(0.45) + `"/>`)
	}

	for _, d := range v.docks {
		a.dock(d.c, d.turn, d.size, 3)
	}
	for _, h := range v.haystacks {
		a.haystack(h, 1.7)
	}
	for _, h := range v.houses {
		a.building(h, 1.1)
	}
	for _, h := range v.barns {
		a.building(h, 1.2)
	}
	for _, h := range v.church {
		a.building(h, 1.6)
	}
	if v.apse != nil {
		a.round(v.apse.c, v.apse.size, 1)
	}
	for _, g := range v.granaries {
		a.round(g, 2.4, 0.9)
	}
	for i, t := range v.woods {
		a.treeAt(t, 3.6, float64(i)*0.77)
	}
	for i, t := range v.trees {
		a.treeAt(t, 3.4, float64(i)*1.3)
	}
	for i, t := range v.hedgeTrees {
		a.treeAt(t, 2.8, float64(i)*2.1)
	}
	for i, t := range v.orchard {
		a.treeAt(t, 1.9, float64(i)*0.5)
	}
	a.write(&b, 0.35)

	top := &art{pen: a.pen, lk: lk}
	for _, s := range v.sheep {
		top.over.WriteString(`<path d="` + top.circle(cpt{s[0] + 0.4, s[1] + 0.5}, 1.3) + `" fill="` + lk.shadow + `" fill-opacity="0.2"/><path d="` + top.circle(s, 1.25) + `" fill="#f7f4ec" stroke="` + lk.ink + `" stroke-width="` + w(0.25) + `"/><path d="` + top.circle(cpt{s[0] + 1.2, s[1]}, 0.45) + `" fill="` + lk.ink + `"/>`)
	}
	for _, wm := range v.windmills {
		top.windmill(wm.c, wm.size, wm.turn)
	}
	for _, wh := range v.wheels {
		top.waterwheel(wh.c, wh.size, wh.turn)
	}
	for _, bt := range v.boats {
		top.boat(bt.c, bt.size, bt.turn, bt.flag)
	}
	for _, c := range v.carts {
		load := ""
		switch c.tag {
		case "hay":
			load = lk.hay
		case "wood":
			load = lk.wood
		}
		top.cart(c.c, c.size, c.turn, load)
	}
	for _, g := range v.graves {
		top.grave(g.c, g.size, g.flag)
	}
	if v.well != nil {
		top.well(*v.well, 1.8)
	}
	top.write(&b, 0.35)
	b.WriteString(paperGrain(v.Size, 0.06))
	b.WriteString(`</svg>`)
	return []byte(b.String())
}

// offsetLine is a polyline moved sideways by off.
func offsetLine(pts []cpt, off float64) []cpt {
	out := make([]cpt, len(pts))
	for i := range pts {
		a, b := pts[max(0, i-1)], pts[min(len(pts)-1, i+1)]
		d := cpt{b[0] - a[0], b[1] - a[1]}
		l := math.Hypot(d[0], d[1])
		if l == 0 {
			out[i] = pts[i]
			continue
		}
		out[i] = cpt{pts[i][0] - d[1]/l*off, pts[i][1] + d[0]/l*off}
	}
	return out
}

// smoothClosed is a closed loop through points as curves.
func (p pen) smoothClosed(ps []cpt) string {
	n := len(ps)
	if n < 3 {
		return p.poly(ps)
	}
	var b strings.Builder
	b.WriteString("M" + p.pt(ps[0]))
	for i := 0; i < n; i++ {
		p0, p1, p2, p3 := ps[(i-1+n)%n], ps[i], ps[(i+1)%n], ps[(i+2)%n]
		c1 := cpt{p1[0] + (p2[0]-p0[0])/6, p1[1] + (p2[1]-p0[1])/6}
		c2 := cpt{p2[0] - (p3[0]-p1[0])/6, p2[1] - (p3[1]-p1[1])/6}
		b.WriteString("C" + p.pt(c1) + " " + p.pt(c2) + " " + p.pt(p2))
	}
	b.WriteString("Z")
	return b.String()
}
