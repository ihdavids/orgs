package orgs

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// The DM screen's music: tracks from the music folder (dndMusicPath, its
// subfolders albums, archives read as folders), played in the browser, and,
// on a Mac with the Spotify app, Spotify's - played in the app, so it sounds
// wherever the app does, steered here by AppleScript. Searching Spotify and
// listing the DM's playlists goes through Spotify's Web API, signed in once
// with PKCE (spotifyClientId; no secret is kept).

const musicPrefix = "@music/"

// The music folder: dndMusicPath, else music/ in the shared folder.
func musicDir() (string, bool) {
	if Conf().Server == nil {
		return "", false
	}
	p := strings.TrimSpace(Conf().Server.DndMusicPath)
	if p == "" {
		if shared, ok := sharedDir(); ok {
			return filepath.Join(shared, "music"), true
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

type musicTrack struct {
	Path  string `json:"path"`
	Name  string `json:"name"`
	Album string `json:"album,omitempty"` // the folder it is in, inside the music folder
}

/* SDOC: API
* GET /dnd/music — The Music Folder
	Every track in the music folder (=dndMusicPath=), with the folder each is in as its album (a
	zip, 7z, rar or tar is a folder of its own), sorted by album and name. Fetch one with
	=GET /dnd/music/file=.

	*Method:* =GET=

	*Response:* ={Ok, Msg, Dir, Tracks: [{path, name, album}], Albums: [{name, count}]}=
	EDOC */
func RequestDndMusic(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	dir, ok := musicDir()
	if !ok {
		dndJson(w, map[string]interface{}{"Ok": false, "Msg": "no music folder: set dndMusicPath (or dndSharedPath) in orgs.yaml", "Tracks": []musicTrack{}, "Albums": []interface{}{}})
		return
	}
	out := []musicTrack{}
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
				album := path.Dir(rel)
				if album == "." {
					album = ""
				}
				inner := path.Dir(en)
				name := path.Base(stem)
				if inner != "." {
					name = path.Base(inner)
				}
				if album != "" {
					name = album + "/" + name
				}
				out = append(out, musicTrack{Path: musicPrefix + rel + "/" + en, Name: soundName(path.Base(en)), Album: name})
			}
			return nil
		}
		if !audioExts[strings.ToLower(filepath.Ext(p))] {
			return nil
		}
		album := path.Dir(rel)
		if album == "." {
			album = ""
		}
		out = append(out, musicTrack{Path: musicPrefix + rel, Name: soundName(e.Name()), Album: album})
		return nil
	})
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Album != out[j].Album {
			return out[i].Album < out[j].Album
		}
		return strings.ToLower(out[i].Path) < strings.ToLower(out[j].Path)
	})
	counts := map[string]int{}
	order := []string{}
	for _, t := range out {
		if t.Album == "" {
			continue
		}
		if counts[t.Album] == 0 {
			order = append(order, t.Album)
		}
		counts[t.Album]++
	}
	albums := []map[string]interface{}{}
	for _, a := range order {
		albums = append(albums, map[string]interface{}{"name": a, "count": counts[a]})
	}
	dndJson(w, map[string]interface{}{"Ok": true, "Dir": dir, "Tracks": out, "Albums": albums})
}

/* SDOC: API
* GET /dnd/music/file — A Track
	A track from the music folder by its =path= (=@music/...=), out of an archive when its path runs
	through one, turned into Ogg Opus first when a browser could not play it.

	*Method:* =GET=

	*Response:* the track.
	EDOC */
func RequestDndMusicFile(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	dir, ok := musicDir()
	rel := r.URL.Query().Get("path")
	if !ok || !strings.HasPrefix(rel, musicPrefix) || strings.Contains(rel, "..") {
		dndError(w, http.StatusBadRequest, "not a track in the music folder")
		return
	}
	p := filepath.Clean(filepath.Join(dir, filepath.FromSlash(strings.TrimPrefix(rel, musicPrefix))))
	if in, err := filepath.Rel(dir, p); err != nil || strings.HasPrefix(in, "..") {
		dndError(w, http.StatusBadRequest, "not a track in the music folder")
		return
	}
	ext := strings.ToLower(filepath.Ext(p))
	if !audioExts[ext] {
		dndError(w, http.StatusNotFound, "no track at %s", rel)
		return
	}
	if audioTranscode[ext] {
		if out, err := soundTranscode(p); err == nil {
			w.Header().Set("Content-Type", "audio/ogg")
			http.ServeFile(w, r, out)
			return
		}
	}
	if !serveShow(w, r, p) {
		dndError(w, http.StatusNotFound, "no track at %s", rel)
	}
}

// ---- the Spotify app ---------------------------------------------------------

// Whether the Spotify app is on this machine (a Mac: it is steered with
// AppleScript).
func spotifyApp() bool {
	if runtime.GOOS != "darwin" {
		return false
	}
	for _, p := range []string{"/Applications/Spotify.app", filepath.Join(os.Getenv("HOME"), "Applications/Spotify.app")} {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	return false
}

func osa(script string) (string, error) {
	out, err := exec.Command("osascript", "-e", script).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s %s", err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

type spotifyState struct {
	Running  bool    `json:"running"`
	Playing  bool    `json:"playing"`
	Track    string  `json:"track,omitempty"`
	Artist   string  `json:"artist,omitempty"`
	Album    string  `json:"album,omitempty"`
	Uri      string  `json:"uri,omitempty"`
	Duration float64 `json:"duration,omitempty"` // seconds
	Position float64 `json:"position,omitempty"`
	Volume   int     `json:"volume,omitempty"` // 0..100
}

// What the app is playing; nothing (and the app left alone) when it is not
// running.
func spotifyNow() spotifyState {
	if !spotifyApp() {
		return spotifyState{}
	}
	out, err := osa(`if application "Spotify" is running then
	tell application "Spotify"
		set s to (player state as string)
		try
			set t to current track
			return s & "\t" & (name of t) & "\t" & (artist of t) & "\t" & (album of t) & "\t" & (duration of t) & "\t" & (player position) & "\t" & (sound volume) & "\t" & (spotify url of t)
		on error
			return s & "\t\t\t\t0\t0\t" & (sound volume) & "\t"
		end try
	end tell
else
	return "off"
end if`)
	if err != nil || out == "off" {
		return spotifyState{}
	}
	f := strings.Split(out, "\t")
	for len(f) < 8 {
		f = append(f, "")
	}
	st := spotifyState{Running: true, Playing: f[0] == "playing", Track: f[1], Artist: f[2], Album: f[3], Uri: f[7]}
	if d, err := strconv.ParseFloat(strings.ReplaceAll(f[4], ",", "."), 64); err == nil {
		st.Duration = d / 1000
	}
	st.Position, _ = strconv.ParseFloat(strings.ReplaceAll(f[5], ",", "."), 64)
	st.Volume, _ = strconv.Atoi(f[6])
	return st
}

// A Spotify uri, or a link to one turned into one; anything else refused.
func spotifyUri(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "https://open.spotify.com/") {
		parts := strings.Split(strings.Split(strings.TrimPrefix(s, "https://open.spotify.com/"), "?")[0], "/")
		if len(parts) >= 2 {
			s = "spotify:" + parts[len(parts)-2] + ":" + parts[len(parts)-1]
		}
	}
	ok := strings.HasPrefix(s, "spotify:") && !strings.ContainsAny(s, "\"\\\n")
	return s, ok
}

/* SDOC: API
* GET /dnd/music/spotify — Spotify, as the DM Screen Sees It
	Whether the Spotify app is on this machine and playing (and what), whether Spotify's Web API
	is set up (=spotifyClientId=) and signed in, and the redirect URI to give Spotify's app.

	*Method:* =GET=

	*Response:* ={Ok, App, Configured, Connected, Redirect, State: {running, playing, track, artist, album, uri, duration, position, volume}}=
	EDOC */
func RequestDndSpotify(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	_, connected := spotifyToken()
	dndJson(w, map[string]interface{}{
		"Ok": true, "App": spotifyApp(), "Configured": spotifyClientId() != "", "Connected": connected,
		"Redirect": spotifyRedirect(), "State": spotifyNow(),
	})
}

/* SDOC: API
* POST /dnd/music/spotify/play — Play Something on Spotify
	Plays a Spotify track, album or playlist (its uri, or its open.spotify.com link) in the Spotify app,
	starting the app if it is not running. Without the app (not a Mac), through the Web API on the
	DM's active device, which Spotify allows only with Premium.

	*Method:* =POST=

	*Request Body:* ={uri}=

	*Response:* ={Ok, Msg, State}=
	EDOC */
func PostDndSpotifyPlay(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	var req struct {
		Uri string `json:"uri"`
	}
	if !dndBody(w, r, &req) {
		return
	}
	uri, ok := spotifyUri(req.Uri)
	if !ok {
		dndError(w, http.StatusBadRequest, "%q is not a Spotify uri", req.Uri)
		return
	}
	if spotifyApp() {
		// Started first and waited for, so the play is not lost on an app
		// still opening.
		if _, err := osa(`if application "Spotify" is not running then
	tell application "Spotify" to launch
	repeat 40 times
		if application "Spotify" is running then exit repeat
		delay 0.25
	end repeat
	delay 1.5
end if
tell application "Spotify" to play track "` + uri + `"`); err != nil {
			dndError(w, http.StatusBadGateway, "Spotify would not play it: %s", err)
			return
		}
		time.Sleep(400 * time.Millisecond)
		dndJson(w, map[string]interface{}{"Ok": true, "Msg": "Playing", "State": spotifyNow()})
		return
	}
	body := map[string]interface{}{}
	if strings.HasPrefix(uri, "spotify:track:") {
		body["uris"] = []string{uri}
	} else {
		body["context_uri"] = uri
	}
	b, _ := json.Marshal(body)
	if _, err := spotifyApi("PUT", "/me/player/play", b); err != nil {
		dndError(w, http.StatusBadGateway, "Spotify would not play it: %s", err)
		return
	}
	dndJson(w, map[string]interface{}{"Ok": true, "Msg": "Playing"})
}

/* SDOC: API
* POST /dnd/music/spotify/control — Pause, Resume, Skip or Turn Spotify Up
	Steers the Spotify app: =action= is =pause=, =resume=, =next=, =previous= or =volume= (with
	=value= 0..100). A pause when the app is not running does nothing (and does not start it).

	*Method:* =POST=

	*Request Body:* ={action, value}=

	*Response:* ={Ok, Msg, State}=
	EDOC */
func PostDndSpotifyControl(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	var req struct {
		Action string `json:"action"`
		Value  int    `json:"value"`
	}
	if !dndBody(w, r, &req) {
		return
	}
	cmd := map[string]string{"pause": "pause", "resume": "play", "next": "next track", "previous": "previous track"}[req.Action]
	if req.Action == "volume" {
		v := req.Value
		if v < 0 {
			v = 0
		}
		if v > 100 {
			v = 100
		}
		cmd = "set sound volume to " + strconv.Itoa(v)
	}
	if cmd == "" {
		dndError(w, http.StatusBadRequest, "%q is not something Spotify does", req.Action)
		return
	}
	if !spotifyApp() {
		path := map[string]string{"pause": "/me/player/pause", "resume": "/me/player/play", "next": "/me/player/next", "previous": "/me/player/previous"}[req.Action]
		method := "PUT"
		if req.Action == "next" || req.Action == "previous" {
			method = "POST"
		}
		if req.Action == "volume" {
			path = "/me/player/volume?volume_percent=" + strconv.Itoa(req.Value)
		}
		if _, err := spotifyApi(method, path, nil); err != nil {
			dndError(w, http.StatusBadGateway, "%s", err)
			return
		}
		dndJson(w, map[string]interface{}{"Ok": true})
		return
	}
	if _, err := osa(`if application "Spotify" is running then tell application "Spotify" to ` + cmd); err != nil {
		dndError(w, http.StatusBadGateway, "%s", err)
		return
	}
	dndJson(w, map[string]interface{}{"Ok": true, "State": spotifyNow()})
}

// ---- Spotify's Web API ---------------------------------------------------------

func spotifyClientId() string {
	if Conf().Server == nil {
		return ""
	}
	return strings.TrimSpace(Conf().Server.SpotifyClientId)
}

// Where Spotify sends the DM back after signing in: what the app on Spotify's
// side must list. Spotify takes plain http only for a loopback address.
func spotifyRedirect() string {
	if Conf().Server == nil {
		return ""
	}
	if r := strings.TrimSpace(Conf().Server.SpotifyRedirect); r != "" {
		return r
	}
	port := Conf().Server.Port
	if port == 0 {
		port = 8010
	}
	return fmt.Sprintf("http://127.0.0.1:%d/dnd/music/spotify/callback", port)
}

type spotifyTokens struct {
	Access  string    `json:"access_token"`
	Refresh string    `json:"refresh_token"`
	Expires time.Time `json:"expires"`
}

var spotifyMu sync.Mutex

func spotifyTokenFile() string {
	base, err := os.UserConfigDir()
	if err != nil {
		base = os.TempDir()
	}
	return filepath.Join(base, "orgs", "spotify.json")
}

func saveSpotifyTokens(t spotifyTokens) error {
	b, _ := json.MarshalIndent(t, "", "  ")
	f := spotifyTokenFile()
	if err := os.MkdirAll(filepath.Dir(f), 0700); err != nil {
		return err
	}
	return os.WriteFile(f, b, 0600)
}

// A token to call the Web API with, refreshed when it has run out; false
// when not signed in (or not set up).
func spotifyToken() (string, bool) {
	if spotifyClientId() == "" {
		return "", false
	}
	spotifyMu.Lock()
	defer spotifyMu.Unlock()
	var t spotifyTokens
	b, err := os.ReadFile(spotifyTokenFile())
	if err != nil || json.Unmarshal(b, &t) != nil || t.Refresh == "" {
		return "", false
	}
	if time.Now().Before(t.Expires.Add(-time.Minute)) {
		return t.Access, true
	}
	form := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {t.Refresh}, "client_id": {spotifyClientId()}}
	nt, err := spotifyTokenCall(form)
	if err != nil {
		fmt.Fprintf(os.Stderr, "spotify: refreshing the sign in: %s\n", err)
		return "", false
	}
	if nt.Refresh == "" {
		nt.Refresh = t.Refresh
	}
	saveSpotifyTokens(nt)
	return nt.Access, true
}

func spotifyTokenCall(form url.Values) (spotifyTokens, error) {
	resp, err := http.PostForm("https://accounts.spotify.com/api/token", form)
	if err != nil {
		return spotifyTokens{}, err
	}
	defer resp.Body.Close()
	var r struct {
		Access  string `json:"access_token"`
		Refresh string `json:"refresh_token"`
		Expires int    `json:"expires_in"`
		Error   string `json:"error_description"`
	}
	json.NewDecoder(resp.Body).Decode(&r)
	if resp.StatusCode >= 400 || r.Access == "" {
		return spotifyTokens{}, fmt.Errorf("spotify answered %s %s", resp.Status, r.Error)
	}
	return spotifyTokens{Access: r.Access, Refresh: r.Refresh, Expires: time.Now().Add(time.Duration(r.Expires) * time.Second)}, nil
}

func spotifyApi(method, p string, body []byte) ([]byte, error) {
	tok, ok := spotifyToken()
	if !ok {
		return nil, fmt.Errorf("not signed in to Spotify")
	}
	var rd io.Reader
	if body != nil {
		rd = strings.NewReader(string(body))
	}
	req, _ := http.NewRequest(method, "https://api.spotify.com/v1"+p, rd)
	req.Header.Set("Authorization", "Bearer "+tok)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := (&http.Client{Timeout: 12 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("spotify answered %s: %s", resp.Status, strings.TrimSpace(string(b)))
	}
	return b, nil
}

// Sign ins under way: the state Spotify hands back, and its verifier.
var spotifyPending = struct {
	sync.Mutex
	m map[string]string
}{m: map[string]string{}}

func randomString(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

/* SDOC: API
* GET /dnd/music/spotify/login — Sign In to Spotify
	Sends the browser to Spotify to let orgs search and read the DM's playlists (PKCE: no secret).
	Spotify sends it back to =/dnd/music/spotify/callback=, which keeps the sign in.

	*Method:* =GET=
	EDOC */
func RequestDndSpotifyLogin(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	id := spotifyClientId()
	if id == "" {
		dndError(w, http.StatusBadRequest, "set spotifyClientId in orgs.yaml first")
		return
	}
	verifier := randomString(48)
	sum := sha256.Sum256([]byte(verifier))
	state := randomString(16)
	spotifyPending.Lock()
	spotifyPending.m[state] = verifier
	spotifyPending.Unlock()
	q := url.Values{
		"client_id": {id}, "response_type": {"code"}, "redirect_uri": {spotifyRedirect()}, "state": {state},
		"code_challenge_method": {"S256"}, "code_challenge": {base64.RawURLEncoding.EncodeToString(sum[:])},
		"scope": {"playlist-read-private playlist-read-collaborative user-library-read user-read-playback-state user-modify-playback-state"},
	}
	http.Redirect(w, r, "https://accounts.spotify.com/authorize?"+q.Encode(), http.StatusFound)
}

// The page Spotify sends the browser back to. Public (the browser comes back
// on 127.0.0.1, without the cookie it has for the server's own name): only a
// state this server handed out is taken.
func RequestDndSpotifyCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	spotifyPending.Lock()
	verifier, ok := spotifyPending.m[q.Get("state")]
	delete(spotifyPending.m, q.Get("state"))
	spotifyPending.Unlock()
	page := func(msg string) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<!doctype html><meta charset="utf-8"><title>Spotify</title><body style="font:18px Georgia,serif;background:#1b130b;color:#f4e9d4;display:grid;place-items:center;height:90vh"><p>%s</p><script>setTimeout(()=>window.close(),2500)</script>`, msg)
	}
	if !ok {
		page("That sign in is not one orgs started. Try again from the music panel.")
		return
	}
	if e := q.Get("error"); e != "" {
		page("Spotify said no: " + e)
		return
	}
	form := url.Values{"grant_type": {"authorization_code"}, "code": {q.Get("code")}, "redirect_uri": {spotifyRedirect()}, "client_id": {spotifyClientId()}, "code_verifier": {verifier}}
	t, err := spotifyTokenCall(form)
	if err != nil {
		page("Spotify would not sign in: " + err.Error())
		return
	}
	spotifyMu.Lock()
	err = saveSpotifyTokens(t)
	spotifyMu.Unlock()
	if err != nil {
		page("Signed in, but the sign in could not be kept: " + err.Error())
		return
	}
	page("Spotify is connected. You can close this tab.")
}

type spotifyItem struct {
	Kind     string  `json:"kind"` // track, album, playlist
	Name     string  `json:"name"`
	By       string  `json:"by,omitempty"`
	Uri      string  `json:"uri"`
	Image    string  `json:"image,omitempty"`
	Duration float64 `json:"duration,omitempty"`
	Count    int     `json:"count,omitempty"`
}

type spotifyImage struct {
	Url string `json:"url"`
}

func smallest(imgs []spotifyImage) string {
	if len(imgs) == 0 {
		return ""
	}
	return imgs[len(imgs)-1].Url
}

/* SDOC: API
* GET /dnd/music/spotify/search — Search Spotify
	Tracks, albums and playlists for =q=, through the Web API (signed in).

	*Method:* =GET=

	*Response:* ={Ok, Items: [{kind, name, by, uri, image, duration, count}]}=
	EDOC */
func RequestDndSpotifySearch(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		dndJson(w, map[string]interface{}{"Ok": true, "Items": []spotifyItem{}})
		return
	}
	b, err := spotifyApi("GET", "/search?type=track,album,playlist&limit=8&q="+url.QueryEscape(q), nil)
	if err != nil {
		dndError(w, http.StatusBadGateway, "%s", err)
		return
	}
	var res struct {
		Tracks struct {
			Items []struct {
				Name     string `json:"name"`
				Uri      string `json:"uri"`
				Duration int    `json:"duration_ms"`
				Artists  []struct {
					Name string `json:"name"`
				} `json:"artists"`
				Album struct {
					Images []spotifyImage `json:"images"`
				} `json:"album"`
			} `json:"items"`
		} `json:"tracks"`
		Albums struct {
			Items []*struct {
				Name    string         `json:"name"`
				Uri     string         `json:"uri"`
				Images  []spotifyImage `json:"images"`
				Total   int            `json:"total_tracks"`
				Artists []struct {
					Name string `json:"name"`
				} `json:"artists"`
			} `json:"items"`
		} `json:"albums"`
		Playlists struct {
			Items []*spotifyPlaylist `json:"items"`
		} `json:"playlists"`
	}
	json.Unmarshal(b, &res)
	out := []spotifyItem{}
	for _, p := range res.Playlists.Items {
		if p != nil {
			out = append(out, p.item())
		}
	}
	for _, a := range res.Albums.Items {
		if a == nil {
			continue
		}
		by := ""
		if len(a.Artists) > 0 {
			by = a.Artists[0].Name
		}
		out = append(out, spotifyItem{Kind: "album", Name: a.Name, By: by, Uri: a.Uri, Image: smallest(a.Images), Count: a.Total})
	}
	for _, t := range res.Tracks.Items {
		by := []string{}
		for _, a := range t.Artists {
			by = append(by, a.Name)
		}
		out = append(out, spotifyItem{Kind: "track", Name: t.Name, By: strings.Join(by, ", "), Uri: t.Uri, Image: smallest(t.Album.Images), Duration: float64(t.Duration) / 1000})
	}
	dndJson(w, map[string]interface{}{"Ok": true, "Items": out})
}

type spotifyPlaylist struct {
	Name   string         `json:"name"`
	Uri    string         `json:"uri"`
	Images []spotifyImage `json:"images"`
	Owner  struct {
		Name string `json:"display_name"`
	} `json:"owner"`
	Tracks struct {
		Total int `json:"total"`
	} `json:"tracks"`
}

func (p spotifyPlaylist) item() spotifyItem {
	return spotifyItem{Kind: "playlist", Name: p.Name, By: p.Owner.Name, Uri: p.Uri, Image: smallest(p.Images), Count: p.Tracks.Total}
}

/* SDOC: API
* GET /dnd/music/spotify/playlists — The DM's Spotify Playlists
	The signed in DM's playlists, their own and followed, up to two hundred.

	*Method:* =GET=

	*Response:* ={Ok, Items: [{kind, name, by, uri, image, count}]}=
	EDOC */
func RequestDndSpotifyPlaylists(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	out := []spotifyItem{}
	for off := 0; off < 200; off += 50 {
		b, err := spotifyApi("GET", fmt.Sprintf("/me/playlists?limit=50&offset=%d", off), nil)
		if err != nil {
			if off == 0 {
				dndError(w, http.StatusBadGateway, "%s", err)
				return
			}
			break
		}
		var page struct {
			Items []*spotifyPlaylist `json:"items"`
			Next  string             `json:"next"`
		}
		json.Unmarshal(b, &page)
		for _, p := range page.Items {
			if p != nil {
				out = append(out, p.item())
			}
		}
		if page.Next == "" {
			break
		}
	}
	dndJson(w, map[string]interface{}{"Ok": true, "Items": out})
}

var spotifyIdRe = regexp.MustCompile(`^[0-9a-fA-F]{32}$`)

// setYamlKey sets `key: "value"` in the server: section of a yaml file's
// lines, replacing the key's line (commented out or not) or adding it after
// `after` (else at the section's top), indented as the section is. Only that
// line changes: the rest of the file - comments, order, spacing - stays.
func setYamlKey(lines []string, key, value, after string) ([]string, error) {
	start, end := -1, len(lines)
	for i, l := range lines {
		if start < 0 && strings.TrimRight(l, " \t") == "server:" {
			start = i
			continue
		}
		if start >= 0 && i > start && len(l) > 0 && l[0] != ' ' && l[0] != '\t' && l[0] != '#' {
			end = i
			break
		}
	}
	if start < 0 {
		return nil, fmt.Errorf("the file has no server: section")
	}
	indent := "  "
	for i := start + 1; i < end; i++ {
		if t := strings.TrimLeft(lines[i], " "); t != "" && !strings.HasPrefix(t, "#") {
			indent = lines[i][:len(lines[i])-len(t)]
			break
		}
	}
	line := indent + key + ": " + strconv.Quote(value)
	keyRe := regexp.MustCompile(`^\s*#?\s*` + regexp.QuoteMeta(key) + `\s*:`)
	for i := start + 1; i < end; i++ {
		if keyRe.MatchString(lines[i]) {
			out := append([]string{}, lines...)
			out[i] = line
			return out, nil
		}
	}
	at := start + 1
	if after != "" {
		afterRe := regexp.MustCompile(`^\s*` + regexp.QuoteMeta(after) + `\s*:`)
		for i := start + 1; i < end; i++ {
			if afterRe.MatchString(lines[i]) {
				at = i + 1
				break
			}
		}
	}
	out := append([]string{}, lines[:at]...)
	out = append(out, line)
	return append(out, lines[at:]...), nil
}

/* SDOC: API
* POST /dnd/music/spotify/setup — Keep the Spotify App's Client Id
	Writes =spotifyClientId= (a Spotify app's 32 character client id) into the server's own yaml
	file - that line only, replacing it (commented out or not) or adding it beside =dndMusicPath=
	- after keeping the file as it was in =<file>.before-spotify=, then reloads the configuration,
	so the music panel can connect at once without a restart.

	*Method:* =POST=

	*Request Body:* ={clientId}=

	*Response:* ={Ok, Msg, File, Redirect}=
	EDOC */
func PostDndSpotifySetup(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	var req struct {
		ClientId string `json:"clientId"`
	}
	if !dndBody(w, r, &req) {
		return
	}
	id := strings.TrimSpace(req.ClientId)
	if !spotifyIdRe.MatchString(id) {
		dndError(w, http.StatusBadRequest, "a Spotify client id is 32 letters and digits (0-9, a-f): that one is not")
		return
	}
	file, code, err := editConfigFile(".before-spotify", func(lines []string) ([]string, error) {
		return setYamlKey(lines, "spotifyClientId", strings.ToLower(id), "dndMusicPath")
	})
	if err != nil {
		dndError(w, code, "%s: %s", filepath.Base(file), err)
		return
	}
	if _, err := ReloadConfig(); err != nil {
		dndError(w, http.StatusInternalServerError, "written, but the configuration did not reload: %s", err)
		return
	}
	dndJson(w, map[string]interface{}{"Ok": true, "Msg": "Saved in " + filepath.Base(file), "File": file, "Redirect": spotifyRedirect()})
}
