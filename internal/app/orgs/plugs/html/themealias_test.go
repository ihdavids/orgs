package htmlexp

import "testing"

func TestEmacsThemeNamesFindTheirLookalike(t *testing.T) {
	cases := []struct{ theme, setup, want string }{
		{"readtheorg", "", "rtd"},
		{"", "https://fniessen.github.io/org-html-themes/org/theme-readtheorg.setup", "rtd"},
		{"", "~/themes/theme-bigblow.setup", "docs"},
		{"furo", "theme-readtheorg.setup", "furo"},
		{"", "", ""},
		{"Docs", "", "docs"},
	}
	for _, c := range cases {
		if got := themeAlias(c.theme, c.setup); got != c.want {
			t.Errorf("%q %q: got %q, want %q", c.theme, c.setup, got, c.want)
		}
	}
}
