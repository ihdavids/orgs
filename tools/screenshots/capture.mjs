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

function setupDemo() {
  const work = fs.mkdtempSync(path.join(os.tmpdir(), 'orgs-shots-'));
  const org = path.join(work, 'org');
  fs.mkdirSync(org);
  for (const f of fs.readdirSync(demoSrc)) {
    if (!f.endsWith('.org')) continue;
    fs.writeFileSync(path.join(org, f),
      expandDates(fs.readFileSync(path.join(demoSrc, f), 'utf8')));
  }
  fs.mkdirSync(path.join(org, 'worklog'));
  // Saved queries, the kanban board and the flashcard deck. The server keeps
  // these beside its config file.
  fs.copyFileSync(path.join(demoSrc, '../extensions.yaml'), path.join(work, 'orgs_extensions.yaml'));
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
url: "${BASE}"
`);
  return { work, org, config };
}

// ------------------------------------------------------------------- server

function buildOrgs(work) {
  const bin = path.join(work, process.platform === 'win32' ? 'orgs.exe' : 'orgs');
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

export { CLI, WORG, terminal, shootTerminal, worgPage, openApp, shootPage };

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
  try {
    const pick = n => !want.length || want.includes(n);
    for (const [name, title, args, opts] of CLI) {
      if (!pick(name)) continue;
      console.error(`cli  ${name}`);
      const html = await terminal(bin, config, args, opts);
      await shootTerminal(b, name, title, html);
    }
    await shootWorg(b, want);
  } finally {
    await b.close();
    stop();
  }
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  main().catch(e => { console.error(e); process.exit(1); });
}
