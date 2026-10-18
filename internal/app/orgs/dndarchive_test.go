package orgs

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"
)

func TestPackName(t *testing.T) {
	for in, want := range map[string][2]string{
		"Goblin_Archer_A1_Bow_01":                   {"Goblin Archer", ""},
		"Lamia_A1_Large_Monstrosity_01":             {"Lamia", "large"},
		"Manticore_A1_Large_Scale200_Monstrosity_01": {"Manticore", "large"},
		"Medusa_A1_Medium_Monstrosity_01":           {"Medusa", "small"},
		"Ancient_Red_Dragon_A1_Gargantuan_01":       {"Ancient Red Dragon", "large"},
		"Wolf":                                       {"Wolf", ""},
		"!Core_Barbarian_A1_Greataxe_01":            {"Barbarian", ""},
	} {
		name, size := packName(in)
		if name != want[0] || size != want[1] {
			t.Errorf("%s: got %q %q, want %q %q", in, name, size, want[0], want[1])
		}
	}
}

func TestZipFolder(t *testing.T) {
	dir := t.TempDir()
	zp := filepath.Join(dir, "Pack.zip")
	f, _ := os.Create(zp)
	w := zip.NewWriter(f)
	png := []byte("\x89PNG\r\n\x1a\nfake")
	for _, n := range []string{"Pack/Creatures/Wolf_A1_01.png", "Pack/Creatures/Wolf_A2_01.png", "Pack/Creatures/Owlbear_A1_Large_01.png", "Pack/NPCs/Guard_A1_01.png", "Pack/readme.txt"} {
		e, _ := w.Create(n)
		e.Write(png)
	}
	w.Close()
	f.Close()
	cats := map[string]bool{}
	pics := archivePictures(zp, "Pack.zip", "tokens/", true, tokenSizes{Sizes: map[string]string{}}, false, cats)
	if len(pics) != 4 {
		t.Fatalf("want 4 pictures, got %d: %+v", len(pics), pics)
	}
	byName := map[string]showPicture{}
	for _, p := range pics {
		byName[p.Name] = p
	}
	if byName["Wolf"].Path != "tokens/Pack.zip/Pack/Creatures/Wolf_A1_01.png" || byName["Wolf 2"].Path == "" {
		t.Fatalf("names: %+v", pics)
	}
	if byName["Owlbear"].Size != "large" || byName["Owlbear"].Category != "Creatures" || !cats["NPCs"] {
		t.Fatalf("size and categories: %+v %v", byName["Owlbear"], cats)
	}
	rec := httptest.NewRecorder()
	if !serveShow(rec, httptest.NewRequest("GET", "/", nil), filepath.Join(zp, "Pack/Creatures/Wolf_A1_01.png")) {
		t.Fatal("not served")
	}
	if rec.Body.String() != string(png) || rec.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("served %q as %q", rec.Body.String(), rec.Header().Get("Content-Type"))
	}
	if serveShow(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil), filepath.Join(zp, "Pack/readme.txt")) {
		t.Fatal("served a text file out of the zip")
	}
}

func TestTarArchives(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1a\nfake")
	for _, name := range []string{"Pack.tar.gz", "Pack.tar.xz", "Pack.tar.zst", "Pack.tar"} {
		dir := t.TempDir()
		p := filepath.Join(dir, name)
		f, _ := os.Create(p)
		var w io.WriteCloser = nopCloser{f}
		switch {
		case strings.HasSuffix(name, ".gz"):
			w = gzip.NewWriter(f)
		case strings.HasSuffix(name, ".xz"):
			w, _ = xz.NewWriter(f)
		case strings.HasSuffix(name, ".zst"):
			w, _ = zstd.NewWriter(f)
		}
		tw := tar.NewWriter(w)
		for _, n := range []string{"Beasts/Wolf_A1_01.png", "Beasts/Bear_A1_Large_01.png", "notes.txt"} {
			tw.WriteHeader(&tar.Header{Name: n, Mode: 0644, Size: int64(len(png)), Typeflag: tar.TypeReg})
			tw.Write(png)
		}
		tw.Close()
		w.Close()
		f.Close()
		pics := archivePictures(p, name, "tokens/", true, tokenSizes{Sizes: map[string]string{}}, false, map[string]bool{})
		if len(pics) != 2 {
			t.Fatalf("%s: want 2 pictures, got %+v", name, pics)
		}
		rec := httptest.NewRecorder()
		if !serveShow(rec, httptest.NewRequest("GET", "/", nil), filepath.Join(p, "Beasts/Bear_A1_Large_01.png")) || rec.Body.String() != string(png) {
			t.Fatalf("%s: not served: %q", name, rec.Body.String())
		}
	}
}

type nopCloser struct{ io.Writer }

func (nopCloser) Close() error { return nil }
