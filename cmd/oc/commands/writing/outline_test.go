package writing

import (
	"strings"
	"testing"
)

func TestOutlinePaths(t *testing.T) {
	doc := strings.Join([]string{
		"#+TITLE: x",
		"* TODO [#A] Part one :draft:",
		"text",
		"*** Skipped a level",
		"#+begin_src org",
		"* not a heading",
		"#+end_src",
		"** COMMENT Chapter",
		"* Part two",
		"",
	}, "\n")
	nodes := Outline(strings.Split(doc, "\n"), map[string]bool{"TODO": true, "DONE": true})
	want := []Node{
		{Line: 1, Level: 1, Title: "Part one", End: 7, Path: "Part one"},
		{Line: 3, Level: 3, Title: "Skipped a level", End: 6, Path: "Part one\x00\x00Skipped a level"},
		{Line: 7, Level: 2, Title: "Chapter", End: 7, Path: "Part one\x00Chapter"},
		{Line: 8, Level: 1, Title: "Part two", End: 8, Path: "Part two"},
	}
	if len(nodes) != len(want) {
		t.Fatalf("%d nodes: %+v", len(nodes), nodes)
	}
	for i := range want {
		if nodes[i] != want[i] {
			t.Errorf("node %d: %+v, want %+v", i, nodes[i], want[i])
		}
	}
	if n := SnapWords("* Part two\nIt's a dog’s life, 42 times."); n != 7 {
		t.Errorf("words: %d", n)
	}
}
