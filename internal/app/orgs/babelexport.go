package orgs

// What a source block inherits that an exporter needs: :exports. The html,
// latex, markdown and typst writers read a block's own #+BEGIN_SRC line and
// nothing else, so `#+PROPERTY: header-args :exports none` (or the same on a
// heading, or babel.defaultHeaderArgs in the yaml) never reached them.
// Rather than teach four writers inheritance, the copy every export reads
// (exportFileWith) has each block's inherited :exports written onto its own
// line - in memory, never in the file, like transclusion.
//
// When nothing says, the writers keep what they have always done: the code,
// and its results when there are any. Org's own default is :exports code;
// `babel: defaultHeaderArgs: ":exports code"` gives that.

import (
	"regexp"
	"sort"
	"strings"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

var (
	exSrcBeginRe   = regexp.MustCompile(`(?i)^(\s*#\+begin_src)\s+(\S+)(.*)$`)
	exportsWordRe  = regexp.MustCompile(`(?i)(^|\s):exports(\s|$)`)
	headerLineRe   = regexp.MustCompile(`(?i)^\s*#\+headers?:(.*)$`)
	exAffiliatedRe = regexp.MustCompile(`(?i)^\s*#\+(name|caption|attr_\S+|headers?|results|plot):`)
)

// babelExportsMayInherit is whether text could have a block, a #+CALL: or
// inline code whose :exports comes from somewhere other than its own line
// (or, for inline code, says :exports at all - the writers ignore it there),
// read cheaply.
func babelExportsMayInherit(text string) bool {
	low := strings.ToLower(text)
	code := strings.Contains(low, "#+begin_src") || strings.Contains(low, "#+call:") || strings.Contains(low, "src_") || strings.Contains(low, "call_")
	if !code {
		return false
	}
	if s := Conf().Server; s != nil && (s.Babel.DefaultHeaderArgs != "" || len(s.Babel.LanguageHeaderArgs) > 0) {
		return true
	}
	return strings.Contains(low, "header-args") || strings.Contains(low, "#+setupfile") ||
		(strings.Contains(low, ":exports") && (strings.Contains(low, "src_") || strings.Contains(low, "call_")))
}

// babelExportLines is lines with each source block that says nothing about
// :exports itself (on its line or its #+HEADER: lines) given the :exports it
// inherits. changed is false when there was nothing to write.
func babelExportLines(path string, lines []string) (out []string, changed bool) {
	text := strings.Join(lines, "\n")
	doc := GetConfig().Silent().Parse(strings.NewReader(text+"\n"), path)
	f := &common.OrgFile{Filename: path, Doc: doc}
	out = append([]string{}, lines...)
	inBlock := false
	for i, l := range lines {
		if inBlock {
			inBlock = !txBlockEndRe.MatchString(l)
			continue
		}
		m := exSrcBeginRe.FindStringSubmatch(l)
		if m == nil {
			inBlock = txBlockBeginRe.MatchString(l)
			if inBlock {
				continue
			}
			if c := callLineRe.FindStringSubmatch(l); c != nil {
				// A #+CALL: is exported by its own headers (the writers read
				// them), so what it inherits - the call's file and the called
				// block's arguments, as org-babel-lob-get-info merges them - is
				// written onto it.
				if !exportsWordRe.MatchString(c[3]) && !exportsWordRe.MatchString(c[5]) {
					if job, _, err := callJob(c[2], c[3], c[4], c[5], f, path); err == nil {
						if v, ok := job.arg("exports"); ok && v != "" {
							out[i] = strings.TrimRight(l, " \t") + " :exports " + v
							changed = true
						}
					}
				}
				continue
			}
			if nl, ok := inlineExports(f, path, text, i, l); ok {
				out[i], changed = nl, true
			}
			continue
		}
		inBlock = true
		if exportsWordRe.MatchString(m[3]) || babelHeadersSay(lines, i, exportsWordRe) {
			continue
		}
		chain := []*org.Section{}
		for s := sectionAtRow(f, i); s != nil; s = s.Parent {
			chain = append([]*org.Section{s}, chain...)
		}
		val := ""
		for _, a := range inheritedArgsIn(path, text, m[2], chain) {
			if strings.EqualFold(a.Key, "exports") {
				val = strings.Trim(strings.TrimSpace(a.Value), `"`)
			}
		}
		if val == "" {
			continue
		}
		out[i] = strings.TrimRight(l, " \t") + " :exports " + val
		changed = true
	}
	return out, changed
}

// babelHeadersSay is whether a #+HEADER: line above the block on row matches
// re, looking up over the affiliated keywords it sits among.
func babelHeadersSay(lines []string, row int, re *regexp.Regexp) bool {
	for r := row - 1; r >= 0 && exAffiliatedRe.MatchString(lines[r]); r-- {
		if m := headerLineRe.FindStringSubmatch(lines[r]); m != nil && re.MatchString(m[1]) {
			return true
		}
	}
	return false
}

// inlineExports is a line with its inline code cut to what :exports says:
// results (org's default for inline code) keeps only the {{{results(...)}}},
// code only the code, none neither, both both. An inline call has no code
// of its own on the page, so code and none take its result away. Only when
// something says :exports - the snippet's own header, or what it inherits -
// is anything cut; the writers otherwise show the code and its result.
func inlineExports(f *common.OrgFile, path, text string, row int, line string) (string, bool) {
	codes := inlineCodeIn(line)
	if len(codes) == 0 {
		return line, false
	}
	sort.Slice(codes, func(a, b int) bool { return codes[a].start > codes[b].start })
	var chain []*org.Section
	chainDone := false
	changed := false
	for _, c := range codes {
		val := ""
		if exportsWordRe.MatchString(c.head) || (c.call && exportsWordRe.MatchString(c.tail)) {
			_, _, args, _ := splitArgs(splitParametersLike(" " + c.head + " " + c.tail))
			for _, a := range args {
				if strings.EqualFold(a.Key, "exports") {
					val = strings.Trim(strings.TrimSpace(a.Value), `"`)
				}
			}
		} else if c.call {
			if job, _, err := callJob(c.lang, "["+c.head+"]", c.body, c.tail, f, path); err == nil {
				val, _ = job.arg("exports")
			}
		} else {
			if !chainDone {
				for s := sectionAtRow(f, row); s != nil; s = s.Parent {
					chain = append([]*org.Section{s}, chain...)
				}
				chainDone = true
			}
			for _, a := range inheritedArgsIn(path, text, c.lang, chain) {
				if strings.EqualFold(a.Key, "exports") {
					val = strings.Trim(strings.TrimSpace(a.Value), `"`)
				}
			}
		}
		from, to := -1, -1
		switch {
		case val == "none":
			from, to = c.start, c.resEnd
		case val == "results" && !c.call:
			from, to = c.start, c.end
			// And the space between the code and its result.
			if to < len(line) && line[to] == ' ' && c.resEnd > c.end {
				to++
			}
		case val == "code" && c.resEnd > c.end:
			from, to = c.end, c.resEnd
		}
		if from >= 0 && to > from {
			line = line[:from] + line[to:]
			changed = true
		}
	}
	return line, changed
}
