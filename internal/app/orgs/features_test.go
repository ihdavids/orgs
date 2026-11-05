package orgs

import (
	"strings"
	"testing"
	"time"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

// withFeatures turns features on for one test.
func withFeatures(t *testing.T, set func(*common.FeatureSettings)) {
	t.Helper()
	if Conf().Server == nil {
		t.Skip("no server settings")
	}
	old := *Conf().Server
	set(&Conf().Server.Features)
	t.Cleanup(func() { *Conf().Server = old })
}

func secAt(t *testing.T, path string, olp ...string) (*common.OrgFile, *org.Section) {
	t.Helper()
	f, sec := GetDb().GetFromTarget(&common.Target{Type: "file+olp", Filename: path, Id: strings.Join(olp, "::")}, false)
	if sec == nil {
		t.Fatalf("no heading %v in %s", olp, path)
	}
	return f, sec
}

func markDone(t *testing.T, path string, olp ...string) common.Result {
	t.Helper()
	_, sec := secAt(t, path, olp...)
	res, _ := ChangeStatus(&common.TodoItemChange{Hash: sec.Hash, Value: "DONE"})
	return res
}

func TestFeaturesYamlBool(t *testing.T) {
	out, err := setYamlBool([]string{"server:", "  port: 8010"}, []string{"server", "features", "edna"}, true)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(out, "\n")
	if !strings.Contains(got, "  features:\n    edna: true") {
		t.Fatalf("got\n%s", got)
	}
}

func TestChecklistResetOnRepeat(t *testing.T) {
	text := `* TODO Weekly review [1/3]
SCHEDULED: <2026-11-02 Mon +1w>
:PROPERTIES:
:RESET_CHECK_BOXES: t
:END:
- [X] inbox
- [ ] calendar
- [-] projects
  - [X] a
  - [ ] b
#+begin_example
- [X] not a box
#+end_example
** Sub [1/1]
- [X] nested
`
	withFeatures(t, func(f *common.FeatureSettings) { f.ChecklistReset = false })
	paths := loadOrg(t, map[string]string{"r.org": text})
	p := paths["r.org"]
	markDone(t, p, "Weekly review [1/3]")
	if got := readOrg(t, p); !strings.Contains(got, "- [X] inbox") {
		t.Fatalf("reset with the feature off:\n%s", got)
	}

	withFeatures(t, func(f *common.FeatureSettings) { f.ChecklistReset = true })
	paths = loadOrg(t, map[string]string{"r.org": text})
	p = paths["r.org"]
	res := markDone(t, p, "Weekly review [1/3]")
	got := readOrg(t, p)
	for _, want := range []string{"* TODO Weekly review [0/3]", "- [ ] inbox", "- [ ] projects", "  - [ ] a", "- [X] not a box", "** Sub [0/1]", "- [ ] nested", "<2026-11-09 Mon +1w>"} {
		if !strings.Contains(got, want) {
			t.Errorf("want %q in\n%s", want, got)
		}
	}
	if !strings.Contains(res.Msg, "4 checkboxes unticked") {
		t.Errorf("Msg = %q", res.Msg)
	}

	// No repeater, no reset, even with the property.
	paths = loadOrg(t, map[string]string{"n.org": "* TODO Once\n:PROPERTIES:\n:RESET_CHECK_BOXES: t\n:END:\n- [X] a\n"})
	markDone(t, paths["n.org"], "Once")
	if got := readOrg(t, paths["n.org"]); !strings.Contains(got, "- [X] a") {
		t.Errorf("reset without a repeat:\n%s", got)
	}
}

func TestEdnaParse(t *testing.T) {
	good := []struct{ prop, text string }{
		{"TRIGGER", `next-sibling todo!(NEXT) scheduled!("+1d")`},
		{"BLOCKER", `ids(a1 "b 2") !done?`},
		{"TRIGGER", `if siblings done? then parent todo!(DONE) else self set-property!("X" "y") endif`},
		{"BLOCKER", `children(todo-only) consider(all) todo-state?(WAITING)`},
	}
	for _, c := range good {
		if err := common.ValidateEdna(c.prop, c.text); err != nil {
			t.Errorf("%s %q: %v", c.prop, c.text, err)
		}
	}
	bad := []struct{ prop, text, want string }{
		{"TRIGGER", `next-sibling todoo!(NEXT)`, "did you mean todo!"},
		{"TRIGGER", `chldren todo!(NEXT)`, "did you mean children"},
		{"BLOCKER", `self todo!(DONE)`, "cannot act"},
		{"TRIGGER", `if self done? then parent todo!(DONE)`, "without an endif"},
		{"TRIGGER", `self set-property!("X")`, "takes 2 arguments"},
		{"TRIGGER", `ids("unclosed)`, "never closed"},
		{"TRIGGER", `children(nonsense) todo!(X)`, "no option"},
	}
	for _, c := range bad {
		err := common.ValidateEdna(c.prop, c.text)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s %q: got %v, want %q", c.prop, c.text, err, c.want)
		}
	}
}

func TestEdnaBlockerAndTrigger(t *testing.T) {
	text := `#+TODO: TODO NEXT WAITING | DONE
* Project
** TODO Outline
:PROPERTIES:
:ID: outline-id
:END:
** TODO Draft
:PROPERTIES:
:BLOCKER: ids(outline-id)
:TRIGGER: next-sibling todo!(NEXT) scheduled!("2026-12-01") tag!("writing") self set-property!("DRAFTED" "yes")
:END:
** TODO Review
:PROPERTIES:
:TRIGGER: parent set-property!("REVIEWED" "t") rest-of-siblings archive!
:END:
** TODO Publish
* Loop
** TODO A
:PROPERTIES:
:TRIGGER: next-sibling todo!(DONE)
:END:
** TODO B
:PROPERTIES:
:TRIGGER: previous-sibling todo!(DONE)
:END:
`
	withFeatures(t, func(f *common.FeatureSettings) { f.Edna = false })
	paths := loadOrg(t, map[string]string{"e.org": text})
	p := paths["e.org"]
	if res := markDone(t, p, "Project", "Draft"); !res.Ok {
		t.Fatalf("blocked with edna off: %+v", res)
	}
	if got := readOrg(t, p); strings.Contains(got, "NEXT Review") {
		t.Fatalf("triggered with edna off:\n%s", got)
	}

	withFeatures(t, func(f *common.FeatureSettings) { f.Edna = true })
	oldTarget := Conf().ArchiveDefaultTarget
	Conf().ArchiveDefaultTarget = "%s_archive::"
	t.Cleanup(func() { Conf().ArchiveDefaultTarget = oldTarget })
	paths = loadOrg(t, map[string]string{"e.org": text})
	p = paths["e.org"]
	res := markDone(t, p, "Project", "Draft")
	if res.Ok || !strings.Contains(res.Msg, `waiting on "Outline"`) {
		t.Fatalf("not blocked: %+v", res)
	}
	f, sec := secAt(t, p, "Project", "Draft")
	if why := ednaBlocker(f, sec); why == "" {
		t.Fatal("IsBlocked would say no")
	}
	if res := markDone(t, p, "Project", "Outline"); !res.Ok {
		t.Fatalf("outline: %+v", res)
	}
	res = markDone(t, p, "Project", "Draft")
	if !res.Ok || !strings.Contains(res.Msg, "TRIGGER") {
		t.Fatalf("draft: %+v", res)
	}
	got := readOrg(t, p)
	for _, want := range []string{"** NEXT Review", ":writing:", "SCHEDULED: <2026-12-01 Tue>", ":DRAFTED:  yes"} {
		if !strings.Contains(got, want) {
			t.Errorf("want %q in\n%s", want, got)
		}
	}

	// archive! is done last and all together; parent's property first.
	res = markDone(t, p, "Project", "Review")
	if !res.Ok {
		t.Fatalf("review: %+v", res)
	}
	got = readOrg(t, p)
	if !strings.Contains(got, ":REVIEWED: t") || strings.Contains(got, "TODO Publish") {
		t.Errorf("review trigger:\n%s\n%s", got, res.Msg)
	}

	// A cycle of triggers stops.
	res = markDone(t, p, "Loop", "A")
	if !res.Ok {
		t.Fatalf("loop: %+v", res)
	}
	got = readOrg(t, p)
	if !strings.Contains(got, "** DONE A") || !strings.Contains(got, "** DONE B") {
		t.Errorf("loop:\n%s", got)
	}
}

func TestEdnaStamp(t *testing.T) {
	now := time.Date(2026, 11, 4, 9, 30, 0, 0, time.Local)
	run := &ednaRun{now: now}
	sec := &org.Section{Headline: &org.Headline{Lvl: 1}}
	cases := map[string]string{
		"+2d":  "<2026-11-06 Fri>",
		"++1w": "<2026-11-11 Wed>",
		"++2h": "<2026-11-04 Wed 11:30>",
		"rm":   "",
	}
	for arg, want := range cases {
		got, err := run.ednaStamp(ednaRef{}, sec, "SCHEDULED", arg)
		if err != nil || got != want {
			t.Errorf("%s: got %q %v, want %q", arg, got, err, want)
		}
	}
}

func TestNoterPageAndInsert(t *testing.T) {
	for in, want := range map[string][2]float64{"4": {4, -1}, "(4 . 0.31)": {4, 0.31}, "(12 0.5 . 0.2)": {12, 0.5}} {
		p, y, ok := parseNoterPage(in)
		if !ok || float64(p) != want[0] || y != want[1] {
			t.Errorf("%s: %d %v %v", in, p, y, ok)
		}
	}
	withFeatures(t, func(f *common.FeatureSettings) { f.Noter = true })
	paths := loadOrg(t, map[string]string{"n.org": `* Paper
:PROPERTIES:
:NOTER_DOCUMENT: paper.pdf
:END:
** Early
:PROPERTIES:
:NOTER_PAGE: 2
:END:
** Late
:PROPERTIES:
:NOTER_PAGE: (9 . 0.5)
:END:
* After
`})
	p := paths["n.org"]
	f, sec := secAt(t, p, "Paper")
	lines, err := insertNoterNote(p, sec, 5, 0.25, "Middle", "quoted\nwords", "my note")
	if err != nil {
		t.Fatal(err)
	}
	writeLines(p, lines)
	got := readOrg(t, p)
	want := `** Early
:PROPERTIES:
:NOTER_PAGE: 2
:END:
** Middle
:PROPERTIES:
:NOTER_PAGE: (5 . 0.25)
:END:
#+begin_quote
quoted
words
#+end_quote
my note
** Late`
	if !strings.Contains(got, want) {
		t.Fatalf("got\n%s", got)
	}
	f, sec = secAt(t, p, "Paper")
	notes := noterNotes(f, sec)
	if len(notes) != 3 || notes[1].Headline != "Middle" || notes[1].Quote != "quoted\nwords" || notes[1].Text != "my note" {
		t.Fatalf("notes %+v", notes)
	}
}

func TestRemarkAnchor(t *testing.T) {
	text := []rune("one two\nthree four five\nthree four")
	h := RemarkHighlight{Beg: 25, Text: "three four"}
	anchor(text, &h)
	if h.Lost || h.Moved || h.Line != 2 || h.Col != 0 || h.End != 35 {
		t.Fatalf("in place: %+v", h)
	}
	h = RemarkHighlight{Beg: 3, Text: "two three"} // across a line break
	anchor(text, &h)
	if h.Lost || h.Beg != 5 || h.Text != "two\nthree" || !h.Moved {
		t.Fatalf("across lines: %+v", h)
	}
	h = RemarkHighlight{Beg: 1, Text: "six"}
	if anchor(text, &h); !h.Lost {
		t.Fatalf("gone: %+v", h)
	}
}

func TestExpiryDue(t *testing.T) {
	withFeatures(t, func(f *common.FeatureSettings) { f.Expiry, f.AutoArchive = true, true })
	Conf().Server.AutoArchive.After = "30d"
	paths := loadOrg(t, map[string]string{"x.org": `* Old link
:PROPERTIES:
:EXPIRY: <2026-01-01 Thu>
:END:
* Fresh link
:PROPERTIES:
:CREATED: [2026-11-01 Sun 10:00]
:EXPIRY: +2w
:END:
* Stale link
:PROPERTIES:
:CREATED: [2026-09-01 Tue 10:00]
:EXPIRY: +2w
:END:
* DONE Long done
CLOSED: [2026-08-01 Sat 10:00]
* DONE Just done
CLOSED: [2026-10-30 Fri 10:00]
* Keep
:PROPERTIES:
:AUTO_ARCHIVE: nil
:END:
** DONE Old but kept
CLOSED: [2026-01-01 Thu 10:00]
* DONE Logged only
:LOGBOOK:
- State "DONE"       from "TODO"       [2026-07-01 Wed 10:00]
:END:
`})
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.Local)
	got := map[string]string{}
	for _, it := range expiryDue(now) {
		got[it.Headline] = it.Feature + "/" + it.Action
	}
	want := map[string]string{"Old link": "expiry/archive", "Stale link": "expiry/archive", "Long done": "autoArchive/archive", "Logged only": "autoArchive/archive"}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: got %q want %q (all %v)", k, got[k], v, got)
		}
	}

	Conf().Server.Expiry.Handler = "tag"
	oldTarget := Conf().ArchiveDefaultTarget
	Conf().ArchiveDefaultTarget = "%s_archive::"
	t.Cleanup(func() { Conf().ArchiveDefaultTarget = oldTarget })
	res := runExpiry(now, nil)
	if !res.Ok || res.Done != 4 {
		t.Fatalf("run %+v", res)
	}
	text := readOrg(t, paths["x.org"])
	if !strings.Contains(text, "* Old link") || !strings.Contains(text, ":ARCHIVE:") || strings.Contains(text, "Long done") || !strings.Contains(text, "Old but kept") {
		t.Fatalf("after run:\n%s", text)
	}
	if left := expiryDue(now); len(left) != 0 {
		t.Fatalf("still due: %+v", left)
	}
}
