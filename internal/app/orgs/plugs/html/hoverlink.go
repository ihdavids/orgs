package htmlexp

// Hover previews (features.hoverEditor, the server's preview.go): while the
// feature is on every link is wrapped in a span naming the link as written
// and the org file it is in, which is how worg finds what a rendered link
// points at (the href alone has lost it: [[*Title]] is #heading-3 by now).
// Off, the page is byte for byte what it was.

import (
	"fmt"
	"html"
	"path/filepath"

	"github.com/ihdavids/go-org/org"
)

// HoverLinksOn is set by the server (preview.go's init); nil is off.
var HoverLinksOn func() bool

func (w *OrgHtmlWriter) hoverLinkOpen(l org.RegularLink) string {
	if HoverLinksOn == nil || !HoverLinksOn() {
		return ""
	}
	// A picture drawn in the page is itself, not a link to preview.
	if l.Description == nil && (l.Kind() == "image" || l.Kind() == "video") {
		return ""
	}
	from := ""
	if w.Document != nil && w.Document.Path != "" && filepath.IsAbs(w.Document.Path) {
		from = fmt.Sprintf(` data-org-from="%s"`, html.EscapeString(w.Document.Path))
	}
	return fmt.Sprintf(`<span class="org-link" data-org-link="%s"%s>`, html.EscapeString(l.URL), from)
}
