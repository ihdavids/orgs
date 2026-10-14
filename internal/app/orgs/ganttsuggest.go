package orgs

// POST /gantt/suggest: what a chart built from a pile of tickets could be told.
//
// A query over imported tickets gives a chart with every bar starting today:
// nothing says what waits on what, half of it has no estimate, and the lanes
// are whatever SECTION happens to hold. The tickets usually say most of it in
// their own words - "blocked by ABC-12", "needs the login page first", a
// numbered list of steps - and the clock says how long work like it took last
// time. This reads that and hands back suggestions; it writes nothing. A client
// shows them, a person picks, and the picks go through /gantt/link and
// /property.
//
// The task set is exactly what /gantt/tasks answers for the same query
// (ganttTaskSet), so a suggestion never names a bar the chart did not draw.
//
// One rule shapes the dependency suggestions: AFTER holds one id (insights.go,
// resolve), so a task waits on exactly one thing. A task that already waits on
// something (an AFTER, or an ORDERED parent) gets no suggestion, because
// applying one would replace what is there; and each task gets at most one
// suggestion, the most certain, because applying two would leave only the
// second.
//
// The pure parts - reading phrases, matching keys and titles, numbered steps,
// the transitive reduction, rounding and lane scoring - take plain sgTask
// values and are pinned by ganttsuggest_test.go. Only buildGanttSuggest and
// gatherSuggestHistory read the database.

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/app/orgs/plugs/mermaid"
	"github.com/ihdavids/orgs/internal/common"
)

// ---------------------------------------------------------------------------
// The tasks, as the pure parts see them
// ---------------------------------------------------------------------------

type sgTask struct {
	Hash        string
	Title       string
	File        string
	Line        int
	Parent      string // hash
	ParentTitle string
	ID          string
	CustomID    string
	Tags        []string
	Props       map[string]string
	Assignee    string
	Open        bool
	HasEffort   bool
	// The heading's own body, children left out.
	Body []string
	// What it waits on now (a hash in the chart), and whether that wait is
	// written as AFTER (removable) rather than given by an ORDERED parent.
	After    string
	ViaAfter bool
	// With its AFTER cleared, what its ORDERED parent would chain it behind.
	ChainPrev string
}

type sgIndex struct {
	tasks   []*sgTask
	byHash  map[string]*sgTask
	byID    map[string]*sgTask // ID and CUSTOM_ID
	byTitle map[string][]*sgTask
	byKey   map[string]*sgTask
	keyJira map[string]bool
	words   map[*sgTask][]string
}

func newSgIndex(tasks []*sgTask) *sgIndex {
	idx := &sgIndex{
		tasks: tasks, byHash: map[string]*sgTask{}, byID: map[string]*sgTask{},
		byTitle: map[string][]*sgTask{}, byKey: map[string]*sgTask{}, keyJira: map[string]bool{},
		words: map[*sgTask][]string{},
	}
	for _, t := range tasks {
		idx.byHash[t.Hash] = t
		for _, id := range []string{t.ID, t.CustomID} {
			if id != "" {
				if _, taken := idx.byID[id]; !taken {
					idx.byID[id] = t
				}
			}
		}
		nt := sgNormTitle(t.Title)
		idx.byTitle[nt] = append(idx.byTitle[nt], t)
		if bare := sgNormTitle(stripOrdinal(t.Title)); bare != nt {
			idx.byTitle[bare] = append(idx.byTitle[bare], t)
		}
		keys, jira := sgTaskKeys(t)
		for _, k := range keys {
			if _, taken := idx.byKey[k]; !taken {
				idx.byKey[k] = t
				idx.keyJira[k] = jira[k]
			}
		}
		idx.words[t] = sgWords(stripOrdinal(t.Title))
	}
	return idx
}

// ---------------------------------------------------------------------------
// Words, titles and ticket keys
// ---------------------------------------------------------------------------

var sgStopWords = map[string]bool{
	"the": true, "a": true, "an": true, "to": true, "of": true, "for": true, "and": true, "or": true,
	"in": true, "on": true, "at": true, "is": true, "be": true, "it": true, "this": true, "that": true,
	"with": true, "we": true, "our": true, "us": true, "until": true, "once": true, "been": true, "has": true,
	"have": true, "being": true, "task": true, "ticket": true, "issue": true, "story": true, "done": true,
	"finished": true, "first": true, "get": true, "gets": true, "getting": true, "by": true, "from": true,
	"as": true, "are": true, "was": true, "will": true, "can": true, "its": true, "their": true, "all": true,
	"some": true, "any": true, "into": true, "up": true, "out": true, "then": true, "than": true,
	"todo": true, "next": true, "please": true, "also": true, "still": true, "should": true, "must": true,
}

var sgWordRe = regexp.MustCompile(`[\p{L}\p{N}]+`)

func sgStem(w string) string {
	switch {
	case len(w) > 5 && strings.HasSuffix(w, "ing"):
		return w[:len(w)-3]
	case len(w) > 4 && strings.HasSuffix(w, "ed"):
		return w[:len(w)-2]
	case len(w) > 4 && strings.HasSuffix(w, "es"):
		return w[:len(w)-2]
	case len(w) > 3 && strings.HasSuffix(w, "s") && !strings.HasSuffix(w, "ss"):
		return w[:len(w)-1]
	}
	return w
}

// The significant words of a piece of text, stemmed, in order, once each.
func sgWords(s string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, w := range sgWordRe.FindAllString(strings.ToLower(sgLinkRe.ReplaceAllString(s, " $2 ")), -1) {
		if sgStopWords[w] || len(w) < 2 {
			continue
		}
		w = sgStem(w)
		if !seen[w] {
			seen[w] = true
			out = append(out, w)
		}
	}
	return out
}

var sgCookieRe = regexp.MustCompile(`\[\d*%\]|\[\d*/\d*\]`)

// A title as a link or a phrase would spell it: lower case, no cookies, a
// link's description in place of the link, single spaces.
func sgNormTitle(s string) string {
	s = sgLinkRe.ReplaceAllStringFunc(s, func(m string) string {
		sm := sgLinkRe.FindStringSubmatch(m)
		if sm[2] != "" {
			return sm[2]
		}
		return sm[1]
	})
	s = sgCookieRe.ReplaceAllString(s, "")
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

// Letters and digits only, for "does the phrase contain the title".
func sgFlat(s string) string {
	return " " + strings.Join(sgWordRe.FindAllString(strings.ToLower(s), -1), " ") + " "
}

var sgKeyRe = regexp.MustCompile(`\b[A-Z][A-Z0-9]{1,9}-[0-9]{1,6}\b`)
var sgKeyWhole = regexp.MustCompile(`^[A-Z][A-Z0-9]{1,9}-[0-9]{1,6}$`)

// Ticket keys (ABC-123) in a piece of text, once each.
func ticketKeys(s string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, k := range sgKeyRe.FindAllString(s, -1) {
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	return out
}

// The properties a ticket's key may be kept in. The jira plugin writes it as
// CUSTOM_ID (templates/jiradefault.tpl) and a browse url in LINK.
var sgKeyProps = []string{"CUSTOM_ID", "ID", "KEY", "JIRA", "JIRA_KEY", "ISSUE", "ISSUEKEY", "ISSUE_KEY", "TICKET", "LINK"}

// The keys that name a task, and which of them came from a jira import (a
// :JIRA: tag, or the PROJECTKEY / ISSUETYPE the jira template writes).
func sgTaskKeys(t *sgTask) ([]string, map[string]bool) {
	jiraTask := false
	for _, tg := range t.Tags {
		if strings.EqualFold(tg, "jira") {
			jiraTask = true
		}
	}
	if sgProp(t.Props, "PROJECTKEY") != "" || sgProp(t.Props, "ISSUETYPE") != "" {
		jiraTask = true
	}
	keys := []string{}
	jira := map[string]bool{}
	seen := map[string]bool{}
	add := func(k string, fromJira bool) {
		if !seen[k] {
			seen[k] = true
			keys = append(keys, k)
			jira[k] = fromJira
		}
	}
	for _, p := range sgKeyProps {
		v := sgProp(t.Props, p)
		if v == "" {
			continue
		}
		if sgKeyWhole.MatchString(strings.TrimSpace(v)) {
			add(strings.TrimSpace(v), jiraTask)
		} else if p == "LINK" || p == "JIRA" || p == "KEY" || p == "TICKET" || p == "ISSUE" {
			for _, k := range ticketKeys(v) {
				add(k, jiraTask)
			}
		}
	}
	for _, k := range ticketKeys(t.Title) {
		add(k, false)
	}
	return keys, jira
}

func sgProp(props map[string]string, name string) string {
	if v, ok := props[name]; ok {
		return strings.TrimSpace(v)
	}
	for k, v := range props {
		if strings.EqualFold(k, name) {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// Phrases: "blocked by X", "blocks X"
// ---------------------------------------------------------------------------

type depPhrase struct {
	Verb     string
	Reverse  bool // X waits on this task
	Strength float64
	Links    []sgLink
	Keys     []string
	Text     string // the object, with links read as their descriptions, for title matching
	Evidence string
}

type sgLink struct {
	Target string
	Desc   string
}

var sgLinkRe = regexp.MustCompile(`\[\[([^\]]+)\](?:\[([^\]]*)\])?\]`)

var depVerbRe = regexp.MustCompile(`(?i)\b(?:(blocked\s+(?:by|on)|depends\s+(?:on|upon)|depend[ae]nt\s+(?:on|upon)|waiting\s+(?:on|for)|waits\s+(?:on|for)|prerequisites?\s+(?:for|of|to)|prereqs?\s+(?:for|of|to)|required\s+by|needed\s+by|requires|needs|follows|after|blocks|blocking|before)\b|(prerequisites?|prereqs?|depends|dependencies|blockers?|requires|required)\s*:)`)

// What a verb says, and how much it can be trusted. "after", "before" and
// "needs" are ordinary words as often as they are dependencies, so a title
// match behind them has to be better to count.
func sgVerb(v string) (reverse bool, strength float64) {
	v = strings.ToLower(strings.Join(strings.Fields(strings.TrimSuffix(strings.TrimSpace(v), ":")), " "))
	switch {
	case strings.HasPrefix(v, "prerequisite ") || strings.HasPrefix(v, "prereq "):
		return true, 1
	case v == "blocks" || v == "blocking":
		return true, 1
	case v == "required by":
		return true, 0.95
	case v == "needed by":
		return true, 0.9
	case v == "before":
		return true, 0.8
	case v == "requires" || v == "required":
		return false, 0.9
	case v == "needs":
		return false, 0.8
	case v == "after" || v == "follows":
		return false, 0.8
	}
	return false, 1
}

var sgClauseEndRe = regexp.MustCompile(`[.;!?](\s|$)`)
var sgObjectCutRe = regexp.MustCompile(`(?i),|\(|\s[-–—]\s|\b(but|because|so|which|once|when|until|while|since|as soon as|and then)\b`)

// extractDepPhrases reads one line of a heading's body for the phrases that
// name a dependency, each with what it names: links and ticket keys in the
// clause after the verb, and the clause itself for matching against titles.
func extractDepPhrases(line string) []depPhrase {
	links := sgLinkRe.FindAllStringSubmatchIndex(line, -1)
	inLink := func(i int) bool {
		for _, l := range links {
			if i >= l[0] && i < l[1] {
				return true
			}
		}
		return false
	}
	verbs := [][]int{}
	for _, m := range depVerbRe.FindAllStringIndex(line, -1) {
		if !inLink(m[0]) {
			verbs = append(verbs, m)
		}
	}
	out := []depPhrase{}
	for i, m := range verbs {
		end := len(line)
		if i+1 < len(verbs) {
			end = verbs[i+1][0]
		}
		// The clause stops at a full stop, a semicolon or the like - not one
		// inside a link, where file.org::*Heading has a dot of its own.
		for _, c := range sgClauseEndRe.FindAllStringIndex(line[m[1]:end], -1) {
			if !inLink(m[1] + c[0]) {
				end = m[1] + c[0]
				break
			}
		}
		clause := line[m[1]:end]
		reverse, strength := sgVerb(line[m[0]:m[1]])
		p := depPhrase{Verb: strings.ToLower(strings.TrimSpace(line[m[0]:m[1]])), Reverse: reverse, Strength: strength}
		for _, l := range links {
			if l[0] >= m[1] && l[1] <= end {
				target := line[l[2]:l[3]]
				desc := ""
				if l[4] >= 0 {
					desc = line[l[4]:l[5]]
				}
				p.Links = append(p.Links, sgLink{Target: target, Desc: desc})
			}
		}
		p.Keys = ticketKeys(clause)
		text := sgLinkRe.ReplaceAllStringFunc(clause, func(s string) string {
			sm := sgLinkRe.FindStringSubmatch(s)
			if sm[2] != "" {
				return sm[2]
			}
			return sm[1]
		})
		if c := sgObjectCutRe.FindStringIndex(text); c != nil {
			text = text[:c[0]]
		}
		p.Text = strings.TrimSpace(text)
		p.Evidence = sgEvidence(line, m[0])
		if len(p.Links) == 0 && len(p.Keys) == 0 && p.Text == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}

// About 120 characters of the line, around where the phrase starts.
func sgEvidence(line string, at int) string {
	line = strings.TrimSpace(line)
	r := []rune(line)
	if len(r) <= 120 {
		return line
	}
	// Rune position of the phrase in the trimmed line.
	lead := len(line) - len(strings.TrimLeft(line, " \t"))
	pos := len([]rune(line[:max(0, min(len(line), at-lead))]))
	start := pos - 30
	if start < 0 {
		start = 0
	}
	stop := start + 117
	if stop > len(r) {
		stop = len(r)
		start = max(0, stop-117)
	}
	s := string(r[start:stop])
	if start > 0 {
		s = "…" + s
	}
	if stop < len(r) {
		s += "…"
	}
	return s
}

var sgDrawerRe = regexp.MustCompile(`^:[A-Za-z_-]+:$`)

// The lines of a heading's body worth reading for phrases: no drawers,
// planning or clock lines, keywords, or the inside of source blocks.
func sgProseLines(body []string) []string {
	out := []string{}
	inDrawer, inBlock := false, false
	for _, l := range body {
		t := strings.TrimSpace(l)
		up := strings.ToUpper(t)
		switch {
		case inBlock:
			if strings.HasPrefix(up, "#+END_") {
				inBlock = false
			}
			continue
		case inDrawer:
			if up == ":END:" {
				inDrawer = false
			}
			continue
		case strings.HasPrefix(up, "#+BEGIN_SRC") || strings.HasPrefix(up, "#+BEGIN_EXAMPLE"):
			inBlock = true
			continue
		case sgDrawerRe.MatchString(t) && up != ":END:":
			inDrawer = true
			continue
		case strings.HasPrefix(t, "#"), strings.HasPrefix(up, "CLOCK:"), planningRe.MatchString(l):
			continue
		}
		if t != "" {
			out = append(out, l)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Resolving what a phrase names
// ---------------------------------------------------------------------------

var sgProtoRe = regexp.MustCompile(`^([a-zA-Z][a-zA-Z0-9+.-]*):`)

// resolveLink finds the chart task an org link points at: [[id:x]],
// [[#custom-id]], [[*Heading]], [[file:x.org::*Heading]], [[Heading]], or a
// url carrying a ticket key (.../browse/ABC-12).
func (idx *sgIndex) resolveLink(target string) *sgTask {
	target = strings.TrimSpace(target)
	if strings.HasPrefix(target, "id:") {
		return idx.byID[strings.TrimSpace(target[3:])]
	}
	path, search := "", target
	if strings.HasPrefix(target, "file:") || strings.Contains(target, "::") {
		t := strings.TrimPrefix(target, "file:")
		if i := strings.Index(t, "::"); i >= 0 {
			path, search = t[:i], t[i+2:]
		} else {
			return nil // a whole file is not a task
		}
	} else if m := sgProtoRe.FindStringSubmatch(target); m != nil {
		for _, k := range ticketKeys(target) {
			if t := idx.byKey[k]; t != nil {
				return t
			}
		}
		return nil
	}
	search = strings.TrimSpace(search)
	switch {
	case strings.HasPrefix(search, "#"):
		return idx.byID[strings.TrimSpace(search[1:])]
	case strings.HasPrefix(search, "*"):
		search = search[1:]
	}
	cands := idx.byTitle[sgNormTitle(search)]
	for _, t := range cands {
		if path == "" || filepath.Base(t.File) == filepath.Base(path) {
			return t
		}
	}
	return nil
}

// matchTitle finds the task whose title the phrase names, and how well. A
// title standing whole in the phrase is near certain; otherwise it is the
// share of the title's words the phrase has, weighted over the share of the
// phrase's words the title has. A title of two words or more needs two of
// them, so "needs review" does not find "Review the PR". A runner-up nearly as
// good makes the best less certain.
func (idx *sgIndex) matchTitle(text string, exclude *sgTask) (*sgTask, float64) {
	pw := sgWords(text)
	if len(pw) > 10 {
		pw = pw[:10]
	}
	if len(pw) == 0 {
		return nil, 0
	}
	flat := sgFlat(text)
	inP := map[string]bool{}
	for _, w := range pw {
		inP[w] = true
	}
	var best, second *sgTask
	bestS, secondS := 0.0, 0.0
	for _, t := range idx.tasks {
		if t == exclude {
			continue
		}
		tw := idx.words[t]
		if len(tw) == 0 {
			continue
		}
		s := 0.0
		ft := sgFlat(stripOrdinal(t.Title))
		if len(strings.TrimSpace(ft)) >= 4 && strings.Contains(flat, ft) {
			s = 0.95
		} else {
			inter := 0
			for _, w := range tw {
				if inP[w] {
					inter++
				}
			}
			need := 2
			if len(tw) < 2 {
				need = len(tw)
			}
			if inter == 0 || inter < need {
				continue
			}
			s = 0.65*float64(inter)/float64(len(tw)) + 0.35*float64(inter)/float64(len(pw))
		}
		if s > bestS {
			second, secondS = best, bestS
			best, bestS = t, s
		} else if s > secondS {
			second, secondS = t, s
		}
	}
	if best != nil && second != nil && secondS >= bestS-0.05 {
		bestS *= 0.7
	}
	return best, bestS
}

type sgCand struct {
	Task, After *sgTask
	Evidence    string
	Source      string
	Conf        float64
}

func round2(x float64) float64 { return math.Round(x*100) / 100 }

// phraseCands turns one phrase written on task t into candidate links.
// Links and keys are taken at their word; a phrase that has some but names
// nothing in the chart is about something outside it and is not guessed at.
func (idx *sgIndex) phraseCands(t *sgTask, p depPhrase, source string) []sgCand {
	out := []sgCand{}
	add := func(other *sgTask, src string, conf float64) {
		if other == nil || other == t {
			return
		}
		c := sgCand{Task: t, After: other, Evidence: p.Evidence, Source: src, Conf: round2(conf)}
		if p.Reverse {
			c.Task, c.After = other, t
		}
		out = append(out, c)
	}
	for _, l := range p.Links {
		src := "link"
		if source != "" {
			src = source
		}
		add(idx.resolveLink(l.Target), src, 0.9*p.Strength)
	}
	for _, k := range p.Keys {
		src := "key"
		if idx.keyJira[k] {
			src = "jira"
		}
		if source != "" {
			src = source
		}
		add(idx.byKey[k], src, 0.85*p.Strength)
	}
	if len(p.Links) == 0 && len(p.Keys) == 0 && source == "" {
		if other, s := idx.matchTitle(p.Text, t); other != nil {
			if conf := 0.75 * s * p.Strength; conf >= 0.4 {
				add(other, "text", conf)
			}
		}
	}
	return out
}

// Candidates from every task's own words.
func (idx *sgIndex) textCands() []sgCand {
	out := []sgCand{}
	for _, t := range idx.tasks {
		for _, l := range sgProseLines(t.Body) {
			for _, p := range extractDepPhrases(l) {
				out = append(out, idx.phraseCands(t, p, "")...)
			}
		}
	}
	return out
}

// Issue links kept as properties. The jira template does not write any, but
// a richer import (or a hand) can: BLOCKED_BY / DEPENDS_ON name what the task
// waits on, BLOCKS what waits on it, ISSUELINKS holds jira's own wording
// ("blocks ABC-1, is blocked by ABC-2").
var sgIssueLinkProps = map[string]string{
	"BLOCKED_BY": "blocked by ", "BLOCKEDBY": "blocked by ", "DEPENDS_ON": "depends on ",
	"DEPENDSON": "depends on ", "DEPENDENCIES": "depends on ", "BLOCKS": "blocks ", "BLOCKING": "blocks ",
	"ISSUELINKS": "", "ISSUE_LINKS": "", "LINKS": "",
}

func (idx *sgIndex) propCands() []sgCand {
	out := []sgCand{}
	for _, t := range idx.tasks {
		names := []string{}
		for k := range t.Props {
			if _, ok := sgIssueLinkProps[strings.ToUpper(k)]; ok {
				names = append(names, k)
			}
		}
		sort.Strings(names)
		for _, k := range names {
			v := t.Props[k]
			line := sgIssueLinkProps[strings.ToUpper(k)] + v
			for _, p := range extractDepPhrases(line) {
				p.Evidence = sgEvidence(":"+strings.ToUpper(k)+": "+strings.TrimSpace(v), 0)
				p.Strength = 1
				out = append(out, idx.phraseCands(t, p, "jira")...)
			}
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Numbered steps
// ---------------------------------------------------------------------------

var sgOrdWordRe = regexp.MustCompile(`(?i)^\s*(?:step|phase|part|stage|sprint|milestone|week|day|iteration|round)\s+([0-9]+|[ivxlc]+)\b[\s.:)-]*`)
var sgOrdNumRe = regexp.MustCompile(`^\s*\(?([0-9]{1,3})(?:[.)]|\s*[-:–])\s+`)
var sgOrdRomanRe = regexp.MustCompile(`(?i)^\s*([ivxlc]+)[.)]\s+`)

func romanValue(s string) (int, bool) {
	vals := map[rune]int{'i': 1, 'v': 5, 'x': 10, 'l': 50, 'c': 100}
	s = strings.ToLower(s)
	total, prev := 0, 0
	r := []rune(s)
	for i := len(r) - 1; i >= 0; i-- {
		v, ok := vals[r[i]]
		if !ok {
			return 0, false
		}
		if v < prev {
			total -= v
		} else {
			total += v
			prev = v
		}
	}
	return total, total > 0
}

// ordinalOf reads the step number a title starts with: "1. Design",
// "2) Build", "3 - Test", "Step 4", "Phase II", "Part iii.". Zero and false
// for a title that does not start with one.
func ordinalOf(title string) (int, bool) {
	for _, re := range []*regexp.Regexp{sgOrdWordRe, sgOrdNumRe, sgOrdRomanRe} {
		if m := re.FindStringSubmatch(title); m != nil {
			if n, err := strconv.Atoi(m[1]); err == nil {
				return n, true
			}
			if n, ok := romanValue(m[1]); ok {
				return n, true
			}
		}
	}
	return 0, false
}

func stripOrdinal(title string) string {
	for _, re := range []*regexp.Regexp{sgOrdWordRe, sgOrdNumRe, sgOrdRomanRe} {
		if loc := re.FindStringIndex(title); loc != nil {
			if rest := strings.TrimSpace(title[loc[1]:]); rest != "" {
				return rest
			}
		}
	}
	return title
}

// Siblings numbered as steps: each waits on the step before it. Two with the
// same number both wait on the last of the number before.
func (idx *sgIndex) sequenceCands() []sgCand {
	groups := map[string][]*sgTask{}
	order := []string{}
	for _, t := range idx.tasks {
		if _, ok := ordinalOf(t.Title); !ok {
			continue
		}
		k := t.File + "\x00" + t.Parent
		if _, seen := groups[k]; !seen {
			order = append(order, k)
		}
		groups[k] = append(groups[k], t)
	}
	out := []sgCand{}
	for _, k := range order {
		g := groups[k]
		if len(g) < 2 {
			continue
		}
		sort.SliceStable(g, func(a, b int) bool {
			na, _ := ordinalOf(g[a].Title)
			nb, _ := ordinalOf(g[b].Title)
			if na != nb {
				return na < nb
			}
			return g[a].Line < g[b].Line
		})
		var prevLast *sgTask
		prevN := -1
		var curLast *sgTask
		for _, t := range g {
			n, _ := ordinalOf(t.Title)
			if n != prevN {
				prevLast = curLast
				prevN = n
			}
			if prevLast != nil {
				ev := fmt.Sprintf("numbered steps: %q then %q", prevLast.Title, t.Title)
				if t.ParentTitle != "" {
					ev = fmt.Sprintf("numbered steps under %q: %q then %q", t.ParentTitle, prevLast.Title, t.Title)
				}
				out = append(out, sgCand{Task: t, After: prevLast, Evidence: sgEvidence(ev, 0), Source: "sequence", Conf: 0.5})
			}
			curLast = t
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Choosing which candidates to suggest
// ---------------------------------------------------------------------------

var sgSourceRank = map[string]int{"link": 0, "jira": 1, "key": 2, "text": 3, "sequence": 4}

// acceptDeps picks, most certain first, the candidates that can be applied:
// not a self link, not on a task that already waits on something (AFTER holds
// one id, so it would replace it), one per task, never one that closes a
// circle with the existing waits and the suggestions already taken, and never
// a wait on the task's own ancestor (a heading is not done before its
// subtree).
func acceptDeps(cands []sgCand, idx *sgIndex) []common.GanttDepSuggestion {
	sort.SliceStable(cands, func(a, b int) bool {
		ca, cb := cands[a], cands[b]
		if ca.Conf != cb.Conf {
			return ca.Conf > cb.Conf
		}
		if sgSourceRank[ca.Source] != sgSourceRank[cb.Source] {
			return sgSourceRank[ca.Source] < sgSourceRank[cb.Source]
		}
		if ca.Task.File != cb.Task.File {
			return ca.Task.File < cb.Task.File
		}
		return ca.Task.Line < cb.Task.Line
	})
	accepted := map[string]string{}
	waitsOn := func(h string) string {
		if a, ok := accepted[h]; ok {
			return a
		}
		if t := idx.byHash[h]; t != nil {
			return t.After
		}
		return ""
	}
	out := []common.GanttDepSuggestion{}
	for _, c := range cands {
		if c.Task == nil || c.After == nil || c.Task == c.After || c.Task.Hash == c.After.Hash {
			continue
		}
		if c.Task.After != "" {
			continue
		}
		if _, taken := accepted[c.Task.Hash]; taken {
			continue
		}
		cycle := false
		seen := map[string]bool{}
		for cur := c.After.Hash; cur != "" && !seen[cur]; cur = waitsOn(cur) {
			if cur == c.Task.Hash {
				cycle = true
				break
			}
			seen[cur] = true
		}
		if cycle {
			continue
		}
		ancestor := false
		seenP := map[string]bool{}
		for p := c.Task.Parent; p != "" && !seenP[p]; {
			seenP[p] = true
			if p == c.After.Hash {
				ancestor = true
				break
			}
			pt := idx.byHash[p]
			if pt == nil {
				break
			}
			p = pt.Parent
		}
		if ancestor {
			continue
		}
		accepted[c.Task.Hash] = c.After.Hash
		out = append(out, common.GanttDepSuggestion{
			Task: c.Task.Hash, After: c.After.Hash, AfterTitle: c.After.Title,
			Evidence: c.Evidence, Source: c.Source, Confidence: c.Conf,
		})
	}
	return out
}

// ---------------------------------------------------------------------------
// The existing links: what is redundant, what goes round in a circle
// ---------------------------------------------------------------------------

type sgEdge struct {
	From, To  string // From waits on To
	Removable bool
}

// redundantEdges is a transitive reduction: a removable edge is redundant
// when its target is still reached from its source without it. Edges are
// taken in order and one found redundant is treated as gone, so two links
// that each make the other redundant are not both reported.
func redundantEdges(edges []sgEdge) []common.GanttRedundantLink {
	out := []common.GanttRedundantLink{}
	removed := make([]bool, len(edges))
	for i, e := range edges {
		if !e.Removable || e.From == e.To {
			continue
		}
		// Breadth first from e.From, never along edge i or a removed one.
		prev := map[string]string{e.From: ""}
		queue := []string{e.From}
		found := false
		for len(queue) > 0 && !found {
			cur := queue[0]
			queue = queue[1:]
			for j, f := range edges {
				if j == i || removed[j] || f.From != cur {
					continue
				}
				if _, seen := prev[f.To]; seen {
					continue
				}
				prev[f.To] = cur
				if f.To == e.To {
					found = true
					break
				}
				queue = append(queue, f.To)
			}
		}
		if !found {
			continue
		}
		via := []string{}
		for n := prev[e.To]; n != "" && n != e.From; n = prev[n] {
			via = append([]string{n}, via...)
		}
		removed[i] = true
		out = append(out, common.GanttRedundantLink{Task: e.From, After: e.To, Via: via, Removable: true})
	}
	return out
}

// findCycles reports each set of tasks that wait on each other in a circle,
// once, as hashes in wait order (each waits on the next, the last on the
// first), starting from whichever comes first in the input.
func findCycles(edges []sgEdge) [][]string {
	out := [][]string{}
	nodes := []string{}
	adj := map[string][]string{}
	seenN := map[string]bool{}
	for _, e := range edges {
		adj[e.From] = append(adj[e.From], e.To)
		for _, n := range []string{e.From, e.To} {
			if !seenN[n] {
				seenN[n] = true
				nodes = append(nodes, n)
			}
		}
	}
	// Tarjan's strongly connected components.
	index := map[string]int{}
	low := map[string]int{}
	onStack := map[string]bool{}
	stack := []string{}
	next := 0
	comp := map[string]int{}
	comps := [][]string{}
	var strong func(v string)
	strong = func(v string) {
		index[v] = next
		low[v] = next
		next++
		stack = append(stack, v)
		onStack[v] = true
		for _, w := range adj[v] {
			if _, ok := index[w]; !ok {
				strong(w)
				low[v] = min(low[v], low[w])
			} else if onStack[w] {
				low[v] = min(low[v], index[w])
			}
		}
		if low[v] == index[v] {
			c := []string{}
			for {
				w := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				onStack[w] = false
				comp[w] = len(comps)
				c = append(c, w)
				if w == v {
					break
				}
			}
			comps = append(comps, c)
		}
	}
	for _, n := range nodes {
		if _, ok := index[n]; !ok {
			strong(n)
		}
	}
	done := map[int]bool{}
	for _, n := range nodes {
		ci := comp[n]
		if done[ci] {
			continue
		}
		c := comps[ci]
		selfLoop := false
		for _, w := range adj[n] {
			if w == n {
				selfLoop = true
			}
		}
		if len(c) == 1 && !selfLoop {
			continue
		}
		done[ci] = true
		if len(c) == 1 {
			out = append(out, []string{n})
			continue
		}
		// Walk inside the component from n until a node repeats.
		path := []string{}
		at := map[string]int{}
		cur := n
		for {
			if i, ok := at[cur]; ok {
				out = append(out, path[i:])
				break
			}
			at[cur] = len(path)
			path = append(path, cur)
			nx := ""
			for _, w := range adj[cur] {
				if comp[w] == ci {
					nx = w
					break
				}
			}
			if nx == "" {
				break
			}
			cur = nx
		}
	}
	return out
}

// The chart's dependency edges as they stand. With latent, an AFTER task's
// ORDERED chain is added as a fixed edge, which is what makes an AFTER the
// chain already implies show up as redundant: clearing it hands the task back
// to a chain that still comes after the same heading.
func sgEdges(tasks []*sgTask, latent bool) []sgEdge {
	in := map[string]bool{}
	for _, t := range tasks {
		in[t.Hash] = true
	}
	out := []sgEdge{}
	for _, t := range tasks {
		if t.After != "" && in[t.After] {
			out = append(out, sgEdge{From: t.Hash, To: t.After, Removable: t.ViaAfter})
		}
	}
	if latent {
		for _, t := range tasks {
			if t.ViaAfter && t.ChainPrev != "" && in[t.ChainPrev] && t.ChainPrev != t.Hash {
				out = append(out, sgEdge{From: t.Hash, To: t.ChainPrev})
			}
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Estimates
// ---------------------------------------------------------------------------

// A finished task, measured.
type sgSample struct {
	Words    []string
	Tags     []string
	Assignee string
	File     string
	Mins     float64
}

func sgFmtNum(x float64) string { return strconv.FormatFloat(x, 'f', -1, 64) }

// roundEffort writes minutes of work as an org effort a person would have
// written: quarter hours below two hours ("45min", "1.25h"), half hours below
// a day ("3.5h"), half days above ("1.5d"), a day being dayHours. It answers
// the effort and the minutes it stands for.
func roundEffort(mins, dayHours float64) (string, float64) {
	if mins <= 0 {
		return "", 0
	}
	if dayHours <= 0 {
		dayHours = 8
	}
	day := dayHours * 60
	if mins < 120 {
		q := math.Max(15, math.Round(mins/15)*15)
		if q < 60 {
			return sgFmtNum(q) + "min", q
		}
		if q < day {
			return sgFmtNum(q/60) + "h", q
		}
	}
	if mins < day {
		q := math.Round(mins/30) * 30
		if q < day {
			return sgFmtNum(q/60) + "h", q
		}
	}
	d := math.Max(0.5, math.Round(mins/day*2)/2)
	return sgFmtNum(d) + "d", d * day
}

func median(xs []float64) float64 {
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	return quantile(s, 0.5)
}

// guessEffort estimates an open task from the finished ones most like it:
// the same person on the same tag, the same tag, the same person, the same
// file, then titles sharing words - the first with three or more measured
// tasks. With none, the median EFFORT already written in the chart, said to
// be a last resort.
func guessEffort(t *sgTask, hist []sgSample, chartEfforts []float64, dayHours float64) (common.GanttEstimateGuess, bool) {
	g := common.GanttEstimateGuess{Task: t.Hash}
	type level struct {
		base  float64
		basis string
		mins  []float64
	}
	pick := func(keep func(s sgSample) bool) []float64 {
		out := []float64{}
		for _, s := range hist {
			if keep(s) {
				out = append(out, s.Mins)
			}
		}
		return out
	}
	// The tag with the most measured tasks, among this task's own tags.
	bestTag := func(extra func(s sgSample) bool) (string, []float64) {
		bt, bm := "", []float64(nil)
		for _, tag := range t.Tags {
			m := pick(func(s sgSample) bool { return hasTagFold(s.Tags, tag) && extra(s) })
			if len(m) > len(bm) {
				bt, bm = tag, m
			}
		}
		return bt, bm
	}
	levels := []level{}
	who := strings.TrimSpace(t.Assignee)
	if who != "" {
		tag, m := bestTag(func(s sgSample) bool { return strings.EqualFold(s.Assignee, who) })
		levels = append(levels, level{0.8, fmt.Sprintf("tagged :%s: by %s", tag, who), m})
	}
	if tag, m := bestTag(func(sgSample) bool { return true }); tag != "" {
		levels = append(levels, level{0.65, fmt.Sprintf("tagged :%s:", tag), m})
	}
	if who != "" {
		levels = append(levels, level{0.5, "by " + who, pick(func(s sgSample) bool { return strings.EqualFold(s.Assignee, who) })})
	}
	if t.File != "" {
		levels = append(levels, level{0.45, "in " + filepath.Base(t.File), pick(func(s sgSample) bool { return s.File == t.File })})
	}
	tw := sgWords(stripOrdinal(t.Title))
	if len(tw) > 0 {
		shared := map[string]int{}
		m := pick(func(s sgSample) bool {
			inter := 0
			seen := map[string]bool{}
			for _, w := range s.Words {
				seen[w] = true
			}
			hit := []string{}
			for _, w := range tw {
				if seen[w] {
					inter++
					hit = append(hit, w)
				}
			}
			union := len(tw) + len(s.Words) - inter
			if inter >= 2 || (union > 0 && float64(inter)/float64(union) >= 0.34) {
				for _, w := range hit {
					shared[w]++
				}
				return true
			}
			return false
		})
		words := []string{}
		for w := range shared {
			words = append(words, w)
		}
		sort.SliceStable(words, func(a, b int) bool {
			if shared[words[a]] != shared[words[b]] {
				return shared[words[a]] > shared[words[b]]
			}
			return words[a] < words[b]
		})
		if len(words) > 3 {
			words = words[:3]
		}
		levels = append(levels, level{0.4, "with titles like this one (" + strings.Join(words, ", ") + ")", m})
	}
	for _, l := range levels {
		if len(l.mins) < minTasksForMultiplier {
			continue
		}
		med := median(l.mins)
		g.Effort, g.Mins = roundEffort(med, dayHours)
		if g.Effort == "" {
			continue
		}
		g.Samples = len(l.mins)
		g.Basis = fmt.Sprintf("median of %d finished tasks %s", g.Samples, l.basis)
		conf := l.base * (0.7 + 0.3*math.Min(1, float64(g.Samples-3)/7))
		s := append([]float64(nil), l.mins...)
		sort.Float64s(s)
		if p25 := quantile(s, 0.25); p25 > 0 && quantile(s, 0.75)/p25 > 3 {
			conf *= 0.75
		}
		g.Confidence = round2(conf)
		return g, true
	}
	if len(chartEfforts) > 0 {
		g.Effort, g.Mins = roundEffort(median(chartEfforts), dayHours)
		if g.Effort == "" {
			return g, false
		}
		g.Samples = len(chartEfforts)
		g.Basis = fmt.Sprintf("median EFFORT of %d estimated task%s in this chart: no finished work like it to measure", g.Samples, plural(g.Samples))
		g.Confidence = 0.2
		return g, true
	}
	return g, false
}

// ---------------------------------------------------------------------------
// Lanes
// ---------------------------------------------------------------------------

const sgNone = "(none)"

// Properties that are identity, bookkeeping or scheduling, never a lane.
var sgLaneSkip = map[string]bool{
	"ID": true, "CUSTOM_ID": true, "EFFORT": true, "AFTER": true, "ORDERED": true, "NOBLOCKING": true,
	"CREATED": true, "UPDATED": true, "LINK": true, "PERCENTDONE": true, "GANTT_COLOR": true, "COLOR": true,
	"GANTT_ORDER": true, "CLOCKSUM": true, "LAST_REPEAT": true, "STYLE": true, "ARCHIVE_TIME": true,
	"ARCHIVE_FILE": true, "ARCHIVE_OLPATH": true, "ARCHIVE_CATEGORY": true, "ARCHIVE_TODO": true,
	"ARCHIVE_ITAGS": true, "ROAM_ALIASES": true, "ALIASES": true, "LOGGING": true, "REPEAT_TO_STATE": true,
	"ITEM": true, "TODO": true, "ALLTAGS": true, "TAGS": true, "FILE": true, "BLOCKED": true, "CLOSED": true,
	"SCHEDULED": true, "DEADLINE": true, "TIMESTAMP": true, "STATUS": true, "DESCRIPTION": true,
	"BLOCKED_BY": true, "BLOCKEDBY": true, "DEPENDS_ON": true, "DEPENDSON": true, "DEPENDENCIES": true,
	"BLOCKS": true, "BLOCKING": true, "ISSUELINKS": true, "ISSUE_LINKS": true, "LINKS": true,
}

// A property value as a lane name: [email] and [id] parts dropped (the jira
// template writes ASSIGNED as "Name [email]"), spaces collapsed.
func sgLaneValue(v string) string {
	v = personLinkRe.ReplaceAllString(v, "")
	return strings.Join(strings.Fields(v), " ")
}

// scoreLanes rates one grouping of n tasks. Coverage (tasks not in "(none)")
// counts most, then how evenly they are spread, then having two to eight
// groups, then how few dependency edges cross between groups.
func scoreLanes(groups []common.GanttLaneGroup, n int, cross, edges int) float64 {
	if n == 0 {
		return 0
	}
	covered := 0
	sizes := []float64{}
	for _, g := range groups {
		if g.Name == sgNone {
			continue
		}
		covered += len(g.Tasks)
		sizes = append(sizes, float64(len(g.Tasks)))
	}
	k := len(sizes)
	if k < 2 {
		return 0
	}
	cov := float64(covered) / float64(n)
	h := 0.0
	for _, s := range sizes {
		p := s / float64(covered)
		if p > 0 {
			h -= p * math.Log(p)
		}
	}
	bal := h / math.Log(float64(k))
	fit := 1.0
	if k > 8 {
		fit = 8 / float64(k)
	}
	crossOk := 1.0
	if edges > 0 {
		crossOk = 1 - float64(cross)/float64(edges)
	}
	return round2(0.35*cov + 0.25*bal + 0.2*fit + 0.2*crossOk)
}

// laneSuggestions tries each way of splitting the tasks and keeps the best
// five distinct ones. edges are the dependencies, existing and suggested, as
// [waiter, waited-on] hashes.
func laneSuggestions(tasks []*sgTask, edges [][2]string) []common.GanttLaneSuggestion {
	n := len(tasks)
	if n == 0 {
		return []common.GanttLaneSuggestion{}
	}
	in := map[string]bool{}
	for _, t := range tasks {
		in[t.Hash] = true
	}
	inEdges := [][2]string{}
	for _, e := range edges {
		if in[e[0]] && in[e[1]] && e[0] != e[1] {
			inEdges = append(inEdges, e)
		}
	}

	type cand struct {
		by, prop, label string
		of              map[string]string // hash -> group
	}
	cands := []cand{}

	// Tags: each task's first tag that two or more tasks carry and not all.
	tagCount := map[string]int{}
	for _, t := range tasks {
		seen := map[string]bool{}
		for _, tg := range t.Tags {
			l := strings.ToLower(tg)
			if !seen[l] {
				seen[l] = true
				tagCount[l]++
			}
		}
	}
	byTag := map[string]string{}
	for _, t := range tasks {
		for _, tg := range t.Tags {
			if c := tagCount[strings.ToLower(tg)]; c >= 2 && c < n {
				byTag[t.Hash] = tg
				break
			}
		}
	}
	cands = append(cands, cand{"tag", "", "By tag", byTag})

	byParent := map[string]string{}
	for _, t := range tasks {
		if t.ParentTitle != "" {
			byParent[t.Hash] = t.ParentTitle
		}
	}
	cands = append(cands, cand{"parent", "", "By parent heading", byParent})

	byFile := map[string]string{}
	for _, t := range tasks {
		if t.File != "" {
			byFile[t.Hash] = filepath.Base(t.File)
		}
	}
	cands = append(cands, cand{"file", "", "By file", byFile})

	// Chains: the connected pieces of the dependency graph, each named after
	// the first task in it that waits on nothing in it.
	parent := map[string]string{}
	var find func(x string) string
	find = func(x string) string {
		if p, ok := parent[x]; ok && p != x {
			r := find(p)
			parent[x] = r
			return r
		}
		parent[x] = x
		return x
	}
	waits := map[string]bool{}
	for _, e := range inEdges {
		a, b := find(e[0]), find(e[1])
		if a != b {
			parent[a] = b
		}
		waits[e[0]] = true
	}
	compSize := map[string]int{}
	for _, t := range tasks {
		compSize[find(t.Hash)]++
	}
	compName := map[string]string{}
	for _, t := range tasks {
		r := find(t.Hash)
		if compSize[r] >= 2 && !waits[t.Hash] {
			if _, named := compName[r]; !named {
				compName[r] = t.Title
			}
		}
	}
	// A chain that is all circle has no first task; it is named after
	// whichever comes first.
	for _, t := range tasks {
		r := find(t.Hash)
		if _, named := compName[r]; !named && compSize[r] >= 2 {
			compName[r] = t.Title
		}
	}
	byChain := map[string]string{}
	for _, t := range tasks {
		r := find(t.Hash)
		if compSize[r] < 2 {
			continue
		}
		byChain[t.Hash] = compName[r]
	}
	cands = append(cands, cand{"chain", "", "By dependency chain", byChain})

	// Properties with a few values covering most tasks.
	propNames := map[string]string{}
	for _, t := range tasks {
		for k := range t.Props {
			u := strings.ToUpper(k)
			if !sgLaneSkip[u] {
				if _, ok := propNames[u]; !ok {
					propNames[u] = k
				}
			}
		}
	}
	names := []string{}
	for u := range propNames {
		names = append(names, u)
	}
	sort.Strings(names)
	for _, u := range names {
		of := map[string]string{}
		distinct := map[string]bool{}
		for _, t := range tasks {
			if v := sgLaneValue(sgProp(t.Props, u)); v != "" {
				of[t.Hash] = v
				distinct[v] = true
			}
		}
		if len(distinct) < 2 || len(distinct) > 10 || float64(len(of)) < 0.6*float64(n) {
			continue
		}
		cands = append(cands, cand{"property", u, "By " + u, of})
	}

	out := []common.GanttLaneSuggestion{}
	sigs := map[string]int{}
	for _, c := range cands {
		groups := []common.GanttLaneGroup{}
		at := map[string]int{}
		for _, t := range tasks {
			name := c.of[t.Hash]
			if name == "" {
				name = sgNone
			}
			i, ok := at[name]
			if !ok {
				i = len(groups)
				at[name] = i
				groups = append(groups, common.GanttLaneGroup{Name: name, Tasks: []string{}})
			}
			groups[i].Tasks = append(groups[i].Tasks, t.Hash)
		}
		sort.SliceStable(groups, func(a, b int) bool {
			if (groups[a].Name == sgNone) != (groups[b].Name == sgNone) {
				return groups[b].Name == sgNone
			}
			return len(groups[a].Tasks) > len(groups[b].Tasks)
		})
		cross := 0
		for _, e := range inEdges {
			if c.of[e[0]] != c.of[e[1]] || c.of[e[0]] == "" {
				cross++
			}
		}
		score := scoreLanes(groups, n, cross, len(inEdges))
		if score <= 0 {
			continue
		}
		// The same split twice (every file one parent, say) is one choice.
		parts := []string{}
		for _, g := range groups {
			s := append([]string(nil), g.Tasks...)
			sort.Strings(s)
			parts = append(parts, strings.Join(s, ","))
		}
		sort.Strings(parts)
		sig := strings.Join(parts, "|")
		s := common.GanttLaneSuggestion{By: c.by, Prop: c.prop, Label: c.label, Score: score, Groups: groups, CrossLinks: cross}
		if i, dup := sigs[sig]; dup {
			if score > out[i].Score {
				out[i] = s
			}
			continue
		}
		sigs[sig] = len(out)
		out = append(out, s)
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].Score > out[b].Score })
	if len(out) > 5 {
		out = out[:5]
	}
	return out
}

// ---------------------------------------------------------------------------
// Reading the database
// ---------------------------------------------------------------------------

// The finished tasks in the window with clocked time, measured as the
// estimates insight measures them (insights.go, buildEstimates): a task's own
// clocks plus those of headings under it with no EFFORT of their own, and a
// rollup over estimated headings is not a sample. ratios are actual over
// EFFORT where there is one.
func gatherSuggestHistory(rng clockRange, dayHours float64, now time.Time) (samples []sgSample, ratios []float64) {
	registerAllSections()
	samples, ratios = []sgSample{}, []float64{}
	var walk func(s *org.Section, f *common.OrgFile)
	walk = func(s *org.Section, f *common.OrgFile) {
		if s.Headline == nil {
			return
		}
		for _, c := range s.Children {
			walk(c, f)
		}
		h := s.Headline
		if h.Status == "" || isCancelledKeyword(h.Status) || !insightDone(s, f) || hasEstimatedDescendant(s) {
			return
		}
		at, ok := finishedAt(h)
		if rng.Set && (!ok || !inRange(rng, at)) {
			return
		}
		mins := unestimatedSubtreeMins(s, now)
		if mins <= 0 {
			return
		}
		tags := []string{}
		for _, tag := range inheritedTags(s) {
			if strings.EqualFold(tag, "archive") {
				continue
			}
			tags = append(tags, tag)
		}
		samples = append(samples, sgSample{
			Words: sgWords(stripOrdinal(insightTitle(s))), Tags: tags,
			Assignee: assigneeOf(s), File: f.Filename, Mins: mins,
		})
		if est, _, ok := effortMins(insightProp(s, "EFFORT"), dayHours); ok && est > 0 {
			if r := mins / est; r >= 0.05 && r <= 20 {
				ratios = append(ratios, round2(r))
			}
		}
	}
	for _, name := range GetDb().GetFiles() {
		f := GetDb().GetFile(name)
		if f == nil || f.Doc == nil {
			continue
		}
		for _, s := range f.Doc.Outline.Children {
			walk(s, f)
		}
	}
	return samples, ratios
}

// The chart's tasks read into sgTasks: bodies off the files' own lines, the
// person by assigneeOf, and for an AFTER task what its ORDERED chain would be
// with the AFTER gone.
func sgTasksFrom(set *ganttSet) []*sgTask {
	fileLines := map[string][]string{}
	linesOf := func(fn string) []string {
		if l, ok := fileLines[fn]; ok {
			return l
		}
		b, err := os.ReadFile(fn)
		var l []string
		if err == nil {
			l = strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
		}
		fileLines[fn] = l
		return l
	}
	in := map[string]*common.GanttTask{}
	for i := range set.Data.Tasks {
		in[set.Data.Tasks[i].Hash] = &set.Data.Tasks[i]
	}
	out := []*sgTask{}
	for i := range set.Data.Tasks {
		gt := &set.Data.Tasks[i]
		t := &sgTask{
			Hash: gt.Hash, Title: gt.Headline, File: gt.Filename, Line: gt.LineNum, Parent: gt.Parent,
			ID: strings.TrimSpace(gt.Props["ID"]), CustomID: strings.TrimSpace(gt.Props["CUSTOM_ID"]),
			Tags: gt.Tags, Props: gt.Props, Open: gt.IsActive && !gt.Done, HasEffort: strings.TrimSpace(gt.Effort) != "",
		}
		if t.Props == nil {
			t.Props = map[string]string{}
		}
		if gt.After != "" && in[gt.After] != nil {
			t.After = gt.After
			t.ViaAfter = strings.TrimSpace(gt.AfterId) != ""
		}
		if p := in[gt.Parent]; p != nil {
			t.ParentTitle = p.Headline
		} else if gt.Parent != "" {
			if ps := GetDb().FindByHash(gt.Parent); ps != nil && ps.Headline != nil {
				t.ParentTitle = insightTitle(ps)
			}
		}
		if sec := GetDb().FindByHash(gt.Hash); sec != nil && sec.Headline != nil {
			f := GetDb().FileFromSection(sec)
			t.Assignee = assigneeOf(sec)
			if f != nil {
				t.Open = insightOpen(sec, f)
				lines := linesOf(f.Filename)
				row := sec.Headline.Pos.Row
				if row >= 0 && row < len(lines) {
					end := ownEnd(lines, row)
					if end >= row+1 && end < len(lines) {
						t.Body = append([]string(nil), lines[row+1:end+1]...)
					}
				}
			}
		}
		if t.ViaAfter {
			if td := set.Todos[gt.Hash]; td != nil {
				bare := *td
				bare.Props = map[string]string{}
				for k, v := range td.Props {
					if k != "AFTER" {
						bare.Props[k] = v
					}
				}
				if dep := mermaid.After(set.have, db, &bare); dep != nil {
					if h := canonHash(set.canon, dep); h != gt.Hash {
						t.ChainPrev = h
					}
				}
			}
		}
		out = append(out, t)
	}
	return out
}

func buildGanttSuggest(req common.GanttSuggestReq, now time.Time) (common.GanttSuggest, error) {
	res := common.GanttSuggest{
		Query: req.Query, Deps: []common.GanttDepSuggestion{}, Redundant: []common.GanttRedundantLink{},
		Cycles: [][]string{}, Estimates: []common.GanttEstimateGuess{}, Lanes: []common.GanttLaneSuggestion{},
		Ratios: []float64{},
	}
	want := map[string]bool{}
	for _, k := range req.Kinds {
		k = strings.ToLower(strings.TrimSpace(k))
		switch k {
		case "":
		case "deps", "estimates", "lanes", "ratios":
			want[k] = true
		default:
			return res, fmt.Errorf("kind %q: ask for deps, estimates, lanes or ratios", k)
		}
	}
	all := len(want) == 0
	dayHours := req.DayHours
	if dayHours <= 0 {
		dayHours = 8
	}
	if dayHours > 24 {
		return res, fmt.Errorf("DayHours %v: hours in a day of effort, 1 to 24", dayHours)
	}
	from := strings.TrimSpace(req.From)
	if from == "" {
		from = "-180d"
	}

	set, err := ganttTaskSet(req.Query)
	if err != nil {
		return res, fmt.Errorf("gantt suggest: failed to query expression, %v [%s]", err, req.Query)
	}
	tasks := sgTasksFrom(set)
	idx := newSgIndex(tasks)

	var deps []common.GanttDepSuggestion
	if all || want["deps"] || want["lanes"] {
		cands := idx.textCands()
		cands = append(cands, idx.propCands()...)
		cands = append(cands, idx.sequenceCands()...)
		deps = acceptDeps(cands, idx)
	}
	if all || want["deps"] {
		res.Deps = deps
		res.Redundant = redundantEdges(sgEdges(tasks, true))
		res.Cycles = findCycles(sgEdges(tasks, false))
	}
	if all || want["estimates"] || want["ratios"] {
		rng, err := insightRange("", from, "", now)
		if err != nil {
			return res, err
		}
		hist, ratios := gatherSuggestHistory(rng, dayHours, now)
		if all || want["ratios"] {
			res.Ratios = ratios
		}
		if all || want["estimates"] {
			chart := []float64{}
			for _, t := range set.Data.Tasks {
				if m, _, ok := effortMins(t.Effort, dayHours); ok {
					chart = append(chart, m)
				}
			}
			for _, t := range tasks {
				if !t.Open || t.HasEffort {
					continue
				}
				if g, ok := guessEffort(t, hist, chart, dayHours); ok {
					res.Estimates = append(res.Estimates, g)
				}
			}
		}
	}
	if all || want["lanes"] {
		edges := [][2]string{}
		for _, e := range sgEdges(tasks, false) {
			edges = append(edges, [2]string{e.From, e.To})
		}
		for _, d := range deps {
			edges = append(edges, [2]string{d.Task, d.After})
		}
		res.Lanes = laneSuggestions(tasks, edges)
	}
	res.Ok = true
	return res, nil
}

/* SDOC: API
* POST /gantt/suggest — Suggestions for Setting Up a Chart
	What a chart built from a query could be told that it is not yet, read
	from the tasks themselves and from finished work. Nothing is written: a
	client shows the suggestions, a person picks, and the picks are written
	with =/gantt/link= (dependencies) and =/property= (EFFORT, a lane
	property). The tasks are exactly what =/gantt/tasks= finds for the query.

	- *Dependencies* come from each task's own body (not its children's):
	  "blocked by", "depends on", "dependent on", "waiting on/for", "after",
	  "requires", "needs", "follows", "prerequisite:" name what it waits on;
	  "blocks", "before", "prerequisite for", "required by" what waits on it.
	  What the phrase names is found by org link (=[[id:..]]=, =[[#custom]]=,
	  =[[*Heading]]=, =[[file:x.org::*Heading]]=; =Source= "link"), by ticket
	  key such as =ABC-123= against another task's CUSTOM_ID, LINK or headline
	  ("jira" when that task came from the jira import, else "key"), and
	  otherwise by matching the words against task headlines ("text",
	  confidence by how well they match). Issue links kept as =BLOCKED_BY=,
	  =DEPENDS_ON=, =BLOCKS= or =ISSUELINKS= properties count as "jira", and
	  numbered siblings (=1.=, =2)=, =Step 3=, =Phase II=) as "sequence".
	  AFTER holds one id, so a task that already waits on something (AFTER,
	  or an ORDERED parent) is given none, each task is given at most one,
	  and none closes a circle with the existing links and the more certain
	  suggestions before it. Most certain first.
	- *Redundant* lists AFTER links the chart would keep without: the
	  ORDERED chain the task would fall back to already comes after the same
	  heading. *Cycles* lists tasks already waiting on each other in a circle.
	- *Estimates* are for open tasks with no EFFORT: the median clocked time
	  of finished tasks in the window most like it (same person and tag, same
	  tag, same person, same file, similar title; three or more needed),
	  else the median EFFORT already in the chart. Rounded to quarter hours
	  under two hours, half hours under a day, half days above.
	- *Lanes* are the five best ways to split the tasks into swim lanes: by
	  tag, parent heading, file, dependency chain, or any property with two
	  to ten values on 60% of the tasks. Scored on coverage, balance, two to
	  eight groups and few dependencies crossing between lanes.
	- *Ratios* are actual over estimated for every finished task in the
	  window with both an EFFORT and clocked time, for a forecast.

	*Method:* =POST=

	*Request Body:* A =GanttSuggestReq=.
	| Field      | Type   | Description                                                      |
	|------------+--------+------------------------------------------------------------------|
	| =Query=    | string | The chart's query.                                               |
	| =Kinds=    | list   | Any of =deps=, =estimates=, =lanes=, =ratios=. Empty means all. |
	| =DayHours= | number | Hours in a day of effort. Default 8.                             |
	| =From=     | string | Start of the history window, as typed anywhere. Default =-180d=. |

	*Response:* A =GanttSuggest=: =Ok=, =Msg=, =Query=, =Deps= (=Task=,
	=After=, =AfterTitle=, =Evidence=, =Source=, =Confidence=), =Redundant=
	(=Task=, =After=, =Via=, =Removable=), =Cycles= (lists of hashes, each
	waiting on the next), =Estimates= (=Task=, =Effort=, =Mins=, =Basis=,
	=Samples=, =Confidence=), =Lanes= (=By=, =Prop=, =Label=, =Score=,
	=Groups= of =Name= and =Tasks=, =CrossLinks=) and =Ratios=.
	EDOC */
func PostGanttSuggest(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req common.GanttSuggestReq
	if len(strings.TrimSpace(string(body))) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			answerErr(w, err)
			return
		}
	}
	res, err := buildGanttSuggest(req, time.Now())
	if err != nil {
		res.Ok = false
		res.Msg = err.Error()
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}
