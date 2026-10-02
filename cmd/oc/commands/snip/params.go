package snip

// Parameters: the holes in a saved command line.
//
// Two ways of writing one, because a snippet is an org source block and org
// already has one:
//
//	docker logs -f <container=web>            pet's: a hole with a default
//	kubectl -n <ns=|_dev_||_staging_||_prod_|> get pods     pet's choices
//	rsync -av <src> <dest?>                    optional: left empty, it goes
//	echo \<not a parameter\>                   a literal angle bracket
//
//	#+begin_src sh :var host="db.internal" :var port=5432
//	psql -h $host -p ${port} app
//	#+end_src                                  babel's: a :var the body uses
//
// A `:var` is a parameter when the body names it ($host, ${host}), and its
// value is the default; a lisp list - `:var env='("dev" "prod")` - is a choice.
// The block still runs as written under C-c C-c in Emacs, which is the point of
// using babel's own spelling rather than inventing a third.
//
// Filling in is where pet hurts (its issues #41, #119, #150): a value with a
// space breaks the command, one pasted in gains spaces, `=` in a value splits
// it, there is no escape. Here a value is quoted for the shell as much as where
// it lands needs - inside the template's own quotes it is escaped for those
// quotes; bare, it is single-quoted only if it has something the shell would
// split or act on. A glob or a $VAR in a value is left alone on purpose:
// `<pattern=*.log>` is a person asking for a glob.

import (
	"regexp"
	"sort"
	"strings"

	"github.com/ihdavids/orgs/internal/common"
)

// Param is one hole.
type Param struct {
	Name     string
	Default  string
	Choices  []string
	Optional bool
	// FromVar says it came from a :var header argument.
	FromVar bool
}

// A hole: `<` then a name, an optional `?`, an optional `=default`, then `>`.
// The name has to look like a name, so `a < b > c` and `<<EOF` are not holes;
// what pet takes for one (`<b>` in some html) can be written `\<b>`.
var holeRe = regexp.MustCompile(`\\?<([A-Za-z_][A-Za-z0-9_.-]*)(\?)?(?:=([^<>\n]*))?>`)

var choiceRe = regexp.MustCompile(`\|_(.*?)_\|`)

// lispList reads `'("dev" "prod")` or `("dev" "prod")` or `'(dev prod)`.
var lispList = regexp.MustCompile(`^'?\((.*)\)$`)
var lispItem = regexp.MustCompile(`"((?:[^"\\]|\\.)*)"|(\S+)`)

// Parse finds a command's parameters, in the order they are first written.
// A name used twice is one parameter; the last default written wins, as in
// pet. A :var the body uses is a parameter too, and gives the default to an
// inline hole of the same name that has none.
func Parse(cmd string, vars []common.CodeVar) []Param {
	out := []Param{}
	at := map[string]int{}
	for _, m := range holeRe.FindAllStringSubmatchIndex(cmd, -1) {
		if cmd[m[0]] == '\\' {
			continue
		}
		name := cmd[m[2]:m[3]]
		p := Param{Name: name, Optional: m[4] >= 0}
		if m[6] >= 0 {
			def := cmd[m[6]:m[7]]
			if cs := choiceRe.FindAllStringSubmatch(def, -1); len(cs) > 0 {
				for _, c := range cs {
					p.Choices = append(p.Choices, c[1])
				}
				def = p.Choices[0]
			}
			p.Default = def
		}
		if i, ok := at[name]; ok {
			if m[6] >= 0 {
				out[i].Default, out[i].Choices = p.Default, p.Choices
			}
			out[i].Optional = out[i].Optional || p.Optional
			continue
		}
		at[name] = len(out)
		out = append(out, p)
	}
	for _, v := range vars {
		if v.Ref != "" && v.RefKind != "" {
			// A table or another block's result is data, not a value to type.
			continue
		}
		def, choices := varValue(v.Value)
		if i, ok := at[v.Name]; ok {
			if out[i].Default == "" && len(out[i].Choices) == 0 {
				out[i].Default, out[i].Choices = def, choices
			}
			out[i].FromVar = true
			continue
		}
		if !usesVar(cmd, v.Name) {
			continue
		}
		at[v.Name] = len(out)
		out = append(out, Param{Name: v.Name, Default: def, Choices: choices, FromVar: true})
	}
	return out
}

func varValue(v string) (string, []string) {
	v = strings.TrimSpace(v)
	if m := lispList.FindStringSubmatch(v); m != nil {
		choices := []string{}
		for _, it := range lispItem.FindAllStringSubmatch(m[1], -1) {
			if it[1] != "" || strings.HasPrefix(it[0], `"`) {
				choices = append(choices, strings.ReplaceAll(it[1], `\"`, `"`))
			} else {
				choices = append(choices, it[2])
			}
		}
		if len(choices) > 0 {
			return choices[0], choices
		}
	}
	if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
		return strings.ReplaceAll(v[1:len(v)-1], `\"`, `"`), nil
	}
	return v, nil
}

func varRefRe(name string) *regexp.Regexp {
	q := regexp.QuoteMeta(name)
	return regexp.MustCompile(`\$\{` + q + `\}|\$` + q + `\b`)
}

func usesVar(cmd, name string) bool { return varRefRe(name).MatchString(cmd) }

// Fill puts values into the holes and the variables. A parameter with no
// value takes its default; an optional one with neither disappears, with the
// space in front of it.
func Fill(cmd string, params []Param, values map[string]string) string {
	val := func(p Param) string {
		if v, ok := values[p.Name]; ok {
			return v
		}
		return p.Default
	}
	byName := map[string]Param{}
	for _, p := range params {
		byName[p.Name] = p
	}

	out := strings.Builder{}
	last := 0
	for _, m := range holeRe.FindAllStringSubmatchIndex(cmd, -1) {
		if cmd[m[0]] == '\\' {
			// The escape goes; the brackets stay as written.
			out.WriteString(cmd[last:m[0]])
			out.WriteString(cmd[m[0]+1 : m[1]])
			last = m[1]
			continue
		}
		p := byName[cmd[m[2]:m[3]]]
		v := val(p)
		if v == "" && p.Optional {
			s := cmd[last:m[0]]
			// An optional hole takes one space with it, so `a <b?> c`
			// is `a c`, not `a  c`.
			if strings.HasSuffix(s, " ") && (m[1] >= len(cmd) || cmd[m[1]] == ' ' || cmd[m[1]] == '\n') {
				s = s[:len(s)-1]
			}
			out.WriteString(s)
			last = m[1]
			continue
		}
		out.WriteString(cmd[last:m[0]])
		out.WriteString(quoteFor(cmd, m[0], v))
		last = m[1]
	}
	out.WriteString(cmd[last:])
	s := out.String()

	// Then the :var parameters, wherever the body names them.
	names := []string{}
	for _, p := range params {
		if p.FromVar {
			names = append(names, p.Name)
		}
	}
	// Longest first, so $hostname is not taken for $host followed by "name".
	sort.Slice(names, func(a, b int) bool { return len(names[a]) > len(names[b]) })
	for _, n := range names {
		re := varRefRe(n)
		v := val(byName[n])
		res := strings.Builder{}
		last := 0
		for _, m := range re.FindAllStringIndex(s, -1) {
			if quoteAt(s, m[0]) == '\'' {
				// Inside single quotes the shell would not have expanded it
				// either.
				continue
			}
			res.WriteString(s[last:m[0]])
			res.WriteString(quoteFor(s, m[0], v))
			last = m[1]
		}
		res.WriteString(s[last:])
		s = res.String()
	}
	return s
}

// quoteAt is the quote the shell is inside at byte i of s: '\'', '"' or 0.
func quoteAt(s string, i int) byte {
	var q byte
	for j := 0; j < i && j < len(s); j++ {
		c := s[j]
		switch {
		case q == 0 && c == '\\':
			j++
		case q == '"' && c == '\\':
			j++
		case q == 0 && (c == '\'' || c == '"'):
			q = c
		case q != 0 && c == q:
			q = 0
		}
	}
	return q
}

// quoteFor writes a value so that it lands as one word where it is going.
func quoteFor(s string, i int, v string) string {
	switch quoteAt(s, i) {
	case '\'':
		return strings.ReplaceAll(v, `'`, `'\''`)
	case '"':
		r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "`", "\\`")
		return r.Replace(v)
	}
	if v == "" || !strings.ContainsAny(v, " \t\n'\";&|<>()`\\") {
		return v
	}
	return "'" + strings.ReplaceAll(v, `'`, `'\''`) + "'"
}
