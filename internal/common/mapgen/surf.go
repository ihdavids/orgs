package mapgen

import "math"

// Surf is a region's coast as lines for the map viewer to animate: the
// shore itself (where the foam breaks) and lines of swell further out, each
// a list of polylines in fractions of the picture (x0, y0, x1, y1, ...),
// nearest the shore first. A hex world, a town or a village has none.
func Surf(p Params, strokes []Stroke) [][][]float64 {
	p = p.Normal()
	if p.Generator != "terrain" && p.Generator != "shores" {
		return nil
	}
	t := Generate(p, strokes)
	W, H := float64(t.W), float64(t.H)
	cell := math.Max(4, math.Round(math.Max(W, H)/320))
	gw, gh := int(W/cell)+1, int(H/cell)+1
	g := make([]float64, gw*gh)
	for y := 0; y < gh; y++ {
		for x := 0; x < gw; x++ {
			g[y*gw+x] = t.sample(float64(x)*cell, float64(y)*cell)
		}
	}
	out := [][][]float64{}
	// The shore, then swell from well out to sea in: more lines further out,
	// so a wave can be seen coming a long way before it breaks.
	for _, lv := range []float64{-0.003, -0.012, -0.024, -0.038, -0.055, -0.075, -0.1} {
		var lines [][]float64
		for _, l := range contours(g, gw, gh, cell, lv) {
			if len(l) < 6 {
				continue // a speck of rock, not a coast
			}
			l = chaikinLine(l, 1)
			pts := make([]float64, 0, len(l)*2)
			for _, q := range l {
				pts = append(pts, math.Round(q[0]/W*1e4)/1e4, math.Round(q[1]/H*1e4)/1e4)
			}
			lines = append(lines, pts)
		}
		out = append(out, lines)
	}
	return out
}
