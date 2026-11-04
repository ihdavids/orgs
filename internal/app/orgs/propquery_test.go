package orgs

import (
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

func queryTitles(t *testing.T, q string) string {
	t.Helper()
	got := []string{}
	if err := evalEachSection(q, func(s *org.Section, _ *common.OrgFile) { got = append(got, headlineTitle(s)) }); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	sort.Strings(got)
	return strings.Join(got, ",")
}

func TestPropertyComparisonsAndInheritance(t *testing.T) {
	if Conf().Server == nil {
		t.Skip("no server settings")
	}
	old := Conf().Server.PropertyInheritance
	t.Cleanup(func() { Conf().Server.PropertyInheritance = old })
	Conf().Server.PropertyInheritance = nil
	loadOrg(t, map[string]string{"p.org": `#+PROPERTY: CLIENT acme
* TODO Small
:PROPERTIES:
:EFFORT: 0:30
:COST: 12
:END:
* TODO Large
SCHEDULED: <2026-11-20 Fri>
:PROPERTIES:
:EFFORT: 1d
:COST: 900
:END:
* Project
:PROPERTIES:
:CATEGORY: big
:OWNER: sam
:END:
** TODO [#A] Child
`})
	cases := []struct{ q, want string }{
		{`PropCompare("EFFORT", "<", "2:00")`, "Small"},
		{`PropCompare("EFFORT", ">=", "4h")`, "Large"},
		{`PropCompare("COST", ">", "100")`, "Large"},
		{`PropCompare("COST", "!=", "12")`, "Child,Large,Project"},
		{`PropCompare("SCHEDULED", ">", "<2026-11-19>")`, "Large"},
		{`PropCompare("PRIORITY", "=", "A")`, "Child"},
		{`PropCompare("LEVEL", ">", "1")`, "Child"},
		{`PropCompare("CATEGORY", "=", "big")`, "Child,Project"},
		// Not inherited unless the setting says so.
		{`HasProperty("OWNER")`, "Project"},
		{`InheritedProperty("OWNER", "^sam$")`, "Child,Project"},
		{`InheritedProperty("CLIENT")`, "Child,Large,Project,Small"},
		{`effort:<2h`, "Small"},
	}
	for _, c := range cases {
		if got := queryTitles(t, c.q); got != c.want {
			t.Errorf("%s: %s, want %s", c.q, got, c.want)
		}
	}
	Conf().Server.PropertyInheritance = []string{"OWNER", "CLIENT"}
	if got := queryTitles(t, `MatchProperty("OWNER", "sam")`); got != "Child,Project" {
		t.Errorf("inherited OWNER: %s", got)
	}
	if got := queryTitles(t, `MatchProperty("CLIENT", "acme")`); got != "Child,Large,Project,Small" {
		t.Errorf("file CLIENT: %s", got)
	}
}

func TestPropCompareValues(t *testing.T) {
	now := time.Date(2026, 11, 3, 9, 0, 0, 0, time.Local)
	cases := []struct {
		val, op, want string
		ok            bool
	}{
		{"<2026-11-09 Mon>", "<", "<+1w>", true},
		{"<2026-11-10 Tue>", "<", "<+1w>", false}, // the same day is not before it
		{"<2026-11-12 Thu>", "<", "<+1w>", false},
		{"[2026-11-02 Mon 10:00]", "<", "<today>", true},
		{"10", "<", "9", false}, // numbers, not text
		{"abc", "<", "abd", true},
		{"1:30", "<", "2h", true},
	}
	for _, c := range cases {
		if got := propCompare(c.val, true, c.op, c.want, now); got != c.ok {
			t.Errorf("%s %s %s = %v", c.val, c.op, c.want, got)
		}
	}
	if propCompare("", false, "<", "3", now) || !propCompare("", false, "!=", "3", now) {
		t.Errorf("a missing value is only unequal")
	}
}
