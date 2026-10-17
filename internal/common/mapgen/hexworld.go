package mapgen

import (
	"image"
	"math"
	"math/rand/v2"
	"sort"
)

// A whole world on a hexagon grid, in the manner of hexgen
// (https://github.com/eranimo/hexgen - the ideas, from its README; none of
// its code): a diamond-square heightmap that wraps east to west, the sea
// filled to the share of land asked for, temperature from latitude (the
// tilt widens the swing between equator and poles) and height, moisture
// from the sea and the rivers and the great belts of rising and sinking
// air, biomes from temperature and moisture, rivers run downhill from the
// heights, lakes where rivers meet, craters, volcanoes, and territories
// grown out from scattered capitals.
//
// The grid is pointy-topped hexes in rows, odd rows pushed half a hex east
// (HexCenter); the map viewer's hex grid overlay uses the same layout, so it
// lies exactly on these hexes when it is given their width.

type HexWorld struct {
	Cols, Rows int
	W          float64 // a hex's width, flat side to flat side, in px
	Alt        []float64
	Sea        float64 // the altitude of the sea's surface
	Temp       []float64
	Moist      []float64
	Biome      []string
	River      []int // how many river segments touch the hex
	Lake       []bool
	Feature    []string // "", "volcano", "crater"
	Territory  []int    // 0 for none
	Segments   [][2]int // rivers, hex to hex
}

// HexCenter is the middle of hex (c, r) in pixels, for hexes w wide.
func HexCenter(c, r int, w float64) (float64, float64) {
	R := w / math.Sqrt(3)
	x := w/2 + float64(c)*w
	if r%2 == 1 {
		x += w / 2
	}
	return x, R + float64(r)*1.5*R
}

// HexImageSize is the size of the picture of a cols × rows grid.
func HexImageSize(cols, rows int, w float64) (int, int) {
	R := w / math.Sqrt(3)
	return int(math.Ceil(float64(cols)*w + w/2)), int(math.Ceil(float64(rows-1)*1.5*R + 2*R))
}

// hexRows is how many rows a world of cols hexes across has: a world map's
// two-to-one, in hexes.
func hexRows(cols int) int { return max(8, int(math.Round(float64(cols)*0.58))) }

func (h *HexWorld) neighbours(i int) []int {
	c, r := i%h.Cols, i/h.Cols
	var d [6][2]int
	if r%2 == 0 {
		d = [6][2]int{{1, 0}, {-1, 0}, {0, -1}, {-1, -1}, {0, 1}, {-1, 1}}
	} else {
		d = [6][2]int{{1, 0}, {-1, 0}, {1, -1}, {0, -1}, {1, 1}, {0, 1}}
	}
	out := make([]int, 0, 6)
	for _, v := range d {
		nc, nr := c+v[0], r+v[1]
		if nr < 0 || nr >= h.Rows {
			continue
		}
		// The world is round: east runs into west.
		nc = (nc + h.Cols) % h.Cols
		out = append(out, nr*h.Cols+nc)
	}
	return out
}

// diamondSquare is a (2^k+1)-square heightmap, wrapping east to west, its
// roughness the share of each level's height kept at the next.
func diamondSquare(k int, rough float64, r *rand.Rand) ([]float64, int) {
	n := 1<<k + 1
	g := make([]float64, n*n)
	at := func(x, y int) *float64 {
		x = ((x % (n - 1)) + (n - 1)) % (n - 1)
		return &g[y*n+x]
	}
	// Seeded on a coarse grid, not just the corners: from the corners
	// alone a world is one great slope, its land all at one end.
	q := (n - 1) / 4
	for y := 0; y < n; y += q {
		for x := 0; x < n-1; x += q {
			*at(x, y) = r.Float64()*2 - 1
		}
	}
	amp := 0.7
	keep := 0.4 + rough*0.4
	for step := q; step > 1; step /= 2 {
		half := step / 2
		for y := half; y < n; y += step {
			for x := half; x < n-1+half; x += step {
				avg := (*at(x-half, y-half) + *at(x+half, y-half) + *at(x-half, y+half) + *at(x+half, y+half)) / 4
				*at(x, y) = avg + (r.Float64()*2-1)*amp
			}
		}
		for y := 0; y < n; y += half {
			start := half
			if (y/half)%2 == 1 {
				start = 0
			}
			for x := start; x < n-1; x += step {
				s, c := 0.0, 0.0
				for _, d := range [4][2]int{{-half, 0}, {half, 0}, {0, -half}, {0, half}} {
					yy := y + d[1]
					if yy < 0 || yy >= n {
						continue
					}
					s += *at(x+d[0], yy)
					c++
				}
				*at(x, y) = s/c + (r.Float64()*2-1)*amp
			}
		}
		amp *= keep
	}
	// The last column is the first, east meets west.
	for y := 0; y < n; y++ {
		g[y*n+n-1] = g[y*n]
	}
	return g, n
}

// GenerateHex makes a hex world.
func GenerateHex(p Params) *HexWorld {
	p = p.Normal()
	r := rand.New(rand.NewPCG(p.Seed, p.Seed^0x2545f4914f6cdd1d))
	h := &HexWorld{Cols: p.Hexes, Rows: hexRows(p.Hexes), W: p.HexPx}
	N := h.Cols * h.Rows
	k := 2
	for 1<<k < max(h.Cols, h.Rows*2) {
		k++
	}
	g, n := diamondSquare(k, p.Roughness, r)
	h.Alt = make([]float64, N)
	for i := range h.Alt {
		c, row := i%h.Cols, i/h.Cols
		fx := (float64(c) + 0.5*float64(row%2)) / float64(h.Cols) * float64(n-1)
		fy := float64(row) / float64(h.Rows-1) * float64(n-1)
		x0, y0 := int(fx), int(math.Min(fy, float64(n-2)))
		ax, ay := fx-float64(x0), fy-float64(y0)
		v := func(x, y int) float64 { return g[y*n+(x%(n-1))] }
		h.Alt[i] = lerp(lerp(v(x0, y0), v(x0+1, y0), ax), lerp(v(x0, y0+1), v(x0+1, y0+1), ax), ay)
	}
	// The sea comes up until the share of land is right.
	sorted := append([]float64(nil), h.Alt...)
	sort.Float64s(sorted)
	h.Sea = sorted[int(clamp((1-p.LandShare)*float64(N), 0, float64(N-1)))]
	lo, hi := sorted[0], sorted[N-1]
	land := func(i int) bool { return h.Alt[i] > h.Sea }
	height := func(i int) float64 { // 0 at the shore, 1 at the highest peak
		if hi <= h.Sea {
			return 0
		}
		return math.Max(0, (h.Alt[i]-h.Sea)/(hi-h.Sea))
	}
	_ = lo

	// Temperature: warm at the equator, cold at the poles - more so the
	// more the world is tilted - and colder with height.
	h.Temp = make([]float64, N)
	swing := 28 + math.Abs(p.Tilt)*0.45
	for i := range h.Temp {
		lat := (float64(i/h.Cols)/float64(h.Rows-1) - 0.5) * math.Pi // -π/2 .. π/2
		t := p.AvgTemp + swing*(math.Cos(lat)-0.6)
		if land(i) {
			t -= height(i) * 30
		}
		h.Temp[i] = t
	}

	// Rivers first: they wet the land they cross.
	h.River = make([]int, N)
	h.Lake = make([]bool, N)
	cands := []int{}
	for i := 0; i < N; i++ {
		if land(i) && height(i) > 0.35 && h.Temp[i] > -4 {
			cands = append(cands, i)
		}
	}
	r.Shuffle(len(cands), func(a, b int) { cands[a], cands[b] = cands[b], cands[a] })
	onRiver := make([]bool, N)
	sources := []int{}
	made := 0
	for _, start := range cands {
		if made >= p.RiverCount {
			break
		}
		if onRiver[start] {
			continue
		}
		// Rivers rise apart: not within four hexes of another's source.
		sx, sy := HexCenter(start%h.Cols, start/h.Cols, h.W)
		near := false
		for _, o := range sources {
			ox, oy := HexCenter(o%h.Cols, o/h.Cols, h.W)
			if math.Hypot(ox-sx, oy-sy) < 4*h.W {
				near = true
				break
			}
		}
		if near {
			continue
		}
		path := []int{start}
		cur := start
		ok := false
		for steps := 0; steps < N; steps++ {
			next, best := -1, h.Alt[cur]
			for _, nb := range h.neighbours(cur) {
				if h.Alt[nb] < best {
					next, best = nb, h.Alt[nb]
				}
			}
			if next < 0 {
				// A hollow: the river fills it.
				if len(path) >= 3 {
					h.Lake[cur] = true
					ok = true
				}
				break
			}
			path = append(path, next)
			if !land(next) || onRiver[next] {
				ok = len(path) >= 3
				break
			}
			cur = next
		}
		if !ok {
			continue
		}
		made++
		sources = append(sources, start)
		for j := 0; j+1 < len(path); j++ {
			a, b := path[j], path[j+1]
			h.Segments = append(h.Segments, [2]int{a, b})
			h.River[a]++
			h.River[b]++
			if land(a) {
				onRiver[a] = true
			}
		}
	}
	if p.Lakes {
		for i := 0; i < N; i++ {
			if land(i) && h.River[i] >= 4 {
				h.Lake[i] = true
			}
		}
	}

	// Moisture: water close by, carried inland a few hexes, and the belts of
	// rain at the equator and the middle latitudes, of dry air at the horse
	// latitudes and the poles.
	dist := make([]int, N)
	q := []int{}
	for i := range dist {
		dist[i] = -1
		if !land(i) || h.Lake[i] || h.River[i] > 0 {
			dist[i] = 0
			q = append(q, i)
		}
	}
	for len(q) > 0 {
		i := q[0]
		q = q[1:]
		for _, nb := range h.neighbours(i) {
			if dist[nb] < 0 {
				dist[nb] = dist[i] + 1
				q = append(q, nb)
			}
		}
	}
	noise := newNoise(p.Seed*13 + 1)
	h.Moist = make([]float64, N)
	for i := range h.Moist {
		latDeg := math.Abs(float64(i/h.Cols)/float64(h.Rows-1)-0.5) * 180
		belt := 0.55 + 0.35*math.Cos(latDeg*math.Pi/30) // wet at 0° and 60°, dry at 30° and 90°
		near := math.Exp(-float64(max(dist[i], 0)) / (3 + float64(h.Cols)/40))
		c, row := float64(i%h.Cols), float64(i/h.Cols)
		h.Moist[i] = clamp(near*0.6+belt*0.45+noise.at(c*0.15, row*0.15)*0.2-0.15, 0, 1)
	}

	// Biomes.
	h.Biome = make([]string, N)
	for i := range h.Biome {
		h.Biome[i] = hexBiome(land(i), h.Lake[i], height(i), h.Temp[i], h.Moist[i])
	}

	// Volcanoes on the heights, craters anywhere on land.
	h.Feature = make([]string, N)
	place := func(kind string, count int, ok func(i int) bool) {
		pool := []int{}
		for i := 0; i < N; i++ {
			if ok(i) {
				pool = append(pool, i)
			}
		}
		r.Shuffle(len(pool), func(a, b int) { pool[a], pool[b] = pool[b], pool[a] })
		for _, i := range pool[:min(count, len(pool))] {
			h.Feature[i] = kind
		}
	}
	place("volcano", p.Volcanoes, func(i int) bool { return land(i) && height(i) > 0.45 && !h.Lake[i] })
	place("crater", p.Craters, func(i int) bool { return land(i) && h.Feature[i] == "" && !h.Lake[i] && h.River[i] == 0 })

	// Territories: capitals scattered over the land, each realm taking
	// one neighbouring hex at a time, the hungrier realms more often, until
	// the land is shared out.
	h.Territory = make([]int, N)
	if p.Territories > 0 {
		lands := []int{}
		for i := 0; i < N; i++ {
			if land(i) && !h.Lake[i] {
				lands = append(lands, i)
			}
		}
		r.Shuffle(len(lands), func(a, b int) { lands[a], lands[b] = lands[b], lands[a] })
		type realm struct {
			edge   []int
			hunger float64
		}
		realms := []*realm{}
		for t, i := range lands[:min(p.Territories, len(lands))] {
			h.Territory[i] = t + 1
			realms = append(realms, &realm{edge: []int{i}, hunger: 0.4 + r.Float64()})
		}
		for active := len(realms); active > 0; {
			active = 0
			for t, rl := range realms {
				if len(rl.edge) == 0 {
					continue
				}
				active++
				if r.Float64() > rl.hunger {
					continue
				}
				// Grow from a random hex on the edge into a free neighbour.
				k := r.IntN(len(rl.edge))
				i := rl.edge[k]
				free := []int{}
				for _, nb := range h.neighbours(i) {
					if land(nb) && !h.Lake[nb] && h.Territory[nb] == 0 {
						free = append(free, nb)
					}
				}
				if len(free) == 0 {
					rl.edge = append(rl.edge[:k], rl.edge[k+1:]...)
					continue
				}
				nb := free[r.IntN(len(free))]
				h.Territory[nb] = t + 1
				rl.edge = append(rl.edge, nb)
			}
		}
	}
	return h
}

func hexBiome(land, lake bool, height, temp, moist float64) string {
	switch {
	case lake:
		return "lake"
	case !land && temp < -11:
		return "sea ice"
	case !land:
		return "ocean"
	case temp < -12:
		return "ice"
	case height > 0.82:
		if temp < 2 {
			return "snow"
		}
		return "mountain"
	}
	w := int(clamp(moist*4, 0, 3.999))
	switch {
	case temp < 2:
		return [4]string{"tundra", "tundra", "taiga", "taiga"}[w]
	case temp < 14:
		return [4]string{"cold desert", "grassland", "temperate forest", "temperate rain forest"}[w]
	case temp < 22:
		return [4]string{"desert", "shrubland", "temperate forest", "temperate rain forest"}[w]
	}
	return [4]string{"desert", "savanna", "tropical seasonal forest", "tropical rain forest"}[w]
}

var hexBiomeColour = map[string]rgb{
	"ocean": hexRGB(0x1f4672), "sea ice": hexRGB(0xd8e4ea), "lake": hexRGB(0x3a74a8),
	"ice": hexRGB(0xeef3f6), "snow": hexRGB(0xf6f6f4), "mountain": hexRGB(0x8a8273),
	"tundra": hexRGB(0xa3a68a), "taiga": hexRGB(0x4f6e4c),
	"cold desert": hexRGB(0xb4a787), "grassland": hexRGB(0x9aaa5e), "temperate forest": hexRGB(0x5b8a46),
	"temperate rain forest": hexRGB(0x3c6c46), "desert": hexRGB(0xd9c38c), "shrubland": hexRGB(0xa9a46a),
	"savanna": hexRGB(0xbdb067), "tropical seasonal forest": hexRGB(0x6a9a3c), "tropical rain forest": hexRGB(0x2e6a34),
}

// territoryColour is a realm's colour: hues spread round the wheel by the
// golden angle, soft enough to read the land through.
func territoryColour(t int) rgb {
	hue := math.Mod(float64(t)*137.508, 360)
	s, l := 0.55, 0.62
	c := (1 - math.Abs(2*l-1)) * s
	x := c * (1 - math.Abs(math.Mod(hue/60, 2)-1))
	m := l - c/2
	var r, g, b float64
	switch int(hue / 60) {
	case 0:
		r, g, b = c, x, 0
	case 1:
		r, g, b = x, c, 0
	case 2:
		r, g, b = 0, c, x
	case 3:
		r, g, b = 0, x, c
	case 4:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}
	return rgb{(r + m) * 255, (g + m) * 255, (b + m) * 255}
}

// RenderHex draws a hex world: each hex filled - true colour (shaded by
// height, with a grain), its biome's flat colour, or its realm's - with a
// faint line between hexes, a darker one along coasts and between realms,
// the rivers from hex middle to hex middle, and the volcanoes and craters.
func RenderHex(h *HexWorld, p Params) *image.RGBA {
	p = p.Normal()
	W, H := HexImageSize(h.Cols, h.Rows, h.W)
	img := image.NewRGBA(image.Rect(0, 0, W, H))
	R := h.W / math.Sqrt(3)
	grain := newNoise(p.Seed*5 + 2)
	hiAlt := h.Sea
	for _, a := range h.Alt {
		hiAlt = math.Max(hiAlt, a)
	}
	land := func(i int) bool { return h.Alt[i] > h.Sea && !h.Lake[i] }
	colourOf := func(i int) rgb {
		b := h.Biome[i]
		base := hexBiomeColour[b]
		switch p.HexStyle {
		case "political":
			if !land(i) {
				return mix(base, rgb{220, 228, 232}, 0.55)
			}
			if t := h.Territory[i]; t > 0 {
				return mix(mix(base, rgb{236, 230, 214}, 0.6), territoryColour(t), 0.55)
			}
			return mix(base, rgb{236, 230, 214}, 0.6)
		case "satellite":
			if !land(i) {
				if b == "ocean" {
					depth := clamp((h.Sea-h.Alt[i])/math.Max(1e-9, h.Sea-0), 0, 1)
					_ = depth
				}
				return base
			}
			// Lit by height: heights paler, lowlands deeper.
			ht := (h.Alt[i] - h.Sea) / math.Max(1e-9, hiAlt-h.Sea)
			return mix(base, rgb{base[0] * 0.8, base[1] * 0.8, base[2] * 0.8}, 0.5-ht*0.5)
		}
		return base
	}
	cols := make([]rgb, len(h.Biome))
	for i := range cols {
		cols[i] = colourOf(i)
	}
	// The sea's depth, for satellite: the deeper the darker.
	if p.HexStyle == "satellite" {
		lowest := h.Sea
		for _, a := range h.Alt {
			lowest = math.Min(lowest, a)
		}
		for i := range cols {
			if h.Alt[i] <= h.Sea && h.Biome[i] == "ocean" {
				d := (h.Sea - h.Alt[i]) / math.Max(1e-9, h.Sea-lowest)
				cols[i] = mix(hexRGB(0x3d79a8), hexRGB(0x13304f), math.Sqrt(d))
			}
		}
	}

	// Which hex a pixel is in, and how far it is from that hex's edge with
	// each neighbour: the bisector between the two middles.
	hexAt := func(x, y float64) int {
		row := int(math.Round((y - R) / (1.5 * R)))
		best, bd := -1, math.Inf(1)
		for rr := row - 1; rr <= row+1; rr++ {
			if rr < 0 || rr >= h.Rows {
				continue
			}
			off := 0.0
			if rr%2 == 1 {
				off = h.W / 2
			}
			cc := int(math.Round((x - h.W/2 - off) / h.W))
			for c := cc - 1; c <= cc+1; c++ {
				cx, cy := HexCenter(c, rr, h.W)
				if d := (cx-x)*(cx-x) + (cy-y)*(cy-y); d < bd {
					bd, best = d, rr*h.Cols+((c%h.Cols)+h.Cols)%h.Cols
				}
			}
		}
		return best
	}
	parallelRows(H, func(y int) {
		for x := 0; x < W; x++ {
			fx, fy := float64(x)+0.5, float64(y)+0.5
			i := hexAt(fx, fy)
			if i < 0 {
				continue
			}
			c := cols[i]
			if p.HexStyle == "satellite" {
				g := 1 + grain.fbm(fx*0.08, fy*0.08, 3)*0.12
				c = rgb{c[0] * g, c[1] * g, c[2] * g}
			}
			// Edges: the nearest neighbour's bisector.
			cx, cy := HexCenter(i%h.Cols, i/h.Cols, h.W)
			// (The wrap: a hex at the west edge is drawn whole, its middle
			// found from the pixel's side.)
			if math.Abs(cx-fx) > h.W {
				cx -= float64(h.Cols) * h.W * math.Copysign(1, cx-fx)
			}
			edge, other := math.Inf(1), -1
			for _, nb := range h.neighbours(i) {
				nx, ny := HexCenter(nb%h.Cols, nb/h.Cols, h.W)
				if math.Abs(nx-cx) > 2*h.W {
					nx -= float64(h.Cols) * h.W * math.Copysign(1, nx-cx)
				}
				// Distance from the pixel to the perpendicular bisector.
				mx, my := (cx+nx)/2, (cy+ny)/2
				dx, dy := nx-cx, ny-cy
				l := math.Hypot(dx, dy)
				d := ((mx-fx)*dx + (my-fy)*dy) / l
				if d < edge {
					edge, other = d, nb
				}
			}
			if other >= 0 {
				switch {
				case land(i) != land(other):
					// A coast.
					c = mix(c, rgb{30, 34, 40}, clamp(1.6-edge, 0, 1)*0.75)
				case h.Territory[i] != h.Territory[other] && p.Territories > 0 && land(i):
					// A border between realms (and the shore of a realm).
					ink := rgb{60, 40, 40}
					if p.HexStyle == "political" {
						ink = rgb{90, 50, 50}
					}
					c = mix(c, ink, clamp(1.4-edge, 0, 1)*0.7)
				default:
					c = mix(c, rgb{0, 0, 0}, clamp(0.9-edge, 0, 1)*0.12)
				}
			}
			o := img.PixOffset(x, y)
			img.Pix[o] = uint8(clamp(c[0], 0, 255))
			img.Pix[o+1] = uint8(clamp(c[1], 0, 255))
			img.Pix[o+2] = uint8(clamp(c[2], 0, 255))
			img.Pix[o+3] = 255
		}
	})
	river := hexRGB(0x2f6aa0)
	for _, s := range h.Segments {
		ax, ay := HexCenter(s[0]%h.Cols, s[0]/h.Cols, h.W)
		bx, by := HexCenter(s[1]%h.Cols, s[1]/h.Cols, h.W)
		if math.Abs(bx-ax) > 2*h.W {
			continue // across the wrap: not drawn
		}
		w := math.Max(1.2, h.W*0.12) * (1 + 0.25*math.Min(4, float64(h.River[s[1]]-1)))
		line(img, [2]float64{ax, ay}, [2]float64{bx, by}, w, w, river)
	}
	for i, f := range h.Feature {
		cx, cy := HexCenter(i%h.Cols, i/h.Cols, h.W)
		switch f {
		case "volcano":
			cone(img, cx, cy, h.W*0.36)
		case "crater":
			ring(img, cx, cy, h.W*0.28, math.Max(1, h.W*0.08), rgb{70, 62, 54})
		}
	}
	return img
}

// cone is a volcano: a dark cone with a glowing mouth.
func cone(img *image.RGBA, cx, cy, s float64) {
	for y := int(cy - s); y <= int(cy+s*0.7); y++ {
		for x := int(cx - s); x <= int(cx+s); x++ {
			if !(image.Point{x, y}.In(img.Bounds())) {
				continue
			}
			fy := (float64(y) - (cy - s)) / (1.7 * s) // 0 at the top, 1 at the foot
			half := s * (0.25 + 0.75*fy)
			if fy < 0 || fy > 1 || math.Abs(float64(x)-cx) > half {
				continue
			}
			c := rgb{58, 46, 40}
			if fy < 0.22 {
				c = rgb{232, 92, 40}
			}
			o := img.PixOffset(x, y)
			img.Pix[o], img.Pix[o+1], img.Pix[o+2] = uint8(c[0]), uint8(c[1]), uint8(c[2])
		}
	}
}

func ring(img *image.RGBA, cx, cy, r, w float64, c rgb) {
	for y := int(cy - r - w); y <= int(cy+r+w); y++ {
		for x := int(cx - r - w); x <= int(cx+r+w); x++ {
			if !(image.Point{x, y}.In(img.Bounds())) {
				continue
			}
			d := math.Abs(math.Hypot(float64(x)+0.5-cx, float64(y)+0.5-cy) - r)
			a := clamp(w/2+0.5-d, 0, 1)
			if a <= 0 {
				continue
			}
			o := img.PixOffset(x, y)
			for k := 0; k < 3; k++ {
				img.Pix[o+k] = uint8(lerp(float64(img.Pix[o+k]), c[k], a))
			}
		}
	}
}
