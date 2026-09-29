package orgs

import (
	"regexp"
	"regexp/syntax"
	"strings"
	"testing"
)

// The whole safety of the trigram index rests on one property: it may narrow
// the set of files, and it may never exclude one that could match. Everything
// here is that property, said in the several ways it could be got wrong.

func tris(s string) []trigram {
	return trigramsOf([]byte(s))
}

func hasTri(ts []trigram, s string) bool {
	want := tri(s[0], s[1], s[2])
	for _, t := range ts {
		if t == want {
			return true
		}
	}
	return false
}

func TestTrigramsOfText(t *testing.T) {
	got := tris("hello")
	for _, want := range []string{"hel", "ell", "llo"} {
		if !hasTri(got, want) {
			t.Errorf("missing %q", want)
		}
	}
	if len(got) != 3 {
		t.Errorf("expected three trigrams, got %d", len(got))
	}
}

// Anything shorter than a trigram has none, and must not therefore be treated
// as matching everything.
func TestTrigramsOfSomethingTooShort(t *testing.T) {
	for _, s := range []string{"", "a", "ab"} {
		if got := tris(s); len(got) != 0 {
			t.Errorf("%q gave %v", s, got)
		}
	}
}

// The index is case folded on both sides, so that one index serves a case
// sensitive search and an insensitive one. Folding can only make it match too
// many files, which the real expression throws out.
func TestTrigramsAreCaseFolded(t *testing.T) {
	a, b := tris("Hello"), tris("hELLO")
	if len(a) != len(b) {
		t.Fatalf("different counts: %d vs %d", len(a), len(b))
	}
	for _, x := range a {
		found := false
		for _, y := range b {
			if x == y {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("case folding did not agree")
		}
	}
}

// ---------------------------------------------------------------------------
// What a pattern requires
// ---------------------------------------------------------------------------

func TestRequiredTrigramsOfAPlainWord(t *testing.T) {
	got := requiredTrigrams("migration")
	if len(got) == 0 {
		t.Fatal("a plain word should give trigrams")
	}
	if !hasTri(got, "mig") || !hasTri(got, "ion") {
		t.Errorf("expected the word's own trigrams, got %d", len(got))
	}
}

// A pattern the index cannot reason about must give nothing back, which is the
// signal to scan everything. Answering anything else would exclude files that
// could match - the one failure the construction has to make impossible.
func TestPatternsTheIndexCannotHelpWith(t *testing.T) {
	for _, p := range []string{
		".",      // matches anything
		"a",      // too short for a trigram
		"ab",     //
		"a|b",    // either, so neither is required
		"[a-z]+", // a class requires no particular byte
		"^",      //
		".*",     //
		"a?b?c?", // every byte optional, so none is required
	} {
		if got := requiredTrigrams(p); len(got) != 0 {
			t.Errorf("%q should have given nothing, got %d trigrams", p, got)
		}
	}
}

// An unparseable pattern is an ordinary state of a box somebody is typing in,
// and must be nothing rather than a panic.
func TestRequiredTrigramsOfNonsense(t *testing.T) {
	if got := requiredTrigrams("[unclosed"); len(got) != 0 {
		t.Errorf("got %v", got)
	}
}

// The run has to be a *mandatory* one. Where a pattern has a literal run and
// then something optional, only the run counts.
func TestRequiredTrigramsTakesTheMandatoryRun(t *testing.T) {
	got := requiredTrigrams("migration[0-9]*")
	if !hasTri(got, "mig") {
		t.Error("the literal run should be used")
	}
	// A run either side of a class: the longest is the one worth having.
	got = requiredTrigrams("ab[0-9]+kitchen")
	if !hasTri(got, "kit") {
		t.Error("expected the longer run")
	}
	if hasTri(got, "ab0") {
		t.Error("a trigram was made across the class boundary")
	}
}

// A run after an alternation, or before an optional character, is still
// mandatory - and using it is the difference between narrowing and not.
// `(cat|dog)house` must contain "house"; `colou?r` must contain "colo".
func TestRequiredTrigramsFindsARunPastSomethingOptional(t *testing.T) {
	if got := requiredTrigrams("(cat|dog)house"); !hasTri(got, "hou") {
		t.Errorf("expected the run after the alternation, got %d trigrams", len(got))
	}
	if got := requiredTrigrams("colou?r"); !hasTri(got, "col") {
		t.Errorf("expected the run before the optional character, got %d trigrams", len(got))
	}
	// And neither may demand a trigram that spans the optional part.
	if hasTri(requiredTrigrams("colou?r"), "our") {
		t.Error("a trigram was made across the optional character")
	}
}

// `(?i)` is how the search asks for case insensitivity, and it must not stop
// the index working - the index is folded anyway.
func TestRequiredTrigramsThroughCaseInsensitivity(t *testing.T) {
	got := requiredTrigrams("(?i)Migration")
	if len(got) == 0 {
		t.Fatal("a case insensitive literal should still give trigrams")
	}
	if !hasTri(got, "mig") {
		t.Errorf("expected folded trigrams, got %d", len(got))
	}
}

// The property the whole thing rests on: every trigram the index demands must
// genuinely appear in any text the pattern matches. If this can be broken, a
// search silently loses results.
//
// Checked by construction over a spread of patterns and a spread of text: for
// every string the pattern matches, the text must hold every required trigram.
func TestARequiredTrigramIsAlwaysPresentInAMatch(t *testing.T) {
	patterns := []string{
		"migration", "(?i)Kitchen", "the plan", "abc.*def", "foo[0-9]bar",
		"a+bcdef", "scheduled", "^notes", "END:$", "one|onetwothree",
		"colou?rful", "[[:alpha:]]{3}kitchen",
	}
	corpus := []string{
		"", "migration", "a migration plan", "MIGRATION", "the plan is",
		"abcXYZdef", "foo7bar", "aaabcdef", "scheduled for later",
		"notes about things", "  :END:", "one", "onetwothree",
		"colorful", "colourful", "xyzkitchen", "the Kitchen rebuild",
		"nothing in here at all", strings.Repeat("padding ", 20) + "kitchen",
	}
	for _, p := range patterns {
		re, err := regexp.Compile(p)
		if err != nil {
			t.Fatalf("%q: %v", p, err)
		}
		req := requiredTrigrams(p)
		if len(req) == 0 {
			continue // scans everything, so nothing to prove
		}
		for _, text := range corpus {
			if !re.MatchString(text) {
				continue
			}
			have := tris(text)
			for _, need := range req {
				found := false
				for _, h := range have {
					if h == need {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("pattern %q matches %q but the index would have excluded it: missing trigram %06x",
						p, text, uint32(need))
				}
			}
		}
	}
}

// ---------------------------------------------------------------------------
// The intersection
// ---------------------------------------------------------------------------

func TestIntersect(t *testing.T) {
	cases := []struct{ a, b, want []int32 }{
		{[]int32{1, 2, 3}, []int32{2, 3, 4}, []int32{2, 3}},
		{[]int32{1, 2, 3}, []int32{4, 5}, []int32{}},
		{[]int32{}, []int32{1}, []int32{}},
		{[]int32{1, 5, 9}, []int32{1, 5, 9}, []int32{1, 5, 9}},
	}
	for _, c := range cases {
		a := append([]int32{}, c.a...)
		got := intersect(a, c.b)
		if len(got) != len(c.want) {
			t.Errorf("%v ∩ %v = %v, want %v", c.a, c.b, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%v ∩ %v = %v, want %v", c.a, c.b, got, c.want)
				break
			}
		}
	}
}

// ---------------------------------------------------------------------------
// The index itself
// ---------------------------------------------------------------------------

func newTestIndex() *trigramIndex {
	return &trigramIndex{at: map[string]int{}, post: map[trigram][]int32{}}
}

func TestIndexNarrowsToTheRightFiles(t *testing.T) {
	idx := newTestIndex()
	idx.mu.Lock()
	idx.addLocked("/a.org", tris("the kitchen rebuild"), 1, 1, 1)
	idx.addLocked("/b.org", tris("shopping list milk"), 1, 1, 1)
	idx.addLocked("/c.org", tris("kitchen again"), 1, 1, 1)
	idx.mu.Unlock()

	all := []string{"/a.org", "/b.org", "/c.org"}
	got := idx.candidates("kitchen", all)
	if len(got) != 2 || !got["/a.org"] || !got["/c.org"] {
		t.Errorf("expected a and c, got %v", got)
	}
	if n := idx.candidates("milk", all); len(n) != 1 || !n["/b.org"] {
		t.Errorf("expected b, got %v", n)
	}
	// Nothing holds it: an empty set, not nil - "nothing can match" is an
	// answer, and nil would mean "look at everything".
	none := idx.candidates("aardvark", all)
	if none == nil {
		t.Error("a word nothing holds should answer with an empty set, not nil")
	}
	if len(none) != 0 {
		t.Errorf("got %v", none)
	}
	// A pattern it cannot reason about: nil, meaning scan everything.
	if idx.candidates(".", all) != nil {
		t.Error("a pattern with no mandatory run should answer nil")
	}
}

// A search arriving before the index has read a file must scan that file rather
// than skip it.
//
// This is the way an index of this shape is silently and badly wrong: a file
// the index has never read is in no posting list, so an answer built out of the
// lists alone excludes it - and the result is not a slow search but a search
// that finds nothing, for every file, for as long as the build takes, and
// forever for any file that could not be read.
func TestAnUnindexedFileIsStillSearched(t *testing.T) {
	idx := newTestIndex()
	idx.mu.Lock()
	idx.addLocked("/known.org", tris("kitchen rebuild"), 1, 1, 1)
	idx.mu.Unlock()

	got := idx.candidates("kitchen", []string{"/known.org", "/notyet.org"})
	if got == nil {
		t.Fatal("with one file indexed it should narrow, not give up")
	}
	if !got["/notyet.org"] {
		t.Error("a file the index has never read was excluded from the search")
	}
	if !got["/known.org"] {
		t.Error("the file that holds it was excluded")
	}
	// And a word nothing indexed holds still has to let the unread file through.
	got = idx.candidates("aardvark", []string{"/known.org", "/notyet.org"})
	if got["/known.org"] {
		t.Error("an indexed file that cannot match was returned")
	}
	if !got["/notyet.org"] {
		t.Error("the unread file was excluded")
	}
}

// Nothing indexed at all is a cold start, and the honest answer is "look at
// everything" rather than a set that happens to be everything.
func TestAColdIndexGivesUp(t *testing.T) {
	idx := newTestIndex()
	if got := idx.candidates("kitchen", []string{"/a.org", "/b.org"}); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

// A file read again must replace its own entry rather than adding a second, or
// its old text goes on answering queries forever.
func TestReindexingAFileReplacesIt(t *testing.T) {
	idx := newTestIndex()
	idx.mu.Lock()
	idx.addLocked("/a.org", tris("kitchen"), 1, 1, 1)
	idx.mu.Unlock()
	if len(idx.candidates("kitchen", []string{"/a.org"})) != 1 {
		t.Fatal("setup")
	}

	idx.mu.Lock()
	idx.removeLocked("/a.org")
	idx.addLocked("/a.org", tris("bathroom"), 1, 1, 2)
	idx.mu.Unlock()

	if got := idx.candidates("kitchen", []string{"/a.org"}); len(got) != 0 {
		t.Errorf("the old text is still answering: %v", got)
	}
	if got := idx.candidates("bathroom", []string{"/a.org"}); len(got) != 1 {
		t.Errorf("the new text is not: %v", got)
	}
}

// A removed file's slot is reused, and the number that used to mean it must not
// go on being returned for the trigrams it used to hold.
func TestARemovedFileStopsAnswering(t *testing.T) {
	idx := newTestIndex()
	idx.mu.Lock()
	idx.addLocked("/a.org", tris("kitchen"), 1, 1, 1)
	idx.addLocked("/b.org", tris("bathroom"), 1, 1, 1)
	idx.removeLocked("/a.org")
	idx.mu.Unlock()

	if got := idx.candidates("kitchen", []string{"/b.org"}); len(got) != 0 {
		t.Errorf("a removed file is still answering: %v", got)
	}
	// And its slot is handed to the next file, whose own trigrams must be the
	// only ones it answers for.
	idx.mu.Lock()
	idx.addLocked("/c.org", tris("cellar"), 1, 1, 1)
	idx.mu.Unlock()
	if got := idx.candidates("kitchen", []string{"/b.org", "/c.org"}); len(got) != 0 {
		t.Errorf("the reused slot answers for the old file: %v", got)
	}
	if got := idx.candidates("cellar", []string{"/b.org", "/c.org"}); len(got) != 1 || !got["/c.org"] {
		t.Errorf("the reused slot does not answer for its own file: %v", got)
	}
}

// The posting lists have to stay ascending or the intersection quietly loses
// entries - and a reused file number can land anywhere in one.
func TestPostingListsStayAscending(t *testing.T) {
	idx := newTestIndex()
	idx.mu.Lock()
	idx.addLocked("/a.org", tris("kitchen"), 1, 1, 1)
	idx.addLocked("/b.org", tris("kitchen"), 1, 1, 1)
	idx.addLocked("/c.org", tris("kitchen"), 1, 1, 1)
	idx.removeLocked("/b.org") // frees slot 1
	idx.addLocked("/d.org", tris("kitchen"), 1, 1, 1)
	for t2, p := range idx.post {
		for i := 1; i < len(p); i++ {
			if p[i] < p[i-1] {
				idx.mu.Unlock()
				t.Fatalf("posting list for %06x is not ascending: %v", uint32(t2), p)
			}
		}
	}
	idx.mu.Unlock()
	if got := idx.candidates("kitchen", []string{"/a.org", "/c.org", "/d.org"}); len(got) != 3 {
		t.Errorf("expected three files, got %v", got)
	}
}

// A literal longer than the cap uses only the first few of its trigrams. That
// is a choice about cost, and it has to stay on the safe side: fewer required
// trigrams means a wider candidate set, never a narrower one.
func TestALongLiteralIsCappedButStillCorrect(t *testing.T) {
	long := "extraordinarily-long-search-term-here"
	if n := len(requiredTrigrams(long)); n < trigramMaxQuery {
		t.Fatalf("setup: expected more than %d trigrams, got %d", trigramMaxQuery, n)
	}
	idx := newTestIndex()
	idx.mu.Lock()
	idx.addLocked("/a.org", tris("a file with "+long+" in it"), 1, 1, 1)
	idx.mu.Unlock()
	if got := idx.candidates(long, []string{"/a.org"}); len(got) != 1 {
		t.Errorf("the file holding it was excluded: %v", got)
	}
}

// Sanity: the parser this all rests on behaves the way the literal walk assumes.
func TestSyntaxAssumptions(t *testing.T) {
	re, err := syntax.Parse("(?i)Kitchen", syntax.Perl)
	if err != nil {
		t.Fatal(err)
	}
	if lit := longestLiteral(re.Simplify()); !strings.EqualFold(lit, "kitchen") {
		t.Errorf("a case insensitive literal came back as %q", lit)
	}
}
