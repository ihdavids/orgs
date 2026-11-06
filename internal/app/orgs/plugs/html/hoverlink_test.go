package htmlexp

import (
	"strings"
	"testing"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

func TestHoverLinkSpans(t *testing.T) {
	render := func() string {
		src := "see [[id:budget-1][it]], [[*Plan]] and [[file:cat.png]]\n* Plan\n"
		doc := org.New().Parse(strings.NewReader(src), "/org/main.org")
		w := NewOrgHtmlWriter(&OrgHtmlExporter{Props: map[string]interface{}{}, pm: &common.PluginManager{}})
		w.Document = doc
		org.WriteNodes(w, doc.Nodes...)
		return w.String()
	}
	old := HoverLinksOn
	t.Cleanup(func() { HoverLinksOn = old })
	HoverLinksOn = func() bool { return false }
	if h := render(); strings.Contains(h, "org-link") {
		t.Fatalf("spans while off: %s", h)
	}
	HoverLinksOn = func() bool { return true }
	h := render()
	for _, want := range []string{
		`<span class="org-link" data-org-link="id:budget-1" data-org-from="/org/main.org"><a href="id:budget-1">it</a></span>`,
		`data-org-link="*Plan"`,
	} {
		if !strings.Contains(h, want) {
			t.Fatalf("no %s in %s", want, h)
		}
	}
	// A picture drawn in the page is not wrapped.
	if strings.Count(h, "data-org-link") != 2 {
		t.Fatalf("picture wrapped: %s", h)
	}
}
