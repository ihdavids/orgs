// Package noweb expands org's noweb references (org-babel-expand-noweb-
// references): `<<name>>` is the code of the block called name, or of every
// block whose :noweb-ref is name; `<<name(args)>>` is the result of
// evaluating name with those arguments, as a #+CALL: would.
//
// Shared by the tangler (plugs/tangle) and by running a block (babel), so a
// reference reads the same whether the code goes to a file or to a program.
// It knows nothing of org files or of running code: a Resolver supplies the
// bodies and the evaluation.
package noweb

import (
	"fmt"
	"regexp"
	"strings"
)

// The depth past which an expansion is taken to be a block that includes
// itself, and refused.
const MaxDepth = 20

// A reference: org's default wrap, `<<` and `>>`, around a name that neither
// starts nor ends with whitespace - so `cout << x >> y` is not one - with an
// optional (arguments) after it.
var refRe = regexp.MustCompile(`<<(\S(?:[^<>\n]*?\S)?)>>`)

// Ref is one reference in a text.
type Ref struct {
	Name string
	// The text inside the parentheses, when there are any.
	Args string
	// Written name(...): the block's result, not its code.
	Call bool
	// Where it is in the text, as a byte offset, and the whole of it as
	// written.
	At   int
	Text string
}

func parse(inner string) (name, args string, call bool) {
	if strings.HasSuffix(inner, ")") {
		if i := strings.Index(inner, "("); i > 0 {
			return strings.TrimSpace(inner[:i]), strings.TrimSpace(inner[i+1 : len(inner)-1]), true
		}
	}
	return inner, "", false
}

// Refs lists the references a text holds, in order.
func Refs(text string) []Ref {
	out := []Ref{}
	for _, m := range refRe.FindAllStringSubmatchIndex(text, -1) {
		name, args, call := parse(text[m[2]:m[3]])
		out = append(out, Ref{Name: name, Args: args, Call: call, At: m[0], Text: text[m[0]:m[1]]})
	}
	return out
}

// Has says whether a text holds any reference.
func Has(text string) bool { return refRe.MatchString(text) }

// Strip takes every reference out, leaving the rest of its line (org's
// :noweb strip-tangle and strip-export).
func Strip(text string) string { return refRe.ReplaceAllString(text, "") }

// Resolver says what a reference stands for.
type Resolver struct {
	// The code of the block called name, or of every block whose :noweb-ref
	// is name joined with their :noweb-sep. ok is false when there is none.
	Body func(name string) (body string, ok bool)
	// The result of evaluating name with args (the text between the
	// parentheses, as a #+CALL: writes it). Nil leaves <<name(args)>> as
	// written, with a warning.
	Call func(name, args string) (string, error)
	// org's :noweb-prefix: the text before a reference on its line is put in
	// front of every line of the expansion, not only the first - so
	// `# <<license>>` comments out the whole licence. Org's default is yes.
	Prefix bool
}

type expander struct {
	r        Resolver
	warnings []string
	seen     map[string]bool
}

func (e *expander) warn(format string, args ...interface{}) {
	w := fmt.Sprintf(format, args...)
	if !e.seen[w] {
		e.seen[w] = true
		e.warnings = append(e.warnings, w)
	}
}

// Expand replaces every reference in text with what it stands for. A
// reference nothing answers to is left as written and named in warnings, as
// org leaves it; a reference that expands into itself is an error.
func Expand(text string, r Resolver) (string, []string, error) {
	e := &expander{r: r, seen: map[string]bool{}}
	out, err := e.text(text, 0)
	return out, e.warnings, err
}

func (e *expander) text(t string, depth int) (string, error) {
	if !refRe.MatchString(t) {
		return t, nil
	}
	lines := strings.Split(t, "\n")
	for i, l := range lines {
		out, err := e.line("", l, depth)
		if err != nil {
			return "", err
		}
		lines[i] = out
	}
	return strings.Join(lines, "\n"), nil
}

// line expands the references in s, which follows pre on its line - pre is
// what has already been written there, and is the prefix a reference in s
// repeats.
func (e *expander) line(pre, s string, depth int) (string, error) {
	m := refRe.FindStringSubmatchIndex(s)
	if m == nil {
		return s, nil
	}
	before, literal, after := s[:m[0]], s[m[0]:m[1]], s[m[1]:]
	linePrefix := pre + before
	exp, ok, err := e.ref(s[m[2]:m[3]], depth)
	if err != nil {
		return "", err
	}
	if !ok {
		rest, err := e.line(linePrefix+literal, after, depth)
		return before + literal + rest, err
	}
	parts := strings.Split(exp, "\n")
	b := strings.Builder{}
	b.WriteString(before + parts[0])
	for _, p := range parts[1:] {
		b.WriteString("\n")
		if e.r.Prefix {
			b.WriteString(linePrefix)
		}
		b.WriteString(p)
	}
	// What a reference later on the same line follows: the expansion's last
	// line, with its prefix.
	last := linePrefix + parts[0]
	if len(parts) > 1 {
		last = parts[len(parts)-1]
		if e.r.Prefix {
			last = linePrefix + last
		}
	}
	rest, err := e.line(last, after, depth)
	if err != nil {
		return "", err
	}
	return b.String() + rest, nil
}

// ref is what one reference expands to, already expanded itself.
func (e *expander) ref(inner string, depth int) (string, bool, error) {
	if depth >= MaxDepth {
		return "", false, fmt.Errorf("<<%s>> expands more than %d deep - does a block include itself?", inner, MaxDepth)
	}
	name, args, call := parse(inner)
	if call {
		if e.r.Call == nil {
			e.warn("<<%s>> asks for a block's result, which is not evaluated here", inner)
			return "", false, nil
		}
		res, err := e.r.Call(name, args)
		if err != nil {
			return "", false, fmt.Errorf("<<%s>>: %v", inner, err)
		}
		return strings.TrimRight(res, "\n"), true, nil
	}
	if e.r.Body == nil {
		e.warn("nothing is called %s", name)
		return "", false, nil
	}
	body, ok := e.r.Body(name)
	if !ok {
		e.warn("nothing is called %s", name)
		return "", false, nil
	}
	out, err := e.text(strings.TrimSuffix(body, "\n"), depth+1)
	return out, err == nil, err
}
