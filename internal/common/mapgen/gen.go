package mapgen

import (
	"container/heap"
	"math"
	"runtime"
	"sort"
	"sync"
)

// Terrain is a map before it is coloured: the elevation of every pixel (sea
// below 0, land 0 to about 1), and on a coarser grid of cells the rain that
// fell, the lakes, and the rivers as lines.
type Terrain struct {
	W, H int
	Elev []float32 // per pixel

	// The coarse grid: Cell pixels a side.
	Cell   int
	CW, CH int
	Moist  []float32 // 0 dry .. 1 wet
	Lake   []float32 // how far under the water the land is, 0 for none
	Rivers []River
}

// River is a line of water, in pixels, getting wider downstream.
type River struct {
	Pts   [][2]float64
	Width []float64
}

func parallelRows(h int, f func(y int)) {
	n := runtime.NumCPU()
	var wg sync.WaitGroup
	rows := make(chan int, h)
	for y := 0; y < h; y++ {
		rows <- y
	}
	close(rows)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for y := range rows {
				f(y)
			}
		}()
	}
	wg.Wait()
}

// Generate makes the terrain of a map.
func Generate(p Params, strokes []Stroke) *Terrain {
	p = p.Normal()
	t := &Terrain{W: p.Width, H: p.Height}
	t.Elev = elevation(p, strokes)
	t.climate(p)
	return t
}

// elevation is the land and the sea. The map's longer side runs from -1 to
// 1, so the shapes are the same at any size.
func elevation(p Params, strokes []Stroke) []float32 {
	W, H := p.Width, p.Height
	n := newNoise(p.Seed)
	ridges := newNoise(p.Seed*7919 + 13)
	coast := newNoise(p.Seed*104729 + 7)
	scale := 2.0 / float64(max(W, H))
	ox, oy := float64(W)/2, float64(H)/2
	// The edge of the map, as a distance: 0 in the middle, 1 at the nearest
	// edge, squarish so an island fills an oblong map.
	ex, ey := float64(W)/2*scale, float64(H)/2*scale
	out := make([]float32, W*H)
	parallelRows(H, func(y int) {
		for x := 0; x < W; x++ {
			nx, ny := (float64(x)-ox)*scale, (float64(y)-oy)*scale
			base := n.fbm(nx*1.5+31.7, ny*1.5-12.3, 6)
			detail := coast.fbm(nx*5+3.1, ny*5+8.4, 4) * (0.06 + 0.3*p.Coast)
			dx, dy := math.Abs(nx)/ex, math.Abs(ny)/ey
			edge := math.Pow(math.Pow(dx, 4)+math.Pow(dy, 4), 0.25)
			e := base*1.25 + detail + (p.Land-0.5)*0.7 + 0.1 - p.Island*smoothstep(0.35, 1.08, edge)*0.95
			if e > 0 {
				r := ridges.ridged(nx*2.2+5.5, ny*2.2-1.7, 5)
				m := math.Pow(r, 1+p.Jagged*2.5)
				e += p.Mountains * m * smoothstep(0.0, 0.3, e) * 1.1
			}
			for _, s := range strokes {
				sx, sy := s.X*float64(W), s.Y*float64(H)
				sr := s.R * float64(W)
				if sr <= 0 {
					continue
				}
				d := math.Hypot(float64(x)-sx, float64(y)-sy) / sr
				if d >= 1 {
					continue
				}
				w := smoothstep(1, 0.15, d)
				switch s.Kind {
				case "mountain":
					e += w * 0.55 * (0.7 + 0.6*ridges.ridged(nx*3, ny*3, 3))
					if e < 0.05 {
						e = lerp(e, 0.2, w)
					}
				case "hill":
					e += w * 0.2
					if e < 0.05 {
						e = lerp(e, 0.12, w)
					}
				case "valley":
					e = lerp(e, 0.06, w*0.85)
				case "land":
					if e < 0.08 {
						e = lerp(e, 0.08, w)
					}
				case "shallow":
					e = lerp(e, -0.06, w)
				case "ocean":
					e = lerp(e, -0.45, w)
				}
			}
			out[y*W+x] = float32(e)
		}
	})
	return out
}

// sample is the elevation at a point, bilinear, clamped to the map.
func (t *Terrain) sample(x, y float64) float64 {
	x = clamp(x, 0, float64(t.W-1))
	y = clamp(y, 0, float64(t.H-1))
	x0, y0 := int(x), int(y)
	x1, y1 := min(x0+1, t.W-1), min(y0+1, t.H-1)
	fx, fy := x-float64(x0), y-float64(y0)
	a := lerp(float64(t.Elev[y0*t.W+x0]), float64(t.Elev[y0*t.W+x1]), fx)
	b := lerp(float64(t.Elev[y1*t.W+x0]), float64(t.Elev[y1*t.W+x1]), fx)
	return lerp(a, b, fy)
}

// climate carries rain on the wind, gathers it into rivers and finds the
// lakes, on the coarse grid.
func (t *Terrain) climate(p Params) {
	t.Cell = max(3, max(t.W, t.H)/320)
	t.CW, t.CH = (t.W+t.Cell-1)/t.Cell, (t.H+t.Cell-1)/t.Cell
	N := t.CW * t.CH
	cx := func(i int) float64 { return (float64(i%t.CW) + 0.5) * float64(t.Cell) }
	cy := func(i int) float64 { return (float64(i/t.CW) + 0.5) * float64(t.Cell) }
	E := make([]float64, N)
	for i := range E {
		E[i] = t.sample(cx(i), cy(i))
	}

	// The wind: every cell takes the air of the cell upwind of it, so the
	// cells are visited upwind first.
	a := p.Wind * math.Pi / 180
	wx, wy := math.Cos(a), math.Sin(a)
	order := make([]int, N)
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(i, j int) bool {
		ai, aj := order[i], order[j]
		return float64(ai%t.CW)*wx+float64(ai/t.CW)*wy < float64(aj%t.CW)*wx+float64(aj/t.CW)*wy
	})
	hum := make([]float64, N)
	rain := make([]float64, N)
	at := func(g []float64, x, y float64, outside float64) float64 {
		if x < 0 || y < 0 || x > float64(t.CW-1) || y > float64(t.CH-1) {
			return outside
		}
		x0, y0 := int(x), int(y)
		x1, y1 := min(x0+1, t.CW-1), min(y0+1, t.CH-1)
		fx, fy := x-float64(x0), y-float64(y0)
		return lerp(lerp(g[y0*t.CW+x0], g[y0*t.CW+x1], fx), lerp(g[y1*t.CW+x0], g[y1*t.CW+x1], fx), fy)
	}
	step := 1.0
	for _, i := range order {
		x, y := float64(i%t.CW), float64(i/t.CW)
		h := at(hum, x-wx*step, y-wy*step, 0.55)
		if E[i] < 0 {
			h = math.Min(1, h+0.04+p.Evaporate*0.12)
			rain[i] = h * 0.1
		} else {
			eu := at(E, x-wx*step, y-wy*step, E[i])
			lift := math.Max(0, E[i]-eu) * float64(t.Cell) * 0.25
			r := h * (0.003 + p.Rain*0.015 + p.Shadow*lift*4)
			r = math.Min(r, h*0.9)
			h -= r
			rain[i] = r
		}
		hum[i] = h
	}
	moist := make([]float64, N)
	for i := range moist {
		moist[i] = 1 - math.Exp(-rain[i]*(120+p.Rain*170))
	}

	// Rivers. The land drains to the sea: a priority flood from the sea
	// raises every hollow to its rim (what it is raised by is a lake), and
	// each cell drains to the cell it was reached from - downhill, all the
	// way to the sea.
	filled := make([]float64, N)
	down := make([]int, N)
	done := make([]bool, N)
	q := &cellHeap{}
	for i := 0; i < N; i++ {
		x, y := i%t.CW, i/t.CW
		down[i] = -1
		if E[i] < 0 || x == 0 || y == 0 || x == t.CW-1 || y == t.CH-1 {
			filled[i] = E[i]
			done[i] = true
			heap.Push(q, cellAt{i, E[i]})
		}
	}
	dirs := [8][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, 1}, {-1, 1}, {1, -1}, {-1, -1}}
	for q.Len() > 0 {
		c := heap.Pop(q).(cellAt)
		x, y := c.i%t.CW, c.i/t.CW
		for _, d := range dirs {
			nx, ny := x+d[0], y+d[1]
			if nx < 0 || ny < 0 || nx >= t.CW || ny >= t.CH {
				continue
			}
			j := ny*t.CW + nx
			if done[j] {
				continue
			}
			done[j] = true
			filled[j] = math.Max(E[j], filled[c.i]+1e-5)
			down[j] = c.i
			heap.Push(q, cellAt{j, filled[j]})
		}
	}
	t.Lake = make([]float32, N)
	for i := range E {
		if E[i] >= 0 && filled[i]-E[i] > 0.015 {
			t.Lake[i] = float32(filled[i] - E[i])
			moist[i] = 1
		}
	}

	byHeight := make([]int, N)
	for i := range byHeight {
		byHeight[i] = i
	}
	sort.Slice(byHeight, func(a, b int) bool { return filled[byHeight[a]] > filled[byHeight[b]] })
	flow := make([]float64, N)
	total := 0.0
	for _, i := range byHeight {
		if E[i] < 0 {
			continue
		}
		flow[i] += rain[i] + 0.002
		total += rain[i] + 0.002
		if d := down[i]; d >= 0 {
			flow[d] += flow[i]
		}
	}
	threshold := total * lerp(0.0015, 0.02, p.Rivers) * 320 / float64(max(t.CW, t.CH))
	// Rivers make the land beside them green; blurred, so it is a broad
	// band along the river and not a line.
	for i := range flow {
		if E[i] >= 0 && flow[i] > threshold {
			moist[i] = math.Min(1, moist[i]+0.35)
		}
	}
	moist = blur(moist, t.CW, t.CH, 3)
	t.Moist = make([]float32, N)
	for i := range moist {
		t.Moist[i] = float32(moist[i])
	}

	// A river is drawn from each place a stream becomes a river, down to the
	// sea or to a river already drawn. Its points are moved a little off the
	// grid's centres, or every river would run in eight directions only.
	isRiver := func(i int) bool { return E[i] >= 0 && flow[i] > threshold && t.Lake[i] == 0 }
	upstream := make([]int, N)
	for i := range flow {
		if isRiver(i) && down[i] >= 0 {
			upstream[down[i]]++
		}
	}
	jit := newNoise(p.Seed*31 + 5)
	pos := func(i int) [2]float64 {
		x, y := cx(i), cy(i)
		c := float64(t.Cell)
		return [2]float64{x + jit.at(x*0.13, y*0.13)*c*0.9, y + jit.at(x*0.13+40, y*0.13+40)*c*0.9}
	}
	drawn := make([]bool, N)
	widthOf := func(i int) float64 {
		return (0.8 + p.RiverWidth*1.6) * math.Pow(flow[i]/threshold, 0.42) * float64(t.Cell) / 4
	}
	for _, i := range byHeight {
		if !isRiver(i) || drawn[i] {
			continue
		}
		r := River{}
		j := i
		for j >= 0 {
			r.Pts = append(r.Pts, pos(j))
			r.Width = append(r.Width, widthOf(j))
			if drawn[j] || E[j] < 0 || t.Lake[j] > 0 {
				break
			}
			drawn[j] = true
			j = down[j]
		}
		if len(r.Pts) >= 3 {
			t.Rivers = append(t.Rivers, chaikin(r, 2))
		}
	}
}

// chaikin rounds a river's corners.
func chaikin(r River, times int) River {
	for k := 0; k < times; k++ {
		if len(r.Pts) < 3 {
			return r
		}
		out := River{Pts: [][2]float64{r.Pts[0]}, Width: []float64{r.Width[0]}}
		for i := 0; i+1 < len(r.Pts); i++ {
			a, b := r.Pts[i], r.Pts[i+1]
			wa, wb := r.Width[i], r.Width[i+1]
			out.Pts = append(out.Pts, [2]float64{0.75*a[0] + 0.25*b[0], 0.75*a[1] + 0.25*b[1]}, [2]float64{0.25*a[0] + 0.75*b[0], 0.25*a[1] + 0.75*b[1]})
			out.Width = append(out.Width, 0.75*wa+0.25*wb, 0.25*wa+0.75*wb)
		}
		out.Pts = append(out.Pts, r.Pts[len(r.Pts)-1])
		out.Width = append(out.Width, r.Width[len(r.Width)-1])
		r = out
	}
	return r
}

func blur(g []float64, w, h, passes int) []float64 {
	out := append([]float64(nil), g...)
	tmp := make([]float64, len(g))
	for p := 0; p < passes; p++ {
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				s, n := 0.0, 0.0
				for dx := -1; dx <= 1; dx++ {
					if xx := x + dx; xx >= 0 && xx < w {
						s += out[y*w+xx]
						n++
					}
				}
				tmp[y*w+x] = s / n
			}
		}
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				s, n := 0.0, 0.0
				for dy := -1; dy <= 1; dy++ {
					if yy := y + dy; yy >= 0 && yy < h {
						s += tmp[yy*w+x]
						n++
					}
				}
				out[y*w+x] = s / n
			}
		}
	}
	return out
}

type cellAt struct {
	i int
	e float64
}

type cellHeap []cellAt

func (h cellHeap) Len() int            { return len(h) }
func (h cellHeap) Less(a, b int) bool  { return h[a].e < h[b].e }
func (h cellHeap) Swap(a, b int)       { h[a], h[b] = h[b], h[a] }
func (h *cellHeap) Push(x interface{}) { *h = append(*h, x.(cellAt)) }
func (h *cellHeap) Pop() interface{} {
	old := *h
	x := old[len(old)-1]
	*h = old[:len(old)-1]
	return x
}
