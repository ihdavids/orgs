// notes: compact, for working notes and handouts. Small margins, a one line
// title, headings that do not shout; #+TYPST_COLUMNS: 2 sets it in two
// columns with the title across both.
#let org-template(
  title: none, title-text: none, subtitle: none, authors: (), date: none, email: none,
  lang: "en", paper: "a4", description: none, keywords: (),
  toc: false, toc-depth: none, numbered: false, num-depth: none, columns: 1, body,
) = {
  set document(title: title-text, author: authors, keywords: keywords, description: description)
  set page(
    paper: paper,
    margin: (x: 1.6cm, top: 1.7cm, bottom: 1.8cm),
    columns: columns,
    footer: context {
      set text(size: 0.75em, fill: luma(120))
      if title-text != none { title-text }
      h(1fr)
      counter(page).display("1 / 1", both: true)
    },
  )
  set std.columns(gutter: 1.4em)
  set text(font: "Libertinus Serif", size: 10pt, lang: lang)
  set par(justify: true, leading: 0.55em, spacing: 0.85em)

  set heading(numbering: if numbered {
    (..n) => if num-depth == none or n.pos().len() <= num-depth { numbering("1.1", ..n.pos()) }
  } else { none })
  show heading: set block(above: 1.3em, below: 0.6em)
  show heading.where(level: 1): it => block(
    width: 100%, inset: (bottom: 0.25em), stroke: (bottom: 0.6pt + org-accent),
    text(size: 1.25em, weight: "bold", fill: org-accent, it),
  )
  show heading.where(level: 2): set text(size: 1.08em, weight: "bold")
  show heading.where(level: 3): set text(size: 1em, weight: "bold", style: "italic")

  show link: set text(fill: org-accent)
  show raw: set text(font: "DejaVu Sans Mono", size: 0.84em)
  show raw.where(block: true): block.with(width: 100%, inset: 0.6em, radius: 2pt, fill: luma(245))
  set table(stroke: 0.4pt + luma(190), inset: (x: 0.45em, y: 0.35em))
  show figure.caption: set text(size: 0.85em)
  set list(indent: 0.3em, spacing: 0.5em)
  set enum(indent: 0.3em, spacing: 0.5em)
  show footnote.entry: set text(size: 0.8em)

  if title != none {
    place(top, float: true, scope: "parent", clearance: 1.2em, block(width: 100%, {
      grid(columns: (1fr, auto), align: (left + bottom, right + bottom),
        {
          text(size: 1.6em, weight: "bold", title)
          if subtitle != none { h(0.6em); text(size: 1.1em, fill: luma(90), subtitle) }
        },
        text(size: 0.9em, fill: luma(90), {
          if authors.len() > 0 { authors.join(", ") }
          if authors.len() > 0 and date != none { [ · ] }
          if date != none { date }
        }),
      )
      v(-0.4em)
      line(length: 100%, stroke: 1pt + org-accent)
      if toc { v(0.4em); text(size: 0.9em, outline(title: none, depth: toc-depth, indent: auto)) }
    }))
  } else if toc {
    outline(depth: toc-depth, indent: auto)
  }
  body
}
