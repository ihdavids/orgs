package orgs

// The map view (features.mapView, org-osm): headings with a
// place, as pins on a map.
//
// A heading's place is org's own :LOCATION: property (what Emacs's calendar
// tools and the icalendar export read), holding coordinates in any of their
// usual spellings or a maps url (geo.Parse), or words - an address, a place
// name - that the mapGeocoder looks up (mapgeocode.go). Also read:
// :LATITUDE:/:LONGITUDE:, and a record's :ADDRESS:/:ADDRESS_<label>: (the
// contacts' vcard fields), each its own pin. Coordinates found for an
// address can be written back beside it as :GEO: (mapView.geoProperty), only
// when asked, so Emacs and an offline server have them.
//
// Writes are line edits through reviewWrite (orgreview.go): the property
// drawer is made where org keeps it and follows the file's indentation.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"

	"github.com/gorilla/mux"
	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
	"github.com/ihdavids/orgs/internal/common/geo"
)

func registerMapRoutes(api *mux.Router) {
	api.HandleFunc("/map/places", RequestMapPlaces).Methods("GET")
	api.HandleFunc("/map/geocode", PostMapGeocode).Methods("POST")
	api.HandleFunc("/map/set", PostMapSet).Methods("POST")
	api.HandleFunc("/map/status", RequestMapStatus).Methods("GET")
	api.HandleFunc("/map/static", RequestMapStatic).Methods("GET")
}

// ---------------------------------------------------------------------------
// Reading places
// ---------------------------------------------------------------------------

// mapProps is a heading's properties by upper-case name, and their order.
func mapProps(sec *org.Section) ([]string, map[string]string) {
	keys, props := sectionProps(sec)
	out := map[string]string{}
	order := []string{}
	for _, k := range keys {
		u := strings.ToUpper(k)
		if _, seen := out[u]; !seen {
			order = append(order, u)
		}
		out[u] = props[k]
	}
	return order, out
}

// mapAddressKey is whether a property holds an address: ADDRESS, or a
// record's ADDRESS_HOME / ADDRESS_WORK (its label lower case).
func mapAddressKey(k string) (label string, ok bool) {
	if k == "ADDRESS" {
		return "", true
	}
	if strings.HasPrefix(k, "ADDRESS_") && len(k) > len("ADDRESS_") {
		return strings.ToLower(strings.TrimPrefix(k, "ADDRESS_")), true
	}
	return "", false
}

// mapPlacesOf is every place one heading has. The first is its main place:
// the one :GEO: belongs to and /map/set writes.
func mapPlacesOf(f *common.OrgFile, sec *org.Section, lines []string, done map[string]bool) []geo.Place {
	if sec == nil || sec.Headline == nil || sec.Headline.Properties == nil {
		return nil
	}
	order, props := mapProps(sec)
	base := geo.Place{
		Hash: sec.Hash, Title: sectionTitle(sec), Keyword: sec.Headline.Status,
		Done: sec.Headline.Status != "" && done[sec.Headline.Status],
		Tags: append([]string{}, sec.Headline.Tags...), Filename: f.Filename, Line: sec.Headline.Pos.Row,
	}
	written, hasWritten := geo.Parse(props[mapGeoProp()])
	out := []geo.Place{}
	add := func(p geo.Place) {
		if len(out) == 0 && lines != nil {
			p.Snippet = mapSnippet(lines, sec)
		}
		out = append(out, p)
	}
	address := func(p geo.Place, words string, main bool) geo.Place {
		p.Address = words
		if main && hasWritten {
			p.Has, p.Lat, p.Lon, p.How = true, written.Lat, written.Lon, "geo"
			return p
		}
		if e, ok := mapGeo.cached(words); ok {
			if e.Missing {
				p.NotFound = true
			} else {
				p.Has, p.Lat, p.Lon, p.How, p.Found = true, e.Lat, e.Lon, "geocoded", e.Name
			}
		}
		return p
	}
	if v := strings.TrimSpace(props["LOCATION"]); v != "" {
		p := base
		p.Key, p.Value = "LOCATION", v
		if ll, ok := geo.Parse(v); ok {
			p.Has, p.Lat, p.Lon, p.How = true, ll.Lat, ll.Lon, "coordinates"
		} else if !geo.IsMapUrl(v) {
			p = address(p, v, true)
		}
		add(p)
	}
	lat, lon := strings.TrimSpace(props["LATITUDE"]), strings.TrimSpace(props["LONGITUDE"])
	if lat != "" && lon != "" {
		if ll, ok := geo.Parse(lat + "," + lon); ok {
			p := base
			p.Key, p.Value = "LATITUDE", lat+", "+lon
			p.Has, p.Lat, p.Lon, p.How = true, ll.Lat, ll.Lon, "latlon"
			add(p)
		}
	}
	for _, k := range order {
		label, ok := mapAddressKey(k)
		if !ok || strings.TrimSpace(props[k]) == "" {
			continue
		}
		p := base
		p.Key, p.Value, p.Label = k, strings.TrimSpace(props[k]), label
		if ll, ok := geo.Parse(p.Value); ok {
			p.Has, p.Lat, p.Lon, p.How = true, ll.Lat, ll.Lon, "coordinates"
		} else {
			p = address(p, p.Value, len(out) == 0)
		}
		add(p)
	}
	// A heading with only :GEO: (written by hand, or its LOCATION taken out).
	if len(out) == 0 && hasWritten {
		p := base
		p.Key, p.Value = mapGeoProp(), props[mapGeoProp()]
		p.Has, p.Lat, p.Lon, p.How = true, written.Lat, written.Lon, "coordinates"
		add(p)
	}
	return out
}

var mapLinkRe = regexp.MustCompile(`\[\[(?:[^\]]+)\]\[([^\]]+)\]\]|\[\[([^\]]+)\]\]`)

// mapSnippet is the start of a heading's own text, for a pin's popup:
// drawers, planning, keywords and blocks left out, links as their words.
func mapSnippet(lines []string, sec *org.Section) string {
	from := sec.Headline.Pos.Row
	if from < 0 || from >= len(lines) {
		return ""
	}
	to := ownLinesEnd(lines, from, subtreeEndRow(lines, from, sec.Headline.Lvl, from))
	var b strings.Builder
	inDrawer, inBlock := false, false
	for i := from + 1; i <= to && i < len(lines); i++ {
		t := strings.TrimSpace(lines[i])
		u := strings.ToUpper(t)
		switch {
		case inBlock:
			if strings.HasPrefix(u, "#+END_") {
				inBlock = false
			}
			continue
		case strings.HasPrefix(u, "#+BEGIN_"):
			inBlock = true
			continue
		case inDrawer:
			if u == ":END:" {
				inDrawer = false
			}
			continue
		case strings.HasPrefix(t, ":") && strings.HasSuffix(t, ":") && len(t) > 2 && !strings.Contains(t, " "):
			inDrawer = true
			continue
		case t == "" || strings.HasPrefix(t, "#") || planningRe.MatchString(t) || strings.HasPrefix(u, "CLOCK:"):
			continue
		}
		t = mapLinkRe.ReplaceAllStringFunc(t, func(m string) string {
			s := mapLinkRe.FindStringSubmatch(m)
			if s[1] != "" {
				return s[1]
			}
			return s[2]
		})
		if b.Len() > 0 {
			b.WriteString(" ")
		}
		b.WriteString(t)
		if b.Len() > 240 {
			break
		}
	}
	s := b.String()
	if r := []rune(s); len(r) > 200 {
		s = strings.TrimSpace(string(r[:200])) + "…"
	}
	return s
}

// mapDoneKeywords is the done keywords of a file, as a set.
func mapDoneKeywords(f *common.OrgFile, memo map[string]map[string]bool) map[string]bool {
	if d, ok := memo[f.Filename]; ok {
		return d
	}
	_, done := ValidStatusFromFile(f)
	d := map[string]bool{}
	for _, k := range done {
		d[k] = true
	}
	memo[f.Filename] = d
	return d
}

// mapPlaces is the places of the headings a query finds (every heading
// with none; only file's when file is given), in file order.
func mapPlaces(query, file, colourBy string) ([]geo.Place, error) {
	out := []geo.Place{}
	cache := lineCache{}
	memo := map[string]map[string]bool{}
	visit := func(sec *org.Section, f *common.OrgFile) {
		if f == nil || sec == nil || sec.Headline == nil {
			return
		}
		if file != "" && f.Filename != file {
			return
		}
		for _, p := range mapPlacesOf(f, sec, cache.of(f.Filename), mapDoneKeywords(f, memo)) {
			p.Colour = geo.PinColour(colourBy, p.Keyword, p.Done, p.Tags)
			out = append(out, p)
		}
	}
	query = strings.TrimSpace(query)
	if query != "" {
		if err := queryEachSection(&common.StringQuery{Query: query}, visit); err != nil {
			return nil, err
		}
		return out, nil
	}
	if file != "" {
		f := GetDb().FindByFile(file)
		if f == nil {
			return nil, fmt.Errorf("no org file %q", file)
		}
		registerAllSections()
		var walk func(*org.Section)
		walk = func(s *org.Section) {
			for _, c := range s.Children {
				if isRealHeading(c) {
					visit(c, f)
				}
				walk(c)
			}
		}
		if f.Doc != nil && f.Doc.Outline.Section != nil {
			walk(f.Doc.Outline.Section)
		}
		return out, nil
	}
	registerAllSections()
	walkSections(func(f *common.OrgFile, sec *org.Section) { visit(sec, f) })
	return out, nil
}

// mapUnplaced is the addresses in places that the cache does not know.
func mapUnplaced(places []geo.Place) []string {
	out := []string{}
	for _, p := range places {
		if !p.Has && !p.NotFound && p.Address != "" {
			out = append(out, p.Address)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Endpoints
// ---------------------------------------------------------------------------

/* SDOC: API
* GET /map/places — Headings With A Place

	Needs =features: mapView:=. Every heading with a =:LOCATION:= (coordinates,
	a =geo:= uri, a maps url, or an address), =:LATITUDE:=/=:LONGITUDE:=, or a
	record's =:ADDRESS:=/=:ADDRESS_<label>:= - one entry per place, so a
	contact with a home and a work address is two pins.

	| Parameter | Description |
	|-----------+--------------------------------------------------------------|
	| =query=   | only the headings this query finds (=IsTask()=, =HasTags("travel")=) |
	| =file=    | only this file's headings |
	| =colour=  | =keyword= (default: none / to do / done) or =tag= (the first tag) |

	Each place has =Hash=, =Title=, =Keyword=, =Done=, =Tags=, =Filename=,
	=Line=, =Key= (the property), =Label= (home, work), =Value= (as
	written), =Has= with =Lat=/=Lon=, =How= (=coordinates=, =latlon=,
	=geo= - written back -, =geocoded= - looked up and cached), =Address=,
	=Found= (the geocoder's name), =NotFound=, =Snippet= and =Colour=.
	An address not looked up yet has =Has= false; =Unplaced= counts them and
	=Pending= is how many are waiting for the geocoder. With
	=mapView.autoGeocode= they are queued at once; otherwise =POST
	/map/geocode {All: true}= does it.
EDOC */
func RequestMapPlaces(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, mapFeature) {
		return
	}
	q := r.URL.Query()
	colour := q.Get("colour")
	if colour == "" {
		colour = q.Get("color")
	}
	places, err := mapPlaces(q.Get("query"), q.Get("file"), colour)
	if err != nil {
		dynJson(w, geo.PlacesAnswer{Msg: err.Error(), Places: []geo.Place{}})
		return
	}
	un := mapUnplaced(places)
	pending := mapGeo.pending()
	if mapConf().AutoGeocode && len(un) > 0 {
		pending = mapGeo.enqueue(un)
	}
	dynJson(w, geo.PlacesAnswer{Ok: true, Places: places, Unplaced: len(un), Pending: pending})
}

// MapGeocodeRequest is POST /map/geocode.
type MapGeocodeRequest struct {
	Address string
	Limit   int
	Hash    string
	Write   bool // with Hash: write the coordinates found as :GEO:
	All     bool // queue every address no lookup has answered yet
	Query   string
}

/* SDOC: API
* POST /map/geocode — Look An Address Up

	Needs =features: mapView:=. One of:

	- ={Address: "Granville Island, Vancouver", Limit: 5}= - the places the
	  mapGeocoder knows by that name, best first (=Results=: =Lat=, =Lon=,
	  =Name=, =Kind=). For "place this heading" searches.
	- ={Hash: "...", Write: true}= - looks the heading's address up again
	  (even one the cache says was not found) and, with =Write=, writes the
	  coordinates beside it as =:GEO:= (=mapView.geoProperty=). Answers the
	  =Place=.
	- ={All: true, Query: "..."}= - queues every address the cache has no
	  answer for (of the query's headings, or all) and answers at once with
	  =Pending=; they are looked up one a second, and a =map= event is sent
	  as they are placed.

	The mapGeocoder is =mapView.geocodeUrl= (Nominatim's own by default); every
	answer is kept in =<config>_geocode.json=. Unreachable: =Ok= false and
	why, and nothing is kept.
EDOC */
func PostMapGeocode(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, mapFeature) {
		return
	}
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	var req MapGeocodeRequest
	if err := json.Unmarshal(body, &req); err != nil {
		dynJson(w, common.Result{Msg: err.Error()})
		return
	}
	switch {
	case req.All:
		places, err := mapPlaces(req.Query, "", "")
		if err != nil {
			dynJson(w, common.Result{Msg: err.Error()})
			return
		}
		un := mapUnplaced(places)
		n := mapGeo.enqueue(un)
		dynJson(w, map[string]interface{}{"Ok": true, "Pending": n,
			"Msg": fmt.Sprintf("%d address%s to look up, one a second", n, mapEs(n))})
	case req.Hash != "":
		sec := GetDb().FindByHash(req.Hash)
		if sec == nil || sec.Headline == nil {
			dynJson(w, common.Result{Msg: "no heading with that hash - it may have moved; refresh and try again"})
			return
		}
		f := GetDb().FileFromSection(sec)
		if f == nil {
			dynJson(w, common.Result{Msg: "could not find the file that heading is in"})
			return
		}
		places := mapPlacesOf(f, sec, nil, map[string]bool{})
		var p *geo.Place
		for i := range places {
			if places[i].Address != "" {
				p = &places[i]
				break
			}
		}
		if p == nil {
			if len(places) > 0 && places[0].Has {
				dynJson(w, map[string]interface{}{"Ok": true, "Place": places[0], "Msg": "it has coordinates already: " + geo.Format(geo.LatLon{Lat: places[0].Lat, Lon: places[0].Lon})})
				return
			}
			dynJson(w, common.Result{Msg: "that heading has no address to look up (no :LOCATION: or :ADDRESS:)"})
			return
		}
		e, err := mapGeo.lookup(p.Address, true)
		if err != nil {
			dynJson(w, common.Result{Msg: err.Error()})
			return
		}
		if e.Missing {
			p.NotFound, p.Has = true, false
			dynJson(w, map[string]interface{}{"Ok": false, "Place": p, "Msg": fmt.Sprintf("the mapGeocoder does not know %q - try fewer words, or set coordinates", p.Address)})
			return
		}
		p.Has, p.Lat, p.Lon, p.How, p.Found, p.NotFound = true, e.Lat, e.Lon, "geocoded", e.Name, false
		msg := "found " + e.Name
		if req.Write {
			if err := reviewWrite(f, sec, []reviewProp{{mapGeoProp(), geo.Format(geo.LatLon{Lat: e.Lat, Lon: e.Lon})}}, nil); err != nil {
				dynJson(w, common.Result{Msg: err.Error()})
				return
			}
			p.How = "geo"
			msg += "; written as :" + mapGeoProp() + ":"
		}
		Publish(Event{Kind: EventMap, File: f.Filename, Hash: req.Hash})
		dynJson(w, map[string]interface{}{"Ok": true, "Place": p, "Msg": msg})
	case strings.TrimSpace(req.Address) != "":
		if ll, ok := geo.Parse(req.Address); ok {
			dynJson(w, map[string]interface{}{"Ok": true, "Results": []MapGeoHit{{Lat: ll.Lat, Lon: ll.Lon, Name: geo.Format(ll), Kind: "coordinates"}}})
			return
		}
		limit := req.Limit
		if limit <= 0 {
			limit = 5
		}
		hits, err := mapGeo.search(req.Address, limit)
		if err != nil {
			dynJson(w, common.Result{Msg: err.Error()})
			return
		}
		e := mapGeoEntry{Missing: true}
		if len(hits) > 0 {
			e = mapGeoEntry{Lat: hits[0].Lat, Lon: hits[0].Lon, Name: hits[0].Name}
		}
		mapGeo.put(req.Address, e)
		dynJson(w, map[string]interface{}{"Ok": true, "Results": hits})
	default:
		dynJson(w, common.Result{Msg: "what to look up? Address, Hash or All"})
	}
}

func mapEs(n int) string {
	if n == 1 {
		return ""
	}
	return "es"
}

// MapSetRequest is POST /map/set.
type MapSetRequest struct {
	Hash     string
	Location string
	// Geo: when Location is an address, look it up now and write the
	// coordinates as :GEO: too.
	Geo bool
	// Lat/Lon (both set): the coordinates of what Location names, already
	// known to the client (a search result picked): no lookup, and with
	// Geo they are written as :GEO:.
	Lat, Lon float64
}

/* SDOC: API
* POST /map/set — Give A Heading Its Place

	Needs =features: mapView:=. ={Hash, Location}= writes =:LOCATION:= (a line
	edit of the property drawer, made if missing); an empty =Location= takes
	it off. =Location= is written as given: "49.2827,-123.1207" from a click
	on the map, or an address. A stale =:GEO:= is taken off with it. With
	=Geo: true= an address's coordinates are written as =:GEO:= as well -
	=Lat=/=Lon= when the client has them (a search result it picked), else
	looked up now. Answers the heading's =Place= as =/map/places= would.
EDOC */
func PostMapSet(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, mapFeature) {
		return
	}
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	var req MapSetRequest
	if err := json.Unmarshal(body, &req); err != nil {
		dynJson(w, common.Result{Msg: err.Error()})
		return
	}
	sec := GetDb().FindByHash(req.Hash)
	if sec == nil || sec.Headline == nil {
		dynJson(w, common.Result{Msg: "no heading with that hash - it may have moved; refresh and try again"})
		return
	}
	f := GetDb().FileFromSection(sec)
	if f == nil {
		dynJson(w, common.Result{Msg: "could not find the file that heading is in"})
		return
	}
	loc := strings.Join(strings.Fields(req.Location), " ")
	props := []reviewProp{{"LOCATION", loc}}
	msg := "location taken off"
	geoVal := ""
	if loc != "" {
		msg = "location set to " + loc
		if _, ok := geo.Parse(loc); !ok && req.Geo {
			ll := geo.LatLon{Lat: req.Lat, Lon: req.Lon}
			if !ll.Valid() {
				e, err := mapGeo.lookup(loc, false)
				if err != nil {
					dynJson(w, common.Result{Msg: err.Error() + " - nothing written"})
					return
				}
				if e.Missing {
					dynJson(w, common.Result{Msg: fmt.Sprintf("the mapGeocoder does not know %q - nothing written", loc)})
					return
				}
				ll = geo.LatLon{Lat: e.Lat, Lon: e.Lon}
			} else {
				mapGeo.put(loc, mapGeoEntry{Lat: ll.Lat, Lon: ll.Lon, Name: loc})
			}
			geoVal = geo.Format(ll)
			msg += ", at " + geoVal
		} else if ll := (geo.LatLon{Lat: req.Lat, Lon: req.Lon}); ll.Valid() {
			// A picked search result: remembered, so the pin shows at once.
			if _, ok := geo.Parse(loc); !ok {
				mapGeo.put(loc, mapGeoEntry{Lat: ll.Lat, Lon: ll.Lon, Name: loc})
			}
		}
	}
	// :GEO: belongs to the old address: written anew or taken off.
	_, had := mapProps(sec)
	if geoVal != "" || had[mapGeoProp()] != "" {
		props = append(props, reviewProp{mapGeoProp(), geoVal})
	}
	if err := reviewWrite(f, sec, props, nil); err != nil {
		dynJson(w, common.Result{Msg: err.Error()})
		return
	}
	Publish(Event{Kind: EventMap, File: f.Filename, Hash: req.Hash})
	out := map[string]interface{}{"Ok": true, "Msg": msg}
	// The heading again: its hash may not have changed (only properties
	// did), but its row has not moved either, so find it by line.
	if nf := GetDb().FindByFile(f.Filename); nf != nil {
		if s := findHeadingByLine(nf, sec.Headline.Pos.Row); s != nil && s.Headline != nil {
			lines, _ := readFileLines(nf.Filename)
			memo := map[string]map[string]bool{}
			if ps := mapPlacesOf(nf, s, lines, mapDoneKeywords(nf, memo)); len(ps) > 0 {
				out["Place"] = ps[0]
			}
			out["Hash"] = s.Hash
		}
	}
	dynJson(w, out)
}

/* SDOC: API
* GET /map/status — The Map View's Settings And Geocoder

	Needs =features: mapView:=. What a client needs to draw the map
	(=TileUrl=, =TileDarkUrl=, =Attribution=, =GeoProperty=) and how the
	mapGeocoder is doing: =GeocodeUrl=, =AutoGeocode=, =Cached= / =Missing=
	(addresses in the cache, found and not), =Pending= (waiting their turn),
	=Asked= (lookups since the server started), =LastError= and =LastAt=.
EDOC */
func RequestMapStatus(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, mapFeature) {
		return
	}
	found, missing := mapGeo.counts()
	mapGeo.mu.Lock()
	lastErr, lastAt, asked := mapGeo.lastErr, mapGeo.lastAt, mapGeo.asked
	mapGeo.mu.Unlock()
	dynJson(w, map[string]interface{}{
		"Ok": true, "GeocodeUrl": mapGeocodeUrl(), "AutoGeocode": mapConf().AutoGeocode,
		"TileUrl": mapTileUrl(), "TileDarkUrl": strings.TrimSpace(mapConf().TileDarkUrl), "Attribution": mapAttribution(),
		"GeoProperty": mapGeoProp(), "StaticTiles": mapStaticTiles(),
		"Cached": found, "Missing": missing, "Pending": mapGeo.pending(), "Asked": asked,
		"LastError": lastErr, "LastAt": lastAt, "CacheFile": mapGeocodeCachePath(),
	})
}

// mapSortPlaces puts places in file then line order (a query answers in
// database order, which is already that per file).
func mapSortPlaces(ps []geo.Place) {
	sort.SliceStable(ps, func(i, j int) bool {
		if ps[i].Filename != ps[j].Filename {
			return ps[i].Filename < ps[j].Filename
		}
		return ps[i].Line < ps[j].Line
	})
}
