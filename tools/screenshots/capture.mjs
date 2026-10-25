// Captures the documentation screenshots: the orgs command line and worg,
// both run against the demo notebook in docs/screenshots/demo so no real
// notes end up in the docs.
//
//   cd tools/screenshots && npm install
//   node capture.mjs            # every shot
//   node capture.mjs cli-agenda # only the shots named
//   node capture.mjs serve      # just run the demo server, to look around
//
// The demo files carry their dates as {{D+3}} (a date with its weekday),
// {{ISO-2}} (a bare date) and {{MD+9}} (month and day, for birthdays), so the
// agenda and the habits are always about today, whenever this runs.
//
// The server is a throwaway: port 8099, noAuth, its own config and org
// directory in a temp dir. It runs from the repo root because the template
// manager resolves templates/ against the working directory.

import { spawn, execFileSync } from 'node:child_process';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const repo = path.resolve(here, '../..');
const demoSrc = path.join(repo, 'docs/screenshots/demo');
const outDir = path.join(repo, 'docs/images');
const PORT = Number(process.env.SHOT_PORT || 8099);
const BASE = `http://localhost:${PORT}`;
const CHROME = process.env.CHROME ||
  '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome';

// ---------------------------------------------------------------- demo data

const DAYS = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];
const pad = n => String(n).padStart(2, '0');
function dayOffset(n) {
  const d = new Date();
  d.setHours(12, 0, 0, 0);
  d.setDate(d.getDate() + n);
  return d;
}
const iso = d => `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;

export function expandDates(text) {
  return text.replace(/\{\{(D|ISO|MD)([+-]\d+)\}\}/g, (_, kind, off) => {
    const d = dayOffset(Number(off));
    if (kind === 'ISO') return iso(d);
    if (kind === 'MD') return `${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
    return `${iso(d)} ${DAYS[d.getDay()]}`;
  });
}

// A story (stories/*.mjs) may bring a notebook of its own (src), its own
// saved queries and settings (extensions) and more server settings (server,
// yaml indented two, spliced under server:), and files written into the run's
// folder before the server starts ({ 'focus_data.json': '...' }: state the
// server only reads at startup).
function setupDemo({ src = demoSrc, extensions = path.join(demoSrc, '../extensions.yaml'), server = '', files = {} } = {}) {
  const work = fs.mkdtempSync(path.join(os.tmpdir(), 'orgs-shots-'));
  const org = path.join(work, 'org');
  fs.mkdirSync(org);
  // Subfolders too, so a notebook can have a manuscript/ of chapters.
  const copy = (from, to) => {
    for (const f of fs.readdirSync(from, { withFileTypes: true })) {
      if (f.isDirectory()) {
        fs.mkdirSync(path.join(to, f.name), { recursive: true });
        copy(path.join(from, f.name), path.join(to, f.name));
      } else if (f.name.endsWith('.org')) {
        fs.writeFileSync(path.join(to, f.name), expandDates(fs.readFileSync(path.join(from, f.name), 'utf8')));
      }
    }
  };
  copy(src, org);
  fs.mkdirSync(path.join(org, 'worklog'), { recursive: true });
  // Saved queries, the kanban board and the flashcard deck. The server keeps
  // these beside its config file.
  fs.writeFileSync(path.join(work, 'orgs_extensions.yaml'), expandDates(fs.readFileSync(extensions, 'utf8')));
  const config = path.join(work, 'orgs.yaml');
  fs.writeFileSync(config, `server:
  orgDirs:
    - "${org}"
  orgSalt: "screenshots"
  port: ${PORT}
  allowHttp: true
  allowHttps: false
  noAuth: true
  useProjectTag: true
  defaultTodoStates: "TODO NEXT INPROGRESS WAITING BLOCKED | DONE CANCELLED"
  templatePath: "./templates"
  dayPagePath: "${org}/worklog"
  dayPageTemplate: "daypage.tpl"
  refileTargets:
    - ".*\\\\.org"
  exporters:
    - name: "gantt"
    - name: "mermaid"
    - name: "mindmap"
    - name: "html"
${server}url: "${BASE}"
`);
  for (const [rel, content] of Object.entries(files)) fs.writeFileSync(path.join(work, rel), content);
  return { work, org, config };
}

// ------------------------------------------------------------------- server

function buildOrgs(work) {
  // In a folder of its own: the server keeps some state (watch notices) in
  // the folder above its binary's, which should be this run's, not the
  // temp dir every run shares.
  const bin = path.join(work, 'bin', process.platform === 'win32' ? 'orgs.exe' : 'orgs');
  execFileSync('go', ['build', '-o', bin, './cmd/orgs'], { cwd: repo, stdio: 'inherit' });
  return bin;
}

async function waitFor(url, ms = 20000) {
  const end = Date.now() + ms;
  while (Date.now() < end) {
    try { if ((await fetch(url)).status < 500) return; } catch { }
    await new Promise(r => setTimeout(r, 250));
  }
  throw new Error(`server did not answer at ${url}`);
}

async function startServer(bin, config, work) {
  const log = fs.openSync(path.join(work, 'server.log'), 'w');
  const proc = spawn(bin, ['-config', config, 'serve', '-port', String(PORT)],
    { cwd: repo, stdio: ['ignore', log, log] });
  proc.on('exit', code => { if (code) console.error(`server exited ${code}; see ${work}/server.log`); });
  await waitFor(`${BASE}/`);
  return proc;
}

// ---------------------------------------------------------------- terminal

// Runs one command in a pseudo terminal, so it draws as it would for a
// person (colour, width, TUIs), and returns the screen as html. keys are
// [delayMs, text] pairs sent after the program starts; settle is how long to
// wait after the last one before reading the screen.
async function terminal(bin, config, args, { cols = 100, rows = 30, keys = [], settle = 1500, env = {} } = {}) {
  const pty = (await import('node-pty')).default;
  const xterm = await import('@xterm/headless');
  const { Terminal } = xterm.Terminal ? xterm : xterm.default;
  const ser_ = await import('@xterm/addon-serialize');
  const { SerializeAddon } = ser_.SerializeAddon ? ser_ : ser_.default;
  const term = new Terminal({ cols, rows, allowProposedApi: true, theme: THEME });
  const ser = new SerializeAddon();
  term.loadAddon(ser);
  // stderr goes to a file: every run logs its config load there, and a
  // screenshot of the answer should not open on two log lines.
  const errLog = path.join(path.dirname(config), 'cli-stderr.log');
  const p = pty.spawn('/bin/sh', ['-c', `exec "$0" "$@" 2>>'${errLog}'`, bin, '-config', config, ...args], {
    name: 'xterm-256color', cols, rows, cwd: repo,
    env: { ...process.env, TERM: 'xterm-256color', COLORTERM: 'truecolor', NO_COLOR: '', ...env },
  });
  let done = false;
  p.onData(d => term.write(d));
  p.onExit(() => { done = true; });
  const sleep = ms => new Promise(r => setTimeout(r, ms));
  for (const [delay, text] of keys) { await sleep(delay); if (!done) p.write(text); }
  const end = Date.now() + settle + 15000;
  if (keys.length) await sleep(settle);
  else while (!done && Date.now() < end) await sleep(100);
  await sleep(200);
  await new Promise(r => term.write('', r));
  const html = ser.serializeAsHTML({ onlySelection: false, includeGlobalBackground: true });
  if (!done) p.kill();
  return html;
}

// One Dark, more or less. Black is the window itself: tview paints "black"
// behind everything it draws, and meant the terminal's own background.
const BG = '#1e2127';
const THEME = {
  background: BG, foreground: '#d7dae0', cursor: BG,
  black: BG, red: '#e06c75', green: '#98c379', yellow: '#e5c07b', blue: '#61afef',
  magenta: '#c678dd', cyan: '#56b6c2', white: '#d7dae0',
  brightBlack: '#5c6370', brightRed: '#ef8189', brightGreen: '#a9d48a', brightYellow: '#f0d08e',
  brightBlue: '#7cc0f7', brightMagenta: '#d68fe8', brightCyan: '#6cc9d4', brightWhite: '#ffffff',
};

// The serialized screen inside a window frame, sized to its content.
function terminalPage(title, body) {
  return `<!doctype html><html><head><meta charset="utf-8"><style>
  html,body{margin:0;background:transparent}
  .win{display:inline-block;margin:24px;border-radius:10px;overflow:hidden;
    box-shadow:0 12px 32px rgba(0,0,0,.28),0 0 0 1px rgba(0,0,0,.25);background:${BG}}
  .bar{height:30px;background:#2c313a;display:flex;align-items:center;padding:0 12px;gap:8px;
    font:12px -apple-system,BlinkMacSystemFont,sans-serif;color:#9da3ad}
  .dot{width:12px;height:12px;border-radius:50%}
  .t{flex:1;text-align:center;margin-right:52px}
  .screen{padding:10px 14px 14px}
  .screen pre{margin:0}
  .screen pre>div{background:${BG}!important;font-family:Menlo,"SF Mono",Consolas,monospace!important;font-size:13px!important}
  .screen pre>div>div{height:16px;line-height:16px;white-space:pre}
  .screen pre>div>div>span{display:inline-block;height:16px;vertical-align:top}
  </style></head><body><div class="win"><div class="bar">
  <span class="dot" style="background:#ff5f57"></span><span class="dot" style="background:#febc2e"></span>
  <span class="dot" style="background:#28c840"></span><span class="t">${title}</span></div>
  <div class="screen">${body}</div></div></body></html>`;
}

// ------------------------------------------------------------------ browser

async function browser() {
  const puppeteer = (await import('puppeteer-core')).default;
  return puppeteer.launch({ executablePath: CHROME, headless: true,
    args: ['--hide-scrollbars', '--force-color-profile=srgb'] });
}

async function shootTerminal(b, name, title, html) {
  const page = await b.newPage();
  await page.setViewport({ width: 1400, height: 900, deviceScaleFactor: 2 });
  await page.setContent(terminalPage(title, html));
  // Blank rows under the last thing drawn: a short answer in a tall terminal.
  await page.evaluate(() => {
    const rows = [...document.querySelectorAll('.screen pre>div>div')];
    for (let i = rows.length - 1; i > 0; i--) {
      const r = rows[i];
      const painted = [...r.querySelectorAll('span')].some(sp => sp.style.backgroundColor);
      if (r.textContent.trim() || painted) break;
      r.remove();
    }
  });
  const el = await page.$('.win');
  const box = await el.boundingBox();
  await page.screenshot({ path: path.join(outDir, `${name}.png`), omitBackground: true,
    clip: { x: box.x - 20, y: box.y - 14, width: box.width + 40, height: box.height + 36 } });
  await page.close();
}

// ---------------------------------------------------------------- annotated

// A terminal picture with numbered badges on its parts and a legend under it
// saying what each is. A note finds its part by text on the screen, so the
// badges follow the layout wherever the command draws it:
//   card: text on a box's top edge ('╭─ Now'); the badge sits on that edge
//         and the box is outlined down to its bottom corner.
//   line: text on a row of its own; the badge goes after the row's last word,
//         or with word: after the word the text runs into.
//   no find: a part the demo does not show (a section only some days have);
//         legend only, its number hollow.
// side puts the legend beside the window, for output tall and narrow.
const BADGE = '#f5a623';
function annotatedPage(title, body, notes, side) {
  const legend = notes.map((n, i) => `<li><span class="num${n.find ? '' : ' off'}">${i + 1}</span><div><b>${n.title}</b>${n.text}</div></li>`).join('');
  return terminalPage(title, body)
    .replace('</style>', `
  .shot{display:inline-block;margin:24px}
  .shot .win{margin:0;position:relative}
  .shot .legend{display:grid}
  .badge{position:absolute;width:18px;height:18px;margin:-10px 0 0 -9px;border-radius:50%;background:${BADGE};
    color:#1e2127;font:700 11px/18px -apple-system,BlinkMacSystemFont,sans-serif;text-align:center;
    box-shadow:0 0 0 2px #1e2127,0 2px 6px rgba(0,0,0,.4);z-index:2}
  .ring{position:absolute;border:1.5px solid ${BADGE};border-radius:8px;opacity:.55;pointer-events:none}
  .legend{margin:14px 0 0;padding:16px 20px;list-style:none;background:#fbfaf7;border-radius:10px;
    box-shadow:0 6px 18px rgba(0,0,0,.16),0 0 0 1px rgba(0,0,0,.08);display:grid;grid-template-columns:1fr 1fr;
    gap:12px 28px;font:13px/1.45 -apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;color:#2b2f36}
  .legend li{display:flex;gap:10px;align-items:flex-start}
  .legend .num{flex:none;width:22px;height:22px;border-radius:50%;background:${BADGE};color:#1e2127;
    font-weight:700;text-align:center;line-height:22px}
  .legend b{display:block;margin-bottom:2px;color:#111}
  .legend .num.off{background:none;box-shadow:inset 0 0 0 1.5px ${BADGE};color:#a26a0c}
  .shot.side{display:inline-flex;gap:18px;align-items:flex-start}
  .shot.side .legend{margin:0;grid-template-columns:1fr;width:380px;flex:none}
  .legend code{font:12px Menlo,"SF Mono",Consolas,monospace;background:#efece4;padding:0 3px;border-radius:3px;white-space:nowrap}
  </style>`)
    .replace('<div class="win">', `<div class="shot${side ? ' side' : ''}"><div class="win">`)
    .replace(/<\/div><\/body>/, `</div><ol class="legend">${legend}</ol></div></body>`);
}

// Puts the badges on, in the page: returns the parts it could not find.
function placeBadges(notes) {
  const win = document.querySelector('.win');
  // The legend as wide as the window, not as wide as its longest note.
  const shot = document.querySelector('.shot');
  if (!shot.classList.contains('side')) {
    const legend = document.querySelector('.legend');
    legend.style.boxSizing = 'border-box';
    legend.style.width = `${win.offsetWidth}px`;
  }
  const origin = win.getBoundingClientRect();
  const rows = [...document.querySelectorAll('.screen pre>div>div')];
  // The screen rect of the character at offset in a row's text.
  const charRect = (row, offset) => {
    const walk = document.createTreeWalker(row, NodeFilter.SHOW_TEXT);
    for (let n = walk.nextNode(); n; n = walk.nextNode()) {
      if (offset < n.length) {
        const r = document.createRange();
        r.setStart(n, offset); r.setEnd(n, offset + 1);
        return r.getBoundingClientRect();
      }
      offset -= n.length;
    }
    return null;
  };
  const put = (cls, css, text = '') => {
    const el = document.createElement('div');
    el.className = cls;
    Object.assign(el.style, css);
    el.textContent = text;
    win.appendChild(el);
  };
  const px = v => `${Math.round(v)}px`;
  const missing = [];
  notes.forEach((n, i) => {
    if (!n.find) return;
    const at = rows.findIndex(r => r.textContent.includes(n.find));
    if (at < 0) { missing.push(n.find); return; }
    const row = rows[at];
    const text = row.textContent;
    if (n.card) {
      const tl = charRect(row, text.indexOf(n.find));
      let tr = null, bottom = null;
      for (let k = text.indexOf(n.find); k < text.length; k++) if (text[k] === '╮') { tr = charRect(row, k); break; }
      for (let j = at + 1; j < rows.length && !bottom; j++) {
        const t = rows[j].textContent;
        for (let k = t.indexOf('╰'); k >= 0; k = t.indexOf('╰', k + 1)) {
          const r = charRect(rows[j], k);
          if (Math.abs(r.left - tl.left) < 3) { bottom = r; break; }
        }
      }
      if (!tr || !bottom) { missing.push(n.find); return; }
      put('ring', { left: px(tl.left - origin.left - 3), top: px(tl.top - origin.top - 1),
        width: px(tr.right - tl.left + 6), height: px(bottom.bottom - tl.top + 2) });
      put('badge', { left: px(tr.left - origin.left - 18), top: px((tl.top + tl.bottom) / 2 - origin.top) }, String(i + 1));
    } else {
      // After the word the text leads into, or the row's last word.
      let end = text.trimEnd().length - 1;
      if (n.word) {
        end = text.indexOf(n.find) + n.find.length;
        while (end < text.length && text[end] !== ' ') end++;
        end--;
      }
      const r = charRect(row, end);
      put('badge', { left: px(r.right - origin.left + 18), top: px((r.top + r.bottom) / 2 - origin.top) }, String(i + 1));
    }
  });
  return missing;
}

async function shootAnnotated(b, name, title, html, notes, { side = false } = {}) {
  const page = await b.newPage();
  await page.setViewport({ width: 1600, height: 1000, deviceScaleFactor: 2 });
  await page.setContent(annotatedPage(title, html, notes, side));
  await page.evaluate(() => {
    const rows = [...document.querySelectorAll('.screen pre>div>div')];
    for (let i = rows.length - 1; i > 0 && !rows[i].textContent.trim(); i--) rows[i].remove();
  });
  const missing = await page.evaluate(placeBadges, notes.map(({ find, card, word }) => ({ find, card: !!card, word: !!word })));
  if (missing.length) throw new Error(`${name}: nothing on screen reads ${missing.map(m => `"${m}"`).join(', ')}`);
  const box = await (await page.$('.shot')).boundingBox();
  await page.screenshot({ path: path.join(outDir, `${name}.png`), omitBackground: true,
    clip: { x: box.x - 20, y: box.y - 14, width: box.width + 40, height: box.height + 36 } });
  await page.close();
}

// A worg page logged in: a noAuth server ignores the token, but worg shows its
// login screen until it finds one in this cookie.
async function worgPage(b, { width = 1440, height = 860, theme = 'light' } = {}) {
  const page = await b.newPage();
  await page.setViewport({ width, height, deviceScaleFactor: 2 });
  await page.setCookie({ name: 'orgs_auth', url: BASE,
    value: encodeURIComponent(JSON.stringify({ token: 'screenshots', expiresAt: Date.now() + 86400000 })) });
  // ?theme= is only read when worg is embedded (VS Code); a plain tab takes
  // Joy's remembered mode, else the system's.
  await page.emulateMediaFeatures([{ name: 'prefers-color-scheme', value: theme }]);
  await page.evaluateOnNewDocument(t => { try { localStorage.setItem('joy-mode', t); } catch { } }, theme);
  page.theme = theme;
  return page;
}

async function openApp(page, app, extra = '') {
  await page.goto(`${BASE}/?app=${app}&theme=${page.theme}${extra}`, { waitUntil: 'load' });
  // worg keeps a connection open for notifications, so the network never
  // goes idle: give the tab time to fetch and draw instead.
  await new Promise(r => setTimeout(r, 2500));
}

async function shootPage(page, name, clip) {
  await page.screenshot({ path: path.join(outDir, `${name}.png`), ...(clip ? { clip } : {}) });
}

// ------------------------------------------------------------------- shots

// Each shot: a name (the png's), what kind, and how to get there.
const wait = ms => [[ms, '']];
const CLI = [
  // name, window title, args, terminal options
  ['cli-agenda', 'orgs agenda', ['agenda'], { cols: 112, rows: 38, keys: wait(2500) }],
  ['cli-tui', 'orgs tui', ['tui'], { cols: 118, rows: 30, keys: wait(2500) }],
  ['cli-kanban', 'orgs kanban', ['kanban'], { cols: 120, rows: 34, keys: wait(2500) }],
  ['cli-gantt', 'orgs gantt Plan -crit', ['gantt', 'Plan', '-crit'], { cols: 118, rows: 20 }],
  ['cli-cols', 'orgs cols work.org', ['cols', 'work.org'], { cols: 100, rows: 24, keys: wait(2500) }],
  ['cli-habits', 'orgs habits', ['habits'], { cols: 84, rows: 14 }],
  ['cli-plan', 'orgs plan', ['plan'], { cols: 100, rows: 30 }],
  ['cli-review', 'orgs review', ['review', '-limit', '4'], { cols: 100, rows: 40 }],
  ['cli-tables', 'orgs tables', ['tables'], { cols: 118, rows: 26, keys: wait(3000) }],
  ['cli-search', `orgs search 'IsTask() && HasTags("work")'`, ['search', 'IsTask() && HasTags("work")'], { cols: 90, rows: 16 }],
  ['cli-drill', 'orgs drill Spanish', ['drill', 'Spanish'], { cols: 96, rows: 20, keys: [[2500, '\r'], [1500, ' ']] }],
  ['cli-stats', 'orgs stats', ['stats'], { cols: 80, rows: 14 }],
];
// Annotated shots run after all the others, because their setup changes the
// demo (a habit ticked, a task blocked, a pomodoro clocked in) and the other
// pictures should not show it.
const cli = (bin, config, ...args) => execFileSync(bin, ['-config', config, ...args],
  { cwd: repo, stdio: ['ignore', 'pipe', 'ignore'] }).toString();
const post = (route, body) => fetch(`${BASE}/${route}`, { method: 'POST',
  headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body ?? {}) }).then(r => r.json());
function hashOf(bin, config, headline) {
  const found = JSON.parse(cli(bin, config, 'search', 'IsTask()', '-json')).find(t => t.Headline === headline);
  if (!found) throw new Error(`no task "${headline}" in the demo`);
  return found.Hash;
}
// A pomodoro on the Q4 goals, unless one is running already (a shot taken
// after cli-dash finds its session).
async function ensureFocus(bin, config) {
  const st = await fetch(`${BASE}/focus`).then(r => r.json());
  if (st.Phase === 'work') return;
  await post('focus/start', { Hash: hashOf(bin, config, 'Draft the Q4 goals'), Work: 25, Short: 5, Long: 15, LongEvery: 4 });
}
const ANNOTATED = [
  ['cli-dash', 'orgs dash', ['dash'], { cols: 130, rows: 40 }, async (bin, config) => {
    // Something in every card: a pomodoro running, one habit kept, and a
    // watched query that has just found something.
    await ensureFocus(bin, config);
    cli(bin, config, 'habits', 'done', 'Morning run');
    await post('watches', { name: 'Blocked', label: 'Blocked work', query: 'IsTask() && IsStatus("BLOCKED")' });
    cli(bin, config, 'todo', '-yes', '-hash', hashOf(bin, config, 'Tidy the on-call runbook'), 'BLOCKED');
    // The file's reload runs the watch; asking for a check as well can
    // report the same change twice.
    for (let i = 0; i < 40; i++) {
      const n = await fetch(`${BASE}/notifications?unread=t`).then(r => r.json());
      if (n.unread) break;
      await new Promise(r => setTimeout(r, 250));
    }
  }, [
    { find: '  week ', word: true, title: 'The day',
      text: 'Today\'s date, the ISO week number and the time the dashboard was drawn.' },
    { find: ' to do', title: 'The summary',
      text: 'The whole screen in one line: appointments, tasks still to do, what is overdue, habits left, the inbox and new notices. A clear day says so.' },
    { find: '╭─ Now', card: true, title: 'Now',
      text: 'What you are doing this minute: a running pomodoro with its countdown and progress bar, or the heading you are clocked into and for how long.' },
    { find: '╭─ Overdue', card: true, title: 'Overdue',
      text: 'Scheduled (<code>○</code>) and deadline (<code>◆</code>) entries whose day has passed, with how late each one is. The first six; the rest are counted.' },
    { find: '╭─ Today', card: true, title: 'Today',
      text: 'Appointments by time, with a <code>now</code> line among them (past ones dimmed, the current one bold), then today\'s scheduled tasks and coming deadlines (<code>◇ due in 2d</code>, <code>◆ due today</code>). Keywords and priorities sit on the right.' },
    { find: '╭─ Habits', card: true, title: 'Habits',
      text: 'The habits due today, ticked or not, each with its last seven days (<code>●</code> kept, <code>○</code> missed, <code>·</code> not due) and its current streak.' },
    { find: '╭─ Waiting for you', card: true, title: 'Waiting for you',
      text: 'The inbox, with its first few entries, and what your watched queries (<code>orgs q watch</code>) have found since you last looked.' },
    { find: '→ orgs', title: 'What to do next',
      text: 'The command that acts on each card that wants something: <code>orgs agenda</code> for the late, <code>orgs triage</code> for the inbox, <code>orgs habits</code>, <code>orgs focus</code>.' },
  ]],
  ['cli-standup', 'orgs standup', ['standup'], { cols: 76, rows: 34, side: true }, ensureFocus, [
    { find: '*Standup ·', title: 'The title',
      text: 'Bold in Slack (<code>*…*</code>); a <code>##</code> heading with <code>-md</code>.' },
    { find: '✅ *Done', title: 'Done since the last working day',
      text: 'What was closed (cancelled ones struck through, repeats counted), and the habits kept. On a Monday that means since Friday; <code>-since</code> picks the day.' },
    { find: '⏱ *Clocked', title: 'Clocked',
      text: 'Time on the clock over the same days, by project, with its biggest headings.' },
    { find: '📅 *Today', title: 'Today',
      text: 'What is running now (<code>⏳</code>), then today\'s agenda: appointments with their times, overdue and coming deadlines, next actions. <code>-n</code> caps it; the rest are counted.' },
    { find: '🚧 *Blockers', title: 'Blockers',
      text: 'What you are waiting on somebody for, and work held up behind an unfinished task (<code>AFTER</code>, <code>ORDERED</code>).' },
  ]],
  ['cli-recap', 'orgs recap', ['recap'], { cols: 76, rows: 34, side: true }, null, [
    { find: '*Recap ·', title: 'The range',
      text: 'The last seven days unless told otherwise: <code>-month</code>, <code>-last -week</code>, <code>-from</code>/<code>-to</code>, or a clock table <code>-block</code>.' },
    { find: ' clocked · ', title: 'The headline numbers',
      text: 'Done, clocked, words written, habits kept out of possible, blockers: the week in one line.' },
    { find: '✅ *Done', title: 'Done, by project',
      text: 'Everything finished, grouped by project (or file), biggest group first.' },
    { find: '⏱ *Clocked', title: 'Clocked',
      text: 'Where the time went: each project\'s hours and its share of the total.' },
    { title: 'Writing',
      text: 'Words written in writing mode, against your writing goal, by file. Only when you write with a goal or a log.' },
    { find: '🔁 *Habits', title: 'Habits',
      text: 'Each habit: days kept out of days it asked for.' },
    { find: '🚧 *Blocked', title: 'Blocked',
      text: 'Still in the way at the end of the range: waiting on somebody, or held up by an unfinished task.' },
  ]],
  ['cli-help', 'orgs help standup', ['help', 'standup'], { cols: 122, rows: 40 }, null, [
    { find: 'orgs standup  what', title: 'What it is',
      text: 'The command and its one-line description, as <code>orgs help</code> lists it.' },
    { find: '-copy ', title: 'Its flags',
      text: 'In alphabetical order, each with what it does and, dim in brackets, its default.' },
    { find: '-json ', title: 'The flags every command has',
      text: '<code>-json</code>, <code>-format</code>, <code>-dry-run</code> and <code>-no-color</code>, unless the command uses the name for something of its own.' },
    { find: 'Examples', title: 'Examples',
      text: 'Ways to run it, and what each is for.' },
    { find: 'The morning standup', title: 'What the guides say',
      text: 'The documentation\'s section about this command (here, from these pages), carried inside the binary.' },
    { title: 'The query language',
      text: 'Commands that take a query (<code>search</code>, <code>tui</code>, <code>focus</code>…) end with the whole query language and every function it has.' },
  ]],
];

// Clicks the innermost element whose own text is exactly text (the last one
// on the page when within is "last"). Throws when there is none, so a renamed
// button fails the shot rather than silently shooting the wrong screen.
async function clickText(page, text, { within = 'body', last = false, settle = 900 } = {}) {
  const ok = await page.evaluate((text, within, last) => {
    const els = [...document.querySelectorAll(`${within} *`)].filter(e =>
      e.textContent.trim() === text && ![...e.children].some(c => c.textContent.trim() === text) &&
      e.getClientRects().length);
    const el = last ? els[els.length - 1] : els[0];
    if (!el) return false;
    el.scrollIntoView({ block: 'center' });
    el.dispatchEvent(new MouseEvent('mousedown', { bubbles: true }));
    el.click();
    return true;
  }, text, within, last);
  if (!ok) throw new Error(`nothing reads "${text}"`);
  await new Promise(r => setTimeout(r, settle));
}

// A MUI select: open the one showing current, choose option from its list.
async function choose(page, current, option) {
  await clickText(page, current, { settle: 500 });
  // An option can carry more than its name (the mind map's shows the query
  // under it), so match the start.
  const ok = await page.evaluate(option => {
    const el = [...document.querySelectorAll('[role=listbox] [role=option]')]
      .find(o => o.textContent.trim().startsWith(option));
    if (el) el.click();
    return !!el;
  }, option);
  if (!ok) throw new Error(`no option "${option}"`);
  await new Promise(r => setTimeout(r, 900));
}

const settle = ms => new Promise(r => setTimeout(r, ms));
const WORG = [
  ['worg-agenda', async p => { await openApp(p, 'Agenda'); await shootPage(p, 'worg-agenda'); }],
  ['worg-agenda-dark', Object.assign(async p => { await openApp(p, 'Agenda'); await shootPage(p, 'worg-agenda-dark'); }, { opts: { theme: 'dark' } })],
  ['worg-search', async p => { await openApp(p, 'Search'); await shootPage(p, 'worg-search'); }],
  ['worg-kanban', async p => { await openApp(p, 'Kanban'); await shootPage(p, 'worg-kanban'); }],
  ['worg-gantt', async p => {
    await openApp(p, 'GanttChart');
    await choose(p, 'Query', 'Plan');
    await settle(1500);
    // worg's own chart, not the exporter's mermaid diagram.
    await choose(p, 'Mermaid', 'React');
    await settle(2500);
    await shootPage(p, 'worg-gantt');
  }],
  ['worg-columns', async p => {
    await openApp(p, 'Columns');
    await p.click('input[placeholder="pick a file"]');
    await p.keyboard.type('work.org');
    await settle(400);
    await p.keyboard.press('ArrowDown');
    await p.keyboard.press('Enter');
    await settle(2000);
    await shootPage(p, 'worg-columns');
  }],
  ['worg-files', async p => {
    await openApp(p, 'Files');
    await clickText(p, 'work.org', { settle: 1500 });
    await clickText(p, 'Org', { settle: 1500 });
    await shootPage(p, 'worg-files');
  }],
  ['worg-tables', async p => { await openApp(p, 'Tables'); await clickText(p, 'budget'); await settle(1000); await shootPage(p, 'worg-tables'); }],
  ['worg-contacts', async p => { await openApp(p, 'Contacts'); await clickText(p, 'Maya Chen', { last: true }); await shootPage(p, 'worg-contacts'); }],
  ['worg-flashcards', async p => {
    await openApp(p, 'Drill');
    await clickText(p, 'Drill 5 of 5', { settle: 1500 });
    await shootPage(p, 'worg-flashcards');
  }],
  ['worg-review', async p => { await openApp(p, 'WeeklyReview'); await shootPage(p, 'worg-review'); }],
  ['worg-mindmap', async p => {
    await openApp(p, 'MindMap');
    await choose(p, 'Pick one', 'Plan');
    await settle(2500);
    await shootPage(p, 'worg-mindmap');
  }],
  ['worg-code', async p => { await openApp(p, 'Code'); await clickText(p, 'Fibonacci in Go'); await shootPage(p, 'worg-code'); }],
];

export { CLI, ANNOTATED, WORG, setupDemo, BASE, PORT, terminal, shootTerminal, worgPage, openApp, shootPage };

// ------------------------------------------------------------ annotated worg

// A worg page with numbered badges and a legend, the browser counterpart of
// shootAnnotated. A note's at finds its element in the page:
//   { text: 'Inbox' }            the innermost element whose own text is that
//   { text: 'Inbox', starts }    ... or starts with it
//   { selector: '.MuiCard-root' } the first match (nth: n for another)
//   last: true                   the last match rather than the first
//   up: n / closest: 'css'       then an ancestor of it, for the whole panel
// badge: tr (default), tl, bl, br, l, r - where on the element's box the
// badge sits; ring: false leaves the box unoutlined. A note without at is
// legend only, as in terminal shots. clip (from the shot's go) crops to part
// of the page; every position is worked out against it.
function findRects(notes) {
  const pick = at => {
    let els;
    if (at.selector) els = [...document.querySelectorAll(at.selector)];
    else els = [...document.querySelectorAll('body *')].filter(e => {
      const t = e.textContent.trim();
      const ok = at.starts ? t.startsWith(at.text) : t === at.text;
      if (!ok || !e.getClientRects().length) return false;
      return ![...e.children].some(c => at.starts ? c.textContent.trim().startsWith(at.text) : c.textContent.trim() === at.text);
    });
    els = els.filter(e => e.getClientRects().length);
    if (at.within) els = els.filter(e => e.closest(at.within));
    let el = at.last ? els[els.length - 1] : els[at.nth || 0];
    if (!el) return null;
    for (let i = 0; i < (at.up || 0) && el.parentElement; i++) el = el.parentElement;
    if (at.closest) el = el.closest(at.closest) || el;
    const r = el.getBoundingClientRect();
    return { x: r.left, y: r.top, w: r.width, h: r.height };
  };
  return notes.map(n => n.at ? pick(n.at) : undefined);
}

function annotatedImagePage(png, width, height, notes, rects, { side = false, title = '' } = {}) {
  const legend = notes.map((n, i) => `<li><span class="num${n.at ? '' : ' off'}">${i + 1}</span><div><b>${n.title}</b>${n.text}</div></li>`).join('');
  const marks = notes.map((n, i) => {
    const r = rects[i];
    if (!r) return '';
    const pos = { tl: [r.x, r.y], tr: [r.x + r.w, r.y], bl: [r.x, r.y + r.h], br: [r.x + r.w, r.y + r.h],
      l: [r.x, r.y + r.h / 2], r: [r.x + r.w, r.y + r.h / 2] }[n.badge || 'tr'];
    const ring = n.ring === false ? '' :
      `<div class="ring" style="left:${r.x - 3}px;top:${r.y - 3}px;width:${r.w + 6}px;height:${r.h + 6}px"></div>`;
    const bx = Math.min(Math.max(pos[0], 12), width - 12), by = Math.min(Math.max(pos[1], 12), height - 12);
    return ring + `<div class="badge" style="left:${bx}px;top:${by}px">${i + 1}</div>`;
  }).join('');
  return `<!doctype html><html><head><meta charset="utf-8"><style>
  html,body{margin:0;background:transparent}
  .shot{display:inline-block;margin:24px}
  .shot.side{display:inline-flex;gap:18px;align-items:flex-start}
  .frame{position:relative;border-radius:10px;overflow:hidden;width:${width}px;
    box-shadow:0 12px 32px rgba(0,0,0,.22),0 0 0 1px rgba(0,0,0,.14);background:#fff}
  .bar{height:30px;background:#e9e7e2;display:flex;align-items:center;padding:0 12px;gap:8px;
    font:12px -apple-system,BlinkMacSystemFont,sans-serif;color:#5d636c}
  .dot{width:12px;height:12px;border-radius:50%}
  .t{flex:1;text-align:center;margin-right:52px}
  .page{position:relative;width:${width}px;height:${height}px}
  .page img{display:block;width:${width}px;height:${height}px}
  .badge{position:absolute;width:24px;height:24px;margin:-12px 0 0 -12px;border-radius:50%;background:${BADGE};
    color:#1e2127;font:700 13px/24px -apple-system,BlinkMacSystemFont,sans-serif;text-align:center;
    box-shadow:0 0 0 2.5px #fff,0 2px 8px rgba(0,0,0,.35);z-index:2}
  .ring{position:absolute;border:2px solid ${BADGE};border-radius:8px;box-shadow:0 0 0 3px rgba(245,166,35,.18);pointer-events:none}
  .legend{margin:14px 0 0;padding:16px 20px;list-style:none;background:#fbfaf7;border-radius:10px;box-sizing:border-box;width:${width}px;
    box-shadow:0 6px 18px rgba(0,0,0,.16),0 0 0 1px rgba(0,0,0,.08);display:grid;grid-template-columns:1fr 1fr;
    gap:12px 28px;font:13px/1.45 -apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;color:#2b2f36}
  .shot.side .legend{margin:0;grid-template-columns:1fr;width:380px;flex:none}
  .legend li{display:flex;gap:10px;align-items:flex-start}
  .legend .num{flex:none;width:22px;height:22px;border-radius:50%;background:${BADGE};color:#1e2127;
    font-weight:700;text-align:center;line-height:22px}
  .legend .num.off{background:none;box-shadow:inset 0 0 0 1.5px ${BADGE};color:#a26a0c}
  .legend b{display:block;margin-bottom:2px;color:#111}
  .legend code{font:12px Menlo,"SF Mono",Consolas,monospace;background:#efece4;padding:0 3px;border-radius:3px;white-space:nowrap}
  </style></head><body><div class="shot${side ? ' side' : ''}"><div class="frame">
  <div class="bar"><span class="dot" style="background:#ff5f57"></span><span class="dot" style="background:#febc2e"></span>
  <span class="dot" style="background:#28c840"></span><span class="t">${title}</span></div>
  <div class="page"><img src="data:image/png;base64,${png}">${marks}</div></div>
  <ol class="legend">${legend}</ol></div></body></html>`;
}

// Takes the page as the shot's go left it (go may return a clip), finds the
// notes in it, and draws the annotated picture.
async function shootAnnotatedWorg(b, page, name, notes, { clip, side, title = 'worg' } = {}) {
  const vp = page.viewport();
  const area = clip || { x: 0, y: 0, width: vp.width, height: vp.height };
  const rects = (await page.evaluate(findRects, notes.map(n => ({ at: n.at })))).map(r => r &&
    { x: r.x - area.x, y: r.y - area.y, w: r.w, h: r.h });
  const missing = notes.filter((n, i) => n.at && !rects[i]).map(n => JSON.stringify(n.at));
  if (missing.length) throw new Error(`${name}: nothing on the page matches ${missing.join(', ')}`);
  const png = await page.screenshot({ clip: area, encoding: 'base64' });
  const out = await b.newPage();
  await out.setViewport({ width: area.width + (side ? 460 : 60), height: area.height + 700, deviceScaleFactor: 2 });
  await out.setContent(annotatedImagePage(png, area.width, area.height, notes, rects, { side, title }));
  const box = await (await out.$('.shot')).boundingBox();
  await out.screenshot({ path: path.join(outDir, `${name}.png`), omitBackground: true, captureBeyondViewport: true,
    clip: { x: box.x - 20, y: box.y - 14, width: box.width + 40, height: box.height + 36 } });
  await out.close();
}

// ------------------------------------------------------------------- stories

// A story is a guide's pictures: stories/<name>.mjs exports
//   { demo?: setupDemo's options, shots: [...] }
// and each shot is one of
//   { name, cli: { title, args, opts }, setup?, notes }        terminal
//   { name, worg: async (page, ctx) => clip?, page?, setup?, notes, title? }
// setup and worg get ctx: { bin, config, base, cli(...args), post(route, body),
// hashOf(headline), openApp, clickText, choose, settle }. Each story runs on a
// server of its own, so what one sets up never shows in another's pictures.
async function loadStories() {
  const dir = path.join(here, 'stories');
  if (!fs.existsSync(dir)) return [];
  const out = [];
  for (const f of fs.readdirSync(dir).filter(f => f.endsWith('.mjs')).sort()) {
    const mod = await import(path.join(dir, f));
    out.push({ name: f.replace(/\.mjs$/, ''), ...mod.default });
  }
  return out;
}

async function runStory(b, bin0, story, pick) {
  const { work, config } = setupDemo(story.demo || {});
  const bin = path.join(work, 'bin', path.basename(bin0));
  fs.mkdirSync(path.dirname(bin), { recursive: true });
  fs.copyFileSync(bin0, bin);
  fs.chmodSync(bin, 0o755);
  console.error(`story ${story.name}: ${work}`);
  const server = await startServer(bin, config, work);
  const ctx = { bin, config, base: BASE, work, cli: (...a) => cli(bin, config, ...a), post,
    hashOf: h => hashOf(bin, config, h), openApp, clickText, choose, settle };
  try {
    for (const shot of story.shots) {
      if (!pick(shot.name)) continue;
      console.error(`  ${shot.name}`);
      try {
        if (shot.setup) await shot.setup(ctx);
        if (shot.cli) {
          const html = await terminal(bin, config, shot.cli.args, shot.cli.opts);
          await shootAnnotated(b, shot.name, shot.cli.title, html, shot.notes, shot.cli.opts || {});
        } else {
          const page = await worgPage(b, shot.page || {});
          try {
            const clip = await shot.worg(page, ctx);
            await shootAnnotatedWorg(b, page, shot.name, shot.notes, { clip, side: shot.side, title: shot.title });
          } finally { await page.close(); }
        }
      } catch (e) { console.error(`    ${shot.name} failed: ${e.message}`); }
    }
  } finally { server.kill(); await settle(500); }
}

async function shootWorg(b, want) {
  for (const [name, fn] of WORG) {
    if (want.length && !want.includes(name)) continue;
    console.error(`worg ${name}`);
    const page = await worgPage(b, fn.opts || {});
    try { await fn(page); }
    catch (e) { console.error(`  ${name} failed: ${e.message}`); }
    await page.close();
  }
}

async function main() {
  let want = process.argv.slice(2);
  // -reuse: worg shots against a demo server already running (`serve` in
  // another terminal), to iterate on one shot without a rebuild each time.
  if (want[0] === '-reuse') {
    want = want.slice(1);
    const b = await browser();
    try { await shootWorg(b, want); } finally { await b.close(); }
    return;
  }
  // Before the server: a story that does not load should not leave one running.
  const stories = await loadStories();
  const { work, config } = setupDemo();
  console.error(`demo notebook in ${work}`);
  const bin = buildOrgs(work);
  const server = await startServer(bin, config, work);
  const stop = () => { try { server.kill(); } catch { } };
  process.on('SIGINT', () => { stop(); process.exit(1); });
  process.on('SIGTERM', () => { stop(); process.exit(1); });

  if (want[0] === 'serve') {
    console.error(`serving the demo at ${BASE} (ctrl-c to stop)\n  cli: ${bin} -config ${config} <command>`);
    await new Promise(() => { });
  }

  fs.mkdirSync(outDir, { recursive: true });
  const b = await browser();
  // -story NAME: every picture of one story.
  let onlyStory = null;
  if (want[0] === '-story') { onlyStory = want[1]; want = want.slice(2); }
  const pick = n => !want.length || want.includes(n);
  try {
    if (onlyStory) {
      stop();
      const st = stories.find(s => s.name === onlyStory);
      if (!st) throw new Error(`no story ${onlyStory} in stories/`);
      await runStory(b, bin, st, pick);
      return;
    }
    for (const [name, title, args, opts] of CLI) {
      if (!pick(name)) continue;
      console.error(`cli  ${name}`);
      const html = await terminal(bin, config, args, opts);
      await shootTerminal(b, name, title, html);
    }
    await shootWorg(b, want);
    for (const [name, title, args, opts, setup, notes] of ANNOTATED) {
      if (!pick(name)) continue;
      console.error(`cli  ${name}`);
      if (setup) await setup(bin, config);
      const html = await terminal(bin, config, args, opts);
      await shootAnnotated(b, name, title, html, notes, opts);
    }
    stop();
    for (const st of stories) {
      if (st.shots.some(sh => pick(sh.name))) await runStory(b, bin, st, pick);
    }
  } finally {
    await b.close();
    stop();
  }
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  main().catch(e => { console.error(e); process.exit(1); });
}
