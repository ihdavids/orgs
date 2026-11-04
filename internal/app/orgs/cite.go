package orgs

// Citations: org-cite's [cite:@key] objects, #+bibliography:, #+cite_export:
// and #+print_bibliography:, expanded in the copy every export reads
// (exportFileFor), never in the file - as transclusion and inherited
// :exports are. go-org has no citation node, so the copy is where they can be
// turned into what each exporter already draws.
//
// Which processor, as org-cite chooses (org-cite-export-processors): the
// file's #+cite_export: line, else citations.export in the yaml, else basic.
//
//	basic [bib-style] [cite-style]  every exporter: "(Doe and Roe, 2020)" as
//	                                text, linked to its entry when the file
//	                                prints a bibliography; cite-style numeric
//	                                writes [1] and numbers the list
//	biblatex [style]                LaTeX and Beamer: \autocite and kin,
//	                                \printbibliography, the preamble lines
//	natbib [bibstyle]               LaTeX and Beamer: \citep and kin
//
// biblatex and natbib fall back to basic for any other exporter, because html
// cannot run biber. Bibliographies are .bib (BibTeX/BibLaTeX) or .json
// (CSL-JSON) files: the file's #+bibliography: lines (relative to it) and
// citations.bibliography in the yaml. A key no bibliography has is written as
// the key and said on stderr.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ihdavids/orgs/internal/common/biblio"
)

var (
	citeRe       = regexp.MustCompile(`\[cite(/[^:\]\s]*)?:([^\]]*)\]`)
	bibKeywordRe = regexp.MustCompile(`(?i)^\s*#\+bibliography:\s*(.+?)\s*$`)
	citeExportRe = regexp.MustCompile(`(?i)^\s*#\+cite_export:\s*(.*?)\s*$`)
	printBibRe   = regexp.MustCompile(`(?i)^(\s*)#\+print_bibliography:(.*)$`)
	citeTargetRe = regexp.MustCompile(`[^A-Za-z0-9_.:-]`)
)

// citeMayApply is whether text could have anything for this pass, read
// cheaply.
func citeMayApply(text string) bool {
	low := strings.ToLower(text)
	return strings.Contains(low, "[cite") || strings.Contains(low, "#+print_bibliography")
}

var bibCache = struct {
	sync.Mutex
	m map[string]bibCached
}{m: map[string]bibCached{}}

type bibCached struct {
	mod     time.Time
	size    int64
	entries []*biblio.Entry
	err     error
}

// readBib is a bibliography file's entries, read again only when the file
// changes.
func readBib(path string) ([]*biblio.Entry, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	bibCache.Lock()
	defer bibCache.Unlock()
	if c, ok := bibCache.m[path]; ok && c.mod.Equal(st.ModTime()) && c.size == st.Size() {
		return c.entries, c.err
	}
	data, err := os.ReadFile(path)
	var es []*biblio.Entry
	if err == nil {
		es, err = biblio.Parse(path, data)
	}
	bibCache.m[path] = bibCached{st.ModTime(), st.Size(), es, err}
	return es, err
}

func citeExpandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

// citeSources are the bibliography files a document cites from: its own
// #+bibliography: lines, then the yaml's.
func citeSources(path string, lines []string) []string {
	out := []string{}
	seen := map[string]bool{}
	add := func(p string) {
		p = citeExpandHome(strings.Trim(strings.TrimSpace(p), `"`))
		if p == "" {
			return
		}
		if !filepath.IsAbs(p) {
			p = filepath.Join(filepath.Dir(path), p)
		}
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	for _, l := range citeLines(lines) {
		if m := bibKeywordRe.FindStringSubmatch(lines[l]); m != nil {
			add(m[1])
		}
	}
	if s := Conf().Server; s != nil {
		for _, p := range s.Citations.Bibliography {
			add(p)
		}
	}
	return out
}

// CiteEntries is every entry a document can cite, by key (first source
// wins), and the files read.
func CiteEntries(path string, lines []string) (map[string]*biblio.Entry, []string) {
	srcs := citeSources(path, lines)
	m := map[string]*biblio.Entry{}
	for _, s := range srcs {
		es, err := readBib(s)
		if err != nil {
			fmt.Fprintf(os.Stderr, "cite: %s: bibliography %s: %v\n", filepath.Base(path), s, err)
			continue
		}
		for _, e := range es {
			if _, ok := m[e.Key]; !ok {
				m[e.Key] = e
			}
		}
	}
	return m, srcs
}

// citeLines are the rows outside blocks, where org reads objects and
// keywords.
func citeLines(lines []string) []int {
	out := []int{}
	inBlock := false
	for i, l := range lines {
		if inBlock {
			inBlock = !txBlockEndRe.MatchString(l)
			continue
		}
		if txBlockBeginRe.MatchString(l) {
			inBlock = true
			continue
		}
		out = append(out, i)
	}
	return out
}

func latexFamily(backend string) bool {
	switch strings.ToLower(backend) {
	case "latex", "pdf", "beamer", "dndbook":
		return true
	}
	return false
}

func citeTarget(key string) string { return "cite-" + citeTargetRe.ReplaceAllString(key, "_") }

// citeExportLines is lines with every citation and #+print_bibliography:
// written as the processor in force writes them for backend (an exporter's
// name; "" for any). changed is false when there was nothing to do.
func citeExportLines(path string, lines []string, backend string) ([]string, bool) {
	rows := citeLines(lines)
	spec := ""
	if s := Conf().Server; s != nil {
		spec = s.Citations.Export
	}
	for _, r := range rows {
		if m := citeExportRe.FindStringSubmatch(lines[r]); m != nil {
			spec = m[1]
		}
	}
	words := strings.Fields(strings.ToLower(spec))
	proc, bibStyle, citeStyle := "basic", "", ""
	if len(words) > 0 {
		proc = words[0]
	}
	if len(words) > 1 {
		bibStyle = words[1]
	}
	if len(words) > 2 {
		citeStyle = words[2]
	}
	if (proc == "biblatex" || proc == "natbib") && !latexFamily(backend) {
		proc = "basic"
		bibStyle, citeStyle = "", ""
	}
	if proc != "biblatex" && proc != "natbib" {
		proc = "basic"
	}
	numeric := proc == "basic" && (citeStyle == "numeric" || bibStyle == "numeric")

	entries, srcs := CiteEntries(path, lines)

	// Pass one: what is cited, in order, for numbering and the list.
	order := []string{}
	cited := map[string]bool{}
	all, printed := false, false
	for _, r := range rows {
		if printBibRe.MatchString(lines[r]) {
			printed = true
		}
		for _, m := range citeRe.FindAllStringSubmatch(lines[r], -1) {
			for _, ref := range biblio.ParseCitation(m[1], m[2]).Refs {
				if ref.Key == "*" {
					all = true
					continue
				}
				if !cited[ref.Key] {
					cited[ref.Key] = true
					order = append(order, ref.Key)
				}
			}
		}
	}
	if len(order) == 0 && !all && !printed {
		return lines, false
	}
	listed := []*biblio.Entry{}
	if all {
		for _, e := range entries {
			listed = append(listed, e)
		}
		biblio.Sort(listed)
		if numeric {
			// Cited ones first, in order, then the rest.
			first := []*biblio.Entry{}
			for _, k := range order {
				if e := entries[k]; e != nil {
					first = append(first, e)
				}
			}
			for _, e := range listed {
				if !cited[e.Key] {
					first = append(first, e)
				}
			}
			listed = first
		}
	} else {
		for _, k := range order {
			if e := entries[k]; e != nil {
				listed = append(listed, e)
			}
		}
		if !numeric {
			biblio.Sort(listed)
		}
	}
	numbers := map[string]int(nil)
	if numeric {
		numbers = map[string]int{}
		for i, e := range listed {
			numbers[e.Key] = i + 1
		}
	}

	missing := map[string]bool{}
	var link func(key, text string) string
	if printed {
		link = func(key, text string) string { return "[[" + citeTarget(key) + "][" + text + "]]" }
	}

	// Pass two: write them.
	out := append([]string{}, lines...)
	for _, r := range rows {
		l := out[r]
		if m := printBibRe.FindStringSubmatch(l); m != nil {
			out[r] = strings.Join(bibliographyLines(proc, bibStyle, m[1], listed, numeric, srcs), "\n")
			continue
		}
		out[r] = citeRe.ReplaceAllStringFunc(l, func(s string) string {
			m := citeRe.FindStringSubmatch(s)
			c := biblio.ParseCitation(m[1], m[2])
			switch proc {
			case "biblatex":
				return "@@latex:" + biblatexCite(c) + "@@"
			case "natbib":
				return "@@latex:" + natbibCite(c) + "@@"
			}
			return biblio.Format(c, entries, numbers, func(k string) { missing[k] = true }, link)
		})
	}
	// The preamble, for the LaTeX processors.
	head := []string{}
	switch proc {
	case "biblatex":
		opts := "backend=biber"
		if bibStyle != "" {
			opts += ",style=" + bibStyle
		}
		head = append(head, `#+LATEX_HEADER: \usepackage[`+opts+`]{biblatex}`)
		for _, s := range srcs {
			if strings.HasSuffix(strings.ToLower(s), ".bib") {
				head = append(head, `#+LATEX_HEADER: \addbibresource{`+s+`}`)
			}
		}
	case "natbib":
		head = append(head, `#+LATEX_HEADER: \usepackage[round]{natbib}`)
	}
	if len(missing) > 0 {
		keys := []string{}
		for k := range missing {
			keys = append(keys, "@"+k)
		}
		sort.Strings(keys)
		fmt.Fprintf(os.Stderr, "cite: %s: no bibliography entry for %s\n", filepath.Base(path), strings.Join(keys, ", "))
	}
	return append(head, out...), true
}

// bibliographyLines are what #+print_bibliography: becomes.
func bibliographyLines(proc, bibStyle, indent string, listed []*biblio.Entry, numeric bool, srcs []string) []string {
	switch proc {
	case "biblatex":
		return []string{"#+BEGIN_EXPORT latex", `\printbibliography`, "#+END_EXPORT"}
	case "natbib":
		style := bibStyle
		if style == "" {
			style = "plainnat"
		}
		bibs := []string{}
		for _, s := range srcs {
			if strings.HasSuffix(strings.ToLower(s), ".bib") {
				bibs = append(bibs, strings.TrimSuffix(s, filepath.Ext(s)))
			}
		}
		return []string{"#+BEGIN_EXPORT latex", `\bibliographystyle{` + style + `}`, `\bibliography{` + strings.Join(bibs, ",") + `}`, "#+END_EXPORT"}
	}
	out := []string{}
	for i, e := range listed {
		bullet := "-"
		if numeric {
			bullet = fmt.Sprintf("%d.", i+1)
		}
		out = append(out, fmt.Sprintf("%s%s <<%s>>%s", indent, bullet, citeTarget(e.Key), e.Reference()))
	}
	return out
}

var latexSpecials = strings.NewReplacer(`\`, `\textbackslash{}`, "{", `\{`, "}", `\}`, "&", `\&`, "%", `\%`, "$", `\$`, "#", `\#`, "_", `\_`, "~", `\textasciitilde{}`, "^", `\textasciicircum{}`)

func latexEsc(s string) string { return latexSpecials.Replace(s) }

// citeKeys are the citation's keys, comma separated; * stays *.
func citeKeys(c biblio.Citation) string {
	ks := []string{}
	for _, r := range c.Refs {
		ks = append(ks, r.Key)
	}
	return strings.Join(ks, ",")
}

// citeNotes are a citation's [pre][post] for a command that takes one pair:
// the common prefix and suffix, or the only work's own.
func citeNotes(c biblio.Citation) string {
	pre, post := c.Prefix, c.Suffix
	if len(c.Refs) == 1 {
		pre = strings.TrimSpace(pre + " " + c.Refs[0].Prefix)
		post = strings.TrimSpace(strings.TrimPrefix(c.Refs[0].Suffix, ",") + " " + post)
	}
	switch {
	case pre != "":
		return "[" + latexEsc(pre) + "][" + latexEsc(post) + "]"
	case post != "":
		return "[" + latexEsc(post) + "]"
	}
	return ""
}

func capsCmd(cmd string, caps bool) string {
	if caps {
		return strings.ToUpper(cmd[:1]) + cmd[1:]
	}
	return cmd
}

// biblatexCite is org-cite biblatex's command for a citation.
func biblatexCite(c biblio.Citation) string {
	cmd := "autocite"
	switch c.Style {
	case "text":
		cmd = capsCmd("textcite", c.Caps)
	case "author":
		cmd = capsCmd("citeauthor", c.Caps)
	case "noauthor":
		cmd = "autocite*"
	case "nocite":
		return `\nocite{` + citeKeys(c) + `}`
	case "numeric":
		cmd = "cite"
	case "note":
		cmd = "footcite"
	default:
		if c.Bare {
			cmd = "cite"
		}
	}
	// Each work with its own notes: the plural form, \autocites[..][..]{a}[..][..]{b}.
	own := false
	for _, r := range c.Refs {
		if r.Prefix != "" || r.Suffix != "" {
			own = true
		}
	}
	if own && len(c.Refs) > 1 && !strings.HasSuffix(cmd, "*") {
		var b strings.Builder
		b.WriteString(`\` + cmd + "s")
		if c.Prefix != "" || c.Suffix != "" {
			b.WriteString("(" + latexEsc(c.Prefix) + ")(" + latexEsc(c.Suffix) + ")")
		}
		for _, r := range c.Refs {
			one := biblio.Citation{Refs: []biblio.Reference{r}}
			b.WriteString(citeNotes(one) + "{" + r.Key + "}")
		}
		return b.String()
	}
	return `\` + cmd + citeNotes(c) + "{" + citeKeys(c) + "}"
}

// natbibCite is org-cite natbib's command for a citation.
func natbibCite(c biblio.Citation) string {
	cmd := "citep"
	switch c.Style {
	case "text":
		cmd = "citet"
	case "author":
		cmd = "citeauthor"
	case "noauthor":
		cmd = "citeyearpar"
	case "nocite":
		return `\nocite{` + citeKeys(c) + `}`
	default:
		if c.Bare {
			cmd = "citealp"
		}
	}
	cmd = capsCmd(cmd, c.Caps)
	if c.Full {
		cmd += "*"
	}
	return `\` + cmd + citeNotes(c) + "{" + citeKeys(c) + "}"
}

/* SDOC: API
* GET /cite/keys — The Works A File Can Cite

	=?file=paper.org=. Every entry of the file's bibliographies (its
	=#+bibliography:= lines and =citations.bibliography= in the yaml), by
	key, for completing =[cite:@...]=.

	*Response:* ={Ok, Msg, Sources: [paths], Keys: [{Key, Author, Year, Title}]}=
EDOC */
func RequestCiteKeys(w http.ResponseWriter, r *http.Request) {
	type answer struct {
		Ok      bool
		Msg     string `json:",omitempty"`
		Sources []string
		Keys    []biblio.Key
	}
	res := answer{Sources: []string{}, Keys: []biblio.Key{}}
	f := GetDb().FindByFile(r.URL.Query().Get("file"))
	if f == nil {
		res.Msg = "no such org file"
		json.NewEncoder(w).Encode(res)
		return
	}
	lines, err := readFileLines(f.Filename)
	if err != nil {
		res.Msg = err.Error()
		json.NewEncoder(w).Encode(res)
		return
	}
	entries, srcs := CiteEntries(f.Filename, lines)
	res.Ok, res.Sources = true, srcs
	for _, e := range entries {
		res.Keys = append(res.Keys, biblio.Key{Key: e.Key, Author: e.AuthorShort(false), Year: e.Year(), Title: e.Title()})
	}
	sort.Slice(res.Keys, func(a, b int) bool { return res.Keys[a].Key < res.Keys[b].Key })
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}
