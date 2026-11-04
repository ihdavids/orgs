package orgs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const citeBib = `@article{doe2020, author = {Doe, Jane and Richard Roe}, title = {Typed Holes}, journal = {JFP}, year = 2020}
@book{who2019, author = {{World Health Organization}}, title = {Report}, publisher = {WHO}, year = 2019}
@misc{zed, author = {Zed, A.}, title = {Uncited}, year = 2001}
`

func citeFixture(t *testing.T, org string) (string, []string) {
	t.Helper()
	paths := loadOrg(t, map[string]string{"paper.org": org})
	p := paths["paper.org"]
	os.WriteFile(filepath.Join(filepath.Dir(p), "refs.bib"), []byte(citeBib), 0644)
	lines, _ := readFileLines(p)
	return p, lines
}

func TestBasicCitationsAndBibliography(t *testing.T) {
	p, lines := citeFixture(t, `#+bibliography: refs.bib
* Intro
As shown [cite:@doe2020], and by [cite/t:@who2019] - see [cite:see @doe2020 p. 3; @nobody].
#+BEGIN_EXAMPLE
[cite:@doe2020] stays as written here
#+END_EXAMPLE
* References
#+print_bibliography:
`)
	out, ok := citeExportLines(p, lines, "html")
	got := strings.Join(out, "\n")
	if !ok {
		t.Fatal("nothing changed")
	}
	for _, want := range []string{
		"As shown ([[cite-doe2020][Doe and Roe, 2020]]), and by [[cite-who2019][World Health Organization (2019)]]",
		"(see [[cite-doe2020][Doe and Roe, 2020]], p. 3; nobody)",
		"[cite:@doe2020] stays as written here",
		"- <<cite-doe2020>>Doe, Jane, and Richard Roe. 2020.",
		"- <<cite-who2019>>World Health Organization. 2019. /Report/. WHO.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("no %q in\n%s", want, got)
		}
	}
	if strings.Contains(got, "Uncited") || strings.Index(got, "<<cite-doe2020>>") > strings.Index(got, "<<cite-who2019>>") {
		t.Errorf("list: only what is cited, by author:\n%s", got)
	}
}

func TestNumericAndNocite(t *testing.T) {
	p, lines := citeFixture(t, "#+bibliography: refs.bib\n#+cite_export: basic numeric numeric\nOne [cite:@who2019], two [cite:@doe2020; @who2019].[cite/n:@*]\n#+print_bibliography:\n")
	out, _ := citeExportLines(p, lines, "")
	got := strings.Join(out, "\n")
	for _, want := range []string{"One [[[cite-who2019][1]]], two [[[cite-doe2020][2]], [[cite-who2019][1]]].", "1. <<cite-who2019>>", "2. <<cite-doe2020>>", "3. <<cite-zed>>"} {
		if !strings.Contains(got, want) {
			t.Errorf("no %q in\n%s", want, got)
		}
	}
}

func TestBiblatexForLatexOnly(t *testing.T) {
	p, lines := citeFixture(t, "#+bibliography: refs.bib\n#+cite_export: biblatex authoryear\nA [cite/t:@doe2020] and [cite:see @doe2020 p. 3; @who2019 ch. 2].\n#+print_bibliography:\n")
	out, _ := citeExportLines(p, lines, "latex")
	got := strings.Join(out, "\n")
	bib := filepath.Join(filepath.Dir(p), "refs.bib")
	for _, want := range []string{
		`#+LATEX_HEADER: \usepackage[backend=biber,style=authoryear]{biblatex}`,
		`#+LATEX_HEADER: \addbibresource{` + bib + `}`,
		`A @@latex:\textcite{doe2020}@@ and @@latex:\autocites[see][p. 3]{doe2020}[ch. 2]{who2019}@@.`,
		"#+BEGIN_EXPORT latex\n\\printbibliography\n#+END_EXPORT",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("no %q in\n%s", want, got)
		}
	}
	// html cannot run biber: the same file is basic there.
	out, _ = citeExportLines(p, lines, "html")
	if got := strings.Join(out, "\n"); !strings.Contains(got, "Doe and Roe (2020)") || strings.Contains(got, "@@latex") {
		t.Errorf("html:\n%s", got)
	}
}

func TestNatbibCommands(t *testing.T) {
	p, lines := citeFixture(t, "#+bibliography: refs.bib\n#+cite_export: natbib\n[cite:@doe2020 p. 3] [cite/a/c:@who2019] [cite/na:@doe2020]\n#+print_bibliography:\n")
	out, _ := citeExportLines(p, lines, "beamer")
	got := strings.Join(out, "\n")
	for _, want := range []string{`@@latex:\citep[p. 3]{doe2020}@@ @@latex:\Citeauthor{who2019}@@ @@latex:\citeyearpar{doe2020}@@`, `\bibliographystyle{plainnat}`, `\usepackage[round]{natbib}`} {
		if !strings.Contains(got, want) {
			t.Errorf("no %q in\n%s", want, got)
		}
	}
}

func TestCitationsReachTheHtmlExport(t *testing.T) {
	p, _ := citeFixture(t, "#+bibliography: refs.bib\n* Intro\nAs shown [cite:@doe2020].\n* References\n#+print_bibliography:\n")
	f, _ := exportFileFor(p, false, "html")
	if f == nil || f == GetDb().FindByFile(p) {
		t.Fatal("the export read the file as it is")
	}
	if !f.Doc.Targets["cite-doe2020"] {
		t.Errorf("no target for the entry: %v", f.Doc.Targets)
	}
}
