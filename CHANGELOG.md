# Changelog

Each release is cut by `./makerelease.sh`, which tags the version below and
pushes the tag; `.github/workflows/release.yml` then builds and publishes it.
Add the next version's section here before bumping `VERSION` in that script.

## Unreleased

- Babel, closer to org:
  - `:var` indexing (`data[2,1]`, `data[,0]`, `data[1:3]`, `data[-1]`), lists
    and example blocks by name, and `other.org:name` across files.
  - `:colnames`, `:rownames`, `:hlines`: a table's header (a first row over a
    rule) is taken off before the block sees it and put back on a table
    result, as org does - previously it was passed as a data row.
  - go, emacs-lisp, awk and sqlite take `:var` (go as package vars with
    ob-go's main wrapping and `:imports`; elisp in a `let`; awk as `-v`; sqlite
    as `$name` substitution with tables as csv for `.import`, `:db`, and table
    results). `:stdin` feeds a table to a block's standard input.
  - `:prologue`, `:epilogue`, `:wrap`, `:post name(x=*this*)` and `:cache yes`
    (`#+RESULTS[hash]:`; a block whose code and inputs have not changed is not
    run).
  - A failing block writes its output and standard error as its result;
    `:stderr yes` adds standard error to a block that worked.
  - Noweb references are expanded when a block runs (`:noweb yes`/`eval`/...),
    `<<name(args)>>` inserts a block's result (when running and tangling), and
    org's noweb prefix repeats the text before a reference on each line.
  - `:results` words merge by group as in org, so a block's own `replace`
    overrides an inherited `append`; `:results link`/`graphics` link a file the
    code wrote itself, and `:results value pp` pretty prints python's value.

- `orgs detangle FILE` puts edits made in a tangled file back into the org
  blocks they came from (org's `org-babel-detangle`), as line edits, with the
  block's indentation and org's comma escapes kept; `-dry-run` previews.
  Needs the markers `:comments link` now writes the way org does
  (`[[file:x.org::*Heading][Heading:N]]` … `Heading:N ends here`, in each
  language's own comment syntax). `POST /detangle`.
- Auto-tangle on save for a file that asks (`#+auto_tangle: t`): only those
  files, only on a save (not when the server starts), only files whose
  content changed - and never over a tangled file edited by hand since orgs
  last wrote it (it stops and says so: detangle, or `orgs tangle -w`).
- Tangle fixes: named blocks (`#+NAME:`) were never tangled; `:tangle yes`
  wrote a file called `yes` (now `<file>.<ext>` as in org);
  `#+PROPERTY: header-args` was never read, and `header-args:<lang>` and
  headings' `header-args` properties are now inherited as org does; block
  indentation and comma escapes (`,*`) are taken off; `:comments link` wrote a
  link to the tangled file itself and no end marker; `:comments org`/`both`
  prose is written; `:noweb tangle`/`no-export`/`strip-export` expand and
  `strip-tangle` strips; a noweb expansion no longer adds a blank line after
  every reference; a file that comes out the same is not rewritten.

- Scheduled source blocks: a block or `#+CALL:` with `:schedule 1h` (`30m`,
  `daily 07:30`, `hourly`; inheritable through `header-args`) is re-run by
  the server and its `#+RESULTS:` updated with a line edit - a dashboard of
  disk use, CI status or a database query that keeps itself current. Needs
  `babel.enable`. A run that produces what is already there writes nothing
  (true of every babel run now, `C-c C-c` style runs included).
- Dynamic blocks and table formulas kept current: `:refresh 1h` on a
  `#+BEGIN:` line, `#+REFRESH: 1h` for a whole file (its dynamic blocks and
  every `#+TBLFM`), or `refresh.every` for every file. A table is written only
  when its values change, never just to realign it.
- Exports read a file with its dynamic blocks refreshed and formulas run, in
  memory, so an exported page (html, `orgs site`, typst and beamer pdfs) is
  never stale and exporting never rewrites the file. `refresh.onExport: false`
  turns it off; `orgs export -no-refresh` skips it once.
- `orgs refresh` lists everything on a timer (how often, last run, next, how it
  went); `orgs refresh run N|due|all`, `orgs refresh FILE [-blocks]`.
  `GET /refresh`, `POST /refresh/run`; a `refreshed` event says when one
  changed its file or failed.

- `orgs import` reads markdown and whole Obsidian vaults (`orgs import
  ~/Vault -o vault`): folders kept, front matter into the file's head
  (title, tags, aliases as `ROAM_ALIASES`, date, the rest as properties),
  `[[wikilinks]]` (by note name anywhere in the vault, `#heading`, `^block`,
  `|alias`) as relative `file:` links, `![[embeds]]` as pictures or
  `#+INCLUDE:`, callouts as special blocks, tables, tasks (and the Tasks
  plugin's dates), footnotes, math, `%%comments%%`, `#tags` as `FILETAGS`;
  the pictures a note uses are copied beside it.
- `orgs import todo.txt` and `orgs export -f todotxt <file|query>`: todo.txt
  in and out by the same rules (priority, dates, `+project` as the parent
  heading, `@context` as a tag, `due:`/`t:`/`rec:` as DEADLINE, SCHEDULED and
  a repeater), so a list survives the round trip.
- `orgs import calendar.ics` and `orgs export -f ics <file|query>`: events
  (VEVENT, with ranges, all-day spans and repeats as RRULE) and to-dos
  (VTODO: DUE, DTSTART, priority, status, completion). Time zones by name,
  Outlook's Windows names included; `-since` leaves old events out, and
  `-append` onto an existing file adds only events it does not already have
  (`:ICAL_UID:`).
- `orgs import ... -append` adds to the end of an existing org file;
  `POST /import/org` is the endpoint the client-side importers write through.
- `orgs csv <file.org | 'query'>` (`-q saved`), `orgs cols FILE -csv` and
  `orgs q NAME -csv`: a column view (with its rollups) or a query's headings
  as csv; `-columns`, `-sep semicolon|tab`, `-excel`, `-meta`, `-own`, `-out`.
  `GET /csv` is the endpoint.
- Exporters that need no settings (todotxt, ics, beamer, typst) answer
  whether or not the yaml lists them, and `/exporters` lists them.
- Fixed: `/columns?file=` with a name relative to the org directory read no
  properties, so every EFFORT was blank.
- `orgs print [week|day|month]`: the agenda as a paper planner, html or pdf
  (`-f pdf`, a local headless Chrome). A week is one landscape spread (days,
  overdue, undated tasks, habits, notes), a day an hour grid beside today's
  lists, a month a calendar. Themes classic, minimal, bullet (dot grid) and
  modern; `-paper a4|letter`, `-weekstart sun`, `-weeks/-days/-months N`,
  `-query`. `GET /agenda/print` lays out the same agenda as `GET /agenda`.
- `orgs kanban <board> -html` writes the board as one self-contained page
  (nothing fetched) to mail or publish: the same columns, order, colours,
  checklists and dates as the terminal board and worg, with a filter box,
  folding columns, cards that open, light/dark, a phone layout and print css.
  Themes default, paper and midnight; `-out`, `-open`.
- `orgs export -f beamer talk.org` writes a LaTeX Beamer deck the way ox-beamer
  does (`H:` frame level, `BEAMER_env` blocks, columns, overlays, notes,
  againframe, automatic `fragile`); `-f beamer-pdf` compiles it
  (`GET /beamer/pdf`). Presets for a deck with no `#+BEAMER_THEME`: clean,
  dark, metropolis, classic, academic, minimal (`-theme`, `-list-themes`).
- `orgs export -f typst notes.org` writes a Typst document; `-f typst-pdf`
  compiles it with `typst` on the server (`GET /typst/pdf`, pictures copied
  in, cached). Templates article, report (title page, chapters, running
  heads) and notes (compact, two columns) via `-theme` or `#+TYPST_TEMPLATE`;
  `#+TYPST_PAPER`, `#+TYPST_HEADER`, `#+TYPST:`, `#+BEGIN_EXPORT typst` and
  org's `#+OPTIONS` honoured. LaTeX maths through mitex, or plain typst maths
  with `#+TYPST_MATH: typst`.
- `orgs site <folder> -out ./public` (also `orgs export -f site`): a folder of
  org files as a static web site - a page per file with relative links (it
  works from disk or any path), `index.org` as the home page (and the site's
  title) or a generated one, tag pages, an archive, backlinks, per-page
  contents, an Atom feed, a sitemap, a 404 page, search with no server, and
  code coloured at build time. Seven themes, each light and dark: paper, ink,
  tufte (sidenotes), garden, docs (page tree), mono and journal (magazine);
  a theme is a folder (`templates/site/<name>/`), so your own template folder
  can add one. `site.yaml` or `-theme`, `-drafts`, `-base-url`, `-title`,
  `-clean`, `-open`, `-local`, `-list-themes`. `GET /site` is the endpoint.

- `orgs gantt <query> -csv` writes the scheduled plan as csv (ID, WBS,
  start/finish, predecessors by ID for MS Project or ProjectLibre); `-jira`
  writes a file for Jira's "Import issues from CSV" (epic/task/sub-task
  hierarchy, Blocks links, estimates, labels). `-out FILE` to write a file.
- `orgs invoice` and `orgs timesheet`: clocked time by client (a `CLIENT`
  property, or a client's tag; `-by tag|project|file`), priced with `RATE`
  and the new `invoice:` settings, rounded per line. Text, csv, html or pdf
  (`-f pdf`, printed by a local headless Chrome). Six themes - classic,
  modern, minimal, ledger, studio, letterpress - plus `-accent`, `-paper`,
  `-logo` and `-css`; a theme is one stylesheet in `invoice/themes/`.
  `GET /timesheet` is the endpoint behind both.
- `orgs export -f manuscript story.org`: a .docx in standard manuscript
  format, written by orgs (no pandoc). Styles `modern` (default), `shunn`
  (Courier, underlined italics), `novel` (cover page, chapters a third down a
  new page) and `reader` (a reading copy). Parts (`:part:`), chapters, scene
  breaks (deeper headings, `-----`, `***`), footnotes, word count and the
  `Surname / TITLE / page` header; contact details from the file
  (`#+ADDRESS`, `#+PEN_NAME`, `#+AGENT`...) or the `manuscript` exporter's
  yaml.
- `orgs export -f epub` now writes an EPUB 3 itself (cover, title page,
  nested contents, footnote pop-ups, series metadata) in four themes:
  classic, literary, modern, plain. pandoc's epub is `-f pandoc -to epub`.
  `GET /book` is the endpoint for both.

## v0.2.0 - released

- Login tickets: `POST /login/ticket` (logged in) hands out a one-use,
  one-minute link, `GET /login/ticket/<t>`, that logs a browser in to worg
  without a password. `orgs worg [View]` opens one.
- The command line makes its own token for a server on this machine when its
  config holds that server's keys (`orgJWS`, `orgJWE`, `orgSalt`) and it has
  no good token of its own - so the binary VS Code installs, which reads the
  server's `orgs.yaml` beside it, needs no login.
- `httpsOnly: true` serves https and nothing else: no http listener on
  `port`. Needs `allowHttps` and a certificate.
- `orgs cert` makes a self-signed certificate for this machine (localhost,
  127.0.0.1, ::1, plus `-host` names) at `servercrt`/`serverkey`, good for
  820 days; it leaves an existing pair alone unless `-force`.

## v0.1.0 - released

The first tagged release. Binaries for linux, macOS and windows (amd64 and
arm64), each a single `orgs` executable that is both the server and the
command line client.

### Server

- Watches org directories, parses them with go-org and serves a REST API
  (HTTP and/or HTTPS) with JWT logins, a per-user keystore and token expiry.
- Embeds the worg web app, the html/latex templates, the documentation
  themes and fonts, so a server needs no files beside the binary.
- `GET /version` reports the running version and whether a newer release is
  on github (cached 6h; `server.noUpdateCheck` turns it off).
- Exporters: html, markdown, latex/pdf, pandoc, mermaid, gantt, Confluence,
  reveal.js, impress.js, deck.js, webslides, tangle.
- Background plugins: Jira, Todoist, Trello, Google Calendar, ics calendars,
  GitHub/GitLab, git sync of the org directory, notifications, auto clock out.
- D&D character sheets, rulesets and session logs.

### Command line

- Tasks and planning: `agenda`, `capture`, `refile`, `search`, `find`,
  `grep`, `projects`, `plan`, `review`, `triage`, `month`, `daypage`.
- Time: `clockin`, `clockout`, `clocks`, `clocktable`, `focus`.
- Views in the terminal: `tui`, `kanban`, `gantt`, `cols`, `habits`,
  `insights`, `pres`, `drill`.
- Notes and links: `links`, `mentions`, `golink`, `rec`, `contact`,
  `tables`, `code`, `snip`, `export`.
- `orgs mcp` serves the org files to AI assistants over MCP.
- `-local` (or `-orgdir`) runs a server in-process for one command, for git
  hooks, CI and machines with no daemon.
- Every command takes `-json`, `-format`, `-dry-run` and `-no-color`.
- `orgs version` prints the client's and the server's version.
