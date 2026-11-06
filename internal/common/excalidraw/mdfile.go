package excalidraw

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// Excalidraw's markdown file (`.excalidraw.md`). Read: the scene in
// the ```compressed-json or ```json block, and the pictures its Embedded
// Files section names. Written back: the scene as a ```json block (the
// plugin reads both, and compresses again on its own next save if it is set
// to), and the Text Elements section made again from the scene - the plugin
// takes text from that section over the json's, so leaving it stale would
// undo every text edit made here.

var (
	obsFenceRe    = regexp.MustCompile("^```(compressed-json|json)\\s*$")
	obsCloseRe    = regexp.MustCompile("^```\\s*$")
	obsDrawingRe  = regexp.MustCompile(`^#{1,6}\s+Drawing\s*$`)
	obsTextRe     = regexp.MustCompile(`^#{1,6}\s+Text Elements\s*$`)
	obsEmbeddedRe = regexp.MustCompile(`(?i)^#{1,6}\s+Embedded files\s*$`)
	obsHeadRe     = regexp.MustCompile(`^#{1,6}\s`)
	obsEmbedRe    = regexp.MustCompile(`^([0-9A-Za-z_-]+):\s*(.+?)\s*$`)
)

// MarkdownInfo is what the markdown says besides the scene.
type MarkdownInfo struct {
	// Embedded is the Embedded Files section: file id → what it names,
	// `[[picture.png]]`, a url, or `$$latex$$`.
	Embedded map[string]string
	// Compressed says the scene was in a compressed-json block.
	Compressed bool
}

// drawingBlock finds the scene's fence: the first json block after a
// Drawing heading, else the first json block in the file. It answers the
// fence lines (open, close) or -1.
func drawingBlock(lines []string) (open, close int, kind string) {
	start := 0
	for i, l := range lines {
		if obsDrawingRe.MatchString(l) {
			start = i
			break
		}
	}
	for pass := 0; pass < 2; pass++ {
		for i := start; i < len(lines); i++ {
			m := obsFenceRe.FindStringSubmatch(strings.TrimRight(lines[i], "\r"))
			if m == nil {
				continue
			}
			for j := i + 1; j < len(lines); j++ {
				if obsCloseRe.MatchString(strings.TrimRight(lines[j], "\r")) {
					return i, j, m[1]
				}
			}
			return -1, -1, ""
		}
		start = 0
	}
	return -1, -1, ""
}

// ReadMarkdown takes the scene's json out of an `.excalidraw.md` file.
func ReadMarkdown(text string) ([]byte, *MarkdownInfo, error) {
	lines := strings.Split(text, "\n")
	open, close, kind := drawingBlock(lines)
	if open < 0 {
		return nil, nil, fmt.Errorf("no ```json or ```compressed-json drawing block in this .excalidraw.md file")
	}
	body := strings.Join(lines[open+1:close], "\n")
	info := &MarkdownInfo{Embedded: map[string]string{}, Compressed: kind == "compressed-json"}
	var js string
	if info.Compressed {
		s, err := DecompressFromBase64(body)
		if err != nil {
			return nil, nil, err
		}
		js = s
	} else {
		js = body
	}
	in := false
	for _, l := range lines[:open] {
		l = strings.TrimRight(l, "\r")
		switch {
		case obsEmbeddedRe.MatchString(l):
			in = true
		case obsHeadRe.MatchString(l) || strings.HasPrefix(l, "%%"):
			in = false
		case in:
			if m := obsEmbedRe.FindStringSubmatch(l); m != nil {
				info.Embedded[m[1]] = m[2]
			}
		}
	}
	return []byte(js), info, nil
}

// EmbeddedTarget is the file an Embedded Files entry names, `[[a/b.png|x]]`
// → `a/b.png`, or "" when it is not a file (a url, latex).
func EmbeddedTarget(v string) string {
	v = strings.TrimSpace(v)
	if !strings.HasPrefix(v, "[[") || !strings.HasSuffix(v, "]]") {
		return ""
	}
	v = v[2 : len(v)-2]
	if i := strings.Index(v, "|"); i >= 0 {
		v = v[:i]
	}
	return strings.TrimSpace(v)
}

// textElementLines is the Text Elements section's body for a scene: each
// text as written, then ` ^id`, a blank line after each (the plugin's own
// layout).
func textElementLines(s *Scene) []string {
	out := []string{}
	for _, e := range s.Live() {
		if e.Type != "text" {
			continue
		}
		t := e.OriginalText
		if t == "" {
			t = e.Text
		}
		out = append(out, t+" ^"+e.ID, "")
	}
	return out
}

// WriteMarkdown is the markdown with the scene put back: the drawing block
// as ```json, the Text Elements made again, everything else (front matter,
// Embedded Files, the warning line, notes) as it was. Pictures the Embedded
// Files section names are taken out of the json's files: the plugin keeps
// them as vault files, and a data url beside the link would double them.
func WriteMarkdown(text string, scene []byte) (string, error) {
	s, err := ParseScene(scene)
	if err != nil {
		return "", err
	}
	_, info, err := ReadMarkdown(text)
	if err != nil {
		return "", err
	}
	if len(info.Embedded) > 0 {
		var m map[string]json.RawMessage
		if json.Unmarshal(scene, &m) == nil {
			var files map[string]json.RawMessage
			if json.Unmarshal(m["files"], &files) == nil {
				for id := range info.Embedded {
					delete(files, id)
				}
				m["files"], _ = json.Marshal(files)
				if b, err := json.Marshal(m); err == nil {
					scene = b
				}
			}
		}
	}
	lines := strings.Split(text, "\n")
	open, close, _ := drawingBlock(lines)
	pretty := strings.TrimSuffix(string(Pretty(scene)), "\n")
	out := append([]string{}, lines[:open]...)
	out = append(out, "```json")
	out = append(out, strings.Split(pretty, "\n")...)
	out = append(out, lines[close:]...)

	// Text Elements, when the file has the section. One without it (a very
	// old file) is left so; the plugin writes the section on its next save.
	for i, l := range out {
		if !obsTextRe.MatchString(strings.TrimRight(l, "\r")) {
			continue
		}
		end := i + 1
		for end < len(out) {
			t := strings.TrimRight(out[end], "\r")
			if obsHeadRe.MatchString(t) || strings.HasPrefix(t, "%%") {
				break
			}
			end++
		}
		body := textElementLines(s)
		if len(body) == 0 {
			body = []string{""}
		}
		repl := append([]string{}, out[:i+1]...)
		repl = append(repl, body...)
		repl = append(repl, out[end:]...)
		out = repl
		break
	}
	return strings.Join(out, "\n"), nil
}

// NewMarkdown is a new `.excalidraw.md` file around a scene, laid out as the
// plugin lays one out.
func NewMarkdown(scene []byte) string {
	return "---\n\nexcalidraw-plugin: parsed\ntags: [excalidraw]\n\n---\n" +
		"==⚠  Switch to EXCALIDRAW VIEW in the MORE OPTIONS menu of this document. ⚠== You can decompress Drawing data with the command palette: 'Decompress current Excalidraw file'. For more info check in plugin settings under 'Saving'\n\n" +
		"# Excalidraw Data\n\n## Text Elements\n%%\n## Drawing\n```json\n" + string(Pretty(scene)) + "```\n%%"
}
