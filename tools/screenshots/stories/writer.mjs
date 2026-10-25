// The writer story's pictures (docs/story-writer.org): Owen Hale writing a
// novel, The Tidewater Clock, in worg's writing mode and at the command
// line. Its own notebook (docs/screenshots/writer), so none of it shows up in
// the other pictures.
//
// The writing log is posted in the first shot's setup, not seeded in the
// yaml, because it is keyed by the server's absolute path to the file. It is
// a running count: each day starts where the last ended. worg posts the
// file's real count the moment the editor opens, so yesterday's end is set
// to that count less today's words (TODAY), worked out here with worg's own
// rules (prose only: no drawers, keywords, comments or tags).

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const repo = path.resolve(here, '../../..');
const notebook = path.join(repo, 'docs/screenshots/writer');

// Words a day for the last six weeks, oldest first, ending yesterday: a
// streak of nine days now, a bad patch three weeks back, rest days.
const DAYS = [
  420, 610, 0, 530, 505, 690, 0,
  560, 380, 0, 0, 240, 515, 0,
  300, 0, 210, 0, 0, 450, 620,
  580, 0, 640, 510, 720, 530, 0,
  505, 610, 550, 0, 680, 540, 525,
  600, 515, 830, 560, 505, 610, 545,
];
const TODAY = 437;

const WORD = /[\p{L}\p{N}][\p{L}\p{N}'’]*(?:-[\p{L}\p{N}][\p{L}\p{N}'’]*)*/gu;
// worg's countWords (writing.ts), for this file's shapes.
function proseWords(text) {
  let n = 0, drawer = false;
  for (let l of text.split('\n')) {
    if (/^\s*:(PROPERTIES|LOGBOOK):\s*$/i.test(l)) { drawer = true; continue; }
    if (drawer) { if (/^\s*:END:\s*$/i.test(l)) drawer = false; continue; }
    if (/^\s*#/.test(l) || /^\s*(SCHEDULED|DEADLINE|CLOSED):/.test(l)) continue;
    const h = /^(\*+)\s+(.*)$/.exec(l);
    if (h) l = h[2].replace(/\s+(:[\w@#%:]+:)\s*$/, '');
    const m = l.match(WORD);
    if (m) n += m.length;
  }
  return n;
}

const pad = n => String(n).padStart(2, '0');
function iso(off) {
  const d = new Date();
  d.setHours(12, 0, 0, 0);
  d.setDate(d.getDate() + off);
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

// The Apprentice as it was before the second pass: the snapshot the
// revision view compares against.
function olderApprentice(text) {
  return text
    .replace('She climbed the tower before the gulls were up, one hundred and twelve stone steps, counting them the way Silas had taught her, because a person who counts does not think about how far down the bottom is.',
      'Every morning she climbed the tower before it was light. There were a great many steps and she counted them, because Silas said that counting helped.')
    .replace('He looked at her for a long moment, the way he looked at a movement he did not trust.', 'He looked at her for a long time.')
    .replace(' She had very strong opinions about pencils.', '')
    .replace('"Then the sea is wrong," said Ada, "and I have never yet won an argument with the sea."', '"Then the sea is wrong," said Ada.');
}

async function seedWriting(ctx) {
  const novel = path.join(ctx.work, 'org', 'novel.org');
  const comp = path.join(ctx.work, 'org', 'novel.compendium.org');
  const text = fs.readFileSync(novel, 'utf8');
  await ctx.post('writing/goals', { Daily: 500, Weekly: 3000, Monthly: 12000, RestDays: 1 });

  // The novel: a baseline the day before the first, then a running count.
  const sum = DAYS.reduce((a, b) => a + b, 0);
  let count = proseWords(text) - TODAY - sum;
  await ctx.post('writing/log', { File: novel, Date: iso(-DAYS.length - 1), Words: count });
  for (let i = 0; i < DAYS.length; i++) {
    count += DAYS[i];
    await ctx.post('writing/log', { File: novel, Date: iso(i - DAYS.length), Words: count });
  }
  // Today, as worg posts it the moment the editor opens, so a picture taken
  // without opening it (orgs write) still has today's words.
  await ctx.post('writing/log', { File: novel, Date: iso(0), Words: proseWords(text) });
  // A little in the compendium now and then: notes on people and places.
  let c = 400;
  await ctx.post('writing/log', { File: comp, Date: iso(-30), Words: c });
  for (const [off, w] of [[-26, 120], [-19, 85], [-12, 140], [-5, 95], [-2, 60]]) {
    c += w;
    await ctx.post('writing/log', { File: comp, Date: iso(off), Words: c });
  }

  // The snapshot, taken five days ago before the second pass.
  const lines = text.split('\n');
  const from = lines.findIndex(l => /^\*\* The Apprentice\s*$/.test(l));
  const to = lines.findIndex((l, i) => i > from && /^\*{1,2} /.test(l));
  const old = olderApprentice(lines.slice(from, to).join('\n').trimEnd());
  const r = await ctx.post('writing/snapshots', { File: novel, Snapshot: {
    Heading: 'Part One: The Wrong Tide\u0000The Apprentice', Title: 'Before the second pass',
    Text: old, Words: (old.match(/\p{L}[\p{L}'’]*/gu) || []).length } });
  if (!r.Ok) throw new Error(`snapshot: ${r.Msg}`);
  const store = path.join(ctx.work, 'org', '.snapshots', 'novel.org.json');
  if (fs.existsSync(store)) {
    const all = JSON.parse(fs.readFileSync(store, 'utf8'));
    const back = new Date(Date.now() - 5 * 86400000);
    back.setHours(9, 12, 0, 0);
    for (const s of Array.isArray(all) ? all : all.Snapshots || []) s.Time = back.toISOString();
    fs.writeFileSync(store, JSON.stringify(all, null, 2));
  }
}

// Once per server, from whichever shot runs first (a run can ask for any).
const seeded = new Set();
async function seedOnce(ctx) {
  if (seeded.has(ctx.work)) return;
  seeded.add(ctx.work);
  await seedWriting(ctx);
}

// ---------------------------------------------------------------- worg

const settings = {
  writing: true, writingFocus: 'off', writingTypewriter: false, writingSessionTarget: 1000,
  editorKeys: 'emacs', writingOutline: true, compendiumUnderline: true,
  // The book's British spellings and its names, for the speller.
  writingWords: ['harbour', 'harbourmaster', 'grey', 'Saltmere', 'Aldous', 'Penhallow', 'Jory', 'Marr', 'oilskin'].join('\n'),
};
const wait = ms => new Promise(r => setTimeout(r, ms));

// Writing mode on the novel, the whole window.
async function openWriting(page, ctx) {
  await page.evaluateOnNewDocument(s => { try { localStorage.setItem('worg.settings', JSON.stringify(s)); } catch { } }, settings);
  await ctx.openApp(page, 'Files');
  await ctx.clickText(page, 'novel.org', { within: 'main', settle: 1500 });
  await ctx.clickText(page, 'Write', { within: 'main', settle: 2500 });
  await bar(page, 'FullscreenRoundedIcon');
}

// A button on the writing bar by its icon (icons repeat elsewhere: the app
// panel's toggle is a Dashboard icon too).
async function bar(page, icon, settle = 1500) {
  const ok = await page.evaluate(id => {
    const o = [...document.querySelectorAll('button')].find(b => b.textContent.trim() === 'Outline');
    let el = o;
    while (el && !el.querySelector('svg[data-testid=FlagRoundedIcon]')) el = el.parentElement;
    const svg = el && el.querySelector(`svg[data-testid=${id}]`);
    const b = svg && svg.closest('button');
    if (b) b.click();
    return !!b;
  }, icon);
  if (!ok) throw new Error(`no ${icon} on the writing bar`);
  await wait(settle);
}

// Tags the parts of the screen a picture's notes point at, as data-shot
// attributes, so a note is { selector: '[data-shot=bar]' } however deep the
// part sits. Each finder runs in the page and returns an element or null.
async function mark(page, finders) {
  const missing = await page.evaluate(src => {
    const out = [];
    for (const [name, body] of Object.entries(src)) {
      const el = new Function(body)();
      if (el) el.setAttribute('data-shot', name); else out.push(name);
    }
    return out;
  }, finders);
  if (missing.length) throw new Error(`nothing to mark for ${missing.join(', ')}`);
}
// Finder bodies used by several shots.
const F = {
  bar: `const o = [...document.querySelectorAll('button')].find(b => b.textContent.trim() === 'Outline');
        let el = o; while (el && !el.querySelector('svg[data-testid=FlagRoundedIcon]')) el = el.parentElement; return el;`,
  right: `const f = document.querySelector('svg[data-testid=FlagRoundedIcon]').closest('button');
          let el = f; while (el && !el.querySelector('svg[data-testid=FullscreenExitRoundedIcon]')) el = el.parentElement; return el;`,
  status: `return [...document.querySelectorAll('span,div')].filter(e => /\\/ 85,000 words/.test(e.textContent) && e.getClientRects().length)
             .sort((a, b) => a.textContent.length - b.textContent.length)[0] || null;`,
  outline: `const d = [...document.querySelectorAll('*')].find(e => e.textContent.trim().startsWith('Drag to move') && !e.children.length);
            let el = d; while (el && !el.textContent.includes('Part One: The Wrong Tide')) el = el.parentElement; return el;`,
};
const at = name => ({ selector: `[data-shot=${name}]` });
// The writing bar's button with this icon (the last on the page: the bar
// comes after the app panel, whose icons repeat some of its).
const barIcon = id => ({ selector: `svg[data-testid=${id}]`, closest: 'button', last: true });
// The smallest element holding this text that is at least h pixels tall: a
// card, a section, a row of tiles.
const holding = (text, h = 40, w = 0) => `let el = [...document.querySelectorAll('main *, [role=dialog] *, body *')]
  .filter(e => e.textContent.trim().startsWith(${JSON.stringify(text)}) && e.getClientRects().length)
  .sort((a, b) => a.textContent.length - b.textContent.length)[0];
  while (el && (el.getBoundingClientRect().height < ${h} || el.getBoundingClientRect().width < ${w})) el = el.parentElement; return el || null;`;
// The smallest element holding all of these texts, right of x: the box
// that is exactly a card or a row. Never in the editor, which holds the same
// words (a synopsis, a status) out of sight behind a pane.
const common = (texts, x = 0) => `return [...document.querySelectorAll('body *')]
  .filter(e => !e.closest('.cm-editor') && ${JSON.stringify(texts)}.every(t => e.textContent.includes(t)) && e.getClientRects().length && e.getBoundingClientRect().left >= ${x})
  .sort((a, b) => a.textContent.length - b.textContent.length)[0] || null;`;
// A dialog's box, with room round it, as a clip.
async function dialogClip(page, pad = 16) {
  const r = await page.evaluate(() => {
    const d = [...document.querySelectorAll('[role=dialog]')].filter(e => e.getClientRects().length).pop();
    const b = d.getBoundingClientRect();
    return { x: b.left, y: b.top, w: b.width, h: b.height };
  });
  return { x: Math.max(0, Math.round(r.x - pad)), y: Math.max(0, Math.round(r.y - pad)),
    width: Math.round(r.w + 2 * pad), height: Math.round(r.h + 2 * pad) };
}

const page = { width: 1440, height: 900 };
const shot = o => ({ setup: seedOnce, page, ...o });

export default {
  demo: {
    src: notebook,
    extensions: path.join(notebook, 'extensions.yaml'),
  },
  shots: [
    // ------------------------------------------------------------ writing mode
    shot({
      name: 'writer-writing', title: 'worg · Files · novel.org, writing mode', page,
      worg: async (p, ctx) => {
        await openWriting(p, ctx);
        await ctx.clickText(p, 'The tower at dawn', { settle: 1200 });
        // A sentence typed this session, at the end of the scene, above its
        // revision note.
        const END = 'and he said nothing at all.';
        const box = await p.evaluate(end => {
          const line = [...document.querySelectorAll('.cm-line')].find(l => l.textContent.endsWith(end));
          line.scrollIntoView({ block: 'center' });
          const r = line.getBoundingClientRect();
          return { x: r.right - 4, y: r.bottom - 10 };
        }, END);
        await wait(600);
        await p.mouse.click(box.x, box.y);
        await p.keyboard.press('End');
        await p.keyboard.type(' Below them the boats sat on the mud like dropped shoes.', { delay: 8 });
        await wait(1500);
        await mark(p, { ...F,
          text: `return [...document.querySelectorAll('.cm-line')].find(l => l.textContent.startsWith('He looked at her for a long moment'));`,
          note: `let el = [...document.querySelectorAll('*')].filter(e => e.textContent.trim().startsWith('Silas\\'s silence lands') && e.getClientRects().length).pop();
                 while (el && el.getBoundingClientRect().height < 40) el = el.parentElement; return el;`,
        });
      },
      notes: [
        { at: at('bar'), badge: 'bl', title: 'The writing bar',
          text: 'Panes to open beside the text: outline, corkboard, compendium, ideas, the assistant, dictation, scene synopses, continuity, prose, revision and storyline. Then typewriter, focus, font, column, and the checker with its spelling, grammar and style counts.' },
        { at: at('right'), badge: 'bl', title: 'Targets, goals and compile',
          text: 'Word targets for the session, the file and this heading; goals and rhythm; compile; the thesaurus; full screen; leave writing mode.' },
        { at: at('outline'), badge: 'r', title: 'The outline',
          text: 'Every heading with its words. The dot is its <code>STATUS</code> (blue final, orange revised, red first draft, grey to do), the bar fills toward its <code>WORD_TARGET</code>, the grey line is its <code>SYNOPSIS</code>. Drag a heading to move it and everything under it.' },
        { at: at('text'), badge: 'tl', ring: false, title: 'The text',
          text: 'Org as it is, with headings, drawers and keywords kept quiet. The checkers underline as you type: red spelling, blue grammar, green style (a word used twice close together, a passive).' },
        { at: at('note'), badge: 'tl', title: 'A revision note',
          text: 'A <code># NOTE:</code> line under a paragraph, shown in the margin. It is an org comment, so it never reaches the book.' },
        { at: at('status'), badge: 'tl', title: 'The status line',
          text: 'Words in the file against <code>#+WORD_TARGET</code>, words written this session (the ring fills toward the session target), words in this heading, and the notes still to fix.' },
      ],
    }),
    // ------------------------------------------------------------ planning
    shot({
      name: 'writer-corkboard', title: 'worg · writing mode · corkboard', page,
      worg: async (p, ctx) => {
        await openWriting(p, ctx);
        await bar(p, 'DashboardRoundedIcon');
        await ctx.clickText(p, '8 under it ›', { settle: 1500 });
        await mark(p, { outline: F.outline,
          crumb: `return [...document.querySelectorAll('main *, body *')].find(e => e.textContent.trim() === 'The whole file' && !e.children.length) || null;`,
          card: `${common(['The Apprentice', 'Wren winds the harbour', 'Final'], 250)}`,
          under: `return [...document.querySelectorAll('*')].filter(e => e.textContent.trim().startsWith('2 under it') && e.getClientRects().length).sort((a, b) => a.textContent.length - b.textContent.length)[1] || null;`,
        });
        return { x: 0, y: 0, width: 1440, height: 300 };
      },
      notes: [
        { at: at('crumb'), badge: 'tr', title: 'Where you are',
          text: 'The whole file, then the part you went into. Click a step to go back up.' },
        { at: at('card'), badge: 'tr', title: 'An index card',
          text: 'A heading as a card: its <code>SYNOPSIS</code>, its words against its <code>WORD_TARGET</code> (the ring), its <code>STATUS</code> (the colour along the top). Click to edit the synopsis, double-click to write it, drag to reorder the chapters.' },
        { at: at('under'), badge: 'tr', title: 'What is under it',
          text: 'The scenes in a chapter, as cards of their own.' },
        { at: at('outline'), badge: 'r', ring: false, title: 'The outline',
          text: 'Still beside it, and the same headings: a card moved is a heading moved.' },
      ],
    }),
    shot({
      name: 'writer-storyline', title: 'worg · writing mode · storyline', page,
      worg: async (p, ctx) => {
        await openWriting(p, ctx);
        await ctx.clickText(p, 'Outline', { settle: 800 });
        await bar(p, 'TimelineRoundedIcon');
        await mark(p, {
          level: `return [...document.querySelectorAll('button,[role=combobox]')].find(e => e.textContent.includes('Scenes: headings')) || null;`,
          names: `${holding('Names found in the text', 20, 150)}`,
          cast: `${common(['Wren', 'Silas', 'Jory', 'Ada'], 0)}`.replace("e.getClientRects().length", "e.getClientRects().length && e.getBoundingClientRect().height < 60"),
          tension: `return [...document.querySelectorAll('svg text, text')].find(e => e.textContent.trim() === 'tension') || null;`,
          words: `return [...document.querySelectorAll('svg text, text')].find(e => e.textContent.trim() === '246') || null;`,
        });
        return { x: 0, y: 0, width: 1440, height: 450 };
      },
      notes: [
        { at: at('level'), badge: 'tr', title: 'Which headings are scenes',
          text: 'The headings with nothing under them, or one outline level. Parts are drawn as bands behind them.' },
        { at: at('names'), badge: 'tr', title: 'Who is in a scene',
          text: 'From the scene\'s <code>CHARACTERS</code> and <code>POV</code>, and, with this on, names written in it often enough.' },
        { at: at('cast'), badge: 'tr', title: 'The cast',
          text: 'Everyone in the book and how many scenes they are in. Click one to follow their line alone.' },
        { at: at('tension'), badge: 'r', ring: false, title: 'The tension arc',
          text: 'Each scene\'s <code>TENSION</code> (0 to 10), drawn over the scenes: the shape of the book at a glance.' },
        { at: at('words'), badge: 'r', ring: false, title: 'Lines and words',
          text: 'A line per character through the scenes, drawn together where they meet. Under each scene, its words.' },
      ],
    }),
    shot({
      name: 'writer-compendium', title: 'worg · writing mode · compendium', page,
      worg: async (p, ctx) => {
        await openWriting(p, ctx);
        await ctx.clickText(p, 'Outline', { settle: 800 });
        await bar(p, 'MenuBookRoundedIcon');
        await ctx.clickText(p, 'Wren Aldous', { settle: 800 });
        await ctx.clickText(p, 'Next mention', { settle: 1200 });
        await p.evaluate(() => {
          const n = [...document.querySelectorAll('*')].find(e => e.textContent.trim() === 'Wren Aldous' && !e.children.length);
          if (n) n.scrollIntoView({ block: 'start' });
        });
        await wait(500);
        await mark(p, {
          search: `return document.querySelector('input[placeholder="Search names, facts, notes"]');`,
          card: `${holding('Wren Aldous', 300, 200)}`,
          places: `return [...document.querySelectorAll('*')].find(e => /^places/i.test(e.textContent.trim()) && e.textContent.trim().length < 12 && e.getClientRects().length) || null;`,
          text: `return [...document.querySelectorAll('.cm-line')].find(l => l.textContent.startsWith('Silas Marr came up the last steps')) || null;`,
        });
      },
      notes: [
        { at: at('search'), badge: 'tr', title: 'The compendium',
          text: 'The people, places and things in <code>novel.compendium.org</code> (named by <code>#+COMPENDIUM:</code>), searched by name, fact or note.' },
        { at: at('card'), badge: 'tr', title: 'An entry',
          text: 'Its other names (<code>ALIASES</code>), its facts (every other property, and one to add), its notes, the scenes it is in and how often. <em>Next mention</em> goes to the next one in the text.' },
        { at: at('places'), badge: 'r', ring: false, title: 'By kind',
          text: 'Characters, places, things and groups, from <code>KIND</code> or the top heading an entry sits under.' },
        { at: at('text'), badge: 'tl', ring: false, title: 'Names in the text',
          text: 'Every name the compendium knows is underlined; rest on one for its card.' },
      ],
    }),
    shot({
      name: 'writer-prose', title: 'worg · writing mode · prose', page,
      worg: async (p, ctx) => {
        await openWriting(p, ctx);
        await ctx.clickText(p, 'Outline', { settle: 800 });
        await bar(p, 'QueryStatsRoundedIcon', 2000);
        await mark(p, {
          tiles: `${holding('1,', 50, 600)}`,
          rhythm: `${holding('Sentence rhythm', 150)}`,
          read: `${holding('Readability by chapter', 150)}`,
          dialogue: `${holding('Dialogue and narration by scene', 150)}`,
          words: `${holding('Words worth a second look', 120)}`,
        });
      },
      notes: [
        { at: at('tiles'), badge: 'tr', title: 'The file in numbers',
          text: 'Words, sentences, their average length, how many run over forty words, reading ease, grade level and how much is dialogue. Prose only: headings, drawers and notes are left out.' },
        { at: at('rhythm'), badge: 'tr', title: 'Sentence rhythm',
          text: 'Every sentence as a bar, in order: a long run of the same height drones.' },
        { at: at('read'), badge: 'tr', title: 'Readability by chapter',
          text: 'Reading ease and words a sentence for each part or chapter.' },
        { at: at('dialogue'), badge: 'tr', title: 'Dialogue by scene',
          text: 'How much of each scene is inside quotation marks.' },
        { at: at('words'), badge: 'tr', title: 'Words worth a second look',
          text: 'Adverbs, filter words (<em>saw</em>, <em>felt</em>), weak words, dialogue tags, the most used words and repeated phrases. Open one to see each place it is used; a click goes there.' },
      ],
    }),
    shot({
      name: 'writer-goals', title: 'worg · writing mode · goals and rhythm', page,
      worg: async (p, ctx) => {
        await openWriting(p, ctx);
        await bar(p, 'InsightsRoundedIcon', 2000);
        await mark(p, {
          cards: `${common(['63 to go', 'This week', 'This month'])}`,
          stats: `${holding('day streak', 40, 700)}`,
          heat: `const m = [...document.querySelectorAll('[role=dialog] *')].find(e => e.textContent.trim() === 'Mon' && !e.children.length); let el = m; while (el && el.getBoundingClientRect().width < 700) el = el.parentElement; return el;`,
          day: `${common(['words of 500', '+437'])}`,
          scope: `return [...document.querySelectorAll('[role=dialog] button')].find(b => b.textContent.trim() === 'This file')?.parentElement || null;`,
        });
        return dialogClip(p);
      },
      notes: [
        { at: at('scope'), badge: 'bl', title: 'This file or every file',
          text: 'The novel alone, or everything written in writing mode. <em>Goals and look</em> sets the goals and the heatmap\'s colours.' },
        { at: at('cards'), badge: 'tr', title: 'Today, this week, this month',
          text: 'Words written against each goal: 500 a day, 3,000 a week, 12,000 a month here.' },
        { at: at('stats'), badge: 'tr', title: 'The rhythm',
          text: 'The streak (one rest day a week is allowed), the longest, words a day over the last thirty, how many of those met the goal, the year so far and the best day.' },
        { at: at('heat'), badge: 'tr', title: 'Every day you wrote',
          text: 'Darker is more words; a dot is a day the goal was met. Click a day to see it below.' },
        { at: at('day'), badge: 'tr', title: 'The day picked',
          text: 'Its words against the goal, file by file. Words taken out count against words put in.' },
      ],
    }),
    shot({
      name: 'writer-revision', title: 'worg · writing mode · revision', page,
      worg: async (p, ctx) => {
        await openWriting(p, ctx);
        await bar(p, 'RateReviewRoundedIcon', 1000);
        await ctx.choose(p, 'Off', 'Snapshot: Before the second pass');
        await p.keyboard.press('Escape');
        await wait(600);
        await ctx.clickText(p, 'The tower at dawn', { settle: 1500 });
        await p.evaluate(() => {
          const l = [...document.querySelectorAll('.cm-line')].find(l => l.textContent.startsWith('The harbour clock of Saltmere'));
          if (l) l.scrollIntoView({ block: 'start' });
        });
        await wait(800);
        await mark(p, { outline: F.outline,
          changed: `return [...document.querySelectorAll('.cm-line')].find(l => l.textContent.startsWith('Aldous had kept')) || [...document.querySelectorAll('.cm-line')].find(l => l.textContent.includes('Every morning she')) || null;`,
          note: `let el = [...document.querySelectorAll('*')].filter(e => e.textContent.trim().startsWith('Silas\\'s silence lands') && e.getClientRects().length).pop();
                 while (el && el.getBoundingClientRect().height < 30) el = el.parentElement; return el;`,
        });
      },
      notes: [
        { at: barIcon('RateReviewRoundedIcon'), badge: 'br', title: 'Revision',
          text: 'Add a note, list every note, and <em>Show changes since</em>: when the file was opened, the last save, or a snapshot. Here, the snapshot taken before the second pass.' },
        { at: at('changed'), badge: 'tr', ring: false, title: 'What changed',
          text: 'Words put in since the snapshot are lit; words taken out are struck through where they were. The file holds only the new text.' },
        { at: at('outline'), badge: 'r', ring: false, title: 'Snapshots',
          text: 'Taken from a heading\'s section dialog in the outline (or <code>orgs write snapshot</code>), kept beside the file, never in it. Compare or restore from there.' },
        { at: at('note'), badge: 'tl', title: 'Notes still to fix',
          text: 'The unticked ones; the status line counts them.' },
      ],
    }),
    shot({
      name: 'writer-compile', title: 'worg · writing mode · compile', page,
      worg: async (p, ctx) => {
        await openWriting(p, ctx);
        await bar(p, 'IosShareRoundedIcon', 1500);
        await mark(p, {
          tree: `${holding('What goes in', 300)}`,
          as: `return [...document.querySelectorAll('[role=dialog] button,[role=dialog] [role=combobox]')].find(e => e.textContent.includes('Word manuscript')) || null;`,
          style: `${holding('Manuscript style', 60, 200)}`,
          leave: `${holding('Leave out drawers', 30, 200)}`,
          go: `return [...document.querySelectorAll('[role=dialog] button')].find(b => b.textContent.trim() === 'Compile') || null;`,
        });
        return dialogClip(p);
      },
      notes: [
        { at: at('tree'), badge: 'tr', title: 'What goes in',
          text: 'Every heading, ticked or not, with its status. Research notes (<code>:noexport:</code>, or the status Notes) start left out.' },
        { at: at('as'), badge: 'tr', title: 'What comes out',
          text: 'A Word manuscript, an ebook, a PDF typeset by Typst, a web page, org or plain text.' },
        { at: at('style'), badge: 'tr', title: 'How it looks',
          text: 'A manuscript\'s style (modern, Shunn, novel, reader) and paper; an ebook\'s theme; a PDF\'s template. The file\'s own keywords (<code>#+CHAPTER_NUMBERS</code>, <code>#+SCENE_BREAK</code>) fill in the rest.' },
        { at: at('leave'), badge: 'tr', title: 'What is left out',
          text: 'Drawers and planning lines, and the synopses, statuses and targets writing mode keeps. Revision notes never go in.' },
        { at: at('go'), badge: 'tr', title: 'Compile',
          text: 'Downloads the book. Your files are not changed.' },
      ],
    }),
    // ------------------------------------------------------------ the terminal
    shot({
      name: 'writer-cli-write', cli: { title: 'orgs write', args: ['write'], opts: { cols: 84, rows: 18 } },
      notes: [
        { find: 'Today', title: 'Today, this week, this month',
          text: 'Words written against each goal, with what it takes to make the week.' },
        { find: 'Streak', title: 'The streak',
          text: 'Days in a row that met the daily goal, the longest, and the rest days a week allowed.' },
        { find: 'Last 14', title: 'The last fortnight',
          text: 'A spark line of words a day (green on goal days, <code>·</code> for none), the average, and how many of the last thirty days had words.' },
        { find: 'novel.org', title: 'Where today\'s words went',
          text: 'File by file. The log is kept by worg\'s writing mode as you type; this only reads it.' },
      ],
    }),
    shot({
      name: 'writer-cli-ideas', cli: { title: 'orgs write ideas place -seed 11', args: ['write', 'ideas', 'place', '-seed', '11'], opts: { cols: 104, rows: 16 } },
      notes: [
        { find: 'A fen island', title: 'The idea',
          text: 'A place for a scene, made from word tables in the binary: no service, nothing sent anywhere.' },
        { find: 'Could go wrong', title: 'Its parts',
          text: 'What you see, hear, smell and feel there, what is odd about it, and what could go wrong.' },
        { title: 'Make it again',
          text: '<code>-seed</code> makes the same idea again; <code>-org</code> prints it as a heading to paste or capture; the Ideas pane in writing mode files one in the compendium.' },
      ],
    }),
    shot({
      name: 'writer-cli-recap', cli: { title: 'orgs recap', args: ['recap'], opts: { cols: 92, rows: 34, side: true } },
      notes: [
        { find: ' words', title: 'The week in one line',
          text: 'Tasks done, hours clocked, words written against the weekly goal, habits kept, blockers.' },
        { find: '✅ *Done', title: 'Done',
          text: 'The business of the book finished this week: revisions, research.' },
        { find: '⏱ *Clocked', title: 'Clocked',
          text: 'Time on the clock (a pomodoro clocks in for you), by project.' },
        { find: '✍️ *Writing', title: 'Writing',
          text: 'Words written from the writing log, the days written, the days the daily goal was met, and the files.' },
        { find: '🚧 *Blocked', title: 'Blocked',
          text: "Waiting on somebody: here, the editor's notes." },
      ],
    }),
  ],
};
