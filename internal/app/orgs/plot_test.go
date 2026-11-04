package orgs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const plotFixture = `* Sales
#+PLOT: title:"Sales" ind:1 deps:(2 3) type:2d with:lines set:"yrange [0:]" file:"sales.svg"
| Month | North | South |
|-------+-------+-------|
| Jan   |    10 |     4 |
| Feb   |    12 |     6 |
`

func TestPlotOptionsAndScript(t *testing.T) {
	o := plotOptions(`title:"Sales by month" ind:1 deps:(2 3) type:2d with:histograms set:"yrange [0:]" set:"grid" file:"s.png"`)
	if o["title"][0] != "Sales by month" || o["deps"][0] != "(2 3)" || len(o["set"]) != 2 || o["file"][0] != "s.png" {
		t.Errorf("options: %v", o)
	}
	lines := strings.Split(plotFixture, "\n")
	rows, names, end := plotTableRows(lines, 2)
	if len(rows) != 2 || strings.Join(names, ",") != "Month,North,South" || end != 5 {
		t.Errorf("rows %v names %v end %d", rows, names, end)
	}
	s := plotScript(plotOptions(lines[1][len("#+PLOT: "):]), "/d.tsv", "/o/sales.svg", names, rows)
	for _, want := range []string{"set terminal svg", `set output "/o/sales.svg"`, `set title "Sales"`, "set yrange [0:]",
		`plot "/d.tsv" using 2:xticlabels(1) with lines title "North", "/d.tsv" using 3:xticlabels(1) with lines title "South"`} {
		if !strings.Contains(s, want) {
			t.Errorf("no %q in\n%s", want, s)
		}
	}
	if got := plotLineAbove(lines, 2); !strings.HasPrefix(got, `title:"Sales"`) {
		t.Errorf("plot line: %q", got)
	}
}

func TestPlotIsDrawnOnExport(t *testing.T) {
	need(t, "gnuplot")
	babelOn(t, nil)
	paths := loadOrg(t, map[string]string{"p.org": plotFixture})
	p := paths["p.org"]
	old := Conf().Server.OrgDirs
	Conf().Server.OrgDirs = []string{filepath.Dir(p)}
	t.Cleanup(func() { Conf().Server.OrgDirs = old })
	text, ok := exportTextFor(p, false, "html")
	if !ok || !strings.Contains(text, "| Feb   |    12 |     6 |\n\n[[file:sales.svg]]") {
		t.Fatalf("no link under the table:\n%s", text)
	}
	b, err := os.ReadFile(filepath.Join(filepath.Dir(p), "sales.svg"))
	if err != nil || !strings.Contains(string(b), "<svg") {
		t.Errorf("no picture: %v", err)
	}
	// The file keeps its lines.
	if strings.Contains(readOrg(t, p), "[[file:sales.svg]]") {
		t.Errorf("the file was written")
	}
}
