package orgs

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/ihdavids/go-org/org"

	"github.com/ihdavids/orgs/internal/common"
)

// The examples in docs/tables.org are run here, so the guide cannot drift from
// what the evaluator does. Every #+BEGIN_SRC org block holding a #+TBLFM line
// is evaluated with its formula targets blanked first, and must come back
// exactly as the guide shows it. Blocks whose table carries a #+NAME are
// registered first, so remote() examples work.

var (
	guideBlock     = regexp.MustCompile(`(?ms)^[ \t]*#\+BEGIN_SRC org[^\n]*\n(.*?)^[ \t]*#\+END_SRC`)
	guideName      = regexp.MustCompile(`(?m)^#\+NAME:\s*(\S+)`)
	guideCellTgt   = regexp.MustCompile(`@(\d+|>)\$(\d+)\s*=`)
	guideColTgt    = regexp.MustCompile(`(^|::)\s*\$(\d+)\s*=`)
	guideTblfmLine = regexp.MustCompile(`(?m)^#\+TBLFM:(.*)$`)
)

func guideTable(t *testing.T, text string) (*org.Document, *org.Section, *org.Table) {
	doc := org.New().Parse(strings.NewReader("* Example\n"+text), "guide.org")
	if len(doc.Outline.Children) == 0 || len(doc.Outline.Children[0].Headline.Tables) == 0 {
		t.Fatalf("no table in example:\n%s", text)
	}
	sec := doc.Outline.Children[0]
	return doc, sec, sec.Headline.Tables[0]
}

func dataRows(tbl *org.Table) int {
	n := 0
	for i := range tbl.Rows {
		if !tbl.IsSeparatorRow(i) {
			n++
		}
	}
	return n
}

func TestTablesGuideExamples(t *testing.T) {
	raw, err := os.ReadFile("../../../docs/tables.org")
	if err != nil {
		t.Fatal(err)
	}
	saved := odb
	odb = NewOrgDb()
	t.Cleanup(func() { odb = saved })

	blocks := []string{}
	for _, m := range guideBlock.FindAllStringSubmatch(string(raw), -1) {
		blocks = append(blocks, dedent(m[1]))
	}
	for _, b := range blocks {
		if n := guideName.FindStringSubmatch(b); n != nil {
			doc, _, tbl := guideTable(t, b)
			odb.NamedTables[n[1]] = append(odb.NamedTables[n[1]], &TableFile{Table: tbl, File: &common.OrgFile{Filename: "guide.org", Doc: doc}})
		}
	}
	ran := 0
	for _, b := range blocks {
		fm := guideTblfmLine.FindStringSubmatch(b)
		if fm == nil || !strings.Contains(b, "|") {
			continue
		}
		ran++
		_, _, shown := guideTable(t, b)
		doc, sec, tbl := guideTable(t, b)
		last := dataRows(tbl)
		for _, m := range guideCellTgt.FindAllStringSubmatch(fm[1], -1) {
			row := last
			if m[1] != ">" {
				row = atoi(m[1])
			}
			tbl.SetVal(row, atoi(m[2]), "")
		}
		for _, m := range guideColTgt.FindAllStringSubmatch(fm[1], -1) {
			for r := 2; r <= last; r++ {
				// Formulas never write a marker row (! ^ _ $).
				if real, _ := tbl.GetRealRowCol(r, 1); !ShouldSkipAdvancedRow(tbl.Rows[real].IsAdvanced) {
					tbl.SetVal(r, atoi(m[2]), "")
				}
			}
		}
		if err := ExecuteFormula(nil, sec, &common.OrgFile{Filename: "guide.org", Doc: doc}, tbl); err != nil {
			t.Errorf("example fails to run: %v\n%s", err, b)
			continue
		}
		bad := false
		for r := 1; r <= last; r++ {
			for c := 1; c <= len(tbl.Rows[0].Columns); c++ {
				if got, want := cell(tbl, r, c), cell(shown, r, c); got != want {
					t.Errorf("@%d$%d: computed %q, guide shows %q", r, c, got, want)
					bad = true
				}
			}
		}
		if bad {
			w := org.NewOrgWriter()
			tbl.RecomputeColumnInfos()
			org.WriteNodes(w, tbl)
			t.Errorf("in example:\n%s\ncomputed:\n%s", b, w.String())
		}
	}
	if ran < 10 {
		t.Errorf("only %d examples ran; is the block pattern still matching?", ran)
	}
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		n = n*10 + int(c-'0')
	}
	return n
}
