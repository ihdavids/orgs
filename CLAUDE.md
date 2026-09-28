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

### The pickers

Five commands are an fzf list with a pane beside it: **`orgs code`**, **`orgs links`**, **`orgs tables`**, **`orgs rec`** and **`orgs contact`**, each bare (no subcommand). `orgs grep` was the first of these and hands its pane to `bat`, which knows how to colour a file and nothing about org. None of them is a file — a source block is a header line and variables and code, a link is a target and the paragraph it was written in, a table is a grid and its formulas, a record is a drawer of fields whose kinds were worked out for it — so the pane is drawn by this binary.

Which means the preview is **a second run of the same binary**: fzf's `--preview` shells out to `orgs <thing> preview …`. The machinery is `cmd/oc/commands/picker.go` — `Pick`, `SelfCommand`, `Shq`, `PaneWidth`, `OpenBox`/`BoxLine`/`CloseBox` — shared so they behave identically and a fix lands once. Four rules, each a bug before it was a rule:

1. **The child has to reach the same server.** It is a fresh process with a fresh config load, so `-config` and `-url` are passed through explicitly rather than left to defaults that resolve against a working directory fzf's child does not necessarily have.
2. **The command is a shell string**, so every path in it is single quoted (`Shq`). fzf quotes the `{1}`/`{2}` it substitutes, so those are left alone.
3. **The list is tab separated**, address fields first and hidden (`--with-nth`). `orgs grep` splits on `|` and a path with a pipe in it takes a line apart in the wrong place. What is *shown* is also what is *searched*, which is why the full path is in the display as well as in the address.
4. **The pane draws to whatever width it is given** (`FZF_PREVIEW_COLUMNS`, then `COLUMNS`, then 80). Its boxes are **open on the right** on purpose: a box that closes has to know the display width of every line in it, and one line with a tab or a wide character makes that a guess. A box with one ragged edge reads as a box; one with the wrong edge reads as a bug.

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

### Backlinks and the link graph

`internal/app/orgs/links.go` walks every `[[target][description]]` link out of every parsed file, resolves it against the database, and indexes it from both ends. It serves `/links` (backlinks for one file), `/links/graph` (the graph around a file, or the whole database, at file or heading granularity) and `/links/stats` (per-file counts). Wire types are in `internal/common/links.go`; the worg client is `components/Files.tsx` plus the plain-svg force layout in `components/LinkGraph.tsx`.

The index is cached against `OrgDb.ReloadIndex` and rebuilt whenever a file reloads. Two things about the rest of the system shape it:

1. Sections are registered into `ByHash` / `ById` / `ByCustomId` lazily, as queries walk them (`ScanNode`'s `RegisterSection` call is commented out), so `buildLinkIndex` walks every file into the registry in a first pass before resolving anything.
2. go-org ends a headline's body at a drawer written in column zero, which leaves `Headline.Properties` nil and drops the rest of that heading into `Document.Nodes` at the top level. So links are attributed to the last heading starting above them rather than trusting the outline alone, and ids are read from a local `idIndex` that also picks up those hoisted property drawers.

The query language reaches this index too. `HasBacklinks()` (optionally with a floor: `HasBacklinks(2)`), `BacklinkCount()` as a number to compare, `HasLinks()`, `LinksTo(re)` and `HasBrokenLinks()` are defined in `todo.go` and made of `BacklinksTo` / `LinksOut` at the foot of `links.go`. Two rules there: a link naming the **file** is not counted against any heading in it, or every heading in a linked-to file would claim a backlink it has not got; and `LinksTo` runs its pattern over the link as written, its description, and the file and headline it lands on, because "links to notes.org" and "links to the migration heading" are both things somebody means by it. The index is cached against `ReloadIndex`, so the first heading a query evaluates builds it and a query mentioning no link function never builds it at all.

That same worg file view shows a file three ways, picked from buttons over the page: the html exporter's page, the file's own text with org syntax colouring (`components/OrgSource.tsx` in worg - nothing server side), and, for the files `/dnd/characters` reports, the `dndsheet` character sheet fetched from `/file/dndsheet` as a string rather than written to disk.

### Todo keywords over the wire

`GET /status` answers with the keywords this server accepts - `defaultTodoStates` split the way org splits it, everything before the `|` active and everything after it finished - in the order they were configured, which is the order they are meant to be read in. `GET /status/{hash}` is the per-heading question and answers from that heading's own file (`#+TODO:`) when it has one. worg's kanban builds a board's default columns out of the global list.

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

The thing to understand before changing it is **where the template language lives**. A `CaptureTemplate` carries a `template:` string and the server does nothing with it - `common.CaptureTemplate` says so in its own comment. It is a form for the client to put up, and `worg/src/capture.ts` is the only implementation of it there is: `{{CONTENT}}` is the body, `{{name}}` is a value to fill in, `{{name|prompt}}` is the same asked for in your own words, and a dozen names (`now`, `today`, `date`, `week`…) fill themselves in and stay editable. `capture.test.ts` pins all of it, and the SDOC block in `capture.go` documents it for whoever writes a template.

Four things about the rest of it:

1. **A yaml template longer than one line needs a block scalar** (`template: |-`). A plain multi-line scalar folds its newlines into spaces, which turns a property drawer into one long line. The shipped example used to get this wrong.
2. **A line whose only content was an unanswered placeholder is dropped**, so a template offering three optional properties writes the one that was answered rather than one and two empty ones - the same rule the record editor follows.
3. **A capture has no heading yet**, which is why dictating and pasting a picture need `internal/app/orgs/capturestash.go`. The kanban card can keep a picture *and* link it in one call because it has a heading; a capture being composed has to keep the file first and carry the link in the text it is still typing. Both need to know which file the link will be relative to, and the only thing the dialog knows is the template - hence `stash=1&template=NAME` on `/image/paste`, and `/voice/link`.
4. **The mic writes into the box the cursor was in**, which it works out on mouse down by reading `document.activeElement` back to a `data-cap-key` — not from a focus handler it kept. Pressing a button *moves* the focus onto the button, so asking afterwards is too late and asking a remembered state is asking something that may never have been told. The tooltip names the box it is about to write into, so there is no guessing before you speak.
5. **The preview is `OrgSource` with `openDrawers`**. The file view starts drawers shut because a file of headings is mostly drawer; a four-line preview starts them open, because what is *in* the drawer is exactly what somebody is checking before they press Capture.

`InsertEntryUsingTemplate` used to write the content as one concatenation - indent, the whole string, a newline - which indented only its first line. Every line is indented now, because anything a template produces is several lines and a `:PROPERTIES:` drawer landing at column zero is the go-org trap that hoists the rest of the heading to the top of the document. It writes `NewNode.Tags` as well, which the wire type has always carried and nothing wrote.

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

### Documentation extraction (SDOC / EDOC)

Comment blocks fenced with `SDOC: <section>` and `EDOC` inside the Go sources are extracted by `cmd/docex` into Org documentation. When editing existing comments that contain these markers, preserve the markers and their section names — they are load-bearing for the doc build, not dead comments.

**Do not run `gofmt` on a file whose SDOC block sits directly above a declaration** (the `/dnd/*` and `/records/*` endpoint files are all like this). Go 1.19+ reformats doc comments: it re-indents the block, turns the `* Heading` line into a `- Heading` list item and reflows the org tables inside it, which silently mangles the extracted docs. Two of the breakages are load-bearing and invisible in the diff unless you look for them — `docex` matches both markers as whole lines:

- `/* SDOC: API` becomes `/*` on one line and `SDOC: API` on the next, and the block is **not extracted at all**;
- `EDOC */` becomes `EDOC` and `*/`, and the block **runs on** into whatever follows.

`grep -c '/\* SDOC' file.go` and `grep -c 'EDOC \*/' file.go` should be equal and should match the number of blocks. Building the extractor and diffing its output is the real check: `go build -o /tmp/docex ./cmd/docex && /tmp/docex -src . -out /tmp/docs.org`. A block separated from the next declaration by a blank line — the ones above `import` in `internal/common/dnd/` — is left alone. `go build ./...` and `go vet ./...` are the health checks here, not `gofmt`.
