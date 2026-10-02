# Notes on the CLI commands (`cmd/oc/commands`)

Loaded when working in this directory. References like **Traps: line edits** point at the Traps list in the root `CLAUDE.md`.

## The CLI commands that read the server's newer endpoints

Terminal halves of records, links, code, file search and babel. Each is one request to an existing endpoint; a command with its own logic would be a second answer to a question the server already answers.

- **`orgs search '<expr>'`** — `/search`, the query language. `-sort`, `-limit`, `-count`, `-group`, `-open`.
- **`orgs find '<re>'`** — `/files/search`, regex over raw text. Unlike `orgs grep`'s unsplittable `"file|line|text"` strings, matches carry offsets, so colouring needs no second regex run in another engine.
- **`orgs links`** — `/links/all`, `/links`, `/links/stats`. Bare: a picker with the far end drawn beside it; `ls` is the listing. See **The pickers**.
- **`orgs tables`** — `/tables`, `/table`, `/table/eval`. The Tables tab as a picker (grid with `@n`/`$n` rulers and formulas). See **The pickers**.
- **`orgs code`** — fzf picker with the block drawn beside it; `ls`, `show`, `run` name one outright. See **The pickers**.
- **`orgs rec`** — record endpoints over *every* collection. `orgs contact` (same engine, address-book defaults) is unchanged. `record` is now an alias for `rec`, not `contact` (a contact default was wrong for the general name).
- **`orgs tui`**, **`orgs mcp`** — see below.

Hashes in url paths go through `commands.HashPath` (see **Traps: hashes**); a raw one fails as "no heading with that hash", which looks like a stale hash.

## The write verbs, and how a command names the heading it is about

`orgs todo`, `sched`, `deadline`, `tag`, `prop`, `rename`, `note`, `check`, `archive`, `rm` live in `cmd/oc/commands/edit/` (endpoints previously reachable only from `orgs mcp` and the `tui`/`agenda` screens). One package so **`commands.Resolve` in `target.go`** picks the heading identically for all. Four ways in:

```sh
orgs todo DONE                        # a picker
orgs todo DONE 'IsStatus("NEXT")'     # a query
orgs todo DONE hOpOB7vIg6oiYz5sMVS=   # a hash
orgs search '…' -json | jq -r .Hash | orgs tag +stale -   # hashes on stdin
```

Load-bearing rules:

1. **More than one match is a question.** Interactive: a picker. Pipe only: refuse and say how many. `-all` means all, `-at N` picks one.
2. **A hash is recognised by shape** (28 chars of base64 ending in `=`), so the tool's own output pastes back without a flag.
3. **`orgs tag` reads current tags first**, because `POST /tags` *toggles* (`+work` on a heading already tagged work would remove it). This makes it idempotent and pipeline-safe.
4. **`orgs note` strips the property drawer and planning line** (`proseOnly`) from `/body/{hash}` before writing via `/body/change`: the body comes back with its drawer, and the drawer is regenerated from the headline, so it was written twice.
5. **A dash-word is a value.** `orgs tag -someday` removes a tag, but Go's flag package exits on undefined flags. For `commands.DashWords` commands, `main.go` lifts such words out *before* parsing - only words not defined as flags, so `-json` stays a flag with no list to maintain.

## Tab completion

`orgs completion zsh|bash|fish` prints a script that shells out to **`orgs __complete <words…>`**, so logic lives once in Go and new commands need no script edits. Completes files, tags, *this heading's file's* keywords, saved queries, filters, exporters, themes, collections.

- **900ms timeout** (`Rest.Timeout`), degrading to no suggestions - an unresponsive prompt is worse than an empty one.
- **Never completes the target of a destructive verb** (e.g. the heading `orgs rm` will delete) - it would make deleting the wrong thing quick.

## `orgs review`, `orgs diff`, `orgs blame`, `orgs doctor`

- **`review`**: ten named checks, each only a query and a sentence (own code would duplicate the query language). An empty check prints a green line, not silence - its most valuable output. `-fix` walks findings into the editor, not a menu of automatic repairs: "no next action" needs a decision.
- **`diff` and `blame`** (`gitorg/`) report which headings changed and how, not lines. Both revisions are parsed **locally**, so they work in a pre-commit hook and on any checkout. Traps: nested headings are `*org.Headline` (see **Traps: pointer trap**); go-org's default keyword list is only `TODO | DONE`, so in a file without `#+TODO` every other keyword was read as headline text and NEXT→DONE looked like a rename.
- **`doctor`** checks everything at once, because "server down", "token expired" and "exporter not in the yaml" all surface as "could not". Marked `NeedsNoServer` so startup checks cannot block it.

## `orgs help`, and the commands that need no server

`commands.Offline` (`NeedsNoServer()`) marks a command with nothing to authenticate to. The `main.go` token check used to run for every command (so `orgs completion zsh` refused over a token it never sends), and exited on an expired token **without trying the refresh**.

`commands.Grouped` (`HelpGroup()`) lets a runtime-registered command name its listing group - every yaml filter is a command, and the group table in `help.go` cannot name them.

## The commands that were endpoints with nobody to call them

`cmd/oc/commands/orgtools/` = `tangle`, `fmt`, `tags`, `outline`, `log`; `savedq/` = `orgs q`; `voice/` = `orgs voice`. Each is one request to an existing endpoint.

- **`orgs tangle`**: by default says what blocks would write; `-w` writes on the server's disk; `-o DIR` writes here (the `orgs export` distinction). Under `-o`, paths are taken **relative to the org file**, because the server answers with its own absolute paths.
- **`orgs fmt -check`** writes nothing, exits non-zero (pre-commit contract). `POST /reformat` always writes, so it uses **`GET /reformat`** (what the writer *would* produce, and whether the file matches); the server compares since the file may not be local.
- **`orgs outline -match`**: sparse tree - query hits plus ancestors to locate them, which `orgs search` cannot show.
- **`orgs q`**: worg's stored queries; runs `/search` with the stored text, so every `search` feature applies.
- **`orgs voice`** records with `sox`/`ffmpeg`/`arecord` (not linked, same reason whisperd.go supervises go-whisper). **Upload audio before anything else; show the transcript before writing.** Failed upload: save to a temp file and print the path - never lose the only copy. Uses `Rest.PostFile` (multipart).

Bugs fixed while building these (pointer trap in the tangler and `ChangeBody`, an inverted `IsBlockedProject`, `FindByHash` now building the index on a miss, repeaters on planning lines pinned by `TestSDCKeepsItsCookie`) are written up in `docs/claude/bug-history.md`.

## The pickers

Bare **`orgs code`**, **`orgs links`**, **`orgs tables`**, **`orgs rec`**, **`orgs contact`** are an fzf list with a pane. Unlike `orgs grep` (pane via `bat`, which knows nothing of org), their items are not files, so this binary draws the pane: fzf's `--preview` runs `orgs <thing> preview …`. Shared machinery in `cmd/oc/commands/picker.go` (`Pick`, `SelfCommand`, `Shq`, `PaneWidth`, `OpenBox`/`BoxLine`/`CloseBox`) so fixes land once. Rules (each was a bug):

1. **Pass `-config` and `-url` to the child explicitly**: it is a fresh process whose working directory may differ.
2. **The command is a shell string**: single-quote every path (`Shq`). fzf already quotes the `{1}`/`{2}` it substitutes; leave those alone.
3. **Tab-separated list**, hidden address fields first (`--with-nth`); `orgs grep`'s `|` split breaks on paths containing a pipe. The full path is also displayed, because what is shown is what is searched.
4. **Pane width**: `FZF_PREVIEW_COLUMNS`, then `COLUMNS`, then 80. Boxes are **open on the right**: closing one needs every line's display width, which tabs and wide characters make a guess.
5. **`commands.PickerOutput` tells the pane it may colour** (like `bat --color=always`): the child's stdout is a pipe, so `Colour()` said no. Visible once `orgs habits` got a list-**reload** key (coloured parent list replaced by uncoloured child list). `-no-color`/`NO_COLOR` still win; pass `-no-color` on to children or pane and listing disagree.

Also: **`commands.FreeArgs` consumes arguments as it parses**, so a command with flags-anywhere *and* a subcommand must read the subcommand from the slice it returns; the flag set is empty afterwards (this made `orgs links preview` search for "preview").

### The code pane

Pane = inputs, code, outputs:
- A box of switches, header arguments and `:var` bindings, each resolved (`:var data=monthly` shows `→ table 6×7 in notes.org`); a name resolving to nothing is in danger colours, since the block cannot run.
- The code, through `bat` if on PATH, else plainly line-numbered.
- A `results` box when the block has `#+RESULTS:`.

The list line carries `← monthly` (the table it reads) as well as the name, because blocks are often looked for by their input, and an unnamed block has nothing else to be found by.

`ctrl-r` runs the highlighted block in place (the pane is for deciding whether to). Picking one prints it then **asks** - enter on a list is not consent to run a program. `-run` skips the question; `-no-run` never asks.

`commands.Interactive()` gates every picker: a pipe, `-json` or cron gets the listing instead.

### The links pane

The pane answers "is this the one I meant", mostly by **where the link was written** (an old pasted ticket is identified by its paragraph, not its url): the link's text and kind, the surrounding file lines with its line marked, then the far end - target heading's body (org link), the file (`file:`), or a plain refusal (broken).

* **Address = row position.** A link has no id (only file, line, text), and three fields through a shell command invite quoting bugs. The pane re-runs the query and indexes in, so `previewArgs` must pass **every** flag affecting the answer, or `-at 4` differs between child and parent.
* **Context lines are read from the local file**, not an endpoint (too much server for a preview). If the file is not here, the pane says so and still renders the rest.
* **Enter opens where the link was written**; following it is the browser's job. `ctrl-o` follows via the desktop opener, since only it knows what `doi:` or `mailto:` means.

go-org gives every inline node the position of the node it was parsed in, so `fixLinkLines` in `links.go` finds each link's real line in the file (history in `docs/claude/bug-history.md`).

### `orgs go` and `orgs gocap`

`cmd/oc/commands/golink/` (`go` is a Go keyword). `orgs go <name>`: case-insensitive prefix match on link **descriptions** over `/links/all`; one target (duplicates of the same `Raw` count as one, and an exact description wins) is followed, more opens the picker. Its pane is `links.RenderPane`; the child is `orgs go -pane-at N <name>` - a flag, not a subcommand word, so a link called "preview" still resolves. Enter follows; **ctrl-e uses `commands.PickKey` (fzf `--expect`)** so the picker is closed before the editor starts - `execute-silent` would leave a terminal editor fighting fzf for the screen.

`orgs gocap [link] [description]` files through `/capture` with the built-in **`GoLink`** template (`builtinCaptureTemplates` in `internal/app/orgs/capture.go`, target from `server.goLinksFile`/`goLinksHeading`). A user template named `GoLink` replaces it.

### The records pane

`orgs rec` and `orgs contact` draw everything (list line, card, bindings) via **`cmd/oc/commands/recview`**, separate because neither command owns the other and copies would drift. It knows no specific collection: a record is a name, server-typed fields, notes and history.

* **Fields ordered by kind; unknown kinds sort into the middle, not last**, so a collection's odd fields (the point of e.g. a guitar-pedal record) are not buried under urls.
* **The two commands differ in one drawing decision**, a `PickOptions` field: `rec` labels rows with their collection; `contact` does not (all rows share one collection).
* **A record is addressed by its own hash**, not list position (records have identity, links do not), so the pane still finds it after filtering. Encode via `commands.HashPath` (see **Traps: hashes**).

Occasion arithmetic ("turns 36 in eight days") is deliberately **not** in the pane: celebration fields are already defined twice (see **Traps: logic said twice**); `orgs rec birthdays` asks the server instead of adding a third copy.

Both commands keep their old narrow-to-one chooser as `narrow`: `show`, `edit` and `set` resolve a name to one record with a survey prompt (right for a command given a name rather than opened to browse).

### The tables pane

`orgs tables` draws what worg's Tables tab draws:

1. **Rulers** - `@1` down the side, `$1` across the top - because formulas are written in those coordinates (`$3=$1*$2`).
2. **Rules.** A `|---+---|` line takes **no** row number; org does not count it, so numbering it would offset every formula below.
3. **Formulas against the cells they fill.** `CellFormulas` is keyed `"row,col"`; computed cells are green. A `#+TBLFM:` line like `$4=$2*$3` does not say where results land.

Columns start at their widest cell and are squeezed a character at a time off the widest until the grid fits (too-wide is the common case).

`orgs tables eval` writes formulas back via **`POST /table/eval`** (file + table ordinal). **Not `/exectable`**: its SDOC says "the file is re-saved to disk after the update", but `ExecTable` only updates the *in-memory* table and returns rendered org text - it seems to work (reads hit the same memory) but values vanish on the next reload. That doc comment is still wrong; worth fixing.

## `orgs tui`

One screen: a table of a query's results. `/` filter, `:` re-query, `t` change keyword, `v` heading body, Enter open.

Query and filter are separate on purpose: the **query** costs a request and can say `IsStatus("NEXT")`; the **filter** is free and can say "the one about the invoice", so it runs on every keystroke. The filter is `internal/common/dnd/fuzzy.go`, the same matcher as the dnd chooser and worg's palette.

1. **The filter's haystack is the file's base name, never its path**: fuzzy matching ignores letter distance, so a long shared absolute path matches nearly anything (under `/Users/someone/dev/notes`, `mig` found every heading). The base name is also what people mean.
2. **The keyword menu asks `/status/{hash}`** on open, not the last query's keywords, because the heading's file may have its own `#+TODO:` and a keyword it lacks will not read back. Same rule as worg's kanban.
3. **Opening an editor suspends the app** (`app.Suspend`): editor and tview both want the terminal and the loser draws over the other.
