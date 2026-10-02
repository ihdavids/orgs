# Notes on `orghl`

Loaded when working in this directory. References like **Traps: line edits** point at the Traps list in the root `CLAUDE.md`.

## `orghl`, org colouring for a terminal

`cmd/oc/commands/orghl/`, terminal sibling of `worg/src/codehl.ts`, is a **scanner**: no scope, can't tell multiplication from bold, may miscolour a word. It must never lose a character (`TestSpansAreTheLine`: spans concatenate to the line).

- Returns **spans**, not escapes: one walk renders `ANSI` (listings) or `Tview` (forms), and the capture form puts its cursor inside a span.
- **Per-line `State`**, because source blocks and drawers change line meaning; without it, mid-file screens start a block wrong.
- **Keywords from `GET /status`**, so `TODO buy milk` colours as a task and unknown keywords are plain. The read may fail silently - no reason to refuse a capture.
- **Escape for tview**: `[[links]]` and `[2026-09-30 Wed]` are colour tags to a dynamic-colour TextView; unescaped, a timestamp *disappears*.
