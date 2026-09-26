package worg

import "embed"

// What gets served from inside the binary. This list is the gate: a file that
// lands in worg's `public/` and is not matched here is copied into this folder
// by tools/buildworg.sh, committed, and then 404s at runtime with nothing to
// say why - which is exactly what happened to the unicorn icons the first time.
// `*.png` is a pattern rather than a list of names so that dropping another
// image into public/ does not need this file edited again.
//
//go:embed all:static index.html favicon.ico manifest.json asset-manifest.json *.png
var Content embed.FS
