# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project overview

`orgs` is a Go-based Org Mode server + CLI. A single binary (`cmd/orgs`) acts as both:

- the HTTP/HTTPS server that watches org files, parses them, serves a REST API, and runs background plugins; and
- the command-line client — the first positional argument selects a subcommand from the registry (`serve`, `agenda`, `refile`, `grep`, `capture`, `export`, …) which then talks to a running server over REST.

A second binary, `cmd/docex`, scrapes `SDOC:` / `EDOC` marker comments out of the Go sources to generate documentation — it is not needed for normal builds.

Go module: `github.com/ihdavids/orgs` (Go 1.23+, toolchain 1.24.2). There is no test suite to speak of - the only `_test.go` files cover the D&D appearance step and the CLI list chooser.

## Build and run

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

### Server-side plugins

Server plugins implement one of the interfaces in `internal/common/plugs.go` (`Exporter`, `Poller`, `Updater`) and register themselves in `init()`. They are instantiated from the YAML config under `server.exporters`, `server.plugins`, and `server.updaters`, then started by `ParseConfig` via `pd.Plugin.Startup(...)`. As with CLI commands, a new plugin package must be blank-imported in `internal/app/orgs/plugs/all/all.go` to be reachable from YAML.

The `PluginManager` passed to plugins carries the shared templates, filter map, tag groups, org directories, and a cached password helper that can read from the OS keyring.

### Voice notes and go-whisper

`internal/app/orgs/voice.go` records nothing and transcribes nothing. A client records, the server keeps the audio, and transcription is a request to a [go-whisper](https://github.com/mutablelogic/go-whisper) server over its own http api (`/api/whisper/model`, `/api/whisper/transcribe`) — which is what keeps the model, and the hardware it wants, out of this program. Settings live under `voice:` in the server config (`url`, `model`, `language`, `dir`, `timeout`, `maxMb`, `tags`, `target`); `orgs.yaml`'s `voice.dir` is relative to the first orgDir so a note's audio sits in the org database beside the heading that links to it.

Run the service alongside orgs:

```sh
gowhisper run --http.addr localhost:8081 --models /path/to/models --whisper.gpu
```

Five things to keep in mind when changing it:

1. **The recording is saved before anything is attempted on it, and the transcript goes back to the client before anything is written.** A transcription fails in a dozen ways — whisper down, a model still loading, a take past the timeout — and none of them should cost the words that were said. `POST /voice/note` writes the *text it was sent*, never a fresh transcription, because what lands in the file has to be what was on screen.
2. **Nothing is converted.** go-whisper decodes through ffmpeg, so a browser's webm/opus, a phone's m4a and a recorder's wav all go straight through; the extension follows the recording and the `filename` field tells whisper what container it is looking at.
3. **`/api/whisper/model` answers with a bare JSON array**, whatever the api doc says about an object with a `models` field. `whisperModels` reads both, because guessing wrong is silent: an empty model list is indistinguishable from a server with nothing installed.
4. The heading is built as **lines of text and spliced in** (`voiceNoteLines` + `insertLinesAt`), not written through go-org. Writing it through the document would rewrite the whole target file — every drawer re-indented, every table reflowed — to add one heading to the end of it.
5. A recording id **is its own filename** (`voiceIdRe`), so there is no index beside the folder to fall out of step with it, and a path that is not exactly that shape is refused rather than joined.

### Per-user extensions

`internal/app/orgs/extensions.go` is a small per-user store written beside the main config (`orgs.yaml` → `orgs_extensions.yaml`), holding the things a user accumulates rather than configures: stored queries, their own capture templates, and kanban boards. Every handler reads the username off the auth token, so there is no user parameter anywhere in the API.

Two things about the kanban board endpoints are worth knowing before changing them. A `KanbanBoard` deliberately holds **no cards** - it names a query and says how to draw whatever that finds, so it can never be stale and deleting one touches no heading. And `POST /ext/kanban/boards` replaces the whole list in one write, because renaming a board and reordering the tabs both change a list rather than one entry: done as a delete plus an add, a lost second call would leave the boards half written.

The board that made it necessary is worg's Kanban tab, which writes a heading's property when a card is dropped in a column. That exposed a nil dereference in `SetProperty` (`todo.go`): a heading with no `:PROPERTIES:` drawer has `Headline.Properties == nil`, and the old `if props == nil` check could never fire, having taken the address of a field first. It now creates the drawer, which the org writer prints directly under the headline - so anything setting a property from outside the editor works on a heading that has never had one.

### Backlinks and the link graph

`internal/app/orgs/links.go` walks every `[[target][description]]` link out of every parsed file, resolves it against the database, and indexes it from both ends. It serves `/links` (backlinks for one file), `/links/graph` (the graph around a file, or the whole database, at file or heading granularity) and `/links/stats` (per-file counts). Wire types are in `internal/common/links.go`; the worg client is `components/Files.tsx` plus the plain-svg force layout in `components/LinkGraph.tsx`.

The index is cached against `OrgDb.ReloadIndex` and rebuilt whenever a file reloads. Two things about the rest of the system shape it:

1. Sections are registered into `ByHash` / `ById` / `ByCustomId` lazily, as queries walk them (`ScanNode`'s `RegisterSection` call is commented out), so `buildLinkIndex` walks every file into the registry in a first pass before resolving anything.
2. go-org ends a headline's body at a drawer written in column zero, which leaves `Headline.Properties` nil and drops the rest of that heading into `Document.Nodes` at the top level. So links are attributed to the last heading starting above them rather than trusting the outline alone, and ids are read from a local `idIndex` that also picks up those hoisted property drawers.

That same worg file view shows a file three ways, picked from buttons over the page: the html exporter's page, the file's own text with org syntax colouring (`components/OrgSource.tsx` in worg - nothing server side), and, for the files `/dnd/characters` reports, the `dndsheet` character sheet fetched from `/file/dndsheet` as a string rather than written to disk.

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

### Documentation extraction (SDOC / EDOC)

Comment blocks fenced with `SDOC: <section>` and `EDOC` inside the Go sources are extracted by `cmd/docex` into Org documentation. When editing existing comments that contain these markers, preserve the markers and their section names — they are load-bearing for the doc build, not dead comments.

**Do not run `gofmt` on a file whose SDOC block sits directly above a declaration** (the `/dnd/*` endpoint files are all like this). Go 1.19+ reformats doc comments: it re-indents the block, turns the `* Heading` line into a `- Heading` list item and reflows the org tables inside it, which silently mangles the extracted docs. A block separated from the next declaration by a blank line — the ones above `import` in `internal/common/dnd/` — is left alone. `go build ./...` and `go vet ./...` are the health checks here, not `gofmt`.
