package orgs

// Looking addresses up for the map view (features.mapView): Nominatim by
// default (mapView.geocodeUrl names any server that answers its /search),
// one request a second at most (its usage policy), a User-Agent naming orgs,
// and every answer - found or not - kept in <config>_geocode.json beside the
// yaml so an address is asked for once. A failure to reach the server is not
// kept: the next try asks again.
//
// Nothing here runs unless asked: an explicit lookup (POST /map/geocode,
// `orgs map geocode`), or mapView.autoGeocode, which queues the addresses a
// /map/places answer could not place. The queue is worked by one goroutine
// at the same one-a-second pace; it publishes a `map` event when it has
// placed something, so a map showing the places draws them.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ihdavids/orgs/internal/common"
	"github.com/ihdavids/orgs/internal/common/geo"
)

const (
	mapFeature           = "mapView"
	mapDefaultGeocodeUrl = "https://nominatim.openstreetmap.org/search"
	mapDefaultTileUrl    = "https://tile.openstreetmap.org/{z}/{x}/{y}.png"
	mapDefaultAttrib     = "© OpenStreetMap contributors"
	// EventMap: the geocoder placed something (mapgeocode.go).
	EventMap = "map"
)

func mapConf() common.MapViewSettings {
	if s := Conf().Server; s != nil {
		return s.MapView
	}
	return common.MapViewSettings{}
}

func mapGeocodeUrl() string {
	if u := strings.TrimSpace(mapConf().GeocodeUrl); u != "" {
		return u
	}
	return mapDefaultGeocodeUrl
}

func mapTileUrl() string {
	if u := strings.TrimSpace(mapConf().TileUrl); u != "" {
		return u
	}
	return mapDefaultTileUrl
}

func mapAttribution() string {
	if a := strings.TrimSpace(mapConf().Attribution); a != "" {
		return a
	}
	if mapConf().TileUrl == "" {
		return mapDefaultAttrib
	}
	return ""
}

func mapGeoProp() string {
	if p := strings.ToUpper(strings.TrimSpace(mapConf().GeoProperty)); p != "" {
		return p
	}
	return "GEO"
}

func mapStaticTiles() bool {
	if b := mapConf().StaticTiles; b != nil {
		return *b
	}
	return true
}

func mapUserAgent() string {
	return "orgs/" + common.Version + " (org mode server; https://github.com/ihdavids/orgs)"
}

// MapGeoHit is one place a geocoder found.
type MapGeoHit struct {
	Lat, Lon float64
	Name     string
	Kind     string `json:",omitempty"` // Nominatim's class/type: amenity/cafe
}

// mapGeoEntry is one address in the cache. Missing: the geocoder answered and
// knew no such place.
type mapGeoEntry struct {
	Lat     float64 `json:"lat,omitempty"`
	Lon     float64 `json:"lon,omitempty"`
	Name    string  `json:"name,omitempty"`
	Missing bool    `json:"missing,omitempty"`
	When    string  `json:"when"`
}

type mapGeocoder struct {
	mu      sync.Mutex
	loaded  bool
	path    string
	entries map[string]mapGeoEntry
	dirty   bool

	// One request at a time, a second apart.
	reqMu sync.Mutex
	last  time.Time

	queue   []string
	queued  map[string]bool
	working bool
	lastErr string
	lastAt  string
	asked   int
}

var mapGeo = &mapGeocoder{}

// mapGeoGap is the least time between two lookups (Nominatim: one a second).
var mapGeoGap = time.Second

// mapHTTP is how the geocoder and the tile fetcher ask. A test swaps it.
var mapHTTP = &http.Client{Timeout: 12 * time.Second}

var mapSpacesRe = regexp.MustCompile(`\s+`)

// mapGeoKey is how an address is filed: case and spacing do not make two.
func mapGeoKey(address string) string {
	return strings.ToLower(mapSpacesRe.ReplaceAllString(strings.TrimSpace(address), " "))
}

func mapGeocodeCachePath() string {
	if cfg := Conf().Config; cfg != "" {
		ext := filepath.Ext(cfg)
		return strings.TrimSuffix(cfg, ext) + "_geocode.json"
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "orgs", "geocode.json")
}

func (g *mapGeocoder) load() {
	path := mapGeocodeCachePath()
	if g.loaded && g.path == path {
		return
	}
	g.loaded, g.path, g.entries = true, path, map[string]mapGeoEntry{}
	if b, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(b, &g.entries); err != nil {
			fmt.Fprintf(os.Stderr, "map: %s is not readable (%v); starting a new one\n", path, err)
			g.entries = map[string]mapGeoEntry{}
		}
	}
}

// save writes the cache (temp + rename), sorted keys by json's own map order.
func (g *mapGeocoder) save() {
	if !g.dirty {
		return
	}
	b, err := json.MarshalIndent(g.entries, "", " ")
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(g.path), 0755)
	tmp := g.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0600); err != nil {
		fmt.Fprintf(os.Stderr, "map: could not write %s: %v\n", g.path, err)
		return
	}
	if err := os.Rename(tmp, g.path); err != nil {
		fmt.Fprintf(os.Stderr, "map: could not write %s: %v\n", g.path, err)
		return
	}
	g.dirty = false
}

// cached is what the cache knows about an address.
func (g *mapGeocoder) cached(address string) (mapGeoEntry, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.load()
	e, ok := g.entries[mapGeoKey(address)]
	return e, ok
}

func (g *mapGeocoder) put(address string, e mapGeoEntry) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.load()
	e.When = time.Now().Format(time.RFC3339)
	g.entries[mapGeoKey(address)] = e
	g.dirty = true
	g.save()
}

func (g *mapGeocoder) counts() (found, missing int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.load()
	for _, e := range g.entries {
		if e.Missing {
			missing++
		} else {
			found++
		}
	}
	return
}

// search asks the geocoder, waiting its turn. Up to limit hits, best first.
func (g *mapGeocoder) search(address string, limit int) ([]MapGeoHit, error) {
	address = strings.TrimSpace(address)
	if address == "" {
		return nil, fmt.Errorf("no address to look up")
	}
	if limit <= 0 || limit > 10 {
		limit = 1
	}
	base, err := url.Parse(mapGeocodeUrl())
	if err != nil || (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" {
		return nil, fmt.Errorf("mapView.geocodeUrl %q is not an http(s) url", mapGeocodeUrl())
	}
	q := base.Query()
	q.Set("q", address)
	q.Set("format", "jsonv2")
	q.Set("limit", strconv.Itoa(limit))
	if e := strings.TrimSpace(mapConf().Email); e != "" {
		q.Set("email", e)
	}
	base.RawQuery = q.Encode()

	g.reqMu.Lock()
	defer g.reqMu.Unlock()
	if wait := mapGeoGap - time.Since(g.last); wait > 0 {
		time.Sleep(wait)
	}
	defer func() { g.last = time.Now() }()

	req, err := http.NewRequest(http.MethodGet, base.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", mapUserAgent())
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Language", "en;q=0.9,*;q=0.5")
	g.mu.Lock()
	g.asked++
	g.lastAt = time.Now().Format(time.RFC3339)
	g.mu.Unlock()
	resp, err := mapHTTP.Do(req)
	if err != nil {
		g.noteErr(err.Error())
		return nil, fmt.Errorf("could not reach the geocoder (%s): %v", base.Host, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		g.noteErr(resp.Status)
		return nil, fmt.Errorf("the geocoder (%s) answered %s", base.Host, resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		g.noteErr(err.Error())
		return nil, err
	}
	var raw []struct {
		Lat, Lon    string
		DisplayName string `json:"display_name"`
		Name        string `json:"name"`
		Category    string `json:"category"`
		Class       string `json:"class"`
		Type        string `json:"type"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		g.noteErr("not a Nominatim answer")
		return nil, fmt.Errorf("the geocoder's answer is not a list of places: %v", err)
	}
	g.noteErr("")
	out := []MapGeoHit{}
	for _, r := range raw {
		lat, e1 := strconv.ParseFloat(r.Lat, 64)
		lon, e2 := strconv.ParseFloat(r.Lon, 64)
		if e1 != nil || e2 != nil || !(geo.LatLon{Lat: lat, Lon: lon}).Valid() {
			continue
		}
		kind := r.Category
		if kind == "" {
			kind = r.Class
		}
		if r.Type != "" {
			kind += "/" + r.Type
		}
		name := r.DisplayName
		if name == "" {
			name = r.Name
		}
		out = append(out, MapGeoHit{Lat: lat, Lon: lon, Name: name, Kind: strings.Trim(kind, "/")})
	}
	return out, nil
}

func (g *mapGeocoder) noteErr(s string) {
	g.mu.Lock()
	g.lastErr = s
	g.mu.Unlock()
}

// lookup is one address's place: from the cache unless force, else asked
// and kept (found or missing).
func (g *mapGeocoder) lookup(address string, force bool) (mapGeoEntry, error) {
	if !force {
		if e, ok := g.cached(address); ok {
			return e, nil
		}
	}
	hits, err := g.search(address, 1)
	if err != nil {
		return mapGeoEntry{}, err
	}
	e := mapGeoEntry{Missing: true}
	if len(hits) > 0 {
		e = mapGeoEntry{Lat: hits[0].Lat, Lon: hits[0].Lon, Name: hits[0].Name}
	}
	g.put(address, e)
	return e, nil
}

// enqueue adds addresses the cache does not know to the queue and starts
// the worker. Answers how many are waiting.
func (g *mapGeocoder) enqueue(addresses []string) int {
	g.mu.Lock()
	g.load()
	if g.queued == nil {
		g.queued = map[string]bool{}
	}
	for _, a := range addresses {
		k := mapGeoKey(a)
		if k == "" || g.queued[k] {
			continue
		}
		if _, ok := g.entries[k]; ok {
			continue
		}
		g.queued[k] = true
		g.queue = append(g.queue, a)
	}
	n := len(g.queue)
	start := n > 0 && !g.working
	if start {
		g.working = true
	}
	g.mu.Unlock()
	if start {
		go g.work()
	}
	return n
}

func (g *mapGeocoder) pending() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.queue)
}

// work looks the queue up one a second (search waits its turn). It stops
// when the queue is empty, the feature is turned off, or the geocoder
// cannot be reached (the rest stay unplaced, and are asked again next time).
func (g *mapGeocoder) work() {
	placed := 0
	defer func() {
		g.mu.Lock()
		g.working = false
		g.mu.Unlock()
		if placed > 0 {
			Publish(Event{Kind: EventMap})
		}
	}()
	for {
		g.mu.Lock()
		if len(g.queue) == 0 || !featureOn(mapFeature) {
			g.queue, g.queued = nil, map[string]bool{}
			g.mu.Unlock()
			return
		}
		a := g.queue[0]
		g.mu.Unlock()
		e, err := g.lookup(a, false)
		g.mu.Lock()
		g.queue = g.queue[1:]
		delete(g.queued, mapGeoKey(a))
		g.mu.Unlock()
		if err != nil {
			fmt.Fprintf(os.Stderr, "map: %v\n", err)
			g.mu.Lock()
			g.queue, g.queued = nil, map[string]bool{}
			g.mu.Unlock()
			return
		}
		if !e.Missing {
			placed++
			// Every few, so a long queue fills the map as it goes.
			if placed%5 == 0 {
				Publish(Event{Kind: EventMap})
			}
		}
	}
}
