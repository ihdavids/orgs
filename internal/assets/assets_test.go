package assets

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuiltinUnpacked(t *testing.T) {
	dir := Dir("templates")
	if dir == "" {
		t.Fatal("templates did not unpack")
	}
	if _, err := os.Stat(filepath.Join(dir, "html_default.tpl")); err != nil {
		t.Fatalf("html_default.tpl missing from the built in copy: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "embed.go")); err == nil {
		t.Error("embed.go was unpacked")
	}
	for _, tree := range []string{"web", "webfonts"} {
		if entries, err := os.ReadDir(Dir(tree)); err != nil || len(entries) == 0 {
			t.Errorf("%s did not unpack: %v", tree, err)
		}
	}
}

func TestConfiguredFolderWins(t *testing.T) {
	// Run from a directory with no ./templates so only the configured folder
	// and the built in copy are on the path.
	cwd, _ := os.Getwd()
	defer os.Chdir(cwd)
	os.Chdir(t.TempDir())

	mine := t.TempDir()
	os.MkdirAll(filepath.Join(mine, "html_styles"), 0o755)
	os.WriteFile(filepath.Join(mine, "html_default.tpl"), []byte("mine"), 0o644)
	os.WriteFile(filepath.Join(mine, "html_styles", "zzz_style.css"), []byte("x"), 0o644)

	if got := FindTemplate("html_default.tpl", mine); got != filepath.Join(mine, "html_default.tpl") {
		t.Errorf("configured copy not preferred: %s", got)
	}
	if got := FindTemplate("html_rtd.tpl", mine); got != filepath.Join(Dir("templates"), "html_rtd.tpl") {
		t.Errorf("built in copy not found behind the configured folder: %s", got)
	}
	themes := GlobTemplates("html_styles/*_style.css", mine)
	has := map[string]bool{}
	for _, n := range themes {
		if has[n] {
			t.Errorf("%s listed twice", n)
		}
		has[n] = true
	}
	if !has["html_styles/zzz_style.css"] || !has["html_styles/default_style.css"] {
		t.Errorf("listing should hold both the added and the built in themes: %v", themes)
	}
}
