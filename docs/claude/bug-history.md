# Bugs found while building orgs

History moved out of `CLAUDE.md`. Each section names the CLAUDE.md section it came from. The rules these bugs taught are still in CLAUDE.md.

## `orgs help`, and the commands that need no server

While in there: **the dispatch loop in `main.go` did not stop at the command it
found.** It walked `CmdRegistry` in map order mutating `args` as it went, so a
command whose *argument* was another command's name ran both, in whichever order
the map felt like that run - `orgs __complete tag ''` ran the completion and then
`orgs tag`.

## The commands that were endpoints with nobody to call them

Four bugs surfaced while building these, all of them silent and none of them new:

1. **`collectBlocks` in the tangler matched `case org.Block`**, and the parser
   only ever produces `*org.Block` - so no block was ever collected and tangling
   any file answered "nothing to tangle" however many `:tangle` headers it had.
   `/tangle` had never worked. The same pointer trap as `*org.Headline`.
2. **`ChangeBody` deleted every child heading.** Its loop preserved
   `case org.Headline`, which never fires, so appending one line of note to a
   project heading took the project's tasks with it and answered
   `{"status":true}`. It dropped the planning line too, since SCHEDULED/DEADLINE
   are SDC children.
3. **`IsBlockedProject` returned the opposite of its name** - `childHasNext`, so
   it answered true for exactly the projects that were *not* blocked. docs.org
   has always described it correctly ("DOES NOT have a child marked NEXT"). It
   also required an argument it never read, so `IsBlockedProject()`, which is how
   the documentation writes it, panicked.
4. **`/hash/{hash}` failed on a server that had not answered a query yet.**
   Sections are registered into `ByHash` lazily, as queries walk them, so on a
   freshly started server the index is empty and every hash lookup answered "no
   heading with that hash" - which reads like a stale hash rather than an empty
   index. `FindByHash` now builds the index on a miss and caches it against
   `ReloadIndex`, like the link and record indexes. This is why
   `orgs search … | orgs tag +x -` failed under `-local`, which is a fresh
   server by definition, and it was the same for worg and `orgs mcp` against a
   server that had just started.
5. **go-org could not parse a repeater on a planning line.** `CompileSDCRe` built
   its regex with `nocookie`, which wanted the closing bracket immediately after
   the day - so `SCHEDULED: <2026-09-28 Mon .+2d>` did not match the
   planning-line pattern at all: **every habit's schedule was invisible**, the
   date never reached `Headline.Scheduled`, and the line was re-parsed as body
   text. `ToDate` then dropped the cookie on the way out, so even a parsed
   repeater was lost on the next write. Both fixed in the go-org checkout the
   `replace` directive points at, pinned by `TestSDCKeepsItsCookie`.

## The links pane

While building it: **go-org gives every inline node the position of the node it was parsed in**, so two links in a two-line paragraph both claimed the first line. `fixLinkLines` in `links.go` now finds each link's real line in the file — in document order, with a cursor that never goes backwards (so the same url twice resolves to the first occurrence and then the second), starting from the parser's row, which is right or early but never late. A link it cannot place keeps the row it had. This was wrong everywhere the line was used: the links tab, `orgs links`, the editor jump. The pane drawing the file around the link is only where it became visible.

## What DONE does

Four bugs surfaced building this, all silent, none of them new, and the first
three in the vendored go-org:

1. **`trimFastTags` only stripped a three-character cookie.** `TODO(t)` worked;
   `NEXT(n!)` and `WAITING(w@/!)` - org's own logging notation, and the reason
   cookies matter here at all - were left on the keyword. Since keywords are
   matched on a headline by literal prefix, `* NEXT Write the thing` came out
   with **no keyword at all**, titled "NEXT Write the thing": not a task, not
   findable by keyword, never in an agenda. Every file using the notation the org
   manual documents was affected. `ParseTodoStates` had the same bug on its own
   side, so `/status` offered `NEXT(n!)` as a keyword to write.
2. **Only one planning keyword per line was read.** Org keeps all of a heading's
   planning on one line, and `DEADLINE: <...> SCHEDULED: <...>` - which Emacs
   writes whenever a heading has both - lost the deadline entirely. So did
   `CLOSED: [...] DEADLINE: <...>`, which is what stamping CLOSED produces. One
   line in is still one line out (`SDC.Others`), or the first rewrite would split
   every done heading in the file.
3. **A habit's `.+1d/3d` did not parse.** org-habit's slack half was absent from
   the cookie pattern, so the whole timestamp failed to match: the planning line
   was re-read as body text and the heading came back with `Scheduled` nil - the
   same silence `CompileSDCRe` used to produce for a plain repeater. Orgs' own
   timestamp pattern in `logbook.go` had to learn it too, where the symptom was a
   habit that never repeated. Fixing the parse meant also writing the slack half
   back, or it would have introduced the data loss it removed.
4. **`parseHabitCompletions` matched `org.Drawer`**, and the parser only ever
   produces `*org.Drawer` - the same pointer trap as `*org.Headline` - so the
   loop never fired and every habit came back with no completions whatever was in
   its logbook. It also looked only for `LOGBOOK`, which is wrong for anybody who
   pointed the drawer elsewhere.

On the real corpus (541 files, 7,187 headings) the three parser changes make
**zero** difference - they only widen what parses - which is how they were
checked: the same walk under both versions, diffed.

## A file that is a D&D book

Getting the example file (`dnd_pdf_example.org`) to compile needed four fixes in the latex exporter, all of which were breaking every dndbook export, not just this one:

1. **`MakeTemplateRegistry`'s parameters were the wrong way round.** The one caller passes (class, default) and the signature said (default, class), so a dndbook document read `book_templates.yaml` *first* and its own file second. Nothing failed outright, because the generic file has a worse answer for everything the class defines - tables came out as plain `tabular`s in a class whose whole point is that they should not.
2. **A dndbook table is a `DndTable`.** The writer already built the column spec out of tabularx's `X` columns for this class, which mean nothing to a `tabular` - `\begin{tabular}{ XXX }` is an "Empty preamble" error. `dndbook_templates.yaml` now carries the `default` table template that wraps them in `DndTable`, and drops the `|---+---|` rule rather than turning it into an `\hline`, because that environment draws its own header.
3. **`\par` inside a macro argument ends the run.** The paragraph template writes `\par` before every paragraph and the writer has always had a `docclass != "dndbook"` guard saying this class should not get it - a guard that could never fire, because the template always won over the branch holding it. The class now has its own paragraph template, and `MONSTERTYPE`'s content is squeezed to one line (`oneLine`), because `\DndMonsterType` is not `\long`.
4. **pongo2 escapes for html unless told not to.** Every latex template is now rendered inside `{% autoescape off %}` (`OrgLatexWriter.render`). An apostrophe arriving as `&#39;` is not cosmetic: `&` is LaTeX's column separator, so a monster whose text mentions "the creature's turn" ends the run with "Misplaced alignment tab character &". Saying it once there beats `| safe` on every value of every template, which is what the dnd templates were quietly relying on somebody to remember.

## The three documentation themes

While building them: **`#+TITLE:` had never reached the html template.**
`ExportToString` wrote it into the caller's `props` map and the template
is rendered from `self.Props`, so every exported page was titled
"Schedule" - `ValidateMap`'s default, and a leftover from the agenda. It
is now set on `self.Props` on every export (the exporter is shared between
requests, so a title set only when a file names one would keep the last
page's), and a file naming no title is named **after itself** rather than
after the agenda. That matters more here than elsewhere: in these two
themes the title is the name at the head of the rail.

## The `docs` theme

Two things the theme needed from the exporter, and both were bugs on their
own:

- **`HighlightCodeBlock` threw the block's language away.** It wrote
  `<pre><code  >` whatever the block declared, so highlight.js guessed from
  the text - and a short block of json guesses css as readily as json. The
  class is written now (`language-<lang>`), and the docs page maps the aliases
  org uses (`sh`, `emacs-lisp`) and calls a language hljs has never heard of
  `plaintext` rather than letting it guess. It also registers a small `org`
  grammar, since nineteen blocks in docs.org are org and hljs has no org.
- **`#+HTML_HIGHLIGHT_STYLE:` had never worked.** `ExportToString` wrote the
  value to `hljsstyle`; every template reads `hljs_style`. Worse, writing the
  first key stopped `ValidateMap` filling in the second, so a file naming its
  own highlight style came out with **no** stylesheet rather than the wrong
  one. `hljs_style_default` was added beside it so a theme can tell "nobody
  asked" from "this one was asked for" - the docs theme loads github and
  github-dark together and switches them with the page, and gets out of the
  way entirely when the file named one.

## The search tab's file affordances

While in there: the Headline column's sort arrow was sorting on `id`, which a search row does not have, so every comparison came out equal and `stableSort` kept the order it was given — the arrow turned round and nothing moved. It sorts on `Headline` now.

## Pictures: pasting one, and filing a chart

Two things in the html exporter were wrong and are fixed here, both surfaced by this:

1. `WriteRegularLink` chopped a fixed seven characters off `l.URL` assuming `file://`. Org writes `file:path` too - and that is what a link somebody typed looks like - so `file:images/x.png` became `ages/x.png`. It reads the target properly now.
2. The default branch built `http://localhost:<port>/images/...`. That works on the machine running the server and nowhere else, which now matters: there is a QR code for opening worg on a phone. A page this server rendered and is about to serve gets a path of its own. The `filelinks;`, `httpslinks;` and `httplinks;` opts are unchanged - they are for a file on disk and for vscode.

## Gantt charts

Everything a gantt client *changes* goes through endpoints that already existed (`/headline/change`, `/date/change`, `/property`, `/status/change`, `/delete`), which is why three bugs in those surfaced while building it and are fixed here:

1. `subtreeEndRow` in `refile.go` - `Headline.GetEnd()` under-reports for a heading whose body is only a planning line and a property drawer, because go-org keeps the drawer in `Headline.Properties` rather than among the body nodes it measures. `/delete` left the drawer behind, orphaned under the parent, and `/gantt/add` wrote new tasks *into* the last child, between its date and its drawer. Both now take the end of the subtree off the file's own lines - the next heading at the same level or above - and that only ever extends the range go-org gives, never shrinks it.
2. `SetProperty` in `todo.go` - writing a property with an **empty value now removes it** (and the drawer with it, when it was the last one) rather than leaving `:AFTER:` sitting there with nothing after it. Undo writes the old value back, and for a property that was not there before, the old value is nothing.
3. `noAuthUser` in `auth.go` - with `noAuth: true` the authenticate middleware was skipped entirely, so no username reached the per-user endpoints and stored queries, kanban boards and capture templates all answered 401. There is now a middleware either way, and with authentication off everything is done as `local`.

## Moving headings: refile, copy and archive

Building it surfaced three pre-existing bugs, all of which were corrupting files through the *old* `/refile` endpoint long before any of this existed. `internal/app/orgs/refile_test.go` pins all three:

1. **`formatHeading` wrote a subtree once per level.** It recursed over `sec.Children` on top of the recursion `WriteHeadline` already does (`WriteNodesLB(1, w, h.Children...)`, and a headline's children include the headlines nested in it). Refiling a heading with a child and a grandchild wrote the child twice and the grandchild three times. The recursion is gone.
2. **The delete was measured against the file as it was before the insert.** `DeleteTree` re-reads the file - which `InsertSection` has just rewritten - but took its rows and its level off the parse tree beforehand. Two things had moved: every row below the insertion point, and the source's own `Headline.Lvl`, because `formatHeadingAt` calls `fixUpLevel` on a `CopySection` that *shares* the `*org.Headline` (it has to - the headlines nested in `Headline.Children` are those same pointers, which is what makes renumbering the subtree for writing work at all). `subtreeEndRow` then walked forward to the next heading at the *destination's* depth and ran straight through the source's siblings. Refiling `Alpha` out of `Projects/Kitchen` up into `Inbox` deleted `Kitchen`, `Gamma` and `Gamma child` with it and reported success. `Refile` now reloads the source file and finds the heading again by its outline path before deleting it, which answers the rows and the level at once.
3. **Archive had never worked.** `FindArchiveTarget` built a target of type `"file+heading"`; `GetFromTarget` only knows `"file+headline"`. One word.
