package orgs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ihdavids/orgs/internal/common"
)

// Babel on with python sessions stopped at the end, so no test leaves a
// python behind or hands its globals to the next.
func sessionsOn(t *testing.T) {
	t.Helper()
	need(t, "python3")
	babelOn(t, nil)
	t.Cleanup(StopBabelSessions)
}

func runAll(t *testing.T, path string) common.BabelExecResult {
	t.Helper()
	return ExecBabel(&common.BabelExec{Filename: path, All: true})
}

func TestASessionKeepsWhatAnEarlierBlockMade(t *testing.T) {
	sessionsOn(t)
	paths := loadOrg(t, map[string]string{"s.org": `#+PROPERTY: header-args:python :session s :results output
* Cells
#+BEGIN_SRC python
x = 21
print("set")
#+END_SRC

#+BEGIN_SRC python
print(x * 2)
x
#+END_SRC

#+BEGIN_SRC python
x + 1;
#+END_SRC
`})
	res := runAll(t, paths["s.org"])
	if !res.Ok {
		t.Fatalf("run all: %s %+v", res.Msg, res.Ran)
	}
	want := `#+PROPERTY: header-args:python :session s :results output
* Cells
#+BEGIN_SRC python
x = 21
print("set")
#+END_SRC

#+RESULTS:
: set

#+BEGIN_SRC python
print(x * 2)
x
#+END_SRC

#+RESULTS:
: 42
: 21

#+BEGIN_SRC python
x + 1;
#+END_SRC`
	wantOrg(t, "first run", readOrg(t, paths["s.org"]), want)

	// Again: replaced, not added to, and the session is the same one.
	res = runAll(t, paths["s.org"])
	if !res.Ok {
		t.Fatalf("second run: %s", res.Msg)
	}
	wantOrg(t, "second run", readOrg(t, paths["s.org"]), want)
}

func TestWithoutASessionEveryBlockStartsAfresh(t *testing.T) {
	sessionsOn(t)
	paths := loadOrg(t, map[string]string{"n.org": `* H
#+BEGIN_SRC python :results output
y = 1
#+END_SRC

#+BEGIN_SRC python :results output :session none
print(y)
#+END_SRC
`})
	res := runAll(t, paths["n.org"])
	if res.Ok || len(res.Ran) != 2 || res.Ran[1].Result.Ok {
		t.Fatalf("the second block should not have seen y: %+v", res)
	}
}

func TestASessionErrorIsWrittenAsItsTraceback(t *testing.T) {
	sessionsOn(t)
	paths := loadOrg(t, map[string]string{"e.org": `* H
#+BEGIN_SRC python :session e :results output
print("before")
1 / 0
#+END_SRC
`})
	res := ExecBabel(&common.BabelExec{Filename: paths["e.org"], Line: 2})
	if res.Ok || len(res.Ran) != 1 {
		t.Fatalf("a failing block should report failure: %+v", res)
	}
	r := res.Ran[0].Result
	if r.Kind != "error" || !strings.Contains(r.Msg, "ZeroDivisionError") {
		t.Fatalf("kind %q msg %q", r.Kind, r.Msg)
	}
	got := readOrg(t, paths["e.org"])
	for _, w := range []string{"#+RESULTS:", ": before", ": Traceback (most recent call last):", `File "<e.org:2>", line 2`, ": ZeroDivisionError: division by zero"} {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q in\n%s", w, got)
		}
	}
	if strings.Contains(got, "<string>") {
		t.Errorf("the driver's own frames leaked into the traceback:\n%s", got)
	}
	// A second run replaces the traceback rather than stacking another.
	ExecBabel(&common.BabelExec{Filename: paths["e.org"], Line: 2})
	if again := readOrg(t, paths["e.org"]); strings.Count(again, "#+RESULTS:") != 1 || again != got {
		t.Errorf("second run changed the file:\n%s", again)
	}
}

func TestASessionSavesTheFiguresABlockDrew(t *testing.T) {
	sessionsOn(t)
	// A stand-in for matplotlib, so the test needs nothing installed: the
	// driver only asks pyplot for its figure numbers, a figure, and close.
	paths := loadOrg(t, map[string]string{"f.org": `* H
#+BEGIN_SRC python :session f :results output :file pics/plot.png
import sys, types
class Fig:
    def savefig(self, path, **kw):
        open(path, 'wb').write(b'\x89PNG not really')
plt = types.ModuleType('matplotlib.pyplot')
plt._figs = {1: Fig(), 2: Fig()}
plt.get_fignums = lambda: sorted(plt._figs)
plt.figure = lambda n: plt._figs[n]
plt.close = lambda which='all': plt._figs.clear()
sys.modules['matplotlib.pyplot'] = plt
print("drew two")
#+END_SRC
`})
	execAt(t, paths["f.org"], 2)
	got := readOrg(t, paths["f.org"])
	want := `#+RESULTS:
:results:
: drew two
[[file:pics/plot.png]]
[[file:pics/plot-2.png]]
:end:`
	if !strings.HasSuffix(got, want) {
		t.Fatalf("got\n%s\nwant it to end\n%s", got, want)
	}
	dir := filepath.Dir(paths["f.org"])
	for _, f := range []string{"pics/plot.png", "pics/plot-2.png"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("%s was not written: %v", f, err)
		}
	}
	// Run again: the drawer is replaced whole.
	execAt(t, paths["f.org"], 2)
	if again := readOrg(t, paths["f.org"]); again != got {
		t.Errorf("second run changed the file:\n%s", again)
	}
}

func TestASessionValueBlockReturns(t *testing.T) {
	sessionsOn(t)
	paths := loadOrg(t, map[string]string{"v.org": `* H
#+BEGIN_SRC python :session v
rows = [[1, 2], [3, 4]]
#+END_SRC

#+BEGIN_SRC python :session v
print("not this")
return rows
#+END_SRC
`})
	res := runAll(t, paths["v.org"])
	if !res.Ok {
		t.Fatalf("%s", res.Msg)
	}
	if r := res.Ran[1].Result; r.Kind != "table" || !strings.Contains(r.Result, "| 1 | 2 |") {
		t.Fatalf("value of the return should be the table: %+v", r)
	}
}

func TestASessionRunsCommonNotebookMagics(t *testing.T) {
	sessionsOn(t)
	paths := loadOrg(t, map[string]string{"m.org": `* H
#+BEGIN_SRC python :session m :results output
%matplotlib inline
!echo from the shell
print("and python")
#+END_SRC
`})
	execAt(t, paths["m.org"], 2)
	got := readOrg(t, paths["m.org"])
	if !strings.Contains(got, ": from the shell\n: and python") {
		t.Fatalf("got\n%s", got)
	}
}

func TestATimedOutSessionBlockIsInterruptedAndTheSessionKept(t *testing.T) {
	sessionsOn(t)
	Conf().Server.Babel.Timeout = 1
	paths := loadOrg(t, map[string]string{"t.org": `* H
#+BEGIN_SRC python :session t :results output
kept = "still here"
#+END_SRC

#+BEGIN_SRC python :session t :results output
import time
time.sleep(30)
#+END_SRC

#+BEGIN_SRC python :session t :results output
print(kept)
#+END_SRC
`})
	res := runAll(t, paths["t.org"])
	if len(res.Ran) != 3 {
		t.Fatalf("%+v", res)
	}
	if r := res.Ran[1].Result; r.Ok || !strings.Contains(r.Msg, "interrupted") {
		t.Errorf("the sleeping block should have been interrupted: %+v", r)
	}
	if r := res.Ran[2].Result; !r.Ok || strings.TrimSpace(r.Result) != "still here" {
		t.Errorf("the session lost what it had: %+v", r)
	}
}

func TestHeaderArgumentsAreInheritedFromHeadings(t *testing.T) {
	need(t, "python3")
	babelOn(t, nil)
	paths := loadOrg(t, map[string]string{"h.org": `#+PROPERTY: header-args :results silent
* Analysis
:PROPERTIES:
:header-args:python: :session inherited :results output
:END:
** Step
#+BEGIN_SRC python
print(1)
#+END_SRC
`})
	f := GetDb().FindByFile(paths["h.org"])
	block, _, err := findCodeBlock(f.Doc.Path, 0)
	if err != nil {
		t.Fatal(err)
	}
	j := jobForBlock(block, f)
	if s := j.sessionName(); s != "inherited" {
		t.Errorf("session %q", s)
	}
	// The heading's :results output and the file's silent are different
	// groups (collection, handling), so both are in force, as in org; a
	// nearer word replaces a farther one only within its group.
	if w := j.results(); !hasWord(w, "output") || !hasWord(w, "silent") {
		t.Errorf("results %v", w)
	}
}

func TestComposeResult(t *testing.T) {
	cases := []struct {
		text  string
		table [][]string
		links []string
		kind  string
		want  string
	}{
		{"hello", nil, nil, "text", "hello"},
		{"", [][]string{{"a", "b"}, {"1", "x|y"}}, nil, "table", "| a | b         |\n|---+-----------|\n| 1 | x\\vert{}y |"},
		{"", nil, []string{"[[file:a.png]]"}, "file", "[[file:a.png]]"},
		{"hi", nil, []string{"[[file:a.png]]"}, "drawer", ":results:\n: hi\n[[file:a.png]]\n:end:"},
	}
	for _, c := range cases {
		k, r := composeResult(c.text, c.table, c.links, nil)
		if k != c.kind || r != c.want {
			t.Errorf("%q: got %s\n%s\nwant %s\n%s", c.text, k, r, c.kind, c.want)
		}
	}
}
