package orgs

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ihdavids/orgs/internal/common"
)

// runOne runs the only block of a one-block file and answers what was
// written under it.
func runOne(t *testing.T, lang, header, code string) string {
	t.Helper()
	paths := loadOrg(t, map[string]string{"b.org": "#+NAME: blk\n#+BEGIN_SRC " + lang + " " + header + "\n" + code + "\n#+END_SRC\n"})
	// A failed run is written too (its error is the result), so the run is
	// not asked to succeed here; the caller looks at what was written.
	ExecBabel(&common.BabelExec{Filename: paths["b.org"], Line: 1})
	got := readOrg(t, paths["b.org"])
	i := strings.Index(got, "#+RESULTS:")
	if i < 0 {
		t.Fatalf("%s: no result in\n%s", lang, got)
	}
	return got[i:]
}

func TestCompiledLanguages(t *testing.T) {
	babelOn(t, nil)
	cases := []struct{ lang, header, code, want, needs string }{
		{"C", ":var n=6 :var who=\"world\" :includes <math.h> :libs -lm", `printf("%d %s %.0f\n", n * 7, who, sqrt(16));`, ": 42 world 4", "cc"},
		{"C++", ":var n=3", `cout << n * 2 << endl;`, ": 6", "c++"},
		{"rust", ":var n=5", `println!("{}", n + 1);`, ": 6", "rustc"},
		{"julia", ":var n=3", `println(n * 2)`, ": 6", "julia"},
		{"scheme", ":var n=4", `(display (* n n))`, ": 16", "guile"},
	}
	for _, c := range cases {
		if _, err := exec.LookPath(c.needs); err != nil {
			t.Logf("%s: %s is not installed", c.lang, c.needs)
			continue
		}
		if got := runOne(t, c.lang, c.header, c.code); !strings.Contains(got, c.want) {
			t.Errorf("%s: want %q in\n%s", c.lang, c.want, got)
		}
	}
}

func TestACompileErrorIsTheResult(t *testing.T) {
	need(t, "cc")
	babelOn(t, nil)
	if got := runOne(t, "C", "", "this is not C"); !strings.Contains(got, "did not compile") {
		t.Errorf("compile error not shown:\n%s", got)
	}
}

func TestJavaRuns(t *testing.T) {
	if err := exec.Command("java", "-version").Run(); err != nil {
		t.Skip("no java runtime")
	}
	babelOn(t, nil)
	if got := runOne(t, "java", ":var n=20", `System.out.println(n + 1);`); !strings.Contains(got, ": 21") {
		t.Errorf("java:\n%s", got)
	}
}

func TestLatexDrawsAPicture(t *testing.T) {
	need(t, "pdflatex")
	need(t, "pdftocairo")
	babelOn(t, nil)
	for _, ext := range []string{"png", "svg"} {
		got := runOne(t, "latex", ":file eq."+ext, `$e^{i\pi} + 1 = 0$`)
		if !strings.Contains(got, "[[file:eq."+ext+"]]") {
			t.Errorf("%s link:\n%s", ext, got)
		}
	}
}

func TestSqlEngines(t *testing.T) {
	babelOn(t, nil)
	j := babelJob{Lang: "sql", Args: argsOf(":engine postgresql :dbhost db.local :dbport 5433 :dbuser sam :database shop")}
	r, err := prepareSql(&j, runners["sql"])
	if err != nil || strings.Join(r.cmd, " ") != "psql --csv -X -q -v ON_ERROR_STOP=1 -h db.local -p 5433 -U sam -d shop" || !r.csvOut {
		t.Errorf("psql: %v %v", r.cmd, err)
	}
	j.Args = argsOf(":engine mysql :dbuser sam :database shop")
	if r, _ := prepareSql(&j, runners["sql"]); strings.Join(r.cmd, " ") != "mysql --batch -u sam shop" {
		t.Errorf("mysql: %v", r.cmd)
	}
	j.Args = argsOf(":engine oracle")
	if _, err := prepareSql(&j, runners["sql"]); err == nil {
		t.Errorf("an unknown engine was taken")
	}
	need(t, "sqlite3")
	db := filepath.Join(t.TempDir(), "t.db")
	exec.Command("sqlite3", db, "create table t(a); insert into t values (1),(2);").Run()
	if got := runOne(t, "sql", ":engine sqlite :database "+db, "select sum(a) as total from t;"); !strings.Contains(got, "3") {
		t.Errorf("sqlite engine:\n%s", got)
	}
	os.Remove(db)
}

func argsOf(h string) []common.CodeArg {
	_, _, args, _ := splitArgs(splitParametersLike(" " + h))
	return args
}

// Assembled for languages not installed here: what would be run.
func TestAssembledPrograms(t *testing.T) {
	j := &babelJob{Lang: "haskell"}
	if got := assembleHaskell(j, `print (n * 2)`, []string{bindHaskell("n", scalarVal(common.CodeVar{Name: "n", Value: "21"}))}); got != "n = 21\nmain :: IO ()\nmain = do\n  print (n * 2)\n" {
		t.Errorf("haskell:\n%s", got)
	}
	if got := bindClojure("xs", listVal("xs", []string{"1", "a"})); got != `(def xs [1 "a"])` {
		t.Errorf("clojure: %s", got)
	}
	if got := bindOctave("m", tableVal("m", [][]string{{"1", "x"}})); got != "m = {{1, 'x'}};" {
		t.Errorf("octave: %s", got)
	}
	jj := &babelJob{Lang: "java", Args: argsOf(":imports java.util.List")}
	src := assembleJava(jj, `System.out.println(n);`, []string{bindJava("n", scalarVal(common.CodeVar{Name: "n", Value: "3"}))})
	if !strings.Contains(src, "import java.util.List;") || !strings.Contains(src, "public class Main {\n  static int n = 3;") || !strings.Contains(src, "public static void main(String[] args)") {
		t.Errorf("java:\n%s", src)
	}
	c := assembleC(&babelJob{Lang: "C", Args: argsOf(":defines \"N 10\" :main no")}, "int main(){return N;}", nil)
	if !strings.Contains(c, "#define N 10") || strings.Count(c, "main(") != 1 {
		t.Errorf("C :main no:\n%s", c)
	}
	tbl := bindC("t", tableVal("t", [][]string{{"a", "b"}, nil, {"c", "d"}}))
	if !strings.Contains(tbl, `const char* t[2][2] = {{"a", "b"}, {"c", "d"}};`) || !strings.Contains(tbl, "const int t_rows = 2;") {
		t.Errorf("C table: %s", tbl)
	}
}
