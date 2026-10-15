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

## Invoice and timesheet themes

`invoice/invoice.html` is one page for both kinds; `invoice/base.css` owns layout, paging and print, and a theme in `invoice/themes/` only sets the custom properties (`--accent`, `--ink`, `--muted`, `--rule`, `--tint`, `--paper`, `--font-body/display/num`) and dresses the parts. Listing is a glob of that folder, so a theme is one file. Printing (Chrome's print-to-pdf) notes:

- **Page margins are real `@page` margins** and `.sheet` loses its padding in print, so a second page has margins; `min-height` a little under the page keeps the foot at the bottom of a one-page invoice without spilling a blank page.
- Column rules in base.css hit `td` only: `.lines .num` once set the monospace and muted colour on header cells too (ledger's white-on-green "DATES" disappeared).
- A tinted paper (letterpress) needs `@page { background }` as well, or the margins print white around a tinted box.

## Ebook themes

`epub/themes/*.css` (classic, literary, modern, plain) are the whole stylesheet of an EPUB from `orgs export -f epub`; a new theme is one file. Readers override fonts and sizes, so themes use `em` and lean on the classes `epub.go` writes: `p.first` (no indent), `.scene-break`, `.chapter-head`/`.chapter-label`/`.chapter-title`, `section.part`, `.verse`, `.center`, `.titlepage`, `.dedication`, `.footnotes`, `a.noteref`, `.underline`. Flourishes (`::first-letter`, small caps) must degrade to plain text on readers that ignore them.

## Static site themes (`GET /site`, `orgs site`)

`internal/app/orgs/plugs/site` builds from parsed documents into a map of paths to bytes; `internal/app/orgs/site.go` zips it (`GET /site`, or `local=t&out=` writes on the server) and `cmd/oc/commands/site` unzips it here.

- **Every link is relative** (`Page.href`, `View.Root`), so a site works from `file://` - except the 404 page, rooted at the base url (or `/`). Search loads `assets/search.js`, not the json: a page opened from disk may load a script but not fetch.
- Heading ids are slugs (`CUSTOM_ID` wins, duplicates `-2`), all assigned (`index`) before any page is written, because links point forward. Linked files are copied only from inside the org directories (`Options.Roots`); outside, the link is plain text.
- The site's title is `site.yaml`'s or the request's, else the root `index.org`'s `#+TITLE`, else the folder's name. A summary is the opening prose: headings, code, tables, figures and footnote refs are taken out first.
- A tag called `index` gets `tags/index-tag.html`, so it does not overwrite the tags index. The outline root carries an empty level-0 headline; skip it.
- **A theme is a folder** `templates/site/<name>/`: `theme.yaml` (description, fonts, chroma `highlight: {light, dark}`, `layout:` switches - toc, backlinks, home, sidenotes, nav tree), `style.css`, optional `theme.js` and template overrides. Templates are Go `html/template` parsed in order: `site/base` layout and partials, the theme's layout and partials, base `<kind>.html`, the theme's `<kind>.html`; a later `{{define}}` wins. The shared folder is `site/base`, not `_base`: go:embed skips names starting with `_` or `.`.
- Every file is found through `assets.FindTemplate`, so a user's folder can add a theme or replace one file of a built-in one. Colours use the usual triple `:root` definition.
- **Code colouring is chroma classes**, light style plain and dark style scoped like the palette. A dark style says nothing about tokens it leaves plain, so the light style's colours leaked through (tango's black names on monokai's dark ground); each token is reset to plain before the dark rules (`highlightCSS`).
