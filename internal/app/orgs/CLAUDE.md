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

The line in force is the first of: the request's, the file's `#+COLUMNS:`, `columns.default` in yaml, a built-in; `From` says which, so the view can explain a missing column. Built-in: org's `%25ITEM %TODO %3PRIORITY %TAGS` plus `%EFFORT{:} %CLOCKSUM`, because summing effort is the view's purpose and few files declare a columns line.

1. **A cell carries `Value` (shown) and `Own` (written).** They differ on rolled-up parents (`8:45` summed, `2h` own). The editor gets `Own`, or the first return writes the total onto the parent.
2. **Rollup = children's total *plus* the heading's own value**, so no typed number is silently discarded.
3. **Properties are read off the file's lines** (see **Traps: column-zero drawers**), or EFFORT looks unset for those headings.
4. **`%CLOCKSUM` always rolls up** (org's meaning); `%EFFORT` only with an operator, because inventing one would disagree with org about the file.
5. **Sort within each parent**, never flat (it is an outline; flat sorting separates children from parents). Empty cells sort last either direction (absence, not a small value).

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

1. **Variables are prepended in the language's syntax**: `:var scale=2` an assignment, `:var data=monthly` a list of lists with numeric cells bare so `sum(r[1] for r in data)` works. Go and emacs-lisp run without variables (a prepended assignment survives in neither), and say so.
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
2. **Writes are line edits of the card's own lines**: properties via `ensurePropertyDrawer`/`setPropIn` at the drawer's own indent (Emacs writes drawers in column zero), SCHEDULED via `setScheduled` on the planning line, `leech` via `addHeadlineTag`. Never `ChangeDate` (it calls `WriteOutOrgFile`).
3. **Floats are written as Emacs prints them** (`lispFloat`: `4.0`, not `4`), so a file drilled here diffs cleanly with one drilled in Emacs.
4. **The SM5 matrix is keyed by ease rounded to 3 places** (deliberately not org-drill, whose float keys never match what is read back) and saved per user in extensions after every rating.
5. **Quirks kept on purpose**: the EF floor is applied before the change; Simple8 does not count a failure in the total; the stored interval is the hypothetical one the button showed (org-drill's smart reschedule).
6. Test the review endpoint with `Content-Type: application/json`: without it the form-parsing middleware eats the body and the hash comes through empty ("no heading with that hash").
