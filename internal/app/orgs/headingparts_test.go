package orgs

import "testing"

func TestSetHeadlineParts(t *testing.T) {
	strp := func(s string) *string { return &s }
	tagsp := func(ts ...string) *[]string { return &ts }
	cases := []struct {
		name, line, status string
		prio               *string
		tags               *[]string
		want               string
	}{
		{"add tags", "** TODO Docs", "TODO", nil, tagsp("work", "ui"), "** TODO Docs  :work:ui:"},
		{"replace tags", "** TODO Test  :blocked:", "TODO", nil, tagsp("qa"), "** TODO Test  :qa:"},
		{"take tags off", "** TODO Test  :blocked:", "TODO", nil, tagsp(), "** TODO Test"},
		{"right-aligned tags stay aligned",
			"** Read organice documentation for android                                      :READ:",
			"", nil, tagsp("READ", "CMP"),
			"** Read organice documentation for android                                  :READ:CMP:"},
		{"a priority added after the keyword", "** TODO Docs  :work:", "TODO", strp("A"), nil, "** TODO [#A] Docs  :work:"},
		{"a priority changed, lower case read as upper", "* NEXT [#C] Ship it", "NEXT", strp("b"), nil, "* NEXT [#B] Ship it"},
		{"a priority taken off", "* [#A] Plan", "", strp(""), nil, "* Plan"},
		{"a title that starts like a keyword is not one", "** TODOS for later", "", strp("A"), nil, "** [#A] TODOS for later"},
		{"tags kept when only the priority changes", "** Beta  :M1:", "", strp("A"), nil, "** [#A] Beta  :M1:"},
	}
	for _, c := range cases {
		if got := setHeadlineParts(c.line, c.status, c.prio, c.tags); got != c.want {
			t.Errorf("%s:\n got %q\nwant %q", c.name, got, c.want)
		}
	}
}
