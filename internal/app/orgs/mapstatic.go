package orgs

// The map view's pictures (features.mapView): GET /map/static and the
// #+MAP: line in exports.
//
// A picture is an SVG: the map's tiles fetched and composed here into one
// JPEG (mapView.tileUrl, OpenStreetMap's by default), pins drawn over it as
// vector marks with a number each, each pin a link to openstreetmap.org, and
// the attribution in the corner. Tiles are kept in the user cache folder for
// a month and asked for with a User-Agent naming orgs, two at a time, at
// most mapMaxTiles per picture - an export is a person looking at a page,
// which is what the OSM tile policy allows. With staticTiles off, or the
// tile server unreachable, the pins are drawn over a plain grid of degrees
// instead, so an export never fails for want of a network.
//
//   #+MAP: query:"HasTags(\"travel\")" zoom:5 center:"48.85,2.35" height:400 title:"Trips"
//
// Words as #+TRACKER:'s (common.ParseTrackerOptions). Without query: the
// places of the heading the line is under, and its subtree (above the first
// heading: the whole file). html exports get the picture and a
// numbered list of the places; every other backend the list.

import (
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"html"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	_ "image/png"
	"io"
	"math"
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
	mapMaxTiles  = 30
	mapTileTTL   = 30 * 24 * time.Hour
	mapMaxPinNum = 40 // more than this and the pins carry no numbers
)

var mapLineRe = regexp.MustCompile(`(?i)^(\s*)#\+MAP:\s*(.*)$`)

// mapViewSpec is how one picture is laid out.
type mapViewSpec struct {
	W, H     int
	Center   geo.LatLon
	Zoom     int
	HasView  bool // center and zoom were given
	Tiles    bool
	Title    string
	Query    string
	File     string
	Colour   string
	Options  string
	From, To int // the rows of the subtree a #+MAP: line under a heading shows (To 0: the whole file)
}

// mapSpecFrom reads a #+MAP: line's words.
func mapSpecFrom(options, file string) (mapViewSpec, error) {
	opts := common.ParseTrackerOptions(options)
	s := mapViewSpec{W: 720, H: 360, Tiles: mapStaticTiles(), Options: options, File: file}
	s.Query = common.TrackerGet(opts, "query", "")
	s.Title = common.TrackerGet(opts, "title", "")
	s.Colour = common.TrackerGet(opts, "colour", common.TrackerGet(opts, "color", "keyword"))
	if v := common.TrackerGet(opts, "height", ""); v != "" {
		n, err := strconv.Atoi(strings.TrimSuffix(v, "px"))
		if err != nil || n < 80 || n > 1600 {
			return s, fmt.Errorf("height:%s is not a height in pixels (80-1600)", v)
		}
		s.H = n
	}
	if v := common.TrackerGet(opts, "width", ""); v != "" {
		n, err := strconv.Atoi(strings.TrimSuffix(v, "px"))
		if err != nil || n < 120 || n > 2000 {
			return s, fmt.Errorf("width:%s is not a width in pixels (120-2000)", v)
		}
		s.W = n
	}
	zoom := -1
	if v := common.TrackerGet(opts, "zoom", ""); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 || n > 19 {
			return s, fmt.Errorf("zoom:%s is not a zoom (0-19)", v)
		}
		zoom = n
	}
	if v := common.TrackerGet(opts, "center", common.TrackerGet(opts, "centre", "")); v != "" {
		ll, ok := geo.Parse(v)
		if !ok {
			return s, fmt.Errorf("center:%s is not coordinates", v)
		}
		s.Center, s.HasView = ll, true
		if zoom < 0 {
			zoom = 12
		}
	}
	if zoom >= 0 {
		s.Zoom = zoom
		if !s.HasView {
			s.Zoom = -zoom - 1 // a zoom without a centre: fit the centre, keep the zoom
		}
	}
	if t := strings.ToLower(common.TrackerGet(opts, "tiles", "")); t == "nil" || t == "no" || t == "false" || t == "off" {
		s.Tiles = false
	}
	return s, nil
}

// view is the centre and zoom the picture shows.
func (s mapViewSpec) view(pts []geo.LatLon) (geo.LatLon, int) {
	if s.HasView {
		return s.Center, s.Zoom
	}
	c, z := geo.FitView(pts, s.W, s.H, 36, 17)
	if s.Zoom < 0 {
		z = -s.Zoom - 1
	}
	return c, z
}

// ---------------------------------------------------------------------------
// Tiles
// ---------------------------------------------------------------------------

var mapTiles = struct {
	sync.Mutex
	downUntil time.Time
}{}

func mapTileDir() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	h := sha1.Sum([]byte(mapTileUrl()))
	return filepath.Join(dir, "orgs", "map-tiles", hex.EncodeToString(h[:])[:10])
}

func mapTileURL(z, x, y int) string {
	r := strings.NewReplacer("{z}", strconv.Itoa(z), "{x}", strconv.Itoa(x), "{y}", strconv.Itoa(y), "{s}", "a", "{r}", "")
	return r.Replace(mapTileUrl())
}

// mapTile is one tile, from the cache when it is under a month old, else
// asked for (and a stale copy used if the server cannot be reached).
func mapTile(z, x, y int) (image.Image, error) {
	path := filepath.Join(mapTileDir(), strconv.Itoa(z), strconv.Itoa(x), strconv.Itoa(y))
	readFile := func() (image.Image, error) {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		img, _, err := image.Decode(f)
		return img, err
	}
	if st, err := os.Stat(path); err == nil && time.Since(st.ModTime()) < mapTileTTL {
		if img, err := readFile(); err == nil {
			return img, nil
		}
	}
	mapTiles.Lock()
	down := time.Now().Before(mapTiles.downUntil)
	mapTiles.Unlock()
	if down {
		if img, err := readFile(); err == nil {
			return img, nil
		}
		return nil, fmt.Errorf("the tile server was unreachable a moment ago")
	}
	u := mapTileURL(z, x, y)
	if pu, err := url.Parse(u); err != nil || (pu.Scheme != "http" && pu.Scheme != "https") {
		return nil, fmt.Errorf("mapView.tileUrl %q is not an http(s) url", mapTileUrl())
	}
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", mapUserAgent())
	req.Header.Set("Accept", "image/png,image/jpeg,image/*")
	resp, err := mapHTTP.Do(req)
	if err != nil || resp.StatusCode != 200 {
		if resp != nil {
			resp.Body.Close()
		}
		mapTiles.Lock()
		mapTiles.downUntil = time.Now().Add(5 * time.Minute)
		mapTiles.Unlock()
		if img, err2 := readFile(); err2 == nil {
			return img, nil
		}
		if err == nil {
			err = fmt.Errorf("%s answered %s", req.URL.Host, resp.Status)
		}
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("the tile at %s is not a picture: %v", u, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err == nil {
		_ = os.WriteFile(path, b, 0644)
	}
	return img, nil
}

// mapBackdrop is the view's tiles as one JPEG, or nil when tiles are off,
// too many, or not all to be had.
func mapBackdrop(c geo.LatLon, z, w, h int) []byte {
	cx, cy := geo.WorldPx(c, float64(z))
	x0, y0 := cx-float64(w)/2, cy-float64(h)/2
	tx0, ty0 := int(math.Floor(x0/geo.TileSize)), int(math.Floor(y0/geo.TileSize))
	tx1, ty1 := int(math.Floor((x0+float64(w)-1)/geo.TileSize)), int(math.Floor((y0+float64(h)-1)/geo.TileSize))
	if (tx1-tx0+1)*(ty1-ty0+1) > mapMaxTiles {
		return nil
	}
	n := 1 << z
	canvas := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(canvas, canvas.Bounds(), &image.Uniform{color.RGBA{0xaa, 0xd3, 0xdf, 0xff}}, image.Point{}, draw.Src)
	type job struct{ tx, ty int }
	jobs := []job{}
	for ty := ty0; ty <= ty1; ty++ {
		if ty < 0 || ty >= n {
			continue
		}
		for tx := tx0; tx <= tx1; tx++ {
			jobs = append(jobs, job{tx, ty})
		}
	}
	var mu sync.Mutex
	failed := false
	var wg sync.WaitGroup
	ch := make(chan job)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range ch {
				img, err := mapTile(z, ((j.tx%n)+n)%n, j.ty)
				mu.Lock()
				if err != nil {
					if !failed {
						fmt.Fprintf(os.Stderr, "map: tiles: %v\n", err)
					}
					failed = true
				} else {
					at := image.Pt(int(math.Round(float64(j.tx*geo.TileSize)-x0)), int(math.Round(float64(j.ty*geo.TileSize)-y0)))
					draw.Draw(canvas, image.Rectangle{at, at.Add(image.Pt(geo.TileSize, geo.TileSize))}, img, img.Bounds().Min, draw.Src)
				}
				mu.Unlock()
			}
		}()
	}
	for _, j := range jobs {
		mu.Lock()
		stop := failed
		mu.Unlock()
		if stop {
			break
		}
		ch <- j
	}
	close(ch)
	wg.Wait()
	if failed {
		return nil
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, canvas, &jpeg.Options{Quality: 80}); err != nil {
		return nil
	}
	return buf.Bytes()
}

// ---------------------------------------------------------------------------
// The picture
// ---------------------------------------------------------------------------

func mapAttr(s string) string { return html.EscapeString(s) }

// mapGrid is the backdrop without tiles: a pale ground and lines every
// round number of degrees, labelled at the edges.
func mapGrid(b *strings.Builder, c geo.LatLon, z, w, h int) {
	cx, cy := geo.WorldPx(c, float64(z))
	x0, y0 := cx-float64(w)/2, cy-float64(h)/2
	tl := geo.FromWorldPx(x0, y0, float64(z))
	br := geo.FromWorldPx(x0+float64(w), y0+float64(h), float64(z))
	span := math.Max(br.Lon-tl.Lon, tl.Lat-br.Lat)
	step := 0.0001
	for _, s := range []float64{0.0001, 0.0002, 0.0005, 0.001, 0.002, 0.005, 0.01, 0.02, 0.05, 0.1, 0.2, 0.5, 1, 2, 5, 10, 15, 30, 45, 90} {
		step = s
		if span/s <= 8 {
			break
		}
	}
	fmt.Fprintf(b, `<rect width="%d" height="%d" fill="#eef2f4"/>`, w, h)
	prec := 0
	for p := step; p < 1 && prec < 4; p *= 10 {
		prec++
	}
	label := func(v float64, pos, neg string) string {
		hemi := pos
		if v < 0 {
			hemi, v = neg, -v
		}
		return strconv.FormatFloat(v, 'f', prec, 64) + "°" + hemi
	}
	b.WriteString(`<g stroke="#d3dbe1" stroke-width="1">`)
	labels := []string{}
	for lon := math.Ceil(tl.Lon/step) * step; lon <= br.Lon; lon += step {
		x, _ := geo.WorldPx(geo.LatLon{Lat: c.Lat, Lon: lon}, float64(z))
		x -= x0
		fmt.Fprintf(b, `<line x1="%.1f" y1="0" x2="%.1f" y2="%d"/>`, x, x, h)
		labels = append(labels, fmt.Sprintf(`<text x="%.1f" y="%d" text-anchor="middle">%s</text>`, x, h-6, label(lon, "E", "W")))
	}
	for lat := math.Ceil(br.Lat/step) * step; lat <= tl.Lat; lat += step {
		_, y := geo.WorldPx(geo.LatLon{Lat: lat, Lon: c.Lon}, float64(z))
		y -= y0
		fmt.Fprintf(b, `<line x1="0" y1="%.1f" x2="%d" y2="%.1f"/>`, y, w, y)
		labels = append(labels, fmt.Sprintf(`<text x="6" y="%.1f" dominant-baseline="middle">%s</text>`, y-6, label(lat, "N", "S")))
	}
	b.WriteString(`</g><g fill="#8a99a6" font-size="10" font-family="system-ui,sans-serif">`)
	b.WriteString(strings.Join(labels, ""))
	b.WriteString(`</g>`)
}

// mapSvg draws places (those with coordinates) as numbered pins.
func mapSvg(places []geo.Place, s mapViewSpec) string {
	pts := []geo.LatLon{}
	for _, p := range places {
		pts = append(pts, geo.LatLon{Lat: p.Lat, Lon: p.Lon})
	}
	c, z := s.view(pts)
	w, h := s.W, s.H
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" class="org-map-svg" viewBox="0 0 %d %d" width="%d" height="%d" role="img" aria-label="%s" style="max-width:100%%;height:auto;display:block;border-radius:6px">`,
		w, h, w, h, mapAttr(fmt.Sprintf("A map of %d place%s", len(places), plural(len(places)))))
	tiled := false
	if s.Tiles {
		if jpg := mapBackdrop(c, z, w, h); jpg != nil {
			fmt.Fprintf(&b, `<image href="data:image/jpeg;base64,%s" x="0" y="0" width="%d" height="%d"/>`, base64.StdEncoding.EncodeToString(jpg), w, h)
			tiled = true
		}
	}
	if !tiled {
		mapGrid(&b, c, z, w, h)
	}
	cx, cy := geo.WorldPx(c, float64(z))
	x0, y0 := cx-float64(w)/2, cy-float64(h)/2
	numbered := len(places) <= mapMaxPinNum
	// Drawn bottom to top so a southern pin's head is not under a northern one's point.
	order := make([]int, len(places))
	for i := range order {
		order[i] = i
	}
	ys := make([]float64, len(places))
	for i, p := range places {
		_, y := geo.WorldPx(geo.LatLon{Lat: p.Lat, Lon: p.Lon}, float64(z))
		ys[i] = y
	}
	for i := 1; i < len(order); i++ {
		for j := i; j > 0 && ys[order[j]] < ys[order[j-1]]; j-- {
			order[j], order[j-1] = order[j-1], order[j]
		}
	}
	b.WriteString(`<g font-family="system-ui,sans-serif" font-weight="600">`)
	scale := math.Pow(2, float64(z)) * geo.TileSize
	for _, i := range order {
		p := places[i]
		x, y := geo.WorldPx(geo.LatLon{Lat: p.Lat, Lon: p.Lon}, float64(z))
		x -= x0
		// The same place on the next world over, when the view wraps.
		for x < -20 {
			x += scale
		}
		for x > float64(w)+20 {
			x -= scale
		}
		y -= y0
		if x < -20 || y < -4 || x > float64(w)+20 || y > float64(h)+30 {
			continue
		}
		col := p.Colour
		if col == "" {
			col = geo.ColourPlain
		}
		ll := geo.LatLon{Lat: p.Lat, Lon: p.Lon}
		fmt.Fprintf(&b, `<a href="%s" target="_blank" data-hash="%s" data-file="%s" data-line="%d"><title>%s</title><g transform="translate(%.1f %.1f)">`,
			mapAttr(geo.OSMUrl(ll, 16)), mapAttr(p.Hash), mapAttr(p.Filename), p.Line, mapAttr(p.Title+" — "+geo.Format(ll)), x, y)
		fmt.Fprintf(&b, `<ellipse cx="0" cy="0" rx="5" ry="2" fill="#000" fill-opacity=".25"/><path d="M0 0C-2-6-10-9.5-10-17a10 10 0 1 1 20 0C10-9.5 2-6 0 0Z" fill="%s" stroke="#fff" stroke-width="1.5"/>`, col)
		if numbered {
			fmt.Fprintf(&b, `<text x="0" y="-13.5" text-anchor="middle" font-size="10.5" fill="#fff">%d</text>`, i+1)
		} else {
			b.WriteString(`<circle cx="0" cy="-17" r="3.5" fill="#fff"/>`)
		}
		b.WriteString(`</g></a>`)
	}
	b.WriteString(`</g>`)
	if attr := mapAttribution(); tiled && attr != "" {
		tw := float64(len([]rune(attr)))*5.4 + 10
		fmt.Fprintf(&b, `<a href="https://www.openstreetmap.org/copyright" target="_blank"><rect x="%.1f" y="%d" width="%.1f" height="16" fill="#fff" fill-opacity=".75"/><text x="%d" y="%d" text-anchor="end" font-size="10" font-family="system-ui,sans-serif" fill="#333">%s</text></a>`,
			float64(w)-tw, h-16, tw, w-5, h-4, mapAttr(attr))
	}
	b.WriteString(`</svg>`)
	return b.String()
}

// mapPlaced is the places that have coordinates.
func mapPlaced(ps []geo.Place) []geo.Place {
	out := []geo.Place{}
	for _, p := range ps {
		if p.Has {
			out = append(out, p)
		}
	}
	return out
}

// mapFigure is the html an export shows for a #+MAP: line: the picture and
// the numbered places, with what worg needs to draw a live map instead.
func mapFigure(places []geo.Place, s mapViewSpec, unplaced int) string {
	var b strings.Builder
	rows := ""
	if s.To > 0 {
		rows = fmt.Sprintf(` data-map-from="%d" data-map-to="%d"`, s.From, s.To)
	}
	fmt.Fprintf(&b, `<figure class="org-map" data-map-options="%s" data-map-file="%s" data-map-height="%d"%s style="margin:1em 0">`,
		mapAttr(s.Options), mapAttr(s.File), s.H, rows)
	b.WriteString(mapSvg(places, s))
	b.WriteString(`<figcaption style="font-size:.9em;margin-top:.4em;text-align:left">`)
	if s.Title != "" {
		fmt.Fprintf(&b, `<strong>%s</strong>`, mapAttr(s.Title))
	}
	if len(places) == 0 {
		b.WriteString(` <em>No places to show.</em>`)
	}
	b.WriteString(`<ol class="org-map-places" style="margin:.3em 0;padding-left:1.6em;columns:2 22em">`)
	for i, p := range places {
		if i >= mapMaxPinNum {
			fmt.Fprintf(&b, `</ol><p>and %d more</p><ol style="display:none">`, len(places)-mapMaxPinNum)
			break
		}
		ll := geo.LatLon{Lat: p.Lat, Lon: p.Lon}
		label := p.Title
		if p.Label != "" {
			label += " (" + p.Label + ")"
		}
		fmt.Fprintf(&b, `<li><a href="%s" target="_blank">%s</a> <span class="org-map-coords" style="opacity:.65;font-size:.9em">%s</span></li>`,
			mapAttr(geo.OSMUrl(ll, 16)), mapAttr(label), mapAttr(geo.Format(ll)))
	}
	b.WriteString(`</ol>`)
	if unplaced > 0 {
		fmt.Fprintf(&b, `<p style="opacity:.65;font-size:.9em;margin:0">%d place%s not on the map yet (addresses not looked up).</p>`, unplaced, plural(unplaced))
	}
	b.WriteString(`</figcaption></figure>`)
	return b.String()
}

// mapListLines is a #+MAP: line for a backend with no pictures: the places
// as a numbered org list, each linked to openstreetmap.org.
func mapListLines(ind string, places []geo.Place, s mapViewSpec) []string {
	out := []string{}
	if s.Title != "" {
		out = append(out, ind+"*"+strings.ReplaceAll(s.Title, "*", "")+"*", "")
	}
	if len(places) == 0 {
		return append(out, ind+"/No places to show./")
	}
	for i, p := range places {
		ll := geo.LatLon{Lat: p.Lat, Lon: p.Lon}
		title := strings.NewReplacer("[", "(", "]", ")").Replace(p.Title)
		if p.Label != "" {
			title += " (" + p.Label + ")"
		}
		out = append(out, fmt.Sprintf("%s%d. [[%s][%s]]: %s", ind, i+1, geo.OSMUrl(ll, 16), title, geo.Format(ll)))
	}
	return out
}

// mapSubtreeOf is the rows of the original file covered by the heading a
// #+MAP: line of the export copy is under, or ok false above the first
// heading. The copy's rows are not the file's (an earlier pass may have
// turned one line into three), so the heading is found again in the file by
// its outline path: the headline lines above it, each level's own.
func mapSubtreeOf(path string, copyLines []string, at int) (from, to int, ok bool) {
	want := []string{}
	lvl := 1 << 30
	for i := at; i >= 0; i-- {
		l := copyLines[i]
		if !anyHeadRe.MatchString(l) {
			continue
		}
		n := len(l) - len(strings.TrimLeft(l, "*"))
		if n < lvl {
			want = append([]string{strings.TrimSpace(l)}, want...)
			lvl = n
			if n == 1 {
				break
			}
		}
	}
	if len(want) == 0 {
		return 0, 0, false
	}
	file, err := readFileLines(path)
	if err != nil {
		return 0, 0, false
	}
	k, row := 0, -1
	for i, l := range file {
		if anyHeadRe.MatchString(l) && strings.TrimSpace(l) == want[k] {
			k++
			if k == len(want) {
				row = i
				break
			}
		}
	}
	if row < 0 {
		return 0, 0, false
	}
	n := len(file[row]) - len(strings.TrimLeft(file[row], "*"))
	return row, subtreeEndRow(file, row, n, row), true
}

func mapInRows(places []geo.Place, from, to int) []geo.Place {
	out := []geo.Place{}
	for _, p := range places {
		if p.Line >= from && p.Line <= to {
			out = append(out, p)
		}
	}
	return out
}

// mapMayApply is whether a file's export copy needs the map pass.
func mapMayApply(text string) bool {
	return featureOn(mapFeature) && strings.Contains(strings.ToLower(text), "#+map:")
}

// mapExportLines turns each #+MAP: line (outside blocks) of the export copy
// into its picture (html) or list (anything else).
func mapExportLines(path string, lines []string, backend string) ([]string, bool) {
	kind := trackerBackendKind(backend)
	out := make([]string, 0, len(lines))
	changed := false
	inBlock := ""
	for i, l := range lines {
		t := strings.ToLower(strings.TrimSpace(l))
		if inBlock != "" {
			if strings.HasPrefix(t, "#+end_"+inBlock) {
				inBlock = ""
			}
			out = append(out, l)
			continue
		}
		if strings.HasPrefix(t, "#+begin_") {
			inBlock = strings.Fields(strings.TrimPrefix(t, "#+begin_") + " ")[0]
			out = append(out, l)
			continue
		}
		m := mapLineRe.FindStringSubmatch(l)
		if m == nil {
			out = append(out, l)
			continue
		}
		changed = true
		ind := m[1]
		spec, err := mapSpecFrom(strings.TrimSpace(m[2]), path)
		var places []geo.Place
		if err == nil {
			file := path
			if spec.Query != "" {
				file = ""
			}
			places, err = mapPlaces(spec.Query, file, spec.Colour)
			if err == nil && spec.Query == "" {
				if from, to, ok := mapSubtreeOf(path, lines, i); ok {
					spec.From, spec.To = from, to
					places = mapInRows(places, from, to)
				}
			}
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "map: %s:%d: %v\n", filepath.Base(path), i+1, err)
			out = append(out, ind+"/Map: "+strings.ReplaceAll(err.Error(), "/", "")+"/")
			continue
		}
		placed := mapPlaced(places)
		if kind == "html" {
			out = append(out, ind+"#+BEGIN_EXPORT html", mapFigure(placed, spec, len(places)-len(placed)), ind+"#+END_EXPORT")
		} else {
			out = append(out, mapListLines(ind, placed, spec)...)
		}
	}
	return out, changed
}

/* SDOC: API
* GET /map/static — A Map As A Picture

	Needs =features: mapView:=. The SVG an html export draws for a =#+MAP:=
	line (tiles composed into one JPEG under numbered pins, or pins over a
	grid of degrees when tiles are off or out of reach), as
	=image/svg+xml=. Parameters: =query=, =file= (as =/map/places=; neither
	= every place), or =ll=49.28,-123.12= (one pin, titled =title=);
	=options= (a =#+MAP:= line's words: =zoom=, =center=, =height=,
	=width=, =colour=, =tiles:nil=), and =width=/=height=/=zoom= on their
	own. =format=json= answers ={Ok, Svg, Places}= instead (for a client
	that only speaks json, VS Code's hover).
EDOC */
func RequestMapStatic(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, mapFeature) {
		return
	}
	q := r.URL.Query()
	opts := q.Get("options")
	for _, k := range []string{"zoom", "center", "height", "width", "colour", "tiles", "query"} {
		if v := q.Get(k); v != "" {
			opts += " " + k + ":" + `"` + strings.ReplaceAll(v, `"`, `\"`) + `"`
		}
	}
	spec, err := mapSpecFrom(opts, q.Get("file"))
	if err != nil {
		dynJson(w, common.Result{Msg: err.Error()})
		return
	}
	var places []geo.Place
	if v := q.Get("ll"); v != "" {
		ll, ok := geo.Parse(v)
		if !ok {
			dynJson(w, common.Result{Msg: fmt.Sprintf("ll=%s is not coordinates", v)})
			return
		}
		places = []geo.Place{{Title: q.Get("title"), Has: true, Lat: ll.Lat, Lon: ll.Lon, Colour: geo.ColourPlain}}
	} else {
		places, err = mapPlaces(spec.Query, q.Get("file"), spec.Colour)
		if err != nil {
			dynJson(w, common.Result{Msg: err.Error()})
			return
		}
		places = mapPlaced(places)
	}
	svg := mapSvg(places, spec)
	if q.Get("format") == "json" {
		dynJson(w, map[string]interface{}{"Ok": true, "Svg": svg, "Places": len(places)})
		return
	}
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "private, max-age=300")
	io.WriteString(w, svg)
}
