package orgs

import (
	"math"
	"testing"
	"time"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// org-drill's first SM5 review of a new card, ease 2.5: q3 → 3.86 days and
// ease 2.36, q4 → 4.0 and 2.5, q5 → 4.14 and 2.6.
func TestSM5FirstReview(t *testing.T) {
	deck := DrillDeck{}.WithDefaults()
	for _, c := range []struct {
		q      int
		iv, ef float64
	}{{3, 3.86, 2.36}, {4, 4.0, 2.5}, {5, 4.14, 2.6}} {
		o := sm5(DrillData{}, c.q, deck, DrillMatrix{}, nil)
		if !near(roundTo(o.Interval, 4), c.iv) || !near(roundTo(*o.Data.Ease, 3), c.ef) {
			t.Errorf("q%d: interval %v ease %v, want %v %v", c.q, o.Interval, *o.Data.Ease, c.iv, c.ef)
		}
		if o.Data.Repeats != 2 || o.Data.TotalRepeats != 1 {
			t.Errorf("q%d: repeats %d total %d", c.q, o.Data.Repeats, o.Data.TotalRepeats)
		}
	}
	// The second review multiplies the last interval by the optimal factor,
	// which starts at the ease: 4 days × 2.5.
	ef := 2.5
	d := DrillData{LastInterval: 4, Repeats: 2, TotalRepeats: 1, Ease: &ef}
	if o := sm5(d, 4, deck, DrillMatrix{}, nil); !near(o.Interval, 10) {
		t.Errorf("second review: %v, want 10", o.Interval)
	}
}

func TestSM5MatrixLearnsAndRoundTrips(t *testing.T) {
	deck := DrillDeck{}.WithDefaults()
	m := DrillMatrix{}
	o := sm5(DrillData{}, 3, deck, m, nil)
	if len(m) != 0 {
		t.Error("the matrix passed in was changed; it should be copied")
	}
	// What was written is found again by the ease as stored in the file.
	ef := roundTo(*o.Data.Ease, 3)
	if got := o.Matrix.get(1, ef, 4.0); !near(got, 3.86) {
		t.Errorf("matrix lookup at the stored ease: %v", got)
	}
	// A failure still teaches the matrix.
	f := sm5(DrillData{}, 1, deck, DrillMatrix{}, nil)
	if f.Interval != -1 || len(f.Matrix[1]) != 1 || f.Data.Failures != 1 || f.Data.Repeats != 1 {
		t.Errorf("failure: %+v", f)
	}
}

func TestSM2(t *testing.T) {
	o := sm2(DrillData{}, 4, 2, nil)
	if o.Interval != 1 || o.Data.Repeats != 2 {
		t.Errorf("first: %+v", o)
	}
	o = sm2(o.Data, 4, 2, nil)
	if o.Interval != 6 {
		t.Errorf("second: %v", o.Interval)
	}
	o = sm2(o.Data, 5, 2, nil)
	if !near(o.Interval, 6*2.6) {
		t.Errorf("third: %v", o.Interval)
	}
}

func TestSimple8(t *testing.T) {
	deck := DrillDeck{Algorithm: "simple8"}.WithDefaults()
	o := simple8(DrillData{}, 4, deck, nil)
	if !near(o.Interval, 2.4849) {
		t.Errorf("first: %v", o.Interval)
	}
	f := simple8(o.Data, 1, deck, nil)
	if f.Interval != -1 || f.Data.TotalRepeats != o.Data.TotalRepeats || f.Data.Repeats != 0 {
		t.Errorf("a failure is not counted in the total, as in org-drill: %+v", f.Data)
	}
}

func TestEaseFloorIsAppliedBeforeTheChange(t *testing.T) {
	// org-drill's order: 1.3 at q3 drops to 1.16 for a review.
	if got := modifyEF(1.3, 3); !near(roundTo(got, 2), 1.16) {
		t.Errorf("got %v", got)
	}
	if got := modifyEF(1.2, 3); got != 1.3 {
		t.Errorf("got %v", got)
	}
}

func TestNextReviewDaysNeverFallWithABetterRating(t *testing.T) {
	deck := DrillDeck{Noise: true}.WithDefaults()
	ef := 2.2
	d := DrillData{LastInterval: 12, Repeats: 4, TotalRepeats: 6, Ease: &ef}
	now := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	days := nextReviewDays(d, deck, DrillMatrix{}, 0, "abc", now)
	for q := 1; q < 6; q++ {
		if days[q] < days[q-1] {
			t.Errorf("q%d %v < q%d %v", q, days[q], q-1, days[q-1])
		}
	}
	if days[0] != 0 || days[2] != 0 {
		t.Errorf("a failure comes back today: %v", days)
	}
	// The noise is drawn the same way twice, so the promise is kept.
	if again := nextReviewDays(d, deck, DrillMatrix{}, 0, "abc", now); again != days {
		t.Error("noise is not repeatable")
	}
	// A weight of 2 roughly halves the step.
	w := nextReviewDays(d, DrillDeck{}.WithDefaults(), DrillMatrix{}, 2, "abc", now)
	plain := nextReviewDays(d, DrillDeck{}.WithDefaults(), DrillMatrix{}, 0, "abc", now)
	if !near(w[4], 12+(plain[4]-12)/2) {
		t.Errorf("weight: %v vs %v", w[4], plain[4])
	}
}

func TestDrillStatus(t *testing.T) {
	deck := DrillDeck{}.WithDefaults()
	now := time.Now()
	cases := []struct {
		name      string
		scheduled bool
		due, lq   int
		li        float64
		hasLI     bool
		leech     bool
		want      string
	}{
		{"never seen", false, 0, -1, 0, false, false, DrillNew},
		{"failed last time", false, 0, 1, 0, true, false, DrillFailed},
		{"not yet", true, -3, 4, 6, true, false, DrillFuture},
		{"due today, young", true, 0, 4, 6, true, false, DrillYoung},
		{"due today, old", true, 0, 4, 30, true, false, DrillOld},
		{"well overdue", true, 9, 4, 6, true, false, DrillOverdue},
		{"a leech", true, 0, 4, 6, true, true, DrillSkipped},
	}
	for _, c := range cases {
		if got := drillStatus(deck, c.scheduled, c.due, c.lq, c.li, c.hasLI, c.leech, false, nil, now); got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}
	recent := now.Add(-2 * time.Hour)
	if got := drillStatus(deck, true, -5, 4, 6, true, false, true, &recent, now); got != DrillFuture {
		t.Errorf("cram: seen two hours ago is not due, got %s", got)
	}
}

func TestPropsAreWrittenTheWayEmacsWritesThem(t *testing.T) {
	mq, ef := 4.0, 2.5
	d := DrillData{LastInterval: 4, Repeats: 2, TotalRepeats: 1, MeanQ: &mq, Ease: &ef}
	got := map[string]string{}
	for _, kv := range drillProps(d, 4, time.Date(2026, 10, 1, 14, 3, 0, 0, time.Local)) {
		got[kv[0]] = kv[1]
	}
	want := map[string]string{
		"DRILL_LAST_INTERVAL": "4.0", "DRILL_REPEATS_SINCE_FAIL": "2", "DRILL_TOTAL_REPEATS": "1",
		"DRILL_FAILURE_COUNT": "0", "DRILL_AVERAGE_QUALITY": "4.0", "DRILL_EASE": "2.5",
		"DRILL_LAST_QUALITY": "4", "DRILL_LAST_REVIEWED": "[2026-10-01 Thu 14:03]",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
	back := readDrillData(got)
	if back.LastInterval != 4 || back.Repeats != 2 || *back.Ease != 2.5 {
		t.Errorf("read back: %+v", back)
	}
}
