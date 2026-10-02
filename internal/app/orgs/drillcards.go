package orgs

// Flashcards: reading cards out of the org files, and writing ratings back.
//
// A card is a heading a deck's query found. What it shows is org-drill's: the
// heading's own body is the question (with any [cloze] hidden), and its direct
// child headings are the answer - or, for a two- or many-sided card, one child
// is the question and the rest the answer. The cards are read off the file's
// lines rather than the parse tree, because a card with drill data has a
// property drawer, and a drawer in column zero makes go-org hoist the heading's
// body out of it (see **Traps: column-zero drawers**).
//
// Each card comes back rendered once, with every cloze wrapped in a span that
// carries its number and hint. Which clozes to hide and which side to ask is
// chosen per review by the client (worg/src/drill.ts), because org-drill
// chooses afresh every time a card is shown and a request per showing would be
// a pause per card.
//
// A rating is one request and one line edit of the card's own lines: the
// DRILL_* properties, the SCHEDULED stamp and, after enough failures, a
// `leech` tag - exactly what Emacs writes (see **Traps: line edits**).

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"math"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

// ── What the client is sent ─────────────────────────────────────────────────

type DrillSide = common.DrillSide

type DrillCard = common.DrillCard

type DrillCounts = common.DrillCounts

type DrillCardsReply = common.DrillCardsReply

// The card types org-drill knows. Only twosided and multisided may have an
// empty body: their question is a side.
var drillTypes = map[string]bool{
	"simple": true, "simpletyped": true, "twosided": true, "multisided": true,
	"hide1cloze": true, "multicloze": true, "hide2cloze": true, "show1cloze": true,
	"show2cloze": true, "hidefirst": true, "hidelast": true, "hide1_firstmore": true,
	"show1_lastmore": true, "show1_firstless": true, "conjugate": true,
	"decline_noun": true, "spanish_verb": true,
}

var drillEmptyOk = map[string]bool{"twosided": true, "multisided": true, "conjugate": true, "decline_noun": true}

// The properties the generated card types read, with inheritance.
var drillTypeProps = []string{
	"VERB_INFINITIVE", "VERB_INFINITIVE_HINT", "VERB_TRANSLATION", "VERB_TENSE", "VERB_MOOD",
	"NOUN", "NOUN_HINT", "NOUN_ROOT", "NOUN_GENDER", "NOUN_TRANSLATION", "DECLINE_DEFINITE", "DECLINE_PLURAL",
}

// ── Finding the cards ───────────────────────────────────────────────────────

type drillHit struct {
	sec  *org.Section
	file *common.OrgFile
}

// drillSections runs a deck's query, answering with every heading it found in
// document order - parents before children - and which of them have a found
// heading somewhere under them. Which of those are cards is decided once their
// text has been read (see RequestDrillCards).
func drillSections(query string) ([]drillHit, map[*org.Section]bool, error) {
	ctx := Conf().PlugManager.Tempo.GetAugmentedStandardContextFromStringMap(Conf().Filters, true)
	expanded := Conf().PlugManager.Tempo.ExecuteTemplateString(query, ctx)
	exp, err := ParseString(&common.StringQuery{Query: expanded})
	if err != nil {
		return nil, nil, err
	}
	registerAllSections()
	files, _ := filesForQuery(expanded)
	hits := []drillHit{}
	above := map[*org.Section]bool{}
	for _, name := range files {
		f := GetDb().GetFile(name)
		if f == nil || f.Doc == nil {
			continue
		}
		var nodes []*org.Section
		for _, v := range f.Doc.Outline.Children {
			nodes, _ = EvalForNodes(exp, v, f, nodes)
		}
		for _, s := range nodes {
			if s.Headline == nil {
				continue
			}
			hits = append(hits, drillHit{sec: s, file: f})
			for p := s.Parent; p != nil; p = p.Parent {
				above[p] = true
			}
		}
	}
	return hits, above, nil
}

// ── Reading one card ────────────────────────────────────────────────────────

var (
	scheduledRe = regexp.MustCompile(`SCHEDULED:\s*<(\d{4}-\d{2}-\d{2})[^>]*>`)
	reviewedRe  = regexp.MustCompile(`\[(\d{4}-\d{2}-\d{2})(?:\s+\w+)?(?:\s+(\d{1,2}:\d{2}))?`)
	fileProp    = regexp.MustCompile(`(?i)^#\+PROPERTY:\s+(\S+)\s+(.*)$`)
)

// cardFile is one file's lines, read once per request however many cards it
// holds.
type cardFile struct {
	lines []string
	props map[string]string // #+PROPERTY: lines
}

func readCardFile(cache map[string]*cardFile, name string) *cardFile {
	if cf, ok := cache[name]; ok {
		return cf
	}
	b, err := os.ReadFile(name)
	if err != nil {
		cache[name] = nil
		return nil
	}
	cf := &cardFile{lines: strings.Split(strings.TrimSuffix(string(b), "\n"), "\n"), props: map[string]string{}}
	for _, l := range cf.lines {
		if m := fileProp.FindStringSubmatch(strings.TrimSpace(l)); m != nil {
			cf.props[strings.ToUpper(m[1])] = strings.TrimSpace(m[2])
		}
	}
	cache[name] = cf
	return cf
}

// headingRange is a heading's own lines: its row, where its own body ends
// (before the first child) and where its subtree ends.
func headingRange(lines []string, row, lvl int) (ownEnd, end int) {
	end = subtreeEndRow(lines, row, lvl, row)
	if end >= len(lines) {
		end = len(lines) - 1
	}
	ownEnd = end
	for i := row + 1; i <= end; i++ {
		if n := headingStars(lines[i]); n > lvl {
			ownEnd = i - 1
			break
		}
	}
	return ownEnd, end
}

func headingStars(line string) int {
	n := 0
	for n < len(line) && line[n] == '*' {
		n++
	}
	if n > 0 && n < len(line) && line[n] == ' ' {
		return n
	}
	return 0
}

// proseOf is a run of lines with the bookkeeping taken out: planning lines and
// every drawer, which org-drill folds away while a card is shown.
func proseOf(lines []string, from, to int) string {
	out := []string{}
	inSrc := false
	for i := from; i <= to && i < len(lines); i++ {
		l := lines[i]
		t := strings.ToLower(strings.TrimSpace(l))
		if strings.HasPrefix(t, "#+begin_") {
			inSrc = true
		} else if strings.HasPrefix(t, "#+end_") {
			inSrc = false
		}
		if !inSrc && planningRe.MatchString(l) {
			continue
		}
		if !inSrc && drawerOpenRe.MatchString(l) && !drawerEndRe.MatchString(l) {
			j := i + 1
			for j <= to && j < len(lines) && !drawerEndRe.MatchString(lines[j]) {
				j++
			}
			if j <= to && j < len(lines) {
				i = j
				continue
			}
		}
		out = append(out, l)
	}
	return strings.Trim(strings.Join(out, "\n"), "\n")
}

// ── Clozes ──────────────────────────────────────────────────────────────────

// clozes wraps every cloze in a card's org text in an html span carrying its
// number, hint and length, for the client to hide or show (see
// common.Clozes for the syntax and the numbering).
func clozes(text string, first int) (string, int) {
	return common.Clozes(text, first, func(c common.Cloze) string {
		return fmt.Sprintf(`@@html:<span class="drill-cloze" data-n="%d" data-hint="%s" data-len="%d">@@%s@@html:</span>@@`,
			c.N, html.EscapeString(c.Hint), len([]rune(c.Text)), c.Text)
	})
}

// ── Rendering ───────────────────────────────────────────────────────────────

func drillRender(text, path string) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}
	doc := org.New().Parse(strings.NewReader(text+"\n"), path)
	if exp := htmlExporter(); exp != nil {
		return exp.RenderFragmentIn(doc, doc.Nodes...)
	}
	w := org.NewHTMLWriter()
	w.Document = doc
	org.WriteNodes(w, doc.Nodes...)
	return w.String()
}

var outerP = regexp.MustCompile(`(?s)^\s*<p>\s*(.*?)\s*</p>\s*$`)

// drillInline renders a title: inline markup, no paragraph around it.
func drillInline(text, path string) string {
	h := drillRender(text, path)
	if m := outerP.FindStringSubmatch(h); m != nil {
		return m[1]
	}
	return h
}

// ── Building a card ─────────────────────────────────────────────────────────

func buildCard(hit drillHit, cf *cardFile, deck DrillDeck, matrix DrillMatrix, cram bool, raw bool, now time.Time) (DrillCard, string, bool) {
	// raw is format=org: the text as written, for a client that draws org
	// itself (`orgs drill`) and finds the clozes with common.Clozes.
	wrap := func(text string, first int) (string, int) {
		if raw {
			_, n := clozes(text, first)
			return text, n
		}
		return clozes(text, first)
	}
	block := func(text, path string) string {
		if raw {
			return text
		}
		return drillRender(text, path)
	}
	inline := func(text, path string) string {
		if raw {
			return text
		}
		return drillInline(text, path)
	}
	s := hit.sec
	h := s.Headline
	row, lvl := h.Pos.Row, h.Lvl
	lines := cf.lines
	path := hit.file.Filename
	if row < 0 || row >= len(lines) {
		return DrillCard{}, "lost its place in " + path, false
	}
	ownEnd, end := headingRange(lines, row, lvl)
	props := propsFromLines(lines, row, ownEnd)
	title := strings.TrimSpace(org.String(h.Title...))
	if m := headlineRe.FindStringSubmatch(lines[row]); m != nil {
		title = strings.TrimSpace(m[4])
	}
	c := DrillCard{
		Hash: s.Hash, File: path, Line: row + 1, Name: title,
		Tags: h.Tags, LastQuality: -1,
	}
	inherited := func(key string) string {
		if v, ok := props[key]; ok {
			return v
		}
		for p := s.Parent; p != nil; p = p.Parent {
			if p.Headline == nil {
				continue
			}
			_, pe := headingRange(lines, p.Headline.Pos.Row, p.Headline.Lvl)
			pp := propsFromLines(lines, p.Headline.Pos.Row, pe)
			if v, ok := pp[key]; ok {
				return v
			}
		}
		return cf.props[key]
	}
	c.Type = strings.ToLower(strings.TrimSpace(inherited("DRILL_CARD_TYPE")))
	if c.Type == "" || c.Type == "nil" {
		c.Type = "simple"
	}
	if !drillTypes[c.Type] {
		return c, fmt.Sprintf("%s (%s:%d): unrecognised card type %q", title, path, row+1, c.Type), false
	}

	// The question, and whether there is one.
	body := proseOf(lines, row+1, ownEnd)
	if strings.TrimSpace(body) == "" && !drillEmptyOk[c.Type] {
		return c, fmt.Sprintf("%s (%s:%d): nothing to ask - the card has no body", title, path, row+1), true
	}
	body, c.Clozes = wrap(body, 1)
	c.Body = block(body, path)
	t, _ := wrap(title, 0)
	c.Title = inline(t, path)

	// The sides: each direct child, rendered with its own subtree.
	for i := ownEnd + 1; i <= end && i < len(lines); i++ {
		if headingStars(lines[i]) != lvl+1 {
			continue
		}
		_, ce := headingRange(lines, i, lvl+1)
		name := strings.TrimSpace(strings.TrimLeft(lines[i], "* "))
		if m := headlineRe.FindStringSubmatch(lines[i]); m != nil {
			name = strings.TrimSpace(m[4])
		}
		sideBody, _ := wrap(proseOf(lines, i+1, ce), 0)
		ct, _ := wrap(name, 0)
		// A child that has been drilled as a card in its own right stays
		// folded in the answer, as org-drill leaves a child card folded.
		// Found-by-the-query is no test: tags are inherited, so every child
		// of a card is found.
		cp := propsFromLines(lines, i, ce)
		_, drilled := cp["DRILL_TOTAL_REPEATS"]
		_, typed := cp["DRILL_CARD_TYPE"]
		c.Sides = append(c.Sides, DrillSide{Name: name, Title: inline(ct, path), Body: block(sideBody, path), IsCard: drilled || typed})
		i = ce
	}

	// The explanation: org-drill's :explain: tag.
	explained := hasTag(h.Tags, "explain")
	for p := s.Parent; p != nil && !explained; p = p.Parent {
		if p.Headline != nil && hasTag(p.Headline.Tags, "explain") {
			explained = true
		}
	}
	if explained {
		chain := []string{}
		for p := s.Parent; p != nil && p.Headline != nil && hasTag(p.Headline.Tags, "explain"); p = p.Parent {
			po, _ := headingRange(lines, p.Headline.Pos.Row, p.Headline.Lvl)
			chain = append([]string{block(proseOf(lines, p.Headline.Pos.Row+1, po), path)}, chain...)
			if p.Headline.Lvl <= 1 {
				break
			}
		}
		c.Explain = chain
	}
	if c.Type == "conjugate" || c.Type == "decline_noun" || c.Type == "spanish_verb" {
		c.Props = map[string]string{}
		for _, k := range drillTypeProps {
			if v := inherited(k); v != "" {
				c.Props[k] = strings.Trim(v, `"`)
			}
		}
	}

	// Where it stands.
	c.Data = readDrillData(props)
	if q, err := strconv.Atoi(strings.TrimSpace(props["DRILL_LAST_QUALITY"])); err == nil {
		c.LastQuality = q
	}
	if w, err := strconv.ParseFloat(strings.TrimSpace(props["DRILL_CARD_WEIGHT"]), 64); err == nil {
		c.Weight = w
	}
	c.Leech = hasTag(h.Tags, "leech")
	var reviewed *time.Time
	if m := reviewedRe.FindStringSubmatch(props["DRILL_LAST_REVIEWED"]); m != nil {
		layout, v := "2006-01-02", m[1]
		if m[2] != "" {
			layout, v = "2006-01-02 15:04", m[1]+" "+m[2]
		}
		if t, err := time.ParseInLocation(layout, v, time.Local); err == nil {
			reviewed = &t
			c.LastReviewed = props["DRILL_LAST_REVIEWED"]
		}
	}
	scheduled := false
	for i := row + 1; i <= ownEnd && i < len(lines) && planningRe.MatchString(lines[i]); i++ {
		if m := scheduledRe.FindStringSubmatch(lines[i]); m != nil {
			if t, err := time.ParseInLocation("2006-01-02", m[1], time.Local); err == nil {
				scheduled = true
				c.Scheduled = m[1]
				c.Due = daysBetween(t, now)
			}
		}
	}
	_, hasInterval := props["DRILL_LAST_INTERVAL"]
	c.Status = drillStatus(deck, scheduled, c.Due, c.LastQuality, c.Data.LastInterval, hasInterval, c.Leech, cram, reviewed, now)
	c.Age = float64(c.Due) + c.Data.LastInterval
	if added := reviewedRe.FindStringSubmatch(props["DATE_ADDED"]); added != nil {
		if t, err := time.ParseInLocation("2006-01-02", added[1], time.Local); err == nil {
			c.Age = float64(daysBetween(t, now))
		}
	}
	c.Next = nextReviewDays(c.Data, deck, matrix, c.Weight, c.Hash, now)
	return c, "", false
}

func hasTag(tags []string, want string) bool {
	for _, t := range tags {
		if strings.EqualFold(t, want) {
			return true
		}
	}
	return false
}

// ── Decks, and which one a request means ────────────────────────────────────

func (self *ExtensionsConfig) GetDrillDecks(username string) []DrillDeck {
	self.mu.RLock()
	defer self.mu.RUnlock()
	if u, ok := self.Users[username]; ok && u.DrillDecks != nil {
		return u.DrillDecks
	}
	return []DrillDeck{}
}

func (self *ExtensionsConfig) SetDrillDecks(username string, decks []DrillDeck) error {
	self.mu.Lock()
	defer self.mu.Unlock()
	if decks == nil {
		decks = []DrillDeck{}
	}
	self.getUser(username).DrillDecks = decks
	return self.save()
}

func (self *ExtensionsConfig) drillMatrix(username string) DrillMatrix {
	self.mu.RLock()
	defer self.mu.RUnlock()
	if u, ok := self.Users[username]; ok && u.DrillMatrix != nil {
		return u.DrillMatrix.clone()
	}
	return DrillMatrix{}
}

func (self *ExtensionsConfig) setDrillMatrix(username string, m DrillMatrix) error {
	self.mu.Lock()
	defer self.mu.Unlock()
	self.getUser(username).DrillMatrix = m
	return self.save()
}

// requestDeck is the deck a request names - a saved deck by name, or an
// unsaved one described in the query string - and the query that finds its
// cards.
func requestDeck(r *http.Request, username string) (DrillDeck, string, error) {
	q := r.URL.Query()
	var deck DrillDeck
	if name := q.Get("deck"); name != "" {
		found := false
		for _, d := range GetExtensions().GetDrillDecks(username) {
			if d.Name == name {
				deck, found = d, true
			}
		}
		if !found {
			return deck, "", fmt.Errorf("no deck named %q", name)
		}
	} else {
		deck = DrillDeck{Name: "", Query: q.Get("query"), Algorithm: q.Get("algorithm")}
	}
	deck = deck.WithDefaults()
	query := deck.Query
	if deck.StoredQuery != "" {
		sq := GetExtensions().GetStoredQuery(username, deck.StoredQuery)
		if sq == nil {
			return deck, "", fmt.Errorf("the deck's saved search %q no longer exists", deck.StoredQuery)
		}
		query = sq.Query
	}
	if strings.TrimSpace(query) == "" {
		return deck, "", fmt.Errorf("the deck has no query: say which headings are its cards")
	}
	return deck, query, nil
}

// ── REST ────────────────────────────────────────────────────────────────────

/* SDOC: API
* GET /drill/cards — The Cards of a Flashcard Deck
	Runs a deck's query and answers with every card it found, each with its
	question and answer drawn as html, where it stands (new, failed, overdue,
	young, old, or not due), and how many days each rating 0-5 would put it
	away for. This is worg's Drill tab's one read; the session itself - which
	card next, the again pile, the limits - runs in the browser.

	*Method:* =GET=

	*Query Parameters:*
	| Parameter   | Description                                                      |
	|-------------+------------------------------------------------------------------|
	| =deck=      | A saved deck's name (see =/ext/drill/decks=).                     |
	| =query=     | Or a query expression, for a deck that is not saved.              |
	| =algorithm= | With =query=: =sm5= (default), =sm2= or =simple8=.                |
	| =cram=      | =1= for cram mode: everything not seen in the last 12 hours is due. |
	| =theme=     | The html theme to draw the cards in; the reply's =style= is its css. |
	| =format=    | =org= for the cards' org text instead of html (=orgs drill= draws it). |

	*Response:* ={ok, msg, deck, query, cards: [DrillCard], counts, problems, style}=.
	A card that cannot be drilled (an unknown =DRILL_CARD_TYPE=, an empty body)
	is left out and named in =problems=.
	EDOC */
func RequestDrillCards(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	username := GetUsername(r)
	deck, query, err := requestDeck(r, username)
	if err != nil {
		json.NewEncoder(w).Encode(DrillCardsReply{Ok: false, Msg: err.Error(), Deck: deck})
		return
	}
	hits, above, err := drillSections(query)
	if err != nil {
		json.NewEncoder(w).Encode(DrillCardsReply{Ok: false, Msg: "the query: " + err.Error(), Deck: deck, Query: query})
		return
	}
	cram := r.URL.Query().Get("cram") == "1"
	raw := r.URL.Query().Get("format") == "org"
	now := time.Now()
	matrix := GetExtensions().drillMatrix(username)
	cache := map[string]*cardFile{}
	reply := DrillCardsReply{Ok: true, Deck: deck, Query: query, Cards: []DrillCard{}}
	// A found heading is a card unless a card above it was found too - then
	// it is that card's answer or side, the way org-drill takes only the
	// heading carrying the tag itself. A found heading with nothing to ask
	// and found headings under it is a heading that groups cards: queries
	// match by inherited tags, so `HasTags("spanish")` finds the
	// `* Vocabulary :spanish:` heading as well as the words under it.
	accepted := map[*org.Section]bool{}
	for _, h := range hits {
		inside := false
		for p := h.sec.Parent; p != nil; p = p.Parent {
			if accepted[p] {
				inside = true
				break
			}
		}
		if inside {
			continue
		}
		cf := readCardFile(cache, h.file.Filename)
		if cf == nil {
			continue
		}
		c, problem, empty := buildCard(h, cf, deck, matrix, cram, raw, now)
		if problem != "" {
			if !(empty && above[h.sec]) {
				reply.Problems = append(reply.Problems, problem)
			}
			continue
		}
		accepted[h.sec] = true
		reply.Cards = append(reply.Cards, c)
		reply.Counts.Total++
		switch c.Status {
		case DrillNew:
			reply.Counts.New++
		case DrillFailed:
			reply.Counts.Failed++
		case DrillOverdue:
			reply.Counts.Overdue++
		case DrillYoung:
			reply.Counts.Young++
		case DrillOld:
			reply.Counts.Old++
		case DrillSkipped:
			reply.Counts.Skipped++
		case DrillFuture:
			reply.Counts.Future++
			if c.Due == -1 {
				reply.Counts.DueTomorrow++
			}
		}
	}
	theme := r.URL.Query().Get("theme")
	if deck.Theme != "" {
		theme = deck.Theme
	}
	if exp := htmlExporter(); exp != nil && theme != "" {
		reply.Style = exp.ThemeStyle(theme)
	}
	json.NewEncoder(w).Encode(reply)
}

type DrillReview = common.DrillReview

type DrillReviewReply = common.DrillReviewReply

/* SDOC: API
* POST /drill/review — Rate a Flashcard
	Records how well a card was remembered and schedules its next review, the
	way org-drill does: the =DRILL_*= properties are rewritten, =SCHEDULED= is
	set to the next review (or taken off, after a failure, so the card is asked
	again next session), and a card failed more often than the deck's leech
	threshold is tagged =leech=. Only the card's own lines change.

	*Method:* =POST=

	*Request Body (JSON):*
	#+BEGIN_SRC json
	{"hash": "hOpOB7vIg6oiYz5sMVSlzGiJXic=", "quality": 4, "deck": "Spanish"}
	#+END_SRC
	=quality= is 0-5; at or below the deck's failure quality (2) is a failure.

	*Response:* ={ok, msg, days, scheduled, data, leech, failed}=.
	EDOC */
func PostDrillReview(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	body, _ := io.ReadAll(r.Body)
	var req DrillReview
	if err := json.Unmarshal(body, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(DrillReviewReply{Msg: err.Error()})
		return
	}
	username := GetUsername(r)
	reply, err := drillReview(username, req, time.Now())
	if err != nil {
		json.NewEncoder(w).Encode(DrillReviewReply{Msg: err.Error()})
		return
	}
	json.NewEncoder(w).Encode(reply)
}

func drillReview(username string, req DrillReview, now time.Time) (DrillReviewReply, error) {
	if req.Quality < 0 || req.Quality > 5 {
		return DrillReviewReply{}, fmt.Errorf("a rating is 0 to 5")
	}
	deck := DrillDeck{Algorithm: req.Algorithm}
	if req.Deck != "" {
		for _, d := range GetExtensions().GetDrillDecks(username) {
			if d.Name == req.Deck {
				deck = d
			}
		}
	}
	deck = deck.WithDefaults()
	sec := GetDb().FindByHash(req.Hash)
	f := GetDb().ByHashToFile[req.Hash]
	if sec == nil || f == nil || sec.Headline == nil {
		return DrillReviewReply{}, fmt.Errorf("no heading with that hash: has the file changed since the session began?")
	}
	lines, from, to, ok := recordLines(f.Filename, sec)
	if !ok {
		return DrillReviewReply{}, fmt.Errorf("could not read %s", f.Filename)
	}
	lvl := sec.Headline.Lvl
	ownEnd, _ := headingRange(lines, from, lvl)
	props := propsFromLines(lines, from, ownEnd)
	d := readDrillData(props)
	weight, _ := strconv.ParseFloat(strings.TrimSpace(props["DRILL_CARD_WEIGHT"]), 64)
	due := 0
	for i := from + 1; i <= ownEnd && i < len(lines) && planningRe.MatchString(lines[i]); i++ {
		if m := scheduledRe.FindStringSubmatch(lines[i]); m != nil {
			if t, err := time.ParseInLocation("2006-01-02", m[1], time.Local); err == nil {
				due = daysBetween(t, now)
			}
		}
	}
	q := req.Quality
	// A card very long overdue is taken to be forgotten, when the deck says
	// so - but the rating given is what is recorded as given.
	effective := q
	if deck.LapseOverdue && due > 90 {
		effective = min(q, deck.FailureQuality)
	}
	matrix := GetExtensions().drillMatrix(username)
	days := nextReviewDays(d, deck, matrix, weight, req.Hash, now)[effective]
	out := determine(d, effective, deck, matrix, noiseFor(deck, req.Hash, d, now))
	nd := out.Data
	// The interval written is the one the rating promised, as org-drill's
	// smart reschedule overrides the algorithm's with it.
	nd.LastInterval = days

	ind := indentOf(lvl)
	lines, ps, pe, _ := ensurePropertyDrawer(lines, from, to, ind)
	// New keys go in at the drawer's own indent: Emacs writes drawers in
	// column zero, and a drawer half at one indent and half at another
	// is the first thing anybody would notice in a diff.
	pind := lines[ps][:len(lines[ps])-len(strings.TrimLeft(lines[ps], " \t"))]
	for i := ps + 1; i < pe; i++ {
		if m := propLineRe.FindStringSubmatch(lines[i]); m != nil && strings.EqualFold(m[2], "LEARN_DATA") {
			lines = append(lines[:i], lines[i+1:]...)
			pe--
			break
		}
	}
	for _, kv := range drillProps(nd, q, now) {
		lines, pe = setPropIn(lines, ps, pe, pind, kv[0], kv[1])
	}
	stamp := ""
	switch {
	case days <= 0 && math.Round(days) == 0:
		// A failure (or a 0-day interval) is unscheduled: it is new again
		// for the next session, and failed until it is passed.
	default:
		stamp = now.AddDate(0, 0, int(math.Round(days))).Format("<2006-01-02 Mon>")
	}
	lines = setScheduled(lines, from, ind, stamp)
	failed := effective <= deck.FailureQuality
	leech := false
	if failed && nd.Failures > deck.LeechThreshold {
		if l, changed := addHeadlineTag(lines[from], "leech"); changed {
			lines[from] = l
		}
		leech = true
	}
	if err := writeLines(f.Filename, lines); err != nil {
		return DrillReviewReply{}, err
	}
	if deck.Algorithm == "sm5" && out.Matrix != nil {
		if err := GetExtensions().setDrillMatrix(username, out.Matrix); err != nil {
			fmt.Fprintf(os.Stderr, "drill: could not save the SM5 matrix: %v\n", err)
		}
	}
	reply := DrillReviewReply{Ok: true, Days: days, Data: nd, Leech: leech, Failed: failed}
	if stamp != "" {
		reply.Scheduled = strings.Trim(stamp, "<>")
		reply.Msg = fmt.Sprintf("Next review in %d days", int(math.Round(days)))
	} else {
		reply.Msg = "Failed: it will be asked again"
	}
	return reply, nil
}

var scheduledOnLineRe = regexp.MustCompile(`\s*SCHEDULED:\s*<[^>]*>`)

// setScheduled puts a SCHEDULED stamp on the heading's planning line, or takes
// it off when stamp is empty, leaving any DEADLINE or CLOSED where it is.
func setScheduled(lines []string, from int, ind, stamp string) []string {
	for i := from + 1; i < len(lines); i++ {
		if !planningRe.MatchString(lines[i]) {
			break
		}
		if scheduledOnLineRe.MatchString(lines[i]) {
			rest := strings.TrimSpace(scheduledOnLineRe.ReplaceAllString(lines[i], ""))
			lead := lines[i][:len(lines[i])-len(strings.TrimLeft(lines[i], " \t"))]
			switch {
			case stamp == "" && rest == "":
				return append(lines[:i], lines[i+1:]...)
			case stamp == "":
				lines[i] = lead + rest
			case rest == "":
				lines[i] = lead + "SCHEDULED: " + stamp
			default:
				lines[i] = lead + rest + " SCHEDULED: " + stamp
			}
			return lines
		}
		if stamp != "" {
			lines[i] = strings.TrimRight(lines[i], " ") + " SCHEDULED: " + stamp
			return lines
		}
		return lines
	}
	if stamp == "" {
		return lines
	}
	return splice(lines, from+1, []string{ind + "SCHEDULED: " + stamp})
}

/* SDOC: API
* GET /ext/drill/decks — List Flashcard Decks
	Every flashcard deck the user has saved, in the order worg shows them. A
	deck is a query that picks its cards and org-drill's settings for drilling
	them (algorithm, session limits, leech handling...). It holds no cards.

	*Method:* =GET=

	*Response:* A JSON array of =DrillDeck= objects.
	EDOC */
func RequestDrillDecks(w http.ResponseWriter, r *http.Request) {
	username := GetUsername(r)
	if username == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(GetExtensions().GetDrillDecks(username))
}

/* SDOC: API
* POST /ext/drill/decks — Replace Every Flashcard Deck
	Writes the user's whole deck list at once, which is what adding, renaming,
	reordering and deleting all go through.

	*Method:* =POST=

	*Request Body (JSON):* An array of =DrillDeck= objects.

	*Response:* A =ResultMsg=. Refused when a deck has no name, two share one,
	or a deck has neither a query nor a saved search.
	EDOC */
func PostDrillDecks(w http.ResponseWriter, r *http.Request) {
	username := GetUsername(r)
	if username == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	body, _ := io.ReadAll(r.Body)
	var decks []DrillDeck
	if err := json.Unmarshal(body, &decks); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(common.ResultMsg{Ok: false, Msg: err.Error()})
		return
	}
	seen := map[string]bool{}
	for _, d := range decks {
		msg := ""
		switch {
		case strings.TrimSpace(d.Name) == "":
			msg = "every deck needs a name"
		case seen[d.Name]:
			msg = fmt.Sprintf("two decks named %q", d.Name)
		case strings.TrimSpace(d.Query) == "" && d.StoredQuery == "":
			msg = fmt.Sprintf("deck %q has no query", d.Name)
		}
		if msg != "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(common.ResultMsg{Ok: false, Msg: msg})
			return
		}
		seen[d.Name] = true
	}
	if err := GetExtensions().SetDrillDecks(username, decks); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(common.ResultMsg{Ok: false, Msg: err.Error()})
		return
	}
	json.NewEncoder(w).Encode(common.ResultMsg{Ok: true, Msg: "saved"})
}
