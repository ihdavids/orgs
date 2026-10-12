// Package templates carries this folder inside the binary. internal/assets
// unpacks it and puts it behind the configured templatePath, so a server run
// from anywhere still has every template and theme; a file of the same name in
// the configured folder wins.
package templates

import "embed"

//go:embed *
var Content embed.FS
