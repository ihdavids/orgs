# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project overview

`orgs` is a Go-based Org Mode server + CLI. A single binary (`cmd/orgs`) is both:

- the HTTP/HTTPS server: watches org files, parses them, serves a REST API, runs background plugins;
- the command-line client: the first positional argument picks a subcommand from the registry (`serve`, `agenda`, `refile`, `grep`, `capture`, `export`, …), which talks to a running server over REST.

`cmd/docex` scrapes `SDOC:` / `EDOC` marker comments out of the Go sources into documentation; not needed for normal builds.

Go module: `github.com/ihdavids/orgs` (Go 1.23+, toolchain 1.24.2). Little test coverage - the only `_test.go` files cover the D&D appearance step and the CLI list chooser.

## Build and run

The worg frontend is embedded from `worg/` (`worg/embed.go`) and served by `StartServer`. `tools/buildworg.sh` builds worg from `../worg` and pulls the result in, clearing old files first because build names carry content hashes and a plain copy leaves every previous build in the binary. The built app asks the page's own origin for the api, so any port works.

- Subcommand flags go *after* the subcommand (`./orgs serve -port 8010`); a flag before it is consumed by the global parse and the command list is printed instead.
- The HTTP listener runs in a goroutine while HTTPS blocks: with `allowHttps: false` the process falls through both and exits, so a throwaway server on another port still needs `allowHttps: true` and a cert to stay up.

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

`orgs export`:
- Writes **here** (client side) by default; `-local` makes the server write at that path on its own disk. (It used to be reversed, so `orgs -url https://box:8010 export -out ./notes.html` wrote on box.)
- `-f pdf` is not an exporter call: the pdf exporter writes a file and returns nothing to a string, so `/pdf` is its own endpoint answering with bytes, cached against the org file's state when built.
- `-list` / `-list-themes` ask the server. `/exporters` exists for `-list` because an exporter compiled in but not named in the yaml is absent as far as a request is concerned, and its refusal looks identical to one for a nonexistent exporter.

No Makefile, no lint config. Health checks: `go vet ./...` and `go build ./...`. Tests: `go test ./internal/common/dnd/ ./cmd/oc/commands/dnd/`; plain `go test ./...` fails on its default vet check over pre-existing `fmt.Printf` calls in several plugins.

## Configuration

`Config.ParseConfig` in `internal/app/orgs/settings.go` is the authoritative loader.

- `flag.Parse()` runs **twice**: before the YAML load (so `-config` can name a file) and after (so flags override YAML). Register flags in `SetupCommandLine` / `AddCommands` before the first parse.
- `Conf()` is a lazy singleton; most code uses `orgs.Conf()` rather than passing config around.
- One `Config` struct holds server settings (`Server *common.ServerSettings`) and CLI-only options (`Url`, `Token`, `EditorTemplate`, `Aliases`). The CLI reads `orgs.Conf().Url` for the server. `Token` stores the JWT from the last `login` and is applied as a Bearer header on startup.
- Aliases (`aliases:` in YAML) are expanded in `cmd/orgs/main.go` *before* dispatch, so `args[0]` may become a multi-word command + flags.

## Architecture

Bug histories (what broke, how it was found, how it was fixed) are in `docs/claude/bug-history.md`. Read it before changing a section's code if the section points there.

### Traps that apply everywhere

Each of these comes up in several features below. They are stated once here, and sections refer to them by name (e.g. "see **Traps: line edits**").

- **Pointer trap.** go-org produces pointers for nested nodes: `*org.Headline`, `*org.Block`, `*org.Drawer`. A `case org.Headline` (value) type switch never fires and fails silently. The exception is `Block.Result`, which is a `Result` value, not a pointer.
- **Column-zero drawers.** A drawer or block written in column zero (which Emacs does for property drawers) ends the headline's body. The rest of the heading is hoisted to the top level of `Document.Nodes`, so `Headline.Properties` is nil and `Headline.Children` is empty. Walk nodes in order and attribute each to the **last heading starting above it**. Read properties off the file's own lines, not from `Headline.Properties`.
- **Line edits, not document rewrites.** Writing a parsed document back (`WriteOutOrgFile`) re-indents every drawer, reflows every table and loses timestamp spellings across the whole file. Every write splices only the lines it changes.
- **Re-measure after the line count changes.** After any edit that adds or removes lines, find the heading's extent again (`reread`). Measuring once and reusing it writes the next edit into the wrong heading, and the write still reports success.
- **Hashes.** A heading hash is base64 already. In a url path it must be encoded again (`commands.HashPath`): `GetHash` base64-URL-decodes the segment, and a raw `+` or `/` mangles it into "no heading with that hash". A hash is accumulated from the file name and the chain of titles above it, so **any edit to a file can change the hashes of every heading after it**. Sections register into `ByHash`/`ById`/`ByCustomId` lazily as queries walk them, not at load, so an index must be built on a miss.
- **Caches key on `OrgDb.ReloadIndex`.** The link, record, code and hash indexes are rebuilt when it changes. Anything that writes a file bumps it, so nothing behind those caches goes stale.
- **Stdout is the answer.** Logs, plugin chatter and every diagnostic go to stderr, because `-json | jq`, `-local` and `orgs mcp` all share stdout with the result.
- **Media paths are resolved server side.** A link in an org file is relative to that file, while the file server is rooted at the first org directory; the client knows neither. Resolve through `MediaSrc`/`plugs.MediaURL`/`mediaURL` and hand out a path (`/images/...`), never a `http://localhost:port` url, so the page works from any host (including a phone).
- **Logic said twice must change together.** Each of these exists in two or three languages because a round trip or a server would defeat its purpose. Nothing at build time notices when they drift:
  - `common.ClientHash` ↔ `worg/src/clienthash.ts` (pinned by the same vector in `TestClientHashVector` and `clienthash.test.ts`)
  - `internal/common/dnd/fuzzy.go` ↔ `worg/src/fuzzy.ts` ↔ the dnd sheet's palette JS ↔ `templates/html_docs_app.js`
  - `HealthLevel` ↔ the sheet's `hpLevel` (`TestHealthBands`)
  - record field kinds: `fieldOf` ↔ `records.ts`; occasions: `birthdayFields` ↔ `CELEBRATIONS`
  - habit tone rules: `cmd/oc/commands/habits/view.go` ↔ `worg/src/habits.ts`
  - capture template grammar: `internal/common/captemplate.go` ↔ `worg/src/capture.ts`
  - code block header order: `headerLine` in Go ↔ `headerLine` in `worg/src/code.ts`
  - note line breaks: `orgBreakLines`/`orgStripBreaks` ↔ the sheet's `orgToHtml`
  - query functions: the map in `internal/app/orgs/todo.go` ↔ `QUERY_FUNCTIONS` in `worg/src/orgquery.ts` (a function missing there still works, it just is never suggested)
  - flashcard sessions: `worg/src/drill.ts` ↔ `cmd/oc/commands/drill/session.go` (same scenarios in `drill.test.ts` and `session_test.go`)

### Where the rest of the notes live

Feature notes sit in `CLAUDE.md` files beside the code and load when you work there:

- `cmd/oc/commands/CLAUDE.md`: the CLI commands (`cmd/oc/commands`)
- `cmd/oc/commands/capture/CLAUDE.md`: `orgs cap`
- `cmd/oc/commands/habits/CLAUDE.md`: `orgs habits`
- `cmd/oc/commands/mcp/CLAUDE.md`: `orgs mcp`
- `cmd/oc/commands/orghl/CLAUDE.md`: `orghl`
- `cmd/oc/commands/pres/CLAUDE.md`: `orgs pres`, the terminal slideshow
- `cmd/oc/commands/drill/CLAUDE.md`: `orgs drill`, flashcards in the terminal
- `cmd/oc/commands/snip/CLAUDE.md`: `orgs snip`, command-line snippets (pet over source blocks)
- `docs/claude/worg.md`: the worg frontend
- `internal/app/orgs/CLAUDE.md`: the server (`internal/app/orgs`)
- `internal/app/orgs/plugs/html/CLAUDE.md`: the html exporter
- `internal/app/orgs/plugs/slides/CLAUDE.md`: the presentation exporters
- `internal/common/dnd/CLAUDE.md`: the D&D module
- `templates/CLAUDE.md`: the html templates and documentation themes
- `docs/claude/bug-history.md`: what broke while building each feature, and how it was fixed

### Top-level layout

- `cmd/orgs/main.go` — entry point for server and CLI. Builds a `commands.Core`, then routes to a subcommand from `commands.CmdRegistry` or (via `serve`) calls `orgs.StartServer`.
- `cmd/oc/commands/` — one subpackage per CLI subcommand. `commands.go` defines the `Cmd` interface, `Core` (wraps a `common.Rest` client), `CmdRegistry`, and the generic `SendReceiveGet` / `SendReceivePost` helpers.
- `cmd/oc/commands/all/all.go` — blank-imports every command package so `init()` registers it. **New commands must be added here or they are unreachable at runtime.**
- `internal/app/orgs/` — the server: REST handlers (`rest.go`), auth (`auth.go`, `jwt.go`, `user.go`), in-memory org DB (`orgdb.go`), capture/refile/archive/clock logic, settings, server-side plugin host.
- `internal/app/orgs/plugs/` — server plugins (exporters, pollers, updaters) like `html`, `revealjs`, `latex`, `jira`, `todoist`, `googlecal`, `notify`; each self-registers in `init()`.
- `internal/app/orgs/plugs/all/all.go` — blank-imports every server plugin, same reason.
- `internal/common/` — shared client/server code: `restclient.go` (`RestGet[T]` / `RestPost[T]`), `serversettings.go`, `plugs.go` (`Exporter` / `Poller` / `Updater` interfaces and `PluginManager`), wire data types.
- `internal/templates/` — pongo2 template manager for exporters and capture templates.
- `templates/`, `web/`, `webfonts/` — static assets served by the HTTP file server beside the REST API.

### Server request flow

1. `orgs serve` calls `orgs.StartServer(sets *common.ServerSettings)` in `internal/app/orgs/serve.go`.
2. It creates a `mux.Router`, calls `RestApi(router)` in `rest.go`, mounts static directories (`/images`, `/orgimages`, `/orgfonts`, `/`), starts background plugins (`startPlugins`), and listens on HTTP and/or HTTPS per config.
3. `RestApi` registers `POST /login` as the only **public** route; everything else is under a subrouter using the `authenticate` middleware from `auth.go`.
4. `authenticate` accepts an `Authorization: Bearer <token>` header **or** an `orgstoken` cookie. Tokens are JWE-encrypted JWS (`jwt.go`): HS256-signed with `OrgJWS`, PBES2-encrypted with `OrgJWE` from server settings, expiring after 5 minutes.
5. Handlers in `rest.go` typically unmarshal JSON into a `common.*` type, delegate to `orgdb.go` / `capture.go` / `refile.go` / etc., and JSON-encode the result.

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

`init()` calls `commands.AddCmd("name", "usage", factory)`. `main.go`:

1. Parses global flags, loads config, builds `commands.Core` with `core.Rest.Url = Conf().Url`.
2. Expands aliases from YAML.
3. Finds the entry in `CmdRegistry`, lets its `flag.FlagSet` parse the remaining args, calls `Exec(core)`.

Helpers in `cmd/oc/commands/commands.go` for use inside `Exec`:

- `SendReceiveGet[RESP](core, "path", params, &resp)` — wraps `common.RestGet`.
- `SendReceivePost[REQ, RESP](core, "path", &req, &resp)` — wraps `common.RestPost`.
- `core.Rest.Header` carries `Authorization: Bearer`; `login` POSTs to `/login`, saves the token to the YAML (`token:`) and prints it; later runs auto-load it.
- `core.ConfigFile` is the active config path, so commands can read/update it without importing `internal/app/orgs`.
- `core.LaunchEditor(filename, line)` opens the editor configured by `editorTemplate`.

**Import cycle constraint**: packages under `cmd/oc/commands/` **cannot** import `internal/app/orgs`, because it imports `cmd/oc/commands/all` (which blank-imports every command). Use only `cmd/oc/commands` and `internal/common`; pass config values through `Core` fields set in `main.go`.

Adding a CLI command: create `cmd/oc/commands/<name>/<name>.go`, implement the interface, register in `init()`, **and add the blank import to `cmd/oc/commands/all/all.go`**. `go build ./...` from the repo root verifies it links.

### Every command's four flags, and where they come from

`-json`, `-format`, `-dry-run` and `-no-color` are registered by no command. `Config.AddCommands` (`settings.go`) calls `commands.AddGlobalFlags` on each flag set **after** `SetupParameters`, and it only defines names still free. So a command's own meaning wins: the dnd client keeps its own `-json` (a saved D&D Beyond payload) and `-format` (html/latex/pdf).

`cmd/oc/commands/output.go`:

- **`Render(rows, plain)` / `RenderOne(row, plain)`** replace a command's own printing: `-json` marshals; `-format` runs a Go `text/template` per row (funcs `base`, `dir`, `upper`, `lower`, `trim`, `join`, `cut`, `pad`, `prop` - the last reads a heading's `Props` map); neither runs `plain()`.
- **`Machine()`** = a program, not a person, is reading. Check before opening an fzf chooser or survey prompt; refuse rather than block on an unanswerable question.
- **`Colour()`** = a terminal, and not `-no-color`, not `NO_COLOR`, not `Machine()`. `C(code)` returns the escape or `""`, so callers concatenate without branching.
- **`Fail`** reports an error as a `{Ok, Msg}` object under `-json`, else a stderr line.

`-dry-run` is handled in **`SendReceivePost`** (prints method, path, body to stderr and returns), so no command can forget it. `SendReceivePostErr` returns `ErrDryRun`: treat as "nothing happened", not failure. Non-POST writes (exporter writing on the server's disk, local file write, running a source block) call `Wrote(what, detail)` explicitly.

This depended on (see **Traps: stdout is the answer**):

1. `logToFile` in `cmd/orgs/main.go`, `RestGet`'s unmarshal complaint and the missing-config-file message in `settings.go` all write to stderr (`logToFile` once used `io.MultiWriter(os.Stdout, f)`, breaking `-json | jq` and `orgs mcp`).
2. **`FreeArgs`/`FreeText`** re-run the parse after taking each word off, because Go's `flag` stops at the first non-flag (so `orgs search 'IsTask()' -json` used to treat `-json` as a query word). Used by `search`, `find`, `links`, `export`, `tui`, `code`, `rec`. Rule: flags and words may come in any order.

### Running without a server: `-local`

`orgs -local <command>` (implied by `orgs -orgdir ./notes <command>`) runs the server in-process for one command - for git hooks (`orgs agenda`), CI (`orgs fmt -check`), ssh with no daemon. Port 0 (kernel-assigned), auth off, readiness polled. Required fixes:

1. `orgs serve` with `allowHttps: false` exited immediately (the https listener was what held `StartServer` open). It now waits on a signal, which is also the only path where `StopWhisper` and `stopPlugins` run.
2. ~300 diagnostic `fmt.Printf` calls in server and plugins went to stdout, which `-local` shares with the command (`WATCHING:`, `PLUGIN START:` spliced into json). All go to stderr now (see **Traps: stdout is the answer**).

### Server-side plugins

Plugins implement `Exporter`, `Poller` or `Updater` from `internal/common/plugs.go` and register in `init()`. They are instantiated from YAML `server.exporters`, `server.plugins`, `server.updaters`, and started by `ParseConfig` via `pd.Plugin.Startup(...)`. A new plugin package must be blank-imported in `internal/app/orgs/plugs/all/all.go` to be reachable from YAML.

The `PluginManager` passed to plugins carries the shared templates, filter map, tag groups, org directories, and a cached password helper that can read the OS keyring.

### Filters and tag groups

`Filters` and `TagGroups` in the YAML are macro-like helpers referenced by queries. `AddInternalFilters` / `AddInternalTagGroups` in `settings.go` seed a default set (`AllTasks`, `HomeTasks`, `WorkTasks`, `WorkProjects`, `PERSONAL`, `HOME`, `WORK`) unless `noInternalFilters` / `noInternalTagGroups` is set. Queries reference filters using `{{ FilterName }}` handlebars-style substitution.

### Documentation extraction (SDOC / EDOC)

`cmd/docex` extracts comment blocks fenced with `SDOC: <section>` and `EDOC` in the Go sources into Org documentation. Preserve the markers and section names when editing such comments; the doc build depends on them.

**Do not run `gofmt` on a file whose SDOC block sits directly above a declaration** (all the `/dnd/*` and `/records/*` endpoint files). Go 1.19+ reformats doc comments: re-indents the block, turns `* Heading` into `- Heading`, and reflows org tables. Two breakages are invisible in a diff, because `docex` matches both markers as whole lines:

- `/* SDOC: API` becomes `/*` on one line and `SDOC: API` on the next, and the block is **not extracted at all**;
- `EDOC */` becomes `EDOC` and `*/`, and the block **runs on** into whatever follows.

Checks: `grep -c '/\* SDOC' file.go` and `grep -c 'EDOC \*/' file.go` should be equal and match the block count. The real check is building and diffing the output: `go build -o /tmp/docex ./cmd/docex && /tmp/docex -src . -out /tmp/docs.org`. A block separated from the next declaration by a blank line (those above `import` in `internal/common/dnd/`) is left alone. Use `go build ./...` and `go vet ./...` as health checks, not `gofmt`.
