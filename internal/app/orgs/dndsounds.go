package orgs

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// The DM screen's sound board: sounds from the sound folder (dndSoundPath, its
// subfolders categories, archives read as folders), and sounds found on the
// web - every service that lets a sound be played and downloaded without an
// account, searched at once - each kept in the sound folder (cache/) the
// moment the DM plays it, with where it came from and its licence.
//
// The services, and why each is used as it is:
//   - Freesound: its search page lists each sound's preview mp3 (128k), which
//     needs no account; the original file does (it redirects to a login).
//   - BBC Sound Effects: its search api and its mp3s need no account
//     (RemArc licence: personal, educational and research use).
//   - Tabletop Audio: its whole catalogue is one JSON (tta_data) with each
//     track's mp3, searched here by title, tags and flavour text.
//   - Openverse: sound effects from sources other than Freesound (Wikimedia
//     and the like), through its open api.
//   - Mixkit: its tag pages (free-sound-effects/<tag>/) list each sound's
//     preview mp3 on its asset store; the query's words are its tags. Its
//     pages also show paid Envato items, which are left out (their previews
//     are not on Mixkit's store).
//   - SoundBible: its search page has each sound's mp3, title, author and
//     licence (mostly Creative Commons Attribution 3.0, some public domain).
//   - Soundgator: its search (?keyword=) lists each sound's mp3; its name
//     is read off the file's.
// Not used, because they cannot be searched without an account or without
// driving a real browser: Pixabay (pages refuse anything not a browser, api
// wants an account's key), freesoundeffects.com and Sonniss (Cloudflare's
// bot check on every page; Sonniss's free GDC bundles are zips to download
// by hand into the sound folder, where they are read as folders).

// Sounds a browser plays as they are - the open formats (Ogg with Vorbis,
// Opus or FLAC in it, Opus, FLAC, WAV, WebM) and the closed ones it knows
// (mp3, AAC, m4a) - and those it does not, which are turned into Ogg Opus
// with ffmpeg once, when played, and kept in the cache (soundTranscode).
var audioExts = map[string]bool{
	".mp3": true, ".ogg": true, ".oga": true, ".opus": true, ".flac": true, ".wav": true, ".wave": true,
	".webm": true, ".weba": true, ".m4a": true, ".aac": true, ".mp4a": true,
	".aif": true, ".aiff": true, ".aifc": true, ".wma": true, ".spx": true, ".au": true, ".snd": true,
	".ape": true, ".amr": true, ".caf": true, ".mka": true, ".ac3": true, ".wv": true, ".tta": true,
}

var audioTranscode = map[string]bool{
	".aif": true, ".aiff": true, ".aifc": true, ".wma": true, ".spx": true, ".au": true, ".snd": true,
	".ape": true, ".amr": true, ".caf": true, ".mka": true, ".ac3": true, ".wv": true, ".tta": true,
}

const soundPrefix = "@sounds/"
const soundMaxBytes = 80 << 20

type soundItem struct {
	Path     string  `json:"path,omitempty"` // in the sound folder: @sounds/...
	Name     string  `json:"name"`
	Category string  `json:"category,omitempty"`
	Source   string  `json:"source,omitempty"` // a web sound's service
	Url      string  `json:"url,omitempty"`    // a web sound's audio
	Page     string  `json:"page,omitempty"`   // and the page it is on
	License  string  `json:"license,omitempty"`
	Author   string  `json:"author,omitempty"`
	Duration float64 `json:"duration,omitempty"` // seconds
	Cached   string  `json:"cached,omitempty"`   // a web sound already kept: its path
}

// The sound folder: dndSoundPath, else sounds/ in the shared folder.
func soundDir() (string, bool) {
	if Conf().Server == nil {
		return "", false
	}
	p := strings.TrimSpace(Conf().Server.DndSoundPath)
	if p == "" {
		if shared, ok := sharedDir(); ok {
			return filepath.Join(shared, "sounds"), true
		}
		return "", false
	}
	if strings.HasPrefix(p, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(p, "~"), "/"))
		}
	}
	if !filepath.IsAbs(p) && len(Conf().Server.OrgDirs) > 0 {
		p = filepath.Join(Conf().Server.OrgDirs[0], p)
	}
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	return p, true
}

// What the sound folder has kept from the web, by the audio's url.
type soundCache struct {
	Sounds map[string]soundItem `json:"sounds"`
}

var soundCacheMu sync.Mutex

func readSoundCache(dir string) soundCache {
	var c soundCache
	if b, err := os.ReadFile(filepath.Join(dir, "cache", "sounds.json")); err == nil {
		json.Unmarshal(b, &c)
	}
	if c.Sounds == nil {
		c.Sounds = map[string]soundItem{}
	}
	return c
}

/* SDOC: API
* GET /dnd/sounds — The Sound Folder
	Every sound in the sound folder (=dndSoundPath=): its subfolders are categories, and a zip,
	7z, rar or tar in it is read as a folder. Fetch one with =GET /dnd/sounds/file=.

	*Method:* =GET=

	*Response:* ={Ok, Dir, Sounds: [{path, name, category, source, page, license}], Categories}=;
	no folder set is =Ok= false.
	EDOC */
func RequestDndSounds(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	dir, ok := soundDir()
	if !ok {
		dndJson(w, map[string]interface{}{"Ok": false, "Msg": "no sound folder: set dndSoundPath (or dndSharedPath) in orgs.yaml", "Sounds": []soundItem{}, "Categories": []string{}})
		return
	}
	cache := readSoundCache(dir)
	byPath := map[string]soundItem{}
	for _, s := range cache.Sounds {
		byPath[s.Path] = s
	}
	cats := map[string]bool{}
	out := []soundItem{}
	filepath.WalkDir(dir, func(p string, e os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if e.IsDir() {
			if strings.HasPrefix(e.Name(), ".") && p != dir {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		rel = filepath.ToSlash(rel)
		if k, stem := archiveKind(e.Name()); k != "" {
			a, err := openArchive(p)
			if err != nil {
				return nil
			}
			for _, en := range a.entries {
				if !audioExts[strings.ToLower(path.Ext(en))] {
					continue
				}
				cat := path.Base(stem)
				if d := strings.Split(path.Dir(en), "/"); len(d) > 0 && d[0] != "." {
					cat = d[len(d)-1]
				}
				cats[cat] = true
				out = append(out, soundItem{Path: soundPrefix + rel + "/" + en, Name: soundName(path.Base(en)), Category: cat})
			}
			return nil
		}
		if !audioExts[strings.ToLower(filepath.Ext(p))] {
			return nil
		}
		cat := ""
		if parts := strings.Split(rel, "/"); len(parts) > 1 {
			cat = parts[0]
			if cat == "cache" && len(parts) > 2 {
				cat = parts[1]
			}
		}
		if cat != "" {
			cats[cat] = true
		}
		it := soundItem{Path: soundPrefix + rel, Name: soundName(e.Name()), Category: cat}
		if c, ok := byPath[it.Path]; ok {
			it.Source, it.Page, it.License, it.Author, it.Url, it.Duration = c.Source, c.Page, c.License, c.Author, c.Url, c.Duration
		}
		out = append(out, it)
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	list := []string{}
	for c := range cats {
		list = append(list, c)
	}
	sort.Strings(list)
	dndJson(w, map[string]interface{}{"Ok": true, "Dir": dir, "Sounds": out, "Categories": list})
}

// A file's name as a sound's: no extension, words for underscores.
func soundName(file string) string {
	n := strings.TrimSuffix(file, path.Ext(file))
	n = strings.NewReplacer("_", " ", "-", " ").Replace(n)
	return strings.Join(strings.Fields(n), " ")
}

/* SDOC: API
* GET /dnd/sounds/file — A Sound
	A sound from the sound folder by its =path= (=@sounds/...=, as =GET /dnd/sounds= gives it),
	out of an archive when its path runs through one.

	*Method:* =GET=

	*Response:* the sound.
	EDOC */
func RequestDndSoundFile(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	dir, ok := soundDir()
	rel := r.URL.Query().Get("path")
	if !ok || !strings.HasPrefix(rel, soundPrefix) || strings.Contains(rel, "..") {
		dndError(w, http.StatusBadRequest, "not a sound in the sound folder")
		return
	}
	p := filepath.Clean(filepath.Join(dir, filepath.FromSlash(strings.TrimPrefix(rel, soundPrefix))))
	if in, err := filepath.Rel(dir, p); err != nil || strings.HasPrefix(in, "..") {
		dndError(w, http.StatusBadRequest, "not a sound in the sound folder")
		return
	}
	ext := strings.ToLower(filepath.Ext(p))
	if !audioExts[ext] {
		dndError(w, http.StatusNotFound, "no sound at %s", rel)
		return
	}
	if audioTranscode[ext] {
		if out, err := soundTranscode(p); err == nil {
			w.Header().Set("Cache-Control", "private, max-age=86400")
			w.Header().Set("Content-Type", "audio/ogg")
			http.ServeFile(w, r, out)
			return
		} else {
			fmt.Fprintf(os.Stderr, "dnd: %s plays as it is: %s\n", filepath.Base(p), err)
		}
	}
	if !serveShow(w, r, p) {
		dndError(w, http.StatusNotFound, "no sound at %s", rel)
	}
}

// soundTranscode turns a sound a browser cannot play (AIFF, WMA, Speex...)
// into Ogg Opus with ffmpeg, once: kept in the user's cache under a name made
// of the file's path, size and time. A sound in an archive is taken out of
// it first.
func soundTranscode(p string) (string, error) {
	ff, err := exec.LookPath("ffmpeg")
	if err != nil {
		return "", fmt.Errorf("no ffmpeg to turn it into something a browser plays")
	}
	var data []byte
	var size int64
	var mod time.Time
	if st, err := os.Stat(p); err == nil && !st.IsDir() {
		size, mod = st.Size(), st.ModTime()
	} else if ap, entry, ok := splitArchive(p); ok {
		a, err := openArchive(ap)
		if err != nil {
			return "", err
		}
		if data, mod, err = a.open(entry); err != nil {
			return "", err
		}
		size = int64(len(data))
	} else {
		return "", os.ErrNotExist
	}
	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}
	sum := sha1.Sum([]byte(fmt.Sprintf("%s|%d|%d", p, size, mod.UnixNano())))
	out := filepath.Join(base, "orgs", "sounds", hex.EncodeToString(sum[:10])+".ogg")
	if _, err := os.Stat(out); err == nil {
		return out, nil
	}
	if err := os.MkdirAll(filepath.Dir(out), 0755); err != nil {
		return "", err
	}
	in := p
	if data != nil {
		tmp, err := os.CreateTemp("", "orgs-sound-*"+filepath.Ext(p))
		if err != nil {
			return "", err
		}
		defer os.Remove(tmp.Name())
		tmp.Write(data)
		tmp.Close()
		in = tmp.Name()
	}
	part := out + ".part.ogg"
	cmd := exec.Command(ff, "-v", "error", "-y", "-i", in, "-vn", "-c:a", "libopus", "-b:a", "128k", part)
	if msg, err := cmd.CombinedOutput(); err != nil {
		os.Remove(part)
		return "", fmt.Errorf("ffmpeg: %s %s", err, strings.TrimSpace(string(msg)))
	}
	return out, os.Rename(part, out)
}

// ---- the web ------------------------------------------------------------------

var soundClient = &http.Client{Timeout: 9 * time.Second}

func soundGet(u string, body []byte) ([]byte, error) {
	method := "GET"
	var rd io.Reader
	if body != nil {
		method, rd = "POST", bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, u, rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := soundClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("%s answered %s", req.URL.Host, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 8<<20))
}

// A title as a name: no file ending on it (Freesound's often keep one).
var fileEnding = regexp.MustCompile(`(?i)\.(wav|wave|mp3|aiff?|flac|ogg|m4a)$`)

func cleanTitle(t string) string { return strings.TrimSpace(fileEnding.ReplaceAllString(strings.TrimSpace(t), "")) }

var (
	fsResult  = regexp.MustCompile(`(?s)aria-label="Sound (.*?) by ([^"]*)".*?data-sound-id="(\d+)".*?data-username="([^"]*)".*?data-mp3="([^"]+)"(.*?)(?:bw-search__result|$)`)
	fsDur     = regexp.MustCompile(`data-duration="([\d.]+)"`)
	fsLicense = regexp.MustCompile(`title="License: ([^"]+)"`)
)

// Freesound, from its search page: each sound's high quality preview.
func searchFreesound(q string) ([]soundItem, error) {
	b, err := soundGet("https://freesound.org/search/?q="+url.QueryEscape(q), nil)
	if err != nil {
		return nil, err
	}
	out := []soundItem{}
	for _, part := range strings.Split(string(b), "bw-search__result")[1:] {
		m := fsResult.FindStringSubmatch(part)
		if m == nil {
			continue
		}
		it := soundItem{
			Source: "Freesound", Name: cleanTitle(html.UnescapeString(m[1])), Author: html.UnescapeString(m[2]),
			Url:  strings.Replace(m[5], "-lq.mp3", "-hq.mp3", 1),
			Page: "https://freesound.org/people/" + url.PathEscape(html.UnescapeString(m[4])) + "/sounds/" + m[3] + "/",
		}
		if d := fsDur.FindStringSubmatch(part); d != nil {
			it.Duration, _ = strconv.ParseFloat(d[1], 64)
		}
		if l := fsLicense.FindStringSubmatch(part); l != nil {
			it.License = l[1]
		}
		out = append(out, it)
	}
	return out, nil
}

// BBC Sound Effects, from its search api.
func searchBBC(q string) ([]soundItem, error) {
	body, _ := json.Marshal(map[string]interface{}{"criteria": map[string]interface{}{
		"from": 0, "size": 20, "tags": nil, "categories": nil, "durations": nil, "continents": nil, "sortBy": nil, "source": nil, "recent": false, "query": q,
	}})
	b, err := soundGet("https://sound-effects-api.bbcrewind.co.uk/api/sfx/search", body)
	if err != nil {
		return nil, err
	}
	var r struct {
		Results []struct {
			Id          string  `json:"id"`
			Description string  `json:"description"`
			Duration    float64 `json:"duration"`
		} `json:"results"`
	}
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, err
	}
	out := []soundItem{}
	for _, x := range r.Results {
		out = append(out, soundItem{
			Source: "BBC", Name: x.Description, Duration: x.Duration / 1000,
			Url: "https://sound-effects-media.bbcrewind.co.uk/mp3/" + x.Id + ".mp3", Page: "https://sound-effects.bbcrewind.co.uk/search?q=" + x.Id,
			License: "BBC RemArc: personal, educational or research use",
		})
	}
	return out, nil
}

// Tabletop Audio's catalogue, read once every twelve hours.
var tta = struct {
	sync.Mutex
	at     time.Time
	tracks []struct {
		Title  string   `json:"track_title"`
		Type   string   `json:"track_type"`
		Flavor string   `json:"flavor_text"`
		Link   string   `json:"link"`
		Tags   []string `json:"tags"`
		Genre  []string `json:"track_genre"`
	}
}{}

func searchTabletopAudio(q string) ([]soundItem, error) {
	tta.Lock()
	if time.Since(tta.at) > 12*time.Hour {
		b, err := soundGet("https://tabletopaudio.com/tta_data", nil)
		if err != nil {
			tta.Unlock()
			return nil, err
		}
		var d struct {
			Tracks json.RawMessage `json:"tracks"`
		}
		if err := json.Unmarshal(b, &d); err == nil {
			json.Unmarshal(d.Tracks, &tta.tracks)
			tta.at = time.Now()
		}
	}
	tracks := tta.tracks
	tta.Unlock()
	words := strings.Fields(strings.ToLower(q))
	type hit struct {
		it    soundItem
		score int
	}
	hits := []hit{}
	for _, t := range tracks {
		title := strings.ToLower(t.Title)
		hay := strings.ToLower(t.Title + " " + t.Type + " " + t.Flavor + " " + strings.Join(t.Tags, " ") + " " + strings.Join(t.Genre, " "))
		score := 0
		for _, w := range words {
			switch {
			case strings.Contains(title, w):
				score += 3
			case strings.Contains(hay, w):
				score++
			default:
				score = -1000
			}
		}
		if score > 0 {
			hits = append(hits, hit{soundItem{Source: "Tabletop Audio", Name: t.Title + " (" + t.Type + ")", Url: t.Link, Page: "https://tabletopaudio.com/",
				License: "Tabletop Audio: free for personal use", Author: "Tabletop Audio"}, score})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].score > hits[j].score })
	out := []soundItem{}
	for i, h := range hits {
		if i >= 15 {
			break
		}
		out = append(out, h.it)
	}
	return out, nil
}

var (
	mixPreview = regexp.MustCompile(`data-audio-player-preview-url-value="(https://assets\.mixkit\.co/active_storage/sfx/(\d+)/[^"]+\.mp3)"`)
	mixTitle   = regexp.MustCompile(`(?s)item-grid-card__title[^>]*>\s*([^<]+?)\s*<`)
)

// Mixkit, by tag: the whole query as one tag, then each word.
func searchMixkit(q string) ([]soundItem, error) {
	words := strings.Fields(strings.ToLower(regexp.MustCompile(`[^\p{L}\p{N} ]+`).ReplaceAllString(q, " ")))
	if len(words) == 0 {
		return nil, nil
	}
	tags := []string{strings.Join(words, "-")}
	if len(words) > 1 {
		tags = append(tags, words...)
	}
	if len(tags) > 4 {
		tags = tags[:4]
	}
	pages := make([]string, len(tags))
	var wg sync.WaitGroup
	for i, t := range tags {
		wg.Add(1)
		go func(i int, t string) {
			defer wg.Done()
			if b, err := soundGet("https://mixkit.co/free-sound-effects/"+url.PathEscape(t)+"/", nil); err == nil {
				pages[i] = string(b)
			}
		}(i, t)
	}
	wg.Wait()
	seen := map[string]bool{}
	out := []soundItem{}
	for i, h := range pages {
		for _, m := range mixPreview.FindAllStringSubmatchIndex(h, -1) {
			u, id := h[m[2]:m[3]], h[m[4]:m[5]]
			if seen[id] {
				continue
			}
			seen[id] = true
			// Its title is the first one written after its player.
			name := ""
			to := m[1] + 3000
			if to > len(h) {
				to = len(h)
			}
			if t := mixTitle.FindStringSubmatch(h[m[1]:to]); t != nil {
				name = html.UnescapeString(strings.TrimSpace(t[1]))
			}
			if name == "" {
				name = "Mixkit " + tags[i] + " " + id
			}
			out = append(out, soundItem{Source: "Mixkit", Name: name, Url: u, Page: "https://mixkit.co/free-sound-effects/" + tags[i] + "/",
				License: "Mixkit Sound Effects Free License"})
			if len(out) >= 18 {
				return out, nil
			}
		}
	}
	return out, nil
}

var (
	sbItem    = regexp.MustCompile(`data-source="(mp3/[^"]+\.mp3)"`)
	sbTitle   = regexp.MustCompile(`(?s)<h3[^>]*>\s*<a href='([^']+)'[^>]*>([^<]+)</a>`)
	sbLicense = regexp.MustCompile(`(Attribution[^<\n]{0,20}|Public Domain|Sampling Plus[^<\n]{0,8}|Noncommercial[^<\n]{0,20})`)
	sbAuthor  = regexp.MustCompile(`(?s)</p>\s*(?:<[^>]+>\s*)*([A-Z][^<&\n]{1,40}?)\s*(?:<[^>]+>\s*)*&nbsp;\d`)
)

// SoundBible, from its search page.
func searchSoundBible(q string) ([]soundItem, error) {
	b, err := soundGet("https://soundbible.com/search.php?q="+url.QueryEscape(q), nil)
	if err != nil {
		return nil, err
	}
	h := string(b)
	out := []soundItem{}
	// Its search answers loosely (a rainforest for "door"): only sounds whose
	// title or description says one of the words are taken.
	words := []string{}
	for _, w := range strings.Fields(strings.ToLower(q)) {
		if len(w) > 2 {
			words = append(words, strings.TrimSuffix(w, "s"))
		}
	}
	tags := regexp.MustCompile(`<[^>]+>`)
	all := sbItem.FindAllStringSubmatchIndex(h, -1)
	for k, m := range all {
		mp3 := h[m[2]:m[3]]
		to := m[1] + 4000
		if k+1 < len(all) && all[k+1][0] < to {
			to = all[k+1][0]
		}
		if to > len(h) {
			to = len(h)
		}
		seg := h[m[1]:to]
		if len(words) > 0 {
			text := strings.ToLower(tags.ReplaceAllString(seg, " ") + " " + mp3)
			hit := false
			for _, w := range words {
				if strings.Contains(text, w) {
					hit = true
					break
				}
			}
			if !hit {
				continue
			}
		}
		it := soundItem{Source: "SoundBible", Url: "https://soundbible.com/" + strings.ReplaceAll(mp3, " ", "%20")}
		if t := sbTitle.FindStringSubmatch(seg); t != nil {
			it.Name = html.UnescapeString(strings.TrimSpace(t[2]))
			it.Page = "https://soundbible.com/" + t[1]
		} else {
			it.Name = soundName(path.Base(mp3))
		}
		if l := sbLicense.FindStringSubmatch(seg); l != nil {
			it.License = strings.TrimSpace(l[1])
		}
		if a := sbAuthor.FindStringSubmatch(seg); a != nil {
			it.Author = strings.TrimSpace(a[1])
		}
		out = append(out, it)
		if len(out) >= 18 {
			break
		}
	}
	return out, nil
}

var sgItem = regexp.MustCompile(`src="(https://www\.soundgator\.com/uploads/data/\d+/(\d+)_([^"]+)\.mp3)"`)

// Soundgator, from its search: each sound's mp3, named by its file.
func searchSoundgator(q string) ([]soundItem, error) {
	b, err := soundGet("https://www.soundgator.com/search/?keyword="+url.QueryEscape(q), nil)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	out := []soundItem{}
	for _, m := range sgItem.FindAllStringSubmatch(string(b), -1) {
		if seen[m[2]] {
			continue
		}
		seen[m[2]] = true
		name := soundName(m[3])
		if name != "" {
			name = strings.ToUpper(name[:1]) + name[1:]
		}
		out = append(out, soundItem{Source: "Soundgator", Name: name, Url: m[1], Page: "https://www.soundgator.com/search/?keyword=" + url.QueryEscape(q),
			License: "SoundGator: free to use (see its licence)"})
		if len(out) >= 18 {
			break
		}
	}
	return out, nil
}

// Openverse's sound effects, but Freesound's (searched on their own).
func searchOpenverse(q string) ([]soundItem, error) {
	b, err := soundGet("https://api.openverse.org/v1/audio/?page_size=20&category=sound_effect&q="+url.QueryEscape(q), nil)
	if err != nil {
		return nil, err
	}
	var r struct {
		Results []struct {
			Title    string `json:"title"`
			Url      string `json:"url"`
			Page     string `json:"foreign_landing_url"`
			Creator  string `json:"creator"`
			License  string `json:"license"`
			Version  string `json:"license_version"`
			Source   string `json:"source"`
			Duration int    `json:"duration"`
		} `json:"results"`
	}
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, err
	}
	out := []soundItem{}
	for _, x := range r.Results {
		if x.Source == "freesound" || x.Url == "" {
			continue
		}
		out = append(out, soundItem{Source: "Openverse (" + strings.ReplaceAll(x.Source, "_", " ") + ")", Name: cleanTitle(x.Title), Url: x.Url, Page: x.Page,
			Author: x.Creator, License: strings.ToUpper(x.License) + " " + x.Version, Duration: float64(x.Duration) / 1000})
	}
	return out, nil
}

/* SDOC: API
* GET /dnd/sounds/search — Search the Web for Sounds
	Searches every sound service that lets a sound be played and downloaded without an account
	at once - Freesound (previews), BBC Sound Effects, Tabletop Audio, Mixkit, SoundBible, Soundgator and
	Openverse - and answers
	what each found, a service that fails or is slow (nine seconds) left out with its reason.

	*Method:* =GET=

	*Query Parameters:* =q=, what to look for.

	*Response:* ={Ok, Sounds: [{source, name, url, page, license, author, duration, cached}], Failed: {service: why}}=
	EDOC */
func RequestDndSoundSearch(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		dndJson(w, map[string]interface{}{"Ok": true, "Sounds": []soundItem{}})
		return
	}
	services := []struct {
		name string
		fn   func(string) ([]soundItem, error)
	}{{"Freesound", searchFreesound}, {"BBC", searchBBC}, {"Tabletop Audio", searchTabletopAudio}, {"Mixkit", searchMixkit}, {"SoundBible", searchSoundBible}, {"Soundgator", searchSoundgator}, {"Openverse", searchOpenverse}}
	results := make([][]soundItem, len(services))
	failed := map[string]string{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i, s := range services {
		wg.Add(1)
		go func(i int, name string, fn func(string) ([]soundItem, error)) {
			defer wg.Done()
			got, err := fn(q)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failed[name] = err.Error()
				return
			}
			results[i] = got
		}(i, s.name, s.fn)
	}
	wg.Wait()
	cache := soundCache{Sounds: map[string]soundItem{}}
	if dir, ok := soundDir(); ok {
		cache = readSoundCache(dir)
	}
	// Taken a few from each in turn, so no one service crowds the rest out.
	out := []soundItem{}
	for round := 0; ; round++ {
		any := false
		for _, list := range results {
			for k := round * 3; k < len(list) && k < round*3+3; k++ {
				it := list[k]
				if c, ok := cache.Sounds[it.Url]; ok {
					it.Cached = c.Path
				}
				out = append(out, it)
				any = true
			}
		}
		if !any {
			break
		}
	}
	dndJson(w, map[string]interface{}{"Ok": true, "Sounds": out, "Failed": failed})
}

/* SDOC: API
* POST /dnd/sounds/cache — Keep a Sound from the Web
	Downloads a web sound into the sound folder's =cache/<service>/=, and notes where it came from,
	its licence and its author in =cache/sounds.json=. A sound kept already is not fetched again.

	*Method:* =POST=

	*Request Body:* ={source, name, url, page, license, author, duration}=, a sound as
	=GET /dnd/sounds/search= gives it.

	*Response:* ={Ok, Msg, Path}=, the sound's path in the sound folder; or, for a sound still
	downloading after fifteen seconds (a long recording), ={Ok, Pending: true}=: it goes on in the
	background, and asking again answers its path once it is kept.
	EDOC */
func PostDndSoundCache(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	var it soundItem
	if !dndBody(w, r, &it) {
		return
	}
	dir, ok := soundDir()
	if !ok {
		dndError(w, http.StatusBadRequest, "no sound folder: set dndSoundPath (or dndSharedPath) in orgs.yaml")
		return
	}
	u, err := url.Parse(it.Url)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		dndError(w, http.StatusBadRequest, "%q is not a web address", it.Url)
		return
	}
	soundCacheMu.Lock()
	cache := readSoundCache(dir)
	if c, ok := cache.Sounds[it.Url]; ok {
		if p, _, ok := resolveSound(dir, c.Path); ok {
			if _, err := os.Stat(p); err == nil {
				soundCacheMu.Unlock()
				dndJson(w, map[string]interface{}{"Ok": true, "Msg": "Kept already", "Path": c.Path})
				return
			}
		}
	}
	job := soundJobs[it.Url]
	if job == nil {
		job = &soundJob{done: make(chan struct{})}
		soundJobs[it.Url] = job
		go keepSound(dir, it, job)
	}
	soundCacheMu.Unlock()
	// A short sound is kept before the answer; a long one (Freesound's
	// CDN gives a long recording out slowly) goes on in the background, and
	// asking again says how it went.
	select {
	case <-job.done:
		if job.err != nil {
			dndError(w, http.StatusBadGateway, "%s", job.err)
			return
		}
		dndJson(w, map[string]interface{}{"Ok": true, "Msg": "Kept " + path.Base(job.path), "Path": job.path})
	case <-time.After(15 * time.Second):
		dndJson(w, map[string]interface{}{"Ok": true, "Pending": true, "Msg": "Still keeping it: a long one"})
	}
}

// A sound being kept: done when it is, with its path or why not.
type soundJob struct {
	done chan struct{}
	path string
	err  error
}

var soundJobs = map[string]*soundJob{}

// keepSound downloads a web sound into the sound folder, written to a
// .part file and moved into place whole, then noted in the cache's index.
func keepSound(dir string, it soundItem, job *soundJob) {
	defer func() {
		soundCacheMu.Lock()
		delete(soundJobs, it.Url)
		soundCacheMu.Unlock()
		close(job.done)
	}()
	u, _ := url.Parse(it.Url)
	req, _ := http.NewRequest("GET", it.Url, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36")
	resp, err := (&http.Client{Timeout: 45 * time.Minute}).Do(req)
	if err != nil {
		job.err = fmt.Errorf("could not fetch it: %s", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		job.err = fmt.Errorf("%s answered %s", u.Host, resp.Status)
		return
	}
	// Its kind: by the address, else by what the server said.
	ext := strings.ToLower(path.Ext(u.Path))
	if ct := resp.Header.Get("Content-Type"); !audioExts[ext] && ct != "" {
		if strings.HasPrefix(ct, "text/") {
			job.err = fmt.Errorf("that address is a page, not a sound")
			return
		}
		if exts, _ := mime.ExtensionsByType(strings.Split(ct, ";")[0]); len(exts) > 0 && audioExts[exts[0]] {
			ext = exts[0]
		}
	}
	if !audioExts[ext] {
		ext = ".mp3"
	}
	service := safeShowName(it.Source)
	if service == "" {
		service = "web"
	}
	folder := filepath.Join(dir, "cache", service)
	if err := os.MkdirAll(folder, 0755); err != nil {
		job.err = err
		return
	}
	stem := safeShowName(it.Name)
	if len([]rune(stem)) > 70 {
		stem = strings.TrimSpace(string([]rune(stem)[:70]))
	}
	if stem == "" {
		stem = "sound " + time.Now().Format("2006-01-02 150405")
	}
	p := filepath.Join(folder, stem+ext)
	for i := 2; ; i++ {
		if _, err := os.Stat(p); os.IsNotExist(err) {
			break
		}
		p = filepath.Join(folder, fmt.Sprintf("%s %d%s", stem, i, ext))
	}
	part := p + ".part"
	f, err := os.Create(part)
	if err != nil {
		job.err = err
		return
	}
	n, err := io.Copy(f, io.LimitReader(resp.Body, soundMaxBytes+1))
	f.Close()
	if err != nil || n > soundMaxBytes {
		os.Remove(part)
		if err == nil {
			err = fmt.Errorf("it is over %d MB", soundMaxBytes>>20)
		}
		job.err = fmt.Errorf("could not fetch it: %s", err)
		return
	}
	if err := os.Rename(part, p); err != nil {
		job.err = err
		return
	}
	rel, _ := filepath.Rel(dir, p)
	it.Path = soundPrefix + filepath.ToSlash(rel)
	it.Cached = ""
	soundCacheMu.Lock()
	cache := readSoundCache(dir)
	cache.Sounds[it.Url] = it
	if out, err := json.MarshalIndent(cache, "", "  "); err == nil {
		os.WriteFile(filepath.Join(dir, "cache", "sounds.json"), out, 0644)
	}
	soundCacheMu.Unlock()
	job.path = it.Path
}

// A sound's file by its @sounds/ path.
func resolveSound(dir, rel string) (string, string, bool) {
	if !strings.HasPrefix(rel, soundPrefix) || strings.Contains(rel, "..") {
		return "", "", false
	}
	p := filepath.Clean(filepath.Join(dir, filepath.FromSlash(strings.TrimPrefix(rel, soundPrefix))))
	if in, err := filepath.Rel(dir, p); err != nil || strings.HasPrefix(in, "..") {
		return "", "", false
	}
	return p, dir, true
}
