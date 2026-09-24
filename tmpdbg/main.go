package main

import (
	"fmt"
	"os"

	"github.com/flosch/pongo2/v5"
	pdnd "github.com/ihdavids/orgs/internal/app/orgs/plugs/dnd"
	"github.com/ihdavids/orgs/internal/common/dnd"
)

func main() {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("PANIC:", r)
		}
	}()
	pdnd.SetPaths(pdnd.DefaultPaths("templates"))
	data, err := os.ReadFile("/Users/idavids/dev/gtd/gooey-mcfart.org")
	if err != nil {
		fmt.Println("read err:", err)
		return
	}
	c, rs, err := pdnd.ParseCharacter(string(data))
	if err != nil {
		fmt.Println("parse err:", err)
		return
	}
	fmt.Println("parsed:", c.Name, "ruleset:", c.Ruleset, "rs nil?", rs == nil)
	sheet := dnd.Compute(c, rs)
	fmt.Println("computed sheet:", sheet.Name)
	m := dnd.SheetMap(sheet, dnd.LatexEscape)
	fmt.Println("sheetmap keys:", len(m))
	tpl, terr := pongo2.FromFile("templates/dnd_character.tpl")
	if terr != nil {
		fmt.Println("template parse err:", terr)
		return
	}
	ctx := pongo2.Context{"sheet": pongo2.AsSafeValue(m), "title": sheet.Name, "fontfamily": "Cinzel"}
	res, eerr := tpl.Execute(ctx)
	if eerr != nil {
		fmt.Println("template exec err:", eerr)
		return
	}
	fmt.Println("tex bytes:", len(res))
	os.WriteFile("/private/tmp/claude-501/-Users-idavids-dev-orgs/16d4e6f0-6688-400b-aad0-a07f4a5c816b/scratchpad/sheet.tex", []byte(res), 0644)
}
