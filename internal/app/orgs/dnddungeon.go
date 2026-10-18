package orgs

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/ihdavids/orgs/internal/common/dnd/randgen"
	"github.com/ihdavids/orgs/internal/common/mapgen"
)

// The random dungeon (randgen/dungeon.go): generating one, telling the editor
// what a changed map's exits are, and saving it as an org file with its maps
// and its editable data beside it.

/* SDOC: API
* GET /dnd/dungeon — The Dungeon Generator's Settings
	The settings form of the random dungeon generator and the props the editor can place.

	*Method:* =GET=

	*Response:* ={ok, fields: [...], props: [...]}=, fields as =GET /dnd/generate= describes them.
	EDOC */
func RequestDndDungeonFields(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	dndJson(w, map[string]interface{}{"ok": true, "fields": randgen.DungeonFields(), "props": randgen.PropKinds})
}

/* SDOC: API
* POST /dnd/dungeon — Generate a Random Dungeon
	A dungeon - map, rooms, doors, stairs, props, a title and backstory, a key and a wandering
	monster table - stocked for the party. The same =seed= and options give the same dungeon.
	The map is data, not a picture: worg draws and edits it.

	*Method:* =POST=

	*Request Body:* ={name, seed, options: {name: value}, ruleset}=

	*Response:* ={ok, dungeon, exits}=, exits being each room's ways out by room id.
	EDOC */
func PostDndDungeon(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	var req struct {
		randgen.DungeonRequest
		Ruleset string `json:"ruleset"`
	}
	if !dndBody(w, r, &req) {
		return
	}
	d := randgen.GenerateDungeon(dndRuleset(req.Ruleset), req.DungeonRequest)
	dndJson(w, map[string]interface{}{"ok": true, "dungeon": d, "exits": d.Exits()})
}

/* SDOC: API
* POST /dnd/dungeon/exits — Ways Out of Each Room
	Reads each room's exits off a dungeon map as it is now, for an editor that has just
	changed the map.

	*Method:* =POST=

	*Request Body:* ={dungeon}=

	*Response:* ={ok, exits: {roomId: [{label, text}]}}=
	EDOC */
func PostDndDungeonExits(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	var req struct {
		Dungeon randgen.Dungeon `json:"dungeon"`
	}
	if !dndBody(w, r, &req) {
		return
	}
	dndJson(w, map[string]interface{}{"ok": true, "exits": req.Dungeon.Exits()})
}

type dndDungeonSave struct {
	Dungeon   randgen.Dungeon `json:"dungeon"`
	Filename  string          `json:"filename"`
	Overwrite bool            `json:"overwrite"`
	// The pictures, drawn by the editor: the GM's map, the players' map
	// (secret doors and traps left out) and the one-page sheet.
	Map    string `json:"map"`
	Player string `json:"player"`
	Page   string `json:"page"`
	// A new picture for the dungeon's backdrop, base64: written beside the
	// org file in place of the drawings, and the backdrop pointed at it.
	Picture string `json:"picture,omitempty"`
}

// pictureExt says what a picture is by its first bytes, which is the only
// thing trusted about it: no svg, nothing that runs.
func pictureExt(b []byte) string {
	switch {
	case bytes.HasPrefix(b, []byte{0xFF, 0xD8, 0xFF}):
		return ".jpg"
	case bytes.HasPrefix(b, []byte("\x89PNG\r\n\x1a\n")):
		return ".png"
	case len(b) > 12 && string(b[:4]) == "RIFF" && string(b[8:12]) == "WEBP":
		return ".webp"
	case bytes.HasPrefix(b, []byte("GIF8")):
		return ".gif"
	}
	return ""
}

var pictureExts = []string{".jpg", ".png", ".webp", ".gif"}

// relPicture is a backdrop's path as the dungeon keeps it: under the org
// file's folder, never out of it.
func relPicture(dir, rel string) (string, bool) {
	rel = filepath.FromSlash(strings.TrimSpace(rel))
	if rel == "" || filepath.IsAbs(rel) || strings.Contains(rel, "..") {
		return "", false
	}
	p := filepath.Join(dir, rel)
	if _, ok := insideOrgDirs(p); !ok {
		return "", false
	}
	// An svg is a town the map generator drew; it is served with a policy
	// that lets it run nothing (svgPolicy).
	for _, e := range append(pictureExts, ".svg") {
		if strings.EqualFold(filepath.Ext(p), e) {
			return p, true
		}
	}
	return "", false
}

/* SDOC: API
* POST /dnd/dungeon/save — Save a Dungeon as an Org File
	Writes =name.org= (title, backstory, settings, wandering monsters, and a heading per room
	with its exits worked out from the map), the pictures =maps/name-map.svg=,
	=maps/name-player.svg= and =maps/name-page.svg= in a =maps= folder beside it, and
	=name.dungeon.json=, the data the editor and the map viewer open it from. The
	file must be inside an org directory and end in =.org=; an existing file is refused
	unless =overwrite= is set.

	A dungeon with a =backdrop= (a picture it is shown as) has no drawings: =picture=, the
	picture's bytes in base64 (jpeg, png, webp or gif), is written as =maps/name-map.jpg= (or
	=.png=...) and the backdrop pointed at it, and any drawings saved before are removed. Saved
	again without =picture=, the backdrop must already name a picture beside the file.

	*Method:* =POST=

	*Request Body:* ={dungeon, filename, overwrite, map, player, page, picture}=

	*Response:* ={Ok, Msg, Filename, Backdrop}=, =Backdrop= being where the picture is, if it has one.
	EDOC */
func PostDndDungeonSave(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	var req dndDungeonSave
	if !dndBody(w, r, &req) {
		return
	}
	fname, ok := insideOrgDirs(strings.TrimSpace(req.Filename))
	if !ok {
		dndError(w, http.StatusBadRequest, "the file has to be inside one of the org directories")
		return
	}
	if filepath.Ext(fname) != ".org" {
		dndError(w, http.StatusBadRequest, "the file has to end in .org")
		return
	}
	if !req.Overwrite {
		if _, err := os.Stat(fname); err == nil {
			dndError(w, http.StatusConflict, "%s already exists", filepath.Base(fname))
			return
		}
	}
	pics := map[string]string{"-map.svg": req.Map, "-player.svg": req.Player, "-page.svg": req.Page}
	var picture []byte
	pictureType := ""
	if wd := req.Dungeon.World; wd != nil {
		// A generated map: the server draws its picture, from its params.
		pics = map[string]string{}
		pic, err := worldPicture(wd)
		if err != nil {
			dndError(w, http.StatusInternalServerError, "could not draw the map: %s", err)
			return
		}
		picture, pictureType = pic.b, pic.ext
		pw, ph := mapgen.ImageSize(wd.Params)
		req.Dungeon.FitWorld("", pw, ph)
	} else if b := req.Dungeon.Backdrop; b != nil {
		// A dungeon shown as a picture has no drawings of its own.
		pics = map[string]string{}
		if req.Picture != "" {
			raw, err := base64.StdEncoding.DecodeString(req.Picture)
			if err != nil {
				dndError(w, http.StatusBadRequest, "the picture is not base64: %s", err)
				return
			}
			if pictureType = pictureExt(raw); pictureType == "" {
				dndError(w, http.StatusBadRequest, "the picture has to be a jpeg, png, webp or gif")
				return
			}
			picture = raw
		} else if _, ok := relPicture(filepath.Dir(fname), b.Src); !ok {
			dndError(w, http.StatusBadRequest, "the dungeon's picture has to be sent with it")
			return
		}
	}
	for _, svg := range pics {
		if svg != "" {
			if err := safeSvg(svg); err != nil {
				dndError(w, http.StatusBadRequest, "%s", err)
				return
			}
		}
	}
	// A dungeon saved over itself keeps its fog of war and its pin, as long
	// as the map is still the same size (the fog is a cell per bit).
	var keep *dungeonState
	if old, err := os.ReadFile(strings.TrimSuffix(fname, ".org") + ".dungeon.json"); err == nil {
		var prev randgen.Dungeon
		if json.Unmarshal(old, &prev) == nil && prev.Rows == req.Dungeon.Rows && prev.Cols == req.Dungeon.Cols {
			st := readDungeonState(fileLines(fname))
			if st.Fog != "" || len(st.Party) == 2 || st.FogGM || st.FogPlayers || st.Combat != "" || st.Track || st.Left != "" || st.Roving != "" || st.Measure != "" || st.Trail != "" || st.Camera != "" || st.Fx != "" || st.Tokens != "" {
				keep = &st
			}
		}
	}
	if err := os.MkdirAll(filepath.Dir(fname), 0755); err != nil {
		dndError(w, http.StatusInternalServerError, "%s", err)
		return
	}
	base := strings.TrimSuffix(fname, ".org")
	stem := filepath.Base(base)
	// The pictures go in a maps folder beside the file, so a folder of
	// dungeons stays a folder of dungeons.
	mapsDir := filepath.Join(filepath.Dir(fname), "maps")
	links := [][2]string{}
	if b := req.Dungeon.Backdrop; b != nil {
		if picture != nil {
			if err := os.MkdirAll(mapsDir, 0755); err != nil {
				dndError(w, http.StatusInternalServerError, "%s", err)
				return
			}
			for _, e := range append(pictureExts, ".svg") {
				os.Remove(filepath.Join(mapsDir, stem+"-map"+e))
			}
			if err := os.WriteFile(filepath.Join(mapsDir, stem+"-map"+pictureType), picture, 0644); err != nil {
				dndError(w, http.StatusInternalServerError, "%s", err)
				return
			}
			b.Src = "maps/" + stem + "-map" + pictureType
		}
		// The picture replaces the drawings, so the old ones go.
		for _, e := range []string{"-map.svg", "-player.svg", "-page.svg"} {
			os.Remove(filepath.Join(mapsDir, stem+e))
		}
		links = append(links, [2]string{"Map", filepath.ToSlash(b.Src)})
	}
	for _, p := range [][2]string{{"Map", "-map.svg"}, {"Players' map", "-player.svg"}, {"One page", "-page.svg"}} {
		svg := pics[p[1]]
		if svg == "" {
			continue
		}
		if err := os.MkdirAll(mapsDir, 0755); err != nil {
			dndError(w, http.StatusInternalServerError, "%s", err)
			return
		}
		if err := os.WriteFile(filepath.Join(mapsDir, stem+p[1]), []byte(svg), 0644); err != nil {
			dndError(w, http.StatusInternalServerError, "%s", err)
			return
		}
		links = append(links, [2]string{p[0], "maps/" + stem + p[1]})
	}
	// Stairs to other maps are kept relative to this file's folder.
	req.Dungeon.RelinkStairs(filepath.Dir(fname), true)
	data, err := json.MarshalIndent(&req.Dungeon, "", " ")
	if err != nil {
		dndError(w, http.StatusBadRequest, "%s", err)
		return
	}
	if err := os.WriteFile(base+".dungeon.json", data, 0644); err != nil {
		dndError(w, http.StatusInternalServerError, "%s", err)
		return
	}
	org := req.Dungeon.OrgFile(links, stem+".dungeon.json")
	if req.Dungeon.World != nil {
		org = req.Dungeon.WorldOrgFile(req.Dungeon.Backdrop.Src, stem+".dungeon.json")
	}
	if err := os.WriteFile(fname, []byte(org), 0644); err != nil {
		dndError(w, http.StatusInternalServerError, "%s", err)
		return
	}
	if keep != nil {
		if err := writeDungeonState(fname, *keep); err != nil {
			fmt.Fprintf(os.Stderr, "dungeon save: could not keep the fog: %s\n", err)
		}
	}
	GetDb().ReloadFile(fname)
	out := map[string]interface{}{"Ok": true, "Msg": fmt.Sprintf("Saved %s", filepath.Base(fname)), "Filename": fname}
	if req.Dungeon.Backdrop != nil {
		// Where the picture went, for an editor still showing it from memory.
		out["Backdrop"] = req.Dungeon.Backdrop.Src
	}
	dndJson(w, out)
}

/* SDOC: API
* GET /dnd/dungeon/image — A Dungeon's Picture
	The picture a dungeon is shown as (a Dungeon Alchemist export, or any image), by the path
	its data keeps, relative to the dungeon's org file.

	*Method:* =GET=

	*Query Parameters:*
	| Parameter | Type   | Required | Description                                   |
	|-----------+--------+----------+-----------------------------------------------|
	| =file=    | string | yes      | The dungeon's org file.                       |
	| =path=    | string | yes      | The picture, relative to it (=maps/x-map.jpg=). |

	A path starting =@shared/= is in the shared folder (=dndSharedPath=) instead: its =tokens= or
	=images=, pictures or (tokens) videos.

	*Response:* the picture.
	EDOC */
func RequestDndDungeonImage(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	// A token or picture in the shared folder (dndSharedPath).
	if rel := r.URL.Query().Get("path"); strings.HasPrefix(rel, sharedPrefix) {
		var p string
		var ok bool
		for _, folder := range []string{"tokens", "images"} {
			if p, _, ok = resolveShow("", folder, rel); ok {
				break
			}
		}
		ext := strings.ToLower(filepath.Ext(p))
		if !ok || !(showExts[ext] || videoExts[ext]) {
			dndError(w, http.StatusBadRequest, "not a picture in the shared folder")
			return
		}
		if !serveShow(w, r, p) {
			dndError(w, http.StatusNotFound, "no picture at %s", rel)
		}
		return
	}
	fname, ok := insideOrgDirs(strings.TrimSpace(r.URL.Query().Get("file")))
	if !ok {
		dndError(w, http.StatusBadRequest, "the file has to be inside one of the org directories")
		return
	}
	p, ok := relPicture(filepath.Dir(fname), r.URL.Query().Get("path"))
	if !ok {
		dndError(w, http.StatusBadRequest, "not a picture beside the dungeon")
		return
	}
	// A file, or a picture inside a zip in a tokens or images folder.
	if !serveShow(w, r, p) {
		dndError(w, http.StatusNotFound, "no picture at %s", r.URL.Query().Get("path"))
	}
}

/* SDOC: API
* POST /dnd/dungeon/stock — Fill In Rooms
	What is in each room - its purpose, how it looks, a monster, trap, treasure or feature - as a
	generated dungeon's rooms are filled, for rooms marked on a map from somewhere else. Room 1
	is the way in. Optionally a wandering monster table and a name and backstory too.

	*Method:* =POST=

	*Request Body:* ={seed, options, rooms: [{id, w, h, shape, hints}], wandering, story, ruleset}=,
	options as =GET /dnd/dungeon= lists them (level, party size and motif matter); =w= and =h=
	are a room's size in squares, 0 to leave its size out. =hints= ={area, lights, colour}= is
	what a map from elsewhere says of the room (a Dungeon Alchemist export's lights): a room with
	hints gets a Light line - its lights, or darkness - and a sentence of it in its note, and an
	irregular one its area.

	*Response:* ={ok, rooms: [{id, name, note, text}], wandering, name, story}=
	EDOC */
func PostDndDungeonStock(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	var req struct {
		randgen.StockRequest
		Ruleset string `json:"ruleset"`
	}
	if !dndBody(w, r, &req) {
		return
	}
	out := randgen.StockDungeon(dndRuleset(req.Ruleset), req.StockRequest)
	dndJson(w, map[string]interface{}{"ok": true, "rooms": out.Rooms, "wandering": out.Wandering, "name": out.Name, "story": out.Story})
}

// safeSvg refuses a picture that could run anything: it is written into the
// org directories and served from there, so it must be only a drawing. It is
// read as xml and judged by its elements and attributes, not by its text, so a
// room labelled "onerror=" is still a picture.
func safeSvg(s string) error {
	t := strings.TrimSpace(s)
	if !strings.HasPrefix(t, "<svg") || !strings.HasSuffix(t, "</svg>") {
		return fmt.Errorf("a map has to be an svg picture")
	}
	dec := xml.NewDecoder(strings.NewReader(t))
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("a map has to be well formed svg: %s", err)
		}
		switch x := tok.(type) {
		case xml.StartElement:
			switch strings.ToLower(x.Name.Local) {
			// <use> is allowed: its href must point inside the picture
			// (checked with the other attributes below), and the hatching
			// draws thousands of strokes as uses of a few symbols.
			case "script", "foreignobject", "iframe", "object", "embed", "image", "a":
				return fmt.Errorf("a map may not contain <%s>", x.Name.Local)
			}
			for _, at := range x.Attr {
				name := strings.ToLower(at.Name.Local)
				val := strings.ToLower(strings.TrimSpace(at.Value))
				if strings.HasPrefix(name, "on") {
					return fmt.Errorf("a map may not contain event handlers")
				}
				if strings.Contains(val, "javascript:") || (name == "href" && !strings.HasPrefix(val, "#")) {
					return fmt.Errorf("a map may not link anywhere")
				}
			}
		case xml.ProcInst, xml.Directive:
			return fmt.Errorf("a map may not contain processing instructions or doctypes")
		}
	}
}

/* SDOC: API
* GET /dnd/dungeon/saved — Saved Dungeons
	The dungeons saved under the org directories, newest first, by their =.dungeon.json= files.

	*Method:* =GET=

	*Response:* ={ok, dungeons: [{file, name, modified}]}=, file being the org file.
	EDOC */
func RequestDndDungeonsSaved(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	type saved struct {
		File     string `json:"file"`
		Name     string `json:"name"`
		Modified int64  `json:"modified"`
		// A generated map, not a dungeon.
		World bool `json:"world,omitempty"`
	}
	out := []saved{}
	for _, dir := range Conf().Server.OrgDirs {
		filepath.WalkDir(dir, func(p string, e fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if e.IsDir() {
				if p != dir && strings.HasPrefix(e.Name(), ".") {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(e.Name(), ".dungeon.json") || len(out) >= 500 {
				return nil
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return nil
			}
			var head struct {
				Name  string          `json:"name"`
				World json.RawMessage `json:"world"`
			}
			json.Unmarshal(b, &head)
			info, _ := e.Info()
			var mod int64
			if info != nil {
				mod = info.ModTime().Unix()
			}
			out = append(out, saved{File: strings.TrimSuffix(p, ".dungeon.json") + ".org", Name: head.Name, Modified: mod,
				World: len(head.World) > 0 && string(head.World) != "null"})
			return nil
		})
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Modified > out[b].Modified })
	dndJson(w, map[string]interface{}{"ok": true, "dungeons": out})
}

/* SDOC: API
* GET /dnd/dungeon/load — Open a Saved Dungeon
	The data of a dungeon saved by =POST /dnd/dungeon/save=, for the editor.

	*Method:* =GET=

	*Query Parameters:*
	| Parameter | Type   | Required | Description                                          |
	|-----------+--------+----------+------------------------------------------------------|
	| =file=    | string | yes      | The dungeon's org file (or its =.dungeon.json=).     |

	*Response:* ={ok, dungeon, exits, state}=, state being the fog of war and the party's pin
	(see =POST /dnd/dungeon/state=).
	EDOC */
func RequestDndDungeonLoad(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	f, ok := insideOrgDirs(r.URL.Query().Get("file"))
	if !ok {
		dndError(w, http.StatusBadRequest, "the file has to be inside one of the org directories")
		return
	}
	f = strings.TrimSuffix(strings.TrimSuffix(f, ".dungeon.json"), ".org") + ".dungeon.json"
	b, err := os.ReadFile(f)
	if err != nil {
		dndError(w, http.StatusNotFound, "no saved dungeon at %s", filepath.Base(f))
		return
	}
	var d randgen.Dungeon
	if err := json.Unmarshal(b, &d); err != nil {
		dndError(w, http.StatusBadRequest, "%s is not a dungeon: %s", filepath.Base(f), err)
		return
	}
	org := strings.TrimSuffix(f, ".dungeon.json") + ".org"
	out := map[string]interface{}{"ok": true}
	if d.World != nil {
		// A map is drawn again if its params in the org file have changed,
		// or its picture has gone.
		if warn, err := refreshWorld(org, &d); err != nil {
			dndError(w, http.StatusInternalServerError, "could not draw the map: %s", err)
			return
		} else if warn != "" {
			out["warning"] = warn
		}
	}
	st := readDungeonState(fileLines(org))
	d.RelinkStairs(filepath.Dir(org), false)
	out["dungeon"], out["exits"], out["state"] = &d, d.Exits(), st
	dndJson(w, out)
}


// dungeonState is what the map viewer changes while the game is played: the
// fog of war (the explored cells, a bitset row by row, base64), where the
// party's pin stands, and whether the fog is shown on the GM's view and the
// players'. It lives in the org file, as properties of the dungeon's Map
// heading, so it travels with the notes rather than in a side file.
type dungeonState struct {
	Fog        string `json:"fog"`
	Party      []int  `json:"party"` // [row, col], or empty for the entrance
	FogGM      bool   `json:"fogGM"`
	FogPlayers bool   `json:"fogPlayers"`
	// The fight the initiative tracker is running, as worg keeps it (JSON,
	// in base64 so it stays one property line). Opaque to the server.
	Combat string `json:"combat"`
	// The players' screen follows the party, zoomed to the room it is in.
	Track bool `json:"track"`
	// The party went down (or up) linked stairs to this map: the players'
	// screen goes after it. Cleared when the party comes back.
	Left string `json:"left,omitempty"`
	// Encounters roving the map (worg's, base64 JSON like the fight): what
	// they are and where they stand now. Opaque to the server.
	Roving string `json:"roving,omitempty"`
	// Shapes measured on the map (worg's measure.ts) and the way the party
	// has come (its trail), both base64 JSON and opaque here.
	Measure string `json:"measure,omitempty"`
	Trail   string `json:"trail,omitempty"`
	// The players' camera: the box of the map their screen shows, "x,y,w,h"
	// in the picture's pixels; empty when their screen frames itself.
	Camera string `json:"camera,omitempty"`
	// The weather over the map and the spells lately cast on it (worg's
	// fx.ts, base64 JSON), for the players' screen to play.
	Fx string `json:"fx,omitempty"`
	// Tokens standing on the map (worg's tokens.ts, base64 JSON): which
	// picture, where, how big, whether the players see it.
	Tokens string `json:"tokens,omitempty"`
}

// The Map heading's extent in the file's lines: its row, and the last row
// before the next top level heading.
func dungeonMapHeading(lines []string) (from, to int, ok bool) {
	from = -1
	for i, l := range lines {
		if strings.HasPrefix(l, "* ") {
			if from >= 0 {
				return from, i - 1, true
			}
			if strings.TrimSpace(strings.SplitN(l[2:], ":", 2)[0]) == "Map" {
				from = i
			}
		}
	}
	if from >= 0 {
		return from, len(lines) - 1, true
	}
	return -1, -1, false
}

func readDungeonState(lines []string) dungeonState {
	st := dungeonState{}
	from, to, ok := dungeonMapHeading(lines)
	if !ok {
		return st
	}
	ps, pe, ok := drawerAt(lines, from, to, "PROPERTIES")
	if !ok {
		return st
	}
	for i := ps + 1; i < pe; i++ {
		m := propLineRe.FindStringSubmatch(lines[i])
		if m == nil {
			continue
		}
		val := strings.TrimSpace(m[4])
		switch strings.ToUpper(m[2]) {
		case "DND_FOG":
			st.Fog = val
		case "DND_PARTY":
			var r, c int
			if n, _ := fmt.Sscanf(val, "%d %d", &r, &c); n == 2 {
				st.Party = []int{r, c}
			}
		case "DND_FOG_GM":
			st.FogGM = val == "t" || val == "on"
		case "DND_FOG_PLAYERS":
			st.FogPlayers = val == "t" || val == "on"
		case "DND_TRACK":
			st.Track = val == "t" || val == "on"
		case "DND_COMBAT":
			st.Combat = val
		case "DND_LEFT":
			st.Left = val
		case "DND_ROVING":
			st.Roving = val
		case "DND_MEASURE":
			st.Measure = val
		case "DND_TRAIL":
			st.Trail = val
		case "DND_CAMERA":
			st.Camera = val
		case "DND_FX":
			st.Fx = val
		case "DND_TOKENS":
			st.Tokens = val
		}
	}
	return st
}

// writeDungeonState sets the four properties on the Map heading, a line edit
// of that drawer only (see Traps: line edits).
func writeDungeonState(fname string, st dungeonState) error {
	lines, err := ganttFileLines(fname)
	if err != nil {
		return err
	}
	from, to, ok := dungeonMapHeading(lines)
	if !ok {
		return fmt.Errorf("%s has no Map heading to keep the fog under", filepath.Base(fname))
	}
	lines, ps, pe, _ := ensurePropertyDrawer(lines, from, to, "  ")
	onOff := func(b bool) string {
		if b {
			return "t"
		}
		return "nil"
	}
	party := ""
	if len(st.Party) == 2 {
		party = fmt.Sprintf("%d %d", st.Party[0], st.Party[1])
	}
	for _, kv := range [][2]string{{"DND_FOG", st.Fog}, {"DND_PARTY", party}, {"DND_FOG_GM", onOff(st.FogGM)}, {"DND_FOG_PLAYERS", onOff(st.FogPlayers)}, {"DND_COMBAT", st.Combat}, {"DND_TRACK", onOff(st.Track)}} {
		lines, pe = setPropIn(lines, ps, pe, "  ", kv[0], kv[1])
	}
	// Only while the party is away, not an empty line on every map.
	if st.Left != "" {
		lines, pe = setPropIn(lines, ps, pe, "  ", "DND_LEFT", st.Left)
	} else {
		var dropped bool
		if lines, dropped = dropPropIn(lines, ps, pe, "DND_LEFT"); dropped {
			pe--
		}
	}
	// Likewise only on a map something roams, something is measured, or
	// the party has left a trail.
	for _, kv := range [][2]string{{"DND_ROVING", st.Roving}, {"DND_MEASURE", st.Measure}, {"DND_TRAIL", st.Trail}, {"DND_CAMERA", st.Camera}, {"DND_FX", st.Fx}, {"DND_TOKENS", st.Tokens}} {
		if kv[1] != "" {
			lines, pe = setPropIn(lines, ps, pe, "  ", kv[0], kv[1])
		} else {
			var dropped bool
			if lines, dropped = dropPropIn(lines, ps, pe, kv[0]); dropped {
				pe--
			}
		}
	}
	return writeLines(fname, lines)
}

var fogRe = regexp.MustCompile(`^[A-Za-z0-9+/=]*$`)
var cameraRe = regexp.MustCompile(`^([-0-9.]+(,[-0-9.]+){3})?$`)

/* SDOC: API
* POST /dnd/dungeon/state — Keep the Fog of War
	Writes what the map viewer changes during play - the explored cells, where the party's
	pin stands, and whether the fog shows on the GM's and the players' view - as
	=DND_FOG=, =DND_PARTY=, =DND_FOG_GM=, =DND_FOG_PLAYERS=, =DND_COMBAT= (the initiative
	tracker's fight), =DND_TRACK= (the players' screen follows the party room by room) and
	=DND_LEFT= (the map the party went to by linked stairs; a players' screen goes after it) and
	=DND_ROVING= (encounters roving the map), =DND_MEASURE= (shapes measured on it) and =DND_TRAIL=
	(the way the party has come), the last three worg's base64 JSON, and =DND_CAMERA= (the box of the map
	the players' screen shows, =x,y,w,h= in the picture's pixels) and =DND_FX= (the weather and recent spell
	effects, worg's base64 JSON) and =DND_TOKENS= (tokens standing on the map, worg's base64 JSON), on
	the dungeon's =Map= heading. =GET /dnd/dungeon/load= answers them back as =state=.

	*Method:* =POST=

	*Request Body:* ={file, state: {fog, party: [row, col], fogGM, fogPlayers, combat, track, left, roving, measure, trail, camera, fx, tokens}}=; =fog=
	is a bitset of explored cells, row by row, in base64, and =combat= worg's fight as base64 JSON.

	*Response:* ={Ok, Msg}=
	EDOC */
func PostDndDungeonState(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	var req struct {
		File  string       `json:"file"`
		State dungeonState `json:"state"`
	}
	if !dndBody(w, r, &req) {
		return
	}
	fname, ok := insideOrgDirs(req.File)
	if !ok || filepath.Ext(fname) != ".org" {
		dndError(w, http.StatusBadRequest, "the file has to be an org file inside one of the org directories")
		return
	}
	if !fogRe.MatchString(req.State.Fog) || !fogRe.MatchString(req.State.Combat) || !fogRe.MatchString(req.State.Roving) ||
		!fogRe.MatchString(req.State.Measure) || !fogRe.MatchString(req.State.Trail) || !cameraRe.MatchString(req.State.Camera) || !fogRe.MatchString(req.State.Fx) || !fogRe.MatchString(req.State.Tokens) {
		dndError(w, http.StatusBadRequest, "the fog, the fight and the roving encounters have to be base64")
		return
	}
	if req.State.Left != "" {
		left, ok := insideOrgDirs(req.State.Left)
		if !ok || filepath.Ext(left) != ".org" || strings.ContainsAny(left, "\r\n") {
			dndError(w, http.StatusBadRequest, "the map the party went to has to be an org file inside one of the org directories")
			return
		}
		req.State.Left = left
	}
	if err := writeDungeonState(fname, req.State); err != nil {
		dndError(w, http.StatusBadRequest, "%s", err)
		return
	}
	dndJson(w, map[string]interface{}{"Ok": true, "Msg": "kept"})
}
