// EXPORTER: MARKDOWN Export

/* SDOC: Exporters

* Markdown
  The markdown exporter hands an org file to everybody who does not use org:
  GitHub, Obsidian, and every static site generator there is.

  To enable it, add it to the =exporters= list in your orgs.yaml:

	#+BEGIN_SRC yaml
    exporters:
      - name: "markdown"
	#+END_SRC

  It needs nothing else - no template, no stylesheet, no network. Everything
  below has a default that produces a sensible GitHub-flavoured document.

	#+BEGIN_SRC yaml
    exporters:
      - name: "markdown"
        flavor: "obsidian"      # gfm (default), obsidian, commonmark
        frontMatter: "yaml"     # yaml (default), toml, none
        keyword: "bold"         # bold (default), text, tag, strip
        tags: "hash"            # hash (default), code, strip
        drawers: "strip"        # strip (default), list, keep
        planning: "strip"       # strip (default), list
        orgLinksToMd: true      # rewrite a link at foo.org to point at foo.md
        headingIds: false       # add an explicit {#anchor} to each heading
	#+END_SRC

** Flavors
   - =gfm= - GitHub Flavored Markdown. Pipe tables, =- [ ]= task lists,
     =~~strikethrough~~= and =[^1]= footnotes. This is what GitHub, GitLab and
     most static site generators read.
   - =obsidian= - GFM, plus =[[wikilinks]]= for links that point at another org
     file and =![[embeds]]= for local images, which is how Obsidian addresses
     things inside a vault.
   - =commonmark= - The lowest common denominator. No tables, no strikethrough
     and no footnote syntax, so those fall back to inline html - which
     CommonMark passes through untouched.

** What happens to the parts of org that markdown has no word for
   Markdown is a smaller language than org, and the places where that shows are
   the places worth knowing about:

   - *Property drawers and LOGBOOK* are org's own bookkeeping and are dropped by
     default. =drawers: list= writes them as a bullet list instead, and
     =drawers: keep= writes them as an indented code block, exactly as written.
   - *SCHEDULED / DEADLINE / CLOSED* are dropped by default; =planning: list=
     writes them as a line of bold labels under the heading.
   - *Headings past level six* have no markdown form - there is no =#######= -
     so they are written as bold text at the deepest indent markdown allows.
   - *Org tables with no =|---|= rule* get an empty header row, because
     GitHub's tables require one. Promoting the first row instead would turn
     data into column headings, which is a worse kind of wrong.
   - *Anything else with no equivalent* - subscripts, superscripts, latex
     fragments, =#+HTML:= - falls through as inline html, which every markdown
     dialect that matters passes through.

** Front matter
   By default the document's =#+TITLE:=, =#+AUTHOR:=, =#+DATE:= and
   =#+FILETAGS:= are written as a yaml front matter block, which is what
   Obsidian, Hugo, Jekyll and Zola all read. Any =#+MD_<name>:= keyword is
   written into it too, under =<name>= lowercased, so a file can carry whatever
   its destination wants:

	#+BEGIN_SRC org
    #+TITLE: Kitchen rebuild
    #+MD_DRAFT: false
    #+MD_WEIGHT: 20
	#+END_SRC

** Per file settings
   A file can override the exporter's configuration for itself:

	#+BEGIN_SRC org
    #+MD_FLAVOR: obsidian
    #+MD_FRONTMATTER: none
    #+MD_KEYWORD: strip
    #+MD_TAGS: strip
    #+MD_DRAWERS: list
    #+MD_PLANNING: list
	#+END_SRC

EDOC */

package markdown

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
	"gopkg.in/op/go-logging.v1"
)

// ---------------------------------------------------------------------------
// Settings
// ---------------------------------------------------------------------------

// What a document is written as. Held apart from the exporter because the
// exporter is shared between requests and a file may override any of it for
// itself - writing the file's answer onto the exporter would leave it there for
// the next file, which is a bug the html exporter has had more than once.
type settings struct {
	flavor       string
	frontMatter  string
	keyword      string
	tags         string
	drawers      string
	planning     string
	orgLinksToMd bool
	headingIds   bool
}

const (
	flavorGFM        = "gfm"
	flavorObsidian   = "obsidian"
	flavorCommonMark = "commonmark"
)

// Do tables, task lists, strikethrough and footnotes exist in this flavor?
// CommonMark is the one where they do not.
func (self *settings) extended() bool {
	return self.flavor != flavorCommonMark
}

func (self *settings) wikiLinks() bool {
	return self.flavor == flavorObsidian
}

// ---------------------------------------------------------------------------
// The exporter
// ---------------------------------------------------------------------------

type OrgMdExporter struct {
	Props map[string]interface{} `yaml:"props"`

	Flavor       string `yaml:"flavor"`
	FrontMatter  string `yaml:"frontMatter"`
	Keyword      string `yaml:"keyword"`
	Tags         string `yaml:"tags"`
	Drawers      string `yaml:"drawers"`
	Planning     string `yaml:"planning"`
	OrgLinksToMd *bool  `yaml:"orgLinksToMd"`
	HeadingIds   bool   `yaml:"headingIds"`

	out *logging.Logger
	pm  *common.PluginManager
}

// One of a fixed set, in this order of preference: what the file asked for,
// what the yaml asked for, and the default. Anything unrecognised is the
// default rather than an error - a typo in a keyword must not fail an export.
func pick(fileValue, confValue, fallback string, allowed ...string) string {
	for _, v := range []string{fileValue, confValue} {
		v = strings.ToLower(strings.TrimSpace(v))
		for _, a := range allowed {
			if v == a {
				return v
			}
		}
	}
	return fallback
}

// The settings for one document.
func (self *OrgMdExporter) settingsFor(d *org.Document, props map[string]string) settings {
	get := func(key string) string {
		if d == nil {
			return ""
		}
		return d.Get(key)
	}
	// A caller can ask for a flavor of its own, which wins over what the file
	// says - the same rule the html exporter follows for a theme, and what lets
	// a client offer "give me this file as Obsidian markdown" without editing it.
	flavor := props["flavor"]
	if flavor == "" {
		flavor = get("MD_FLAVOR")
	}
	s := settings{
		flavor:       pick(flavor, self.Flavor, flavorGFM, flavorGFM, flavorObsidian, flavorCommonMark),
		frontMatter:  pick(get("MD_FRONTMATTER"), self.FrontMatter, "yaml", "yaml", "toml", "none"),
		keyword:      pick(get("MD_KEYWORD"), self.Keyword, "bold", "bold", "text", "tag", "strip"),
		tags:         pick(get("MD_TAGS"), self.Tags, "hash", "hash", "code", "strip"),
		drawers:      pick(get("MD_DRAWERS"), self.Drawers, "strip", "strip", "list", "keep"),
		planning:     pick(get("MD_PLANNING"), self.Planning, "strip", "strip", "list"),
		orgLinksToMd: self.OrgLinksToMd == nil || *self.OrgLinksToMd,
		headingIds:   self.HeadingIds,
	}
	return s
}

func (self *OrgMdExporter) Unmarshal(unmarshal func(interface{}) error) error {
	return unmarshal(self)
}

func (self *OrgMdExporter) Startup(manager *common.PluginManager, opts *common.PluginOpts) {
	self.out = manager.Out
	self.pm = manager
}

func (self *OrgMdExporter) Export(db common.ODb, query string, to string, opts string, props map[string]string) error {
	err, str := self.ExportToString(db, query, opts, props)
	if err != nil {
		return err
	}
	return os.WriteFile(to, []byte(str), 0644)
}

func (self *OrgMdExporter) ExportToString(db common.ODb, query string, opts string, props map[string]string) (error, string) {
	f := db.FindByFile(query)
	if f == nil {
		return fmt.Errorf("markdown: no file named %s", query), ""
	}
	if props == nil {
		props = map[string]string{}
	}
	w := NewOrgMdWriter(self, db, self.settingsFor(f, props))
	w.Opts = opts
	// go-org's own `Before` would do this and is never called here, so it is
	// done outright: without it the writer's document is the empty default,
	// its Path is "" - so a relative `:DIR:` resolves against the process's
	// working directory - and `#+LINK:` abbreviations expand to nothing.
	w.Document = f
	// Before and After are not called by WriteNodes - the html exporter does not
	// call them either - so the front matter and the footnote definitions are
	// written here, around the body.
	w.writeFrontMatter(f)
	org.WriteNodes(w, f.Nodes...)
	w.writeFootnotes()
	return nil, tidy(w.String())
}

func NewMarkdownExp() *OrgMdExporter {
	return &OrgMdExporter{Props: map[string]interface{}{}}
}

func init() {
	common.AddExporter("markdown", func() common.Exporter {
		return NewMarkdownExp()
	})
	// Markdown is spelled two ways and both are what somebody would type.
	common.AddExporter("md", func() common.Exporter {
		return NewMarkdownExp()
	})
}

// ---------------------------------------------------------------------------
// The writer
// ---------------------------------------------------------------------------

// Built on go-org's html writer rather than on the bare Writer interface, the
// same way the confluence and reveal exporters are.
//
// That is not laziness: markdown *contains* html in every dialect anybody reads
// it in, so a node with no markdown form - a subscript, a latex fragment, a
// table under CommonMark - falls through to the html writer and still renders.
// It also means a node type added to go-org later compiles here without being a
// blank page.
type OrgMdWriter struct {
	*org.HTMLWriter
	Exp  *OrgMdExporter
	Set  settings
	Opts string
	db   common.ODb

	// Raw mode: inside a source block or an export block nothing is escaped.
	raw bool
	// Inside a table cell, where a bare pipe would end the cell.
	inTable bool
	// How deep a list is nested, for indenting the bullets.
	listDepth int
	// The footnotes seen, in the order they were referenced, so the numbering
	// matches the reading order rather than the order they were defined in.
	footOrder []string
	footSeen  map[string]int
	footDefs  map[string][]org.Node
	// Headline titles already used, so two headings with the same text get
	// distinct anchors the way GitHub numbers them.
	slugs map[string]int
	// The heading being written, for `[[attachment:...]]`: an attachment link
	// says which file rather than where it is, so the same name under two
	// headings is two different files.
	inHeadline *org.Headline
}

func NewOrgMdWriter(exp *OrgMdExporter, db common.ODb, set settings) *OrgMdWriter {
	w := &OrgMdWriter{
		HTMLWriter: org.NewHTMLWriter(),
		Exp:        exp,
		Set:        set,
		db:         db,
		footSeen:   map[string]int{},
		footDefs:   map[string][]org.Node{},
		slugs:      map[string]int{},
	}
	// The circular reference is what makes org.WriteNodes dispatch back into
	// these overrides rather than into the html writer's own.
	w.ExtendingWriter = w
	return w
}

func (w *OrgMdWriter) WriterWithExtensions() org.Writer { return w }

// The html writer's version swaps the builder out, which is what nesting needs,
// but it dispatches through itself - so it is restated here to keep the raw and
// table flags, which are this writer's own.
func (w *OrgMdWriter) WriteNodesAsString(nodes ...org.Node) string {
	original := w.Builder
	w.Builder = strings.Builder{}
	org.WriteNodes(w, nodes...)
	out := w.String()
	w.Builder = original
	return out
}

// ---------------------------------------------------------------------------
// Escaping
// ---------------------------------------------------------------------------

// Characters that always start something in markdown, wherever they appear.
var alwaysEscape = strings.NewReplacer(
	`\`, `\\`,
	"`", "\\`",
	`*`, `\*`,
	`[`, `\[`,
	`]`, `\]`,
	`<`, `\<`,
)

// A line that markdown would read as a heading, a bullet, a quote or a numbered
// item. Only at the start of a line, which is the only place any of them mean
// anything.
var lineStartRe = regexp.MustCompile(`^(\s*)(#{1,6}\s|[-+]\s|>\s|\d+[.)]\s|={3,}\s*$|-{3,}\s*$)`)

// An underscore that would open or close emphasis: one at a word boundary.
// Inside a word - snake_case, a file name, a variable - GFM leaves it alone, and
// escaping it there is the thing that makes machine-written markdown ugly.
var wordEdgeUnderscore = regexp.MustCompile(`(^|\W)_|_($|\W)`)

// Text, with the characters that would change its meaning escaped and nothing
// else.
//
// The temptation is to escape every punctuation mark, which is safe and makes
// prose unreadable in the source - and markdown's whole point is that the source
// is readable. So each rule below is about a character in the one position where
// it does something.
func (w *OrgMdWriter) escape(s string) string {
	if w.raw {
		return s
	}
	out := alwaysEscape.Replace(s)
	out = wordEdgeUnderscore.ReplaceAllStringFunc(out, func(m string) string {
		return strings.Replace(m, "_", `\_`, 1)
	})
	if w.inTable {
		// A pipe ends the cell. This is the one character that has to be
		// escaped inside a table and must not be escaped outside one.
		out = strings.ReplaceAll(out, "|", `\|`)
	}
	// A line that would be read as a construct is defused at its first
	// character. Applied per line, because "start of a line" is what it means.
	lines := strings.Split(out, "\n")
	for i, l := range lines {
		if m := lineStartRe.FindStringSubmatch(l); m != nil {
			lines[i] = m[1] + `\` + strings.TrimPrefix(l, m[1])
		}
	}
	return strings.Join(lines, "\n")
}

// ---------------------------------------------------------------------------
// Front matter
// ---------------------------------------------------------------------------

// The keywords that become front matter under their own names, in the order
// they are written. Anything else has to be asked for with #+MD_<name>:.
var frontMatterKeys = []struct{ key, as string }{
	{"TITLE", "title"},
	{"AUTHOR", "author"},
	{"DATE", "date"},
	{"DESCRIPTION", "description"},
	{"EMAIL", "email"},
}

func (w *OrgMdWriter) writeFrontMatter(d *org.Document) {
	if w.Set.frontMatter == "none" || d == nil {
		return
	}
	type pair struct {
		key  string
		val  string
		list []string
	}
	var out []pair
	for _, k := range frontMatterKeys {
		if v := strings.TrimSpace(d.Get(k.key)); v != "" {
			out = append(out, pair{key: k.as, val: v})
		}
	}
	// File tags are a list, and a list is what every front matter reader wants
	// them as - "tags: [a, b]" rather than "tags: :a:b:".
	if v := strings.TrimSpace(d.Get("FILETAGS")); v != "" {
		var tags []string
		for _, t := range strings.Split(v, ":") {
			if t = strings.TrimSpace(t); t != "" {
				tags = append(tags, t)
			}
		}
		if len(tags) > 0 {
			out = append(out, pair{key: "tags", list: tags})
		}
	}
	// Anything the file asked for by name. Sorted, because BufferSettings is a
	// map and an export that reordered its own front matter between two runs
	// would show up as a diff in every file every time.
	var extra []string
	for k := range d.BufferSettings {
		if strings.HasPrefix(k, "MD_") && !isMdSetting(k) {
			extra = append(extra, k)
		}
	}
	sort.Strings(extra)
	for _, k := range extra {
		name := strings.ToLower(strings.TrimPrefix(k, "MD_"))
		if v := strings.TrimSpace(d.Get(k)); v != "" {
			out = append(out, pair{key: name, val: v})
		}
	}
	if len(out) == 0 {
		return
	}

	if w.Set.frontMatter == "toml" {
		w.WriteString("+++\n")
		for _, p := range out {
			if p.list != nil {
				w.WriteString(fmt.Sprintf("%s = [%s]\n", p.key, strings.Join(quoteAll(p.list, tomlQuote), ", ")))
			} else {
				w.WriteString(fmt.Sprintf("%s = %s\n", p.key, tomlQuote(p.val)))
			}
		}
		w.WriteString("+++\n\n")
		return
	}
	w.WriteString("---\n")
	for _, p := range out {
		if p.list != nil {
			w.WriteString(fmt.Sprintf("%s: [%s]\n", p.key, strings.Join(quoteAll(p.list, yamlQuote), ", ")))
		} else {
			w.WriteString(fmt.Sprintf("%s: %s\n", p.key, yamlQuote(p.val)))
		}
	}
	w.WriteString("---\n\n")
}

// The MD_ keywords that configure this exporter rather than describing the
// document. They must not end up in the front matter.
func isMdSetting(k string) bool {
	switch k {
	case "MD_FLAVOR", "MD_FRONTMATTER", "MD_KEYWORD", "MD_TAGS", "MD_DRAWERS", "MD_PLANNING":
		return true
	}
	return false
}

func quoteAll(vs []string, q func(string) string) []string {
	out := make([]string, len(vs))
	for i, v := range vs {
		out[i] = q(v)
	}
	return out
}

// A yaml scalar. Quoted whenever it could be read as something other than a
// string - a number, a bool, a date, or anything with a colon in it - because a
// title of "1.0" read back as a float is a title nobody can search for.
func yamlQuote(s string) string {
	if s == "" {
		return `""`
	}
	plain := true
	for _, r := range s {
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' || r == '-' || r == '_' || r == '.' || r == '/') {
			plain = false
			break
		}
	}
	if plain && !looksLikeScalar(s) && !strings.HasPrefix(s, " ") && !strings.HasSuffix(s, " ") {
		return s
	}
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`).Replace(s) + `"`
}

func tomlQuote(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`).Replace(s) + `"`
}

// A target that names a picture. go-org's own RegularLink.Kind() answers
// "regular" for `[[file:shed.png][a shed]]` - a link with a description - because
// in html that becomes an anchor wrapped round an img. Markdown has no such
// thing: the description *is* the alt text, so the extension is what decides.
var imageExtRe = regexp.MustCompile(`(?i)[.](png|gif|jpe?g|svg|tiff?|webp|avif|bmp|ico)$`)

func isImageTarget(target string) bool {
	if i := strings.IndexAny(target, "?#"); i >= 0 {
		target = target[:i]
	}
	return imageExtRe.MatchString(target)
}

var scalarRe = regexp.MustCompile(`^(-?\d+(\.\d+)?|true|false|yes|no|on|off|null|~|\d{4}-\d{2}-\d{2}.*)$`)

func looksLikeScalar(s string) bool {
	return scalarRe.MatchString(strings.ToLower(s))
}

// ---------------------------------------------------------------------------
// Headlines
// ---------------------------------------------------------------------------

// GitHub's anchor for a heading: lower cased, punctuation dropped, spaces
// hyphenated, and a number appended when the same text appears twice.
var slugDrop = regexp.MustCompile(`[^\p{L}\p{N}\- ]+`)

func slugOf(title string) string {
	s := strings.ToLower(strings.TrimSpace(title))
	s = slugDrop.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, " ", "-")
	return s
}

func (w *OrgMdWriter) anchorFor(title string) string {
	base := slugOf(title)
	n := w.slugs[base]
	w.slugs[base] = n + 1
	if n == 0 {
		return base
	}
	return fmt.Sprintf("%s-%d", base, n)
}

func (w *OrgMdWriter) WriteHeadline(h org.Headline) {
	if h.IsExcluded(w.Document) {
		return
	}
	was := w.inHeadline
	w.inHeadline = &h
	defer func() { w.inHeadline = was }()
	title := strings.TrimSpace(w.WriteNodesAsString(h.Title...))

	var lead []string
	if h.Status != "" {
		switch w.Set.keyword {
		case "bold":
			lead = append(lead, "**"+h.Status+"**")
		case "text":
			lead = append(lead, h.Status)
		case "tag":
			lead = append(lead, "#"+h.Status)
		}
	}
	if h.Priority != "" && w.Set.keyword != "strip" {
		lead = append(lead, "`#"+h.Priority+"`")
	}
	line := strings.TrimSpace(strings.Join(append(lead, title), " "))

	if len(h.Tags) > 0 && w.Set.tags != "strip" {
		var ts []string
		for _, t := range h.Tags {
			switch w.Set.tags {
			case "hash":
				// Obsidian reads these as tags and everything else draws them
				// as plain words, which is the right failure.
				ts = append(ts, "#"+t)
			case "code":
				ts = append(ts, "`"+t+"`")
			}
		}
		line = strings.TrimSpace(line + "  " + strings.Join(ts, " "))
	}

	// Markdown stops at six levels. Anything deeper is written as bold text at
	// the deepest heading available, which keeps it visually a heading and keeps
	// the document's structure legible - `#######` renders as literal hashes.
	if h.Lvl <= 6 {
		anchor := w.anchorFor(title)
		w.WriteString("\n" + strings.Repeat("#", h.Lvl) + " " + line)
		if w.Set.headingIds && anchor != "" {
			w.WriteString(" {#" + anchor + "}")
		}
		w.WriteString("\n\n")
	} else {
		w.anchorFor(title)
		w.WriteString("\n###### " + strings.TrimSpace("**"+line+"**") + "\n\n")
	}

	w.writePlanning(h)
	if h.Properties != nil {
		w.WritePropertyDrawer(*h.Properties)
	}
	org.WriteNodes(w, h.Children...)
}

// SCHEDULED, DEADLINE and CLOSED. Markdown has no notion of any of them, so by
// default they are dropped - a README does not want them - and `planning: list`
// writes them as one line of bold labels for whoever is exporting a plan rather
// than a document.
func (w *OrgMdWriter) writePlanning(h org.Headline) {
	if w.Set.planning != "list" {
		return
	}
	var parts []string
	add := func(label string, s *org.SDC) {
		if s.IsZero() {
			return
		}
		// Without org's own brackets: they say active or inactive, which
		// markdown has no notion of, and `<...>` is close enough to an html tag
		// to be worth not writing by accident.
		parts = append(parts, fmt.Sprintf("**%s:** %s", label, bareDate(s.Date.ToDate())))
	}
	add("Scheduled", h.Scheduled)
	add("Deadline", h.Deadline)
	add("Closed", h.Closed)
	if len(parts) > 0 {
		w.WriteString(strings.Join(parts, " &middot; ") + "\n\n")
	}
}

func bareDate(s string) string {
	return strings.Trim(s, "<>[]")
}

// The planning lines also arrive as children of the headline, so they would be
// written twice. This is the one that fires for those, and it writes nothing -
// writePlanning above has already had its say.
func (w *OrgMdWriter) WriteSDC(s org.SDC) {}

// A clock line is a record of work, not part of the document.
func (w *OrgMdWriter) WriteClock(c org.Clock) {}

// ---------------------------------------------------------------------------
// Drawers
// ---------------------------------------------------------------------------

func (w *OrgMdWriter) WritePropertyDrawer(d org.PropertyDrawer) {
	if w.Set.drawers == "strip" || len(d.Properties) == 0 {
		return
	}
	if w.Set.drawers == "keep" {
		w.WriteString("```\n:PROPERTIES:\n")
		for _, p := range d.Properties {
			w.WriteString(fmt.Sprintf(":%s: %s\n", p[0], p[1]))
		}
		w.WriteString(":END:\n```\n\n")
		return
	}
	for _, p := range d.Properties {
		w.WriteString(fmt.Sprintf("- **%s:** %s\n", p[0], p[1]))
	}
	w.WriteString("\n")
}

func (w *OrgMdWriter) WriteDrawer(d org.Drawer) {
	// A LOGBOOK is clock lines and state changes - org's record of what
	// happened, not the document. It goes even when other drawers are kept.
	if strings.EqualFold(d.Name, "LOGBOOK") || w.Set.drawers == "strip" {
		return
	}
	if w.Set.drawers == "keep" {
		w.WriteString("```\n:" + d.Name + ":\n")
		w.WriteString(strings.TrimRight(w.WriteNodesAsString(d.Children...), "\n"))
		w.WriteString("\n:END:\n```\n\n")
		return
	}
	w.WriteString("**" + d.Name + "**\n\n")
	org.WriteNodes(w, d.Children...)
}

// ---------------------------------------------------------------------------
// Blocks
// ---------------------------------------------------------------------------

func isRawBlock(name string) bool {
	switch strings.ToUpper(name) {
	case "SRC", "EXAMPLE", "EXPORT":
		return true
	}
	return false
}

// The content of a block, with escaping turned off for the kinds whose content
// is meant to be taken literally.
func (w *OrgMdWriter) blockContent(name string, children []org.Node) string {
	if !isRawBlock(name) {
		return w.WriteNodesAsString(children...)
	}
	was := w.raw
	w.raw = true
	out := w.WriteNodesAsString(children...)
	w.raw = was
	return strings.TrimRightFunc(out, unicode.IsSpace)
}

// A fence long enough to hold content that itself contains backticks. Three is
// almost always right; a block quoting markdown is where it is not.
func fenceFor(content string) string {
	longest := 0
	run := 0
	for _, r := range content {
		if r == '`' {
			run++
			if run > longest {
				longest = run
			}
		} else {
			run = 0
		}
	}
	if longest < 3 {
		return "```"
	}
	return strings.Repeat("`", longest+1)
}

func (w *OrgMdWriter) WriteBlock(b org.Block) {
	content, params := w.blockContent(b.Name, b.Children), b.ParameterMap()

	switch strings.ToUpper(b.Name) {
	case "SRC":
		if params[":exports"] == "results" || params[":exports"] == "none" {
			break
		}
		lang := ""
		if len(b.Parameters) >= 1 {
			lang = strings.ToLower(b.Parameters[0])
		}
		fence := fenceFor(content)
		w.WriteString(fence + lang + "\n" + content + "\n" + fence + "\n\n")
	case "EXAMPLE":
		fence := fenceFor(content)
		w.WriteString(fence + "\n" + content + "\n" + fence + "\n\n")
	case "EXPORT":
		// An export block names who it is for. Markdown takes its own and
		// html's, since markdown carries html; everything else is for another
		// exporter and is not this document's business.
		if len(b.Parameters) >= 1 {
			switch strings.ToLower(b.Parameters[0]) {
			case "md", "markdown", "html":
				w.WriteString(content + "\n\n")
			}
		}
	case "QUOTE":
		w.WriteString(blockQuote(content) + "\n")
	case "VERSE":
		// Verse keeps its line breaks, and the only way markdown keeps a line
		// break inside a paragraph is a hard break at the end of each line.
		lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
		w.WriteString(strings.Join(lines, "  \n") + "\n\n")
	case "CENTER":
		// Nothing in markdown centres anything, so this is one of the places
		// html earns its keep.
		w.WriteString("<div align=\"center\">\n\n" + content + "\n</div>\n\n")
	default:
		w.WriteString(content)
		if !strings.HasSuffix(content, "\n") {
			w.WriteString("\n")
		}
		w.WriteString("\n")
	}

	if b.Result != nil && params[":exports"] != "code" && params[":exports"] != "none" {
		org.WriteNodes(w, b.Result)
	}
}

// Every line prefixed with "> ", including the blank ones, which is what keeps
// a multi-paragraph quote one quote.
func blockQuote(content string) string {
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	for i, l := range lines {
		if l == "" {
			lines[i] = ">"
		} else {
			lines[i] = "> " + l
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

func (w *OrgMdWriter) WriteInlineBlock(b org.InlineBlock) {
	content := w.blockContent(strings.ToUpper(b.Name), b.Children)
	switch b.Name {
	case "src":
		w.WriteString("`" + content + "`")
	case "export":
		if len(b.Parameters) >= 1 {
			switch strings.ToLower(b.Parameters[0]) {
			case "md", "markdown", "html":
				w.WriteString(content)
			}
		}
	}
}

func (w *OrgMdWriter) WriteExample(e org.Example) {
	was := w.raw
	w.raw = true
	content := strings.TrimRightFunc(w.WriteNodesAsString(e.Children...), unicode.IsSpace)
	w.raw = was
	fence := fenceFor(content)
	w.WriteString(fence + "\n" + content + "\n" + fence + "\n\n")
}

func (w *OrgMdWriter) WriteResult(r org.Result) {
	org.WriteNodes(w, r.Node)
}

// ---------------------------------------------------------------------------
// Inline
// ---------------------------------------------------------------------------

func (w *OrgMdWriter) WriteText(t org.Text) {
	w.WriteString(w.escape(t.Content))
}

func (w *OrgMdWriter) WriteEmphasis(e org.Emphasis) {
	content := w.WriteNodesAsString(e.Content...)
	switch e.Kind {
	case "/":
		w.WriteString("*" + content + "*")
	case "*":
		w.WriteString("**" + content + "**")
	case "+":
		if w.Set.extended() {
			w.WriteString("~~" + content + "~~")
		} else {
			w.WriteString("<del>" + content + "</del>")
		}
	case "~", "=":
		// Verbatim and code both become code, and what is inside is taken
		// literally - so it is written raw rather than escaped, and given a
		// fence long enough to hold any backticks of its own.
		raw := strings.TrimSpace(org.String(e.Content...))
		w.WriteString(inlineCode(raw))
	case "_":
		// Markdown has no underline. This is html or nothing.
		w.WriteString("<u>" + content + "</u>")
	case "_{}":
		w.WriteString("<sub>" + content + "</sub>")
	case "^{}":
		w.WriteString("<sup>" + content + "</sup>")
	default:
		w.WriteString(content)
	}
}

// Inline code, with a fence that does not collide with the content and the
// padding spaces markdown needs when the content starts or ends with a backtick.
func inlineCode(s string) string {
	fence := "`"
	for strings.Contains(s, fence) {
		fence += "`"
	}
	pad := ""
	if strings.HasPrefix(s, "`") || strings.HasSuffix(s, "`") {
		pad = " "
	}
	return fence + pad + s + pad + fence
}

func (w *OrgMdWriter) WriteStatisticToken(s org.StatisticToken) {
	w.WriteString("`[" + s.Content + "]`")
}

func (w *OrgMdWriter) WriteLineBreak(l org.LineBreak) {
	w.WriteString(strings.Repeat("\n", l.Count))
}

// Markdown's hard break: two spaces then the newline.
func (w *OrgMdWriter) WriteExplicitLineBreak(l org.ExplicitLineBreak) {
	w.WriteString("  \n")
}

func (w *OrgMdWriter) WriteLatexFragment(l org.LatexFragment) {
	// Passed through as written. Every place that renders maths in markdown -
	// GitHub, Obsidian, Hugo with KaTeX - reads the delimiters org already uses.
	was := w.raw
	w.raw = true
	w.WriteString(l.OpeningPair)
	org.WriteNodes(w, l.Content...)
	w.WriteString(l.ClosingPair)
	w.raw = was
}

func (w *OrgMdWriter) WriteTimestamp(t org.Timestamp) {
	if w.Document != nil && w.Document.GetOption("<") == "nil" {
		return
	}
	w.WriteString("`" + t.Time.ToDate() + "`")
}

func (w *OrgMdWriter) WriteMacro(m org.Macro) {
	if macro := w.Document.Macros[m.Name]; macro != "" {
		for i, param := range m.Parameters {
			macro = strings.Replace(macro, fmt.Sprintf("$%d", i+1), param, -1)
		}
		macroDocument := w.Document.Parse(strings.NewReader(macro), w.Document.Path)
		if macroDocument.Error == nil {
			org.WriteNodes(w, macroDocument.Nodes...)
		}
	}
}

// ---------------------------------------------------------------------------
// Links
// ---------------------------------------------------------------------------

// Where a link points, once org's own ways of saying it have been turned into
// something a markdown reader can follow.
//
// This is the part of the export that only a server holding the whole database
// can do: an `id:` link names a heading by a uuid written in some other file,
// and turning that into `other.md#the-heading` needs the index.
func (w *OrgMdWriter) linkTarget(l org.RegularLink) string {
	url := l.URL
	switch l.Protocol {
	case common.AttachProtocol:
		return w.attachTarget(l.URL)
	case "id", "custom-id":
		return w.resolveId(strings.TrimPrefix(strings.TrimPrefix(url, l.Protocol), ":"))
	case "file":
		url = strings.TrimPrefix(url, "file:")
	case "":
		// A bare `*Heading` link is a heading in this document.
		if strings.HasPrefix(url, "*") {
			return "#" + slugOf(strings.TrimPrefix(url, "*"))
		}
		if strings.HasPrefix(url, "#") {
			return "#" + slugOf(strings.TrimPrefix(url, "#"))
		}
	default:
		// A link abbreviation from #+LINK:, expanded the way org expands it.
		if prefix := w.Document.Links[l.Protocol]; prefix != "" {
			tag := strings.TrimPrefix(url, l.Protocol+":")
			if strings.Contains(prefix, "%s") {
				return strings.ReplaceAll(prefix, "%s", tag)
			}
			return prefix + tag
		}
		return url
	}

	// A `::` search inside a file becomes an anchor, which is how both GitHub
	// and Obsidian address a heading in another document.
	anchor := ""
	if i := strings.Index(url, "::"); i >= 0 {
		anchor = strings.TrimPrefix(strings.TrimPrefix(url[i+2:], "*"), "#")
		url = url[:i]
	}
	if w.Set.orgLinksToMd && strings.HasSuffix(strings.ToLower(url), ".org") {
		url = strings.TrimSuffix(url, path.Ext(url)) + ".md"
	}
	if anchor != "" {
		url += "#" + slugOf(anchor)
	}
	return url
}

// `[[attachment:report.pdf]]` as a path a markdown reader can follow.
//
// Relative to the org file, because that is where the markdown is going to sit
// - the same rule every other local link here follows - and left as it was when
// the heading owns no folder, since a link that still says `attachment:` is one
// somebody can act on and an invented path is not.
func (w *OrgMdWriter) attachTarget(target string) string {
	name, ok := common.AttachLinkName(target)
	if !ok || w.inHeadline == nil {
		return target
	}
	orgFile := ""
	if w.Document != nil {
		orgFile = w.Document.Path
	}
	dirs := []string{}
	set := common.AttachSettings{}
	if w.Exp != nil && w.Exp.pm != nil {
		dirs = w.Exp.pm.OrgDirs
		set = w.Exp.pm.Attach
	}
	dir, from, _ := common.AttachDirFrom(
		mdHeadlineProp(w.inHeadline, "DIR", "ATTACH_DIR"),
		mdHeadlineProp(w.inHeadline, "ID"),
		orgFile, dirs, set)
	if dir == "" || from == "" {
		return target
	}
	abs := filepath.Join(dir, filepath.FromSlash(name))
	if orgFile != "" {
		if rel, err := filepath.Rel(filepath.Dir(orgFile), abs); err == nil && !strings.HasPrefix(rel, "..") {
			return filepath.ToSlash(rel)
		}
	}
	return filepath.ToSlash(abs)
}

func mdHeadlineProp(h *org.Headline, names ...string) string {
	if h == nil || h.Properties == nil {
		return ""
	}
	for _, p := range h.Properties.Properties {
		for _, n := range names {
			if strings.EqualFold(p[0], n) {
				return p[1]
			}
		}
	}
	return ""
}

// An `id:` or `custom-id:` link, resolved against the database.
//
// Answering with the raw id would be a link to nothing; answering with the
// heading it names is a link somebody can follow. A target the database has
// never heard of is left as the id, because inventing an anchor for it would be
// a link to nothing that looks like a link to something.
func (w *OrgMdWriter) resolveId(id string) string {
	if w.db == nil {
		return "#" + slugOf(id)
	}
	td := w.db.FindByAnyId(id)
	if td == nil {
		return "#" + slugOf(id)
	}
	anchor := "#" + slugOf(td.Headline)
	// In this document it is a bare anchor; in another it needs the file too.
	if w.Document != nil && td.Filename != "" && td.Filename != w.Document.Path {
		name := path.Base(td.Filename)
		if w.Set.orgLinksToMd && strings.HasSuffix(strings.ToLower(name), ".org") {
			name = strings.TrimSuffix(name, path.Ext(name)) + ".md"
		}
		if w.Set.wikiLinks() {
			return strings.TrimSuffix(name, path.Ext(name)) + anchor
		}
		return name + anchor
	}
	return anchor
}

func (w *OrgMdWriter) WriteRegularLink(l org.RegularLink) {
	target := w.linkTarget(l)
	description := ""
	if l.Description != nil {
		description = strings.TrimSpace(w.WriteNodesAsString(l.Description...))
	}
	// An attachment with nothing written over it reads as its own name. The
	// resolved path is three folders of uuid and says nothing a person wants.
	if description == "" {
		if name, ok := common.AttachLinkName(l.URL); ok {
			description = path.Base(name)
		}
	}
	isImage := l.Kind() == "image" || isImageTarget(target)
	local := l.Protocol == "file" || l.Protocol == ""

	// Obsidian addresses things in the vault by name rather than by path, and
	// an embed is how it shows a picture. Only for local targets: a url is a url
	// in every dialect.
	if w.Set.wikiLinks() && local && !strings.HasPrefix(target, "#") {
		name := strings.TrimSuffix(target, path.Ext(target))
		if isImage {
			// A picture keeps its extension - the embed names a file, not a note.
			if description != "" {
				return
			}
			w.WriteString("![[" + target + "]]")
			return
		}
		if description != "" && description != target {
			w.WriteString("[[" + name + "|" + description + "]]")
		} else {
			w.WriteString("[[" + name + "]]")
		}
		return
	}

	if isImage {
		alt := description
		if alt == "" {
			alt = path.Base(target)
		}
		w.WriteString("![" + escapeLinkText(alt) + "](" + escapeURL(target) + ")")
		return
	}

	switch l.Kind() {
	case "video":
		// No markdown dialect has a video. The html works everywhere that
		// renders html and degrades to a link everywhere that does not.
		w.WriteString(`<video src="` + escapeURL(target) + `" controls></video>`)
	default:
		if description == "" {
			// An autolink: the url as its own text, which is what org means by
			// a link with nothing written over it.
			if l.AutoLink && w.Set.extended() {
				w.WriteString("<" + target + ">")
				return
			}
			description = target
		}
		w.WriteString("[" + escapeLinkText(description) + "](" + escapeURL(target) + ")")
	}
}

// Inside the square brackets of a link, only the brackets themselves matter.
func escapeLinkText(s string) string {
	return strings.NewReplacer(`[`, `\[`, `]`, `\]`).Replace(s)
}

// Inside the round brackets, a space or a bracket ends the url early. Angle
// brackets are markdown's own way of saying "all of this is the address".
func escapeURL(s string) string {
	if strings.ContainsAny(s, " ()") {
		return "<" + strings.NewReplacer("<", "%3C", ">", "%3E").Replace(s) + ">"
	}
	return s
}

// ---------------------------------------------------------------------------
// Lists
// ---------------------------------------------------------------------------

func (w *OrgMdWriter) WriteList(l org.List) {
	w.listDepth++
	org.WriteNodes(w, l.Items...)
	w.listDepth--
	if w.listDepth == 0 {
		w.WriteString("\n")
	}
}

// Two spaces per level, which every dialect reads as nesting. Four would also
// work for a bullet and would turn a nested item under an ordered list into a
// code block, so two it is.
func (w *OrgMdWriter) listIndent() string {
	if w.listDepth <= 1 {
		return ""
	}
	return strings.Repeat("  ", w.listDepth-1)
}

func (w *OrgMdWriter) WriteListItem(li org.ListItem) {
	bullet := "-"
	if strings.ContainsAny(li.Bullet, ".)") {
		bullet = "1."
		if li.Value != "" {
			bullet = li.Value + "."
		}
	}
	box := ""
	if li.Status != "" && w.Set.extended() {
		switch li.Status {
		case "X":
			box = "[x] "
		case "-":
			// Markdown has no third state. An unfinished box is the honest one.
			box = "[ ] "
		default:
			box = "[ ] "
		}
	}
	indent := w.listIndent()
	content := w.itemContent(li.Children, indent)
	w.WriteString(indent + bullet + " " + box + content)
}

func (w *OrgMdWriter) WriteDescriptiveListItem(di org.DescriptiveListItem) {
	// Markdown has no description list. Bold term, then the details, which is
	// what everybody writes by hand and what every dialect renders.
	indent := w.listIndent()
	term := "?"
	if len(di.Term) != 0 {
		term = strings.TrimSpace(w.WriteNodesAsString(di.Term...))
	}
	box := ""
	if di.Status != "" && w.Set.extended() {
		box = "[ ] "
		if di.Status == "X" {
			box = "[x] "
		}
	}
	content := strings.TrimLeft(w.itemContent(di.Details, indent), " ")
	w.WriteString(indent + "- " + box + "**" + term + "**")
	if content != "" {
		w.WriteString(" — " + content)
	} else {
		w.WriteString("\n")
	}
}

// A line that is a bullet, at any indent.
var bulletLine = regexp.MustCompile(`^\s*([-*+]|\d+[.)])\s`)

// The body of a list item, with every line after the first indented to line up
// under the bullet - which is what keeps a second paragraph, or a nested list,
// inside the item rather than ending it.
func (w *OrgMdWriter) itemContent(children []org.Node, indent string) string {
	out := w.WriteNodesAsString(children...)
	out = strings.TrimRight(out, "\n")
	if out == "" {
		return "\n"
	}
	lines := strings.Split(out, "\n")
	// An item's text is written as a paragraph, and a paragraph ends with a
	// blank line - so a nested list arrives one blank line below the text it
	// hangs off. Markdown reads that as a *loose* list and wraps every item of
	// the whole list in its own paragraph, which puts a line of air between
	// each one. Nothing in the parse says whether the author meant loose or
	// tight, and tight is what a nested list is nearly always meant to be.
	var kept []string
	for i, l := range lines {
		if l == "" && i+1 < len(lines) && bulletLine.MatchString(lines[i+1]) {
			continue
		}
		kept = append(kept, l)
	}
	lines = kept
	for i := 1; i < len(lines); i++ {
		if lines[i] == "" {
			continue
		}
		// A nested list has already indented itself, so this only pads the
		// lines that are this item's own prose.
		if !strings.HasPrefix(lines[i], indent+"  ") {
			lines[i] = indent + "  " + lines[i]
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

// ---------------------------------------------------------------------------
// Tables
// ---------------------------------------------------------------------------

// go-org spells a column's alignment out - "left", not "l" - and infers it for
// a column of numbers even where the file did not ask.
var alignMarkers = map[string]string{
	"":       "---",
	"left":   ":---",
	"center": ":---:",
	"right":  "---:",
}

func (w *OrgMdWriter) WriteTable(t org.Table) {
	// CommonMark has no tables at all, so the html writer's version is the only
	// honest answer there.
	if !w.Set.extended() {
		w.HTMLWriter.WriteTable(t)
		w.WriteString("\n")
		return
	}

	was := w.inTable
	w.inTable = true
	defer func() { w.inTable = was }()

	// The rows, as cells, and the widest cell in each column - so the source is
	// readable, which is the whole reason anybody writes markdown by hand.
	type row struct {
		cells []string
		rule  bool
	}
	var rows []row
	widths := map[int]int{}
	cols := 0
	for _, r := range t.Rows {
		// A `|---|` rule is both empty *and* special, so it has to be
		// recognised before the special check rather than after it - the other
		// way round it is skipped, no header is ever found, and every table
		// comes out with a blank header row and its real one as data.
		if len(r.Columns) == 0 {
			rows = append(rows, row{rule: true})
			continue
		}
		// An alignment row (`| <l> | <r> |`) or a formula row is org's own
		// machinery and is not part of the table's content.
		if r.IsSpecial {
			continue
		}
		var cells []string
		for i, c := range r.Columns {
			cell := strings.TrimSpace(w.WriteNodesAsString(c.Children...))
			cells = append(cells, cell)
			if len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
		if len(cells) > cols {
			cols = len(cells)
		}
		rows = append(rows, row{cells: cells})
	}
	if cols == 0 {
		return
	}

	align := make([]string, cols)
	for i := 0; i < cols; i++ {
		if i < len(t.ColumnInfos) {
			align[i] = alignMarkers[t.ColumnInfos[i].Align]
		}
		if align[i] == "" {
			align[i] = "---"
		}
		if len(align[i]) > widths[i] {
			widths[i] = len(align[i])
		}
	}

	write := func(cells []string) {
		w.WriteString("|")
		for i := 0; i < cols; i++ {
			cell := ""
			if i < len(cells) {
				cell = cells[i]
			}
			w.WriteString(" " + cell + strings.Repeat(" ", widths[i]-len(cell)) + " |")
		}
		w.WriteString("\n")
	}
	// The rule is padded out to the column width with its own dashes, so it
	// lines up with the cells above it rather than sitting short of them.
	writeRule := func() {
		w.WriteString("|")
		for i := 0; i < cols; i++ {
			left := strings.HasPrefix(align[i], ":")
			right := strings.HasSuffix(align[i], ":")
			dashes := widths[i]
			if left {
				dashes--
			}
			if right {
				dashes--
			}
			if dashes < 3 {
				dashes = 3
			}
			marker := strings.Repeat("-", dashes)
			if left {
				marker = ":" + marker
			}
			if right {
				marker = marker + ":"
			}
			w.WriteString(" " + marker + " |")
		}
		w.WriteString("\n")
	}

	// GitHub's tables have exactly one header rule and it must be the second
	// line. An org table can have a rule anywhere, or none at all.
	//
	// With no rule there is no header, and markdown still insists on one - so
	// an empty header row is written. Promoting the first row instead is the
	// obvious alternative and is worse: it turns data into column headings and
	// nothing on the page says it happened.
	headerRows := 0
	for i, r := range rows {
		if r.rule {
			headerRows = i
			break
		}
	}
	if headerRows == 0 {
		write(make([]string, cols))
		writeRule()
		for _, r := range rows {
			if r.rule {
				continue
			}
			write(r.cells)
		}
	} else {
		for i := 0; i < headerRows; i++ {
			write(rows[i].cells)
		}
		writeRule()
		for _, r := range rows[headerRows:] {
			// Every rule after the first is dropped: markdown has nowhere to
			// put it, and a second line of dashes in the body is drawn as data.
			if r.rule {
				continue
			}
			write(r.cells)
		}
	}
	w.WriteString("\n")
}

// ---------------------------------------------------------------------------
// Footnotes
// ---------------------------------------------------------------------------

func (w *OrgMdWriter) WriteFootnoteLink(l org.FootnoteLink) {
	if w.Document != nil && w.Document.GetOption("f") == "nil" {
		return
	}
	if _, seen := w.footSeen[l.Name]; !seen {
		w.footSeen[l.Name] = len(w.footOrder) + 1
		w.footOrder = append(w.footOrder, l.Name)
	}
	if l.Definition != nil {
		if _, have := w.footDefs[l.Name]; !have {
			w.footDefs[l.Name] = l.Definition.Children
		}
	}
	n := w.footSeen[l.Name]
	if w.Set.extended() {
		w.WriteString(fmt.Sprintf("[^%d]", n))
		return
	}
	// CommonMark has no footnotes, so the reference is a link to an anchor at
	// the foot of the document and the definition is written under it.
	w.WriteString(fmt.Sprintf(`<sup><a href="#fn-%d" id="fnref-%d">%d</a></sup>`, n, n, n))
}

func (w *OrgMdWriter) WriteFootnoteDefinition(f org.FootnoteDefinition) {
	// Kept rather than written where it stands: markdown gathers footnotes at
	// the end, and org lets them be defined anywhere.
	w.footDefs[f.Name] = f.Children
	if _, seen := w.footSeen[f.Name]; !seen {
		w.footSeen[f.Name] = len(w.footOrder) + 1
		w.footOrder = append(w.footOrder, f.Name)
	}
}

func (w *OrgMdWriter) writeFootnotes() {
	if len(w.footOrder) == 0 {
		return
	}
	w.WriteString("\n")
	for _, name := range w.footOrder {
		n := w.footSeen[name]
		body := strings.TrimSpace(w.WriteNodesAsString(w.footDefs[name]...))
		// A footnote referenced but never defined still gets a line, so the
		// reference is not a link to nothing.
		if body == "" {
			body = "*(no definition)*"
		}
		if w.Set.extended() {
			w.WriteString(fmt.Sprintf("[^%d]: %s\n", n, indentContinuation(body)))
		} else {
			w.WriteString(fmt.Sprintf(`<p id="fn-%d"><sup>%d</sup> %s <a href="#fnref-%d">&#8617;</a></p>`+"\n", n, n, body, n))
		}
	}
}

// The second and later lines of a footnote have to be indented to stay part of
// it.
func indentContinuation(s string) string {
	lines := strings.Split(s, "\n")
	for i := 1; i < len(lines); i++ {
		if lines[i] != "" {
			lines[i] = "    " + lines[i]
		}
	}
	return strings.Join(lines, "\n")
}

// ---------------------------------------------------------------------------
// The rest
// ---------------------------------------------------------------------------

func (w *OrgMdWriter) WriteParagraph(p org.Paragraph) {
	if len(p.Children) == 0 {
		return
	}
	content := strings.TrimSpace(w.WriteNodesAsString(p.Children...))
	if content == "" {
		return
	}
	w.WriteString(content + "\n\n")
}

func (w *OrgMdWriter) WriteHorizontalRule(h org.HorizontalRule) {
	w.WriteString("---\n\n")
}

func (w *OrgMdWriter) WriteComment(c org.Comment) {}

func (w *OrgMdWriter) WriteKeyword(k org.Keyword) {
	switch strings.ToUpper(k.Key) {
	case "HTML":
		w.WriteString(k.Value + "\n")
	case "MD", "MARKDOWN":
		w.WriteString(k.Value + "\n")
	}
	// Everything else - TITLE, TODO, TBLFM, STARTUP, PROPERTY - is either in the
	// front matter already or is org's own machinery, and belongs in neither.
}

func (w *OrgMdWriter) WriteInclude(i org.Include) {
	org.WriteNodes(w, i.Resolve())
}

// A #+CAPTION: on a table or a picture. Markdown has no caption, so it is
// written as the emphasised line underneath that everybody uses for one.
func (w *OrgMdWriter) WriteNodeWithMeta(n org.NodeWithMeta) {
	org.WriteNodes(w, n.Node)
	if len(n.Meta.Caption) > 0 {
		var parts []string
		for _, c := range n.Meta.Caption {
			parts = append(parts, strings.TrimSpace(w.WriteNodesAsString(c...)))
		}
		if caption := strings.TrimSpace(strings.Join(parts, " ")); caption != "" {
			w.WriteString("*" + caption + "*\n\n")
		}
	}
}

func (w *OrgMdWriter) WriteNodeWithName(n org.NodeWithName) {
	org.WriteNodes(w, n.Node)
}

// ---------------------------------------------------------------------------
// Tidying
// ---------------------------------------------------------------------------

var manyBlankLines = regexp.MustCompile(`\n{3,}`)

// The writers above each end what they write with a blank line, which is the
// only way each one can be written without knowing what follows it - and which
// leaves runs of them wherever two meet. Squeezed here, once, rather than every
// writer trying to guess whether it is last.
func tidy(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	// Trailing spaces are markdown's hard line break, so only runs of three or
	// more are stripped - two are deliberate.
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		trimmed := strings.TrimRight(l, " \t")
		if len(l)-len(trimmed) == 2 && trimmed != "" {
			lines[i] = trimmed + "  "
		} else {
			lines[i] = trimmed
		}
	}
	s = strings.Join(lines, "\n")
	s = manyBlankLines.ReplaceAllString(s, "\n\n")
	s = strings.TrimLeft(s, "\n")
	return strings.TrimRight(s, "\n") + "\n"
}
