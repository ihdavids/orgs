package commands

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/ihdavids/orgs/internal/common"
)

// ResolveOrgFiles turns what somebody typed into the server's names for org
// files: a path that exists here (made absolute), or a name the server knows
// by its base name or the end of its path. A word matching nothing fails the
// command, naming the word.
func ResolveOrgFiles(core *Core, words []string) []string {
	known := SendReceiveGetOr[common.FileList](core, "files", nil)
	out := []string{}
	for _, w := range words {
		if abs, err := filepath.Abs(w); err == nil {
			if _, err := os.Stat(abs); err == nil {
				out = append(out, abs)
				continue
			}
		}
		hit := false
		for _, k := range known {
			if k == w || filepath.Base(k) == w || strings.HasSuffix(k, "/"+w) {
				out = append(out, k)
				hit = true
			}
		}
		if !hit {
			Fail("no org file called %s", w)
		}
	}
	return out
}

// ResolveOrgFile is ResolveOrgFiles for one word that must name one file.
func ResolveOrgFile(core *Core, word string) string {
	files := ResolveOrgFiles(core, []string{word})
	if len(files) > 1 {
		Fail("%s names %d files: %s", word, len(files), strings.Join(files, ", "))
	}
	return files[0]
}
