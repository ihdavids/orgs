package excalidraw

import (
	"encoding/base64"
	"encoding/xml"
	"io"
	"regexp"
	"strings"
	"testing"
)

// Made with lz-string 1.5.0's compressToBase64 in node.
func TestDecompressFromBase64(t *testing.T) {
	cases := []struct{ plain, packed string }{
		{"", "Q==="},
		{"a", "IZA="},
		{"hello hello hello", "BYUwNmD2AEoTcpA="},
		{`{"type":"excalidraw","version":2,"elements":[{"id":"x1","type":"text","text":"Héllo ✓ 😀"}]}`,
			"N4IgLgngDgpiBcIYA8DGBDANgSwCYCd0B3EAGhADcZ8BnbAewDsEAmcmTGAWxkbBoQBtUHgQhkARjLhocRGBRhpC5EsQAJAJeZM9AASBkcj2BeDcAAeyAC+AXQtA"},
		{strings.Repeat("abcabcabcabcabcabcabcabcabcabcabcabcabcabcabc", 20),
			"IYIwxqHpPXUNo+TUvczase33BOR+xhJ5Zlp1FNVtD9TdLjrzbnH37vXfPfkMEiBY4eNETpooA==="},
	}
	for _, c := range cases {
		got, err := DecompressFromBase64(c.packed)
		if err != nil || got != c.plain {
			t.Errorf("%q: got %q, %v; want %q", c.packed, got, err, c.plain)
		}
		// The block is broken into lines.
		if len(c.packed) > 20 {
			split := c.packed[:10] + "\n" + c.packed[10:20] + "\r\n" + c.packed[20:]
			if got, _ := DecompressFromBase64(split); got != c.plain {
				t.Errorf("split %q: got %q", c.packed, got)
			}
		}
	}
	if _, err := DecompressFromBase64("!!!!notlz"); err == nil {
		t.Errorf("garbage decompressed")
	}
}

const sample = `{
  "type": "excalidraw",
  "version": 2,
  "source": "https://excalidraw.com",
  "elements": [
    {"id": "r1", "type": "rectangle", "x": 0, "y": 0, "width": 200, "height": 100, "angle": 0,
     "strokeColor": "#1e1e1e", "backgroundColor": "#a5d8ff", "fillStyle": "hachure", "strokeWidth": 2,
     "strokeStyle": "solid", "roundness": {"type": 3}, "opacity": 100, "isDeleted": false},
    {"id": "t1", "type": "text", "x": 50, "y": 40, "width": 100, "height": 25, "text": "Héllo <world>",
     "originalText": "Héllo <world>", "fontSize": 20, "fontFamily": 5, "textAlign": "center",
     "strokeColor": "#1e1e1e", "containerId": "r1", "lineHeight": 1.25},
    {"id": "a1", "type": "arrow", "x": 200, "y": 50, "width": 100, "height": 0, "points": [[0,0],[100,0]],
     "strokeColor": "#e03131", "strokeWidth": 2, "strokeStyle": "dashed", "endArrowhead": "arrow", "startArrowhead": null},
    {"id": "e1", "type": "ellipse", "x": 300, "y": 0, "width": 80, "height": 100, "angle": 0.5,
     "strokeColor": "#000", "backgroundColor": "transparent", "strokeWidth": 1},
    {"id": "f1", "type": "freedraw", "x": 0, "y": 150, "points": [[0,0],[10,5],[20,0],[30,8]], "strokeColor": "#000", "strokeWidth": 1},
    {"id": "gone", "type": "rectangle", "x": 9000, "y": 9000, "width": 5, "height": 5, "isDeleted": true},
    {"id": "i1", "type": "image", "x": 0, "y": 200, "width": 10, "height": 10, "fileId": "pic"}
  ],
  "appState": {"viewBackgroundColor": "#ffffff"},
  "files": {"pic": {"id": "pic", "mimeType": "image/png", "dataURL": "data:image/png;base64,iVBORw0KGgo="}}
}`

func TestParseScene(t *testing.T) {
	s, err := ParseScene([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Live()) != 6 {
		t.Errorf("live elements: %d", len(s.Live()))
	}
	bad := []string{
		``, `[]`, `{"type":"excalidrawlib","elements":[]}`, `{"type":"excalidraw"}`,
		`{"type":"excalidraw","elements":[{"id":"x"}]}`,
		`{"type":"excalidraw","elements":[],"files":{"a":{"dataURL":"https://x/y.png"}}}`,
	}
	for _, b := range bad {
		if _, err := ParseScene([]byte(b)); err == nil {
			t.Errorf("%q passed", b)
		}
	}
	if _, err := ParseScene(Empty("")); err != nil {
		t.Errorf("an empty drawing does not read: %v", err)
	}
}

func TestRender(t *testing.T) {
	s, _ := ParseScene([]byte(sample))
	svg := Render(s, []byte(sample))
	// Well formed.
	dec := xml.NewDecoder(strings.NewReader(svg))
	counts := map[string]int{}
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("not xml: %v\n%s", err, svg)
		}
		if se, ok := tok.(xml.StartElement); ok {
			counts[se.Name.Local]++
		}
	}
	for el, n := range map[string]int{"rect": 2, "ellipse": 1, "pattern": 1, "image": 1, "tspan": 1} {
		if counts[el] != n {
			t.Errorf("%d <%s>, want %d\n%s", counts[el], el, n, svg)
		}
	}
	for _, want := range []string{
		"Héllo &lt;world&gt;", `text-anchor="middle"`, `stroke-dasharray="8 10"`, `rx="25"`,
		`transform="rotate(28.65 340 50)"`, "url(#orgs-fill-0)", "payload-type:application/vnd.excalidraw+json",
	} {
		if !strings.Contains(svg, want) {
			t.Errorf("missing %q\n%s", want, svg)
		}
	}
	if strings.Contains(svg, "9000") {
		t.Errorf("a deleted element was drawn or counted")
	}
	// The scene rides along in the form excalidraw.com reads back.
	m := regexp.MustCompile(`<!-- payload-start -->(.+?)<!-- payload-end -->`).FindStringSubmatch(svg)
	if m == nil {
		t.Fatal("no payload")
	}
	if b, _ := base64.StdEncoding.DecodeString(m[1]); string(b) != sample {
		t.Errorf("payload does not decode to the scene")
	}
	// The box: the freehand stroke's width left of 0, padded 10; the turned
	// ellipse above 0; the image's bottom at 210, padded.
	if !strings.HasPrefix(svg, `<svg xmlns="http://www.w3.org/2000/svg" version="1.1" viewBox="-12 -23.06 421.07 243.06"`) {
		t.Errorf("box: %s", svg[:120])
	}
	// An empty drawing is still a picture.
	e, _ := ParseScene(Empty(""))
	if out := Render(e, nil); !strings.Contains(out, "<svg") {
		t.Errorf("empty: %s", out)
	}
}

const markdownFile = "---\n\nexcalidraw-plugin: parsed\ntags: [excalidraw]\n\n---\n==⚠  Switch to EXCALIDRAW VIEW ⚠==\n\n" +
	"# Excalidraw Data\n\n## Text Elements\nold words ^t1\n\nstale ^gone\n\n" +
	"## Embedded Files\nabc123: [[pics/cat.png|cat]]\n\n%%\n## Drawing\n```compressed-json\n" +
	"N4IgLgngDgpiBcIYA8DGBDANgSwCYCd0B3EAGhADcZ8BnbAewDsEAmcmTGAWxkbBoQBtUHgQhkARjLhocRGBRhpC5EsQAJAJeZM9AASBkcj2BeDcAAeyAC+AXQtA" +
	"\n```\n%%"

func TestMarkdownDrawing(t *testing.T) {
	js, info, err := ReadMarkdown(markdownFile)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Compressed || !strings.Contains(string(js), "Héllo ✓ 😀") {
		t.Errorf("read: %s %+v", js, info)
	}
	if EmbeddedTarget(info.Embedded["abc123"]) != "pics/cat.png" {
		t.Errorf("embedded: %+v", info.Embedded)
	}
	scene := `{"type":"excalidraw","version":2,"elements":[{"id":"t1","type":"text","text":"new\nwords","originalText":"new\nwords"}],` +
		`"files":{"abc123":{"id":"abc123","mimeType":"image/png","dataURL":"data:image/png;base64,AA=="},"keep":{"id":"keep","dataURL":"data:image/png;base64,AA=="}}}`
	out, err := WriteMarkdown(markdownFile, []byte(scene))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"## Text Elements\nnew\nwords ^t1\n\n## Embedded Files\nabc123: [[pics/cat.png|cat]]", "```json\n{", "excalidraw-plugin: parsed", "\"keep\"", "\n```\n%%"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	if strings.Contains(out, "stale ^gone") || strings.Contains(out, "compressed-json") || strings.Contains(out, `"abc123":`) {
		t.Errorf("left behind:\n%s", out)
	}
	// And it reads back.
	js2, _, err := ReadMarkdown(out)
	if err != nil {
		t.Fatal(err)
	}
	if s, err := ParseScene(js2); err != nil || len(s.Elements) != 1 {
		t.Errorf("read back: %v", err)
	}
	// A new file is one the reader reads.
	if js3, _, err := ReadMarkdown(NewMarkdown(Empty(""))); err != nil {
		t.Error(err)
	} else if _, err := ParseScene(js3); err != nil {
		t.Error(err)
	}
}

func TestNames(t *testing.T) {
	if !IsDrawingName("a/B.Excalidraw") || !IsDrawingName("x.excalidraw.md") || IsDrawingName("x.md") || IsDrawingName("x.excalidraw.svg") {
		t.Error("IsDrawingName")
	}
	if PictureBase("d.excalidraw.md") != "d.excalidraw" || PictureBase("d.excalidraw") != "d.excalidraw" {
		t.Error("PictureBase")
	}
}
