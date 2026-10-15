package orgs

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/ihdavids/orgs/internal/common"
)

// runFile runs every block and call in a file and hands back the file.
func runFile(t *testing.T, name, text string) string {
	t.Helper()
	paths := loadOrg(t, map[string]string{name: text})
	p := paths[name]
	res := ExecBabel(&common.BabelExec{Filename: p, All: true})
	got, _ := os.ReadFile(p)
	if !res.Ok {
		t.Logf("run: %s\n%s", res.Msg, got)
	}
	return string(got)
}

func wantIn(t *testing.T, got string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(got, w) {
			t.Errorf("missing\n%s\n---- in ----\n%s", w, got)
		}
	}
}

const peopleTable = `#+NAME: tbl
| name | n |
|------+---|
| a    | 1 |
| b    | 2 |
| c    | 3 |
`

func TestVarIndexing(t *testing.T) {
	need(t, "python3")
	babelOn(t, nil)
	got := runFile(t, "idx.org", peopleTable+`
#+NAME: cell
#+BEGIN_SRC python :var x=tbl[2,1] :results output
print(x * 10)
#+END_SRC

#+NAME: col
#+BEGIN_SRC python :var c=tbl[2:4,0] :results output
print(",".join(c))
#+END_SRC

#+NAME: last
#+BEGIN_SRC python :var r=tbl[-1] :results output
print(r)
#+END_SRC

#+NAME: bad
#+BEGIN_SRC python :var r=tbl[9] :results output
print(r)
#+END_SRC
`)
	wantIn(t, got, "#+RESULTS: cell\n: 10", "#+RESULTS: col\n: a,b,c", "#+RESULTS: last\n: ['c', 3]")
	if strings.Contains(got, "#+RESULTS: bad") {
		t.Errorf("an index out of range ran")
	}
}

func TestColnamesRownamesHlines(t *testing.T) {
	need(t, "python3")
	babelOn(t, nil)
	got := runFile(t, "cols.org", peopleTable+`
#+NAME: times
#+BEGIN_SRC python :var t=tbl
return [[r[0], r[1] * 10] for r in t]
#+END_SRC

#+NAME: nocols
#+BEGIN_SRC python :var t=tbl :colnames no :results output
print(t[0])
#+END_SRC

#+NAME: rows
#+BEGIN_SRC python :var t=tbl :rownames yes
return [[r[0] + 1] for r in t]
#+END_SRC

#+NAME: keep
#+BEGIN_SRC python :var t=tbl :colnames no :hlines yes :results output
print(t[1])
#+END_SRC

#+NAME: named
#+BEGIN_SRC python :colnames '(x y)
return [[1, 2]]
#+END_SRC
`)
	wantIn(t, got,
		"#+RESULTS: times\n| name | n  |\n|------+----|\n| a    | 10 |",
		"#+RESULTS: nocols\n: ['name', 'n']",
		"#+RESULTS: rows\n| name | n |\n|------+---|\n| a    | 2 |",
		"#+RESULTS: keep\n: None",
		"#+RESULTS: named\n| x | y |\n|---+---|\n| 1 | 2 |")
}

func TestStderrInResults(t *testing.T) {
	need(t, "bash")
	babelOn(t, nil)
	got := runFile(t, "err.org", `#+NAME: fails
#+BEGIN_SRC sh
echo partial
echo "it broke" >&2
exit 3
#+END_SRC

#+NAME: warns
#+BEGIN_SRC sh :stderr yes
echo answer
echo "a warning" >&2
#+END_SRC

#+NAME: quiet
#+BEGIN_SRC sh
echo answer
echo "a warning" >&2
#+END_SRC
`)
	wantIn(t, got, "#+RESULTS: fails\n: partial\n: it broke", "#+RESULTS: warns\n: answer\n: a warning", "#+RESULTS: quiet\n: answer\n")
}

func TestWrapPostPrologue(t *testing.T) {
	need(t, "bash")
	need(t, "python3")
	babelOn(t, nil)
	got := runFile(t, "wrap.org", `#+NAME: ex
#+BEGIN_SRC sh :wrap example
echo "* looks like a heading"
#+END_SRC

#+NAME: bare
#+BEGIN_SRC sh :wrap
echo hi
#+END_SRC

#+NAME: double
#+BEGIN_SRC python :var x=0
return int(x) * 2
#+END_SRC

#+NAME: posted
#+BEGIN_SRC sh :post double(x=*this*)
echo 21
#+END_SRC

#+NAME: around
#+BEGIN_SRC sh :prologue "echo first" :epilogue "echo last"
echo middle
#+END_SRC
`)
	wantIn(t, got,
		"#+RESULTS: ex\n#+begin_example\n,* looks like a heading\n#+end_example",
		"#+RESULTS: bare\n#+begin_results\nhi\n#+end_results",
		"#+RESULTS: posted\n: 42",
		"#+RESULTS: around\n: first\n: middle\n: last")
}

func TestCache(t *testing.T) {
	need(t, "bash")
	babelOn(t, nil)
	paths := loadOrg(t, map[string]string{"cache.org": `#+NAME: slow
#+BEGIN_SRC sh :cache yes :var n=1
echo "$n $RANDOM$RANDOM"
#+END_SRC
`})
	p := paths["cache.org"]
	ExecBabel(&common.BabelExec{Filename: p, All: true})
	first, _ := os.ReadFile(p)
	if !regexp.MustCompile(`#\+RESULTS\[[0-9a-f]{40}\]: slow\n: 1 \d+`).Match(first) {
		t.Fatalf("no hashed result:\n%s", first)
	}
	res := ExecBabel(&common.BabelExec{Filename: p, All: true})
	second, _ := os.ReadFile(p)
	if string(second) != string(first) || !res.Ran[0].Result.Cached {
		t.Errorf("a cached block ran again:\n%s\n%s", first, second)
	}
	// A different input is a different hash, and runs.
	os.WriteFile(p, []byte(strings.Replace(string(second), ":var n=1", ":var n=2", 1)), 0644)
	GetDb().ReloadFile(p)
	ExecBabel(&common.BabelExec{Filename: p, All: true})
	third, _ := os.ReadFile(p)
	if !strings.Contains(string(third), ": 2 ") {
		t.Errorf("a changed input did not run:\n%s", third)
	}
}

func TestNowebWhenRunning(t *testing.T) {
	need(t, "bash")
	babelOn(t, nil)
	got := runFile(t, "noweb.org", `#+NAME: greet
#+BEGIN_SRC sh
echo hello
echo there
#+END_SRC

#+NAME: answer
#+BEGIN_SRC sh :var n=40
echo $((n + 2))
#+END_SRC

#+NAME: main
#+BEGIN_SRC sh :noweb yes
<<greet>>
echo "<<answer(n=1)>> and <<answer()>>"
echo "a << b >> c"
#+END_SRC

#+NAME: off
#+BEGIN_SRC sh
echo "<<greet>>"
#+END_SRC
`)
	wantIn(t, got, "#+RESULTS: main\n: hello\n: there\n: 3 and 42\n: a << b >> c", "#+RESULTS: off\n: <<greet>>")
}

func TestMoreLanguagesTakeVars(t *testing.T) {
	babelOn(t, nil)
	text := peopleTable + `
#+NAME: awk
#+BEGIN_SRC awk :var who="world" :stdin tbl
BEGIN { print "hello " who }
{ total += $2 }
END { print total }
#+END_SRC

#+NAME: sql
#+BEGIN_SRC sqlite :var t=tbl :var lim=2 :colnames yes
.import $t people
select name, n * 100 as big from people limit $lim;
#+END_SRC

#+NAME: golang
#+BEGIN_SRC go :var t=tbl :var label="rows"
fmt.Println(label, len(t))
#+END_SRC

#+NAME: elisp
#+BEGIN_SRC emacs-lisp :var tb=tbl :var k=5
(princ (format "%d %s" (+ k (length tb)) (car (car tb))))
#+END_SRC
`
	for _, bin := range []string{"awk", "sqlite3", "go", "emacs"} {
		if _, err := os.Stat("/usr/bin/" + bin); err != nil {
			if _, err := os.Stat("/opt/homebrew/bin/" + bin); err != nil {
				t.Skipf("%s is not installed", bin)
			}
		}
	}
	got := runFile(t, "langs.org", text)
	wantIn(t, got,
		"#+RESULTS: awk\n: hello world\n: 6",
		"#+RESULTS: sql\n| name | big |\n|------+-----|\n| a    | 100 |\n| b    | 200 |",
		"#+RESULTS: golang\n: rows 3",
		"#+RESULTS: elisp\n: 8 a")
}

func TestCrossFileVarAndResultsMerge(t *testing.T) {
	need(t, "python3")
	babelOn(t, nil)
	paths := loadOrg(t, map[string]string{
		"data.org": peopleTable,
		"use.org": `#+PROPERTY: header-args :results append
#+NAME: count
#+BEGIN_SRC python :var t=data.org:tbl :results replace output
print(len(t))
#+END_SRC
`,
	})
	p := paths["use.org"]
	ExecBabel(&common.BabelExec{Filename: p, All: true})
	ExecBabel(&common.BabelExec{Filename: p, All: true})
	got, _ := os.ReadFile(p)
	if !strings.Contains(string(got), "#+RESULTS: count\n: 3\n") || strings.Count(string(got), ": 3") != 1 {
		t.Errorf("cross-file var or :results replace over an inherited append:\n%s", got)
	}
}

func TestResultsLinkAndPp(t *testing.T) {
	need(t, "bash")
	need(t, "python3")
	babelOn(t, nil)
	paths := loadOrg(t, map[string]string{"link.org": `#+NAME: writes
#+BEGIN_SRC sh :file out.txt :results file link
echo "in the file" > out.txt
echo "on stdout"
#+END_SRC

#+NAME: pretty
#+BEGIN_SRC python :results value pp
return {"b": "y" * 50, "a": "x" * 40}
#+END_SRC
`})
	p := paths["link.org"]
	ExecBabel(&common.BabelExec{Filename: p, All: true})
	got, _ := os.ReadFile(p)
	wantIn(t, string(got), "#+RESULTS: writes\n[[file:out.txt]]", "#+RESULTS: pretty\n: {'a': ")
	if b, _ := os.ReadFile(filepath.Join(filepath.Dir(p), "out.txt")); string(b) != "in the file\n" {
		t.Errorf(":results link wrote stdout over the file: %q", b)
	}
	if !strings.Contains(string(got), "\n:  'b': 'yyy") {
		t.Errorf("pp did not pretty print:\n%s", got)
	}
}

func TestApplyIndex(t *testing.T) {
	tbl := tableVal("d", [][]string{{"h1", "h2"}, nil, {"a", "1"}, {"b", "2"}, {"c", "3"}})
	cases := map[string]string{
		"[2,1]":   "text:1",
		"[2:4,0]": "list:a b c",
		"[-1]":    "list:c 3",
		"[,0]":    "list:h1 - a b c",
		"[2:3]":   "table:a,1|b,2",
		"[4:2,1]": "list:3 2 1",
		"[*,1]":   "list:h2 - 1 2 3",
	}
	show := func(b babelVal) string {
		switch {
		case b.isTbl:
			rs := []string{}
			for _, r := range b.rows {
				rs = append(rs, strings.Join(r, ","))
			}
			return "table:" + strings.Join(rs, "|")
		case b.isList:
			items := []string{}
			for _, c := range b.list {
				if c == "" {
					c = "-"
				}
				items = append(items, c)
			}
			return "list:" + strings.Join(items, " ")
		}
		return "text:" + b.v.Value
	}
	for idx, want := range cases {
		got, err := applyIndex(tbl, idx)
		if err != nil || show(got) != want {
			t.Errorf("%s: %s %v, want %s", idx, show(got), err, want)
		}
	}
	if _, err := applyIndex(tbl, "[7]"); err == nil {
		t.Errorf("out of range should say so")
	}
	if _, err := applyIndex(scalarVal(common.CodeVar{Name: "n", Value: "3"}), "[0]"); err == nil {
		t.Errorf("a scalar cannot be indexed")
	}
}

func TestResultsMerge(t *testing.T) {
	j := babelJob{Args: []common.CodeArg{{Key: "results", Value: "append output table"}, {Key: "results", Value: "replace raw"}, {Key: "results", Value: "value"}}}
	if got := strings.Join(j.results(), " "); got != "value table raw replace" {
		t.Errorf("got %q", got)
	}
}

func TestTangleEvaluatesCalls(t *testing.T) {
	need(t, "bash")
	babelOn(t, nil)
	tangleHome(t)
	paths := loadOrg(t, map[string]string{"gen.org": `* Gen
#+NAME: version
#+BEGIN_SRC sh
echo 1.2.3
#+END_SRC
#+BEGIN_SRC sh :tangle out.sh :noweb yes
# version <<version()>>
#+END_SRC
`})
	p := paths["gen.org"]
	tangleNow(t, p)
	got, _ := os.ReadFile(filepath.Join(filepath.Dir(p), "out.sh"))
	if string(got) != "# version 1.2.3\n" {
		t.Errorf("%q", got)
	}
}
