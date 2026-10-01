package orgs

import (
	"strings"
	"testing"

	"github.com/ihdavids/orgs/internal/common"
)

// The language itself is pinned in internal/common/captemplate_test.go. This is
// the wiring: that a list on its way to a client comes back expanded, one
// capture at a time.
func TestFillCaptureTemplateAutosIsPerTemplate(t *testing.T) {
	temps := []common.CaptureTemplate{
		{Name: "A", Template: ":CUSTOM_ID: {{uuid}}\n{{CONTENT}}"},
		{Name: "B", Template: ":CUSTOM_ID: {{uuid}}\n{{CONTENT}}"},
		{Name: "C", Template: ""},
	}
	out := FillCaptureTemplateAutos(temps, "ian")

	for _, o := range out[:2] {
		// The name survives, so a client may still answer it itself.
		if !strings.Contains(o.Template, "{{uuid|=") {
			t.Errorf("%s was not expanded: %q", o.Name, o.Template)
		}
		if !strings.Contains(o.Template, "{{CONTENT}}") {
			t.Errorf("%s lost its content placeholder: %q", o.Name, o.Template)
		}
	}
	if out[0].Template == out[1].Template {
		t.Errorf("two templates got the same uuid:\n%s", out[0].Template)
	}
	// A template with no string of its own is still a headline and a body.
	if out[2].Template != "" {
		t.Errorf("an empty template grew something: %q", out[2].Template)
	}
}
