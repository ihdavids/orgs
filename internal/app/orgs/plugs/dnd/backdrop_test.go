// An end to end test of the backdrop: an org sheet naming a folder of
// pictures, rendered through the real html template, has to come out with
// both fade layers, the pictures inlined, and a cycle the page can run.
package dnd

import (
	b64 "encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
	"github.com/ihdavids/orgs/internal/templates"
	logging "gopkg.in/op/go-logging.v1"
)

// stubDb is an ODb that knows nothing: the sheet exporter only ever asks it
// to look up a file, and a character sheet given by path does not need it to.
type stubDb struct{}

func (stubDb) QueryTodosExpr(string) (common.Todos, error) { return common.Todos{}, nil }
func (stubDb) FindByAnyId(string) *common.Todo             { return nil }
func (stubDb) FindByHash(string) *common.Todo              { return nil }
func (stubDb) FindNextSibling(string) *common.Todo         { return nil }
func (stubDb) FindPrevSibling(string) *common.Todo         { return nil }
func (stubDb) FindLastChild(string) *common.Todo           { return nil }
func (stubDb) FindByFile(string) *org.Document             { return nil }
func (stubDb) GetFile(string) *common.OrgFile              { return nil }
func (stubDb) GetFromTarget(*common.Target, bool) (*common.OrgFile, *org.Section) {
	return nil, nil
}
func (stubDb) GetFromPreciseTarget(*common.PreciseTarget, org.NodeType) (*common.OrgFile, *org.Section, org.Node) {
	return nil, nil, nil
}

// onePixelPng is the smallest thing a browser will call a picture.
const onePixelPng = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAAC0lEQVR42mNk" +
	"YPhfDwAChwGA60e6kgAAAABJRU5ErkJggg=="

// sheetFor writes a character sheet naming the given backdrop, renders it
// through the real exporter and hands back the html.
func sheetFor(t *testing.T, backdrop string, props map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	art := filepath.Join(dir, "art")
	if err := os.MkdirAll(art, 0755); err != nil {
		t.Fatal(err)
	}
	png := decodePng(t)
	for _, name := range []string{"tavern.png", "woods.png"} {
		if err := os.WriteFile(filepath.Join(art, name), png, 0644); err != nil {
			t.Fatal(err)
		}
	}
	sheet := filepath.Join(dir, "lyra.org")
	org := fmt.Sprintf(`* Lyra
   :PROPERTIES:
   :DND_NAME:           Lyra
   :DND_RACE:           elf
   :DND_CLASSES:        druid::3
   :DND_LEVEL:          3
   :DND_BACKDROP:       %s
   :DND_BACKDROP_CYCLE: 4m-12m
   :END:
`, backdrop)
	if err := os.WriteFile(sheet, []byte(org), 0644); err != nil {
		t.Fatal(err)
	}

	// The exporter needs a template manager pointed at the repo's templates.
	root, err := filepath.Abs("../../../../../templates")
	if err != nil {
		t.Fatal(err)
	}
	tempo := &templates.TemplateManager{TemplatePath: root}
	tempo.Initialize()
	exp := &SheetExporter{Format: "html"}
	exp.Startup(&common.PluginManager{
		Out:   logging.MustGetLogger("test"),
		Tempo: tempo,
	}, &common.PluginOpts{})

	err, html := exp.ExportToString(stubDb{}, sheet, "", props)
	if err != nil {
		t.Fatalf("export: %s", err)
	}
	return html
}

func decodePng(t *testing.T) []byte {
	t.Helper()
	data, err := base64Decode(onePixelPng)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// backdropData reads back the list of pictures the page was given.
func backdropData(t *testing.T, html string) []string {
	t.Helper()
	const open = `<script type="application/json" id="dnd-backdrop-data">`
	i := strings.Index(html, open)
	if i < 0 {
		return nil
	}
	rest := html[i+len(open):]
	var list []string
	if err := json.Unmarshal([]byte(rest[:strings.Index(rest, "</script>")]), &list); err != nil {
		t.Fatalf("backdrop json: %s", err)
	}
	return list
}

// A folder is the point of the feature: it stands for every picture in it.
func TestHtmlSheetTakesABackdropFolder(t *testing.T) {
	html := sheetFor(t, "art", nil)
	list := backdropData(t, html)
	if len(list) != 2 {
		t.Fatalf("got %d backdrops, want the two in the folder", len(list))
	}
	for _, src := range list {
		if !strings.HasPrefix(src, "data:image/png;base64,") {
			t.Errorf("backdrop not inlined: %.40q", src)
		}
	}
	// Two layers, because changing picture is a crossfade.
	for _, id := range []string{`id="backdrop-a"`, `id="backdrop-b"`} {
		if !strings.Contains(html, id) {
			t.Errorf("no %s layer in the page", id)
		}
	}
	// The stat boxes turn to glass only when there is scenery behind them.
	if !strings.Contains(html, `class="page has-backdrop"`) {
		t.Error("the page does not ask its stat boxes to show the backdrop")
	}
	// 4m-12m, as the sheet asked for, in the milliseconds the page counts in.
	if !strings.Contains(html, "var minWait = 240 * 1000") ||
		!strings.Contains(html, "var maxWait = 720 * 1000") {
		t.Error("the page was not given the 4m-12m cycle the sheet asked for")
	}
}

// A sheet with no backdrop carries none of the machinery for one.
func TestHtmlSheetWithoutABackdrop(t *testing.T) {
	html := sheetFor(t, "", nil)
	if strings.Contains(html, `id="backdrop-a"`) ||
		strings.Contains(html, "dnd-backdrop-data") ||
		strings.Contains(html, `class="page has-backdrop"`) {
		t.Error("a sheet with no backdrop still carries the backdrop layers")
	}
}

// A backdrop handed to the export itself paints that one sheet, which is what
// "orgs dnd sheet -backdrop ..." does without touching the org file.
func TestExportBackdropOverridesTheSheet(t *testing.T) {
	html := sheetFor(t, "art/tavern.png", map[string]string{
		"backdrop":        "art/woods.png, https://example.com/keep.png",
		"backdropCycle":   "30s",
		"backdropOpacity": "25%",
	})
	list := backdropData(t, html)
	if len(list) != 2 || list[1] != "https://example.com/keep.png" {
		t.Fatalf("backdrops = %#v, want the two the export named", list)
	}
	if !strings.Contains(html, "var minWait = 30 * 1000") {
		t.Error("the export's own cycle was not used")
	}
	if !strings.Contains(html, "--wash: 0.25;") {
		t.Error("the export's own opacity was not used")
	}
}

func base64Decode(s string) ([]byte, error) { return b64.StdEncoding.DecodeString(s) }
