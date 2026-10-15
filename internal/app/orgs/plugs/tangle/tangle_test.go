package tangle

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ihdavids/go-org/org"
)

func tangleText(t *testing.T, text string) map[string]string {
	t.Helper()
	path := filepath.Join("/notes", "build.org")
	doc := org.New().Silent().Parse(strings.NewReader(text), path)
	if doc.Error != nil {
		t.Fatal(doc.Error)
	}
	res, err := assembleTangle(doc, path)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, f := range res.Files {
		rel, _ := filepath.Rel("/notes", f.Filename)
		out[rel] = f.Content
	}
	return out
}

func TestTangleYesIsNamedAfterTheFile(t *testing.T) {
	got := tangleText(t, `* Code
#+BEGIN_SRC python :tangle yes
print("hi")
#+END_SRC
#+BEGIN_SRC emacs-lisp :tangle yes
(message "hi")
#+END_SRC
#+BEGIN_SRC fennel :tangle yes
(print "hi")
#+END_SRC
#+BEGIN_SRC sh :tangle no
echo never
#+END_SRC
`)
	for name, want := range map[string]string{"build.py": "print(\"hi\")\n", "build.el": "(message \"hi\")\n", "build.fennel": "(print \"hi\")\n"} {
		if got[name] != want {
			t.Errorf("%s = %q, want %q (all: %v)", name, got[name], want, keys(got))
		}
	}
	if _, ok := got["yes"]; ok || len(got) != 3 {
		t.Errorf("files: %v", keys(got))
	}
}

func TestFileHeaderArgs(t *testing.T) {
	got := tangleText(t, `#+PROPERTY: header-args :tangle all.txt :mkdirp yes
#+PROPERTY: header-args:python :tangle "py out/main.py"
* Code
#+BEGIN_SRC sh
echo one
#+END_SRC
#+BEGIN_SRC python
print(2)
#+END_SRC
#+BEGIN_SRC sh :tangle no
echo skipped
#+END_SRC
`)
	if got["all.txt"] != "echo one\n" {
		t.Errorf("plain header-args: %v", got)
	}
	// A language's own line wins over the plain one, and a quoted value keeps its space.
	if got["py out/main.py"] != "print(2)\n" {
		t.Errorf("header-args:python: %v", keys(got))
	}
}

func TestHeadingHeaderArgsAndPrecedence(t *testing.T) {
	got := tangleText(t, `#+PROPERTY: header-args :tangle file.txt
* Scripts
:PROPERTIES:
:header-args: :tangle scripts.sh
:header-args:python: :tangle scripts.py
:END:
#+BEGIN_SRC bash
echo bash
#+END_SRC
#+BEGIN_SRC python
print("py")
#+END_SRC
** Deeper
   :PROPERTIES:
   :header-args:sh: :tangle deep.sh
   :END:
#+BEGIN_SRC bash
echo deep
#+END_SRC
#+BEGIN_SRC bash :tangle own.sh
echo own
#+END_SRC
* Elsewhere
#+BEGIN_SRC sh
echo file
#+END_SRC
`)
	want := map[string]string{
		"scripts.sh": "echo bash\n",   // the heading over the file
		"scripts.py": "print(\"py\")\n", // the heading's language line over its plain one
		"deep.sh":    "echo deep\n",   // the nearer heading, header-args:sh reaching bash
		"own.sh":     "echo own\n",    // the block's own over everything
		"file.txt":   "echo file\n",   // outside the heading, the file's
	}
	for name, w := range want {
		if got[name] != w {
			t.Errorf("%s = %q, want %q (all: %v)", name, got[name], w, keys(got))
		}
	}
	if len(got) != len(want) {
		t.Errorf("files: %v", keys(got))
	}
}

func TestHeaderArgsParsing(t *testing.T) {
	m := map[string]string{}
	headerArgs(`:tangle "a b.sh" :mkdirp yes :shebang #!/usr/bin/env bash`, m)
	if m[":tangle"] != "a b.sh" || m[":mkdirp"] != "yes" || m[":shebang"] != "#!/usr/bin/env bash" {
		t.Errorf("%v", m)
	}
}

func keys(m map[string]string) []string {
	out := []string{}
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestNamedBlocksTangleAndTextIsOrgs(t *testing.T) {
	got := tangleText(t, `* Setup
Install the tools.
#+NAME: install
#+BEGIN_SRC sh :tangle setup.sh :comments both
  ,* not a heading
  echo hi
    indented
#+END_SRC
#+BEGIN_SRC sh :tangle setup.sh :comments link
echo two
#+END_SRC
#+BEGIN_SRC css :tangle style.css :comments link
a { color: red }
#+END_SRC
`)
	want := "# Install the tools.\n# [[file:build.org::*Setup][install]]\n* not a heading\necho hi\n  indented\n# install ends here\n\n" +
		"# [[file:build.org::*Setup][Setup:2]]\necho two\n# Setup:2 ends here\n"
	if got["setup.sh"] != want {
		t.Errorf("setup.sh\n got %q\nwant %q", got["setup.sh"], want)
	}
	if got["style.css"] != "/* [[file:build.org::*Setup][Setup:3]] */\na { color: red }\n/* Setup:3 ends here */\n" {
		t.Errorf("style.css %q", got["style.css"])
	}
	secs := ParseMarkers(got["setup.sh"] + got["style.css"])
	if len(secs) != 3 || secs[0].Label != "install" || secs[0].Heading != "Setup" || secs[0].File != "build.org" ||
		strings.Join(secs[0].Body, "\n") != "* not a heading\necho hi\n  indented" || secs[2].Label != "Setup:3" {
		t.Errorf("markers: %+v", secs)
	}
}

func TestEscapeLine(t *testing.T) {
	for in, want := range map[string]string{"* x": ",* x", "  #+BEGIN": "  ,#+BEGIN", ",* x": ",,* x", "echo *": "echo *"} {
		if got := EscapeLine(in); got != want {
			t.Errorf("EscapeLine(%q) = %q want %q", in, got, want)
		}
		if got := unescapeLine(EscapeLine(in)); got != in {
			t.Errorf("round trip %q -> %q", in, got)
		}
	}
}

func TestNowebModes(t *testing.T) {
	got := tangleText(t, `* Code
#+BEGIN_SRC sh :noweb-ref greet
echo hello
#+END_SRC
#+BEGIN_SRC sh :tangle a.sh :noweb tangle
<<greet>>
#+END_SRC
#+BEGIN_SRC sh :tangle b.sh :noweb strip-tangle
<<greet>>
echo after
#+END_SRC
`)
	if got["a.sh"] != "echo hello\n" || got["b.sh"] != "\necho after\n" {
		t.Errorf("a %q b %q", got["a.sh"], got["b.sh"])
	}
}

func TestNowebPrefixAndNamedBodies(t *testing.T) {
	got := tangleText(t, `* Code
#+NAME: license
#+BEGIN_SRC text
Copyright me
All rights
#+END_SRC
#+BEGIN_SRC python :tangle a.py :noweb yes
# <<license>>
def f():
    <<body>>
#+END_SRC
#+BEGIN_SRC python :noweb-ref body
x = 1
return x
#+END_SRC
#+BEGIN_SRC python :tangle b.py :noweb yes :noweb-prefix no
# <<license>>
#+END_SRC
`)
	if got["a.py"] != "# Copyright me\n# All rights\ndef f():\n    x = 1\n    return x\n" {
		t.Errorf("a.py %q", got["a.py"])
	}
	if got["b.py"] != "# Copyright me\nAll rights\n" {
		t.Errorf("b.py %q", got["b.py"])
	}
}

func TestNowebCallsGoThroughEvaluate(t *testing.T) {
	old := Evaluate
	t.Cleanup(func() { Evaluate = old })
	src := `* Code
#+BEGIN_SRC sh :tangle v.sh :noweb yes
VERSION=<<version()>>
SQ=<<square(x=4)>>
#+END_SRC
`
	Evaluate = nil
	path := filepath.Join("/notes", "build.org")
	doc := org.New().Silent().Parse(strings.NewReader(src), path)
	res, err := assembleTangle(doc, path)
	if err != nil || len(res.Files) != 1 || !strings.Contains(res.Files[0].Content, "<<version()>>") || len(res.Warnings) == 0 {
		t.Fatalf("without Evaluate: %+v %v", res, err)
	}
	seen := []string{}
	Evaluate = func(orgPath, name, args string) (string, error) {
		seen = append(seen, filepath.Base(orgPath)+":"+name+"("+args+")")
		if name == "version" {
			return "1.2.3\n", nil
		}
		return "16", nil
	}
	got := tangleText(t, src)
	if got["v.sh"] != "VERSION=1.2.3\nSQ=16\n" {
		t.Errorf("v.sh %q", got["v.sh"])
	}
	if strings.Join(seen, ",") != "build.org:version(),build.org:square(x=4)" {
		t.Errorf("calls %v", seen)
	}
}
