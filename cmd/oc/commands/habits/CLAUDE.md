# Notes on `orgs habits`

Loaded when working in this directory. References like **Traps: line edits** point at the Traps list in the root `CLAUDE.md`.

## `orgs habits`, the tracker at a prompt

`cmd/oc/commands/habits/`: `orgs habits` lists, `show` draws one, `done` ticks, `untick` removes today's tick, `pick` is the fzf picker with the habit beside the list, `-short` is one status-bar line, `-w` redraws as files change. One `/habits` request; writes go to `/status/change` or `/habits/untick`. Tone is in `view.go` with `view_test.go`, mirroring `habits.ts` and its tests.

The six rules are repeated here and **must not drift** (see **Traps: logic said twice**); `view_test.go` pins them. One has drifted, and the fix belongs in worg: `Cadence` says "does not repeat" for a habit with no repeater while `cadence()` in `habits.ts` says "every day". The server reports `Every` as 1 there only to avoid dividing by zero, so ask `Repeater`.

Terminal-specific:

1. **Day states are three characters** (`█` kept, `░` missed, `·` in hand) as well as colours, because colour is off in pipes, under `NO_COLOR`, in status bars and with `-no-color`, and a miss must stay readable. A legend at the foot explains them ("missed" vs "in hand" is why there are three).
2. **The window is trimmed from the oldest end to fit, never sampled**: fetched once at the widest and cut per screen, like worg's `recentDays`. The footer says how many days show.
3. **The default is the tracker, not a picker** (unlike `orgs tables`/`orgs code`): a few habits all fit. In `orgs habits pick`, `ctrl-d` ticks and **reloads** via a hidden `orgs habits lines` so the square fills; `ctrl-u` unticks, because a key that writes needs one that unwrites. Both use `quiet()`: `execute`, not `execute-silent`, output discarded, but a *refusal* waits for a key, since a failed write otherwise looks like an unchanged square. `Binds` is a function so `TestBindingsParse` can run the strings through fzf's parser; a malformed binding stops the picker opening.
4. **The month ruler is drawn once at the foot**: every habit shares the same days (worg draws it per row for lack of space). The pane is a **calendar**, not a strip, because one habit is asked "when": which Tuesdays, whether gaps are weekends.
