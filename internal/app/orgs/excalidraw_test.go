package orgs

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func excalidrawTestDir(t *testing.T, on bool) string {
	t.Helper()
	dir := t.TempDir()
	oldDirs, oldOn := Conf().Server.OrgDirs, Conf().Server.Features.Excalidraw
	Conf().Server.OrgDirs = []string{dir}
	Conf().Server.Features.Excalidraw = on
	t.Cleanup(func() { Conf().Server.OrgDirs, Conf().Server.Features.Excalidraw = oldDirs, oldOn })
	return dir
}

func drawCall(t *testing.T, h http.HandlerFunc, method, url string, body interface{}) map[string]interface{} {
	t.Helper()
	var r *http.Request
	if body != nil {
		b, _ := json.Marshal(body)
		r = httptest.NewRequest(method, url, strings.NewReader(string(b)))
	} else {
		r = httptest.NewRequest(method, url, nil)
	}
	w := httptest.NewRecorder()
	h(w, r)
	out := map[string]interface{}{}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("%s %s: %v: %s", method, url, err, w.Body.String())
	}
	return out
}

func TestExcalidrawOffDoesNothing(t *testing.T) {
	dir := excalidrawTestDir(t, false)
	res := drawCall(t, PostExcalidrawNew, "POST", "/excalidraw/new", map[string]string{"File": "x"})
	if res["Off"] != true {
		t.Errorf("answered while off: %v", res)
	}
	if _, err := os.Stat(filepath.Join(dir, "x.excalidraw")); err == nil {
		t.Error("made a drawing while off")
	}
	lines := []string{"[[file:x.excalidraw]]"}
	if excalidrawMayApply(strings.Join(lines, "\n")) {
		t.Error("the export pass applies while off")
	}
}

func TestExcalidrawNewSaveListSvg(t *testing.T) {
	dir := excalidrawTestDir(t, true)
	res := drawCall(t, PostExcalidrawNew, "POST", "/excalidraw/new", map[string]string{"File": "drawings/flow"})
	if res["Ok"] != true {
		t.Fatalf("new: %v", res)
	}
	path := filepath.Join(dir, "drawings", "flow.excalidraw")
	if res["File"] != path || res["Link"] != "[[file:"+filepath.ToSlash(path)+"]]" {
		t.Errorf("new answered %v", res)
	}
	if again := drawCall(t, PostExcalidrawNew, "POST", "/excalidraw/new", map[string]string{"File": "drawings/flow"}); again["Ok"] == true {
		t.Error("made the same drawing twice")
	}
	if out := drawCall(t, PostExcalidrawNew, "POST", "/excalidraw/new", map[string]string{"File": "../escape"}); out["Ok"] == true {
		t.Error("made a drawing outside the org directories")
	}

	got := drawCall(t, RequestExcalidraw, "GET", "/excalidraw?file=drawings/flow.excalidraw", nil)
	if got["Ok"] != true || got["Format"] != "excalidraw" {
		t.Fatalf("get: %v", got)
	}
	mod := int64(got["Modified"].(float64))

	scene := json.RawMessage(`{"type":"excalidraw","version":2,"elements":[{"id":"r","type":"rectangle","x":0,"y":0,"width":50,"height":20,"strokeColor":"#000","backgroundColor":"transparent","strokeWidth":1}],"appState":{},"files":{}}`)
	bad := []ExcalidrawSave{
		{File: "drawings/flow.excalidraw", Scene: json.RawMessage(`{"type":"nope","elements":[]}`)},
		{File: "drawings/flow.excalidraw", Scene: scene, Svg: `<svg><script>alert(1)</script></svg>`},
		{File: "drawings/flow.excalidraw", Scene: scene, Svg: `<svg><image href="https://x/y.png"/></svg>`},
		{File: "drawings/flow.excalidraw", Scene: scene, Png: "aGVsbG8="},
		{File: "notes.org", Scene: scene},
		{File: "drawings/flow.excalidraw", Scene: scene, Modified: mod - 5000},
	}
	for i, b := range bad {
		if out := drawCall(t, PostExcalidrawSave, "POST", "/excalidraw/save", b); out["Ok"] == true {
			t.Errorf("bad save %d was taken", i)
		}
	}
	svg := `<svg xmlns="http://www.w3.org/2000/svg"><metadata><!-- payload-start -->e30=<!-- payload-end --></metadata>` +
		`<symbol id="image-a"><image href="data:image/png;base64,AA=="/></symbol><use href="#image-a"/>` +
		`<a href="https://example.com"><rect width="5" height="5"/></a></svg>`
	time.Sleep(10 * time.Millisecond)
	out := drawCall(t, PostExcalidrawSave, "POST", "/excalidraw/save", ExcalidrawSave{File: path, Scene: scene, Svg: svg, Modified: mod})
	if out["Ok"] != true {
		t.Fatalf("save: %v", out)
	}
	if b, _ := os.ReadFile(path + ".svg"); string(b) != svg {
		t.Errorf("svg beside it: %q", b)
	}
	if b, _ := os.ReadFile(path); !strings.Contains(string(b), "\n  \"elements\": [") {
		t.Errorf("scene not written indented: %s", b)
	}

	org := filepath.Join(dir, "notes.org")
	os.WriteFile(org, []byte("* Flow\n#+CAPTION: The flow\n[[file:drawings/flow.excalidraw]]\n#+begin_src org\n[[file:drawings/flow.excalidraw]]\n#+end_src\n"), 0644)
	list := drawCall(t, RequestExcalidrawList, "GET", "/excalidraw/list", nil)
	ds := list["Drawings"].([]interface{})
	if len(ds) != 1 {
		t.Fatalf("list: %v", list)
	}
	d := ds[0].(map[string]interface{})
	if d["Rel"] != "drawings/flow.excalidraw" || d["Name"] != "flow" || d["Fresh"] != true {
		t.Errorf("listed %v", d)
	}

	// The export copy: the fresh svg is used as it is; inside a src block the
	// link stays.
	lines := strings.Split(strings.TrimSuffix(mustRead(t, org), "\n"), "\n")
	ex, changed := excalidrawExportLines(org, lines, "html")
	if !changed || ex[2] != "[[file:drawings/flow.excalidraw.svg]]" || ex[4] != "[[file:drawings/flow.excalidraw]]" {
		t.Errorf("export copy: %q", ex)
	}
	// Stale: the drawing is newer than its picture, so the server draws one.
	later := time.Now().Add(time.Minute)
	os.Chtimes(path, later, later)
	if _, changed := excalidrawExportLines(org, lines, "html"); !changed {
		t.Error("stale drawing not linked")
	}
	if b := mustRead(t, path+".svg"); !strings.Contains(b, "drawn by orgs") {
		t.Errorf("stale picture not drawn again: %s", b)
	}
	// LaTeX without a png (and no rsvg-convert on the test machine's PATH
	// asked for): left as a link unless it can make one.
	t.Setenv("PATH", "")
	if ex, _ := excalidrawExportLines(org, lines, "latex"); ex[2] != lines[2] {
		t.Errorf("latex without a png: %q", ex[2])
	}

	w := httptest.NewRecorder()
	RequestExcalidrawSvg(w, httptest.NewRequest("GET", "/excalidraw/svg?file=/images/drawings/flow.excalidraw.svg&render=server", nil))
	if w.Code != 200 || w.Header().Get("X-Orgs-Drawn") != "server" || !strings.Contains(w.Body.String(), "<rect") {
		t.Errorf("svg: %d %s", w.Code, w.Body.String())
	}
}

func TestExcalidrawMarkdownSave(t *testing.T) {
	dir := excalidrawTestDir(t, true)
	res := drawCall(t, PostExcalidrawNew, "POST", "/excalidraw/new", map[string]string{"File": "Drawing.excalidraw.md"})
	if res["Ok"] != true {
		t.Fatalf("new: %v", res)
	}
	path := filepath.Join(dir, "Drawing.excalidraw.md")
	os.WriteFile(filepath.Join(dir, "cat.png"), []byte("\x89PNG\r\n\x1a\nxx"), 0644)
	md := strings.Replace(mustRead(t, path), "%%\n## Drawing", "## Embedded Files\nf1: [[cat.png]]\n\n%%\n## Drawing", 1)
	os.WriteFile(path, []byte(md), 0644)
	got := drawCall(t, RequestExcalidraw, "GET", "/excalidraw?file=Drawing.excalidraw.md", nil)
	if got["Format"] != "obsidian" {
		t.Fatalf("get: %v", got)
	}
	files := got["Scene"].(map[string]interface{})["files"].(map[string]interface{})
	if f, ok := files["f1"].(map[string]interface{}); !ok || !strings.HasPrefix(f["dataURL"].(string), "data:image/png;base64,") {
		t.Errorf("embedded picture not read: %v", files)
	}
	scene := json.RawMessage(`{"type":"excalidraw","version":2,"elements":[{"id":"t","type":"text","text":"hi","originalText":"hi"}],"files":{"f1":{"id":"f1","dataURL":"data:image/png;base64,AA=="}}}`)
	out := drawCall(t, PostExcalidrawSave, "POST", "/excalidraw/save", ExcalidrawSave{File: path, Scene: scene})
	if out["Ok"] != true {
		t.Fatalf("save: %v", out)
	}
	b := mustRead(t, path)
	if !strings.Contains(b, "## Text Elements\nhi ^t\n") || !strings.Contains(b, "f1: [[cat.png]]") || strings.Contains(b, `"f1": {`) {
		t.Errorf("saved:\n%s", b)
	}
}

func mustRead(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// "Linked from" counts a link with a description as well as a bare one.
func TestDrawingAnyLink(t *testing.T) {
	for _, c := range []struct{ line, want string }{
		{"[[file:bed-layout.excalidraw]]", "bed-layout.excalidraw"},
		{"see [[file:bed-layout.excalidraw][Bed layout]] here", "bed-layout.excalidraw"},
		{"[[excalidraw:d/flow.excalidraw][Flow]]", "d/flow.excalidraw"},
		{"[[plan.excalidraw.md]]", "plan.excalidraw.md"},
		{"[[file:bed.excalidraw.svg]]", ""},
	} {
		m := drawingAnyLinkRe.FindStringSubmatch(c.line)
		got := ""
		if m != nil {
			got = m[2]
		}
		if got != c.want {
			t.Errorf("%q: got %q, want %q", c.line, got, c.want)
		}
	}
}
