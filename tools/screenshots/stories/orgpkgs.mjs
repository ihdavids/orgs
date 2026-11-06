// The pictures for docs/org-packages.org, the optional org packages of the
// second set: org-recur's repeats, org-clock-budget's time budgets, toc-org's
// tables of contents, org-review's cadences, org-chef's recipes and ledger
// journals. The demo notebook with files of its own on top (docs/screenshots/
// orgpkgs): routines with rules in their titles, a week of clocked time
// against budgets, a handbook with a :TOC: heading (and reading notes whose
// table is out of date), areas to review, a cookbook with pictures drawn
// beside it, and four months of household money in a ledger block.
//
//   SHOT_PORT=8141 node capture.mjs -story orgpkgs
//   SHOT_PORT=8141 node capture.mjs -story orgpkgs cli-ledger

import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { at, tag, clipAround, mainClip, toTop, clickShown } from './engineer-worg.mjs';

const here = path.dirname(fileURLToPath(import.meta.url));
const shots = path.resolve(here, '../../../docs/screenshots');

const server = `  features:
    recur: true
    clockBudget: true
    toc: true
    review: true
    chef: true
    ledger: true
  chef:
    file: recipes.org
`;

export const demo = { src: [path.join(shots, 'demo'), path.join(shots, 'orgpkgs')], server };

const sleep = ms => new Promise(r => setTimeout(r, ms));

// Puts a query in Search's query box and waits for the rows.
async function search(page, ctx, query) {
  await ctx.openApp(page, 'Search');
  // The first input on screen in main: worg draws the phone's header too.
  await page.evaluate(() => {
    const el = [...document.querySelectorAll('main input')].find(e => e.getBoundingClientRect().width > 0);
    el.setAttribute('data-shot', 'query');
    el.focus(); el.select();
  });
  await page.keyboard.press('Backspace');
  await page.type('[data-shot="query"]', query);
  await page.keyboard.press('Enter');
  await sleep(2500);
}

// The demo lives in a temp folder; where worg prints its path, say ~/notes
// (as features.mjs does). React draws the bar again now and then.
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

// The ... menu of the search row with this title.
async function rowMenu(page, title) {
  const ok = await page.evaluate(title => {
    const row = [...document.querySelectorAll('main tr')].find(r => r.textContent.includes(title));
    const btns = row ? [...row.querySelectorAll('button')] : [];
    btns[btns.length - 1]?.click();
    return !!btns.length;
  }, title);
  if (!ok) throw new Error(`no row ${title}`);
  await sleep(700);
}

// A writing session under way, for the header's budget chip.
function writing(ctx) {
  if (!/No active clock/.test(ctx.cli('clocks'))) return;
  ctx.cli('clockin', '-hash', ctx.hashOf('Chapter four: the move north'));
}

export default {
  demo,
  shots: [
    // ------------------------------------------------------------ org-recur
    {
      name: 'worg-recur', title: 'worg · Search · IsRecurring()', page: { height: 1000 },
      worg: async (page, ctx) => {
        await search(page, ctx, 'IsRecurring()');
        await tag(page, [
          { name: 'two', text: 'every 2 days', contains: true, within: 'main table', thenUp: 1 },
          { name: 'mon', text: 'every Monday and Thursday', contains: true, within: 'main table', thenUp: 1 },
          { name: 'dec', text: 'every 1 December', contains: true, within: 'main table', thenUp: 1 },
          { name: 'table', selector: 'main table' },
        ]);
        return clipAround(page, ['table'], 16);
      },
      notes: [
        { at: at('two'), badge: 'l', title: 'The rule as a chip',
          text: '<code>* TODO Water the balcony herbs |+2|</code>: the bars come out of the title and the rule is said in words, with the date it moves to when marked done.' },
        { at: at('mon'), badge: 'l', title: 'Weekdays, days of the month',
          text: '<code>|Mon,Thu|</code>, <code>|1,15|</code>, <code>|wkdy|</code>, <code>|Sun 18:00|</code>: the earliest of the list is the next date.' },
        { at: at('dec'), badge: 'l', title: 'Once a year',
          text: '<code>|dec 1|</code>. The query here, <code>IsRecurring()</code>, finds every heading with a rule.' },
      ],
    },
    {
      name: 'worg-recur-dialog', title: 'worg · Search · Repeats…', page: { height: 900 },
      worg: async (page, ctx) => {
        await search(page, ctx, 'IsRecurring()');
        await rowMenu(page, 'Water the balcony herbs');
        await ctx.clickText(page, 'Repeats…', { settle: 900 });
        await page.evaluate(() => {
          const el = document.querySelector('.MuiModalDialog-root input');
          el.setAttribute('data-shot', 'rule'); el.focus(); el.select();
        });
        await page.keyboard.press('Backspace');
        await page.keyboard.type('Wed,Sat 08:00', { delay: 30 });
        await sleep(900);
        await tag(page, [
          { name: 'dialog', selector: '.MuiModalDialog-root' },
          { name: 'dates', text: 'Done today, then each time it comes round:', within: '.MuiModalDialog-root', thenUp: 1 },
          { name: 'stop', text: 'Stop repeating', within: '.MuiModalDialog-root' },
        ]);
        return clipAround(page, ['dialog'], 28);
      },
      notes: [
        { at: at('rule'), badge: 'r', title: 'The rule',
          text: 'Typed as it goes between the bars, and read back in words as you type.' },
        { at: at('dates'), badge: 'r', title: 'The next dates',
          text: 'Where marking it done would move <code>SCHEDULED</code>, and the times after that.' },
        { at: at('stop'), badge: 'l', title: 'Stop repeating',
          text: 'Takes the rule out of the title. <em>Set</em> writes it: a one-line edit to the headline.' },
      ],
    },
    { name: 'cli-recur',
      cli: { title: 'orgs recur -n 2', args: ['recur', '-n', '2'], opts: { cols: 150, rows: 9 } },
      notes: [
        { find: 'Water the balcony herbs', title: 'The rule',
          text: 'As written between the bars, then in words: <code>|+2|</code> is every 2 days, <code>|1,15|</code> the 1st and 15th of each month.' },
        { find: 'Take the recycling out', title: 'The next dates',
          text: 'Where done moves it, and after that (<code>-n</code> how many). <code>orgs recur explain \'|1,15|\'</code> does the same for a rule not yet written.' },
        { find: 'Change the furnace filter', title: 'Where it is now',
          text: 'After <code>now</code>: the date the next done moves. <code>orgs recur set \'Water\' \'|Mon,Fri|\'</code> gives a heading a rule.' },
      ] },

    // -------------------------------------------------------------- toc-org
    {
      name: 'worg-toc', title: 'worg · Files · allotment.org', page: { height: 900 },
      worg: async (page, ctx) => {
        await ctx.openApp(page, 'Files');
        await ctx.clickText(page, 'allotment.org', { settle: 2500 });
        await clickShown(page, '[aria-label="Hide the file tree"], [title="Hide the file tree"]');
        await scrub(page);
        await toTop(page);
        await clickShown(page, 'button[title="Table of contents (toc-org, org-make-toc)"]');
        await sleep(600);
        await tag(page, [
          { name: 'btn', selector: 'button[title="Table of contents (toc-org, org-make-toc)"]' },
          { name: 'menu', selector: '[role=menu]' },
          { name: 'frame', selector: 'main iframe' },
        ]);
        return mainClip(page);
      },
      notes: [
        { at: at('btn'), badge: 'l', title: 'The table of contents button',
          text: 'In the Files tab\'s bar and in the org editor.' },
        { at: at('menu'), badge: 'r', title: 'Update or insert',
          text: 'Bring the tables up to date, or add a <code>* Contents :TOC:</code> heading or org-make-toc\'s <code>:CONTENTS:</code> drawer.' },
        { at: at('frame'), badge: 'l', ring: false, title: 'Kept current',
          text: 'The <code>:TOC:</code> heading\'s body is the list, rewritten on every save from anywhere; its links go to the heading here and in exports.' },
      ],
    },
    { name: 'cli-toc',
      cli: { title: 'orgs toc -check', args: ['toc', '-check'], opts: { cols: 84, rows: 3 } },
      notes: [
        { find: 'areas.org: stale', title: 'Any kind of table',
          text: 'toc-org\'s heading, org-make-toc\'s drawer, or a <code>#+BEGIN: toc</code> block. For a git hook or CI: <code>orgs -local toc -check</code>. <code>orgs toc -print reading.org</code> shows what it should say.' },
        { find: 'reading.org: stale', title: 'A table out of date',
          text: 'A heading was added to <code>reading.org</code> outside orgs\' reach; its <code>:TOC:</code> list does not have it yet. Nothing is written, and the exit status is 1.' },
      ] },

    // ----------------------------------------------------------- org-review
    {
      name: 'worg-reviews', title: 'worg · Due for Review', page: { height: 900 },
      worg: async (page, ctx) => {
        await ctx.openApp(page, 'Reviews');
        await sleep(800);
        await tag(page, [
          { name: 'count', text: '1 of', contains: true, within: 'main' },
          { name: 'card', text: 'Friends and family', within: 'main', closest: '.MuiCard-root, .MuiSheet-root' },
          { name: 'late', text: 'days late', contains: true, within: 'main', upTo: 'last reviewed' },
          { name: 'keys', text: 'reviewed, with a note', contains: true, within: 'main', upTo: 'open' },
        ]);
        return clipAround(page, ['count', 'card', 'keys'], 28);
      },
      notes: [
        { at: at('count'), badge: 'tr', dx: 4, dy: -12, title: 'One at a time',
          text: 'Everything due, most overdue first; the switch adds what falls due this week.' },
        { at: at('card'), badge: 'tl', ring: false, title: 'The heading, rendered',
          text: 'Its text as you wrote it, so the review is reading it, not opening a file.' },
        { at: at('late'), badge: 'r', dx: 22, title: 'How late, how often',
          text: 'From <code>:LAST_REVIEW:</code> and <code>:REVIEW_DELAY:</code>, here inherited from <code>#+PROPERTY: REVIEW_DELAY +2w</code>.' },
        { at: at('keys'), badge: 'br', title: 'One key per decision',
          text: '<code>r</code> writes <code>LAST_REVIEW</code> and <code>NEXT_REVIEW</code>; snooze moves only the next; <code>c</code> changes the cadence.' },
      ],
    },
    { name: 'cli-reviews',
      cli: { title: 'orgs reviews', args: ['reviews'], opts: { cols: 112, rows: 6 } },
      notes: [
        { find: 'due 16 days ago', title: 'Most overdue first',
          text: 'Worked out from <code>:LAST_REVIEW:</code> and the <code>:REVIEW_DELAY:</code> it has or inherits; <code>:NEXT_REVIEW:</code> wins when set.' },
        { find: 'Areas:', title: 'Where it lives',
          text: 'The file and the headings above it. <code>-within 7</code> adds what falls due this week.' },
        { find: 'orgs reviews done N', title: 'Marking one reviewed',
          text: '<code>orgs reviews done 2 -note \'on track\'</code>, or <code>walk</code> for one at a time with the keys worg uses.' },
      ] },

    // -------------------------------------------------------------- org-chef
    {
      name: 'worg-chef', title: 'worg · Recipes', page: { height: 1240 },
      worg: async (page, ctx) => {
        await ctx.openApp(page, 'Recipes');
        await ctx.clickText(page, 'Weeknight Chicken Curry', { within: 'main', settle: 1500 });
        await toTop(page);
        await page.evaluate(() => {
          const plus = [...document.querySelectorAll('main button')].filter(b => b.querySelector('[data-testid="AddRoundedIcon"]'));
          plus[plus.length - 1]?.setAttribute('data-plus', '1');
        });
        for (let i = 0; i < 2; i++) { await page.click('[data-plus]'); await sleep(900); }
        await tag(page, [
          { name: 'list', text: 'Lemon Ricotta Pancakes', within: 'main', closest: '.MuiSheet-root' },
          { name: 'serv', text: '6 servings', within: 'main', upTo: 'as written' },
          { name: 'ingr', text: '3 pounds boneless chicken thighs, cut into bite-size pieces', contains: true, within: 'main' },
          { name: 'cook', text: 'Cook', within: 'main' },
          { name: 'pic', selector: 'main img[src*="chicken-curry"]', last: true },
        ]);
        return mainClip(page, { bottom: await page.evaluate(() => {
          const els = [...document.querySelectorAll('main *')].filter(e => e.textContent.trim() === 'a small bunch of coriander, to serve');
          return els[els.length - 1].getBoundingClientRect().bottom + 36; }) });
      },
      notes: [
        { at: at('list'), badge: 'tr', title: 'The cookbook',
          text: 'Every heading with <code>:source-url:</code>, <code>:RECORD: recipe</code>, or Ingredients and Directions under it, with its <code>:IMAGE:</code>. Searched by name and ingredient.' },
        { at: at('serv'), badge: 'r', title: 'Servings',
          text: 'Scaled from <code>:servings: 4</code>; only the leading amounts change (<code>2 pounds</code> to <code>3 pounds</code>). The file is left as written.' },
        { at: at('ingr'), badge: 'r', title: 'Ingredients to tick off',
          text: 'The ticks are kept in the browser, not the file.' },
        { at: at('cook'), badge: 'l', title: 'Cook',
          text: 'One step at a time in large type, the arrow keys to move. <em>Add to list</em> gathers recipes into a shopping list of checkboxes.' },
      ],
    },
    { name: 'cli-chef',
      cli: { title: 'orgs chef show curry -servings 6', args: ['chef', 'show', 'curry', '-servings', '6'], opts: { cols: 124, rows: 26 } },
      notes: [
        { find: 'serves 6', title: 'Scaled for six',
          text: 'From <code>:servings: 4</code>, org-chef\'s property. Nothing is written.' },
        { find: '1 1/2 (14 ounce) cans', title: 'Only the leading amount',
          text: 'Spellings and units are kept: <code>1 (14 ounce) can</code> becomes <code>1 1/2 (14 ounce) cans</code>, <code>2 pounds</code> becomes <code>3 pounds</code>.' },
        { find: 'Directions', word: true, title: 'Steps',
          text: '<code>orgs chef cook curry</code> goes through them one at a time; <code>orgs chef shop curry shakshuka</code> writes a shopping list.' },
      ] },

    // ---------------------------------------------------------------- ledger
    {
      name: 'worg-ledger', title: 'worg · Ledger', page: { height: 1120 },
      worg: async (page, ctx) => {
        await ctx.openApp(page, 'Ledger');
        const d = new Date(); d.setDate(1); d.setMonth(d.getMonth() - 4);
        const since = `since ${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-01`;
        await page.type('main input[placeholder^="or: "]', since);
        await page.keyboard.press('Enter');
        await sleep(1500);
        await ctx.clickText(page, 'Food', { within: 'main', settle: 2000 });
        await toTop(page);
        await tag(page, [
          { name: 'tree', text: 'Assets', within: 'main', closest: '.MuiSheet-root' },
          { name: 'period', selector: 'main input[placeholder^="or: "]' },
          { name: 'chart', text: 'by month', contains: true, within: 'main' },
          { name: 'reg', selector: 'main table' },
        ]);
        return mainClip(page, { bottom: await page.evaluate(() =>
          document.querySelectorAll('[data-shot="reg"] tr')[9].getBoundingClientRect().bottom + 2) });
      },
      notes: [
        { at: at('period'), badge: 'r', title: 'A journal and a period',
          text: 'Every source, one file or one block; periods in words (<code>this year</code>, <code>q3</code>, <code>since 2026-07-01</code>), or one typed.' },
        { at: at('tree'), badge: 'tr', title: 'The accounts',
          text: 'Read by orgs itself from the <code>#+BEGIN_SRC ledger</code> block in <code>money.org</code>: no ledger or hledger needed.' },
        { at: at('chart'), badge: 'r', title: 'Month by month',
          text: 'The account picked, here <code>Expenses:Food</code>, as bars; <em>Table</em> shows the numbers.' },
        { at: at('reg'), badge: 'tr', ring: false, title: 'Its register',
          text: 'Each posting with a running total; a row opens its line in the file.' },
      ],
    },
    { name: 'cli-ledger',
      cli: { title: 'orgs ledger bal', args: ['ledger', 'bal'], opts: { cols: 60, rows: 23, side: true } },
      notes: [
        { find: 'Assets:Bank', title: 'As ledger prints it',
          text: 'Balances of every journal: the ledger blocks in <code>money.org</code> here, read by orgs with no ledger installed.' },
        { find: 'Utilities', word: true, title: 'The account tree',
          text: 'Parents carry their children\'s total. <code>-depth 2</code> stops at two levels; <code>bal Expenses not Rent -period \'this year\'</code> narrows it.' },
        { find: '-------', title: 'Balanced',
          text: 'Every transaction sums to zero, so the total does. <code>orgs ledger check</code> reports the ones that don\'t, at their line.' },
      ] },
    // ----------------------------------------------------- org-clock-budget
    { name: 'cli-budget',
      cli: { title: 'orgs budget', args: ['budget'], opts: { cols: 92, rows: 11 } },
      notes: [
        { find: 'Week 2026-W', title: 'A period',
          text: 'How far through it is, what has been given out of the 84 budgetable hours (<code>clockBudget.dailyHours</code>) and what has been clocked.' },
        { find: 'Writing the book', title: 'A budget',
          text: '<code>:BUDGET_WEEK: 5:00</code> on the heading; the bar\'s line is the pace.' },
        { find: '0:25 over', title: 'Over',
          text: '<code>orgs budget -over</code> lists only these; clocking in under one says so.' },
      ] },

    {
      name: 'worg-budget', title: 'worg · Time Budgets', setup: writing, page: { height: 1000 },
      worg: async (page, ctx) => {
        await ctx.openApp(page, 'Budgets');
        await sleep(1000);
        await tag(page, [
          { name: 'chip', selector: 'button[aria-label^="Budget:"]' },
          { name: 'periods', text: 'Quarter', within: 'main', upTo: 'Year' },
          { name: 'over', text: 'Spanish practice', within: 'main', closest: '.MuiSheet-root, [class*="Sheet"]' },
          { name: 'overpill', text: '0:25 over', within: 'main', upTo: 'Spanish practice' },
          { name: 'week', text: 'Week', within: 'main', nth: 1 },
          { name: 'add', text: 'Add a budget', within: 'main' },
        ]);
        return mainClip(page, { bottom: await page.evaluate(() => {
          const els = [...document.querySelectorAll('main *')].filter(e => e.textContent.trim() === 'Garden planner app');
          return els[els.length - 1].getBoundingClientRect().bottom + 50; }) });
      },
      notes: [
        { at: at('chip'), badge: 'l', title: 'While the clock runs',
          text: 'A chip beside the focus timer: this week\'s 5:00 for <em>Writing the book</em>, with the clock under it counted up to now. Red once over.' },
        { at: at('periods'), badge: 'tr', title: 'Each period',
          text: '<code>:BUDGET_DAY:</code> to <code>:BUDGET_YEAR:</code>, a period at a time with the arrows.' },
        { at: at('overpill'), badge: 'tr', title: 'Over budget',
          text: 'Time clocked under the heading and everything below it, against its allowance. The tick on each bar is how far through the week it is.' },
        { at: at('add'), badge: 'tr', title: 'Add or change one',
          text: 'Finds a heading by its title and writes the property; the pencil changes or takes one off.' },
      ],
    },
  ],
};
