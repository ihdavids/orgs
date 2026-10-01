# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project overview

`orgs` is a Go-based Org Mode server + CLI. A single binary (`cmd/orgs`) acts as both:

- the HTTP/HTTPS server that watches org files, parses them, serves a REST API, and runs background plugins; and
- the command-line client — the first positional argument selects a subcommand from the registry (`serve`, `agenda`, `refile`, `grep`, `capture`, `export`, …) which then talks to a running server over REST.

A second binary, `cmd/docex`, scrapes `SDOC:` / `EDOC` marker comments out of the Go sources to generate documentation — it is not needed for normal builds.

Go module: `github.com/ihdavids/orgs` (Go 1.23+, toolchain 1.24.2). There is no test suite to speak of - the only `_test.go` files cover the D&D appearance step and the CLI list chooser.

## Build and run

The worg frontend is embedded into the binary from `worg/` (see `worg/embed.go`) and served by `StartServer`. `tools/buildworg.sh` builds worg from `../worg` and pulls the result in — it clears the old files first rather than writing over them, because the build's names carry content hashes and a plain copy leaves every previous build inside the binary. The built app asks the page's own origin for the api, so it works whatever port the server was started on.

Flags for a subcommand go *after* it (`./orgs serve -port 8010`), not before it - a flag ahead of the subcommand is consumed by the global parse and the command list is printed instead. And note that the HTTP listener runs in a goroutine while the HTTPS one blocks: with `allowHttps: false` the process falls straight through both and exits, so a throwaway server on another port still needs `allowHttps: true` and a cert to stay up.

```sh
# Build everything (binaries land in the current directory or GOBIN):
go build ./...

# Build just the main binary explicitly:
go build -o orgs ./cmd/orgs

# Start the server (reads ./orgs.yaml by default, falls back to ./orgc.yaml):
./orgs serve                       # uses config
./orgs -config /path/to.yaml serve # override config file
./orgs serve -port 8010 -tlsport 443

# Run a client command against a running server:
./orgs -url http://localhost:8010 <command> [flags...]

# Build the doc extractor:
go build -o docex ./cmd/docex
```

`orgs export` writes **here** by default and `-local` makes the server write it at that path on its own disk. It used to be the other way round, which is right for a server and a client on the same machine and surprising everywhere else: `orgs -url https://box:8010 export -out ./notes.html` wrote notes.html on box. `-f pdf` is not an exporter call — the pdf exporter writes a file and returns nothing to a string, so `/pdf` is its own endpoint answering with bytes, cached against what the org file was when it was built. `-list` and `-list-themes` ask the server what it has; `/exporters` was added for the first of those, because an exporter compiled in but not named in the yaml is not there as far as a request is concerned and the refusal looks the same as the one for an exporter that does not exist.

There is no Makefile and no lint config. `go vet ./...` and `go build ./...` are the practical health checks. `go test ./internal/common/dnd/ ./cmd/oc/commands/dnd/` runs the handful of tests that do exist - a plain `go test ./...` fails on the vet check `go test` runs by default, over pre-existing `fmt.Printf` calls in several plugins.

## Configuration

`Config.ParseConfig` in `internal/app/orgs/settings.go` is the authoritative loader. Key behavior to know before changing config plumbing:

- `flag.Parse()` is called **twice** — once before the YAML load (so `-config` can point at a file) and once after (so command-line flags override YAML). Anything that registers flags must do so in `SetupCommandLine` / `AddCommands` before the first parse.
- `Conf()` is a lazy singleton; most of the codebase reaches the config via `orgs.Conf()` rather than passing it around.
- The same `Config` struct holds both server settings (under `Server *common.ServerSettings`) and CLI-only options (`Url`, `Token`, `EditorTemplate`, `Aliases`). The CLI reads `orgs.Conf().Url` to know which server to hit. The `Token` field stores the JWT from the last `login` command and is automatically applied as a Bearer header on startup.
- Aliases (`aliases:` in YAML) are expanded in `cmd/orgs/main.go` *before* command dispatch, so `args[0]` may be rewritten into a multi-word command + flags.

## Architecture

### Top-level layout

- `cmd/orgs/main.go` — entry point for both server and CLI. Builds a `commands.Core`, then either routes to a subcommand from `commands.CmdRegistry` or (via the `serve` command) calls `orgs.StartServer`.
- `cmd/oc/commands/` — every CLI subcommand lives in its own subpackage here. `commands.go` defines the `Cmd` interface, `Core` (which wraps a `common.Rest` client), the `CmdRegistry`, and the generic `SendReceiveGet` / `SendReceivePost` helpers used by commands.
- `cmd/oc/commands/all/all.go` — blank-imports every command package so their `init()` functions register with `CmdRegistry`. **New commands must be added here or they will not be reachable at runtime.**
- `internal/app/orgs/` — the server: REST handlers (`rest.go`), auth (`auth.go`, `jwt.go`, `user.go`), in-memory org DB (`orgdb.go`), capture/refile/archive/clock logic, settings, and the server-side plugin host.
- `internal/app/orgs/plugs/` — server-side plugins (exporters, pollers, updaters) like `html`, `revealjs`, `latex`, `jira`, `todoist`, `googlecal`, `notify`. Each plugin self-registers via `init()` in its own package.
- `internal/app/orgs/plugs/all/all.go` — blank-imports every server plugin for the same reason as `cmd/oc/commands/all/all.go`.
- `internal/common/` — code shared between client and server: `restclient.go` (the generic REST client with `RestGet[T]` / `RestPost[T]`), `serversettings.go`, `plugs.go` (defines `Exporter` / `Poller` / `Updater` interfaces and `PluginManager`), plus data types used on the wire.
- `internal/templates/` — pongo2-based template manager used by exporters and capture templates.
- `templates/`, `web/`, `webfonts/` — static assets served by the HTTP file server alongside the REST API.

### Server request flow

1. `orgs serve` calls `orgs.StartServer(sets *common.ServerSettings)` in `internal/app/orgs/serve.go`.
2. It creates a `mux.Router`, calls `RestApi(router)` in `rest.go`, mounts static directories (`/images`, `/orgimages`, `/orgfonts`, `/`), starts the background plugins (`startPlugins`), and listens on HTTP and/or HTTPS depending on config.
3. `RestApi` registers `POST /login` as the only **public** route, then mounts every other endpoint under a subrouter that uses the `authenticate` middleware from `auth.go`.
4. `authenticate` accepts either an `Authorization: Bearer <token>` header **or** an `orgstoken` cookie. Tokens are JWE-encrypted JWS (`jwt.go`), signed with HS256 using `OrgJWS` and encrypted with PBES2 using `OrgJWE` from server settings, and expire after 5 minutes.
5. Handlers in `rest.go` typically unmarshal JSON into a `common.*` type, delegate to a function in `orgdb.go` / `capture.go` / `refile.go` / etc., and JSON-encode the result.

### CLI subcommand pattern

Every CLI subcommand is a small package implementing:

```go
type Cmd interface {
    StartPlugin(manager *common.PluginManager)
    Unmarshal(unmarshal func(interface{}) error) error
    Exec(core *Core)
    SetupParameters(*flag.FlagSet)
}
```

The package's `init()` calls `commands.AddCmd("name", "usage", factory)`. `main.go` then:

1. Parses global flags, loads config, builds `commands.Core` with `core.Rest.Url = Conf().Url`.
2. Expands aliases from YAML.
3. Finds the matching entry in `CmdRegistry`, lets that entry's `flag.FlagSet` parse the remaining args, and calls `Exec(core)`.

Inside `Exec`, commands talk to the server using the helpers in `cmd/oc/commands/commands.go`:

- `SendReceiveGet[RESP](core, "path", params, &resp)` — wraps `common.RestGet`.
- `SendReceivePost[REQ, RESP](core, "path", &req, &resp)` — wraps `common.RestPost`.
- `core.Rest.Header` carries the `Authorization: Bearer` header. The `login` command POSTs credentials to `/login`, saves the returned token into the YAML config file (`token:` field), and prints it to stdout. On subsequent runs, `main.go` auto-loads the stored token onto `core.Rest.Header`, so all commands are automatically authenticated.
- `core.ConfigFile` holds the path to the active config file, so commands can read/update it without importing `internal/app/orgs`.
- `core.LaunchEditor(filename, line)` opens files in the editor configured by `editorTemplate`.

**Import cycle constraint**: CLI command packages under `cmd/oc/commands/` **cannot** import `internal/app/orgs` — that package already imports `cmd/oc/commands/all` (which blank-imports every command), creating a cycle. Commands must only use `cmd/oc/commands` and `internal/common`. If you need config values, pass them through `Core` fields set in `main.go`.

When adding a new CLI command: create `cmd/oc/commands/<name>/<name>.go`, implement the interface, register in `init()`, **and add the blank import to `cmd/oc/commands/all/all.go`**. Running `go build ./...` from the repo root is sufficient to verify it links.

### Every command's four flags, and where they come from

`-json`, `-format`, `-dry-run` and `-no-color` are not registered by any command. `Config.AddCommands` in `settings.go` calls `commands.AddGlobalFlags` on each command's flag set **after** `SetupParameters`, and `AddGlobalFlags` only defines a name that is still free — so the dnd client keeps its own `-json` (a saved D&D Beyond payload) and its own `-format` (html/latex/pdf), and every other command grows both. The command's own meaning always wins, which is the right way round.

`cmd/oc/commands/output.go` is the whole of it:

- **`Render(rows, plain)` / `RenderOne(row, plain)`** are what a command calls instead of printing its own listing. `-json` marshals; `-format` runs a Go `text/template` once per row (with `base`, `dir`, `upper`, `lower`, `trim`, `join`, `cut`, `pad` and `prop` — `prop` reaching into the `Props` map every heading carries); neither runs `plain()`. A command writes its listing once and is scriptable without knowing it.
- **`Machine()`** is "something is reading this rather than somebody", which is the question to ask before opening an fzf chooser or a survey prompt. A command that would block on a question nobody can answer refuses instead.
- **`Colour()`** is a terminal, not `-no-color`, not `NO_COLOR`, and not `Machine()`. `C(code)` returns the escape or `""`, so callers concatenate without branching.
- **`Fail`** says what went wrong the way the caller asked to be told: as a `{Ok, Msg}` object when `-json` is on, as a line on stderr otherwise.

`-dry-run` is answered in **`SendReceivePost`** rather than in each command, so a command written tomorrow gets it without asking and one that forgets cannot write anyway. It prints the method, the path and the request body to stderr and returns. `SendReceivePostErr` does the same and answers with `ErrDryRun`, which a caller must treat as "nothing happened" rather than as a failure. The handful of writes that are not a POST — an exporter asked to put its answer on the server's disk, a local file write, running a source block — call `Wrote(what, detail)` explicitly.

Three things this depended on, all of which were bugs on their own:

1. **The log goes to stderr.** `logToFile` in `cmd/orgs/main.go` used to `io.MultiWriter(os.Stdout, f)`, so every run put `Loading: orgs.yaml` into its own stdout. Invisible on a terminal, fatal for `orgs search -json | jq`, and fatal on the first line for `orgs mcp`.
2. **`RestGet`'s unmarshal complaint goes to stderr** for the same reason, and so does the missing-config-file message in `settings.go`.
3. **`FreeArgs`/`FreeText`** re-run the parse after taking each word off, because Go's `flag` stops at the first argument that is not a flag — so `orgs search 'IsTask()' -json` parsed no flags at all and `-json` became a second word of the query. `search`, `find`, `links`, `export`, `tui`, `code` and `rec` all use it, and the rule is that flags and words may be written in any order.

### The CLI commands that read the server's newer endpoints

The server grew records, links, code, file search and babel; these are the terminal halves of them. All are thin — each is one request to an endpoint that already exists, because a command that needed logic of its own would be a second answer to a question the server already answers.

- **`orgs search '<expr>'`** — `/search`, the query language. `-sort`, `-limit`, `-count`, `-group`, `-open`.
- **`orgs find '<re>'`** — `/files/search`, a regular expression over the raw text. Not `orgs grep`, which answers with `"file|line|text"` strings that cannot be taken apart again when the line holds a colon; here a match carries its offsets, so the match is coloured without running the pattern a second time and the two regular expression engines never have to agree.
- **`orgs links`** — `/links/all`, `/links`, `/links/stats`. Bare, it is a picker with the far end drawn beside it; `ls` is the listing. See **The pickers** below.
- **`orgs tables`** — `/tables`, `/table`, `/table/eval`. The Tables tab as a picker: the grid with its `@n`/`$n` rulers and its formulas. See **The pickers** below.
- **`orgs code`** — an fzf picker with the block drawn beside it, the way `orgs grep` is an fzf picker with the file drawn beside it. `ls`, `show` and `run` are still there for naming one outright. See **The code picker** below.
- **`orgs rec`** — the record endpoints, over *every* collection rather than one. `orgs contact` is the same engine with the address book's manners on and is unchanged; `record` used to be a second name for `contact` defaulting to the contact collection, which is exactly the wrong default for a command named after the general thing, and is now an alias for `rec`.
- **`orgs tui`** — see below.
- **`orgs mcp`** — see below.

One edge they all share: **a heading's hash in a url path is base64 encoded a second time.** The hash arrives on a `Todo` as base64 already, and `GetHash` in `rest.go` base64-URL-*decodes* the path segment before looking it up. Written into a url as it stands, the `+` becomes a space and the `/` a path separator; the handler answers "no heading with that hash", which reads like a stale hash rather than a mangled one. `commands.HashPath` is the one place that does it.

### The write verbs, and how a command names the heading it is about

`orgs todo`, `sched`, `deadline`, `tag`, `prop`, `rename`, `note`, `check`,
`archive` and `rm` are `cmd/oc/commands/edit/`. Every endpoint behind them
already existed and every one was already reachable - from `orgs mcp`, and from
inside the `tui` and `agenda` screens - but none of them was reachable from a
prompt, so an agent had better write access to somebody's org files than they
did.

They are one package because what makes them worth having is the part they
share: **`commands.Resolve` in `target.go`** decides which heading, identically
for all of them. Four ways in, and the last is the one that makes the read half
of the tool worth more than the sum of it:

```sh
orgs todo DONE                        # a picker
orgs todo DONE 'IsStatus("NEXT")'     # a query
orgs todo DONE hOpOB7vIg6oiYz5sMVS=   # a hash
orgs search '…' -json | jq -r .Hash | orgs tag +stale -   # hashes on stdin
```

Rules that are load-bearing:

1. **More than one match is a question, not an assumption.** With a person
   there the matches go to a picker; with only a pipe it refuses and says how
   many it found. `-all` is how you say you meant all of them, `-at N` for one.
2. **A hash is recognised by its shape** - 28 characters of base64 ending in
   `=` - so the thing the tool itself printed can be pasted back without a flag.
3. **`orgs tag` reads the heading's current tags first.** `POST /tags` *toggles*,
   so `+work` on a heading already tagged work would take the tag off; doing
   what was asked means knowing what is there, which is what makes the verb safe
   to run twice and safe in a pipeline nobody is watching.
4. **`orgs note` strips the property drawer and the planning line** out of what
   `/body/{hash}` hands back before writing it through `/body/change`
   (`proseOnly`). The body is returned *as written*, drawer included, and the
   drawer is regenerated from the headline - so handing the whole thing back
   wrote it twice.
5. **A dash-word is a value, not a flag.** `orgs tag -someday` is a tag being
   removed, and Go's flag package calls it an undefined flag and exits. A
   command implementing `commands.DashWords` has those words taken off the line
   in `main.go` *before* the parse - and only words it has not defined as a
   flag, so `-json` stays a flag everywhere with no list to maintain.

### Running without a server: `-local`

`orgs -local <command>`, or `orgs -orgdir ./notes <command>` which implies it,
runs the server inside the process for the length of one command. That is what
makes `orgs agenda` work in a git hook, `orgs fmt -check` in CI and `orgs
search` over ssh on a box where no daemon was ever started. The port is asked of
the kernel (port 0), authentication is off, and readiness is polled rather than
assumed.

Two things had to be fixed for it to work at all, and both were bugs on their
own:

1. **`orgs serve` exited immediately with `allowHttps: false`.** The https
   listener was what held `StartServer` open; without it the process fell
   through both listeners and ended. It now waits on a signal instead, which is
   also the only path on which `StopWhisper` and `stopPlugins` ever run.
2. **The server printed to stdout.** Three hundred-odd `fmt.Printf` calls across
   the server and the plugins are diagnostics, and in `-local` mode the server
   shares stdout with the command - so `orgs search -json | jq` got `WATCHING:`
   and `PLUGIN START:` spliced into its json. They all go to stderr now. Same
   rule as the log and `orgs mcp`: **stdout is the answer.**

### `GET /events`, and the things that are live

`internal/app/orgs/events.go` is server-sent events on one long GET. The server
has watched the org files since the beginning and had no way to say so; the
websocket API that would have carried it was commented out years ago.

- `orgs watch` prints each change, `-exec 'make notes'` runs something about it
  (`{}` becomes the file), `-file` narrows it, `-once` waits for one.
- `orgs clocks -w -short` is a status line: one line, redrawn when the clock
  changes *and* once a minute, because the elapsed time moves with nothing
  happening.
- The agenda redraws itself when a file changes under it (`-no-live` to stop it),
  through `app.QueueUpdateDraw` - tview owns the screen and a goroutine drawing
  to it directly is a race.

Four things about the stream: a subscriber that is not reading is **dropped, not
waited for** (a sleeping laptop must not stall the file watcher); an event says
**what happened, never what the thing now is**; there is a **heartbeat every
twenty seconds** because a proxy closes an idle connection and the reader cannot
tell that from a quiet database; and **reloads are coalesced** over 120ms,
because one save is often three filesystem events and three redraws read as a
flicker. The client half is `internal/common/events.go`, which **reconnects with
a backoff** - a watch is meant to be left running for a day, and in that day the
server will be restarted.

### Tab completion

`orgs completion zsh|bash|fish` prints a script; all three do the same thing and
shell out to **`orgs __complete <words…>`**, so the interesting part is written
once in Go rather than three times in shell, and a command added tomorrow
completes without anybody editing a script. What it completes is server-knowable:
files, tags, *this heading's file's* keywords, saved queries, filters, exporters,
themes, collections.

Two rules: it has a **900ms timeout** (`Rest.Timeout`) and degrades to no
suggestions, because a prompt that has stopped responding is worse than one with
nothing in it; and it **does not complete the target of a destructive verb** -
completing a query is a convenience, completing the heading `orgs rm` will delete
is a way to delete the wrong thing quickly.

### `orgs review`, `orgs diff`, `orgs blame`, `orgs doctor`

- **`review`** is the weekly review as ten named checks, each one a query and a
  sentence and nothing else - a check that needed code of its own would be a
  second implementation of the query language. A check that finds nothing prints
  a green line rather than staying silent; that line is the most valuable output
  it has. `-fix` walks the findings into the editor, deliberately not into a menu
  of automatic repairs: "this project has no next action" is answered by deciding
  something.
- **`diff` and `blame`** (`gitorg/`) read git as *org*: which headings changed
  and how, rather than which lines moved. They parse both revisions **here**
  rather than asking the server, which is what makes them work in a pre-commit
  hook and on somebody else's checkout. Two traps: a nested heading is a
  `*org.Headline` (the pointer trap again), and go-org's default keyword list is
  `TODO | DONE` alone - so in a file with no `#+TODO` line every other keyword
  was read as part of the headline text and every NEXT→DONE looked like a rename.
- **`doctor`** asks every question at once, because "the server is not running",
  "the token expired" and "that exporter was never put in the yaml" all answer
  "could not" and none of them says which. It is marked `NeedsNoServer` so the
  checks on the way in cannot stop the command that exists to diagnose them.

### `orgs help`, and the commands that need no server

`commands.Offline` (`NeedsNoServer()`) marks a command that has nothing to
authenticate to. The token check in `main.go` used to run for every command, so
`orgs completion zsh` - a script printed from a table in this binary - refused to
run because of a token it was never going to send. It also exited on an expired
token **without trying the refresh**, so a session that could have been renewed
told people to log in again.

`commands.Grouped` (`HelpGroup()`) is how a command registered at runtime says
where it belongs in the listing - every filter in the yaml is a command, and the
group table in `help.go` can never name them.

While in there: **the dispatch loop in `main.go` did not stop at the command it
found.** It walked `CmdRegistry` in map order mutating `args` as it went, so a
command whose *argument* was another command's name ran both, in whichever order
the map felt like that run - `orgs __complete tag ''` ran the completion and then
`orgs tag`.

### The commands that were endpoints with nobody to call them

`cmd/oc/commands/orgtools/` is `tangle`, `fmt`, `tags`, `outline` and `log`;
`savedq/` is `orgs q`; `voice/` is `orgs voice`. Each is one request to an
endpoint that already existed.

- **`orgs tangle`** says what the blocks would write by default, `-w` writes on
  the server's disk and `-o DIR` writes here - the distinction `orgs export` had
  to learn. Under `-o` the paths are taken **relative to the org file**, because
  the server answers with its own absolute ones.
- **`orgs fmt -check`** writes nothing and exits non-zero, which is the whole
  contract a pre-commit hook needs. `POST /reformat` always writes, so the check
  goes to a new **`GET /reformat`** which answers with what the writer *would*
  produce and whether the file already says it. The comparison is the server's:
  the file may not be on this machine.
- **`orgs outline -match`** is a sparse tree - the headings a query found plus
  enough ancestors to say where they are, which is org's most useful way of
  reading a big file and the one thing `orgs search` cannot be.
- **`orgs q`** is the same stored queries worg's search tab keeps. Running one is
  `/search` with the text looked up first, so everything `search` grew works on a
  saved query without this command knowing any of it.
- **`orgs voice`** records with `sox`/`ffmpeg`/`arecord` - not linked in, for the
  same reason whisperd.go supervises go-whisper rather than linking it - then
  follows the voice notes' order exactly: **the audio is uploaded before anything
  is attempted on it, and the transcript is shown before anything is written.** A
  recording that cannot be uploaded is written to a temp file and the path is
  printed; losing the only copy of something somebody said is the one outcome it
  must never have. `Rest.PostFile` is the multipart upload it needed.

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

### The query functions the review needed

`HasScheduled()`, `HasDeadline()`, `HasTimestamp()`, `HasAnyDate()`,
`DeadlinePast()`, `ScheduledPast()`, `HasClock()`, `DaysOld()` and
`OlderThan(30)` are in `todo.go` beside the rest. The language could ask what a
heading *was* and whether a date fell on a given day, and could not ask the two
questions a review is made of: does this have a date at all, and has it gone
past. `HasChecklist()`, `ChecklistDone()`, `ChecklistCount()` and
`ChecklistLeft()` are in `checklist.go` rather than `todo.go`, because they have
to count boxes exactly the way the *write* counts them - `orgs check 3` and
`HasChecklist()` disagreeing would put a tick on the wrong line.

Two things are deliberate: **past means before today**, counted in days rather
than instants, so a deadline of today is due rather than overdue; and a heading
with no date answers `DaysOld() == -1`, so `OlderThan` is never true of a heading
that has no date to be old.

### The pickers

Five commands are an fzf list with a pane beside it: **`orgs code`**, **`orgs links`**, **`orgs tables`**, **`orgs rec`** and **`orgs contact`**, each bare (no subcommand). `orgs grep` was the first of these and hands its pane to `bat`, which knows how to colour a file and nothing about org. None of them is a file — a source block is a header line and variables and code, a link is a target and the paragraph it was written in, a table is a grid and its formulas, a record is a drawer of fields whose kinds were worked out for it — so the pane is drawn by this binary.

Which means the preview is **a second run of the same binary**: fzf's `--preview` shells out to `orgs <thing> preview …`. The machinery is `cmd/oc/commands/picker.go` — `Pick`, `SelfCommand`, `Shq`, `PaneWidth`, `OpenBox`/`BoxLine`/`CloseBox` — shared so they behave identically and a fix lands once. Four rules, each a bug before it was a rule:

1. **The child has to reach the same server.** It is a fresh process with a fresh config load, so `-config` and `-url` are passed through explicitly rather than left to defaults that resolve against a working directory fzf's child does not necessarily have.
2. **The command is a shell string**, so every path in it is single quoted (`Shq`). fzf quotes the `{1}`/`{2}` it substitutes, so those are left alone.
3. **The list is tab separated**, address fields first and hidden (`--with-nth`). `orgs grep` splits on `|` and a path with a pipe in it takes a line apart in the wrong place. What is *shown* is also what is *searched*, which is why the full path is in the display as well as in the address.
4. **The pane draws to whatever width it is given** (`FZF_PREVIEW_COLUMNS`, then `COLUMNS`, then 80). Its boxes are **open on the right** on purpose: a box that closes has to know the display width of every line in it, and one line with a tab or a wide character makes that a guess. A box with one ragged edge reads as a box; one with the wrong edge reads as a bug.
5. **The pane has to be told it may use colour** (`commands.PickerOutput`). fzf runs the child with a *pipe* for stdout, so `term.IsTerminal` is false and `Colour()` answered no - every one of these panes was quietly drawing in black and white from the day it was written. This is the same thing `bat --color=always` is for. It was invisible for as long as the panes were the only thing affected; it became visible the moment `orgs habits` grew a key that **reloads the list**, because then a coloured list drawn by the parent was replaced by an uncoloured one drawn by a child, in front of somebody watching. `-no-color` and `NO_COLOR` still win, and a command that passes flags to its children has to pass `-no-color` among them or the pane and the listing disagree.

And one that is not about fzf at all: **`commands.FreeArgs` consumes the arguments as it parses them**, so a command wanting both flags-anywhere *and* a subcommand has to take the words once and read the subcommand off that slice. Asking the flag set afterwards gets nothing, which silently turned `orgs links preview` into a search for the word "preview".

#### The code pane

The pane is the inputs, the code, and the outputs: a box of switches, header arguments and `:var` bindings — each resolved, so `:var data=monthly` shows `→ table 6×7 in notes.org` and a name that resolves to nothing is said in danger colours because that is a block that cannot run — then the code (through `bat` when it is on PATH, line-numbered plainly when it is not), then a `results` box when the block has a `#+RESULTS:`.

A block is very often looked for by **the table it reads** rather than by its own name, and an unnamed block has nothing else to be found by, so the list line carries `← monthly` as well as the name.

Two ways to run from the picker: `ctrl-r` runs the highlighted block without leaving the list (the point of a preview pane is deciding whether to run the thing), and picking one prints it and then **asks**. Running somebody's program because they pressed enter on a list is not what enter on a list means; `-run` skips the question and `-no-run` never asks.

`commands.Interactive()` gates every picker. A pipe, a `-json` run or a cron job gets the listing instead — the same question answered in a form that caller can use, rather than a full-screen application it can neither see nor answer.

#### The links pane

The list answers "where do my links go". The pane answers the question you opened the list to ask — "is this the one I meant" — and for a link that is mostly about **where it was written**: a ticket pasted into a heading eighteen months ago is identified by the paragraph around it, not by its url. So the pane is what the link says, what it is, and the lines of the org file around it with its own line marked; then the far end — the target heading's body for an org link, the file for a `file:` one, a plain refusal for a broken one.

Three things about it:

* **The address is the row's position in the list**, not anything about the link. A link has no id of its own — it is named by the file, the line and what it said — and threading three fields through a shell command is three chances to quote something wrong. The pane re-runs the same query and indexes into the same answer, which is why `previewArgs` has to pass on **every** flag that changes what is in that answer. Miss one and `-at 4` means a different link in the child than in the parent.
* **The context lines are read from the file here**, not asked for. A whole endpoint for "six lines of a file" is a lot of server for a preview pane, and when the file is not on this machine the pane says so and the rest of it still reads.
* **Enter opens where the link was written**, not where it points. Following it is the browser's job; getting back to the heading you wrote it in is what the terminal can do that the browser cannot. `ctrl-o` follows it, through the desktop's own opener — there is no knowing here what a `doi:` or a `mailto:` should do.

While building it: **go-org gives every inline node the position of the node it was parsed in**, so two links in a two-line paragraph both claimed the first line. `fixLinkLines` in `links.go` now finds each link's real line in the file — in document order, with a cursor that never goes backwards (so the same url twice resolves to the first occurrence and then the second), starting from the parser's row, which is right or early but never late. A link it cannot place keeps the row it had. This was wrong everywhere the line was used: the links tab, `orgs links`, the editor jump. The pane drawing the file around the link is only where it became visible.

#### The records pane

`orgs rec` and `orgs contact` are both pickers over the same records, and the whole of what they draw - the list line, the card, the bindings - is **`cmd/oc/commands/recview`**. That package is its own thing rather than living in either command because neither owns the other: `rec` is the general one over every collection, `contact` is the same engine with the address book's manners on. A copy in each would say different things about the same record within a month, which is exactly what the records engine avoids by working a field's kind out from its name rather than having a view per collection.

Nothing in `recview` knows what a contact is, or a guitar pedal. A record is a name, some fields whose kinds the server decided, some notes and a history, and that is all it draws. Three things follow:

* **Fields are ordered by kind, and an unknown kind sorts into the middle rather than last.** The odds and ends a collection of guitar pedals grows are the whole point of that record and should not be hidden under its urls.
* **The two commands differ in exactly one drawing decision**, and it is a `PickOptions` field: the general one labels each row with its collection, the address book does not, because every row there is in the same one and a column of the same word is not a column.
* **A record is addressed by its own hash**, not by its position in the list the way a link is — a record has an identity and a link does not. So the pane still finds it after the list has been filtered under it. The hash goes into the url encoded a second time (`commands.HashPath`): it is base64 already, and written in as it stands its `+` becomes a space and its `/` a path separator.

Occasion arithmetic — "turns 36 in eight days" — is deliberately **not** in the pane. The rule for which date fields are celebrations is already written twice (`birthdayFields` in Go, `CELEBRATIONS` in TypeScript); a third copy in the CLI is the drift the codebase keeps warning about. `orgs rec birthdays` asks the server, which is the one place that knows.

Both commands kept their previous narrow-to-one chooser under the name `narrow`: `show`, `edit` and `set` still resolve a name to one record with a survey prompt, which is right for a command given a name rather than opened to browse.

#### The tables pane

`orgs tables` is worg's Tables tab as a picker, and it draws the three things that view draws:

1. **The rulers** — `@1` down the side, `$1` across the top — because a formula is written in those coordinates, and a table without them is one you have to count along with a finger to read `$3=$1*$2`.
2. **The rules.** A `|---+---|` line is a rule and does **not** take a row number: org does not count it, so numbering it would put every formula below one off by however many there are.
3. **The formulas against the cells they fill.** `CellFormulas` is keyed `"row,col"` and a computed cell is drawn in green. This is the part nothing else will tell you — a `#+TBLFM:` line at the foot of a table says `$4=$2*$3` and says nothing about where that lands.

Columns are measured from their widest cell and then squeezed, a character at a time off whatever is currently widest, until the grid fits the pane. A table too wide for the pane is the common case rather than the exception.

`orgs tables eval` runs the formulas and writes them back through **`POST /table/eval`** — the file and the table's ordinal, the same address everything else here uses. **Not `/exectable`**, which looks like the right one and is not: its SDOC says "the file is re-saved to disk after the update" and `ExecTable` does no such thing — it runs the formulas into the *in-memory* table and returns the rendered org text. Calling it appears to work, because the read afterwards comes from the same memory and shows the computed values, and it leaves the file on disk untouched so they vanish on the next reload. That doc comment is still wrong and is worth fixing.

### `orgs tui`

One screen: a table of whatever a query found, `/` to filter, `:` to re-query, `t` to change a keyword, `v` for the heading's body, Enter to open it.

The two boxes are two on purpose. The **query** costs a request and can say `IsStatus("NEXT")`; the **filter** costs nothing and can say "the one about the invoice". Keeping them apart is what lets the filter run on every keystroke, which is the whole reason it exists. The filter is `internal/common/dnd/fuzzy.go` — the same matcher the dnd chooser and worg's palette use — so the letters that find a heading in one of them find it here.

Three things in it are load-bearing:

1. **The filter's haystack is the file's base name, never its path.** A fuzzy match walks for its letters in order and does not care how far apart they are, so an absolute path — long, identical on every row, and full of letters nobody typed — matches very nearly anything: with files under `/Users/someone/dev/notes`, `mig` found every heading in the database. It is also what a person means when they type a file at a filter.
2. **The keyword menu asks `/status/{hash}`** when it opens, rather than offering the keywords the last query happened to turn up. A heading's own file may declare its own `#+TODO:`, and offering a keyword that file does not have is offering to write something org will not read back. Same rule worg's kanban follows.
3. **Opening an editor suspends the application** (`app.Suspend`). A terminal editor and a tview application both want the terminal, and whichever loses draws over the other.

### `orgs mcp`

The org database as tools an agent can call, over MCP on stdin and stdout. It is a **client**, not a second server: every tool is one request to a running orgs server, which is what makes it worth having rather than a second implementation to keep in step with the first. `cmd/oc/commands/mcp/mcp.go` is the transport, `tools.go` the table.

```json
{"mcpServers": {"orgs": {"command": "orgs", "args": ["mcp"]}}}
```

- **Stdout is the protocol** — one JSON object per line and nothing else ever. Anything added that prints must print to stderr. (This is what the log change above was for.)
- **A notification has no id and gets no answer.** `notifications/initialized` arrives right after the handshake and replying to it is a protocol error, not a harmless extra.
- **A tool that fails answers with `isError`, not a JSON-RPC error.** The model is meant to see what went wrong and try something else; a transport-level error is for the client library and never reaches it. Only an unknown method or unreadable json is a real error.
- **An empty answer is said out loud** ("(nothing matched)"). A blank string and a failure look identical to a model, and it will assume the latter and try again.
- **`-read-only` drops every tool that writes**, which is a smaller surface than trusting an agent not to. `-dry-run` works too and the refusal comes back as something the model can read rather than as an error it would retry.
- Every list tool takes a **`limit`** and says when it used one. Trimming before the answer reaches a model is not tidiness, it is the difference between an answer and a context window.
- Two tools look alike and the descriptions say which to reach for: `org_search` understands headings, keywords, tags and dates; `org_find` understands nothing and is the one that finds a phrase in a drawer, a table or a source block.

`orgs mcp -list` prints the table with the writing tools marked. `-json` on it hands over the name, description, `Writes` and schema rather than the `Tool` struct, which holds the function that runs it and will not marshal.

### Server-side plugins

Server plugins implement one of the interfaces in `internal/common/plugs.go` (`Exporter`, `Poller`, `Updater`) and register themselves in `init()`. They are instantiated from the YAML config under `server.exporters`, `server.plugins`, and `server.updaters`, then started by `ParseConfig` via `pd.Plugin.Startup(...)`. As with CLI commands, a new plugin package must be blank-imported in `internal/app/orgs/plugs/all/all.go` to be reachable from YAML.

The `PluginManager` passed to plugins carries the shared templates, filter map, tag groups, org directories, and a cached password helper that can read from the OS keyring.

### Voice notes and go-whisper

`internal/app/orgs/voice.go` records nothing and transcribes nothing. A client records, the server keeps the audio, and transcription is a request to a [go-whisper](https://github.com/mutablelogic/go-whisper) server over its own http api (`/api/whisper/model`, `/api/whisper/transcribe`).

`internal/app/orgs/whisperd.go` **runs that server**. Two settings are the whole configuration:

```yaml
voice:
  models: "/path/to/whisper/models"
  port: 8081
```

With those, `StartWhisper` (called from `StartServer`) spawns `gowhisper run --http.addr localhost:<port> --models <dir> --whisper.gpu` and looks after it; everything else under `voice:` has a default (`bin`, `gpu`, `args`, `url`, `model`, `language`, `dir`, `timeout`, `maxMb`, `tags`, `target`). `voice.dir` is relative to the first orgDir so a note's audio sits in the org database beside the heading that links to it.

Orgs does **not** link whisper, and should not be made to: the model needs cgo and a built whisper.cpp, so linking it would put CMake and a whisper.cpp build on everybody who compiles orgs and never records anything. Supervising the binary keeps `go build ./...` working on a bare checkout. Three things about the supervisor are deliberate:

- A whisper **already listening on the port is adopted**, not started again — so one you run by hand still works, and an orphan left by a `kill -9` is picked back up rather than fought with. An adopted server is never stopped by orgs; it was somebody else's.
- The child is stopped **on a signal as well as on a clean exit**. `StartServer` ends the process through `log.Fatal`, which does not unwind, so a `SIGINT`/`SIGTERM` handler is the only place a ctrl-c can be caught — without it every run leaves a 500mb model resident.
- Readiness is **polled, not assumed**, for up to three minutes, and `/voice/config` reports `state` (off/starting/ready/adopted/failed) with the tail of whisper's own output. "Still loading a model" and "not installed" are both not-ready and need very different words in front of somebody about to record.

Five things to keep in mind when changing the note side:

1. **The recording is saved before anything is attempted on it, and the transcript goes back to the client before anything is written.** A transcription fails in a dozen ways — whisper down, a model still loading, a take past the timeout — and none of them should cost the words that were said. `POST /voice/note` writes the *text it was sent*, never a fresh transcription, because what lands in the file has to be what was on screen.
2. **Nothing is converted.** go-whisper decodes through ffmpeg, so a browser's webm/opus, a phone's m4a and a recorder's wav all go straight through; the extension follows the recording and the `filename` field tells whisper what container it is looking at.
3. **`/api/whisper/model` answers with a bare JSON array**, whatever the api doc says about an object with a `models` field. `whisperModels` reads both, because guessing wrong is silent: an empty model list is indistinguishable from a server with nothing installed.
4. The heading is built as **lines of text and spliced in** (`voiceNoteLines` + `insertLinesAt`), not written through go-org. Writing it through the document would rewrite the whole target file — every drawer re-indented, every table reflowed — to add one heading to the end of it.
5. A recording id **is its own filename** (`voiceIdRe`), so there is no index beside the folder to fall out of step with it, and a path that is not exactly that shape is refused rather than joined.

### Per-user extensions

`internal/app/orgs/extensions.go` is a small per-user store written beside the main config (`orgs.yaml` → `orgs_extensions.yaml`), holding the things a user accumulates rather than configures: stored queries, their own capture templates, and kanban boards. Every handler reads the username off the auth token, so there is no user parameter anywhere in the API.

Two things about the kanban board endpoints are worth knowing before changing them. A `KanbanBoard` deliberately holds **no cards** - it names a query and says how to draw whatever that finds, so it can never be stale and deleting one touches no heading. And `POST /ext/kanban/boards` replaces the whole list in one write, because renaming a board and reordering the tabs both change a list rather than one entry: done as a delete plus an add, a lost second call would leave the boards half written.

A board is drawn one of two ways, which `layout` picks: a row of columns of cards, or one grouped list with a table column per field (`listFields`, a key from worg's own list or `prop:NAME`). They are **layouts over one model**, not two boards - which group a heading is in, what order the rows are in and what a drag writes are all answered by `kanban.ts`, so a heading dragged into Done in the list is the same write as a card dropped in the Done column. Two things are the list's own. The **todo keyword leads every row** rather than sitting among the fields - it is what a todo list is read down - and it is a control: clicking it asks `/status/{hash}` what that heading's own file allows and offers those, so it is never the board's columns being offered. It is therefore filtered out of `listFields` (`PINNED_FIELDS`) rather than being pickable. And the **section order is the column order**, so moving a section up or down on a board that was reading its columns off the server's keywords takes them as its own - which is what ordering them means. How a keyword is drawn - icon, colour, and a dark-page ink as well as a light-page one - lives in `components/statuslook.tsx` and is shared with the search table, because a keyword drawn two ways is two things as far as the reader is concerned.

A third, which has bitten twice: **a board setting that is not a field of `KanbanBoard`/`KanbanColumn` in `extensions.go` is silently dropped on save.** The board travels as json both ways and go unmarshals into the struct, so a field worg knows about and the server does not survives in the browser for as long as the tab is open and is gone the next time the boards are read back - with no error anywhere. Column `aliases`, the card header's `headerKey`/`headerColors`, the board's `folded` list, and `layout`/`listFields` are all there for that reason. Add the go field in the same change as the typescript one.

The board that made it necessary is worg's Kanban tab, which writes a heading's property when a card is dropped in a column. That exposed a nil dereference in `SetProperty` (`todo.go`): a heading with no `:PROPERTIES:` drawer has `Headline.Properties == nil`, and the old `if props == nil` check could never fire, having taken the address of a field first. It now creates the drawer, which the org writer prints directly under the headline - so anything setting a property from outside the editor works on a heading that has never had one.

### Users, and the keystore

`internal/common/keystore.go` is the credential store; `orgs user` (`cmd/oc/commands/user/`) writes it and `internal/app/orgs/user.go` is the server's half. It is in `internal/common` because a CLI command package cannot import the server package, and both halves have to agree about what a password is.

Before this, **the `keystore:` setting was documented and never read**: `DefaultKeystore()` was the only thing that ever assigned `currentKeystore`, so every server anywhere had exactly one account, `admin`/`default`, and there was no way to add another. `LoadKeystore()` now runs in `StartServer` right after `Conf()` - it cannot run before, and `DefaultKeystore()` still exists as the pre-config bootstrap because the config load can fail and a nil keystore turns that into a panic rather than a message.

**Passwords are never sent to the server.** A client asks `GET /salt?user=<name>`, hashes with `common.ClientHash`, and sends that; the server hashes what it received again with its own `orgSalt` (`common.StoredHash`) before comparing. So the file holds one value, the wire carries another, and neither is the password:

- a **stolen keystore is not a set of logins** - what is in it is not what `/login` accepts;
- a **captured hash still is** a login, so this is not a substitute for TLS. It protects the password, which is reused elsewhere and outlives any session. `requireHashedLogin: true` refuses a cleartext login outright; it is off by default because a client older than the hashing would be locked out, and every such login is named in the log until it is turned on.

Six things are load-bearing:

1. **`common.ClientHash` is said again in `worg/src/clienthash.ts`**, because a browser cannot ask the server to do the one thing whose point is that it does not travel. The prefix, the separators and the order of the parts must match exactly, and nothing at build time would notice if they stopped - `TestClientHashVector` and `clienthash.test.ts` pin the **same vector** on both sides. Change one and change both, and run both.
2. **`crypto.subtle` does not exist outside a secure context**, so a worg served over plain http from another machine *cannot* hash and says so in the console rather than sending something that looks like a hash. https, or localhost.
3. **`GET /salt` answers for a user who does not exist** as readily as for one who does, with a stable decoy derived from `orgSalt` - it is the one thing a login page can ask before it has any credentials, so telling the two apart would be a way to ask the server for a list of its users.
4. **Both legacy directions are folded into one comparison** (`credMatches`): a cleartext password from an old client, and a cleartext entry in a keystore somebody typed by hand. Whatever arrives, what is compared is a hash of fixed length, in constant time. A hand-written `password: hunter2` keeps working, and `orgs user passwd` is how to convert one.
5. **A username is part of its own hash**, so the same password on two accounts is not the same value on the wire. That also means a rename invalidates the password, which is why there is no rename - remove the account and add it again.
6. **`orgs user` writes a file rather than calling an endpoint.** The keystore decides who may talk to the server at all, so an endpoint that wrote it would be an endpoint that grants access, reachable by anybody who already has some. The consequence is the one to know: it runs on the server's own machine, and the server reads the file on startup, so a change needs a restart. It is `commands.Offline` - the machine being set up for the first time is exactly the one with no server to authenticate to.

Point 6 has one exception, and it is `orgs adduser` (`cmd/oc/commands/adduser/`,
`internal/app/orgs/adduser.go`, `POST /users/add`). Adding an account over the
wire is exactly the endpoint that comment argues against, and what makes it
safe enough to have is that "anybody who already has some access" is not
enough: a `Cred` now carries `Admin`, and only an account marked `admin: true`
may add another. Where the first administrator comes from is the whole of it -
the built-in `admin` account is one, and `orgs user admin <name>` is the only
way to make another, on the server's own machine, which is deliberately *not*
reachable over the wire. An administrator who could promote an account could
promote one they had just added, and being an administrator would then be one
call away from being granted rather than something somebody decided.

Five things about it:

1. **The password still does not travel.** The client makes a salt for the new
   user and sends `ClientHash(name, salt, password)` under it, exactly as a
   login does; the server hashes that again with its own `orgSalt` on the way
   into the file. `AddUser` refuses anything that is not a client hash - unlike
   `/login`, which has to take a cleartext password because clients older than
   the hashing exist, and nothing is older than this endpoint.
2. **The account works at once**, because the server writes the keystore it is
   already holding rather than a file it will read next time. That is the whole
   difference from `orgs user add`, which still says "restart the server".
3. **Three refusals, each with a reason rather than a 403 and silence**: the
   caller is not an administrator; `noAuth: true` is set, so there is no
   authenticated user to be an administrator and an account written then would
   outlive the setting; and the server is running on the built-in keystore,
   which has no file behind it, so `Save` is a no-op and the account would
   vanish at the next restart. Each names the command that fixes it.
4. **An add never replaces.** Quietly resetting an existing account's password
   would be a way to take it over, so a name that is taken is a refusal.
5. **`SetPassword` keeps the `Admin` flag.** It used to replace the whole
   `Cred`, which would have made changing a password a way to silently demote
   somebody. `TestPasswordChangeKeepsAdmin` pins it.

`YamlKeystore` grew a mutex while this was built. It was already being mutated
from request handlers - every successful login records a time and saves - and
adding a second writer made two goroutines writing the same map a matter of two
requests arriving together rather than of bad luck.

`WarnOnInsecureCredentials` prints a block, not a line, when any account still has the default password, when any is stored as cleartext, or when `noAuth` is set. What it is competing with is three hundred lines of plugin and watcher chatter scrolling past on startup, and the state it describes - a server on a network with a published password - is one somebody has to find out about from their own logs rather than from somebody else. It names the accounts and the three commands that fix it. A keystore that is configured but absent, unreadable, or empty falls back to the built-in account rather than refusing to start (a server that would not start could not be set up), and the fallback is always said out loud in the block as `Why:`.

### Backlinks and the link graph

`internal/app/orgs/links.go` walks every `[[target][description]]` link out of every parsed file, resolves it against the database, and indexes it from both ends. It serves `/links` (backlinks for one file), `/links/graph` (the graph around a file, or the whole database, at file or heading granularity) and `/links/stats` (per-file counts). Wire types are in `internal/common/links.go`; the worg client is `components/Files.tsx` plus the plain-svg force layout in `components/LinkGraph.tsx`.

The index is cached against `OrgDb.ReloadIndex` and rebuilt whenever a file reloads. Two things about the rest of the system shape it:

1. Sections are registered into `ByHash` / `ById` / `ByCustomId` lazily, as queries walk them (`ScanNode`'s `RegisterSection` call is commented out), so `buildLinkIndex` walks every file into the registry in a first pass before resolving anything.
2. go-org ends a headline's body at a drawer written in column zero, which leaves `Headline.Properties` nil and drops the rest of that heading into `Document.Nodes` at the top level. So links are attributed to the last heading starting above them rather than trusting the outline alone, and ids are read from a local `idIndex` that also picks up those hoisted property drawers.

The query language reaches this index too. `HasBacklinks()` (optionally with a floor: `HasBacklinks(2)`), `BacklinkCount()` as a number to compare, `HasLinks()`, `LinksTo(re)` and `HasBrokenLinks()` are defined in `todo.go` and made of `BacklinksTo` / `LinksOut` at the foot of `links.go`. Two rules there: a link naming the **file** is not counted against any heading in it, or every heading in a linked-to file would claim a backlink it has not got; and `LinksTo` runs its pattern over the link as written, its description, and the file and headline it lands on, because "links to notes.org" and "links to the migration heading" are both things somebody means by it. The index is cached against `ReloadIndex`, so the first heading a query evaluates builds it and a query mentioning no link function never builds it at all.

That same worg file view shows a file three ways, picked from buttons over the page: the html exporter's page, the file's own text with org syntax colouring (`components/OrgSource.tsx` in worg - nothing server side), and, for the files `/dnd/characters` reports, the `dndsheet` character sheet fetched from `/file/dndsheet` as a string rather than written to disk.

### Todo keywords over the wire

`GET /status` answers with the keywords this server accepts - `defaultTodoStates` split the way org splits it, everything before the `|` active and everything after it finished - in the order they were configured, which is the order they are meant to be read in. `GET /status/{hash}` is the per-heading question and answers from that heading's own file (`#+TODO:`) when it has one. worg's kanban builds a board's default columns out of the global list.

### The habit tracker

`GET /habits` (`internal/app/orgs/habit.go`) answers with every `:STYLE: habit`
heading and how it is going: the current run, the best run it has ever been on,
how much of what the cadence asked for actually happened, and one entry per day
for the last eight weeks. worg's Agenda tab draws it above the calendar
(`components/HabitTracker.tsx`, model and tone in `src/habits.ts`), folding to
one line and a progress bar.

It only became possible when `ChangeStatus` learnt to write `State "DONE"` lines:
the history is read back out of each heading's own logbook, so before that this
endpoint could only have answered about habits kept in Emacs. Ticking one off in
the tracker is `POST /status/change` like everything else - the server moves the
repeater on, writes the line, and the next read of `/habits` fills the square.

The decisions worth keeping are all about **not lying to somebody about their own
habits**, which is the way a tracker fails:

1. **Three day states, not two.** `done`, `miss`, and `ok` - where `ok` is a day
   inside the grace the habit's own repeater grants. A habit written `.+1d/3d`
   not done yesterday is a day you had in hand; drawing that as a gap makes a
   well-kept habit look like a failing one.
2. **The day a habit comes round is never a miss.** You have that day to do it.
   Off by one here paints a red square on every habit every morning, which is a
   tracker that cannot be trusted and so will not be looked at.
3. **No day before the habit's first completion is a miss** either - there was
   nothing to miss yet, and a new habit opening with eight weeks of red is the
   tracker arguing with somebody who has just started.
4. **`Rate` counts from the first completion**, not from the edge of the window.
   A habit started on Tuesday and kept since is at 100%, not at 4%.
5. **"Best ever" needs a previous run to beat** (`Total > Streak`). A first
   unbroken run is technically its own best from day two, and saying so daily
   makes the words worthless by the time the run is worth something - 40 needs no
   adjective.
6. **A broken run never says what it was.** "start again", not "you lost a 40 day
   run": the job at that moment is to make starting again look small.

`habits.ts` is a separate module for the same reason `chartspec.ts` is - getting
the tone wrong is not the sort of bug a screenshot catches, so the branching is
pinned by tests. The per-file half of the walk is cached with `FileParts`; the
half that changes at midnight (the window, the streaks) is not.

#### `orgs habits`, the tracker at a prompt

`cmd/oc/commands/habits/` is the same tracker in a terminal: `orgs habits` is the
listing, `show` draws one, `done` ticks one off, `untick` takes today's tick off
again, `pick` is the fzf picker with the habit drawn beside the list, `-short` is one line for a status bar and `-w`
redraws it as the files change. One request to `/habits` and one write to
`/status/change` (or `/habits/untick`) - the tone decisions are in `view.go` beside
`view_test.go`,
which is the same split as `habits.ts` and its tests and exists for the same
reason.

The six rules above are said again there, deliberately, and **must not drift**: a
habit read in the browser and the same habit read in a terminal have to be worth
the same thing, or it reads as two habits. `view_test.go` pins them on this side.
One of them has already drifted in worg's favour and the fix belongs there rather
than here: `Cadence` answers "does not repeat" for a heading marked `:STYLE: habit`
whose schedule has no repeater, where `cadence()` in `habits.ts` says "every day" -
the server reports `Every` as 1 for those because a zero would divide, not because
they happen daily, so the question has to be asked of `Repeater`.

Four things are the terminal's own:

1. **Three day states are three *characters*** (`█` kept, `░` missed, `·` in hand)
   as well as three colours. The browser can rely on colour; here it is off down a
   pipe, off under `NO_COLOR`, off in a status bar and off whenever somebody passes
   `-no-color`, and the one rule that matters about the strip - a missed day has to
   be as readable as a kept one - has to survive all of that. The legend at the
   foot says which is which, because three shades of block are not self
   explanatory and the difference between "missed" and "in hand" is the whole
   reason there are three.
2. **The window is trimmed to the terminal from the oldest end, never sampled.**
   It is asked for once at the widest and cut per screen, the way `recentDays`
   does in worg. A strip that dropped every other day would fit any width and
   could not say what it was looking at, and the footer names how many days it is
   actually showing.
3. **The default is the tracker, not a picker.** `orgs tables` and `orgs code`
   open a picker because they are about one of hundreds of things; somebody has
   eight habits and all eight fit on the screen. `orgs habits pick` is there for
   when choosing one is what you came for, and the picker's `ctrl-d` ticks off and
   **reloads** - the square filling in is the point, so it goes through a hidden
   `orgs habits lines` rather than leaving the row it just wrote to on screen
   unchanged. `ctrl-u` takes today's tick off again, and the two are a pair rather
   than a feature and a nicety: the moment somebody finds out they pressed the
   wrong key is the moment they need the other one, and a picker with a key that
   writes and no key that unwrites is one you cannot afford to explore. Both go through `quiet()` -
   `execute` rather than `execute-silent`, output thrown away, the *refusal*
   waiting for a key - because a write that worked is about to redraw the row it
   changed, and one that could not looks exactly like a square that declined to
   change. `Binds` is a function so that `TestBindingsParse` can hand the strings
   to fzf's own parser: a malformed binding is not a dead key, it is the picker
   refusing to open.
4. **The month ruler is drawn once**, at the foot, rather than under every row.
   Every habit's window is the same days - one request answers about all of them -
   so a ruler per row is the same row of month names once per habit. worg draws it
   per row because there it has nowhere else to go. The pane is a **calendar**
   rather than a strip for the matching reason: a row answers "how often" and one
   habit on its own is being asked "when" - which Tuesdays, and whether the gaps
   are weekends.

#### Taking today's tick off: `POST /habits/untick`

A habit ticked off by accident has to be clearable, and the obvious way - keep a
copy of the file and put it back - **was built first and had to be thrown away**.
It is worth knowing why, because the reasoning applies to anything else tempted by
an undo buffer over org files: a copy of a whole file is only good while the file
is untouched, and **habits live many to a file**. Tick a second one off and the
first one's copy is stale; so is any write from worg, a clock, or Emacs. The undo
refused precisely when somebody was most likely to want it, which is halfway down a
list of habits.

`internal/app/orgs/habituntick.go` asks the *file* instead - is there a completion
recorded for today? - so there is nothing to go stale. It works on a habit ticked
off in Emacs this morning, on one ticked off twice, after a restart, and under
`-local`. `orgs habits untick [name]` is the client and `ctrl-u` in the picker is
the same call; "nothing recorded for today" is an answer rather than a failure,
which is what makes pressing the key twice harmless.

Five things it decides:

1. **A day is the unit**, which is what the whole tracker counts by, so *every*
   completion recorded today comes off rather than the last one. A habit ticked
   twice in an afternoon is one filled square and has to be cleared as one, or the
   square stays filled and the key has to be pressed again.
2. **What comes off is exactly what fills the square** - the lines
   `parseHabitCompletions` counts, `habitDoneRe` and all. A tracker and its eraser
   disagreeing about which lines count is either a square that cannot be cleared or
   a line vanishing from a file with nothing changing on screen.
3. **The date only goes back on evidence that it went forward today**, which is
   what `:LAST_REPEAT:` is. Without it the date is left alone: a habit whose
   repeater did not move today has nothing to put back, and moving it anyway would
   be inventing a schedule.
4. **A `.+` date is reconstructed from the previous completion, not by stepping
   back.** `.+2d` means "two days after I actually did it", so the date the heading
   was carrying is the *previous* completion plus two days - which is in the
   logbook, once today's line has come out of it. Stepping back one interval lands
   on today, which is right only for a habit ticked off on the day it was due: for
   one ticked off early it moves the schedule a day, and for an overdue one it
   erases the fact that it was overdue. `+2d` is exact either way and `++2d` may
   have taken several steps with nothing in the file to say how many, so those step
   back. `habituntick_test.go` pins the early, on-time and overdue cases.
5. **The keyword it had is read off the line being removed.** A habit that does not
   repeat is sitting on DONE with a `CLOSED:` stamp, and the `from "NEXT"` half of
   its own state line says what it was - so nothing has to remember it and nothing
   has to guess which live keyword to pick. The stamp comes off with it.

Everything is a **line edit** confined to the habit's own lines, `ownLinesEnd`
being what keeps a child heading's logbook out of it - a parent counting its
child's completions would clear a square nobody pressed. The extent is measured
again after every edit that changes the line count, for the reason
`applyStatusChange` gives.

Not byte-identical to before the tick, in one cosmetic way worth knowing before
diffing: dropping `:LAST_REPEAT:` re-aligns the property drawer through
`alignDrawer`, which has a minimum width, so a hand-written `:STYLE: habit` comes
back as `:STYLE:    habit`. That is the same house style `setPropIn` applies, and
the alternative - leaving the drawer padded for a key that is no longer in it - is
worse.

### Column view, and the effort rollup

`EFFORT` was read in one place - the gantt plugin, to size a bar - and nowhere
else. `internal/app/orgs/columns.go` is org's `#+COLUMNS:` as an endpoint
(`GET /columns`), and the part that makes it more than a table is the **summary
operator**: `%EFFORT{:}` means a parent shows the total of everything under it,
so a project says how big it is without anybody maintaining a number. worg's
Column View tab (`components/Columns.tsx`, model in `src/columns.ts`) draws it
and lets the cells be edited.

The line in force is the first of: the request's own, the file's `#+COLUMNS:`,
`columns.default` in the yaml, a built-in. The answer says which in `From`, so
the view can explain why an expected column is missing. The default is org's
(`%25ITEM %TODO %3PRIORITY %TAGS`) plus `%EFFORT{:} %CLOCKSUM`, because adding
effort up is what the view is for and almost no file declares a columns line.

Five things are load-bearing:

1. **A cell carries both what is shown and what is written.** They are the same
   on a leaf and different on every parent that rolls up. A project showing
   `8:45` summed from its tasks has `2h` of its own, and an editor pre-filled
   with the total writes the total onto the parent the first time anybody
   presses return in it. `Own` is what the editor gets; `Value` is what the
   table shows.
2. **The rollup is the children's total *plus* the heading's own value.** Of the
   two readings of a parent that has an estimate and children with estimates,
   this is the one that never silently discards a number somebody typed.
3. **Properties are read off the file's own lines**, not from
   `Headline.Properties` - which go-org leaves nil for a drawer written in
   column zero. A column view whose EFFORT column was blank for exactly those
   headings would read as the properties not being set.
4. **`%CLOCKSUM` rolls up whether or not an operator was written**, because that
   is what org means by it. `%EFFORT` does not: the operator is how a file asks
   for a total, and inventing one would make orgs disagree with org about what
   the file says.
5. **Sorting is within each parent**, never across the flat list - a column view
   is an outline, and sorting it flat moves children away from their parents and
   turns the indentation into a lie. An empty cell sorts last whichever way
   round the column is, because "nothing here" is an absence rather than a small
   value.

`POST /columns/spec` writes the file's own `#+COLUMNS:` line, replacing the one
it had or joining the `#+KEYWORD:` block at the top.

Two things this turned up. **`POST /property` was a whole-document rewrite**
(`SetProperty` through `WriteOutOrgFile`), so setting one estimate re-indented
every drawer in the file, reflowed its tables and ate a space in a `CLOCK:`
line. Tolerable when a property was something a kanban drag set once; not when
it is a screen somebody sits in front of filling in estimates. It is
`setHeadingProperty` in columns.go now - a line edit reaching the heading's own
drawer and nothing else, with the keys re-aligned inside it the way the record
editor does. And **`ParseDuration` counts a week as five days** (7200 minutes),
not org's seven - deliberate for estimating, and worth knowing before comparing
a total with Emacs. It is shared with the gantt plugin, so changing it would
move every bar.

### What DONE does

`ChangeStatus` in `todo.go` used to set `Headline.Status` and write the file, and
that was all of it. Org does three more things when a heading reaches a DONE
keyword, and `internal/app/orgs/logbook.go` is those three: it stamps `CLOSED:`,
it writes a `- State "DONE"  from "NEXT"  [...]` line, and it **moves a repeating
date on instead of leaving the keyword stuck on DONE**. Every writer funnels
through this one function - worg's kanban drag, `orgs todo`, the MCP tool, the
tui's `t` menu, the gantt client - so a habit touched anywhere outside Emacs was
quietly broken everywhere.

The consequence was already in the tree: `parseHabitCompletions` builds the
agenda's habit graph by reading exactly those `State "DONE"` lines, out of a
drawer nothing in orgs had ever written. `IsHabit()`, `HabitStreak()` and
`MissedHabit()` (`habit.go`) are the queries that history is worth having for.

Configured the way org configures it, narrowest wins: `log:` in the yaml
(`done`, `repeat`, `intoDrawer`, `states`, `repeatToState`), then the file's
`#+STARTUP:` words (`nologdone`, `lognotedone`, `logrepeat`, `nologdrawer` and
the rest) and `#+PROPERTY: LOG_INTO_DRAWER`, then a heading's own `:LOGGING:` or
`:LOG_INTO_DRAWER:`. Two defaults differ from vanilla Emacs deliberately -
`done: time` and `intoDrawer: LOGBOOK` - because the complaint this answers is
that marking something done recorded nothing, and the habit graph reads `LOGBOOK`
specifically.

Five things here are load-bearing:

1. **It is a line edit, not a document rewrite.** `ChangeStatus` went through
   `WriteOutOrgFile`, so ticking off one task reformatted every drawer and
   reflowed every table in its file. Everything else that writes splices lines;
   this is also what lets a timestamp keep the spelling it was written with.
2. **Every edit that adds or removes a line moves the ones below it**, so the
   heading's extent is measured again after each (`reread`). Measuring once at
   the top and using it throughout writes the second edit into the wrong heading
   and reports success.
3. **A repeat is not a closure.** No `CLOSED:` is stamped and any existing one
   comes off, `:LAST_REPEAT:` is written, and the keyword goes back to a live
   state - which is why the reply carries a `Msg` saying so. A card dragged into
   Done that reappears in Next otherwise reads as a failed write.
4. **The three repeater spellings are three different sums.** `+2d` counts from
   the old date and may stay in the past; `++2d` counts from it until strictly
   in the future; `.+2d` counts from today - but keeps the *time of day* the
   stamp carried, or a daily 09:00 reminder walks round the clock over a week of
   being ticked off late.
5. **A day is the unit for habit counting**, so a habit ticked twice in an
   afternoon is one day's worth and a double-click is harmless.

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

### Filters and tag groups

`Filters` and `TagGroups` in the YAML are macro-like helpers referenced by queries. `AddInternalFilters` / `AddInternalTagGroups` in `settings.go` seed a default set (`AllTasks`, `HomeTasks`, `WorkTasks`, `WorkProjects`, `PERSONAL`, `HOME`, `WORK`) unless `noInternalFilters` / `noInternalTagGroups` is set. Queries reference filters using `{{ FilterName }}` handlebars-style substitution.

### The D&D module

The Dungeons & Dragons character module spans four places:

- `internal/common/dnd/` — the engine, shared by server and CLI (so it must not import `internal/app/orgs`). `types.go` is the data model, `ruleset.go` loads and merges yaml ruleset modules, `rules.go` computes a `Sheet` from a `Character`, `builder.go` is the interactive creation state machine, `orgfile.go` reads/writes the org character sheet, `view.go` converts a sheet into the map form the templates consume, `inventory.go` stacks the equipment table into containers and applies inventory changes, `spellcast.go` works out what casting a spell rolls, `slots.go` spends and hands back the spell slots that pays for, `money.go` is the coin purse and the arithmetic of paying out of it (including making change), `spellbook.go` decides which spells a character may learn or prepare and holds them to the allowances their class gets, `fuzzy.go` is the list matcher shared with the CLI chooser, `ddbeyond.go` converts a D&D Beyond character-service payload into a `Character` by matching its names onto ruleset ids, `uses.go` reads "so many uses per rest" limits out of a feature's own rules text, or off a `usesByLevel` column the ruleset declares for the resources whose size the rules print in a class table instead, `rest.go` works out what a short or long rest asks for and gives back, `conditions.go` holds the condition catalog and the damage a character shrugs off (`DefendDamage` applies a resistance, immunity or vulnerability to one blow), `rolleffects.go` works out what those conditions do to a d20 - the advantage, the disadvantage, the saves that fail outright - and hands the sheet a map to look rolls up in, `health.go` is damage, healing and temporary hit points between rests (and `HealthLevel`, the band that colours the sheet's hit point bar), `inspiration.go` is the one bit the DM hands out and the player spends, `concentration.go` is the spell a caster is holding and the Constitution save a blow costs them, `consume.go` reads what drinking a potion does out of the item's own text and applies it, `shop.go` buys and sells against the purse in one call, `undo.go` takes the last operation back off the logs - the item, and the hit points or the coin that moved with it, `portrait.go` picks the drawn fallback portrait for a character with no art of their own, and `backdrop.go` is the scenery washed out behind the html sheet - the pictures `DND_BACKDROP` names (a folder stands for everything in it), how long each stays up and how strongly it shows through.
- `internal/common/dnd/data/*.yaml` — the SRD ruleset, embedded into the binary with `go:embed`. **Generated** — do not hand edit; regenerate with `python3 tools/dndsrd/gen_srd.py ../dndsrd` (the [dndsrd](https://github.com/OldManUmby/DND.SRD.Wiki) markdown SRD).
- `internal/common/dnd/data/portraits/*.svg` — one drawn fallback portrait per race, embedded the same way and used when a character has no `DND_IMAGE`. They cover the SRD races plus the ones the common supplement rulesets add (aarakocra, tabaxi, aasimar, firbolg, triton, goliath, warforged, yuan-ti, cairnborn), and each is drawn to what its race entry actually says — a tabaxi has a cat's ears, muzzle and slit pupils — rather than being a recoloured human. Also **generated** — regenerate with `python3 tools/dndportraits/gen_portraits.py` rather than editing the svg.
- The html sheet's own side drawers: the dice tray (whose auto-open is a three-way setting - every roll, not on casts, never - because a cast rolls to hit and for damage at once and is the one that gets tiresome), the level-up tray, and the session drawer, which holds Notes, Timeline, Combat, Sessions, Inventory and Search.
- `internal/app/orgs/dnd.go` — the `/dnd/*` REST endpoints and the character build session store; `dndsession.go` the play session logs and `dndinventory.go` the inventory endpoints, `dndspells.go` the spell and spell slot endpoints, `dndmoney.go` the coin endpoints, `dndrest.go` the rest and feature-use endpoints, `dndconditions.go` the conditions and defenses endpoints, `dndhealth.go` the hit point and death save ones, `dndconcentration.go` what a caster is holding, `dndinspiration.go` the inspiration marker, and `dndundo.go` the undo the html sheet's menu presses, all of which rewrite the character's own org file, and `dndimport.go` the D&D Beyond import (the CLI does the fetching, so no D&D Beyond credential reaches the server). `internal/app/orgs/plugs/dnd/` owns the shared ruleset library plus the `dndsheet`/`dndlatex`/`dndpdf` exporters.
- `cmd/oc/commands/dnd/` — the `orgs dnd` client, and `templates/dnd_character*.tpl` are the html and latex sheets.
- `internal/common/dnd/levelup.go` — the level-up flow behind `orgs dnd levelup` and `POST /dnd/levelup` (`internal/app/orgs/dndlevelup.go`). It emits the same `Prompt` type the builder does, so the CLI's chooser renders it unchanged - and so does the html sheet's level-up tray, which walks exactly the same prompts in a drawer beside the dice one.

A few rules to keep in mind when changing it:

1. The builder replays every stored answer onto a fresh character whenever anything changes, so an `apply` function must be deterministic. Anything random (dice) has to be stored on the `Answer` (see `Answer.Pool`) or it will change under replay.
2. Only the property drawer plus the equipment, spell, inventory history, coin history, condition history and health history tables are parsed back out of an org sheet. Every other section is regenerated by `RenderOrg`, so derived data must never be the only copy of something. In particular the `*** Defenses` block under Combat is regenerated from `DND_CONDITIONS` / `DND_RESISTANCES` and friends; editing it does nothing.
3. A short rest hands back one spell slot of each level the character has spent one at. That is a house rule, not the SRD - by the book only a warlock's slots come back on a short rest - and it lives in `ApplyRest` and `RestPlan` in `rest.go`, pinned by `TestShortRestGivesBackOneSlotOfEachLevel`.
4. `HealthLevel` in `health.go` bands the hit point bar, and the html sheet says the same bands again in javascript (`hpLevel`) so a rest can recolour the bar without a round trip. Change one and change both; `TestHealthBands` pins them.
5. The use limits in `uses.go` are parsed from prose, so they are best effort by design: a limit the text does not actually state is never guessed at. The three resources whose size lives in a class table instead - rage, ki, sorcery points - carry a `usesByLevel` column emitted by `USES_FEATURES` in `tools/dndsrd/gen_srd.py`, and a declared column or formula always beats the prose. `rest_test.go` pins both the wordings and the columns that must keep working; add a case there before loosening a pattern.

6. `LevelUp` in `levelup.go` never modifies the character handed to it — it returns a levelled copy. The flow is stateless: the client resends every answer each call and the engine replays them, so levelling in place would add the levels again on every pass. Related trap: `classLevelOf` answers with the character's *whole* level when they do not have the class, which is right while building and wrong for multiclassing; `classLevelIn` is the strict one.
7. Ruleset merging is not uniform: an entry under `races:` or `classes:` whose id already exists is merged field by field, but one under `spells:`, `items:`, `backgrounds:` or `feats:` **replaces** the existing entry outright. So a module must never restate an SRD spell just to change one field — it would fork the generated data and go stale. To hand an existing spell to another class, use the `spellLists:` block (`spellLists: {druid: [...]}`), which appends the class at index time and accumulates across modules; `spelllists_test.go` pins it.
8. What an item does when you use it up (`consume.go`) and what it is worth (`shop.go`) are read from the ruleset, never from the page: the html sheet is handed `use`, `price` and `sale` on each inventory entry and posts back what its dice landed on. As with `uses.go` the text parsing is best effort — healing that the item says it gives *every hour* or *while worn* is deliberately not a dose, and `TestItemUseFromSRD` pins the three SRD items that do have one, so a new hit there is a change to look at rather than a free win.
9. Conditions reach the dice through `rolleffects.go` and nowhere else. The page is handed a `RollAdviceView` - what each kind of roll is owed, keyed by ability for saves - and never learns what "poisoned" means, so a ruleset module that adds a condition adds a row to `conditionRules` and the sheet needs no changes. Two things there are easy to get wrong: advantage and disadvantage **cancel** rather than counting, and nothing is ever forced - the sheet still rolls all three readings and only changes which one stands, because caveats like "while the source of your fear is in sight" are the table's to judge. Every rollable in the template must carry `data-roll-as` (and `data-ability` where it has one) or it is owed nothing.
10. Undo (`undo.go`, `/dnd/undo`) reverses only the *last* operation, and only when it really is the last. It is built out of the history tables rather than out of any server side state, which is why the Health History carries a `Was` column and the Coin History one too: where a character stood before a line cannot be worked back from the numbers after it, and undo must never guess. A line read off a sheet written before those columns existed has no before reading at all, and is refused rather than treated as a character who was on nothing. The other guard is that the character must still stand exactly where the line left them - anything else means a blow, a rest or a hand edit has happened since.
11. The command palette (`/` or ctrl-k on the html sheet) indexes the page's own controls - every `.rollable[data-label]`, every `.cast-btn`, every usable inventory row, every `.feature[data-uses]` - rather than a list kept beside them, so it cannot go stale and anything the sheet grows later turns up in it unbidden. Its matcher is `FuzzyScore`/`FuzzyScoreAll` from `fuzzy.go` said again in javascript, because a round trip per keystroke is the delay the palette exists to remove; change one and change both, or the letters that find a spell in the terminal chooser stop finding it here.
12. What a spell looks like is decided in the sheet's `ELEMENTS` table and nowhere else. An element is found in this order: the spell's own **name** (`SPELL_ELEMENTS` - water, earth, healing, holding, missiles and bangs are things the rules describe in prose and never label), then its **damage type**, then the magic circle as a fallback, so every cast shows something. An element may carry a `variants` list and `pickLook` chooses one per cast. Two traps: the elemental layer draws through `projectUp`, a *leaned* camera, because the dice camera looks straight down and anything standing up would project to nothing; and every effect must take its times from the frame's `now` rather than the wall clock, or it is already expired on its first frame.
13. The `SPELL_ELEMENTS` list is walked **in order** and the first regex that matches wins, so where a pattern sits in it is part of what it means. Storms and hands go near the top for that reason: sleet storm does no damage at all and would otherwise fall through to the magic circle, and "Bigby's grasping hand" would be claimed by the binding regex's `grasp\w*`. A weapon usually arrives already labelled with its damage type and never reaches the list at all.
14. A **skill roll** can have a flourish too: `SKILL_ELEMENTS` maps a `.rollable`'s label to an element, and `specFor` puts it on the spec the same way a spell's damage type does. Only the skills with an obvious picture are in it - there is no drawing of an Athletics check that is not silly - and a skill with no entry gets the dice and nothing else, which is what keeps the ones that do have a flourish worth looking at. The magnifying glass is the one that also needs to know *what you rolled* (it breaks on a low roll and sparkles on a high one), so the roll's natural d20 is handed to `throwDice` as `opts.quality` and the board keeps it.
15. Anything built as one merged silhouette - the ghost hand is the only one so far - must be **filled**, never stroked, as a whole. `ctx.stroke()` on a path made of many subpaths outlines every one of them, so the palm gets drawn straight across the backs of the fingers lying on top of it; the rim is an under-fill of the same shape a hair proud of itself instead. The same routine is also why the hand reads at all: nails, knuckle wrinkles, tendons, veins, web notches and a lit side are each one line of drawing nobody would miss on their own, and together they are the whole difference between a hand and a cartoon of one.
16. Anything flat that stands up facing the reader - a shield, a book, a hand, a cat - uses `DiceBoard.prototype.billboard` for its frame. The obvious alternative is to project three points (origin, one across, one up) and make a basis from them; that is what the first of these did and it is wrong, because the three points sit at different heights, `projectUp` scales by height, and the frame comes out with a shear in it. A hand survives that and merely looks like it is leaning; a rectangular shield came out as a parallelogram lying on the table.
17. **When replacing a block of the sheet template by index, anchor both ends.** The flourish routines are not in the order they were written - each new one was inserted before the banner of the last, so the file runs newest-first - and `s.index(endBanner)` without a start offset will happily find a banner that sits *earlier* than the block being replaced. `s[:a] + new + s[b:]` with `b < a` then duplicates everything between them, the stale copy is defined later and silently wins, and the first sign of it is an effect that has stopped responding to edits. This has happened twice; `TestNoDuplicateFlourishRoutines` is there to catch the third time.
18. Five of the flourishes are **real meshes**, not drawings: the cairn's stones, the panther, the mage hand, the shield and the rocks they share. `meshBuilder().tube()` walks a path, puts a ring of points round each node and stitches consecutive rings into quads, so an animal is nine calls to it and a hand is seven; `rockMesh` deforms an icosahedron instead. `DiceBoard.prototype.drawMesh` transforms, projects, drops back-faces, sorts far-to-near and flat-shades each face from its own normal - no gradients anywhere, which is what makes low poly look low poly. Two modes: `solid` for stone and `wire` for anything conjured (a faint fill so the far side is hidden, and all the light in the edges and the vertex nodes). Five traps: a mesh built with its own z as "up" is seen from **directly above** by this camera and reads as a column, so anything meant to be looked at from the side is pitched a quarter turn; a shield is a dished plate rather than a closed solid, so its face and rim are wound against each other and it must ask for `twoSided` rather than be culled, and its dish must be **positive** or the quarter turn leaves it facing into the paper; `tube()` guesses a ring's frame from the direction of travel, and for a tube running along z the guess swaps which pair of axes the two radii land on - so anything whose cross-section is not round (a palm is nearly 3:1) has to pass `upHint` or it comes out on its edge; a tube cap must be one polygon rather than a fan, because a fan is a ring of slivers and the wireframe's wide glow pass turns each one into a spike; and a pose change means rebuilding the mesh, so the panther quantises its leg swing to twelve poses rather than rebuilding every frame.
19. Two things about drawing on the flourish layer are easy to get backwards. First, **the page is cream**, so added light does almost nothing: an effect built out of `globalCompositeOperation = 'lighter'` alone washes out over parchment. Anything meant to look bright needs a dark pass behind it in normal blending first - `drawStrike` lays a soft bruise of dark ink down before the four light passes, and the sparks, motes and embers draw their glow with `lighter` and their core in a saturated colour without it. For the same reason the strike's `flash` **darkens and cools** the page rather than whitening it; it is also a full-screen brightness change, so it is skipped outright when `FLASH_OFF` (read once from `prefers-reduced-motion`). Second, an effect's **sound** is chosen by `ELEM_VOICE` keyed on the look's `kind`, not on the element, and a kind with no entry is silent. Every voice holds a table of variants and picks one per cast with `pickOne`, the same way `pickLook` works on the drawing side; `bind` is the exception that is handed the look's `lay` instead of a loudness, so hemp, chain and vine each get their own noise - so a ruleset module adding a cold spell gets the ice picture and no sound, which is the right default for something nobody has designed a noise for.
20. Undo has two mechanisms and tries them in that order. `dndjournal.go` keeps a copy of the last thirty files this server changed, hooked into the two choke points every write goes through (`dndWriteCharacter` and `updateDndSession`), and an entry is only good while the file is still exactly as it was left - anything else and putting the old text back would throw that away, so the entry is dropped. That is what reaches the changes which deliberately write nothing down, and session files as well as character ones. When the journal has nothing to say, `undo.go` works the last operation out of the character's own history tables instead; that one survives a restart and works on a file something else changed. The journal is memory only on purpose: it is a convenience for the last few minutes of play, not a record.
21. Deleting is not the same as doing something. `InventoryDelete`, `DeleteNote` and `DeleteRoll` all take a line off the sheet and write **nothing** to any history: they are corrections to the record - a line added by a stray click, a note nobody wants kept - rather than events in the character's life, and a history line about one would be a record of something that did not happen. Dropping, using and selling are the ones that leave a line. The two note and roll deletes also take a `was` (the note's stamp, the roll's label) and refuse when it no longer matches, because deleting shifts every entry after it up by one and a stale page would otherwise throw away the wrong one.
22. Notes keep the line breaks they were typed with. Org runs consecutive prose into one paragraph, so `orgBreakLines` in `sessionlog.go` writes org's hard break (`\\`) wherever two lines would otherwise be flowed together, and `orgStripBreaks` takes it off again on the way back - only where the writer would have added one, so the round trip is exact. The html sheet's own `orgToHtml` keeps them too; change one and change both or the sheet and the export disagree about what the note looks like.
23. Every pane of the session drawer scrolls on its own (`.nd-view { overflow-y: auto }`), because the drawer is a fixed height and a night's timeline or a fight with nine combatants in it runs well past the bottom of it. The notes pane is the exception - it is a two-column split whose halves scroll themselves, and a pane scrollbar on top of those would be two bars doing different things. The **Timeline** and **Combat** tabs are both worked out rather than stored. The timeline reads a session's two logs back and finds the shape of the evening in them - rolls closer together than `TL_GAP` minutes and at least `TL_LEAST` of them are one Combat block, notes written inside one hang off it, and `TL_SCENE` minutes of quiet is a scene break - so a session edited anywhere else is right the next time it is opened, and nothing is written back. The combat tracker is the one part of the sheet that is about the table rather than the character: initiative, rounds, turns, hit points for whatever the party walked into, and two kinds of countdown - an effect on a combatant, which ticks at the start of *their* turn (where the rules put "until the end of your next turn"), and a clock on the table, which ticks at the top of each round. It lives in `localStorage` and talks to no endpoint, because six goblins have no business in a character's org file once the fight is over.
24. Concentration is stored, not derived (`DND_CONCENTRATION`), the same way the conditions are. `/dnd/hp` works out the DC a blow calls for — 10 or half the damage, counting what temporary hit points soaked up — and hands it back on the answer as `save`; the roll is the sheet's and comes back to `/dnd/concentration`. Going to zero hit points ends it with no save at all, and so does a long rest.
25. The timeline's **annotations** are the one thing in it that is stored (`sessionmark.go`, a `* Timeline` section of the session file). Everything else about a block is worked out from the logs each time, so nothing about the block may be written down beside the annotation - not how many rolls it held, not how long it ran - or it goes stale the moment a roll is corrected. The anchor is a **time and a kind together**, and the kind is part of it rather than a hint: a name written about the fight at 19:32 must not silently reattach itself to a note taken at the same minute. A mark that finds no block is drawn as a beat of its own rather than dropped, which is both how a name survives the block it named being deleted and how a moment nobody rolled for gets onto the timeline at all - that is what the toolbar's *Annotate* writes, with kind `moment`, which no block ever has.
26. Deleting a timeline **block** is one call (`POST /dnd/play/session/{id}/delete`) carrying every roll and note it is made of, not one call per line. That is what makes it one write, one journal entry and **one press of undo**; a refusal anywhere refuses the lot, because half a fight deleted is worse than none of it. `DeleteEntries` removes back to front so the shifting never reaches an index it has not used yet.
27. The timeline has its **own** undo (`/dnd/play/session/{id}/undo`), which is the journal narrowed to one file (`dndLastChangeTo`). The character sheet's Undo button takes back the last thing that happened anywhere; a drawer showing one evening must offer the last thing that happened to *that evening*, or deleting a block and then rolling a die leaves the timeline offering to un-roll the die. Only the newest entry for a file is ever offered, which is what makes taking one out of the middle of the journal safe.
28. Folding and the timeline's search box are the page's own and are written nowhere - they are about reading the evening, not about what happened in it. One card folds and opens by **its dot on the spine** (`tlDot`), not by a chevron among the tools: the dot is already the mark the eye runs down looking for a place in the evening, and the tools on the right are the two that change the session file - name this, throw this away - so a third button among them that only changed what you could see read as one of those. Two things there are easy to get wrong: *fold all* clears the per-card overrides, because a card left open an hour ago quietly staying open is not what the button says; and the search matches on **everything a card holds** (`tlHay`), not on what it is currently showing, or folding the timeline would hide the very spell somebody folded it to go looking for.

### What a kanban card shows of its heading's text

A card reads three things out of the body under its heading: a **checklist**, a **recording** and a **picture**. All three come from `GET /body/{hash}` (`internal/app/orgs/checklist.go`), which answers with the body **as it is written in the file** - not `/todohtml/{hash}`, which is right for reading and useless here: a rendered `- [ ]` is a disabled input with no way back to the line it came from.

Four things to keep in mind:

1. **Ticking a box is a line edit**, `POST /checklist`, and nothing else. `/body/change` exists and would work, but it parses the new text and writes the whole document back through go-org - a lot of file to risk for one character, and every drawer and table reformatted on the way past.
2. The write says **which box and what it said**. The index alone goes stale the moment a line is added above it; the text alone cannot tell two identical items apart. Both have to agree or the write is refused. It also says the state it wants rather than asking for a flip, so a double click cannot land as two flips.
3. A `*` in column zero is a **heading, not a bullet**. `* [ ] something` is a heading whose text happens to start with a box, and counting it would put another card's item on this one. Client and server match the same shape and count the same items - they have to, because the client numbers the boxes and the server counts them again to find the line.
4. The audio and image urls are resolved **server side** and handed over ready to use. A link is written relative to the org file that holds it and the file server is rooted at the first org directory; the client knows neither. It is the same sum the html exporter does for its audio players.

Bodies are cached in the browser by hash. A hash does not change when the text does, so nothing expires on its own: the board clears the cache when it reads itself back, and a tick refreshes just that heading. Both **replace rather than drop** - the old body stays on screen until the new one arrives, because blank is worse than stale and dropping it makes the whole checklist blink out on every tick.

### The search tab's inspect view

Inspect shows a heading three ways at once: its rendered html for the prose, a player for a recording it points at, its pictures, and its tables drawn the way the Tables tab draws one (`components/OrgTableView.tsx` - read-only, with the `@n`/`$n` rulers).

The tables and pictures come from `/body/{hash}` rather than from the html, and the rendered `<table>`s are **hidden in that view** (`'& table': { display: 'none' }`) so nothing is shown twice. Org tables are parsed by `tablesIn` in `worg/src/orgbody.ts`: a `|---+---|` line is a rule rather than a row of dashes, a `#+TBLFM:` line is not a row at all, and a blank line ends a table the way org ends one.

The prose is shown in the html theme the files view uses, which is what `?theme=` on `/todohtml/{hash}` is for: the answer carries the theme's stylesheet in `Style`, since a fragment has no `<head>` to put it in. The files view can afford an iframe, which gets a document for free at the price of a fixed height; a popup has to grow to whatever the heading is, so `components/ThemedHtml.tsx` puts the fragment in a **shadow root** instead, where the theme's stylesheet reaches the heading and nothing else on the page. `worg/src/htmlframe.ts` rewrites the two things that have no meaning inside one, and getting either wrong is silent - the heading still renders, in whatever survived:

1. `html` and `body`, which are the theme's name for the page, become `.org-page`, the wrapper the fragment sits in. The theme's page box (`padding: 1.5em`, `margin: 3em auto`, `max-width: 40em`) is then overridden, because a popup is already a frame.
2. `:root`, which is where most of these themes keep their whole palette, becomes `:host`. `:root` matches the document element, which a shadow root does not have - so every `var(--bg)` resolved to nothing and the heading came out in the theme's fonts with none of its colours. `htmlframe.test.ts` pins both rewrites.

### Editing a row of the search table in place

Four of the table's columns are edited where they sit rather than in a dialog in the middle of the window: the **keyword**, the **date**, the **tags** and the **properties** (`worg/src/components/Search.tsx`). Changing one of these is something you do to a row and then to the row under it, so a lightbox is a trip across the desk every time.

The keyword is a Joy `Dropdown`, the same control the kanban list's rows use (`StatusCell` in `KanbanList.tsx`), and like it asks `/status/{hash}` when the menu opens rather than while two hundred rows are drawn - the keywords offered are the heading's own file's. The other three are a `Popper` with a `ClickAwayListener` rather than a `Menu`, because what is in them is a form and a menu moves the focus with the arrow keys and answers to typing.

One rule holds all four together: **a cell that opens a panel must not stop the click from propagating.** The rows have no click handler of their own, so there is nothing to guard against - and the panels close themselves through a `ClickAwayListener`, which listens on the *document*. `stopPropagation` keeps the native event from ever reaching it, so opening one panel leaves the last one still on screen and the table ends up wearing two.

Two smaller things. The tags panel is one box that both filters the tags the database already knows and, on Enter, adds one it has never seen - and it keeps the focus through a chip click, because tagging is usually more than one tag. And the date panel is the only one with a Save button: a date with no kind is meaningless and a time with no date is nothing at all, so the three are held together and sent at once, where the others commit as you go.

### Records: the contact book, and everything else worth a list

A **record** is one heading standing for one thing - a person, a laptop, a playing card. The heading is its name, the property drawer is its fields, the body is the notes, the `LOGBOOK` is the history. `docs/records.org` is the format written out for somebody who wants to type one by hand; `internal/app/orgs/records.go` carries the same thing as an SDOC block, plus the engine and the endpoints. `internal/common/records.go` is the wire, `worg/src/records.ts` the client half, `RecordBrowser.tsx` the view both worg tabs are made of, and `cmd/oc/commands/rec/` the terminal client - with `cmd/oc/commands/contact/` the same engine with the address book's manners on.

One property is the whole identity: **`:RECORD: contact`**, whose value names the collection. A heading with one is a record wherever it sits, and a heading without one is not a record however it looks. `IsRecord()` and `IsCollection()` are query functions, so the search tab leaves them out with `!IsRecord() && !IsCollection()` rather than filtering in the browser.

Six things to keep in mind when changing it:

1. **A field's kind is worked out from its name, never declared.** `EMAIL_WORK` is an email labelled work, `PHONE_MOBILE` a phone labelled mobile, `BOUGHT_DATE` a date called bought. That is what lets a contact and a guitar pedal share one view, and it is why nothing in `RecordBrowser.tsx` names a property. The rules live in `fieldOf` in `records.go` and are **said again in `records.ts`** so an add form can show the right input before anything is saved - change one and change both. The same goes for which date fields are an *occasion*: `birthdayFields` in Go, `CELEBRATIONS` in TypeScript. A laptop's purchase date is a date; "turns 3 in 5 months" about a laptop is the view being clever at the reader's expense.
2. **Every write is a line edit, not a document rewrite.** A record shares its file with a hundred others, and writing the parsed document back would reformat all of it to change one phone number. The drawers are found by walking the lines under the headline rather than by asking go-org, for the column-zero reason that bites everywhere else; after any edit that changes the line count the record's range is **found again** rather than adjusted; and the property drawer is re-aligned afterwards, because a record is a text file somebody opens in an editor.
3. **`RECORD` and `ADDED` are refused by the update endpoint.** One is the record's identity and the other is when it started - neither is somebody's to edit through a form. `ID` is allowed, because a duplicated entry needs its old one cleared.
4. **Reading every record means walking every section of every file and opening every file they are in**, so it is cached against `OrgDb.ReloadIndex` the way the link index is. The contacts tab searches as you type; without the cache that is the whole database per keystroke. Nothing can go stale behind it, because anything that writes a file bumps the counter.
5. **`InCollection("contact")` is the loose one and `IsRecord("contact")` the strict one.** `IsRecord` answers only for the records themselves; `InCollection` walks up the outline and is also true for the collection's container and for anything written underneath it. A contact's notes are not a record — they have no `RECORD` property of their own — but they are part of the address book, and a query saying "not the address book" means them too.
6. **Birthdays are worked out, never stored.** `/records/birthdays` takes a window and answers with the occurrences in it, so the agenda, the CLI and the contact card all get the same answer - including the 29th of February, which is kept on the 28th in a year that has no 29th. A birthday corrected in the file is right everywhere on the next read.

The agenda draws them as all-day entries with a cake, coloured by a `BIRTHDAY` entry in its own `STATUS_COLORS`. They are merged into the agenda's two lists (`dayShown`, `everything`) rather than into either fetch, so whichever request lands second still shows both.

### The Links tab: where every link goes

`GET /links/all` (`internal/app/orgs/linklist.go`) is the flat sibling of `/links` and `/links/graph`. Those answer "what points at this file" and throw away everything that is not org to org; this keeps **all** of it, because the links worth going back and finding are usually the external ones - a ticket pasted into a heading eighteen months ago is findable by grep and by nothing else. It is built from the same cached index the graph is, so it costs nothing extra and the two cannot disagree about what a link is.

Four things it decides, all of them server side:

1. **The service** (`serviceOf`) - the name a person would use for where a link goes. A short list covers the ones that are either not named after themselves (`*.atlassian.net` is Jira) or worth gathering under one name (drive, docs and sheets are all Google Docs); everything else is called after its own domain. The list cannot be complete, which is why the fallback matters more than the entries: a host nobody has heard of still has to land in a group of its own rather than in no group.
2. **The scheme and host** (`linkSchemeHost`) are only read for a link the resolver itself calls external. `splitProtocol` calls anything before a colon a protocol, so `notes.org::*Plans` comes back as a "scheme" called `notes.org` - checking against `externalProtocols` is what keeps the answer agreeing with the `Kind` the link was given.
3. **A `file:` link is grouped by its protocol, not as "external"**, or a folder of pasted screenshots ends up in with the things that are genuinely elsewhere.
4. **A link naming a file on disk is resolved to something showable** - `mediaURL` plus `mediaKindOf`, the same sum a source block's result file and a kanban card's picture do - so the tab can show the picture rather than the path.

The client half is `worg/src/links.ts` (pure, tested) and `components/Links.tsx`. The **regular expression** is the reason the model is a separate module: somebody typing a pattern types half a pattern first, so "does not compile yet" is an ordinary state of the box - it says why, and it **keeps matching everything** rather than emptying the list under the person typing. The pattern runs over everything about a link (`linkHay`: target, description, host, service, heading, file, and where it lands), because "that jira link about the migration" and "that link in the meeting notes" are both things people type.

The right-hand pane shows the far end of whatever is selected: an http(s) page in a sandboxed frame, an org link's target heading rendered through `ThemedHtml` the way the search tab's inspect view draws one, and a picture, player or pdf for a link at a file. **A framed page may refuse to be framed** - most big sites do - and there is no way to ask in advance, so the way out ("open it in a tab") sits above the frame from the start rather than appearing after a blank.

### The Code tab: finding source blocks

`GET /code` (`internal/app/orgs/code.go`) answers with every `#+BEGIN_SRC` block, **read** rather than merely located: language, name, switches, babel header arguments and variables. worg's Code tab (`components/Code.tsx`, helpers in `src/code.ts`) is the Tables tab for code.

The reason it is an endpoint rather than a grep is the **variables**. `:var data=monthly` and `:var scale=2` are the same shape and mean completely different things — one names a table further up the file, the other is the number two — and only the database can tell which. Each variable comes back resolved: what it points at, what kind of thing that is, where it lives and, for a table, its shape. A name that resolves to nothing comes back with an empty kind and is drawn in danger colours, because that is a block that cannot run.

Four things go-org makes harder than they look, all of them silent when got wrong:

1. **`Headline.Blocks` is not the blocks under a heading.** The loop that fills it `break`s after the first node it looks at, so a heading with a paragraph and then a block has an empty list — which is most headings with a block in them. The blocks are walked out of the node tree instead, and attributed to the **last heading starting above them**, the same rule the link index follows and for the same column-zero-drawer reason.
2. **A block never sees its own `#+NAME:`.** The parser consumes it as a *named node* keyword, so it is absent from the block's keyword list; and the name map it goes into is then overwritten by a `#+RESULTS: chart` under the block, which registers the *result* under the same name and wins by being written later. The name is read off the file's own lines instead, scanning up over the affiliated keywords.
3. **`Block.Result` is a `Result` value, not a `*Result`.** Asserting only the pointer form gave no result, no error and nothing on screen to say a block had ever been run.
4. **`splitParameters` leaves the switches glued to the language**, because it splits on `" :"` — so `python -n -r` arrives as one token and the first word has to be taken off it.

The index is cached against `OrgDb.ReloadIndex` like the records, because the tab searches as you type. The language counts are of the whole database rather than of what survived the filter: counting the filtered set would empty the strip the moment a language was picked and leave no way back.

### Running a source block

`POST /code/run` (`internal/app/orgs/babel.go`) runs one block and hands back what it produced. It is **off unless `babel.enable` is set** in the server settings, and the refusal says what to write: reading somebody's org files and executing the programs inside them are different promises, the server may be reachable from more than the machine it runs on, and `noAuth` is a setting people use.

Three things carry the feature:

1. **The variables are written in front of the code in the language's own syntax.** `:var scale=2` becomes an assignment; `:var data=monthly` naming a table becomes that table as a list of lists, with cells that parse as numbers left bare so `sum(r[1] for r in data)` works without the block converting anything. This is what the resolution in `/code` was *for*. Go and emacs-lisp run without their variables rather than pretending — a prepended assignment does not survive either — and that is stated rather than silent.
2. **The code is dedented before it runs**, and arrives dedented on the wire. A block under a heading is indented, and that indent belongs to the org file rather than to the program: handed to python as it stands it is an `IndentationError` on line one. `UpdateBlock` re-indents on the way back, so the round trip is exact.
3. **A table comes back as org table text.** The server does not invent a row format and the client does not need one: `tablesIn` parses it and `OrgTableView` draws it — the same parser and the same viewer the Tables tab uses. `:results` is read where it says something and the shape is guessed from the output where it does not, and the guess is deliberately timid (org table, tab-separated, or a printed list of lists) because a paragraph cut into columns on a hunch reads worse than a paragraph. Whatever was guessed, "as it printed" is one click away.

A result that **names a file** is only half an answer, so the server goes and looks. `describeResultFile` reads the name out of the link, resolves it against the block's own directory first and then each org directory, and hands back what it found: the size, a url this server will serve it from, what kind of thing it is (`Media`: image, audio, video, pdf, text or binary), the language to colour it as, and - for text small enough to be worth it, capped at 512kB - the contents. The client draws the link exactly as before and then a second box: a picture, a player, or the file's own text coloured by `CodeText`. Four things about it:

1. **The path is resolved server side.** A block runs beside the org file it is written in, so that is what `plot.png` is relative to - and the client knows neither that directory nor the org roots. It is the same sum `MediaSrc` does for a voice note's audio.
2. **A block that printed its own link keeps it.** `babelFormat` used to wrap the output in `[[file:...]]` unconditionally, which turned a printed `[[file:plot.png]]` into `[[file:[[file:plot.png]]]]` - a link naming no file. Printing the link is what emacs babel puts in the buffer, so it is the common case; `TestFileResultIsNotWrappedTwice` pins it.
3. **A named file that is not there is said out loud.** The link on its own reads like an answer, so a block that named a file it never wrote has to be caught - that is most of why the server looks at all.
4. **Named like text and full of bytes is `binary`, not printed.** The extension is a claim, and `looksLikeText` checks it against the first 8kB before a megabyte of noise is put on the page.

`POST /code/update` writes a block's name, header line and body back. The header is rebuilt from its parts in org's own order — language, switches, variables, then the rest — so a block edited in worg reads like one written by hand; `headerLine` says that order in Go and `headerLine` in `worg/src/code.ts` says it again for the preview, and the two have to agree. Only the lines the block occupies are touched, and the body is spliced before the header and the name are, so every row used is still the row it was worked out from.

The Code tab gathers its list three ways (`codeGroupBy`): flat, by language, or by file with the Tables tab's base-name formatting and the full path on the tooltip. Same shape as the search tab's grouping — a sort plus bands in the same list.

### Syntax colouring inside a source block

The code inside a `#+BEGIN_SRC` block is coloured by `worg/src/codehl.ts`, painted out of **the org theme the reader chose** — so a block of python in a Nord file looks like Nord and the same block in Solarized looks like Solarized. That is why it is hand written rather than a library: highlight.js and its kin arrive with their own themes, and a theme that cannot follow the one next to it is the whole feature missed. A scheme publishes eight new roles (`codeKeyword`, `codeString`, `codeComment`, `codeNumber`, `codeFunc`, `codeType`, `codeBuiltin`, `codePunct`) built from the same base colours as everything else, so every existing scheme got it for free.

Punctuation is two roles rather than one. `faint` is the right weight for a horizontal rule — felt rather than read — and the wrong one for a brace: in Solarized Dark it is `#073642` on a `#002b36` ground, a contrast ratio of **1.15:1**, which is a brace you have to hunt for. Both are mixed towards the foreground (`codePunct` 0.6, `codeOp` 0.85), operators further because `:=` is something a reader is *reading* while `{` is something they are only locating. The blend is computed from each scheme's own two colours rather than picked by hand eleven times, so a scheme added later gets it right without anybody thinking about it.

It is a scanner, not a parser, exactly like the org highlighter it lives beside. It does not know scope, cannot tell a division from a regex, and will call a variable named `class` a keyword — each of which is one word the wrong colour, the failure a highlighter is allowed. What it must never do is lose a character or let a string run away with the rest of the file, and `codehl.test.ts` pins both: every span concatenated has to be the line it was given.

Two things about how it plugs in:

1. **The state is carried per line by `walk`, not worked out while painting.** The source view paints a screenful at a time and in fold order, so a scanner run over what happens to be on screen would start a python file in the middle of a docstring and colour the rest of it as prose. `LineState` carries the block's language and where the scanner had got to.
2. **Only `src` blocks in a language with a spec are coloured.** An `example`, an `export`, or a language nobody wrote a spec for stays one plain span — half-colouring by guesswork reads worse than not colouring at all. `isKnownLang` is the gate, and the fallback spec claims no keywords for the same reason.

### The command palette

⌘K / ctrl-K (or ctrl-shift-P, or the ⌘ on the rail) opens `worg/src/components/Palette.tsx`: one box that goes anywhere and does the handful of things that are never about the page you are on. It offers the panels, the capture templates, every org file, the saved searches, the kanban boards, the collections, a few global toggles - and **headings, asked for as you type**, which is the one thing in it that cannot be a list held in the browser.

Four things carry it:

1. **The matcher is `worg/src/fuzzy.ts`, which is `internal/common/dnd/fuzzy.go` said again in typescript** - the same matcher the terminal chooser and the character sheet's own palette use. It is written out again rather than asked for because a palette runs it on every keystroke, and a round trip per keystroke is the delay a palette exists to remove. Change one and change both. A term matches a command's **name** loosely (`mm` finds Mind Map) and the line under it only on **whole words**, because fuzzy matching over a sentence matches nearly everything.
2. **What you pick is remembered** (`noteUse`, localStorage), and with nothing typed the recent ones come first under their own band. That is the whole difference between a palette people open and one they forget. Recency is a *nudge* once something is typed - capped, so it breaks ties between equally good matches and never lifts a worse one over a better; `palette.test.ts` pins that.
3. **A panel's other words live in `PANELS`** in `palette.ts` - "spreadsheet" finds Tables, "calendar" finds the Agenda - so the palette is searched the way somebody thinks of a thing rather than the way the sidebar labels it. It says again what `AppBar.tsx` draws, deliberately: one is a place on screen, the other is a name you type at.
4. **Headings are a query, debounced, each request aborting the one before it**, and `headlineQuery` makes typed text safe to be a regular expression *and* case insensitive (`(?i)`). Without that, "jane" never finds "Jane Roe" - which is most of what typing at a palette is for. Headings are also the one kind of command never written to the recents, because the id would name a row that has since moved or gone.

The palette does nothing to the panels directly: it is handed a `PaletteActions` from `App`, which is where switching panels, opening the capture dialog and putting a jump down all live. A board and a collection are opened by **writing the browser setting the panel reads on the way in** (`kanbanBoard`, `collection`) rather than by adding a second way in.

### Sending the reader from one tab to another

`worg/src/NavContext.tsx` is how a panel puts the reader somewhere in another panel — the search tab's jump button opens the file in the Files tab at that heading's line. It follows `ChromeContext`'s shape and exists for the same reason: the panels are siblings routed by `App`, so neither can call the other without an import cycle.

A jump is **left waiting rather than delivered**. `toFile` puts it down and switches panels, and the Files tab — which that switch *mounts* — takes it on the way in and clears it. Delivering it the other way round would mean handing something to a component that does not exist yet.

Every jump carries **the way back**, because going to look at the file is nearly always something you do *while* doing something else: reading down a search, working through a list of tables. The jump names the panel to return to, what the button should say, and an opaque `where` that only the panel which wrote it reads - `NavContext` has no business knowing that a search has pages or that the Tables tab has a selection. `goBack` puts the `where` down and switches back, exactly the outward trip run backwards, and the Files tab draws the button (twice: in the toolbar, and floating over the page when the toolbar has been slid away, which is precisely when a full-page read of somebody else's file wants it).

Two rules there, both learnt the hard way:

1. **The restore is taken at once and acted on later.** The panel being returned to is mounted by the switch that set the restore, so its own list has not arrived yet and there is nothing to select. Taking it on arrival and holding it in local state until the row, block or table it names turns up is what works; consuming it on the first pass leaves the reader looking at "pick something" - and leaving it down instead would leave it lying about for another panel to find.
2. **Leaving the Files tab by the sidebar drops it.** A button offering to return you to a search you walked away from twenty minutes ago is a button that lies.

The search tab's rows carry `data-row-hash` for this: a React key is React's handle on a row, not the document's, and coming back has to find it again and give it a moment of colour - one row in twenty-five needs saying which, not just showing.

The two views land differently because they have to: the exported page has no line numbers, so it scrolls to the heading by its text (`pendingHeading`), while the source view scrolls to the line (`OrgSource`'s `scrollToLine`, which wins over `scrollTo` because two headings in one file can read the same). Both flash what they landed on.

Which view the Files tab reads in is a **browser setting** (`fileView`) rather than component state. It always said it kept whatever view you were reading in; as state it lost that the moment you looked at another tab and came back, which is exactly the trip a jump makes. A character sheet still overrides it and is never stored — "sheet" is not a way of reading every file.

### A file that is a D&D book

`#+LATEX_CLASS: dndbook` is somebody writing an adventure or a bestiary to be printed the way the books are. The file view already offers a character sheet for the files the server reads as characters; a book gets the same treatment and two more buttons: **Book**, which is the html exporter's own `dnd` theme, and **PDF**, which is the file run through pdflatex.

`GET /dnd/books` (`internal/app/orgs/dndbook.go`) is the book-shaped sibling of `/dnd/characters` - every watched file carrying that class. `GET /pdf` is the other half, and is an endpoint of its own rather than a `/file/{type}` because the answer is **bytes**: the pdf exporter writes a file and returns nothing to a string. Three things about it:

1. **It is cached against what the file was when it was built** - path, modification time and size - so opening a book twice runs pdflatex once, and editing the org file invalidates it on the next request because the key changes with it. `refresh=t` (which the file view's reload button sends, and only it) is the way past a cached failure.
2. **One build at a time per file.** Two tabs asking at once would otherwise have pdflatex writing the same output twice, and the loser of that race serves half a file.
3. **The client fetches it as a blob, not as an iframe src**, because the request has to carry the `Authorization` header and an iframe cannot be given one. The object url it makes is revoked when it is replaced - a look at a book would otherwise leak a pdf into the tab.

Getting the example file (`dnd_pdf_example.org`) to compile needed four fixes in the latex exporter, all of which were breaking every dndbook export, not just this one:

1. **`MakeTemplateRegistry`'s parameters were the wrong way round.** The one caller passes (class, default) and the signature said (default, class), so a dndbook document read `book_templates.yaml` *first* and its own file second. Nothing failed outright, because the generic file has a worse answer for everything the class defines - tables came out as plain `tabular`s in a class whose whole point is that they should not.
2. **A dndbook table is a `DndTable`.** The writer already built the column spec out of tabularx's `X` columns for this class, which mean nothing to a `tabular` - `\begin{tabular}{ XXX }` is an "Empty preamble" error. `dndbook_templates.yaml` now carries the `default` table template that wraps them in `DndTable`, and drops the `|---+---|` rule rather than turning it into an `\hline`, because that environment draws its own header.
3. **`\par` inside a macro argument ends the run.** The paragraph template writes `\par` before every paragraph and the writer has always had a `docclass != "dndbook"` guard saying this class should not get it - a guard that could never fire, because the template always won over the branch holding it. The class now has its own paragraph template, and `MONSTERTYPE`'s content is squeezed to one line (`oneLine`), because `\DndMonsterType` is not `\long`.
4. **pongo2 escapes for html unless told not to.** Every latex template is now rendered inside `{% autoescape off %}` (`OrgLatexWriter.render`). An apostrophe arriving as `&#39;` is not cosmetic: `&` is LaTeX's column separator, so a monster whose text mentions "the creature's turn" ends the run with "Misplaced alignment tab character &". Saying it once there beats `| safe` on every value of every template, which is what the dnd templates were quietly relying on somebody to remember.

### A file that says how it wants to be read

Three kinds of file ask for a view of their own, and the file view offers each as a button beside Rendered and Org: a **character sheet** (from `/dnd/characters`), a **D&D book** (`/dnd/books`), and a file naming its own html theme with `#+HTML_THEME:` (`/files/themes`, in `dndbook.go` beside the other two). A file that asks opens in what it asked for; anything else keeps whatever view you were already reading in.

The theme one exists because the Rendered button *cannot* show it. That view passes the reader's own html theme setting, which overrides the file's `#+HTML_THEME:` unless the setting happens to be "file" - so a documentation file written to be read in the `docs` theme was only ever shown in it by accident. The button is labelled with the theme's own name (`themeLabel`: "docs" → Docs), asks for that theme **by name** rather than by sending an empty theme, and the ground behind the frame follows the file's theme rather than the reader's - a stylesheet that never names a background leaves the page transparent, and the wrong ground shows through.

None of the four owned views (`sheet`, `book`, `pdf`, `theme`) is ever written to the `fileView` setting: none of them is a way of reading *every* file.

The theme a file names is reported **as the file wrote it**, whether or not this server has a stylesheet by that name - `/html/themes` is the list of the ones it has, and answering with only those would swallow a typo silently.

### The three documentation themes

`docs`, `rtd` and `furo` are **one application in three dresses**. The
application is `templates/html_docs_app.js`, included by all three
templates with `{% include %}` rather than copied into each of them - the
contents rail, the fuzzy search with its arrow-key preview, the folding,
the anchors, the code copy and the scroll spy are the same code on all
three pages. A theme is a **shell** (`html_<name>.tpl`) and a
**stylesheet** (`html_styles/<name>_style.css`), and nothing else:

- **`docs`** - a masthead across the top, a contents rail down the left,
  the document in one column. Written first; the other two are drawings
  of it.
- **`rtd`** - sphinx_rtd_theme: a dark rail the full height of the window
  with a blue search block at the top of it, breadcrumbs over the page,
  the document on white in an 800px column.
- **`furo`** - the Furo theme, which is what diataxis.fr is read in:
  three columns, almost no rules, one blue. Contents left, document
  middle at a 46rem measure, "On this page" right.

Nothing in the exporter knows there are three. `HtmlThemes()` lists every
`*_style.css` it finds and `GetTemplate` picks up `html_<theme>.tpl` when
one exists, so a fourth is two files and no Go.

**The script asks the page for ids, never for a layout**, which is the
whole of what makes one application wear three shells. A shell must
provide `#sidebar`, `#navbar`, `#searchfield`, `#searchresults`,
`#searchform`, `#rail-expand`, `#rail-collapse`, `#rail-toggle`,
`#theme-toggle`, a `.doc-body` around the document, and
`[data-sticky-header]` on whatever stands over the top of the page. Two
things in the script used to be the docs theme's own and are now asked
rather than assumed, because the three disagree about both:

1. **`headerHeight()` measures `[data-sticky-header]`**, and answers zero
   when there is none. It named `.header-container` and defaulted to 52px
   - which is right for a masthead on every width and wrong for a bar that
   exists only on a narrow screen, where it would have left 52px of dead
   space above every jump on the desktop.
2. **`closeRail()` asks whether `#rail-toggle` is on screen** rather than
   testing `innerWidth <= 950`. The toggle is drawn only where the rail is
   over the page rather than beside it, so that one media query in each
   stylesheet is the whole definition of "narrow" for that theme - 950px
   for docs, 768 for rtd, 820 for furo.

And two things it now says out loud for a shell to use, both no-ops in a
theme that styles nothing for them: **`docs:current`**, a `CustomEvent`
carrying the heading the spy has settled on - rtd's last breadcrumb and
furo's right hand column are both drawn from it, rather than either
watching the scroll a second time and disagreeing at the edges - and
**`.is-branch`** on the rail rows from the root down to that heading,
which is what rtd's open-section band is drawn from.

Three decisions the two new themes share:

- **The light/dark key is `docsTheme` in all three.** They are the same
  documentation read three ways; choosing dark in one and finding light in
  another reads as a bug rather than as a separate setting.
- **`{{fontfamily}}` is the display face only** - the brand and the
  headings - and the prose is the reader's own UI stack. It is a *server*
  setting (`exporters: - name: html / props: fontfamily:`), so putting it
  in front of the one stack Furo sets everything in meant every Furo page
  came out in whatever that happened to say, which is the opposite of
  asking for the Furo theme. A file's own `#+HTML_FONTFAMILY:` still wins,
  in the place a theme has for a face of its own.
- **The rail is a pinned head and a scrolling body**, not one box that
  scrolls. `overflow-y: auto` forces the other axis to `auto` too, so a
  results panel wider than the rail is clipped at its edge however it is
  positioned - and a result is a heading, the outline path above it and a
  line of the prose it was found in, which is three ellipses in 300px.

`#+SUBTITLE:` is read by the exporter for these: rtd puts it where that
theme puts a version, furo under the project name. A file that names none
writes no line anywhere.

Offering a theme means naming it, and worg named it by capitalising it -
which gets "Rtd". `htmlThemeLabel` in **`worg/src/settings.ts`** is the
label table plus that fallback, and it is there rather than beside either
caller because there are two of them: the settings menu, where a theme is
being chosen, and the file view's button for the theme a file asked for.
The same theme reading two ways in one application is two themes as far as
the reader is concerned. A stylesheet added to the server still needs no
change there - a name goes in the table only when capitalising it gets it
wrong, or when the name alone does not say what the page will look like.

While building them: **`#+TITLE:` had never reached the html template.**
`ExportToString` wrote it into the caller's `props` map and the template
is rendered from `self.Props`, so every exported page was titled
"Schedule" - `ValidateMap`'s default, and a leftover from the agenda. It
is now set on `self.Props` on every export (the exporter is shared between
requests, so a title set only when a file names one would keep the last
page's), and a file naming no title is named **after itself** rather than
after the agenda. That matters more here than elsewhere: in these two
themes the title is the name at the head of the rail.

#### The `docs` theme

`#+HTML_THEME: docs` is the one theme that was an *application* rather than
a stylesheet first:
`templates/html_docs.tpl` plus `templates/html_styles/docs_style.css`.
It is a masthead, a contents rail built from the heading tree the exporter
already walks out of the document (`nodes_json`), and the document itself in
one column. The page carries no jQuery - it had been carrying it for the tree
and the search box and nothing else.

What it is built on is `OrgHtmlWriter.WriteHeadline`'s own ids: every heading
is a `heading-wrapper` with a `-title`, a `-content` and a `-text` inside it,
and `nodes_json` names them. Six things are load-bearing:

1. **Clicking a heading in the rail goes to the heading.** The old tree
   *hid every other heading* and showed that one, which is why it read as the
   link being broken - nothing moved, and the page you were looking at
   vanished. Now the whole document is on the page and the rail scrolls to it,
   unfolding whatever it is inside on the way.
2. **A heading is addressed by a slug, not by its uuid.** `WriteHeadline`
   makes a fresh `uuid.New()` per heading per export, so a link copied out of
   the page was dead the next time the docs were built. The slug is made from
   the heading's own name at load, deduplicated, and put on the title element,
   which is what the anchor button copies and what `location.hash` is read
   against.
3. **Everything is reached by `getElementById`, never by a selector.** Those
   uuids start with a digit about half the time, and `#1bfeb…` is not a valid
   CSS id selector - so anything written as `querySelector('#' + id)` works
   for one heading and throws for the next.
4. **The search is fuzzy, and it is `internal/common/dnd/fuzzy.go` said a
   third time** - `worg/src/fuzzy.ts` says it a second. A term matches a
   heading's own name loosely and the prose only on a whole word, which is
   what keeps a filter from matching half the document. Change one and change
   all three. What the old box did was `new RegExp(what you typed)` against
   each heading's `innerHTML`, which found nothing on a half-typed pattern and
   put `<mark>` inside tags when it did find something.
5. **Marking a hit walks text nodes**, for that reason: a replace over markup
   lands inside a tag as readily as inside a sentence.
6. **A long jump does not animate.** The page is a hundred thousand pixels
   tall and `scroll-behavior: smooth` across all of it is a second of blur
   that says nothing about where you landed. Anything past three screens is a
   cut.
7. **Arrowing through the hits takes the page to each one, and that is a
   *look* rather than a move.** A one-line snippet cannot say whether a hit
   is the one you meant, so the preview is the whole of what the search is
   for; what makes the arrow keys safe to press is that Escape puts back
   everything the look changed - where the page was, which sections were
   unfolded on the way in, which row the rail called current - and Enter
   keeps it. Four things follow. A preview **cuts rather than animates**
   whatever the distance, because arrow keys repeat and a queued smooth
   scroll arrives after the key that asked for it. The **first** arrow press
   shows what is already selected instead of stepping past it, or the top
   hit - the one the search thinks you meant - is the one result you can
   never look at first. **Editing the query ends the look**, since the result
   being previewed is about to stop existing. And the heading is dropped
   **below the results panel** when the two actually overlap, which is only
   on a narrow screen: elsewhere the panel is over the margin and the preview
   lands under the masthead like any other jump.

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

The stylesheet is read by `GetStylesheet`, which rewrites every `url(...)` in
it to point at `http://localhost:8010/` - so **docs_style.css must contain no
`url()`**; the disclosure chevrons and the fold markers are drawn from borders
for that reason as much as for the look of them. `{{fontfamily}}` is
substituted by the same pass and is the display face: the masthead and the
first two heading levels. The prose is set in the reader's own UI stack, and
the palette is custom properties defined three times - bare `:root` for light,
`:root:not([data-theme="light"])` under `prefers-color-scheme: dark`, and
`:root[data-theme="dark"]` so the masthead's switch wins in both directions.

### Searching the text of every file

The files tab has two boxes. The first filters by what files are *called*; the second (`/files/search`, `internal/app/orgs/filesearch.go`) is a regular expression over what is *in* them, and turns the panel into the list of lines that matched - emacs' swiper, over the whole database. The arrow keys walk it from the box, Enter and a click do the same thing, and moving **opens** rather than merely highlighting, because reading a hit list is looking at the lines one after another.

It is not `/search`, which queries the parsed database and understands headings, keywords and dates. This reads lines and understands nothing, which is exactly what finds a phrase written in a drawer, a table or a source block. It is not `/grep` either, which has always done something close to this but answers with `"file:12:text"` strings - which cannot be taken apart again when the line holds a colon, and say nothing about what was left out.

Five things it has to get right:

1. **A half-typed pattern is a state of the box, not a failed request.** `Ok: false` with the reason comes back `200`, and the panel says "Not a pattern yet — …" while keeping what it was showing.
2. **The cap is on the lines kept, never on the count.** `.` matches every line of every file, and somebody types `.` on the way to something; each file reports how many lines matched even when only the first forty came back (`Truncated`).
3. **The scanner's buffer is raised to 4MB.** One pasted image or minified blob is a line past `bufio`'s 64k default, and that stops the scan dead - silently losing every match after it in that file. `TestSearchPastAVeryLongLine` pins it.
4. **A very long line is sent as a window around the match** (`trimAround`), with the offsets moved to match and an ellipsis to say it was cut - and the match itself is never cut in half.
5. **The offsets come from the server**, so the client picks the match out of the line without running the pattern again: Go's regular expressions and the browser's do not have to agree, and they differ over exactly the things people reach for in a search box.

A hit lands in the **source** view, whatever the file would otherwise open as - a line number means nothing in a rendered page.

### The Tables tab's jump

The open table's header carries the same jump the search tab's rows do - the file, at the table's own line, in the Files tab - and the same way back. A table has a `Line` already (it is how the list says `code.org:19`), so there was nothing to add on the server for it.

### The search tab's file affordances

Two browser settings, both about reading rather than about what was found. `searchShowFile` puts the file and line in small muted type under each headline — outside the hover tooltip, so running the pointer along a path does not open the heading preview. `searchGroupByFile` gathers the results under a band per file, with a fold and a count.

Grouping is a **sort plus header rows in the same table**, not a different structure: the rows are the same rows and the columns still line up, so sorting, selecting and editing in place keep working with no second code path. The bands take a slot in the page the way a row does — paging over rows and drawing bands around them afterwards is the obvious alternative and is wrong, because a folded file has no rows on any page and its band would have nowhere to be drawn.

While in there: the Headline column's sort arrow was sorting on `id`, which a search row does not have, so every comparison came out equal and `stableSort` kept the order it was given — the arrow turned round and nothing moved. It sorts on `Headline` now.

### Quick capture from worg

`c` anywhere in worg, or the pencil on the rail, opens the capture dialog (`worg/src/components/Capture.tsx`). It asks `/capture/templates` which templates exist, puts one up as a form, and posts to `/capture` - the same two endpoints `orgs cap` uses from the terminal.

The thing to understand before changing it is **where the template language lives**, and it is now in three places that share one definition. **`internal/common/captemplate.go` is the grammar** - the pattern, the self-answering names, `CapFields`, `CapSubstitute`, `FillCapTemplate` - and it is in `internal/common` precisely so the end that expands a template and the end that fills it in cannot have private understandings of the same string. `internal/app/orgs/capturefill.go` is the server's wiring, `worg/src/capture.ts` the browser's, `cmd/oc/commands/capture/` the terminal's.

Four spellings, the last two written by the *server* rather than by whoever wrote the template:

```
{{name}}                   a value to fill in, labelled from the name
{{name|prompt}}            the same, asked for in your own words
{{name|=default}}          pre-filled with a default, still editable
{{name|prompt|=default}}   both
{{CONTENT}}                the body
```

**A name that answers itself is answered on the way out of `/capture/templates`** - `{{uuid}}`/`{{guid}}`, `{{username}}`, `{{now}}`, `{{today}}`, `{{tomorrow}}`, `{{week}}`, `{{hostname}}` and the rest of `CapAutoValue` - and the answer goes in as that placeholder's **default, not over the top of it**: `{{uuid}}` comes back as `{{uuid|=f81d4fae-…}}`. That is how a capture gets a `:CUSTOM_ID:` with a GUID in it - `{{uuid}}` and nothing else - and both halves of the shape are load-bearing. A client that knows nothing about it has a usable value in the box already, which is the whole reason to do it server side at all (`crypto.randomUUID` is secure-context only in a browser - the same wall `clienthash.ts` runs into - so for worg over plain http from another machine this is the *only* end that can answer it). And a client that would rather answer the name itself still sees the name, which a substitution would have taken away: a literal value is a value nobody can change, and the timestamp on a capture is very often the one thing somebody does change.

The `|=` marker is why it is a marker and not just the prompt slot. `{{source|Where from?}}` is a question and `{{uuid|=f81d4fae-…}}` is an answer, and a client has to tell them apart **without keeping its own copy of the list of names** - which is exactly the second copy that goes stale (ClientHash, FuzzyScore, HealthLevel all say the same thing). Five rules, pinned by `captemplate_test.go` and `capture.test.ts` on the two sides:

- **A name with no answer is left standing, braces and all.** A client's reading of a template is unchanged by the pass: it sees fewer holes, never different ones, so a server and a worg of different ages still work together.
- **The expansion is idempotent.** A placeholder that already carries a default is left alone, so a template cannot collect a second answer.
- **One value per name per template**, aliases counted as one name (`CapCanonName`), so `{{uuid}}` twice is one uuid and a `:CUSTOM_ID:` plus an `id:` link to it agree. Two templates in one answer get two uuids, because they are two captures.
- **A value that could not be written back out is not written.** A default holding `}` or `|` would be a placeholder that no longer parses, so the name is left bare and the client asks.
- **They are answered when the list is asked for, not when the capture is made.** worg re-asks on every open of its dialog and `orgs cap` asks once per run, so a uuid is fresh per capture; a client that keeps one list all day and captures twice offers the same uuid twice. `/ext/capture/templates`, the *editing* endpoint for a user's own templates, is deliberately **not** expanded - it hands back the template as written, or saving one back would bake this afternoon into it for ever.

Four things about the rest of it:

1. **A yaml template longer than one line needs a block scalar** (`template: |-`). A plain multi-line scalar folds its newlines into spaces, which turns a property drawer into one long line. The shipped example used to get this wrong.
2. **A line whose only content was an unanswered placeholder is dropped**, so a template offering three optional properties writes the one that was answered rather than one and two empty ones - the same rule the record editor follows.
3. **A capture has no heading yet**, which is why dictating and pasting a picture need `internal/app/orgs/capturestash.go`. The kanban card can keep a picture *and* link it in one call because it has a heading; a capture being composed has to keep the file first and carry the link in the text it is still typing. Both need to know which file the link will be relative to, and the only thing the dialog knows is the template - hence `stash=1&template=NAME` on `/image/paste`, and `/voice/link`.
4. **The mic writes into the box the cursor was in**, which it works out on mouse down by reading `document.activeElement` back to a `data-cap-key` — not from a focus handler it kept. Pressing a button *moves* the focus onto the button, so asking afterwards is too late and asking a remembered state is asking something that may never have been told. The tooltip names the box it is about to write into, so there is no guessing before you speak.
5. **The preview is `OrgSource` with `openDrawers`**. The file view starts drawers shut because a file of headings is mostly drawer; a four-line preview starts them open, because what is *in* the drawer is exactly what somebody is checking before they press Capture.

`InsertEntryUsingTemplate` used to write the content as one concatenation - indent, the whole string, a newline - which indented only its first line. Every line is indented now, because anything a template produces is several lines and a `:PROPERTIES:` drawer landing at column zero is the go-org trap that hoists the rest of the heading to the top of the document. It writes `NewNode.Tags` as well, which the wire type has always carried and nothing wrote.

### `orgs cap`, the template as the form

`cmd/oc/commands/capture/` used to ask two unlabelled questions - a headline, then a body - whatever the template said, so a template with a property drawer in it worked in worg and did nothing at a prompt. Now the template **is** the form (`form.go`): the entry is drawn as org, coloured as org, with the cursor in whichever hole is being filled in and tab moving to the next. That is org-capture's own idea - what you are editing is a document, not a dialog.

Four things in it are load-bearing:

1. **A hole nobody has filled in is still drawn as a hole** - `{{project}}`, in the colour `orghl` gives a placeholder - rather than as an empty space. An empty space says "this line is finished"; the braces say "this line is waiting", which is true, and is what the same line looks like in an Emacs capture buffer.
2. **Nothing is dropped while it is being edited.** `FillCapTemplate` drops a line whose only content was an unanswered placeholder, which is right when filing and disastrous while typing - the line you are editing would vanish the moment you cleared it. The editor substitutes everything and keeps every line (`CapSubstitute`, which answers "leave it as written" where `FillCapTemplate` answers "write nothing"); the dropping happens once, on the way out.
3. **The preview is the entry as it will be filed**, down to the stars and the indent, because `InsertEntryUsingTemplate` writes the headline at the target's level plus one and indents every line of the content under it. A preview of something else is worse than no preview.
4. **The caret is a span, not a character.** It is pushed into the text as a sentinel, the text is coloured, and the span holding the sentinel is split so the character under the cursor comes out in reverse video *inside* the coloured org. Colouring around a caret instead loses the colour at the join.

The headline and the tags are holes in the same list as the template's own, because somebody filling this in is making one entry and does not care which half of it the template owns. Non-interactively - a pipe, `-json`, a cron job - there is no form: the defaults stand, `-head`, `-cont`, `-tags` and `-set name=value` answer what they answer, and a missing headline on an `entry` is a refusal rather than a prompt nobody can see. `-dry-run` works because every write goes through `SendReceivePost`.

### `orghl`, org colouring for a terminal

`cmd/oc/commands/orghl/` is `worg/src/codehl.ts`'s sibling for the other kind of screen, and a **scanner** for the same reasons: it does not know scope, it cannot tell a multiplication from a pair of bold markers, and it will occasionally give one word the wrong colour - the failure a highlighter is allowed. What it must never do is lose a character, and `TestSpansAreTheLine` pins exactly that: every span of a line, concatenated, is the line.

- It answers in **spans**, not escape codes, so the same walk draws two ways (`ANSI` for a listing, `Tview` for a form) and a caller can do something else with a span entirely - which is what the capture form does to put a cursor inside one.
- **State is carried per line** (`State`), because a source block and a drawer change what the lines inside them mean. Reading a screenful out of the middle of a file without it starts a block in the wrong place.
- **The keywords come from the server** (`GET /status`), so a headline typed `TODO buy milk` is coloured as a task and a keyword this server does not have is just words. A server that will not answer is not a reason to refuse to capture, so that read is allowed to fail silently.
- **Escaping for tview is the part that matters more than the colours.** Org text is full of `[[links]]` and `[2026-09-30 Wed]`, and every one of those is a colour tag to a TextView with dynamic colours on - an unescaped timestamp does not come out the wrong colour, it comes out *missing*.

### Pictures: pasting one, and filing a chart

`POST /image/paste` keeps a picture in `images/` under the first org directory and appends a link to it. It is told where in one of two ways:

- **a heading hash** - a picture pasted onto the back of a kanban card, which then turns up on the front, because the front already draws the first picture the heading points at;
- **a filename and an `afterLine`** - the Tables view filing a chart, which belongs under the table it is of rather than at the end of whatever heading the table happens to sit in.

`afterLine` is a **zero-based index**, which is what a table's `EndLine` is - go-org counts rows from zero. Inserting *at* that index puts the picture inside the table, one row up from the bottom.

Nothing is converted: the bytes go down as they arrive, so the extension follows what was actually pasted. The link is written relative to the org file that holds it, so moving the org directory keeps every picture - the same rule a voice note's audio follows.

Two things in the html exporter were wrong and are fixed here, both surfaced by this:

1. `WriteRegularLink` chopped a fixed seven characters off `l.URL` assuming `file://`. Org writes `file:path` too - and that is what a link somebody typed looks like - so `file:images/x.png` became `ages/x.png`. It reads the target properly now.
2. The default branch built `http://localhost:<port>/images/...`. That works on the machine running the server and nowhere else, which now matters: there is a QR code for opening worg on a phone. A page this server rendered and is about to serve gets a path of its own. The `filelinks;`, `httpslinks;` and `httplinks;` opts are unchanged - they are for a file on disk and for vscode.

### Speaking into a heading that already exists

The mic at the foot of a kanban card's back records, transcribes and appends - `POST /voice/append` (`internal/app/orgs/voiceappend.go`). It is the sibling of `/voice/note`, which files a recording as a **new heading** under a target; here the heading is already there and is what the card is about, so the words go into its own body and the audio is linked from its own `:AUDIO:`.

The order is the voice notes' order and for the same reason: `/voice/recording` saves the audio the moment recording stops, **before** anything is attempted on it, and the text written is the text sent rather than a fresh transcription. A transcription that fails costs the transcription, not the words - and when whisper says nothing at all the recording is still filed and still linked, with the card saying so.

Two things about the write:

1. It is a **line splice**, like the voice notes and the checklist. Appending a paragraph to one heading should not reformat every drawer and table in the file it happens to live in.
2. A heading that already has an `:AUDIO:` **keeps it**, and the second recording's link goes in the body beside its own words. The property holds one link; overwriting it would leave the first recording on disk with nothing pointing at it, and losing a link to a recording quietly is worse than having them in two places.

### Reaching the server from a phone

`GET /addresses` answers with every url this machine can be reached on, private addresses first. It exists because the page cannot work it out: the browser's bar almost always says `localhost`, and a phone pointed at `localhost` reaches itself. worg's sidebar foot turns the answer into a QR code.

### Charting a spreadsheet selection

worg's Tables tab charts the numbers in a selection (ctrl-g, or the chart button). It used to be one hand-drawn svg line chart; there are eighteen types now - lines, bars, parts of a whole, comparisons - so **echarts** draws them and `worg/src/chartspec.ts` decides what to draw. That module is pure and tested, because a chart drawn from the wrong axis still looks like a chart: the failure is a picture that reads perfectly and says something untrue. echarts is imported when the popup first opens rather than with the app - it is a megabyte and most sessions never chart anything.

Three rules are pinned by `chartspec.test.ts`:

1. **A gap is never a zero.** A missing reading is `null` all the way through, and lines break rather than being drawn across. Radar is the one exception - it has no notion of a missing point - and that is why it is the only place a zero stands in.
2. **A type that cannot show several series folds them** (`foldToTotals`) rather than drawing the first and dropping the rest silently. Pie, donut, rose and funnel say so in the subtitle when they do it.
3. **Horizontal bars swap the axes, not the data**, so the tooltip and the legend say the same thing whichever way round the chart is.

### The four presentation exporters

`revealjs`, `impressjs`, `webslides` and `deckjs` are four drawings of one thing: an org file is a deck, a headline is a slide, a property says how that slide behaves. They disagree about the html and about almost nothing else, so everything up to the html is **`internal/app/orgs/plugs/slides`** - which headlines are slides, where the speaker notes are, what a background means, how a file link becomes a url. worg's Presentations tab offers all four over `/file/{exporter}`.

The rule that buys: **a property means the same thing in every framework.** `:BACKGROUND: blue` is a blue slide in all four; `:REVEAL_BACKGROUND:` is a blue slide in reveal only; `#+SLIDE_LEVEL:` works everywhere. Nothing has to be written four times to work four times. `Conf` is that ladder - the slide's own prefixed property, its neutral one, then the document's - and `captemplate`-style drift is avoided by there being one implementation.

**The thing to know before changing any of it: a parsed org document is not reliably a tree.** go-org ends a headline's body at the first drawer or block written in column zero and hoists the rest to the top level, and Emacs writes property drawers in column zero - so for any file that configures a slide, `Headline.Children` is empty and `Headline.Properties` is nil, and some headlines in the same file nest while others do not. `BuildDeck` therefore walks the nodes **in order** and attributes each one to the last headline that started above it, picking up a hoisted property drawer as the properties of the headline it follows - the same rule `links.go` and `code.go` arrived at. Two bugs that had been live for as long as the exporters existed fall out of fixing it: a slide with a property drawer **lost its entire body**, and **no per-slide property was ever read** (`:REVEAL_TRANSITION:` silently did nothing on exactly the slides that asked for it).

Five more things are load-bearing:

1. **Speaker notes are taken out of the body, not styled out of it.** A `:NOTES:` drawer, a `#+BEGIN_NOTES` block and a child heading called Notes all mean the same thing, because people write all three. A deck that put the speaker's crib sheet on the screen behind them is the worst failure this code has available to it, so notes are moved to `Slide.Notes` by the builder rather than left in the body for a writer to skip.
2. **An excluded heading takes its hoisted body with it.** `:noexport:` is checked on the headline, but by then the body is no longer inside it - the builder keeps skipping until a heading at that level or above.
3. **Fragments are a property of the slide, plus `#+ATTR_SLIDE: :frag`.** go-org parses exactly three affiliated keywords into a node's metadata, so `#+ATTR_REVEAL:` arrives as a plain `Keyword` node sitting immediately before the thing it was written above; the writers pick it up in `WriteKeyword`, hold it, and spend it on the next element (`slides.Pending`). The class differs per framework - reveal's `fragment`, deck.js' nested `slide`, impress's `substep` - and `FragList` rebuilds the list so the class lands on the `<li>` itself, because every one of these libraries hides the element it is given and hiding a span inside a bullet leaves the bullet behind.
4. **Only what the file asks for is written into the javascript config** (`slides.JSOpts`). These libraries have good defaults and change them between versions; writing out all forty pins the exporter to the version it was written against.
5. **`plugs.MediaURL` is the one copy of "where is that picture".** The html exporter had the careful version and the two presentation exporters each had a worse one that assumed the link was relative to the first org directory and that the reader was on the same machine as the server - so every image in a deck under `notes/talks/` pointed at the wrong place, and none of them worked from a phone.

**Themes are one file for four frameworks** (`internal/app/orgs/plugs/slides/theme.go`, `templates/slides_theme_*.css`). reveal ships twelve themes, deck.js three, WebSlides and impress none, all written against their own class names - so "make my deck look like this" was four jobs and three of the answers did not exist. A theme is now two files loaded together: `slides_theme_base.css`, the **mapping**, which says what a heading and a table and a block of code are in each of the four and sets them from a dozen custom properties; and `slides_theme_<name>.css`, which is those properties. A new theme is one file of colours and fonts that works everywhere, and a framework growing a new surface is fixed once.

Four things about it:

- **The mapping loads only when a theme is in use.** Its rules are written to win over the library's own, and on top of reveal's `league` it would be a third thing fighting for the same headings.
- **The `.org-*` rules are qualified by framework** (`.reveal .org-subtitle, #webslides .org-subtitle, …`). They compete with classes the libraries define themselves - WebSlides' `.text-landing` sets a title size and `.text-subtitle` forces uppercase - and an unqualified rule loses, so a theme's type scale was silently ignored in one framework out of four.
- **A theme names its own highlight.js style and mermaid theme** (`@hljs:` / `@mermaid:` in its comment header, parsed by `LoadTheme`), because a theme that has chosen its greys cannot have somebody else's monokai dropped into the middle of it, and a mermaid diagram defaults to a white card, which on a dark deck is a hole in the slide.
- **A slide's own `:BACKGROUND:` can contradict the theme's ink**, and that is the one combination that produces a slide nobody can read - `:BACKGROUND: #102030` under the black-on-white `mono` theme. `slides.ReadInk` measures the ground and the mapping flips the text (`org-on-dark`/`org-on-light`). It is *luminance*, not brightness: the eye is far more sensitive to green than to blue, so an average calls `#0000ff` light and `#00ff00` dark, which is backwards for exactly the saturated colours people reach for. A picture is unknowable and is left alone; `:BACKGROUND_INK: light` says so outright.

`SearchPath` is set from `templatePath:` at startup, because the one thing a theme must not depend on is which directory the server was started in - and `orgs serve` is normally started somewhere else entirely.

**Too much on a slide is shrunk to fit** (`templates/slides_autofit.js`, wrapped by `slides.Fit`). Every one of these frameworks simply does not draw the overflow - reveal clips it, deck.js hides it, impress runs it off the step, WebSlides lets it scroll where nobody will scroll - so the failure is silent and the deck looks finished until it is on a projector. Five rules, all of them learnt from getting it wrong first:

1. **It measures in rendered pixels, from two rectangles.** Everything sits inside one or two outer transforms (reveal scales its whole stage, impress its canvas), and mixing layout pixels with rendered ones is wrong by exactly that factor. Measuring from the wrapper's own top also means whatever sits above it is simply room the content does not have, with nothing to calculate.
2. **It refines.** The wrapper is laid out at `100% / scale` and scaled back down, so shrinking re-wraps the text to *fewer* lines and the slide gets shorter than the first measurement said - one pass always overshoots. Three passes converge to within a percent.
3. **The resize observer has a cooldown**, because fitting changes the size of the very element being observed: without it every fit schedules another and a slide re-lays itself out for as long as it is on screen.
4. **A background tab never gets an animation frame**, so every trigger has a timer behind it and a `visibilitychange` listener in front of it.
5. **It only shrinks, and it stops at 45%.** Scaling a thin slide up makes a deck with uneven type; past the floor the slide is not overfull, it is a document, and you want to notice that while writing rather than while presenting. `:FIT: grow`, `:FIT: 0.3` and `:FIT: nil` are the escapes. deck.js' own `scale` extension already does this, so there is usually nothing left for the fitter to do there; WebSlides needs `data-fit-box="viewport"` because its sections *grow* with their content and are never found to be overfull otherwise.

Per framework, the things that are its own:

- **reveal.js** - a nested headline is a vertical stack, and the stack wrapper now contains *only* sections (the parent's own heading used to go in the wrapper, where reveal renders it behind the slides). Plugins are script tags plus `plugins: [...]`: the `dependencies: [...]` list the template used was **removed in reveal 4.0**, so the speaker notes, the search and the zoom had quietly not been loading at all. `center: false`, `navigationMode: 'grid'` and `pdfMaxPagesPerSlide: 1` stay the defaults because the template used to hard-code them and a deck exported last week has to look the way it looked.
- **impress.js** - steps cannot nest (only the direct children of `#impress` are steps), so the tree is flattened; the old exporter wrote a step inside a step for any nested headline and impress.js ignored it, putting the content on the page where nobody could reach it. `#+IMPRESS_LAYOUT:` places the steps - line, grid, spiral, ring, zoom, random - because hand-placing twenty things in space is the reason people try impress once and go back to reveal. `random` is deterministic for the same reason the dnd builder's dice are: a layout that moved on every export could not be rehearsed. The old `:IMPRESS_X: .6` relative spelling still means six tenths of a screen along.
- **WebSlides** - a page rather than a slide engine: it scrolls, it is responsive, and the deck you present is the file you can send somebody. `:CLASS:` goes straight onto the section, which is how a deck reaches its forty components without this code naming them. It has no notes view, so the `n` panel is ours - and it asks `window.ws.currentSlide_` which slide it is on rather than looking for `.current`, because during a transition *both* slides carry that class and the one the DOM finds first is the one being left behind. WebSlides also **renames every section id** to `section-N`, so `:CUSTOM_ID:` does not survive there.
- **deck.js** - nesting *is* stepping, which is the one thing it has that the others do not: an outline maps onto it exactly. Centred content is wrapped in `<div class="vcenter">` rather than classed on the section, because all three themes position `.vcenter` absolutely and doing that to the section fights deck.js' own positioning and lands the slide off-screen. jQuery 3 is fine: deck.js only uses `.bind()`, which is deprecated rather than removed.

### Mind maps

worg's Mind Map tab draws a saved query's headings as a map, with three engines behind a dropdown: **mind-elixir** (the default - a map you can drag branches about in), **jsMind** (boxes and branches, easiest to read at a glance) and **mermaid** (a fixed drawing, the one that goes into a document). The engine is a browser setting (`mindEngine`), not a server one.

All three are handed one tree, built in `worg/src/mindmap.ts` from the query's own rows rather than from the server's mermaid source. That is the thing to keep: the mermaid `mindmap` exporter emits a label and an indent and nothing else - no keyword, no tags, no file, no hash - so a map built by parsing it can never be clicked back to the heading, coloured by keyword, or filtered, and the old tab's regex-scraping of the exporter's html was exactly that. Four rules live in that module:

1. A heading hangs off **the last heading above it with a smaller level**, whatever that level was. A query can find a level-3 heading whose level-2 parent does not match, and any other rule either orphans it or makes it a top-level branch.
2. **One file goes straight under the root; several files each get a branch of their own**, because a map whose branches come from three files and does not say so reads as one outline.
3. A node's **id is the heading's hash**, which is what lets a click in any of the three engines find its way back to the heading.
4. Every node knows **which branch off the root it hangs from** (`branch`), because the classic mind-map colouring is by branch rather than by depth - a branch and everything on it share a hue, which is what makes a wide map readable.

Nothing any engine does is written back to the org files. Dragging a node in mind-elixir rearranges the drawing, not the outline: this view is for reading a plan and moving it around to think, and a drag that silently refiled a heading is not what somebody rearranging a map expects.

### Audio in an exported page

A heading that names a recording in one of its properties gets a player in the html export (`WriteAudio` in `plugs/html/html.go`). `:AUDIO:` is the property orgs writes itself, from a voice note, but any property whose value points at a file with an audio extension gets one - so `:INTERVIEW: [[file:takes/mira.wav]]` works without the exporter knowing what an interview is. Three things about it:

1. **Nothing is fetched until it is asked for** (`preload="none"`). A file with forty voice notes in it should cost one page, not forty recordings.
2. **A link is written relative to the org file that holds it** - that is what makes moving the org directory keep every note's audio - while the file server is rooted at the *first org directory*, so `MediaSrc` puts the two back together, and falls back to resolving against the root because plenty of files are written that way. Outside the org directory there is nothing to serve, so it hands back a `file://` link rather than a url that would 404.
3. The url is a **path** (`/images/...`) rather than `http://localhost:port/...`, so the page works whatever host and port the server was reached on - the `filelinks;`/`httpslinks;`/`httplinks;` opts still produce the absolute forms for an exported file on disk and for a vscode webview. `WriteRegularLink` keeps its own older rule for images; the two are not shared on purpose, because changing how an image url is built would change every page anybody has already exported.

### Gantt charts

`internal/app/orgs/gantt.go` serves the two endpoints worg's Gantt tab needs that nothing else provided: **`GET /gantt/tasks`**, which answers with the schedule itself - every heading a query finds, its lane, its effort, what it comes after and the **hash to change it by** - and **`POST /gantt/add`**, which writes a new heading under a parent given by hash (`/capture` needs a template, and a chart knows its parent by hash). Wire types are in `internal/common/gantt.go`.

It restates none of the scheduling rules: which heading comes after which, what lane it is in and whose it is are read with the mermaid exporter's own exported helpers (`mermaid.After`, `GetSection`, `GetResource`), so the charts worg draws and the page `/file/mermaid` exports cannot drift apart.

A heading's **lane** (`GetSection`) is the first of these that says anything, and nesting alone is not one of them - being somebody's child does nothing on its own:

1. the heading's own `:SECTION:` property;
2. the `:SECTION:` of its parent, but *only* when that parent is tagged `:project:`;
3. its own `:ASSIGNED:` property.

Anything still without one is drawn in a lane the client calls `main` (`DEFAULT_SECTION` in worg's `gantt.ts`). The **resource** (`GetResource`) is a separate ladder - `:ASSIGNED:`, then `:RID:`, then `:RESOURCEID:`, falling back to the *headline* of a parent tagged `:project:` - which is why a task with no assignee of its own is coloured by its project when the chart is coloured by assignee. Dates are deliberately **not laid out** server side - a task says when it starts or what it comes after, and the client resolves the chain, the same division of labour the exporter already has with mermaid's renderer. A task the query did not find but something it did find comes after is included with `Implied: true`, so the chain reads end to end.

Everything a gantt client *changes* goes through endpoints that already existed (`/headline/change`, `/date/change`, `/property`, `/status/change`, `/delete`), which is why three bugs in those surfaced while building it and are fixed here:

1. `subtreeEndRow` in `refile.go` - `Headline.GetEnd()` under-reports for a heading whose body is only a planning line and a property drawer, because go-org keeps the drawer in `Headline.Properties` rather than among the body nodes it measures. `/delete` left the drawer behind, orphaned under the parent, and `/gantt/add` wrote new tasks *into* the last child, between its date and its drawer. Both now take the end of the subtree off the file's own lines - the next heading at the same level or above - and that only ever extends the range go-org gives, never shrinks it.
2. `SetProperty` in `todo.go` - writing a property with an **empty value now removes it** (and the drawer with it, when it was the last one) rather than leaving `:AFTER:` sitting there with nothing after it. Undo writes the old value back, and for a property that was not there before, the old value is nothing.
3. `noAuthUser` in `auth.go` - with `noAuth: true` the authenticate middleware was skipped entirely, so no username reached the per-user endpoints and stored queries, kanban boards and capture templates all answered 401. There is now a middleware either way, and with authentication off everything is done as `local`.

### Moving headings: refile, copy and archive

`POST /move` (`internal/app/orgs/move.go`, wire types in `internal/common/move.go`) is one endpoint for all three, taking a list of sources and an `Op`. The three are one operation with three endings: all of them find a heading, work out where it should go and write it there; a refile then deletes the original, a copy does not, and an archive decides the destination for itself out of org's rules rather than being told. `GET /refile/targets` is the structured sibling of `/refilefiles` - the same places, but as hash, outline path, level, keyword and tags rather than as `"file|H1|H2"` strings that cannot tell two headings called Notes apart. `POST /copy` is the single-heading form of the copy, beside the existing `/refile`.

**The whole batch is one request because a heading's hash is not stable across an edit to its file.** The hash is accumulated from the document name and the chain of headline titles the parser has walked, so moving one heading out of a file changes the hash of every heading after it. A client that collects twenty hashes off a search and posts twenty refiles gets the first one right and is then addressing headings that are not there - or, worse, headings that have since inherited those hashes. `Move` resolves **every** source to `(file, outline path, headline)` before it writes anything, then addresses each one by `file+olp`, which does survive the file being rewritten, and reloads every touched file between operations.

Three more things it decides:

1. **A source whose ancestor is in the same batch is skipped, not failed.** Moving the ancestor takes it along, and moving it as well would be moving it out of the thing that has just moved. The result says so per heading (`Skipped`), because a client has to be able to tell "it went with its parent" from "it could not be found".
2. **`Ok` is whether *everything* worked**, and the per-heading results are always present rather than only on failure. A batch that half worked is neither a success nor a failure and the honest summary says both halves.
3. **`Create` is off by default.** Creating a destination heading because somebody mistyped the one they meant is a worse outcome than being told it does not exist.

Building it surfaced three pre-existing bugs, all of which were corrupting files through the *old* `/refile` endpoint long before any of this existed. `internal/app/orgs/refile_test.go` pins all three:

1. **`formatHeading` wrote a subtree once per level.** It recursed over `sec.Children` on top of the recursion `WriteHeadline` already does (`WriteNodesLB(1, w, h.Children...)`, and a headline's children include the headlines nested in it). Refiling a heading with a child and a grandchild wrote the child twice and the grandchild three times. The recursion is gone.
2. **The delete was measured against the file as it was before the insert.** `DeleteTree` re-reads the file - which `InsertSection` has just rewritten - but took its rows and its level off the parse tree beforehand. Two things had moved: every row below the insertion point, and the source's own `Headline.Lvl`, because `formatHeadingAt` calls `fixUpLevel` on a `CopySection` that *shares* the `*org.Headline` (it has to - the headlines nested in `Headline.Children` are those same pointers, which is what makes renumbering the subtree for writing work at all). `subtreeEndRow` then walked forward to the next heading at the *destination's* depth and ran straight through the source's siblings. Refiling `Alpha` out of `Projects/Kitchen` up into `Inbox` deleted `Kitchen`, `Gamma` and `Gamma child` with it and reported success. `Refile` now reloads the source file and finds the heading again by its outline path before deleting it, which answers the rows and the level at once.
3. **Archive had never worked.** `FindArchiveTarget` built a target of type `"file+heading"`; `GetFromTarget` only knows `"file+headline"`. One word.

The client half is `worg/src/move.ts` (pure, tested), `MoveDialog.tsx` (one dialog for all three) and `FileMove.tsx` (the file view's outline picker), reachable from the search tab's row menu and bulk bar, the kanban card's back, and the files tab's toolbar.

### Documentation extraction (SDOC / EDOC)

Comment blocks fenced with `SDOC: <section>` and `EDOC` inside the Go sources are extracted by `cmd/docex` into Org documentation. When editing existing comments that contain these markers, preserve the markers and their section names — they are load-bearing for the doc build, not dead comments.

**Do not run `gofmt` on a file whose SDOC block sits directly above a declaration** (the `/dnd/*` and `/records/*` endpoint files are all like this). Go 1.19+ reformats doc comments: it re-indents the block, turns the `* Heading` line into a `- Heading` list item and reflows the org tables inside it, which silently mangles the extracted docs. Two of the breakages are load-bearing and invisible in the diff unless you look for them — `docex` matches both markers as whole lines:

- `/* SDOC: API` becomes `/*` on one line and `SDOC: API` on the next, and the block is **not extracted at all**;
- `EDOC */` becomes `EDOC` and `*/`, and the block **runs on** into whatever follows.

`grep -c '/\* SDOC' file.go` and `grep -c 'EDOC \*/' file.go` should be equal and should match the number of blocks. Building the extractor and diffing its output is the real check: `go build -o /tmp/docex ./cmd/docex && /tmp/docex -src . -out /tmp/docs.org`. A block separated from the next declaration by a blank line — the ones above `import` in `internal/common/dnd/` — is left alone. `go build ./...` and `go vet ./...` are the health checks here, not `gofmt`.
