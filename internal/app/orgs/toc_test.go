package orgs

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// The cases are vscode-org-everywhere/test/fixtures/toc_cases.json too: the
// TypeScript twin (src/toc-model.ts) must give the same answers.
type tocCase struct {
	Name string
	In   []string
	Out  []string
}

func loadTocCases(t *testing.T) []tocCase {
	b, err := os.ReadFile("testdata/toc_cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cs []tocCase
	if err := json.Unmarshal(b, &cs); err != nil {
		t.Fatal(err)
	}
	return cs
}

func TestTocCases(t *testing.T) {
	for _, c := range loadTocCases(t) {
		got, _, changed := ApplyTocs(c.In, nil)
		if strings.Join(got, "\n") != strings.Join(c.Out, "\n") {
			t.Errorf("%s:\n--- got\n%s\n--- want\n%s", c.Name, strings.Join(got, "\n"), strings.Join(c.Out, "\n"))
			continue
		}
		if !changed {
			t.Errorf("%s: said unchanged", c.Name)
		}
		// The update on save must not loop: the second run finds nothing.
		again, sites, changed := ApplyTocs(got, nil)
		if changed || strings.Join(again, "\n") != strings.Join(got, "\n") {
			t.Errorf("%s: a second run changed it again:\n%s", c.Name, strings.Join(again, "\n"))
		}
		for _, s := range sites {
			if s.Changed {
				t.Errorf("%s: site %s at %d still says changed", c.Name, s.Kind, s.Row)
			}
		}
	}
}

func TestTocInsert(t *testing.T) {
	in := []string{"#+TITLE: x", "Some preamble.", "* One", "** Two", "* Three"}
	got, row := TocInsert(in, "toc-org", -1, nil)
	want := []string{"#+TITLE: x", "Some preamble.", "* Table of Contents :TOC:", "- [[#one][One]]", "  - [[#two][Two]]", "- [[#three][Three]]", "", "* One", "** Two", "* Three"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") || row != 2 {
		t.Errorf("toc-org insert (row %d):\n%s", row, strings.Join(got, "\n"))
	}
	if again, _ := TocInsert(got, "toc-org", -1, nil); strings.Join(again, "\n") != strings.Join(got, "\n") {
		t.Errorf("a second insert added another:\n%s", strings.Join(again, "\n"))
	}
	in = []string{"* One", "  :PROPERTIES:", "  :ID: 1", "  :END:", "Text.", "* Two"}
	got, _ = TocInsert(in, "org-make-toc", 0, nil)
	want = []string{"* One", "  :PROPERTIES:", "  :ID: 1", "  :TOC: :include all :ignore this", "  :END:", "  :CONTENTS:", "  - [[#two][Two]]", "  :END:", "Text.", "* Two"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("org-make-toc insert:\n%s", strings.Join(got, "\n"))
	}
	got, row = TocInsert([]string{"* A", "SCHEDULED: <2026-11-04 Wed>"}, "org-make-toc", 0, nil)
	want = []string{"* A", "SCHEDULED: <2026-11-04 Wed>", ":PROPERTIES:", ":TOC: :include all :ignore this", ":END:", ":CONTENTS:", ":END:"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") || row != 0 {
		t.Errorf("org-make-toc insert with no drawer:\n%s", strings.Join(got, "\n"))
	}
}
