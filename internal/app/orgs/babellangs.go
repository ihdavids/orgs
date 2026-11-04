package orgs

// More languages for babel (org manual 16.9), beside babel.go's: compiled
// ones (C, C++, Rust) built then run; Java through the single-file source
// launcher; Julia, Haskell, Clojure, Scheme and Octave interpreted; SQL
// against an :engine; LaTeX drawn as a picture. Each takes its :var values
// in its own syntax, as ob-C, ob-java and the rest do:
//
//	C, C++   :includes "<math.h>" :defines "N 10" :flags "-O2" :libs "-lm"
//	         :main no; a body without main() is put in one
//	Rust     a body without fn main() is put in one
//	Java     :classname Main; a body without a class is put in main()
//	SQL      :engine postgresql|mysql|sqlite :dbhost :dbport :dbuser
//	         :database (or :db); the answer comes back as a table
//	LaTeX    :file eq.png or eq.svg; a fragment is set with the standalone class
//
// What runs is still babel.commands' to replace (babel.commands.c: [clang]),
// and the language still has to be allowed (babel.languages).

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func init() {
	for k, r := range map[string]runner{
		"c": {cmd: []string{"cc"}, ext: ".c", bind: bindC, assemble: assembleC, defaultCollect: "output",
			build: func(in, bin string, flags, libs []string) []string {
				return append(append(append([]string{"cc"}, flags...), "-o", bin, in), libs...)
			}, prepareRun: prepareCompiled},
		"cpp": {cmd: []string{"c++"}, ext: ".cpp", bind: bindCpp, assemble: assembleCpp, defaultCollect: "output",
			build: func(in, bin string, flags, libs []string) []string {
				return append(append(append([]string{"c++", "-std=c++17"}, flags...), "-o", bin, in), libs...)
			}, prepareRun: prepareCompiled},
		"rust": {cmd: []string{"rustc"}, ext: ".rs", bind: bindRust, assemble: assembleRust, defaultCollect: "output",
			build: func(in, bin string, flags, libs []string) []string {
				return append(append([]string{"rustc", "--edition", "2021", "-o", bin}, flags...), in)
			}, prepareRun: prepareCompiled},
		"java":    {cmd: []string{"java"}, ext: ".java", bind: bindJava, assemble: assembleJava, defaultCollect: "output", prepareRun: prepareJava},
		"julia":   {cmd: []string{"julia"}, ext: ".jl", bind: bindJulia, defaultCollect: "output"},
		"haskell": {cmd: []string{"runghc"}, ext: ".hs", bind: bindHaskell, assemble: assembleHaskell, defaultCollect: "output"},
		"clojure": {cmd: []string{"clojure", "-M"}, ext: ".clj", bind: bindClojure, defaultCollect: "output"},
		"scheme":  {cmd: []string{"guile", "-s"}, ext: ".scm", bind: bindScheme, defaultCollect: "output"},
		"octave":  {cmd: []string{"octave", "--no-gui", "--quiet"}, ext: ".m", bind: bindOctave, defaultCollect: "output"},
		"sql":     {cmd: []string{"psql"}, bind: bindSql, assemble: assembleSqlite, defaultCollect: "output", prepareRun: prepareSql},
		"latex": {cmd: []string{"pdflatex"}, ext: ".tex", defaultCollect: "output", custom: runLatexPicture,
			picture: &picture{}},
	} {
		runners[k] = r
	}
	for a, k := range map[string]string{
		"C": "c", "c++": "cpp", "cxx": "cpp", "rs": "rust", "jl": "julia", "hs": "haskell",
		"clj": "clojure", "guile": "scheme", "matlab": "octave", "mysql": "sql", "postgresql": "sql",
		"psql": "sql", "postgres": "sql", "tex": "latex",
	} {
		runnerAliases[strings.ToLower(a)] = k
	}
}

// ---------------------------------------------------------------------------
// C and C++
// ---------------------------------------------------------------------------

func prepareCompiled(j *babelJob, r runner) (runner, error) {
	if f, ok := j.arg("flags"); ok {
		r.flags = shellWords(f)
	}
	if l, ok := j.arg("libs"); ok {
		r.libs = shellWords(l)
	}
	// babel.commands may name another compiler; the first word is it.
	if r.build != nil && len(r.cmd) > 0 && r.cmd[0] != "cc" && r.cmd[0] != "c++" && r.cmd[0] != "rustc" {
		compiler, base := r.cmd[0], r.build
		r.build = func(in, bin string, flags, libs []string) []string {
			out := base(in, bin, flags, libs)
			out[0] = compiler
			return out
		}
	}
	return r, nil
}

func cScalar(v babelVal) (typ, lit string) {
	t := strings.TrimSpace(strings.Trim(v.v.Value, `"`))
	if !v.text {
		if _, err := strconv.Atoi(t); err == nil {
			return "int", t
		}
		if _, err := strconv.ParseFloat(t, 64); err == nil {
			return "double", t
		}
	}
	return "const char*", quoteDouble(t)
}

// bindC is a variable as ob-C writes it: a scalar as int, double or string; a
// list as an array of strings; a table as a 2D array of strings with its
// sizes beside it (name_rows, name_cols).
func bindC(name string, v babelVal) string {
	switch {
	case v.isTbl:
		rows, cols := [][]string{}, 0
		for _, r := range v.rows {
			if r != nil {
				rows = append(rows, r)
				cols = max(cols, len(r))
			}
		}
		out := []string{}
		for _, r := range rows {
			cells := []string{}
			for i := 0; i < cols; i++ {
				c := ""
				if i < len(r) {
					c = r[i]
				}
				cells = append(cells, quoteDouble(c))
			}
			out = append(out, "{"+strings.Join(cells, ", ")+"}")
		}
		return fmt.Sprintf("const char* %s[%d][%d] = {%s};\nconst int %s_rows = %d;\nconst int %s_cols = %d;",
			name, max(len(rows), 1), max(cols, 1), strings.Join(out, ", "), name, len(rows), name, cols)
	case v.isList:
		cells := []string{}
		for _, c := range v.list {
			cells = append(cells, quoteDouble(c))
		}
		return fmt.Sprintf("const char* %s[%d] = {%s};\nconst int %s_length = %d;", name, max(len(cells), 1), strings.Join(cells, ", "), name, len(cells))
	}
	typ, lit := cScalar(v)
	return typ + " " + name + " = " + lit + ";"
}

func bindCpp(name string, v babelVal) string {
	if !v.isTbl && !v.isList {
		typ, lit := cScalar(v)
		if typ == "const char*" {
			typ = "std::string"
		}
		return typ + " " + name + " = " + lit + ";"
	}
	return bindC(name, v)
}

var cMainRe = regexp.MustCompile(`(?m)\bmain\s*\(`)

func cHead(j *babelJob, defaults []string) []string {
	head := []string{}
	includes := defaults
	if inc, ok := j.arg("includes"); ok {
		includes = append(includes, shellWords(inc)...)
	}
	seen := map[string]bool{}
	for _, i := range includes {
		if !strings.HasPrefix(i, "<") && !strings.HasPrefix(i, `"`) {
			i = "<" + i + ">"
		}
		if !seen[i] {
			seen[i] = true
			head = append(head, "#include "+i)
		}
	}
	if def, ok := j.arg("defines"); ok {
		for _, d := range strings.Split(def, ";") {
			if d = strings.TrimSpace(d); d != "" {
				head = append(head, "#define "+d)
			}
		}
	}
	return head
}

func assembleCLike(j *babelJob, code string, binds []string, defaults []string) string {
	parts := cHead(j, defaults)
	parts = append(parts, binds...)
	main, _ := j.arg("main")
	if cMainRe.MatchString(code) || main == "no" {
		return strings.Join(append(parts, code), "\n") + "\n"
	}
	body := []string{}
	for _, l := range strings.Split(code, "\n") {
		body = append(body, "  "+l)
	}
	return strings.Join(append(parts, "int main() {", strings.Join(body, "\n"), "  return 0;", "}"), "\n") + "\n"
}

func assembleC(j *babelJob, code string, binds []string) string {
	return assembleCLike(j, code, binds, []string{"<stdio.h>", "<stdlib.h>", "<string.h>"})
}

func assembleCpp(j *babelJob, code string, binds []string) string {
	src := assembleCLike(j, code, binds, []string{"<iostream>", "<string>", "<vector>", "<cstdio>"})
	if !strings.Contains(code, "using namespace") {
		// What a block written for ob-C++ assumes.
		src = strings.Replace(src, "\nint main() {", "\nusing namespace std;\nint main() {", 1)
	}
	return src
}

// ---------------------------------------------------------------------------
// Rust, Java, Haskell
// ---------------------------------------------------------------------------

func bindRust(name string, v babelVal) string {
	switch {
	case v.isTbl, v.isList:
		return "let " + name + ": Vec<Vec<&str>> = vec![" + rustRows(v) + "];"
	}
	typ, lit := cScalar(v)
	switch typ {
	case "int":
		return "let " + name + ": i64 = " + lit + ";"
	case "double":
		return "let " + name + ": f64 = " + lit + ";"
	}
	return "let " + name + ": &str = " + lit + ";"
}

func rustRows(v babelVal) string {
	rows := v.rows
	if v.isList {
		for _, c := range v.list {
			rows = append(rows, []string{c})
		}
	}
	out := []string{}
	for _, r := range rows {
		if r == nil {
			continue
		}
		cells := []string{}
		for _, c := range r {
			cells = append(cells, quoteDouble(c))
		}
		out = append(out, "vec!["+strings.Join(cells, ", ")+"]")
	}
	return strings.Join(out, ", ")
}

var rustMainRe = regexp.MustCompile(`(?m)\bfn\s+main\s*\(`)

func assembleRust(j *babelJob, code string, binds []string) string {
	if rustMainRe.MatchString(code) {
		// The bindings are lets, so they go at the top of main.
		loc := rustMainRe.FindStringIndex(code)
		brace := strings.IndexByte(code[loc[1]:], '{')
		if brace < 0 || len(binds) == 0 {
			return code
		}
		at := loc[1] + brace + 1
		return code[:at] + "\n    " + strings.Join(binds, "\n    ") + code[at:]
	}
	body := append([]string{}, binds...)
	body = append(body, strings.Split(code, "\n")...)
	for i := range body {
		body[i] = "    " + body[i]
	}
	return "#![allow(unused)]\nfn main() {\n" + strings.Join(body, "\n") + "\n}\n"
}

func javaScalar(v babelVal) string {
	typ, lit := cScalar(v)
	switch typ {
	case "int":
		return "static int %s = " + lit + ";"
	case "double":
		return "static double %s = " + lit + ";"
	}
	return "static String %s = " + lit + ";"
}

func bindJava(name string, v babelVal) string {
	if v.isTbl || v.isList {
		return "static String[][] " + name + " = {" + strings.NewReplacer("vec![", "{", "]", "}").Replace(rustRows(v)) + "};"
	}
	return fmt.Sprintf(javaScalar(v), name)
}

var javaClassRe = regexp.MustCompile(`(?m)\bclass\s+(\w+)`)

func javaClassOf(j *babelJob, code string) string {
	if c, ok := j.arg("classname"); ok && c != "" {
		return c
	}
	if m := javaClassRe.FindStringSubmatch(code); m != nil {
		return m[1]
	}
	return "Main"
}

// prepareJava names the file after the class, as the source launcher wants.
func prepareJava(j *babelJob, r runner) (runner, error) {
	r.ext = "/" + javaClassOf(j, j.Code) + ".java"
	return r, nil
}

func assembleJava(j *babelJob, code string, binds []string) string {
	class := javaClassOf(j, code)
	imports := []string{}
	if imp, ok := j.arg("imports"); ok {
		for _, w := range shellWords(imp) {
			imports = append(imports, "import "+strings.TrimSuffix(w, ";")+";")
		}
	}
	if javaClassRe.MatchString(code) {
		if len(binds) == 0 {
			return strings.Join(append(imports, code), "\n")
		}
		loc := javaClassRe.FindStringIndex(code)
		brace := strings.IndexByte(code[loc[1]:], '{')
		at := loc[1] + brace + 1
		return strings.Join(imports, "\n") + "\n" + code[:at] + "\n  " + strings.Join(binds, "\n  ") + code[at:]
	}
	body := []string{}
	for _, l := range strings.Split(code, "\n") {
		body = append(body, "    "+l)
	}
	return strings.Join(imports, "\n") + "\npublic class " + class + " {\n  " + strings.Join(binds, "\n  ") +
		"\n  public static void main(String[] args) throws Exception {\n" + strings.Join(body, "\n") + "\n  }\n}\n"
}

func bindHaskell(name string, v babelVal) string {
	return name + " = " + literalOf(v, quoteDouble, "[", "]", ", ", "[]")
}

var haskellMainRe = regexp.MustCompile(`(?m)^main\s*(::|=)`)

func assembleHaskell(j *babelJob, code string, binds []string) string {
	if haskellMainRe.MatchString(code) {
		return strings.Join(append(binds, code), "\n") + "\n"
	}
	body := []string{}
	for _, l := range strings.Split(code, "\n") {
		body = append(body, "  "+l)
	}
	return strings.Join(append(binds, "main :: IO ()", "main = do", strings.Join(body, "\n")), "\n") + "\n"
}

// ---------------------------------------------------------------------------
// Julia, Clojure, Scheme, Octave
// ---------------------------------------------------------------------------

func bindJulia(name string, v babelVal) string {
	return name + " = " + literalOf(v, quoteDouble, "[", "]", ", ", "nothing")
}

func bindClojure(name string, v babelVal) string {
	return "(def " + name + " " + literalOf(v, quoteDouble, "[", "]", " ", "nil") + ")"
}

func bindScheme(name string, v babelVal) string {
	return "(define " + name + " " + strings.Replace(literalOf(v, quoteDouble, "(list ", ")", " ", "'()"), "(list )", "'()", -1) + ")"
}

func bindOctave(name string, v babelVal) string {
	if v.isTbl || v.isList {
		return name + " = " + literalOf(v, quoteSingleOctave, "{", "}", ", ", "{}") + ";"
	}
	return name + " = " + literalOf(v, quoteSingleOctave, "", "", "", "[]") + ";"
}

func quoteSingleOctave(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

// ---------------------------------------------------------------------------
// SQL against an engine
// ---------------------------------------------------------------------------

// bindSql is sqlite's: $name in the query becomes the value.
func bindSql(name string, v babelVal) string { return bindSqlite(name, v) }

// prepareSql picks the client for :engine and gives it the connection
// arguments, as ob-sql does. sqlite goes to the sqlite runner.
func prepareSql(j *babelJob, r runner) (runner, error) {
	engine, _ := j.arg("engine")
	arg := func(k string) string { v, _ := j.arg(k); return v }
	db := arg("database")
	if db == "" {
		db = arg("db")
	}
	switch strings.ToLower(engine) {
	case "postgresql", "postgres", "psql", "":
		r.cmd = []string{"psql", "--csv", "-X", "-q", "-v", "ON_ERROR_STOP=1"}
		if h := arg("dbhost"); h != "" {
			r.cmd = append(r.cmd, "-h", h)
		}
		if p := arg("dbport"); p != "" {
			r.cmd = append(r.cmd, "-p", p)
		}
		if u := arg("dbuser"); u != "" {
			r.cmd = append(r.cmd, "-U", u)
		}
		if db != "" {
			r.cmd = append(r.cmd, "-d", db)
		}
		r.csvOut = true
	case "mysql", "mariadb":
		r.cmd = []string{"mysql", "--batch"}
		if h := arg("dbhost"); h != "" {
			r.cmd = append(r.cmd, "-h", h)
		}
		if p := arg("dbport"); p != "" {
			r.cmd = append(r.cmd, "-P", p)
		}
		if u := arg("dbuser"); u != "" {
			r.cmd = append(r.cmd, "-u", u)
		}
		if pw := arg("dbpassword"); pw != "" {
			r.cmd = append(r.cmd, "-p"+pw)
		}
		if db != "" {
			r.cmd = append(r.cmd, db)
		}
	case "sqlite", "sqlite3":
		s := runners["sqlite"]
		if db != "" {
			s.cmd = append(append([]string{}, s.cmd...), db)
		}
		return s, nil
	default:
		return r, fmt.Errorf("orgs runs sql with :engine postgresql, mysql or sqlite, not %q", engine)
	}
	if cmd, ok := Conf().Server.Babel.Commands["sql"]; ok && len(cmd) > 0 {
		r.cmd = append(append([]string{}, cmd...), r.cmd[1:]...)
	}
	return r, nil
}

// ---------------------------------------------------------------------------
// LaTeX as a picture
// ---------------------------------------------------------------------------

// runLatexPicture sets the block with pdflatex (in the standalone class when
// it is a fragment) and turns the pdf into :file's format: pdf as it is,
// png and svg through pdftocairo.
func runLatexPicture(r runner, program, dir string, timeout time.Duration, out string) (babelOutcome, error) {
	res := babelOutcome{}
	tmp, err := os.MkdirTemp("", "orgslatex")
	if err != nil {
		return res, err
	}
	defer os.RemoveAll(tmp)
	src := program
	if !strings.Contains(src, `\documentclass`) {
		src = "\\documentclass[border=2pt]{standalone}\n\\usepackage{amsmath,amssymb}\n\\begin{document}\n" + src + "\n\\end{document}\n"
	}
	if err := os.WriteFile(filepath.Join(tmp, "block.tex"), []byte(src), 0o600); err != nil {
		return res, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	start := time.Now()
	run := func(name string, args ...string) error {
		c := exec.CommandContext(ctx, name, args...)
		c.Dir = tmp
		var b bytes.Buffer
		c.Stdout, c.Stderr = &b, &b
		if err := c.Run(); err != nil {
			if errors.Is(err, exec.ErrNotFound) {
				return fmt.Errorf("%s is not installed on the server (or not on its PATH)", name)
			}
			res.code = 1
			res.stderr = name + ":\n" + latexTail(b.String(), 2000)
			return nil
		}
		return nil
	}
	tex := "pdflatex"
	if len(r.cmd) > 0 && r.cmd[0] != "" {
		tex = r.cmd[0]
	}
	if err := run(tex, "-interaction=nonstopmode", "-halt-on-error", "block.tex"); err != nil || res.code != 0 {
		res.took = time.Since(start)
		return res, err
	}
	pdf := filepath.Join(tmp, "block.pdf")
	os.MkdirAll(filepath.Dir(out), 0o755)
	switch strings.ToLower(filepath.Ext(out)) {
	case ".pdf":
		b, err := os.ReadFile(pdf)
		if err != nil {
			return res, err
		}
		err = os.WriteFile(out, b, 0o644)
		res.took = time.Since(start)
		return res, err
	case ".svg":
		err = run("pdftocairo", "-svg", pdf, out)
	default:
		err = run("pdftocairo", "-png", "-singlefile", "-r", "150", pdf, strings.TrimSuffix(out, filepath.Ext(out)))
	}
	res.took = time.Since(start)
	return res, err
}

func latexTail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}
