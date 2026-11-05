package common

// org-edna's little language, read: the TRIGGER and BLOCKER properties. Pure,
// so the server (which runs them, internal/app/orgs/edna.go) and `orgs lsp`
// (which points at a misspelt one) read them the same way.
//
//	:TRIGGER: next-sibling todo!(NEXT) scheduled!("+1d")
//	:BLOCKER: previous-sibling
//	:BLOCKER: ids(a1b2 c3d4) !done?
//	:TRIGGER: if siblings done? then parent todo!(DONE) endif
//
// A form is a finder (`children`), an action (`todo!(NEXT)`), a condition
// (`done?`, `!done?`), a consideration (`consider(all)`) or one of `if`,
// `then`, `else`, `endif`. Finders gather headings; the next action or
// condition works on what they gathered, and a finder after an action starts
// a new gathering.

import (
	"fmt"
	"sort"
	"strings"
)

type EdnaKind string

const (
	EdnaFinder    EdnaKind = "finder"
	EdnaAction    EdnaKind = "action"
	EdnaCondition EdnaKind = "condition"
	EdnaConsider  EdnaKind = "consider"
	EdnaKeyword   EdnaKind = "keyword"
)

// EdnaForm is one word of a TRIGGER or BLOCKER. Start and End are byte
// offsets into the text it was read from.
type EdnaForm struct {
	Kind  EdnaKind
	Name  string
	Args  []string `json:",omitempty"`
	Neg   bool     `json:",omitempty"`
	Start int
	End   int
}

// EdnaError is a problem with one stretch of the text.
type EdnaError struct {
	Msg   string
	Start int
	End   int
}

func (e *EdnaError) Error() string { return e.Msg }

// What each name is, and a line on what it does - for errors, completion and
// `orgs edna -names`.
var EdnaFinders = map[string]string{
	"self":                 "this heading",
	"parent":               "the heading above",
	"children":             "the headings directly under it",
	"first-child":          "the first heading under it",
	"last-child":           "the last heading under it",
	"descendants":          "every heading under it, at any depth",
	"ancestors":            "every heading above it, nearest first",
	"siblings":             "the other headings under the same parent",
	"siblings-wrap":        "the other siblings, the ones after it first",
	"rest-of-siblings":     "the siblings after it",
	"next-sibling":         "the sibling after it",
	"next-sibling-wrap":    "the sibling after it, or the first if it is last",
	"previous-sibling":     "the sibling before it",
	"ids":                  "the headings with these IDs or CUSTOM_IDs: ids(a1b2 c3d4)",
	"match":                "the headings an org match string finds: match(\"+work-done\")",
	"olp":                  "a heading by file and outline path: olp(\"tasks.org\" \"Project/Step\")",
}

var EdnaActions = map[string]string{
	"todo!":            "set the keyword: todo!(NEXT)",
	"scheduled!":       "set SCHEDULED: \"+1d\", \"++2h\" (from now), \"mon\", \"2026-12-01\", \"cp\" (copy this heading's), \"rm\"",
	"deadline!":        "set DEADLINE, as scheduled!",
	"tag!":             "set the tags: tag!(\"work:urgent\")",
	"set-property!":    "set a property: set-property!(\"OWNER\" \"me\")",
	"delete-property!": "remove a property: delete-property!(\"OWNER\")",
	"set-priority!":    "set the priority: set-priority!(\"A\"), \"up\", \"down\", \"remove\"",
	"set-effort!":      "set EFFORT: set-effort!(\"1:30\")",
	"archive!":         "archive the heading",
	"chain!":           "copy one of this heading's properties onto the target: chain!(\"TRIGGER\")",
}

var EdnaConditions = map[string]string{
	"done?":         "is in a done keyword",
	"todo-state?":   "has this keyword: todo-state?(WAITING)",
	"headings?":     "the finders found anything at all",
	"has-property?": "has this property, with this value: has-property?(\"OWNER\" \"me\")",
	"has-tags?":     "has any of these tags: has-tags?(\"work\" \"home\")",
	"matches?":      "an org match string finds it: matches?(\"+urgent\")",
	"re-search?":    "its text matches a regular expression: re-search?(\"TODO\")",
}

// The finder options edna's finders take: children(todo-only).
var EdnaFinderOptions = map[string]string{
	"todo-only":          "only headings with an active keyword",
	"todo-and-done-only": "only headings with any keyword",
	"no-comment":         "not COMMENT headings",
	"no-archive":         "not headings tagged ARCHIVE",
}

// EdnaNames is every name, sorted, for completion.
func EdnaNames() []string {
	out := []string{"if", "then", "else", "endif", "consider"}
	for _, m := range []map[string]string{EdnaFinders, EdnaActions, EdnaConditions} {
		for k := range m {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// ParseEdna reads a TRIGGER or BLOCKER value.
func ParseEdna(text string) ([]EdnaForm, error) {
	out := []EdnaForm{}
	i := 0
	n := len(text)
	for i < n {
		for i < n && (text[i] == ' ' || text[i] == '\t') {
			i++
		}
		if i >= n {
			break
		}
		start := i
		neg := false
		if text[i] == '!' {
			neg = true
			i++
		}
		ns := i
		for i < n && text[i] != '(' && text[i] != ' ' && text[i] != '\t' {
			i++
		}
		name := strings.ToLower(text[ns:i])
		if name == "" {
			return out, &EdnaError{Msg: "a ! with nothing after it", Start: start, End: i}
		}
		var args []string
		if i < n && text[i] == '(' {
			a, end, err := ednaArgs(text, i)
			if err != nil {
				return out, err
			}
			args, i = a, end
		}
		f := EdnaForm{Name: name, Args: args, Neg: neg, Start: start, End: i}
		switch {
		case name == "if" || name == "then" || name == "else" || name == "endif":
			f.Kind = EdnaKeyword
		case name == "consider" || name == "consideration":
			f.Kind, f.Name = EdnaConsider, "consider"
		case strings.HasSuffix(name, "!"):
			f.Kind = EdnaAction
			if _, ok := EdnaActions[name]; !ok {
				return out, &EdnaError{Msg: fmt.Sprintf("no action called %s%s", name, suggestEdna(name, EdnaActions)), Start: start, End: i}
			}
		case strings.HasSuffix(name, "?"):
			f.Kind = EdnaCondition
			if _, ok := EdnaConditions[name]; !ok {
				return out, &EdnaError{Msg: fmt.Sprintf("no condition called %s%s", name, suggestEdna(name, EdnaConditions)), Start: start, End: i}
			}
		default:
			f.Kind = EdnaFinder
			if _, ok := EdnaFinders[name]; !ok {
				return out, &EdnaError{Msg: fmt.Sprintf("no finder called %s%s", name, suggestEdna(name, EdnaFinders)), Start: start, End: i}
			}
		}
		if neg && f.Kind != EdnaCondition {
			return out, &EdnaError{Msg: "only a condition can be negated with !: " + name, Start: start, End: i}
		}
		if err := checkEdnaArgs(f); err != nil {
			return out, err
		}
		out = append(out, f)
	}
	return out, checkEdnaIfs(out)
}

// ednaArgs reads `(a "b c" d)` from the paren at i: words split on spaces,
// quotes keep spaces in a word, a nested paren is part of a word.
func ednaArgs(text string, i int) ([]string, int, error) {
	open := i
	i++
	args := []string{}
	var cur strings.Builder
	have := false
	depth := 0
	for i < len(text) {
		c := text[i]
		switch {
		case c == '"':
			have = true
			i++
			for i < len(text) && text[i] != '"' {
				if text[i] == '\\' && i+1 < len(text) {
					i++
				}
				cur.WriteByte(text[i])
				i++
			}
			if i >= len(text) {
				return nil, i, &EdnaError{Msg: "a quote that is never closed", Start: open, End: i}
			}
		case c == '(':
			depth++
			cur.WriteByte(c)
			have = true
		case c == ')' && depth > 0:
			depth--
			cur.WriteByte(c)
		case c == ')':
			if have {
				args = append(args, cur.String())
			}
			return args, i + 1, nil
		case c == ' ' || c == '\t':
			if have {
				args = append(args, cur.String())
				cur.Reset()
				have = false
			}
		default:
			cur.WriteByte(c)
			have = true
		}
		i++
	}
	return nil, i, &EdnaError{Msg: "a ( that is never closed", Start: open, End: i}
}

func checkEdnaArgs(f EdnaForm) error {
	bad := func(msg string) error { return &EdnaError{Msg: f.Name + ": " + msg, Start: f.Start, End: f.End} }
	switch f.Kind {
	case EdnaFinder:
		switch f.Name {
		case "ids", "match":
			if len(f.Args) == 0 {
				return bad("needs at least one argument")
			}
		case "olp":
			if len(f.Args) != 2 {
				return bad("needs a file and an outline path")
			}
		default:
			for _, a := range f.Args {
				if _, ok := EdnaFinderOptions[strings.ToLower(a)]; !ok {
					return bad(fmt.Sprintf("no option %q (todo-only, todo-and-done-only, no-comment, no-archive)", a))
				}
			}
		}
	case EdnaAction:
		want := map[string]int{"todo!": 1, "scheduled!": 1, "deadline!": 1, "tag!": 1, "set-property!": 2,
			"delete-property!": 1, "set-priority!": 1, "set-effort!": 1, "archive!": 0, "chain!": 1}[f.Name]
		if len(f.Args) != want {
			return bad(fmt.Sprintf("takes %d argument%s, not %d", want, map[bool]string{true: "", false: "s"}[want == 1], len(f.Args)))
		}
	case EdnaCondition:
		switch f.Name {
		case "todo-state?", "matches?", "re-search?":
			if len(f.Args) != 1 {
				return bad("takes one argument")
			}
		case "has-property?":
			if len(f.Args) != 2 {
				return bad("takes a property and a value")
			}
		case "has-tags?":
			if len(f.Args) == 0 {
				return bad("needs at least one tag")
			}
		}
	case EdnaConsider:
		if len(f.Args) != 1 {
			return bad("takes any, all, a count or a fraction")
		}
	}
	return nil
}

// if ... then ... [else ...] endif, properly nested.
func checkEdnaIfs(forms []EdnaForm) error {
	type open struct {
		at       int
		sawThen  bool
		sawElse  bool
	}
	stack := []open{}
	for i, f := range forms {
		if f.Kind != EdnaKeyword {
			continue
		}
		err := func(msg string) error { return &EdnaError{Msg: msg, Start: f.Start, End: f.End} }
		switch f.Name {
		case "if":
			stack = append(stack, open{at: i})
		case "then":
			if len(stack) == 0 || stack[len(stack)-1].sawThen {
				return err("then without an if")
			}
			stack[len(stack)-1].sawThen = true
		case "else":
			if len(stack) == 0 || !stack[len(stack)-1].sawThen || stack[len(stack)-1].sawElse {
				return err("else without an if ... then")
			}
			stack[len(stack)-1].sawElse = true
		case "endif":
			if len(stack) == 0 || !stack[len(stack)-1].sawThen {
				return err("endif without an if ... then")
			}
			stack = stack[:len(stack)-1]
		}
	}
	if len(stack) > 0 {
		f := forms[stack[len(stack)-1].at]
		return &EdnaError{Msg: "if without an endif", Start: f.Start, End: f.End}
	}
	return nil
}

// ValidateEdna is ParseEdna plus what makes sense in which property: a
// BLOCKER may not act, and a TRIGGER's condition only means something
// inside an if.
func ValidateEdna(prop, text string) error {
	forms, err := ParseEdna(text)
	if err != nil {
		return err
	}
	depth := 0
	for _, f := range forms {
		if f.Kind == EdnaKeyword {
			switch f.Name {
			case "if":
				depth++
			case "endif":
				depth--
			}
		}
		switch strings.ToUpper(prop) {
		case "BLOCKER":
			if f.Kind == EdnaAction {
				return &EdnaError{Msg: "a BLOCKER cannot act (" + f.Name + "): that belongs in a TRIGGER", Start: f.Start, End: f.End}
			}
			if f.Kind == EdnaKeyword {
				return &EdnaError{Msg: "if ... endif belongs in a TRIGGER", Start: f.Start, End: f.End}
			}
		case "TRIGGER":
			if f.Kind == EdnaCondition && depth == 0 {
				return &EdnaError{Msg: "a condition in a TRIGGER only means something inside if ... then", Start: f.Start, End: f.End}
			}
		}
	}
	return nil
}

func suggestEdna(name string, in map[string]string) string {
	best, bestD := "", 4
	for k := range in {
		if d := editDistance(name, k); d < bestD {
			best, bestD = k, d
		}
	}
	if best == "" {
		return ""
	}
	return " - did you mean " + best + "?"
}

func editDistance(a, b string) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			c := 1
			if a[i-1] == b[j-1] {
				c = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+c)
		}
		prev = cur
	}
	return prev[len(b)]
}
