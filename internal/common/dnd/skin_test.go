package dnd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSkinScope(t *testing.T) {
	ok := []string{
		`html[data-skin="moss"] { --paper: #eee; }`,
		`html[data-skin="moss"] .box, html[data-skin="moss"] .fact:is(.a, .b) { color: red; }`,
		`@media (max-width: 600px) { html[data-skin="moss"].m .box { padding: 0; } }`,
		`@keyframes moss-sway { to { transform: rotate(2deg); } }`,
		`/* a { } */ html[data-skin="moss"] a[href*="x,y"] { color: blue; }`,
		`@font-face { font-family: Moss; src: url(data:font/woff2;base64,AAAA); }`,
	}
	for _, css := range ok {
		if err := CheckSkinScope("moss", css); err != nil {
			t.Errorf("%q refused: %v", css, err)
		}
	}
	bad := []string{
		`.box { color: red; }`,
		`html[data-skin="moss"] .box, .fact { color: red; }`,
		`html[data-skin="mossy"] .box { color: red; }`,
		`@media print { body { color: red; } }`,
		`@keyframes sway { to { opacity: 0; } }`,
		`@import url(x.css); html[data-skin="moss"] { }`,
		`html[data-skin="moss"] .box { color: red; `,
	}
	for _, css := range bad {
		if err := CheckSkinScope("moss", css); err == nil {
			t.Errorf("%q let through", css)
		}
	}
}

func TestParseSkin(t *testing.T) {
	css := "/* skin: Wild Wood\n   about: Leaves.\n   swatch: #123 nope #abcdef\n   fonts: Kalam:wght@400;700 | Uncial Antiqua\n*/\nhtml[data-skin=\"wild\"] { color: red; }"
	s, err := ParseSkin("wild", css, "")
	if err != nil {
		t.Fatal(err)
	}
	if s.Name != "Wild Wood" || s.About != "Leaves." || len(s.Swatch) != 2 || s.Face != "Kalam" {
		t.Errorf("read %+v", s)
	}
	want := "https://fonts.googleapis.com/css2?family=Kalam:wght@400;700&family=Uncial+Antiqua&display=swap"
	if s.FontsUrl != want {
		t.Errorf("fonts url %q, want %q", s.FontsUrl, want)
	}
	if !strings.HasPrefix(s.Css, "@media screen {") {
		t.Error("a skin that does not print should be screen only")
	}
	if _, err := ParseSkin("Bad Name", css, ""); err == nil {
		t.Error("a bad id was taken")
	}
}

// TestShippedSkins checks every skin that ships keeps to its own page.
func TestShippedSkins(t *testing.T) {
	files, _ := filepath.Glob("../../../templates/dnd_skins/*.css")
	for _, f := range files {
		id := strings.TrimSuffix(filepath.Base(f), ".css")
		css, _ := os.ReadFile(f)
		svg, _ := os.ReadFile(strings.TrimSuffix(f, ".css") + ".svg")
		s, err := ParseSkin(id, string(css), string(svg))
		if err != nil {
			t.Errorf("%s: %v", f, err)
			continue
		}
		if s.About == "" || len(s.Swatch) == 0 || s.Name == id {
			t.Errorf("%s: the header needs skin:, about: and swatch:", f)
		}
		if len(css)+len(svg) > 90*1024 {
			t.Errorf("%s: %d bytes; keep a skin under 90 KB", f, len(css)+len(svg))
		}
	}
}
