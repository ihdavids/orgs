package orgs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ihdavids/orgs/internal/common"
	"github.com/ihdavids/orgs/internal/common/dnd/randgen"
)

// An encounter written into a room lands in the room's key in both the data
// and the org file, under what the room already said, and the map's play
// state (fog, fight, roving encounters) survives the org file being written
// again.
func TestEncounterIntoARoom(t *testing.T) {
	dir := t.TempDir()
	config.Server = &common.ServerSettings{OrgDirs: []string{dir}}
	odb = nil
	t.Cleanup(func() { odb = nil })

	d := randgen.Dungeon{
		Version: 1, Name: "The Test Hole", Rows: 3, Cols: 4,
		Grid: []string{"####", "#..#", "####"},
		Room: []int{0, 0, 0, 0, 0, 1, 1, 0, 0, 0, 0, 0},
		Rooms: []randgen.DRoom{{Id: 1, Name: "Cellar", Note: "Damp.", Text: "- Here :: barrels\n"}},
	}
	raw, _ := json.Marshal(&d)
	base := filepath.Join(dir, "hole")
	if err := os.WriteFile(base+".dungeon.json", raw, 0644); err != nil {
		t.Fatal(err)
	}
	org := d.OrgFile(nil, "hole.dungeon.json")
	if err := os.WriteFile(base+".org", []byte(org), 0644); err != nil {
		t.Fatal(err)
	}
	if err := writeDungeonState(base+".org", dungeonState{Fog: "AAE=", FogGM: true, Roving: "eyJ4IjoxfQ=="}); err != nil {
		t.Fatal(err)
	}

	body := "Easy encounter.\n\n| # | Monster | CR |\n|---+---------+----|\n| 3 | Goblin  | 1/4 |\n"
	code, out := call(t, PostDndDungeonEncounter, "POST", "/dnd/dungeon/encounter", nil,
		map[string]interface{}{"file": base + ".org", "room": 1, "title": "3 × Goblin", "summary": "Easy · 150 XP", "body": body})
	if code != 200 || !strings.Contains(string(out), `"Ok":true`) {
		t.Fatalf("%d %s", code, out)
	}

	raw, _ = os.ReadFile(base + ".dungeon.json")
	var got randgen.Dungeon
	json.Unmarshal(raw, &got)
	text := got.Rooms[0].Text
	if !strings.HasPrefix(text, "- Here :: barrels\n\n- Encounter :: 3 × Goblin (Easy · 150 XP)\n") || !strings.Contains(text, "| 3 | Goblin  | 1/4 |") {
		t.Errorf("room text:\n%s", text)
	}
	orgText, _ := os.ReadFile(base + ".org")
	if !strings.Contains(string(orgText), "   - Encounter :: 3 × Goblin") {
		t.Errorf("org file:\n%s", orgText)
	}
	st := readDungeonState(fileLines(base + ".org"))
	if st.Fog != "AAE=" || !st.FogGM || st.Roving != "eyJ4IjoxfQ==" {
		t.Errorf("state lost: %+v", st)
	}

	// No such room is refused, and nothing is written.
	code, _ = call(t, PostDndDungeonEncounter, "POST", "/dnd/dungeon/encounter", nil,
		map[string]interface{}{"file": base + ".org", "room": 9, "title": "x", "body": "y"})
	if code != 400 {
		t.Errorf("room 9: %d", code)
	}
}

// DND_ROVING is written only while something roams, and goes when nothing does.
func TestRovingStateComesAndGoes(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "m.org")
	os.WriteFile(f, []byte("#+TITLE: M\n\n* Map\n  :PROPERTIES:\n  :SEED:     1\n  :END:\n* Rooms\n"), 0644)
	if err := writeDungeonState(f, dungeonState{Roving: "abc=", Measure: "bWU=", Trail: "dHI=", Camera: "10,20.5,300,180"}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(f)
	if st := readDungeonState(fileLines(f)); st.Roving != "abc=" || st.Measure != "bWU=" || st.Trail != "dHI=" || st.Camera != "10,20.5,300,180" {
		t.Fatalf("not written:\n%s", b)
	}
	writeDungeonState(f, dungeonState{})
	b, _ = os.ReadFile(f)
	if strings.Contains(string(b), "DND_ROVING") || strings.Contains(string(b), "DND_TRAIL") || !strings.Contains(string(b), "* Rooms") {
		t.Fatalf("not dropped:\n%s", b)
	}
}
