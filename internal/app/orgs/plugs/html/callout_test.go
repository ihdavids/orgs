package htmlexp

import (
	"strings"
	"testing"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

const calloutSrc = "* A\n#+begin_warning Mind *the* gap :fold closed\nText.\n#+end_warning\n#+begin_tip\nTip.\n#+end_tip\n#+begin_details x\nPlain.\n#+end_details\n"

func writeCalloutDoc(on bool) string {
	common.CalloutsOn = func() bool { return on }
	defer func() { common.CalloutsOn = nil }()
	doc := org.New().Parse(strings.NewReader(calloutSrc), "test.org")
	w := NewOrgHtmlWriter(&OrgHtmlExporter{Props: map[string]interface{}{}, pm: &common.PluginManager{}})
	w.SrcDoc = doc
	org.WriteNodes(w, doc.Nodes...)
	return w.String()
}

func TestCalloutsInHtml(t *testing.T) {
	off := writeCalloutDoc(false)
	if strings.Contains(off, "callout") || !strings.Contains(off, `<div class="warning-block">`) {
		t.Errorf("off: want go-org's special block, got:\n%s", off)
	}
	on := writeCalloutDoc(true)
	for _, want := range []string{
		`<details class="callout callout-warning is-collapsible" data-callout="warning">`,
		`<summary class="callout-title"><span class="callout-icon" aria-hidden="true">⚠</span><span class="callout-title-inner">Mind <strong>the</strong> gap</span></summary>`,
		`<div class="callout callout-tip" data-callout="tip">`,
		`<span class="callout-title-inner">Tip</span>`,
		`<div class="details-block">`,
	} {
		if !strings.Contains(on, want) {
			t.Errorf("on: missing %s in:\n%s", want, on)
		}
	}
}
