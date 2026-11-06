package geo

import (
	"encoding/json"
	"math"
	"os"
	"strings"
	"testing"
)

// The cases are shared with worg (src/mapview_cases.json is a copy): change
// both when one changes.
type geoCases struct {
	Parse []struct {
		In       string
		Lat, Lon float64
		None     bool
	}
	Format []struct {
		Lat, Lon float64
		Out      string
	}
	Colour []struct {
		By, Keyword string
		Done        bool
		Tags        []string
		Out         string
	}
	Hash []struct {
		In  string
		N   int
		Out int
	}
	Fit []struct {
		Pts      [][2]float64
		W, H     int
		Lat, Lon float64
		Zoom     int
	}
}

func loadCases(t *testing.T) geoCases {
	b, err := os.ReadFile("testdata/geo_cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var c geoCases
	if err := json.Unmarshal(b, &c); err != nil {
		t.Fatal(err)
	}
	return c
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-5 }

func TestGeoCases(t *testing.T) {
	c := loadCases(t)
	for _, k := range c.Parse {
		p, ok := Parse(k.In)
		if k.None {
			if ok {
				t.Errorf("Parse(%q) = %v, want none", k.In, p)
			}
			continue
		}
		if !ok || !near(p.Lat, k.Lat) || !near(p.Lon, k.Lon) {
			t.Errorf("Parse(%q) = %v %v, want %v,%v", k.In, p, ok, k.Lat, k.Lon)
		}
	}
	for _, k := range c.Format {
		if got := Format(LatLon{k.Lat, k.Lon}); got != k.Out {
			t.Errorf("Format(%v,%v) = %q, want %q", k.Lat, k.Lon, got, k.Out)
		}
	}
	for _, k := range c.Hash {
		if got := HashIndex(k.In, k.N); got != k.Out {
			t.Errorf("HashIndex(%q) = %d, want %d", k.In, got, k.Out)
		}
	}
	for _, k := range c.Colour {
		want := k.Out
		if want == "PALETTE" {
			want = Palette[HashIndex(strings.ToLower(k.Tags[0]), len(Palette))]
		}
		if got := PinColour(k.By, k.Keyword, k.Done, k.Tags); got != want {
			t.Errorf("PinColour(%+v) = %s, want %s", k, got, want)
		}
	}
	for _, k := range c.Fit {
		pts := []LatLon{}
		for _, p := range k.Pts {
			pts = append(pts, LatLon{p[0], p[1]})
		}
		ctr, z := FitView(pts, k.W, k.H, 40, 18)
		if z != k.Zoom || !near(ctr.Lat, k.Lat) || !near(ctr.Lon, k.Lon) {
			t.Errorf("FitView(%v) = %.6f,%.6f z%d, want %v,%v z%d", k.Pts, ctr.Lat, ctr.Lon, z, k.Lat, k.Lon, k.Zoom)
		}
	}
}

func TestWorldPxRoundTrip(t *testing.T) {
	p := LatLon{49.2827, -123.1207}
	x, y := WorldPx(p, 12)
	q := FromWorldPx(x, y, 12)
	if !near(p.Lat, q.Lat) || !near(p.Lon, q.Lon) {
		t.Fatalf("round trip %v -> %v", p, q)
	}
}

func TestGeoJSON(t *testing.T) {
	b, err := MarshalGeoJSON([]Place{{Title: "Cafe", Has: true, Lat: 49.28, Lon: -123.12, Tags: []string{"coffee"}}, {Title: "Nowhere"}})
	if err != nil {
		t.Fatal(err)
	}
	var fc FeatureCollection
	if err := json.Unmarshal(b, &fc); err != nil {
		t.Fatal(err)
	}
	if len(fc.Features) != 1 || fc.Features[0].Geometry["coordinates"].([]interface{})[0].(float64) != -123.12 {
		t.Fatalf("got %s", b)
	}
}
