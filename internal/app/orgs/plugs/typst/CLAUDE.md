# Notes on the typst exporter (`plugs/typst`)

Loaded when working in this directory. The SDOC at the top of `typst.go` is the user guide.

`orgs export -f typst` writes a `.typ`; `orgs export -f typst-pdf` asks `GET /typst/pdf` (`internal/app/orgs/typstpdf.go`), which compiles with the `typst` binary in a scratch folder (pictures copied in as `img/N-name`) and caches the pdf against the file's state and the settings. `Standalone`: no yaml needed. Templates are `templates/typst/<name>.typ` (article, report, notes), found with `assets.FindTemplate`, so a user's folder adds one; `GET /typst/templates` lists them.

A document is: header comment, the mitex import (only when a formula went to mitex), `prelude` (the `org-*` helpers), the template's text (inlined, so the `.typ` stands alone), `#+TYPST_HEADER` lines, `#show: org-template.with(...)`, the body. A template must accept every argument `showRule` passes (title, title-text, subtitle, authors, date, email, lang, paper, description, keywords, toc, toc-depth, numbered, num-depth, columns) or typst refuses the document; it may redefine any `org-*` helper.

Rules (each was a bug found by compiling):

1. **The writer implements `org.Writer` itself**, not on top of go-org's html writer as markdown does: anything falling through to html would print tags in the pdf.
2. **Escaping is `escape.go` only.** Markup: `\ # * _ ` $ < > @ [ ] ~` always, `/` before `/` or `*` (comments), and `= - + / N.` at the start of a line - where "start of a line" includes just after the `[` that opens a content block (`atLineStart`). Strings: `str`. Raw: `fence`, longer than any backtick run inside.
3. **An embedded expression swallows a following `.field` or `(args)`**: `#strong[b].c` is a field access and an error. `text` escapes a leading `.` or `(` after an expression (`afterExpr`); keep writing expressions through `expr`.
4. **Lists, enums and terms are calls** (`#list([..], [..])`), not `- item` markup, so items holding paragraphs, code or lists need no indentation bookkeeping.
5. **Labels must be unique** or every link to them errors: `labelFor` dedupes (`sec-x`, `sec-x-2`). Links to headings are `#link(<label>)[...]`, never `@label` - a reference to an unnumbered heading is an error.
6. **`go-org`'s `GetOption` answers "nil" for an option nobody set** (and logs it), which turned numbering and the contents off for every file. `option()` reads `#+OPTIONS:`/the configured default itself and falls back to org's own defaults (`optionDefaults`).
7. **Captions**: go-org drops `#+CAPTION:` when an unknown keyword (`#+ATTR_TYPST:`, `#+NAME:` after it) follows; the writer keeps a `CAPTION` keyword it is handed as `pendingCaption` for the next picture, table or block, and `ATTR_TYPST` likewise as `pendingAttrs`.
8. **Tags in a heading** are `h(1fr)` and a box, both labelled `<org-tags>`, which the prelude's `show outline` hides: a `1fr`-wide box overlapped a long title, and an unlabelled spacer left gaps in the contents.
9. **Header cells are `org-th[...]` and the header rule a `table.hline()` inside `table.header`**: styling "row 0" in a template bolded the first row of tables that have no header.
10. Fonts are typst's bundled ones (Libertinus Serif, New Computer Modern, DejaVu Sans Mono): a family missing on the machine is a warning on every compile.
11. A template's argument named `columns` shadows the `columns` function: use `std.columns` inside it.

Known go-org quirks seen through this exporter (not fixed here): `$5 and $10` parses as a LaTeX fragment (org would not, the closing `$` being followed by a digit); `// x /* y */` reads as italics.

Tests: `go test ./internal/app/orgs/plugs/typst/` - substring checks over `testdata/sample.org`, escaping tables, the math translation, and `TestCompile`, which compiles the sample with every template plus `testdata/hostile.org` when `typst` is installed (`TYPST_OUT=dir` keeps the pdfs and `.typ`s to look at).
