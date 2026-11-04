package common

import (
	"strings"
	"testing"
	"time"
)

func TestOrgCaptureTemplates(t *testing.T) {
	now := time.Date(2026, 11, 3, 14, 5, 0, 0, time.Local)
	cases := []struct{ in, want string }{
		{"* TODO %?\n  %i\n  %a", "* TODO {{CONTENT}}\n  {{initial|Initial text}}\n  {{annotation}}"},
		{"* %^{Who|Sam|Kim} called %U\n%\\1 again", "* {{who|Who|=Sam}} called {{now}}\n{{who}} again"},
		{"* Meeting %^{Topic}\n%^{EFFORT}p%?", "* Meeting {{topic|Topic}}\n:PROPERTIES:\n:EFFORT: {{effort|EFFORT}}\n:END:\n{{CONTENT}}"},
		{"* %:description\n%:link\n%:initial", "* {{description}}\n{{link}}\n{{initial}}"},
		{"* Journal %<%Y-%m-%d %H:%M> 100%%", "* Journal 2026-11-03 14:05 100%"},
		{"* %t %T %u %^g", "* {{today}} {{active_now}} {{inactive_today}} {{tags|Tags}}"},
	}
	for _, c := range cases {
		if !IsOrgCaptureTemplate(c.in) {
			t.Errorf("%q not taken as an org template", c.in)
		}
		if got := OrgCaptureToOrgs(c.in, now); got != c.want {
			t.Errorf("%q:\n got %q\nwant %q", c.in, got, c.want)
		}
	}
	for _, t2 := range []string{"* {{title}} %?", "* Plain heading", "* 50% done"} {
		if IsOrgCaptureTemplate(t2) {
			t.Errorf("%q taken as an org template", t2)
		}
	}
	// The result is a template orgs fills.
	got := FillCapTemplate(OrgCaptureToOrgs("* %^{Who} %?", now), map[string]string{"who": "Sam", "CONTENT": "rang"})
	if !strings.Contains(got, "* Sam rang") {
		t.Errorf("filled: %q", got)
	}
}
