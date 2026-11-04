package orgs

/* SDOC: Querying::Short Queries
  A second way to write a query, for typing into a search box. It is turned
  into the full expression on the server, so it can ask everything the full
  form can ask through the functions above, and the two can be used anywhere
  a query is taken.

  #+BEGIN_SRC sh
  orgs search 'NEXT :work:'
  orgs search 'todo due:<3d -:home:'
  orgs search 'project stuck | WAITING old:>14'
  #+END_SRC

  - Words side by side must all match; =|= (or =or=) means either.
  - =-= (or =not=) in front of a term turns it round; brackets group.
  - Archived headings are left out unless the query says =archived=.
  - A plain word, ="quoted phrase"= or =/regex/= searches headlines. Words and
    phrases match anywhere and ignore case; a regex is used as written.

  | Term                          | Means                                       |
  |-------------------------------+---------------------------------------------|
  | =todo= =done= =task= =project= | IsTodo IsDone IsTask IsProject             |
  | =next= =stuck= =habit= =missed= | IsNextTask IsBlockedProject IsHabit MissedHabit |
  | =archived= =record= =collection= | IsArchived IsRecord IsCollection         |
  | =tagged= =untagged=           | has tags / NoTags                           |
  | =scheduled= =deadline= =dated= | HasScheduled HasDeadline HasAnyDate        |
  | =clocked= =linked= =broken= =orphan= | HasClock HasLinks HasBrokenLinks, no backlinks |
  | =checklist= =table= =drawer= =block= | HasChecklist HasTable HasDrawer HasBlock |
  | =NEXT= (any word in capitals) | IsStatus("NEXT")                            |
  | =:work:= =:work:urgent:=      | HasTags("work") - every tag named           |
  | =:/^M[0-9]+$/:=               | HasTagMatching                              |
  | =#A=                          | IsPriority("A")                             |
  | =due:past= =due:today= =due:<3d= =due:fri= | DeadlinePast, DeadlineWithin(0/3/days to friday) |
  | =due:any= =due:none=          | HasDeadline, !HasDeadline                   |
  | =sched:...=                   | the same for SCHEDULED                      |
  | =on:today= =on:fri= =on:2026-10-30= | OnDate                                |
  | =before:today= =after:fri= =before:-2w= | a SCHEDULED, DEADLINE or timestamp of its own before / after that day (DateBefore DateAfter) |
  | =old:>30= =old:<7=            | OlderThan(30), DaysOld under 7              |
  | =streak:>7= =backlinks:0= =left:>0= =items:>5= =age:<3= | HabitStreak BacklinkCount ChecklistLeft ChecklistCount DaysOld, compared |
  | =file:work= =in:contact= =record:contact= | InFile InCollection IsRecord     |
  | =group:WORK= =text:word= =title:word= =link:word= =tag:work= | InTagGroup MatchContent MatchHeadline LinksTo HasTags |
  | =status:next= =priority:a=    | IsStatus("NEXT") IsPriority("A")            |
  | =client:acme=                 | any other key is a property: MatchProperty("CLIENT", ...) |
  | =client:*=                    | HasProperty("CLIENT")                       |
  | =effort:<2h= =level:>=2= =scheduled:<<+1w>= | PropCompare("EFFORT", "<", "2h"): =<= =<== =>= =>== ===, =!== |
  | ={{ WorkTasks }}=             | a yaml filter, as in the full form          |

  Org's own match strings work too, as an agenda's tags search writes them:
  =+work-boss+PRIORITY="A"|urgent=, =work/NEXT|WAITING=, =+project/!=,
  =Effort<2:00=, =CATEGORY={^cli}=, =SCHEDULED<="<+1w>"/-DONE=. One is told
  from a short query by having no spaces and starting with =+= or =-= or using
  one of === =<= =>= =/= ={= =&=.

  A query is read as the full form when it calls a function (=IsTodo()=) or
  uses =&&= or =||=, so every query written before keeps its meaning.
  =orgs search -explain= prints the full form a short query becomes.
EDOC */

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/ihdavids/orgs/internal/common"
	"github.com/ihdavids/orgs/internal/orgdate"
)

// The short query syntax: `todo :work: due:<3d` rather than
// `!IsArchived() && IsTodo() && HasTags("work") && DeadlineWithin(3)`.
//
// It is a translation into the full (govaluate) form and nothing more, so every
// query function, the bad-call guard and the function list the assistant is
// told are shared, and the translation always contains a function call - which
// is what makes it idempotent: translating twice is translating once.

// Keyword terms: a lowercase word that is a question about the heading.
var shortKeywords = map[string]string{
	"todo":       "IsTodo()",
	"done":       "IsDone()",
	"task":       "IsTask()",
	"project":    "IsProject()",
	"next":       "IsNextTask()",
	"stuck":      "IsBlockedProject()",
	"habit":      "IsHabit()",
	"missed":     "MissedHabit()",
	"archived":   "IsArchived()",
	"record":     "IsRecord()",
	"collection": "IsCollection()",
	// HasTags() with no tags named is always true, so "has any tag" is NoTags turned round.
	"tagged":    "!NoTags()",
	"untagged":  "NoTags()",
	"scheduled": "HasScheduled()",
	"deadline":  "HasDeadline()",
	"dated":     "HasAnyDate()",
	"clocked":   "HasClock()",
	"linked":    "HasLinks()",
	"broken":    "HasBrokenLinks()",
	"orphan":    "BacklinkCount() == 0",
	"checklist": "HasChecklist()",
	"table":     "HasTable()",
	"drawer":    "HasDrawer()",
	"block":     "HasBlock()",
}

// Keys whose value is text handed to one function.
var shortTextKeys = map[string]string{
	"file":   "InFile",
	"in":     "InCollection",
	"group":  "InTagGroup",
	"text":   "MatchContent",
	"title":  "MatchHeadline",
	"link":   "LinksTo",
	"record": "IsRecord",
}

// Keys whose value is a number to compare.
var shortNumberKeys = map[string]string{
	"streak":    "HabitStreak()",
	"backlinks": "BacklinkCount()",
	"left":      "ChecklistLeft()",
	"items":     "ChecklistCount()",
	"age":       "DaysOld()",
}

var (
	// A function call or a full-form operator: the query is the full form.
	fullFormCall = regexp.MustCompile(`[A-Z][A-Za-z0-9_]*\s*\(`)
	filterRef    = regexp.MustCompile(`\{\{.*?\}\}`)
	statusWord   = regexp.MustCompile(`^[A-Z][A-Z0-9_-]+$`)
	priorityWord = regexp.MustCompile(`^#[A-Za-z0-9]$`)
	compareValue = regexp.MustCompile(`^(<=|>=|<|>|=)?(\d+(?:\.\d+)?)$`)
	daysValue    = regexp.MustCompile(`^<?=?(\d+)([dw]?)$`)
)

// isShortQuery is false for anything written in the full form - a function
// call, && or ||, or nothing but {{ filters }} - so a query from before the
// short form existed means what it always meant.
func isShortQuery(q string) bool {
	// Quoted text may hold anything, a bracket included.
	var b strings.Builder
	in := false
	for _, r := range q {
		if r == '"' {
			in = !in
			continue
		}
		if !in {
			b.WriteRune(r)
		}
	}
	bare := b.String()
	if fullFormCall.MatchString(bare) || strings.Contains(bare, "&&") || strings.Contains(bare, "||") {
		return false
	}
	rest := strings.TrimSpace(filterRef.ReplaceAllString(q, ""))
	return rest != "" && rest != "true" && rest != "false"
}

// shortToFull turns a short query into the full form; anything else comes back
// as it was. Errors name the column, for a search box to point at.
func shortToFull(q string) (string, error) {
	if !isShortQuery(q) {
		return q, nil
	}
	return translateShort(q, time.Now())
}

func translateShort(q string, now time.Time) (string, error) {
	// An org match string (orgmatch.go) before a short query.
	if full, ok := orgMatchToFull(q); ok {
		return full, nil
	}
	toks, err := lexShort(q)
	if err != nil {
		return "", err
	}
	p := &shortParser{toks: toks, now: now}
	n, err := p.or()
	if err != nil {
		return "", err
	}
	if p.pos < len(p.toks) {
		t := p.toks[p.pos]
		return "", fmt.Errorf("col %d: unexpected %q", t.col, t.text)
	}
	if !p.sawArchived {
		n = &shortNode{kind: '&', kids: []*shortNode{{leaf: "!IsArchived()"}, n}}
	}
	return n.render(0), nil
}

/* SDOC: API
* GET /query/explain?query=Q

  What a query becomes before it runs: =Full= is a short query translated into
  the full form, =Expanded= that with its {{ filters }} written in. =Ok= is
  false, with =Msg=, when it would not parse. What =orgs search -explain= shows.
EDOC */
func RequestQueryExplain(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("query")
	res := explainQuery(q)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

func explainQuery(q string) common.QueryExplain {
	res := common.QueryExplain{Query: q, Short: isShortQuery(q)}
	full, err := shortToFull(q)
	if err != nil {
		res.Msg = err.Error()
		return res
	}
	res.Full = full
	res.Expanded = expandQueryFilters(q)
	if _, err := ParseString(&common.StringQuery{Query: res.Expanded}); err != nil {
		res.Msg = err.Error()
		return res
	}
	res.Ok = true
	return res
}

// ---------------------------------------------------------------------------
// Tokens
// ---------------------------------------------------------------------------

type shortTok struct {
	kind byte // 'w' term, '(' ')' '|' '&' '!'
	text string
	col  int
}

func lexShort(q string) ([]shortTok, error) {
	rs := []rune(q)
	var toks []shortTok
	i := 0
	for i < len(rs) {
		r := rs[i]
		col := i + 1
		switch {
		case unicode.IsSpace(r):
			i++
		case r == '(' || r == ')':
			toks = append(toks, shortTok{kind: byte(r), text: string(r), col: col})
			i++
		case r == '|':
			toks = append(toks, shortTok{kind: '|', text: "|", col: col})
			i++
		case r == '&':
			toks = append(toks, shortTok{kind: '&', text: "&", col: col})
			i++
		case (r == '-' || r == '!') && i+1 < len(rs) && !unicode.IsSpace(rs[i+1]):
			toks = append(toks, shortTok{kind: '!', text: string(r), col: col})
			i++
		case r == '{' && i+1 < len(rs) && rs[i+1] == '{':
			end := strings.Index(string(rs[i:]), "}}")
			if end < 0 {
				return nil, fmt.Errorf("col %d: {{ with no }}", col)
			}
			n := len([]rune(string(rs[i:])[:end+2]))
			toks = append(toks, shortTok{kind: 'w', text: string(rs[i : i+n]), col: col})
			i += n
		default:
			// A term runs to a space, a bracket or a bar - except inside quotes
			// or a /regex/, which may hold any of them.
			start := i
			for i < len(rs) {
				c := rs[i]
				if c == '"' || (c == '/' && (i == start || rs[i-1] == ':')) {
					close := c
					j := i + 1
					for j < len(rs) && rs[j] != close {
						if rs[j] == '\\' {
							j++
						}
						j++
					}
					if j >= len(rs) {
						return nil, fmt.Errorf("col %d: %c with no closing %c", i+1, c, c)
					}
					i = j + 1
					continue
				}
				if unicode.IsSpace(c) || c == '(' || c == ')' || c == '|' {
					break
				}
				i++
			}
			toks = append(toks, shortTok{kind: 'w', text: string(rs[start:i]), col: col})
		}
	}
	// Word operators, so a query reads aloud.
	for k := range toks {
		if toks[k].kind != 'w' {
			continue
		}
		switch toks[k].text {
		case "or", "OR":
			toks[k].kind = '|'
		case "and", "AND":
			toks[k].kind = '&'
		case "not", "NOT":
			toks[k].kind = '!'
		}
	}
	return toks, nil
}

// ---------------------------------------------------------------------------
// Parse
// ---------------------------------------------------------------------------

type shortNode struct {
	kind byte // 0 leaf, '&', '|', '!'
	leaf string
	kids []*shortNode
}

// render writes the node with only the brackets precedence needs: an or
// inside an and, anything compound under a not.
func (n *shortNode) render(parent byte) string {
	switch n.kind {
	case '!':
		k := n.kids[0]
		if k.kind == 0 && !compoundLeaf(k.leaf) {
			if strings.HasPrefix(k.leaf, "!") {
				return k.leaf[1:]
			}
			return "!" + k.leaf
		}
		if k.kind == 0 {
			return "!(" + k.leaf + ")"
		}
		return "!(" + k.render(0) + ")"
	case '&', '|':
		parts := make([]string, len(n.kids))
		for i, k := range n.kids {
			parts[i] = k.render(n.kind)
		}
		op := " && "
		if n.kind == '|' {
			op = " || "
		}
		s := strings.Join(parts, op)
		if parent == '&' && n.kind == '|' {
			return "(" + s + ")"
		}
		return s
	}
	// A leaf that is itself an expression (a comparison) is bracketed
	// wherever it is not alone.
	if parent != 0 && compoundLeaf(n.leaf) {
		return "(" + n.leaf + ")"
	}
	return n.leaf
}

// compoundLeaf is true for a leaf with an operator in it outside its string
// literals: `HabitStreak() > 7`, but not `MatchHeadline("a > b")`.
func compoundLeaf(leaf string) bool {
	if strings.HasPrefix(leaf, "({{") {
		return false // already bracketed
	}
	in := false
	for i := 0; i < len(leaf); i++ {
		switch c := leaf[i]; {
		case in && c == '\\':
			i++
		case c == '"':
			in = !in
		case !in && strings.IndexByte("=<>&|", c) >= 0:
			return true
		}
	}
	return false
}

type shortParser struct {
	toks        []shortTok
	pos         int
	now         time.Time
	sawArchived bool
}

func (p *shortParser) peek() *shortTok {
	if p.pos < len(p.toks) {
		return &p.toks[p.pos]
	}
	return nil
}

func (p *shortParser) or() (*shortNode, error) {
	first, err := p.and()
	if err != nil {
		return nil, err
	}
	kids := []*shortNode{first}
	for t := p.peek(); t != nil && t.kind == '|'; t = p.peek() {
		p.pos++
		n, err := p.and()
		if err != nil {
			return nil, err
		}
		kids = append(kids, n)
	}
	if len(kids) == 1 {
		return first, nil
	}
	return &shortNode{kind: '|', kids: kids}, nil
}

func (p *shortParser) and() (*shortNode, error) {
	var kids []*shortNode
	for t := p.peek(); t != nil && t.kind != '|' && t.kind != ')'; t = p.peek() {
		if t.kind == '&' {
			p.pos++
			continue
		}
		n, err := p.unary()
		if err != nil {
			return nil, err
		}
		kids = append(kids, n)
	}
	if len(kids) == 0 {
		if t := p.peek(); t != nil {
			return nil, fmt.Errorf("col %d: nothing before %q", t.col, t.text)
		}
		return nil, fmt.Errorf("col %d: the query ends where a term was wanted", p.endCol())
	}
	if len(kids) == 1 {
		return kids[0], nil
	}
	return &shortNode{kind: '&', kids: kids}, nil
}

func (p *shortParser) endCol() int {
	if len(p.toks) == 0 {
		return 1
	}
	t := p.toks[len(p.toks)-1]
	return t.col + len([]rune(t.text))
}

func (p *shortParser) unary() (*shortNode, error) {
	t := p.peek()
	if t == nil {
		return nil, fmt.Errorf("col %d: the query ends where a term was wanted", p.endCol())
	}
	switch t.kind {
	case '!':
		p.pos++
		n, err := p.unary()
		if err != nil {
			return nil, err
		}
		if n.kind == '!' {
			return n.kids[0], nil // --todo is todo
		}
		return &shortNode{kind: '!', kids: []*shortNode{n}}, nil
	case '(':
		p.pos++
		n, err := p.or()
		if err != nil {
			return nil, err
		}
		if c := p.peek(); c == nil || c.kind != ')' {
			return nil, fmt.Errorf("col %d: ( with no )", t.col)
		}
		p.pos++
		return n, nil
	case ')':
		return nil, fmt.Errorf("col %d: ) with no (", t.col)
	}
	p.pos++
	leaf, err := p.term(t.text)
	if err != nil {
		return nil, fmt.Errorf("col %d: %v", t.col, err)
	}
	return &shortNode{leaf: leaf}, nil
}

// ---------------------------------------------------------------------------
// Terms
// ---------------------------------------------------------------------------

func (p *shortParser) term(w string) (string, error) {
	switch {
	case strings.HasPrefix(w, "{{"):
		// A yaml filter, expanded after translation. Bracketed so its own
		// || cannot leak into the and around it.
		return "(" + w + ")", nil
	case strings.HasPrefix(w, ":"):
		return tagTerm(w)
	case priorityWord.MatchString(w):
		return qcall("IsPriority", strings.ToUpper(w[1:])), nil
	case strings.HasPrefix(w, `"`) || strings.HasPrefix(w, "/"):
		pat, err := textPattern(w)
		if err != nil {
			return "", err
		}
		return qcall("MatchHeadline", pat), nil
	}
	if k, v, ok := strings.Cut(w, ":"); ok && k != "" {
		return p.keyTerm(strings.ToLower(k), v)
	}
	if f, ok := shortKeywords[w]; ok {
		if w == "archived" {
			p.sawArchived = true
		}
		return f, nil
	}
	if statusWord.MatchString(w) {
		return qcall("IsStatus", w), nil
	}
	return qcall("MatchHeadline", "(?i)"+regexp.QuoteMeta(w)), nil
}

// :work: :work:urgent: :/^M[0-9]+$/:
func tagTerm(w string) (string, error) {
	body := strings.TrimPrefix(w, ":")
	if strings.HasPrefix(body, "/") {
		end := strings.LastIndex(body, "/")
		if end <= 0 || strings.Trim(body[end+1:], ":") != "" {
			return "", fmt.Errorf("a tag pattern is written :/regex/:")
		}
		if _, err := regexp.Compile(body[1:end]); err != nil {
			return "", err
		}
		return qcall("HasTagMatching", body[1:end]), nil
	}
	var tags []string
	for _, t := range strings.Split(body, ":") {
		if t != "" {
			tags = append(tags, t)
		}
	}
	if len(tags) == 0 {
		return "", fmt.Errorf("%q names no tag", w)
	}
	return qcall("HasTags", tags...), nil
}

// textPattern reads a value meant as text to look for: a word or a "quoted
// phrase" matches anywhere and ignores case, a /regex/ is used as written.
func textPattern(v string) (string, error) {
	if len(v) >= 2 && v[0] == '/' && v[len(v)-1] == '/' {
		re := v[1 : len(v)-1]
		if _, err := regexp.Compile(re); err != nil {
			return "", err
		}
		return re, nil
	}
	if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
		v = strings.ReplaceAll(v[1:len(v)-1], `\"`, `"`)
	}
	if v == "" {
		return "", fmt.Errorf("nothing to look for")
	}
	return "(?i)" + regexp.QuoteMeta(v), nil
}

func (p *shortParser) keyTerm(k, v string) (string, error) {
	switch k {
	case "due":
		return dateKey("Deadline", v, p.now)
	case "sched", "scheduled":
		return dateKey("Scheduled", v, p.now)
	case "on", "before", "after":
		d, ok, err := orgdate.ParseDate(v, p.now)
		if err != nil || !ok || d.Day.IsZero() {
			return "", fmt.Errorf("%s: wants a day - today, fri, +2d, -1w, 2026-10-30", k)
		}
		f := map[string]string{"on": "OnDate", "before": "DateBefore", "after": "DateAfter"}[k]
		return qcall(f, d.Day.Format("2006 02 01")), nil
	case "old":
		m := compareValue.FindStringSubmatch(v)
		if m == nil {
			return "", fmt.Errorf("old: wants a number of days - old:>30, old:<7")
		}
		switch m[1] {
		case "", ">=":
			return "OlderThan(" + m[2] + ")", nil
		case ">":
			n, _ := strconv.Atoi(m[2])
			return "OlderThan(" + strconv.Itoa(n+1) + ")", nil
		}
		return compare("DaysOld()", m[1], m[2]) + " && DaysOld() >= 0", nil
	case "status":
		if v == "" {
			return "HasAStatus()", nil
		}
		return qcall("IsStatus", strings.ToUpper(v)), nil
	case "priority", "p":
		if v == "" {
			return "", fmt.Errorf("priority: wants a letter")
		}
		return qcall("IsPriority", strings.ToUpper(v)), nil
	case "tag":
		return tagTerm(":" + v + ":")
	}
	if f, ok := shortNumberKeys[k]; ok {
		m := compareValue.FindStringSubmatch(v)
		if m == nil {
			return "", fmt.Errorf("%s: wants a number - %s:3, %s:>3", k, k, k)
		}
		return compare(f, m[1], m[2]), nil
	}
	if f, ok := shortTextKeys[k]; ok {
		if f == "InTagGroup" || f == "InCollection" || f == "IsRecord" {
			// A name, not text to look for.
			if v == "" {
				return "", fmt.Errorf("%s: wants a name", k)
			}
			return qcall(f, strings.Trim(v, `"`)), nil
		}
		pat, err := textPattern(v)
		if err != nil {
			return "", fmt.Errorf("%s: %v", k, err)
		}
		return qcall(f, pat), nil
	}
	// Any other key is a property.
	prop := strings.ToUpper(k)
	if v == "" || v == "*" {
		return qcall("HasProperty", prop), nil
	}
	// effort:<2h, scheduled:<today>, level:>=2: a comparison (PropCompare).
	if m := propOpRe.FindStringSubmatch(v); m != nil {
		return qcall("PropCompare", prop, m[1], strings.Trim(m[2], `"`)), nil
	}
	pat, err := textPattern(v)
	if err != nil {
		return "", fmt.Errorf("%s: %v", k, err)
	}
	return qcall("MatchProperty", prop, pat), nil
}

// due: and sched:. past, today, any, none, a number of days (3, 3d, <3d, 2w)
// or a day (fri, eom, 2026-11-02) - "by then".
func dateKey(which, v string, now time.Time) (string, error) {
	switch strings.ToLower(v) {
	case "past", "over", "overdue":
		return which + "Past()", nil
	case "today":
		return which + "Within(0)", nil
	case "", "any", "*":
		return "Has" + which + "()", nil
	case "none":
		return "!Has" + which + "()", nil
	}
	if m := daysValue.FindStringSubmatch(v); m != nil {
		n, _ := strconv.Atoi(m[1])
		if m[2] == "w" {
			n *= 7
		}
		return which + "Within(" + strconv.Itoa(n) + ")", nil
	}
	d, ok, err := orgdate.ParseDate(strings.TrimPrefix(strings.TrimPrefix(v, "<="), "<"), now)
	if err != nil || !ok || d.Day.IsZero() {
		return "", fmt.Errorf("%s wants past, today, any, none, a number of days or a day", strings.ToLower(which[:1])+which[1:])
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	days := int(d.Day.Sub(today).Hours()/24 + 0.5)
	if days < 0 {
		return "", fmt.Errorf("%s is in the past - try %s:past", v, map[string]string{"Deadline": "due", "Scheduled": "sched"}[which])
	}
	return which + "Within(" + strconv.Itoa(days) + ")", nil
}

func compare(f, op, n string) string {
	switch op {
	case "", "=":
		op = "=="
	}
	return f + " " + op + " " + n
}

// qcall writes a function call with its arguments as govaluate strings: a
// backslash escapes anything there, so a regex's own are doubled.
var propOpRe = regexp.MustCompile(`^(<=|>=|<>|!=|<|>|=)(.+)$`)

func qcall(f string, args ...string) string {
	q := make([]string, len(args))
	for i, a := range args {
		a = strings.ReplaceAll(a, `\`, `\\`)
		a = strings.ReplaceAll(a, `"`, `\"`)
		q[i] = `"` + a + `"`
	}
	return f + "(" + strings.Join(q, ", ") + ")"
}
