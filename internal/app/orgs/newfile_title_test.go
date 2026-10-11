package orgs

import "testing"

func TestTitleFromFilename(t *testing.T) {
	for in, want := range map[string]string{
		"/a/snippets.org":       "Snippets",
		"q3_plan-notes.org":     "Q3 plan notes",
		"/a/links.org":          "Links",
		"/a/.org":               "",
	} {
		if got := titleFromFilename(in); got != want {
			t.Errorf("titleFromFilename(%q) = %q, want %q", in, got, want)
		}
	}
}
