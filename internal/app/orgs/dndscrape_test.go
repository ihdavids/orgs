package orgs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScrapePictures(t *testing.T) {
	page := `<html><head><meta property="og:image" content="/og.jpg"></head><body>
<img src="/logo.png" alt="Site">
<a href="/full/crypt_3x4.png"><img src="/thumbs/crypt.jpg" alt="Crypt floor"></a>
<img data-src="lazy/bridge.webp" src="data:image/gif;base64,R0l" alt="Bridge">
<img srcset="/s/a-200.jpg 200w, /s/a-1200.jpg 1200w" src="/s/a-200.jpg">
<div style="background-image: url('/bg/water.png')"></div>
<img src="/icons/star.svg"><img src="/thumbs/crypt.jpg">
<a href="/wagon.png"><img src="/wagon.png" alt="Wagon"></a>
</body></html>`
	got := scrapePictures([]byte(page), "https://tiles.example/pack/index.html")
	want := []string{
		"https://tiles.example/full/crypt_3x4.png",
		"https://tiles.example/wagon.png",
		"https://tiles.example/pack/lazy/bridge.webp",
		"https://tiles.example/s/a-1200.jpg",
		"https://tiles.example/bg/water.png",
		"https://tiles.example/og.jpg",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d pictures: %+v", len(got), got)
	}
	for i, w := range want {
		if got[i].Url != w {
			t.Errorf("%d: got %s, want %s", i, got[i].Url, w)
		}
	}
	if got[0].Name != "Crypt floor" || got[1].Name != "Wagon" || got[2].Name != "Bridge" || got[3].Name != "a-1200" {
		t.Errorf("names: %q %q %q %q", got[0].Name, got[1].Name, got[2].Name, got[3].Name)
	}
}

func TestTileNamesAndSquares(t *testing.T) {
	for _, c := range []struct{ stem, name, size string }{
		{"Room_Crypt_3x4", "Room Crypt", "3x4"},
		{"bridge-02x01", "bridge", "2x1"},
		{"Lava Pool", "Lava Pool", ""},
		{"corridor 10 x 2 dark", "corridor dark", "10x2"},
		{"tile_0x3", "tile", ""},
	} {
		if n := tileName(c.stem); n != c.name {
			t.Errorf("%s: name %q, want %q", c.stem, n, c.name)
		}
		if s := tileSquares(c.stem); s != c.size {
			t.Errorf("%s: squares %q, want %q", c.stem, s, c.size)
		}
	}
	if !tileSizeValue.MatchString("1.5x0.75") || tileSizeValue.MatchString("3x") {
		t.Error("tile size values")
	}
}

func TestFolderTagsAndSizes(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "tiles")
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, "Pit_2x2.png"), []byte("\x89PNG\r\n\x1a\nx"), 0644)
	m := readTokenSizes(dir)
	m.Tags["Pit_2x2.png"] = cleanPictureTags([]string{"Trap, dungeon", " trap ", "Deep  Pit"})
	if err := writeTokenSizes(dir, m); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "tiles.json")); err != nil {
		t.Fatal("kept in tiles.json:", err)
	}
	pics := walkShow(dir, "tiles/", "tiles", false, map[string]bool{})
	if len(pics) != 1 || pics[0].Name != "Pit" || pics[0].Size != "2x2" {
		t.Fatalf("%+v", pics)
	}
	if got := pics[0].Tags; len(got) != 3 || got[0] != "trap" || got[1] != "dungeon" || got[2] != "deep pit" {
		t.Errorf("tags %q", got)
	}
	if !looksArchive([]byte("PK\x03\x04rest")) || looksArchive([]byte("\x89PNG")) || archiveSuffix("Pack.TAR.GZ") != ".tar.gz" {
		t.Error("archives")
	}
}

func TestATileNamedOtherThanItsFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "tiles")
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, "IMG_2231_3x2.png"), []byte("\x89PNG\r\n\x1a\nx"), 0644)
	m := readTokenSizes(dir)
	m.Names["IMG_2231_3x2.png"] = "The Old Mill"
	writeTokenSizes(dir, m)
	pics := walkShow(dir, "tiles/", "tiles", false, map[string]bool{})
	if len(pics) != 1 || pics[0].Name != "The Old Mill" || pics[0].Size != "3x2" || pics[0].Path != "tiles/IMG_2231_3x2.png" {
		t.Fatalf("%+v", pics)
	}
}

