package orgs

// Org's own match strings - the tags/property match of an agenda search
// (org manual 6.4, 11.3.3) - read as queries, so a match copied out of an
// Emacs config works in a search box:
//
//	+work-boss+PRIORITY="A"+Coffee="unlimited"+Effort<2|urgent
//	work/NEXT|WAITING        +project/!        {^boss.*}-@home
//	CATEGORY={^cli}&LEVEL>1  SCHEDULED<="<+1w>"/-DONE
//
// Terms: a tag (inherited tags count, as for :tag: in a short query), {regex}
// for tags, NAME op value for a property (PropCompare; NAME={regex} is
// MatchProperty), each + to require or - to exclude; & or nothing between
// terms is and, | is or. After /, the keyword part: NEXT|WAITING, -DONE, and
// ! for open keywords only. Archived trees are left out, as org's agenda
// leaves them.
//
// Told from a short query (shortquery.go) by having no spaces, starting with
// + or - or using one of = < > / { &, and parsing to the end; a short query
// like due:<3d or -:home: does not parse here (a name has no colon), so it
// stays what it was.

import (
	"fmt"
	"strings"
)

// orgMatchToFull is the full form of an org match string, and whether q is
// one.
func orgMatchToFull(q string) (string, bool) {
	q = strings.TrimSpace(q)
	if q == "" || strings.ContainsAny(q, " \t\n") {
		return "", false
	}
	if !strings.HasPrefix(q, "+") && !strings.HasPrefix(q, "-") && !strings.ContainsAny(q, "=<>/{&") {
		return "", false
	}
	// One -word is a short query's negation (-tagged, -orphan) unless
	// something else says org.
	if strings.HasPrefix(q, "-") && strings.Count(q, "-") == 1 && !strings.ContainsAny(q, "=<>/{&|+") {
		return "", false
	}
	p := &orgMatchParser{s: q}
	expr, err := p.tagsPart()
	if err != nil {
		return "", false
	}
	todo := ""
	if p.peek() == '/' {
		p.i++
		if todo, err = p.todoPart(); err != nil {
			return "", false
		}
	}
	if p.i != len(p.s) {
		return "", false
	}
	parts := []string{"!IsArchived()"}
	if expr != "" {
		parts = append(parts, "("+expr+")")
	}
	if todo != "" {
		parts = append(parts, "("+todo+")")
	}
	return strings.Join(parts, " && "), true
}

type orgMatchParser struct {
	s string
	i int
}

func (p *orgMatchParser) peek() byte {
	if p.i < len(p.s) {
		return p.s[p.i]
	}
	return 0
}

func isMatchNameChar(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '@' || c == '#' || c == '%'
}

func (p *orgMatchParser) name() string {
	start := p.i
	for p.i < len(p.s) && isMatchNameChar(p.s[p.i]) {
		p.i++
	}
	return p.s[start:p.i]
}

// braces reads {regex}, the opening brace already at p.i.
func (p *orgMatchParser) braces() (string, error) {
	end := strings.IndexByte(p.s[p.i:], '}')
	if end < 0 {
		return "", fmt.Errorf("unclosed {")
	}
	re := p.s[p.i+1 : p.i+end]
	p.i += end + 1
	return re, nil
}

// tagsPart is the and/or of terms before any /.
func (p *orgMatchParser) tagsPart() (string, error) {
	ors := []string{}
	for {
		and, err := p.andPart()
		if err != nil {
			return "", err
		}
		if and != "" {
			ors = append(ors, and)
		}
		if p.peek() != '|' {
			break
		}
		p.i++
	}
	if len(ors) > 1 {
		for i := range ors {
			ors[i] = "(" + ors[i] + ")"
		}
	}
	return strings.Join(ors, " || "), nil
}

func (p *orgMatchParser) andPart() (string, error) {
	terms := []string{}
	for {
		c := p.peek()
		if c == '&' {
			p.i++
			c = p.peek()
		}
		if c == 0 || c == '|' || c == '/' {
			break
		}
		neg := false
		if c == '+' || c == '-' {
			neg = c == '-'
			p.i++
		}
		t, err := p.term()
		if err != nil {
			return "", err
		}
		if neg {
			t = "!" + t
		}
		terms = append(terms, t)
	}
	return strings.Join(terms, " && "), nil
}

func (p *orgMatchParser) term() (string, error) {
	if p.peek() == '{' {
		re, err := p.braces()
		if err != nil {
			return "", err
		}
		return qcall("HasTagMatching", re), nil
	}
	n := p.name()
	if n == "" {
		return "", fmt.Errorf("col %d: a tag or a property", p.i+1)
	}
	op := ""
	for _, o := range []string{"<=", ">=", "<>", "!=", "==", "=", "<", ">"} {
		if strings.HasPrefix(p.s[p.i:], o) {
			op = o
			break
		}
	}
	if op == "" {
		return qcall("HasTags", n), nil
	}
	p.i += len(op)
	switch c := p.peek(); {
	case c == '{':
		re, err := p.braces()
		if err != nil {
			return "", err
		}
		m := qcall("MatchProperty", strings.ToUpper(n), re)
		if op == "<>" || op == "!=" {
			return "!" + m, nil
		}
		return m, nil
	case c == '"':
		end := strings.IndexByte(p.s[p.i+1:], '"')
		if end < 0 {
			return "", fmt.Errorf("unclosed quote")
		}
		v := p.s[p.i+1 : p.i+1+end]
		p.i += end + 2
		return qcall("PropCompare", strings.ToUpper(n), op, v), nil
	default:
		// A number or a bare word: up to the next term.
		start := p.i
		if p.peek() == '-' {
			p.i++
		}
		for p.i < len(p.s) && !strings.ContainsRune("+-|&/", rune(p.s[p.i])) {
			p.i++
		}
		v := p.s[start:p.i]
		if v == "" {
			return "", fmt.Errorf("col %d: a value after %s", p.i+1, op)
		}
		return qcall("PropCompare", strings.ToUpper(n), op, v), nil
	}
}

// todoPart is the keyword part after /: ! for open keywords only, then
// keywords joined by | (or), each + required or - excluded.
func (p *orgMatchParser) todoPart() (string, error) {
	open := false
	if p.peek() == '!' {
		open = true
		p.i++
	}
	ors := []string{}
	for p.i < len(p.s) {
		ands := []string{}
		for p.i < len(p.s) && p.peek() != '|' {
			neg := false
			if c := p.peek(); c == '+' || c == '-' {
				neg = c == '-'
				p.i++
			}
			kw := p.name()
			if kw == "" {
				return "", fmt.Errorf("col %d: a keyword", p.i+1)
			}
			t := qcall("IsStatus", kw)
			if neg {
				t = "!" + t
			}
			ands = append(ands, t)
		}
		ors = append(ors, strings.Join(ands, " && "))
		if p.peek() == '|' {
			p.i++
		}
	}
	out := strings.Join(ors, " || ")
	if len(ors) > 1 {
		out = "(" + out + ")"
	}
	if open {
		if out == "" {
			return "HasAStatus() && !IsDone()", nil
		}
		return "HasAStatus() && !IsDone() && " + out, nil
	}
	return out, nil
}
