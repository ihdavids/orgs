package commands

import (
	"flag"
	"reflect"
	"testing"
)

// Flags may come after words even when a `--` ends them: `orgs snip new -d
// 'what it does' -- cmd` once took -d and its value for words of the command.
func TestFreeArgsWithDoubleDash(t *testing.T) {
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	d := fs.String("d", "", "")
	if err := fs.Parse([]string{"new", "-d", "say hi", "--", "echo", "-n", "hi"}); err != nil {
		t.Fatal(err)
	}
	got := FreeArgs(fs)
	if *d != "say hi" || !reflect.DeepEqual(got, []string{"new", "echo", "-n", "hi"}) {
		t.Errorf("d=%q words=%q", *d, got)
	}
}
