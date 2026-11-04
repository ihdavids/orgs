package babelargs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sameAs(lang string) func(string) bool {
	return func(l string) bool { return strings.EqualFold(l, lang) }
}

func TestNearestReplacesAndPlusAdds(t *testing.T) {
	levels := [][]Entry{
		{{"header-args", ":session a :results output"}, {"header-args+", ":dir /tmp"}, {"header-args:python", ":cache yes"}},
		{{"header-args", ":results silent"}},
		{{"header-args+", ":exports both"}, {"header-args:sh", ":eval no"}},
	}
	g, s := Resolve(levels, sameAs("python"))
	if g != ":results silent :exports both" {
		t.Errorf("general %q", g)
	}
	if s != ":cache yes" {
		t.Errorf("specific %q", s)
	}
}

func TestFileEntriesReadSetupFilesWhereTheyStand(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "setup.org"), []byte("#+PROPERTY: header-args :results output\n#+SETUPFILE: main.org\n"), 0644)
	main := filepath.Join(dir, "main.org")
	text := "#+PROPERTY: header-args :session x\n#+SETUPFILE: setup.org\n#+PROPERTY: header-args+ :dir d\n#+BEGIN_SRC org\n#+PROPERTY: header-args :eval never\n#+END_SRC\n"
	got := FileEntries(main, text)
	want := []Entry{{"header-args", ":session x"}, {"header-args", ":results output"}, {"header-args+", ":dir d"}}
	if len(got) != len(want) {
		t.Fatalf("entries %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("entry %d: %v, want %v", i, got[i], want[i])
		}
	}
}

func TestDefaultsAndModes(t *testing.T) {
	d := Defaults(":exports code", map[string]string{"python": ":session main", "sh": ":eval no"}, sameAs("python"))
	if len(d) != 2 || d[0] != ":exports code" || d[1] != ":session main" {
		t.Errorf("defaults %v", d)
	}
	for in, want := range map[string]os.FileMode{"(identity #o755)": 0755, "#o600": 0600, "0644": 0644, "755": 0755} {
		if m, ok := ParseMode(in); !ok || m != want {
			t.Errorf("%s: %o %v", in, m, ok)
		}
	}
	if _, ok := ParseMode("rwx"); ok {
		t.Errorf("rwx read as a mode")
	}
}
