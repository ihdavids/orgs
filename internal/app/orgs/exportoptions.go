package orgs

// #+OPTIONS: keys the writers do not read themselves, done once in the copy
// every export reads (exportFileFor, subtreeViewFor), so html, LaTeX, Typst,
// Beamer, markdown and the rest agree (org manual 13.2):
//
//	H:N       headings deeper than N become list items, their text under them
//	p:nil     planning lines (SCHEDULED, DEADLINE, CLOSED) left out
//	d:nil     drawers left out; d:("LOGBOOK" "NOTES") only those kept;
//	          d:(not "LOGBOOK") those left out
//	c:nil     CLOCK: lines left out
//	prop:nil  property drawers left out
//	tasks:nil headings with a keyword left out (with what is under them);
//	          tasks:todo the finished ones; tasks:("NEXT" "TODO") all but those
//	stat:nil  statistics cookies ([2/5], [40%]) taken off
//	\n:t      every line break kept
//	::nil     fixed-width lines (": text") left out
//	|:nil     tables left out
//	*:nil     *bold*, /italic/, _underline_ and +strike+ printed as written
//	          (a zero-width space before the opening mark, so it is not one)
//	timestamp:t  the time of the export at the end ("Created: ..."), as org's
//	          postamble writes it, for html and LaTeX
//	broken-links:mark  a link to nothing in the database becomes
//	          "[BROKEN LINK: target]"; broken-links:t keeps its words only
//
// Only what a file's #+OPTIONS: line says is done: a key it does not mention
// leaves the writers as they were. (Org's own defaults are p:nil, c:nil,
// prop:nil and d:(not "LOGBOOK"); a file wanting exactly org's export writes
// them. Applying them to every file would take the planning badges and the
// logbook out of every rendered page worg shows.) toc, num, todo, pri, tags,
// e, f, <, ^, arch, author and date are read by the writers.

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ihdavids/orgs/internal/common"
)

var optionsLineRe = regexp.MustCompile(`(?i)^\s*#\+OPTIONS:\s*(.*)$`)

// exportOptions are a file's #+OPTIONS: keys, as written: d:(not "LOGBOOK")
// is one word, ::nil is the key ":".
func exportOptions(lines []string) map[string]string {
	out := map[string]string{}
	for _, r := range citeLines(lines) {
		m := optionsLineRe.FindStringSubmatch(lines[r])
		if m == nil {
			continue
		}
		toks, cur, depth := []string{}, strings.Builder{}, 0
		for _, c := range m[1] {
			switch {
			case c == '(':
				depth++
			case c == ')':
				depth--
			case (c == ' ' || c == '\t') && depth <= 0:
				if cur.Len() > 0 {
					toks = append(toks, cur.String())
					cur.Reset()
				}
				continue
			}
			cur.WriteRune(c)
		}
		if cur.Len() > 0 {
			toks = append(toks, cur.String())
		}
		for _, t := range toks {
			if strings.HasPrefix(t, "::") {
				out[":"] = t[2:]
				continue
			}
			if k, v, ok := strings.Cut(t, ":"); ok && k != "" {
				out[k] = v
			}
		}
	}
	return out
}

// optionsMayApply is whether text has an #+OPTIONS: line naming a key this
// pass does, read cheaply.
func optionsMayApply(text string) bool {
	low := strings.ToLower(text)
	if !strings.Contains(low, "#+options:") {
		return false
	}
	for _, k := range []string{"h:", "p:", "d:", "c:", "prop:", "tasks:", "stat:", `\n:`, "::", "|:", "*:", "timestamp:", "broken-links:"} {
		if strings.Contains(low, k) {
			return true
		}
	}
	return false
}

// quotedList reads ("A" "B") or (not "A" "B") into its words and whether it
// is a not.
func quotedList(v string) (words []string, not bool) {
	v = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(v), "("), ")"))
	if strings.HasPrefix(v, "not ") {
		not, v = true, v[4:]
	}
	for _, w := range strings.Fields(v) {
		if w = strings.Trim(w, `"`); w != "" {
			words = append(words, w)
		}
	}
	return words, not
}

var (
	optDrawerRe   = regexp.MustCompile(`^\s*:([\w-]+):\s*$`)
	optFixedRe    = regexp.MustCompile(`^\s*:(\s|$)`)
	optTableRe    = regexp.MustCompile(`^\s*(\||\+-)`)
	optTblfmRe    = regexp.MustCompile(`(?i)^\s*#\+tblfm:`)
	optCookieRe   = regexp.MustCompile(`\s*\[(\d*%|\d*/\d*)\]`)
	optItemRe     = regexp.MustCompile(`^\s*([-+]|\d+[.)])\s`)
	optKeywordRe  = regexp.MustCompile(`^\s*#\+`)
	optHeadlineRe = regexp.MustCompile(`^(\*+)\s+(.*)$`)
)

// exportOptionLines is lines with the file's options applied. changed is
// false when none applied.
func exportOptionLines(f *common.OrgFile, lines []string) ([]string, bool) {
	opts := exportOptions(lines)
	get := func(k string) (string, bool) { v, ok := opts[k]; return v, ok }
	out := append([]string{}, lines...)
	changed := false
	drop := func(i int) {
		out[i] = "\x00"
		changed = true
	}

	// tasks: whole subtrees first, so nothing below touches what is gone.
	if v, ok := get("tasks"); ok && v != "t" {
		done, active := keywordSets(f)
		keepKw := map[string]bool{}
		mode := v
		if strings.HasPrefix(v, "(") {
			kws, _ := quotedList(v)
			for _, k := range kws {
				keepKw[k] = true
			}
			mode = "list"
		}
		s := newScLines(out)
		for i := 0; i < len(out); i++ {
			lvl := s.level(i)
			if lvl == 0 {
				continue
			}
			words := strings.Fields(optHeadlineRe.FindStringSubmatch(out[i])[2])
			kw := ""
			if len(words) > 0 && (done[words[0]] || active[words[0]]) {
				kw = words[0]
			}
			gone := false
			switch mode {
			case "nil":
				gone = kw != ""
			case "todo":
				gone = done[kw]
			case "done":
				gone = active[kw]
			case "list":
				gone = kw != "" && !keepKw[kw]
			}
			if gone {
				end := s.subtreeEnd(i)
				for j := i; j < end; j++ {
					drop(j)
				}
				i = end - 1
			}
		}
	}

	pOff := opts["p"] == "nil"
	cOff := opts["c"] == "nil"
	propOff := opts["prop"] == "nil"
	fixedOff := opts[":"] == "nil"
	tableOff := opts["|"] == "nil"
	statOff := opts["stat"] == "nil"
	breaks := opts[`\n`] == "t"
	dv, dSet := get("d")
	dKeep := func(name string) bool {
		if !dSet || dv == "t" {
			return true
		}
		if dv == "nil" {
			return false
		}
		words, not := quotedList(dv)
		in := false
		for _, w := range words {
			if strings.EqualFold(w, name) {
				in = true
			}
		}
		return in != not
	}

	inBlock := false
	for i := 0; i < len(out); i++ {
		l := out[i]
		if l == "\x00" {
			continue
		}
		if inBlock {
			inBlock = !txBlockEndRe.MatchString(l)
			continue
		}
		if txBlockBeginRe.MatchString(l) {
			inBlock = true
			continue
		}
		if m := optDrawerRe.FindStringSubmatch(l); m != nil && !strings.EqualFold(m[1], "end") {
			e := txEndOf(out, i, txDrawerEndRe.MatchString)
			if e >= 0 {
				isProp := strings.EqualFold(m[1], "properties")
				if (isProp && propOff) || (!isProp && !dKeep(m[1])) {
					for j := i; j <= e; j++ {
						drop(j)
					}
				} else if cOff {
					for j := i + 1; j < e; j++ {
						if txClockRe.MatchString(out[j]) {
							drop(j)
						}
					}
				}
				i = e
				continue
			}
		}
		switch {
		case pOff && txPlanningRe.MatchString(l):
			drop(i)
		case cOff && txClockRe.MatchString(l):
			drop(i)
		case fixedOff && optFixedRe.MatchString(l):
			drop(i)
		case tableOff && (optTableRe.MatchString(l) || optTblfmRe.MatchString(l)):
			drop(i)
		case statOff && (optHeadlineRe.MatchString(l) || optItemRe.MatchString(l)) && optCookieRe.MatchString(l):
			out[i] = optCookieRe.ReplaceAllString(l, "")
			changed = true
		case breaks && strings.TrimSpace(l) != "" && !optHeadlineRe.MatchString(l) && !optKeywordRe.MatchString(l) &&
			!optTableRe.MatchString(l) && !optFixedRe.MatchString(l) && i+1 < len(out) && strings.TrimSpace(out[i+1]) != "" &&
			!strings.HasSuffix(strings.TrimSpace(l), `\\`):
			out[i] = l + ` \\`
			changed = true
		}
	}

	// H: deeper headings become items, with their text indented under them.
	if v, ok := get("H"); ok {
		if h, err := strconv.Atoi(v); err == nil && h > 0 {
			depth := 0
			inBlock = false
			for i, l := range out {
				if l == "\x00" {
					continue
				}
				if m := optHeadlineRe.FindStringSubmatch(l); m != nil && !inBlock {
					if lvl := len(m[1]); lvl > h {
						depth = lvl - h
						out[i] = strings.Repeat("  ", depth-1) + "- " + m[2]
						changed = true
					} else {
						depth = 0
					}
					continue
				}
				if inBlock {
					inBlock = !txBlockEndRe.MatchString(l)
				} else {
					inBlock = txBlockBeginRe.MatchString(l)
				}
				if depth > 0 && strings.TrimSpace(l) != "" {
					out[i] = strings.Repeat("  ", depth) + l
					changed = true
				}
			}
		}
	}

	if opts["*"] == "nil" {
		inBlock = false
		for i, l := range out {
			if l == "\x00" {
				continue
			}
			if inBlock {
				inBlock = !txBlockEndRe.MatchString(l)
				continue
			}
			if txBlockBeginRe.MatchString(l) {
				inBlock = true
				continue
			}
			if optKeywordRe.MatchString(l) {
				continue
			}
			if nl := noEmphasis(l); nl != l {
				out[i], changed = nl, true
			}
		}
	}
	if v := opts["broken-links"]; v == "mark" || v == "t" {
		if markBrokenLinks(f, out, v == "mark") {
			changed = true
		}
	}
	if opts["timestamp"] == "t" {
		stamp := time.Now().Format("2006-01-02 Mon 15:04")
		out = append(out, "", "#+BEGIN_EXPORT html",
			`<div id="postamble" class="status"><p class="date">Created: `+stamp+`</p></div>`,
			"#+END_EXPORT", "#+BEGIN_EXPORT latex", "% Created "+stamp, "#+END_EXPORT")
		changed = true
	}
	if !changed {
		return lines, false
	}
	kept := out[:0]
	for _, l := range out {
		if l != "\x00" {
			kept = append(kept, l)
		}
	}
	return kept, true
}

var (
	emphOpenRe = regexp.MustCompile(`(^|[\s\-({'"])([*/_+])([^\s*/_+])`)
	optHeadStarsRe = regexp.MustCompile(`^\*+\s`)
	optLinkRe  = regexp.MustCompile(`\[\[([^\]]+)\](?:\[([^\]]*)\])?\]`)
)

// noEmphasis puts a zero-width space before every mark that would open
// emphasis, so go-org (whose opening mark needs a space or -({'" before it)
// reads it as text. A heading's own stars are left alone.
func noEmphasis(l string) string {
	head := ""
	if m := optHeadStarsRe.FindString(l); m != "" {
		head, l = m, l[len(m):]
	}
	return head + emphOpenRe.ReplaceAllString(l, "${1}\u200b${2}${3}")
}

// markBrokenLinks rewrites the links of f that point at nothing (the link
// index's Broken, less fuzzy targets the copy's own <<target>> or #+NAME:
// answer): as "[BROKEN LINK: target]" with mark, else as their words. It
// reports whether it changed anything.
func markBrokenLinks(f *common.OrgFile, out []string, mark bool) bool {
	if f == nil {
		return false
	}
	idx := getLinkIndex()
	broken := map[string]bool{}
	for _, i := range idx.fromFile[f.Filename] {
		if l := idx.links[i]; l.Broken {
			broken[l.Raw] = true
		}
	}
	if len(broken) == 0 {
		return false
	}
	text := strings.Join(out, "\n")
	changed := false
	for i, l := range out {
		if l == "\x00" || !strings.Contains(l, "[[") {
			continue
		}
		nl := optLinkRe.ReplaceAllStringFunc(l, func(s string) string {
			m := optLinkRe.FindStringSubmatch(s)
			target := m[1]
			if !broken[target] || strings.Contains(text, "<<"+target+">>") || regexp.MustCompile(`(?im)^\s*#\+NAME:\s*`+regexp.QuoteMeta(target)+`\s*$`).MatchString(text) {
				return s
			}
			if mark {
				return "[BROKEN LINK: " + target + "]"
			}
			if m[2] != "" {
				return m[2]
			}
			return target
		})
		if nl != l {
			out[i], changed = nl, true
		}
	}
	return changed
}
