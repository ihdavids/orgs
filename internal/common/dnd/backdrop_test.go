// Tests for the sheet backdrop: the list of pictures a property can name, the
// folder that stands for a pile of them, and the dwell time between changes.
package dnd

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseImageRefs(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", []string{}},
		{"art/tavern.jpg", []string{"art/tavern.jpg"}},
		// commas and whitespace both separate, in any mixture
		{"a.png, b.png", []string{"a.png", "b.png"}},
		{"a.png b.png", []string{"a.png", "b.png"}},
		{" a.png ,,  b.png\n c.png ", []string{"a.png", "b.png", "c.png"}},
		// a bracket link is read whole, description and all
		{"[[file:art/a b.png][A Tavern]], b.png",
			[]string{"art/a b.png", "b.png"}},
		{"<file:~/pics/woods.gif> https://example.com/x.png",
			[]string{"~/pics/woods.gif", "https://example.com/x.png"}},
		// a url may carry commas inside a query, so wrap it to keep it whole
		{"[[https://example.com/x.png?a=1,2]]", []string{"https://example.com/x.png?a=1,2"}},
		// a bare data uri holds a comma of its own and still ends at a space
		{"data:image/gif;base64,R0lGOD a.png",
			[]string{"data:image/gif;base64,R0lGOD", "a.png"}},
		// an unterminated link is still worth reading rather than dropping
		{"[[file:art/a.png", []string{"art/a.png"}},
	}
	for _, c := range cases {
		if got := ParseImageRefs(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("ParseImageRefs(%q) = %#v, want %#v", c.in, got, c.want)
		}
	}
}

// A folder is the point of the feature: drop another picture in and the sheet
// has another backdrop, with no property to edit.
func TestBackdropRefsExpandsAFolder(t *testing.T) {
	dir := t.TempDir()
	art := filepath.Join(dir, "art")
	os.MkdirAll(filepath.Join(art, "nested"), 0755)
	for _, name := range []string{"b.png", "a.jpg", "notes.txt", ".hidden.png"} {
		os.WriteFile(filepath.Join(art, name), []byte("x"), 0644)
	}
	os.WriteFile(filepath.Join(art, "nested", "deep.png"), []byte("x"), 0644)

	got := BackdropRefs("art", dir)
	want := []string{"art/a.jpg", "art/b.png"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("BackdropRefs(folder) = %#v, want %#v", got, want)
	}
	// A folder among other references does not swallow them.
	got = BackdropRefs("https://example.com/x.png, art", dir)
	want = []string{"https://example.com/x.png", "art/a.jpg", "art/b.png"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("BackdropRefs(url + folder) = %#v, want %#v", got, want)
	}
	// An absolute folder yields absolute files.
	got = BackdropRefs(art, "")
	if len(got) != 2 || !strings.HasSuffix(got[0], "art/a.jpg") {
		t.Errorf("BackdropRefs(absolute folder) = %#v", got)
	}
}

func TestParseCycle(t *testing.T) {
	cases := []struct {
		in       string
		min, max int
	}{
		{"", 5 * 60, 25 * 60},
		{"8m", 8 * 60, 8 * 60},
		{"45s", 45, 45},
		{"10", 10 * 60, 10 * 60}, // a bare number is minutes
		{"4m-12m", 4 * 60, 12 * 60},
		{"12m - 4m", 4 * 60, 12 * 60}, // written backwards, read forwards
		{"1h", 25 * 60, 25 * 60},      // nothing waits longer than the cap
		{"2h-3h", 25 * 60, 25 * 60},
		{"0.5s", 5, 5}, // nor shorter than the floor
		{"off", 0, 0},
		{"none", 0, 0},
		{"0", 0, 0},
		{"whenever", 5 * 60, 25 * 60},
	}
	for _, c := range cases {
		min, max := ParseCycle(c.in)
		if min != c.min || max != c.max {
			t.Errorf("ParseCycle(%q) = %d, %d, want %d, %d", c.in, min, max, c.min, c.max)
		}
	}
}

func TestParseOpacity(t *testing.T) {
	cases := []struct {
		in   string
		want float64
	}{
		{"", 0.14},
		{"0.2", 0.2},
		{"20%", 0.2},
		{"20", 0.2}, // a number over one was meant as a percentage
		{"1", 1},
		{"nonsense", 0.14},
		{"-3", 0.14},
	}
	for _, c := range cases {
		if got := ParseOpacity(c.in); got != c.want {
			t.Errorf("ParseOpacity(%q) = %v, want %v", c.in, got, c.want)
		}
	}
	// The default is written back out as nothing, so an untouched sheet keeps
	// an empty property rather than growing one.
	if got := FormatOpacity(ParseOpacity("")); got != "" {
		t.Errorf("FormatOpacity(default) = %q, want empty", got)
	}
	if got := FormatOpacity(0.2); got != "20%" {
		t.Errorf("FormatOpacity(0.2) = %q, want 20%%", got)
	}
}

// One picture has nothing to change to, so a sheet carrying one does not ask
// the page to cycle at all.
func TestPlanBackdropOnlyCyclesWithMoreThanOne(t *testing.T) {
	s := &Sheet{}
	PlanBackdrop(s, "a.png", "4m", 0, "")
	if s.BackdropMinSeconds != 0 || s.BackdropMaxSeconds != 0 {
		t.Errorf("one backdrop cycles: %d, %d", s.BackdropMinSeconds, s.BackdropMaxSeconds)
	}
	if s.BackdropOpacity != 0.14 {
		t.Errorf("BackdropOpacity = %v, want the default", s.BackdropOpacity)
	}
	PlanBackdrop(s, "a.png, b.png", "4m", 0.25, "")
	if s.BackdropMinSeconds != 240 || s.BackdropMaxSeconds != 240 {
		t.Errorf("two backdrops: %d, %d", s.BackdropMinSeconds, s.BackdropMaxSeconds)
	}
	if s.BackdropOpacity != 0.25 {
		t.Errorf("BackdropOpacity = %v, want 0.25", s.BackdropOpacity)
	}
}

// A backdrop that cannot be read is worth saying out loud, but it must not
// take the ones that can down with it.
func TestResolveBackdropsSkipsWhatItCannotRead(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.png"), []byte("not really a png"), 0644)
	srcs, warnings := ResolveBackdrops(
		[]string{"a.png", "gone.png", "https://example.com/x.png"}, dir)
	if len(srcs) != 2 {
		t.Fatalf("srcs = %#v, want the readable two", srcs)
	}
	if !strings.HasPrefix(srcs[0], "data:image/png;base64,") {
		t.Errorf("local backdrop not inlined: %q", srcs[0])
	}
	if srcs[1] != "https://example.com/x.png" {
		t.Errorf("url backdrop = %q", srcs[1])
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "backdrop gone.png") {
		t.Errorf("warnings = %#v", warnings)
	}
}

// The property drawer is the only copy of a backdrop, so it has to survive a
// round trip through the org file.
func TestBackdropRoundTripsThroughOrg(t *testing.T) {
	rs := srd(t)
	c := &Character{
		Name: "Lyra", Race: "elf", Classes: []ClassLevel{{Class: "druid", Level: 3}},
		Backdrop:        "[[file:art/woods.png]], https://example.com/x.png",
		BackdropCycle:   "4m-12m",
		BackdropOpacity: 0.2,
	}
	back, err := ParseOrg(RenderOrg(c, rs), rs)
	if err != nil {
		t.Fatalf("ParseOrg: %s", err)
	}
	if back.Backdrop != c.Backdrop {
		t.Errorf("Backdrop = %q, want %q", back.Backdrop, c.Backdrop)
	}
	if back.BackdropCycle != c.BackdropCycle {
		t.Errorf("BackdropCycle = %q, want %q", back.BackdropCycle, c.BackdropCycle)
	}
	if back.BackdropOpacity != c.BackdropOpacity {
		t.Errorf("BackdropOpacity = %v, want %v", back.BackdropOpacity, c.BackdropOpacity)
	}
}
