package common

// The two things a table of contents (toc-org, org-make-toc) and the html
// exporter must agree on, or a TOC link written into a file goes nowhere on
// the exported page: the text a heading is shown by, and the github-style
// anchor made from it. The TOC itself is worked out in internal/app/orgs/toc.go;
// its TypeScript twin is vscode-org-everywhere/src/toc-model.ts.

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

var (
	tocLinkRe   = regexp.MustCompile(`\[\[([^\]]*)\](?:\[([^\]]*)\])?\]`)
	tocCookieRe = regexp.MustCompile(`\[\d*/\d*\]|\[\d*%\]`)
)

// TocVisibleTitle is a heading's title as a reader sees it: a link is its
// description (or its target when it has none), statistics cookies are gone
// (they change as boxes are ticked, and a TOC that changed with them would be
// rewritten on every tick), and runs of white space are one space. The
// keyword, priority and tags must already be off.
func TocVisibleTitle(title string) string {
	t := tocLinkRe.ReplaceAllStringFunc(title, func(m string) string {
		sm := tocLinkRe.FindStringSubmatch(m)
		if sm[2] != "" {
			return sm[2]
		}
		return sm[1]
	})
	t = tocCookieRe.ReplaceAllString(t, "")
	return strings.Join(strings.Fields(t), " ")
}

// TocGhAnchor is toc-org's `toc-org-hrefify-gh` (and github's): spaces become
// hyphens, lower case, and anything but letters, digits, `_` and `-` dropped.
// No leading `#`.
func TocGhAnchor(visible string) string {
	s := strings.ToLower(strings.ReplaceAll(visible, " ", "-"))
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// TocAnchors gives every heading of a document its anchor, in document
// order: the second "Notes" is `notes-1`, the third `notes-2`, as github and
// toc-org number them. Every heading counts, shown or not, so the numbering
// does not depend on which TOC is asking.
func TocAnchors(visibles []string) []string {
	seen := map[string]bool{}
	out := make([]string, len(visibles))
	for i, v := range visibles {
		base := TocGhAnchor(v)
		a := base
		for n := 1; seen[a]; n++ {
			a = base + "-" + strconv.Itoa(n)
		}
		seen[a] = true
		out[i] = a
	}
	return out
}
