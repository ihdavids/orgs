package palette

import (
	"reflect"
	"testing"
)

func TestSplitWords(t *testing.T) {
	cases := map[string][]string{
		``:                             {},
		`DONE 'IsStatus("NEXT")'`:      {"DONE", `IsStatus("NEXT")`},
		`a "b \"c\" d" e\ f`:           {"a", `b "c" d`, "e f"},
		`  -json   -limit 3 `:          {"-json", "-limit", "3"},
		`''`:                           {""},
		`MatchHeadline("(?i)re port")`: {`MatchHeadline((?i)re port)`},
	}
	for in, want := range cases {
		got, err := splitWords(in)
		if err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
	if _, err := splitWords(`'open`); err == nil {
		t.Error("an unclosed quote should be refused")
	}
}

// What shellJoin prints, splitWords must read back as the same words.
func TestShellJoinRoundTrip(t *testing.T) {
	for _, words := range [][]string{
		{"q", "stale"},
		{"search", "{{NextTasks}}"},
		{"search", `IsStatus("NEXT") && HasTags('work')`, "-limit", "5"},
		{"note", "it's done"},
	} {
		back, err := splitWords(shellJoin(words))
		if err != nil || !reflect.DeepEqual(back, words) {
			t.Errorf("%q -> %q -> %q (%v)", words, shellJoin(words), back, err)
		}
	}
}
