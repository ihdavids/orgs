package orgs

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

var (
	scDone   = map[string]bool{"DONE": true, "CANCELLED": true}
	scActive = map[string]bool{"TODO": true, "NEXT": true}
)

func scRefresh(text string, row int) string {
	return strings.Join(refreshCookiesAround(strings.Split(text, "\n"), row, scDone, scActive), "\n")
}

// The same scenarios as orgcmds.test.ts (refreshSection), after the tick.
func TestCookiesFollowBoxes(t *testing.T) {
	got := scRefresh("* Tasks [0/2]\n- [ ] one [0%]\n  - [X] a\n  - [ ] b\n- [ ] two", 2)
	if want := "* Tasks [0/2]\n- [-] one [50%]\n  - [X] a\n  - [ ] b\n- [ ] two"; got != want {
		t.Errorf("one ticked:\n%s", got)
	}
	got = scRefresh("* Tasks [0/2]\n- [-] one [50%]\n  - [X] a\n  - [X] b\n- [ ] two", 3)
	if want := "* Tasks [1/2]\n- [X] one [100%]\n  - [X] a\n  - [X] b\n- [ ] two"; got != want {
		t.Errorf("both ticked:\n%s", got)
	}
}

func TestCookiesCountChildKeywordsAndCookieData(t *testing.T) {
	if got := scRefresh("* P [/]\n** TODO a\n** DONE b\n** note", 0); got != "* P [1/2]\n** TODO a\n** DONE b\n** note" {
		t.Errorf("keywords:\n%s", got)
	}
	// recursive: every heading below, not only the children.
	got := scRefresh("* P [/]\n:PROPERTIES:\n:COOKIE_DATA: todo recursive\n:END:\n** TODO a\n*** DONE a1\n*** DONE a2\n** DONE b", 0)
	if !strings.HasPrefix(got, "* P [3/4]") {
		t.Errorf("recursive todo:\n%s", got)
	}
	// todo: the children's keywords even with boxes in the section.
	got = scRefresh("* P [%]\n:PROPERTIES:\n:COOKIE_DATA: todo\n:END:\n- [X] box\n** DONE a\n** TODO b", 0)
	if !strings.HasPrefix(got, "* P [50%]") {
		t.Errorf("todo over boxes:\n%s", got)
	}
	// checkbox recursive: every box at any depth.
	got = scRefresh("* P [/]\n:PROPERTIES:\n:COOKIE_DATA: checkbox recursive\n:END:\n- [ ] one\n  - [X] a\n  - [ ] b\n** DONE child", 0)
	if !strings.HasPrefix(got, "* P [1/3]") {
		t.Errorf("recursive boxes:\n%s", got)
	}
	// A grandchild's keyword reaches a recursive grandparent.
	got = scRefresh("* G [/]\n:PROPERTIES:\n:COOKIE_DATA: recursive\n:END:\n** P [/]\n*** DONE x\n*** TODO y", 5)
	if !strings.HasPrefix(got, "* G [1/2]") || !strings.Contains(got, "** P [1/2]") {
		t.Errorf("up the tree:\n%s", got)
	}
	// Inside a block nothing is counted.
	got = scRefresh("* P [/]\n#+BEGIN_EXAMPLE\n- [X] not a box\n#+END_EXAMPLE\n** DONE a", 0)
	if !strings.HasPrefix(got, "* P [1/1]") {
		t.Errorf("block:\n%s", got)
	}
}

func TestServerWritesKeepCookiesTrue(t *testing.T) {
	paths := loadOrg(t, map[string]string{"c.org": "* Project [0/2]\n** TODO a\n** TODO b\n"})
	p := paths["c.org"]
	f := GetDb().FindByFile(p)
	sec := f.Doc.Outline.Children[0].Children[0]
	if _, err := applyStatusChange(f, sec, "DONE", "", time.Now()); err != nil {
		t.Fatal(err)
	}
	if got := readOrg(t, p); !strings.HasPrefix(got, "* Project [1/2]\n") {
		t.Errorf("after DONE:\n%s", got)
	}
}

func TestTickingThroughTheEndpointUpdatesTheCookie(t *testing.T) {
	paths := loadOrg(t, map[string]string{"k.org": "* Card [0/2]\n- [ ] one\n- [ ] two\n"})
	p := paths["k.org"]
	f := GetDb().FindByFile(p)
	hash := f.Doc.Outline.Children[0].Hash
	GetDb().RegisterSection(hash, f.Doc.Outline.Children[0], f)
	body, _ := json.Marshal(ChecklistToggle{Hash: hash, Index: 1, Text: "two", Done: true})
	w := httptest.NewRecorder()
	PostChecklistToggle(w, httptest.NewRequest("POST", "/checklist", bytes.NewReader(body)))
	if !strings.Contains(w.Body.String(), `"Ok":true`) {
		t.Fatalf("toggle: %s", w.Body.String())
	}
	if got := readOrg(t, p); got != "* Card [1/2]\n- [ ] one\n- [X] two" {
		t.Errorf("after the tick: %q", got)
	}
}

func TestTodoDependenciesBlockDone(t *testing.T) {
	if Conf().Server == nil {
		t.Skip("no server settings")
	}
	old := *Conf().Server
	Conf().Server.EnforceTodoDependencies = true
	Conf().Server.EnforceCheckboxDependencies = true
	t.Cleanup(func() { *Conf().Server = old })
	paths := loadOrg(t, map[string]string{"d.org": `* TODO Parent
** TODO Child
* Steps
:PROPERTIES:
:ORDERED: t
:END:
** TODO First
** TODO Second
* TODO Boxes
- [ ] one
* TODO Free
:PROPERTIES:
:NOBLOCKING: t
:END:
** TODO Open child
`})
	p := paths["d.org"]
	f := GetDb().FindByFile(p)
	top := f.Doc.Outline.Children
	cases := []struct {
		sec  *org.Section
		want string
	}{
		{top[0], `"Child" under it is still TODO`},
		{top[1].Children[1], `"First" comes first ("Steps" is ORDERED)`},
		{top[1].Children[0], ""},
		{top[2], "1 of its 1 checkboxes are not ticked"},
		{top[3], ""},
	}
	for _, c := range cases {
		got := statusBlocked(f, c.sec, "DONE")
		if (c.want == "") != (got == "") || !strings.Contains(got, c.want) {
			t.Errorf("%s: %q, want %q", headlineTitle(c.sec), got, c.want)
		}
	}
	// Not into an open keyword, and not when already done.
	if why := statusBlocked(f, top[0], "TODO"); why != "" {
		t.Errorf("to TODO: %s", why)
	}
	// The query function is the same rule, setting or not.
	Conf().Server.EnforceTodoDependencies = false
	got := []string{}
	if err := evalEachSection(`IsBlocked()`, func(s *org.Section, _ *common.OrgFile) { got = append(got, headlineTitle(s)) }); err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != "Parent,Second" {
		t.Errorf("IsBlocked(): %v", got)
	}
	// Force goes past it, as Emacs's C-u C-u C-u C-c C-t does.
	Conf().Server.EnforceTodoDependencies = true
	GetDb().RegisterSection(top[0].Hash, top[0], f)
	if _, err := ChangeStatus(&common.TodoItemChange{Hash: top[0].Hash, Value: "DONE"}); err == nil {
		t.Errorf("a blocked heading was marked done")
	}
	if _, err := ChangeStatus(&common.TodoItemChange{Hash: top[0].Hash, Value: "DONE", Force: true}); err != nil {
		t.Errorf("forced: %v", err)
	}
}
