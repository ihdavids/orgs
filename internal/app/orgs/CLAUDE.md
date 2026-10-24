# Notes on the server (`internal/app/orgs`)

Loaded when working in this directory. References like **Traps: line edits** point at the Traps list in the root `CLAUDE.md`.

## `GET /events`, and the things that are live

`internal/app/orgs/events.go`: server-sent events on one long GET (the old websocket API was commented out years ago).

- `orgs watch` prints each change; `-exec 'make notes'` runs something (`{}` becomes the file), `-file` narrows, `-once` waits for one.
- `orgs clocks -w -short` is a one-line status, redrawn on clock change *and* once a minute, because elapsed time moves on its own.
- The agenda redraws when a file changes (`-no-live` stops it) via `app.QueueUpdateDraw`, because tview owns the screen and drawing from a goroutine is a race.

Stream rules:
- A subscriber not reading is **dropped, not waited for** - a sleeping laptop must not stall the file watcher.
- An event says **what happened, never what the thing now is**.
- **Heartbeat every twenty seconds**, because proxies close idle connections and the reader cannot tell that from a quiet database.
- **Reloads coalesced over 120ms**, because one save is often three filesystem events and three redraws flicker.
- Client half `internal/common/events.go` **reconnects with backoff**, since a day-long watch will outlive a server restart.

## The agenda

`GET /agenda` and `GET /agenda/nodes` (`agenda.go`, wire types and the rules in `internal/common/agenda.go`, pinned by `agenda_test.go`) say what lands on each day, so worg, vscode, OrgExtended and `orgs agenda` stop working it out four ways. `OnDate()`, `Today()`, `Yesterday()` and `ThisWeek()` (`IsOn`/`IsIn`, now in `agenda.go`) answer from the same engine with today = the real today, so a day view built from `IsTodo() && OnDate(...)` and `/agenda` agree.

Rules (org's agenda):
1. **A plain active stamp** in the headline or the heading's own body lands on its day; a child's, a drawer's, an inactive one never. Read `Headline.Timestamps` (go-org, every own active stamp, drawers excluded), not `Headline.Timestamp` (the *last* one, drawers included, kept for old callers).
2. **Repeaters** (`+`, `++`, `.+` alike) land on the base day and every interval after, never before. Months and years are counted from the base each time and clamped to the month: Jan 31 `+1m` is Feb 28 then Mar 31; Feb 29 `+1y` is Feb 28 in a common year. (`repeater.step` in `logbook.go` uses `AddDate`, so *marking it done* moves Jan 31 to Mar 3 - a known disagreement.)
3. **Ranges** land on every day spanned: times on the first day (start to `FirstEnd`, else to `23:59` - clipped to the day, so a client drawing hours knows it runs on) and last day (`00:00`/`LastStart` to end), all day between.
4. **SCHEDULED, not done, past**: its own day plus **today only**, `Late` = days since the date in the file (a repeat that also falls today is replaced by the late entry, not doubled).
5. **DEADLINE, not done**: its day plus today while inside the warning (`-Nd` on the stamp, else `agenda.deadlineWarningDays`, unset = 14, `0` = its day only) with `DueIn`, or past with `Late`. The deadline's own day also carries `DueIn` while warning.
6. **Done** (a finished keyword of the heading's *own file*): only the days its dates name (repeats included), no carry, no warning. **CLOSED** only with `closed=t`.
7. **Archived trees** (tag ARCHIVE, here or above) are left out unless `archived=t`, as org skips them (`org-agenda-skip-archived-trees`): a task filed away years ago must not come back as today's overdue item (`TestAgendaSkipsArchivedTrees`). `/agenda/nodes` does not skip them: a list view's query says what it wants.
8. **Skipped subtrees**: a `COMMENT` heading and everything under it never lands (`agendaCommented`), like an archived tree.
9. **One entry a day per stamp**: a repeating range longer than its repeat overlaps itself; the earliest occurrence speaks for the day. **`+1h`** lands every day from its base (the agenda places days; go-org only parses an hourly repeater since this change - before, such a stamp was not a stamp at all). An unknown unit still lands on its base day.
10. **`-0d` is no warning** (the deadline's own day only). go-org keeps the number as written in `OrgDate.WarnNum`, because an rrule cannot hold an interval of 0: `-0d` used to read as `-1d` *and be written back as `-1d`*.
11. A day sorts timed entries by time, then all day by kind (deadline, scheduled, timestamp, closed), priority (none = B), title.

Days are **day numbers on the calendar** (`agendaDayNum`), never instants, so DST and time zones do not shift a date. The answer is cached on `ReloadIndex` + expanded query + every parameter (`today` included) + the minute (running clocks move); see **Traps: caches**. Sections are registered (`registerAllSections`) before the walk, so node hashes work in `/hash` and the edit endpoints (`TestAgendaHashesAreLive`; see **Traps: hashes**). The query goes through `evalEachSection`, the walk `/search` now also uses (`queryEachSection` = filters expanded + that).

Other users of the engine: `ThisWeek()` is the seven days ending today (it was eight: `IsIn`'s end is exclusive); the `notify` poller asks `ODb.Agenda` for today and announces only today's own timed, unfinished occurrences (it read `Todo.Date`, which is nil for a deadline-only heading - a crash once `OnDate` returned those - and is an overdue item's *old* time); `Todo.Date` is the heading's *first own* stamp (`Headline.Timestamps[0]`), no longer the last one seen with a drawer's counted.

Traps hit:
- **A stamp in the headline is part of its title**: `Todo.Headline` of `* Meeting <2026-10-12 Mon>` is `Meeting <2026-10-12 Mon>`, as `/search` has always said.
- **`org.ParseTimestamp` does not join `<a>--<b>`**; only the inline parser does. Build a range through a parsed document.
- **`OrgDate.Before`/`After` were inverted** (Before meant "starts at or after") and `DateTimeInRange` read `HasEnd() ||`. Fixed in go-org; the only callers were the old `IsOn`/`IsIn`.
- **The SDOC marker must be `/* SDOC: API` on one line and `EDOC */` on one line.** The `/*` newline `SDOC: API` form (gantt.go, much of rest.go - 89 blocks) is silently not extracted by docex.
- `go test` here needs `-vet=off` (pre-existing vet failures in `api.go`, `tableops.go`, ...).

## The query functions the review needed

In `todo.go`: `HasScheduled()`, `HasDeadline()`, `HasTimestamp()`, `HasAnyDate()`, `DeadlinePast()`, `ScheduledPast()`, `HasClock()`, `DaysOld()`, `OlderThan(30)` - answering "does this have a date at all" and "has it gone past", which the language could not ask.

In `checklist.go` (not `todo.go`): `HasChecklist()`, `ChecklistDone()`, `ChecklistCount()`, `ChecklistLeft()`, because they must count boxes exactly as the *write* does - `orgs check 3` and `HasChecklist()` disagreeing would tick the wrong line.

Deliberate:
- **Past means before today**, counted in days not instants, so a deadline of today is due, not overdue.
- A dateless heading has `DaysOld() == -1`, so `OlderThan` is never true for it.

## Voice notes and go-whisper

`internal/app/orgs/voice.go` neither records nor transcribes: a client records, the server keeps the audio, and transcription is a request to a [go-whisper](https://github.com/mutablelogic/go-whisper) server (`/api/whisper/model`, `/api/whisper/transcribe`).

`internal/app/orgs/whisperd.go` **runs that server**. Two settings are the whole configuration:

```yaml
voice:
  models: "/path/to/whisper/models"
  port: 8081
```

`StartWhisper` (called from `StartServer`) spawns and supervises `gowhisper run --http.addr localhost:<port> --models <dir> --whisper.gpu`. Other `voice:` keys default (`bin`, `gpu`, `args`, `url`, `model`, `language`, `dir`, `timeout`, `maxMb`, `tags`, `target`). `voice.dir` is relative to the first orgDir, so audio sits beside the heading linking it.

Do **not** link whisper: it needs cgo and a built whisper.cpp, forcing CMake on every build. Supervising the binary keeps `go build ./...` working on a bare checkout.

- A whisper **already on the port is adopted**, not restarted (a hand-run one works; a `kill -9` orphan is reused), and orgs never stops an adopted one.
- The child is stopped **on a signal as well as on clean exit**: `StartServer` exits via `log.Fatal`, which does not unwind, so a `SIGINT`/`SIGTERM` handler is the only place to catch ctrl-c; otherwise each run leaves a 500mb model resident.
- Readiness is **polled** for up to three minutes. `/voice/config` reports `state` (off/starting/ready/adopted/failed) plus whisper's output tail, because "still loading" and "not installed" need different messages.

Note side:

1. **Save the recording before anything else; return the transcript before writing anything**, so no failure (whisper down, model loading, timeout) loses the words. `POST /voice/note` writes the *text it was sent*, never a fresh transcription, so the file matches the screen.
2. **Nothing is converted**: go-whisper decodes via ffmpeg (webm/opus, m4a, wav). The extension follows the recording; the `filename` field tells whisper the container.
3. **`/api/whisper/model` returns a bare JSON array**, not the documented object with `models`. `whisperModels` reads both, since a wrong guess silently looks like nothing installed.
4. The heading is **lines spliced in** (`voiceNoteLines` + `insertLinesAt`) (see **Traps: line edits**).
5. A recording id **is its filename** (`voiceIdRe`): no index to drift, and any other path shape is refused rather than joined.

## Per-user extensions

`internal/app/orgs/extensions.go` is a per-user store beside the config (`orgs.yaml` → `orgs_extensions.yaml`): stored queries, the user's capture templates, kanban boards. Handlers take the username from the auth token; no API has a user parameter.

Kanban boards:

- A `KanbanBoard` holds **no cards**, only a query and how to draw it, so it is never stale and deleting one touches no heading.
- `POST /ext/kanban/boards` replaces the whole list in one write, since rename and reorder change the list; delete-plus-add could leave it half written if the second call is lost.
- `layout` picks columns of cards or one grouped list with a column per field (`listFields`: a worg key or `prop:NAME`). Both are **layouts over one model** in `kanban.ts` (grouping, row order, what a drag writes), so a list drag into Done equals a card drop in the Done column.
- In the list the **todo keyword leads every row** and is a control: clicking asks `/status/{hash}` for that heading's file's keywords, never the board's columns. Hence it is excluded from `listFields` (`PINNED_FIELDS`).
- **Section order is column order**: reordering a board whose columns came from the server's keywords makes them the board's own.
- Keyword look (icon, colour, light and dark ink) is `components/statuslook.tsx`, shared with the search table so a keyword never looks two ways.
- **A setting that is not a field of `KanbanBoard`/`KanbanColumn` in `extensions.go` is silently dropped on save** (bitten twice): Go unmarshals into the struct, so a worg-only field lives until the next read-back, with no error. Column `aliases`, `headerKey`/`headerColors`, `folded`, and `layout`/`listFields` exist for this. Add the Go field in the same change as the TypeScript one.

Dropping a card writes a property. `SetProperty` (`todo.go`) used to nil-deref on a heading with no drawer (`Headline.Properties == nil`; the `if props == nil` check took a field's address first and never fired). It now creates the drawer, printed directly under the headline.

## Users, and the keystore

`internal/common/keystore.go` is the credential store; `orgs user` (`cmd/oc/commands/user/`) writes it; `internal/app/orgs/user.go` is the server half. It is in `internal/common` because CLI commands cannot import the server package and both must agree on what a password is.

`keystore:` used to be documented but never read (`DefaultKeystore()` alone set `currentKeystore`, so every server had only `admin`/`default`). `LoadKeystore()` now runs in `StartServer` right after `Conf()`, not before. `DefaultKeystore()` remains as the pre-config bootstrap, because a failed config load with a nil keystore would panic instead of reporting.

**Passwords never reach the server.** The client asks `GET /salt?user=<name>`, hashes with `common.ClientHash`, and sends that; the server re-hashes with its `orgSalt` (`common.StoredHash`) before comparing. So:

- a **stolen keystore is not a set of logins** (not what `/login` accepts);
- a **captured hash is** a login, so this does not replace TLS; it protects the reused, longer-lived password. `requireHashedLogin: true` refuses cleartext logins; off by default so old clients still work, and each cleartext login is logged until it is on.

1. **`common.ClientHash` is repeated in `worg/src/clienthash.ts`** (the browser cannot delegate the one thing that must not travel). Prefix, separators and order must match; `TestClientHashVector` and `clienthash.test.ts` pin one vector. Change and run both (see **Traps: logic said twice**).
2. **`crypto.subtle` needs a secure context**: worg over plain http from another machine cannot hash and says so in the console rather than send a fake hash. Use https or localhost.
3. **`GET /salt` answers for nonexistent users** with a stable decoy from `orgSalt`, because it needs no credentials and telling them apart would leak the user list.
4. **One comparison covers both legacy cases** (`credMatches`): cleartext from an old client, and a hand-typed cleartext keystore entry. Always a fixed-length hash, constant time. `password: hunter2` keeps working; `orgs user passwd` converts it.
5. **The username is part of its hash**, so equal passwords differ on the wire, and a rename would break the password: no rename, remove and re-add.
6. **`orgs user` writes a file, not an endpoint**, because an endpoint writing the keystore grants access to anyone with some. It runs on the server's machine and the server reads the file at startup, so changes need a restart. It is `commands.Offline`, since a machine being set up has no server.

The exception to 6 is `orgs adduser` (`cmd/oc/commands/adduser/`, `internal/app/orgs/adduser.go`, `POST /users/add`). Some access is not enough: `Cred` carries `Admin`, and only an `admin: true` account may add one. The built-in `admin` is an administrator; `orgs user admin <name>`, on the server's machine only and deliberately not over the wire, is the only way to make another, because an admin who could promote could promote an account they just added.

1. **The password still does not travel**: the client salts for the new user and sends `ClientHash(name, salt, password)`; the server re-hashes with `orgSalt`. `AddUser` refuses anything but a client hash (`/login` must accept cleartext for old clients; nothing predates this endpoint).
2. **The account works at once**: the server writes the keystore it holds, unlike `orgs user add`, which says "restart the server".
3. **Three refusals, each with a reason and the fixing command** (not a bare 403): caller not admin; `noAuth: true` (no authenticated admin, and the account would outlive the setting); built-in keystore (no file, `Save` is a no-op, gone on restart).
4. **An add never replaces**; resetting an existing password would be a takeover.
5. **`SetPassword` keeps `Admin`** (it used to replace the whole `Cred`, silently demoting). `TestPasswordChangeKeepsAdmin` pins it.

**Deleting org files** (`deletefile.go`, `GET`/`POST /file/delete`) is for an administrator or an account with `delete: true` (`Cred.Delete`, `MayDelete`; granted only on the server's machine with `orgs user candelete <name>` / `nodelete` - `orgs user delete` already meant removing a user). `SetPassword` keeps `Delete` as it keeps `Admin`. A path must be **exactly** one the OrgDb holds (`FindByFile` also answers for a lower-cased or similar name, which must not choose what gets deleted) and inside `orgDirs`; each file goes with `os.Remove` + `UnloadFile`, and one failure does not stop the rest.

`YamlKeystore` has a mutex: handlers already mutated it (each login records a time and saves), and a second writer made concurrent map writes likely.

`WarnOnInsecureCredentials` prints a block, not a line (to stand out from startup chatter), when any account has the default password, any is cleartext, or `noAuth` is set, naming the accounts and the three fixing commands. A configured keystore that is absent, unreadable or empty falls back to the built-in account rather than refusing to start (an unstartable server cannot be set up), and the block states this as `Why:`.

## Backlinks and the link graph

`internal/app/orgs/links.go` walks every `[[target][description]]` link, resolves it, and indexes both ends. Endpoints: `/links` (backlinks for a file), `/links/graph` (around a file or whole database, file or heading granularity), `/links/stats` (per-file counts). Wire types `internal/common/links.go`; worg client `components/Files.tsx` and the plain-svg force layout `components/LinkGraph.tsx`.

Cached against `OrgDb.ReloadIndex` (see **Traps: caches**).

1. Sections register lazily (`ScanNode`'s `RegisterSection` call is commented out; see **Traps: hashes**), so `buildLinkIndex` first walks every file into the registry, then resolves.
2. Per **Traps: column-zero drawers**, links go to the last heading above them, and ids come from a local `idIndex` that also reads hoisted property drawers.

Query functions: `HasBacklinks()` (optional floor `HasBacklinks(2)`), `BacklinkCount()`, `HasLinks()`, `LinksTo(re)`, `HasBrokenLinks()`, in `todo.go`, built on `BacklinksTo` / `LinksOut` at the foot of `links.go`.

- A link naming a **file** counts against no heading in it, or every heading there would claim a backlink.
- `LinksTo` matches the link text, its description, and the file and headline it lands on ("links to notes.org" and "to the migration heading" are both meant).
- The first heading evaluated builds the index; a query without link functions never does.

The worg file view has three buttons: the html exporter's page; the raw text with org colouring (`components/OrgSource.tsx`, client-only); and for files `/dnd/characters` reports, the `dndsheet` sheet fetched from `/file/dndsheet` as a string, not written to disk.

### One heading's links, and unlinked mentions

`mentions.go` (wire types `internal/common/mentions.go`, pinned by `mentions_test.go`): `GET /links/heading` (links into/out of one heading, or a file, each with its line as `Context` and cut at the link as `Before`/`Shown`/`After`), `GET /mentions` (plain-text occurrences of a heading's title and `ROAM_ALIASES`/`ALIASES`, or a file's `#+TITLE`/file name and preamble aliases), `POST /mentions/link`. A target is `hash=`, or `filename=` with optional `line=` (the heading that zero-based line is under; above the first heading = the file) or `heading=` (text). Clients: worg's `LinksPanel.tsx`, `orgs backlinks`/`orgs mentions`.

- **Matching is a walk of the raw lines** (`scanMentionLines`), not the AST: a link is written at a byte position, and go-org gives inline nodes their paragraph's row. Excluded: links, bare urls, timestamps, verbatim/code, inline src, cookies, footnotes; src/example/export/comment blocks (quote/verse are prose); PROPERTIES and LOGBOOK drawers; `#+` lines, comments, fixed width, planning and CLOCK lines; headline keyword/priority/tags; the target's own subtree (or whole file). Case-insensitive, whole word (only checked at a letter edge, so `C++` works), whitespace runs match, longest term first.
- **The guard**: a title under three letters or made only of stop words (`mentionStopWords`: Notes, Meeting, Ideas, …) is not searched unless `all=t`; `Target.Skipped` says why. Aliases are taken at their word (min two letters).
- **Cached**: the per-file scan via `FileParts` (`mentionParts`), answers in a map reset on `ReloadIndex` (see **Traps: caches**). Results are sorted file/line/col, then cut at `limit` (default 500, `Total` before the cut).
- **Linking is a line edit per mention**, applied bottom-up/right-to-left, one `writeLines` per file. Each mention's `Text` must still be at `Line`/`Col` (a *byte* offset) and not inside a link/code span now, or it is refused with a message while the others go ahead. **Every mention is checked before anything is written**, so an all-stale request changes nothing, not even the id.
- **The id comes first**: a heading without `:ID:` gets a uuid via `ensurePropertyDrawer`/`setPropIn` (drawer at its own indent). That adds lines under the target's headline, so mentions further down the *same* file are shifted by the delta (see **Traps: re-measure**). A file target links `[[id:…]]` when its preamble drawer has an ID, else a relative `[[file:./x.org][text]]`.
- **A mention in a headline changes that heading's hash** (and its children's). worg's panel addresses its target by file + headline line for this reason; the id drawer goes *below* the headline so the line does not move.

## Todo keywords over the wire

`GET /status` returns the server's keywords: `defaultTodoStates` split at `|` (active before, finished after), in configured order, which is reading order. `GET /status/{hash}` answers from that heading's file's `#+TODO:` when present. worg's kanban builds default columns from the global list.

## The habit tracker

`GET /habits` (`internal/app/orgs/habit.go`) returns every `:STYLE: habit` heading with its current run, best run, how much the cadence asked for was done, and one entry per day for eight weeks. worg's Agenda draws it above the calendar (`components/HabitTracker.tsx`, model and tone in `src/habits.ts`), folding to one line and a progress bar.

History is the logbook's `State "DONE"` lines, which `ChangeStatus` writes (before that only Emacs habits had any). Ticking is `POST /status/change`: the server moves the repeater and writes the line; the next `/habits` read fills the square.

Rules, all about **not misreporting someone's habits**:

1. **Three day states**: `done`, `miss`, `ok` (inside the grace the repeater grants). `.+1d/3d` undone yesterday is in hand; a gap would make a kept habit look failing.
2. **The day a habit comes round is never a miss**; off by one paints every habit red each morning.
3. **No day before the first completion is a miss**, or a new habit opens with eight weeks of red.
4. **`Rate` counts from the first completion**, not the window edge (started Tuesday and kept: 100%, not 4%).
5. **"Best ever" needs a previous run to beat** (`Total > Streak`), or a first run says it daily from day two and the words mean nothing. 40 needs no adjective.
6. **A broken run never says what it was**: "start again", not "you lost a 40 day run".

`habits.ts` is its own module (like `chartspec.ts`) because tone bugs do not show in screenshots; tests pin its branching. The per-file half of the walk is cached with `FileParts`; the midnight-dependent half (window, streaks) is not.

### Taking today's tick off: `POST /habits/untick`

A first version restored a saved copy of the file and **was thrown away**: the copy is valid only while the file is untouched, and **habits live many to a file**, so a second tick (or any write from worg, a clock, Emacs) made it stale. This applies to any undo buffer over org files.

`internal/app/orgs/habituntick.go` asks the file whether a completion is recorded today, so nothing goes stale: it works on Emacs ticks, double ticks, after a restart, and under `-local`. Clients: `orgs habits untick [name]` and the picker's `ctrl-u`. "Nothing recorded for today" is an answer, not a failure, so pressing twice is harmless.

1. **A day is the unit**: *every* completion recorded today comes off (two ticks in an afternoon are one square).
2. **Exactly what fills the square comes off**: the lines `parseHabitCompletions` counts, `habitDoneRe` and all, or a square cannot clear or a line vanishes with nothing changing on screen.
3. **The date goes back only on evidence it moved today** (`:LAST_REPEAT:`); otherwise it is left alone rather than inventing a schedule.
4. **A `.+` date is rebuilt from the previous completion, not stepped back.** `.+2d` is two days after actually doing it, so the prior date is the previous logbook completion (after removing today's line) plus two days. Stepping back lands on today: right only when on time, it shifts an early tick by a day and erases that an overdue one was overdue. `+2d` steps back exactly; `++2d` may have taken an unknown number of steps, so it also steps back. `habituntick_test.go` pins early, on-time and overdue.
5. **The previous keyword comes from the removed line**: a non-repeating habit sits on DONE with `CLOSED:`, and its state line's `from "NEXT"` says what it was, so nothing remembers or guesses. `CLOSED:` comes off too.

Edits are line edits confined to the habit's own lines (see **Traps: line edits** and **Traps: re-measure**, as `applyStatusChange` explains); `ownLinesEnd` keeps a child's logbook out, or a parent would clear a square nobody pressed.

Not byte-identical afterwards: dropping `:LAST_REPEAT:` re-aligns the drawer via `alignDrawer` (minimum width), so `:STYLE: habit` becomes `:STYLE:    habit`, matching `setPropIn`'s house style; better than padding for a removed key.

## Column view, and the effort rollup

`internal/app/orgs/columns.go` is org's `#+COLUMNS:` as `GET /columns` (before it, only the gantt plugin read `EFFORT`, to size bars). The point is the **summary operator**: `%EFFORT{:}` makes a parent show the total under it, so nobody maintains a project's size. worg: `components/Columns.tsx`, model `src/columns.ts`; cells are editable.

The line in force is the first of: the request's, the file's `#+COLUMNS:`, `columns.default` in yaml, a built-in; `From` says which, so the view can explain a missing column. Built-in: org's `ITEM TODO PRIORITY TAGS` (without org's terminal character widths, so worg can share the screen between them) plus `%EFFORT{:} %CLOCKSUM`, because summing effort is the view's purpose and few files declare a columns line.

1. **A cell carries `Value` (shown) and `Own` (written).** They differ on rolled-up parents (`8:45` summed, `2h` own). The editor gets `Own`, or the first return writes the total onto the parent.
2. **Rollup = children's total *plus* the heading's own value**, so no typed number is silently discarded.
3. **Properties are read off the file's lines** (see **Traps: column-zero drawers**), or EFFORT looks unset for those headings.
4. **`%CLOCKSUM` always rolls up** (org's meaning); `%EFFORT` only with an operator, because inventing one would disagree with org about the file.
5. **Sort within each parent**, never flat (it is an outline; flat sorting separates children from parents). Empty cells sort last either direction (absence, not a small value).

`POST /heading/parts` (`headingparts.go`, tested) sets a heading's tags (the whole list) and/or priority as an edit of the headline line alone - stars, keyword and title kept as written, tags right-aligned to where they ended if they were (a run of spaces before them), else the gap they had. The older `POST /tags` toggle goes through `WriteOutOrgFile` (see **Traps: line edits**) and should not be used for new code.

`GET /columns/values` (`columnvalues.go`, tested on a parsed document) lists every property a file's headings use with each value and its count, most used first; the keyword, tags and priority come in as TODO, TAGS and PRIORITY. It feeds worg's value listing, its type-ahead in cell editors and its property-name suggestions in the columns line.

`POST /columns/spec` writes the file's `#+COLUMNS:`, replacing it or joining the `#+KEYWORD:` block at the top.

- **`POST /property` is a line edit**: `setHeadingProperty` in columns.go touches only the heading's drawer and re-aligns its keys like the record editor. It was `SetProperty` via `WriteOutOrgFile` (see **Traps: line edits**; it also ate a space in a `CLOCK:` line) - unacceptable for filling in estimates one after another.
- **`ParseDuration` counts a week as five days** (7200 minutes), not org's seven - deliberate for estimating; mind it when comparing with Emacs. Shared with gantt, so changing it moves every bar.

## What DONE does

`internal/app/orgs/logbook.go` does what org does on reaching DONE, which `ChangeStatus` (`todo.go`) used to skip: stamp `CLOSED:`, write `- State "DONE"  from "NEXT"  [...]`, and **move a repeating date on instead of sticking on DONE**. All writers funnel through `ChangeStatus` (kanban drag, `orgs todo`, MCP tool, tui `t` menu, gantt client), so habits touched outside Emacs used to break. `parseHabitCompletions` builds the habit graph from those `State "DONE"` lines; `IsHabit()`, `HabitStreak()`, `MissedHabit()` (`habit.go`) query it.

Config, narrowest wins: yaml `log:` (`done`, `repeat`, `intoDrawer`, `states`, `repeatToState`) → file `#+STARTUP:` (`nologdone`, `lognotedone`, `logrepeat`, `nologdrawer`, …) and `#+PROPERTY: LOG_INTO_DRAWER` → heading `:LOGGING:` / `:LOG_INTO_DRAWER:`. Defaults `done: time` and `intoDrawer: LOGBOOK` deliberately differ from Emacs, because marking done used to record nothing and the habit graph reads `LOGBOOK`.

1. **Line edit** (see **Traps: line edits**); also keeps timestamp spellings.
2. **`reread` after each line-count change** (see **Traps: re-measure**).
3. **A repeat is not a closure**: no `CLOSED:` (existing one removed), `:LAST_REPEAT:` written, keyword back to live, and the reply's `Msg` says so - else a card dragged to Done reappearing in Next looks like a failed write.
4. **Repeaters**: `+2d` from the old date (may stay past); `++2d` from it until strictly future; `.+2d` from today but keeping the stamp's time of day, or a daily 09:00 reminder drifts when ticked late.
5. **A day is the habit-counting unit**: two ticks in an afternoon count once; double-clicks are harmless.

Four parser fixes came out of this (cookies like `NEXT(n!)`, several planning keywords on one line, `.+1d/3d`, `*org.Drawer` in `parseHabitCompletions`); see `docs/claude/bug-history.md`.

## What a kanban card shows of its heading's text

A card's **checklist**, **recording** and **picture** come from `GET /body/{hash}` (`internal/app/orgs/checklist.go`), the body **as written**. Not `/todohtml/{hash}`: a rendered `- [ ]` is a disabled input with no way back to its line.

1. **Ticking a box is a line edit**, `POST /checklist` - not `/body/change`, which rewrites the whole document (see **Traps: line edits**).
2. The write names **which box and what it said**; both must agree or it is refused, because the index goes stale when a line is added above and the text cannot tell identical items apart. It sends the wanted state, not a flip, so a double click cannot land as two flips.
3. A `*` in column zero is a **heading, not a bullet**: `* [ ] something` is a heading, and counting it would put another card's item on this one. Client and server must match the same shape and count the same items, because the client numbers the boxes and the server recounts them to find the line.
4. Audio and image urls are resolved **server side** and handed over ready to use (see **Traps: media paths**) - the same sum the html exporter does for its audio players.

Bodies are cached in the browser by hash; a hash does not change with the text, so the board clears the cache when it reads itself back and a tick refreshes that heading. Both **replace rather than drop** (old body stays until the new arrives), because blank is worse than stale and the checklist would blink on every tick.

## Records: the contact book, and everything else worth a list

A **record** is one heading for one thing (person, laptop, playing card): heading = name, property drawer = fields, body = notes, `LOGBOOK` = history. `docs/records.org` (format for hand-typing), `internal/app/orgs/records.go` (same as SDOC, plus engine, endpoints), `internal/common/records.go` (wire), `worg/src/records.ts` (client), `RecordBrowser.tsx` (both worg tabs), `cmd/oc/commands/rec/` (terminal), `cmd/oc/commands/contact/` (same engine, address-book manners).

Identity is **`:RECORD: contact`** (value = collection); without it a heading is never a record. The search tab excludes records with the query `!IsRecord() && !IsCollection()`, not a browser filter.

1. **A field's kind comes from its name**: `EMAIL_WORK` (email, work), `PHONE_MOBILE` (phone, mobile), `BOUGHT_DATE` (date, bought). So a contact and a guitar pedal share one view and `RecordBrowser.tsx` names no property. `fieldOf` (`records.go`) is repeated in `records.ts` so an add form shows the right input before saving; occasions are `birthdayFields` ↔ `CELEBRATIONS` (see **Traps: logic said twice**) - a laptop's purchase date is not an occasion.
2. **Line edits only** (see **Traps: line edits**; a record shares its file with many). Drawers are found by walking lines under the headline (see **Traps: column-zero drawers**); the range is **found again** after a line-count change (see **Traps: re-measure**); the property drawer is re-aligned, because people edit the file by hand.
3. **The update endpoint refuses `RECORD` and `ADDED`** - identity and start date are not form-editable. `ID` is allowed, because a duplicated entry needs its old one cleared.
4. **Reading all records walks every file**, so it is cached on `OrgDb.ReloadIndex` (see **Traps: caches**); the contacts tab searches per keystroke.
5. **`InCollection("contact")` is loose, `IsRecord("contact")` strict.** `IsRecord` is true only for records; `InCollection` walks up the outline and is also true for the collection's container and anything under it. A contact's notes have no `RECORD` of their own but are part of the address book, so "not the address book" must exclude them too.
6. **Birthdays are computed, never stored.** `/records/birthdays` takes a window and returns occurrences in it, so the agenda, CLI and contact card agree; 29 February is kept on the 28th in non-leap years. A corrected birthday is right everywhere on the next read.

The agenda draws birthdays as all-day entries with a cake, coloured by a `BIRTHDAY` entry in its own `STATUS_COLORS`. They are merged into its two lists (`dayShown`, `everything`), not into either fetch, so whichever request lands second still shows both.

### Importing vCards: `POST /contacts/import`

`vcard.go` (the parser and the card → field mapping, pure, the mapping table at its top), `vcardimport.go` (matching, plan, writes, the endpoint), wire types `internal/common/vcardimport.go`, pinned by `vcard_test.go` (Apple 3.0 with item groups and a folded photo, Google 3.0, Outlook 2.1 quoted-printable in Windows-1252, 4.0 with `PREF=1` and a `data:` photo). Clients: `orgs contact import` and worg's Contacts tab (`ContactImport.tsx`).

1. **One plan, then writes.** `planImport` decides everything from one read of the records (and is the dry run); nothing is decided while writing. Cards that are the same person (UID, an email, the name) are folded first; matching is UID (`VCARD_UID`) → any email → name, case-insensitive.
2. **merge never changes a value.** A new email/phone/url/address the record lacks goes beside the old one under the next free key (`EMAIL_WORK_2`); a different single value (birthday, company) is reported as `kept`. A value already present under another key is not added again (`sameValue`: phone digits with or without the country code, url without scheme, a year-less `--05-15` equal to `1980-05-15`). overwrite replaces, but never an address the card still carries elsewhere. Notes are appended only when new; tags only added.
3. **Updates first, then new people in one splice, the target row measured again in between.** Measured once, the splice landed inside the first updated record's subtree (see **Traps: re-measure**; found by diffing a live import). One splice, because `AddRecord` per person re-reads the collections each call - 400 people take ~50ms this way.
4. **Fields through `UpdateRecord`** with `RecordUpdate.Via` (`- Updated [...] EMAIL_WORK_2 from vCard import`); tags and the note are a line edit after it (`addHeadlineTag`, `appendNote` before any child heading, a `Noted` line). New records get `- Added [...] from vCard import`.
5. **Photos** are written to `images/contacts/<slug>.<ext>` beside the org file and linked `[[file:...]]` (relative; see **Traps: media paths**); the same bytes already there are reused, so a re-import does not make `jane-roe-2.png`. A photo url goes into `IMAGE` as is.
6. **Every byte order mark is dropped**, not just the first: several files sent as one text carry one each and the second hid a card. 8-bit Windows-1252 files are decoded **client side** (`common.VCardText`, `vcfText` in worg) because JSON cannot carry them.
7. Helper names are prefixed (`vcOneLine`, `vcHasTag`, `vcPlural`, `vcIsHeading`): the package already had `hasTag`, `plural`, `isHeadingLine`, and `oneLine` arrived from another file the same day.

## The Links tab: where every link goes

`GET /links/all` (`internal/app/orgs/linklist.go`) is the flat sibling of `/links` and `/links/graph` (which drop non-org links). It keeps **all** links, because the ones worth finding again are usually external. It reuses the graph's cached index, so it is free and cannot disagree with the graph.

Decided server side:

1. **Service** (`serviceOf`): the name a person would use. A short list covers hosts not named after themselves (`*.atlassian.net` is Jira) or worth grouping (drive, docs, sheets are all Google Docs); anything else is named after its domain. The list cannot be complete, so the fallback matters most: an unknown host gets its own group, never none.
2. **Scheme and host** (`linkSchemeHost`) are read only for links the resolver calls external. `splitProtocol` calls anything before a colon a protocol (`notes.org::*Plans` gives scheme `notes.org`), so checking against `externalProtocols` keeps it consistent with the link's `Kind`.
3. **A `file:` link is grouped by its protocol, not as "external"**, or pasted screenshots mix with things genuinely elsewhere.
4. **A link to a file on disk is resolved to something showable** - `mediaURL` plus `mediaKindOf`, as for a source block's result file and a kanban card's picture - so the tab shows the picture, not the path.

5. **`server.linkProtocols`** (`jira:` → url, or → a command) is applied here (`userLinkProtocol`) and by `GET /links/resolve`; a url one also sets `Host`, so it groups under that service. The html exporter writes url protocols as their url (via `PluginManager.LinkProtocols`); command ones stay as written and worg's Files click handler asks `/links/resolve`, opening the tab *before* the await (popup blocker) and showing the command line, since a browser cannot run it.

Client: `worg/src/links.ts` (pure, tested; separate because of the **regex**) and `components/Links.tsx`. A half-typed pattern that does not compile is normal: the box says why and **keeps matching everything**. The pattern runs over `linkHay` (target, description, host, service, heading, file, where it lands), because people search by any of these.

The right pane shows the far end: http(s) in a sandboxed frame, an org target via `ThemedHtml`, or a picture, player or pdf. **Pages may refuse framing** undetectably, so "open it in a tab" sits above the frame from the start.

## The Code tab: finding source blocks

`GET /code` (`internal/app/orgs/code.go`) returns every `#+BEGIN_SRC` block **read**, not just located: language, name, switches, babel header arguments, variables. worg's Code tab (`components/Code.tsx`, helpers in `src/code.ts`) is the Tables tab for code.

It is an endpoint, not a grep, because only the database can tell `:var data=monthly` (a table) from `:var scale=2` (a number). Each variable comes back resolved: target, kind, location, table shape. An unresolved name has an empty kind and is drawn in danger colours - the block cannot run.

go-org pitfalls, all silent:

1. **`Headline.Blocks` is not the blocks under a heading** - its filling loop `break`s after the first node, so a paragraph then a block gives an empty list. Blocks are walked from the node tree and attributed to the last heading above them (see **Traps: column-zero drawers**), as the link index does.
2. **A block never sees its own `#+NAME:`.** The parser consumes it as a named-node keyword, so it is not in the block's keywords, and a `#+RESULTS: chart` under the block then overwrites the name map entry with the *result*. The name is read off the file's lines, scanning up over affiliated keywords.
3. **`Block.Result` is a `Result` value, not `*Result`** (see **Traps: pointer trap**); asserting only the pointer gave no result and no error.
4. **`splitParameters` leaves switches glued to the language** because it splits on `" :"`, so `python -n -r` is one token and the first word must be split off.

The index is cached against `OrgDb.ReloadIndex` (see **Traps: caches**), because the tab searches as you type. Language counts are over the whole database, not the filtered set, or picking a language would empty the strip and leave no way back.

## Running a source block

`POST /code/run` (`internal/app/orgs/babel.go`) runs one block. **Off unless `babel.enable` is set** (the refusal says what to write), because reading files and executing them are different promises, the server may be reachable beyond its machine, and people use `noAuth`.

1. **Variables are bound in the language's syntax** (`runner.bind`, or `assemble` for go/elisp/sqlite, `bindArgs` for awk): `:var scale=2` an assignment, `:var data=monthly` a list of lists with numeric cells bare so `sum(r[1] for r in data)` works. See **Babel values and the run pipeline** below.
2. **Code is dedented before it runs** and travels dedented on the wire, because the indent belongs to the org file (python would raise `IndentationError` on line one). `UpdateBlock` re-indents on the way back, so the round trip is exact.
3. **A table result is org table text**, parsed by `tablesIn` and drawn by `OrgTableView`. `:results` is honoured when present; otherwise the shape is guessed timidly (org table, tab-separated, printed list of lists), because a paragraph cut into columns reads worse. "As it printed" is one click away.

A result that **names a file**: `describeResultFile` resolves it against the block's directory, then each org directory, and returns size, a serving url, kind (`Media`: image, audio, video, pdf, text, binary), colouring language, and text contents up to 512kB. The client draws the link plus a picture, player, or text via `CodeText`.

1. **Resolved server side**: `plot.png` is relative to the block's org file, which the client does not know (see **Traps: media paths**; same sum as `MediaSrc`).
2. **A block that printed its own link keeps it**: `babelFormat` no longer wraps printed `[[file:plot.png]]` a second time (which gave a link naming no file). Printing the link is what emacs babel does, so it is the common case; pinned by `TestFileResultIsNotWrappedTwice`.
3. **A named file that is missing is reported**, because the link alone reads like an answer - this is most of why the server checks.
4. **Text-named but byte-filled is `binary`, not printed**: `looksLikeText` checks the first 8kB, because the extension is only a claim.

`POST /code/update` writes name, header and body. The header is rebuilt in org's order (language, switches, variables, rest); `headerLine` in Go and `worg/src/code.ts` must agree (see **Traps: logic said twice**). Only the block's lines are touched, body spliced before header and name so every row stays valid.

The Code tab groups its list (`codeGroupBy`): flat, by language, or by file (Tables tab base-name formatting, full path in the tooltip) - a sort plus bands in one list, like the search tab's grouping.

## A file that is a D&D book

`#+LATEX_CLASS: dndbook` marks an adventure or bestiary to print like the books. It gets buttons **Book** (html exporter's `dnd` theme) and **PDF** (pdflatex).

`GET /dnd/books` (`internal/app/orgs/dndbook.go`) is the sibling of `/dnd/characters`: every watched file with that class. `GET /pdf` is its own endpoint, not a `/file/{type}`, because the answer is **bytes** - the pdf exporter writes a file and returns no string.

1. **Cached against the file as built** (path, modification time, size), so a second open runs no pdflatex and an edit invalidates it by changing the key. `refresh=t` (sent only by the file view's reload button) gets past a cached failure.
2. **One build at a time per file**, or two tabs have pdflatex write the same output and the loser serves half a file.
3. **The client fetches a blob, not an iframe src**, because the request needs the `Authorization` header. The object url is revoked when replaced, or each look leaks a pdf into the tab.

dndbook export depends on four latex exporter fixes: `MakeTemplateRegistry` takes (class, default); tables are `DndTable`; the class has its own paragraph template with no `\par`, and `MONSTERTYPE` is squeezed to one line; every latex template renders inside `{% autoescape off %}`. Details in `docs/claude/bug-history.md`.

## A file that says how it wants to be read

The file view offers a button beside Rendered and Org for three kinds of file: a **character sheet** (`/dnd/characters`), a **D&D book** (`/dnd/books`), and a file naming its html theme with `#+HTML_THEME:` (`/files/themes`, in `dndbook.go`). Such a file opens in what it asked for; others keep the current view.

- The theme button exists because Rendered *cannot* show it: Rendered passes the reader's html theme setting, which overrides `#+HTML_THEME:` unless the setting is "file". The button is labelled with the theme's name (`themeLabel`: "docs" → Docs), requests the theme **by name** (not an empty theme), and the ground behind the frame follows the file's theme, because a stylesheet naming no background leaves the page transparent.
- None of the four owned views (`sheet`, `book`, `pdf`, `theme`) is written to `fileView`: none is a way of reading *every* file.
- The theme is reported **as the file wrote it**, whether or not the server has that stylesheet (`/html/themes` lists those it has), so a typo is not swallowed silently.

## Searching the text of every file

Files tab, second box: `/files/search` (`internal/app/orgs/filesearch.go`), a regex over file contents listing matching lines (swiper for the whole database); the first box filters names. Arrows walk it from the box, Enter = click, and moving **opens** the hit. Unlike `/search` (parsed headings/keywords/dates) it reads raw lines, so it finds text in drawers, tables and source blocks. Unlike `/grep`, it doesn't return `"file:12:text"` strings, which break on colons and hide omissions.

1. **A half-typed pattern is a state, not an error**: `200` with `Ok: false` and the reason; the panel says "Not a pattern yet — …" and keeps prior results.
2. **Cap lines kept, never the count**: `.` is typed on the way to things; each file reports total matches even when only forty lines return (`Truncated`).
3. **Scanner buffer is 4MB**: a pasted image or minified blob exceeds `bufio`'s 64k and silently ends that file's scan. `TestSearchPastAVeryLongLine`.
4. **Long lines are sent as a window around the match** (`trimAround`), offsets adjusted, ellipsis marking the cut, match never split.
5. **Offsets come from the server** so the client never re-runs the pattern; Go's and the browser's regex engines differ.

Hits open in the **source** view: a line number means nothing in a rendered page.

## Quick capture from worg

`c` or the rail's pencil opens `worg/src/components/Capture.tsx`: reads `/capture/templates`, shows one as a form, posts `/capture` (same endpoints as `orgs cap`).

**`internal/common/captemplate.go` is the grammar** (pattern, self-answering names, `CapFields`, `CapSubstitute`, `FillCapTemplate`), in `internal/common` so expander and filler can't diverge. Wiring: `internal/app/orgs/capturefill.go` (server), `worg/src/capture.ts` (browser), `cmd/oc/commands/capture/` (terminal). See **Traps: logic said twice**.

The `|=` forms are written by the *server*:

```
{{name}}                   a value to fill in, labelled from the name
{{name|prompt}}            the same, asked for in your own words
{{name|=default}}          pre-filled with a default, still editable
{{name|prompt|=default}}   both
{{CONTENT}}                the body
```

**`/capture/templates` answers self-answering names** (`{{uuid}}`/`{{guid}}`, `{{username}}`, `{{now}}`, `{{today}}`, `{{tomorrow}}`, `{{week}}`, `{{hostname}}`, rest of `CapAutoValue`) **as a default, not a substitution**: `{{uuid}}` → `{{uuid|=f81d4fae-…}}` (how a `:CUSTOM_ID:` gets a GUID). An unaware client still has a value - and the server must do it because `crypto.randomUUID` is secure-context only (as with `clienthash.ts`), so worg over plain http can't. A client can still see the name and answer it itself, and the value stays editable (people often change a capture's timestamp). `|=` lets a client tell an answer from a question (`{{source|Where from?}}`) without its own, drift-prone name list.

Rules (`captemplate_test.go`, `capture.test.ts`):
- **Unanswered names stay as written**, braces and all - clients see fewer holes, never different ones, so mismatched server/worg versions work.
- **Idempotent**: a placeholder with a default is left alone.
- **One value per name per template**, aliases as one (`CapCanonName`): two `{{uuid}}`s agree (a `:CUSTOM_ID:` and an `id:` link to it). Two templates in one answer get two uuids.
- **A value containing `}` or `|` isn't written** (it wouldn't parse); the name is left bare for the client to ask.
- **Answered when the list is fetched**, not at capture: worg re-asks per dialog open, `orgs cap` per run, so uuids are fresh; a client holding one list all day repeats a uuid. `/ext/capture/templates` (editing a user's templates) is **not** expanded, or saving would bake today's values in.

Also:
1. **Multi-line yaml templates need `template: |-`**; a plain scalar folds newlines to spaces, flattening a drawer. (The shipped example used to get this wrong.)
2. **Lines containing only an unanswered placeholder are dropped**, so only answered optional properties are written (as in the record editor).
3. **A capture has no heading yet**, so dictation and picture paste use `internal/app/orgs/capturestash.go`: keep the file first, carry the link in the text (a kanban card has a heading, so it stores and links in one call). The link is relative to the destination, which the dialog knows only via the template: hence `stash=1&template=NAME` on `/image/paste`, and `/voice/link`.
4. **The mic writes into the box the cursor was in**, found on mouse down from `document.activeElement` → `data-cap-key`, not a remembered focus handler (clicking the button moves focus to it). The tooltip names the box.
5. **Preview is `OrgSource` with `openDrawers`**: the file view starts drawers shut, but in a capture preview the drawer is what's being checked.

`InsertEntryUsingTemplate` indents **every** content line (it used to indent only the first) because a column-zero `:PROPERTIES:` hoists the heading (see **Traps: column-zero drawers**), and writes `NewNode.Tags`, which the wire type always carried and nothing wrote.

## Pictures: pasting one, and filing a chart

`POST /image/paste` stores a picture in `images/` under the first org directory and appends a link, placed by:
- **a heading hash** - a kanban card's back; the front then shows it (it draws the heading's first picture);
- **a filename and `afterLine`** - the Tables view filing a chart under its table, not at the end of the enclosing heading.

`afterLine` is **zero-based**, like a table's `EndLine` (go-org counts rows from zero); inserting *at* it lands one row above the table's bottom, inside it. Bytes are stored unconverted (extension matches what was pasted). The link is relative to its org file, so moving the org directory keeps pictures (as with voice-note audio; see **Traps: media paths**).

`WriteRegularLink` reads `file:path` as well as `file://`, and pages this server serves get `/images/...` paths rather than `localhost` urls (history in `docs/claude/bug-history.md`).

## Speaking into a heading that already exists

The kanban card back's mic records, transcribes and appends: `POST /voice/append` (`internal/app/orgs/voiceappend.go`). Unlike `/voice/note` (new heading), words go into the existing heading's body and audio is linked from its `:AUDIO:`. Voice-note order: `/voice/recording` saves audio as soon as recording stops, **before** anything else, and the text written is the text sent, not re-transcribed - a failed transcription loses only itself; if whisper returns nothing the recording is still filed and linked, and the card says so.

1. **Line splice**, like voice notes and the checklist (see **Traps: line edits**).
2. An existing `:AUDIO:` is **kept**; the new link goes in the body beside its words, since overwriting the one-link property would orphan the first recording.

## Reaching the server from a phone

`GET /addresses` lists every url this machine is reachable on, private first; the page can't know (the bar says `localhost`, which on a phone is the phone). worg's sidebar foot shows it as a QR code.

## Gantt charts

`internal/app/orgs/gantt.go` adds the two endpoints worg's Gantt tab needs (wire types in `internal/common/gantt.go`):

- **`GET /gantt/tasks`**: every heading a query finds, with its lane, effort, what it comes after and the **hash to change it by**.
- **`POST /gantt/add`**: writes a new heading under a parent given by hash (`/capture` needs a template; a chart knows its parent by hash).

It restates no scheduling rules: order, lane and owner come from the mermaid exporter's exported helpers (`mermaid.After`, `GetSection`, `GetResource`), so worg's charts and `/file/mermaid` cannot drift.

A heading's **lane** (`GetSection`) is the first of these that is set; nesting alone does nothing:

1. the heading's own `:SECTION:` property;
2. the `:SECTION:` of its parent, but *only* when that parent is tagged `:project:`;
3. its own `:ASSIGNED:` property.

Otherwise the client draws it in lane `main` (`DEFAULT_SECTION` in worg's `gantt.ts`). The **resource** (`GetResource`) is a separate ladder: `:ASSIGNED:`, `:RID:`, `:RESOURCEID:`, then the *headline* of a parent tagged `:project:`, which is why an unassigned task is coloured by its project when colouring by assignee. Dates are **not laid out** server side: a task says when it starts or what it follows and the client resolves the chain (the same split the exporter has with mermaid's renderer). A task the query did not find, but that a found task comes after, is included with `Implied: true` so the chain reads end to end.

Saved chart setups are `GET/POST /ext/gantt/views` (`ganttviews.go`, `ganttViews` in the extensions yaml, the whole list per write). The server only keeps them - rule matching, presets and baselines are worg's - but **every field worg saves needs a Go field** or it is dropped on save (as with kanban boards). Baselines live inside a view. A view needs a saved query or a query of its own.

`POST /gantt/marker` (`ganttmarker.go`, pinned by `ganttmarker_test.go`) adds, changes or deletes a marker heading with line edits. Its date is the heading's own timestamp - the first active stamp in its own lines outside the planning line and drawers, which is what go-org reads as `Headline.Timestamp` - or its DEADLINE. A single day on a heading dated only by SCHEDULED moves the SCHEDULED; a range cannot live in SCHEDULED, so it is taken off and the range written as the heading's own stamp. Headline edits keep the stars, keyword and priority cookie; a child's stamp is never touched (`ownLinesEnd`).

`POST /date/change` (`datechange.go`, `datechange_test.go`) is how a dragged bar moves, and undoing it is the same write with the old date: a line edit of the planning line (made straight under the headline when there is none, at the indent of the heading's own drawer so column-zero files stay column zero) or of the heading's own stamp. A stamp sent as written (`<2026-10-05 Mon +1w>`) is kept as written; a bare date is read and written in org's form. Tags from the gantt inspector go through `/heading/parts` (the whole list), properties through `/property` (empty value removes).

`setPropIn` (attach.go) indents a new property like the drawer it joins, not by heading depth: Emacs writes drawers in column zero, and the old indent left a three-space stray in them (seen on every gantt reorder). Every property writer goes through it.

### Setting a chart up from tickets: `/gantt/suggest` and `/gantt/link`

`ganttsuggest.go` (wire types at the foot of `internal/common/gantt.go`, pure parts pinned by `ganttsuggest_test.go`) reads the tasks `/gantt/tasks` draws for a query (`ganttTaskSet`, factored out of the handler so the two cannot disagree) and suggests dependencies, estimates and lanes; it writes nothing. `ganttlink.go` writes one dependency. Clients: worg's gantt tools and the `gantt_*` MCP tools.

1. **AFTER holds one heading** (as `/insights/deps` says), so a task gets at most one dependency suggestion, none if it already waits on something (AFTER or an ORDERED parent), and none that closes a circle with the existing links plus the more certain suggestions taken before it. `/gantt/link` **refuses** to add to a task whose AFTER names another heading (remove first) rather than drop a wait silently; overriding an ORDERED chain is allowed and said.
2. **Phrases are read from the heading's own body** (`ownEnd`), drawers, planning, clock and block lines skipped. Links and ticket keys in the clause are taken at their word and never guessed past: a key naming nothing in the chart is about something outside it. Only a clause with neither is matched against titles, and a two-word title needs two of its words.
3. **Ticket keys**: the jira template keeps the key in `CUSTOM_ID` and a browse url in `LINK`, and writes no issue links; `BLOCKED_BY`/`DEPENDS_ON`/`BLOCKS`/`ISSUELINKS` are read if a richer import writes them.
4. **Redundant** = an AFTER whose ORDERED chain (the one clearing it would hand the task back to) already reaches the same heading. With one wait per task there is no other kind; `redundantEdges` is a general transitive reduction fed that chain as a fixed edge.
5. **Estimates** sample finished, clocked work in the window the way `/insights/estimates` measures it (rollups skipped, unestimated children's clocks included); `Mins` are working minutes with `DayHours` days.
6. `/gantt/link` gives the waited-on heading an `ID` made by `ganttIdFor`, worg's `idFor` (see **Traps: logic said twice**), then re-finds the waiter before writing AFTER (see **Traps: re-measure**).

Gantt edits go through the existing endpoints, and three of them were fixed for it: `subtreeEndRow` takes a subtree's end from the file's own lines; `SetProperty` with an empty value removes the property, and the drawer too if it ends up empty; with `noAuth: true` everything runs as user `local`. Details in `docs/claude/bug-history.md`.

## Moving headings: refile, copy and archive

`POST /move` (`internal/app/orgs/move.go`, wire types `internal/common/move.go`) does all three, taking a list of sources and an `Op`. Each finds a heading, works out its destination and writes it there; refile then deletes the original, copy does not, and archive picks its own destination from org's rules. `GET /refile/targets` is the structured sibling of `/refilefiles`: hash, outline path, level, keyword and tags instead of `"file|H1|H2"` strings that cannot tell two headings called Notes apart. `POST /copy` is the single-heading copy, beside `/refile`.

- **The batch is one request because hashes shift when a file is edited** (see **Traps: hashes**): twenty separate refiles get the first right, then address headings that are gone or that inherited those hashes. `Move` resolves **every** source to `(file, outline path, headline)` before writing, addresses each by `file+olp` (which survives a rewrite), and reloads every touched file between operations.
- **A source whose ancestor is in the same batch is skipped, not failed**, since moving the ancestor takes it along. The result marks it `Skipped` so a client can tell "went with its parent" from "not found".
- **`Ok` means *everything* worked**, and per-heading results are always present, because a half-worked batch is neither success nor failure.
- **`Create` is off by default**: creating a destination because of a typo is worse than being told it does not exist.

Three bugs that corrupted files through the old `/refile` are fixed and pinned by `refile_test.go`: subtrees written once per level, the delete measured before the insert, and archive's `file+heading` typo. See `docs/claude/bug-history.md`.

Client: `worg/src/move.ts` (pure, tested), `MoveDialog.tsx` (one dialog for all three) and `FileMove.tsx` (the file view's outline picker), reached from the search tab's row menu and bulk bar, the kanban card's back, and the files tab's toolbar.

## Flashcards: org-drill over a query

`drill.go` is org-drill's arithmetic (SM5, SM2, Simple8, statuses, property formatting), pinned against org-drill's own worked numbers in `drill_test.go`; `drillcards.go` reads cards and writes ratings. Notes on the worg side are in `docs/claude/worg.md`; user guide `docs/drill.org`.

1. **Cards are read off the file's lines**, never `Headline.Children` (every card with drill data has a drawer, see **Traps: column-zero drawers**).
2. **Writes are line edits of the card's own lines**: properties via `ensurePropertyDrawer`/`setPropIn` at the drawer's own indent (Emacs writes drawers in column zero), SCHEDULED via `setScheduled` on the planning line, `leech` via `addHeadlineTag`. (`ChangeDate` is a line edit too now, in `datechange.go`, but drill keeps its own `setScheduled`.)
3. **Floats are written as Emacs prints them** (`lispFloat`: `4.0`, not `4`), so a file drilled here diffs cleanly with one drilled in Emacs.
4. **The SM5 matrix is keyed by ease rounded to 3 places** (deliberately not org-drill, whose float keys never match what is read back) and saved per user in extensions after every rating.
5. **Quirks kept on purpose**: the EF floor is applied before the change; Simple8 does not count a failure in the total; the stored interval is the hypothetical one the button showed (org-drill's smart reschedule).
6. Test the review endpoint with `Content-Type: application/json`: without it the form-parsing middleware eats the body and the hash comes through empty ("no heading with that hash").

## Table formula functions

`functions` in `tablefuncs.go` is the `#+TBLFM` function map (the query language has its own, local to `todo.go`); implementations are split over `tablefuncs_range.go` (conditional aggregates, lookups, statistics, argument helpers), `tablefuncs_misc.go` (logic, text, rounding), `tablefuncs_dates.go` and `tablefuncs_finance.go`. The user guide is `docs/tables.org`; the SDOC block at the top of `tablefuncs.go` is the short reference. Keep both in step with the map. `TestTablesGuideExamples` (`tablesguide_test.go`) runs every `#+BEGIN_SRC org` table in the guide with its formula targets blanked and requires the shown answers back, so a wrong example (or a behaviour change) fails the tests. Other tests run real `#+TBLFM` lines through `ExecuteFormula` (`tablefuncs_test.go`, `evalOrgTable`).

1. **Every cell reference arrives as a `*RangeIter`**, and the function decides what it means: a range parameter expands it (`argList`, or `argCells` to keep empty cells so two ranges stay aligned) and `$2` is then the whole column; a value parameter reads it with `argVal` and `$2` is the current row's cell.
2. **A column iterator runs one row past the end** of a table with separators. Anything walking a range must check `isDataRow` (`makelistfromrange` and `walkCells` do); before it did, `vcount($1);E` counted a phantom cell.
3. **govaluate splats a lone list argument and appends later arguments onto a list-valued first one**, so a list result (`sort`, `lookupall`) works only as the last argument.
4. **Arguments are evaluated before the call**, so `if` evaluates both branches and `iferror` can only catch NaN, infinity and nil, never a function's error.
5. **Cell references are replaced inside string literals too** (`'$1'` becomes a range), and `remote(Name, range)` needs an unquoted name to be rewritten.
6. **govaluate's own operators take only float64.** A function answering an `int` could not be added to or compared, so an `init` in `tablefuncs.go` wraps every function in the map to hand an `int` on as a `float64`. Date literals (`'2026-03-01'`) arrive as Unix seconds (float64); `toTime` reads them.
7. **The lexer reads a run of symbols as one operator**, so `$2*$rate` was `*$`. `ExecuteFormula` spaces an operator from a following `$name` (`RE_OP_BEFORE_NAME`); `@>$3`/`@<$3` are excluded.
8. **The header row is skipped before evaluating, not just before writing**: `$4=$2*$3` used to multiply the column titles and fail. Arithmetic between two cells treats an empty cell as 0 (`OpGen` in `tableops.go`), as arithmetic with a literal already did.

## A file's git history

`filehistory.go`: `GET /history` (commits touching a file, parents rewritten by `git log --parents -- file` so it draws as a graph; staged/modified state with line counts; `Last`, the newest commit from HEAD that touched it, which the uncommitted copies sit on) and `GET /history/text` (the file at a commit, `INDEX` or `WORKING`). For worg's Diff tab; the diffing is done in the browser.

- Only files the OrgDb knows (`FindByFile`); a revision must match `revPattern`, never start with `-`, and never contain `..`.
- A commit from before the file existed answers `Ok` with `Missing`, not an error.
- `repoOf` resolves symlinks on both the file and the toplevel (macOS `/tmp` is `/private/tmp`), or the relative path comes out as `../..`.
- Merge commits have no numstat (git prints none for merges by default), so they show no line counts.

## Dynamic blocks

`dynblock.go`: `#+BEGIN: name :params` … `#+END:`, refreshed by `POST /dblocks/update` (one block by line, a file, or every file) and listed by `GET /dblocks`. Built in: `clocktable` (in `clocktable.go`, org's layout: a time column per level, bold total, a File column when several files), `columnview` (over `BuildColumnView`, so rollups match the Columns tab), `query` (an orgs query as a table, rows built with `ownRow`) and `insertdatetime`; any other name falls back to `PlugManager.BlockExec`. Clients: `orgs dblock`, `/execb` (vscode).

1. **Line splice between the BEGIN and END lines only**; every block's text is worked out against the file as it stands, then spliced bottom up so the rows stay true (see **Traps: re-measure**). The file is reloaded first so the rows the database attributes blocks by are the disk's. A block that comes out the same leaves the file unwritten.
2. **A `#+BEGIN:` inside a SRC/EXAMPLE/EXPORT/COMMENT block is not a block** - every guide showing one has such a line.
3. **The heading a block is under is the last one starting above it** (`sectionAtRow`), as for links and code blocks.
4. **An unknown block is reported, never emptied**, and the blocks beside it still refresh.
5. A column view's `#+TBLFM:` lines are kept and run again over the new rows (`applyTableFormulas`).

Clock ranges are `clockBlockRange` (`:block`) and `clockRangeOf` (`:tstart`/`:tend` through `internal/orgdate`): half open, weeks from Monday, and a clock is clipped to the range (`clockMinsIn`); a running clock counts to now. `/clockreport` uses the same, carries each entry's `Sessions`, and answers `Error` for a range it cannot read.

## Timesheets and invoices: `GET /timesheet`

`timesheet.go` (wire types and the `invoice:` settings in `internal/common/timesheet.go`, pinned by `timesheet_test.go`). Clients: `orgs invoice`, `orgs timesheet`. The SDOC at its top is the user documentation.

- **Every sitting belongs to exactly one client**, or an invoice bills it twice: the inherited `CLIENT` property (file `#+PROPERTY: CLIENT` too), else the nearest tag naming a configured client/alias or `clientTags`. `by=tag` takes the first allowed tag nearest the heading; with nothing configured, any tag but `project`/`archive`/`nobill`. Unattributed time is its own client `""`, listed last, never invoiced, and warned about.
- **Rounding is per invoice line** (a task, or a task-day with `lines=day`), to the minute first then to `round` - ten five-minute sittings bill as one increment, not ten. Entry amounts are unrounded.
- Rate: heading `RATE` (inherited) > client > request `rate` > `invoice.rate`. `BILLABLE: no` or a `nobill` tag: listed on timesheets, off invoices (`unbillable=t` puts them on at no charge).
- The invoice number's month is the **range's** first month, not the issue date.
- html: `templates/invoice/invoice.html` (Go `html/template`, not pongo2: it escapes) + `invoice/base.css` (layout, paging) + `invoice/themes/<name>.css`, all found with `assets.FindTemplate`, so a user's copy replaces any of them. `accent` must look like a colour (`cssColorRe`); a request's `logo` must be a url - a path only from the yaml, or any client could read any server file into the page.

## Manuscripts and ebooks: `GET /book`

`plugs/manuscript/` (SDOC there is the user guide): `book.go` reads an org file into a `Book` (chapters, blocks, runs, footnotes), `docx.go` and `epub.go` write it - by hand, as zips of XML, so nothing needs installing. Both writers work only from `Book`, never from the org tree, so they agree about what a chapter or scene is. `GET /book` (in `export.go`) uses the `manuscript`/`epub` exporters' yaml when configured (either one's contact details serve both) and works without.

- **Structure**: shallowest heading = chapter; `:part:` makes its children the chapters; deeper headings are scene breaks unless `#+MANUSCRIPT_SCENES: nil`. Scene marks are read from a paragraph's **source** (`***` parses as a bold star), and `#` / `* * *` cannot be marks at all (comment, heading).
- **Word requires schema order** inside `w:pPr` and `w:rPr`; a misordered file opens as "unreadable content". `pp.xml()` and `rp.xml()` are written in that order - keep them so.
- Fonts and spacing are on the `Normal` style as well as the doc defaults: Quick Look and some converters read only the style (shunn rendered in Times without it). Quick Look also ignores page breaks and tab stops, so it is no judge of those.
- EPUB: `mimetype` first and stored; the id is a uuid from title + author unless `#+ISBN`/`#+IDENTIFIER`, so a re-export is the same book in a library. Pictures are copied in; a url or missing file is left out and reported (`X-Orgs-Warning`). Themes are `templates/epub/themes/*.css`.
- Not checked with epubcheck (no Java here); the tests check well-formedness and structure.

## Babel: results, calls, inline code, pictures, ledgers

`babelexec.go` holds the engine; `babel.go` the runners and bindings. Everything runs a `babelJob` (`runJob`): a block (`jobForBlock`), a `#+CALL:` (`callJob`: the named block, the call's arguments over its `:var`s, the call's header arguments over its own) or inline `src_lang[…]{…}` / `call_name(…)`. `POST /babel/exec` runs what is at a line or everything in a file; `POST /code/run` writes too with `Write`.

1. **A named block is found in its own file first, then any file** (`findNamedSrc`) - a file of blocks is the library of babel.
2. **`:var` naming a block runs it first** (depth capped at 8), naming a `#+RESULTS:` reads it; table rows are passed to the binder (`bind(name, v, rows)`), never through shared state.
3. **Results are a line splice under the END line or the call** (`placeResult`): blank line then `#+RESULTS:` (with the block's name) when there is none, otherwise only what is under the existing header is replaced. `resultExtent` ends a result where its element ends - not at the next blank line.
4. **`:eval no`/`never` never runs**; `:results silent` runs and writes nothing.
5. **Run-all re-reads the file after every write and takes the next item by its place in the order**, not its row (see **Traps: re-measure**).
6. **Pictures** (`dot`, `plantuml`, `ditaa`, `mermaid`, `gnuplot`) need `:file`; the path is made absolute before the program sees it, because the program runs in the file's directory. gnuplot tables go to temp data files bound by name, as org does.
7. **`:cmdline`** goes after the code's file, split by `shellWords` (quotes, no shell). `beancount` runs `bean-query -f csv` (csv back as a table), `ledger` runs `ledger -f`.

## Small additions from the orgextended pass

- `POST /table/import` (`tableimport.go`): csv/tsv/semicolon text as an aligned table, under a heading, after a line, or at the end; separator guessed from the first line, a comma losing ties.
- Query functions `IsDone()` (a finished keyword by the heading's own file) and `InFile(re)` (matches the absolute path too); `IsPartOfProject()` takes no argument now. Built-in filters gained OrgExtended's views: `Projects`, `ActiveProjects`, `StuckProjects`, `LooseTasks`, `NextTasks`, `DoneTasks`, `Clocked`, `Phone`, `Meetings`, `Notes`.
- `GET /gantt/tasks` carries each task's own `Clocked` spans (worg's Actual time).
- `GET /pandoc?filename=&to=docx` answers with bytes (like `/pdf`), through the `pandoc` exporter's settings when one is configured; the `trello` poller mirrors boards into files.
- `#+HTML_THEME: readtheorg` / a `theme-readtheorg.setup` SETUPFILE pick `rtd` (`themeAlias` in the html exporter); bigblow picks `docs`.

## Inbox triage, the someday sweep and the weekly review

`review_wizard.go` (routes in `registerReviewRoutes`), wire types and the `review:` settings in `internal/common/review.go`, pinned by `review_wizard_test.go`. `GET /review/inbox`, `GET /review/someday`, `GET /review/weekly` (stuck projects with their open tasks, waiting items, last week's clocked vs planned time, stale scheduled items), `POST /review/act` + `POST /review/undo`, `GET /review/last`, `POST /review/step`, `POST /review/done`. Clients: worg's Inbox and Weekly Review tabs, `orgs triage`, `orgs review week`.

- **What the lists are is config** (`server.review`): `inbox` is a query (default the top of every `inbox.org`, not archived, not done), someday is `somedayKeywords`/`somedayTags` (lists, not a query, because promote has to undo exactly what made it someday: set the keyword *and* take the tag off), `waiting` a query, `waitingDays` 7.
- **Only the outermost heading a query finds is an item** (`outermostMatches`): a project in the inbox is one item, and a `:someday:` tag (inherited by every child) makes one item of a tree. `Children` says how many go with it.
- **Every decision is `POST /review/act`, not the individual endpoints, because of undo.** It calls what `/move`, `/date/change`, `/heading/parts` and `/status/change` call, and around that keeps every touched file's bytes before and after (the heading's file, the refile destination, the archive file from `FindArchiveTarget`). Undo writes "before" back **only if the file still reads exactly as the action left it** - the habit untick note above explains why a saved copy is otherwise poison. Because each action's "after" is the next one's "before", undos pop cleanly in reverse order; an edit from anywhere else in between makes the undo refuse, never clobber. An archive that created the archive file removes it again. Kept in memory per user, fifty deep.
- **Waiting since** = the newest `State "WAITING" from ...` logbook line for the keyword it is on now, else a `CREATED`/`ADDED` property, else its own date, else unknown (`Days: -1`, still listed - a waiting item with no date is the forgotten one). `SinceFrom` says which. A chase writes org's `- Note taken on [stamp] \\` / `Chased: ...` into the log drawer and restarts the count.
- **Stale** = open, `ScheduledPast()`, not a habit, not waiting, and no clock/state/note line dated on or after the scheduled day. Logbooks are read from the heading's **own** lines (`ownEnd`: up to the next heading of any level), so a child's clock never freshens its parent.
- **Time** walks every non-archived heading: clocked minutes inside the week (`clockBlockRange`, `headlineClockMins`), planned = EFFORT of a keyworded heading scheduled or due in the week. Attributed to the nearest project at or above it, else its file; and to each inherited tag (not `project` itself when `useTagForProjects`), so tag rows do not sum to the total.
- **The review's step and tally live in the extensions file** (`UserExt.Review`), so a review stopped in worg resumes in the terminal and vice versa, and the end still adds up. `done` writes `last` and keeps twelve in `history`.
- Helpers are prefixed `review…` where a plain name was taken (`daysBetween` is drill's, `sectionProps` records').

## Focus sessions (pomodoro)

`focus.go`, wire types in `internal/common/focus.go`, pinned by `focus_test.go`. `GET /focus` (= `/focus/state`), `GET /focus/history?days=N`, `POST /focus/start|stop|extend|skip`. Clients: worg's Focus tab and header chip, `orgs focus` (`cmd/oc/commands/focus/`).

- **The timer is the server's**: a goroutine started by `registerFocusRoutes` ticks once a second. The session (and 30 days of history) is `focus_data.json` beside `clock_data.json` in the plugin home dir, so it survives a restart; a phase that ran out while the server was down is caught up on the first tick, clocking out **at the moment it ended** (`Clock().ClockOutAt`, added for this) rather than at restart time. AutoNext does not start work after such a gap.
- Work over → clock out (the CLOCK line) → `- Pomodoro: 25 min focus, 5 min break [stamp]` at the top of the clock drawer (a line edit via `logInto`, the heading found again after the clock-out wrote) → break. Break over → idle (or the next pomodoro with `AutoNext`). Stop in work = clock out, no note, recorded as not completed. Skip in work = finish early (counts); skip in a break = start the next one. Every `LongEvery`th break is long; the streak resets after a long break or a gap longer than one.
- **The clock is the truth**: if the clock is stopped or moved to another heading from anywhere, the next tick ends the work phase (`focusstop`). Both clock and session are file+olp, so a hash is worked out afresh on every answer (see **Traps: hashes**).
- Zero/missing lengths in a start mean "what was used last" (default 25/5/15/4), so every client starts with the same lengths. Minutes are floats: `{"Work":0.1,"Short":0.1}` runs a whole cycle in twelve seconds.
- Events: `focusstart`, `focusend`, `breakstart`, `breakend`, `focusstop`, `focusextend` (constants in `focus.go`), each with the heading's hash/headline/file. Every answer is the `FocusState` after the change; a refusal is that state with `Ok: false`, status 409.
- The engine (`focusEngine`) takes its clock and side effects through `focusHooks`; tests use a fake. Testing live: build the binary into a nested dir so its `focus_data.json`/`clock_data.json` stay out of the real ones.

## Time-blocking: the day planner

`planner.go` (routes in `registerPlannerRoutes`), wire types `internal/common/plan.go`, pinned by `planner_test.go`. `GET /plan/day?date=&query=&dayStart=&dayEnd=`, `POST /plan/block {Hash, Date, Start, End}`, `POST /plan/unblock {Hash, KeepDate}`. Clients: worg's Agenda day view (`DayPlanner.tsx`), `orgs plan`.

- **A block is only `SCHEDULED: <2026-10-12 Mon 10:00-11:00>`** - no planner file - so Emacs and every agenda see the plan. The write is `changeDateLines` (the `/date/change` line edit); `/plan/block` exists because the client cannot know what the old stamp carries: its **repeater and warning are kept as written** (`planCookies`), and an empty `End` is EFFORT else 60 min, an empty `Start` the first free gap (`planFreeSlot`; from now, to the quarter hour, on today).
- **Unblock removes SCHEDULED, except on a repeater** (or `KeepDate`), where only the time comes off: a drag back to the list must never end a series.
- **The day** is `buildAgenda` for that one day: timed entries are `Blocks` (a stamp with no end gets EFFORT/60 min, `Implied`; a deadline is a moment, `Mins` 0; a range past midnight runs to 24:00). `Movable` = a one-day SCHEDULED, not done; plain stamps (appointments) and deadlines are fixed. Untimed entries go to `AllDay` unless they are offered as tasks.
- **Tasks** = the query (default `IsTask()`) less done, archived, `COMMENT`ed, habits, anything with a block on the day, and anything scheduled after the day. Scheduled on/before it without a time sorts first (`Late`, days before the asked day), then by deadline, priority, file, line.
- **Load** counts minutes of the working window (default 09:00-17:00) a minute at a time: covered once (`PlannedMins`), twice (`DoubleMins`), outside the window (`OutsideMins`). worg recomputes the same numbers for its optimistic drags (see **Traps: logic said twice**).
- Block headlines have their own stamps stripped (`markerStampRe`): a meeting's `<… 12:00-13:00>` in its title only repeats where it is drawn.

## GitHub and GitLab issues (`plugs/github`)

One package, two yaml names (`github`, `gitlab`) registering the same `Forge` poller with the provider preset; its SDOC (Pollers) is the user documentation and config table. `api.go` (requests, pagination via `Link`/`X-Next-Page`, rate limits), `providers.go` (each api's json → `Item`), `orgfile.go` (scanning and line edits), `sync.go` (the poll). `forge.go` here: `POST /forge/sync` (poll now), `GET /forge/check` (whose token); CLI `orgs forge [sync|check|token github|gitlab]` (`cmd/oc/commands/forge/`), `token` storing into this machine's keyring as `orgs-github`/`orgs-gitlab`. Wire type `common.ForgeSync`.

- **Sync is three-way**: `<P>_STATE` is upstream's state as of the last poll. Upstream ≠ it → the keyword follows upstream; upstream = it but the keyword's done-ness disagrees → a person changed it, close/reopen upstream. The poller always writes keyword and state together, so its own writes can't look like a person's (no loop; `TestClosedUpstreamBecomesDoneAndIsNotPushedBack`). A state is **not recorded when its keyword change failed**, or the next poll would push the stale keyword upstream (`TestAFailedStateChangeIsNotRecorded`).
- **Write order is forced**: pushes upstream, then keyword changes one at a time (each re-reads and finds the heading by `<P>_ID`, since the one before added CLOSED/LOGBOOK lines above - see **Traps: re-measure**), then fields bottom-up and new items in one write. A failed fetch writes nothing: a partial answer would read as every missing item gone.
- **Keyword changes go through the server's `applyStatusChange`** via `PluginManager.ChangeStatus` (`plughooks.go`; plugins cannot import this package). `PluginManager.WriteLines` is `writeLines`, which reloads the db before returning so the next `ChangeStatus` measures the new lines. Both check the file really is the loaded one (`loadedFileAt`): `FindByFile` falls back to base-name matching. A file outside the org directories falls back to the plugin's own keyword+CLOSED edit, with no logbook.
- **Pull/merge requests are closed only with `close: all`, never merged, never when only a review request** (`Item.Mine`); reopen is issues only. Items out of every query are fetched once each; still open there = `gone` (gone keyword, no longer followed or pushed). Repos are filtered client side (GitHub search ORs `repo:`/`org:` unpredictably).
- Tags: `<P>_LABELS` remembers which tags were labels, so a removed label comes off and hand-added tags stay. Title follows upstream (a rename in org is overwritten).
- Testing live: a fake api (python, 127.0.0.1) plus `GITHUB_TOKEN=…` in the server's environment; `orgs forge sync` instead of waiting for `freq`.

## Python sessions, and importing Jupyter notebooks

`babelsession.go` gives python blocks `:session` (no other language has one; theirs is ignored and each block runs alone, as before). `runJob` hands a python job with a session to `runInSession`; `:session none` or none at all is the old fresh-process path.

- **One python per (name, org file)**, started with a driver script (`pythonSessionDriver`, run with `-u -c`). Requests on fd 3, replies on fd 4, each `<length>\n<json>` - never prompt scraping. While a block runs, fd 1 and 2 are `dup2`'d onto one temp file, so prints, subprocess output and warnings land in order. The block's last expression is evaluated separately (Jupyter's execute_result): shown after the output for `:results output`, the whole result for `value` (a top-level `return` is wrapped in a function whose call is that expression). A trailing `;` hides it.
- **Figures**: `MPLBACKEND=Agg`; after a block, every open pyplot figure is saved to `:file` (`-2`, `-3`... for more) or `<base>_files/fig-<sha1 of code>.png`, then closed. A pandas DataFrame/Series value comes back as rows (an org table).
- **Several outputs go in a `:results:` drawer** (`composeResult`/`composePieces`, `resultBody` writes a `drawer` kind as it stands), the emacs-jupyter shape; `resultExtent` already ends a drawer at `:END:`.
- **An exception is written** (Kind `error`, `Ok: false`): `runItem` and `RunBlock` write `res.Ok || res.Kind == "error"`. Driver frames are dropped from the traceback; the code's file is `<notes.org:12>` (`babelJob.Where`).
- **Timeout = SIGINT** (KeyboardInterrupt, session kept, reply written with the interrupt in it); no reply 5s later = kill. Idle an hour = stopped by the reaper; `StopBabelSessions` runs on the server's signal path; a killed server leaves nothing (the python reads EOF on fd 3 and exits). `GET /babel/sessions`, `POST /babel/sessions/stop` (restart kernel).
- `%matplotlib`, `!cmd`, `%pip`, `%%bash`, `%%time`... are rewritten by the driver only when the code fails to parse as python. `:python` picks the interpreter (also without a session); `babel.commands.python` for every file.
- **Header arguments are inherited** now (`inheritedArgs` in `babelexec.go`, for blocks and inline code): `#+PROPERTY: header-args` then `header-args:<lang>`, then the `header-args`/`header-args:<lang>` properties of the headings above, outermost first, then the block's own. `results()` gathers every `:results` word, which is org's merge in effect.
- **An empty result with none there before writes nothing** (`hasResultAfter`), so a block that only defines things gets no bare `#+RESULTS:` - and an imported cell without output does not change on its first run.

`ipynb.go` is the notebook converter (pure; `ipynb_test.go`), `ipynbimport.go` the endpoint `POST /import/ipynb` (wire types `internal/common/ipynb.go`). Clients: `orgs import` (`cmd/oc/commands/importcmd/`), worg's new-file dialog.

1. **Results are written by babel's own writer** (`resultBody`, `composePieces`, `tableResultText`), so running an imported notebook replaces its outputs; `TestAnImportedNotebookRunsAndItsResultsAreReplaced` pins it (the file is identical after a run).
2. **One session for the file**: `#+PROPERTY: header-args:<lang> :session <base> :results output`. A cell with pictures gets `:results graphics file :file <base>_files/cell-N.png` (python only), the name the session gives its figures, so a re-run draws over the same file. Markdown attachments are `<base>_files/md-<cell>-<name>`.
3. Title: `metadata.title` (or v3 `name`), else a level-one first heading (dropped from the body; the remaining headings are shifted up so the shallowest is `*`), else the file name.
4. A dataframe's html becomes an org table (`htmlTable`, pandas' two-row header folded); other html prefers a meaningful `text/plain`, else an export block (up to 100kB). Tracebacks lose their ANSI. Text is capped at 2000 lines per output.
5. `%%bash`/`%%html`/`%%latex` cells become `sh` blocks or export blocks. nbformat 3 is upgraded in place; 2 and older are refused with a message.
6. The endpoint refuses paths outside the org dirs (`insideOrgDirs`), non-`.org` names, and an existing file *or picture* unless `Overwrite` (409); `DryRun` writes nothing, warns about a clash instead of refusing, and answers the org text. An overwrite stops the file's sessions.

## Saved-query notifications (watches)

`querywatch.go` (wire types `internal/common/querywatch.go`, pinned by `querywatch_test.go`): a watch is a saved query by name (read afresh each check) or an expression of its own, kept per user as `queryWatches` in the extensions file. `GET/POST/DELETE /watches`, `POST /watches/check`, `GET /notifications` (`unread=t`, `limit`), `POST /notifications/read|clear` (`{ids}` or `{all:true}`). Clients: worg's header bell and the Saved Queries tab's per-row bell, `orgs q watch|unwatch|watches`, `orgs notifications [-unread] [-read] [-clear] [-follow [-exec]]`, and `orgs watch -kinds querymatch`.

- **The first check is a silent baseline**: after a server start (baselines are memory only), on POST /watches, when the query text changes, and on un-pausing. Otherwise every restart announces every match as new.
- **Identity** (`watchKey`): `ID`, then `CUSTOM_ID`, then file + outline path (ordinal `#2` for twins), then hash (see **Traps: hashes**). A gained and a left match without ids in the same file at the same line or with the same title are one heading edited and are not reported (`diffMatches`).
- **Checks are debounced on `OrgDb.ReloadIndex`** (read once a second, check after 1.5s still), plus every five minutes and at a day change for time-based queries. New query functions for those: `DeadlineWithin(n)`, `ScheduledWithin(n)` (today to n days ahead, calendar days, past excluded; also in worg's `QUERY_FUNCTIONS`).
- A query that stops parsing keeps its baseline and reports `error` on the watch; a POST with an unparsable query or a missing saved query is refused (400).
- Delivery: a `querymatch` event (`watch`, `label`, `count`, `notice`, `headings`, `left`, `msg`) and, with `desktop`, a notification through `deliverNote` (see **Notifications**), its click opening the first new heading. **Events now carry an unsent `User`**: `RequestEvents` drops another user's, since one stream serves every login.
- Notifications: newest first, fifty per user, 100 headings each, in `querywatch_data.json` beside `focus_data.json`. The engine takes its clock, runner and delivery as fields, so the tests need no database.

## Insights: estimates against the clock, and dependency health

`insights.go` (routes in `registerInsightRoutes`), wire types `internal/common/insights.go`, pinned by `insights_test.go`. `GET /insights/estimates`, `GET /insights/deps`. Clients: worg's Insights tab, `orgs estimates` and `orgs deps` (`cmd/oc/commands/insights/`).

Estimates:
- **Measured** = a finished task (not CANCELLED and kin) with an EFFORT and clocked time, in the range by its CLOSED (else the end of its last clock). Its actual is **every** clock it has (the estimate was for the whole task, not the range) plus the clocks of headings under it with no EFFORT of their own. A heading with an EFFORT over other estimated headings is a **rollup**: counted, never measured, or the same work is counted twice. Open tasks only with `open=t`, flagged `InProgress`.
- **Nothing is silently dropped**: `ClockNoEffort` (time with no estimate above or below it, minutes in the range, the ten biggest in `Unestimated`), `EffortNoClock`, `Rollups`.
- **A day of EFFORT is 24h by default** (org's own, and what the weekly review and column view sum); `day=8` rescales every `d`/`w`/`y` (`effortMins`; a week stays five days, as `ParseDuration` has it). `DayEfforts` says how many estimates are written in days, which is when it matters; both clients offer the switch.
- **The multiplier is the median per-task ratio** rounded to a tenth, only for three tasks or more, because one big miss should not set the plan; the group's `Ratio` (total over total) is beside it. Spread is the 25th-75th percentile (interpolated). Worst lists are ranked by **minutes** missed, not ratio - a 5 minute task taking 30 is 6× and matters to nobody.
- Person is `assigneeOf` (`ASSIGNEE`, `ASSIGNED`, `RID`, `RESOURCEID`, `[..]` stripped) - worg's `assigneeOf` in `ganttplan.ts` (see **Traps: logic said twice**).

Dependencies (`buildDepGraph`, `analyzeDeps`):
- **The gantt's rules, re-implemented over an outline the walk builds itself**, not `mermaid.After`: a health check needs to know *why* (AFTER or ORDERED, which id was missing) and must walk past a cycle to report it. Parents are the walk's own pointers, never a parent hash (an untitled heading once shared its parent's hash; see `docs/claude/bug-history.md`). AFTER is looked up exactly as written (hash, ID, CUSTOM_ID), as the gantt does; `id:x` or two words get a suggestion saying so rather than a silent resolve. The first child of an ORDERED parent waits on what the parent waits on.
- **Every heading waits on at most one other**, so cycles are found by colouring (on the path / finished): each heading is visited once and each cycle reported once, from whichever heading reaches it first. Also reported as `cycle`: an open heading AFTER its own open ancestor (a deadlock, not a loop), and a heading with its parent's hash.
- `waiting-on-done` is through an AFTER only: a WAITING step of an ORDERED project is usually waiting on somebody outside, and `ordered-stuck` already reports it. A blocked chain behind such a step, made only of its siblings, is left to `ordered-stuck` too, so one stuck step is one problem.
- Each chain heading carries `After`, `NoBlocking` and `Chained` (would an ORDERED parent chain it once AFTER is cleared) - what worg's `unlinkOp` needs to take the wait away with `/property` writes.

## Encrypted headings: revealing, sealing, editing

`crypt.go` is org-crypt itself (design notes at its top); `cryptui.go` adds what a reader needs: `GET /crypt/state` (held or not, `UnlockSeconds`; runs no gpg, reads no file, so it can be polled), `GET /crypt/file` (one file's tagged headings, metadata only), `POST /crypt/reveal`, `/crypt/seal`, `/crypt/edit`. Wire types `CryptState`, `CryptOp`, `CryptOpResult` in `internal/common/crypt.go`; tests `cryptui_test.go` (skip without gpg). Clients: worg (`docs/claude/worg.md`), `orgs crypt`, the tui's `x`/`E`/`L`.

1. **A passphrase is held only after it has worked.** `passFor` used to hold whatever arrived before gpg had answered, so a typo replaced a good held passphrase; it now only chooses, and handlers call `holdAfter` on success. `/crypt/reveal`, `/seal` and `/edit` hold only when the request says `keep` (the older endpoints still hold whenever `unlockSeconds` allows).
2. **Symmetric mode with no passphrase never reaches gpg** (`NeedPassphrase`): gpg would go looking for one, which on a desktop is a pinentry window on the server's screen. `WrongPassphrase` is gpg's "Bad session key"/"Bad passphrase", told apart so a client can say "try again" rather than "stop".
3. **A heading is named by hash or by file + its headline's row, exactly** (`cryptOpTarget`). A row inside the body is refused, not taken as the heading above: a reveal hands over plaintext and must not guess.
4. **Seal tags the headline after encrypting** (a line edit), because an armoured body under an untagged heading is invisible to the listing, the sweep and Emacs's `org-decrypt-entries`. A failed encryption leaves the file untouched; the hash is looked up again afterwards.
5. **Edit checks before it writes**: the passphrase must open what is there (a typo cannot re-encrypt the note under a passphrase nobody knows), and `base` (sha256 of the revealed plaintext) must match what is there now. Empty text is refused. The splice is `spliceArmor`, shared with `/crypt/update`.
6. **Plaintext is never written or logged.** Reveal renders through `drillRender` when `html` is asked for; nothing caches it. The request log prints paths only. The trigram index (`.orgs-index`) holds trigrams of what is on disk, so a heading in the clear before it is sealed is there until its file is re-indexed (on the write).
7. **The html exporter does not write the armour as written**: `-----` becomes `—–` (smart dashes) and the blank line after the header splits it into two paragraphs. worg's `replaceArmorHtml` matches that; anything else that wants to hide the block in html has to as well.

## Structural refactors: tag rename, split, merge, list to headings

`refactor.go` (wire types `internal/common/refactor.go`, pinned by `refactor_test.go`): `POST /refactor/{tag,split,merge,list}/{preview,apply}`, `GET /refactor/tags` (every tag with headline/FILETAGS counts), `GET /refactor/lists?hash=` (the lists in a heading's own text). Clients: worg's Refactor tab, `orgs retag`/`split`/`merge`/`list2headings`.

1. **One computation answers both halves.** A `refactorPlan` reads each file once, edits lines in memory (see **Traps: line edits**) and answers per file with `Before`, `After`, its `Hash` and a unified `Patch` (Myers over lines, `lineDiff`, for the terminal; worg diffs `Before`/`After` itself). The apply runs the same function on the files as they are now and **refuses (`Stale`) unless its `Token` - a hash over every file's before and after - equals the preview's**. A minted id would make the second run differ, so a split's preview answers `NewId` and the apply sends it back. Applies are serialised (`refactorMu`); new files are made first with `O_EXCL`, so one that appeared since the preview is refused before anything is touched.
2. **Tag rename** walks raw lines: headline tags through `setHeadlineParts` (right-aligned tags keep their end column), `#+FILETAGS:` in either spelling (`:a:b:` / `a b`; left empty, the line goes), `#+TAGS:` words keeping fast keys and group markers. Lines inside blocks are skipped. A new name already on the heading is a merge (old dropped, not written twice). `Query` limits it to the matched headlines and leaves keyword lines alone. Filters, tag groups and every user's saved queries naming the tag are **reported in `Mentions`, never changed**.
3. **Split** writes `#+TITLE:` plus the source's `#+FILETAGS`/`#+TODO`-family/`#+TAGS`/`#+CATEGORY` lines (or the keywords stop parsing and tag searches stop matching), then the subtree promoted to level 1 (`relevel`: text moves left by one amount for every line, from the least indented, so blocks keep their shape; column-zero text does not move). The heading keeps its drawer and gets an `ID` in the new file if it had none. Relative `file:`/`./` links are rebased. **The original becomes one headline at the same level titled `[[id:…][Title]]`, no keyword or tags** (a list item could not stand where a heading stood - it would join the previous sibling's body - and a keyword or tags would show the task twice). Warns about inherited tags lost and `[[*Title]]` links that cross the cut.
4. **Merge** (From into Into): body after Into's body, children after Into's children re-levelled, tags unioned, properties added where Into has none (Into wins; conflicts in `Warnings`), LOGBOOK entries appended, planning taken only if Into has none. **IDs**: Into without one takes From's (nothing else changes); with both, every `[[id:From…]]` in every file is re-pointed (shown in the preview) and From's id is kept on Into as `MERGED_IDS`. An alias property was rejected: only a resolver that knows to look for it would honour it, and Emacs' org-id does not. Merging a heading into its own descendant is refused.
5. **List to headings**: the list (`findLists`: items at its indent or deeper, text indented past the bullet, two blank lines end it; blocks and drawers skipped) is cut out and its items written as children at the end of the heading's own text, before existing children - text after the list stays above them (warned). `[ ]`/`[-]` → the file's first open keyword, `[X]` → first done (`#+TODO` line, else the server's), `term :: desc` → title and text. Item text keeps the file's style (indented when the list was).
6. Properties are added with `addProps`, **not `setPropIn`**: `setPropIn` realigns the whole drawer, which would put every existing property line in the diff.

## Notifications

One path for everything that pops up: `deliverNote` (`notifydeliver.go`) publishes a `notify` event (`title`, `msg` = body, `url`, `group`, `urgency`, `sound`) and hands the note to every plugin with a `Deliver` method (found by method; plugins reach it as `PluginManager.Deliver`). Callers: the notify plugin's reminders and deadline digest, watches with `desktop`, and focus phases ending on the timer (`focusNote`; a stop or skip someone pressed is not announced). `GET /notify/status`, `POST /notify/test`. Client: `orgs notify` (`cmd/oc/commands/notifycmd/`).

- **Showing it is `internal/common/desktop`**, per OS: macOS terminal-notifier (click opens, `-group` replaces) else osascript; Linux the freedesktop D-Bus service (replaces_id per group, an Open action running `xdg-open`, urgency hint) else notify-send; Windows a go-toast toast (PowerShell AppID, protocol activation). The command builders are pure and tested; Linux and Windows are type-checked (`GOOS=linux|windows go vet`) but untried on real desktops.
- **The plugin's settings** (`plugs/notify`, SDOC has the yaml): `remind: [10, 2]` minutes before a timed appointment (each offset once; a late start sends only the smallest crossed one), `deadlines: "08:30"` a once-a-day digest (overdue, due today, warning), `quiet: "22:00-07:00"` (crossing midnight; critical still shows), `sound`, `focus`, `desktop: false` (events only, for a server whose screen nobody sees), `open` (worg's address for a click), `icon`. Old `beep`/`notifyBeforeMins` are still read.
- **What was sent is kept** in `notify_data.json` in the plugin home dir (reminder keys with an expiry a day after the start, the digest's day), so a restart repeats nothing.
- **A click opens worg at the heading**: `common.HeadingLink` → `?app=Files&open=<file>&line=<0-based>&heading=…`. The base is `open`, else `http://localhost:<port>`. `StartServer` copies the `-port`/`-tlsport` it was started with into `PlugManager` and `Conf().Server`: the yaml's port was in the links while the flag's was being served.

## Org made elsewhere: `POST /import/org`

`orgimport.go` (wire types `internal/common/orgimport.go`, `orgimport_test.go`). The markdown/Obsidian, todo.txt and iCalendar importers convert **on the client** (`cmd/oc/commands/importcmd/formats.go`, with `internal/common/mdorg`, `todotxt`, `ical`) and send org here, so where a file may go and what it would clash with are decided by the server that owns the directories.

- Paths go through `insideOrgDirs`; files must end in `.org`; a file already there is refused (409) unless `Overwrite` or `Append`. **Assets are written first**, so a file never links to a picture not yet there; an asset already there is kept (and said) unless `Overwrite`, because it is nearly always the same picture from the last import.
- **`Append` adds only top level entries** (`importEntries` splits at `* `) and leaves off the new text's `#+KEYWORD:` head. With `SkipKnown`, an entry whose `KeyProp` (default `ID`; calendars use `ICAL_UID`) the file already holds anywhere (`knownKeys`, any case, any indent) is left out and counted in `Skipped` - which is what makes re-importing a calendar idempotent.
- `DryRun` answers each file's text (what an append would add, not the whole file) and turns a clash into a warning.

## CSV: `GET /csv`

`csvexport.go`. `file=` is `BuildColumnView` (rollups and all); `query=` is `QueryColumnView` - `queryEachSection` (filters expanded, as `/search`) with `ownRow` per heading, nothing summed, default columns `queryCsvColumns`. So the spreadsheet's cells are the Columns tab's cells. `sep`, `header=f`, `bom=t` (Excel needs it for utf-8), `meta=t`, `own=t`. Clients: `orgs csv`, `orgs cols -csv`, `orgs q -csv`.

`BuildColumnView` used to read property drawers from `fileLines(filename)` - the name *as asked* - so `/columns?file=plan.org` (relative to the org directory, which `FindByFile` accepts) found no drawer and every EFFORT was blank; `orgs cols` and worg never noticed because they send absolute paths. It reads `f.Filename` now.

## Exporters that need no yaml

`exporterNamed` (`export.go`) is the configured exporter of that name, or else a registered one implementing `common.Standalone` (made and started once, on first use). `/file/{type}` and `/exporters` (`ExporterNames`) both go through it, so `todotxt`, `ics`, `beamer` and `typst` work on a bare config. Keep `Standalone` for exporters with nothing to configure: one that needs a url or a password would fail at the first request instead of saying what is missing.

- **`todotxt`** (`plugs/todotxt`) and **`ics`** (`plugs/ics/export.go`, beside the old ics *poller*) take a file (every heading) or a query (`plugs.Headings`); titles go through `plugs.PlainText` (links as description + url, emphasis dropped). The rules live in `internal/common/todotxt` and `internal/common/ical`, shared with the importers, so a round trip is read by the rules it was written by (`TestRoundTrip` in each).
- The `ics` uid is `:ICAL_UID:` (what the importer wrote), else `:ID:`/`:CUSTOM_ID:`, else a hash of file name and outline path, so exporting again updates events rather than duplicating them. A timed stamp with no end lasts its `EFFORT` (H:MM) or `duration` (60). A VTODO with an all-day DUE writes DTSTART as a day too (RFC 5545 wants them the same kind).

## The paper planner: `GET /agenda/print`

`plannerprint.go` (`plannerprint_test.go`; client `orgs print`) lays out `buildAgenda`'s answer and works out no dates of its own. Carried-forward overdue entries (`Late > 0`) leave today's column for the Overdue panel, so each appears once. A deadline's own day says "due" even while it is warning (the agenda gives it `DueIn` there too); other days "in Nd". With the habits strip on (the default), habits are left out of the day lists and Overdue, since a daily habit would fill every cell. The tasks panel is `IsTask() && !HasAnyDate()`, NEXT/STARTED/DOING first, then priority - not `Todo.IsActive`, which is true of every open keyword.

The page is `html/template` + `templates/planner/base.css` + one `planner/themes/<name>.css`, all found with `assets.FindTemplate` and inlined, so a theme is one file. Sheets are sized by the server (`--sheet-w`/`--sheet-h`) a hair under the page, so each prints as exactly one page. Two Chrome printing traps: a pattern on many elements (a dot grid on every month cell) becomes a picture per element (a 3.9MB pdf) - put it on the sheet once (`--sheet-bg`); and a tinted paper needs `@page { background }`, or the margins print white.

## Beamer pdfs

`beamerpdf.go`: `GET /beamer/pdf?filename=&theme=&code=&refresh=` compiles the `beamer` exporter's tex (cached in memory on a hash of the tex) and `GET /beamer/themes` lists the presets. Notes on the exporter itself are in `plugs/beamer/CLAUDE.md`.

## Keeping blocks current: `refresh.go`

`GET /refresh`, `POST /refresh/run`, a ticker started by `registerRefreshRoutes` (every `refresh.tick` seconds, default 30), records in `refresh_data.json` beside `focus_data.json`, pinned by `refresh_test.go`. Client: `orgs refresh`. Four kinds of entry, found by `refreshEntriesOf` (rescanned only when `ReloadIndex` moves):

- **`block`**: a source block whose job (`jobForBlock`, so inherited `header-args` count) has `:schedule`; keyed by `#+NAME:` else language + hash of the code, so it survives lines moving. **`call`**: a `#+CALL:` with `:schedule` in its own header arguments, keyed by a hash of the line. Both run through `runItem` - the same path as `orgs code at` - so results, sessions and `babel.enable` (checked in `runJob`) are babel's own; an entry says `Blocked` rather than failing every tick when babel or `refresh.scheduled` is off.
- **`dblock`**: a dynamic block with `:refresh` (generators ignore the extra parameter), keyed by its `#+BEGIN:` line and ordinal; **`file`**: `#+REFRESH:` above the first heading, or `refresh.every` for files with a dynamic block or a `#+TBLFM:` - all dynamic blocks, then every formula (`refreshFile`).
- Schedules (`parseSchedule`): spans (`30m`, `1h30m`, `2d`, `1w`, floor one minute), `hourly`/`daily`/`weekly`, or a time of day (`07:30`, `daily 07:30`: due once at or after it). Never run = due now; the record survives restarts, so a restart does not rerun everything.
- **Nothing is rewritten for nothing**: `writeBlockResult` and the call path compare before writing (`CodeResult.Unchanged`), `dynBlockLines` already did, and `recalcTableLines` compares the table rendered before and after its formulas - not the file's text, which a hand-aligned table never matches. A clock table's `#+CAPTION:` carries the minute, so it does change each run, as in Emacs.
- **Exports read a refreshed copy** (`exportFile`/`exportView`/`exportFileStamped`): `RequestFile` (unless `subtree=` or `refresh=f`), `/typst/pdf` (the stamp joins its cache key, which is otherwise the file's mtime), `/beamer/pdf` (keyed on the tex already), and `/site`. `refreshFile(…, false)` writes nothing; the document handed over has the formulas' values in it.
- **`dynBlockLines` reloads only a changed file** (`OrgDb.ReloadIfChanged`). It used to `ReloadFile` unconditionally, harmless for `orgs dblock`, but an export goes through it now, and every reload is a `reload` event that sends worg back for the page - a loop.

## Tangle, detangle, auto-tangle

The tangler is `plugs/tangle` (exporter `tangle`, `GET /tangle`, `orgs tangle`); `detangle.go` here is `POST /detangle`, auto-tangle and the ledger; tests `plugs/tangle/tangle_test.go`, `detangle_test.go`.

- **Blocks** come from `collectBlocks`, which must unwrap `org.NodeWithName` (a `#+NAME:`d block) and `org.NodeWithMeta` (`#+CAPTION:`/`#+ATTR_`) - before it did, named blocks were never tangled. It records each block's heading chain, its 1-based index in its heading's own text, and the paragraph text before it.
- **Header arguments** (`argsFor`): go-org keeps *every* `#+PROPERTY:` line under the single key `PROPERTY` (newline-joined), so `doc.Get("header-args")` is always empty - the old reading never saw a file's header-args. Order: file plain, file `:lang`, each heading outermost first (plain then `:lang`; property keys are upper-cased by go-org), then the block. Languages match by family (`sh`/`bash`/`shell`). The plugin cannot import this package, so this is a second reading of what `inheritedArgs` does at run time; keep the two in step.
- **Noweb** references are expanded by `internal/common/noweb`, shared with running a block: a name can't start or end with whitespace (C++ `<<`/`>>` is not a reference); `:noweb-prefix` (default yes) repeats the text before a reference on every line; an unresolved one stays as written with a warning (`TangleResult.Warnings`); depth 20 is an error. `<<name(args)>>` goes through `tangle.Evaluate`.
- **Code** is dedented (unless `-i`) and org's comma escape undone (`unescapeLine`); detangle re-escapes (`EscapeLine`) and re-indents to what the block's body already uses. `:tangle yes` is `<org name>.<ext>` (`tangleExt`, org-babel-tangle-lang-exts).
- **Markers** are org's: `PRE[[file:<org, relative to the tangled file>::*Heading][Label]]POST` … `PRE Label ends here POST`, `Label` = `#+NAME:` else `Heading:N` (above the first heading, the org file's name). `commentDelims` gives each language's comment form, block comments (`/* */`, `<!-- -->`) included; json gets none.
- **Detangle** (`Detangle`) parses markers (`tangle.ParseMarkers`; nested markers = a noweb expansion, refused), finds the org file by the link relative to the tangled file and then by name, the block by label (`tangle.Blocks`), and splices bottom up per org file. A block with `<<refs>>` it would expand is skipped: its tangled text is the expansion. The server reads the tangled file itself only inside the org directories; the client sends `Text`.
- **Auto-tangle**: `LoadFile` calls `autoTangleAfterLoad` on a *re*-load only (`have`: a server starting is not a save); it is debounced 400ms and only for `#+auto_tangle: t` (`AUTO_TANGLE`). `tangle.WriteFile` writes nothing when the file already says it. **The ledger** (`tangle_data.json` in the plugin home, fed by `tangle.OnWrite` from every write, `orgs tangle -w` included, and from "already right") holds a hash of what orgs last wrote to each target: a target on disk that matches neither that nor the new content was edited by hand, and auto-tangle stops for it (event `tangled`, a stderr line) rather than overwrite it. A target orgs never wrote is written (the user just turned auto-tangle on). After a detangle the next auto-tangle finds the file already right and re-records it, so it does not conflict.

## Babel values and the run pipeline

`babelvalue.go` (the value model), `runJob` in `babelexec.go` (the pipeline), binders in `babel.go`; tests `babelfeatures_test.go` (real interpreters, skipped when missing).

- **A `:var` resolves to a `babelVal`** (`resolveValue`): scalar as written (code: `:var s="x"` stays a string literal), text (from the file: a cell, a result - a number if it reads as one), list, or table with **nil rows for rules**. `j.bound` holds values given outright (`:post`'s `*this*`). `other.org:name` is read in `readVar` (`crossFile`) into `RefFile`.
- **Index before shape**, as org (`org-babel-ref-index-list`): rules count as rows and pass through; a level that comes out one long is unwrapped (so `[2,1]` is a cell, `[,0]` a column); ranges are inclusive, negatives from the end.
- **Shape** (`tableShape.prepare`): header off when a rule follows the first row (or `:colnames yes`), first column off with `:rownames yes`, rules out unless `:hlines yes` (then the binder's null: `None`, `nil`, `hline` in elisp). The first table's header/rownames are put back on a *table* result of the same width/height (`reassemble`) - **unless the result already has a header over a rule** (sqlite's, a csv's), or it doubles. This changed behaviour: a table's header used to be passed as a data row.
- **Order in `runJob`**: noweb (`nowebOnEval`, `internal/common/noweb` - same engine as tangle; `Body` = noweb-ref blocks then the named block, `Call` = `callJob`+`runJob`) → picture prepare → value wrap (python; `pp` swaps in `pprint`) → variables (tables to files for `tableFile` languages: gnuplot `.dat`, sqlite `.csv` *with* the header so `.import` names columns) → `assemble`/prepend → `:prologue`/`:epilogue` → `:cache` hash (`babelHash` over the final program, args, stdin, results words, file; `j.cachedHash` is read from `#+RESULTS[...]` by the caller; a match returns `Cached`, written by nobody) → run → **non-zero exit = Kind `error` with stdout + stderr** (written, like a session traceback) → shape by `:results` → `finish`: `reassemble`, `:post` (via `callJob`, `*this*` bound), `:wrap` (Kind `wrap`, written as it stands).
- **`:results` merge** (`results()`): words in org's four groups; a later word replaces an earlier one *of its own group*; inherited args come first, the block's last. A test that relied on words piling up was updated.
- `placeResult` takes the hash and rewrites the header's `[hash]`; `resultsLineRe` already allowed it. `runProgram` takes `stdin` (only usable when the program goes in a file: awk now does, `-f block.awk`).
- go: a body with `package` gets its vars after the imports; one without is wrapped (`package main`, imports from `:imports` plus `fmt` when used, `func main` when missing). elisp: `(let (binds) code)` - a variable called `t` is a constant there and fails, as in Emacs.
- `tangle.Evaluate` is set in `detangle.go`'s init to run `<<name(args)>>` through `callJob`/`runJob`, so tangling such a reference needs `babel.enable`.

## Writing mode's server half: `writing.go`

`GET/POST /writing/snapshots` (a file's snapshots in `.snapshots/<name>.json` beside it, written via a temp file and rename, one writer at a time), `GET/POST /writing/log` (`UserExt.WritingLog`), `POST /compile` (org text from worg exported as though it were the file: `bookBytes` in `export.go` for docx/epub, shared with `/book`; `typstExporter().PDF` for pdf, since the `pdf` exporter only writes a file it reads from the database; else `ExportToString` over a `subtreeDb`). Notes on the worg side in `docs/claude/worg.md` (Writing mode).

`languagetool.go` runs LanguageTool for writing mode's checker the way `whisperd.go` runs whisper (`server.languageTool` in yaml; `StartLanguageTool`/`StopLanguageTool` in `StartServer`, its own signal handler, which also stops whisper - and whisper's stops it, since whichever handler exits first wins): java on `path` (`languagetool-server.jar`) else `docker run --name orgs-languagetool -p 127.0.0.1:<port>:8010`; something answering `/v2/languages` on the port is adopted. Default port 8082, because whisper's is 8081 and an adopted whisper would look like a broken LanguageTool. `/languagetool/check` passes the form body through (re-encoding `r.PostForm` if something parsed it first), `/languagetool/languages` likewise, `/languagetool/status` says state and the child's last lines. macOS has a `/usr/bin/java` stub with no runtime, so `LookPath` passes and the failure shows as the child's output in status.

`assistant.go` is the writing assistant (`server.assistant`): `GET /assistant/status` (backends, ready or why not, never keys) and `POST /assistant/chat` (newline-delimited JSON: `{"t"}` pieces, then `{"done"}` or `{"error"}`). Claude through `anthropic-sdk-go` (beta messages: the system prompt then the writer's text as a second block with `cache_control`, so a conversation about one file reuses its prefix; `fallbacks: "default"` under `server-side-fallback-2026-07-01`), ChatGPT/OpenAI-compatible and Gemini through their REST streams (`sseLines`). Keys: literal, `env:`, `keyring:` (`common.KeyringGet`), else the usual variable. **It reads no file but the `Others` named in the request, each `FindByFile`-checked**; the current file's text comes from the editor. A `Model` not in the backend's `model`/`models` is refused. Adding the SDK raised `golang.org/x/oauth2` and `google.golang.org/api` (minimum versions), which the googlecal plugin builds against unchanged. Keys set from worg (`assistantkeys.go`) are tried first, kept in the keyring (`assistant-<backend>`) or a 0600 file beside the config, and outrank orgs.yaml and the environment.

`assistantcode.go` is the `claudecode` backend (off unless `assistant.claudecode.enabled`): each turn runs `claude -p --output-format stream-json --include-partial-messages` in an empty temp folder, the conversation so far as one prompt on stdin, the system prompt from `--system-prompt-file`. Its tools are orgs's, over MCP (streamable HTTP, plain JSON answers) from a listener on 127.0.0.1 opened for the turn at a secret path: `list_files`, `read_file`, `find`, and `edit_file` when the request's `Edit` names a file. The open file is read from and edited in `Doc` (the editor's text), never the disk: an edit goes to worg as `{"edit": {file, old, changes}}` with **UTF-16** positions (CodeMirror counts those) and is made there (`placeEdit`); another file is a string splice on disk + `ReloadIfChanged`. Each tool call also sends `{"tool": "Read x.org"}` for the pane. `Others` are not put in its prompt (it reads them on demand). Status asks `claude auth status --json` (cached a minute). `ORGS_DEBUG_CLAUDECODE=1` logs its events and MCP calls to stderr. Flags that cost time:
- **`--safe-mode` drops MCP servers given with `--mcp-config` too** (no tools, and the model writes fake `<function_calls>` as text). `--restricted --setting-sources=` keeps them and still leaves out the user's CLAUDE.md, hooks and plugins.
- **`--tools ""` hides MCP tools as well**; `--tools` names the MCP tools instead (no built-ins).
- **A tool schema with `"required": null` makes the CLI refuse the whole list** (it asks `tools/list` again and again, then runs with none); send `[]`.
- **`--bare` never reads OAuth or the keychain** (API key only), so it would defeat the point.
- `ANTHROPIC_API_KEY`/`ANTHROPIC_AUTH_TOKEN` are removed from its environment (either makes it bill the API), and `CLAUDECODE` (a server started from a Claude Code session would be refused as nested).

`ollama.go` and `assistantollama.go` are the `ollama` backend (off unless `assistant.ollama.enabled`; settings `common.OllamaSettings`, which inlines `AssistantBackend`). The chat is a **native tool loop** against `/api/chat` (stream, `options.num_ctx`), not the claude CLI: tool calls are run with `codeFiles.call` (the same tools, files and `{edit}`/`{tool}` events as `claudecode`), answered as `role: tool` messages, up to `ollamaRounds`. A model whose `/api/show` capabilities lack `tools` is not offered them (Ollama refuses the whole request with "does not support tools"; an unknown one is retried without) and worg gets a `{"tool", "warn": true}` note; so does a prompt estimated past 90% of `num_ctx` (Ollama drops the start silently). `num_ctx` = the request's `NumCtx` (worg's `ollamaContext`), else `contextLength` (16384), clamped to the model's `context_length`. `ollama.go` finds the binary (command, PATH, Ollama.app, else `<user cache>/orgs/ollama/<tag>`), installs the latest release (tag from the GitHub API; the redirect's url does not carry it), unpacks the **whole** archive (binary + runner libraries + symlinks; Linux is `.tar.zst`, via klauspost/compress), adopts a server already on the url, else runs `ollama serve` with `OLLAMA_HOST`/`OLLAMA_CONTEXT_LENGTH`/`OLLAMA_MODELS`/`OLLAMA_KEEP_ALIVE` (local urls only), and stops it on the signal paths (`StopOllama` beside `StopWhisper`). Pulls run in goroutines with per-layer progress summed, kept two minutes after finishing for the status. Two things that cost time: an install must leave the `installing` state before `ensureOllama` (which refuses while installing) can start it; and `/assistant/status` must ask the key (`ollamaReady`, which starts the server) before the model list, or the model comes back empty. Testing: give the throwaway server its own `HOME` (the install goes to `$HOME/Library/Caches`), `modelDir` and port, and `qwen3:4b` - 1.7b calls tools but cannot finish a task.

`localrt.go`, `locallamacpp.go`, `locallmstudio.go`, `localmlx.go`, `localhf.go` and `assistantlocal.go` are the `local` backend (off unless `assistant.local.enabled`; settings `common.LocalSettings`): llama.cpp, LM Studio and MLX, each a `localRuntime` supervised by a `localRT` with Ollama's states and rules (adopt what answers, start only on a local url, Stop keeps it down, admin only). The chat is `openAIChat` in `assistant.go`, the OpenAI-compatible tool loop (`chatgpt` takes it too with `tools: true`); its tools are any `assistantToolset` (`codeFiles` today), which is the seam for giving the assistant other surfaces' tools. Models are `runtime:name`; servers under `assistant.local.servers` (vLLM...) are only talked to. Pinned by `localrt_test.go` (stream joining, the loop against a fake server, the no-tools retry, `whereURL`, asset names, gguf picking, safe files). Each runtime was run end to end through orgs on 2026-10-23 (install from nothing, download, start on demand, tool call, edit). Things that cost time or are easy to break:
- **llama.cpp's "latest" GitHub release is a library version with no binaries**; the builds are the `bNNNNN` pre-releases, so `llamaRelease` walks `/releases` for the newest numbered one with the asset. Router mode (`--models-dir`, `--models-max 1`) swaps models by name but **reads its folder only at start**: a pull or delete marks a running router `stale` and it is restarted before the next use (an adopted one cannot be, and `prepare` says so).
- **MLX stops at 512 tokens unless told** (`max_tokens` on every request, `--max-tokens` at start), gets models **by path** with `HF_HUB_OFFLINE=1` so a chat never downloads, and is installed into a venv built **in place** (a venv moved after building has scripts naming the old path). macOS's `/usr/bin/python3` is 3.9; `python()` looks for a 3.10+ one (Homebrew's) and caches it, since the status is polled.
- **LM Studio's `lms daemon up` / `server start` return at once**: no child to wait on, so a start is recorded as `detached` in the pid file and stopped with `lms server stop`. Switching models unloads every loaded LLM instance first (else LM Studio keeps both). Its API has no delete or cancel.
- **A child is recorded in `<cache>/orgs/<runtime>/server.pid`** and claimed (if `ps` says it is still that program) by the next orgs, because another signal handler may exit before this one stops it. Two stops at once (the signal handler and `StartServer`'s own) are serialised (`stopMu`), or the second returns at once and the process exits under the first.
- **Only files that hold no code are fetched** (`localSafeFile`: gguf, safetensors, tokenizer json/txt/model/jinja); a repository with `.py` is refused; no runtime gets `--trust-remote-code`. Paths from Hugging Face go through `hfCleanPath`, deletes through `insideDir`.
- Installer output is read with `scanLinesOrReturns` (progress bars redraw with `\r`) and a bar becomes a percentage on the status message (`progressBarRe`), not a log line. Thinking models' leading blank lines are trimmed in the loop; reasoning arrives separately (`reasoning_content`/`reasoning`) and is not shown.
- Testing: a throwaway server with its own `HOME` (installs and pid files go to `$HOME/Library/Caches/orgs`), per-runtime `url`s off the defaults (8091, 1234, 8092), and Qwen3-4B - 0.6B does not call tools.

`assistantview.go` is the assistant in worg's views (column view, gantt, kanban, search; `assistantReq.View`). `chatgpt`, `gemini` and `local` get `viewTools` (an `assistantToolset`): list/find (the query language)/read, and with `Edit` set_status/property/tags/priority/date, add_heading, set_after, plus suggest_query (not in the column view, which shows a file). `claude` gets the first headings in its prompt and no tools; `claudecode` and `ollama` are refused (they are bound to `codeFiles`). Pinned end to end with a scripted fake OpenAI server as a `servers:` entry, and run with Qwen3-4B on llama.cpp (2026-10-23).
- **Headings are named to the model as turn ids (h1...) kept as file + outline path** and resolved again at every call (`GetFromTarget` `file+olp`), never as hashes: a turn makes several writes and each changes the hashes after it. Twins (same outline path) resolve to the first.
- **Writes reuse the endpoints' own functions**: `ChangeStatus`, `ChangeProperty`, `ChangeDate`, `setHeadingParts` (pulled out of `PostHeadingParts`), `ganttAdd` (out of `PostGanttAdd`), `ganttLink`. All reload the database before returning, so the next call sees the change.
- **A turn is undone whole** (`POST /assistant/undo`): each file is kept before its first write and after the turn, and put back only if it still reads as the turn left it (`/review/undo`'s rule). Tokens are per user, in memory, fifty kept.
- **The query language is described from the function map itself** (`queryFunctionNames`, captured in `guardQueryFunctions`), so the list the model sees cannot drift from what a query may call.
- Small models echo the ids and Markdown bold despite the prompt; worg's `answerRuns` takes the ids out and draws the bold. They also prefer find_headings to suggest_query unless told plainly that only suggest_query changes the view.

`chatgpt` and `gemini` go through `openAIChat` too (Gemini by `https://generativelanguage.googleapis.com/v1beta/openai`, Bearer key, `tools` on unless `tools: false`). **Gemini refuses a turn whose earlier tool calls lack their thought signature**: a tool call keeps every field the server put on it (`openAIToolCall.Extra`, carried back by `MarshalJSON`; `TestOpenAIToolCallKeepsExtraFields`). Its errors come as a JSON *list*, which `sseLines` reads. Under load its OpenAI endpoint answers a plain request with a 503 but can hold one with tools open for minutes.

`POST /voice/dictate` (voice.go) is writing mode's dictation: one short clip (multipart `audio`, `prompt`, `language`, `model`) into a temp file, `whisperTranscribe`, temp removed - nothing is kept, unlike `/voice/recording` + `/voice/transcribe`. `prompt` is the text before the cursor (cut to its last 600 bytes). Note that `whisperTranscribe` streams the upload **chunked** (an `io.Pipe`), so a fake whisper for tests must read chunked bodies.

`harper.go` runs Harper for worg's checker on the server (`server.harper`): `harper-ls --stdio` started on the first `POST /harper/lint` (PATH, `path`, else the pinned release downloaded to the user cache), spoken to over LSP in Go. Not a port: Harper is Rust, and running its own server keeps browser and server answers identical. Its wasm build was rejected as the Go route: wasm-bindgen glue with 51 hash-named JS imports per release. No signal handling needed - it exits when its stdin closes. Offsets are UTF-16 (`utf16LineStarts`), as in the browser.
