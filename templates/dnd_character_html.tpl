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
  /* the corner every stat box shares - ability scores, initiative, speed */
  --stat-radius: 20px;
  /* ---- the mood ----
     How the page is feeling, which is how the halo round its edge is
     coloured and how hard the backdrop is pushed. --mood-rgb is the colour,
     --mood-a how strongly it shows, and --mood-wash is added to the
     backdrop's own opacity. All three are set from the hit points and the
     conditions by the script; these are the values for a character who is
     perfectly well, where the halo is a faint warm vignette and nothing
     more. */
  --mood-rgb: 139, 126, 102;
  --mood-a: .16;
  --mood-wash: 0;
  --mood-pulse: 0s;
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
  /* the stacking context the backdrop layers sit in, see below */
  position: relative;
  z-index: 0;
}

/* ---------------- backdrop ----------------
   The scenery behind the sheet: one or more pictures from DND_BACKDROP,
   washed out so the text still wins. There are two identical layers because
   changing picture is a crossfade - the one coming in is painted underneath,
   then the two swap opacity.

   z-index: -1 puts them behind everything in the page but still in front of
   the paper colour and its two gradients, which is why .page has to be a
   stacking context of its own: without it a negative layer would fall all the
   way behind the page and vanish. */
.backdrop {
  position: absolute;
  inset: 0;
  z-index: -1;
  pointer-events: none;
  background-position: center center;
  background-size: cover;
  background-repeat: no-repeat;
  /* The sheet is taller than the window, often much taller. Sizing the
     picture to the window instead of to the whole page keeps it from being
     blown up several times over, and it stays put while the sheet scrolls
     past it. Where a browser will not do that it simply scrolls along, which
     is the plain behaviour and no worse. */
  background-attachment: fixed;
  /* a photograph at full saturation fights the ink even at low opacity */
  filter: saturate(.55) contrast(.92);
  opacity: 0;
  transition: opacity 2.2s ease-in-out;
}
/* The backdrop is pushed a little harder as the character gets hurt, so the
   scenery closes in on them. --mood-wash is nought for a hale character, so
   this is the plain wash until something goes wrong. */
.backdrop.on { opacity: calc(var(--wash, .14) + var(--mood-wash, 0)); }

/* ---------------- the halo ----------------
   A gradient bled in from the four edges of the page, coloured by how the
   character is doing: warm and barely there while they are well, orange as
   they are worn down, red and breathing when they are nearly out, and a cold
   grey when they are down altogether. Conditions tint it too - green for
   poisoned, violet while a spell is being held.

   It is drawn as one fixed layer over the paper and under everything else, so
   it costs nothing to recolour and never moves the text about. Pointer events
   are off, so it is scenery in the strictest sense: there is nothing on the
   sheet it can get in the way of. */
.page-halo {
  position: fixed; inset: 0; z-index: 0; pointer-events: none;
  /* Four soft washes, one from each edge, plus a wider one over the whole
     window so the middle is not left conspicuously clean. */
  background:
    linear-gradient(to right, rgba(var(--mood-rgb), var(--mood-a)), transparent 22%),
    linear-gradient(to left, rgba(var(--mood-rgb), var(--mood-a)), transparent 22%),
    linear-gradient(to bottom, rgba(var(--mood-rgb), var(--mood-a)), transparent 18%),
    linear-gradient(to top, rgba(var(--mood-rgb), var(--mood-a)), transparent 18%),
    radial-gradient(ellipse 130% 100% at 50% 50%,
                    transparent 48%, rgba(var(--mood-rgb), var(--mood-a)) 100%);
  transition: background 1.6s ease-in-out;
}
/* A spell being concentrated on: a thin violet line just inside the edge, laid
   over whatever the hit points are already saying. It is drawn as an inset
   shadow rather than a fifth gradient so it reads as a line and not as more
   wash. */
.page-halo.holding {
  box-shadow: inset 0 0 0 2px rgba(106, 87, 168, .22),
              inset 0 0 26px rgba(106, 87, 168, .1);
}
/* Nearly out of hit points: the edge breathes, slowly, the way a held breath
   does. It is deliberately long and shallow - a sheet that flashed at the
   player would be unreadable for the whole fight. */
.page-halo.breathing { animation: halo-breathe 4.2s ease-in-out infinite; }
@keyframes halo-breathe {
  0%, 100% { opacity: .72; }
  50% { opacity: 1; }
}
/* A hit landing, a critical, a natural 1: one swell of the halo and gone.
   The class is taken off again by the script so it can be replayed. */
.page-halo.struck { animation: halo-strike .62s ease-out 1; }
@keyframes halo-strike {
  0% { opacity: 1; transform: scale(1); }
  22% { opacity: 1.6; transform: scale(1.04); }
  100% { opacity: 1; transform: scale(1); }
}
.page-halo.crit { animation: halo-crit 1s ease-out 1; }
@keyframes halo-crit {
  0% { opacity: 1; }
  18% { opacity: 1.9; }
  100% { opacity: 1; }
}
@media (prefers-reduced-motion: reduce) {
  .page-halo, .page-halo.breathing, .page-halo.struck, .page-halo.crit {
    animation: none; transition: none;
  }
  .conc .conc-mark, .conc.owed { animation: none; }
}
@media print { .page-halo { display: none; } }

/* ---------------- panels over a backdrop ----------------
   The panels holding the sheet's content - ability scores, saving throws,
   skills, combat and the rest - go a little translucent so the scenery shows
   through them. Simply thinning their paper would not quite do it: what
   little of the picture survives a second layer of cream is next to nothing.
   So each panel paints the picture itself, pinned to the window the way the
   page's own backdrop is - background-attachment: fixed sizes both to the
   viewport, so the piece of the picture inside a panel is exactly the piece
   that would be behind it, and the panels read as glass over the same scene
   rather than as patches of a different one.

   --glass-a is how much paper is over that picture, set from the sheet's own
   wash so a panel always shows a little less of it than the bare page does.
   The stat boxes sitting on the panels stay solid, so every number on the
   sheet still has plain paper behind it. */
.has-backdrop .box {
  background-color: transparent;
  background-image:
    linear-gradient(rgba(236, 228, 213, var(--glass-a, .94)),
                    rgba(236, 228, 213, var(--glass-a, .94))),
    var(--backdrop-img, none);
  background-size: cover;
  background-position: center center;
  background-attachment: fixed;
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
/* ---- the portrait as the fight goes on ----
   The picture drains of colour and light as the hit points fall, and the
   glass over it cracks. It is the one place on the sheet that says how the
   character is doing without a number, which is exactly what you want when
   you glance up from the table.

   The transition is slow on purpose - a second and a half - so that healing
   reads as relief rather than as a flicker. */
.portrait { transition: filter 1.2s ease; }
.portrait.hp-hurt     { filter: drop-shadow(0 3px 7px rgba(28,26,23,.34)) saturate(.88); }
.portrait.hp-bloodied { filter: drop-shadow(0 3px 7px rgba(28,26,23,.34)) saturate(.62) brightness(.94); }
.portrait.hp-dying    { filter: drop-shadow(0 3px 9px rgba(90,20,14,.5)) saturate(.36) brightness(.85) contrast(1.06); }
/* Down is not "hurt more". The colour goes out of it altogether, which is
   the same thing the page's own edge does - see MOODS in the mood block. */
.portrait.hp-down     { filter: drop-shadow(0 3px 9px rgba(28,26,23,.5)) grayscale(1) brightness(.78); }
.portrait.hp-dead     { filter: drop-shadow(0 2px 6px rgba(18,16,14,.6)) grayscale(1) brightness(.52) contrast(.9); }

/* The blood that creeps in under the glass at low hit points, and the grey
   that replaces it once the character is down. */
.portrait-well::before {
  content: "";
  position: absolute; inset: 0;
  border-radius: 50%;
  background: radial-gradient(circle at 50% 118%, rgba(var(--por-rgb, 150,40,30), .55) 0%,
              rgba(var(--por-rgb, 150,40,30), 0) 62%);
  opacity: 0;
  transition: opacity 1.2s ease;
  pointer-events: none; z-index: 1;
}
.portrait.hp-bloodied .portrait-well::before { opacity: .5; }
.portrait.hp-dying .portrait-well::before { opacity: .85; }
.portrait.hp-down .portrait-well::before,
.portrait.hp-dead .portrait-well::before {
  --por-rgb: 92, 88, 84;
  opacity: .9;
}

.portrait-cracks .crack {
  fill: none;
  stroke: rgba(250, 246, 238, .78);
  stroke-width: 1.5;
  stroke-linecap: round; stroke-linejoin: round;
  opacity: 0;
  transition: opacity .7s ease;
  filter: drop-shadow(0 1px 0 rgba(28,26,23,.55));
}
.portrait-cracks .twig { stroke-width: .9; }
.portrait.hp-bloodied .crack.c1 { opacity: .55; }
.portrait.hp-dying .crack.c1 { opacity: .8; }
.portrait.hp-dying .crack.c2 { opacity: .6; }
.portrait.hp-down .crack.c1,
.portrait.hp-down .crack.c2 { opacity: .85; }
.portrait.hp-down .crack.c3 { opacity: .7; }
.portrait.hp-dead .crack { opacity: .95; }

/* Inspiration is the one thing that puts light back into it. */
.portrait.inspired { filter: drop-shadow(0 0 10px rgba(212,168,60,.75))
                             drop-shadow(0 3px 7px rgba(28,26,23,.34)); }
.portrait.inspired .portrait-well::after {
  box-shadow: inset 0 0 15px rgba(212,168,60,.45), inset 0 0 3px rgba(240,205,120,.9);
}
@media (prefers-reduced-motion: reduce) {
  .portrait, .portrait-well::before, .portrait-cracks .crack { transition: none; }
}
@media print {
  .portrait { filter: none !important; }
  .portrait-cracks { display: none; }
  .portrait-well::before { display: none; }
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
  border-radius: var(--stat-radius);
  text-align: center;
  padding: 8px 2px 4px;
  position: relative;
}
.ability .tile-label { font-size: .6rem; text-transform: uppercase; color: var(--muted); }
.ability .mod { font-size: 1.55rem; font-weight: 700; line-height: 1.05; }
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
  border-radius: var(--stat-radius);
  text-align: center;
  padding: 6px 4px;
}
.tile .tile-label { display: block; font-size: .58rem; text-transform: uppercase; color: var(--muted); }
.tile .big { font-size: 1.5rem; font-weight: 700; line-height: 1.15; }
.tile .sub { font-size: .68rem; color: var(--muted); }
.tile.shield {
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
  position: relative;
  flex: 1 1 auto; min-width: 0;
  height: 9px; border-radius: 5px; background: #d8cbb2;
  border: 1px solid var(--line); overflow: hidden;
}
/* Temporary hit points are hit points: they are spent before the real ones
   and they are what stands between you and the floor, so they are drawn on
   the bar in front of what you have rather than being left as a number
   somewhere else. Hatched and in the second colour, because they are not
   yours to keep - a rest takes them away again. */
.hp-temp-fill {
  position: absolute; top: 0; bottom: 0;
  background: repeating-linear-gradient(135deg,
    var(--accent-2) 0 3px, rgba(255,255,255,.45) 3px 6px);
  opacity: .85;
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
/* Del is the one that leaves no trace, so it is the one that looks like a
   mistake being rubbed out rather than an action being taken. */
.ib.gone:hover { color: #7b1b1b; border-color: rgba(123,27,27,.6); }
.ib.sell:hover { color: var(--accent-2); }
/* A potion whose own text says what drinking it does carries the dice on the
   button, so the roll is visible before it is made. */
.ib.use.does { color: #5a6b2a; border-color: rgba(90,107,42,.4); }
.ib .ib-does {
  font-family: Cinzel, Georgia, serif; font-size: .62rem;
  margin-left: 4px; opacity: .85;
}

/* ---- attunement ----
   The star that says an item is attuned to, which for anything that asks for
   attunement is a button: three is all anyone gets, and the fourth does
   nothing, so swapping one for another has to be possible without editing the
   equipment table by hand. */
.attunebtn {
  font: inherit; font-size: .78rem; line-height: 1;
  border: 1px solid transparent; border-radius: 4px;
  background: none; color: var(--line);
  padding: 1px 4px; margin-left: 2px; cursor: pointer;
}
.attunebtn:hover { color: var(--muted); border-color: var(--line); background: #fbf7ee; }
.attunebtn.on { color: var(--accent-2); }
.attunebtn.on:hover { color: var(--accent); }
/* No slots left: still pressable - the server is the one that says no, and it
   says it in words - but it should not look like an invitation. */
.attunebtn.full { opacity: .45; }
.attunebtn:focus-visible { outline: 2px solid var(--accent-2); outline-offset: -1px; }

/* the counter over the bag */
.att {
  display: flex; align-items: center; gap: 7px; flex-wrap: wrap;
  margin-top: 7px; padding-top: 6px; border-top: 1px solid var(--line);
  font-size: .72rem; color: var(--muted);
}
.att-label {
  font-family: Cinzel, Georgia, serif; font-size: .58rem;
  text-transform: uppercase; letter-spacing: .07em;
}
.att-pips { display: inline-flex; gap: 3px; }
.att-pip {
  width: 8px; height: 8px; transform: rotate(45deg);
  border: 1px solid var(--line); background: transparent;
}
.att-pip.on { background: var(--accent-2); border-color: var(--accent-2); }
.att-count { font-family: Cinzel, Georgia, serif; font-weight: 700; color: var(--ink); }
.att-items { font-style: italic; }
.att.over { color: var(--accent); }
.att.over .att-pip.on { background: var(--accent); border-color: var(--accent); }
.att-warn { color: var(--accent); }
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
  /* A button that looks like the number it is: temporary hit points are the
     one thing on the sheet you want gone in one click, since they are used
     up or slept off rather than spent a point at a time. */
  border: 1px solid transparent; border-radius: 4px;
  background: none; padding: 0 5px; line-height: 1.3; cursor: pointer;
}
.hp-temp .val:hover { border-color: var(--accent-2); background: #fbf7ee; }
.hp-temp .val:focus-visible { outline: 2px solid var(--accent-2); outline-offset: 1px; }
.hp-temp .val.none {
  color: var(--line); font-weight: 400; cursor: default;
}
.hp-temp .val.none:hover { border-color: transparent; background: none; }
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
/* Temp belongs to the same row and so wears the same button. It is the third
   thing you do to hit points, not a different kind of control. */
.hpb.temp { color: var(--accent-2); }

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

/* ---------------- death saves ----------------
   Six marks, three of each, and one of the few places on the sheet where the
   drawing matters more than the number: a player on nought hit points is
   looking at how many are left, not reading a fraction. While the character
   is up it is small print out of the way; the moment they go down the whole
   line comes forward. */
.death {
  display: flex; align-items: center; gap: 8px; flex-wrap: wrap;
  margin-top: 6px; font-size: .78rem; color: var(--muted);
}
.death-roll {
  font-family: Cinzel, Georgia, serif; font-size: .6rem;
  text-transform: uppercase; letter-spacing: .06em;
}
.death-pips { display: inline-flex; align-items: center; gap: 4px; }
.dp {
  width: 11px; height: 11px; border-radius: 50%;
  border: 1px solid var(--line); background: transparent;
  transition: background-color .18s ease, border-color .18s ease, box-shadow .18s ease;
}
.dp.win.on { background: #4a7c2f; border-color: #3f6b2a; }
.dp.lose.on { background: var(--accent); border-color: var(--accent); }
.dp-sep {
  font-size: .56rem; text-transform: uppercase; letter-spacing: .07em;
  color: var(--line); margin: 0 2px;
}
.death-word {
  font-family: Cinzel, Georgia, serif; font-size: .62rem;
  text-transform: uppercase; letter-spacing: .08em;
}
/* Down and unsettled: the marks become buttons and the line stops whispering. */
.death[data-dying="1"] {
  color: var(--ink);
  border-top: 1px solid var(--line); padding-top: 6px;
}
.death[data-dying="1"] .dp {
  width: 15px; height: 15px; cursor: pointer;
  border-color: var(--muted);
}
.death[data-dying="1"] .dp:hover {
  box-shadow: 0 0 0 2px rgba(150, 40, 30, .18);
}
.death[data-dying="1"] .death-word { color: var(--accent); }
.death.stable .death-word { color: #3f6b2a; }
.death.dead .death-word { color: var(--accent); letter-spacing: .14em; }
.death .death-do {
  font-family: Cinzel, Georgia, serif; font-size: .58rem;
  text-transform: uppercase; letter-spacing: .06em;
  border: 1px solid var(--line); border-radius: 4px;
  background: var(--paper); padding: 2px 7px; cursor: pointer; color: inherit;
}
.death .death-do:hover { border-color: var(--accent); }

/* ---------------- concentration ----------------
   The one spell a caster is holding, and the save a blow costs them. It is
   worth a band of its own rather than a line of small print: forgetting it is
   the commonest mistake at a table, and a sheet that shows it cannot be
   forgotten. */
.conc {
  display: flex; align-items: center; gap: 8px; flex-wrap: wrap;
  margin-top: 7px; padding: 5px 8px;
  border: 1px solid rgba(90, 70, 140, .45); border-radius: 5px;
  background: rgba(120, 100, 180, .09);
  font-size: .78rem;
}
.conc .conc-mark {
  width: 9px; height: 9px; border-radius: 50%;
  background: #6a57a8; flex: 0 0 auto;
  animation: conc-pulse 2.4s ease-in-out infinite;
}
@keyframes conc-pulse {
  0%, 100% { opacity: .45; transform: scale(.82); }
  50% { opacity: 1; transform: scale(1.08); }
}
.conc .conc-name {
  font-family: Cinzel, Georgia, serif; font-weight: 700; font-size: .84rem;
}
.conc .conc-sub { color: var(--muted); font-size: .68rem; }
.conc .conc-b {
  font-family: Cinzel, Georgia, serif; font-size: .58rem;
  text-transform: uppercase; letter-spacing: .06em;
  border: 1px solid var(--line); border-radius: 4px;
  background: var(--paper); padding: 3px 8px; cursor: pointer; color: inherit;
}
.conc .conc-b:hover { border-color: #6a57a8; }
.conc .conc-b.keep { color: #3f6b2a; }
.conc .conc-b.give { color: var(--accent); }
/* A save is owed: the band stops being scenery until it has been rolled. */
.conc.owed {
  border-color: var(--accent);
  background: rgba(150, 40, 30, .1);
  animation: conc-owed 1.1s ease-in-out 3;
}
@keyframes conc-owed {
  0%, 100% { box-shadow: 0 0 0 0 rgba(150, 40, 30, 0); }
  50% { box-shadow: 0 0 0 3px rgba(150, 40, 30, .18); }
}
.conc .conc-dc {
  font-family: Cinzel, Georgia, serif; font-weight: 700; color: var(--accent);
}

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
/* The line the delete modal carries, which is the whole of why it is a
   different button from Drop. */
.inv-warn {
  margin: 2px 0 8px;
  padding: 6px 9px;
  border-left: 2px solid rgba(123,27,27,.7);
  background: rgba(150,40,30,.09);
  font-size: .72rem; line-height: 1.45; color: var(--muted);
}
.inv-go.gone {
  background: linear-gradient(180deg, #b1524a, #7b1b1b);
  border-color: rgba(123,27,27,.9); color: #fdeceb;
}
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
/* display:flex beats the hidden attribute, which is how the Into row and the
   How many row stayed on screen for a Drop and a Sell that have no use for
   either. Anything hidden is hidden. */
.inv-row[hidden] { display: none; }
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
/* Buy sits beside Add and wears the gold rather than the red, because the two
   do different things: adding something is free and undone by dropping it,
   paying for it comes out of the purse. */
.inv-go.buy {
  background: var(--accent-2); border-color: var(--accent-2); color: #241f10;
}
.inv-go.buy:hover { background: #cf9a13; }
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
/* The second damage a few spells deal. It is a separate button because it is
   a separate roll: ice knife's burst lands on a saving throw the shard never
   made, and hellfire's necrotic half is a different type from its fire, so
   adding the two together would lose which half a resistant target shrugs
   off. Quieter than Cast - it is the follow up, not the spell. */
.cast2-btn {
  font: 600 .58rem/1 Cinzel, "Trajan Pro", Georgia, serif;
  letter-spacing: .1em; text-transform: uppercase;
  color: #7a6a4a; background: transparent;
  border: 1px dashed rgba(184,134,11,.5); border-radius: 4px;
  padding: 2px 6px; margin-left: 4px; cursor: pointer;
  vertical-align: middle;
}
.cast2-btn:hover { background: #fbf7ee; border-color: var(--accent); color: #4a3c20; }
.cast2-btn:focus-visible { outline: 2px solid rgba(184,134,11,.75); outline-offset: 2px; }
/* A spell whose text says what a bigger slot buys is cast at a level of your
   choosing: the slot levels this character has sit beside the cast button,
   and the one picked is what is rolled and what is struck off the file. */
.cast-at {
  float: right;
  font-family: Cinzel, Georgia, serif; font-size: .56rem;
  text-transform: uppercase; letter-spacing: .04em;
  border: 1px solid var(--line); border-radius: 3px;
  background: var(--paper); color: var(--muted);
  padding: 0 2px; margin-left: 6px; cursor: pointer; line-height: 1.6;
}
.cast-at:hover { border-color: var(--accent); color: var(--accent); }
/* Picked a level above the spell's own and the control stops being scenery. */
.cast-at.up { color: var(--accent-2); border-color: var(--accent-2); font-weight: 700; }
.cast-at option:disabled { color: var(--muted); }
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
  /* .rollable rounds its own corners for the hover halo, and it comes later
     in this sheet than the boxes do. Without this the stat boxes you can
     click - every one of them but speed - would square off to 4px. */
  border-radius: var(--stat-radius);
}
.tile.shield.rollable { border-radius: 6px 6px 26px 26px / 6px 6px 34px 34px; }
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

/* ---------------- what a blow looks like ----------------
   A hit washes the page in the colour of the damage that did it. It is one
   element and one custom property: the colour comes from the damage type,
   and which of the three animations runs comes from how that kind of damage
   feels - lightning snaps, fire blooms, cold creeps in and stays. */
#hit-wash {
  position: fixed; inset: 0;
  pointer-events: none; opacity: 0; z-index: 4;
  background:
    radial-gradient(ellipse at 50% 50%, rgba(var(--hit-rgb), 0) 34%,
                    rgba(var(--hit-rgb), .30) 78%, rgba(var(--hit-rgb), .62) 100%);
}
#hit-wash.snap  { animation: hit-snap .46s ease-out 1; }
#hit-wash.bloom { animation: hit-bloom .92s ease-out 1; }
#hit-wash.creep { animation: hit-creep 1.5s ease-out 1; }
@keyframes hit-snap {
  0% { opacity: 0; }
  8% { opacity: 1; }
  26% { opacity: .15; }
  42% { opacity: .85; }
  100% { opacity: 0; }
}
@keyframes hit-bloom {
  0% { opacity: 0; transform: scale(1.06); }
  18% { opacity: 1; transform: scale(1); }
  100% { opacity: 0; transform: scale(1); }
}
@keyframes hit-creep {
  0% { opacity: 0; }
  30% { opacity: .9; }
  62% { opacity: .75; }
  100% { opacity: 0; }
}
/* A blow that lands with weight rattles the page rather than only colouring
   it: something hit you, and the paper is on the table it hit. */
.page.jolted { animation: page-jolt .34s ease-out 1; }
@keyframes page-jolt {
  0%, 100% { transform: translate(0, 0); }
  18% { transform: translate(-5px, 3px); }
  42% { transform: translate(4px, -2px); }
  68% { transform: translate(-2px, 1px); }
}
/* And the hit point bar flinches, so the number you are watching is the one
   that moves. */
.hp-gauge.flinch { animation: hp-flinch .4s ease-out 1; }
@keyframes hp-flinch {
  0%, 100% { transform: translateX(0); }
  25% { transform: translateX(-4px); }
  55% { transform: translateX(3px); }
  80% { transform: translateX(-1px); }
}
@media (prefers-reduced-motion: reduce) {
  #hit-wash.snap, #hit-wash.bloom, #hit-wash.creep,
  .page.jolted, .hp-gauge.flinch { animation: none; }
}
@media print { #hit-wash { display: none; } }

.hp-ctl .hp-type {
  flex: 0 0 auto; max-width: 8.2em;
  font-size: .68rem; padding: 3px 4px;
  color: var(--muted);
}

/* ---------------- command palette ----------------
   One box over the middle of the sheet, near the top where a search box
   belongs. It is the only thing on the page allowed to cover the sheet
   outright, because while it is open the sheet is not what you are looking
   at. */
.cmdk {
  position: fixed; inset: 0; z-index: 150;
  display: none; align-items: flex-start; justify-content: center;
  padding: 10vh 16px 16px;
  background: rgba(28,26,23,.5);
}
.cmdk.open { display: flex; }
.cmdk-card {
  width: min(620px, 100%);
  max-height: 72vh; display: flex; flex-direction: column;
  background: var(--paper);
  border: 1px solid var(--edge); border-radius: 8px;
  box-shadow: 0 22px 60px rgba(0,0,0,.55);
  overflow: hidden;
}
.cmdk-top {
  display: flex; align-items: center; gap: 8px;
  padding: 10px 12px; border-bottom: 1px solid var(--line);
}
.cmdk-slash {
  font-family: Cinzel, Georgia, serif; font-size: 1rem;
  color: var(--accent-2); opacity: .7;
  width: 1.1em; text-align: center; flex: 0 0 auto;
}
.cmdk-in {
  flex: 1 1 auto; min-width: 0;
  font: inherit; font-size: 1.05rem;
  border: 0; background: none; color: var(--ink); padding: 2px 0;
}
.cmdk-in:focus { outline: none; }
.cmdk-in::placeholder { color: var(--line); }
.cmdk-count {
  flex: 0 0 auto; font-size: .62rem; letter-spacing: .08em;
  text-transform: uppercase; color: var(--line);
}
.cmdk-list { overflow-y: auto; padding: 5px; }
.cmdk-row {
  display: flex; align-items: baseline; gap: 8px;
  width: 100%; text-align: left;
  font: inherit; border: 0; border-radius: 4px;
  background: none; color: var(--ink);
  padding: 5px 8px; cursor: pointer;
}
.cmdk-row.on { background: rgba(184,134,11,.16); }
/* A feature with nothing left is still worth listing - "have I got one
   left" is half the question - but it cannot be pressed. */
.cmdk-row.dead { opacity: .45; cursor: default; }
.cmdk-kind {
  flex: 0 0 4.6em;
  font-family: Cinzel, Georgia, serif; font-size: .58rem;
  text-transform: uppercase; letter-spacing: .09em;
  color: var(--muted);
}
.cmdk-kind.cast { color: var(--accent-2); }
.cmdk-kind.attack { color: var(--accent); }
.cmdk-kind.use { color: #5a6b2a; }
.cmdk-kind.do { color: var(--edge); }
.cmdk-name { flex: 0 1 auto; font-size: .92rem; }
.cmdk-hint {
  flex: 1 1 auto; min-width: 0;
  font-size: .68rem; color: var(--muted);
  text-align: right; white-space: nowrap;
  overflow: hidden; text-overflow: ellipsis;
}
.cmdk-omen { flex: 0 0 auto; display: inline-flex; }
.cmdk-omen svg { width: 9px; height: 9px; }
.cmdk-omen svg path { fill: currentColor; stroke: currentColor; }
.cmdk-omen.fail svg path, .cmdk-omen.flat svg path { fill: none; }
.cmdk-omen.down { color: var(--accent); }
.cmdk-omen.up { color: #3f6b2a; }
.cmdk-omen.fail { color: #7b1b1b; }
.cmdk-omen.flat { color: var(--line); }
.cmdk-empty, .cmdk-tip {
  padding: 10px 12px; font-size: .68rem; color: var(--muted);
}
.cmdk-tip { border-top: 1px solid var(--line); letter-spacing: .04em; }
/* The hit point box lights up for a moment when the palette sends you to
   it, so it is obvious where the cursor just went. */
.hp-ctl.asking .hp-amt {
  border-color: var(--accent-2);
  box-shadow: 0 0 0 3px rgba(184,134,11,.22);
}
@media print { .cmdk { display: none !important; } }

/* ---------------- the omen ----------------
   The mark that follows the mouse over a roll a condition has an opinion
   about. It is deliberately tiny and always beside the cursor rather than
   under it: the whole point is to warn without hiding the thing being
   pointed at. */
#roll-omen {
  position: fixed; left: 0; top: 0;
  display: none; align-items: center; gap: 4px;
  padding: 2px 7px 2px 5px;
  border: 1px solid var(--line); border-radius: 11px;
  background: rgba(252,248,238,.97);
  box-shadow: 0 2px 7px rgba(60,48,28,.22);
  font-family: Cinzel, Georgia, serif;
  font-size: .62rem; letter-spacing: .05em; text-transform: uppercase;
  white-space: nowrap; pointer-events: none;
  z-index: 120;
}
#roll-omen.on { display: inline-flex; }
#roll-omen svg { width: 11px; height: 11px; display: block; flex: 0 0 auto; }
#roll-omen svg path { fill: currentColor; stroke: currentColor; }
#roll-omen.fail svg path, #roll-omen.flat svg path { fill: none; }
#roll-omen.down { color: var(--accent); border-color: rgba(150,40,30,.45); }
#roll-omen.up   { color: #3f6b2a; border-color: rgba(63,107,42,.45); }
#roll-omen.fail { color: #7b1b1b; border-color: rgba(123,27,27,.6);
                  background: rgba(255,240,236,.97); }
#roll-omen.flat { color: var(--muted); }

/* The same three states again on the roll card, where there is room to say
   which conditions did it and what the rules actually say. */
.rc-advice {
  display: flex; align-items: flex-start; gap: 6px;
  margin: 7px 0 2px; padding: 5px 8px;
  border-left: 2px solid currentColor; border-radius: 3px;
  background: rgba(255,255,255,.05);
  font-size: .64rem; line-height: 1.45; letter-spacing: .02em;
  color: #9d9078;
}
.rc-advice svg { width: 11px; height: 11px; flex: 0 0 auto; margin-top: 2px; }
.rc-advice svg path { fill: currentColor; stroke: currentColor; }
.rc-advice.fails svg path, .rc-advice.cancels svg path { fill: none; }
.rc-advice.down  { color: #e5837a; }
.rc-advice.up    { color: #93e2a2; }
.rc-advice.fails { color: #e5837a; background: rgba(160,40,30,.16); }
.rc-advice.cancels { color: #c8b98f; }
@media print { #roll-omen { display: none !important; } }

/* ---------------- dice overlay ---------------- */
#dice-canvas {
  position: fixed;
  left: 0; top: 0;
  width: 100%; height: 100%;
  pointer-events: none;
  z-index: 60;
}
/* Coin spilt onto the sheet has a layer of its own rather than sharing the
   dice one: a roll and a payment can land at the same moment, and neither
   should clear the other off the table. */
#coin-canvas {
  position: fixed;
  left: 0; top: 0;
  width: 100%; height: 100%;
  pointer-events: none;
  z-index: 59;
}
/* And a spell going off has a third, for the same reason: casting something
   that rolls to hit would otherwise have the dice clear the flourish away
   the moment they land. */
#spell-canvas {
  position: fixed;
  left: 0; top: 0;
  width: 100%; height: 100%;
  pointer-events: none;
  z-index: 61;
}

/* ---------------- level up tray ----------------
   The same drawer the dice come out of, for the one thing on the sheet that
   is a conversation rather than a button: the rules ask a handful of
   questions when a character gains a level, and this is where they get
   asked. It is the command line's levelup walk, in a tray. */
:root { --lvl-w: 430px; }
@media (max-width: 520px) { :root { --lvl-w: 92vw; } }

#lvl-tab {
  position: fixed;
  right: 0; top: 62%;
  z-index: 95;
  display: flex; flex-direction: column; align-items: center; gap: 7px;
  padding: 13px 7px;
  background: linear-gradient(180deg, #2c2620, #1a1713);
  color: var(--accent-2);
  border: 1px solid rgba(184,134,11,.55); border-right: 0;
  border-radius: 10px 0 0 10px;
  cursor: pointer;
  transform: translateY(-50%);
  transition: transform .28s cubic-bezier(.2,.8,.3,1), background .2s;
  font: 700 .58rem/1 Cinzel, "Trajan Pro", Georgia, serif;
  letter-spacing: .16em; text-transform: uppercase;
}
#lvl-tab span { writing-mode: vertical-rl; }
#lvl-tab svg { width: 15px; height: 15px; fill: none; stroke: currentColor; stroke-width: 1.7; }
#lvl-tab:hover { background: linear-gradient(180deg, #3a3227, #211c17); }
#lvl-tab.shifted { transform: translateY(-50%) translateX(calc(-1 * var(--lvl-w))); }
/* Something new to spend is worth noticing without opening the drawer. */
#lvl-tab.ready { color: #f6e3a4; box-shadow: -3px 0 16px rgba(212,168,60,.4); }

#lvl-tray {
  position: fixed;
  top: 0; right: 0; bottom: 0;
  width: var(--lvl-w);
  z-index: 92;
  display: flex; flex-direction: column;
  background: linear-gradient(180deg, #241f1a, #17140f);
  border-left: 3px double var(--accent-2);
  box-shadow: -16px 0 44px rgba(0,0,0,.55);
  color: #ece2cd;
  font-family: "EB Garamond", Palatino, Georgia, serif;
  transform: translateX(103%);
  transition: transform .28s cubic-bezier(.2,.8,.3,1);
}
#lvl-tray.open { transform: none; }
.lvl-head {
  display: flex; align-items: baseline; gap: 8px;
  padding: 12px 14px 10px;
  border-bottom: 1px solid rgba(184,134,11,.35);
}
.lvl-head h2 {
  margin: 0; font: 700 .78rem/1 Cinzel, "Trajan Pro", Georgia, serif;
  letter-spacing: .16em; text-transform: uppercase; color: var(--accent-2);
}
.lvl-head .lvl-who { font-size: .72rem; color: #9d9078; flex: 1; }
.lvl-body { flex: 1; min-height: 0; overflow-y: auto; padding: 12px 14px 16px; }
.lvl-foot {
  padding: 10px 14px 13px; border-top: 1px solid rgba(184,134,11,.3);
  display: flex; align-items: center; gap: 8px;
}
.lvl-foot .lvl-spacer { flex: 1; }

/* The climb itself: which class, how far, and then one question at a time. */
.lvl-arc {
  display: flex; align-items: baseline; gap: 8px; margin-bottom: 12px;
  font-family: Cinzel, "Trajan Pro", Georgia, serif;
}
.lvl-arc b { font-size: 1.5rem; color: #f4e6c2; }
.lvl-arc .to { font-size: 1.5rem; color: var(--accent-2); }
.lvl-arc .cls { font-size: .78rem; letter-spacing: .1em; text-transform: uppercase;
                color: #9d9078; }

/* ------------------------------------------------------------- timeline
   A spine down the left with the clock outside it and the cards hung off
   the right. The spine is a gradient rather than a line so it fades in at
   the top and out at the bottom - the evening carries on either side of
   what is written down. */
.nd-pick { max-width: 240px; }
.tl-opt {
  display: flex; align-items: center; gap: 5px; white-space: nowrap;
  font-size: .64rem; text-transform: uppercase; letter-spacing: .08em;
  color: #9d9078; cursor: pointer;
}
.tl-opt input { accent-color: #b8860b; margin: 0; }
.tl-title {
  display: flex; align-items: baseline; gap: 10px; flex-wrap: wrap;
  padding: 2px 4px 12px;
}
.tl-title b {
  font: 700 1.05rem/1.2 Cinzel, "Trajan Pro", Georgia, serif; color: #f4e6c2;
}
.tl-title span { font-size: .76rem; color: #9d9078; font-style: italic; }
.tl { position: relative; padding: 4px 4px 4px 0; }
.tl::before {
  content: ''; position: absolute; left: 72px; top: 0; bottom: 0; width: 2px;
  background: linear-gradient(180deg, rgba(184,134,11,0), rgba(184,134,11,.45) 8%,
              rgba(184,134,11,.45) 92%, rgba(184,134,11,0));
}
.tl-row {
  position: relative; display: grid;
  grid-template-columns: 52px 22px 1fr; align-items: start;
  gap: 0 10px; margin: 0 0 14px;
}
.tl-row > time {
  font: 600 .66rem/1.7 ui-monospace, SFMono-Regular, Menlo, monospace;
  color: #8d8368; text-align: right; padding-top: 5px; letter-spacing: .04em;
}
.tl-dot {
  position: relative; z-index: 1; width: 22px; height: 22px; margin-top: 2px;
  border-radius: 50%; display: grid; place-items: center;
  background: #221f1a; box-shadow: 0 0 0 2px rgba(184,134,11,.4);
}
.tl-dot svg { width: 12px; height: 12px; fill: #b8860b; }
.tl-row.fight .tl-dot { box-shadow: 0 0 0 2px rgba(196,78,42,.7); }
.tl-row.fight .tl-dot svg { fill: #e07a4e; }
.tl-row.note .tl-dot svg { fill: #9fc3e8; }
.tl-row.note .tl-dot { box-shadow: 0 0 0 2px rgba(120,160,200,.45); }
.tl-row.rolls .tl-dot svg { fill: #8d8368; }
.tl-row.rolls .tl-dot { box-shadow: 0 0 0 2px rgba(140,130,100,.35); }
.tl-row.hot .tl-dot {
  box-shadow: 0 0 0 2px rgba(240,212,122,.9), 0 0 14px 2px rgba(240,212,122,.45);
}
.tl-row.hot .tl-dot svg { fill: #ffe9a8; }
.tl-row.cold .tl-dot { box-shadow: 0 0 0 2px rgba(120,190,150,.6); }
.tl-card {
  background: linear-gradient(180deg, rgba(255,248,232,.055), rgba(255,248,232,.02));
  border: 1px solid rgba(184,134,11,.22); border-left-width: 3px;
  border-radius: 6px; padding: 9px 11px;
}
.tl-row.fight .tl-card { border-left-color: rgba(196,78,42,.65); }
.tl-row.note .tl-card { border-left-color: rgba(120,160,200,.5); }
.tl-row.hot .tl-card { border-left-color: #f0d47a; }
.tl-row.cold .tl-card { border-left-color: #6fae8a; }
.tl-head {
  display: flex; align-items: baseline; gap: 9px; flex-wrap: wrap;
  margin-bottom: 6px;
}
.tl-head b {
  font: 700 .74rem/1.3 Cinzel, "Trajan Pro", Georgia, serif;
  letter-spacing: .1em; text-transform: uppercase; color: #f4e6c2;
}
.tl-span { font-size: .68rem; color: #8d8368; }
.tl-chips { display: flex; flex-wrap: wrap; gap: 5px; }
.tl-chip {
  font-size: .66rem; letter-spacing: .05em; color: #9d9078;
  background: rgba(0,0,0,.24); border: 1px solid rgba(184,134,11,.22);
  border-radius: 999px; padding: 1px 8px;
}
.tl-chip b { color: #d8caa8; font-weight: 700; }
.tl-chip.hurt b { color: #e0845e; }
.tl-chip.crit { border-color: rgba(240,212,122,.55); }
.tl-chip.crit b { color: #f0d47a; }
.tl-chip.fumble { border-color: rgba(150,180,160,.4); }
.tl-chip.quiet { opacity: .8; }
.tl-best { font-size: .74rem; color: #9d9078; margin-top: 7px; }
.tl-best b { color: #d8caa8; }
.tl-spells { display: flex; flex-wrap: wrap; gap: 4px; margin-top: 7px; }
.tl-spells span {
  font-size: .66rem; font-style: italic; color: #c7b4e8;
  border-bottom: 1px dotted rgba(199,180,232,.4);
}
.tl-spells .more { color: #8d8368; font-style: normal; border: 0; }
.tl-beats { margin-top: 9px; border-top: 1px dashed rgba(184,134,11,.2); padding-top: 7px; }
.tl-beat { display: grid; grid-template-columns: 40px 1fr; gap: 8px; margin-top: 5px; }
.tl-beat time {
  font: 600 .62rem/1.8 ui-monospace, SFMono-Regular, Menlo, monospace; color: #8d8368;
}
.tl-beat .org-rich { font-size: .78rem; }
.tl-more {
  margin-top: 8px; background: none; border: 0; padding: 0; cursor: pointer;
  font: inherit; font-size: .64rem; letter-spacing: .08em; text-transform: uppercase;
  color: #8d8368; border-bottom: 1px dotted rgba(184,134,11,.4);
}
.tl-more:hover { color: var(--accent-2); }
.tl-one { display: flex; align-items: baseline; gap: 9px; }
.tl-one .nm { font-size: .82rem; color: #f4e6c2; }
.tl-one .fm { flex: 1; font-size: .68rem; color: #8d8368; }
.tl-one b {
  font: 700 .9rem/1 Cinzel, "Trajan Pro", Georgia, serif; color: #d8caa8;
}
.tl-blow { margin-top: 8px; }
.tl-line {
  display: grid; grid-template-columns: 40px 1fr auto 42px; gap: 8px;
  align-items: baseline; font-size: .72rem; padding: 2px 0;
  border-bottom: 1px solid rgba(184,134,11,.08);
}
.tl-line time {
  font: 600 .62rem/1.8 ui-monospace, SFMono-Regular, Menlo, monospace; color: #8d8368;
}
.tl-line .nm { color: #d8caa8; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.tl-line .fm { color: #8d8368; font-size: .66rem; }
.tl-line b { text-align: right; color: #f4e6c2; }
.tl-line.crit b { color: #f0d47a; }
.tl-line.fumble b { color: #8fbd9c; }
.tl-gap {
  display: grid; grid-template-columns: 52px 22px 1fr; gap: 0 10px;
  margin: 0 0 14px;
}
.tl-gap span {
  grid-column: 3; font-size: .64rem; letter-spacing: .12em; text-transform: uppercase;
  color: #6f6754; padding: 3px 0;
  border-top: 1px dashed rgba(184,134,11,.22);
}
.tl-end {
  display: grid; grid-template-columns: 52px 22px 1fr; gap: 0 10px;
}
.tl-end span {
  grid-column: 3; font-size: .64rem; letter-spacing: .12em; text-transform: uppercase;
  color: #6f6754;
}

/* -------------------------------------------------------- combat tracker */
/* The tracker reads as a column, not as a stripe: a name and a number
   thrown apart by nine hundred pixels of nothing is hard to pair up. */
#view-combat { max-width: 1040px; }
.cbt-bar button { flex: 0 0 auto; }
.cbt-bar {
  display: flex; align-items: center; gap: 10px; flex-wrap: wrap;
  padding: 10px 12px; margin-bottom: 10px; border-radius: 7px;
  background: linear-gradient(180deg, rgba(255,248,232,.06), rgba(255,248,232,.02));
  border: 1px solid rgba(184,134,11,.25);
}
.cbt-bar.on { border-color: rgba(196,78,42,.5); }
.cbt-round {
  display: flex; flex-direction: column; align-items: center; min-width: 58px;
  padding-right: 10px; border-right: 1px solid rgba(184,134,11,.22);
}
.cbt-round em {
  font-style: normal; font-size: .58rem; letter-spacing: .14em;
  text-transform: uppercase; color: #8d8368;
}
.cbt-round b {
  font: 700 1.5rem/1.1 Cinzel, "Trajan Pro", Georgia, serif; color: #f4e6c2;
}
.cbt-bar.on .cbt-round b { color: #e07a4e; }
.cbt-now { flex: 1; min-width: 140px; display: flex; flex-direction: column; }
.cbt-now em {
  font-style: normal; font-size: .58rem; letter-spacing: .14em;
  text-transform: uppercase; color: #8d8368;
}
.cbt-now b {
  font: 700 1rem/1.3 Cinzel, "Trajan Pro", Georgia, serif; color: #f4e6c2;
}
.cbt-now span { font-size: .68rem; color: #8d8368; }
.cbt-gone {
  display: flex; flex-wrap: wrap; gap: 6px; margin-bottom: 10px;
}
.cbt-gone span {
  font-size: .68rem; color: #f0d47a; padding: 2px 9px; border-radius: 999px;
  background: rgba(240,212,122,.1); border: 1px solid rgba(240,212,122,.35);
}
.cbt-add { display: flex; gap: 6px; align-items: center; margin-bottom: 10px; }
.cbt-add .nd-input { flex: 1; min-width: 0; }
.cbt-num { flex: 0 0 62px !important; text-align: center; }
.cbt-list { list-style: none; margin: 0; padding: 0; }
.cbt-row {
  border: 1px solid rgba(184,134,11,.18); border-left-width: 3px;
  border-radius: 6px; padding: 7px 9px; margin-bottom: 7px;
  background: rgba(0,0,0,.14);
}
.cbt-row.mine { border-left-color: rgba(184,134,11,.8); }
.cbt-row.now {
  border-color: rgba(196,78,42,.6); border-left-color: #e07a4e;
  background: linear-gradient(90deg, rgba(196,78,42,.14), rgba(0,0,0,.14) 60%);
  box-shadow: 0 0 0 1px rgba(196,78,42,.25);
}
.cbt-row.out { opacity: .45; }
.cbt-row.out .cbt-name { text-decoration: line-through; }
.cbt-main { display: flex; align-items: center; gap: 6px; }
.cbt-init-edit {
  width: 38px; flex: 0 0 38px; text-align: center; font: inherit;
  font: 700 .95rem/1 Cinzel, "Trajan Pro", Georgia, serif;
  background: rgba(0,0,0,.3); color: #f4e6c2; padding: 4px 2px;
  border: 1px solid rgba(184,134,11,.3); border-radius: 4px;
  -moz-appearance: textfield;
}
.cbt-init-edit::-webkit-outer-spin-button,
.cbt-init-edit::-webkit-inner-spin-button { -webkit-appearance: none; margin: 0; }
.cbt-name {
  flex: 1; min-width: 0; font-size: .84rem; color: #f4e6c2;
  overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
}
.cbt-hp {
  font: 700 .82rem/1 ui-monospace, SFMono-Regular, Menlo, monospace; color: #9fc38a;
  min-width: 46px; text-align: right;
}
.cbt-hp em { font-style: normal; font-size: .66rem; color: #6f6754; }
.cbt-hp.low { color: #e0845e; }
.cbt-hp.none { color: #4f4a3e; }
.cbt-amt { flex: 0 0 46px !important; text-align: center; padding: 3px !important; }
.cbt-fx { display: flex; flex-wrap: wrap; gap: 4px; margin: 6px 0 0 44px; }
.cbt-tag {
  display: inline-flex; align-items: center; gap: 4px;
  font-size: .66rem; color: #d8caa8; padding: 1px 4px 1px 7px; border-radius: 999px;
  background: rgba(0,0,0,.3); border: 1px solid rgba(184,134,11,.28);
}
.cbt-tag b {
  font: 700 .62rem/1 ui-monospace, SFMono-Regular, Menlo, monospace;
  color: #f0d47a; background: rgba(240,212,122,.12); border-radius: 3px; padding: 1px 3px;
}
.cbt-tag.soon { border-color: rgba(224,122,78,.6); }
.cbt-tag.soon b { color: #e07a4e; background: rgba(224,122,78,.14); }
.cbt-fx-x, .cbt-clock-x {
  background: none; border: 0; cursor: pointer; color: #6f6754;
  font: inherit; line-height: 1; padding: 0 1px;
}
.cbt-fx-x:hover, .cbt-clock-x:hover { color: #e07a4e; }
/* Adding an effect is a thing you do to one combatant now and then, not
   a thing you look at. Seven rows of empty boxes is most of the height of
   the list, so the box only appears on the row you are pointing at, the
   row you are typing in, and whoever's turn it is. */
.cbt-fx-add-row { display: none; gap: 5px; margin: 6px 0 0 44px; }
.cbt-row:hover .cbt-fx-add-row,
.cbt-row:focus-within .cbt-fx-add-row,
.cbt-row.now .cbt-fx-add-row { display: flex; }
.cbt-fx-add-row .nd-input { flex: 1; min-width: 0; padding: 3px 6px !important; }
.cbt-clocks {
  margin-top: 14px; padding-top: 10px; border-top: 1px dashed rgba(184,134,11,.22);
}
.cbt-sub {
  font: 700 .62rem/1.6 Cinzel, "Trajan Pro", Georgia, serif;
  letter-spacing: .14em; text-transform: uppercase; color: var(--accent-2);
  margin-bottom: 6px;
}
.cbt-hint { font-size: .72rem; line-height: 1.5; color: #8d8368; margin: 0 0 8px; }
.cbt-clocks ul { list-style: none; margin: 0 0 8px; padding: 0; }
.cbt-clocks li {
  display: flex; align-items: center; gap: 8px; padding: 4px 8px; margin-bottom: 4px;
  border-radius: 5px; background: rgba(0,0,0,.2);
  border: 1px solid rgba(184,134,11,.2);
}
.cbt-clocks li b {
  font: 700 .9rem/1 Cinzel, "Trajan Pro", Georgia, serif; color: #f0d47a;
  min-width: 20px; text-align: center;
}
.cbt-clocks li span { flex: 1; font-size: .78rem; color: #d8caa8; }
.cbt-clocks li.soon { border-color: rgba(224,122,78,.55); }
.cbt-clocks li.soon b { color: #e07a4e; }

.lvl-xp { margin: -6px 0 14px; }
.lvl-xp-line {
  display: flex; justify-content: space-between; align-items: baseline; gap: 8px;
  font-size: .68rem; letter-spacing: .06em; color: #9d9078; margin-bottom: 4px;
}
.lvl-xp-line span { color: #d8caa8; }
.lvl-xp-line em { font-style: normal; color: var(--accent-2); }
.lvl-xp-bar {
  height: 5px; border-radius: 3px; overflow: hidden;
  background: rgba(0,0,0,.35); box-shadow: inset 0 0 0 1px rgba(184,134,11,.25);
}
.lvl-xp-bar i {
  display: block; height: 100%; border-radius: 3px;
  background: linear-gradient(90deg, #8a6a18, #b8860b 60%, #f0d47a);
  transition: width .5s ease;
}
.lvl-xp-bar[data-full] i {
  background: linear-gradient(90deg, #b8860b, #f0d47a 50%, #fff3c4);
  animation: lvlXpGlow 1.8s ease-in-out infinite;
}
@keyframes lvlXpGlow { 50% { filter: brightness(1.35); } }
@media (prefers-reduced-motion: reduce) {
  .lvl-xp-bar i { transition: none; }
  .lvl-xp-bar[data-full] i { animation: none; }
}
.lvl-q { font-size: 1rem; line-height: 1.45; margin: 0 0 4px; color: #f4e6c2; }
.lvl-title {
  font: 700 .62rem/1.6 Cinzel, "Trajan Pro", Georgia, serif;
  letter-spacing: .14em; text-transform: uppercase; color: var(--accent-2);
  margin-bottom: 3px;
}
.lvl-help { font-size: .76rem; line-height: 1.5; color: #9d9078; margin: 6px 0 12px; }
.lvl-opts { display: flex; flex-direction: column; gap: 6px; }
.lvl-opt {
  display: block; width: 100%; text-align: left;
  font: inherit; color: #ece2cd; cursor: pointer;
  background: rgba(255,255,255,.03);
  border: 1px solid rgba(184,134,11,.3); border-radius: 6px;
  padding: 8px 11px;
}
.lvl-opt:hover { border-color: rgba(184,134,11,.75); background: rgba(184,134,11,.1); }
.lvl-opt.on { border-color: var(--accent-2); background: rgba(184,134,11,.2); }
.lvl-opt.off { opacity: .4; cursor: default; }
.lvl-opt .nm { font-size: .95rem; }
.lvl-opt .tip { color: #b8a472; font-size: .68rem; margin-left: 6px; }
.lvl-opt .sum { display: block; font-size: .74rem; color: #9d9078; margin-top: 2px;
                line-height: 1.4; }
.lvl-opt .why { display: block; font-size: .7rem; color: #b06a12; margin-top: 2px; }
.lvl-custom { margin-top: 9px; display: flex; align-items: center; gap: 7px; }
.lvl-custom label { font-size: .68rem; text-transform: uppercase; letter-spacing: .06em;
                    color: #9d9078; }
.lvl-field {
  flex: 1; font: inherit; font-size: .9rem; color: #f4e6c2;
  background: rgba(0,0,0,.3); border: 1px solid rgba(184,134,11,.4);
  border-radius: 5px; padding: 4px 8px;
}
.lvl-field:focus { outline: none; border-color: rgba(184,134,11,.9); }
.lvl-count {
  font-size: .68rem; color: #9d9078; letter-spacing: .05em; margin-top: 9px;
}
.lvl-steps {
  display: flex; flex-wrap: wrap; gap: 4px; margin-bottom: 12px;
}
.lvl-step {
  width: 7px; height: 7px; border-radius: 50%;
  border: 1px solid rgba(184,134,11,.5);
}
.lvl-step.done { background: var(--accent-2); border-color: var(--accent-2); }
.lvl-step.now { box-shadow: 0 0 0 3px rgba(184,134,11,.25); }
.lvl-gained { list-style: none; margin: 8px 0 0; padding: 0; }
.lvl-gained li {
  padding: 5px 0 5px 16px; position: relative;
  font-size: .9rem; line-height: 1.45;
  border-bottom: 1px dotted rgba(184,134,11,.18);
}
.lvl-gained li::before {
  content: "\002726"; position: absolute; left: 0; top: 5px;
  color: var(--accent-2); font-size: .7rem;
}
.lvl-note {
  margin-top: 12px; padding: 8px 10px;
  border-left: 2px solid rgba(184,134,11,.5); background: rgba(184,134,11,.07);
  font-size: .76rem; line-height: 1.5; color: #b8a472;
}
.lvl-err {
  margin-top: 10px; padding: 7px 10px;
  border-left: 2px solid #b1524a; background: rgba(150,40,30,.14);
  font-size: .78rem; color: #e5837a;
}
.lvl-done-head {
  font: 700 1.1rem/1.2 Cinzel, "Trajan Pro", Georgia, serif;
  color: var(--accent-2); letter-spacing: .06em; margin-bottom: 4px;
}
@media print { #lvl-tab, #lvl-tray { display: none !important; } }

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
.dt-auto select {
  font: inherit; font-size: .6rem; letter-spacing: .06em; text-transform: none;
  background: rgba(0,0,0,.25); color: #d8caa8; cursor: pointer;
  border: 1px solid rgba(184,134,11,.4); border-radius: 4px; padding: 1px 3px;
}
.dt-auto select:focus-visible { outline: 2px solid var(--accent-2); outline-offset: 1px; }
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
/* A natural 20 or a natural 1, said outright. It is the loudest line on the
   card because at the table it is the loudest thing that happened, and it
   arrives with the swell of the halo and the sound rather than on its own. */
.rc-flourish {
  margin-top: 7px; text-align: center;
  font-family: Cinzel, "Trajan Pro", Georgia, serif;
  font-size: .78rem; text-transform: uppercase; letter-spacing: .22em;
  padding: 3px 0 4px; border-radius: 5px;
  animation: rc-flourish-in .5s ease-out 1;
}
.rc-flourish.crit {
  color: #f6e7b4; background: rgba(184,134,11,.2);
  border: 1px solid rgba(184,134,11,.65);
  text-shadow: 0 0 14px rgba(246,231,180,.5);
}
.rc-flourish.fumble {
  color: #e5a19a; background: rgba(150,40,30,.18);
  border: 1px solid rgba(180,70,60,.5);
}
@keyframes rc-flourish-in {
  0% { opacity: 0; transform: translateY(-4px) scale(.94); letter-spacing: .05em; }
  100% { opacity: 1; transform: none; letter-spacing: .22em; }
}
@media (prefers-reduced-motion: reduce) { .rc-flourish { animation: none; } }
/* Every d20 is thrown twice so the card can show all three readings of it at
   once. Which one the table is owed is the player's call, so while a roll is
   still the one on the card its readings are buttons. */
button.rc-cell {
  display: block; width: 100%;
  font: inherit; color: inherit; cursor: pointer;
  -webkit-appearance: none; appearance: none;
  transition: border-color .12s, background .12s, transform .08s;
}
button.rc-cell:hover {
  border-color: rgba(184,134,11,.7);
  background: rgba(184,134,11,.11);
}
button.rc-cell:active { transform: translateY(1px); }
button.rc-cell:focus-visible { outline: 2px solid rgba(184,134,11,.75); outline-offset: 2px; }
.rc-cell.chosen { box-shadow: inset 0 0 0 1px rgba(244,230,194,.45); }
.rc-cell.chosen span::after { content: " \2713"; color: #d9b45a; }
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
/* and so does undo, which needs the room to say what it will take back */
.dc-undo { display: block; }
.dc-undo .dt-btn { width: 100%; text-align: left; }
.dc-undo .dt-btn:disabled {
  opacity: .45; cursor: default; background: transparent;
}
.dc-undo .du-what {
  display: block; margin-top: 2px;
  font-size: .6rem; letter-spacing: .04em; text-transform: none;
  color: #9d9078; white-space: normal; line-height: 1.35;
}
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
/* Every pane of the session drawer scrolls on its own.
   The drawer is a fixed height, so anything longer than it - a night's
   timeline, a fight with nine combatants in it, a long search - has to
   be scrollable or the bottom of it is simply unreachable. Only the
   notes pane opts out: it is a two-column split whose halves scroll
   themselves, and letting the pane scroll as well would give it two
   scrollbars doing different things. */
.nd-view {
  flex: 1; min-width: 0; min-height: 0; display: none;
  flex-direction: column; padding: 12px 14px; overflow-y: auto;
  overscroll-behavior: contain;
}
.nd-view.on { display: flex; }
#view-notes { overflow: hidden; }
/* The panes that are a toolbar over a list want the toolbar to stay
   put while the list moves under it. */
.nd-view > .nd-toolbar { flex: 0 0 auto; position: sticky; top: -12px;
  z-index: 2; background: linear-gradient(180deg, #221d18 80%, rgba(34,29,24,0));
  padding-top: 12px; margin-top: -12px; }
/* A scrollbar that belongs to this drawer rather than to the browser. */
.nd-view::-webkit-scrollbar, #sess-body::-webkit-scrollbar { width: 9px; }
.nd-view::-webkit-scrollbar-track,
#sess-body::-webkit-scrollbar-track { background: rgba(0,0,0,.25); }
.nd-view::-webkit-scrollbar-thumb,
#sess-body::-webkit-scrollbar-thumb {
  background: rgba(184,134,11,.4); border-radius: 5px;
}
.nd-view::-webkit-scrollbar-thumb:hover,
#sess-body::-webkit-scrollbar-thumb:hover { background: rgba(184,134,11,.65); }
.nd-view, #sess-body { scrollbar-width: thin;
  scrollbar-color: rgba(184,134,11,.45) rgba(0,0,0,.25); }
/* And the play-session popup in the dock, which can outgrow the dock
   once the sign-in box and an error are both showing. */
#sess-body { max-height: 60vh; overflow-y: auto; overscroll-behavior: contain; }
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
/* A note that is already in the file can be said better: the card turns into
   the same kind of box it was typed in, and saving rewrites that one entry in
   the session file. */
.note-edit {
  float: right; margin: -2px 0 0 8px;
  font: 700 .55rem/1.6 Cinzel, "Trajan Pro", Georgia, serif;
  letter-spacing: .1em; text-transform: uppercase;
  color: #8d8168; background: none; cursor: pointer;
  border: 1px solid rgba(184,134,11,.35); border-radius: 4px; padding: 1px 6px;
}
.note-edit:hover { color: #f0e2c0; border-color: rgba(184,134,11,.8); }
/* Del wears the same button as Edit so the pair reads as one control, and
   only turns the colour of a mistake once it is the one being confirmed. */
.note-del {
  float: right; margin: -2px 0 0 6px;
  font: 700 .55rem/1.6 Cinzel, "Trajan Pro", Georgia, serif;
  letter-spacing: .1em; text-transform: uppercase;
  color: #8d8168; background: none; cursor: pointer;
  border: 1px solid rgba(184,134,11,.35); border-radius: 4px; padding: 1px 6px;
}
.note-del:hover { color: #e5837a; border-color: rgba(229,131,122,.75); }
.note-del.yes {
  color: #f6d9d4; background: rgba(150,40,30,.5);
  border-color: rgba(229,131,122,.8);
}
.note-del.yes:hover { background: rgba(178,48,36,.7); }
.note-del.yes:disabled { opacity: .5; cursor: default; }
.note-del-no {
  float: right; margin: -2px 0 0 6px;
  font: 700 .55rem/1.6 Cinzel, "Trajan Pro", Georgia, serif;
  letter-spacing: .1em; text-transform: uppercase;
  color: #8d8168; background: none; cursor: pointer;
  border: 1px solid rgba(184,134,11,.35); border-radius: 4px; padding: 1px 6px;
}
.note-del-no:hover { color: #f0e2c0; border-color: rgba(184,134,11,.8); }
.note-ask {
  float: right; margin: 0 8px 0 0;
  font: 400 .58rem/1.9 "EB Garamond", Palatino, Georgia, serif;
  letter-spacing: .04em; text-transform: none; color: #e5837a;
}
.note-ask.bad { color: #e5837a; }
/* A note about to go says so down its edge, the way one being edited does. */
.note-entry.deleting { border-left-color: rgba(178,48,36,.85); }

/* The same button on a row of the roll table. It lives in a column of its
   own at the end so it never pushes a number about, and the row it is
   asking about stays readable while it asks. */
.log-table .log-act { width: 1%; white-space: nowrap; text-align: right; }
.roll-del, .roll-del-no {
  font: 700 .52rem/1.5 Cinzel, "Trajan Pro", Georgia, serif;
  letter-spacing: .1em; text-transform: uppercase;
  color: #8d8168; background: none; cursor: pointer;
  border: 1px solid rgba(184,134,11,.3); border-radius: 4px;
  padding: 0 5px; margin-left: 4px;
}
.roll-del:hover { color: #e5837a; border-color: rgba(229,131,122,.7); }
.roll-del-no:hover { color: #f0e2c0; border-color: rgba(184,134,11,.8); }
.roll-del.yes {
  color: #f6d9d4; background: rgba(150,40,30,.5);
  border-color: rgba(229,131,122,.8);
}
.roll-del.yes:hover { background: rgba(178,48,36,.7); }
.roll-del.yes:disabled { opacity: .5; cursor: default; }
.log-table tr.deleting td { background: rgba(150,40,30,.16); }
.note-entry.editing { border-left-color: rgba(184,134,11,.9); }
.note-editor textarea {
  width: 100%; min-height: 110px; resize: vertical;
  padding: 8px 10px;
  font-family: "EB Garamond", Palatino, Georgia, serif;
  font-size: .95rem; line-height: 1.5; color: #f4e6c2;
  background: rgba(0,0,0,.3);
  border: 1px solid rgba(184,134,11,.4); border-radius: 7px;
}
.note-editor textarea:focus { outline: none; border-color: rgba(184,134,11,.85); }
.note-editor-foot { display: flex; align-items: center; gap: 8px; margin-top: 7px; }
.note-editor-foot .nd-hint { flex: 1; }
.note-editor .nd-err { margin-top: 6px; }

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
  /* the backdrop still changes, it just stops crossfading to do it */
  .backdrop { transition: none; }
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
  .backdrop { display: none !important; }
  /* the panels stop being glass when there is nothing behind them */
  .has-backdrop .box {
    background-image: none;
    background-color: var(--paper-2);
  }
  #dice-canvas, #coin-canvas, #spell-canvas, #dice-tray, #dice-tab, #dice-dock,
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
  .inv-tabs, .inv-actions, .inv-foot, .inv-modal, .cast-btn, .cast-at, .rest-modal,
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
<!-- The halo round the edge of the page, coloured by how the character is
     doing. It is outside the page because it is pinned to the window rather
     than to the sheet, and it carries the band the hit points fall in from the
     moment the page loads so an exported sheet with no server still shows the
     right mood. -->
<div class="page-halo" id="page-halo" data-level="{{ sheet.health.level }}" aria-hidden="true"></div>
<div class="page{% if sheet.backdropSrcs %} has-backdrop{% endif %}">
{% if sheet.backdropSrcs %}
  <div class="backdrop" id="backdrop-a" style="--wash: {{ sheet.backdropOpacity }};" aria-hidden="true"></div>
  <div class="backdrop" id="backdrop-b" style="--wash: {{ sheet.backdropOpacity }};" aria-hidden="true"></div>
{% endif %}

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
        <!-- What the character looks like as the night wears on. The cracks
             are in the glass over the portrait rather than in the picture -
             a broken medallion, not a broken face - and they come one at a
             time as the hit points fall. Each is clipped to the window so
             none of them runs out over the frame. -->
        <clipPath id="portraitWell"><circle cx="105" cy="105" r="71"/></clipPath>
        <g class="portrait-cracks" clip-path="url(#portraitWell)">
          <path class="crack c1" d="M105 34 L98 62 L112 79 L96 104 L104 128 L92 152 L97 176"/>
          <path class="crack c1 twig" d="M98 62 L74 56 M112 79 L136 70 M96 104 L70 110"/>
          <path class="crack c2" d="M34 88 L62 96 L82 88 L108 100 L134 92 L158 104 L176 98"/>
          <path class="crack c2 twig" d="M82 88 L78 64 M108 100 L116 126 M134 92 L142 68"/>
          <path class="crack c3" d="M40 40 L64 70 L74 96 L96 118 L112 148 L140 170"/>
          <path class="crack c3 twig" d="M74 96 L48 118 M112 148 L146 140 M64 70 L88 48"/>
        </g>
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
               data-roll-as="check" data-ability="{{ a.id }}"
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
               data-roll-as="save" data-ability="{{ a.id }}"
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
               data-roll-as="check" data-ability="{{ s.ability }}"
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
              <div class="big" id="ac-value">{{ sheet.ac }}</div>
              <span class="sub" id="ac-source">{{ sheet.acSource }}</span>
            </div>
            <div class="tile rollable" data-kind="check" data-mod="{{ sheet.initiativeStr }}"
                 data-roll-as="check" data-ability="dex"
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
                   style="width:{{ sheet.hpPercent }}%"></div><div class="hp-temp-fill"
                   id="hp-temp-fill"{% if not sheet.hpTemp %} hidden{% endif %}
                   style="left:{{ sheet.hpPercent }}%;width:{{ sheet.health.tempPercent }}%"></div></div>
              <span class="hp-max" id="hp-max" title="Maximum hit points">{{ sheet.hpMax }}</span>
            </div>
            <!-- What happens to hit points between rests: a number and three
                 things to do with it. The panel is redrawn from what the server
                 wrote into the org file, so the sheet and the file never
                 disagree about how badly hurt somebody is. -->
            <div class="hp-temp" id="hp-temp-line">
              <span class="nm">Temporary hit points</span>
              <button type="button" class="val{% if not sheet.hpTemp %} none{% endif %}"
                      id="hp-temp-val" data-hp="cleartemp"
                      {% if not sheet.hpTemp %}disabled{% endif %}
                      title="Click to give up your temporary hit points">{{ sheet.hpTemp }}</button>
            </div>
            <div class="hp-ctl" id="hp-ctl">
              <input type="number" class="inv-field hp-amt" id="hp-amount" min="0"
                     inputmode="numeric" placeholder="0" autocomplete="off"
                     aria-label="How many hit points">
              <select class="inv-field hp-type" id="hp-type"
                      aria-label="What kind of damage"
                      title="What kind of damage it was. Your own resistances,
immunities and vulnerabilities are applied to it - the sheet does the halving,
not you.">
                <option value="">damage</option>
              </select>
              <button type="button" class="hpb hurt" data-hp="hurt">Hurt</button>
              <button type="button" class="hpb heal" data-hp="heal">Heal</button>
              <button type="button" class="hpb temp" data-hp="temp"
                      title="Set your temporary hit points to that number. They sit in
front of your hit points and are spent first; click the number below to give
them up.">Temp</button>
              <span class="inv-msg" id="hp-msg"></span>
            </div>
            <!-- The death saves. On anything above nothing at all this is one
                 line of small print; at nought hit points it becomes the six
                 marks that decide whether the character gets up again, and the
                 page fills them in from the d20 it threw. -->
            <div class="death" id="death-line"
                 data-dying="{% if sheet.health.dying %}1{% else %}0{% endif %}">
              <span class="rollable death-roll" data-kind="check"
                    data-mod="{{ sheet.deathSaveStr }}"
                    data-label="Death Saving Throw">Death save{% if sheet.deathSaveBonus %}
                    {{ sheet.deathSaveStr }}{% endif %}</span>
              <span class="death-pips" id="death-pips">
                {% for i in sheet.health.deathPips %}<span class="dp win{% if i <= sheet.health.deathSuccesses %} on{% endif %}"></span>{% endfor %}
                <span class="dp-sep">of three</span>
                {% for i in sheet.health.deathPips %}<span class="dp lose{% if i <= sheet.health.deathFailures %} on{% endif %}"></span>{% endfor %}
              </span>
              <span class="death-word" id="death-word">{% if sheet.health.dead %}Gone{% elif sheet.health.stable %}Stable{% elif sheet.health.dying %}Dying{% endif %}</span>
            </div>
            <!-- What the character is still holding their attention on. Empty
                 and out of the way until a concentration spell goes up, and
                 the place the Constitution save a blow calls for is asked
                 for. -->
            <div class="conc" id="conc-line" hidden></div>
            <div class="tagline" style="margin-top:4px">
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
                    data-roll-as="attack"
                    data-label="{{ a.name }} Attack">{{ a.name }}{% if a.notes %}<div class="tagline">{{ a.notes }}</div>{% endif %}</td>
                <td class="num rollable" data-kind="check" data-mod="{{ a.bonus }}"
                    data-roll-as="attack"
                    data-label="{{ a.name }} Attack">{{ a.bonus }}</td>
                {% if a.damage %}<td class="rollable" data-kind="damage" data-roll="{{ a.damage }}"
                    data-damage-type="{{ a.type }}"
                    data-label="{{ a.name }} Damage">{{ a.damage }} {{ a.type }}{% if a.versatile %}<div
                    class="tagline"><span class="rollable" data-kind="damage" data-roll="{{ a.versatile }}"
                    data-damage-type="{{ a.type }}"
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
                 data-roll-as="attack"
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
                        data-school="{{ sp.school }}"
                        data-detail="{{ sp.cast.detail }}" data-line="{{ sp.cast.line }}"
                        data-short="{{ sp.cast.short }}" data-level="{{ lvl.level }}"
                        {% if sp.ritual %}data-ritual="1"{% endif %}
                        {% if sp.concentration %}data-conc="1" data-spell-id="{{ sp.id }}"{% endif %}
                        {% if sp.cast.attack %}data-attack="{{ sp.cast.attackBonus }}"{% endif %}
                        {% if sp.cast.damage %}data-damage="{{ sp.cast.damage }}"
                        data-damage-type="{{ sp.cast.damageType }}"{% endif %}
                        {% if sp.cast.heal %}data-heal="{{ sp.cast.heal }}"{% endif %}
                        {% if sp.cast.damage2 %}data-damage2="{{ sp.cast.damage2 }}"
                        data-damage2-type="{{ sp.cast.damage2Type }}"
                        data-damage2-label="{{ sp.cast.damage2Label }}"{% endif %}
                        {% if sp.cast.save %}data-save="{{ sp.cast.saveName }}"
                        data-dc="{{ sp.cast.saveDc }}"{% endif %}
                        title="Cast {{ sp.name }}">Cast</button>
                {% if sp.cast.damage2 %}
                <!-- The spell's second damage, rolled on its own. -->
                <button type="button" class="cast2-btn"
                        title="Roll {{ sp.name }}'s {{ sp.cast.damage2 }} {{ sp.cast.damage2Type }}"
                        >{{ sp.cast.damage2Label }}</button>
                {% endif %}
                <!-- A spell that says what a higher slot buys is cast at the
                     level picked here: the dice the level asks for are rolled
                     and a slot of exactly that level is struck off. Levels
                     with nothing left are greyed out once the sheet has a
                     server to ask. -->
                {% if sp.cast.upcast %}
                <select class="cast-at" data-base="{{ lvl.level }}"
                        title="Cast {{ sp.name }} at a higher level"
                        aria-label="Slot level to cast {{ sp.name }} at">
                  <option value="{{ lvl.level }}" selected>{{ lvl.level }}{% if lvl.level == 1 %}st{% elif lvl.level == 2 %}nd{% elif lvl.level == 3 %}rd{% else %}th{% endif %}</option>
                  {% for up in sp.cast.upcast %}<option value="{{ up.level }}"
                          data-damage="{{ up.damage }}" data-heal="{{ up.heal }}"
                          data-damage2="{{ up.damage2 }}"
                          data-detail="{{ up.detail }}" data-short="{{ up.short }}"
                          data-note="{{ up.note }}" title="{{ up.note }}">{{ up.label }}</option>{% endfor %}
                </select>
                {% endif %}
                <!-- A cantrip spends no slot, so its picker is not a choice of
                     slot at all: it is the character levels the cantrip's own
                     dice grow at, up to the one this character has reached. It
                     starts on the tier they are at - which is what the cast
                     button already rolls - and a lower one is there to be seen,
                     and to be rolled for someone further down the table. -->
                {% if sp.cast.tiers %}
                <select class="cast-at" data-tier="1" data-base="{{ sp.cast.tierLevel }}"
                        title="{{ sp.name }} grows with your level"
                        aria-label="Character level to roll {{ sp.name }} at">
                  {% for t in sp.cast.tiers %}<option value="{{ t.level }}"{% if t.current %} selected{% endif %}
                          data-damage="{{ t.damage }}" data-heal="{{ t.heal }}"
                          data-damage2="{{ t.damage2 }}"
                          data-detail="{{ t.detail }}" data-short="{{ t.short }}"
                          data-note="{{ t.note }}" title="{{ t.note }}">{{ t.label }}</option>{% endfor %}
                </select>
                {% endif %}
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
     data-character-id="{{ sheet.id }}" data-file="{{ sheetFile }}"
     data-xp="{{ sheet.xp }}" data-next-xp="{{ sheet.nextLevelXp }}"
     data-level="{{ sheet.level }}"></div>
<script type="application/json" id="dnd-inventory-data">{{ inventoryJson|safe }}</script>
<script type="application/json" id="dnd-money-data">{{ moneyJson|safe }}</script>
<script type="application/json" id="dnd-defenses-data">{{ defensesJson|safe }}</script>
<script type="application/json" id="dnd-health-data">{{ healthJson|safe }}</script>
<script type="application/json" id="dnd-concentration-data">{{ concentrationJson|safe }}</script>
<script type="application/json" id="dnd-attunement-data">{{ attunementJson|safe }}</script>
{% if sheet.backdropSrcs %}
<script type="application/json" id="dnd-backdrop-data">{{ backdropJson|safe }}</script>
<script>
/* ---------------------------------------------------------------------------
   Backdrop: the pictures DND_BACKDROP names, washed out behind the sheet.
   With more than one of them the page picks the next at random - never the
   one already up - and crossfades to it after a random wait. The engine caps
   that wait at twenty five minutes, so nothing here needs to.
--------------------------------------------------------------------------- */
(function () {
  'use strict';
  var list = [];
  try { list = JSON.parse(document.getElementById('dnd-backdrop-data').textContent) || []; }
  catch (e) { list = []; }
  var front = document.getElementById('backdrop-a');
  var back = document.getElementById('backdrop-b');
  if (!list.length || !front || !back) { return; }

  var minWait = {{ sheet.backdropMinSeconds }} * 1000;
  var maxWait = {{ sheet.backdropMaxSeconds }} * 1000;
  var at = -1;

  // The stat boxes paint the same picture behind their own paper, so they
  // need the picture and how much paper to put over it. A box always shows
  // less of the scene than the bare page does, or it would stop reading as a
  // box at all.
  var page = document.querySelector('.page');
  var wash = parseFloat(getComputedStyle(front).getPropertyValue('--wash')) || 0.14;
  page.style.setProperty('--glass-a', String(1 - 0.45 * wash));

  // A quote in a filename is the one character that could end the url() early.
  function cssUrl(src) { return 'url("' + String(src).replace(/"/g, '%22') + '")'; }

  // The picture coming in is painted on the layer underneath, then the two
  // layers trade opacity, which is the crossfade.
  function paint(i) {
    back.style.backgroundImage = cssUrl(list[i]);
    back.classList.add('on');
    front.classList.remove('on');
    var swap = front; front = back; back = swap;
    at = i;
    // The boxes cannot crossfade - they are one layer, not two - so they
    // change once the page has finished fading rather than part way through
    // it, when the two pictures would be visibly out of step.
    var glass = function () { page.style.setProperty('--backdrop-img', cssUrl(list[i])); };
    if (first) { first = false; glass(); } else { setTimeout(glass, 2300); }
  }
  var first = true;

  // Fetch first, fade second: fading up a picture the browser has not
  // finished loading shows the bare paper through the middle of the fade.
  function show(i) {
    var img = new Image();
    img.onload = img.onerror = function () { paint(i); };
    img.src = list[i];
  }

  function pick() {
    var n = at;
    while (n === at) { n = Math.floor(Math.random() * list.length); }
    return n;
  }

  // Which picture the sheet opens on is random too, so reloading it is not
  // always the same room.
  show(Math.floor(Math.random() * list.length));

  if (list.length > 1 && maxWait > 0) {
    (function wait() {
      setTimeout(function () { show(pick()); wait(); },
                 minWait + Math.random() * (maxWait - minWait));
    })();
  }
})();
</script>
{% endif %}
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
    this.fx = null;
    this.elem = null;
    this.heart = null;
    this.school = '';
    this.element = '';
    this.tint = '';
    this.ctx.clearRect(0, 0, this.w, this.h);
  };

  // throwDice tosses one die per entry of spec, arcing from the click over to
  // the clear patch of sheet picked by the caller. The whole sequence is kept
  // to roughly two and a half seconds: a die that takes longer than that to
  // tell you its number is in the way, not in the scene.
  // opts says what these dice are the damage of, which is two different
  // things and both of them are about what the roll looks like:
  //
  //   school  - the spell that threw them, so every bounce leaves a little
  //             of that school's flourish where it struck the page rather
  //             than all the magic happening back at the button; and
  //   element - the kind of damage they are rolling, so when they come to
  //             rest the table does what that damage does: ice pushes up
  //             out of the paper, fire licks over them, lightning comes
  //             down on them.
  DiceBoard.prototype.throwDice = function (spec, from, target, opts) {
    this.stop();
    opts = opts || {};
    this.school = opts.school || '';
    this.element = opts.element || '';
    // What a bang should be coloured by, when the thing that happens is a
    // bang and the thing it does is something else.
    this.tint = opts.tint || '';
    // What the roll came to, for the one or two flourishes that care.
    this.quality = opts.quality || 0;
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
    this.landed = null;
    this.t0 = performance.now();
    this.last = this.t0;
    this.phaseAt = this.t0;
    var tick = function (now) {
      self.raf = requestAnimationFrame(tick);
      self.step(now);
    };
    this.raf = requestAnimationFrame(tick);
  };

  // whenLanded registers a one shot callback for the moment this roll's dice
  // come to rest. A new roll replaces it: the last thing thrown is the one
  // anybody is waiting on.
  DiceBoard.prototype.whenLanded = function (fn) { this.landed = fn; };

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
        // What the damage does, over a readable number rather than over a
        // blur of tumbling dice.
        if (this.element) { this.elemental(this.element, now); }
        // The dice are down and their faces are turning up, which is the
        // moment anything that waited for the answer may have it.
        if (this.landed) {
          var cb = this.landed;
          this.landed = null;
          cb();
        }
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
      this.elemShove(now);
    } else if (this.phase === 'hold') {
      this.elemShove(now);
      // A natural 20 or a natural 1 is worth looking at, so the dice stay on
      // the table until the fireworks are over or the knife has stopped
      // quivering rather than melting away underneath them.
      if (now - this.phaseAt > 900 && !this.fxBusy(now) && !this.elemBusy(now)) {
        this.startMelt();
        this.phase = 'melt';
        this.phaseAt = now;
      }
    } else if (this.phase === 'melt') {
      if (now - this.phaseAt > 620) { this.stop(); return; }
    } else if (this.phase === 'elem') {
      // An element standing on its own, with no dice to wait for.
      if (!this.elemBusy(now)) { this.stop(); return; }
    }

    // Everything is drawn relative to where the page was when the dice were
    // thrown, so they stay put on the parchment if the reader scrolls.
    var ctx = this.ctx;
    ctx.clearRect(0, 0, this.w, this.h);
    ctx.save();
    // A knife going into the table knocks everything on it, dice included.
    var shake = this.fxShake(now);
    ctx.translate(shake[0], this.scrollShift() + shake[1]);
    if (this.phase === 'melt') {
      this.drawMelt((now - this.phaseAt) / 0.62 / 1000);
    } else {
      // The shadows first, then whatever the damage put on the table, then
      // the dice - so a crystal that grew up behind a die is behind it and
      // one in front of it is in front.
      this.drawShadows(ctx);
      this.stepElem(ctx, now);
      this.drawDice(ctx);
    }
    // Fireworks go off above the dice and a knife stands in one, so both are
    // drawn last, over everything else on the table.
    this.stepFx(ctx, now);
    ctx.restore();
  };

  // onPage is where a die is as the reader sees it: projected onto the
  // canvas and then shifted by however far the page has scrolled since the
  // throw, which is the translate the draw applies. The spell board anchors
  // itself the same way, so a spark left by a bounce stays on the spot of
  // parchment it was struck from.
  DiceBoard.prototype.onPage = function (d) {
    var p = this.project(d.pos);
    return { x: p[0], y: p[1] + this.scrollShift() };
  };

  DiceBoard.prototype.integrate = function (d, dt) {
    if (d.settled) { return; }
    d.vel[2] -= GRAVITY * dt;
    d.pos[0] += d.vel[0] * dt;
    d.pos[1] += d.vel[1] * dt;
    d.pos[2] += d.vel[2] * dt;

    var sp = len(d.spin);
    if (sp > 1e-4) { d.quat = qNorm(qMul(qFromAxis(d.spin, sp * dt), d.quat)); }

    // The edges of the sheet are the rim of the table. A die coming off one
    // fast enough to hear gets a tick, which is a lighter, brighter sound than
    // landing: it glanced off something rather than coming to rest on it.
    var m = d.size * 1.5, t = this.table;
    var rim = 0;
    if (d.pos[0] < t.l + m) { rim = Math.abs(d.vel[0]); d.pos[0] = t.l + m; d.vel[0] = rim * 0.5; }
    if (d.pos[0] > t.r - m) { rim = Math.abs(d.vel[0]); d.pos[0] = t.r - m; d.vel[0] = -rim * 0.5; }
    if (d.pos[1] < t.t + m) { rim = Math.abs(d.vel[1]); d.pos[1] = t.t + m; d.vel[1] = rim * 0.5; }
    if (d.pos[1] > t.b - m) { rim = Math.abs(d.vel[1]); d.pos[1] = t.b - m; d.vel[1] = -rim * 0.5; }
    if (rim > 150) { tick(Math.min(1, rim / 900)); }

    if (d.pos[2] <= d.rest) {
      d.pos[2] = d.rest;
      // Lively enough to read as a bounce, damped hard enough that the
      // third one is the last.
      if (d.vel[2] < -55) {
        // How hard it came down is how loud it is, so the first bounce is the
        // one you hear and the third is barely there - which is what the
        // damping below is already doing to the picture.
        knock(Math.min(1, -d.vel[2] / 620), d.sides >= 12 ? 760 : 1000);
        // A spell's dice strike the page and a little of the school comes
        // off where they hit. Harder bounces leave more, so the first is
        // the one you see and the third is barely there - the same curve
        // the sound already follows.
        if (this.school) {
          var force = Math.min(1, -d.vel[2] / 620);
          spellFlourish(this.school, this.onPage(d), 0.2 + force * 0.24);
        }
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
    // The last, softest sound of the roll: the die settling onto its face.
    knock(0.16, 1250);
    // And the last of the magic settling with it, a touch larger than a
    // bounce because this is where the number is about to be read.
    if (this.school) { spellFlourish(this.school, this.onPage(d), 0.5); }
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

  // -------------------------------------------------- what the damage does
  //
  // Dice that have come to rest sit on the parchment for a second before
  // they melt away, and for that second the table does what the damage
  // does: ice pushes up out of the paper around them, rocks heave up, fire
  // licks over them and smokes, lightning comes down on them, a squall
  // shoves them about.
  //
  // It is drawn in the same world the dice are - viewport pixels flat on the
  // page, z up, the camera overhead - so everything genuinely rises out of
  // the sheet rather than being pasted over it. That is the whole trick: a
  // crystal is a tapered prism from z of nought to z of forty, projected the
  // same way the dice are, so it stands up off the page and the dice sit
  // among it.
  //
  // Every element is the same half dozen shapes with different numbers. The
  // shapes are the work; the elements are a table.
  var ELEM_GRAVITY = 900;

  // What colour a bang is. Keyed by the damage the spell actually does, so
  // the same explosion serves a fireball and a shatter.
  var BOOM_TINTS = {
    fire: { outer: '#e8531a', mid: '#ffa33c', core: '#fff6d2', edge: '#ffd27a',
            smoke: 'rgba(92,80,70,.55)', wave: 'rgba(255,170,90,.55)' },
    cold: { outer: '#3f86b8', mid: '#7fc8ea', core: '#f2fbff', edge: '#bfe8ff',
            smoke: 'rgba(150,178,196,.5)', wave: 'rgba(150,215,245,.55)' },
    lightning: { outer: '#c8a018', mid: '#ffe14d', core: '#ffffff', edge: '#fff7c0',
                 smoke: 'rgba(120,120,130,.45)', wave: 'rgba(255,225,77,.55)' },
    thunder: { outer: '#5f77a8', mid: '#a8bede', core: '#f4f8ff', edge: '#d8e6ff',
               smoke: 'rgba(140,152,176,.5)', wave: 'rgba(170,195,235,.55)' },
    force: { outer: '#7a3fd0', mid: '#c49cff', core: '#ffffff', edge: '#e8dcff',
             smoke: 'rgba(120,100,150,.45)', wave: 'rgba(200,165,255,.55)' },
    necrotic: { outer: '#4a2a58', mid: '#8a5aa0', core: '#e0c8ec', edge: '#c49cd8',
                smoke: 'rgba(58,44,68,.55)', wave: 'rgba(140,90,170,.5)' },
    radiant: { outer: '#d8a832', mid: '#ffe9a8', core: '#ffffff', edge: '#fff8d8',
               smoke: 'rgba(190,178,150,.4)', wave: 'rgba(255,230,150,.55)' },
    acid: { outer: '#6a9a18', mid: '#b8e04a', core: '#f4ffd0', edge: '#d8f27a',
            smoke: 'rgba(110,124,80,.5)', wave: 'rgba(180,225,80,.55)' },
    poison: { outer: '#3f7a2a', mid: '#79c055', core: '#e2f8d4', edge: '#a8d98a',
              smoke: 'rgba(84,110,70,.5)', wave: 'rgba(120,190,85,.55)' },
    psychic: { outer: '#c03f96', mid: '#ff8ad4', core: '#ffe8f6', edge: '#ffc0e8',
               smoke: 'rgba(140,90,125,.5)', wave: 'rgba(255,140,210,.5)' }
  };

  // ---- noise -------------------------------------------------------------
  //
  // Value noise: a hash at every whole number, smoothly interpolated
  // between them, and a few octaves of it summed at doubling frequency and
  // halving weight. It is not Perlin's gradient noise, but at the sizes
  // these effects are drawn at the difference is invisible and this is a
  // dozen lines with no table to carry around.
  //
  // What it is for: a flame whose edge wanders, a wave whose crest is
  // lumpy, a shaft of light with grain in it. Anything that would
  // otherwise be a clean curve, which is the thing that makes drawn
  // effects look drawn.
  function nhash(i) {
    var x = Math.sin(i * 127.1 + 311.7) * 43758.5453;
    return x - Math.floor(x);
  }

  function noise1(x) {
    var i = Math.floor(x), f = x - i;
    var u = f * f * (3 - 2 * f);
    return nhash(i) * (1 - u) + nhash(i + 1) * u;
  }

  // fbm is the sum, in the range zero to one.
  function fbm1(x, octaves) {
    var sum = 0, amp = 0.5, freq = 1, total = 0;
    for (var o = 0; o < (octaves || 3); o++) {
      sum += noise1(x * freq) * amp;
      total += amp;
      amp *= 0.5;
      freq *= 2.07;
    }
    return sum / total;
  }

  // The dice board's camera looks straight down, which is right for dice -
  // they show their own faces - and useless for anything standing up off the
  // page: a crystal forty units high would project two per cent wider and no
  // taller at all. So everything on this layer is drawn with the camera
  // leaned back a little, turning height in the world into height on the
  // screen. It is the one place the two disagree, and it is the difference
  // between a crystal and a smudge.
  var ELEM_LEAN = 0.66;

  // A strike pulls the whole page down for a moment, which is a
  // full-screen change of brightness several times a fight. A reader who
  // has asked the operating system for less motion gets the bolt and not
  // the sky. Read once: the effect runs inside a draw loop and asking the
  // browser to match a media query on every frame is work for nothing.
  var FLASH_OFF = (function () {
    try {
      return window.matchMedia &&
        window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    } catch (e) { return false; }
  })();

  DiceBoard.prototype.projectUp = function (p) {
    var q = this.project(p);
    return [q[0], q[1] - p[2] * ELEM_LEAN * q[2], q[2]];
  };

  var ELEMENTS = {
    cold: { kind: 'shards', face: '#eaf6ff', side: '#6fa8cf', edge: '#ffffff',
            n: 9, h: [46, 96], r: [8, 15], sides: 8, spread: 0.6, tone: 0.55,
            ring: 'rgba(160,210,240,.5)', dur: 1250 },
    piercing: { kind: 'shards', face: '#eef0f4', side: '#5d636f', edge: '#ffffff',
                n: 7, h: [34, 62], r: [5, 9], sides: 7, spread: 0.55, tone: 0.6,
                bands: 'blade', ring: 'rgba(150,158,172,.4)', dur: 950 },
    // A blade. The old answer here was a ring of small standing blades,
    // which is what a sword looks like in a display case rather than
    // what one looks like being swung.
    slashing: { kind: 'slash',
      core: '#ffffff', face: '#e8f4ff', glow: '#7fbcff', ink: 'rgba(60,84,120,.8)',
      n: 1, dur: 1250,
      variants: [
        // One clean cut, steel blue.
        { n: 1, dur: 1150, core: '#ffffff', face: '#e8f4ff',
          glow: '#7fbcff', ink: 'rgba(60,84,120,.8)' },
        // Two, crossed: the second comes in a beat later the other way.
        { n: 2, dur: 1350, core: '#fffdf4', face: '#ffe9a8',
          glow: '#ffb02e', ink: 'rgba(122,82,26,.75)' },
        // A flurry of three, fast and violet.
        { n: 3, dur: 1450, core: '#ffffff', face: '#f0dcff',
          glow: '#a96dff', ink: 'rgba(84,54,128,.75)' },
        // A heavy single cut, wide and ember red.
        { n: 1, dur: 1500, wide: 1.9, core: '#fff4e2', face: '#ffbc7a',
          glow: '#ff5a2a', ink: 'rgba(132,54,26,.75)' }
      ] },
    oldSlashing: { kind: 'shards', face: '#f1efeb', side: '#66615a', edge: '#ffffff',
                n: 6, h: [30, 56], r: [5, 10], sides: 7, spread: 0.6, tone: 0.6,
                bands: 'blade', ring: 'rgba(160,152,140,.4)', dur: 950, lean: 0.7 },
    // Rock, not steel and not spikes: stylised low-poly boulders in warm
    // browns, squatter than they are wide, shoved up out of the ground.
    bludgeoning: { kind: 'stones', face: '#c29662', side: '#4a3623', edge: '#dcc094',
                   n: 6, h: [26, 52], r: [16, 30], sides: 9, spread: 0.6, tone: 0.55,
                   dust: '#c2ab86', dur: 1250 },
    fire: { kind: 'blaze', hot: '#fff0b8', mid: '#ff9a34', low: '#b8330e',
            smoke: '#948779', n: 22, h: [58, 122], r: [7, 13], dur: 1400,
            ember: '#ffb347', embers: 26,
      variants: [
        // A bonfire: a crowd of ordinary flames, plenty of smoke.
        { n: 24, h: [52, 118], r: [6, 13], embers: 24 },
        // A pyre: fewer but much taller and thinner, the kind that roars.
        { n: 16, h: [96, 205], r: [7, 12], embers: 36, dur: 1550,
          hot: '#fff6d2', mid: '#ffae3d', low: '#c23c0a' },
        // Green witchfire: low, wide and wrong.
        { n: 26, h: [40, 84], r: [8, 16], embers: 20, dur: 1350,
          hot: '#e8ffd0', mid: '#8fdd52', low: '#1f7a3c', smoke: '#7f9a7a',
          ember: '#b6f06a' }
      ] },
    // Not all one colour: a strike has a cold blue edge to it and a
    // violet afterglow, and mixing them through the flicker is most of
    // what makes it read as electricity rather than as a yellow line.
    lightning: { kind: 'bolt', core: '#fffdf2', glow: '#ffe14d',
                 glows: ['#ffe14d', '#8fd4ff', '#c79cff', '#ffe14d', '#a8e0ff'],
                 ring: 'rgba(255,214,60,.6)', n: 5, dur: 1050,
      spark: '#ffbe3c', flash: 'rgba(46,58,96,1)',
      // Four strikes, and none of them the same weather.
      variants: [
        // A forked tree of a bolt: one heavy trunk splitting four ways,
        // yellow going to violet as it fades.
        { n: 7, forks: 7, w0: 15, w1: 4, sway0: 24, sway1: 44, spread: 11,
          steps: 18, beads: true, lead: 540, peak: 0.3, sparks: 38,
          glows: ['#ffe14d', '#c79cff', '#8fd4ff', '#ffe14d', '#b98cff',
                  '#ffd36b', '#a8e0ff'] },
        // A ribbon: one slow wide channel of cold blue-white light with
        // hardly any flicker after it, the long-exposure look.
        { n: 4, forks: 4, w0: 12, w1: 5, sway0: 28, sway1: 36, spread: 7,
          steps: 24, ribbon: true, lead: 760, dur: 1350, peak: 0.42,
          sparks: 26, spark: '#7fc4ff',
          core: '#ffffff', glows: ['#aee2ff', '#d8ecff', '#8fd4ff', '#c4e6ff'] },
        // A squall: nine thin strikes walking across the page in quick
        // succession, every one of them beaded on the way out.
        { n: 13, forks: 3, w0: 8, w1: 2.6, sway0: 32, sway1: 56, spread: 34,
          steps: 14, beads: true, lead: 320, dur: 1300, peak: 0.16, sparks: 44,
          glows: ['#ffe14d', '#8fd4ff', '#c79cff', '#fff0a0', '#a8e0ff',
                  '#d4a6ff', '#ffe14d', '#bfe8ff', '#ffd36b', '#9ad8ff',
                  '#e0b0ff', '#fff4b8', '#8fd4ff'] },
        // The one that arrives on its own: a single enormous trunk down
        // the middle, branching the whole way, with the page whited out
        // under it and a long slow afterglow. Rare by being one of four,
        // and the reason to keep casting lightning.
        { n: 3, forks: 9, w0: 26, w1: 7, sway0: 34, sway1: 40, spread: 4,
          steps: 26, beads: true, lead: 900, dur: 1700, peak: 0.46,
          sparks: 60, spark: '#ffd24a',
          core: '#ffffff', flash: 'rgba(32,42,78,1)',
          glows: ['#fff2a8', '#c79cff', '#aee2ff'] }
      ] },
    thunder: { kind: 'gale', cloud: '#c3cfe4', gust: '#e6ecf6',
               edge: 'rgba(196,214,240,.7)', n: 22, dur: 1250 },
    acid: { kind: 'brew', pool: '#8fbf2a', bub: '#c8e84a', vapour: '#b6d98a',
            n: 16, dur: 1200 },
    poison: { kind: 'brew', pool: '#4f8f34', bub: '#79c055', vapour: '#9ecb86',
              n: 16, dur: 1200 },
    necrotic: { kind: 'wither', mote: '#7a4a8a', dark: '#2e1f38', n: 20, dur: 1350 },
    psychic: { kind: 'wither', mote: '#ff7ad1', dark: '#5a2a4a', n: 20, dur: 1350 },
    radiant: { kind: 'shaft', beam: '255, 231, 150', mote: '#fff6d8', dur: 1200 },
    force: { kind: 'shaft', beam: '201, 166, 255', mote: '#e8dcff', dur: 1050 },
    // Somebody else's hand, which is not there.
    hand: { kind: 'hand', dur: 1600 },

    // What a roll that mattered looks like. These are not spells: they
    // are picked when the dice themselves were remarkable, so they have
    // to work over any roll on the sheet and say nothing about what
    // kind of roll it was.
    hexring: { kind: 'hexring', dur: 1300,
      variants: [
        { face: '#6fb8ff', edge: '#cfeaff', rim: 'rgba(110,184,255,.7)',
          cell: 17, spin: 0.5 },
        { face: '#ffb43c', edge: '#fff0c8', rim: 'rgba(255,180,60,.7)',
          cell: 13, spin: -0.7 },
        { face: '#a86cf0', edge: '#e8d8ff', rim: 'rgba(168,108,240,.7)',
          cell: 21, spin: 0.35 },
        { face: '#3fe0c8', edge: '#d8fff2', rim: 'rgba(63,224,200,.7)',
          cell: 15, spin: -0.5 }
      ] },

    // Something large coming out of the page at you.
    panther: { kind: 'panther', dur: 1700,
      variants: [
        { fill: '#3a2f6a', edge: '#b6a8ff', glow: '#7f6bd8', node: '#e8e0ff' },
        { fill: '#12483a', edge: '#7affc8', glow: '#2fd8a0', node: '#dfffee' },
        { fill: '#5a1a2e', edge: '#ff9ab4', glow: '#e0486e', node: '#ffe0e8' },
        { fill: '#173a5e', edge: '#8fd0ff', glow: '#2f88d8', node: '#e0f2ff' }
      ] },

    // Thorns up through the paper, with the undergrowth they came with.
    thorns: { kind: 'thorns', dur: 1900,
      face: '#6b4a2a', tip: '#c8a874', dark: '#2a1a0e',
      grass: '#5f8a3a', grassDark: '#2f4a1c',
      variants: [
        { n: 18, tufts: 12, h: [34, 92], r: [4.5, 9] },
        { n: 26, tufts: 16, h: [24, 62], r: [3.5, 7], dur: 1800 },
        { n: 12, tufts: 8, h: [56, 130], r: [6, 12], dur: 2000 }
      ] },

    // A shield, and the blow it stops. Eight boards and five patterns,
    // and the cast rolls the pattern separately from the board, so the
    // same shape turns up with different light on it.
    shield: { kind: 'shield', dur: 1800,
      patterns: ['hex', 'rings', 'scan', 'runes', 'star'],
      variants: [
        { shape: 'heater', rim: '#bfe0ff', glow: '#6fb8ff', fill: '#1e4a7a', node: '#eaf6ff',
          studs: 8, spins: 2 },
        { shape: 'round', rim: '#ffe0a0', glow: '#ffb43c', fill: '#7a5010', node: '#fff4dc',
          studs: 10, spins: 3 },
        { shape: 'kite', rim: '#d0ffb0', glow: '#7ade52', fill: '#2c5c18', node: '#eaffdc',
          studs: 6, spins: 2 },
        { shape: 'buckler', rim: '#eef4ff', glow: '#c8d8f0', fill: '#49566a', node: '#ffffff',
          studs: 12, spins: 4 },
        { shape: 'tower', rim: '#e0c0ff', glow: '#a86cf0', fill: '#3f2068', node: '#f2e8ff',
          studs: 8, spins: 2 },
        { shape: 'pavise', rim: '#ffc8d8', glow: '#ff5f86', fill: '#701c36', node: '#ffe8ee',
          studs: 14, spins: 2 },
        { shape: 'targe', rim: '#b0fff0', glow: '#3fe0c8', fill: '#10584c', node: '#e0fff8',
          studs: 18, spins: 3 },
        { shape: 'hoplon', rim: '#ffd8b0', glow: '#ff8a3c', fill: '#7a3208', node: '#fff0e0',
          studs: 16, spins: 3 }
      ] },

    // A dome of cells over the dice, humming.
    // The dome itself never moves. What moves is the light on it - a
    // meridian sweeping round, rings climbing from the rim to the cap,
    // and every panel flickering on its own clock - which is what makes
    // a still thing look live.
    dome: { kind: 'dome', dur: 2200,
      // Six domes that are six different objects, not one object in
      // six colours: the panelling, the motion, the height and how much
      // skin there is at all change together.
      base: 0.2, flick: 0.36, churn: 0.8, weight: 1, ribAlpha: 0.4,
      turn: 0.5, bloom: 0.8,
      ribWeight: 1.6, hoops: 0, runes: 0, nodes: true, climb: 0.85,
      variants: [
        // A quad lattice, staggered like brickwork, swept by a
        // meridian of light. The plain one.
        { style: 'quad', face: '#6fc0ff', seam: '#2b6fae', edge: '#eaf6ff',
          node: '#ffffff',
          rim: 'rgba(90,170,235,.75)', rings: 6, base2: 16, ribs: 8,
          spin: 1.6, motion: 'both', tall: 0.9, turn: 0.42, bloom: 0.9 },
        // Hexagons, violet, breathing from the rim up rather than
        // turning: the honeycomb one.
        { style: 'hex', face: '#b088ff', seam: '#5b2f9e', edge: '#f2e8ff',
          node: '#ffffff',
          rim: 'rgba(150,110,240,.75)', rings: 5, base2: 13, ribs: 0,
          spin: 0, climb: 1.2, motion: 'pulse', tall: 1.0, hoops: 5,
          turn: -0.3, bloom: 1.1 },
        // A proper geodesic: triangles, gold, dense, and turning fast.
        { style: 'tri', face: '#ffc860', seam: '#9a6a12', edge: '#fff6dc',
          node: '#fffbe8',
          rim: 'rgba(225,175,80,.75)', rings: 7, base2: 14, ribs: 0,
          spin: 2.6, motion: 'scan', tall: 0.85, weight: 0.8, flick: 0.18,
          turn: 0.75, bloom: 0.7 },
        // Overlapping scales, green, low and wide, flickering with no
        // pattern to it at all.
        { style: 'scale', face: '#5fe8c0', seam: '#1d7a62', edge: '#e8fff8',
          node: '#ffffff',
          rim: 'rgba(70,215,175,.75)', rings: 7, base2: 16, ribs: 0,
          spin: 0, motion: 'storm', tall: 0.62, churn: 2.6, flick: 0.55,
          nodes: false, turn: -0.55, bloom: 1.3 },
        // A lantern: long vertical staves the whole height of it, tall
        // and narrow, with hoops round it and a slow turn.
        { style: 'staves', face: '#ff9a6a', seam: '#a8431c', edge: '#ffe8d8',
          node: '#fff0e0',
          rim: 'rgba(230,130,80,.75)', rings: 1, base2: 18, ribs: 0,
          spin: 1.0, motion: 'scan', tall: 1.25, hoops: 7, weight: 1.4,
          nodes: false, turn: 0.28, bloom: 0.6 },
        // Hardly a dome at all: a cage of ribs with a few plates left
        // in it, warded with a band of writing round the base.
        { style: 'cage', face: '#dfe8f4', seam: '#4a5a72', edge: '#ffffff',
          node: '#ffffff',
          rim: 'rgba(200,220,245,.75)', rings: 6, base2: 12, ribs: 14,
          spin: -1.3, motion: 'both', tall: 1.05, ribAlpha: 0.65,
          ribWeight: 2.2, hoops: 4, runes: 18, base: 0.06, flick: 0.2,
          turn: 0.9, bloom: 1.0 }
      ] },

    // Pieces from all over the sheet, stacking themselves into a figure.
    // Flat shading takes a ramp rather than three named colours: the
    // shadow, the body and the highlight, mixed per facet from its own
    // normal. Stone is cool and grey; bark is warm and much darker in
    // its shadow, because it is fissured and the light never gets in.
    stoneskin: { kind: 'cairn', dur: 2600, bark: false, lit: '#b8b0a0',
                 shade: [44, 42, 38], mid: [122, 116, 104],
                 tone: [206, 200, 186] },
    barkskin: { kind: 'cairn', dur: 2600, bark: true, lit: '#a07848',
                shade: [30, 20, 12], mid: [106, 74, 42],
                tone: [196, 152, 96] },

    // ---- the skills. Not spells, but the same machinery.
    glass: { kind: 'glass', dur: 1600,
      variants: [
        // Brass and dark wood.
        { rim: '#b08a3c', rimLit: '#f0d88a', rimDark: '#4a3410',
          wood: '#5a3a20', woodLit: '#9a6c42', woodDark: '#241407',
          glow: '#ffe9a8' },
        // Old silver and pale wood.
        { rim: '#9aa2ac', rimLit: '#eef2f6', rimDark: '#3c434c',
          wood: '#8a6a44', woodLit: '#c6a074', woodDark: '#3e2a16',
          glow: '#dfeeff' },
        // Blackened iron and ebony, for a grimmer sort of scholar.
        { rim: '#4e5058', rimLit: '#a8acb6', rimDark: '#1a1b20',
          wood: '#2a2420', woodLit: '#5c5048', woodDark: '#0e0c0a',
          glow: '#b6d0ff' }
      ] },
    bloom: { kind: 'bloom', dur: 2000,
      // Every stem in the patch rolls its own flower, its own colours
      // and its own habit, so a Nature check is never the same garden
      // twice - nor is any one clump of it all the same plant.
      forms: ['daisy', 'poppy', 'star', 'tulip', 'foxglove', 'thistle', 'rose'],
      stems: ['plain', 'twine', 'briar', 'fern', 'grassy'],
      greens: [
        { stem: '#6f9a3c', leaf: '#5f9a3a', dark: '#2c4a19' },
        { stem: '#4f7a34', leaf: '#74ab48', dark: '#1e3a12' },
        { stem: '#86a04c', leaf: '#a8bd5e', dark: '#44521f' },
        { stem: '#3f6b46', leaf: '#4f8a56', dark: '#16301c' },
        { stem: '#7a8a3a', leaf: '#93a842', dark: '#3a4415' }
      ],
      blooms: [
        { petal: '#f2a8c8', petal2: '#d4719e', heart: '#ffd86a', petals: 6 },
        { petal: '#ffd98a', petal2: '#e8a63c', heart: '#8a5a1c', petals: 8 },
        { petal: '#b9c8ff', petal2: '#7f93e0', heart: '#fff0b0', petals: 5 },
        { petal: '#ffffff', petal2: '#dcd6c4', heart: '#f0c03c', petals: 7 },
        { petal: '#d8a8f0', petal2: '#a05fd0', heart: '#fff2c8', petals: 6 },
        { petal: '#ff9a7a', petal2: '#d8543c', heart: '#ffe08a', petals: 5 },
        { petal: '#e84a4a', petal2: '#9e1f22', heart: '#1c1208', petals: 4 },
        { petal: '#9fd8ff', petal2: '#4f8fc8', heart: '#fff0a0', petals: 6 },
        { petal: '#fff4d0', petal2: '#e0c88a', heart: '#c8963c', petals: 8 },
        { petal: '#c8a0ff', petal2: '#6f3fa8', heart: '#f0e0ff', petals: 5 }
      ] },
    // Three books. The paper is nearly the same in all of them because
    // paper is paper; what changes is what it is bound in, what colour
    // the ink burns and how filthy the pages have got.
    tome: { kind: 'tome', dur: 2400,
      variants: [
        { cover: '#6b3a22', coverDark: '#2e1409',
          page: '#e6d9b4', pageDeep: '#c8b68a', pageEdge: '#d4c49c',
          pageLine: '#9a8a62', gutter: 'rgba(70,52,28,.55)',
          stain: 'rgba(122,88,42,.5)', fox: 'rgba(108,70,32,.6)',
          thumb: 'rgba(96,66,30,.42)',
          ribbon: '#8e2230', ribbonDark: '#4a0f17', rune: '#8a5ad8' },
        { cover: '#24405c', coverDark: '#0b1624',
          page: '#dfe4de', pageDeep: '#bcc3bc', pageEdge: '#cdd3cb',
          pageLine: '#8a9290', gutter: 'rgba(26,42,58,.55)',
          stain: 'rgba(72,96,110,.5)', fox: 'rgba(92,78,60,.55)',
          thumb: 'rgba(60,78,88,.42)',
          ribbon: '#c8a03c', ribbonDark: '#6e5416', rune: '#3ca8d8' },
        { cover: '#3c5230', coverDark: '#16240e',
          page: '#e8dcbc', pageDeep: '#c6b58c', pageEdge: '#d6c8a2',
          pageLine: '#9c8c64', gutter: 'rgba(48,56,28,.55)',
          stain: 'rgba(104,96,44,.5)', fox: 'rgba(118,84,36,.6)',
          thumb: 'rgba(88,80,34,.42)',
          ribbon: '#2f6f5a', ribbonDark: '#12332a', rune: '#c8a03c' }
      ] },
    notes: { kind: 'notes', dur: 1800,
             hues: [262, 292, 205, 44, 330, 172] },

    // A storm. Not a ring of anything: a column of driven sleet turning
    // hard around one spot, which is what the spell describes and what
    // nothing else in this table looks remotely like.
    // Colours worth stating plainly, because the obvious ones are wrong.
    // Snow is white and the page is cream, so a white storm on it is an
    // invisible storm: what a squall actually looks like against a
    // bright ground is a darker, colder patch with pale specks in it.
    // So the haze is a grey-blue that dims the paper, the sleet is
    // steel, and only the flakes nearest the eye are near white.
    blizzard: { kind: 'blizzard', flake: '#eef7fd', shard: '#5f8fb4',
                haze: 'rgba(104,140,172,.3)', rime: 'rgba(132,170,198,.5)',
                mark: 'rgba(110,152,182,.42)', gloom: 'rgba(96,124,156,.34)',
                n: 150, dur: 1700,
      variants: [
        // Driving sleet: hard, fast, mostly shards, tight column.
        { n: 165, spin: 6.2, tight: 0.85, fall: 1.0, shards: 0.68, dur: 1650,
          shard: '#4f7fa6', haze: 'rgba(88,126,160,.34)' },
        // A snow squall: slower, wider, fat flakes wandering down.
        { n: 140, spin: 3.4, tight: 1.35, fall: 0.55, shards: 0.22, dur: 1900,
          flake: '#ffffff', shard: '#7ba4c4', haze: 'rgba(124,158,186,.28)',
          gloom: 'rgba(120,146,174,.26)' },
        // A whiteout: everything at once, and you cannot see the dice.
        { n: 195, spin: 8.4, tight: 1.05, fall: 1.25, shards: 0.45, dur: 1750,
          haze: 'rgba(146,178,202,.42)', gloom: 'rgba(110,140,170,.42)' }
      ] },

    // These three are not damage types the rules have, so nothing arrives
    // here asking for them by name - they are picked out of the spell's
    // own title, which is the same best effort the rest of this module
    // makes when the data does not say. See SPELL_ELEMENTS.
    water: { kind: 'wave', deep: '#16537a', face: '#4aa6d8', foam: '#f2fbff',
             n: 26, dur: 1400 },
    // Healing: a ring of sigils turning and opening out, with the old
    // apothecary's cross rising through it.
    mend: { kind: 'mend', ring: '#8fe6b4', glow: '#e8fff2', mark: '#b7f2cf',
            n: 9, plus: 7, dur: 1400 },
    // Holding: something loops over the dice and pulls tight.
    // Three ways of being held: tied, chained, or grown over. Which one
    // it is, is picked fresh each cast.
    bind: { kind: 'bind', n: 6, dur: 1450,
            variants: [
              { lay: 'twist', rope: '#c89a52', glow: '#ffe6a8', dark: '#5f4218',
                n: 6, w: [5, 9] },
              { lay: 'links', rope: '#b9bfc9', glow: '#eaf2ff', dark: '#3f4650',
                n: 5, w: [6, 10] },
              { lay: 'leaves', rope: '#6f7f3a', leaf: '#5f9a3a',
                glow: '#d4f5a8', dark: '#313d17',
                n: 7, w: [4, 7] }
            ] },
    // A bang. Tinted by whatever the spell actually does, so a fireball
    // goes off orange and a shatter goes off blue-white; the shape of the
    // thing is the same either way.
    blast: { kind: 'boom', dur: 1500,
             variants: [
               { spikes: 18, rough: 0.5, rays: 14, lobes: 7 },
               { spikes: 26, rough: 0.34, rays: 20, lobes: 9 },
               { spikes: 12, rough: 0.72, rays: 9, lobes: 5 }
             ] },
    // Arrows, darts, rays and bolts: anything that arrives from somewhere
    // else and hits. Three ways of arriving, picked fresh each cast.
    // Seven of these, and every one of them picks its own head, its own
    // number of spirals and its own grit on top of that - so two casts
    // of the same spell are never the same projectile.
    missile: { kind: 'missile', dur: 1250,
      variants: [
        // A volley of neon arrows, hard and straight.
        { n: 3, face: '#7fb4ff', glow: '#3f6fd8', core: '#ffffff',
          burst: 'rgba(140,190,255,.65)', w: [4, 7], stagger: 130,
          head: 'arrow', spirals: 1, twist: 9, chevrons: true, grit: 0.4,
          wander: 8, churn: 1, shed: 0.07, shedN: 2 },
        // Burning shot: fewer, fatter, trailing flame and embers.
        { n: 3, face: '#ff9a4a', glow: '#d8481a', core: '#fff3d0',
          burst: 'rgba(255,150,70,.6)', w: [6, 9], stagger: 150,
          head: 'orb', spirals: 0, twist: 6, burns: true, grit: 0.7,
          wander: 16, churn: 2.2, shed: 0.05, shedN: 3, dur: 1400 },
        // One enormous violet lance, no hurry about it.
        { n: 1, face: '#e3a8ff', glow: '#8b3fd8', core: '#ffffff',
          burst: 'rgba(200,150,255,.7)', w: [10, 14], stagger: 0,
          head: 'lance', spirals: 3, twist: 16, chevrons: true, grit: 0.55,
          wander: 10, churn: 0.8, shed: 0.045, shedN: 3, dur: 1500 },
        // A swarm of small fast darts.
        { n: 7, face: '#a8ffd8', glow: '#1f9a72', core: '#ffffff',
          burst: 'rgba(130,255,205,.6)', w: [2.5, 4.5], stagger: 62,
          head: 'star', spirals: 0, twist: 4, grit: 0.35,
          wander: 22, churn: 3, shed: 0.1, shedN: 1 },
        // Frozen shards, spiralling hard.
        { n: 4, face: '#cfeeff', glow: '#3d8fc4', core: '#ffffff',
          burst: 'rgba(190,235,255,.65)', w: [5, 8], stagger: 110,
          head: 'shard', spirals: 2, twist: 22, chevrons: true, grit: 0.8,
          wander: 6, churn: 0.6, shed: 0.06, shedN: 2 },
        // Gold, ceremonial, a single guided bolt with everything on it.
        { n: 2, face: '#ffdd88', glow: '#c98a1c', core: '#fffaf0',
          burst: 'rgba(255,215,130,.7)', w: [8, 11], stagger: 190,
          head: 'arrow', spirals: 3, twist: 13, chevrons: true, burns: true,
          grit: 0.65, wander: 12, churn: 1.4, shed: 0.05, shedN: 3,
          dur: 1450 },
        // Something very wrong: dark comets with a sick green trail.
        { n: 5, face: '#b6f06a', glow: '#3f7a1c', core: '#f2ffd0',
          burst: 'rgba(160,230,100,.6)', w: [4, 7], stagger: 88,
          head: 'orb', spirals: 1, twist: 7, grit: 0.85,
          wander: 26, churn: 2.6, shed: 0.06, shedN: 2 }
      ] },
    // And the one everything else falls back to. A spell that neither
    // burns nor freezes nor binds anybody is still magic, and what magic
    // looks like when it has nothing else to show for itself is a circle
    // of light on the ground with writing round it.
    circle: { kind: 'circle', dur: 1900 }
  };

  // A ring of sigils around something being mended: triangles standing on
  // the paper, turning slowly and opening outwards, each with a bloom
  // behind it. Healing in this game is a circle of light and a symbol, and
  // this is both.
  DiceBoard.prototype.drawSigil = function (ctx, it, grown, alpha) {
    var p = this.projectUp([it.x, it.y, it.z]);
    var r = it.r * p[2] * (0.4 + 0.6 * grown);
    var a = it.spin;
    ctx.save();
    ctx.globalCompositeOperation = 'lighter';
    // The bloom behind it, which is most of what is seen at this size.
    var g = ctx.createRadialGradient(p[0], p[1], 0, p[0], p[1], r * 2.6);
    g.addColorStop(0, it.glow);
    g.addColorStop(1, 'rgba(0,0,0,0)');
    ctx.globalAlpha = alpha * 0.5;
    ctx.fillStyle = g;
    ctx.beginPath();
    ctx.arc(p[0], p[1], r * 2.6, 0, Math.PI * 2);
    ctx.fill();
    // And the triangle itself, outlined rather than filled so it reads as
    // drawn light.
    ctx.globalAlpha = alpha;
    ctx.strokeStyle = it.face;
    ctx.lineWidth = Math.max(1, 1.8 * p[2]);
    ctx.lineJoin = 'round';
    ctx.beginPath();
    for (var k = 0; k < 3; k++) {
      var ka = a + k * Math.PI * 2 / 3;
      var x = p[0] + Math.cos(ka) * r, y = p[1] + Math.sin(ka) * r * 0.85;
      if (k) { ctx.lineTo(x, y); } else { ctx.moveTo(x, y); }
    }
    ctx.closePath();
    ctx.stroke();
    ctx.restore();
  };

  // A cross of the kind that means "this will help", rising and fading.
  DiceBoard.prototype.drawPlus = function (ctx, it, alpha) {
    var p = this.projectUp([it.x, it.y, it.z]);
    var r = it.r * p[2], w = r * 0.36;
    ctx.save();
    ctx.globalCompositeOperation = 'lighter';
    ctx.globalAlpha = alpha;
    ctx.fillStyle = it.face;
    ctx.beginPath();
    ctx.rect(p[0] - r, p[1] - w, r * 2, w * 2);
    ctx.rect(p[0] - w, p[1] - r, w * 2, r * 2);
    ctx.fill();
    // A soft halo, so it glows rather than being a shape cut out of the
    // page.
    var g = ctx.createRadialGradient(p[0], p[1], 0, p[0], p[1], r * 2.2);
    g.addColorStop(0, it.glow);
    g.addColorStop(1, 'rgba(0,0,0,0)');
    ctx.globalAlpha = alpha * 0.45;
    ctx.fillStyle = g;
    ctx.beginPath();
    ctx.arc(p[0], p[1], r * 2.2, 0, Math.PI * 2);
    ctx.fill();
    ctx.restore();
  };

  // A rope, a chain or a vine bursting out of the paper, looping over the
  // dice and pulling tight.
  //
  // The arc is a lift off the page at the middle and both ends buried in
  // the sheet, so it genuinely goes over rather than round. As it tightens
  // the lift drops and the ends draw in, which is the whole animation:
  // something thrown over a thing and then hauled.
  //
  // Two things stop it reading as a glowing arc. The first is that it
  // comes *out* of the paper: each end tears a hole, and the hole is drawn
  // - a dark socket, a pale ragged rim, and two flaps of parchment lifted
  // where it broke through. The second is that the length of it has a
  // surface: a rope is drawn with its lay as diagonal ticks, a chain as
  // links, a vine with leaves and thorns.
  var ROPE_KINDS = {
    hemp: { rope: '#c89a52', glow: '#ffe6a8', dark: '#5f4218', lay: 'twist' },
    chain: { rope: '#b9bfc9', glow: '#eaf2ff', dark: '#3f4650', lay: 'links' },
    vine: { rope: '#6f7f3a', leaf: '#5f9a3a', glow: '#d4f5a8',
            dark: '#313d17', lay: 'leaves' }
  };

  // The hole one end of it came through.
  DiceBoard.prototype.drawBurst = function (ctx, at, r, angle, it, alpha) {
    var i;
    ctx.save();
    ctx.translate(at[0], at[1]);
    ctx.scale(1, 0.5);
    // The socket. Darker than the paper rather than brighter, because it
    // is a hole and not a light.
    ctx.globalAlpha = alpha * 0.85;
    var g = ctx.createRadialGradient(0, 0, 0, 0, 0, r);
    g.addColorStop(0, 'rgba(24,18,12,.92)');
    g.addColorStop(0.7, 'rgba(48,36,24,.6)');
    g.addColorStop(1, 'rgba(48,36,24,0)');
    ctx.fillStyle = g;
    ctx.beginPath();
    ctx.arc(0, 0, r, 0, Math.PI * 2);
    ctx.fill();
    // The torn rim: a ragged ring, its wobble fixed per hole so it does
    // not crawl.
    ctx.globalAlpha = alpha;
    ctx.strokeStyle = 'rgba(232,220,196,.85)';
    ctx.lineWidth = 1.6;
    ctx.beginPath();
    for (i = 0; i <= 22; i++) {
      var a = (i / 22) * Math.PI * 2;
      var rr = r * (0.78 + 0.34 * fbm1(it.seed * 3 + i * 0.9, 2));
      var x = Math.cos(a) * rr, y = Math.sin(a) * rr;
      if (i) { ctx.lineTo(x, y); } else { ctx.moveTo(x, y); }
    }
    ctx.closePath();
    ctx.stroke();
    ctx.restore();

    // Two flaps of paper lifted where it came through, standing up off
    // the page on the side the rope leans away from.
    ctx.save();
    ctx.globalAlpha = alpha * 0.9;
    ctx.fillStyle = 'rgba(240,231,210,.95)';
    ctx.strokeStyle = 'rgba(150,132,100,.7)';
    ctx.lineWidth = 0.8;
    for (i = 0; i < 2; i++) {
      var fa = angle + (i ? 2.1 : -2.1) + (fbm1(it.seed + i * 5, 1) - 0.5);
      var fl = r * (0.45 + 0.35 * fbm1(it.seed * 2 + i, 1));
      // Wide at the base and short: a flap of paper bent back, not a
      // spike. The first version was a narrow triangle and every hole
      // looked like it had grown thorns.
      ctx.beginPath();
      ctx.moveTo(at[0] + Math.cos(fa - 0.85) * r * 0.9,
                 at[1] + Math.sin(fa - 0.85) * r * 0.45);
      ctx.quadraticCurveTo(
        at[0] + Math.cos(fa - 0.3) * (r + fl * 1.1),
        at[1] + Math.sin(fa - 0.3) * (r + fl) * 0.4 - fl * 0.55,
        at[0] + Math.cos(fa + 0.25) * (r + fl),
        at[1] + Math.sin(fa + 0.25) * (r + fl) * 0.4 - fl * 0.4);
      ctx.lineTo(at[0] + Math.cos(fa + 0.85) * r * 0.9,
                 at[1] + Math.sin(fa + 0.85) * r * 0.45);
      ctx.closePath();
      ctx.fill();
      ctx.stroke();
    }
    ctx.restore();
  };

  // One leaf, drawn at (x, y) pointing along ang.
  //
  // The silhouette is two mirrored curves from the base to the tip, each
  // pushed out by a lobe function - so it is widest a third of the way
  // along, comes to a point, and has a shallow serration running down
  // both edges. The two halves are shaded differently because a leaf is
  // never flat to the light, and the veins are what carry the size: a
  // midrib and four pairs of side veins leaving it at a slant.
  //
  // The colour is knocked about per leaf from the seed - some yellowed,
  // some nearly black in shadow - because a bush of identical greens is
  // the single thing that makes drawn foliage look drawn.
  DiceBoard.prototype.drawLeaf = function (ctx, x, y, ang, size, it, seed, alpha) {
    var i, L = size * 2.3, W = size * 0.92;
    var lean = (nhash(seed) - 0.5) * 0.5;
    // A green of its own. Six of them, because the thing that gives a
    // drawn plant away is every leaf being the same colour - a real one
    // has new growth, old growth, one going over and one in shadow.
    var GREENS = ['#a9cf62', '#8ab748', '#6da236', '#57892c', '#416f24', '#c3c45a'];
    var SHADES = ['#3d5c22', '#35521d', '#2c4a19', '#24401a', '#1c3315', '#4a4f1c'];
    var tone = Math.min(5, Math.floor(nhash(seed * 3) * 6));
    var lit = GREENS[tone], shade = SHADES[tone];

    // The edge of one half, from base to tip. side is +1 or -1.
    var half = function (side, swell) {
      var out = [];
      for (i = 0; i <= 18; i++) {
        var u = i / 18;
        // Widest a third along, pinched to nothing at both ends, with a
        // serration laid over it that dies out at the tip.
        var lobe = Math.pow(Math.sin(Math.pow(u, 0.72) * Math.PI), 0.85);
        var teeth = Math.sin(u * 13 + seed) * 0.07 * (1 - u) * (u > 0.12 ? 1 : 0);
        out.push([u * L, side * (lobe + teeth) * W * swell]);
      }
      return out;
    };
    var blade = function (colour, swell, from, to, a2) {
      var pts = half(from, swell).concat(half(to, swell).reverse());
      ctx.globalAlpha = alpha * a2;
      ctx.fillStyle = colour;
      ctx.beginPath();
      for (i = 0; i < pts.length; i++) {
        if (i) { ctx.lineTo(pts[i][0], pts[i][1]); }
        else { ctx.moveTo(pts[i][0], pts[i][1]); }
      }
      ctx.closePath();
      ctx.fill();
    };

    ctx.save();
    ctx.translate(x, y);
    ctx.rotate(ang + lean);

    // The stalk holding it off the stem. Without one the leaf grows out
    // of the bark and the whole thing reads as a cutout.
    ctx.globalAlpha = alpha * 0.9;
    ctx.strokeStyle = shade;
    ctx.lineWidth = Math.max(0.7, size * 0.11);
    ctx.lineCap = 'round';
    ctx.beginPath();
    ctx.moveTo(-size * 0.5, size * 0.1);
    ctx.quadraticCurveTo(-size * 0.1, 0, 0, 0);
    ctx.stroke();

    // Both halves at once for the dark under-side, then the lit half
    // over it - a leaf turned a little out of the light.
    blade(shade, 1.02, 1, -1, 0.95);
    blade(lit, 0.97, 1, 0, 1);

    // Midrib and side veins.
    ctx.globalAlpha = alpha * 0.55;
    ctx.strokeStyle = shade;
    ctx.lineWidth = Math.max(0.5, size * 0.07);
    ctx.beginPath();
    ctx.moveTo(0, 0);
    ctx.quadraticCurveTo(L * 0.5, W * 0.06, L, 0);
    ctx.stroke();
    ctx.lineWidth = Math.max(0.35, size * 0.04);
    ctx.globalAlpha = alpha * 0.38;
    for (i = 1; i <= 4; i++) {
      var at = i / 5.2, ax = at * L;
      var reach = Math.pow(Math.sin(Math.pow(at, 0.72) * Math.PI), 0.85) * W * 0.78;
      ctx.beginPath();
      ctx.moveTo(ax, 0);
      ctx.quadraticCurveTo(ax + L * 0.12, reach * 0.5, ax + L * 0.16, reach);
      ctx.moveTo(ax, 0);
      ctx.quadraticCurveTo(ax + L * 0.12, -reach * 0.5, ax + L * 0.16, -reach);
      ctx.stroke();
    }
    ctx.restore();
    ctx.globalAlpha = alpha;
  };

  DiceBoard.prototype.drawRope = function (ctx, it, t, alpha) {
    var self = this;
    var tight = Math.min(1, Math.max(0, (t - 0.18) / 0.5));
    var lift = it.lift * (1 - tight * 0.82);
    var span = it.span * (1 - tight * 0.24);
    var a = it.angle;
    var dx = Math.cos(a), dy = Math.sin(a) * 0.72;
    var n = 26, i, pts = [], along = [];
    for (i = 0; i <= n; i++) {
      var u = i / n;
      // A loop, not a line. Every rope is bowed sideways by its own
      // amount as well as lifted, or they all pass through the same point
      // over the middle of the dice and the whole thing reads as a bundle
      // of straws tied at the top rather than as separate loops.
      var arc = Math.sin(u * Math.PI);
      var wob = (fbm1(it.seed + u * 5 + t * 1.6, 2) - 0.5) * 11 * arc +
                it.bow * arc;
      pts.push(this.projectUp([
        it.x + dx * span * (u - 0.5) * 2 - dy * wob,
        it.y + dy * span * (u - 0.5) * 2 + dx * wob,
        lift * arc]));
      along.push(arc);
    }

    // The unit normal to the drawn line at a node - which side is "off
    // the stem" on screen. Everything hung on the vine needs it.
    var edgeAt = function (i) {
      var a = pts[i > 0 ? i - 1 : 0], b = pts[i < n ? i + 1 : n];
      var tx = b[0] - a[0], ty = b[1] - a[1];
      var len = Math.hypot(tx, ty) || 1;
      return [-ty / len, tx / len];
    };
    var ang0 = function (e, side) { return Math.atan2(e[1] * side, e[0] * side); };

    // The holes it came out of, drawn first so the rope rises from them.
    var hole = it.w * 2.6;
    this.drawBurst(ctx, pts[0], hole, a + Math.PI, it, alpha);
    this.drawBurst(ctx, pts[n], hole, a, it, alpha);

    ctx.save();
    ctx.lineCap = 'round';
    ctx.lineJoin = 'round';
    var stroke = function (colour, width, a2) {
      ctx.globalAlpha = alpha * a2;
      ctx.strokeStyle = colour;
      ctx.lineWidth = width;
      ctx.beginPath();
      for (i = 0; i <= n; i++) {
        if (i) { ctx.lineTo(pts[i][0], pts[i][1]); }
        else { ctx.moveTo(pts[i][0], pts[i][1]); }
      }
      ctx.stroke();
    };
    // Bloom, body, and a lit edge along the top of it. A vine gets less
    // of the bloom than a rope or a chain does: a plant is not a lit
    // thing, and haloing every stem turns a bush into one green smear.
    var halo = it.lay === 'leaves' ? 0.45 : 1;
    stroke(it.glow, it.w * 5, 0.16 * halo);
    stroke(it.glow, it.w * 2.6, 0.22 * halo);
    stroke(it.dark, it.w * 1.5, 0.9);
    stroke(it.rope, it.w, 1);

    // ---- the surface. This is what tells a rope from a chain from a
    // vine, and it is the only part of any of them that is not an arc.
    ctx.globalAlpha = alpha * 0.85;
    if (it.lay === 'twist') {
      // The lay of a rope: short diagonal ticks across it, leaning the
      // same way all along.
      ctx.strokeStyle = it.dark;
      ctx.lineWidth = Math.max(0.8, it.w * 0.3);
      for (i = 1; i < n; i++) {
        var px = pts[i][0], py = pts[i][1];
        var tx = pts[i + 1 > n ? n : i + 1][0] - pts[i - 1][0];
        var ty = pts[i + 1 > n ? n : i + 1][1] - pts[i - 1][1];
        var len = Math.hypot(tx, ty) || 1;
        var nx = -ty / len, ny = tx / len;
        var lean = 0.45;
        ctx.beginPath();
        ctx.moveTo(px - nx * it.w * 0.52 - tx / len * it.w * lean,
                   py - ny * it.w * 0.52 - ty / len * it.w * lean);
        ctx.lineTo(px + nx * it.w * 0.52 + tx / len * it.w * lean,
                   py + ny * it.w * 0.52 + ty / len * it.w * lean);
        ctx.stroke();
      }
    } else if (it.lay === 'links') {
      // A chain: an oval every other step, turned the way the chain runs.
      ctx.strokeStyle = it.glow;
      ctx.lineWidth = Math.max(0.8, it.w * 0.26);
      for (i = 1; i < n; i += 2) {
        var lx = pts[i][0], ly = pts[i][1];
        var ax = pts[Math.min(n, i + 1)][0] - pts[i - 1][0];
        var ay = pts[Math.min(n, i + 1)][1] - pts[i - 1][1];
        ctx.save();
        ctx.translate(lx, ly);
        ctx.rotate(Math.atan2(ay, ax));
        ctx.beginPath();
        ctx.ellipse(0, 0, it.w * 0.72, it.w * 0.4, 0, 0, Math.PI * 2);
        ctx.stroke();
        ctx.restore();
      }
    } else {
      // A vine. The first go at this was ellipses with a line through
      // them alternating strictly left and right, which is the drawing a
      // child makes of a plant: every leaf the same, all the same green,
      // all the same way up, on a stem of constant thickness.
      //
      // What makes a real one read: the stem tapers and is lit along one
      // side; leaves have a pointed tip, a rounded base, a stalk holding
      // them off the stem and a serrated edge; no two are the same size,
      // the same green or at the same angle; and there are tendrils,
      // which are the detail the eye actually uses to say "vine".
      //
      // A tapered body over the uniform stroke. Woody and dark along the
      // bottom, lit along the top, thickest in the middle of the loop
      // where it is nearest and thinning into the paper at both ends.
      var wide = function (u) {
        return it.w * (0.34 + 1.05 * Math.sin(Math.min(1, Math.max(0, u)) * Math.PI)) *
               (0.82 + 0.36 * fbm1(it.seed * 3 + u * 6, 2));
      };
      var ribbon = function (colour, off, scale, a2) {
        ctx.globalAlpha = alpha * a2;
        ctx.fillStyle = colour;
        ctx.beginPath();
        var j;
        for (j = 0; j <= n; j++) {
          var e = edgeAt(j), ww = wide(j / n) * scale;
          ctx.lineTo(pts[j][0] + e[0] * (off - ww), pts[j][1] + e[1] * (off - ww));
        }
        for (j = n; j >= 0; j--) {
          var e2 = edgeAt(j), w2 = wide(j / n) * scale;
          ctx.lineTo(pts[j][0] + e2[0] * (off + w2), pts[j][1] + e2[1] * (off + w2));
        }
        ctx.closePath();
        ctx.fill();
      };
      ribbon(it.dark, it.w * 0.18, 1.05, 0.95);
      ribbon(it.rope, 0, 0.9, 1);
      ribbon(it.glow, -it.w * 0.34, 0.26, 0.5);

      // Bark: short dark flecks along the stem, denser where it is fat.
      ctx.strokeStyle = it.dark;
      ctx.lineWidth = 0.7;
      ctx.globalAlpha = alpha * 0.45;
      for (i = 1; i < n; i++) {
        if (nhash(it.seed * 5 + i) > 0.55) { continue; }
        var be = edgeAt(i), bw = wide(i / n);
        var bo = (nhash(it.seed * 9 + i) - 0.5) * 1.5 * bw;
        ctx.beginPath();
        ctx.moveTo(pts[i][0] + be[0] * bo, pts[i][1] + be[1] * bo);
        ctx.lineTo(pts[i + 1 > n ? n : i + 1][0] + be[0] * bo,
                   pts[i + 1 > n ? n : i + 1][1] + be[1] * bo);
        ctx.stroke();
      }

      // The leaves. Placed on a seeded walk rather than every third node,
      // so they cluster and gap the way growth does, and the side they
      // take is a coin rather than a strict alternation.
      var step = 3;
      for (i = 2; i < n - 1; i += step) {
        step = 3 + Math.floor(nhash(it.seed * 21 + i) * 3);
        var e3 = edgeAt(i);
        var ang = Math.atan2(pts[i + 1 > n ? n : i + 1][1] - pts[i - 1][1],
                             pts[i + 1 > n ? n : i + 1][0] - pts[i - 1][0]);
        var side = nhash(it.seed * 33 + i) < 0.5 ? 1 : -1;
        // Off the stem and along it: leaves lean towards the growing tip
        // rather than sticking out square.
        var tilt = side * (0.55 + nhash(it.seed * 41 + i) * 0.75);
        // Sized off the stem, not off the page: a leaf wider than the
        // dice is a bush, and the dice are the thing being looked at.
        var leaf = it.w * (0.75 + 1.0 * along[i]) *
                   (0.55 + 0.9 * nhash(it.seed * 7 + i));
        self.drawLeaf(ctx, pts[i][0] + e3[0] * side * wide(i / n) * 0.7,
                      pts[i][1] + e3[1] * side * wide(i / n) * 0.7,
                      ang + tilt, leaf, it, it.seed * 13 + i, alpha);
      }

      // Tendrils: two or three coils spiralling off the stem. Nothing
      // else in the drawing says "climbing plant" as fast as these do.
      ctx.lineCap = 'round';
      for (i = 0; i < 3; i++) {
        var tAt = 4 + Math.floor(nhash(it.seed * 55 + i) * (n - 9));
        var te = edgeAt(tAt);
        var tSide = nhash(it.seed * 61 + i) < 0.5 ? 1 : -1;
        var tLen = it.w * (2.4 + nhash(it.seed * 67 + i) * 2.6) * (0.4 + along[tAt]);
        var turns = 1.6 + nhash(it.seed * 71 + i) * 1.3;
        ctx.save();
        ctx.translate(pts[tAt][0] + te[0] * tSide * it.w * 0.6,
                      pts[tAt][1] + te[1] * tSide * it.w * 0.6);
        ctx.globalAlpha = alpha * 0.9;
        ctx.strokeStyle = it.rope;
        ctx.lineWidth = Math.max(0.7, it.w * 0.22);
        ctx.beginPath();
        for (var q = 0; q <= 30; q++) {
          var qu = q / 30;
          // A spiral that opens as it goes, which is what a tendril that
          // has not found anything to hold does.
          var rr = tLen * (0.12 + qu * 0.5);
          var th = qu * turns * Math.PI * 2 * tSide;
          var sx = Math.cos(ang0(te, tSide)) * tLen * qu * 0.8 +
                   Math.cos(th) * rr * 0.55;
          var sy = Math.sin(ang0(te, tSide)) * tLen * qu * 0.8 * 0.72 +
                   Math.sin(th) * rr * 0.4;
          if (q) { ctx.lineTo(sx, sy); } else { ctx.moveTo(sx, sy); }
        }
        ctx.stroke();
        ctx.restore();
      }
    }
    // A bright thread along the very top, over the surface detail. On a
    // rope or a chain that is the highlight; on a vine an unbroken white
    // line down every stem is the one thing left that reads as plastic,
    // so it is dimmed to a sheen there.
    stroke(it.glow, it.w * (it.lay === 'leaves' ? 0.16 : 0.3),
           it.lay === 'leaves' ? 0.32 : 0.9);
    ctx.restore();
  };

  // ----------------------------------------------------------- the slash
  //
  // A blade going through the page.
  //
  // The shape is the classic one: a crescent, fat in the middle and
  // pointed at both ends, swung along an arc. What sells it is that it
  // is not drawn all at once - the leading edge runs ahead down the arc
  // and the tail follows it, so the eye reads a single fast movement
  // rather than a shape being switched on. Behind it a much thinner
  // line stays on the paper for a moment, which is the cut itself.
  //
  // Drawn flat in screen space rather than standing up in the world.
  // A sword swing across a character sheet is a mark on the sheet, and
  // leaning it into the elemental camera would only make it a mark on a
  // sheet seen from an angle.
  DiceBoard.prototype.drawSlash = function (ctx, it, t, alpha) {
    // Out fast and then done: the whole swing is over in the first fifth
    // of the item's life and the rest of it is the glow going out.
    var run = Math.min(1, t / 0.3);
    var lead = 1 - Math.pow(1 - run, 3);
    // A swing that hit something stops there, and shivers a little
    // where it stopped rather than simply ending.
    if (it.stopAt && lead > it.stopAt) {
      lead = it.stopAt + Math.sin(t * 60) * 0.012 * Math.max(0, 1 - t * 4);
    }
    // The tail follows a beat behind, so the crescent has a length
    // rather than growing from nothing at a fixed point.
    var tail = Math.max(0, 1 - Math.pow(1 - Math.max(0, (t - 0.1) / 0.3), 3));
    if (lead <= tail) { return; }

    var p = this.project([it.x, it.y, 0]);
    var R = it.r * p[2], W = it.w * p[2];
    var fade = t < 0.3 ? 1 : Math.max(0, 1 - (t - 0.3) / 0.45);

    var self = this;
    // One crescent between two points along the arc, at a given width.
    var crescent = function (from, to, fat, colour, a2) {
      var n = 26, i, u, a, hw, outer = [], inner = [];
      for (i = 0; i <= n; i++) {
        u = from + (to - from) * (i / n);
        a = it.a0 + it.sweep * u;
        // Pointed at both ends of the whole swing, not of this segment,
        // so a partly drawn crescent still tapers the way it will when
        // it is finished.
        hw = W * fat * Math.pow(Math.sin(Math.PI * u), 0.62);
        outer.push([p[0] + Math.cos(a) * (R + hw),
                    p[1] + Math.sin(a) * (R + hw) * it.squash]);
        inner.push([p[0] + Math.cos(a) * (R - hw),
                    p[1] + Math.sin(a) * (R - hw) * it.squash]);
      }
      ctx.globalAlpha = alpha * a2 * fade;
      ctx.fillStyle = colour;
      ctx.beginPath();
      ctx.moveTo(outer[0][0], outer[0][1]);
      for (i = 1; i <= n; i++) { ctx.lineTo(outer[i][0], outer[i][1]); }
      for (i = n; i >= 0; i--) { ctx.lineTo(inner[i][0], inner[i][1]); }
      ctx.closePath();
      ctx.fill();
    };

    ctx.save();
    // Wide and coloured underneath, then the body, then a white core.
    // As everywhere else on this page the halo is painted rather than
    // added, because there is no headroom above cream parchment.
    crescent(tail, lead, 2.6, it.glow, 0.16);
    crescent(tail, lead, 1.5, it.glow, 0.3);
    crescent(tail, lead, 1.0, it.face, 0.85);
    crescent(tail, lead, 0.34, it.core, 1);

    // The leading edge: a bright bead at the front of the swing, which
    // is where a real blade would be.
    if (run < 1) {
      var la = it.a0 + it.sweep * lead;
      var lp = [p[0] + Math.cos(la) * R, p[1] + Math.sin(la) * R * it.squash];
      var lg = ctx.createRadialGradient(lp[0], lp[1], 0, lp[0], lp[1], W * 1.5);
      lg.addColorStop(0, it.core);
      lg.addColorStop(0.4, it.face);
      lg.addColorStop(1, 'rgba(0,0,0,0)');
      ctx.globalAlpha = alpha * (1 - run) * 0.9;
      ctx.fillStyle = lg;
      ctx.beginPath();
      ctx.arc(lp[0], lp[1], W * 1.5, 0, Math.PI * 2);
      ctx.fill();
    }
    ctx.restore();
    ctx.globalAlpha = 1;
  };

  // The cut left on the paper afterwards: a hairline along the same arc,
  // with the edges of it slightly parted, that stays a beat longer than
  // the light does and then goes.
  DiceBoard.prototype.drawScar = function (ctx, it, t, alpha) {
    var p = this.project([it.x, it.y, 0]);
    var R = it.r * p[2];
    var n = 30, i, side;
    ctx.save();
    // Two lines a whisker apart, because a cut has two edges - and the
    // gap between them opens in the middle where the blade went deepest.
    for (side = -1; side <= 1; side += 2) {
      ctx.globalAlpha = alpha * (side < 0 ? 0.8 : 0.5);
      ctx.strokeStyle = side < 0 ? it.ink : it.face;
      ctx.lineWidth = Math.max(1.1, 3.2 * p[2]);
      ctx.lineCap = 'round';
      ctx.beginPath();
      var far2 = it.stopAt || 1;
      for (i = 0; i <= n; i++) {
        var u = (i / n) * far2;
        var a = it.a0 + it.sweep * u;
        var off = side * 2.6 * p[2] * Math.sin(Math.PI * u);
        var rr = R + off;
        var x = p[0] + Math.cos(a) * rr;
        var y = p[1] + Math.sin(a) * rr * it.squash;
        if (i) { ctx.lineTo(x, y); } else { ctx.moveTo(x, y); }
      }
      ctx.stroke();
    }
    ctx.restore();
    ctx.globalAlpha = 1;
  };

  // ------------------------------------------------- a low poly panther
  //
  // A real mesh of a cat, built out of tubes.
  //
  // Every part of an animal is a chain of rings: the torso is a wide
  // one, a leg is a narrow one, the tail is a narrow one that tapers to
  // nothing. So there is one routine that walks a path, puts a ring of
  // points round each point of it, and stitches consecutive rings into
  // quads - and the whole cat is nine calls to it.
  //
  // Six sides to a ring, which is what makes it low poly: the facets
  // are big enough to see, every one is flat shaded from its own
  // normal, and the creases between them do all the drawing. A rounder
  // cat would need smooth shading and would stop looking like this.

  function meshBuilder() {
    return {
      verts: [], faces: [],
      // One tube along `path`, where each entry is a centre and a pair
      // of radii (across, up). `twistUp` keeps every ring's "up" the
      // same, so the tube does not corkscrew along its length.
      // `upHint` is which way is up for the rings. Without one the
      // frame is guessed from the direction of travel, and for a tube
      // running along z the guess puts the ring's wide axis across
      // the wrong pair of axes - which turns a flat palm into a
      // vertical slab and every finger on its edge. Anything whose
      // cross-section is not round wants to say.
      tube: function (path, sides, capA, capB, upHint) {
        var self = this, i, k;
        var first = this.verts.length;
        var rings = [];
        for (i = 0; i < path.length; i++) {
          var c = path[i].c;
          // The way the tube is going at this point.
          var prev = path[Math.max(0, i - 1)].c;
          var next = path[Math.min(path.length - 1, i + 1)].c;
          var dir = norm(sub(next, prev));
          if (!isFinite(dir[0])) { dir = [1, 0, 0]; }
          // A frame square to it. Up is what the caller asked for, or
          // world up unless the tube is going straight up, in which
          // case forward will do.
          var upv = upHint ||
                    (Math.abs(dir[2]) > 0.95 ? [1, 0, 0] : [0, 0, 1]);
          var side = norm(cross(dir, upv));
          var top = norm(cross(side, dir));
          var ring = [];
          for (k = 0; k < sides; k++) {
            var a = (k / sides) * Math.PI * 2;
            var p = add(c, add(scale(side, Math.cos(a) * path[i].r[0]),
                               scale(top, Math.sin(a) * path[i].r[1])));
            ring.push(self.verts.length);
            self.verts.push(p);
          }
          rings.push(ring);
        }
        for (i = 0; i < rings.length - 1; i++) {
          for (k = 0; k < sides; k++) {
            var k2 = (k + 1) % sides;
            this.faces.push([rings[i][k], rings[i][k2],
                             rings[i + 1][k2], rings[i + 1][k]]);
          }
        }
        // Caps, as one flat polygon each rather than a fan to a point
        // on the axis. The fan was a ring of very thin triangles, and
        // the wide glow pass the wireframe draws over every edge
        // turned each of them into a spike - every fingertip came out
        // with a starburst on it.
        if (capA) { this.faces.push(rings[0].slice().reverse()); }
        if (capB) { this.faces.push(rings[rings.length - 1].slice()); }
        return first;
      }
    };
  }

  // A leg, from a shoulder or a hip: three bones, each turned further
  // than the last, tapering to a paw.
  function catLeg(mb, at, swing, bend, len, thick, side) {
    var a = swing, p = [at[0], at[1], at[2]], path = [];
    var bones = [len * 0.42, len * 0.36, len * 0.22];
    var rad = [thick, thick * 0.78, thick * 0.6, thick * 0.55];
    path.push({ c: p.slice(), r: [rad[0] * 1.25, rad[0] * 1.4] });
    for (var b = 0; b < 3; b++) {
      a += b === 0 ? bend * 0.85 : -bend * 1.15;
      p = [p[0] + Math.cos(a) * bones[b], p[1],
           p[2] - Math.abs(Math.sin(a)) * bones[b] - bones[b] * 0.35];
      path.push({ c: p.slice(), r: [rad[b + 1], rad[b + 1] * 1.1] });
    }
    // The paw: forward and flat.
    path.push({ c: [p[0] + len * 0.13, p[1], p[2] - len * 0.05],
                r: [thick * 0.62, thick * 0.45] });
    mb.tube(path, 6, true, true);
    return p;
  }

  // The whole animal, in its own space: +x is forward out of the nose,
  // +z is up, y is across. One unit is about a shoulder height.
  function catMesh(reach) {
    var mb = meshBuilder();
    var i;
    // ---- the torso, rump to withers. Deep through the chest, tucked
    // at the waist, and the haunch the widest part of it.
    mb.tube([
      { c: [-1.18, 0, 0.52], r: [0.22, 0.20] },
      { c: [-0.92, 0, 0.60], r: [0.34, 0.33] },
      { c: [-0.60, 0, 0.60], r: [0.33, 0.32] },
      { c: [-0.22, 0, 0.56], r: [0.26, 0.29] },
      { c: [0.18, 0, 0.56], r: [0.27, 0.32] },
      { c: [0.56, 0, 0.58], r: [0.31, 0.36] },
      { c: [0.86, 0, 0.56], r: [0.27, 0.31] }
    ], 8, true, false);
    // ---- the neck and the head.
    mb.tube([
      { c: [0.86, 0, 0.56], r: [0.25, 0.28] },
      { c: [1.12, 0, 0.54], r: [0.21, 0.23] },
      { c: [1.34, 0, 0.50], r: [0.21, 0.22] },
      { c: [1.56, 0, 0.48], r: [0.19, 0.18] },
      { c: [1.74, 0, 0.45], r: [0.13, 0.12] },
      { c: [1.84, 0, 0.43], r: [0.09, 0.08] }
    ], 8, false, true);
    // ---- the ears, two little cones off the back of the skull.
    for (i = -1; i <= 1; i += 2) {
      mb.tube([
        { c: [1.44, i * 0.13, 0.60], r: [0.09, 0.09] },
        { c: [1.38, i * 0.18, 0.74], r: [0.02, 0.02] }
      ], 5, true, true);
    }
    // ---- the legs. Front pair reaching forward, back pair thrown out
    // behind, and the further of each pair a shade thinner.
    catLeg(mb, [0.62, -0.20, 0.42], -1.25 + reach * 1.05, 0.55 - reach * 0.8,
           0.96, 0.115, -1);
    catLeg(mb, [0.62, 0.20, 0.42], -1.15 + reach * 1.0, 0.5 - reach * 0.75,
           0.96, 0.125, 1);
    catLeg(mb, [-0.86, -0.19, 0.44], 1.0 + reach * 0.9, -0.5 + reach * 0.95,
           1.02, 0.125, -1);
    catLeg(mb, [-0.86, 0.19, 0.44], 1.05 + reach * 0.85, -0.45 + reach * 0.9,
           1.02, 0.135, 1);
    // ---- the tail: long, heavy at the root, whipping up at the end.
    mb.tube([
      { c: [-1.16, 0, 0.56], r: [0.10, 0.10] },
      { c: [-1.52, 0.04, 0.62], r: [0.082, 0.082] },
      { c: [-1.88, 0.02, 0.66], r: [0.065, 0.065] },
      { c: [-2.22, -0.04, 0.76], r: [0.05, 0.05] },
      { c: [-2.48, -0.06, 0.92], r: [0.032, 0.032] }
    ], 6, true, true);
    return mb;
  }

  // ------------------------------------------------------- low poly rock
  //
  // A real mesh, not a drawing of one.
  //
  // The stones in the cairn used to be irregular polygons with a few
  // lines scribbled on them, which is a picture of a rock. This builds
  // an actual solid: an icosahedron, optionally subdivided, with every
  // vertex pushed in or out along its own direction by noise, so the
  // result is a lumpy convex-ish boulder of flat triangles. Then it is
  // turned, projected, back-faces dropped, and each face filled with
  // one flat tone from its own normal - which is exactly what makes
  // low-poly rock look like low-poly rock: no gradients anywhere, and
  // a visible crease at every edge because the two faces either side
  // of it caught the light differently.
  //
  // The mesh is made once per stone and then only transformed, so the
  // rock keeps its shape while it tumbles instead of boiling.

  // The twenty faces of an icosahedron, worked out rather than typed:
  // any three vertices that are all one edge-length apart are a face.
  var ICO = (function () {
    var v = permute(0, 1, PHI, true), f = [], i, j, k;
    var d2 = function (a, b) {
      var dx = a[0] - b[0], dy = a[1] - b[1], dz = a[2] - b[2];
      return dx * dx + dy * dy + dz * dz;
    };
    // The shortest distance between any two vertices is the edge.
    var e = Infinity;
    for (i = 0; i < v.length; i++) {
      for (j = i + 1; j < v.length; j++) { e = Math.min(e, d2(v[i], v[j])); }
    }
    for (i = 0; i < v.length; i++) {
      for (j = i + 1; j < v.length; j++) {
        if (Math.abs(d2(v[i], v[j]) - e) > 0.001) { continue; }
        for (k = j + 1; k < v.length; k++) {
          if (Math.abs(d2(v[i], v[k]) - e) > 0.001) { continue; }
          if (Math.abs(d2(v[j], v[k]) - e) > 0.001) { continue; }
          f.push([i, j, k]);
        }
      }
    }
    return { verts: v.map(norm), faces: f };
  })();

  // One rock. `rough` is how far the vertices wander, `squash` how far
  // off a ball it is, `fine` whether the icosahedron is subdivided once
  // into eighty faces or left at twenty.
  function rockMesh(seed, rough, squash, fine) {
    var verts = ICO.verts.map(function (p) { return p.slice(); });
    var faces = ICO.faces.map(function (f) { return f.slice(); });
    var i, k;

    if (fine) {
      // Subdivide: every triangle into four, the new vertices pushed
      // back out onto the sphere so it stays round before the noise
      // gets at it.
      var mid = {}, out = [];
      var midpoint = function (a, b) {
        var key = Math.min(a, b) + ':' + Math.max(a, b);
        if (mid[key] === undefined) {
          mid[key] = verts.length;
          verts.push(norm(scale(add(verts[a], verts[b]), 0.5)));
        }
        return mid[key];
      };
      for (i = 0; i < faces.length; i++) {
        var a = faces[i][0], b = faces[i][1], c = faces[i][2];
        var ab = midpoint(a, b), bc = midpoint(b, c), ca = midpoint(c, a);
        out.push([a, ab, ca], [ab, b, bc], [ca, bc, c], [ab, bc, ca]);
      }
      faces = out;
    }

    // Push every vertex in or out along its own direction. Three
    // octaves, so the rock has a big lopsided shape, a few shelves in
    // it, and a bit of chip on the small edges.
    for (i = 0; i < verts.length; i++) {
      var p = verts[i];
      var n = 1;
      for (k = 0; k < 3; k++) {
        var f2 = Math.pow(2.3, k);
        n += (fbm1(seed * (k + 1) * 3.7 +
                   (p[0] + 2) * 5.1 * f2 + (p[1] + 2) * 3.3 * f2 +
                   (p[2] + 2) * 7.9 * f2, 2) - 0.5) * rough / (k + 1);
      }
      // Squashed, so it sits like a stone rather than floating like a
      // ball, and a flat bottom on some of them.
      verts[i] = [p[0] * n, p[1] * n * squash[0], p[2] * n * squash[1]];
      if (verts[i][2] < -0.72) { verts[i][2] = -0.72; }
    }
    return { verts: verts, faces: faces };
  }

  function rotate3(p, yaw, pitch, roll) {
    var cy = Math.cos(yaw), sy = Math.sin(yaw);
    var cp = Math.cos(pitch), sp = Math.sin(pitch);
    var cr = Math.cos(roll), sr = Math.sin(roll);
    var x = p[0] * cy - p[1] * sy, y = p[0] * sy + p[1] * cy, z = p[2];
    var y2 = y * cp - z * sp; z = y * sp + z * cp;
    var x2 = x * cr - z * sr; z = x * sr + z * cr;
    return [x2, y2, z];
  }

  // The light everything stony is lit by: over the reader's left
  // shoulder and a little in front, which is where the rest of the
  // sheet's shading assumes it is.
  var ROCK_LIGHT = norm([-0.45, -0.55, 0.7]);

  // Draw one mesh at a world point. Back-faces dropped, faces painted
  // far to near, and then either flat shaded like stone or drawn as a
  // glowing wireframe like something conjured.
  //
  // The two modes are the same geometry and the same lighting; what
  // changes is whether the light goes into the face or into its edges.
  DiceBoard.prototype.drawMesh = function (ctx, mesh, at, size, rot,
                                           look, alpha) {
    var self = this, i, k;
    var world = mesh.verts.map(function (v) {
      var r = rotate3(v, rot[0], rot[1], rot[2]);
      return [at[0] + r[0] * size[0], at[1] + r[1] * size[1],
              at[2] + r[2] * size[2]];
    });
    var screen = world.map(function (w) { return self.projectUp(w); });

    var draw = [];
    for (i = 0; i < mesh.faces.length; i++) {
      var f = mesh.faces[i];
      var a = screen[f[0]], b = screen[f[1]], c = screen[f[2]];
      // Back-face cull on the screen winding: for a closed solid this
      // is both the cheapest test and the right one.
      //
      // A shield is not closed - it is a dished plate, and its face
      // and its rim are wound against each other - so it asks for
      // both sides and leans on the depth sort instead. A face turned
      // away has its normal flipped for the lighting, or the back of
      // the board would be lit as though you could see through it.
      var area = (b[0] - a[0]) * (c[1] - a[1]) - (c[0] - a[0]) * (b[1] - a[1]);
      var away = area >= 0;
      if (away && !look.twoSided) { continue; }
      var wa = world[f[0]], wb = world[f[1]], wc = world[f[2]];
      var nrm = norm(cross(sub(wb, wa), sub(wc, wa)));
      if (away) { nrm = scale(nrm, -1); }
      draw.push({ f: f, n: nrm, depth: (a[1] + b[1] + c[1]) / 3, i: i });
    }
    // Far first. Culling alone is enough for a convex solid and these
    // are only nearly convex, so this is the cheap insurance.
    draw.sort(function (p, q) { return p.depth - q.depth; });

    var wire = look.mode === 'wire';
    for (i = 0; i < draw.length; i++) {
      var d = draw[i];
      // Flat shading, one tone for the whole face. A half-Lambert, so
      // the faces pointing away are dark but not black - anything in
      // the open is lit by the sky as well as by the sun.
      var lit = dot(d.n, ROCK_LIGHT) * 0.5 + 0.5;
      lit = 0.16 + Math.pow(lit, 1.35) * 0.84;
      // And a little per-face grit, so two faces at the same angle are
      // still two different faces.
      lit *= 0.9 + 0.2 * nhash(d.i * 7.3 + mesh.verts.length);
      lit = Math.max(0, Math.min(1, lit));

      ctx.beginPath();
      var p0 = screen[d.f[0]];
      ctx.moveTo(p0[0], p0[1]);
      for (k = 1; k < d.f.length; k++) {
        var pk = screen[d.f[k]];
        ctx.lineTo(pk[0], pk[1]);
      }
      ctx.closePath();

      if (wire) {
        // Something conjured: the facet itself is barely there, and
        // all the light is in the edges. Filled first anyway, faintly,
        // because a wireframe with nothing behind it is a tangle - the
        // fill is what hides the far side of the solid and lets the
        // shape read.
        ctx.globalAlpha = alpha * (0.1 + lit * 0.34);
        ctx.fillStyle = look.fill;
        ctx.fill();
        // A soft wide pass and a hard thin one, so the edges bloom.
        ctx.globalAlpha = alpha * (0.06 + lit * 0.16);
        ctx.strokeStyle = look.glow;
        ctx.lineWidth = 3.4;
        ctx.stroke();
        ctx.globalAlpha = alpha * (0.3 + lit * 0.7);
        ctx.strokeStyle = look.edge;
        ctx.lineWidth = 1;
        ctx.stroke();
      } else {
        var col = mixRock(look, lit);
        ctx.globalAlpha = alpha;
        ctx.fillStyle = col;
        ctx.strokeStyle = col;
        ctx.lineWidth = 0.7;
        ctx.fill();
        // Stroked in its own colour, because a hairline of background
        // shows between two filled triangles otherwise.
        ctx.stroke();
      }
    }
    // The nodes: a point of light at every vertex that faced us. This
    // is the detail that makes a wireframe read as a conjured thing
    // rather than as a drawing of one.
    if (wire && look.nodes) {
      var seen = {};
      ctx.fillStyle = look.node;
      for (i = 0; i < draw.length; i++) {
        for (k = 0; k < draw[i].f.length; k++) {
          var vi = draw[i].f[k];
          if (seen[vi]) { continue; }
          seen[vi] = 1;
          var sp = screen[vi];
          ctx.globalAlpha = alpha * 0.85;
          ctx.beginPath();
          ctx.arc(sp[0], sp[1], 1.5, 0, Math.PI * 2);
          ctx.fill();
        }
      }
    }
    ctx.globalAlpha = 1;
  };

  // Stone still calls it by its old name.
  DiceBoard.prototype.drawRockMesh = function (ctx, mesh, at, size, rot,
                                               look, alpha) {
    this.drawMesh(ctx, mesh, at, size, rot, look, alpha);
  };

  // ---------------------------------------------------- a low poly hand
  //
  // The same tubes as the cat. A palm is one short wide tube, a finger
  // is a narrow one with three bends in it, and a gesture is the set of
  // angles those bends are at - so the six gestures are six numbers
  // each rather than six drawings.
  // A hand is laid out in the x-z plane and is flat front to back, so
  // every ring wants its wide axis along x and its narrow one along y.
  // This is the vector that makes the frame come out that way round.
  var HAND_UP = [0, -1, 0];

  function handMesh(gesture) {
    var mb = meshBuilder();
    var g = HAND_GESTURES[gesture];
    var i;
    // The palm: wrist to knuckles, flattened front to back the way a
    // hand is, and wider at the knuckles than at the wrist.
    mb.tube([
      { c: [0, 0, -0.25], r: [0.26, 0.11] },
      { c: [0, 0, 0.05], r: [0.33, 0.13] },
      { c: [0, 0, 0.55], r: [0.38, 0.14] },
      { c: [0, 0, 1.0], r: [0.40, 0.13] }
    ], 8, true, true, HAND_UP);
    // The thenar pad, which is most of the width of a hand.
    mb.tube([
      { c: [-0.24, 0, 0.0], r: [0.13, 0.1] },
      { c: [-0.36, 0, 0.36], r: [0.15, 0.11] },
      { c: [-0.34, 0, 0.7], r: [0.1, 0.08] }
    ], 6, true, true, HAND_UP);
    // Five fingers. Each bone turns further than the last, and the
    // curl comes straight out of the gesture table.
    for (i = 0; i < HAND_FINGERS.length; i++) {
      var f = HAND_FINGERS[i];
      var curl = Math.min(1.25, g.curl[i]) * 0.62;
      var a = f.aim * g.spread;
      var x = f.at[0], z = f.at[1];
      var path = [{ c: [x, 0, z], r: [f.w * 0.5, f.w * 0.42] }];
      for (var b = 0; b < 3; b++) {
        a += curl * (b === 0 ? 0.85 : 1.25);
        x += Math.sin(a) * f.bones[b];
        z += Math.cos(a) * f.bones[b];
        var w = f.w * HAND_TAPER[b * 2 + 2];
        path.push({ c: [x, 0, z], r: [w, w * 0.85] });
      }
      mb.tube(path, 6, true, true, HAND_UP);
    }
    return mb;
  }


  // Three stops rather than two: rock does not fade evenly from its
  // shadow to its highlight, it goes through a warmer middle.
  function mixRock(look, t) {
    var a, b, u;
    if (t < 0.5) { a = look.shade; b = look.mid; u = t * 2; }
    else { a = look.mid; b = look.lit; u = (t - 0.5) * 2; }
    return 'rgb(' + Math.round(a[0] + (b[0] - a[0]) * u) + ',' +
           Math.round(a[1] + (b[1] - a[1]) * u) + ',' +
           Math.round(a[2] + (b[2] - a[2]) * u) + ')';
  }

  // ------------------------------------------------- stoneskin, barkskin
  //
  // Pieces come up through the paper all over the sheet, fly in, and
  // stack themselves into a figure - and then the figure moves.
  //
  // The slots are an inukshuk: two legs, a slab across them, a body, two
  // arms out to the sides and a head on top. Each slot says where the
  // piece sits, how big it is and how it is turned; the piece itself is
  // an irregular polygon made from its own seed, so a cairn of eleven
  // stones is eleven stones rather than one drawn eleven times.
  var CAIRN_SLOTS = [
    { x: -0.42, y: 0.16, w: 0.30, h: 0.34, rot: 0.06 },
    { x: 0.42, y: 0.16, w: 0.30, h: 0.34, rot: -0.05 },
    { x: -0.40, y: 0.56, w: 0.28, h: 0.30, rot: -0.08 },
    { x: 0.40, y: 0.56, w: 0.28, h: 0.30, rot: 0.07 },
    { x: 0.00, y: 0.88, w: 0.92, h: 0.19, rot: 0.02 },
    { x: 0.00, y: 1.20, w: 0.44, h: 0.32, rot: -0.03 },
    { x: 0.00, y: 1.54, w: 0.52, h: 0.22, rot: 0.04 },
    { x: -0.78, y: 1.58, w: 0.34, h: 0.15, rot: 0.1 },
    { x: 0.78, y: 1.58, w: 0.34, h: 0.15, rot: -0.09 },
    { x: 0.00, y: 1.88, w: 0.30, h: 0.28, rot: 0.03 },
    { x: 0.00, y: 2.18, w: 0.22, h: 0.2, rot: -0.06 }
  ];

  DiceBoard.prototype.drawCairn = function (ctx, it, grown, alpha, now) {
    var i, age = (now - it.born) / 1000;
    // Three acts: the pieces fly in, the figure settles, the figure
    // flexes. The flex is what makes it a creature rather than a pile.
    var flyT = Math.min(1, Math.max(0, (age - 0.15) / 0.85));
    var ease = 1 - Math.pow(1 - flyT, 3);
    var flexT = Math.max(0, age - 1.15);
    var flex = Math.sin(flexT * 3.1) * Math.min(1, flexT * 1.6);
    var s = it.r;

    for (i = 0; i < it.rocks.length; i++) {
      var rk = it.rocks[i];
      var slot = CAIRN_SLOTS[i % CAIRN_SLOTS.length];
      // Each piece has its own moment of arriving, so they land one
      // after another rather than all at once.
      var mine = Math.min(1, Math.max(0, (ease - rk.delay * 0.35) / 0.65));
      mine = mine * mine * (3 - 2 * mine);
      if (mine <= 0) { continue; }

      // The flex: arms swing, the body leans, the head rocks. Pieces
      // higher up move more, the way anything hinged at the feet does.
      var sx = slot.x, sz = slot.y;
      if (flexT > 0) {
        var lever = sz / 2.2;
        sx += flex * 0.22 * lever * (slot.x >= 0 ? 1 : -1) + flex * 0.1 * lever;
        sz += Math.abs(flex) * 0.1 * lever;
      }
      // Where it ends up. Mostly up the screen rather than up off the
      // page: this camera looks down, so a figure built in world z
      // would be seen from above and read as a pile rather than as
      // something standing. A little real height is kept so the light
      // still finds the tops of the stones.
      var tx = it.x + sx * s, ty = it.y - sz * s * 0.74, tz = 6 + sz * s * 0.26;
      // Where it came from: flat on the paper, somewhere else entirely.
      var px = rk.from[0] + (tx - rk.from[0]) * mine;
      var py = rk.from[1] + (ty - rk.from[1]) * mine;
      var pz = 2 + (tz - 2) * mine + Math.sin(mine * Math.PI) * rk.arc;

      // Spinning as it comes, and square by the time it lands.
      var rot = [rk.spin[0] * (1 - mine) + (slot.rot + flex * 0.14 *
                                            (sz / 2.2)) * mine,
                 rk.spin[1] * (1 - mine) + rk.rest[1] * mine,
                 rk.spin[2] * (1 - mine) + rk.rest[2] * mine];

      this.drawMesh(ctx, rk.mesh, [px, py, pz],
                    [s * slot.w, s * slot.w * 0.72, s * slot.h],
                    rot, it.look, alpha * (0.45 + 0.55 * mine));
    }
    ctx.globalAlpha = 1;
  };

  // ------------------------------------------------------- phantom beast
  //
  // A big cat coming out of the page at you.
  //
  // Drawn side-on and three-quarters, because a cat head-on is a circle
  // with ears; what reads as a pounce is the line of the thing - front
  // legs reaching, back legs trailing, spine stretched, tail streaming -
  // and then the whole silhouette growing as it closes on you, which is
  // the bit that makes it a pounce rather than a jump.
  //
  // Everything is one merged path filled once, the same as the hand: a
  // cat drawn as separate limbs has a seam at every shoulder.
  DiceBoard.prototype.drawPanther = function (ctx, it, grown, alpha, now) {
    var age = (now - it.born) / 1000;
    // Crouch, spring, fly, gone. The pounce is the scale: it trebles
    // on the way in and fades as it goes past your shoulder.
    var leap = Math.min(1, age / 1.05);
    var reach = Math.min(1, Math.max(0, (age - 0.14) / 0.42));
    var grow = 0.45 + Math.pow(leap, 1.7) * 2.1;
    var s = it.r * grow;
    // Up out of the hole, over the top of its arc, and down at you.
    var z = Math.sin(Math.min(1, leap * 1.1) * Math.PI) * it.h + 8;
    var fly = Math.pow(leap, 1.5);

    // The legs move, so the mesh is rebuilt when the pose has changed
    // enough to matter - twelve poses over the whole leap, which is
    // far more than the eye can follow and a fraction of the cost of
    // rebuilding it every frame.
    var step = Math.round(reach * 11);
    if (it.pose !== step) { it.pose = step; it.mesh = catMesh(step / 11); }

    // Standing it up.
    //
    // The elemental camera looks down at the page, so a mesh built
    // with its own z as "up" is seen from directly above - which for
    // a cat means you are looking at its back and it reads as a
    // column of legs. Pitching it a quarter turn puts the animal's up
    // along the screen's up instead, so you see it from the side the
    // way you would see a cat in a room; the yaw then swings it round
    // to three-quarters, which is what shows the chest and the head
    // at once. The pounce itself is the scale, not the angle.
    this.drawMesh(ctx, it.mesh,
                  [it.x + it.away[0] * fly, it.y + it.away[1] * fly, z],
                  [s, s, s],
                  [it.yaw, Math.PI / 2 - 0.42 + leap * 0.5, it.roll],
                  it.look, alpha);
  };

  // A plate of hexagonal energy laid flat on the paper and thrown
  // outward: the shield's own honeycomb, off the shield.
  //
  // Cells light up as a ring of force passes over them and go out
  // behind it, so the whole thing reads as something expanding rather
  // than as a picture getting bigger.
  DiceBoard.prototype.drawHexRing = function (ctx, it, t, alpha) {
    var self = this, i, k;
    var wave = Math.pow(t, 0.55) * it.r;
    var p = this.project([it.x, it.y, 0]);
    var hr = it.cell * p[2];
    ctx.save();
    ctx.translate(p[0], p[1]);
    ctx.scale(1, 0.44);
    ctx.rotate(it.spin * t);
    ctx.lineJoin = 'round';
    var reach = Math.ceil((it.r * p[2]) / (hr * 1.5)) + 1;
    for (var row = -reach; row <= reach; row++) {
      for (var col = -reach; col <= reach; col++) {
        var cx = col * hr * 1.73 + (row % 2 ? hr * 0.866 : 0);
        var cy = row * hr * 1.5;
        var d = Math.hypot(cx, cy);
        if (d > it.r * p[2]) { continue; }
        // How near the expanding front this cell is.
        var near = 1 - Math.abs(d - wave * p[2]) / (hr * 2.6);
        if (near <= 0) { continue; }
        var lit = Math.pow(near, 1.6) *
                  (0.6 + 0.5 * fbm1(it.seed + col * 5 + row * 3, 2));
        ctx.globalAlpha = alpha * Math.min(1, lit) * (1 - t * 0.6);
        ctx.beginPath();
        for (k = 0; k <= 6; k++) {
          var ha = (k / 6) * Math.PI * 2 + Math.PI / 6;
          var hx = cx + Math.cos(ha) * hr * 0.9;
          var hy = cy + Math.sin(ha) * hr * 0.9;
          if (k) { ctx.lineTo(hx, hy); } else { ctx.moveTo(hx, hy); }
        }
        ctx.closePath();
        ctx.fillStyle = it.face;
        ctx.globalAlpha = alpha * Math.min(0.5, lit * 0.45) * (1 - t * 0.6);
        ctx.fill();
        ctx.globalAlpha = alpha * Math.min(1, lit) * (1 - t * 0.6);
        ctx.strokeStyle = it.edge;
        ctx.lineWidth = 1 + 1.6 * near;
        ctx.stroke();
      }
    }
    ctx.restore();
    // The front itself, as a hard ring.
    this.drawGroundRing(ctx, it.x, it.y, wave, it.rim, 7,
                        alpha * (1 - t) * 0.8);
    ctx.globalAlpha = 1;
  };

  // -------------------------------------------------------- shield spells
  //
  // A shield comes up out of the paper spinning, squares itself off, and
  // the blade that was coming stops dead on it.
  //
  // It is a conjured shield, so it is a real shield you can see through:
  // the board is a translucent pane, and everything that makes it read
  // as a shield - the rim, the boss, the banding, the rivets - is drawn
  // as light on that pane rather than as a painted surface. Over the top
  // of it goes an energy pattern, which is what says the thing was cast
  // rather than carried.
  //
  // Eight outlines and five patterns, so the same spell twice is very
  // rarely the same shield.
  var SHIELD_SHAPES = {
    // A heater: flat across the top, curving to a point at the bottom.
    heater: function (ctx, w, h) {
      ctx.moveTo(-w, -h);
      ctx.lineTo(w, -h);
      ctx.bezierCurveTo(w, h * 0.2, w * 0.6, h * 0.75, 0, h);
      ctx.bezierCurveTo(-w * 0.6, h * 0.75, -w, h * 0.2, -w, -h);
      ctx.closePath();
    },
    // A round one, very slightly out of true.
    round: function (ctx, w, h) {
      var i;
      for (i = 0; i <= 28; i++) {
        var a = (i / 28) * Math.PI * 2;
        var r = 1 + 0.02 * Math.sin(a * 5);
        var x = Math.cos(a) * w * r, y = Math.sin(a) * h * r;
        if (i) { ctx.lineTo(x, y); } else { ctx.moveTo(x, y); }
      }
      ctx.closePath();
    },
    // A kite: tall, rounded over the top, long point at the bottom.
    kite: function (ctx, w, h) {
      ctx.moveTo(0, -h);
      ctx.bezierCurveTo(w, -h * 0.9, w, -h * 0.2, w * 0.7, h * 0.1);
      ctx.bezierCurveTo(w * 0.45, h * 0.6, w * 0.16, h * 0.9, 0, h);
      ctx.bezierCurveTo(-w * 0.16, h * 0.9, -w * 0.45, h * 0.6, -w * 0.7, h * 0.1);
      ctx.bezierCurveTo(-w, -h * 0.2, -w, -h * 0.9, 0, -h);
      ctx.closePath();
    },
    // A buckler: small, round, mostly boss.
    buckler: function (ctx, w, h) {
      ctx.moveTo(w, 0);
      ctx.ellipse(0, 0, w, h, 0, 0, Math.PI * 2);
      ctx.closePath();
    },
    // A tower: a slab with the corners taken off.
    tower: function (ctx, w, h) {
      var c = w * 0.3;
      ctx.moveTo(-w + c, -h);
      ctx.lineTo(w - c, -h);
      ctx.quadraticCurveTo(w, -h, w, -h + c);
      ctx.lineTo(w, h - c);
      ctx.quadraticCurveTo(w, h, w - c, h);
      ctx.lineTo(-w + c, h);
      ctx.quadraticCurveTo(-w, h, -w, h - c);
      ctx.lineTo(-w, -h + c);
      ctx.quadraticCurveTo(-w, -h, -w + c, -h);
      ctx.closePath();
    },
    // A pavise: a tower with a hump down the middle and a gabled top.
    pavise: function (ctx, w, h) {
      ctx.moveTo(-w * 0.86, -h * 0.82);
      ctx.quadraticCurveTo(0, -h * 1.04, w * 0.86, -h * 0.82);
      ctx.lineTo(w * 0.92, h * 0.78);
      ctx.quadraticCurveTo(w * 0.9, h, w * 0.7, h);
      ctx.lineTo(-w * 0.7, h);
      ctx.quadraticCurveTo(-w * 0.9, h, -w * 0.92, h * 0.78);
      ctx.closePath();
    },
    // A targe: round with a scalloped rim, which is what the studs are
    // hammered through on the real ones.
    targe: function (ctx, w, h) {
      var i, n = 24;
      for (i = 0; i <= n; i++) {
        var a = (i / n) * Math.PI * 2;
        var r = 1 + 0.045 * Math.cos(a * 12);
        var x = Math.cos(a) * w * r, y = Math.sin(a) * h * r;
        if (i) { ctx.lineTo(x, y); } else { ctx.moveTo(x, y); }
      }
      ctx.closePath();
    },
    // A hoplon: round, but dished, so the far edge shows as a second
    // curve inside the first.
    hoplon: function (ctx, w, h) {
      ctx.moveTo(w, 0);
      ctx.ellipse(0, -h * 0.04, w, h, 0, 0, Math.PI * 2);
      ctx.closePath();
    }
  };

  // ------------------------------------------------- a low poly shield
  //
  // The shield outlines are canvas paths and there is no point having
  // them twice, so this walks one of them with a recorder in place of
  // a context and comes back with the boundary as points. Anything
  // SHIELD_SHAPES uses - lines, cubics, quadratics, an ellipse - it
  // knows how to sample.
  function pathPoints(fn, w, h) {
    var pts = [], cur = [0, 0], i;
    var rec = {
      moveTo: function (x, y) { cur = [x, y]; pts.push([x, y]); },
      lineTo: function (x, y) { cur = [x, y]; pts.push([x, y]); },
      bezierCurveTo: function (a, b, c, d, e, f) {
        var p = cur;
        for (i = 1; i <= 5; i++) {
          var t = i / 5, u = 1 - t;
          pts.push([u * u * u * p[0] + 3 * u * u * t * a + 3 * u * t * t * c +
                    t * t * t * e,
                    u * u * u * p[1] + 3 * u * u * t * b + 3 * u * t * t * d +
                    t * t * t * f]);
        }
        cur = [e, f];
      },
      quadraticCurveTo: function (a, b, c, d) {
        var p = cur;
        for (i = 1; i <= 3; i++) {
          var t = i / 3, u = 1 - t;
          pts.push([u * u * p[0] + 2 * u * t * a + t * t * c,
                    u * u * p[1] + 2 * u * t * b + t * t * d]);
        }
        cur = [c, d];
      },
      ellipse: function (cx, cy, rx, ry, rot, a0, a1) {
        for (i = 1; i <= 20; i++) {
          var a = a0 + (a1 - a0) * (i / 20);
          pts.push([cx + Math.cos(a) * rx, cy + Math.sin(a) * ry]);
        }
        cur = pts[pts.length - 1];
      },
      closePath: function () {}
    };
    fn(rec, w, h);
    return pts;
  }

  // A shield as a solid: the outline stepped inwards in rings, each
  // ring standing a little further out of the face, so the board is
  // dished the way a real one is - and every step is a visible band of
  // facets rather than a smooth curve, which is the whole look.
  //
  // Then the rim: the outer ring repeated behind the face and stitched
  // to it, which gives the board an edge you can see. Then the boss.
  function shieldMesh(shape, dish) {
    var mb = meshBuilder();
    var outline = pathPoints(SHIELD_SHAPES[shape] || SHIELD_SHAPES.heater,
                             0.8, 1);
    var n = outline.length, RINGS = 4, i, k;
    var rings = [];
    for (k = 0; k <= RINGS; k++) {
      var f = 1 - k / RINGS;
      var ring = [];
      for (i = 0; i < n; i++) {
        ring.push(mb.verts.length);
        // x across, y the depth out of the face, z up the shield.
        //
        // The dish is positive, which after the quarter turn that
        // stands the shield up puts the face out of the page towards
        // the reader. Negative had it dished into the paper, so you
        // were looking at the back of the board.
        mb.verts.push([outline[i][0] * f,
                       dish * (1 - f * f),
                       -outline[i][1] * f]);
      }
      rings.push(ring);
    }
    // The face, ring to ring.
    for (k = 0; k < RINGS; k++) {
      for (i = 0; i < n; i++) {
        var j = (i + 1) % n;
        mb.faces.push([rings[k][i], rings[k][j],
                       rings[k + 1][j], rings[k + 1][i]]);
      }
    }
    // The rim: the outline again, a little behind, stitched to the
    // front edge so the board has a thickness.
    var back = [];
    for (i = 0; i < n; i++) {
      back.push(mb.verts.length);
      mb.verts.push([outline[i][0] * 0.97, -0.1, -outline[i][1] * 0.97]);
    }
    for (i = 0; i < n; i++) {
      var j2 = (i + 1) % n;
      mb.faces.push([rings[0][j2], rings[0][i], back[i], back[j2]]);
    }
    // The boss: a little dome standing out of the middle of it.
    var bossR = 0.26, bt = [];
    for (k = 0; k <= 2; k++) {
      var bf = 1 - k / 2;
      var br = [];
      for (i = 0; i < 10; i++) {
        var a = (i / 10) * Math.PI * 2;
        br.push(mb.verts.length);
        mb.verts.push([Math.cos(a) * bossR * bf,
                       dish + 0.16 * (1 - bf * bf) + 0.04,
                       -0.1 + Math.sin(a) * bossR * bf]);
      }
      bt.push(br);
    }
    for (k = 0; k < 2; k++) {
      for (i = 0; i < 10; i++) {
        var j3 = (i + 1) % 10;
        mb.faces.push([bt[k][i], bt[k][j3], bt[k + 1][j3], bt[k + 1][i]]);
      }
    }
    return mb;
  }

  DiceBoard.prototype.drawShield = function (ctx, it, grown, alpha, now) {
    var age = (now - it.born) / 1000;
    // Up out of the paper spinning, slowing into square as it arrives.
    var rise = Math.min(1, age / 0.42);
    var ease = 1 - Math.pow(1 - rise, 3);
    var spin = (1 - ease) * it.spins * Math.PI * 2 + age * 0.3;
    var s = it.r * (0.35 + ease * 0.75);
    var z = it.z * ease;

    // Stood up on the screen and turned about its own upright axis,
    // so the spin is the board turning edge-on and back rather than a
    // picture being squashed. Everything the shield is - the dish,
    // the rim, the boss - is in the mesh, and the facets it is made
    // of are the point of it.
    this.drawMesh(ctx, it.mesh, [it.x, it.y, z], [s, s, s],
                  [0, Math.PI / 2 + Math.sin(age * 1.4) * 0.07, spin],
                  it.look, alpha);

    // The ward burning round the outside of it: the outline again,
    // flat, a shade larger, pulsing.
    var p = this.projectUp([it.x, it.y, z]);
    ctx.save();
    ctx.translate(p[0], p[1]);
    ctx.scale(s * p[2] * Math.abs(Math.cos(spin)), s * p[2]);
    ctx.globalAlpha = alpha * (0.22 + 0.18 * Math.sin(age * 4.6));
    ctx.strokeStyle = it.glow;
    ctx.lineWidth = 0.16;
    ctx.beginPath();
    (SHIELD_SHAPES[it.shape] || SHIELD_SHAPES.heater)(ctx, 0.86, 1.07);
    ctx.stroke();
    ctx.restore();
    ctx.globalAlpha = 1;
  };

  // The skin of a dome, as a list of panels in polar form - two
  // latitudes and two longitudes each - turned into screen points at
  // draw time, because the dome itself never moves and only the light
  // on it does.
  //
  // Six ways of panelling one. They are genuinely different shapes,
  // not one shape in six colours: quads staggered like brickwork,
  // proper hexagons, triangles alternating point-up and point-down,
  // long vertical staves, overlapping scales, and a bare cage with
  // hardly any skin at all. Which one a cast gets changes the whole
  // silhouette, which is the point.
  function domePanels(style, rings, base) {
    var out = [], i, j;
    var push = function (v0, v1, a0, a1, kind, seed) {
      out.push({ v0: v0, v1: v1, a0: a0, a1: a1, kind: kind,
                 mid: (v0 + v1) / 2, midA: (a0 + a1) / 2, seed: seed % 97,
                 // Its own clock, so the blooms never line up.
                 phase: nhash(seed * 3.7) * Math.PI * 2,
                 rate: 1.1 + nhash(seed * 5.3) * 3.4 });
    };
    if (style === 'staves') {
      // Long vertical panels running the whole way up, like the ribs
      // of a lantern. No horizontal seams at all.
      for (j = 0; j < base; j++) {
        push(0, 0.96, (j / base) * Math.PI * 2, ((j + 1) / base) * Math.PI * 2,
             'quad', j * 7);
      }
      return out;
    }
    for (i = 0; i < rings; i++) {
      var v0 = i / rings, v1 = (i + 1) / rings;
      // Fewer segments the higher up we are, so the panels stay about
      // the same size instead of pinching to slivers at the top.
      var seg = Math.max(4, Math.round(base * Math.cos(v0 * Math.PI / 2)));
      // Every other ring offset by half a segment, so the seams
      // stagger like brickwork rather than running up in columns.
      var off = (i % 2) * (Math.PI / seg);
      for (j = 0; j < seg; j++) {
        var a0 = off + (j / seg) * Math.PI * 2;
        var a1 = off + ((j + 1) / seg) * Math.PI * 2;
        if (style === 'tri') {
          // Both triangles of the cell, split along its diagonal. The
          // first go at this alternated one or the other per cell,
          // which does not tessellate: it leaves a diamond hole
          // between every pair and reads as scattered zigzags.
          push(v0, v1, a0, a1, 'triA', i * 31 + j * 7);
          push(v0, v1, a0, a1, 'triB', i * 31 + j * 7 + 3);
        } else if (style === 'scale') {
          push(v0, v1, a0, a1, 'scale', i * 31 + j * 7);
        } else if (style === 'hex') {
          push(v0, v1, a0, a1, 'hex', i * 31 + j * 7);
        } else if (style === 'cage') {
          // Only a scattering of the cells are skinned; the rest of
          // the dome is the ribs and nothing else.
          if (nhash(i * 13 + j * 5) > 0.72) {
            push(v0, v1, a0, a1, 'quad', i * 31 + j * 7);
          }
        } else {
          push(v0, v1, a0, a1, 'quad', i * 31 + j * 7);
        }
      }
    }
    return out;
  }

  DiceBoard.prototype.drawDome = function (ctx, it, grown, alpha, now) {
    var self = this, i, k, age = (now - it.born) / 1000;
    var R = it.r, H = it.h;
    // The dome comes up once and then holds absolutely still.
    // Everything that moves afterwards is light travelling over it,
    // which is the whole point: a shield that wobbles reads as a
    // bubble.
    var up = grown;

    // The whole dome turns on its own axis. Every longitude gets the
    // same offset, so the geometry rotates rather than the light
    // merely travelling over a fixed shell - the seams themselves go
    // round, which is the difference you can actually see.
    var turn = age * it.turn;

    // A point on the hemisphere, from a latitude (0 at the rim, 1 at
    // the top) and a longitude.
    var on = function (v, a) {
      var lat = v * Math.PI / 2;
      var rr = Math.cos(lat) * R;
      return self.projectUp([it.x + Math.cos(a + turn) * rr,
                             it.y + Math.sin(a + turn) * rr,
                             Math.sin(lat) * H * up]);
    };

    ctx.save();
    ctx.lineJoin = 'round';
    ctx.lineCap = 'round';

    // Far side first, so the near half is drawn over it and the thing
    // reads as solid even though you can see through all of it.
    var order = it.panels.slice().sort(function (p, q) {
      return Math.sin(p.midA + turn) - Math.sin(q.midA + turn);
    });

    // What is moving on it this frame depends on the dome: a meridian
    // sweeping round, rings climbing from the rim to the cap, a
    // scatter that flickers with no pattern at all, or all three.
    var scan = age * it.spin;
    var pulse = age * it.climb;

    for (i = 0; i < order.length; i++) {
      var pn = order[i];
      var near = 0, band = 0;
      if (it.motion !== 'pulse') {
        var da = Math.abs(((pn.midA + turn - scan) % (Math.PI * 2) +
                           Math.PI * 3) % (Math.PI * 2) - Math.PI) / Math.PI;
        near = Math.pow(1 - da, it.motion === 'scan' ? 12 : 8);
      }
      if (it.motion !== 'scan') {
        band = Math.pow(Math.max(0, 1 - Math.abs(
          ((pn.mid - pulse) % 1 + 1) % 1 - 0.5) * 3.4), 3);
      }
      var flick = 0.5 + 0.5 * fbm1(it.seed + pn.seed * 0.7 + age * it.churn, 2);
      if (it.motion === 'storm') {
        // No wave at all: every panel on its own clock, and hard.
        flick = Math.pow(flick, 3);
        near = 0; band = 0;
      }
      var lit = it.base + it.flick * flick + 0.75 * near + 0.5 * band + bloom;
      // Panels bloom on their own: each has a phase of its own and a
      // period of its own, so at any moment a handful of them are lit
      // right up and going out again with no pattern between them.
      var beat = 0.5 + 0.5 * Math.sin(age * pn.rate + pn.phase);
      var bloom = Math.pow(beat, 7) * it.bloom;
      // The far side is dimmer, so the curve of it reads.
      var facing = 0.45 + 0.55 * (Math.sin(pn.midA + turn) * 0.5 + 0.5);

      // The panel's own outline, which is what the style comes down to.
      var pts;
      if (pn.kind === 'hex') {
        // A hexagon inscribed in the cell: the two mid-latitude points
        // are pushed out to the cell's corners and the top and bottom
        // pulled in to its middle.
        var am = (pn.a0 + pn.a1) / 2;
        pts = [on(pn.v0, am), on(pn.v0 + (pn.v1 - pn.v0) * 0.28, pn.a1),
               on(pn.v0 + (pn.v1 - pn.v0) * 0.72, pn.a1), on(pn.v1, am),
               on(pn.v0 + (pn.v1 - pn.v0) * 0.72, pn.a0),
               on(pn.v0 + (pn.v1 - pn.v0) * 0.28, pn.a0)];
      } else if (pn.kind === 'triA') {
        pts = [on(pn.v0, pn.a0), on(pn.v0, pn.a1), on(pn.v1, pn.a1)];
      } else if (pn.kind === 'triB') {
        pts = [on(pn.v0, pn.a0), on(pn.v1, pn.a1), on(pn.v1, pn.a0)];
      } else if (pn.kind === 'scale') {
        // A scale: square across the bottom, round over the top, and
        // overlapping the ring above it.
        pts = [on(pn.v0, pn.a0), on(pn.v0, pn.a1)];
        for (k = 0; k <= 5; k++) {
          var sa = pn.a1 + (pn.a0 - pn.a1) * (k / 5);
          var lift = pn.v0 + (pn.v1 - pn.v0) *
                     (1.25 * Math.sin((k / 5) * Math.PI) * 0.8 + 0.4);
          pts.push(on(Math.min(1, lift), sa));
        }
      } else {
        pts = [on(pn.v0, pn.a0), on(pn.v0, pn.a1),
               on(pn.v1, pn.a1), on(pn.v1, pn.a0)];
      }

      ctx.beginPath();
      ctx.moveTo(pts[0][0], pts[0][1]);
      for (k = 1; k < pts.length; k++) { ctx.lineTo(pts[k][0], pts[k][1]); }
      ctx.closePath();
      // The glass of the panel: almost nothing, so the sheet shows
      // through, and a real lift where the light is on it.
      ctx.globalAlpha = alpha * facing * Math.min(0.6, 0.1 + lit * 0.42);
      ctx.fillStyle = it.face;
      ctx.fill();
      // The frame, which is what you actually see. Drawn twice: a
      // saturated seam that reads against cream parchment whatever the
      // light is doing, and a pale one over it only where the light
      // has reached. Painting the whole thing in the pale colour is
      // what made the last one invisible on the page.
      ctx.globalAlpha = alpha * facing * Math.min(1, 0.45 + lit * 0.5);
      ctx.strokeStyle = it.seam;
      ctx.lineWidth = it.weight * (1.3 + 2.4 * Math.max(near, band));
      ctx.stroke();
      if (lit > 0.45) {
        ctx.globalAlpha = alpha * facing * Math.min(1, (lit - 0.45) * 1.6);
        ctx.strokeStyle = it.edge;
        ctx.lineWidth = it.weight * (0.7 + 1.6 * Math.max(near, band));
        ctx.stroke();
      }
      // A node at each corner of the lit panels.
      if (lit > 0.72 && it.nodes) {
        ctx.globalAlpha = alpha * facing * (lit - 0.72) * 2;
        ctx.fillStyle = it.node;
        for (k = 0; k < pts.length; k++) {
          ctx.beginPath();
          ctx.arc(pts[k][0], pts[k][1], 1.8, 0, Math.PI * 2);
          ctx.fill();
        }
      }
    }

    // The meridians: great circles over the whole dome, drawn over the
    // panels, which is the skeleton the panels hang on.
    ctx.globalAlpha = alpha * (it.ribAlpha + 0.25);
    ctx.strokeStyle = it.seam;
    ctx.lineWidth = it.ribWeight;
    for (i = 0; i < it.ribs; i++) {
      var ra = (i / it.ribs) * Math.PI * 2 - turn;
      ctx.beginPath();
      for (k = 0; k <= 14; k++) {
        var q = on(k / 14, ra);
        if (k) { ctx.lineTo(q[0], q[1]); } else { ctx.moveTo(q[0], q[1]); }
      }
      ctx.stroke();
    }
    // Latitude hoops, on the domes that have them.
    for (i = 1; i < it.hoops; i++) {
      ctx.globalAlpha = alpha * (it.ribAlpha + 0.15);
      ctx.beginPath();
      for (k = 0; k <= 26; k++) {
        var hq = on(i / it.hoops * 0.95, (k / 26) * Math.PI * 2);
        if (k) { ctx.lineTo(hq[0], hq[1]); } else { ctx.moveTo(hq[0], hq[1]); }
      }
      ctx.closePath();
      ctx.stroke();
    }
    // A band of writing round the base, on the ones that are warded
    // rather than built.
    if (it.runes) {
      ctx.globalAlpha = alpha * 0.85;
      ctx.strokeStyle = it.seam;
      ctx.lineWidth = 1.6;
      for (i = 0; i < it.runes; i++) {
        var wa = -age * 0.3 - turn + (i / it.runes) * Math.PI * 2;
        // Floored: the dome's seed is a float, and a fractional index
        // into an array is a hole rather than a glyph.
        var gl = RUNE_STROKES[Math.floor(it.seed + i * 3) % RUNE_STROKES.length];
        var c0 = on(0.08, wa);
        var c1 = on(0.08, wa + 0.09);
        var c2 = on(0.2, wa);
        ctx.beginPath();
        for (k = 0; k < gl.length; k++) {
          var ax = (gl[k][0] + 1) / 2, ay = (gl[k][1] + 1) / 2;
          var bx = (gl[k][2] + 1) / 2, by = (gl[k][3] + 1) / 2;
          ctx.moveTo(c0[0] + (c1[0] - c0[0]) * ax + (c2[0] - c0[0]) * ay,
                     c0[1] + (c1[1] - c0[1]) * ax + (c2[1] - c0[1]) * ay);
          ctx.lineTo(c0[0] + (c1[0] - c0[0]) * bx + (c2[0] - c0[0]) * by,
                     c0[1] + (c1[1] - c0[1]) * bx + (c2[1] - c0[1]) * by);
        }
        ctx.stroke();
      }
    }
    // And the cap at the very top, which every one of these has.
    ctx.globalAlpha = alpha * (0.6 + 0.35 * Math.sin(age * 3.2));
    ctx.strokeStyle = it.seam;
    ctx.lineWidth = it.ribWeight + 0.6;
    ctx.beginPath();
    for (k = 0; k <= 20; k++) {
      var cq = on(0.93, (k / 20) * Math.PI * 2);
      if (k) { ctx.lineTo(cq[0], cq[1]); } else { ctx.moveTo(cq[0], cq[1]); }
    }
    ctx.closePath();
    ctx.stroke();

    // The rim where it meets the paper, and a wash of light inside it.
    this.drawGroundRing(ctx, it.x, it.y, R, it.rim, 11, alpha * 0.85);
    var fp = this.project([it.x, it.y, 0]);
    var fg = ctx.createRadialGradient(fp[0], fp[1], 0, fp[0], fp[1], R * fp[2]);
    fg.addColorStop(0, 'rgba(0,0,0,0)');
    fg.addColorStop(0.7, 'rgba(0,0,0,0)');
    fg.addColorStop(1, it.rim);
    ctx.globalAlpha = alpha * 0.4;
    ctx.fillStyle = fg;
    ctx.beginPath();
    ctx.ellipse(fp[0], fp[1], R * fp[2], R * fp[2] * 0.42, 0, 0, Math.PI * 2);
    ctx.fill();
    ctx.restore();
    ctx.globalAlpha = 1;
  };

  // --------------------------------------------------------- spike growth
  //
  // Thorns through the paper. Not the smooth cones the ice uses: a real
  // thorn is knobbly - it swells and pinches along its length, it is
  // bent, and it has barbs coming off it. So each one is a tapering
  // outline built from a run of radii with noise on top, drawn along a
  // curve, with smaller barbs hung off the same curve.
  DiceBoard.prototype.drawThorn = function (ctx, it, grown, alpha) {
    var self = this, i;
    var h = it.h * grown;

    // The spine of it: a curve leaning the way the thorn grew.
    var spineAt = function (u, len) {
      var lean = it.lean * u * u;
      return [it.x + Math.cos(it.aim) * lean, it.y + Math.sin(it.aim) * lean * 0.7,
              len * u];
    };

    // One knobbly spike along a spine, from base radius r0 to a point.
    var spike = function (ox, oy, aim, len, r0, seed, curve) {
      var n = 11, pts = [], k;
      for (k = 0; k <= n; k++) {
        var u = k / n;
        var lean = curve * u * u;
        var w = self.projectUp([ox + Math.cos(aim) * lean,
                                oy + Math.sin(aim) * lean * 0.7, len * u]);
        // Knobbly: a slow swell down the length with a faster pinch on
        // top of it, and it never quite reaches nothing until the point.
        var knob = (0.55 + 0.75 * fbm1(seed + u * 5.5, 3)) *
                   (1 + 0.32 * Math.sin(u * 13 + seed));
        pts.push({ p: w, r: r0 * Math.pow(1 - u, 0.72) * knob * w[2] });
      }
      var side = function (k2, sign) {
        var a = pts[Math.max(0, k2 - 1)].p, b = pts[Math.min(n, k2 + 1)].p;
        var dx = b[0] - a[0], dy = b[1] - a[1], l = Math.hypot(dx, dy) || 1;
        return [pts[k2].p[0] + (-dy / l) * pts[k2].r * sign,
                pts[k2].p[1] + (dx / l) * pts[k2].r * sign];
      };
      var q = side(0, 1);
      ctx.moveTo(q[0], q[1]);
      for (k = 1; k <= n; k++) { q = side(k, 1); ctx.lineTo(q[0], q[1]); }
      for (k = n; k >= 0; k--) { q = side(k, -1); ctx.lineTo(q[0], q[1]); }
      ctx.closePath();
    };

    var whole = function (fat) {
      ctx.beginPath();
      spike(it.x, it.y, it.aim, h, it.r + fat, it.seed, it.lean);
      // Barbs: smaller thorns off the main one, alternating sides and
      // leaning further out the higher up they are.
      for (i = 0; i < it.barbs; i++) {
        var bu = 0.18 + (i / Math.max(1, it.barbs)) * 0.55;
        var base = spineAt(bu, h);
        var out = (i % 2 ? 1 : -1) * (0.9 + nhash(it.seed * 5 + i) * 0.7);
        spike(base[0], base[1], it.aim + out,
              h * (0.2 + nhash(it.seed * 3 + i) * 0.22),
              (it.r + fat) * 0.42, it.seed * 7 + i,
              it.lean * 0.5 + out * 8);
      }
    };

    ctx.save();
    // A shadow under it first, then the body, then the lit face.
    ctx.globalAlpha = alpha * 0.32;
    ctx.fillStyle = it.dark;
    whole(it.r * 0.22);
    ctx.fill();
    ctx.globalAlpha = alpha * 0.95;
    var top = this.projectUp([it.x, it.y, h]);
    var foot = this.projectUp([it.x, it.y, 0]);
    var g = ctx.createLinearGradient(0, top[1], 0, foot[1]);
    g.addColorStop(0, it.tip);
    g.addColorStop(0.45, it.face);
    g.addColorStop(1, it.dark);
    ctx.fillStyle = g;
    whole(0);
    ctx.fill();
    // The lit side, the same shape squeezed towards one edge.
    ctx.globalAlpha = alpha * 0.4;
    ctx.save();
    whole(0);
    ctx.clip();
    var lg = ctx.createLinearGradient(foot[0] - it.r * 3, 0, foot[0] + it.r * 3, 0);
    lg.addColorStop(0, 'rgba(0,0,0,0)');
    lg.addColorStop(0.75, it.tip);
    lg.addColorStop(1, 'rgba(0,0,0,0)');
    ctx.fillStyle = lg;
    ctx.fillRect(foot[0] - it.r * 4, top[1] - 8, it.r * 8, foot[1] - top[1] + 16);
    ctx.restore();
    ctx.restore();
    ctx.globalAlpha = 1;
  };

  // A tuft of grass: a handful of blades out of one spot, each bent its
  // own way, tapering to nothing.
  DiceBoard.prototype.drawTuft = function (ctx, it, grown, alpha) {
    var i, k;
    ctx.save();
    ctx.lineCap = 'round';
    for (i = 0; i < it.blades; i++) {
      var aim = it.aim + (i - it.blades / 2) * 0.5 +
                (nhash(it.seed + i) - 0.5) * 0.6;
      var len = it.h * grown * (0.5 + nhash(it.seed * 3 + i) * 0.9);
      var bend = (nhash(it.seed * 7 + i) - 0.5) * len * 0.9;
      ctx.globalAlpha = alpha * (0.55 + nhash(it.seed * 11 + i) * 0.4);
      ctx.strokeStyle = i % 3 ? it.face : it.dark;
      ctx.beginPath();
      for (k = 0; k <= 6; k++) {
        var u = k / 6;
        ctx.lineWidth = Math.max(0.4, it.w * (1 - u) * 1.4);
        var p = this.projectUp([it.x + Math.cos(aim) * bend * u * u,
                                it.y + Math.sin(aim) * bend * u * u * 0.7,
                                len * u]);
        if (k) { ctx.lineTo(p[0], p[1]); } else { ctx.moveTo(p[0], p[1]); }
      }
      ctx.stroke();
    }
    ctx.restore();
    ctx.globalAlpha = 1;
  };

  // ------------------------------------------------ the skill flourishes
  //
  // Four things that are not spells: a glass over the dice, something
  // growing out of the page, a book that opens, and a bar of music. They
  // share the elemental layer with everything else and are drawn in the
  // same leaned frame, so a book stands on the paper the way a crystal
  // does.

  // A frame for anything flat that stands up facing the reader: a
  // shield, a book, a hand, a glass.
  //
  // The obvious way to build one is to project three points - the
  // origin, one unit across and one unit up - and make a basis out of
  // them. That is what the first of these did, and it is wrong: the
  // three points sit at different heights, projectUp scales by height,
  // and the result is a frame with a shear in it. A hand can carry that
  // and looks as though it is leaning; a rectangular shield cannot, and
  // came out as a parallelogram lying on the table.
  //
  // So the frame is built in screen space instead: across is across, up
  // is up, and the only thing taken from the projection is how big one
  // unit is at that distance. Local +y is up.
  DiceBoard.prototype.billboard = function (ctx, x, y, z, s) {
    var o = this.projectUp([x, y, z]);
    var k = s * o[2];
    ctx.transform(k, 0, 0, -k, o[0], o[1]);
    return o;
  };

  // ------------------------------------------------------ the looking glass
  //
  // A lens held over the dice, translucent and lit from inside.
  //
  // The first version was a ring with a stick on it. What a real glass
  // has, and this now has: a thick bevelled rim with a bright top edge
  // and a dark underside, a collar where the rim meets the handle, a
  // turned wooden handle with a swell and a ferrule, and - the part
  // that matters - glass, which is not a pale disc but a thin film with
  // a bright rim inside the frame, a specular streak across the upper
  // left and a second fainter one opposite, and a shadow it throws.
  DiceBoard.prototype.drawGlass = function (ctx, it, grown, alpha, now) {
    var i, age = (now - it.born) / 1000;
    // It swings in, then hangs there breathing.
    var bob = Math.sin(age * 2.3 + it.seed) * 0.05;
    var lean = it.lean + Math.sin(age * 1.7) * 0.06;
    var s = it.r * (0.6 + grown * 0.4);
    ctx.save();
    this.billboard(ctx, it.x, it.y, it.z + bob * s, s);
    ctx.rotate(lean);

    // ---- the handle, behind the rim so the collar covers its top.
    ctx.save();
    ctx.rotate(2.3);
    ctx.lineCap = 'butt';
    // A turned shaft: three lengths of different thickness rather than
    // one bar, which is what makes it look made rather than drawn.
    var shaft = [[0.98, 1.32, 0.15], [1.32, 1.42, 0.2],
                 [1.42, 1.92, 0.155], [1.92, 2.06, 0.21]];
    for (i = 0; i < shaft.length; i++) {
      var sg = ctx.createLinearGradient(0, -shaft[i][2], 0, shaft[i][2]);
      sg.addColorStop(0, it.woodLit);
      sg.addColorStop(0.42, it.wood);
      sg.addColorStop(1, it.woodDark);
      ctx.globalAlpha = alpha * 0.85;
      ctx.fillStyle = sg;
      ctx.beginPath();
      ctx.rect(shaft[i][0], -shaft[i][2], shaft[i][1] - shaft[i][0],
               shaft[i][2] * 2);
      ctx.fill();
    }
    // Rounded off at the end.
    ctx.beginPath();
    ctx.ellipse(2.06, 0, 0.09, 0.21, 0, 0, Math.PI * 2);
    ctx.fill();
    // The grain, and the two rings turned into it.
    ctx.globalAlpha = alpha * 0.3;
    ctx.strokeStyle = it.woodDark;
    ctx.lineWidth = 0.022;
    for (i = 0; i < 4; i++) {
      var gy = -0.1 + i * 0.07;
      ctx.beginPath();
      ctx.moveTo(1.0, gy);
      ctx.bezierCurveTo(1.4, gy + 0.03, 1.7, gy - 0.02, 2.02, gy + 0.01);
      ctx.stroke();
    }
    ctx.globalAlpha = alpha * 0.5;
    ctx.lineWidth = 0.03;
    [1.33, 1.41, 1.9].forEach(function (rx) {
      ctx.beginPath();
      ctx.moveTo(rx, -0.19); ctx.lineTo(rx, 0.19);
      ctx.stroke();
    });
    ctx.restore();

    // ---- the glass.
    //
    // A thin film, not a pane: almost clear in the middle, picking up
    // towards the edge where you are looking through more of it.
    ctx.globalAlpha = alpha;
    var g = ctx.createRadialGradient(-0.2, -0.24, 0.05, 0, 0, 1);
    g.addColorStop(0, 'rgba(255,255,255,.1)');
    g.addColorStop(0.55, 'rgba(214,236,250,.1)');
    g.addColorStop(0.86, 'rgba(178,212,236,.24)');
    g.addColorStop(1, 'rgba(232,246,255,.5)');
    ctx.fillStyle = g;
    ctx.beginPath();
    ctx.arc(0, 0, 1, 0, Math.PI * 2);
    ctx.fill();
    // What the lens does to what is under it: a faint lift in the
    // middle, which is the only honest way to draw magnification.
    var mg = ctx.createRadialGradient(0, 0, 0, 0, 0, 0.9);
    mg.addColorStop(0, 'rgba(255,252,240,.18)');
    mg.addColorStop(1, 'rgba(255,252,240,0)');
    ctx.fillStyle = mg;
    ctx.beginPath();
    ctx.arc(0, 0, 0.9, 0, Math.PI * 2);
    ctx.fill();

    // The two speculars. A hard narrow one across the upper left and a
    // soft wide one opposite it - one light source and its bounce, and
    // between them they are most of what says "glass".
    ctx.save();
    ctx.beginPath();
    ctx.arc(0, 0, 0.98, 0, Math.PI * 2);
    ctx.clip();
    var sweep = it.good ? ((age * 0.55) % 1.6) - 0.3 : 0;
    ctx.globalAlpha = alpha * 0.55;
    ctx.fillStyle = 'rgba(255,255,255,.9)';
    ctx.save();
    ctx.rotate(-0.7 + sweep * 0.5);
    ctx.beginPath();
    ctx.ellipse(-0.3 + sweep, 0, 0.14, 0.72, 0, 0, Math.PI * 2);
    ctx.fill();
    ctx.globalAlpha = alpha * 0.22;
    ctx.beginPath();
    ctx.ellipse(-0.04 + sweep, 0, 0.06, 0.52, 0, 0, Math.PI * 2);
    ctx.fill();
    ctx.restore();
    // The bounce: a wide soft crescent inside the far rim.
    ctx.globalAlpha = alpha * 0.2;
    ctx.strokeStyle = '#ffffff';
    ctx.lineWidth = 0.22;
    ctx.beginPath();
    ctx.arc(0, 0, 0.8, Math.PI * 0.12, Math.PI * 0.72);
    ctx.stroke();
    ctx.restore();

    // A bright hairline just inside the frame, where the glass is
    // seated - the detail that separates the glass from its rim.
    ctx.globalAlpha = alpha * 0.5;
    ctx.strokeStyle = '#ffffff';
    ctx.lineWidth = 0.022;
    ctx.beginPath();
    ctx.arc(0, 0, 0.955, 0, Math.PI * 2);
    ctx.stroke();

    // ---- the rim: a bevelled band, dark underneath and lit on top.
    var rg = ctx.createLinearGradient(-0.9, -0.9, 0.7, 1.0);
    rg.addColorStop(0, it.rimLit);
    rg.addColorStop(0.35, it.rim);
    rg.addColorStop(0.72, it.rimDark);
    rg.addColorStop(1, it.rim);
    ctx.globalAlpha = alpha * 0.92;
    ctx.strokeStyle = rg;
    ctx.lineWidth = 0.14;
    ctx.beginPath();
    ctx.arc(0, 0, 1.04, 0, Math.PI * 2);
    ctx.stroke();
    // The bright top edge of the bevel and the dark under edge.
    ctx.globalAlpha = alpha * 0.7;
    ctx.strokeStyle = it.rimLit;
    ctx.lineWidth = 0.035;
    ctx.beginPath();
    ctx.arc(0, 0, 1.10, Math.PI * 0.95, Math.PI * 1.95);
    ctx.stroke();
    ctx.globalAlpha = alpha * 0.55;
    ctx.strokeStyle = it.rimDark;
    ctx.beginPath();
    ctx.arc(0, 0, 0.985, Math.PI * 0.05, Math.PI * 0.95);
    ctx.stroke();
    // The collar where the rim meets the handle.
    ctx.save();
    ctx.rotate(2.3);
    ctx.globalAlpha = alpha * 0.9;
    ctx.fillStyle = it.rim;
    ctx.beginPath();
    ctx.ellipse(1.0, 0, 0.16, 0.24, 0, 0, Math.PI * 2);
    ctx.fill();
    ctx.globalAlpha = alpha * 0.6;
    ctx.strokeStyle = it.rimLit;
    ctx.lineWidth = 0.028;
    ctx.stroke();
    ctx.restore();

    // The ward on it, faint, so it is clearly a conjured glass.
    ctx.globalAlpha = alpha * (0.16 + 0.12 * Math.sin(age * 4.2));
    ctx.strokeStyle = it.glow;
    ctx.lineWidth = 0.2;
    ctx.beginPath();
    ctx.arc(0, 0, 1.08, 0, Math.PI * 2);
    ctx.stroke();

    if (it.good) {
      // Something was noticed: the lens takes light, and a few points
      // of it catch round the inside of the rim.
      ctx.globalAlpha = alpha * (0.14 + 0.1 * Math.sin(age * 5.5));
      var gg = ctx.createRadialGradient(0, 0, 0.2, 0, 0, 1);
      gg.addColorStop(0, 'rgba(255,248,210,.5)');
      gg.addColorStop(1, 'rgba(255,248,210,0)');
      ctx.fillStyle = gg;
      ctx.beginPath();
      ctx.arc(0, 0, 1, 0, Math.PI * 2);
      ctx.fill();
    }

    if (it.cracked) {
      // A break: a few lines out of one point, rings round it, and the
      // glass either side of each line catching the light differently.
      var op = Math.min(1, (age - it.crackAt) * 6);
      if (op > 0) {
        var cx = Math.cos(it.seed) * 0.3, cy = Math.sin(it.seed * 1.7) * 0.3;
        ctx.save();
        ctx.beginPath();
        ctx.arc(0, 0, 0.98, 0, Math.PI * 2);
        ctx.clip();
        ctx.lineJoin = 'round';
        for (i = 0; i < 8; i++) {
          var ca = (i / 8) * Math.PI * 2 + it.seed;
          var len = (0.8 + nhash(it.seed * 11 + i) * 0.7) * op;
          var path = [[cx, cy]];
          for (var k = 1; k <= 3; k++) {
            var kk = k / 3;
            var wob = (nhash(it.seed * 13 + i * 4 + k) - 0.5) * 0.35;
            path.push([cx + Math.cos(ca + wob) * len * kk,
                       cy + Math.sin(ca + wob) * len * kk]);
          }
          // A dark fracture with a bright edge beside it, which is what
          // a crack in glass actually looks like.
          [[0.02, it.rimDark, 0.06], [-0.012, '#ffffff', 0.032]].forEach(
            function (pass) {
              ctx.globalAlpha = alpha * (pass[1] === '#ffffff' ? 0.85 : 0.4) * op;
              ctx.strokeStyle = pass[1];
              ctx.lineWidth = pass[2];
              ctx.beginPath();
              ctx.moveTo(path[0][0] + pass[0], path[0][1] + pass[0]);
              for (var q = 1; q < path.length; q++) {
                ctx.lineTo(path[q][0] + pass[0], path[q][1] + pass[0]);
              }
              ctx.stroke();
            });
        }
        // Two rings round the impact, the way glass actually goes.
        ctx.globalAlpha = alpha * 0.55 * op;
        ctx.strokeStyle = '#ffffff';
        ctx.lineWidth = 0.026;
        for (i = 1; i <= 2; i++) {
          ctx.beginPath();
          ctx.arc(cx, cy, 0.24 * i * op, 0, Math.PI * 2);
          ctx.stroke();
        }
        // And the bloom of the break itself, gone in a moment.
        ctx.globalAlpha = alpha * 0.45 * Math.max(0, 1 - op);
        ctx.fillStyle = '#ffffff';
        ctx.beginPath();
        ctx.arc(cx, cy, 0.3, 0, Math.PI * 2);
        ctx.fill();
        ctx.restore();
      }
    }
    ctx.restore();
    ctx.globalAlpha = 1;
  };

  // ------------------------------------------------------------- growth
  //
  // A stem out of the paper with a flower on the end of it.
  //
  // Seven kinds of flower and five kinds of stem, rolled separately,
  // because a Nature check twice running should not be the same plant.
  // The flowers differ in what a petal *is* - a narrow ray, a broad
  // cupped thing, a hanging bell, a spike - and how many there are and
  // in how many rows, which between them is most of what tells one
  // flower from another at this size. The stems differ in whether they
  // are woody or soft, straight or twining, and what grows off them.

  // One petal, in its own frame: the tip is at (r, 0) and the middle
  // of the flower at the origin. `open` is nought when it is a bud.
  function petalPath(ctx, form, r, open, wob, seed) {
    var i;
    if (form === 'ray') {
      // A daisy: long, narrow, blunt-ended, barely tapering.
      ctx.moveTo(r * 0.16, -r * 0.1 * open);
      ctx.bezierCurveTo(r * 0.6, -r * 0.16 * open, r * 0.95, -r * 0.13 * open,
                        r * wob, 0);
      ctx.bezierCurveTo(r * 0.95, r * 0.13 * open, r * 0.6, r * 0.16 * open,
                        r * 0.16, r * 0.1 * open);
      ctx.closePath();
    } else if (form === 'broad') {
      // A poppy: wide, round, overlapping its neighbours, with a
      // ragged edge where the tissue has torn.
      ctx.moveTo(0, 0);
      for (i = 0; i <= 10; i++) {
        var u = i / 10;
        var flare = Math.sin(u * Math.PI) * 0.62 * open *
                    (1 + 0.14 * Math.sin(u * 9 + seed));
        ctx.lineTo(r * u * wob, r * flare);
      }
      for (i = 10; i >= 0; i--) {
        var u2 = i / 10;
        var f2 = Math.sin(u2 * Math.PI) * 0.62 * open *
                 (1 + 0.14 * Math.sin(u2 * 11 + seed * 2));
        ctx.lineTo(r * u2 * wob, -r * f2);
      }
      ctx.closePath();
    } else if (form === 'point') {
      // A star: a sharp lance of a petal.
      ctx.moveTo(0, 0);
      ctx.lineTo(r * 0.45 * wob, r * 0.3 * open);
      ctx.lineTo(r * wob, 0);
      ctx.lineTo(r * 0.45 * wob, -r * 0.3 * open);
      ctx.closePath();
    } else if (form === 'cup') {
      // A tulip: broad at the shoulder, curling back in at the tip.
      ctx.moveTo(0, 0);
      ctx.bezierCurveTo(r * 0.3, r * 0.5 * open, r * 0.78, r * 0.44 * open,
                        r * wob, -r * 0.1 * open);
      ctx.bezierCurveTo(r * 0.78, -r * 0.5 * open, r * 0.3, -r * 0.52 * open,
                        0, 0);
      ctx.closePath();
    } else if (form === 'bell') {
      // A foxglove: a hanging trumpet, wider at the mouth.
      ctx.moveTo(0, -r * 0.16);
      ctx.bezierCurveTo(r * 0.5, -r * 0.3 * open, r * 0.9, -r * 0.42 * open,
                        r * wob, -r * 0.3 * open);
      ctx.lineTo(r * wob, r * 0.3 * open);
      ctx.bezierCurveTo(r * 0.9, r * 0.42 * open, r * 0.5, r * 0.3 * open,
                        0, r * 0.16);
      ctx.closePath();
    } else if (form === 'spike') {
      // A thistle: a bristle rather than a petal.
      ctx.moveTo(0, -r * 0.045);
      ctx.lineTo(r * wob, -r * 0.012 * open);
      ctx.lineTo(r * wob, r * 0.012 * open);
      ctx.lineTo(0, r * 0.045);
      ctx.closePath();
    } else {
      // A rose: a short curled scoop, of which there are many rows.
      ctx.moveTo(0, 0);
      ctx.bezierCurveTo(r * 0.42, r * 0.44 * open, r * 0.88, r * 0.24 * open,
                        r * wob, -r * 0.06 * open);
      ctx.bezierCurveTo(r * 0.7, -r * 0.3 * open, r * 0.3, -r * 0.24 * open,
                        0, 0);
      ctx.closePath();
    }
  }

  // The seven. `rows` is how many rings of petals; `turn` how far each
  // row is rotated off the one below, which is what makes a rose a
  // rose; `heart` how big the middle is.
  var FLOWER_FORMS = {
    daisy:    { form: 'ray',    petals: 13, rows: 1, turn: 0,    heart: 0.3,
                stamens: 12, droop: 0 },
    poppy:    { form: 'broad',  petals: 4,  rows: 1, turn: 0,    heart: 0.26,
                stamens: 16, droop: 0 },
    star:     { form: 'point',  petals: 5,  rows: 1, turn: 0,    heart: 0.22,
                stamens: 5,  droop: 0 },
    tulip:    { form: 'cup',    petals: 6,  rows: 2, turn: 0.5,  heart: 0.14,
                stamens: 0,  droop: 0 },
    foxglove: { form: 'bell',   petals: 5,  rows: 1, turn: 0,    heart: 0.1,
                stamens: 4,  droop: 0.5 },
    thistle:  { form: 'spike',  petals: 26, rows: 2, turn: 0.4,  heart: 0.42,
                stamens: 0,  droop: 0 },
    rose:     { form: 'curl',   petals: 7,  rows: 3, turn: 0.42, heart: 0.12,
                stamens: 0,  droop: 0 }
  };

  DiceBoard.prototype.drawBloom = function (ctx, it, grown, alpha, now) {
    var self = this, i, age = (now - it.born) / 1000;
    var up = Math.min(1, grown * 1.15);
    var sway = Math.sin(age * 1.9 + it.seed) * 0.12;
    var fm = FLOWER_FORMS[it.flower] || FLOWER_FORMS.daisy;

    // Where the stem is at a given height. A twiner corkscrews on the
    // way up; a briar zig-zags; the rest just lean.
    var stemAt = function (u) {
      var bend = it.bend * u * u + sway * u * u * it.h * 0.16;
      var coil = 0;
      if (it.stem === 'twine') { coil = Math.sin(u * 9 + it.seed) * it.h * 0.1; }
      if (it.stem === 'briar') {
        coil = (nhash(it.seed + Math.floor(u * 5)) - 0.5) * it.h * 0.16;
      }
      return [it.x + bend + coil, it.y + bend * 0.35 + coil * 0.3,
              it.h * u * up];
    };

    var n = 18, pts = [];
    for (i = 0; i <= n; i++) { pts.push(this.projectUp(stemAt(i / n))); }

    ctx.save();
    ctx.lineCap = 'round';
    ctx.lineJoin = 'round';
    var stem = function (colour, w, a2) {
      ctx.globalAlpha = alpha * a2;
      ctx.strokeStyle = colour;
      ctx.beginPath();
      for (i = 0; i <= n; i++) {
        ctx.lineWidth = w * (1.1 - (i / n) * 0.55);
        if (i) { ctx.lineTo(pts[i][0], pts[i][1]); }
        else { ctx.moveTo(pts[i][0], pts[i][1]); }
      }
      ctx.stroke();
    };
    stem(it.dark, it.w * 1.5, 0.85);
    stem(it.stemCol, it.w, 1);
    stem('rgba(220,255,190,.7)', it.w * 0.28, 0.5);

    // Thorns, on the stems that have them.
    if (it.stem === 'briar') {
      ctx.globalAlpha = alpha * 0.9;
      ctx.fillStyle = it.dark;
      for (i = 3; i < n - 1; i += 3) {
        if (i / n > up) { continue; }
        var tx = pts[i + 1][0] - pts[i - 1][0], ty = pts[i + 1][1] - pts[i - 1][1];
        var tl = Math.hypot(tx, ty) || 1;
        var sd = i % 6 === 3 ? 1 : -1;
        ctx.beginPath();
        ctx.moveTo(pts[i][0], pts[i][1] - it.w * 0.5);
        ctx.lineTo(pts[i][0] + (-ty / tl) * sd * it.w * 2.4 - (tx / tl) * it.w * 1.6,
                   pts[i][1] + (tx / tl) * sd * it.w * 2.4 - (ty / tl) * it.w * 1.6);
        ctx.lineTo(pts[i][0], pts[i][1] + it.w * 0.5);
        ctx.closePath();
        ctx.fill();
      }
    }

    // Leaves - or fronds, on a fern, which are a row of little leaves
    // down a rib rather than one leaf.
    for (i = 0; i < it.leaves; i++) {
      var lu = 0.18 + (i / Math.max(1, it.leaves)) * 0.58;
      if (lu > up) { continue; }
      var lp = this.projectUp(stemAt(lu));
      var lq = this.projectUp(stemAt(Math.min(1, lu + 0.08)));
      var lang = Math.atan2(lq[1] - lp[1], lq[0] - lp[0]);
      var side = i % 2 ? 1 : -1;
      var pal = { leaf: it.leafCol, rope: it.stemCol, dark: it.dark };
      if (it.stem === 'fern') {
        var frond = it.w * (3.4 + nhash(it.seed + i) * 1.6);
        for (var f = 0; f < 6; f++) {
          var fu = f / 6;
          self.drawLeaf(ctx,
            lp[0] + Math.cos(lang + side * 0.9) * frond * fu,
            lp[1] + Math.sin(lang + side * 0.9) * frond * fu,
            lang + side * 1.5, it.w * (1.5 - fu * 0.8), pal,
            it.seed * 7 + i * 9 + f, alpha);
        }
      } else if (it.stem === 'grassy') {
        // Blades, not leaves.
        ctx.globalAlpha = alpha * 0.9;
        ctx.strokeStyle = it.leafCol;
        ctx.lineWidth = it.w * 0.6;
        ctx.beginPath();
        ctx.moveTo(lp[0], lp[1]);
        ctx.quadraticCurveTo(lp[0] + Math.cos(lang + side) * it.w * 5,
                             lp[1] + Math.sin(lang + side) * it.w * 5,
                             lp[0] + Math.cos(lang + side * 0.4) * it.w * 9,
                             lp[1] + Math.sin(lang + side * 0.4) * it.w * 9);
        ctx.stroke();
      } else {
        self.drawLeaf(ctx, lp[0], lp[1], lang + side * 1.0,
                      it.w * (2.6 + nhash(it.seed + i) * 1.8), pal,
                      it.seed * 7 + i, alpha);
      }
    }

    // Tendrils, on a twiner.
    if (it.stem === 'twine') {
      ctx.globalAlpha = alpha * 0.8;
      ctx.strokeStyle = it.stemCol;
      ctx.lineWidth = Math.max(0.6, it.w * 0.3);
      for (i = 0; i < 2; i++) {
        var tu = 0.3 + i * 0.28;
        if (tu > up) { continue; }
        var tp = this.projectUp(stemAt(tu));
        ctx.beginPath();
        for (var q = 0; q <= 26; q++) {
          var qu = q / 26;
          var rr = it.w * (1.4 + qu * 3.2);
          var th = qu * 3.4 * Math.PI * (i ? 1 : -1);
          if (q) {
            ctx.lineTo(tp[0] + Math.cos(th) * rr + qu * it.w * 5,
                       tp[1] + Math.sin(th) * rr * 0.6);
          } else {
            ctx.moveTo(tp[0] + rr, tp[1]);
          }
        }
        ctx.stroke();
      }
    }

    // ---- the flower, which opens once the stem is most of the way up.
    var openT = Math.max(0, Math.min(1, (up - 0.55) / 0.4));
    if (openT <= 0) { ctx.restore(); ctx.globalAlpha = 1; return; }
    var tip = pts[n];
    var scale = this.projectUp(stemAt(1))[2];
    var fr = it.fr * scale * (0.35 + openT * 0.65);
    ctx.save();
    ctx.translate(tip[0], tip[1] + fm.droop * fr * 0.5);
    ctx.rotate(it.spin + age * 0.25);

    // Rows from the outside in, so the inner ones sit on top.
    for (var row = fm.rows - 1; row >= 0; row--) {
      var rr2 = fr * (1 - row * 0.26);
      var cnt = Math.max(3, Math.round(fm.petals / (row * 0.6 + 1)));
      for (i = 0; i < cnt; i++) {
        var pa = (i / cnt) * Math.PI * 2 + row * fm.turn;
        var wob = 0.82 + 0.36 * nhash(it.seed * 3 + i + row * 11);
        ctx.save();
        ctx.rotate(pa);
        ctx.globalAlpha = alpha * (0.95 - row * 0.06);
        // The inner rows are darker, the way a real flower's throat is.
        ctx.fillStyle = row === 0 ? (i % 2 ? it.petal : it.petal2) : it.petal2;
        ctx.beginPath();
        petalPath(ctx, fm.form, rr2, 0.35 + openT * 0.65, wob,
                  it.seed + i + row * 5);
        ctx.fill();
        // A vein down the middle of the outer row.
        if (row === 0 && fm.form !== 'spike') {
          ctx.globalAlpha = alpha * 0.3;
          ctx.strokeStyle = it.petal2;
          ctx.lineWidth = Math.max(0.4, rr2 * 0.05);
          ctx.beginPath();
          ctx.moveTo(rr2 * 0.2, 0);
          ctx.lineTo(rr2 * 0.9 * wob, 0);
          ctx.stroke();
        }
        ctx.restore();
      }
    }
    // The middle: a disc, with stamens round it on the ones that show
    // them. A rose and a tulip have neither.
    if (fm.heart > 0) {
      ctx.globalAlpha = alpha;
      ctx.fillStyle = it.heart;
      ctx.beginPath();
      ctx.arc(0, 0, fr * fm.heart, 0, Math.PI * 2);
      ctx.fill();
    }
    ctx.fillStyle = '#fff6c0';
    for (i = 0; i < fm.stamens; i++) {
      var sa2 = (i / fm.stamens) * Math.PI * 2 + it.seed;
      ctx.globalAlpha = alpha * 0.8 * openT;
      ctx.beginPath();
      ctx.arc(Math.cos(sa2) * fr * fm.heart * 0.85,
              Math.sin(sa2) * fr * fm.heart * 0.85,
              Math.max(0.4, fr * 0.055), 0, Math.PI * 2);
      ctx.fill();
    }
    ctx.restore();
    ctx.restore();
    ctx.globalAlpha = 1;
  };

  // ------------------------------------------------------------ the tome
  //
  // A heavy old book, open, hanging in the air and see-through.
  //
  // The thing that kept this looking flat was drawing it as surfaces.
  // A book is a *solid*: two blocks of paper with a board under each,
  // and what tells you so is not the top of the page but the sides -
  // the fore-edge where four hundred leaves show as four hundred lines,
  // the square of the cover board standing proud below them, and the
  // thickness of both catching the light along the bottom edge. So
  // everything here is built in a box: u across the spread from the
  // left fore-edge to the right, v down the page, w up off the table.
  //
  // On top of that goes the age. Old paper is not one colour: it is
  // foxed, water-stained at the edges, darker where five centuries of
  // thumbs have turned the outer corner, and the ink on it is in a hand
  // nobody living writes. And a ribbon marker, lying over the page and
  // hanging off the fore-edge, because every book like this has one and
  // it is the detail that says the book is *used*.

  // Stains are fixed per cast so they do not crawl about, and they are
  // in page coordinates so they lie on the paper and bend with it.
  function tomeStains(seed) {
    var out = [], i;
    for (i = 0; i < 26; i++) {
      var h = nhash(seed * 3 + i * 7);
      out.push({
        // Most of them hug the outer edge, where a book takes its damp.
        u: 0.18 + Math.pow(nhash(seed + i), 0.6) * 0.78,
        v: (nhash(seed * 5 + i * 3) - 0.5) * 1.5,
        r: 0.05 + h * 0.19,
        // A few are the small round foxing spots; the rest are washes.
        fox: h > 0.72,
        seed: nhash(seed * 11 + i) * 40,
        a: 0.05 + nhash(seed * 13 + i) * 0.13
      });
    }
    return out;
  }

  DiceBoard.prototype.drawTome = function (ctx, it, grown, alpha, now) {
    var self = this, i, k, age = (now - it.born) / 1000;
    // Shut for a moment, then the covers swing down, then it holds and
    // breathes. It never opens flat: a heavy book held open sits at
    // about a hundred and forty degrees, and drawing it at a hundred
    // and eighty is what makes one look like a folded card.
    var open = Math.max(0, Math.min(1, (age - 0.28) / 0.72));
    open = open * open * (3 - 2 * open);
    var breathe = Math.sin(age * 1.5) * 0.01;
    var s = it.r * (0.62 + grown * 0.38);
    var bob = Math.sin(age * 1.8) * 0.018;

    // The box the book lives in. u is across the spread, v down the
    // page, w up - and the camera sees it from in front and above, so w
    // goes up the screen and v goes down and slightly in.
    var o = this.projectUp([it.x, it.y, it.z + bob * s]);
    var eu = this.projectUp([it.x + s, it.y, it.z + bob * s]);
    var ev = this.projectUp([it.x, it.y + s * 0.60, it.z + bob * s - s * 0.42]);
    var ew = this.projectUp([it.x, it.y, it.z + bob * s + s * 0.62]);
    var at3 = function (u, v, w) {
      return [o[0] + (eu[0] - o[0]) * u + (ev[0] - o[0]) * v + (ew[0] - o[0]) * w,
              o[1] + (eu[1] - o[1]) * u + (ev[1] - o[1]) * v + (ew[1] - o[1]) * w];
    };

    // How far each half has swung down off the vertical.
    var ang = (1 - open) * 1.34 + 0.17;
    var span = 1.0;
    var THICK = 0.46;           // the block of paper
    var BOARD = 0.085;          // the cover board under it

    // A point on the top page of one side. t runs 0 at the gutter to 1
    // at the fore-edge; d drops it into the stack.
    var page = function (dir, t, v, d) {
      var sag = -0.11 * span * Math.sin(t * Math.PI) * open + breathe;
      return at3(dir * t * Math.cos(ang) * span,
                 v,
                 t * Math.sin(ang) * span + sag - (d || 0));
    };

    ctx.save();
    ctx.lineJoin = 'round';
    ctx.lineCap = 'round';

    var poly = function (pts, fill, a2, line, lw) {
      ctx.beginPath();
      for (var j = 0; j < pts.length; j++) {
        if (j) { ctx.lineTo(pts[j][0], pts[j][1]); }
        else { ctx.moveTo(pts[j][0], pts[j][1]); }
      }
      ctx.closePath();
      if (fill) { ctx.globalAlpha = alpha * a2; ctx.fillStyle = fill; ctx.fill(); }
      if (line) {
        ctx.globalAlpha = alpha * (a2 * 0.9);
        ctx.strokeStyle = line;
        ctx.lineWidth = lw || 1;
        ctx.stroke();
      }
    };

    // The top surface of one side, as a strip of quads so it curves.
    var surface = function (dir, d, fill, a2) {
      var pts = [], j;
      for (j = 0; j <= 10; j++) { pts.push(page(dir, j / 10, -0.72, d)); }
      for (j = 10; j >= 0; j--) { pts.push(page(dir, j / 10, 0.72, d)); }
      poly(pts, fill, a2);
      return pts;
    };

    var side = function (dir) {
      // ---- the cover board: a slab, wider than the paper, with its
      // own square edge showing all the way round the outside.
      var cover = function (d, fill, a2) {
        var pts = [], j;
        for (j = 0; j <= 10; j++) { pts.push(page(dir, (j / 10) * 1.06, -0.8, d)); }
        for (j = 10; j >= 0; j--) { pts.push(page(dir, (j / 10) * 1.06, 0.8, d)); }
        poly(pts, fill, a2);
      };
      cover(THICK + BOARD, it.coverDark, 0.5);
      cover(THICK, it.cover, 0.62);
      // The square of the board along the near edge and the fore-edge,
      // which is the whole reason it reads as a board and not a sheet.
      var boardEdge = [];
      for (i = 0; i <= 10; i++) { boardEdge.push(page(dir, (i / 10) * 1.06, 0.8, THICK)); }
      for (i = 10; i >= 0; i--) { boardEdge.push(page(dir, (i / 10) * 1.06, 0.8, THICK + BOARD)); }
      poly(boardEdge, it.coverDark, 0.65);

      // ---- the block of leaves. The bottom face first, then the two
      // faces you can see, then the top page over them.
      surface(dir, THICK, it.pageDeep, 0.4);

      // The fore-edge: the end of the whole stack, and the face with
      // every leaf in the book showing on it.
      var fore = [];
      for (i = 0; i <= 6; i++) { fore.push(page(dir, 1, -0.72 + (i / 6) * 1.44, 0)); }
      for (i = 6; i >= 0; i--) { fore.push(page(dir, 1, -0.72 + (i / 6) * 1.44, THICK)); }
      poly(fore, it.pageEdge, 0.55);
      // And the near face, along the bottom of the page.
      var nearF = [];
      for (i = 0; i <= 10; i++) { nearF.push(page(dir, i / 10, 0.72, 0)); }
      for (i = 10; i >= 0; i--) { nearF.push(page(dir, i / 10, 0.72, THICK)); }
      poly(nearF, it.pageEdge, 0.6);

      // The leaves themselves: a line for every few of them across both
      // visible faces. This is the single thing that says "a thousand
      // pages" rather than "a slab".
      ctx.strokeStyle = it.pageLine;
      ctx.lineWidth = 0.7;
      for (i = 1; i < 56; i++) {
        // Unevenly spaced and unevenly dark, because a block of hand
        // cut paper is not a comb: some leaves are thicker, some have
        // come loose, and a few gape.
        var jig = (nhash(it.seed * 7 + i) - 0.5) * 0.5;
        var d = ((i + jig) / 56) * THICK;
        ctx.globalAlpha = alpha * (0.18 + 0.4 * nhash(it.seed * 3 + i * 5));
        // along the bottom face
        ctx.beginPath();
        var q0 = page(dir, 0.04, 0.72, d), q1 = page(dir, 1, 0.72, d);
        ctx.moveTo(q0[0], q0[1]);
        ctx.lineTo(q1[0], q1[1]);
        ctx.stroke();
        // and round the fore-edge
        ctx.beginPath();
        var r0 = page(dir, 1, -0.72, d), r1 = page(dir, 1, 0.72, d);
        ctx.moveTo(r0[0], r0[1]);
        ctx.lineTo(r1[0], r1[1]);
        ctx.stroke();
      }

      // ---- the page you are reading.
      var top = surface(dir, 0, it.page, 0.5);
      // Its sheen, brightest near the gutter where the paper curves up
      // into the light.
      ctx.save();
      ctx.beginPath();
      for (i = 0; i < top.length; i++) {
        if (i) { ctx.lineTo(top[i][0], top[i][1]); }
        else { ctx.moveTo(top[i][0], top[i][1]); }
      }
      ctx.closePath();
      ctx.clip();

      // Age. Washes and foxing first, then the thumbed corner, then the
      // damp along the outer edge.
      for (i = 0; i < it.stains.length; i++) {
        var st = it.stains[i];
        if ((st.v < 0) !== (dir < 0) && i % 2) { continue; }
        var c = page(dir, Math.min(1, st.u), st.v, 0);
        var e1 = page(dir, Math.min(1, st.u + st.r), st.v, 0);
        var e2 = page(dir, Math.min(1, st.u), st.v + st.r, 0);
        var rx = Math.hypot(e1[0] - c[0], e1[1] - c[1]);
        var ry = Math.hypot(e2[0] - c[0], e2[1] - c[1]);
        ctx.globalAlpha = alpha * st.a;
        if (st.fox) {
          ctx.fillStyle = it.fox;
          ctx.beginPath();
          ctx.ellipse(c[0], c[1], rx * 0.3, ry * 0.3, 0, 0, Math.PI * 2);
          ctx.fill();
        } else {
          // A wash with a ragged edge and a darker tideline, which is
          // what a water stain in paper actually is.
          ctx.fillStyle = it.stain;
          ctx.beginPath();
          for (k = 0; k <= 14; k++) {
            var sa = (k / 14) * Math.PI * 2;
            var wob = 0.65 + 0.5 * fbm1(st.seed + k * 0.8, 2);
            var sx = c[0] + Math.cos(sa) * rx * wob;
            var sy = c[1] + Math.sin(sa) * ry * wob;
            if (k) { ctx.lineTo(sx, sy); } else { ctx.moveTo(sx, sy); }
          }
          ctx.closePath();
          ctx.fill();
          ctx.globalAlpha = alpha * st.a * 0.9;
          ctx.strokeStyle = it.stain;
          ctx.lineWidth = 1.1;
          ctx.stroke();
        }
      }
      // The outer corner, gone brown from five hundred years of being
      // turned by the same two fingers.
      var cn = page(dir, 1, 0.72, 0);
      var tg = ctx.createRadialGradient(cn[0], cn[1], 0, cn[0], cn[1], s * 0.6);
      tg.addColorStop(0, it.thumb);
      tg.addColorStop(1, 'rgba(0,0,0,0)');
      ctx.globalAlpha = alpha * 0.85;
      ctx.fillStyle = tg;
      ctx.fillRect(cn[0] - s, cn[1] - s, s * 2, s * 2);
      // And the same at the top outer corner, less of it.
      var cn2 = page(dir, 1, -0.72, 0);
      var tg2 = ctx.createRadialGradient(cn2[0], cn2[1], 0, cn2[0], cn2[1], s * 0.42);
      tg2.addColorStop(0, it.thumb);
      tg2.addColorStop(1, 'rgba(0,0,0,0)');
      ctx.globalAlpha = alpha * 0.5;
      ctx.fillStyle = tg2;
      ctx.fillRect(cn2[0] - s, cn2[1] - s, s * 2, s * 2);

      // ---- what is written on it.
      if (open > 0.4) {
        var ink = alpha * Math.min(1, (open - 0.4) / 0.4);
        ctx.strokeStyle = it.rune;
        // Lines of runes, set as text is set: a ragged right edge, and
        // a gap where a capital or a diagram interrupts them.
        ctx.globalAlpha = ink * 0.6;
        ctx.lineWidth = 1.1;
        var skip = dir < 0 ? [2, 3, 4, 5] : [];
        for (i = 0; i < 13; i++) {
          if (skip.indexOf(i) >= 0) { continue; }
          var lv = -0.60 + i * 0.099;
          // A line of writing, not a row of a grid. Glyphs are packed
          // tight into words of two to five, words are separated by a
          // space wider than the gap inside one, every glyph is its own
          // width, a paragraph's first line is indented, and the line
          // stops wherever the last word happened to end - which is
          // what gives prose its ragged right edge. Setting them at a
          // fixed pitch is what made the page read as a spreadsheet.
          var para = nhash(it.seed * 17 + i) < 0.22;
          var lu = 0.13 + (para ? 0.06 : 0);
          var right = 0.9 - nhash(it.seed * 19 + i) * 0.14;
          var word = 0, letters = 0;
          for (k = 0; k < 40; k++) {
            if (lu > right) { break; }
            if (letters >= 2 + Math.floor(nhash(it.seed * 7 + i * 3 + word) * 4)) {
              // A word space, and on to the next word.
              lu += 0.028 + nhash(it.seed * 23 + i + word) * 0.016;
              word++;
              letters = 0;
              continue;
            }
            letters++;
            var gl = RUNE_STROKES[(it.seed + i * 5 + k * 3) % RUNE_STROKES.length];
            // Every glyph its own width, so a line of them is not a
            // row of identical cells.
            var gw = 0.036 + nhash(it.seed * 29 + i * 7 + k) * 0.03;
            var bp = page(dir, lu, lv, 0);
            var op2 = page(dir, lu + gw, lv, 0);
            var up2 = page(dir, lu, lv + 0.042, 0);
            ctx.beginPath();
            for (var g2 = 0; g2 < gl.length; g2++) {
              var ax = (gl[g2][0] + 1) / 2, ay = (gl[g2][1] + 1) / 2;
              var bx = (gl[g2][2] + 1) / 2, by = (gl[g2][3] + 1) / 2;
              ctx.moveTo(bp[0] + (op2[0] - bp[0]) * ax + (up2[0] - bp[0]) * ay,
                         bp[1] + (op2[1] - bp[1]) * ax + (up2[1] - bp[1]) * ay);
              ctx.lineTo(bp[0] + (op2[0] - bp[0]) * bx + (up2[0] - bp[0]) * by,
                         bp[1] + (op2[1] - bp[1]) * bx + (up2[1] - bp[1]) * by);
            }
            ctx.stroke();
            // Letters in a word almost touch; the space between words
            // is added above when the word ends.
            lu += gw + 0.008;
          }
        }

        // The circle, in the gap the writing left for it on the left
        // page: the same figure that burns on the ground for a ritual,
        // inked flat on the paper and turning very slowly.
        if (dir < 0) {
          var cu = 0.52, cv = -0.12, cr = 0.3;
          var cp = page(dir, cu, cv, 0);
          var cxp = page(dir, cu + cr, cv, 0);
          var cyp = page(dir, cu, cv + cr * 1.15, 0);
          ctx.save();
          ctx.transform(cxp[0] - cp[0], cxp[1] - cp[1],
                        cyp[0] - cp[0], cyp[1] - cp[1], cp[0], cp[1]);
          var turn = age * 0.35;
          ctx.globalAlpha = ink * 0.75;
          ctx.strokeStyle = it.rune;
          ctx.lineWidth = 0.045;
          [1, 0.88, 0.62, 0.34].forEach(function (rr) {
            ctx.beginPath();
            ctx.arc(0, 0, rr, 0, Math.PI * 2);
            ctx.stroke();
          });
          ctx.lineWidth = 0.05;
          ctx.beginPath();
          for (i = 0; i <= 7; i++) {
            var pa2 = turn + ((i * 3) % 7) / 7 * Math.PI * 2 - Math.PI / 2;
            if (i) { ctx.lineTo(Math.cos(pa2) * 0.6, Math.sin(pa2) * 0.6); }
            else { ctx.moveTo(Math.cos(pa2) * 0.6, Math.sin(pa2) * 0.6); }
          }
          ctx.stroke();
          ctx.lineWidth = 0.03;
          for (i = 0; i < 12; i++) {
            var ra2 = -turn + (i / 12) * Math.PI * 2;
            var gl2 = RUNE_STROKES[(it.seed + i * 3) % RUNE_STROKES.length];
            ctx.save();
            ctx.translate(Math.cos(ra2) * 0.94, Math.sin(ra2) * 0.94);
            ctx.rotate(ra2 + Math.PI / 2);
            ctx.scale(0.045, 0.045);
            ctx.beginPath();
            for (k = 0; k < gl2.length; k++) {
              ctx.moveTo(gl2[k][0], gl2[k][1]);
              ctx.lineTo(gl2[k][2], gl2[k][3]);
            }
            ctx.stroke();
            ctx.restore();
          }
          ctx.restore();
        }
      }
      ctx.restore();

      // The gutter shadow, where the page turns down into the spine.
      var g0 = page(dir, 0, -0.72, 0), g1 = page(dir, 0.3, 0.72, 0);
      var gg = ctx.createLinearGradient(g0[0], g0[1], g1[0], g1[1]);
      gg.addColorStop(0, it.gutter);
      gg.addColorStop(1, 'rgba(0,0,0,0)');
      ctx.globalAlpha = alpha * 0.5;
      ctx.fillStyle = gg;
      var gp = [];
      for (i = 0; i <= 6; i++) { gp.push(page(dir, (i / 6) * 0.34, -0.72, 0)); }
      for (i = 6; i >= 0; i--) { gp.push(page(dir, (i / 6) * 0.34, 0.72, 0)); }
      ctx.beginPath();
      for (i = 0; i < gp.length; i++) {
        if (i) { ctx.lineTo(gp[i][0], gp[i][1]); } else { ctx.moveTo(gp[i][0], gp[i][1]); }
      }
      ctx.closePath();
      ctx.fill();
    };

    // The far half, the spine between them, then the near half.
    side(-1);

    // ---- the spine: the round of the back, seen end-on in the valley.
    var sp = [];
    for (i = 0; i <= 8; i++) {
      var sv = -0.8 + (i / 8) * 1.6;
      sp.push(at3(0, sv, -THICK - BOARD));
    }
    for (i = 8; i >= 0; i--) {
      var sv2 = -0.8 + (i / 8) * 1.6;
      sp.push(at3(0, sv2, 0.06));
    }
    poly(sp, it.coverDark, 0.6);

    side(1);

    // ---- the ribbon marker: out of the spine, over the near page, and
    // hanging off the fore-edge with a frayed, stained end.
    if (open > 0.3) {
      var rw = 0.075;
      var hang = 0.55 * Math.min(1, (open - 0.3) / 0.5);
      var band = function (wobble, fill, a2) {
        var pts = [], j;
        // Over the page, then off the end and down.
        for (j = 0; j <= 8; j++) {
          var t = j / 8;
          pts.push(page(1, t * 0.98, 0.3 + wobble, 0));
        }
        for (j = 1; j <= 5; j++) {
          var d2 = j / 5;
          var e = page(1, 0.98, 0.3 + wobble, 0);
          pts.push([e[0] + (at3(0, 0, -hang * d2)[0] - at3(0, 0, 0)[0]),
                    e[1] + (at3(0, 0, -hang * d2)[1] - at3(0, 0, 0)[1])]);
        }
        for (j = 5; j >= 1; j--) {
          var d3 = j / 5;
          var e2 = page(1, 0.98, 0.3 + wobble + rw, 0);
          pts.push([e2[0] + (at3(0, 0, -hang * d3)[0] - at3(0, 0, 0)[0]),
                    e2[1] + (at3(0, 0, -hang * d3)[1] - at3(0, 0, 0)[1])]);
        }
        for (j = 8; j >= 0; j--) {
          var t2 = j / 8;
          pts.push(page(1, t2 * 0.98, 0.3 + wobble + rw, 0));
        }
        poly(pts, fill, a2);
      };
      band(0.01, it.ribbonDark, 0.55);
      band(0, it.ribbon, 0.7);
      // The stained, frayed end of it.
      var tip = page(1, 0.98, 0.3, 0);
      var down = at3(0, 0, -hang);
      var base = at3(0, 0, 0);
      ctx.globalAlpha = alpha * 0.35;
      ctx.fillStyle = it.stain;
      ctx.beginPath();
      ctx.ellipse(tip[0] + (down[0] - base[0]), tip[1] + (down[1] - base[1]),
                  s * 0.05, s * 0.035, 0, 0, Math.PI * 2);
      ctx.fill();
    }

    // The light coming off the open book.
    if (open > 0.4) {
      var ga = alpha * Math.min(1, (open - 0.4) / 0.4);
      var at0 = at3(0, 0, 0.25);
      var pg = ctx.createRadialGradient(at0[0], at0[1], 0, at0[0], at0[1], s * 1.7);
      pg.addColorStop(0, 'rgba(255,238,190,' + (ga * 0.26).toFixed(3) + ')');
      pg.addColorStop(1, 'rgba(255,238,190,0)');
      ctx.globalAlpha = 1;
      ctx.fillStyle = pg;
      ctx.beginPath();
      ctx.arc(at0[0], at0[1], s * 1.7, 0, Math.PI * 2);
      ctx.fill();
    }
    ctx.restore();
    ctx.globalAlpha = 1;
  };

  // One musical note, rising and growing.
  //
  // Three shapes: a crotchet, a quaver with a flag, and a beamed pair.
  // All of them are an oval head leaning over, a stem, and whatever goes
  // on the top of the stem - which is nearly all of music notation and
  // entirely enough of it at this size.
  DiceBoard.prototype.drawNote = function (ctx, it, grown, alpha, now) {
    var age = (now - it.born) / 1000;
    var p = this.projectUp([it.x, it.y, it.z]);
    var r = it.r * p[2] * (0.5 + grown * 0.9);
    ctx.save();
    ctx.translate(p[0], p[1]);
    ctx.rotate(it.tilt + Math.sin(age * 2.2 + it.seed) * 0.12);
    ctx.globalAlpha = alpha * 0.9;

    var headAt = function (hx) {
      ctx.save();
      ctx.translate(hx, 0);
      ctx.rotate(-0.35);
      ctx.beginPath();
      ctx.ellipse(0, 0, r * 0.52, r * 0.38, 0, 0, Math.PI * 2);
      ctx.fill();
      ctx.restore();
    };

    // The halo, then the note. Painted rather than added, because there
    // is no headroom above cream parchment.
    var draw = function (colour, fat, a2) {
      ctx.globalAlpha = alpha * a2;
      ctx.fillStyle = colour;
      ctx.strokeStyle = colour;
      ctx.lineCap = 'round';
      headAt(0);
      if (it.kind === 'beam') { headAt(r * 1.5); }
      ctx.lineWidth = r * 0.16 + fat;
      ctx.beginPath();
      ctx.moveTo(r * 0.44, -r * 0.12);
      ctx.lineTo(r * 0.44, -r * 1.9);
      if (it.kind === 'beam') {
        ctx.moveTo(r * 1.94, -r * 0.12);
        ctx.lineTo(r * 1.94, -r * 1.9);
      }
      ctx.stroke();
      if (it.kind === 'beam') {
        ctx.lineWidth = r * 0.3 + fat;
        ctx.beginPath();
        ctx.moveTo(r * 0.44, -r * 1.82);
        ctx.lineTo(r * 1.94, -r * 1.82);
        ctx.stroke();
      } else if (it.kind === 'flag') {
        ctx.lineWidth = r * 0.14 + fat;
        ctx.beginPath();
        ctx.moveTo(r * 0.44, -r * 1.9);
        ctx.quadraticCurveTo(r * 1.3, -r * 1.5, r * 0.9, -r * 0.7);
        ctx.quadraticCurveTo(r * 1.05, -r * 1.3, r * 0.44, -r * 1.5);
        ctx.stroke();
      }
    };
    draw(it.glow, r * 0.3, 0.55 * (it.wisp === undefined ? 1 : it.wisp));
    draw(it.face, 0, it.wisp === undefined ? 0.95 : it.wisp);
    ctx.restore();
    ctx.globalAlpha = 1;
  };

  // ------------------------------------------------------- the ghost hand
  //
  // A hand that is not there, standing up off the page and doing
  // something with itself before it goes.
  //
  // It is built rather than drawn: a palm and five fingers, each finger
  // three bones with a joint angle, so a gesture is a table of curls
  // instead of a picture. That is what makes it worth doing at all - one
  // set of geometry gives a point, a wave, a beckon and a fist, and the
  // cast picks one along with a colour it has never had before.
  //
  // The first version drew each bone as a capsule, which is a sausage:
  // the same width end to end with a round bulge at every joint, five of
  // them side by side. What a finger actually is, is a tapering column
  // that is *widest at the knuckles* and narrow between them, ending in
  // a blunt tip - so the outline here is built from a run of points with
  // a radius each, and the knuckles are where the radius goes back up.
  // Everything after the fill - the creases across the joints, the
  // metacarpal heads, the tendons running down the back of the hand -
  // is the detail that says which side of the hand you are looking at.
  //
  // The fingers are listed thumb first, then index out to little. Each
  // entry is where it leaves the palm, which way it sets off, how long
  // its three bones are and how thick it is at the base.
  var HAND_FINGERS = [
    { at: [-0.44, 0.28], aim: -0.92, bones: [0.31, 0.25, 0.18], w: 0.27 },
    { at: [-0.29, 1.00], aim: -0.24, bones: [0.33, 0.23, 0.16], w: 0.235 },
    { at: [-0.10, 1.07], aim: -0.06, bones: [0.36, 0.26, 0.17], w: 0.245 },
    { at: [0.11, 1.04], aim: 0.13, bones: [0.34, 0.24, 0.16], w: 0.232 },
    { at: [0.31, 0.94], aim: 0.34, bones: [0.25, 0.18, 0.13], w: 0.198 }
  ];

  // How thick a finger is along its length, as a fraction of its base
  // width. Seven readings per finger: base, mid-proximal, the big
  // knuckle, mid-middle, the second knuckle, mid-distal, tip. The two
  // rises are the joints, and they are the whole difference between a
  // finger and a tube.
  var HAND_TAPER = [0.52, 0.47, 0.525, 0.435, 0.475, 0.40, 0.365];

  // A gesture is how curled each finger is, nought to one, and what the
  // whole hand does while it holds it. The movers are read in drawHand.
  var HAND_GESTURES = [
    // Pointing at something over there.
    { curl: [0.55, 0.0, 0.86, 0.9, 0.92], spread: 0.6, tilt: -0.25,
      move: 'drift' },
    // An open palm, turning slowly: hello, or wait.
    { curl: [0.15, 0.1, 0.05, 0.1, 0.2], spread: 1.3, tilt: 0.05,
      move: 'wave' },
    // Beckoning - the index curling and uncurling on its own.
    { curl: [0.5, 0.25, 0.8, 0.86, 0.9], spread: 0.7, tilt: -0.1,
      move: 'beckon' },
    // A fist, loose enough that the knuckles still show.
    { curl: [0.72, 0.82, 0.84, 0.82, 0.8], spread: 0.45, tilt: 0.1,
      move: 'clench' },
    // Fingers splayed, holding something invisible and turning it.
    { curl: [0.45, 0.4, 0.38, 0.42, 0.5], spread: 1.5, tilt: -0.15,
      move: 'turn' },
    // A pinch, thumb and forefinger together, lifting something small.
    { curl: [0.72, 0.72, 0.35, 0.45, 0.6], spread: 0.8, tilt: -0.3,
      move: 'lift' }
  ];

  // The hues it may turn up in. Anything but the parchment's own colour,
  // because a ghost the colour of the page is not a ghost.
  var HAND_HUES = [188, 205, 262, 292, 320, 96, 44, 12];

  DiceBoard.prototype.drawHand = function (ctx, it, grown, alpha, now) {
    var age = (now - it.born) / 1000;
    var g = HAND_GESTURES[it.gesture];

    // What the hand is doing with itself this frame. A mesh can turn
    // in three dimensions, so a wave is a real twist of the wrist
    // rather than a shear, and "turn" actually turns the thing over.
    var swing = 0, spin = 0, rise = 0, tipTo = 0;
    if (g.move === 'wave') { swing = Math.sin(age * 4.4) * 0.5; }
    if (g.move === 'drift') { swing = Math.sin(age * 1.6) * 0.12; }
    if (g.move === 'turn') { spin = age * 1.1; }
    if (g.move === 'lift') { rise = Math.sin(age * 2.6) * 0.16; }
    if (g.move === 'beckon') { tipTo = Math.sin(age * 5.6) * 0.18; }
    if (g.move === 'clench') { tipTo = Math.min(0.3, age * 0.4); }

    var s = it.size * (0.55 + grown * 0.45);
    // It rises out of the paper: the wrist stays in it while the
    // fingers come clear.
    var z = it.z + (0.15 + rise) * s + grown * s * 0.5;
    // Pitched a quarter turn for the same reason the cat is: the
    // camera looks down at the page, and a hand built with its own z
    // as "up" would be seen from directly above, palm-down, which is
    // a hand doing nothing.
    this.drawMesh(ctx, it.mesh,
                  [it.x, it.y, z], [s, s, s],
                  [swing + spin, Math.PI / 2 + g.tilt + tipTo - 0.18, 0.1],
                  it.look, alpha * (0.35 + grown * 0.65));
  };

  // The comic-book bang: a jagged starburst punched out of the middle of
  // everything, a cauliflower of smoke round the rim of it, and lines
  // shooting off in all directions.
  //
  // Drawn flat and hard-edged on purpose. A realistic explosion on a
  // parchment character sheet would be mud; the vector-art version - big
  // shapes, bright outlines, no gradients doing the work - is both
  // legible at this size and the right register for dice on a table.
  //
  // The star's spikes are fixed when it spawns so it expands rather than
  // writhing, which is the difference between an explosion and a fire.
  DiceBoard.prototype.drawBoom = function (ctx, it, t, alpha) {
    var i, n = it.spikes.length;
    var p = this.projectUp([it.x, it.y, it.z]);
    // Out fast, then easing to a stop, the way a blast front does.
    var grow = 1 - Math.pow(1 - Math.min(1, t / 0.45), 3);
    var R = it.r * grow * p[2];
    ctx.save();
    ctx.translate(p[0], p[1]);
    ctx.scale(1, 0.82);

    // The body of it: alternate long and short points round the ring, each
    // with its own length, so the outline is ragged rather than a cog.
    var star = function (scale) {
      ctx.beginPath();
      for (i = 0; i < n; i++) {
        var a = it.spin + (i / n) * Math.PI * 2;
        var rr = R * it.spikes[i] * scale;
        var x = Math.cos(a) * rr, y = Math.sin(a) * rr;
        if (i) { ctx.lineTo(x, y); } else { ctx.moveTo(x, y); }
      }
      ctx.closePath();
    };

    // Three shells: the outer flame, the hotter middle, the white heart.
    ctx.globalAlpha = alpha * 0.9;
    ctx.fillStyle = it.outer;
    star(1);
    ctx.fill();
    ctx.globalAlpha = alpha;
    ctx.strokeStyle = it.edge;
    ctx.lineWidth = 2.4;
    ctx.stroke();

    ctx.globalAlpha = alpha * 0.95;
    ctx.fillStyle = it.mid;
    star(0.66);
    ctx.fill();

    ctx.globalAlpha = alpha * Math.max(0, 1 - t * 1.6);
    ctx.fillStyle = it.core;
    star(0.34);
    ctx.fill();
    ctx.restore();

    // The flash, which is over almost before it starts and is most of
    // what sells the first frame.
    if (t < 0.3) {
      var fa = (1 - t / 0.3);
      var fg = ctx.createRadialGradient(p[0], p[1], 0, p[0], p[1], R * 1.9);
      fg.addColorStop(0, it.core);
      fg.addColorStop(0.4, it.mid);
      fg.addColorStop(1, 'rgba(0,0,0,0)');
      ctx.save();
      ctx.globalCompositeOperation = 'lighter';
      ctx.globalAlpha = alpha * fa * 0.9;
      ctx.fillStyle = fg;
      ctx.beginPath();
      ctx.arc(p[0], p[1], R * 1.9, 0, Math.PI * 2);
      ctx.fill();
      ctx.restore();
    }

    // And the lines shooting off it, tapered and not evenly spaced.
    ctx.save();
    ctx.globalCompositeOperation = 'lighter';
    ctx.strokeStyle = it.edge;
    ctx.lineCap = 'round';
    for (i = 0; i < it.rays.length; i++) {
      var ra = it.rays[i].a;
      var from = R * (1 + 0.12 * it.rays[i].o);
      var to = from + R * it.rays[i].l * grow;
      ctx.globalAlpha = alpha * 0.55 * (1 - t);
      ctx.lineWidth = Math.max(0.8, it.rays[i].w * (1 - t));
      ctx.beginPath();
      ctx.moveTo(p[0] + Math.cos(ra) * from, p[1] + Math.sin(ra) * from * 0.82);
      ctx.lineTo(p[0] + Math.cos(ra) * to, p[1] + Math.sin(ra) * to * 0.82);
      ctx.stroke();
    }
    ctx.restore();
  };

  // One lobe of the smoke cauliflower: a ring of overlapping circles,
  // which is how the drawn ones do a cloud and is far more readable at
  // this size than anything soft.
  DiceBoard.prototype.drawLobe = function (ctx, it, grown, alpha) {
    var p = this.projectUp([it.x, it.y, it.z]);
    var R = it.r * grown * p[2], i;
    ctx.save();
    ctx.globalAlpha = alpha;
    ctx.fillStyle = it.face;
    ctx.beginPath();
    for (i = 0; i < it.bumps.length; i++) {
      var a = it.spin + (i / it.bumps.length) * Math.PI * 2;
      var d = R * 0.6;
      ctx.moveTo(Math.cos(a) * d + R * it.bumps[i], Math.sin(a) * d * 0.8);
      ctx.arc(Math.cos(a) * d, Math.sin(a) * d * 0.8, R * it.bumps[i], 0, Math.PI * 2);
    }
    ctx.arc(0, 0, R * 0.72, 0, Math.PI * 2);
    ctx.fill();
    // A lit rim on the side the light is on, which gives the cloud a
    // top and a bottom.
    ctx.globalAlpha = alpha * 0.5;
    ctx.strokeStyle = it.lit;
    ctx.lineWidth = 1.6;
    ctx.beginPath();
    ctx.arc(0, 0, R * 0.9, Math.PI * 1.05, Math.PI * 1.95);
    ctx.stroke();
    ctx.restore();
  };

  // A missile: something streaking in out of the dark and arriving.
  //
  // The first version of this was one tapering ribbon with a lens on the
  // front, and after three casts you had seen all of it. What the
  // reference art for this kind of effect actually has, and what this
  // has now, is layers: a smear of speed behind the head, a body that
  // wanders rather than following a clean curve, ribbons spiralling
  // round it, chevrons flicked off the sides, flame or frost shedding
  // backwards, and a head that is a different object each time - an
  // arrowhead, a comet, a lance, a shard, a star.
  //
  // Everything is chosen once when the missile spawns, so it keeps its
  // identity for the whole flight instead of boiling.

  // The kinds of thing that can be on the front of one.
  var MISSILE_HEADS = ['arrow', 'orb', 'lance', 'shard', 'star'];

  DiceBoard.prototype.drawMissile = function (ctx, it, t, alpha) {
    var i, self = this;
    // Where it is now and where it has been. The arc is a bezier from
    // its starting point to the target, bent sideways so it comes in on
    // a curve rather than a straight line - and then pushed off that
    // curve again by a slow noise, so no two flights are the same line
    // even between two missiles that started in the same place.
    var mx = (it.x0 + it.x1) / 2 + it.bend[0];
    var my = (it.y0 + it.y1) / 2 + it.bend[1];
    var mz = Math.max(it.z0, 40) * 0.8 + it.lift;
    var at = function (u) {
      var e = Math.min(1, Math.max(0, u));
      var k = 1 - e;
      var wob = it.wander * Math.sin(e * 6.1 + it.seed) *
                (fbm1(it.seed * 2.3 + e * 3.4, 2) - 0.5) * 4;
      return [
        k * k * it.x0 + 2 * k * e * mx + e * e * it.x1 + wob,
        k * k * it.y0 + 2 * k * e * my + e * e * it.y1 + wob * 0.6,
        k * k * it.z0 + 2 * k * e * mz + e * e * it.z1
      ];
    };

    var head = Math.min(1, t / 0.72);
    var tail = Math.max(0, head - it.trail);
    var steps = 28;
    var pts = [], wide = [], world = [];
    for (i = 0; i <= steps; i++) {
      var u = tail + (head - tail) * (i / steps);
      var w3 = at(u);
      world.push(w3);
      pts.push(this.projectUp(w3));
      // Widest just behind the head, nothing at the very back, and
      // lumpy all the way along.
      var f = i / steps;
      wide.push(it.w * Math.pow(f, 0.55) *
                (0.55 + 0.7 * fbm1(it.seed + f * 7 + it.churn * t * 6, 3)));
    }

    // The two edges of the trail, off the path's own direction.
    var left = [], right = [], norm = [];
    for (i = 0; i <= steps; i++) {
      var a = pts[Math.max(0, i - 1)], b = pts[Math.min(steps, i + 1)];
      var tx = b[0] - a[0], ty = b[1] - a[1];
      var len = Math.hypot(tx, ty) || 1;
      var nx = -ty / len, ny = tx / len;
      norm.push([nx, ny, tx / len, ty / len]);
      left.push([pts[i][0] + nx * wide[i], pts[i][1] + ny * wide[i]]);
      right.push([pts[i][0] - nx * wide[i], pts[i][1] - ny * wide[i]]);
    }

    var ribbon = function (colour, scale, a2) {
      ctx.globalAlpha = alpha * a2;
      ctx.fillStyle = colour;
      ctx.beginPath();
      ctx.moveTo(pts[0][0], pts[0][1]);
      for (i = 0; i <= steps; i++) {
        var mx2 = pts[i][0], my2 = pts[i][1];
        ctx.lineTo(mx2 + (left[i][0] - mx2) * scale, my2 + (left[i][1] - my2) * scale);
      }
      for (i = steps; i >= 0; i--) {
        var mx3 = pts[i][0], my3 = pts[i][1];
        ctx.lineTo(mx3 + (right[i][0] - mx3) * scale, my3 + (right[i][1] - my3) * scale);
      }
      ctx.closePath();
      ctx.fill();
    };

    var hp = pts[steps], hb = pts[steps - 1];
    var hang = Math.atan2(hp[1] - hb[1], hp[0] - hb[0]);

    ctx.save();
    ctx.globalCompositeOperation = 'lighter';

    // The smear: a long soft oval lying along the last stretch of the
    // path. This is the motion blur, and it is what stops the trail
    // reading as a drawn shape rather than as something moving fast.
    var sm = pts[Math.max(0, steps - 7)];
    var smr = it.w * 5.5;
    ctx.save();
    ctx.translate((hp[0] + sm[0]) / 2, (hp[1] + sm[1]) / 2);
    ctx.rotate(hang);
    var sg = ctx.createRadialGradient(0, 0, 0, 0, 0, smr);
    sg.addColorStop(0, it.face);
    sg.addColorStop(0.5, it.glow);
    sg.addColorStop(1, 'rgba(0,0,0,0)');
    ctx.globalAlpha = alpha * 0.3;
    ctx.fillStyle = sg;
    ctx.beginPath();
    ctx.ellipse(-Math.hypot(hp[0] - sm[0], hp[1] - sm[1]) * 0.25, 0,
                smr * 1.5, smr * 0.42, 0, 0, Math.PI * 2);
    ctx.fill();
    ctx.restore();

    // The body, four passes from wide and faint to narrow and white.
    ribbon(it.glow, 2.9, 0.15);
    ribbon(it.glow, 1.7, 0.28);
    ribbon(it.face, 1, 0.7);
    ribbon(it.core, 0.4, 0.95);

    // Ribbons spiralling round it. Two or three thin helices offset
    // along the normal by a sine of how far down the trail they are,
    // which from any one angle reads as something twisting.
    for (var sp = 0; sp < it.spirals; sp++) {
      var phase = (sp / Math.max(1, it.spirals)) * Math.PI * 2 + it.seed;
      ctx.globalAlpha = alpha * 0.55;
      ctx.strokeStyle = sp % 2 ? it.core : it.face;
      ctx.lineWidth = Math.max(0.6, it.w * 0.22);
      ctx.lineCap = 'round';
      ctx.beginPath();
      for (i = 0; i <= steps; i++) {
        var sw = Math.sin((i / steps) * it.twist + phase) * wide[i] * 2.1;
        var sx = pts[i][0] + norm[i][0] * sw;
        var sy = pts[i][1] + norm[i][1] * sw;
        if (i) { ctx.lineTo(sx, sy); } else { ctx.moveTo(sx, sy); }
      }
      ctx.stroke();
    }

    // Chevrons: short strokes flicked off the trail at an angle, which
    // is the one bit of the reference art that reads as speed rather
    // than as light.
    if (it.chevrons) {
      ctx.globalAlpha = alpha * 0.5;
      ctx.strokeStyle = it.face;
      ctx.lineWidth = Math.max(0.5, it.w * 0.16);
      for (i = 4; i < steps - 1; i += 3) {
        var cs = 1 - i / steps;
        var side = (i % 6 < 3) ? 1 : -1;
        var cl = wide[i] * (1.4 + 2.4 * (1 - cs));
        ctx.beginPath();
        ctx.moveTo(pts[i][0] + norm[i][0] * side * wide[i] * 0.6,
                   pts[i][1] + norm[i][1] * side * wide[i] * 0.6);
        ctx.lineTo(pts[i][0] + norm[i][0] * side * cl - norm[i][2] * cl * 0.9,
                   pts[i][1] + norm[i][1] * side * cl - norm[i][3] * cl * 0.9);
        ctx.stroke();
      }
    }

    // Flame tongues shed backwards off the body, for the ones that burn.
    if (it.burns) {
      for (i = 3; i < steps - 2; i += 2) {
        var fl = (1 - i / steps);
        var fa = fbm1(it.seed * 4 + i * 1.7 + t * 14, 2);
        var flen = wide[i] * (2.2 + fa * 4.5);
        ctx.globalAlpha = alpha * 0.34 * (i / steps);
        ctx.fillStyle = fa > 0.62 ? it.core : it.face;
        ctx.beginPath();
        ctx.moveTo(pts[i][0] + norm[i][0] * wide[i], pts[i][1] + norm[i][1] * wide[i]);
        ctx.quadraticCurveTo(
          pts[i][0] - norm[i][2] * flen * 0.5 + norm[i][0] * flen * 0.8,
          pts[i][1] - norm[i][3] * flen * 0.5 + norm[i][1] * flen * 0.8,
          pts[i][0] - norm[i][2] * flen, pts[i][1] - norm[i][3] * flen);
        ctx.quadraticCurveTo(
          pts[i][0] - norm[i][2] * flen * 0.5 - norm[i][0] * flen * 0.4,
          pts[i][1] - norm[i][3] * flen * 0.5 - norm[i][1] * flen * 0.4,
          pts[i][0] - norm[i][0] * wide[i] * 0.4,
          pts[i][1] - norm[i][1] * wide[i] * 0.4);
        ctx.closePath();
        ctx.fill();
      }
    }

    // Grit in the trail: little points of light that are not on the
    // centre line, which is most of what "noisy" means here.
    ctx.fillStyle = it.core;
    for (i = 2; i < steps; i += 1) {
      if (nhash(it.seed * 9 + i) > it.grit) { continue; }
      var gsp = 1 - i / steps;
      var jx = (fbm1(it.seed * 3 + i * 2.7 + t * 9, 2) - 0.5) * wide[i] * 4.5;
      var jy = (fbm1(it.seed * 5 + i * 1.3 + t * 7, 2) - 0.5) * wide[i] * 4.5;
      ctx.globalAlpha = alpha * 0.55 * (i / steps);
      ctx.beginPath();
      ctx.arc(pts[i][0] + jx, pts[i][1] + jy,
              Math.max(0.4, wide[i] * 0.26 * gsp + 0.5), 0, Math.PI * 2);
      ctx.fill();
    }
    ctx.restore();

    // ---- the head. A different object depending on the missile.
    ctx.save();
    ctx.globalCompositeOperation = 'lighter';
    ctx.translate(hp[0], hp[1]);
    ctx.rotate(hang);
    var hr = it.w * 1.5;
    // The bloom behind whatever the head is, common to all of them.
    var hg = ctx.createRadialGradient(0, 0, 0, 0, 0, hr * 3.4);
    hg.addColorStop(0, it.core);
    hg.addColorStop(0.28, it.face);
    hg.addColorStop(1, 'rgba(0,0,0,0)');
    ctx.globalAlpha = alpha * 0.9;
    ctx.fillStyle = hg;
    ctx.beginPath();
    ctx.ellipse(0, 0, hr * 3.4, hr * 1.7, 0, 0, Math.PI * 2);
    ctx.fill();

    ctx.fillStyle = it.core;
    ctx.strokeStyle = it.core;
    ctx.globalAlpha = alpha * 0.95;
    if (it.head === 'arrow') {
      // A chevron arrowhead with a notch cut out of the back of it.
      ctx.beginPath();
      ctx.moveTo(hr * 2.6, 0);
      ctx.lineTo(-hr * 0.9, -hr * 1.15);
      ctx.lineTo(-hr * 0.1, 0);
      ctx.lineTo(-hr * 0.9, hr * 1.15);
      ctx.closePath();
      ctx.fill();
    } else if (it.head === 'lance') {
      // A long thin spike with a collar behind it.
      ctx.beginPath();
      ctx.moveTo(hr * 4.2, 0);
      ctx.lineTo(-hr * 1.2, -hr * 0.42);
      ctx.lineTo(-hr * 1.2, hr * 0.42);
      ctx.closePath();
      ctx.fill();
      ctx.globalAlpha = alpha * 0.7;
      ctx.lineWidth = hr * 0.3;
      ctx.beginPath();
      ctx.moveTo(-hr * 0.9, -hr * 1.1);
      ctx.lineTo(-hr * 0.9, hr * 1.1);
      ctx.stroke();
    } else if (it.head === 'shard') {
      // A crystal: a long facet and a short one, so it has a side that
      // catches the light and a side that does not.
      ctx.beginPath();
      ctx.moveTo(hr * 2.8, 0);
      ctx.lineTo(0, -hr * 0.95);
      ctx.lineTo(-hr * 1.4, 0);
      ctx.lineTo(0, hr * 0.7);
      ctx.closePath();
      ctx.fill();
      ctx.globalAlpha = alpha * 0.55;
      ctx.fillStyle = it.face;
      ctx.beginPath();
      ctx.moveTo(hr * 2.8, 0);
      ctx.lineTo(0, hr * 0.7);
      ctx.lineTo(-hr * 1.4, 0);
      ctx.closePath();
      ctx.fill();
      ctx.fillStyle = it.core;
    } else if (it.head === 'star') {
      // Four points, the long axis along the way it is going.
      ctx.beginPath();
      for (i = 0; i < 8; i++) {
        var sa = (i / 8) * Math.PI * 2;
        var srr = (i % 2 ? 0.34 : (i % 4 === 0 ? 2.9 : 1.2)) * hr;
        var px = Math.cos(sa) * srr, py = Math.sin(sa) * srr * 0.8;
        if (i) { ctx.lineTo(px, py); } else { ctx.moveTo(px, py); }
      }
      ctx.closePath();
      ctx.fill();
    } else {
      // A comet: a round head with a hot pit in the middle of it.
      ctx.beginPath();
      ctx.ellipse(hr * 0.3, 0, hr * 1.5, hr * 1.15, 0, 0, Math.PI * 2);
      ctx.fill();
      ctx.globalAlpha = alpha * 0.6;
      ctx.fillStyle = it.face;
      ctx.beginPath();
      ctx.ellipse(hr * 0.1, 0, hr * 0.7, hr * 0.5, 0, 0, Math.PI * 2);
      ctx.fill();
      ctx.fillStyle = it.core;
    }

    // The cross flare, which says "this is bright" more than any amount
    // of glow does. Its length breathes, so it twinkles.
    var fl2 = 0.8 + 0.4 * Math.sin(t * 40 + it.seed);
    ctx.strokeStyle = it.core;
    ctx.lineWidth = 1.2;
    ctx.globalAlpha = alpha * 0.8;
    ctx.beginPath();
    ctx.moveTo(-hr * 5 * fl2, 0); ctx.lineTo(hr * 6 * fl2, 0);
    ctx.moveTo(0, -hr * 2 * fl2); ctx.lineTo(0, hr * 2 * fl2);
    if (it.head === 'star' || it.head === 'shard') {
      ctx.moveTo(-hr * 2.2 * fl2, -hr * 2.2 * fl2);
      ctx.lineTo(hr * 2.2 * fl2, hr * 2.2 * fl2);
      ctx.moveTo(-hr * 2.2 * fl2, hr * 2.2 * fl2);
      ctx.lineTo(hr * 2.2 * fl2, -hr * 2.2 * fl2);
    }
    ctx.stroke();
    ctx.restore();
    ctx.globalAlpha = 1;

    // ---- what it sheds on the way past.
    //
    // Real particles, pushed into the effect's own list, so they keep
    // falling and cooling after the missile that dropped them has gone.
    // Rate-limited by wall position rather than by frame, or a slow
    // frame would drop a different number than a fast one.
    if (it.shed && head < 1 && this.elem) {
      var want = Math.floor(head / it.shed);
      if (want > (it.shedded || 0)) {
        it.shedded = want;
        var hw = world[steps];
        var born = it.born + t * it.life;
        for (var k = 0; k < it.shedN; k++) {
          if (it.burns) {
            this.elem.items.push({ what: 'ember',
              x: hw[0] + (Math.random() - 0.5) * 12,
              y: hw[1] + (Math.random() - 0.5) * 10,
              z: Math.max(2, hw[2]) + (Math.random() - 0.5) * 12,
              vx: (Math.random() - 0.5) * 40, vy: (Math.random() - 0.5) * 30,
              vz: -20 + Math.random() * 60, r: 0.8 + Math.random() * 1.6,
              seed: Math.random() * 50, face: it.face,
              born: born, life: 420 + Math.random() * 420, grow: 0 });
          } else {
            this.elem.items.push({ what: 'mote',
              x: hw[0] + (Math.random() - 0.5) * 14,
              y: hw[1] + (Math.random() - 0.5) * 11,
              z: Math.max(2, hw[2]) + (Math.random() - 0.5) * 14,
              vz: -14 + Math.random() * 30, r: 0.6 + Math.random() * 1.5,
              seed: Math.random() * 40, face: Math.random() < 0.4 ? it.core : it.face,
              born: born, life: 340 + Math.random() * 420, grow: 0 });
          }
        }
      }
    }
  };

  var RUNE_STROKES = [
    [[0, -1, 0, 1], [-0.6, -0.4, 0.6, -0.4]],
    [[-0.6, -1, 0.6, -1], [0, -1, 0, 1], [-0.5, 1, 0.5, 1]],
    [[-0.6, 1, 0, -1], [0, -1, 0.6, 1], [-0.3, 0.1, 0.3, 0.1]],
    [[0, -1, 0, 1], [0, -0.3, 0.7, -1], [0, -0.3, -0.7, -1]],
    [[-0.6, -1, -0.6, 1], [-0.6, -1, 0.5, -0.2], [-0.6, 0.2, 0.5, -0.2]],
    [[0, -1, 0, 1], [-0.55, -0.7, 0.55, -0.7], [-0.4, 0.5, 0.4, 0.5]],
    [[-0.5, -1, 0.5, -1], [0, -1, 0, 0.2], [-0.5, 1, 0, 0.2], [0.5, 1, 0, 0.2]],
    [[-0.6, -0.8, 0.6, 0.8], [0.6, -0.8, -0.6, 0.8]],
    [[0, -1, 0.6, 0], [0.6, 0, 0, 1], [0, 1, -0.6, 0], [-0.6, 0, 0, -1]],
    [[-0.5, -1, -0.5, 1], [0.5, -1, 0.5, 1], [-0.5, 0, 0.5, 0]]
  ];

  // Four circles worth drawing. They differ in what the inner figure is,
  // how much writing goes round the outside and what colour the whole
  // thing burns.
  var CIRCLE_STYLES = [
    { ink: '64, 150, 232', hot: '190, 236, 255', points: 5, skip: 2,
      runes: 24, rings: [1, 0.94, 0.8, 0.52, 0.2], ticks: 48 },
    { ink: '214, 146, 28', hot: '255, 238, 176', points: 6, skip: 2,
      runes: 18, rings: [1, 0.9, 0.74, 0.58, 0.24], ticks: 36 },
    { ink: '140, 78, 226', hot: '226, 200, 255', points: 7, skip: 3,
      runes: 30, rings: [1, 0.96, 0.86, 0.46, 0.18], ticks: 56 },
    { ink: '30, 176, 124', hot: '188, 250, 222', points: 8, skip: 3,
      runes: 16, rings: [1, 0.88, 0.7, 0.5, 0.28], ticks: 32 }
  ];

  DiceBoard.prototype.drawCircleSigil = function (ctx, it, grown, alpha, now) {
    var st = it.style, R = it.r * grown, i, k;
    var p = this.project([it.x, it.y, 0]);
    var age = (now - it.born) / 1000;
    var ink = function (a) { return 'rgba(' + st.ink + ',' + a.toFixed(3) + ')'; };
    var hot = function (a) { return 'rgba(' + st.hot + ',' + a.toFixed(3) + ')'; };

    ctx.save();
    ctx.translate(p[0], p[1]);
    ctx.scale(p[2], p[2] * 0.55);
    ctx.globalCompositeOperation = 'lighter';
    ctx.lineCap = 'round';
    ctx.lineJoin = 'round';

    // The wash under it, so the circle sits in a pool of its own light
    // rather than being a wire frame lying on parchment.
    var wash = ctx.createRadialGradient(0, 0, 0, 0, 0, R * 1.15);
    wash.addColorStop(0, ink(0.16 * alpha));
    wash.addColorStop(0.75, ink(0.22 * alpha));
    wash.addColorStop(1, ink(0));
    ctx.fillStyle = wash;
    ctx.beginPath();
    ctx.arc(0, 0, R * 1.15, 0, Math.PI * 2);
    ctx.fill();

    // ---- the rings. Two hairlines together read as one engraved line,
    // which is what the drawn ones do and a single stroke never does.
    var ringA = function (r, w, a) {
      ctx.strokeStyle = ink(a * alpha);
      ctx.lineWidth = w;
      ctx.beginPath();
      ctx.arc(0, 0, R * r, 0, Math.PI * 2);
      ctx.stroke();
    };
    ringA(st.rings[0], 3, 1);
    ringA(st.rings[0] - 0.035, 1.4, 0.8);
    ringA(st.rings[1], 1.4, 0.7);
    ringA(st.rings[2], 3.6, 1);
    ringA(st.rings[2] - 0.03, 1.2, 0.65);
    ringA(st.rings[3], 1.9, 0.85);
    ringA(st.rings[4], 1.4, 0.9);

    // ---- the outer ring is broken into arcs that turn on their own,
    // which is the detail that makes the whole thing look mechanical.
    var seg = 12, gap = 0.34;
    ctx.strokeStyle = ink(1 * alpha);
    ctx.lineWidth = 6;
    for (i = 0; i < seg; i++) {
      var a0 = age * 0.5 + (i / seg) * Math.PI * 2;
      ctx.beginPath();
      ctx.arc(0, 0, R * (st.rings[0] + 0.06), a0, a0 + (Math.PI * 2 / seg) * (1 - gap));
      ctx.stroke();
    }

    // ---- the writing, turning the other way.
    var rr = R * (st.rings[1] + st.rings[2]) / 2;
    var gh = R * (st.rings[1] - st.rings[2]) * 0.34;
    ctx.strokeStyle = ink(1 * alpha);
    ctx.lineWidth = 2.2;
    for (i = 0; i < st.runes; i++) {
      var ra = -age * 0.34 + (i / st.runes) * Math.PI * 2;
      var glyph = RUNE_STROKES[(i * 7 + it.seed) % RUNE_STROKES.length];
      ctx.save();
      ctx.translate(Math.cos(ra) * rr, Math.sin(ra) * rr);
      ctx.rotate(ra + Math.PI / 2);
      ctx.beginPath();
      for (k = 0; k < glyph.length; k++) {
        ctx.moveTo(glyph[k][0] * gh, glyph[k][1] * gh);
        ctx.lineTo(glyph[k][2] * gh, glyph[k][3] * gh);
      }
      ctx.stroke();
      ctx.restore();
    }

    // ---- ticks pointing inwards off the heavy ring.
    ctx.strokeStyle = ink(0.8 * alpha);
    ctx.lineWidth = 1.5;
    for (i = 0; i < st.ticks; i++) {
      var ta = age * 0.22 + (i / st.ticks) * Math.PI * 2;
      var long = i % 4 === 0 ? 0.085 : 0.045;
      ctx.beginPath();
      ctx.moveTo(Math.cos(ta) * R * st.rings[2], Math.sin(ta) * R * st.rings[2]);
      ctx.lineTo(Math.cos(ta) * R * (st.rings[2] - long),
                 Math.sin(ta) * R * (st.rings[2] - long));
      ctx.stroke();
    }

    // ---- the star, drawn in one stroke the way a pentagram is: step
    // round the points skipping some each time until you are back where
    // you started.
    var pr = R * st.rings[3];
    var spin = -age * 0.62;
    ctx.strokeStyle = ink(1 * alpha);
    ctx.lineWidth = 2.8;
    ctx.beginPath();
    for (i = 0; i <= st.points; i++) {
      var sa = spin + ((i * st.skip) % st.points) / st.points * Math.PI * 2 - Math.PI / 2;
      var x = Math.cos(sa) * pr, y = Math.sin(sa) * pr;
      if (i) { ctx.lineTo(x, y); } else { ctx.moveTo(x, y); }
    }
    ctx.closePath();
    ctx.stroke();

    // ---- and a small circle sitting on each of its points.
    for (i = 0; i < st.points; i++) {
      var ca = spin + (i / st.points) * Math.PI * 2 - Math.PI / 2;
      ctx.strokeStyle = ink(0.7 * alpha);
      ctx.lineWidth = 1.3;
      ctx.beginPath();
      ctx.arc(Math.cos(ca) * pr, Math.sin(ca) * pr, R * 0.055, 0, Math.PI * 2);
      ctx.stroke();
      ctx.fillStyle = hot(0.5 * alpha);
      ctx.beginPath();
      ctx.arc(Math.cos(ca) * pr, Math.sin(ca) * pr, R * 0.018, 0, Math.PI * 2);
      ctx.fill();
    }

    // ---- the heart of it, which breathes.
    var beat = 0.72 + 0.28 * Math.sin(age * 5);
    var core = ctx.createRadialGradient(0, 0, 0, 0, 0, R * st.rings[4] * 1.6);
    core.addColorStop(0, hot(0.7 * alpha * beat));
    core.addColorStop(0.5, ink(0.3 * alpha * beat));
    core.addColorStop(1, ink(0));
    ctx.fillStyle = core;
    ctx.beginPath();
    ctx.arc(0, 0, R * st.rings[4] * 1.6, 0, Math.PI * 2);
    ctx.fill();
    ctx.restore();
  };

  // pickLook chooses one of an element's variants and folds it over the
  // base. Every element may carry a variants list; the same fire twice
  // running should not be the same fire, and the cheapest way to get that
  // is a handful of hand-made versions rather than more randomness inside
  // one of them.
  function pickLook(look) {
    if (!look || !look.variants || !look.variants.length) { return look; }
    var v = look.variants[Math.floor(Math.random() * look.variants.length)];
    var out = {}, k;
    for (k in look) { if (k !== 'variants') { out[k] = look[k]; } }
    for (k in v) { out[k] = v[k]; }
    return out;
  }

  // ---- the shapes --------------------------------------------------------

  // A faceted solid standing on the page: a crystal, a blade, a boulder.
  //
  // One routine for all of them, because they only differ in their band
  // table - how many rings of vertices there are, how far up each sits and
  // how wide it is. A crystal is five narrowing rings ending in a point; a
  // boulder is six that bulge low down and close over the top. Everything
  // else is the same work.
  //
  // Three things turn that into something that reads as a real object:
  //
  //   - per-vertex wobble, made once when the solid is spawned so it keeps
  //     its shape while it grows instead of boiling;
  //   - per-band drift, so the thing leans and is not a barrel; and
  //   - per-facet tone, so two facets at the same angle are still
  //     different. That last one is what low-poly rock is made of, and
  //     without it a nine-sided boulder reads as a smooth lump.
  //
  // Shading is two fills rather than a choice between two colours: the
  // dark side first, then the lit colour over it at that facet's own
  // strength. Two inputs, a whole range of facets out of them, and the
  // steps between them stay hard, which is what makes it look cut.
  DiceBoard.prototype.drawFaceted = function (ctx, it, grown, alpha) {
    var sides = it.sides || 7, bands = it.bands, i, b;
    var h = it.h * grown, self = this;

    var rings = bands.map(function (band, bi) {
      var jit = it.facets[bi] || [];
      var dx = it.lean[0] * band.z, dy = it.lean[1] * band.z;
      var out = [];
      for (i = 0; i < sides; i++) {
        var a = it.spin + (i / sides) * Math.PI * 2;
        var rr = it.r * band.r * (jit[i] || 1) * grown;
        out.push(self.projectUp([it.x + dx + Math.cos(a) * rr,
                                 it.y + dy + Math.sin(a) * rr * 0.82,
                                 h * band.z]));
      }
      return out;
    });

    // A band of radius nought is a point, not a ring: the tip of a crystal.
    var last = bands.length - 1;
    var apex = bands[last].r === 0
      ? this.projectUp([it.x + it.lean[0], it.y + it.lean[1], h])
      : null;

    for (b = 0; b < last; b++) {
      var lo = rings[b], hi = rings[b + 1];
      var pointed = apex && b === last - 1;
      // Facets higher up face more upward and so catch more light.
      var up = 0.18 + 0.4 * (b / Math.max(1, last - 1));
      var tones = it.tones[b] || [];
      for (i = 0; i < sides; i++) {
        var j = (i + 1) % sides;
        var a2 = it.spin + ((i + 0.5) / sides) * Math.PI * 2;
        var lit = (Math.max(0, Math.cos(a2 - 2.2)) * (1 - up) + up) *
                  (tones[i] || 1);
        ctx.beginPath();
        ctx.moveTo(lo[i][0], lo[i][1]);
        ctx.lineTo(lo[j][0], lo[j][1]);
        if (pointed) {
          ctx.lineTo(apex[0], apex[1]);
        } else {
          ctx.lineTo(hi[j][0], hi[j][1]);
          ctx.lineTo(hi[i][0], hi[i][1]);
        }
        ctx.closePath();
        ctx.globalAlpha = alpha;
        ctx.fillStyle = it.side;
        ctx.fill();
        if (lit > 0.02) {
          ctx.globalAlpha = alpha * Math.min(1, lit);
          ctx.fillStyle = it.face;
          ctx.fill();
        }
      }
      if (pointed) { break; }
    }

    // The cap, where there is one. It faces straight up and so is the
    // brightest plane on the solid.
    if (!apex) {
      var cap = rings[last];
      ctx.globalAlpha = alpha;
      ctx.fillStyle = it.face;
      ctx.beginPath();
      for (i = 0; i < sides; i++) {
        if (i) { ctx.lineTo(cap[i][0], cap[i][1]); }
        else { ctx.moveTo(cap[i][0], cap[i][1]); }
      }
      ctx.closePath();
      ctx.fill();
    }

    // A bright seam up one corner, which is what makes ice read as ice.
    if (it.edge) {
      ctx.globalAlpha = alpha * 0.75;
      ctx.strokeStyle = it.edge;
      ctx.lineWidth = 1;
      ctx.beginPath();
      ctx.moveTo(rings[0][0][0], rings[0][0][1]);
      for (b = 1; b <= last; b++) { ctx.lineTo(rings[b][0][0], rings[b][0][1]); }
      if (apex) { ctx.lineTo(apex[0], apex[1]); }
      ctx.stroke();
    }
    ctx.globalAlpha = 1;
  };

  // The band tables. A crystal narrows in steps to a point; a blade is the
  // same idea with fewer, tighter steps; a boulder bulges low down and
  // closes over a flat top.
  var BANDS_ICE = [
    { z: 0, r: 1 }, { z: 0.22, r: 0.88 }, { z: 0.46, r: 0.66 },
    { z: 0.68, r: 0.46 }, { z: 0.86, r: 0.26 }, { z: 1, r: 0 }
  ];
  // A blade is not a cone either. It swells out of a wider footing, is
  // pinched at the waist and comes to a point out of a second, narrower
  // swell - which is what a shard driven through something looks like, and
  // is six flat planes' worth of detail rather than three.
  var BANDS_BLADE = [
    { z: 0, r: 1 }, { z: 0.12, r: 0.86 }, { z: 0.3, r: 0.94 },
    { z: 0.48, r: 0.6 }, { z: 0.66, r: 0.66 }, { z: 0.84, r: 0.3 },
    { z: 1, r: 0 }
  ];
  var ROCK_BANDS = [
    { z: 0, r: 0.7 }, { z: 0.16, r: 0.93 }, { z: 0.38, r: 1 },
    { z: 0.58, r: 0.88 }, { z: 0.76, r: 0.66 }, { z: 0.9, r: 0.44 },
    { z: 1, r: 0.26 }
  ];

  // facetsOf is the per-vertex wobble one solid keeps for its whole life.
  function facetsOf(n, spread) {
    var out = [];
    for (var i = 0; i < n; i++) {
      out.push(1 - spread / 2 + Math.random() * spread);
    }
    return out;
  }

  // solidOf builds one faceted thing: its wobble, its per-facet tone and
  // its lean, all fixed now so none of it changes while it grows.
  function solidOf(at, bands, sides, spread, tone) {
    var facets = [], tones = [], b;
    for (b = 0; b < bands.length; b++) {
      facets.push(facetsOf(sides, spread));
      tones.push(facetsOf(sides, tone === undefined ? 0.5 : tone));
    }
    at.bands = bands;
    at.sides = sides;
    at.facets = facets;
    at.tones = tones;
    at.spin = Math.random() * Math.PI * 2;
    return at;
  }

  // An irregular lump floating at height z: a rock heaved up out of the
  // ground. The same seed every frame, so it does not boil.
  DiceBoard.prototype.drawChunk = function (ctx, it, alpha) {
    var p = this.projectUp([it.x, it.y, it.z]);
    var n = 7, i;
    ctx.globalAlpha = alpha;
    ctx.fillStyle = it.face;
    ctx.beginPath();
    for (i = 0; i <= n; i++) {
      var a = (i / n) * Math.PI * 2 + it.spin;
      // A fixed wobble per vertex, from the seed, so the lump keeps its
      // shape while it rises.
      var wob = 0.72 + 0.34 * Math.abs(Math.sin(it.seed + i * 2.3));
      var rr = it.r * wob * p[2];
      var x = p[0] + Math.cos(a) * rr, y = p[1] + Math.sin(a) * rr * 0.82;
      if (i) { ctx.lineTo(x, y); } else { ctx.moveTo(x, y); }
    }
    ctx.closePath();
    ctx.fill();
    // A lit top edge, which is what stops it reading as a flat blob.
    ctx.strokeStyle = it.edge;
    ctx.lineWidth = 1.2;
    ctx.beginPath();
    ctx.ellipse(p[0], p[1] - it.r * 0.22 * p[2], it.r * 0.62 * p[2],
                it.r * 0.3 * p[2], 0, Math.PI * 1.05, Math.PI * 1.95);
    ctx.stroke();
    ctx.globalAlpha = 1;
  };

  // A tongue of flame.
  //
  // A flame is not a triangle. It is a column whose width falls away as it
  // rises and whose edges wander, and it wanders more the higher it goes
  // and the further it is from the fuel. So both edges are sampled up the
  // height with fbm driving the sway and the width, at a frequency that
  // rises with height - which is the fractal part, and the whole
  // difference between fire and bunting.
  //
  // Time goes into the noise as well, so the same flame keeps reshaping
  // itself instead of standing still and merely growing.
  DiceBoard.prototype.drawFlame = function (ctx, it, grown, alpha, now) {
    var steps = 22, i;
    var h = it.h * grown;
    var age = (now - it.born) / 1000;
    var left = [], right = [];
    for (i = 0; i <= steps; i++) {
      var u = i / steps;
      // Narrowing towards the tip, but not smoothly: noise at two scales
      // pinches and swells it on the way up - a slow one for the body of
      // the flame and a fast one for the ragged edge - and the fast one
      // gets stronger towards the tip, where a real flame comes apart.
      var taper = Math.pow(1 - u, 0.72);
      // Four octaves multiplied together rather than added. Added noise
      // gives a wobbly edge; multiplied noise can pinch the flame to
      // nearly nothing and then let it swell again, which is what makes
      // a real one look like it is coming apart into tongues instead of
      // being one lumpy leaf. Each octave runs faster than the last, in
      // space and in time, so the tip boils and the base only breathes.
      var lump = 1, o, f;
      for (o = 0; o < 4; o++) {
        f = Math.pow(2.15, o);
        lump *= 0.68 + 0.64 * fbm1(it.seed * (o * 1.7 + 1) + u * 3.1 * f +
                                   age * (2.1 + o * 2.4), 2);
      }
      // And a set of tongues cut into the edge, sharpening towards the
      // top where the flame is thinnest and tearing most.
      var tongue = 1 - 0.42 * u * u *
        Math.pow(Math.abs(Math.sin(u * 9.3 + it.seed * 2.2 + age * 5.5)), 0.6);
      var w = it.r * taper * Math.max(0.1, lump * tongue);
      // The sway builds with height, the way a flame leans off its own
      // draught rather than being blown over from the bottom - with a
      // faster wobble on top of it so the tip flickers.
      var sway = (fbm1(it.seed * 1.7 + u * 2.1 + age * 3.1, 3) - 0.5) *
                 it.r * 3.6 * u * u +
                 (fbm1(it.seed * 5.3 + u * 9 + age * 9, 2) - 0.5) * it.r * 1.4 * u;
      var z = h * u;
      left.push(this.projectUp([it.x + sway - w, it.y + sway * 0.3, z]));
      right.push(this.projectUp([it.x + sway + w, it.y + sway * 0.3, z]));
    }
    ctx.save();
    ctx.globalAlpha = alpha;
    var lo = left[0], hi = left[steps];
    var g = ctx.createLinearGradient(0, hi[1], 0, lo[1]);
    g.addColorStop(0, it.tip);
    g.addColorStop(0.45, it.face);
    g.addColorStop(1, it.side);
    ctx.fillStyle = g;
    ctx.beginPath();
    ctx.moveTo(left[0][0], left[0][1]);
    for (i = 1; i <= steps; i++) { ctx.lineTo(left[i][0], left[i][1]); }
    for (i = steps; i >= 0; i--) { ctx.lineTo(right[i][0], right[i][1]); }
    ctx.closePath();
    ctx.fill();
    // The hotter heart of it, the same shape drawn narrower and shorter.
    ctx.globalAlpha = alpha * 0.85;
    ctx.fillStyle = it.tip;
    ctx.beginPath();
    var inner = Math.round(steps * 0.62);
    for (i = 0; i <= inner; i++) {
      var m = (left[i][0] + right[i][0]) / 2;
      ctx.lineTo(m + (left[i][0] - m) * 0.42, left[i][1]);
    }
    for (i = inner; i >= 0; i--) {
      var m2 = (left[i][0] + right[i][0]) / 2;
      ctx.lineTo(m2 + (right[i][0] - m2) * 0.42, right[i][1]);
    }
    ctx.closePath();
    ctx.fill();
    ctx.restore();
  };

  // A soft round blot: smoke, cloud, a bubble, dust.
  DiceBoard.prototype.drawPuff = function (ctx, it, alpha) {
    var p = this.projectUp([it.x, it.y, it.z]);
    var r = Math.max(0.6, it.r * p[2]);
    var g = ctx.createRadialGradient(p[0], p[1], 0, p[0], p[1], r);
    g.addColorStop(0, it.face);
    g.addColorStop(1, 'rgba(0,0,0,0)');
    ctx.globalAlpha = alpha;
    ctx.fillStyle = g;
    ctx.beginPath();
    ctx.arc(p[0], p[1], r, 0, Math.PI * 2);
    ctx.fill();
    ctx.globalAlpha = 1;
  };

  // ------------------------------------------------------------ lightning
  //
  // A strike is not a zigzag line. Photographs of one show a trunk that
  // wanders at two scales at once - long slow leans with sharp kinks on
  // top of them - and forks that leave the trunk, wander a bit of the way
  // and stop in mid air. Drawing one line, however jagged, gets the idea
  // and none of the look, so this builds a path and then hangs branches
  // off it, both from a seed the bolt carries: the same bolt has to come
  // out the same shape on every frame of its short life or it writhes.
  //
  // boltPath walks from the top down to the strike point. The wander is
  // pinched to nothing at the bottom so the trunk lands where it was
  // aimed, and left wide open at the top where nobody is looking.
  DiceBoard.prototype.boltPath = function (from, to, sway, seed, steps) {
    var n = steps || 14, i, out = [];
    for (i = 0; i <= n; i++) {
      var t = i / n;
      // Free at the top, pinched only in the last stretch. A strike has
      // no idea where it is going until it is nearly there: the first
      // version of this eased off at both ends, which made every bolt
      // arrive as a straight tube and only kink in the middle.
      var room = (1 - Math.pow(t, 2.2)) * (0.55 + Math.sin(t * Math.PI) * 0.55);
      var slow = (fbm1(seed + t * 1.7, 2) - 0.5) * sway * 2.2;
      var kink = (nhash(seed * 31 + i) - 0.5) * sway * 1.15;
      var kink2 = (nhash(seed * 17 + i * 5 + 3) - 0.5) * sway * 0.6;
      out.push([
        from[0] + (to[0] - from[0]) * t + (slow + kink) * room,
        from[1] + (to[1] - from[1]) * t + (slow * 0.4 + kink2) * room * 0.6,
        from[2] + (to[2] - from[2]) * t]);
    }
    return out;
  };

  // One pass of a path, projected and stroked. Everything about a bolt is
  // this called several times over at different widths and alphas with
  // the composite set to lighter, which is what makes the middle of it go
  // white without ever being painted white.
  DiceBoard.prototype.strokePath = function (ctx, path, colour, width, alpha) {
    var i, p;
    ctx.globalAlpha = alpha;
    ctx.strokeStyle = colour;
    ctx.lineWidth = Math.max(0.4, width);
    ctx.lineCap = 'round';
    ctx.lineJoin = 'round';
    ctx.beginPath();
    for (i = 0; i < path.length; i++) {
      p = this.projectUp(path[i]);
      if (i) { ctx.lineTo(p[0], p[1]); } else { ctx.moveTo(p[0], p[1]); }
    }
    ctx.stroke();
    ctx.globalAlpha = 1;
  };

  // The forks. Each leaves the trunk at a node, keeps roughly the trunk's
  // heading with a lean on it, and dies well short of the ground - a fork
  // that reached the target would read as a second strike.
  DiceBoard.prototype.boltForks = function (trunk, sway, seed, howMany) {
    var out = [], k, i;
    var n = trunk.length - 1;
    for (k = 0; k < howMany; k++) {
      // Never off the last few nodes: a fork splitting a hair above the
      // impact looks like a mistake rather than a fork.
      var at = 2 + Math.floor(nhash(seed * 7 + k) * (n - 6));
      if (at < 1 || at >= n - 1) { continue; }
      var head = trunk[at];
      var run = 3 + Math.floor(nhash(seed * 13 + k * 3) * 5);
      var lean = (nhash(seed * 23 + k) - 0.5) * 2.4;
      var drop = (trunk[n][2] - head[2]) / (n - at);
      var path = [head], cx = head[0], cy = head[1], cz = head[2];
      for (i = 1; i <= run; i++) {
        cx += Math.cos(lean) * sway * (0.5 + nhash(seed + k * 9 + i) * 0.9);
        cy += Math.sin(lean) * sway * (0.3 + nhash(seed + k * 11 + i) * 0.5);
        cz += drop * (0.7 + nhash(seed + k * 5 + i) * 0.8);
        lean += (nhash(seed * 3 + k * 7 + i) - 0.5) * 1.1;
        // A fork stops in the air. One that carried on under the paper
        // would read as a second strike coming up out of the table.
        if (cz < 14) { path.push([cx, cy, 14]); break; }
        path.push([cx, cy, cz]);
      }
      out.push(path);
    }
    return out;
  };

  // A whole strike: trunk, forks, and - for the ribbon kind - a second
  // trunk laid a little off the first, which is what a wide slow bolt
  // photographed at a long exposure actually is.
  DiceBoard.prototype.drawStrike = function (ctx, it, flash, alpha) {
    var self = this;
    var from = [it.x + (it.slant === undefined ? 40 : it.slant),
                it.y - 30, it.h];
    var to = [it.x, it.y, 8];
    var w = (it.width || 4) * flash;
    var trunk = this.boltPath(from, to, it.sway, it.seed, it.steps || 14);

    // The dark first. The page is cream, and a white-hot line added on
    // top of cream is very nearly cream: what makes a strike read on a
    // bright ground is the storm round it, not the light in it. So a
    // wide soft bruise of dark ink goes down in normal blending before
    // anything is added over the top of it.
    ctx.save();
    this.strokePath(ctx, trunk, 'rgba(38,30,72,1)', w * 7, alpha * 0.16);
    this.strokePath(ctx, trunk, 'rgba(30,24,60,1)', w * 3.4, alpha * 0.26);
    ctx.restore();

    ctx.save();
    ctx.globalCompositeOperation = 'lighter';

    // The forks first and faintest, so the trunk reads as the bright one.
    // And forks off the forks: one level of branching reads as a diagram,
    // two reads as lightning, because that is where the eye stops being
    // able to count them.
    if (it.forks) {
      var boughs = this.boltForks(trunk, it.sway * 0.55, it.seed + 41, it.forks);
      boughs.forEach(function (f, k) {
        var fw = w * (0.5 - k * 0.05);
        if (fw < 0.3) { return; }
        self.strokePath(ctx, f, it.glow, fw * 3.4, alpha * 0.14);
        self.strokePath(ctx, f, it.glow, fw * 1.4, alpha * 0.3);
        self.strokePath(ctx, f, it.face, fw * 0.4, alpha * 0.7);
        if (f.length < 4 || fw < 0.9) { return; }
        self.boltForks(f, it.sway * 0.3, it.seed + 71 + k * 13, 2)
          .forEach(function (g) {
            self.strokePath(ctx, g, it.glow, fw * 1.0, alpha * 0.16);
            self.strokePath(ctx, g, it.face, fw * 0.24, alpha * 0.42);
          });
      });
    }

    // A ribbon bolt. Not a second random walk beside the first - that
    // reads as two strikes - but the same walk pushed sideways, so the
    // two edges belong to one channel of light and the gap between them
    // is the flat of the ribbon turning as it falls.
    if (it.ribbon) {
      var i2, edge = [[], []];
      var swing = (it.width || 4) * 0.9;
      for (i2 = 0; i2 < trunk.length; i2++) {
        var u2 = i2 / (trunk.length - 1);
        // The ribbon turns on its way down, so the two edges cross and
        // the channel narrows and opens again rather than running
        // parallel like a pipe.
        var turn = Math.sin(u2 * 5.5 + it.seed) * swing;
        edge[0].push([trunk[i2][0] + turn, trunk[i2][1] + turn * 0.4, trunk[i2][2]]);
        edge[1].push([trunk[i2][0] - turn, trunk[i2][1] - turn * 0.4, trunk[i2][2]]);
      }
      this.strokePath(ctx, edge[0], it.glow, w * 1.6, alpha * 0.3);
      this.strokePath(ctx, edge[1], it.glow, w * 1.6, alpha * 0.3);
      this.strokePath(ctx, edge[0], it.face, w * 0.3, alpha * 0.75);
      this.strokePath(ctx, edge[1], it.face, w * 0.3, alpha * 0.75);
    }

    // Four passes, widest and faintest outwards, so the light piles up
    // towards the middle and goes white on its own.
    this.strokePath(ctx, trunk, it.glow, w * 4.5, alpha * 0.22);
    this.strokePath(ctx, trunk, it.glow, w * 2.2, alpha * 0.4);
    this.strokePath(ctx, trunk, it.glow, w * 1.0, alpha * 0.7);
    this.strokePath(ctx, trunk, it.face, w * 0.36, alpha);

    // Beads: a dying strike breaks into a dotted line rather than fading
    // evenly, and it is the last thing you see of it.
    if (it.beads && flash < 0.6) {
      ctx.globalAlpha = alpha * (0.6 - flash) * 1.6;
      ctx.fillStyle = it.face;
      for (var i = 2; i < trunk.length; i += 2) {
        var p = this.projectUp(trunk[i]);
        var r = w * (0.4 + nhash(it.seed + i) * 0.7);
        ctx.beginPath();
        ctx.arc(p[0], p[1], Math.max(0.6, r), 0, Math.PI * 2);
        ctx.fill();
      }
      ctx.globalAlpha = 1;
    }
    ctx.restore();
  };

  // A ring lying on the paper: frost, a scorch, a shockwave. Stroked with
  // a radial gradient rather than a flat colour, so it has a soft inner
  // edge and fades out rather than ending - a hard hairline on parchment
  // reads as a drawing, and this is meant to read as a mark.
  DiceBoard.prototype.drawGroundRing = function (ctx, x, y, r, colour, width, alpha) {
    var p = this.project([x, y, 0]);
    var rr = r * p[2], w = width * p[2];
    var g = ctx.createRadialGradient(p[0], p[1], Math.max(0, rr - w),
                                     p[0], p[1], rr + w * 0.7);
    g.addColorStop(0, 'rgba(0,0,0,0)');
    g.addColorStop(0.45, colour);
    g.addColorStop(0.62, colour);
    g.addColorStop(1, 'rgba(0,0,0,0)');
    ctx.save();
    ctx.globalAlpha = alpha;
    ctx.strokeStyle = g;
    ctx.lineWidth = w;
    ctx.beginPath();
    ctx.ellipse(p[0], p[1], rr, rr * 0.42, 0, 0, Math.PI * 2);
    ctx.stroke();
    ctx.restore();
  };

  // A wave: a crest rolling across the page and breaking over the dice.
  //
  // The first version of this was one sine hump, which is a parabola with
  // extra steps and looks like one. A wave is not a curve, it is a body of
  // water with a shape, so this is built out of four things:
  //
  //   - a profile that is three sine harmonics at unrelated frequencies,
  //     one of them travelling, so the crest is lumpy and lopsided and no
  //     two frames of it are the same silhouette;
  //   - a waterline that is not straight either, because the front of a
  //     wave arrives before the rest of it;
  //   - a swell behind the crest, drawn first and darker, which is what
  //     gives it a back as well as a front; and
  //   - a lip that throws forward once it breaks, with the water under the
  //     overhang darker because it is water seen through water.
  //
  // Everything is see-through: an opaque wave hides the number that was
  // just rolled, which is the one thing on the table nobody wants covered.
  DiceBoard.prototype.drawCrest = function (ctx, it, t, alpha) {
    var n = 34, i;
    // Up, over, and down.
    var rise = Math.min(1, t / 0.34);
    var fall = t > 0.55 ? (t - 0.55) / 0.45 : 0;
    var h = it.h * rise * (1 - fall * 0.9);
    var lead = it.from + (it.r * 3.0) * Math.min(1, t / 0.62);
    var curl = Math.max(0, (t - 0.26) / 0.38) * 46;
    var self = this;

    // The height of the crest across its width. The envelope keeps the
    // ends on the ground; everything inside it is weather.
    var profile = function (u) {
      var env = Math.sin(u * Math.PI);
      var wob = 0.62
        + 0.20 * Math.sin(u * Math.PI * 3.3 + it.seed)
        + 0.12 * Math.sin(u * Math.PI * 7.1 - it.seed * 1.7 + t * 5)
        + 0.24 * fbm1(it.seed + u * 4.2, 3);
      return env * Math.max(0.15, wob);
    };
    // The waterline. The middle of the front runs ahead of the edges.
    var front = function (u) {
      return lead + Math.sin(u * Math.PI) * it.r * 0.22 +
             (fbm1(it.seed * 2.3 + u * 3.1, 2) - 0.5) * 12;
    };
    var acrossAt = function (u) {
      return it.x + (u - 0.5) * it.r * 2.3;
    };

    // band draws one sheet of water: a filled shape between the waterline
    // and a crest line, offset back and scaled so the swell behind sits
    // higher up the page than the face in front of it.
    var band = function (back, tall, wide, colour, a) {
      var base = [], top = [];
      for (i = 0; i <= n; i++) {
        var u = i / n;
        var wx = acrossAt(u) * 1 + (acrossAt(u) - it.x) * (wide - 1);
        var wy = front(u) - back;
        base.push(self.projectUp([wx, it.y + wy, 0]));
        top.push(self.projectUp([wx, it.y + wy + curl * profile(u) / 1.2,
                                 h * tall * profile(u)]));
      }
      ctx.globalAlpha = alpha * a;
      var g = ctx.createLinearGradient(0, top[n >> 1][1], 0, base[n >> 1][1]);
      g.addColorStop(0, colour[0]);
      g.addColorStop(1, colour[1]);
      ctx.fillStyle = g;
      ctx.beginPath();
      ctx.moveTo(base[0][0], base[0][1]);
      for (i = 0; i <= n; i++) { ctx.lineTo(top[i][0], top[i][1]); }
      for (i = n; i >= 0; i--) { ctx.lineTo(base[i][0], base[i][1]); }
      ctx.closePath();
      ctx.fill();
      return top;
    };

    ctx.save();
    // Back to front: the swell behind, the body, the overhang. Each is
    // darker than the one in front of it, which is what gives the water
    // thickness rather than being one pane of blue.
    band(it.r * 0.68, 1.22, 1.18, [it.deep, it.deep], 0.34);
    band(it.r * 0.3, 1.1, 1.08, [it.face, it.deep], 0.3);
    var crest = band(0, 1, 1, [it.face, it.deep], 0.5);
    if (curl > 2) { band(-curl * 0.4, 0.78, 0.92, [it.deep, it.deep], 0.32); }

    // Streaks down the face of it. Water falling has grain, and without
    // it the front of the wave is a flat wash however many layers are
    // behind it.
    ctx.globalAlpha = alpha * 0.28;
    ctx.strokeStyle = it.foam;
    ctx.lineWidth = 1.4;
    for (i = 2; i < n - 1; i += 2) {
      var us = i / n, pr = profile(us);
      if (pr < 0.25) { continue; }
      var down = 0.25 + 0.55 * fbm1(it.seed * 4.7 + us * 8, 2);
      var topP = crest[i];
      var footP = self.projectUp([acrossAt(us), it.y + front(us),
                                  h * pr * (1 - down)]);
      ctx.beginPath();
      ctx.moveTo(topP[0], topP[1]);
      ctx.lineTo(footP[0], footP[1]);
      ctx.stroke();
    }

    // The white along the top, thickest where the crest is highest - foam
    // gathers on the part that is breaking, not evenly along the whole
    // front.
    ctx.lineCap = 'round';
    ctx.lineJoin = 'round';
    for (i = 0; i < n; i++) {
      var u2 = (i + 0.5) / n;
      var pf = profile(u2);
      ctx.globalAlpha = alpha * (0.35 + 0.6 * pf) * 0.95;
      ctx.strokeStyle = it.foam;
      ctx.lineWidth = (2 + 7 * pf) * Math.min(1, t * 2.4);
      ctx.beginPath();
      ctx.moveTo(crest[i][0], crest[i][1]);
      ctx.lineTo(crest[i + 1][0], crest[i + 1][1]);
      ctx.stroke();
    }

    // And the foam that has torn off the lip: blobs sitting just above
    // the crest where it is breaking hardest, which is what stops the top
    // edge reading as a drawn line.
    if (curl > 6) {
      ctx.globalAlpha = alpha * 0.55;
      ctx.fillStyle = it.foam;
      for (i = 1; i < n; i += 3) {
        var u3 = i / n, pf3 = profile(u3);
        if (pf3 < 0.45) { continue; }
        var blob = 1.5 + 4.5 * fbm1(it.seed * 8.1 + u3 * 11 + t * 2, 2);
        ctx.beginPath();
        ctx.ellipse(crest[i][0] + (fbm1(it.seed + u3 * 15, 1) - 0.5) * 10,
                    crest[i][1] - blob * 1.4,
                    blob, blob * 0.78, 0, 0, Math.PI * 2);
        ctx.fill();
      }
    }
    ctx.restore();
  };

  // A shaft of light standing on the page.
  //
  // Not a cone: light coming down a column does not get wider on the way,
  // and drawing it as a wedge reads as a spotlight in a theatre rather
  // than as something arriving. So it is a cylinder - three of them, one
  // inside the other, each narrower and each brighter - which is what
  // gives the middle of it depth instead of being a flat pane of colour.
  //
  // And the grain: each cylinder is drawn as a row of vertical strips
  // whose brightness comes from noise that drifts upwards over time. Light
  // through air is never even, and the moment it is, the eye reads it as a
  // gradient somebody painted.
  DiceBoard.prototype.drawBeam = function (ctx, x, y, r, h, rgb, alpha, now) {
    var shells = [
      { r: 1.0, a: 0.3, strips: 16 },
      { r: 0.66, a: 0.42, strips: 11 },
      { r: 0.34, a: 0.6, strips: 7 }
    ];
    var lo = this.projectUp([x, y, 0]), hi = this.projectUp([x, y, h]);
    var drift = (now || 0) / 900;
    ctx.save();
    ctx.globalCompositeOperation = 'lighter';
    shells.forEach(function (sh, si) {
      var rl = r * sh.r * lo[2], rh = r * sh.r * hi[2];
      for (var i = 0; i < sh.strips; i++) {
        var u0 = i / sh.strips, u1 = (i + 1) / sh.strips;
        // How bright this strip is, and where in the noise it sits, so the
        // grain crawls up the shaft rather than flickering in place.
        var n = fbm1(si * 17 + u0 * 6 + drift, 3);
        var a = alpha * sh.a * (0.35 + 1.3 * n);
        if (a <= 0.004) { continue; }
        var g = ctx.createLinearGradient(0, hi[1], 0, lo[1]);
        g.addColorStop(0, 'rgba(' + rgb + ',0)');
        g.addColorStop(0.3, 'rgba(' + rgb + ',' + a.toFixed(3) + ')');
        g.addColorStop(1, 'rgba(' + rgb + ',' + (a * 0.45).toFixed(3) + ')');
        ctx.fillStyle = g;
        ctx.beginPath();
        ctx.moveTo(hi[0] - rh + 2 * rh * u0, hi[1]);
        ctx.lineTo(hi[0] - rh + 2 * rh * u1, hi[1]);
        ctx.lineTo(lo[0] - rl + 2 * rl * u1, lo[1]);
        ctx.lineTo(lo[0] - rl + 2 * rl * u0, lo[1]);
        ctx.closePath();
        ctx.fill();
      }
    });
    // The two edges of the outer cylinder, faintly. Light against cream
    // parchment has very little contrast to work with, and without a rim
    // the whole column reads as a smudge rather than as a shape.
    var rl0 = r * lo[2], rh0 = r * hi[2];
    ctx.strokeStyle = 'rgba(' + rgb + ',' + (0.5 * alpha).toFixed(3) + ')';
    ctx.lineWidth = 1.6;
    [-1, 1].forEach(function (sgn) {
      ctx.beginPath();
      ctx.moveTo(hi[0] + sgn * rh0, hi[1]);
      ctx.lineTo(lo[0] + sgn * rl0, lo[1]);
      ctx.stroke();
    });
    // The pool of light where it meets the paper, which is what stops the
    // column looking like it is hovering.
    var pg = ctx.createRadialGradient(lo[0], lo[1], 0, lo[0], lo[1], r * lo[2] * 1.6);
    pg.addColorStop(0, 'rgba(' + rgb + ',' + (0.75 * alpha).toFixed(3) + ')');
    pg.addColorStop(1, 'rgba(' + rgb + ',0)');
    ctx.fillStyle = pg;
    ctx.beginPath();
    ctx.ellipse(lo[0], lo[1], r * lo[2] * 1.6, r * lo[2] * 0.62, 0, 0, Math.PI * 2);
    ctx.fill();
    ctx.restore();
  };

  // ---- what each element puts on the table -------------------------------
  //
  // diceHeart is where the dice ended up and how far they spread, which is
  // what every recipe below arranges itself around.
  DiceBoard.prototype.diceHeart = function () {
    var x = 0, y = 0, r = 0, i;
    // A standing elemental with no dice under it says where it is instead.
    if (this.heart) { return this.heart; }
    if (!this.dice.length) { return null; }
    for (i = 0; i < this.dice.length; i++) {
      x += this.dice[i].pos[0];
      y += this.dice[i].pos[1];
      r = Math.max(r, this.dice[i].size);
    }
    x /= this.dice.length;
    y /= this.dice.length;
    var spread = r * 1.6;
    for (i = 0; i < this.dice.length; i++) {
      spread = Math.max(spread, Math.hypot(this.dice[i].pos[0] - x,
                                           this.dice[i].pos[1] - y) + r * 1.4);
    }
    return { x: x, y: y, r: Math.min(spread, 130) };
  };

  // elemental starts whatever the damage does. It is called the moment the
  // dice come to rest, so it happens over a readable number rather than
  // over a blur.
  //
  // now is the frame's own timestamp rather than the wall clock. Every time
  // that matters here - when each crystal starts growing, how long a flame
  // lasts - is measured against the clock the frames are drawn on, and
  // taking the two from different places leaves the whole effect already
  // expired on its first frame.
  DiceBoard.prototype.elemental = function (element, now) {
    var look = pickLook(ELEMENTS[String(element || '').toLowerCase()]);
    var at = this.diceHeart();
    if (!look || !at) { this.elem = null; return; }
    now = now || performance.now();
    var items = [];
    var rnd = function (a, b) { return a + Math.random() * (b - a); };
    // Two ways of placing things: around the dice, and on them. Ice
    // pushes up out of the paper in a ring; fire burns on the dice
    // themselves, and putting it on a ring would set light to the table
    // around them instead.
    var ring = function (i, n, pad) {
      var a = (i / n) * Math.PI * 2 + Math.random() * 0.5;
      var d = at.r * (pad || 1) * (0.55 + Math.random() * 0.6);
      return [at.x + Math.cos(a) * d, at.y + Math.sin(a) * d * 0.75];
    };
    var dice = this.dice;
    var onDice = function (i, jitter) {
      if (!dice.length) { return [at.x, at.y]; }
      var d = dice[i % dice.length];
      var j = jitter === undefined ? 9 : jitter;
      return [d.pos[0] + rnd(-j, j), d.pos[1] + rnd(-j, j)];
    };
    var i, p;

    switch (look.kind) {
      case 'shards':
        // Crystals pushing up out of the paper in a ring around the dice,
        // each with its own facets so nine of them are nine crystals
        // rather than one drawn nine times.
        for (i = 0; i < look.n; i++) {
          p = ring(i, look.n, 1.15);
          var lean = look.lean || 0.25;
          items.push(solidOf({ what: 'spike', x: p[0], y: p[1],
            h: rnd(look.h[0], look.h[1]), r: rnd(look.r[0], look.r[1]),
            lean: [rnd(-7, 7) * lean, rnd(-7, 7) * lean],
            face: look.face, side: look.side, edge: look.edge,
            born: now + i * 42, life: look.dur, grow: 190 },
            look.bands === 'blade' ? BANDS_BLADE : BANDS_ICE, look.sides || 7,
            look.spread || 0.55, look.tone));
        }
        if (look.ring) {
          items.push({ what: 'ring', x: at.x, y: at.y, r: at.r * 1.25,
            face: look.ring, born: now, life: look.dur, grow: 420 });
        }
        break;

      case 'stones':
        // Boulders shoved up out of the ground around the dice, each with
        // its own facets, and the dust they knocked loose coming up with
        // them. A couple of smaller ones tumble out beside the big ones.
        var rs = look.sides || 9;
        for (i = 0; i < look.n; i++) {
          p = ring(i, look.n, 1.1);
          items.push(solidOf({ what: 'rock', x: p[0], y: p[1],
            h: rnd(look.h[0], look.h[1]), r: rnd(look.r[0], look.r[1]),
            lean: [rnd(-9, 9), rnd(-7, 7)],
            face: look.face, side: look.side, edge: '',
            born: now + i * 62, life: look.dur, grow: 300 },
            ROCK_BANDS, rs, look.spread, look.tone));
          items.push({ what: 'puff', x: p[0], y: p[1], z: 4, vz: rnd(20, 50),
            r: rnd(8, 17), face: look.dust,
            born: now + i * 62 + 90, life: look.dur * 0.7, grow: 0, swell: 10 });
          if (i % 2 === 0) {
            items.push(solidOf({ what: 'rock',
              x: p[0] + rnd(-22, 22), y: p[1] + rnd(-16, 16),
              h: rnd(12, 22), r: rnd(7, 13),
              lean: [rnd(-6, 6), rnd(-5, 5)],
              face: look.face, side: look.side, edge: '',
              born: now + i * 62 + 40, life: look.dur - 200, grow: 240 },
              ROCK_BANDS, rs, look.spread, look.tone));
          }
        }
        break;

      case 'blaze':
        // Flames licking up off the dice themselves, each reshaping itself
        // as it burns, with the smoke they leave climbing above them.
        //
        // A fire is not a row of equal flames. A third of them are short
        // licks that come and go in a moment, which is what fills the gaps
        // between the tall ones and stops the whole thing pulsing as one.
        for (i = 0; i < look.n; i++) {
          var lick = i % 3 === 2;
          p = onDice(i, lick ? 16 : 11);
          items.push({ what: 'flame', x: p[0], y: p[1],
            h: rnd(look.h[0], look.h[1]) * (lick ? 0.42 : 1),
            r: rnd(look.r[0], look.r[1]) * (lick ? 0.7 : 1),
            seed: Math.random() * 90,
            tip: look.hot, face: look.mid, side: look.low,
            born: now + i * 18 + (lick ? rnd(0, 300) : 0),
            life: lick ? 220 + Math.random() * 220 : 520 + Math.random() * 380,
            grow: lick ? 70 : 150 });
          if (i % 3 === 0) {
            items.push({ what: 'puff', x: p[0], y: p[1], z: 46,
              vz: rnd(45, 85), r: rnd(6, 12), face: look.smoke,
              born: now + 260 + i * 45, life: look.dur - 240, grow: 0, swell: 16 });
          }
        }
        // Embers. They come off the fire rather than out of the ground,
        // rise while they are hot and start to fall once they are not.
        for (i = 0; i < (look.embers || 24); i++) {
          p = onDice(i, 15);
          items.push({ what: 'ember', x: p[0], y: p[1], z: rnd(8, 46),
            vx: rnd(-16, 16), vy: rnd(-9, 9), vz: rnd(52, 138),
            r: rnd(0.9, 2.4), seed: Math.random() * 50,
            face: look.ember || '#ffb347',
            born: now + rnd(60, 620), life: rnd(620, 1150), grow: 0 });
        }
        break;

      case 'bolt':
        // Not one strike but a handful down the same column, a beat apart
        // and none of them the same thickness - which is what a strike
        // actually looks like, and what the single clean bolt was missing.
        //
        // Which handful depends on the variant the look picked: a forked
        // tree of a strike, a slow wide ribbon, or a whole squall of thin
        // ones walking across the page.
        var hit = onDice(0, 4);
        var spread = look.spread || 9;
        for (i = 0; i < (look.n || 5); i++) {
          var lead = i === 0;
          items.push({ what: 'bolt',
            x: hit[0] + rnd(-spread, spread),
            y: hit[1] + rnd(-spread * 0.7, spread * 0.7),
            h: 520 + rnd(-60, 60),
            // Every strike walks its own path from its own seed, so two
            // of them are never the same shape even at the same width.
            seed: rnd(0, 900),
            slant: rnd(-18, 52),
            steps: look.steps || 14,
            // The first is the fat one; the rest are the flicker after it,
            // and none of them the same thickness.
            width: lead ? rnd(look.w0 || 13, (look.w0 || 13) + 4)
                        : rnd(look.w1 || 3.5, (look.w1 || 3.5) + 5.5),
            sway: lead ? (look.sway0 || 20) : rnd(14, look.sway1 || 40),
            forks: lead ? (look.forks === undefined ? 4 : look.forks)
                        : Math.round((look.forks === undefined ? 4 : look.forks) * 0.4),
            ribbon: !!look.ribbon && lead,
            beads: !!look.beads,
            face: look.core,
            glow: (look.glows && look.glows[i % look.glows.length]) || look.glow,
            born: now + (lead ? 0 : 50 + i * rnd(40, 95)),
            life: lead ? (look.lead || 480) : rnd(190, 320), grow: 0 });
        }
        // The whole page going pale for an instant. This is most of what
        // makes a strike feel big: the bolt is small on the screen, and
        // the room changing colour round it is not.
        items.push({ what: 'flash', face: look.flash || 'rgba(224,238,255,1)',
          peak: look.peak === undefined ? 0.3 : look.peak,
          born: now, life: 220, grow: 0 });
        // The flash on the paper under it, and the scorch that outlives it.
        items.push({ what: 'pool', x: hit[0], y: hit[1], r: at.r * 1.1,
          face: 'rgba(255,248,214,.9)', born: now, life: 260, grow: 90 });
        items.push({ what: 'ring', x: at.x, y: at.y, r: at.r * 1.5,
          face: look.ring, width: 13, born: now + 60, life: 820, grow: 320 });
        for (i = 0; i < 14; i++) {
          p = ring(i, 14, 1.1);
          items.push({ what: 'puff', x: p[0], y: p[1], z: 6, vz: rnd(60, 160),
            r: rnd(3, 8), face: look.glow,
            born: now + 90 + i * 12, life: 460, grow: 0 });
        }
        // Sparks off the impact, thrown hard and low and bouncing once.
        for (i = 0; i < (look.sparks || 32); i++) {
          var sa = Math.random() * Math.PI * 2;
          var sv = rnd(90, 320);
          items.push({ what: 'spark', x: hit[0], y: hit[1], z: rnd(2, 14),
            vx: Math.cos(sa) * sv, vy: Math.sin(sa) * sv * 0.7,
            vz: rnd(40, 230), r: rnd(0.8, 2.2), seed: Math.random() * 30,
            face: look.spark || '#fff3c0',
            born: now + rnd(0, 90), life: rnd(320, 720), grow: 0 });
        }
        // And the motes left hanging in the air once it is over, which
        // is what stops the effect simply stopping.
        for (i = 0; i < 20; i++) {
          p = ring(i, 20, 1.5);
          items.push({ what: 'mote', x: p[0], y: p[1], z: rnd(10, 120),
            vz: rnd(-6, 20), r: rnd(0.7, 1.9), seed: Math.random() * 40,
            face: (look.glows && look.glows[i % look.glows.length]) || look.glow,
            born: now + rnd(120, 420), life: rnd(500, 1000), grow: 0 });
        }
        break;

      case 'gale':
        // The bang first: two rings going out across the paper, which is
        // the part of a thunderclap you can actually draw.
        items.push({ what: 'ring', x: at.x, y: at.y, r: at.r * 2.4,
          face: look.edge, width: 16, born: now, life: 620, grow: 560 });
        items.push({ what: 'ring', x: at.x, y: at.y, r: at.r * 1.5,
          face: look.edge, width: 10, born: now + 120, life: 620, grow: 480 });
        // A squall around the dice: cloud turning one way and the gusts
        // that shove through it.
        for (i = 0; i < look.n; i++) {
          var a0 = (i / look.n) * Math.PI * 2;
          items.push({ what: 'puff', x: at.x + Math.cos(a0) * at.r,
            y: at.y + Math.sin(a0) * at.r * 0.7, z: rnd(6, 40),
            vz: rnd(-6, 14), r: rnd(10, 22), face: look.cloud,
            orbit: { x: at.x, y: at.y, a: a0, d: at.r * rnd(0.7, 1.3), sp: rnd(1.6, 3.2) },
            born: now + i * 26, life: look.dur - i * 12, grow: 0, swell: 8 });
        }
        for (i = 0; i < 9; i++) {
          items.push({ what: 'gust', x: at.x - at.r * 2.2,
            y: at.y + rnd(-at.r, at.r) * 0.8, z: rnd(4, 46),
            r: rnd(30, 70), vx: rnd(420, 700), face: look.gust,
            born: now + i * 70, life: 480, grow: 0 });
        }
        break;

      case 'brew':
        // A puddle at the dice with things coming up through it.
        items.push({ what: 'pool', x: at.x, y: at.y, r: at.r * 1.15,
          face: look.pool, born: now, life: look.dur, grow: 260 });
        for (i = 0; i < look.n; i++) {
          p = ring(i, look.n, 0.95);
          items.push({ what: 'puff', x: p[0], y: p[1], z: 2,
            vz: rnd(26, 62), r: rnd(3, 8), face: look.bub,
            born: now + i * 46, life: 620, grow: 0, swell: 4 });
          if (i % 3 === 0) {
            items.push({ what: 'puff', x: p[0], y: p[1], z: 14,
              vz: rnd(34, 60), r: rnd(8, 16), face: look.vapour,
              born: now + 180 + i * 40, life: look.dur - 300, grow: 0, swell: 16 });
          }
        }
        break;

      case 'wither':
        // Nothing rises here: the dark comes down onto the dice and the
        // colour goes out of the paper under them.
        items.push({ what: 'pool', x: at.x, y: at.y, r: at.r * 1.3,
          face: look.dark, born: now, life: look.dur, grow: 420 });
        for (i = 0; i < look.n; i++) {
          p = ring(i, look.n, 1.4);
          items.push({ what: 'puff', x: p[0], y: p[1], z: rnd(60, 150),
            vz: -rnd(55, 110), r: rnd(4, 10), face: look.mote,
            born: now + i * 38, life: 760, grow: 0 });
        }
        for (i = 0; i < 6; i++) {
          p = ring(i, 6, 0.9);
          items.push(solidOf({ what: 'spike', x: p[0], y: p[1],
            h: rnd(20, 40), r: rnd(3, 6),
            lean: [rnd(-14, 14), rnd(-14, 14)],
            face: look.mote, side: look.dark, edge: '',
            born: now + 120 + i * 50, life: look.dur - 300, grow: 300 },
            BANDS_BLADE, 4, 0.5, 0.4));
        }
        break;

      case 'wave':
        // A standing wave that rises off the sheet to one side of the
        // dice, crests over them and comes down. The crest is one shape
        // drawn along a line rather than a crowd of particles, because a
        // wave is a single thing moving - the spray it throws is the
        // crowd, and that comes off the top of it as it breaks.
        items.push({ what: 'crest', x: at.x, y: at.y, r: at.r,
          h: 92 + at.r * 0.7, from: -at.r * 1.9, seed: Math.random() * 40,
          deep: look.deep, face: look.face, foam: look.foam,
          born: now, life: look.dur - 200, grow: 0 });
        // Foam spreading out on the paper where it lands.
        items.push({ what: 'ring', x: at.x, y: at.y, r: at.r * 1.7,
          face: 'rgba(234,246,255,.6)', width: 14,
          born: now + 420, life: look.dur - 400, grow: 480 });
        items.push({ what: 'pool', x: at.x, y: at.y, r: at.r * 1.25,
          face: 'rgba(74,166,216,.55)', born: now + 380, life: look.dur - 360,
          grow: 320 });
        // Droplets thrown off the lip as it breaks. These are the thing
        // you actually watch: they arc forward, land on the parchment and
        // leave a wet mark that soaks in and dries. The mist behind them
        // is only there to give them something to come out of.
        for (i = 0; i < 34; i++) {
          var thrown = Math.random() * Math.PI * 2;
          var reach = at.r * rnd(0.2, 1.9);
          items.push({ what: 'drop',
            x: at.x + Math.cos(thrown) * reach * 0.4,
            y: at.y + Math.sin(thrown) * reach * 0.3 - at.r * 0.5,
            z: rnd(52, 108),
            vx: Math.cos(thrown) * rnd(20, 105),
            vy: Math.sin(thrown) * rnd(14, 74) + 26,
            vz: rnd(18, 92), r: rnd(0.9, 2.6),
            face: i % 3 ? look.face : look.foam,
            wet: 'rgba(86,142,186,.42)',
            born: now + 300 + i * 11, life: 1500, grow: 0 });
        }
        for (i = 0; i < look.n; i++) {
          p = ring(i, look.n, 1.0);
          items.push({ what: 'puff', x: p[0], y: p[1], z: rnd(30, 80),
            vz: rnd(40, 110), r: rnd(3, 8),
            face: i % 3 ? look.foam : look.face,
            born: now + 330 + i * 16, life: 620, grow: 0, swell: 5 });
          if (i % 2 === 0) {
            items.push({ what: 'drop', x: p[0] + rnd(-20, 20), y: p[1] + rnd(-14, 14),
              z: rnd(40, 95), vz: rnd(30, 70), r: rnd(1.6, 3.6),
              face: look.foam,
              born: now + 380 + i * 22, life: 700, grow: 0 });
          }
        }
        break;

      case 'boom':
        // The tint comes from what the spell actually does. Fire is the
        // default because most things that go bang are on fire.
        var tint = BOOM_TINTS[String(this.tint || '').toLowerCase()] || BOOM_TINTS.fire;
        var spikes = [], rays = [], bumps;
        for (i = 0; i < look.spikes; i++) {
          // Alternating long and short, each with its own wobble - a ring
          // of identical points is a cog, not a bang.
          spikes.push((i % 2 ? 1 : 0.58) * (1 - look.rough / 2 + Math.random() * look.rough));
        }
        for (i = 0; i < look.rays; i++) {
          rays.push({ a: Math.random() * Math.PI * 2, l: rnd(0.35, 1.5),
                      w: rnd(1.5, 5), o: Math.random() });
        }
        items.push({ what: 'boom', x: at.x, y: at.y, z: at.r * 0.35,
          r: at.r * 1.9, spikes: spikes, rays: rays,
          spin: Math.random() * Math.PI * 2,
          outer: tint.outer, mid: tint.mid, core: tint.core, edge: tint.edge,
          born: now, life: 720, grow: 0 });
        // The cauliflower of smoke, rolling up and outwards behind it.
        for (i = 0; i < look.lobes; i++) {
          var la = (i / look.lobes) * Math.PI * 2 + rnd(-0.3, 0.3);
          bumps = [];
          for (var bq = 0; bq < 6; bq++) { bumps.push(rnd(0.34, 0.62)); }
          items.push({ what: 'lobe',
            x: at.x + Math.cos(la) * at.r * 1.2,
            y: at.y + Math.sin(la) * at.r * 0.9,
            z: rnd(10, 50), vz: rnd(22, 60),
            r: rnd(at.r * 0.4, at.r * 0.8), bumps: bumps,
            spin: Math.random() * Math.PI * 2,
            face: tint.smoke, lit: tint.mid,
            born: now + 120 + i * 42, life: look.dur - 200, grow: 420 });
        }
        items.push({ what: 'ring', x: at.x, y: at.y, r: at.r * 2.6,
          face: tint.wave, width: 18, born: now + 40, life: 620, grow: 420 });
        for (i = 0; i < 20; i++) {
          p = ring(i, 20, 1.3);
          items.push({ what: 'drop', x: p[0], y: p[1], z: rnd(20, 70),
            vz: rnd(70, 190), r: rnd(1.5, 4), face: tint.core,
            born: now + 60 + i * 11, life: 620, grow: 0 });
        }
        break;

      case 'missile':
        // They come in from off the sheet, from different directions and
        // a beat apart, and each one ends on the dice.
        for (i = 0; i < look.n; i++) {
          var ma = Math.random() * Math.PI * 2;
          var far = 420 + Math.random() * 260;
          var hitAt = onDice(i, 10);
          items.push({ what: 'missile',
            x0: at.x + Math.cos(ma) * far, y0: at.y + Math.sin(ma) * far * 0.7,
            z0: 180 + Math.random() * 220,
            x1: hitAt[0], y1: hitAt[1], z1: 10,
            bend: [rnd(-150, 150), rnd(-110, 110)], lift: rnd(30, 130),
            w: rnd(look.w[0], look.w[1]), trail: 0.3 + Math.random() * 0.25,
            seed: Math.random() * 60,
            // The look sets the character of the volley; each missile in
            // it still rolls its own head, its own number of spirals and
            // its own grit, so a volley of five is five projectiles
            // rather than one drawn five times.
            head: Math.random() < 0.7 ? look.head
              : MISSILE_HEADS[Math.floor(Math.random() * MISSILE_HEADS.length)],
            spirals: Math.max(0, (look.spirals || 0) +
                               (Math.random() < 0.35 ? 1 : 0)),
            twist: (look.twist || 8) * (0.75 + Math.random() * 0.7),
            chevrons: look.chevrons && Math.random() < 0.85,
            burns: !!look.burns,
            grit: (look.grit === undefined ? 0.5 : look.grit) *
                  (0.7 + Math.random() * 0.6),
            wander: (look.wander || 10) * (0.5 + Math.random()),
            churn: look.churn === undefined ? 1 : look.churn,
            shed: look.shed || 0, shedN: look.shedN || 2,
            face: look.face, glow: look.glow, core: look.core,
            born: now + i * look.stagger, life: 560, grow: 0 });
          // What it does when it lands.
          items.push({ what: 'ring', x: hitAt[0], y: hitAt[1], r: at.r * 0.85,
            face: look.burst, width: 9,
            born: now + i * look.stagger + 420, life: 420, grow: 240 });
          for (var q = 0; q < 8; q++) {
            items.push({ what: 'puff', x: hitAt[0], y: hitAt[1], z: 6,
              vz: rnd(50, 150), r: rnd(2, 5), face: look.core,
              born: now + i * look.stagger + 410 + q * 9, life: 320, grow: 0 });
          }
        }
        break;

      case 'circle':
        // One circle, turning, with motes rising off it. Which of the
        // four it is, is picked here rather than drawn here - see
        // CIRCLE_STYLES.
        var st = CIRCLE_STYLES[Math.floor(Math.random() * CIRCLE_STYLES.length)];
        items.push({ what: 'circle', x: at.x, y: at.y,
          r: at.r * 1.9, style: st, seed: Math.floor(Math.random() * 97),
          born: now, life: look.dur, grow: 520 });
        for (i = 0; i < 14; i++) {
          p = ring(i, 14, 1.5);
          items.push({ what: 'puff', x: p[0], y: p[1], z: rnd(0, 20),
            vz: rnd(24, 70), r: rnd(2, 5),
            face: 'rgba(' + st.hot + ',1)',
            born: now + 220 + i * 52, life: look.dur - 400, grow: 0 });
        }
        break;

      case 'mend':
        // A ring of sigils around the dice, turning one way and opening
        // out as it turns, and the crosses rising up through the middle
        // of it.
        for (i = 0; i < look.n; i++) {
          var sa = (i / look.n) * Math.PI * 2;
          items.push({ what: 'sigil', x: 0, y: 0, z: rnd(6, 26),
            r: rnd(9, 15), spin: Math.random() * Math.PI,
            orbit: { x: at.x, y: at.y, a: sa, d: at.r * 1.05, sp: 0.9 },
            open: at.r * 0.5,
            face: look.ring, glow: look.glow,
            born: now + i * 34, life: look.dur - 200, grow: 320 });
        }
        items.push({ what: 'ring', x: at.x, y: at.y, r: at.r * 1.25,
          face: 'rgba(143,230,180,.55)', width: 11,
          born: now, life: look.dur - 150, grow: 420 });
        for (i = 0; i < look.plus; i++) {
          p = ring(i, look.plus, 0.8);
          items.push({ what: 'plus', x: p[0], y: p[1], z: rnd(0, 30),
            vz: rnd(55, 120), r: rnd(5, 13),
            face: look.mark, glow: look.glow,
            born: now + 120 + i * 78, life: look.dur - 300, grow: 0 });
        }
        break;

      case 'bind':
        // Ropes thrown over the dice from several sides, each pulling
        // tight a beat after the last.
        for (i = 0; i < look.n; i++) {
          items.push({ what: 'rope', x: at.x + rnd(-10, 10), y: at.y + rnd(-8, 8),
            // Fanned right round, so they cross over the top of the dice
            // from every side rather than all lying the same way.
            angle: (i / look.n) * Math.PI + rnd(-0.18, 0.18),
            span: at.r * rnd(1.5, 2.2), lift: at.r * rnd(0.55, 1.35),
            bow: at.r * rnd(-0.6, 0.6),
            w: rnd(look.w[0], look.w[1]), seed: Math.random() * 40,
            lay: look.lay,
            rope: look.rope, glow: look.glow, dark: look.dark,
            born: now + i * 85, life: look.dur - i * 40, grow: 0 });
        }
        items.push({ what: 'ring', x: at.x, y: at.y, r: at.r * 1.2,
          face: look.lay === 'leaves' ? 'rgba(120,190,80,.55)'
                : look.lay === 'links' ? 'rgba(185,191,201,.5)'
                : 'rgba(216,180,106,.55)',
          width: 10, born: now + 200, life: look.dur - 300, grow: 300 });
        break;

      case 'slash':
        // One swing, or two crossed, or a flurry of three. The arc is
        // struck through the dice rather than round them: the centre of
        // the circle is well off to one side, so the part of it that
        // crosses the page is nearly straight and reads as a cut.
        for (i = 0; i < (look.n || 1); i++) {
          var swung = Math.random() * Math.PI * 2;
          var far = at.r * (2.6 + Math.random() * 1.6);
          // The centre sits out along a random bearing, and the arc is
          // aimed back at the dice from there.
          var cx = at.x + Math.cos(swung) * far;
          var cy = at.y + Math.sin(swung) * far * 0.8;
          var back = Math.atan2(at.y - cy, at.x - cx);
          var sweep = (0.8 + Math.random() * 0.5) * (Math.random() < 0.5 ? -1 : 1);
          var slash = { what: 'slash', x: cx, y: cy,
            r: Math.hypot(at.x - cx, at.y - cy),
            w: at.r * 0.8 * (look.wide || 1) * (0.8 + Math.random() * 0.5),
            a0: back - sweep / 2, sweep: sweep,
            squash: 0.78 + Math.random() * 0.3,
            core: look.core, face: look.face, glow: look.glow, ink: look.ink,
            born: now + i * 165, life: 900, grow: 0 };
          items.push(slash);
          // The cut it leaves, on the same arc and outliving the light.
          items.push({ what: 'scar', x: cx, y: cy, r: slash.r,
            a0: slash.a0, sweep: sweep, squash: slash.squash,
            face: look.face, ink: look.ink,
            born: now + i * 165 + 90, life: look.dur - 200, grow: 60 });
          // Sparks thrown off where the blade crosses the dice.
          for (var sk = 0; sk < 16; sk++) {
            var sang = Math.random() * Math.PI * 2;
            var ssp = 70 + Math.random() * 240;
            items.push({ what: 'spark',
              x: at.x + rnd(-at.r * 0.5, at.r * 0.5),
              y: at.y + rnd(-at.r * 0.4, at.r * 0.4), z: rnd(4, 26),
              vx: Math.cos(sang) * ssp, vy: Math.sin(sang) * ssp * 0.7,
              vz: rnd(30, 170), r: rnd(0.7, 1.9), seed: Math.random() * 30,
              face: look.glow,
              born: now + i * 165 + rnd(40, 130), life: rnd(260, 560), grow: 0 });
          }
        }
        break;

      case 'hexring':
        items.push({ what: 'hexring', x: at.x, y: at.y, r: at.r * 4.2,
          cell: look.cell, spin: look.spin, seed: Math.random() * 40,
          face: look.face, edge: look.edge, rim: look.rim,
          born: now, life: look.dur, grow: 0 });
        for (i = 0; i < 18; i++) {
          p = ring(i, 18, 1.4);
          items.push({ what: 'mote', x: p[0], y: p[1], z: rnd(4, 70),
            vz: rnd(20, 70), r: rnd(0.7, 1.9), seed: Math.random() * 40,
            face: look.edge,
            born: now + rnd(60, 500), life: rnd(360, 700), grow: 0 });
        }
        break;

      case 'panther':
        // One cat, out of one hole, coming towards the bottom of the
        // screen because that is where the person reading this is.
        items.push({ what: 'panther', x: at.x, y: at.y + at.r * 0.2, z: 0,
          r: at.r * 0.85, h: at.r * 2.4,
          away: [rnd(-70, 70), at.r * 3.5 + rnd(0, 90)],
          // Yawed most of the way round, so it is coming out of the
          // page at you rather than running across it.
          // Three-quarters on, one way or the other.
          yaw: (Math.random() < 0.5 ? -0.75 : 0.75) + rnd(-0.25, 0.25),
          roll: rnd(-0.18, 0.18),
          pose: -1, mesh: catMesh(0),
          look: { mode: 'wire', fill: look.fill, edge: look.edge,
                  glow: look.glow, node: look.node, nodes: true },
          born: now + 180, life: look.dur - 200, grow: 1 });
        // The paper it came through, and what that threw up.
        items.push({ what: 'burst', x: at.x, y: at.y + at.r * 0.2,
          r: at.r * 1.5, angle: Math.PI / 2, seed: Math.random() * 40,
          glow: look.edge, born: now, life: look.dur, grow: 180 });
        for (i = 0; i < 22; i++) {
          var pa = Math.random() * Math.PI * 2;
          items.push({ what: 'spark', x: at.x, y: at.y + at.r * 0.2, z: 4,
            vx: Math.cos(pa) * rnd(50, 200), vy: Math.sin(pa) * rnd(30, 130),
            vz: rnd(40, 200), r: rnd(0.8, 2.4), seed: Math.random() * 30,
            face: look.edge,
            born: now + 120 + rnd(0, 120), life: rnd(320, 640), grow: 0 });
        }
        items.push({ what: 'ring', x: at.x, y: at.y + at.r * 0.2, r: at.r * 1.4,
          face: 'rgba(160,140,255,.5)', width: 12,
          born: now + 150, life: 720, grow: 300 });
        break;

      case 'thorns':
        // Thorns through the paper in a thicket, each with the hole it
        // tore on the way up, and grass filling the gaps between them.
        for (i = 0; i < look.n; i++) {
          p = ring(i, look.n, 1.5);
          var th = rnd(look.h[0], look.h[1]);
          var tr = rnd(look.r[0], look.r[1]);
          items.push({ what: 'thorn', x: p[0], y: p[1],
            h: th, r: tr, aim: Math.random() * Math.PI * 2,
            lean: rnd(6, 26), barbs: 2 + Math.floor(Math.random() * 3),
            seed: Math.random() * 40,
            face: look.face, tip: look.tip, dark: look.dark,
            born: now + i * 38, life: look.dur - i * 24, grow: 340 });
          items.push({ what: 'burst', x: p[0], y: p[1], r: tr * 2.2,
            angle: Math.random() * Math.PI * 2, seed: Math.random() * 40,
            glow: look.tip, born: now + i * 38, life: look.dur - i * 24,
            grow: 120 });
        }
        // Grass and creeping vine between them.
        for (i = 0; i < look.tufts; i++) {
          p = ring(i, look.tufts, 1.7);
          items.push({ what: 'tuft', x: p[0], y: p[1],
            h: rnd(16, 40), w: rnd(1.4, 2.8),
            blades: 4 + Math.floor(Math.random() * 4),
            aim: Math.random() * Math.PI * 2, seed: Math.random() * 40,
            face: look.grass, dark: look.grassDark,
            born: now + 120 + i * 34, life: look.dur - 300, grow: 420 });
        }
        items.push({ what: 'ring', x: at.x, y: at.y, r: at.r * 1.8,
          face: 'rgba(110,150,70,.45)', width: 13,
          born: now + 200, life: look.dur - 400, grow: 340 });
        break;

      case 'shield':
        // The shield comes up, and the blow that was coming stops on it.
        // A shield is tall and the dice land wherever they land, so
        // the spot is pulled back inside a box big enough to hold the
        // whole board - otherwise half of it is off the window and
        // the flourish is one nobody sees.
        var shr = at.r * 1.25;
        var shx = Math.min(Math.max(at.x + rnd(-at.r * 0.3, at.r * 0.3),
                                    shr * 1.1), this.w - shr * 1.1);
        var shy = Math.min(Math.max(at.y - at.r * 0.25, shr * 1.5),
                           this.h - shr * 0.7);
        items.push({ what: 'shield', x: shx, y: shy, z: at.r * 0.5,
          r: at.r * 1.25, shape: look.shape, spins: look.spins,
          shape: look.shape, glow: look.glow,
          mesh: shieldMesh(look.shape, 0.28 + Math.random() * 0.22),
          look: { mode: 'wire', fill: look.fill, edge: look.rim,
                  glow: look.glow, node: look.node, nodes: true,
                  twoSided: true },
          seed: Math.floor(Math.random() * 97),
          born: now, life: look.dur, grow: 1 });
        // The swing, arriving a beat after the shield is up and stopping
        // dead on it. It is the same crescent the weapons use, told
        // where to stop.
        var swung2 = Math.random() * Math.PI * 2;
        var far3 = at.r * (2.6 + Math.random() * 1.2);
        var cx2 = shx + Math.cos(swung2) * far3;
        var cy2 = shy + Math.sin(swung2) * far3 * 0.8;
        var back2 = Math.atan2(shy - cy2, shx - cx2);
        var sweep2 = (0.8 + Math.random() * 0.4) * (Math.random() < 0.5 ? -1 : 1);
        items.push({ what: 'slash', x: cx2, y: cy2,
          r: Math.hypot(shx - cx2, shy - cy2), w: at.r * 0.7,
          a0: back2 - sweep2 / 2, sweep: sweep2,
          squash: 0.8 + Math.random() * 0.25,
          // Stopped halfway along its arc, which is where the shield is.
          stopAt: 0.5,
          core: '#ffffff', face: '#ffe9b0', glow: '#ff9a3c',
          ink: 'rgba(120,80,30,.7)',
          born: now + 420, life: 900, grow: 0 });
        // And what a blade stopping on a shield throws off.
        for (i = 0; i < 40; i++) {
          var ka = Math.random() * Math.PI * 2;
          items.push({ what: 'spark', x: shx, y: shy, z: at.r * 0.5,
            vx: Math.cos(ka) * rnd(80, 330), vy: Math.sin(ka) * rnd(50, 200),
            vz: rnd(20, 240), r: rnd(0.7, 2.1), seed: Math.random() * 30,
            face: i % 3 ? '#fff0b0' : look.glow,
            born: now + 600 + rnd(0, 70), life: rnd(280, 620), grow: 0 });
        }
        for (i = 0; i < 14; i++) {
          items.push({ what: 'ember', x: shx + rnd(-16, 16),
            y: shy + rnd(-12, 12), z: at.r * 0.5,
            vx: rnd(-40, 40), vy: rnd(-26, 26), vz: rnd(-30, 50),
            r: rnd(0.9, 2.2), seed: Math.random() * 50, face: '#ffb347',
            born: now + 610 + rnd(0, 120), life: rnd(420, 820), grow: 0 });
        }
        items.push({ what: 'flash', face: 'rgba(255,226,150,1)', peak: 0.12,
          born: now + 600, life: 200, grow: 0 });
        break;

      case 'dome':
        // The panels are worked out once here and never again: the
        // dome is a fixed thing and only the light on it changes.
        var dr = at.r * 2.0;
        items.push({ what: 'dome', x: at.x, y: at.y, r: dr,
          h: dr * 0.9 * look.tall,
          panels: domePanels(look.style, look.rings, look.base2),
          ribs: look.ribs, hoops: look.hoops, runes: look.runes,
          spin: look.spin, climb: look.climb, motion: look.motion,
          churn: look.churn, weight: look.weight, nodes: look.nodes,
          ribAlpha: look.ribAlpha, ribWeight: look.ribWeight,
          base: look.base, flick: look.flick,
          turn: look.turn, bloom: look.bloom,
          seed: Math.random() * 40,
          face: look.face, seam: look.seam, edge: look.edge,
          node: look.node, rim: look.rim,
          born: now, life: look.dur, grow: 520 });
        for (i = 0; i < 14; i++) {
          p = ring(i, 14, 2.1);
          items.push({ what: 'mote', x: p[0], y: p[1], z: rnd(10, 110),
            vz: rnd(4, 26), r: rnd(0.6, 1.7), seed: Math.random() * 40,
            face: look.edge,
            born: now + rnd(200, 1400), life: rnd(400, 800), grow: 0 });
        }
        break;

      case 'cairn':
        // Pieces come up through the paper all over the sheet and fly
        // in. Where they start is deliberately nowhere near the dice:
        // the point of it is that the character's own sheet comes apart
        // and puts itself back together as something else.
        var rocks = [];
        for (i = 0; i < CAIRN_SLOTS.length; i++) {
          var fx = rnd(this.w * 0.06, this.w * 0.94);
          var fy = rnd(this.h * 0.08, this.h * 0.92);
          rocks.push({ from: [fx, fy], arc: rnd(40, 170),
            // A tumble in all three axes on the way in, and its own
            // resting attitude when it lands, so no two stones in the
            // stack sit the same way up.
            spin: [rnd(-7, 7), rnd(-7, 7), rnd(-7, 7)],
            rest: [0, rnd(-0.5, 0.5), rnd(-0.3, 0.3)],
            delay: Math.random(),
            // A real mesh each. Bark is rougher and flatter than
            // stone and splits into more facets.
            mesh: rockMesh(Math.random() * 60,
                           look.bark ? 0.68 : 0.46,
                           look.bark ? [0.5, 1.15] : [0.82, 0.78],
                           look.bark) });
          // The hole it came out of, back where it started.
          items.push({ what: 'burst', x: fx, y: fy, r: rnd(11, 22),
            angle: Math.random() * Math.PI * 2, seed: Math.random() * 40,
            glow: look.lit, born: now + i * 26, life: look.dur - 400,
            grow: 140 });
          items.push({ what: 'puff', x: fx, y: fy, z: 4, vz: rnd(20, 70),
            r: rnd(5, 12), face: look.lit,
            born: now + i * 26, life: 560, grow: 0, swell: 8 });
        }
        items.push({ what: 'cairn', x: at.x, y: at.y + at.r * 0.2,
          r: at.r * 1.15, rocks: rocks, bark: look.bark,
          look: { mode: 'solid', shade: look.shade, mid: look.mid,
                  lit: look.tone },
          born: now, life: look.dur, grow: 1 });
        items.push({ what: 'ring', x: at.x, y: at.y + at.r * 0.2, r: at.r * 1.3,
          face: look.bark ? 'rgba(150,110,60,.5)' : 'rgba(150,146,130,.5)',
          width: 12, born: now + 900, life: look.dur - 1000, grow: 280 });
        break;

      case 'glass':
        // A glass held over the dice, and what it does depends on what
        // the dice said. A bad roll breaks it; a good one lights it up.
        var nat = this.quality || 0;
        var bad = nat > 0 && nat <= 5;
        var good = nat >= 16;
        items.push({ what: 'glass', x: at.x - at.r * 0.25,
          y: at.y - at.r * 0.35, z: at.r * 0.5,
          r: at.r * 0.78, lean: rnd(-0.28, 0.18), seed: Math.random() * 40,
          rim: look.rim, rimLit: look.rimLit, rimDark: look.rimDark,
          wood: look.wood, woodLit: look.woodLit, woodDark: look.woodDark,
          glow: look.glow,
          cracked: bad, crackAt: 0.55, good: good,
          born: now, life: look.dur, grow: 300 });
        if (good) {
          // Sparkles round the rim, because something was noticed.
          for (i = 0; i < 18; i++) {
            p = ring(i, 18, 1.35);
            items.push({ what: 'mote', x: p[0], y: p[1],
              z: at.r * 0.5 + rnd(-24, 40),
              vz: rnd(6, 34), r: rnd(0.7, 2.1), seed: Math.random() * 40,
              face: i % 3 ? '#fff6d0' : '#bfe4ff',
              born: now + rnd(320, 1000), life: rnd(420, 760), grow: 0 });
          }
        }
        if (bad) {
          // And a few splinters off the break.
          for (i = 0; i < 14; i++) {
            var ga = Math.random() * Math.PI * 2;
            items.push({ what: 'spark', x: at.x - at.r * 0.25,
              y: at.y - at.r * 0.35, z: at.r * 0.5,
              vx: Math.cos(ga) * rnd(30, 130), vy: Math.sin(ga) * rnd(20, 90),
              vz: rnd(-10, 70), r: rnd(0.6, 1.8), seed: Math.random() * 30,
              face: '#dbeaf8',
              born: now + 560 + rnd(0, 90), life: rnd(400, 760), grow: 0 });
          }
        }
        break;

      case 'bloom':
        // Stems up out of the paper round the dice, each with a flower
        // of its own on the end of it.
        var many = 6 + Math.floor(Math.random() * 4);
        for (i = 0; i < many; i++) {
          p = ring(i, many, 1.25);
          var fl2 = look.blooms[Math.floor(Math.random() * look.blooms.length)];
          var gr = look.greens[Math.floor(Math.random() * look.greens.length)];
          var kind = look.stems[Math.floor(Math.random() * look.stems.length)];
          items.push({ what: 'bloom', x: p[0], y: p[1],
            h: at.r * rnd(1.0, 2.3), w: rnd(2.2, 4.4),
            bend: rnd(-22, 22), fr: rnd(12, 24),
            leaves: 2 + Math.floor(Math.random() * 3),
            stem: kind,
            flower: look.forms[Math.floor(Math.random() * look.forms.length)],
            spin: Math.random() * Math.PI * 2, seed: Math.random() * 40,
            stemCol: gr.stem, leafCol: gr.leaf, dark: gr.dark,
            petal: fl2.petal, petal2: fl2.petal2, heart: fl2.heart,
            born: now + i * 90, life: look.dur - i * 60, grow: 620 });
        }
        // The ground going green under them.
        items.push({ what: 'ring', x: at.x, y: at.y, r: at.r * 1.5,
          face: 'rgba(122,182,86,.5)', width: 12,
          born: now + 120, life: look.dur - 300, grow: 320 });
        // And pollen in the air once they are out.
        for (i = 0; i < 16; i++) {
          p = ring(i, 16, 1.4);
          items.push({ what: 'mote', x: p[0], y: p[1], z: rnd(20, 90),
            vz: rnd(4, 22), r: rnd(0.7, 1.9), seed: Math.random() * 40,
            face: '#ffe8a0',
            born: now + rnd(700, 1400), life: rnd(400, 700), grow: 0 });
        }
        break;

      case 'tome':
        // One book, shut, that opens.
        var tseed = Math.floor(Math.random() * 97);
        items.push({ what: 'tome', x: at.x, y: at.y - at.r * 0.35,
          z: at.r * 0.5, r: at.r * 1.5, seed: tseed,
          // The foxing and the water stains are fixed when the book is
          // conjured, so they stay where they are instead of crawling
          // about the page while you read it.
          stains: tomeStains(tseed + 1),
          cover: look.cover, coverDark: look.coverDark,
          page: look.page, pageDeep: look.pageDeep, pageEdge: look.pageEdge,
          pageLine: look.pageLine, gutter: look.gutter,
          stain: look.stain, fox: look.fox, thumb: look.thumb,
          ribbon: look.ribbon, ribbonDark: look.ribbonDark, rune: look.rune,
          born: now, life: look.dur, grow: 340 });
        // Something coming off the page once it is open.
        for (i = 0; i < 20; i++) {
          p = ring(i, 20, 0.9);
          items.push({ what: 'mote', x: p[0], y: p[1],
            z: at.r * 0.6 + rnd(0, 50), vz: rnd(10, 46),
            r: rnd(0.7, 2.0), seed: Math.random() * 40,
            face: i % 4 ? '#ffe9b0' : look.rune,
            born: now + 900 + rnd(0, 700), life: rnd(400, 760), grow: 0 });
        }
        break;

      case 'notes':
        // A bar of music going up off the page.
        for (i = 0; i < 9; i++) {
          var nh = look.hues[Math.floor(Math.random() * look.hues.length)];
          p = ring(i, 9, 1.0);
          items.push({ what: 'note', x: p[0], y: p[1], z: rnd(0, 30),
            vz: rnd(48, 118), r: rnd(11, 22),
            kind: ['plain', 'flag', 'flag', 'beam'][Math.floor(Math.random() * 4)],
            tilt: rnd(-0.6, 0.6), seed: Math.random() * 40,
            // Music you can see through. Every one its own colour, its
            // own weight and its own faintness - a bar of nine notes at
            // the same opacity reads as stickers, and these are meant
            // to be barely there.
            wisp: 0.24 + Math.random() * 0.4,
            face: 'hsla(' + nh + ',' + Math.round(55 + Math.random() * 35) +
                  '%,' + Math.round(48 + Math.random() * 22) + '%,' +
                  (0.3 + Math.random() * 0.3).toFixed(2) + ')',
            glow: 'hsla(' + nh + ',95%,72%,' +
                  (0.16 + Math.random() * 0.2).toFixed(2) + ')',
            born: now + i * 72 + rnd(0, 180),
            life: 900 + Math.random() * 700, grow: 420 });
        }
        break;

      case 'hand':
        // One hand, a colour it has not been before, and something to do
        // with itself. Everything about it is chosen here and fixed, so
        // it holds its gesture rather than flickering between them.
        var hue = HAND_HUES[Math.floor(Math.random() * HAND_HUES.length)];
        var hsz = Math.max(38, at.r * 1.15);
        // A spell that rolls no dice is put down somewhere on the page
        // at random, and a hand is tall: dropped near an edge, most of
        // it ends up outside the window, which is a flourish nobody
        // sees. So the spot is pulled back inside a box big enough to
        // hold the whole hand - it reaches about six tenths of its size
        // either side and getting on for two sizes above its wrist.
        var hx = Math.min(Math.max(at.x, hsz * 0.9), this.w - hsz * 0.9);
        var hy = Math.min(Math.max(at.y - at.r * 0.2, hsz * 2.1),
                          this.h - hsz * 0.4);
        var ges = Math.floor(Math.random() * HAND_GESTURES.length);
        items.push({ what: 'hand', x: hx, y: hy, z: 0,
          size: hsz * 0.62, hue: hue, gesture: ges, mesh: handMesh(ges),
          look: { mode: 'wire',
                  fill: 'hsl(' + hue + ',70%,32%)',
                  edge: 'hsl(' + hue + ',95%,72%)',
                  glow: 'hsl(' + hue + ',90%,58%)',
                  node: 'hsl(' + hue + ',100%,88%)', nodes: true },
          born: now, life: look.dur, grow: 340 });
        // Wisps coming off it, so it has an edge that is not a line.
        for (i = 0; i < 14; i++) {
          var wa = (i / 14) * Math.PI * 2 + Math.random() * 0.5;
          var wd = at.r * (0.5 + Math.random() * 0.7);
          items.push({ what: 'mote',
            x: hx + Math.cos(wa) * wd, y: hy + Math.sin(wa) * wd * 0.75,
            z: rnd(4, 90),
            vz: rnd(8, 40), r: rnd(0.8, 2.2), seed: Math.random() * 40,
            face: 'hsl(' + (hue + (i % 3) * 7) + ',90%,70%)',
            born: now + rnd(80, 700), life: rnd(500, 900), grow: 0 });
        }
        break;

      case 'blizzard':
        // A hurricane of sleet turning around the dice. Everything here
        // is one particle type on an orbit: the shape of the storm is
        // the distribution of the orbits rather than anything drawn.
        //
        // The funnel is narrow at the bottom and opens out towards the
        // top, and a particle's angular speed goes up as its radius
        // comes down, which is the one detail that makes a crowd of dots
        // read as a vortex instead of as a swarm.
        var spin = look.spin || 5.4;
        var tight = look.tight || 1;
        for (i = 0; i < look.n; i++) {
          var up = Math.random();
          // Cubed, so most of them are down near the paper where the
          // storm is thickest and only a few are up in the thin air.
          var zz = Math.pow(up, 0.55) * 230;
          var rad = at.r * tight * (0.2 + Math.pow(up, 0.8) * 1.5) *
                    (0.55 + Math.random() * 0.8);
          var hard = Math.random() < (look.shards === undefined ? 0.5 : look.shards);
          items.push({ what: 'sleet',
            orbit: { x: at.x, y: at.y, a: Math.random() * Math.PI * 2,
                     d: rad, sp: spin * (1.1 - up * 0.45) *
                               (0.75 + Math.random() * 0.5) },
            z: zz, fall: (34 + Math.random() * 120) * (look.fall || 0.85),
            r: hard ? 0.8 + Math.random() * 1.3 : 1.4 + Math.random() * 2.4,
            hard: hard, seed: Math.random() * 40,
            face: hard ? look.shard : look.flake,
            mark: look.mark, born: now + Math.random() * 380,
            life: 520 + Math.random() * 760, grow: 0 });
        }
        // The air in it: pale arcs turning with the storm, which is what
        // fills the gaps the particles leave.
        for (i = 0; i < 16; i++) {
          items.push({ what: 'swirl',
            orbit: { x: at.x, y: at.y, a: Math.random() * Math.PI * 2,
                     d: at.r * tight * (0.5 + Math.random() * 1.5),
                     sp: spin * (0.5 + Math.random() * 0.4) },
            z: Math.random() * 190, r: 24 + Math.random() * 60,
            face: look.haze, born: now + i * 26,
            life: look.dur - 300, grow: 160 });
        }
        // The gloom the storm casts on the page. Drawn first and lying
        // flat, so everything else in the storm has something darker
        // than parchment to be pale against.
        items.push({ what: 'pool', x: at.x, y: at.y, r: at.r * 2.6,
          face: look.gloom || 'rgba(96,124,156,.34)',
          born: now, life: look.dur, grow: 300 });
        // Slush on the paper under it, spreading as it settles.
        items.push({ what: 'ring', x: at.x, y: at.y, r: at.r * 1.9,
          face: look.rime, width: 16, born: now + 140,
          life: look.dur - 200, grow: 420 });
        items.push({ what: 'pool', x: at.x, y: at.y, r: at.r * 1.5,
          face: 'rgba(214,236,248,.5)', born: now + 220,
          life: look.dur - 260, grow: 340 });
        break;

      case 'shaft':
        // A column of light standing on the dice, with motes drifting up
        // inside it.
        items.push({ what: 'beam', x: at.x, y: at.y, r: at.r * 0.62, h: 430,
          face: look.beam, born: now, life: look.dur, grow: 220 });
        items.push({ what: 'ring', x: at.x, y: at.y, r: at.r * 1.2,
          face: 'rgba(' + look.beam + ',.55)', born: now, life: look.dur, grow: 340 });
        for (i = 0; i < 16; i++) {
          p = ring(i, 16, 0.8);
          items.push({ what: 'puff', x: p[0], y: p[1], z: rnd(0, 40),
            vz: rnd(30, 90), r: rnd(2, 6), face: look.mote,
            born: now + i * 34, life: look.dur - 200, grow: 0 });
        }
        break;
    }

    this.elem = { look: look, at: at, t0: now, last: now,
                  until: now + look.dur + 260, items: items };
    // The sound goes with the picture, not with the roll: a flourish the
    // page decided not to show is a flourish nobody should hear either.
    elemVoice(look.kind, look);
  };

  DiceBoard.prototype.elemBusy = function (now) {
    return !!(this.elem && now < this.elem.until);
  };

  // elsewhere runs an element on its own, with no dice under it, somewhere
  // on the page. A spell that rolls nothing still happened, and watching a
  // wave break over an empty patch of parchment is better than watching
  // nothing at all.
  DiceBoard.prototype.elsewhere = function (element, at, tint) {
    if (!ELEMENTS[String(element || '').toLowerCase()] || !at) { return; }
    this.stop();
    this.tint = tint || '';
    this.resize();
    this.scrollY0 = window.pageYOffset || document.documentElement.scrollTop || 0;
    this.table = this.tableBounds();
    var now = performance.now();
    this.heart = { x: at.x, y: at.y, r: 88 };
    this.elemental(element, now);
    this.phase = 'elem';
    this.phaseAt = now;
    this.t0 = now;
    this.last = now;
    this.runLoop();
  };

  // runLoop is the frame pump, which the dice and a standing element both
  // want and which only one of them can own at a time.
  DiceBoard.prototype.runLoop = function () {
    if (this.raf) { return; }
    var self = this;
    var tick = function (now) {
      self.raf = requestAnimationFrame(tick);
      self.step(now);
    };
    this.raf = requestAnimationFrame(tick);
  };

  // A gale actually pushes the dice about, which is the only element that
  // touches them rather than happening around them.
  DiceBoard.prototype.elemShove = function (now) {
    var e = this.elem;
    if (!e || e.look.kind !== 'gale') { return; }
    var t = (now - e.t0) / e.look.dur;
    if (t < 0 || t > 1) { return; }
    var push = Math.sin(t * Math.PI) * 0.9;
    for (var i = 0; i < this.dice.length; i++) {
      var d = this.dice[i];
      d.pos[0] += push * (1.4 + Math.sin(now / 90 + i) * 0.9);
      d.pos[1] += push * Math.sin(now / 130 + i * 2) * 0.7;
    }
  };

  DiceBoard.prototype.stepElem = function (ctx, now) {
    var e = this.elem;
    if (!e) { return; }
    if (now > e.until) { this.elem = null; return; }
    var dt = Math.min(0.032, (now - e.last) / 1000);
    e.last = now;
    // The last quarter second is spent going away, so nothing vanishes
    // between one frame and the next.
    var left = e.until - now;
    var fade = left < 300 ? Math.max(0, left / 300) : 1;
    var self = this;

    ctx.save();
    // Anything lying on the paper goes down before anything standing on it.
    var flat = [], up = [];
    e.items.forEach(function (it) {
      (it.what === 'pool' || it.what === 'ring' || it.what === 'spot' ||
       it.what === 'flash' || it.what === 'scar' || it.what === 'burst'
        ? flat : up).push(it);
    });
    up.sort(function (a, b) { return a.y - b.y; });

    // A droplet that lands pushes its mark onto e.items mid-pass. That is
    // deliberate - the mark belongs where the drop stopped and nowhere
    // else - and it simply gets drawn from the next frame on.
    flat.concat(up).forEach(function (it) {
      if (now < it.born) { return; }
      var age = now - it.born, t = Math.min(1, age / it.life);
      if (t >= 1) { return; }
      // Everything appears by growing and leaves by fading, which is what
      // makes a crystal look pushed up rather than switched on.
      var grown = it.grow ? Math.min(1, age / it.grow) : 1;
      grown = grown * grown * (3 - 2 * grown);
      var alpha = fade * (t > 0.7 ? (1 - t) / 0.3 : 1);
      if (alpha <= 0.01) { return; }

      switch (it.what) {
        case 'boom':
          self.drawBoom(ctx, it, t, alpha);
          break;
        case 'lobe':
          it.z += (it.vz || 0) * dt;
          it.spin += 0.5 * dt;
          self.drawLobe(ctx, it, grown, alpha * 0.9);
          break;
        case 'missile':
          self.drawMissile(ctx, it, t, alpha);
          break;
        case 'circle':
          self.drawCircleSigil(ctx, it, grown, alpha, now);
          break;
        case 'sigil':
          // Turning, and drifting outwards as it turns.
          it.orbit.a += it.orbit.sp * dt;
          var od = it.orbit.d + it.open * grown;
          it.x = it.orbit.x + Math.cos(it.orbit.a) * od;
          it.y = it.orbit.y + Math.sin(it.orbit.a) * od * 0.72;
          it.spin += 1.4 * dt;
          self.drawSigil(ctx, it, grown, alpha);
          break;
        case 'plus':
          it.z += (it.vz || 0) * dt;
          self.drawPlus(ctx, it, alpha);
          break;
        case 'rope':
          self.drawRope(ctx, it, t, alpha);
          break;
        case 'drop':
          // Up, over and down again, which is what a thrown droplet does
          // and a rising puff does not. When it lands it stops being a
          // droplet and becomes a wet mark on the paper, which is the
          // half of a splash you actually go on seeing.
          if (it.spent) { break; }
          it.vz -= ELEM_GRAVITY * 0.55 * dt;
          it.x += (it.vx || 0) * dt;
          it.y += (it.vy || 0) * dt;
          it.z += it.vz * dt;
          if (it.z <= 0) {
            it.spent = true;
            e.items.push({ what: 'spot', x: it.x, y: it.y,
              r: it.r * (2.2 + Math.random() * 2.6), seed: Math.random() * 30,
              face: it.wet || 'rgba(96,150,190,.5)',
              born: now, life: 900 + Math.random() * 700, grow: 130 });
            // A couple of far smaller beads thrown back up out of the
            // impact, which is the bit that makes it read as a splash
            // rather than as a droplet being switched off.
            if (it.r > 1.6) {
              for (var sb = 0; sb < 2; sb++) {
                e.items.push({ what: 'drop', x: it.x, y: it.y, z: 1,
                  vx: (Math.random() - 0.5) * 55, vy: (Math.random() - 0.5) * 38,
                  vz: 34 + Math.random() * 42, r: it.r * 0.4,
                  face: it.face, wet: it.wet,
                  born: now, life: 520, grow: 0 });
              }
            }
            break;
          }
          var dp = self.projectUp([it.x, it.y, it.z]);
          // A droplet in the air is stretched along the way it is going.
          // Not much, though: the first go at this drew tall opaque ovals
          // and they read as pills rather than as water.
          var dl = 1 + Math.min(1.3, Math.abs(it.vz) / 140);
          ctx.globalAlpha = alpha * 0.62;
          ctx.fillStyle = it.face;
          ctx.beginPath();
          ctx.ellipse(dp[0], dp[1], it.r * dp[2], it.r * dp[2] * dl, 0, 0, Math.PI * 2);
          ctx.fill();
          // Only the big ones catch a glint. On a two pixel bead a
          // highlight is just a lighter bead.
          if (it.r > 2) {
            ctx.globalAlpha = alpha * 0.45;
            ctx.fillStyle = 'rgba(255,255,255,.7)';
            ctx.beginPath();
            ctx.ellipse(dp[0] - it.r * dp[2] * 0.3, dp[1] - it.r * dp[2] * dl * 0.3,
                        it.r * dp[2] * 0.22, it.r * dp[2] * 0.22, 0, 0, Math.PI * 2);
            ctx.fill();
          }
          ctx.globalAlpha = 1;
          break;
        case 'spot':
          // Water on paper. It darkens where it soaks in, has a slightly
          // heavier rim where the edge dried last, and is not round.
          var sp = self.project([it.x, it.y, 0]);
          var sr = it.r * grown * sp[2];
          ctx.save();
          ctx.globalAlpha = alpha * 0.75;
          ctx.translate(sp[0], sp[1]);
          ctx.scale(1, 0.42);
          var sg = ctx.createRadialGradient(0, 0, 0, 0, 0, sr);
          sg.addColorStop(0, it.face);
          sg.addColorStop(0.72, it.face);
          sg.addColorStop(1, 'rgba(0,0,0,0)');
          ctx.fillStyle = sg;
          ctx.beginPath();
          for (var sq = 0; sq <= 16; sq++) {
            var sa = (sq / 16) * Math.PI * 2;
            var srr = sr * (0.8 + 0.35 * fbm1(it.seed + sq * 0.8, 2));
            var sx2 = Math.cos(sa) * srr, sy2 = Math.sin(sa) * srr;
            if (sq) { ctx.lineTo(sx2, sy2); } else { ctx.moveTo(sx2, sy2); }
          }
          ctx.closePath();
          ctx.fill();
          ctx.restore();
          ctx.globalAlpha = 1;
          break;
        case 'ember':
          // A scrap of the fire that got away. It rises while it is hot,
          // wanders on the draught, and starts to drop once it has cooled
          // - and its colour is its temperature, so it goes from white
          // through orange to a dull red before it goes out.
          var eh = 1 - t;
          it.vz -= ELEM_GRAVITY * 0.14 * dt;
          it.vz += 46 * eh * dt;
          it.x += (it.vx + Math.sin(now / 240 + it.seed) * 22) * dt;
          it.y += (it.vy + Math.cos(now / 310 + it.seed) * 12) * dt;
          it.z = Math.max(0, it.z + it.vz * dt);
          var ep = self.projectUp([it.x, it.y, it.z]);
          var er = it.r * ep[2] * (0.6 + eh * 0.7);
          ctx.save();
          ctx.globalCompositeOperation = 'lighter';
          // The glow round it, then the coal itself.
          ctx.globalAlpha = alpha * 0.34 * eh;
          ctx.fillStyle = it.face;
          ctx.beginPath();
          ctx.arc(ep[0], ep[1], er * 3.4, 0, Math.PI * 2);
          ctx.fill();
          ctx.restore();
          ctx.globalAlpha = alpha * (0.4 + eh * 0.6);
          ctx.fillStyle = eh > 0.66 ? '#ffd24a' : eh > 0.3 ? it.face : '#c8481a';
          ctx.beginPath();
          ctx.arc(ep[0], ep[1], Math.max(0.4, er), 0, Math.PI * 2);
          ctx.fill();
          ctx.globalAlpha = 1;
          break;
        case 'spark':
          // Thrown off a strike: a short bright streak with a tail behind
          // it, falling, and going out rather than fading evenly.
          it.vz -= ELEM_GRAVITY * 0.85 * dt;
          var px0 = it.x, py0 = it.y, pz0 = it.z;
          it.x += it.vx * dt;
          it.y += it.vy * dt;
          it.z += it.vz * dt;
          if (it.z < 0) { it.z = 0; it.vz *= -0.32; it.vx *= 0.6; it.vy *= 0.6; }
          var s0 = self.projectUp([px0, py0, pz0]);
          var s1 = self.projectUp([it.x, it.y, it.z]);
          // A spark flickers: it is a bit of metal tumbling, not a lamp.
          var flick = 0.45 + 0.55 * Math.abs(Math.sin(now / 34 + it.seed * 9));
          ctx.save();
          ctx.globalCompositeOperation = 'lighter';
          ctx.strokeStyle = it.face;
          ctx.lineCap = 'round';
          ctx.globalAlpha = alpha * 0.28 * flick;
          ctx.lineWidth = it.r * 3.4;
          ctx.beginPath();
          ctx.moveTo(s0[0], s0[1]); ctx.lineTo(s1[0], s1[1]);
          ctx.stroke();
          ctx.restore();
          // The spark itself in plain blending and a saturated colour, so
          // it is still a spark over cream parchment.
          ctx.globalAlpha = alpha * flick;
          ctx.strokeStyle = it.face;
          ctx.lineCap = 'round';
          ctx.lineWidth = it.r;
          ctx.beginPath();
          ctx.moveTo(s0[0], s0[1]); ctx.lineTo(s1[0], s1[1]);
          ctx.stroke();
          ctx.globalAlpha = 1;
          break;
        case 'mote':
          // What is left hanging in the air afterwards. Barely moving,
          // barely there, and the reason the effect does not simply stop.
          it.z += (it.vz || 0) * dt;
          it.x += Math.sin(now / 420 + it.seed) * 9 * dt;
          var mp = self.projectUp([it.x, it.y, it.z]);
          var mr = it.r * mp[2] * (0.7 + 0.5 * Math.sin(now / 160 + it.seed * 3));
          ctx.save();
          ctx.globalCompositeOperation = 'lighter';
          ctx.globalAlpha = alpha * 0.5;
          ctx.fillStyle = it.face;
          ctx.beginPath();
          ctx.arc(mp[0], mp[1], Math.max(0.3, mr * 2.6), 0, Math.PI * 2);
          ctx.fill();
          ctx.restore();
          ctx.globalAlpha = alpha * 0.85;
          ctx.fillStyle = it.face;
          ctx.beginPath();
          ctx.arc(mp[0], mp[1], Math.max(0.25, mr), 0, Math.PI * 2);
          ctx.fill();
          ctx.globalAlpha = 1;
          break;
        case 'flash':
          // The sky over the page for an instant.
          //
          // The obvious thing is to wash the page white, and on a cream
          // sheet that does nothing at all - there is no headroom above
          // cream. What a strike actually does to a room is throw
          // everything else into shadow, so this goes the other way: a
          // cold blue-grey pulls the whole page down for a moment and
          // the bolt is the only bright thing left in it.
          //
          // It is also a full-screen change of brightness, so it is
          // skipped outright for a reader who has asked for less motion.
          if (FLASH_OFF) { break; }
          var fl = Math.pow(1 - t, 2.2);
          ctx.globalAlpha = alpha * fl * (it.peak || 0.3);
          ctx.fillStyle = it.face;
          // In css pixels: the context carries the device-pixel scale.
          ctx.fillRect(0, 0, self.w, self.h);
          ctx.globalAlpha = 1;
          break;
        case 'slash':
          self.drawSlash(ctx, it, t, alpha);
          break;
        case 'scar':
          self.drawScar(ctx, it, t, alpha * 0.8);
          break;
        case 'hexring':
          self.drawHexRing(ctx, it, t, alpha);
          break;
        case 'panther':
          self.drawPanther(ctx, it, grown, alpha, now);
          break;
        case 'thorn':
          self.drawThorn(ctx, it, grown, alpha);
          break;
        case 'tuft':
          self.drawTuft(ctx, it, grown, alpha);
          break;
        case 'burst':
          // A hole in the paper on its own, with nothing coming out of
          // it that the burst itself has to know about.
          self.drawBurst(ctx, self.projectUp([it.x, it.y, 0]),
                         it.r * grown, it.angle, it, alpha * 0.9);
          break;
        case 'shield':
          self.drawShield(ctx, it, grown, alpha, now);
          break;
        case 'dome':
          self.drawDome(ctx, it, grown, alpha, now);
          break;
        case 'cairn':
          self.drawCairn(ctx, it, grown, alpha, now);
          break;
        case 'glass':
          self.drawGlass(ctx, it, grown, alpha, now);
          break;
        case 'bloom':
          self.drawBloom(ctx, it, grown, alpha, now);
          break;
        case 'tome':
          self.drawTome(ctx, it, grown, alpha, now);
          break;
        case 'note':
          it.z += (it.vz || 0) * dt;
          self.drawNote(ctx, it, grown, alpha, now);
          break;
        case 'hand':
          self.drawHand(ctx, it, grown, alpha, now);
          break;
        case 'sleet':
          // Round the column and down. The streak is drawn from where it
          // was to where it is, so a fast one near the middle of the
          // storm smears and a slow one high up does not - which is the
          // whole of the motion blur and costs nothing.
          if (it.done) { break; }
          var qx = it.orbit.x + Math.cos(it.orbit.a) * it.orbit.d;
          var qy = it.orbit.y + Math.sin(it.orbit.a) * it.orbit.d * 0.72;
          var qz = it.z;
          it.orbit.a += it.orbit.sp * dt;
          // Drawn inwards as it falls, the way anything in a funnel is.
          it.orbit.d *= 1 - 0.32 * dt;
          it.z -= it.fall * dt;
          if (it.z <= 0) {
            it.z = 0;
            it.done = true;
            // Sleet that lands wets the paper the way water does, only
            // paler, and only the hard ones do it.
            if (it.hard && it.mark && Math.random() < 0.45) {
              e.items.push({ what: 'spot', x: qx, y: qy,
                r: it.r * (1.8 + Math.random() * 1.8),
                seed: Math.random() * 30, face: it.mark,
                born: now, life: 700 + Math.random() * 600, grow: 110 });
            }
            break;
          }
          var n0 = self.projectUp([qx, qy, qz]);
          var n1 = self.projectUp([
            it.orbit.x + Math.cos(it.orbit.a) * it.orbit.d,
            it.orbit.y + Math.sin(it.orbit.a) * it.orbit.d * 0.72, it.z]);
          ctx.globalAlpha = alpha * (it.hard ? 0.9 : 0.75);
          if (it.hard) {
            ctx.strokeStyle = it.face;
            ctx.lineCap = 'round';
            ctx.lineWidth = it.r * n1[2];
            ctx.beginPath();
            ctx.moveTo(n0[0], n0[1]);
            ctx.lineTo(n1[0], n1[1]);
            ctx.stroke();
          } else {
            // A flake is a soft blob that tumbles, so it breathes in and
            // out as it goes rather than being a fixed dot.
            var fr = it.r * n1[2] * (0.7 + 0.45 * Math.sin(now / 110 + it.seed));
            ctx.fillStyle = it.face;
            ctx.beginPath();
            ctx.arc(n1[0], n1[1], Math.max(0.4, fr), 0, Math.PI * 2);
            ctx.fill();
          }
          ctx.globalAlpha = 1;
          break;
        case 'swirl':
          // A smear of the air itself, turning with the storm.
          it.orbit.a += it.orbit.sp * dt;
          it.x = it.orbit.x + Math.cos(it.orbit.a) * it.orbit.d;
          it.y = it.orbit.y + Math.sin(it.orbit.a) * it.orbit.d * 0.72;
          var wp = self.projectUp([it.x, it.y, it.z]);
          ctx.save();
          ctx.globalAlpha = alpha * 0.4 * grown;
          ctx.strokeStyle = it.face;
          ctx.lineCap = 'round';
          ctx.lineWidth = 5 * wp[2];
          ctx.beginPath();
          // An arc of the orbit it is on, trailing behind it.
          for (var wq = 0; wq <= 8; wq++) {
            var wa = it.orbit.a - (wq / 8) * 0.9;
            var wpt = self.projectUp([
              it.orbit.x + Math.cos(wa) * it.orbit.d,
              it.orbit.y + Math.sin(wa) * it.orbit.d * 0.72, it.z]);
            if (wq) { ctx.lineTo(wpt[0], wpt[1]); } else { ctx.moveTo(wpt[0], wpt[1]); }
          }
          ctx.stroke();
          ctx.restore();
          ctx.globalAlpha = 1;
          break;
        case 'flame':
          self.drawFlame(ctx, it, grown, alpha, now);
          break;
        case 'spike':
          self.drawFaceted(ctx, it, grown, alpha);
          break;
        case 'rock':
          self.drawFaceted(ctx, it, grown, alpha);
          break;
        case 'chunk':
          // Up hard, then settling back a little, the way something heaved
          // out of the ground does.
          it.z = it.h * (grown < 0.75 ? grown / 0.75 : 1 - (grown - 0.75) * 0.5);
          self.drawChunk(ctx, it, alpha);
          break;
        case 'puff':
          if (it.orbit) {
            it.orbit.a += it.orbit.sp * dt;
            it.x = it.orbit.x + Math.cos(it.orbit.a) * it.orbit.d;
            it.y = it.orbit.y + Math.sin(it.orbit.a) * it.orbit.d * 0.7;
          }
          it.z += (it.vz || 0) * dt;
          if (it.z < 0) { it.z = 0; }
          if (it.swell) { it.r += it.swell * dt; }
          self.drawPuff(ctx, it, alpha * 0.85);
          break;
        case 'gust':
          it.x += it.vx * dt;
          ctx.globalAlpha = alpha * 0.5;
          ctx.strokeStyle = it.face;
          ctx.lineWidth = 1.6;
          ctx.lineCap = 'round';
          var a1 = self.projectUp([it.x, it.y, it.z]);
          var b1 = self.projectUp([it.x - it.r, it.y, it.z]);
          ctx.beginPath();
          ctx.moveTo(b1[0], b1[1]);
          ctx.lineTo(a1[0], a1[1]);
          ctx.stroke();
          ctx.globalAlpha = 1;
          break;
        case 'bolt':
          // Struck, then the after image of it, then gone. The strike
          // itself is drawn in one place so every kind of bolt - forked,
          // ribbon, beaded - gets the same light piled up the same way.
          self.drawStrike(ctx, it, 1 - t * t, alpha);
          break;
        case 'ring':
          self.drawGroundRing(ctx, it.x, it.y, it.r * (0.3 + grown * 0.7),
                              it.face, it.width || 7, alpha * 0.8);
          break;
        case 'pool':
          var pp = self.project([it.x, it.y, 0]);
          var pr = it.r * grown * pp[2];
          var pg = ctx.createRadialGradient(pp[0], pp[1], 0, pp[0], pp[1], pr);
          pg.addColorStop(0, it.face);
          pg.addColorStop(1, 'rgba(0,0,0,0)');
          ctx.globalAlpha = alpha * 0.55;
          ctx.fillStyle = pg;
          ctx.beginPath();
          ctx.ellipse(pp[0], pp[1], pr, pr * 0.45, 0, 0, Math.PI * 2);
          ctx.fill();
          ctx.globalAlpha = 1;
          break;
        case 'crest':
          self.drawCrest(ctx, it, t, alpha);
          break;
        case 'beam':
          self.drawBeam(ctx, it.x, it.y, it.r * grown, it.h * grown,
                        it.face, alpha, now);
          break;
      }
    });
    ctx.restore();
  };

  // ------------------------------------------------------- roll spectacle
  //
  // What a natural 20 and a natural 1 look like on the table. Both are drawn
  // onto the dice canvas in the same world the dice live in - viewport pixels
  // flat on the page, z up - so a firework goes off above the die that rolled
  // it and a knife comes down onto that die rather than onto the middle of
  // the screen.
  //
  // The whole thing is over in about a second and a half, and the hold phase
  // waits for it rather than the other way round: the dice have to still be
  // there to be celebrated over or stabbed.
  var FX_GRAVITY = 1150;

  // A shell bursts in one of these: mostly gold, with a green and a red in
  // the bag so five shells are not five of the same firework.
  var SHELL_COLOURS = [
    [255, 226, 138], [255, 244, 214], [255, 170, 72],
    [176, 255, 188], [255, 138, 118]
  ];

  // What a fumble bleeds. Not blood - the page is already the colour of
  // blood - but the green of something that should not have been in the vial.
  var ICHOR = [86, 174, 58];
  var ICHOR_DARK = [52, 112, 34];

  function rgba(c, a) {
    return 'rgba(' + c[0] + ',' + c[1] + ',' + c[2] + ',' + a.toFixed(3) + ')';
  }

  // anchorDie is what the spectacle happens to: the d20 that actually rolled
  // the natural, or whatever else is on the table if it cannot be found.
  DiceBoard.prototype.anchorDie = function (nat) {
    var found = null;
    this.dice.forEach(function (d) {
      if (!found && d.sides === 20 && d.value === nat) { found = d; }
    });
    return found || this.dice[0] || null;
  };

  // spectacle starts one. It is called the moment the dice come to rest, so
  // the number is already readable underneath it.
  DiceBoard.prototype.spectacle = function (kind) {
    var d = this.anchorDie(kind === 'crit' ? 20 : 1);
    if (!d) { this.fx = null; return; }
    var now = performance.now();
    if (kind === 'crit') { this.startFireworks(d, now); }
    else { this.startKnife(d, now); }
  };

  DiceBoard.prototype.fxBusy = function (now) {
    return !!(this.fx && now < this.fx.until);
  };

  // A stab knocks the table. Nothing else does.
  DiceBoard.prototype.fxShake = function (now) {
    var f = this.fx;
    if (!f || f.kind !== 'fumble' || now < f.hit) { return [0, 0]; }
    var t = (now - f.hit) / 320;
    if (t >= 1) { return [0, 0]; }
    var fall = (1 - t) * (1 - t);
    return [Math.sin(t * 58) * 7 * fall, Math.cos(t * 47) * 5 * fall];
  };

  // ---- a natural 20 ---------------------------------------------------
  DiceBoard.prototype.startFireworks = function (d, now) {
    var shells = [], i;
    for (i = 0; i < 6; i++) {
      var ang = (i / 6) * Math.PI * 2 + Math.random() * 0.9;
      var reach = 44 + Math.random() * 92;
      shells.push({
        at: now + i * 125 + Math.random() * 60,
        rise: 290 + Math.random() * 110,
        from: [d.pos[0], d.pos[1], d.rest + 4],
        // Shells lean outwards as they climb, so they burst in a ring over
        // the die rather than in a column on top of it.
        to: [d.pos[0] + Math.cos(ang) * reach,
             d.pos[1] + Math.sin(ang) * reach * 0.66,
             d.rest + 170 + Math.random() * 150],
        colour: SHELL_COLOURS[i % SHELL_COLOURS.length],
        gone: false
      });
    }
    this.fx = { kind: 'crit', t0: now, last: now, until: now + 1750,
                shells: shells, sparks: [], flashes: [] };
  };

  DiceBoard.prototype.burst = function (shell, now) {
    var f = this.fx, n = 34 + Math.floor(Math.random() * 14), i;
    for (i = 0; i < n; i++) {
      // An even scatter over the sphere, squashed a little so a burst reads
      // as a firework seen from above rather than as a flat ring.
      var u = Math.random() * 2 - 1;
      var th = Math.random() * Math.PI * 2;
      var r = Math.sqrt(1 - u * u);
      var sp = 210 + Math.random() * 260;
      f.sparks.push({
        p: [shell.to[0], shell.to[1], shell.to[2]],
        q: [shell.to[0], shell.to[1], shell.to[2]],
        v: [Math.cos(th) * r * sp, Math.sin(th) * r * sp * 0.8, u * sp * 0.9],
        c: shell.colour, born: now, life: 620 + Math.random() * 320
      });
    }
    f.flashes.push({ at: shell.to.slice(), born: now, colour: shell.colour });
    pop(0.5 + Math.random() * 0.4);
  };

  DiceBoard.prototype.stepFireworks = function (ctx, now, dt) {
    var f = this.fx, self = this, i;
    // Shells on their way up, drawn as a comet with a short tail.
    f.shells.forEach(function (sh) {
      if (sh.gone || now < sh.at) { return; }
      var t = (now - sh.at) / sh.rise;
      if (t >= 1) { sh.gone = true; self.burst(sh, now); return; }
      // Easing out on the way up is what makes a shell look like it is
      // running out of push rather than being lifted at a constant rate.
      var e = 1 - (1 - t) * (1 - t);
      var at = [sh.from[0] + (sh.to[0] - sh.from[0]) * e,
                sh.from[1] + (sh.to[1] - sh.from[1]) * e,
                sh.from[2] + (sh.to[2] - sh.from[2]) * e];
      var back = Math.max(0, e - 0.11);
      var tail = [sh.from[0] + (sh.to[0] - sh.from[0]) * back,
                  sh.from[1] + (sh.to[1] - sh.from[1]) * back,
                  sh.from[2] + (sh.to[2] - sh.from[2]) * back];
      var a = self.project(at), b = self.project(tail);
      ctx.globalCompositeOperation = 'lighter';
      ctx.strokeStyle = rgba(sh.colour, 0.75);
      ctx.lineWidth = 2.4 * a[2];
      ctx.lineCap = 'round';
      ctx.beginPath();
      ctx.moveTo(b[0], b[1]);
      ctx.lineTo(a[0], a[1]);
      ctx.stroke();
      ctx.globalCompositeOperation = 'source-over';
    });

    // The white of the burst itself, one frame or two of it.
    f.flashes = f.flashes.filter(function (fl) { return now - fl.born < 220; });
    f.flashes.forEach(function (fl) {
      var t = (now - fl.born) / 220;
      var p = self.project(fl.at);
      var r = (14 + t * 96) * p[2];
      var g = ctx.createRadialGradient(p[0], p[1], 0, p[0], p[1], r);
      g.addColorStop(0, 'rgba(255,255,255,' + (0.7 * (1 - t)).toFixed(3) + ')');
      g.addColorStop(0.22, rgba(fl.colour, 0.6 * (1 - t)));
      g.addColorStop(1, rgba(fl.colour, 0));
      ctx.globalCompositeOperation = 'lighter';
      ctx.fillStyle = g;
      ctx.beginPath();
      ctx.arc(p[0], p[1], r, 0, Math.PI * 2);
      ctx.fill();
      ctx.globalCompositeOperation = 'source-over';
    });

    // And the sparks, each drawn as the streak between where it was last
    // frame and where it is now, which is what gives them their tails for
    // nothing.
    ctx.globalCompositeOperation = 'lighter';
    ctx.lineCap = 'round';
    var alive = [];
    for (i = 0; i < f.sparks.length; i++) {
      var s = f.sparks[i];
      var age = (now - s.born) / s.life;
      if (age >= 1) { continue; }
      s.q = s.p.slice();
      s.v[2] -= FX_GRAVITY * 0.5 * dt;
      var drag = Math.max(0, 1 - 2.1 * dt);
      s.v[0] *= drag; s.v[1] *= drag; s.v[2] *= drag;
      s.p[0] += s.v[0] * dt; s.p[1] += s.v[1] * dt; s.p[2] += s.v[2] * dt;
      // A spark that reaches the paper is out; it does not roll about.
      if (s.p[2] < 0) { continue; }
      var pa = this.project(s.p), pb = this.project(s.q);
      // Fading and cooling at once: a spark goes from white hot to its own
      // colour to nothing, which is most of what sells it.
      var fade = 1 - age * age;
      // White hot for the first blink of it, then its own colour.
      ctx.strokeStyle = age < 0.14
        ? 'rgba(255,255,244,' + (0.95 * fade).toFixed(3) + ')'
        : rgba(s.c, 0.95 * fade);
      ctx.lineWidth = Math.max(0.8, 3 * pa[2] * fade);
      ctx.beginPath();
      ctx.moveTo(pb[0], pb[1]);
      ctx.lineTo(pa[0], pa[1]);
      ctx.stroke();
      alive.push(s);
    }
    f.sparks = alive;
    ctx.globalCompositeOperation = 'source-over';
  };

  // ---- a natural 1 ----------------------------------------------------
  DiceBoard.prototype.startKnife = function (d, now) {
    this.fx = {
      kind: 'fumble', t0: now, last: now, until: now + 1500,
      hit: now + 290,
      // Where the point goes in: the top face of the die that rolled it.
      at: [d.pos[0], d.pos[1], d.rest + d.size * 0.55],
      drops: [], splats: [], struck: false
    };
  };

  DiceBoard.prototype.splatter = function (now) {
    var f = this.fx, i;
    for (i = 0; i < 26; i++) {
      var th = Math.random() * Math.PI * 2;
      var sp = 90 + Math.random() * 260;
      f.drops.push({
        p: f.at.slice(),
        q: f.at.slice(),
        v: [Math.cos(th) * sp, Math.sin(th) * sp * 0.75,
            60 + Math.random() * 300],
        r: 1.2 + Math.random() * Math.random() * 4.6,
        dark: Math.random() < 0.35
      });
    }
    // A few land under the blade straight away, which is what makes the hit
    // look wet rather than making the drops look thrown.
    for (i = 0; i < 5; i++) {
      var a = Math.random() * Math.PI * 2, rad = Math.random() * 26;
      f.splats.push({
        x: f.at[0] + Math.cos(a) * rad, y: f.at[1] + Math.sin(a) * rad * 0.7,
        r: 2.5 + Math.random() * 5, born: now, dark: Math.random() < 0.5,
        ox: Math.random() * 1.6 - 0.8, oy: Math.random() * 1.6 - 0.8
      });
    }
    stab();
  };

  // The knife itself, drawn with its point at the origin and the handle
  // going up: a tapered blade, a bronze crossguard and a bound grip.
  DiceBoard.prototype.drawKnife = function (ctx, x, y, s, ang) {
    ctx.save();
    ctx.translate(x, y);
    ctx.rotate(ang);
    ctx.scale(s, s);

    ctx.shadowColor = 'rgba(0,0,0,.35)';
    ctx.shadowBlur = 9;
    ctx.shadowOffsetY = 3;

    var steel = ctx.createLinearGradient(-8, 0, 8, 0);
    steel.addColorStop(0, '#6d7480');
    steel.addColorStop(0.34, '#e8edf3');
    steel.addColorStop(0.52, '#aab3bd');
    steel.addColorStop(1, '#5b626c');
    ctx.fillStyle = steel;
    ctx.beginPath();
    ctx.moveTo(0, 0);
    ctx.lineTo(-6.5, -16);
    ctx.lineTo(-7.5, -54);
    ctx.lineTo(7.5, -54);
    ctx.lineTo(6.5, -16);
    ctx.closePath();
    ctx.fill();
    ctx.shadowColor = 'transparent';

    // The fuller down the middle of the blade, which is the one line that
    // stops it reading as a grey triangle.
    ctx.strokeStyle = 'rgba(255,255,255,.55)';
    ctx.lineWidth = 1.4;
    ctx.beginPath();
    ctx.moveTo(-1.4, -6);
    ctx.lineTo(-1.4, -50);
    ctx.stroke();

    ctx.fillStyle = '#7a5a22';
    ctx.beginPath();
    ctx.moveTo(-17, -54);
    ctx.lineTo(17, -54);
    ctx.lineTo(13, -62);
    ctx.lineTo(-13, -62);
    ctx.closePath();
    ctx.fill();
    ctx.fillStyle = 'rgba(255,225,150,.45)';
    ctx.fillRect(-17, -56, 34, 1.6);

    var grip = ctx.createLinearGradient(-6, 0, 6, 0);
    grip.addColorStop(0, '#2e2115');
    grip.addColorStop(0.45, '#6b4a26');
    grip.addColorStop(1, '#241a10');
    ctx.fillStyle = grip;
    ctx.beginPath();
    ctx.moveTo(-5.4, -62);
    ctx.lineTo(5.4, -62);
    ctx.lineTo(6.4, -92);
    ctx.lineTo(-6.4, -92);
    ctx.closePath();
    ctx.fill();
    // The binding on the grip.
    ctx.strokeStyle = 'rgba(0,0,0,.45)';
    ctx.lineWidth = 1;
    for (var i = 0; i < 5; i++) {
      var gy = -66 - i * 5.5;
      ctx.beginPath();
      ctx.moveTo(-5.6, gy);
      ctx.lineTo(5.6, gy + 1.6);
      ctx.stroke();
    }

    ctx.fillStyle = '#8a6526';
    ctx.beginPath();
    ctx.ellipse(0, -95, 7.6, 5.4, 0, 0, Math.PI * 2);
    ctx.fill();
    ctx.restore();
  };

  DiceBoard.prototype.stepKnife = function (ctx, now, dt) {
    var f = this.fx, self = this;

    // The pools on the paper go down first: everything else happens over
    // them.
    f.splats = f.splats.filter(function (sp) { return now - sp.born < 1100; });
    f.splats.forEach(function (sp) {
      var t = (now - sp.born) / 1100;
      var p = self.project([sp.x, sp.y, 0]);
      // A drop spreads as it soaks in, and goes as it dries. Three blobs
      // rather than one, because liquid does not land in an ellipse - the
      // offsets are fixed per splat so it does not crawl about while it
      // fades.
      var r = sp.r * (1 + t * 0.55) * p[2];
      ctx.fillStyle = rgba(sp.dark ? ICHOR_DARK : ICHOR, 0.72 * (1 - t * t));
      ctx.beginPath();
      ctx.ellipse(p[0], p[1], r, r * 0.62, 0, 0, Math.PI * 2);
      ctx.ellipse(p[0] + sp.ox * r, p[1] + sp.oy * r * 0.6, r * 0.55,
                  r * 0.36, 0, 0, Math.PI * 2);
      ctx.ellipse(p[0] - sp.oy * r * 0.8, p[1] + sp.ox * r * 0.5, r * 0.4,
                  r * 0.26, 0, 0, Math.PI * 2);
      ctx.fill();
    });

    if (!f.struck && now >= f.hit) {
      f.struck = true;
      this.splatter(now);
    }

    // The blade: falling in, then buried and quivering.
    var s, ang, at;
    if (!f.struck) {
      var t = (now - f.t0) / (f.hit - f.t0);
      var e = t * t;
      // It comes in from up and to the right, so the page is not hidden
      // behind it on the way down.
      at = [f.at[0] + (1 - e) * 120, f.at[1] - (1 - e) * 90,
            f.at[2] + (1 - e) * 640];
      ang = -0.62 + e * 0.44;
      var p = this.project(at);
      s = p[2] * 1.05;
      this.drawKnife(ctx, p[0], p[1], s, ang);
    } else {
      var since = now - f.hit;
      // Quivering, hard at first and still within a third of a second.
      var q = Math.exp(-since / 110) * Math.sin(since / 15) * 0.09;
      var pp = this.project(f.at);
      this.drawKnife(ctx, pp[0], pp[1], pp[2] * 1.05, -0.18 + q);
    }

    // And the drops thrown out of the wound.
    var alive = [];
    for (var i = 0; i < f.drops.length; i++) {
      var d = f.drops[i];
      d.q = d.p.slice();
      d.v[2] -= FX_GRAVITY * dt;
      d.p[0] += d.v[0] * dt; d.p[1] += d.v[1] * dt; d.p[2] += d.v[2] * dt;
      if (d.p[2] <= 0) {
        // It has hit the page, so it stops being a drop and starts being a
        // stain.
        f.splats.push({ x: d.p[0], y: d.p[1], r: d.r * 1.3, born: now, dark: d.dark,
                        ox: Math.random() * 1.6 - 0.8, oy: Math.random() * 1.6 - 0.8 });
        continue;
      }
      // Drawn as the streak between where it was and where it is, so a drop
      // thrown hard reads as a flick of liquid and one falling slowly reads
      // as a bead. The same trick the fireworks use for their tails.
      var a = this.project(d.p), b = this.project(d.q);
      ctx.strokeStyle = rgba(d.dark ? ICHOR_DARK : ICHOR, 0.92);
      ctx.lineWidth = Math.max(1, d.r * 2 * a[2]);
      ctx.lineCap = 'round';
      ctx.beginPath();
      ctx.moveTo(b[0], b[1]);
      ctx.lineTo(a[0], a[1]);
      ctx.stroke();
      alive.push(d);
    }
    f.drops = alive;
  };

  DiceBoard.prototype.stepFx = function (ctx, now) {
    var f = this.fx;
    if (!f) { return; }
    if (now > f.until) { this.fx = null; return; }
    var dt = Math.min(0.032, (now - f.last) / 1000);
    f.last = now;
    ctx.save();
    // The last quarter second of anything is spent going away, so a knife
    // still standing in a die when its time is up fades rather than
    // vanishing between one frame and the next.
    var left = f.until - now;
    if (left < 260) { ctx.globalAlpha = Math.max(0, left / 260); }
    if (f.kind === 'crit') { this.stepFireworks(ctx, now, dt); }
    else { this.stepKnife(ctx, now, dt); }
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
    var best = null, spots = [], i, j;
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
        spots.push({ x: x, y: y, score: score });
        if (!best || score > best.score) { best = spots[spots.length - 1]; }
      }
    }
    if (!best) { return { x: (b.l + b.r) / 2, y: (b.t + b.b) / 2 }; }
    // Not the single best patch every time. The grid is the same on every
    // roll, so taking the winner outright means the dice - and anything
    // that happens where they land - come down in exactly the same corner
    // of the sheet all evening. Anything close to the best will do, picked
    // at random and weighted so the clearer patches still win more often.
    var good = spots.filter(function (c) { return c.score >= best.score * 0.72; });
    var total = 0, k;
    for (k = 0; k < good.length; k++) { total += good[k].score; }
    var want = Math.random() * total, at = good[good.length - 1];
    for (k = 0; k < good.length; k++) {
      want -= good[k].score;
      if (want <= 0) { at = good[k]; break; }
    }
    // And a little jitter inside the cell, so two rolls into the same
    // patch of parchment are not on top of each other.
    return { x: at.x + (Math.random() - 0.5) * cw * 0.7,
             y: at.y + (Math.random() - 0.5) * ch * 0.7 };
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

  // ------------------------------------------------- conditions and the dice
  //
  // What is wrong with the character, as it bears on a d20. The rules are the
  // engine's (rolleffects.go) and arrive worked out: a map of what each kind
  // of roll is owed, which the page looks things up in and never reasons
  // about. Every conditions answer carries a fresh one, so switching a
  // condition on in the panel changes the very next roll.
  var ADVICE = null;

  function takeAdvice(state) {
    if (state && state.advice) { ADVICE = state.advice; paintOmen(null); }
    // The same answer carries the ruleset's damage types, which is what the
    // Hurt row's picker is built from.
    if (state && state.damageTypes) { fillHitTypes(); }
  }

  // adviceFor is what the conditions have to say about one rollable. A
  // rollable that has not said what kind of roll it is - a damage roll, a hit
  // die, the custom roll box - is owed nothing: no condition in the book
  // touches those.
  // adviceKind is the same lookup as adviceFor for the rolls the page makes
  // itself rather than from a rollable on the sheet: a concentration save, a
  // spell attack inside a cast.
  function adviceKind(as, ability) {
    if (!ADVICE) { return null; }
    var a = as === 'attack' ? ADVICE.attack
          : as === 'save' ? (ADVICE.save || {})[ability] || (ADVICE.save || {})['']
          : (ADVICE.check || {})[ability] || (ADVICE.check || {})[''];
    return a && a.effects && a.effects.length ? a : null;
  }

  function adviceFor(el) {
    if (!ADVICE || !el) { return null; }
    var as = el.getAttribute('data-roll-as');
    if (!as) { return null; }
    var ability = el.getAttribute('data-ability') || '';
    var a = as === 'attack' ? ADVICE.attack
          : as === 'save' ? (ADVICE.save || {})[ability] || (ADVICE.save || {})['']
          : (ADVICE.check || {})[ability] || (ADVICE.check || {})[''];
    if (!a || !a.effects || !a.effects.length) { return null; }
    return a;
  }

  // leanPick turns advice into the reading of the d20 that stands. Advantage
  // and disadvantage cancelling is the engine's answer, not a count done
  // here - see rolleffects.go for why that is worth being careful about.
  function leanPick(a) {
    if (!a) { return ''; }
    if (a.lean === 'advantage') { return 'adv'; }
    if (a.lean === 'disadvantage') { return 'dis'; }
    return '';
  }

  // The mark itself. Three states and nothing else: something is against
  // you, something is for you, or this roll fails whatever it comes up.
  var OMEN_MARK = {
    down: '<svg viewBox="0 0 16 16" aria-hidden="true"><path d="M8 13.5 2.5 5h11z"/></svg>',
    up:   '<svg viewBox="0 0 16 16" aria-hidden="true"><path d="M8 2.5 13.5 11h-11z"/></svg>',
    fail: '<svg viewBox="0 0 16 16" aria-hidden="true">' +
          '<path d="M4 4l8 8M12 4l-8 8" stroke-width="2.4" stroke-linecap="round" fill="none"/></svg>',
    flat: '<svg viewBox="0 0 16 16" aria-hidden="true">' +
          '<path d="M3 8h10" stroke-width="2.4" stroke-linecap="round" fill="none"/></svg>'
  };

  function omenKind(a) {
    if (!a) { return 'flat'; }
    if (a.fails) { return 'fail'; }
    if (a.cancels) { return 'flat'; }
    if (a.lean === 'advantage') { return 'up'; }
    if (a.lean === 'disadvantage') { return 'down'; }
    return 'flat';
  }

  // ------------------------------------------------------------- the omen
  //
  // A mark that follows the mouse over anything a condition has an opinion
  // about, so you find out before you click rather than after. It is the
  // smallest thing that can carry the news - an arrow and one word - and it
  // sits beside the cursor rather than over the thing being pointed at,
  // which is the one bit of the page you must not cover.
  //
  // It flips around the cursor near the edges of the window. Down and to the
  // right is the resting place, because that is where a pointer's own
  // shadow already falls and so is the least surprising; against the right
  // edge it goes left, against the bottom it goes above, and it is never
  // allowed off the screen in either direction.
  var omen, omenAt = { x: 0, y: 0 };

  function buildOmen() {
    omen = document.createElement('div');
    omen.id = 'roll-omen';
    omen.setAttribute('aria-hidden', 'true');
    document.body.appendChild(omen);

    // Delegated, because the rollables are redrawn constantly - the spell
    // list, the attacks table and the inventory all rebuild themselves - and
    // a listener per element would leak with every redraw.
    document.addEventListener('mouseover', function (e) {
      var el = e.target.closest && e.target.closest('.rollable');
      paintOmen(el);
      if (el) { moveOmen(e.clientX, e.clientY); }
    });
    document.addEventListener('mouseout', function (e) {
      var el = e.target.closest && e.target.closest('.rollable');
      if (el && !el.contains(e.relatedTarget)) { paintOmen(null); }
    });
    document.addEventListener('mousemove', function (e) {
      if (omen.classList.contains('on')) { moveOmen(e.clientX, e.clientY); }
    }, { passive: true });
    // Anything that moves the page out from under the pointer takes it away:
    // a mark left behind pointing at nothing is worse than no mark.
    window.addEventListener('scroll', function () { paintOmen(null); }, { passive: true });
    window.addEventListener('blur', function () { paintOmen(null); });
  }

  function paintOmen(el) {
    if (!omen) { return; }
    var a = el ? adviceFor(el) : null;
    if (!a) { omen.classList.remove('on'); return; }
    var kind = omenKind(a);
    omen.className = 'on ' + kind;
    omen.innerHTML = OMEN_MARK[kind] +
      '<span>' + esc(a.short || (a.fails ? 'fails' : 'conditions')) + '</span>';
    moveOmen(omenAt.x, omenAt.y);
  }

  function moveOmen(x, y) {
    if (!omen || !omen.classList.contains('on')) { return; }
    omenAt.x = x; omenAt.y = y;
    // Measuring costs a layout, but only while the mark is up and only over
    // a rollable, so it is a few reads a second at the very worst.
    var w = omen.offsetWidth, h = omen.offsetHeight;
    var pad = 6, gapX = 15, gapY = 17;
    var vw = window.innerWidth, vh = window.innerHeight;
    var left = x + gapX;
    if (left + w > vw - pad) { left = x - gapX - w; }
    if (left < pad) { left = pad; }
    var top = y + gapY;
    if (top + h > vh - pad) { top = y - gapY - h; }
    if (top < pad) { top = pad; }
    omen.style.transform = 'translate(' + Math.round(left) + 'px,' + Math.round(top) + 'px)';
  }

  function modOf(el) {
    var raw = el.getAttribute('data-mod');
    if (raw === null || raw === '') { return 0; }
    var n = parseInt(String(raw).replace(/[–—−]/g, '-').replace(/[^0-9+-]/g, ''), 10);
    return isNaN(n) ? 0 : n;
  }

  // ------------------------------------------------------------------- tray
  var tray, tab, latestEl, historyEl, autoBox, board, history = [], seq = 0;

  // Whether a roll pops the tray open. Casting is the loud case - a spell
  // rolls to hit and for damage at once, and in a fight that is most of
  // what you do - so "not on casts" is its own setting rather than the
  // player having to choose between the tray always and never. Remembered,
  // because it is a preference about how you play rather than about this
  // roll.
  var POP_KEY = 'orgs.dnd.pop';
  var POP = { when: 'all' };
  try {
    var popSaved = localStorage.getItem(POP_KEY);
    if (popSaved === 'all' || popSaved === 'nocast' || popSaved === 'off') {
      POP.when = popSaved;
    }
  } catch (e) { /* private browsing */ }

  function shouldPop(isCast) {
    if (POP.when === 'off') { return false; }
    if (POP.when === 'nocast' && isCast) { return false; }
    return true;
  }
  // The coin spill has a board of its own - see CoinBoard below - because a
  // payment and a roll can land at the same moment.
  var coinBoard;

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
        '<label class="dt-auto" title="When the roll tray should open itself">' +
          '<select id="dice-auto">' +
            '<option value="all">every roll</option>' +
            '<option value="nocast">not on casts</option>' +
            '<option value="off">never</option>' +
          '</select>' +
        '</label>' +
        '<label class="dt-auto" title="Hear the dice land, and a flourish on a ' +
          'natural 20 or a natural 1">' +
          '<input type="checkbox" id="dice-sound"' + (SOUND.on ? ' checked' : '') +
          '> sound' +
        '</label>' +
        '<button type="button" class="dt-btn" id="dice-clear">Clear</button>' +
        '<button type="button" class="dt-btn dt-x" id="dice-close" aria-label="Hide roll tray">&times;</button>' +
      '</div>' +
      '<div id="dice-latest" aria-live="polite"></div>' +
      '<div class="dt-hist-head">History</div>' +
      '<div id="dice-history"></div>' +
      '<div class="dt-tip">Click anything underlined on the sheet to roll it. ' +
      'Click a reading of the newest roll to settle it on that number. ' +
      'Click a history line to roll it again.</div>';
    document.body.appendChild(tray);

    latestEl = tray.querySelector('#dice-latest');
    historyEl = tray.querySelector('#dice-history');
    autoBox = tray.querySelector('#dice-auto');
    autoBox.value = POP.when;
    autoBox.addEventListener('change', function () {
      POP.when = autoBox.value;
      try { localStorage.setItem(POP_KEY, POP.when); } catch (e) { /* private */ }
    });
    var soundBox = tray.querySelector('#dice-sound');
    soundBox.addEventListener('change', function () {
      SOUND.on = soundBox.checked;
      soundSave();
      // A knock as it is turned on, so it is obvious what was just switched on
      // without having to roll something to find out.
      if (SOUND.on) { knock(0.4, 1000); }
    });

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
      if (PAL.open) { setPalette(false); return; }
      if (LVL.open) { setLevelTray(false); return; }
      if (sessPanel && sessPanel.classList.contains('open')) { setSessPanel(false); return; }
      if (panel && panel.classList.contains('open')) { setPanel(false); return; }
      if (drawer && drawer.classList.contains('open') &&
          !drawer.contains(document.activeElement)) { setDrawer(false); return; }
      if (tray.classList.contains('open')) { setTray(false); }
    });
    renderHistory();
  }

  function setTray(open) {
    // Two drawers on the same edge: opening one closes the other.
    if (open && typeof LVL !== 'undefined' && LVL.open) { setLevelTray(false); }
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

  // cell draws one reading of a roll. Given a pick name it is drawn as a
  // button instead, which settles the roll on that reading when it is clicked.
  function cell(name, value, cls, nat, pick, chosen) {
    var body = '<span>' + name + '</span><b>' + value + '</b>';
    if (!pick) {
      return '<div class="rc-cell' + (cls || '') + natClass(nat) + '">' + body + '</div>';
    }
    return '<button type="button" class="rc-cell' + (cls || '') + natClass(nat) +
      (chosen ? ' chosen' : '') + '" data-pick="' + pick + '" ' +
      'title="Settle this roll on ' + esc(pickTitle(pick)) + '">' + body + '</button>';
  }

  // ------------------------------------------------------- settling a roll
  // A d20 is rolled twice and reported three ways: the flat roll, the better
  // of the two and the worse. Nothing on the sheet knows which of them the
  // table was owed - whether you had advantage is a thing that happens at the
  // table - so the player says so afterwards by clicking one. Until then the
  // flat roll stands, which is what an unremarkable roll should do.
  //
  // pick lives on the result rather than on the group it settles, so that a
  // cast, which rolls to hit and for damage at once, still carries exactly one
  // answer for each of them: pick for the d20, dpick for the damage.
  // pickWord names a reading the way the session table and the history line
  // want it said; pickTitle the way the button's tooltip wants it.
  function pickWord(p) {
    if (p === 'adv') { return 'advantage'; }
    if (p === 'dis') { return 'disadvantage'; }
    if (p === 'crit') { return 'critical'; }
    if (p === 'total') { return 'not a critical'; }
    return 'normal';
  }

  function pickTitle(p) {
    if (p === 'crit') { return 'a critical'; }
    if (p === 'total') { return 'no critical'; }
    if (p === 'normal') { return 'the flat roll'; }
    return pickWord(p);
  }

  // d20At is which of the three readings counts: what the player settled on
  // if they settled it, otherwise whatever the character's conditions leave
  // standing, otherwise the flat roll. Everything that reports a d20 - the
  // card, the history line, the session log, the natural 20 - goes through
  // here, so none of them can disagree with another.
  function d20At(r) {
    if (!r) { return 'normal'; }
    return r.pick || leanPick(r.advice) || 'normal';
  }

  // adviceSaid says who decided which reading stood: the player clicking
  // one, or the character being poisoned. The session log wants to know -
  // "disadvantage (Poisoned)" is a different fact from "disadvantage chosen".
  function adviceSaid(r) {
    if (r.pick) { return ' chosen'; }
    return r.advice && r.advice.short ? ' (' + r.advice.short + ')' : '';
  }

  function d20Value(g, pick) {
    if (pick === 'adv') { return g.adv; }
    if (pick === 'dis') { return g.dis; }
    return g.normal;
  }

  function d20Nat(g, pick) {
    if (pick === 'adv') { return g.advNat; }
    if (pick === 'dis') { return g.disNat; }
    return g.normalNat;
  }

  function hasCrit(g) { return g.crit !== null && g.crit !== undefined; }

  function dmgValue(g, pick) {
    return pick === 'crit' && hasCrit(g) ? g.crit : g.total;
  }

  // The one number a roll comes down to, once the player has had their say.
  function rollValue(r) {
    if (r.kind === 'check') { return d20Value(r, d20At(r)); }
    if (r.kind === 'cast') {
      if (r.damage) { return dmgValue(r.damage, r.dpick); }
      return r.attack ? d20Value(r.attack, d20At(r)) : null;
    }
    return dmgValue(r, r.dpick);
  }

  // d20Cells draws the three readings of a d20. The one that counts wears the
  // main style whether it was chosen or is simply what stands; the one that
  // was actually clicked is marked as chosen as well.
  function d20Cells(g, pick, lean) {
    var at = pick || lean || 'normal';
    return '<div class="rc-grid three">' +
      cell('Disadv', g.dis, at === 'dis' ? ' main' : '', g.disNat, 'dis', pick === 'dis') +
      cell('Normal', g.normal, at === 'normal' ? ' main' : '', g.normalNat,
           'normal', pick === 'normal') +
      cell('Advant', g.adv, at === 'adv' ? ' main' : '', g.advNat, 'adv', pick === 'adv') +
      '</div>';
  }

  // dmgCells draws a damage roll and, when it could have been a critical, the
  // number it would have done - which is the other thing a player settles.
  function dmgCells(g, pick) {
    if (!hasCrit(g)) {
      return '<div class="rc-grid one">' + cell('Total', g.total, ' main') + '</div>';
    }
    var at = pick === 'crit' ? 'crit' : 'total';
    return '<div class="rc-grid two">' +
      cell('Total', g.total, at === 'total' ? ' main' : '', 0, 'total', pick === 'total') +
      cell('If critical', g.crit, at === 'crit' ? ' main' : '', 0, 'crit', pick === 'crit') +
      '</div>';
  }

  // castCard lays out one cast: the attack it rolled, the damage or healing
  // that followed, and the saving throw the target still owes.
  function castCard(r) {
    var out = '<div class="rc"><div class="rc-label">' + SPELL_MARK + esc(r.label) + '</div>' +
      '<div class="rc-formula">' + esc(r.detail) + '</div>' + flourishLine(r);
    if (r.attack) {
      out += '<div class="rc-sub">To hit ' + esc(r.attack.formula) + '</div>' +
        adviceLine(r) + d20Cells(r.attack, r.pick, leanPick(r.advice));
    }
    if (r.damage) {
      out += '<div class="rc-sub">' + esc(r.damage.name) + ' ' + esc(r.damage.formula) + '</div>' +
        dmgCells(r.damage, r.dpick);
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

  // flourishLine is the one line a natural 20 or a natural 1 gets on the card.
  // It is the loudest thing on it, because at the table it is the loudest thing
  // that happened.
  // adviceLine is the one line that says why a roll is not the flat one -
  // which conditions had a say, what each of them does, and the caveat the
  // rules put on it. The caveat matters: frightened only bites while the
  // thing you are frightened of is in sight, so the line is an explanation
  // of what stands rather than a ruling, and the three readings are all
  // still there to be clicked.
  function adviceLine(r) {
    var a = r && r.advice;
    if (!a || !a.why) { return ''; }
    var cls = a.fails ? ' fails'
            : a.cancels ? ' cancels'
            : a.lean === 'advantage' ? ' up'
            : a.lean === 'disadvantage' ? ' down' : '';
    return '<div class="rc-advice' + cls + '">' + OMEN_MARK[omenKind(a)] +
      '<span>' + esc(a.why) + '</span></div>';
  }

  function flourishLine(r) {
    if (r.flourish === 'crit') {
      return '<div class="rc-flourish crit">Natural 20</div>';
    }
    if (r.flourish === 'fumble') {
      return '<div class="rc-flourish fumble">Natural 1</div>';
    }
    return '';
  }

  function latestCard(r) {
    if (r.kind === 'cast') { return castCard(r); }
    var head = '<div class="rc-label">' + esc(r.label) + '</div>' +
               '<div class="rc-formula">' + esc(r.formula) + '</div>' + flourishLine(r);
    if (r.kind === 'check') {
      return '<div class="rc">' + head + adviceLine(r) +
        d20Cells(r, r.pick, leanPick(r.advice)) +
        '<div class="rc-dice">two d20: <b>' + r.pair[0] + '</b>, <b>' + r.pair[1] + '</b>' +
        (r.mod ? ' &middot; modifier ' + signed(r.mod) : '') + '</div>' +
        '</div>';
    }
    return '<div class="rc">' + head + dmgCells(r, r.dpick) +
      '<div class="rc-dice">' + esc(r.detail) + '</div></div>';
  }

  function renderHistory() {
    if (!history.length) {
      historyEl.innerHTML = '<div class="dt-empty">No rolls yet.</div>';
      return;
    }
    historyEl.innerHTML = history.map(function (r) {
      if (r.kind === 'cast') { return castRow(r); }
      // A settled roll says so where it would otherwise say what the other
      // readings came to - that is the thing worth knowing about it now.
      var extra = r.kind === 'check'
        ? '<span class="hx">' + (d20At(r) !== 'normal' ? esc(pickWord(d20At(r))) : r.dis + ' / ' + r.adv) + '</span>'
        : '<span class="hx">' + (r.dpick === 'crit' ? 'critical' : esc(r.formula)) + '</span>';
      return '<button type="button" class="hr" data-roll-id="' + r.id + '" ' +
        'title="Roll ' + esc(r.label) + ' again">' +
        '<span class="ht">' + r.time + '</span>' +
        '<span class="hl">' + esc(r.label) + '</span>' + extra +
        '<span class="hv' + natClass(r.kind === 'check' ? d20Nat(r, d20At(r)) : 0) + '">' +
        rollValue(r) + '</span>' +
        '</button>';
    }).join('');
  }

  // One line of history for a cast: the mark, the spell, and the number that
  // matters most - the damage it did, or what it hit on.
  function castRow(r) {
    var main = '&middot;', natural = 0;
    if (r.damage) {
      main = dmgValue(r.damage, r.dpick);
    } else if (r.attack) {
      main = d20Value(r.attack, d20At(r));
      natural = d20Nat(r.attack, d20At(r));
    }
    var extra = r.attack && r.damage
      ? '<span class="hx">hit ' + d20Value(r.attack, d20At(r)) + '</span>'
      : '<span class="hx">' + esc(r.short) + '</span>';
    return '<button type="button" class="hr" data-roll-id="' + r.id + '" ' +
      'title="Cast ' + esc(r.label) + ' again">' +
      '<span class="ht">' + r.time + '</span>' +
      '<span class="hl">' + SPELL_MARK + esc(r.label) + '</span>' + extra +
      '<span class="hv' + natClass(natural) + '">' + main + '</span>' +
      '</button>';
  }

  // pickClick settles the roll that is still on the card. Clicking another
  // reading changes the answer, and it keeps changing until you roll again:
  // the next roll takes the card, and what is in the history stands.
  function pickClick(e) {
    var btn = e.target.closest('.rc-cell[data-pick]');
    if (!btn || !latestEl || !latestEl.contains(btn)) { return false; }
    var r = history[0];
    if (!r) { return true; }
    var what = btn.getAttribute('data-pick');
    if (what === 'crit' || what === 'total') { r.dpick = what; } else { r.pick = what; }
    latestEl.innerHTML = latestCard(r);
    renderHistory();
    relogRoll(r);
    // The reading that counts has changed, so what was natural about it may
    // have changed with it.
    showFlourish(r);
    return true;
  }

  function historyClick(e) {
    if (pickClick(e)) { return; }
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
    if (open) {
      exprInput.focus();
      exprInput.select();
      // What undo would take back is only worth asking for when the menu it
      // lives in is actually open. It goes stale the moment anything else on
      // the sheet is pressed, so asking now beats keeping it up to date.
      undoLoad();
    }
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
      '<div class="dc-actions dc-undo">' +
        '<button type="button" class="dt-btn" id="sheet-undo" disabled ' +
          'title="Take back the last thing that happened">Undo' +
          '<span class="du-what" id="sheet-undo-what">nothing yet</span>' +
        '</button>' +
      '</div>' +
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
    panel.querySelector('#sheet-undo').addEventListener('click', function () {
      undoLast();
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
               advice: adviceFor(el), element: skillElement(label),
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
             formula: text, element: el.getAttribute('data-damage-type') || '',
             noCrit: el.getAttribute('data-nocrit') === '1' };
  }

  // ------------------------------------------------------------- casting
  //
  // A cast is one click that does everything the spell asks for: the attack
  // roll if it needs one, the damage or healing that follows, and - for a
  // spell that rolls nothing itself - the saving throw the target owes,
  // written out ready to read across the table.
  //
  // A spell whose text says what a bigger slot buys carries a level picker
  // beside its button, and every level on it knows its own dice. Casting
  // reads the level picked, rolls what that level asks for, and spends a slot
  // of exactly that level.
  //
  // A cantrip carries a picker too, but of the character levels its dice grow
  // at rather than of slots - so `tier` says which kind this is, and a tier
  // never costs anything off the sheet.
  function castLevelOf(el) {
    var base = parseInt(el.getAttribute('data-level'), 10) || 0;
    var host = el.closest('summary') || el.parentNode;
    var sel = host ? host.querySelector('.cast-at') : null;
    var at = { level: base, base: base, opt: null, tier: false };
    if (!sel) { return at; }
    at.tier = sel.getAttribute('data-tier') === '1';
    at.level = parseInt(sel.value, 10) || base;
    at.opt = sel.options[sel.selectedIndex] || null;
    return at;
  }

  function castSpecFor(el) {
    var at = castLevelOf(el);
    // Above the spell's own level the picked option is the authority on what
    // is rolled; at its own level the button is, as it always was. Every tier
    // of a cantrip carries its own dice, so there the option always is.
    var read = function (name) {
      if (at.opt && (at.tier || at.level > at.base)) {
        var up = at.opt.getAttribute('data-' + name);
        if (up !== null) { return up; }
      }
      return el.getAttribute('data-' + name);
    };
    var spec = {
      kind: 'cast',
      label: el.getAttribute('data-spell') || 'Spell',
      school: el.getAttribute('data-school') || '',
      named: spellElement(el.getAttribute('data-spell'),
                          !!el.getAttribute('data-heal')),
      detail: read('detail') || '',
      line: el.getAttribute('data-line') || '',
      save: el.getAttribute('data-save') || '',
      dc: el.getAttribute('data-dc') || '',
      // A tier is not a level the spell is cast at - the cantrip is still a
      // cantrip - so the spell's own level is what the rest of the roll sees.
      level: at.tier ? at.base : at.level
    };
    var atk = el.getAttribute('data-attack');
    if (atk !== null && atk !== '') {
      var mod = parseInt(atk, 10) || 0;
      spec.attack = { flat: mod, formula: 'd20 ' + signed(mod) };
      spec.advice = adviceKind('attack', '');
    }
    var dmg = read('damage');
    if (dmg) {
      var parsed = parseDice(dmg);
      if (parsed && parsed.terms.length) {
        spec.damage = { name: 'Damage', terms: parsed.terms, flat: parsed.flat,
                        formula: dmg + ' ' + (el.getAttribute('data-damage-type') || '') };
        spec.element = el.getAttribute('data-damage-type') || '';
      }
    }
    var heal = read('heal');
    if (heal && !spec.damage) {
      var h = parseDice(heal);
      if (h && h.terms.length) {
        spec.damage = { name: 'Healing', terms: h.terms, flat: h.flat,
                        formula: heal, noCrit: true };
      }
    }
    // The few words a history row shows when there is no second roll to put
    // there. The full line only fits on the card.
    spec.short = read('short') || '';
    return spec;
  }

  // cast2SpecFor is the spell's second damage on its own: ice knife's cold
  // burst, hellfire's necrotic half. It is rolled as a plain damage roll and
  // not as a cast, because the spell has already been cast - the slot is
  // spent, the save has been called for, and this is the other half of what
  // that one casting deals. Rolling it as a cast would spend a second slot.
  //
  // It reads the level picker the same way the Cast button does, so a spell
  // upcast for more of its second damage rolls the bigger dice here.
  function cast2SpecFor(btn) {
    var el = btn.parentNode.querySelector('.cast-btn');
    if (!el) { return null; }
    var at = castLevelOf(el);
    var dmg = el.getAttribute('data-damage2');
    if (at.opt && (at.tier || at.level > at.base)) {
      var up = at.opt.getAttribute('data-damage2');
      if (up) { dmg = up; }
    }
    if (!dmg) { return null; }
    var parsed = parseDice(dmg);
    if (!parsed || !parsed.terms.length) { return null; }
    var type = el.getAttribute('data-damage2-type') || '';
    var label = el.getAttribute('data-damage2-label') || type || 'Damage';
    var name = el.getAttribute('data-spell') || 'Spell';
    return {
      kind: 'damage',
      label: name + ' \u00b7 ' + label,
      formula: dmg + (type ? ' ' + type : ''),
      terms: parsed.terms,
      flat: parsed.flat,
      // A second damage almost always lands on a saving throw rather than on
      // the attack roll - ice knife bursts whether the shard hit or not - and
      // a saving throw does not crit. So no crit is offered for one on a
      // spell that calls for a save, which is all of them that the rulesets
      // carry. A spell whose second damage rides the attack roll would say so
      // by having no save at all.
      noCrit: (el.getAttribute('data-save') || '') !== ''
    };
  }

  // Half of what a spell does is not a damage type. Water, earth, mending
  // and holding somebody still are all things the rules describe in prose
  // and never label - a tidal wave does bludgeoning, mold earth does
  // nothing at all - so they are picked out of the spell's own name.
  //
  // This is best effort by design, the same way the item texts are read
  // elsewhere in this module: a spell none of these catches simply gets
  // whatever its damage type would have given it, which is what happened
  // before any of this existed. The order matters - a spell called
  // "healing wave" is healing first.
  // What a skill looks like when you roll it.
  //
  // Only the ones with an obvious picture. An Athletics check is a person
  // climbing something and there is no drawing of that which is not
  // silly, so most skills quite deliberately have no entry here and get
  // the dice and nothing else - which is what keeps the ones that do
  // have a flourish worth looking at.
  var SKILL_ELEMENTS = {
    religion: 'mend', medicine: 'mend',
    investigation: 'glass',
    nature: 'bloom',
    insight: 'tome', history: 'tome',
    performance: 'notes'
  };

  // A roll that mattered.
  //
  // Some rolls are worth marking whatever they were for: the twenty you
  // needed, the one you did not, and the initiative that decided who
  // went first. None of them is a spell and none of them has a picture
  // of its own, so they draw from a small bank of abstract ones - the
  // magic circle and the hex burst - chosen at random so the same
  // twenty does not look the same twice.
  //
  // Only attack rolls and initiative, and only at the ends of the die.
  // A flourish on every good roll is a flourish on nothing.
  var RITE_BANK = ['circle', 'hexring', 'hexring'];

  function riteElement(label, nat) {
    if (!nat) { return ''; }
    if (nat < 18 && nat > 2) { return ''; }
    var k = String(label || '').toLowerCase();
    if (k !== 'initiative' && k.indexOf('attack') < 0) { return ''; }
    return RITE_BANK[Math.floor(Math.random() * RITE_BANK.length)];
  }

  function skillElement(label) {
    var k = String(label || '').toLowerCase().trim();
    return SKILL_ELEMENTS[k] || '';
  }

  var SPELL_ELEMENTS = [
    // Weapons. Most of them arrive here already labelled as doing
    // slashing damage and never reach this list, but an unarmed strike
    // or a homebrew attack with no damage type still swings something.
    { as: 'slashing', re: /\b(sword|blade|sabre|saber|scimitar|katana|falchion|glaive|greatsword|longsword|shortsword|rapier|axe|halberd|cleav\w*|slash\w*|slice|sever|swing)\b/i },
    // Named spells with a picture of their own. All of these go before
    // the generic patterns below - "spike growth" would otherwise be
    // claimed by the earth regex's `spike\w*`, and "protection from
    // energy" by nothing at all.
    { as: 'panther', re: /\b(panther|jaguar|leopard|lion|tiger|great\s*cat|phantom\s*beast)\b/i },
    { as: 'thorns', re: /\b(spike\s*growth|thorn\w*|briar\w*|bramble\w*|entangle)\b/i },
    { as: 'stoneskin', re: /\b(stone\s*skin|stoneskin)\b/i },
    { as: 'barkskin', re: /\b(bark\s*skin|barkskin)\b/i },
    { as: 'shield', re: /\b(shield|blade\s*ward|parry|deflect\w*)\b/i },
    { as: 'dome', re: /\b(protection|protect\w*|sanctuary|globe|ward|warding|guardian|invulnerab\w*)\b/i },
    // A hand before a hold: "Bigby's grasping hand" is a hand, and the
    // binding regex would otherwise claim it for the ropes.
    { as: 'hand', re: /\b(mage\s*hand|arcane\s*hand|spectral\s*hand|unseen\s*servant|bigby\w*|hand)\b/i },
    // Storms first. Sleet storm does no damage at all, so without this it
    // falls all the way through to the magic circle - and a circle of
    // light on the ground is the one thing a sleet storm is not.
    { as: 'blizzard', re: /\b(sleet|blizzard|snow|snowball|hail|frostbite|whiteout|rime)\b/i },
    { as: 'blizzard', re: /\b(ice|cold|frost)\s+storm\b/i },
    { as: 'blast', re: /\b(fireball|explos\w*|blast|burst|detonat\w*|shatter|boom|nova|thunderclap|meteor|delayed)\b/i },
    { as: 'missile', re: /\b(missile|missiles|arrow|arrows|bolt|dart|darts|ray|rays|orb|spear|lance|shot|seeking|guiding)\b/i },
    { as: 'mend', re: /\b(heal|healing|cure|cures|mend|mending|restoration|restore|revivify|regenerate|goodberry|vigor|vitality|life)\b/i },
    { as: 'bind', re: /\b(hold|entangle|entangling|snare|snaring|web|bind|binding|ensnar\w*|grasp\w*|restrain\w*|immobil\w*|paraly\w*|command|slow)\b/i },
    { as: 'water', re: /\b(water|wave|tidal|tsunami|torrent|geyser|maelstrom|whelm|deluge|flood)\b/i },
    { as: 'stones', re: /\b(earth|earthen|stone|stones|rock|boulder|mud|sand|dust|tremor|quake|spike\w*)\b/i }
  ];

  // spellElement is what a spell looks like when nothing has said. It is
  // also handed whether the spell heals, because a spell that restores hit
  // points is a healing spell whatever it happens to be called.
  function spellElement(name, heals) {
    if (heals) { return 'mend'; }
    for (var i = 0; i < SPELL_ELEMENTS.length; i++) {
      if (SPELL_ELEMENTS[i].re.test(String(name || ''))) {
        return SPELL_ELEMENTS[i].as === 'stones' ? 'bludgeoning' : SPELL_ELEMENTS[i].as;
      }
    }
    return '';
  }

  function castRoll(spec, origin) {
    // The school's own moment, at the button that cast it. It runs whatever
    // the dice then do, because the spell going off is not the roll.
    spellFlourish(spec.school, origin);
    var result = { kind: 'cast', label: spec.label, detail: spec.detail,
                   line: spec.line, short: spec.short, spec: spec,
                   advice: spec.advice || null,
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
    if (shouldPop(true)) { setTray(true); }
    tab.classList.remove('pulse');
    void tab.offsetWidth;
    tab.classList.add('pulse');

    var throwable = shown.filter(function (s) { return SOLID_SIDES[s.sides]; });
    if (throwable.length && board) {
      var radius = 52 + throwable.length * 6;
      board.throwDice(throwable.slice(0, 12), origin, pickLanding(origin, radius),
                      { school: spec.school,
                        // What the spell is called wins over what it
                        // damages: mold earth does no damage at all, and a
                        // tidal wave is water before it is bludgeoning.
                        // Anything with neither gets the magic circle,
                        // because every spell should show something.
                        element: spec.named ||
                                 ((spec.damage && spec.damage.name === 'Damage')
                                  ? spec.element : '') || 'circle',
                        tint: spec.element });
      // Cheering before the dice have landed gives the answer away, so the
      // flourish waits for the table.
      board.whenLanded(function () { showFlourish(result); });
    } else if (board) {
      // A spell that rolls nothing still clears the table - and still
      // happened, so it gets its moment somewhere on the parchment as
      // well as at the button. Mage hand and misty step should look like
      // something, and the emptiest patch of sheet is where the dice
      // would have landed anyway.
      board.stop();
      var away = pickLanding(origin, 70);
      spellFlourish(spec.school, away);
      var el = spec.named ||
        ((spec.damage && spec.damage.name === 'Damage') ? spec.element : '') ||
        'circle';
      board.elsewhere(el, away, spec.element);
      showFlourish(result);
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
                 advice: spec.advice || null,
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
    if (shouldPop(false)) { setTray(true); }
    tab.classList.remove('pulse');
    void tab.offsetWidth;
    tab.classList.add('pulse');

    var throwable = shown.filter(function (s) { return SOLID_SIDES[s.sides]; });
    if (throwable.length && board) {
      var radius = 52 + throwable.length * 6;
      board.throwDice(throwable.slice(0, 12), origin, pickLanding(origin, radius),
                      { element: (result.kind === 'check' &&
                                  riteElement(spec.label, natOf(result))) ||
                                 spec.element,
                        // The magnifying glass cracks on a bad roll and
                        // sparkles on a good one, so the flourish has to
                        // be told what the d20 actually did.
                        quality: result.kind === 'check' ? natOf(result) : 0 });
      board.whenLanded(function () { showFlourish(result); });
    } else if (board) {
      // Nothing to throw - a d100, say. Clear the board so the previous
      // roll's dice are not left sitting there looking like this result.
      board.stop();
      showFlourish(result);
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
  // The session currently open in the detail view, so a note card there knows
  // which note of which session it is showing.
  var openDetail = null;

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
      if (rolls.length) {
        noteRollIndexes(rolls, info);
        LOG.rolls = LOG.rolls.slice(rolls.length);
      } else { LOG.notes = LOG.notes.slice(notes.length); }
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
  // and carry their id for later queries. It also says which org sheet this
  // page came from, which is what the session file links back to.
  function who(body) {
    body.character = CHARACTER;
    body.characterId = CHARACTER_ID;
    body.characterFile = LOG.file;
    return body;
  }

  function clockNow() {
    var now = new Date();
    return ('0' + now.getHours()).slice(-2) + ':' + ('0' + now.getMinutes()).slice(-2);
  }

  // rollRow turns a result from the tray into a row of the session table. The
  // table is org text, so a cast is written out in words - the spell mark the
  // tray draws stays in the tray.
  //
  // A roll the player has settled on a different reading writes that number as
  // its result and says so in the notes; one nobody has said anything about
  // reads exactly as it always did. rid is the tray's own id for the roll,
  // which is how a row already sent is found again if it is settled later. The
  // server has no use for it and ignores it.
  function rollRow(r) {
    var row = { time: r.time, character: CHARACTER, label: r.label, formula: r.formula,
                rid: r.id };
    if (r.kind === 'cast') {
      var notes = [];
      row.formula = [r.attack ? 'attack ' + r.attack.formula : '',
                     r.damage ? r.damage.formula : ''].filter(Boolean).join(', ') || 'cast';
      row.result = r.damage ? String(dmgValue(r.damage, r.dpick))
                            : (r.attack ? String(d20Value(r.attack, d20At(r))) : '');
      row.dice = r.dice || '';
      if (r.attack) {
        var hit = 'hit ' + d20Value(r.attack, d20At(r));
        if (d20At(r) !== 'normal') { hit += ', ' + pickWord(d20At(r)) + adviceSaid(r); }
        hit += ' (' + (d20At(r) !== 'normal' ? 'normal ' + r.attack.normal + ', ' : '') +
          'adv ' + r.attack.adv + ', dis ' + r.attack.dis + ')';
        notes.push(hit);
      }
      if (r.damage && hasCrit(r.damage)) {
        if (r.dpick === 'crit') {
          notes.push('critical chosen, normal ' + r.damage.total);
        } else {
          if (r.dpick === 'total') { notes.push('not a critical'); }
          notes.push('crit ' + r.damage.crit);
        }
      }
      if (r.line) { notes.push(r.line); }
      if (r.detail) { notes.push(r.detail); }
      row.notes = notes.join('; ');
      return row;
    }
    if (r.kind === 'check') {
      row.result = String(d20Value(r, d20At(r)));
      row.dice = 'd20 ' + r.pair[0] + ', d20 ' + r.pair[1];
      row.notes = (d20At(r) !== 'normal' ? pickWord(d20At(r)) + adviceSaid(r) + ', ' : '') +
        (d20At(r) !== 'normal' ? 'normal ' + r.normal + ', ' : '') +
        'adv ' + r.adv + ', dis ' + r.dis +
        (r.mod ? ', mod ' + signed(r.mod) : '');
      return row;
    }
    row.result = String(dmgValue(r, r.dpick));
    row.dice = r.detail;
    var dn = [];
    if (hasCrit(r)) {
      if (r.dpick === 'crit') {
        dn.push('critical chosen, normal ' + r.total);
      } else {
        if (r.dpick === 'total') { dn.push('not a critical'); }
        dn.push('crit ' + r.crit);
      }
    }
    row.notes = dn.join('; ');
    return row;
  }

  function logRoll(r) {
    if (!LOG.session) { return; }
    LOG.rolls.push(rollRow(r));
    saveLog();
    flush();
    renderSession();
  }

  // relogRoll says the roll again after the player has settled it. It is the
  // same roll, not another one, so the row that was already written is
  // rewritten - a roll that never happened has no business in the table.
  //
  // A roll still waiting in the queue has not been written yet, so settling it
  // only changes what goes out. One already in the file is edited by index,
  // and edits are taken one at a time: a player changing their mind twice in a
  // second must not have the two answers land out of order.
  var editBusy = false, editNext = null;

  function relogRoll(r) {
    if (!LOG.session) { return; }
    var row = rollRow(r), i;
    for (i = 0; i < LOG.rolls.length; i++) {
      if (LOG.rolls[i].rid === r.id) {
        LOG.rolls[i] = row;
        saveLog();
        flush();
        renderSession();
        return;
      }
    }
    if (r.logIndex === undefined || r.logIndex === null) { return; }
    if (editBusy) { editNext = r; return; }
    editBusy = true;
    api('POST', '/dnd/play/session/' + encodeURIComponent(LOG.session.id) +
        '/roll/' + r.logIndex, { was: r.label, roll: row })
      .then(function (info) {
        if (info && info.id) { LOG.session = info; }
        LOG.error = '';
      }, function (err) {
        LOG.error = err.message || String(err);
      })
      .then(function () {
        editBusy = false;
        saveLog();
        renderSession();
        var again = editNext;
        editNext = null;
        if (again) { relogRoll(again); }
      });
  }

  // noteRollIndexes remembers where in the table the rows just sent landed.
  // The session file answers with how many rolls it now holds and rows are
  // only ever appended, so the batch is the last of them. That index is what a
  // roll needs to be settled after it has been written.
  function noteRollIndexes(rows, info) {
    var total = info && info.rolls;
    if (!total) { return; }
    var base = total - rows.length;
    if (base < 0) { return; }
    rows.forEach(function (row, i) {
      if (row.rid === undefined || row.rid === null) { return; }
      for (var j = 0; j < history.length; j++) {
        if (history[j].id === row.rid) { history[j].logIndex = base + i; return; }
      }
    });
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
    // Newlines are kept. Org itself flows consecutive lines of prose into
    // one paragraph, and for prose that is right - but a note typed at the
    // table as four short lines means four lines, and running them together
    // is never what was meant. So the break somebody typed is the break they
    // get, and the org written out carries org's own hard line break at the
    // end of each of them (orgBreakLines in sessionlog.go) so that the
    // exported file reads the same way this does.
    //
    // A trailing backslash pair is dropped rather than shown: a note read
    // back off a file written by hand may carry them, and they are markup
    // here, not text.
    var ORG_BREAK = /\s*\\\\\s*$/;
    function closePara() {
      if (!para.length) { return; }
      out.push('<p>' + para.map(function (line) {
        return orgInline(line.replace(ORG_BREAK, ''));
      }).join('<br>') + '</p>');
      para = [];
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

  // A note card knows where the note lives - which session, and which note of
  // that session - so it can be rewritten in place. A note the server has not
  // been told about yet is not editable: it has no place in the file to edit.
  function noteCard(n, index, sid, editable) {
    var where = '';
    if (sid && index >= 0) {
      where = ' data-note="' + index + '" data-session="' + esc(sid) + '"';
    }
    return '<div class="note-entry"' + where + '>' +
      noteCardInner(n, editable) + '</div>';
  }

  function noteCardInner(n, editable) {
    return '<div class="note-time">' + esc(n.time || '') +
        (editable
          ? '<button type="button" class="note-del">Del</button>' +
            '<button type="button" class="note-edit">Edit</button>'
          : '') +
      '</div>' +
      '<div class="org-rich">' + orgToHtml(n.text) + '</div>';
  }

  // Deleting asks first, in the card rather than in a dialog box: the note it
  // is about to throw away is right there to be read, which is the whole
  // question, and a browser confirm would cover it up.
  function askNoteDelete(el) {
    var at = noteCardOf(el);
    if (!at) { return; }
    at.card.classList.add('deleting');
    var line = at.card.querySelector('.note-time');
    if (!line) { return; }
    line.innerHTML = esc(at.note.time || '') +
      '<button type="button" class="note-del yes">Delete</button>' +
      '<button type="button" class="note-del-no">Keep</button>' +
      '<span class="note-ask">Throw this note away?</span>';
  }

  function unaskNoteDelete(el) {
    var at = noteCardOf(el);
    if (!at) { return; }
    at.card.classList.remove('deleting');
    at.card.innerHTML = noteCardInner(at.note, true);
  }

  // And doing it. The session is read back afterwards rather than the card
  // simply being removed, because deleting shifts every note after it up by
  // one and the page's numbering has to catch up with the file's.
  function doNoteDelete(el) {
    var at = noteCardOf(el);
    if (!at) { return; }
    var btn = at.card.querySelector('.note-del.yes');
    if (btn) { btn.disabled = true; }
    api('DELETE', '/dnd/play/session/' + encodeURIComponent(at.sid) +
        '/note/' + at.index + '?was=' + encodeURIComponent(at.note.time || ''))
      .then(function (info) {
        if (LOG.session && LOG.session.id === info.id) {
          LOG.session = info;
          saveLog();
          renderSession();
          loadStream(info.id);
        }
        if (openDetail && openDetail.id === at.sid) { openSession(at.sid); }
      }, function (e) {
        at.card.classList.remove('deleting');
        at.card.innerHTML = noteCardInner(at.note, true);
        var line = at.card.querySelector('.note-time');
        if (line) {
          line.innerHTML += '<span class="note-ask bad">' +
            esc(e.message || String(e)) + '</span>';
        }
      });
  }

  // The same card, opened up for editing. The text is the org markup as it
  // was typed, not what it renders to.
  function noteEditorHtml(n) {
    return '<div class="note-time">' + esc(n.time || '') + '</div>' +
      '<div class="note-editor">' +
        '<textarea spellcheck="true">' + esc(n.text || '') + '</textarea>' +
        '<div class="note-editor-foot">' +
          '<span class="nd-hint">Ctrl + Enter saves, Escape puts it back</span>' +
          '<button type="button" class="dt-btn note-edit-cancel">Cancel</button>' +
          '<button type="button" class="dc-roll note-edit-save">Save note</button>' +
        '</div>' +
        '<div class="nd-err"></div>' +
      '</div>';
  }

  // Which note a card is showing. The live stream is this session's, and the
  // detail view holds whichever session was opened to read.
  function noteAt(sid, index) {
    if (LOG.session && LOG.session.id === sid && LOG.stream[index]) {
      return LOG.stream[index];
    }
    if (openDetail && openDetail.id === sid && (openDetail.noteLog || [])[index]) {
      return openDetail.noteLog[index];
    }
    return null;
  }

  function noteCardOf(el) {
    var card = el.closest('.note-entry');
    if (!card) { return null; }
    var sid = card.getAttribute('data-session');
    var index = parseInt(card.getAttribute('data-note'), 10);
    var note = (sid && index >= 0) ? noteAt(sid, index) : null;
    return note ? { card: card, sid: sid, index: index, note: note } : null;
  }

  function openNoteEditor(el) {
    var at = noteCardOf(el);
    if (!at) { return; }
    at.card.classList.add('editing');
    at.card.innerHTML = noteEditorHtml(at.note);
    var box = at.card.querySelector('textarea');
    box.focus();
    box.setSelectionRange(box.value.length, box.value.length);
  }

  function closeNoteEditor(el) {
    var at = noteCardOf(el);
    if (!at) { return; }
    at.card.classList.remove('editing');
    at.card.innerHTML = noteCardInner(at.note, true);
  }

  // Saving rewrites the one entry in the session file and then reads the
  // session back, because the file is the state: what comes back is what the
  // note now says, whoever else has been typing into it.
  function saveNoteEdit(el) {
    var at = noteCardOf(el);
    if (!at) { return; }
    var box = at.card.querySelector('textarea');
    var err = at.card.querySelector('.nd-err');
    var text = box.value;
    if (!String(text).trim()) {
      err.textContent = 'A note cannot be left empty.';
      return;
    }
    var btn = at.card.querySelector('.note-edit-save');
    btn.disabled = true;
    err.textContent = '';
    api('POST', '/dnd/play/session/' + encodeURIComponent(at.sid) + '/note/' + at.index,
        { text: text, was: at.note.time || '' }).then(function (info) {
      if (LOG.session && LOG.session.id === info.id) {
        LOG.session = info;
        saveLog();
        renderSession();
        loadStream(info.id);
      }
      if (openDetail && openDetail.id === at.sid) { openSession(at.sid); }
    }, function (e) {
      btn.disabled = false;
      err.textContent = e.message || String(e);
    });
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
    // Notes still waiting to be sent are in the stream but not yet in the
    // file, and a note the file has never seen is not one to edit. They are
    // always the last of the stream, which is what makes the count enough.
    var written = LOG.stream.length - LOG.notes.length;
    var sid = LOG.session.id;
    var items = LOG.stream.map(function (n, i) {
      return noteCard(n, i, sid, !!LOG.url && i < written);
    }).reverse().join('');
    stream.innerHTML = items;
  }

  // Who played, for the session list and the session view.
  function castOf(s) {
    return (s.characters || []).map(function (c) { return c.name || c.id; })
      .filter(Boolean).join(', ');
  }

  // A roll a session has already recorded can be thrown away, the same way
  // a note can: the dice are sent automatically as they land, so the log
  // collects the ones rolled to see what a modifier came to as well as the
  // ones that mattered.
  function rollTable(rolls, sid) {
    if (!rolls.length) { return '<div class="dt-empty">No rolls recorded.</div>'; }
    var live = !!(sid && LOG.url);
    return '<table class="log-table"><thead><tr>' +
      '<th>Time</th><th>Who</th><th>Roll</th><th>Formula</th><th>Result</th><th>Dice</th>' +
      (live ? '<th class="log-act"></th>' : '') +
      '</tr></thead><tbody>' +
      rolls.map(function (r, i) {
        return '<tr' + (live ? ' data-roll="' + i + '" data-session="' + esc(sid) + '"' : '') +
          '><td>' + esc(r.time || '') + '</td><td>' + esc(r.character || '') +
          '</td><td>' + esc(r.label || '') + '</td><td>' + esc(r.formula || '') +
          '</td><td class="num">' + esc(r.result || '') + '</td><td class="dim">' +
          esc(r.dice || '') + '</td>' +
          (live ? '<td class="log-act">' +
            '<button type="button" class="roll-del">Del</button></td>' : '') +
          '</tr>';
      }).join('') + '</tbody></table>';
  }

  // Which roll a row is showing. Only the session being read has its rolls to
  // hand, which is the only place the button is drawn.
  function rollRowOf(el) {
    var row = el.closest('tr[data-roll]');
    if (!row) { return null; }
    var sid = row.getAttribute('data-session');
    var index = parseInt(row.getAttribute('data-roll'), 10);
    var roll = (openDetail && openDetail.id === sid &&
                (openDetail.rollLog || [])[index]) || null;
    return roll ? { row: row, sid: sid, index: index, roll: roll } : null;
  }

  // Asking happens in the row, for the same reason it happens in the card
  // for a note: the thing being thrown away should still be readable while
  // the question is being answered.
  function askRollDelete(el) {
    var at = rollRowOf(el);
    if (!at) { return; }
    at.row.classList.add('deleting');
    var cell = at.row.querySelector('.log-act');
    if (!cell) { return; }
    cell.innerHTML = '<button type="button" class="roll-del yes">Delete</button>' +
      '<button type="button" class="roll-del-no">Keep</button>';
  }

  function unaskRollDelete(el) {
    var at = rollRowOf(el);
    if (!at) { return; }
    at.row.classList.remove('deleting');
    var cell = at.row.querySelector('.log-act');
    if (cell) { cell.innerHTML = '<button type="button" class="roll-del">Del</button>'; }
  }

  function doRollDelete(el) {
    var at = rollRowOf(el);
    if (!at) { return; }
    var btn = at.row.querySelector('.roll-del.yes');
    if (btn) { btn.disabled = true; }
    api('DELETE', '/dnd/play/session/' + encodeURIComponent(at.sid) +
        '/roll/' + at.index + '?was=' + encodeURIComponent(at.roll.label || ''))
      .then(function () {
        // Read the session back rather than dropping the row: deleting
        // shifts every roll after it up by one, and the page's numbering
        // has to catch up with the file's.
        openSession(at.sid);
      }, function (e) {
        unaskRollDelete(el);
        var cell = at.row.querySelector('.log-act');
        if (cell) {
          cell.innerHTML += '<span class="note-ask bad">' +
            esc(e.message || String(e)) + '</span>';
        }
      });
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


  // ================================================================ timeline
  //
  // A session file is a flat list of rolls and a flat list of notes, in the
  // order they happened. That is the right thing to write down and the wrong
  // thing to read: forty attack rolls in a row tell you nothing, and the one
  // note between them is the bit you were looking for.
  //
  // So the timeline reads the two logs back and works out the shape of the
  // evening from them. The rule it uses is the one a table already feels:
  // rolls that come quickly one after another are a fight, and a gap is the
  // space between scenes. A run of them is drawn as one Combat block with
  // what came of it - how long, how many rounds of it, what was cast, the
  // worst hit, whether anybody rolled a twenty - and the notes that were
  // written while it was going on are hung inside it. Everything else is a
  // beat in its own right.
  //
  // Nothing here is stored. The session file is unchanged, and the timeline
  // is worked out again from it each time, so a session edited anywhere else
  // is right the next time this is opened.
  var TL = { id: '', list: null, detail: null, cache: {}, open: {},
             everything: false, busy: false, error: '' };

  // A fight is rolls no further apart than this, and it takes at least this
  // many of them before a flurry counts as one.
  var TL_GAP = 8;          // minutes
  var TL_LEAST = 3;        // rolls
  var TL_SCENE = 25;       // minutes of quiet that ends a scene

  function tlMinutes(hhmm) {
    var m = /^(\d{1,2}):(\d{2})/.exec(String(hhmm || ''));
    if (!m) { return null; }
    return parseInt(m[1], 10) * 60 + parseInt(m[2], 10);
  }

  // Sessions run past midnight, and a clock that reads 00:12 after 23:58 has
  // not gone backwards by a day. Walk the list once and push every wrap on
  // by twenty-four hours so the arithmetic downstream is plain subtraction.
  function tlUnwrap(items) {
    var day = 0, last = null;
    items.forEach(function (it) {
      if (it.at === null) { it.abs = last === null ? 0 : last; return; }
      if (last !== null && it.at + day < last - 120) { day += 1440; }
      it.abs = it.at + day;
      last = it.abs;
    });
  }

  // What a logged roll was. The session table keeps what was rolled rather
  // than what kind of thing it was, so this reads it back off the formula
  // and the dice the way a person would.
  function tlKind(r) {
    var f = String(r.formula || ''), d = String(r.dice || '');
    if (/^attack |, /.test(f) && /^attack /.test(f)) { return 'cast'; }
    if (f === 'cast') { return 'cast'; }
    if (/^d20 /.test(d)) { return 'check'; }
    return 'damage';
  }

  // The natural die under a d20 roll, once the reading the player settled on
  // is taken into account. Returns null when the row does not say - a cast
  // writes its attack up in prose, and guessing at it would put twenties on
  // the timeline that nobody rolled.
  function tlNatural(r) {
    var d = String(r.dice || '');
    var m = d.match(/d20 (\d+)/g);
    if (!m || !m.length) { return null; }
    var vals = m.map(function (x) { return parseInt(x.replace('d20 ', ''), 10); });
    var notes = String(r.notes || '');
    if (/disadvantage/.test(notes)) { return Math.min.apply(null, vals); }
    if (/advantage/.test(notes)) { return Math.max.apply(null, vals); }
    return vals[0];
  }

  function tlNumber(v) {
    var n = parseInt(String(v || '').replace(/[^0-9-]/g, ''), 10);
    return isNaN(n) ? 0 : n;
  }

  // Everything in the session, in order, as one list.
  function tlItems(d) {
    var out = [];
    (d.noteLog || []).forEach(function (n, i) {
      out.push({ what: 'note', at: tlMinutes(n.time), time: n.time || '',
                 text: n.text || '', index: i });
    });
    (d.rollLog || []).forEach(function (r, i) {
      out.push({ what: 'roll', at: tlMinutes(r.time), time: r.time || '',
                 row: r, kind: tlKind(r), nat: tlNatural(r), index: i });
    });
    // Stable: notes and rolls written in the same minute keep the order the
    // file has them in, which is the order they were written.
    out.sort(function (a, b) {
      if (a.at === null) { return b.at === null ? 0 : -1; }
      if (b.at === null) { return 1; }
      return a.at - b.at;
    });
    tlUnwrap(out);
    return out;
  }

  // The shape of the evening: combat blocks, beats, and the quiet between.
  function tlEvents(d) {
    var items = tlItems(d);
    var rolls = items.filter(function (it) { return it.what === 'roll'; });
    var events = [], used = {}, i;

    // First pass: find the flurries, and claim the rolls in them.
    var run = [];
    var flush = function () {
      if (run.length >= TL_LEAST) {
        var block = tlFight(run, items);
        run.forEach(function (it) { used[it.index] = true; });
        events.push(block);
      }
      run = [];
    };
    for (i = 0; i < rolls.length; i++) {
      if (run.length && rolls[i].abs - run[run.length - 1].abs > TL_GAP) { flush(); }
      run.push(rolls[i]);
    }
    flush();

    // Second pass: everything a flurry did not swallow, in place.
    var loose = [];
    var dumpLoose = function (before) {
      if (!loose.length) { return; }
      events.push({ kind: 'rolls', abs: loose[0].abs, time: loose[0].time,
                    items: loose.slice(),
                    key: 'r' + loose[0].index });
      loose = [];
    };
    items.forEach(function (it) {
      if (it.what === 'note') {
        // A note written during a fight belongs to the fight, not beside it.
        var owner = null;
        events.forEach(function (ev) {
          if (ev.kind === 'fight' && it.abs >= ev.abs && it.abs <= ev.until) {
            owner = ev;
          }
        });
        if (owner) { owner.beats.push(it); return; }
        dumpLoose();
        events.push({ kind: 'note', abs: it.abs, time: it.time, note: it,
                      key: 'n' + it.index });
        return;
      }
      if (used[it.index]) { dumpLoose(); return; }
      // A single remarkable roll stands on its own; the rest gather up.
      if (it.nat === 20 || it.nat === 1) {
        dumpLoose();
        events.push({ kind: 'roll', abs: it.abs, time: it.time, item: it,
                      key: 'x' + it.index });
        return;
      }
      if (loose.length && it.abs - loose[loose.length - 1].abs > TL_SCENE) {
        dumpLoose();
      }
      loose.push(it);
    });
    dumpLoose();

    events.sort(function (a, b) { return a.abs - b.abs; });
    return { events: events, items: items };
  }

  // One fight, summed up.
  function tlFight(run, items) {
    var first = run[0], last = run[run.length - 1];
    var dmg = 0, best = null, crits = 0, fumbles = 0, spells = {}, checks = 0;
    run.forEach(function (it) {
      if (it.nat === 20) { crits++; }
      if (it.nat === 1) { fumbles++; }
      if (it.kind === 'check') { checks++; return; }
      var n = tlNumber(it.row.result);
      dmg += n;
      if (!best || n > tlNumber(best.row.result)) { best = it; }
      if (it.kind === 'cast' && it.row.label) { spells[it.row.label] = 1; }
    });
    // A round is about six seconds at the table and a minute or two in the
    // room, so guessing rounds from the clock would be a lie. What can be
    // counted honestly is how many exchanges there were.
    return { kind: 'fight', abs: first.abs, until: last.abs,
             time: first.time, endTime: last.time,
             rolls: run, beats: [], key: 'f' + first.index,
             mins: Math.max(0, last.abs - first.abs),
             damage: dmg, best: best, crits: crits, fumbles: fumbles,
             checks: checks, spells: Object.keys(spells) };
  }

  // ---------------------------------------------------------- drawing it
  var TL_ICON = {
    fight: '<svg viewBox="0 0 24 24"><path d="M3 3h3l12 12v3h-3L3 6z"/>' +
      '<path d="M21 3h-3L6 15v3h3L21 6z"/></svg>',
    note: '<svg viewBox="0 0 24 24"><path d="M5 3h9l5 5v13H5zM13 3.8V9h5.2"/></svg>',
    roll: '<svg viewBox="0 0 24 24"><path d="M12 2 3 7v10l9 5 9-5V7zm0 2.4 6.4 3.6' +
      'L12 11.6 5.6 8z"/></svg>',
    rolls: '<svg viewBox="0 0 24 24"><path d="M4 6h16v2H4zm0 5h16v2H4zm0 5h10v2H4z"/></svg>'
  };

  function tlWhen(mins) {
    if (mins < 1) { return 'a moment'; }
    if (mins < 60) { return mins + ' min'; }
    var h = Math.floor(mins / 60), m = mins % 60;
    return h + 'h' + (m ? ' ' + m + 'm' : '');
  }

  function openTimeline() {
    // The one being recorded, or the one that was being looked at, or the
    // newest there is.
    var want = TL.id || (LOG.session && LOG.session.id) || '';
    if (TL.list) { loadTimeline(want, false); return; }
    TL.busy = true;
    renderTimeline();
    api('GET', '/dnd/play/sessions' + sessionsQuery()).then(function (list) {
      TL.list = list || [];
      TL.busy = false;
      if (!want && TL.list.length) { want = TL.list[0].id; }
      loadTimeline(want, false);
    }, function (err) {
      TL.busy = false;
      TL.error = err.message || String(err);
      renderTimeline();
    });
  }

  function loadTimeline(id, force) {
    TL.id = id || '';
    TL.error = '';
    if (!TL.id) { TL.detail = null; renderTimeline(); return; }
    if (!force && TL.cache[TL.id]) {
      TL.detail = TL.cache[TL.id];
      renderTimeline();
      return;
    }
    TL.busy = true;
    TL.detail = null;
    renderTimeline();
    api('GET', '/dnd/play/session/' + encodeURIComponent(TL.id)).then(function (d) {
      TL.busy = false;
      TL.cache[TL.id] = d;
      TL.detail = d;
      renderTimeline();
    }, function (err) {
      TL.busy = false;
      TL.error = err.message || String(err);
      renderTimeline();
    });
  }

  function renderTimeline() {
    if (!drawer) { return; }
    var pick = drawer.querySelector('#tl-pick');
    var body = drawer.querySelector('#tl-body');
    if (!body) { return; }
    if (pick) {
      pick.innerHTML = (TL.list || []).map(function (sv) {
        var live = LOG.session && LOG.session.id === sv.id;
        return '<option value="' + esc(sv.id) + '"' +
          (sv.id === TL.id ? ' selected' : '') + '>' +
          (live ? '● ' : '') + esc(sv.name || sv.id) +
          (sv.date ? ' · ' + esc(sv.date) : '') + '</option>';
      }).join('') || '<option value="">no sessions yet</option>';
    }
    if (TL.error) {
      body.innerHTML = '<div class="nd-err">' + esc(TL.error) + '</div>';
      return;
    }
    if (TL.busy) { body.innerHTML = '<div class="dt-empty">Reading the night back&hellip;</div>'; return; }
    if (!TL.detail) {
      body.innerHTML = '<div class="dt-empty">No session to show. Start one from ' +
        'the session panel and the evening will draw itself here.</div>';
      return;
    }

    var d = TL.detail;
    var shape = tlEvents(d);
    if (!shape.events.length) {
      body.innerHTML = '<div class="dt-empty">Nothing has happened yet tonight.</div>';
      return;
    }

    var head = '<div class="tl-title"><b>' + esc(d.name || d.id) + '</b>' +
      (d.summary ? '<span>' + esc(d.summary) + '</span>' : '') + '</div>';

    var prev = null;
    var out = shape.events.map(function (ev) {
      var gap = '';
      if (prev !== null && ev.abs - prev >= TL_SCENE) {
        gap = '<div class="tl-gap"><span>' + tlWhen(ev.abs - prev) + ' later</span></div>';
      }
      prev = ev.kind === 'fight' ? ev.until : ev.abs;
      return gap + tlCard(ev);
    }).join('');

    body.innerHTML = head + '<div class="tl">' + out +
      '<div class="tl-end"><span>' + shape.events.length + ' ' +
      (shape.events.length === 1 ? 'moment' : 'moments') + '</span></div></div>';
  }

  function tlCard(ev) {
    if (ev.kind === 'fight') { return tlFightCard(ev); }
    if (ev.kind === 'note') { return tlNoteCard(ev); }
    if (ev.kind === 'roll') { return tlRollCard(ev); }
    return tlRollsCard(ev);
  }

  function tlDot(kind, extra) {
    return '<span class="tl-dot ' + kind + (extra || '') + '">' +
      (TL_ICON[kind] || '') + '</span>';
  }

  function tlRow(kind, time, inner, extra) {
    return '<div class="tl-row ' + kind + (extra || '') + '">' +
      '<time>' + esc(time || '') + '</time>' + tlDot(kind, extra) +
      '<div class="tl-card">' + inner + '</div></div>';
  }

  function tlChip(label, value, cls) {
    return '<span class="tl-chip' + (cls ? ' ' + cls : '') + '">' +
      '<b>' + esc(String(value)) + '</b> ' + esc(label) + '</span>';
  }

  function tlFightCard(ev) {
    var chips = tlChip(ev.rolls.length === 1 ? 'roll' : 'rolls', ev.rolls.length);
    if (ev.damage) { chips += tlChip('damage', ev.damage, 'hurt'); }
    if (ev.crits) { chips += tlChip(ev.crits === 1 ? 'crit' : 'crits', ev.crits, 'crit'); }
    if (ev.fumbles) { chips += tlChip(ev.fumbles === 1 ? 'fumble' : 'fumbles', ev.fumbles, 'fumble'); }
    if (ev.mins) { chips += tlChip('', tlWhen(ev.mins), 'quiet'); }

    var best = ev.best
      ? '<div class="tl-best">Worst of it: <b>' + esc(ev.best.row.label || 'a hit') +
        '</b> for <b>' + esc(String(ev.best.row.result)) + '</b></div>'
      : '';
    var spells = ev.spells.length
      ? '<div class="tl-spells">' + ev.spells.slice(0, 8).map(function (nm) {
          return '<span>' + esc(nm) + '</span>'; }).join('') +
        (ev.spells.length > 8 ? '<span class="more">+' + (ev.spells.length - 8) +
          '</span>' : '') + '</div>'
      : '';
    var beats = ev.beats.length
      ? '<div class="tl-beats">' + ev.beats.map(function (b) {
          return '<div class="tl-beat"><time>' + esc(b.time) + '</time>' +
            '<div class="org-rich">' + orgToHtml(b.text) + '</div></div>';
        }).join('') + '</div>'
      : '';
    var open = TL.open[ev.key];
    var detail = (open || TL.everything)
      ? '<div class="tl-blow">' + ev.rolls.map(tlLine).join('') + '</div>' : '';

    return tlRow('fight', ev.time,
      '<div class="tl-head"><b>Combat</b>' +
        '<span class="tl-span">' + esc(ev.time) +
          (ev.endTime && ev.endTime !== ev.time ? '&ndash;' + esc(ev.endTime) : '') +
        '</span></div>' +
      '<div class="tl-chips">' + chips + '</div>' + best + spells + beats +
      '<button type="button" class="tl-more" data-tl-open="' + esc(ev.key) + '">' +
        (open ? 'hide the rolls' : 'all ' + ev.rolls.length + ' rolls') + '</button>' +
      detail,
      ev.crits ? ' hot' : '');
  }

  function tlLine(it) {
    var nat = it.nat === 20 ? ' crit' : it.nat === 1 ? ' fumble' : '';
    return '<div class="tl-line' + nat + '"><time>' + esc(it.time) + '</time>' +
      '<span class="nm">' + esc(it.row.label || '') + '</span>' +
      '<span class="fm">' + esc(it.row.formula || '') + '</span>' +
      '<b>' + esc(String(it.row.result || '')) + '</b></div>';
  }

  function tlNoteCard(ev) {
    return tlRow('note', ev.time,
      '<div class="org-rich">' + orgToHtml(ev.note.text) + '</div>');
  }

  function tlRollCard(ev) {
    var nat = ev.item.nat;
    return tlRow('roll', ev.time,
      '<div class="tl-head"><b>' + esc(ev.item.row.label || 'A roll') + '</b>' +
        '<span class="tl-span">' + (nat === 20 ? 'natural 20' : 'natural 1') +
        '</span></div>' +
      '<div class="tl-chips">' + tlChip('rolled', ev.item.row.result || '?',
        nat === 20 ? 'crit' : 'fumble') +
        tlChip('', ev.item.row.formula || '', 'quiet') + '</div>',
      nat === 20 ? ' hot' : ' cold');
  }

  function tlRollsCard(ev) {
    // One roll on its own says everything it has to say in a line. Giving
    // it a heading, a count and a button to unfold it makes three lines of
    // furniture round one number.
    if (ev.items.length === 1) {
      var it = ev.items[0];
      return tlRow('rolls', ev.time,
        '<div class="tl-one">' +
          '<span class="nm">' + esc(it.row.label || 'a roll') + '</span>' +
          '<span class="fm">' + esc(it.row.formula || '') + '</span>' +
          '<b>' + esc(String(it.row.result || '')) + '</b></div>');
    }
    var open = TL.open[ev.key] || TL.everything;
    var names = {}, i;
    ev.items.forEach(function (it) { names[it.row.label || 'a roll'] = 1; });
    var list = Object.keys(names);
    return tlRow('rolls', ev.time,
      '<div class="tl-head"><b>' + ev.items.length + ' ' +
        (ev.items.length === 1 ? 'roll' : 'rolls') + '</b>' +
        '<span class="tl-span">' + esc(list.slice(0, 4).join(', ')) +
        (list.length > 4 ? '…' : '') + '</span></div>' +
      (open ? '<div class="tl-blow">' + ev.items.map(tlLine).join('') + '</div>' : '') +
      '<button type="button" class="tl-more" data-tl-open="' + esc(ev.key) + '">' +
        (open ? 'hide' : 'show them') + '</button>');
  }


  // ========================================================= combat tracker
  //
  // Initiative, rounds, turns, and the things that run out.
  //
  // This is the one part of the sheet that is not about the character: it is
  // about the table, and everybody at it. So it is kept here in the browser
  // and written nowhere - a fight belongs to the hour it is fought in, and
  // the character's org file has no business carrying six goblins around in
  // it afterwards. It does survive a reload, because a laptop that goes to
  // sleep in the middle of a fight should not lose the order of it.
  //
  // What counts down and when is the thing worth getting right. An effect
  // measured in rounds ticks at the start of its owner's turn and is gone
  // when it reaches zero, which is where the rules put "until the end of
  // your next turn". A countdown on the table itself - the lair action, the
  // collapsing bridge, how long until the ritual finishes - ticks once at
  // the top of each round instead, because it belongs to nobody's turn.
  var CBT_KEY = 'orgs.dnd.combat';
  var CBT = { on: false, round: 0, turn: 0, who: [], clocks: [], expired: [] };

  function combatLoad() {
    try {
      var raw = JSON.parse(localStorage.getItem(CBT_KEY) || 'null');
      if (raw && typeof raw === 'object') {
        CBT.on = !!raw.on;
        CBT.round = raw.round || 0;
        CBT.turn = raw.turn || 0;
        CBT.who = Array.isArray(raw.who) ? raw.who : [];
        CBT.clocks = Array.isArray(raw.clocks) ? raw.clocks : [];
      }
    } catch (e) { /* nothing worth doing about it */ }
  }

  function combatSave() {
    try {
      localStorage.setItem(CBT_KEY, JSON.stringify({
        on: CBT.on, round: CBT.round, turn: CBT.turn,
        who: CBT.who, clocks: CBT.clocks }));
    } catch (e) { /* private browsing */ }
  }

  var cbtSeq = 0;
  function cbtId() { return 'c' + (++cbtSeq) + '-' + CBT.who.length; }

  // Initiative order: highest first, and a tie is broken by whoever has the
  // better Dexterity - which the sheet only knows for its own character, so
  // everybody else falls back to the order they were entered in. That is the
  // same answer a table reaches by asking "who's got the higher dex?" and
  // then giving up and going round the table.
  function combatOrder() {
    CBT.who.sort(function (a, b) {
      if (b.init !== a.init) { return b.init - a.init; }
      if (a.mine !== b.mine) { return a.mine ? -1 : 1; }
      return (a.seq || 0) - (b.seq || 0);
    });
  }

  function combatCurrent() {
    if (!CBT.on || !CBT.who.length) { return null; }
    return CBT.who[CBT.turn % CBT.who.length] || null;
  }

  function combatStart() {
    if (!CBT.who.length) { return; }
    combatOrder();
    CBT.on = true;
    CBT.round = 1;
    CBT.turn = 0;
    CBT.expired = [];
    combatTick(CBT.who[0], true);
    combatSave();
    renderCombat();
  }

  function combatEnd() {
    CBT.on = false;
    CBT.round = 0;
    CBT.turn = 0;
    CBT.expired = [];
    // Effects measured in rounds have no meaning outside a fight.
    CBT.who.forEach(function (w) { w.fx = []; });
    CBT.clocks = [];
    combatSave();
    renderCombat();
  }

  // Everything one combatant owns ticks by a round, and whatever runs out
  // is remembered so the sheet can say so rather than silently dropping it.
  function combatTick(w, quiet) {
    if (!w || !w.fx) { return; }
    w.fx = w.fx.filter(function (f) {
      if (f.rounds === null || f.rounds === undefined) { return true; }
      f.rounds -= 1;
      if (f.rounds > 0) { return true; }
      CBT.expired.push({ who: w.name, text: f.text });
      return false;
    });
    if (!quiet) { /* the caller saves */ }
  }

  function combatStep(by) {
    if (!CBT.on || !CBT.who.length) { return; }
    CBT.expired = [];
    var n = CBT.who.length;
    var next = CBT.turn + by;
    if (next >= n) {
      next = 0;
      CBT.round += 1;
      // The table's own clocks run at the top of the round.
      CBT.clocks = CBT.clocks.filter(function (c) {
        c.rounds -= 1;
        if (c.rounds > 0) { return true; }
        CBT.expired.push({ who: '', text: c.text });
        return false;
      });
    } else if (next < 0) {
      // Going back is a correction, not a rewind: a turn undone does not
      // hand back the round an effect already spent, because nobody can
      // say which of them it was. Round numbers do go back.
      next = n - 1;
      CBT.round = Math.max(1, CBT.round - 1);
      combatSave();
      CBT.turn = next;
      renderCombat();
      return;
    }
    CBT.turn = next;
    if (by > 0) { combatTick(CBT.who[next]); }
    combatSave();
    renderCombat();
  }

  function combatAdd() {
    var box = drawer.querySelector('#cbt-name');
    var initBox = drawer.querySelector('#cbt-init');
    var hpBox = drawer.querySelector('#cbt-hp');
    if (!box) { return; }
    var name = box.value.trim();
    if (!name) { box.focus(); return; }
    var init = parseInt(initBox.value, 10);
    var hp = parseInt(hpBox.value, 10);
    // "Goblin x4" is how a DM actually types four goblins.
    var many = /\s*[x×]\s*(\d{1,2})\s*$/i.exec(name);
    var count = 1;
    if (many) { count = Math.min(20, parseInt(many[1], 10)); name = name.slice(0, many.index).trim(); }
    for (var i = 0; i < count; i++) {
      CBT.who.push({
        id: cbtId(), seq: CBT.who.length,
        name: count > 1 ? name + ' ' + (i + 1) : name,
        init: isNaN(init) ? 0 : init,
        hp: isNaN(hp) ? null : hp, maxHp: isNaN(hp) ? null : hp,
        mine: false, out: false, fx: [] });
    }
    box.value = '';
    initBox.value = '';
    hpBox.value = '';
    if (CBT.on) { combatOrder(); }
    combatSave();
    renderCombat();
    drawer.querySelector('#cbt-name').focus();
  }

  // The character whose sheet this is, added with whatever the hit point
  // bar currently says, so the row tracks the same number the sheet does.
  function combatAddMe() {
    if (CBT.who.some(function (w) { return w.mine; })) { return; }
    var hp = null, max = null;
    if (typeof HEALTH === 'object' && HEALTH && HEALTH.state && HEALTH.state.hp) {
      hp = HEALTH.state.hp.current;
      max = HEALTH.state.hp.max;
    }
    if (typeof hp !== 'number') { hp = null; max = null; }
    CBT.who.push({ id: cbtId(), seq: CBT.who.length, name: CHARACTER || 'Me',
                   init: 0, hp: hp, maxHp: max, mine: true, out: false, fx: [] });
    combatSave();
    renderCombat();
  }

  // Roll this character's initiative on the sheet itself, so it goes through
  // the same dice, the same conditions and the same tray as every other
  // roll - and lands in the session log like one, because it is one.
  function combatRollMine() {
    var tile = document.querySelector('.rollable[data-label="Initiative"]');
    var mine = CBT.who.filter(function (w) { return w.mine; })[0];
    if (!mine) { combatAddMe(); mine = CBT.who.filter(function (w) { return w.mine; })[0]; }
    if (!tile || !mine) { return; }
    var spec = specFor(tile);
    if (!spec) { return; }
    roll(spec, originOf(tile, null), function (r) {
      mine.init = d20Value(r, d20At(r));
      if (CBT.on) { combatOrder(); }
      combatSave();
      renderCombat();
    });
  }

  function combatFind(id) {
    return CBT.who.filter(function (w) { return w.id === id; })[0] || null;
  }

  function combatAddEffect(id) {
    var w = combatFind(id);
    var row = drawer.querySelector('[data-who="' + id + '"]');
    if (!w || !row) { return; }
    var text = row.querySelector('.cbt-fx-text').value.trim();
    var rounds = parseInt(row.querySelector('.cbt-fx-rounds').value, 10);
    if (!text) { return; }
    w.fx.push({ text: text, rounds: isNaN(rounds) || rounds < 1 ? null : rounds });
    row.querySelector('.cbt-fx-text').value = '';
    row.querySelector('.cbt-fx-rounds').value = '';
    combatSave();
    renderCombat();
    var again = drawer.querySelector('[data-who="' + id + '"] .cbt-fx-text');
    if (again) { again.focus(); }
  }

  function combatAddClock() {
    var text = drawer.querySelector('#cbt-clock-text');
    var rounds = drawer.querySelector('#cbt-clock-rounds');
    if (!text || !text.value.trim()) { return; }
    var n = parseInt(rounds.value, 10);
    CBT.clocks.push({ text: text.value.trim(), rounds: isNaN(n) || n < 1 ? 1 : n });
    text.value = '';
    rounds.value = '';
    combatSave();
    renderCombat();
    drawer.querySelector('#cbt-clock-text').focus();
  }

  function combatClick(e) {
    var b = e.target.closest('button');
    if (!b) { return; }
    var id = b.id;
    if (id === 'cbt-add') { combatAdd(); return; }
    if (id === 'cbt-add-me') { combatAddMe(); return; }
    if (id === 'cbt-roll-me') { combatRollMine(); return; }
    if (id === 'cbt-start') { combatStart(); return; }
    if (id === 'cbt-end') { combatEnd(); return; }
    if (id === 'cbt-next') { combatStep(1); return; }
    if (id === 'cbt-prev') { combatStep(-1); return; }
    if (id === 'cbt-clear') { CBT.who = []; CBT.clocks = []; combatEnd(); return; }
    if (id === 'cbt-clock-add') { combatAddClock(); return; }
    if (b.classList.contains('cbt-fx-add')) {
      combatAddEffect(b.closest('[data-who]').getAttribute('data-who'));
      return;
    }
    if (b.classList.contains('cbt-fx-x')) {
      var w1 = combatFind(b.closest('[data-who]').getAttribute('data-who'));
      if (w1) { w1.fx.splice(parseInt(b.getAttribute('data-fx'), 10), 1); }
      combatSave();
      renderCombat();
      return;
    }
    if (b.classList.contains('cbt-clock-x')) {
      CBT.clocks.splice(parseInt(b.getAttribute('data-clock'), 10), 1);
      combatSave();
      renderCombat();
      return;
    }
    if (b.classList.contains('cbt-x')) {
      var gone = b.closest('[data-who]').getAttribute('data-who');
      CBT.who = CBT.who.filter(function (w) { return w.id !== gone; });
      if (CBT.turn >= CBT.who.length) { CBT.turn = 0; }
      combatSave();
      renderCombat();
      return;
    }
    if (b.classList.contains('cbt-down')) {
      var w2 = combatFind(b.closest('[data-who]').getAttribute('data-who'));
      if (w2) { w2.out = !w2.out; }
      combatSave();
      renderCombat();
      return;
    }
    if (b.classList.contains('cbt-hurt') || b.classList.contains('cbt-mend')) {
      var row = b.closest('[data-who]');
      var w3 = combatFind(row.getAttribute('data-who'));
      var box = row.querySelector('.cbt-amt');
      var amt = parseInt(box.value, 10);
      if (!w3 || isNaN(amt) || w3.hp === null) { return; }
      w3.hp += b.classList.contains('cbt-mend') ? amt : -amt;
      if (w3.maxHp && w3.hp > w3.maxHp) { w3.hp = w3.maxHp; }
      if (w3.hp <= 0) { w3.hp = 0; w3.out = true; }
      box.value = '';
      combatSave();
      renderCombat();
    }
  }

  function combatChange(e) {
    var row = e.target.closest('[data-who]');
    if (!row) { return; }
    var w = combatFind(row.getAttribute('data-who'));
    if (!w) { return; }
    if (e.target.classList.contains('cbt-init-edit')) {
      w.init = parseInt(e.target.value, 10) || 0;
      if (CBT.on) { combatOrder(); }
      combatSave();
      renderCombat();
    }
  }

  // ------------------------------------------------------------ drawing it
  function renderCombat() {
    if (!drawer) { return; }
    var body = drawer.querySelector('#cbt-body');
    if (!body) { return; }
    var cur = combatCurrent();

    var bar = CBT.on
      ? '<div class="cbt-bar on">' +
          '<div class="cbt-round"><em>Round</em><b>' + CBT.round + '</b></div>' +
          '<div class="cbt-now">' +
            '<em>' + (cur ? 'now' : '') + '</em>' +
            '<b>' + esc(cur ? cur.name : '—') + '</b>' +
            '<span>' + esc(combatNext()) + '</span>' +
          '</div>' +
          '<button type="button" class="dt-btn" id="cbt-prev">&larr; Back</button>' +
          '<button type="button" class="dc-roll" id="cbt-next">Next turn &rarr;</button>' +
          '<button type="button" class="dt-btn" id="cbt-end">End</button>' +
        '</div>'
      : '<div class="cbt-bar">' +
          '<div class="cbt-round"><em>Not in combat</em>' +
            '<b>' + CBT.who.length + '</b></div>' +
          '<div class="cbt-now"><em>ready</em><b>' +
            (CBT.who.length ? 'Roll for initiative' : 'Add who is here') + '</b>' +
            '<span>Everyone below, highest initiative first.</span></div>' +
          '<button type="button" class="dc-roll" id="cbt-start"' +
            (CBT.who.length ? '' : ' disabled') + '>Begin combat</button>' +
        '</div>';

    var gone = CBT.expired.length
      ? '<div class="cbt-gone">' + CBT.expired.map(function (x) {
          return '<span>' + (x.who ? esc(x.who) + ': ' : '') + esc(x.text) +
            ' ran out</span>'; }).join('') + '</div>'
      : '';

    var add = '<div class="cbt-add">' +
      '<input type="text" id="cbt-name" class="nd-input" placeholder="Who? (Goblin x4 works)" ' +
        'autocomplete="off">' +
      '<input type="number" id="cbt-init" class="nd-input cbt-num" placeholder="init">' +
      '<input type="number" id="cbt-hp" class="nd-input cbt-num" placeholder="hp">' +
      '<button type="button" class="dt-btn" id="cbt-add">Add</button>' +
      (CBT.who.some(function (w) { return w.mine; })
        ? '<button type="button" class="dt-btn" id="cbt-roll-me">Roll mine</button>'
        : '<button type="button" class="dt-btn" id="cbt-add-me">Add me</button>') +
      (CBT.who.length ? '<button type="button" class="dt-btn dt-x" id="cbt-clear" ' +
        'title="Clear everyone">&times;</button>' : '') +
      '</div>';

    var list = CBT.who.length
      ? '<ol class="cbt-list">' + CBT.who.map(function (w, i) {
          return combatRow(w, i, cur);
        }).join('') + '</ol>'
      : '<div class="dt-empty">Nobody here yet. Add the party and whatever ' +
        'they walked into, then begin.</div>';

    var clocks = '<div class="cbt-clocks">' +
      '<div class="cbt-sub">Countdowns</div>' +
      (CBT.clocks.length
        ? '<ul>' + CBT.clocks.map(function (c, i) {
            return '<li' + (c.rounds <= 1 ? ' class="soon"' : '') + '>' +
              '<b>' + c.rounds + '</b><span>' + esc(c.text) + '</span>' +
              '<button type="button" class="cbt-clock-x dt-x" data-clock="' + i +
                '" aria-label="Remove">&times;</button></li>';
          }).join('') + '</ul>'
        : '<p class="cbt-hint">Anything on the table’s own clock: the lair ' +
          'action, the burning rope, how long until the ritual lands. These ' +
          'tick once at the top of every round.</p>') +
      '<div class="cbt-add">' +
        '<input type="text" id="cbt-clock-text" class="nd-input" ' +
          'placeholder="What is running out?" autocomplete="off">' +
        '<input type="number" id="cbt-clock-rounds" class="nd-input cbt-num" ' +
          'placeholder="rds">' +
        '<button type="button" class="dt-btn" id="cbt-clock-add">Add</button>' +
      '</div></div>';

    body.innerHTML = bar + gone + add + list + clocks;
  }

  function combatNext() {
    if (!CBT.on || CBT.who.length < 2) { return ''; }
    var nxt = CBT.who[(CBT.turn + 1) % CBT.who.length];
    return nxt ? 'then ' + nxt.name : '';
  }

  function combatRow(w, i, cur) {
    var now = cur && cur.id === w.id;
    var hp = w.hp === null
      ? '<span class="cbt-hp none">&mdash;</span>'
      : '<span class="cbt-hp' + (w.maxHp && w.hp <= w.maxHp / 4 ? ' low' : '') + '">' +
        w.hp + (w.maxHp ? '<em>/' + w.maxHp + '</em>' : '') + '</span>';
    var fx = w.fx && w.fx.length
      ? '<div class="cbt-fx">' + w.fx.map(function (f, k) {
          return '<span class="cbt-tag' + (f.rounds === 1 ? ' soon' : '') + '">' +
            (f.rounds ? '<b>' + f.rounds + '</b>' : '') + esc(f.text) +
            '<button type="button" class="cbt-fx-x" data-fx="' + k +
              '" aria-label="Remove">&times;</button></span>';
        }).join('') + '</div>'
      : '';
    return '<li class="cbt-row' + (now ? ' now' : '') + (w.out ? ' out' : '') +
        (w.mine ? ' mine' : '') + '" data-who="' + esc(w.id) + '">' +
      '<div class="cbt-main">' +
        '<input type="number" class="cbt-init-edit" value="' + w.init +
          '" aria-label="Initiative for ' + esc(w.name) + '">' +
        '<span class="cbt-name">' + esc(w.name) + '</span>' + hp +
        '<input type="number" class="nd-input cbt-amt" placeholder="0" ' +
          'aria-label="Amount">' +
        '<button type="button" class="dt-btn cbt-hurt"' +
          (w.hp === null ? ' disabled' : '') + '>Hurt</button>' +
        '<button type="button" class="dt-btn cbt-mend"' +
          (w.hp === null ? ' disabled' : '') + '>Heal</button>' +
        '<button type="button" class="dt-btn cbt-down" title="Out of the fight">' +
          (w.out ? 'Up' : 'Down') + '</button>' +
        '<button type="button" class="dt-btn dt-x cbt-x" aria-label="Remove">&times;</button>' +
      '</div>' + fx +
      '<div class="cbt-fx-add-row">' +
        '<input type="text" class="nd-input cbt-fx-text" ' +
          'placeholder="Condition or effect" autocomplete="off">' +
        '<input type="number" class="nd-input cbt-num cbt-fx-rounds" placeholder="rds">' +
        '<button type="button" class="dt-btn cbt-fx-add">Track</button>' +
      '</div></li>';
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
    openDetail = d;
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
          (notes.length ? notes.map(function (n, i) {
            return noteCard(n, i, d.id, !!LOG.url);
          }).join('') : '<div class="dt-empty">No notes.</div>') +
        '</div>' +
        '<div class="nd-col"><div class="nd-sub">Rolls</div>' +
          rollTable(rolls, d.id) + '</div>' +
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
          '<button type="button" class="nd-tab" data-view="timeline">Timeline</button>' +
          '<button type="button" class="nd-tab" data-view="combat">Combat</button>' +
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
        '<div class="nd-view" id="view-timeline">' +
          '<div class="nd-toolbar">' +
            '<select id="tl-pick" class="nd-input nd-pick"></select>' +
            '<button type="button" class="dt-btn" id="tl-refresh">Refresh</button>' +
            '<label class="tl-opt"><input type="checkbox" id="tl-all"> ' +
              'every roll</label>' +
            '<span class="nd-meta">The night as it happened.</span>' +
          '</div>' +
          '<div id="tl-body"></div>' +
        '</div>' +
        '<div class="nd-view" id="view-combat">' +
          '<div id="cbt-body"></div>' +
        '</div>' +
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
    // Editing a note that is already in the file: the card opens up, and
    // saving writes that one entry back through the server.
    drawer.addEventListener('click', function (e) {
      if (e.target.closest('.roll-del-no')) { unaskRollDelete(e.target); return; }
      if (e.target.closest('.roll-del.yes')) { doRollDelete(e.target); return; }
      if (e.target.closest('.roll-del')) { askRollDelete(e.target); return; }
      if (e.target.closest('.note-del-no')) { unaskNoteDelete(e.target); return; }
      if (e.target.closest('.note-del.yes')) { doNoteDelete(e.target); return; }
      if (e.target.closest('.note-del')) { askNoteDelete(e.target); return; }
      if (e.target.closest('.note-edit')) { openNoteEditor(e.target); return; }
      if (e.target.closest('.note-edit-cancel')) { closeNoteEditor(e.target); return; }
      if (e.target.closest('.note-edit-save')) { saveNoteEdit(e.target); }
    });
    drawer.addEventListener('keydown', function (e) {
      if (!e.target.closest || !e.target.closest('.note-editor')) { return; }
      if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
        e.preventDefault();
        saveNoteEdit(e.target);
        return;
      }
      if (e.key === 'Escape') {
        e.preventDefault();
        e.stopPropagation();
        closeNoteEditor(e.target);
      }
    });

    drawer.querySelector('.nd-tabs').addEventListener('click', function (e) {
      var b = e.target.closest('.nd-tab');
      if (!b) { return; }
      var name = b.getAttribute('data-view');
      setView(name);
      if (name === 'sessions') { loadSessions(); }
      if (name === 'timeline') { openTimeline(); }
      if (name === 'combat') { renderCombat(); }
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

    drawer.querySelector('#tl-refresh').addEventListener('click', function () {
      TL.cache = {};
      loadTimeline(TL.id, true);
    });
    drawer.querySelector('#tl-pick').addEventListener('change', function (e) {
      TL.id = e.target.value;
      loadTimeline(TL.id, false);
    });
    drawer.querySelector('#tl-all').addEventListener('change', function (e) {
      TL.everything = e.target.checked;
      renderTimeline();
    });
    drawer.querySelector('#tl-body').addEventListener('click', function (e) {
      var more = e.target.closest('[data-tl-open]');
      if (!more) { return; }
      var key = more.getAttribute('data-tl-open');
      if (TL.open[key]) { delete TL.open[key]; } else { TL.open[key] = true; }
      renderTimeline();
    });
    drawer.querySelector('#view-combat').addEventListener('click', combatClick);
    drawer.querySelector('#view-combat').addEventListener('keydown', function (e) {
      if (e.key !== 'Enter') { return; }
      if (e.target.id === 'cbt-name' || e.target.id === 'cbt-init' ||
          e.target.id === 'cbt-hp') {
        e.preventDefault();
        combatAdd();
      } else if (e.target.classList.contains('cbt-fx-text') ||
                 e.target.classList.contains('cbt-fx-rounds')) {
        e.preventDefault();
        combatAddEffect(e.target.closest('[data-who]').getAttribute('data-who'));
      }
    });
    drawer.querySelector('#view-combat').addEventListener('change', combatChange);

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
      XP.now = parseInt(cfg.getAttribute('data-xp'), 10) || 0;
      XP.next = parseInt(cfg.getAttribute('data-next-xp'), 10) || 0;
      XP.level = parseInt(cfg.getAttribute('data-level'), 10) || 0;
    }
    if (!LOG.url && location.protocol.indexOf('http') === 0) { LOG.url = location.origin; }
    loadLog();
    combatLoad();
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

  // paintArmorClass rewrites the armour class tile. Wearing a suit of armour
  // or taking one off moves the number and what it is called, and the change
  // is made in the inventory panel, so the tile is repainted from what the
  // server worked out rather than left saying what was true at export time.
  function paintArmorClass(state) {
    if (!state || typeof state.ac !== 'number') { return; }
    var val = document.getElementById('ac-value');
    if (val) { val.textContent = state.ac; }
    var src = document.getElementById('ac-source');
    if (src) { src.textContent = state.acSource || ''; }
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
      paintArmorClass(state);
      invSpreadState(state);
    }, function (err) {
      if (!quiet) { INV.error = err.message || String(err); renderInventory(); }
    });
  }

  // invSpreadState hands the rest of the sheet what an inventory answer knows.
  // The purse, the hit points and the armour class all move when the bag does,
  // and one answer that carries all of it beats three round trips - which is the
  // same reason the armour class has ridden along since the Worn column became
  // a toggle.
  function invSpreadState(state) {
    if (!state) { return; }
    if (state.purse) {
      COIN.state = COIN.state || {};
      COIN.state.purse = state.purse;
      if (state.moneyHistory) { COIN.state.history = state.moneyHistory; }
      COIN.state.name = COIN.state.name || state.name;
      renderCoins();
      // Selling something is coin arriving just as much as picking it up is.
      noteCoinArrivals(state.moneyHistory);
    }
    // Only when the hit points actually moved: every inventory answer carries
    // them, and taking a stale set as news would undo a hit that landed while a
    // potion was being stowed.
    if (state.hp && HEALTH.state && HEALTH.state.hp &&
        (state.hp.current !== HEALTH.state.hp.current ||
         state.hp.temp !== HEALTH.state.hp.temp ||
         state.hp.deathSaves !== HEALTH.state.hp.deathSaves)) {
      HEALTH.state.hp = state.hp;
      HEALTH.msg = state.msg || HEALTH.msg;
      paintHitPoints();
    } else if (state.hp && !HEALTH.state) {
      HEALTH.state = { hp: state.hp, history: [] };
      paintHitPoints();
    }
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
      paintArmorClass(state);
      // Buying and selling move the purse and drinking a potion moves the hit
      // points, and the answer carries both - so the other panels are redrawn
      // from it rather than being left stale until something asks them to load.
      invSpreadState(state);
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
      '</div>' +
      attunementHtml();
  }

  // The Worn cell. Anything that can be worn or wielded gets a button that
  // toggles it; everything else keeps the plain tick it always had. So does
  // anything stowed in a container - something in a backpack is not being
  // worn, which is the same rule that takes gear off when it is packed away,
  // so it has to come out before it can go on.
  // The attunement mark. An item that asks to be attuned to gets a star that
  // is a button: three of them is all anyone has, and an item over that limit
  // does nothing at all, so it is worth being able to swap one for another
  // without editing the equipment table by hand. Anything that does not ask
  // for attunement has no mark, and an item already attuned to keeps its star
  // whether or not the server can be reached.
  function invStar(e) {
    if (!e.attunement) {
      return e.attuned ? ' <span class="worn" title="Attuned">&#9733;</span>' : '';
    }
    var on = !!e.attuned;
    var att = INV.state && INV.state.attunement;
    var full = !on && att && att.used >= att.slots;
    return ' <button type="button" class="attunebtn' + (on ? ' on' : '') +
      (full ? ' full' : '') + '" data-attune="' + (on ? '0' : '1') +
      '" aria-pressed="' + (on ? 'true' : 'false') +
      '" aria-label="' + (on ? 'Attuned, give up attunement to ' : 'Attune to ') +
      esc(e.name) + '" title="' +
      (on ? 'Attuned &mdash; click to give it up'
          : full ? 'Attune to this &mdash; but you have no slots left'
                 : 'Requires attunement &mdash; click to attune') +
      '">&#9733;</button>';
  }

  function invWorn(e) {
    var star = invStar(e);
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

  // The attunement counter over the bag: three slots, what is in them, and a
  // word when the equipment table claims more than it can hold.
  function attunementHtml() {
    var a = INV.state && INV.state.attunement;
    if (!a || !a.slots) { return ''; }
    // Nothing magical and nothing attuned is not worth a line of its own.
    if (!a.used && !a.over && !(a.items && a.items.length)) { return ''; }
    var pips = '';
    for (var i = 1; i <= a.slots; i++) {
      pips += '<span class="att-pip' + (i <= a.used ? ' on' : '') + '"></span>';
    }
    return '<div class="att' + (a.over ? ' over' : '') + '">' +
      '<span class="att-label">Attuned</span>' +
      '<span class="att-pips">' + pips + '</span>' +
      '<span class="att-count">' + a.used + ' of ' + a.slots + '</span>' +
      (a.items && a.items.length
        ? '<span class="att-items">' + a.items.map(esc).join(', ') + '</span>' : '') +
      (a.over
        ? '<span class="att-warn">More marked attuned than you have slots for &mdash; ' +
          'the ones past the third do nothing.</span>' : '') +
      '</div>';
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
      '<td class="inv-actions">' + invActions(e) +
      '</td></tr>';
  }

  // What can be done with one line of the bag. Using something up is the button
  // every consumable has had; what is new is that a potion whose own text says
  // what drinking it does rolls those dice on the table and puts the hit points
  // where they land, all in one press.
  function invActions(e) {
    var out = '';
    if (e.usable !== false) {
      var use = e.use;
      var verb = (use && use.verb) || 'Use';
      var what = useSummary(use);
      out += '<button type="button" class="ib use' + (what ? ' does' : '') +
        '" data-act="use" title="' +
        (what ? esc(verb + ' one: ' + what + ', rolled on the table')
              : 'Use one, recorded in your inventory history') + '">' +
        esc(verb) + (what ? ' <span class="ib-does">' + esc(what) + '</span>' : '') +
        '</button>';
    }
    out += '<button type="button" class="ib" data-act="move" ' +
      'title="Move into another container">Move</button>';
    // Only what the rules put a price on can be sold: the coin for anything
    // else is taken on the coin tab by hand, which the refusal says.
    if (e.sale) {
      out += '<button type="button" class="ib sell" data-act="sell" title="Sell one ' +
        'for ' + esc(e.sale) + ', half what it costs new">Sell</button>';
    }
    out += '<button type="button" class="ib drop" data-act="drop" ' +
      'title="Drop or give away">Drop</button>';
    // Dropping is something the character did and goes in the history.
    // Deleting is a correction to the sheet - a line added by a stray click,
    // or one the table decided was never there - and goes nowhere.
    out += '<button type="button" class="ib gone" data-act="delete" ' +
      'title="Take this line off the sheet altogether. Nothing is written to ' +
      'your inventory history - this is for a line that should never have ' +
      'been here, not for something you are getting rid of.">Del</button>';
    return out;
  }

  // The few words the Use button has room for: the dice it rolls, or the
  // temporary hit points it hands over. A potion that comes in four strengths
  // says so rather than picking one.
  function useSummary(use) {
    if (!use) { return ''; }
    if (use.variants && use.variants.length) { return use.variants.length + ' strengths'; }
    if (use.heal) { return use.heal; }
    if (use.healFlat) { return '+' + use.healFlat; }
    if (use.temp) { return use.temp + ' temp'; }
    return '';
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

  // ----------------------------------------------------- using something up
  //
  // Drinking a potion is one press. The dice its own text asks for are thrown
  // on the table like every other roll, and what they land on goes to the
  // server with the potion being used up - so the bag and the hit points move
  // together, and there is never a moment where the potion is gone and the
  // healing has not happened.
  //
  // A potion the rules print in several strengths - a Potion of Healing is one
  // entry with four rows under it - asks which one is being drunk rather than
  // guessing. Everything the page knows about what an item does came from the
  // server (see consume.go), so a homebrew ruleset that words its potions
  // differently works the same way.
  function useEntry(entry, where, btn) {
    var use = entry.use;
    if (!use) {
      // Nothing the sheet can apply, so this is the plain use it always was.
      invChange({ action: 'use', item: entry.key, name: entry.name, qty: 1,
                  container: where });
      return;
    }
    if (use.variants && use.variants.length) {
      openUsePick(entry, where, btn);
      return;
    }
    useNow(entry, where, btn, use.heal || '', use.healFlat || use.temp || 0,
           useKind(use), '');
  }

  function useKind(use) {
    if (!use) { return ''; }
    if (use.heal || use.healFlat || (use.variants && use.variants.length)) { return 'heal'; }
    if (use.temp) { return 'temp'; }
    return '';
  }

  // useNow rolls what the item asks for and posts the answer. Dice go on the
  // table; a flat number does not need any.
  function useNow(entry, where, btn, dice, flat, effect, variant) {
    var send = function (amount) {
      invChange({ action: 'use', item: entry.key, name: entry.name, qty: 1,
                  container: where, amount: amount, effect: effect,
                  variant: variant || '' });
    };
    if (!dice) { send(flat || 0); return; }
    var parsed = parseDice(dice);
    if (!parsed || !parsed.terms.length) { send(flat || 0); return; }
    roll({ kind: 'damage',
           label: entry.name + (variant ? ' \u00b7 ' + variant : ''),
           formula: dice + (effect === 'temp' ? ' temporary' : ' healing'),
           terms: parsed.terms, flat: parsed.flat,
           // A potion does not crit, however well it is drunk.
           noCrit: true },
         btn ? originOf(btn, null) : { x: window.innerWidth / 2, y: 240 },
         function (result) { send(result.total); });
  }

  // The strength picker, for the potions the rules print as one entry with a
  // table of them. It is the same modal shell the add and move boxes use.
  var invUse, invUseWhat = null;

  function openUsePick(entry, where, btn) {
    if (!invUse) { return; }
    invUseWhat = { entry: entry, where: where, btn: btn };
    invUse.querySelector('#inv-use-title').textContent = 'Drink ' + entry.name;
    invUse.querySelector('#inv-use-list').innerHTML =
      (entry.use.variants || []).map(function (v, i) {
        return '<button type="button" class="inv-hit" data-variant="' + i + '">' +
          '<span class="nm">' + esc(v.name) + '</span>' +
          (v.rarity ? '<span class="rar-tag ' + rarityClass(v.rarity) + '">' +
            esc(v.rarity) + '</span>' : '') +
          '<div class="meta">' + esc(v.heal) + ' hit points</div></button>';
      }).join('');
    invUse.classList.add('open');
  }

  function closeUsePick() {
    if (invUse) { invUse.classList.remove('open'); }
    invUseWhat = null;
  }

  function doUsePick(i) {
    if (!invUseWhat) { return; }
    var what = invUseWhat;
    var v = (what.entry.use.variants || [])[i];
    closeUsePick();
    if (!v) { return; }
    useNow(what.entry, what.where, what.btn, v.heal, 0, 'heal', v.name);
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
    var hit = invHit >= 0 ? invHits[invHit] : null;
    var buy = invAdd.querySelector('#inv-buy-go');
    // Only something the rules price can be bought. Loot and homebrew are
    // added, and the coin for them is settled on the coin tab by hand.
    var cost = hit ? itemPrice(hit) : '';
    if (buy) { buy.hidden = !cost; }
    if (hit) {
      var qty = parseInt(invAdd.querySelector('#inv-qty').value, 10) || 1;
      var price = '';
      if (cost) {
        price = qty > 1
          ? ' &middot; <b>' + esc(cost) + '</b> each, <b>' + esc(invTotalCost(hit)) +
            '</b> for ' + qty
          : ' &middot; <b>' + esc(cost) + '</b>';
      } else {
        price = ' &middot; the rules put no price on it, so it cannot be bought';
      }
      el.innerHTML = 'Adding <b>' + esc(hit.name) + '</b>' + price;
    } else if (typed) {
      el.innerHTML = 'Adding <b>' + esc(typed) + '</b>, which the rules do not know';
    } else {
      el.textContent = '';
    }
  }

  // itemPrice is the cost a search hit carries, or "" for the things the rules
  // price at nothing - which is most of the magic items, and is why an empty
  // price has to mean "not for sale" rather than "free".
  function itemPrice(hit) {
    var cost = String((hit && hit.cost) || '').trim();
    if (!cost || /^0(\.0+)?\s*[a-z]*$/i.test(cost)) { return ''; }
    return cost;
  }

  // What the whole purchase comes to, said in the coins the rules quoted the
  // price in. The server works it out again and is the authority; this is only
  // so the button can say what it is about to spend.
  function invTotalCost(hit) {
    var qty = parseInt(invAdd.querySelector('#inv-qty').value, 10) || 1;
    var cost = itemPrice(hit);
    if (!cost) { return ''; }
    if (qty <= 1) { return cost; }
    var m = /^([\d.]+)\s*([a-z]{2})$/i.exec(cost);
    if (!m) { return qty + ' \u00d7 ' + cost; }
    return (parseFloat(m[1]) * qty) + ' ' + m[2];
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

  function doAdd(paying) {
    var q = invAdd.querySelector('#inv-q').value.trim();
    var qty = parseInt(invAdd.querySelector('#inv-qty').value, 10) || 1;
    var where = invAdd.querySelector('#inv-where').value;
    var hit = invHit >= 0 ? invHits[invHit] : null;
    var action = paying ? 'buy' : 'add';
    // A search that matched nothing is still a thing you picked up: homebrew
    // and loot with no entry in the rules go in under the name as typed. It
    // cannot be bought, though - there is no price to pay.
    var body = hit
      ? { action: action, item: hit.id, name: hit.name, qty: qty, container: where }
      : { action: action, item: '', name: q, qty: qty, container: where };
    if (!hit && !q) {
      invAdd.querySelector('#inv-err').textContent = 'what are you adding?';
      return;
    }
    if (paying && !itemPrice(hit)) {
      invAdd.querySelector('#inv-err').textContent =
        'the rules put no price on that - add it and pay on the coin tab';
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
    invMoveWhat = { key: entry.key, name: entry.name, qty: entry.qty, from: from,
                    sale: entry.sale || '' };
    var title = (mode === 'move' ? 'Move ' : mode === 'sell' ? 'Sell '
                 : mode === 'delete' ? 'Delete ' : 'Drop ') + entry.name;
    invMove.querySelector('#inv-move-title').textContent = title;
    invMove.querySelector('#inv-move-qty').value = mode === 'move' ? entry.qty : 1;
    invMove.querySelector('#inv-move-qty').max = entry.qty;
    invMove.querySelector('#inv-move-max').textContent = 'of ' + entry.qty +
      (mode === 'sell' && entry.sale ? ' \u00b7 ' + entry.sale + ' each' : '');
    invMove.querySelector('#inv-move-err').textContent = '';
    // Deleting takes the whole line, so there is no number to ask for -
    // and the warning matters more than the arithmetic.
    var qtyRow = invMove.querySelector('#inv-move-qty').closest('.inv-row');
    var warn = invMove.querySelector('#inv-move-warn');
    if (qtyRow) { qtyRow.hidden = mode === 'delete'; }
    if (warn) {
      warn.hidden = mode !== 'delete';
      warn.textContent = 'Takes all ' + entry.qty + ' off the sheet. Nothing is ' +
        'written to your inventory history - use Drop for something you are ' +
        'actually getting rid of.';
    }
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
    var go = invMove.querySelector('#inv-move-go');
    go.textContent = mode === 'move' ? 'Move' : mode === 'sell' ? 'Sell'
                     : mode === 'delete' ? 'Delete' : 'Drop';
    go.classList.toggle('gone', mode === 'delete');
    invMove.classList.add('open');
    if (mode === 'delete') { go.focus(); }
    else { invMove.querySelector('#inv-move-qty').focus(); }
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
    } else if (invMode === 'sell') {
      body.action = 'sell';
    } else if (invMode === 'delete') {
      body.action = 'delete';
      // The whole line goes, so whatever the hidden box still says is not
      // part of the question.
      delete body.qty;
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
    // The attunement counter comes over separately, so the three slots are
    // right before the server has been asked anything.
    var attSeed = document.getElementById('dnd-attunement-data');
    if (attSeed && INV.state) {
      try {
        var att = JSON.parse(attSeed.textContent || 'null');
        if (att) { INV.state.attunement = att; }
      } catch (e) { /* the counter waits for the server */ }
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
          // Buying is adding and paying in one call, so the purse cannot end up
          // holding coin for something already in the bag. It only appears for
          // something the rules put a price on.
          '<button type="button" class="inv-go buy" id="inv-buy-go" hidden>Buy</button>' +
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
        '<div class="inv-warn" id="inv-move-warn" hidden></div>' +
        '<div class="inv-row">' +
          '<button type="button" class="inv-go" id="inv-move-go">Move</button>' +
        '</div>' +
        '<div class="inv-err" id="inv-move-err"></div>' +
      '</div>';
    document.body.appendChild(invMove);

    invUse = document.createElement('div');
    invUse.className = 'inv-modal';
    invUse.id = 'inv-use';
    invUse.innerHTML =
      '<div class="inv-card" role="dialog" aria-label="Which strength">' +
        '<h3 id="inv-use-title">Drink' +
          '<button type="button" class="x" id="inv-use-close" aria-label="Close">&times;</button></h3>' +
        '<div class="inv-results" id="inv-use-list"></div>' +
        '<div class="inv-hint">The rules print these as one entry with a table ' +
          'of strengths, so which one you are drinking is yours to say. The ' +
          'dice go on the table and the hit points follow.</div>' +
      '</div>';
    document.body.appendChild(invUse);
    invUse.addEventListener('click', function (e) {
      if (e.target === invUse || e.target.closest('#inv-use-close')) {
        closeUsePick();
        return;
      }
      var hit = e.target.closest('[data-variant]');
      if (hit) { doUsePick(parseInt(hit.getAttribute('data-variant'), 10)); }
    });
    invUse.addEventListener('keydown', function (e) {
      if (e.key === 'Escape') { closeUsePick(); }
    });

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
      var btn = e.target.closest('.ib, .wearbtn, .attunebtn');
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
      if (btn.classList.contains('attunebtn')) {
        invChange({ action: 'attune', item: entry.key, name: entry.name,
                    container: where,
                    attuned: btn.getAttribute('data-attune') === '1' });
        return;
      }
      var act = btn.getAttribute('data-act');
      if (act === 'use') { useEntry(entry, where, btn); return; }
      if (act === 'sell' || act === 'delete') { openMove(act, entry, where); return; }
      openMove(act === 'move' ? 'move' : 'drop', entry, where);
    });

    // A click picks a match rather than adding it outright: an item added by
    // a stray click has to be found and dropped again, which is a worse
    // mistake than one more click. Enter, Add, or a double click adds it.
    invAdd.addEventListener('click', function (e) {
      if (e.target === invAdd || e.target.closest('#inv-add-close')) { closeAdd(); return; }
      if (e.target.closest('#inv-add-go')) { doAdd(false); return; }
      if (e.target.closest('#inv-buy-go')) { doAdd(true); return; }
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
        doAdd(false);
      }
    });
    invAdd.querySelector('#inv-qty').addEventListener('input', invShowPick);
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
      // Enter adds rather than buys. Spending coin should be a button pressed
      // on purpose, not what happens when you finish typing.
      if (e.key === 'Enter') { e.preventDefault(); doAdd(false); }
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
      noteCoinArrivals(state.history);
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
      // Coin coming in gets thrown onto the sheet.
      noteCoinArrivals(state.history);
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

  // -------------------------------------------------------- spell flourish
  //
  // What casting looks like. The eight schools of magic are the oldest piece
  // of flavour the game has and the sheet has never done anything with them,
  // so each one gets its own moment at the button that cast it: evocation
  // throws light outward, conjuration pulls it in, necromancy seeps upward,
  // abjuration puts a ward up.
  //
  // It is flat rather than on the table: a cast happens at the spell in the
  // list, not on the parchment where the dice land, and it has a canvas of
  // its own so a spell that also rolls to hit does not fight the dice for it.
  //
  // Every school is the same handful of particles with different rules about
  // where they start, which way they go and what colour they are. That is
  // deliberate - eight bespoke effects would be eight things to maintain, and
  // the difference the eye actually reads is colour and direction.
  var SCHOOLS = {
    abjuration: {
      colours: ['#bcd8f2', '#e8f2fb', '#8fb4d8'],
      mode: 'ward', count: 14, speed: 60, rings: 3, dur: 900, sides: 6
    },
    conjuration: {
      colours: ['#9fe6c8', '#d8f6e8', '#5fbf9a'],
      mode: 'in', count: 24, speed: 240, rings: 1, dur: 820
    },
    divination: {
      colours: ['#e6e8f0', '#ffffff', '#b9c2d6'],
      mode: 'iris', count: 14, speed: 95, rings: 2, dur: 1000
    },
    enchantment: {
      colours: ['#f7b6d8', '#ffe0ef', '#e07ab0'],
      mode: 'curl', count: 18, speed: 150, rings: 0, dur: 980
    },
    evocation: {
      colours: ['#ffd27a', '#fff3cf', '#ff9a3c'],
      mode: 'out', count: 30, speed: 330, rings: 1, dur: 760
    },
    illusion: {
      colours: ['#cdb4f2', '#ece0ff', '#9a72d8'],
      mode: 'phase', count: 16, speed: 90, rings: 3, dur: 1100
    },
    necromancy: {
      colours: ['#9ab87a', '#6b4a7a', '#3f5a34'],
      mode: 'rise', count: 20, speed: 70, rings: 0, dur: 1150
    },
    transmutation: {
      colours: ['#f0c26a', '#a8d86a', '#e08a3c'],
      mode: 'orbit', count: 20, speed: 180, rings: 1, dur: 1000
    }
  };
  // A spell whose school the rules never said still gets something, because
  // a cast that does nothing at all reads as a broken button.
  var SCHOOL_PLAIN = {
    colours: ['#e8dcc0', '#fff6e2', '#c2ad83'],
    mode: 'out', count: 18, speed: 210, rings: 1, dur: 700
  };

  function SpellBoard(canvas) {
    this.canvas = canvas;
    this.ctx = canvas.getContext('2d');
    this.live = [];
    this.raf = 0;
    this.resize();
    var self = this;
    window.addEventListener('resize', function () { self.resize(); });
  }

  SpellBoard.prototype.resize = function () {
    var dpr = window.devicePixelRatio || 1;
    this.w = this.canvas.clientWidth || window.innerWidth;
    this.h = this.canvas.clientHeight || window.innerHeight;
    this.canvas.width = Math.round(this.w * dpr);
    this.canvas.height = Math.round(this.h * dpr);
    this.ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
  };

  // cast starts one flourish at a point on the screen. Size scales the
  // whole thing down without changing what it is: the moment at the button
  // is full size, and the ones the dice leave as they bounce are a third of
  // it, so a fireball scatters sparks across the table rather than setting
  // off eight more fireballs.
  SpellBoard.prototype.cast = function (school, at, size) {
    var look = SCHOOLS[String(school || '').toLowerCase()] || SCHOOL_PLAIN;
    size = size || 1;
    var now = performance.now();
    var parts = [], i;
    var reach = (34 + Math.random() * 16) * size;
    var count = Math.max(4, Math.round(look.count * size));
    // Only the full sized flourish draws its rings. Shrunk down they are a
    // smudge rather than a ward, and eight of them at once is a mess.
    var rings = size >= 0.7 ? look.rings : 0;
    var dur = look.dur * (0.45 + 0.55 * size);
    // Everything below reads the look through these, so one object carries
    // both what the school is and how big this instance of it is.
    look = { colours: look.colours, mode: look.mode, count: count,
             speed: look.speed * (0.6 + 0.4 * size), rings: rings,
             dur: dur, sides: look.sides };
    for (i = 0; i < look.count; i++) {
      var ang = (i / look.count) * Math.PI * 2 + Math.random() * 0.4;
      var sp = look.speed * (0.7 + Math.random() * 0.6);
      var p = {
        a: ang, sp: sp, r: 0,
        x: at.x, y: at.y, vx: 0, vy: 0,
        size: (1.4 + Math.random() * 2.4) * (0.6 + 0.4 * size),
        c: look.colours[i % look.colours.length],
        born: now + Math.random() * look.dur * 0.22,
        life: look.dur * (0.55 + Math.random() * 0.45),
        wob: (Math.random() - 0.5) * 6
      };
      // Where each school's particles start, which is half of what tells
      // them apart: conjuration arrives from outside, necromancy seeps out
      // of the ground under the spell, everything else begins at the point.
      if (look.mode === 'in') {
        p.r = (60 + Math.random() * 30) * size;
      } else if (look.mode === 'rise') {
        p.x = at.x + (Math.random() - 0.5) * 54 * size;
        p.y = at.y + (10 + Math.random() * 12) * size;
      } else if (look.mode === 'orbit' || look.mode === 'ward') {
        p.r = reach * (0.5 + Math.random() * 0.5);
      }
      parts.push(p);
    }
    this.live.push({ look: look, at: at, t0: now, parts: parts, reach: reach,
                     size: size });
    // Anchored to the page rather than the window, the way the dice and the
    // coin are: a flourish left over a spell in the list should stay over
    // that spell if the reader scrolls.
    if (this.live.length === 1) {
      this.scrollY0 = window.pageYOffset || document.documentElement.scrollTop || 0;
    }
    this.start();
  };

  SpellBoard.prototype.scrollShift = function () {
    return (this.scrollY0 || 0) -
      (window.pageYOffset || document.documentElement.scrollTop || 0);
  };

  SpellBoard.prototype.start = function () {
    if (this.raf) { return; }
    var self = this;
    this.last = performance.now();
    var tick = function (now) {
      self.raf = requestAnimationFrame(tick);
      self.step(now);
    };
    this.raf = requestAnimationFrame(tick);
  };

  SpellBoard.prototype.step = function (now) {
    var dt = Math.min(0.032, (now - this.last) / 1000);
    this.last = now;
    var ctx = this.ctx;
    ctx.clearRect(0, 0, this.w, this.h);
    var self = this;
    ctx.save();
    ctx.translate(0, this.scrollShift());
    this.live = this.live.filter(function (fx) {
      return self.drawOne(ctx, fx, now, dt);
    });
    ctx.restore();
    if (!this.live.length) {
      cancelAnimationFrame(this.raf);
      this.raf = 0;
    }
  };

  SpellBoard.prototype.drawOne = function (ctx, fx, now, dt) {
    var look = fx.look, age = now - fx.t0;
    if (age > look.dur + 300) { return false; }
    var t = Math.min(1, age / look.dur);
    ctx.save();
    ctx.globalCompositeOperation = 'lighter';

    // The rings, which is the other half of what tells the schools apart: a
    // ward is a hard polygon, an iris is a thin circle opening, an illusion
    // is three of them out of step with each other.
    for (var i = 0; i < look.rings; i++) {
      var rt = Math.min(1, Math.max(0, (t - i * 0.14) / 0.72));
      if (rt <= 0 || rt >= 1) { continue; }
      var sz = fx.size || 1;
      var rad = (10 + rt * (look.mode === 'in' ? 70 : 62)) * sz;
      if (look.mode === 'in') { rad = (80 - rt * 66) * sz; }
      var fade = look.mode === 'phase'
        ? Math.abs(Math.sin(rt * Math.PI * 2)) * (1 - rt)
        : (1 - rt) * (1 - rt);
      ctx.strokeStyle = fx.look.colours[i % look.colours.length];
      ctx.globalAlpha = 0.75 * fade;
      ctx.lineWidth = look.mode === 'iris' ? 1.1 : 2;
      ctx.beginPath();
      if (look.sides) {
        for (var k = 0; k <= look.sides; k++) {
          var pa = (k / look.sides) * Math.PI * 2 - Math.PI / 2 + rt * 0.5;
          var px = fx.at.x + Math.cos(pa) * rad, py = fx.at.y + Math.sin(pa) * rad;
          if (k) { ctx.lineTo(px, py); } else { ctx.moveTo(px, py); }
        }
      } else {
        ctx.arc(fx.at.x, fx.at.y, rad, 0, Math.PI * 2);
      }
      ctx.stroke();
    }
    ctx.globalAlpha = 1;

    var alive = false;
    for (var n = 0; n < fx.parts.length; n++) {
      var p = fx.parts[n];
      if (now < p.born) { alive = true; continue; }
      var pt = (now - p.born) / p.life;
      if (pt >= 1) { continue; }
      alive = true;
      // Each mode is three lines: how the particle moves, and everything
      // else is shared.
      switch (look.mode) {
        case 'in':
          p.r = Math.max(0, p.r - p.sp * dt);
          p.x = fx.at.x + Math.cos(p.a) * p.r;
          p.y = fx.at.y + Math.sin(p.a) * p.r;
          break;
        case 'rise':
          p.y -= p.sp * dt;
          p.x += Math.sin((now - p.born) / 220 + p.wob) * 14 * dt;
          break;
        case 'orbit':
          p.a += (p.sp / 90) * dt;
          p.r += 34 * dt;
          p.x = fx.at.x + Math.cos(p.a) * p.r;
          p.y = fx.at.y + Math.sin(p.a) * p.r * 0.7;
          break;
        case 'curl':
          p.a += 2.6 * dt;
          p.r += p.sp * dt * (1 - pt * 0.6);
          p.x = fx.at.x + Math.cos(p.a) * p.r;
          p.y = fx.at.y + Math.sin(p.a) * p.r * 0.8;
          break;
        case 'ward':
        case 'iris':
          p.r += p.sp * dt;
          p.x = fx.at.x + Math.cos(p.a) * p.r;
          p.y = fx.at.y + Math.sin(p.a) * p.r;
          break;
        default:  // out, and anything the rules never named
          p.r += p.sp * dt * (1 - pt * 0.5);
          p.x = fx.at.x + Math.cos(p.a) * p.r;
          p.y = fx.at.y + Math.sin(p.a) * p.r * 0.85;
      }
      var fade = look.mode === 'phase'
        ? Math.abs(Math.sin(pt * Math.PI * 3)) * (1 - pt)
        : 1 - pt * pt;
      ctx.fillStyle = p.c;
      ctx.globalAlpha = 0.9 * fade;
      ctx.beginPath();
      ctx.arc(p.x, p.y, Math.max(0.4, p.size * (1 - pt * 0.5)), 0, Math.PI * 2);
      ctx.fill();
    }
    ctx.restore();
    return alive || t < 1;
  };

  var spellBoard;

  // spellFlourish is what every caller uses. A cast with no school - an item,
  // a feature, a homebrew line - still gets the plain one.
  function spellFlourish(school, origin, size) {
    if (!spellBoard || !origin) { return; }
    spellBoard.cast(school, origin, size);
  }

  // ------------------------------------------------------------ coin spill
  //
  // Coin coming into the purse is worth seeing arrive. A couple of pieces of
  // the right metal drop onto the sheet, bounce, roll and settle - the same
  // world the dice live in, viewport pixels flat on the page with z up, on a
  // layer of its own so a roll and a payment can happen at once.
  //
  // A coin is drawn from its normal: the disc projects to an ellipse whose
  // short axis is how much of the face you can see, and the rim shows as a
  // band on the low side. That is the whole of the 3d - there is no mesh
  // here, because a disc does not need one and the shading of the rim is
  // what actually sells it.
  var COIN_CAM_Z = 1400;
  var COIN_GRAVITY = 3600;

  // What each metal looks like: the two ends of the face gradient, the rim,
  // and how big the piece is. Copper is a little thicker and duller, platinum
  // wider and nearly white, which is how they are told apart at a glance
  // without reading anything.
  var COIN_LOOK = {
    cp: { face: ['#e0a05e', '#8a4a22'], rim: '#6d3a1a', edge: '#c9763c', r: 15, t: 3.6 },
    sp: { face: ['#f2f2ee', '#8f8f88'], rim: '#77776f', edge: '#dcdcd6', r: 15.5, t: 3.0 },
    ep: { face: ['#e8f0e4', '#7f9179'], rim: '#6b7a66', edge: '#d8e2d2', r: 15.5, t: 3.0 },
    gp: { face: ['#ffe28a', '#a9801a', ], rim: '#8a6512', edge: '#e8c34a', r: 16, t: 3.2 },
    pp: { face: ['#ffffff', '#9fabb6'], rim: '#8c98a3', edge: '#eef1f4', r: 17, t: 3.0 }
  };

  function CoinBoard(canvas) {
    this.canvas = canvas;
    this.ctx = canvas.getContext('2d');
    this.coins = [];
    this.raf = 0;
    this.dpr = 1;
    this.resize();
    var self = this;
    window.addEventListener('resize', function () { self.resize(); });
  }

  CoinBoard.prototype.resize = function () {
    var dpr = window.devicePixelRatio || 1;
    this.dpr = dpr;
    this.w = this.canvas.clientWidth || window.innerWidth;
    this.h = this.canvas.clientHeight || window.innerHeight;
    this.canvas.width = Math.round(this.w * dpr);
    this.canvas.height = Math.round(this.h * dpr);
    this.ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
  };

  CoinBoard.prototype.project = function (x, y, z) {
    var s = COIN_CAM_Z / Math.max(120, COIN_CAM_Z - z);
    return [this.w / 2 + (x - this.w / 2) * s, this.h / 2 + (y - this.h / 2) * s, s];
  };

  CoinBoard.prototype.stop = function () {
    if (this.raf) { cancelAnimationFrame(this.raf); this.raf = 0; }
    this.coins = [];
    this.ctx.clearRect(0, 0, this.w, this.h);
  };

  // spill drops coins onto the sheet. `pieces` is a list of {id, n} in the
  // order they should land, and only a handful are ever thrown however much
  // coin changed hands: this is a flourish, not an inventory.
  CoinBoard.prototype.spill = function (pieces, at) {
    var self = this;
    this.resize();
    this.scrollY0 = window.pageYOffset || document.documentElement.scrollTop || 0;
    var now = performance.now();
    var made = [];
    pieces.forEach(function (p, i) {
      var look = COIN_LOOK[p.id];
      if (!look) { return; }
      var tx = at.x + (Math.random() - 0.5) * 120;
      var ty = at.y + (Math.random() - 0.5) * 70;
      tx = Math.max(40, Math.min(self.w - 40, tx));
      ty = Math.max(40, Math.min(self.h - 40, ty));
      // Thrown in from above and a little to one side, so they arrive on an
      // arc rather than dropping straight down like a lift.
      var flight = 0.34 + Math.random() * 0.06;
      var h0 = 420 + Math.random() * 160;
      var sx = tx - 110 - Math.random() * 90;
      var sy = ty - 150 - Math.random() * 80;
      var rest = look.t / 2;
      made.push({
        look: look,
        x: sx, y: sy, z: h0,
        vx: (tx - sx) / flight, vy: (ty - sy) / flight,
        vz: (rest - h0 + 0.5 * COIN_GRAVITY * flight * flight) / flight,
        // Orientation: how far from lying flat, which way the low edge
        // points, and the spin of the face itself.
        tilt: 0.5 + Math.random() * 1.0,
        phase: Math.random() * Math.PI * 2,
        tiltRate: (Math.random() - 0.5) * 16,
        phaseRate: (Math.random() < 0.5 ? -1 : 1) * (7 + Math.random() * 7),
        spin: Math.random() * Math.PI * 2,
        spinRate: (Math.random() - 0.5) * 13,
        rest: rest, bounces: 0, settling: false, done: 0,
        wake: now + i * 95 + Math.random() * 60
      });
    });
    if (!made.length) { return; }
    // A spill already running is joined rather than replaced: two payments in
    // quick succession put more coin on the table, they do not sweep the
    // first lot off it.
    this.coins = this.coins.concat(made);
    this.last = now;
    if (!this.raf) {
      var tick = function (t) {
        self.raf = requestAnimationFrame(tick);
        self.step(t);
      };
      this.raf = requestAnimationFrame(tick);
    }
  };

  CoinBoard.prototype.scrollShift = function () {
    return this.scrollY0 - (window.pageYOffset || document.documentElement.scrollTop || 0);
  };

  CoinBoard.prototype.step = function (now) {
    var dt = Math.min(0.032, (now - this.last) / 1000);
    this.last = now;
    var self = this, alive = [];
    this.coins.forEach(function (c) {
      if (now < c.wake) { alive.push(c); return; }
      if (self.move(c, dt, now)) { alive.push(c); }
    });
    this.coins = alive;

    var ctx = this.ctx;
    ctx.clearRect(0, 0, this.w, this.h);
    if (!this.coins.length) {
      cancelAnimationFrame(this.raf);
      this.raf = 0;
      return;
    }
    ctx.save();
    ctx.translate(0, this.scrollShift());
    // Coin further down the page is drawn later so the near ones overlap the
    // far ones, exactly as the dice do.
    this.coins.slice()
      .sort(function (a, b) { return a.y - b.y; })
      .forEach(function (c) {
        if (now < c.wake) { return; }
        self.drawShadow(ctx, c);
      });
    this.coins.slice()
      .sort(function (a, b) { return a.y - b.y; })
      .forEach(function (c) {
        if (now < c.wake) { return; }
        self.drawCoin(ctx, c);
      });
    ctx.restore();
  };

  // move advances one coin and says whether it is still worth drawing.
  CoinBoard.prototype.move = function (c, dt, now) {
    if (c.done) {
      // Lying still and fading into the paper.
      c.fade = (now - c.done) / 420;
      return c.fade < 1;
    }
    if (!c.settling) {
      c.vz -= COIN_GRAVITY * dt;
      c.x += c.vx * dt;
      c.y += c.vy * dt;
      c.z += c.vz * dt;
      c.tilt += c.tiltRate * dt;
      c.phase += c.phaseRate * dt;
      c.spin += c.spinRate * dt;
      // Keep the tilt in the half turn it means something over: past flat on
      // the other side is the same picture from underneath.
      if (c.tilt < 0) { c.tilt = -c.tilt; c.phase += Math.PI; }
      if (c.tilt > Math.PI / 2) { c.tilt = Math.PI - c.tilt; c.tiltRate = -c.tiltRate; }

      // How low the coin sits is how far over it is: flat on its face it is
      // half a thickness off the paper, up on its edge it is a whole radius.
      var low = c.look.t / 2 * Math.cos(c.tilt) + c.look.r * Math.sin(c.tilt);
      if (c.z <= low) {
        c.z = low;
        if (c.vz < -40) {
          c.bounces++;
          clink(Math.min(1, -c.vz / 700), c.look);
          c.vz = -c.vz * 0.4;
          c.vx *= 0.72; c.vy *= 0.72;
          c.tiltRate *= 0.5;
          c.spinRate *= 0.7;
        } else {
          c.vz = 0;
          // Down and out of speed: the coin now does what a dropped coin
          // does, which is spin down onto its face getting faster and
          // quieter as it goes.
          if (Math.abs(c.vx) + Math.abs(c.vy) < 120) {
            c.settling = true;
            c.settleT = 0;
            c.tilt0 = Math.max(0.35, c.tilt);
            c.clinkAt = 0;
          }
        }
        // Rolling friction, which is much lower on edge than on the face.
        var f = Math.max(0, 1 - (c.tilt > 1.1 ? 1.1 : 5.0) * dt);
        c.vx *= f; c.vy *= f;
      }
      return true;
    }

    // ---- settling: the wobble ---------------------------------------
    // A coin losing the last of its energy lies down while its contact point
    // races round the rim. The tilt falls away and the precession speeds up
    // as it goes, which is the sound and the picture everybody knows.
    c.settleT += dt;
    var k = Math.min(1, c.settleT / 0.85);
    c.tilt = c.tilt0 * (1 - k) * (1 - k);
    c.phase += (9 + 46 * k * k) * dt * (c.phaseRate < 0 ? -1 : 1);
    c.z = c.look.t / 2 * Math.cos(c.tilt) + c.look.r * Math.sin(c.tilt);
    c.x += c.vx * dt; c.y += c.vy * dt;
    c.vx *= Math.max(0, 1 - 7 * dt); c.vy *= Math.max(0, 1 - 7 * dt);
    // The rattle of the last few turns, closer together each time.
    if (k > 0.45 && c.settleT > c.clinkAt) {
      c.clinkAt = c.settleT + 0.075 * (1 - k) + 0.012;
      clink(0.1 + 0.12 * (1 - k), c.look);
    }
    if (k >= 1) {
      c.tilt = 0;
      c.z = c.look.t / 2;
      c.done = now + 260;
    }
    return true;
  };

  CoinBoard.prototype.drawShadow = function (ctx, c) {
    var lift = Math.max(0, c.z - c.look.t / 2);
    var fade = Math.max(0, 1 - lift / 360);
    if (fade <= 0.02) { return; }
    var p = this.project(c.x, c.y, 0);
    var r = c.look.r * (1.05 + lift / 700) * p[2];
    ctx.save();
    ctx.globalAlpha = 0.3 * fade * (1 - (c.fade || 0));
    ctx.fillStyle = '#3a3123';
    ctx.beginPath();
    ctx.ellipse(p[0], p[1] + 2, r, r * 0.42, 0, 0, Math.PI * 2);
    ctx.fill();
    ctx.restore();
  };

  // drawCoin draws the disc from its normal. Tilt is how far it has fallen
  // over, phase is which way, and everything else follows: the face squashes
  // along the phase direction, and the rim appears on the low side of it.
  CoinBoard.prototype.drawCoin = function (ctx, c) {
    var p = this.project(c.x, c.y, c.z);
    var s = p[2], look = c.look;
    var r = look.r * s;
    // cos(tilt) is how much of the face is turned towards the camera, and so
    // is exactly the squash of the ellipse.
    var squash = Math.abs(Math.cos(c.tilt));
    var band = look.t * s * Math.abs(Math.sin(c.tilt));
    var dirX = Math.cos(c.phase), dirY = Math.sin(c.phase);

    ctx.save();
    ctx.globalAlpha = 1 - (c.fade || 0);
    ctx.translate(p[0], p[1]);
    ctx.rotate(c.phase);

    // The rim first, as a second ellipse pushed out along the tilt: what is
    // left showing round the edge of the face is the thickness of the coin.
    if (band > 0.4) {
      var rim = ctx.createLinearGradient(0, -r, 0, r);
      rim.addColorStop(0, look.rim);
      rim.addColorStop(0.5, look.edge);
      rim.addColorStop(1, look.rim);
      ctx.fillStyle = rim;
      ctx.beginPath();
      ctx.ellipse(0, band / 2, r, r * squash + band / 2, 0, 0, Math.PI * 2);
      ctx.fill();
    }

    // Then the face on top of it.
    var g = ctx.createLinearGradient(-r * 0.7, -r * squash, r * 0.7, r * squash);
    g.addColorStop(0, look.face[0]);
    g.addColorStop(0.45, look.face[0]);
    g.addColorStop(1, look.face[1]);
    ctx.fillStyle = g;
    ctx.beginPath();
    ctx.ellipse(0, -band / 2, r, Math.max(0.5, r * squash), 0, 0, Math.PI * 2);
    ctx.fill();

    // A struck coin is not a flat disc: a ring inside the rim and a device in
    // the middle, both squashed with the face, are enough to read as one.
    if (squash > 0.12) {
      ctx.save();
      ctx.translate(0, -band / 2);
      ctx.scale(1, Math.max(0.02, squash));
      ctx.rotate(c.spin);
      ctx.strokeStyle = 'rgba(0,0,0,.22)';
      ctx.lineWidth = Math.max(0.6, 1.1 * s);
      ctx.beginPath();
      ctx.arc(0, 0, r * 0.76, 0, Math.PI * 2);
      ctx.stroke();
      ctx.fillStyle = 'rgba(0,0,0,.18)';
      ctx.beginPath();
      // A rough six pointed device, which at this size reads as "there is
      // something stamped on it" and no more.
      for (var i = 0; i < 12; i++) {
        var a = i * Math.PI / 6;
        var rr = (i % 2 ? r * 0.18 : r * 0.42);
        if (i) { ctx.lineTo(Math.cos(a) * rr, Math.sin(a) * rr); }
        else { ctx.moveTo(Math.cos(a) * rr, Math.sin(a) * rr); }
      }
      ctx.closePath();
      ctx.fill();
      ctx.restore();
    }

    // The glint along the top edge, which is what makes it metal.
    ctx.strokeStyle = 'rgba(255,255,255,.55)';
    ctx.lineWidth = Math.max(0.5, 1 * s);
    ctx.beginPath();
    ctx.ellipse(0, -band / 2, r * 0.95, Math.max(0.4, r * squash * 0.95),
                0, Math.PI * 1.08, Math.PI * 1.72);
    ctx.stroke();
    ctx.restore();
  };

  // ---- what to throw ---------------------------------------------------
  //
  // A payment of 143 gp is not 143 coins on the table. The spill shows what
  // kind of coin came in rather than how much of it: the biggest
  // denominations that changed hands, one piece each, four at the most.
  var COIN_SPILL_MAX = 4;

  function coinPieces(money) {
    var out = [];
    if (!money) { return out; }
    COIN_ORDER.forEach(function (id) {
      var n = money[id] || 0;
      if (n <= 0 || out.length >= COIN_SPILL_MAX) { return; }
      // Two of a kind when there is plenty of it, so a purse of gold looks
      // like more than a purse with one gold piece in it.
      out.push({ id: id });
      if (n > 1 && out.length < COIN_SPILL_MAX) { out.push({ id: id }); }
    });
    return out;
  }

  // COIN_SEEN is how far through the coin history the page has already read.
  // Coin arriving is noticed by a new "gained" line appearing rather than by
  // each caller remembering to say so, because coin comes in from three
  // places - the coin panel, selling something, and undoing a sale going the
  // other way - and only one of them is the coin panel.
  var COIN_SEEN = { n: -1 };

  function noteCoinArrivals(history) {
    if (!history) { return; }
    // The first sight of the log is the page catching up with the file, not
    // news: a sheet opened after a night's looting should be quiet.
    if (COIN_SEEN.n < 0 || history.length <= COIN_SEEN.n) {
      COIN_SEEN.n = history.length;
      return;
    }
    var fresh = history.slice(COIN_SEEN.n);
    COIN_SEEN.n = history.length;
    var got = null;
    fresh.forEach(function (e) {
      if (e && e.action === 'gained' && e.amount) { got = e.amount; }
    });
    if (got) { spillCoins(got); }
  }

  // spillCoins is what every caller uses: hand it the amount that came in and
  // where on the page it came from.
  function spillCoins(money, origin) {
    if (!coinBoard) { return; }
    var pieces = coinPieces(money);
    if (!pieces.length) { return; }
    var at = origin;
    if (!at) {
      // Onto the coin panel when it is on screen, since that is where the
      // number the player is watching has just changed.
      var box = document.getElementById('coin-panel') || coinBox;
      var r = box && box.getBoundingClientRect();
      at = (r && r.width && r.bottom > 0 && r.top < window.innerHeight)
        ? { x: r.left + r.width / 2, y: r.top + Math.min(r.height / 2, 120) }
        : { x: window.innerWidth / 2, y: window.innerHeight * 0.4 };
    }
    coinBoard.spill(pieces, at);
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
      takeAdvice(state);
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
      // A condition switched on changes the very next roll, so the dice are
      // told before anything else happens.
      takeAdvice(state);
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
        if (got && got.conditions) { DEF.state = got; takeAdvice(got); }
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
  var hpCtl, hpAmount, hpType;

  function healthConfigured() { return !!(LOG.file || CHARACTER_ID); }

  // The damage types come from the ruleset, by way of the conditions answer
  // the defenses panel already has - the sheet does not keep a list of its
  // own, because a module that adds a damage type should reach here too.
  function fillHitTypes() {
    if (!hpType) { return; }
    var types = (defState().damageTypes || []);
    if (!types.length) { return; }
    var was = hpType.value;
    hpType.innerHTML = '<option value="">damage</option>' +
      types.map(function (d) {
        return '<option value="' + esc(d.id) + '">' + esc(d.name.toLowerCase()) + '</option>';
      }).join('');
    if (was) { hpType.value = was; }
  }

  function healthLoad(quiet) {
    if (!healthConfigured()) { return Promise.resolve(); }
    var q = '/dnd/hp?filename=' + encodeURIComponent(LOG.file || '') +
      '&id=' + encodeURIComponent(CHARACTER_ID || '');
    return api('GET', q).then(function (state) {
      HEALTH.state = state;
      HEALTH.error = '';
      paintHitPoints();
      // Every hit point answer carries what is being held, so the banner keeps
      // up without a second question.
      if (state.concentration) { concTake(state.concentration); }
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

  // What each kind of damage looks like when it lands. The colour is the one
  // the type has worn on dice and in books for forty years - fire orange,
  // lightning yellow, necrotic a bruised purple - and the temper is how the
  // damage arrives: lightning and a sword snap, fire and acid bloom, cold and
  // the mind-affecting ones creep in and take their time leaving. Anything
  // with weight behind it also rattles the page.
  var DAMAGE_LOOK = {
    acid:        { rgb: '154, 205, 50',  temper: 'bloom' },
    bludgeoning: { rgb: '176, 137, 104', temper: 'snap', jolt: true },
    cold:        { rgb: '142, 208, 240', temper: 'creep' },
    fire:        { rgb: '255, 122, 42',  temper: 'bloom' },
    force:       { rgb: '201, 166, 255', temper: 'snap', jolt: true },
    lightning:   { rgb: '255, 225, 77',  temper: 'snap' },
    necrotic:    { rgb: '122, 74, 138',  temper: 'creep' },
    piercing:    { rgb: '207, 207, 214', temper: 'snap', jolt: true },
    poison:      { rgb: '95, 174, 58',   temper: 'bloom' },
    psychic:     { rgb: '255, 122, 209', temper: 'creep' },
    radiant:     { rgb: '255, 243, 196', temper: 'snap' },
    slashing:    { rgb: '208, 106, 90',  temper: 'snap', jolt: true },
    thunder:     { rgb: '159, 184, 232', temper: 'snap', jolt: true }
  };
  // A blow of no particular kind still lands, and still looks like blood.
  var DAMAGE_PLAIN = { rgb: '150, 40, 30', temper: 'snap', jolt: true };

  var hitWash;

  function buildHitWash() {
    hitWash = document.createElement('div');
    hitWash.id = 'hit-wash';
    document.body.appendChild(hitWash);
  }

  // hitFlash washes the page in the colour of what just hit you. A blow that
  // the character shrugged off is washed more faintly, because the point of
  // the colour is how hard it landed and a resisted blow did not land hard.
  function hitFlash(type, defense) {
    if (!hitWash) { return; }
    var look = DAMAGE_LOOK[String(type || '').toLowerCase()] || DAMAGE_PLAIN;
    var root = document.documentElement;
    root.style.setProperty('--hit-rgb', look.rgb);
    // Immunity is the blow arriving and doing nothing: a breath of colour and
    // no rattle at all.
    var weak = defense === 'immunity';
    hitWash.style.setProperty('opacity', '');
    hitWash.style.setProperty('filter', weak ? 'opacity(.35)'
                              : defense === 'resistance' ? 'opacity(.6)' : '');
    hitWash.className = '';
    void hitWash.offsetWidth;
    hitWash.className = look.temper;
    if (look.jolt && !weak) {
      var page = document.querySelector('.page');
      if (page) {
        page.classList.remove('jolted');
        void page.offsetWidth;
        page.classList.add('jolted');
        setTimeout(function () { page.classList.remove('jolted'); }, 420);
      }
    }
    var gauge = document.querySelector('.hp-gauge');
    if (gauge && !weak) {
      gauge.classList.remove('flinch');
      void gauge.offsetWidth;
      gauge.classList.add('flinch');
      setTimeout(function () { gauge.classList.remove('flinch'); }, 460);
    }
  }

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

  // paintTempBar draws the temporary hit points onto the front of the bar.
  // They are not a higher ceiling - the maximum has not moved - they are hit
  // points standing in front of the ones you own, which is why they start
  // where yours end and why a blow eats them first.
  function paintTempBar(hp) {
    var seg = document.getElementById('hp-temp-fill');
    if (!seg) { return; }
    var pct = hp && typeof hp.percent === 'number' ? hp.percent : 0;
    var temp = hp && typeof hp.tempPercent === 'number' ? hp.tempPercent : 0;
    // The bar only has so much room. More temporary hit points than the
    // character's whole maximum fill what is left of it and no more.
    var room = Math.max(0, 100 - pct);
    seg.style.left = pct + '%';
    seg.style.width = Math.min(temp, room) + '%';
    seg.hidden = !(hp && hp.temp > 0);
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
    paintTempBar(hp);
    var val = document.getElementById('hp-temp-val');
    if (val) {
      val.textContent = hp.temp || 0;
      // Nought temporary hit points is worth saying, but not worth shouting -
      // and with none to give up there is nothing to press.
      val.classList.toggle('none', !hp.temp);
      val.disabled = !hp.temp;
    }
    paintDeathSaves(hp);
    // The edge of the page is coloured from the hit points, so it moves with
    // them rather than being told separately.
    paintMood();
  }

  // ------------------------------------------------------------ death saves
  //
  // Three successes and you are stable, three failures and you are gone. On
  // anything above nothing at all this is one line of small print; the moment
  // the character goes down the marks become the thing being looked at, and
  // clicking one fills it in.
  //
  // The rollable beside them throws the d20 and reads itself: a natural 20 is
  // not a success but standing back up, a natural 1 costs two failures, and ten
  // or better is a success. All of that is the engine's - see deathSaveResult
  // in health.go - so the page posts the number it threw rather than deciding.
  function paintDeathSaves(hp) {
    var line = document.getElementById('death-line');
    if (!line || !hp) { return; }
    var dying = !!hp.dying;
    line.setAttribute('data-dying', dying ? '1' : '0');
    line.classList.toggle('stable', !!hp.stable);
    line.classList.toggle('dead', !!hp.dead);
    var pips = document.getElementById('death-pips');
    if (pips) {
      var marks = pips.querySelectorAll('.dp');
      Array.prototype.forEach.call(marks, function (pip, i) {
        // The first three are the successes, the last three the failures.
        var win = i < 3;
        var n = win ? i + 1 : i - 2;
        pip.classList.toggle('on',
          n <= (win ? (hp.deathSuccesses || 0) : (hp.deathFailures || 0)));
        // Only worth offering as a control while the matter is unsettled.
        if (dying) {
          pip.setAttribute('role', 'button');
          pip.setAttribute('tabindex', '0');
          pip.setAttribute('data-mark', (win ? 'success:' : 'failure:') + n);
          pip.setAttribute('title', n + ' of three ' + (win ? 'successes' : 'failures'));
        } else {
          pip.removeAttribute('role');
          pip.removeAttribute('tabindex');
          pip.removeAttribute('data-mark');
        }
      });
    }
    var word = document.getElementById('death-word');
    if (word) {
      word.textContent = hp.dead ? 'Gone' : hp.stable ? 'Stable' : dying ? 'Dying' : '';
    }
    // The two things worth doing to a dying character that are not a save.
    var act = line.querySelector('.death-acts');
    if (!act) {
      act = document.createElement('span');
      act.className = 'death-acts';
      line.appendChild(act);
    }
    act.innerHTML = dying
      ? '<button type="button" class="death-do" data-death="stabilize" ' +
        'title="Three successes at once: someone got to you">Stabilise</button>'
      : ((hp.deathSuccesses || hp.deathFailures)
        ? '<button type="button" class="death-do" data-death="cleardeath" ' +
          'title="Wipe the marks without touching the hit points">Clear</button>'
        : '');
  }

  // deathSave posts one mark and redraws from what comes back. The roll, when
  // there is one, goes with it so the history can say what the die landed on.
  function deathSave(body) {
    if (!healthConfigured()) {
      HEALTH.error = 'this sheet does not know which org file it came from';
      paintHitPoints();
      return;
    }
    HEALTH.busy = true;
    HEALTH.error = '';
    paintHitPoints();
    var req = { filename: LOG.file || '', id: CHARACTER_ID || '' };
    Object.keys(body).forEach(function (k) { req[k] = body[k]; });
    api('POST', '/dnd/hp', req).then(function (state) {
      HEALTH.busy = false;
      HEALTH.state = state;
      HEALTH.msg = state.msg || '';
      paintHitPoints();
      var last = (state.history || [])[state.history.length - 1];
      if (last) { logNote(healthOrgLine(last, state.msg)); }
    }, function (err) {
      HEALTH.busy = false;
      HEALTH.msg = '';
      HEALTH.error = err.message || String(err);
      paintHitPoints();
    });
  }

  // healthOrgLine is the line the session notes get, which is HealthEventLine
  // in health.go said again here for the same reason hpLevel is: the page has
  // the answer already and should not ask for it again.
  function healthOrgLine(e, msg) {
    var text = String(msg || '');
    text = text.charAt(0).toUpperCase() + text.slice(1);
    switch (e.action) {
      case 'hurt': return '*Damage.* ' + text + '.';
      case 'healed': return '*Healing.* ' + text + '.';
      case 'death save':
      case 'revived':
      case 'stabilized':
      case 'death saves cleared':
        return '*Death Save.* ' + text + '.';
    }
    return '*Hit Points.* ' + text + '.';
  }

  // A death saving throw rolled on the sheet. The d20 is thrown on the table
  // like any other roll, and what it landed on is posted once it is down - so
  // the dice decide it in front of everyone rather than the page deciding and
  // then showing dice that agree.
  function rollDeathSave(el, ev) {
    var hp = (HEALTH.state && HEALTH.state.hp) || null;
    if (!hp || !hp.dying) {
      // Not dying, so this is just a saving throw to look at.
      activate(el, ev);
      return;
    }
    var mod = parseInt(el.getAttribute('data-mod'), 10) || 0;
    roll({ kind: 'check', label: 'Death Saving Throw', formula: 'd20 ' + signed(mod),
           flat: mod },
         originOf(el, ev),
         function (result) {
           // The flat roll is what a death save is: there is no advantage on
           // one unless something at the table says so, and if it does the
           // player settles the card and presses a mark themselves.
           deathSave({ action: 'deathsave', roll: result.normalNat });
         });
  }


  // Most hit point changes are a number and a direction. Giving up the
  // temporary ones is neither: there is nothing to type, so the box is not
  // asked for and an empty one is not an error.
  var HP_NO_AMOUNT = { cleartemp: true, stabilize: true, cleardeath: true };

  function healthChange(action) {
    if (!healthConfigured()) {
      HEALTH.error = 'this sheet does not know which org file it came from';
      paintHitPoints();
      return;
    }
    if (HP_NO_AMOUNT[action]) { healthPost(action, 0); return; }
    var amount = parseInt(hpAmount && hpAmount.value, 10);
    if (isNaN(amount) || amount < 0) {
      HEALTH.error = action === 'temp' ? 'how many temporary hit points?' : 'how much?';
      HEALTH.msg = '';
      paintHitPoints();
      if (hpAmount) { hpAmount.focus(); }
      return;
    }
    healthPost(action, amount);
  }

  function healthPost(action, amount) {
    HEALTH.busy = true;
    HEALTH.error = '';
    paintHitPoints();
    // What kind of damage it was only means anything on a hit, and the
    // engine is the one that knows what the character shrugs off - the page
    // sends the word and is told what actually landed.
    var type = (action === 'hurt' && hpType) ? hpType.value : '';
    api('POST', '/dnd/hp', { filename: LOG.file || '', id: CHARACTER_ID || '',
                             action: action, amount: amount, type: type })
      .then(function (state) {
        HEALTH.busy = false;
        HEALTH.state = state;
        HEALTH.msg = state.msg || '';
        if (hpAmount) { hpAmount.value = ''; hpAmount.focus(); }
        paintHitPoints();
        if (action === 'hurt') {
          var hit = (state.history || [])[state.history.length - 1];
          hitFlash(type || (hit && hit.type), hit && hit.defense);
        }
        // A blow landing is worth a line in the night's log.
        var last = (state.history || [])[state.history.length - 1];
        if (last) { logNote(healthOrgLine(last, state.msg)); }
        // A hit while holding a spell owes a Constitution save, and the server
        // has worked out the DC from how much landed.
        concAfterHealth(state, action);
      }, function (err) {
        HEALTH.busy = false;
        HEALTH.msg = '';
        HEALTH.error = err.message || String(err);
        paintHitPoints();
      });
  }

  function buildHealthUi() {
    buildHitWash();
    hpCtl = document.getElementById('hp-ctl');
    if (!hpCtl) { return; }
    hpAmount = document.getElementById('hp-amount');
    hpType = document.getElementById('hp-type');
    fillHitTypes();

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

    // Clicking the temporary hit points gives them up. They are the one
    // reading on the sheet that is routinely wanted gone outright rather than
    // moved a point at a time, and there is nothing else pressing it could
    // reasonably mean.
    var tempLine = document.getElementById('hp-temp-line');
    if (tempLine) {
      tempLine.addEventListener('click', function (e) {
        var btn = e.target.closest('[data-hp]');
        if (btn && !btn.disabled) { healthChange(btn.getAttribute('data-hp')); }
      });
    }

    // The death saves. A mark is pressed to fill it in - which is what happens
    // when the DM rolls for you, or when the save was made away from the sheet -
    // and the two buttons are for the rest of it.
    var deathLine = document.getElementById('death-line');
    if (deathLine) {
      deathLine.addEventListener('click', function (e) {
        var act = e.target.closest('[data-death]');
        if (act) { deathSave({ action: act.getAttribute('data-death') }); return; }
        var pip = e.target.closest('[data-mark]');
        if (!pip) { return; }
        var mark = pip.getAttribute('data-mark').split(':');
        deathSave({ action: 'deathsave', result: mark[0] });
      });
      deathLine.addEventListener('keydown', function (e) {
        if (e.key !== 'Enter' && e.key !== ' ') { return; }
        var pip = e.target.closest && e.target.closest('[data-mark]');
        if (!pip) { return; }
        e.preventDefault();
        deathSave({ action: 'deathsave',
                    result: pip.getAttribute('data-mark').split(':')[0] });
      });
    }
    hpCtl.addEventListener('keydown', function (e) {
      if (e.key !== 'Enter' || e.target !== hpAmount) { return; }
      e.preventDefault();
      // Enter is the one that gets pressed in a fight, and in a fight it is
      // nearly always damage - unless the palette put the cursor here
      // asking for something else, in which case that is what it means.
      var asked = hpAmount.getAttribute('data-pending');
      hpAmount.removeAttribute('data-pending');
      healthChange(asked || 'hurt');
    });

    paintHitPoints();
    healthLoad(true);
  }

  // ------------------------------------------------------------------ undo
  //
  // Every click on this sheet is written straight into the org file, which is
  // what makes it worth trusting and what makes a misplaced click expensive:
  // pressing Drink on the wrong line spends the potion and heals you for it.
  // So the last thing that happened can be taken back, item, hit points, coin
  // and history lines together.
  //
  // The engine decides what is undoable (undo.go) - the page only asks and
  // presses. That matters because the file is the state: another window, the
  // command line or a text editor may have moved things on since, and only the
  // server can see that.
  var UNDO = { plan: null, busy: false, error: '' };

  function undoConfigured() { return !!(LOG.file || CHARACTER_ID); }

  function paintUndo() {
    var btn = document.getElementById('sheet-undo');
    if (!btn) { return; }
    var what = document.getElementById('sheet-undo-what');
    var plan = UNDO.plan;
    var can = !!(plan && plan.can) && !UNDO.busy;
    btn.disabled = !can;
    if (!what) { return; }
    if (UNDO.busy) { what.textContent = 'taking it back…'; return; }
    if (UNDO.error) { what.textContent = UNDO.error; return; }
    if (!plan) { what.textContent = 'asking…'; return; }
    // Either what pressing it does, or why it cannot be pressed. Both are the
    // server's words, so the button never says something the engine would
    // then disagree with.
    what.textContent = plan.can ? plan.what : (plan.why || 'nothing to take back');
  }

  function undoLoad() {
    if (!undoConfigured()) {
      UNDO.plan = { can: false, why: 'this sheet does not know which org file it came from' };
      paintUndo();
      return Promise.resolve();
    }
    UNDO.error = '';
    var q = '/dnd/undo?filename=' + encodeURIComponent(LOG.file || '') +
      '&id=' + encodeURIComponent(CHARACTER_ID || '');
    return api('GET', q).then(function (plan) {
      UNDO.plan = plan;
      paintUndo();
    }, function (err) {
      UNDO.plan = null;
      UNDO.error = err.message || String(err);
      paintUndo();
    });
  }

  // undoLast presses it. One answer comes back carrying everything the undo
  // touched - the bag, the purse, the hit points and both histories - so the
  // whole sheet catches up without asking four more questions.
  function undoLast() {
    if (!undoConfigured() || UNDO.busy) { return; }
    UNDO.busy = true;
    UNDO.error = '';
    paintUndo();
    api('POST', '/dnd/undo', { filename: LOG.file || '', id: CHARACTER_ID || '' })
      .then(function (state) {
        UNDO.busy = false;
        UNDO.plan = state.undo || { can: false, why: 'nothing left to take back' };
        paintUndo();
        undoRepaint(state);
        // The night's log should say the accident was put right rather than
        // quietly disagreeing with the sheet for the rest of the session.
        if (state.msg) {
          logNote('*Undo.* ' + state.msg.charAt(0).toUpperCase() + state.msg.slice(1) + '.');
        }
      }, function (err) {
        UNDO.busy = false;
        UNDO.error = err.message || String(err);
        paintUndo();
      });
  }

  // undoRepaint hands the answer to every panel it could have moved.
  function undoRepaint(state) {
    if (!state) { return; }
    INV.state = state;
    INV.msg = state.msg || '';
    INV.error = '';
    if (state.inventory && !containerOf(state.inventory, INV.where)) { INV.where = ''; }
    renderInventory();
    renderInvHistory();
    paintArmorClass(state);
    if (state.purse) {
      COIN.state = COIN.state || {};
      COIN.state.purse = state.purse;
      if (state.moneyHistory) { COIN.state.history = state.moneyHistory; }
      COIN.state.name = COIN.state.name || state.name;
      renderCoins();
      // An undo can put coin back in the purse, but it does it by taking a
      // line off the log rather than adding one, so nothing is thrown.
      noteCoinArrivals(state.moneyHistory);
    }
    if (state.hp) {
      HEALTH.state = HEALTH.state || { hp: null, history: [] };
      HEALTH.state.hp = state.hp;
      if (state.healthHistory) { HEALTH.state.history = state.healthHistory; }
      HEALTH.msg = state.msg || '';
      HEALTH.error = '';
      paintHitPoints();
    }
  }

  // --------------------------------------------------------- concentration
  //
  // The one spell a caster is holding. It lives in the character's own org file
  // as DND_CONCENTRATION, so a spell that is still up is still up when the
  // sheet is reloaded - which is the point of writing it down at all.
  //
  // The band under the hit points shows it, and does the two things that can
  // happen to it: the Constitution saving throw a blow calls for, and letting
  // it go. The DC is the server's - it knows how much damage landed - and
  // arrives on the answer to the blow.
  var CONC = { view: null, owed: null, busy: false };
  var concBox;

  function concConfigured() { return !!(LOG.file || CHARACTER_ID); }

  function concWho(body) {
    body = body || {};
    body.filename = LOG.file || '';
    body.id = CHARACTER_ID || '';
    return body;
  }

  // concTake is how every other panel hands the banner what it learned. The
  // hit point answers carry concentration, so a hit redraws this without the
  // page asking a second question.
  function concTake(view) {
    CONC.view = view || null;
    // Nothing is being held any more, so there is nothing to save for either.
    if (!CONC.view || !CONC.view.on) { CONC.owed = null; }
    renderConcentration();
    paintMood();
  }

  function concLoad() {
    if (!concConfigured()) { return; }
    api('GET', '/dnd/concentration?filename=' + encodeURIComponent(LOG.file || '') +
        '&id=' + encodeURIComponent(CHARACTER_ID || ''))
      .then(function (state) { concTake(state.concentration); },
            function () { /* the sheet already shows what was exported */ });
  }

  function concChange(body, said) {
    if (!concConfigured()) { return Promise.resolve(null); }
    CONC.busy = true;
    renderConcentration();
    return api('POST', '/dnd/concentration', concWho(body)).then(function (state) {
      CONC.busy = false;
      CONC.owed = null;
      concTake(state.concentration);
      if (said !== false && state.msg) { logNote(concOrgLine(state.msg)); }
      return state;
    }, function () {
      CONC.busy = false;
      renderConcentration();
      return null;
    });
  }

  function concOrgLine(msg) {
    var text = String(msg || '');
    if (!text) { return ''; }
    return '*Concentration.* ' + text.charAt(0).toUpperCase() + text.slice(1) + '.';
  }

  // concStart is called when a concentration spell is cast. It is fire and
  // forget: the cast has already happened and the dice are already on the
  // table, so a banner that is a moment late is better than a cast that waits
  // for it.
  function concStart(spell, level) {
    if (!spell) { return; }
    concChange({ action: 'start', spell: spell, level: level || 0 });
  }

  function concDrop() {
    concChange({ action: 'drop' });
  }

  // concAfterHealth reads the save a blow called for off the hit point answer.
  // The server works out the DC because it knows how much damage landed after
  // the temporary hit points had their say.
  function concAfterHealth(state, action) {
    if (!state) { return; }
    if (state.concentration) { concTake(state.concentration); }
    if (action === 'hurt' && state.save) {
      CONC.owed = state.save;
      renderConcentration();
    }
  }

  // rollConcSave throws the Constitution save on the table and posts what it
  // came to. As with a death save the dice decide it in front of everyone.
  function rollConcSave() {
    var owed = CONC.owed;
    if (!owed) { return; }
    var btn = concBox && concBox.querySelector('.conc-b.keep');
    var mod = owed.mod || 0;
    roll({ kind: 'check', label: 'Concentration: ' + (owed.name || 'spell'),
           formula: 'd20 ' + signed(mod), flat: mod,
           advice: adviceKind('save', 'con') },
         btn ? originOf(btn, null) : { x: window.innerWidth / 2, y: 200 },
         function (result) {
           concChange({ action: 'save', dc: owed.dc, roll: result.normal });
         });
  }

  function renderConcentration() {
    if (!concBox) { return; }
    var v = CONC.view;
    if (!v || !v.on) {
      concBox.hidden = true;
      concBox.innerHTML = '';
      concBox.classList.remove('owed');
      return;
    }
    concBox.hidden = false;
    var owed = CONC.owed;
    concBox.classList.toggle('owed', !!owed);
    var sub = [];
    if (v.duration) { sub.push(v.duration); }
    if (v.since) { sub.push('since ' + v.since); }
    if (owed) {
      // A save is owed, so the band stops saying what is up and starts saying
      // what has to be rolled to keep it.
      concBox.innerHTML =
        '<span class="conc-mark"></span>' +
        '<span class="conc-name">' + esc(v.label || v.name) + '</span>' +
        '<span class="conc-sub">Constitution save <span class="conc-dc">DC ' +
          (owed.dc || 10) + '</span> to keep it &middot; ' +
          esc(owed.modStr || signed(owed.mod || 0)) + '</span>' +
        '<button type="button" class="conc-b keep">Roll it</button>' +
        '<button type="button" class="conc-b give">Let it go</button>';
      return;
    }
    concBox.innerHTML =
      '<span class="conc-mark"></span>' +
      '<span class="conc-sub">Concentrating on</span>' +
      '<span class="conc-name">' + esc(v.label || v.name) + '</span>' +
      (sub.length ? '<span class="conc-sub">' + esc(sub.join(' \u00b7 ')) + '</span>' : '') +
      '<span class="conc-sub">save ' + esc(v.saveStr || '') + '</span>' +
      '<button type="button" class="conc-b give"' +
        (CONC.busy ? ' disabled' : '') + '>Let it go</button>';
  }

  function buildConcentrationUi() {
    concBox = document.getElementById('conc-line');
    if (!concBox) { return; }

    // The sheet was exported with whatever was being held at the time, so the
    // band is right before the server has said anything.
    var seed = document.getElementById('dnd-concentration-data');
    if (seed) {
      try { CONC.view = JSON.parse(seed.textContent || 'null'); }
      catch (e) { /* wait for the server */ }
    }

    concBox.addEventListener('click', function (e) {
      if (e.target.closest('.conc-b.keep')) { rollConcSave(); return; }
      if (e.target.closest('.conc-b.give')) { concDrop(); }
    });

    renderConcentration();
    concLoad();
  }

  // ------------------------------------------------------------ inspiration
  // One bit, handed out by the DM and spent by the player. Pressing the
  // marker in the header posts to the server, which rewrites
  // DND_INSPIRATION in the character's own org file - so the sheet, the file
  // and anyone else reading it agree, the same way hit points do.
  var INSP = { held: false, busy: false };
  var inspBox, inspBtn, inspWord;

  function paintInspiration() {
    paintPortrait();
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
    sbPaintCastLevels();
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
    var at = castLevelOf(btn);
    var level = at.level;
    // A spell that asks for concentration is taken up whether or not it costs a
    // slot: a cantrip can concentrate, and so can a spell cast from a scroll.
    // Casting a second one lets the first go, which is the engine's business.
    if (btn.getAttribute('data-conc') === '1') {
      concStart(btn.getAttribute('data-spell-id') ||
                btn.getAttribute('data-spell') || '',
                at.tier ? 0 : level);
    }
    // A tier picker reads character levels, not slots. Its number must never
    // reach the slot endpoint: a 5th level cantrip tier is not a 5th level
    // slot, and spending one for a cantrip would be a theft.
    if (at.tier || level < 1 || !sbCanWrite()) { return; }
    var ritual = btn.getAttribute('data-ritual') === '1';
    // A cast at the spell's own level may reach upward for a slot when that
    // level is empty, which is the server's business. A level picked by hand
    // means that level and no other: the dice on the table were rolled for
    // it, so reaching past it would cost a slot the roll never had.
    var action = level > at.base ? 'spend' : 'cast';
    sbSlotChange(action, level, btn.getAttribute('data-spell') || '',
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

  // ------------------------------------------------ casting at a higher level
  //
  // A spell whose text says what a bigger slot buys - "the healing increases
  // by 1d4 for each slot level above 1st" - is cast from a level of your
  // choosing. The rules engine worked out the dice for every level this
  // character has slots at, so the picker beside the cast button only has to
  // say which levels are still worth reaching for.
  function sbOrdinal(n) {
    var tens = n % 100, ones = n % 10;
    if (tens >= 11 && tens <= 13) { return n + 'th'; }
    return n + (ones === 1 ? 'st' : ones === 2 ? 'nd' : ones === 3 ? 'rd' : 'th');
  }

  function sbUpcastHtml(cast, level, name) {
    var ups = (cast && cast.upcast) || [];
    if (!ups.length) { return ''; }
    var opts = '<option value="' + level + '" selected>' +
      sbOrdinal(level) + '</option>';
    ups.forEach(function (up) {
      opts += '<option value="' + up.level + '"' +
        ' data-damage="' + esc(up.damage || '') + '"' +
        ' data-heal="' + esc(up.heal || '') + '"' +
        ' data-damage2="' + esc(up.damage2 || '') + '"' +
        ' data-detail="' + esc(up.detail || '') + '"' +
        ' data-short="' + esc(up.short || '') + '"' +
        ' data-note="' + esc(up.note || '') + '"' +
        ' title="' + esc(up.note || '') + '">' + esc(up.label) + '</option>';
    });
    return '<select class="cast-at" data-base="' + level + '" title="Cast ' +
      esc(name) + ' at a higher level" aria-label="Slot level to cast ' +
      esc(name) + ' at">' + opts + '</select>';
  }

  // The same picker for a cantrip, whose levels are the ones its dice grow at
  // rather than slots to spend. It starts on the tier this character is at.
  function sbTierHtml(cast, name) {
    var tiers = (cast && cast.tiers) || [];
    if (!tiers.length) { return ''; }
    var base = cast.tierLevel || tiers[tiers.length - 1].level;
    var opts = tiers.map(function (t) {
      return '<option value="' + t.level + '"' + (t.current ? ' selected' : '') +
        ' data-damage="' + esc(t.damage || '') + '"' +
        ' data-heal="' + esc(t.heal || '') + '"' +
        ' data-damage2="' + esc(t.damage2 || '') + '"' +
        ' data-detail="' + esc(t.detail || '') + '"' +
        ' data-short="' + esc(t.short || '') + '"' +
        ' data-note="' + esc(t.note || '') + '"' +
        ' title="' + esc(t.note || '') + '">' + esc(t.label) + '</option>';
    }).join('');
    return '<select class="cast-at" data-tier="1" data-base="' + base +
      '" title="' + esc(name) + ' grows with your level"' +
      ' aria-label="Character level to roll ' + esc(name) + ' at">' +
      opts + '</select>';
  }

  // What is left at each slot level, read off the pips the page is already
  // showing rather than kept a second time.
  function sbSlotsLeft() {
    var left = {};
    Array.prototype.forEach.call(
      document.querySelectorAll('.slot-row[data-level]'), function (row) {
        var lvl = parseInt(row.getAttribute('data-level'), 10) || 0;
        if (!lvl) { return; }
        var total = row.querySelectorAll('.slot').length;
        var used = parseInt(row.getAttribute('data-used'), 10) || 0;
        left[lvl] = Math.max(0, total - used);
      });
    return left;
  }

  // What the picker says about itself once a level is chosen: a level above
  // the spell's own is worth seeing at a glance.
  function sbMarkCastLevel(sel) {
    var base = parseInt(sel.getAttribute('data-base'), 10) || 0;
    var lvl = parseInt(sel.value, 10) || base;
    if (!sel.getAttribute('data-title')) {
      sel.setAttribute('data-title', sel.title || 'Cast at a higher level');
    }
    var tier = sel.getAttribute('data-tier') === '1';
    sel.classList.toggle('up', tier ? lvl !== base : lvl > base);
    var opt = sel.options[sel.selectedIndex];
    var note = opt ? (opt.getAttribute('data-note') || '') : '';
    if (tier) {
      sel.title = lvl === base
        ? sel.getAttribute('data-title')
        : 'Rolled as a ' + sbOrdinal(lvl) + ' level character' +
          (note ? ': ' + note : '');
      return;
    }
    sel.title = lvl > base
      ? 'Cast from a ' + sbOrdinal(lvl) + ' level slot' + (note ? ': ' + note : '')
      : sel.getAttribute('data-title');
  }

  // A level with no slots left is no level to cast from, so it is greyed out
  // - the spell's own level excepted, which the server may still answer for
  // with a slot from higher up, or with a ritual.
  function sbPaintCastLevels() {
    var left = sbSlotsLeft();
    Array.prototype.forEach.call(
      document.querySelectorAll('.cast-at'), function (sel) {
        var base = parseInt(sel.getAttribute('data-base'), 10) || 0;
        // A cantrip's tiers cost no slot, so there is nothing to run out of.
        if (sel.getAttribute('data-tier') === '1') { sbMarkCastLevel(sel); return; }
        Array.prototype.forEach.call(sel.options, function (opt) {
          var lvl = parseInt(opt.value, 10) || 0;
          var n = left[lvl];
          var note = opt.getAttribute('data-note') || '';
          if (lvl !== base) {
            opt.disabled = n === 0;
          }
          if (n !== undefined) {
            opt.title = note + (note ? ' \u00b7 ' : '') + n + ' left';
          }
        });
        var chosen = sel.options[sel.selectedIndex];
        if (chosen && chosen.disabled) { sel.value = String(base); }
        sbMarkCastLevel(sel);
      });
  }

  // One spell as the sheet shows it. The cast button carries everything the
  // dice need, which the rules engine worked out when it computed the sheet.
  function sbSpellHtml(sp, level) {
    var c = sp.cast || {};
    var attrs = ' data-spell="' + esc(sp.name) + '"' +
      ' data-school="' + esc(sp.school || '') + '"' +
      ' data-detail="' + esc(c.detail || '') + '"' +
      ' data-line="' + esc(c.line || '') + '"' +
      ' data-short="' + esc(c.short || '') + '"';
    if (c.attack) { attrs += ' data-attack="' + c.attackBonus + '"'; }
    if (c.damage) {
      attrs += ' data-damage="' + esc(c.damage) + '"' +
        ' data-damage-type="' + esc(c.damageType || '') + '"';
    }
    if (c.heal) { attrs += ' data-heal="' + esc(c.heal) + '"'; }
    if (c.damage2) {
      attrs += ' data-damage2="' + esc(c.damage2) + '"' +
        ' data-damage2-type="' + esc(c.damage2Type || '') + '"' +
        ' data-damage2-label="' + esc(c.damage2Label || '') + '"';
    }
    attrs += ' data-level="' + (level || 0) + '"';
    if (sp.ritual) { attrs += ' data-ritual="1"'; }
    // A spell that asks for concentration says so, and says what it is called
    // in the ruleset: the banner is written into the character file by id.
    if (sp.concentration) {
      attrs += ' data-conc="1" data-spell-id="' + esc(sp.id || sp.name) + '"';
    }
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
      (c.damage2
        ? '<button type="button" class="cast2-btn" title="Roll ' + esc(sp.name) +
          '\'s ' + esc(c.damage2) + ' ' + esc(c.damage2Type || '') + '">' +
          esc(c.damage2Label || 'Also') + '</button>'
        : '') +
      sbUpcastHtml(c, level, sp.name) +
      sbTierHtml(c, sp.name) +
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
    sbPaintCastLevels();
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
      if (e.target.closest('.cast-at')) {
        // The picker sits inside the spell's <summary>, where a click would
        // otherwise fold the spell open or shut. The dropdown itself opens on
        // mousedown, so stopping the click does not stop it opening.
        e.preventDefault();
        return;
      }
      var pip = e.target.closest('.slot');
      if (pip) { sbSlotClick(pip); }
    });

    sbBox.addEventListener('change', function (e) {
      var sel = e.target.closest ? e.target.closest('.cast-at') : null;
      if (sel) { sbMarkCastLevel(sel); }
    });

    // The exported markup arrives with pickers already on it, before the
    // server has said anything: say what they can reach right now.
    sbPaintCastLevels();

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

  // ----------------------------------------------------------- the flourish
  //
  // A natural 20 and a natural 1 are the two rolls everyone at the table looks
  // up for, so the sheet marks them: a swell of the halo round the page, a
  // short fanfare or dirge, and the card says which it was.
  //
  // It fires on the reading that actually counts. A d20 is thrown twice and
  // reported three ways, and until the player says whether they had advantage
  // the flat roll is what stands - so that is the one read here. Settling the
  // roll onto another reading afterwards can fire it again, because a 20 the
  // player has just claimed with advantage is every bit as much a 20.
  function natOf(r) {
    if (!r) { return 0; }
    if (r.kind === 'check') { return d20Nat(r, d20At(r)); }
    if (r.kind === 'cast' && r.attack) { return d20Nat(r.attack, d20At(r)); }
    return 0;
  }

  // showFlourish marks the roll and plays it. The word goes on the result so
  // the card can say it, and a roll settled onto another reading is played
  // again - a 20 the player has just claimed with advantage is as much a 20 as
  // one that came up flat.
  function showFlourish(r) {
    if (!r) { return; }
    var nat = natOf(r);
    var kind = nat === 20 ? 'crit' : nat === 1 ? 'fumble' : '';
    if (kind === r.flourish) { return; }
    r.flourish = kind;
    // Redraw whichever way it went. Settling a roll back off its natural
    // reading has to take the line away again, or the card goes on claiming a
    // natural 20 that the player has just said they did not have.
    if (history[0] === r && latestEl) {
      latestEl.innerHTML = latestCard(r);
    }
    if (!kind) { return; }
    moodFlash(kind);
    if (kind === 'crit') { cheer(); } else { dirge(); }
    // And the thing itself: fireworks over the die that rolled the twenty, a
    // knife into the one that rolled the one. Only while the dice are still
    // on the table - a natural read off the card afterwards gets the colour
    // and the sound, but there is nothing left to throw a firework over.
    if (board && board.dice && board.dice.length) { board.spectacle(kind); }
  }

  // ------------------------------------------------------------------ sound
  //
  // The dice are heard as well as seen: a knock as each one lands, a lighter
  // tick off the edges of the sheet, and a short flourish on a natural 20 or a
  // natural 1. Every sound here is synthesised with the web audio api - the
  // exported sheet has no dependencies and no files beside it, so a wav would
  // have to be a data uri several hundred kilobytes long, and a knock made out
  // of filtered noise is both smaller and better.
  //
  // Nothing is created until the first roll: a page that opens an audio
  // context before anyone has clicked is one the browser warns about, and a
  // sheet that has only been read should make no noise at all.
  var SOUND = { on: true, ctx: null, gain: null, last: 0 };
  var SOUND_KEY = 'dnd-sheet-sound';

  function soundLoad() {
    try {
      var saved = window.localStorage.getItem(SOUND_KEY);
      if (saved !== null) { SOUND.on = saved === '1'; }
    } catch (e) { /* no storage, so the default stands */ }
  }

  function soundSave() {
    try { window.localStorage.setItem(SOUND_KEY, SOUND.on ? '1' : '0'); }
    catch (e) { /* nothing to do about it */ }
  }

  // audio is the context, made on the first sound that actually plays. It is
  // resumed each time because a browser may suspend it the moment the tab goes
  // to the background, and the next roll should still be heard.
  function audio() {
    if (!SOUND.on) { return null; }
    var Ctx = window.AudioContext || window.webkitAudioContext;
    if (!Ctx) { return null; }
    if (!SOUND.ctx) {
      try { SOUND.ctx = new Ctx(); } catch (e) { SOUND.ctx = null; return null; }
      SOUND.gain = SOUND.ctx.createGain();
      // Well under half, and the whole point: this is a sound effect on a
      // character sheet, not a game.
      SOUND.gain.gain.value = 0.34;
      SOUND.gain.connect(SOUND.ctx.destination);
    }
    if (SOUND.ctx.state === 'suspended' && SOUND.ctx.resume) { SOUND.ctx.resume(); }
    return SOUND.ctx;
  }

  // noiseBuffer is a third of a second of white noise, made once and reused.
  // Every knock is a window cut out of it, which is what makes the knocks
  // sound like the same dice each time rather than like four different ones.
  var noiseBuf = null;
  function noiseBuffer(ctx) {
    if (noiseBuf) { return noiseBuf; }
    var n = Math.floor(ctx.sampleRate * 0.3);
    noiseBuf = ctx.createBuffer(1, n, ctx.sampleRate);
    var data = noiseBuf.getChannelData(0);
    for (var i = 0; i < n; i++) { data[i] = Math.random() * 2 - 1; }
    return noiseBuf;
  }

  // knock is one die hitting something: a burst of noise through a band pass,
  // with a very short decay. strength is 0..1 and moves both how loud it is
  // and how bright, because a hard knock on paper is both.
  //
  // A pitch that moves a little each time keeps a handful of dice landing
  // together from sounding like one loud die.
  function knock(strength, pitch) {
    var ctx = audio();
    if (!ctx) { return; }
    // A cap on how often a knock may be heard. Four dice settling in the same
    // frame is one sound, not four on top of each other.
    var now = ctx.currentTime;
    if (now - SOUND.last < 0.022) { return; }
    SOUND.last = now;

    strength = Math.max(0.05, Math.min(1, strength || 0.5));
    var src = ctx.createBufferSource();
    src.buffer = noiseBuffer(ctx);
    src.playbackRate.value = 0.8 + Math.random() * 0.5;
    var band = ctx.createBiquadFilter();
    band.type = 'bandpass';
    band.frequency.value = (pitch || 900) * (0.85 + Math.random() * 0.35);
    band.Q.value = 1.1 + strength * 1.4;
    // A little body under the knock, so it lands on a table rather than in a
    // vacuum. One cycle of a low sine is all it takes.
    var body = ctx.createOscillator();
    body.type = 'sine';
    body.frequency.value = 96 + Math.random() * 40;
    var bodyGain = ctx.createGain();
    var g = ctx.createGain();
    var dur = 0.045 + strength * 0.05;

    g.gain.setValueAtTime(0, now);
    g.gain.linearRampToValueAtTime(strength * 0.9, now + 0.004);
    g.gain.exponentialRampToValueAtTime(0.0008, now + dur);
    bodyGain.gain.setValueAtTime(0, now);
    bodyGain.gain.linearRampToValueAtTime(strength * 0.35, now + 0.006);
    bodyGain.gain.exponentialRampToValueAtTime(0.0008, now + dur * 1.5);

    src.connect(band); band.connect(g); g.connect(SOUND.gain);
    body.connect(bodyGain); bodyGain.connect(SOUND.gain);
    src.start(now); src.stop(now + dur + 0.02);
    body.start(now); body.stop(now + dur * 1.6);
  }

  // tick is a die glancing off the rim of the sheet: shorter, brighter and
  // quieter than a knock, so a die rattling along an edge is heard as that
  // rather than as landing over and over.
  function tick(strength) { knock(Math.min(0.5, strength * 0.55), 2100); }

  // tone is one clean note, which is what the flourishes are built out of.
  function tone(freq, at, dur, level, type) {
    var ctx = audio();
    if (!ctx) { return; }
    var t = ctx.currentTime + (at || 0);
    var osc = ctx.createOscillator();
    var g = ctx.createGain();
    osc.type = type || 'triangle';
    osc.frequency.setValueAtTime(freq, t);
    g.gain.setValueAtTime(0, t);
    g.gain.linearRampToValueAtTime(level, t + 0.012);
    g.gain.exponentialRampToValueAtTime(0.0008, t + dur);
    osc.connect(g); g.connect(SOUND.gain);
    osc.start(t); osc.stop(t + dur + 0.02);
  }

  // crowdBuffer is two and a half seconds of the noise a roomful of people
  // makes, built once and reused. White noise on its own is a hiss; what
  // turns it into a crowd is two things, and both are baked in here rather
  // than done with filters afterwards:
  //
  //   - a one pole low pass, which takes the glassy top off it and leaves
  //     something with a throat; and
  //   - a flutter made of a few slow sines at rates that do not divide into
  //     each other, so the roar never sits still and never repeats. That
  //     unevenness is what the ear reads as many voices rather than one.
  var crowdBuf = null;
  function crowdBuffer(ctx) {
    if (crowdBuf) { return crowdBuf; }
    var n = Math.floor(ctx.sampleRate * 2.5);
    crowdBuf = ctx.createBuffer(1, n, ctx.sampleRate);
    var d = crowdBuf.getChannelData(0);
    var warm = 0;
    for (var i = 0; i < n; i++) {
      var w = Math.random() * 2 - 1;
      warm = warm * 0.86 + w * 0.14;
      var t = i / ctx.sampleRate;
      var flutter = 0.70 +
        0.17 * Math.sin(t * 27.3) +
        0.11 * Math.sin(t * 41.9 + 1.7) +
        0.09 * Math.sin(t * 13.1 + 3.4);
      // The warm part carries the body, and a little of the raw noise is
      // left on top for the breath of it.
      d[i] = (warm * 3.4 + w * 0.3) * flutter;
    }
    return crowdBuf;
  }

  // cheer is what a natural 20 sounds like: the table going up. Not a
  // fanfare - a fanfare is the page congratulating you, and after the
  // fiftieth one it is a beep - but a room, heard from across it. Two bands
  // of crowd noise swelling and falling away, a handful of scattered claps
  // over the top of them, and four quiet voices underneath that rise the way
  // a shout does. It is deliberately soft: you should be able to roll well
  // twice in a minute without wanting it turned off.
  function cheer() {
    var ctx = audio();
    if (!ctx) { return; }
    var now = ctx.currentTime;
    var buf = crowdBuffer(ctx);

    // The body of the room, and a thinner band over it for the brightness
    // of a lot of people talking at once.
    [{ lo: 230, hi: 1500, level: 0.11, at: 0, dur: 2.0 },
     { lo: 1100, hi: 3200, level: 0.036, at: 0.04, dur: 1.7 }].forEach(function (L) {
      var src = ctx.createBufferSource();
      src.buffer = buf;
      // A different offset into the noise for each layer, so the two are not
      // the same sound twice.
      src.playbackRate.value = 0.9 + Math.random() * 0.25;
      var hp = ctx.createBiquadFilter();
      hp.type = 'highpass';
      hp.frequency.value = L.lo;
      var lp = ctx.createBiquadFilter();
      lp.type = 'lowpass';
      lp.frequency.value = L.hi;
      var g = ctx.createGain();
      var t0 = now + L.at;
      // The shape of a cheer: up fast, held while everyone catches up with
      // each other, then a long tail as it dies down.
      g.gain.setValueAtTime(0.0008, t0);
      g.gain.linearRampToValueAtTime(L.level, t0 + 0.17);
      g.gain.linearRampToValueAtTime(L.level * 0.82, t0 + 0.5);
      g.gain.exponentialRampToValueAtTime(0.0008, t0 + L.dur);
      src.connect(hp); hp.connect(lp); lp.connect(g); g.connect(SOUND.gain);
      src.start(t0); src.stop(t0 + L.dur + 0.05);
    });

    // Claps. Irregularly spaced and thinning out, because a room does not
    // applaud in time and the ear notices at once when it does.
    var at = 0.1;
    for (var i = 0; i < 7; i++) {
      clap(at, 0.032 + Math.random() * 0.036);
      at += 0.06 + Math.random() * 0.13 + i * 0.02;
    }

    // And four voices under all of it, too quiet to pick out and detuned
    // enough to beat against each other, which is what keeps them from
    // reading as a chord.
    for (var v = 0; v < 4; v++) {
      var osc = ctx.createOscillator();
      var vg = ctx.createGain();
      var vlp = ctx.createBiquadFilter();
      var f0 = 152 + v * 37 + Math.random() * 14;
      osc.type = 'sawtooth';
      osc.frequency.setValueAtTime(f0, now);
      // A cheer rises and then sags, which is most of what makes it a cheer.
      osc.frequency.linearRampToValueAtTime(f0 * 1.12, now + 0.28);
      osc.frequency.linearRampToValueAtTime(f0 * 0.94, now + 1.5);
      vlp.type = 'lowpass';
      vlp.frequency.setValueAtTime(520, now);
      vlp.frequency.linearRampToValueAtTime(760, now + 0.3);
      vlp.frequency.exponentialRampToValueAtTime(320, now + 1.6);
      vg.gain.setValueAtTime(0.0008, now);
      vg.gain.linearRampToValueAtTime(0.011, now + 0.2 + v * 0.03);
      vg.gain.exponentialRampToValueAtTime(0.0008, now + 1.6);
      osc.connect(vlp); vlp.connect(vg); vg.connect(SOUND.gain);
      osc.start(now); osc.stop(now + 1.7);
    }
  }

  // clap is one pair of hands: a very short band of noise with no tail. On
  // its own it is a tick; seven of them scattered over three quarters of a
  // second are a room.
  function clap(at, level) {
    var ctx = audio();
    if (!ctx) { return; }
    var t = ctx.currentTime + at;
    var src = ctx.createBufferSource();
    src.buffer = noiseBuffer(ctx);
    src.playbackRate.value = 0.7 + Math.random() * 0.5;
    var band = ctx.createBiquadFilter();
    band.type = 'bandpass';
    band.frequency.value = 900 + Math.random() * 700;
    band.Q.value = 1.1;
    var g = ctx.createGain();
    g.gain.setValueAtTime(0.0008, t);
    g.gain.linearRampToValueAtTime(level, t + 0.003);
    g.gain.exponentialRampToValueAtTime(0.0008, t + 0.05 + Math.random() * 0.03);
    src.connect(band); band.connect(g); g.connect(SOUND.gain);
    src.start(t); src.stop(t + 0.1);
  }

  // scratchBuffer is a second of the noise a needle makes dragged across a
  // record, made once and reused. Two things make it read as vinyl rather
  // than as static:
  //
  //   - the groove. A record scratch is not noise, it is the *rumble* of a
  //     stylus crossing several hundred grooves a second, so the noise is
  //     run through a wobble at a rate that falls as the drag slows. That
  //     wobble is the whole sound; without it this is a hiss.
  //   - the dust. A few louder ticks scattered through it, which is what
  //     the ear hears as a physical thing touching another physical thing.
  var scratchBuf = null;
  function scratchBuffer(ctx) {
    if (scratchBuf) { return scratchBuf; }
    var n = Math.floor(ctx.sampleRate * 1.0);
    scratchBuf = ctx.createBuffer(1, n, ctx.sampleRate);
    var d = scratchBuf.getChannelData(0);
    var warm = 0, phase = 0;
    for (var i = 0; i < n; i++) {
      var t = i / n;
      var w = Math.random() * 2 - 1;
      warm = warm * 0.72 + w * 0.28;
      // The grooves go by fast at first and slow as the drag runs out.
      phase += (170 - 120 * t) / ctx.sampleRate;
      var groove = 0.55 + 0.45 * Math.sin(phase * Math.PI * 2);
      // A tick of dust now and then.
      var dust = Math.random() < 0.0015 ? 2.4 : 1;
      d[i] = warm * 2.3 * groove * dust;
    }
    return scratchBuf;
  }

  // A natural 1 is a record scratch: the moment everything stops and the
  // room looks at you. Soft, and over in half a second - the point is the
  // pause it implies, not the noise itself.
  function dirge() {
    var ctx = audio();
    if (!ctx) { return; }
    var now = ctx.currentTime;
    var src = ctx.createBufferSource();
    src.buffer = scratchBuffer(ctx);
    // The drag slowing down, which is the shape of the whole thing.
    src.playbackRate.setValueAtTime(1.5, now);
    src.playbackRate.exponentialRampToValueAtTime(0.55, now + 0.42);
    // Bandpassed to the middle, where a stylus lives: no sizzle on top and
    // no rumble underneath, either of which would turn it back into noise.
    var band = ctx.createBiquadFilter();
    band.type = 'bandpass';
    band.Q.value = 1.1;
    band.frequency.setValueAtTime(1500, now);
    band.frequency.exponentialRampToValueAtTime(420, now + 0.45);
    var hp = ctx.createBiquadFilter();
    hp.type = 'highpass';
    hp.frequency.value = 180;
    var g = ctx.createGain();
    g.gain.setValueAtTime(0.0008, now);
    g.gain.linearRampToValueAtTime(0.3, now + 0.02);
    g.gain.linearRampToValueAtTime(0.22, now + 0.24);
    g.gain.exponentialRampToValueAtTime(0.0008, now + 0.52);
    src.connect(band); band.connect(hp); hp.connect(g); g.connect(SOUND.gain);
    src.start(now); src.stop(now + 0.6);
    // And the thud of the needle going down, under the front of it.
    var thud = ctx.createOscillator();
    var tg = ctx.createGain();
    thud.type = 'sine';
    thud.frequency.setValueAtTime(128, now);
    thud.frequency.exponentialRampToValueAtTime(58, now + 0.16);
    tg.gain.setValueAtTime(0.0008, now);
    tg.gain.linearRampToValueAtTime(0.14, now + 0.012);
    tg.gain.exponentialRampToValueAtTime(0.0008, now + 0.22);
    thud.connect(tg); tg.connect(SOUND.gain);
    thud.start(now); thud.stop(now + 0.26);
  }

  // clink is a coin landing: a metal disc rings at frequencies that are not
  // a harmonic series, which is exactly why it sounds like metal and not like
  // a note. Four inharmonic partials and a scrape of noise under them is
  // enough, and the bigger the coin the lower it rings.
  function clink(strength, look) {
    var ctx = audio();
    if (!ctx) { return; }
    var now = ctx.currentTime;
    // Two coins landing in the same frame is one sound, as with the dice.
    if (now - (SOUND.lastClink || 0) < 0.018) { return; }
    SOUND.lastClink = now;
    strength = Math.max(0.04, Math.min(1, strength || 0.4));
    var base = 1950 * (15.5 / ((look && look.r) || 15.5)) * (0.94 + Math.random() * 0.14);
    var partials = [1, 1.61, 2.39, 3.11], levels = [0.16, 0.1, 0.07, 0.04];
    for (var i = 0; i < partials.length; i++) {
      var osc = ctx.createOscillator();
      var g = ctx.createGain();
      osc.type = 'sine';
      osc.frequency.value = base * partials[i];
      var dur = 0.42 - i * 0.07;
      g.gain.setValueAtTime(0, now);
      g.gain.linearRampToValueAtTime(levels[i] * strength, now + 0.002);
      g.gain.exponentialRampToValueAtTime(0.0006, now + dur);
      osc.connect(g); g.connect(SOUND.gain);
      osc.start(now); osc.stop(now + dur + 0.02);
    }
    // The click of the edge touching down, under the ring.
    var src = ctx.createBufferSource();
    src.buffer = noiseBuffer(ctx);
    src.playbackRate.value = 1.6;
    var hp = ctx.createBiquadFilter();
    hp.type = 'highpass';
    hp.frequency.value = 2400;
    var ng = ctx.createGain();
    ng.gain.setValueAtTime(strength * 0.22, now);
    ng.gain.exponentialRampToValueAtTime(0.0006, now + 0.035);
    src.connect(hp); hp.connect(ng); ng.connect(SOUND.gain);
    src.start(now); src.stop(now + 0.06);
  }

  // pop is one firework shell going off, heard from a distance: a soft
  // thump with a roll of crackle after it rather than a crack.
  //
  // Fireworks a long way off have no top end at all - the air takes it -
  // and that is the whole of the difference between a firework and a
  // firecracker going off beside your ear. So the noise is slowed right
  // down before it is filtered, the filter never opens above the low
  // thousands, and the weight of the sound is a sine an octave and a half
  // below where a crack would sit.
  function pop(strength) {
    var ctx = audio();
    if (!ctx) { return; }
    var now = ctx.currentTime;
    strength = Math.max(0.15, Math.min(1, strength || 0.6));
    var src = ctx.createBufferSource();
    src.buffer = noiseBuffer(ctx);
    // Slowing the noise down pitches the whole burst an octave and a half
    // lower before anything else touches it.
    src.playbackRate.value = 0.38 + Math.random() * 0.2;
    var lp = ctx.createBiquadFilter();
    lp.type = 'lowpass';
    lp.frequency.setValueAtTime(1700, now);
    lp.frequency.exponentialRampToValueAtTime(200, now + 0.45);
    // Nothing above the low thousands - and the bottom is cut too, because
    // anything under seventy hertz is inaudible on the speakers in a laptop
    // and spending the sound's headroom there would only make it quieter.
    var hp = ctx.createBiquadFilter();
    hp.type = 'highpass';
    hp.frequency.value = 72;
    var g = ctx.createGain();
    g.gain.setValueAtTime(0.0008, now);
    g.gain.linearRampToValueAtTime(strength * 0.42, now + 0.012);
    g.gain.exponentialRampToValueAtTime(0.0008, now + 0.55);
    src.connect(lp); lp.connect(hp); hp.connect(g); g.connect(SOUND.gain);
    src.start(now); src.stop(now + 0.62);

    // The body of the shell: a sine falling away, which is the part you
    // feel rather than hear.
    var boom = ctx.createOscillator();
    var bg = ctx.createGain();
    boom.type = 'sine';
    boom.frequency.setValueAtTime(148 + Math.random() * 34, now);
    boom.frequency.exponentialRampToValueAtTime(62, now + 0.3);
    bg.gain.setValueAtTime(0.0008, now);
    bg.gain.linearRampToValueAtTime(strength * 0.3, now + 0.014);
    bg.gain.exponentialRampToValueAtTime(0.0008, now + 0.38);
    boom.connect(bg); bg.connect(SOUND.gain);
    boom.start(now); boom.stop(now + 0.42);
  }


  // ------------------------------------------------- the elemental voices
  //
  // One sound per kind of flourish, all synthesised like everything else
  // here: the exported sheet has no files beside it, so a wav would have
  // to be a data uri a hundred kilobytes long, and a fire made out of
  // filtered noise is both smaller and easier to tune.
  //
  // They are all quiet on purpose. This plays behind a dice roll at a
  // table, and a character sheet that roars is a character sheet nobody
  // leaves the sound on for. Each one is also short: the visual is over
  // in a second and a half, and a sound still going afterwards is a sound
  // that has come loose from what it was for.

  // A soft bed of noise, shaped by a filter and an envelope. Four of the
  // five voices below are this with different numbers, which is most of
  // what separates a fire from a wave.
  function noiseBed(ctx, opts) {
    var now = ctx.currentTime;
    var src = ctx.createBufferSource();
    src.buffer = noiseBuffer(ctx);
    src.loop = true;
    src.playbackRate.value = opts.rate || 1;
    var f = ctx.createBiquadFilter();
    f.type = opts.type || 'lowpass';
    if (opts.q !== undefined) { f.Q.value = opts.q; }
    f.frequency.setValueAtTime(opts.from, now);
    f.frequency.exponentialRampToValueAtTime(opts.to, now + opts.dur);
    var g = ctx.createGain();
    g.gain.setValueAtTime(0.0001, now);
    g.gain.linearRampToValueAtTime(opts.level, now + (opts.attack || 0.1));
    if (opts.hold) { g.gain.setValueAtTime(opts.level, now + opts.hold); }
    g.gain.exponentialRampToValueAtTime(0.0001, now + opts.dur);
    src.connect(f);
    f.connect(g);
    g.connect(SOUND.gain);
    src.start(now);
    src.stop(now + opts.dur + 0.05);
    return { src: src, filter: f, gain: g, at: now };
  }

  // One of a list, at random. The same idea as pickLook on the drawing
  // side: an effect that sounds identical every time stops being heard
  // after the third cast, and the cheapest way to keep it alive is to
  // have written several and let the roll choose.
  function pickOne(list) {
    return list[Math.floor(Math.random() * list.length)];
  }

  // crackle is a fire: a breathy bed with a scatter of pops over it.
  //
  // The pops are the whole thing. A fire with an even hiss sounds like
  // rain; what says fire is the irregular snapping, so they are scattered
  // on an uneven clock and no two are the same pitch or the same size.
  //
  // Three fires. A hearth is slow and mostly bed with the odd fat pop in
  // it; a brushfire is all quick bright ticking; a roar has a low body
  // under it and starts with the draught catching.
  var FIRES = [
    { bed: [900, 380, 0.075, 0.7], rate: 0.7, gap: [0.03, 0.13], n: 20,
      band: [420, 2600], big: 0.26, thin: 12 },
    { bed: [1800, 900, 0.055, 0.4], rate: 1.4, gap: [0.012, 0.055], n: 34,
      band: [1400, 5200], big: 0.1, thin: 26, q: [5, 9] },
    { bed: [520, 220, 0.11, 0.9], rate: 0.45, gap: [0.035, 0.16], n: 16,
      band: [260, 1700], big: 0.4, thin: 9, woomph: true }
  ];

  function crackle() {
    var ctx = audio();
    if (!ctx) { return; }
    var now = ctx.currentTime;
    var v = pickOne(FIRES);
    // The draught: wide noise, rolled right off at the top, swelling and
    // dying back the way a flame does when it takes hold.
    noiseBed(ctx, { from: v.bed[0], to: v.bed[1], dur: 1.5, level: v.bed[2],
                    attack: 0.18, hold: v.bed[3], rate: v.rate });
    // The draught catching, on the fires that have one: a soft low thump
    // under the first pops.
    if (v.woomph) {
      var wo = ctx.createOscillator();
      var wg = ctx.createGain();
      wo.type = 'sine';
      wo.frequency.setValueAtTime(110, now);
      wo.frequency.exponentialRampToValueAtTime(46, now + 0.45);
      wg.gain.setValueAtTime(0.0001, now);
      wg.gain.linearRampToValueAtTime(0.13, now + 0.06);
      wg.gain.exponentialRampToValueAtTime(0.0001, now + 0.6);
      wo.connect(wg); wg.connect(SOUND.gain);
      wo.start(now); wo.stop(now + 0.65);
    }
    // The snapping. Scheduled ahead rather than driven from the frame
    // loop, so it keeps its timing while the page is busy drawing.
    var q = v.q || [2.6, 8.6];
    var at = 0.04, i = 0;
    while (at < 1.35 && i < v.n) {
      var src = ctx.createBufferSource();
      src.buffer = noiseBuffer(ctx);
      src.playbackRate.value = 0.5 + Math.random() * 1.9;
      var band = ctx.createBiquadFilter();
      band.type = 'bandpass';
      band.Q.value = q[0] + Math.random() * (q[1] - q[0]);
      band.frequency.value = v.band[0] + Math.random() * (v.band[1] - v.band[0]);
      var g = ctx.createGain();
      var big = Math.random() < v.big;
      var peak = (big ? 0.2 : 0.075) * (0.5 + Math.random() * 0.8);
      var len = big ? 0.06 : 0.022;
      g.gain.setValueAtTime(0.0001, now + at);
      g.gain.linearRampToValueAtTime(peak, now + at + 0.003);
      g.gain.exponentialRampToValueAtTime(0.0001, now + at + len);
      src.connect(band); band.connect(g); g.connect(SOUND.gain);
      src.start(now + at);
      src.stop(now + at + len + 0.02);
      // Unevenly spaced, and thinning out as the fire settles.
      at += v.gap[0] + Math.random() * (v.gap[1] - v.gap[0]) * (1 + i / v.thin);
      i++;
    }
  }

  // wash is a wave: a swell that arrives, breaks and drains away.
  //
  // Three parts, because that is what you hear at the coast - the low
  // body of the water coming, the hiss of it breaking, and the long
  // sibilant drag of it going back over the shingle. Which of the three
  // is loudest is the whole difference between a roller coming in off
  // deep water and a foot of surf slapping a rock.
  var WAVES = [
    // Surf: even handed, the one you picture.
    { body: [240, 900, 0.16, 0.42, 0.55], brk: [0.34, 1100, 3400, 0.13, 1.25],
      drag: [2600, 5200, 0.055, 0.7, 1.6] },
    // A deep swell: mostly body, arriving slowly, hardly breaking at all.
    { body: [140, 520, 0.21, 0.66, 0.38], brk: [0.5, 700, 1900, 0.07, 1.4],
      drag: [1800, 3600, 0.04, 0.9, 1.2] },
    // Shallow water slapping stone: almost no body, a hard break, and a
    // long bright hiss off the shingle afterwards.
    { body: [420, 1300, 0.09, 0.22, 0.85], brk: [0.24, 1800, 5200, 0.17, 1.0],
      drag: [3400, 7000, 0.085, 0.5, 2.1] }
  ];

  function wash() {
    var ctx = audio();
    if (!ctx) { return; }
    var now = ctx.currentTime;
    var v = pickOne(WAVES);
    // The body arriving: dark noise swelling up.
    noiseBed(ctx, { from: v.body[0], to: v.body[1], dur: 0.95,
                    level: v.body[2], attack: v.body[3], rate: v.body[4] });
    // The break: brighter, a beat later, short.
    var t0 = v.brk[0];
    var brk = ctx.createBufferSource();
    brk.buffer = noiseBuffer(ctx);
    brk.loop = true;
    brk.playbackRate.value = 1.25;
    var bp = ctx.createBiquadFilter();
    bp.type = 'bandpass';
    bp.Q.value = 0.7;
    bp.frequency.setValueAtTime(v.brk[1], now + t0);
    bp.frequency.exponentialRampToValueAtTime(v.brk[2], now + t0 + 0.38);
    var bg = ctx.createGain();
    bg.gain.setValueAtTime(0.0001, now + t0);
    bg.gain.linearRampToValueAtTime(v.brk[3], now + t0 + 0.16);
    bg.gain.exponentialRampToValueAtTime(0.0001, now + v.brk[4]);
    brk.connect(bp); bp.connect(bg); bg.connect(SOUND.gain);
    brk.start(now + t0); brk.stop(now + v.brk[4] + 0.05);
    // The drag back: high, thin, and the last thing you hear.
    noiseBed(ctx, { type: 'highpass', from: v.drag[0], to: v.drag[1], dur: 1.5,
                    level: v.drag[2], attack: v.drag[3], rate: v.drag[4] });
  }

  // choir is the magic circle: a held chord with breath in it.
  //
  // Not a sample of a choir, which cannot be faked at this size, but the
  // thing underneath one. What makes a room full of people singing one
  // chord sound like more than one organ pipe is that it is built in
  // layers that do not line up: a sub an octave down carrying the room,
  // the chord itself, an upper octave sitting on top of it, and a thin
  // shimmer above that which nobody hears as a note. Each of those is
  // three voices a few cents apart so they beat against each other.
  //
  // That is eighteen oscillators and six more driving the vibrato, which
  // sounds like a lot for a character sheet and is about a fiftieth of
  // what the page spends on drawing the same second.
  var CHOIRS = [
    // Major, open: the plain holy one. Root, fifth, octave, third, and
    // the ninth above it, which is the note that makes it sound like a
    // room rather than a chord.
    { root: 174.6, steps: [0, 7, 12, 16, 19, 26], sub: true, shimmer: 0.5,
      vowel: [760, 1240] },
    // Minor, closed: the same voices a shade darker, for a circle that
    // is not necessarily on your side.
    { root: 146.8, steps: [0, 7, 12, 15, 19, 22], sub: true, shimmer: 0.3,
      vowel: [620, 1080] },
    // Open fifths only, no third at all: plainchant, older and emptier.
    { root: 196.0, steps: [0, 7, 12, 19, 24, 31], sub: false, shimmer: 0.65,
      vowel: [700, 1500] },
    // Lydian, bright: the fourth raised, which is the interval that
    // makes film composers reach for it whenever anything is holy.
    { root: 164.8, steps: [0, 7, 12, 16, 18, 23], sub: true, shimmer: 0.8,
      vowel: [820, 1420] }
  ];

  function choir() {
    var ctx = audio();
    if (!ctx) { return; }
    var now = ctx.currentTime;
    var v = pickOne(CHOIRS);
    var notes = v.steps.map(function (st) {
      return v.root * Math.pow(2, st / 12);
    });
    if (v.sub) { notes.unshift(v.root / 2); }

    // The formants of an "aah", roughly. Everything else about the sound
    // is a stack of tuned oscillators, and these two peaks are what turn
    // it into a vowel instead of an organ.
    var mouth = ctx.createBiquadFilter();
    mouth.type = 'bandpass';
    mouth.Q.value = 1.1;
    mouth.frequency.value = v.vowel[0];
    var mouth2 = ctx.createBiquadFilter();
    mouth2.type = 'peaking';
    mouth2.Q.value = 1.6;
    mouth2.frequency.value = v.vowel[1];
    mouth2.gain.value = 7;
    // The vowel opens a little as the chord swells, the way a held note
    // does when a singer runs out of politeness about it.
    mouth.frequency.setValueAtTime(v.vowel[0] * 0.8, now);
    mouth.frequency.linearRampToValueAtTime(v.vowel[0] * 1.12, now + 0.9);
    var bus = ctx.createGain();
    bus.gain.setValueAtTime(0.0001, now);
    // Slow in, slower out: a pad that starts sharply is not a pad.
    bus.gain.linearRampToValueAtTime(0.44, now + 0.5);
    bus.gain.setValueAtTime(0.44, now + 0.9);
    bus.gain.exponentialRampToValueAtTime(0.0001, now + 2.2);
    bus.connect(mouth); mouth.connect(mouth2); mouth2.connect(SOUND.gain);

    // Three voices per note. Sine for the body, triangle for the reed in
    // it, and a third sine a whisker sharp, which is the one doing the
    // beating. The gain is divided by how many notes there are so a
    // six-note voicing is not twice as loud as a four-note one.
    var share = 0.9 / notes.length;
    notes.forEach(function (hz, k) {
      // One vibrato per note, shared by its three voices, so they drift
      // together the way one singer does rather than three.
      var lfo = ctx.createOscillator();
      var lg = ctx.createGain();
      lfo.frequency.value = 3.8 + k * 0.37;
      lg.gain.value = hz * 0.0045;
      lfo.start(now); lfo.stop(now + 2.3);
      lfo.connect(lg);

      [[-7, 'sine'], [0, 'triangle'], [8, 'sine']].forEach(function (voice, j) {
        var o = ctx.createOscillator();
        o.type = voice[1];
        o.frequency.value = hz * Math.pow(2, voice[0] / 1200);
        lg.connect(o.frequency);
        var g = ctx.createGain();
        // The high notes are quieter than the low ones, and the upper
        // voices come in behind the lower ones, so the chord opens out
        // rather than landing all at once.
        var lvl = share / (1 + k * 0.45) * (j === 1 ? 0.7 : 1);
        g.gain.setValueAtTime(0.0001, now);
        g.gain.linearRampToValueAtTime(lvl, now + 0.38 + k * 0.1);
        g.gain.exponentialRampToValueAtTime(0.0001, now + 2.1);
        o.connect(g); g.connect(bus);
        o.start(now); o.stop(now + 2.2);
      });
    });

    // The shimmer: two very quiet partials two octaves above the root,
    // slightly out with each other. Nobody hears these as notes - they
    // are what stops the pad sounding like it has a lid on it.
    if (v.shimmer) {
      [2, 3].forEach(function (mult, j) {
        var sh = ctx.createOscillator();
        var sg = ctx.createGain();
        sh.type = 'sine';
        sh.frequency.value = v.root * 4 * mult / 2 * (j ? 1.004 : 1);
        sg.gain.setValueAtTime(0.0001, now);
        sg.gain.linearRampToValueAtTime(0.022 * v.shimmer, now + 0.8);
        sg.gain.exponentialRampToValueAtTime(0.0001, now + 2.0);
        sh.connect(sg); sg.connect(SOUND.gain);
        sh.start(now); sh.stop(now + 2.1);
      });
    }

    // A breath of air behind the voices, which is what stops it sounding
    // like a synthesiser pretending.
    noiseBed(ctx, { type: 'bandpass', q: 1.4, from: v.vowel[0], to: v.vowel[1],
                    dur: 1.9, level: 0.035, attack: 0.6, rate: 0.6 });
  }

  // rumble is stone: something very heavy moving somewhere below.
  //
  // Mostly felt rather than heard. A sine sliding down under everything,
  // a wide band of low noise for the mass of it, and a narrow band with
  // a slow wobble on top, which is the grinding.
  var STONES = [
    // A slab shifting: the grinding one.
    { sub: [74, 38, 0.28, 1.1], bed: [320, 110, 0.19, 0.45], grind: 0.1,
      wob: 5.5, dur: 1.4 },
    // Something much bigger and further off. Almost no grind, a very low
    // sub, and it takes its time.
    { sub: [52, 26, 0.34, 1.7], bed: [200, 70, 0.22, 0.8], grind: 0.04,
      wob: 2.1, dur: 1.9 },
    // Rubble: short, drier, and with loose stone ticking down it.
    { sub: [96, 54, 0.2, 0.7], bed: [460, 190, 0.15, 0.25], grind: 0.07,
      wob: 9, dur: 1.0, ticks: 14 }
  ];

  function rumble() {
    var ctx = audio();
    if (!ctx) { return; }
    var now = ctx.currentTime;
    var v = pickOne(STONES);
    var sub = ctx.createOscillator();
    var sg = ctx.createGain();
    sub.type = 'sine';
    sub.frequency.setValueAtTime(v.sub[0], now);
    sub.frequency.exponentialRampToValueAtTime(v.sub[1], now + v.sub[3]);
    sg.gain.setValueAtTime(0.0001, now);
    sg.gain.linearRampToValueAtTime(v.sub[2], now + 0.14);
    sg.gain.exponentialRampToValueAtTime(0.0001, now + v.dur);
    sub.connect(sg); sg.connect(SOUND.gain);
    sub.start(now); sub.stop(now + v.dur + 0.05);
    // The mass of it.
    noiseBed(ctx, { from: v.bed[0], to: v.bed[1], dur: v.dur, level: v.bed[2],
                    attack: 0.1, hold: v.bed[3], rate: 0.32 });
    // The grinding: a narrow band scraped slowly across, which is two
    // faces of rock going past each other.
    var grind = noiseBed(ctx, { type: 'bandpass', q: 7, from: 340, to: 190,
                                dur: v.dur * 0.9, level: v.grind,
                                attack: 0.2, rate: 0.4 });
    var wob = ctx.createOscillator();
    var wg = ctx.createGain();
    wob.frequency.value = v.wob;
    wg.gain.value = 90;
    wob.connect(wg); wg.connect(grind.filter.frequency);
    wob.start(now); wob.stop(now + v.dur);
    // Loose stone coming down after it, on the variant that has any.
    var i, tAt = 0.18;
    for (i = 0; i < (v.ticks || 0); i++) {
      var tick = ctx.createBufferSource();
      tick.buffer = noiseBuffer(ctx);
      tick.playbackRate.value = 0.3 + Math.random() * 0.5;
      var tb = ctx.createBiquadFilter();
      tb.type = 'bandpass';
      tb.Q.value = 4 + Math.random() * 5;
      tb.frequency.value = 180 + Math.random() * 700;
      var tg = ctx.createGain();
      tg.gain.setValueAtTime(0.0001, now + tAt);
      tg.gain.linearRampToValueAtTime(0.05 + Math.random() * 0.06, now + tAt + 0.004);
      tg.gain.exponentialRampToValueAtTime(0.0001, now + tAt + 0.05);
      tick.connect(tb); tb.connect(tg); tg.connect(SOUND.gain);
      tick.start(now + tAt); tick.stop(now + tAt + 0.07);
      tAt += 0.025 + Math.random() * 0.09;
    }
  }

  // thunder is the strike: the crack, and then the sky.
  //
  // The crack has to arrive before the eye has finished with the flash
  // or the two come apart, so there is no delay on it at all - which is
  // wrong for a strike a mile off and right for one on the table.
  var SKIES = [
    // Close: mostly crack, a short roll behind it.
    { crack: [0.3, 2200, 600, 0.18, 1.8], roll: [700, 90, 0.17, 1.5],
      sub: [96, 42, 0.24, 0.8] },
    // Further off: hardly any crack, and a long roll that takes its time
    // getting out of the room.
    { crack: [0.14, 1400, 400, 0.3, 1.2], roll: [420, 60, 0.2, 2.2],
      sub: [72, 32, 0.26, 1.3] },
    // A peal: the crack, and then a second roll arriving late over the
    // first, which is what a strike sounds like off a valley wall.
    { crack: [0.34, 2800, 700, 0.15, 2.1], roll: [820, 110, 0.15, 1.6],
      sub: [110, 46, 0.2, 0.7], echo: [0.42, 520, 80, 0.11, 1.4] }
  ];

  function thunder(size) {
    var ctx = audio();
    if (!ctx) { return; }
    var now = ctx.currentTime;
    size = size === undefined ? 1 : size;
    var v = pickOne(SKIES);
    // The crack: a very short burst of bright noise, gone in a blink.
    var crack = ctx.createBufferSource();
    crack.buffer = noiseBuffer(ctx);
    crack.playbackRate.value = v.crack[4];
    var hp = ctx.createBiquadFilter();
    hp.type = 'highpass';
    hp.frequency.setValueAtTime(v.crack[1], now);
    hp.frequency.exponentialRampToValueAtTime(v.crack[2], now + 0.18);
    var cg = ctx.createGain();
    cg.gain.setValueAtTime(0.0001, now);
    cg.gain.linearRampToValueAtTime(v.crack[0] * size, now + 0.004);
    cg.gain.exponentialRampToValueAtTime(0.0001, now + v.crack[3] + 0.06);
    crack.connect(hp); hp.connect(cg); cg.connect(SOUND.gain);
    crack.start(now); crack.stop(now + v.crack[3] + 0.1);
    // The roll after it, falling away and taking its time about it.
    noiseBed(ctx, { from: v.roll[0], to: v.roll[1],
                    dur: v.roll[3] * (0.7 + size * 0.5),
                    level: v.roll[2] * size, attack: 0.06, hold: 0.2, rate: 0.3 });
    var sub = ctx.createOscillator();
    var sg2 = ctx.createGain();
    sub.type = 'sine';
    sub.frequency.setValueAtTime(v.sub[0], now);
    sub.frequency.exponentialRampToValueAtTime(v.sub[1], now + v.sub[3]);
    sg2.gain.setValueAtTime(0.0001, now);
    sg2.gain.linearRampToValueAtTime(v.sub[2] * size, now + 0.02);
    sg2.gain.exponentialRampToValueAtTime(0.0001, now + v.sub[3] + 0.2);
    sub.connect(sg2); sg2.connect(SOUND.gain);
    sub.start(now); sub.stop(now + v.sub[3] + 0.25);
    // The second roll off the hillside, for the sky that has one.
    if (v.echo) {
      var ec = ctx.createBufferSource();
      ec.buffer = noiseBuffer(ctx);
      ec.loop = true;
      ec.playbackRate.value = 0.3;
      var ef = ctx.createBiquadFilter();
      ef.type = 'lowpass';
      ef.frequency.setValueAtTime(v.echo[1], now + v.echo[0]);
      ef.frequency.exponentialRampToValueAtTime(v.echo[2], now + v.echo[0] + v.echo[4]);
      var eg = ctx.createGain();
      eg.gain.setValueAtTime(0.0001, now + v.echo[0]);
      eg.gain.linearRampToValueAtTime(v.echo[3] * size, now + v.echo[0] + 0.12);
      eg.gain.exponentialRampToValueAtTime(0.0001, now + v.echo[0] + v.echo[4]);
      ec.connect(ef); ef.connect(eg); eg.connect(SOUND.gain);
      ec.start(now + v.echo[0]); ec.stop(now + v.echo[0] + v.echo[4] + 0.05);
    }
  }

  // creak is something being tied: rope taking the strain, and the fibre
  // giving up a little at a time while it does.
  //
  // Creaking is stick-slip - a thing gripping, letting go a fraction and
  // gripping again - so it cannot be one smooth tone. What makes it read
  // is a low note whose pitch and volume both jerk on an uneven clock,
  // with dry little pops of fibre over the top.
  //
  // Which of the three you get follows what is actually on the screen:
  // hemp creaks, chain clinks, vines are wetter and quieter and rustle.
  var LASHINGS = {
    twist: { note: 'sawtooth', hz: [92, 148], band: [320, 1100], q: 6,
             jerks: 9, pops: 14, popBand: [900, 4200], level: 0.1,
             strain: [500, 2300, 0.075] },
    links: { note: 'square', hz: [210, 340], band: [1400, 4200], q: 11,
             jerks: 5, pops: 20, popBand: [2200, 7000], level: 0.055,
             strain: [1800, 5200, 0.05], metal: true },
    leaves: { note: 'triangle', hz: [70, 112], band: [220, 780], q: 4,
              jerks: 7, pops: 9, popBand: [400, 1900], level: 0.085,
              strain: [300, 1400, 0.09] }
  };

  function creak(lay) {
    var ctx = audio();
    if (!ctx) { return; }
    var now = ctx.currentTime;
    var v = LASHINGS[lay] || LASHINGS.twist;
    // The note the rope is making, bent upward as it comes tight.
    var o = ctx.createOscillator();
    o.type = v.note;
    o.frequency.setValueAtTime(v.hz[0], now);
    var band = ctx.createBiquadFilter();
    band.type = 'bandpass';
    band.Q.value = v.q;
    band.frequency.setValueAtTime(v.band[0], now);
    band.frequency.exponentialRampToValueAtTime(v.band[1], now + 0.9);
    var g = ctx.createGain();
    g.gain.setValueAtTime(0.0001, now);
    // Stick, slip, stick. Each jerk is a step up in pitch and a bump in
    // level, on an uneven clock, and between them it sags back.
    var at = 0.05, k, hz = v.hz[0];
    for (k = 0; k < v.jerks; k++) {
      hz += (v.hz[1] - v.hz[0]) / v.jerks * (0.4 + Math.random() * 1.3);
      o.frequency.setValueAtTime(hz * (0.94 + Math.random() * 0.12), now + at);
      g.gain.linearRampToValueAtTime(v.level * (0.45 + Math.random() * 0.75),
                                     now + at + 0.012);
      g.gain.linearRampToValueAtTime(v.level * 0.18, now + at + 0.07);
      at += 0.035 + Math.random() * 0.13;
      if (at > 1.0) { break; }
    }
    g.gain.exponentialRampToValueAtTime(0.0001, now + 1.25);
    o.connect(band); band.connect(g); g.connect(SOUND.gain);
    o.start(now); o.stop(now + 1.3);

    // The fibre popping - or, for a chain, links knocking together.
    var pAt = 0.03;
    for (k = 0; k < v.pops; k++) {
      var src = ctx.createBufferSource();
      src.buffer = noiseBuffer(ctx);
      src.playbackRate.value = v.metal ? 1.4 + Math.random() : 0.6 + Math.random();
      var pb = ctx.createBiquadFilter();
      pb.type = 'bandpass';
      pb.Q.value = v.metal ? 14 + Math.random() * 14 : 4 + Math.random() * 7;
      pb.frequency.value = v.popBand[0] +
        Math.random() * (v.popBand[1] - v.popBand[0]);
      var pg = ctx.createGain();
      var len = v.metal ? 0.05 : 0.016;
      pg.gain.setValueAtTime(0.0001, now + pAt);
      pg.gain.linearRampToValueAtTime(0.03 + Math.random() * 0.05, now + pAt + 0.002);
      pg.gain.exponentialRampToValueAtTime(0.0001, now + pAt + len);
      src.connect(pb); pb.connect(pg); pg.connect(SOUND.gain);
      src.start(now + pAt); src.stop(now + pAt + len + 0.02);
      pAt += 0.02 + Math.random() * 0.12;
      if (pAt > 1.1) { break; }
    }

    // And the hiss of it drawing tight through itself.
    noiseBed(ctx, { type: 'bandpass', q: 1.8, from: v.strain[0], to: v.strain[1],
                    dur: 0.95, level: v.strain[2], attack: 0.3, rate: 0.9 });
  }

  // howl is the storm: wind with something turning inside it.
  //
  // Two bands of noise sweeping past each other at different speeds is
  // most of a wind; the whistle is a third, much narrower, wandering
  // slowly, which is what an edge in the airstream actually does.
  var STORMS = [
    { low: [180, 420, 0.15], air: [1400, 3800, 0.1], whistle: [900, 2, 0.05] },
    { low: [110, 260, 0.2], air: [900, 2400, 0.07], whistle: [1500, 1.2, 0.035] },
    { low: [240, 600, 0.12], air: [2200, 5600, 0.13], whistle: [2400, 3.1, 0.06] }
  ];

  function howl() {
    var ctx = audio();
    if (!ctx) { return; }
    var now = ctx.currentTime;
    var v = pickOne(STORMS);
    noiseBed(ctx, { type: 'lowpass', from: v.low[0], to: v.low[1], dur: 1.7,
                    level: v.low[2], attack: 0.35, hold: 0.8, rate: 0.4 });
    noiseBed(ctx, { type: 'bandpass', q: 0.8, from: v.air[0], to: v.air[1],
                    dur: 1.8, level: v.air[2], attack: 0.5, hold: 0.9, rate: 1.7 });
    // The whistle: one narrow band wandering, which is the note in a gale.
    var w = noiseBed(ctx, { type: 'bandpass', q: 18, from: v.whistle[0],
                            to: v.whistle[0] * 1.6, dur: 1.6,
                            level: v.whistle[2], attack: 0.55, rate: 1.2 });
    var lfo = ctx.createOscillator();
    var lg = ctx.createGain();
    lfo.frequency.value = v.whistle[1];
    lg.gain.value = v.whistle[0] * 0.35;
    lfo.connect(lg); lg.connect(w.filter.frequency);
    lfo.start(now); lfo.stop(now + 1.6);
  }

  // murmur is the ghost hand: the same held voices as the magic circle,
  // but half of them and half as long.
  //
  // The first go at this was a bell, and a bell is the wrong instrument
  // entirely - it has a hard edge on the front of it and a hand fading
  // up out of a page has no edge anywhere. So it is built the way choir
  // is: a few notes, two detuned voices each, through the same pair of
  // "aah" formant filters, with no attack to speak of. Three notes
  // rather than six and a second and a half rather than two, so it sits
  // behind the circle's chord rather than competing with it.
  var MURMURS = [
    { root: 261.6, steps: [0, 7, 12], vowel: [740, 1180] },
    { root: 293.7, steps: [0, 5, 12], vowel: [680, 1320] },
    { root: 220.0, steps: [0, 7, 16], vowel: [800, 1260] },
    { root: 246.9, steps: [0, 9, 14], vowel: [700, 1400] }
  ];

  function murmur() {
    var ctx = audio();
    if (!ctx) { return; }
    var now = ctx.currentTime;
    var v = pickOne(MURMURS);
    var mouth = ctx.createBiquadFilter();
    mouth.type = 'bandpass';
    mouth.Q.value = 1.1;
    mouth.frequency.setValueAtTime(v.vowel[0] * 0.85, now);
    mouth.frequency.linearRampToValueAtTime(v.vowel[0] * 1.1, now + 0.7);
    var mouth2 = ctx.createBiquadFilter();
    mouth2.type = 'peaking';
    mouth2.Q.value = 1.6;
    mouth2.frequency.value = v.vowel[1];
    mouth2.gain.value = 6;
    var bus = ctx.createGain();
    bus.gain.setValueAtTime(0.0001, now);
    bus.gain.linearRampToValueAtTime(0.38, now + 0.34);
    bus.gain.setValueAtTime(0.38, now + 0.6);
    bus.gain.exponentialRampToValueAtTime(0.0001, now + 1.55);
    bus.connect(mouth); mouth.connect(mouth2); mouth2.connect(SOUND.gain);

    v.steps.forEach(function (st, k) {
      var hz = v.root * Math.pow(2, st / 12);
      var lfo = ctx.createOscillator();
      var lg = ctx.createGain();
      lfo.frequency.value = 4.2 + k * 0.5;
      lg.gain.value = hz * 0.004;
      lfo.connect(lg);
      lfo.start(now); lfo.stop(now + 1.6);
      [[-6, 'sine'], [7, 'triangle']].forEach(function (voice) {
        var o = ctx.createOscillator();
        o.type = voice[1];
        o.frequency.value = hz * Math.pow(2, voice[0] / 1200);
        lg.connect(o.frequency);
        var g = ctx.createGain();
        g.gain.setValueAtTime(0.0001, now);
        g.gain.linearRampToValueAtTime(0.16 / (k + 1.3), now + 0.28 + k * 0.09);
        g.gain.exponentialRampToValueAtTime(0.0001, now + 1.5);
        o.connect(g); g.connect(bus);
        o.start(now); o.stop(now + 1.55);
      });
    });
    // The breath, which is the part that says voices rather than organ.
    noiseBed(ctx, { type: 'bandpass', q: 1.5, from: v.vowel[0], to: v.vowel[1],
                    dur: 1.4, level: 0.03, attack: 0.45, rate: 0.6 });
  }

  // whoosh is a blade going past: air, and then the edge of it.
  //
  // A swing is nearly all noise sweeping upward in pitch as the blade
  // accelerates and then dropping away as it passes - the doppler of
  // something going by your ear - with a short ring on the end for the
  // steel. The ring is what tells a sword from a stick.
  var SWINGS = [
    // Steel: bright, a clear ring after it.
    { band: [380, 2600, 900], q: 2.2, level: 0.14, ring: [2400, 0.07, 0.5] },
    // Something heavier: lower, slower, and it thuds rather than rings.
    { band: [220, 1400, 500], q: 1.6, level: 0.18, ring: [900, 0.05, 0.28] },
    // Fast and light, three-quarters air.
    { band: [600, 4200, 1600], q: 3.1, level: 0.11, ring: [3600, 0.05, 0.36] }
  ];

  function whoosh() {
    var ctx = audio();
    if (!ctx) { return; }
    var now = ctx.currentTime;
    var v = pickOne(SWINGS);
    var src = ctx.createBufferSource();
    src.buffer = noiseBuffer(ctx);
    src.loop = true;
    src.playbackRate.value = 1.1;
    var band = ctx.createBiquadFilter();
    band.type = 'bandpass';
    band.Q.value = v.q;
    // Up as it comes round, down as it goes past.
    band.frequency.setValueAtTime(v.band[0], now);
    band.frequency.exponentialRampToValueAtTime(v.band[1], now + 0.11);
    band.frequency.exponentialRampToValueAtTime(v.band[2], now + 0.34);
    var g = ctx.createGain();
    g.gain.setValueAtTime(0.0001, now);
    g.gain.linearRampToValueAtTime(v.level, now + 0.09);
    g.gain.exponentialRampToValueAtTime(0.0001, now + 0.42);
    src.connect(band); band.connect(g); g.connect(SOUND.gain);
    src.start(now); src.stop(now + 0.45);
    // The edge ringing afterwards.
    var o = ctx.createOscillator();
    var og = ctx.createGain();
    o.type = 'sine';
    o.frequency.setValueAtTime(v.ring[0], now + 0.1);
    o.frequency.exponentialRampToValueAtTime(v.ring[0] * 0.92, now + 0.1 + v.ring[2]);
    og.gain.setValueAtTime(0.0001, now + 0.1);
    og.gain.linearRampToValueAtTime(v.ring[1], now + 0.12);
    og.gain.exponentialRampToValueAtTime(0.0001, now + 0.1 + v.ring[2]);
    o.connect(og); og.connect(SOUND.gain);
    o.start(now + 0.1); o.stop(now + 0.15 + v.ring[2]);
  }

  // Which voice a flourish has. Keyed on the kind rather than on the
  // element, so a ruleset module that adds a spell doing cold damage
  // gets the ice sound with no wiring at all - and a kind with no entry
  // here is silent, which is the right default for a new one.
  var ELEM_VOICE = {
    blaze: crackle,
    wave: wash,
    circle: choir,
    stones: rumble,
    bolt: thunder,
    blizzard: howl,
    hand: murmur,
    slash: whoosh,
    // A binding sounds like whatever it is made of, and the look has
    // already chosen that - so this one is handed the lay rather than a
    // loudness, and hemp, chain and vine each get their own noise.
    bind: function (arg) { creak(arg); }
  };

  function elemVoice(kind, look) {
    var play = ELEM_VOICE[kind];
    if (!play || !SOUND.on) { return; }
    var arg = 1;
    // A big strike is louder than a flicker of one; a binding is told
    // what it is made of instead.
    if (kind === 'bolt') { arg = 0.7 + (look && look.peak ? look.peak : 0.3); }
    if (kind === 'bind') { arg = look && look.lay; }
    try { play(arg); }
    catch (e) { /* audio is a nicety, never a reason to lose the picture */ }
  }


  // stab is a natural 1: a short scrape of steel and then something wet.
  function stab() {
    var ctx = audio();
    if (!ctx) { return; }
    var now = ctx.currentTime;
    // The blade going in - a band of noise sliding downwards.
    var src = ctx.createBufferSource();
    src.buffer = noiseBuffer(ctx);
    src.playbackRate.value = 1.4;
    var band = ctx.createBiquadFilter();
    band.type = 'bandpass';
    band.Q.value = 5;
    band.frequency.setValueAtTime(2600, now);
    band.frequency.exponentialRampToValueAtTime(320, now + 0.22);
    var g = ctx.createGain();
    g.gain.setValueAtTime(0, now);
    g.gain.linearRampToValueAtTime(0.4, now + 0.006);
    g.gain.exponentialRampToValueAtTime(0.0008, now + 0.26);
    src.connect(band); band.connect(g); g.connect(SOUND.gain);
    src.start(now); src.stop(now + 0.3);
    // And the splatter after it, lower and softer.
    var wet = ctx.createBufferSource();
    wet.buffer = noiseBuffer(ctx);
    wet.playbackRate.value = 0.45;
    var lp = ctx.createBiquadFilter();
    lp.type = 'lowpass';
    lp.frequency.setValueAtTime(900, now + 0.05);
    lp.frequency.exponentialRampToValueAtTime(160, now + 0.3);
    var wg = ctx.createGain();
    wg.gain.setValueAtTime(0, now + 0.05);
    wg.gain.linearRampToValueAtTime(0.3, now + 0.075);
    wg.gain.exponentialRampToValueAtTime(0.0008, now + 0.34);
    wet.connect(lp); lp.connect(wg); wg.connect(SOUND.gain);
    wet.start(now + 0.05); wet.stop(now + 0.38);
    tone(88, 0.04, 0.16, 0.16, 'sine');
  }

  // ------------------------------------------------------------------- mood
  //
  // How the page feels, which is the halo bled in round its edge and how hard
  // the backdrop is pushed. It is the one piece of the sheet that says
  // something without words: a player glancing up from the table sees the
  // paper going orange before they have read a number off it.
  //
  // Everything here is set as css variables on the root, so recolouring costs
  // one style write and the transition does the rest. Nothing is laid out
  // again and no text moves.
  var halo;

  // What each band of hit points looks like. The colours are the same ones the
  // hit point bar is banded in - see HealthLevel in health.go, and hpLevel
  // above - so the edge of the page and the bar always agree.
  var MOODS = {
    hale:     { rgb: '139, 126, 102', a: 0.16, wash: 0 },
    hurt:     { rgb: '150, 122, 58',  a: 0.20, wash: 0.02 },
    bloodied: { rgb: '158, 92, 38',   a: 0.26, wash: 0.05 },
    dying:    { rgb: '150, 40, 30',   a: 0.34, wash: 0.09 },
    // Down altogether: the colour goes out of the world rather than getting
    // redder. Being unconscious is not being hurt more.
    down:     { rgb: '92, 88, 84',    a: 0.40, wash: 0.13 },
    dead:     { rgb: '60, 58, 56',    a: 0.46, wash: 0.16 }
  };

  // A condition that is worth colouring the page for. Most of them are not -
  // being prone does not change how the room looks - so only the few that
  // read as something in the air get a tint, and hit points always win.
  var CONDITION_MOODS = {
    poisoned: { rgb: '74, 116, 52', a: 0.26 },
    petrified: { rgb: '104, 104, 110', a: 0.30 },
    frightened: { rgb: '70, 62, 110', a: 0.26 },
    charmed: { rgb: '132, 74, 122', a: 0.24 },
    unconscious: { rgb: '92, 88, 84', a: 0.38 },
    blinded: { rgb: '52, 50, 48', a: 0.34 }
  };

  function moodFor(hp) {
    if (!hp) { return MOODS.hale; }
    if (hp.dead) { return MOODS.dead; }
    if (hp.down) { return MOODS.down; }
    return MOODS[hp.level] || MOODS.hale;
  }

  // paintMood is the one place that writes the mood variables. It is called
  // whenever the hit points move, a condition goes on or off, or a spell is
  // taken up or let go, and works the whole answer out again each time rather
  // than trying to remember what it last did.
  // The portrait carries the hit points too. It is the same banding the bar
  // and the edge of the page use - HealthLevel in health.go - so the picture
  // going grey and the bar going red are always the same fact said twice.
  var PORTRAIT_STATES = ['hp-hale', 'hp-hurt', 'hp-bloodied', 'hp-dying',
                         'hp-down', 'hp-dead'];

  function paintPortrait() {
    var por = document.querySelector('.portrait');
    if (!por) { return; }
    var hp = (HEALTH.state && HEALTH.state.hp) || null;
    var state = 'hp-hale';
    if (hp) {
      state = hp.dead ? 'hp-dead' : hp.down ? 'hp-down' : 'hp-' + (hp.level || 'hale');
    }
    PORTRAIT_STATES.forEach(function (c) { por.classList.toggle(c, c === state); });
    // Inspiration puts the light back, and beats being merely hurt: it is
    // the one thing on the sheet that is unambiguously good news.
    por.classList.toggle('inspired', !!(INSP && INSP.held));
  }

  function paintMood() {
    if (!halo) { return; }
    paintPortrait();
    var hp = (HEALTH.state && HEALTH.state.hp) || null;
    var mood = moodFor(hp);
    // A condition colours the page only while the hit points are not already
    // saying something louder. Being poisoned at full health is worth a green
    // edge; being poisoned at two hit points is not the headline.
    if (hp && !hp.down && (hp.level === 'hale' || hp.level === 'hurt')) {
      var conds = defConditions().active || [];
      for (var i = 0; i < conds.length; i++) {
        var tint = CONDITION_MOODS[String(conds[i].id || '').toLowerCase()];
        if (tint) { mood = { rgb: tint.rgb, a: tint.a, wash: mood.wash }; break; }
      }
    }
    var root = document.documentElement;
    root.style.setProperty('--mood-rgb', mood.rgb);
    root.style.setProperty('--mood-a', String(mood.a));
    root.style.setProperty('--mood-wash', String(mood.wash));
    halo.setAttribute('data-level', (hp && hp.level) || 'hale');
    // Nearly out of hit points and still up: the edge breathes. Once you are
    // down it stops - there is no breath left to take, and a page that kept
    // pulsing over an unconscious character would read as hopeful.
    //
    // The band is what matters here, not the death saves: "dying" on the hit
    // point view is the one and hp.dying the other, and only the first of them
    // can be true while the character is still standing.
    halo.classList.toggle('breathing', !!(hp && hp.level === 'dying' && !hp.down));
    // A spell being held lays a thin violet line inside the edge, on top of
    // whatever the hit points said, so the page carries the fact as well as
    // the band beside the hit points does.
    halo.classList.toggle('holding', !!(CONC.view && CONC.view.on));
  }

  // moodFlash is one swell of the halo and gone: a hit landing, a critical, a
  // natural 1. The class is taken off first so the animation can be replayed
  // while it is still running, which in a fight it will be.
  function moodFlash(kind) {
    if (!halo) { return; }
    var cls = kind === 'crit' ? 'crit' : 'struck';
    halo.classList.remove('struck', 'crit');
    void halo.offsetWidth;
    halo.classList.add(cls);
    // Take the class off again once the animation has run, so the halo is not
    // left wearing the last thing that happened to it.
    setTimeout(function () { halo.classList.remove(cls); }, 1100);
    // A critical is worth a moment of colour as well as a swell.
    if (kind === 'crit' || kind === 'fumble') {
      var was = document.documentElement.style.getPropertyValue('--mood-rgb');
      var wasA = document.documentElement.style.getPropertyValue('--mood-a');
      document.documentElement.style.setProperty('--mood-rgb',
        kind === 'crit' ? '184, 134, 11' : '96, 92, 88');
      document.documentElement.style.setProperty('--mood-a',
        kind === 'crit' ? '0.42' : '0.34');
      setTimeout(function () {
        // Put back whatever the mood was, rather than a remembered value: the
        // hit points may well have moved in the second the flourish took.
        document.documentElement.style.setProperty('--mood-rgb', was || '139, 126, 102');
        document.documentElement.style.setProperty('--mood-a', wasA || '0.16');
        paintMood();
      }, kind === 'crit' ? 900 : 620);
    }
  }

  // ------------------------------------------------------------- level up
  //
  // The one thing on the sheet that is a conversation rather than a button.
  // Gaining a level asks a handful of questions - how the hit points came,
  // which fighting style, which spells, where the ability points go - and
  // they depend on each other, so they have to be asked in order.
  //
  // The engine already knows how to do this: levelup.go emits exactly the
  // same Prompt the character builder does, and `orgs dnd levelup` walks
  // them on the command line. This is that walk in a drawer. Nothing about
  // the rules lives here; the tray asks what it is told to ask and sends
  // back what was picked.
  //
  // The flow is stateless by design (see POST /dnd/levelup): every call
  // carries every answer given so far and the engine replays them. That is
  // what makes Back free - drop the last answer and ask again - and what
  // makes closing the tray halfway cost nothing at all. Nothing is written
  // until the last call, which is the only one that says commit.
  // What the rules engine said this character has and what the next level
  // costs. Read off the sheet rather than asked for again: the same two
  // numbers already draw the ring round the portrait, so the tray and the
  // ring can never disagree.
  var XP = { now: 0, next: 0, level: 0 };

  var LVL = { open: false, plan: null, answers: {}, order: [], seed: 0,
              klass: '', levels: 1, busy: false, error: '', picked: [],
              custom: '', committed: false, classes: null };
  var lvlTab, lvlTray, lvlBody, lvlFoot, lvlWho;

  var LVL_ICON = '<svg viewBox="0 0 24 24" aria-hidden="true">' +
    '<path d="M12 3.2 20 11h-4.4v9.2H8.4V11H4z"/></svg>';

  function lvlConfigured() { return !!LOG.file; }

  function buildLevelUi() {
    lvlTab = document.createElement('button');
    lvlTab.id = 'lvl-tab';
    lvlTab.type = 'button';
    lvlTab.title = 'Gain a level';
    lvlTab.setAttribute('aria-expanded', 'false');
    lvlTab.innerHTML = LVL_ICON + '<span>Level</span>';
    document.body.appendChild(lvlTab);

    lvlTray = document.createElement('aside');
    lvlTray.id = 'lvl-tray';
    lvlTray.setAttribute('aria-label', 'Level up');
    lvlTray.innerHTML =
      '<div class="lvl-head"><h2>Level Up</h2>' +
        '<span class="lvl-who" id="lvl-who"></span>' +
        '<button type="button" class="dt-btn dt-x" id="lvl-close" ' +
          'aria-label="Close level up">&times;</button></div>' +
      '<div class="lvl-body" id="lvl-body"></div>' +
      '<div class="lvl-foot" id="lvl-foot"></div>';
    document.body.appendChild(lvlTray);
    lvlBody = lvlTray.querySelector('#lvl-body');
    lvlFoot = lvlTray.querySelector('#lvl-foot');
    lvlWho = lvlTray.querySelector('#lvl-who');

    lvlTab.addEventListener('click', function () { setLevelTray(!LVL.open); });
    lvlTray.querySelector('#lvl-close').addEventListener('click', function () {
      setLevelTray(false);
    });
    lvlTray.addEventListener('click', lvlClick);
    lvlTray.addEventListener('keydown', function (e) {
      if (e.key === 'Enter' && e.target.id === 'lvl-custom') {
        e.preventDefault();
        lvlAnswer();
      }
    });
  }

  function setLevelTray(open) {
    if (open && tray && tray.classList.contains('open')) { setTray(false); }
    LVL.open = open;
    lvlTray.classList.toggle('open', open);
    lvlTab.classList.toggle('shifted', open);
    lvlTab.setAttribute('aria-expanded', open ? 'true' : 'false');
    if (!open) { return; }
    // Starting fresh each time the drawer opens: a half finished climb that
    // was abandoned is not a thing worth remembering, and the engine has no
    // memory of it either.
    LVL.answers = {};
    LVL.order = [];
    LVL.picked = [];
    LVL.custom = '';
    LVL.error = '';
    LVL.committed = false;
    LVL.seed = 0;
    LVL.levels = 1;
    LVL.klass = '';
    lvlAsk(false);
  }

  // lvlAsk is the one call this makes. Every answer so far goes with it and
  // the engine replays them, which is why there is nothing to keep in sync.
  function lvlAsk(commit) {
    if (!lvlConfigured()) {
      LVL.error = 'this sheet does not know which org file it came from';
      lvlRender();
      return Promise.resolve();
    }
    LVL.busy = true;
    lvlRender();
    return api('POST', '/dnd/levelup', {
      filename: LOG.file, levels: LVL.levels, class: LVL.klass,
      answers: LVL.answers, seed: LVL.seed, commit: !!commit
    }).then(function (plan) {
      LVL.busy = false;
      LVL.error = plan.error || '';
      LVL.plan = plan;
      if (plan.seed) { LVL.seed = plan.seed; }
      LVL.picked = [];
      LVL.custom = '';
      if (commit) { LVL.committed = true; }
      lvlRender();
      return plan;
    }, function (err) {
      LVL.busy = false;
      LVL.error = err.message || String(err);
      lvlRender();
    });
  }

  // ---- drawing it --------------------------------------------------------
  // How far along this level the character is, said in the two ways that
  // matter at the table: the bar, and the one number a player actually
  // asks for - how much more experience until the next one.
  function xpBar() {
    if (!XP.next) {
      // Level 20, or a table that does not track experience at all.
      return XP.now
        ? '<div class="lvl-xp"><div class="lvl-xp-line">' +
          '<span>' + XP.now.toLocaleString() + ' XP</span>' +
          '<em>no further levels</em></div></div>'
        : '';
    }
    var left = Math.max(0, XP.next - XP.now);
    var floor = xpFloor(XP.level);
    var span = Math.max(1, XP.next - floor);
    var pct = Math.max(0, Math.min(100, ((XP.now - floor) / span) * 100));
    return '<div class="lvl-xp">' +
      '<div class="lvl-xp-line">' +
        '<span>' + XP.now.toLocaleString() + ' / ' + XP.next.toLocaleString() +
          ' XP</span>' +
        '<em>' + (left ? left.toLocaleString() + ' to level ' + (XP.level + 1)
                       : 'ready to level') + '</em>' +
      '</div>' +
      '<div class="lvl-xp-bar"' + (left ? '' : ' data-full="1"') + '>' +
        '<i style="width:' + pct.toFixed(1) + '%"></i></div></div>';
  }

  // Where this level started. The sheet is only handed the next threshold,
  // so the bar needs the one below it to know how long the level is; the
  // SRD table is fixed, and a ruleset that changes it changes nextLevelXp
  // with it, which is what the bar is anchored on.
  var XP_TABLE = [0, 0, 300, 900, 2700, 6500, 14000, 23000, 34000, 48000,
                  64000, 85000, 100000, 120000, 140000, 165000, 195000,
                  225000, 265000, 305000, 355000];

  function xpFloor(level) {
    if (level > 0 && level < XP_TABLE.length) { return XP_TABLE[level]; }
    return 0;
  }

  function lvlRender() {
    if (!lvlBody) { return; }
    var p = LVL.plan;
    lvlWho.textContent = p ? (p.character || '') : '';

    if (LVL.committed) { lvlRenderWritten(p); return; }
    if (!p) {
      lvlBody.innerHTML = xpBar() + (LVL.error
        ? '<div class="lvl-err">' + esc(LVL.error) + '</div>'
        : '<div class="dt-empty">Asking the rules what this level wants…</div>');
      lvlFoot.innerHTML = '';
      return;
    }

    var head = '<div class="lvl-arc"><b>' + p.fromLevel + '</b>' +
      '<span class="to">&rarr; ' + p.toLevel + '</span>' +
      '<span class="cls">' + esc(p.className || p.class || '') + '</span></div>' +
      xpBar();

    // One pip per question the climb asks, filled in as they are answered,
    // so how much is left is visible without reading anything.
    var asked = p.asked || [];
    if (asked.length) {
      head += '<div class="lvl-steps">' + asked.map(function (id) {
        var done = LVL.answers[id] !== undefined;
        var now = p.next && p.next.step === id;
        return '<span class="lvl-step' + (done ? ' done' : '') +
          (now ? ' now' : '') + '"></span>';
      }).join('') + '</div>';
    }

    if (p.done) { lvlRenderDone(head, p); return; }
    lvlRenderQuestion(head, p);
  }

  function lvlRenderQuestion(head, p) {
    var q = p.next;
    if (!q) {
      lvlBody.innerHTML = head + '<div class="lvl-err">The rules asked nothing ' +
        'and finished nothing, which should not happen.</div>';
      lvlFoot.innerHTML = '';
      return;
    }
    var many = q.kind === 'multiselect';
    var need = many ? (q.max || q.min || 1) : 1;
    var opts = (q.options || []).map(function (o, i) {
      var on = LVL.picked.indexOf(o.id) >= 0;
      return '<button type="button" class="lvl-opt' + (on ? ' on' : '') +
        (o.disabled ? ' off' : '') + '" data-opt="' + esc(o.id) + '"' +
        (o.disabled ? ' disabled' : '') + '>' +
        '<span class="nm">' + esc(o.name) + '</span>' +
        (o.recommended ? '<span class="tip">recommended</span>' : '') +
        (o.summary ? '<span class="sum">' + esc(o.summary) + '</span>' : '') +
        (o.disabled && o.reason ? '<span class="why">' + esc(o.reason) + '</span>' : '') +
        '</button>';
    }).join('');

    lvlBody.innerHTML = head +
      '<div class="lvl-title">' + esc(q.title || 'Choose') + '</div>' +
      '<p class="lvl-q">' + esc(q.question || '') + '</p>' +
      (q.help ? '<div class="lvl-help">' + esc(q.help) + '</div>' : '') +
      '<div class="lvl-opts">' + opts + '</div>' +
      (q.allowCustom
        ? '<div class="lvl-custom"><label for="lvl-custom">Or type it</label>' +
          '<input type="text" class="lvl-field" id="lvl-custom" autocomplete="off" ' +
          'value="' + esc(LVL.custom) + '"></div>'
        : '') +
      (many ? '<div class="lvl-count">Choose ' + need + ' &middot; ' +
              LVL.picked.length + ' picked</div>' : '') +
      (LVL.error ? '<div class="lvl-err">' + esc(LVL.error) + '</div>' : '');

    var ready = LVL.custom.trim() !== '' ||
                (many ? LVL.picked.length === need : LVL.picked.length === 1);
    lvlFoot.innerHTML =
      '<button type="button" class="dt-btn" id="lvl-back"' +
        (LVL.order.length ? '' : ' disabled') + '>&larr; Back</button>' +
      (q.allowRandom ? '<button type="button" class="dt-btn" id="lvl-random">' +
        'Surprise me</button>' : '') +
      '<span class="lvl-spacer"></span>' +
      '<button type="button" class="dc-roll" id="lvl-go"' +
        (ready && !LVL.busy ? '' : ' disabled') + '>' +
        (LVL.busy ? 'Thinking…' : 'Choose') + '</button>';
  }

  function lvlRenderDone(head, p) {
    var gained = (p.gained || []).map(function (g) {
      return '<li>' + esc(g) + '</li>';
    }).join('');
    // Choices from levels the character already had that the sheet never
    // wrote down. They are not what this level earned, so they are not
    // asked about - but they are said out loud rather than lost.
    var lost = (p.unrecorded || []).length
      ? '<div class="lvl-note"><b>Not written down before now:</b> ' +
        esc(p.unrecorded.join('; ')) + '</div>'
      : '';
    lvlBody.innerHTML = head +
      '<div class="lvl-done-head">' + esc(p.className || 'Level') + ' ' +
        p.toLevel + '</div>' +
      '<div class="lvl-help">Nothing has been written yet. This is what the ' +
        'levels come to.</div>' +
      (gained ? '<ul class="lvl-gained">' + gained + '</ul>'
              : '<div class="dt-empty">Nothing to report.</div>') +
      lost +
      (LVL.error ? '<div class="lvl-err">' + esc(LVL.error) + '</div>' : '');
    lvlFoot.innerHTML =
      '<button type="button" class="dt-btn" id="lvl-back"' +
        (LVL.order.length ? '' : ' disabled') + '>&larr; Back</button>' +
      '<span class="lvl-spacer"></span>' +
      '<button type="button" class="dc-roll" id="lvl-commit"' +
        (LVL.busy ? ' disabled' : '') + '>' +
        (LVL.busy ? 'Writing…' : 'Write it to the sheet') + '</button>';
  }

  function lvlRenderWritten(p) {
    lvlBody.innerHTML =
      '<div class="lvl-done-head">Written</div>' +
      '<div class="lvl-help">' + esc(p.character || 'The character') +
        ' is now ' + esc(p.className || '') + ' ' + p.toLevel +
        '. The sheet on screen is the one from before, so it wants reading ' +
        'again.</div>' +
      '<ul class="lvl-gained">' +
        (p.gained || []).map(function (g) { return '<li>' + esc(g) + '</li>'; }).join('') +
      '</ul>';
    lvlFoot.innerHTML =
      '<span class="lvl-spacer"></span>' +
      '<button type="button" class="dc-roll" id="lvl-reload">Read it again</button>';
  }

  // ---- answering ---------------------------------------------------------
  function lvlClick(e) {
    var opt = e.target.closest('.lvl-opt');
    if (opt && !opt.disabled) {
      var id = opt.getAttribute('data-opt');
      var q = LVL.plan && LVL.plan.next;
      var many = q && q.kind === 'multiselect';
      var at = LVL.picked.indexOf(id);
      if (at >= 0) { LVL.picked.splice(at, 1); }
      else if (many) {
        var need = (q.max || q.min || 1);
        LVL.picked.push(id);
        // Picking one too many drops the oldest, which is kinder than
        // refusing the click.
        while (LVL.picked.length > need) { LVL.picked.shift(); }
      } else {
        LVL.picked = [id];
      }
      LVL.error = '';
      lvlRender();
      return;
    }
    if (e.target.closest('#lvl-go')) { lvlAnswer(); return; }
    if (e.target.closest('#lvl-back')) { lvlBack(); return; }
    if (e.target.closest('#lvl-random')) { lvlRandom(); return; }
    if (e.target.closest('#lvl-commit')) { lvlAsk(true); return; }
    if (e.target.closest('#lvl-reload')) { location.reload(); return; }
    var box = lvlTray.querySelector('#lvl-custom');
    if (box) { LVL.custom = box.value; }
  }

  function lvlAnswer() {
    var q = LVL.plan && LVL.plan.next;
    if (!q) { return; }
    var box = lvlTray.querySelector('#lvl-custom');
    var typed = box ? box.value.trim() : '';
    var values = typed ? [typed] : LVL.picked.slice();
    if (!values.length) { return; }
    LVL.answers[q.step] = values;
    LVL.order.push(q.step);
    lvlAsk(false);
  }

  function lvlRandom() {
    var q = LVL.plan && LVL.plan.next;
    if (!q || !q.options) { return; }
    var open = q.options.filter(function (o) { return !o.disabled; });
    if (!open.length) { return; }
    var need = q.kind === 'multiselect' ? (q.max || q.min || 1) : 1;
    var pool = open.slice(), out = [];
    while (out.length < need && pool.length) {
      out.push(pool.splice(Math.floor(Math.random() * pool.length), 1)[0].id);
    }
    LVL.picked = out;
    lvlRender();
  }

  // Back is free because the flow is stateless: drop the last answer and
  // ask again, and the engine rebuilds everything from what is left.
  function lvlBack() {
    if (!LVL.order.length) { return; }
    delete LVL.answers[LVL.order.pop()];
    LVL.error = '';
    lvlAsk(false);
  }

  // -------------------------------------------------------- command palette
  //
  // One box that does everything on the sheet. Press "/", type "stealth" or
  // "fire bolt" or "potion", press Enter. In a fight the sheet is three tabs
  // and a scroll away from whatever you actually want, and hunting for it is
  // the slowest thing about using it.
  //
  // The index is built from the page's own controls rather than from a list
  // kept beside them: every rollable, every cast button, every usable line of
  // the inventory and every feature with uses left is already in the dom with
  // a label on it. So the palette cannot go stale, and anything the sheet
  // grows later turns up in it without being told to.
  //
  // The matcher is FuzzyScore from fuzzy.go said again in javascript. It has
  // to be said again rather than asked for - a round trip per keystroke is
  // exactly the delay this is meant to remove - but it is the same rules, so
  // the letters that find a spell here find it in the terminal chooser too:
  // every term must match, letters may be skipped, and a term that only
  // matches the summary after the name always sorts below one that matched
  // the name itself.
  var PAL = { open: false, hits: [], at: 0, q: '' };
  var palBox, palInput, palList, palCount;

  function palBoundary(c) {
    return c === ' ' || c === '-' || c === '_' || c === '/' || c === '(' ||
           c === ')' || c === ',' || c === '.' || c === "'" || c === ':';
  }

  // fuzzyScore matches pattern against text as a subsequence and scores how
  // good the match is: adjacent letters, letters that start a word and a
  // match right at the front all count for more, a big gap counts for less,
  // and a short option that matched beats a long one that merely contains
  // the same letters. It returns -1 for no match.
  function fuzzyScore(pattern, text) {
    if (!pattern) { return 0; }
    var pat = pattern.toLowerCase(), txt = text.toLowerCase();
    var score = 0, pi = 0, last = -1;
    for (var ti = 0; ti < txt.length && pi < pat.length; ti++) {
      if (txt.charAt(ti) !== pat.charAt(pi)) { continue; }
      score += 10;
      if (ti === 0) { score += 20; }
      else if (last === ti - 1) { score += 12; }
      else if (palBoundary(txt.charAt(ti - 1))) { score += 10; }
      if (last >= 0 && ti - last > 1) {
        score -= Math.min(6, ti - last - 1);
      }
      last = ti;
      pi++;
    }
    if (pi < pat.length) { return -1; }
    return score - Math.floor(txt.length / 12);
  }

  // fuzzyAll requires every term to match somewhere. A term matches the name
  // loosely, but the hint after it only counts on a whole word: fuzzy
  // matching over a sentence matches almost anything, which would leave a
  // search that reads like it narrowed the list showing half of it.
  function fuzzyAll(terms, name, hint) {
    var rest = (hint || '').toLowerCase();
    var total = 0;
    for (var i = 0; i < terms.length; i++) {
      var n = fuzzyScore(terms[i], name);
      if (n >= 0) { total += n * 2; continue; }
      var at = rest.indexOf(terms[i]);
      if (at >= 0) { total += 20 - Math.floor(at / 8); continue; }
      return -1;
    }
    return total;
  }

  // The mark in front of each row, which says what pressing Enter will do.
  var PAL_KINDS = {
    attack: 'Attack', save: 'Save', check: 'Check', roll: 'Roll',
    cast: 'Cast', use: 'Use', feature: 'Spend', 'do': 'Do'
  };

  // Which kind wins when two rows score the same, which they do whenever two
  // things share a name. A spell is both a cast button and - because the
  // sheet makes every dice expression in its description rollable - a bare
  // damage roll called the same thing; typing the spell's name means casting
  // it, not rolling the dice out of the middle of its own text.
  var PAL_RANK = {
    'do': 0, attack: 1, cast: 2, check: 3, save: 3, use: 4, feature: 5, roll: 6
  };

  // ---- what is on the sheet ---------------------------------------------
  function palIndex() {
    var out = [];
    var add = function (kind, name, hint, run, el) {
      if (!name) { return; }
      out.push({ kind: kind, name: name, hint: hint || '', run: run, el: el || null });
    };

    // The things that are always there, first, so an empty box reads as a
    // menu of what the sheet can do rather than as an empty list.
    add('do', 'Short rest', 'spend hit dice, recover what an hour gives back',
        function () { openRest('short'); });
    add('do', 'Long rest', 'hit points, hit dice, slots and every feature',
        function () { openRest('long'); });
    add('do', 'Undo', 'take back the last thing that happened',
        function () { undoLast(); });
    add('do', 'Hurt', 'take damage', function () { palFocusHp('hurt'); });
    add('do', 'Heal', 'regain hit points', function () { palFocusHp('heal'); });
    add('do', 'Temporary hit points', 'set them', function () { palFocusHp('temp'); });
    if (HEALTH.state && HEALTH.state.hp && HEALTH.state.hp.temp) {
      add('do', 'Clear temporary hit points', 'give them up',
          function () { healthChange('cleartemp'); });
    }
    add('do', 'Inspiration', (INSP && INSP.held) ? 'spend it' : 'the DM has given you one',
        function () { inspirationToggle(); });
    add('do', 'Conditions', 'what is wrong with you', function () { openConditions(); });
    add('do', 'Session notes', 'the night\'s log', function () { setSessPanel(true); });
    add('do', 'Custom roll', 'any dice you like', function () { setPanel(true); });
    add('do', 'Roll tray', 'what has been rolled', function () { setTray(true); });
    add('do', 'Level up', 'gain a level, one question at a time',
        function () { setLevelTray(true); });
    add('do', 'Print sheet', 'on paper', function () { window.print(); });

    // Every rollable on the page. One pass covers the skills, the ability
    // checks, the saving throws, the attacks, initiative and the hit die,
    // because all of them are the same kind of thing already.
    var seen = {};
    Array.prototype.forEach.call(document.querySelectorAll('.rollable[data-label]'),
      function (el) {
        var label = el.getAttribute('data-label');
        var as = el.getAttribute('data-roll-as') ||
                 (el.getAttribute('data-kind') === 'damage' ? 'roll' : 'check');
        var key = as + '|' + label;
        // The attacks table puts the same roll on two cells, and the palette
        // should offer it once.
        if (!label || seen[key]) { return; }
        seen[key] = true;
        var hint = el.getAttribute('data-mod') || el.getAttribute('data-roll') ||
                   el.getAttribute('data-pool') || '';
        add(PAL_KINDS[as] ? as : 'roll', label, hint,
            function () { activate(el, null); }, el);
      });

    // Spells that can be cast right now, which is what the spell panel has
    // already worked out and drawn a button for.
    Array.prototype.forEach.call(document.querySelectorAll('.cast-btn[data-spell]'),
      function (el) {
        var detail = (el.getAttribute('data-detail') || '').split('·')
          .slice(0, 2).join('·').trim();
        add('cast', el.getAttribute('data-spell'), detail,
            function () { el.click(); }, el);
      });

    // Anything in the bag that is spent by being used, with what using one
    // does where the rules say plainly.
    Array.prototype.forEach.call(document.querySelectorAll('tr[data-name]'),
      function (row) {
        var btn = row.querySelector('.ib.use[data-act="use"]');
        if (!btn) { return; }
        var qty = row.querySelector('.qty');
        var does = btn.querySelector('.ib-does');
        var hint = (qty ? qty.textContent.trim() + ' carried' : '');
        if (does) { hint += (hint ? ' · ' : '') + does.textContent.trim(); }
        add('use', row.getAttribute('data-name'), hint,
            function () { btn.click(); }, btn);
      });

    // Features with uses left. A feature that is spent out is still listed,
    // saying so, because "have I got one left" is half the question.
    Array.prototype.forEach.call(document.querySelectorAll('.feature[data-uses]'),
      function (host) {
        var box = host.querySelector('.uses');
        if (!box) { return; }
        var pips = Array.prototype.slice.call(box.querySelectorAll('.use-pip'));
        var spent = parseInt(box.getAttribute('data-spent'), 10) || 0;
        var left = pips.length - spent;
        add('feature', host.getAttribute('data-uses-name'),
            left > 0 ? left + ' of ' + pips.length + ' left' : 'none left',
            left > 0 ? function () { pips[spent].click(); } : null, host);
      });

    return out;
  }

  // palFocusHp is what the three hit point commands do. None of them can act
  // on their own - they all need a number - so rather than guessing one they
  // put the cursor in the box beside the button and let it be typed.
  function palFocusHp(which) {
    var amt = document.getElementById('hp-amount');
    if (!amt) { return; }
    amt.scrollIntoView({ block: 'center', behavior: 'smooth' });
    amt.focus();
    amt.select();
    // Which button the number is for, so Enter in the box presses the one
    // that was asked for rather than the one Enter normally means.
    amt.setAttribute('data-pending', which);
    var ctl = document.getElementById('hp-ctl');
    if (ctl) {
      ctl.classList.remove('asking');
      void ctl.offsetWidth;
      ctl.classList.add('asking');
      setTimeout(function () { ctl.classList.remove('asking'); }, 1400);
    }
  }

  // ---- drawing it --------------------------------------------------------
  function palRender() {
    var terms = PAL.q.toLowerCase().split(/\s+/).filter(Boolean);
    var all = palIndex();
    var hits;
    if (!terms.length) {
      hits = all.slice(0, 40);
    } else {
      var scored = [];
      all.forEach(function (c, i) {
        var n = fuzzyAll(terms, c.name, c.hint);
        if (n >= 0) { scored.push({ c: c, n: n, i: i }); }
      });
      // Score first, then what kind of thing it is, then the order the sheet
      // itself lists things - so nothing shuffles about as letters are typed.
      scored.sort(function (a, b) {
        return b.n - a.n ||
               (PAL_RANK[a.c.kind] || 9) - (PAL_RANK[b.c.kind] || 9) ||
               a.i - b.i;
      });
      hits = scored.slice(0, 40).map(function (h) { return h.c; });
    }
    PAL.hits = hits;
    if (PAL.at >= hits.length) { PAL.at = Math.max(0, hits.length - 1); }

    if (!hits.length) {
      palList.innerHTML = '<div class="cmdk-empty">Nothing on the sheet matches that.</div>';
      palCount.textContent = '';
      return;
    }
    palList.innerHTML = hits.map(function (c, i) {
      // A roll a condition has an opinion about wears the same mark here as
      // it does under the mouse, so the palette does not quietly hand you a
      // roll the sheet would have warned you about.
      var a = c.el && c.el.classList && c.el.classList.contains('rollable')
        ? adviceFor(c.el) : null;
      var mark = a ? '<span class="cmdk-omen ' + omenKind(a) + '">' +
        OMEN_MARK[omenKind(a)] + '</span>' : '';
      return '<button type="button" class="cmdk-row' + (i === PAL.at ? ' on' : '') +
        (c.run ? '' : ' dead') + '" data-at="' + i + '">' +
        '<span class="cmdk-kind ' + c.kind + '">' + PAL_KINDS[c.kind] + '</span>' +
        '<span class="cmdk-name">' + esc(c.name) + '</span>' + mark +
        (c.hint ? '<span class="cmdk-hint">' + esc(c.hint) + '</span>' : '') +
        '</button>';
    }).join('');
    palCount.textContent = hits.length >= 40 ? 'first 40' : hits.length + '';
    var on = palList.querySelector('.cmdk-row.on');
    if (on && on.scrollIntoView) { on.scrollIntoView({ block: 'nearest' }); }
  }

  function palMove(by) {
    if (!PAL.hits.length) { return; }
    PAL.at = (PAL.at + by + PAL.hits.length) % PAL.hits.length;
    palRender();
  }

  function palRun() {
    var c = PAL.hits[PAL.at];
    if (!c || !c.run) { return; }
    setPalette(false);
    // After the box has gone, so the roll lands on a sheet you can see and
    // the dice are not thrown behind a modal.
    c.run();
  }

  function setPalette(open) {
    PAL.open = open;
    palBox.classList.toggle('open', open);
    if (open) {
      PAL.q = '';
      PAL.at = 0;
      palInput.value = '';
      palRender();
      palInput.focus();
    } else if (palInput) {
      palInput.blur();
    }
  }

  function buildPalette() {
    palBox = document.createElement('div');
    palBox.className = 'cmdk';
    palBox.id = 'cmdk';
    palBox.innerHTML =
      '<div class="cmdk-card" role="dialog" aria-label="Command palette">' +
        '<div class="cmdk-top">' +
          '<span class="cmdk-slash">/</span>' +
          '<input type="text" id="cmdk-in" class="cmdk-in" autocomplete="off" ' +
            'spellcheck="false" placeholder="stealth, fire bolt, potion, long rest…" ' +
            'aria-label="Search the sheet">' +
          '<span class="cmdk-count" id="cmdk-count"></span>' +
        '</div>' +
        '<div class="cmdk-list" id="cmdk-list"></div>' +
        '<div class="cmdk-tip">&uarr;&darr; to choose &middot; Enter to do it &middot; Esc to close</div>' +
      '</div>';
    document.body.appendChild(palBox);
    palInput = palBox.querySelector('#cmdk-in');
    palList = palBox.querySelector('#cmdk-list');
    palCount = palBox.querySelector('#cmdk-count');

    palInput.addEventListener('input', function () {
      PAL.q = palInput.value;
      PAL.at = 0;
      palRender();
    });
    palInput.addEventListener('keydown', function (e) {
      if (e.key === 'ArrowDown') { e.preventDefault(); palMove(1); }
      else if (e.key === 'ArrowUp') { e.preventDefault(); palMove(-1); }
      else if (e.key === 'Enter') { e.preventDefault(); palRun(); }
      else if (e.key === 'Escape') { e.preventDefault(); setPalette(false); }
    });
    palList.addEventListener('click', function (e) {
      var row = e.target.closest('.cmdk-row');
      if (!row) { return; }
      PAL.at = parseInt(row.getAttribute('data-at'), 10) || 0;
      palRun();
    });
    palList.addEventListener('mousemove', function (e) {
      var row = e.target.closest('.cmdk-row');
      if (!row) { return; }
      var at = parseInt(row.getAttribute('data-at'), 10) || 0;
      if (at === PAL.at) { return; }
      PAL.at = at;
      // Only the highlight moves, so this does not rebuild the list under
      // the pointer every time the mouse twitches.
      Array.prototype.forEach.call(palList.querySelectorAll('.cmdk-row'),
        function (r, i) { r.classList.toggle('on', i === at); });
    });
    palBox.addEventListener('mousedown', function (e) {
      if (e.target === palBox) { setPalette(false); }
    });

    document.addEventListener('keydown', function (e) {
      if (PAL.open) { return; }
      if (e.altKey || e.ctrlKey && e.key !== 'k' || e.metaKey && e.key !== 'k') { return; }
      // "/" opens it, and ctrl or cmd with k does too, because that is what
      // every other palette in the world answers to.
      var slash = e.key === '/' && !e.ctrlKey && !e.metaKey;
      var kay = (e.ctrlKey || e.metaKey) && (e.key === 'k' || e.key === 'K');
      if (!slash && !kay) { return; }
      // Not while something else is being typed into, and not over a modal
      // that is already asking a question.
      var t = e.target;
      if (t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' ||
                t.tagName === 'SELECT' || t.isContentEditable)) { return; }
      if (document.querySelector('.inv-modal.open, .sb-modal.open, .rest-modal.open')) {
        return;
      }
      e.preventDefault();
      setPalette(true);
    });
  }

  function buildMood() {
    halo = document.getElementById('page-halo');
    paintMood();
  }

  function init() {
    // Whether the dice are heard is remembered between sessions, and the tray
    // draws the box from it, so it has to be read before the tray is built.
    soundLoad();
    var canvas = document.createElement('canvas');
    canvas.id = 'dice-canvas';
    document.body.appendChild(canvas);
    board = new DiceBoard(canvas);
    var coinCanvas = document.createElement('canvas');
    coinCanvas.id = 'coin-canvas';
    document.body.appendChild(coinCanvas);
    coinBoard = new CoinBoard(coinCanvas);
    var spellCanvas = document.createElement('canvas');
    spellCanvas.id = 'spell-canvas';
    document.body.appendChild(spellCanvas);
    spellBoard = new SpellBoard(spellCanvas);
    buildTray();
    buildCustomDock();
    watchPrinting();
    buildSessionLog();
    buildInventoryUi();
    buildCoinUi();
    buildSpellUi();
    buildRestUi();
    buildDefenseUi();
    buildOmen();
    buildLevelUi();
    buildPalette();
    buildHealthUi();
    buildConcentrationUi();
    buildInspirationUi();
    buildSectionTabs();
    // The mood reads the hit points and the conditions, so it goes last: both
    // panels have seeded themselves from the export by now.
    buildMood();
    fitRingText();

    Array.prototype.forEach.call(document.querySelectorAll('[data-kind]'), prepare);
    Array.prototype.forEach.call(
      document.querySelectorAll('.feature p, details.spell p, .box .tagline, .quote'),
      linkifyProse);

    document.addEventListener('click', function (e) {
      if (e.target.closest('#dice-tray')) { historyClick(e); return; }
      var second = e.target.closest('.cast2-btn');
      if (second) {
        // Inside a <summary>, so the same guard the cast button needs.
        e.preventDefault();
        e.stopPropagation();
        var spec2 = cast2SpecFor(second);
        if (spec2) { roll(spec2, originOf(second, e)); }
        return;
      }
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
