package orgs

import (
	"testing"
)

// A requirement taken from a query must be one that every match genuinely
// satisfies. Everything here is that, said in the ways it could be got wrong -
// the failure being a query that silently returns fewer results than it should,
// which nothing downstream could ever notice.

func TestNeedsOfAPlainConjunction(t *testing.T) {
	n := needsOf(`IsTask() && HasTags("work")`)
	if len(n.tags) != 1 || n.tags[0] != "work" {
		t.Errorf("expected the tag, got %v", n.tags)
	}
	n = needsOf(`IsStatus("NEXT")`)
	if len(n.keywords) != 1 || n.keywords[0] != "NEXT" {
		t.Errorf("expected the keyword, got %v", n.keywords)
	}
}

// HasTags is an and - it answers true only when the heading carries all of
// them - so every argument is a requirement.
func TestNeedsOfSeveralTags(t *testing.T) {
	n := needsOf(`HasTags("work", "urgent")`)
	if len(n.tags) != 2 {
		t.Fatalf("expected both tags, got %v", n.tags)
	}
}

// Tags are matched case insensitively by HasTag, so the requirement has to be
// folded the same way the summary is - or `HasTags("WORK")` skips every file.
func TestTagRequirementsAreFolded(t *testing.T) {
	n := needsOf(`HasTags("WORK")`)
	if len(n.tags) != 1 || n.tags[0] != "work" {
		t.Errorf("expected a folded tag, got %v", n.tags)
	}
}

// The important half: anything that makes a conjunct optional or inverts it
// must narrow nothing at all. In `A || B` neither side is mandatory, and under
// a `!` a requirement becomes its opposite.
func TestNothingIsTakenFromADisjunctionOrANegation(t *testing.T) {
	for _, q := range []string{
		`IsStatus("NEXT") || IsStatus("TODO")`,
		`HasTags("work") || IsProject()`,
		`!HasTags("work")`,
		`IsTask() && !HasTags("work")`,
		`!IsStatus("NEXT")`,
		`IsTask() || (HasTags("work") && IsProject())`,
	} {
		if n := needsOf(q); n.any() {
			t.Errorf("%q should have narrowed nothing, got keywords %v tags %v", q, n.keywords, n.tags)
		}
	}
}

// `!=` is a comparison, not a negation, and must not stop a query narrowing.
func TestNotEqualIsNotANegation(t *testing.T) {
	n := needsOf(`IsStatus("NEXT") && ChecklistLeft() != 0`)
	if len(n.keywords) != 1 {
		t.Errorf("expected the keyword to survive a !=, got %v", n.keywords)
	}
}

// A fragment left by splitting a parenthesised group is not a conjunct, and
// reading one as a requirement would take it out of whatever grouping it was
// in - which might have been a disjunction.
func TestFragmentsAreNotRequirements(t *testing.T) {
	for _, q := range []string{
		`(IsStatus("NEXT") && IsTask()) && IsProject()`,
		`IsTask() && (HasTags("work") && IsProject())`,
	} {
		n := needsOf(q)
		for _, k := range n.keywords {
			if k != "NEXT" {
				t.Errorf("%q gave an unexpected keyword %q", q, k)
			}
		}
		// Whatever it takes, it must be from a conjunct it recognised whole.
		if len(n.keywords) > 1 || len(n.tags) > 1 {
			t.Errorf("%q took too much: keywords %v tags %v", q, n.keywords, n.tags)
		}
	}
}

// A function this does not understand contributes nothing, which costs speed
// and never costs results.
func TestUnknownFunctionsNarrowNothing(t *testing.T) {
	for _, q := range []string{
		`IsProject()`, `MatchHeadline("kitchen")`, `DaysOld() > 30`,
		`HasBacklinks(2)`, `true`, ``, `OlderThan(30) && HasClock()`,
	} {
		if n := needsOf(q); n.any() {
			t.Errorf("%q should have narrowed nothing, got %v %v", q, n.keywords, n.tags)
		}
	}
}

// A query that happens to mention a call inside a string must not have it read
// as a requirement - the anchored match is what stops that.
func TestACallInsideAStringIsNotARequirement(t *testing.T) {
	n := needsOf(`MatchHeadline("IsStatus(\"NEXT\")")`)
	if n.any() {
		t.Errorf("a call inside a string was read as a requirement: %v %v", n.keywords, n.tags)
	}
}

// ---------------------------------------------------------------------------
// Whether a file could match
// ---------------------------------------------------------------------------

func summary(keywords, tags []string) fileSummary {
	s := fileSummary{keywords: map[string]bool{}, tags: map[string]bool{}}
	for _, k := range keywords {
		s.keywords[k] = true
	}
	for _, t := range tags {
		s.tags[t] = true
	}
	return s
}

func TestPossible(t *testing.T) {
	sum := summary([]string{"TODO", "NEXT"}, []string{"work", "home"})

	yes := needsOf(`IsStatus("NEXT")`)
	if !yes.possible(sum) {
		t.Error("a file holding the keyword was excluded")
	}
	no := needsOf(`IsStatus("WAITING")`)
	if no.possible(sum) {
		t.Error("a file with no such keyword was kept")
	}
	both := needsOf(`HasTags("work", "home")`)
	if !both.possible(sum) {
		t.Error("a file holding both tags was excluded")
	}
	missing := needsOf(`HasTags("work", "garden")`)
	if missing.possible(sum) {
		t.Error("a file missing one of the tags was kept")
	}
	// Nothing required: every file is possible.
	none := needsOf(`IsProject()`)
	if !(&none).possible(summary(nil, nil)) {
		t.Error("a query requiring nothing excluded a file")
	}
}

// A summary is a superset - every tag written anywhere in the file, because
// tags are inherited from a parent heading and from the file's own FILETAGS.
// Being a superset is the direction that is safe to be wrong in: it keeps files
// that turn out not to match, and never drops one that does.
func TestASummaryIsASuperset(t *testing.T) {
	// A file whose only `work` tag is on a parent heading still has to be kept
	// for a query about a child that inherits it.
	sum := summary([]string{"TODO"}, []string{"work"})
	n := needsOf(`HasTags("work")`)
	if !(&n).possible(sum) {
		t.Error("a file whose tag is inherited was excluded")
	}
}
