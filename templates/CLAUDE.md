# Notes on the html templates and documentation themes

Loaded when working in this directory. References like **Traps: line edits** point at the Traps list in the root `CLAUDE.md`.

## The three documentation themes

`docs`, `rtd` and `furo` share one application, `templates/html_docs_app.js`, pulled into each template with `{% include %}` (not copied): rail, fuzzy search with arrow-key preview, folding, anchors, code copy, scroll spy. A theme is only a shell (`html_<name>.tpl`) plus a stylesheet (`html_styles/<name>_style.css`):

- **`docs`** - masthead, contents rail left, one-column document. The original; the others are modelled on it.
- **`rtd`** - sphinx_rtd_theme: full-height dark rail with a blue search block on top, breadcrumbs, document on white in an 800px column.
- **`furo`** - Furo (as on diataxis.fr): three columns, few rules, one blue; contents left, document at a 46rem measure, "On this page" right.

The exporter doesn't know there are three: `HtmlThemes()` lists every `*_style.css`, `GetTemplate` uses `html_<theme>.tpl` if present. A fourth theme is two files, no Go.

**The script asks the page for ids, never a layout.** A shell must provide `#sidebar`, `#navbar`, `#searchfield`, `#searchresults`, `#searchform`, `#rail-expand`, `#rail-collapse`, `#rail-toggle`, `#theme-toggle`, a `.doc-body` around the document, and `[data-sticky-header]` on whatever covers the top. Asked, not assumed, because the themes differ:

1. **`headerHeight()` measures `[data-sticky-header]`** (zero if none). The old `.header-container` with a 52px default left 52px of dead space above every desktop jump in a theme whose bar exists only on narrow screens.
2. **`closeRail()` checks whether `#rail-toggle` is on screen**, not `innerWidth <= 950`. The toggle shows only when the rail overlays the page, so each stylesheet's one media query defines "narrow": 950px docs, 768 rtd, 820 furo.

Emitted for shells (no-ops where unstyled): **`docs:current`**, a `CustomEvent` with the heading the spy settled on - rtd's last breadcrumb and furo's right column use it instead of watching scroll again and disagreeing at the edges; and **`.is-branch`** on rail rows from root to that heading, which draws rtd's open-section band.

Shared decisions:
- **Light/dark key is `docsTheme` in all three** - same docs; differing would read as a bug.
- **`{{fontfamily}}` is the display face only** (brand, headings); prose uses the reader's UI stack. It is a *server* setting (`exporters: - name: html / props: fontfamily:`), so applying it to Furo's single stack overrode the Furo look. A file's `#+HTML_FONTFAMILY:` still wins, in the theme's own face slot.
- **Rail = pinned head + scrolling body.** `overflow-y: auto` forces the other axis to `auto`, clipping a results panel wider than the rail (a result is heading + outline path + prose line).

`#+SUBTITLE:`: rtd shows it in its version slot, furo under the project name; none, no line.

`htmlThemeLabel` in **`worg/src/settings.ts`** is a label table with a capitalise fallback (which alone gives "Rtd"). It sits there because two callers (settings menu, file view's own-theme button) must label a theme identically. New stylesheets need no entry unless capitalising is wrong or the name doesn't describe the page.

`#+TITLE:` is set on `self.Props` on every export, and a file with no title is named after itself (history in `docs/claude/bug-history.md`).

### The `docs` theme

`#+HTML_THEME: docs`: `templates/html_docs.tpl` + `templates/html_styles/docs_style.css`. Masthead, rail built from the exporter's heading tree (`nodes_json`), one-column document, no jQuery. Built on `OrgHtmlWriter.WriteHeadline`'s ids: each heading is a `heading-wrapper` with `-title`, `-content`, `-text`, named in `nodes_json`.

1. **Rail click scrolls to the heading**, unfolding ancestors (the old tree hid all other headings, which looked like a broken link).
2. **Headings are addressed by slug, not uuid**: `WriteHeadline` makes a fresh `uuid.New()` per export, killing copied links. The slug is built from the name at load, deduplicated, set on the title element; the anchor button copies it and `location.hash` matches it.
3. **`getElementById`, never selectors**: uuids often start with a digit, and `querySelector('#1bfeb…')` throws.
4. **Search is a third copy of `internal/common/dnd/fuzzy.go`** (see **Traps: logic said twice**): loose on a heading's name, whole words only in prose, so a filter doesn't match half the document. (Replaced `new RegExp(input)` over `innerHTML`, which failed on half-typed patterns and put `<mark>` inside tags.)
5. **Hits are marked by walking text nodes**, since replacing over markup lands inside tags.
6. **Jumps past three screens cut instead of animating** - smooth scroll over ~100,000px is a second of blur.
7. **Arrowing through hits previews each (a look, not a move)**, since a snippet can't show if it's the right hit. Escape restores scroll, unfolded sections and the rail's current row; Enter keeps it. Previews always cut (repeating keys would queue smooth scrolls that arrive late). The first arrow press shows the selected hit rather than skipping it, or the top hit could never be previewed. Editing the query ends the look. The heading drops **below the results panel** only where they overlap (narrow screens); otherwise it lands under the masthead.

`HighlightCodeBlock` writes `language-<lang>`, and templates read `hljs_style`, with `hljs_style_default` so a theme can tell "nobody asked" from an explicit choice (history in `docs/claude/bug-history.md`).

`GetStylesheet` rewrites every `url(...)` to `http://localhost:8010/`, so **docs_style.css must contain no `url()`** (chevrons and fold markers are borders partly for this). It also substitutes `{{fontfamily}}` (masthead and first two heading levels; prose uses the UI stack). Palette custom properties are defined three times: bare `:root` (light), `:root:not([data-theme="light"])` under `prefers-color-scheme: dark`, and `:root[data-theme="dark"]`, so the masthead switch wins both ways.
