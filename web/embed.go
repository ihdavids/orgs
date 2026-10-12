// Package web carries the old web pages inside the binary (served at /web/).
package web

import "embed"

//go:embed *
var Content embed.FS
