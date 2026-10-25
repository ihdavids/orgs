// The pictures for docs/story-engineer-cli.org: an engineer's week at the
// command line, GTD with pomodoros. Its own notebook (docs/screenshots/
// engineer-cli): Marcus's projects, a week of pomodoro-sized clock lines,
// finished tasks with estimates, habits, an inbox, and an inbox capture
// template that files at the top of inbox.org, where triage looks.
//
//   SHOT_PORT=8101 node capture.mjs -story engineer-cli
//   SHOT_PORT=8101 node capture.mjs -story engineer-cli eng-cli-focus

import path from 'node:path';
import { fileURLToPath } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const src = path.resolve(here, '../../../docs/screenshots/engineer-cli');

const server = `  captureTemplates:
    - name: "Inbox"
      type: "entry"
      target:
        type: "file"
        filename: "inbox.org"
      template: |-
        :PROPERTIES:
        :CREATED: {{now}}
        :END:
        {{CONTENT}}
`;

const sleep = ms => new Promise(r => setTimeout(r, ms));

// A pomodoro on the idempotency work, unless one is running already.
async function focusOn(ctx, headline = 'Implement idempotency keys') {
  const st = await fetch(`${ctx.base}/focus`).then(r => r.json());
  if (st.Phase === 'work') return;
  await ctx.post('focus/start', { Hash: ctx.hashOf(headline), Work: 25, Short: 5, Long: 15, LongEvery: 4 });
}

// Today's plan, made the way the guide says: the idempotency work in the
// morning, the take-home after lunch.
function blockDay(ctx) {
  ctx.cli('plan', 'idempotency', '10:00-12:00', '-yes');
  ctx.cli('plan', 'take-home', '15:00-16:00', '-yes');
}

// Priya grants the replica access: the waiting item leaves the watched query.
async function watchFires(ctx) {
  ctx.cli('q', 'save', 'waiting', 'IsStatus("WAITING") || IsStatus("BLOCKED")');
  ctx.cli('q', 'watch', 'waiting', '-left');
  ctx.cli('todo', '-yes', '-hash', ctx.hashOf('Read access to the production replica'), 'NEXT');
  for (let i = 0; i < 40; i++) {
    const n = await fetch(`${ctx.base}/notifications?unread=t`).then(r => r.json());
    if (n.unread) return;
    await sleep(250);
  }
  throw new Error('the watch never fired');
}

const wait = ms => [[ms, '']];

export default {
  demo: { src, extensions: path.join(src, 'extensions.yaml'), server },
  shots: [
    { name: 'eng-cli-review-week',
      // Straight to step 2: step 1 is the triage screen, pictured on its own.
      setup: ctx => ctx.post('review/step', { Step: 2, Tally: { refiled: 3, scheduled: 1 } }),
      cli: { title: 'orgs review week', args: ['review', 'week'], opts: { cols: 100, rows: 22, keys: [[2500, ' '], [2000, '']] } },
      notes: [
        { find: 'Projects with no next action', word: true, title: 'Which step',
          text: 'Step 2 of the seven: projects with no next action. <code>1 of 1</code> on the right is how far through it you are.' },
        { find: '  Developer portal', title: 'The project',
          text: 'A heading tagged <code>:project:</code> with nothing marked <code>NEXT</code> under it, with its file and tags.' },
        { find: ' 1  TODO ', title: 'Its open tasks, numbered',
          text: 'Press a number and that task becomes the project\'s <code>NEXT</code> action. Usually that is the whole decision.' },
        { find: 'Nobody has said', title: 'Its notes',
          text: 'The start of the project\'s own text, to remind you what it was for.' },
        { find: '1-9 make', title: 'The keys',
          text: '<code>a</code> writes a new next action, <code>u</code> takes the last decision back, <code>n</code> leaves it for now, <code>q</code> stops; the review resumes at this step next time, here or in worg.' },
      ] },
    { name: 'eng-cli-triage',
      cli: { title: 'orgs triage', args: ['triage'], opts: { cols: 100, rows: 12, keys: wait(2500) } },
      notes: [
        { find: '  Inbox', word: true, title: 'The inbox, one item at a time',
          text: 'Everything at the top of <code>inbox.org</code>; on the right, which item this is and how many are left.' },
        { find: 'TODO Ask Priya', title: 'The item',
          text: 'One captured thought. Decide what it is now, while it is the only thing on the screen.' },
        { find: '  inbox.org', word: true, title: 'Where it lives',
          text: 'Its file, and its tags once it has some.' },
        { find: 'r refile', title: 'One key per decision',
          text: '<code>r</code> refile under a project, <code>s</code> schedule (<code>fri</code>, <code>+2w</code>), <code>t</code> tag, <code>a</code> archive, <code>d</code> done (it took two minutes), <code>n</code> next. <code>u</code> undoes the last one.' },
      ] },
    { name: 'eng-cli-plan', setup: blockDay,
      cli: { title: 'orgs plan', args: ['plan'], opts: { cols: 100, rows: 34 } },
      notes: [
        { find: 'planned · ', title: 'The day as a bar',
          text: 'The working hours, a character a quarter hour: blue is booked, grey is free. On the right, how much of the day is planned and how much is left.' },
        { find: 'TODO Standup', title: 'Appointments',
          text: 'Meetings and anything else with a time, fixed where they are.' },
        { find: '10:00-12:00', title: 'A block',
          text: '<code>orgs plan idempotency 10:00-12:00</code> wrote this: a time range on the task\'s <code>SCHEDULED</code>, so Emacs and every agenda see the plan.' },
        { find: 'free 12:00-14:00', title: 'The gaps',
          text: 'Free time between blocks, with its length: where the next task could go (<code>orgs plan &lt;task&gt; next</code> picks the first that fits).' },
        { find: 'deadline in 1d', title: 'All day',
          text: 'Deadlines and scheduled things with no time, habits among them.' },
        { find: 'To place (', title: 'What is left to place',
          text: 'Open tasks with no block today, the most urgent first: late, then due soonest. The second column is each one\'s <code>EFFORT</code>.' },
      ] },
    { name: 'eng-cli-focus', setup: focusOn,
      cli: { title: 'orgs focus', args: ['focus'], opts: { cols: 100, rows: 4, keys: wait(3000) } },
      notes: [
        { find: 'Implement idempotency keys work.org', title: 'The task',
          text: 'What this pomodoro is for, and its file. The server clocked in on it when the session started.' },
        { find: '🍅 ', word: true, title: 'The countdown',
          text: '<code>🍅</code> and the time left while working (<code>☕</code> on a break), then the pomodoro as a bar filling as it runs. The server keeps the time, so closing this window does not stop it.' },
        { find: 'q stop', title: 'The keys',
          text: '<code>q</code> stops early (clocked, not counted), <code>+</code> adds five minutes, <code>n</code> finishes now (counted), <code>d</code> leaves the timer running and gives the terminal back.' },
      ] },
    { name: 'eng-cli-cap',
      cli: { title: 'orgs cap Inbox', args: ['cap', 'Inbox'], opts: { cols: 96, rows: 12,
        keys: [[2500, 'TODO Check why the nightly backup job paged at 3am'], [500, '\t'], [400, 'ops'], [700, '']] } },
      notes: [
        { find: '3am  ops', word: true, title: 'The headline',
          text: 'Keyword and title, with the tags typed so far on the right.' },
        { find: ':PROPERTIES:', word: true, title: 'The template',
          text: 'The rest of the Inbox template: a drawer whose <code>:CREATED:</code> stamp is the server\'s answer to <code>{{now}}</code>, still editable.' },
        { find: '{{CONTENT}}', word: true, title: 'The body',
          text: 'Optional. Most captures are one line: get it down and get back to the timer.' },
        { find: 'Tags · 2 of 4', title: 'Which box',
          text: 'The field the cursor is in, and how many the template has.' },
        { find: 'tab next', title: 'The keys',
          text: '<code>tab</code> moves on, <code>ctrl-s</code> files it in <code>inbox.org</code>, <code>esc</code> throws it away.' },
      ] },
    { name: 'eng-cli-notifications', setup: watchFires,
      cli: { title: 'orgs notifications', args: ['notifications'], opts: { cols: 90, rows: 8 } },
      notes: [
        { find: '1 now', word: true, title: 'Which watch',
          text: 'The saved query that found something (<code>orgs q watch waiting -left</code>), when, and how many it matches now. <code>●</code> is unread.' },
        { find: '- WAITING', title: 'What changed',
          text: '<code>+</code> a heading that started matching, <code>-</code> one that stopped (asked for with <code>-left</code>): here, access to the replica arrived.' },
        { find: 'unread of', title: 'The count',
          text: '<code>-read</code> marks them read, <code>-follow</code> prints each one as it happens.' },
      ] },
    { name: 'eng-cli-deps',
      cli: { title: 'orgs deps', args: ['deps'], opts: { cols: 132, rows: 9 } },
      notes: [
        { find: 'held up behind', title: 'The kind of problem',
          text: 'Here a blocked chain; also cycles, waiting on something done, an <code>AFTER</code> naming nothing, a stuck <code>ORDERED</code> project.' },
        { find: 'keys → Load', title: 'The chain',
          text: 'Each task waits on the one before it (<code>AFTER</code> or an <code>ORDERED</code> parent).' },
        { find: 'Everything here waits', title: 'What it means',
          text: 'And what would fix it: finish the first task, or split the chain.' },
        { find: '1 problem', title: 'The summary',
          text: 'Problems by kind, and how many headings were checked. <code>-check</code> exits 1 on an error, for a git hook.' },
      ] },
    { name: 'eng-cli-estimates',
      cli: { title: 'orgs estimates -day 8', args: ['estimates', '-day', '8'], opts: { cols: 100, rows: 24 } },
      notes: [
        { find: 'Estimates ·', title: 'What is measured',
          text: 'Every finished task with an <code>EFFORT</code> and clocked time, grouped by tag. <code>-day 8</code> reads a day of effort as a working day.' },
        { find: 'tasks  estimate', title: 'The columns',
          text: 'Estimated against actual time, the ratio of the totals, the median ratio per task and its spread; the bar draws actual over estimate.' },
        { find: '  all ', title: 'Everything together',
          text: 'Tag rows can share a task, so they need not add up to this line.' },
        { find: 'Work takes', title: 'The verdict',
          text: 'Once there are enough tasks to say: here, work takes 0.6× the estimate.' },
        { find: 'Worst over-runs', title: 'The outliers',
          text: 'The tasks furthest off in minutes, both ways, with their file and line.' },
      ] },
  ],
};
