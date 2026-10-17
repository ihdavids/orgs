package orgs

import (
	"os"
	"path/filepath"
	"regexp"
	"reflect"
	"strings"
	"testing"

	"github.com/ihdavids/orgs/internal/common/dnd/randgen"
	"github.com/ihdavids/orgs/internal/common/mapgen"
)

func TestDndGenLines(t *testing.T) {
	it := randgen.Item{
		Kind: "npc", Title: "Ulgrim  Granitebeard", Summary: "Old dwarf, thug",
		Body: "- Race :: Dwarf\n\n| a | b |\n", Tags: []string{"dnd", "npc", "two words"},
		Props: map[string]string{"race": "Dwarf"},
	}
	got := dndGenLines(2, &it)
	want := []string{
		"** Ulgrim Granitebeard  :dnd:npc:two_words:",
		"   :PROPERTIES:",
		"   :GENERATOR: npc",
		"   :RACE:     Dwarf",
		"   :END:",
		"   Old dwarf, thug",
		"",
		"   - Race :: Dwarf",
		"",
		"   | a | b |",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got\n%q\nwant\n%q", got, want)
	}
}

func TestSafeSvg(t *testing.T) {
	ok := []string{
		`<svg xmlns="http://www.w3.org/2000/svg"><rect width="1" height="1"/></svg>`,
		// Text that only looks like a handler is text.
		`<svg xmlns="http://www.w3.org/2000/svg"><text>&lt;b onload=&quot;x&quot;&gt; javascript: onerror=</text></svg>`,
		`<svg xmlns="http://www.w3.org/2000/svg"><defs><pattern id="g"/></defs><path fill="url(#g)"/></svg>`,
		`<svg xmlns="http://www.w3.org/2000/svg"><symbol id="c3"><path d="M0 0"/></symbol><use href="#c3"/></svg>`,
	}
	bad := []string{
		`<svg><script>alert(1)</script></svg>`,
		`<svg><rect onload="x()"/></svg>`,
		`<svg><a href="javascript:x()"/></svg>`,
		`<svg><foreignObject/></svg>`,
		`<html></html>`,
		`<svg><image href="http://x/y.png"/></svg>`,
		`<svg><use href="other.svg#c3"/></svg>`,
		`<svg><rect/>`,
	}
	for _, s := range ok {
		if err := safeSvg(s); err != nil {
			t.Errorf("%s refused: %s", s, err)
		}
	}
	for _, s := range bad {
		if safeSvg(s) == nil {
			t.Errorf("%s allowed", s)
		}
	}
}

func TestDungeonStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	f := dir + "/keep.org"
	org := "#+TITLE: Keep\n\nOnce.\n\n* Map\n  :PROPERTIES:\n  :SEED:      4\n  :END:\n  - Map :: [[file:maps/keep-map.svg]]\n* Rooms\n** 1. Hall\n"
	if err := os.WriteFile(f, []byte(org), 0644); err != nil {
		t.Fatal(err)
	}
	if st := readDungeonState(fileLines(f)); st.Fog != "" || st.Party != nil || st.FogGM || st.FogPlayers {
		t.Fatalf("a fresh dungeon has state %+v", st)
	}
	want := dungeonState{Fog: "AP8=", Party: []int{3, 7}, FogGM: true, FogPlayers: true, Combat: "eyJyb3VuZCI6MX0=", Track: true}
	if err := writeDungeonState(f, want); err != nil {
		t.Fatal(err)
	}
	got := readDungeonState(fileLines(f))
	if !reflect.DeepEqual(got, want) {
		t.Errorf("read back %+v, want %+v", got, want)
	}
	// Only the Map heading's drawer changed: the rest of the file is as it was.
	b, _ := os.ReadFile(f)
	text := string(b)
	// (The drawer is re-aligned, as every property write does, so :SEED: may
	// gain spaces; its value may not change.)
	if !regexp.MustCompile(`:SEED:\s+4\n`).MatchString(text) {
		t.Errorf("lost the seed:\n%s", text)
	}
	for _, keep := range []string{"#+TITLE: Keep", "- Map :: [[file:maps/keep-map.svg]]", "* Rooms\n** 1. Hall"} {
		if !strings.Contains(text, keep) {
			t.Errorf("lost %q:\n%s", keep, text)
		}
	}
	// Written twice is still one of each.
	want.FogGM = false
	writeDungeonState(f, want)
	b, _ = os.ReadFile(f)
	if n := strings.Count(string(b), ":DND_FOG_GM:"); n != 1 {
		t.Errorf("%d DND_FOG_GM lines:\n%s", n, b)
	}
	if got := readDungeonState(fileLines(f)); got.FogGM {
		t.Error("DND_FOG_GM did not turn off")
	}
}

func TestPartyFile(t *testing.T) {
	dir := t.TempDir()
	f := dir + "/party.org"
	if err := os.WriteFile(f, []byte(partyHeader), 0644); err != nil {
		t.Fatal(err)
	}
	ann := partyMember{Name: "Thorin Oakenshield", Player: "Ann", AC: 18, HP: 34, Init: 1, Level: 5, Class: "Fighter", Passive: 13}
	bo := partyMember{Name: "Lirael", Player: "Bo", AC: 13, HP: 22, Init: 3, Level: 5, Class: "Wizard", Passive: 15}
	if err := writeParty(f, []partyMember{ann, bo}); err != nil {
		t.Fatal(err)
	}
	got := readParty(f)
	if len(got) != 2 || got[0].Name != "Thorin Oakenshield" || got[0].AC != 18 || got[1].Init != 3 || got[0].Id == "" {
		t.Fatalf("read back %+v", got)
	}
	// A note written under a character survives an edit to that character.
	b, _ := os.ReadFile(f)
	// (Lirael is the last heading, so the end of the file is under her drawer.)
	text := strings.TrimRight(string(b), "\n") + "\n  Carries the Book of Doors.\n"
	os.WriteFile(f, []byte(text), 0644)
	got = readParty(f)
	got[1].HP = 30
	got[1].Name = "Lirael Abhorsen"
	got = got[1:] // and Thorin leaves the party
	if err := writeParty(f, got); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(f)
	out := string(b)
	if strings.Contains(out, "Thorin") {
		t.Errorf("Thorin is still there:\n%s", out)
	}
	if !strings.Contains(out, "* Lirael Abhorsen") || !strings.Contains(out, "Carries the Book of Doors.") || !strings.Contains(out, "#+TITLE: The Party") {
		t.Errorf("lost something:\n%s", out)
	}
	if again := readParty(f); len(again) != 1 || again[0].HP != 30 {
		t.Errorf("read back %+v", again)
	}
}

// A dungeon's picture is judged by its bytes, not its name: no svg gets in as
// a "jpeg".
func TestPictureExt(t *testing.T) {
	cases := map[string]string{
		"\xFF\xD8\xFF\xE0rest":           ".jpg",
		"\x89PNG\r\n\x1a\nrest":          ".png",
		"RIFF\x00\x00\x00\x00WEBPVP8 ":   ".webp",
		"GIF89a":                         ".gif",
		"<svg onload=alert(1)>":          "",
		"":                               "",
	}
	for in, want := range cases {
		if got := pictureExt([]byte(in)); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}

// A map's params live in its org file: changed there, the map is drawn
// again when it is next opened; left alone, it is not; a word where a number
// should be is said, and the rest still taken.
func TestWorldFollowsItsOrgFile(t *testing.T) {
	dir := t.TempDir()
	org := dir + "/coast.org"
	p := mapgen.Defaults()
	p.Width, p.Height, p.Seed = 256, 192, 5
	d := randgen.Dungeon{Name: "The Coast", World: &randgen.World{Params: p, Places: []randgen.WorldPlace{{Id: 1, Kind: "town", Name: "Ashford", Note: "A market town.", X: 0.5, Y: 0.5}}}}
	if _, err := refreshWorld(org, &d); err != nil {
		t.Fatal(err)
	}
	if d.Backdrop == nil || d.Backdrop.Src != "maps/coast-map.jpg" || d.Backdrop.Width != 256 || d.Rows == 0 {
		t.Fatalf("not fitted: %+v %d", d.Backdrop, d.Rows)
	}
	if err := os.WriteFile(org, []byte(d.WorldOrgFile(d.Backdrop.Src, "coast.dungeon.json")), 0644); err != nil {
		t.Fatal(err)
	}
	text, _ := os.ReadFile(org)
	for _, want := range []string{":SEED:", ":RAIN:", "#+DND_WORLD: t", "** Ashford", ":KIND:    town", "A market town."} {
		if !strings.Contains(string(text), want) {
			t.Errorf("org file has no %q:\n%s", want, text)
		}
	}
	hash := d.World.Hash
	if _, err := refreshWorld(org, &d); err != nil || d.World.Hash != hash {
		t.Errorf("drawn again with nothing changed")
	}
	edited := regexp.MustCompile(`(?m)^(\s*:SEED:\s*)\d+`).ReplaceAllString(string(text), "${1}77")
	edited = regexp.MustCompile(`(?m)^(\s*:RAIN:\s*)[\d.]+`).ReplaceAllString(edited, "${1}lots")
	os.WriteFile(org, []byte(edited), 0644)
	warn, err := refreshWorld(org, &d)
	if err != nil || d.World.Params.Seed != 77 || d.World.Hash == hash {
		t.Errorf("an edited seed did not redraw the map: %v %+v", err, d.World.Params)
	}
	if !strings.Contains(warn, "RAIN") || d.World.Params.Rain != p.Rain {
		t.Errorf("a bad RAIN: warn %q, rain %v", warn, d.World.Params.Rain)
	}
}

// DND_LEFT is there only while the party is away on another map.
func TestDungeonStateLeft(t *testing.T) {
	f := filepath.Join(t.TempDir(), "keep.org")
	if err := os.WriteFile(f, []byte("#+TITLE: Keep\n* Map\n  [[file:maps/keep-map.svg]]\n* Rooms\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	read := func() (dungeonState, string) {
		b, _ := os.ReadFile(f)
		return readDungeonState(strings.Split(string(b), "\n")), string(b)
	}
	if err := writeDungeonState(f, dungeonState{Party: []int{3, 4}, Left: "/notes/warrens.org"}); err != nil {
		t.Fatal(err)
	}
	if st, _ := read(); st.Left != "/notes/warrens.org" || len(st.Party) != 2 {
		t.Fatalf("away: %+v", st)
	}
	if err := writeDungeonState(f, dungeonState{Party: []int{3, 4}}); err != nil {
		t.Fatal(err)
	}
	if st, text := read(); st.Left != "" || strings.Contains(text, "DND_LEFT") {
		t.Fatalf("back, but still away:\n%s", text)
	}
}
