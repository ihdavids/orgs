// The pictures for docs/story-engineer-worg.org: an engineer's week in worg,
// GTD and pomodoros. Its own notebook (docs/screenshots/engineer-worg) adds
// what the base demo lacks: finished tasks with estimates and clocks for
// Insights, last week's time, a stale task, a someday file, ids for AFTER.
//
//   SHOT_PORT=8102 node capture.mjs -story engineer-worg
//   SHOT_PORT=8102 node capture.mjs -story engineer-worg eng-worg-focus
//
// The shots share one server and run in order, so each one's setup is placed
// where it spoils nothing after it. The weekly review's step is kept on the
// server, so every review shot jumps to its step by name.
//
// Each shot marks what its notes point at with data-shot attributes (tag),
// because worg's labels share elements with numbers that change ("45m planned
// of 8h") and with key caps ("r refile"), and a panel is easier to ring than
// the words in its corner.

import path from 'node:path';
import { fileURLToPath } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const notebook = path.resolve(here, '../../../docs/screenshots/engineer-worg');

const server = `  captureTemplates:
    - name: "Inbox"
      type: "entry"
      target:
        type: "file"
        filename: "inbox.org"
      template: |-
        :PROPERTIES:
        :CREATED:  {{now}}
        :END:
        {{CONTENT}}
    - name: "Interruption"
      type: "entry"
      target:
        type: "file"
        filename: "inbox.org"
      template: |-
        :PROPERTIES:
        :CREATED:  {{now}}
        :FROM:     {{from|Who asked?}}
        :END:
        {{CONTENT}}
`;

// Two pomodoros already done this morning, so the Focus tab's Today list and
// the set's dots have something to say. The timer reads its file once, at
// startup, which is why this is a file and not a request.
function focusFile() {
  const ago = m => new Date(Date.now() - m * 60000).toISOString();
  const rec = (start, title) => ({
    Hash: '', Title: title, File: 'work.org', Start: ago(start), End: ago(start - 25),
    Minutes: 25, Completed: true, Break: 5,
  });
  return JSON.stringify({
    Phase: 'idle', LastEnd: ago(4), Streak: 2,
    Settings: { Work: 25, Short: 5, Long: 15, LongEvery: 4, AutoNext: false },
    History: [rec(34, 'Add retries to the webhook sender'), rec(64, 'Update the API docs')],
  });
}

const sleep = ms => new Promise(r => setTimeout(r, ms));
const at = name => ({ selector: `[data-shot="${name}"]` });

// Marks elements for the notes. A spec finds by selector, or by text (exact,
// or contains) inside within; then nth, up (parents), upTo (the nearest
// ancestor that also holds that text), next (its next sibling), closest,
// thenUp. Only
// what is on screen counts: worg keeps drawers parked off to the right.
async function tag(page, specs) {
  const missing = await page.evaluate(specs => {
    const shown = e => {
      const r = e.getBoundingClientRect();
      return r.width > 0 && r.height > 0 && r.left < innerWidth && r.right > 0;
    };
    const out = [];
    for (const s of specs) {
      let els;
      if (s.selector) els = [...document.querySelectorAll(s.selector)];
      else {
        const has = e => s.contains ? e.textContent.trim().includes(s.text) : e.textContent.trim() === s.text;
        els = [...document.querySelectorAll(`${s.within || 'body'} *`)].filter(e => has(e) && ![...e.children].some(has));
      }
      els = els.filter(shown);
      let el = s.last ? els[els.length - 1] : els[s.nth || 0];
      if (!el) { out.push(s.name); continue; }
      for (let i = 0; i < (s.up || 0); i++) el = el.parentElement;
      if (s.upTo) while (el.parentElement && !el.textContent.includes(s.upTo)) el = el.parentElement;
      if (s.next) el = el.nextElementSibling || el;
      if (s.closest) el = el.closest(s.closest) || el;
      for (let i = 0; i < (s.thenUp || 0); i++) el = el.parentElement;
      el.setAttribute('data-shot', s.name);
    }
    return out;
  }, specs);
  if (missing.length) throw new Error(`could not tag ${missing.join(', ')}`);
}

// Clicks the first element on screen that matches (worg draws its header
// twice, the phone's copy first and hidden).
async function clickShown(page, selector) {
  const ok = await page.evaluate(sel => {
    const el = [...document.querySelectorAll(sel)].find(e => {
      const r = e.getBoundingClientRect();
      return r.width > 0 && r.height > 0 && r.left < innerWidth;
    });
    el?.click();
    return !!el;
  }, selector);
  if (!ok) throw new Error(`nothing on screen matches ${selector}`);
  await sleep(900);
}

// The rectangle around the tagged elements, padded, kept on the page.
async function clipAround(page, names, pad = 20) {
  const r = await page.evaluate(names => {
    const rs = names.map(n => document.querySelector(`[data-shot="${n}"]`)?.getBoundingClientRect()).filter(Boolean);
    return { x0: Math.min(...rs.map(r => r.left)), y0: Math.min(...rs.map(r => r.top)),
      x1: Math.max(...rs.map(r => r.right)), y1: Math.max(...rs.map(r => r.bottom)) };
  }, names);
  const vp = page.viewport();
  const x = Math.max(0, Math.floor(r.x0 - pad)), y = Math.max(0, Math.floor(r.y0 - pad));
  return { x, y, width: Math.min(vp.width - x, Math.ceil(r.x1 - x + pad)), height: Math.min(vp.height - y, Math.ceil(r.y1 - y + pad)) };
}

// The main column, without the navigation down the left; from top (a page
// y) down to bottom, or to the bottom of the page.
async function mainClip(page, { top, bottom } = {}) {
  const r = await page.evaluate(() => {
    const b = document.querySelector('main').getBoundingClientRect();
    return { x: b.left, y: b.top, w: b.width };
  });
  const vp = page.viewport();
  const y = Math.max(0, top ?? r.y);
  const end = Math.min(vp.height, bottom ?? vp.height);
  return { x: Math.floor(r.x), y: Math.floor(y), width: Math.floor(Math.min(r.w, vp.width - r.x)), height: Math.floor(end - y) };
}

// Where a tagged element starts or ends on the page, for a clip.
const edge = (page, name, side = 'top') => page.evaluate((n, s) =>
  document.querySelector(`[data-shot="${n}"]`).getBoundingClientRect()[s], name, side);

// Opens the weekly review on one step, by the step's name in the stepper.
async function reviewStep(page, ctx, step) {
  await ctx.openApp(page, 'WeeklyReview');
  await ctx.clickText(page, step, { within: 'main', settle: 1800 });
  await toTop(page);
  await tag(page, [{ name: 'step', text: 'Step', contains: true, within: 'main', nth: 0 }]);
}

// clickText scrolls what it clicks into the middle; put the page back.
async function toTop(page) {
  await page.evaluate(() => { scrollTo(0, 0); document.querySelectorAll('*').forEach(e => { if (e.scrollTop) e.scrollTop = 0; }); });
  await sleep(400);
}

export default {
  demo: { src: notebook, extensions: path.join(notebook, 'extensions.yaml'), server, files: { 'focus_data.json': focusFile() } },
  shots: [
    // ------------------------------------------------------------ Monday
    {
      name: 'eng-worg-review-inbox', title: 'worg · Weekly Review',
      worg: async (page, ctx) => {
        await reviewStep(page, ctx, 'Empty the inbox');
        await tag(page, [
          { name: 'steps', text: 'Empty the inbox', within: 'main', upTo: 'Someday sweep' },
          { name: 'last', text: 'Your first weekly review', contains: true, within: 'main', closest: '.MuiAlert-root' },
          { name: 'card', text: 'Call the plumber about the kitchen tap', within: 'main', closest: '.MuiSheet-variantOutlined' },
          { name: 'keys', text: 'rrefile', within: 'main', up: 1 },
        ]);
        return mainClip(page, { top: await edge(page, 'steps') - 24, bottom: await edge(page, 'keys', 'bottom') + 40 });
      },
      notes: [
        { at: at('steps'), title: 'The seven steps',
          text: 'GTD\'s weekly review in the order it makes its decisions. Click one to jump to it. The step you are on is kept on the server, so stopping halfway resumes here, in worg or in <code>orgs review week</code>.' },
        { at: at('last'), title: 'When the last one was',
          text: 'How long ago the last review was finished, or, as here, that this is the first.' },
        { at: at('card'), title: 'The inbox, one item at a time',
          text: 'Step one is the Inbox tab\'s triage, inside the review: where the item lives, its keyword and headline.' },
        { at: at('keys'), badge: 'r', title: 'One key per decision',
          text: 'Refile, schedule, tag, archive, done or next, and the next item comes up. <code>u</code> takes the last one back.' },
      ],
    },
    {
      name: 'eng-worg-review-projects', title: 'worg · Weekly Review',
      worg: async (page, ctx) => {
        await reviewStep(page, ctx, 'Projects with no next action');
        await tag(page, [
          { name: 'project', text: 'Website relaunch', within: 'main', closest: '.MuiSheet-root' },
          { name: 'pick', text: 'Make it NEXT', within: 'main' },
          { name: 'write', selector: 'main input[placeholder^="…or write the next action"]' },
        ]);
        return mainClip(page, { top: await edge(page, 'step') - 20, bottom: await edge(page, 'project', 'bottom') + 30 });
      },
      notes: [
        { at: at('project'), title: 'A stuck project',
          text: 'Open tasks, and not one of them NEXT: nothing in it moves until somebody decides what comes first.' },
        { at: at('pick'), badge: 'l', title: 'Pick its next action',
          text: 'One click makes an open task NEXT, which is nearly always the whole answer.' },
        { at: at('write'), badge: 'r', title: '…or write a new one',
          text: 'When the next step isn\'t written down yet: type it and press Enter.' },
      ],
    },
    {
      name: 'eng-worg-review-time', title: 'worg · Weekly Review',
      worg: async (page, ctx) => {
        await reviewStep(page, ctx, 'Last week\'s time');
        await tag(page, [
          { name: 'numbers', text: 'planned', within: 'main', closest: '.MuiSheet-variantSoft', thenUp: 1 },
          { name: 'projects', selector: 'main table', nth: 0 },
          { name: 'tags', selector: 'main table', nth: 1 },
        ]);
        return mainClip(page, { top: await edge(page, 'step') - 20, bottom: await edge(page, 'tags', 'bottom') + 50 });
      },
      notes: [
        { at: at('numbers'), title: 'Clocked against planned',
          text: 'Every clock line last week, against the EFFORT of what was scheduled or due in it.' },
        { at: at('projects'), title: 'By project',
          text: 'Each project\'s time: solid for clocked, faint for planned and not spent, gold for over the plan.' },
        { at: at('tags'), title: 'By tag',
          text: 'The same by tag, inherited tags included, so the rows don\'t add up to the total.' },
      ],
    },
    {
      name: 'eng-worg-estimates', title: 'worg · Insights', page: { height: 1000 },
      worg: async (page, ctx) => {
        await ctx.openApp(page, 'Insights');
        await ctx.clickText(page, 'Estimates', { within: 'main', settle: 1200 });
        await ctx.clickText(page, 'Project', { within: 'main', settle: 1200 });
        await ctx.clickText(page, '8h', { within: 'main', settle: 1500 });
        await toTop(page);
        await tag(page, [
          { name: 'tabs', text: 'Estimates', within: 'main', up: 1 },
          { name: 'controls', text: 'Finished', within: 'main', closest: '.MuiSheet-root' },
          { name: 'numbers', text: 'tasks measured', within: 'main', closest: '.MuiSheet-root', thenUp: 1 },
          { name: 'table', selector: 'main table' },
          { name: 'over', text: 'Worst over-runs', within: 'main', closest: '.MuiCard-root, .MuiSheet-root' },
        ]);
        return mainClip(page, { top: await edge(page, 'tabs') - 16, bottom: await edge(page, 'over', 'bottom') + 30 });
      },
      notes: [
        { at: at('controls'), title: 'What to measure',
          text: 'Finished tasks with an EFFORT and clock lines, over a range, grouped by tag, project, file or person. <code>8h</code> makes a day of effort a working day.' },
        { at: at('numbers'), title: 'How far off',
          text: 'Tasks measured, their estimates, what they took, the typical ratio, and a sentence on what to do about it.' },
        { at: at('table'), title: 'Group by group',
          text: 'Each project\'s estimate against its actual as a bar, with the spread. Click a row for its tasks.' },
        { at: at('over'), title: 'The worst misses',
          text: 'The tasks furthest over their estimates, and beside them the biggest under-runs.' },
      ],
    },
    {
      name: 'eng-worg-planner', title: 'worg · Agenda', page: { height: 1100 },
      setup: async ctx => {
        const today = new Date();
        const day = `${today.getFullYear()}-${String(today.getMonth() + 1).padStart(2, '0')}-${String(today.getDate()).padStart(2, '0')}`;
        await ctx.post('plan/block', { Hash: ctx.hashOf('Add retries to the webhook sender'), Date: day, Start: '10:00', End: '11:45' });
        await ctx.post('plan/block', { Hash: ctx.hashOf('Update the API docs'), Date: day, Start: '14:15', End: '15:00' });
      },
      worg: async (page, ctx) => {
        await ctx.openApp(page, 'Agenda');
        await tag(page, [
          { name: 'load', text: 'planned of', contains: true, within: 'main', thenUp: 1 },
          { name: 'block', selector: '[aria-label^="Add retries to the webhook sender,"]' },
          { name: 'place', text: 'To place', within: 'main', closest: '.MuiSheet-root' },
        ]);
        // The hour grid opens on the morning; bring the 10:00 block near its top.
        await page.evaluate(() => {
          const b = document.querySelector('[data-shot="block"]');
          let s = b.parentElement;
          while (s && s.scrollHeight <= s.clientHeight + 4) s = s.parentElement;
          if (s && s !== document.documentElement) s.scrollTop += b.getBoundingClientRect().top - s.getBoundingClientRect().top - 60;
        });
        await sleep(600);
        return mainClip(page, { top: await edge(page, 'load') - 20, bottom: 1100 });
      },
      notes: [
        { at: at('load'), badge: 'tl', title: 'The day\'s load',
          text: 'Planned against the working day, what is free, and how much EFFORT is still waiting for a slot.' },
        { at: at('block'), title: 'A time block',
          text: 'A task dragged onto an hour gets a SCHEDULED time range in its file. Drag the block\'s edge to change how long.' },
        { at: at('place'), title: 'To place',
          text: 'Open tasks without a time, late and soon-due first. Drag one onto the grid, or press Enter for the first free slot.' },
      ],
    },
    // ------------------------------------------------------------ Tuesday
    {
      name: 'eng-worg-focus', title: 'worg · Focus', page: { height: 1000 },
      setup: async ctx => {
        await ctx.post('focus/start', { Hash: ctx.hashOf('Add retries to the webhook sender'), Work: 25, Short: 5, Long: 15, LongEvery: 4 });
        await sleep(1500);
      },
      worg: async (page, ctx) => {
        await ctx.openApp(page, 'Focus');
        await sleep(1000);
        await tag(page, [
          { name: 'ring', selector: 'main [aria-label$=" of 4"]', up: 1 },
          { name: 'controls', text: 'Stop', within: 'main', closest: 'button', thenUp: 1 },
          { name: 'presets', text: '25 / 5', within: 'main', upTo: 'Notify me' },
          { name: 'tasks', selector: 'main input[placeholder="Find a task…"]', closest: '.MuiSheet-root' },
          { name: 'today', text: 'Today', within: 'main', closest: '.MuiSheet-root' },
        ]);
        return mainClip(page, { top: await edge(page, 'tasks') - 12, bottom: await edge(page, 'today', 'bottom') + 30 });
      },
      notes: [
        { at: at('ring'), title: 'The pomodoro',
          text: 'The phase and its countdown. The dots are this one\'s place in the set of four; the fourth earns the long break.' },
        { at: at('controls'), title: 'Its controls',
          text: '<code>Stop</code>, five more minutes, or <code>Done</code> to count it and take the break now. Space, <code>s</code> and <code>+</code> do the same.' },
        { at: at('presets'), title: 'The lengths',
          text: 'Presets or your own, remembered by the server for worg and <code>orgs focus</code> alike. A switch starts the next one when the break ends.' },
        { at: at('tasks'), title: 'What to work on',
          text: 'Open tasks, the ones you worked on lately first. Type to find one, then <code>Start</code>.' },
        { at: at('today'), title: 'Today',
          text: 'Pomodoros and minutes so far, and each session: a full dot for finished, an empty one for stopped early.' },
      ],
    },
    {
      name: 'eng-worg-focus-chip', title: 'worg · the focus chip', page: { height: 700 },
      worg: async (page, ctx) => {
        await ctx.openApp(page, 'Kanban');
        await clickShown(page, 'button[aria-label^="Focus:"]');
        await tag(page, [
          { name: 'chip', selector: 'button[aria-label^="Focus:"]' },
          { name: 'pop', text: 'Open task', closest: '.MuiPopper-root, [role="tooltip"], .MuiSheet-root' },
        ]);
        const c = await clipAround(page, ['chip', 'pop'], 30);
        return { ...c, x: Math.max(0, c.x - 200), width: c.width + Math.min(200, c.x) };
      },
      notes: [
        { at: at('chip'), badge: 'tl', title: 'The chip',
          text: 'On every tab while a pomodoro or a break runs: the ring, the countdown, the task. The browser tab\'s title counts down too.' },
        { at: at('pop'), badge: 'tl', title: 'Without leaving the tab',
          text: 'Stop, five more minutes, done or skip; open the task, or go to the Focus tab.' },
      ],
    },
    {
      name: 'eng-worg-capture', title: 'worg · Capture',
      worg: async (page, ctx) => {
        await ctx.openApp(page, 'Kanban');
        await page.keyboard.press('c');
        await sleep(1000);
        await ctx.clickText(page, 'Interruption', { within: '.MuiModalDialog-root', settle: 1000 });
        await page.type('.MuiModalDialog-root input[placeholder="What is it?"]', 'TODO Look at the flaky export test');
        await tag(page, [{ name: 'who', text: 'Who asked?', within: '.MuiModalDialog-root', up: 1 }]);
        const who = await page.$('[data-shot="who"] input');
        if (who) await who.type('Priya');
        await page.type('.MuiModalDialog-root textarea', 'Fails one run in ten on CI, always in the CSV step.');
        await sleep(900);
        await tag(page, [
          { name: 'dialog', selector: '.MuiModalDialog-root' },
          { name: 'head', selector: '.MuiModalDialog-root input[placeholder="What is it?"]' },
          { name: 'preview', text: 'Goes in as', within: '.MuiModalDialog-root', next: true },
          { name: 'go', text: 'Capture', within: '.MuiModalDialog-root', closest: 'button', last: true },
        ]);
        return clipAround(page, ['dialog'], 24);
      },
      notes: [
        { at: at('head'), title: 'What it is',
          text: 'The headline, keyword and all, and its tags on the line under it.' },
        { at: at('who'), title: 'The template\'s own questions',
          text: 'Each <code>{{name|question}}</code> in the template is a field; <code>{{now}}</code> and the like fill themselves in.' },
        { at: at('preview'), title: 'What gets written',
          text: 'The entry as it will land in <code>inbox.org</code>, updated as you type.' },
        { at: at('go'), title: 'Back to work',
          text: '<code>Capture</code> (or Ctrl-Enter) files it and closes. The pomodoro never stopped.' },
      ],
    },
    // ---------------------------------------------------------- Wednesday
    {
      name: 'eng-worg-refactor', title: 'worg · Refactor', page: { height: 1150 },
      worg: async (page, ctx) => {
        await ctx.openApp(page, 'Refactor');
        await ctx.clickText(page, 'List to headings', { within: 'main', settle: 900 });
        await page.type('input[placeholder="Find the heading whose list it is"]', 'retries webhook');
        await sleep(1500);
        await page.keyboard.press('Enter');
        await sleep(1200);
        await ctx.clickText(page, 'Preview', { within: 'main', settle: 2000 });
        await toTop(page);
        await tag(page, [
          { name: 'ops', text: 'List to headings', within: 'main', closest: '[role="tablist"]' },
          { name: 'heading', text: 'Heading', within: 'main', closest: '.MuiFormControl-root' },
          { name: 'diff', text: 'headings added', contains: true, within: 'main', upTo: 'in 1 place' },
          { name: 'end', text: 'unchanged lines', contains: true, within: 'main', last: true },
        ]);
        // The diff box is as tall as the window, whatever it holds: stop the
        // picture where the diff does.
        return mainClip(page, { top: await edge(page, 'ops') - 20, bottom: await edge(page, 'end', 'bottom') + 16 });
      },
      notes: [
        { at: at('ops'), title: 'Four refactors',
          text: 'Rename or merge a tag everywhere, split a subtree into its own file, merge two headings, or turn a list into headings.' },
        { at: at('heading'), title: 'Which heading',
          text: 'Found by typing a few letters of it. A heading with more than one list asks which.' },
        { at: at('diff'), title: 'The preview',
          text: 'What will change, and below it the diff, before anything is written: each item of the list becomes a child heading.' },
        { title: 'Apply, or not',
          text: 'Under the diff, <code>Discard</code> or <code>Apply to 1 file</code>. Nothing changes until you apply, and the Diff &amp; History tab can take it back after.' },
      ],
    },
    {
      name: 'eng-worg-deps', title: 'worg · Insights', page: { height: 1100 },
      worg: async (page, ctx) => {
        await ctx.openApp(page, 'Insights');
        await ctx.clickText(page, 'Dependencies', { within: 'main', settle: 2000 });
        await tag(page, [
          { name: 'waiting', text: 'is WAITING, but', contains: true, within: 'main', closest: '.MuiSheet-root, .MuiCard-root' },
          { name: 'chain', text: 'headings held up behind', contains: true, within: 'main', closest: '.MuiSheet-root, .MuiCard-root' },
        ]);
        // Open both problems, for what they say about themselves.
        for (const t of ['is WAITING, but', 'headings held up behind']) {
          await page.evaluate(t => {
            const el = [...document.querySelectorAll('main *')].find(e => e.textContent.includes(t) && ![...e.children].some(c => c.textContent.includes(t)));
            el?.click();
          }, t);
          await sleep(700);
        }
        await toTop(page);
        await tag(page, [
          { name: 'tabs', text: 'Estimates', within: 'main', up: 1 },
          { name: 'controls', text: 'Only problems touching', within: 'main', closest: '.MuiSheet-root' },
          { name: 'summary', text: 'headings checked', contains: true, within: 'main', up: 1 },
        ]);
        return mainClip(page, { top: await edge(page, 'tabs') - 16, bottom: await edge(page, 'chain', 'bottom') + 30 });
      },
      notes: [
        { at: at('controls'), title: 'The check',
          text: 'Every AFTER and ORDERED link in your notes, checked; narrow it to the problems touching a query.' },
        { at: at('summary'), title: 'What it found',
          text: 'A chip for each kind of problem, and how many headings it looked at.' },
        { at: at('waiting'), title: 'Waiting on done',
          text: 'A WAITING task whose AFTER is already finished: nothing holds it any more. Open it to unlink it in one click.' },
        { at: at('chain'), title: 'A blocked chain',
          text: 'Work queued behind one unfinished task: here an ORDERED project, behind the component library.' },
      ],
    },
    // ----------------------------------------------------------- Thursday
    {
      name: 'eng-worg-bell', title: 'worg · notifications', page: { height: 700 },
      setup: async ctx => {
        await ctx.post('watches', { name: 'Blocked', storedQuery: 'Blocked' });
        await sleep(500);
        ctx.cli('todo', '-yes', '-hash', ctx.hashOf('Tidy the on-call runbook'), 'WAITING');
        for (let i = 0; i < 40; i++) {
          const n = await fetch(`${ctx.base}/notifications?unread=t`).then(r => r.json());
          if (n.unread) break;
          await sleep(250);
        }
      },
      worg: async (page, ctx) => {
        await ctx.openApp(page, 'StoredQueries');
        await clickShown(page, 'button[aria-label*="notification"]');
        await tag(page, [
          { name: 'bell', selector: 'button[aria-label*="unread notification"]' },
          { name: 'pop', text: 'Manage watches', closest: '.MuiPopper-root, [role="tooltip"], .MuiSheet-root' },
        ]);
        const c = await clipAround(page, ['bell', 'pop'], 30);
        return { ...c, x: Math.max(0, c.x - 120), width: c.width + Math.min(120, c.x) };
      },
      notes: [
        { at: at('bell'), badge: 'tl', title: 'The bell',
          text: 'Appears once you watch a saved query, and counts what the watches found that you haven\'t read.' },
        { at: at('pop'), badge: 'tl', title: 'What changed',
          text: 'Each notice says which watch, what came into it (and what left, if you asked), and when. Watch a saved query from the bell on its row in Saved Queries.' },
      ],
    },
    // ------------------------------------------------------------- Friday
    {
      name: 'eng-worg-inbox', title: 'worg · Inbox',
      worg: async (page, ctx) => {
        await ctx.openApp(page, 'Inbox');
        await sleep(800);
        await tag(page, [
          { name: 'mode', text: 'Someday sweep', within: 'main', closest: '[role="group"]' },
          { name: 'card', text: 'Call the plumber about the kitchen tap', within: 'main', closest: '.MuiSheet-variantOutlined' },
          { name: 'keys', text: 'rrefile', within: 'main', up: 1 },
        ]);
        return mainClip(page, { top: await edge(page, 'mode') - 30, bottom: await edge(page, 'keys', 'bottom') + 40 });
      },
      notes: [
        { at: at('mode'), title: 'Inbox, or someday',
          text: 'The inbox, or the someday pile to sweep: keep, promote, schedule or drop.' },
        { at: at('card'), title: 'One item',
          text: 'Where it lives, its keyword and headline, and what it has already: tags, a date, an effort. Above it, how far through the pile you are.' },
        { at: at('keys'), badge: 'r', title: 'One key each',
          text: '<code>r</code> refile, <code>s</code> schedule (in words: fri, +2w), <code>t</code> tag, <code>a</code> archive, <code>d</code> done, <code>n</code> make it NEXT. <code>u</code> takes the last one back.' },
      ],
    },
  ],
};
