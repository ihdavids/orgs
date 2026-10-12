// Package webfonts carries the font awesome fonts inside the binary (served at
// /webfonts/, where web/fontawesome.css looks for them).
package webfonts

import "embed"

//go:embed *
var Content embed.FS
