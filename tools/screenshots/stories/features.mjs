// The pictures of org mode's own features, for the guides that had none:
// clocking and the clock report, links and backlinks, the command palette,
// the editor (folding, inline pictures, the date picker), refiling, the
// starmap, charting a table and the Collections tab. The demo notebook with
// one file of its own on top (docs/screenshots/features): client work with a
// week of clocked time, a sketch shown inline and a table of hours.
//
//   SHOT_PORT=8104 node capture.mjs -story features
//   SHOT_PORT=8104 node capture.mjs -story features worg-clock

import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { at, tag, clipAround, mainClip, toTop, clickShown } from './engineer-worg.mjs';

const here = path.dirname(fileURLToPath(import.meta.url));
const shots = path.resolve(here, '../../../docs/screenshots');

// The kickoff sketch clients.org shows inline. Written beside the org files
// (setupDemo copies only .org and .bib from a notebook folder).
const SKETCH = `<svg xmlns="http://www.w3.org/2000/svg" width="1040" height="360" viewBox="0 0 1040 360" font-family="Helvetica, Arial, sans-serif">
  <rect width="1040" height="360" fill="#fbfaf6"/>
  <g stroke="#2f3a4a" stroke-width="2.5" fill="#fff">
    <rect x="40" y="130" width="200" height="100" rx="14"/>
    <rect x="420" y="40" width="200" height="100" rx="14"/>
    <rect x="420" y="220" width="200" height="100" rx="14"/>
    <rect x="800" y="130" width="200" height="100" rx="14"/>
  </g>
  <g stroke="#2f6feb" stroke-width="3" fill="none" marker-end="url(#a)">
    <path d="M240 165 C330 140 340 100 415 92"/>
    <path d="M240 195 C330 220 340 260 415 268"/>
    <path d="M620 92 C700 100 720 150 795 168"/>
    <path d="M620 268 C700 260 720 210 795 192"/>
  </g>
  <defs><marker id="a" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto"><path d="M0 0L10 5L0 10z" fill="#2f6feb"/></marker></defs>
  <g fill="#1f2328" font-size="22" text-anchor="middle">
    <text x="140" y="187">Phone</text>
    <text x="520" y="85">Cart API</text><text x="520" y="113" font-size="15" fill="#6b7280">cached, 40ms</text>
    <text x="520" y="265">Image CDN</text><text x="520" y="293" font-size="15" fill="#6b7280">lazy, srcset</text>
    <text x="900" y="187">Orders DB</text>
  </g>
  <text x="520" y="350" font-size="14" fill="#9aa0a6" text-anchor="middle" font-style="italic">kickoff, whiteboard</text>
</svg>
`;

// Source blocks may run: the Code tab and orgs code run the python block
// that reads the table of hours.
const server = `  babel:
    enable: true
    timeout: 20
`;

export const demo = {
  src: [path.join(shots, 'demo'), path.join(shots, 'features')],
  files: { 'org/checkout.svg': SKETCH },
  server,
};

const sleep = ms => new Promise(r => setTimeout(r, ms));
// The demo lives in a temp folder; where worg prints its path (the file
// tree's header, the editor's title) say ~/notes instead. React draws the
// header again now and then, so this keeps watching.
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

// A file in the Files tab, in the view asked for, with the file tree shut so
// the page has the room.
async function openFile(page, ctx, file, view = 'Org') {
  await ctx.openApp(page, 'Files');
  await ctx.clickText(page, file, { settle: 2000 });
  await ctx.clickText(page, view, { within: 'main', settle: 1500 });
  await clickShown(page, '[aria-label="Hide the file tree"], [title="Hide the file tree"]');
  await sleep(600);
}

// The middle of the line holding text, in the source view or the editor.
const lineAt = (page, sel, text) => page.evaluate((sel, text) => {
  const l = [...document.querySelectorAll(sel)].find(e => e.textContent.includes(text));
  if (!l) return null;
  l.scrollIntoView({ block: 'center' });
  const b = l.getBoundingClientRect();
  return { x: b.left + 260, y: b.top + b.height / 2 };
}, sel, text);

async function clickLine(page, sel, text) {
  const at = await lineAt(page, sel, text);
  if (!at) throw new Error(`no line reads "${text}"`);
  await page.mouse.click(at.x, at.y);
  await sleep(400);
}

// A pomodoro-free clock running on the order history page, for the clock
// report's banner.
function clockIn(ctx) {
  const st = ctx.cli('clocks');
  if (!/No active clock/.test(st)) return;
  ctx.cli('clockin', '-hash', ctx.hashOf('Order history page'));
}

export default {
  demo,
  shots: [
    { name: 'cli-clocktable',
      cli: { title: 'orgs clocktable -block untilnow', args: ['clocktable', '-block', 'untilnow'], opts: { cols: 76, rows: 20 } },
      notes: [
        { find: 'Clock Report: untilnow', title: 'The range',
          text: 'org\'s <code>:block</code>: <code>today</code>, <code>thisweek</code>, <code>lastmonth</code>, <code>today-3</code>, <code>2026-W40</code>, <code>2026-Q3</code>… and the total of everything in it.' },
        { find: 'File: clients.org', title: 'By file',
          text: 'Each file\'s share, then its headings with the time clocked against each, from the <code>CLOCK:</code> lines in their <code>:LOGBOOK:</code> drawers.' },
        { find: 'Accessibility review', title: 'A heading',
          text: 'Several clock lines on one heading are added up. <code>-mermaid</code> prints the same entries as a gantt diagram of when the work happened.' },
      ] },
    { name: 'cli-code-run',
      cli: { title: 'orgs code run busiest', args: ['code', 'run', 'busiest'], opts: { cols: 76, rows: 10 } },
      notes: [
        { find: 'June ', title: 'What the block printed',
          text: 'A python block named <code>busiest</code> reading the table above it through <code>:var data=hours</code>: the table arrives as rows. <code>-w</code> writes it under the block as <code>#+RESULTS:</code>, as <code>C-c C-c</code> does.' },
        { title: 'Turned on in the yaml',
          text: 'Nothing runs unless <code>babel.enable</code> is set; <code>babel.languages</code> narrows it to the languages you trust.' },
      ] },
    {
      name: 'worg-clock-report', title: 'worg · Clock Report', setup: clockIn,
      worg: async (page, ctx) => {
        await ctx.openApp(page, 'ClockReport');
        await ctx.clickText(page, 'Today', { within: 'main', settle: 600 });
        await ctx.clickText(page, 'Last Week', { within: '[role=listbox]', settle: 2000 });
        await toTop(page);
        await tag(page, [
          { name: 'running', text: 'Clock out', within: 'main', closest: '.MuiAlert-root, .MuiSheet-root', thenUp: 0 },
          { name: 'block', text: 'Time Block', within: 'main', up: 1 },
          { name: 'filters', text: 'Headline', within: 'main', upTo: 'Refresh' },
          { name: 'total', text: 'Total:', contains: true, within: 'main', up: 1 },
          { name: 'table', selector: 'main table' },
        ]);
        return mainClip(page, { bottom: await page.evaluate(() => document.querySelector('main table').getBoundingClientRect().bottom + 28) });
      },
      notes: [
        { at: at('running'), title: 'The running clock',
          text: 'What you are clocked into, for how long, and a button to clock out. Clock in from a heading in Search, the agenda, or <code>orgs clockin</code>.' },
        { at: at('block'), badge: 'bl', title: 'The range',
          text: 'Today, yesterday, this or last week or month: org\'s clock table <code>:block</code>.' },
        { at: at('filters'), badge: 'tr', title: 'Narrowing it',
          text: 'By heading, by file, and by outline level, as <code>:maxlevel</code> does.' },
        { at: at('total'), badge: 'r', ring: false, title: 'The total',
          text: 'Every <code>CLOCK:</code> line in the range, added up.' },
        { at: at('table'), badge: 'tl', ring: false, title: 'Each heading',
          text: 'With its file and the time clocked against it. Download PDF prints the report.' },
      ],
    },
    {
      name: 'worg-editor', title: 'worg · Files · editing work.org', page: { height: 980 },
      worg: async (page, ctx) => {
        await openFile(page, ctx, 'work.org');
        await ctx.clickText(page, 'Edit', { within: 'main', settle: 2500 });
        // Fold the first task's drawers, as za would, so the gutter shows both.
        for (const t of [':PROPERTIES:', ':LOGBOOK:']) {
          await page.evaluate(t => {
            const lines = [...document.querySelectorAll('.cm-line')];
            const i = lines.findIndex((l, k) => k > lines.findIndex(x => x.textContent.includes('Audit the current pages')) && l.textContent.includes(t));
            const b = lines[i].getBoundingClientRect();
            const marks = [...document.querySelectorAll('.cm-foldGutter .cm-gutterElement')];
            const m = marks.find(g => Math.abs(g.getBoundingClientRect().top - b.top) < 4);
            (m?.querySelector('span') || m)?.dispatchEvent(new MouseEvent('click', { bubbles: true }));
          }, t);
          await sleep(500);
        }
        await clickLine(page, '.cm-line', 'Build the component library');
        await page.evaluate(() => document.querySelector('.cm-scroller').scrollTop = 0);
        await sleep(500);
        await scrub(page);
        await tag(page, [
          { name: 'fold', selector: '.cm-foldGutter' },
          { name: 'folded', selector: '.cm-content .cm-foldPlaceholder', nth: 1 },
          { name: 'todo', text: 'Migrate the blog templates', within: '.cm-content' },
          { name: 'mode', text: 'NORMAL', within: 'main' },
          { name: 'write', text: 'Writing', within: 'main', closest: 'button' },
          { name: 'keys', text: 'Keys', within: 'main', closest: 'button' },
        ]);
        await sleep(800);
        await scrub(page);
        return mainClip(page);
      },
      notes: [
        { at: at('fold'), badge: 'tl', ring: false, title: 'Folding',
          text: 'Headings, drawers and blocks fold from the gutter, or with <code>TAB</code>/<code>S-TAB</code> as in org (<code>za</code>, <code>zM</code>, <code>zR</code> in vim).' },
        { at: at('folded'), badge: 'br', title: 'A folded drawer',
          text: 'Properties and the logbook stay out of the way until wanted.' },
        { at: at('todo'), badge: 'r', title: 'Org\'s own keys',
          text: '<code>S-←/→</code> cycles the keyword, <code>S-↑/↓</code> the priority, <code>M-↑/↓</code> moves the subtree, <code>C-c C-s</code> schedules. Statistics cookies and checkboxes keep count.' },
        { at: at('mode'), badge: 'tl', title: 'Vim or Emacs',
          text: 'Vim by default (the mode shows here), or an Emacs keymap from the settings menu.' },
        { at: at('write'), badge: 'tr', title: 'Writing mode',
          text: 'The same file as a page to write on: goals, prose checks, the compendium.' },
        { at: at('keys'), badge: 'tr', title: 'Every key',
          text: 'F1 or <code>SPC ?</code> opens the editor\'s help.' },
      ],
    },
    {
      name: 'worg-inline-images', title: 'worg · Files · clients.org', page: { height: 900 },
      worg: async (page, ctx) => {
        await openFile(page, ctx, 'clients.org');
        await scrub(page);
        await tag(page, [
          { name: 'attr', text: ':width 520', contains: true, within: 'main' },
          { name: 'pic', selector: 'main img[src*="checkout"]' },
          { name: 'all', selector: '[aria-label="Hide pictures"], [title="Hide pictures"]' },
          { name: 'drawer', text: ':LOGBOOK:', contains: true, within: 'main' },
        ]);
        await sleep(800);
        await scrub(page);
        return mainClip(page);
      },
      notes: [
        { at: at('attr'), badge: 'tr', title: 'Its width',
          text: '<code>#+ATTR_ORG: :width</code> (or <code>#+ATTR_HTML:</code>) sizes it, as in Emacs; <code>#+STARTUP: inlineimages</code> shows a file\'s pictures when it opens.' },
        { at: at('pic'), badge: 'tr', title: 'The picture, under its link',
          text: 'Any image a link points at, relative to the file: org\'s <code>org-display-inline-images</code>. Click it to see it full size.' },
        { at: at('all'), badge: 'bl', title: 'All on or off',
          text: 'For the whole file; an eye at the end of a link\'s line does one.' },
        { at: at('drawer'), badge: 'tr', title: 'Drawers fold',
          text: 'Logbooks and property drawers start shut in the Org view.' },
      ],
    },
    {
      name: 'worg-links-panel', title: 'worg · Files · the links panel', page: { height: 820 },
      worg: async (page, ctx) => {
        await openFile(page, ctx, 'notes.org');
        await clickShown(page, '[aria-label="Backlinks and mentions"], [title="Backlinks and mentions"]');
        await clickLine(page, '[data-line]', 'Design system');
        await sleep(2000);
        await scrub(page);
        await tag(page, [
          { name: 'scope', text: 'This heading', within: 'main', up: 1 },
          { name: 'from', text: 'Linked from', contains: true, within: 'main', up: 1 },
          { name: 'out', text: 'Links out', contains: true, within: 'main', up: 1 },
          { name: 'mention', text: 'Unlinked mentions', contains: true, within: 'main', up: 1 },
        ]);
        await sleep(800);
        await scrub(page);
        return mainClip(page);
      },
      notes: [
        { at: at('scope'), badge: 'tl', title: 'This heading or the whole file',
          text: 'It follows the reader: the heading at the top of the page, or the one clicked.' },
        { at: at('from'), badge: 'tl', title: 'Backlinks',
          text: 'Every link to here, from any file, with the line it is on. Click one to go there.' },
        { at: at('out'), badge: 'tl', title: 'Links out',
          text: 'What this heading links to: files, headings (<code>*Title</code>, <code>id:</code>, <code>::CUSTOM_ID</code>), urls.' },
        { at: at('mention'), badge: 'tl', title: 'Unlinked mentions',
          text: 'Where the heading\'s title is written but not linked; Link turns one into a link, Link all does them all.' },
      ],
    },
    {
      name: 'worg-palette', title: 'worg · the command palette', page: { height: 620 },
      worg: async (page, ctx) => {
        await ctx.openApp(page, 'Agenda');
        await page.keyboard.down('Meta'); await page.keyboard.press('k'); await page.keyboard.up('Meta');
        await sleep(800);
        await page.keyboard.type('design', { delay: 80 });
        await sleep(1800);
        await tag(page, [
          { name: 'sheet', selector: '.MuiModal-root .MuiSheet-variantOutlined' },
          { name: 'input', selector: '.MuiModal-root input' },
          { name: 'head', text: 'Design system', within: '.MuiModal-root', up: 1 },
          { name: 'keys', text: 'esc close', contains: true, within: '.MuiModal-root', up: 1 },
        ]);
        return clipAround(page, ['sheet'], 40);
      },
      notes: [
        { at: at('input'), badge: 'r', ring: false, title: '⌘K anywhere',
          text: 'Or ctrl-K, or ctrl-shift-P. Type part of a name: <code>mm</code> finds Mind Map.' },
        { at: at('head'), badge: 'r', title: 'Headings, as you type',
          text: 'Any heading in any file, searched on the server; Enter opens it in the Files tab.' },
        { at: at('keys'), badge: 'r', ring: false, title: 'And everything else',
          text: 'Tabs, capture templates, files, saved searches, boards, collections and settings, the ones you use most first.' },
      ],
    },
    {
      name: 'worg-refile', title: 'worg · Files · refiling', page: { height: 900 },
      worg: async (page, ctx) => {
        await openFile(page, ctx, 'inbox.org');
        await clickShown(page, '[aria-label="Move headings"], [title="Move headings"]');
        await sleep(1000);
        const boxes = await page.$$('[role=dialog] input[type=checkbox]');
        await boxes[0].click(); await sleep(300);
        await boxes[3].click(); await sleep(300);
        await ctx.clickText(page, 'Refile', { within: '[role=dialog]', settle: 1500 });
        await page.keyboard.type('errands', { delay: 60 });
        await sleep(1500);
        await tag(page, [
          { name: 'dlg', text: 'Refile 2 headings', within: '[role=dialog]', closest: '[role=dialog]' },
          { name: 'what', text: 'Look into a standing desk', within: '[role=dialog]', last: true, upTo: 'Call the plumber' },
          { name: 'find', selector: '[role=dialog] input[type=text], [role=dialog] input:not([type])', last: true },
          { name: 'target', text: 'Errands', within: '[role=dialog]', last: true, up: 1 },
        ]);
        return clipAround(page, ['dlg'], 30);
      },
      notes: [
        { at: at('what'), badge: 'tr', title: 'What moves',
          text: 'The headings ticked in the file, each with everything under it (<code>C-c C-w</code> for several at once). Copy to leaves them where they are; Archive sends them to the archive file.' },
        { at: at('find'), badge: 'r', ring: false, title: 'Where to',
          text: 'Any heading in any file, or the end of a file, found by typing: org\'s refile targets.' },
        { at: at('target'), badge: 'r', title: 'The target',
          text: 'A heading as its outline path. <em>Make it if it is not there</em> creates the path first. <code>orgs refile</code> does the same from the terminal.' },
      ],
    },
    {
      name: 'worg-chart', title: 'worg · Tables · a #+PLOT: line', page: { width: 1600, height: 960 },
      worg: async (page, ctx) => {
        await ctx.openApp(page, 'Tables');
        await ctx.clickText(page, 'hours', { within: 'main', settle: 1500 });
        await ctx.clickText(page, 'July', { within: 'main table', settle: 500 });
        await page.keyboard.down('Control'); await page.keyboard.press('g'); await page.keyboard.up('Control');
        await sleep(3000);
        await tag(page, [
          { name: 'dlg', selector: '[role=dialog]', last: true },
          { name: 'title', text: 'Hours by client', within: '[role=dialog]' },
          { name: 'type', text: 'Bars', within: '[role=dialog]', closest: 'button' },
          { name: 'way', text: 'Rows', within: '[role=dialog]', upTo: 'Columns' },
          { name: 'legend', text: 'Northwind', within: '[role=dialog]', last: true, up: 2 },
        ]);
        return clipAround(page, ['dlg'], 16);
      },
      notes: [
        { at: at('title'), badge: 'tr', title: 'From the table\'s #+PLOT: line',
          text: '<code>#+PLOT: title:"Hours by client" ind:1 deps:(2 3) with:histograms</code>: org-plot\'s options pick the axis, the series and the kind of chart. Exports draw the same line with gnuplot.' },
        { at: at('type'), badge: 'tl', title: 'Eighteen kinds',
          text: 'Lines, bars, pies, radar, heatmap, box plots… <code>with:</code> chooses the first.' },
        { at: at('way'), badge: 'tr', ring: false, title: 'Rows or columns',
          text: 'Without a <code>#+PLOT:</code> line, select cells and press ctrl-G to chart the selection, either way round.' },
        { at: at('legend'), badge: 'tl', title: 'The series',
          text: 'Named by the header row. Click one to hide it; the save button files the chart under the table.' },
      ],
    },
    {
      name: 'worg-code-run', title: 'worg · Code · running a block', page: { height: 900 },
      worg: async (page, ctx) => {
        await ctx.openApp(page, 'Code');
        await ctx.clickText(page, 'busiest', { settle: 1500 });
        await ctx.clickText(page, 'Run', { within: 'main', settle: 4000 });
        await toTop(page);
        await tag(page, [
          { name: 'list', text: 'busiest', within: 'main', upTo: '3L' },
          { name: 'takes', text: 'Takes', contains: true, within: 'main', up: 1 },
          { name: 'run', text: 'Run', within: 'main', closest: 'button' },
          { name: 'out', text: 'September  Contoso', contains: true, within: 'main', closest: 'pre, .MuiSheet-root' },
        ]);
        return mainClip(page);
      },
      notes: [
        { at: at('list'), badge: 'tr', title: 'Every block',
          text: 'All the source blocks in your notes, by language or by file, named ones by their <code>#+NAME:</code>.' },
        { at: at('takes'), badge: 'tl', title: 'Its variables, resolved',
          text: '<code>:var data=hours</code> is a table further up the file, and this says so; a name that points at nothing is a warning, before you run it.' },
        { at: at('run'), badge: 'bl', title: 'Run it',
          text: 'As <code>C-c C-c</code>: the server runs it (when <code>babel.enable</code> is set) and the pencil writes the result under the block.' },
        { at: at('out'), badge: 'tr', title: 'What it printed',
          text: '<code>:results output</code>; <code>:results value</code>, tables and files come back as org would write them.' },
      ],
    },
    {
      name: 'worg-starmap', title: 'worg · Starmap', page: { height: 900 },
      worg: async (page, ctx) => {
        await ctx.openApp(page, 'Starmap');
        await sleep(3000);
        await tag(page, [
          { name: 'find', selector: 'main input[placeholder^="find a star"]' },
          { name: 'chips', text: 'threads', contains: true, within: 'main', up: 1 },
          { name: 'time', text: 'now', contains: true, within: 'main', last: true, up: 1 },
        ]);
        return mainClip(page, { top: await page.evaluate(() => document.querySelector('main input[placeholder^="find a star"]').getBoundingClientRect().top - 24) });
      },
      notes: [
        { title: 'Every heading a star',
          text: 'Files are constellations, links are the threads between stars. Drag to turn it, scroll to zoom, click a star to read the heading.' },
        { at: at('find'), badge: 'r', ring: false, title: 'Find a star',
          text: 'By title; the view flies to it.' },
        { at: at('time'), badge: 'tr', title: 'Through time',
          text: 'Play your notes growing, from the first heading written to now.' },
      ],
    },
    {
      name: 'worg-collections', title: 'worg · Collections', page: { height: 820 },
      worg: async (page, ctx) => {
        await ctx.openApp(page, 'Collections');
        // A Joy chip answers on its action button, not its label.
        await page.evaluate(() => {
          const label = [...document.querySelectorAll('main .MuiChip-label')].find(e => e.textContent.trim().startsWith('🍷') || e.textContent.includes('Wine'));
          const chip = label.closest('.MuiChip-root');
          (chip.querySelector('.MuiChip-action, button') || chip).click();
        });
        await sleep(1500);
        await ctx.clickText(page, 'Tenuta delle Terre Nere Etna Rosso', { within: 'main', settle: 1500 });
        await toTop(page);
        await tag(page, [
          { name: 'pick', text: 'Wine', within: 'main', upTo: 'Contact' },
          { name: 'list', text: 'Cloudy Bay Sauvignon Blanc', within: 'main', upTo: 'Quinta do Crasto' },
          { name: 'card', text: 'Tenuta delle Terre Nere', within: 'main', last: true, upTo: 'Sicily' },
        ]);
        return mainClip(page);
      },
      notes: [
        { at: at('pick'), badge: 'tr', title: 'The collections',
          text: 'Every <code>RECORD</code> type in your notes. A heading with <code>:COLLECTION: wine</code> gives one its name, its icon and a place for new records.' },
        { at: at('list'), badge: 'tr', title: 'The records',
          text: 'Each a heading with <code>:RECORD: wine</code>, wherever it lives, with a search over them.' },
        { at: at('card'), badge: 'tr', title: 'One record',
          text: 'Its property drawer as fields and the heading\'s text as notes, with when it was added. Edit writes the changes back to the drawer and notes them in its <code>LOGBOOK</code>.' },
      ],
    },
  ],
};

