// The pictures of what v0.6.0 and v0.7.0 added, for docs/cli.org, docs/worg.org
// and the guides of each feature: checklists, agenda views, the relative
// timer, hooks, transclusion, CalDAV/CardDAV, org-protocol and citations. The
// demo notebook with a few files of its own on top (docs/screenshots/
// whatsnew): checklists, a meeting page that transcludes the design notes,
// and a short paper with a bibliography.
//
//   SHOT_PORT=8102 node capture.mjs -story whatsnew
//   SHOT_PORT=8102 node capture.mjs -story whatsnew cli-list

import http from 'node:http';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { at, tag, clipAround, mainClip, toTop } from './engineer-worg.mjs';

const here = path.dirname(fileURLToPath(import.meta.url));
const shots = path.resolve(here, '../../../docs/screenshots');

// Where the work-to-chat webhook posts: a listener started by the hooks
// shot, standing in for a chat service, so its runs show as delivered.
const HOOK_PORT = Number(process.env.HOOK_PORT || 8199);

const server = `  captureTemplates:
    - name: "Web"
      type: "entry"
      target:
        type: "file"
        filename: "inbox.org"
      template: |-
        :PROPERTIES:
        :SOURCE: {{url}}
        :END:
        {{CONTENT}}
  agenda:
    views:
      - name: Work day
        key: w
        sections:
          - {kind: agenda, title: Today at work, days: 1, query: 'HasTags("work") || InFile("work.org")'}
          - {kind: todo, title: Next and waiting, keywords: "NEXT|WAITING|INPROGRESS"}
          - {kind: tags, title: Blocked, query: 'TODO="BLOCKED"'}
          - {kind: stuck, title: Stuck projects}
  transclusion:
    enable: true
  dav:
    enable: true
  citations:
    export: "basic author-year"
  hooks:
    enable: true
    rules:
      - name: done-log
        on: [statechange]
        match: {to: "DONE|CANCELLED"}
        run: ["logger", "-t", "orgs", "done: {{headline}}"]
      - name: work-to-chat
        on: [statechange, capture]
        match: {tags: work}
        url: http://localhost:${HOOK_PORT}/hooks/orgs
      - name: overdue
        on: [deadline]
        shell: 'logger -t orgs "$ORGS_HEADLINE is overdue"'
`;

export const demo = { src: [path.join(shots, 'demo'), path.join(shots, 'whatsnew')], server };

const sleep = ms => new Promise(r => setTimeout(r, ms));
let hookListener = null;
function listen() {
  if (hookListener) return;
  hookListener = http.createServer((req, res) => { req.resume(); req.on('end', () => res.end('ok')); });
  hookListener.listen(HOOK_PORT);
  hookListener.unref();
}

// A finished task in work.org: done-log runs it, and work-to-chat posts it.
async function hookRuns(ctx) {
  listen();
  ctx.cli('todo', '-yes', '-hash', ctx.hashOf('Renew the TLS certificates'), 'DONE');
  for (let i = 0; i < 40; i++) {
    const runs = await fetch(`${ctx.base}/hooks/runs`).then(r => r.json()).catch(() => null);
    if (Array.isArray(runs) && runs.length >= 2) return;
    await sleep(250);
  }
}

// Two devices, as somebody with a phone and a tablet would have.
function davDevices(ctx) {
  const st = JSON.parse(ctx.cli('dav', '-json'));
  if ((st.Passwords || st.AppPasswords || []).length) return;
  ctx.cli('dav', 'password', 'phone');
  ctx.cli('dav', 'password', 'tablet');
}

// A meeting under way: clocked in, the timer running for a while, a note taken.
function meeting(ctx) {
  ctx.cli('clockin', '-hash', ctx.hashOf('Content review with marketing'));
  ctx.cli('timer', 'start');
  ctx.cli('timer', 'set', '0:18:05');
  ctx.cli('timer', 'note', 'blog moves after launch, not before');
}

// The rendered page lives in an iframe, so a note can only point at the
// frame; the clip is the main column.
async function filePage(page, ctx, file) {
  await ctx.openApp(page, 'Files');
  await ctx.clickText(page, file, { settle: 2500 });
  await toTop(page);
}

const PAGE = 'https://bradfrost.com/blog/post/atomic-web-design/';

export default {
  demo,
  shots: [
    { name: 'cli-list',
      cli: { title: 'orgs list ls', args: ['list', 'ls'], opts: { cols: 84, rows: 37 } },
      notes: [
        { find: 'Groceries [2/5]', title: 'A list',
          text: 'Any heading with <code>- [ ]</code> boxes in its own text, and how many are ticked. <code>orgs list new</code> files new ones under <em>Checklists</em> in the inbox.' },
        { find: 'Build the component library [2/4]', title: 'Found wherever it is',
          text: 'Boxes under a task in <code>work.org</code> are a list too, so refiling a list never loses it.' },
        { find: '3. ☐  Sourdough', title: 'Numbered items',
          text: 'The numbers <code>orgs list check -l groceries 3</code>, <code>edit</code> and <code>rm</code> take. Each change is a one-line edit to the file.' },
        { find: '% of all tasks complete', title: 'The tally',
          text: 'As taskbook ends its board. <code>-open</code> leaves out the finished lists.' },
      ] },
    { name: 'cli-agenda-view',
      cli: { title: 'orgs agenda -view w', args: ['agenda', '-view', 'w'], opts: { cols: 92, rows: 23 } },
      notes: [
        { find: 'Today at work', word: true, title: 'An agenda section',
          text: 'One day of the agenda, narrowed by a query to work: org\'s block agendas, from <code>agenda: views:</code> in the yaml.' },
        { find: 'Next and waiting', word: true, title: 'A todo section',
          text: 'Open tasks with these keywords (<code>keywords: NEXT|WAITING|INPROGRESS</code>).' },
        { find: 'Blocked', word: true, title: 'A match',
          text: 'Any query: an org match string (<code>TODO="BLOCKED"</code>), a short query or a full one.' },
        { find: 'Stuck projects', word: true, title: 'Stuck projects',
          text: 'Projects with nothing marked next. <code>orgs agenda -views</code> lists every view; <code>-as html</code> or <code>-as ics</code> prints it for a page or a calendar.' },
      ] },
    { name: 'cli-dav', setup: davDevices,
      cli: { title: 'orgs dav', args: ['dav'], opts: { cols: 84, rows: 10 } },
      notes: [
        { find: '  calendar  http', title: 'The addresses',
          text: 'Your dated headings as a calendar, your contacts as an address book, for the phone\'s own apps.' },
        { find: 'phones must use', title: 'https only',
          text: 'Over plain http only from this machine, unless <code>dav.allowHttp</code> says otherwise.' },
        { find: 'App passwords:', word: true, title: 'One password per device',
          text: '<code>orgs dav password phone</code> makes one, shown once; <code>orgs dav revoke</code> cuts a device off. Your login password never works here.' },
      ] },
    {
      name: 'worg-agenda-views', title: 'worg · Agenda views', page: { height: 1300 },
      worg: async (page, ctx) => {
        await ctx.openApp(page, 'Agenda');
        await ctx.choose(page, 'Agenda views…', 'w');
        await sleep(1500);
        await toTop(page);
        await tag(page, [
          { name: 'pick', selector: 'main .MuiSelect-root' },
          { name: 'day', text: 'Today at work', within: 'main' },
          { name: 'next', text: 'Next and waiting', within: 'main' },
          { name: 'stuck', text: 'Stuck projects', within: 'main' },
          { name: 'sheet', text: 'Today at work', within: 'main', closest: '.MuiSheet-root' },
        ]);
        return clipAround(page, ['pick', 'sheet'], 24);
      },
      notes: [
        { at: at('pick'), badge: 'r', title: 'The views',
          text: 'Org\'s own (<code>a</code> the week, <code>t</code> open tasks, <code>n</code> both, <code>#</code> stuck projects), the yaml\'s, and your own. Above the calendar on the Agenda tab.' },
        { at: at('day'), badge: 'r', title: 'Sections, one after another',
          text: 'Here a day of work, then the open NEXT and WAITING tasks, then a match. Click an item to open it.' },
        { at: at('stuck'), badge: 'r', title: 'Stuck projects',
          text: 'Projects with no next action, as org\'s <code>#</code> finds them.' },
      ],
    },
    {
      name: 'worg-transclusion', title: 'worg · Files · a transcluding page', page: { height: 1240 },
      worg: async (page, ctx) => { await filePage(page, ctx, 'meetings.org'); return mainClip(page); },
      notes: [
        { title: 'The text of another file',
          text: '<code>#+transclude: [[file:notes.org::*Colour tokens]]</code> draws that heading here, in worg and in every export; the file keeps the one line.' },
        { title: 'Where it lives',
          text: 'Each transclusion has a bar naming its source. Its edit button opens the source\'s lines, and the edit is written back there.' },
        { title: 'Always current',
          text: 'Change the design notes and this page draws again. Turned on in the settings menu, or with <code>orgs transclude on</code>.' },
      ],
    },
    {
      name: 'worg-timer', title: 'worg · Focus · the timer', setup: meeting, page: { height: 1000 },
      worg: async (page, ctx) => {
        await ctx.openApp(page, 'Focus');
        await tag(page, [{ name: 'card', text: 'Timer', within: 'main', closest: '.MuiSheet-root' }]);
        await page.evaluate(() => document.querySelector('[data-shot="card"]').scrollIntoView({ block: 'center' }));
        await page.type('[data-shot="card"] input', 'Maya takes the navigation bar');
        await page.keyboard.press('Enter');
        await sleep(1200);
        await tag(page, [
          { name: 'time', text: 'Timer', within: '[data-shot="card"]', next: true },
          { name: 'noted', text: 'Noted:', contains: true, within: '[data-shot="card"]' },
        ]);
        return clipAround(page, ['card'], 24);
      },
      notes: [
        { at: at('time'), badge: 'r', title: 'The relative timer',
          text: 'Org\'s <code>org-timer</code>: time since it started, or a count down. The server keeps it, so <code>orgs timer</code> shows the same.' },
        { at: at('noted'), badge: 'r', title: 'A note stamped with it',
          text: 'Written under the heading you are clocked into as <code>- 0:18:22 :: …</code>, the way <code>org-timer-item</code> writes meeting notes.' },
      ],
    },
    {
      name: 'worg-protocol', title: 'worg · capture from org-protocol',
      worg: async (page, ctx) => {
        const q = new URLSearchParams({ protocol: 'capture', template: 'Web', url: PAGE, title: 'Atomic Web Design',
          body: 'We\'re not designing pages, we\'re designing systems of components.' });
        await page.goto(`${ctx.base}/?${q}`, { waitUntil: 'load' });
        await sleep(3000);
        await tag(page, [{ name: 'dialog', selector: '.MuiModalDialog-root' }]);
        return clipAround(page, ['dialog'], 24);
      },
      notes: [
        { title: 'From the page you were reading',
          text: 'A bookmarklet (or an <code>org-protocol://capture</code> link, with <code>orgs protocol install</code>) opens worg\'s capture with the Web template.' },
        { title: 'Filled in',
          text: 'The title is the headline, <code>{{url}}</code> lands in <code>:SOURCE:</code>, the selected text goes in the body.' },
        { title: 'Nothing filed unseen',
          text: 'Any web page can open such a link, so a capture always waits for you to press Capture.' },
      ],
    },
    {
      name: 'worg-citations', title: 'worg · Files · citations',
      worg: async (page, ctx) => { await filePage(page, ctx, 'paper.org'); return mainClip(page); },
      notes: [
        { title: 'Citations',
          text: '<code>[cite:@ousterhout2018]</code> keys from <code>refs.bib</code>, set out by <code>#+cite_export: basic author-year</code>.' },
        { title: 'The bibliography',
          text: '<code>#+print_bibliography:</code> lists the works cited, in every export.' },
      ],
    },
    // Last: it marks a task done, which the other pictures should not show.
    { name: 'cli-hooks', setup: hookRuns,
      cli: { title: 'orgs hooks', args: ['hooks'], opts: { cols: 118, rows: 11 } },
      notes: [
        { find: 'done-log', word: true, title: 'A rule',
          text: 'Its name, the events it waits for and what must match, then what it runs: a program, a shell line or a webhook.' },
        { find: '    POST http', title: 'A webhook',
          text: 'The event is posted as json, signed when the rule has a <code>secret</code>.' },
        { find: 'Runs:', word: true, title: 'The last runs',
          text: 'What ran, on which event, how long it took and whether it worked. <code>orgs hooks test done-log to=DONE</code> runs one on a made-up event.' },
      ] },
  ],
};
