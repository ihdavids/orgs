package mapgen

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"math"
)

type rgb [3]float64

func mix(a, b rgb, t float64) rgb {
	return rgb{lerp(a[0], b[0], t), lerp(a[1], b[1], t), lerp(a[2], b[2], t)}
}

// A palette: the sea from shallow to deep, the land at four heights from dry
// to wet, the beach, snow, rivers and the coastline's ink.
type palette struct {
	shallow, deep rgb
	land          [4][2]rgb // low, mid, high, peak × dry, wet
	beach, snow   rgb
	river, ink    rgb
	foam          rgb
}

var palettes = map[string]palette{
	"biomes": {river: rgb{34, 85, 136}, ink: rgb{40, 38, 34}},
	"atlas": {
		shallow: rgb{122, 170, 200}, deep: rgb{38, 74, 120},
		land: [4][2]rgb{
			{{204, 196, 142}, {112, 152, 84}},
			{{182, 174, 124}, {72, 120, 64}},
			{{156, 146, 126}, {104, 112, 96}},
			{{186, 178, 166}, {240, 240, 238}},
		},
		beach: rgb{226, 212, 166}, snow: rgb{246, 246, 244},
		river: rgb{70, 114, 160}, ink: rgb{40, 38, 34}, foam: rgb{196, 222, 232},
	},
	"parchment": {
		shallow: rgb{196, 190, 160}, deep: rgb{150, 146, 122},
		land: [4][2]rgb{
			{{232, 216, 176}, {204, 196, 150}},
			{{222, 204, 160}, {188, 180, 136}},
			{{200, 182, 142}, {178, 164, 128}},
			{{214, 200, 170}, {236, 228, 206}},
		},
		beach: rgb{238, 224, 186}, snow: rgb{242, 236, 218},
		river: rgb{122, 124, 120}, ink: rgb{70, 52, 34}, foam: rgb{214, 206, 176},
	},
	"ink": {
		shallow: rgb{206, 220, 228}, deep: rgb{150, 172, 190},
		land: [4][2]rgb{
			{{246, 244, 238}, {236, 238, 228}},
			{{236, 234, 226}, {226, 228, 218}},
			{{218, 216, 208}, {208, 210, 202}},
			{{232, 230, 224}, {250, 250, 248}},
		},
		beach: rgb{246, 244, 236}, snow: rgb{255, 255, 255},
		river: rgb{110, 140, 170}, ink: rgb{30, 30, 34}, foam: rgb{230, 238, 242},
	},
}

// Biomes, by height zone (low to high) and moisture zone (dry to wet), as
// Amit Patel's polygon map generator draws them (after Whittaker's diagram),
// with its colours.
var (
	biomeColour = map[string]rgb{
		"snow": hexRGB(0xffffff), "tundra": hexRGB(0xbbbbaa), "bare": hexRGB(0x888888), "scorched": hexRGB(0x555555),
		"taiga": hexRGB(0x99aa77), "shrubland": hexRGB(0x889977), "temperate desert": hexRGB(0xc9d29b),
		"temperate rain forest": hexRGB(0x448855), "temperate deciduous forest": hexRGB(0x679459), "grassland": hexRGB(0x88aa55),
		"subtropical desert": hexRGB(0xd2b98b), "tropical rain forest": hexRGB(0x337755), "tropical seasonal forest": hexRGB(0x559944),
	}
	biomeTable = [4][6]string{
		{"subtropical desert", "grassland", "tropical seasonal forest", "tropical seasonal forest", "tropical rain forest", "tropical rain forest"},
		{"temperate desert", "grassland", "grassland", "temperate deciduous forest", "temperate deciduous forest", "temperate rain forest"},
		{"temperate desert", "temperate desert", "shrubland", "shrubland", "taiga", "taiga"},
		{"scorched", "bare", "tundra", "snow", "snow", "snow"},
	}
	biomeOcean, biomeLake, biomeBeach = hexRGB(0x44447a), hexRGB(0x336699), hexRGB(0xa09077)
)

func hexRGB(v int) rgb { return rgb{float64(v >> 16 & 255), float64(v >> 8 & 255), float64(v & 255)} }

// Biome names the biome of land at a height (0..1) and wetness (0..1).
func Biome(e, m float64) string {
	z := int(clamp(e/0.95*4, 0, 3.999))
	w := int(clamp(m*6, 0, 5.999))
	return biomeTable[z][w]
}

func (pl *palette) landColour(e, m float64) rgb {
	m = clamp(m, 0, 1)
	band := func(k int) rgb { return mix(pl.land[k][0], pl.land[k][1], m) }
	switch {
	case e < 0.35:
		return mix(band(0), band(1), e/0.35)
	case e < 0.7:
		return mix(band(1), band(2), (e-0.35)/0.35)
	default:
		return mix(band(2), band(3), smoothstep(0.7, 1.05, e))
	}
}

// Render draws a terrain: colour by height and wetness, shaded by the light,
// then the rivers and the coastline.
func Render(t *Terrain, p Params) *image.RGBA {
	p = p.Normal()
	pl := palettes[p.Style]
	W, H := t.W, t.H
	img := image.NewRGBA(image.Rect(0, 0, W, H))
	la := p.Light * math.Pi / 180
	// The light comes from that way, 45° up.
	lx, ly, lz := -math.Cos(la)*0.7071, -math.Sin(la)*0.7071, 0.7071
	relief := 0.4 + p.Relief*3.2
	cell := float64(t.Cell)
	coarse := func(g []float32, x, y float64) float64 {
		fx, fy := x/cell-0.5, y/cell-0.5
		fx, fy = clamp(fx, 0, float64(t.CW-1)), clamp(fy, 0, float64(t.CH-1))
		x0, y0 := int(fx), int(fy)
		x1, y1 := min(x0+1, t.CW-1), min(y0+1, t.CH-1)
		ax, ay := fx-float64(x0), fy-float64(y0)
		return lerp(lerp(float64(g[y0*t.CW+x0]), float64(g[y0*t.CW+x1]), ax), lerp(float64(g[y1*t.CW+x0]), float64(g[y1*t.CW+x1]), ax), ay)
	}
	elev := func(x, y int) float64 {
		return float64(t.Elev[min(max(y, 0), H-1)*W+min(max(x, 0), W-1)])
	}
	var polys *sites
	edgeNoise := newNoise(p.Seed*17 + 3)
	if p.Style == "biomes" {
		polys = newSites(W, H, max(8, max(W, H)/110), p.Seed)
	}
	parallelRows(H, func(y int) {
		for x := 0; x < W; x++ {
			e := elev(x, y)
			fx, fy := float64(x), float64(y)
			lake := coarse(t.Lake, fx, fy)
			var c rgb
			water := e < 0 || lake > 0.002
			if polys != nil {
				// The polygon a pixel is in: the nearest of the sites, found
				// from a point pushed about by noise, so the borders wander.
				k := 1.0 / float64(polys.size)
				px := fx + (edgeNoise.at(fx*k*0.9, fy*k*0.9)+0.5*edgeNoise.at(fx*k*2.7+9, fy*k*2.7+9))*float64(polys.size)*0.9
				py := fy + (edgeNoise.at(fx*k*0.9+71, fy*k*0.9+71)+0.5*edgeNoise.at(fx*k*2.7+33, fy*k*2.7+33))*float64(polys.size)*0.9
				s := polys.nearest(px, py)
				se := t.sample(s[0], s[1])
				sl := coarse(t.Lake, s[0], s[1])
				switch {
				case se < 0:
					c = mix(biomeOcean, rgb{36, 36, 92}, smoothstep(0, 0.3, -se)*p.Ocean)
					water = true
				case sl > 0.002:
					c, water = biomeLake, true
				case se < 0.03:
					c, water = biomeBeach, false
				default:
					c, water = biomeColour[Biome(se, coarse(t.Moist, s[0], s[1]))], false
				}
			} else if water {
				depth := -e
				if e >= 0 {
					depth = lake * 4
				}
				// Shallows hug the coast; the open sea is deep, a little
				// lighter over banks.
				c = mix(pl.shallow, pl.deep, smoothstep(0, 0.03+0.08*(1-p.Ocean), depth)*0.85+smoothstep(0.1, 0.6, depth)*0.15)
			} else {
				m := coarse(t.Moist, fx, fy)
				c = pl.landColour(e, m)
				if e < 0.018 {
					c = mix(pl.beach, c, e/0.018)
				}
			}
			// Shading: the slope against the light. A flat cell is as it is.
			dx := (elev(x+1, y) - elev(x-1, y)) * relief * float64(W) / 512
			dy := (elev(x, y+1) - elev(x, y-1)) * relief * float64(W) / 512
			if water {
				dx, dy = dx*0.15, dy*0.15
			}
			nl := math.Sqrt(dx*dx + dy*dy + 1)
			s := (-dx*lx - dy*ly + lz) / nl / lz
			s = clamp(s, 0.35, 1.45)
			c = rgb{c[0] * s, c[1] * s, c[2] * s}
			// The coast: ink on the land side, a pale line in the water.
			if polys != nil {
				// (Polygons have their own coast, where their colours change.)
			} else if !water {
				if elev(x+1, y) < 0 || elev(x-1, y) < 0 || elev(x, y+1) < 0 || elev(x, y-1) < 0 {
					c = mix(c, pl.ink, 0.25+p.Outline*0.65)
				}
			} else if e < 0 && e > -0.035 {
				c = mix(c, pl.foam, (1-(-e/0.035))*0.45)
			}
			i := img.PixOffset(x, y)
			img.Pix[i] = uint8(clamp(c[0], 0, 255))
			img.Pix[i+1] = uint8(clamp(c[1], 0, 255))
			img.Pix[i+2] = uint8(clamp(c[2], 0, 255))
			img.Pix[i+3] = 255
		}
	})
	for _, r := range t.Rivers {
		for i := 0; i+1 < len(r.Pts); i++ {
			line(img, r.Pts[i], r.Pts[i+1], r.Width[i], r.Width[i+1], pl.river)
		}
	}
	if p.Style == "parchment" {
		paper(img, p.Seed)
	}
	return img
}

// sites are the middles of the polygons: one per square of a grid, put
// somewhere at random in it, so the nearest site to a point is among the
// nine squares round it.
type sites struct {
	size, gw, gh int
	pts          [][2]float64
}

func newSites(W, H, size int, seed uint64) *sites {
	s := &sites{size: size, gw: W/size + 2, gh: H/size + 2}
	n := newNoise(seed*101 + 9)
	for gy := 0; gy < s.gh; gy++ {
		for gx := 0; gx < s.gw; gx++ {
			jx := (n.at(float64(gx)*1.37+0.5, float64(gy)*1.37+0.5) + 1) / 2
			jy := (n.at(float64(gx)*1.37+50.5, float64(gy)*1.37+50.5) + 1) / 2
			s.pts = append(s.pts, [2]float64{(float64(gx) + 0.1 + 0.8*jx) * float64(size), (float64(gy) + 0.1 + 0.8*jy) * float64(size)})
		}
	}
	return s
}

func (s *sites) nearest(x, y float64) [2]float64 {
	gx, gy := int(x)/s.size, int(y)/s.size
	best, bd := [2]float64{x, y}, math.Inf(1)
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			cx, cy := gx+dx, gy+dy
			if cx < 0 || cy < 0 || cx >= s.gw || cy >= s.gh {
				continue
			}
			p := s.pts[cy*s.gw+cx]
			if d := (p[0]-x)*(p[0]-x) + (p[1]-y)*(p[1]-y); d < bd {
				bd, best = d, p
			}
		}
	}
	return best
}

// line draws a segment of river, round-ended so the segments join, its width
// going from wa to wb, edges smoothed.
func line(img *image.RGBA, a, b [2]float64, wa, wb float64, c rgb) {
	r := math.Max(wa, wb)/2 + 1.5
	x0, x1 := int(math.Floor(math.Min(a[0], b[0])-r)), int(math.Ceil(math.Max(a[0], b[0])+r))
	y0, y1 := int(math.Floor(math.Min(a[1], b[1])-r)), int(math.Ceil(math.Max(a[1], b[1])+r))
	bnd := img.Bounds()
	vx, vy := b[0]-a[0], b[1]-a[1]
	L := vx*vx + vy*vy
	for y := max(y0, bnd.Min.Y); y <= min(y1, bnd.Max.Y-1); y++ {
		for x := max(x0, bnd.Min.X); x <= min(x1, bnd.Max.X-1); x++ {
			px, py := float64(x)+0.5, float64(y)+0.5
			t := 0.0
			if L > 0 {
				t = clamp(((px-a[0])*vx+(py-a[1])*vy)/L, 0, 1)
			}
			d := math.Hypot(px-(a[0]+vx*t), py-(a[1]+vy*t))
			w := lerp(wa, wb, t) / 2
			alpha := clamp(w+0.5-d, 0, 1)
			if alpha <= 0 {
				continue
			}
			i := img.PixOffset(x, y)
			for k := 0; k < 3; k++ {
				img.Pix[i+k] = uint8(lerp(float64(img.Pix[i+k]), c[k], alpha))
			}
		}
	}
}

// paper ages a parchment map: a faint mottle and darker edges.
func paper(img *image.RGBA, seed uint64) {
	n := newNoise(seed*3 + 1)
	W, H := img.Bounds().Dx(), img.Bounds().Dy()
	parallelRows(H, func(y int) {
		for x := 0; x < W; x++ {
			fx, fy := float64(x)/float64(W), float64(y)/float64(H)
			mottle := 1 + n.fbm(fx*9, fy*9*float64(H)/float64(W), 4)*0.08
			ex := math.Min(math.Min(fx, 1-fx), math.Min(fy, 1-fy))
			vig := 0.82 + 0.18*smoothstep(0, 0.12, ex)
			i := img.PixOffset(x, y)
			for k := 0; k < 3; k++ {
				img.Pix[i+k] = uint8(clamp(float64(img.Pix[i+k])*mottle*vig, 0, 255))
			}
		}
	})
}

// Picture is a map drawn and encoded, ready to save or send, with the
// extension it is saved under: a town is SVG, the rest jpeg.
func Picture(p Params, strokes []Stroke) ([]byte, string, error) {
	p = p.Normal()
	if p.Generator == "city" {
		return GenerateCity(p).SVG(p.CityStyle), ".svg", nil
	}
	if p.Generator == "shores" {
		return ShoresSVG(p, strokes), ".svg", nil
	}
	if p.Generator == "village" {
		return GenerateVillage(p).SVG(p.CityStyle), ".svg", nil
	}
	b, err := JPEG(p, strokes)
	return b, ".jpg", err
}

// JPEG is a terrain or hex world map drawn and encoded as jpeg.
func JPEG(p Params, strokes []Stroke) ([]byte, error) {
	p = p.Normal()
	var img *image.RGBA
	if p.Generator == "hex" {
		img = RenderHex(GenerateHex(p), p)
	} else {
		img = Render(Generate(p, strokes), p)
	}
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, &jpeg.Options{Quality: 90}); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

var _ = color.RGBA{}

// ImageSize is the size of the picture a map's params make.
func ImageSize(p Params) (int, int) {
	p = p.Normal()
	if p.Generator == "hex" {
		return HexImageSize(p.Hexes, hexRows(p.Hexes), p.HexPx)
	}
	if p.Generator == "city" || p.Generator == "village" {
		return p.CityPx, p.CityPx
	}
	return p.Width, p.Height
}
