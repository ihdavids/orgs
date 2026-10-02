# Notes on the worg frontend

Read this before changing worg's UI (source in `../worg/src`). References like **Traps: line edits** point at the Traps list in the root `CLAUDE.md`.

## The search tab's inspect view

Inspect shows a heading's rendered prose, a player for its recording, its pictures, and its tables (`components/OrgTableView.tsx`, read-only, `@n`/`$n` rulers).

- Tables and pictures come from `/body/{hash}`, not the html; the rendered `<table>`s are **hidden** (`'& table': { display: 'none' }`) so nothing shows twice.
- Org tables are parsed by `tablesIn` in `worg/src/orgbody.ts`: a `|---+---|` line is a rule, a `#+TBLFM:` line is not a row, a blank line ends a table.
- Prose uses the files view's theme via `?theme=` on `/todohtml/{hash}`; the stylesheet comes back in `Style` (a fragment has no `<head>`). An iframe has a fixed height and a popup must grow, so `components/ThemedHtml.tsx` uses a **shadow root**, where the stylesheet reaches only the heading.
- `worg/src/htmlframe.ts` rewrites two things meaningless in a shadow root (getting either wrong is silent - the heading still renders, partially styled); `htmlframe.test.ts` pins both:
  1. `html`/`body` become `.org-page`, the fragment's wrapper; the theme's page box (`padding: 1.5em`, `margin: 3em auto`, `max-width: 40em`) is then overridden, because a popup is already a frame.
  2. `:root` (where most themes keep their palette) becomes `:host`, because a shadow root has no document element - otherwise every `var(--bg)` resolves to nothing and the heading gets the theme's fonts but none of its colours.

## Editing a row of the search table in place

**Keyword**, **date**, **tags** and **properties** are edited in place (`worg/src/components/Search.tsx`), not in a dialog, because you edit row after row.

- The keyword is a Joy `Dropdown`, as in the kanban list (`StatusCell` in `KanbanList.tsx`); it asks `/status/{hash}` when the menu opens, not while rows are drawn, so it offers the heading's own file's keywords.
- The other three are a `Popper` with a `ClickAwayListener`, not a `Menu`, because they hold forms and a menu steals arrow keys and typing.
- **A cell that opens a panel must not stop click propagation.** Rows have no click handler to guard against, and panels close via `ClickAwayListener` on the *document*; `stopPropagation` keeps the event from reaching it, leaving two panels open.
- The tags panel is one box: it filters known tags and, on Enter, adds a new one; it keeps focus through a chip click, because tagging is usually several tags.
- The date panel alone has a Save button: date, kind and time are sent together, because a date with no kind or a time with no date is meaningless. The others commit as you go.

## Syntax colouring inside a source block

`worg/src/codehl.ts` colours `#+BEGIN_SRC` code from **the reader's org theme**; hand-written rather than highlight.js, whose own themes cannot follow it. Each scheme publishes eight roles (`codeKeyword`, `codeString`, `codeComment`, `codeNumber`, `codeFunc`, `codeType`, `codeBuiltin`, `codePunct`) built from its base colours, so every existing scheme got it for free.

- Punctuation has two roles. `faint` suits a horizontal rule but not a brace: in Solarized Dark it is `#073642` on `#002b36`, contrast **1.15:1**. Both roles are mixed towards the foreground (`codePunct` 0.6, `codeOp` 0.85), operators further because `:=` is read while `{` is only located. The blend is computed from each scheme's own two colours, not hand-picked for all eleven, so a new scheme gets it right automatically.
- A scanner, not a parser: no scope, no division-vs-regex, a variable named `class` may be a keyword - acceptable. It must never lose a character or let a string run on; `codehl.test.ts` pins that spans concatenate to the line.
- **State is carried per line by `walk`**, not computed while painting, because the source view paints a screenful at a time in fold order and would otherwise start mid-docstring. `LineState` carries the block's language and the scanner's position.
- **Only `src` blocks in a language with a spec are coloured** (gate: `isKnownLang`). `example`, `export` and unknown languages stay one plain span, because guessed half-colouring reads worse than none; the fallback spec claims no keywords for the same reason.

## The command palette

⌘K / ctrl-K (or ctrl-shift-P, or ⌘ on the rail) opens `worg/src/components/Palette.tsx`: panels, capture templates, org files, saved searches, kanban boards, collections, global toggles, and **headings, queried as you type** (the only thing not held in the browser).

1. **The matcher is `worg/src/fuzzy.ts`** (see **Traps: logic said twice**), written in TypeScript because a round trip per keystroke defeats a palette. A term matches a command's **name** loosely (`mm` finds Mind Map) and the line under it only on **whole words**, because fuzzy matching a sentence matches nearly everything.
2. **Picks are remembered** (`noteUse`, localStorage); with nothing typed, recents come first in their own band. Once something is typed recency is a capped *nudge* - it breaks ties, never lifts a worse match over a better one; pinned by `palette.test.ts`.
3. **A panel's other words live in `PANELS`** in `palette.ts` ("spreadsheet" finds Tables, "calendar" finds the Agenda), so people search by how they think of a thing. It deliberately restates what `AppBar.tsx` draws: one is a place on screen, the other a name you type.
4. **Headings are a debounced query, each request aborting the previous one**; `headlineQuery` makes typed text regex-safe *and* case insensitive (`(?i)`), or "jane" would not find "Jane Roe". Headings are never written to recents, because the id may name a row that has moved or gone.

The palette never touches panels directly: `App` hands it `PaletteActions` (switching panels, opening the capture dialog, putting a jump down). A board or collection is opened by **writing the browser setting the panel reads on the way in** (`kanbanBoard`, `collection`), not by adding a second way in.

## Sending the reader from one tab to another

`worg/src/NavContext.tsx` lets a panel send the reader into another (e.g. search's jump opens the Files tab at the heading's line). Shaped like `ChromeContext`, because panels are siblings routed by `App` and direct calls would be an import cycle.

- A jump is **left waiting, not delivered**: `toFile` puts it down and switches panels; the Files tab, which that switch *mounts*, takes it on the way in and clears it. Delivering it would mean handing it to a component that does not exist yet.
- Every jump carries **the way back** (it is usually mid-task): return panel, button label, and an opaque `where` read only by its writer (`NavContext` must not know about search pages or table selections). `goBack` puts the `where` down and switches back. The Files tab draws the button in the toolbar and floating over the page when the toolbar is slid away.
- **The restore is taken at once and acted on later**: the returned-to panel is mounted by the switch, so its list has not arrived. Take it on arrival and hold it in local state until the named row, block or table appears; consuming it on the first pass leaves "pick something", and leaving it down lets another panel find it.
- **Leaving the Files tab by the sidebar drops the way back**, so the button never offers a return to a search abandoned long ago.
- Search rows carry `data-row-hash` so the return can find the row and flash it - a React key is React's handle, not the document's.
- The exported view has no line numbers, so it scrolls to the heading by text (`pendingHeading`); the source view scrolls to the line (`OrgSource`'s `scrollToLine`, which wins over `scrollTo` because two headings can read the same). Both flash what they landed on.
- The Files tab's view is a **browser setting** (`fileView`), not component state, because state was lost when you visited another tab - exactly the trip a jump makes. A character sheet overrides it and is never stored, because "sheet" is not a way of reading every file.

## The Tables tab's jump

The open table's header has the search rows' jump (file at the table's `Line`, in the Files tab) and way back. `Line` already existed, so no server change.

## The search tab's file affordances

Browser settings: `searchShowFile` shows file and line under each headline, outside the hover tooltip so pointing at the path doesn't open the preview; `searchGroupByFile` groups results under foldable, counted per-file bands.

Grouping is **a sort plus header rows in the same table**, so columns align and sort/select/edit need no second path. Bands occupy page slots like rows: drawing bands after paging fails because a folded file has no rows on any page.

## Charting a spreadsheet selection

Tables tab charts a selection (ctrl-g or chart button) in eighteen types (lines, bars, parts of a whole, comparisons) drawn by **echarts**; `worg/src/chartspec.ts` decides what to draw and is pure and tested, because a wrong-axis chart still looks plausible. echarts (~1MB) loads on first popup open, not with the app.

`chartspec.test.ts` pins:
1. **A gap is never a zero**: `null` throughout, lines break. Radar alone uses zero (no missing-point notion).
2. **Multi-series data on single-series types is folded** (`foldToTotals`), not silently truncated; pie, donut, rose and funnel say so in the subtitle.
3. **Horizontal bars swap axes, not data**, so tooltip and legend agree.

## Mind maps

worg's Mind Map tab draws a saved query's headings with one of three engines, chosen by the browser setting `mindEngine` (not a server one): **mind-elixir** (default; branches can be dragged), **jsMind** (boxes and branches, easiest to read) and **mermaid** (a fixed drawing, for documents).

All three get one tree, built in `worg/src/mindmap.ts` from the query's own rows. Do not build it from the server's mermaid source: the `mindmap` exporter emits only a label and an indent (no keyword, tags, file or hash), so a map parsed from it (as the old tab's regex-scraping did) cannot be clicked back to a heading, coloured by keyword, or filtered. Rules in that module:

1. A heading hangs off **the last heading above it with a smaller level**, whatever that level, because a query can find a level-3 heading whose level-2 parent did not match; any other rule orphans it or makes it a top-level branch.
2. **One file goes straight under the root; several files each get their own branch**, or branches from three files read as one outline.
3. A node's **id is the heading's hash**, so a click in any engine finds the heading.
4. Every node knows **which root branch it hangs from** (`branch`), because colouring is by branch, not depth; that is what makes a wide map readable.

Nothing an engine does is written back. Dragging in mind-elixir rearranges the drawing, not the outline, because a drag that silently refiled a heading is not what someone rearranging a map expects.
