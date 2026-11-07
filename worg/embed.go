package worg

import "embed"

// What gets served from inside the binary. This list is the gate: a file that
// lands in worg's `public/` and is not matched here is copied into this folder
// by tools/buildworg.sh, committed, and then 404s at runtime with nothing to
// say why - which is exactly what happened to the unicorn icons the first time.
// `*.png` is a pattern rather than a list of names so that dropping another
// image into public/ does not need this file edited again. gantt-viewer.js is
// what a gantt chart saved as html runs (worg's scripts/build-viewer.js):
// worg fetches it and puts it in the page, so it has to be here to fetch.
// dicefx.js is the character sheet's casting effects for the map screens
// (tools/dicefx/extract.py), loaded on the first effect. dict/ is the
// English dictionary the editor's writing mode spells with (en.aff, en.dic).
// pdfjs/ is pdf.js's worker for the Noter tab (worg's `npm run pdfjs`): pdf.js
// loads it by url, so it is a file here rather than part of the bundle.
// excalidraw-assets/ is the drawing editor's fonts and chunks (worg's `npm
// run excalidraw`), fetched by url from the page's own folder.
//
// tools/buildworg.sh stores the compressible files gzipped, as name.gz with no
// plain copy, which is why the scripts are matched with a trailing `*`; the
// server unzips them for a client that does not take gzip (worgserve.go).
//
//go:embed all:static index.html favicon.ico manifest.json asset-manifest.json* *.png gantt-viewer.js* dicefx.js* dict harper pdfjs excalidraw-assets
var Content embed.FS
