package orgpkgs

// org-chef from the command line.
//
//	orgs chef add URL [-file recipes.org] [-under Mains]   fetch a recipe into a heading
//	orgs chef add URL -dry-run          show the heading it would write
//	orgs chef ls [words]                every recipe (words search names, ingredients, tags)
//	orgs chef show NAME [-servings 6]   one recipe, scaled (-factor 2 for one without servings)
//	orgs chef show NAME -cook           one step at a time, enter for the next
//	orgs chef shop NAME... [-servings 4] [-file F -under H] [-preview]
//	                                    a merged shopping checklist
//
// Needs `orgs features chef on`.

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

type Chef struct {
	base
	File      string
	Under     string
	Servings  float64
	Factor    float64
	NoPicture bool
	Force     bool
	Preview   bool
	Cook      bool
	Edit      bool
}

func (self *Chef) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	fset.StringVar(&self.File, "file", "", "add, shop: the org file to write into (default chef.file, recipes.org)")
	fset.StringVar(&self.Under, "under", "", "add, shop: the heading to write under (title, a::b path or hash)")
	fset.Float64Var(&self.Servings, "servings", 0, "show, shop: scale to this many servings")
	fset.Float64Var(&self.Factor, "factor", 0, "show: scale by this much (for a recipe that does not say what it serves)")
	fset.BoolVar(&self.NoPicture, "no-picture", false, "add: keep the picture's url instead of saving it beside the file")
	fset.BoolVar(&self.Force, "force", false, "add: add a page that is already in a file")
	fset.BoolVar(&self.Preview, "preview", false, "shop: print the list, write nothing")
	fset.BoolVar(&self.Cook, "cook", false, "show: walk the steps one at a time")
	fset.BoolVar(&self.Edit, "edit", false, "show: open the recipe in the editor")
}

type chefStep struct {
	Section string `json:",omitempty"`
	Text    string
}

type chefRecipe struct {
	Hash, Filename                string
	Line                          int
	Tags                          []string `json:",omitempty"`
	Picture                       string   `json:",omitempty"`
	Name, Url, Description, Image string
	Yield                         string
	Servings                      float64
	PrepTime, CookTime, TotalTime string
	Author, Category, Cuisine     string
	Keywords                      []string `json:",omitempty"`
	Ingredients                   []string
	Directions                    []chefStep
}

type chefShopItem struct {
	Text string
	From []string `json:",omitempty"`
}

func num(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

// about is the one line of facts under a recipe's name.
func (r *chefRecipe) about() string {
	parts := []string{}
	if r.Servings > 0 {
		parts = append(parts, "serves "+num(r.Servings))
	} else if r.Yield != "" {
		parts = append(parts, r.Yield)
	}
	for _, t := range [][2]string{{"prep", r.PrepTime}, {"cook", r.CookTime}, {"ready in", r.TotalTime}} {
		if t[1] != "" {
			parts = append(parts, t[0]+" "+t[1])
		}
	}
	return strings.Join(parts, " · ")
}

func (self *Chef) recipes(core *commands.Core, q string) []chefRecipe {
	ps := map[string]string{}
	if q != "" {
		ps["q"] = q
	}
	res, err := commands.SendReceiveGetErr[struct {
		offAnswer
		Recipes []chefRecipe
	}](core, "chef/recipes", ps)
	if err != nil {
		commands.Fail("%v", err)
	}
	failIfOff(res.offAnswer)
	return res.Recipes
}

// find is the recipe a word names: a hash, the exact name, else the only
// one whose name has every word; several are offered in the picker.
func (self *Chef) find(core *commands.Core, all []chefRecipe, word string) chefRecipe {
	low := strings.ToLower(strings.TrimSpace(word))
	for _, r := range all {
		if r.Hash == word || strings.ToLower(r.Name) == low {
			return r
		}
	}
	hits := []chefRecipe{}
	for _, r := range all {
		name := strings.ToLower(r.Name)
		ok := true
		for _, w := range strings.Fields(low) {
			if !strings.Contains(name, w) {
				ok = false
				break
			}
		}
		if ok {
			hits = append(hits, r)
		}
	}
	if len(hits) == 1 {
		return hits[0]
	}
	if len(hits) == 0 {
		hits = all
		if len(all) == 0 {
			commands.Fail("no recipes yet (orgs chef add URL)")
		}
		if word != "" {
			if commands.Machine() || !commands.Interactive() {
				commands.Fail("no recipe called %q", word)
			}
		}
	}
	if commands.Machine() || !commands.Interactive() {
		names := []string{}
		for _, h := range hits {
			names = append(names, h.Name)
		}
		commands.Fail("%q could be any of: %s", word, strings.Join(names, "; "))
	}
	lines := []string{}
	for i, h := range hits {
		lines = append(lines, commands.PickLine([]string{strconv.Itoa(i)}, h.Name+"  "+dim(h.about())))
	}
	picked := commands.Pick(commands.PickOpts{Lines: lines, AddressFields: 1, Prompt: "Recipe> ", Extra: []string{"--query", word}})
	if len(picked) == 0 {
		os.Exit(1)
	}
	addr, _ := commands.Address(picked[0], 1)
	i, _ := strconv.Atoi(addr[0])
	return hits[i]
}

func printRecipe(r *chefRecipe) {
	fmt.Printf("%s%s%s\n", commands.C(commands.AnsiBold), r.Name, commands.C(commands.AnsiReset))
	if a := r.about(); a != "" {
		fmt.Println(dim(a))
	}
	if r.Url != "" {
		fmt.Println(dim(r.Url))
	}
	if r.Description != "" {
		fmt.Println()
		fmt.Println(r.Description)
	}
	fmt.Printf("\n%sIngredients%s\n", commands.C(commands.AnsiBold), commands.C(commands.AnsiReset))
	for _, i := range r.Ingredients {
		fmt.Printf("  - %s\n", i)
	}
	fmt.Printf("\n%sDirections%s\n", commands.C(commands.AnsiBold), commands.C(commands.AnsiReset))
	section, n := "\x00", 0
	for _, s := range r.Directions {
		if s.Section != section {
			section, n = s.Section, 0
			if s.Section != "" {
				fmt.Printf("  %s\n", dim(s.Section))
			}
		}
		n++
		fmt.Printf("  %d. %s\n", n, s.Text)
	}
}

func (self *Chef) Exec(core *commands.Core) {
	words := commands.FreeArgs(self.fset)
	verb := ""
	if len(words) > 0 {
		verb, words = words[0], words[1:]
	}
	switch verb {
	case "", "ls", "list":
		all := self.recipes(core, strings.Join(words, " "))
		commands.Render(all, func() {
			for _, r := range all {
				fmt.Printf("%s:%d %s  %s\n", dim(commands.BaseName(r.Filename)), r.Line+1, r.Name, dim(r.about()))
			}
			if len(all) == 0 {
				fmt.Println("no recipes (orgs chef add URL)")
			}
		})
	case "add", "fetch":
		if len(words) != 1 {
			commands.Fail("orgs chef add URL [-file recipes.org] [-under HEADING] [-no-picture] [-force]")
		}
		if verb == "fetch" || commands.DryRun {
			// A read: the server writes nothing for /chef/fetch.
			res, err := common.RestPost[struct {
				offAnswer
				Recipe chefRecipe
				Org    string
			}](&core.Rest, "chef/fetch", &struct{ Url string }{words[0]})
			if err != nil {
				commands.Fail("%v", err)
			}
			failIfOff(res.offAnswer)
			if !commands.RenderOne(res.Recipe, func() { fmt.Print(res.Org) }) && commands.DryRun {
				commands.Wrote("POST /chef/add", map[string]interface{}{"Url": words[0], "File": self.File, "Under": self.Under})
			}
			return
		}
		type addReq struct {
			Url, File, Under string
			NoPicture, Force bool
		}
		type addAnswer struct {
			Ok             bool
			Msg            string
			Hash, Filename string
			Line           int
			Existing       string
			Recipe         chefRecipe
			Warnings       []string
		}
		req := addReq{words[0], self.File, self.Under, self.NoPicture, self.Force}
		res, err := commands.SendReceivePostErr[addReq, addAnswer](core, "chef/add", &req)
		if err == commands.ErrDryRun {
			return
		}
		if err != nil {
			commands.Fail("%v", err)
		}
		if !res.Ok {
			commands.Fail("%s", res.Msg)
		}
		for _, w := range res.Warnings {
			fmt.Fprintln(os.Stderr, "warning: "+w)
		}
		if commands.JsonOut {
			commands.RenderOne(res, nil)
			return
		}
		commands.Ok(1, "added", []string{fmt.Sprintf("%s %s", res.Recipe.Name, dim(fmt.Sprintf("%s:%d", commands.BaseName(res.Filename), res.Line+1)))})
	case "show", "cook":
		all := self.recipes(core, "")
		r := self.find(core, all, strings.Join(words, " "))
		if self.Edit {
			core.LaunchEditor(r.Filename, r.Line+1)
			return
		}
		ps := map[string]string{"hash": r.Hash}
		if self.Servings > 0 {
			ps["servings"] = num(self.Servings)
		}
		if self.Factor > 0 {
			ps["factor"] = num(self.Factor)
		}
		res, err := commands.SendReceiveGetErr[struct {
			offAnswer
			Recipe chefRecipe
			Factor float64
		}](core, "chef/scale", ps)
		if err != nil {
			commands.Fail("%v", err)
		}
		failIfOff(res.offAnswer)
		if res.Msg != "" {
			fmt.Fprintln(os.Stderr, res.Msg)
		}
		if self.Cook || verb == "cook" {
			if commands.Machine() {
				commands.Fail("cook mode is for a person at a terminal")
			}
			cook(&res.Recipe)
			return
		}
		commands.RenderOne(res.Recipe, func() { printRecipe(&res.Recipe) })
	case "shop":
		if len(words) == 0 {
			commands.Fail("orgs chef shop NAME... [-servings 4] [-file F -under H] [-preview]")
		}
		all := self.recipes(core, "")
		type shopReq struct {
			Hashes      []string
			Servings    float64
			File, Under string
			Preview     bool
		}
		req := shopReq{Servings: self.Servings, File: self.File, Under: self.Under, Preview: self.Preview}
		for _, w := range words {
			req.Hashes = append(req.Hashes, self.find(core, all, w).Hash)
		}
		type shopAnswer struct {
			Ok             bool
			Off            bool
			Msg            string
			Items          []chefShopItem
			Hash, Filename string
			Line           int
		}
		var res shopAnswer
		var err error
		if self.Preview {
			res, err = common.RestPost[shopAnswer](&core.Rest, "chef/shop", &req)
		} else {
			res, err = commands.SendReceivePostErr[shopReq, shopAnswer](core, "chef/shop", &req)
			if err == commands.ErrDryRun {
				return
			}
		}
		if err != nil {
			commands.Fail("%v", err)
		}
		if !res.Ok {
			commands.Fail("%s", res.Msg)
		}
		commands.Render(res.Items, func() {
			for _, it := range res.Items {
				fmt.Printf("- [ ] %s  %s\n", it.Text, dim(strings.Join(it.From, ", ")))
			}
			if !self.Preview {
				fmt.Printf("%s✓%s %s %s\n", commands.C(commands.AnsiGreen), commands.C(commands.AnsiReset), res.Msg, dim(fmt.Sprintf("%s:%d", commands.BaseName(res.Filename), res.Line+1)))
			}
		})
	default:
		commands.Fail("orgs chef [ls|add|show|cook|shop] (unknown %q)", verb)
	}
}

// cook shows the ingredients, then a step at a time: enter for the next, b
// for back, q to stop.
func cook(r *chefRecipe) {
	printIngredients := func() {
		fmt.Printf("%s%s%s  %s\n\n", commands.C(commands.AnsiBold), r.Name, commands.C(commands.AnsiReset), dim(r.about()))
		for _, i := range r.Ingredients {
			fmt.Printf("  - %s\n", i)
		}
	}
	printIngredients()
	in := bufio.NewReader(os.Stdin)
	fmt.Print(dim("\nenter: first step  q: stop "))
	i := 0
	for i < len(r.Directions) {
		line, err := in.ReadString('\n')
		if err != nil {
			return
		}
		switch strings.TrimSpace(strings.ToLower(line)) {
		case "q":
			return
		case "b":
			if i > 1 {
				i -= 2
			} else {
				i = 0
			}
		case "i":
			printIngredients()
			fmt.Print(dim("\nenter: next "))
			continue
		}
		s := r.Directions[i]
		fmt.Print("\033[2J\033[H")
		head := fmt.Sprintf("Step %d of %d", i+1, len(r.Directions))
		if s.Section != "" {
			head += " · " + s.Section
		}
		fmt.Printf("%s\n\n%s%s%s\n\n", dim(head), commands.C(commands.AnsiBold), s.Text, commands.C(commands.AnsiReset))
		i++
		if i < len(r.Directions) {
			fmt.Print(dim("enter: next  b: back  i: ingredients  q: stop "))
		}
	}
	fmt.Println(dim("done - enjoy it"))
}

func init() {
	commands.AddCmd("chef", "recipes from web pages into org headings, scaled, and a shopping list (org-chef)",
		func() commands.Cmd { return &Chef{} })
}
