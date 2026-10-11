package orgs

import (
	"strings"
	"testing"

	"github.com/ihdavids/go-org/org"
)

func firstTable(t *testing.T, src string) *org.Table {
	doc := org.New().Parse(strings.NewReader(src), "t.org")
	for _, n := range doc.Nodes {
		if tb, ok := n.(org.Table); ok {
			return &tb
		}
		if tb, ok := n.(*org.Table); ok {
			return tb
		}
	}
	t.Fatalf("no table in %q", src)
	return nil
}

// A random row is a data row: never the header, never a rule.
func TestTableDataRows(t *testing.T) {
	cases := map[string]int{
		"| Name | Age |\n|------+-----|\n| Ann | 3 |\n| Bob | 4 |\n|------+-----|\n| Cy | 5 |\n": 3,
		"| a |\n| b |\n| c |\n":           3, // no rule: no header either
		"| Only | Header |\n|---+---|\n": 1, // nothing under the rule: the row above is data
		"|---+---|\n":                     0,
	}
	for src, want := range cases {
		tb := firstTable(t, src)
		got := tableDataRows(tb)
		if len(got) != want {
			t.Errorf("%q: %d data rows %v, want %d", src, len(got), got, want)
		}
		for _, i := range got {
			if len(tb.Rows[i].Columns) == 0 {
				t.Errorf("%q: picked a rule", src)
			}
		}
	}
}
