package dnd

import (
	"encoding/json"
	"testing"

	"gopkg.in/yaml.v3"
)

// cloneRuleset used to be a yaml round trip, which turns nil slices and maps
// into empty ones; the sheet and worg were served `[]` for them, and the
// json copy must serve the same.
func TestCloneRulesetMatchesYaml(t *testing.T) {
	l := &Library{Rulesets: map[string]*Ruleset{}}
	l.loadBuiltin()
	for id, rs := range l.Rulesets {
		data, _ := yaml.Marshal(rs)
		var want Ruleset
		if err := yaml.Unmarshal(data, &want); err != nil {
			t.Fatal(err)
		}
		got := cloneRuleset(rs)
		if got == rs {
			t.Fatalf("%s: not copied", id)
		}
		got.Modules = nil
		wj, _ := json.Marshal(&want)
		gj, _ := json.Marshal(got)
		if string(wj) != string(gj) {
			t.Errorf("%s: json copy differs from the yaml one (%d vs %d bytes)", id, len(gj), len(wj))
		}
	}
}
