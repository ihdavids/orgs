package common

// org-ql's query language, turned into orgs' own full-form query, so an
// org-ql block (internal/app/orgs/orgql.go) is answered by the query engine
// everything else uses and nothing is evaluated twice.
//
// Both of org-ql's spellings are read:
//
//	(and (todo "NEXT") (tags "work") (not (done)) (deadline :to today))
//	todo:NEXT tags:work !done: deadline:to=today
//
// In the second (org-ql's "non-sexp" syntax) terms side by side must all
// match, `!` turns one round, `pred:a,b` gives a predicate several
// arguments, `pred:key=value` a keyword one, and a plain word or "phrase" is
// looked for in the heading and its text. Predicates orgs has nothing to
// answer with (ancestors, outline-path, clocked ranges...) are an error that
// names them, never silently true.

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// OrgQlToQuery translates an org-ql query, sexp or plain, into orgs' full
// form. now is the day relative dates (-7, today) are counted from.
func OrgQlToQuery(q string, now time.Time) (string, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return "", fmt.Errorf("an empty org-ql query")
	}
	var node qlNode
	var err error
	if strings.HasPrefix(q, "(") {
		r := &qlReader{s: q}
		node, err = r.read()
		if err == nil {
			r.skip()
			if r.i < len(r.s) {
				err = fmt.Errorf("more after the query's closing bracket: %q", r.s[r.i:])
			}
		}
	} else {
		node, err = qlPlain(q)
	}
	if err != nil {
		return "", err
	}
	t := qlTranslator{now: now}
	return t.expr(node)
}

// ---------------------------------------------------------------------------
// Reading
// ---------------------------------------------------------------------------

// A read query: an atom (a symbol, :keyword, number) or a "string", or a
// list.
type qlNode struct {
	Atom   string
	Str    bool // Atom came in quotes
	List   []qlNode
	IsList bool
}

func (n qlNode) String() string {
	if !n.IsList {
		if n.Str {
			return strconv.Quote(n.Atom)
		}
		return n.Atom
	}
	parts := make([]string, len(n.List))
	for i, c := range n.List {
		parts[i] = c.String()
	}
	return "(" + strings.Join(parts, " ") + ")"
}

type qlReader struct {
	s string
	i int
}

func (r *qlReader) skip() {
	for r.i < len(r.s) {
		c := r.s[r.i]
		if c == ';' { // a comment to the end of the line
			for r.i < len(r.s) && r.s[r.i] != '\n' {
				r.i++
			}
			continue
		}
		if !unicode.IsSpace(rune(c)) {
			return
		}
		r.i++
	}
}

func (r *qlReader) read() (qlNode, error) {
	r.skip()
	if r.i >= len(r.s) {
		return qlNode{}, fmt.Errorf("the query ends before its brackets close")
	}
	switch c := r.s[r.i]; {
	case c == '\'': // 'symbol and '(list) read as themselves
		r.i++
		return r.read()
	case c == '(':
		r.i++
		n := qlNode{IsList: true}
		for {
			r.skip()
			if r.i >= len(r.s) {
				return qlNode{}, fmt.Errorf("a ( is never closed")
			}
			if r.s[r.i] == ')' {
				r.i++
				return n, nil
			}
			c, err := r.read()
			if err != nil {
				return qlNode{}, err
			}
			n.List = append(n.List, c)
		}
	case c == ')':
		return qlNode{}, fmt.Errorf("a ) with no ( before it")
	case c == '"':
		r.i++
		var b strings.Builder
		for r.i < len(r.s) && r.s[r.i] != '"' {
			if r.s[r.i] == '\\' && r.i+1 < len(r.s) {
				r.i++
			}
			b.WriteByte(r.s[r.i])
			r.i++
		}
		if r.i >= len(r.s) {
			return qlNode{}, fmt.Errorf("a string is never closed")
		}
		r.i++
		return qlNode{Atom: b.String(), Str: true}, nil
	}
	start := r.i
	for r.i < len(r.s) && !unicode.IsSpace(rune(r.s[r.i])) && !strings.ContainsRune(`()"`, rune(r.s[r.i])) {
		r.i++
	}
	return qlNode{Atom: r.s[start:r.i]}, nil
}

// qlPlain reads org-ql's non-sexp syntax into the sexp it stands for.
func qlPlain(q string) (qlNode, error) {
	out := qlNode{IsList: true, List: []qlNode{{Atom: "and"}}}
	for _, term := range qlTerms(q) {
		neg := false
		if strings.HasPrefix(term, "!") {
			neg, term = true, term[1:]
		}
		var n qlNode
		pred, args, isPred := strings.Cut(term, ":")
		if isPred && qlIsPredName(pred) && !strings.HasPrefix(term, `"`) {
			n = qlNode{IsList: true, List: []qlNode{{Atom: pred}}}
			if args != "" {
				for _, a := range qlSplitArgs(args) {
					if k, v, ok := strings.Cut(a, "="); ok && qlKeywordArg(pred, k) {
						n.List = append(n.List, qlNode{Atom: ":" + k}, qlNode{Atom: qlUnquote(v)})
						continue
					}
					// property:CLIENT=acme is the name and the value.
					if k, v, ok := strings.Cut(a, "="); ok && (pred == "property" || pred == "prop") {
						n.List = append(n.List, qlNode{Atom: k, Str: true}, qlNode{Atom: qlUnquote(v), Str: true})
						continue
					}
					n.List = append(n.List, qlNode{Atom: qlUnquote(a), Str: true})
				}
			}
		} else {
			n = qlNode{IsList: true, List: []qlNode{{Atom: "rifle"}, {Atom: qlUnquote(term), Str: true}}}
		}
		if neg {
			n = qlNode{IsList: true, List: []qlNode{{Atom: "not"}, n}}
		}
		out.List = append(out.List, n)
	}
	if len(out.List) == 1 {
		return qlNode{}, fmt.Errorf("an empty org-ql query")
	}
	return out, nil
}

// Words, keeping "quoted phrases" (and pred:"quoted, values") whole.
func qlTerms(q string) []string {
	out := []string{}
	var b strings.Builder
	in := false
	for _, r := range q {
		switch {
		case r == '"':
			in = !in
			b.WriteRune(r)
		case unicode.IsSpace(r) && !in:
			if b.Len() > 0 {
				out = append(out, b.String())
				b.Reset()
			}
		default:
			b.WriteRune(r)
		}
	}
	if b.Len() > 0 {
		out = append(out, b.String())
	}
	return out
}

func qlSplitArgs(s string) []string {
	out := []string{}
	var b strings.Builder
	in := false
	for _, r := range s {
		switch {
		case r == '"':
			in = !in
			b.WriteRune(r)
		case r == ',' && !in:
			out = append(out, b.String())
			b.Reset()
		default:
			b.WriteRune(r)
		}
	}
	return append(out, b.String())
}

func qlUnquote(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

// The predicates the plain syntax knows by name, so `http://x` or `a:b` in
// a search is text, not an unknown predicate.
var qlPredNames = map[string]bool{
	"todo": true, "done": true, "tags": true, "tags-all": true, "tags&": true, "tags-local": true,
	"tags-inherited": true, "tags-i": true, "tags-l": true, "itags": true, "ltags": true,
	"priority": true, "heading": true, "h": true, "regexp": true, "r": true, "rifle": true, "smart": true,
	"property": true, "prop": true, "level": true, "habit": true, "scheduled": true, "deadline": true,
	"closed": true, "planning": true, "ts": true, "ts-active": true, "ts-a": true, "ts-inactive": true,
	"ts-i": true, "clocked": true, "category": true, "path": true, "blocked": true, "link": true,
	"children": true, "descendants": true, "parent": true, "ancestors": true, "src": true, "outline-path": true,
	"olp": true, "outline-path-segment": true, "olps": true, "effort": true, "heading-regexp": true, "h*": true,
}

func qlIsPredName(p string) bool { return qlPredNames[strings.ToLower(p)] }

// Which name=value words are keyword arguments: the date predicates' from,
// to, on; property's inherit. Anything else (property:CLIENT=acme) is an
// argument.
func qlKeywordArg(pred, key string) bool {
	switch strings.ToLower(key) {
	case "from", "to", "on":
		return pred != "property" && pred != "prop"
	case "inherit":
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// Translating
// ---------------------------------------------------------------------------

type qlTranslator struct{ now time.Time }

func qlQ(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}

func qlCall(f string, args ...string) string {
	q := make([]string, len(args))
	for i, a := range args {
		q[i] = qlQ(a)
	}
	return f + "(" + strings.Join(q, ", ") + ")"
}

func qlJoin(op string, parts []string) string {
	if len(parts) == 1 {
		return parts[0]
	}
	return "(" + strings.Join(parts, " "+op+" ") + ")"
}

// Words a heading's text is searched for, case ignored unless the word has
// a capital (org's case-fold-search, as org-ql uses it).
func qlTextRe(word string, literal bool) string {
	re := word
	if literal {
		re = regexp.QuoteMeta(word)
	}
	if strings.ToLower(word) == word {
		re = "(?i)" + re
	}
	return re
}

func (t qlTranslator) expr(n qlNode) (string, error) {
	if !n.IsList {
		if n.Str { // a bare string is org-ql's default predicate
			return qlCall("MatchContent", qlTextRe(n.Atom, true)), nil
		}
		return "", fmt.Errorf("%q is not a predicate: write (%s)", n.Atom, n.Atom)
	}
	if len(n.List) == 0 || n.List[0].IsList || n.List[0].Str {
		return "", fmt.Errorf("%s does not start with a predicate", n)
	}
	pred := strings.ToLower(n.List[0].Atom)
	args, kw := qlArgs(n.List[1:])
	strs := func() []string {
		out := []string{}
		for _, a := range args {
			if !a.IsList {
				out = append(out, a.Atom)
			}
		}
		return out
	}
	sub := func(op string) (string, error) {
		parts := []string{}
		for _, a := range args {
			s, err := t.expr(a)
			if err != nil {
				return "", err
			}
			parts = append(parts, s)
		}
		if len(parts) == 0 {
			return "", fmt.Errorf("(%s) needs something inside it", pred)
		}
		return qlJoin(op, parts), nil
	}
	anyOf := func(f string, vals []string) string {
		parts := []string{}
		for _, v := range vals {
			parts = append(parts, qlCall(f, v))
		}
		return qlJoin("||", parts)
	}
	allOf := func(f string, vals []string) string {
		parts := []string{}
		for _, v := range vals {
			parts = append(parts, qlCall(f, v))
		}
		return qlJoin("&&", parts)
	}

	switch pred {
	case "and":
		return sub("&&")
	case "or":
		return sub("||")
	case "not":
		s, err := sub("&&")
		if err != nil {
			return "", err
		}
		return "!" + qlParen(s), nil
	case "todo":
		if v := strs(); len(v) > 0 {
			return anyOf("IsStatus", v), nil
		}
		return "IsTodo()", nil
	case "done":
		return "IsDone()", nil
	case "tags", "tags-inherited", "tags-i", "itags", "tags-local", "tags-l", "ltags":
		if v := strs(); len(v) > 0 {
			return anyOf("HasTags", v), nil
		}
		return "!NoTags()", nil
	case "tags-all", "tags&":
		v := strs()
		if len(v) == 0 {
			return "", fmt.Errorf("(%s) needs the tags", pred)
		}
		return allOf("HasTags", v), nil
	case "priority":
		return t.priority(strs())
	case "heading", "h", "heading-regexp", "h*":
		v := strs()
		if len(v) == 0 {
			return "", fmt.Errorf("(%s) needs words to find", pred)
		}
		parts := []string{}
		for _, w := range v {
			parts = append(parts, qlCall("MatchHeadline", qlTextRe(w, pred == "heading" || pred == "h")))
		}
		return qlJoin("&&", parts), nil
	case "regexp", "r", "rifle", "smart":
		v := strs()
		if len(v) == 0 {
			return "", fmt.Errorf("(%s) needs words to find", pred)
		}
		parts := []string{}
		for _, w := range v {
			parts = append(parts, qlCall("MatchContent", qlTextRe(w, pred != "regexp" && pred != "r")))
		}
		return qlJoin("&&", parts), nil
	case "property", "prop":
		v := strs()
		inherit := kw["inherit"] != "" && kw["inherit"] != "nil"
		switch {
		case len(v) == 0:
			return "", fmt.Errorf("(%s) needs a property's name", pred)
		case len(v) == 1 && inherit:
			return qlCall("InheritedProperty", v[0]), nil
		case len(v) == 1:
			return qlCall("HasProperty", v[0]), nil
		case inherit:
			return qlCall("InheritedProperty", v[0], v[1]), nil
		}
		return qlCall("MatchProperty", v[0], "^"+regexp.QuoteMeta(v[1])+"$"), nil
	case "level":
		return t.level(strs())
	case "habit":
		return "IsHabit()", nil
	case "blocked":
		return "IsBlocked()", nil
	case "category":
		v := strs()
		if len(v) == 0 {
			return qlCall("PropCompare", "CATEGORY", "!=", ""), nil
		}
		parts := []string{}
		for _, c := range v {
			parts = append(parts, qlCall("PropCompare", "CATEGORY", "=", c))
		}
		return qlJoin("||", parts), nil
	case "path":
		v := strs()
		if len(v) == 0 {
			return "", fmt.Errorf("(path) needs part of a file's path")
		}
		return anyOf("InFile", v), nil
	case "link":
		if v := strs(); len(v) > 0 {
			return anyOf("LinksTo", v), nil
		}
		return "HasLinks()", nil
	case "effort":
		return t.effort(strs())
	case "scheduled", "deadline", "closed":
		return t.planning(strings.ToUpper(pred), strs(), kw)
	case "planning":
		parts := []string{}
		for _, p := range []string{"SCHEDULED", "DEADLINE", "CLOSED"} {
			s, err := t.planning(p, nil, kw)
			if err != nil {
				return "", err
			}
			parts = append(parts, s)
		}
		return qlJoin("||", parts), nil
	case "ts", "ts-active", "ts-a", "ts-inactive", "ts-i":
		return t.ts(kw)
	case "clocked":
		if len(kw) > 0 || len(args) > 0 {
			return "", fmt.Errorf("(clocked) with a range is not something orgs can ask yet; (clocked) alone is")
		}
		return "HasClock()", nil
	}
	return "", fmt.Errorf("org-ql's (%s) is not something orgs can answer", pred)
}

func qlParen(s string) string {
	if strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")") {
		return s
	}
	return "(" + s + ")"
}

// Plain arguments and :keyword values apart. A keyword with no value (the
// end of the list, or another keyword next) reads as t.
func qlArgs(list []qlNode) ([]qlNode, map[string]string) {
	args := []qlNode{}
	kw := map[string]string{}
	for i := 0; i < len(list); i++ {
		n := list[i]
		if !n.IsList && !n.Str && strings.HasPrefix(n.Atom, ":") {
			k := strings.ToLower(n.Atom[1:])
			if i+1 < len(list) && !(list[i+1].IsList || (!list[i+1].Str && strings.HasPrefix(list[i+1].Atom, ":"))) {
				kw[k] = list[i+1].Atom
				i++
			} else {
				kw[k] = "t"
			}
			continue
		}
		args = append(args, n)
	}
	return args, kw
}

var qlCompare = map[string]string{"<": "<", "<=": "<=", ">": ">", ">=": ">=", "=": "="}

// (priority), (priority "A" "B"), (priority >= "B"). A is the highest, so
// >= B is A or B: the letters compare the other way round.
func (t qlTranslator) priority(v []string) (string, error) {
	if len(v) == 0 {
		return qlCall("PropCompare", "PRIORITY", "!=", ""), nil
	}
	if op, ok := qlCompare[v[0]]; ok {
		if len(v) != 2 {
			return "", fmt.Errorf(`(priority %s) needs one priority after it: (priority %s "B")`, v[0], v[0])
		}
		flip := map[string]string{"<": ">", "<=": ">=", ">": "<", ">=": "<=", "=": "="}[op]
		return qlCall("PropCompare", "PRIORITY", flip, strings.ToUpper(v[1])), nil
	}
	parts := []string{}
	for _, p := range v {
		parts = append(parts, qlCall("IsPriority", strings.ToUpper(p)))
	}
	return qlJoin("||", parts), nil
}

// (level 2), (level < 3), (level 2 4) for 2 to 4.
func (t qlTranslator) level(v []string) (string, error) {
	num := func(s string) error {
		if _, err := strconv.Atoi(s); err != nil {
			return fmt.Errorf("(level) takes numbers, not %q", s)
		}
		return nil
	}
	switch {
	case len(v) == 1:
		if err := num(v[0]); err != nil {
			return "", err
		}
		return qlCall("PropCompare", "LEVEL", "=", v[0]), nil
	case len(v) == 2 && qlCompare[v[0]] != "":
		if err := num(v[1]); err != nil {
			return "", err
		}
		return qlCall("PropCompare", "LEVEL", qlCompare[v[0]], v[1]), nil
	case len(v) == 2:
		if err := num(v[0]); err != nil {
			return "", err
		}
		if err := num(v[1]); err != nil {
			return "", err
		}
		return "(" + qlCall("PropCompare", "LEVEL", ">=", v[0]) + " && " + qlCall("PropCompare", "LEVEL", "<=", v[1]) + ")", nil
	}
	return "", fmt.Errorf("(level) takes a level, a comparison and a level, or two levels")
}

// (effort "1:00") at most, (effort "0:30" "2:00") a span, (effort < "1:00").
func (t qlTranslator) effort(v []string) (string, error) {
	switch {
	case len(v) == 0:
		return qlCall("HasProperty", "EFFORT"), nil
	case len(v) == 1:
		return qlCall("PropCompare", "EFFORT", "<=", v[0]), nil
	case len(v) == 2 && qlCompare[v[0]] != "":
		return qlCall("PropCompare", "EFFORT", qlCompare[v[0]], v[1]), nil
	case len(v) == 2:
		return "(" + qlCall("PropCompare", "EFFORT", ">=", v[0]) + " && " + qlCall("PropCompare", "EFFORT", "<=", v[1]) + ")", nil
	}
	return "", fmt.Errorf("(effort) takes at most two durations")
}

// A date argument as a timestamp PropCompare reads: today, a number of days
// from now (-7, 30), or a date written out.
func (t qlTranslator) stamp(v string) (string, error) {
	v = strings.Trim(strings.TrimSpace(v), `"`)
	switch strings.ToLower(v) {
	case "today":
		return "<today>", nil
	case "yesterday":
		return "<-1d>", nil
	case "tomorrow":
		return "<+1d>", nil
	}
	if n, err := strconv.Atoi(v); err == nil {
		if n >= 0 {
			return fmt.Sprintf("<+%dd>", n), nil
		}
		return fmt.Sprintf("<%dd>", n), nil
	}
	if d, err := qlDay(v); err == nil {
		return "<" + d.Format("2006-01-02") + ">", nil
	}
	return "", fmt.Errorf("%q is not a day org-ql takes (today, a number of days, or 2026-11-05)", v)
}

func qlDay(v string) (time.Time, error) {
	for _, f := range []string{"2006-01-02", "2006-01-02 15:04", "2006-01-02T15:04"} {
		if d, err := time.ParseInLocation(f, v, time.Local); err == nil {
			return d, nil
		}
	}
	return time.Time{}, fmt.Errorf("not a date")
}

// (deadline), (deadline :to today), (scheduled :from -7 :to 7),
// (closed :on today), (deadline auto) - inside its warning period or past.
func (t qlTranslator) planning(which string, v []string, kw map[string]string) (string, error) {
	if len(v) == 1 && strings.EqualFold(v[0], "auto") && which == "DEADLINE" {
		return "(DeadlinePast() || DeadlineWithin(14))", nil
	}
	parts := []string{}
	add := func(key, op string) error {
		val, ok := kw[key]
		if !ok {
			return nil
		}
		s, err := t.stamp(val)
		if err != nil {
			return err
		}
		parts = append(parts, qlCall("PropCompare", which, op, s))
		return nil
	}
	if err := add("on", "="); err != nil {
		return "", err
	}
	if err := add("from", ">="); err != nil {
		return "", err
	}
	if err := add("to", "<="); err != nil {
		return "", err
	}
	if len(parts) == 0 {
		switch which {
		case "SCHEDULED":
			return "HasScheduled()", nil
		case "DEADLINE":
			return "HasDeadline()", nil
		}
		// Every CLOSED stamp is after this; a heading with none never is.
		return qlCall("PropCompare", "CLOSED", ">", "<1900-01-01>"), nil
	}
	return qlJoin("&&", parts), nil
}

// (ts :from -7 :to today), (ts :on today): any date of the heading's own -
// a timestamp, SCHEDULED or DEADLINE - in the span (DateBefore/DateAfter,
// which count all three, so this is a little wider than org-ql's, which
// leaves the planning line out).
func (t qlTranslator) ts(kw map[string]string) (string, error) {
	day := func(v string) (string, error) {
		s, err := t.stamp(v)
		if err != nil {
			return "", err
		}
		s = strings.Trim(s, "<>")
		d := time.Date(t.now.Year(), t.now.Month(), t.now.Day(), 0, 0, 0, 0, time.Local)
		switch {
		case s == "today":
		case strings.HasSuffix(s, "d"):
			n, _ := strconv.Atoi(strings.TrimSuffix(s, "d"))
			d = d.AddDate(0, 0, n)
		default:
			d, _ = qlDay(s)
		}
		return d.Format("2006 02 01"), nil // year, day, month: what OnDate takes
	}
	parts := []string{"HasAnyDate()"}
	if v, ok := kw["on"]; ok {
		d, err := day(v)
		if err != nil {
			return "", err
		}
		parts = append(parts, qlCall("OnDate", d))
	}
	if v, ok := kw["from"]; ok {
		d, err := day(v)
		if err != nil {
			return "", err
		}
		parts = append(parts, "!"+qlCall("DateBefore", d))
	}
	if v, ok := kw["to"]; ok {
		d, err := day(v)
		if err != nil {
			return "", err
		}
		parts = append(parts, "!"+qlCall("DateAfter", d))
	}
	return qlJoin("&&", parts), nil
}

// ---------------------------------------------------------------------------
// The block's other parameters
// ---------------------------------------------------------------------------

// OrgQlColumn is one of an org-ql block's :columns: heading, todo, priority,
// deadline, scheduled, closed, tags, or a property's name, titled by
// (name "Title") or else by the name capitalised.
type OrgQlColumn struct {
	Name  string
	Title string
}

// OrgQlColumns reads `(todo (priority "P") heading deadline)`; one word
// without brackets is one column.
func OrgQlColumns(s string) ([]OrgQlColumn, error) {
	list, err := qlList(s)
	if err != nil {
		return nil, fmt.Errorf(":columns: %v", err)
	}
	out := []OrgQlColumn{}
	for _, n := range list {
		c := OrgQlColumn{}
		switch {
		case !n.IsList:
			c.Name = n.Atom
		case len(n.List) >= 1 && !n.List[0].IsList:
			c.Name = n.List[0].Atom
			if len(n.List) >= 2 && !n.List[1].IsList {
				c.Title = n.List[1].Atom
			}
		default:
			return nil, fmt.Errorf(":columns: %s is not a column", n)
		}
		if c.Title == "" {
			c.Title = strings.ToUpper(c.Name[:1]) + c.Name[1:]
		}
		out = append(out, c)
	}
	return out, nil
}

// OrgQlWords reads a list of words, `(deadline priority)` or `date`.
func OrgQlWords(s string) ([]string, error) {
	list, err := qlList(s)
	if err != nil {
		return nil, err
	}
	out := []string{}
	for _, n := range list {
		if n.IsList {
			return nil, fmt.Errorf("%s is not a word", n)
		}
		out = append(out, strings.ToLower(n.Atom))
	}
	return out, nil
}

func qlList(s string) ([]qlNode, error) {
	s = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s), "'"))
	if s == "" {
		return nil, nil
	}
	if !strings.HasPrefix(s, "(") {
		s = "(" + s + ")"
	}
	r := &qlReader{s: s}
	n, err := r.read()
	if err != nil {
		return nil, err
	}
	return n.List, nil
}
