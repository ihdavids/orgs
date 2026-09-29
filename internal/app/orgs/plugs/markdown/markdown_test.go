package markdown

import (
	"strings"
	"testing"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

// ---------------------------------------------------------------------------
// Harness
// ---------------------------------------------------------------------------

// A database that knows about a handful of headings, for the link resolution -
// the one part of this exporter that cannot work without one.
type fakeDb struct {
	byId map[string]*common.Todo
}

func (self *fakeDb) QueryTodosExpr(query string) (common.Todos, error) { return nil, nil }
func (self *fakeDb) FindByAnyId(id string) *common.Todo                { return self.byId[id] }
func (self *fakeDb) FindByHash(hash string) *common.Todo               { return self.byId[hash] }
func (self *fakeDb) FindNextSibling(hash string) *common.Todo          { return nil }
func (self *fakeDb) FindPrevSibling(hash string) *common.Todo          { return nil }
func (self *fakeDb) FindLastChild(hash string) *common.Todo            { return nil }
func (self *fakeDb) FindByFile(filename string) *org.Document          { return nil }
func (self *fakeDb) GetFile(filename string) *common.OrgFile           { return nil }
func (self *fakeDb) GetFromTarget(t *common.Target, allowCreate bool) (*common.OrgFile, *org.Section) {
	return nil, nil
}
func (self *fakeDb) GetFromPreciseTarget(t *common.PreciseTarget, typeId org.NodeType) (*common.OrgFile, *org.Section, org.Node) {
	return nil, nil, nil
}

// Export a string of org, with whatever configuration the test is about.
func md(t *testing.T, src string, conf *OrgMdExporter) string {
	t.Helper()
	return mdWithDb(t, src, conf, nil)
}

func mdWithDb(t *testing.T, src string, conf *OrgMdExporter, db common.ODb) string {
	t.Helper()
	if conf == nil {
		conf = NewMarkdownExp()
	}
	d := org.New().Silent().Parse(strings.NewReader(src), "notes.org")
	if d.Error != nil {
		t.Fatalf("the org would not parse: %v", d.Error)
	}
	w := NewOrgMdWriter(conf, db, conf.settingsFor(d, map[string]string{}))
	w.Document = d
	w.writeFrontMatter(d)
	org.WriteNodes(w, d.Nodes...)
	w.writeFootnotes()
	return tidy(w.String())
}

func has(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Errorf("expected to find\n  %q\nin\n%s", want, got)
	}
}

func hasNot(t *testing.T, got, want string) {
	t.Helper()
	if strings.Contains(got, want) {
		t.Errorf("did not expect to find\n  %q\nin\n%s", want, got)
	}
}

func eq(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func withFlavor(f string) *OrgMdExporter {
	e := NewMarkdownExp()
	e.Flavor = f
	return e
}

// ---------------------------------------------------------------------------
// Headlines
// ---------------------------------------------------------------------------

func TestHeadlineLevels(t *testing.T) {
	out := md(t, "* One\n** Two\n*** Three\n", nil)
	has(t, out, "# One")
	has(t, out, "## Two")
	has(t, out, "### Three")
}

// Markdown stops at six. `#######` renders as literal hashes, so a deeper
// heading has to become something else or the document reads as line noise.
func TestHeadlinePastSixBecomesBold(t *testing.T) {
	out := md(t, "* 1\n** 2\n*** 3\n**** 4\n***** 5\n****** 6\n******* 7\n", nil)
	has(t, out, "###### 6")
	has(t, out, "###### **7**")
	hasNot(t, out, "#######")
}

func TestHeadlineKeywordPriorityAndTags(t *testing.T) {
	src := "#+TODO: TODO | DONE\n* TODO [#A] Fix the tap :home:plumbing:\n"
	has(t, md(t, src, nil), "# **TODO** `#A` Fix the tap  #home #plumbing")

	e := NewMarkdownExp()
	e.Keyword = "strip"
	e.Tags = "strip"
	eq(t, md(t, src, e), "# Fix the tap\n")

	e = NewMarkdownExp()
	e.Keyword = "tag"
	e.Tags = "code"
	has(t, md(t, src, e), "# #TODO `#A` Fix the tap  `home` `plumbing`")
}

// A file may override the exporter for itself.
func TestFileOverridesTheConfiguration(t *testing.T) {
	src := "#+MD_KEYWORD: strip\n#+MD_FRONTMATTER: none\n#+TODO: TODO | DONE\n* TODO Thing\n"
	eq(t, md(t, src, nil), "# Thing\n")
}

// ---------------------------------------------------------------------------
// Anchors
// ---------------------------------------------------------------------------

func TestSlugFollowsGithub(t *testing.T) {
	eq(t, slugOf("Hello, World!"), "hello-world")
	eq(t, slugOf("  Spaces   kept as one  "), "spaces---kept-as-one")
	eq(t, slugOf("Ünïcödé stays"), "ünïcödé-stays")
	eq(t, slugOf("C++ & Go"), "c--go")
}

// Two headings with the same text need different anchors, or every link to
// either of them goes to the first.
func TestRepeatedHeadingsGetDistinctAnchors(t *testing.T) {
	e := NewMarkdownExp()
	e.HeadingIds = true
	out := md(t, "* Notes\n* Notes\n* Notes\n", e)
	has(t, out, "{#notes}")
	has(t, out, "{#notes-1}")
	has(t, out, "{#notes-2}")
}

// ---------------------------------------------------------------------------
// Emphasis
// ---------------------------------------------------------------------------

func TestEmphasis(t *testing.T) {
	out := md(t, "/italic/ *bold* +gone+ ~code~ =verbatim=\n", nil)
	has(t, out, "*italic*")
	has(t, out, "**bold**")
	has(t, out, "~~gone~~")
	has(t, out, "`code`")
	has(t, out, "`verbatim`")
}

// CommonMark has no strikethrough, so it falls through to html - which
// CommonMark does pass through.
func TestStrikethroughFallsBackUnderCommonMark(t *testing.T) {
	out := md(t, "+gone+\n", withFlavor(flavorCommonMark))
	has(t, out, "<del>gone</del>")
}

// Code containing a backtick needs a longer fence, and one starting or ending
// with a backtick needs padding spaces, or markdown reads it as empty.
func TestInlineCodeWithBackticks(t *testing.T) {
	eq(t, inlineCode("plain"), "`plain`")
	eq(t, inlineCode("a ` b"), "``a ` b``")
	eq(t, inlineCode("`x`"), "`` `x` ``")
	eq(t, inlineCode("``"), "``` `` ```")
}

// Verbatim means verbatim: what is inside must not be escaped on its way out.
func TestCodeContentIsNotEscaped(t *testing.T) {
	out := md(t, "The value is =a_b*c= here\n", nil)
	has(t, out, "`a_b*c`")
	hasNot(t, out, `\_`)
}

// ---------------------------------------------------------------------------
// Escaping
// ---------------------------------------------------------------------------

// Escaping every punctuation mark is safe and makes the source unreadable,
// which is the one thing markdown is for. Each of these is about a character in
// the position where it actually does something.
func TestEscapingIsNarrow(t *testing.T) {
	// Intra-word underscores are what machine-written markdown usually ruins.
	out := md(t, "The file is my_long_name.txt today\n", nil)
	has(t, out, "my_long_name.txt")
	hasNot(t, out, `\_`)

	// A word-edge underscore would open emphasis.
	has(t, md(t, "a _b c\n", nil), `\_b`)

	// An asterisk anywhere would.
	has(t, md(t, "five * three\n", nil), `\*`)
}

func TestLineStartConstructsAreDefused(t *testing.T) {
	// A paragraph that happens to begin with a hash is not a heading.
	has(t, md(t, "#+TITLE: T\n#+MD_FRONTMATTER: none\n\\# not a heading\n", nil), `\#`)
	// Nor is a line beginning with a dash a bullet.
	out := md(t, "* H\n\n#+BEGIN_EXAMPLE\nkept\n#+END_EXAMPLE\n", nil)
	has(t, out, "kept")
}

// A pipe ends a cell, and only inside a table. Escaping it everywhere would put
// backslashes through every bit of prose that mentions one.
func TestPipesEscapedOnlyInTables(t *testing.T) {
	has(t, md(t, "a | b\n", nil), "a | b")
	hasNot(t, md(t, "a | b\n", nil), `\|`)
	has(t, md(t, "| a \\vert b | c |\n", nil), "|")
}

// ---------------------------------------------------------------------------
// Lists
// ---------------------------------------------------------------------------

func TestLists(t *testing.T) {
	out := md(t, "- one\n- two\n", nil)
	has(t, out, "- one\n- two")

	out = md(t, "1. first\n2. second\n", nil)
	has(t, out, "1. first")
}

func TestTaskLists(t *testing.T) {
	out := md(t, "- [ ] todo\n- [X] done\n- [-] partly\n", nil)
	has(t, out, "- [ ] todo")
	has(t, out, "- [x] done")
	// Markdown has no third state; an unfinished box is the honest reading.
	has(t, out, "- [ ] partly")
}

func TestNestedListsIndent(t *testing.T) {
	out := md(t, "- outer\n  - inner\n    - deepest\n", nil)
	has(t, out, "- outer")
	has(t, out, "  - inner")
	has(t, out, "    - deepest")
}

// An item's text is a paragraph and a paragraph ends with a blank line, so a
// nested list arrives one line below its parent - which markdown reads as a
// loose list and draws with a line of air between every item.
func TestNestedListsStayTight(t *testing.T) {
	out := md(t, "- [X] screws\n  - [ ] nested\n- [ ] paint\n", nil)
	has(t, out, "- [x] screws\n  - [ ] nested\n- [ ] paint")
}

func TestDescriptiveList(t *testing.T) {
	out := md(t, "- term :: what it means\n", nil)
	has(t, out, "- **term** — what it means")
}

// ---------------------------------------------------------------------------
// Tables
// ---------------------------------------------------------------------------

// How many lines of a table are a rule - a row of nothing but dashes, colons
// and pipes. Counting the string "| ---" would count an aligned rule twice.
func ruleLines(out string) int {
	n := 0
	for _, l := range strings.Split(out, "\n") {
		if l == "" || !strings.HasPrefix(l, "|") {
			continue
		}
		if strings.Trim(l, "|-: ") == "" {
			n++
		}
	}
	return n
}

func TestTableWithHeader(t *testing.T) {
	src := "| Name | Qty |\n|------+-----|\n| nail | 12  |\n| screw | 3  |\n"
	out := md(t, src, nil)
	// Padded out to the widest cell, because the whole point of markdown is
	// that the source is readable.
	has(t, out, "| Name  | Qty  |")
	has(t, out, "| nail  | 12   |")
	has(t, out, "| screw | 3    |")
	// The rule is the second line and there is exactly one of them.
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 2 || strings.Trim(lines[1], "|-: ") != "" {
		t.Errorf("the rule has to be the second line:\n%s", out)
	}
	if n := ruleLines(out); n != 1 {
		t.Errorf("expected one header rule, got %d:\n%s", n, out)
	}
}

// go-org reads a column of numbers as right aligned even where the file did not
// say so, which is what org itself does when it draws one.
func TestNumericColumnsComeBackRightAligned(t *testing.T) {
	out := md(t, "| Name | Qty |\n|------+-----|\n| nail | 12 |\n", nil)
	has(t, out, "---:")
}

// Markdown insists on a header row. Promoting the first data row instead is the
// obvious alternative and turns data into column headings.
func TestTableWithNoRuleGetsAnEmptyHeader(t *testing.T) {
	out := md(t, "| a | 1 |\n| b | 2 |\n", nil)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 4 {
		t.Fatalf("expected an empty header, a rule and two rows:\n%s", out)
	}
	has(t, out, "| a   | 1")
	has(t, out, "| b   | 2")
	if strings.Contains(lines[0], "a") {
		t.Errorf("the first data row must not become the header:\n%s", out)
	}
}

// An org table can have a rule anywhere and as many as it likes; markdown has
// room for exactly one and it has to be the second line.
func TestExtraTableRulesAreDropped(t *testing.T) {
	src := "| a |\n|---|\n| b |\n|---|\n| c |\n"
	out := md(t, src, nil)
	if n := ruleLines(out); n != 1 {
		t.Errorf("expected one rule, got %d:\n%s", n, out)
	}
	has(t, out, "| b   |")
	has(t, out, "| c   |")
}

func TestTableAlignment(t *testing.T) {
	src := "| <l> | <c> | <r> |\n| a | b | c |\n|---+---+---|\n| 1 | 2 | 3 |\n"
	out := md(t, src, nil)
	has(t, out, ":--")
	has(t, out, "--:")
}

func TestCommonMarkHasNoTables(t *testing.T) {
	out := md(t, "| a |\n|---|\n| b |\n", withFlavor(flavorCommonMark))
	has(t, out, "<table>")
}

// ---------------------------------------------------------------------------
// Blocks
// ---------------------------------------------------------------------------

func TestSourceBlockIsFenced(t *testing.T) {
	out := md(t, "#+BEGIN_SRC python\nprint(1)\n#+END_SRC\n", nil)
	has(t, out, "```python\nprint(1)\n```")
}

// A block quoting markdown contains backticks of its own, and three would end
// the fence early - leaving the rest of the document inside a code block.
func TestFenceGrowsPastItsContent(t *testing.T) {
	eq(t, fenceFor("plain"), "```")
	eq(t, fenceFor("a ``` b"), "````")
	eq(t, fenceFor("````"), "`````")
	out := md(t, "#+BEGIN_SRC md\n```go\nx\n```\n#+END_SRC\n", nil)
	has(t, out, "````md")
}

func TestQuoteBlock(t *testing.T) {
	out := md(t, "#+BEGIN_QUOTE\nfirst line\n\nsecond para\n#+END_QUOTE\n", nil)
	has(t, out, "> first line")
	has(t, out, "> second para")
	// The blank line between them has to be quoted too, or it ends the quote.
	has(t, out, "\n>\n")
}

// Verse is the one block whose line breaks are the content.
func TestVerseKeepsItsLineBreaks(t *testing.T) {
	out := md(t, "#+BEGIN_VERSE\none\ntwo\n#+END_VERSE\n", nil)
	has(t, out, "one  \ntwo")
}

func TestExampleBlockIsFencedAndLiteral(t *testing.T) {
	out := md(t, "#+BEGIN_EXAMPLE\na_b *c* [d]\n#+END_EXAMPLE\n", nil)
	has(t, out, "```\na_b *c* [d]\n```")
}

// An export block names who it is for. Markdown takes its own and html's - html
// because markdown carries html - and leaves the rest to whoever they are for.
func TestExportBlocksAreTakenBySpeciality(t *testing.T) {
	has(t, md(t, "#+BEGIN_EXPORT md\n**mine**\n#+END_EXPORT\n", nil), "**mine**")
	has(t, md(t, "#+BEGIN_EXPORT html\n<b>ok</b>\n#+END_EXPORT\n", nil), "<b>ok</b>")
	hasNot(t, md(t, "#+BEGIN_EXPORT latex\n\\textbf{no}\n#+END_EXPORT\n", nil), "textbf")
}

func TestCenterBlockUsesHtml(t *testing.T) {
	out := md(t, "#+BEGIN_CENTER\nmiddle\n#+END_CENTER\n", nil)
	has(t, out, `<div align="center">`)
	has(t, out, "middle")
}

// ---------------------------------------------------------------------------
// Links
// ---------------------------------------------------------------------------

func TestExternalLink(t *testing.T) {
	has(t, md(t, "See [[https://example.com][the site]].\n", nil), "[the site](https://example.com)")
}

func TestBareUrlIsAnAutolink(t *testing.T) {
	has(t, md(t, "See [[https://example.com]].\n", nil), "https://example.com")
}

func TestImageLink(t *testing.T) {
	has(t, md(t, "[[file:shed.png]]\n", nil), "![shed.png](shed.png)")
	has(t, md(t, "[[file:shed.png][a shed]]\n", nil), "![a shed](shed.png)")
}

// Exporting a tree of org files produces a tree of markdown files, so a link to
// a sibling has to point at what that sibling will be called.
func TestOrgLinksBecomeMdLinks(t *testing.T) {
	has(t, md(t, "[[file:other.org][other]]\n", nil), "[other](other.md)")

	e := NewMarkdownExp()
	no := false
	e.OrgLinksToMd = &no
	has(t, md(t, "[[file:other.org][other]]\n", e), "[other](other.org)")
}

func TestHeadingLinksBecomeAnchors(t *testing.T) {
	has(t, md(t, "* Plans\n[[*Plans][back]]\n", nil), "[back](#plans)")
	has(t, md(t, "[[file:other.org::*The Plan][x]]\n", nil), "[x](other.md#the-plan)")
}

// The one thing only a server holding the whole database can do: an id: link
// names a heading by a uuid written in some other file.
func TestIdLinksAreResolvedAgainstTheDatabase(t *testing.T) {
	db := &fakeDb{byId: map[string]*common.Todo{
		"abc-123": {Headline: "The Kitchen Plan", Filename: "projects.org"},
		"here-1":  {Headline: "Local Heading", Filename: "notes.org"},
	}}
	out := mdWithDb(t, "[[id:abc-123][see the plan]]\n", nil, db)
	has(t, out, "[see the plan](projects.md#the-kitchen-plan)")

	// In this document it is a bare anchor rather than a link to the file.
	out = mdWithDb(t, "[[id:here-1][here]]\n", nil, db)
	has(t, out, "[here](#local-heading)")
}

// Inventing an anchor for an id nothing knows about would be a link to nothing
// that looks like a link to something.
func TestUnknownIdIsNotInvented(t *testing.T) {
	db := &fakeDb{byId: map[string]*common.Todo{}}
	out := mdWithDb(t, "[[id:missing][x]]\n", nil, db)
	has(t, out, "[x](#missing)")
}

func TestObsidianWikiLinks(t *testing.T) {
	e := withFlavor(flavorObsidian)
	has(t, md(t, "[[file:other.org][other]]\n", e), "[[other|other]]")
	has(t, md(t, "[[file:shed.png]]\n", e), "![[shed.png]]")
	// A url is a url in every dialect.
	has(t, md(t, "[[https://example.com][site]]\n", e), "[site](https://example.com)")
}

// A space inside the round brackets ends the url early, and markdown's own way
// of saying "all of this is the address" is angle brackets.
func TestUrlsWithSpacesAreWrapped(t *testing.T) {
	has(t, md(t, "[[file:my notes.pdf][notes]]\n", nil), "[notes](<my notes.pdf>)")
}

func TestLinkAbbreviationsAreExpanded(t *testing.T) {
	src := "#+LINK: gh https://github.com/%s\n[[gh:ihdavids/orgs][orgs]]\n"
	has(t, md(t, src, nil), "[orgs](https://github.com/ihdavids/orgs)")
}

// ---------------------------------------------------------------------------
// Footnotes
// ---------------------------------------------------------------------------

func TestFootnotes(t *testing.T) {
	src := "Text[fn:1] and more[fn:2].\n\n[fn:1] first note\n[fn:2] second note\n"
	out := md(t, src, nil)
	has(t, out, "[^1]")
	has(t, out, "[^2]")
	has(t, out, "[^1]: first note")
	has(t, out, "[^2]: second note")
}

// Org lets a footnote be defined anywhere; markdown gathers them at the end, so
// the numbering has to follow the order they are *read* in.
func TestFootnotesAreNumberedByReference(t *testing.T) {
	src := "[fn:zebra] defined first\n\nText[fn:apple] then[fn:zebra].\n\n[fn:apple] apple note\n"
	out := md(t, src, nil)
	i1 := strings.Index(out, "[^1]:")
	i2 := strings.Index(out, "[^2]:")
	if i1 < 0 || i2 < 0 || i1 > i2 {
		t.Errorf("footnote definitions out of order:\n%s", out)
	}
}

// A reference with no definition still gets a line, or it is a link to nothing.
func TestUndefinedFootnoteStillLands(t *testing.T) {
	out := md(t, "Text[fn:nope].\n", nil)
	has(t, out, "[^1]")
	has(t, out, "[^1]: *(no definition)*")
}

func TestCommonMarkFootnotesFallBackToHtml(t *testing.T) {
	src := "Text[fn:1].\n\n[fn:1] a note\n"
	out := md(t, src, withFlavor(flavorCommonMark))
	has(t, out, `href="#fn-1"`)
	has(t, out, `id="fn-1"`)
	hasNot(t, out, "[^1]")
}

// ---------------------------------------------------------------------------
// Front matter
// ---------------------------------------------------------------------------

func TestYamlFrontMatter(t *testing.T) {
	src := "#+TITLE: Kitchen rebuild\n#+AUTHOR: Ian\n#+FILETAGS: :home:diy:\n* Body\n"
	out := md(t, src, nil)
	if !strings.HasPrefix(out, "---\n") {
		t.Errorf("front matter has to be first:\n%s", out)
	}
	has(t, out, "title: Kitchen rebuild")
	has(t, out, "author: Ian")
	has(t, out, "tags: [home, diy]")
}

// A title of "1.0" read back as a float is a title nobody can search for.
func TestFrontMatterQuotesWhatWouldBeMisread(t *testing.T) {
	has(t, md(t, "#+TITLE: 1.0\n", nil), `title: "1.0"`)
	has(t, md(t, "#+TITLE: true\n", nil), `title: "true"`)
	has(t, md(t, "#+TITLE: Notes: a sequel\n", nil), `title: "Notes: a sequel"`)
	has(t, md(t, "#+TITLE: Plain words\n", nil), "title: Plain words\n")
}

func TestMdKeywordsBecomeFrontMatter(t *testing.T) {
	src := "#+TITLE: T\n#+MD_DRAFT: false\n#+MD_WEIGHT: 20\n"
	out := md(t, src, nil)
	has(t, out, `draft: "false"`)
	has(t, out, `weight: "20"`)
}

// The MD_ keywords that configure the exporter describe the export, not the
// document, and must not end up in its front matter.
func TestMdSettingsAreNotFrontMatter(t *testing.T) {
	out := md(t, "#+TITLE: T\n#+MD_FLAVOR: obsidian\n#+MD_TAGS: strip\n", nil)
	hasNot(t, out, "flavor:")
	hasNot(t, out, "tags: strip")
}

func TestTomlFrontMatter(t *testing.T) {
	e := NewMarkdownExp()
	e.FrontMatter = "toml"
	out := md(t, "#+TITLE: T\n#+FILETAGS: :a:\n", e)
	has(t, out, "+++\n")
	has(t, out, `title = "T"`)
	has(t, out, `tags = ["a"]`)
}

func TestNoFrontMatter(t *testing.T) {
	e := NewMarkdownExp()
	e.FrontMatter = "none"
	eq(t, md(t, "#+TITLE: T\n* H\n", e), "# H\n")
}

// BufferSettings is a map, and an export that reordered its own front matter
// between two runs would show up as a diff in every file every time.
func TestFrontMatterOrderIsStable(t *testing.T) {
	src := "#+TITLE: T\n#+MD_ZULU: z\n#+MD_ALPHA: a\n#+MD_MIKE: m\n"
	first := md(t, src, nil)
	for i := 0; i < 8; i++ {
		if again := md(t, src, nil); again != first {
			t.Fatalf("front matter is not stable:\n%s\nvs\n%s", first, again)
		}
	}
	ia := strings.Index(first, "alpha:")
	im := strings.Index(first, "mike:")
	iz := strings.Index(first, "zulu:")
	if !(ia < im && im < iz) {
		t.Errorf("expected the named keys sorted:\n%s", first)
	}
}

// ---------------------------------------------------------------------------
// Drawers and planning
// ---------------------------------------------------------------------------

func TestDrawersAreStrippedByDefault(t *testing.T) {
	src := "* H\n:PROPERTIES:\n:ID: abc\n:END:\n\nbody\n"
	out := md(t, src, nil)
	hasNot(t, out, "PROPERTIES")
	hasNot(t, out, "abc")
	has(t, out, "body")
}

func TestDrawersAsAList(t *testing.T) {
	e := NewMarkdownExp()
	e.Drawers = "list"
	out := md(t, "* H\n:PROPERTIES:\n:ID: abc\n:OWNER: Ian\n:END:\n", e)
	has(t, out, "- **ID:** abc")
	has(t, out, "- **OWNER:** Ian")
}

func TestDrawersKept(t *testing.T) {
	e := NewMarkdownExp()
	e.Drawers = "keep"
	out := md(t, "* H\n:PROPERTIES:\n:ID: abc\n:END:\n", e)
	has(t, out, ":PROPERTIES:")
	has(t, out, ":ID: abc")
}

// A LOGBOOK is org's record of what happened rather than part of the document,
// so it goes even when the other drawers are kept.
func TestLogbookIsAlwaysDropped(t *testing.T) {
	src := "* H\n:LOGBOOK:\nCLOCK: [2026-09-01 Tue 09:00]--[2026-09-01 Tue 10:00] =>  1:00\n:END:\n\nbody\n"
	for _, mode := range []string{"strip", "list", "keep"} {
		e := NewMarkdownExp()
		e.Drawers = mode
		out := md(t, src, e)
		hasNot(t, out, "LOGBOOK")
		hasNot(t, out, "CLOCK")
		has(t, out, "body")
	}
}

func TestPlanningLines(t *testing.T) {
	// On separate lines, deliberately: go-org parses only one planning keyword
	// per line, so the canonical org form - both on one line - silently loses
	// the second. That is a parser bug rather than this exporter's, and it is
	// not this change's to fix, but a test written the canonical way here would
	// be testing the bug rather than the exporter.
	src := "#+TODO: TODO | DONE\n* TODO H\nSCHEDULED: <2026-09-28 Mon>\nDEADLINE: <2026-10-05 Mon>\n"
	hasNot(t, md(t, src, nil), "Scheduled")

	e := NewMarkdownExp()
	e.Planning = "list"
	out := md(t, src, e)
	has(t, out, "**Scheduled:**")
	has(t, out, "**Deadline:**")
	has(t, out, "2026-09-28")
}

// The planning line arrives both on the headline and as a child of it, so the
// obvious implementation writes it twice.
func TestPlanningIsNotWrittenTwice(t *testing.T) {
	e := NewMarkdownExp()
	e.Planning = "list"
	src := "#+TODO: TODO | DONE\n* TODO H\nSCHEDULED: <2026-09-28 Mon>\n"
	if n := strings.Count(md(t, src, e), "2026-09-28"); n != 1 {
		t.Errorf("the schedule was written %d times:\n%s", n, md(t, src, e))
	}
}

// ---------------------------------------------------------------------------
// Whole documents
// ---------------------------------------------------------------------------

func TestTidyLeavesNoRunsOfBlankLines(t *testing.T) {
	src := "* A\n\n\n\nbody\n\n\n\n** B\n\n\n\nmore\n"
	out := md(t, src, nil)
	hasNot(t, out, "\n\n\n")
	if !strings.HasSuffix(out, "\n") || strings.HasSuffix(out, "\n\n") {
		t.Errorf("a document should end in exactly one newline:\n%q", out)
	}
}

// Two trailing spaces are markdown's hard line break and have to survive the
// tidying that strips every other kind of trailing whitespace.
func TestHardBreaksSurviveTidying(t *testing.T) {
	eq(t, tidy("a  \nb   \nc\t\n"), "a  \nb\nc\n")
}

func TestKeywordsThatAreNotOurBusinessAreDropped(t *testing.T) {
	src := "#+STARTUP: overview\n#+PROPERTY: header-args :results none\n#+TBLFM: $3=$1*$2\n* H\n"
	out := md(t, src, nil)
	hasNot(t, out, "STARTUP")
	hasNot(t, out, "PROPERTY")
	hasNot(t, out, "TBLFM")
}

func TestHtmlAndMdKeywordsArePassedThrough(t *testing.T) {
	has(t, md(t, "#+HTML: <hr/>\n", nil), "<hr/>")
	has(t, md(t, "#+MD: **raw**\n", nil), "**raw**")
}

func TestCaptionBecomesAnEmphasisedLine(t *testing.T) {
	src := "#+CAPTION: The shed\n[[file:shed.png]]\n"
	out := md(t, src, nil)
	has(t, out, "![shed.png](shed.png)")
	has(t, out, "*The shed*")
}

func TestHorizontalRule(t *testing.T) {
	has(t, md(t, "one\n\n-----\n\ntwo\n", nil), "---")
}

// The whole thing end to end, so that the joins between the pieces are pinned
// as well as the pieces.
func TestAWholeDocument(t *testing.T) {
	src := `#+TITLE: Kitchen rebuild
#+AUTHOR: Ian
#+FILETAGS: :home:
#+TODO: TODO NEXT | DONE

Some opening prose with /emphasis/ and a [[https://example.com][link]].

* TODO Buy materials :shopping:
:PROPERTIES:
:ID: buy-1
:END:

| Item  | Qty |
|-------+-----|
| nail  | 12  |
| screw | 3   |

** DONE Measure the floor
Done it.

#+BEGIN_SRC sh
echo hello
#+END_SRC
`
	out := md(t, src, nil)
	want := `---
title: Kitchen rebuild
author: Ian
tags: [home]
---

Some opening prose with *emphasis* and a [link](https://example.com).

# **TODO** Buy materials  #shopping

| Item  | Qty  |
| ----- | ---: |
| nail  | 12   |
| screw | 3    |

## **DONE** Measure the floor

Done it.

` + "```sh\necho hello\n```" + `
`
	eq(t, out, want)
}
