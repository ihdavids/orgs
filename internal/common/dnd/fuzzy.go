package dnd

// Fuzzy list filtering, shared by the terminal chooser in the cli and the
// item search behind the html character sheet's inventory. Both filter a long
// list of named things by typing at it, and both want the same answer for the
// same letters, so the matcher lives here rather than in either client.

import (
	"sort"
	"strings"
)

// FuzzyMatches returns the indexes of the options a filter matches, best
// first. labels are what is shown - a name followed by a summary - and names,
// when given, is just the name part of each label. An empty filter keeps the
// list in the order it came in, which matters because that order is
// meaningful: recommended options come first.
func FuzzyMatches(filter string, labels, names []string) []int {
	filter = strings.TrimSpace(filter)
	out := []int{}
	if filter == "" {
		for i := range labels {
			out = append(out, i)
		}
		return out
	}
	type hit struct {
		idx, score int
	}
	hits := []hit{}
	terms := strings.Fields(strings.ToLower(filter))
	for i := range labels {
		name := labels[i]
		if i < len(names) && names[i] != "" {
			name = names[i]
		}
		score, ok := FuzzyScoreAll(terms, name, labels[i])
		if ok {
			hits = append(hits, hit{i, score})
		}
	}
	sort.SliceStable(hits, func(a, b int) bool { return hits[a].score > hits[b].score })
	for _, h := range hits {
		out = append(out, h.idx)
	}
	return out
}

// FuzzyScoreAll requires every term to match somewhere, so "fire bolt" and
// "bolt fire" both find the cantrip.
//
// A term matches the option's name loosely - "eldbl" is Eldritch Blast - but
// the summary that follows it only counts on a whole word. Fuzzy matching over
// a sentence matches almost anything, which would leave a filter that reads
// like it should have narrowed the list showing half of it.
func FuzzyScoreAll(terms []string, name, label string) (int, bool) {
	rest := strings.ToLower(label)
	if i := strings.Index(rest, strings.ToLower(name)); i >= 0 {
		rest = rest[i+len(name):]
	}
	total := 0
	for _, t := range terms {
		if n, ok := FuzzyScore(t, name); ok {
			total += n * 2
			continue
		}
		if i := strings.Index(rest, t); i >= 0 {
			// worth having, but always below anything named for it
			total += 20 - i/8
			continue
		}
		return 0, false
	}
	return total, true
}

// FuzzyScore matches pattern against text as a subsequence and scores how good
// the match is: adjacent letters, letters that start a word and a match right
// at the front all count for more, and a big gap between letters counts for
// less. Both arguments are compared case insensitively.
func FuzzyScore(pattern, text string) (int, bool) {
	if pattern == "" {
		return 0, true
	}
	pat := []rune(strings.ToLower(pattern))
	txt := []rune(strings.ToLower(text))
	score, pi, last := 0, 0, -1
	for ti := 0; ti < len(txt) && pi < len(pat); ti++ {
		if txt[ti] != pat[pi] {
			continue
		}
		score += 10
		switch {
		case ti == 0:
			score += 20
		case last == ti-1:
			score += 12 // running on from the previous letter
		case isBoundary(txt[ti-1]):
			score += 10 // the start of a word
		}
		if last >= 0 && ti-last > 1 {
			gap := ti - last - 1
			if gap > 6 {
				gap = 6
			}
			score -= gap
		}
		last = ti
		pi++
	}
	if pi < len(pat) {
		return 0, false
	}
	// A short option that matched is more likely to be what was meant than a
	// long one that happens to contain the same letters.
	score -= len(txt) / 12
	return score, true
}

func isBoundary(r rune) bool {
	switch r {
	case ' ', '-', '_', '/', '(', ')', ',', '.', '\'', ':':
		return true
	}
	return false
}
