package triage

import (
	"strings"
	"testing"
)

// A tags line either edits the list ("+work -home") or is the new list.
func TestApplyTagEdit(t *testing.T) {
	cases := []struct{ have, line, want string }{
		{"home", "+work", "home work"},
		{"home work", "-HOME", "work"},
		{"home", "+home", "home"},
		{"home", "work urgent", "work urgent"},
		{"home", ":a:b:", "a b"},
		{"home", "+work -nothing", "home work"},
	}
	for _, c := range cases {
		have := strings.Fields(c.have)
		got := strings.Join(ApplyTagEdit(have, c.line), " ")
		if got != c.want {
			t.Errorf("%q with %q = %q, want %q", c.have, c.line, got, c.want)
		}
	}
}
