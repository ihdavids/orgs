package mapgen

import (
	"fmt"
	"math"
	"math/rand/v2"
	"sort"
	"strings"
)

// A region drawn by hand, in the manner of watabou's Perilous Shores
// (https://watabou.itch.io/perilous-shores - its look, not its code): the
// terrain generator's land and rivers, inked. A heavy coastline with ripples
// round it out to sea; mountains as little peaks, their far sides hatched;
// hills as humps; forests as crowds of round trees; marsh tufts; rivers
// tapering to the sea; a compass rose, a scale and a frame.

var ShoresStyles = []string{"sepia", "ink", "colour"}

type shoresLook struct {
	paper, ink, sea, land, forest, mountain, ripple string
}

var shoresLooks = map[string]shoresLook{
	"sepia":  {"#f1e6cc", "#3b2c1e", "#f1e6cc", "#f1e6cc", "#f1e6cc", "#f1e6cc", "#7d6a52"},
	"ink":    {"#fbfaf6", "#161616", "#fbfaf6", "#fbfaf6", "#fbfaf6", "#fbfaf6", "#5c5c5c"},
	"colour": {"#efe6cf", "#33291f", "#cfe0e2", "#ece3c3", "#cdd9a8", "#ddd1b4", "#6d8a93"},
}

// contours traces the lines where a grid of values crosses a level
// (marching squares), chained into polylines in pixels; cell is the grid's
// spacing.
func contours(g []float64, w, h int, cell, level float64) [][]cpt {
	key := func(p cpt) [2]int { return [2]int{int(math.Round(p[0] * 8)), int(math.Round(p[1] * 8))} }
	type seg struct{ a, b cpt }
	var segs []seg
	at := func(x, y int) float64 { return g[y*w+x] - level }
	lerpP := func(x0, y0 int, v0 float64, x1, y1 int, v1 float64) cpt {
		t := v0 / (v0 - v1)
		return cpt{(float64(x0) + (float64(x1)-float64(x0))*t) * cell, (float64(y0) + (float64(y1)-float64(y0))*t) * cell}
	}
	for y := 0; y+1 < h; y++ {
		for x := 0; x+1 < w; x++ {
			a, b, c, d := at(x, y), at(x+1, y), at(x+1, y+1), at(x, y+1)
			var pts []cpt
			if (a > 0) != (b > 0) {
				pts = append(pts, lerpP(x, y, a, x+1, y, b))
			}
			if (b > 0) != (c > 0) {
				pts = append(pts, lerpP(x+1, y, b, x+1, y+1, c))
			}
			if (c > 0) != (d > 0) {
				pts = append(pts, lerpP(x+1, y+1, c, x, y+1, d))
			}
			if (d > 0) != (a > 0) {
				pts = append(pts, lerpP(x, y+1, d, x, y, a))
			}
			if len(pts) == 2 {
				segs = append(segs, seg{pts[0], pts[1]})
			} else if len(pts) == 4 {
				segs = append(segs, seg{pts[0], pts[1]}, seg{pts[2], pts[3]})
			}
		}
	}
	// Chain the pieces end to end.
	ends := map[[2]int][]int{}
	for i, s := range segs {
		ends[key(s.a)] = append(ends[key(s.a)], i)
		ends[key(s.b)] = append(ends[key(s.b)], i)
	}
	used := make([]bool, len(segs))
	var out [][]cpt
	for i := range segs {
		if used[i] {
			continue
		}
		used[i] = true
		line := []cpt{segs[i].a, segs[i].b}
		for dir := 0; dir < 2; dir++ {
			for {
				tip := line[len(line)-1]
				next := -1
				for _, j := range ends[key(tip)] {
					if !used[j] {
						next = j
						break
					}
				}
				if next < 0 {
					break
				}
				used[next] = true
				if key(segs[next].a) == key(tip) {
					line = append(line, segs[next].b)
				} else {
					line = append(line, segs[next].a)
				}
			}
			// And the other way from the start.
			for l, r := 0, len(line)-1; l < r; l, r = l+1, r-1 {
				line[l], line[r] = line[r], line[l]
			}
		}
		if len(line) >= 4 {
			out = append(out, line)
		}
	}
	return out
}

func chaikinLine(p []cpt, times int) []cpt {
	for k := 0; k < times; k++ {
		if len(p) < 3 {
			return p
		}
		closed := p[0] == p[len(p)-1]
		out := []cpt{}
		if !closed {
			out = append(out, p[0])
		}
		for i := 0; i+1 < len(p); i++ {
			a, b := p[i], p[i+1]
			out = append(out, cpt{0.75*a[0] + 0.25*b[0], 0.75*a[1] + 0.25*b[1]}, cpt{0.25*a[0] + 0.75*b[0], 0.25*a[1] + 0.75*b[1]})
		}
		if !closed {
			out = append(out, p[len(p)-1])
		} else {
			out = append(out, out[0])
		}
		p = out
	}
	return p
}

// ShoresSVG draws a region in the Perilous Shores manner.
func ShoresSVG(p Params, strokes []Stroke) []byte {
	p = p.Normal()
	t := Generate(p, strokes)
	lk, ok := shoresLooks[p.ShoresStyle]
	if !ok {
		lk = shoresLooks["sepia"]
	}
	r := rand.New(rand.NewPCG(p.Seed, p.Seed^0x51ab3c7d2e9f4a61))
	W, H := float64(t.W), float64(t.H)
	pn := pen{S: 1}
	f := func(v float64) string { return fmt.Sprintf("%.1f", v) }

	// A coarse grid of elevation for the lines.
	cell := 3.0
	gw, gh := int(W/cell)+1, int(H/cell)+1
	g := make([]float64, gw*gh)
	for y := 0; y < gh; y++ {
		for x := 0; x < gw; x++ {
			g[y*gw+x] = t.sample(float64(x)*cell, float64(y)*cell)
		}
	}
	lake := make([]float64, gw*gh)
	for y := 0; y < gh; y++ {
		for x := 0; x < gw; x++ {
			cx, cy := int(float64(x)*cell)/t.Cell, int(float64(y)*cell)/t.Cell
			if cx < t.CW && cy < t.CH {
				lake[y*gw+x] = float64(t.Lake[cy*t.CW+cx])
			}
		}
	}
	lake = blur(lake, gw, gh, 2)
	path := func(lines [][]cpt, smooth int) string {
		var b strings.Builder
		for _, l := range lines {
			b.WriteString(pn.line(chaikinLine(l, smooth)))
		}
		return b.String()
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`, t.W, t.H, t.W, t.H))
	b.WriteString(`<rect width="100%" height="100%" fill="` + lk.sea + `"/>`)
	lw := math.Max(1, W/900)

	// The land, as the area inside the coast: drawn by filling each pixel
	// row's runs of land, coarsely, then the coast drawn over its edge.
	var land strings.Builder
	for y := 0; y < gh; y++ {
		x := 0
		for x < gw {
			if g[y*gw+x] <= 0 {
				x++
				continue
			}
			x0 := x
			for x < gw && g[y*gw+x] > 0 {
				x++
			}
			land.WriteString(fmt.Sprintf("M%s %sh%sv%sh%sz", f(float64(x0)*cell-cell/2), f(float64(y)*cell-cell/2), f(float64(x-x0)*cell), f(cell), f(-float64(x-x0)*cell)))
		}
	}
	// Ripples out to sea, fainter as they go.
	for i, lv := range []float64{-0.012, -0.03, -0.055, -0.085} {
		op := 0.75 - float64(i)*0.16
		b.WriteString(`<path d="` + path(contours(g, gw, gh, cell, lv), 2) + `" fill="none" stroke="` + lk.ripple + `" stroke-opacity="` + fmt.Sprintf("%.2f", op) + `" stroke-width="` + f(lw*0.9) + `" stroke-linejoin="round"/>`)
	}
	b.WriteString(`<path d="` + land.String() + `" fill="` + lk.land + `"/>`)
	coast := path(contours(g, gw, gh, cell, 0), 2)
	b.WriteString(`<path d="` + coast + `" fill="none" stroke="` + lk.land + `" stroke-width="` + f(cell*1.6) + `"/>`)

	// Lakes.
	lakes := contours(lake, gw, gh, cell, 0.004)
	if len(lakes) > 0 {
		var lp strings.Builder
		for _, l := range lakes {
			lp.WriteString(pn.poly(chaikinLine(l, 2)))
		}
		b.WriteString(`<path d="` + lp.String() + `" fill="` + lk.sea + `" stroke="` + lk.ink + `" stroke-width="` + f(lw*1.4) + `"/>`)
	}

	// Rivers, thin at the source, thick at the mouth.
	var rivers strings.Builder
	riverAt := map[[2]int]bool{}
	for _, rv := range t.Rivers {
		for i := 0; i+1 < len(rv.Pts); i++ {
			wd := math.Max(lw*0.7, math.Min(rv.Width[i]*0.7, lw*4))
			rivers.WriteString(`<path d="M` + f(rv.Pts[i][0]) + ` ` + f(rv.Pts[i][1]) + `L` + f(rv.Pts[i+1][0]) + ` ` + f(rv.Pts[i+1][1]) + `" stroke-width="` + f(wd) + `"/>`)
			riverAt[[2]int{int(rv.Pts[i][0] / 10), int(rv.Pts[i][1] / 10)}] = true
		}
	}
	b.WriteString(`<g fill="none" stroke="` + lk.ink + `" stroke-linecap="round">` + rivers.String() + `</g>`)

	// The glyphs, from the back of the picture to the front, so each
	// covers what is behind it.
	type glyph struct {
		kind string
		x, y float64
		s    float64
	}
	var glyphs []glyph
	unit := math.Max(W, H) / 110
	nearRiver := func(x, y float64) bool {
		for dx := -1; dx <= 1; dx++ {
			for dy := -1; dy <= 1; dy++ {
				if riverAt[[2]int{int(x/10) + dx, int(y/10) + dy}] {
					return true
				}
			}
		}
		return false
	}
	moist := func(x, y float64) float64 {
		cx, cy := int(x)/t.Cell, int(y)/t.Cell
		if cx < 0 || cy < 0 || cx >= t.CW || cy >= t.CH {
			return 0
		}
		return float64(t.Moist[cy*t.CW+cx])
	}
	coastDist := func(x, y float64, rr float64) bool {
		for _, d := range [][2]float64{{rr, 0}, {-rr, 0}, {0, rr}, {0, -rr}} {
			if t.sample(x+d[0], y+d[1]) <= 0 {
				return true
			}
		}
		return false
	}
	// Mountains and hills on a loose grid.
	for y := unit; y < H; y += unit * 1.3 {
		for x := unit; x < W; x += unit * 1.6 {
			jx, jy := x+(r.Float64()-0.5)*unit, y+(r.Float64()-0.5)*unit*0.8
			e := t.sample(jx, jy)
			switch {
			case e > 0.55 && !coastDist(jx, jy, unit):
				glyphs = append(glyphs, glyph{"mountain", jx, jy, unit * (1.9 + (e-0.55)*3) * (0.85 + r.Float64()*0.3)})
			case e > 0.36 && r.Float64() < 0.55 && !nearRiver(jx, jy):
				glyphs = append(glyphs, glyph{"hill", jx, jy, unit * (0.9 + r.Float64()*0.3)})
			case e > 0.02 && e < 0.36 && r.Float64() < 0.22 && !nearRiver(jx, jy) && !coastDist(jx, jy, unit*0.5):
				glyphs = append(glyphs, glyph{"grass", jx, jy, unit * 0.35})
			}
		}
	}
	// Forests where it is wet and not high; marsh where it is wet and low.
	for y := unit * 0.4; y < H; y += unit * 0.5 {
		for x := unit * 0.4; x < W; x += unit * 0.55 {
			jx, jy := x+(r.Float64()-0.5)*unit*0.4, y+(r.Float64()-0.5)*unit*0.4
			e := t.sample(jx, jy)
			m := moist(jx, jy)
			if e <= 0.015 || nearRiver(jx, jy) || coastDist(jx, jy, unit*0.3) {
				continue
			}
			forest := newNoise(p.Seed*3+5).fbm(jx/(unit*9), jy/(unit*9), 3)
			switch {
			case e < 0.55 && m > 0.45 && forest > 0.02-m*0.15:
				glyphs = append(glyphs, glyph{"tree", jx, jy, unit * (0.32 + r.Float64()*0.1)})
			case e < 0.06 && m > 0.75 && r.Float64() < 0.12:
				glyphs = append(glyphs, glyph{"marsh", jx, jy, unit * 0.6})
			}
		}
	}
	sort.Slice(glyphs, func(i, j int) bool { return glyphs[i].y < glyphs[j].y })
	var gl strings.Builder
	ink := `stroke="` + lk.ink + `" stroke-width="` + f(lw) + `" stroke-linejoin="round" stroke-linecap="round"`
	for _, gg := range glyphs {
		x, y, s := gg.x, gg.y, gg.s
		switch gg.kind {
		case "mountain":
			hw, h := s*0.62, s*0.95
			px := x + (r.Float64()-0.5)*s*0.15
			// The body, filled to hide what is behind it.
			gl.WriteString(`<path d="M` + f(x-hw) + ` ` + f(y) + `L` + f(px) + ` ` + f(y-h) + `L` + f(x+hw) + ` ` + f(y) + `Z" fill="` + lk.mountain + `" stroke="none"/>`)
			gl.WriteString(`<path d="M` + f(px) + ` ` + f(y-h) + `L` + f(x+hw) + ` ` + f(y) + `L` + f(px+(x-px)*0.25+s*0.05) + ` ` + f(y) + `Z" fill="` + lk.ink + `" fill-opacity="0.08" stroke="none"/>`)
			gl.WriteString(`<path d="M` + f(x-hw) + ` ` + f(y) + `L` + f(px) + ` ` + f(y-h) + `L` + f(x+hw) + ` ` + f(y) + `" fill="none" stroke="` + lk.ink + `" stroke-width="` + f(lw*1.3) + `" stroke-linejoin="round" stroke-linecap="round"/>`)
			// The far side hatched, from the ridge down.
			var hatch strings.Builder
			n := 4 + int(s/unit*1.5)
			for k := 1; k <= n; k++ {
				tt := float64(k) / float64(n+1)
				sx, sy := px+(x+hw-px)*tt, (y-h)+h*tt
				ex, ey := px+(x-px)*0.2+(x+hw-px)*tt*0.55, y-h*0.05*(1-tt)
				hatch.WriteString("M" + f(sx) + " " + f(sy) + "L" + f(ex) + " " + f(ey))
			}
			gl.WriteString(`<path d="` + hatch.String() + `" fill="none" stroke="` + lk.ink + `" stroke-width="` + f(lw*0.6) + `" stroke-linecap="round"/>`)
			// A ridge down the middle.
			gl.WriteString(`<path d="M` + f(px) + ` ` + f(y-h) + `L` + f(px+(x-px)*0.3+s*0.05) + ` ` + f(y-h*0.45) + `" fill="none" stroke="` + lk.ink + `" stroke-width="` + f(lw*0.7) + `"/>`)
		case "hill":
			hw, h := s*0.55, s*0.32
			gl.WriteString(`<path d="M` + f(x-hw) + ` ` + f(y) + `Q` + f(x) + ` ` + f(y-h*2) + ` ` + f(x+hw) + ` ` + f(y) + `" fill="` + lk.land + `" ` + ink + `/>`)
			gl.WriteString(`<path d="M` + f(x+hw*0.25) + ` ` + f(y-h*0.6) + `L` + f(x+hw*0.1) + ` ` + f(y-h*0.1) + `M` + f(x+hw*0.55) + ` ` + f(y-h*0.35) + `L` + f(x+hw*0.42) + ` ` + f(y-h*0.02) + `" fill="none" stroke="` + lk.ink + `" stroke-width="` + f(lw*0.6) + `"/>`)
		case "tree":
			rr := s
			gl.WriteString(`<path d="M` + f(x) + ` ` + f(y) + `L` + f(x) + ` ` + f(y-rr*0.9) + `" ` + ink + `/>`)
			gl.WriteString(`<circle cx="` + f(x) + `" cy="` + f(y-rr*1.35) + `" r="` + f(rr) + `" fill="` + lk.forest + `" ` + ink + `/>`)
			gl.WriteString(`<path d="M` + f(x+rr*0.15) + ` ` + f(y-rr*0.55) + `A` + f(rr) + ` ` + f(rr) + ` 0 0 0 ` + f(x+rr*0.95) + ` ` + f(y-rr*1.2) + `" fill="none" stroke="` + lk.ink + `" stroke-width="` + f(lw*0.6) + `"/>`)
		case "grass":
			gl.WriteString(`<path d="M` + f(x-s*0.4) + ` ` + f(y) + `L` + f(x-s*0.55) + ` ` + f(y-s*0.6) + `M` + f(x) + ` ` + f(y) + `L` + f(x) + ` ` + f(y-s*0.8) + `M` + f(x+s*0.4) + ` ` + f(y) + `L` + f(x+s*0.55) + ` ` + f(y-s*0.6) + `" fill="none" stroke="` + lk.ink + `" stroke-opacity="0.7" stroke-width="` + f(lw*0.55) + `" stroke-linecap="round"/>`)
		case "marsh":
			var m strings.Builder
			for k := -1; k <= 1; k++ {
				mx := x + float64(k)*s*0.5
				m.WriteString("M" + f(mx-s*0.2) + " " + f(y) + "L" + f(mx+s*0.2) + " " + f(y))
				m.WriteString("M" + f(mx) + " " + f(y) + "L" + f(mx) + " " + f(y-s*0.35))
				m.WriteString("M" + f(mx) + " " + f(y) + "L" + f(mx-s*0.15) + " " + f(y-s*0.25))
				m.WriteString("M" + f(mx) + " " + f(y) + "L" + f(mx+s*0.15) + " " + f(y-s*0.25))
			}
			gl.WriteString(`<path d="` + m.String() + `" fill="none" stroke="` + lk.ink + `" stroke-width="` + f(lw*0.6) + `" stroke-linecap="round"/>`)
		}
	}
	b.WriteString(gl.String())
	// The coast in ink, over everything on the land's edge.
	b.WriteString(`<path d="` + coast + `" fill="none" stroke="` + lk.ink + `" stroke-width="` + f(lw*2.2) + `" stroke-linejoin="round" stroke-linecap="round"/>`)

	// The compass rose, out at sea in the emptiest corner.
	best, bx, by := math.Inf(1), W*0.1, H*0.12
	for _, c := range [][2]float64{{0.1, 0.13}, {0.9, 0.13}, {0.1, 0.85}, {0.9, 0.85}} {
		x, y := W*c[0], H*c[1]
		sum := 0.0
		for dx := -2; dx <= 2; dx++ {
			for dy := -2; dy <= 2; dy++ {
				sum += math.Max(0, t.sample(x+float64(dx)*W*0.02, y+float64(dy)*H*0.02))
			}
		}
		if sum < best {
			best, bx, by = sum, x, y
		}
	}
	R := math.Min(W, H) * 0.06
	var rose strings.Builder
	rose.WriteString(`<circle cx="` + f(bx) + `" cy="` + f(by) + `" r="` + f(R*0.72) + `" fill="` + lk.paper + `" stroke="` + lk.ink + `" stroke-width="` + f(lw) + `"/>`)
	rose.WriteString(`<circle cx="` + f(bx) + `" cy="` + f(by) + `" r="` + f(R*0.62) + `" fill="none" stroke="` + lk.ink + `" stroke-width="` + f(lw*0.5) + `"/>`)
	for i := 0; i < 8; i++ {
		a := float64(i)*math.Pi/4 - math.Pi/2
		long := R
		if i%2 == 1 {
			long = R * 0.6
		}
		tip := cpt{bx + math.Cos(a)*long, by + math.Sin(a)*long}
		l := cpt{bx + math.Cos(a-math.Pi/2)*R*0.12, by + math.Sin(a-math.Pi/2)*R*0.12}
		rr := cpt{bx + math.Cos(a+math.Pi/2)*R*0.12, by + math.Sin(a+math.Pi/2)*R*0.12}
		rose.WriteString(`<path d="M` + f(l[0]) + ` ` + f(l[1]) + `L` + f(tip[0]) + ` ` + f(tip[1]) + `L` + f(bx) + ` ` + f(by) + `Z" fill="` + lk.paper + `" stroke="` + lk.ink + `" stroke-width="` + f(lw*0.7) + `"/>`)
		rose.WriteString(`<path d="M` + f(rr[0]) + ` ` + f(rr[1]) + `L` + f(tip[0]) + ` ` + f(tip[1]) + `L` + f(bx) + ` ` + f(by) + `Z" fill="` + lk.ink + `"/>`)
	}
	rose.WriteString(`<text x="` + f(bx) + `" y="` + f(by-R*1.12) + `" font-family="Georgia, serif" font-size="` + f(R*0.38) + `" font-weight="700" text-anchor="middle" fill="` + lk.ink + `">N</text>`)
	b.WriteString(rose.String())

	// A scale, and the frame.
	sx, sy, sl := W*0.06, H*0.94, W*0.16
	var scale strings.Builder
	for i := 0; i < 4; i++ {
		fill := lk.ink
		if i%2 == 1 {
			fill = lk.paper
		}
		scale.WriteString(`<rect x="` + f(sx+sl/4*float64(i)) + `" y="` + f(sy) + `" width="` + f(sl/4) + `" height="` + f(lw*4) + `" fill="` + fill + `" stroke="` + lk.ink + `" stroke-width="` + f(lw*0.6) + `"/>`)
	}
	scale.WriteString(`<text x="` + f(sx) + `" y="` + f(sy-lw*3) + `" font-family="Georgia, serif" font-size="` + f(W/80) + `" fill="` + lk.ink + `">0</text>`)
	scale.WriteString(`<text x="` + f(sx+sl) + `" y="` + f(sy-lw*3) + `" font-family="Georgia, serif" font-size="` + f(W/80) + `" text-anchor="end" fill="` + lk.ink + `">20 miles</text>`)
	b.WriteString(scale.String())
	in := math.Min(W, H) * 0.012
	b.WriteString(`<rect x="` + f(in) + `" y="` + f(in) + `" width="` + f(W-2*in) + `" height="` + f(H-2*in) + `" fill="none" stroke="` + lk.ink + `" stroke-width="` + f(lw*2.4) + `"/>`)
	b.WriteString(`<rect x="` + f(in*1.8) + `" y="` + f(in*1.8) + `" width="` + f(W-3.6*in) + `" height="` + f(H-3.6*in) + `" fill="none" stroke="` + lk.ink + `" stroke-width="` + f(lw*0.7) + `"/>`)
	b.WriteString(paperGrain(math.Max(W, H), 0.05))
	b.WriteString(`</svg>`)
	return []byte(b.String())
}
