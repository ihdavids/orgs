package kanban

// The css of -html. cssLayout is the page; a theme is only the custom
// properties in front of it (colours, faces, corners), set three times like
// the documentation themes' palettes: bare :root for light, the reader's dark
// setting unless the page was switched to light, and the page switched to
// dark - so the switch wins both ways. Fonts are the reader's own: a page
// that is mailed has nowhere to load one from.

const cssDefault = `
:root {
  --bg: #f6f7f9; --surface: #eceef2; --card: #ffffff; --text: #1f2328; --dim: #5f6773; --faint: #9aa1ab;
  --line: #dde1e6; --chip: #e9ecf0; --accent: #3e63dd; --danger: #d93036; --ok: #218358;
  --shadow: 0 1px 2px rgba(16,24,40,.06), 0 1px 3px rgba(16,24,40,.08);
  --font: system-ui, -apple-system, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
  --font-title: var(--font); --radius: 10px; --col-w: 300px;
}
@media (prefers-color-scheme: dark) { :root:not([data-theme="light"]) {
  --bg: #111418; --surface: #181c22; --card: #1f252d; --text: #e6e9ef; --dim: #9aa3ae; --faint: #5b6470;
  --line: #2a313b; --chip: #2b333e; --accent: #7c9cf5; --danger: #f2555a; --ok: #3dd68c;
  --shadow: 0 1px 2px rgba(0,0,0,.4);
} }
:root[data-theme="dark"] {
  --bg: #111418; --surface: #181c22; --card: #1f252d; --text: #e6e9ef; --dim: #9aa3ae; --faint: #5b6470;
  --line: #2a313b; --chip: #2b333e; --accent: #7c9cf5; --danger: #f2555a; --ok: #3dd68c;
  --shadow: 0 1px 2px rgba(0,0,0,.4);
}
`

const cssPaper = `
:root {
  --bg: #f4efe6; --surface: #ebe4d6; --card: #fffdf8; --text: #2b2620; --dim: #6d6356; --faint: #a49a8b;
  --line: #d9cfbd; --chip: #efe7d8; --accent: #8a4b2a; --danger: #b3261e; --ok: #4a7a3a;
  --shadow: 0 1px 0 rgba(80,60,30,.08), 0 2px 6px rgba(80,60,30,.06);
  --font: "Iowan Old Style", "Palatino Linotype", Palatino, Charter, Georgia, serif;
  --font-title: "Iowan Old Style", "Palatino Linotype", Palatino, Georgia, serif; --radius: 4px; --col-w: 310px;
}
@media (prefers-color-scheme: dark) { :root:not([data-theme="light"]) {
  --bg: #1d1a16; --surface: #24201b; --card: #2c2721; --text: #ece3d3; --dim: #b3a792; --faint: #7a6f5f;
  --line: #3a332a; --chip: #352f27; --accent: #e0a37a; --danger: #ef6b5e; --ok: #93c27f;
  --shadow: 0 1px 2px rgba(0,0,0,.45);
} }
:root[data-theme="dark"] {
  --bg: #1d1a16; --surface: #24201b; --card: #2c2721; --text: #ece3d3; --dim: #b3a792; --faint: #7a6f5f;
  --line: #3a332a; --chip: #352f27; --accent: #e0a37a; --danger: #ef6b5e; --ok: #93c27f;
  --shadow: 0 1px 2px rgba(0,0,0,.45);
}
.theme-paper h1 { font-style: italic; font-weight: 600; }
.theme-paper .card { border: 1px solid var(--line); border-top: 3px solid var(--hc); }
.theme-paper .col-head { text-transform: none; letter-spacing: 0; font-size: 1.05rem; font-family: var(--font-title); }
`

const cssMidnight = `
:root {
  --bg: #eef1f8; --surface: #e2e7f2; --card: #ffffff; --text: #141a2e; --dim: #56607a; --faint: #98a1b7;
  --line: #d3d9e8; --chip: #e6eaf5; --accent: #5b5bd6; --danger: #e5484d; --ok: #12a594;
  --shadow: 0 2px 8px rgba(20,26,46,.08);
  --font: "Inter", system-ui, -apple-system, "Segoe UI", Roboto, sans-serif;
  --font-title: var(--font); --radius: 14px; --col-w: 290px;
}
@media (prefers-color-scheme: dark) { :root:not([data-theme="light"]) {
  --bg: #0b1020; --surface: #11182d; --card: #172038; --text: #e8ecfb; --dim: #a3acc9; --faint: #5f6a8c;
  --line: #222c48; --chip: #222d4c; --accent: #8e8cff; --danger: #ff6b81; --ok: #3ee0c4;
  --shadow: 0 0 0 1px rgba(142,140,255,.08), 0 6px 18px rgba(0,0,0,.45);
} }
:root[data-theme="dark"] {
  --bg: #0b1020; --surface: #11182d; --card: #172038; --text: #e8ecfb; --dim: #a3acc9; --faint: #5f6a8c;
  --line: #222c48; --chip: #222d4c; --accent: #8e8cff; --danger: #ff6b81; --ok: #3ee0c4;
  --shadow: 0 0 0 1px rgba(142,140,255,.08), 0 6px 18px rgba(0,0,0,.45);
}
.theme-midnight .column { background: linear-gradient(180deg, var(--surface), transparent 85%); }
.theme-midnight .col-head .dot { box-shadow: 0 0 10px var(--col); }
.theme-midnight .when, .theme-midnight .file, .theme-midnight .meta code { font-family: ui-monospace, "SF Mono", Menlo, Consolas, monospace; font-size: .74rem; }
`

const cssLayout = `
*, *::before, *::after { box-sizing: border-box; }
html { -webkit-text-size-adjust: 100%; }
body { margin: 0; background: var(--bg); color: var(--text); font: 15px/1.45 var(--font); }
a { color: var(--accent); }
code { font-family: ui-monospace, "SF Mono", Menlo, Consolas, monospace; font-size: .86em; background: var(--chip); padding: .05em .3em; border-radius: 4px; }
.card { --hc: var(--hc-l); }
@media (prefers-color-scheme: dark) { :root:not([data-theme="light"]) .card { --hc: var(--hc-d); } }
:root[data-theme="dark"] .card { --hc: var(--hc-d); }

.bar { position: sticky; top: 0; z-index: 5; display: flex; gap: 16px; align-items: center; justify-content: space-between;
  flex-wrap: wrap; padding: 14px 20px; background: var(--bg); border-bottom: 1px solid var(--line); }
.bar h1 { margin: 0; font: 650 1.35rem/1.2 var(--font-title); }
.meta { margin: 3px 0 0; color: var(--dim); font-size: .82rem; display: flex; flex-wrap: wrap; gap: 0 .35em; }
.meta code { max-width: 40ch; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; display: inline-block; vertical-align: bottom; }
.tools { display: flex; gap: 8px; align-items: center; }
#filter { width: 220px; max-width: 60vw; padding: 7px 11px; border: 1px solid var(--line); border-radius: 999px;
  background: var(--card); color: var(--text); font: inherit; font-size: .9rem; }
#filter:focus { outline: 2px solid var(--accent); outline-offset: 1px; }
#mode { width: 34px; height: 34px; border-radius: 50%; border: 1px solid var(--line); background: var(--card); color: var(--text);
  font-size: 1rem; cursor: pointer; }
.problem, .note { margin: 12px 20px 0; color: var(--dim); font-size: .88rem; }
.problem { color: var(--danger); }

.board { display: flex; gap: 14px; align-items: flex-start; padding: 16px 20px 28px; overflow-x: auto; scroll-snap-type: x proximity; scroll-padding-inline: 20px; }
.column { flex: 0 0 var(--col-w); width: var(--col-w); background: var(--surface); border-radius: var(--radius); padding: 10px;
  scroll-snap-align: start; border-top: 3px solid var(--col); }
.col-head { display: flex; align-items: center; gap: 8px; margin: 2px 4px 10px; font: 600 .78rem/1.3 var(--font);
  text-transform: uppercase; letter-spacing: .06em; color: var(--dim); cursor: pointer; user-select: none; }
.col-head .dot { width: 9px; height: 9px; border-radius: 50%; background: var(--col); flex: none; }
.col-head .col-title { color: var(--text); }
.col-head .n { margin-left: auto; background: var(--chip); color: var(--dim); border-radius: 999px; padding: 1px 8px; font-size: .74rem; letter-spacing: 0; }
.col-head .limit { font-size: .74rem; letter-spacing: 0; }
.col-head .limit.over { color: var(--danger); font-weight: 700; }
.column.folded { flex-basis: 46px; width: 46px; padding: 10px 6px; }
.column.folded .cards { display: none; }
.column.folded .col-head { writing-mode: vertical-rl; margin: 2px auto; gap: 10px; }
.column.folded .col-head .n { margin: 0; }
.cards { display: flex; flex-direction: column; gap: 9px; }
.empty { margin: 4px; color: var(--faint); font-size: .85rem; font-style: italic; }

.card { position: relative; background: var(--card); border-radius: calc(var(--radius) - 3px); padding: 11px 12px 9px 13px;
  box-shadow: var(--shadow); border-top: 3px solid var(--hc); overflow: hidden; }
.card::before { content: ""; position: absolute; left: 0; top: 0; bottom: 0; width: 4px; background: var(--stripe, transparent); }
.card.has-more { cursor: pointer; }
.card:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
.card .title { margin: 0; font: 600 .95rem/1.35 var(--font-title); overflow-wrap: anywhere; }
.pri { display: inline-block; min-width: 1.45em; text-align: center; border-radius: 5px; font: 700 .7rem/1.55 var(--font);
  margin-right: 6px; vertical-align: 1px; padding: 0 .3em; }
.clock { color: var(--ok); margin-right: 5px; }
.labels { display: flex; flex-wrap: wrap; gap: 4px; margin-bottom: 7px; }
.label { border-radius: 999px; font-size: .7rem; font-weight: 600; padding: 1px 8px; }
.chips { display: flex; flex-wrap: wrap; gap: 4px 6px; margin-top: 7px; }
.chip { font-size: .74rem; color: var(--dim); }
.chip.kw { background: var(--chip); border-radius: 4px; padding: 0 6px; font-weight: 600; letter-spacing: .02em; }
.chip.badge { border: 1px solid var(--line); border-radius: 4px; padding: 0 5px; }
.chip.tag { color: var(--tag); }
.progress { position: relative; height: 18px; margin-top: 8px; background: var(--chip); border-radius: 999px; overflow: hidden; }
.progress span { position: absolute; inset: 0 auto 0 0; background: color-mix(in srgb, var(--accent) 35%, transparent); border-radius: 999px; }
.progress.complete span { background: color-mix(in srgb, var(--ok) 40%, transparent); }
.progress em { position: relative; display: block; font: 600 .7rem/18px var(--font); font-style: normal; padding-left: 8px; color: var(--text); }
.body { margin-top: 7px; font-size: .84rem; color: var(--dim); overflow-wrap: anywhere; }
.card:not(.open) .body { display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; }
.card:not(.open) .body > * { display: inline; margin: 0; padding: 0; }
.card:not(.open) .body li { list-style: none; }
.card:not(.open) .body > * + *::before { content: " "; }
.card.open .body { color: var(--text); margin-top: 9px; padding-top: 8px; border-top: 1px dashed var(--line); }
.body p { margin: 0 0 .5em; }
.body ul { margin: 0 0 .5em; padding-left: 1.2em; }
.body .cut { color: var(--faint); }
.body .link { text-decoration: underline dotted; }
.more { display: none; margin: 8px 0 0; font-size: .84rem; }
.card.open .more { display: block; }
.checks { list-style: none; padding: 0 !important; }
.checks li { display: flex; gap: 7px; align-items: baseline; margin: 2px 0; }
.checks .box { flex: none; width: 13px; height: 13px; border: 1.5px solid var(--faint); border-radius: 3px; font-size: .65rem; line-height: 10px; text-align: center; color: var(--ok); }
.checks .done { color: var(--dim); text-decoration: line-through; }
.checks .done .box { border-color: var(--ok); }
.card > footer { display: flex; gap: 10px; align-items: center; margin-top: 8px; font-size: .76rem; color: var(--dim); }
.when.overdue { color: var(--danger); font-weight: 700; }
.file { margin-left: auto; color: var(--faint); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 50%; }

.list { padding: 8px 20px 28px; max-width: 1100px; }
.group { margin: 14px 0 22px; }
.group .col-head { font-size: .82rem; }
.group.folded table { display: none; }
.group table { width: 100%; border-collapse: collapse; background: var(--card); border-radius: var(--radius); overflow: hidden; box-shadow: var(--shadow); border-top: 3px solid var(--col); }
.group th { text-align: left; font-size: .72rem; text-transform: uppercase; letter-spacing: .05em; color: var(--dim); padding: 8px 10px; border-bottom: 1px solid var(--line); }
.group td { padding: 7px 10px; border-bottom: 1px solid var(--line); font-size: .88rem; vertical-align: top; }
.group tr:last-child td { border-bottom: 0; }
.group .item { font-weight: 600; }
.made { padding: 0 20px 22px; color: var(--faint); font-size: .75rem; }
.made a { color: inherit; }

@media (max-width: 640px) {
  .bar { padding: 10px 16px; }
  .board { padding: 12px 16px 24px; gap: 10px; }
  .column { --col-w: 84vw; }
  #filter { width: 100%; max-width: none; }
  .tools { flex: 1 1 100%; }
  .list { padding: 8px 16px 24px; }
  .group table { display: block; overflow-x: auto; }
}

@media print {
  :root { --bg: #fff; --surface: #fff; --card: #fff; --text: #000; --dim: #444; --faint: #777; --line: #bbb; --chip: #eee; --shadow: none; }
  .card { --hc: var(--hc-l) !important; border: 1px solid #ccc; }
  .bar { position: static; border: 0; }
  .tools, .made { display: none; }
  .board { flex-wrap: wrap; overflow: visible; }
  .column { break-inside: avoid-page; border: 1px solid #ccc; }
  .card { break-inside: avoid; }
  .card .more { display: block; }
  .card .body { display: block !important; -webkit-line-clamp: none; }
  .column.folded { flex-basis: var(--col-w); width: var(--col-w); }
  .column.folded .cards { display: flex; }
  .column.folded .col-head { writing-mode: horizontal-tb; }
}
`
