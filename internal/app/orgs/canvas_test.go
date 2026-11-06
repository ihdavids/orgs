package orgs

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ihdavids/orgs/internal/common"
	"github.com/ihdavids/orgs/internal/common/jcanvas"
)

func canvasSetup(t *testing.T, files map[string]string) map[string]string {
	t.Helper()
	if Conf().Server == nil {
		t.Skip("no server settings")
	}
	withFeatures(t, func(f *common.FeatureSettings) { f.Canvas = true })
	paths := loadOrg(t, files)
	var dir string
	for _, p := range paths {
		dir = filepath.Dir(p)
	}
	old := Conf().Server.OrgDirs
	Conf().Server.OrgDirs = []string{dir}
	t.Cleanup(func() { Conf().Server.OrgDirs = old })
	return paths
}

func canvasCall(t *testing.T, h http.HandlerFunc, method, url string, body interface{}) canvasAnswer {
	t.Helper()
	var r *http.Request
	if body != nil {
		b, _ := json.Marshal(body)
		r = httptest.NewRequest(method, url, bytes.NewReader(b))
	} else {
		r = httptest.NewRequest(method, url, nil)
	}
	w := httptest.NewRecorder()
	h(w, r)
	var a canvasAnswer
	if err := json.Unmarshal(w.Body.Bytes(), &a); err != nil {
		t.Fatalf("%s %s: %v\n%s", method, url, err, w.Body.String())
	}
	return a
}

// A heading put on a board gets an :ID: (in column zero, where this file
// keeps its drawers), is named by it, and is still found after it is
// renamed and refiled into another file.
func TestCanvasHeadingFollowsItsId(t *testing.T) {
	paths := canvasSetup(t, map[string]string{
		"plans.org": "* Projects\n** TODO Write the report :work:\n:PROPERTIES:\n:EFFORT: 1:00\n:END:\nThe report.\n** Other\n",
		"work.org":  "* Work\n",
	})
	board := filepath.Join(filepath.Dir(paths["plans.org"]), "boards", "plan.canvas")
	if a := canvasCall(t, PostCanvasNew, "POST", "/canvas/new", map[string]string{"File": "boards/plan"}); !a.Ok {
		t.Fatalf("new: %+v", a)
	}
	if a := canvasCall(t, PostCanvasNew, "POST", "/canvas/new", map[string]string{"File": "boards/plan"}); a.Ok {
		t.Fatalf("a second new should be refused")
	}
	_, sec := secAt(t, paths["plans.org"], "Projects", "Write the report")
	a := canvasCall(t, PostCanvasAdd, "POST", "/canvas/add", map[string]interface{}{"File": "plan", "Kind": "heading", "Hash": sec.Hash, "Color": "green"})
	if !a.Ok {
		t.Fatalf("add: %+v", a)
	}
	got, _ := os.ReadFile(paths["plans.org"])
	if !strings.Contains(string(got), ":EFFORT:   1:00\n:ID:       ") {
		t.Fatalf("no :ID: in the drawer:\n%s", got)
	}
	var node map[string]interface{}
	json.Unmarshal(a.Node, &node)
	if node["file"] != "plans.org" || !strings.HasPrefix(node["subpath"].(string), "#^") || node["color"] != "4" {
		t.Fatalf("node %v", node)
	}
	id := strings.TrimPrefix(node["subpath"].(string), "#^")

	// Renamed and moved to work.org.
	text := strings.Replace(string(got), "** TODO Write the report :work:", "** DONE Write the final report :work:", 1)
	cut := strings.Index(text, "** DONE")
	end := strings.Index(text, "** Other")
	moved := text[cut:end]
	os.WriteFile(paths["plans.org"], []byte(text[:cut]+text[end:]), 0644)
	os.WriteFile(paths["work.org"], []byte("* Work\n"+moved), 0644)
	GetDb().LoadFile(paths["plans.org"], true)
	GetDb().LoadFile(paths["work.org"], true)

	r := canvasCall(t, RequestCanvas, "GET", "/canvas?file=plan&html=f", nil)
	if !r.Ok {
		t.Fatalf("read: %+v", r)
	}
	card := r.Cards[node["id"].(string)]
	if card.Kind != "heading" || card.Title != "Write the final report" || !card.Moved || card.File != "work.org" {
		t.Fatalf("card after the move: %+v", card)
	}

	// And the board counts as a link to it.
	if n := BacklinksTo(card.Hash); n != 1 {
		t.Errorf("backlinks from the board: %d", n)
	}
	if !strings.Contains(string(canvasRead(t, board)), id) {
		t.Errorf("the board does not name the id")
	}
}

func canvasRead(t *testing.T, p string) []byte {
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestCanvasSaveKeepsExtrasAndRefusesStale(t *testing.T) {
	paths := canvasSetup(t, map[string]string{"a.org": "* A\n"})
	p := filepath.Join(filepath.Dir(paths["a.org"]), "b.canvas")
	os.WriteFile(p, []byte(`{"nodes":[{"id":"n","type":"text","text":"# Hi","x":0,"y":0,"width":10,"height":10,"plugin":{"k":1}}],"edges":[],"metadata":{"v":1}}`), 0644)
	r := canvasCall(t, RequestCanvas, "GET", "/canvas?file=b.canvas", nil)
	if !r.Ok || r.Cards["n"].Kind != "text" || !strings.Contains(r.Cards["n"].Html, "Hi") {
		t.Fatalf("%+v", r)
	}
	var doc map[string]interface{}
	json.Unmarshal(r.Canvas, &doc)
	doc["nodes"].([]interface{})[0].(map[string]interface{})["x"] = 50
	if a := canvasCall(t, PostCanvasSave, "POST", "/canvas/save", map[string]interface{}{"File": "b", "Canvas": doc, "Base": "stale"}); a.Ok || !a.Conflict {
		t.Fatalf("a stale save went through: %+v", a)
	}
	if a := canvasCall(t, PostCanvasSave, "POST", "/canvas/save", map[string]interface{}{"File": "b", "Canvas": doc, "Base": r.Version}); !a.Ok {
		t.Fatalf("save: %+v", a)
	}
	got := string(canvasRead(t, p))
	for _, want := range []string{`"x":50`, `"plugin":{"k":1}`, `"metadata":{"v":1}`} {
		if !strings.Contains(got, want) {
			t.Errorf("no %s in\n%s", want, got)
		}
	}
	doc["edges"] = []interface{}{map[string]interface{}{"id": "e", "fromNode": "n", "toNode": "gone"}}
	if a := canvasCall(t, PostCanvasSave, "POST", "/canvas/save", map[string]interface{}{"File": "b", "Canvas": doc}); a.Ok || len(a.Problems) == 0 {
		t.Fatalf("an edge to nothing was saved: %+v", a)
	}
}

func TestCanvasOffDoesNothing(t *testing.T) {
	if Conf().Server == nil {
		t.Skip("no server settings")
	}
	withFeatures(t, func(f *common.FeatureSettings) { f.Canvas = false })
	w := httptest.NewRecorder()
	RequestCanvasList(w, httptest.NewRequest("GET", "/canvas/list", nil))
	if !strings.Contains(w.Body.String(), `"Off":true`) {
		t.Fatalf("%s", w.Body.String())
	}
	if _, ok := canvasEmbed("", "x.canvas"); ok {
		t.Fatalf("drawn while off")
	}
}

func TestCardDialect(t *testing.T) {
	for text, want := range map[string]string{
		"# Plan\n**bold**":        "markdown",
		"[a](https://x.org)":      "markdown",
		"* Plan\n*bold*":          "org",
		"[[https://x.org][a]]":    "org",
		"plain words":             "org",
		"# Heading\n#+TITLE: x":   "org",
		"> quote\n\n```\ncode\n```": "markdown",
	} {
		if got := CardDialect(text); got != want {
			t.Errorf("CardDialect(%q) = %s, want %s", text, got, want)
		}
	}
	_ = jcanvas.Card{}
}

// A backlink from a board points at the line its node starts on, however
// the json is laid out.
func TestCanvasNodeLine(t *testing.T) {
	pretty := strings.Split("{\n\t\"nodes\": [\n\t\t{\n\t\t\t\"id\": \"a\",\n\t\t\t\"type\": \"text\"\n\t\t},\n\t\t{\n\t\t\t\"id\": \"b\",\n\t\t\t\"type\": \"file\"\n\t\t}\n\t]\n}", "\n")
	if got := canvasNodeLine(pretty, "b"); got != 6 {
		t.Errorf("pretty: line %d", got)
	}
	compact := strings.Split("{\n\t\"nodes\":[\n\t\t{\"id\":\"a\",\"type\":\"text\"},\n\t\t{\"id\":\"b\",\"type\":\"file\"}\n\t]\n}", "\n")
	if got := canvasNodeLine(compact, "b"); got != 3 {
		t.Errorf("compact: line %d", got)
	}
	if got := canvasNodeLine(compact, "zz"); got != 0 {
		t.Errorf("missing: line %d", got)
	}
}
