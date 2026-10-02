# Notes on `orgs pres`

Loaded when working in this directory. References like **Traps: column-zero drawers** point at the Traps list in the root `CLAUDE.md`.

## A terminal renderer of the slides decks

`orgs pres` is a fifth renderer for `internal/app/orgs/plugs/slides` (see its CLAUDE.md). It calls `slides.BuildDeck` with `Conf{Prefix: "TERM"}`, so slide level, notes, `:noexport:`, `:FRAGMENT:`, `#+ATTR_SLIDE: :frag`, `:BACKGROUND:`/`ReadInk` and `:ALIGN:` mean what they mean in reveal. **Don't reimplement deck rules here**; fix them in `slides` and every renderer gets them. The import is legal: `slides` doesn't import `internal/app/orgs` (see **Import cycle constraint**).

- **Parsed locally, `NeedsNoServer`.** A talk is a file in front of you; a name that isn't a path is looked up in `ServerSettings.OrgDirs`.
- **Palette = the html slide theme's css custom properties** (`--slide-bg`, `--slide-ink`, `--slide-accent`, `--slide-code-bg`, …) read from `templates/slides_theme_<name>.css` via `slides.LoadTheme`; `rgba()` is blended onto the ground. `@hljs:` maps to a chroma style (`hljsToChroma`). A new theme file works here with no code. `slides.SearchPath` must be set from `TemplatePath` (the server does this in `serve.go`; this command does it itself).
- **Two passes**: `render.go` turns nodes into `[]line` (segments + indent + fragment step) for a column width; `layout.go` places lines on a `canvas` of cells. Transitions blend two canvases, `-print` turns a canvas into ansi, and tests read it as text: one drawing path.
- **Hidden fragments keep their space** (blank, not absent), so revealing a bullet moves nothing. `TestFragmentsStepAndNotesStayOff` pins it.
- **Values and pointers both matched** in every node switch (see **Traps: pointer trap**). A src block's `Parameters` is one string (`"go -n"`), split with `Fields`. go-org's table `ColumnInfo.Align` says "right" for every column, so alignment is guessed from whether a column is mostly numbers.
- **Pictures are half blocks** (`▀`, fg = top pixel, bg = bottom), box-filtered, alpha blended onto the slide's ground. No kitty/iTerm protocol: it has to work in tmux and over ssh.
- **Two windows sync through a temp file** (`syncPath`: `$TMPDIR/orgs-pres-<sha1 of abs path>.pos`, `"idx step"`), polled every 300ms; either window drives. The file is also polled for mtime and reloaded in place.
- **`e` edits**: `$VISUAL`/`$EDITOR` takes the terminal (screen suspended, then reload); `editorTemplate` is a gui launcher that returns at once, and the save is caught by the reload.
- Checking colours: `script -q /dev/null orgs pres -print f.org` forces a tty for ansi; tmux `capture-pane -e` drops cell backgrounds, so don't judge colours from it.
