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
.char-sub { color: var(--muted); font-size: 1.05rem; margin-top: 4px; }
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

/* ---------------- layout ---------------- */
.columns {
  display: grid;
  grid-template-columns: 250px 1fr 1fr;
  gap: 18px;
  align-items: start;
}
@media (max-width: 1000px) { .columns { grid-template-columns: 1fr 1fr; } }
@media (max-width: 700px)  { .columns { grid-template-columns: 1fr; } }
.col { display: flex; flex-direction: column; gap: 16px; min-width: 0; }

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
.hp-bar {
  height: 9px; border-radius: 5px; background: #d8cbb2;
  border: 1px solid var(--line); overflow: hidden; margin-top: 6px;
}
.hp-fill { height: 100%; background: linear-gradient(90deg, #7b1b1b, #a33); }

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

/* ---------------- features & text ---------------- */
.feature { margin-bottom: 9px; }
.feature h3 {
  font-size: .82rem; margin: 0 0 1px; color: var(--accent);
}
.feature .src { font-size: .62rem; color: var(--muted); text-transform: uppercase; }
.feature p { margin: 2px 0; font-size: .86rem; white-space: pre-wrap; }
.scroller { max-height: 900px; overflow-y: auto; padding-right: 6px; }
.quote { font-style: italic; white-space: pre-wrap; margin: 0 0 8px; }
.quote .label { display: block; font-style: normal; font-size: .6rem; text-transform: uppercase; color: var(--muted); }

/* ---------------- spells ---------------- */
.spell-head { display: flex; flex-wrap: wrap; gap: 10px; margin-bottom: 10px; }
.slot-row { display: flex; align-items: center; gap: 6px; margin: 4px 0 6px; }
.slot-row .lvl { font-family: Cinzel, serif; font-size: .68rem; color: var(--muted); width: 3.2em; }
.slot { width: 12px; height: 12px; border: 1px solid var(--muted); border-radius: 3px; background: var(--paper); }
.slot.used { background: var(--muted); }
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
details.spell summary::marker { color: var(--muted); }
details.spell p { font-size: .82rem; margin: 4px 0 8px 12px; white-space: pre-wrap; }
.tagline { font-size: .68rem; color: var(--muted); }
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
  width: 27px; height: 27px;
  fill: none; stroke: currentColor;
  stroke-width: 1.35; stroke-linejoin: round; stroke-linecap: round;
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
  body { background: #fff; }
  #dice-canvas, #dice-tray, #dice-tab, #dice-dock,
  #notes-drawer, #notes-tab { display: none !important; }
  .rollable {
    text-decoration: none !important;
    box-shadow: none !important;
    background: none !important;
  }
  .page { box-shadow: none; max-width: none; padding: 0; }
  .scroller { max-height: none; overflow: visible; }
  .box { break-inside: avoid; }
  details.spell { break-inside: avoid; }
  details.spell[open] summary ~ * { display: block; }
}
</style>
</head>
<body>
<div class="page">

  <header class="sheet-head">
    <div class="name-block">
      <h1 class="char-name">{{ sheet.name }}</h1>
      <div class="char-sub">{{ sheet.classLine }} &middot; {{ sheet.raceName }}{% if sheet.background %} &middot; {{ sheet.background }}{% endif %}</div>
    </div>
    <div class="head-facts">
      <div class="fact"><span class="label">Level</span><span class="value">{{ sheet.level }}</span></div>
      <div class="fact"><span class="label">Proficiency</span><span class="value">{{ sheet.proficiencyStr }}</span></div>
      {% if sheet.alignment %}<div class="fact"><span class="label">Alignment</span><span class="value">{{ sheet.alignment }}</span></div>{% endif %}
      <div class="fact"><span class="label">Experience</span><span class="value">{{ sheet.xp }}{% if sheet.nextLevelXp %} / {{ sheet.nextLevelXp }}{% endif %}</span></div>
      {% if sheet.player %}<div class="fact"><span class="label">Player</span><span class="value">{{ sheet.player }}</span></div>{% endif %}
      <div class="fact"><span class="label">Inspiration</span><span class="value">{% if sheet.inspiration %}Yes{% else %}&mdash;{% endif %}</span></div>
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
      <div class="box">
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
        <div style="margin-top:10px">
          <div class="row" style="border:0">
            <span class="nm"><strong>Hit Points</strong> {{ sheet.hpCurrent }} / {{ sheet.hpMax }}{% if sheet.hpTemp %} (+{{ sheet.hpTemp }} temp){% endif %}</span>
            {% if sheet.hitDice %}<span class="val rollable" data-kind="hitdie"
                  data-pool="{{ sheet.hitDice }}" data-mod="{{ sheet.abilityMap.con.mod }}"
                  data-label="Hit Die">{{ sheet.hitDice }}</span>{% endif %}
          </div>
          <div class="hp-bar"><div class="hp-fill" style="width:{{ sheet.hpPercent }}%"></div></div>
          <div class="tagline" style="margin-top:4px">
            Hit dice {{ sheet.hitDice }}{% if sheet.hitDiceUsed %}, {{ sheet.hitDiceUsed }} spent{% endif %} &middot;
            <span class="rollable" data-kind="check" data-mod="{{ sheet.deathSaveStr }}"
                  data-label="Death Saving Throw">death saves {{ sheet.deathSaves }}{% if sheet.deathSaveBonus %}
                  ({{ sheet.deathSaveStr }}){% endif %}</span> &middot;
            carrying {{ sheet.weight }} of {{ sheet.carryCapacity }} lb
          </div>
        </div>
      </div>

      <div class="box">
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

      <div class="box">
        <h2>Equipment</h2>
        <div class="table-wrap">
        <table>
          <thead><tr><th>Item</th><th class="num">Qty</th><th class="num">Wt</th><th>Worn</th></tr></thead>
          <tbody>
          {% for g in sheet.equipment %}
            <tr>
              <td>{{ g.name }}{% if g.notes %} <span class="tagline">({{ g.notes }})</span>{% endif %}</td>
              <td class="num">{{ g.qty }}</td>
              <td class="num">{% if g.weight %}{{ g.weight }}{% endif %}</td>
              <td>{% if g.equipped %}&#10003;{% endif %}</td>
            </tr>
          {% endfor %}
          </tbody>
        </table>
        </div>
        <div class="tagline" style="margin-top:6px">
          {{ sheet.money.cp }} cp &middot; {{ sheet.money.sp }} sp &middot; {{ sheet.money.ep }} ep &middot;
          {{ sheet.money.gp }} gp &middot; {{ sheet.money.pp }} pp
        </div>
      </div>

      {% if sheet.personality or sheet.ideals or sheet.bonds or sheet.flaws %}
      <div class="box">
        <h2>Personality</h2>
        {% if sheet.personality %}<p class="quote"><span class="label">Traits</span>{{ sheet.personality }}</p>{% endif %}
        {% if sheet.ideals %}<p class="quote"><span class="label">Ideals</span>{{ sheet.ideals }}</p>{% endif %}
        {% if sheet.bonds %}<p class="quote"><span class="label">Bonds</span>{{ sheet.bonds }}</p>{% endif %}
        {% if sheet.flaws %}<p class="quote"><span class="label">Flaws</span>{{ sheet.flaws }}</p>{% endif %}
      </div>
      {% endif %}

      {% if sheet.age or sheet.height or sheet.weightStr or sheet.eyes or sheet.skin or sheet.hair or sheet.appearance %}
      <div class="box">
        <h2>Appearance</h2>
        <div class="rows">
          {% if sheet.age %}<div class="row"><span class="nm">Age</span><span class="val">{{ sheet.age }}</span></div>{% endif %}
          {% if sheet.height %}<div class="row"><span class="nm">Height</span><span class="val">{{ sheet.height }}</span></div>{% endif %}
          {% if sheet.weightStr %}<div class="row"><span class="nm">Weight</span><span class="val">{{ sheet.weightStr }}</span></div>{% endif %}
          {% if sheet.eyes %}<div class="row"><span class="nm">Eyes</span><span class="val">{{ sheet.eyes }}</span></div>{% endif %}
          {% if sheet.skin %}<div class="row"><span class="nm">Skin</span><span class="val">{{ sheet.skin }}</span></div>{% endif %}
          {% if sheet.hair %}<div class="row"><span class="nm">Hair</span><span class="val">{{ sheet.hair }}</span></div>{% endif %}
        </div>
        {% if sheet.appearance %}<p class="quote" style="margin-top:6px">{{ sheet.appearance }}</p>{% endif %}
      </div>
      {% endif %}
    </div>

    <!-- =============== right column =============== -->
    <div class="col">
      {% if sheet.isCaster %}
      <div class="box">
        <h2>Spellcasting</h2>
        <div class="spell-head">
          <div class="tile"><span class="tile-label">Ability</span><div class="big" style="font-size:1rem">{{ sheet.castingAbility }}</div></div>
          <div class="tile"><span class="tile-label">Save DC</span><div class="big">{{ sheet.spellSaveDc }}</div></div>
          <div class="tile rollable" data-kind="check" data-mod="{{ sheet.spellAttackStr }}"
               data-label="Spell Attack"><span class="tile-label">Attack</span><div class="big">{{ sheet.spellAttackStr }}</div></div>
        </div>
        <div class="tagline">
          {% if sheet.cantripsKnown %}{{ sheet.cantripsKnown }} cantrips{% endif %}
          {% if sheet.spellsKnown %} &middot; {{ sheet.spellsKnown }} spells known{% endif %}
          {% if sheet.preparedMax %} &middot; {{ sheet.spellsPrepared }}/{{ sheet.preparedMax }} prepared{% endif %}
          {% if sheet.spellNotes %} &middot; {{ sheet.spellNotes }}{% endif %}
        </div>
        {% for s in sheet.slots %}
        <div class="slot-row">
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
            </summary>
            <p><em>{{ sp.school }}{% if sp.components %} &middot; {{ sp.components }}{% endif %}{% if sp.duration %} &middot; {{ sp.duration }}{% endif %}</em>
{{ sp.text }}{% if sp.higherLevel %}

<strong>At higher levels.</strong> {{ sp.higherLevel }}{% endif %}</p>
          </details>
          {% endfor %}
        </div>
        {% endfor %}
      </div>
      {% endif %}

      <div class="box">
        <h2>Features &amp; Traits</h2>
        <div class="scroller">
          {% for t in sheet.traits %}
          <div class="feature">
            <h3>{{ t.name }}</h3>
            {% if t.source %}<span class="src">{{ t.source }}</span>{% endif %}
            <p>{{ t.text }}</p>
          </div>
          {% endfor %}
          {% for f in sheet.features %}
          <div class="feature">
            <h3>{{ f.name }}</h3>
            {% if f.source %}<span class="src">{{ f.source }}</span>{% endif %}
            <p>{{ f.text }}</p>
          </div>
          {% endfor %}
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
     data-character-id="{{ sheet.id }}"></div>
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

  function latestCard(r) {
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

  var D20_PLUS_ICON = '<svg viewBox="0 0 24 24" aria-hidden="true" focusable="false">' +
    '<g transform="scale(.72)">' +
    '<path d="M12 1.6 22 7.4v9.2L12 22.4 2 16.6V7.4z"/>' +
    '<path d="M12 1.6 12 22.4M2 7.4l10 3.2 10-3.2M2 16.6l10-6 10 6"/></g>' +
    '<path d="M19.4 15.6v6.5M16.2 18.8h6.5"/></svg>';

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
    fab.title = 'Custom roll';
    fab.setAttribute('aria-label', 'Custom roll');
    fab.setAttribute('aria-expanded', 'false');
    fab.innerHTML = D20_PLUS_ICON;
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

  function roll(spec, origin) {
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
    var e = new Error('sign in to the orgs server to record this session');
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

  // The api returns either a plain string or a {ok, msg} envelope on failure.
  function serverError(text) {
    if (!text) { return ''; }
    try {
      var v = JSON.parse(text);
      if (typeof v === 'string') { return v; }
      if (v && v.msg) { return v.msg; }
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

  // logRoll turns a result from the tray into a row of the session table.
  function logRoll(r) {
    if (!LOG.session) { return; }
    var row = { time: r.time, character: CHARACTER, label: r.label, formula: r.formula };
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
      el.innerHTML = '<div class="dt-empty">No sessions yet.</div>';
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

  function loadSessions() {
    var el = document.getElementById('sessions-list');
    if (el) { el.innerHTML = '<div class="dt-empty">Loading&hellip;</div>'; }
    api('GET', '/dnd/play/sessions').then(function (list) {
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

  function init() {
    var canvas = document.createElement('canvas');
    canvas.id = 'dice-canvas';
    document.body.appendChild(canvas);
    board = new DiceBoard(canvas);
    buildTray();
    buildCustomDock();
    buildSessionLog();

    Array.prototype.forEach.call(document.querySelectorAll('[data-kind]'), prepare);
    Array.prototype.forEach.call(
      document.querySelectorAll('.feature p, details.spell p, .box .tagline, .quote'),
      linkifyProse);

    document.addEventListener('click', function (e) {
      if (e.target.closest('#dice-tray')) { historyClick(e); return; }
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

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();
</script>
</body>
</html>
