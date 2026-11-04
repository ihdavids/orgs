package orgs

import (
	"strings"
	"testing"

	"github.com/ihdavids/orgs/internal/common"
)

func TestTagDefsParse(t *testing.T) {
	d := parseTagDefs("[ Work : ProjA ProjB {P@.+} ]\n{ @home(h) @work(w) @errand }\n[ ProjA : Kitchen ]")
	names, pats := d.expand("work")
	if strings.Join(names, ",") != "work,ProjA,Kitchen,ProjB" || strings.Join(pats, ",") != "P@.+" {
		t.Errorf("expand: %v %v", names, pats)
	}
	if len(d.exclusive) != 1 || strings.Join(d.exclusive[0], ",") != "@home,@work,@errand" {
		t.Errorf("exclusive: %v", d.exclusive)
	}
	got := exclusiveTags(d, []string{"@home", "x"}, []string{"@home", "x", "@work"})
	if strings.Join(got, ",") != "x,@work" {
		t.Errorf("the new one stays: %v", got)
	}
}

func TestGroupTagsSearchAndExclusiveTagsWrite(t *testing.T) {
	paths := loadOrg(t, map[string]string{"g.org": `#+TAGS: [ Work : ProjA {^P@} ] { @home @work }
* One                                                   :ProjA:
* Two                                                   :P@acme:
* Three                                                 :Other:
* Four                                                  :@home:keep:
* Five
`})
	if got := queryTitles(t, `HasTags("Work")`); got != "One,Two" {
		t.Errorf("group search: %s", got)
	}
	if got := queryTitles(t, `:work:`); got != "One,Two" {
		t.Errorf("short query: %s", got)
	}
	p := paths["g.org"]
	f := GetDb().FindByFile(p)
	four := f.Doc.Outline.Children[3]
	GetDb().RegisterSection(four.Hash, four, f)
	if res, err := ToggleTag(&common.TodoItemChange{Hash: four.Hash, Value: "@work"}); err != nil || !res.Ok {
		t.Fatalf("toggle: %v %+v", err, res)
	}
	got := readOrg(t, p)
	if !strings.Contains(got, "* Four") || !strings.Contains(got, ":keep:@work:") || strings.Contains(got, "@home:") {
		t.Errorf("@work should replace @home:\n%s", got)
	}
	// A line edit: the other headlines are untouched.
	if !strings.Contains(got, "* One                                                   :ProjA:") {
		t.Errorf("the file was rewritten:\n%s", got)
	}
}
