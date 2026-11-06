package orgs

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ihdavids/orgs/internal/common"
)

// mapFakeGeocoder is a Nominatim that knows two places, on localhost: the
// tests never ask the real one.
func mapFakeGeocoder(t *testing.T) *int32 {
	t.Helper()
	var asked int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&asked, 1)
		if r.Header.Get("User-Agent") == "" || !strings.HasPrefix(r.Header.Get("User-Agent"), "orgs/") {
			t.Errorf("no User-Agent naming orgs: %q", r.Header.Get("User-Agent"))
		}
		q := strings.ToLower(r.URL.Query().Get("q"))
		switch {
		case strings.Contains(q, "granville island"):
			w.Write([]byte(`[{"lat":"49.2713","lon":"-123.1340","display_name":"Granville Island, Vancouver","category":"place","type":"island"}]`))
		case strings.Contains(q, "1 main st"):
			w.Write([]byte(`[{"lat":"49.2800","lon":"-123.1000","display_name":"1 Main Street, Vancouver","class":"building","type":"house"}]`))
		default:
			w.Write([]byte(`[]`))
		}
	}))
	t.Cleanup(srv.Close)
	old := Conf().Server.MapView
	oldCfg := Conf().Config
	oldGap := mapGeoGap
	off := false
	Conf().Server.MapView = common.MapViewSettings{GeocodeUrl: srv.URL + "/search", StaticTiles: &off}
	Conf().Config = filepath.Join(t.TempDir(), "orgs.yaml")
	mapGeoGap = 0
	mapGeo = &mapGeocoder{}
	t.Cleanup(func() {
		Conf().Server.MapView, Conf().Config, mapGeoGap = old, oldCfg, oldGap
		mapGeo = &mapGeocoder{}
	})
	return &asked
}

func mapPlacesBy(t *testing.T, q string) (map[string]map[string]interface{}, map[string]interface{}) {
	t.Helper()
	r := mediaCall(t, RequestMapPlaces, "GET", "/map/places"+q, nil)
	out := map[string]map[string]interface{}{}
	ps, _ := r["Places"].([]interface{})
	for _, p := range ps {
		m := p.(map[string]interface{})
		k := m["Title"].(string)
		if l, _ := m["Label"].(string); l != "" {
			k += "/" + l
		}
		out[k] = m
	}
	return out, r
}

func TestMapView(t *testing.T) {
	text := `#+TITLE: Places
* TODO Coffee at the corner :cafe:
:PROPERTIES:
:LOCATION: 49.2827, -123.1207
:END:
Good beans, and a [[https://example.com][website]] too.
* Market
  :PROPERTIES:
  :LOCATION: Granville Island, Vancouver
  :END:
* DONE Saw the clock :travel:
:PROPERTIES:
:LOCATION: https://www.openstreetmap.org/?mlat=51.5007&mlon=-0.1246#map=17/51.5007/-0.1246
:END:
* Old pin
:PROPERTIES:
:LATITUDE: 48.8584
:LONGITUDE: 2.2945
:END:
* Ada Lovelace
:PROPERTIES:
:RECORD: contact
:ADDRESS_HOME: 1 Main St, Vancouver
:ADDRESS_WORK: Nowhere Street 99, Atlantis
:END:
* No place here
Words.
`
	asked := mapFakeGeocoder(t)
	withFeatures(t, func(f *common.FeatureSettings) { f.MapView = false })
	paths := loadOrg(t, map[string]string{"p.org": text})
	p := paths["p.org"]
	if r := mediaCall(t, RequestMapPlaces, "GET", "/map/places", nil); r["Off"] != true {
		t.Fatalf("answered with the feature off: %v", r)
	}
	if lines, _ := readFileLines(p); true {
		if out, changed := exportLinesForMapOff(p, lines); changed {
			t.Fatalf("the export pass ran with the feature off: %v", out)
		}
	}
	withFeatures(t, func(f *common.FeatureSettings) { f.MapView = true })

	got, r := mapPlacesBy(t, "")
	if len(got) != 6 {
		t.Fatalf("places %v", r)
	}
	c := got["Coffee at the corner"]
	if c["Has"] != true || c["How"] != "coordinates" || c["Keyword"] != "TODO" || c["Colour"] != "#e8743b" {
		t.Errorf("coffee %v", c)
	}
	if s, _ := c["Snippet"].(string); s != "Good beans, and a website too." {
		t.Errorf("snippet %q", s)
	}
	if m := got["Saw the clock"]; m["Has"] != true || m["Done"] != true || m["Lat"].(float64) != 51.5007 {
		t.Errorf("osm url %v", m)
	}
	if m := got["Old pin"]; m["How"] != "latlon" {
		t.Errorf("lat/lon %v", m)
	}
	if m := got["Market"]; m["Has"] == true || m["Address"] != "Granville Island, Vancouver" {
		t.Errorf("an address not looked up yet %v", m)
	}
	if r["Unplaced"].(float64) != 3 || atomic.LoadInt32(asked) != 0 {
		t.Errorf("unplaced %v, asked %d (nothing should be looked up unasked)", r["Unplaced"], *asked)
	}

	// Queue them all; the worker places two, the third is not found.
	if r := mediaCall(t, PostMapGeocode, "POST", "/map/geocode", map[string]interface{}{"All": true}); r["Ok"] != true || r["Pending"].(float64) != 3 {
		t.Fatalf("all %v", r)
	}
	deadline := time.Now().Add(5 * time.Second)
	for mapGeo.pending() > 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	got, r = mapPlacesBy(t, "")
	if m := got["Market"]; m["Has"] != true || m["How"] != "geocoded" || m["Lat"].(float64) != 49.2713 {
		t.Errorf("market after lookup %v", m)
	}
	if m := got["Ada Lovelace/home"]; m["Has"] != true || m["Key"] != "ADDRESS_HOME" {
		t.Errorf("record home %v", m)
	}
	if m := got["Ada Lovelace/work"]; m["Has"] == true || m["NotFound"] != true {
		t.Errorf("record work %v", m)
	}
	n := atomic.LoadInt32(asked)
	// Asked once each, and the cache is on disk beside the config.
	if n != 3 {
		t.Errorf("asked %d times, want 3", n)
	}
	if b, err := os.ReadFile(mapGeocodeCachePath()); err != nil || !strings.Contains(string(b), "granville island, vancouver") {
		t.Errorf("cache file %v %s", err, b)
	}
	mapGeo = &mapGeocoder{} // read back from the file
	mapPlacesBy(t, "")
	if atomic.LoadInt32(asked) != n {
		t.Errorf("asked again after a restart")
	}

	// A query.
	got, _ = mapPlacesBy(t, "?query="+urlq(`IsTodo()`))
	if len(got) != 1 || got["Coffee at the corner"] == nil {
		t.Errorf("query: %v", got)
	}

	// Set: a click on the map, into a column-zero drawer; then an address
	// written with its coordinates; then off again.
	_, sec := secAt(t, p, "Market")
	r = mediaCall(t, PostMapSet, "POST", "/map/set", map[string]interface{}{"Hash": sec.Hash, "Location": "Granville Island, Vancouver", "Geo": true})
	if r["Ok"] != true {
		t.Fatalf("set %v", r)
	}
	b, _ := os.ReadFile(p)
	if !strings.Contains(string(b), "  :LOCATION: Granville Island, Vancouver\n  :GEO:      49.2713,-123.134\n  :END:") {
		t.Errorf("geo written at the drawer's indent:\n%s", b)
	}
	got, _ = mapPlacesBy(t, "")
	if m := got["Market"]; m["How"] != "geo" {
		t.Errorf("written geo not read %v", m)
	}
	_, sec = secAt(t, p, "No place here")
	r = mediaCall(t, PostMapSet, "POST", "/map/set", map[string]interface{}{"Hash": sec.Hash, "Location": "49.25,-123.1"})
	if r["Ok"] != true {
		t.Fatalf("set coords %v", r)
	}
	b, _ = os.ReadFile(p)
	if !strings.Contains(string(b), "* No place here\n:PROPERTIES:\n:LOCATION: 49.25,-123.1\n:END:\nWords.") {
		t.Errorf("a new drawer in a column-zero file:\n%s", b)
	}
	_, sec = secAt(t, p, "Market")
	mediaCall(t, PostMapSet, "POST", "/map/set", map[string]interface{}{"Hash": sec.Hash, "Location": ""})
	b, _ = os.ReadFile(p)
	if strings.Contains(string(b), "Granville") || strings.Contains(string(b), ":GEO:") {
		t.Errorf("location not taken off:\n%s", b)
	}

	// Searching an address answers several; coordinates answer themselves.
	r = mediaCall(t, PostMapGeocode, "POST", "/map/geocode", map[string]interface{}{"Address": "granville island"})
	if res, _ := r["Results"].([]interface{}); len(res) != 1 {
		t.Errorf("search %v", r)
	}
	before := atomic.LoadInt32(asked)
	r = mediaCall(t, PostMapGeocode, "POST", "/map/geocode", map[string]interface{}{"Address": "49.1, -123.2"})
	if res, _ := r["Results"].([]interface{}); len(res) != 1 || atomic.LoadInt32(asked) != before {
		t.Errorf("coordinates went to the geocoder %v", r)
	}
}

// exportLinesForMapOff is the map pass's guard as exportLinesFor uses it.
func exportLinesForMapOff(p string, lines []string) ([]string, bool) {
	text := strings.Join(lines, "\n") + "\n#+MAP:"
	if !mapMayApply(text) {
		return nil, false
	}
	return mapExportLines(p, lines, "html")
}

func urlq(s string) string { return url.QueryEscape(s) }

func TestMapExport(t *testing.T) {
	text := `* Trip
#+MAP: title:"Where we went" height:300
Some text.
** Here
:PROPERTIES:
:LOCATION: geo:49.2827,-123.1207
:END:
** There :travel:
:PROPERTIES:
:LOCATION: 49°17'N 123°7'W
:END:
#+begin_example
#+MAP: not this one
#+end_example
* Elsewhere
:PROPERTIES:
:LOCATION: 51.5, -0.12
:END:
`
	mapFakeGeocoder(t)
	withFeatures(t, func(f *common.FeatureSettings) { f.MapView = true })
	paths := loadOrg(t, map[string]string{"t.org": text})
	p := paths["t.org"]
	lines, _ := readFileLines(p)
	out, changed := mapExportLines(p, lines, "html")
	all := strings.Join(out, "\n")
	if !changed || !strings.Contains(all, `<figure class="org-map"`) || strings.Count(all, "<path d=") != 2 {
		t.Fatalf("html:\n%s", all)
	}
	if !strings.Contains(all, `data-map-from="0" data-map-to="13"`) {
		t.Errorf("subtree rows not carried:\n%s", all)
	}
	// An earlier pass that turns one line into three must not move the subtree.
	shifted := append([]string{"#+BEGIN_EXPORT html", "<p>x</p>", "#+END_EXPORT"}, lines...)
	if o2, _ := mapExportLines(p, shifted, "html"); strings.Count(strings.Join(o2, "\n"), "<path d=") != 2 {
		t.Errorf("shifted copy drew the wrong places")
	}
	if !strings.Contains(all, `data-map-options="title:&#34;Where we went&#34; height:300"`) || !strings.Contains(all, `viewBox="0 0 720 300"`) {
		t.Errorf("options not carried:\n%s", all)
	}
	if !strings.Contains(all, "#+MAP: not this one") || !strings.Contains(all, "°N") {
		t.Errorf("block or grid wrong:\n%s", all)
	}
	out, _ = mapExportLines(p, lines, "txt")
	all = strings.Join(out, "\n")
	if !strings.Contains(all, "1. [[https://www.openstreetmap.org/?mlat=49.2827&mlon=-123.1207#map=16/49.2827/-123.1207][Here]]: 49.2827,-123.1207") {
		t.Errorf("text:\n%s", all)
	}
	if _, err := mapSpecFrom("zoom:40", p); err == nil {
		t.Errorf("zoom 40 read")
	}
	// The static picture of one point.
	rec := httptest.NewRecorder()
	RequestMapStatic(rec, httptest.NewRequest("GET", "/map/static?ll=49.28,-123.12&title=X", nil))
	if rec.Header().Get("Content-Type") != "image/svg+xml" || !strings.Contains(rec.Body.String(), "<title>X — 49.28,-123.12</title>") {
		t.Errorf("static %s", rec.Body.String())
	}
}
