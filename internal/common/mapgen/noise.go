package mapgen

import (
	"math"
	"math/rand/v2"
)

// noise is 2D gradient noise (Perlin's improved noise, in two dimensions),
// seeded: the same seed, the same terrain.
type noise struct {
	perm [512]uint8
}

func newNoise(seed uint64) *noise {
	r := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
	n := &noise{}
	p := r.Perm(256)
	for i := 0; i < 512; i++ {
		n.perm[i] = uint8(p[i&255])
	}
	return n
}

func fade(t float64) float64 { return t * t * t * (t*(t*6-15) + 10) }

func grad(h uint8, x, y float64) float64 {
	switch h & 7 {
	case 0:
		return x + y
	case 1:
		return -x + y
	case 2:
		return x - y
	case 3:
		return -x - y
	case 4:
		return x
	case 5:
		return -x
	case 6:
		return y
	}
	return -y
}

// at is noise at a point, about -1..1.
func (n *noise) at(x, y float64) float64 {
	fx, fy := math.Floor(x), math.Floor(y)
	xi, yi := int(fx)&255, int(fy)&255
	x, y = x-fx, y-fy
	u, v := fade(x), fade(y)
	a, b := n.perm[xi]+uint8(yi), n.perm[xi+1]+uint8(yi)
	aa, ab := n.perm[a], n.perm[uint16(a)+1]
	ba, bb := n.perm[b], n.perm[uint16(b)+1]
	l1 := lerp(grad(aa, x, y), grad(ba, x-1, y), u)
	l2 := lerp(grad(ab, x, y-1), grad(bb, x-1, y-1), u)
	return lerp(l1, l2, v) * 0.75
}

// fbm is octaves of noise, each twice as fine and half as strong.
func (n *noise) fbm(x, y float64, octaves int) float64 {
	sum, amp, norm := 0.0, 1.0, 0.0
	for i := 0; i < octaves; i++ {
		sum += amp * n.at(x, y)
		norm += amp
		amp *= 0.5
		// A turn between octaves keeps the grid out of the coastline.
		x, y = x*1.6+y*1.2+17.3, -x*1.2+y*1.6+5.1
	}
	return sum / norm
}

// ridged is noise folded into sharp crests: 1 on a ridge line, falling off
// either side - mountain chains rather than blobs.
func (n *noise) ridged(x, y float64, octaves int) float64 {
	sum, amp, norm, prev := 0.0, 1.0, 0.0, 1.0
	for i := 0; i < octaves; i++ {
		r := 1 - math.Abs(n.at(x, y)*1.3)
		r *= r
		sum += amp * r * prev
		norm += amp
		prev = r
		amp *= 0.5
		x, y = x*1.7+y*1.1+3.7, -x*1.1+y*1.7+11.9
	}
	return sum / norm
}

func lerp(a, b, t float64) float64 { return a + (b-a)*t }

func clamp(v, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, v)) }

func smoothstep(a, b, x float64) float64 {
	t := clamp((x-a)/(b-a), 0, 1)
	return t * t * (3 - 2*t)
}
