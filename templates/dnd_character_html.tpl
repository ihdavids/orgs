<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{ sheet.name }} - Character Sheet</title>
<link rel="preconnect" href="https://fonts.googleapis.com">
<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
<link rel="stylesheet" href="https://fonts.googleapis.com/css2?family=Cinzel:wght@500;700&family=EB+Garamond:ital,wght@0,400;0,600;1,400&display=swap">
<style>
:root {
  --ink: #1c1a17;
  --paper: #f4efe4;
  --paper-2: #ece4d5;
  --line: #cbbfa6;
  --edge: #8b7e66;
  --accent: #7b1b1b;
  --accent-2: #b8860b;
  --muted: #6b6355;
  --shadow: 0 1px 3px rgba(28,26,23,.18);
}
* { box-sizing: border-box; }
html { -webkit-text-size-adjust: 100%; }
body {
  margin: 0;
  background: #2b2723;
  color: var(--ink);
  font-family: "EB Garamond", Palatino, "Palatino Linotype", Georgia, serif;
  font-size: 15px;
  line-height: 1.45;
}
/* The sheet is one page tall: the header, the columns and the footer stack
   down a flex column that is at least the height of the window, and the
   columns take whatever the header and footer leave. That is what lets the
   two long boxes - attacks and inventory on one side, spells and features on
   the other - grow into the empty space at the bottom rather than leaving it
   blank. */
.page {
  max-width: 1180px;
  margin: 0 auto;
  padding: 22px;
  background: var(--paper);
  background-image:
    radial-gradient(circle at 20% 10%, rgba(184,134,11,.06), transparent 45%),
    radial-gradient(circle at 85% 70%, rgba(123,27,27,.05), transparent 40%);
  box-shadow: 0 0 40px rgba(0,0,0,.55);
  min-height: 100vh;
  display: flex;
  flex-direction: column;
}
h1, h2, h3, .label, .tile-label, th {
  font-family: Cinzel, "Trajan Pro", Georgia, serif;
  letter-spacing: .06em;
}

/* ---------------- header ---------------- */
header.sheet-head {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-end;
  gap: 18px;
  border-bottom: 3px double var(--accent);
  padding-bottom: 12px;
  margin-bottom: 18px;
}
.name-block { flex: 1 1 320px; }
.char-name {
  font-size: 2.6rem;
  margin: 0;
  line-height: 1;
  color: var(--accent);
  text-shadow: 0 1px 0 rgba(255,255,255,.6);
}
.char-title { color: var(--muted); font-size: 1.05rem; margin-top: 4px; }
.char-sub { color: var(--muted); font-size: .95rem; margin-top: 2px; }
.head-facts { display: flex; flex-wrap: wrap; gap: 10px; }
.fact {
  background: var(--paper-2);
  border: 1px solid var(--line);
  border-radius: 4px;
  padding: 5px 12px;
  min-width: 92px;
  box-shadow: var(--shadow);
}
.fact .label { display: block; font-size: .62rem; text-transform: uppercase; color: var(--muted); }
.fact .value { font-size: 1.05rem; font-weight: 600; }

/* Inspiration is the one fact on the header that is also a control: the DM
   hands it out and the player spends it, so it is a button that writes
   straight back to the org file. Held, it lights up. */
.fact.insp { padding: 0; }
.insp-btn {
  display: block; width: 100%; height: 100%;
  font: inherit; color: inherit; text-align: left; cursor: pointer;
  background: none; border: 0; border-radius: 4px; padding: 5px 12px;
}
.insp-btn:hover { background: #f6f0e2; }
.insp-btn:disabled { cursor: default; }
.insp-btn .value { display: flex; align-items: center; gap: 5px; }
.insp-btn .spark { width: 15px; height: 15px; flex: 0 0 auto; opacity: .3; }
.insp-btn .word { font-size: .92rem; color: var(--muted); font-weight: 400; }
.fact.insp.on { border-color: var(--accent-2); background: #fbf3dc; }
.fact.insp.on .insp-btn .spark { opacity: 1; }
.fact.insp.on .insp-btn .word { color: var(--accent-2); font-weight: 700; }
.fact.insp.busy { opacity: .6; }

/* ---------------- portrait medallion ----------------
   The portrait is an ordinary <img> in a round window with the frame drawn
   over it in svg, rather than an svg <image>: that way an animated gif or
   webp keeps animating, and object-fit does the cropping.

   --fx/--fy are the point of the picture that belongs in the middle of the
   medallion and --zoom is how far in to push. object-position lines that
   point up with the same point of the window, the scale keeps it pinned
   there, and the translate slides it into the centre. */
.portrait {
  --fx: 50%; --fy: 50%; --zoom: 1;
  position: relative;
  flex: 0 0 auto;
  width: 196px; height: 196px;
  margin: 0;
  filter: drop-shadow(0 3px 7px rgba(28,26,23,.34));
}
.portrait-well {
  position: absolute;
  inset: 16.19%;            /* the clear window inside the frame */
  border-radius: 50%;
  overflow: hidden;
  background: #2b2723;
}
.portrait-well img {
  display: block;
  width: 100%; height: 100%;
  object-fit: cover;
  object-position: var(--fx) var(--fy);
  transform-origin: var(--fx) var(--fy);
  transform: translate(calc(50% - var(--fx)), calc(50% - var(--fy))) scale(var(--zoom));
}
/* a little inward shading so the picture sits under the frame rather than
   on top of it */
.portrait-well::after {
  content: "";
  position: absolute; inset: 0;
  border-radius: 50%;
  background: radial-gradient(circle at 50% 38%, rgba(244,239,228,0) 54%, rgba(43,39,35,.3) 100%);
  box-shadow: inset 0 0 15px rgba(28,26,23,.5), inset 0 0 2px rgba(28,26,23,.8);
  pointer-events: none;
}
.portrait-ring {
  position: absolute; inset: 0;
  width: 100%; height: 100%;
  overflow: visible;
  pointer-events: none;
}
.ring-band { fill: none; stroke: url(#ringBand); stroke-width: 22; }
.ring-edge { fill: none; stroke: var(--accent-2); stroke-width: 1.1; opacity: .9; }
.ring-hair { fill: none; stroke: var(--edge); stroke-width: .7; opacity: .55; }
.xp-track { fill: none; stroke: rgba(28,26,23,.15); stroke-width: 2.4; }
.xp-fill {
  fill: none; stroke: var(--accent-2); stroke-width: 2.4; stroke-linecap: round;
}
.ring-name {
  font-family: Cinzel, "Trajan Pro", Georgia, serif;
  font-weight: 700; font-size: 13px; letter-spacing: .1em;
  fill: var(--accent);
}
.ring-xp {
  font-family: Cinzel, "Trajan Pro", Georgia, serif;
  font-weight: 500; font-size: 9.5px; letter-spacing: .14em;
  fill: #7a5c12;
}
.ring-gem { fill: var(--accent-2); }
.ring-gem-core { fill: var(--accent); }

.sheet-head.has-portrait { align-items: center; }
.has-portrait .char-title {
  font-family: Cinzel, "Trajan Pro", Georgia, serif;
  font-size: 1.55rem; line-height: 1.15; margin-top: 0;
  color: var(--accent);
  text-shadow: 0 1px 0 rgba(255,255,255,.6);
}
@media (max-width: 700px) {
  .portrait { width: 158px; height: 158px; }
}
.sr-only {
  position: absolute; width: 1px; height: 1px;
  margin: -1px; padding: 0; overflow: hidden;
  clip: rect(0 0 0 0); clip-path: inset(50%); white-space: nowrap;
}

/* ---------------- layout ---------------- */
.columns {
  display: grid;
  grid-template-columns: 250px 1fr 1fr;
  gap: 18px;
  /* stretch, not start: every column runs the full height of the row, so the
     boxes marked .grow inside them have somewhere to grow into. */
  align-items: stretch;
  flex: 1 1 auto;
  min-height: 0;
}
@media (max-width: 1000px) { .columns { grid-template-columns: 1fr 1fr; } }
@media (max-width: 700px)  { .columns { grid-template-columns: 1fr; } }
.col { display: flex; flex-direction: column; gap: 16px; min-width: 0; min-height: 0; }

/* A box that takes the rest of its column. Both of them do, so the two end
   on the same line and the page reads as one sheet rather than two ragged
   ones. What overflows scrolls inside the box; the page itself does not
   grow past the window because of it. */
.box.grow {
  flex: 1 1 0;
  min-height: 320px;
  display: flex;
  flex-direction: column;
}
.box.grow > .tabpane { min-height: 0; }
.box.grow.has-tabs > .tabpane.on {
  flex: 1 1 auto;
  overflow-y: auto;
  overscroll-behavior: contain;
  padding-right: 4px;
}
/* Without the script there are no tabs, so the sections simply stack and the
   whole box scrolls instead. */
.box.grow:not(.has-tabs) { overflow-y: auto; }
/* On one column there is no empty space beside anything to fill, and a short
   scrolling box inside an already long page is worse than no box at all. */
@media (max-width: 700px) {
  .box.grow, .box.grow.has-tabs > .tabpane.on {
    display: block; flex: 0 0 auto; min-height: 0; overflow: visible;
  }
}

.box {
  background: var(--paper-2);
  border: 1px solid var(--line);
  border-radius: 5px;
  box-shadow: var(--shadow);
  padding: 10px 12px 12px;
}
.box > h2 {
  font-size: .74rem;
  text-transform: uppercase;
  margin: 0 0 8px;
  padding-bottom: 5px;
  border-bottom: 1px solid var(--line);
  color: var(--accent);
}

/* ---------------- tabbed sections ----------------
   Several sections share one box and are shown a tab at a time. Until the
   script has built the bar the panes are simply stacked, each under its own
   heading, which is what a sheet with no script keeps. */
.box.tabbed > .tabpane > h2 {
  font-size: .74rem;
  text-transform: uppercase;
  margin: 0 0 8px;
  padding-bottom: 5px;
  border-bottom: 1px solid var(--line);
  color: var(--accent);
}
.box.tabbed > .tabpane + .tabpane { margin-top: 14px; }
.box.has-tabs > .tabpane { display: none; }
.box.has-tabs > .tabpane.on { display: block; }
.box.has-tabs > .tabpane + .tabpane { margin-top: 0; }
/* the tab bar carries the section name, so the headings stand down */
.box.has-tabs > .tabpane > h2 { display: none; }
.tabbar {
  display: flex; flex-wrap: wrap; gap: 3px;
  margin: -2px 0 10px;
  border-bottom: 1px solid var(--line);
}
.tabbtn {
  font-family: Cinzel, "Trajan Pro", Georgia, serif;
  font-size: .68rem; text-transform: uppercase; letter-spacing: .06em;
  border: 1px solid transparent; border-bottom: 0;
  border-radius: 4px 4px 0 0;
  background: none; color: var(--muted);
  padding: 4px 10px 5px; margin-bottom: -1px; cursor: pointer;
}
.tabbtn:hover { color: var(--accent); }
.tabbtn.on {
  background: var(--paper); border-color: var(--line);
  color: var(--accent); font-weight: 700;
}
.tabbtn:focus-visible { outline: 2px solid var(--accent-2); outline-offset: -2px; }

/* ---------------- abilities ---------------- */
.abilities { display: grid; grid-template-columns: repeat(3, 1fr); gap: 8px; }
@media (max-width: 1000px) { .abilities { grid-template-columns: repeat(6, 1fr); } }
@media (max-width: 600px)  { .abilities { grid-template-columns: repeat(3, 1fr); } }
.ability {
  background: var(--paper);
  border: 2px solid var(--line);
  border-radius: 10px;
  text-align: center;
  padding: 8px 2px 4px;
  position: relative;
}
.ability .tile-label { font-size: .6rem; text-transform: uppercase; color: var(--muted); }
.ability .mod { font-size: 1.7rem; font-weight: 700; line-height: 1.05; }
.ability .score {
  display: inline-block;
  margin-top: 2px;
  min-width: 34px;
  border: 1px solid var(--line);
  border-radius: 12px;
  background: var(--paper-2);
  font-size: .8rem;
  padding: 0 6px;
}

/* ---------------- lists ---------------- */
.rows { display: flex; flex-direction: column; gap: 2px; }
.row {
  display: flex;
  align-items: baseline;
  gap: 7px;
  padding: 1px 2px;
  border-bottom: 1px dotted rgba(139,126,102,.45);
  font-size: .92rem;
}
.row:last-child { border-bottom: 0; }
.pip {
  width: 11px; height: 11px;
  border: 1px solid var(--muted);
  border-radius: 50%;
  flex: 0 0 auto;
  background: var(--paper);
}
.pip.on { background: var(--ink); }
.pip.exp { background: var(--accent-2); box-shadow: inset 0 0 0 2px var(--paper); }
.row .val { width: 2.4em; text-align: right; font-weight: 600; }
.row .abbr { color: var(--muted); font-size: .72rem; width: 2.6em; }
.row .nm { flex: 1; }

/* A .rows.facts list holds words where an ordinary row holds a modifier, so
   the value column sizes to what is in it and wraps rather than being clipped
   to the 2.4em a "+5" needs. "Magenta" and "Chestnut brown" are eye and hair
   colours, and both are longer than any number the sheet ever puts here. */
.rows.facts .row { align-items: flex-start; }
.rows.facts .row .nm { flex: 0 0 auto; }
.rows.facts .row .val {
  width: auto;
  flex: 1 1 auto;
  min-width: 2.4em;
  text-align: right;
  overflow-wrap: anywhere;
}

/* A subheading inside a section, for a block that used to be a tab of its
   own and now sits at the foot of another one. */
h3.subhead {
  font-size: .74rem; text-transform: uppercase; color: var(--accent);
  border-bottom: 1px solid var(--line);
  margin: 14px 0 5px; padding-bottom: 2px;
}

/* ---------------- combat tiles ---------------- */
.combat { display: grid; grid-template-columns: repeat(3, 1fr); gap: 10px; }
.tile {
  background: var(--paper);
  border: 2px solid var(--line);
  border-radius: 6px;
  text-align: center;
  padding: 6px 4px;
}
.tile .tile-label { display: block; font-size: .58rem; text-transform: uppercase; color: var(--muted); }
.tile .big { font-size: 1.5rem; font-weight: 700; line-height: 1.15; }
.tile .sub { font-size: .68rem; color: var(--muted); }
.shield {
  border-radius: 6px 6px 26px 26px / 6px 6px 34px 34px;
  border-color: var(--accent);
}
/* The hit point bar, with the number you are on at one end of it and the
   number you started with at the other, both on the bar's own line so the
   pair reads as the fraction it is. */
.hp-gauge { display: flex; align-items: center; gap: 8px; margin-top: 6px; }
.hp-gauge .hp-now, .hp-gauge .hp-max {
  font-family: Cinzel, Georgia, serif; font-weight: 700;
  font-variant-numeric: tabular-nums; line-height: 1;
  flex: 0 0 auto; min-width: 2.2em;
}
.hp-gauge .hp-now { font-size: 1.15rem; text-align: right; }
.hp-gauge .hp-max { font-size: .95rem; color: var(--muted); text-align: left; }
.hp-bar {
  flex: 1 1 auto; min-width: 0;
  height: 9px; border-radius: 5px; background: #d8cbb2;
  border: 1px solid var(--line); overflow: hidden;
}
/* Green while you are hale, and down through yellow and orange to red as the
   bar empties, so how much trouble you are in is the colour rather than the
   arithmetic. The band is worked out by the engine (HealthLevel), so the
   exported page and the live one always agree. */
.hp-fill { height: 100%; background: linear-gradient(90deg, #3f6b2a, #5d8f33); }
.hp-fill.hurt     { background: linear-gradient(90deg, #97891c, #c0ac26); }
.hp-fill.bloodied { background: linear-gradient(90deg, #b06a12, #d98a1c); }
.hp-fill.dying    { background: linear-gradient(90deg, #7b1b1b, #a33); }
/* The number you are on takes the same colour as the bar under it. */
.hp-now.hale { color: #3f6b2a; }
.hp-now.hurt { color: #857a15; }
.hp-now.bloodied { color: #a3620f; }
.hp-now.dying { color: var(--accent); }

/* ---------------- tables ---------------- */
table { width: 100%; border-collapse: collapse; font-size: .88rem; }
th {
  text-align: left; font-size: .6rem; text-transform: uppercase;
  color: var(--muted); border-bottom: 1px solid var(--line); padding: 2px 4px;
  font-weight: 600;
}
td { padding: 2px 4px; border-bottom: 1px dotted rgba(139,126,102,.4); vertical-align: top; }
tr:last-child td { border-bottom: 0; }
td.num, th.num { text-align: right; white-space: nowrap; }
.table-wrap { overflow-x: auto; }

/* ---------------- inventory ----------------
   The equipment box is live: things are added, used, dropped and packed away
   from here and the org file behind the sheet is rewritten. Everything is
   drawn from the same inventory the server computed, so the printed page and
   the page you are clicking on agree. */
.enc { margin-bottom: 8px; }
.enc-line {
  display: flex; align-items: baseline; gap: 8px; flex-wrap: wrap;
  font-size: .84rem;
}
.enc-line .wt { font-weight: 700; font-size: 1.05rem; }
.enc-line .cap { color: var(--muted); }
.enc-badge {
  margin-left: auto;
  font-family: Cinzel, Georgia, serif;
  font-size: .58rem; text-transform: uppercase; letter-spacing: .08em;
  border: 1px solid var(--line); border-radius: 10px;
  padding: 1px 8px; background: var(--paper); color: var(--muted);
  white-space: nowrap;
}
.enc-badge.encumbered { border-color: var(--accent-2); color: #7a5a08; background: #f6ecd2; }
.enc-badge.heavy { border-color: var(--accent); color: var(--accent); background: #f6e4e1; }
.enc-badge.over { border-color: var(--accent); color: var(--paper); background: var(--accent); }
.enc-bar {
  height: 7px; margin-top: 5px; border-radius: 4px;
  background: #d8cbb2; border: 1px solid var(--line); overflow: hidden;
  position: relative;
}
.enc-fill { height: 100%; background: linear-gradient(90deg, #6f7b4b, #98a05a); }
.enc-fill.encumbered { background: linear-gradient(90deg, #b8860b, #d0a12a); }
.enc-fill.heavy, .enc-fill.over { background: linear-gradient(90deg, #7b1b1b, #a33); }
/* the two variant thresholds, drawn as nicks in the bar */
.enc-mark { position: absolute; top: 0; bottom: 0; width: 1px; background: rgba(28,26,23,.4); }
.enc-note { font-size: .68rem; color: var(--muted); margin-top: 3px; }

.inv-tabs {
  display: flex; flex-wrap: wrap; gap: 4px;
  border-bottom: 1px solid var(--line); margin-bottom: 6px;
}
.inv-tab {
  font-family: Cinzel, Georgia, serif;
  font-size: .6rem; text-transform: uppercase; letter-spacing: .05em;
  border: 1px solid var(--line); border-bottom: 0;
  border-radius: 4px 4px 0 0;
  background: var(--paper); color: var(--muted);
  padding: 3px 8px; cursor: pointer; margin-bottom: -1px;
}
.inv-tab:hover { color: var(--ink); }
.inv-tab.on { background: var(--paper-2); color: var(--accent); border-color: var(--edge); }
.inv-tab .n { color: var(--muted); font-weight: 400; margin-left: 4px; }
.inv-tab.over .n { color: var(--accent); font-weight: 700; }
.inv-pane { display: none; }
.inv-pane.on { display: block; }
.inv-pane-name { display: none; }
.inv-cap { font-size: .68rem; color: var(--muted); margin: 0 0 4px; }
.inv-cap b { color: var(--ink); font-weight: 600; }
.inv-cap.over b { color: var(--accent); }
.inv-empty { font-size: .8rem; color: var(--muted); font-style: italic; padding: 4px 2px 6px; }

.inv-table td { vertical-align: middle; }
.inv-table .qty { font-weight: 700; }
.inv-name .worn { color: var(--accent-2); }
/* The Worn column is a toggle: a click puts something on or takes it off.
   Only what can actually be worn or wielded gets a button, so a coil of rope
   is a blank cell rather than a control that does nothing. Off is drawn as an
   empty ring - faint enough to stay out of the way of the ticks beside it,
   solid enough to look like somewhere to click. */
.inv-worn { width: 1%; white-space: nowrap; }
.wearbtn {
  font: inherit; font-size: .8rem; line-height: 1;
  border: 1px solid transparent; border-radius: 4px;
  background: none; color: var(--line);
  padding: 1px 5px; cursor: pointer;
}
.wearbtn:hover { color: var(--muted); border-color: var(--line); background: #fbf7ee; }
.wearbtn.on { color: var(--accent-2); }
.wearbtn.on:hover { color: var(--accent); }
.wearbtn:focus-visible { outline: 2px solid var(--accent-2); outline-offset: -1px; }
.inv-actions { text-align: right; white-space: nowrap; width: 1%; }
.ib {
  font-family: inherit; font-size: .72rem;
  border: 1px solid var(--line); border-radius: 4px;
  background: var(--paper); color: var(--muted);
  padding: 0 6px; margin-left: 3px; cursor: pointer; line-height: 1.5;
}
.ib:hover { color: var(--ink); border-color: var(--edge); background: #fbf7ee; }
.ib.use:hover { color: #5a6b2a; }
.ib.drop:hover { color: var(--accent); }
.inv-foot {
  display: flex; align-items: center; gap: 8px; flex-wrap: wrap; margin-top: 8px;
}
.inv-add {
  font-family: Cinzel, Georgia, serif; font-size: .62rem;
  text-transform: uppercase; letter-spacing: .06em;
  border: 1px solid var(--edge); border-radius: 4px;
  background: var(--paper); color: var(--accent);
  padding: 3px 10px; cursor: pointer;
}
.inv-add:hover { background: #fbf7ee; }
.inv-msg { font-size: .7rem; color: var(--muted); }
.inv-err { font-size: .72rem; color: var(--accent); }

/* the coin panel. Money is the same kind of live section the inventory is,
   so it borrows the inventory's buttons and message line and only adds what
   coin needs: the purse total, the five denominations, and the spend box. */
.coin-head {
  display: flex; align-items: baseline; gap: 8px; flex-wrap: wrap;
  margin-bottom: 6px;
}
.coin-total { font-weight: 700; font-size: 1.15rem; }
.coin-sub { color: var(--muted); font-size: .78rem; }
.coin-table td { vertical-align: middle; }
.coin-table .num { font-weight: 700; }
.coin-table td.num.dim { font-weight: 400; color: var(--muted); }
.coin-table tr.empty td { color: var(--muted); }
.coin-table tr.empty .num { font-weight: 400; }
/* a little disc of the right metal in front of each denomination */
.coin-pip {
  display: inline-block; width: 9px; height: 9px; margin-right: 6px;
  border-radius: 50%; border: 1px solid rgba(28,26,23,.35);
  vertical-align: baseline;
}
.coin-pip.cp { background: linear-gradient(140deg, #c9763c, #8a4a22); }
.coin-pip.sp { background: linear-gradient(140deg, #dcdcd6, #9d9d96); }
.coin-pip.ep { background: linear-gradient(140deg, #d8e2d2, #8fa38c); }
.coin-pip.gp { background: linear-gradient(140deg, #e8c34a, #a9801a); }
.coin-pip.pp { background: linear-gradient(140deg, #eef1f4, #a8b3bd); }
.coin-spend {
  display: flex; align-items: center; gap: 6px; flex-wrap: wrap; margin-top: 8px;
}
.coin-spend .amount { width: 8.5rem; }
.coin-spend .why { flex: 1 1 8rem; min-width: 6rem; }
.coin-hint { font-size: .68rem; color: var(--muted); margin-top: 4px; }
.coin-log { margin-top: 8px; border-top: 1px solid var(--line); padding-top: 6px; }
.coin-log h4 {
  font-family: Cinzel, Georgia, serif; font-size: .6rem; text-transform: uppercase;
  letter-spacing: .06em; color: var(--muted); margin: 0 0 4px;
}
.coin-log ul { list-style: none; margin: 0; padding: 0; }
.coin-log li {
  display: flex; gap: 6px; align-items: baseline;
  font-size: .74rem; padding: 1px 0;
}
.coin-log li .when { color: var(--muted); font-size: .66rem; white-space: nowrap; }
.coin-log li .what { flex: 1; }
.coin-log li .amt { font-weight: 700; white-space: nowrap; }
.coin-log li.spent .amt { color: var(--accent); }
.coin-log li.gained .amt { color: #5a6b2a; }
.coin-set-grid {
  display: grid; grid-template-columns: repeat(5, 1fr); gap: 6px; margin: 4px 0 8px;
}
.coin-set-grid label {
  display: flex; flex-direction: column; gap: 2px;
  font-size: .62rem; text-transform: uppercase; letter-spacing: .05em;
  color: var(--muted);
}

/* ---------------- hit points ----------------
   The line under the bar that says what is between the character and the
   floor, and the three things that can be done about it. Hurt and Heal are
   the two buttons that get pressed in a fight, so they are the ones that
   carry colour. */
.hp-temp {
  display: flex; align-items: baseline; gap: 8px;
  font-size: .78rem; margin-top: 5px;
}
.hp-temp .nm { color: var(--muted); }
.hp-temp .val {
  font-family: Cinzel, Georgia, serif; font-weight: 700;
  color: var(--accent-2); font-size: .95rem;
}
.hp-temp .val.none { color: var(--line); font-weight: 400; }
.hp-ctl { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; margin-top: 7px; }
.hp-ctl .hp-amt {
  width: 62px; flex: 0 0 auto; text-align: center;
  font-family: Cinzel, Georgia, serif; font-size: .9rem; padding: 3px 4px;
}
.hpb {
  font-family: Cinzel, Georgia, serif; font-size: .64rem;
  text-transform: uppercase; letter-spacing: .06em;
  border: 1px solid var(--line); border-radius: 4px;
  background: var(--paper); padding: 4px 11px; cursor: pointer;
}
.hpb:hover { background: #fbf7ee; border-color: currentColor; }
.hpb:disabled { opacity: .5; cursor: default; }
/* Red on Hurt and green on Heal: the colour says which way the number goes,
   so neither button is pressed by mistake in the middle of a fight. */
.hpb.hurt { color: var(--accent); }
.hpb.heal { color: #3f6b2a; }

/* The hit die roller. It sits beside the hit points and is easy to take for a
   number rather than a button, so it says what it is and what rolling it
   costs. */
.hitdie {
  display: inline-flex; align-items: baseline; gap: 6px;
  border: 1px solid var(--line); border-radius: 4px;
  background: var(--paper); padding: 2px 8px 3px; cursor: pointer;
}
.hitdie:hover { border-color: var(--accent); }
.hitdie .hd-label {
  font-family: Cinzel, Georgia, serif; font-size: .58rem;
  text-transform: uppercase; letter-spacing: .06em; color: var(--muted);
}
.hitdie .hd-val {
  font-family: Cinzel, Georgia, serif; font-weight: 700; font-size: .95rem;
}
.hitdie .hd-spent { font-size: .62rem; color: var(--accent); }

/* ---------------- defenses and conditions ----------------
   Damage shrugged off, and whatever is currently wrong with the character.
   The three defenses are told apart by colour rather than by reading the
   label, since at the table they are glanced at rather than read. */
.subhead + .def-rows { margin-top: 4px; }
.def-rows { display: flex; flex-direction: column; gap: 5px; }
.def-row { display: flex; align-items: center; gap: 5px; flex-wrap: wrap; }
.def-kind {
  font-family: Cinzel, Georgia, serif; font-size: .58rem;
  text-transform: uppercase; letter-spacing: .06em;
  width: 76px; flex: 0 0 auto; color: var(--muted);
}
.def-kind.res { color: #1b4f9c; }
.def-kind.imm { color: #1f7a3d; }
.def-kind.vul { color: var(--accent); }
.def-chip {
  font-size: .74rem; border: 1px solid currentColor; border-radius: 10px;
  padding: 1px 9px; white-space: nowrap;
  display: inline-flex; align-items: center; gap: 5px;
}
.def-chip.res { color: #1b4f9c; background: rgba(27,79,156,.07); }
.def-chip.imm { color: #1f7a3d; background: rgba(31,122,61,.07); }
.def-chip.vul { color: var(--accent); background: rgba(155,42,42,.07); }
.def-chip .x {
  border: 0; background: none; cursor: pointer; color: inherit;
  font-size: .9rem; line-height: 1; padding: 0; opacity: .55;
}
.def-chip .x:hover { opacity: 1; }
.def-empty { font-size: .78rem; color: var(--muted); margin: 4px 0 0; }
.def-foot { display: flex; gap: 6px; flex-wrap: wrap; align-items: center; margin-top: 8px; }

/* the conditions that are on, each with its mark and its rules */
.cond-active { display: flex; flex-direction: column; gap: 6px; margin-top: 5px; }
.cond-on {
  border: 1px solid var(--line); border-left: 3px solid var(--accent);
  border-radius: 4px; background: #fbf7ee; padding: 5px 9px;
}
.cond-on b { font-family: Cinzel, Georgia, serif; font-size: .78rem; white-space: nowrap; }
.cond-on .tagline { margin-left: 7px; }
.cond-on p { font-size: .74rem; color: var(--muted); margin: 3px 0 0; }
.cond-on .ico { width: 15px; height: 15px; vertical-align: -2px; margin-right: 5px; }
.cond-on .lvl { margin-left: auto; display: inline-flex; gap: 3px; }
.cond-head { display: flex; align-items: center; gap: 4px; }
.cond-head .x {
  margin-left: auto; border: 0; background: none; cursor: pointer;
  color: var(--muted); font-size: 1rem; line-height: 1; padding: 0 2px;
}
.cond-head .x:hover { color: var(--accent); }

/* the picker: every condition there is, as a grid of marks to tap */
.cond-grid {
  display: grid; grid-template-columns: repeat(auto-fill, minmax(86px, 1fr));
  gap: 6px; overflow-y: auto; margin-top: 8px; flex: 1; min-height: 90px;
}
.cond-pick {
  display: flex; flex-direction: column; align-items: center; gap: 4px;
  border: 1px solid var(--line); border-radius: 5px;
  background: var(--paper); color: var(--muted);
  font-family: inherit; font-size: .62rem; text-align: center;
  padding: 8px 4px 6px; cursor: pointer;
}
.cond-pick:hover { border-color: var(--edge); color: var(--ink); background: #fbf7ee; }
.cond-pick.on {
  border-color: var(--accent); color: var(--accent);
  background: rgba(155,42,42,.08); font-weight: 600;
}
.cond-pick .ico { width: 26px; height: 26px; }
.cond-pick .lv { font-size: .58rem; letter-spacing: .06em; text-transform: uppercase; }
.ico { fill: none; stroke: currentColor; stroke-width: 1.5;
       stroke-linecap: round; stroke-linejoin: round; }
.cond-why { font-size: .74rem; color: var(--muted); margin-top: 8px; min-height: 2.4em; }
.cond-step {
  border: 1px solid var(--line); border-radius: 4px; background: var(--paper);
  color: var(--muted); font: inherit; font-size: .7rem; line-height: 1;
  padding: 1px 6px; cursor: pointer;
}
.cond-step:hover { color: var(--ink); border-color: var(--edge); }

/* the add, move and manage spells boxes, over the sheet. The shell is
   shared: only what goes inside them differs. */
.inv-modal, .sb-modal {
  position: fixed; inset: 0; z-index: 120;
  display: none; align-items: flex-start; justify-content: center;
  padding: 8vh 16px 16px;
  background: rgba(28,26,23,.45);
}
.inv-modal.open, .sb-modal.open { display: flex; }
.inv-card, .sb-card {
  width: min(520px, 100%);
  max-height: 76vh; display: flex; flex-direction: column;
  background: var(--paper);
  border: 1px solid var(--edge); border-radius: 6px;
  box-shadow: 0 18px 50px rgba(0,0,0,.5);
  padding: 12px 14px 14px;
}
.inv-card h3, .sb-card h3 {
  font-family: Cinzel, Georgia, serif; font-size: .78rem; text-transform: uppercase;
  color: var(--accent); margin: 0 0 8px; padding-bottom: 5px;
  border-bottom: 1px solid var(--line);
  display: flex; align-items: center; gap: 8px;
}
.inv-card h3 .x, .sb-card h3 .x {
  margin-left: auto; border: 0; background: none; cursor: pointer;
  color: var(--muted); font-size: 1.1rem; line-height: 1; padding: 0 2px;
}
.inv-field, .sb-field {
  width: 100%; font-family: inherit; font-size: .95rem;
  border: 1px solid var(--line); border-radius: 4px;
  background: #fbf7ee; color: var(--ink); padding: 5px 8px;
}
.inv-field:focus, .sb-field:focus { outline: none; border-color: var(--accent-2); }
/* the groups the add box searches in, a wrapped row of chips under the
   search field */
.inv-filters { display: flex; flex-wrap: wrap; gap: 4px; margin-top: 8px; }
.inv-filter {
  font-family: Cinzel, Georgia, serif; font-size: .58rem;
  text-transform: uppercase; letter-spacing: .06em;
  border: 1px solid var(--line); border-radius: 10px;
  background: var(--paper); color: var(--muted);
  padding: 2px 9px; cursor: pointer;
}
.inv-filter:hover { border-color: var(--edge); color: var(--ink); }
.inv-filter.on {
  background: var(--accent); border-color: var(--accent); color: var(--paper);
}
.inv-results { overflow-y: auto; margin: 8px 0 0; flex: 1; min-height: 60px; }
.inv-hit {
  display: block; width: 100%; text-align: left;
  border: 1px solid transparent; border-radius: 4px;
  background: none; font-family: inherit; color: inherit;
  padding: 4px 6px; cursor: pointer;
}
.inv-hit:hover, .inv-hit.on { background: var(--paper-2); border-color: var(--line); }
.inv-hit .nm { font-size: .92rem; }
.inv-hit .meta { font-size: .68rem; color: var(--muted); }
.inv-hit .own { color: var(--accent-2); }
.inv-hit .box-tag {
  font-size: .58rem; text-transform: uppercase; letter-spacing: .06em;
  border: 1px solid var(--line); border-radius: 8px; padding: 0 5px;
  color: var(--muted); margin-left: 5px;
}
/* ---------------- rarity ----------------
   How rare a thing is, in the colours the ladder is usually drawn in, picked
   dark enough to read as text on parchment. Anything mundane has no rarity
   and keeps the ink colour, so only the notable lines are coloured. */
.rar { font-weight: 600; }
.rar-common { color: #4f5a4a; }
.rar-uncommon { color: #1f7a3d; }
.rar-rare { color: #1b4f9c; }
.rar-very-rare { color: #6b2d9e; }
.rar-legendary { color: #b3600a; }
.rar-artifact { color: #9c1b1b; }
.rar-mythic { color: #a4157a; }
.rar-varies { color: var(--accent-2); }
.rar-tag {
  font-size: .58rem; text-transform: uppercase; letter-spacing: .06em;
  border: 1px solid currentColor; border-radius: 8px; padding: 0 5px;
  margin-left: 5px; white-space: nowrap; font-weight: 600;
}

.inv-row { display: flex; align-items: center; gap: 8px; margin-top: 9px; flex-wrap: wrap; }
.inv-row label { font-size: .66rem; text-transform: uppercase; color: var(--muted); }
.inv-row .inv-field { width: auto; flex: 1; min-width: 120px; }
.inv-row .qty-field { width: 68px; flex: 0 0 auto; }
.inv-go, .sb-go {
  font-family: Cinzel, Georgia, serif; font-size: .66rem;
  text-transform: uppercase; letter-spacing: .06em;
  border: 1px solid var(--accent); border-radius: 4px;
  background: var(--accent); color: var(--paper);
  padding: 5px 14px; cursor: pointer;
}
.inv-go:hover, .sb-go:hover { background: #8f2222; }
.inv-go.plain { background: var(--paper); color: var(--accent); }
.inv-hint, .sb-hint { font-size: .68rem; color: var(--muted); margin-top: 6px; }

/* ---------------- resting ----------------
   The rest walkthrough uses the same card shell as the add item and manage
   spells boxes, one step at a time with a footer that moves through them. */
.rest-modal { position: fixed; inset: 0; z-index: 130;
  display: none; align-items: flex-start; justify-content: center;
  padding: 8vh 16px 16px; background: rgba(28,26,23,.5); }
.rest-modal.open { display: flex; }
.rest-card {
  width: min(560px, 100%); max-height: 80vh; display: flex; flex-direction: column;
  background: var(--paper); border: 1px solid var(--edge); border-radius: 6px;
  box-shadow: 0 18px 50px rgba(0,0,0,.5); padding: 12px 14px 14px;
}
.rest-card h3 {
  font-family: Cinzel, Georgia, serif; font-size: .78rem; text-transform: uppercase;
  color: var(--accent); margin: 0 0 8px; padding-bottom: 5px;
  border-bottom: 1px solid var(--line); display: flex; align-items: center; gap: 8px;
}
.rest-card h3 .x {
  margin-left: auto; border: 0; background: none; cursor: pointer;
  color: var(--muted); font-size: 1.1rem; line-height: 1; padding: 0 2px;
}
/* one dot per step, filled up to the one being read */
.rest-dots { display: flex; gap: 5px; margin-bottom: 9px; }
.rest-dots i {
  width: 7px; height: 7px; border-radius: 50%; background: var(--line);
}
.rest-dots i.on { background: var(--accent); }
.rest-body { overflow-y: auto; flex: 1; min-height: 60px; }
.rest-body h4 {
  font-family: Cinzel, Georgia, serif; font-size: .72rem; text-transform: uppercase;
  letter-spacing: .06em; color: var(--accent); margin: 0 0 5px;
}
.rest-body p { margin: 0 0 8px; font-size: .88rem; }
.rest-hp {
  display: flex; align-items: baseline; gap: 8px; margin: 8px 0;
  font-family: Cinzel, Georgia, serif; font-size: .74rem; color: var(--muted);
}
.rest-hp b { font-family: inherit; font-size: 1.3rem; color: var(--ink); }
/* the hit dice spender: one pip per die, spent ones filled */
.rest-dice { display: flex; flex-wrap: wrap; gap: 6px; align-items: center; margin: 8px 0; }
.rest-die {
  width: 15px; height: 15px; border: 1px solid var(--muted); border-radius: 3px;
  background: var(--paper); padding: 0; cursor: default;
}
.rest-die.spent { background: var(--accent); border-color: var(--accent); }
.rest-roll {
  font-family: Cinzel, Georgia, serif; font-size: .66rem; text-transform: uppercase;
  letter-spacing: .06em; border: 1px solid var(--accent); border-radius: 4px;
  background: var(--accent); color: var(--paper); padding: 4px 12px; cursor: pointer;
}
.rest-roll:hover { background: #8f2222; }
.rest-roll[disabled] { opacity: .45; cursor: default; }
.rest-log { font-size: .74rem; color: var(--muted); margin: 6px 0 0; }
.rest-log li { margin: 1px 0; }
.rest-back-list { margin: 4px 0 0; padding-left: 18px; font-size: .82rem; }
.rest-note { width: 100%; font-family: inherit; font-size: .9rem; margin-top: 6px;
  border: 1px solid var(--line); border-radius: 4px; background: #fbf7ee;
  color: var(--ink); padding: 5px 8px; }
.rest-foot { display: flex; align-items: center; gap: 8px; margin-top: 10px;
  padding-top: 9px; border-top: 1px solid var(--line); flex-wrap: wrap; }
.rest-foot .grow { flex: 1; }
.rest-err { font-size: .74rem; color: var(--accent); }
.rest-msg { font-size: .74rem; color: var(--muted); }

/* ---------------- features & text ---------------- */
.feature { margin-bottom: 9px; }
.feature h3 {
  font-size: .82rem; margin: 0 0 1px; color: var(--accent);
}
.feature .src { font-size: .62rem; color: var(--muted); text-transform: uppercase; }
.feature p { margin: 2px 0; font-size: .86rem; white-space: pre-wrap; }
/* The uses a rationed feature has left, drawn like spell slots. A filled pip
   is a use that is gone. */
.uses { display: flex; align-items: center; flex-wrap: wrap; gap: 5px; margin: 3px 0 1px; }
.use-pip {
  width: 12px; height: 12px; border: 1px solid var(--muted); border-radius: 50%;
  background: var(--paper); flex: 0 0 auto;
}
.use-pip.used { background: var(--accent); border-color: var(--accent); }
.uses-note { font-size: .62rem; color: var(--muted); }
.uses.live .use-pip { cursor: pointer; }
.uses.live .use-pip:hover { border-color: var(--accent); box-shadow: 0 0 0 2px rgba(184,134,11,.25); }
.uses.busy { opacity: .55; pointer-events: none; }
.uses .uses-err { font-size: .62rem; color: var(--accent); }
/* The box that holds it does the scrolling now, so the scroller is
   just a block. It keeps its name because a sheet printed or read with no
   script still finds it. */
.scroller { padding-right: 2px; }
.quote { font-style: italic; white-space: pre-wrap; margin: 0 0 8px; }
.quote .label { display: block; font-style: normal; font-size: .6rem; text-transform: uppercase; color: var(--muted); }

/* ---------------- spells ---------------- */
.spell-head { display: flex; flex-wrap: wrap; gap: 10px; margin-bottom: 10px; }
.slot-row { display: flex; align-items: center; gap: 6px; margin: 4px 0 6px; }
.slot-row .lvl { font-family: Cinzel, serif; font-size: .68rem; color: var(--muted); width: 3.2em; }
.slot { width: 12px; height: 12px; border: 1px solid var(--muted); border-radius: 3px; background: var(--paper); flex: 0 0 auto; }
.slot.used { background: var(--muted); }
/* With a server behind the sheet a slot is a control: click an empty one to
   spend it, a spent one to give it back. */
.slot-row.live .slot { cursor: pointer; }
.slot-row.live .slot:hover { border-color: var(--accent); box-shadow: 0 0 0 2px rgba(184,134,11,.25); }
.spell-level { margin-bottom: 12px; }
.spell-level h3 {
  font-size: .74rem; text-transform: uppercase; color: var(--accent);
  border-bottom: 1px solid var(--line); margin: 0 0 4px; padding-bottom: 2px;
}
.prep { color: var(--accent-2); font-weight: 700; }
details.spell summary {
  cursor: pointer; font-size: .88rem; padding: 1px 0;
  border-bottom: 1px dotted rgba(139,126,102,.4);
}
.cast-btn {
  float: right;
  font-family: Cinzel, Georgia, serif; font-size: .56rem;
  text-transform: uppercase; letter-spacing: .06em;
  border: 1px solid var(--line); border-radius: 3px;
  background: var(--paper); color: var(--accent);
  padding: 0 6px; margin-left: 6px; cursor: pointer; line-height: 1.6;
}
.cast-btn:hover { background: #fbf7ee; border-color: var(--accent); }
details.spell summary::marker { color: var(--muted); }
details.spell p { font-size: .82rem; margin: 4px 0 8px 12px; white-space: pre-wrap; }
.tagline { font-size: .68rem; color: var(--muted); }

/* ---------------- managing spells ----------------
   The spell page is live in the same way the inventory is: spells are learned,
   given back and prepared from the manage box and the org file behind the
   sheet is rewritten. What may be taken is the rules engine's answer, so the
   box only ever offers spells the character is actually entitled to. */
.spell-foot {
  display: flex; align-items: center; gap: 10px; flex-wrap: wrap;
  margin-top: 4px; padding-top: 6px; border-top: 1px solid var(--line);
}
.spell-manage {
  font-family: Cinzel, Georgia, serif; font-size: .62rem;
  text-transform: uppercase; letter-spacing: .06em;
  border: 1px dashed var(--line); border-radius: 4px;
  background: var(--paper); color: var(--accent);
  padding: 4px 12px; cursor: pointer;
}
.spell-manage:hover { background: #fbf7ee; border-style: solid; border-color: var(--accent); }
.sb-msg { font-size: .7rem; color: var(--muted); }
.sb-err { font-size: .72rem; color: var(--accent); }

/* the budgets, on the sheet and again at the top of the manage box */
.sb-budgets { display: flex; gap: 6px; flex-wrap: wrap; }
.sb-budget {
  font-size: .66rem; color: var(--muted);
  border: 1px solid var(--line); border-radius: 10px;
  padding: 1px 9px; background: var(--paper); white-space: nowrap;
}
.sb-budget b { color: var(--ink); font-weight: 700; }
.sb-budget.full { border-color: var(--accent-2); background: #f6ecd2; }
.sb-budget.full b { color: #7a5a08; }

.sb-card { width: min(680px, 100%); max-height: 84vh; }
.sb-tools { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; margin: 8px 0 2px; }
.sb-tools .lbl {
  font-size: .6rem; text-transform: uppercase; letter-spacing: .05em;
  color: var(--muted); margin-left: 4px;
}
.sb-chip {
  font-size: .68rem; font-family: inherit; color: var(--muted);
  border: 1px solid transparent; border-radius: 10px;
  background: none; padding: 1px 9px; cursor: pointer;
}
.sb-chip:hover { color: var(--ink); }
.sb-chip.on { background: var(--paper-2); color: var(--accent); border-color: var(--edge); }
.sb-list { overflow-y: auto; margin: 8px 0 0; flex: 1; min-height: 120px; }
.sb-group {
  font-family: Cinzel, Georgia, serif;
  font-size: .66rem; text-transform: uppercase; letter-spacing: .06em;
  color: var(--accent); border-bottom: 1px solid var(--line);
  margin: 10px 0 3px; padding-bottom: 2px;
}
.sb-group:first-child { margin-top: 0; }
.sb-group .n { color: var(--muted); font-weight: 400; letter-spacing: 0; }
.sb-item {
  display: flex; align-items: baseline; gap: 8px;
  border-bottom: 1px dotted rgba(139,126,102,.4);
  padding: 3px 2px;
}
.sb-item.mine { background: rgba(196,154,44,.08); }
.sb-item.blocked .sb-name { color: var(--muted); }
.sb-what { flex: 1; min-width: 0; cursor: pointer; }
.sb-name { font-size: .88rem; }
.sb-name .mark { color: var(--accent-2); font-weight: 700; margin-right: 3px; }
.sb-meta { font-size: .66rem; color: var(--muted); }
.sb-meta .why { color: var(--accent); }
.sb-tag {
  font-size: .56rem; text-transform: uppercase; letter-spacing: .06em;
  border: 1px solid var(--line); border-radius: 8px; padding: 0 5px;
  color: var(--muted); margin-left: 5px; white-space: nowrap;
}
.sb-tag.held { border-color: var(--accent-2); color: #7a5a08; }
.sb-acts { white-space: nowrap; flex: 0 0 auto; }
.sb-b {
  font-family: Cinzel, Georgia, serif; font-size: .56rem;
  text-transform: uppercase; letter-spacing: .06em;
  border: 1px solid var(--line); border-radius: 3px;
  background: var(--paper); color: var(--accent);
  padding: 1px 7px; margin-left: 4px; cursor: pointer; line-height: 1.6;
}
.sb-b:hover { background: #fbf7ee; border-color: var(--accent); }
.sb-b.give { color: var(--muted); }
.sb-b.give:hover { color: var(--accent); }
.sb-text {
  font-size: .78rem; margin: 2px 0 6px 12px; white-space: pre-wrap;
  color: var(--ink);
}
.sb-empty { font-size: .8rem; color: var(--muted); font-style: italic; padding: 8px 2px; }
footer.sheet-foot {
  margin-top: 20px; padding-top: 8px; border-top: 1px solid var(--line);
  font-size: .68rem; color: var(--muted); display: flex; justify-content: space-between; gap: 12px;
}
.warn { color: var(--accent); font-size: .78rem; }

/* ---------------- rollable things ---------------- */
.rollable {
  cursor: pointer;
  border-radius: 4px;
  outline: none;
  transition: background .12s ease, box-shadow .12s ease;
}
.rollable:hover, .rollable:focus-visible {
  background: rgba(184,134,11,.20);
  box-shadow: 0 0 0 2px rgba(184,134,11,.5);
}
.rollable:active { background: rgba(184,134,11,.34); }
.row.rollable, td.rollable, .inline-roll {
  text-decoration: underline dotted rgba(184,134,11,.85);
  text-underline-offset: 2px;
  text-decoration-thickness: 1px;
}
.ability.rollable, .tile.rollable {
  box-shadow: inset 0 0 0 1px rgba(184,134,11,.4);
}
.ability.rollable:hover, .tile.rollable:hover,
.ability.rollable:focus-visible, .tile.rollable:focus-visible {
  box-shadow: inset 0 0 0 1px rgba(184,134,11,.4), 0 0 0 2px rgba(184,134,11,.5);
}
.inline-roll { white-space: nowrap; font-variant-numeric: tabular-nums; }
.ability .check-note {
  display: block;
  font-size: .55rem;
  letter-spacing: .04em;
  color: var(--accent-2);
  margin-top: 1px;
}
.roll-hint { color: var(--accent-2); }

/* ---------------- dice overlay ---------------- */
#dice-canvas {
  position: fixed;
  left: 0; top: 0;
  width: 100%; height: 100%;
  pointer-events: none;
  z-index: 60;
}

/* ---------------- roll tray ---------------- */
:root { --tray-w: 330px; }
@media (max-width: 430px) { :root { --tray-w: 86vw; } }

#dice-tab {
  position: fixed;
  right: 0; top: 46%;
  z-index: 95;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 7px;
  padding: 13px 7px;
  background: linear-gradient(180deg, #2c2620, #1a1713);
  color: var(--accent-2);
  border: 1px solid rgba(184,134,11,.55);
  border-right: 0;
  border-radius: 10px 0 0 10px;
  cursor: pointer;
  font: 700 .62rem/1 Cinzel, "Trajan Pro", Georgia, serif;
  letter-spacing: .16em;
  text-transform: uppercase;
  box-shadow: -5px 0 16px rgba(0,0,0,.4);
  transform: translateY(-50%);
  transition: transform .28s cubic-bezier(.2,.8,.3,1), background .15s ease;
}
#dice-tab:hover { background: linear-gradient(180deg, #3a3227, #211c17); }
#dice-tab.shifted { transform: translateY(-50%) translateX(calc(-1 * var(--tray-w))); }
#dice-tab span { writing-mode: vertical-rl; }
#dice-tab svg, #dice-tray h2 svg {
  width: 16px; height: 16px;
  fill: none; stroke: currentColor;
  stroke-width: 1.35; stroke-linejoin: round;
}
@keyframes dice-pulse {
  0%   { box-shadow: -5px 0 16px rgba(0,0,0,.4), 0 0 0 0 rgba(184,134,11,.6); }
  100% { box-shadow: -5px 0 16px rgba(0,0,0,.4), 0 0 0 16px rgba(184,134,11,0); }
}
#dice-tab.pulse { animation: dice-pulse .7s ease-out 1; }

#dice-tray {
  position: fixed;
  top: 0; right: 0; bottom: 0;
  width: var(--tray-w);
  z-index: 90;
  display: flex;
  flex-direction: column;
  background: linear-gradient(180deg, #241f1a, #17140f);
  border-left: 3px double var(--accent-2);
  box-shadow: -16px 0 44px rgba(0,0,0,.55);
  color: #ece2cd;
  font-family: "EB Garamond", Palatino, Georgia, serif;
  font-size: .9rem;
  transform: translateX(103%);
  transition: transform .28s cubic-bezier(.2,.8,.3,1);
}
#dice-tray.open { transform: none; }
#dice-tray .dt-head {
  display: flex; align-items: center; gap: 8px;
  padding: 12px 12px 10px;
  border-bottom: 1px solid rgba(184,134,11,.35);
}
#dice-tray h2 {
  flex: 1; margin: 0;
  display: flex; align-items: center; gap: 7px;
  font-size: .78rem; text-transform: uppercase;
  letter-spacing: .16em; color: var(--accent-2);
}
.dt-auto {
  display: flex; align-items: center; gap: 4px;
  font-size: .62rem; text-transform: uppercase; letter-spacing: .1em;
  color: #9d9078; cursor: pointer;
}
.dt-auto input { accent-color: #b8860b; margin: 0; }
.dt-btn {
  font: inherit; font-size: .64rem; text-transform: uppercase; letter-spacing: .1em;
  background: transparent; color: #d8caa8;
  border: 1px solid rgba(184,134,11,.4); border-radius: 4px;
  padding: 3px 7px; cursor: pointer;
}
.dt-btn:hover { background: rgba(184,134,11,.2); }
.dt-x { font-size: 1rem; line-height: 1; padding: 1px 7px; }

#dice-latest { padding: 12px 12px 4px; min-height: 34px; }
.rc-label {
  font-family: Cinzel, "Trajan Pro", Georgia, serif;
  font-size: .95rem; color: #f4e6c2; line-height: 1.2;
}
.rc-formula {
  font-size: .7rem; color: #9d9078; letter-spacing: .07em; margin-top: 2px;
}
.rc-grid { display: grid; gap: 6px; margin-top: 10px; }
.rc-grid.three { grid-template-columns: 1fr 1.3fr 1fr; }
.rc-grid.two   { grid-template-columns: 1fr 1fr; }
.rc-grid.one   { grid-template-columns: 1fr; }
.rc-cell {
  background: rgba(255,255,255,.045);
  border: 1px solid rgba(184,134,11,.25);
  border-radius: 6px;
  text-align: center;
  padding: 7px 3px 6px;
}
.rc-cell span {
  display: block;
  font-size: .55rem; text-transform: uppercase; letter-spacing: .11em;
  color: #9d9078;
}
.rc-cell b {
  display: block; margin-top: 2px;
  font-family: Cinzel, "Trajan Pro", Georgia, serif;
  font-size: 1.45rem; color: #f0e2c0;
}
.rc-cell.main {
  border-color: rgba(184,134,11,.8);
  background: rgba(184,134,11,.13);
}
.rc-cell.main b { font-size: 1.95rem; }
.rc-cell.crit b { color: #93e2a2; text-shadow: 0 0 12px rgba(147,226,162,.45); }
.rc-cell.fumble b { color: #e5837a; }
.rc-dice { margin-top: 9px; font-size: .7rem; color: #9d9078; }
.rc-dice b { color: #dccca0; font-weight: 600; }
/* a cast reports two rolls and what the target has to do about them, so the
   card grows a heading per roll and a line for the save */
.rc-sub {
  margin-top: 9px;
  font: 700 .55rem/1 Cinzel, "Trajan Pro", Georgia, serif;
  letter-spacing: .16em; text-transform: uppercase; color: #8d8168;
}
.rc-line {
  margin-top: 9px; padding: 6px 8px;
  border: 1px dashed rgba(184,134,11,.45); border-radius: 6px;
  font-size: .76rem; color: #e8d7ae;
}
/* what the cast cost, once the file has been told about it */
.rc-slot { margin-top: 7px; font-size: .7rem; color: #9d9078; }
.rc-slot.bad { color: #e5837a; }
/* the mark that says a line of history is a spell rather than a die */
.spell-mark {
  display: inline-block; width: 12px; height: 12px;
  margin-right: 5px; vertical-align: -1px; color: var(--accent-2);
}
.spell-mark svg { width: 100%; height: 100%; fill: none; stroke: currentColor; stroke-width: 1.6; }
.rc-label .spell-mark { width: 14px; height: 14px; vertical-align: -2px; }

.dt-hist-head {
  font: 700 .6rem/1 Cinzel, "Trajan Pro", Georgia, serif;
  letter-spacing: .18em; text-transform: uppercase; color: #8d8168;
  padding: 10px 12px 6px;
  border-top: 1px solid rgba(184,134,11,.18);
}
#dice-history { flex: 1; overflow-y: auto; padding: 0 8px 8px; }
.hr {
  display: flex; align-items: baseline; gap: 8px;
  width: 100%; text-align: left;
  background: transparent; border: 0;
  border-bottom: 1px dotted rgba(184,134,11,.2);
  color: #cfc3a8; font: inherit; font-size: .78rem;
  padding: 5px 4px; cursor: pointer;
}
.hr:hover { background: rgba(184,134,11,.13); }
.ht { flex: 0 0 auto; width: 3.1em; font-size: .62rem; color: #7e735d; }
.hl { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.hx { flex: 0 0 auto; font-size: .62rem; color: #7e735d; }
.hv {
  flex: 0 0 auto; width: 2.5em; text-align: right;
  font-family: Cinzel, "Trajan Pro", Georgia, serif; font-weight: 700; color: #f0e2c0;
}
.hv.crit { color: #93e2a2; }
.hv.fumble { color: #e5837a; }
.dt-empty { color: #7e735d; font-size: .75rem; padding: 8px 6px; }
.dt-tip {
  font-size: .62rem; line-height: 1.5; color: #7e735d;
  padding: 8px 12px 12px;
  border-top: 1px solid rgba(184,134,11,.18);
}

@media (prefers-reduced-motion: reduce) {
  #dice-tray, #dice-tab { transition: none; }
  #dice-tab.pulse { animation: none; }
}

/* ---------------- custom roll dock ---------------- */
#dice-dock {
  position: fixed;
  right: 22px; bottom: 22px;
  z-index: 94;
  transition: transform .28s cubic-bezier(.2,.8,.3,1);
}
#dice-dock.shifted { transform: translateX(calc(-1 * var(--tray-w))); }

#dice-fab {
  display: grid;
  place-items: center;
  width: 54px; height: 54px;
  padding: 0;
  border-radius: 50%;
  background: radial-gradient(circle at 34% 26%, #3b3227, #17130f);
  color: var(--accent-2);
  border: 1px solid rgba(184,134,11,.6);
  box-shadow: 0 6px 20px rgba(0,0,0,.45);
  cursor: pointer;
  transition: background .15s ease, box-shadow .15s ease;
}
#dice-fab:hover { background: radial-gradient(circle at 34% 26%, #4c4231, #221c15); }
#dice-fab:focus-visible { outline: 2px solid rgba(184,134,11,.8); outline-offset: 3px; }
#dice-fab svg {
  width: 26px; height: 26px;
  fill: none; stroke: currentColor;
  stroke-width: 1.9; stroke-linejoin: round; stroke-linecap: round;
}

#dice-custom {
  position: absolute;
  right: 0; bottom: 66px;
  width: 320px;
  max-width: calc(100vw - 44px);
  padding: 12px;
  border: 1px solid rgba(184,134,11,.5);
  border-radius: 10px;
  background: linear-gradient(180deg, #241f1a, #17140f);
  box-shadow: 0 16px 44px rgba(0,0,0,.55);
  color: #ece2cd;
  font-family: "EB Garamond", Palatino, Georgia, serif;
  opacity: 0;
  transform: translateY(14px) scale(.97);
  transform-origin: 100% 100%;
  pointer-events: none;
  transition: opacity .18s ease, transform .24s cubic-bezier(.2,.8,.3,1);
}
#dice-custom.open { opacity: 1; transform: none; pointer-events: auto; }
@keyframes dice-nope {
  0%, 100% { transform: none; }
  25%      { transform: translateX(-5px); }
  75%      { transform: translateX(5px); }
}
#dice-custom.nope { animation: dice-nope .22s ease 1; }

#dice-custom .dc-head, #dnd-sess .dc-head {
  display: flex; align-items: center; gap: 8px; margin-bottom: 9px;
}
#dice-custom h3 {
  flex: 1; margin: 0;
  font-family: Cinzel, "Trajan Pro", Georgia, serif;
  font-size: .74rem; text-transform: uppercase;
  letter-spacing: .16em; color: var(--accent-2);
}
#dice-expr {
  width: 100%;
  padding: 7px 9px;
  font-family: "EB Garamond", Palatino, Georgia, serif;
  font-size: 1.05rem;
  letter-spacing: .02em;
  color: #f4e6c2;
  background: rgba(0,0,0,.3);
  border: 1px solid rgba(184,134,11,.4);
  border-radius: 6px;
}
#dice-expr:focus { outline: none; border-color: rgba(184,134,11,.85); }
#dice-expr::placeholder { color: #6f6553; }

.dc-dice {
  display: grid;
  grid-template-columns: repeat(6, 1fr);
  gap: 5px;
  margin: 9px 0;
}
.dc-dice button {
  padding: 7px 0;
  font: 700 .74rem/1 Cinzel, "Trajan Pro", Georgia, serif;
  color: #e2d3ae;
  background: rgba(255,255,255,.05);
  border: 1px solid rgba(184,134,11,.32);
  border-radius: 5px;
  cursor: pointer;
  transition: background .12s ease, border-color .12s ease;
}
.dc-dice button:hover { background: rgba(184,134,11,.24); border-color: rgba(184,134,11,.7); }
.dc-dice button:active { background: rgba(184,134,11,.4); }

.dc-mod { display: flex; align-items: center; gap: 8px; }
.dc-mod span {
  flex: 1;
  font-size: .6rem; text-transform: uppercase; letter-spacing: .12em; color: #9d9078;
}
.dc-mod button {
  width: 26px; height: 26px;
  font: 700 1rem/1 Georgia, serif;
  color: #e2d3ae;
  background: rgba(255,255,255,.05);
  border: 1px solid rgba(184,134,11,.32);
  border-radius: 5px;
  cursor: pointer;
}
.dc-mod button:hover { background: rgba(184,134,11,.24); }
.dc-mod b {
  min-width: 2.4em; text-align: center;
  font-family: Cinzel, "Trajan Pro", Georgia, serif;
  font-size: .95rem; color: #f0e2c0;
}

.dc-hint { margin: 9px 0 10px; font-size: .64rem; line-height: 1.5; color: #7e735d; }
.dc-actions { display: flex; align-items: center; gap: 6px; }
.dc-actions .dt-btn { flex: 0 0 auto; }
.dc-actions + .dc-actions { margin-top: 7px; }
/* the two rests get a row to themselves, split evenly */
.dc-rest .dt-btn { flex: 1; }
/* so does printing, which is the width of the panel and nothing else */
.dc-print .dt-btn { flex: 1; }
.dc-roll {
  flex: 1; min-width: 5.4em;
  padding: 8px 0;
  font: 700 .74rem/1 Cinzel, "Trajan Pro", Georgia, serif;
  letter-spacing: .16em; text-transform: uppercase;
  color: #1d1809;
  background: linear-gradient(180deg, #f2d489, #c9942a);
  border: 1px solid rgba(184,134,11,.9);
  border-radius: 6px;
  cursor: pointer;
  transition: filter .12s ease;
}
.dc-roll:hover { filter: brightness(1.1); }
.dc-roll:disabled { opacity: .4; cursor: default; filter: none; }

@media (prefers-reduced-motion: reduce) {
  #dice-dock, #dice-custom { transition: none; }
  #dice-custom.nope { animation: none; }
}

/* ---------------- session log: panel, notes drawer, search ---------------- */
:root { --notes-h: 46vh; }
@media (max-height: 620px) { :root { --notes-h: 62vh; } }

#dnd-sess {
  position: absolute;
  right: 0; bottom: 66px;
  width: 320px;
  max-width: calc(100vw - 44px);
  padding: 12px;
  border: 1px solid rgba(184,134,11,.5);
  border-radius: 10px;
  background: linear-gradient(180deg, #241f1a, #17140f);
  box-shadow: 0 16px 44px rgba(0,0,0,.55);
  color: #ece2cd;
  font-family: "EB Garamond", Palatino, Georgia, serif;
  opacity: 0;
  transform: translateY(14px) scale(.97);
  transform-origin: 100% 100%;
  pointer-events: none;
  transition: opacity .18s ease, transform .24s cubic-bezier(.2,.8,.3,1);
}
#dnd-sess.open { opacity: 1; transform: none; pointer-events: auto; }
#dnd-sess h3 {
  flex: 1; margin: 0;
  font-family: Cinzel, "Trajan Pro", Georgia, serif;
  font-size: .74rem; text-transform: uppercase;
  letter-spacing: .16em; color: var(--accent-2);
}
.sess-label {
  display: block; margin: 8px 0 3px;
  font-size: .58rem; text-transform: uppercase; letter-spacing: .12em; color: #9d9078;
}
.nd-input {
  width: 100%;
  padding: 6px 8px;
  font-family: "EB Garamond", Palatino, Georgia, serif;
  font-size: .92rem;
  color: #f4e6c2;
  background: rgba(0,0,0,.3);
  border: 1px solid rgba(184,134,11,.4);
  border-radius: 6px;
}
.nd-input:focus { outline: none; border-color: rgba(184,134,11,.85); }
.nd-input::placeholder { color: #6f6553; }
#dnd-sess .dc-actions { margin-top: 10px; }
.sess-live {
  display: flex; align-items: center; gap: 7px;
  font-family: Cinzel, "Trajan Pro", Georgia, serif;
  font-size: .95rem; color: #f0e2c0;
}
.sess-file {
  margin-top: 3px;
  font-size: .6rem; color: #7e735d;
  overflow-wrap: anywhere;
}
.sess-counts { margin-top: 6px; font-size: .72rem; color: #a4977c; }
.sess-login {
  margin-top: 10px; padding-top: 9px;
  border-top: 1px solid rgba(184,134,11,.22);
}
.sess-login .nd-input { margin-bottom: 5px; }
.sess-server { margin-top: 10px; font-size: .62rem; color: #7e735d; }
.sess-server summary {
  cursor: pointer; text-transform: uppercase; letter-spacing: .12em; margin-bottom: 5px;
}
.nd-err {
  margin-top: 9px; padding: 6px 8px;
  font-size: .72rem; color: #f0b8ae;
  background: rgba(160,60,50,.16);
  border: 1px solid rgba(200,90,80,.4); border-radius: 5px;
}
@keyframes rec-blink { 0%, 100% { opacity: 1; } 50% { opacity: .25; } }
.rec-dot {
  display: inline-block; flex: 0 0 auto;
  width: 8px; height: 8px; border-radius: 50%;
  background: #d2544a; box-shadow: 0 0 7px rgba(210,84,74,.75);
  animation: rec-blink 1.8s ease-in-out infinite;
}
#dice-fab.rec::after {
  content: ""; position: absolute;
  right: 2px; top: 2px;
  width: 9px; height: 9px; border-radius: 50%;
  background: #d2544a; box-shadow: 0 0 7px rgba(210,84,74,.75);
}
#dice-fab { position: relative; }

/* ---- the notes drawer ---- */
#notes-tab {
  position: fixed;
  left: 28px; bottom: 0;
  z-index: 95;
  display: flex; align-items: center; gap: 8px;
  padding: 8px 15px 9px;
  background: linear-gradient(180deg, #2c2620, #1a1713);
  color: var(--accent-2);
  border: 1px solid rgba(184,134,11,.55);
  border-bottom: 0;
  border-radius: 10px 10px 0 0;
  cursor: pointer;
  font: 700 .62rem/1 Cinzel, "Trajan Pro", Georgia, serif;
  letter-spacing: .16em; text-transform: uppercase;
  box-shadow: 0 -5px 16px rgba(0,0,0,.4);
  transition: bottom .28s cubic-bezier(.2,.8,.3,1), background .15s ease;
}
#notes-tab:hover { background: linear-gradient(180deg, #3a3227, #211c17); }
#notes-tab.raised { bottom: var(--notes-h); }
#notes-tab svg, #notes-drawer h2 svg {
  width: 15px; height: 15px;
  fill: none; stroke: currentColor;
  stroke-width: 1.35; stroke-linejoin: round; stroke-linecap: round;
}
#notes-tab em {
  font-style: normal; letter-spacing: .04em; text-transform: none;
  font-family: "EB Garamond", Palatino, Georgia, serif; color: #cdbe98;
  max-width: 15em; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
}
#notes-tab.rec { border-color: rgba(210,84,74,.6); }
#notes-tab.rec::before {
  content: ""; width: 8px; height: 8px; border-radius: 50%;
  background: #d2544a; box-shadow: 0 0 7px rgba(210,84,74,.75);
  animation: rec-blink 1.8s ease-in-out infinite;
}

#notes-drawer {
  position: fixed;
  left: 0; right: 0; bottom: 0;
  height: var(--notes-h);
  z-index: 93;
  display: flex; flex-direction: column;
  background: linear-gradient(180deg, #241f1a, #17140f);
  border-top: 3px double var(--accent-2);
  box-shadow: 0 -16px 44px rgba(0,0,0,.55);
  color: #ece2cd;
  font-family: "EB Garamond", Palatino, Georgia, serif;
  transform: translateY(101%);
  transition: transform .28s cubic-bezier(.2,.8,.3,1), right .28s cubic-bezier(.2,.8,.3,1);
}
#notes-drawer.open { transform: none; }
#notes-drawer.tray-open { right: var(--tray-w); }
.nd-head {
  display: flex; align-items: center; gap: 12px;
  padding: 9px 14px;
  border-bottom: 1px solid rgba(184,134,11,.3);
}
#notes-drawer h2 {
  margin: 0; display: flex; align-items: center; gap: 7px;
  font-size: .74rem; text-transform: uppercase;
  letter-spacing: .16em; color: var(--accent-2);
}
.nd-tabs { display: flex; gap: 5px; }
.nd-tab {
  font: inherit; font-size: .66rem; text-transform: uppercase; letter-spacing: .1em;
  background: transparent; color: #9d9078;
  border: 1px solid transparent; border-radius: 5px;
  padding: 4px 9px; cursor: pointer;
}
.nd-tab:hover { color: #e2d3ae; background: rgba(184,134,11,.12); }
.nd-tab.on {
  color: #f0e2c0;
  border-color: rgba(184,134,11,.55);
  background: rgba(184,134,11,.16);
}
.nd-status {
  flex: 1; text-align: right;
  display: flex; align-items: center; justify-content: flex-end; gap: 7px;
  font-size: .72rem; color: #cdbe98;
}
.nd-off { color: #7e735d; font-size: .66rem; text-transform: uppercase; letter-spacing: .1em; }
.nd-warn { color: #e0b166; font-size: .66rem; }
.nd-body { flex: 1; min-height: 0; display: flex; }
.nd-view { flex: 1; min-width: 0; display: none; flex-direction: column; padding: 12px 14px; }
.nd-view.on { display: flex; }
.nd-toolbar {
  display: flex; align-items: center; gap: 9px; margin-bottom: 10px;
}
.nd-toolbar .nd-input { flex: 1; }
.nd-title { font-family: Cinzel, "Trajan Pro", Georgia, serif; color: #f0e2c0; }
.nd-meta { flex: 1; font-size: .68rem; color: #7e735d; }
.nd-sub {
  font: 700 .6rem/1 Cinzel, "Trajan Pro", Georgia, serif;
  letter-spacing: .18em; text-transform: uppercase; color: #8d8168;
  padding-bottom: 7px; margin-bottom: 9px;
  border-bottom: 1px solid rgba(184,134,11,.2);
}

.nd-split { flex: 1; min-height: 0; display: flex; gap: 18px; }
.nd-compose { flex: 0 0 40%; display: flex; flex-direction: column; min-height: 0; }
#note-text {
  flex: 1; min-height: 78px; resize: none;
  padding: 9px 11px;
  font-family: "EB Garamond", Palatino, Georgia, serif;
  font-size: .95rem; line-height: 1.5;
  color: #f4e6c2;
  background: rgba(0,0,0,.3);
  border: 1px solid rgba(184,134,11,.4);
  border-radius: 7px;
}
#note-text:focus { outline: none; border-color: rgba(184,134,11,.85); }
#note-text::placeholder { color: #6f6553; }
.nd-compose-foot { display: flex; align-items: center; gap: 8px; margin-top: 8px; }
.nd-hint { flex: 1; font-size: .62rem; color: #7e735d; }
.nd-compose-foot .dc-roll { flex: 0 0 auto; padding: 8px 14px; }
.note-preview {
  margin-top: 9px; max-height: 34%; overflow-y: auto;
  padding: 8px 10px;
  border: 1px dashed rgba(184,134,11,.32); border-radius: 7px;
}
.nd-stream, .nd-col { flex: 1; min-width: 0; overflow-y: auto; padding-right: 6px; }
.nd-detail { flex: 1; min-height: 0; display: flex; gap: 18px; }
.note-entry {
  border-left: 2px solid rgba(184,134,11,.35);
  padding: 1px 0 1px 11px;
  margin-bottom: 13px;
}
.note-time {
  font: 700 .58rem/1 Cinzel, "Trajan Pro", Georgia, serif;
  letter-spacing: .14em; color: #8d8168; margin-bottom: 4px;
}

/* org markup as it appears in a note */
.org-rich { font-size: .9rem; line-height: 1.55; color: #ded2b8; }
.org-rich p { margin: 0 0 7px; }
.org-rich h3, .org-rich h4 {
  margin: 9px 0 5px;
  font-family: Cinzel, "Trajan Pro", Georgia, serif;
  color: #f0e2c0; font-size: .92rem; letter-spacing: .04em;
}
.org-rich h4 { font-size: .82rem; color: #dcc890; }
.org-rich ul, .org-rich ol { margin: 0 0 7px; padding-left: 19px; }
.org-rich li { margin-bottom: 2px; }
.org-rich code {
  font-family: ui-monospace, Menlo, Consolas, monospace;
  font-size: .82em; color: #e8cf95;
  background: rgba(184,134,11,.14); border-radius: 3px; padding: 0 3px;
}
.org-rich pre {
  margin: 0 0 7px; padding: 7px 9px; overflow-x: auto;
  font-family: ui-monospace, Menlo, Consolas, monospace; font-size: .78rem;
  background: rgba(0,0,0,.32); border: 1px solid rgba(184,134,11,.22); border-radius: 6px;
}
.org-rich blockquote {
  margin: 0 0 7px; padding: 4px 11px;
  border-left: 3px solid rgba(184,134,11,.5); color: #cdbe98; font-style: italic;
}
.org-rich a { color: var(--accent-2); }
.org-rich .org-table {
  width: auto; border-collapse: collapse; margin: 0 0 8px; font-size: .82rem;
}
/* The sheet itself is dark ink on parchment, so a table in a note has to say
   what colour it is or it inherits the page's ink onto the dark drawer. */
.org-rich .org-table td {
  border: 1px solid rgba(184,134,11,.22); padding: 3px 7px; color: #ded2b8;
}

/* session list, search hits and the roll log */
.sess-row, .hit {
  display: flex; align-items: baseline; gap: 11px;
  width: 100%; text-align: left;
  background: transparent; border: 0;
  border-bottom: 1px dotted rgba(184,134,11,.2);
  color: #cfc3a8; font: inherit; font-size: .84rem;
  padding: 7px 5px; cursor: pointer;
}
.sess-row:hover, .hit:hover { background: rgba(184,134,11,.13); }
.sess-row.here { background: rgba(184,134,11,.1); }
.sess-date { flex: 0 0 auto; width: 6.6em; font-size: .68rem; color: #7e735d; }
.sess-name {
  flex: 0 0 auto; max-width: 16em;
  font-family: Cinzel, "Trajan Pro", Georgia, serif; color: #f0e2c0;
  overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
}
.sess-sum {
  flex: 1; min-width: 0; color: #a4977c;
  overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
}
.sess-who {
  flex: 0 0 auto; max-width: 14em; font-size: .72rem; color: #8d8168;
  overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
}
.sess-meta { flex: 0 0 auto; font-size: .64rem; color: #7e735d; }
.hit { flex-direction: column; gap: 3px; }
.hit-head {
  display: flex; gap: 9px; align-items: baseline; width: 100%;
  font-family: Cinzel, "Trajan Pro", Georgia, serif; color: #f0e2c0; font-size: .8rem;
}
.hit-where { font-family: inherit; font-size: .64rem; color: #7e735d; }
.hit-text { color: #bfb298; font-size: .82rem; }
.log-table { width: 100%; border-collapse: collapse; font-size: .78rem; }
.log-table th {
  text-align: left; padding: 3px 6px;
  font: 700 .58rem/1.6 Cinzel, "Trajan Pro", Georgia, serif;
  letter-spacing: .12em; text-transform: uppercase; color: #8d8168;
  border-bottom: 1px solid rgba(184,134,11,.3);
}
.log-table td {
  padding: 3px 6px; border-bottom: 1px dotted rgba(184,134,11,.16); color: #cfc3a8;
}
.log-table td.num {
  font-family: Cinzel, "Trajan Pro", Georgia, serif; color: #f0e2c0;
}
.log-table td.dim { color: #7e735d; font-size: .72rem; }

@media (max-width: 860px) {
  .nd-split, .nd-detail { flex-direction: column; }
  .nd-compose { flex: 0 0 auto; }
  #note-text { min-height: 66px; }
  .sess-sum, .sess-who { display: none; }
}
@media (prefers-reduced-motion: reduce) {
  #notes-drawer, #notes-tab, #dnd-sess { transition: none; }
  .rec-dot, #notes-tab.rec::before { animation: none; }
}

@media print {
  /* the margin an ordinary desktop printer can reach, whatever the paper */
  @page { margin: 0.45in; }
  /* Paper has no scrollbar to make up for a size that does not quite fit, so
     the whole sheet is set one step down. Everything measured in rem follows
     the root size, and the lines per page that buys back are the point. */
  html { font-size: 14px; }
  body { background: #fff; font-size: 12.6px; line-height: 1.38; }
  /* The paper is already the colour the sheet paints on, so the wash and the
     two gradients over it are a whole page of toner spent on nothing. */
  .page { background: #fff; background-image: none; }
  #dice-canvas, #dice-tray, #dice-tab, #dice-dock,
  #notes-drawer, #notes-tab { display: none !important; }
  .rollable {
    text-decoration: none !important;
    box-shadow: none !important;
    background: none !important;
  }
  .page {
    box-shadow: none; max-width: none; padding: 0;
    display: block; min-height: 0;
  }
  .scroller { max-height: none; overflow: visible; }

  /* ---- the header, one band instead of two ----
     On screen the facts wrap under the name because there is width above
     them going spare. On paper that wrap costs the better part of an inch at
     the top of page one, so the medallion comes down a size and the facts
     sit beside the name, where they now fit. */
  header.sheet-head {
    flex-wrap: nowrap; align-items: center;
    gap: 12px; padding-bottom: 8px; margin-bottom: 12px;
  }
  .portrait { width: 130px; height: 130px; filter: none; }
  .name-block { flex: 1 1 auto; }
  .char-name { font-size: 2rem; }
  .has-portrait .char-title { font-size: 1.3rem; }
  .char-sub { font-size: .88rem; }
  .head-facts { flex: 0 1 auto; gap: 6px; justify-content: flex-end; }
  .fact { min-width: 0; padding: 3px 9px; box-shadow: none; }
  .insp-btn { padding: 3px 9px; }
  .fact .value { font-size: .95rem; }

  /* ---- the columns, poured rather than laid out ----
     On screen the three columns are a grid, and each column is one grid item.
     Paper cannot break a single item without breaking the whole row it sits
     in, so the grid printed as it stands empties the rest of every page the
     moment one column runs out - which is why the sheet used to come off the
     printer as a stack of half blank pages. Poured down a multi-column flow
     instead, the boxes fill column one to the foot of the page, then column
     two, then carry on to the next page, and a page is only short of content
     when the sheet is.

     Two columns, pinned rather than left to the screen breakpoints: the
     printable width lands either side of the 700px one depending on the
     paper, and a sheet that comes out in one column on A4 and two on letter
     is no use. */
  .columns { display: block; columns: 2; column-gap: 14px; }
  /* The boxes are what flows now, so the columns they were sorted into stand
     out of the way. Their flex gap goes with them, hence the box margin. */
  .col { display: contents; }
  .box {
    margin: 0 0 9px; padding: 7px 9px 8px;
    box-shadow: none; break-inside: avoid;
  }
  .box > h2, .box.tabbed > .tabpane > h2 {
    margin-bottom: 6px; padding-bottom: 4px; break-after: avoid;
  }
  /* Paper has no empty bottom half to fill and nothing to scroll, so the
     boxes that grow on screen go back to being as tall as their contents. */
  .box.grow, .box.grow.has-tabs > .tabpane.on {
    display: block; min-height: 0; overflow: visible;
  }
  /* These two are the long ones, and a panel that runs for three pages stops
     being a panel: it is just a tinted page, and an expensive one. They give
     up their frame and set on the paper instead, which leaves the tint to the
     short blocks it still means something on. */
  .box.grow {
    background: none; border: 0; box-shadow: none; padding: 0; margin-bottom: 0;
  }
  .box.grow > .tabpane { margin-bottom: 11px; }
  /* A box that fits in a column is kept whole. The long ones - spells,
     features, the inventory - are taller than any page and have to break
     somewhere, so they are let break at their own seams instead: between one
     feature and the next, one spell and the next, one row and the next. */
  .box.tabbed, .box.tabbed > .tabpane, .scroller, .inv-pane, .spell-level,
  .rows, table { break-inside: auto; }
  .feature, details.spell, .slot-row, .rows > .row, tr, .atk-row, .coin-row {
    break-inside: avoid;
  }
  .feature h3, .spell-level h3, .inv-pane-name { break-after: avoid; }

  /* the inventory prints as every container in turn rather than whichever
     tab happened to be open */
  .coin-spend, .coin-hint, .coin-log,
  .inv-tabs, .inv-actions, .inv-foot, .inv-modal, .cast-btn, .rest-modal,
  .sb-modal, .spell-foot, .tabbar, .hp-ctl, .def-foot, .def-chip .x,
  .cond-head .x, .cond-on .lvl { display: none !important; }
  /* on paper there is nothing to click, so every tab is printed as the
     section it was, heading and all */
  .box.has-tabs > .tabpane { display: block !important; }
  .box.has-tabs > .tabpane > h2 { display: block !important; }
  .box.has-tabs > .tabpane + .tabpane { margin-top: 12px; }
  .inv-pane { display: block !important; }
  /* on paper the toggle is just the tick it was showing */
  .wearbtn { border: 0 !important; padding: 0 !important; }
  .wearbtn:not(.on) { visibility: hidden; }
  .inv-pane-name {
    display: block; font-family: Cinzel, Georgia, serif;
    font-size: .68rem; text-transform: uppercase; color: var(--accent);
    margin: 8px 0 2px;
  }
  /* Browsers drop background colours unless "background graphics" is ticked,
     and these fills are not decoration: an empty pip means not proficient, a
     filled one means proficient, and a spent spell slot is a filled box. Ask
     for them by name so the printed sheet says what the screen one says. */
  .pip, .slot, .use-pip, .hp-bar, .hp-fill, .def-chip {
    -webkit-print-color-adjust: exact;
    print-color-adjust: exact;
  }
  details.spell[open] summary ~ * { display: block; }
  /* a line left behind on its own at a column foot reads as a mistake */
  p, .quote { orphans: 2; widows: 2; }
}
</style>
</head>
<body>
<div class="page">

  <header class="sheet-head{% if sheet.imageSrc %} has-portrait{% endif %}">
    {% if sheet.imageSrc %}
    <!-- The portrait wears the character's name and experience as its frame,
         so neither is repeated in the header beside it. -->
    <figure class="portrait" style="--fx: {{ sheet.imageFocusX }}%; --fy: {{ sheet.imageFocusY }}%; --zoom: {{ sheet.imageZoom }};">
      <div class="portrait-well"><img src="{{ sheet.imageSrc }}" alt="{{ sheet.name }}"></div>
      <svg class="portrait-ring" viewBox="0 0 210 210" aria-hidden="true">
        <defs>
          <linearGradient id="ringBand" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0" stop-color="#f8f3e6"/>
            <stop offset=".47" stop-color="#e8ddc4"/>
            <stop offset="1" stop-color="#d2c29f"/>
          </linearGradient>
          <path id="ringTop" d="M 105,105 m -78,0 a 78,78 0 1,1 156,0"/>
          <path id="ringBot" d="M 105,105 m -90,0 a 90,90 0 1,0 180,0"/>
        </defs>
        <circle class="xp-track" cx="105" cy="105" r="100"/>
        {% if sheet.xpPercent %}<circle class="xp-fill" cx="105" cy="105" r="100"
                pathLength="100" stroke-dasharray="{{ sheet.xpPercent }} 100"
                transform="rotate(-90 105 105)"/>{% endif %}
        <circle class="ring-band" cx="105" cy="105" r="83.5"/>
        <circle class="ring-edge" cx="105" cy="105" r="94.5"/>
        <circle class="ring-edge" cx="105" cy="105" r="72.5"/>
        <circle class="ring-hair" cx="105" cy="105" r="91.6"/>
        <circle class="ring-hair" cx="105" cy="105" r="75.4"/>
        <text class="ring-name" style="font-size: {% if sheet.name|length > 30 %}7.5{% elif sheet.name|length > 24 %}9{% elif sheet.name|length > 18 %}10.5{% elif sheet.name|length > 13 %}12{% else %}13.5{% endif %}px">
          <textPath href="#ringTop" startOffset="50%" text-anchor="middle">{{ sheet.name }}</textPath>
        </text>
        <text class="ring-xp">
          <textPath href="#ringBot" startOffset="50%" text-anchor="middle">{{ sheet.xp }}{% if sheet.nextLevelXp %} / {{ sheet.nextLevelXp }}{% endif %} XP</textPath>
        </text>
        <g class="ring-gem">
          <path d="M 21.5,98.2 26.8,105 21.5,111.8 16.2,105 Z"/>
          <path d="M 188.5,98.2 193.8,105 188.5,111.8 183.2,105 Z"/>
        </g>
        <g class="ring-gem-core">
          <circle cx="21.5" cy="105" r="1.3"/>
          <circle cx="188.5" cy="105" r="1.3"/>
        </g>
      </svg>
    </figure>
    {% endif %}
    <div class="name-block">
      <h1 class="char-name{% if sheet.imageSrc %} sr-only{% endif %}">{{ sheet.name }}</h1>
      <div class="char-title">{{ sheet.classLine }}</div>
      <div class="char-sub">{{ sheet.raceName }}{% if sheet.background %} &middot; {{ sheet.background }}{% endif %}</div>
    </div>
    <div class="head-facts">
      <div class="fact"><span class="label">Level</span><span class="value">{{ sheet.level }}</span></div>
      <div class="fact"><span class="label">Proficiency</span><span class="value">{{ sheet.proficiencyStr }}</span></div>
      {% if sheet.alignment %}<div class="fact"><span class="label">Alignment</span><span class="value">{{ sheet.alignment }}</span></div>{% endif %}
      {% if not sheet.imageSrc %}<div class="fact"><span class="label">Experience</span><span class="value">{{ sheet.xp }}{% if sheet.nextLevelXp %} / {{ sheet.nextLevelXp }}{% endif %}</span></div>{% endif %}
      {% if sheet.player %}<div class="fact"><span class="label">Player</span><span class="value">{{ sheet.player }}</span></div>{% endif %}
      <!-- Inspiration is a button, not a reading: pressing it posts to the
           server, which rewrites DND_INSPIRATION in the character's own org
           file. Without a script - or on paper - it is still the fact it
           always was. -->
      <div class="fact insp{% if sheet.inspiration %} on{% endif %}" id="insp">
        <button type="button" class="insp-btn" id="insp-btn"
                aria-pressed="{% if sheet.inspiration %}true{% else %}false{% endif %}"
                title="Inspiration: spend it for advantage on one roll. Press to gain or spend it.">
          <span class="label">Inspiration</span>
          <span class="value">
            <svg class="spark" viewBox="0 0 24 24" aria-hidden="true">
              <path d="M12 2 L14.6 8.6 L21.5 9.4 L16.4 14 L17.9 20.8 L12 17.3
                       L6.1 20.8 L7.6 14 L2.5 9.4 L9.4 8.6 Z"
                    fill="var(--accent-2)"/>
            </svg>
            <span class="word" id="insp-word">{% if sheet.inspiration %}Held{% else %}None{% endif %}</span>
          </span>
        </button>
      </div>
    </div>
  </header>

  <div class="columns">

    <!-- =============== left column =============== -->
    <div class="col">
      <div class="box">
        <h2>Ability Scores</h2>
        <div class="abilities">
          {% for a in sheet.abilities %}
          <div class="ability rollable" data-kind="check" data-mod="{{ a.checkStr }}"
               data-label="{{ a.name }} Check">
            <span class="tile-label">{{ a.short }}</span>
            <div class="mod">{{ a.mod }}</div>
            <span class="score">{{ a.score }}</span>
            {% if a.check != a.modifier %}<span class="check-note">check {{ a.checkStr }}</span>{% endif %}
          </div>
          {% endfor %}
        </div>
      </div>

      <div class="box">
        <h2>Saving Throws</h2>
        <div class="rows">
          {% for a in sheet.abilities %}
          <div class="row rollable" data-kind="check" data-mod="{{ a.saveStr }}"
               data-label="{{ a.name }} Saving Throw">
            <span class="pip{% if a.saveProf %} on{% endif %}"></span>
            <span class="nm">{{ a.name }}</span>
            <span class="val">{{ a.saveStr }}</span>
          </div>
          {% endfor %}
        </div>
      </div>

      <div class="box">
        <h2>Skills</h2>
        <div class="rows">
          {% for s in sheet.skills %}
          <div class="row rollable" data-kind="check" data-mod="{{ s.mod }}"
               data-label="{{ s.name }}">
            <span class="pip{% if s.expertise %} exp{% elif s.proficient %} on{% endif %}"></span>
            <span class="abbr">{{ s.short }}</span>
            <span class="nm">{{ s.name }}</span>
            <span class="val">{{ s.mod }}</span>
          </div>
          {% endfor %}
        </div>
      </div>

      <div class="box">
        <h2>Passive Senses</h2>
        <div class="rows">
          <div class="row"><span class="nm">Passive Perception</span><span class="val">{{ sheet.passivePerception }}</span></div>
          <div class="row"><span class="nm">Passive Insight</span><span class="val">{{ sheet.passiveInsight }}</span></div>
          <div class="row"><span class="nm">Passive Investigation</span><span class="val">{{ sheet.passiveInvestigation }}</span></div>
          {% if sheet.darkvision %}<div class="row"><span class="nm">Darkvision</span><span class="val">{{ sheet.darkvision }}</span></div>{% endif %}
        </div>
      </div>

      <div class="box">
        <h2>Proficiencies &amp; Languages</h2>
        <div class="rows">
          <div class="row"><span class="nm"><strong>Armor</strong> {% if sheet.armorProficiencies %}{{ sheet.armorProficiencies|join:", " }}{% else %}none{% endif %}</span></div>
          <div class="row"><span class="nm"><strong>Weapons</strong> {% if sheet.weaponProficiencies %}{{ sheet.weaponProficiencies|join:", " }}{% else %}none{% endif %}</span></div>
          <div class="row"><span class="nm"><strong>Tools</strong> {% if sheet.toolProficiencies %}{{ sheet.toolProficiencies|join:", " }}{% else %}none{% endif %}</span></div>
          <div class="row"><span class="nm"><strong>Languages</strong> {% if sheet.languages %}{{ sheet.languages|join:", " }}{% else %}none{% endif %}</span></div>
        </div>
      </div>
    </div>

    <!-- =============== middle column =============== -->
    <div class="col">
      <!-- Combat and the defenses share one box, shown a tab at a time:
           what the character does on their turn, and what is done to
           them. As everywhere else the markup is only the two sections
           stacked, so a sheet with no script - or a printed one - still
           shows both. -->
      <div class="box tabbed" data-tabs="combat">
        <div class="tabpane">
          <h2>Combat</h2>
          <div class="combat">
            <div class="tile shield">
              <span class="tile-label">Armor Class</span>
              <div class="big">{{ sheet.ac }}</div>
              <span class="sub">{{ sheet.acSource }}</span>
            </div>
            <div class="tile rollable" data-kind="check" data-mod="{{ sheet.initiativeStr }}"
                 data-label="Initiative">
              <span class="tile-label">Initiative</span>
              <div class="big">{{ sheet.initiativeStr }}</div>
              <span class="sub">dexterity</span>
            </div>
            <div class="tile">
              <span class="tile-label">Speed</span>
              <div class="big">{{ sheet.speed }}</div>
              <span class="sub">feet &middot; {{ sheet.size }}</span>
            </div>
          </div>
          <!-- Hit points, the bar under them and the hit dice line are the
               three things a rest moves, so each is named: the rest walkthrough
               rewrites them in place rather than asking for the page again. -->
          <div style="margin-top:10px">
            <div class="row" style="border:0">
              <span class="nm" id="hp-line"><strong>Hit Points</strong>{% if sheet.hpTemp %} <span class="tagline">+{{ sheet.hpTemp }} temp</span>{% endif %}</span>
              {% if sheet.hitDice %}<span class="hitdie rollable" data-kind="hitdie"
                    data-pool="{{ sheet.hitDice }}" data-mod="{{ sheet.abilityMap.con.mod }}"
                    data-label="Hit Die"
                    title="Roll one hit die and add your Constitution modifier. Spending one on a short rest is how you heal without magic; you get them back on a long rest."
                    ><span class="hd-label">Roll hit die</span><span
                     class="hd-val" id="hitdie-pool">{{ sheet.hitDice }}</span>{% if sheet.hitDiceUsed %}<span
                     class="hd-spent" id="hitdie-spent">{{ sheet.hitDiceUsed }} spent</span>{% else %}<span
                     class="hd-spent" id="hitdie-spent"></span>{% endif %}</span>{% endif %}
            </div>
            <!-- The bar reads left to right as the fraction it is: what you are
                 on now, the bar itself, and what you started the day with. -->
            <div class="hp-gauge">
              <span class="hp-now {{ sheet.health.level }}" id="hp-now"
                    title="Current hit points">{{ sheet.hpCurrent }}</span>
              <div class="hp-bar" role="img"
                   aria-label="{{ sheet.hpCurrent }} of {{ sheet.hpMax }} hit points"
                   id="hp-bar"><div class="hp-fill {{ sheet.health.level }}" id="hp-fill"
                   style="width:{{ sheet.hpPercent }}%"></div></div>
              <span class="hp-max" id="hp-max" title="Maximum hit points">{{ sheet.hpMax }}</span>
            </div>
            <!-- What happens to hit points between rests: a number and three
                 things to do with it. The panel is redrawn from what the server
                 wrote into the org file, so the sheet and the file never
                 disagree about how badly hurt somebody is. -->
            <div class="hp-temp" id="hp-temp-line">
              <span class="nm">Temporary hit points</span>
              <span class="val{% if not sheet.hpTemp %} none{% endif %}" id="hp-temp-val">{{ sheet.hpTemp }}</span>
            </div>
            <div class="hp-ctl" id="hp-ctl">
              <input type="number" class="inv-field hp-amt" id="hp-amount" min="0"
                     inputmode="numeric" placeholder="0" autocomplete="off"
                     aria-label="How many hit points">
              <button type="button" class="hpb hurt" data-hp="hurt">Hurt</button>
              <button type="button" class="hpb heal" data-hp="heal">Heal</button>
              <button type="button" class="ib" data-hp="temp"
                      title="Set your temporary hit points to that number">Temp</button>
              <span class="inv-msg" id="hp-msg"></span>
            </div>
            <div class="tagline" style="margin-top:4px">
              <span class="rollable" data-kind="check" data-mod="{{ sheet.deathSaveStr }}"
                    data-label="Death Saving Throw">death saves {{ sheet.deathSaves }}{% if sheet.deathSaveBonus %}
                    ({{ sheet.deathSaveStr }}){% endif %}</span> &middot;
              carrying {{ sheet.weight }} of {{ sheet.carryCapacity }} lb
            </div>
          </div>
        </div>

        <!-- What the character shrugs off, and what is currently wrong with
             them. Neither is derived from anything: a resistance may come from
             a race, a spell that is running or a boon the DM handed out, and a
             condition is something that happened at the table, so both are
             stored on the character and written into the org file. The panel
             is live; what is rendered here is what a sheet with no script -
             or no server - still shows. -->
        <div class="tabpane" id="defenses" data-tab="Defenses">
          <h2>Defenses</h2>
          <div id="def-live">
            <h3 class="subhead">Resistances, Immunities &amp; Vulnerabilities</h3>
            {% if sheet.defenses.any %}
            <div class="def-rows">
              {% if sheet.defenses.resistances %}<div class="def-row"><span class="def-kind res">Resistant</span>
                {% for d in sheet.defenses.resistances %}<span class="def-chip res">{{ d.name }}</span>{% endfor %}</div>{% endif %}
              {% if sheet.defenses.immunities %}<div class="def-row"><span class="def-kind imm">Immune</span>
                {% for d in sheet.defenses.immunities %}<span class="def-chip imm">{{ d.name }}</span>{% endfor %}</div>{% endif %}
              {% if sheet.defenses.vulnerabilities %}<div class="def-row"><span class="def-kind vul">Vulnerable</span>
                {% for d in sheet.defenses.vulnerabilities %}<span class="def-chip vul">{{ d.name }}</span>{% endfor %}</div>{% endif %}
            </div>
            {% else %}
            <p class="def-empty">Nothing yet. You take every kind of damage the way it comes.</p>
            {% endif %}

            <h3 class="subhead">Conditions</h3>
            {% if sheet.conditions.active %}
            <div class="cond-active">
              {% for c in sheet.conditions.active %}
              <div class="cond-on">
                <b>{{ c.label }}</b>
                {% if c.note %}<span class="tagline">{{ c.note }}</span>{% endif %}
                {% if c.text %}<p>{{ c.text }}</p>{% endif %}
              </div>
              {% endfor %}
            </div>
            {% else %}
            <p class="def-empty">Nothing is on you.</p>
            {% endif %}
          </div>
        </div>
      </div>

      <!-- What the character does and who they are share one box, shown a
           tab at a time. The markup is only the sections stacked as they
           always were - a sheet with no script shows every one of them, one
           under the other - and the bar of tabs is built from the heading
           each section already carries. -->
      <div class="box tabbed grow" data-tabs="play">
        <div class="tabpane">
          <h2>Attacks</h2>
          <div class="table-wrap">
          <table>
            <thead><tr><th>Attack</th><th class="num">Bonus</th><th>Damage</th><th>Range</th></tr></thead>
            <tbody>
            {% for a in sheet.attacks %}
              <tr>
                <td class="rollable" data-kind="check" data-mod="{{ a.bonus }}"
                    data-label="{{ a.name }} Attack">{{ a.name }}{% if a.notes %}<div class="tagline">{{ a.notes }}</div>{% endif %}</td>
                <td class="num rollable" data-kind="check" data-mod="{{ a.bonus }}"
                    data-label="{{ a.name }} Attack">{{ a.bonus }}</td>
                {% if a.damage %}<td class="rollable" data-kind="damage" data-roll="{{ a.damage }}"
                    data-label="{{ a.name }} Damage">{{ a.damage }} {{ a.type }}{% if a.versatile %}<div
                    class="tagline"><span class="rollable" data-kind="damage" data-roll="{{ a.versatile }}"
                    data-label="{{ a.name }} Damage (two handed)">{{ a.versatile }}</span> two handed</div>{% endif %}</td>
                {% else %}<td>{{ a.damage }} {{ a.type }}</td>{% endif %}
                <td>{{ a.range }}</td>
              </tr>
            {% endfor %}
            </tbody>
          </table>
          </div>
        </div>

        <!-- The inventory is live: the panel below is redrawn by the page from
             the same inventory the server computed, and every change is written
             back to the org character sheet. What is rendered here is what a
             sheet with no script - or no server - still shows. -->
        <div class="tabpane" id="inventory">
          <h2>Inventory</h2>
          <div class="enc" id="inv-enc">
            <div class="enc-line">
              <span class="wt">{{ sheet.inventory.weight }} lb</span>
              <span class="cap">of {{ sheet.inventory.carryCapacity }} lb carried</span>
              <span class="enc-badge {{ sheet.inventory.level }}">{{ sheet.inventory.label }}</span>
            </div>
            <div class="enc-bar">
              <div class="enc-fill {{ sheet.inventory.level }}" style="width:{{ sheet.inventory.percent }}%"></div>
            </div>
            <div class="enc-note">
              Encumbered over {{ sheet.inventory.encumberedAt }} lb, heavily over
              {{ sheet.inventory.heavilyEncumberedAt }} lb, push, drag or lift
              {{ sheet.inventory.pushDragLift }} lb.
              {% if sheet.inventory.stored %}Another {{ sheet.inventory.stored }} lb is stowed
              in extradimensional space and is not carried.{% endif %}
            </div>
          </div>
          <div id="inv-live">
            {% for box in sheet.inventory.containers %}
            <div class="inv-pane on">
              <h3 class="inv-pane-name">{{ box.name }}</h3>
              {% if box.capacity %}<p class="inv-cap{% if box.over %} over{% endif %}">
                <b>{{ box.weight }} lb</b> of {{ box.capacity }} lb</p>{% endif %}
              {% if box.entries %}
              <div class="table-wrap">
              <table class="inv-table">
                <thead><tr><th>Item</th><th class="num">Qty</th><th class="num">Wt</th><th>Worn</th></tr></thead>
                <tbody>
                {% for e in box.entries %}
                  <tr>
                    <td>{{ e.name }}{% if e.notes %} <span class="tagline">({{ e.notes }})</span>{% endif %}</td>
                    <td class="num">{{ e.qty }}</td>
                    <td class="num">{% if e.total %}{{ e.total }}{% endif %}</td>
                    <td>{% if e.equipped %}&#10003;{% endif %}</td>
                  </tr>
                {% endfor %}
                </tbody>
              </table>
              </div>
              {% else %}<div class="inv-empty">Empty.</div>{% endif %}
            </div>
            {% endfor %}
          </div>
          <div class="tagline" style="margin-top:6px">
            {{ sheet.purse.total }} in coin &middot; see the Coins tab
          </div>
        </div>

        <!-- The purse is live the same way the inventory is: the panel below is
             redrawn from what the server computed, and spending, earning or
             changing coin up is written back to the org character sheet. What
             is rendered here is what a sheet with no script - or no server -
             still shows. -->
        <div class="tabpane" id="coins" data-tab="Coins">
          <h2>Coins</h2>
          <div id="coin-live">
            <div class="coin-head">
              <span class="coin-total">{{ sheet.purse.total }}</span>
              <span class="coin-sub">{{ sheet.purse.count }} coins &middot; {{ sheet.purse.weight }} lb</span>
            </div>
            <div class="table-wrap">
            <table class="coin-table">
              <thead><tr><th>Coin</th><th class="num">Held</th><th class="num">Worth</th></tr></thead>
              <tbody>
              {% for c in sheet.purse.coins %}
                <tr class="coin-{{ c.id }}">
                  <td><span class="coin-pip {{ c.id }}"></span>{{ c.name }}
                      <span class="tagline">{{ c.abbr }}</span></td>
                  <td class="num">{{ c.qty }}</td>
                  <td class="num">{{ c.gold }} gp</td>
                </tr>
              {% endfor %}
              </tbody>
            </table>
            </div>
          </div>
          <p class="enc-note">
            A gold piece is 100 cp, 10 sp, 2 ep, or a tenth of a platinum piece.
            Fifty coins of any kind weigh a pound.
          </p>
        </div>

        <!-- Personality and appearance are one section, not two. They are the
             same question asked twice - who is this - and splitting them cost
             a tab to show six short lines. Appearance sits at the bottom under
             its own subheading, so it is still findable and still prints as
             its own block. The heading only says Personality when there is
             personality to show; a character with nothing but a description
             gets a section called Appearance rather than a misnamed one. -->
        {% if sheet.personality or sheet.ideals or sheet.bonds or sheet.flaws or sheet.age or sheet.height or sheet.weightStr or sheet.eyes or sheet.skin or sheet.hair or sheet.appearance %}
        <div class="tabpane">
          {% if sheet.personality or sheet.ideals or sheet.bonds or sheet.flaws %}
          <h2>Personality</h2>
          {% if sheet.personality %}<p class="quote"><span class="label">Traits</span>{{ sheet.personality }}</p>{% endif %}
          {% if sheet.ideals %}<p class="quote"><span class="label">Ideals</span>{{ sheet.ideals }}</p>{% endif %}
          {% if sheet.bonds %}<p class="quote"><span class="label">Bonds</span>{{ sheet.bonds }}</p>{% endif %}
          {% if sheet.flaws %}<p class="quote"><span class="label">Flaws</span>{{ sheet.flaws }}</p>{% endif %}
          {% if sheet.age or sheet.height or sheet.weightStr or sheet.eyes or sheet.skin or sheet.hair or sheet.appearance %}
          <h3 class="subhead">Appearance</h3>
          {% endif %}
          {% else %}
          <h2>Appearance</h2>
          {% endif %}
          {% if sheet.age or sheet.height or sheet.weightStr or sheet.eyes or sheet.skin or sheet.hair %}
          <div class="rows facts">
            {% if sheet.age %}<div class="row"><span class="nm">Age</span><span class="val">{{ sheet.age }}</span></div>{% endif %}
            {% if sheet.height %}<div class="row"><span class="nm">Height</span><span class="val">{{ sheet.height }}</span></div>{% endif %}
            {% if sheet.weightStr %}<div class="row"><span class="nm">Weight</span><span class="val">{{ sheet.weightStr }}</span></div>{% endif %}
            {% if sheet.eyes %}<div class="row"><span class="nm">Eyes</span><span class="val">{{ sheet.eyes }}</span></div>{% endif %}
            {% if sheet.skin %}<div class="row"><span class="nm">Skin</span><span class="val">{{ sheet.skin }}</span></div>{% endif %}
            {% if sheet.hair %}<div class="row"><span class="nm">Hair</span><span class="val">{{ sheet.hair }}</span></div>{% endif %}
          </div>
          {% endif %}
          {% if sheet.appearance %}<p class="quote" style="margin-top:6px">{{ sheet.appearance }}</p>{% endif %}
        </div>
        {% endif %}
      </div>
    </div>

    <!-- =============== right column =============== -->
    <div class="col">
      <!-- The spell page and the features share a box the same way the
           sections in the middle column do; see the note there. -->
      <div class="box tabbed grow" data-tabs="lore">
        {% if sheet.isCaster %}
        <!-- The spell page is live: everything inside spell-live is redrawn by
             the page from what the server computed, and learning, giving back
             or preparing a spell is written into the org character sheet. What
             is rendered here is what a sheet with no script - or no server -
             still shows. -->
        <div class="tabpane" id="spellcasting">
          <h2>Spellcasting</h2>
          <div class="spell-head">
            <div class="tile"><span class="tile-label">Ability</span><div class="big" style="font-size:1rem">{{ sheet.castingAbility }}</div></div>
            <div class="tile"><span class="tile-label">Save DC</span><div class="big">{{ sheet.spellSaveDc }}</div></div>
            <div class="tile rollable" data-kind="check" data-mod="{{ sheet.spellAttackStr }}"
                 data-label="Spell Attack"><span class="tile-label">Attack</span><div class="big">{{ sheet.spellAttackStr }}</div></div>
          </div>
          <div id="spell-live">
          <div class="tagline">
            {% if sheet.cantripsKnown %}{{ sheet.cantripsKnown }} cantrips{% endif %}
            {% if sheet.spellsKnown %} &middot; {{ sheet.spellsKnown }} spells known{% endif %}
            {% if sheet.preparedMax %} &middot; {{ sheet.spellsPrepared }}/{{ sheet.preparedMax }} prepared{% endif %}
            {% if sheet.spellNotes %} &middot; {{ sheet.spellNotes }}{% endif %}
          </div>
          <!-- One box per spell slot. Clicking an empty one spends it and
               clicking a filled one hands it back, both written straight to
               the org character sheet; casting a spell spends one by itself. -->
          {% for s in sheet.slots %}
          <div class="slot-row" data-level="{{ s.level }}" data-used="{{ s.used }}">
            <span class="lvl">{{ s.level }}{% if s.level == 1 %}st{% elif s.level == 2 %}nd{% elif s.level == 3 %}rd{% else %}th{% endif %}</span>
            {% for i in s.pips %}<span class="slot{% if i <= s.used %} used{% endif %}"></span>{% endfor %}
            <span class="tagline">{{ s.total }} slot{% if s.total > 1 %}s{% endif %}</span>
          </div>
          {% endfor %}

          {% for lvl in sheet.spellLevels %}
          <div class="spell-level">
            <h3>{{ lvl.name }}{% if lvl.slots %} ({{ lvl.slots }} slot{% if lvl.slots > 1 %}s{% endif %}){% endif %}</h3>
            {% for sp in lvl.spells %}
            <details class="spell">
              <summary>
                {% if sp.prepared and lvl.level > 0 %}<span class="prep">&#9679;</span> {% endif %}{{ sp.name }}
                <span class="tagline">&mdash; {{ sp.castingTime }}, {{ sp.range }}{% if sp.concentration %}, concentration{% endif %}{% if sp.ritual %}, ritual{% endif %}</span>
                <!-- Casting rolls the attack and the damage in one go, and says
                     what the target has to roll back. Everything it needs was
                     worked out by the rules engine. -->
                <button type="button" class="cast-btn" data-spell="{{ sp.name }}"
                        data-detail="{{ sp.cast.detail }}" data-line="{{ sp.cast.line }}"
                        data-short="{{ sp.cast.short }}" data-level="{{ lvl.level }}"
                        {% if sp.ritual %}data-ritual="1"{% endif %}
                        {% if sp.cast.attack %}data-attack="{{ sp.cast.attackBonus }}"{% endif %}
                        {% if sp.cast.damage %}data-damage="{{ sp.cast.damage }}"
                        data-damage-type="{{ sp.cast.damageType }}"{% endif %}
                        {% if sp.cast.heal %}data-heal="{{ sp.cast.heal }}"{% endif %}
                        {% if sp.cast.save %}data-save="{{ sp.cast.saveName }}"
                        data-dc="{{ sp.cast.saveDc }}"{% endif %}
                        title="Cast {{ sp.name }}">Cast</button>
              </summary>
              <p><em>{{ sp.school }}{% if sp.components %} &middot; {{ sp.components }}{% endif %}{% if sp.duration %} &middot; {{ sp.duration }}{% endif %}</em>
  {{ sp.text }}{% if sp.higherLevel %}

  <strong>At higher levels.</strong> {{ sp.higherLevel }}{% endif %}</p>
            </details>
            {% endfor %}
          </div>
          {% endfor %}
          </div>
          <div class="spell-foot">
            <button type="button" class="spell-manage" id="spell-manage-btn">Manage spells</button>
            <span id="spell-budgets"></span>
          </div>
        </div>
        {% endif %}

        <!-- A feature the rules ration - "twice, and you regain both on a
             short rest" - wears a row of slots for those uses, the same way
             spell slots are drawn. Clicking one spends a use and writes it to
             the org character sheet; a rest gives them all back. Which
             features have a limit, and how many, the rules engine worked out
             from each feature's own text, or off the class level table for
             the resources printed there rather than written out - rage, ki,
             sorcery points. -->
        <div class="tabpane">
          <h2>Features &amp; Traits</h2>
          <div class="scroller" id="feature-live">
            {% for t in sheet.traits %}
            <div class="feature{% if t.usesMax %} limited{% endif %}"{% if t.usesMax %} data-uses="{{ t.usesId }}" data-uses-name="{{ t.name }}" data-recharge="{{ t.recharge }}"{% endif %}>
              <h3>{{ t.name }}</h3>
              {% if t.source %}<span class="src">{{ t.source }}</span>{% endif %}
              {% if t.usesMax %}<div class="uses" data-max="{{ t.usesMax }}" data-spent="{{ t.usesSpent }}">
                {% for i in t.usesPips %}<span class="use-pip{% if i <= t.usesSpent %} used{% endif %}"></span>{% endfor %}
                <span class="uses-note">{{ t.usesNote }}</span>
              </div>{% endif %}
              <p>{{ t.text }}</p>
            </div>
            {% endfor %}
            {% for f in sheet.features %}
            <div class="feature{% if f.usesMax %} limited{% endif %}"{% if f.usesMax %} data-uses="{{ f.usesId }}" data-uses-name="{{ f.name }}" data-recharge="{{ f.recharge }}"{% endif %}>
              <h3>{{ f.name }}</h3>
              {% if f.source %}<span class="src">{{ f.source }}</span>{% endif %}
              {% if f.usesMax %}<div class="uses" data-max="{{ f.usesMax }}" data-spent="{{ f.usesSpent }}">
                {% for i in f.usesPips %}<span class="use-pip{% if i <= f.usesSpent %} used{% endif %}"></span>{% endfor %}
                <span class="uses-note">{{ f.usesNote }}</span>
              </div>{% endif %}
              <p>{{ f.text }}</p>
            </div>
            {% endfor %}
          </div>
        </div>
      </div>

      {% if sheet.backstory or sheet.allies or sheet.treasure or sheet.notes %}
      <div class="box">
        <h2>Journal</h2>
        {% if sheet.backstory %}<p class="quote"><span class="label">Backstory</span>{{ sheet.backstory }}</p>{% endif %}
        {% if sheet.allies %}<p class="quote"><span class="label">Allies &amp; Organizations</span>{{ sheet.allies }}</p>{% endif %}
        {% if sheet.treasure %}<p class="quote"><span class="label">Treasure</span>{{ sheet.treasure }}</p>{% endif %}
        {% if sheet.notes %}<p class="quote"><span class="label">Notes</span>{{ sheet.notes }}</p>{% endif %}
      </div>
      {% endif %}
    </div>
  </div>

  {% if sheet.warnings %}
  <div class="box" style="margin-top:16px">
    <h2>Sheet Warnings</h2>
    {% for w in sheet.warnings %}<div class="warn">{{ w }}</div>{% endfor %}
  </div>
  {% endif %}

  <footer class="sheet-foot">
    <span>{{ sheet.name }} &middot; {{ sheet.classLine }} &middot; {{ sheet.raceName }}</span>
    <span>generated by orgs &middot; {{ sheet.rulesetName }}</span>
  </footer>
</div>

<div id="dnd-log-config" hidden
     data-server="{{ serverUrl }}" data-character="{{ sheet.name }}"
     data-character-id="{{ sheet.id }}" data-file="{{ sheetFile }}"></div>
<script type="application/json" id="dnd-inventory-data">{{ inventoryJson|safe }}</script>
<script type="application/json" id="dnd-money-data">{{ moneyJson|safe }}</script>
<script type="application/json" id="dnd-defenses-data">{{ defensesJson|safe }}</script>
<script type="application/json" id="dnd-health-data">{{ healthJson|safe }}</script>
<noscript><style>.rollable { cursor: auto; text-decoration: none; }</style></noscript>
<script>
/* ---------------------------------------------------------------------------
   Dice tray: rollable sheet elements, a 3d dice throw and a roll history tray.
   Everything here is self contained, the exported sheet has no dependencies.
--------------------------------------------------------------------------- */
(function () {
  'use strict';

  // ------------------------------------------------------------------ maths
  var TAU = Math.PI * 2;
  var PHI = (1 + Math.sqrt(5)) / 2;

  function v3(x, y, z) { return [x, y, z]; }
  function sub(a, b) { return [a[0] - b[0], a[1] - b[1], a[2] - b[2]]; }
  function add(a, b) { return [a[0] + b[0], a[1] + b[1], a[2] + b[2]]; }
  function scale(a, s) { return [a[0] * s, a[1] * s, a[2] * s]; }
  function dot(a, b) { return a[0] * b[0] + a[1] * b[1] + a[2] * b[2]; }
  function cross(a, b) {
    return [a[1] * b[2] - a[2] * b[1], a[2] * b[0] - a[0] * b[2], a[0] * b[1] - a[1] * b[0]];
  }
  function len(a) { return Math.sqrt(dot(a, a)); }
  function norm(a) { var l = len(a) || 1; return [a[0] / l, a[1] / l, a[2] / l]; }
  function centroid(pts) {
    var c = [0, 0, 0];
    for (var i = 0; i < pts.length; i++) { c[0] += pts[i][0]; c[1] += pts[i][1]; c[2] += pts[i][2]; }
    return scale(c, 1 / (pts.length || 1));
  }

  // Quaternions are [x, y, z, w].
  function qMul(a, b) {
    return [
      a[3] * b[0] + a[0] * b[3] + a[1] * b[2] - a[2] * b[1],
      a[3] * b[1] - a[0] * b[2] + a[1] * b[3] + a[2] * b[0],
      a[3] * b[2] + a[0] * b[1] - a[1] * b[0] + a[2] * b[3],
      a[3] * b[3] - a[0] * b[0] - a[1] * b[1] - a[2] * b[2]
    ];
  }
  function qNorm(q) {
    var l = Math.sqrt(q[0] * q[0] + q[1] * q[1] + q[2] * q[2] + q[3] * q[3]) || 1;
    return [q[0] / l, q[1] / l, q[2] / l, q[3] / l];
  }
  function qFromAxis(axis, angle) {
    var a = norm(axis), s = Math.sin(angle / 2);
    return [a[0] * s, a[1] * s, a[2] * s, Math.cos(angle / 2)];
  }
  function qRotate(q, v) {
    var qx = q[0], qy = q[1], qz = q[2], qw = q[3];
    var tx = 2 * (qy * v[2] - qz * v[1]);
    var ty = 2 * (qz * v[0] - qx * v[2]);
    var tz = 2 * (qx * v[1] - qy * v[0]);
    return [
      v[0] + qw * tx + (qy * tz - qz * ty),
      v[1] + qw * ty + (qz * tx - qx * tz),
      v[2] + qw * tz + (qx * ty - qy * tx)
    ];
  }
  // Shortest rotation taking unit vector a onto unit vector b.
  function qBetween(a, b) {
    var d = dot(a, b);
    if (d > 0.999999) { return [0, 0, 0, 1]; }
    if (d < -0.999999) {
      var axis = cross([1, 0, 0], a);
      if (len(axis) < 1e-6) { axis = cross([0, 1, 0], a); }
      return qFromAxis(axis, Math.PI);
    }
    var c = cross(a, b);
    return qNorm([c[0], c[1], c[2], 1 + d]);
  }
  function qSlerp(a, b, t) {
    var d = a[0] * b[0] + a[1] * b[1] + a[2] * b[2] + a[3] * b[3];
    var bb = b.slice();
    if (d < 0) { d = -d; bb = [-b[0], -b[1], -b[2], -b[3]]; }
    if (d > 0.9995) {
      return qNorm([a[0] + (bb[0] - a[0]) * t, a[1] + (bb[1] - a[1]) * t,
                    a[2] + (bb[2] - a[2]) * t, a[3] + (bb[3] - a[3]) * t]);
    }
    var theta = Math.acos(d), s = Math.sin(theta);
    var w1 = Math.sin((1 - t) * theta) / s, w2 = Math.sin(t * theta) / s;
    return qNorm([a[0] * w1 + bb[0] * w2, a[1] * w1 + bb[1] * w2,
                  a[2] * w1 + bb[2] * w2, a[3] * w1 + bb[3] * w2]);
  }

  // A tiny deterministic prng, so a face keeps the same marbling every frame.
  function seeded(seed) {
    var s = (seed * 1103515245 + 12345) >>> 0;
    return function () {
      s = (s * 1664525 + 1013904223) >>> 0;
      return s / 4294967296;
    };
  }

  // -------------------------------------------------------------- geometry
  // Newell normal of a polygon, used to force every face to wind outwards.
  function faceNormal(verts, idx) {
    var n = [0, 0, 0];
    for (var i = 0; i < idx.length; i++) {
      var a = verts[idx[i]], b = verts[idx[(i + 1) % idx.length]];
      n[0] += (a[1] - b[1]) * (a[2] + b[2]);
      n[1] += (a[2] - b[2]) * (a[0] + b[0]);
      n[2] += (a[0] - b[0]) * (a[1] + b[1]);
    }
    return norm(n);
  }

  // Every solid here is convex and centred on the origin, so a face whose
  // normal points back towards the centre is simply wound the wrong way.
  function fixWinding(verts, faces) {
    return faces.map(function (idx) {
      var n = faceNormal(verts, idx);
      var c = centroid(idx.map(function (i) { return verts[i]; }));
      return dot(n, c) < 0 ? idx.slice().reverse() : idx.slice();
    });
  }

  // Brute force convex hull face finder. Every triple of vertices spans a
  // candidate plane; it is a real face when the whole solid sits on one side
  // of it. Twenty vertices is nothing, and it beats hand written index
  // tables for the dodecahedron and the icosahedron.
  function hullFaces(verts) {
    var planes = [], seen = {}, eps = 1e-6;
    for (var i = 0; i < verts.length; i++) {
      for (var j = i + 1; j < verts.length; j++) {
        for (var k = j + 1; k < verts.length; k++) {
          var n = cross(sub(verts[j], verts[i]), sub(verts[k], verts[i]));
          if (len(n) < eps) { continue; }
          n = norm(n);
          var d = dot(n, verts[i]);
          var above = 0, below = 0;
          for (var m = 0; m < verts.length; m++) {
            var t = dot(verts[m], n) - d;
            if (t > eps) { above++; } else if (t < -eps) { below++; }
          }
          if (above && below) { continue; }
          if (above) { n = scale(n, -1); d = -d; }
          if (d <= eps) { continue; }
          var key = n[0].toFixed(4) + ',' + n[1].toFixed(4) + ',' + n[2].toFixed(4);
          if (seen[key]) { continue; }
          seen[key] = 1;
          planes.push([n, d]);
        }
      }
    }
    return planes.map(function (pl) {
      var n = pl[0], d = pl[1], idx = [];
      verts.forEach(function (v, i) { if (Math.abs(dot(v, n) - d) < 1e-5) { idx.push(i); } });
      var c = centroid(idx.map(function (i) { return verts[i]; }));
      var u = norm(sub(verts[idx[0]], c));
      var w = cross(n, u);
      idx.sort(function (a, b) {
        var va = sub(verts[a], c), vb = sub(verts[b], c);
        return Math.atan2(dot(va, w), dot(va, u)) - Math.atan2(dot(vb, w), dot(vb, u));
      });
      return idx;
    });
  }

  // All sign and cyclic permutations of a coordinate triple, the usual way of
  // writing out icosahedral vertex sets.
  function permute(a, b, c, cyclic) {
    var out = [], seen = {};
    var base = cyclic ? [[a, b, c], [b, c, a], [c, a, b]] : [[a, b, c]];
    base.forEach(function (t) {
      [1, -1].forEach(function (sx) {
        [1, -1].forEach(function (sy) {
          [1, -1].forEach(function (sz) {
            var p = [t[0] * sx, t[1] * sy, t[2] * sz];
            var key = p.map(function (n) { return n.toFixed(5); }).join(',');
            if (!seen[key]) { seen[key] = 1; out.push(p); }
          });
        });
      });
    });
    return out;
  }

  // Pair opposite faces up and number them so that they sum to max + 1, the
  // way real dice are numbered.
  function labelAntipodal(normals, max) {
    var labels = new Array(normals.length).fill(0);
    var next = 1;
    for (var i = 0; i < normals.length; i++) {
      if (labels[i]) { continue; }
      var best = -1, bestDot = -0.5;
      for (var j = 0; j < normals.length; j++) {
        if (j === i || labels[j]) { continue; }
        var d = dot(normals[i], normals[j]);
        if (d < bestDot) { bestDot = d; best = j; }
      }
      labels[i] = next;
      if (best >= 0) { labels[best] = max + 1 - next; }
      next++;
    }
    return labels;
  }

  function makeSolid(verts, faces, sides, sizeFactor) {
    var maxR = 0;
    verts.forEach(function (v) { maxR = Math.max(maxR, len(v)); });
    var vs = verts.map(function (v) { return scale(v, sizeFactor / maxR); });
    var fs = fixWinding(vs, faces);
    var normals = fs.map(function (idx) { return faceNormal(vs, idx); });
    var labels = labelAntipodal(normals, sides);
    // Face radius drives the size of the numeral and the marbling.
    var radii = fs.map(function (idx, i) {
      var c = centroid(idx.map(function (k) { return vs[k]; }));
      var r = Infinity;
      for (var k = 0; k < idx.length; k++) {
        var a = vs[idx[k]], b = vs[idx[(k + 1) % idx.length]];
        var ab = sub(b, a), ac = sub(c, a);
        var t = Math.max(0, Math.min(1, dot(ac, ab) / (dot(ab, ab) || 1)));
        r = Math.min(r, len(sub(ac, scale(ab, t))));
      }
      return r;
    });
    return { verts: vs, faces: fs, normals: normals, labels: labels, radii: radii, sides: sides };
  }

  var SOLIDS = {};
  function solid(sides) {
    if (SOLIDS[sides]) { return SOLIDS[sides]; }
    var g;
    if (sides === 4) {
      g = makeSolid(
        [[1, 1, 1], [1, -1, -1], [-1, 1, -1], [-1, -1, 1]],
        [[0, 1, 2], [0, 3, 1], [0, 2, 3], [1, 3, 2]], 4, 1.06);
    } else if (sides === 6) {
      var cv = [[-1, -1, -1], [1, -1, -1], [1, 1, -1], [-1, 1, -1],
                [-1, -1, 1], [1, -1, 1], [1, 1, 1], [-1, 1, 1]];
      g = makeSolid(cv, [[4, 5, 6, 7], [0, 3, 2, 1], [1, 2, 6, 5],
                         [0, 4, 7, 3], [3, 7, 6, 2], [0, 1, 5, 4]], 6, 0.94);
    } else if (sides === 8) {
      var ov = [[1, 0, 0], [-1, 0, 0], [0, 1, 0], [0, -1, 0], [0, 0, 1], [0, 0, -1]];
      g = makeSolid(ov, [[0, 2, 4], [2, 1, 4], [1, 3, 4], [3, 0, 4],
                         [2, 0, 5], [1, 2, 5], [3, 1, 5], [0, 3, 5]], 8, 1.0);
    } else if (sides === 10 || sides === 100) {
      // Pentagonal trapezohedron. The apex height is the one that keeps the
      // kite faces flat for a ring at z = +-a.
      var a = 0.12, h = 9.47216 * a, vv = [[0, 0, h], [0, 0, -h]];
      for (var i = 0; i < 5; i++) {
        vv.push([Math.cos(i * TAU / 5), Math.sin(i * TAU / 5), a]);
      }
      for (var j = 0; j < 5; j++) {
        vv.push([Math.cos(j * TAU / 5 + TAU / 10), Math.sin(j * TAU / 5 + TAU / 10), -a]);
      }
      var tf = [];
      for (var k = 0; k < 5; k++) {
        tf.push([0, 2 + k, 7 + k, 2 + (k + 1) % 5]);
        tf.push([1, 7 + k, 2 + (k + 1) % 5, 7 + (k + 1) % 5]);
      }
      g = makeSolid(vv, tf, 10, 1.02);
      if (sides === 100) {
        g = { verts: g.verts, faces: g.faces, normals: g.normals, radii: g.radii,
              sides: 100, labels: g.labels.map(function (n) { return n % 10 * 10; }) };
      }
    } else if (sides === 12) {
      var dv = permute(1, 1, 1, false).concat(permute(0, 1 / PHI, PHI, true));
      g = makeSolid(dv, hullFaces(dv), 12, 1.07);
    } else {
      var iv = permute(0, 1, PHI, true);
      g = makeSolid(iv, hullFaces(iv), 20, 1.12);
    }
    SOLIDS[sides] = g;
    return g;
  }

  // ------------------------------------------------------------- marbling
  // Black marble with white granite veining, generated once per face and
  // then reused, so the stone pattern stays put as the die tumbles.
  function marbleFor(g, fi) {
    if (!g.marble) { g.marble = []; }
    if (g.marble[fi]) { return g.marble[fi]; }
    var rnd = seeded(g.sides * 977 + fi * 31 + 7);
    var r = g.radii[fi], blotches = [], veins = [], i, k;
    for (i = 0; i < 5; i++) {
      blotches.push({
        x: (rnd() - 0.5) * 2.4 * r,
        y: (rnd() - 0.5) * 2.4 * r,
        r: r * (0.35 + rnd() * 0.75),
        light: rnd() < 0.4
      });
    }
    for (i = 0; i < 4; i++) {
      var ang = rnd() * TAU, x = Math.cos(ang) * r * 1.9, y = Math.sin(ang) * r * 1.9;
      var dir = ang + Math.PI + (rnd() - 0.5) * 1.1;
      var pts = [{ x: x, y: y }];
      var steps = 3 + Math.floor(rnd() * 3);
      for (k = 0; k < steps; k++) {
        dir += (rnd() - 0.5) * 1.3;
        var stepLen = r * (0.5 + rnd() * 0.8);
        x += Math.cos(dir) * stepLen;
        y += Math.sin(dir) * stepLen;
        pts.push({ x: x, y: y });
      }
      veins.push({ pts: pts, w: r * (0.035 + rnd() * 0.075), a: 0.3 + rnd() * 0.45 });
    }
    g.marble[fi] = { blotches: blotches, veins: veins };
    return g.marble[fi];
  }

  // ------------------------------------------------------------ dice board
  // The sheet is the table. World x and y are viewport pixels lying flat on
  // the page, z is height above it, and the camera hangs over the middle of
  // the viewport looking straight down.
  var GRAVITY = 4200;
  var CAM_Z = 1500;
  var LIGHT = norm([-0.36, -0.52, 0.78]);

  function DiceBoard(canvas) {
    this.canvas = canvas;
    this.ctx = canvas.getContext('2d');
    this.dice = [];
    this.phase = 'idle';
    this.raf = 0;
    this.last = 0;
    this.melt = null;
    this.dpr = 1;
    this.resize();
    var self = this;
    window.addEventListener('resize', function () { self.resize(); });
  }

  DiceBoard.prototype.resize = function () {
    var dpr = window.devicePixelRatio || 1;
    this.dpr = dpr;
    this.w = this.canvas.clientWidth || window.innerWidth;
    this.h = this.canvas.clientHeight || window.innerHeight;
    this.canvas.width = Math.round(this.w * dpr);
    this.canvas.height = Math.round(this.h * dpr);
    this.ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
  };

  // project maps a world point onto the canvas.
  DiceBoard.prototype.project = function (p) {
    var s = CAM_Z / Math.max(120, CAM_Z - p[2]);
    return [this.w / 2 + (p[0] - this.w / 2) * s, this.h / 2 + (p[1] - this.h / 2) * s, s];
  };

  // The parchment itself is the table, so the dice bounce off the edges of
  // the page rather than the edges of the window.
  DiceBoard.prototype.tableBounds = function () {
    var page = document.querySelector('.page');
    var l = 0, t = 0, r = this.w, b = this.h;
    if (page) {
      var pr = page.getBoundingClientRect();
      l = Math.max(0, pr.left);
      r = Math.min(this.w, pr.right);
      t = Math.max(0, pr.top);
      b = Math.min(this.h, pr.bottom);
    }
    if (r - l < 140) { l = 0; r = this.w; }
    if (b - t < 140) { t = 0; b = this.h; }
    return { l: l, t: t, r: r, b: b };
  };

  DiceBoard.prototype.scrollShift = function () {
    return this.scrollY0 - (window.pageYOffset || document.documentElement.scrollTop || 0);
  };

  DiceBoard.prototype.stop = function () {
    if (this.raf) { cancelAnimationFrame(this.raf); this.raf = 0; }
    this.phase = 'idle';
    this.dice = [];
    this.melt = null;
    this.ctx.clearRect(0, 0, this.w, this.h);
  };

  // throwDice tosses one die per entry of spec, arcing from the click over to
  // the clear patch of sheet picked by the caller. The whole sequence is kept
  // to roughly two and a half seconds: a die that takes longer than that to
  // tell you its number is in the way, not in the scene.
  DiceBoard.prototype.throwDice = function (spec, from, target) {
    this.stop();
    this.resize();
    this.scrollY0 = window.pageYOffset || document.documentElement.scrollTop || 0;
    this.table = this.tableBounds();
    var self = this, n = spec.length, tb = this.table;
    var spread = Math.min(150, 46 + n * 16);
    this.dice = spec.map(function (d, i) {
      var g = solid(d.sides);
      var size = (d.sides === 20 ? 42 : d.sides === 12 ? 40 : d.sides === 4 ? 36 : 38);
      var jitter = n === 1 ? 0 : (i / (n - 1) - 0.5) * 2;
      var tx = target.x + jitter * spread + (Math.random() - 0.5) * 34;
      var ty = target.y + (Math.random() - 0.5) * spread * 0.7;
      // Scatter cannot push a die off the edge of the sheet.
      var pad = size * 1.7;
      tx = Math.max(tb.l + pad, Math.min(tb.r - pad, tx));
      ty = Math.max(tb.t + pad, Math.min(tb.b - pad, ty));
      var sx = from.x + (Math.random() - 0.5) * 40;
      var sy = from.y + (Math.random() - 0.5) * 40;
      var h0 = 105 + Math.random() * 55;
      var flight = 0.40 + Math.random() * 0.07;
      var rest = size * 0.92;
      return {
        g: g, sides: d.sides, value: d.value, size: size,
        pos: [sx, sy, h0],
        vel: [(tx - sx) / flight, (ty - sy) / flight,
              (rest - h0 + 0.5 * GRAVITY * flight * flight) / flight],
        quat: qNorm([Math.random() - 0.5, Math.random() - 0.5, Math.random() - 0.5, Math.random() - 0.5]),
        spin: [(Math.random() - 0.5) * 30, (Math.random() - 0.5) * 30, (Math.random() - 0.5) * 30],
        rest: rest, settled: false, target: null, alignT: 0
      };
    });
    this.phase = 'roll';
    this.t0 = performance.now();
    this.last = this.t0;
    this.phaseAt = this.t0;
    var tick = function (now) {
      self.raf = requestAnimationFrame(tick);
      self.step(now);
    };
    this.raf = requestAnimationFrame(tick);
  };

  DiceBoard.prototype.step = function (now) {
    var dt = Math.min(0.032, (now - this.last) / 1000);
    this.last = now;
    var elapsed = now - this.t0;
    var i, d;

    if (this.phase === 'roll') {
      var allRest = true;
      for (i = 0; i < this.dice.length; i++) {
        d = this.dice[i];
        this.integrate(d, dt);
        if (!d.settled) { allRest = false; }
      }
      // A hard cap keeps a pathological bounce from stalling the roll.
      if (allRest || elapsed > 1250) {
        for (i = 0; i < this.dice.length; i++) { this.beginAlign(this.dice[i]); }
        this.phase = 'align';
        this.phaseAt = now;
      }
    } else if (this.phase === 'align') {
      var t = Math.min(1, (now - this.phaseAt) / 220);
      var e = t * t * (3 - 2 * t);
      for (i = 0; i < this.dice.length; i++) {
        d = this.dice[i];
        d.quat = qSlerp(d.from, d.target, e);
        d.pos[2] = d.restFrom + (d.rest - d.restFrom) * e;
      }
      if (t >= 1) { this.phase = 'hold'; this.phaseAt = now; }
    } else if (this.phase === 'hold') {
      if (now - this.phaseAt > 900) {
        this.startMelt();
        this.phase = 'melt';
        this.phaseAt = now;
      }
    } else if (this.phase === 'melt') {
      if (now - this.phaseAt > 620) { this.stop(); return; }
    }

    // Everything is drawn relative to where the page was when the dice were
    // thrown, so they stay put on the parchment if the reader scrolls.
    var ctx = this.ctx;
    ctx.clearRect(0, 0, this.w, this.h);
    ctx.save();
    ctx.translate(0, this.scrollShift());
    if (this.phase === 'melt') {
      this.drawMelt((now - this.phaseAt) / 0.62 / 1000);
    } else {
      this.draw(ctx);
    }
    ctx.restore();
  };

  DiceBoard.prototype.integrate = function (d, dt) {
    if (d.settled) { return; }
    d.vel[2] -= GRAVITY * dt;
    d.pos[0] += d.vel[0] * dt;
    d.pos[1] += d.vel[1] * dt;
    d.pos[2] += d.vel[2] * dt;

    var sp = len(d.spin);
    if (sp > 1e-4) { d.quat = qNorm(qMul(qFromAxis(d.spin, sp * dt), d.quat)); }

    // The edges of the sheet are the rim of the table.
    var m = d.size * 1.5, t = this.table;
    if (d.pos[0] < t.l + m) { d.pos[0] = t.l + m; d.vel[0] = Math.abs(d.vel[0]) * 0.5; }
    if (d.pos[0] > t.r - m) { d.pos[0] = t.r - m; d.vel[0] = -Math.abs(d.vel[0]) * 0.5; }
    if (d.pos[1] < t.t + m) { d.pos[1] = t.t + m; d.vel[1] = Math.abs(d.vel[1]) * 0.5; }
    if (d.pos[1] > t.b - m) { d.pos[1] = t.b - m; d.vel[1] = -Math.abs(d.vel[1]) * 0.5; }

    if (d.pos[2] <= d.rest) {
      d.pos[2] = d.rest;
      // Lively enough to read as a bounce, damped hard enough that the
      // third one is the last.
      if (d.vel[2] < -55) {
        d.vel[2] = -d.vel[2] * 0.46;
        d.vel[0] *= 0.7; d.vel[1] *= 0.7;
        d.spin = scale(d.spin, 0.55);
      } else {
        d.vel[2] = 0;
      }
      // Rolling friction once it is down on the paper.
      var f = Math.max(0, 1 - 5.5 * dt);
      d.vel[0] *= f; d.vel[1] *= f;
      d.spin = scale(d.spin, Math.max(0, 1 - 6.5 * dt));
      if (Math.abs(d.vel[0]) + Math.abs(d.vel[1]) < 40 && len(d.spin) < 2.2) {
        d.settled = true;
      }
    }
  };

  DiceBoard.prototype.beginAlign = function (d) {
    var fi = d.g.labels.indexOf(d.value);
    if (fi < 0) { fi = 0; }
    var worldN = qRotate(d.quat, d.g.normals[fi]);
    d.from = d.quat;
    d.target = qNorm(qMul(qBetween(worldN, [0, 0, 1]), d.quat));
    d.restFrom = d.pos[2];
    d.settled = true;
    d.vel = [0, 0, 0];
    d.spin = [0, 0, 0];
  };

  // ---------------------------------------------------------------- drawing
  // Dice nearer the bottom of the sheet draw last so they overlap correctly.
  DiceBoard.prototype.order = function () {
    return this.dice.slice().sort(function (a, b) { return a.pos[1] - b.pos[1]; });
  };

  DiceBoard.prototype.draw = function (ctx) {
    this.drawShadows(ctx);
    this.drawDice(ctx);
  };

  DiceBoard.prototype.drawShadows = function (ctx) {
    var self = this;
    this.order().forEach(function (d) { self.drawShadow(ctx, d); });
  };

  DiceBoard.prototype.drawDice = function (ctx) {
    var self = this;
    this.order().forEach(function (d) { self.drawDie(ctx, d); });
  };

  // The dice cast onto the sheet itself, which is what sells the page as a
  // table top: the higher the die, the wider and fainter its shadow.
  DiceBoard.prototype.drawShadow = function (ctx, d) {
    var lift = Math.max(0, d.pos[2] - d.rest);
    var fade = Math.max(0, 1 - lift / 420);
    if (fade <= 0.01) { return; }
    var r = d.size * (1.05 + lift / 900);
    var x = d.pos[0] + lift * 0.16, y = d.pos[1] + d.size * 0.35 + lift * 0.22;
    var grad = ctx.createRadialGradient(x, y, r * 0.15, x, y, r);
    grad.addColorStop(0, 'rgba(24,18,10,' + (0.42 * fade).toFixed(3) + ')');
    grad.addColorStop(0.6, 'rgba(24,18,10,' + (0.2 * fade).toFixed(3) + ')');
    grad.addColorStop(1, 'rgba(24,18,10,0)');
    ctx.save();
    ctx.fillStyle = grad;
    ctx.beginPath();
    ctx.ellipse(x, y, r, r * 0.62, 0, 0, TAU);
    ctx.fill();
    ctx.restore();
  };

  DiceBoard.prototype.drawDie = function (ctx, d) {
    var g = d.g, self = this;
    var base = ctx.getTransform();
    var wv = g.verts.map(function (v) {
      var r = qRotate(d.quat, scale(v, d.size));
      return [r[0] + d.pos[0], r[1] + d.pos[1], r[2] + d.pos[2]];
    });
    var pv = wv.map(function (p) { return self.project(p); });
    var cam = [this.w / 2, this.h / 2, CAM_Z];

    for (var i = 0; i < g.faces.length; i++) {
      var idx = g.faces[i];
      var n = qRotate(d.quat, g.normals[i]);
      var c = centroid(idx.map(function (k) { return wv[k]; }));
      var view = norm(sub(cam, c));
      var facing = dot(n, view);
      if (facing <= 0.015) { continue; }

      ctx.save();
      ctx.beginPath();
      for (var k = 0; k < idx.length; k++) {
        var p = pv[idx[k]];
        if (k === 0) { ctx.moveTo(p[0], p[1]); } else { ctx.lineTo(p[0], p[1]); }
      }
      ctx.closePath();

      var diff = Math.max(0, dot(n, LIGHT));
      var half = norm(add(LIGHT, view));
      var spec = Math.pow(Math.max(0, dot(n, half)), 26) * 0.75;
      var lo = Math.round(9 + 26 * diff * diff);
      var hi = Math.round(20 + 52 * diff);
      var pc = this.project(c);
      var lp = this.project(add(c, scale(LIGHT, d.size)));
      var grd = ctx.createLinearGradient(lp[0], lp[1], 2 * pc[0] - lp[0], 2 * pc[1] - lp[1]);
      grd.addColorStop(0, 'rgb(' + (hi + 8) + ',' + (hi + 7) + ',' + (hi + 11) + ')');
      grd.addColorStop(1, 'rgb(' + lo + ',' + lo + ',' + (lo + 3) + ')');
      ctx.fillStyle = grd;
      ctx.fill();

      ctx.clip();
      this.paintFace(ctx, base, d, i, c, n, wv, idx, facing, spec);
      ctx.restore();

      // A cool rim on every visible edge reads as a polished bevel.
      ctx.save();
      ctx.beginPath();
      for (var m = 0; m < idx.length; m++) {
        var q = pv[idx[m]];
        if (m === 0) { ctx.moveTo(q[0], q[1]); } else { ctx.lineTo(q[0], q[1]); }
      }
      ctx.closePath();
      ctx.lineWidth = 1.1;
      ctx.strokeStyle = 'rgba(196,196,214,' + (0.10 + 0.30 * diff).toFixed(3) + ')';
      ctx.stroke();
      ctx.restore();
    }
  };

  // paintFace draws the marbling and the numeral in the plane of the face,
  // so both stay glued to the stone as the die turns.
  DiceBoard.prototype.paintFace = function (ctx, base, d, fi, c, n, wv, idx, facing, spec) {
    var g = d.g;
    var u = norm(sub(wv[idx[0]], c));
    var w = cross(n, u);
    var s = 12;
    var p0 = this.project(c);
    var pu = this.project(add(c, scale(w, s)));
    var pv = this.project(add(c, scale(u, -s)));
    var a = (pu[0] - p0[0]) / s, b = (pu[1] - p0[1]) / s;
    var cc = (pv[0] - p0[0]) / s, dd = (pv[1] - p0[1]) / s;
    if (a * dd - b * cc < 0) { a = -a; b = -b; }

    ctx.setTransform(base);
    ctx.transform(a, b, cc, dd, p0[0], p0[1]);

    var r = g.radii[fi] * d.size;
    var mb = marbleFor(g, fi);
    var i;

    // mottling
    for (i = 0; i < mb.blotches.length; i++) {
      var bl = mb.blotches[i];
      var bx = bl.x * d.size, by = bl.y * d.size, br = bl.r * d.size;
      var bg = ctx.createRadialGradient(bx, by, 0, bx, by, br);
      var tint = bl.light ? '108,104,116' : '4,4,7';
      bg.addColorStop(0, 'rgba(' + tint + ',' + (bl.light ? 0.20 : 0.42) + ')');
      bg.addColorStop(1, 'rgba(' + tint + ',0)');
      ctx.fillStyle = bg;
      ctx.beginPath();
      ctx.arc(bx, by, br, 0, TAU);
      ctx.fill();
    }

    // white granite veins
    ctx.lineCap = 'round';
    ctx.lineJoin = 'round';
    for (i = 0; i < mb.veins.length; i++) {
      var vn = mb.veins[i], pts = vn.pts, k;
      ctx.beginPath();
      ctx.moveTo(pts[0].x * d.size, pts[0].y * d.size);
      for (k = 1; k < pts.length; k++) {
        var prev = pts[k - 1], cur = pts[k];
        ctx.quadraticCurveTo(
          ((prev.x + cur.x) / 2 + (cur.y - prev.y) * 0.16) * d.size,
          ((prev.y + cur.y) / 2 - (cur.x - prev.x) * 0.16) * d.size,
          cur.x * d.size, cur.y * d.size);
      }
      ctx.strokeStyle = 'rgba(246,244,250,' + (vn.a * 0.75).toFixed(3) + ')';
      ctx.lineWidth = Math.max(0.6, vn.w * d.size);
      ctx.stroke();
      ctx.strokeStyle = 'rgba(255,255,255,' + (vn.a * 0.3).toFixed(3) + ')';
      ctx.lineWidth = Math.max(0.4, vn.w * d.size * 0.4);
      ctx.stroke();
    }

    // specular sheen, soft edged so it reads as polish and not as a smudge
    if (spec > 0.01) {
      var sx = -r * 0.32, sy = -r * 0.38, sr = r * 1.15;
      var sg = ctx.createRadialGradient(sx, sy, 0, sx, sy, sr);
      sg.addColorStop(0, 'rgba(255,255,255,' + Math.min(0.34, spec).toFixed(3) + ')');
      sg.addColorStop(0.55, 'rgba(255,255,255,' + Math.min(0.12, spec * 0.35).toFixed(3) + ')');
      sg.addColorStop(1, 'rgba(255,255,255,0)');
      ctx.fillStyle = sg;
      ctx.beginPath();
      ctx.arc(sx, sy, sr, 0, TAU);
      ctx.fill();
    }

    // gold numeral, faded out as the face turns away from the camera
    var alpha = Math.min(1, Math.max(0, (facing - 0.12) / 0.35));
    if (alpha > 0.02) {
      var label = g.labels[fi];
      var text = (g.sides === 100 && label === 0) ? '00' : String(label);
      var sizeFactor = idx.length === 3 ? 1.55 : 1.3;
      var fs = r * sizeFactor * (text.length > 1 ? 0.68 : 1);
      ctx.font = '700 ' + fs.toFixed(1) + 'px Cinzel, "Trajan Pro", Georgia, serif';
      ctx.textAlign = 'center';
      ctx.textBaseline = 'middle';
      ctx.globalAlpha = alpha;
      var gy = idx.length === 3 ? r * 0.18 : 0;
      var gg = ctx.createLinearGradient(0, gy - fs * 0.55, 0, gy + fs * 0.55);
      gg.addColorStop(0, '#fff3c4');
      gg.addColorStop(0.42, '#f2c14e');
      gg.addColorStop(0.72, '#c9942a');
      gg.addColorStop(1, '#8c6414');
      ctx.lineWidth = Math.max(1, fs * 0.09);
      ctx.strokeStyle = 'rgba(24,18,4,0.72)';
      ctx.strokeText(text, 0, gy);
      ctx.fillStyle = gg;
      ctx.fillText(text, 0, gy);
      // Dice underline 6 and 9 so they cannot be read upside down.
      if (label === 6 || label === 9) {
        ctx.beginPath();
        ctx.moveTo(-fs * 0.28, gy + fs * 0.52);
        ctx.lineTo(fs * 0.28, gy + fs * 0.52);
        ctx.lineWidth = Math.max(1, fs * 0.07);
        ctx.strokeStyle = '#d9ad46';
        ctx.stroke();
      }
      ctx.globalAlpha = 1;
    }
    ctx.setTransform(base);
  };

  // ------------------------------------------------------------------ melt
  // When the dice come to rest they slump into the page: the settled frame is
  // cached once, then blitted back in narrow columns that sag and fade.
  DiceBoard.prototype.startMelt = function () {
    var minx = Infinity, miny = Infinity, maxx = -Infinity, maxy = -Infinity, self = this;
    this.dice.forEach(function (d) {
      d.g.verts.forEach(function (v) {
        var r = qRotate(d.quat, scale(v, d.size));
        var p = self.project([r[0] + d.pos[0], r[1] + d.pos[1], r[2] + d.pos[2]]);
        if (p[0] < minx) { minx = p[0]; }
        if (p[0] > maxx) { maxx = p[0]; }
        if (p[1] < miny) { miny = p[1]; }
        if (p[1] > maxy) { maxy = p[1]; }
      });
    });
    if (!isFinite(minx)) { this.melt = null; return; }
    var pad = 46;
    var x = Math.max(0, Math.floor(minx - pad)), y = Math.max(0, Math.floor(miny - pad));
    var w = Math.min(this.w, Math.ceil(maxx + pad)) - x;
    var h = Math.min(this.h, Math.ceil(maxy + pad)) - y;
    if (w <= 0 || h <= 0) { this.melt = null; return; }

    var dpr = this.dpr;
    var off = document.createElement('canvas');
    off.width = Math.ceil(w * dpr);
    off.height = Math.ceil(h * dpr);
    var octx = off.getContext('2d');
    octx.setTransform(dpr, 0, 0, dpr, -x * dpr, -y * dpr);
    this.drawDice(octx);

    // The noise has to be low frequency: neighbouring columns must sag by
    // almost the same amount or the die comes apart like a venetian blind
    // instead of slumping.
    // Columns are assembled in a work buffer at full opacity and the whole
    // buffer is faded in one go; fading each column as it is blitted would
    // double up the alpha wherever two of them overlap and stripe the die.
    var work = document.createElement('canvas');
    var wh = Math.min(4000, Math.ceil((h * 3.2 + 220)));
    work.width = Math.ceil((w + 24) * dpr);
    work.height = Math.ceil(wh * dpr);
    var wctx = work.getContext('2d');
    wctx.setTransform(dpr, 0, 0, dpr, 12 * dpr, 0);

    var col = 4, cols = Math.ceil(w / col), noise = [], i;
    for (i = 0; i < cols; i++) {
      var n = 0.5 + 0.34 * Math.sin(i * 0.055 + 1.3) + 0.16 * Math.sin(i * 0.019 + 4.1);
      noise.push(Math.max(0, Math.min(1, n)));
    }
    this.melt = { canvas: off, work: work, wctx: wctx, wh: wh,
                  x: x, y: y, w: w, h: h, col: col, cols: cols,
                  noise: noise, scale: dpr };
  };

  DiceBoard.prototype.drawMelt = function (t) {
    var ctx = this.ctx;
    var m = this.melt;
    if (!m) { return; }
    var tt = Math.max(0, Math.min(1, t));
    ctx.save();
    // The shadow is not sliced up with the die, it sinks along with it and
    // fades out early, before the two can visibly come apart.
    var sink = tt * tt * (m.h * 0.2 + 40);
    ctx.globalAlpha = Math.max(0, 1 - tt * 1.7);
    if (ctx.globalAlpha > 0.01) {
      ctx.save();
      ctx.translate(0, sink);
      this.drawShadows(ctx);
      ctx.restore();
    }
    ctx.globalAlpha = 1;
    var wctx = m.wctx;
    wctx.save();
    wctx.setTransform(1, 0, 0, 1, 0, 0);
    wctx.clearRect(0, 0, m.work.width, m.work.height);
    wctx.restore();
    for (var i = 0; i < m.cols; i++) {
      var nz = m.noise[i];
      var delay = nz * 0.12;
      var p = (tt - delay) / (1 - delay);
      if (p <= 0) { p = 0; }
      if (p > 1) { p = 1; }
      var e = p * p;
      // The top of a column sinks a little, the bottom smears a long way:
      // the die softens and runs into the paper rather than sliding off it.
      var drop = e * (m.h * 0.2 + 80 * nz);
      var stretch = 1 + e * (1.7 + nz * 1.0);
      var wob = Math.sin(i * 0.09 + p * 3.2) * 3 * p;
      wctx.drawImage(m.canvas,
        i * m.col * m.scale, 0, m.col * m.scale, m.canvas.height,
        i * m.col + wob, drop, m.col + 1.4, m.h * stretch);
    }
    ctx.globalAlpha = Math.max(0, 1 - Math.pow(tt, 1.5));
    ctx.drawImage(m.work, 0, 0, m.work.width, m.work.height,
      m.x - 12, m.y, m.w + 24, m.wh);
    ctx.restore();
  };

  // --------------------------------------------------------- landing zones
  // "Throw the dice at the emptiest bit of sheet, away from where I clicked."
  // The sheet is dense, so rather than probing hit testing point by point -
  // which is far too slow to do hundreds of times per roll - every element
  // that actually carries ink is measured once and painted into a coarse
  // occupancy grid. Cells nothing covers are the gaps between the boxes.
  var INK = '.page p, .page td, .page th, .page h1, .page h2, .page h3,' +
            ' .page summary, .page .row, .page .fact, .page .ability, .page .tile,' +
            ' .page .tagline, .page .src, .page .slot-row, .page .char-sub, .page .quote';
  var GRID_COLS = 30, GRID_ROWS = 20;

  function occupancy(bounds) {
    var cw = (bounds.r - bounds.l) / GRID_COLS, ch = (bounds.b - bounds.t) / GRID_ROWS;
    var cells = new Float32Array(GRID_COLS * GRID_ROWS);
    if (cw <= 0 || ch <= 0) { return cells; }
    var mark = function (r, weight) {
      if (!r.width || !r.height) { return; }
      if (r.right < bounds.l || r.left > bounds.r) { return; }
      if (r.bottom < bounds.t || r.top > bounds.b) { return; }
      var i0 = Math.max(0, Math.floor((r.left - bounds.l) / cw));
      var i1 = Math.min(GRID_COLS - 1, Math.floor((r.right - bounds.l) / cw));
      var j0 = Math.max(0, Math.floor((r.top - bounds.t) / ch));
      var j1 = Math.min(GRID_ROWS - 1, Math.floor((r.bottom - bounds.t) / ch));
      for (var i = i0; i <= i1; i++) {
        for (var j = j0; j <= j1; j++) { cells[j * GRID_COLS + i] += weight; }
      }
    };
    var els = document.querySelectorAll(INK);
    for (var e = 0; e < els.length; e++) { mark(els[e].getBoundingClientRect(), 1); }
    // The tray, the dice button and the custom roll panel float over the
    // sheet, so they are not landing ground however empty the page is there.
    var over = document.querySelectorAll('#dice-tray.open, #dice-custom.open, #dice-fab');
    for (var o = 0; o < over.length; o++) { mark(over[o].getBoundingClientRect(), 40); }
    return cells;
  }

  function pickLanding(click, radius) {
    var W = window.innerWidth, H = window.innerHeight;
    var margin = Math.max(80, radius * 1.5);
    var diag = Math.sqrt(W * W + H * H);
    var b = { l: margin, t: margin, r: W - margin, b: H - margin };
    var page = document.querySelector('.page');
    if (page) {
      var pr = page.getBoundingClientRect();
      b.l = Math.max(b.l, pr.left + margin * 0.4);
      b.r = Math.min(b.r, pr.right - margin * 0.4);
      b.t = Math.max(b.t, pr.top + margin * 0.4);
      b.b = Math.min(b.b, pr.bottom - margin * 0.4);
    }
    // Never land under the roll tray.
    var panel = document.getElementById('dice-tray');
    if (panel && panel.classList.contains('open')) {
      b.r = Math.min(b.r, W - panel.offsetWidth - margin * 0.4);
    }
    if (b.r - b.l < 120) { b.l = margin; b.r = W - margin; }
    if (b.b - b.t < 120) { b.t = margin; b.b = H - margin; }

    var cells = occupancy(b);
    var cw = (b.r - b.l) / GRID_COLS, ch = (b.b - b.t) / GRID_ROWS;
    var best = null, i, j;
    for (i = 0; i < GRID_COLS; i++) {
      for (j = 0; j < GRID_ROWS; j++) {
        var x = b.l + (i + 0.5) * cw, y = b.t + (j + 0.5) * ch;
        // Sample the immediate neighbourhood too, so the winner is a clear
        // patch rather than a lucky single cell in a wall of text.
        var ink = 0, n = 0, di, dj;
        for (di = -1; di <= 1; di++) {
          for (dj = -1; dj <= 1; dj++) {
            var ii = i + di, jj = j + dj;
            if (ii < 0 || ii >= GRID_COLS || jj < 0 || jj >= GRID_ROWS) { continue; }
            ink += cells[jj * GRID_COLS + ii];
            n++;
          }
        }
        var open = 1 / (1 + (ink / (n || 1)) * 0.8);
        var dx = x - click.x, dy = y - click.y;
        var far = Math.min(1, Math.sqrt(dx * dx + dy * dy) / diag * 2.1);
        var score = open * (0.2 + 1.8 * far);
        if (!best || score > best.score) { best = { x: x, y: y, score: score }; }
      }
    }
    return best || { x: (b.l + b.r) / 2, y: (b.t + b.b) / 2 };
  }

  // ------------------------------------------------------------ roll maths
  // Which dice can be thrown in 3d. A d100 is deliberately absent: a
  // percentile die's faces read 00 to 90, so a roll of 57 has no face to land
  // on. It is rolled and reported like any other die, just not animated.
  var SOLID_SIDES = { 4: 1, 6: 1, 8: 1, 10: 1, 12: 1, 20: 1 };

  function parseDice(expr) {
    var s = String(expr || '').toLowerCase().replace(/\s+/g, '').replace(/[–—−]/g, '-');
    var re = /([+-]?)(?:(\d*)d(\d+)|(\d+))/g;
    var m, terms = [], flat = 0, found = false;
    while ((m = re.exec(s)) !== null) {
      var sign = m[1] === '-' ? -1 : 1;
      if (m[3] !== undefined) {
        var count = m[2] === '' ? 1 : parseInt(m[2], 10);
        var sides = parseInt(m[3], 10);
        if (!count || !sides) { continue; }
        terms.push({ count: Math.min(24, count), sides: sides, sign: sign });
        found = true;
      } else if (m[4] !== undefined) {
        flat += sign * parseInt(m[4], 10);
        found = true;
      }
    }
    return found ? { terms: terms, flat: flat } : null;
  }

  function rollDie(sides) { return 1 + Math.floor(Math.random() * sides); }

  function rollTerms(terms) {
    var out = [];
    terms.forEach(function (t) {
      for (var i = 0; i < t.count; i++) {
        out.push({ sides: t.sides, value: rollDie(t.sides), sign: t.sign });
      }
    });
    return out;
  }

  function sumRolls(rolls) {
    return rolls.reduce(function (a, r) { return a + r.sign * r.value; }, 0);
  }

  function signed(n) { return (n < 0 ? '' : '+') + n; }

  function modOf(el) {
    var raw = el.getAttribute('data-mod');
    if (raw === null || raw === '') { return 0; }
    var n = parseInt(String(raw).replace(/[–—−]/g, '-').replace(/[^0-9+-]/g, ''), 10);
    return isNaN(n) ? 0 : n;
  }

  // ------------------------------------------------------------------- tray
  var tray, tab, latestEl, historyEl, autoBox, board, history = [], seq = 0;

  var D20_ICON = '<svg viewBox="0 0 24 24" aria-hidden="true" focusable="false">' +
    '<path d="M12 1.6 22 7.4v9.2L12 22.4 2 16.6V7.4z"/>' +
    '<path d="M12 1.6 12 22.4M2 7.4l10 3.2 10-3.2M2 16.6l10-6 10 6"/></svg>';

  function buildTray() {
    tab = document.createElement('button');
    tab.id = 'dice-tab';
    tab.type = 'button';
    tab.setAttribute('aria-expanded', 'false');
    tab.innerHTML = D20_ICON + '<span>Rolls</span>';
    document.body.appendChild(tab);

    tray = document.createElement('aside');
    tray.id = 'dice-tray';
    tray.setAttribute('aria-label', 'Dice rolls');
    tray.innerHTML =
      '<div class="dt-head">' +
        '<h2>' + D20_ICON + 'Dice</h2>' +
        '<label class="dt-auto" title="Pop the tray open on every roll">' +
          '<input type="checkbox" id="dice-auto" checked> auto' +
        '</label>' +
        '<button type="button" class="dt-btn" id="dice-clear">Clear</button>' +
        '<button type="button" class="dt-btn dt-x" id="dice-close" aria-label="Hide roll tray">&times;</button>' +
      '</div>' +
      '<div id="dice-latest" aria-live="polite"></div>' +
      '<div class="dt-hist-head">History</div>' +
      '<div id="dice-history"></div>' +
      '<div class="dt-tip">Click anything underlined on the sheet to roll it. ' +
      'Click a history line to roll it again.</div>';
    document.body.appendChild(tray);

    latestEl = tray.querySelector('#dice-latest');
    historyEl = tray.querySelector('#dice-history');
    autoBox = tray.querySelector('#dice-auto');

    tab.addEventListener('click', function () { setTray(!tray.classList.contains('open')); });
    tray.querySelector('#dice-close').addEventListener('click', function () { setTray(false); });
    tray.querySelector('#dice-clear').addEventListener('click', function () {
      history = [];
      latestEl.innerHTML = '';
      renderHistory();
    });
    document.addEventListener('keydown', function (e) {
      if (e.key !== 'Escape') { return; }
      // Whatever is on top closes first.
      if (sessPanel && sessPanel.classList.contains('open')) { setSessPanel(false); return; }
      if (panel && panel.classList.contains('open')) { setPanel(false); return; }
      if (drawer && drawer.classList.contains('open') &&
          !drawer.contains(document.activeElement)) { setDrawer(false); return; }
      if (tray.classList.contains('open')) { setTray(false); }
    });
    renderHistory();
  }

  function setTray(open) {
    tray.classList.toggle('open', open);
    tab.classList.toggle('shifted', open);
    tab.setAttribute('aria-expanded', open ? 'true' : 'false');
    if (dock) { dock.classList.toggle('shifted', open); }
    if (drawer) { drawer.classList.toggle('tray-open', open); }
  }

  function esc(s) {
    return String(s).replace(/[&<>"]/g, function (c) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c];
    });
  }

  function natClass(n) {
    if (n === 20) { return ' crit'; }
    if (n === 1) { return ' fumble'; }
    return '';
  }

  function cell(name, value, cls, nat) {
    return '<div class="rc-cell' + (cls || '') + natClass(nat) + '">' +
      '<span>' + name + '</span><b>' + value + '</b></div>';
  }

  // castCard lays out one cast: the attack it rolled, the damage or healing
  // that followed, and the saving throw the target still owes.
  function castCard(r) {
    var out = '<div class="rc"><div class="rc-label">' + SPELL_MARK + esc(r.label) + '</div>' +
      '<div class="rc-formula">' + esc(r.detail) + '</div>';
    if (r.attack) {
      out += '<div class="rc-sub">To hit ' + esc(r.attack.formula) + '</div>' +
        '<div class="rc-grid three">' +
          cell('Disadv', r.attack.dis, '', r.attack.disNat) +
          cell('Normal', r.attack.normal, ' main', r.attack.normalNat) +
          cell('Advant', r.attack.adv, '', r.attack.advNat) +
        '</div>';
    }
    if (r.damage) {
      var crit = r.damage.crit !== null && r.damage.crit !== undefined;
      out += '<div class="rc-sub">' + esc(r.damage.name) + ' ' + esc(r.damage.formula) + '</div>' +
        '<div class="rc-grid ' + (crit ? 'two' : 'one') + '">' +
          cell('Total', r.damage.total, ' main') +
          (crit ? cell('If critical', r.damage.crit, '') : '') +
        '</div>';
    }
    if (r.line) { out += '<div class="rc-line">' + esc(r.line) + '</div>'; }
    if (r.dice) { out += '<div class="rc-dice">' + esc(r.dice) + '</div>'; }
    // The slot this cost is struck off the character file while the dice are
    // in the air, so the answer arrives after the card is first drawn.
    if (r.slotNote) {
      out += '<div class="rc-slot' + (r.slotBad ? ' bad' : '') + '">' +
        esc(r.slotNote) + '</div>';
    }
    return out + '</div>';
  }

  function latestCard(r) {
    if (r.kind === 'cast') { return castCard(r); }
    var head = '<div class="rc-label">' + esc(r.label) + '</div>' +
               '<div class="rc-formula">' + esc(r.formula) + '</div>';
    if (r.kind === 'check') {
      return '<div class="rc">' + head +
        '<div class="rc-grid three">' +
          cell('Disadv', r.dis, '', r.disNat) +
          cell('Normal', r.normal, ' main', r.normalNat) +
          cell('Advant', r.adv, '', r.advNat) +
        '</div>' +
        '<div class="rc-dice">two d20: <b>' + r.pair[0] + '</b>, <b>' + r.pair[1] + '</b>' +
        (r.mod ? ' &middot; modifier ' + signed(r.mod) : '') + '</div>' +
        '</div>';
    }
    var cells = cell('Total', r.total, ' main');
    if (r.crit !== null && r.crit !== undefined) { cells += cell('If critical', r.crit, ''); }
    return '<div class="rc">' + head +
      '<div class="rc-grid ' + (r.crit === null || r.crit === undefined ? 'one' : 'two') + '">' +
      cells + '</div>' +
      '<div class="rc-dice">' + esc(r.detail) + '</div></div>';
  }

  function renderHistory() {
    if (!history.length) {
      historyEl.innerHTML = '<div class="dt-empty">No rolls yet.</div>';
      return;
    }
    historyEl.innerHTML = history.map(function (r) {
      if (r.kind === 'cast') { return castRow(r); }
      var main = r.kind === 'check' ? r.normal : r.total;
      var extra = r.kind === 'check'
        ? '<span class="hx">' + r.dis + ' / ' + r.adv + '</span>'
        : '<span class="hx">' + esc(r.formula) + '</span>';
      return '<button type="button" class="hr" data-roll-id="' + r.id + '" ' +
        'title="Roll ' + esc(r.label) + ' again">' +
        '<span class="ht">' + r.time + '</span>' +
        '<span class="hl">' + esc(r.label) + '</span>' + extra +
        '<span class="hv' + natClass(r.kind === 'check' ? r.normalNat : 0) + '">' + main + '</span>' +
        '</button>';
    }).join('');
  }

  // One line of history for a cast: the mark, the spell, and the number that
  // matters most - the damage it did, or what it hit on.
  function castRow(r) {
    var main = '&middot;', natural = 0;
    if (r.damage) {
      main = r.damage.total;
    } else if (r.attack) {
      main = r.attack.normal;
      natural = r.attack.normalNat;
    }
    var extra = r.attack && r.damage
      ? '<span class="hx">hit ' + r.attack.normal + '</span>'
      : '<span class="hx">' + esc(r.short) + '</span>';
    return '<button type="button" class="hr" data-roll-id="' + r.id + '" ' +
      'title="Cast ' + esc(r.label) + ' again">' +
      '<span class="ht">' + r.time + '</span>' +
      '<span class="hl">' + SPELL_MARK + esc(r.label) + '</span>' + extra +
      '<span class="hv' + natClass(natural) + '">' + main + '</span>' +
      '</button>';
  }

  function historyClick(e) {
    var btn = e.target.closest('.hr');
    if (!btn) { return; }
    var id = parseInt(btn.getAttribute('data-roll-id'), 10);
    var found = null;
    history.forEach(function (r) { if (r.id === id) { found = r; } });
    if (!found) { return; }
    var box = btn.getBoundingClientRect();
    roll(found.spec, { x: box.left + box.width / 2, y: box.top + box.height / 2 });
  }

  // ------------------------------------------------------- custom roll dock
  // A dice button in the corner of the page, and the one off roll panel it
  // opens. Anything rolled here goes through roll() exactly like a roll made
  // from the sheet, so it animates and lands in the history the same way.
  var dock, fab, panel, exprInput, modLabel;

  // The fab opens dice, session and history, so it wears a plain menu stack
  // rather than a die that would promise only rolls.
  var MENU_ICON = '<svg viewBox="0 0 24 24" aria-hidden="true" focusable="false">' +
    '<path d="M4 7h16M4 12h16M4 17h16"/></svg>';

  // A cast is marked in the tray so a spell is told apart from a die roll at
  // a glance. It is drawn into the panel only - the session log is text, and
  // the mark has no business in an org file.
  var SPELL_MARK = '<span class="spell-mark" aria-hidden="true">' +
    '<svg viewBox="0 0 24 24" focusable="false">' +
    '<path d="M12 2.6 14 9l6.4 2-6.4 2-2 6.4-2-6.4L3.6 11 10 9z"/>' +
    '<path d="M18.4 3.2 19 5l1.8.6-1.8.6-.6 1.8-.6-1.8L16 5.6 17.8 5z"/></svg></span>';

  var QUICK_DICE = [4, 6, 8, 10, 12, 20];

  // formatDice renders a parsed pool back to text, merging repeats and
  // putting the biggest die first the way a dice tray is read.
  function formatDice(parsed) {
    var groups = {}, order = [];
    parsed.terms.forEach(function (t) {
      var key = (t.sign < 0 ? '-' : '+') + t.sides;
      if (!groups[key]) {
        groups[key] = { sides: t.sides, sign: t.sign, count: 0 };
        order.push(key);
      }
      groups[key].count += t.count;
    });
    order.sort(function (a, b) { return groups[b].sides - groups[a].sides; });
    var out = '';
    order.forEach(function (k) {
      var g = groups[k], term = g.count + 'd' + g.sides;
      if (!out) { out = (g.sign < 0 ? '-' : '') + term; }
      else { out += (g.sign < 0 ? ' - ' : ' + ') + term; }
    });
    if (parsed.flat) { out += (out ? ' ' : '') + signed(parsed.flat); }
    return out;
  }

  function currentPool() {
    return parseDice(exprInput.value) || { terms: [], flat: 0 };
  }

  function setPool(parsed) {
    exprInput.value = formatDice(parsed);
    syncPool();
  }

  function syncPool() {
    var parsed = currentPool();
    modLabel.textContent = signed(parsed.flat);
    var ready = parsed.terms.length > 0;
    panel.querySelector('#dice-custom-roll').disabled = !ready;
    // Say what the roll is going to do before it is made.
    var single = ready && parsed.terms.length === 1 &&
      parsed.terms[0].sides === 20 && parsed.terms[0].count === 1 &&
      parsed.terms[0].sign > 0;
    panel.querySelector('.dc-hint').textContent = single
      ? 'A single d20 reports normal, advantage and disadvantage.'
      : 'Pick dice, or type an expression such as 2d6 + 3.';
  }

  function addDie(sides) {
    var parsed = currentPool(), total = 0;
    parsed.terms.forEach(function (t) { total += t.count; });
    if (total >= 20) { return; }
    parsed.terms.push({ count: 1, sides: sides, sign: 1 });
    setPool(parsed);
  }

  function bumpMod(delta) {
    var parsed = currentPool();
    parsed.flat = Math.max(-99, Math.min(99, parsed.flat + delta));
    setPool(parsed);
  }

  function customRoll() {
    var parsed = parseDice(exprInput.value);
    if (!parsed || !parsed.terms.length) {
      panel.classList.remove('nope');
      void panel.offsetWidth;
      panel.classList.add('nope');
      return;
    }
    var text = formatDice(parsed);
    exprInput.value = text;
    syncPool();
    // One plain d20 is a check, and wants advantage and disadvantage with it.
    // Anything else is just a handful of dice, so no critical line either.
    var single = parsed.terms.length === 1 && parsed.terms[0].sides === 20 &&
      parsed.terms[0].count === 1 && parsed.terms[0].sign > 0;
    var spec = single
      ? { kind: 'check', label: 'Custom Check', terms: [], flat: parsed.flat,
          formula: 'd20' + (parsed.flat ? ' ' + signed(parsed.flat) : '') }
      : { kind: 'damage', label: 'Custom Roll', noCrit: true,
          terms: parsed.terms, flat: parsed.flat, formula: text };
    var box = fab.getBoundingClientRect();
    roll(spec, { x: box.left + box.width / 2, y: box.top + box.height / 2 });
  }

  function setPanel(open) {
    panel.classList.toggle('open', open);
    fab.setAttribute('aria-expanded', open ? 'true' : 'false');
    if (open) { exprInput.focus(); exprInput.select(); }
  }

  function buildCustomDock() {
    dock = document.createElement('div');
    dock.id = 'dice-dock';

    panel = document.createElement('aside');
    panel.id = 'dice-custom';
    panel.setAttribute('aria-label', 'Custom roll');
    panel.innerHTML =
      '<div class="dc-head"><h3>Custom Roll</h3>' +
        '<button type="button" class="dt-btn dt-x" id="dice-custom-close" ' +
        'aria-label="Close custom roll">&times;</button></div>' +
      '<input id="dice-expr" type="text" spellcheck="false" autocomplete="off" ' +
        'placeholder="2d6 + 3" aria-label="Dice expression">' +
      '<div class="dc-dice">' +
        QUICK_DICE.map(function (n) {
          return '<button type="button" data-die="' + n + '">d' + n + '</button>';
        }).join('') +
      '</div>' +
      '<div class="dc-mod"><span>Modifier</span>' +
        '<button type="button" data-mod="-1" aria-label="Lower modifier">&minus;</button>' +
        '<b id="dice-mod-val">+0</b>' +
        '<button type="button" data-mod="1" aria-label="Raise modifier">+</button>' +
      '</div>' +
      '<div class="dc-hint"></div>' +
      '<div class="dc-actions dc-rest">' +
        '<button type="button" class="dt-btn" id="rest-short" ' +
          'title="Take a short rest: spend hit dice and recover what an ' +
          'hour gives back">Short rest</button>' +
        '<button type="button" class="dt-btn" id="rest-long" ' +
          'title="Take a long rest: hit points, hit dice, spell slots and ' +
          'every feature">Long rest</button>' +
      '</div>' +
      '<div class="dc-actions dc-print">' +
        '<button type="button" class="dt-btn" id="sheet-print" ' +
          'title="Print this sheet: everything folded away is opened, the ' +
          'screen furniture goes, and the paper keeps its ink">Print sheet</button>' +
      '</div>' +
      '<div class="dc-actions">' +
        '<button type="button" class="dt-btn" id="dice-custom-clear">Clear</button>' +
        '<button type="button" class="dt-btn" id="sess-open" ' +
          'title="Start or manage the recorded play session">Session</button>' +
        '<button type="button" class="dt-btn" id="sess-list" ' +
          'title="Every session you have played">Sessions</button>' +
        '<button type="button" class="dc-roll" id="dice-custom-roll">Roll</button>' +
      '</div>';
    dock.appendChild(panel);

    fab = document.createElement('button');
    fab.id = 'dice-fab';
    fab.type = 'button';
    fab.title = 'Dice and session menu';
    fab.setAttribute('aria-label', 'Dice and session menu');
    fab.setAttribute('aria-expanded', 'false');
    fab.innerHTML = MENU_ICON;
    dock.appendChild(fab);
    document.body.appendChild(dock);

    exprInput = panel.querySelector('#dice-expr');
    modLabel = panel.querySelector('#dice-mod-val');

    fab.addEventListener('click', function () {
      setPanel(!panel.classList.contains('open'));
    });
    panel.querySelector('#dice-custom-close').addEventListener('click', function () {
      setPanel(false);
    });
    panel.querySelector('#dice-custom-clear').addEventListener('click', function () {
      exprInput.value = '';
      syncPool();
      exprInput.focus();
    });
    panel.querySelector('#dice-custom-roll').addEventListener('click', customRoll);
    panel.querySelector('#sess-open').addEventListener('click', function () {
      setPanel(false);
      setSessPanel(true);
    });
    panel.querySelector('#sess-list').addEventListener('click', function () {
      setPanel(false);
      setDrawer(true);
      setView('sessions');
      loadSessions();
    });
    panel.querySelector('#sheet-print').addEventListener('click', function () {
      setPanel(false);
      window.print();
    });
    panel.querySelector('#rest-short').addEventListener('click', function () {
      setPanel(false);
      openRest('short');
    });
    panel.querySelector('#rest-long').addEventListener('click', function () {
      setPanel(false);
      openRest('long');
    });
    panel.addEventListener('click', function (e) {
      var b = e.target.closest('[data-die]');
      if (b) { addDie(parseInt(b.getAttribute('data-die'), 10)); return; }
      var m = e.target.closest('[data-mod]');
      if (m) { bumpMod(parseInt(m.getAttribute('data-mod'), 10)); }
    });
    exprInput.addEventListener('input', syncPool);
    exprInput.addEventListener('keydown', function (e) {
      if (e.key === 'Enter') { e.preventDefault(); customRoll(); }
    });
    syncPool();
  }

  // ------------------------------------------------------------- printing
  // On screen a spell description is a fold you click open; paper has nothing
  // to click, so every fold is opened for the length of the print and shut
  // again afterwards. The layout itself is the sheet's own @media print rules
  // - this only opens what they cannot reach, because a closed <details> has
  // no printable content at all. It hangs off the print events rather than
  // the button so that the browser's own print command gets it too, and it is
  // guarded because a browser may give us both the event and the media query.
  var printFolds = [];
  var printing = false;

  function openForPrint() {
    if (printing) { return; }
    printing = true;
    printFolds = [];
    Array.prototype.forEach.call(
      document.querySelectorAll('.page details'), function (d) {
        if (!d.open) { printFolds.push(d); d.open = true; }
      });
  }

  function closeAfterPrint() {
    if (!printing) { return; }
    printing = false;
    printFolds.forEach(function (d) { d.open = false; });
    printFolds = [];
  }

  function watchPrinting() {
    window.addEventListener('beforeprint', openForPrint);
    window.addEventListener('afterprint', closeAfterPrint);
    if (!window.matchMedia) { return; }
    var mq = window.matchMedia('print');
    var onChange = function (e) {
      if (e.matches) { openForPrint(); } else { closeAfterPrint(); }
    };
    if (mq.addEventListener) { mq.addEventListener('change', onChange); }
    else if (mq.addListener) { mq.addListener(onChange); }
  }

  // ------------------------------------------------------------ rolling it
  function specFor(el) {
    var kind = el.getAttribute('data-kind') || 'check';
    var label = el.getAttribute('data-label') || 'Roll';
    var mod = modOf(el);

    if (kind === 'hitdie') {
      var pool = el.getAttribute('data-pool') || '1d8';
      var m = /d(\d+)/i.exec(pool);
      var sides = m ? parseInt(m[1], 10) : 8;
      return { kind: 'damage', label: label, noCrit: true,
               terms: [{ count: 1, sides: sides, sign: 1 }], flat: mod,
               formula: '1d' + sides + (mod ? ' ' + signed(mod) : '') };
    }
    if (kind === 'check') {
      return { kind: 'check', label: label, terms: [], flat: mod,
               formula: 'd20' + (mod ? ' ' + signed(mod) : '') };
    }
    var parsed = parseDice(el.getAttribute('data-roll') || '');
    if (!parsed) { return null; }
    parsed.flat += mod;
    var text = parsed.terms.map(function (t) {
      return (t.sign < 0 ? '-' : '') + t.count + 'd' + t.sides;
    }).join(' + ').replace(/\+ -/g, '- ');
    if (parsed.flat) { text += ' ' + signed(parsed.flat); }
    return { kind: 'damage', label: label, terms: parsed.terms, flat: parsed.flat,
             formula: text, noCrit: el.getAttribute('data-nocrit') === '1' };
  }

  // ------------------------------------------------------------- casting
  //
  // A cast is one click that does everything the spell asks for: the attack
  // roll if it needs one, the damage or healing that follows, and - for a
  // spell that rolls nothing itself - the saving throw the target owes,
  // written out ready to read across the table.
  function castSpecFor(el) {
    var spec = {
      kind: 'cast',
      label: el.getAttribute('data-spell') || 'Spell',
      detail: el.getAttribute('data-detail') || '',
      line: el.getAttribute('data-line') || '',
      save: el.getAttribute('data-save') || '',
      dc: el.getAttribute('data-dc') || ''
    };
    var atk = el.getAttribute('data-attack');
    if (atk !== null && atk !== '') {
      var mod = parseInt(atk, 10) || 0;
      spec.attack = { flat: mod, formula: 'd20 ' + signed(mod) };
    }
    var dmg = el.getAttribute('data-damage');
    if (dmg) {
      var parsed = parseDice(dmg);
      if (parsed && parsed.terms.length) {
        spec.damage = { name: 'Damage', terms: parsed.terms, flat: parsed.flat,
                        formula: dmg + ' ' + (el.getAttribute('data-damage-type') || '') };
      }
    }
    var heal = el.getAttribute('data-heal');
    if (heal && !spec.damage) {
      var h = parseDice(heal);
      if (h && h.terms.length) {
        spec.damage = { name: 'Healing', terms: h.terms, flat: h.flat,
                        formula: heal, noCrit: true };
      }
    }
    // The few words a history row shows when there is no second roll to put
    // there. The full line only fits on the card.
    spec.short = el.getAttribute('data-short') || '';
    return spec;
  }

  function castRoll(spec, origin) {
    var result = { kind: 'cast', label: spec.label, detail: spec.detail,
                   line: spec.line, short: spec.short, spec: spec,
                   formula: spec.detail };
    var shown = [], bits = [];

    if (spec.attack) {
      var a = rollDie(20), b = rollDie(20);
      var hi = Math.max(a, b), lo = Math.min(a, b);
      result.attack = {
        formula: spec.attack.formula, mod: spec.attack.flat,
        pair: [a, b],
        normal: a + spec.attack.flat, adv: hi + spec.attack.flat, dis: lo + spec.attack.flat,
        normalNat: a, advNat: hi, disNat: lo
      };
      shown.push({ sides: 20, value: a }, { sides: 20, value: b });
      bits.push('to hit d20 ' + a + ', d20 ' + b +
        (spec.attack.flat ? '  \u00b7  modifier ' + signed(spec.attack.flat) : ''));
    }
    if (spec.damage) {
      var rolls = rollTerms(spec.damage.terms);
      var total = sumRolls(rolls) + spec.damage.flat;
      var crit = null;
      // A spell attack crits like any other; a saving throw does not, and
      // neither does healing.
      if (spec.attack && !spec.damage.noCrit) {
        crit = total + sumRolls(rollTerms(spec.damage.terms));
      }
      result.damage = { name: spec.damage.name, formula: spec.damage.formula.trim(),
                        total: total, crit: crit };
      rolls.forEach(function (r) { shown.push({ sides: r.sides, value: r.value }); });
      bits.push(spec.damage.name.toLowerCase() + ' ' + rolls.map(function (r) {
        return (r.sign < 0 ? '-' : '') + 'd' + r.sides + ' ' + r.value;
      }).join(', ') + (spec.damage.flat ? '  \u00b7  modifier ' + signed(spec.damage.flat) : ''));
    }
    result.dice = bits.join('  \u00b7  ');

    result.id = ++seq;
    result.time = clockNow();
    history.unshift(result);
    if (history.length > 60) { history.length = 60; }
    latestEl.innerHTML = latestCard(result);
    renderHistory();
    logRoll(result);
    if (autoBox.checked) { setTray(true); }
    tab.classList.remove('pulse');
    void tab.offsetWidth;
    tab.classList.add('pulse');

    var throwable = shown.filter(function (s) { return SOLID_SIDES[s.sides]; });
    if (throwable.length && board) {
      var radius = 52 + throwable.length * 6;
      board.throwDice(throwable.slice(0, 12), origin, pickLanding(origin, radius));
    } else if (board) {
      // A spell that rolls nothing still clears the table.
      board.stop();
    }
    return result;
  }

  // The slot answer comes back after the dice have landed. While the cast is
  // still the roll on the card, the card is drawn again to say what it cost.
  function noteCastSlot(result, msg, bad) {
    if (!result || !msg) { return; }
    result.slotNote = msg;
    result.slotBad = !!bad;
    if (history[0] === result && latestEl) {
      latestEl.innerHTML = latestCard(result);
    }
  }

  // done, when given, is handed the result once the roll is in the history.
  // It is how the rest walkthrough spends a hit die: the die is thrown on the
  // table like any other, and the walkthrough reads what it landed on.
  function roll(spec, origin, done) {
    if (spec.kind === 'cast') { castRoll(spec, origin); return; }
    var now = new Date();
    var time = ('0' + now.getHours()).slice(-2) + ':' + ('0' + now.getMinutes()).slice(-2);
    var result, shown;

    if (spec.kind === 'check') {
      var a = rollDie(20), b = rollDie(20);
      var hi = Math.max(a, b), lo = Math.min(a, b);
      result = { kind: 'check', label: spec.label, formula: spec.formula, spec: spec,
                 pair: [a, b], mod: spec.flat,
                 normal: a + spec.flat, adv: hi + spec.flat, dis: lo + spec.flat,
                 normalNat: a, advNat: hi, disNat: lo };
      shown = [{ sides: 20, value: a }, { sides: 20, value: b }];
    } else {
      var rolls = rollTerms(spec.terms);
      var total = sumRolls(rolls) + spec.flat;
      var crit = null, extra = null;
      if (!spec.noCrit) {
        extra = rollTerms(spec.terms);
        crit = total + sumRolls(extra);
      }
      var detail = rolls.map(function (r) {
        return (r.sign < 0 ? '-' : '') + 'd' + r.sides + ' ' + r.value;
      }).join(', ');
      if (spec.flat) { detail += '  \u00b7  modifier ' + signed(spec.flat); }
      if (extra && extra.length) {
        detail += '  \u00b7  crit dice ' + extra.map(function (r) { return r.value; }).join(', ');
      }
      result = { kind: 'damage', label: spec.label, formula: spec.formula, spec: spec,
                 total: total, crit: crit, detail: detail };
      shown = rolls.map(function (r) { return { sides: r.sides, value: r.value }; });
    }

    result.id = ++seq;
    result.time = time;
    history.unshift(result);
    if (history.length > 60) { history.length = 60; }
    latestEl.innerHTML = latestCard(result);
    renderHistory();
    logRoll(result);
    if (autoBox.checked) { setTray(true); }
    tab.classList.remove('pulse');
    void tab.offsetWidth;
    tab.classList.add('pulse');

    var throwable = shown.filter(function (s) { return SOLID_SIDES[s.sides]; });
    if (throwable.length && board) {
      var radius = 52 + throwable.length * 6;
      board.throwDice(throwable.slice(0, 12), origin, pickLanding(origin, radius));
    } else if (board) {
      // Nothing to throw - a d100, say. Clear the board so the previous
      // roll's dice are not left sitting there looking like this result.
      board.stop();
    }
    if (done) { done(result); }
  }

  function originOf(el, ev) {
    if (ev && typeof ev.clientX === 'number' && (ev.clientX || ev.clientY)) {
      return { x: ev.clientX, y: ev.clientY };
    }
    var box = el.getBoundingClientRect();
    return { x: box.left + box.width / 2, y: box.top + box.height / 2 };
  }

  function activate(el, ev) {
    var spec = specFor(el);
    if (!spec) { return; }
    roll(spec, originOf(el, ev));
  }

  // --------------------------------------------------- wiring up the sheet
  function prepare(el) {
    if (el.dataset.rollReady) { return; }
    var spec = specFor(el);
    // A flat damage line such as an unarmed strike's "2" has nothing to roll.
    if (!spec || (spec.kind !== 'check' && !spec.terms.length)) {
      el.classList.remove('rollable');
      return;
    }
    el.dataset.rollReady = '1';
    el.classList.add('rollable');
    if (!el.hasAttribute('tabindex')) { el.setAttribute('tabindex', '0'); }
    el.setAttribute('role', 'button');
    if (!el.hasAttribute('title')) {
      el.setAttribute('title', spec.label + ': roll ' + spec.formula);
    }
  }

  // One whole dice expression: any number of dice terms joined with + or -,
  // then an optional flat modifier. It has to swallow the terms greedily or a
  // multiclass hit dice pool like "6d10 + 9d8" comes apart into "6d10 + 9"
  // and a stray "d8". Die sizes are listed longest first so that d100 is not
  // read as a d10 with a trailing zero.
  var DIE = '\\d{0,2}d(?:100|20|12|10|8|6|4)(?!\\d)';
  var DICE_TEXT = new RegExp(
    DIE + '(?:\\s*[+-]\\s*' + DIE + ')*(?:\\s*[+-]\\s*\\d{1,3}(?!\\s*d\\s*\\d))?', 'gi');

  // Spell and feature prose is full of live dice expressions; make each one
  // clickable in place rather than asking the player to hunt for the row.
  function linkifyProse(root) {
    var walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT, null);
    var nodes = [], node;
    while ((node = walker.nextNode())) { nodes.push(node); }
    nodes.forEach(function (n) {
      var text = n.nodeValue;
      if (!text || text.length < 2 || !/d\d/i.test(text)) { return; }
      var parent = n.parentElement;
      if (!parent || parent.closest('.rollable')) { return; }
      // Name the roll after whatever the prose belongs to - the spell, the
      // feature, or failing those the box it sits in.
      var host = parent.closest('details.spell, .feature, .box');
      var label = 'Dice';
      if (host) {
        var t = host.querySelector('summary, h3, h2');
        if (t) { label = t.textContent.replace(/\s+/g, ' ').split('—')[0].trim() || label; }
      }
      DICE_TEXT.lastIndex = 0;
      var frag = null, last = 0, m;
      while ((m = DICE_TEXT.exec(text)) !== null) {
        var before = m.index === 0 ? '' : text.charAt(m.index - 1);
        if (/[A-Za-z0-9]/.test(before)) { continue; }
        if (!frag) { frag = document.createDocumentFragment(); }
        if (m.index > last) {
          frag.appendChild(document.createTextNode(text.slice(last, m.index)));
        }
        var span = document.createElement('span');
        span.className = 'rollable inline-roll';
        span.setAttribute('data-kind', 'damage');
        span.setAttribute('data-roll', m[0]);
        span.setAttribute('data-label', label);
        span.textContent = m[0];
        prepare(span);
        frag.appendChild(span);
        last = m.index + m[0].length;
      }
      if (!frag) { return; }
      if (last < text.length) { frag.appendChild(document.createTextNode(text.slice(last))); }
      parent.replaceChild(frag, n);
    });
  }

  // ------------------------------------------------- session log and notes
  // The sheet can record a night's play into an org file on the orgs server:
  // every roll into a table, every note into the notes section. All of it is
  // optional - with no server reachable, or no session started, the sheet
  // behaves exactly as it always did and nothing here gets in the way.

  var LOG = {
    url: '', token: '', expires: 0, session: null,
    // file is the org character sheet this page was exported from, which is
    // what inventory changes are written back to.
    file: '',
    rolls: [], notes: [], stream: [],
    busy: false, error: '', needLogin: false
  };
  var CHARACTER = '', CHARACTER_ID = '';
  var STORE_KEY = 'orgs.dnd.sessionlog';
  var drawer, notesTab, sessPanel, view = 'notes', refreshTimer = null;

  var QUILL_ICON = '<svg viewBox="0 0 24 24" aria-hidden="true" focusable="false">' +
    '<path d="M20.5 3.2c-6 .6-10.4 3.4-12.6 7.3-1 1.8-1.4 3.6-1.5 5.2"/>' +
    '<path d="M3.6 20.4c1.2-2.6 3-4.6 5.2-6"/>' +
    '<path d="M9.2 15.9c4.4.5 8-1.6 9.6-5.2"/></svg>';

  // ------------------------------------------------------------ storage
  function saveLog() {
    try {
      localStorage.setItem(STORE_KEY, JSON.stringify({
        url: LOG.url, token: LOG.token, expires: LOG.expires,
        session: LOG.session, rolls: LOG.rolls, notes: LOG.notes
      }));
    } catch (e) { /* private browsing, nothing worth doing about it */ }
  }

  function loadLog() {
    var raw = null;
    try { raw = localStorage.getItem(STORE_KEY); } catch (e) { return; }
    if (!raw) { return; }
    try {
      var v = JSON.parse(raw) || {};
      if (v.url) { LOG.url = v.url; }
      LOG.token = v.token || '';
      LOG.expires = v.expires || 0;
      LOG.session = v.session || null;
      LOG.rolls = v.rolls || [];
      LOG.notes = v.notes || [];
    } catch (e) { /* a corrupt entry is not worth a broken sheet */ }
  }

  // ------------------------------------------------------------- server
  function apiUrl(path) {
    return String(LOG.url || '').replace(/\/+$/, '') + path;
  }

  function loginNeeded() {
    LOG.needLogin = true;
    var e = new Error('sign in to the orgs server');
    e.status = 401;
    return e;
  }

  function request(method, path, body, auth) {
    if (!LOG.url) { return Promise.reject(new Error('no orgs server address set')); }
    var opts = { method: method, headers: {}, mode: 'cors' };
    if (body !== undefined && body !== null) {
      opts.headers['Content-Type'] = 'application/json';
      opts.body = JSON.stringify(body);
    }
    if (auth !== false && LOG.token) { opts.headers.Authorization = 'Bearer ' + LOG.token; }
    return fetch(apiUrl(path), opts).then(function (res) {
      if (res.status === 401 || res.status === 403) { throw loginNeeded(); }
      if (!res.ok) {
        return res.text().then(function (text) {
          var e = new Error(serverError(text) || ('the server said ' + res.status));
          e.status = res.status;
          throw e;
        });
      }
      var type = res.headers.get('content-type') || '';
      return type.indexOf('json') >= 0 ? res.json() : res.text();
    });
  }

  // The api returns either a plain string or an {Ok, Msg} envelope on failure.
  // Go marshals that envelope with capitals and some handlers answer in lower
  // case, so both spellings are read - otherwise a refused change shows the
  // reader the raw json instead of the sentence inside it.
  function serverError(text) {
    if (!text) { return ''; }
    try {
      var v = JSON.parse(text);
      if (typeof v === 'string') { return v; }
      if (v && (v.Msg || v.msg)) { return v.Msg || v.msg; }
    } catch (e) { /* not json, use it as it came */ }
    return String(text).slice(0, 200);
  }

  // A token only lasts an hour, so renew it before it lapses rather than
  // finding out halfway through a fight.
  function api(method, path, body) {
    var soon = LOG.expires && Date.now() > LOG.expires - 30000;
    var start = soon && LOG.token ? renewToken() : Promise.resolve();
    return start.then(function () {
      return request(method, path, body);
    }).catch(function (err) {
      if (err.status !== 401 || !LOG.token) { throw err; }
      return renewToken().then(function () { return request(method, path, body); });
    });
  }

  function renewToken() {
    if (!LOG.token) { return Promise.reject(loginNeeded()); }
    return request('POST', '/refresh', null).then(keepToken, function () {
      LOG.token = '';
      saveLog();
      throw loginNeeded();
    });
  }

  function keepToken(res) {
    LOG.token = (res && res.token) || '';
    LOG.expires = res && res.ExpiresAt ? Date.parse(res.ExpiresAt) : 0;
    LOG.needLogin = false;
    saveLog();
    scheduleRenew();
    return res;
  }

  function scheduleRenew() {
    if (refreshTimer) { clearTimeout(refreshTimer); refreshTimer = null; }
    if (!LOG.expires || !LOG.token) { return; }
    var wait = LOG.expires - Date.now() - 60000;
    refreshTimer = setTimeout(function () {
      renewToken().catch(function () { renderSession(); });
    }, Math.max(15000, wait));
  }

  function signIn(user, pass) {
    return request('POST', '/login', { username: user, password: pass }, false)
      .then(keepToken);
  }

  // ------------------------------------------------------- the write queue
  // Rolls and notes are queued and flushed, so a server that is briefly away
  // costs you nothing: the queue is on disk and goes out with the next one.
  function flush() {
    if (LOG.busy || !LOG.session || !LOG.url) { return; }
    var rolls = LOG.rolls.slice(0, 25);
    var notes = LOG.notes.slice(0, 10);
    if (!rolls.length && !notes.length) { return; }
    var path = '/dnd/play/session/' + encodeURIComponent(LOG.session.id);
    var send = rolls.length
      ? api('POST', path + '/roll', who({ rolls: rolls }))
      : api('POST', path + '/note', who({ notes: notes }));
    LOG.busy = true;
    send.then(function (info) {
      if (rolls.length) { LOG.rolls = LOG.rolls.slice(rolls.length); }
      else { LOG.notes = LOG.notes.slice(notes.length); }
      if (info && info.id) { LOG.session = info; }
      LOG.busy = false;
      LOG.error = '';
      saveLog();
      renderSession();
      flush();
    }, function (err) {
      LOG.busy = false;
      LOG.error = err.message || String(err);
      renderSession();
    });
  }

  function pending() { return LOG.rolls.length + LOG.notes.length; }

  // Every write names the character, so the session file can list who played
  // and carry their id for later queries.
  function who(body) {
    body.character = CHARACTER;
    body.characterId = CHARACTER_ID;
    return body;
  }

  function clockNow() {
    var now = new Date();
    return ('0' + now.getHours()).slice(-2) + ':' + ('0' + now.getMinutes()).slice(-2);
  }

  // logRoll turns a result from the tray into a row of the session table. The
  // table is org text, so a cast is written out in words - the spell mark the
  // tray draws stays in the tray.
  function logRoll(r) {
    if (!LOG.session) { return; }
    var row = { time: r.time, character: CHARACTER, label: r.label, formula: r.formula };
    if (r.kind === 'cast') {
      var notes = [];
      row.formula = [r.attack ? 'attack ' + r.attack.formula : '',
                     r.damage ? r.damage.formula : ''].filter(Boolean).join(', ') || 'cast';
      row.result = r.damage ? String(r.damage.total)
                            : (r.attack ? String(r.attack.normal) : '');
      row.dice = r.dice || '';
      if (r.attack) {
        notes.push('hit ' + r.attack.normal + ' (adv ' + r.attack.adv +
          ', dis ' + r.attack.dis + ')');
      }
      if (r.damage && r.damage.crit !== null && r.damage.crit !== undefined) {
        notes.push('crit ' + r.damage.crit);
      }
      if (r.line) { notes.push(r.line); }
      if (r.detail) { notes.push(r.detail); }
      row.notes = notes.join('; ');
      LOG.rolls.push(row);
      saveLog();
      flush();
      renderSession();
      return;
    }
    if (r.kind === 'check') {
      row.result = String(r.normal);
      row.dice = 'd20 ' + r.pair[0] + ', d20 ' + r.pair[1];
      row.notes = 'adv ' + r.adv + ', dis ' + r.dis +
        (r.mod ? ', mod ' + signed(r.mod) : '');
    } else {
      row.result = String(r.total);
      row.dice = r.detail;
      row.notes = (r.crit === null || r.crit === undefined) ? '' : 'crit ' + r.crit;
    }
    LOG.rolls.push(row);
    saveLog();
    flush();
    renderSession();
  }

  function logNote(text) {
    if (!LOG.session || !String(text).trim()) { return; }
    var note = { time: clockNow(), text: text };
    LOG.notes.push(note);
    LOG.stream.push(note);
    saveLog();
    flush();
    renderNotes();
  }

  // --------------------------------------------------------- session state
  function startSession(name, summary) {
    return api('POST', '/dnd/play/session', who({ name: name, summary: summary }))
      .then(function (info) {
        LOG.session = info;
        LOG.error = '';
        LOG.stream = [];
        saveLog();
        renderSession();
        flush();
        return loadStream(info.id);
      });
  }

  function resumeSession(info) {
    LOG.session = info;
    LOG.error = '';
    saveLog();
    renderSession();
    flush();
    return loadStream(info.id);
  }

  function stopSession() {
    LOG.session = null;
    LOG.stream = [];
    saveLog();
    renderSession();
    renderNotes();
  }

  // loadStream pulls the notes already in the file so the stream reads as the
  // whole evening, not just what this browser happens to have typed.
  function loadStream(id) {
    return api('GET', '/dnd/play/session/' + encodeURIComponent(id)).then(function (d) {
      LOG.stream = (d && d.noteLog) || [];
      renderNotes();
      return d;
    }, function (err) {
      LOG.error = err.message || String(err);
      renderNotes();
    });
  }

  // ---------------------------------------------------------- org markup
  // Just enough org to make a note look like a note: headings, lists, tables,
  // quotes, source blocks and the usual inline emphasis.
  function orgInline(s) {
    s = esc(s);
    s = s.replace(/\[\[([^\]]+)\]\[([^\]]+)\]\]/g, function (m, href, label) {
      return '<a href="' + href + '" rel="noreferrer noopener" target="_blank">' + label + '</a>';
    });
    s = s.replace(/\[\[([^\]]+)\]\]/g,
      '<a href="$1" rel="noreferrer noopener" target="_blank">$1</a>');
    s = s.replace(/(^|[\s([{])\*([^*\n]+)\*(?=$|[\s.,;:!?)\]}])/g, '$1<b>$2</b>');
    s = s.replace(/(^|[\s([{])\/([^/\n]+)\/(?=$|[\s.,;:!?)\]}])/g, '$1<i>$2</i>');
    s = s.replace(/(^|[\s([{])_([^_\n]+)_(?=$|[\s.,;:!?)\]}])/g, '$1<u>$2</u>');
    s = s.replace(/(^|[\s([{])=([^=\n]+)=(?=$|[\s.,;:!?)\]}])/g, '$1<code>$2</code>');
    s = s.replace(/(^|[\s([{])~([^~\n]+)~(?=$|[\s.,;:!?)\]}])/g, '$1<code>$2</code>');
    s = s.replace(/(^|[\s([{])\+([^+\n]+)\+(?=$|[\s.,;:!?)\]}])/g, '$1<s>$2</s>');
    return s;
  }

  function blockHtml(block) {
    if (block.kind === 'quote') {
      return '<blockquote>' + orgInline(block.text || '') + '</blockquote>';
    }
    return '<pre>' + esc(block.text || '') + '</pre>';
  }

  function orgToHtml(text) {
    var lines = String(text === null || text === undefined ? '' : text).split('\n');
    var out = [], list = null, table = false, block = null, para = [];
    function closeList() { if (list) { out.push('</' + list + '>'); list = null; } }
    function closeTable() { if (table) { out.push('</tbody></table>'); table = false; } }
    function closePara() {
      if (para.length) { out.push('<p>' + orgInline(para.join(' ')) + '</p>'); para = []; }
    }
    function closeAll() { closePara(); closeList(); closeTable(); }

    lines.forEach(function (line) {
      var t = line.trim();
      if (block) {
        if (/^#\+END_/i.test(t)) {
          out.push(blockHtml(block));
          block = null;
          return;
        }
        block.text = (block.text ? block.text + '\n' : '') + line;
        return;
      }
      var begin = /^#\+BEGIN_(\w+)/i.exec(t);
      if (begin) {
        closeAll();
        block = { text: '', kind: begin[1].toLowerCase() };
        return;
      }
      if (/^#\+/.test(t)) { return; }
      var head = /^(\*+)\s+(.*)$/.exec(t);
      if (head) {
        closeAll();
        var lvl = Math.min(4, head[1].length + 2);
        out.push('<h' + lvl + '>' + orgInline(head[2]) + '</h' + lvl + '>');
        return;
      }
      if (/^\|/.test(t)) {
        closePara(); closeList();
        if (/^\|[-+]/.test(t)) { return; }
        var cells = t.replace(/^\|/, '').replace(/\|$/, '').split('|');
        if (!table) {
          out.push('<table class="org-table"><tbody>');
          table = true;
        }
        out.push('<tr>' + cells.map(function (c) {
          return '<td>' + orgInline(c.trim()) + '</td>';
        }).join('') + '</tr>');
        return;
      }
      closeTable();
      var bullet = /^[-+*]\s+(.*)$/.exec(t);
      var number = /^(\d+)[.)]\s+(.*)$/.exec(t);
      if (bullet || number) {
        closePara();
        var want = bullet ? 'ul' : 'ol';
        if (list && list !== want) { closeList(); }
        if (!list) { out.push('<' + want + '>'); list = want; }
        out.push('<li>' + orgInline(bullet ? bullet[1] : number[2]) + '</li>');
        return;
      }
      if (t === '') { closePara(); closeList(); return; }
      para.push(t);
    });
    if (block) { out.push(blockHtml(block)); }
    closeAll();
    return out.join('');
  }

  // ------------------------------------------------------- the notes panel
  function setDrawer(open) {
    drawer.classList.toggle('open', open);
    notesTab.classList.toggle('raised', open);
    notesTab.setAttribute('aria-expanded', open ? 'true' : 'false');
    if (dock) { dock.classList.toggle('raised', open); }
    if (open) {
      var box = drawer.querySelector('#note-text');
      if (view === 'notes' && box) { box.focus(); }
    }
  }

  function setView(name) {
    view = name;
    Array.prototype.forEach.call(drawer.querySelectorAll('.nd-view'), function (el) {
      el.classList.toggle('on', el.id === 'view-' + name);
    });
    Array.prototype.forEach.call(drawer.querySelectorAll('.nd-tab'), function (el) {
      var owns = el.getAttribute('data-view') === name ||
        (name === 'detail' && el.getAttribute('data-view') === 'sessions');
      el.classList.toggle('on', owns);
    });
  }

  function noteCard(n) {
    return '<div class="note-entry"><div class="note-time">' + esc(n.time || '') + '</div>' +
      '<div class="org-rich">' + orgToHtml(n.text) + '</div></div>';
  }

  function renderNotes() {
    var stream = document.getElementById('note-stream');
    if (!stream) { return; }
    if (!LOG.session) {
      stream.innerHTML = '<div class="dt-empty">No session is being recorded. ' +
        'Press <b>Session</b> on the dice panel to start one and your notes and ' +
        'rolls will be written to an org file.</div>';
      return;
    }
    if (!LOG.stream.length) {
      stream.innerHTML = '<div class="dt-empty">No notes yet in ' +
        esc(LOG.session.name) + '.</div>';
      return;
    }
    var items = LOG.stream.slice().reverse().map(noteCard).join('');
    stream.innerHTML = items;
  }

  // Who played, for the session list and the session view.
  function castOf(s) {
    return (s.characters || []).map(function (c) { return c.name || c.id; })
      .filter(Boolean).join(', ');
  }

  function rollTable(rolls) {
    if (!rolls.length) { return '<div class="dt-empty">No rolls recorded.</div>'; }
    return '<table class="log-table"><thead><tr>' +
      '<th>Time</th><th>Who</th><th>Roll</th><th>Formula</th><th>Result</th><th>Dice</th>' +
      '</tr></thead><tbody>' +
      rolls.map(function (r) {
        return '<tr><td>' + esc(r.time || '') + '</td><td>' + esc(r.character || '') +
          '</td><td>' + esc(r.label || '') + '</td><td>' + esc(r.formula || '') +
          '</td><td class="num">' + esc(r.result || '') + '</td><td class="dim">' +
          esc(r.dice || '') + '</td></tr>';
      }).join('') + '</tbody></table>';
  }

  // ---------------------------------------------------------- session list
  function renderSessionList(list) {
    var el = document.getElementById('sessions-list');
    if (!el) { return; }
    if (!list.length) {
      el.innerHTML = '<div class="dt-empty">' +
        ((CHARACTER_ID || CHARACTER)
          ? 'No sessions recorded for ' + esc(CHARACTER || 'this character') + ' yet.'
          : 'No sessions yet.') + '</div>';
      return;
    }
    el.innerHTML = list.map(function (s) {
      var here = LOG.session && LOG.session.id === s.id;
      return '<button type="button" class="sess-row' + (here ? ' here' : '') +
        '" data-session="' + esc(s.id) + '">' +
        '<span class="sess-date">' + esc(s.date || '') + '</span>' +
        '<span class="sess-name">' + esc(s.name || s.id) + '</span>' +
        '<span class="sess-sum">' + esc(s.summary || 'No summary yet') + '</span>' +
        '<span class="sess-who">' + esc(castOf(s)) + '</span>' +
        '<span class="sess-meta">' + s.rolls + ' rolls &middot; ' + s.notes + ' notes</span>' +
        '</button>';
    }).join('');
  }

  // The sessions list is this character's, not the table's. The server filters
  // on the id embedded in each session file's Characters section, and the name
  // goes along for sessions logged before that id existed.
  function sessionsQuery() {
    var q = [];
    if (CHARACTER_ID) { q.push('character=' + encodeURIComponent(CHARACTER_ID)); }
    if (CHARACTER) { q.push('name=' + encodeURIComponent(CHARACTER)); }
    return q.length ? '?' + q.join('&') : '';
  }

  function loadSessions() {
    var el = document.getElementById('sessions-list');
    if (el) { el.innerHTML = '<div class="dt-empty">Loading&hellip;</div>'; }
    api('GET', '/dnd/play/sessions' + sessionsQuery()).then(function (list) {
      renderSessionList(list || []);
    }, function (err) {
      if (el) { el.innerHTML = '<div class="nd-err">' + esc(err.message || String(err)) + '</div>'; }
      renderSession();
    });
  }

  function openSession(id) {
    var el = document.getElementById('view-detail');
    el.innerHTML = '<div class="dt-empty">Loading&hellip;</div>';
    setView('detail');
    api('GET', '/dnd/play/session/' + encodeURIComponent(id)).then(renderDetail, function (err) {
      el.innerHTML = '<div class="nd-err">' + esc(err.message || String(err)) + '</div>';
    });
  }

  function renderDetail(d) {
    var el = document.getElementById('view-detail');
    var rolls = d.rollLog || [], notes = d.noteLog || [];
    var here = LOG.session && LOG.session.id === d.id;
    el.innerHTML =
      '<div class="nd-toolbar">' +
        '<button type="button" class="dt-btn" id="detail-back">&larr; Sessions</button>' +
        '<b class="nd-title">' + esc(d.name || d.id) + '</b>' +
        '<span class="nd-meta">' + esc(d.date || '') +
          (castOf(d) ? ' &middot; ' + esc(castOf(d)) : '') +
          ' &middot; ' + rolls.length + ' rolls &middot; ' + notes.length + ' notes</span>' +
        '<button type="button" class="dt-btn" id="detail-resume"' + (here ? ' disabled' : '') +
          '>' + (here ? 'Recording here' : 'Record here') + '</button>' +
      '</div>' +
      '<div class="nd-toolbar">' +
        '<input type="text" id="detail-summary" class="nd-input" placeholder="One line summary" ' +
          'value="' + esc(d.summary || '') + '">' +
        '<button type="button" class="dt-btn" id="detail-summary-save">Save summary</button>' +
      '</div>' +
      '<div class="nd-detail">' +
        '<div class="nd-col"><div class="nd-sub">Notes</div>' +
          (notes.length ? notes.map(noteCard).join('') : '<div class="dt-empty">No notes.</div>') +
        '</div>' +
        '<div class="nd-col"><div class="nd-sub">Rolls</div>' + rollTable(rolls) + '</div>' +
      '</div>';

    el.querySelector('#detail-back').addEventListener('click', function () {
      setView('sessions');
      loadSessions();
    });
    el.querySelector('#detail-resume').addEventListener('click', function () {
      resumeSession({ id: d.id, name: d.name, date: d.date, file: d.file,
                      summary: d.summary, rolls: d.rolls, notes: d.notes });
      setView('notes');
    });
    el.querySelector('#detail-summary-save').addEventListener('click', function () {
      var text = el.querySelector('#detail-summary').value;
      api('POST', '/dnd/play/session/' + encodeURIComponent(d.id) + '/summary',
          { summary: text }).then(function (info) {
        d.summary = info.summary;
        if (LOG.session && LOG.session.id === info.id) { LOG.session = info; saveLog(); }
        renderSession();
      }, function (err) {
        LOG.error = err.message || String(err);
        renderSession();
      });
    });
  }

  // --------------------------------------------------------------- search
  function runSearch() {
    var q = drawer.querySelector('#search-q').value;
    var el = drawer.querySelector('#search-results');
    if (!String(q).trim()) { el.innerHTML = ''; return; }
    el.innerHTML = '<div class="dt-empty">Searching&hellip;</div>';
    api('GET', '/dnd/play/search?q=' + encodeURIComponent(q)).then(function (hits) {
      hits = hits || [];
      if (!hits.length) {
        el.innerHTML = '<div class="dt-empty">Nothing in any session mentions ' +
          esc(q) + '.</div>';
        return;
      }
      el.innerHTML = hits.map(function (h) {
        return '<button type="button" class="hit" data-session="' + esc(h.id) + '">' +
          '<span class="hit-head">' + esc(h.name || h.id) +
            '<span class="hit-where">' + esc(h.date || '') +
            (h.context ? ' &middot; ' + esc(h.context) : '') + '</span></span>' +
          '<span class="hit-text">' + esc(h.text) + '</span>' +
          '</button>';
      }).join('');
    }, function (err) {
      el.innerHTML = '<div class="nd-err">' + esc(err.message || String(err)) + '</div>';
    });
  }

  // -------------------------------------------------- the session controls
  function setSessPanel(open) {
    sessPanel.classList.toggle('open', open);
    if (open) {
      renderSession();
      var name = sessPanel.querySelector('#sess-name');
      if (name && !LOG.session) { name.focus(); }
    }
  }

  function renderSession() {
    var recording = !!LOG.session;
    if (notesTab) {
      notesTab.classList.toggle('rec', recording);
      var badge = notesTab.querySelector('#notes-tab-badge');
      if (badge) { badge.textContent = recording ? LOG.session.name : ''; }
    }
    if (fab) { fab.classList.toggle('rec', recording); }
    var status = document.getElementById('nd-status');
    if (status) {
      status.innerHTML = recording
        ? '<span class="rec-dot"></span>' + esc(LOG.session.name) +
          (pending() ? ' <span class="nd-warn">' + pending() + ' waiting</span>' : '')
        : '<span class="nd-off">not recording</span>';
    }
    if (!sessPanel) { return; }
    var body = sessPanel.querySelector('#sess-body');
    var out = '';
    if (recording) {
      out += '<div class="sess-live"><span class="rec-dot"></span>' +
        esc(LOG.session.name) + '</div>' +
        '<div class="sess-file">' + esc(LOG.session.file || '') + '</div>' +
        '<div class="sess-counts">' + (LOG.session.rolls || 0) + ' rolls &middot; ' +
        (LOG.session.notes || 0) + ' notes' +
        (pending() ? ' &middot; <b>' + pending() + ' waiting to send</b>' : '') + '</div>' +
        '<div class="dc-actions">' +
          '<button type="button" class="dt-btn" id="sess-stop">Stop recording</button>' +
          '<button type="button" class="dt-btn" id="sess-notes">Notes</button>' +
        '</div>';
    } else {
      out += '<label class="sess-label" for="sess-name">Session name</label>' +
        '<input type="text" id="sess-name" class="nd-input" placeholder="' +
          esc(defaultSessionName()) + '" autocomplete="off">' +
        '<label class="sess-label" for="sess-summary">Summary</label>' +
        '<input type="text" id="sess-summary" class="nd-input" ' +
          'placeholder="One line, shown in the session list" autocomplete="off">' +
        '<div class="dc-actions"><button type="button" class="dc-roll" id="sess-start">' +
          'Start session</button></div>';
    }
    if (LOG.needLogin) {
      out += '<div class="sess-login"><div class="sess-label">Sign in to ' +
        esc(LOG.url || 'the server') + '</div>' +
        '<input type="text" id="sess-user" class="nd-input" placeholder="user" ' +
          'autocomplete="username">' +
        '<input type="password" id="sess-pass" class="nd-input" placeholder="password" ' +
          'autocomplete="current-password">' +
        '<div class="dc-actions"><button type="button" class="dt-btn" id="sess-login">' +
          'Sign in</button></div></div>';
    }
    out += '<details class="sess-server"' + (LOG.url ? '' : ' open') + '>' +
      '<summary>Server</summary>' +
      '<input type="text" id="sess-url" class="nd-input" placeholder="http://localhost:8010" ' +
        'value="' + esc(LOG.url || '') + '"></details>';
    if (LOG.error) { out += '<div class="nd-err">' + esc(LOG.error) + '</div>'; }
    body.innerHTML = out;
  }

  function defaultSessionName() {
    var d = new Date();
    return 'Session ' + d.toDateString().slice(0, 10);
  }

  function sessionAction(e) {
    var id = e.target.id || (e.target.closest('button') || {}).id;
    if (!id) { return; }
    if (id === 'sess-start') {
      LOG.error = '';
      var name = sessPanel.querySelector('#sess-name').value;
      var summary = sessPanel.querySelector('#sess-summary').value;
      startSession(name, summary).then(function () {
        setSessPanel(false);
        setDrawer(true);
        setView('notes');
      }, function (err) {
        LOG.error = err.message || String(err);
        renderSession();
      });
      return;
    }
    if (id === 'sess-stop') { stopSession(); return; }
    if (id === 'sess-notes') { setSessPanel(false); setDrawer(true); setView('notes'); return; }
    if (id === 'sess-login') {
      var user = sessPanel.querySelector('#sess-user').value;
      var pass = sessPanel.querySelector('#sess-pass').value;
      LOG.error = '';
      signIn(user, pass).then(function () {
        renderSession();
        flush();
        // The inventory could not be read while we were signed out.
        invLoad();
      }, function (err) {
        LOG.error = err.message || String(err);
        renderSession();
      });
    }
  }

  function buildSessionPanel() {
    sessPanel = document.createElement('aside');
    sessPanel.id = 'dnd-sess';
    sessPanel.setAttribute('aria-label', 'Play session');
    sessPanel.innerHTML =
      '<div class="dc-head"><h3>Play Session</h3>' +
        '<button type="button" class="dt-btn dt-x" id="sess-close" ' +
        'aria-label="Close session panel">&times;</button></div>' +
      '<div id="sess-body"></div>' +
      '<div class="dc-hint">Rolls and notes are appended to a dated org file ' +
        'in your session folder.</div>';
    dock.insertBefore(sessPanel, dock.firstChild);
    sessPanel.querySelector('#sess-close').addEventListener('click', function () {
      setSessPanel(false);
    });
    sessPanel.addEventListener('click', sessionAction);
    sessPanel.addEventListener('keydown', function (e) {
      if (e.key !== 'Enter') { return; }
      if (e.target.id === 'sess-name' || e.target.id === 'sess-summary') {
        e.preventDefault();
        sessionAction({ target: { id: 'sess-start' } });
      } else if (e.target.id === 'sess-user' || e.target.id === 'sess-pass') {
        e.preventDefault();
        sessionAction({ target: { id: 'sess-login' } });
      }
    });
    sessPanel.addEventListener('change', function (e) {
      if (e.target.id !== 'sess-url') { return; }
      LOG.url = e.target.value.trim();
      LOG.error = '';
      saveLog();
    });
  }

  function buildNotesDrawer() {
    notesTab = document.createElement('button');
    notesTab.id = 'notes-tab';
    notesTab.type = 'button';
    notesTab.setAttribute('aria-expanded', 'false');
    notesTab.innerHTML = QUILL_ICON + '<span>Session Notes</span>' +
      '<em id="notes-tab-badge"></em>';
    document.body.appendChild(notesTab);

    drawer = document.createElement('aside');
    drawer.id = 'notes-drawer';
    drawer.setAttribute('aria-label', 'Session notes');
    drawer.innerHTML =
      '<div class="nd-head">' +
        '<h2>' + QUILL_ICON + 'Session</h2>' +
        '<div class="nd-tabs">' +
          '<button type="button" class="nd-tab on" data-view="notes">Notes</button>' +
          '<button type="button" class="nd-tab" data-view="sessions">Sessions</button>' +
          '<button type="button" class="nd-tab" data-view="inventory">Inventory</button>' +
          '<button type="button" class="nd-tab" data-view="search">Search</button>' +
        '</div>' +
        '<span class="nd-status" id="nd-status"></span>' +
        '<button type="button" class="dt-btn dt-x" id="notes-close" ' +
          'aria-label="Hide session notes">&times;</button>' +
      '</div>' +
      '<div class="nd-body">' +
        '<div class="nd-view on" id="view-notes"><div class="nd-split">' +
          '<div class="nd-compose">' +
            '<textarea id="note-text" spellcheck="true" ' +
              'placeholder="What just happened?&#10;&#10;Org markup works here: * a heading, ' +
              '- a list, *bold*, /italic/, =code= and [[links][links]]."></textarea>' +
            '<div class="nd-compose-foot">' +
              '<span class="nd-hint">Ctrl + Enter adds the note</span>' +
              '<button type="button" class="dt-btn" id="note-preview-btn">Preview</button>' +
              '<button type="button" class="dc-roll" id="note-save">Add note</button>' +
            '</div>' +
            '<div class="org-rich note-preview" id="note-preview" hidden></div>' +
          '</div>' +
          '<div class="nd-stream" id="note-stream"></div>' +
        '</div></div>' +
        '<div class="nd-view" id="view-sessions">' +
          '<div class="nd-toolbar">' +
            '<button type="button" class="dt-btn" id="sessions-refresh">Refresh</button>' +
            '<span class="nd-meta">Every session you have played, newest first. ' +
              'Pick one to read its notes and rolls.</span>' +
          '</div>' +
          '<div id="sessions-list"></div>' +
        '</div>' +
        '<div class="nd-view" id="view-inventory">' +
          '<div class="nd-toolbar">' +
            '<button type="button" class="dt-btn" id="inv-history-refresh">Refresh</button>' +
            '<span class="nd-meta">Everything you have picked up, used, dropped ' +
              'or stowed, newest first. This one lives on your character sheet.</span>' +
          '</div>' +
          '<div id="inv-history"></div>' +
        '</div>' +
        '<div class="nd-view" id="view-search">' +
          '<div class="nd-toolbar">' +
            '<input type="search" id="search-q" class="nd-input" ' +
              'placeholder="Search every session: a name, a place, a spell...">' +
            '<button type="button" class="dt-btn" id="search-go">Search</button>' +
          '</div>' +
          '<div id="search-results"></div>' +
        '</div>' +
        '<div class="nd-view" id="view-detail"></div>' +
      '</div>';
    document.body.appendChild(drawer);

    notesTab.addEventListener('click', function () {
      setDrawer(!drawer.classList.contains('open'));
    });
    drawer.querySelector('#notes-close').addEventListener('click', function () {
      setDrawer(false);
    });
    drawer.querySelector('.nd-tabs').addEventListener('click', function (e) {
      var b = e.target.closest('.nd-tab');
      if (!b) { return; }
      var name = b.getAttribute('data-view');
      setView(name);
      if (name === 'sessions') { loadSessions(); }
      if (name === 'inventory') { renderInvHistory(); invLoad(); }
    });

    var box = drawer.querySelector('#note-text');
    var preview = drawer.querySelector('#note-preview');
    drawer.querySelector('#note-save').addEventListener('click', function () {
      if (!LOG.session) {
        setSessPanel(true);
        return;
      }
      logNote(box.value);
      box.value = '';
      preview.hidden = true;
      box.focus();
    });
    drawer.querySelector('#note-preview-btn').addEventListener('click', function () {
      preview.hidden = !preview.hidden;
      if (!preview.hidden) { preview.innerHTML = orgToHtml(box.value); }
    });
    box.addEventListener('keydown', function (e) {
      if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
        e.preventDefault();
        drawer.querySelector('#note-save').click();
      }
    });
    box.addEventListener('input', function () {
      if (!preview.hidden) { preview.innerHTML = orgToHtml(box.value); }
    });

    drawer.querySelector('#sessions-refresh').addEventListener('click', loadSessions);
    drawer.querySelector('#inv-history-refresh').addEventListener('click', function () {
      invLoad();
    });
    drawer.querySelector('#view-sessions').addEventListener('click', function (e) {
      var row = e.target.closest('[data-session]');
      if (row) { openSession(row.getAttribute('data-session')); }
    });
    drawer.querySelector('#search-go').addEventListener('click', runSearch);
    drawer.querySelector('#search-q').addEventListener('keydown', function (e) {
      if (e.key === 'Enter') { e.preventDefault(); runSearch(); }
    });
    drawer.querySelector('#search-results').addEventListener('click', function (e) {
      var row = e.target.closest('[data-session]');
      if (row) { openSession(row.getAttribute('data-session')); }
    });
  }

  // The sheet is normally opened straight off disk, so the server it was
  // exported from is baked in; anything else is remembered from last time.
  function buildSessionLog() {
    var cfg = document.getElementById('dnd-log-config');
    if (cfg) {
      LOG.url = cfg.getAttribute('data-server') || '';
      CHARACTER = cfg.getAttribute('data-character') || '';
      CHARACTER_ID = cfg.getAttribute('data-character-id') || '';
      LOG.file = cfg.getAttribute('data-file') || '';
    }
    if (!LOG.url && location.protocol.indexOf('http') === 0) { LOG.url = location.origin; }
    loadLog();
    buildSessionPanel();
    buildNotesDrawer();
    renderSession();
    renderNotes();
    if (LOG.session) {
      scheduleRenew();
      loadStream(LOG.session.id);
      flush();
    }
    // Anything the server could not take is retried quietly in the background.
    setInterval(function () { if (pending()) { flush(); } }, 20000);
  }

  // ------------------------------------------------------------- inventory
  //
  // The equipment box is the character's bag: things stack, they live either
  // on the character or in one of the containers they own, and every change
  // is written straight back into the org character sheet along with a line
  // in its Inventory History. The panel is drawn from the inventory the
  // server computed, so weight and encumbrance are the rules engine's answer
  // rather than something worked out twice in two places.
  var INV = { state: null, where: '', busy: false, error: '', msg: '' };
  var invBox, invLive, invEnc, invAdd, invMove, invHits = [], invHit = -1;
  var invSearchTimer = null, invMoveWhat = null, invMode = 'move';
  var invFilter = 'all';

  // The groups the add box searches in. The ids are the ones the server knows
  // in dnd.ItemFilters - a name it does not know comes back as an error, not
  // as everything - so the two lists have to say the same thing.
  var INV_FILTERS = [
    { id: 'all', label: 'All' },
    { id: 'weapon', label: 'Weapons' },
    { id: 'armor', label: 'Armor' },
    { id: 'potion', label: 'Potions' },
    { id: 'focus', label: 'Components' },
    { id: 'gear', label: 'Gear' },
    { id: 'tool', label: 'Tools' },
    { id: 'pack', label: 'Packs' },
    { id: 'container', label: 'Containers' },
    { id: 'magic', label: 'Magic' }
  ];

  function invFilterLabel(id) {
    var out = '';
    INV_FILTERS.forEach(function (f) { if (f.id === id) { out = f.label; } });
    return out;
  }

  // How rare a thing is, as a class the stylesheet colours. Mundane gear has
  // no rarity and keeps the ink colour it would have had.
  function rarityClass(rarity) {
    var r = String(rarity || '').toLowerCase().trim();
    if (!r) { return ''; }
    return 'rar rar-' + r.replace(/[^a-z0-9]+/g, '-');
  }

  function invConfigured() { return !!(LOG.file || CHARACTER_ID); }

  // who the request is about: the file it came from, and the id as a fallback
  // for a sheet that has been moved somewhere else.
  function invWho(body) {
    body = body || {};
    body.filename = LOG.file || '';
    body.id = CHARACTER_ID || '';
    return body;
  }

  function invLoad(quiet) {
    if (!invConfigured()) { return Promise.resolve(); }
    var q = '/dnd/inventory?filename=' + encodeURIComponent(LOG.file || '') +
      '&id=' + encodeURIComponent(CHARACTER_ID || '');
    return api('GET', q).then(function (state) {
      INV.state = state;
      INV.error = '';
      renderInventory();
      renderInvHistory();
    }, function (err) {
      if (!quiet) { INV.error = err.message || String(err); renderInventory(); }
    });
  }

  // invChange posts one change and redraws from what comes back, so the panel
  // always shows what is actually written in the file.
  function invChange(body) {
    if (!invConfigured()) {
      INV.error = 'this sheet does not know which org file it came from';
      renderInventory();
      return Promise.reject(new Error(INV.error));
    }
    INV.busy = true;
    INV.error = '';
    renderInventory();
    return api('POST', '/dnd/inventory', invWho(body)).then(function (state) {
      INV.busy = false;
      INV.state = state;
      INV.msg = state.msg || '';
      if (state.inventory && !containerOf(state.inventory, INV.where)) { INV.where = ''; }
      renderInventory();
      renderInvHistory();
      return state;
    }, function (err) {
      INV.busy = false;
      INV.error = err.message || String(err);
      renderInventory();
      throw err;
    });
  }

  function containerOf(inv, key) {
    var found = null;
    (inv.containers || []).forEach(function (b) { if (b.key === key) { found = b; } });
    return found;
  }

  function invContainers() {
    return (INV.state && INV.state.inventory && INV.state.inventory.containers) || [];
  }

  // Every container something can be put into: the person, plus the
  // containers actually owned. A container that has gone missing is not
  // offered, only shown.
  function invDestinations() {
    return invContainers().filter(function (b) { return !b.missing; });
  }

  function weightStr(n) {
    n = Math.round((Number(n) || 0) * 100) / 100;
    return String(n);
  }

  function renderEncumbrance(inv) {
    if (!invEnc) { return; }
    var pct = inv.carryCapacity ? Math.min(100, inv.weight / inv.carryCapacity * 100) : 0;
    var mark = function (at) {
      if (!inv.carryCapacity || !at) { return ''; }
      return '<span class="enc-mark" style="left:' +
        Math.min(100, at / inv.carryCapacity * 100) + '%"></span>';
    };
    invEnc.innerHTML =
      '<div class="enc-line">' +
        '<span class="wt">' + weightStr(inv.weight) + ' lb</span>' +
        '<span class="cap">of ' + inv.carryCapacity + ' lb carried</span>' +
        '<span class="enc-badge ' + esc(inv.level || '') + '">' + esc(inv.label || '') + '</span>' +
      '</div>' +
      '<div class="enc-bar"><div class="enc-fill ' + esc(inv.level || '') +
        '" style="width:' + pct + '%"></div>' + mark(inv.encumberedAt) +
        mark(inv.heavilyEncumberedAt) + '</div>' +
      '<div class="enc-note">' +
        (inv.effect ? '<b>' + esc(inv.effect) + '</b> ' : '') +
        'Encumbered over ' + inv.encumberedAt + ' lb, heavily over ' +
        inv.heavilyEncumberedAt + ' lb, push, drag or lift ' + inv.pushDragLift + ' lb.' +
        (inv.stored ? ' Another ' + weightStr(inv.stored) +
          ' lb is stowed in extradimensional space and is not carried.' : '') +
      '</div>';
  }

  // The Worn cell. Anything that can be worn or wielded gets a button that
  // toggles it; everything else keeps the plain tick it always had. So does
  // anything stowed in a container - something in a backpack is not being
  // worn, which is the same rule that takes gear off when it is packed away,
  // so it has to come out before it can go on.
  function invWorn(e) {
    var star = e.attuned ? ' <span class="worn" title="Attuned">&#9733;</span>' : '';
    if (!e.wearable || e.container) {
      var tick = e.equipped
        ? '<span class="worn" title="Worn or wielded">&#10003;</span>' : '';
      return tick + star;
    }
    var on = !!e.equipped;
    return '<button type="button" class="wearbtn' + (on ? ' on' : '') +
      '" data-wear="' + (on ? '0' : '1') +
      '" aria-pressed="' + (on ? 'true' : 'false') +
      '" aria-label="' + (on ? 'Worn, take off ' : 'Wear or wield ') + esc(e.name) +
      '" title="' + (on ? 'Worn or wielded &mdash; click to take it off'
                        : 'Click to wear or wield this') + '">' +
      (on ? '&#10003;' : '&#9675;') + '</button>' + star;
  }

  function invRow(e) {
    var notes = e.notes ? ' <span class="tagline">(' + esc(e.notes) + ')</span>' : '';
    var tag = e.isContainer ? ' <span class="tagline">holds things</span>' : '';
    var rare = e.rarity
      ? ' <span class="rar-tag ' + rarityClass(e.rarity) + '">' + esc(e.rarity) + '</span>'
      : '';
    return '<tr data-item="' + esc(e.key) + '" data-name="' + esc(e.name) + '">' +
      '<td class="inv-name"><span class="' + rarityClass(e.rarity) + '">' +
        esc(e.name) + '</span>' + rare + tag + notes + '</td>' +
      '<td class="num qty">' + e.qty + '</td>' +
      '<td class="num">' + (e.total ? weightStr(e.total) : '') + '</td>' +
      '<td class="inv-worn">' + invWorn(e) + '</td>' +
      '<td class="inv-actions">' +
        '<button type="button" class="ib use" data-act="use" title="Use one, ' +
          'recorded in your inventory history">Use</button>' +
        '<button type="button" class="ib" data-act="move" title="Move into another container">Move</button>' +
        '<button type="button" class="ib drop" data-act="drop" title="Drop or give away">Drop</button>' +
      '</td></tr>';
  }

  function invPane(box) {
    var cap = '';
    if (box.capacity) {
      cap = '<p class="inv-cap' + (box.over ? ' over' : '') + '"><b>' +
        weightStr(box.weight) + ' lb</b> of ' + box.capacity + ' lb' +
        (box.over ? ' &mdash; overfull' : '') + '</p>';
    } else if (box.key) {
      cap = '<p class="inv-cap"><b>' + weightStr(box.weight) + ' lb</b> inside</p>';
    }
    if (box.extradimensional) {
      cap += '<p class="inv-cap">Extradimensional, its contents are not carried.</p>';
    }
    if (box.missing) {
      cap += '<p class="inv-cap over">You no longer carry this container. ' +
        'Move what is left somewhere else.</p>';
    }
    var body = box.entries && box.entries.length
      ? '<div class="table-wrap"><table class="inv-table">' +
          '<thead><tr><th>Item</th><th class="num">Qty</th><th class="num">Wt</th>' +
          '<th class="inv-worn">Worn</th><th class="inv-actions"></th></tr></thead><tbody>' +
          box.entries.map(invRow).join('') +
        '</tbody></table></div>'
      : '<div class="inv-empty">Nothing here yet.</div>';
    return '<div class="inv-pane' + (box.key === INV.where ? ' on' : '') +
      '" data-pane="' + esc(box.key) + '">' +
      '<h3 class="inv-pane-name">' + esc(box.name) + '</h3>' + cap + body + '</div>';
  }

  function renderInventory() {
    if (!invLive) { return; }
    var inv = INV.state && INV.state.inventory;
    if (!inv) { return; }
    renderEncumbrance(inv);
    var boxes = inv.containers || [];
    if (!containerOf(inv, INV.where)) { INV.where = ''; }
    var tabs = boxes.map(function (b) {
      return '<button type="button" class="inv-tab' + (b.key === INV.where ? ' on' : '') +
        (b.over ? ' over' : '') + '" data-tab="' + esc(b.key) + '">' + esc(b.name) +
        '<span class="n">' + b.items + '</span></button>';
    }).join('');
    invLive.innerHTML =
      '<div class="inv-tabs">' + tabs + '</div>' +
      boxes.map(invPane).join('') +
      '<div class="inv-foot">' +
        '<button type="button" class="inv-add" id="inv-add-btn">+ Add item</button>' +
        (INV.busy ? '<span class="inv-msg">saving&hellip;</span>'
                  : (INV.msg ? '<span class="inv-msg">' + esc(INV.msg) + '</span>' : '')) +
        (INV.error ? '<span class="inv-err">' + esc(INV.error) + '</span>' : '') +
      '</div>';
  }

  // ------------------------------------------------------- the add item box
  // Searches are answered out of order when one request is slower than the
  // next - a token renewal in the middle of typing is enough - so anything
  // but the newest answer is dropped rather than drawn over the newer one.
  var invSearchSeq = 0;

  function invSearch(q) {
    var list = invAdd.querySelector('#inv-results');
    var mine = ++invSearchSeq;
    var path = '/dnd/items?limit=40&q=' + encodeURIComponent(q) +
      '&filter=' + encodeURIComponent(invFilter) +
      '&filename=' + encodeURIComponent(LOG.file || '') +
      '&id=' + encodeURIComponent(CHARACTER_ID || '');
    api('GET', path).then(function (hits) {
      if (mine !== invSearchSeq) { return; }
      invHits = hits || [];
      invHit = invHits.length ? 0 : -1;
      invShowPick();
      if (!invHits.length) {
        list.innerHTML = '<div class="inv-empty">Nothing ' +
          (invFilter === 'all' ? '' : 'in ' + esc(invFilterLabel(invFilter)) + ' ') +
          'matches. Anything you type is added as it stands' +
          (invFilter === 'all' ? '.' : ', or look in All.') + '</div>';
        return;
      }
      list.innerHTML = invHits.map(function (h, i) {
        var meta = [h.category || h.kind, h.cost, h.weight ? h.weight + ' lb' : '',
          h.damage].filter(Boolean).join(' · ');
        var rar = h.rarity
          ? '<span class="rar-tag ' + rarityClass(h.rarity) + '">' + esc(h.rarity) + '</span>'
          : '';
        return '<button type="button" class="inv-hit' + (i === invHit ? ' on' : '') +
          '" data-hit="' + i + '"><span class="nm ' + rarityClass(h.rarity) + '">' +
          esc(h.name) + '</span>' + rar +
          (h.isContainer ? '<span class="box-tag">container</span>' : '') +
          (h.owned ? ' <span class="own">you have ' + h.owned + '</span>' : '') +
          '<div class="meta">' + esc(meta) + '</div></button>';
      }).join('');
    }, function (err) {
      if (mine !== invSearchSeq) { return; }
      list.innerHTML = '<div class="inv-err">' + esc(err.message || String(err)) + '</div>';
    });
  }

  function renderInvFilters() {
    var el = invAdd && invAdd.querySelector('#inv-filters');
    if (!el) { return; }
    el.innerHTML = INV_FILTERS.map(function (f) {
      return '<button type="button" class="inv-filter' +
        (f.id === invFilter ? ' on' : '') + '" data-filter="' + esc(f.id) + '">' +
        esc(f.label) + '</button>';
    }).join('');
  }

  function invPickHit(i) {
    if (i < 0 || i >= invHits.length) { return; }
    invHit = i;
    Array.prototype.forEach.call(invAdd.querySelectorAll('.inv-hit'), function (el, n) {
      el.classList.toggle('on', n === i);
    });
    var el = invAdd.querySelectorAll('.inv-hit')[i];
    if (el && el.scrollIntoView) { el.scrollIntoView({ block: 'nearest' }); }
    invShowPick();
  }

  // The name of whatever the Add button would add right now.
  function invShowPick() {
    var el = invAdd.querySelector('#inv-picked');
    if (!el) { return; }
    var typed = invAdd.querySelector('#inv-q').value.trim();
    if (invHit >= 0 && invHits[invHit]) {
      el.innerHTML = 'Adding <b>' + esc(invHits[invHit].name) + '</b>';
    } else if (typed) {
      el.innerHTML = 'Adding <b>' + esc(typed) + '</b>, which the rules do not know';
    } else {
      el.textContent = '';
    }
  }

  function invWhereOptions(selected, skip) {
    return invDestinations().filter(function (b) { return b.key !== skip; })
      .map(function (b) {
        return '<option value="' + esc(b.key) + '"' +
          (b.key === selected ? ' selected' : '') + '>' + esc(b.name) + '</option>';
      }).join('');
  }

  function openAdd() {
    if (!invAdd) { return; }
    invAdd.querySelector('#inv-where').innerHTML = invWhereOptions(INV.where, null);
    invAdd.querySelector('#inv-qty').value = '1';
    invAdd.querySelector('#inv-err').textContent = '';
    invAdd.classList.add('open');
    invHits = [];
    invHit = -1;
    invFilter = 'all';
    renderInvFilters();
    var q = invAdd.querySelector('#inv-q');
    q.value = '';
    q.focus();
    invShowPick();
    invSearch('');
  }

  function closeAdd() { if (invAdd) { invAdd.classList.remove('open'); } }

  function doAdd() {
    var q = invAdd.querySelector('#inv-q').value.trim();
    var qty = parseInt(invAdd.querySelector('#inv-qty').value, 10) || 1;
    var where = invAdd.querySelector('#inv-where').value;
    var hit = invHit >= 0 ? invHits[invHit] : null;
    // A search that matched nothing is still a thing you picked up: homebrew
    // and loot with no entry in the rules go in under the name as typed.
    var body = hit
      ? { action: 'add', item: hit.id, name: hit.name, qty: qty, container: where }
      : { action: 'add', item: '', name: q, qty: qty, container: where };
    if (!hit && !q) {
      invAdd.querySelector('#inv-err').textContent = 'what are you adding?';
      return;
    }
    invChange(body).then(function () {
      INV.where = where;
      renderInventory();
      closeAdd();
    }, function (err) {
      invAdd.querySelector('#inv-err').textContent = err.message || String(err);
    });
  }

  // -------------------------------------------------- the move and drop box
  function openMove(mode, entry, from) {
    invMode = mode;
    invMoveWhat = { key: entry.key, name: entry.name, qty: entry.qty, from: from };
    var title = (mode === 'move' ? 'Move ' : 'Drop ') + entry.name;
    invMove.querySelector('#inv-move-title').textContent = title;
    invMove.querySelector('#inv-move-qty').value = mode === 'move' ? entry.qty : 1;
    invMove.querySelector('#inv-move-qty').max = entry.qty;
    invMove.querySelector('#inv-move-max').textContent = 'of ' + entry.qty;
    invMove.querySelector('#inv-move-err').textContent = '';
    var dest = invMove.querySelector('#inv-move-where');
    var destRow = invMove.querySelector('#inv-move-dest');
    if (mode === 'move') {
      dest.innerHTML = invWhereOptions(null, from);
      destRow.hidden = false;
      if (!dest.options.length) {
        invMove.querySelector('#inv-move-err').textContent =
          'you have nothing else to put it in - add a backpack or a pouch first';
      }
    } else {
      destRow.hidden = true;
    }
    invMove.querySelector('#inv-move-go').textContent = mode === 'move' ? 'Move' : 'Drop';
    invMove.classList.add('open');
    invMove.querySelector('#inv-move-qty').focus();
  }

  function closeMove() { if (invMove) { invMove.classList.remove('open'); } }

  function doMove() {
    if (!invMoveWhat) { return; }
    var qty = parseInt(invMove.querySelector('#inv-move-qty').value, 10) || 1;
    var body = { item: invMoveWhat.key, name: invMoveWhat.name, qty: qty,
                 container: invMoveWhat.from };
    if (invMode === 'move') {
      body.action = 'move';
      body.to = invMove.querySelector('#inv-move-where').value;
      if (!body.to && body.to !== '') { return; }
    } else {
      body.action = 'drop';
    }
    invChange(body).then(closeMove, function (err) {
      invMove.querySelector('#inv-move-err').textContent = err.message || String(err);
    });
  }

  // ---------------------------------------------------------- the history
  function invHistoryRows(list) {
    if (!list || !list.length) {
      return '<div class="dt-empty">Nothing has come or gone yet. Add, use, ' +
        'drop or stow something on the sheet and it is written here and into ' +
        'your character file.</div>';
    }
    return '<table class="log-table"><thead><tr>' +
      '<th>Date</th><th>Time</th><th>What</th><th>Item</th><th>Qty</th><th>Where</th>' +
      '</tr></thead><tbody>' +
      list.slice().reverse().map(function (e) {
        var where = e.action === 'moved'
          ? esc(e.from || '') + ' &rarr; ' + esc(e.to || '')
          : esc(e.to || e.from || '');
        return '<tr><td>' + esc(e.date || '') + '</td><td>' + esc(e.time || '') +
          '</td><td>' + esc(e.action || '') + '</td><td>' + esc(e.item || '') +
          '</td><td class="num">' + (e.qty || 1) + '</td><td class="dim">' + where +
          (e.notes ? ' <span class="dim">' + esc(e.notes) + '</span>' : '') +
          '</td></tr>';
      }).join('') + '</tbody></table>';
  }

  function renderInvHistory() {
    var el = document.getElementById('inv-history');
    if (!el) { return; }
    if (!INV.state) {
      el.innerHTML = '<div class="dt-empty">' +
        (INV.error ? esc(INV.error) : 'Loading&hellip;') + '</div>';
      return;
    }
    el.innerHTML =
      '<div class="nd-meta" style="margin-bottom:6px">Kept in the ' +
        '<b>Inventory History</b> section of ' + esc(INV.state.name || 'your character sheet') +
        ', not in the session log.</div>' +
      invHistoryRows(INV.state.history);
  }

  // ------------------------------------------------------------- building
  function buildInventoryUi() {
    invBox = document.getElementById('inventory');
    if (!invBox) { return; }
    invLive = invBox.querySelector('#inv-live');
    invEnc = invBox.querySelector('#inv-enc');

    // The sheet was exported with the bag it had at the time, so the panel is
    // live before the server has said anything - and stays usable if the
    // server never answers.
    var seed = document.getElementById('dnd-inventory-data');
    if (seed) {
      try {
        var inv = JSON.parse(seed.textContent || 'null');
        if (inv) { INV.state = { inventory: inv, history: null }; }
      } catch (e) { /* an unreadable blob just means we wait for the server */ }
    }

    invAdd = document.createElement('div');
    invAdd.className = 'inv-modal';
    invAdd.id = 'inv-add';
    invAdd.innerHTML =
      '<div class="inv-card" role="dialog" aria-label="Add an item">' +
        '<h3>Add to your inventory' +
          '<button type="button" class="x" id="inv-add-close" aria-label="Close">&times;</button></h3>' +
        '<input type="search" class="inv-field" id="inv-q" autocomplete="off" ' +
          'placeholder="Search the rules: healing potion, rope, bkpk...">' +
        '<div class="inv-filters" id="inv-filters"></div>' +
        '<div class="inv-results" id="inv-results"></div>' +
        '<div class="inv-row">' +
          '<label for="inv-qty">How many</label>' +
          '<input type="number" class="inv-field qty-field" id="inv-qty" min="1" value="1">' +
          '<label for="inv-where">Into</label>' +
          '<select class="inv-field" id="inv-where"></select>' +
          '<button type="button" class="inv-go" id="inv-add-go">Add</button>' +
        '</div>' +
        '<div class="inv-hint" id="inv-picked"></div>' +
        '<div class="inv-hint">Click or use up and down to pick a match, enter or ' +
          'Add to take it. Anything the rules do not know is added under the ' +
          'name you type.</div>' +
        '<div class="inv-err" id="inv-err"></div>' +
      '</div>';
    document.body.appendChild(invAdd);

    invMove = document.createElement('div');
    invMove.className = 'inv-modal';
    invMove.id = 'inv-move';
    invMove.innerHTML =
      '<div class="inv-card" role="dialog" aria-label="Move or drop an item">' +
        '<h3 id="inv-move-title">Move' +
          '<button type="button" class="x" id="inv-move-close" aria-label="Close">&times;</button></h3>' +
        '<div class="inv-row">' +
          '<label for="inv-move-qty">How many</label>' +
          '<input type="number" class="inv-field qty-field" id="inv-move-qty" min="1" value="1">' +
          '<span class="inv-msg" id="inv-move-max"></span>' +
        '</div>' +
        '<div class="inv-row" id="inv-move-dest">' +
          '<label for="inv-move-where">Into</label>' +
          '<select class="inv-field" id="inv-move-where"></select>' +
        '</div>' +
        '<div class="inv-row">' +
          '<button type="button" class="inv-go" id="inv-move-go">Move</button>' +
        '</div>' +
        '<div class="inv-err" id="inv-move-err"></div>' +
      '</div>';
    document.body.appendChild(invMove);

    // ---- events
    invBox.addEventListener('click', function (e) {
      var tab = e.target.closest('.inv-tab');
      if (tab) {
        INV.where = tab.getAttribute('data-tab') || '';
        INV.msg = '';
        renderInventory();
        return;
      }
      if (e.target.closest('#inv-add-btn')) { openAdd(); return; }
      var btn = e.target.closest('.ib, .wearbtn');
      if (!btn) { return; }
      var row = btn.closest('tr');
      var pane = btn.closest('.inv-pane');
      if (!row || !pane) { return; }
      var where = pane.getAttribute('data-pane') || '';
      var box = containerOf(INV.state.inventory, where);
      var key = row.getAttribute('data-item');
      var entry = null;
      (box ? box.entries : []).forEach(function (x) { if (x.key === key) { entry = x; } });
      if (!entry) { return; }
      // The button says which way it goes rather than the sheet flipping what
      // it thinks it knows, so a stale row cannot toggle the wrong way.
      if (btn.classList.contains('wearbtn')) {
        invChange({ action: 'equip', item: entry.key, name: entry.name,
                    container: where,
                    equipped: btn.getAttribute('data-wear') === '1' });
        return;
      }
      var act = btn.getAttribute('data-act');
      if (act === 'use') {
        invChange({ action: 'use', item: entry.key, name: entry.name, qty: 1,
                    container: where });
        return;
      }
      openMove(act === 'move' ? 'move' : 'drop', entry, where);
    });

    // A click picks a match rather than adding it outright: an item added by
    // a stray click has to be found and dropped again, which is a worse
    // mistake than one more click. Enter, Add, or a double click adds it.
    invAdd.addEventListener('click', function (e) {
      if (e.target === invAdd || e.target.closest('#inv-add-close')) { closeAdd(); return; }
      if (e.target.closest('#inv-add-go')) { doAdd(); return; }
      var chip = e.target.closest('.inv-filter');
      if (chip) {
        invFilter = chip.getAttribute('data-filter') || 'all';
        renderInvFilters();
        invSearch(invAdd.querySelector('#inv-q').value);
        return;
      }
      var hit = e.target.closest('.inv-hit');
      if (hit) { invPickHit(parseInt(hit.getAttribute('data-hit'), 10)); }
    });
    invAdd.addEventListener('dblclick', function (e) {
      var hit = e.target.closest('.inv-hit');
      if (hit) {
        invPickHit(parseInt(hit.getAttribute('data-hit'), 10));
        doAdd();
      }
    });
    invAdd.querySelector('#inv-q').addEventListener('input', function (e) {
      var q = e.target.value;
      invHit = -1;
      invShowPick();
      if (invSearchTimer) { clearTimeout(invSearchTimer); }
      invSearchTimer = setTimeout(function () { invSearch(q); }, 140);
    });
    invAdd.addEventListener('keydown', function (e) {
      if (e.key === 'Escape') { closeAdd(); return; }
      if (e.key === 'ArrowDown') { e.preventDefault(); invPickHit(invHit + 1); return; }
      if (e.key === 'ArrowUp') { e.preventDefault(); invPickHit(invHit - 1); return; }
      if (e.key === 'Enter') { e.preventDefault(); doAdd(); }
    });

    invMove.addEventListener('click', function (e) {
      if (e.target === invMove || e.target.closest('#inv-move-close')) { closeMove(); return; }
      if (e.target.closest('#inv-move-go')) { doMove(); }
    });
    invMove.addEventListener('keydown', function (e) {
      if (e.key === 'Escape') { closeMove(); return; }
      if (e.key === 'Enter') { e.preventDefault(); doMove(); }
    });

    renderInventory();
    // The file on disk is the truth; the page catches up with it on load.
    invLoad(true);
  }

  // --------------------------------------------------------------- coin
  //
  // The purse. Coin is spent, earned and changed on the server against the org
  // character sheet, exactly the way the inventory is, so the file stays the
  // one account of what a character can actually afford and two people looking
  // at the same character see the same gold.
  //
  // Making change is the server's business, not the page's: a two copper
  // candle paid for out of a single gold piece comes back as nine silver and
  // eight copper, and all this does is draw the answer.
  var COIN = { state: null, busy: false, error: '', msg: '', amount: '', why: '' };
  var coinBox, coinLive, coinSetBox, coinSwapBox;

  // The denominations largest first, which is how a purse is read out.
  var COIN_ORDER = ['pp', 'gp', 'ep', 'sp', 'cp'];

  function coinConfigured() { return !!(LOG.file || CHARACTER_ID); }

  function coinWho(body) {
    body = body || {};
    body.filename = LOG.file || '';
    body.id = CHARACTER_ID || '';
    return body;
  }

  function coinPurse() { return (COIN.state && COIN.state.purse) || null; }

  function coinLoad(quiet) {
    if (!coinConfigured()) { return Promise.resolve(); }
    var q = '/dnd/money?filename=' + encodeURIComponent(LOG.file || '') +
      '&id=' + encodeURIComponent(CHARACTER_ID || '');
    return api('GET', q).then(function (state) {
      COIN.state = state;
      COIN.error = '';
      renderCoins();
    }, function (err) {
      if (!quiet) { COIN.error = err.message || String(err); renderCoins(); }
    });
  }

  // coinChange posts one change and redraws from what comes back, so the panel
  // always shows what is actually written in the file. A refused payment
  // leaves the purse alone, so nothing needs undoing here.
  function coinChange(body) {
    if (!coinConfigured()) {
      COIN.error = 'this sheet does not know which org file it came from';
      renderCoins();
      return Promise.reject(new Error(COIN.error));
    }
    COIN.busy = true;
    COIN.error = '';
    renderCoins();
    return api('POST', '/dnd/money', coinWho(body)).then(function (state) {
      COIN.busy = false;
      COIN.state = state;
      COIN.msg = state.msg || '';
      COIN.amount = '';
      COIN.why = '';
      renderCoins();
      return state;
    }, function (err) {
      COIN.busy = false;
      COIN.error = err.message || String(err);
      renderCoins();
      throw err;
    });
  }

  // moneyStr says an amount the way the sheet writes it, largest coin first.
  function moneyStr(m) {
    if (!m) { return '0 gp'; }
    var parts = [];
    COIN_ORDER.forEach(function (id) {
      var n = m[id] || 0;
      if (n) { parts.push(n + ' ' + id); }
    });
    return parts.length ? parts.join(' ') : '0 gp';
  }

  function coinRow(c) {
    return '<tr class="' + (c.qty ? '' : 'empty') + '">' +
      '<td><span class="coin-pip ' + esc(c.id) + '"></span>' + esc(c.name) +
        ' <span class="tagline">' + esc(c.abbr) + '</span></td>' +
      '<td class="num">' + (c.qty || 0) + '</td>' +
      '<td class="num dim">' + (c.qty ? weightStr(c.gold) + ' gp' : '&mdash;') + '</td>' +
      '</tr>';
  }

  function coinWhat(e) {
    if (e.notes) { return e.notes; }
    switch (e.action) {
      case 'spent': return 'spent';
      case 'gained': return 'picked up';
      case 'consolidated': return 'changed up';
      case 'exchanged': return 'exchanged';
      case 'set': return 'purse counted';
    }
    return e.action || 'changed';
  }

  function coinSign(e) {
    if (e.action === 'spent') { return '−'; }
    if (e.action === 'gained') { return '+'; }
    return '';
  }

  // The last few lines of the character file's Coin History, newest first.
  function coinLogHtml() {
    var list = (COIN.state && COIN.state.history) || [];
    if (!list.length) { return ''; }
    var rows = list.slice(-6).reverse().map(function (e) {
      return '<li class="' + esc(e.action || '') + '">' +
        '<span class="when">' + esc(e.date || '') + '</span>' +
        '<span class="what">' + esc(coinWhat(e)) + '</span>' +
        '<span class="amt">' + esc(coinSign(e) + moneyStr(e.amount)) + '</span>' +
        '</li>';
    }).join('');
    return '<div class="coin-log"><h4>Lately</h4><ul>' + rows + '</ul>' +
      '<div class="coin-hint">All of it is kept in the <b>Coin History</b> ' +
      'section of your character file.</div></div>';
  }

  function renderCoins() {
    if (!coinLive) { return; }
    var p = coinPurse();
    if (!p) {
      coinLive.innerHTML = '<div class="inv-empty">' +
        (COIN.error ? esc(COIN.error) : 'Counting your coin…') + '</div>';
      return;
    }
    coinLive.innerHTML =
      '<div class="coin-head">' +
        '<span class="coin-total">' + esc(p.total || '0 gp') + '</span>' +
        '<span class="coin-sub">' + (p.count || 0) + ' coin' +
          (p.count === 1 ? '' : 's') + ' &middot; ' + weightStr(p.weight) + ' lb</span>' +
      '</div>' +
      '<div class="table-wrap"><table class="coin-table">' +
        '<thead><tr><th>Coin</th><th class="num">Held</th><th class="num">Worth</th></tr></thead>' +
        '<tbody>' + (p.coins || []).map(coinRow).join('') + '</tbody>' +
      '</table></div>' +
      '<div class="coin-spend">' +
        '<input type="text" class="inv-field amount" id="coin-amount" autocomplete="off" ' +
          'placeholder="15 gp 3 sp" aria-label="How much" value="' + esc(COIN.amount) + '">' +
        '<input type="text" class="inv-field why" id="coin-why" autocomplete="off" ' +
          'placeholder="what for" aria-label="What for" value="' + esc(COIN.why) + '">' +
        '<button type="button" class="ib drop" data-coin="spend">Spend</button>' +
        '<button type="button" class="ib use" data-coin="gain">Gain</button>' +
      '</div>' +
      '<div class="coin-spend">' +
        '<button type="button" class="ib" data-coin="consolidate" ' +
          'title="Change small coin up into the largest that hold the same value">Change up</button>' +
        '<button type="button" class="ib" data-coin="swap">Exchange&hellip;</button>' +
        '<button type="button" class="ib" data-coin="set">Count purse&hellip;</button>' +
        (COIN.busy ? '<span class="inv-msg">saving&hellip;</span>'
                   : (COIN.msg ? '<span class="inv-msg">' + esc(COIN.msg) + '</span>' : '')) +
        (COIN.error ? '<span class="inv-err">' + esc(COIN.error) + '</span>' : '') +
      '</div>' +
      '<div class="coin-hint">Say an amount however you like: <b>15 gp</b>, ' +
        '<b>3 gp 4 sp</b>, or a bare number for gold. Paying with a coin that is ' +
        'too big is fine - the change comes back.</div>' +
      coinLogHtml();
  }

  function coinFocusAmount() {
    var el = coinBox && coinBox.querySelector('#coin-amount');
    if (el) { el.focus(); }
  }

  // ------------------------------------------------------- counting the purse
  function coinSetOptions() {
    var p = coinPurse();
    var held = (p && p.money) || {};
    return COIN_ORDER.map(function (id) {
      return '<label for="coin-set-' + id + '">' + id +
        '<input type="number" class="inv-field" id="coin-set-' + id + '" ' +
        'data-coin-field="' + id + '" min="0" value="' + (held[id] || 0) + '"></label>';
    }).join('');
  }

  function openCoinSet() {
    if (!coinSetBox) { return; }
    coinSetBox.querySelector('#coin-set-grid').innerHTML = coinSetOptions();
    coinSetBox.querySelector('#coin-set-err').textContent = '';
    coinSetBox.classList.add('open');
    var first = coinSetBox.querySelector('input');
    if (first) { first.focus(); first.select(); }
  }

  function doCoinSet() {
    var money = {};
    Array.prototype.forEach.call(
      coinSetBox.querySelectorAll('[data-coin-field]'), function (el) {
        money[el.getAttribute('data-coin-field')] = Math.max(0, parseInt(el.value, 10) || 0);
      });
    coinChange({ action: 'set', money: money, notes: 'counted' }).then(function () {
      coinSetBox.classList.remove('open');
    }, function (err) {
      coinSetBox.querySelector('#coin-set-err').textContent = err.message || String(err);
    });
  }

  // ------------------------------------------------------------- exchanging
  function coinSwapOptions(selected) {
    var p = coinPurse();
    var held = (p && p.money) || {};
    return COIN_ORDER.map(function (id) {
      return '<option value="' + id + '"' + (id === selected ? ' selected' : '') + '>' +
        id + (held[id] ? ' (' + held[id] + ')' : '') + '</option>';
    }).join('');
  }

  function openCoinSwap() {
    if (!coinSwapBox) { return; }
    coinSwapBox.querySelector('#coin-swap-from').innerHTML = coinSwapOptions('sp');
    coinSwapBox.querySelector('#coin-swap-to').innerHTML = coinSwapOptions('gp');
    coinSwapBox.querySelector('#coin-swap-err').textContent = '';
    coinSwapBox.classList.add('open');
    coinSwapBox.querySelector('#coin-swap-qty').focus();
  }

  function doCoinSwap() {
    var qty = parseInt(coinSwapBox.querySelector('#coin-swap-qty').value, 10) || 0;
    coinChange({
      action: 'exchange',
      from: coinSwapBox.querySelector('#coin-swap-from').value,
      to: coinSwapBox.querySelector('#coin-swap-to').value,
      qty: qty
    }).then(function () {
      coinSwapBox.classList.remove('open');
    }, function (err) {
      coinSwapBox.querySelector('#coin-swap-err').textContent = err.message || String(err);
    });
  }

  // --------------------------------------------------------------- building
  function buildCoinUi() {
    coinBox = document.getElementById('coins');
    if (!coinBox) { return; }
    coinLive = coinBox.querySelector('#coin-live');

    // The sheet was exported with the purse it had at the time, so the panel
    // is live before the server has said anything - and stays readable if the
    // server never answers.
    var seed = document.getElementById('dnd-money-data');
    if (seed) {
      try {
        var money = JSON.parse(seed.textContent || 'null');
        if (money && money.purse) {
          COIN.state = { purse: money.purse, history: money.history || [] };
        }
      } catch (e) { /* an unreadable blob just means we wait for the server */ }
    }

    coinSetBox = document.createElement('div');
    coinSetBox.className = 'inv-modal';
    coinSetBox.id = 'coin-set';
    coinSetBox.innerHTML =
      '<div class="inv-card" role="dialog" aria-label="Count your purse">' +
        '<h3>Count your purse' +
          '<button type="button" class="x" data-coin-close="1" aria-label="Close">&times;</button></h3>' +
        '<div class="coin-set-grid" id="coin-set-grid"></div>' +
        '<div class="inv-row">' +
          '<button type="button" class="inv-go" id="coin-set-go">Set</button>' +
        '</div>' +
        '<div class="inv-hint">Says outright what is in the purse, for when it has ' +
          'drifted from what is on the table. It is written into your character ' +
          'file like any other change.</div>' +
        '<div class="inv-err" id="coin-set-err"></div>' +
      '</div>';
    document.body.appendChild(coinSetBox);

    coinSwapBox = document.createElement('div');
    coinSwapBox.className = 'inv-modal';
    coinSwapBox.id = 'coin-swap';
    coinSwapBox.innerHTML =
      '<div class="inv-card" role="dialog" aria-label="Exchange coin">' +
        '<h3>Exchange coin' +
          '<button type="button" class="x" data-coin-close="1" aria-label="Close">&times;</button></h3>' +
        '<div class="inv-row">' +
          '<label for="coin-swap-qty">Change</label>' +
          '<input type="number" class="inv-field qty-field" id="coin-swap-qty" min="1" value="10">' +
          '<select class="inv-field" id="coin-swap-from" aria-label="Coin to change"></select>' +
          '<label for="coin-swap-to">for</label>' +
          '<select class="inv-field" id="coin-swap-to" aria-label="Coin wanted back"></select>' +
          '<button type="button" class="inv-go" id="coin-swap-go">Exchange</button>' +
        '</div>' +
        '<div class="inv-hint">At the usual rate: 10 cp to the silver, 10 sp or 2 ep ' +
          'to the gold, 10 gp to the platinum. A swap that will not come out even is ' +
          'refused rather than rounded.</div>' +
        '<div class="inv-err" id="coin-swap-err"></div>' +
      '</div>';
    document.body.appendChild(coinSwapBox);

    // ---- events
    coinBox.addEventListener('input', function (e) {
      if (e.target.id === 'coin-amount') { COIN.amount = e.target.value; }
      if (e.target.id === 'coin-why') { COIN.why = e.target.value; }
    });
    coinBox.addEventListener('keydown', function (e) {
      if (e.key !== 'Enter') { return; }
      if (e.target.id !== 'coin-amount' && e.target.id !== 'coin-why') { return; }
      e.preventDefault();
      coinSpend('spend');
    });
    coinBox.addEventListener('click', function (e) {
      var btn = e.target.closest('[data-coin]');
      if (!btn) { return; }
      var act = btn.getAttribute('data-coin');
      if (act === 'set') { openCoinSet(); return; }
      if (act === 'swap') { openCoinSwap(); return; }
      if (act === 'consolidate') {
        coinChange({ action: 'consolidate' }).catch(function () {});
        return;
      }
      coinSpend(act);
    });

    [coinSetBox, coinSwapBox].forEach(function (box) {
      box.addEventListener('click', function (e) {
        if (e.target === box || e.target.closest('[data-coin-close]')) {
          box.classList.remove('open');
          return;
        }
        if (e.target.closest('#coin-set-go')) { doCoinSet(); }
        if (e.target.closest('#coin-swap-go')) { doCoinSwap(); }
      });
      box.addEventListener('keydown', function (e) {
        if (e.key === 'Escape') { box.classList.remove('open'); return; }
        if (e.key !== 'Enter') { return; }
        e.preventDefault();
        if (box === coinSetBox) { doCoinSet(); } else { doCoinSwap(); }
      });
    });

    renderCoins();
    // The file on disk is the truth; the page catches up with it on load.
    coinLoad(true);
  }

  // coinSpend is Spend and Gain, which differ only in which way the coin goes.
  function coinSpend(action) {
    var amount = (COIN.amount || '').trim();
    if (!amount) {
      COIN.error = 'how much?';
      COIN.msg = '';
      renderCoins();
      coinFocusAmount();
      return;
    }
    coinChange({ action: action, amount: amount, notes: (COIN.why || '').trim() })
      .then(coinFocusAmount, coinFocusAmount);
  }

  // --------------------------------------------------- defenses & conditions
  //
  // What the character shrugs off, and what is currently wrong with them.
  // Neither can be worked out from the rules - a resistance may come from a
  // race, a spell that is running or a cloak being worn, and a condition is
  // something that happened at the table - so both are stored on the character
  // and written straight into the org file, the same way coin and inventory
  // are. Two people looking at the same character see the same state, and it
  // is still there next session.
  var DEF = { state: null, busy: false, error: '', msg: '', pick: '' };
  var defBox, defLive, condModal, defAddBox;

  // One mark per condition. They are line drawings in the same hand as the
  // dice and quill marks elsewhere on the sheet, so a condition is recognised
  // rather than read - which is the whole point of a picker you tap.
  var COND_ICONS = {
    blinded: '<path d="M2 12s4-6 10-6 10 6 10 6-4 6-10 6-10-6-10-6z"/>' +
      '<circle cx="12" cy="12" r="2.6"/><path d="M3.5 3.5l17 17"/>',
    charmed: '<path d="M12 20.4S3.6 14.9 3.6 8.9A4.4 4.4 0 0 1 12 7a4.4 4.4 0 0 1 8.4 1.9' +
      'c0 6-8.4 11.5-8.4 11.5z"/>',
    deafened: '<path d="M7 9.4a5 5 0 0 1 10 0c0 3-3 3.6-3 6a2.4 2.4 0 0 1-4.6.9"/>' +
      '<path d="M3.5 3.5l17 17"/>',
    exhaustion: '<path d="M6 3h12M6 21h12"/>' +
      '<path d="M8.4 3v3.4c0 2 3.6 3.5 3.6 5.6 0-2.1 3.6-3.6 3.6-5.6V3"/>' +
      '<path d="M8.4 21v-3.4c0-2 3.6-3.5 3.6-5.6 0 2.1 3.6 3.6 3.6 5.6V21"/>',
    frightened: '<circle cx="12" cy="12" r="9"/><circle cx="9" cy="10.2" r="1.1"/>' +
      '<circle cx="15" cy="10.2" r="1.1"/>' +
      '<path d="M8.4 17.2c1-1.7 2.2-2.5 3.6-2.5s2.6.8 3.6 2.5"/>',
    grappled: '<path d="M9 11.4V5.8a1.6 1.6 0 0 1 3.2 0v5.2"/>' +
      '<path d="M12.2 10.6V4.6a1.6 1.6 0 0 1 3.2 0v6.2"/>' +
      '<path d="M15.4 11V7.4a1.6 1.6 0 0 1 3.2 0v7c0 3.6-2.6 6.6-6.4 6.6-3.2 0-4.8-1.6-6.4-4.2' +
      'L4 14.4a1.7 1.7 0 0 1 2.8-1.9L9 15.2"/>',
    incapacitated: '<circle cx="12" cy="12" r="9"/><path d="M5.6 5.6l12.8 12.8"/>',
    invisible: '<path d="M2.4 12s4-6 9.6-6 9.6 6 9.6 6-4 6-9.6 6-9.6-6-9.6-6z" ' +
      'stroke-dasharray="3.4 3"/><circle cx="12" cy="12" r="2.4"/>',
    paralyzed: '<path d="M13.2 2.4 5.4 13.2h5.6l-1.6 8.4 8.8-11.6h-5.8z"/>',
    petrified: '<path d="M4.2 14.8 7.8 6.4 14 4l6 5.6L18.4 19 8 20.2z"/>' +
      '<path d="M7.8 6.4 14 4M8 20.2l1-6.2 5.4-3.4L20 9.6"/>',
    poisoned: '<path d="M9.6 3h4.8M11 3v6.2L5.9 18a2.4 2.4 0 0 0 2.1 3.6h8a2.4 2.4 0 0 0 2.1-3.6' +
      'L13 9.2V3"/><circle cx="10.8" cy="16.4" r=".9"/><circle cx="14" cy="18.6" r="1.2"/>',
    prone: '<path d="M12 3v10.6"/><path d="M7.8 9.6 12 13.8l4.2-4.2"/><path d="M4 19.8h16"/>',
    restrained: '<path d="M9.4 14.6 14.6 9.4"/>' +
      '<path d="M7.8 12.2 6 14a3.5 3.5 0 0 0 5 5l1.8-1.8"/>' +
      '<path d="M16.2 11.8 18 10a3.5 3.5 0 0 0-5-5l-1.8 1.8"/>',
    stunned: '<path d="M12 15a3 3 0 1 0-3-3 5 5 0 1 0 5 5 7 7 0 1 1-7-7"/>',
    unconscious: '<path d="M3 8.6h5.6L3 15.4h5.6"/><path d="M11.4 3.8h4.8l-4.8 5.6h4.8"/>' +
      '<path d="M13.6 15h5.8l-5.8 6h5.8"/>',
    // Anything homebrew the ruleset does not carry a drawing for.
    unknown: '<circle cx="12" cy="12" r="9"/><path d="M12 7.6v5.6"/>' +
      '<circle cx="12" cy="16.6" r=".7"/>'
  };

  function condIcon(id, cls) {
    var d = COND_ICONS[id] || COND_ICONS.unknown;
    return '<svg class="ico ' + (cls || '') + '" viewBox="0 0 24 24" ' +
      'aria-hidden="true" focusable="false">' + d + '</svg>';
  }

  function defConfigured() { return !!(LOG.file || CHARACTER_ID); }

  function defWho(body) {
    body = body || {};
    body.filename = LOG.file || '';
    body.id = CHARACTER_ID || '';
    return body;
  }

  function defLoad(quiet) {
    if (!defConfigured()) { return Promise.resolve(); }
    var q = '/dnd/conditions?filename=' + encodeURIComponent(LOG.file || '') +
      '&id=' + encodeURIComponent(CHARACTER_ID || '');
    return api('GET', q).then(function (state) {
      DEF.state = state;
      DEF.error = '';
      renderDefenses();
    }, function (err) {
      if (!quiet) { DEF.error = err.message || String(err); renderDefenses(); }
    });
  }

  // One change, then redraw from what came back, so the panel always shows
  // what is actually written in the file. A refused change leaves the
  // character alone and there is nothing to undo.
  function defChange(body) {
    if (!defConfigured()) {
      DEF.error = 'this sheet does not know which org file it came from';
      renderDefenses();
      return Promise.reject(new Error(DEF.error));
    }
    DEF.busy = true;
    DEF.error = '';
    renderDefenses();
    return api('POST', '/dnd/conditions', defWho(body)).then(function (state) {
      DEF.busy = false;
      DEF.state = state;
      DEF.msg = state.msg || '';
      renderDefenses();
      // The night's log gets a line, the way a rest does: what happened to
      // this character is part of what happened at the table.
      logNote(defLine(body, state));
      return state;
    }, function (err) {
      DEF.busy = false;
      DEF.error = err.message || String(err);
      renderDefenses();
      throw err;
    });
  }

  // The line the session log gets. The server says what it did; this only
  // decides which of the two headings it belongs under.
  function defLine(body, state) {
    if (!state.msg) { return ''; }
    var act = body.action || '';
    var head = (act === 'resist' || act === 'immune' || act === 'vulnerable' ||
                act === 'unprotect') ? '*Defenses.*' : '*Condition.*';
    var msg = state.msg;
    return head + ' ' + msg.charAt(0).toUpperCase() + msg.slice(1) + '.';
  }

  function defState() { return DEF.state || {}; }
  function defConditions() { return defState().conditions || {}; }
  function defDefenses() { return defState().defenses || {}; }

  // ------------------------------------------------------- drawing the panel
  function defChipHtml(d, kind) {
    return '<span class="def-chip ' + kind + '">' + esc(d.name || d.id) +
      '<button type="button" class="x" data-drop="' + esc(d.name || d.id) +
      '" aria-label="Drop ' + esc(d.name || d.id) + '">&times;</button></span>';
  }

  function defRowHtml(label, kind, list) {
    if (!list || !list.length) { return ''; }
    return '<div class="def-row"><span class="def-kind ' + kind + '">' + label +
      '</span>' + list.map(function (d) { return defChipHtml(d, kind); }).join('') +
      '</div>';
  }

  function condOnHtml(c) {
    var step = '';
    if (c.levels) {
      step = '<span class="lvl">' +
        '<button type="button" class="cond-step" data-lvl="' + esc(c.id) + ':' +
          (c.level - 1) + '" aria-label="Ease ' + esc(c.name) + '">&minus;</button>' +
        '<button type="button" class="cond-step" data-lvl="' + esc(c.id) + ':' +
          (c.level + 1) + '" aria-label="Worsen ' + esc(c.name) + '"' +
          (c.level >= c.levels ? ' disabled' : '') + '>+</button></span>';
    }
    return '<div class="cond-on"><div class="cond-head">' +
      condIcon(c.icon || c.id) + '<b>' + esc(c.label || c.name) + '</b>' +
      (c.note ? '<span class="tagline">' + esc(c.note) + '</span>' : '') + step +
      '<button type="button" class="x" data-off="' + esc(c.id) +
        '" aria-label="End ' + esc(c.name) + '">&times;</button></div>' +
      (c.text ? '<p>' + esc(c.text) + '</p>' : '') + '</div>';
  }

  function renderDefenses() {
    if (!defLive) { return; }
    var d = defDefenses();
    var c = defConditions();
    var rows = defRowHtml('Resistant', 'res', d.resistances) +
      defRowHtml('Immune', 'imm', d.immunities) +
      defRowHtml('Vulnerable', 'vul', d.vulnerabilities);
    var active = (c.active || []).map(condOnHtml).join('');

    defLive.innerHTML =
      '<h3 class="subhead">Resistances, Immunities &amp; Vulnerabilities</h3>' +
      (rows ? '<div class="def-rows">' + rows + '</div>'
            : '<p class="def-empty">Nothing yet. You take every kind of damage ' +
              'the way it comes.</p>') +
      '<div class="def-foot">' +
        '<button type="button" class="inv-add" data-def="add">Add a defense&hellip;</button>' +
      '</div>' +
      '<h3 class="subhead" style="margin-top:12px">Conditions</h3>' +
      (active ? '<div class="cond-active">' + active + '</div>'
              : '<p class="def-empty">Nothing is on you.</p>') +
      '<div class="def-foot">' +
        '<button type="button" class="inv-add" data-def="pick">Conditions&hellip;</button>' +
        ((c.active || []).length > 1
          ? '<button type="button" class="ib" data-def="clear">Clear all</button>' : '') +
        (DEF.busy ? '<span class="inv-msg">saving&hellip;</span>'
                  : (DEF.msg ? '<span class="inv-msg">' + esc(DEF.msg) + '</span>' : '')) +
        (DEF.error ? '<span class="inv-err">' + esc(DEF.error) + '</span>' : '') +
      '</div>' +
      '<div class="coin-hint">All of it is kept in your character file: the ' +
        'defenses in its property drawer, and everything that has come and gone ' +
        'in its <b>Condition History</b> section.</div>';
  }

  // ------------------------------------------------------------- the picker
  function condPickHtml(c) {
    return '<button type="button" class="cond-pick' + (c.on ? ' on' : '') +
      '" data-cond="' + esc(c.id) + '" aria-pressed="' + (c.on ? 'true' : 'false') +
      '" title="' + esc(c.text || '') + '">' +
      condIcon(c.icon || c.id) + '<span>' + esc(c.name) + '</span>' +
      (c.on && c.levels ? '<span class="lv">level ' + c.level + '</span>' : '') +
      '</button>';
  }

  function renderCondPicker() {
    if (!condModal) { return; }
    var all = defConditions().all || [];
    condModal.querySelector('#cond-grid').innerHTML = all.map(condPickHtml).join('');
    var why = condModal.querySelector('#cond-why');
    var found = null;
    all.forEach(function (c) { if (c.id === DEF.pick) { found = c; } });
    why.innerHTML = found
      ? '<b>' + esc(found.name) + '.</b> ' + esc(found.text || '') +
        (found.levels && found.on && found.note
          ? ' <i>Level ' + found.level + ': ' + esc(found.note) + '.</i>' : '')
      : 'Tap a condition to put it on or take it off. Exhaustion arrives at ' +
        'level 1; step it up and down from the panel behind this.';
    condModal.querySelector('#cond-err').textContent = DEF.error || '';
  }

  function openConditions() {
    if (!condModal) { return; }
    DEF.pick = '';
    renderCondPicker();
    condModal.classList.add('open');
  }

  // The kinds of damage come from the ruleset rather than from a list kept
  // here, so a module that adds one is offered it without touching the sheet.
  // Every condition is offered too: "immune to being frightened" is a defense.
  function fillDamageTypes() {
    var list = defAddBox && defAddBox.querySelector('#def-types');
    if (!list) { return; }
    var names = (defState().damageTypes || []).map(function (d) { return d.name; })
      .concat((defConditions().all || []).map(function (c) { return c.name; }));
    list.innerHTML = names.map(function (n) {
      return '<option value="' + esc(n) + '">';
    }).join('');
  }

  function openDefenseAdd() {
    fillDamageTypes();
    if (!defAddBox) { return; }
    defAddBox.querySelector('#def-what').value = '';
    defAddBox.querySelector('#def-err').textContent = '';
    defAddBox.classList.add('open');
    defAddBox.querySelector('#def-what').focus();
  }

  function doDefenseAdd(kind) {
    var what = (defAddBox.querySelector('#def-what').value || '').trim();
    if (!what) {
      defAddBox.querySelector('#def-err').textContent = 'against what?';
      return;
    }
    defChange({ action: kind, name: what }).then(function () {
      defAddBox.classList.remove('open');
    }, function (err) {
      defAddBox.querySelector('#def-err').textContent = err.message || String(err);
    });
  }

  // --------------------------------------------------------------- building
  function buildDefenseUi() {
    defBox = document.getElementById('defenses');
    if (!defBox) { return; }
    defLive = defBox.querySelector('#def-live');

    // The sheet was exported with whatever state the character was in, so the
    // panel is live before the server has said anything - and stays readable
    // if the server never answers.
    var seed = document.getElementById('dnd-defenses-data');
    if (seed) {
      try {
        var got = JSON.parse(seed.textContent || 'null');
        if (got && got.conditions) { DEF.state = got; }
      } catch (e) { /* an unreadable blob just means we wait for the server */ }
    }

    condModal = document.createElement('div');
    condModal.className = 'inv-modal';
    condModal.id = 'cond-picker';
    condModal.innerHTML =
      '<div class="inv-card" role="dialog" aria-label="Conditions">' +
        '<h3>Conditions' +
          '<button type="button" class="x" data-def-close="1" aria-label="Close">' +
          '&times;</button></h3>' +
        '<div class="cond-grid" id="cond-grid"></div>' +
        '<div class="cond-why" id="cond-why"></div>' +
        '<div class="inv-err" id="cond-err"></div>' +
      '</div>';
    document.body.appendChild(condModal);

    defAddBox = document.createElement('div');
    defAddBox.className = 'inv-modal';
    defAddBox.id = 'def-add';
    defAddBox.innerHTML =
      '<div class="inv-card" role="dialog" aria-label="Add a defense">' +
        '<h3>Add a defense' +
          '<button type="button" class="x" data-def-close="1" aria-label="Close">' +
          '&times;</button></h3>' +
        '<div class="inv-row">' +
          '<input type="text" class="inv-field" id="def-what" autocomplete="off" ' +
            'list="def-types" placeholder="fire" aria-label="Against what">' +
          '<datalist id="def-types"></datalist>' +
        '</div>' +
        '<div class="inv-row">' +
          '<button type="button" class="inv-go" data-add="resist">Resistant</button>' +
          '<button type="button" class="inv-go" data-add="immune">Immune</button>' +
          '<button type="button" class="inv-go" data-add="vulnerable">Vulnerable</button>' +
        '</div>' +
        '<div class="inv-hint">A kind of damage - <b>fire</b>, <b>necrotic</b>, ' +
          '<b>bludgeoning</b> - or a condition you cannot be put under, or ' +
          'anything else you want written down, such as ' +
          '<b>bludgeoning from nonmagical attacks</b>. One kind of damage sits ' +
          'in one of the three lists at a time.</div>' +
        '<div class="inv-err" id="def-err"></div>' +
      '</div>';
    document.body.appendChild(defAddBox);

    // ---- events
    defBox.addEventListener('click', function (e) {
      var act = e.target.closest('[data-def]');
      if (act) {
        var what = act.getAttribute('data-def');
        if (what === 'pick') { openConditions(); }
        else if (what === 'add') { openDefenseAdd(); }
        else if (what === 'clear') { defChange({ action: 'clear' }).catch(noop); }
        return;
      }
      var off = e.target.closest('[data-off]');
      if (off) {
        defChange({ action: 'remove', name: off.getAttribute('data-off') }).catch(noop);
        return;
      }
      var lvl = e.target.closest('[data-lvl]');
      if (lvl) {
        var bits = lvl.getAttribute('data-lvl').split(':');
        defChange({ action: 'level', name: bits[0],
                    level: parseInt(bits[1], 10) || 0 }).catch(noop);
        return;
      }
      var drop = e.target.closest('[data-drop]');
      if (drop) {
        defChange({ action: 'unprotect', name: drop.getAttribute('data-drop') })
          .catch(noop);
      }
    });

    condModal.addEventListener('click', function (e) {
      if (e.target === condModal || e.target.closest('[data-def-close]')) {
        condModal.classList.remove('open');
        return;
      }
      var pick = e.target.closest('[data-cond]');
      if (!pick) { return; }
      DEF.pick = pick.getAttribute('data-cond');
      // The picker stays open: putting three conditions on at once is one
      // thing that happens, and closing after each would be three trips.
      defChange({ action: 'toggle', name: DEF.pick })
        .then(renderCondPicker, renderCondPicker);
    });
    condModal.addEventListener('keydown', function (e) {
      if (e.key === 'Escape') { condModal.classList.remove('open'); }
    });

    defAddBox.addEventListener('click', function (e) {
      if (e.target === defAddBox || e.target.closest('[data-def-close]')) {
        defAddBox.classList.remove('open');
        return;
      }
      var go = e.target.closest('[data-add]');
      if (go) { doDefenseAdd(go.getAttribute('data-add')); }
    });
    defAddBox.addEventListener('keydown', function (e) {
      if (e.key === 'Escape') { defAddBox.classList.remove('open'); return; }
      if (e.key === 'Enter') { e.preventDefault(); doDefenseAdd('resist'); }
    });
    fillDamageTypes();

    renderDefenses();
    // The file on disk is the truth; the page catches up with it on load.
    defLoad(true);
  }

  function noop() {}

  // ------------------------------------------------------------- hit points
  //
  // The box next to the hit point bar: a number, and the three things that
  // can be done with it between rests. Everything goes through the server and
  // into the org file, so the damage taken tonight is still there next time
  // the sheet is opened - and lands in the Health History section with it.
  var HEALTH = { state: null, busy: false, error: '', msg: '' };
  var hpCtl, hpAmount;

  function healthConfigured() { return !!(LOG.file || CHARACTER_ID); }

  function healthLoad(quiet) {
    if (!healthConfigured()) { return Promise.resolve(); }
    var q = '/dnd/hp?filename=' + encodeURIComponent(LOG.file || '') +
      '&id=' + encodeURIComponent(CHARACTER_ID || '');
    return api('GET', q).then(function (state) {
      HEALTH.state = state;
      HEALTH.error = '';
      paintHitPoints();
    }, function (err) {
      if (!quiet) { HEALTH.error = err.message || String(err); paintHitPoints(); }
    });
  }

  // The bands the bar is coloured in. These are HealthLevel in health.go said
  // again in javascript, for the one caller - a rest - that works out where
  // the hit points landed itself rather than being told by the server.
  function hpLevel(percent) {
    if (percent < 25) { return 'dying'; }
    if (percent < 50) { return 'bloodied'; }
    if (percent < 75) { return 'hurt'; }
    return 'hale';
  }

  var HP_LEVELS = ['hale', 'hurt', 'bloodied', 'dying'];

  // paintHpBar draws the gauge: the number you are on at the left, the bar,
  // and the number you started with at the right, all three carrying the same
  // colour so the bar is read without doing the arithmetic.
  function paintHpBar(current, max, percent, level) {
    if (typeof percent !== 'number') {
      percent = max > 0 ? Math.max(0, Math.min(100, Math.round(current * 100 / max))) : 0;
    }
    level = level || hpLevel(percent);
    var now = document.getElementById('hp-now');
    if (now) {
      now.textContent = current;
      HP_LEVELS.forEach(function (l) { now.classList.toggle(l, l === level); });
    }
    var top = document.getElementById('hp-max');
    if (top) { top.textContent = max; }
    var bar = document.getElementById('hp-bar');
    if (bar) { bar.setAttribute('aria-label', current + ' of ' + max + ' hit points'); }
    var fill = document.getElementById('hp-fill');
    if (fill) {
      fill.style.width = percent + '%';
      HP_LEVELS.forEach(function (l) { fill.classList.toggle(l, l === level); });
    }
  }

  // paintHitPoints redraws the three things the hit point line is made of:
  // the numbers, the bar and the temporary hit points. It is the one place
  // that touches them, so a rest and a hit leave the sheet looking the same.
  function paintHitPoints() {
    var hp = (HEALTH.state && HEALTH.state.hp) || null;
    var msg = document.getElementById('hp-msg');
    if (msg) {
      msg.className = HEALTH.error ? 'inv-err' : 'inv-msg';
      msg.textContent = HEALTH.busy ? 'saving…' : (HEALTH.error || HEALTH.msg || '');
    }
    if (!hp) { return; }
    var line = document.getElementById('hp-line');
    if (line) {
      line.innerHTML = '<strong>Hit Points</strong>' +
        (hp.temp ? ' <span class="tagline">+' + hp.temp + ' temp</span>' : '');
    }
    paintHpBar(hp.current, hp.max, hp.percent, hp.level);
    var val = document.getElementById('hp-temp-val');
    if (val) {
      val.textContent = hp.temp || 0;
      // Nought temporary hit points is worth saying, but not worth shouting.
      val.classList.toggle('none', !hp.temp);
    }
  }

  function healthChange(action) {
    if (!healthConfigured()) {
      HEALTH.error = 'this sheet does not know which org file it came from';
      paintHitPoints();
      return;
    }
    var amount = parseInt(hpAmount && hpAmount.value, 10);
    if (isNaN(amount) || amount < 0) {
      HEALTH.error = action === 'temp' ? 'how many temporary hit points?' : 'how much?';
      HEALTH.msg = '';
      paintHitPoints();
      if (hpAmount) { hpAmount.focus(); }
      return;
    }
    HEALTH.busy = true;
    HEALTH.error = '';
    paintHitPoints();
    api('POST', '/dnd/hp', { filename: LOG.file || '', id: CHARACTER_ID || '',
                             action: action, amount: amount })
      .then(function (state) {
        HEALTH.busy = false;
        HEALTH.state = state;
        HEALTH.msg = state.msg || '';
        if (hpAmount) { hpAmount.value = ''; hpAmount.focus(); }
        paintHitPoints();
        // A blow landing is worth a line in the night's log.
        var last = (state.history || [])[state.history.length - 1];
        if (last) {
          var text = state.msg.charAt(0).toUpperCase() + state.msg.slice(1);
          logNote((last.action === 'hurt' ? '*Damage.* '
                 : last.action === 'healed' ? '*Healing.* '
                 : '*Hit Points.* ') + text + '.');
        }
      }, function (err) {
        HEALTH.busy = false;
        HEALTH.msg = '';
        HEALTH.error = err.message || String(err);
        paintHitPoints();
      });
  }

  function buildHealthUi() {
    hpCtl = document.getElementById('hp-ctl');
    if (!hpCtl) { return; }
    hpAmount = document.getElementById('hp-amount');

    var seed = document.getElementById('dnd-health-data');
    if (seed) {
      try {
        var got = JSON.parse(seed.textContent || 'null');
        if (got && got.hp) { HEALTH.state = got; }
      } catch (e) { /* wait for the server */ }
    }

    hpCtl.addEventListener('click', function (e) {
      var btn = e.target.closest('[data-hp]');
      if (btn) { healthChange(btn.getAttribute('data-hp')); }
    });
    hpCtl.addEventListener('keydown', function (e) {
      if (e.key !== 'Enter' || e.target !== hpAmount) { return; }
      e.preventDefault();
      // Enter is the one that gets pressed in a fight, and in a fight it is
      // nearly always damage.
      healthChange('hurt');
    });

    paintHitPoints();
    healthLoad(true);
  }

  // ------------------------------------------------------------ inspiration
  // One bit, handed out by the DM and spent by the player. Pressing the
  // marker in the header posts to the server, which rewrites
  // DND_INSPIRATION in the character's own org file - so the sheet, the file
  // and anyone else reading it agree, the same way hit points do.
  var INSP = { held: false, busy: false };
  var inspBox, inspBtn, inspWord;

  function paintInspiration() {
    if (!inspBox) { return; }
    inspBox.classList.toggle('on', !!INSP.held);
    inspBox.classList.toggle('busy', !!INSP.busy);
    if (inspWord) { inspWord.textContent = INSP.held ? 'Held' : 'None'; }
    if (inspBtn) {
      inspBtn.setAttribute('aria-pressed', INSP.held ? 'true' : 'false');
      inspBtn.disabled = !!INSP.busy;
    }
  }

  function inspirationLoad() {
    if (!healthConfigured()) { return; }
    api('GET', '/dnd/inspiration?filename=' + encodeURIComponent(LOG.file || '') +
        '&id=' + encodeURIComponent(CHARACTER_ID || ''))
      .then(function (state) {
        INSP.held = !!state.inspiration;
        paintInspiration();
      }, function () { /* the sheet already shows what was exported */ });
  }

  function inspirationToggle() {
    if (INSP.busy) { return; }
    if (!healthConfigured()) {
      // Nothing to write to, so the marker stays where the export left it.
      return;
    }
    INSP.busy = true;
    paintInspiration();
    var spending = INSP.held;
    api('POST', '/dnd/inspiration', { filename: LOG.file || '',
                                      id: CHARACTER_ID || '', action: 'toggle' })
      .then(function (state) {
        INSP.busy = false;
        INSP.held = !!state.inspiration;
        paintInspiration();
        logNote(spending ? '*Inspiration.* Spent.' : '*Inspiration.* Gained.');
      }, function () {
        INSP.busy = false;
        paintInspiration();
      });
  }

  function buildInspirationUi() {
    inspBox = document.getElementById('insp');
    if (!inspBox) { return; }
    inspBtn = document.getElementById('insp-btn');
    inspWord = document.getElementById('insp-word');
    INSP.held = inspBox.classList.contains('on');
    if (inspBtn) { inspBtn.addEventListener('click', inspirationToggle); }
    paintInspiration();
    inspirationLoad();
  }

  // ------------------------------------------------------------- spells
  //
  // The spell page is the character's book: what they know, what of it is
  // ready to cast, and how much room is left in the allowances their class
  // gets at this level. Every change is written straight back into the spell
  // tables of the org character sheet.
  //
  // What may be taken is never decided here. The server answers with what the
  // rules engine allows - a spell has to be on a list the character can draw
  // from, of a level they have slots for, with room left in the budget it
  // comes out of - and this only draws that answer and posts the change back.
  var SB = { state: null, hits: [], busy: false, error: '', msg: '',
             group: 'level', show: 'all', q: '', open: '' };
  var sbBox, sbLive, sbBudgets, sbModal, sbSearchTimer = null;

  function sbConfigured() { return !!(LOG.file || CHARACTER_ID); }

  function sbWho(body) {
    body = body || {};
    body.filename = LOG.file || '';
    body.id = CHARACTER_ID || '';
    return body;
  }

  function sbBook() { return (SB.state && SB.state.book) || null; }

  function sbLoad(quiet) {
    if (!sbConfigured()) { return Promise.resolve(); }
    var q = '/dnd/spellbook?filename=' + encodeURIComponent(LOG.file || '') +
      '&id=' + encodeURIComponent(CHARACTER_ID || '');
    return api('GET', q).then(function (state) {
      SB.state = state;
      SB.error = '';
      renderSpells();
    }, function (err) {
      if (!quiet) { SB.error = err.message || String(err); renderSpells(); }
    });
  }

  // sbChange posts one change and redraws from what comes back, so the sheet
  // always shows what is actually written in the file.
  function sbChange(action, spell) {
    if (!sbConfigured()) {
      SB.error = 'this sheet does not know which org file it came from';
      renderSpells();
      return Promise.reject(new Error(SB.error));
    }
    SB.busy = true;
    SB.error = '';
    renderSpells();
    return api('POST', '/dnd/spellbook',
               sbWho({ action: action, spell: spell })).then(function (state) {
      SB.busy = false;
      SB.state = state;
      SB.msg = state.msg || '';
      renderSpells();
      // The budgets moved, so what the picker may offer has moved with them.
      return sbSearch(SB.q, true).then(function () { return state; });
    }, function (err) {
      SB.busy = false;
      SB.error = err.message || String(err);
      renderSpells();
      throw err;
    });
  }

  // ---------------------------------------------------------- spell slots
  //
  // Casting a spell of 1st level or higher costs a slot, and the slot is
  // struck off the org character sheet as the dice are rolled. The server
  // decides which one it comes out of: a spell may always be cast from a
  // higher slot, so when its own level is empty the cast reaches upward and
  // says where it landed.
  //
  // The pips are also a control in their own right. A spell cast from a
  // scroll or a ring costs nothing, and a cast the table decides never
  // happened can be given back, so clicking an empty pip spends a slot and
  // clicking a spent one hands it back.
  var sbMsgTimer = null;

  // Writing needs a server to write to as well as a file to write into.
  function sbCanWrite() { return !!(LOG.url && sbConfigured()); }

  // sbSay puts a line beside the budgets and takes it away again, so the
  // last cast is visible without the sheet keeping a running commentary.
  function sbSay(msg, err) {
    SB.msg = err ? '' : (msg || '');
    SB.error = err ? msg : '';
    sbPaintBudgets();
    if (sbMsgTimer) { clearTimeout(sbMsgTimer); sbMsgTimer = null; }
    if (!msg) { return; }
    sbMsgTimer = setTimeout(function () {
      SB.msg = '';
      SB.error = '';
      sbPaintBudgets();
    }, 6000);
  }

  // Only the pips moved, so only the pips are repainted: a full redraw would
  // fold shut whatever spell was open on the table at the time.
  function sbPaintSlots() {
    var book = sbBook();
    if (!sbLive || !book) { return; }
    var rows = sbLive.querySelectorAll('.slot-row');
    if (!rows.length) { renderSpells(); return; }
    (book.slots || []).forEach(function (s) {
      var row = sbLive.querySelector('.slot-row[data-level="' + s.level + '"]');
      if (!row) { return; }
      var pips = row.querySelectorAll('.slot');
      if (pips.length !== s.total) { renderSpells(); return; }
      row.setAttribute('data-used', s.used);
      Array.prototype.forEach.call(pips, function (pip, i) {
        pip.classList.toggle('used', i < s.used);
      });
    });
  }

  // sbSlotPips moves the pips of one row before the server has answered, so a
  // cast feels immediate on a slow link. What comes back puts it right.
  function sbSlotPips(row, used) {
    var pips = row.querySelectorAll('.slot');
    Array.prototype.forEach.call(pips, function (pip, i) {
      pip.classList.toggle('used', i < used);
    });
    row.setAttribute('data-used', used);
  }

  // note, when given, is told what the server said, so a cast can put it on
  // the card the dice landed on as well as beside the budgets.
  function sbSlotChange(action, level, spell, note) {
    if (!sbCanWrite()) {
      var why = LOG.url
        ? 'this sheet does not know which org file it came from'
        : 'set your orgs server address in the session panel first';
      sbSay(why, true);
      if (note) { note(why, true); }
      return Promise.resolve(null);
    }
    return api('POST', '/dnd/slots',
               sbWho({ action: action, level: level, spell: spell || '' }))
      .then(function (state) {
        SB.state = state;
        sbPaintSlots();
        sbSay(state.msg || '');
        if (note) { note(sbSlotNote(state), false); }
        return state;
      }, function (err) {
        // The file did not move, so neither should the pips.
        sbPaintSlots();
        var msg = err.message || String(err);
        sbSay(msg, true);
        if (note) { note(msg, true); }
        return null;
      });
  }

  // What the cast cost, in the few words the roll card has room for.
  function sbSlotNote(state) {
    var slot = state && state.slot;
    if (!slot) { return ''; }
    var lvl = slot.label || slot.level;
    return (slot.up ? 'Cast at ' + lvl + ' level. ' : '') +
      lvl + ' level slots: ' + slot.left + ' of ' + slot.total + ' left';
  }

  // castSlot is called once the dice are on the table. A cantrip costs
  // nothing; everything else marks a slot off the file.
  function castSlot(btn, result) {
    var level = parseInt(btn.getAttribute('data-level'), 10) || 0;
    if (level < 1 || !sbCanWrite()) { return; }
    var ritual = btn.getAttribute('data-ritual') === '1';
    sbSlotChange('cast', level, btn.getAttribute('data-spell') || '',
      function (msg, bad) {
        // A ritual can be cast the slow way for no slot at all, so being out
        // of slots is not a refusal for one of those - it is the answer.
        if (bad && ritual) {
          noteCastSlot(result, 'No slot to spend, so cast as a ritual: ' +
            '10 minutes longer, and nothing off the sheet.', false);
          sbSay('cast as a ritual, no slot spent');
          return;
        }
        noteCastSlot(result, msg, bad);
      });
  }

  // A click on a pip: past what is spent it spends one, at or below it hands
  // one back. Unlike a cast this means the level clicked and no other.
  function sbSlotClick(pip) {
    var row = pip.closest('.slot-row');
    if (!row || !sbCanWrite()) { return; }
    var level = parseInt(row.getAttribute('data-level'), 10) || 0;
    var was = parseInt(row.getAttribute('data-used'), 10) || 0;
    var pips = Array.prototype.slice.call(row.querySelectorAll('.slot'));
    var at = pips.indexOf(pip) + 1;
    if (!level || at < 1) { return; }
    var action = at > was ? 'spend' : 'recover';
    sbSlotPips(row, was + (action === 'spend' ? 1 : -1));
    sbSlotChange(action, level, '');
  }

  // ------------------------------------------------------- drawing the page
  function sbBudgetHtml(list) {
    if (!list || !list.length) { return ''; }
    return '<span class="sb-budgets">' + list.map(function (a) {
      return '<span class="sb-budget' + (a.full ? ' full' : '') + '" title="' +
        esc(a.note || '') + '">' + esc(a.name) + ' <b>' + a.used + '/' + a.max +
        '</b></span>';
    }).join('') + '</span>';
  }

  function sbSlotsHtml(slots) {
    var live = sbCanWrite();
    return (slots || []).map(function (s) {
      var pips = '';
      for (var i = 1; i <= s.total; i++) {
        pips += '<span class="slot' + (i <= s.used ? ' used' : '') + '"></span>';
      }
      return '<div class="slot-row' + (live ? ' live' : '') + '" data-level="' +
        s.level + '" data-used="' + s.used + '"' +
        (live ? ' title="Click a slot to spend it, a spent one to give it back"' : '') +
        '><span class="lvl">' + esc(s.label) + '</span>' +
        pips + '<span class="tagline">' + s.total + ' slot' +
        (s.total > 1 ? 's' : '') + '</span></div>';
    }).join('');
  }

  // One spell as the sheet shows it. The cast button carries everything the
  // dice need, which the rules engine worked out when it computed the sheet.
  function sbSpellHtml(sp, level) {
    var c = sp.cast || {};
    var attrs = ' data-spell="' + esc(sp.name) + '"' +
      ' data-detail="' + esc(c.detail || '') + '"' +
      ' data-line="' + esc(c.line || '') + '"' +
      ' data-short="' + esc(c.short || '') + '"';
    if (c.attack) { attrs += ' data-attack="' + c.attackBonus + '"'; }
    if (c.damage) {
      attrs += ' data-damage="' + esc(c.damage) + '"' +
        ' data-damage-type="' + esc(c.damageType || '') + '"';
    }
    if (c.heal) { attrs += ' data-heal="' + esc(c.heal) + '"'; }
    attrs += ' data-level="' + (level || 0) + '"';
    if (sp.ritual) { attrs += ' data-ritual="1"'; }
    if (c.save) {
      attrs += ' data-save="' + esc(c.saveName || '') + '" data-dc="' + c.saveDc + '"';
    }
    var tags = [sp.castingTime, sp.range].filter(Boolean).join(', ');
    if (sp.concentration) { tags += ', concentration'; }
    if (sp.ritual) { tags += ', ritual'; }
    var body = '<em>' + esc(sp.school || '') +
      (sp.components ? ' &middot; ' + esc(sp.components) : '') +
      (sp.duration ? ' &middot; ' + esc(sp.duration) : '') + '</em>\n' + esc(sp.text || '');
    if (sp.higherLevel) {
      body += '\n\n<strong>At higher levels.</strong> ' + esc(sp.higherLevel);
    }
    return '<details class="spell"><summary>' +
      (sp.prepared && level > 0 ? '<span class="prep">&#9679;</span> ' : '') +
      esc(sp.name) + ' <span class="tagline">&mdash; ' + esc(tags) + '</span>' +
      '<button type="button" class="cast-btn"' + attrs + ' title="Cast ' +
        esc(sp.name) + '">Cast</button>' +
      '</summary><p>' + body + '</p></details>';
  }

  function sbPaintBudgets() {
    var book = sbBook();
    if (!sbBudgets) { return; }
    sbBudgets.innerHTML = sbBudgetHtml(book && book.allotments) +
      (SB.busy ? ' <span class="sb-msg">saving&hellip;</span>'
               : (SB.msg ? ' <span class="sb-msg">' + esc(SB.msg) + '</span>' : '')) +
      (SB.error ? ' <span class="sb-err">' + esc(SB.error) + '</span>' : '');
  }

  function renderSpells() {
    if (!sbLive) { return; }
    var book = sbBook();
    sbPaintBudgets();
    if (!book) { return; }
    var counts = [];
    (book.allotments || []).forEach(function (a) {
      counts.push(a.used + '/' + a.max + ' ' + a.name.toLowerCase());
    });
    if (book.notes) { counts.push(book.notes); }
    var levels = (book.levels || []).map(function (lvl) {
      if (!lvl.spells || !lvl.spells.length) { return ''; }
      return '<div class="spell-level"><h3>' + esc(lvl.name) +
        (lvl.slots ? ' (' + lvl.slots + ' slot' + (lvl.slots > 1 ? 's' : '') + ')' : '') +
        '</h3>' + lvl.spells.map(function (sp) {
          return sbSpellHtml(sp, lvl.level);
        }).join('') + '</div>';
    }).join('');
    sbLive.innerHTML =
      '<div class="tagline">' + esc(counts.join(' · ')) + '</div>' +
      sbSlotsHtml(book.slots) +
      (levels || '<div class="sb-empty">No spells yet. Manage spells to take some.</div>');
    // Dice written into a spell's text are clickable, the same as they are
    // in the markup this replaced.
    Array.prototype.forEach.call(sbLive.querySelectorAll('details.spell p'), linkifyProse);
  }

  // ------------------------------------------------------- the manage box
  // The whole list the character may draw from is fetched at once, because
  // the box regroups and filters it locally: asking the server again every
  // time somebody switches from level to school would be a round trip for an
  // answer we already have.
  var sbSearchSeq = 0;

  function sbSearch(q, quiet) {
    SB.q = q;
    var mine = ++sbSearchSeq;
    var path = '/dnd/spells?q=' + encodeURIComponent(q || '') +
      '&filename=' + encodeURIComponent(LOG.file || '') +
      '&id=' + encodeURIComponent(CHARACTER_ID || '');
    if (!quiet) { sbListEl().innerHTML = '<div class="sb-empty">Looking&hellip;</div>'; }
    return api('GET', path).then(function (hits) {
      if (mine !== sbSearchSeq) { return; }
      SB.hits = hits || [];
      renderSbList();
    }, function (err) {
      if (mine !== sbSearchSeq) { return; }
      sbListEl().innerHTML = '<div class="sb-err">' + esc(err.message || String(err)) + '</div>';
    });
  }

  function sbListEl() { return sbModal.querySelector('#sb-list'); }

  // The heading a spell falls under, for each way of grouping the list.
  function sbGroupOf(h) {
    switch (SB.group) {
      case 'school': return h.school ? titleCase(h.school) : 'Unschooled';
      case 'time':
        return (h.castingTime || 'Unknown casting time').split(',')[0].trim();
      case 'status':
        if (h.granted) { return 'Granted by your subclass'; }
        if (h.known && h.prepared) { return 'Ready to cast'; }
        if (h.known) { return 'In your book, not prepared'; }
        if (h.canLearn) { return 'You could take these'; }
        return 'Out of reach for now';
      default: return h.levelName || 'Cantrips';
    }
  }

  function titleCase(s) {
    return String(s).replace(/\b[a-z]/g, function (c) { return c.toUpperCase(); });
  }

  function sbRankOf(h) {
    if (SB.group === 'level') { return h.level; }
    if (SB.group === 'status') {
      if (h.known && h.prepared) { return 0; }
      if (h.known) { return 1; }
      if (h.granted) { return 2; }
      if (h.canLearn) { return 3; }
      return 4;
    }
    return null;   // school and casting time keep the order they arrived in
  }

  function sbKeep(h) {
    if (SB.show === 'mine') { return h.known; }
    if (SB.show === 'open') { return h.canLearn; }
    return true;
  }

  function sbItemHtml(h) {
    var meta = [h.levelName, h.school ? titleCase(h.school) : '', h.castingTime,
                h.range, h.duration].filter(Boolean).join(' · ');
    var tags = '';
    if (h.concentration) { tags += '<span class="sb-tag">concentration</span>'; }
    if (h.ritual) { tags += '<span class="sb-tag">ritual</span>'; }
    if (h.expanded) { tags += '<span class="sb-tag">expanded list</span>'; }
    if (h.granted) { tags += '<span class="sb-tag held">' + esc(h.source) + '</span>'; }
    var acts = '';
    if (h.canLearn) {
      acts += '<button type="button" class="sb-b" data-act="learn" data-spell="' +
        esc(h.id) + '">' + (sbBook() && sbBook().mode === 'list' ? 'Prepare' : 'Learn') +
        '</button>';
    }
    // Preparing is only a step of its own for a class that prepares out of a
    // spellbook; everywhere else having the spell is having it ready.
    if (h.canPrepare) {
      acts += '<button type="button" class="sb-b" data-act="' +
        (h.prepared ? 'unprepare' : 'prepare') + '" data-spell="' + esc(h.id) + '">' +
        (h.prepared ? 'Unprepare' : 'Prepare') + '</button>';
    }
    if (h.canForget) {
      acts += '<button type="button" class="sb-b give" data-act="forget" data-spell="' +
        esc(h.id) + '" title="Take it off your sheet">Give back</button>';
    }
    var mark = '';
    if (h.known) { mark = '<span class="mark">' + (h.prepared ? '&#9679;' : '&#9675;') + '</span>'; }
    var why = h.why && !h.known ? ' <span class="why">' + esc(h.why) + '</span>' : '';
    var open = SB.open === h.id;
    return '<div class="sb-item' + (h.known ? ' mine' : '') +
      (!h.known && !h.canLearn ? ' blocked' : '') + '" data-id="' + esc(h.id) + '">' +
      '<div class="sb-what" data-what="' + esc(h.id) + '">' +
        '<div class="sb-name">' + mark + esc(h.name) + tags + '</div>' +
        '<div class="sb-meta">' + esc(meta) + why + '</div>' +
        (open ? '<div class="sb-text">' + esc(h.text || '') + '</div>' : '') +
      '</div>' +
      '<div class="sb-acts">' + acts + '</div></div>';
  }

  function renderSbList() {
    var el = sbListEl();
    if (!el) { return; }
    var kept = SB.hits.filter(sbKeep);
    if (!kept.length) {
      el.innerHTML = '<div class="sb-empty">Nothing here. ' +
        (SB.show === 'open'
          ? 'Every spell you can reach is already on your sheet.'
          : 'Try another search or another filter.') + '</div>';
      return;
    }
    // The server hands the list back in level order; grouping walks it in
    // that order so a group's spells stay in it.
    var order = [], groups = {}, rank = {};
    kept.forEach(function (h) {
      var key = sbGroupOf(h);
      if (!groups[key]) { groups[key] = []; order.push(key); rank[key] = sbRankOf(h); }
      groups[key].push(h);
    });
    if (order.length && rank[order[0]] !== null) {
      order.sort(function (a, b) { return rank[a] - rank[b]; });
    }
    el.innerHTML = order.map(function (key) {
      return '<div class="sb-group">' + esc(key) + ' <span class="n">' +
        groups[key].length + '</span></div>' +
        groups[key].map(sbItemHtml).join('');
    }).join('');
  }

  function sbHeader() {
    var book = sbBook();
    if (!book) { return ''; }
    var how = {
      known: 'Your spells are always ready to cast - taking one is all there is to it.',
      list: 'You prepare from the whole ' + esc(book.className || 'class') +
        ' list, so a spell on your sheet is a spell prepared today.',
      spellbook: 'Spells you take are written into your spellbook. Preparing them ' +
        'out of it is a second, smaller allowance.'
    }[book.mode] || '';
    return '<div class="sb-hint" style="margin:0 0 6px">' +
      sbBudgetHtml(book.allotments) +
      (how ? '<div style="margin-top:4px">' + how +
        (book.maxLevel ? ' You can cast up to level ' + book.maxLevel + '.' : '') +
        '</div>' : '') + '</div>';
  }

  function openSpellManager() {
    if (!sbModal) { return; }
    sbModal.querySelector('#sb-head').innerHTML = sbHeader();
    sbModal.querySelector('#sb-err').textContent = '';
    sbModal.classList.add('open');
    var q = sbModal.querySelector('#sb-q');
    q.value = SB.q || '';
    q.focus();
    sbSearch(q.value);
  }

  function closeSpellManager() { if (sbModal) { sbModal.classList.remove('open'); } }

  function sbChips(name, value, opts) {
    return opts.map(function (o) {
      return '<button type="button" class="sb-chip' + (o[0] === value ? ' on' : '') +
        '" data-' + name + '="' + o[0] + '">' + o[1] + '</button>';
    }).join('');
  }

  function sbRenderTools() {
    sbModal.querySelector('#sb-tools').innerHTML =
      '<span class="lbl">Group by</span>' +
      sbChips('group', SB.group, [['level', 'Level'], ['school', 'School'],
                                  ['time', 'Casting time'], ['status', 'Status']]) +
      '<span class="lbl">Show</span>' +
      sbChips('show', SB.show, [['all', 'All'], ['mine', 'On my sheet'],
                                ['open', 'I can take']]);
  }

  // ------------------------------------------------------------- building
  function buildSpellUi() {
    sbBox = document.getElementById('spellcasting');
    if (!sbBox) { return; }
    sbLive = sbBox.querySelector('#spell-live');
    sbBudgets = sbBox.querySelector('#spell-budgets');

    sbModal = document.createElement('div');
    sbModal.className = 'sb-modal';
    sbModal.id = 'sb-manage';
    sbModal.innerHTML =
      '<div class="sb-card" role="dialog" aria-label="Manage your spells">' +
        '<h3>Manage spells' +
          '<button type="button" class="x" id="sb-close" aria-label="Close">&times;</button></h3>' +
        '<div id="sb-head"></div>' +
        '<input type="search" class="sb-field" id="sb-q" autocomplete="off" ' +
          'placeholder="Search your lists: mag mis, ritual, healing...">' +
        '<div class="sb-tools" id="sb-tools"></div>' +
        '<div class="sb-list" id="sb-list"></div>' +
        '<div class="sb-hint">A filled dot is ready to cast, an open one is ' +
          'written down but not prepared. Click a spell to read it.</div>' +
        '<div class="sb-err" id="sb-err"></div>' +
      '</div>';
    document.body.appendChild(sbModal);
    sbRenderTools();

    sbBox.addEventListener('click', function (e) {
      if (e.target.closest('#spell-manage-btn')) { openSpellManager(); return; }
      var pip = e.target.closest('.slot');
      if (pip) { sbSlotClick(pip); }
    });

    sbModal.addEventListener('click', function (e) {
      if (e.target === sbModal || e.target.closest('#sb-close')) {
        closeSpellManager();
        return;
      }
      var chip = e.target.closest('[data-group]');
      if (chip) {
        SB.group = chip.getAttribute('data-group');
        sbRenderTools();
        renderSbList();
        return;
      }
      chip = e.target.closest('[data-show]');
      if (chip) {
        SB.show = chip.getAttribute('data-show');
        sbRenderTools();
        renderSbList();
        return;
      }
      var act = e.target.closest('.sb-b');
      if (act) {
        sbModal.querySelector('#sb-err').textContent = '';
        sbChange(act.getAttribute('data-act'), act.getAttribute('data-spell')).then(
          function () { sbModal.querySelector('#sb-head').innerHTML = sbHeader(); },
          function (err) {
            sbModal.querySelector('#sb-err').textContent = err.message || String(err);
          });
        return;
      }
      // Anywhere else on a spell opens it up to read, and shuts it again.
      var what = e.target.closest('.sb-what');
      if (what) {
        var id = what.getAttribute('data-what');
        SB.open = SB.open === id ? '' : id;
        renderSbList();
      }
    });

    sbModal.querySelector('#sb-q').addEventListener('input', function (e) {
      var q = e.target.value;
      if (sbSearchTimer) { clearTimeout(sbSearchTimer); }
      sbSearchTimer = setTimeout(function () { sbSearch(q); }, 140);
    });
    sbModal.addEventListener('keydown', function (e) {
      if (e.key === 'Escape') { closeSpellManager(); }
    });

    // The file on disk is the truth; the page catches up with it on load.
    sbLoad(true);
  }

  // ------------------------------------------------- feature uses and rests
  //
  // Some features are rationed - "twice, and you regain both when you finish
  // a short rest" - and the rules engine reads that limit out of the
  // feature's own text. Here those limits become a row of slots you can click
  // through, and the two rests become a walkthrough that says what to do,
  // rolls the hit dice on the table, and writes the result to the org
  // character sheet and to the session log in one go.
  //
  // As everywhere else on this sheet, the file is the state: nothing here is
  // remembered in the page, and every change is a round trip.

  var REST = {
    plans: null,      // both plans, as the server last computed them
    kind: '',         // the rest being taken
    plan: null,
    step: 0,
    spent: 0,         // hit dice spent so far in this rest
    healed: 0,        // what those dice came to
    rolls: [],        // what each of them landed on
    note: '',
    done: null,       // the result, once the rest has been taken
    busy: false, error: ''
  };
  var restModal, restBody, restFoot, restDots, featureLive;

  function restConfigured() { return !!(LOG.url && (LOG.file || CHARACTER_ID)); }

  function restWho(body) {
    body.filename = LOG.file || '';
    body.id = CHARACTER_ID || '';
    return body;
  }

  // ------------------------------------------------------ the uses on a feature
  // A pip is one use. Clicking an empty one spends it, clicking a filled one
  // hands it back. The page moves first and puts itself right again if the
  // server refuses, so a click feels immediate on a slow link.
  function usesPips(box, spent) {
    var pips = box.querySelectorAll('.use-pip');
    Array.prototype.forEach.call(pips, function (pip, i) {
      pip.classList.toggle('used', i < spent);
    });
    box.setAttribute('data-spent', spent);
  }

  function useChange(feature, box, action, was) {
    var spent = was + (action === 'spend' ? 1 : -1);
    usesPips(box, spent);
    box.classList.add('busy');
    api('POST', '/dnd/uses', restWho({ action: action, feature: feature }))
      .then(function (state) {
        REST.plans = state;
        box.classList.remove('busy');
        restNote(box, state.msg || '');
        // The rest menu now offers something different, and so may the
        // walkthrough if it happens to be open on its first page.
        if (restModal && restModal.classList.contains('open') && !REST.done) {
          REST.plan = state[REST.kind] || REST.plan;
          renderRest();
        }
      }, function (err) {
        usesPips(box, was);
        box.classList.remove('busy');
        restNote(box, err.message || String(err), true);
      });
  }

  // What the server said about the last change, shown in place of the limit
  // for a moment and then put back.
  function restNote(box, msg, bad) {
    var note = box.querySelector('.uses-note');
    if (!note) { return; }
    if (note.plainText === undefined) { note.plainText = note.textContent; }
    note.textContent = msg || note.plainText;
    note.classList.toggle('uses-err', !!bad);
    if (note.noteTimer) { clearTimeout(note.noteTimer); note.noteTimer = null; }
    if (!msg) { return; }
    note.noteTimer = setTimeout(function () {
      note.textContent = note.plainText;
      note.classList.remove('uses-err');
    }, 4000);
  }

  function buildFeatureUses() {
    featureLive = document.getElementById('feature-live');
    if (!featureLive) { return; }
    // Without a server there is nothing to write a spent use to, so the pips
    // stay as they are: a record of what the file said, not a control.
    if (!restConfigured()) { return; }
    Array.prototype.forEach.call(featureLive.querySelectorAll('.uses'), function (box) {
      box.classList.add('live');
    });
    featureLive.addEventListener('click', function (e) {
      var pip = e.target.closest('.use-pip');
      if (!pip) { return; }
      var box = pip.closest('.uses'), host = pip.closest('.feature');
      if (!box || !host || box.classList.contains('busy')) { return; }
      var pips = Array.prototype.slice.call(box.querySelectorAll('.use-pip'));
      var at = pips.indexOf(pip) + 1;
      var was = parseInt(box.getAttribute('data-spent'), 10) || 0;
      useChange(host.getAttribute('data-uses'), box,
                at > was ? 'spend' : 'recover', was);
    });
  }

  // resetFeatureUses hands back on screen exactly what the rest handed back
  // on the file: a short rest clears the features that recharge on one, a
  // long rest clears them all.
  function resetFeatureUses(kind) {
    if (!featureLive) { return; }
    Array.prototype.forEach.call(featureLive.querySelectorAll('.feature[data-uses]'),
      function (host) {
        if (kind === 'short' && host.getAttribute('data-recharge') !== 'short') { return; }
        var box = host.querySelector('.uses');
        if (box) { usesPips(box, 0); }
      });
  }

  // ------------------------------------------------------------ the sheet moves
  // What a rest changed, written back into the boxes that show it, so the
  // sheet agrees with the file without being exported again.
  function applyRestToSheet(result, plan) {
    var line = document.getElementById('hp-line');
    if (line) { line.innerHTML = '<strong>Hit Points</strong>'; }
    paintHpBar(result.hpAfter, result.hpMax);
    var pool = document.getElementById('hitdie-pool');
    var spent = document.getElementById('hitdie-spent');
    if (plan) {
      var used = plan.hitDiceMax - plan.hitDiceLeft;
      if (pool) { pool.textContent = plan.hitDice || ''; }
      if (spent) { spent.textContent = used ? used + ' spent' : ''; }
    }
    resetFeatureUses(result.kind);
    // A rest moves the hit points and a long one clears the temporary ones,
    // so the hit point line is repainted from what the file now says.
    healthLoad(true);
    // The slots came back too, and the spell page draws itself from the file.
    if (result.slotsBack > 0) { sbLoad(true); }
  }

  // ------------------------------------------------------------- the walkthrough
  function openRest(kind) {
    REST.kind = kind;
    REST.step = 0;
    REST.spent = 0;
    REST.healed = 0;
    REST.rolls = [];
    REST.note = '';
    REST.done = null;
    REST.error = '';
    REST.plan = REST.plans ? REST.plans[kind] : null;
    restModal.classList.add('open');
    renderRest();
    restLoad();
  }

  function closeRest() {
    restModal.classList.remove('open');
  }

  function restLoad() {
    if (!restConfigured()) {
      REST.error = LOG.url
        ? 'this sheet does not know which org file it came from'
        : 'set your orgs server address in the session panel first';
      renderRest();
      return Promise.resolve();
    }
    REST.busy = !REST.plan;
    renderRest();
    var q = '/dnd/rest?filename=' + encodeURIComponent(LOG.file || '') +
      '&id=' + encodeURIComponent(CHARACTER_ID || '');
    return api('GET', q).then(function (state) {
      REST.plans = state;
      REST.busy = false;
      REST.error = '';
      if (!REST.done) { REST.plan = state[REST.kind] || null; }
      renderRest();
    }, function (err) {
      REST.busy = false;
      REST.error = err.message || String(err);
      renderRest();
    });
  }

  function restSteps() { return (REST.plan && REST.plan.steps) || []; }

  // One hit die, thrown on the table like every other roll this sheet makes,
  // so it lands in the tray and in the session log as itself.
  function rollHitDie(btn) {
    var step = restSteps()[REST.step];
    if (!step || REST.spent >= step.max) { return; }
    var faces = parseInt(String(step.die).replace(/^d/, ''), 10) || 8;
    var mod = step.mod || 0;
    var box = btn.getBoundingClientRect();
    roll({ kind: 'damage', label: 'Hit Die', noCrit: true,
           terms: [{ count: 1, sides: faces, sign: 1 }], flat: mod,
           formula: '1d' + faces + (mod ? ' ' + signed(mod) : '') },
         { x: box.left + box.width / 2, y: box.top + box.height / 2 },
         function (result) {
      // A hit die never gives back less than nothing, however bad the
      // constitution behind it.
      var got = Math.max(0, result.total);
      REST.spent += 1;
      REST.healed += got;
      REST.rolls.push(got);
      renderRest();
    });
  }

  function takeRest() {
    if (!restConfigured()) { return; }
    REST.busy = true;
    REST.error = '';
    renderRest();
    api('POST', '/dnd/rest', restWho({
      kind: REST.kind, hitDiceSpent: REST.spent,
      hitPointsHealed: REST.healed, note: REST.note
    })).then(function (state) {
      REST.busy = false;
      REST.plans = state;
      REST.done = state.result;
      REST.plan = state[REST.kind] || REST.plan;
      applyRestToSheet(state.result, REST.plan);
      // The night's log gets the rest written into it as one note, which is
      // what a rest is at the table: one thing that happened.
      logNote((state.result.lines || []).join('\n'));
      renderRest();
    }, function (err) {
      REST.busy = false;
      REST.error = err.message || String(err);
      renderRest();
    });
  }

  // ------------------------------------------------------------- drawing it
  function restStepHtml(step) {
    var html = '<h4>' + esc(step.title || '') + '</h4>' +
      '<p>' + esc(step.text || '') + '</p>';
    if (step.kind !== 'hitdice') { return html; }
    if (!step.max) {
      return html + '<p class="rest-msg">You have no hit dice left to spend.</p>';
    }
    var pips = '';
    for (var i = 1; i <= step.max; i++) {
      pips += '<span class="rest-die' + (i <= REST.spent ? ' spent' : '') + '"></span>';
    }
    var plan = REST.plan || {};
    var hp = Math.min(plan.hpMax || 0, (plan.hpCurrent || 0) + REST.healed);
    return html +
      '<div class="rest-hp"><b>' + hp + '</b> of ' + (plan.hpMax || 0) + ' hit points' +
        (REST.healed ? ' &middot; ' + REST.healed + ' regained' : '') + '</div>' +
      '<div class="rest-dice">' + pips +
        '<button type="button" class="rest-roll" id="rest-roll"' +
          (REST.spent >= step.max ? ' disabled' : '') + '>Roll ' + esc(step.die) +
          (step.mod ? ' ' + signed(step.mod) : '') + '</button>' +
      '</div>' +
      (REST.rolls.length
        ? '<p class="rest-log">Rolled ' + REST.rolls.join(', ') + '.</p>'
        : '');
  }

  function restDoneHtml() {
    var r = REST.done, back = [];
    (r.featuresBack || []).forEach(function (f) { back.push(esc(f)); });
    if (r.diceBack) {
      back.push(r.diceBack + ' hit di' + (r.diceBack === 1 ? 'e' : 'ce'));
    }
    if (r.slotsBack) {
      back.push(r.slotsBack + ' spell slot' + (r.slotsBack === 1 ? '' : 's'));
    }
    return '<h4>' + esc(r.name) + ' taken</h4>' +
      '<div class="rest-hp"><b>' + r.hpAfter + '</b> of ' + r.hpMax + ' hit points' +
        (r.healed ? ' &middot; ' + r.healed + ' regained' : '') + '</div>' +
      (r.diceSpent ? '<p class="rest-log">Spent ' + r.diceSpent + ' hit di' +
        (r.diceSpent === 1 ? 'e' : 'ce') + '.</p>' : '') +
      (back.length
        ? '<p class="rest-msg">Recovered:</p><ul class="rest-back-list"><li>' +
            back.join('</li><li>') + '</li></ul>'
        : '<p class="rest-msg">Nothing was spent that this rest gives back.</p>') +
      (LOG.session
        ? '<p class="rest-msg">Written to ' + esc(LOG.session.name) + '.</p>'
        : '<p class="rest-msg">No session is recording, so this went to the ' +
          'character sheet only.</p>');
  }

  function renderRest() {
    if (!restModal) { return; }
    var title = restModal.querySelector('.rest-title');
    title.textContent = REST.kind === 'short' ? 'Short Rest' : 'Long Rest';

    if (REST.error && !REST.plan) {
      restDots.innerHTML = '';
      restBody.innerHTML = '<p class="rest-err">' + esc(REST.error) + '</p>';
      restFoot.innerHTML = '<span class="grow"></span>' +
        '<button type="button" class="rest-roll" id="rest-cancel">Close</button>';
      return;
    }
    if (!REST.plan) {
      restDots.innerHTML = '';
      restBody.innerHTML = '<p class="rest-msg">Working out what this rest ' +
        'would give you&hellip;</p>';
      restFoot.innerHTML = '';
      return;
    }

    var steps = restSteps();
    if (REST.done) {
      restDots.innerHTML = '';
      restBody.innerHTML = restDoneHtml();
      restFoot.innerHTML = '<span class="grow"></span>' +
        '<button type="button" class="rest-roll" id="rest-cancel">Done</button>';
      return;
    }

    var dots = '';
    for (var i = 0; i < steps.length; i++) {
      dots += '<i class="' + (i <= REST.step ? 'on' : '') + '"></i>';
    }
    restDots.innerHTML = dots;

    var step = steps[REST.step] || {};
    var last = REST.step >= steps.length - 1;
    restBody.innerHTML = restStepHtml(step) +
      (last
        ? '<label class="rest-msg" for="rest-note">A line for the session log, ' +
            'if you want one</label>' +
          '<input type="text" class="rest-note" id="rest-note" ' +
            'placeholder="Camped in the ruins" value="' + esc(REST.note) + '">'
        : '');

    restFoot.innerHTML =
      (REST.step > 0
        ? '<button type="button" class="rest-roll" id="rest-back">Back</button>'
        : '') +
      '<span class="grow">' +
        (REST.error ? '<span class="rest-err">' + esc(REST.error) + '</span>' :
         REST.busy ? '<span class="rest-msg">saving&hellip;</span>' : '') +
      '</span>' +
      '<button type="button" class="rest-roll" id="rest-next"' +
        (REST.busy ? ' disabled' : '') + '>' +
        (last ? 'Take the ' + (REST.kind === 'short' ? 'short' : 'long') + ' rest'
              : 'Next') +
      '</button>';
  }

  function buildRestUi() {
    buildFeatureUses();

    restModal = document.createElement('div');
    restModal.className = 'rest-modal';
    restModal.id = 'dnd-rest';
    restModal.innerHTML =
      '<div class="rest-card" role="dialog" aria-label="Take a rest">' +
        '<h3><span class="rest-title">Rest</span>' +
          '<button type="button" class="x" id="rest-close" aria-label="Close">' +
          '&times;</button></h3>' +
        '<div class="rest-dots"></div>' +
        '<div class="rest-body"></div>' +
        '<div class="rest-foot"></div>' +
      '</div>';
    document.body.appendChild(restModal);
    restDots = restModal.querySelector('.rest-dots');
    restBody = restModal.querySelector('.rest-body');
    restFoot = restModal.querySelector('.rest-foot');

    restModal.addEventListener('click', function (e) {
      if (e.target === restModal || e.target.closest('#rest-close') ||
          e.target.closest('#rest-cancel')) {
        closeRest();
        return;
      }
      if (e.target.closest('#rest-roll')) {
        rollHitDie(e.target.closest('#rest-roll'));
        return;
      }
      if (e.target.closest('#rest-back')) {
        REST.step = Math.max(0, REST.step - 1);
        renderRest();
        return;
      }
      if (e.target.closest('#rest-next')) {
        var note = restModal.querySelector('#rest-note');
        if (note) { REST.note = note.value; }
        if (REST.step >= restSteps().length - 1) { takeRest(); }
        else { REST.step += 1; renderRest(); }
      }
    });
    restModal.addEventListener('keydown', function (e) {
      if (e.key === 'Escape') { closeRest(); }
    });
  }

  function init() {
    var canvas = document.createElement('canvas');
    canvas.id = 'dice-canvas';
    document.body.appendChild(canvas);
    board = new DiceBoard(canvas);
    buildTray();
    buildCustomDock();
    watchPrinting();
    buildSessionLog();
    buildInventoryUi();
    buildCoinUi();
    buildSpellUi();
    buildRestUi();
    buildDefenseUi();
    buildHealthUi();
    buildInspirationUi();
    buildSectionTabs();
    fitRingText();

    Array.prototype.forEach.call(document.querySelectorAll('[data-kind]'), prepare);
    Array.prototype.forEach.call(
      document.querySelectorAll('.feature p, details.spell p, .box .tagline, .quote'),
      linkifyProse);

    document.addEventListener('click', function (e) {
      if (e.target.closest('#dice-tray')) { historyClick(e); return; }
      var spell = e.target.closest('.cast-btn');
      if (spell) {
        // The button sits inside the spell's <summary>; casting should not
        // also fold the spell open or shut.
        e.preventDefault();
        e.stopPropagation();
        // The dice are the sheet's business and the slot is the file's: the
        // roll lands here and the slot is struck off over there, and what it
        // cost is written onto the card once the file says so.
        castSlot(spell, castRoll(castSpecFor(spell), originOf(spell, e)));
        return;
      }
      var el = e.target.closest('.rollable');
      if (!el) { return; }
      e.preventDefault();
      activate(el, e);
    });
    document.addEventListener('keydown', function (e) {
      if (e.key !== 'Enter' && e.key !== ' ') { return; }
      var el = document.activeElement;
      if (!el || !el.classList || !el.classList.contains('rollable')) { return; }
      e.preventDefault();
      activate(el, null);
    });
  }

  // ------------------------------------------------------------------ tabs
  //
  // A box marked .tabbed holds several sections of the sheet stacked one
  // under the other, each under its own heading. That stack is what the
  // exported markup says and what a sheet with no script - or a printed one -
  // shows. Here the headings are read off and turned into a bar of tabs, and
  // the box shows one section at a time.
  //
  // Nothing is written into the markup about which tabs a box has, so a
  // section the template left out for this character - no spells to cast, no
  // appearance filled in - simply has no tab, and a box left with one section
  // keeps its heading and gets no bar at all.
  //
  // A pane may carry data-tab to put something shorter on its button than its
  // heading says. The heading is still what the section is called - it is what
  // the printed sheet and the screen reader get - so only the button changes.
  var TAB_KEY = 'orgs.dnd.tabs';

  function loadTabs() {
    try { return JSON.parse(localStorage.getItem(TAB_KEY) || '{}') || {}; }
    catch (e) { return {}; }
  }

  function saveTab(group, name) {
    if (!group) { return; }
    var all = loadTabs();
    all[group] = name;
    try { localStorage.setItem(TAB_KEY, JSON.stringify(all)); }
    catch (e) { /* private browsing, nothing worth doing about it */ }
  }

  function buildSectionTabs() {
    var saved = loadTabs();
    Array.prototype.forEach.call(document.querySelectorAll('.box.tabbed'), function (box) {
      var group = box.getAttribute('data-tabs') || '';
      var panes = [];
      Array.prototype.forEach.call(box.children, function (el) {
        if (el.classList && el.classList.contains('tabpane')) { panes.push(el); }
      });
      if (panes.length < 2) { return; }

      var bar = document.createElement('div');
      bar.className = 'tabbar';
      bar.setAttribute('role', 'tablist');

      var labels = [];
      var names = panes.map(function (pane, i) {
        var h2 = pane.querySelector('h2');
        var name = h2 ? h2.textContent.trim() : 'Section ' + (i + 1);
        if (!pane.id) { pane.id = 'tabpane-' + group + '-' + i; }
        pane.setAttribute('role', 'tabpanel');
        pane.setAttribute('aria-label', name);
        labels.push((pane.getAttribute('data-tab') || '').trim() || name);
        return name;
      });

      // The tab last left open is the one reopened, as long as that section
      // is still on the sheet.
      var open = names.indexOf(saved[group]);
      if (open < 0) { open = 0; }

      function show(i) {
        panes.forEach(function (pane, n) { pane.classList.toggle('on', n === i); });
        Array.prototype.forEach.call(bar.children, function (btn, n) {
          btn.classList.toggle('on', n === i);
          btn.setAttribute('aria-selected', n === i ? 'true' : 'false');
          btn.tabIndex = n === i ? 0 : -1;
        });
        saveTab(group, names[i]);
      }

      names.forEach(function (name, i) {
        var btn = document.createElement('button');
        btn.type = 'button';
        btn.className = 'tabbtn';
        btn.setAttribute('role', 'tab');
        btn.setAttribute('aria-controls', panes[i].id);
        btn.textContent = labels[i];
        if (labels[i] !== name) { btn.setAttribute('aria-label', name); }
        btn.addEventListener('click', function () { show(i); });
        bar.appendChild(btn);
      });

      // Left and right walk the bar the way a tablist is expected to.
      bar.addEventListener('keydown', function (e) {
        var step = e.key === 'ArrowRight' ? 1 : (e.key === 'ArrowLeft' ? -1 : 0);
        if (!step) { return; }
        e.preventDefault();
        var at = Array.prototype.indexOf.call(bar.children, document.activeElement);
        var next = (at + step + names.length) % names.length;
        show(next);
        bar.children[next].focus();
      });

      box.insertBefore(bar, box.firstChild);
      box.classList.add('has-tabs');
      show(open);
    });
  }

  // The name curves around the top half of the portrait frame. The template
  // picks a starting size from how long the name is; this trims it further
  // until it actually fits between the two gems, so no adventurer runs off
  // the end of their own medallion.
  function fitRingText() {
    var text = document.querySelector('.portrait-ring .ring-name');
    var path = text && text.querySelector('textPath');
    if (!path || !path.getComputedTextLength) { return; }
    var room = Math.PI * 78 - 58;          // the top arc, less the gem ends
    var size = parseFloat(window.getComputedStyle(text).fontSize) || 13;
    for (var i = 0; i < 24 && size > 5.5; i++) {
      if (path.getComputedTextLength() <= room) { break; }
      size -= 0.5;
      text.style.fontSize = size + 'px';
    }
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();
</script>
</body>
</html>
