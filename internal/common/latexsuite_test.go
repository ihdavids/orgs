package common

// The same cases are in worg/src/latexsuite.test.ts (and the vscode copy's
// test): change them together.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestLatexBuiltinParses(t *testing.T) {
	s, probs := ParseLatexSnippets(LatexBuiltinOrg, "")
	if len(probs) > 0 {
		t.Fatalf("problems: %v", probs)
	}
	if len(s) < 150 {
		t.Fatalf("only %d built-in snippets", len(s))
	}
	for _, x := range s {
		if x.Trigger == "lr|" && x.Replacement != `\left| $1 \right|$0` {
			t.Errorf("lr| = %q", x.Replacement)
		}
		if x.Trigger == "dm" && x.Replacement != "\\[\n$0\n\\]" {
			t.Errorf("dm = %q", x.Replacement)
		}
		if x.Trigger == "@a" && x.Group != "Greek (cdlatex's backquote works too: `a)" {
			t.Errorf("@a group = %q", x.Group)
		}
	}
}

func TestLatexDecodeCell(t *testing.T) {
	for in, want := range map[string]string{
		` \vert x\vert `:   "| x|",
		`\vert{}abc`:       "|abc",
		`a\n$0\n\end{x}`:   "a\n$0\n\\end{x}",
		`\nu \neq \\ next`: `\nu \neq \\ next`,
		`=\lvert x=`:       `\lvert x`,
		`===`:              `===`,
		`~~`:               `~~`,
		`~x~`:              `x`,
	} {
		if got := LatexDecodeCell(in); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
}

type latexExpandCase struct {
	before, mode string
	auto         bool
	body         string // "" = nothing expands
	remove       int
}

var latexExpandCases = []latexExpandCase{
	{"see mk", "text", true, `\( $0 \)`, 2},
	{"remk", "text", true, "", 0},
	{"mk", "inline", true, "", 0},
	{"x + @a", "inline", true, `\alpha`, 2},
	{"x + `a", "inline", true, `\alpha`, 2},
	{"x1", "inline", true, `x_1`, 2},
	{`\alpha1`, "inline", true, `a_1`, 2},
	{"x_12", "display", true, `x_{12}`, 4},
	{"a ->", "inline", true, `\to`, 2},
	{"a <->", "inline", true, `\leftrightarrow`, 3},
	{`\sq`, "inline", true, "", 0},
	{"x sr", "inline", true, `^{2}`, 2},
	{"1 + //", "inline", true, `\frac{$1}{$2}$0`, 2},
	{"2sin", "inline", true, `\sin`, 3},
	{"asin", "inline", true, "", 0},
	{"xhat", "inline", true, `\hat{x}`, 4},
	{"x int", "inline", false, `\int $1 \, d${2:x} $0`, 3},
	{"x int", "inline", true, "", 0},
	{"pmat", "display", true, "\\begin{pmatrix}\n$0\n\\end{pmatrix}", 4},
	{"pmat", "inline", true, `\begin{pmatrix} $0 \end{pmatrix}`, 4},
	{"3rt", "inline", true, `\sqrt[3]{$1}$0`, 3},
	{"lr|", "inline", true, `\left| $1 \right|$0`, 3},
	{"a ===", "inline", true, `\equiv`, 3},
	{"} =", "inline", true, "", 0},
	{"a ~~", "inline", true, `\approx`, 2},
}

func TestLatexExpand(t *testing.T) {
	snips := LatexBuiltinSnippets()
	for _, c := range latexExpandCases {
		e := LatexExpand(snips, c.before, c.mode, c.auto)
		if c.body == "" {
			if e != nil {
				t.Errorf("%q %s: expanded to %q", c.before, c.mode, e.Body)
			}
			continue
		}
		if e == nil {
			t.Errorf("%q %s: nothing", c.before, c.mode)
			continue
		}
		if e.Body != c.body || e.Remove != c.remove {
			t.Errorf("%q %s: got %q -%d, want %q -%d", c.before, c.mode, e.Body, e.Remove, c.body, c.remove)
		}
	}
}

func TestLatexVisual(t *testing.T) {
	snips := LatexBuiltinSnippets()
	if e := LatexVisual(snips, "U", "inline", "a+b"); e == nil || e.Body != `\underbrace{a+b}_{$1}$0` {
		t.Errorf("U: %+v", e)
	}
	if e := LatexVisual(snips, "$", "text", "x$"); e == nil || e.Body != `\( x\$ \)` {
		t.Errorf("$: %+v", e)
	}
	if e := LatexVisual(snips, "U", "text", "a"); e != nil {
		t.Errorf("U in text: %+v", e)
	}
}

func TestLatexMerge(t *testing.T) {
	user, _ := ParseLatexSnippets("| Trigger | Replacement | Options |\n|-\n| sr | ^{two} | mA |\n| xx | | mA |\n| foo | \\foo | m |\n", "mine.org")
	m := MergeLatexSnippets(user, LatexBuiltinSnippets())
	if e := LatexExpand(m, "x sr", "inline", true); e == nil || e.Body != "^{two}" {
		t.Errorf("sr: %+v", e)
	}
	if e := LatexExpand(m, "a xx", "inline", true); e != nil {
		t.Errorf("xx still expands: %+v", e)
	}
	if e := LatexExpand(m, "foo", "inline", false); e == nil || e.Snippet.File != "mine.org" || e.Snippet.Line != 5 {
		t.Errorf("foo: %+v", e)
	}
}

func TestLatexAutoFraction(t *testing.T) {
	for _, c := range []struct {
		before, body string
		remove       int
	}{
		{"a/", `\frac{a}{$1}$0`, 2},
		{"x + (a+b)/", `\frac{a+b}{$1}$0`, 6},
		{`2\alpha^2/`, `\frac{2\alpha^2}{$1}$0`, 10},
		{`\frac{1}{2}/`, `\frac{\frac{1}{2}}{$1}$0`, 12},
		{"x /", "", 0},
		{"x//", "", 0},
	} {
		e := LatexAutoFraction(c.before)
		if c.body == "" {
			if e != nil {
				t.Errorf("%q: %q", c.before, e.Body)
			}
			continue
		}
		if e == nil || e.Body != c.body || e.Remove != c.remove {
			t.Errorf("%q: got %+v want %q -%d", c.before, e, c.body, c.remove)
		}
	}
}

func TestLatexPlace(t *testing.T) {
	for _, c := range []struct {
		body  string
		want  LatexPlaced
	}{
		{`\frac{$1}{$2}$0`, LatexPlaced{`\frac{}{}`, []LatexStop{{1, 6, 6}, {2, 8, 8}, {0, 9, 9}}}},
		{`\lim_{${1:n} \to ${2:\infty}} $0`, LatexPlaced{`\lim_{n \to \infty} `, []LatexStop{{1, 6, 7}, {2, 12, 18}, {0, 20, 20}}}},
		{"\\begin{$1}\n$0\n\\end{$1}", LatexPlaced{"\\begin{}\n\n\\end{}", []LatexStop{{1, 7, 7}, {1, 15, 15}, {0, 9, 9}}}},
		{`${1:x^{2}} + $1`, LatexPlaced{`x^{2} + x^{2}`, []LatexStop{{1, 0, 5}, {1, 8, 13}, {0, 13, 13}}}},
		{`^{2}`, LatexPlaced{`^{2}`, []LatexStop{{0, 4, 4}}}},
		{`x\$ $1`, LatexPlaced{`x$ `, []LatexStop{{1, 3, 3}, {0, 3, 3}}}},
	} {
		if got := LatexPlace(c.body); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%q: got %+v want %+v", c.body, got, c.want)
		}
	}
}

func TestLatexModeAt(t *testing.T) {
	for _, c := range []struct {
		text string
		at   int // -1: the end
		mode string
	}{
		{`a \( x`, -1, "inline"},
		{`a \(x\) b`, 8, "text"},
		{`a \(x\) b`, 4, "inline"},
		{`a \(x\) b`, 5, "inline"},
		{`\[ \text{if } x`, 11, "text"},
		{`\[ \text{if } x`, -1, "display"},
		{`$x$ and $y`, -1, "inline"},
		{`costs $5 and`, -1, "text"},
		{"#+begin_src\n\\(x\\)\n#+end_src", 14, "text"},
		{"\\begin{align}\na &= b", -1, "display"},
		{"\\( a\n\nb", -1, "text"},
		{`$$ x`, -1, "display"},
	} {
		at := c.at
		if at < 0 {
			at = len(c.text)
		}
		if got := LatexModeAt(c.text, at); got.Mode != c.mode {
			t.Errorf("%q at %d: %s, want %s", c.text, at, got.Mode, c.mode)
		}
	}
	f := LatexModeAt("so \\(a+b\\) ok", 6).Fragment
	if f == nil || f.Start != 3 || f.From != 5 || f.To != 8 || f.End != 10 || !f.Closed {
		t.Errorf("fragment: %+v", f)
	}
}

func TestLatexCheck(t *testing.T) {
	text := strings.Join([]string{
		`* Maths`,
		`fine \( \frac{a}{b} \) and $x$`,
		`\( \frac{a}{b \)`,
		`\( \left( x \)`,
		`\[ \begin{foo} x \end{foo} \]`,
		`costs $ 5 and $ more`,
		`\( x`,
		``,
		`#+begin_src`,
		`\( {`,
		`#+end_src`,
		`\begin{align} \begin{cases} x \end{aligned} \end{align}`,
	}, "\n")
	got := []string{}
	for _, p := range LatexCheck(text) {
		got = append(got, strings.Join([]string{p.Rule, p.Severity, strconv.Itoa(p.Line), strconv.Itoa(p.Col)}, " "))
	}
	want := []string{
		"latex-brace error 3 12",
		"latex-leftright error 4 4",
		"latex-env warning 5 4",
		"latex-dollar warning 6 7",
		"latex-unclosed error 7 1",
		"latex-env error 12 31",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// TestLatexBuiltinCopies keeps the clients' offline copies of the built-in
// set (worg/src/latexsnippets.json, vscode-org-everywhere/media/latex-snippets.json)
// the same as latexsnippets.org. LATEX_SNIPPETS_WRITE=1 rewrites them.
func TestLatexBuiltinCopies(t *testing.T) {
	want, _ := json.MarshalIndent(LatexBuiltinSnippets(), "", " ")
	want = append(want, '\n')
	for _, p := range []string{"../../../worg/src/latexsnippets.json", "../../../vscode-org-everywhere/media/latex-snippets.json"} {
		if os.Getenv("LATEX_SNIPPETS_WRITE") != "" {
			if _, err := os.Stat(filepath.Dir(p)); err == nil {
				os.WriteFile(p, want, 0o644)
			}
			continue
		}
		got, err := os.ReadFile(p)
		if err != nil {
			continue // not checked out beside this repo
		}
		if string(got) != string(want) {
			t.Errorf("%s differs from latexsnippets.org: LATEX_SNIPPETS_WRITE=1 go test -run TestLatexBuiltinCopies ./internal/common/", p)
		}
	}
}
