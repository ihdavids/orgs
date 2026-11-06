// Package jcanvas reads, checks and writes JSON Canvas 1.0 documents - the
// `.canvas` files (https://jsoncanvas.org/spec/1.0/) -
// and draws one as a static SVG. Pure: bytes in, bytes out. The server
// (internal/app/orgs/canvas.go) resolves the cards that name org files and
// headings; the CLI reads the wire types.
//
// Round trips keep what this package does not know about: every key of a
// node, an edge or the document that is not in the spec is kept in Extra and
// written back after the known ones, so the extras other apps and their plugins
// add (`styleAttributes`, a plugin's metadata, a future node type's fields)
// survive a save from here. Numbers are kept as float64, so a tool that writes
// 10.5 gets 10.5 back.
//
// The writer lays a document out the usual way: tab-indented, one node
// or edge to a line, keys in the spec's order. A diff of a canvas in git then
// reads as "this card moved".
package jcanvas

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Node is one card or group.
type Node struct {
	ID     string
	Type   string // text, file, link, group (anything else is kept, drawn as a box)
	X, Y   float64
	Width  float64
	Height float64
	Color  string // "" | "1".."6" | "#rrggbb"

	Text            string // text
	File            string // file
	Subpath         string // file: "#Heading", "#^id"
	URL             string // link
	Label           string // group
	Background      string // group
	BackgroundStyle string // group: cover | ratio | repeat

	// Keys the spec does not name, as written.
	Extra map[string]json.RawMessage
	// Which optional keys were present (so `"subpath": ""` round trips).
	has map[string]bool
}

// Edge is one arrow (or line) between two nodes.
type Edge struct {
	ID       string
	FromNode string
	FromSide string // top right bottom left, "" = facing the other node
	FromEnd  string // none (default) | arrow
	ToNode   string
	ToSide   string
	ToEnd    string // arrow (default) | none
	Color    string
	Label    string
	Extra    map[string]json.RawMessage
	has      map[string]bool
}

// Canvas is a whole document.
type Canvas struct {
	Nodes []Node
	Edges []Edge
	Extra map[string]json.RawMessage
}

var (
	NodeTypes        = []string{"text", "file", "link", "group"}
	Sides            = []string{"top", "right", "bottom", "left"}
	Ends             = []string{"none", "arrow"}
	BackgroundStyles = []string{"cover", "ratio", "repeat"}
)

// The six preset colours. The spec leaves the shades to the application;
// these are the common ones, so a canvas looks the same here as elsewhere.
// Twin: CANVAS_PRESETS in worg/src/canvas.ts (TestColorHex / canvas.test.ts).
var Presets = map[string]string{
	"1": "#fb464c", // red
	"2": "#e9973f", // orange
	"3": "#e0de71", // yellow
	"4": "#44cf6e", // green
	"5": "#53dfdd", // cyan
	"6": "#a882ff", // purple
}

// PresetNames are the presets by name, for a person typing `-color green`.
var PresetNames = map[string]string{"red": "1", "orange": "2", "yellow": "3", "green": "4", "cyan": "5", "purple": "6"}

var hexColorRe = regexp.MustCompile(`^#(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$`)

// ColorHex is the css colour of a canvasColor ("" for none or nonsense).
func ColorHex(c string) string {
	c = strings.TrimSpace(c)
	if p, ok := Presets[c]; ok {
		return p
	}
	if hexColorRe.MatchString(c) {
		return strings.ToLower(c)
	}
	return ""
}

// ColorOk is whether a canvasColor is one the spec allows.
func ColorOk(c string) bool { return c == "" || ColorHex(c) != "" }

// NormColor reads what a person typed (`green`, `4`, `#44cf6e`, `44cf6e`).
func NormColor(c string) (string, bool) {
	c = strings.TrimSpace(strings.ToLower(c))
	if c == "" {
		return "", true
	}
	if n, ok := PresetNames[c]; ok {
		return n, true
	}
	if !strings.HasPrefix(c, "#") && hexColorRe.MatchString("#"+c) {
		c = "#" + c
	}
	return c, ColorOk(c)
}

// NewID is a fresh id the usual shape: 16 hex digits.
func NewID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%016x", math.Float64bits(math.Pi))
	}
	return hex.EncodeToString(b)
}

// UniqueID is a fresh id no node or edge of c already has.
func (c *Canvas) UniqueID() string {
	for {
		id := NewID()
		if c.Node(id) == nil && c.Edge(id) == nil {
			return id
		}
	}
}

func (c *Canvas) Node(id string) *Node {
	for i := range c.Nodes {
		if c.Nodes[i].ID == id {
			return &c.Nodes[i]
		}
	}
	return nil
}

func (c *Canvas) Edge(id string) *Edge {
	for i := range c.Edges {
		if c.Edges[i].ID == id {
			return &c.Edges[i]
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Reading
// ---------------------------------------------------------------------------

// Parse reads a document. An empty file is an empty canvas (a new
// canvas is often written as `{}`). Only json that is not a canvas at all is an error;
// what the spec would refuse is for Validate to say.
func Parse(b []byte) (Canvas, error) {
	var c Canvas
	if len(bytes.TrimSpace(b)) == 0 {
		return c, nil
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(b, &top); err != nil {
		return c, fmt.Errorf("not a canvas: %v", err)
	}
	for k, v := range top {
		switch k {
		case "nodes":
			var raw []map[string]json.RawMessage
			if !isNull(v) {
				if err := json.Unmarshal(v, &raw); err != nil {
					return c, fmt.Errorf("nodes: %v", err)
				}
			}
			for i, m := range raw {
				n, err := nodeFrom(m)
				if err != nil {
					return c, fmt.Errorf("node %d: %v", i+1, err)
				}
				c.Nodes = append(c.Nodes, n)
			}
		case "edges":
			var raw []map[string]json.RawMessage
			if !isNull(v) {
				if err := json.Unmarshal(v, &raw); err != nil {
					return c, fmt.Errorf("edges: %v", err)
				}
			}
			for i, m := range raw {
				e, err := edgeFrom(m)
				if err != nil {
					return c, fmt.Errorf("edge %d: %v", i+1, err)
				}
				c.Edges = append(c.Edges, e)
			}
		default:
			if c.Extra == nil {
				c.Extra = map[string]json.RawMessage{}
			}
			c.Extra[k] = v
		}
	}
	return c, nil
}

func isNull(v json.RawMessage) bool { return strings.TrimSpace(string(v)) == "null" }

func str(m map[string]json.RawMessage, k string, has map[string]bool) (string, error) {
	v, ok := m[k]
	if !ok {
		return "", nil
	}
	delete(m, k)
	has[k] = true
	if isNull(v) {
		return "", nil
	}
	var s string
	if err := json.Unmarshal(v, &s); err != nil {
		// A preset colour written as a number (`"color": 4`) is read as one.
		var f float64
		if json.Unmarshal(v, &f) == nil {
			return strconv.FormatFloat(f, 'f', -1, 64), nil
		}
		return "", fmt.Errorf("%s is not a string", k)
	}
	return s, nil
}

func num(m map[string]json.RawMessage, k string, has map[string]bool) (float64, error) {
	v, ok := m[k]
	if !ok {
		return 0, nil
	}
	delete(m, k)
	has[k] = true
	var f float64
	if err := json.Unmarshal(v, &f); err != nil {
		var s string
		if json.Unmarshal(v, &s) == nil {
			if g, err := strconv.ParseFloat(strings.TrimSpace(s), 64); err == nil {
				return g, nil
			}
		}
		return 0, fmt.Errorf("%s is not a number", k)
	}
	return f, nil
}

func nodeFrom(m map[string]json.RawMessage) (Node, error) {
	n := Node{has: map[string]bool{}}
	var err error
	strs := []struct {
		k string
		p *string
	}{{"id", &n.ID}, {"type", &n.Type}, {"color", &n.Color}, {"text", &n.Text}, {"file", &n.File},
		{"subpath", &n.Subpath}, {"url", &n.URL}, {"label", &n.Label}, {"background", &n.Background},
		{"backgroundStyle", &n.BackgroundStyle}}
	for _, s := range strs {
		if *s.p, err = str(m, s.k, n.has); err != nil {
			return n, err
		}
	}
	nums := []struct {
		k string
		p *float64
	}{{"x", &n.X}, {"y", &n.Y}, {"width", &n.Width}, {"height", &n.Height}}
	for _, s := range nums {
		if *s.p, err = num(m, s.k, n.has); err != nil {
			return n, err
		}
	}
	if len(m) > 0 {
		n.Extra = m
	}
	return n, nil
}

func edgeFrom(m map[string]json.RawMessage) (Edge, error) {
	e := Edge{has: map[string]bool{}}
	var err error
	strs := []struct {
		k string
		p *string
	}{{"id", &e.ID}, {"fromNode", &e.FromNode}, {"fromSide", &e.FromSide}, {"fromEnd", &e.FromEnd},
		{"toNode", &e.ToNode}, {"toSide", &e.ToSide}, {"toEnd", &e.ToEnd}, {"color", &e.Color}, {"label", &e.Label}}
	for _, s := range strs {
		if *s.p, err = str(m, s.k, e.has); err != nil {
			return e, err
		}
	}
	if len(m) > 0 {
		e.Extra = m
	}
	return e, nil
}

// FromEndOr / ToEndOr are the ends with the spec's defaults filled in.
func (e Edge) FromEndOr() string {
	if e.FromEnd == "" {
		return "none"
	}
	return e.FromEnd
}

func (e Edge) ToEndOr() string {
	if e.ToEnd == "" {
		return "arrow"
	}
	return e.ToEnd
}

// ---------------------------------------------------------------------------
// Checking
// ---------------------------------------------------------------------------

func in(s string, set []string) bool {
	for _, x := range set {
		if s == x {
			return true
		}
	}
	return false
}

// Validate says what the spec would refuse (Problems) and what is merely
// odd (Warnings: a node type the spec does not name, kept as it is).
func (c *Canvas) Validate() (problems []string, warnings []string) {
	ids := map[string]string{}
	for i, n := range c.Nodes {
		where := fmt.Sprintf("node %d", i+1)
		if n.ID == "" {
			problems = append(problems, where+": no id")
		} else {
			where = fmt.Sprintf("node %q", n.ID)
			if _, dup := ids[n.ID]; dup {
				problems = append(problems, where+": the id is used twice")
			}
			ids[n.ID] = "node"
		}
		switch n.Type {
		case "text":
			if !n.has["text"] {
				problems = append(problems, where+": a text node needs text")
			}
		case "file":
			if strings.TrimSpace(n.File) == "" {
				problems = append(problems, where+": a file node needs a file")
			}
			if n.Subpath != "" && !strings.HasPrefix(n.Subpath, "#") {
				problems = append(problems, where+": a subpath starts with #")
			}
		case "link":
			if strings.TrimSpace(n.URL) == "" {
				problems = append(problems, where+": a link node needs a url")
			}
		case "group":
			if n.BackgroundStyle != "" && !in(n.BackgroundStyle, BackgroundStyles) {
				problems = append(problems, fmt.Sprintf("%s: backgroundStyle %q is not cover, ratio or repeat", where, n.BackgroundStyle))
			}
		case "":
			problems = append(problems, where+": no type")
		default:
			warnings = append(warnings, fmt.Sprintf("%s: type %q is not in JSON Canvas 1.0; kept as it is", where, n.Type))
		}
		for _, k := range []string{"x", "y", "width", "height"} {
			if !n.has[k] {
				problems = append(problems, fmt.Sprintf("%s: no %s", where, k))
			}
		}
		if n.Width < 0 || n.Height < 0 {
			problems = append(problems, where+": a negative size")
		}
		if !ColorOk(n.Color) {
			problems = append(problems, fmt.Sprintf("%s: colour %q is not 1-6 or #hex", where, n.Color))
		}
	}
	for i, e := range c.Edges {
		where := fmt.Sprintf("edge %d", i+1)
		if e.ID == "" {
			problems = append(problems, where+": no id")
		} else {
			where = fmt.Sprintf("edge %q", e.ID)
			if _, dup := ids[e.ID]; dup {
				problems = append(problems, where+": the id is used twice")
			}
			ids[e.ID] = "edge"
		}
		for _, end := range []struct{ name, id string }{{"fromNode", e.FromNode}, {"toNode", e.ToNode}} {
			if end.id == "" {
				problems = append(problems, fmt.Sprintf("%s: no %s", where, end.name))
			} else if c.Node(end.id) == nil {
				problems = append(problems, fmt.Sprintf("%s: %s %q is no node", where, end.name, end.id))
			}
		}
		for _, s := range []string{e.FromSide, e.ToSide} {
			if s != "" && !in(s, Sides) {
				problems = append(problems, fmt.Sprintf("%s: side %q is not top, right, bottom or left", where, s))
			}
		}
		for _, s := range []string{e.FromEnd, e.ToEnd} {
			if s != "" && !in(s, Ends) {
				problems = append(problems, fmt.Sprintf("%s: end %q is not none or arrow", where, s))
			}
		}
		if !ColorOk(e.Color) {
			problems = append(problems, fmt.Sprintf("%s: colour %q is not 1-6 or #hex", where, e.Color))
		}
	}
	return problems, warnings
}

// ---------------------------------------------------------------------------
// Writing
// ---------------------------------------------------------------------------

type kv struct {
	k string
	v interface{}
}

func writeObj(b *bytes.Buffer, pairs []kv, extra map[string]json.RawMessage) {
	b.WriteByte('{')
	first := true
	put := func(k string, raw []byte) {
		if !first {
			b.WriteByte(',')
		}
		first = false
		kb, _ := json.Marshal(k)
		b.Write(kb)
		b.WriteByte(':')
		b.Write(raw)
	}
	for _, p := range pairs {
		put(p.k, marshal(p.v))
	}
	keys := make([]string, 0, len(extra))
	for k := range extra {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		var compact bytes.Buffer
		if json.Compact(&compact, extra[k]) == nil {
			put(k, compact.Bytes())
		} else {
			put(k, extra[k])
		}
	}
	b.WriteByte('}')
}

// json.Marshal without html escaping: `<` and `&` in a card's text stay
// themselves.
func marshal(v interface{}) []byte {
	if f, ok := v.(float64); ok {
		return []byte(strconv.FormatFloat(f, 'f', -1, 64))
	}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.Encode(v)
	return bytes.TrimRight(b.Bytes(), "\n")
}

func (n Node) pairs() []kv {
	out := []kv{{"id", n.ID}, {"type", n.Type}}
	opt := func(k, v string) {
		if v != "" || n.has[k] {
			out = append(out, kv{k, v})
		}
	}
	switch n.Type {
	case "text":
		out = append(out, kv{"text", n.Text})
	case "file":
		out = append(out, kv{"file", n.File})
		opt("subpath", n.Subpath)
	case "link":
		out = append(out, kv{"url", n.URL})
	case "group":
		opt("label", n.Label)
		opt("background", n.Background)
		opt("backgroundStyle", n.BackgroundStyle)
	default:
		opt("text", n.Text)
		opt("file", n.File)
		opt("subpath", n.Subpath)
		opt("url", n.URL)
		opt("label", n.Label)
	}
	out = append(out, kv{"x", n.X}, kv{"y", n.Y}, kv{"width", n.Width}, kv{"height", n.Height})
	if n.Color != "" {
		out = append(out, kv{"color", n.Color})
	}
	return out
}

func (e Edge) pairs() []kv {
	out := []kv{{"id", e.ID}, {"fromNode", e.FromNode}}
	opt := func(k, v string) {
		if v != "" {
			out = append(out, kv{k, v})
		}
	}
	opt("fromSide", e.FromSide)
	opt("fromEnd", e.FromEnd)
	out = append(out, kv{"toNode", e.ToNode})
	opt("toSide", e.ToSide)
	opt("toEnd", e.ToEnd)
	opt("color", e.Color)
	opt("label", e.Label)
	return out
}

// Marshal writes the document in the usual layout.
func (c *Canvas) Marshal() []byte {
	var b bytes.Buffer
	b.WriteString("{\n\t\"nodes\":[")
	for i, n := range c.Nodes {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString("\n\t\t")
		writeObj(&b, n.pairs(), n.Extra)
	}
	if len(c.Nodes) > 0 {
		b.WriteString("\n\t")
	}
	b.WriteString("],\n\t\"edges\":[")
	for i, e := range c.Edges {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString("\n\t\t")
		writeObj(&b, e.pairs(), e.Extra)
	}
	if len(c.Edges) > 0 {
		b.WriteString("\n\t")
	}
	b.WriteString("]")
	keys := make([]string, 0, len(c.Extra))
	for k := range c.Extra {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		kb, _ := json.Marshal(k)
		b.WriteString(",\n\t")
		b.Write(kb)
		b.WriteByte(':')
		var compact bytes.Buffer
		if json.Compact(&compact, c.Extra[k]) == nil {
			b.Write(compact.Bytes())
		} else {
			b.Write(c.Extra[k])
		}
	}
	b.WriteString("\n}\n")
	return b.Bytes()
}

// NewText, NewFile, NewLink and NewGroup make a node with the presence
// flags set, so it writes as a parsed one would.
func newNode(typ string, x, y, w, h float64) Node {
	return Node{Type: typ, X: x, Y: y, Width: w, Height: h,
		has: map[string]bool{"x": true, "y": true, "width": true, "height": true}}
}

func NewText(text string, x, y, w, h float64) Node {
	n := newNode("text", x, y, w, h)
	n.Text, n.has["text"] = text, true
	return n
}

func NewFile(file, subpath string, x, y, w, h float64) Node {
	n := newNode("file", x, y, w, h)
	n.File, n.Subpath = file, subpath
	return n
}

func NewLink(url string, x, y, w, h float64) Node {
	n := newNode("link", x, y, w, h)
	n.URL = url
	return n
}

func NewGroup(label string, x, y, w, h float64) Node {
	n := newNode("group", x, y, w, h)
	n.Label = label
	return n
}
