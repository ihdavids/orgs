package orgs

import (
	"strings"
	"testing"

	"github.com/ihdavids/go-org/org"
)

func TestColumnValuesCountsEveryProperty(t *testing.T) {
	src := "#+TODO: TODO NEXT | DONE\n" +
		"* TODO Design  :work:\n:PROPERTIES:\n:OWNER: alice\n:EFFORT: 2d\n:END:\n" +
		"** NEXT Sketch  :work:ui:\n   :PROPERTIES:\n   :OWNER: alice\n   :END:\n" +
		"* DONE Build\n:PROPERTIES:\n:OWNER: bob\n:Effort: 3d\n:END:\n"
	doc := org.New().Silent().Parse(strings.NewReader(src), "plan.org")
	props := collectColumnValues(doc.Outline.Children, strings.Split(src, "\n"))
	got := map[string]map[string]int{}
	heads := map[string]int{}
	for _, p := range props {
		got[p.Name] = map[string]int{}
		heads[p.Name] = p.Count
		for _, v := range p.Values {
			got[p.Name][v.Value] = v.Count
		}
	}
	if got["OWNER"]["alice"] != 2 || got["OWNER"]["bob"] != 1 || heads["OWNER"] != 3 {
		t.Errorf("OWNER: %v (%d headings)", got["OWNER"], heads["OWNER"])
	}
	// A property's name is the same whatever case it was written in.
	if got["EFFORT"]["2d"] != 1 || got["EFFORT"]["3d"] != 1 {
		t.Errorf("EFFORT: %v", got["EFFORT"])
	}
	if got["TODO"]["TODO"] != 1 || got["TODO"]["NEXT"] != 1 || got["TODO"]["DONE"] != 1 {
		t.Errorf("TODO: %v", got["TODO"])
	}
	if got["TAGS"]["work"] != 2 || got["TAGS"]["ui"] != 1 || heads["TAGS"] != 2 {
		t.Errorf("TAGS: %v", got["TAGS"])
	}
	// Most used first.
	for _, p := range props {
		if p.Name == "OWNER" && p.Values[0].Value != "alice" {
			t.Errorf("OWNER values not most-used first: %v", p.Values)
		}
	}
}
