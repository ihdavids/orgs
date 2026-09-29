package orgs

// Skipping the files a query cannot possibly match.
//
// `/search` evaluates the query expression against every heading in the
// database - seven thousand of them here, about four milliseconds of govaluate
// per query, and linear in the database. Most queries cannot match most files:
// `IsStatus("NEXT")` cannot match a file with no NEXT in it, and
// `HasTags("work")` cannot match a file where the word never appears as a tag.
//
// So each file gets a summary - which todo keywords it holds, which tags - and
// a query that demands one of those can skip every file whose summary says no.
// The expression is still evaluated, unchanged, on every heading of every file
// that survives. This is the same shape as the trigram index over the file text
// and it is safe for the same reason: **it only ever narrows**, and the only
// way it could be wrong is by excluding a file that could match, which is what
// everything below is arranged to make impossible.
//
// Two rules keep it honest, and both are about refusing to be clever:
//
//  1. **A requirement is only taken from a query that is a plain conjunction.**
//     Anything with `||` or `!` in it narrows nothing at all. In `A || B`
//     neither side is mandatory, and under a `!` a requirement becomes its
//     opposite - so rather than reason about either, a query containing one is
//     handed the whole database exactly as before.
//  2. **A conjunct has to be recognised exactly.** `IsStatus("NEXT")` is a
//     requirement; `(IsStatus("NEXT")` - a fragment left by splitting a
//     parenthesised group - is not, and contributes nothing. Failing to
//     recognise something costs speed; misreading something costs results.

import (
	"regexp"
	"strings"

	"github.com/ihdavids/orgs/internal/common"
)

// What one file holds, in the two ways a query can ask about it.
//
// A superset in both cases, deliberately. Tags are inherited - from a parent
// heading and from the file's own `#+FILETAGS:` - so "every tag written
// anywhere in this file" is more than any single heading has, and a heading can
// only carry a tag that appears somewhere in the set. That is the direction
// that is safe to be wrong in.
type fileSummary struct {
	keywords map[string]bool
	tags     map[string]bool
}

var summaryParts = NewFileParts[fileSummary]()

func fileSummaries() []fileSummary {
	return summaryParts.All(func(f *common.OrgFile) fileSummary {
		sum := fileSummary{keywords: map[string]bool{}, tags: map[string]bool{}}
		// The file's own tags reach every heading in it.
		for _, t := range strings.Split(f.Doc.Get("FILETAGS"), ":") {
			if t = strings.ToLower(strings.TrimSpace(t)); t != "" {
				sum.tags[t] = true
			}
		}
		for _, sec := range flattenSections(f) {
			if sec == nil || sec.Headline == nil {
				continue
			}
			if s := sec.Headline.Status; s != "" {
				sum.keywords[s] = true
			}
			for _, t := range sec.Headline.Tags {
				if t = strings.ToLower(strings.TrimSpace(t)); t != "" {
					sum.tags[t] = true
				}
			}
		}
		return sum
	})
}

// What a query demands of a file before any heading in it is worth looking at.
type queryNeeds struct {
	keywords []string
	tags     []string
}

func (self *queryNeeds) any() bool {
	return len(self.keywords) > 0 || len(self.tags) > 0
}

// Can this file possibly hold a match?
func (self *queryNeeds) possible(sum fileSummary) bool {
	for _, k := range self.keywords {
		if !sum.keywords[k] {
			return false
		}
	}
	for _, t := range self.tags {
		if !sum.tags[t] {
			return false
		}
	}
	return true
}

// A conjunct that is exactly one call with string arguments, and nothing else.
//
// Anchored at both ends on purpose: a fragment like `(IsStatus("NEXT")` left
// over from splitting a parenthesised group must not be read as a requirement,
// because the grouping it came from might have been a disjunction.
var callRe = regexp.MustCompile(`^\s*([A-Za-z]+)\s*\(\s*((?:"[^"]*"\s*,?\s*)*)\)\s*$`)
var argRe = regexp.MustCompile(`"([^"]*)"`)

// Anything that makes a conjunct optional or inverts it. `!=` is a comparison
// rather than a negation, so it is allowed through - and `|` alone is not `||`.
var disjunctionRe = regexp.MustCompile(`\|\||![^=]|!$`)

// What a query requires of a file, or nothing when it cannot be told.
//
// Nothing is the answer for anything with a `||` or a `!` in it, for a query
// with no recognised conjunct, and for every query this does not understand -
// which is most of the forty-six functions, and is fine: an unnarrowed query
// costs exactly what every query cost before this file existed.
func needsOf(query string) queryNeeds {
	var needs queryNeeds
	if disjunctionRe.MatchString(query) {
		return needs
	}
	for _, part := range strings.Split(query, "&&") {
		m := callRe.FindStringSubmatch(part)
		if m == nil {
			continue
		}
		args := []string{}
		for _, a := range argRe.FindAllStringSubmatch(m[2], -1) {
			args = append(args, a[1])
		}
		switch m[1] {
		case "IsStatus":
			// `IsStatus("NEXT")` is `Headline.Status == "NEXT"`, so the file has
			// to hold a heading with that keyword.
			if len(args) == 1 && args[0] != "" {
				needs.keywords = append(needs.keywords, args[0])
			}
		case "HasTags":
			// Every argument, because HasTags is an and: it answers true only
			// when the heading carries all of them.
			for _, a := range args {
				if a = strings.ToLower(strings.TrimSpace(a)); a != "" {
					needs.tags = append(needs.tags, a)
				}
			}
		case "IsPriority":
			// Not narrowed on: a priority is not in the summary, and adding it
			// would be a third map for a question nobody asks on its own.
		}
	}
	return needs
}

// The files worth walking for this query, and how many were skipped.
//
// The order is the database's own, which several callers depend on - the search
// results are presented in file order and a client paging through them would
// see rows move if it changed.
func filesForQuery(query string) (files []string, skipped int) {
	all := GetDb().GetFiles()
	needs := needsOf(query)
	if !needs.any() {
		return all, 0
	}
	sums := fileSummaries()
	// A summary per file, in the same order - but only where the two agree
	// about how many there are. They are built from the same list a moment
	// apart, and a file appearing in between would put every summary after it
	// against the wrong file: a mismatch is rare, harmless to ignore, and the
	// one thing that could make this exclude the wrong file.
	if len(sums) != len(all) {
		return all, 0
	}
	out := make([]string, 0, len(all))
	for i, name := range all {
		if needs.possible(sums[i]) {
			out = append(out, name)
			continue
		}
		skipped++
	}
	return out, skipped
}
