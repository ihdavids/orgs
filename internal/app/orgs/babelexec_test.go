package orgs

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ihdavids/orgs/internal/common"
)

// Babel on, for the length of a test, with the commands given.
func babelOn(t *testing.T, commands map[string][]string) {
	t.Helper()
	if Conf().Server == nil {
		t.Skip("no server settings")
	}
	old := Conf().Server.Babel
	Conf().Server.Babel = common.BabelSettings{Enable: true, Commands: commands}
	t.Cleanup(func() { Conf().Server.Babel = old })
}

func need(t *testing.T, bin string) {
	t.Helper()
	if _, err := exec.LookPath(bin); err != nil {
		t.Skipf("%s is not installed", bin)
	}
}

func execAt(t *testing.T, path string, line int) common.BabelExecResult {
	t.Helper()
	res := ExecBabel(&common.BabelExec{Filename: path, Line: line})
	if !res.Ok {
		t.Fatalf("exec line %d: %s (%+v)", line+1, res.Msg, res.Ran)
	}
	return res
}

func TestABlocksResultIsWrittenUnderItAndReplacedNextTime(t *testing.T) {
	need(t, "bash")
	babelOn(t, nil)
	paths := loadOrg(t, map[string]string{"r.org": `* Heading
  #+BEGIN_SRC sh
    echo one
    echo two
  #+END_SRC
Text after.
`})
	execAt(t, paths["r.org"], 2)
	want := `* Heading
  #+BEGIN_SRC sh
    echo one
    echo two
  #+END_SRC

  #+RESULTS:
  : one
  : two
Text after.`
	wantOrg(t, "first run", readOrg(t, paths["r.org"]), want)

	// Run again: the result is replaced, not added to.
	execAt(t, paths["r.org"], 1)
	wantOrg(t, "second run", readOrg(t, paths["r.org"]), want)
}

func TestATableResultIsAnOrgTable(t *testing.T) {
	need(t, "python3")
	babelOn(t, nil)
	paths := loadOrg(t, map[string]string{"t.org": `* H
#+NAME: data
| a | 1 |
| b | 2 |

#+NAME: doubled
#+BEGIN_SRC python :var rows=data :results table
return [[r[0], r[1] * 2] for r in rows]
#+END_SRC
`})
	execAt(t, paths["t.org"], 6)
	text := readOrg(t, paths["t.org"])
	if !strings.Contains(text, "#+RESULTS: doubled\n| a | 2 |\n| b | 4 |") {
		t.Errorf("table result:\n%s", text)
	}
}

func TestACallRunsTheNamedBlockWithItsArguments(t *testing.T) {
	need(t, "python3")
	babelOn(t, nil)
	paths := loadOrg(t, map[string]string{
		"lib.org": `* Library
#+NAME: double
#+BEGIN_SRC python :var n=1
return n * 2
#+END_SRC
`,
		"use.org": `* Uses it
#+CALL: double(n=21)
`,
	})
	res := execAt(t, paths["use.org"], 1)
	if len(res.Ran) != 1 || res.Ran[0].Kind != "call" || res.Ran[0].Name != "double" {
		t.Errorf("ran: %+v", res.Ran)
	}
	wantOrg(t, "call", readOrg(t, paths["use.org"]), `* Uses it
#+CALL: double(n=21)

#+RESULTS:
: 42`)
}

func TestAVariableNamingABlockTakesItsResult(t *testing.T) {
	need(t, "python3")
	babelOn(t, nil)
	paths := loadOrg(t, map[string]string{"v.org": `* H
#+NAME: base
#+BEGIN_SRC python
return 20
#+END_SRC

#+BEGIN_SRC python :var b=base
return b + 1
#+END_SRC
`})
	execAt(t, paths["v.org"], 6)
	if text := readOrg(t, paths["v.org"]); !strings.HasSuffix(text, "#+RESULTS:\n: 21") {
		t.Errorf("result:\n%s", text)
	}
}

func TestInlineCodeWritesItsResultAfterItself(t *testing.T) {
	need(t, "python3")
	babelOn(t, nil)
	paths := loadOrg(t, map[string]string{"i.org": `* H
#+NAME: square
#+BEGIN_SRC python :var x=2
return x * x
#+END_SRC

The answer is src_python{return 6*7} and call_square(x=5) squared, but not my_src_python{1}.
`})
	execAt(t, paths["i.org"], 6)
	want := "The answer is src_python{return 6*7} {{{results(=42=)}}} and call_square(x=5) {{{results(=25=)}}} squared, but not my_src_python{1}."
	text := readOrg(t, paths["i.org"])
	if !strings.HasSuffix(text, want) {
		t.Errorf("inline:\n%s", text)
	}
	// Again: the results are replaced in place.
	execAt(t, paths["i.org"], 6)
	if text := readOrg(t, paths["i.org"]); !strings.HasSuffix(text, want) {
		t.Errorf("inline, second run:\n%s", text)
	}
}

func TestEvalNoIsNeverRun(t *testing.T) {
	need(t, "bash")
	babelOn(t, nil)
	paths := loadOrg(t, map[string]string{"n.org": `* H
#+BEGIN_SRC sh :eval no
echo ran
#+END_SRC
`})
	res := ExecBabel(&common.BabelExec{Filename: paths["n.org"], Line: 1})
	if res.Ok || !strings.Contains(res.Msg, ":eval no") {
		t.Errorf("an :eval no block ran: %+v", res)
	}
	if strings.Contains(readOrg(t, paths["n.org"]), "RESULTS") {
		t.Errorf("something was written")
	}
}

// Running a whole file runs everything in order, and each result lands under
// its own block even though every write moves the lines below it.
func TestRunningAFileRunsEverythingInOrder(t *testing.T) {
	need(t, "bash")
	babelOn(t, nil)
	paths := loadOrg(t, map[string]string{"a.org": `* H
#+NAME: first
#+BEGIN_SRC sh
echo 1
#+END_SRC
#+BEGIN_SRC sh :results silent
echo nothing written
#+END_SRC
#+CALL: first()
#+BEGIN_SRC sh
echo 3
#+END_SRC
`})
	res := ExecBabel(&common.BabelExec{Filename: paths["a.org"], All: true})
	if !res.Ok || len(res.Ran) != 4 {
		t.Fatalf("ran: %+v", res)
	}
	wantOrg(t, "all", readOrg(t, paths["a.org"]), `* H
#+NAME: first
#+BEGIN_SRC sh
echo 1
#+END_SRC

#+RESULTS: first
: 1
#+BEGIN_SRC sh :results silent
echo nothing written
#+END_SRC
#+CALL: first()

#+RESULTS:
: 1
#+BEGIN_SRC sh
echo 3
#+END_SRC

#+RESULTS:
: 3`)
}

// A drawing language's result is the picture :file names. The real programs
// are not needed to see that the plumbing is right: a stand-in for dot that
// writes its -o argument does.
func TestAPictureBlockLinksToItsFile(t *testing.T) {
	need(t, "sh")
	babelOn(t, map[string][]string{
		"dot": {"sh", "-c", `cat > /dev/null; printf 'PNG' > "$3"`, "dot"},
	})
	paths := loadOrg(t, map[string]string{"p.org": `* H
#+BEGIN_SRC dot :file out/graph.png
digraph { a -> b }
#+END_SRC
`})
	execAt(t, paths["p.org"], 1)
	if text := readOrg(t, paths["p.org"]); !strings.HasSuffix(text, "#+RESULTS:\n[[file:out/graph.png]]") {
		t.Errorf("picture result:\n%s", text)
	}
	if b, err := os.ReadFile(filepath.Join(filepath.Dir(paths["p.org"]), "out", "graph.png")); err != nil || string(b) != "PNG" {
		t.Errorf("the picture was not drawn where :file said: %v %q", err, b)
	}
}

func TestAPictureBlockWithoutAFileSaysSo(t *testing.T) {
	babelOn(t, nil)
	paths := loadOrg(t, map[string]string{"p.org": `* H
#+BEGIN_SRC dot
digraph { a -> b }
#+END_SRC
`})
	res := ExecBabel(&common.BabelExec{Filename: paths["p.org"], Line: 1})
	if res.Ok || !strings.Contains(res.Msg, ":file") {
		t.Errorf("want a refusal naming :file, got %+v", res)
	}
}

func TestResultShapes(t *testing.T) {
	long := strings.Repeat("x\n", 11)
	cases := []struct {
		name string
		res  common.CodeResult
		want []string
		out  string
	}{
		{"short text", common.CodeResult{Kind: "text", Result: "a\n\nb"}, nil, ": a\n:\n: b"},
		{"long text", common.CodeResult{Kind: "text", Result: long}, nil, "#+begin_example\n" + strings.TrimSuffix(long, "\n") + "\n#+end_example"},
		{"raw", common.CodeResult{Kind: "text", Result: "*bold*"}, []string{"raw"}, "*bold*"},
		{"drawer", common.CodeResult{Kind: "text", Result: "x"}, []string{"drawer"}, ":RESULTS:\nx\n:END:"},
		{"html", common.CodeResult{Kind: "text", Result: "<b>"}, []string{"html"}, "#+begin_export html\n<b>\n#+end_export"},
		{"escaped", common.CodeResult{Kind: "text", Result: "* not a heading\n" + long}, nil, ""},
	}
	for _, c := range cases {
		got := strings.Join(resultBody(&c.res, c.want, "python"), "\n")
		if c.name == "escaped" {
			if !strings.Contains(got, "\n,* not a heading\n") {
				t.Errorf("escaped: %q", got)
			}
			continue
		}
		if got != c.out {
			t.Errorf("%s: got %q, want %q", c.name, got, c.out)
		}
	}
}

func TestFindingWhatToRun(t *testing.T) {
	lines := strings.Split(`* H
#+BEGIN_SRC python
x = "src_python{not inline}"
#+END_SRC
#+BEGIN_EXAMPLE
#+CALL: not_a_call()
#+END_EXAMPLE
#+CALL: real()
Some src_sh{echo hi} text.
: src_python{a result line}`, "\n")
	items := babelItems(lines)
	kinds := []string{}
	for _, it := range items {
		kinds = append(kinds, it.kind+":"+string(rune('0'+it.row)))
	}
	if strings.Join(kinds, " ") != "block:1 call:7 inline:8" {
		t.Errorf("found %v", kinds)
	}
}

func TestCmdlineIsSplitLikeAShellWithoutOne(t *testing.T) {
	got := shellWords(`SELECT account, sum(position) "GROUP BY" '' x`)
	want := []string{"SELECT", "account,", "sum(position)", "GROUP BY", "", "x"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("got %q", got)
	}
}

// A beancount block is the journal; :cmdline is the query, and the csv
// bean-query prints comes back as a table. A stand-in for bean-query prints
// its query words joined, as bean-query joins them, which is enough to see
// both ends.
func TestABeancountBlockAnswersWithATable(t *testing.T) {
	need(t, "sh")
	babelOn(t, map[string][]string{
		"beancount": {"sh", "-c", `test -f "$1" || exit 3; shift; printf 'account,total\nAssets:Bank,%s\n' "$*"`, "bq"},
	})
	paths := loadOrg(t, map[string]string{"b.org": `* Money
#+BEGIN_SRC beancount :cmdline "100 CAD"
2026-01-01 open Assets:Bank
#+END_SRC
`})
	execAt(t, paths["b.org"], 1)
	if text := readOrg(t, paths["b.org"]); !strings.HasSuffix(text, "#+RESULTS:\n| account     | total   |\n|-------------+---------|\n| Assets:Bank | 100 CAD |") {
		t.Errorf("beancount result:\n%s", text)
	}
}
