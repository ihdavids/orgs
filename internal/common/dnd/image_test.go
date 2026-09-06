// Tests for the character portrait: the org link forms the image property
// accepts, the crop it works out for the printed sheet, and the experience
// ring the sheets draw around it.
package dnd

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseImageRef(t *testing.T) {
	cases := []struct{ in, want string }{
		{"art/lyra.png", "art/lyra.png"},
		{"  art/lyra.png  ", "art/lyra.png"},
		{"file:art/lyra.png", "art/lyra.png"},
		{"[[file:art/lyra.png]]", "art/lyra.png"},
		{"[[file:art/lyra.png][Lyra Silverleaf]]", "art/lyra.png"},
		{"[[./art/lyra.webp]]", "./art/lyra.webp"},
		{"<file:~/pics/lyra.gif>", "~/pics/lyra.gif"},
		{"[[attachment:lyra.png]]", "lyra.png"},
		{"[[file:art/lyra.png::42]]", "art/lyra.png"},
		{"https://example.com/a.png?v=2", "https://example.com/a.png?v=2"},
		{"[[https://example.com/a.png][art]]", "https://example.com/a.png"},
		{"data:image/gif;base64,R0lGOD", "data:image/gif;base64,R0lGOD"},
		{"", ""},
	}
	for _, c := range cases {
		if got := ParseImageRef(c.in); got != c.want {
			t.Errorf("ParseImageRef(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestImageIsRemote(t *testing.T) {
	remote := []string{"http://a/b.png", "https://a/b.png", "data:image/png;base64,AA", "//a/b.png"}
	local := []string{"art/lyra.png", "/tmp/lyra.png", "~/pics/lyra.png", "./a.png", "a b/c.png"}
	for _, r := range remote {
		if !ImageIsRemote(r) {
			t.Errorf("ImageIsRemote(%q) = false, want true", r)
		}
	}
	for _, l := range local {
		if ImageIsRemote(l) {
			t.Errorf("ImageIsRemote(%q) = true, want false", l)
		}
	}
}

func TestImageSrc(t *testing.T) {
	// A url is handed to the browser as it stands.
	url := "https://example.com/lyra.gif"
	if got, err := ImageSrc("[[ "+url+" ]]", ""); err != nil || got != url {
		t.Errorf("ImageSrc(url) = %q, %v, want %q", got, err, url)
	}
	// A file next to the sheet is inlined, keeping the export self contained.
	dir := t.TempDir()
	gif, _ := base64.StdEncoding.DecodeString(
		"R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7")
	if err := os.WriteFile(filepath.Join(dir, "lyra.gif"), gif, 0644); err != nil {
		t.Fatal(err)
	}
	got, err := ImageSrc("[[file:lyra.gif]]", dir)
	if err != nil {
		t.Fatalf("ImageSrc(file) failed: %s", err)
	}
	if !strings.HasPrefix(got, "data:image/gif;base64,") {
		t.Errorf("ImageSrc(file) = %.40q, want a gif data uri", got)
	}
	if !strings.HasSuffix(got, base64.StdEncoding.EncodeToString(gif)) {
		t.Errorf("ImageSrc(file) did not carry the file bytes")
	}
	// A missing file is a warning, not a portrait.
	if got, err := ImageSrc("nope.png", dir); err == nil || got != "" {
		t.Errorf("ImageSrc(missing) = %q, %v, want an error", got, err)
	}
	if got, err := ImageSrc("", dir); err != nil || got != "" {
		t.Errorf("ImageSrc(empty) = %q, %v, want empty", got, err)
	}
}

func TestParseFocusAndZoom(t *testing.T) {
	focus := []struct {
		in   string
		x, y float64
	}{
		{"57% 29%", 57, 29},
		{"57 29", 57, 29},
		{"57%,29%", 57, 29},
		{"40%", 40, 40},
		{"", 50, 50},
		{"the face", 50, 50},
	}
	for _, c := range focus {
		if x, y := ParseFocus(c.in); x != c.x || y != c.y {
			t.Errorf("ParseFocus(%q) = %v,%v, want %v,%v", c.in, x, y, c.x, c.y)
		}
	}
	zoom := []struct {
		in   string
		want float64
	}{{"3.2", 3.2}, {"320%", 3.2}, {"", 1}, {"0", 1}, {"-2", 1}, {"lots", 1}}
	for _, c := range zoom {
		if got := ParseZoom(c.in); got != c.want {
			t.Errorf("ParseZoom(%q) = %v, want %v", c.in, got, c.want)
		}
	}
	if got := FormatZoom(1); got != "" {
		t.Errorf("FormatZoom(1) = %q, want empty", got)
	}
	if got := FormatZoom(3.2); got != "3.2" {
		t.Errorf("FormatZoom(3.2) = %q, want 3.2", got)
	}
}

func TestLatexPortrait(t *testing.T) {
	dir := t.TempDir()
	// A 4x2 png, so the cover fit has to widen it to fill a round frame.
	png, _ := base64.StdEncoding.DecodeString(
		"iVBORw0KGgoAAAANSUhEUgAAAAQAAAACCAYAAAB/qH1jAAAAFElEQVR4nGI6ISf3nwEf" +
			"AAQAAP//P3ICBhWrzRMAAAAASUVORK5CYII=")
	path := filepath.Join(dir, "lyra.png")
	if err := os.WriteFile(path, png, 0644); err != nil {
		t.Fatal(err)
	}
	crop, warn := LatexPortrait("[[file:lyra.png]]", dir, 50, 50, 1)
	if warn != "" || crop == nil {
		t.Fatalf("LatexPortrait(png) = %v, %q, want a crop", crop, warn)
	}
	// Twice as wide as it is tall, so the frame is filled by height.
	if crop.Height != 1 || crop.Width != 2 {
		t.Errorf("crop = %vx%v, want 2x1 medallion diameters", crop.Width, crop.Height)
	}
	// Centred: the middle of the picture sits at the middle of the frame.
	if crop.X != -1 || crop.Y != 0.5 {
		t.Errorf("crop origin = %v,%v, want -1,0.5", crop.X, crop.Y)
	}
	// Zooming in on a face keeps the focus point at the centre.
	crop, _ = LatexPortrait("lyra.png", dir, 25, 50, 2)
	if crop.Width != 4 || crop.X != -1 {
		t.Errorf("zoomed crop = %v wide at %v, want 4 at -1", crop.Width, crop.X)
	}
	// What pdflatex cannot read is reported rather than dropped in silence.
	if _, warn := LatexPortrait("https://example.com/a.png", dir, 50, 50, 1); warn == "" {
		t.Error("a url should warn that the printed sheet has no portrait")
	}
	if err := os.WriteFile(filepath.Join(dir, "lyra.gif"), []byte("GIF89a"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, warn := LatexPortrait("lyra.gif", dir, 50, 50, 1); warn == "" {
		t.Error("a gif should warn that the printed sheet has no portrait")
	}
	if crop, warn := LatexPortrait("", dir, 50, 50, 1); crop != nil || warn != "" {
		t.Error("no portrait at all is not a warning")
	}
}

func TestXPProgress(t *testing.T) {
	// Level 5 runs from 6500 to 14000 experience.
	cases := []struct {
		xp, level, want int
	}{
		{6500, 5, 0},
		{10250, 5, 50},
		{14000, 5, 100},
		{0, 5, 0},
		{999999, 20, 100},
	}
	for _, c := range cases {
		if got := xpProgress(c.xp, c.level); got != c.want {
			t.Errorf("xpProgress(%d, %d) = %d, want %d", c.xp, c.level, got, c.want)
		}
	}
}
