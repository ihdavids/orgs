package orgs

import (
	"encoding/json"
	"reflect"
	"testing"
)

// harper-ls answers a code action request with CodeActions (an edit, and a
// command whose first argument is the lint as a JSON string) and bare
// Commands (command a string). Both shapes come in one list.
func TestHarperReadActions(t *testing.T) {
	r := lspRange{Start: lspPos{0, 0}, End: lspPos{0, 5}}
	res := json.RawMessage(`[
		{"title":"Replace with: “There”","edit":{"changes":{"file:///x":[{"range":{"start":{"line":0,"character":0},"end":{"line":0,"character":5}},"newText":"There"}]}},
		 "command":{"command":"HarperRecordLint","arguments":["{\"Lint\":{\"kind\":\"Grammar\",\"span\":null}}"]}},
		{"title":"Replace with: “There”","edit":{"changes":{"file:///x":[{"range":{"start":{"line":0,"character":0},"end":{"line":0,"character":5}},"newText":"There"}]}}},
		{"title":"Replace elsewhere","edit":{"changes":{"file:///x":[{"range":{"start":{"line":0,"character":6},"end":{"line":0,"character":9}},"newText":"no"}]}}},
		{"title":"Ignore Harper error.","command":"HarperIgnoreLint","arguments":[{"uri":"file:///x"}, 3]}
	]`)
	l := HarperLint{Fixes: []string{}}
	readActions(res, r, &l)
	if l.Kind != "Grammar" || !reflect.DeepEqual(l.Fixes, []string{"There"}) {
		t.Fatalf("got kind %q fixes %v", l.Kind, l.Fixes)
	}
}

func TestUTF16LineStarts(t *testing.T) {
	// 😀 is two UTF-16 units, as LSP counts them.
	got := utf16LineStarts("a😀b\ncd\n")
	if !reflect.DeepEqual(got, []int{0, 5, 8}) {
		t.Fatalf("got %v", got)
	}
}
