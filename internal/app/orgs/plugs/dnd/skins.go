package dnd

import (
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/ihdavids/orgs/internal/assets"
	"github.com/ihdavids/orgs/internal/common/dnd"
)

// LoadSkins reads every skin in the template folders' dnd_skins/ (the
// configured folder first, so a skin of the user's replaces a built-in one
// of the same name), sorted by name. A skin that does not keep to its own
// selectors is left out with a warning on stderr: it could change the
// classic sheet.
func LoadSkins(templatePath string) []dnd.Skin {
	out := []dnd.Skin{}
	for _, rel := range assets.GlobTemplates("dnd_skins/*.css", templatePath) {
		id := strings.TrimSuffix(path.Base(rel), ".css")
		file := assets.FindTemplate(rel, templatePath)
		css, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		svg := ""
		if f := assets.FindTemplate("dnd_skins/"+id+".svg", templatePath); f != "" {
			if b, err := os.ReadFile(f); err == nil {
				svg = string(b)
			}
		}
		s, err := dnd.ParseSkin(id, string(css), svg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "dnd skin %s left out: %s\n", file, err)
			continue
		}
		out = append(out, s)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && strings.ToLower(out[j].Name) < strings.ToLower(out[j-1].Name); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}
