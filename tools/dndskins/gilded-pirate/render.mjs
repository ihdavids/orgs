// node render.mjs out.png '<svg ...>' [scale] [bg]
import puppeteer from 'puppeteer-core';
const [out, svg, scale = '4', bg = '#e8d4a6'] = process.argv.slice(2);
const b = await puppeteer.launch({ executablePath: '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome', headless: 'new' });
const p = await b.newPage();
await p.setViewport({ width: 1000, height: 800, deviceScaleFactor: +scale });
await p.setContent(`<body style="margin:0;background:${bg};display:inline-block;padding:10px">${svg}</body>`);
const el = await p.$('body');
await el.screenshot({ path: out });
await b.close();
