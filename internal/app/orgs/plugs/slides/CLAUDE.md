# Notes on the presentation exporters

Loaded when working in this directory. References like **Traps: line edits** point at the Traps list in the root `CLAUDE.md`.

## The four presentation exporters

(A fifth renderer, `orgs pres`, draws the same `Deck` in a terminal; see `cmd/oc/commands/pres/CLAUDE.md`.)

`revealjs`, `impressjs`, `webslides` and `deckjs`: file = deck, headline = slide, properties = behaviour. Everything before html is shared in **`internal/app/orgs/plugs/slides`** (which headlines are slides, speaker notes, backgrounds, file link → url). worg's Presentations tab uses `/file/{exporter}`.

**A property means the same in every framework**: `:BACKGROUND: blue` everywhere, `:REVEAL_BACKGROUND:` reveal only, `#+SLIDE_LEVEL:` everywhere. `Conf` is the ladder: slide's prefixed property → its neutral one → the document's.

**A parsed document isn't reliably a tree** (see **Traps: column-zero drawers**): slides with config have empty `Headline.Children` and nil `Headline.Properties`, and nesting varies within a file. `BuildDeck` walks nodes in order, attributing each to the last headline above it and taking a hoisted drawer as that headline's properties (as `links.go`, `code.go`). This fixed: slides with property drawers losing their whole body, and per-slide properties never being read (`:REVEAL_TRANSITION:` did nothing).

1. **Speaker notes are moved to `Slide.Notes` by the builder, not styled out**: a `:NOTES:` drawer, `#+BEGIN_NOTES` block and a child heading called Notes are equivalent. Showing notes to the audience is the worst failure here.
2. **An excluded (`:noexport:`) heading takes its hoisted body with it**: the builder skips until a heading at that level or above.
3. **Fragments**: a slide property, plus `#+ATTR_SLIDE: :frag`. go-org parses only three affiliated keywords into metadata, so `#+ATTR_REVEAL:` is a plain `Keyword` node before its target; writers catch it in `WriteKeyword` and apply it to the next element (`slides.Pending`). Class per framework: reveal `fragment`, deck.js nested `slide`, impress `substep`. `FragList` puts the class on the `<li>`, since libraries hide the given element and hiding a span leaves the bullet.
4. **Only options the file sets go into the JS config** (`slides.JSOpts`); defaults change between library versions, and writing all forty pins one version.
5. **`plugs.MediaURL` is the only "where is that picture"** (see **Traps: media paths**); old per-exporter copies broke every image in a deck under `notes/talks/` and all of them from a phone.

**Themes: one file for four frameworks** (`internal/app/orgs/plugs/slides/theme.go`, `templates/slides_theme_*.css`). (reveal ships twelve, deck.js three, WebSlides and impress none, each on its own classes.) `slides_theme_base.css` is the **mapping** - what heading, table, code are per framework, set from a dozen custom properties; `slides_theme_<name>.css` supplies the values. A new theme is one file; a new framework surface is fixed once.

- **The mapping loads only with a theme**: its rules beat the library's, and over e.g. reveal's `league` it'd be a third contender.
- **`.org-*` rules are framework-qualified** (`.reveal .org-subtitle, #webslides .org-subtitle, …`), since library classes (WebSlides' `.text-landing` title size, `.text-subtitle` uppercase) otherwise win and silently override a theme's type scale.
- **A theme names its highlight.js style and mermaid theme** (`@hljs:` / `@mermaid:` in its header comment, parsed by `LoadTheme`), so no foreign monokai and no mermaid white card on a dark slide.
- **A slide's `:BACKGROUND:` can fight the theme's ink** (`:BACKGROUND: #102030` under black-on-white `mono`). `slides.ReadInk` measures the ground and the mapping flips text (`org-on-dark`/`org-on-light`) - by *luminance*, since a plain average calls `#0000ff` light and `#00ff00` dark. Picture backgrounds are left alone; `:BACKGROUND_INK: light` sets it explicitly.

`SearchPath` comes from `templatePath:` at startup, so themes don't depend on the working directory `orgs serve` was started in.

**Overfull slides shrink to fit** (`templates/slides_autofit.js`, wrapped by `slides.Fit`); every framework hides overflow (reveal clips, deck.js hides, impress runs off the step, WebSlides scrolls), so it fails silently until projected.
1. **Measure rendered pixels from two rectangles**: content sits in one or two outer transforms (reveal's stage, impress's canvas), and mixing layout pixels is off by that factor. Measuring from the wrapper's top makes anything above simply lost room.
2. **Refine**: laid out at `100% / scale` and scaled back, shrinking re-wraps to fewer lines so one pass overshoots; three passes converge within 1%.
3. **Resize observer cooldown**, since fitting resizes the observed element and would loop.
4. **Background tabs get no animation frames**: every trigger has a timer behind it and a `visibilitychange` listener in front.
5. **Shrink only, floor 45%**: growing gives uneven type; below the floor it's a document and should be noticed while writing. Escapes: `:FIT: grow`, `:FIT: 0.3`, `:FIT: nil`. deck.js' `scale` extension usually leaves nothing to do; WebSlides needs `data-fit-box="viewport"` because its sections grow with content and never look overfull.

Per framework:
- **reveal.js** - nested headline = vertical stack; the wrapper holds *only* sections (a parent heading there renders behind the slides). Plugins are script tags plus `plugins: [...]`; `dependencies: [...]` was **removed in reveal 4.0**, so notes, search and zoom weren't loading. `center: false`, `navigationMode: 'grid'`, `pdfMaxPagesPerSlide: 1` stay default because the template hard-coded them and old decks must look the same.
- **impress.js** - steps can't nest (only direct children of `#impress`), so the tree is flattened (old nested steps were ignored and unreachable). `#+IMPRESS_LAYOUT:` places steps (line, grid, spiral, ring, zoom, random), since hand-placing is why people abandon impress. `random` is deterministic (like the dnd builder's dice) so it can be rehearsed. Old relative `:IMPRESS_X: .6` still means 0.6 screens.
- **WebSlides** - a scrolling responsive page; the presented deck is the sendable file. `:CLASS:` goes onto the section, reaching its forty components without naming them. No notes view, so the `n` panel is ours; it asks `window.ws.currentSlide_`, not `.current`, which *both* slides carry mid-transition (the DOM finds the outgoing one first). WebSlides **renames section ids** to `section-N`, so `:CUSTOM_ID:` is lost.
- **deck.js** - nesting *is* stepping, so outlines map exactly. Centred content is wrapped in `<div class="vcenter">`, not classed on the section: all three themes position `.vcenter` absolutely, which on the section fights deck.js and puts the slide off-screen. jQuery 3 is fine: deck.js only uses `.bind()`, deprecated not removed.
