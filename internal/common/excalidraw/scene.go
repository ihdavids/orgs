// Package excalidraw reads and writes Excalidraw drawings the way the
// Excalidraw app (excalidraw.com, @excalidraw/excalidraw) and its markdown
// form (`.excalidraw.md`) keep them, and draws a plain SVG of one for exporters
// that cannot run Excalidraw's own JavaScript.
//
// Two kinds of file:
//
//   - `.excalidraw`: the scene as json, {"type": "excalidraw", "version": 2,
//     "source", "elements", "appState", "files"}.
//   - `.excalidraw.md`: the markdown wrapper. Front matter with
//     `excalidraw-plugin: parsed`, a `## Text Elements` section listing each
//     text as `words ^elementId`, an optional `## Embedded Files` section
//     (`fileId: [[picture.png]]`), and the scene in a ```compressed-json
//     (lz-string) or ```json block under `## Drawing`, between `%%` lines.
//
// Pure: bytes in, bytes out. The server (internal/app/orgs/excalidraw.go)
// does the files.
package excalidraw

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// Limits on what a drawing may be. A scene with pictures pasted into it is
// large (each picture is a data url), so the cap is generous.
const (
	MaxSceneBytes = 32 << 20
	MaxElements   = 200000
)

// Element is the part of an Excalidraw element the renderer and the
// checks read. Everything else in the json is kept by keeping the json.
type Element struct {
	ID              string      `json:"id"`
	Type            string      `json:"type"`
	X               float64     `json:"x"`
	Y               float64     `json:"y"`
	Width           float64     `json:"width"`
	Height          float64     `json:"height"`
	Angle           float64     `json:"angle"`
	StrokeColor     string      `json:"strokeColor"`
	BackgroundColor string      `json:"backgroundColor"`
	FillStyle       string      `json:"fillStyle"`
	StrokeWidth     float64     `json:"strokeWidth"`
	StrokeStyle     string      `json:"strokeStyle"`
	Opacity         *float64    `json:"opacity"`
	Roundness       *Roundness  `json:"roundness"`
	IsDeleted       bool        `json:"isDeleted"`
	Points          [][]float64 `json:"points"`
	StartArrowhead  *string     `json:"startArrowhead"`
	EndArrowhead    *string     `json:"endArrowhead"`
	Elbowed         bool        `json:"elbowed"`
	Polygon         bool        `json:"polygon"`
	Text            string      `json:"text"`
	OriginalText    string      `json:"originalText"`
	FontSize        float64     `json:"fontSize"`
	FontFamily      int         `json:"fontFamily"`
	TextAlign       string      `json:"textAlign"`
	VerticalAlign   string      `json:"verticalAlign"`
	LineHeight      float64     `json:"lineHeight"`
	ContainerID     *string     `json:"containerId"`
	FileID          *string     `json:"fileId"`
	Name            *string     `json:"name"`
	Link            *string     `json:"link"`
	Scale           []float64   `json:"scale"`
}

// Roundness is null for sharp corners.
type Roundness struct {
	Type  int      `json:"type"`
	Value *float64 `json:"value"`
}

// FileData is a picture pasted into a scene.
type FileData struct {
	ID       string `json:"id"`
	MimeType string `json:"mimeType"`
	DataURL  string `json:"dataURL"`
	Created  int64  `json:"created,omitempty"`
}

// Scene is the drawing.
type Scene struct {
	Type     string                 `json:"type"`
	Version  int                    `json:"version"`
	Source   string                 `json:"source"`
	Elements []Element              `json:"elements"`
	AppState map[string]interface{} `json:"appState"`
	Files    map[string]FileData    `json:"files"`
}

// Live is the elements not deleted.
func (s *Scene) Live() []Element {
	out := []Element{}
	for _, e := range s.Elements {
		if !e.IsDeleted {
			out = append(out, e)
		}
	}
	return out
}

// Background is the canvas colour the drawing was made on.
func (s *Scene) Background() string {
	if s.AppState != nil {
		if v, ok := s.AppState["viewBackgroundColor"].(string); ok && v != "" {
			return v
		}
	}
	return "#ffffff"
}

// ParseScene reads and checks a scene's json: an object of type
// "excalidraw" with a list of elements, every element an object with a type.
func ParseScene(b []byte) (*Scene, error) {
	if len(b) > MaxSceneBytes {
		return nil, fmt.Errorf("a drawing may be at most %d MB", MaxSceneBytes>>20)
	}
	t := bytes.TrimSpace(b)
	if len(t) == 0 || t[0] != '{' {
		return nil, fmt.Errorf("a drawing has to be an Excalidraw scene (a json object)")
	}
	var s Scene
	if err := json.Unmarshal(t, &s); err != nil {
		return nil, fmt.Errorf("that is not an Excalidraw scene: %s", err)
	}
	if s.Type != "excalidraw" {
		return nil, fmt.Errorf("that is not an Excalidraw scene: its type is %q, not \"excalidraw\"", s.Type)
	}
	if s.Elements == nil {
		// Present but null, or missing: a scene always has the list.
		var probe map[string]json.RawMessage
		json.Unmarshal(t, &probe)
		if raw, ok := probe["elements"]; !ok || string(bytes.TrimSpace(raw)) != "[]" {
			return nil, fmt.Errorf("that is not an Excalidraw scene: it has no list of elements")
		}
	}
	if len(s.Elements) > MaxElements {
		return nil, fmt.Errorf("a drawing may have at most %d elements", MaxElements)
	}
	for i, e := range s.Elements {
		if e.Type == "" {
			return nil, fmt.Errorf("element %d of the drawing has no type", i+1)
		}
	}
	for id, f := range s.Files {
		if f.DataURL != "" && !strings.HasPrefix(f.DataURL, "data:") {
			return nil, fmt.Errorf("picture %s of the drawing is not a data url", id)
		}
	}
	return &s, nil
}

// Empty is a new drawing's json, as excalidraw.com saves one.
func Empty(source string) []byte {
	if source == "" {
		source = "https://github.com/ihdavids/orgs"
	}
	b, _ := json.MarshalIndent(map[string]interface{}{
		"type":     "excalidraw",
		"version":  2,
		"source":   source,
		"elements": []interface{}{},
		"appState": map[string]interface{}{"gridSize": nil, "viewBackgroundColor": "#ffffff"},
		"files":    map[string]interface{}{},
	}, "", "  ")
	return append(b, '\n')
}

// Pretty is the json indented the way excalidraw.com writes a file, keys in
// the order they came.
func Pretty(b []byte) []byte {
	var out bytes.Buffer
	if err := json.Indent(&out, bytes.TrimSpace(b), "", "  "); err != nil {
		return b
	}
	out.WriteByte('\n')
	return out.Bytes()
}

// IsDrawingName says whether a file name is one of the two kinds.
func IsDrawingName(name string) bool {
	l := strings.ToLower(name)
	return strings.HasSuffix(l, ".excalidraw") || strings.HasSuffix(l, ".excalidraw.md")
}

// IsMarkdownName is the markdown kind.
func IsMarkdownName(name string) bool {
	return strings.HasSuffix(strings.ToLower(name), ".excalidraw.md")
}

// PictureBase is the name a drawing's exported picture goes by, without
// the extension: `diagram.excalidraw` for both `diagram.excalidraw` and
// `diagram.excalidraw.md`. org-excalidraw (Emacs) and excalidraw.com's
// "embed scene" export both write `diagram.excalidraw.svg`.
func PictureBase(name string) string {
	if IsMarkdownName(name) {
		return name[:len(name)-3]
	}
	return name
}
