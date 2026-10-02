# Notes on `orgs snip`

Loaded when working in this directory. References like **Traps: line edits** point at the Traps list in the root `CLAUDE.md`.

## pet, over org source blocks

`orgs snip` is pet (github.com/knqyf263/pet) with org source blocks for storage. A snippet is any shell-language block `GET /code` returns (`-all`: any language); title = heading (+ `#+NAME:`), tags = `CodeBlock.Tags` (own + inherited, added to `/code` for this). User docs are the SDOC block at the end of `snip.go`.

- **`params.go` is the whole parameter language**, pure and tested (`params_test.go`): pet's `<name>`, `<name=default>`, `<name=|_a_||_b_|>`, plus `<name?>` (optional, takes one space with it when empty), `\<x>` (literal), and babel `:var` (a parameter only when the body says `$name`/`${name}`; a `:var` naming a table/block is data, skipped; a lisp list is choices). A hole's name must look like a name, so `a < b > c` and heredocs aren't holes.
- **Quoting depends on where the value lands** (`quoteAt` walks the template's quote state): escaped for single or double quotes when inside them; bare values single-quoted only for whitespace/quotes/`;&|<>()\``, never for globs or `$` (`<glob=*.log>` is asking for a glob). This fixes pet's #41/#119/#150.
- **The form (`form.go`) draws the filled command live**, values marked with `\x01`/`\x02` sentinels that pass through `Fill`'s quoting untouched. Esc cancels with exit 130 (the shell widgets stay quiet on 130); pet's ^C ran an empty command.
- **Runs locally, never via `/code/run`**: a snippet is for the machine you are typing at. `:dir` is honoured, relative to the org file. A form filled in is consent to run; with no form, ask (or `-y`); with no tty and no `-y`, refuse.
- **`print` works from a shell widget**: stdout is the `$(...)` pipe, so `choose` allows a picker when stdin is a tty, the picker lines are coloured regardless (`Colour()` says no for a pipe), and fzf and tcell both draw on `/dev/tty`. Widgets capture stderr to a temp file, because the client's startup log lines otherwise land on the prompt.
- **`new` files through the server's built-in `Snippet` capture template** (`builtinCaptureTemplates` in `capture.go`, appended after configured ones so a user template of that name wins), target `snippets.org` / `Snippets`. Same template shows up in `orgs cap` and worg's capture dialog.
- **zsh adds a history line after running it**, so `snip-prev` uses `fc -ln -1 -1`; bash has already added it, so `history 2 | head -1`; fish skips its own `snip-prev` entry.
- Recently used snippets sort first (`$XDG_CACHE_HOME/orgs/snip-usage.json`), keyed by file + heading + name, not block ordinal, which moves when a block is added above.
- `FreeArgs` was fixed for this: words before `--` now get flags-anywhere parsing too (`snip-prev -d 'x' -- cmd` lost `-d`), pinned by `TestFreeArgsWithDoubleDash`.
