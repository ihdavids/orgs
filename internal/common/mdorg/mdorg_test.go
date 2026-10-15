package mdorg

import (
	"strings"
	"testing"
)

func vault() Vault {
	return Vault{
		Notes: []Note{
			{Path: "Home.md", Text: `---
title: My Home
tags: [index, life/planning]
aliases:
  - Start
date: 2026-10-01
status: draft
---
# Ignored because front matter named the title

Welcome to the **vault**. See [[Projects/Garden|the garden]] and [[Recipes#Soups]].
Also [plain md link](Projects/Garden.md#Tomato-plans) and <https://example.com>.
Inline #idea tag, ` + "`code with *stars*`" + ` and $e=mc^2$ and ==highlight== and ~~gone~~.
A_snake_case_word stays, *this* and _that_ are italic. Footnote[^1].

[^1]: The footnote text.

![[photo.png|300]]

![[Recipes]]

> [!warning] Careful
> Hot **pan**.

> Just a quote
> - with a list

| Name | Qty |
| :--- | ---: |
| Eggs \| large | 2 |

- [ ] Water plants 📅 2026-10-20
- [x] Buy seeds ✅ 2026-10-02
	- nested item

` + "```python" + `
# not a heading
* not a heading either
print("hi")
` + "```" + `

Some text ^block1

%% hidden
comment %%
`},
			{Path: "Projects/Garden.md", Text: "# Garden\n\n## Tomato plans\n\nSee [[Home]] and [[Home#^block1|that bit]] and [[Missing note]].\n\n![](../attachments/plan.jpg)\n"},
			{Path: "Recipes.md", Text: "## Soups\n\nWarm.\n\nSetext title\n============\n"},
		},
		Files: []string{"attachments/photo.png", "attachments/plan.jpg", "attachments/unused.pdf"},
	}
}

func TestVault(t *testing.T) {
	out := Convert(vault())
	home := out.Files["Home.org"]
	for _, want := range []string{
		":PROPERTIES:\n:ROAM_ALIASES: \"Start\"\n:STATUS: draft\n:END:\n#+TITLE: My Home\n#+SUBTITLE: Ignored because front matter named the title\n#+DATE: [2026-10-01 Thu]\n#+FILETAGS: :idea:index:life_planning:\n",
		"Welcome to the *vault*. See [[file:./Projects/Garden.org][the garden]] and [[file:./Recipes.org::*Soups][Recipes > Soups]].",
		"[[file:./Projects/Garden.org::*Tomato plans][plain md link]] and [[https://example.com]].",
		"~code with *stars*~ and \\(e=mc^2\\) and *highlight* and +gone+.",
		"A_snake_case_word stays, /this/ and /that/ are italic. Footnote[fn:1].",
		"[fn:1] The footnote text.",
		"#+attr_html: :width 300px\n[[file:./attachments/photo.png]]",
		`#+INCLUDE: "./Recipes.org"`,
		"#+begin_warning\n*Careful*\nHot *pan*.\n#+end_warning",
		"#+begin_quote\nJust a quote\n- with a list\n#+end_quote",
		"| Name | Qty |\n|---+---|\n| Eggs \\vert{} large | 2 |",
		"- [ ] Water plants due <2026-10-20 Tue>\n- [X] Buy seeds done [2026-10-02 Fri]\n  - nested item",
		"#+begin_src python\n# not a heading\n,* not a heading either\nprint(\"hi\")\n#+end_src",
		"Some text <<block1>>",
		"#+begin_comment\nhidden\ncomment\n#+end_comment",
	} {
		if !strings.Contains(home, want) {
			t.Errorf("Home.org missing\n%s\n---- in ----\n%s", want, home)
		}
	}
	garden := out.Files["Projects/Garden.org"]
	for _, want := range []string{
		"#+TITLE: Garden\n",
		"\n* Tomato plans",
		"[[file:../Home.org][Home]] and [[file:../Home.org::block1][that bit]] and [[file:./Missing note.org][Missing note]]",
		"[[file:../attachments/plan.jpg]]",
	} {
		if !strings.Contains(garden, want) {
			t.Errorf("Garden.org missing\n%s\n---- in ----\n%s", want, garden)
		}
	}
	rec := out.Files["Recipes.org"]
	if !strings.Contains(rec, "#+TITLE: Recipes\n") || !strings.Contains(rec, "\n** Soups") || !strings.Contains(rec, "* Setext title") {
		t.Errorf("Recipes.org:\n%s", rec)
	}
	if strings.Join(out.Assets, ",") != "attachments/photo.png,attachments/plan.jpg" {
		t.Errorf("assets %v", out.Assets)
	}
	if out.Stats.Unresolved != 1 || out.Stats.Notes != 3 || out.Stats.Tasks != 2 || out.Stats.Callouts != 1 {
		t.Errorf("stats %+v", out.Stats)
	}
}

func TestSingleNoteHeadingsMoveUp(t *testing.T) {
	got := ConvertText("note.md", "# Title\n\n## One\n\ntext\n\n### Two\n")
	want := "#+TITLE: Title\n\n* One\n\ntext\n\n** Two\n"
	if got != want {
		t.Errorf("got\n%q\nwant\n%q", got, want)
	}
}

func TestHorizontalRuleAndSetext(t *testing.T) {
	got := ConvertText("n.md", "para\n\n---\n\nHeading\n---\n")
	if !strings.Contains(got, "para\n\n-----\n") || !strings.Contains(got, "\n* Heading") {
		t.Errorf("%q", got)
	}
}

func TestShallowestHeadingIsTopLevel(t *testing.T) {
	got := ConvertText("n.md", "## Beds\n\ntext\n\n### Rows\n")
	if !strings.Contains(got, "\n* Beds\n") || !strings.Contains(got, "\n** Rows\n") {
		t.Errorf("%q", got)
	}
}
