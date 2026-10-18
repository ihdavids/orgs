package orgs

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/ihdavids/orgs/internal/common/dnd/randgen"
)

// An encounter from the random generator, written into a room of a saved
// dungeon. The room's key is part of the dungeon's data (.dungeon.json), and
// the org file is written from that data, so this is the editor's save in
// miniature: change the data, write the org file again, keep the play state.
// (An encounter roving a map is the map viewer's play state, DND_ROVING, and
// is written by worg through POST /dnd/dungeon/state.)

type dndEncounterRequest struct {
	File    string `json:"file"`
	Room    int    `json:"room"`
	Title   string `json:"title"`
	Summary string `json:"summary"`
	Body    string `json:"body"`
}

// encounterText is how an encounter reads in a room's key: a line naming it,
// then the generator's own body (its monster table is what the map viewer's
// Fight button reads).
func encounterText(title, summary, body string) string {
	line := "- Encounter :: " + strings.Join(strings.Fields(title), " ")
	if s := strings.Join(strings.Fields(summary), " "); s != "" {
		line += " (" + s + ")"
	}
	var out []string
	for _, l := range strings.Split(strings.TrimRight(body, "\n"), "\n") {
		// A line of stars would be a heading inside the room (see
		// TestEveryGeneratorMakesSomethingFileable); the generator never
		// writes one, but this is the server's door.
		if strings.HasPrefix(l, "*") {
			l = " " + l
		}
		out = append(out, l)
	}
	return line + "\n\n" + strings.Join(out, "\n") + "\n"
}

// rewriteDungeonOrg writes a saved dungeon's org file again from its data, with
// the links to the pictures already beside it, keeping the play state that
// lives in the org file.
func rewriteDungeonOrg(fname string, d *randgen.Dungeon) error {
	st := readDungeonState(fileLines(fname))
	base := strings.TrimSuffix(fname, ".org")
	stem := filepath.Base(base)
	d.RelinkStairs(filepath.Dir(fname), true)
	data, err := json.MarshalIndent(d, "", " ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(base+".dungeon.json", data, 0644); err != nil {
		return err
	}
	var org string
	if d.World != nil {
		org = d.WorldOrgFile(d.Backdrop.Src, stem+".dungeon.json")
	} else {
		links := [][2]string{}
		if b := d.Backdrop; b != nil {
			links = append(links, [2]string{"Map", filepath.ToSlash(b.Src)})
		} else {
			mapsDir := filepath.Join(filepath.Dir(fname), "maps")
			for _, p := range [][2]string{{"Map", "-map.svg"}, {"Players' map", "-player.svg"}, {"One page", "-page.svg"}} {
				if _, err := os.Stat(filepath.Join(mapsDir, stem+p[1])); err == nil {
					links = append(links, [2]string{p[0], "maps/" + stem + p[1]})
				}
			}
		}
		org = d.OrgFile(links, stem+".dungeon.json")
	}
	if err := os.WriteFile(fname, []byte(org), 0644); err != nil {
		return err
	}
	if err := writeDungeonState(fname, st); err != nil {
		fmt.Fprintf(os.Stderr, "dungeon encounter: could not keep the play state: %s\n", err)
	}
	GetDb().ReloadFile(fname)
	return nil
}

/* SDOC: API
* POST /dnd/dungeon/encounter — Write an Encounter into a Room
	Adds an encounter (from =POST /dnd/generate=) to the key of one room of a saved dungeon:
	a =- Encounter ::= line and the encounter's body, under whatever the room already says.
	The dungeon's data and its org file are both written again; the fog, the pin and the
	fight are kept. The map viewer's Fight button then finds the encounter's monster table.

	*Method:* =POST=

	*Request Body:* ={file, room, title, summary, body}=, =file= the dungeon's org file.

	*Response:* ={Ok, Msg}=
	EDOC */
func PostDndDungeonEncounter(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	var req dndEncounterRequest
	if !dndBody(w, r, &req) {
		return
	}
	fname, ok := insideOrgDirs(strings.TrimSpace(req.File))
	if !ok || filepath.Ext(fname) != ".org" {
		dndError(w, http.StatusBadRequest, "the file has to be an org file inside one of the org directories")
		return
	}
	if strings.TrimSpace(req.Title) == "" && strings.TrimSpace(req.Body) == "" {
		dndError(w, http.StatusBadRequest, "there is no encounter to write")
		return
	}
	raw, err := os.ReadFile(strings.TrimSuffix(fname, ".org") + ".dungeon.json")
	if err != nil {
		dndError(w, http.StatusNotFound, "%s is not a saved dungeon", filepath.Base(fname))
		return
	}
	var d randgen.Dungeon
	if err := json.Unmarshal(raw, &d); err != nil {
		dndError(w, http.StatusBadRequest, "%s is not a dungeon: %s", filepath.Base(fname), err)
		return
	}
	if d.World != nil {
		// Its params may have been edited in the org file since: read them
		// back first, or writing the file again would lose them.
		if _, err := refreshWorld(fname, &d); err != nil {
			dndError(w, http.StatusInternalServerError, "could not read the map: %s", err)
			return
		}
	}
	at := -1
	for i := range d.Rooms {
		if d.Rooms[i].Id == req.Room {
			at = i
		}
	}
	if at < 0 {
		dndError(w, http.StatusBadRequest, "%s has no room %d", filepath.Base(fname), req.Room)
		return
	}
	text := strings.TrimRight(d.Rooms[at].Text, "\n")
	if strings.TrimSpace(text) != "" {
		text += "\n\n"
	}
	d.Rooms[at].Text = text + encounterText(req.Title, req.Summary, req.Body)
	if err := rewriteDungeonOrg(fname, &d); err != nil {
		dndError(w, http.StatusInternalServerError, "%s", err)
		return
	}
	name := d.Rooms[at].Name
	if name == "" {
		name = "the room"
	}
	dndJson(w, map[string]interface{}{"Ok": true,
		"Msg": fmt.Sprintf("Wrote %s into room %d, %s, of %s", strings.TrimSpace(req.Title), req.Room, name, d.Name)})
}

/* SDOC: API
* POST /dnd/dungeon/lights — Set a Map's Lights
	Replaces the lights of a saved dungeon or map (braziers, torches, fire pits, hearths, candles,
	lanterns, glows of magic) - what the map viewer's light tool places. Only the dungeon's data
	changes; its org file says nothing about lights.

	*Method:* =POST=

	*Request Body:* ={file, lights: [{x, y, kind, range, colour}]}=, positions in the map's squares.

	*Response:* ={Ok, Msg}=
	EDOC */
func PostDndDungeonLights(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	var req struct {
		File   string           `json:"file"`
		Lights []randgen.DLight `json:"lights"`
	}
	if !dndBody(w, r, &req) {
		return
	}
	fname, ok := insideOrgDirs(strings.TrimSpace(req.File))
	if !ok || filepath.Ext(fname) != ".org" {
		dndError(w, http.StatusBadRequest, "the file has to be an org file inside one of the org directories")
		return
	}
	data := strings.TrimSuffix(fname, ".org") + ".dungeon.json"
	raw, err := os.ReadFile(data)
	if err != nil {
		dndError(w, http.StatusNotFound, "%s is not a saved dungeon", filepath.Base(fname))
		return
	}
	var d randgen.Dungeon
	if err := json.Unmarshal(raw, &d); err != nil {
		dndError(w, http.StatusBadRequest, "%s is not a dungeon: %s", filepath.Base(fname), err)
		return
	}
	for _, l := range req.Lights {
		if _, ok := randgen.LightKinds[l.Kind]; !ok {
			dndError(w, http.StatusBadRequest, "%q is not a kind of light", l.Kind)
			return
		}
	}
	if len(req.Lights) > 400 {
		dndError(w, http.StatusBadRequest, "that is too many lights for one map")
		return
	}
	d.Lights = req.Lights
	out, err := json.MarshalIndent(&d, "", " ")
	if err != nil {
		dndError(w, http.StatusInternalServerError, "%s", err)
		return
	}
	if err := os.WriteFile(data, out, 0644); err != nil {
		dndError(w, http.StatusInternalServerError, "%s", err)
		return
	}
	// The org file did not change, so nothing else says so: a players'
	// screen on another machine reads the map again on this.
	PublishReload(fname)
	dndJson(w, map[string]interface{}{"Ok": true, "Msg": fmt.Sprintf("%d lights", len(req.Lights))})
}
