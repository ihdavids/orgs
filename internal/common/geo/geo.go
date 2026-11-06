// Package geo is the pure half of the map view (features.mapView): reading a
// place out of what a person writes in :LOCATION: (coordinates in their usual
// spellings, a geo: uri, a Google / OpenStreetMap / Apple maps url), writing
// coordinates back, the web mercator arithmetic a map needs, the colour a pin
// is drawn in, and GeoJSON.
//
// Shared by the server (mapview.go) and the CLI (orgs map), so a place reads
// the same everywhere. worg/src/mapview.ts is the TypeScript twin of Parse,
// Format, PinColour and HashIndex: the same cases are in geo_test.go and
// mapview.test.ts (FitView is Go only; worg uses Leaflet's fitBounds).
package geo

import (
	"fmt"
	"math"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// LatLon is a point in degrees, WGS 84.
type LatLon struct {
	Lat float64
	Lon float64
}

// Valid is whether the point is on the globe (and not the 0,0 of a value
// that was never set).
func (p LatLon) Valid() bool {
	return !math.IsNaN(p.Lat) && !math.IsNaN(p.Lon) &&
		p.Lat >= -90 && p.Lat <= 90 && p.Lon >= -180 && p.Lon <= 180 &&
		!(p.Lat == 0 && p.Lon == 0)
}

var (
	orgLinkRe = regexp.MustCompile(`^\[\[([^\]]+)\](?:\[[^\]]*\])?\]$`)
	num       = `([+-]?\d{1,3}(?:\.\d+)?)`
	plainRe   = regexp.MustCompile(`^\(?\s*` + num + `\s*(?:,|;|\s)\s*` + num + `\s*\)?$`)
	geoUriRe  = regexp.MustCompile(`(?i)^geo:\s*` + num + `\s*,\s*` + num + `(?:\s*,\s*[+-]?[\d.]+)?(?:;.*)?$`)
	// Google's /@lat,lon,15z and Apple's / OSM's mlat=&mlon=, #map=z/lat/lon.
	atRe    = regexp.MustCompile(`/@` + num + `,` + num)
	hashRe  = regexp.MustCompile(`#map=\d+(?:\.\d+)?/` + num + `/` + num)
	googleD = regexp.MustCompile(`!3d` + num + `!4d` + num)
	// 49°17'0"N 123°7'0"W, 49.28 N 123.12 W, 49.28N, 123.12W
	hemiRe = regexp.MustCompile(`(?i)(\d{1,3}(?:\.\d+)?)\s*°?\s*(?:(\d{1,2}(?:\.\d+)?)\s*['′’]\s*)?(?:(\d{1,2}(?:\.\d+)?)\s*(?:["″”]|''|′′)\s*)?([NSEW])\b`)
)

// Parse reads coordinates from a :LOCATION: value: "49.28, -123.12",
// "49.28 -123.12", "geo:49.28,-123.12", 49°17'N 123°7'W, or a maps url
// (Google, OpenStreetMap, Apple, Bing - any with lat,lon in q=, ll=, query=,
// center=, destination=, cp=, or mlat=/mlon=), the url bare or as an org link.
// Anything else - an address, a place name - is not coordinates.
func Parse(s string) (LatLon, bool) {
	s = strings.TrimSpace(s)
	if m := orgLinkRe.FindStringSubmatch(s); m != nil {
		s = strings.TrimSpace(m[1])
	}
	if s == "" {
		return LatLon{}, false
	}
	pair := func(a, b string) (LatLon, bool) {
		lat, e1 := strconv.ParseFloat(a, 64)
		lon, e2 := strconv.ParseFloat(b, 64)
		p := LatLon{lat, lon}
		if e1 != nil || e2 != nil || !p.Valid() {
			return LatLon{}, false
		}
		return p, true
	}
	if m := plainRe.FindStringSubmatch(s); m != nil {
		return pair(m[1], m[2])
	}
	if m := geoUriRe.FindStringSubmatch(s); m != nil {
		return pair(m[1], m[2])
	}
	low := strings.ToLower(s)
	if strings.HasPrefix(low, "http://") || strings.HasPrefix(low, "https://") {
		return parseUrl(s)
	}
	if ms := hemiRe.FindAllStringSubmatch(s, -1); len(ms) == 2 {
		// Only when that is all there is: "Flat 4N, 12 Main St" is an address.
		rest := hemiRe.ReplaceAllString(s, "")
		if strings.Trim(rest, " ,;") != "" {
			return LatLon{}, false
		}
		lat, lon := math.NaN(), math.NaN()
		for _, m := range ms {
			v, _ := strconv.ParseFloat(m[1], 64)
			if m[2] != "" {
				x, _ := strconv.ParseFloat(m[2], 64)
				v += x / 60
			}
			if m[3] != "" {
				x, _ := strconv.ParseFloat(m[3], 64)
				v += x / 3600
			}
			switch strings.ToUpper(m[4]) {
			case "N":
				lat = v
			case "S":
				lat = -v
			case "E":
				lon = v
			case "W":
				lon = -v
			}
		}
		p := LatLon{lat, lon}
		if p.Valid() {
			return p, true
		}
	}
	return LatLon{}, false
}

func parseUrl(s string) (LatLon, bool) {
	try := func(v string) (LatLon, bool) {
		if m := plainRe.FindStringSubmatch(strings.TrimSpace(v)); m != nil {
			lat, _ := strconv.ParseFloat(m[1], 64)
			lon, _ := strconv.ParseFloat(m[2], 64)
			p := LatLon{lat, lon}
			if p.Valid() {
				return p, true
			}
		}
		return LatLon{}, false
	}
	u, err := url.Parse(s)
	if err != nil {
		return LatLon{}, false
	}
	q := u.Query()
	if a, b := q.Get("mlat"), q.Get("mlon"); a != "" && b != "" {
		if p, ok := try(a + "," + b); ok {
			return p, true
		}
	}
	// A pin Google dropped (!3d..!4d..) is the place; /@ is where the view was.
	if m := googleD.FindStringSubmatch(s); m != nil {
		if p, ok := try(m[1] + "," + m[2]); ok {
			return p, true
		}
	}
	for _, k := range []string{"q", "query", "ll", "sll", "center", "destination", "daddr", "cp", "coord"} {
		v := strings.ReplaceAll(q.Get(k), "~", ",") // Bing's cp=lat~lon
		if p, ok := try(v); ok {
			return p, true
		}
	}
	if m := atRe.FindStringSubmatch(s); m != nil {
		if p, ok := try(m[1] + "," + m[2]); ok {
			return p, true
		}
	}
	if m := hashRe.FindStringSubmatch(s); m != nil {
		if p, ok := try(m[1] + "," + m[2]); ok {
			return p, true
		}
	}
	return LatLon{}, false
}

// IsMapUrl is whether s is a link (bare or an org link) rather than words:
// a url that names no coordinates is not an address to look up either.
func IsMapUrl(s string) bool {
	s = strings.TrimSpace(s)
	if m := orgLinkRe.FindStringSubmatch(s); m != nil {
		s = m[1]
	}
	l := strings.ToLower(s)
	return strings.HasPrefix(l, "http://") || strings.HasPrefix(l, "https://") || strings.HasPrefix(l, "geo:")
}

// Format writes a point the way the map view writes one into a property:
// "49.2827,-123.1207", up to six decimals (about 10cm), no trailing zeros.
func Format(p LatLon) string {
	return trimNum(p.Lat) + "," + trimNum(p.Lon)
}

func trimNum(v float64) string {
	s := strconv.FormatFloat(math.Round(v*1e6)/1e6, 'f', 6, 64)
	s = strings.TrimRight(s, "0")
	s = strings.TrimSuffix(s, ".")
	if s == "-0" {
		s = "0"
	}
	return s
}

// OSMUrl is the point on openstreetmap.org, with a marker.
func OSMUrl(p LatLon, zoom int) string {
	if zoom <= 0 {
		zoom = 15
	}
	return fmt.Sprintf("https://www.openstreetmap.org/?mlat=%s&mlon=%s#map=%d/%s/%s",
		trimNum(p.Lat), trimNum(p.Lon), zoom, trimNum(p.Lat), trimNum(p.Lon))
}

// SearchUrl is openstreetmap.org searching for an address.
func SearchUrl(address string) string {
	return "https://www.openstreetmap.org/search?query=" + url.QueryEscape(address)
}

// ---------------------------------------------------------------------------
// Web mercator, 256px tiles
// ---------------------------------------------------------------------------

const TileSize = 256

// maxLat is where web mercator stops.
const maxLat = 85.05112878

// WorldPx is the point in pixels of the whole world at zoom (0,0 top left).
func WorldPx(p LatLon, zoom float64) (x, y float64) {
	lat := math.Max(-maxLat, math.Min(maxLat, p.Lat))
	scale := TileSize * math.Pow(2, zoom)
	x = (p.Lon + 180) / 360 * scale
	s := math.Sin(lat * math.Pi / 180)
	y = (0.5 - math.Log((1+s)/(1-s))/(4*math.Pi)) * scale
	return
}

// FromWorldPx is WorldPx the other way.
func FromWorldPx(x, y, zoom float64) LatLon {
	scale := TileSize * math.Pow(2, zoom)
	lon := x/scale*360 - 180
	n := math.Pi - 2*math.Pi*y/scale
	lat := 180 / math.Pi * math.Atan(math.Sinh(n))
	return LatLon{lat, lon}
}

// FitView is the centre and the whole zoom (0..maxZoom) that shows every
// point inside w x h pixels with pad pixels to spare on each side. One point
// is shown at maxZoom; none at zoom 1 over the middle of the world.
func FitView(pts []LatLon, w, h, pad, maxZoom int) (LatLon, int) {
	if maxZoom <= 0 {
		maxZoom = 16
	}
	if len(pts) == 0 {
		return LatLon{20, 0}, 1
	}
	minLat, maxLatV, minLon, maxLon := 90.0, -90.0, 180.0, -180.0
	for _, p := range pts {
		minLat = math.Min(minLat, p.Lat)
		maxLatV = math.Max(maxLatV, p.Lat)
		minLon = math.Min(minLon, p.Lon)
		maxLon = math.Max(maxLon, p.Lon)
	}
	// The centre is the middle of the box in projected space, not in degrees.
	x0, y0 := WorldPx(LatLon{maxLatV, minLon}, 0)
	x1, y1 := WorldPx(LatLon{minLat, maxLon}, 0)
	c := FromWorldPx((x0+x1)/2, (y0+y1)/2, 0)
	if len(pts) == 1 || (x1-x0 < 1e-9 && y1-y0 < 1e-9) {
		return LatLon{pts[0].Lat, pts[0].Lon}, min(maxZoom, 15)
	}
	aw, ah := float64(w-2*pad), float64(h-2*pad)
	if aw < 16 {
		aw = 16
	}
	if ah < 16 {
		ah = 16
	}
	zoom := 0
	for z := maxZoom; z >= 0; z-- {
		s := math.Pow(2, float64(z))
		if (x1-x0)*s <= aw && (y1-y0)*s <= ah {
			zoom = z
			break
		}
	}
	return c, zoom
}

// ---------------------------------------------------------------------------
// Pin colours
// ---------------------------------------------------------------------------

// Palette is the pins' categorical colours (worg's chart palette slots).
var Palette = []string{"#2a78d6", "#e8743b", "#19a979", "#945ecf", "#d64f8a", "#13a4b4", "#bf8b12", "#6c8893"}

// The fixed colours by keyword: no keyword, a keyword still to do, done.
const (
	ColourPlain  = "#2a78d6"
	ColourActive = "#e8743b"
	ColourDone   = "#7d8b96"
)

// HashIndex is a word's slot in a palette of n: sum of its UTF-16 units times
// 31, wrapped to 32 bits, as the TypeScript twin does it.
func HashIndex(s string, n int) int {
	if n <= 0 {
		return 0
	}
	var h int32
	for _, u := range utf16Units(s) {
		h = h*31 + int32(u)
	}
	if h < 0 {
		h = -h
		if h < 0 {
			h = 0
		}
	}
	return int(h) % n
}

func utf16Units(s string) []uint16 {
	out := []uint16{}
	for _, r := range s {
		if r >= 0x10000 {
			r -= 0x10000
			out = append(out, uint16(0xD800+(r>>10)), uint16(0xDC00+(r&0x3FF)))
		} else {
			out = append(out, uint16(r))
		}
	}
	return out
}

// PinColour is the colour a pin is drawn in. by is "keyword" (the default:
// none, to do, done) or "tag" (the first tag's palette slot, the plain colour
// with none).
func PinColour(by, keyword string, done bool, tags []string) string {
	if by == "tag" {
		if len(tags) == 0 {
			return ColourPlain
		}
		return Palette[HashIndex(strings.ToLower(tags[0]), len(Palette))]
	}
	switch {
	case keyword == "":
		return ColourPlain
	case done:
		return ColourDone
	}
	return ColourActive
}
