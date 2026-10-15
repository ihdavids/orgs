package noweb

import (
	"fmt"
	"strings"
	"testing"
)

func bodies(m map[string]string) func(string) (string, bool) {
	return func(n string) (string, bool) {
		b, ok := m[n]
		return b, ok
	}
}

func expand(t *testing.T, text string, r Resolver) string {
	t.Helper()
	out, _, err := Expand(text, r)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestPlainAndMultiLine(t *testing.T) {
	r := Resolver{Body: bodies(map[string]string{"greet": "echo hi\n", "two": "a\nb\n"}), Prefix: true}
	if got := expand(t, "<<greet>>\necho after", r); got != "echo hi\necho after" {
		t.Errorf("%q", got)
	}
	if got := expand(t, "start\n<<two>>\nend", r); got != "start\na\nb\nend" {
		t.Errorf("%q", got)
	}
}

func TestPrefix(t *testing.T) {
	b := bodies(map[string]string{"lic": "line one\nline two"})
	if got := expand(t, "# <<lic>> !", Resolver{Body: b, Prefix: true}); got != "# line one\n# line two !" {
		t.Errorf("prefix yes: %q", got)
	}
	if got := expand(t, "# <<lic>> !", Resolver{Body: b}); got != "# line one\nline two !" {
		t.Errorf("prefix no: %q", got)
	}
	// Indentation is a prefix too.
	if got := expand(t, "    <<lic>>", Resolver{Body: b, Prefix: true}); got != "    line one\n    line two" {
		t.Errorf("indent: %q", got)
	}
	// Two references on one line: the second follows the first's last line.
	two := bodies(map[string]string{"a": "A1\nA2", "b": "B"})
	if got := expand(t, "x <<a>> <<b>>", Resolver{Body: two, Prefix: true}); got != "x A1\nx A2 B" {
		t.Errorf("two: %q", got)
	}
}

func TestNestedAndCycle(t *testing.T) {
	r := Resolver{Body: bodies(map[string]string{"outer": "begin\n  <<inner>>\nend", "inner": "x\ny"}), Prefix: true}
	if got := expand(t, "<<outer>>", r); got != "begin\n  x\n  y\nend" {
		t.Errorf("%q", got)
	}
	loop := Resolver{Body: bodies(map[string]string{"a": "<<b>>", "b": "<<a>>"})}
	if _, _, err := Expand("<<a>>", loop); err == nil || !strings.Contains(err.Error(), "deep") {
		t.Errorf("a cycle should be an error, got %v", err)
	}
}

func TestCalls(t *testing.T) {
	calls := []string{}
	r := Resolver{Prefix: true, Call: func(name, args string) (string, error) {
		calls = append(calls, name+"|"+args)
		if name == "bad" {
			return "", fmt.Errorf("boom")
		}
		return "result of " + name + "\n", nil
	}}
	got := expand(t, `v=<<now()>> w=<<sq(x=4, label="a b")>>`, r)
	if got != "v=result of now w=result of sq" {
		t.Errorf("%q", got)
	}
	if strings.Join(calls, ";") != `now|;sq|x=4, label="a b"` {
		t.Errorf("calls %v", calls)
	}
	if _, _, err := Expand("<<bad()>>", r); err == nil {
		t.Errorf("a failed call is an error")
	}
	// No Call: left as written, with a warning.
	out, warns, err := Expand("<<now()>>", Resolver{})
	if err != nil || out != "<<now()>>" || len(warns) != 1 {
		t.Errorf("%q %v %v", out, warns, err)
	}
}

func TestUnresolvedAndNotReferences(t *testing.T) {
	out, warns, err := Expand("keep <<missing>> and <<missing>>", Resolver{Body: bodies(nil)})
	if err != nil || out != "keep <<missing>> and <<missing>>" || len(warns) != 1 {
		t.Errorf("%q %v %v", out, warns, err)
	}
	cpp := `std::cout << x >> y; a << "<" >> b;`
	if got := expand(t, cpp, Resolver{Body: bodies(map[string]string{"x": "NO"})}); got != cpp {
		t.Errorf("c++ shifts read as references: %q", got)
	}
	if len(Refs(cpp)) != 0 {
		t.Errorf("Refs found %v", Refs(cpp))
	}
}

func TestRefsAndStrip(t *testing.T) {
	refs := Refs("a <<one>> b <<two(x=1)>>")
	if len(refs) != 2 || refs[0].Name != "one" || refs[0].Call || refs[1].Name != "two" || refs[1].Args != "x=1" || !refs[1].Call {
		t.Errorf("%+v", refs)
	}
	if got := Strip("pre <<one>> post\n<<two()>>"); got != "pre  post\n" {
		t.Errorf("%q", got)
	}
	if !Has("<<x>>") || Has("a << b >> c") {
		t.Errorf("Has")
	}
}
