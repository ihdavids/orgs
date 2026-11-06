package geo

import (
	"bytes"
	"encoding/json"
)

// Place is one heading's place on the wire (GET /map/places): where it is,
// what it is, and how the server came to know where it is.
type Place struct {
	Hash     string
	Title    string
	Keyword  string   `json:",omitempty"`
	Done     bool     `json:",omitempty"`
	Tags     []string `json:",omitempty"`
	Filename string
	Line     int // 0 based, the heading's line
	// Key is the property the place was read from: LOCATION, LATITUDE,
	// ADDRESS_HOME... Label is the record's name for it (home, work), if any.
	Key   string
	Label string `json:",omitempty"`
	// Value is the property as written.
	Value string
	// Has is whether Lat/Lon are known. How says how: coordinates (written
	// in the property), latlon (LATITUDE/LONGITUDE), geo (coordinates written
	// back beside an address), geocoded (an address, looked up and cached)
	// or "" with Has false: an address not looked up yet, or not found.
	Has      bool
	Lat      float64 `json:",omitempty"`
	Lon      float64 `json:",omitempty"`
	How      string  `json:",omitempty"`
	Address  string  `json:",omitempty"` // the words looked up, when the value is an address
	Found    string  `json:",omitempty"` // the geocoder's name for what it found
	NotFound bool    `json:",omitempty"` // looked up, and the geocoder knew no such place
	Snippet  string  `json:",omitempty"`
	Colour   string  `json:",omitempty"`
}

// PlacesAnswer is GET /map/places.
type PlacesAnswer struct {
	Ok       bool
	Msg      string `json:",omitempty"`
	Off      bool   `json:",omitempty"`
	Places   []Place
	Unplaced int // places with no coordinates yet (in Places, Has false)
	Pending  int // addresses waiting their turn with the geocoder
}

// Feature is a GeoJSON feature (RFC 7946).
type Feature struct {
	Type       string                 `json:"type"`
	Geometry   map[string]interface{} `json:"geometry"`
	Properties map[string]interface{} `json:"properties"`
}

// FeatureCollection is a GeoJSON document.
type FeatureCollection struct {
	Type     string    `json:"type"`
	Features []Feature `json:"features"`
}

// GeoJSON is the places that have coordinates as a FeatureCollection, each
// point with the heading's title, keyword, tags, file, line and an
// openstreetmap.org link in its properties. GeoJSON is lon, lat.
func GeoJSON(places []Place) FeatureCollection {
	out := FeatureCollection{Type: "FeatureCollection", Features: []Feature{}}
	for _, p := range places {
		if !p.Has {
			continue
		}
		props := map[string]interface{}{
			"title": p.Title, "file": p.Filename, "line": p.Line + 1,
			"hash": p.Hash, "source": p.Key, "location": p.Value,
			"osm": OSMUrl(LatLon{p.Lat, p.Lon}, 16),
		}
		if p.Keyword != "" {
			props["keyword"] = p.Keyword
		}
		if len(p.Tags) > 0 {
			props["tags"] = p.Tags
		}
		if p.Label != "" {
			props["label"] = p.Label
		}
		if p.Colour != "" {
			props["marker-color"] = p.Colour // simplestyle-spec, read by github and geojson.io
		}
		out.Features = append(out.Features, Feature{
			Type:       "Feature",
			Geometry:   map[string]interface{}{"type": "Point", "coordinates": []float64{p.Lon, p.Lat}},
			Properties: props,
		})
	}
	return out
}

// MarshalGeoJSON is GeoJSON, indented.
func MarshalGeoJSON(places []Place) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false) // urls keep their &
	enc.SetIndent("", "  ")
	if err := enc.Encode(GeoJSON(places)); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(b.Bytes(), []byte("\n")), nil
}
