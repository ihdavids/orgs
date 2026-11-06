package htmlexp

// Callouts (features.callouts): a callout block is drawn as a titled box,
// a <details> when it folds; anything else goes to go-org's WriteBlock as
// before. The stylesheet is appended in GetStylesheet.

import (
	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

func (w *OrgHtmlWriter) WriteBlock(b org.Block) {
	if common.CalloutsActive() {
		if c, ok := common.CalloutOfBlock(b); ok {
			w.WriteString(common.CalloutHTMLWith(w.HTMLWriter, c, b))
			return
		}
	}
	w.HTMLWriter.WriteBlock(b)
}
