package orgs

// Tables of contents, worked out from a file's lines (pure; pinned by
// toc_test.go). Three ways of asking for one, each kept current by the same
// code (tocserve.go has the endpoints and the update on save):
//
//   - toc-org: the first heading tagged :TOC: (:TOC_3:, :TOC_2_gh:,
//     :TOC_2_org:, `@` for `_` too) has its whole body - everything after its
//     planning line and drawers, up to the next heading - replaced by the
//     list, as toc-org replaces it. Depth 2 and github links unless the tag
//     says otherwise; :QUOTE: beside it puts the list in a quote block.
//   - org-make-toc: a heading with a :TOC: property (`:include all :depth 2
//     :ignore this`) has the list written into its :CONTENTS: drawer, the
//     text around the drawer kept. No drawer, no TOC: a :TOC: property
//     alone (`:ignore this`) is a setting for the TOCs above it; inserting
//     one (tocInsertLines) makes the drawer.
//   - `#+BEGIN: toc :depth 2` … `#+END:`, a dynamic block (dynblock.go's
//     generator calls tocBlockLines).
//
// The TypeScript twin is vscode-org-everywhere/src/toc-model.ts, with the
// same cases in test/toc-model.test.ts. Change both.

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/ihdavids/orgs/internal/common"
)

// One heading, as the TOC needs it.
type tocHead struct {
	Row, Level int
	Next       int // row of the next heading of any level (len(lines) if none)
	HeadEnd    int // first row after the headline, its planning and its drawers
	Parent     int // index of the parent heading, -1 at the top
	Title      string
	Visible    string
	Anchor     string
	Tags       []string
	Comment    bool
	CustomID   string
	TocProp    string
	HasTocProp bool
}

// One place a TOC goes, and what goes there.
type TocSite struct {
	Kind    string // "toc-org", "org-make-toc" or "dblock"
	Heading string `json:",omitempty"` // the heading it is under ("" above the first)
	Row     int    // the headline's row (the #+BEGIN: row for a block), from 0
	From    int    // rows From..To-1 are replaced (From == To: inserted at From)
	To      int
	Text    string // what replaces them, lines joined with \n
	Entries int
	Changed bool
	Lines   []string `json:"-"` // Text as lines (Text alone cannot tell one blank line from none)
}

var (
	tocHeadRe     = regexp.MustCompile(`^(\*+)(?:[ \t]+(.*?))?[ \t]*$`)
	tocTagsRe     = regexp.MustCompile(`^(.*?)(?:[ \t]+|^)(:(?:[\p{L}\p{N}_@#%]+:)+)$`)
	tocPrioRe     = regexp.MustCompile(`^\[#[^\]]\][ \t]*`)
	tocCommentRe  = regexp.MustCompile(`^COMMENT(?:[ \t]+|$)`)
	tocTodoLineRe = regexp.MustCompile(`(?i)^[ \t]*#\+(?:SEQ_|TYP_)?TODO:[ \t]*(.*)$`)
	tocPlanRe     = regexp.MustCompile(`^[ \t]*(?:SCHEDULED|DEADLINE|CLOSED):`)
	tocDrawerRe   = regexp.MustCompile(`^[ \t]*:([\w-]+):[ \t]*$`)
	tocEndRe      = regexp.MustCompile(`(?i)^[ \t]*:END:[ \t]*$`)
	tocPropRe     = regexp.MustCompile(`^[ \t]*:([^:\s]+):(?:[ \t]+(.*?))?[ \t]*$`)
	tocTagRe      = regexp.MustCompile(`(?i)^toc(?:[_@](\d+))?(?:[_@]([a-z]+))?$`)
	tocNoexpNRe   = regexp.MustCompile(`^noexport_(\d+)$`)
)

func tocIsHeading(line string) bool {
	rest := strings.TrimLeft(line, "*")
	return len(rest) < len(line) && rest != "" && (rest[0] == ' ' || rest[0] == '\t')
}

// The todo keywords a headline may start with: TODO and DONE, the file's own
// #+TODO: lines, and whatever the caller adds (the server's configured ones).
func tocKeywords(lines []string, extra []string) map[string]bool {
	kw := map[string]bool{"TODO": true, "DONE": true}
	for _, l := range lines {
		if m := tocTodoLineRe.FindStringSubmatch(l); m != nil {
			for _, w := range strings.Fields(m[1]) {
				if w == "|" {
					continue
				}
				if i := strings.Index(w, "("); i > 0 {
					w = w[:i]
				}
				kw[w] = true
			}
		}
	}
	for _, w := range extra {
		kw[w] = true
	}
	return kw
}

// tocHeads reads every heading of a file, in order.
func tocHeads(lines []string, extra []string) []tocHead {
	kw := tocKeywords(lines, extra)
	out := []tocHead{}
	stack := []int{}
	for row, l := range lines {
		if !tocIsHeading(l) {
			continue
		}
		m := tocHeadRe.FindStringSubmatch(l)
		h := tocHead{Row: row, Level: len(m[1]), Next: len(lines), Parent: -1}
		rest := m[2]
		if tm := tocTagsRe.FindStringSubmatch(rest); tm != nil {
			rest = tm[1]
			for _, t := range strings.Split(strings.Trim(tm[2], ":"), ":") {
				if t != "" {
					h.Tags = append(h.Tags, t)
				}
			}
		}
		if f := strings.Fields(rest); len(f) > 0 && kw[f[0]] {
			rest = strings.TrimLeft(rest[len(f[0]):], " \t")
		}
		rest = tocPrioRe.ReplaceAllString(rest, "")
		if tocCommentRe.MatchString(rest) {
			h.Comment = true
			rest = tocCommentRe.ReplaceAllString(rest, "")
		}
		h.Title = strings.TrimSpace(rest)
		h.Visible = common.TocVisibleTitle(h.Title)
		for len(stack) > 0 && out[stack[len(stack)-1]].Level >= h.Level {
			stack = stack[:len(stack)-1]
		}
		if len(stack) > 0 {
			h.Parent = stack[len(stack)-1]
		}
		if n := len(out); n > 0 {
			out[n-1].Next = row
		}
		stack = append(stack, len(out))
		out = append(out, h)
	}
	visibles := make([]string, len(out))
	for i := range out {
		visibles[i] = out[i].Visible
	}
	for i, a := range common.TocAnchors(visibles) {
		out[i].Anchor = a
	}
	for i := range out {
		tocReadHead(lines, &out[i])
	}
	return out
}

// tocReadHead finds where a heading's own text starts - after its planning
// line and the drawers directly under it, but not a :CONTENTS: drawer, which
// is org-make-toc's and is body - and reads CUSTOM_ID and TOC out of its
// property drawer.
func tocReadHead(lines []string, h *tocHead) {
	i := h.Row + 1
	for i < h.Next && tocPlanRe.MatchString(lines[i]) {
		i++
	}
	for i < h.Next {
		m := tocDrawerRe.FindStringSubmatch(lines[i])
		if m == nil || strings.EqualFold(m[1], "CONTENTS") || strings.EqualFold(m[1], "END") {
			break
		}
		end := -1
		for j := i + 1; j < h.Next; j++ {
			if tocEndRe.MatchString(lines[j]) {
				end = j
				break
			}
		}
		if end < 0 {
			break
		}
		if strings.EqualFold(m[1], "PROPERTIES") {
			for j := i + 1; j < end; j++ {
				pm := tocPropRe.FindStringSubmatch(lines[j])
				if pm == nil {
					continue
				}
				switch strings.ToUpper(pm[1]) {
				case "CUSTOM_ID":
					h.CustomID = pm[2]
				case "TOC":
					h.TocProp, h.HasTocProp = pm[2], true
				}
			}
		}
		i = end + 1
	}
	h.HeadEnd = i
}

func tocHasTag(h *tocHead, tag string) bool {
	for _, t := range h.Tags {
		if t == tag {
			return true
		}
	}
	return false
}

// The toc-org tag on a heading, if it has one: its depth (0 = not given) and
// link style.
func tocOrgTag(h *tocHead) (bool, int, string) {
	for _, t := range h.Tags {
		if m := tocTagRe.FindStringSubmatch(t); m != nil {
			d, _ := strconv.Atoi(m[1])
			return true, d, strings.ToLower(m[2])
		}
	}
	return false, 0, ""
}

// Out of every TOC: under a :noexport: or COMMENT heading (or the heading
// itself), as org's export leaves them out.
func tocHidden(hs []tocHead, i int) bool {
	for j := i; j >= 0; j = hs[j].Parent {
		if hs[j].Comment || tocHasTag(&hs[j], "noexport") {
			return true
		}
	}
	return false
}

func tocLink(h *tocHead, style string) string {
	if style == "org" {
		return "[[*" + h.Visible + "][" + h.Visible + "]]"
	}
	a := h.Anchor
	if h.CustomID != "" {
		a = h.CustomID
	}
	return "[[#" + a + "][" + h.Visible + "]]"
}

// toc-org's list: every heading down to depth (absolute levels), indented two
// spaces a level; not the TOC headings, nor under :noexport_N: deeper than N-1
// levels below it.
func tocOrgEntries(hs []tocHead, depth int, style string) []string {
	out := []string{}
	for i := range hs {
		h := &hs[i]
		if h.Level > depth || tocHidden(hs, i) {
			continue
		}
		if is, _, _ := tocOrgTag(h); is {
			continue
		}
		cut := false
		for j := h.Parent; j >= 0 && !cut; j = hs[j].Parent {
			for _, t := range hs[j].Tags {
				if m := tocNoexpNRe.FindStringSubmatch(t); m != nil {
					n, _ := strconv.Atoi(m[1])
					if h.Level-hs[j].Level >= n {
						cut = true
					}
				}
			}
		}
		if cut {
			continue
		}
		out = append(out, strings.Repeat("  ", h.Level-1)+"- "+tocLink(h, style))
	}
	return out
}

// What a TOC built org-make-toc's way lists.
type tocSpec struct {
	Include     string // all, descendants, siblings
	Depth       int
	HasDepth    bool
	ForceDepth  bool
	ForceIgnore bool
	Links       string // gh, org
	Self        int    // the heading the TOC is at, -1 above the first heading
	SkipSelf    bool   // a dynamic block does not list the heading it is under
}

// One heading's own :TOC: settings.
type tocProps struct {
	Ignore     map[string]bool
	Depth      int
	HasDepth   bool
	LocalDepth bool
}

func tocPropsOf(h *tocHead) tocProps {
	p := tocProps{Ignore: map[string]bool{}}
	if !h.HasTocProp {
		return p
	}
	v := strings.TrimSpace(h.TocProp)
	if v != "" && !strings.HasPrefix(v, ":") {
		return p // the old form, `all` / `children` / `siblings`: an include only
	}
	ps := parseDynParams(v)
	for _, w := range dynList(ps["ignore"]) {
		p.Ignore[strings.ToLower(w)] = true
	}
	if d, err := strconv.Atoi(strings.TrimSpace(ps["depth"])); err == nil && d >= 0 {
		p.Depth, p.HasDepth = d, true
	}
	for _, w := range dynList(ps["local"]) {
		if strings.EqualFold(w, "depth") {
			p.LocalDepth = true
		}
	}
	return p
}

// tocSpecOf reads the :TOC: property of the heading at self as the TOC it asks for.
func tocSpecOf(h *tocHead, self int) tocSpec {
	s := tocSpec{Include: "all", Links: "gh", Self: self}
	v := strings.TrimSpace(h.TocProp)
	if v != "" && !strings.HasPrefix(v, ":") {
		s.Include = strings.ToLower(strings.Fields(v)[0])
	} else {
		ps := parseDynParams(v)
		if inc := strings.ToLower(strings.TrimSpace(ps["include"])); inc != "" {
			s.Include = inc
		}
		for _, w := range dynList(ps["force"]) {
			switch strings.ToLower(w) {
			case "depth":
				s.ForceDepth = true
			case "ignore":
				s.ForceIgnore = true
			}
		}
		if strings.EqualFold(strings.TrimSpace(ps["links"]), "org") {
			s.Links = "org"
		}
	}
	if s.Include == "children" {
		s.Include = "descendants"
	}
	p := tocPropsOf(h)
	s.Depth, s.HasDepth = p.Depth, p.HasDepth
	return s
}

// tocSpecEntries is org-make-toc's list for a spec, indented two spaces a
// level from the shallowest heading listed.
func tocSpecEntries(hs []tocHead, s tocSpec) []string {
	props := make([]tocProps, len(hs))
	for i := range hs {
		props[i] = tocPropsOf(&hs[i])
	}
	include := s.Include
	if s.Self < 0 {
		include = "all"
	}
	rootLevel := 0
	switch include {
	case "descendants":
		rootLevel = hs[s.Self].Level
	case "siblings":
		rootLevel = hs[s.Self].Level - 1
	}
	// A setting is the TOC heading's own, or one it does not override.
	ignores := func(j int, what string) bool {
		return props[j].Ignore[what] && (j == s.Self || !s.ForceIgnore)
	}
	// Siblings of a heading with `:ignore siblings`.
	sibOut := make([]bool, len(hs))
	for j := range hs {
		if !ignores(j, "siblings") {
			continue
		}
		for k := range hs {
			if k != j && hs[k].Parent == hs[j].Parent && hs[k].Level == hs[j].Level {
				sibOut[k] = true
			}
		}
	}
	type entry struct{ level int; text string }
	picked := []entry{}
	minLevel := 0
	for i := range hs {
		h := &hs[i]
		inScope := false
		switch include {
		case "all":
			inScope = true
		case "descendants":
			for j := h.Parent; j >= 0; j = hs[j].Parent {
				if j == s.Self {
					inScope = true
					break
				}
			}
		case "siblings":
			for j := i; j >= 0; j = hs[j].Parent {
				if hs[j].Parent == hs[s.Self].Parent && hs[j].Level == hs[s.Self].Level {
					inScope = true
					break
				}
			}
		}
		if !inScope || tocHidden(hs, i) {
			continue
		}
		if s.HasDepth && h.Level-rootLevel > s.Depth {
			continue
		}
		if is, _, _ := tocOrgTag(h); is {
			continue
		}
		out := false
		for j := i; j >= 0 && hs[j].Level > rootLevel; j = hs[j].Parent {
			if sibOut[j] {
				out = true
				break
			}
			if j == i {
				continue
			}
			if ignores(j, "descendants") {
				out = true
				break
			}
			if j != s.Self && props[j].HasDepth && !props[j].LocalDepth && !s.ForceDepth && h.Level-hs[j].Level > props[j].Depth {
				out = true
				break
			}
		}
		if out || ignores(i, "this") || (s.SkipSelf && i == s.Self) {
			continue
		}
		if len(picked) == 0 || h.Level < minLevel {
			minLevel = h.Level
		}
		picked = append(picked, entry{h.Level, tocLink(h, s.Links)})
	}
	lines := make([]string, 0, len(picked))
	for _, e := range picked {
		lines = append(lines, strings.Repeat("  ", e.level-minLevel)+"- "+e.text)
	}
	return lines
}

// tocBlockSpec reads a `#+BEGIN: toc` block's parameters: :include (all),
// :depth (none), :links (org - a dynamic block is ours, and org links work in
// Emacs as written).
func tocBlockSpec(params map[string]string, self int) tocSpec {
	s := tocSpec{Include: "all", Links: "org", Self: self, SkipSelf: true}
	if v := strings.ToLower(strings.TrimSpace(params["include"])); v != "" {
		s.Include = v
	}
	if s.Include == "children" {
		s.Include = "descendants"
	}
	if d, err := strconv.Atoi(strings.TrimSpace(params["depth"])); err == nil && d >= 0 {
		s.Depth, s.HasDepth = d, true
	}
	if strings.EqualFold(strings.TrimSpace(params["links"]), "gh") {
		s.Links = "gh"
	}
	return s
}

// The heading a row is under: the last one starting above it, -1 for none.
func tocOwner(hs []tocHead, row int) int {
	owner := -1
	for i := range hs {
		if hs[i].Row >= row {
			break
		}
		owner = i
	}
	return owner
}

// tocBlockLines is the body of a `#+BEGIN: toc` block at row, unindented.
func tocBlockLines(lines []string, row int, params map[string]string, extra []string) []string {
	hs := tocHeads(lines, extra)
	return tocSpecEntries(hs, tocBlockSpec(params, tocOwner(hs, row)))
}

func tocTrailingBlanks(ls []string) int {
	n := 0
	for i := len(ls) - 1; i >= 0 && strings.TrimSpace(ls[i]) == ""; i-- {
		n++
	}
	return n
}

// TocSites finds every TOC in a file and works out what each should say,
// against the lines as they are. Sites never overlap (a block inside a
// toc-org heading's body is that body's to replace) and come in file order.
func TocSites(lines []string, extra []string) []TocSite {
	hs := tocHeads(lines, extra)
	sites := []TocSite{}
	tocOrgDone := false
	for i := range hs {
		h := &hs[i]
		if is, depth, style := tocOrgTag(h); is {
			if tocOrgDone {
				continue // toc-org keeps only the first one
			}
			tocOrgDone = true
			if depth == 0 {
				depth = 2
			}
			if style != "org" {
				style = "gh"
			}
			entries := tocOrgEntries(hs, depth, style)
			body := entries
			if tocHasTagFold(h, "QUOTE") {
				body = append(append([]string{"#+BEGIN_QUOTE"}, entries...), "#+END_QUOTE")
			}
			old := lines[h.HeadEnd:h.Next]
			blanks := tocTrailingBlanks(old)
			if blanks == 0 && h.Next < len(lines) {
				blanks = 1
			}
			for k := 0; k < blanks; k++ {
				body = append(body, "")
			}
			sites = append(sites, tocMakeSite("toc-org", h, h.Row, h.HeadEnd, h.Next, body, len(entries), lines))
			continue
		}
		if !h.HasTocProp {
			continue
		}
		entries := tocSpecEntries(hs, tocSpecOf(h, i))
		from, to, ind := -1, -1, ""
		for r := h.HeadEnd; r < h.Next; r++ {
			if m := tocDrawerRe.FindStringSubmatch(lines[r]); m != nil && strings.EqualFold(m[1], "CONTENTS") {
				for e := r + 1; e < h.Next; e++ {
					if tocEndRe.MatchString(lines[e]) {
						from, to = r+1, e
						ind = lines[r][:len(lines[r])-len(strings.TrimLeft(lines[r], " \t"))]
						break
					}
				}
				break
			}
		}
		body := []string{}
		for _, e := range entries {
			body = append(body, ind+e)
		}
		if from < 0 {
			// org-make-toc writes only into a :CONTENTS: drawer that is there;
			// a :TOC: property alone is settings for TOCs higher up.
			continue
		}
		sites = append(sites, tocMakeSite("org-make-toc", h, h.Row, from, to, body, len(entries), lines))
	}
	for _, b := range findDynBlocks(lines) {
		if b.Name != "toc" {
			continue
		}
		owner := tocOwner(hs, b.Begin)
		entries := tocSpecEntries(hs, tocBlockSpec(b.Params, owner))
		body := []string{}
		for _, e := range entries {
			body = append(body, b.Indent+e)
		}
		var oh *tocHead
		if owner >= 0 {
			oh = &hs[owner]
		}
		sites = append(sites, tocMakeSite("dblock", oh, b.Begin, b.Begin+1, b.End, body, len(entries), lines))
	}
	// File order, and nothing inside a site already kept.
	for i := 1; i < len(sites); i++ {
		for j := i; j > 0 && sites[j].From < sites[j-1].From; j-- {
			sites[j], sites[j-1] = sites[j-1], sites[j]
		}
	}
	kept := []TocSite{}
	for _, s := range sites {
		if n := len(kept); n > 0 && s.From < kept[n-1].To {
			continue
		}
		kept = append(kept, s)
	}
	return kept
}

func tocHasTagFold(h *tocHead, tag string) bool {
	for _, t := range h.Tags {
		if strings.EqualFold(t, tag) {
			return true
		}
	}
	return false
}

func tocMakeSite(kind string, h *tocHead, row, from, to int, body []string, n int, lines []string) TocSite {
	s := TocSite{Kind: kind, Row: row, From: from, To: to, Text: strings.Join(body, "\n"), Entries: n, Lines: body}
	if h != nil {
		s.Heading = h.Visible
	}
	s.Changed = (to-from) != len(body) || strings.Join(lines[from:to], "\n") != s.Text
	return s
}

// ApplyTocs is the file's lines with every TOC brought up to date, and
// whether anything changed. Spliced bottom up, so each site's rows still hold.
func ApplyTocs(lines []string, extra []string) ([]string, []TocSite, bool) {
	sites := TocSites(lines, extra)
	changed := false
	out := lines
	for i := len(sites) - 1; i >= 0; i-- {
		s := sites[i]
		if !s.Changed {
			continue
		}
		body := s.Lines
		next := make([]string, 0, len(out)-(s.To-s.From)+len(body))
		next = append(next, out[:s.From]...)
		next = append(next, body...)
		next = append(next, out[s.To:]...)
		out = next
		changed = true
	}
	return out, sites, changed
}

// TocInsert adds a table of contents and fills it. kind "toc-org" (the
// default) puts a `* Table of Contents :TOC:` heading after the preamble -
// above the first heading - unless the file has one already. kind
// "org-make-toc" gives the heading at row (a headline's row; -1 = a new
// `* Contents` heading after the preamble) the property `:TOC: :include all
// :ignore this` and an empty :CONTENTS: drawer. Answers the new lines and
// the row of the TOC's headline.
func TocInsert(lines []string, kind string, row int, extra []string) ([]string, int) {
	hs := tocHeads(lines, extra)
	first := len(lines)
	if len(hs) > 0 {
		first = hs[0].Row
	}
	if kind != "org-make-toc" {
		for i := range hs {
			if is, _, _ := tocOrgTag(&hs[i]); is {
				out, _, _ := ApplyTocs(lines, extra)
				return out, hs[i].Row
			}
		}
		out := splice(append([]string{}, lines...), first, []string{"* Table of Contents :TOC:"})
		out, _, _ = ApplyTocs(out, extra)
		return out, first
	}
	out := append([]string{}, lines...)
	at := -1
	for i := range hs {
		if hs[i].Row == row {
			at = i
		}
	}
	if at < 0 {
		out = splice(out, first, []string{"* Contents"})
		row = first
	}
	hs = tocHeads(out, extra)
	h := hs[tocOwner(hs, row+1)]
	if !h.HasTocProp {
		ind := ""
		inserted := false
		for r := h.Row + 1; r < h.HeadEnd; r++ {
			if m := tocDrawerRe.FindStringSubmatch(out[r]); m != nil && strings.EqualFold(m[1], "PROPERTIES") {
				ind = out[r][:len(out[r])-len(strings.TrimLeft(out[r], " \t"))]
				for e := r + 1; e < h.HeadEnd; e++ {
					if tocEndRe.MatchString(out[e]) {
						out = splice(out, e, []string{ind + ":TOC: :include all :ignore this"})
						inserted = true
						break
					}
				}
				break
			}
		}
		if !inserted {
			at := h.Row + 1
			for at < h.Next && tocPlanRe.MatchString(out[at]) {
				at++
			}
			out = splice(out, at, []string{":PROPERTIES:", ":TOC: :include all :ignore this", ":END:"})
		}
		hs = tocHeads(out, extra)
		h = hs[tocOwner(hs, row+1)]
	}
	has := false
	for r := h.HeadEnd; r < h.Next; r++ {
		if m := tocDrawerRe.FindStringSubmatch(out[r]); m != nil && strings.EqualFold(m[1], "CONTENTS") {
			has = true
			break
		}
	}
	if !has {
		ind := ""
		if h.HeadEnd > h.Row+1 {
			l := out[h.HeadEnd-1]
			ind = l[:len(l)-len(strings.TrimLeft(l, " \t"))]
		}
		out = splice(out, h.HeadEnd, []string{ind + ":CONTENTS:", ind + ":END:"})
	}
	out, _, _ = ApplyTocs(out, extra)
	return out, row
}
