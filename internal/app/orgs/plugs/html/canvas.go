package htmlexp

// A `[[file:board.canvas]]` link drawn as the canvas itself (a JSON Canvas
// file), inline as an SVG with the link under it - only while
// the server has features.canvas on. The SVG is inline rather than an <img>
// pointing back at the server, so an exported page on disk shows it too.

import (
	"fmt"
	"html"
	"strings"

	"github.com/ihdavids/go-org/org"
)

// Set by the server (canvas.go): the canvas a link names drawn as an SVG,
// or false when the feature is off or there is no such canvas.
var CanvasEmbed func(docPath, target string) (svg string, ok bool)

func isCanvasLink(l org.RegularLink) bool {
	if l.Protocol != "file" && l.Protocol != "" {
		return false
	}
	t := strings.ToLower(LinkTarget(l.URL))
	if i := strings.Index(t, "::"); i >= 0 {
		t = t[:i]
	}
	return strings.HasSuffix(t, ".canvas")
}

func (w *OrgHtmlWriter) writeCanvasLink(l org.RegularLink) bool {
	if CanvasEmbed == nil || w.noCanvasEmbed || !isCanvasLink(l) {
		return false
	}
	docPath := ""
	if w.SrcDoc != nil {
		docPath = w.SrcDoc.Path
	} else if w.Document != nil {
		docPath = w.Document.Path
	}
	target := LinkTarget(l.URL)
	svg, ok := CanvasEmbed(docPath, target)
	if !ok {
		return false
	}
	caption := html.EscapeString(target)
	if l.Description != nil {
		caption = w.WriteNodesAsString(l.Description...)
	}
	// Spans, not a <figure>: the link sits inside a paragraph, and a block
	// element there would end it.
	w.WriteString(fmt.Sprintf(`<span class="org-canvas" data-canvas="%s" style="display:block;margin:1em 0;max-width:100%%;overflow:auto">%s<span class="org-canvas-caption" style="display:block;font-size:0.85em;opacity:0.75">%s</span></span>`,
		html.EscapeString(target), svg, caption))
	return true
}
