package commands

import (
	"fmt"
	"strings"
)

// SetTopLevelYaml sets a top-level key in a yaml file's lines to an already
// quoted value, or appends it, editing that one line so the rest of the file
// (comments, order) is left as written.
//
// Only an unindented `key:` counts. The CLI's `token:`/`tokenExpiry:` sit at
// the top level, and `server.tokenExpiry` - a duration - is indented under
// `server:`. Matching after trimming the indent (as login and the refresh
// both did) overwrote the server's line with the CLI's, dedented, and the next
// command could not parse the yaml at all.
func SetTopLevelYaml(lines []string, key, value string) []string {
	want := key + ":"
	for i, line := range lines {
		if strings.HasPrefix(line, want) {
			lines[i] = fmt.Sprintf("%s: %s", key, value)
			return lines
		}
	}
	// Before a trailing empty line, so the file still ends with a newline.
	if n := len(lines); n > 0 && lines[n-1] == "" {
		return append(lines[:n-1], fmt.Sprintf("%s: %s", key, value), "")
	}
	return append(lines, fmt.Sprintf("%s: %s", key, value))
}
