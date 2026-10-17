package orgs

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/ihdavids/orgs/internal/common/dnd/randgen"
	"github.com/ihdavids/orgs/internal/common/mapgen"
)

// Generated maps (internal/common/mapgen): the generator's settings, a map
// drawn from params for the editor to show, names for places, and keeping a
// map's picture in step with the params in its org file. Saving and loading
// a map is saving and loading a dungeon (dnddungeon.go) whose World is set.

// The last few maps drawn, by hash: the editor asks again for the map it
// has, whenever a setting is put back.
type drawnMap struct {
	b   []byte
	ext string // ".jpg", or ".svg" for a town
}

var worldCache = struct {
	sync.Mutex
	keys []string
	pics map[string]drawnMap
}{pics: map[string]drawnMap{}}

// drawWorld draws a map from its params (or hands back the copy drawn last
// time), with the extension it is kept under, and its hash.
func drawWorld(p mapgen.Params, strokes []mapgen.Stroke) (drawnMap, string, error) {
	h := mapgen.Hash(p, strokes)
	worldCache.Lock()
	if d, ok := worldCache.pics[h]; ok {
		worldCache.Unlock()
		return d, h, nil
	}
	worldCache.Unlock()
	b, ext, err := mapgen.Picture(p, strokes)
	if err != nil {
		return drawnMap{}, h, err
	}
	d := drawnMap{b, ext}
	worldCache.Lock()
	worldCache.pics[h] = d
	worldCache.keys = append(worldCache.keys, h)
	for len(worldCache.keys) > 12 {
		delete(worldCache.pics, worldCache.keys[0])
		worldCache.keys = worldCache.keys[1:]
	}
	worldCache.Unlock()
	return d, h, nil
}

// worldPicture draws a map for saving, and notes in it what it was drawn from.
func worldPicture(wd *randgen.World) (drawnMap, error) {
	wd.Params = wd.Params.Normal()
	d, h, err := drawWorld(wd.Params, wd.Strokes)
	wd.Hash = h
	return d, err
}

// writeWorldPicture puts a map's picture in the maps folder beside its org
// file, taking away one of another kind drawn before (a region made a town),
// and answers its path relative to the org file.
func writeWorldPicture(dir, stem string, d drawnMap) (string, error) {
	maps := filepath.Join(dir, "maps")
	if err := os.MkdirAll(maps, 0755); err != nil {
		return "", err
	}
	for _, e := range append(pictureExts, ".svg") {
		if e != d.ext {
			os.Remove(filepath.Join(maps, stem+"-map"+e))
		}
	}
	if err := os.WriteFile(filepath.Join(maps, stem+"-map"+d.ext), d.b, 0644); err != nil {
		return "", err
	}
	return "maps/" + stem + "-map" + d.ext, nil
}

// worldProps reads the params a map's org file keeps on its Map heading.
func worldProps(lines []string) map[string]string {
	out := map[string]string{}
	from, to, ok := dungeonMapHeading(lines)
	if !ok {
		return out
	}
	ps, pe, ok := drawerAt(lines, from, to, "PROPERTIES")
	if !ok {
		return out
	}
	for k := ps + 1; k < pe; k++ {
		if m := propLineRe.FindStringSubmatch(lines[k]); m != nil {
			out[strings.ToUpper(m[2])] = strings.TrimSpace(m[4])
		}
	}
	return out
}

// refreshWorld brings a map up to its org file: params changed there (or a
// picture gone missing) mean the map is drawn again and its data saved.
// A param that is not a number is left as it was, and said so (warn).
func refreshWorld(org string, d *randgen.Dungeon) (warn string, err error) {
	wd := d.World
	p, perr := mapgen.FromProps(wd.Params, worldProps(fileLines(org)))
	if perr != nil {
		warn = "The map's settings in the org file: " + perr.Error()
	}
	hash := mapgen.Hash(p, wd.Strokes)
	dir := filepath.Dir(org)
	picOK := false
	if d.Backdrop != nil {
		src := filepath.FromSlash(d.Backdrop.Src)
		if src != "" && !filepath.IsAbs(src) && !strings.Contains(src, "..") {
			if _, e := os.Stat(filepath.Join(dir, src)); e == nil {
				picOK = true
			}
		}
	}
	if hash == wd.Hash && picOK {
		return warn, nil
	}
	wd.Params = p
	pic, err := worldPicture(wd)
	if err != nil {
		return warn, err
	}
	rel, err := writeWorldPicture(dir, filepath.Base(strings.TrimSuffix(org, ".org")), pic)
	if err != nil {
		return warn, err
	}
	w, h := mapgen.ImageSize(p)
	d.FitWorld(rel, w, h)
	data, err := json.MarshalIndent(d, "", " ")
	if err != nil {
		return warn, err
	}
	return warn, os.WriteFile(strings.TrimSuffix(org, ".org")+".dungeon.json", data, 0644)
}

/* SDOC: API
* GET /mapgen — The Map Generator's Settings
	The settings of the map generators, for a form: each with its id, its property name in a
	map's org file, a label, a group, its kind (range, number, angle, select), range or choices,
	and which generator it belongs to (=gen=: terrain, hex, or empty for both). Also the kinds
	of terrain that can be painted, the defaults, and the kinds of place (on a region or world map,
	and in a town).

	*Method:* =GET=

	*Response:* ={ok, fields, strokes, defaults, places, cityPlaces}=
	EDOC */
func RequestMapgenFields(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	dndJson(w, map[string]interface{}{"ok": true, "fields": mapgen.Fields(), "strokes": mapgen.StrokeKinds,
		"defaults": mapgen.Defaults(), "places": []string{"city", "town", "village", "castle", "port", "ruin", "camp", "temple", "tower", "cave"},
		"cityPlaces": []string{"tavern", "inn", "temple", "shop", "smithy", "guild", "manor", "barracks", "market", "well", "gate", "tower"}})
}

/* SDOC: API
* POST /mapgen/render — Draw a Map
	Draws a map from its params and painted terrain, as a jpeg - what the map editor shows while
	the settings change. The last dozen maps drawn are kept, so asking again is quick.

	*Method:* =POST=

	*Request Body:* ={params, strokes: [{x, y, r, kind}]}=, as =GET /mapgen= describes the params;
	strokes in fractions of the map's width and height.

	*Response:* the map, =image/jpeg= (a town: =image/svg+xml=), with =X-Map-Width=, =X-Map-Height= and
	=X-Map-Hash=.
	EDOC */
func PostMapgenRender(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	var req struct {
		Params  mapgen.Params   `json:"params"`
		Strokes []mapgen.Stroke `json:"strokes"`
	}
	if !dndBody(w, r, &req) {
		return
	}
	d, h, err := drawWorld(req.Params, req.Strokes)
	if err != nil {
		dndError(w, http.StatusInternalServerError, "%s", err)
		return
	}
	b := d.b
	pw, ph := mapgen.ImageSize(req.Params)
	if d.ext == ".svg" {
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Header().Set("Content-Security-Policy", svgPolicy)
	} else {
		w.Header().Set("Content-Type", "image/jpeg")
	}
	w.Header().Set("X-Map-Width", strconv.Itoa(pw))
	w.Header().Set("X-Map-Height", strconv.Itoa(ph))
	w.Header().Set("X-Map-Hash", h)
	w.Header().Set("Access-Control-Expose-Headers", "X-Map-Width, X-Map-Height, X-Map-Hash")
	w.Write(b)
}

/* SDOC: API
* GET /mapgen/names — Names for Places
	Names for places on a map, English-sounding ("Ashford", "Kingsbridge-on-Lune"), by kind.

	*Method:* =GET=

	*Query Parameters:*
	| Parameter | Type   | Required | Description                                        |
	|-----------+--------+----------+----------------------------------------------------|
	| =kind=    | string | no       | town (the default), village, city, castle or ruin. |
	| =n=       | int    | no       | How many (10, at most 100).                        |
	| =seed=    | int    | no       | The same seed, the same names.                     |

	*Response:* ={ok, names: [...]}=
	EDOC */
func RequestMapgenNames(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	n, _ := strconv.Atoi(r.URL.Query().Get("n"))
	if n <= 0 {
		n = 10
	}
	n = min(n, 100)
	seed, _ := strconv.ParseUint(r.URL.Query().Get("seed"), 10, 64)
	names := []string{}
	seen := map[string]bool{}
	for i := uint64(0); len(names) < n && i < uint64(n)*5; i++ {
		name := mapgen.PlaceName(seed*1000003+i, r.URL.Query().Get("kind"))
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	dndJson(w, map[string]interface{}{"ok": true, "names": names})
}

var _ = fmt.Sprint

// svgPolicy is sent with every SVG a map is drawn as: a picture runs nothing.
const svgPolicy = "default-src 'none'; style-src 'unsafe-inline'; img-src data:"
