package mapgen

import (
	"fmt"
	"math"
	"strings"
)

// Drawing towns and villages as a mapmaker would: roofs with a ridge and a
// lit and a shaded side, a shadow under every building, trees with a crown
// and a highlight, and the things that make a place lived in - windmills,
// water wheels, granaries, boats, docks, carts, haystacks, market stalls,
// graves. Everything is drawn into a few shared paths (one for all shadows,
// one for all lit roof sides...), so a town of thousands of buildings is a
// handful of SVG elements.

// pen turns map units into picture pixels and writes path data.
type pen struct {
	S, ox, oy float64
}

func (p pen) X(v cpt) (float64, float64) { return p.ox + v[0]*p.S, p.oy + v[1]*p.S }

func (p pen) pt(v cpt) string {
	x, y := p.X(v)
	return fmt.Sprintf("%.1f %.1f", x, y)
}

func (p pen) poly(ps []cpt) string {
	if len(ps) < 2 {
		return ""
	}
	var b strings.Builder
	for i, v := range ps {
		if i == 0 {
			b.WriteString("M" + p.pt(v))
		} else {
			b.WriteString("L" + p.pt(v))
		}
	}
	b.WriteString("Z")
	return b.String()
}

func (p pen) line(ps []cpt) string {
	if len(ps) < 2 {
		return ""
	}
	var b strings.Builder
	for i, v := range ps {
		if i == 0 {
			b.WriteString("M" + p.pt(v))
		} else {
			b.WriteString("L" + p.pt(v))
		}
	}
	return b.String()
}

// smooth is a polyline through points as curves (Catmull-Rom).
func (p pen) smooth(ps []cpt) string {
	if len(ps) < 3 {
		return p.line(ps)
	}
	var b strings.Builder
	b.WriteString("M" + p.pt(ps[0]))
	for i := 0; i+1 < len(ps); i++ {
		p0, p1, p2, p3 := ps[max(0, i-1)], ps[i], ps[i+1], ps[min(len(ps)-1, i+2)]
		c1 := cpt{p1[0] + (p2[0]-p0[0])/6, p1[1] + (p2[1]-p0[1])/6}
		c2 := cpt{p2[0] - (p3[0]-p1[0])/6, p2[1] - (p3[1]-p1[1])/6}
		b.WriteString("C" + p.pt(c1) + " " + p.pt(c2) + " " + p.pt(p2))
	}
	return b.String()
}

// circle is a circle as path data, so many share one element.
func (p pen) circle(c cpt, r float64) string {
	x, y := p.X(c)
	R := r * p.S
	return fmt.Sprintf("M%.1f %.1fa%.2f %.2f 0 1 0 %.2f 0a%.2f %.2f 0 1 0 %.2f 0", x-R, y, R, R, 2*R, R, R, -2*R)
}

func (p pen) w(units float64) string { return fmt.Sprintf("%.2f", units*p.S) }

// A place's look: its paper, ink, roofs, water, greens and the rest.
type look struct {
	paper, ground, ink, shadow       string
	roofLit, roofShade, ridge         string
	water, waterDeep, waterInk, foam string
	tree, treeDark, treeLit, grass    string
	crops                             []string
	furrow, hedge, road, roadEdge     string
	wood, sail, stone, hay            string
	plaza                             string
	shadowAlpha                       float64
}

var looks = map[string]look{
	"light": {
		paper: "#ede5d3", ground: "#e4dac4", ink: "#2c231b", shadow: "#3b2e22",
		roofLit: "#d9b38c", roofShade: "#b98d68", ridge: "#5b4130",
		water: "#a8c2cc", waterDeep: "#8eaebb", waterInk: "#56727e", foam: "#d4e3e6",
		tree: "#8fa268", treeDark: "#5f7342", treeLit: "#b5c48a", grass: "#dfe0bd",
		crops: []string{"#e3d39a", "#d7c98c", "#c9cf92", "#b9c584", "#d9c3a0", "#cdbb89"},
		furrow: "#a99b6d", hedge: "#7d915a", road: "#f1ebdc", roadEdge: "#b8ab90",
		wood: "#8a6845", sail: "#f4efe4", stone: "#b7ad9e", hay: "#e0c36c", plaza: "#ebe3cf",
		shadowAlpha: 0.22,
	},
	"parchment": {
		paper: "#ead7a8", ground: "#e0cb98", ink: "#3a2512", shadow: "#4a3216",
		roofLit: "#cf9e6a", roofShade: "#a87749", ridge: "#5a3a1c",
		water: "#b9c2a0", waterDeep: "#a2ad8a", waterInk: "#6b6845", foam: "#d6d4b4",
		tree: "#9a9658", treeDark: "#6a6534", treeLit: "#bdb878", grass: "#dccc95",
		crops: []string{"#e2c983", "#d7bd78", "#c7c27e", "#cdb577", "#d9b98a"},
		furrow: "#ae9560", hedge: "#867f45", road: "#f0e0b6", roadEdge: "#b49a66",
		wood: "#7d5630", sail: "#f2e6c4", stone: "#b4a07c", hay: "#dcb75c", plaza: "#e9d7a8",
		shadowAlpha: 0.2,
	},
	"ink": {
		paper: "#fbfaf6", ground: "#f4f2ec", ink: "#141414", shadow: "#141414",
		roofLit: "#ffffff", roofShade: "#d9d9d9", ridge: "#141414",
		water: "#e2ecf0", waterDeep: "#d2e1e7", waterInk: "#2a3b46", foam: "#f2f7f8",
		tree: "#e4e6dc", treeDark: "#707070", treeLit: "#ffffff", grass: "#f6f5ef",
		crops: []string{"#f7f6f0", "#efeee6", "#f3f1ea"},
		furrow: "#b7b4aa", hedge: "#6e6e6e", road: "#ffffff", roadEdge: "#8c8c8c",
		wood: "#8c8c8c", sail: "#ffffff", stone: "#cfcfcf", hay: "#e8e4d4", plaza: "#f7f6f1",
		shadowAlpha: 0.14,
	},
	"dark": {
		paper: "#1e1c19", ground: "#272420", ink: "#0b0a08", shadow: "#000000",
		roofLit: "#8a7660", roofShade: "#6a5a49", ridge: "#2a221a",
		water: "#20374a", waterDeep: "#1a2d3e", waterInk: "#5f87a3", foam: "#2c4a60",
		tree: "#3f5233", treeDark: "#26331f", treeLit: "#5a7048", grass: "#2b2c22",
		crops: []string{"#3a3526", "#36352a", "#403a2a", "#33372a"},
		furrow: "#4a4433", hedge: "#33422a", road: "#3a3630", roadEdge: "#141210",
		wood: "#6e5236", sail: "#bdb6a6", stone: "#5e574c", hay: "#7d6a3c", plaza: "#36322c",
		shadowAlpha: 0.4,
	},
}

// art collects a place's drawing in layers, each a path or a run of
// elements, written out in order.
type art struct {
	pen
	lk                                  look
	shadows, roofLit, roofShade, ridges strings.Builder
	outlines                            strings.Builder
	treeShadow, treeDark, tree, treeLit strings.Builder
	over                                strings.Builder // things drawn on top of the roofs
	under                               strings.Builder // things drawn under the buildings, over the ground
}

// building draws a building on its lot: a shadow cast down and right, then
// a roof whose ridge runs the long way, one side lit and one in shade.
func (a *art) building(poly []cpt, shadow float64) {
	if len(poly) < 3 {
		return
	}
	// The shadow: the outline pushed down and right.
	sh := make([]cpt, len(poly))
	for i, v := range poly {
		sh[i] = cpt{v[0] + shadow, v[1] + shadow*1.2}
	}
	a.shadows.WriteString(a.poly(sh))
	// The long way is along the longest side.
	long, li := -1.0, 0
	for i := range poly {
		q := poly[(i+1)%len(poly)]
		if l := math.Hypot(q[0]-poly[i][0], q[1]-poly[i][1]); l > long {
			long, li = l, i
		}
	}
	u := cpt{poly[(li+1)%len(poly)][0] - poly[li][0], poly[(li+1)%len(poly)][1] - poly[li][1]}
	ul := math.Hypot(u[0], u[1])
	u = cpt{u[0] / ul, u[1] / ul}
	v := cpt{-u[1], u[0]}
	lo, hi, vlo, vhi := math.Inf(1), math.Inf(-1), math.Inf(1), math.Inf(-1)
	for _, q := range poly {
		pu, pv := q[0]*u[0]+q[1]*u[1], q[0]*v[0]+q[1]*v[1]
		lo, hi, vlo, vhi = math.Min(lo, pu), math.Max(hi, pu), math.Min(vlo, pv), math.Max(vhi, pv)
	}
	vm := (vlo + vhi) / 2
	mid := cpt{u[0]*lo + v[0]*vm, u[1]*lo + v[1]*vm}
	one, two := splitPoly(poly, mid, u)
	// The lit side is the one facing up and left.
	litOne := v[0]+v[1] > 0
	if len(one) >= 3 {
		if litOne {
			a.roofShade.WriteString(a.poly(one))
		} else {
			a.roofLit.WriteString(a.poly(one))
		}
	}
	if len(two) >= 3 {
		if litOne {
			a.roofLit.WriteString(a.poly(two))
		} else {
			a.roofShade.WriteString(a.poly(two))
		}
	}
	// The ridge, stopping short of the ends as a hipped roof's does.
	half := (vhi - vlo) / 2
	r0, r1 := lo+half*0.9, hi-half*0.9
	if r1 > r0 {
		p0 := cpt{u[0]*r0 + v[0]*vm, u[1]*r0 + v[1]*vm}
		p1 := cpt{u[0]*r1 + v[0]*vm, u[1]*r1 + v[1]*vm}
		a.ridges.WriteString("M" + a.pt(p0) + "L" + a.pt(p1))
		// Hips: from the ridge's ends to the corners.
		for _, q := range poly {
			pu := q[0]*u[0] + q[1]*u[1]
			end := p0
			if pu > (r0+r1)/2 {
				end = p1
			}
			if math.Abs(pu-lo) < half*0.6 || math.Abs(pu-hi) < half*0.6 {
				a.ridges.WriteString("M" + a.pt(end) + "L" + a.pt(q))
			}
		}
	}
	a.outlines.WriteString(a.poly(poly))
}

// round draws a round building (a granary, a tower): its shadow, a lit and a
// shaded half, and a cone's lines to its point.
func (a *art) round(c cpt, r float64, shadow float64) {
	a.shadows.WriteString(a.circle(cpt{c[0] + shadow, c[1] + shadow*1.2}, r))
	a.roofShade.WriteString(a.circle(c, r))
	// The lit half: up and left.
	var lit []cpt
	for i := 0; i <= 12; i++ {
		t := math.Pi*0.75 + float64(i)/12*math.Pi
		lit = append(lit, cpt{c[0] + math.Cos(t)*r, c[1] + math.Sin(t)*r})
	}
	a.roofLit.WriteString(a.poly(lit))
	for i := 0; i < 8; i++ {
		t := float64(i) / 8 * 2 * math.Pi
		a.ridges.WriteString("M" + a.pt(c) + "L" + a.pt(cpt{c[0] + math.Cos(t)*r, c[1] + math.Sin(t)*r}))
	}
	a.outlines.WriteString(a.circle(c, r))
}

// treeAt draws a tree: a shadow, a crown of a few overlapping lobes, and a
// highlight up and left.
func (a *art) treeAt(c cpt, r float64, seed float64) {
	a.treeShadow.WriteString(a.circle(cpt{c[0] + r*0.45, c[1] + r*0.55}, r))
	n := 3 + int(math.Abs(math.Sin(seed*12.9898)*43758.5453))%3
	for i := 0; i < n; i++ {
		t := float64(i)/float64(n)*2*math.Pi + seed
		a.tree.WriteString(a.circle(cpt{c[0] + math.Cos(t)*r*0.38, c[1] + math.Sin(t)*r*0.38}, r*0.68))
		a.treeDark.WriteString(a.circle(cpt{c[0] + math.Cos(t)*r*0.38 + r*0.08, c[1] + math.Sin(t)*r*0.38 + r*0.1}, r*0.7))
	}
	a.treeLit.WriteString(a.circle(cpt{c[0] - r*0.25, c[1] - r*0.28}, r*0.32))
}

// windmill: a round tower and four lattice sails, turned to the wind.
func (a *art) windmill(c cpt, s, turn float64) {
	a.round(c, s*0.32, s*0.12)
	var b strings.Builder
	for i := 0; i < 4; i++ {
		t := turn + float64(i)*math.Pi/2
		d := cpt{math.Cos(t), math.Sin(t)}
		n := cpt{-d[1], d[0]}
		root := cpt{c[0] + d[0]*s*0.08, c[1] + d[1]*s*0.08}
		tip := cpt{c[0] + d[0]*s, c[1] + d[1]*s}
		// The sail: a long thin frame to one side of its stock.
		sail := []cpt{
			{root[0] + d[0]*s*0.18, root[1] + d[1]*s*0.18},
			tip,
			{tip[0] + n[0]*s*0.2, tip[1] + n[1]*s*0.2},
			{root[0] + d[0]*s*0.18 + n[0]*s*0.2, root[1] + d[1]*s*0.18 + n[1]*s*0.2},
		}
		b.WriteString(`<path d="` + a.poly(sail) + `" fill="` + a.lk.sail + `" stroke="` + a.lk.ink + `" stroke-width="` + a.w(s*0.025) + `"/>`)
		// Its lattice.
		for k := 1; k < 5; k++ {
			f := 0.18 + float64(k)*0.16
			p := cpt{root[0] + d[0]*s*f, root[1] + d[1]*s*f}
			b.WriteString(`<path d="M` + a.pt(p) + `L` + a.pt(cpt{p[0] + n[0]*s*0.2, p[1] + n[1]*s*0.2}) + `" stroke="` + a.lk.ink + `" stroke-width="` + a.w(s*0.015) + `"/>`)
		}
		b.WriteString(`<path d="M` + a.pt(root) + `L` + a.pt(tip) + `" stroke="` + a.lk.ink + `" stroke-width="` + a.w(s*0.04) + `" stroke-linecap="round"/>`)
	}
	b.WriteString(`<path d="` + a.circle(c, s*0.06) + `" fill="` + a.lk.ink + `"/>`)
	a.over.WriteString(b.String())
}

// waterwheel: a wheel on the bank, its paddles in the water, beside its mill.
func (a *art) waterwheel(c cpt, r, turn float64) {
	var b strings.Builder
	d := cpt{math.Cos(turn), math.Sin(turn)}
	n := cpt{-d[1], d[0]}
	// Seen from above a wheel is a narrow box with paddles across it.
	box := []cpt{
		{c[0] - d[0]*r - n[0]*r*0.22, c[1] - d[1]*r - n[1]*r*0.22},
		{c[0] + d[0]*r - n[0]*r*0.22, c[1] + d[1]*r - n[1]*r*0.22},
		{c[0] + d[0]*r + n[0]*r*0.22, c[1] + d[1]*r + n[1]*r*0.22},
		{c[0] - d[0]*r + n[0]*r*0.22, c[1] - d[1]*r + n[1]*r*0.22},
	}
	b.WriteString(`<path d="` + a.poly(box) + `" fill="` + a.lk.wood + `" stroke="` + a.lk.ink + `" stroke-width="` + a.w(r*0.06) + `"/>`)
	for k := -4; k <= 4; k++ {
		p := cpt{c[0] + d[0]*r*float64(k)/4.5, c[1] + d[1]*r*float64(k)/4.5}
		b.WriteString(`<path d="M` + a.pt(cpt{p[0] - n[0]*r*0.32, p[1] - n[1]*r*0.32}) + `L` + a.pt(cpt{p[0] + n[0]*r*0.32, p[1] + n[1]*r*0.32}) + `" stroke="` + a.lk.ink + `" stroke-width="` + a.w(r*0.05) + `"/>`)
	}
	// The race: white water below the wheel.
	b.WriteString(`<path d="M` + a.pt(cpt{c[0] - d[0]*r*1.6, c[1] - d[1]*r*1.6}) + `L` + a.pt(cpt{c[0] - d[0]*r*1.05, c[1] - d[1]*r*1.05}) + `M` + a.pt(cpt{c[0] + d[0]*r*1.05, c[1] + d[1]*r*1.05}) + `L` + a.pt(cpt{c[0] + d[0]*r*1.8, c[1] + d[1]*r*1.8}) + `" stroke="` + a.lk.foam + `" stroke-width="` + a.w(r*0.35) + `" stroke-linecap="round"/>`)
	a.over.WriteString(b.String())
}

// boat: a hull pointed at the bow, its planks, a mast and (under way) a sail.
func (a *art) boat(c cpt, length, turn float64, sail bool) {
	d := cpt{math.Cos(turn), math.Sin(turn)}
	n := cpt{-d[1], d[0]}
	w := length * 0.34
	at := func(fu, fv float64) cpt { return cpt{c[0] + d[0]*length*fu + n[0]*w*fv, c[1] + d[1]*length*fu + n[1]*w*fv} }
	var hull []cpt
	for i := 0; i <= 10; i++ {
		t := float64(i) / 10
		hull = append(hull, at(-0.5+t, 0.5*math.Sin(math.Pi*math.Pow(t, 0.75))))
	}
	for i := 10; i >= 0; i-- {
		t := float64(i) / 10
		hull = append(hull, at(-0.5+t, -0.5*math.Sin(math.Pi*math.Pow(t, 0.75))))
	}
	var b strings.Builder
	b.WriteString(`<path d="` + a.poly(hull) + `" fill="` + a.lk.wood + `" stroke="` + a.lk.ink + `" stroke-width="` + a.w(length*0.04) + `"/>`)
	b.WriteString(`<path d="M` + a.pt(at(-0.4, 0)) + `L` + a.pt(at(0.35, 0)) + `" stroke="` + a.lk.ink + `" stroke-opacity="0.5" stroke-width="` + a.w(length*0.025) + `"/>`)
	for _, f := range []float64{-0.25, 0.05} {
		b.WriteString(`<path d="M` + a.pt(at(f, -0.42)) + `L` + a.pt(at(f, 0.42)) + `" stroke="` + a.lk.ink + `" stroke-opacity="0.6" stroke-width="` + a.w(length*0.025) + `"/>`)
	}
	if sail {
		s := []cpt{at(0.12, 0), at(-0.32, 0.75), at(-0.2, 0)}
		b.WriteString(`<path d="` + a.poly(s) + `" fill="` + a.lk.sail + `" stroke="` + a.lk.ink + `" stroke-width="` + a.w(length*0.03) + `"/>`)
	}
	// A wake behind it.
	if sail {
		b.WriteString(`<path d="M` + a.pt(at(-0.55, 0.25)) + `L` + a.pt(at(-1.3, 0.6)) + `M` + a.pt(at(-0.55, -0.25)) + `L` + a.pt(at(-1.3, -0.6)) + `" stroke="` + a.lk.foam + `" stroke-width="` + a.w(length*0.05) + `" stroke-linecap="round"/>`)
	}
	a.over.WriteString(b.String())
}

// dock: a pier from the shore out over the water, planked, on posts.
func (a *art) dock(from cpt, turn, length, width float64) {
	d := cpt{math.Cos(turn), math.Sin(turn)}
	n := cpt{-d[1], d[0]}
	at := func(fu, fv float64) cpt { return cpt{from[0] + d[0]*fu + n[0]*fv, from[1] + d[1]*fu + n[1]*fv} }
	var b strings.Builder
	sh := []cpt{at(0, -width/2+width*0.25), at(length+width*0.25, -width/2+width*0.25), at(length+width*0.25, width/2+width*0.3), at(0, width/2+width*0.3)}
	b.WriteString(`<path d="` + a.poly(sh) + `" fill="` + a.lk.shadow + `" fill-opacity="` + fmt.Sprintf("%.2f", a.lk.shadowAlpha) + `"/>`)
	deck := []cpt{at(0, -width/2), at(length, -width/2), at(length, width/2), at(0, width/2)}
	b.WriteString(`<path d="` + a.poly(deck) + `" fill="` + a.lk.wood + `" stroke="` + a.lk.ink + `" stroke-width="` + a.w(width*0.08) + `"/>`)
	for f := width * 0.6; f < length; f += width * 0.45 {
		b.WriteString(`<path d="M` + a.pt(at(f, -width/2)) + `L` + a.pt(at(f, width/2)) + `" stroke="` + a.lk.ink + `" stroke-opacity="0.45" stroke-width="` + a.w(width*0.05) + `"/>`)
	}
	for f := width; f < length; f += width * 1.6 {
		for _, s := range []float64{-0.55, 0.55} {
			b.WriteString(`<path d="` + a.circle(at(f, s*width), width*0.12) + `" fill="` + a.lk.ink + `"/>`)
		}
	}
	a.under.WriteString(b.String())
}

// cart: a box on two wheels with its load, and the horse before it.
func (a *art) cart(c cpt, s, turn float64, load string) {
	d := cpt{math.Cos(turn), math.Sin(turn)}
	n := cpt{-d[1], d[0]}
	at := func(fu, fv float64) cpt { return cpt{c[0] + d[0]*s*fu + n[0]*s*fv, c[1] + d[1]*s*fu + n[1]*s*fv} }
	var b strings.Builder
	box := []cpt{at(-0.5, -0.3), at(0.5, -0.3), at(0.5, 0.3), at(-0.5, 0.3)}
	b.WriteString(`<path d="` + a.poly([]cpt{at(-0.4, -0.2), at(0.6, -0.2), at(0.6, 0.45), at(-0.4, 0.45)}) + `" fill="` + a.lk.shadow + `" fill-opacity="` + fmt.Sprintf("%.2f", a.lk.shadowAlpha) + `"/>`)
	for _, f := range []float64{-0.36, 0.36} {
		b.WriteString(`<path d="M` + a.pt(at(-0.05-0.22, f*1.1)) + `L` + a.pt(at(-0.05+0.22, f*1.1)) + `" stroke="` + a.lk.ink + `" stroke-width="` + a.w(s*0.1) + `" stroke-linecap="round"/>`)
	}
	b.WriteString(`<path d="` + a.poly(box) + `" fill="` + a.lk.wood + `" stroke="` + a.lk.ink + `" stroke-width="` + a.w(s*0.05) + `"/>`)
	if load != "" {
		b.WriteString(`<path d="` + a.poly([]cpt{at(-0.4, -0.2), at(0.4, -0.2), at(0.4, 0.2), at(-0.4, 0.2)}) + `" fill="` + load + `" stroke="` + a.lk.ink + `" stroke-opacity="0.5" stroke-width="` + a.w(s*0.03) + `"/>`)
	}
	// The shafts and the horse.
	b.WriteString(`<path d="M` + a.pt(at(0.5, -0.2)) + `L` + a.pt(at(1.0, -0.15)) + `M` + a.pt(at(0.5, 0.2)) + `L` + a.pt(at(1.0, 0.15)) + `" stroke="` + a.lk.ink + `" stroke-width="` + a.w(s*0.04) + `"/>`)
	horse := []cpt{}
	for i := 0; i < 14; i++ {
		t := float64(i) / 14 * 2 * math.Pi
		horse = append(horse, at(1.25+math.Cos(t)*0.38, math.Sin(t)*0.15))
	}
	b.WriteString(`<path d="` + a.poly(horse) + `" fill="` + a.lk.wood + `" stroke="` + a.lk.ink + `" stroke-width="` + a.w(s*0.04) + `"/>`)
	b.WriteString(`<path d="` + a.circle(at(1.68, 0), s*0.11) + `" fill="` + a.lk.wood + `" stroke="` + a.lk.ink + `" stroke-width="` + a.w(s*0.04) + `"/>`)
	a.over.WriteString(b.String())
}

// haystack: a round stack, lit on one side.
func (a *art) haystack(c cpt, r float64) {
	var b strings.Builder
	b.WriteString(`<path d="` + a.circle(cpt{c[0] + r*0.35, c[1] + r*0.45}, r) + `" fill="` + a.lk.shadow + `" fill-opacity="` + fmt.Sprintf("%.2f", a.lk.shadowAlpha) + `"/>`)
	b.WriteString(`<path d="` + a.circle(c, r) + `" fill="` + a.lk.hay + `" stroke="` + a.lk.ink + `" stroke-width="` + a.w(r*0.12) + `"/>`)
	b.WriteString(`<path d="` + a.circle(cpt{c[0] - r*0.25, c[1] - r*0.25}, r*0.4) + `" fill="#ffffff" fill-opacity="0.25"/>`)
	a.under.WriteString(b.String())
}

// stall: a market stall, its awning striped.
func (a *art) stall(c cpt, s, turn float64, colour string) {
	d := cpt{math.Cos(turn), math.Sin(turn)}
	n := cpt{-d[1], d[0]}
	at := func(fu, fv float64) cpt { return cpt{c[0] + d[0]*s*fu + n[0]*s*fv, c[1] + d[1]*s*fu + n[1]*s*fv} }
	var b strings.Builder
	b.WriteString(`<path d="` + a.poly([]cpt{at(-0.4, -0.25), at(0.6, -0.25), at(0.6, 0.45), at(-0.4, 0.45)}) + `" fill="` + a.lk.shadow + `" fill-opacity="` + fmt.Sprintf("%.2f", a.lk.shadowAlpha) + `"/>`)
	b.WriteString(`<path d="` + a.poly([]cpt{at(-0.5, -0.35), at(0.5, -0.35), at(0.5, 0.35), at(-0.5, 0.35)}) + `" fill="` + colour + `" stroke="` + a.lk.ink + `" stroke-width="` + a.w(s*0.05) + `"/>`)
	for _, f := range []float64{-0.25, 0, 0.25} {
		b.WriteString(`<path d="M` + a.pt(at(f, -0.35)) + `L` + a.pt(at(f, 0.35)) + `" stroke="#ffffff" stroke-opacity="0.6" stroke-width="` + a.w(s*0.09) + `"/>`)
	}
	a.over.WriteString(b.String())
}

// grave: a headstone, or now and then a cross.
func (a *art) grave(c cpt, s float64, cross bool) {
	if cross {
		a.over.WriteString(`<path d="M` + a.pt(cpt{c[0], c[1] - s}) + `L` + a.pt(cpt{c[0], c[1] + s}) + `M` + a.pt(cpt{c[0] - s*0.6, c[1] - s*0.35}) + `L` + a.pt(cpt{c[0] + s*0.6, c[1] - s*0.35}) + `" stroke="` + a.lk.ink + `" stroke-width="` + a.w(s*0.3) + `"/>`)
		return
	}
	a.over.WriteString(`<path d="` + a.poly([]cpt{{c[0] - s*0.5, c[1] - s * 0.3}, {c[0] + s*0.5, c[1] - s*0.3}, {c[0] + s*0.5, c[1] + s*0.3}, {c[0] - s*0.5, c[1] + s*0.3}}) + `" fill="` + a.lk.stone + `" stroke="` + a.lk.ink + `" stroke-width="` + a.w(s*0.15) + `"/>`)
}

// well: a stone ring with water in it.
func (a *art) well(c cpt, r float64) {
	a.over.WriteString(`<path d="` + a.circle(cpt{c[0] + r*0.3, c[1] + r*0.4}, r) + `" fill="` + a.lk.shadow + `" fill-opacity="` + fmt.Sprintf("%.2f", a.lk.shadowAlpha) + `"/>` +
		`<path d="` + a.circle(c, r) + `" fill="` + a.lk.stone + `" stroke="` + a.lk.ink + `" stroke-width="` + a.w(r*0.15) + `"/>` +
		`<path d="` + a.circle(c, r*0.55) + `" fill="` + a.lk.waterDeep + `" stroke="` + a.lk.ink + `" stroke-width="` + a.w(r*0.08) + `"/>`)
}

// write puts the layers out: under, shadows, roofs, ridges, outlines,
// trees, then what stands over the roofs.
func (a *art) write(b *strings.Builder, lineW float64) {
	b.WriteString(a.under.String())
	b.WriteString(`<path d="` + a.treeShadow.String() + `" fill="` + a.lk.shadow + `" fill-opacity="` + fmt.Sprintf("%.2f", a.lk.shadowAlpha*0.9) + `"/>`)
	b.WriteString(`<path d="` + a.shadows.String() + `" fill="` + a.lk.shadow + `" fill-opacity="` + fmt.Sprintf("%.2f", a.lk.shadowAlpha) + `"/>`)
	b.WriteString(`<path d="` + a.roofLit.String() + `" fill="` + a.lk.roofLit + `"/>`)
	b.WriteString(`<path d="` + a.roofShade.String() + `" fill="` + a.lk.roofShade + `"/>`)
	b.WriteString(`<path d="` + a.ridges.String() + `" fill="none" stroke="` + a.lk.ridge + `" stroke-opacity="0.75" stroke-width="` + a.w(lineW*0.7) + `" stroke-linecap="round"/>`)
	b.WriteString(`<path d="` + a.outlines.String() + `" fill="none" stroke="` + a.lk.ink + `" stroke-width="` + a.w(lineW) + `" stroke-linejoin="round"/>`)
	b.WriteString(`<path d="` + a.treeDark.String() + `" fill="` + a.lk.treeDark + `"/>`)
	b.WriteString(`<path d="` + a.tree.String() + `" fill="` + a.lk.tree + `"/>`)
	b.WriteString(`<path d="` + a.treeLit.String() + `" fill="` + a.lk.treeLit + `" fill-opacity="0.7"/>`)
	b.WriteString(a.over.String())
}

// paperGrain is a faint mottle over the whole sheet, like paper's.
func paperGrain(size float64, alpha float64) string {
	sz := fmt.Sprintf("%.0f", size)
	return `<filter id="mg-grain" x="0" y="0" width="100%" height="100%"><feTurbulence type="fractalNoise" baseFrequency="0.85" numOctaves="2" seed="3" stitchTiles="stitch"/><feColorMatrix type="matrix" values="0 0 0 0 0  0 0 0 0 0  0 0 0 0 0  0 0 0 ` + fmt.Sprintf("%.2f", alpha) + ` 0"/></filter>` +
		`<filter id="mg-blot" x="0" y="0" width="100%" height="100%"><feTurbulence type="fractalNoise" baseFrequency="0.006" numOctaves="3" seed="9"/><feColorMatrix type="matrix" values="0 0 0 0 0.35  0 0 0 0 0.25  0 0 0 0 0.12  0 0 0 0.35 -0.12"/></filter>` +
		`<rect width="` + sz + `" height="` + sz + `" filter="url(#mg-blot)"/>` +
		`<rect width="` + sz + `" height="` + sz + `" filter="url(#mg-grain)"/>`
}
