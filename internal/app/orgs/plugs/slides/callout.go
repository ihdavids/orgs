package slides

import (
	"strings"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

// Callout is a slide deck's callout block (features.callouts): the html
// exporter's box, with the stylesheet in front of the first one in the deck
// (a deck has its own stylesheet, which knows nothing of callouts). NOTE and
// NOTES stay speaker notes. ok is false for any other block, which the
// writer writes as before.
func Callout(w *org.HTMLWriter, b org.Block, styled *bool) (string, bool) {
	name := strings.ToUpper(b.Name)
	if name == "NOTE" || name == "NOTES" || !common.CalloutsActive() {
		return "", false
	}
	c, ok := common.CalloutOfBlock(b)
	if !ok {
		return "", false
	}
	out := common.CalloutHTMLWith(w, c, b)
	if !*styled {
		*styled = true
		out = "<style>" + common.CalloutCSS(false, false) + "</style>\n" + out
	}
	return out, true
}
