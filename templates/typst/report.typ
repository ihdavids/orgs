// report: a title page, a contents page, and every top level heading a
// chapter starting on a new page.
#let org-template(
  title: none, title-text: none, subtitle: none, authors: (), date: none, email: none,
  lang: "en", paper: "a4", description: none, keywords: (),
  toc: false, toc-depth: none, numbered: true, num-depth: none, columns: 1, body,
) = {
  set document(title: title-text, author: authors, keywords: keywords, description: description)
  set page(paper: paper, margin: (inside: 3cm, outside: 2.5cm, y: 2.8cm))
  set text(font: "New Computer Modern", size: 11pt, lang: lang)
  set par(justify: true, leading: 0.6em, spacing: 1em, first-line-indent: 1.2em)

  // The title page.
  page(numbering: none, margin: (x: 3cm, y: 3cm), {
    set par(justify: false, first-line-indent: 0pt)
    v(22%)
    line(length: 100%, stroke: 1.2pt + org-accent)
    v(0.8em)
    if title != none { text(size: 2.6em, weight: "bold", hyphenate: false, title) }
    if subtitle != none { v(0.4em); text(size: 1.4em, fill: luma(70), subtitle) }
    v(0.6em)
    line(length: 100%, stroke: 0.5pt + org-accent)
    v(1fr)
    if authors.len() > 0 { text(size: 1.25em, authors.join(linebreak())) }
    if email != none { linebreak(); text(size: 0.95em, fill: luma(90), raw(email)) }
    if date != none { v(0.8em); text(size: 1.05em, fill: luma(80), date) }
    if description != none { v(1.5em); block(width: 80%, text(size: 0.95em, style: "italic", fill: luma(60), description)) }
  })

  set heading(numbering: if numbered {
    (..n) => if num-depth == none or n.pos().len() <= num-depth { numbering("1.1", ..n.pos()) }
  } else { none })

  // Front matter in roman numerals, the chapters from 1.
  set page(numbering: "i", number-align: center)
  if toc {
    show outline.entry.where(level: 1): it => { v(0.6em, weak: true); strong(it) }
    outline(depth: toc-depth, indent: auto)
    pagebreak(weak: true)
  }
  // A running head - the chapter, then the page - on every page after a
  // chapter's first; the page number at the foot of the rest. Only outlined
  // headings are chapters: the contents' own title is a heading too.
  let chapter-before(page) = query(heading.where(level: 1, outlined: true))
    .filter(h => h.location().page() < page)
  let chapter-starts(page) = query(heading.where(level: 1, outlined: true))
    .filter(h => h.location().page() == page)
  set page(numbering: "1", header: context {
    let p = here().page()
    let before = chapter-before(p)
    if before.len() > 0 and chapter-starts(p).len() == 0 {
      set text(size: 0.85em, fill: luma(110))
      emph(before.last().body)
      h(1fr)
      counter(page).display()
    }
  }, footer: context {
    let p = here().page()
    if chapter-before(p).len() == 0 or chapter-starts(p).len() > 0 {
      align(center, text(size: 0.85em, counter(page).display()))
    }
  })
  counter(page).update(1)

  show heading.where(level: 1): it => {
    pagebreak(weak: true)
    v(12%)
    if it.numbering != none {
      text(size: 1.05em, fill: org-accent, tracking: 0.08em, upper[Chapter #counter(heading).display("1")])
      v(0.4em)
    }
    block(below: 2.2em, par(justify: false, text(size: 2em, weight: "bold", hyphenate: false, it.body)))
  }
  show heading.where(level: 2): set text(size: 1.3em)
  show heading.where(level: 3): set text(size: 1.12em)
  show heading: set block(above: 1.7em, below: 0.9em)
  // Below a chapter: the number in the accent colour. A chapter is left as it
  // is, for the rule above.
  // A block, not bare inline content: bare, a heading reads as a paragraph,
  // and the second of two in a row took the paragraph indent.
  show heading: it => {
    if it.level == 1 { return it }
    block({
      if it.numbering != none and counter(heading).get().len() > 0 {
        text(fill: org-accent, counter(heading).display(it.numbering))
        h(0.7em)
      }
      it.body
    })
  }

  show link: set text(fill: org-accent)
  show raw: set text(font: "DejaVu Sans Mono", size: 0.85em)
  show raw.where(block: true): block.with(width: 100%, inset: (x: 1em, y: 0.8em), stroke: (left: 2pt + org-accent.lighten(40%)), fill: luma(248))
  show quote.where(block: true): it => pad(x: 2em, text(style: "italic", it.body))
  set table(
    stroke: none,
    inset: (x: 0.7em, y: 0.5em),
  )
  set table.hline(stroke: 0.5pt)
  show table: it => block(stroke: (y: 1pt), it)
  show figure.where(kind: table): set figure.caption(position: top)
  show figure.caption: set text(size: 0.92em)
  show figure: set block(above: 1.6em, below: 1.6em)
  show footnote.entry: set text(size: 0.85em)
  set list(indent: 0.8em)
  set enum(indent: 0.8em)

  body
}
