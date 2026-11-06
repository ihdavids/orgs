package jcanvas

// Geometry shared by the SVG drawing here and worg's board: which side an
// edge leaves from when the canvas does not say, where on that side, the
// curve between, which group a card sits in, and where a new card goes.
// Twin: worg/src/canvas.ts (autoSides, anchor, edgeCurve, groupOf, place) -
// the same cases are in geom_test.go and canvas.test.ts.

import (
	"math"
	"sort"
)

type Pt struct{ X, Y float64 }

type Rect struct{ X, Y, W, H float64 }

func (n Node) Rect() Rect { return Rect{n.X, n.Y, n.Width, n.Height} }

func (r Rect) Center() Pt { return Pt{r.X + r.W/2, r.Y + r.H/2} }

// Contains: b lies wholly inside r (edges may touch).
func (r Rect) Contains(b Rect) bool {
	return b.X >= r.X && b.Y >= r.Y && b.X+b.W <= r.X+r.W && b.Y+b.H <= r.Y+r.H
}

func (r Rect) Overlaps(b Rect, gap float64) bool {
	return r.X < b.X+b.W+gap && b.X < r.X+r.W+gap && r.Y < b.Y+b.H+gap && b.Y < r.Y+r.H+gap
}

// AutoSides picks the sides an edge leaves and arrives by when the canvas
// names neither: the sides that face each other, along whichever axis the
// centres are further apart on (horizontal wins a tie).
func AutoSides(a, b Rect) (string, string) {
	ca, cb := a.Center(), b.Center()
	dx, dy := cb.X-ca.X, cb.Y-ca.Y
	if math.Abs(dx) >= math.Abs(dy) {
		if dx >= 0 {
			return "right", "left"
		}
		return "left", "right"
	}
	if dy >= 0 {
		return "bottom", "top"
	}
	return "top", "bottom"
}

// Anchor is the middle of one side.
func Anchor(r Rect, side string) Pt {
	switch side {
	case "top":
		return Pt{r.X + r.W/2, r.Y}
	case "bottom":
		return Pt{r.X + r.W/2, r.Y + r.H}
	case "left":
		return Pt{r.X, r.Y + r.H/2}
	default:
		return Pt{r.X + r.W, r.Y + r.H/2}
	}
}

func normal(side string) Pt {
	switch side {
	case "top":
		return Pt{0, -1}
	case "bottom":
		return Pt{0, 1}
	case "left":
		return Pt{-1, 0}
	default:
		return Pt{1, 0}
	}
}

// Curve is an edge as a cubic bezier: P1 to P2 with controls C1, C2.
type Curve struct {
	P1, C1, C2, P2 Pt
	FromSide       string
	ToSide         string
}

// EdgeCurve works out an edge's curve. A side the edge names is used; one it
// does not is the facing side. The controls stand out from each side by half
// the distance between the ends, kept between 30 and 250.
func EdgeCurve(from, to Rect, fromSide, toSide string) Curve {
	af, at := AutoSides(from, to)
	if fromSide == "" {
		fromSide = af
	}
	if toSide == "" {
		toSide = at
	}
	p1, p2 := Anchor(from, fromSide), Anchor(to, toSide)
	d := math.Hypot(p2.X-p1.X, p2.Y-p1.Y) / 2
	d = math.Max(30, math.Min(250, d))
	n1, n2 := normal(fromSide), normal(toSide)
	return Curve{
		P1: p1, P2: p2,
		C1:       Pt{p1.X + n1.X*d, p1.Y + n1.Y*d},
		C2:       Pt{p2.X + n2.X*d, p2.Y + n2.Y*d},
		FromSide: fromSide, ToSide: toSide,
	}
}

// Mid is the curve's point at t = 0.5, where its label goes.
func (c Curve) Mid() Pt {
	return Pt{(c.P1.X + 3*c.C1.X + 3*c.C2.X + c.P2.X) / 8, (c.P1.Y + 3*c.C1.Y + 3*c.C2.Y + c.P2.Y) / 8}
}

// GroupOf is the index of the smallest group wholly holding node i, or -1.
func (c *Canvas) GroupOf(i int) int {
	best, area := -1, math.Inf(1)
	r := c.Nodes[i].Rect()
	for j, g := range c.Nodes {
		if j == i || g.Type != "group" {
			continue
		}
		gr := g.Rect()
		if !gr.Contains(r) {
			continue
		}
		a := gr.W * gr.H
		// Two groups the same size: the earlier one holds the later, never
		// both each other.
		if c.Nodes[i].Type == "group" && a == r.W*r.H && j > i {
			continue
		}
		if a < area {
			best, area = j, a
		}
	}
	return best
}

// Bounds is the box around every node (zero for an empty canvas).
func (c *Canvas) Bounds() Rect {
	if len(c.Nodes) == 0 {
		return Rect{}
	}
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for _, n := range c.Nodes {
		minX, minY = math.Min(minX, n.X), math.Min(minY, n.Y)
		maxX, maxY = math.Max(maxX, n.X+n.Width), math.Max(maxY, n.Y+n.Height)
	}
	return Rect{minX, minY, maxX - minX, maxY - minY}
}

// Gap is the space Place leaves between cards.
const Gap = 40

// Place finds room for a w×h card: inside the group `within` when there is
// room there (index, -1 for none), else on the canvas. It reads rows from
// the top left of the space, left to right, a card's width plus Gap at a
// time, and takes the first spot that touches no card (groups count only
// when placing outside one, so a card is never dropped half over a group).
// The rows are as wide as the canvas or four cards, whichever is wider, so
// cards added one after another make a grid rather than a line.
func (c *Canvas) Place(w, h float64, within int) Pt {
	if len(c.Nodes) == 0 {
		return Pt{0, 0}
	}
	var area Rect
	inGroup := within >= 0 && within < len(c.Nodes)
	if inGroup {
		g := c.Nodes[within].Rect()
		area = Rect{g.X + Gap/2, g.Y + Gap/2, g.W - Gap, g.H - Gap}
	} else {
		area = c.Bounds()
		if area.W < 4*(w+Gap) {
			area.W = 4 * (w + Gap)
		}
	}
	free := func(r Rect) bool {
		for i, n := range c.Nodes {
			if inGroup && i == within {
				continue
			}
			if inGroup && n.Type == "group" && n.Rect().Contains(c.Nodes[within].Rect()) {
				continue
			}
			if r.Overlaps(n.Rect(), Gap/2) {
				return false
			}
		}
		return true
	}
	stepX, stepY := w+Gap, h+Gap
	cols := int(math.Max(1, math.Floor((area.W+Gap)/stepX)))
	rows := 1000
	if inGroup {
		rows = int(math.Floor((area.H + Gap) / stepY))
	}
	for row := 0; row < rows; row++ {
		for col := 0; col < cols; col++ {
			r := Rect{area.X + float64(col)*stepX, area.Y + float64(row)*stepY, w, h}
			if free(r) {
				return Pt{r.X, r.Y}
			}
		}
	}
	if inGroup {
		return c.Place(w, h, -1)
	}
	b := c.Bounds()
	return Pt{b.X, b.Y + b.H + Gap}
}

// Order is the nodes in reading order: top to bottom, then left to right,
// by their top left corner. The outline and the CLI use it.
func (c *Canvas) Order(idx []int) {
	sort.SliceStable(idx, func(a, b int) bool {
		na, nb := c.Nodes[idx[a]], c.Nodes[idx[b]]
		if math.Abs(na.Y-nb.Y) > 20 {
			return na.Y < nb.Y
		}
		return na.X < nb.X
	})
}
