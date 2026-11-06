// The pictures of the second set of more optional packages, for
// docs/note-packages.org: places on a map, timelines, the library, hover
// previews, link titles, quick maths typing and keeping web pictures here.
// The demo notebook with a few files of its own on top (docs/screenshots/
// notes2): a trip to Lisbon with every place's coordinates, a history
// course and a kitchen renovation for the timelines, a reading list with its
// covers, the allotment's notes, a page of Fourier series.
//
//   SHOT_PORT=8143 node capture.mjs -story notes2
//   SHOT_PORT=8143 node capture.mjs -story notes2 worg-map
//
// Link titles and web pictures need web pages: a small site on this machine
// (SITE_PORT, 80 by default so the addresses read without a port) answers
// for crumb.localhost, grainfield.localhost, millstone.localhost and
// photos.localhost, and the server is told it may fetch private addresses.
// Map tiles come from OpenStreetMap when the network is there; without it
// the map is drawn on a grid of degrees.

import fs from 'node:fs';
import http from 'node:http';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { at, tag, clipAround, mainClip, toTop, clickShown } from './engineer-worg.mjs';

const here = path.dirname(fileURLToPath(import.meta.url));
const shots = path.resolve(here, '../../../docs/screenshots');
const notebook = path.join(shots, 'notes2');
const webDir = path.join(shots, 'notes2-web');

const SITE_PORT = Number(process.env.SITE_PORT || 80);
const host = name => `${name}.localhost${SITE_PORT === 80 ? '' : ':' + SITE_PORT}`;
const url = (name, p) => `http://${host(name)}${p}`;

// ---------------------------------------------------------------- the site

const PAGES = {
  'crumb/guides/sourdough-hydration': ['Understanding hydration in sourdough | Crumb', 'Crumb',
    'What 70%, 75% and 80% water do to a dough, and how to tell which your flour wants.'],
  'crumb/recipes/country-loaf': ['A country loaf, step by step | Crumb', 'Crumb',
    'A 75% hydration loaf with a little whole wheat, over two days.'],
  'crumb/guides/shaping-a-batard': ['Shaping a bâtard without tearing the skin | Crumb', 'Crumb',
    'Pre-shape, rest, then build tension in three folds.'],
  'grainfield/articles/why-rye-ferments-faster': ['Why rye ferments faster than wheat - Grainfield', 'Grainfield',
    'Amylase, pentosans and the sour of a good rye starter.'],
  'millstone/flour/protein-and-strength': ['Flour, protein and strength: a miller\'s guide — Millstone', 'Millstone',
    'Reading the protein figure on a bag of flour.'],
};

function page(title, site, desc) {
  return `<!doctype html><html><head><meta charset="utf-8"><title>${title}</title>
<meta property="og:site_name" content="${site}"><meta name="description" content="${desc}">
</head><body><h1>${title}</h1><p>${desc}</p></body></html>`;
}

let site = null;
function startSite() {
  if (site) return;
  site = http.createServer((req, res) => {
    const name = (req.headers.host || '').split('.')[0];
    const p = decodeURIComponent((req.url || '/').split('?')[0]);
    if (name === 'photos' && p.startsWith('/lisbon/')) {
      const f = path.join(webDir, 'photos', path.basename(p));
      if (fs.existsSync(f)) {
        const body = fs.readFileSync(f);
        res.writeHead(200, { 'Content-Type': 'image/png', 'Content-Length': body.length });
        res.end(req.method === 'HEAD' ? undefined : body);
        return;
      }
    }
    const pg = PAGES[name + p];
    if (pg) {
      res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' });
      res.end(page(...pg));
      return;
    }
    res.writeHead(404, { 'Content-Type': 'text/plain' });
    res.end('not here');
  });
  site.listen(SITE_PORT);
  site.unref();
}

// ------------------------------------------------------------ the notebook

// Files that name the site, written here so the addresses carry its port.
const BREAD = `#+TITLE: Bread

* Sourdough notes
  Starter fed 1:5:5 the night before; the dough at 75% hydration, bulk
  until it has risen by half.

  Reading for the weekend:
  - ${url('crumb', '/guides/sourdough-hydration')}
  - ${url('grainfield', '/articles/why-rye-ferments-faster')}
  - [[${url('millstone', '/flour/protein-and-strength')}]]

  The recipe I keep coming back to: ${url('crumb', '/recipes/country-loaf')}

* Bakes
** Saturday loaf
   Overproofed again: the kitchen was 24°. Shorter bulk next time, or the
   fridge for the last hour.

   Shaping help:
`;

const JOURNAL = `#+TITLE: Lisbon journal

* Day two: Belém
  Took the 15 tram out along the river. The queue at the bakery was round
  the corner and moved in ten minutes.

  #+CAPTION: Pastéis, still warm
  #+ATTR_HTML: :width 420
  [[${url('photos', '/lisbon/pastel-de-nata.png')}]]

  We walked back along the river at dusk and caught
  [[${url('photos', '/lisbon/tagus-sunset.png')}][the bridge at sunset]].

* Day three: Alfama
  Up the hill on the 28, down again on foot.

  ${url('photos', '/lisbon/tram-28.png')}

  Tiles on every second house:
  [[${url('photos', '/lisbon/azulejos.png')}]]
`;

const covers = Object.fromEntries(fs.readdirSync(path.join(notebook, 'images/book'))
  .filter(f => f.endsWith('.png'))
  .map(f => [`org/images/book/${f}`, fs.readFileSync(path.join(notebook, 'images/book', f))]));

const server = `  features:
    mapView: true
    timeline: true
    mediaDb: true
    hoverEditor: true
    autoLinkTitle: true
    latexSuite: true
    localImages: true
  mapView:
    autoGeocode: false
  hoverEditor:
    privateHosts: true
  autoLinkTitle:
    allowPrivate: true
  localImages:
    allowPrivate: true
`;

export const demo = {
  src: [path.join(shots, 'demo'), notebook],
  files: { 'org/bread.org': BREAD, 'org/lisbon-journal.org': JOURNAL, ...covers },
  server,
};

// ------------------------------------------------------------------ helpers

const sleep = ms => new Promise(r => setTimeout(r, ms));

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
  await ctx.clickText(page, view, { within: 'main', settle: 1800 });
  await clickShown(page, '[aria-label="Hide the file tree"], [title="Hide the file tree"]');
  await sleep(600);
}


// The rendered view in the exporter's light theme (worg's default is its
// dark one), so the pages match the rest of the light pictures.
async function lightPages(page) {
  if (page.__light) return;
  page.__light = true;
  await page.evaluateOnNewDocument(() => {
    try {
      const k = 'worg.settings';
      const s = JSON.parse(localStorage.getItem(k) || '{}');
      s.htmlTheme = 'default';
      localStorage.setItem(k, JSON.stringify(s));
    } catch { }
  });
}

// The editor on a file, in vim's normal mode.
async function openEditor(page, ctx, file) {
  await openFile(page, ctx, file, 'Org');
  await ctx.clickText(page, 'Edit', { within: 'main', settle: 2500 });
}

// The middle of the line holding text, in the source view or the editor.
const lineAt = (page, sel, text) => page.evaluate((sel, text) => {
  const l = [...document.querySelectorAll(sel)].find(e => e.textContent.includes(text));
  if (!l) return null;
  l.scrollIntoView({ block: 'center' });
  const b = l.getBoundingClientRect();
  return { x: b.left + 40, y: b.top + b.height / 2 };
}, sel, text);

async function clickLine(page, sel, text) {
  const p = await lineAt(page, sel, text);
  if (!p) throw new Error(`no line reads "${text}"`);
  await page.mouse.click(p.x, p.y);
  await sleep(400);
}

// Where a piece of text is inside the rendered page's iframe, in the page's
// coordinates, for the mouse.
async function inFrame(page, text) {
  for (const f of page.frames()) {
    if (f === page.mainFrame()) continue;
    const r = await f.evaluate(text => {
      const el = [...document.querySelectorAll('a, span')].find(e => e.textContent.trim() === text);
      if (!el) return null;
      el.scrollIntoView({ block: 'center' });
      const b = el.getBoundingClientRect();
      return { x: b.left + b.width / 2, y: b.top + b.height / 2 };
    }, text).catch(() => null);
    if (!r) continue;
    const off = await (await f.frameElement()).boundingBox();
    return { x: off.x + r.x, y: off.y + r.y };
  }
  throw new Error(`no "${text}" in the rendered page`);
}

// Where an element of the rendered page's iframe ends, in the page's
// coordinates, for a clip.
async function frameBottom(page, selector) {
  for (const f of page.frames()) {
    if (f === page.mainFrame()) continue;
    const r = await f.evaluate(sel => {
      const el = document.querySelector(sel);
      return el ? el.getBoundingClientRect().bottom : null;
    }, selector).catch(() => null);
    if (r == null) continue;
    const off = await (await f.frameElement()).boundingBox();
    return off.y + r;
  }
  throw new Error(`no ${selector} in the rendered page`);
}

const edge = (page, name, side = 'top') => page.evaluate((n, s) =>
  document.querySelector(`[data-shot="${n}"]`).getBoundingClientRect()[s], name, side);

export default {
  demo,
  shots: [
    // ------------------------------------------------------------- the map
    {
      name: 'worg-map', title: 'worg · Map', page: { width: 1440, height: 960 },
      worg: async (page, ctx) => {
        await ctx.openApp(page, 'Map');
        await sleep(2000);
        await ctx.clickText(page, 'By tag', { within: 'main', settle: 1200 });
        // A pin's card, opened from the pin so the map stays where it is.
        await page.evaluate(() => {
          const pins = [...document.querySelectorAll('.worg-pin')];
          const r = el => el.getBoundingClientRect();
          // The pin nearest the middle of the map, for a card with room round it.
          const box = r(document.querySelector('.worg-map'));
          const mid = { x: box.left + box.width * 0.45, y: box.top + box.height * 0.45 };
          pins.sort((a, b) => Math.hypot(r(a).x - mid.x, r(a).y - mid.y) - Math.hypot(r(b).x - mid.x, r(b).y - mid.y));
          pins[0].click();
        });
        await sleep(2500);
        await toTop(page);
        // The list's foot, where the places not on the map are.
        await page.evaluate(() => { const l = document.querySelector('main .MuiList-root'); l.scrollTop = l.scrollHeight; });
        await sleep(400);
        await debugShot(page, 'map');
        await tag(page, [
          { name: 'query', selector: 'main input[placeholder^="Every place"]' },
          { name: 'colour', text: 'By tag', within: 'main', closest: '.MuiToggleButtonGroup-root, [role=group]' },
          { name: 'legend', text: 'sight 6', within: 'main', up: 2 },
          { name: 'card', selector: '.leaflet-popup-content-wrapper' },
          { name: 'cluster', selector: '.worg-cluster' },
          { name: 'off', text: 'Not on the map (1)', within: 'main', closest: 'li' },
        ]);
        await debugShot(page, 'map');
        return mainClip(page, { top: await edge(page, 'query') - 20 });
      },
      notes: [
        { at: at('query'), badge: 'tr', ring: false, title: 'Every place, or a query',
          text: 'Search\'s queries: <code>HasTags("food")</code>, <code>IsTask()</code> keep only those headings. The copy button gives a <code>#+MAP:</code> line for this view.' },
        { at: at('legend'), badge: 'tr', title: 'Coloured by tag',
          text: 'Or by keyword (to do, done, none). A place is the heading\'s <code>:LOCATION:</code>: <code>38.7108, -9.1424</code>, a <code>geo:</code> uri, an OpenStreetMap or Google Maps link, or an address.' },
        { at: at('card'), badge: 'tr', title: 'A pin\'s card',
          text: 'Title, keyword, tags and the start of its text, with Open, Move pin and Edit place…, which write the <code>:LOCATION:</code> back.' },
        { at: at('cluster'), badge: 'br', ring: false, title: 'Pins close together',
          text: 'Drawn as one counted circle; click it to zoom in. Tiles are OpenStreetMap\'s, dark ones in dark mode.' },
        { at: at('off'), badge: 'tr', title: 'Not on the map',
          text: 'Addresses not looked up yet, here a contact\'s <code>:ADDRESS:</code>. Find them asks Nominatim, one a second, and keeps the answers; Place puts one on by a click.' },
      ],
    },
    {
      name: 'worg-map-page', title: 'worg · Files · lisbon.org', page: { width: 1440, height: 1000 },
      worg: async (page, ctx) => {
        await lightPages(page);
        await openFile(page, ctx, 'lisbon.org', 'Rendered');
        await sleep(3500);
        await scrub(page);
        await tag(page, [{ name: 'frame', selector: 'main iframe' }]);
        return mainClip(page, { bottom: await frameBottom(page, 'figure.org-map') + 24 });
      },
      notes: [
        { title: 'A #+MAP: line',
          text: '<code>#+MAP: title:"Lisbon" height:380 colour:tag</code> draws the map where it stands. With no <code>query:</code> it shows the places of the subtree it is under - here, above the first heading, the whole file.' },
        { title: 'Live in worg',
          text: 'The rendered view puts the live map in its place: drag, zoom, click a pin for its card. An html export has a picture of OpenStreetMap\'s tiles with numbered pins, each a link.' },
        { title: 'The places, numbered',
          text: 'Under the map, matching its pins. Plain text, LaTeX and Typst exports write only the list, each place linked to openstreetmap.org.' },
      ],
    },
    { name: 'cli-map', cli: { title: 'orgs map', args: ['map'], opts: { cols: 108, rows: 16 } },
      notes: [
        { find: '38.7114,-9.1296', title: 'Where each is',
          text: 'Read from <code>:LOCATION:</code>, <code>:LATITUDE:</code>/<code>:LONGITUDE:</code> or a contact\'s <code>:ADDRESS…:</code>, with the file and line and how it was found.' },
        { find: 'not looked up', title: 'An address',
          text: 'Words wait to be looked up: <code>orgs map geocode -all</code>, or <code>-write</code> to keep the answer as <code>:GEO:</code> beside it.' },
        { title: 'For anything else',
          text: '<code>orgs map -placed -format geojson</code> writes a FeatureCollection; <code>orgs map set Market \'Granville Island, Vancouver\' -geo</code> gives a heading its place.' },
      ] },

    // ------------------------------------------------------------ timeline
    {
      name: 'worg-timeline', title: 'worg · Timeline', page: { width: 1600, height: 960 },
      worg: async (page, ctx) => {
        await ctx.openApp(page, 'Timeline');
        await ctx.clickText(page, 'Every dated heading', { within: 'main', settle: 700 });
        await page.evaluate(() => [...document.querySelectorAll('[role=listbox] [role=option]')]
          .find(e => e.textContent.includes('history.org')).click());
        await sleep(2500);
        const p = await page.evaluate(() => {
          const t = [...document.querySelectorAll('main svg text')].find(e => e.textContent.trim() === 'Alfred the Great');
          const b = t.getBoundingClientRect();
          return { x: b.left - 9, y: b.top + b.height / 2 };
        });
        await page.mouse.move(p.x, p.y);
        await sleep(1200);
        await tag(page, [
          { name: 'source', selector: 'main .MuiSelect-root' },
          { name: 'era', text: 'Anglo-Saxon England', within: 'main', closest: 'g' },
          { name: 'hollow', text: 'Sutton Hoo ship burial', within: 'main', closest: 'g' },
          { name: 'card', selector: '.orgs-timeline-view > div[style*="z-index: 5"]' },
          { name: 'jump', selector: 'main input[placeholder^="Jump to"]' },
        ]);
        await debugShot(page, 'timeline');
        return mainClip(page, { top: await edge(page, 'source') - 20 });
      },
      notes: [
        { at: at('source'), badge: 'tl', ring: false, title: 'What to draw',
          text: 'Every dated heading, a query, or a <code>#+TIMELINE:</code> line from the files: here <code>group:tag from:"80 BC" to:1700</code> in history.org, one lane per tag.' },
        { at: at('era'), badge: 'tr', ring: false, title: 'Eras',
          text: 'A span with <code>:TIMELINE: era</code> or the tag <code>:era:</code>: a band across the top and a faint wash down every lane.' },
        { at: at('hollow'), badge: 'tr', ring: false, title: 'A historian\'s dates',
          text: '<code>:DATE: c. 625</code> is drawn hollow. <code>55 BC</code>, <code>1066-10-14</code>, <code>1337 -- 1453</code>, <code>1950s</code> and <code>12th century</code> all read.' },
        { at: at('card'), badge: 'tr', ring: false, title: 'Hover an event',
          text: 'Its date, how long it lasted, its lane and tags and first lines; click to open the heading.' },
        { at: at('jump'), badge: 'tr', title: 'Pan, zoom, jump',
          text: 'Drag and wheel from a million years down to twenty minutes; Jump to… takes any date <code>:DATE:</code> does. The strip below is the whole timeline.' },
      ],
    },
    { name: 'cli-timeline', cli: { title: 'orgs timeline -file kitchen.org -group parent', args: ['timeline', '-file', 'kitchen.org', '-group', 'parent'], opts: { cols: 104, rows: 21 } },
      notes: [
        { find: 'Electrics and plumbing', title: 'Spans',
          text: '<code>SCHEDULED:</code> to a later <code>DEADLINE:</code>, a range or a <code>:DATE:</code> span, with a bar for how long and the number of days.' },
        { find: 'Units arrive', title: 'Points',
          text: 'An active timestamp, <code>CLOSED:</code>, a lone <code>SCHEDULED:</code>, <code>:CREATED:</code> or a datetree day.' },
        { find: 'Measure up and draw', title: 'Lanes',
          text: '<code>-group parent</code>: the heading above. Or <code>tag</code>, <code>file</code>, <code>keyword</code>, <code>property:NAME</code>.' },
        { title: 'The same as the line',
          text: '<code>orgs timeline group:tag from:-500</code> takes a <code>#+TIMELINE:</code> line\'s words; <code>svg -out t.svg</code> writes the exported picture.' },
      ] },

    // ------------------------------------------------------------- library
    {
      name: 'worg-library', title: 'worg · Library', page: { width: 1440, height: 1500 },
      worg: async (page, ctx) => {
        await ctx.openApp(page, 'Library');
        await sleep(1500);
        await tag(page, [
          { name: 'search', selector: 'main input[placeholder^="Find a book"]' },
          { name: 'shelf', text: 'Up next', within: 'main', nth: 0, closest: '.MuiChip-root, button' },
          { name: 'cover', selector: 'main img[src*="dune"]' },
          { name: 'genre', text: 'science fiction', within: 'main', nth: 0 },
          { name: 'sort', text: 'Recently added', within: 'main' },
          { name: 'aside', text: 'Set aside', within: 'main', last: true },
        ]);
        return mainClip(page, { top: await edge(page, 'search') - 34, bottom: await edge(page, 'aside') - 14 });
      },
      notes: [
        { at: at('search'), badge: 'tl', ring: false, title: 'Look one up',
          text: 'A title, an author or an ISBN, in Open Library (books), Wikidata (films, shows, games), MusicBrainz (albums); pick the shelf, stars and where it goes, then add it.' },
        { at: at('shelf'), badge: 'tr', title: 'Shelves',
          text: 'From <code>:STATUS:</code> (to read, reading, read…), else the TODO keyword, else <code>STARTED_DATE</code> and <code>FINISHED_DATE</code>.' },
        { at: at('cover'), badge: 'tr', title: 'A book',
          text: 'A heading with <code>:RECORD: book</code>; <code>:IMAGE:</code> is its cover in <code>images/book/</code>. The stars are <code>:RATING:</code>, clickable on every cover.' },
        { at: at('genre'), badge: 'bl', title: 'Genres are tags',
          text: 'Filter by one, or by words.' },
        { at: at('sort'), badge: 'tl', title: 'In order',
          text: 'Of adding, title, author, year or rating. Click a cover to move it, rate it or fill its details in again.' },
      ],
    },
    { name: 'cli-library', cli: { title: 'orgs library ls', args: ['library', 'ls'], opts: { cols: 100, rows: 13 } },
      notes: [
        { find: 'Piranesi', title: 'What you have',
          text: 'Every record of every kind, or one: <code>orgs library ls book -shelf doing</code>.' },
        { find: '★★★★½', title: 'Shelf and stars',
          text: 'In the kind\'s own words (read, watched, played). <code>orgs library set dune -status read -rating 4.5</code>, or <code>finish</code>, <code>rate dune 4</code>.' },
        { title: 'Adding one',
          text: '<code>orgs library add book 9780441013593 -status reading</code>; with several hits it asks, and under a pipe says how to choose.' },
      ] },

    // ------------------------------------------------------- hover previews
    {
      name: 'worg-hover-preview', title: 'worg · Files · garden.org', page: { width: 1440, height: 820 },
      worg: async (page, ctx) => {
        await lightPages(page);
        await openFile(page, ctx, 'garden.org', 'Rendered');
        await sleep(1500);
        const p = await inFrame(page, 'the compost from last autumn');
        await page.mouse.move(p.x, p.y);
        await sleep(2200);
        await clickShown(page, '[aria-label="Pin: keep it open"]');
        const q = await page.evaluate(() => {
          const el = [...document.querySelectorAll('[data-hp-card] a, [data-hp-card] span')].find(e => e.textContent.trim() === 'the March soil test');
          const b = el.getBoundingClientRect();
          return { x: b.left + b.width / 2, y: b.top + b.height / 2 };
        });
        await page.mouse.move(q.x, q.y);
        await sleep(2500);
        await scrub(page);
        await tag(page, [
          { name: 'first', selector: '[data-hp-card]', nth: 0 },
          { name: 'second', selector: '[data-hp-card]', last: true },
          { name: 'pin', text: 'Compost heap', within: '[data-hp-card]' },
          { name: 'edit', selector: '[aria-label="Edit here"]', last: true },
        ]);
        await debugShot(page, 'hover');
        return mainClip(page);
      },
      notes: [
        { at: at('first'), badge: 'bl', ring: false, title: 'Rest on a link',
          text: 'Its card shows what it points at: here <code>[[id:4f7c…][the compost from last autumn]]</code>. <code>[[*Heading]]</code>, <code>file:x.org::#id</code>, pictures, pdfs and web pages work too.' },
        { at: at('pin'), badge: 'tl', ring: false, title: 'Pinned',
          text: 'Pinned, a card stays when the pointer leaves; drag it by its header (which pins it too) and size it by the corner.' },
        { at: at('second'), badge: 'tr', ring: false, title: 'A card from a card',
          text: 'A link inside a card opens another on top: <code>[[*Soil test, March]]</code>, its table drawn. Esc closes the top one.' },
        { at: at('edit'), badge: 'bl', title: 'Edit in place',
          text: 'The card becomes an editor of the heading\'s own lines; ⌘S writes only those, and refuses if the file changed since.' },
      ],
    },
    { name: 'cli-peek', cli: { title: 'orgs peek id:4f7c2a90-compost-heap', args: ['peek', 'id:4f7c2a90-compost-heap'], opts: { cols: 100, rows: 14 } },
      notes: [
        { find: '~/notes/garden.org:22', title: 'Where it is',
          text: 'Any link org writes: <code>id:</code>, <code>\'[[*Soil]]\' -from garden.org</code>, <code>file:notes.org::#beds</code>, a url (its title and description).' },
        { find: 'Two bays', title: 'What it says',
          text: 'Coloured as <code>orgs show</code> colours it; <code>-lines 10</code> for less, <code>-edit</code> to open it in <code>$EDITOR</code> at its line.' },
      ] },

    // --------------------------------------------------------- link titles
    {
      name: 'worg-link-title', title: 'worg · Files · editing bread.org', page: { width: 1440, height: 720 }, setup: startSite,
      worg: async (page, ctx) => {
        await openEditor(page, ctx, 'bread.org');
        await clickLine(page, '.cm-line', 'Shaping help:');
        await page.keyboard.press('A');
        await page.keyboard.press('Enter');
        await sleep(300);
        await page.evaluate(u => {
          const dt = new DataTransfer();
          dt.setData('text/plain', u);
          document.querySelector('.cm-content').dispatchEvent(new ClipboardEvent('paste', { clipboardData: dt, bubbles: true, cancelable: true }));
        }, url('crumb', '/guides/shaping-a-batard'));
        await sleep(3000);
        await scrub(page);
        await tag(page, [
          { name: 'pasted', text: 'Shaping a bâtard without tearing the skin', contains: true, within: '.cm-content', closest: '.cm-line' },
          { name: 'bare', text: '/recipes/country-loaf', contains: true, within: '.cm-content', closest: '.cm-line' },
          { name: 'button', selector: 'main [aria-label="Add titles to bare links"]', last: true },
        ]);
        return mainClip(page);
      },
      notes: [
        { at: at('pasted'), badge: 'tr', title: 'Paste a url on its own',
          text: 'It goes in as <code>[[url][Fetching title…]]</code> and the page\'s title takes the placeholder\'s place. One undo gives the plain url back; ctrl/cmd+shift+v pastes it as it is.' },
        { at: at('bare'), badge: 'tr', title: 'Urls already there',
          text: 'Bare urls and <code>[[url]]</code> links. Pasting over selected words makes <code>[[url][those words]]</code>, nothing fetched.' },
        { at: at('button'), badge: 'tl', title: 'Add titles to bare links',
          text: 'Every one in the buffer at once, as one undo step, saved when you save.' },
      ],
    },
    {
      name: 'worg-link-title-dialog', title: 'worg · Files · bread.org', page: { width: 1440, height: 860 }, setup: startSite,
      worg: async (page, ctx) => {
        await openFile(page, ctx, 'bread.org', 'Org');
        await clickShown(page, '[aria-label="Add titles to bare links"]');
        await sleep(3500);
        await tag(page, [
          { name: 'dlg', selector: '[role=dialog]', last: true },
          { name: 'title', selector: '[role=dialog] input:not([type=checkbox])', nth: 0 },
          { name: 'line', text: 'line 10', within: '[role=dialog]', closest: '.MuiChip-root' },
          { name: 'add', text: 'Add 4 titles', within: '[role=dialog]', closest: 'button' },
        ]);
        return clipAround(page, ['dlg'], 24);
      },
      notes: [
        { at: at('title'), badge: 'tr', title: 'The page\'s own title',
          text: 'Its <code>og:title</code> or <code>&lt;title&gt;</code>, with the site\'s name taken off (<code> | Crumb</code>, <code> - YouTube</code>, <code> · GitHub</code>). Change any, or untick it.' },
        { at: at('line'), badge: 'tr', title: 'Where it is',
          text: 'A <code>[[url]]</code> counts as bare. Urls in blocks, drawers, tables, keyword lines and <code>=verbatim=</code> are left alone.' },
        { at: at('add'), badge: 'tl', title: 'Written by the server',
          text: 'Only the lines with links change, and it refuses if the file changed since the list was made.' },
      ],
    },
    { name: 'cli-link-title', setup: startSite, cli: { title: 'orgs link-title -retitle bread.org -dry-run', args: ['link-title', '-retitle', 'bread.org', '-dry-run'], opts: { cols: 116, rows: 19 } },
      notes: [
        { find: '[Understanding hydration in sourdough]]', title: 'A file\'s bare links',
          text: '<code>-retitle</code> prints the diff, then asks before writing; <code>-dry-run</code> stops at the diff.' },
        { title: 'One url, or a stream',
          text: '<code>orgs link-title URL</code> prints the link; <code>pbpaste | orgs link-title</code> does every url on stdin; <code>-filter</code> is for an editor (<code>:\'&lt;,\'&gt;!orgs link-title -filter</code>).' },
        { title: 'Careful fetching',
          text: 'http and https only, no private addresses unless <code>allowPrivate</code>, eight seconds, the first 512 kB; titles are kept 30 days.' },
      ] },

    // ---------------------------------------------------------------- maths
    {
      name: 'worg-latex', title: 'worg · Files · editing maths.org', page: { width: 1440, height: 860 },
      worg: async (page, ctx) => {
        await openEditor(page, ctx, 'maths.org');
        await clickLine(page, '.cm-line', "Parseval gives Euler's sum");
        await page.keyboard.press('A');
        await page.keyboard.press('Enter');
        await page.keyboard.press('Enter');
        for (const k of ['dm', 'sum', 'n', 'Tab', 'Tab', 'ooo', 'Tab', '1/n^2', 'Tab', '=@p^2/6']) {
          if (k === 'Tab') await page.keyboard.press('Tab'); else await page.keyboard.type(k, { delay: 70 });
          await sleep(150);
        }
        await sleep(1500);
        await scrub(page);
        await tag(page, [
          { name: 'typed', text: '\\sum_{n=1}^{\\infty} \\frac{1}{n^2}', contains: true, within: '.cm-content', last: true, closest: '.cm-line' },
          { name: 'card', text: 'display maths', within: 'main', up: 1 },
          { name: 'align', text: '\\begin{align}', contains: true, within: '.cm-content' },
        ]);
        await debugShot(page, 'latex');
        return mainClip(page);
      },
      notes: [
        { at: at('typed'), badge: 'tr', title: 'Typed, not spelled out',
          text: '<code>dm sum n TAB TAB ooo TAB 1/n^2 TAB =@p^2/6</code>: <code>dm</code> opens <code>\\[ \\]</code>, <code>sum</code> a sum with its limits, TAB walks its places, <code>ooo</code> is ∞, <code>a/</code> a fraction, <code>@p</code> π.' },
        { at: at('card'), badge: 'r', title: 'Drawn as you type',
          text: 'The maths the cursor is in, by KaTeX, light or dark with the page. The places a snippet left are underlined until TAB reaches them.' },
        { at: at('align'), badge: 'tr', ring: false, title: 'Plain org maths',
          text: '<code>$…$</code>, <code>\\( \\)</code>, <code>\\[ \\]</code> and <code>\\begin{align}</code>, as written: Emacs and every exporter read the file as before.' },
      ],
    },
    {
      name: 'worg-latex-snippets', title: 'worg · Maths snippets', page: { width: 1440, height: 900 },
      worg: async (page, ctx) => {
        await openEditor(page, ctx, 'maths.org');
        await page.keyboard.down('Meta'); await page.keyboard.down('Alt'); await page.keyboard.press('KeyL');
        await page.keyboard.up('Alt'); await page.keyboard.up('Meta');
        await sleep(2000);
        await tag(page, [
          { name: 'dlg', selector: '[role=dialog]', last: true },
          { name: 'find', selector: '[role=dialog] input', nth: 0 },
          { name: 'dm', text: 'dm', within: '[role=dialog]', upTo: 'display maths' },
          { name: 'greek', text: '@a', within: '[role=dialog]', upTo: '\\alpha' },
          { name: 'switches', text: 'Draw maths in place', within: '[role=dialog]' },
          { name: 'own', text: 'Make my own snippets file', within: '[role=dialog]', closest: 'button' },
        ]);
        return clipAround(page, ['dlg'], 20);
      },
      notes: [
        { at: at('find'), badge: 'tr', ring: false, title: 'The cheat sheet',
          text: '⌘⌥L in the editor, or Maths snippets in the palette: every snippet, searchable (<code>greek</code>, <code>frac</code>, <code>matrix</code>).' },
        { at: at('dm'), badge: 'l', ring: false, title: 'Into maths',
          text: '<code>mk</code> and <code>dm</code> in text make inline and display maths; <code>beq</code>, <code>bal</code> an equation and an align.' },
        { at: at('greek'), badge: 'l', ring: false, title: 'Drawn as it comes out',
          text: 'About 170 of them, each with where it works and whether it waits for TAB.' },
        { at: at('switches'), badge: 'tr', ring: false, title: 'Draw maths in place',
          text: 'Every piece of maths drawn but the one the cursor is in, as org-fragtog does in Emacs.' },
        { at: at('own'), badge: 'bl', title: 'Your own',
          text: 'Writes <code>latex-snippets.org</code>: org tables of Trigger, Replacement, Options. A row with a built-in\'s trigger replaces it.' },
      ],
    },
    { name: 'cli-latex', cli: { title: 'orgs latex snippets set', args: ['latex', 'snippets', 'set'], opts: { cols: 92, rows: 18 } },
      notes: [
        { find: 'Sets and logic', title: 'By group',
          text: 'The heading above each table in the snippets file; the words given keep only the snippets that have them all.' },
        { find: 'subseteq', title: 'Trigger and replacement',
          text: '<code>$1</code>, <code>$2</code> are the places TAB visits, <code>$0</code> where the cursor ends.' },
        { find: 'maths auto', title: 'Where, and when',
          text: 'In maths, display or inline maths only, or text; as you type (auto) or on TAB.' },
        { title: 'For other editors',
          text: '<code>orgs latex expand \'x + @a\'</code> shows what typing becomes; <code>orgs latex check notes.org</code> finds braces that do not balance, a <code>\\left</code> with no <code>\\right</code>, maths that never closes.' },
      ] },

    // ------------------------------------------------------- local pictures
    {
      name: 'worg-local-images', title: 'worg · Files · lisbon-journal.org', page: { width: 1440, height: 900 }, setup: startSite,
      worg: async (page, ctx) => {
        await lightPages(page);
        await openFile(page, ctx, 'lisbon-journal.org', 'Rendered');
        await clickShown(page, '[aria-label="Keep web pictures here"]');
        await sleep(3500);
        await tag(page, [
          { name: 'dlg', selector: '[role=dialog]', last: true },
          { name: 'thumb', selector: '[role=dialog] img', nth: 0 },
          { name: 'described', selector: '[role=dialog] img', nth: 1 },
          { name: 'size', text: '26 KB', within: '[role=dialog]', closest: '.MuiChip-root' },
          { name: 'keep', text: 'Keep 4 here', within: '[role=dialog]', closest: 'button' },
          { name: 'settings', text: 'Settings…', within: '[role=dialog]', closest: 'button' },
        ]);
        return clipAround(page, ['dlg'], 24);
      },
      notes: [
        { at: at('thumb'), badge: 'tl', title: 'The file\'s web pictures',
          text: '<code>[[https://…/x.png]]</code>, a picture url alone on its line, and links with a description. A url with no extension is asked what it is.' },
        { at: at('described'), badge: 'tr', ring: false, title: 'Inline or alone',
          text: 'A link in a paragraph is pointed at the copy; one alone on its line also gets <code>#+DOWNLOADED: url @ time</code>, above its <code>#+CAPTION:</code>.' },
        { at: at('size'), badge: 'tl', title: 'Each one\'s size',
          text: 'Untick one to leave it on the web. The bytes decide what a picture is, whatever the site says.' },
        { at: at('keep'), badge: 'tr', title: 'Keep them here',
          text: 'Into <code>images/</code> beside the file; only the lines holding links change.' },
        { at: at('settings'), badge: 'tr', title: 'Settings…',
          text: 'The folder (<code>images/{file}</code> for one per file), download on every save, and the <code>#+DOWNLOADED:</code> line.' },
      ],
    },
    { name: 'cli-local-images', setup: startSite, cli: { title: 'orgs images scan lisbon-journal.org', args: ['images', 'scan', 'lisbon-journal.org'], opts: { cols: 86, rows: 7 } },
      notes: [
        { find: 'images/', title: 'Where they would go',
          text: '<code>localImages.dir</code>, beside the file.' },
        { find: 'pastel-de-nata.png', title: 'Each picture',
          text: 'Its line, its url and its size. <code>orgs images localize lisbon-journal.org</code> downloads them and points the links at the copies.' },
        { title: 'Pictures nothing links',
          text: '<code>orgs images orphans</code> lists them; <code>-delete</code> asks, then deletes.' },
      ] },
  ],
};

async function debugShot(page, name) {
  if (!process.env.SHOT_DEBUG) return;
  await page.screenshot({ path: path.join(process.env.SHOT_DEBUG, `dbg-${name}.png`) });
}
