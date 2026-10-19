package orgs

import (
	"bufio"
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Campaigns: an org file that says it is one gathers the maps of a game into
// areas - sections of the world - each a heading tagged :area: with a list of
// links to the maps in it. worg's campaign window lists every map (every org
// file with a .dungeon.json beside it), the campaigns, and edits them; every
// write is a splice of the lines it changes (see Traps: line edits).
//
//	#+TITLE: The Shattered Coast
//	#+CAMPAIGN: t
//	#+FILETAGS: :campaign:
//	#+CAMPAIGN_ICON: 🐉
//	#+CAMPAIGN_ART: images/the-shattered-coast-art.png
//
//	A line or two about it.
//
//	* The Northern Reaches                                         :area:
//	- [[file:../dungeons/grotto.org][The Drowned Grotto]]

type campaignMap struct {
	File    string `json:"file"`              // the map's org file, absolute
	Title   string `json:"title"`             // as the link describes it
	Link    string `json:"link"`              // as written, relative to the campaign
	Missing bool   `json:"missing,omitempty"` // no map there (moved, or not a map)
}

type campaignArea struct {
	Name string        `json:"name"`
	Line int           `json:"line"` // its heading, zero based
	Maps []campaignMap `json:"maps"`
}

type campaign struct {
	File        string         `json:"file"`
	Title       string         `json:"title"`
	Icon        string         `json:"icon"`
	Art         string         `json:"art"` // relative to the campaign file
	Description string         `json:"description"`
	Areas       []campaignArea `json:"areas"`
	Modified    int64          `json:"modified"`
}

var (
	cpHeadingRe = regexp.MustCompile(`^(\*+)\s+(.*)$`)
	headingTagsRe = regexp.MustCompile(`\s+(:[\p{L}\p{N}_@#%:]+:)\s*$`)
	keywordLineRe = regexp.MustCompile(`^#\+([A-Za-z_]+):\s*(.*)$`)
	fileLinkRe    = regexp.MustCompile(`\[\[file:([^\]]+?)\](?:\[([^\]]*)\])?\]`)
	// A list item that is nothing but a link to an org file: what an area's
	// list of maps is made of (and what setting the list replaces).
	mapItemRe = regexp.MustCompile(`^\s*[-+]\s+\[\[file:[^\]]+?\.org(?:::[^\]]*)?\](?:\[[^\]]*\])?\]\s*$`)
)

// The keywords above the first heading, upper case, and where the preamble
// ends (the first heading's line, or the end).
func preamble(lines []string) (map[string]string, map[string]int, int) {
	kw, at := map[string]string{}, map[string]int{}
	end := len(lines)
	for i, l := range lines {
		if cpHeadingRe.MatchString(l) {
			end = i
			break
		}
		if m := keywordLineRe.FindStringSubmatch(strings.TrimSpace(l)); m != nil {
			k := strings.ToUpper(m[1])
			if _, seen := kw[k]; !seen {
				kw[k], at[k] = strings.TrimSpace(m[2]), i
			}
		}
	}
	return kw, at, end
}

func isCampaignHeader(kw map[string]string) bool {
	if v := strings.ToLower(kw["CAMPAIGN"]); v != "" && v != "nil" && v != "no" && v != "false" {
		return true
	}
	for _, t := range strings.FieldsFunc(kw["FILETAGS"], func(r rune) bool { return r == ':' || r == ' ' }) {
		if strings.EqualFold(t, "campaign") {
			return true
		}
	}
	return false
}

// A heading's title and tags.
func cpHeadingParts(text string) (string, []string) {
	tags := []string{}
	if m := headingTagsRe.FindStringSubmatchIndex(text); m != nil {
		for _, t := range strings.Split(strings.Trim(text[m[2]:m[3]], ":"), ":") {
			if t != "" {
				tags = append(tags, t)
			}
		}
		text = text[:m[0]]
	}
	return strings.TrimSpace(text), tags
}

func cpHasTag(tags []string, t string) bool {
	for _, x := range tags {
		if strings.EqualFold(x, t) {
			return true
		}
	}
	return false
}

// Whether an org file is a map: it has a .dungeon.json beside it.
func isMapFile(org string) bool {
	_, err := os.Stat(strings.TrimSuffix(org, ".org") + ".dungeon.json")
	return err == nil
}

// The description: the preamble's text, keywords, drawers and comments left out.
func campaignDescription(lines []string, end int) string {
	out := []string{}
	in := false
	for _, l := range lines[:end] {
		t := strings.TrimSpace(l)
		switch {
		case strings.HasPrefix(t, ":") && strings.HasSuffix(t, ":") && len(t) > 1 && !in:
			in = !strings.EqualFold(t, ":END:")
			continue
		case in:
			if strings.EqualFold(t, ":END:") {
				in = false
			}
			continue
		case strings.HasPrefix(t, "#"):
			continue
		}
		out = append(out, t)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

// readCampaign reads a campaign from its lines; false when the file does not
// say it is one.
func readCampaign(file string, lines []string) (campaign, bool) {
	kw, _, end := preamble(lines)
	if !isCampaignHeader(kw) {
		return campaign{}, false
	}
	c := campaign{File: file, Title: kw["TITLE"], Icon: kw["CAMPAIGN_ICON"], Art: kw["CAMPAIGN_ART"], Areas: []campaignArea{}}
	if c.Title == "" {
		c.Title = strings.TrimSuffix(filepath.Base(file), ".org")
	}
	c.Description = campaignDescription(lines, end)
	dir := filepath.Dir(file)
	var cur *campaignArea
	for i := end; i < len(lines); i++ {
		if m := cpHeadingRe.FindStringSubmatch(lines[i]); m != nil {
			title, tags := cpHeadingParts(m[2])
			if cpHasTag(tags, "area") {
				c.Areas = append(c.Areas, campaignArea{Name: title, Line: i, Maps: []campaignMap{}})
				cur = &c.Areas[len(c.Areas)-1]
			} else {
				cur = nil
			}
			continue
		}
		if cur == nil {
			continue
		}
		for _, m := range fileLinkRe.FindAllStringSubmatch(lines[i], -1) {
			link := m[1]
			target := strings.SplitN(link, "::", 2)[0]
			if !strings.HasSuffix(strings.ToLower(target), ".org") {
				continue
			}
			abs := target
			if strings.HasPrefix(abs, "~") {
				if home, err := os.UserHomeDir(); err == nil {
					abs = filepath.Join(home, strings.TrimPrefix(abs[1:], "/"))
				}
			} else if !filepath.IsAbs(abs) {
				abs = filepath.Join(dir, filepath.FromSlash(abs))
			}
			abs = filepath.Clean(abs)
			title := m[2]
			if title == "" {
				title = strings.TrimSuffix(filepath.Base(abs), ".org")
			}
			cur.Maps = append(cur.Maps, campaignMap{File: abs, Title: title, Link: link, Missing: !isMapFile(abs)})
		}
	}
	if info, err := os.Stat(file); err == nil {
		c.Modified = info.ModTime().Unix()
	}
	return c, true
}

// headerLines reads a file's lines up to its first heading (enough to know
// whether it is a campaign), without reading the rest of a big file.
func headerLines(p string) []string {
	f, err := os.Open(p)
	if err != nil {
		return nil
	}
	defer f.Close()
	out := []string{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() && len(out) < 300 {
		l := sc.Text()
		if cpHeadingRe.MatchString(l) {
			break
		}
		out = append(out, l)
	}
	return out
}

// walkOrg calls fn for every org file under the org directories, hidden
// folders left out.
func walkOrg(fn func(p string)) {
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
			if strings.HasSuffix(e.Name(), ".org") {
				fn(p)
			}
			return nil
		})
	}
}

// A path for showing: relative to the org directory it is in.
func orgRel(p string) string {
	for _, dir := range Conf().Server.OrgDirs {
		if rel, err := filepath.Rel(dir, p); err == nil && !strings.HasPrefix(rel, "..") {
			return filepath.ToSlash(rel)
		}
	}
	return p
}

/* SDOC: API
* GET /dnd/campaigns — Campaigns
	Every campaign under the org directories: an org file with =#+CAMPAIGN: t= (or =campaign= in its
	=#+FILETAGS:=). Its =#+TITLE:= names it, =#+CAMPAIGN_ICON:= is its icon (an emoji) and
	=#+CAMPAIGN_ART:= its picture (relative to the file, fetched with =GET /dnd/dungeon/image=); the
	text before the first heading describes it. Each heading tagged =:area:= is an area - a section of
	the world - and the links to org files in its own text are its maps, in order.

	*Method:* =GET=

	*Response:* ={Ok, Campaigns: [{file, title, icon, art, description, modified, areas: [{name, line,
	maps: [{file, title, link, missing}]}]}]}=, newest first. =missing= is a link to no map (moved,
	or a file with no =.dungeon.json= beside it).
	EDOC */
func RequestDndCampaigns(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	out := []campaign{}
	walkOrg(func(p string) {
		kw, _, _ := preamble(headerLines(p))
		if !isCampaignHeader(kw) {
			return
		}
		if c, ok := readCampaign(p, fileLinesFresh(p)); ok {
			out = append(out, c)
		}
	})
	sort.Slice(out, func(a, b int) bool { return out[a].Modified > out[b].Modified })
	dndJson(w, map[string]interface{}{"Ok": true, "Campaigns": out})
}

// A file's lines as they are on disk now.
func fileLinesFresh(p string) []string {
	b, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	return strings.Split(strings.TrimSuffix(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n"), "\n")
}

type mapEntry struct {
	File     string `json:"file"`
	Rel      string `json:"rel"`
	Name     string `json:"name"`
	World    bool   `json:"world,omitempty"`
	Modified int64  `json:"modified"`
}

/* SDOC: API
* GET /dnd/maps — Every Map
	Every map and dungeon under the org directories - each org file with a =.dungeon.json= beside it,
	what the file view offers its Map view for - for the campaign window's list.

	*Method:* =GET=

	*Response:* ={Ok, Maps: [{file, rel, name, world, modified}]}=, =rel= the file's path under its org
	directory, =name= the map's own (else the file's =#+TITLE:=, else its name), newest first.
	EDOC */
func RequestDndMaps(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	out := []mapEntry{}
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
			if !strings.HasSuffix(e.Name(), ".dungeon.json") || len(out) >= 2000 {
				return nil
			}
			org := strings.TrimSuffix(p, ".dungeon.json") + ".org"
			if _, err := os.Stat(org); err != nil {
				return nil
			}
			var head struct {
				Name  string          `json:"name"`
				World json.RawMessage `json:"world"`
			}
			if b, err := os.ReadFile(p); err == nil {
				json.Unmarshal(b, &head)
			}
			name := head.Name
			if name == "" {
				kw, _, _ := preamble(headerLines(org))
				name = kw["TITLE"]
			}
			if name == "" {
				name = strings.TrimSuffix(filepath.Base(org), ".org")
			}
			var mod int64
			if info, err := os.Stat(org); err == nil {
				mod = info.ModTime().Unix()
			}
			out = append(out, mapEntry{File: org, Rel: orgRel(org), Name: name, Modified: mod,
				World: len(head.World) > 0 && string(head.World) != "null"})
			return nil
		})
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Modified > out[b].Modified })
	dndJson(w, map[string]interface{}{"Ok": true, "Maps": out})
}

// ---- thumbnails ----------------------------------------------------------------------

// The picture a map is drawn from: its backdrop (a picture dungeon, a
// generated map), else the drawing the dungeon generator saved.
func mapPictureOf(org string) string {
	stem := strings.TrimSuffix(org, ".org")
	var d struct {
		Backdrop *struct {
			Src string `json:"src"`
		} `json:"backdrop"`
	}
	if b, err := os.ReadFile(stem + ".dungeon.json"); err == nil {
		json.Unmarshal(b, &d)
	}
	if d.Backdrop != nil && d.Backdrop.Src != "" && !strings.Contains(d.Backdrop.Src, "..") {
		p := filepath.Join(filepath.Dir(org), filepath.FromSlash(d.Backdrop.Src))
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	base := filepath.Base(stem)
	for _, e := range []string{"-map.svg", "-player.svg", "-page.svg"} {
		p := filepath.Join(filepath.Dir(org), "maps", base+e)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

var thumbMu sync.Mutex

// shrink scales a picture down to at most `w` pixels across by averaging
// the pixels each new one covers (a box filter: no library needed, and a map
// shrunk this way stays legible).
func shrink(src image.Image, w int) image.Image {
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	if sw <= w {
		return src
	}
	h := max(1, sh*w/sw)
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		y0, y1 := b.Min.Y+y*sh/h, b.Min.Y+max((y+1)*sh/h, y*sh/h+1)
		for x := 0; x < w; x++ {
			x0, x1 := b.Min.X+x*sw/w, b.Min.X+max((x+1)*sw/w, x*sw/w+1)
			var r, g, bl, a, n uint64
			// At most 4x4 samples a pixel: enough for a thumbnail, and fast
			// on a picture of eight thousand pixels.
			sx, sy := max(1, (x1-x0)/4), max(1, (y1-y0)/4)
			for yy := y0; yy < y1; yy += sy {
				for xx := x0; xx < x1; xx += sx {
					cr, cg, cb, ca := src.At(xx, yy).RGBA()
					r, g, bl, a, n = r+uint64(cr), g+uint64(cg), bl+uint64(cb), a+uint64(ca), n+1
				}
			}
			dst.SetRGBA(x, y, color.RGBA{uint8(r / n >> 8), uint8(g / n >> 8), uint8(bl / n >> 8), uint8(a / n >> 8)})
		}
	}
	return dst
}

/* SDOC: API
* GET /dnd/map/thumb — A Map's Thumbnail
	A small picture of a map, for lists: its picture (a picture dungeon's, a generated map's) shrunk to
	=w= pixels across (default 360, at most 800) as a JPEG, made once and kept in the user cache until
	the picture changes; a WebP picture is sent as it is. A dungeon drawn by the generator has its
	saved drawing (=maps/name-map.svg=) sent as it is - an svg is small and scales.

	*Method:* =GET=

	*Query Parameters:* =file= (the map's org file), =w=.

	*Response:* the picture, or 404 when the map has none.
	EDOC */
func RequestDndMapThumb(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	org, ok := insideOrgDirs(strings.TrimSpace(r.URL.Query().Get("file")))
	if !ok || filepath.Ext(org) != ".org" {
		dndError(w, http.StatusBadRequest, "the file has to be an org file inside one of the org directories")
		return
	}
	src := mapPictureOf(org)
	if src == "" {
		dndError(w, http.StatusNotFound, "that map has no picture yet")
		return
	}
	ext := strings.ToLower(filepath.Ext(src))
	if ext == ".svg" || ext == ".webp" {
		serveShow(w, r, src)
		return
	}
	width, _ := strconv.Atoi(r.URL.Query().Get("w"))
	if width <= 0 {
		width = 360
	}
	width = min(width, 800)
	info, err := os.Stat(src)
	if err != nil {
		dndError(w, http.StatusNotFound, "no picture at %s", src)
		return
	}
	sum := sha1.Sum([]byte(fmt.Sprintf("%s|%d|%d|%d", src, info.ModTime().UnixNano(), info.Size(), width)))
	cache := ""
	if dir, err := os.UserCacheDir(); err == nil {
		cache = filepath.Join(dir, "orgs", "thumbs", hex.EncodeToString(sum[:])+".jpg")
		if _, err := os.Stat(cache); err == nil {
			w.Header().Set("Cache-Control", "private, max-age=600")
			http.ServeFile(w, r, cache)
			return
		}
	}
	// One at a time: a list of forty maps asks for forty thumbnails at once,
	// and decoding forty big pictures together would take the memory with it.
	thumbMu.Lock()
	defer thumbMu.Unlock()
	f, err := os.Open(src)
	if err != nil {
		dndError(w, http.StatusNotFound, "%s", err)
		return
	}
	img, _, err := image.Decode(f)
	f.Close()
	if err != nil {
		// Not one Go reads: the picture as it is.
		serveShow(w, r, src)
		return
	}
	small := shrink(img, width)
	// A clear picture on the dark of a list, not on black.
	flat := image.NewRGBA(small.Bounds())
	draw.Draw(flat, flat.Bounds(), &image.Uniform{color.RGBA{0x2a, 0x22, 0x1a, 0xff}}, image.Point{}, draw.Src)
	draw.Draw(flat, flat.Bounds(), small, small.Bounds().Min, draw.Over)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, flat, &jpeg.Options{Quality: 82}); err != nil {
		dndError(w, http.StatusInternalServerError, "%s", err)
		return
	}
	if cache != "" {
		os.MkdirAll(filepath.Dir(cache), 0755)
		os.WriteFile(cache, buf.Bytes(), 0644)
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, max-age=600")
	w.Write(buf.Bytes())
}

// ---- writing campaigns ------------------------------------------------------------------

// A link from a campaign file to a map: relative to the campaign's folder.
func relLink(campaignFile, target string) string {
	if rel, err := filepath.Rel(filepath.Dir(campaignFile), target); err == nil {
		rel = filepath.ToSlash(rel)
		if !strings.HasPrefix(rel, "../") && !strings.HasPrefix(rel, "./") {
			rel = "./" + rel
		}
		return rel
	}
	return target
}

func campaignSlug(name string) string {
	s := strings.ToLower(safeShowName(name))
	s = strings.Join(strings.FieldsFunc(s, func(r rune) bool { return r == ' ' || r == '\'' || r == ',' || r == '&' || r == '(' || r == ')' }), "-")
	if s == "" {
		s = "campaign"
	}
	return s
}

// Sets a keyword in the preamble: its line replaced, else a new line after
// the last keyword line (else at the top). An empty value takes it out.
func setKeyword(lines []string, key, val string) []string {
	_, at, end := preamble(lines)
	line := "#+" + key + ": " + val
	if i, ok := at[key]; ok {
		if val == "" {
			return append(lines[:i:i], lines[i+1:]...)
		}
		lines[i] = line
		return lines
	}
	if val == "" {
		return lines
	}
	last := -1
	for i := 0; i < end; i++ {
		if keywordLineRe.MatchString(strings.TrimSpace(lines[i])) {
			last = i
		}
	}
	out := append([]string{}, lines[:last+1]...)
	out = append(out, line)
	return append(out, lines[last+1:]...)
}

// Replaces the description: the preamble after its keywords (drawers kept).
func setDescription(lines []string, text string) []string {
	_, _, end := preamble(lines)
	from := 0
	for i := 0; i < end; i++ {
		t := strings.TrimSpace(lines[i])
		if keywordLineRe.MatchString(t) || (strings.HasPrefix(t, ":") && strings.HasSuffix(t, ":")) {
			from = i + 1
		}
	}
	// Drawer lines between keywords are kept by starting after the last of
	// either; the description is what follows them.
	body := []string{""}
	if t := strings.TrimSpace(text); t != "" {
		body = append(body, strings.Split(t, "\n")...)
		body = append(body, "")
	}
	out := append([]string{}, lines[:from]...)
	out = append(out, body...)
	return append(out, lines[end:]...)
}

// Where an area is: its heading's line, and the end of its subtree (the next
// heading at its level or above) and of its own text (the next heading).
func findArea(lines []string, name string, near int) (int, int, int, bool) {
	best := -1
	for i, l := range lines {
		m := cpHeadingRe.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		title, tags := cpHeadingParts(m[2])
		if cpHasTag(tags, "area") && title == name && (best < 0 || cpAbs(i-near) < cpAbs(best-near)) {
			best = i
		}
	}
	if best < 0 {
		return 0, 0, 0, false
	}
	level := len(cpHeadingRe.FindStringSubmatch(lines[best])[1])
	own, sub := len(lines), len(lines)
	for i := best + 1; i < len(lines); i++ {
		if m := cpHeadingRe.FindStringSubmatch(lines[i]); m != nil {
			if own == len(lines) {
				own = i
			}
			if len(m[1]) <= level {
				sub = i
				break
			}
		}
	}
	return best, own, sub, true
}

func cpAbs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// An area's heading: stars, the name, and its tag a little way after.
func areaHeading(stars, name string) string {
	h := stars + " " + strings.TrimSpace(name)
	return h + strings.Repeat(" ", max(2, 70-len([]rune(h)))) + ":area:"
}

func mapTitleOf(org string) string {
	var head struct {
		Name string `json:"name"`
	}
	if b, err := os.ReadFile(strings.TrimSuffix(org, ".org") + ".dungeon.json"); err == nil {
		json.Unmarshal(b, &head)
	}
	if head.Name != "" {
		return head.Name
	}
	kw, _, _ := preamble(headerLines(org))
	if kw["TITLE"] != "" {
		return kw["TITLE"]
	}
	return strings.TrimSuffix(filepath.Base(org), ".org")
}

// The campaign file asked for, which must be one.
func campaignFile(w http.ResponseWriter, file string) (string, []string, bool) {
	p, ok := insideOrgDirs(strings.TrimSpace(file))
	if !ok || filepath.Ext(p) != ".org" {
		dndError(w, http.StatusBadRequest, "the file has to be an org file inside one of the org directories")
		return "", nil, false
	}
	lines := fileLinesFresh(p)
	if kw, _, _ := preamble(lines); !isCampaignHeader(kw) {
		dndError(w, http.StatusBadRequest, "%s is not a campaign", filepath.Base(p))
		return "", nil, false
	}
	return p, lines, true
}

// The campaign's art: a picture sent or fetched, kept beside the campaign
// (images/<slug>-art.ext), its path relative to the campaign file.
func keepCampaignArt(file, title, data, url string) (string, int, error) {
	pic, _, code, err := takePicture(data, url, "")
	if err != nil {
		return "", code, err
	}
	ext := pictureExt(pic)
	if ext == "" {
		return "", http.StatusUnsupportedMediaType, fmt.Errorf("that is not a JPEG, PNG, WebP or GIF picture")
	}
	if len(pic) > showMaxBytes {
		return "", http.StatusRequestEntityTooLarge, fmt.Errorf("that is more than %d MB", showMaxBytes>>20)
	}
	dir := filepath.Join(filepath.Dir(file), "images")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", http.StatusInternalServerError, err
	}
	// A new name each time: the browser keeps pictures by their address.
	name := fmt.Sprintf("%s-art-%s%s", campaignSlug(title), time.Now().Format("20060102150405"), ext)
	if err := os.WriteFile(filepath.Join(dir, name), pic, 0644); err != nil {
		return "", http.StatusInternalServerError, err
	}
	return "images/" + name, 0, nil
}

/* SDOC: API
* POST /dnd/campaign — Make or Change a Campaign
	Makes a campaign (=op: new=) or changes one (=op: edit=), and its areas and their maps:

	| =op=     | What it does                                                                                  |
	|----------+-----------------------------------------------------------------------------------------------|
	| =new=    | Writes =<dir>/<name>.org= (=dir= under the first org directory, =campaigns= by default).      |
	| =edit=   | Sets =title=, =icon=, =description= when sent; =art= / =artUrl= a new picture, =noArt= none.    |
	| =area=   | Adds an area =name= at the end.                                                               |
	| =rename= | Renames area =area= (its heading at =line=) to =name=.                                        |
	| =delete= | Takes area =area= out, its heading and everything under it.                                   |
	| =order=  | Puts the areas in the order of =names=.                                                       |
	| =maps=   | Sets area =area='s maps to =maps= (org files), in that order: the list items that are only a link to an org file are replaced, the rest of its text kept. |

	*Method:* =POST=

	*Request Body:* ={op, file, dir, name, title, icon, description, art, artUrl, noArt, area, line, names, maps}=

	*Response:* ={Ok, Msg, File, Campaign}=, the campaign as =GET /dnd/campaigns= has it.
	EDOC */
func PostDndCampaign(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	r.Body = http.MaxBytesReader(w, r.Body, showMaxBytes*2)
	var req struct {
		Op          string   `json:"op"`
		File        string   `json:"file"`
		Dir         string   `json:"dir"`
		Name        string   `json:"name"`
		Title       *string  `json:"title"`
		Icon        *string  `json:"icon"`
		Description *string  `json:"description"`
		Art         string   `json:"art"`
		ArtUrl      string   `json:"artUrl"`
		NoArt       bool     `json:"noArt"`
		Area        string   `json:"area"`
		Line        int      `json:"line"`
		Names       []string `json:"names"`
		Maps        []string `json:"maps"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		dndError(w, http.StatusBadRequest, "could not read request: %s", err)
		return
	}
	var file string
	var lines []string
	if req.Op == "new" {
		name := strings.TrimSpace(req.Name)
		if name == "" {
			dndError(w, http.StatusBadRequest, "a campaign needs a name")
			return
		}
		if len(Conf().Server.OrgDirs) == 0 {
			dndError(w, http.StatusBadRequest, "there are no org directories")
			return
		}
		dir := strings.TrimSpace(req.Dir)
		if dir == "" {
			dir = "campaigns"
		}
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(Conf().Server.OrgDirs[0], filepath.FromSlash(dir))
		}
		file = filepath.Join(dir, campaignSlug(name)+".org")
		if _, ok := insideOrgDirs(file); !ok || strings.Contains(req.Dir, "..") {
			dndError(w, http.StatusBadRequest, "the campaign has to go inside one of the org directories")
			return
		}
		if _, err := os.Stat(file); err == nil {
			dndError(w, http.StatusConflict, "%s already exists", orgRel(file))
			return
		}
		if err := os.MkdirAll(dir, 0755); err != nil {
			dndError(w, http.StatusInternalServerError, "%s", err)
			return
		}
		icon := "🗺️"
		if req.Icon != nil && strings.TrimSpace(*req.Icon) != "" {
			icon = strings.TrimSpace(*req.Icon)
		}
		lines = []string{"#+TITLE: " + name, "#+CAMPAIGN: t", "#+FILETAGS: :campaign:", "#+CAMPAIGN_ICON: " + icon, ""}
		if req.Description != nil && strings.TrimSpace(*req.Description) != "" {
			lines = append(lines, strings.Split(strings.TrimSpace(*req.Description), "\n")...)
			lines = append(lines, "")
		}
		if req.Art != "" || req.ArtUrl != "" {
			art, code, err := keepCampaignArt(file, name, req.Art, req.ArtUrl)
			if err != nil {
				dndError(w, code, "%s", err)
				return
			}
			lines = setKeyword(lines, "CAMPAIGN_ART", art)
		}
		if err := writeLines(file, lines); err != nil {
			dndError(w, http.StatusInternalServerError, "%s", err)
			return
		}
		answerCampaign(w, file, "Made "+name)
		return
	}
	var ok bool
	if file, lines, ok = campaignFile(w, req.File); !ok {
		return
	}
	msg := ""
	switch req.Op {
	case "edit":
		kw, _, _ := preamble(lines)
		title := kw["TITLE"]
		if req.Title != nil && strings.TrimSpace(*req.Title) != "" {
			title = strings.TrimSpace(*req.Title)
			lines = setKeyword(lines, "TITLE", title)
		}
		if req.Icon != nil {
			lines = setKeyword(lines, "CAMPAIGN_ICON", strings.TrimSpace(*req.Icon))
		}
		if req.NoArt {
			lines = setKeyword(lines, "CAMPAIGN_ART", "")
		} else if req.Art != "" || req.ArtUrl != "" {
			art, code, err := keepCampaignArt(file, title, req.Art, req.ArtUrl)
			if err != nil {
				dndError(w, code, "%s", err)
				return
			}
			lines = setKeyword(lines, "CAMPAIGN_ART", art)
		}
		if req.Description != nil {
			lines = setDescription(lines, *req.Description)
		}
		msg = "Saved " + title
	case "area":
		name := strings.TrimSpace(req.Name)
		if name == "" || strings.ContainsAny(name, "\n") {
			dndError(w, http.StatusBadRequest, "an area needs a name")
			return
		}
		if _, _, _, found := findArea(lines, name, 0); found {
			dndError(w, http.StatusConflict, "there is already an area called %s", name)
			return
		}
		for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
			lines = lines[:len(lines)-1]
		}
		lines = append(lines, "", areaHeading("*", name))
		msg = "Added " + name
	case "rename":
		at, _, _, found := findArea(lines, req.Area, req.Line)
		name := strings.TrimSpace(req.Name)
		if !found || name == "" {
			dndError(w, http.StatusNotFound, "no area called %s", req.Area)
			return
		}
		m := cpHeadingRe.FindStringSubmatch(lines[at])
		_, tags := cpHeadingParts(m[2])
		h := m[1] + " " + name
		lines[at] = h + strings.Repeat(" ", max(2, 70-len([]rune(h)))) + ":" + strings.Join(tags, ":") + ":"
		msg = "Renamed it " + name
	case "delete":
		at, _, sub, found := findArea(lines, req.Area, req.Line)
		if !found {
			dndError(w, http.StatusNotFound, "no area called %s", req.Area)
			return
		}
		lines = append(lines[:at:at], lines[sub:]...)
		msg = "Took out " + req.Area
	case "order":
		// Each area's subtree, cut out and put back in the order asked, where
		// the first of them was.
		type block struct {
			name  string
			lines []string
		}
		blocks := []block{}
		first := -1
		for {
			found := false
			for i, l := range lines {
				m := cpHeadingRe.FindStringSubmatch(l)
				if m == nil {
					continue
				}
				title, tags := cpHeadingParts(m[2])
				if !cpHasTag(tags, "area") {
					continue
				}
				_, _, sub, _ := findArea(lines, title, i)
				if first < 0 || i < first {
					first = i
				}
				blocks = append(blocks, block{title, append([]string{}, lines[i:sub]...)})
				lines = append(lines[:i:i], lines[sub:]...)
				found = true
				break
			}
			if !found {
				break
			}
		}
		if first < 0 {
			break
		}
		rank := map[string]int{}
		for i, n := range req.Names {
			rank[n] = i + 1
		}
		sort.SliceStable(blocks, func(a, b int) bool {
			ra, rb := rank[blocks[a].name], rank[blocks[b].name]
			if ra == 0 {
				ra = len(req.Names) + 1
			}
			if rb == 0 {
				rb = len(req.Names) + 1
			}
			return ra < rb
		})
		first = min(first, len(lines))
		mid := []string{}
		for _, b := range blocks {
			body := b.lines
			for len(body) > 1 && strings.TrimSpace(body[len(body)-1]) == "" {
				body = body[:len(body)-1]
			}
			mid = append(mid, body...)
			mid = append(mid, "")
		}
		out := append([]string{}, lines[:first]...)
		out = append(out, mid...)
		lines = append(out, lines[first:]...)
		msg = "Put the areas in order"
	case "maps":
		at, own, _, found := findArea(lines, req.Area, req.Line)
		if !found {
			dndError(w, http.StatusNotFound, "no area called %s", req.Area)
			return
		}
		items := []string{}
		seen := map[string]bool{}
		for _, m := range req.Maps {
			p, ok := insideOrgDirs(strings.TrimSpace(m))
			if !ok || filepath.Ext(p) != ".org" || seen[p] {
				continue
			}
			seen[p] = true
			items = append(items, fmt.Sprintf("- [[file:%s][%s]]", relLink(file, p), strings.ReplaceAll(mapTitleOf(p), "]", ")")))
		}
		// The old list out; the new one where it began, else after the
		// heading's own drawers and planning.
		insert := -1
		body := []string{}
		for i := at + 1; i < own; i++ {
			if mapItemRe.MatchString(lines[i]) {
				if insert < 0 {
					insert = len(body)
				}
				continue
			}
			body = append(body, lines[i])
		}
		if insert < 0 {
			insert = 0
			for insert < len(body) {
				t := strings.TrimSpace(body[insert])
				if strings.HasPrefix(t, ":") || strings.HasPrefix(t, "SCHEDULED:") || strings.HasPrefix(t, "DEADLINE:") || strings.HasPrefix(t, "CLOSED:") {
					insert++
					continue
				}
				break
			}
			// After any text the area has, before blank lines at its end.
			end := len(body)
			for end > insert && strings.TrimSpace(body[end-1]) == "" {
				end--
			}
			insert = end
		}
		nb := append([]string{}, body[:insert]...)
		nb = append(nb, items...)
		nb = append(nb, body[insert:]...)
		out := append([]string{}, lines[:at+1]...)
		out = append(out, nb...)
		lines = append(out, lines[own:]...)
		msg = fmt.Sprintf("%s has %d map%s", req.Area, len(items), map[bool]string{true: "", false: "s"}[len(items) == 1])
	default:
		dndError(w, http.StatusBadRequest, "op is new, edit, area, rename, delete, order or maps")
		return
	}
	if err := writeLines(file, lines); err != nil {
		dndError(w, http.StatusInternalServerError, "%s", err)
		return
	}
	answerCampaign(w, file, msg)
}

func answerCampaign(w http.ResponseWriter, file, msg string) {
	c, _ := readCampaign(file, fileLinesFresh(file))
	dndJson(w, map[string]interface{}{"Ok": true, "Msg": msg, "File": file, "Campaign": c})
}
