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
//
//go:embed all:static index.html favicon.ico manifest.json asset-manifest.json *.png gantt-viewer.js dicefx.js dict harper
var Content embed.FS
