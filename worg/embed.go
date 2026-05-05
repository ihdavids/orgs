package worg

import "embed"

//go:embed all:static index.html favicon.ico manifest.json asset-manifest.json
var Content embed.FS
