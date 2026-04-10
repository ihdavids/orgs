# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project overview

`orgs` is a Go-based Org Mode server + CLI. A single binary (`cmd/orgs`) acts as both:

- the HTTP/HTTPS server that watches org files, parses them, serves a REST API, and runs background plugins; and
- the command-line client — the first positional argument selects a subcommand from the registry (`serve`, `agenda`, `refile`, `grep`, `capture`, `export`, …) which then talks to a running server over REST.

A second binary, `cmd/docex`, scrapes `SDOC:` / `EDOC` marker comments out of the Go sources to generate documentation — it is not needed for normal builds.

Go module: `github.com/ihdavids/orgs` (Go 1.23+, toolchain 1.24.2). There are currently no `_test.go` files in the repo.

## Build and run

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

There is no Makefile, no lint config, and no test suite. `go vet ./...` and `go build ./...` are the practical health checks.

## Configuration

`Config.ParseConfig` in `internal/app/orgs/settings.go` is the authoritative loader. Key behavior to know before changing config plumbing:

- `flag.Parse()` is called **twice** — once before the YAML load (so `-config` can point at a file) and once after (so command-line flags override YAML). Anything that registers flags must do so in `SetupCommandLine` / `AddCommands` before the first parse.
- `Conf()` is a lazy singleton; most of the codebase reaches the config via `orgs.Conf()` rather than passing it around.
- The same `Config` struct holds both server settings (under `Server *common.ServerSettings`) and CLI-only options (`Url`, `EditorTemplate`, `Aliases`). The CLI reads `orgs.Conf().Url` to know which server to hit.
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
4. `authenticate` accepts either an `Authorization: Bearer <token>` header **or** an `orgstoken` cookie. Tokens are JWE-encrypted JWS (`jwt.go`), with keys `OrgJWS` / `OrgJWE` from server settings, and expire after 5 minutes.
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
- `core.Rest.Header` is where you attach an `Authorization: Bearer` header after logging in. The `login` command POSTs to `/login` and prints the returned JWE token; callers are expected to re-supply it on subsequent requests.
- `core.LaunchEditor(filename, line)` opens files in the editor configured by `editorTemplate`.

When adding a new CLI command: create `cmd/oc/commands/<name>/<name>.go`, implement the interface, register in `init()`, **and add the blank import to `cmd/oc/commands/all/all.go`**. Running `go build ./...` from the repo root is sufficient to verify it links.

### Server-side plugins

Server plugins implement one of the interfaces in `internal/common/plugs.go` (`Exporter`, `Poller`, `Updater`) and register themselves in `init()`. They are instantiated from the YAML config under `server.exporters`, `server.plugins`, and `server.updaters`, then started by `ParseConfig` via `pd.Plugin.Startup(...)`. As with CLI commands, a new plugin package must be blank-imported in `internal/app/orgs/plugs/all/all.go` to be reachable from YAML.

The `PluginManager` passed to plugins carries the shared templates, filter map, tag groups, org directories, and a cached password helper that can read from the OS keyring.

### Filters and tag groups

`Filters` and `TagGroups` in the YAML are macro-like helpers referenced by queries. `AddInternalFilters` / `AddInternalTagGroups` in `settings.go` seed a default set (`AllTasks`, `HomeTasks`, `WorkTasks`, `WorkProjects`, `PERSONAL`, `HOME`, `WORK`) unless `noInternalFilters` / `noInternalTagGroups` is set. Queries reference filters using `{{ FilterName }}` handlebars-style substitution.

### Documentation extraction (SDOC / EDOC)

Comment blocks fenced with `SDOC: <section>` and `EDOC` inside the Go sources are extracted by `cmd/docex` into Org documentation. When editing existing comments that contain these markers, preserve the markers and their section names — they are load-bearing for the doc build, not dead comments.
