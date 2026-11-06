package jcanvas

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// A board as another app writes it, with extras a plugin added: every byte of meaning
// must survive a parse and a write.
const sampleBoard = `{
	"nodes":[
		{"id":"8132d4d894c80022","type":"text","text":"# Plan\n\n**bold** and [a link](https://x.org)","x":-180,"y":-220,"width":250,"height":160,"color":"4","styleAttributes":{"shape":"pill"}},
		{"id":"0ba6ef0e2a6b0a3d","type":"file","file":"notes/plans.org","subpath":"#^5f0c2b7e-1d1a-4a65-9f4b-3c2d1e0f9a8b","x":140,"y":-220,"width":400,"height":300},
		{"id":"a1","type":"link","url":"https://jsoncanvas.org","x":10.5,"y":200,"width":300,"height":120},
		{"id":"g","type":"group","label":"Ideas","background":"bg.png","backgroundStyle":"cover","x":-220,"y":-260,"width":800,"height":620,"color":"#ff00aa"}
	],
	"edges":[
		{"id":"e1","fromNode":"8132d4d894c80022","fromSide":"right","toNode":"0ba6ef0e2a6b0a3d","toSide":"left","color":"2","label":"leads to"},
		{"id":"e2","fromNode":"a1","fromEnd":"arrow","toNode":"8132d4d894c80022","toEnd":"none","pluginData":{"k":1}}
	],
	"metadata":{"version":"1.0-1.0"}
}
`

func TestRoundTripKeepsEverything(t *testing.T) {
	c, err := Parse([]byte(sampleBoard))
	if err != nil {
		t.Fatal(err)
	}
	if p, _ := c.Validate(); len(p) > 0 {
		t.Fatalf("valid canvas refused: %v", p)
	}
	out := c.Marshal()
	if string(out) != sampleBoard {
		t.Errorf("round trip changed the file:\n%s", out)
	}
	// And the meaning, through a generic decode.
	var a, b interface{}
	json.Unmarshal([]byte(sampleBoard), &a)
	json.Unmarshal(out, &b)
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	if string(ja) != string(jb) {
		t.Errorf("meaning changed")
	}
}

func TestEmptyAndNewNodes(t *testing.T) {
	c, err := Parse([]byte("{}"))
	if err != nil || len(c.Nodes) != 0 {
		t.Fatal(err)
	}
	c.Nodes = append(c.Nodes, NewText("", 0, 0, 250, 60))
	c.Nodes[0].ID = c.UniqueID()
	if p, _ := c.Validate(); len(p) > 0 {
		t.Fatalf("new text node refused: %v", p)
	}
	if !strings.Contains(string(c.Marshal()), `"text":""`) {
		t.Errorf("an empty text must still be written: %s", c.Marshal())
	}
}

func TestValidateRefuses(t *testing.T) {
	bad := `{"nodes":[{"id":"a","type":"text","x":0,"y":0,"width":1,"height":1},
		{"id":"a","type":"file","file":"","x":0,"y":0,"width":1,"height":1,"color":"9"},
		{"id":"b","type":"link","x":0,"y":0,"width":1},
		{"id":"c","type":"widget","x":0,"y":0,"width":1,"height":1}],
		"edges":[{"id":"e","fromNode":"a","toNode":"zz","toSide":"middle","toEnd":"diamond"}]}`
	c, err := Parse([]byte(bad))
	if err != nil {
		t.Fatal(err)
	}
	p, w := c.Validate()
	want := []string{"needs text", "used twice", "needs a file", "colour \"9\"", "needs a url", "no height", "is no node", "side \"middle\"", "end \"diamond\""}
	all := strings.Join(p, "\n")
	for _, x := range want {
		if !strings.Contains(all, x) {
			t.Errorf("no problem saying %q in:\n%s", x, all)
		}
	}
	if len(w) != 1 || !strings.Contains(w[0], "widget") {
		t.Errorf("an unknown type is a warning, not a refusal: %v", w)
	}
}

func TestParseRef(t *testing.T) {
	cases := []struct {
		file, sub string
		want      Ref
	}{
		{"a.org", "", Ref{Path: "a.org"}},
		{"a.org", "#^abc", Ref{Path: "a.org", ID: "abc"}},
		{"a.org", "#id:abc", Ref{Path: "a.org", ID: "abc"}},
		{"a.org", "#Plans#Next year", Ref{Path: "a.org", Olp: []string{"Plans", "Next year"}}},
		{"a.org::*Plans", "", Ref{Path: "a.org", Olp: []string{"Plans"}}},
		{"a.org::#plans", "", Ref{Path: "a.org", CustomID: "plans"}},
		{"a.org::42", "", Ref{Path: "a.org", Line: 42}},
		{"a.org::id:x", "", Ref{Path: "a.org", ID: "x"}},
	}
	for _, c := range cases {
		got := ParseRef(c.file, c.sub)
		if strings.Join(got.Olp, "/") != strings.Join(c.want.Olp, "/") || got.Path != c.want.Path || got.ID != c.want.ID ||
			got.CustomID != c.want.CustomID || got.Line != c.want.Line {
			t.Errorf("ParseRef(%q, %q) = %+v, want %+v", c.file, c.sub, got, c.want)
		}
	}
}

type geomCases struct {
	Colors []struct{ In, Out string }
	Sides  []struct {
		A, B     [4]float64
		From, To string
	}
	Curves []struct {
		A, B             [4]float64
		FromSide, ToSide string
		P1, C1, C2, P2   [2]float64
	}
	Groups struct {
		Nodes   json.RawMessage
		GroupOf map[string]string
	}
	Place []struct {
		Nodes  json.RawMessage
		W, H   float64
		Within string
		At     [2]float64
	}
}

func rect(a [4]float64) Rect { return Rect{a[0], a[1], a[2], a[3]} }

// The same file is read by worg's canvas.test.ts (a copy in worg/src).
func TestGeometryCases(t *testing.T) {
	b, err := os.ReadFile("testdata/geom_cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var g geomCases
	if err := json.Unmarshal(b, &g); err != nil {
		t.Fatal(err)
	}
	for _, c := range g.Colors {
		if got := ColorHex(c.In); got != c.Out {
			t.Errorf("ColorHex(%q) = %q, want %q", c.In, got, c.Out)
		}
	}
	for _, c := range g.Sides {
		f, to := AutoSides(rect(c.A), rect(c.B))
		if f != c.From || to != c.To {
			t.Errorf("AutoSides(%v, %v) = %s %s, want %s %s", c.A, c.B, f, to, c.From, c.To)
		}
	}
	for _, c := range g.Curves {
		cv := EdgeCurve(rect(c.A), rect(c.B), c.FromSide, c.ToSide)
		got := [][2]float64{{cv.P1.X, cv.P1.Y}, {cv.C1.X, cv.C1.Y}, {cv.C2.X, cv.C2.Y}, {cv.P2.X, cv.P2.Y}}
		want := [][2]float64{c.P1, c.C1, c.C2, c.P2}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("EdgeCurve(%v, %v) point %d = %v, want %v", c.A, c.B, i, got[i], want[i])
			}
		}
	}
	cv, err := Parse([]byte(`{"nodes":` + string(g.Groups.Nodes) + `}`))
	if err != nil {
		t.Fatal(err)
	}
	for i, n := range cv.Nodes {
		want := g.Groups.GroupOf[n.ID]
		got := ""
		if j := cv.GroupOf(i); j >= 0 {
			got = cv.Nodes[j].ID
		}
		if got != want {
			t.Errorf("GroupOf(%s) = %q, want %q", n.ID, got, want)
		}
	}
	for _, c := range g.Place {
		cv, err := Parse([]byte(`{"nodes":` + string(c.Nodes) + `}`))
		if err != nil {
			t.Fatal(err)
		}
		within := -1
		for i, n := range cv.Nodes {
			if n.ID == c.Within && c.Within != "" {
				within = i
			}
		}
		at := cv.Place(c.W, c.H, within)
		if at.X != c.At[0] || at.Y != c.At[1] {
			t.Errorf("Place in %s = %v, want %v", c.Nodes, at, c.At)
		}
	}
}

func TestOutlineAndSvg(t *testing.T) {
	c, _ := Parse([]byte(sampleBoard))
	cards := map[string]Card{"0ba6ef0e2a6b0a3d": {Kind: "heading", Title: "Plans", Keyword: "TODO", Tags: []string{"work"}, Plain: "Do the thing"}}
	out := Outline(&c, cards)
	for _, want := range []string{"[Ideas] (#ff00aa)", "  - \"Plan\" (green)", "  - TODO Plans  :work:  (plans.org)", "  - link: https://jsoncanvas.org",
		"Plan -- leads to --> TODO Plans  (orange)", "https://jsoncanvas.org <-- Plan"} {
		if !strings.Contains(out, want) {
			t.Errorf("outline has no %q:\n%s", want, out)
		}
	}
	svg := SVG(&c, cards, SvgOpts{IDPrefix: "t"})
	for _, want := range []string{"<svg", "Ideas", "leads to", "TODO", "Plans", "Do the thing", `fill="#44cf6e"`, "</svg>"} {
		if !strings.Contains(svg, want) {
			t.Errorf("svg has no %q", want)
		}
	}
	if strings.Contains(svg, "**") {
		t.Errorf("markdown marks drawn in the svg")
	}
}

func TestPlainText(t *testing.T) {
	got := strings.Join(PlainText("# Title\n\n**bold** [x](http://y) [[file:a.org][desc]] [[Note|alias]] *org* /it/\n```\ncode **kept**\n```\n"), "|")
	want := "Title||bold x desc alias org it|code **kept**"
	if got != want {
		t.Errorf("PlainText = %q, want %q", got, want)
	}
}

// Two cards side by side joined top to top: the curve loops above both, and
// the picture must hold it (and its label), not just the cards.
func TestSvgBoundsHoldCurves(t *testing.T) {
	c := Canvas{
		Nodes: []Node{{ID: "a", Type: "text", Text: "a", X: 0, Y: 0, Width: 100, Height: 50}, {ID: "b", Type: "text", Text: "b", X: 120, Y: 0, Width: 100, Height: 50}},
		Edges: []Edge{{ID: "e", FromNode: "a", FromSide: "top", ToNode: "b", ToSide: "top", Label: "a rather long label here"}},
	}
	b := drawnBounds(&c)
	// The curve's top is at y = -45 (controls at -60); the label sits there.
	if b.Y > -55 {
		t.Errorf("curve cut off: %+v", b)
	}
	if w := labelWidth("a rather long label here"); b.X > 110-w/2+0.01 || b.X+b.W < 110+w/2-0.01 {
		t.Errorf("label cut off: %+v", b)
	}
	if cb := c.Bounds(); cb.Y != 0 {
		t.Errorf("Bounds (for Place) moved: %+v", cb)
	}
}
