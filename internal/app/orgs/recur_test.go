package orgs

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ihdavids/orgs/internal/common"
)

// Wednesday 2026-11-04, noon.
var recurTestNow = time.Date(2026, 11, 4, 12, 0, 0, 0, time.Local)

func recurDone(t *testing.T, text string) (string, StatusChangeResult) {
	t.Helper()
	paths := loadOrg(t, map[string]string{"recur.org": text})
	p := paths["recur.org"]
	f := GetDb().FindByFile(p)
	sec := f.Doc.Outline.Children[0]
	res, err := applyStatusChange(f, sec, "DONE", "", recurTestNow)
	if err != nil {
		t.Fatal(err)
	}
	return readOrg(t, p), res
}

func TestRecurOnDone(t *testing.T) {
	withFeatures(t, func(f *common.FeatureSettings) { f.Recur = false })
	got, _ := recurDone(t, "* TODO Water plants |+2|\nSCHEDULED: <2026-11-01 Sun>\n")
	if !strings.HasPrefix(got, "* DONE Water plants |+2|") {
		t.Fatalf("with the feature off it is just done:\n%s", got)
	}

	withFeatures(t, func(f *common.FeatureSettings) { f.Recur = true })
	cases := []struct {
		name, text string
		want       []string
		not        []string
	}{
		{"scheduled moves", "* TODO Water plants |+2|\nSCHEDULED: <2026-11-01 Sun>\n",
			[]string{"* TODO Water plants |+2|", "SCHEDULED: <2026-11-06 Fri>", ":LAST_REPEAT: [2026-11-04 Wed 12:00]", `- State "DONE"       from "TODO"`},
			[]string{"CLOSED:"}},
		{"no date gets a SCHEDULED", "* TODO Pay rent |1,15|\nSome text\n",
			[]string{"* TODO Pay rent |1,15|\n  SCHEDULED: <2026-11-15 Sun>\n", "Some text"}, nil},
		{"deadline only moves the deadline", "* TODO Report |Mon|\nDEADLINE: <2026-11-02 Mon>\n",
			[]string{"DEADLINE: <2026-11-09 Mon>"}, []string{"SCHEDULED"}},
		{"the rule beats the repeater on its stamp; another repeater moves by org",
			"* TODO Gym |Mon,Thu|\nSCHEDULED: <2026-11-02 Mon +1w> DEADLINE: <2026-11-01 Sun +1m>\n",
			[]string{"SCHEDULED: <2026-11-05 Thu +1w> DEADLINE: <2026-12-01 Tue +1m>"}, nil},
		{"the stamp keeps its time", "* TODO Standup |wkdy|\nSCHEDULED: <2026-11-03 Tue 09:30>\n",
			[]string{"SCHEDULED: <2026-11-05 Thu 09:30>"}, nil},
		{"the rule's time wins", "* TODO Standup |Thu 10:00-10:15|\nSCHEDULED: <2026-11-03 Tue 09:30>\n",
			[]string{"SCHEDULED: <2026-11-05 Thu 10:00-10:15>"}, nil},
		{"bars that are no rule", "* TODO a | b | c\n", []string{"* DONE a | b | c"}, nil},
	}
	for _, c := range cases {
		got, res := recurDone(t, c.text)
		for _, w := range c.want {
			if !strings.Contains(got, w) {
				t.Errorf("%s: want %q in\n%s", c.name, w, got)
			}
		}
		for _, w := range c.not {
			if strings.Contains(got, w) {
				t.Errorf("%s: did not want %q in\n%s", c.name, w, got)
			}
		}
		if c.name == "scheduled moves" {
			if msg := statusChangeMessage(res, "DONE"); !strings.Contains(msg, "SCHEDULED moved to <2026-11-06 Fri>") || !strings.Contains(msg, "every 2 days") {
				t.Errorf("Msg = %q", msg)
			}
		}
	}
}

func TestRecurSetEndpoint(t *testing.T) {
	withFeatures(t, func(f *common.FeatureSettings) { f.Recur = true })
	line := "* TODO [#A] Water plants                                          :home:"
	paths := loadOrg(t, map[string]string{"s.org": line + "\n"})
	p := paths["s.org"]
	post := func(rule string) string {
		f := GetDb().FindByFile(p)
		sec := f.Doc.Outline.Children[0]
		GetDb().RegisterSection(sec.Hash, sec, f)
		body, _ := json.Marshal(map[string]string{"Hash": sec.Hash, "Rule": rule})
		w := httptest.NewRecorder()
		PostRecurSet(w, httptest.NewRequest("POST", "/recur/set", bytes.NewReader(body)))
		return w.Body.String()
	}
	if out := post("|Mon,Fri|"); !strings.Contains(out, `"Ok":true`) {
		t.Fatalf("set: %s", out)
	}
	got := readOrg(t, p)
	if !strings.HasPrefix(got, "* TODO [#A] Water plants |Mon,Fri|") || len(strings.TrimRight(got, "\n")) != len(line) || !strings.HasSuffix(strings.TrimSpace(got), ":home:") {
		t.Errorf("set:\n%q", got)
	}
	if out := post("bogus"); strings.Contains(out, `"Ok":true`) {
		t.Errorf("a bad rule was taken: %s", out)
	}
	if out := post(""); !strings.Contains(out, `"Ok":true`) {
		t.Fatalf("clear: %s", out)
	}
	if got := strings.TrimRight(readOrg(t, p), "\n"); got != line {
		t.Errorf("clear:\n%q\nwant\n%q", got, line)
	}
}
