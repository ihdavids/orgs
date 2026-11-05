// The pictures of the D&D module, for docs/dnd.org: the character sheet (and
// a spell cast on it), the random, dungeon and map generators, and the DM's
// screen of a saved dungeon - the map, a room's key, the initiative tracker,
// the session drawer - with the players' screen beside it. The demo notebook
// with docs/screenshots/dnd on top: a level 5 sorcerer, and a dungeon saved by
// the dungeon generator (its .dungeon.json and maps/ come along) with the
// players' fog half lifted and a fight in its first round.
//
//   SHOT_PORT=8103 node capture.mjs -story dnd
//   SHOT_PORT=8103 node capture.mjs -story dnd dnd-sheet

import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { at, tag, toTop } from './engineer-worg.mjs';

const here = path.dirname(fileURLToPath(import.meta.url));
const shots = path.resolve(here, '../../../docs/screenshots');

// Spliced in straight after the base config's exporters, so its first line
// adds to that list: the sheet is the dndsheet exporter's page.
const server = `    - name: "dndsheet"
`;

export const demo = { src: [path.join(shots, 'demo'), path.join(shots, 'dnd')], server };

const sleep = ms => new Promise(r => setTimeout(r, ms));
const MAP = 'dungeons/thornas-keep.org';

// The party the initiative tracker plays with. Kept per user in the
// extensions file, by absolute path, so it is made for each run.
async function party(ctx) {
  const org = path.join(ctx.work, 'org');
  await ctx.post('dnd/party/file', { file: path.join(org, 'party.org') });
  await ctx.post('dnd/party', { members: [
    { name: 'Lyra Silverleaf', player: 'Sam', sheet: path.join(org, 'lyra.org') },
    { name: 'Brannoc', player: 'Jo', ac: 18, hp: 44, init: 0, level: 5, class: 'Fighter', passive: 13 },
    { name: 'Mirela', player: 'Alex', ac: 16, hp: 38, init: 1, level: 5, class: 'Cleric', passive: 15 },
    { name: 'Tamsin Reed', player: 'Kai', ac: 15, hp: 33, init: 4, level: 5, class: 'Rogue', passive: 16 },
  ] });
}

// A button by its title or aria-label (the DM screen's are icons).
async function press(page, title) {
  const ok = await page.evaluate(t => {
    const b = [...document.querySelectorAll('button, [title]')].find(b => b.getClientRects().length &&
      (b.title === t || b.getAttribute('aria-label') === t));
    b?.click();
    return !!b;
  }, title);
  if (!ok) throw new Error(`no button "${title}"`);
  await sleep(1000);
}

// The character sheet as its own page, the way `orgs dnd sheet` writes it and
// worg's Files tab frames it; wide enough for its three columns.
async function sheet(page, ctx) {
  const r = await fetch(`${ctx.base}/file/dndsheet?query=lyra.org`).then(r => r.json());
  if (!r.Ok && !r.Msg) throw new Error('no sheet for lyra.org');
  await page.setViewport({ width: 1280, height: 900, deviceScaleFactor: 2 });
  // On the server's origin, so the sheet's own requests (rolls, casting) land.
  await page.goto(`${ctx.base}/web/`, { waitUntil: 'domcontentloaded' }).catch(() => { });
  await page.setContent(r.Msg, { waitUntil: 'load' });
  await sleep(2500);
}

// One of the tabletop tools from the dice menu on the rail.
async function tool(page, ctx, name) {
  await ctx.openApp(page, 'Agenda');
  await press(page, 'Tabletop RPG tools');
  await page.evaluate(n => [...document.querySelectorAll('[role=menuitem]')].find(e => e.textContent.includes(n)).click(), name);
  await sleep(2500);
}

// A button by its text inside the open panel whose heading reads panel (every
// tool is mounted at once, each with its own Generate).
async function panelButton(page, panel, text) {
  const ok = await page.evaluate((panel, text) => {
    const b = [...document.querySelectorAll('button')].find(b => b.getClientRects().length &&
      b.textContent.trim() === text && b.closest('[role=dialog]')?.textContent.includes(panel));
    b?.click();
    return !!b;
  }, panel, text);
  if (!ok) throw new Error(`no "${text}" in ${panel}`);
}

// The saved dungeon in the Files tab, given the whole window.
async function openMap(page, ctx) {
  await ctx.openApp(page, 'Files');
  await press(page, 'Hide the application panel');
  await ctx.clickText(page, 'dungeons', { settle: 1000 });
  await ctx.clickText(page, 'thornas-keep.org', { settle: 3500 });
  await press(page, 'Hide the file tree');
  await press(page, 'Hide this bar and the page title');
  await sleep(1500);
}

// A saved fight opens with the tracker showing; its own close button shuts it.
async function tracker(page, open) {
  const shown = !!(await page.$('button[aria-label="Close the tracker"]'));
  if (shown && !open) await press(page, 'Close the tracker');
  if (!shown && open) await press(page, 'The initiative tracker');
}

export default {
  demo,
  shots: [
    {
      name: 'dnd-sheet', title: 'Lyra Silverleaf - character sheet',
      worg: sheet,
      notes: [
        { at: { selector: '.medallion, .portrait, [class*=medallion]' }, badge: 'r', title: 'The portrait',
          text: 'The character\'s <code>DND_IMAGE</code>, cropped to the medallion, or a drawn bust for their race. Experience runs round its rim.' },
        { at: { text: 'Level', closest: 'div' }, title: 'Level and the rest',
          text: 'Level, proficiency bonus, alignment and inspiration (click to give or spend it).' },
        { at: { text: 'Ability Scores', up: 1 }, badge: 'tl', title: 'Abilities, saves and skills',
          text: 'Scores and modifiers, with every save and skill worked out from them; a dot marks a proficiency. Click one to roll it.' },
        { at: { text: 'Hit Points', up: 2 }, title: 'Combat',
          text: 'Armour class, initiative and speed, then hit points: type a number and press Hurt, Heal or Temp. Death saves and the carrying load sit under it.' },
        { at: { text: 'Attacks', within: 'body', last: true, up: 2 }, badge: 'tl', title: 'Attacks, inventory, coins',
          text: 'Each attack with its bonus and damage, ready to roll; the inventory, purse and personality are tabs of the same card.' },
        { at: { text: 'Spellcasting', up: 2 }, title: 'Spells',
          text: 'Save DC and attack, a box per spell slot, and every spell known with a Cast button that spends the slot and rolls its damage.' },
        { at: { text: 'Rolls', last: true }, badge: 'l', title: 'Rolls',
          text: 'The dice tray: every roll made on the sheet, with its history.' },
        { at: { text: 'Session Notes' }, title: 'Session notes',
          text: 'Notes, the evening\'s timeline and the session log, written into the character\'s org file.' },
      ],
    },
    {
      name: 'dnd-sheet-cast', title: 'Casting Lightning Bolt',
      worg: async (page, ctx) => {
        await sheet(page, ctx);
        await page.evaluate(() => {
          const row = [...document.querySelectorAll('*')].filter(e => e.textContent.includes('Lightning Bolt') && e.querySelector('button')).pop();
          [...row.querySelectorAll('button')].find(b => b.textContent.trim() === 'Cast').click();
        });
        // Long enough for the dice to land, short enough to catch the bolt.
        await sleep(1100);
        await tag(page, [{ name: 'total', text: 'Total', up: 1 }, { name: 'slots', text: '3rd level slots: 1 of 2 left' }]);
      },
      notes: [
        { at: at('total'), badge: 'l', title: 'The roll',
          text: 'The spell\'s damage (8d6 lightning) thrown as 3D dice across the sheet, totalled in the tray with the saving throw it asks for.' },
        { at: at('slots'), badge: 'r', title: 'The slot it spent',
          text: 'Casting ticks a 3rd level slot off; a long rest gives them back.' },
        { title: 'The flourish',
          text: 'Each school and damage type has its own effect - lightning arcs here, fire flares, healing glows. Off in the tray\'s settings, and for anyone who prefers reduced motion.' },
      ],
    },
    {
      name: 'dnd-randgen', title: 'worg - random generator',
      worg: async (page, ctx) => {
        await tool(page, ctx, 'Random generator');
        await panelButton(page, 'Random Generator', 'Generate');
        await sleep(2500);
        await tag(page, [{ name: 'kind', text: 'Generate', within: '[role=dialog]', nth: 0, up: 1 },
          { name: 'save', text: 'Save…', within: '[role=dialog]', last: true }]);
      },
      notes: [
        { at: at('kind'), badge: 'tl', title: 'What to roll',
          text: 'Encounters balanced against the party, treasure, magic items, NPCs, names, taverns, traps, adventure hooks, trinkets, wild magic…' },
        { at: { text: 'Party size', up: 2 }, badge: 'tl', title: 'Its settings',
          text: 'Each generator describes its own form (party, difficulty, environment here), so a new one needs no change to worg.' },
        { at: { selector: '[role=dialog] [type=checkbox]' }, badge: 'l', ring: false, title: 'Keep the good ones',
          text: 'Ticked results stay through the next roll.' },
        { at: at('save'), title: 'Save',
          text: 'Files the ticked results as org headings, under any heading you pick.' },
      ],
    },
    {
      name: 'dnd-dungeon', title: 'worg - dungeon generator',
      worg: async (page, ctx) => {
        await tool(page, ctx, 'Dungeon generator');
        // A seed, so the docs show the same dungeon every time.
        await page.click('input[placeholder="random"]');
        await page.keyboard.type('20261103');
        await panelButton(page, 'Dungeon Generator', 'Roll this');
        await sleep(4000);
        await toTop(page);
      },
      notes: [
        { at: { text: 'The place', up: 1 }, badge: 'tl', title: 'Settings',
          text: 'The kind of place (dungeon, dwelling, cave), the party it is stocked for, size, layout, rooms, corridors, doors and stairs. A seed makes the same dungeon again.' },
        { at: { text: 'GM map' }, badge: 'tl', title: 'Three pictures',
          text: 'The GM\'s map, the players\' map (no secrets, no numbers) and the one-page sheet with the key beside it.' },
        { at: { text: 'Classic hatching' }, title: 'The look',
          text: 'Hatched, grey, cavern, blueprint, dark and others, drawn flat, tilted or in 3D.' },
        { at: { selector: '[aria-label="Renumber rooms"]', up: 1 }, badge: 'r', title: 'Draw on it',
          text: 'Rooms in any shape, corridors, doors, water, stairs, furniture, labels; undo and redo. Or start on a picture of your own (a Dungeon Alchemist export).' },
        { at: { text: 'Backstory', up: 1 }, badge: 'tl', title: 'The key',
          text: 'A backstory, and each room\'s name, a line about it, its ways out, monsters and treasure - all editable, and saved as an org file.' },
      ],
    },
    {
      name: 'dnd-mapgen', title: 'worg - map generator',
      worg: async (page, ctx) => {
        await tool(page, ctx, 'Map generator');
        await ctx.clickText(page, 'Towns & roads', { settle: 1000 });
        await ctx.clickText(page, 'Settle the land', { settle: 3500 });
        // The same button shuts its popover; Escape does not.
        await ctx.clickText(page, 'Towns & roads', { settle: 1000 });
      },
      notes: [
        { at: { text: 'Generator', up: 1 }, badge: 'tl', title: 'What to make',
          text: 'A region\'s terrain, a world on hexes, an inked region, a town or a village. Sliders for land, coast, mountains, climate.' },
        { at: { text: 'Towns & roads' }, title: 'Towns and roads',
          text: 'Settlements placed where people would live, joined by roads; a hex grid over it on request.' },
        { at: { text: 'About this land', up: 1 }, badge: 'tl', title: 'Places',
          text: 'Found a place by clicking the map: it becomes a heading in the map\'s org file, and can have a town or dungeon map of its own.' },
        { title: 'Run games on it',
          text: 'Saved, it opens in the Files tab as a DM screen like a dungeon: fog of war, the party\'s track, pins and a journal.' },
      ],
    },
    {
      name: 'dnd-dm-screen', title: 'worg - the DM screen',
      setup: party,
      worg: async (page, ctx) => {
        await openMap(page, ctx);
        await tracker(page, false);
        await press(page, 'Lair');
        await sleep(1200);
        await tag(page, [{ name: 'card', text: 'Reveal the room', up: 3 }]);
      },
      notes: [
        { at: { text: 'Thorna\'s Keep' }, badge: 'tl', title: 'The map',
          text: 'A dungeon saved by the generator, opened in the Files tab: its org file holds the key, the fog and the fight.' },
        { at: { selector: '[title="See it as the players will"], [aria-label="See it as the players will"]' }, badge: 'bl', ring: false, title: 'At the table',
          text: 'The players\' view, the look, wandering monsters, tokens, tiles, music and sounds, pictures to show, the session drawer and the initiative tracker.' },
        { at: { selector: '[title="Fog on the players\' map"], [aria-label="Fog on the players\' map"]' }, badge: 'r', ring: false, title: 'Fog and tools',
          text: 'Fog on either map with brushes to reveal and hide, the party\'s position and trail, measuring, pins, the journal, lights and effects.' },
        { at: at('card'), badge: 'tl', title: 'A room',
          text: 'Click a room (or its number below) for its key: what is there, its ways out, its monsters - one click reveals it to the players or moves the party in.' },
      ],
    },
    {
      name: 'dnd-initiative', title: 'worg - initiative',
      worg: async (page, ctx) => {
        await openMap(page, ctx);
        await tracker(page, true);
        await page.mouse.move(5, 5);
        await sleep(1200);
      },
      notes: [
        { at: { text: 'Next turn', up: 1 }, badge: 'tl', title: 'The round',
          text: 'Next turn, back a turn, end the fight; roll initiative for the monsters, everyone without one, or everyone.' },
        { at: { text: 'Goblin', up: 3 }, badge: 'r', title: 'Whose turn',
          text: 'In initiative order: hit points, armour class and conditions, with a menu to damage, heal or remove. The party\'s numbers come from their character sheets.' },
        { at: { selector: 'input[placeholder="Add a monster by name…"]' }, badge: 'tl', title: 'Bring them in',
          text: 'Monsters by name (with their stat blocks), from a room\'s card or the wandering table, and the party in one click.' },
        { title: 'Tokens',
          text: 'Everyone in the fight has a token on the map, numbered like the list; drag them as they move.' },
      ],
    },
    {
      name: 'dnd-session', title: 'worg - the session drawer',
      worg: async (page, ctx) => {
        await openMap(page, ctx);
        await tracker(page, false);
        await press(page, 'The session: dice, notes and the evening\'s timeline');
        await sleep(1200);
      },
      notes: [
        { at: { text: 'Not recording', up: 2 }, badge: 'tl', title: 'The session',
          text: 'Start one and every roll, note and fight of the evening is kept in your notes, with when it happened.' },
        { at: { text: 'Your dice', up: 1 }, badge: 'tl', title: 'Dice',
          text: 'Pick a set and a die, or type <code>2d6+3</code>; advantage and disadvantage. They tumble across the map in 3D.' },
        { at: { text: 'Timeline' }, title: 'Notes and timeline',
          text: 'Write as you play; the timeline draws the evening, fights and all, and the DM\'s notes link to rooms and monsters.' },
      ],
    },
    {
      name: 'dnd-players', title: 'worg - the players\' screen',
      page: { width: 1440, height: 900 },
      worg: async (page, ctx) => {
        await page.goto(`${ctx.base}/?players=1&dungeon=${encodeURIComponent(MAP)}`, { waitUntil: 'domcontentloaded' });
        await sleep(4000);
      },
      notes: [
        { title: 'For a second screen',
          text: 'A tab of its own (the DM screen\'s screen button) for a TV or projector: only the players\' map, no numbers, no secrets.' },
        { at: { text: 'Initiative', up: 1 }, badge: 'l', title: 'Whose turn it is',
          text: 'The fight\'s order, with monsters the DM has hidden left out.' },
        { title: 'Fog of war',
          text: 'Lifts as the DM reveals rooms or paints it away, with the party\'s marker where they stand. Changes arrive live.' },
      ],
    },
    {
      name: 'cli-dnd-show', cli: { title: 'orgs dnd show -file lyra.org', args: ['dnd', 'show', '-file', 'lyra.org'], opts: { cols: 82, rows: 34, side: true } },
      notes: [
        { find: 'LYRA SILVERLEAF', title: 'Who',
          text: 'Race, class and level, background and alignment.' },
        { find: 'AC 14', title: 'The numbers',
          text: 'Armour class, hit points, hit dice, initiative, speed, proficiency, and the spell save and attack.' },
        { find: 'STR', title: 'Abilities and saves',
          text: 'Scores, modifiers and saves; a star is a proficiency.' },
        { find: 'Skills:', title: 'Skills', text: 'The proficient ones, with the passive scores under them.' },
        { find: 'Attacks:', title: 'Attacks', text: 'To hit, damage and range.' },
        { find: 'Spell slots', title: 'Spell slots and kit', text: 'Slots by level, equipment, load and purse.' },
      ],
    },
  ],
};
