package typst

// prelude is the helpers the body calls for the parts of org typst has no word
// for: keywords, tags, timestamps, checkboxes, special blocks. Written before
// the template, so a template can redefine any of them to dress them its way.
const prelude = `// Helpers for the parts of org typst has no element for. A template may
// redefine any of them.
#let org-accent = rgb("#2f5d8a")
#let org-muted = luma(110)
#let org-todo(kw, done: false) = box(
  inset: (x: 0.3em, y: 0.12em), outset: (y: 0.1em), radius: 2pt,
  fill: if done { rgb("#e3f1e6") } else { rgb("#fbe6e4") },
  text(size: 0.68em, weight: "bold", tracking: 0.04em,
    fill: if done { rgb("#2e7d43") } else { rgb("#b3261e") }, kw),
)
#let org-priority(p) = box(
  inset: (x: 0.25em, y: 0.12em), outset: (y: 0.1em), radius: 2pt, stroke: 0.5pt + org-muted,
  text(size: 0.68em, weight: "bold", fill: org-muted, "#" + p),
)
// Tags sit at the right of the heading; the label keeps them out of the
// contents (the show rule on outline at the end of this block).
#let org-tags(tags) = [#h(1fr)<org-tags>#box(tags.map(t =>
  box(inset: (x: 0.3em, y: 0.12em), outset: (y: 0.1em), radius: 2pt, fill: luma(240),
    text(size: 0.62em, weight: "regular", fill: org-muted, t))).join(h(0.2em)))<org-tags>]
// A header cell of a table: the rows above an org table's first rule.
#let org-th(body) = strong(body)
#let org-timestamp(s) = text(size: 0.9em, fill: org-muted, s)
#let org-planning(kind, stamp) = block(above: 0.4em, below: 0.8em,
  text(size: 0.8em, fill: org-muted)[#smallcaps(lower(kind)) #h(0.3em) #stamp])
#let org-properties(..cells) = block(above: 0.4em, below: 0.8em,
  text(size: 0.8em, fill: org-muted,
    grid(columns: 2, column-gutter: 1em, row-gutter: 0.4em, ..cells.pos())))
#let org-stat(s) = box(inset: (x: 0.25em), radius: 2pt, fill: luma(240), text(size: 0.8em, fill: org-muted, s))
#let org-check(state) = box(
  width: 0.72em, height: 0.72em, baseline: 0.05em, radius: 1.5pt,
  stroke: 0.6pt + org-muted,
  fill: if state == true { org-muted } else { none },
  if state == none { align(center + horizon, box(width: 0.4em, height: 0.08em, fill: org-muted)) }
  else if state == true { align(center + horizon, text(size: 0.6em, fill: white, weight: "bold", sym.checkmark)) },
)
// Fixed width text and example blocks: raw text, dressed by the template's own
// show rule for raw blocks.
#let org-example(src) = raw(src, block: true)
#let org-verse(body) = block(inset: (left: 1.5em), body)
#let org-special(name, body) = block(
  width: 100%, inset: (x: 0.9em, y: 0.7em), radius: 3pt,
  fill: org-accent.lighten(92%), stroke: (left: 2pt + org-accent),
  {
    set par(first-line-indent: 0pt)
    text(size: 0.72em, weight: "bold", tracking: 0.06em, fill: org-accent, upper(name))
    v(-0.3em)
    body
  },
)
#let org-hr() = align(center, line(length: 30%, stroke: 0.5pt + org-muted))
#show outline: it => { show <org-tags>: none; it }
`

// fallbackTemplate is used when no typst/article.typ can be found at all,
// which only a broken install can arrange: a plain document still comes out.
const fallbackTemplate = `#let org-template(
  title: none, title-text: none, subtitle: none, authors: (), date: none, email: none,
  lang: "en", paper: "a4", description: none, keywords: (),
  toc: false, toc-depth: none, numbered: true, num-depth: none, columns: 1, body,
) = {
  set document(title: title-text, author: authors)
  set page(paper: paper, numbering: "1", columns: columns)
  set text(lang: lang)
  set heading(numbering: if numbered { "1.1" } else { none })
  if title != none { align(center, text(1.6em, weight: "bold", title)) }
  if toc { outline(depth: toc-depth) }
  body
}`
