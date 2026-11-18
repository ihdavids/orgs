package commands

import (
	"strings"
	"testing"
)

func TestSetTopLevelYamlLeavesNestedKeys(t *testing.T) {
	in := "url: \"http://x\"\nserver:\n  tokenExpiry: \"8h\"\n  port: 8010\ntokenExpiry: \"old\"\n"
	lines := SetTopLevelYaml(strings.Split(in, "\n"), "tokenExpiry", `"new"`)
	lines = SetTopLevelYaml(lines, "token", `"abc"`)
	got := strings.Join(lines, "\n")
	want := "url: \"http://x\"\nserver:\n  tokenExpiry: \"8h\"\n  port: 8010\ntokenExpiry: \"new\"\ntoken: \"abc\"\n"
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}
