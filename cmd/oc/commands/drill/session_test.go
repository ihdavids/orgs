package drill

// The same scenarios as worg/src/drill.test.ts: the two sessions must agree.

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ihdavids/orgs/internal/common"
)

func card(hash, status string, mod ...func(*common.DrillCard)) common.DrillCard {
	c := common.DrillCard{Hash: hash, Name: hash, Type: "simple", Status: status, LastQuality: -1}
	for _, m := range mod {
		m(&c)
	}
	return c
}

func reply(cards []common.DrillCard, deck common.DrillDeck) common.DrillCardsReply {
	deck.Name = "d"
	return common.DrillCardsReply{Ok: true, Deck: deck.WithDefaults(), Cards: cards, Counts: common.DrillCounts{DueTomorrow: 2}}
}

var t0 = time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC)

func TestOrderFailedOverdueYoungThenNew(t *testing.T) {
	rng := NewRng(1)
	s := NewSession(reply([]common.DrillCard{
		card("n1", "new"), card("y1", "young"), card("f1", "failed"),
		card("o1", "overdue", func(c *common.DrillCard) { c.Due = 3 }),
		card("o2", "overdue", func(c *common.DrillCard) { c.Due = 9 }),
		card("later", "future"),
	}, common.DrillDeck{}), false, t0, rng)
	order := []string{}
	for h := s.Next(t0, rng); h != ""; h = s.Next(t0, rng) {
		order = append(order, h)
		s.Rate(h, 4, rng)
	}
	if !reflect.DeepEqual(order, []string{"f1", "o2", "o1", "y1", "n1"}) {
		t.Errorf("order %v", order)
	}
	if s.Dormant != 1 {
		t.Errorf("dormant %d", s.Dormant)
	}
}

func TestFailedComesBackUntilPassed(t *testing.T) {
	rng := NewRng(2)
	s := NewSession(reply([]common.DrillCard{card("a", "new"), card("b", "new")}, common.DrillDeck{}), false, t0, rng)
	first := s.Next(t0, rng)
	s.Rate(first, 1, rng)
	second := s.Next(t0, rng)
	if second == first {
		t.Fatal("the failure came straight back")
	}
	s.Rate(second, 5, rng)
	for _, q := range []int{1, 4} {
		if h := s.Next(t0, rng); h != first {
			t.Fatalf("got %q, want the failed card", h)
		}
		s.Rate(first, q, rng)
	}
	if h := s.Next(t0, rng); h != "" {
		t.Errorf("left over: %q", h)
	}
	if !reflect.DeepEqual(s.Qualities, []int{1, 5, 1, 4}) {
		t.Errorf("qualities %v", s.Qualities)
	}
}

func TestLimitsStopNewButNotAgain(t *testing.T) {
	rng := NewRng(3)
	s := NewSession(reply([]common.DrillCard{card("a", "new"), card("b", "new"), card("c", "new")}, common.DrillDeck{MaxItems: 1}), false, t0, rng)
	a := s.Next(t0, rng)
	s.Rate(a, 0, rng)
	b := s.Next(t0, rng)
	s.Rate(b, 4, rng)
	if !s.LimitReached(t0) {
		t.Fatal("limit not reached")
	}
	if h := s.Next(t0, rng); h != a {
		t.Fatalf("again pile: %q", h)
	}
	s.Rate(a, 4, rng)
	if h := s.Next(t0, rng); h != "" {
		t.Errorf("past the limit: %q", h)
	}
	m := NewSession(reply([]common.DrillCard{card("x", "new")}, common.DrillDeck{MaxMinutes: 1}), false, t0, rng)
	if !m.LimitReached(t0.Add(61*time.Second)) || m.Next(t0.Add(61*time.Second), rng) != "" {
		t.Error("minutes limit")
	}
}

func TestCramCountsNothing(t *testing.T) {
	rng := NewRng(4)
	s := NewSession(reply([]common.DrillCard{card("a", "new")}, common.DrillDeck{MaxItems: 1}), true, t0, rng)
	s.Done = []string{"x", "y"}
	if s.LimitReached(t0.Add(1000 * time.Hour)) {
		t.Error("cram has no limits")
	}
	s.Rate(s.Next(t0, rng), 5, rng)
	if len(s.Qualities) != 0 {
		t.Error("cram counted a rating")
	}
}

func TestOverdueOrder(t *testing.T) {
	mk := func(h string, due int, age float64) *common.DrillCard {
		return &common.DrillCard{Hash: h, Due: due, Age: age}
	}
	cs := []*common.DrillCard{mk("a", 5, 0), mk("b", 200, 10), mk("c", 30, 0), mk("d", 150, 400)}
	got := func(lapse bool) string {
		out := []string{}
		for _, c := range OrderOverdue(cs, lapse, NewRng(5)) {
			out = append(out, c.Hash)
		}
		return strings.Join(out, "")
	}
	if g := got(false); g != "bdca" {
		t.Errorf("no lapse: %s", g)
	}
	if g := got(true); g != "cadb" {
		t.Errorf("lapse: %s", g)
	}
}

func TestHideClozes(t *testing.T) {
	rng := NewRng(6)
	if len(HideClozes(4, 1, rng, hideOpts{})) != 1 || len(HideClozes(4, -1, rng, hideOpts{})) != 3 || len(HideClozes(2, 3, rng, hideOpts{})) != 0 {
		t.Error("counts")
	}
	for i := 0; i < 20; i++ {
		if HideClozes(5, 1, rng, hideOpts{forceShowFirst: true})[1] {
			t.Error("forceShowFirst hid the first")
		}
		if h := HideClozes(5, 1, rng, hideOpts{forceHideFirst: true}); !h[1] || len(h) != 1 {
			t.Error("forceHideFirst")
		}
		if HideClozes(5, -1, rng, hideOpts{forceShowLast: true})[5] {
			t.Error("forceShowLast hid the last")
		}
	}
}

func TestCardTypes(t *testing.T) {
	rng := NewRng(7)
	sides := []common.DrillSide{{Name: "English"}, {Name: "Spanish"}, {Name: "Notes"}}
	for i := 0; i < 20; i++ {
		c := card("t", "new", func(c *common.DrillCard) { c.Type = "twosided"; c.Sides = sides })
		if Present(&c, rng).Side >= 2 {
			t.Error("twosided asked a third side")
		}
	}
	c := card("h", "new", func(c *common.DrillCard) { c.Type = "hidelast"; c.Clozes = 3 })
	if h := Present(&c, rng).Hide; !reflect.DeepEqual(h, map[int]bool{3: true}) {
		t.Errorf("hidelast %v", h)
	}
	v := card("c", "new", func(c *common.DrillCard) {
		c.Type = "conjugate"
		c.Props = map[string]string{"VERB_INFINITIVE": "hablar", "VERB_TRANSLATION": "to speak", "VERB_TENSE": "present"}
	})
	if p := Present(&v, rng); p.AnswerHeading != "Present of hablar ==> to speak" {
		t.Errorf("conjugate %q", p.AnswerHeading)
	}
}

func TestClozeOrg(t *testing.T) {
	text := "The capital of [Spain||country] is [Madrid] - done [X] and [[http://x][a link]]."
	q := ClozeOrg(text, 1, map[int]bool{1: true}, false, false)
	if q != "The capital of ~[country...]~ is Madrid - done [X] and [[http://x][a link]]." {
		t.Errorf("question: %s", q)
	}
	if a := ClozeOrg(text, 1, nil, true, false); a != "The capital of *Spain* is *Madrid* - done [X] and [[http://x][a link]]." {
		t.Errorf("answer: %s", a)
	}
	if d := ClozeOrg("[Madrid]", 1, map[int]bool{1: true}, false, true); d != "~[......]~" {
		t.Errorf("dots: %s", d)
	}
	if z := ClozeOrg("a [b] c", 0, map[int]bool{}, false, false); z != "a ~[...]~ c" {
		t.Errorf("number 0 is always hidden in the question: %s", z)
	}
}

func TestReport(t *testing.T) {
	rng := NewRng(8)
	s := NewSession(reply([]common.DrillCard{card("a", "new"), card("b", "new"), card("z", "future")}, common.DrillDeck{}), false, t0, rng)
	a := s.Next(t0, rng)
	s.Rate(a, 5, rng)
	b := s.Next(t0, rng)
	s.Rate(b, 1, rng)
	r := s.MakeReport(t0.Add(90 * time.Second))
	if r.Reviewed != 1 || r.Percent[5] != 50 || r.PassPercent != 50 || !r.Warn || r.Failed != 1 || r.PendingAll != r.Pending+1 || r.DueTomorrow != 2 {
		t.Errorf("%+v", r)
	}
}

func TestClockAndDays(t *testing.T) {
	if Clock(65*time.Second) != "01:05" || Clock(time.Hour) != "++:++" {
		t.Error("clock")
	}
	if Days(0) != "again" || Days(1.2) != "+1 day" || Days(3.86) != "+4 days" {
		t.Error("days")
	}
}
