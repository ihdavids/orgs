package orgs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ihdavids/orgs/internal/common"
)

func jobAt(t *testing.T, path string, n int) babelJob {
	t.Helper()
	f := GetDb().FindByFile(path)
	block, _, err := findCodeBlock(f.Doc.Path, n)
	if err != nil {
		t.Fatal(err)
	}
	return jobForBlock(block, f)
}

func argIs(t *testing.T, j babelJob, key, want string) {
	t.Helper()
	got, ok := j.arg(key)
	if want == "" && ok {
		t.Errorf(":%s is %q, want unset", key, got)
	} else if want != "" && got != want {
		t.Errorf(":%s is %q, want %q", key, got, want)
	}
}

func TestHeaderArgsInheritAsOrgDoes(t *testing.T) {
	babelOn(t, nil)
	Conf().Server.Babel.DefaultHeaderArgs = ":results output :cache yes"
	Conf().Server.Babel.LanguageHeaderArgs = map[string]string{"bash": ":cmdline -x", "python": ":session p"}
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "setup.org"), []byte("#+PROPERTY: header-args :dir from-setup\n"), 0644)
	paths := loadOrg(t, map[string]string{"h.org": `#+SETUPFILE: ` + filepath.Join(dir, "setup.org") + `
#+PROPERTY: header-args+ :session file
* Replaced
:PROPERTIES:
:header-args: :results silent
:END:
#+BEGIN_SRC sh
echo 1
#+END_SRC
* Added to
:PROPERTIES:
:header-args+: :results silent
:END:
#+HEADER: :stdin t
#+BEGIN_SRC sh :cache no
echo 2
#+END_SRC
`})
	// A heading's header-args replaces the file's whole value; the yaml's
	// defaults sit under everything and are merged key by key.
	j := jobAt(t, paths["h.org"], 0)
	argIs(t, j, "results", "silent")
	argIs(t, j, "session", "")
	argIs(t, j, "dir", "")
	argIs(t, j, "cache", "yes")
	argIs(t, j, "cmdline", "-x") // bash's defaults reach sh
	// header-args+ adds to the file's (and its setup file's) value.
	j = jobAt(t, paths["h.org"], 1)
	argIs(t, j, "session", "file")
	argIs(t, j, "dir", "from-setup")
	argIs(t, j, "results", "silent")
	argIs(t, j, "stdin", "t")
	argIs(t, j, "cache", "no")
}

func TestEvalQueryRunsOnlyWhenAsked(t *testing.T) {
	need(t, "sh")
	babelOn(t, nil)
	paths := loadOrg(t, map[string]string{"q.org": "#+NAME: q\n#+BEGIN_SRC sh :eval query\necho asked\n#+END_SRC\n\n#+BEGIN_SRC sh :eval never-export\necho fine\n#+END_SRC\n"})
	p := paths["q.org"]
	all := ExecBabel(&common.BabelExec{Filename: p, All: true})
	if len(all.Ran) != 2 || all.Ran[0].Result.Ok || !strings.Contains(all.Ran[0].Result.Msg, ":eval query") || !all.Ran[1].Result.Ok {
		t.Errorf("run all: %+v", all.Ran)
	}
	one := execAt(t, p, 1)
	if len(one.Ran) != 1 || !one.Ran[0].Result.Ok {
		t.Errorf("run at the line: %+v", one.Ran)
	}
	res, err := RunBlock(&common.CodeRun{Filename: p, Id: 0})
	if err != nil || !res.Ok {
		t.Errorf("run the block: %v %+v", err, res)
	}
}

func TestFileExtOutputDirDescAndMode(t *testing.T) {
	need(t, "sh")
	babelOn(t, nil)
	paths := loadOrg(t, map[string]string{"f.org": `#+NAME: report
#+BEGIN_SRC sh :results file :file-ext txt :output-dir out :mkdirp yes :file-desc :file-mode (identity #o600)
echo hi
#+END_SRC

#+BEGIN_SRC sh :results file :file plain.txt :file-desc Plain
echo there
#+END_SRC
`})
	p := paths["f.org"]
	if res := ExecBabel(&common.BabelExec{Filename: p, All: true}); !res.Ok {
		t.Fatalf("run: %s %+v", res.Msg, res.Ran)
	}
	dir := filepath.Dir(p)
	st, err := os.Stat(filepath.Join(dir, "out", "report.txt"))
	if err != nil || st.Mode().Perm() != 0600 {
		t.Errorf("out/report.txt: %v %v", err, st)
	}
	got := readOrg(t, p)
	for _, want := range []string{"[[file:out/report.txt][out/report.txt]]", "[[file:plain.txt][Plain]]"} {
		if !strings.Contains(got, want) {
			t.Errorf("no %s in\n%s", want, got)
		}
	}
}

func TestExportsAreInheritedOnTheWayToAnExporter(t *testing.T) {
	babelOn(t, nil)
	paths := loadOrg(t, map[string]string{"e.org": `#+PROPERTY: header-args :exports none
* Hidden
#+BEGIN_SRC sh
echo 1
#+END_SRC
* Shown
:PROPERTIES:
:header-args:sh: :exports both
:END:
#+BEGIN_SRC sh
echo 2
#+END_SRC
#+HEADER: :exports results
#+BEGIN_SRC sh
echo 3
#+END_SRC
`})
	lines, _ := readFileLines(paths["e.org"])
	out, changed := babelExportLines(paths["e.org"], lines)
	got := strings.Join(out, "\n")
	for _, want := range []string{"#+BEGIN_SRC sh :exports none\necho 1", "#+BEGIN_SRC sh :exports both\necho 2", "#+HEADER: :exports results\n#+BEGIN_SRC sh\necho 3"} {
		if !strings.Contains(got, want) {
			t.Errorf("no %q in\n%s", want, got)
		}
	}
	if !changed {
		t.Errorf("not changed")
	}
	if f := exportFile(paths["e.org"]); f == GetDb().FindByFile(paths["e.org"]) {
		t.Errorf("the export read the file as it is")
	}
	// The file itself keeps its lines.
	if strings.Contains(readOrg(t, paths["e.org"]), ":exports none\necho") {
		t.Errorf("the file was written")
	}
}

func TestCallsAndInlineCodeInheritExports(t *testing.T) {
	babelOn(t, nil)
	paths := loadOrg(t, map[string]string{"c.org": `#+PROPERTY: header-args :exports none
#+NAME: double
#+BEGIN_SRC sh :exports code
echo 2
#+END_SRC

#+CALL: double()

#+CALL: twice() :exports results

* Shown
:PROPERTIES:
:header-args:python: :exports results
:END:
Six is src_python{return 6} {{{results(=6=)}}} here.
Seven is src_sh[:exports code]{echo 7} {{{results(=7=)}}} too.
And call_double() {{{results(=2=)}}} goes.
`})
	lines, _ := readFileLines(paths["c.org"])
	out, changed := babelExportLines(paths["c.org"], lines)
	got := strings.Join(out, "\n")
	for _, want := range []string{
		"#+CALL: double() :exports code", // the called block's own :exports over the file's
		"#+CALL: twice() :exports results\n",
		"Six is {{{results(=6=)}}} here.",
		"Seven is src_sh[:exports code]{echo 7} too.",
		"And call_double() goes.", // the called block says code: no result on the page
	} {
		if !strings.Contains(got, want) {
			t.Errorf("no %q in\n%s", want, got)
		}
	}
	if !changed {
		t.Errorf("not changed")
	}
}
