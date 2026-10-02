package drill

// The drill session: org-drill's queues, its again pile, its limits, which
// clozes a card hides and which side it asks, and the report at the end.
//
// This is worg/src/drill.ts said again in Go, because the session runs in the
// client and these are two clients (see **Traps: logic said twice**): change
// one, change the other. session_test.go and drill.test.ts pin the same
// scenarios. What is written to a file - the scheduling, the properties - is
// the server's, and neither client says it.

import (
	"math"
	"math/rand"
	"sort"
	"strings"
	"time"

	"github.com/ihdavids/orgs/internal/common"
)

// Rng is a source of [0, 1), like Math.random.
type Rng func() float64

func NewRng(seed int64) Rng {
	r := rand.New(rand.NewSource(seed))
	return r.Float64
}

func randInt(rng Rng, n int) int { return int(math.Floor(rng() * float64(n))) }

func shuffle[T any](xs []T, rng Rng) []T {
	a := append([]T{}, xs...)
	for i := len(a) - 1; i > 0; i-- {
		j := randInt(rng, i+1)
		a[i], a[j] = a[j], a[i]
	}
	return a
}

// Session is one sitting.
type Session struct {
	Deck  common.DrillDeck
	Cram  bool
	// Cards are read fresh from the server on -resume, so not saved.
	Cards map[string]*common.DrillCard `json:"-"`
	// The queues, by hash. Again is the cards failed this session.
	Failed, Overdue, Young, Fresh, Old, Again []string
	Done                                      []string
	Current                                   string
	FailedHere                                map[string]bool
	Qualities                                 []int
	Start                                     time.Time
	Dormant, DueTomorrow, OverdueAtStart      int
	Scanned                                   int
}

const lapseDays = 90

// NewSession sorts a deck's cards into org-drill's queues.
func NewSession(r common.DrillCardsReply, cram bool, now time.Time, rng Rng) *Session {
	s := &Session{
		Deck: r.Deck, Cram: cram, Cards: map[string]*common.DrillCard{},
		FailedHere: map[string]bool{}, Start: now, DueTomorrow: r.Counts.DueTomorrow,
	}
	overdue := []*common.DrillCard{}
	for i := range r.Cards {
		c := &r.Cards[i]
		s.Cards[c.Hash] = c
		s.Scanned++
		switch c.Status {
		case "failed":
			s.Failed = append(s.Failed, c.Hash)
		case "overdue":
			overdue = append(overdue, c)
		case "young":
			s.Young = append(s.Young, c.Hash)
		case "new":
			s.Fresh = append(s.Fresh, c.Hash)
		case "old":
			s.Old = append(s.Old, c.Hash)
		default:
			s.Dormant++
		}
	}
	s.OverdueAtStart = len(overdue)
	for _, c := range OrderOverdue(overdue, r.Deck.LapseOverdue, rng) {
		s.Overdue = append(s.Overdue, c.Hash)
	}
	return s
}

// OrderOverdue is org-drill-order-overdue-entries: shuffled, then the most
// overdue first; cards so long overdue they count as forgotten go last, the
// oldest first.
func OrderOverdue(cards []*common.DrillCard, lapse bool, rng Rng) []*common.DrillCard {
	limit := math.MaxInt
	if lapse {
		limit = lapseDays
	}
	fresh, lapsed := []*common.DrillCard{}, []*common.DrillCard{}
	for _, c := range cards {
		if c.Due <= limit {
			fresh = append(fresh, c)
		} else {
			lapsed = append(lapsed, c)
		}
	}
	fresh = shuffle(fresh, rng)
	sort.SliceStable(fresh, func(a, b int) bool { return fresh[a].Due > fresh[b].Due })
	sort.SliceStable(lapsed, func(a, b int) bool { return lapsed[a].Age > lapsed[b].Age })
	return append(fresh, lapsed...)
}

func (s *Session) LimitReached(now time.Time) bool {
	if s.Cram {
		return false
	}
	count := len(s.Done)
	if s.Deck.CountFailed {
		count += len(s.Again)
	}
	if s.Deck.MaxItems > 0 && count >= s.Deck.MaxItems {
		return true
	}
	if s.Deck.MaxMinutes > 0 && now.Sub(s.Start) > time.Duration(s.Deck.MaxMinutes)*time.Minute {
		return true
	}
	return false
}

func takeRandom(xs *[]string, rng Rng) string {
	i := randInt(rng, len(*xs))
	h := (*xs)[i]
	*xs = append((*xs)[:i], (*xs)[i+1:]...)
	return h
}

// Next is org-drill-pop-next-pending-entry: until a limit, failed from last
// time, then overdue in order, then young, then new and old together in
// proportion; after the limit, and always last, the again pile. "" is the end.
func (s *Session) Next(now time.Time, rng Rng) string {
	h := ""
	if !s.LimitReached(now) {
		switch {
		case len(s.Failed) > 0:
			h = takeRandom(&s.Failed, rng)
		case len(s.Overdue) > 0:
			h, s.Overdue = s.Overdue[0], s.Overdue[1:]
		case len(s.Young) > 0:
			h = takeRandom(&s.Young, rng)
		case len(s.Fresh)+len(s.Old) > 0:
			if rng() < float64(len(s.Fresh))/float64(len(s.Fresh)+len(s.Old)) {
				h = takeRandom(&s.Fresh, rng)
			} else {
				h = takeRandom(&s.Old, rng)
			}
		}
	}
	if h == "" && len(s.Again) > 0 {
		h, s.Again = s.Again[0], s.Again[1:]
	}
	s.Current = h
	return h
}

// Rate records a rating: a failure to the back of a reshuffled again pile, a
// pass to done. Cram counts nothing.
func (s *Session) Rate(hash string, q int, rng Rng) {
	if !s.Cram {
		s.Qualities = append(s.Qualities, q)
	}
	if q <= s.Deck.FailureQuality {
		s.Again = append(shuffle(s.Again, rng), hash)
		s.FailedHere[hash] = true
	} else {
		s.Done = append(s.Done, hash)
	}
	s.Current = ""
}

func (s *Session) Skip() { s.Current = "" }

func (s *Session) Pending() int {
	n := len(s.Failed) + len(s.Overdue) + len(s.Young) + len(s.Fresh) + len(s.Old) + len(s.Again)
	if s.Current != "" {
		n++
	}
	return n
}

// Letter is the status letter org-drill shows: F failed, C cram, N new,
// Y young, o old, ! overdue; and its tone.
func (s *Session) Letter(c *common.DrillCard) (string, string) {
	if s.FailedHere[c.Hash] || c.Status == "failed" {
		return "F", "failed"
	}
	if s.Cram {
		return "C", "neutral"
	}
	switch c.Status {
	case "new":
		return "N", "new"
	case "young":
		return "Y", "mature"
	case "old":
		return "o", "mature"
	case "overdue":
		return "!", "failed"
	}
	return "?", "neutral"
}

// Counters: done, failed, mature (young, old, overdue), new.
func (s *Session) Counters() (int, int, int, int) {
	return len(s.Done), len(s.Again) + len(s.Failed), len(s.Young) + len(s.Old) + len(s.Overdue), len(s.Fresh)
}

// Again is org-drill-again: what is still pending, a fresh clock and tally.
func (s *Session) KeepGoing(now time.Time) {
	s.Cram, s.Done, s.Qualities, s.Start, s.Current = false, nil, nil, now, ""
}

// Clock is org-drill's MM:SS, ++:++ past the hour.
func Clock(d time.Duration) string {
	secs := int(d.Seconds())
	if secs >= 3600 {
		return "++:++"
	}
	return twoDigits(secs/60) + ":" + twoDigits(secs%60)
}

func twoDigits(n int) string {
	if n < 10 {
		return "0" + itoa(n)
	}
	return itoa(n)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	if neg {
		s = "-" + s
	}
	return s
}

// Days writes an interval for a rating: "again", "+1 day", "+4 days".
func Days(d float64) string {
	n := int(math.Round(d))
	switch {
	case n <= 0:
		return "again"
	case n == 1:
		return "+1 day"
	}
	return "+" + itoa(n) + " days"
}

// ── What a card asks ────────────────────────────────────────────────────────

// Presentation is one showing of a card: the clozes hidden (0 always is, in
// the question), the side asked (-1 none), and the generated card types' own
// question and answer heading.
type Presentation struct {
	Hide          map[int]bool
	Side          int
	Prompt        string
	AnswerHeading string
}

const clozeWeight = 4 // org-drill-cloze-text-weight

type hideOpts struct{ forceHideFirst, forceShowFirst, forceShowLast bool }

// HideClozes chooses which of count clozes to hide when n are hidden (a
// negative n: all but -n).
func HideClozes(count, n int, rng Rng, o hideOpts) map[int]bool {
	pos := make([]int, count)
	for i := range pos {
		pos[i] = i + 1
	}
	pos = shuffle(pos, rng)
	without := func(xs []int, v int) []int {
		out := []int{}
		for _, x := range xs {
			if x != v {
				out = append(out, x)
			}
		}
		return out
	}
	if o.forceHideFirst {
		pos = append([]int{1}, without(pos, 1)...)
	}
	if o.forceShowFirst {
		pos = without(pos, 1)
	}
	if o.forceShowLast {
		pos = without(pos, count)
	}
	want := n
	if n < 0 {
		want = count + n
	}
	out := map[int]bool{}
	if want > count {
		return out
	}
	for i := 0; i < max(0, min(want, len(pos))); i++ {
		out[pos[i]] = true
	}
	return out
}

func all(count int) map[int]bool {
	out := map[int]bool{}
	for i := 1; i <= count; i++ {
		out[i] = true
	}
	return out
}

var tenses = []string{"present", "past", "future perfect"}

// Present chooses what this showing of a card asks.
func Present(c *common.DrillCard, rng Rng) Presentation {
	k := c.Clozes
	uncommon := (c.Data.TotalRepeats+1)%clozeWeight == 0
	one := func(i int) map[int]bool {
		if k == 0 {
			return map[int]bool{}
		}
		return map[int]bool{i: true}
	}
	switch c.Type {
	case "twosided":
		side := -1
		if len(c.Sides) > 0 {
			side = randInt(rng, min(2, len(c.Sides)))
		}
		return Presentation{Hide: all(k), Side: side}
	case "multisided":
		side := -1
		if len(c.Sides) > 0 {
			side = randInt(rng, len(c.Sides))
		}
		return Presentation{Hide: all(k), Side: side}
	case "hide1cloze", "multicloze":
		return Presentation{Hide: HideClozes(k, 1, rng, hideOpts{}), Side: -1}
	case "hide2cloze":
		return Presentation{Hide: HideClozes(k, 2, rng, hideOpts{}), Side: -1}
	case "show1cloze":
		return Presentation{Hide: HideClozes(k, -1, rng, hideOpts{}), Side: -1}
	case "show2cloze":
		return Presentation{Hide: HideClozes(k, -2, rng, hideOpts{}), Side: -1}
	case "hidefirst":
		return Presentation{Hide: one(1), Side: -1}
	case "hidelast":
		return Presentation{Hide: one(k), Side: -1}
	case "hide1_firstmore":
		if uncommon {
			return Presentation{Hide: HideClozes(k, 1, rng, hideOpts{forceShowFirst: true}), Side: -1}
		}
		return Presentation{Hide: one(1), Side: -1}
	case "show1_lastmore":
		if uncommon {
			return Presentation{Hide: HideClozes(k, -1, rng, hideOpts{forceHideFirst: true}), Side: -1}
		}
		return Presentation{Hide: HideClozes(k, -1, rng, hideOpts{forceShowLast: true}), Side: -1}
	case "show1_firstless":
		if uncommon {
			return Presentation{Hide: HideClozes(k, -1, rng, hideOpts{forceShowFirst: true}), Side: -1}
		}
		return Presentation{Hide: HideClozes(k, -1, rng, hideOpts{forceHideFirst: true}), Side: -1}
	case "conjugate":
		return conjugate(c, rng)
	case "decline_noun":
		return declineNoun(c, rng)
	case "spanish_verb":
		r := randInt(rng, 6)
		want := "english"
		if r%2 == 0 {
			want = "infinitive"
		}
		side := -1
		for i, sd := range c.Sides {
			if strings.EqualFold(sd.Name, want) {
				side = i
				break
			}
		}
		tense := tenses[r/2]
		prompt := "For the English verb below, give the Spanish infinitive, and conjugate it for the " + tense + " tense."
		if r%2 == 0 {
			prompt = "Translate this Spanish verb, and conjugate it for the " + tense + " tense."
		}
		return Presentation{Hide: all(k), Side: side, Prompt: prompt}
	}
	return Presentation{Hide: all(k), Side: -1}
}

func capFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func conjugate(c *common.DrillCard, rng Rng) Presentation {
	p := c.Props
	inf, tr, hint := p["VERB_INFINITIVE"], p["VERB_TRANSLATION"], p["VERB_INFINITIVE_HINT"]
	if inf == "" {
		inf = c.Name
	}
	tense, mood := p["VERB_TENSE"], p["VERB_MOOD"]
	form := "the given form"
	switch {
	case tense != "" && mood != "":
		form = tense + " tense, " + mood + " mood"
	case tense != "":
		form = tense + " tense"
	case mood != "":
		form = mood + " mood"
	}
	prompt := "Translate the verb\n\n" + inf + "\n\nand conjugate for the " + form + "."
	if rng() >= 0.5 {
		h := ""
		if hint != "" {
			h = "  [HINT: " + hint + "]"
		}
		prompt = "Give the verb that means\n\n" + tr + h + "\n\nand conjugate for the " + form + "."
	}
	what := []string{}
	for _, x := range []string{tense, mood} {
		if x != "" {
			what = append(what, capFirst(x))
		}
	}
	w := strings.Join(what, " ")
	if w == "" {
		w = "Conjugation"
	}
	return Presentation{Hide: map[int]bool{}, Side: -1, Prompt: prompt, AnswerHeading: w + " of " + inf + " ==> " + tr}
}

func declineNoun(c *common.DrillCard, rng Rng) Presentation {
	p := c.Props
	noun, gender, tr, hint := p["NOUN"], p["NOUN_GENDER"], p["NOUN_TRANSLATION"], p["NOUN_HINT"]
	if noun == "" {
		noun = c.Name
	}
	truthy := func(v string) bool { return v != "" && v != "nil" && v != "false" }
	def, hasDef := p["DECLINE_DEFINITE"]
	pl, hasPl := p["DECLINE_PLURAL"]
	forms := ""
	if hasDef || hasPl {
		d, n := "indefinite", "singular"
		if truthy(def) {
			d = "definite"
		}
		if truthy(pl) {
			n = "plural"
		}
		forms = " for the " + d + " " + n + " form"
	}
	prompt := "Translate the noun\n\n" + noun + " (" + gender + ")\n\nand list its declensions" + forms + "."
	if rng() >= 0.5 {
		h := ""
		if hint != "" {
			h = "  [HINT: " + hint + "]"
		}
		prompt = "Give the noun that means\n\n" + tr + h + "\n\nand list its declensions" + forms + "."
	}
	return Presentation{Hide: map[int]bool{}, Side: -1, Prompt: prompt, AnswerHeading: "Declensions of " + noun + " (" + gender + ") ==> " + tr}
}

// ── Drawing clozes ──────────────────────────────────────────────────────────

// ClozeOrg rewrites a card's clozes as org markup for one phase: a hidden one
// as ~[...]~ (or ~[hint...]~), drawn as a code chip; in the answer, every one
// shown in bold. A cloze left showing in the question is plain text. first is
// the numbering (1 for the body, 0 for the title and sides) - the same
// common.Clozes the server numbered them with.
func ClozeOrg(text string, first int, hide map[int]bool, answer, dots bool) string {
	out, _ := common.Clozes(text, first, func(c common.Cloze) string {
		hidden := !answer && (c.N == 0 || hide[c.N])
		if !hidden {
			if answer {
				return "*" + c.Text + "*"
			}
			return c.Text
		}
		fill := "..."
		if dots {
			fill = strings.Repeat(".", max(3, len([]rune(c.Text))))
		}
		switch {
		case c.Hint != "" && strings.Contains(c.Hint, "..."):
			return "~[" + c.Hint + "]~"
		case c.Hint != "":
			return "~[" + c.Hint + fill + "]~"
		}
		return "~[" + fill + "]~"
	})
	return out
}

// ── The report ──────────────────────────────────────────────────────────────

type Report struct {
	Reviewed                         int
	Duration                         time.Duration
	Percent                          map[int]int
	PassPercent, FailureQuality      int
	Pending, PendingAll              int
	Failed, Overdue, Fresh, Young, Old int
	DueTomorrow                      int
	Warn                             bool
	OverduePercent, OverdueAtStart   int
}

// MakeReport is org-drill-final-report's numbers.
func (s *Session) MakeReport(now time.Time) Report {
	n := max(1, len(s.Qualities))
	r := Report{Percent: map[int]int{}, FailureQuality: s.Deck.FailureQuality}
	passed := 0
	for q := 0; q <= 5; q++ {
		c := 0
		for _, x := range s.Qualities {
			if x == q {
				c++
			}
		}
		r.Percent[q] = int(math.Round(100 * float64(c) / float64(n)))
	}
	for _, x := range s.Qualities {
		if x > s.Deck.FailureQuality {
			passed++
		}
	}
	r.PassPercent = int(math.Round(100 * float64(passed) / float64(n)))
	r.Reviewed = len(s.Done)
	r.Duration = now.Sub(s.Start).Round(time.Second)
	r.Pending = s.Pending()
	r.PendingAll = r.Pending + s.Dormant
	r.Failed = len(s.Failed) + len(s.Again)
	r.Overdue, r.Fresh, r.Young, r.Old = len(s.Overdue), len(s.Fresh), len(s.Young), len(s.Old)
	r.DueTomorrow = s.DueTomorrow
	r.Warn = len(s.Qualities) > 0 && r.PassPercent < 100-s.Deck.ForgettingIndex
	r.OverdueAtStart = s.OverdueAtStart
	r.OverduePercent = int(math.Round(100 * float64(s.OverdueAtStart) / float64(max(1, s.Scanned))))
	return r
}
