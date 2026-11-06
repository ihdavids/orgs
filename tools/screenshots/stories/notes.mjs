// The pictures for the first set of more optional packages (docs/note-packages.org):
// canvases, trackers, related notes, Excalidraw drawings, media notes,
// callouts and backlink counts. The demo notebook with a kitchen garden on
// top (docs/screenshots/notes): garden.org (prose for related notes, links
// for the counts, callouts on the tomatoes), talks.org (a talk and a podcast
// with timestamp notes) and tracking.org (three #+TRACKER: lines). What is
// not an org file is made here: a year of day pages with MOOD, SLEEP and
// WALK, the garden board (.canvas), two drawings (.excalidraw), and the
// recordings (made with ffmpeg from talk-slide.png and a little pink noise,
// kept in the temp dir between runs).
//
//   SHOT_PORT=8142 node capture.mjs -story notes
//   SHOT_PORT=8142 node capture.mjs -story notes worg-canvas

import { execFileSync } from 'node:child_process';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { at, tag, clipAround, mainClip, toTop, clickShown } from './engineer-worg.mjs';

const here = path.dirname(fileURLToPath(import.meta.url));
const shots = path.resolve(here, '../../../docs/screenshots');
const notebook = path.join(shots, 'notes');
const sleep = ms => new Promise(r => setTimeout(r, ms));

// ------------------------------------------------------------ day pages

// A year of days, most of them with a mood, a night's sleep and a walk. The
// numbers come from a seeded generator, so every run draws the same year:
// better moods at weekends and in summer, shorter nights mid-week, a long
// walk most Sundays, a week away in August with no pages at all.
function rng(seed) {
  let s = seed >>> 0;
  return () => { s = (s * 1664525 + 1013904223) >>> 0; return s / 4294967296; };
}
const DAYS = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];
const pad = n => String(n).padStart(2, '0');
function dayPages() {
  const r = rng(20261105);
  const out = {};
  const today = new Date(); today.setHours(12, 0, 0, 0);
  for (let back = 364; back >= 0; back--) {
    const d = new Date(today); d.setDate(d.getDate() - back);
    const m = d.getMonth(), wd = d.getDay();
    if (m === 7 && d.getDate() >= 10 && d.getDate() <= 17) continue; // away
    if (back > 0 && r() < 0.1) continue;                              // forgot
    const summer = Math.cos((d.getMonth() + d.getDate() / 31 - 6.5) / 12 * 2 * Math.PI); // 1 in July
    const weekend = wd === 0 || wd === 6;
    const mood = Math.max(1, Math.min(5, Math.round(2.9 + summer * 0.9 + (weekend ? 0.6 : 0) + (r() - 0.5) * 2.8)));
    let sleepMin = 445 + (weekend ? 35 : 0) - (wd === 3 || wd === 4 ? 20 : 0) + Math.round((r() - 0.5) * 70);
    sleepMin = Math.max(330, Math.min(540, Math.round(sleepMin / 5) * 5));
    let walk = Math.round((15 + summer * 12 + (wd === 0 ? 45 : 0) + r() * 35) / 5) * 5;
    if (r() < 0.12) walk = 0;
    const name = `${DAYS[wd]}_${d.getFullYear()}_${pad(d.getMonth() + 1)}_${pad(d.getDate())}`;
    const lines = [':PROPERTIES:', `:MOOD: ${mood}`, `:SLEEP: ${Math.floor(sleepMin / 60)}:${pad(sleepMin % 60)}`];
    if (walk) lines.push(`:WALK: ${walk}m`);
    lines.push(':END:', `#+TITLE: ${name}`, '');
    out[`org/worklog/${name}.org`] = lines.join('\n');
  }
  return out;
}

// ------------------------------------------------------------ the board

// Heading cards name garden.org's headings by their :ID:, as a board made in
// worg does. Two groups, the work before winter and in spring, joined by
// labelled edges; a loose idea in a card of its own.
const G = 'garden.org';
const heading = (id, sub, x, y, w = 320, h = 190) => ({ id, type: 'file', file: G, subpath: `#^${sub}`, x, y, width: w, height: h });
const text = (id, t, x, y, w = 320, h = 110, color) => ({ id, type: 'text', text: t, x, y, width: w, height: h, ...(color ? { color } : {}) });
const BOARD = {
  nodes: [
    { id: 'a1f0c3d2e4b5a601', type: 'group', label: 'Before winter', x: -30, y: -70, width: 820, height: 430, color: '3' },
    { id: 'a1f0c3d2e4b5a602', type: 'group', label: 'Spring', x: 900, y: -70, width: 820, height: 430, color: '4' },
    heading('b2e1d4c3f5a6b701', '8d4c2a71-0e9b-4f3a-b6d5-2a7e9c1f4b02', 0, -20),
    heading('b2e1d4c3f5a6b702', '3b1f6f0e-5c1a-4b0e-9a63-1d2f7c9e8a01', 440, -20),
    text('c3f2e5d4a6b7c801', 'Order *2 m³* of green-waste compost from the council tip before February.', 0, 220),
    text('c3f2e5d4a6b7c802', 'Rake the oak leaves into the cage: next year\'s leaf mould.', 440, 220),
    heading('b2e1d4c3f5a6b703', 'c27e5b39-61f0-4d8a-9e4b-7f3a0d2c6e03', 930, -20),
    heading('b2e1d4c3f5a6b704', 'e5a9d1c4-2f7b-4e63-8a0d-9c4b1e7f2a04', 1370, -20),
    text('c3f2e5d4a6b7c803', 'Raise the butts on blocks: *50 cm* of head for the drip line.', 930, 220),
    text('c3f2e5d4a6b7c804', 'Fleece by the back door until the middle of May.', 1370, 220),
    text('c3f2e5d4a6b7c805', 'Ask Sam about the spare IBC tank - another *1000 litres*?', 930, 440, 320, 100, '5'),
  ],
  edges: [
    { id: 'd4a3f6e5b7c8d901', fromNode: 'c3f2e5d4a6b7c801', fromSide: 'top', toNode: 'b2e1d4c3f5a6b701', toSide: 'bottom', label: 'tops up' },
    { id: 'd4a3f6e5b7c8d902', fromNode: 'b2e1d4c3f5a6b701', fromSide: 'right', toNode: 'b2e1d4c3f5a6b702', toSide: 'left', label: 'mulch, 5 cm' },
    { id: 'd4a3f6e5b7c8d903', fromNode: 'b2e1d4c3f5a6b702', fromSide: 'top', toNode: 'b2e1d4c3f5a6b704', toSide: 'top', label: 'plant into', color: '4' },
    { id: 'd4a3f6e5b7c8d904', fromNode: 'b2e1d4c3f5a6b703', fromSide: 'right', toNode: 'b2e1d4c3f5a6b704', toSide: 'left', label: 'drip line' },
    { id: 'd4a3f6e5b7c8d905', fromNode: 'c3f2e5d4a6b7c803', fromSide: 'top', toNode: 'b2e1d4c3f5a6b703', toSide: 'bottom', label: 'first' },
    { id: 'd4a3f6e5b7c8d906', fromNode: 'c3f2e5d4a6b7c805', fromSide: 'top', toNode: 'c3f2e5d4a6b7c803', toSide: 'bottom', color: '5' },
  ],
};

// ------------------------------------------------------------ drawings

// Excalidraw scenes written element by element: boxes, a diamond, ellipses,
// arrows and hand-lettered labels, in Excalidraw's own pastel fills.
let seq = 0;
function el(type, x, y, w, h, more = {}) {
  seq++;
  return {
    id: `el${seq.toString(36).padStart(6, '0')}`, type, x, y, width: w, height: h, angle: 0,
    strokeColor: '#1e1e1e', backgroundColor: 'transparent', fillStyle: 'hachure', strokeWidth: 2,
    strokeStyle: 'solid', roughness: 1, opacity: 100, groupIds: [], frameId: null,
    roundness: type === 'rectangle' ? { type: 3 } : type === 'arrow' || type === 'line' ? { type: 2 } : null,
    seed: 1000 + seq * 7919, version: 3, versionNonce: 2000 + seq * 104729, isDeleted: false,
    boundElements: [], updated: 1762300000000, link: null, locked: false, ...more,
  };
}
function label(t, x, y, size = 20, more = {}) {
  const lines = t.split('\n');
  const w = Math.max(...lines.map(l => l.length)) * size * 0.52, h = lines.length * size * 1.25;
  return el('text', x - w / 2, y - h / 2, w, h, {
    text: t, originalText: t, fontSize: size, fontFamily: 1, textAlign: 'center', verticalAlign: 'middle',
    containerId: null, autoResize: true, lineHeight: 1.25, baseline: Math.round(size * 0.9), roundness: null,
    fillStyle: 'solid', ...more,
  });
}
const box = (t, x, y, w, h, bg, more = {}) => [el('rectangle', x, y, w, h, { backgroundColor: bg, ...more }), label(t, x + w / 2, y + h / 2)];
const oval = (t, x, y, w, h, bg) => [el('ellipse', x, y, w, h, { backgroundColor: bg }), label(t, x + w / 2, y + h / 2)];
const diamond = (t, x, y, w, h, bg) => [el('diamond', x, y, w, h, { backgroundColor: bg }), label(t, x + w / 2, y + h / 2, 18)];
const arrow = (x, y, pts, more = {}) => {
  const xs = pts.map(p => p[0]), ys = pts.map(p => p[1]);
  return el('arrow', x, y, Math.max(...xs) - Math.min(...xs), Math.max(...ys) - Math.min(...ys), {
    points: pts, lastCommittedPoint: null, startBinding: null, endBinding: null,
    startArrowhead: null, endArrowhead: 'arrow', fillStyle: 'solid', ...more,
  });
};
const scene = elements => JSON.stringify({
  type: 'excalidraw', version: 2, source: 'https://excalidraw.com', elements,
  appState: { gridSize: null, viewBackgroundColor: '#ffffff' }, files: {},
}, null, 2);

function rainwater() {
  seq = 0;
  return scene([
    el('line', 40, 70, 300, 50, { points: [[0, 50], [150, 0], [300, 50]], strokeWidth: 3, roundness: null, lastCommittedPoint: null, startBinding: null, endBinding: null, startArrowhead: null, endArrowhead: null }),
    ...box('Shed', 60, 120, 260, 110, '#ffec99'),
    arrow(330, 130, [[0, 0], [40, 0], [40, 110]], { strokeColor: '#1971c2' }),
    label('downpipe', 430, 175, 16, { strokeColor: '#1971c2' }),
    ...box('Butt 1\n200 l', 330, 260, 110, 130, '#a5d8ff'),
    ...box('Butt 2\n200 l', 470, 260, 110, 130, '#a5d8ff'),
    el('line', 440, 370, 30, 0, { points: [[0, 0], [30, 0]], strokeColor: '#1971c2', roundness: null, lastCommittedPoint: null, startBinding: null, endBinding: null, startArrowhead: null, endArrowhead: null }),
    el('rectangle', 320, 390, 270, 22, { backgroundColor: '#ced4da', fillStyle: 'cross-hatch' }),
    label('on blocks: 50 cm of head', 455, 440, 16, { strokeColor: '#e03131' }),
    arrow(590, 320, [[0, 0], [90, 0]], { strokeColor: '#1971c2' }),
    ...diamond('filter', 690, 280, 120, 80, '#ffd8a8'),
    arrow(810, 320, [[0, 0], [80, 0]], { strokeColor: '#1971c2' }),
    ...box('drip line\ntomato beds', 900, 270, 200, 100, '#b2f2bb'),
    arrow(330, 290, [[0, 0], [-120, 40], [-150, 190]], { strokeColor: '#1971c2', strokeStyle: 'dashed' }),
    label('overflow', 140, 400, 16, { strokeColor: '#1971c2' }),
    ...oval('pond', 100, 490, 200, 90, '#a5d8ff'),
  ]);
}

function beds() {
  seq = 100;
  return scene([
    ...box('Shed', 40, 40, 180, 120, '#ffec99'),
    ...box('1  tomatoes', 280, 40, 260, 110, '#ffc9c9'),
    ...box('2  beans', 580, 40, 260, 110, '#b2f2bb'),
    ...box('3  brassicas', 280, 190, 260, 110, '#d0bfff'),
    ...box('4  roots', 580, 190, 260, 110, '#ffd8a8'),
    ...box('compost', 40, 340, 100, 90, '#e9d5b7'),
    ...box('heating', 160, 340, 100, 90, '#e9d5b7'),
    ...box('ready', 280, 340, 100, 90, '#e9d5b7'),
    arrow(410, 385, [[0, 0], [160, 0]], { strokeStyle: 'dashed' }),
    label('5 cm every autumn', 580, 410, 16, { strokeColor: '#868e96' }),
    label('N', 880, 50, 28),
    arrow(880, 130, [[0, 0], [0, -50]]),
  ]);
}

function rotation() {
  seq = 200;
  const C = [['tomatoes', 380, 60, '#ffc9c9'], ['beans', 640, 260, '#b2f2bb'], ['brassicas', 380, 460, '#d0bfff'], ['roots', 120, 260, '#ffd8a8']];
  return scene([
    ...C.flatMap(([t, x, y, bg]) => oval(t, x, y, 200, 90, bg)),
    arrow(590, 110, [[0, 0], [110, 40], [140, 140]]),
    arrow(740, 360, [[0, 0], [-30, 100], [-150, 150]]),
    arrow(370, 510, [[0, 0], [-110, -40], [-140, -140]]),
    arrow(220, 250, [[0, 0], [30, -100], [150, -150]]),
    label('a bed moves on\none place a year', 480, 305, 18, { strokeColor: '#868e96' }),
  ]);
}

// ------------------------------------------------------------ recordings

// A 38-minute talk (the slide, still) and a 25-minute podcast, made once and
// kept in the temp dir: a few megabytes, and ffmpeg takes seconds.
function recordings() {
  const cache = path.join(os.tmpdir(), 'orgs-shots-media');
  fs.mkdirSync(cache, { recursive: true });
  const video = path.join(cache, 'soil-food-web.mp4'), audio = path.join(cache, 'dry-summer-ep42.m4a');
  const noise = 'anoisesrc=color=pink:amplitude=0.004:sample_rate=16000';
  // Written beside and renamed, so a run stopped halfway leaves no broken file.
  const ff = (out, args) => {
    if (fs.existsSync(out)) return;
    const part = out.replace(/(\.\w+)$/, '.part$1');
    execFileSync('ffmpeg', ['-y', '-hide_banner', '-loglevel', 'error', ...args, part], { stdio: ['ignore', 'ignore', 'inherit'] });
    // A file ffmpeg did not finish has no index (moov) and never plays.
    execFileSync('ffprobe', ['-v', 'error', '-show_entries', 'format=duration', part], { stdio: 'ignore' });
    fs.renameSync(part, out);
  };
  ff(video, ['-loop', '1', '-framerate', '1', '-i', path.join(notebook, 'talk-slide.png'),
    '-f', 'lavfi', '-i', noise, '-t', '2280', '-c:v', 'libx264', '-tune', 'stillimage', '-pix_fmt', 'yuv420p',
    '-r', '1', '-g', '300', '-c:a', 'aac', '-b:a', '12k', '-ac', '1', '-shortest', '-movflags', '+faststart']);
  ff(audio, ['-f', 'lavfi', '-i', noise, '-t', '1500', '-c:a', 'aac', '-b:a', '12k', '-ac', '1', '-movflags', '+faststart']);
  return { 'org/talks/soil-food-web.mp4': fs.readFileSync(video), 'org/talks/dry-summer-ep42.m4a': fs.readFileSync(audio) };
}

const files = { ...dayPages(), ...recordings(),
  'org/boards/garden-plan.canvas': JSON.stringify(BOARD, null, '\t'),
  'org/drawings/rainwater.excalidraw': rainwater(),
  'org/drawings/bed-layout.excalidraw': beds(),
  'org/drawings/crop-rotation.excalidraw': rotation(),
};

const server = `  dayPageMode: "day"
  features:
    canvas: true
    tracker: true
    related: true
    excalidraw: true
    mediaNotes: true
    callouts: true
    backlinkCounts: true
`;

export const demo = { src: [path.join(shots, 'demo'), notebook], files, server };

// ------------------------------------------------------------ helpers

// The demo lives in a temp folder; where worg prints its path say ~/notes.
async function scrub(page) {
  await page.evaluate(() => {
    const fix = () => {
      const walk = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
      for (let n = walk.nextNode(); n; n = walk.nextNode()) {
        if (n.nodeValue.includes('orgs-shots-')) n.nodeValue = n.nodeValue.replace(/\/\S*orgs-shots-[^/\s]+\/org/g, '~/notes');
      }
    };
    fix();
    if (!window.__scrubbing) {
      window.__scrubbing = new MutationObserver(fix);
      window.__scrubbing.observe(document.body, { subtree: true, childList: true, characterData: true });
    }
  });
}

async function openFile(page, ctx, file, view = 'Org') {
  await ctx.openApp(page, 'Files');
  await ctx.clickText(page, file, { settle: 2000 });
  await ctx.clickText(page, view, { within: 'main', settle: 1500 });
  await clickShown(page, '[aria-label="Hide the file tree"], [title="Hide the file tree"]');
  // worg remembers the side panels open; each shot opens the one it wants.
  await page.evaluate(() => {
    for (const l of ['Related notes', 'Backlinks and mentions']) {
      const b = [...document.querySelectorAll(`[aria-label="${l}"]`)].find(e => e.getBoundingClientRect().width > 0);
      if (b && b.className.includes('variantSolid')) b.click();
    }
  });
  await sleep(600);
}

// The rendered view in one of the exporter's light themes (worg's default is
// its dark one), so the pages match the rest of the light pictures. Journal
// for reading; default, wider, for the board, whose small print needs room.
async function pageTheme(page, theme = 'journal') {
  await page.evaluateOnNewDocument(t => {
    try {
      const s = JSON.parse(localStorage.getItem('worg.settings') || '{}');
      s.htmlTheme = t;
      localStorage.setItem('worg.settings', JSON.stringify(s));
    } catch { }
  }, theme);
}

// The rendered view is an iframe that scrolls on its own. Scrolls it so the
// element (a selector, or the innermost element with that text) sits dy
// below the frame's top.
async function frameScroll(page, find, dy = 24) {
  const ok = await page.evaluate((find, dy) => {
    const d = document.querySelector('main iframe')?.contentDocument;
    if (!d) return false;
    const el = find.selector ? d.querySelector(find.selector)
      : [...d.querySelectorAll('body *')].find(e => e.textContent.trim() === find.text && ![...e.children].some(c => c.textContent.trim() === find.text));
    if (!el) return false;
    const se = d.scrollingElement || d.documentElement;
    se.scrollTop += el.getBoundingClientRect().top - dy;
    return true;
  }, find, dy);
  if (!ok) throw new Error(`nothing in the page reads ${JSON.stringify(find)}`);
  await sleep(700);
}

// tag() for what is inside the rendered page's iframe: a transparent box
// over each element found, in the top document, carrying the data-shot name
// the notes point at.
async function tagFrame(page, specs) {
  const missing = await page.evaluate(specs => {
    const fr = document.querySelector('main iframe');
    const d = fr.contentDocument, o = fr.getBoundingClientRect();
    const out = [];
    for (const s of specs) {
      let els = s.selector ? [...d.querySelectorAll(s.selector)]
        : [...d.querySelectorAll('body *')].filter(e => {
          const has = x => s.contains ? x.textContent.trim().includes(s.text) : x.textContent.trim() === s.text;
          return has(e) && ![...e.children].some(has);
        });
      els = els.filter(e => { const r = e.getBoundingClientRect(); return r.width > 0 && r.bottom > 0 && r.top < o.height; });
      let el = s.last ? els[els.length - 1] : els[s.nth || 0];
      if (!el) { out.push(s.name); continue; }
      if (s.closest) el = el.closest(s.closest) || el;
      // range: the text's own box, not its (perhaps page-wide) element's.
      let r = el.getBoundingClientRect();
      if (s.range) { const g = d.createRange(); g.selectNodeContents(el); r = g.getBoundingClientRect(); }
      const top = Math.max(r.top, 0), bottom = Math.min(r.bottom, o.height);
      const box = document.createElement('div');
      box.setAttribute('data-shot', s.name);
      Object.assign(box.style, { position: 'fixed', left: `${o.left + r.left}px`, top: `${o.top + top}px`,
        width: `${r.width}px`, height: `${bottom - top}px`, pointerEvents: 'none' });
      document.body.appendChild(box);
    }
    return out;
  }, specs);
  if (missing.length) throw new Error(`could not tag ${missing.join(', ')} in the page`);
}

// A badge sits on its element's edge, half over it; on a small label that
// hides the words. This moves each name onto a box px wider all round, so
// the badge sits just clear of the text.
async function widen(page, names, px = 16) {
  await page.evaluate((names, px) => {
    for (const n of names) {
      const el = document.querySelector(`[data-shot="${n}"]`);
      if (!el) continue;
      const r = el.getBoundingClientRect();
      el.removeAttribute('data-shot');
      const box = document.createElement('div');
      box.setAttribute('data-shot', n);
      Object.assign(box.style, { position: 'fixed', left: `${r.left - px}px`, top: `${r.top - 3}px`,
        width: `${r.width + 2 * px}px`, height: `${r.height + 6}px`, pointerEvents: 'none' });
      document.body.appendChild(box);
    }
  }, names, px);
}

// The middle of the line holding text, in the source view or the editor.
const lineAt = (page, sel, text) => page.evaluate((sel, text) => {
  const l = [...document.querySelectorAll(sel)].find(e => e.textContent.includes(text));
  if (!l) return null;
  l.scrollIntoView({ block: 'center' });
  const b = l.getBoundingClientRect();
  return { x: b.left + 120, y: b.top + b.height / 2 };
}, sel, text);
async function clickLine(page, sel, text) {
  const p = await lineAt(page, sel, text);
  if (!p) throw new Error(`no line reads "${text}"`);
  await page.mouse.click(p.x, p.y);
  await sleep(500);
}

// Opens a drawing in Excalidraw over the page. worg saves the drawing's own
// svg and png beside it when it opens, which is why this comes last.
async function openDrawing(page, ctx, name) {
  await ctx.openApp(page, 'Drawings');
  await ctx.clickText(page, name, { within: 'main', settle: 6000 });
}


export default {
  demo,
  shots: [
    // ---------------------------------------------------------- canvas
    { name: 'cli-canvas',
      cli: { title: 'orgs canvas show garden-plan', args: ['canvas', 'show', 'garden-plan'], opts: { cols: 116, rows: 24 } },
      notes: [
        { find: 'boards/garden-plan.canvas', title: 'A board',
          text: 'A <code>.canvas</code> file beside the notes, in JSON Canvas, an open format other apps read too. <code>orgs canvas</code> lists them.' },
        { find: '[Before winter]', title: 'Groups',
          text: 'Each group with the cards inside it, and its colour.' },
        { find: '- Compost', title: 'A heading card',
          text: 'garden.org\'s heading, named by its <code>:ID:</code> (<code>"subpath": "#^ID"</code>), so the card follows it through a rename or a refile.' },
        { find: 'Edges:', title: 'Edges',
          text: 'Card to card, with their labels. <code>orgs canvas link plan Compost \'No-dig\' -label mulch</code> adds one.' },
      ] },
    {
      name: 'worg-canvas', title: 'worg · Canvas', page: { height: 780 },
      worg: async (page, ctx) => {
        await ctx.openApp(page, 'Canvas');
        await sleep(1500);
        await tag(page, [
          { name: 'pick', text: 'boards/garden-plan.canvas', within: 'main', closest: '.MuiSelect-root' },
          { name: 'add', selector: 'main input[placeholder="Add a heading…"]' },
          { name: 'group', text: 'Before winter', within: 'main' },
          { name: 'card', text: 'Rainwater butts', within: 'main', up: 1 },
          { name: 'text', text: 'Fleece by the back door until the middle of May.', contains: true, within: 'main', up: 1 },
          { name: 'edge', text: 'plant into', within: 'main' },
        ]);
        await widen(page, ['group', 'edge'], 14);
        return mainClip(page);
      },
      notes: [
        { at: at('pick'), badge: 'tl', title: 'The board',
          text: 'Any <code>.canvas</code> under the org directories, or a new one. Changes are saved a moment after you stop.' },
        { at: at('add'), badge: 'tr', title: 'Add a heading',
          text: 'Found by typing; the buttons beside it add a text card, a group, a link or a file.' },
        { at: at('group'), badge: 'r', title: 'A group',
          text: 'A labelled box that moves with the cards in it, in one of the six colours.' },
        { at: at('card'), badge: 'tr', title: 'A heading card',
          text: 'The heading as the file view shows it, links and all. Double click opens it.' },
        { at: at('text'), badge: 'br', title: 'A text card',
          text: 'A note typed on the board, in org (markdown is read too).' },
        { at: at('edge'), badge: 'r', title: 'An edge',
          text: 'Drag from a dot on a card\'s side to another card; select it to label it or change its arrows and colour.' },
      ],
    },
    {
      name: 'worg-canvas-export', title: 'worg · Files · garden.org', page: { height: 900 },
      worg: async (page, ctx) => {
        await pageTheme(page, 'default');
        await openFile(page, ctx, 'garden.org', 'Rendered');
        await sleep(2500);
        await scrub(page);
        await frameScroll(page, { text: 'The plan' }, 16);
        await tagFrame(page, [
          { name: 'board', selector: '.org-canvas svg' },
          { name: 'caption', selector: '.org-canvas-caption', range: true },
        ]);
        await widen(page, ['caption'], 16);
        const bottom = await page.evaluate(() => document.querySelector('[data-shot="caption"]').getBoundingClientRect().bottom);
        return mainClip(page, { bottom: bottom + 14 });
      },
      notes: [
        { at: at('board'), badge: 'tr', title: 'The board, in the page',
          text: 'A link to a <code>.canvas</code> - <code>[[file:boards/garden-plan.canvas][…]]</code> - is drawn as the board in html exports and worg\'s rendered view: groups, cards, arrows and labels.' },
        { at: at('caption'), badge: 'r', ring: false, title: 'The link\'s description',
          text: 'Under it, as a caption. The board\'s headings count as linked from it, in their backlinks and the link graph.' },
      ],
    },

    // ---------------------------------------------------------- tracker
    { name: 'cli-tracker',
      cli: { title: 'orgs track MOOD', args: ['track', 'MOOD'], opts: { cols: 86, rows: 14 } },
      notes: [
        { find: 'MOOD 20', title: 'A property, a day at a time',
          text: '<code>:MOOD:</code> in each day page\'s drawer (<code>Wed_2026_11_04.org</code>), over the last year. <code>orgs track log MOOD 4</code> writes today\'s.' },
        { find: 'Mon ', title: 'A heatmap calendar',
          text: 'A column a week, a row a day, darker for more. <code>-type line</code>, <code>bar</code> or <code>sparkline</code> draw it otherwise.' },
        { find: 'days · total', title: 'The summary',
          text: 'Days with a value, the mean, the least and most, the current streak and the longest.' },
      ] },
    {
      name: 'worg-tracker', title: 'worg · Trackers', page: { height: 1100 },
      worg: async (page, ctx) => {
        await ctx.openApp(page, 'Trackers');
        await sleep(1500);
        await tag(page, [
          { name: 'mood', text: 'Mood', within: 'main', closest: '.MuiCard-root, .MuiSheet-root' },
          { name: 'stats', text: 'longest', contains: true, within: 'main', nth: 0, up: 1 },
          { name: 'line', text: '#+TRACKER: source:property:MOOD', contains: true, within: 'main' },
          { name: 'log', selector: 'main input[placeholder="MOOD today"]' },
          { name: 'sleep', text: 'goal 7:00', contains: true, within: 'main' },
          { name: 'new', text: 'New tracker', within: 'main', closest: 'button' },
        ]);
        await widen(page, ['line', 'sleep'], 14);
        const [top, bottom] = await page.evaluate(() => [document.querySelector('[data-shot="new"]').getBoundingClientRect().top - 20,
          [...document.querySelectorAll('main *')].filter(e => e.textContent.includes('source:property:WALK')).pop().getBoundingClientRect().bottom + 36]);
        return mainClip(page, { top, bottom });
      },
      notes: [
        { at: at('new'), badge: 'bl', title: 'Every #+TRACKER: line',
          text: 'In any file, each as a card. New tracker builds a line, drawing the chart as you go.' },
        { at: at('stats'), badge: 'bl', ring: false, title: 'The summary',
          text: 'Days counted, the mean, the streak toward the goal and the longest.' },
        { at: at('mood'), badge: 'tl', ring: false, title: 'A year of day pages',
          text: 'The default heatmap. Hover a day for its value; click it to open that day\'s page.' },
        { at: at('line'), badge: 'l', ring: false, title: 'The line itself',
          text: '<code>#+TRACKER: source:property:MOOD title:"Mood" color:green</code> - org-plot style <code>key:value</code> words.' },
        { at: at('log'), badge: 'br', title: 'Log today',
          text: 'Writes <code>:MOOD:</code> into today\'s day page.' },
        { at: at('sleep'), badge: 'r', title: 'A line with a goal',
          text: '<code>type:line from:-90d goal:7:00</code>: durations are read as <code>7:30</code>, <code>1h30m</code> or <code>45m</code>.' },
      ],
    },

    // ---------------------------------------------------------- related
    { name: 'cli-related',
      cli: { title: "orgs related 'Leaf mould'", args: ['related', 'Leaf mould', '-n', '5'], opts: { cols: 124, rows: 14 } },
      notes: [
        { find: 'like Leaf mould', title: 'Like a heading',
          text: 'A title, a query or a hash; or a file, or a line of one. By words here; <code>-engine embeddings</code> asks a model.' },
        { find: 'Mulching', title: 'Each with its score',
          text: 'From 0 to 1, with the file and the outline path.' },
        { find: 'leaf, mould, mulching', title: 'Why',
          text: 'The words the two share most, and the sentence most like the heading asked about.' },
        { find: 'talks.org:', title: 'From any file',
          text: 'Notes on a talk turn up beside the garden. <code>-link 2</code> writes the second into a <code>Related:</code> list.' },
      ] },
    {
      name: 'worg-related', title: 'worg · Files · garden.org', page: { height: 880 },
      worg: async (page, ctx) => {
        // The rendered view asks about the heading at the top of the page.
        await pageTheme(page, 'journal');
        await openFile(page, ctx, 'garden.org', 'Rendered');
        await sleep(2000);
        await clickShown(page, '[aria-label="Related notes"]');
        await sleep(1200);
        await frameScroll(page, { text: 'Leaf mould' }, 12);
        await sleep(2500);
        await scrub(page);
        await tag(page, [
          { name: 'button', selector: '[aria-label="Related notes"]', closest: 'button' },
          { name: 'scope', text: 'This heading', within: 'main', up: 1 },
          { name: 'first', text: 'Mulching', within: 'main', last: true, up: 2 },
          { name: 'link', selector: 'main [aria-label="Link it"]' },
          { name: 'foot', text: 'Embeddings are not set up', contains: true, within: 'main' },
        ]);
        return mainClip(page);
      },
      notes: [
        { at: at('button'), badge: 'bl', title: 'The ✨ button',
          text: 'Opens the panel beside the page; it follows the heading you are on.' },
        { at: at('scope'), badge: 'tr', ring: false, title: 'This heading, or the whole file',
          text: 'A heading asks with its subtree; its parents, its children and what links already join to it are left out (<em>linked too</em> keeps them).' },
        { at: at('first'), badge: 'tl', ring: false, title: 'A note like it',
          text: 'With its score, its shared words and its closest sentence. Click to go there.' },
        { at: at('link'), badge: 'r', title: 'Link it',
          text: 'Writes <code>[[id:…][Mulching]]</code> into a <code>Related:</code> list under the heading.' },
        { at: at('foot'), badge: 'tr', ring: false, title: 'Words, or a model',
          text: 'Words need no setup. Name an embeddings backend (ollama, local, openai, gemini) in <code>related:</code> for notes alike in meaning.' },
      ],
    },

    // ---------------------------------------------------------- media notes
    { name: 'cli-media-notes',
      cli: { title: "orgs media notes 'soil food web'", args: ['media', 'notes', 'soil food web'], opts: { cols: 100, rows: 12 } },
      notes: [
        { find: 'The soil food web', title: 'The heading',
          text: 'Named by words of its title. Its recording is its <code>:MEDIA:</code> property (or <code>:AUDIO:</code>, or a link in it).' },
        { find: 'video:talks', title: 'The recording',
          text: 'A file beside the notes, a url, or <code>cite:key</code> through the bibliography.' },
        { find: '0:11:30-0:13:05', title: 'Notes, by time',
          text: 'Each a list item starting <code>[[video:…#0:06:15][0:06:15]]</code>, as org-media-note writes them; a span is an A-B loop. <code>orgs media add</code> writes one, <code>play</code> opens mpv there.' },
      ] },
    {
      name: 'worg-media-notes', title: 'worg · Media Notes', page: { height: 820 },
      worg: async (page, ctx) => {
        await ctx.openApp(page, 'MediaNotes');
        await ctx.clickText(page, 'The soil food web, at the allotment society', { within: 'main', settle: 2500 });
        await ctx.clickText(page, '0:18:50', { within: 'main', settle: 2500 });
        await page.evaluate(() => document.querySelector('main video')?.pause());
        await page.click('main [placeholder^="A note at"]');
        await page.keyboard.type('worms pull a mulch down in weeks, not months', { delay: 15 });
        await sleep(800);
        await tag(page, [
          { name: 'list', text: 'Recordings', within: 'main', up: 1 },
          { name: 'video', selector: 'main video' },
          { name: 'ab', text: 'A', within: 'main', closest: 'button' },
          { name: 'note', selector: 'main [placeholder^="A note at"]' },
          { name: 'loop', text: '0:11:30-0:13:05', within: 'main' },
          { name: 'now', text: 'leaf mould is fungal, compost is mostly bacterial: use both', within: 'main', up: 1 },
        ]);
        const bottom = await page.evaluate(() => [...document.querySelectorAll('main *')].find(e => e.textContent.trim() === 'the reading list at the end').getBoundingClientRect().bottom);
        return mainClip(page, { bottom: bottom + 40 });
      },
      notes: [
        { at: at('list'), badge: 'tr', ring: false, title: 'Every heading with a recording',
          text: '<code>:MEDIA: talks/soil-food-web.mp4</code>: videos, audio, YouTube and Vimeo urls.' },
        { at: at('now'), badge: 'r', title: 'Click a note to go there',
          text: 'The player jumps to <code>0:18:50</code>.' },
        { at: at('loop'), badge: 'l', title: 'A loop',
          text: '<code>#0:11:30-0:13:05</code> plays its span over and over; <em>A</em> and <em>B</em> mark one.' },
        { at: at('note'), badge: 'r', ring: false, title: 'A new note',
          text: 'Stamped with the moment you started typing; ctrl-enter writes it among the others, in time order.' },
      ],
    },

    // ---------------------------------------------------------- callouts
    { name: 'cli-callouts',
      cli: { title: 'orgs callouts ls garden.org', args: ['callouts', 'ls', 'garden.org'], opts: { cols: 76, rows: 10 } },
      notes: [
        { find: 'Tomatoes', title: 'Under their heading',
          text: 'Every callout in the file; <code>orgs callouts</code> alone lists them in every file.' },
        { find: 'Frost', title: 'The line and the type',
          text: '<code>#+begin_warning Frost</code>: the block\'s name is the type, the words after it the title.' },
        { find: 'splitting? ▸', title: 'Folded',
          text: '<code>:fold closed</code> starts it shut. <code>orgs callouts check</code> warns about a type no one knows.' },
      ] },
    {
      name: 'worg-callouts', title: 'worg · Files · garden.org', page: { height: 1000 },
      worg: async (page, ctx) => {
        await pageTheme(page, 'journal');
        await openFile(page, ctx, 'garden.org', 'Rendered');
        await sleep(2500);
        await scrub(page);
        await frameScroll(page, { selector: '.callout-tip' }, 40);
        await tagFrame(page, [
          { name: 'tip', selector: '.callout-tip .callout-title' },
          { name: 'warning', selector: '.callout-warning' },
          { name: 'info', selector: '.callout-info .callout-title' },
          { name: 'faq', selector: '.callout-question' },
          { name: 'danger', selector: '.callout-danger' },
        ]);
        const bottom = await page.evaluate(() => document.querySelector('[data-shot="danger"]').getBoundingClientRect().bottom);
        return mainClip(page, { bottom: bottom + 30 });
      },
      notes: [
        { at: at('tip'), badge: 'tr', title: 'A special block, as a callout',
          text: '<code>#+begin_tip Pinch out the side shoots</code> … <code>#+end_tip</code>: org\'s own block, which Emacs reads as it always has.' },
        { at: at('warning'), badge: 'r', ring: false, title: 'The callout types and colours',
          text: 'note, tip, info, success, question, warning, danger, bug, quote… and their aliases (<code>caution</code>, <code>faq</code>, <code>error</code>).' },
        { at: at('faq'), badge: 'r', title: 'Folded',
          text: '<code>#+begin_faq Why is the fruit splitting? :fold closed</code> - a <code>&lt;details&gt;</code> that opens in any browser, no script.' },
        { at: at('danger'), badge: 'r', ring: false, title: 'In every export',
          text: 'A tcolorbox in LaTeX, a block in Typst and beamer, a bar in plain text, <code>&gt; [!danger]</code> in markdown.' },
      ],
    },

    // ---------------------------------------------------------- backlink counts
    { name: 'cli-backlink-counts',
      cli: { title: 'orgs linkcounts garden.org', args: ['linkcounts', 'garden.org'], opts: { cols: 84, rows: 8 } },
      notes: [
        { find: 'Compost', title: 'Places linking here',
          text: 'The linked-to headings, each with <code>↩ n</code>: how many headings link to it, from any file (and from boards).' },
        { find: 'Rainwater', title: 'Any kind of link',
          text: '<code>[[id:…]]</code>, <code>[[*Title]]</code>, <code>[[file:x.org::*Title]]</code>, <code>[[#custom-id]]</code>. <code>-mentions</code> adds unlinked mentions (<code>≈ 2</code>), <code>-top</code> the most linked anywhere.' },
      ] },
    {
      name: 'worg-backlink-counts', title: 'worg · Files · garden.org', page: { height: 900 },
      worg: async (page, ctx) => {
        await pageTheme(page, 'journal');
        await openFile(page, ctx, 'garden.org', 'Rendered');
        await sleep(2500);
        await scrub(page);
        await frameScroll(page, { text: 'Soil' }, 20);
        // A badge opens the links panel at its heading.
        await page.evaluate(() => {
          const d = document.querySelector('main iframe').contentDocument;
          const h = [...d.querySelectorAll('h1,h2,h3,h4,h5')].find(e => e.textContent.includes('Compost'));
          h.querySelector('.worg-backlinks').click();
        });
        await sleep(2500);
        await scrub(page);
        await tagFrame(page, [
          { name: 'nodig', selector: '.worg-backlinks', nth: 0, closest: 'h1,h2,h3,h4,h5,h6' },
          { name: 'compost', selector: '.worg-backlinks', nth: 1, closest: 'h1,h2,h3,h4,h5,h6' },
        ]);
        await tag(page, [
          { name: 'from', text: 'Linked from', contains: true, within: 'main', up: 1 },
        ]);
        return mainClip(page);
      },
      notes: [
        { at: at('nodig'), badge: 'r', title: '↩ 4',
          text: 'Four places link to <em>No-dig beds</em>. Worked out from the links already in your files; nothing is written into them.' },
        { at: at('compost'), badge: 'r', title: 'Click a badge',
          text: 'The links panel opens at that heading.' },
        { at: at('from'), badge: 'tl', title: 'Every place linking to it',
          text: 'With the line each is written on - a heading, another file, the garden board. Click one to go there.' },
      ],
    },
    {
      name: 'worg-backlink-counts-editor', title: 'worg · Files · editing garden.org', page: { height: 760 },
      worg: async (page, ctx) => {
        await openFile(page, ctx, 'garden.org', 'Org');
        await ctx.clickText(page, 'Edit', { within: 'main', settle: 2500 });
        await clickLine(page, '.cm-line', 'Three bays: one filling');
        await page.evaluate(() => {
          const l = [...document.querySelectorAll('.cm-line')].find(e => e.textContent.includes('Rainwater butts'));
          const sc = document.querySelector('.cm-scroller');
          sc.scrollTop += l.getBoundingClientRect().top - sc.getBoundingClientRect().top - 40;
          sc.scrollLeft = 0;
        });
        await sleep(1500);
        await scrub(page);
        await tag(page, [
          { name: 'head', text: '↩ 3', contains: true, within: '.cm-content', nth: 0 },
          { name: 'link', text: '↩', contains: true, within: '.cm-content', nth: 1 },
        ]);
        await widen(page, ['head', 'link'], 14);
        return mainClip(page);
      },
      notes: [
        { at: at('head'), badge: 'r', title: 'On the heading',
          text: 'How many places link to it, kept up as you type: the server counts the text you have, saved or not.' },
        { at: at('link'), badge: 'r', title: 'On a link',
          text: 'How many other links go where this one goes. Click either to see them.' },
      ],
    },

    // ---------------------------------------------------------- drawings
    // The editor first, saving: Excalidraw's own svg (with its hand-drawn
    // font inlined) is then what the rendered page and the Drawings tab show.
    {
      name: 'worg-excalidraw-editor', title: 'worg · Drawings · rainwater.excalidraw', page: { height: 820 },
      worg: async (page, ctx) => {
        await openDrawing(page, ctx, 'rainwater');
        await page.evaluate(() => [...document.querySelectorAll('button')].find(b => b.textContent.trim() === 'Save')?.click());
        await sleep(4000);
        await tag(page, [
          { name: 'bar', text: 'rainwater.excalidraw', up: 1 },
          { name: 'save', text: 'Save', closest: 'button' },
        ]);
        return null;
      },
      notes: [
        { title: 'Excalidraw itself',
          text: 'The drawing opens over the page, in Excalidraw\'s own editor; dark mode follows worg\'s.' },
        { at: at('save'), badge: 'bl', title: 'Saved beside it',
          text: 'With ctrl-s and two seconds after the last change: the drawing, <code>rainwater.excalidraw.svg</code> and a png for LaTeX.' },
        { title: 'Changed elsewhere',
          text: 'A drawing changed in another app or excalidraw.com since it opened is not overwritten: the bar offers Overwrite or Reload theirs.' },
      ],
    },
    {
      name: 'worg-excalidraw-export', title: 'worg · Files · garden.org', page: { height: 900 },
      worg: async (page, ctx) => {
        await pageTheme(page, 'journal');
        await openFile(page, ctx, 'garden.org', 'Rendered');
        await sleep(2500);
        await scrub(page);
        await frameScroll(page, { selector: 'img[src*="rainwater"]' }, 60);
        await tagFrame(page, [
          { name: 'pic', selector: 'img[src*="rainwater"]' },
          { name: 'cap', text: 'Rain from the shed roof to the beds', contains: true, last: true, range: true },
        ]);
        await widen(page, ['cap'], 16);
        const bottom = await page.evaluate(() => document.querySelector('[data-shot="cap"]').getBoundingClientRect().bottom);
        return mainClip(page, { bottom: bottom + 40 });
      },
      notes: [
        { at: at('pic'), badge: 'tr', title: 'A drawing, linked as a picture',
          text: '<code>[[file:drawings/rainwater.excalidraw]]</code>: a link with no description is the drawing itself, in every export. Click it to edit.' },
        { at: at('cap'), badge: 'r', ring: false, title: 'Captioned and sized',
          text: '<code>#+CAPTION:</code> and <code>#+ATTR_HTML: :width 640</code>, as for any picture.' },
      ],
    },
    {
      name: 'worg-excalidraw', title: 'worg · Drawings', page: { height: 760 },
      worg: async (page, ctx) => {
        await ctx.openApp(page, 'Drawings');
        await sleep(1500);
        await tag(page, [
          { name: 'card', text: 'rainwater', within: 'main', closest: '.MuiCard-root, .MuiSheet-root' },
          { name: 'linked', text: 'garden.org:', contains: true, within: 'main', nth: 0 },
          { name: 'new', selector: 'main input[placeholder="drawings/new-diagram"]' },
        ]);
        const bottom = await page.evaluate(() => document.querySelector('[data-shot="card"]').getBoundingClientRect().bottom);
        return mainClip(page, { bottom: bottom + 30 });
      },
      notes: [
        { at: at('card'), badge: 'tr', title: 'Every drawing',
          text: 'The <code>.excalidraw</code> files under the org directories (and markdown <code>.excalidraw.md</code> drawings), newest first, with their pictures.' },
        { at: at('linked'), badge: 'r', title: 'What links to it',
          text: 'The org lines that link the drawing; click one to go there.' },
        { at: at('new'), badge: 'bl', title: 'A new drawing',
          text: 'Or <code>orgs draw new drawings/flow -link-from Design</code>, which links it under a heading too.' },
      ],
    },
  ],
};
