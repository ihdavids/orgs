# Notes on `orgs cap`

Loaded when working in this directory. References like **Traps: line edits** point at the Traps list in the root `CLAUDE.md`.

## `orgs cap`, the template as the form

`cmd/oc/commands/capture/` once asked headline then body regardless of template, so drawer templates did nothing at a prompt. Now the template **is** the form (`form.go`): drawn and coloured as org, cursor in the current hole, tab to the next - editing a document, as org-capture does.

1. **Unfilled holes are drawn as holes** (`{{project}}` in `orghl`'s placeholder colour), not blank space, which would look finished.
2. **Nothing is dropped while editing**: the editor uses `CapSubstitute` (unanswered left as written) and keeps every line; `FillCapTemplate` (unanswered writes nothing, placeholder-only lines dropped) runs once on the way out - otherwise a line vanishes when cleared.
3. **Preview = exactly what's filed**, stars and indent included: `InsertEntryUsingTemplate` writes the headline at target level + 1 and indents every content line.
4. **The caret is a span**: insert a sentinel, colour, split the sentinel's span so the cursor character is reverse video *inside* the org colour (colouring around a caret loses colour at the join).

Headline and tags are holes in the same list as the template's. Non-interactively (pipe, `-json`, cron): no form, defaults stand, `-head`, `-cont`, `-tags`, `-set name=value` fill values, and a missing headline on an `entry` is a refusal. `-dry-run` works because writes go through `SendReceivePost`.
