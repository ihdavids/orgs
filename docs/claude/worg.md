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

## The Flashcards tab (Drill)

`components/Drill.tsx`, session logic in `src/drill.ts` (pure, tested by `drill.test.ts`); server in `internal/app/orgs/drill.go` (the algorithms) and `drillcards.go` (reading cards, writing ratings). User guide: `docs/drill.org`. It mirrors Emacs org-drill: same properties, spellings, algorithms and session rules, so a card can be drilled in either.

- **A deck is a query** (or a saved search), stored per user in extensions like kanban boards (`DrillDeck` in Go; a TS-only field is dropped on save, see **Per-user extensions**). A found heading under a found card is part of that card, and a found heading with no body over found headings is a grouping heading: queries match inherited tags, so `HasTags("spanish")` finds the parent as well as the words.
- **What is written lives on the server, what is chosen lives in the browser.** Scheduling, properties, SCHEDULED and leech are server-side (one request per rating). Queue order, the again pile, limits, which clozes hide and which side is asked are `drill.ts`, chosen afresh each showing as org-drill does.
- **Clozes are wrapped server-side** in `<span class="drill-cloze" data-n data-hint data-len>` via `@@html:` export snippets, then `clozeHtml` rewrites them per phase. Own-body clozes are numbered from 1; title and side clozes are 0 (always hidden in the question). A cloze left showing in the question is plain text, or it would mark where the answers are.
- **The "+N days" on a rating button is the interval written**: the server computes `next[0..5]` with noise seeded from hash + day + repeats, so preview and write agree.
- **The session survives leaving the tab** (module-level `live`), which is how `e` (edit in the Files tab) and quit work like org-drill-resume.
- Cram writes nothing and counts nothing, as in org-drill.
- `orgs drill` (`cmd/oc/commands/drill/`) is the terminal client of the same endpoints; its session logic is a Go copy of `drill.ts` (keep the two in step), and it reads cards with `format=org`.

## Editing a file in the Files tab

The Org view has an **Edit** button (offered in no other view) that swaps `OrgSource` for `components/OrgTextEditor.tsx`: CodeMirror 6, coloured by OrgSource's own `lineTokens`/`walk`, with vim (`@replit/codemirror-vim`, the default) or a small hand-written emacs keymap, picked by the browser setting `editorKeys` from the settings menu. A status line under it shows the vim mode (icon + NORMAL/INSERT/VISUAL/V-LINE/REPLACE), cursor, saved state and field-step hint.

- **Saving** is `POST /orgfile` (`internal/app/orgs/orgfilesave.go`) with `Base`, the text the editor started from; the server refuses with `Msg: "changed"` when the file on disk is no longer that, and the editor offers Overwrite (`Force`) or Discard-and-reload. It only writes files the OrgDb knows. `:w`, `:wq`/`:x`, `:q`/`:q!`, ⌘S/ctrl-S, C-x C-s, C-x C-c. Edit refetches the file first, so the base is current.
- **`<x TAB`** (org-tempo's table, `TEMPO` in `orgedit.ts`) expands only with nothing but indentation before the `<`. **`<c TAB`** opens a menu of capture templates (a CodeMirror tooltip, filtered with `fuzzy.ts`); the pick is written *in place* as fields by `captureExpansion`, reusing capture.ts's placeholder rules. TAB/shift-TAB step, TAB off the last finishes (tags formatted `:a:b:`, lines left holding only an empty field removed, as `fillTemplate` does), ESC in a capture removes all of it including `<c`.
- Fields are a StateField mapped greedily at both ends; two fields with one key mirror through a transaction filter. Headline and tags fields are separated by a space because adjacent greedy fields would both swallow text typed at their shared edge.
- The editor's TAB/ESC/menu handler is `Prec.highest` domEventHandlers, ahead of vim's own keydown handler. This vim has no folding; `za zc zo zM zR` are mapped to CodeMirror's fold commands over `outline()`.
- Leaving with unsaved text (another file, another view, refresh) goes through `guard` in `Files.tsx` and asks. The command palette ignores a ctrl-K the editor already took (`defaultPrevented`), so emacs C-k kills.
- **Org commands** (move/promote subtrees and items, the "next thing", checkboxes and `[n/m]`/`[%]` cookies, todo/priority cycling, SCHEDULED/DEADLINE, timestamps and S-arrow shifting, properties) are pure functions in `worg/src/orgcmds.ts`, pinned by `orgcmds.test.ts`: lines + cursor in, line edits (original numbering) + cursor/fields (new numbering) out, null when they do not apply so the key falls through. `CMDS` in the editor names them once for every binding: shared M-/S-arrows and M-RET, vim normal M-hjkl and the `SPC` leader (`LEADER`), emacs `C-c` chords, and the smart expansions `<p <sch <dl <t <i` (`SMART` in `orgedit.ts`).
- **Edits are applied as minimal character diffs** (`lineChanges`' `tight`), not whole-line replacements: a replaced line carries every mapped position through it (the field being filled, the snippet's end, the cursor) to the end of the line or past it - which put a property in the *next* heading's drawer.
- **Never bind a `C-h` prefix in the emacs keymap.** CodeMirror's default keymap binds `Ctrl-h` on a mac, and a key that is both a binding and a multi-stroke prefix throws on every keydown, taking every emacs key down with it. Help is F1.
- `jk`/`jj` are `Vim.map(..., 'insert')`, timed by vim's `insertModeEscKeysTimeout` (200ms). Leaving insert mode that way finishes a field step (the mode-change listener, deferred because vim fires it mid-update) rather than cancelling a capture.
- **The help** is `docs/editor.org` in this repo, embedded by `docs/embed.go` and rendered by `GET /editor/help` (`internal/app/orgs/editorhelp.go`); `components/EditorHelp.tsx` (mounted once in `App`) shows it, opened through a window event by F1, `SPC ?`, `:help`, the Keys button and the settings menu's Editor keys pane. Change a binding and change that file.

## The Diff tab

`components/Diff.tsx` (panel), `DiffView.tsx` (the viewer), `DiffHistory.tsx` (the commit graph), `DiffOutline.tsx` and `DiffSummary.tsx`; the model is `src/orgdiff.ts` and `src/githistory.ts`, both pure and tested. Server: `internal/app/orgs/filehistory.go` (`GET /history`, `GET /history/text`). The Files tab's History button opens it on the file being read (`nav.toDiff`, which writes the `diffFile` setting and switches).

- **History** shows a file's commits as a lane graph (`layoutGraph`), with the staged and working copies on top as dashed nodes. Click = that commit against its parent; shift/⌘-click = compare two, older on the left. `EMPTY` is the side before the file existed.
- **Compare** takes any two sources: an org file the server reads at any revision, a local file (picked or dropped), or pasted text. The server never reads a path outside the OrgDb.
- **All diffing is in the browser**, because whitespace and context options re-run it. Lines are a **patience diff** (unique lines anchor first, Myers in the gaps): a plain shortest edit script matches one heading's `:END:` to another's and reads as a rewrite. Within a change, old and new lines are **paired by likeness** (`pairChunk`, threshold 0.35), not position, so a word diff lights only what changed.
- **Whitespace is not a difference to the eye**: `similarity` collapses runs of space and `wordDiff` treats every space run as one token, or re-padded headline tags read as a different line with a highlighted bar of nothing.
- **The Outline view** (`outlineDiff`) matches headings by ID, then by place (outline path), then by unique title elsewhere (refiled), then by likeness under the same parent (renamed), and reports each change in org terms (keyword with a tone: completed/reopened; priority; tags; SCHEDULED/DEADLINE/CLOSED; properties; body and logbook line counts; refile; promote/demote). Removed headings are put back where they stood. Clicking a heading opens the text view at it, opening the fold that hides it.
- Colours come from the reader's **org theme** (`diffColors`), like the Org view.
- Joy's `ToggleButtonGroup` reads `value` off its **direct children**: a `Tooltip` around a button silently breaks the toggle. Use `title` instead.

## Actual time on the gantt, and C-c |

- The Gantt tab's Options has **Actual time** (React renderer): a mark under each bar per clocked sitting, at its own times, from `/gantt/tasks`' `Clocked` (`clockedSpans`/`clockedSummary` in `gantt.ts`, tested in `ganttplan.test.ts`). Stamps are read as local time - `new Date('2026-10-04')` is UTC and moves evening work a day. Marks are clipped to the chart, which is laid out from the plan. Saved with the view as `actual` (Go field `Actual` in `ganttviews.go`, or it is dropped on save).
- The editor's **C-c |** / `SPC |` is `tableFromText` in `orgcmds.ts`: with no region (the editor commands get a cursor, not a selection) it turns the paragraph at the cursor into a table, split as org splits - tabs if every line has one, commas if every line has one, else spaces; a blank line starts an empty table. Listed in `docs/editor.org`.
- `IsDone` and `InFile` were added to `QUERY_FUNCTIONS` (see **Traps: logic said twice**).

## Gantt: tooltips, removing a dependency, lanes, assignees

- **Bar and name tooltips are not svg `<title>`s** (`ChartTip` in `ReactGantt.tsx`): the browser waits most of a second before showing a title. The tip is driven through a ref handle so pointer moves re-render only the tip, not the chart. 150ms delay; once one is showing, the next bar's shows at once. Vis keeps its readout line above the chart instead, now with the assignee's badge.
- **Removing a dependency**: click an arrow for a red radial menu (`GanttDepMenu.tsx`, shared by the React and Vis charts): ✕ removes it, ← / → select the task at either end. In the React chart, dragging an arrow back onto its source also removes it. `unlinkOp` (`ganttops.ts`) clears AFTER and, when the server says the heading is `Chained` (an ORDERED parent or grandparent would chain it if it had no AFTER), also writes `NOBLOCKING: t`. That is org's own property for "not blocked by siblings", and `After` in `plugs/mermaid` **and** its copy in `plugs/gantt` skip the ORDERED walk for it (AFTER still counts). `Chained` is worked out by `mermaid.Chained`, which runs `After` with AFTER and NOBLOCKING taken off. The inspector's "Comes after: (nothing)" uses the same op.
- **Arrow hit areas** are drawn over the rows and stop short of both ends, where the resize grips and link handles are (`arrowHitPath` in React, `trimmedPath` in Vis). Google draws its own arrows and exposes nothing, so it has no arrow menu.
- **Assignee** = `assigneeOf` in `ganttplan.ts`: ASSIGNEE, then ASSIGNED, RID, RESOURCEID. Not `GanttTask.resource`, which falls back to the owning project's name. The badge (`PersonBadge.tsx`) is coloured by kanban's `autoSwatch`. The inspector's card offers every value of those properties across all files (`GET /properties?all=...` lifts the twelve-value cap for the named properties, fetched once per session). A pick is written as the person is most often spelled, so jira's `Name [email][id]` survives. It is always written as **ASSIGNEE** (an ASSIGNED on the heading is cleared in the same write); ASSIGNED is still read everywhere. On load the chart renames ASSIGNED to ASSIGNEE on every task it shows that has no ASSIGNEE (`assignedToMove` in `ganttplan.ts`, one undoable write, each heading tried once a session), and a saved view naming ASSIGNED for lanes or workload is read as ASSIGNEE (`assigneeName`). New properties are written two spaces in from `:PROPERTIES:`, or at the indent of the drawer's other properties (`setPropIn`).
- **ORDERED arrows**: a wait an ORDERED parent makes (server `After` set, `AfterId` empty, `orderedLink` on the task) is drawn dotted in dark yellow (`ORDERED_COLOR` in `ReactGantt.tsx`), critical-path red still winning. Arrows into a folded epic are not drawn, ORDERED or not.
- **Moving an epic**: `moveBlock` in `ganttepics.ts` swaps a task and everything under it with the sibling block above or below in its lane, then `reorderOps` renumbers the lane's GANTT_ORDER. Alt+↑/↓, the inspector's Move buttons, the bar's right-click menu. Rows in another lane (a SECTION of their own) stay put.
- **Names column width**: dragged by its right edge (double click resets), kept in the `ganttLabelWidth` setting; `ReactGantt`'s `LABEL_WIDTH` is a local shadowing the module default, and the page passes it to `xForDate` and the minimap.
- **Escape with several rows picked** lets go of them even from the bulk bar's fields or under the lightbox editor.
- **Lanes**: `laneColors` in `ganttstyle.ts` (hue from `autoSwatch(lane name)`, wash ~98% towards the background (96.8% on dark), heading band and ink in deeper shades). Used by React directly and by Vis through a `vg-lane-N` class on each lane's groups.
- **Jump To File** (bar menu) goes through `nav.toFile` with a `back.where` carrying the query, filter and selected task. An unsaved query would otherwise be lost on the way back. It is restored after the saved view effect, so it lands on top of that view. Google has the bar menu too: its right click finds the row by the name in the track under the pointer, because Google re-sorts rows by dependency.
- **Where the task editor lives** is the `ganttEditorMode` setting, switched from the editor's own header: `side` (sticky beside the chart), `lightbox` (Joy Modal), `inline` (a Popper on a virtual anchor that re-measures the selected bar each time it is asked: `[data-gantt-bar=hash]` in React, `.vis-item.vis-selected` in Vis, the last press on the chart for Google, so it survives the redraw after a write), `bottom` (full width under the chart, fields in a grid of columns).
- **Undated tasks follow what they wait on, however old.** A task with no date of its own starts when its AFTER ends, so linking a current task into a chain that starts at a years-old SCHEDULED moves it years back, off the visible part of the chart. It looks like it vanished. Seen with a 2022 worklog chain.

## Importing contacts (vCard)

The Contacts tab's **Import** button opens `components/ContactImport.tsx`; pure helpers and wire types in `src/vcardImport.ts` (tested by `vcardImport.test.ts`); the request goes through `ODb.doPost` from the component, not a new `db.ts` method. Server: `POST /contacts/import` (`internal/app/orgs/vcardimport.go`, notes in `internal/app/orgs/CLAUDE.md`).

- **The plan shown is the server's dry run of the same request**, re-asked whenever the files or the mode change (numbered, so a slow answer to an old choice cannot land on a new one). The browser does no matching or merging, so the dialog and `orgs contact import` cannot disagree about a file.
- **Files are read as bytes and decoded here** (`vcfText`: UTF-8 when valid, else Windows-1252), because an old Outlook export in 8 bit would reach the server through JSON as question marks. Several files are joined into one text (`joinVcfTexts`); `countCards` gives the per-file count shown before anything is sent.
- New people, updates (chips: `+ KEY` added, amber replaced, outlined "keeps KEY" with both values in its title) and unchanged people (folded) are listed separately; each row unfolds to its values. The button says what will be written ("Add 3, update 2") and is disabled when nothing would be.
- **After a write the browser is remounted** (`key={generation}` on `RecordBrowser` in `Contacts.tsx`) and the birthday strip re-asked, rather than editing `RecordBrowser.tsx`, which the Collections tab shares.

## Starmap: colour by expression

`starmap/rules.ts` (tested). Each rule is an org query and a colour; the server answers the query (`/search`, no base filter), and every star it finds is drawn in that colour, first matching rule first, ahead of the constellation/keyword colour. A file star is lit by its headings only in file scope; in a heading sky it is a hub. Saved as `colorRules` in the `starmap` setting. Queries are re-asked 450ms after typing stops and on every reload.

## Gantt: room for the chart

- **Chart only** (`ganttMaximised` setting; the button top right of the chart): page title through `useChromeToggle`, the controls Sheet hidden with `display: none` (not unmounted, so a half-typed query survives), the legend left out, the react box allowed `100dvh - 48px`.
- The **title** sits in the header's top-left corner on the dates' line, cut to the names column (full title in its `<title>`). The band above the dates (`MARKER_BAND`) is only there when the chart has markers, since it holds their labels: `headHeightFor(hasMarkers)`, which GanttChart's row measuring uses too.
- `PAD` is 6 and the chart's Sheet has `p: 0.5` for the react renderer.
- **Task names** are cut by measured width (`fitText`, canvas `measureText` in the drawn font, cached) to stop short of the row's grip. They used to be cut at 30 characters, so wide letters ran over it.
- **The filter** (Filter panel) is a second org query. Only rows both queries find are drawn, but dates and arrows still come from the whole plan (`filterModel`), so nothing moves. Lanes left empty go.

## The Inbox and Weekly Review tabs

`components/Inbox.tsx` (inbox / someday toggle, kept in localStorage `worg.inboxMode`), `components/Triage.tsx` (the one-at-a-time walk, shared by both tabs), `components/TriageParts.tsx` (the big item card, key legend, refile/date/tag pickers, `useReviewActs`), `components/WeeklyReview.tsx` (the seven-step wizard). Logic in `src/review.ts` (pure, `review.test.ts`), calls in `src/reviewApi.ts`; `src/reviewtabs.test.tsx` renders both tabs against a stand-in api. Server notes: `internal/app/orgs/CLAUDE.md`, "Inbox triage".

- **Keys**: inbox `r` refile (fuzzy picker over `/refile/targets`, matched by `move.ts`'s `filterTargets`), `s` schedule (typed through `parseWhen`, previewed, sent as an org stamp; what it cannot read - a time - is sent as typed and read by the server's `internal/orgdate`), `t` tag (chips plus known tags from `/alltags`), `a` archive, `d` done; someday `k` keep, `p` promote (then `n` NEXT / `t` TODO), `s` schedule, `a` drop. Always `n`/`→` next, `←` back, `u` undo, `o` open in Files. Every key is also a button. Keys are ignored while typing, with a modifier, or while a picker is open (`isShortcut`).
- **The list is fetched again after every decision, never patched** (hashes shift; see **Traps: hashes**). The walk keeps its place by `itemKey` (file + outline path + title) in a module-level set, so the Files tab round trip (`nav.toFile` with a way back) returns to the same item. A decision that ends the item (refile, archive, done, promote) moves on; one that only changes it (date, tags) keeps it on screen, because the next thing is usually to refile it. The same rule is `orgs triage`'s `Walker.Walk` - keep them in step.
- **Undo** is the server's (`POST /review/undo` with the token `/review/act` handed back); the stack is module-level per list, so it survives leaving the tab.
- **The wizard** keeps its step and running tally on the server (`/review/step`), so it resumes where it was left in either client; steps 1 and 6 embed `Triage`, 2/3/5 are lists with inline actions, 4 is a table with clocked/planned bars (`timeBar`), 7 records it (`/review/done`). The waiting threshold is `worg.weeklyReviewWaitingDays` in localStorage, defaulting to the server's.

## New org file

The rail's note-add button and the palette's "New org file" open `components/NewFileDialog.tsx`. Logic is in `src/newfile.ts` (tested): `rootsOf`/`dirLabel` (folders shown from the org directory's own name down), `rankDirs` (fuzzy, a match on the folder's own name counts double, shallowest first when nothing is typed), `matchPositions` for fzf-style highlighting, `nameError`, `newFilePath`. Server: `GET /newtemplates` (the `new/*.tpl` under the template path), `GET /dirs?all=t` (every folder under the orgDirs, hidden and node_modules skipped), `POST /file`. `/file` now only writes `.org` files inside an org directory (`insideOrgDirs`), only takes `new/*.tpl` templates, and with `mustBeNew` answers 409 rather than reloading an existing file. A new folder is just part of the path: `CreateOrgFileFromTemplate` makes the folders on the way. The last folder and template used are remembered in localStorage.

## Focus (pomodoro)

`components/Focus.tsx` (the tab), `components/FocusStatus.tsx` (the header chip: desktop breadcrumb row, and `compact` in the phone `Header`), logic in `src/focus.ts` (tested by `focus.test.ts`), requests and live state in `src/focusApi.ts`. Server: `internal/app/orgs/focus.go` (see its section in `internal/app/orgs/CLAUDE.md`); `orgs focus` is the terminal client.

- **One store for every view** (`focusApi.ts`'s `store`, `useFocus()`): the chip, the phone chip and the tab share one EventSource, one poller and one notification. The stream opens with the first subscriber and closes with the last.
- **EventSource sends no Authorization header**: it rests on the `orgstoken` cookie. Across origins it is opened credentialed; one that fails before ever opening is retried without credentials (enough under noAuth), then left to polling (30s with the stream up, 10s without, and on tab visibility).
- **The countdown is local**: the end is `now + Remaining` when an answer arrives, not the server's `Ends`, so a skewed laptop clock does not skew the timer. A one-second tick runs only while a phase does, and passing zero asks the server at once rather than waiting for the event.
- `document.title` shows the countdown while running and is put back when idle. Notifications fire only for transitions the session made itself (`transitionNotice`: work→break, break→idle/work), not for a stop; permission is asked only from the tab's "Notify me" switch (`focusNotify` in localStorage).
- The chip sits outside `NavContext`'s provider, so "Open task" / "Focus tab" dispatch a `worg:focus-nav` window event that `App` turns into `nav.toFile` or a panel switch.
- The tab: task list from `ODb.activeTodos()`, recent tasks (from `/focus/history?days=14`) first, `fuzzy.ts` over the headline and the file's base name once typed (recency only nudges). Presets 25/5, 50/10, 90/20 or custom; lengths are seeded from the server's last-used settings. Keys (not while typing, no modifiers): space start / skip a break, `s` stop, `+` five more, `/` the filter.

## The Agenda's day view is a planner (time-blocking)

`components/DayPlanner.tsx` (grid, drags, load bar), `components/PlannerList.tsx` (tasks to place), logic in `src/planner.ts` (pinned by `planner.test.ts`), requests in `src/plannerApi.ts`. Server: `internal/app/orgs/planner.go` (its section in `internal/app/orgs/CLAUDE.md`); `orgs plan` is the terminal client. The old read-only `DayView` and its `/search`-based day fetch are gone; week and month views are unchanged.

- **Three ways onto the day**: drag a task from the list onto an hour (HTML5 drag; the ghost shows where and how long, sized by EFFORT else 60 min, snapped to 15); click a task then click an hour (the phone's way, list items cannot be dragged on touch); Enter (or double click, or the row's ↵) puts it in the first free gap from now. A block's body drags to move, its bottom edge to resize, back onto the list (which lights up) or its × to unplan. Blocks are pointer events with capture (`touch-action: none` on movable ones), so they move on a phone too.
- **Keys**: in the list ↑/↓, Enter places, space picks, `o` opens; in the filter ↓ into the list, Enter places the best match. On a focused block ↑/↓ move 15 min, shift+↑/↓ move its end, Delete unplans, Enter opens. `/` the filter, `p` the list, Esc cancels.
- **Optimistic, in order**: every change is drawn at once (`placeLocally`/`unplanLocally`, which also recompute load and overlaps) and written through a promise queue so a burst of arrow presses lands in order. A refused write restores the day as it was before that change and shows why; reads started before a write are discarded (`fetchSeq`), and the day is re-read after the last write, because the server is the truth.
- **Overlaps** are drawn side by side (`layoutLanes`) with a red edge and a warning icon; the header says planned / free / double-booked within the working day and the backlog's EFFORT. Fixed blocks (appointments, plain stamps) are hatched and only open; deadlines are a dashed line.
- Per browser (`worg.planner` in localStorage): working hours (header selects), the list's query (default `IsTask()` on the server, any saved query, or a typed one), list shown or hidden.
- Not done: planning inside the week view (still read-only).

## Backlinks and unlinked mentions (the links panel)

`components/LinksPanel.tsx` (logic in `src/mentions.ts`, pinned by `mentions.test.ts`; requests in `src/mentionsApi.ts`) shows, for the heading being read or the whole file: **Linked from** (each with its line, the link itself marked), **Links out**, and **Unlinked mentions** grouped by file and heading, the match marked, a Link button each and **Link all**. Server side is `internal/app/orgs/mentions.go`.

- **Files tab**: the link icon beside the Links button toggles it as a column *beside* the page (an overlay on a phone); open/closed is `worg.linksPanel` in localStorage. It follows the reader: the source view reports its top visible row (OrgSource rows carry `data-line`; a throttled `onScrollCapture` halves over them) and the clicked row; the editor reports its cursor (`onCursorLine`); the rendered view reports the last `h1`-`h6` above the top of the frame plus how many same-text headings precede it (`headingForRendered` maps that onto `/filecontents/headings`). The panel waits 250ms after the reader stops before asking.
- **Following an entry pins the panel** (the pin in its header), because going to a mention scrolls the page to another heading and the panel would otherwise switch to that heading's list mid-task. Within the page being read in a rendered view the frame is scrolled rather than the file reopened (a same-file open would not reload the frame, so it would not scroll).
- **The target is sent as file + headline line, not hash**: linking a mention in a headline changes hashes, and the id drawer is written below the target's headline, so its line survives the panel's own writes. After a write the file is read again where the reader was (source view: `pendingLine`; rendered: `keepScroll` restores the frame's `scrollY`), and the outline and link counts are re-asked.
- **Search tab**: the inspect dialog embeds the same panel for the row's heading; following an entry goes to the Files tab with the way back to the search page.

## Importing a Jupyter notebook

The new-file dialog (`components/NewFileDialog.tsx`) has a second mode, **Notebook** (toggle in its header; dropping a `.ipynb` on the dialog in either mode switches to it). Drop or pick the file, see what is in it, pick the folder with the same fzf-style picker, and **Import and open** posts `/import/ipynb` (`src/ipynbimport.ts`, via `ODb.doPost`) and opens the result in the Files tab through the dialog's `onCreated`.

- **The notebook is read in the browser first** (`src/ipynb.ts`, pure, `ipynb.test.ts`): title and where it came from (metadata, first heading, file name - the server's rule), language, cells by kind, pictures, errors - so "is this the one I meant" is answered before anything is sent. It only counts; the conversion is the server's.
- The file name defaults to the notebook's (`orgNameFor`: characters `nameError` refuses dropped, spaces as dashes), and the preview says where pictures will go (`<name>_files/`).
- No template and no title field in this mode: the notebook carries its title. "Replace it if it is there" is the endpoint's `Overwrite`; without it an existing file is a refusal whose message is shown.
- The dialog takes an optional `mode` prop for opening straight into the import; nothing passes it yet (the palette and App were left alone).

## Watched queries and the notification bell

Server side in `internal/app/orgs/CLAUDE.md` (**Saved-query notifications**). Pure logic `src/querywatch.ts` (`querywatch.test.ts`), endpoints and the shared store `src/querywatchApi.ts`, the stream helper `src/eventStream.ts` (focusApi.ts's EventSource fallback - credentialed, then not, then polling - factored out; focusApi.ts keeps its own copy).

- **One store** (`watchStore`, `useWatches()`) feeds the bell twice (breadcrumb row, `compact` in the phone header) and the Saved Queries tab: one `/events?kinds=querymatch` stream, polling every 60s live / 20s not, and on tab return. A `querymatch` is folded in at once (`withNotice`, deduped by id) and then the list is re-read.
- **`components/NotificationBell.tsx`** renders nothing until something is watched or found (like the focus chip). Unread notices are tinted; each gained heading opens in the Files tab through `focusNavigate` (the header is outside NavContext - the focus chip's window event, reused); headings that left are struck through. Mark all read, Clear, Manage watches (to the Saved Queries tab), and a Browser alerts switch (permission asked only from that click; off by default, key `watchNotify`).
- **`components/WatchToggle.tsx`** is the bell on each saved query row: one click watches with defaults (silent baseline on the server); on a watched query it opens options (left, desktop popup - says when the server has no notify plugin -, paused, label, stop). Renaming a saved query moves its watch; deleting one removes it.

## The Insights tab

`components/Insights.tsx`, model `src/insights.ts` (pure, `insights.test.ts`), calls `src/insightsApi.ts`; server notes under **Insights** in `internal/app/orgs/CLAUDE.md`. Two sections, the one last used remembered (`worg.insightsSection`); the estimate controls are remembered too (`worg.insightsAsk`).

- **Estimates**: range (presets are `from=-90d` or a clock-table block, or two dates), group by, a day of effort (24h org / 8h), include open tasks, an optional query (sent on Enter or blur - a half-typed query is a parse error). Tiles, the overall sentence, then a row per group: ratio chip, the weekly review's estimate-against-actual bar (`timeBar` from `review.ts`, so the two pages draw time the same way) and the spread on a log scale centred on 1× (`spreadSpan`: twice as long and half as long are the same size of miss). A row opens to its sentence and worst three. Worst over/under lists and the "not measured" counts open headings in the Files tab with a way back.
- **Dependencies**: problems grouped by kind (`groupProblems`: errors first), each a card folded to its title except errors. The chain is drawn in work order, each heading opening the file; a link that can be taken away (`canUnlink`: an AFTER, or an ORDERED chain NOBLOCKING would end; never in `ordered-stuck`, whose chain is the steps, not a wait to remove) gets a button running **ganttops' `unlinkOp`** on the heading read as a task (`asGanttTask`), with one Undo (`op.revert`). The page is re-asked after every write, because hashes after the edit change (see **Traps: hashes**).
- The server answers both; nothing is computed here that `orgs estimates`/`orgs deps` would print differently.

## Encrypted headings (org-crypt)

`src/crypt.ts` (pure, `crypt.test.ts`), `src/cryptApi.ts` (requests, the shared unlock state, the lock bus), `src/cryptFrame.ts` (the rendered view's DOM), `components/CryptReveal.tsx` (the block and the reveal/seal/save flows), `CryptDialog.tsx` (the passphrase), `CryptLockChip.tsx` (the toolbar lock). Server side: **Encrypted headings** in `internal/app/orgs/CLAUDE.md`.

- **Ciphertext is never drawn.** Source view: `OrgSource`'s `decor` prop replaces the armoured rows (from `cryptLines`, which must find the same rows as crypt.go's `cryptRange`/`isArmored`) with a locked `CryptReveal`, line numbers kept. Rendered view and the theme/book views: the armour is swapped in the html string before `srcDoc` (`replaceArmorHtml` - the exporter writes `—–BEGIN…` as two paragraphs, pinned with real output), and `wireCryptFrame` makes the placeholders clickable; a placeholder is matched to its heading by order, else by the heading above it, else refused. Search: the inspect dialog strips it and draws `CryptReveal` under the heading; the hover popup shows a padlock.
- **Click to reveal**: straight away when the server holds a passphrase (or is in key mode), else the dialog. The dialog's "keep unlocked for N min" appears only when `crypt.unlockSeconds` > 0, is off by default, and the passphrase is sent with `keep` only then; a wrong one keeps the dialog open with the reason. Passphrases go in POST bodies, never urls, in fields marked against autofill.
- **Plaintext lives in the revealing component's state only** (never localStorage, a parent, or a cache) and is dropped on Hide, on any lock (`lockNow`/`announceLock`, and the poll seeing the server's window close), when the browser tab is hidden, and after `revealIdleMs` untouched (the unlock window, else five minutes). An edit in progress is closed only by a lock.
- **Encrypt**: a faint lock on every heading line of the source view when hovered, and a loud "in the clear · encrypt" on one tagged `:crypt:` but still plain (also in inspect). Always asks for the passphrase twice in symmetric mode, even when one is held - which passphrase a note is under is worth saying. `/crypt/seal` tags it.
- **Edit** a revealed heading in place: saved through `/crypt/edit` with the revealed text's `Base`, so the server refuses a stale edit or a passphrase that does not open what is there. The text editor shows the armour as text, with a warning pointing at this instead.
- The toolbar's lock chip shows while a passphrase is held (with time left) or something is revealed; one click locks and hides everything.

## Refactor (tag rename, split, merge, list to headings)

`components/Refactor.tsx`, logic in `src/refactor.ts` (pure, `refactor.test.ts`), requests in `src/refactorApi.ts`. Server notes: **Structural refactors** in `internal/app/orgs/CLAUDE.md`.

- Four operation cards (the last used remembered, `worg.refactor.last`), one form each, **Preview**, then the plan: summary, warnings, the saved queries/filters/tag groups that name a renamed tag (reported, never changed), a chip per file and **the Diff tab's `DiffView`** on that file's `Before`/`After` (a new file is drawn against a missing side). Apply sends `applyRequest` - the request again with the preview's `Token`, each file's `Hash` as `Expect`, and a split's `NewId`. A stale refusal says which file changed and offers to preview again. ⌘/Ctrl-Enter previews, then applies.
- Any change to an input throws the preview away: a plan is only ever of the inputs on screen.
- Pickers are fzf-style lists under the box (↑↓, Enter): tags from `/refactor/tags` ranked with `fuzzy.ts` and their use counts shown; headings asked of the server loosely (`headingQuery`: each term's letters in order) and ranked here (`rankHeadings`); the split's folder reuses `newfile.ts`'s `rankDirs`/`dirLabel` (the file name defaults to `slugOf(title)`, which repeats the server's naming only to show it).
- **The palette** has an action per operation (`REFACTOR_OPS`, built in `Palette.tsx`): `requestRefactorOp` leaves the op in localStorage for the tab about to mount and fires `worg:refactor-op` for one already open.

## Opening on a heading from the url

`?open=<file>&line=<0-based>&heading=<text>` (`initialOpen` in `embed.ts`) starts on the Files tab jumped to that heading - what a click on a server notification sends (`common.HeadingLink`). It sets the initial `jump` state in `App.tsx`; `?app=` still picks the tab otherwise.
