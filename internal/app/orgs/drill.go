package orgs

// Spaced repetition, as org-drill does it.
//
// The flashcards tab (worg's Drill) and the cards it reads are org-drill's: the
// same DRILL_* properties, written in the same spellings, the same SCHEDULED
// line, the same three algorithms with the same constants. A card drilled here
// and then in Emacs with `M-x org-drill` carries on where it left off, in both
// directions - which is the point of mirroring it rather than inventing a
// scheduler. The one thing that is not org-drill's is how cards are chosen: a
// deck is a query (`HasTag("spanish") && IsInFile("vocab.org")`), not the
// `:drill:` tag, so one set of notes can hold many decks.
//
// This file is the arithmetic and nothing else - no files, no requests - so the
// tests can pin it against org-drill's own worked numbers. drillcards.go reads
// cards and writes ratings.
//
// Where org-drill does something odd, this does the same odd thing on purpose,
// and says so; the two places it does not are marked "deliberately not
// org-drill" with the reason.

import (
	"hash/fnv"
	"math"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/ihdavids/orgs/internal/common"
)

// ── Settings ────────────────────────────────────────────────────────────────

// The deck, its card data and the wire types are in internal/common/drill.go,
// shared with `orgs drill`.
type DrillDeck = common.DrillDeck

// ── What a card remembers ───────────────────────────────────────────────────

type DrillData = common.DrillData

// readDrillData is org-drill-get-item-data: the legacy LEARN_DATA list first,
// then the DRILL_ properties, and a virgin card when neither is there.
func readDrillData(props map[string]string) DrillData {
	num := func(k string) (float64, bool) {
		v, ok := props[k]
		if !ok {
			return 0, false
		}
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		return f, err == nil
	}
	integer := func(k string) int {
		f, _ := num(k)
		return int(f)
	}
	if ld, ok := props["LEARN_DATA"]; ok {
		// "(interval repeats ease ...)": a lisp list from org-learn.
		f := strings.Fields(strings.Trim(strings.TrimSpace(ld), "()"))
		at := func(i int) float64 {
			if i < len(f) {
				x, _ := strconv.ParseFloat(f[i], 64)
				return x
			}
			return 0
		}
		d := DrillData{LastInterval: at(0), Repeats: int(at(1)), TotalRepeats: int(at(1)), Failures: integer("DRILL_FAILURE_COUNT")}
		if q, ok := num("DRILL_LAST_QUALITY"); ok {
			d.MeanQ = &q
		}
		e := at(2)
		d.Ease = &e
		return d
	}
	if _, ok := props["DRILL_TOTAL_REPEATS"]; !ok {
		return DrillData{}
	}
	d := DrillData{
		Repeats:      integer("DRILL_REPEATS_SINCE_FAIL"),
		Failures:     integer("DRILL_FAILURE_COUNT"),
		TotalRepeats: integer("DRILL_TOTAL_REPEATS"),
	}
	d.LastInterval, _ = num("DRILL_LAST_INTERVAL")
	if q, ok := num("DRILL_AVERAGE_QUALITY"); ok {
		d.MeanQ = &q
	}
	if e, ok := num("DRILL_EASE"); ok {
		d.Ease = &e
	}
	return d
}

// ── The SM5 matrix ──────────────────────────────────────────────────────────

// DrillMatrix is org-drill-sm5-optimal-factor-matrix: for each repetition
// number, an optimal factor per ease. It is keyed by the ease rounded to three
// places - deliberately not org-drill, which keys by the unrounded float and
// compares with `equal`, so 2.3600000000000003 written and 2.36 read back
// miss each other and SM5 quietly never learns anything.
type DrillMatrix map[int]map[string]float64

func efKey(ef float64) string { return strconv.FormatFloat(roundTo(ef, 3), 'f', -1, 64) }

func (m DrillMatrix) get(n int, ef float64, initial float64) float64 {
	if row, ok := m[n]; ok {
		if of, ok := row[efKey(ef)]; ok {
			return of
		}
	}
	if n == 1 {
		return initial
	}
	return ef
}

func (m DrillMatrix) set(n int, ef, of float64) {
	if m[n] == nil {
		m[n] = map[string]float64{}
	}
	m[n][efKey(ef)] = roundTo(of, 3)
}

func (m DrillMatrix) clone() DrillMatrix {
	out := DrillMatrix{}
	for n, row := range m {
		out[n] = map[string]float64{}
		for k, v := range row {
			out[n][k] = v
		}
	}
	return out
}

// ── The algorithms ──────────────────────────────────────────────────────────

// drillOutcome is what one rating does to a card.
type drillOutcome struct {
	Interval float64 // days; -1 is a failure
	Data     DrillData
	Matrix   DrillMatrix // SM5's, updated; nil for the others
}

func roundTo(x float64, places int) float64 {
	p := math.Pow(10, float64(places))
	return math.Round(x*p) / p
}

// modifyEF is org-drill-modify-e-factor. The floor is applied to the ease it
// is given, before the change, so an ease can dip under 1.3 for one review -
// org-drill's behaviour, kept so the two agree about a card.
func modifyEF(ef float64, q int) float64 {
	if ef < 1.3 {
		return 1.3
	}
	d := float64(5 - q)
	return ef + (0.1 - d*(0.08+d*0.02))
}

func meanQ(d DrillData, q int) float64 {
	if d.MeanQ == nil {
		return float64(q)
	}
	return (float64(q) + *d.MeanQ*float64(d.TotalRepeats)) / float64(d.TotalRepeats+1)
}

// dispersal is org-drill-random-dispersal-factor: a multiplier around 1, from
// about 0.58 to 1.42, bunched near the middle.
func dispersal(r *rand.Rand) float64 {
	const a, b = 0.047, 0.092
	p := r.Float64() - 0.5
	s := 1.0
	if p < 0 {
		s = -1
	}
	return (100 + s*(-1/b)*math.Log(1-(b/a)*math.Abs(p))) / 100
}

func sm2(d DrillData, q int, fail int, noise *rand.Rand) drillOutcome {
	n := max(d.Repeats, 1)
	ef := 2.5
	if d.Ease != nil {
		ef = *d.Ease
	}
	mq := meanQ(d, q)
	if q <= fail {
		return drillOutcome{Interval: -1, Data: DrillData{LastInterval: d.LastInterval, Repeats: 1, Failures: d.Failures + 1, TotalRepeats: d.TotalRepeats + 1, MeanQ: &mq, Ease: &ef}}
	}
	nef := modifyEF(ef, q)
	var iv float64
	switch {
	case n <= 1:
		iv = 1
	case n == 2:
		iv = 6
		if noise != nil {
			iv = []float64{3, 4, 4, 5, 6, 6}[noise.Intn(6)]
		}
	default:
		iv = d.LastInterval * nef
		if noise != nil {
			iv = d.LastInterval + (iv-d.LastInterval)*dispersal(noise)
		}
	}
	return drillOutcome{Interval: iv, Data: DrillData{LastInterval: iv, Repeats: n + 1, Failures: d.Failures, TotalRepeats: d.TotalRepeats + 1, MeanQ: &mq, Ease: &nef}}
}

func sm5(d DrillData, q int, deck DrillDeck, m DrillMatrix, noise *rand.Rand) drillOutcome {
	n := max(d.Repeats, 1)
	ef := 2.5
	if d.Ease != nil {
		ef = *d.Ease
	}
	mq := meanQ(d, q)
	m = m.clone()
	nef := modifyEF(ef, q)
	of := m.get(n, ef, deck.SM5Initial)
	f := deck.LearnFraction
	newOF := (1-f)*of + f*(of*(0.72+0.07*float64(q)))
	// The matrix learns from failures too, at the new ease.
	m.set(n, nef, newOF)
	if q <= deck.FailureQuality {
		return drillOutcome{Interval: -1, Matrix: m, Data: DrillData{LastInterval: d.LastInterval, Repeats: 1, Failures: d.Failures + 1, TotalRepeats: d.TotalRepeats + 1, MeanQ: &mq, Ease: &ef}}
	}
	iv := m.get(n, nef, deck.SM5Initial)
	if n != 1 {
		iv *= d.LastInterval
	}
	if noise != nil {
		iv *= dispersal(noise)
	}
	return drillOutcome{Interval: iv, Matrix: m, Data: DrillData{LastInterval: iv, Repeats: n + 1, Failures: d.Failures, TotalRepeats: d.TotalRepeats + 1, MeanQ: &mq, Ease: &nef}}
}

func simple8(d DrillData, q int, deck DrillDeck, noise *rand.Rand) drillOutcome {
	mq := meanQ(d, q)
	ease := 0.0542*math.Pow(mq, 4) - 0.4848*math.Pow(mq, 3) + 1.4916*mq*mq - 1.2403*mq + 1.4515
	out := DrillData{Failures: d.Failures, Repeats: d.Repeats, TotalRepeats: d.TotalRepeats, MeanQ: &mq, Ease: &ease}
	var next float64
	switch {
	case q <= deck.FailureQuality:
		// org-drill does not count a failure in the total here.
		out.Failures++
		out.Repeats = 0
		next = -1
		out.LastInterval = d.LastInterval
		return drillOutcome{Interval: next, Data: out}
	case d.Repeats == 0 || d.LastInterval == 0:
		next = 2.4849 * math.Exp(-0.057*float64(out.Failures))
	default:
		factor := 1.2 + (ease-1.2)*math.Pow(deck.LearnFraction, math.Log2(float64(d.Repeats)))
		next = d.LastInterval * factor
	}
	out.Repeats++
	out.TotalRepeats++
	if noise != nil && next > 0 {
		next *= dispersal(noise)
	}
	out.LastInterval = next
	return drillOutcome{Interval: next, Data: out}
}

// determine runs the deck's algorithm.
func determine(d DrillData, q int, deck DrillDeck, m DrillMatrix, noise *rand.Rand) drillOutcome {
	switch deck.Algorithm {
	case "sm2":
		return sm2(d, q, deck.FailureQuality, noise)
	case "simple8":
		return simple8(d, q, deck, noise)
	default:
		return sm5(d, q, deck, m, noise)
	}
}

// noiseFor is the random source for one card's review today. It is seeded
// from the card and the day, so the "+N days" the rating buttons promise is the
// interval the rating then writes - org-drill gets that by drawing once when
// it builds the prompt; a request per step cannot, so the draw is made
// repeatable instead.
func noiseFor(deck DrillDeck, hash string, d DrillData, now time.Time) *rand.Rand {
	if !deck.Noise {
		return nil
	}
	h := fnv.New64a()
	h.Write([]byte(hash))
	h.Write([]byte(now.Format("2006-01-02")))
	h.Write([]byte(strconv.Itoa(d.TotalRepeats)))
	return rand.New(rand.NewSource(int64(h.Sum64())))
}

// nextReviewDays is org-drill-hypothetical-next-review-dates: for each rating
// 0-5, the days until the card comes round again. Never less for a better
// rating, a failure is 0, and a card's DRILL_CARD_WEIGHT divides the step.
func nextReviewDays(d DrillData, deck DrillDeck, m DrillMatrix, weight float64, hash string, now time.Time) [6]float64 {
	var out [6]float64
	prev := 0.0
	for q := 0; q <= 5; q++ {
		o := determine(d, q, deck, m, noiseFor(deck, hash, d, now))
		next := o.Interval
		switch {
		case next <= 0:
			next = 0
		case weight > 0:
			next = d.LastInterval + math.Max(1.0, (next-d.LastInterval)/weight)
		}
		next = math.Max(prev, next)
		out[q] = next
		prev = next
	}
	return out
}

// ── Formatting, as org-drill writes it ──────────────────────────────────────

// lispFloat prints a number the way Emacs prints a float: always with a
// decimal point, so 4 is "4.0" and a file drilled here diffs cleanly against
// one drilled in Emacs.
func lispFloat(x float64, places int) string {
	s := strconv.FormatFloat(roundTo(x, places), 'f', -1, 64)
	if !strings.ContainsAny(s, ".eE") {
		s += ".0"
	}
	return s
}

// drillProps is what org-drill-store-item-data and -reschedule write.
func drillProps(d DrillData, q int, now time.Time) [][2]string {
	mq, ef := 0.0, 2.5
	if d.MeanQ != nil {
		mq = *d.MeanQ
	}
	if d.Ease != nil {
		ef = *d.Ease
	}
	return [][2]string{
		{"DRILL_LAST_INTERVAL", lispFloat(d.LastInterval, 4)},
		{"DRILL_REPEATS_SINCE_FAIL", strconv.Itoa(d.Repeats)},
		{"DRILL_TOTAL_REPEATS", strconv.Itoa(d.TotalRepeats)},
		{"DRILL_FAILURE_COUNT", strconv.Itoa(d.Failures)},
		{"DRILL_AVERAGE_QUALITY", lispFloat(mq, 3)},
		{"DRILL_EASE", lispFloat(ef, 3)},
		{"DRILL_LAST_QUALITY", strconv.Itoa(q)},
		{"DRILL_LAST_REVIEWED", now.Format("[2006-01-02 Mon 15:04]")},
	}
}

// ── Where a card stands ─────────────────────────────────────────────────────

// Card statuses, org-drill-entry-status.
const (
	DrillNew     = "new"
	DrillFailed  = "failed"
	DrillOverdue = "overdue"
	DrillYoung   = "young"
	DrillOld     = "old"
	DrillFuture  = "future"  // dormant: not due yet
	DrillSkipped = "skipped" // dormant: a leech the deck skips
)

// drillStatus classifies a card. due is days overdue (today minus the
// scheduled day, 0 when unscheduled); lastQuality is -1 when never rated.
func drillStatus(deck DrillDeck, scheduled bool, due int, lastQuality int, lastInterval float64, hasInterval bool, leech bool, cram bool, lastReviewed *time.Time, now time.Time) string {
	if cram {
		// Everything not seen in the last cramHours is due, leeches included.
		if lastReviewed != nil && now.Sub(*lastReviewed) < time.Duration(deck.CramHours)*time.Hour {
			return DrillFuture
		}
		if !scheduled {
			return DrillNew
		}
		return DrillYoung
	}
	if leech && deck.LeechMethod == "skip" {
		return DrillSkipped
	}
	if scheduled && due < 0 {
		return DrillFuture
	}
	if lastQuality >= 0 && lastQuality <= deck.FailureQuality {
		return DrillFailed
	}
	if !scheduled {
		return DrillNew
	}
	li := lastInterval
	if !hasInterval {
		li = 1
	}
	if li <= 0 {
		li = 1
	}
	if due > 1 && (float64(due)+li+1.0)/li > deck.OverdueFactor {
		return DrillOverdue
	}
	if !hasInterval || lastInterval > float64(deck.DaysBeforeOld) {
		return DrillOld
	}
	return DrillYoung
}

// daysBetween counts calendar days, as org's time-to-days does.
func daysBetween(from, to time.Time) int {
	a := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.UTC)
	b := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, time.UTC)
	return int(math.Round(b.Sub(a).Hours() / 24))
}
