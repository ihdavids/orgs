package orgs

/* SDOC: Links
* Backlinks Of One Heading, And Unlinked Mentions

  The link index answers "what links to this file". Two questions sit next to
  it that a person writing notes asks far more often:

  - *What links to this heading?* The same index, read at the heading's end
    rather than the file's, with the line each link was written on so a
    reader can tell them apart without opening each one.
  - *Where is this written about without a link?* Every place the heading's
    title - or one of its aliases - is written in plain text in some other
    heading. These are the links that should exist and do not, and the answer
    comes with a way to make them: one request turns any of them into a link,
    as a change to that one line.

  A mention is looked for case insensitively, as a whole word or phrase, in
  headlines and body text. It is not looked for inside a link (it is already
  linked), inside a src, example, export or comment block, a property drawer
  or a logbook, a =#+KEYWORD:= line, a comment, a planning line, verbatim or
  code text, or a url - none of those is prose a link belongs in. Nor in the
  target itself: a heading's own subtree, or the whole of a file.

  A title can be what most headings are called somewhere. A heading named
  "Notes" would find every second line, so a title that is only common words
  (or shorter than three letters) is not searched for unless asked with
  =all=; the answer says which names were left out and why. An alias is taken
  at its word - somebody wrote it down to be found by - and is only refused
  when it is shorter than two letters.

  Aliases are the =ROAM_ALIASES= property org-roam writes (space separated,
  double quotes around a name with spaces in it) and a plain =ALIASES=
  property. A file is named by its =#+TITLE:=, or failing that its file name,
  and its aliases are read from the property drawer above its first heading
  and from =#+ALIASES:= / =#+ROAM_ALIASES:= lines.
EDOC */

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

// registerMentionRoutes adds the endpoints for heading backlinks and unlinked mentions. Called from RestApi.
func registerMentionRoutes(api *mux.Router) {
	api.HandleFunc("/links/heading", RequestLinksAround).Methods("GET")
	api.HandleFunc("/mentions", RequestMentions).Methods("GET")
	api.HandleFunc("/mentions/link", PostLinkMentions).Methods("POST")
}

// ---------------------------------------------------------------------------
// Reading a file for the text a mention may be in
// ---------------------------------------------------------------------------

// One line of a file that has prose on it, and which parts of it are prose.
type mentionLine struct {
	row    int
	text   string
	lower  string   // for a cheap test before the regular expression runs
	from   int    // the part of the line a mention may be in, in bytes
	to     int    //
	head   int    // row of the heading the line is under, -1 in the preamble
	isHead bool     // the line is that headline
}

// A file read for mentions: its prose lines and where its headings are.
type mentionScan struct {
	lines    []mentionLine
	headRows []int // headline rows, in order
	headLvls []int // their levels
}

var (
	mentionHeadRe     = regexp.MustCompile(`^(\*+)\s`)
	mentionTagsRe     = regexp.MustCompile(`\s+:[\p{L}\p{N}_@#%:]+:\s*$`)
	mentionBlockRe    = regexp.MustCompile(`(?i)^\s*#\+begin_(\S+)`)
	mentionBlockEndRe = regexp.MustCompile(`(?i)^\s*#\+end_(\S+)`)
	mentionKeywordRe  = regexp.MustCompile(`^\s*#\+`)
	mentionCommentRe  = regexp.MustCompile(`^\s*#(\s|$)`)
	mentionFixedRe    = regexp.MustCompile(`^\s*:(\s|$)`)
	mentionClockRe    = regexp.MustCompile(`^\s*CLOCK:`)
	mentionTodoLineRe = regexp.MustCompile(`(?i)^\s*#\+(?:seq_|typ_)?todo:\s*(.*)$`)

	// What is not prose, inside a line that is. Each is only run over a line
	// holding the text it starts with (mentionSkipTriggers, in step with it),
	// because most lines hold none of them and nine expressions per line add up.
	mentionSkipTriggers = []string{"[[", ":", "<[", "[", "<", "[", "_"}
	mentionSkipRes      = []*regexp.Regexp{
		regexp.MustCompile(`\[\[.*?\]\]`),                                          // a link, description and all
		regexp.MustCompile(`(?i)\b(?:https?|ftps?|file|mailto|id|doi|news|irc):[^\s\]>)]+`), // a bare url
		regexp.MustCompile(`[<\[]\d{4}-\d{2}-\d{2}[^>\]]*[>\]]`),                  // a timestamp
		regexp.MustCompile(`\[fn:[^\]]*\]`),                                        // a footnote
		regexp.MustCompile(`<<<?[^<>]*>>>?`),                                       // a target
		regexp.MustCompile(`\[\d*/\d*\]|\[\d*%\]|\[#[A-Za-z0-9]\]`),               // cookies, a priority
		regexp.MustCompile(`\b(?:src|call)_[\w-]+(?:\[[^\]]*\])?(?:\{[^}]*\}|\([^)]*\))`), // inline code
	}
	// Verbatim and code. Go's regexp has no backreferences, so the two
	// markers are one expression each; the span wanted is group one.
	mentionVerbatimRes = []*regexp.Regexp{
		regexp.MustCompile(`(?:^|[\s({'"])(=[^\s=](?:[^=]*[^\s=])?=)`),
		regexp.MustCompile(`(?:^|[\s({'"])(~[^\s~](?:[^~]*[^\s~])?~)`),
	}
)

// Blocks whose insides are not prose. A quote or a verse block is, and is
// searched like any other text.
var mentionCodeBlocks = map[string]bool{"src": true, "example": true, "export": true, "comment": true}

// Drawers whose insides are not prose.
var mentionSkipDrawers = map[string]bool{"PROPERTIES": true, "LOGBOOK": true}

// The spans of a line that are not prose, within [from, to).
func mentionSkips(text string, from, to int) [][2]int {
	part := text[from:to]
	var out [][2]int
	for i, re := range mentionSkipRes {
		trig := mentionSkipTriggers[i]
		if (i == 0 && !strings.Contains(part, trig)) || (i > 0 && !strings.ContainsAny(part, trig)) {
			continue
		}
		for _, m := range re.FindAllStringIndex(part, -1) {
			out = append(out, [2]int{from + m[0], from + m[1]})
		}
	}
	for i, re := range mentionVerbatimRes {
		if strings.IndexByte(part, "=~"[i]) < 0 {
			continue
		}
		for _, m := range re.FindAllStringSubmatchIndex(part, -1) {
			out = append(out, [2]int{from + m[2], from + m[3]})
		}
	}
	return out
}

// The todo keywords a file's headlines may start with: the server's, and the
// file's own #+TODO lines. Only used to keep the keyword out of the prose of
// a headline, so a keyword missed here costs nothing worse than a keyword
// being searched.
func mentionKeywords(lines []string) map[string]bool {
	kw := map[string]bool{}
	add := func(spec string) {
		for _, w := range strings.Fields(spec) {
			if w == "|" {
				continue
			}
			if i := strings.Index(w, "("); i > 0 {
				w = w[:i]
			}
			kw[w] = true
		}
	}
	if c := Conf(); c != nil && c.Server != nil {
		add(c.Server.DefaultTodoStates)
	}
	add("TODO DONE")
	for _, l := range lines {
		if mentionHeadRe.MatchString(l) {
			break
		}
		if m := mentionTodoLineRe.FindStringSubmatch(l); m != nil {
			add(m[1])
		}
	}
	return kw
}

// scanMentionLines reads a file's lines for the parts a mention may be in. A
// walk of the raw text rather than of the parsed document, because what is
// wanted is byte positions on lines - which is what a link is written at - and
// the parser gives an inline node the position of its paragraph.
func scanMentionLines(lines []string, keywords map[string]bool) mentionScan {
	var out mentionScan
	head := -1
	block := ""  // the code block being skipped, "" when not in one
	drawer := "" // the drawer being skipped
	for row, text := range lines {
		// Inside a block or drawer, the only line that matters is its end.
		if block != "" {
			if m := mentionBlockEndRe.FindStringSubmatch(text); m != nil && strings.EqualFold(m[1], block) {
				block = ""
			}
			continue
		}
		if drawer != "" {
			if drawerEndRe.MatchString(text) {
				drawer = ""
			}
			continue
		}
		if m := mentionHeadRe.FindStringSubmatch(text); m != nil {
			head = row
			out.headRows = append(out.headRows, row)
			out.headLvls = append(out.headLvls, len(m[1]))
			// The title: past the stars, the keyword and the priority, short
			// of the tags.
			from := len(m[0])
			for from < len(text) && (text[from] == ' ' || text[from] == '\t') {
				from++
			}
			if sp := strings.IndexAny(text[from:], " \t"); sp > 0 && keywords[text[from:from+sp]] {
				from += sp
				for from < len(text) && (text[from] == ' ' || text[from] == '\t') {
					from++
				}
			}
			to := len(text)
			if loc := mentionTagsRe.FindStringIndex(text[from:]); loc != nil {
				to = from + loc[0]
			}
			if from < to {
				out.lines = append(out.lines, mentionLine{
					row: row, text: text, lower: strings.ToLower(text[from:to]),
					from: from, to: to, head: head, isHead: true,
				})
			}
			continue
		}
		if m := mentionBlockRe.FindStringSubmatch(text); m != nil {
			if mentionCodeBlocks[strings.ToLower(m[1])] {
				block = m[1]
			}
			continue
		}
		if m := drawerOpenRe.FindStringSubmatch(text); m != nil && !strings.EqualFold(m[1], "END") {
			if mentionSkipDrawers[strings.ToUpper(m[1])] {
				drawer = m[1]
			}
			continue
		}
		if drawerEndRe.MatchString(text) || mentionKeywordRe.MatchString(text) ||
			mentionCommentRe.MatchString(text) || mentionFixedRe.MatchString(text) ||
			planningRe.MatchString(text) || mentionClockRe.MatchString(text) ||
			strings.TrimSpace(text) == "" {
			continue
		}
		out.lines = append(out.lines, mentionLine{
			row: row, text: text, lower: strings.ToLower(text),
			from: 0, to: len(text), head: head,
		})
	}
	return out
}

// The rows a heading's subtree covers, headline included.
func (self *mentionScan) subtree(headRow int) (int, int) {
	for i, r := range self.headRows {
		if r != headRow {
			continue
		}
		for j := i + 1; j < len(self.headRows); j++ {
			if self.headLvls[j] <= self.headLvls[i] {
				return headRow, self.headRows[j] - 1
			}
		}
		return headRow, int(^uint(0) >> 1)
	}
	return headRow, headRow
}

// ---------------------------------------------------------------------------
// Matching
// ---------------------------------------------------------------------------

// One term, ready to look for.
type mentionTerm struct {
	term   string
	needle string // its first word, lowercased: a line without it cannot match
	re     *regexp.Regexp
}

func compileMentionTerms(terms []string) []mentionTerm {
	// Longest first, so "Project Apollo" claims its words before "Apollo" can.
	sorted := append([]string{}, terms...)
	sort.SliceStable(sorted, func(a, b int) bool { return len(sorted[a]) > len(sorted[b]) })
	out := []mentionTerm{}
	for _, t := range sorted {
		words := strings.Fields(t)
		if len(words) == 0 {
			continue
		}
		quoted := make([]string, len(words))
		for i, w := range words {
			quoted[i] = regexp.QuoteMeta(w)
		}
		out = append(out, mentionTerm{
			term:   t,
			needle: strings.ToLower(words[0]),
			re:     regexp.MustCompile(`(?i)` + strings.Join(quoted, `\s+`)),
		})
	}
	return out
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

// A match stands on its own when nothing word-like touches either end. Only
// checked at an end that is itself a letter: "C++" ends in a symbol, and the
// character after it says nothing about whether it was written as a word.
func wholeWord(text string, s, e int) bool {
	first, _ := utf8.DecodeRuneInString(text[s:e])
	last, _ := utf8.DecodeLastRuneInString(text[s:e])
	if isWordRune(first) && s > 0 {
		if r, _ := utf8.DecodeLastRuneInString(text[:s]); isWordRune(r) {
			return false
		}
	}
	if isWordRune(last) && e < len(text) {
		if r, _ := utf8.DecodeRuneInString(text[e:]); isWordRune(r) {
			return false
		}
	}
	return true
}

func overlaps(a, b [2]int) bool { return a[0] < b[1] && b[0] < a[1] }

// The matches on one line: start, end and the term each one is.
type lineMatch struct {
	s, e int
	term string
}

func matchMentionLine(l *mentionLine, terms []mentionTerm) []lineMatch {
	var got []lineMatch
	// The spans that are not prose are worked out only for a line a term is
	// on: nine expressions over every line of every file was most of what a
	// first request cost, and almost no line names the target.
	var skips [][2]int
	skipsDone := false
	for _, t := range terms {
		if !strings.Contains(l.lower, t.needle) {
			continue
		}
		for _, m := range t.re.FindAllStringIndex(l.text[l.from:l.to], -1) {
			s, e := l.from+m[0], l.from+m[1]
			if !wholeWord(l.text, s, e) {
				continue
			}
			span := [2]int{s, e}
			bad := false
			if !skipsDone {
				skips, skipsDone = mentionSkips(l.text, l.from, l.to), true
			}
			for _, sk := range skips {
				if overlaps(span, sk) {
					bad = true
					break
				}
			}
			for _, g := range got {
				if overlaps(span, [2]int{g.s, g.e}) {
					bad = true
					break
				}
			}
			if !bad {
				got = append(got, lineMatch{s, e, t.term})
			}
		}
	}
	sort.Slice(got, func(a, b int) bool { return got[a].s < got[b].s })
	return got
}

// ---------------------------------------------------------------------------
// What a target is called
// ---------------------------------------------------------------------------

var (
	titleLinkRe   = regexp.MustCompile(`\[\[(?:[^\]]*)\]\[([^\]]*)\]\]|\[\[([^\]]*)\]\]`)
	titleCookieRe = regexp.MustCompile(`\[\d*/\d*\]|\[\d*%\]|\[#[A-Za-z0-9]\]`)
	titleStampRe  = regexp.MustCompile(`[<\[]\d{4}-\d{2}-\d{2}[^>\]]*[>\]](?:--[<\[][^>\]]*[>\]])?`)
	titleMarkRe   = regexp.MustCompile(`(^|\s)[*/_=~+]+|[*/_=~+]+(\s|$)`)
)

// A link shown the way it reads: its description, or its target when it has
// none.
func linkText(s string) string {
	return titleLinkRe.ReplaceAllStringFunc(s, func(m string) string {
		sm := titleLinkRe.FindStringSubmatch(m)
		if sm[1] != "" {
			return sm[1]
		}
		return sm[2]
	})
}

// A title as words somebody would write in a sentence: no cookie, no
// timestamp, links as their text, emphasis markers off.
func cleanMentionTitle(s string) string {
	s = linkText(s)
	s = titleCookieRe.ReplaceAllString(s, " ")
	s = titleStampRe.ReplaceAllString(s, " ")
	s = mentionTagsRe.ReplaceAllString(s, "")
	for i := 0; i < 2; i++ {
		s = titleMarkRe.ReplaceAllString(s, "$1$2")
	}
	return strings.Join(strings.Fields(s), " ")
}

// Aliases as org-roam writes them: words separated by spaces, with double
// quotes around an alias that has spaces of its own. A plain ALIASES property
// written with commas is read as a list of those instead.
func splitAliases(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if strings.Contains(s, ",") && !strings.Contains(s, `"`) {
		var out []string
		for _, p := range strings.Split(s, ",") {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
		return out
	}
	var out []string
	var cur strings.Builder
	quoted := false
	flush := func() {
		if t := strings.TrimSpace(cur.String()); t != "" {
			out = append(out, t)
		}
		cur.Reset()
	}
	for _, r := range s {
		switch {
		case r == '"':
			if quoted {
				flush()
			}
			quoted = !quoted
		case unicode.IsSpace(r) && !quoted:
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return out
}

// Words that make a heading's name and say nothing about which heading it is.
// A title made of nothing else would match half the database.
var mentionStopWords = map[string]bool{}

func init() {
	for _, w := range strings.Fields(`a an the and or of to in on at for by with from as is it be
		this that these those my our your his her their its me we you he she they i
		notes note todo todos task tasks done next later someday inbox misc miscellaneous
		ideas idea log journal diary meeting meetings agenda home work people links link
		reading archive index readme summary overview introduction intro references
		questions question answers answer plan plans project projects review weekly daily
		monthly today tomorrow yesterday week month year new old other stuff things thing
		general draft drafts list lists items item random scratch temp test tests example
		examples info about details detail setup config settings help faq part section
		chapter page pages appendix background context goals goal status update updates
		changes change issues issue bugs bug fix fixes data file files`) {
		mentionStopWords[w] = true
	}
}

// Whether a name is worth looking for, and why not when it is not.
func guardMentionTerm(term string, alias bool, force bool) (bool, string) {
	n := utf8.RuneCountInString(term)
	if n == 0 {
		return false, ""
	}
	if n < 2 || (!alias && !force && n < 3) {
		return false, "too short to search for"
	}
	if force || alias {
		return true, ""
	}
	letters := false
	common := true
	for _, w := range strings.Fields(strings.ToLower(term)) {
		w = strings.TrimFunc(w, func(r rune) bool { return !isWordRune(r) })
		if w == "" {
			continue
		}
		if strings.IndexFunc(w, unicode.IsLetter) >= 0 {
			letters = true
		}
		if !mentionStopWords[w] {
			common = false
		}
	}
	if !letters {
		return false, "has no words in it"
	}
	if common {
		return false, "too common a word to search for"
	}
	return true, ""
}

// The terms a target is searched for, and the names that were refused.
func mentionTerms(title string, aliases []string, force bool) ([]string, []string) {
	var terms, skipped []string
	seen := map[string]bool{}
	try := func(t string, alias bool) {
		t = cleanMentionTitle(t)
		key := strings.ToLower(t)
		if t == "" || seen[key] {
			return
		}
		seen[key] = true
		if ok, why := guardMentionTerm(t, alias, force); ok {
			terms = append(terms, t)
		} else if why != "" {
			skipped = append(skipped, fmt.Sprintf("%q %s", t, why))
		}
	}
	try(title, false)
	for _, a := range aliases {
		try(a, true)
	}
	return terms, skipped
}

// The properties in the drawer under a heading, keys upper cased, read off the
// file's lines rather than the parsed drawer (see the column-zero note in
// links.go).
func headingPropsIn(lines []string, from, to int) map[string]string {
	props := map[string]string{}
	ps, pe, ok := drawerAt(lines, from, to, "PROPERTIES")
	if !ok {
		return props
	}
	for i := ps + 1; i < pe && i < len(lines); i++ {
		if m := propLineRe.FindStringSubmatch(lines[i]); m != nil {
			props[strings.ToUpper(m[2])] = strings.TrimSpace(m[4])
		}
	}
	return props
}

var fileKeywordRe = regexp.MustCompile(`(?i)^\s*#\+([a-z_]+):\s*(.*?)\s*$`)

// What a file says about itself above its first heading: its keywords and
// the properties in a drawer there, keys upper cased.
func filePreamble(lines []string) (keywords map[string]string, props map[string]string) {
	keywords, props = map[string]string{}, map[string]string{}
	inDrawer := false
	for _, l := range lines {
		if mentionHeadRe.MatchString(l) {
			break
		}
		if inDrawer {
			if drawerEndRe.MatchString(l) {
				inDrawer = false
				continue
			}
			if m := propLineRe.FindStringSubmatch(l); m != nil {
				props[strings.ToUpper(m[2])] = strings.TrimSpace(m[4])
			}
			continue
		}
		if strings.EqualFold(strings.TrimSpace(l), ":PROPERTIES:") {
			inDrawer = true
			continue
		}
		if m := fileKeywordRe.FindStringSubmatch(l); m != nil {
			k := strings.ToUpper(m[1])
			if _, have := keywords[k]; !have {
				keywords[k] = m[2]
			}
		}
	}
	return keywords, props
}

// readOrgLines reads a file the way writeLines will write it back: split on
// newlines, the last newline not counted as an empty line.
func readOrgLines(filename string) ([]string, error) {
	lines, err := readFileLines(filename)
	if err != nil {
		return nil, err
	}
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}
	return lines, nil
}

// describeLinkTarget fills in what a target is called, its aliases and its id.
// sec is nil for a file.
func describeLinkTarget(f *common.OrgFile, sec *org.Section, force bool) common.LinkTarget {
	t := common.LinkTarget{Kind: "file", Filename: f.Filename}
	lines, _ := readOrgLines(f.Filename)
	var aliases []string
	if sec != nil && sec.Headline != nil {
		t.Kind = "heading"
		t.Hash = sec.Hash
		t.Headline = common.GetHeadlineTitle(sec.Headline)
		t.Olp = outlinePath(sec)
		t.Line = sec.Headline.Pos.Row
		t.Title = t.Headline
		if from := sec.Headline.Pos.Row; from >= 0 && from < len(lines) {
			to := subtreeEndRow(lines, from, sec.Headline.Lvl, from)
			props := headingPropsIn(lines, from, to)
			t.Id = props["ID"]
			aliases = append(aliases, splitAliases(props["ROAM_ALIASES"])...)
			aliases = append(aliases, splitAliases(props["ALIASES"])...)
		}
	} else {
		kw, props := filePreamble(lines)
		t.Title = kw["TITLE"]
		if t.Title == "" {
			// No title: the file's name, which is usually what it is about -
			// "kubernetes.org", "machine-learning.org".
			base := strings.TrimSuffix(filepath.Base(f.Filename), filepath.Ext(f.Filename))
			t.Title = strings.NewReplacer("-", " ", "_", " ").Replace(base)
		}
		t.Id = props["ID"]
		aliases = append(aliases, splitAliases(props["ROAM_ALIASES"])...)
		aliases = append(aliases, splitAliases(props["ALIASES"])...)
		aliases = append(aliases, splitAliases(kw["ROAM_ALIASES"])...)
		aliases = append(aliases, splitAliases(kw["ALIASES"])...)
	}
	t.Terms, t.Skipped = mentionTerms(t.Title, aliases, force)
	return t
}

// ---------------------------------------------------------------------------
// Naming the target of a request
// ---------------------------------------------------------------------------

// A hash read out of a query string. A client that put a hash into a query
// without escaping it has had its "+" turned into a space on the way, and a
// hash never has a space in it.
func queryHash(r *http.Request) string {
	return strings.ReplaceAll(strings.TrimSpace(r.URL.Query().Get("hash")), " ", "+")
}

// The heading or file a request is about: a hash, or a file and optionally
// the line (the heading that line is under) or a heading's text in it. A
// line above the file's first heading names the file.
func requestedTarget(r *http.Request) (*common.OrgFile, *org.Section, error) {
	q := r.URL.Query()
	db := GetDb()
	if hash := queryHash(r); hash != "" {
		sec := db.FindByHash(hash)
		if sec == nil || sec.Headline == nil {
			return nil, nil, fmt.Errorf("no heading with that hash")
		}
		f := db.FileFromSection(sec)
		if f == nil {
			return nil, nil, fmt.Errorf("that heading has no file")
		}
		return f, sec, nil
	}
	fname, err := resolveRequestedFile(q.Get("filename"))
	if err != nil {
		return nil, nil, fmt.Errorf("name a heading by hash, or a file: %v", err)
	}
	f := db.FindByFile(fname)
	if f == nil || f.Doc == nil {
		return nil, nil, fmt.Errorf("file not found in the database: %s", fname)
	}
	if ls := strings.TrimSpace(q.Get("line")); ls != "" {
		n, err := strconv.Atoi(ls)
		if err != nil {
			return nil, nil, fmt.Errorf("line is not a number: %s", ls)
		}
		if sec := findHeadingByLine(f, n); sec != nil {
			return f, sec, nil
		}
		return f, nil, nil
	}
	if h := strings.TrimSpace(q.Get("heading")); h != "" {
		sec := findHeadingByText(f, h)
		if sec == nil {
			return nil, nil, fmt.Errorf("no heading %q in %s", h, filepath.Base(fname))
		}
		return f, sec, nil
	}
	return f, nil, nil
}

// ---------------------------------------------------------------------------
// The links around one heading or file
// ---------------------------------------------------------------------------

// The line a link was written on, as a person reads it.
func contextLine(lines []string, row int) string {
	if row < 0 || row >= len(lines) {
		return ""
	}
	s := strings.TrimSpace(lines[row])
	if m := mentionHeadRe.FindString(s); m != "" {
		s = strings.TrimSpace(s[len(m):])
	}
	s = linkText(s)
	if r := []rune(s); len(r) > 240 {
		s = string(r[:239]) + "…"
	}
	return s
}

// The line a link is on, cut at the link. Empty when the link is not on that
// line as written (a link built by a macro, a file changed since it was read).
func contextParts(lines []string, row int, raw string) (string, string, string) {
	if row < 0 || row >= len(lines) {
		return "", "", ""
	}
	line := lines[row]
	i := strings.Index(line, "[["+raw+"]")
	if i < 0 {
		return "", "", ""
	}
	j := strings.Index(line[i:], "]]")
	if j < 0 {
		return "", "", ""
	}
	j += i + 2
	before := line[:i]
	if m := mentionHeadRe.FindString(strings.TrimLeft(before, " \t")); m != "" {
		before = strings.TrimLeft(before, " \t")[len(m):]
	}
	return cutBefore(before), linkText(line[i:j]), cutAfter(line[j:])
}

func QueryLinksAround(f *common.OrgFile, sec *org.Section) common.LinksAround {
	res := common.LinksAround{Ok: true, Target: describeLinkTarget(f, sec, false),
		In: []common.LinkInContext{}, Out: []common.LinkInContext{}}
	idx := getLinkIndex()
	files := map[string][]string{}
	withContext := func(l common.OrgLink) common.LinkInContext {
		lines, ok := files[l.From.Filename]
		if !ok {
			lines, _ = readOrgLines(l.From.Filename)
			files[l.From.Filename] = lines
		}
		c := common.LinkInContext{OrgLink: l, Context: contextLine(lines, l.From.Line)}
		c.Before, c.Shown, c.After = contextParts(lines, l.From.Line, l.Raw)
		return c
	}
	fname := f.Filename
	if sec != nil {
		hash := sec.Hash
		// A heading's links in are every link landing on it. A link that names
		// the file it is in lands on no heading, the way BacklinksTo counts.
		for _, i := range idx.toFile[fname] {
			l := idx.links[i]
			if l.To.Hash == hash && l.From.Hash != hash {
				res.In = append(res.In, withContext(l))
			}
		}
		for _, i := range idx.fromFile[fname] {
			l := idx.links[i]
			if l.From.Hash == hash && l.To.Hash != hash {
				res.Out = append(res.Out, withContext(l))
			}
		}
	} else {
		for _, i := range idx.toFile[fname] {
			if l := idx.links[i]; l.From.Filename != fname {
				res.In = append(res.In, withContext(l))
			}
		}
		for _, i := range idx.fromFile[fname] {
			if l := idx.links[i]; l.To.Filename != fname {
				res.Out = append(res.Out, withContext(l))
			}
		}
	}
	sortInContext(res.In)
	sortInContext(res.Out)
	return res
}

func sortInContext(ls []common.LinkInContext) {
	sort.SliceStable(ls, func(a, b int) bool {
		if ls[a].From.Filename != ls[b].From.Filename {
			return ls[a].From.Filename < ls[b].From.Filename
		}
		return ls[a].From.Line < ls[b].From.Line
	})
}

/* SDOC: API
* GET /links/heading — Links Into And Out Of One Heading

	The backlinks of a single heading, where =/links= answers for a whole
	file: every link written elsewhere that lands on this heading, and every
	link written under it (not under its children) that goes somewhere else.
	Each comes with the line it was written on, links in it shown as their
	text, so a list of them reads as sentences. Asked about a file instead, it
	answers for the file the way =/links= does (links within the file left
	out).

	*Method:* =GET=

	*Query Parameters:*
	| Parameter  | Type   | Required | Description                                                  |
	|------------+--------+----------+--------------------------------------------------------------|
	| =hash=     | string | one of   | The heading, by hash (query-escaped, not base64 again).      |
	| =filename= | string | one of   | A file. With nothing else, the file is the target.           |
	| =line=     | int    | no       | With =filename=: the heading this zero based line is under.  |
	|            |        |          | A line above the first heading names the file.               |
	| =heading=  | string | no       | With =filename=: a heading by its text.                      |

	*Response:* A =LinksAround= object: =Target= (what was asked about, with
	its title, id and the names mentions are searched for), =In= and =Out=,
	each a list of links with a =Context= line.
EDOC */
func RequestLinksAround(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	f, sec, err := requestedTarget(r)
	if err != nil {
		json.NewEncoder(w).Encode(common.LinksAround{Ok: false, Msg: err.Error()})
		return
	}
	json.NewEncoder(w).Encode(QueryLinksAround(f, sec))
}

// ---------------------------------------------------------------------------
// Unlinked mentions
// ---------------------------------------------------------------------------

// What one file contributes: its prose lines, and its sections by headline
// row so a mention can say which heading it is under.
type mentionPart struct {
	file  *common.OrgFile
	scan  mentionScan
	heads map[int]*org.Section
}

var mentionParts = NewFileParts[mentionPart]()

func buildMentionPart(f *common.OrgFile) mentionPart {
	p := mentionPart{file: f, heads: map[int]*org.Section{}}
	lines, err := readOrgLines(f.Filename)
	if err != nil {
		return p
	}
	p.scan = scanMentionLines(lines, mentionKeywords(lines))
	for _, sec := range flattenSections(f) {
		if sec.Headline != nil {
			p.heads[sec.Headline.Pos.Row] = sec
		}
	}
	return p
}

// Answers, kept until the database next changes. The walk is cached per file
// already; this saves the matching too, which is what a panel that asks again
// every time the reader scrolls back to a heading would otherwise redo.
var (
	mentionCacheLock sync.Mutex
	mentionCacheAt   uint64
	mentionCache     = map[string]common.MentionsResult{}
)

// How much of the line either side of a mention comes back with it.
const mentionSide = 90

func cutBefore(s string) string {
	s = strings.TrimLeft(linkText(s), " \t*")
	r := []rune(s)
	if len(r) <= mentionSide {
		return s
	}
	s = string(r[len(r)-mentionSide:])
	if i := strings.IndexAny(s, " \t"); i >= 0 && i < 20 {
		s = s[i+1:]
	}
	return "…" + s
}

func cutAfter(s string) string {
	s = strings.TrimRight(linkText(s), " \t")
	r := []rune(s)
	if len(r) <= mentionSide {
		return s
	}
	s = string(r[:mentionSide])
	if i := strings.LastIndexAny(s, " \t"); i > len(s)-20 && i > 0 {
		s = s[:i]
	}
	return s + "…"
}

func QueryMentions(f *common.OrgFile, sec *org.Section, force bool, limit int) common.MentionsResult {
	db := GetDb()
	key := fmt.Sprintf("%s\x00%s\x00%v\x00%d", f.Filename, func() string {
		if sec != nil {
			return sec.Hash
		}
		return ""
	}(), force, limit)
	mentionCacheLock.Lock()
	if mentionCacheAt != db.ReloadIndex {
		mentionCache = map[string]common.MentionsResult{}
		mentionCacheAt = db.ReloadIndex
	}
	if got, ok := mentionCache[key]; ok {
		mentionCacheLock.Unlock()
		return got
	}
	mentionCacheLock.Unlock()

	res := common.MentionsResult{Ok: true, Target: describeLinkTarget(f, sec, force), Mentions: []common.Mention{}}
	terms := compileMentionTerms(res.Target.Terms)
	if len(terms) == 0 {
		res.Msg = "nothing to search for"
		if len(res.Target.Skipped) > 0 {
			res.Msg = "not searched: " + strings.Join(res.Target.Skipped, "; ")
		}
	}
	parts := mentionParts.All(buildMentionPart)
	// In file order, so the answer comes out sorted and everything past the
	// limit only has to be counted, not built.
	sort.SliceStable(parts, func(a, b int) bool {
		if parts[a].file == nil || parts[b].file == nil {
			return parts[b].file == nil && parts[a].file != nil
		}
		return parts[a].file.Filename < parts[b].file.Filename
	})
	for _, p := range parts {
		if p.file == nil || len(terms) == 0 {
			continue
		}
		// The target itself is not a mention of itself: a file's own text, or a
		// heading's subtree.
		lo, hi := -1, -1
		if p.file.Filename == f.Filename {
			if sec == nil || sec.Headline == nil {
				continue
			}
			lo, hi = p.scan.subtree(sec.Headline.Pos.Row)
		}
		for i := range p.scan.lines {
			l := &p.scan.lines[i]
			if l.row >= lo && l.row <= hi {
				continue
			}
			for _, m := range matchMentionLine(l, terms) {
				res.Total++
				if limit > 0 && len(res.Mentions) >= limit {
					continue
				}
				men := common.Mention{
					Filename: p.file.Filename, Line: l.row, Col: m.s,
					Text: l.text[m.s:m.e], Term: m.term,
					Before: cutBefore(l.text[l.from:m.s]), After: cutAfter(l.text[m.e:l.to]),
					InHeadline: l.isHead,
				}
				if h := p.heads[l.head]; h != nil {
					men.Hash = h.Hash
					men.Headline = common.GetHeadlineTitle(h.Headline)
					men.Olp = outlinePath(h)
				}
				res.Mentions = append(res.Mentions, men)
			}
		}
	}
	res.Truncated = res.Total > len(res.Mentions)

	mentionCacheLock.Lock()
	if mentionCacheAt == db.ReloadIndex {
		mentionCache[key] = res
	}
	mentionCacheLock.Unlock()
	return res
}

/* SDOC: API
* GET /mentions — Unlinked Mentions Of A Heading Or File

	Every place a heading's title or one of its aliases (or a file's
	=#+TITLE:=) is written in plain text without a link to it. Matching is
	case insensitive and on whole words; links, code and src/example blocks,
	property drawers, logbooks, keyword lines and comments are not searched,
	nor is the target itself. A title made only of common words ("Notes",
	"Meeting") is not searched unless =all= is given; =Target.Skipped= says
	which names were left out and why.

	*Method:* =GET=

	*Query Parameters:*
	| Parameter  | Type   | Required | Description                                                  |
	|------------+--------+----------+--------------------------------------------------------------|
	| =hash=     | string | one of   | The heading, by hash.                                        |
	| =filename= | string | one of   | A file; with =line= or =heading=, a heading in it, as        |
	|            |        |          | =/links/heading= takes them.                                 |
	| =all=      | bool   | no       | Search for a title even when it is only common words.        |
	| =limit=    | int    | no       | At most this many mentions. Defaults to 500; 0 is no limit.  |

	*Response:* A =MentionsResult=: =Target=, then =Mentions=, each with the
	file, zero based =Line=, the byte =Col= the text starts at, the =Text= as
	written, the =Before= and =After= of the line around it, and the heading
	it is under. =Total= is how many there were before the limit.
EDOC */
func RequestMentions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	f, sec, err := requestedTarget(r)
	if err != nil {
		json.NewEncoder(w).Encode(common.MentionsResult{Ok: false, Msg: err.Error()})
		return
	}
	q := r.URL.Query()
	force := q.Get("all") != "" && q.Get("all") != "false" && q.Get("all") != "0"
	limit := 500
	if ls := q.Get("limit"); ls != "" {
		if n, err := strconv.Atoi(ls); err == nil && n >= 0 {
			limit = n
		}
	}
	json.NewEncoder(w).Encode(QueryMentions(f, sec, force, limit))
}

// ---------------------------------------------------------------------------
// Turning mentions into links
// ---------------------------------------------------------------------------

// Replace each mention with a link, in the lines of one file. The mentions are
// applied from the bottom of the file up and from the right of a line to its
// left, so that writing one never moves another. Each is checked against the
// line first: a file that changed since the mention was found is refused
// rather than written into the wrong place.
func linkMentionLines(lines []string, refs []common.MentionRef, link func(text string) string) ([]string, []common.MentionLinkOutcome) {
	order := make([]int, len(refs))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		ra, rb := refs[order[a]], refs[order[b]]
		if ra.Line != rb.Line {
			return ra.Line > rb.Line
		}
		return ra.Col > rb.Col
	})
	out := make([]common.MentionLinkOutcome, len(refs))
	for _, i := range order {
		ref := refs[i]
		o := common.MentionLinkOutcome{MentionRef: ref}
		switch {
		case ref.Text == "":
			o.Msg = "no text to link"
		case ref.Line < 0 || ref.Line >= len(lines):
			o.Msg = fmt.Sprintf("line %d is not in the file any more", ref.Line+1)
		case ref.Col < 0 || ref.Col+len(ref.Text) > len(lines[ref.Line]) ||
			lines[ref.Line][ref.Col:ref.Col+len(ref.Text)] != ref.Text:
			o.Msg = fmt.Sprintf("%q is no longer at line %d, column %d - the file has changed since; look again", ref.Text, ref.Line+1, ref.Col+1)
		default:
			line := lines[ref.Line]
			span := [2]int{ref.Col, ref.Col + len(ref.Text)}
			for _, sk := range mentionSkips(line, 0, len(line)) {
				if overlaps(span, sk) {
					o.Msg = fmt.Sprintf("%q at line %d is inside a link or code now", ref.Text, ref.Line+1)
					break
				}
			}
			if o.Msg != "" {
				break
			}
			o.Link = link(ref.Text)
			lines[ref.Line] = line[:span[0]] + o.Link + line[span[1]:]
			o.Ok = true
		}
		out[i] = o
	}
	return lines, out
}

// The id a heading is linked by, given one as a line edit when it has none:
// a :PROPERTIES: drawer made if need be, and an :ID: in it - a uuid, which is
// what org-id makes. Answers with the row the edit was made under and how
// many lines it added, so mentions further down the same file can be moved.
func ensureHeadingId(filename string, sec *org.Section) (id string, minted bool, row int, delta int, err error) {
	lines, from, to, ok := recordLines(filename, sec)
	if !ok {
		return "", false, 0, 0, fmt.Errorf("could not read the heading in %s", filename)
	}
	if id := headingPropsIn(lines, from, to)["ID"]; id != "" {
		return id, false, from, 0, nil
	}
	before := len(lines)
	id = uuid.New().String()
	ind := indentOf(sec.Headline.Lvl)
	var ps, pe int
	lines, ps, pe, _ = ensurePropertyDrawer(lines, from, to, ind)
	lines, _ = setPropIn(lines, ps, pe, ind, "ID", id)
	if err := writeLines(filename, lines); err != nil {
		return "", false, 0, 0, err
	}
	return id, true, from, len(lines) - before, nil
}

// A path from one org file to another, as a file: link writes it.
func fileLinkPath(from, to string) string {
	if rel, err := filepath.Rel(filepath.Dir(from), to); err == nil {
		if !strings.HasPrefix(rel, ".") {
			rel = "./" + rel
		}
		return filepath.ToSlash(rel)
	}
	return to
}

// org's description cannot hold "]]"; a mention never should, but a client
// sending text the server did not find could.
func linkDesc(s string) string { return strings.ReplaceAll(s, "]", "}") }

func LinkMentions(req common.MentionLinkRequest) (common.MentionLinkResult, error) {
	res := common.MentionLinkResult{}
	if len(req.Mentions) == 0 {
		return res, fmt.Errorf("no mentions to link")
	}
	db := GetDb()

	// The target, and how a link to it is written.
	var sec *org.Section
	var tfile string
	fileId := ""
	if h := strings.TrimSpace(req.Hash); h != "" {
		sec = db.FindByHash(h)
		if sec == nil || sec.Headline == nil {
			return res, fmt.Errorf("no heading with that hash")
		}
		f := db.FileFromSection(sec)
		if f == nil {
			return res, fmt.Errorf("that heading has no file")
		}
		tfile = f.Filename
	} else {
		fname, err := resolveRequestedFile(req.Filename)
		if err != nil {
			return res, err
		}
		tfile = fname
		lines, _ := readOrgLines(fname)
		_, props := filePreamble(lines)
		fileId = props["ID"]
	}

	// Which file each mention is in. Only files the database holds are
	// written, whatever a request names.
	byFile := map[string][]int{}
	var files []string
	res.Results = make([]common.MentionLinkOutcome, len(req.Mentions))
	for i, m := range req.Mentions {
		res.Results[i] = common.MentionLinkOutcome{MentionRef: m}
		f := db.FindByFile(m.Filename)
		if f == nil {
			res.Results[i].Msg = "not a file the server holds: " + m.Filename
			continue
		}
		if _, have := byFile[f.Filename]; !have {
			files = append(files, f.Filename)
		}
		byFile[f.Filename] = append(byFile[f.Filename], i)
	}

	// Check every mention before writing anything, the id included: an id
	// minted for links that then all turn out stale is a change to somebody's
	// file for nothing.
	anyGood := false
	for _, fname := range files {
		lines, err := readOrgLines(fname)
		if err != nil {
			for _, i := range byFile[fname] {
				res.Results[i].Msg = err.Error()
			}
			continue
		}
		refs := make([]common.MentionRef, len(byFile[fname]))
		for k, i := range byFile[fname] {
			refs[k] = req.Mentions[i]
		}
		_, outs := linkMentionLines(lines, refs, func(t string) string { return t })
		for k, i := range byFile[fname] {
			res.Results[i].Msg = outs[k].Msg
			anyGood = anyGood || outs[k].Ok
		}
	}
	if !anyGood {
		res.Msg = "nothing was linked"
		for _, o := range res.Results {
			if o.Msg != "" {
				res.Msg = o.Msg
				break
			}
		}
		return res, nil
	}

	var link func(text string) string
	shiftRow, shiftBy := -1, 0
	if sec != nil {
		id, minted, row, delta, err := ensureHeadingId(tfile, sec)
		if err != nil {
			return res, err
		}
		res.Id, res.NewId = id, minted
		shiftRow, shiftBy = row, delta
		link = func(t string) string { return "[[id:" + id + "][" + linkDesc(t) + "]]" }
	} else if fileId != "" {
		res.Id = fileId
		link = func(t string) string { return "[[id:" + fileId + "][" + linkDesc(t) + "]]" }
	}

	for _, fname := range files {
		lines, err := readOrgLines(fname)
		if err != nil {
			continue
		}
		refs := make([]common.MentionRef, len(byFile[fname]))
		for k, i := range byFile[fname] {
			refs[k] = req.Mentions[i]
			// The drawer the id went into sits under the target's headline, so
			// a mention below it in the same file is that many lines lower now.
			if fname == tfile && shiftBy != 0 && refs[k].Line > shiftRow {
				refs[k].Line += shiftBy
			}
		}
		mk := link
		if mk == nil {
			path := fileLinkPath(fname, tfile)
			mk = func(t string) string { return "[[file:" + path + "][" + linkDesc(t) + "]]" }
		}
		lines, outs := linkMentionLines(lines, refs, mk)
		wrote := false
		for k, i := range byFile[fname] {
			outs[k].MentionRef = req.Mentions[i]
			res.Results[i] = outs[k]
			if outs[k].Ok {
				wrote = true
			}
		}
		if !wrote {
			continue
		}
		if err := writeLines(fname, lines); err != nil {
			for _, i := range byFile[fname] {
				if res.Results[i].Ok {
					res.Results[i].Ok = false
					res.Results[i].Msg = err.Error()
				}
			}
			continue
		}
		for _, i := range byFile[fname] {
			if res.Results[i].Ok {
				res.Linked++
			}
		}
	}
	res.Ok = res.Linked > 0
	switch {
	case res.Linked == len(req.Mentions):
		res.Msg = fmt.Sprintf("linked %d", res.Linked)
	case res.Linked > 0:
		res.Msg = fmt.Sprintf("linked %d of %d", res.Linked, len(req.Mentions))
	default:
		res.Msg = "nothing was linked"
	}
	return res, nil
}

/* SDOC: API
* POST /mentions/link — Turn Mentions Into Links

	Rewrites unlinked mentions as links, each as an edit of its own line and
	nothing else. A heading target is linked by id - =[[id:<ID>][text]]=, the
	text kept as written - and is given an =:ID:= (a uuid, as org-id makes)
	in its property drawer first if it has none. A file target is linked by
	the id in the drawer above its first heading if it has one, otherwise by
	a relative =file:= link.

	Each mention is checked against its line before it is written: if the
	text is no longer at that line and column, or now sits inside a link, it
	is refused with a message and the others still go ahead. Every mention is
	checked before anything is written, so a request whose mentions have all
	gone stale changes nothing - not even the id. Each file is written once.

	Linking a mention that is in a headline changes that headline's text, and
	with it the hash of that heading and every heading under it.

	*Method:* =POST=

	*Request Body:* A =MentionLinkRequest=: =Hash= (a heading) or =Filename=
	(a file) for the target, and =Mentions=, a list of ={Filename, Line, Col,
	Text}= exactly as =/mentions= answered them.

	*Response:* A =MentionLinkResult=: =Ok= when anything was linked,
	=Linked=, the =Id= used, =NewId= when the heading was given one, and a
	=Results= entry per mention with its own =Ok=, =Msg= and the =Link=
	written.
EDOC */
func PostLinkMentions(w http.ResponseWriter, r *http.Request) {
	var req common.MentionLinkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		answerErr(w, err)
		return
	}
	res, err := LinkMentions(req)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(common.MentionLinkResult{Ok: false, Msg: err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}
