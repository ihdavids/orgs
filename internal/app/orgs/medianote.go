package orgs

// org-media-note: notes on a video or a recording, each a list item that
// starts with a link to its moment, written the way org-media-note writes
// them so the file opens in Emacs and the links jump mpv there:
//
//	* Week 3 lecture
//	:PROPERTIES:
//	:MEDIA: lectures/week3.mp4
//	:END:
//	- [[video:lectures/week3.mp4#0:12:30][0:12:30]] the proof starts here
//	- [[video:lectures/week3.mp4#0:31:05-0:32:10][0:31:05-0:32:10]] worth hearing twice
//
// The link grammar is internal/common/medianote.go. org-media-note keeps no
// property of its own (it asks mpv what is playing, or reads a citation key
// from Custom_ID), so a "session" here is any heading with a :MEDIA: property
// (ours: a path, a url, a link or cite:key), an :AUDIO: one (a voice note's
// recording) or a media link in its own text. A videocite/audiocite link's key
// is looked up in the file's bibliographies (cite.go), file field first, then
// url, which is org-media-note-cite's file-first order.
//
// Only with features.mediaNotes on. Local media is played from /images when it
// is under the first org directory (the file server answers Range requests, so
// a player can seek) and otherwise from GET /media/file, which serves only a
// recording that file names, as /noter/pdf does.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/gorilla/mux"
	"github.com/ihdavids/go-org/org"
	htmlexp "github.com/ihdavids/orgs/internal/app/orgs/plugs/html"
	"github.com/ihdavids/orgs/internal/common"
)

func registerMediaNoteRoutes(api *mux.Router) {
	api.HandleFunc("/media/sessions", RequestMediaSessions).Methods("GET")
	api.HandleFunc("/media/notes", RequestMediaNotes).Methods("GET")
	api.HandleFunc("/media/note", PostMediaNote).Methods("POST")
	api.HandleFunc("/media/start", PostMediaStart).Methods("POST")
	api.HandleFunc("/media/transcribe", PostMediaTranscribe).Methods("POST")
	api.HandleFunc("/media/file", RequestMediaFile).Methods("GET")
}

func init() {
	// The html exporter draws the links as a player while the feature is on.
	// It cannot ask the server itself (the server imports the exporters).
	htmlexp.MediaNotesOn = func() bool { return featureOn("mediaNotes") }
	htmlexp.MediaNoteResolve = func(docPath, typ, target string) (string, string, bool) {
		ref := mediaRefFor(docPath, nil, typ, target)
		if ref.Online {
			return ref.Kind, ref.Url, true
		}
		return ref.Kind, ref.Path, false
	}
}

// MediaRef is one recording, as a client plays it.
type MediaRef struct {
	Kind string // video or audio
	// The link type a new note is written with (video, audio, videocite,
	// audiocite) and its target as written: a path relative to the org file,
	// a url or a citation key.
	Link   string
	Target string
	// What notes are grouped by: the local file, the url, or cite:key when the
	// key names nothing that can be played.
	Key string
	// The file on the server's disk; "" for online media.
	Path   string
	Exists bool
	Online bool
	// Where a browser plays it from: a path on this server, or the url.
	Url string
	// For an embedded player.
	Youtube string `json:",omitempty"`
	Vimeo   string `json:",omitempty"`
	// What a cite key resolved to, for saying so.
	Cite string `json:",omitempty"`
}

// MediaNote is one timestamp link and the words around it.
type MediaNote struct {
	Seconds float64
	End     float64 // the end of an A-B loop, -1 when it is not one
	Stamp   string  // as the link writes it
	Label   string  // the link's description
	Text    string  // the list item or line it is in, the links taken out
	Line    int     // 0 based
	Hash    string  // the heading it is under
	Headline string
}

// MediaGroup is the notes on one recording.
type MediaGroup struct {
	Media MediaRef
	Notes []MediaNote
}

// MediaSession is a heading with a recording.
type MediaSession struct {
	Hash     string
	Headline string
	Filename string
	Line     int
	// Where the recording came from: property, audio (a voice note), link.
	From  string
	Media MediaRef
	Notes int
}

func mediaHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

// mediaLocalPath is where a media target is on this machine: relative to the
// org file, as Emacs's expand-file-name reads it, else to the first org dir.
func mediaLocalPath(orgFile, target string) string {
	t := strings.TrimSpace(target)
	t = strings.TrimPrefix(t, "file://")
	t = strings.TrimPrefix(t, "file:")
	t = mediaHome(t)
	if filepath.IsAbs(t) {
		return filepath.Clean(t)
	}
	p := filepath.Join(filepath.Dir(orgFile), filepath.FromSlash(t))
	if _, err := os.Stat(p); err != nil {
		if dirs := Conf().Server.OrgDirs; len(dirs) > 0 {
			alt := filepath.Join(dirs[0], filepath.FromSlash(t))
			if _, err := os.Stat(alt); err == nil {
				p = alt
			}
		}
	}
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	return filepath.Clean(p)
}

// The file a bib entry's file field names: JabRef and Zotero write
// `desc:path:type;desc:path:type`, others a bare path.
func bibMediaFile(field string) string {
	first := ""
	for _, part := range strings.Split(field, ";") {
		part = strings.TrimSpace(strings.Trim(part, "{}"))
		if part == "" {
			continue
		}
		p := part
		if bits := strings.Split(part, ":"); len(bits) >= 3 {
			p = strings.Join(bits[1:len(bits)-1], ":")
		}
		if common.MediaKindOf(p) != "" {
			return p
		}
		if first == "" {
			first = p
		}
	}
	return first
}

// citeMedia is what a citation key plays: its entry's file, else its url.
func citeMedia(orgFile string, lines []string, key string) (string, bool) {
	key = strings.TrimPrefix(strings.TrimSpace(key), "@")
	if lines == nil {
		lines, _ = readFileLines(orgFile)
	}
	entries, _ := CiteEntries(orgFile, lines)
	e := entries[key]
	if e == nil {
		return "", false
	}
	if f := bibMediaFile(e.Field("file")); f != "" {
		p := mediaHome(f)
		if !filepath.IsAbs(p) {
			p = filepath.Join(filepath.Dir(orgFile), p)
		}
		if _, err := os.Stat(p); err == nil {
			return p, true
		}
	}
	if u := strings.TrimSpace(e.Field("url")); u != "" {
		return u, true
	}
	return "", false
}

// mediaRefFor resolves one link's target. lines may be nil (read when a
// citation needs them).
func mediaRefFor(orgFile string, lines []string, typ, target string) MediaRef {
	target = strings.TrimSpace(target)
	r := MediaRef{Link: typ, Target: target, Kind: strings.TrimSuffix(typ, "cite")}
	play := target
	if strings.HasSuffix(typ, "cite") {
		got, ok := citeMedia(orgFile, lines, target)
		if !ok {
			r.Key = "cite:" + strings.TrimPrefix(target, "@")
			return r
		}
		play = got
		r.Cite = got
	}
	if common.IsOnlineMedia(play) {
		r.Online, r.Exists = true, true
		r.Key, r.Url = play, play
		r.Youtube, r.Vimeo = common.YoutubeId(play), common.VimeoId(play)
		if r.Kind == "" {
			r.Kind = "video"
		}
		return r
	}
	r.Path = mediaLocalPath(orgFile, play)
	r.Key = r.Path
	if r.Kind == "" {
		r.Kind = common.MediaKindOf(r.Path)
		if r.Kind == "" {
			r.Kind = "video"
		}
	}
	if st, err := os.Stat(r.Path); err == nil && !st.IsDir() {
		r.Exists = true
		r.Url = mediaServeUrl(orgFile, r.Path)
	}
	return r
}

func mediaServeUrl(orgFile, abs string) string {
	if u := mediaURL(abs, orgFile); u != "" {
		return u
	}
	return "/media/file?file=" + urlQueryEscape(orgFile) + "&path=" + urlQueryEscape(abs)
}

var mediaPropCiteRe = regexp.MustCompile(`^\[?(?:cite:)?@?([A-Za-z0-9_:.\-]+)\]?$`)

// mediaPropTarget reads a :MEDIA: or :AUDIO: value: a media link, a file
// link, a url, cite:key / @key, or a bare path.
func mediaPropTarget(v, defType string) (typ, target string) {
	v = strings.TrimSpace(v)
	if ls := common.FindMediaLinks(v); len(ls) > 0 {
		return ls[0].Type, ls[0].Target
	}
	if strings.HasPrefix(v, "[[") {
		v = strings.TrimPrefix(v, "[[")
		if i := strings.Index(v, "]"); i >= 0 {
			v = v[:i]
		}
	}
	v = strings.TrimPrefix(strings.TrimPrefix(v, "file:"), "attachment:")
	if (strings.HasPrefix(v, "cite:") || strings.HasPrefix(v, "@") || strings.HasPrefix(v, "[cite:")) && !strings.Contains(v, "/") {
		if m := mediaPropCiteRe.FindStringSubmatch(strings.TrimPrefix(v, "[cite:")); m != nil {
			if defType == "" {
				defType = "video"
			}
			return defType + "cite", m[1]
		}
	}
	if defType == "" {
		defType = common.MediaKindOf(v)
		if defType == "" {
			defType = "video"
		}
	}
	return defType, v
}

// mediaSessionOf is a heading's recording, if it has one.
func mediaSessionOf(f *common.OrgFile, sec *org.Section, lines []string) (MediaSession, bool) {
	s := MediaSession{Hash: sec.Hash, Headline: strings.TrimSpace(headlineTitle(sec)), Filename: f.Filename, Line: sec.Headline.Pos.Row}
	if v, ok := ownProp(sec, "MEDIA"); ok && strings.TrimSpace(v) != "" {
		typ, target := mediaPropTarget(v, "")
		s.From, s.Media = "property", mediaRefFor(f.Filename, lines, typ, target)
		return s, true
	}
	if v, ok := ownProp(sec, "AUDIO"); ok && strings.TrimSpace(v) != "" {
		typ, target := mediaPropTarget(v, "audio")
		if common.MediaKindOf(target) != "" || strings.HasSuffix(typ, "cite") {
			s.From, s.Media = "audio", mediaRefFor(f.Filename, lines, typ, target)
			return s, true
		}
	}
	row := sec.Headline.Pos.Row
	if row < 0 || row >= len(lines) {
		return s, false
	}
	own := ownLinesEnd(lines, row, len(lines)-1)
	inBlock := false
	for i := row + 1; i <= own; i++ {
		if mediaBlockRe.MatchString(lines[i]) {
			inBlock = strings.EqualFold(mediaBlockRe.FindStringSubmatch(lines[i])[1], "begin")
			continue
		}
		if inBlock {
			continue
		}
		if ls := common.FindMediaLinks(lines[i]); len(ls) > 0 {
			s.From, s.Media = "link", mediaRefFor(f.Filename, lines, ls[0].Type, ls[0].Target)
			return s, true
		}
	}
	return s, false
}

var mediaItemRe = regexp.MustCompile(`^(\s*)([-+*]|\d+[.)])\s+(\[[ X-]\]\s+)?`)
var mediaBlockRe = regexp.MustCompile(`(?i)^\s*#\+(begin|end)_(src|example)\b`)

// mediaNoteText is a line's words without its bullet, checkbox and media
// links, with a list item's continuation lines after it.
func mediaNoteText(lines []string, i, to int) string {
	l := lines[i]
	parts := []string{}
	strip := func(s string) string {
		ls := common.FindMediaLinks(s)
		for j := len(ls) - 1; j >= 0; j-- {
			s = s[:ls[j].From] + s[ls[j].To:]
		}
		return strings.Join(strings.Fields(s), " ")
	}
	m := mediaItemRe.FindStringSubmatch(l)
	if m == nil {
		return strip(l)
	}
	first := strings.TrimSpace(strip(l[len(m[0]):]))
	first = strings.TrimSpace(strings.TrimPrefix(first, "::"))
	if first != "" {
		parts = append(parts, first)
	}
	ind := len(m[1])
	for j := i + 1; j <= to && j < len(lines); j++ {
		c := lines[j]
		if strings.TrimSpace(c) == "" || anyHeadRe.MatchString(c) || mediaItemRe.MatchString(c) {
			break
		}
		if len(c)-len(strings.TrimLeft(c, " \t")) <= ind {
			break
		}
		parts = append(parts, strip(c))
	}
	return strings.Join(parts, " ")
}

// mediaNotesIn is every timestamp link in rows [from, to], grouped by
// recording, each group in time order. first, if it names a recording, is the
// first group even with no notes.
func mediaNotesIn(f *common.OrgFile, lines []string, from, to int, first *MediaRef) []MediaGroup {
	groups := []MediaGroup{}
	at := map[string]int{}
	add := func(r MediaRef) int {
		if i, ok := at[r.Key]; ok {
			return i
		}
		at[r.Key] = len(groups)
		groups = append(groups, MediaGroup{Media: r, Notes: []MediaNote{}})
		return len(groups) - 1
	}
	if first != nil && first.Key != "" {
		add(*first)
	}
	refs := map[string]MediaRef{}
	inBlock := false
	for i := from; i <= to && i < len(lines); i++ {
		if mediaBlockRe.MatchString(lines[i]) {
			inBlock = strings.EqualFold(mediaBlockRe.FindStringSubmatch(lines[i])[1], "begin")
			continue
		}
		if inBlock || !strings.Contains(lines[i], "[[") {
			continue
		}
		for _, l := range common.FindMediaLinks(lines[i]) {
			if l.Start < 0 {
				continue
			}
			k := l.Type + ":" + l.Target
			r, ok := refs[k]
			if !ok {
				r = mediaRefFor(f.Filename, lines, l.Type, l.Target)
				refs[k] = r
			}
			g := add(r)
			stamp := common.MediaStamp(l.Start, l.Start != float64(int(l.Start)))
			if l.End >= 0 {
				stamp += "-" + common.MediaStamp(l.End, l.End != float64(int(l.End)))
			}
			n := MediaNote{Seconds: l.Start, End: l.End, Stamp: stamp, Label: l.Label, Text: mediaNoteText(lines, i, to), Line: i}
			if sec := sectionAtRow(f, i); sec != nil {
				n.Hash, n.Headline = sec.Hash, strings.TrimSpace(headlineTitle(sec))
			}
			groups[g].Notes = append(groups[g].Notes, n)
		}
	}
	for _, g := range groups {
		sort.SliceStable(g.Notes, func(a, b int) bool { return g.Notes[a].Seconds < g.Notes[b].Seconds })
	}
	return groups
}

/* SDOC: API
* GET /media/sessions — Every Recording With Notes
* GET /media/notes — The Notes On A Heading Or File
* POST /media/note — Add A Note At A Moment
* POST /media/start — Start Notes On A Recording
* POST /media/transcribe — Transcribe A Recording Into Linked Notes
* GET /media/file — The Recording

	#+BEGIN_EXAMPLE
	GET  /media/sessions    {"Ok": true, "Sessions": [{Hash, Headline, Filename, Line, From, Notes,
	                         "Media": {Kind, Link, Target, Key, Path, Exists, Online, Url, Youtube, Vimeo}}]}
	GET  /media/notes?hash=...  (or ?file=notes.org)
	                        {"Ok": true, "Session": {...}, "Groups": [{"Media": {...},
	                         "Notes": [{Seconds, End, Stamp, Label, Text, Line, Hash, Headline}]}]}
	POST /media/note        {"Hash": h, "Seconds": 750, "End": 760, "Text": "...", "Media": "optional"}
	                        (or "File" + "Line" for the heading that line is in)
	POST /media/start       {"File": "notes.org", "Under": "optional parent hash", "Media": "x.mp4", "Title": "..."}
	POST /media/transcribe  {"Hash": h, "Model": "", "Language": "", "Write": true}
	GET  /media/file?file=<org file>&path=<the recording's path>
	#+END_EXAMPLE

	org-media-note's links: =[[video:file.mp4#0:12:30][0:12:30]]=,
	=[[audio:x.mp3#0:01:23-0:01:40][...]]= (an A-B loop), =videocite:= /
	=audiocite:= with a citation key, and urls (YouTube, Vimeo) as the
	target. A session is a heading with =:MEDIA:=, =:AUDIO:= or a media link
	of its own. A note goes in as =- [[video:…#H:MM:SS][H:MM:SS]] text=
	among the heading's other notes on that recording, in time order;
	=End= makes it a loop. =Media= is only needed when the heading has no
	recording, or for another one. Transcribing needs the =voice:= whisper
	server; without =Write= it only answers the segments and the lines.
	=Url= is where a browser plays the recording: under =/images= when it is
	in the first org directory, else =/media/file=, which serves only a
	recording the org file names. Needs =features: mediaNotes:=.
EDOC */
func RequestMediaSessions(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "mediaNotes") {
		return
	}
	out := []MediaSession{}
	cache := map[string][]string{}
	walkSections(func(f *common.OrgFile, sec *org.Section) {
		lines, ok := cache[f.Filename]
		if !ok {
			lines, _ = readFileLines(f.Filename)
			cache[f.Filename] = lines
		}
		if s, ok := mediaSessionOf(f, sec, lines); ok {
			s.Notes = mediaCount(lines, sec)
			out = append(out, s)
		}
	})
	dynJson(w, map[string]interface{}{"Ok": true, "Sessions": out})
}

func mediaCount(lines []string, sec *org.Section) int {
	from := sec.Headline.Pos.Row
	if from < 0 || from >= len(lines) {
		return 0
	}
	to := subtreeEndRow(lines, from, sec.Headline.Lvl, from)
	n := 0
	for i := from; i <= to && i < len(lines); i++ {
		for _, l := range common.FindMediaLinks(lines[i]) {
			if l.Start >= 0 {
				n++
			}
		}
	}
	return n
}

// mediaHeading is the heading a request names, by hash or by file and line.
func mediaHeading(w http.ResponseWriter, hash, file string, line int) (*common.OrgFile, *org.Section, []string, bool) {
	var f *common.OrgFile
	var sec *org.Section
	if hash != "" {
		sec = GetDb().FindByHash(hash)
		if sec == nil || sec.Headline == nil {
			dynJson(w, common.Result{Msg: "no heading with that hash - it may have moved; refresh and try again"})
			return nil, nil, nil, false
		}
		f = GetDb().FileFromSection(sec)
	} else {
		f = GetDb().FindByFile(file)
		if f == nil {
			dynJson(w, common.Result{Msg: fmt.Sprintf("no org file %q", file)})
			return nil, nil, nil, false
		}
		sec = findHeadingByLine(f, line)
		if sec == nil || sec.Headline == nil {
			dynJson(w, common.Result{Msg: "that line is above the first heading: notes go under a heading"})
			return nil, nil, nil, false
		}
	}
	if f == nil {
		dynJson(w, common.Result{Msg: "could not find the file that heading is in"})
		return nil, nil, nil, false
	}
	lines, err := readFileLines(f.Filename)
	if err != nil {
		dynJson(w, common.Result{Msg: err.Error()})
		return nil, nil, nil, false
	}
	return f, sec, lines, true
}

func RequestMediaNotes(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "mediaNotes") {
		return
	}
	q := r.URL.Query()
	if q.Get("hash") == "" {
		f := GetDb().FindByFile(q.Get("file"))
		if f == nil {
			dynJson(w, common.Result{Msg: fmt.Sprintf("no org file %q", q.Get("file"))})
			return
		}
		lines, err := readFileLines(f.Filename)
		if err != nil {
			dynJson(w, common.Result{Msg: err.Error()})
			return
		}
		dynJson(w, map[string]interface{}{"Ok": true, "Filename": f.Filename, "Groups": mediaNotesIn(f, lines, 0, len(lines)-1, nil)})
		return
	}
	f, sec, lines, ok := mediaHeading(w, q.Get("hash"), "", 0)
	if !ok {
		return
	}
	s, has := mediaSessionOf(f, sec, lines)
	var first *MediaRef
	if has {
		first = &s.Media
	}
	from := sec.Headline.Pos.Row
	to := subtreeEndRow(lines, from, sec.Headline.Lvl, from)
	groups := mediaNotesIn(f, lines, from, to, first)
	if has {
		s.Notes = mediaCount(lines, sec)
	}
	dynJson(w, map[string]interface{}{"Ok": true, "Session": s, "Groups": groups})
}

// mediaTargetFor is the link type and target a note is written with: the
// one asked for, else the heading's own recording.
func mediaTargetFor(f *common.OrgFile, sec *org.Section, lines []string, media, typ string) (string, string, error) {
	media = strings.TrimSpace(media)
	if media == "" {
		s, ok := mediaSessionOf(f, sec, lines)
		if !ok {
			return "", "", fmt.Errorf("%q has no recording: give it a :MEDIA: property, or say which recording", strings.TrimSpace(headlineTitle(sec)))
		}
		return s.Media.Link, s.Media.Target, nil
	}
	if ls := common.FindMediaLinks(media); len(ls) > 0 {
		return ls[0].Type, ls[0].Target, nil
	}
	for _, t := range common.MediaLinkTypes {
		if strings.HasPrefix(media, t+":") && !common.IsOnlineMedia(media) {
			p, _, _ := common.SplitMediaTarget(strings.TrimPrefix(media, t+":"))
			return t, p, nil
		}
	}
	if typ == "" {
		if common.IsOnlineMedia(media) {
			typ = "video"
		} else if typ = common.MediaKindOf(media); typ == "" {
			typ = "video"
		}
	}
	return typ, mediaRelTarget(f.Filename, media), nil
}

// An absolute path under the org file's folder is written relative to it,
// which is how org-link-file-path-type's default (adaptive) writes it.
func mediaRelTarget(orgFile, media string) string {
	if !filepath.IsAbs(media) {
		return media
	}
	dir, _ := filepath.Abs(filepath.Dir(orgFile))
	if rel, err := filepath.Rel(dir, media); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return media
}

// mediaNoteLines are a note as list item lines.
func mediaNoteLines(ind, link, text string) []string {
	text = strings.TrimRight(text, " \t\n")
	ls := strings.Split(text, "\n")
	out := []string{strings.TrimRight(ind+"- "+link+" "+strings.TrimSpace(ls[0]), " ")}
	for _, l := range ls[1:] {
		if strings.TrimSpace(l) == "" {
			continue
		}
		out = append(out, ind+"  "+strings.TrimSpace(l))
	}
	return out
}

// mediaInsertAt is where a note at seconds on recording key goes in the
// heading's own lines [from, own]: before the first note on that recording
// that comes after it, else after the last one, else after the heading's
// text. Answers the row and the indent to use.
func mediaInsertAt(f *common.OrgFile, lines []string, from, own, lvl int, key string, seconds float64) (int, string) {
	lastEnd, ind := -1, ""
	inBlock := false
	for i := from + 1; i <= own && i < len(lines); i++ {
		if mediaBlockRe.MatchString(lines[i]) {
			inBlock = strings.EqualFold(mediaBlockRe.FindStringSubmatch(lines[i])[1], "begin")
			continue
		}
		m := mediaItemRe.FindStringSubmatch(lines[i])
		if inBlock || m == nil {
			continue
		}
		ls := common.FindMediaLinks(lines[i])
		if len(ls) == 0 || ls[0].Start < 0 || mediaRefFor(f.Filename, lines, ls[0].Type, ls[0].Target).Key != key {
			continue
		}
		if ls[0].Start > seconds {
			return i, m[1]
		}
		// The item's continuation lines go with it.
		end := i
		for j := i + 1; j <= own && j < len(lines); j++ {
			c := lines[j]
			if strings.TrimSpace(c) == "" || mediaItemRe.MatchString(c) || len(c)-len(strings.TrimLeft(c, " \t")) <= len(m[1]) {
				break
			}
			end = j
		}
		lastEnd, ind = end, m[1]
	}
	if lastEnd >= 0 {
		return lastEnd + 1, ind
	}
	at := own
	for at > from && strings.TrimSpace(lines[at]) == "" {
		at--
	}
	return at + 1, mediaBodyIndent(lines, from, own, lvl)
}

// mediaBodyIndent is the indent of a heading's own text, past its drawers and
// planning; with none, what the file does elsewhere.
func mediaBodyIndent(lines []string, from, own, lvl int) string {
	inDrawer := false
	for i := from + 1; i <= own && i < len(lines); i++ {
		l := lines[i]
		switch {
		case strings.TrimSpace(l) == "":
		case inDrawer:
			inDrawer = !drawerEndRe.MatchString(l)
		case drawerOpenRe.MatchString(l):
			inDrawer = true
		case planningRe.MatchString(l):
		default:
			return l[:len(l)-len(strings.TrimLeft(l, " \t"))]
		}
	}
	return mediaFileIndent(lines, from, own, lvl)
}

// The indent a drawer gets: the heading's own drawer's, else the first one
// in the file (column zero is what Emacs writes since org 9.5), else the
// usual for the level.
func mediaFileIndent(lines []string, from, own, lvl int) string {
	if from+1 <= own && from+1 < len(lines) && (planningRe.MatchString(lines[from+1]) || drawerOpenRe.MatchString(lines[from+1])) {
		return headIndent(lines, from, own, lvl)
	}
	for _, l := range lines {
		if strings.EqualFold(strings.TrimSpace(l), ":PROPERTIES:") {
			return l[:len(l)-len(strings.TrimLeft(l, " \t"))]
		}
	}
	return indentOf(lvl)
}

func PostMediaNote(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "mediaNotes") {
		return
	}
	body, _ := io.ReadAll(r.Body)
	var req struct {
		Hash, File string
		Line       int
		Media      string
		Type       string
		Seconds    float64
		End        *float64
		Text       string
		Fraction   bool
	}
	if err := json.Unmarshal(body, &req); err != nil {
		dynJson(w, common.Result{Msg: err.Error()})
		return
	}
	if req.Seconds < 0 {
		dynJson(w, common.Result{Msg: "a note needs a time, in seconds from the start"})
		return
	}
	f, sec, lines, ok := mediaHeading(w, req.Hash, req.File, req.Line)
	if !ok {
		return
	}
	typ, target, err := mediaTargetFor(f, sec, lines, req.Media, req.Type)
	if err != nil {
		dynJson(w, common.Result{Msg: err.Error()})
		return
	}
	end := -1.0
	if req.End != nil && *req.End > req.Seconds {
		end = *req.End
	}
	from := sec.Headline.Pos.Row
	to := subtreeEndRow(lines, from, sec.Headline.Lvl, from)
	if to >= len(lines) {
		to = len(lines) - 1
	}
	own := ownLinesEnd(lines, from, to)
	key := mediaRefFor(f.Filename, lines, typ, target).Key
	at, ind := mediaInsertAt(f, lines, from, own, sec.Headline.Lvl, key, req.Seconds)
	link := common.MediaLinkText(typ, target, req.Seconds, end, req.Fraction)
	if err := writeLines(f.Filename, splice(lines, at, mediaNoteLines(ind, link, req.Text))); err != nil {
		dynJson(w, common.Result{Msg: err.Error()})
		return
	}
	dynJson(w, map[string]interface{}{"Ok": true, "Msg": "noted at " + common.MediaStamp(req.Seconds, req.Fraction), "Line": at, "Link": link})
}

func PostMediaStart(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "mediaNotes") {
		return
	}
	body, _ := io.ReadAll(r.Body)
	var req struct{ File, Under, Media, Title string }
	if err := json.Unmarshal(body, &req); err != nil {
		dynJson(w, common.Result{Msg: err.Error()})
		return
	}
	media := strings.TrimSpace(req.Media)
	if media == "" {
		dynJson(w, common.Result{Msg: "which recording? a file, a url or cite:key"})
		return
	}
	var f *common.OrgFile
	var parent *org.Section
	if req.Under != "" {
		parent = GetDb().FindByHash(req.Under)
		if parent == nil || parent.Headline == nil {
			dynJson(w, common.Result{Msg: "no heading with that hash - it may have moved; refresh and try again"})
			return
		}
		f = GetDb().FileFromSection(parent)
	} else {
		f = GetDb().FindByFile(req.File)
	}
	if f == nil {
		dynJson(w, common.Result{Msg: fmt.Sprintf("no org file %q", req.File)})
		return
	}
	cite := strings.HasPrefix(media, "cite:") || strings.HasPrefix(media, "@")
	if !cite && !common.IsOnlineMedia(media) && len(common.FindMediaLinks(media)) == 0 {
		media = mediaRelTarget(f.Filename, media)
		if _, err := os.Stat(mediaLocalPath(f.Filename, media)); err != nil {
			dynJson(w, common.Result{Msg: fmt.Sprintf("there is no %s", mediaLocalPath(f.Filename, media))})
			return
		}
	}
	title := strings.TrimSpace(strings.ReplaceAll(req.Title, "\n", " "))
	if title == "" {
		switch {
		case common.YoutubeId(media) != "":
			title = "YouTube " + common.YoutubeId(media)
		case common.IsOnlineMedia(media):
			title = media
		default:
			b := filepath.Base(strings.TrimPrefix(strings.TrimPrefix(media, "cite:"), "@"))
			title = strings.TrimSuffix(b, filepath.Ext(b))
		}
	}
	lines, err := readFileLines(f.Filename)
	if err != nil {
		dynJson(w, common.Result{Msg: err.Error()})
		return
	}
	var at int
	var add []string
	if parent != nil {
		from := parent.Headline.Pos.Row
		end := subtreeEndRow(lines, from, parent.Headline.Lvl, from)
		if end >= len(lines) {
			end = len(lines) - 1
		}
		for end > from && strings.TrimSpace(lines[end]) == "" {
			end--
		}
		lvl := parent.Headline.Lvl + 1
		ind := mediaFileIndent(lines, -2, -2, lvl)
		at = end + 1
		add = []string{strings.Repeat("*", lvl) + " " + title, ind + ":PROPERTIES:", ind + ":MEDIA: " + media, ind + ":END:"}
	} else {
		for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
			lines = lines[:len(lines)-1]
		}
		at = len(lines)
		add = []string{"* " + title, ":PROPERTIES:", ":MEDIA: " + media, ":END:"}
	}
	if err := writeLines(f.Filename, splice(lines, at, add)); err != nil {
		dynJson(w, common.Result{Msg: err.Error()})
		return
	}
	// The new heading's hash, for a client to open it at once.
	hash := ""
	if nf := GetDb().FindByFile(f.Filename); nf != nil {
		if s := findHeadingByLine(nf, at); s != nil && s.Headline != nil && s.Headline.Pos.Row == at {
			hash = s.Hash
		}
	}
	dynJson(w, map[string]interface{}{"Ok": true, "Msg": "started notes on " + media, "Line": at, "Hash": hash, "Filename": f.Filename})
}

func PostMediaTranscribe(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "mediaNotes") {
		return
	}
	body, _ := io.ReadAll(r.Body)
	var req struct {
		Hash, Media, Model, Language string
		Write                        bool
	}
	if err := json.Unmarshal(body, &req); err != nil {
		dynJson(w, common.Result{Msg: err.Error()})
		return
	}
	f, sec, lines, ok := mediaHeading(w, req.Hash, "", 0)
	if !ok {
		return
	}
	typ, target, err := mediaTargetFor(f, sec, lines, req.Media, "")
	if err != nil {
		dynJson(w, common.Result{Msg: err.Error()})
		return
	}
	ref := mediaRefFor(f.Filename, lines, typ, target)
	if ref.Online || !ref.Exists {
		dynJson(w, common.Result{Msg: "only a recording on the server's disk can be transcribed"})
		return
	}
	model, err := whisperModel(req.Model)
	if err != nil {
		dynJson(w, common.Result{Msg: fmt.Sprintf("no answer from the whisper server at %s - %s (see voice: in the yaml)", whisperBase(), whisperReason(err))})
		return
	}
	lang := req.Language
	if lang == "" {
		lang = voiceSettings().Language
	}
	t, err := whisperTranscribe(ref.Path, model, lang, "")
	if err != nil {
		dynJson(w, common.Result{Msg: err.Error()})
		return
	}
	from := sec.Headline.Pos.Row
	to := subtreeEndRow(lines, from, sec.Headline.Lvl, from)
	if to >= len(lines) {
		to = len(lines) - 1
	}
	own := ownLinesEnd(lines, from, to)
	at, ind := mediaInsertAt(f, lines, from, own, sec.Headline.Lvl, ref.Key, 1e12)
	add := []string{}
	for _, s := range t.Segments {
		text := strings.TrimSpace(s.Text)
		if text == "" {
			continue
		}
		if s.Speaker != "" {
			text = s.Speaker + ": " + text
		}
		add = append(add, mediaNoteLines(ind, common.MediaLinkText(typ, target, float64(int(s.Start)), -1, false), text)...)
	}
	if req.Write && len(add) > 0 {
		if err := writeLines(f.Filename, splice(lines, at, add)); err != nil {
			dynJson(w, common.Result{Msg: err.Error()})
			return
		}
	}
	dynJson(w, map[string]interface{}{"Ok": true, "Model": model, "Segments": t.Segments, "Lines": add, "Written": req.Write && len(add) > 0,
		"Msg": fmt.Sprintf("%d segments", len(add))})
}

// mediaNamedIn is whether an org file names this recording, in a link or a
// session's property.
func mediaNamedIn(f *common.OrgFile, path string) bool {
	lines, err := readFileLines(f.Filename)
	if err != nil {
		return false
	}
	for _, g := range mediaNotesIn(f, lines, 0, len(lines)-1, nil) {
		if g.Media.Path == path {
			return true
		}
	}
	found := false
	if f.Doc != nil && f.Doc.Outline.Section != nil {
		var walk func(*org.Section)
		walk = func(s *org.Section) {
			for _, c := range s.Children {
				if found {
					return
				}
				if isRealHeading(c) {
					if ms, ok := mediaSessionOf(f, c, lines); ok && ms.Media.Path == path {
						found = true
						return
					}
				}
				walk(c)
			}
		}
		walk(f.Doc.Outline.Section)
	}
	return found
}

var mediaMime = map[string]string{
	"mp4": "video/mp4", "m4v": "video/mp4", "webm": "video/webm", "ogv": "video/ogg", "ogg": "audio/ogg", "mov": "video/quicktime",
	"mkv": "video/x-matroska", "avi": "video/x-msvideo", "mpg": "video/mpeg", "ts": "video/mp2t", "flv": "video/x-flv",
	"mp3": "audio/mpeg", "m4a": "audio/mp4", "aac": "audio/aac", "wav": "audio/wav", "flac": "audio/flac", "opus": "audio/opus",
	"oga": "audio/ogg", "weba": "audio/webm",
}

func RequestMediaFile(w http.ResponseWriter, r *http.Request) {
	if !featureOn("mediaNotes") {
		http.Error(w, "media notes are turned off", http.StatusNotFound)
		return
	}
	f := GetDb().FindByFile(r.URL.Query().Get("file"))
	path := filepath.Clean(r.URL.Query().Get("path"))
	if f == nil || common.MediaKindOf(path) == "" || !mediaNamedIn(f, path) {
		http.Error(w, "no such recording", http.StatusNotFound)
		return
	}
	fh, err := os.Open(path)
	if err != nil {
		http.Error(w, "no such recording", http.StatusNotFound)
		return
	}
	defer fh.Close()
	st, err := fh.Stat()
	if err != nil || st.IsDir() {
		http.Error(w, "no such recording", http.StatusNotFound)
		return
	}
	if mt := mediaMime[strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))]; mt != "" {
		w.Header().Set("Content-Type", mt)
	}
	// ServeContent answers Range requests, which is what lets a player seek.
	http.ServeContent(w, r, filepath.Base(path), st.ModTime(), fh)
}
