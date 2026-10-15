// article: a title block on the first page, numbered sections, one column.
//
// A template defines org-template, which the exported document hands itself to
// with #show: org-template.with(...). It may redefine any org-* helper from the
// prelude above it to dress keywords, tags and special blocks its own way.
#let org-template(
  title: none, title-text: none, subtitle: none, authors: (), date: none, email: none,
  lang: "en", paper: "a4", description: none, keywords: (),
  toc: false, toc-depth: none, numbered: true, num-depth: none, columns: 1, body,
) = {
  set document(title: title-text, author: authors, keywords: keywords, description: description)
  set page(
    paper: paper,
    margin: (x: 2.4cm, top: 2.6cm, bottom: 2.8cm),
    columns: columns,
    numbering: "1",
    number-align: center,
  )
  set text(font: "Libertinus Serif", size: 11pt, lang: lang)
  set par(justify: true, leading: 0.62em, spacing: 1.05em)

  set heading(numbering: if numbered {
    (..n) => if num-depth == none or n.pos().len() <= num-depth { numbering("1.1", ..n.pos()) }
  } else { none })
  show heading: set block(above: 1.6em, below: 0.8em)
  show heading.where(level: 1): set text(size: 1.4em, weight: "bold")
  show heading.where(level: 2): set text(size: 1.18em, weight: "bold")
  show heading.where(level: 3): set text(size: 1.04em, weight: "bold")
  show heading.where(level: 4): set text(size: 1em, weight: "regular", style: "italic")
  // A block, not bare inline content: bare, a heading reads as a paragraph,
  // and the second of two in a row took the paragraph indent.
  show heading: it => block({
    if it.numbering != none and counter(heading).get().len() > 0 {
      text(fill: org-accent, counter(heading).display(it.numbering))
      h(0.6em)
    }
    it.body
  })

  show link: set text(fill: org-accent)
  show raw: set text(font: "DejaVu Sans Mono", size: 0.86em)
  show raw.where(block: true): block.with(width: 100%, inset: (x: 0.9em, y: 0.75em), radius: 3pt, fill: luma(246))
  show quote.where(block: true): it => block(
    inset: (left: 1em, y: 0.2em), stroke: (left: 1.5pt + luma(200)),
    text(fill: luma(60), style: "italic", it.body),
  )
  set table(
    stroke: none,
    inset: (x: 0.6em, y: 0.45em),
  )
  // Booktabs: a rule above and below, a thin one under the header.
  set table.hline(stroke: 0.5pt)
  show table: it => block(stroke: (y: 0.8pt), it)
  show figure.where(kind: table): set figure.caption(position: top)
  show figure.caption: set text(size: 0.9em)
  set list(indent: 0.6em)
  set enum(indent: 0.6em)
  show footnote.entry: set text(size: 0.85em)

  if title != none {
    align(center, block(width: 85%, below: 2em)[
      #text(size: 1.9em, weight: "bold", title)
      #if subtitle != none { v(0.1em); text(size: 1.2em, fill: luma(80), subtitle) }
      #if authors.len() > 0 { v(0.6em); text(size: 1.05em, authors.join(", ", last: " and ")) }
      #if email != none { linebreak(); text(size: 0.9em, fill: luma(90), link("mailto:" + email, raw(email))) }
      #if date != none { v(0.2em); text(size: 0.95em, fill: luma(90), date) }
    ])
  }
  if toc {
    block(below: 2em, outline(depth: toc-depth, indent: auto))
  }
  body
}
