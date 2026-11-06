package orgs

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ihdavids/orgs/internal/common"
)

func mediaCall(t *testing.T, h http.HandlerFunc, method, url string, body interface{}) map[string]interface{} {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, url, &buf)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h(rec, req)
	out := map[string]interface{}{}
	json.Unmarshal(rec.Body.Bytes(), &out)
	return out
}

func TestMediaNotes(t *testing.T) {
	text := `* Lecture
:PROPERTIES:
:MEDIA: lecture.mp4
:END:
Some words first.
- [[video:lecture.mp4#0:01:00][0:01:00]] the start
  carried on here
- [[video:lecture.mp4#0:10:00-0:10:30][0:10:00-0:10:30]] a loop
** Child
- [[audio:talk.mp3#0:00:05][0:00:05]] another recording
* Linked only
- [[video:https://www.youtube.com/watch?v=dQw4w9WgXcQ#0:00:43][0:00:43]] online
* Voice
:PROPERTIES:
:AUDIO: [[file:voice/v.webm]]
:END:
* Plain
#+begin_src org
- [[video:lecture.mp4#0:02:00][0:02:00]] not a note: in a block
#+end_src
`
	withFeatures(t, func(f *common.FeatureSettings) { f.MediaNotes = false })
	paths := loadOrg(t, map[string]string{"m.org": text, "lecture.mp4": "x", "talk.mp3": "x"})
	p := paths["m.org"]
	if r := mediaCall(t, RequestMediaSessions, "GET", "/media/sessions", nil); r["Off"] != true {
		t.Fatalf("answered with the feature off: %v", r)
	}
	withFeatures(t, func(f *common.FeatureSettings) { f.MediaNotes = true })
	Conf().Server.OrgDirs = []string{filepath.Dir(p)}

	r := mediaCall(t, RequestMediaSessions, "GET", "/media/sessions", nil)
	ss, _ := r["Sessions"].([]interface{})
	got := map[string]string{}
	for _, s := range ss {
		m := s.(map[string]interface{})
		got[m["Headline"].(string)] = m["From"].(string)
	}
	want := map[string]string{"Lecture": "property", "Child": "link", "Linked only": "link", "Voice": "audio"}
	if len(got) != len(want) {
		t.Fatalf("sessions %v", got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("session %s from %q want %q", k, got[k], v)
		}
	}

	_, sec := secAt(t, p, "Lecture")
	r = mediaCall(t, RequestMediaNotes, "GET", "/media/notes?hash="+url.QueryEscape(sec.Hash), nil)
	groups, _ := r["Groups"].([]interface{})
	if len(groups) != 2 {
		t.Fatalf("groups %v", r)
	}
	g0 := groups[0].(map[string]interface{})
	media := g0["Media"].(map[string]interface{})
	if media["Url"] != "/images/lecture.mp4" || media["Kind"] != "video" {
		t.Errorf("media %v", media)
	}
	notes := g0["Notes"].([]interface{})
	if len(notes) != 2 {
		t.Fatalf("notes %v", notes)
	}
	n0, n1 := notes[0].(map[string]interface{}), notes[1].(map[string]interface{})
	if n0["Seconds"] != 60.0 || n0["Text"] != "the start carried on here" || n0["End"] != -1.0 {
		t.Errorf("note 0 %v", n0)
	}
	if n1["Seconds"] != 600.0 || n1["End"] != 630.0 || n1["Stamp"] != "0:10:00-0:10:30" {
		t.Errorf("note 1 %v", n1)
	}

	// A note between the two goes between them; one after both after the loop.
	if r := mediaCall(t, PostMediaNote, "POST", "/media/note", map[string]interface{}{"Hash": sec.Hash, "Seconds": 300, "Text": "middle"}); r["Ok"] != true {
		t.Fatalf("note: %v", r)
	}
	_, sec = secAt(t, p, "Lecture")
	if r := mediaCall(t, PostMediaNote, "POST", "/media/note", map[string]interface{}{"Hash": sec.Hash, "Seconds": 700, "End": 710, "Text": "late\nsecond line"}); r["Ok"] != true {
		t.Fatalf("note: %v", r)
	}
	// By file and line, on a heading with no notes yet.
	f, plain := secAt(t, p, "Plain")
	if r := mediaCall(t, PostMediaNote, "POST", "/media/note", map[string]interface{}{"File": f.Filename, "Line": plain.Headline.Pos.Row + 1, "Media": "talk.mp3", "Seconds": 3}); r["Ok"] != true {
		t.Fatalf("note by line: %v", r)
	}
	b, _ := os.ReadFile(p)
	s := string(b)
	for _, w := range []string{
		"  carried on here\n- [[video:lecture.mp4#0:05:00][0:05:00]] middle\n- [[video:lecture.mp4#0:10:00-0:10:30]",
		"a loop\n- [[video:lecture.mp4#0:11:40-0:11:50][0:11:40-0:11:50]] late\n  second line\n** Child",
		"#+end_src\n- [[audio:talk.mp3#0:00:03][0:00:03]]\n",
	} {
		if !strings.Contains(s, w) {
			t.Errorf("file lacks %q:\n%s", w, s)
		}
	}

	// Start: a new heading with the property, under a parent.
	_, lin := secAt(t, p, "Linked only")
	if r := mediaCall(t, PostMediaStart, "POST", "/media/start", map[string]interface{}{"Under": lin.Hash, "Media": filepath.Join(filepath.Dir(p), "talk.mp3")}); r["Ok"] != true {
		t.Fatalf("start: %v", r)
	}
	b, _ = os.ReadFile(p)
	if !strings.Contains(string(b), "online\n** talk\n:PROPERTIES:\n:MEDIA: talk.mp3\n:END:\n* Voice") {
		t.Errorf("start wrote:\n%s", b)
	}
	if r := mediaCall(t, PostMediaStart, "POST", "/media/start", map[string]interface{}{"File": p, "Media": "nothere.mp4"}); r["Ok"] == true {
		t.Errorf("started on a missing file")
	}

	// The file server half: only a recording the file names.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/media/file?file="+url.QueryEscape(p)+"&path="+url.QueryEscape(filepath.Join(filepath.Dir(p), "talk.mp3")), nil)
	req.Header.Set("Range", "bytes=0-0")
	RequestMediaFile(rec, req)
	if rec.Code != http.StatusPartialContent || rec.Header().Get("Content-Type") != "audio/mpeg" {
		t.Errorf("media file: %d %v", rec.Code, rec.Header())
	}
	rec = httptest.NewRecorder()
	RequestMediaFile(rec, httptest.NewRequest("GET", "/media/file?file="+url.QueryEscape(p)+"&path="+url.QueryEscape(p), nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("served the org file itself: %d", rec.Code)
	}
}

func TestMediaPropTarget(t *testing.T) {
	cases := map[string][2]string{
		"lecture.mp4":                       {"video", "lecture.mp4"},
		"[[file:voice/a.webm]]":             {"video", "voice/a.webm"},
		"[[audio:talk.mp3#0:01:00][x]]":     {"audio", "talk.mp3"},
		"cite:smith2020":                    {"videocite", "smith2020"},
		"@smith2020":                        {"videocite", "smith2020"},
		"https://youtu.be/dQw4w9WgXcQ":      {"video", "https://youtu.be/dQw4w9WgXcQ"},
		"[[https://vimeo.com/76979871]]":    {"video", "https://vimeo.com/76979871"},
	}
	for in, want := range cases {
		typ, target := mediaPropTarget(in, "")
		if typ != want[0] || target != want[1] {
			t.Errorf("mediaPropTarget(%q) = %q %q, want %v", in, typ, target, want)
		}
	}
	if typ, _ := mediaPropTarget("[[file:voice/a.webm]]", "audio"); typ != "audio" {
		t.Errorf(":AUDIO: read as %s", typ)
	}
	if f := bibMediaFile(":/talks/x.mp4:MP4;Slides:/talks/x.pdf:PDF"); f != "/talks/x.mp4" {
		t.Errorf("bib file %q", f)
	}
}
