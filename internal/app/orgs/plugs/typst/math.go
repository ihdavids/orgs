package typst

import (
	"fmt"
	"strings"
	"unicode"
)

// LaTeX math, said in typst's own math language.
//
// mitex (a typst package that reads LaTeX math) is what the exporter uses by
// default, because it understands everything a person might write. This is the
// other way, `#+TYPST_MATH: typst`, for a document that wants plain typst it can
// edit afterwards: the common things - letters, fractions, roots, sub- and
// superscripts, Greek, the usual operators and relations - are translated, and
// anything else makes texToTypst give up so the caller can fall back to mitex
// for that one formula rather than writing something wrong.

// LaTeX commands with a typst name of their own.
var mathSymbols = map[string]string{
	"alpha": "alpha", "beta": "beta", "gamma": "gamma", "delta": "delta",
	"epsilon": "epsilon.alt", "varepsilon": "epsilon", "zeta": "zeta", "eta": "eta",
	"theta": "theta", "vartheta": "theta.alt", "iota": "iota", "kappa": "kappa",
	"lambda": "lambda", "mu": "mu", "nu": "nu", "xi": "xi", "pi": "pi",
	"varpi": "pi.alt", "rho": "rho", "sigma": "sigma", "tau": "tau",
	"upsilon": "upsilon", "phi": "phi.alt", "varphi": "phi", "chi": "chi",
	"psi": "psi", "omega": "omega",
	"Gamma": "Gamma", "Delta": "Delta", "Theta": "Theta", "Lambda": "Lambda",
	"Xi": "Xi", "Pi": "Pi", "Sigma": "Sigma", "Upsilon": "Upsilon",
	"Phi": "Phi", "Psi": "Psi", "Omega": "Omega",
	"sum": "sum", "prod": "product", "int": "integral", "iint": "integral.double",
	"oint": "integral.cont", "infty": "infinity", "partial": "partial", "nabla": "nabla",
	"cdot": "dot.op", "times": "times", "div": "div", "pm": "plus.minus", "mp": "minus.plus",
	"le": "<=", "leq": "<=", "ge": ">=", "geq": ">=", "ne": "!=", "neq": "!=",
	"approx": "approx", "equiv": "equiv", "sim": "tilde.op", "propto": "prop",
	"to": "->", "rightarrow": "->", "leftarrow": "<-", "Rightarrow": "=>",
	"Leftarrow": "arrow.l.double", "leftrightarrow": "<->", "Leftrightarrow": "<=>",
	"mapsto": "|->", "implies": "=>", "iff": "<=>",
	"in": "in", "notin": "in.not", "subset": "subset", "subseteq": "subset.eq",
	"supset": "supset", "supseteq": "supset.eq", "cup": "union", "cap": "sect",
	"emptyset": "emptyset", "forall": "forall", "exists": "exists", "neg": "not",
	"land": "and", "lor": "or", "ldots": "dots", "cdots": "dots.c", "dots": "dots",
	"circ": "compose", "ast": "ast", "star": "star", "prime": "prime",
	"langle": "angle.l", "rangle": "angle.r", "lfloor": "floor.l", "rfloor": "floor.r",
	"lceil": "ceil.l", "rceil": "ceil.r", "mid": "|", "vert": "|", "|": "||",
	"sin": "sin", "cos": "cos", "tan": "tan", "cot": "cot", "sec": "sec", "csc": "csc",
	"arcsin": "arcsin", "arccos": "arccos", "arctan": "arctan", "sinh": "sinh",
	"cosh": "cosh", "tanh": "tanh", "log": "log", "ln": "ln", "exp": "exp",
	"lim": "lim", "max": "max", "min": "min", "sup": "sup", "inf": "inf",
	"det": "det", "gcd": "gcd", "deg": "deg", "dim": "dim", "ker": "ker",
	"quad": "quad", "qquad": "wide", ",": "thin", ";": "med", ":": "med", "!": "",
	"{": "{", "}": "}", "%": "%", "&": "amp", "#": "\\#", "$": "\\$", "_": "\\_",
}

// Commands that take one argument and wrap it.
var mathWrap = map[string]string{
	"sqrt": "sqrt", "hat": "hat", "widehat": "hat", "bar": "overline",
	"overline": "overline", "underline": "underline", "vec": "arrow",
	"dot": "dot", "ddot": "dot.double", "tilde": "tilde", "widetilde": "tilde",
	"mathbf": "bold", "boldsymbol": "bold", "mathit": "italic", "mathrm": "upright",
	"mathcal": "cal", "mathbb": "bb", "mathfrak": "frak", "mathsf": "sans",
	"mathtt": "mono", "operatorname": "op",
}

type mathParser struct {
	s   []rune
	i   int
	err error
}

// texToTypst translates LaTeX math into typst math, or says it cannot.
func texToTypst(tex string) (string, error) {
	p := &mathParser{s: []rune(tex)}
	out := p.seq(0)
	if p.err != nil {
		return "", p.err
	}
	if p.i < len(p.s) {
		return "", fmt.Errorf("unexpected %q", string(p.s[p.i]))
	}
	return strings.Join(strings.Fields(out), " "), nil
}

func (p *mathParser) fail(format string, args ...interface{}) string {
	if p.err == nil {
		p.err = fmt.Errorf(format, args...)
	}
	return ""
}

// seq reads atoms until the end, or until the closing brace of a group
// (depth > 0).
func (p *mathParser) seq(depth int) string {
	parts := []string{}
	for p.i < len(p.s) && p.err == nil {
		r := p.s[p.i]
		if r == '}' {
			if depth == 0 {
				return p.fail("an unmatched }")
			}
			break
		}
		if r == '^' || r == '_' {
			p.i++
			arg := p.atom()
			if len(parts) == 0 {
				parts = append(parts, `""`)
			}
			parts[len(parts)-1] += string(r) + "(" + unparen(arg) + ")"
			continue
		}
		if unicode.IsSpace(r) {
			p.i++
			continue
		}
		parts = append(parts, p.atom())
	}
	return strings.Join(parts, " ")
}

// atom reads one thing: a group, a command (with its arguments), a letter, a
// number or a symbol.
func (p *mathParser) atom() string {
	for p.i < len(p.s) && unicode.IsSpace(p.s[p.i]) {
		p.i++
	}
	if p.i >= len(p.s) {
		return p.fail("something was missing at the end")
	}
	r := p.s[p.i]
	switch {
	case r == '{':
		p.i++
		inner := p.seq(1)
		if p.i >= len(p.s) || p.s[p.i] != '}' {
			return p.fail("an unclosed {")
		}
		p.i++
		return "(" + inner + ")"
	case r == '\\':
		return p.command()
	case unicode.IsLetter(r):
		// One letter is one variable: typst reads `xy` as a name.
		p.i++
		return string(r)
	case unicode.IsDigit(r) || r == '.':
		start := p.i
		for p.i < len(p.s) && (unicode.IsDigit(p.s[p.i]) || p.s[p.i] == '.') {
			p.i++
		}
		return string(p.s[start:p.i])
	case r == '&' || r == '#' || r == '$' || r == '"':
		// Alignment and anything typst would read as code: not a simple formula.
		return p.fail("%q is not something this translation handles", string(r))
	case r == '/':
		p.i++
		return `\/`
	case r == '\'':
		p.i++
		return "'"
	case r == '~':
		p.i++
		return "space"
	default:
		p.i++
		switch r {
		case '(', ')', '[', ']', '+', '-', '=', '<', '>', ',', ';', ':', '!', '|', '*', '?':
			return string(r)
		}
		return p.fail("%q is not something this translation handles", string(r))
	}
}

func (p *mathParser) command() string {
	p.i++ // the backslash
	if p.i >= len(p.s) {
		return p.fail("a lone backslash")
	}
	start := p.i
	if unicode.IsLetter(p.s[p.i]) {
		for p.i < len(p.s) && unicode.IsLetter(p.s[p.i]) {
			p.i++
		}
	} else {
		p.i++
	}
	name := string(p.s[start:p.i])
	switch name {
	case "frac", "dfrac", "tfrac":
		a := p.atom()
		b := p.atom()
		return "frac(" + unparen(a) + ", " + unparen(b) + ")"
	case "binom":
		a := p.atom()
		b := p.atom()
		return "binom(" + unparen(a) + ", " + unparen(b) + ")"
	case "sqrt":
		if p.i < len(p.s) && p.s[p.i] == '[' {
			end := strings.IndexRune(string(p.s[p.i:]), ']')
			if end < 0 {
				return p.fail("an unclosed [")
			}
			n, err := texToTypst(string(p.s[p.i+1 : p.i+end]))
			if err != nil {
				return p.fail("%v", err)
			}
			p.i += end + 1
			return "root(" + n + ", " + unparen(p.atom()) + ")"
		}
		return "sqrt(" + unparen(p.atom()) + ")"
	case "text", "textrm", "mbox", "textit", "textbf":
		if p.i >= len(p.s) || p.s[p.i] != '{' {
			return p.fail("\\%s without braces", name)
		}
		end := strings.IndexRune(string(p.s[p.i:]), '}')
		if end < 0 {
			return p.fail("an unclosed {")
		}
		t := string(p.s[p.i+1 : p.i+end])
		p.i += end + 1
		return str(t)
	case "left", "right", "big", "Big", "bigg", "Bigg", "displaystyle", "limits", "nolimits":
		// Typst sizes delimiters itself; a \left. is nothing at all.
		if p.i < len(p.s) && p.s[p.i] == '.' {
			p.i++
		}
		return ""
	case "begin", "end", "\\":
		return p.fail("\\%s is more than this translation handles", name)
	}
	if fn, ok := mathWrap[name]; ok {
		return fn + "(" + unparen(p.atom()) + ")"
	}
	if sym, ok := mathSymbols[name]; ok {
		return sym
	}
	return p.fail("\\%s is not something this translation knows", name)
}

// A group's parentheses are typst's grouping too; as a function's argument
// they would be drawn.
func unparen(s string) string {
	if strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")") && balanced(s[1:len(s)-1]) {
		return s[1 : len(s)-1]
	}
	return s
}

func balanced(s string) bool {
	depth := 0
	for _, r := range s {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
			if depth < 0 {
				return false
			}
		}
	}
	return depth == 0
}
