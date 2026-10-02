# Notes on `orgs drill`

Loaded when working in this directory. References like **Traps: logic said twice** point at the Traps list in the root `CLAUDE.md`.

## Flashcards in the terminal

The terminal half of worg's Flashcards tab, over the same endpoints (`GET /drill/cards?format=org`, `POST /drill/review`, `GET /ext/drill/decks`). Server notes are in `internal/app/orgs/CLAUDE.md` (Flashcards); worg's in `docs/claude/worg.md`; user guide `docs/drill.org`.

- **`session.go` is `worg/src/drill.ts` said again** (see **Traps: logic said twice**): queues, again pile, limits, card-type cloze/side choice, report. `session_test.go` and `drill.test.ts` pin the same scenarios with the same expected orders; change both.
- **Wire types and the cloze scanner are shared, not copied**: `internal/common/drill.go` (DrillDeck, DrillCard, ...; the server aliases them) and `internal/common/drillcloze.go` (`common.Clozes`, with a wrap callback - html spans on the server, org markup here). Both sides must number clozes identically, because card types hide "the first" or "one at random" by number.
- **`format=org`** makes `/drill/cards` hand back the cards' org text with clozes untouched; `ClozeOrg` rewrites them per phase as `~[...]~` (a code chip) hidden, `*text*` revealed, plain when left showing in the question.
- **Cards are drawn with the slide renderer** through `cmd/oc/commands/pres/api.go` (`RenderOrg`, `RenderInline`, `DrawLine`, `LoadPalette`). A drawing change for slides changes cards too, on purpose.
- **`-resume` is a file** (`$XDG_CACHE_HOME/orgs/drill-session.json`): a process that has exited has to write down where it was. Cards (`json:"-"`) are fetched again and queues pruned of hashes no longer found.
- `Interactive()` gates drilling (stdin and stdout both a terminal); `ls`/`stats`/`cards` are the scriptable parts.
