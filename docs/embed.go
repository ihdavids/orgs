// Package docs carries the documentation the server hands out itself, built
// into the binary so it is there whatever directory the server runs from.
package docs

import _ "embed"

// The Files tab editor's keys and snippets, rendered by GET /editor/help.
//
//go:embed editor.org
var EditorHelp string
