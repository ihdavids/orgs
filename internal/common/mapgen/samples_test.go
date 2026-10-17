package mapgen

import (
	"os"
	"testing"
	"time"
)

// TestSamples draws a map in every style into $MGOUT, to look at; skipped
// without it.
func TestSamples(t *testing.T) {
	dir := os.Getenv("MGOUT")
	if dir == "" {
		t.Skip("set MGOUT to a folder to draw the samples into")
	}
	for _, st := range []string{"atlas", "biomes", "parchment"} {
		p := Defaults()
		p.Seed = 7
		p.Style = st
		t0 := time.Now()
		b, err := JPEG(p, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%s %v %d KB", st, time.Since(t0), len(b)/1024)
		os.WriteFile(dir+"/"+st+".jpg", b, 0644)
	}
	for _, st := range CityStyles {
		p := Defaults()
		p.Generator, p.Seed, p.CityStyle = "city", 7, st
		p.River = st == "parchment"
		p.Seaside = st == "ink"
		t0 := time.Now()
		b, _, err := Picture(p, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("city %s %v %d KB", st, time.Since(t0), len(b)/1024)
		os.WriteFile(dir+"/city-"+st+".svg", b, 0644)
	}
	for i, st := range CityStyles {
		p := Defaults()
		p.Generator, p.Seed, p.CityStyle = "village", uint64(11+i), st
		p.River = i%2 == 0
		p.Seaside = i == 1 || i == 2
		p.Windmills = 2
		t0 := time.Now()
		b, _, err := Picture(p, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("village %s %v %d KB", st, time.Since(t0), len(b)/1024)
		os.WriteFile(dir+"/village-"+st+".svg", b, 0644)
	}
	for _, st := range ShoresStyles {
		p := Defaults()
		p.Generator, p.Seed, p.ShoresStyle = "shores", 7, st
		t0 := time.Now()
		b, _, err := Picture(p, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("shores %s %v %d KB", st, time.Since(t0), len(b)/1024)
		os.WriteFile(dir+"/shores-"+st+".svg", b, 0644)
	}
	for _, st := range HexStyles {
		p := Defaults()
		p.Generator = "hex"
		p.Seed = 7
		p.HexStyle = st
		t0 := time.Now()
		b, err := JPEG(p, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("hex %s %v %d KB", st, time.Since(t0), len(b)/1024)
		os.WriteFile(dir+"/hex-"+st+".jpg", b, 0644)
	}
}
