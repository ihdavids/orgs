package orgpkgs

// Books, films, shows, games and albums from the command line (features.mediaDb).
//
//	orgs library search KIND WORDS...      look up in the catalogues (book, film, show, game, album, other)
//	orgs library add KIND WORDS...|ISBN    look up, choose, and file a heading (-file F -under H)
//	orgs library add KIND -id ID [-source S]   file one by its catalogue id
//	orgs library ls [KIND] [-shelf doing]  what you have (-json for everything)
//	orgs library show NAME                 one in full
//	orgs library set NAME -status read -rating 4   move it to a shelf, give it stars
//	orgs library refresh NAME [-overwrite] fill in what the heading is missing
//	orgs library sources                   which catalogues can be asked, and the keys they want
//
// Needs `orgs features mediaDb on`.

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
	"github.com/ihdavids/orgs/internal/common/mediadb"
)

type Library struct {
	base
	File      string
	Under     string
	Source    string
	Id        string
	Status    string
	Rating    float64
	Shelf     string
	Force     bool
	NoCover   bool
	Preview   bool
	First     bool
	Overwrite bool
	Edit      bool
}

func (self *Library) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	fset.StringVar(&self.File, "file", "", "add: the org file to write into (default: the kind's file, its collection, else library.org)")
	fset.StringVar(&self.Under, "under", "", "add: the heading to write under (title, a::b path or hash)")
	fset.StringVar(&self.Source, "source", "", "search, add: ask this catalogue (openlibrary, googlebooks, tmdb, igdb, rawg, musicbrainz, wikidata, wikipedia)")
	fset.StringVar(&self.Id, "id", "", "add: the catalogue's id (OL893414W, movie/329865, Q20382729...)")
	fset.StringVar(&self.Status, "status", "", "add, set: the shelf (to read, reading, read, set aside - or want, doing, done, dropped)")
	fset.Float64Var(&self.Rating, "rating", -1, "add, set: stars out of 5 (halves allowed; 0 clears)")
	fset.StringVar(&self.Shelf, "shelf", "", "ls: only this shelf")
	fset.BoolVar(&self.Force, "force", false, "add: add one that is already filed")
	fset.BoolVar(&self.NoCover, "no-cover", false, "add: link the cover's url instead of saving it beside the file")
	fset.BoolVar(&self.Preview, "preview", false, "add: print the heading, write nothing")
	fset.BoolVar(&self.First, "first", false, "add: take the first hit without asking")
	fset.BoolVar(&self.Overwrite, "overwrite", false, "refresh: also replace fields that changed")
	fset.BoolVar(&self.Edit, "edit", false, "show: open it in the editor")
}

type libHit struct {
	Kind, Source, Id, Title string
	Year, Creator, Thumb    string
	Blurb, Isbn, Have       string
}

type libTried struct{ Source, Label, Msg string }

type libItem struct {
	Hash, Filename         string
	Line                   int
	Kind, Title, Creator   string
	Date, Year             string
	Shelf, StatusWord      string
	Rating                 float64
	Cover                  string
	Tags                   []string
	MediaId, Isbn, Url     string
	Keyword                string
	Started, Finished      string
	Added                  string
	Fields                 map[string]string
	About                  string
}

func stars(r float64) string {
	if r <= 0 {
		return ""
	}
	full := int(r)
	s := strings.Repeat("★", full)
	if r-float64(full) >= 0.5 {
		s += "½"
	}
	return s
}

// libKind takes a kind off the front of the words, if the first is one.
func libKind(words []string, need bool) (string, []string) {
	if len(words) > 0 {
		if k := mediadb.KindOf(words[0]); k != "" {
			return k, words[1:]
		}
	}
	if need {
		commands.Fail("say which kind first: book, film, show, game, album or other (orgs library search book dune)")
	}
	return "", words
}

func (self *Library) search(core *commands.Core, kind, q string) []libHit {
	ps := map[string]string{"kind": kind, "q": q}
	if self.Source != "" {
		ps["source"] = self.Source
	}
	res, err := commands.SendReceiveGetErr[struct {
		offAnswer
		Source      string
		Attribution string
		Candidates  []libHit
		Tried       []libTried
	}](core, "mediadb/search", ps)
	if err != nil {
		commands.Fail("%v", err)
	}
	failIfOff(res.offAnswer)
	if res.Msg != "" {
		fmt.Fprintln(os.Stderr, res.Msg)
	}
	if len(res.Candidates) > 0 && res.Attribution != "" && !commands.Machine() {
		fmt.Fprintln(os.Stderr, dim(res.Attribution))
	}
	return res.Candidates
}

func hitLine(h libHit) string {
	about := []string{}
	for _, s := range []string{h.Year, h.Creator} {
		if s != "" {
			about = append(about, s)
		}
	}
	line := h.Title
	if len(about) > 0 {
		line += "  " + dim(strings.Join(about, " · "))
	}
	if h.Blurb != "" && h.Creator == "" {
		line += "  " + dim(h.Blurb)
	}
	if h.Have != "" {
		line += "  " + commands.C(commands.AnsiGreen) + "(have it)" + commands.C(commands.AnsiReset)
	}
	return line
}

func (self *Library) items(core *commands.Core, kind string) []libItem {
	ps := map[string]string{}
	if kind != "" {
		ps["kind"] = kind
	}
	if self.Shelf != "" {
		ps["shelf"] = self.Shelf
	}
	res, err := commands.SendReceiveGetErr[struct {
		offAnswer
		Items []libItem
	}](core, "mediadb/items", ps)
	if err != nil {
		commands.Fail("%v", err)
	}
	failIfOff(res.offAnswer)
	return res.Items
}

// find is the heading a word names: a hash, the exact title, else the only
// one whose title has every word; several are offered in the picker.
func (self *Library) find(all []libItem, word string) libItem {
	low := strings.ToLower(strings.TrimSpace(word))
	for _, it := range all {
		if it.Hash == word || strings.ToLower(it.Title) == low {
			return it
		}
	}
	hits := []libItem{}
	for _, it := range all {
		hay := strings.ToLower(it.Title + " " + it.Creator)
		ok := true
		for _, w := range strings.Fields(low) {
			if !strings.Contains(hay, w) {
				ok = false
				break
			}
		}
		if ok {
			hits = append(hits, it)
		}
	}
	if len(hits) == 1 {
		return hits[0]
	}
	if len(hits) == 0 {
		if len(all) == 0 {
			commands.Fail("nothing in the library yet (orgs library add book dune)")
		}
		if word != "" {
			commands.Fail("nothing in the library called %q", word)
		}
		hits = all
	}
	if commands.Machine() || !commands.Interactive() {
		names := []string{}
		for _, h := range hits {
			names = append(names, h.Title)
		}
		commands.Fail("%q could be any of: %s", word, strings.Join(names, "; "))
	}
	lines := []string{}
	for i, h := range hits {
		lines = append(lines, commands.PickLine([]string{strconv.Itoa(i)}, h.Title+"  "+dim(itemAbout(h))))
	}
	picked := commands.Pick(commands.PickOpts{Lines: lines, AddressFields: 1, Prompt: "Which> ", Extra: []string{"--query", word}})
	if len(picked) == 0 {
		os.Exit(1)
	}
	addr, _ := commands.Address(picked[0], 1)
	i, _ := strconv.Atoi(addr[0])
	return hits[i]
}

func itemAbout(it libItem) string {
	parts := []string{}
	for _, s := range []string{it.Creator, it.Year, it.StatusWord, stars(it.Rating)} {
		if s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, " · ")
}

type libAddAnswer struct {
	Ok             bool
	Off            bool
	Msg            string
	Hash, Filename string
	Line           int
	Existing       string
	Org            string
	Warnings       []string
	Item           struct{ Title, Kind string }
}

func (self *Library) Exec(core *commands.Core) {
	words := commands.FreeArgs(self.fset)
	verb := ""
	if len(words) > 0 {
		verb, words = words[0], words[1:]
	}
	switch verb {
	case "", "ls", "list":
		kind, rest := libKind(words, false)
		if len(rest) > 0 && kind == "" {
			commands.Fail("%q is not a kind (book, film, show, game, album, other)", rest[0])
		}
		all := self.items(core, kind)
		commands.Render(all, func() {
			if len(all) == 0 {
				fmt.Println("nothing on the shelves (orgs library add book dune)")
			}
			for _, it := range all {
				fmt.Printf("%-6s %s  %s  %s\n", dim(it.Kind), it.Title, dim(itemAbout(it)), dim(fmt.Sprintf("%s:%d", commands.BaseName(it.Filename), it.Line+1)))
			}
		})
	case "search", "find":
		kind, rest := libKind(words, true)
		if len(rest) == 0 {
			commands.Fail("orgs library search %s WORDS...", kind)
		}
		hits := self.search(core, kind, strings.Join(rest, " "))
		commands.Render(hits, func() {
			for _, h := range hits {
				fmt.Printf("%s  %s\n", hitLine(h), dim(h.Source+":"+h.Id))
			}
		})
	case "add":
		kind, rest := libKind(words, true)
		req := struct {
			Kind, Source, Id, File, Under, Status string
			Rating                                float64
			Force, NoCover, Preview               bool
		}{Kind: kind, Source: self.Source, Id: self.Id, File: self.File, Under: self.Under, Status: self.Status,
			Force: self.Force, NoCover: self.NoCover, Preview: self.Preview || commands.DryRun}
		if self.Rating > 0 {
			req.Rating = self.Rating
		}
		q := strings.Join(rest, " ")
		if req.Id == "" {
			if isbn, ok := mediadb.NormISBN(q); ok && kind == mediadb.Book && self.Source == "" {
				req.Id = isbn
			} else {
				if q == "" {
					commands.Fail("orgs library add %s WORDS... (or -id ID)", kind)
				}
				hits := self.search(core, kind, q)
				if len(hits) == 0 {
					commands.Fail("nothing found for %q", q)
				}
				pick := hits[0]
				if len(hits) > 1 && !self.First {
					if commands.Machine() || !commands.Interactive() {
						commands.Fail("%d found for %q - choose with -id (orgs library search %s %s) or take the first with -first", len(hits), q, kind, q)
					}
					lines := []string{}
					for i, h := range hits {
						lines = append(lines, commands.PickLine([]string{strconv.Itoa(i)}, hitLine(h)))
					}
					picked := commands.Pick(commands.PickOpts{Lines: lines, AddressFields: 1, Prompt: mediadb.KindLabel(kind) + "> "})
					if len(picked) == 0 {
						os.Exit(1)
					}
					addr, _ := commands.Address(picked[0], 1)
					i, _ := strconv.Atoi(addr[0])
					pick = hits[i]
				}
				req.Source, req.Id = pick.Source, pick.Id
			}
		}
		var res libAddAnswer
		var err error
		if req.Preview {
			// A read: the server writes nothing for a preview.
			res, err = common.RestPost[libAddAnswer](&core.Rest, "mediadb/add", &req)
		} else {
			res, err = commands.SendReceivePostErr[struct {
				Kind, Source, Id, File, Under, Status string
				Rating                                float64
				Force, NoCover, Preview               bool
			}, libAddAnswer](core, "mediadb/add", &req)
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
		if req.Preview {
			if !commands.RenderOne(res, func() { fmt.Print(res.Org) }) && commands.DryRun {
				req.Preview = false
				commands.Wrote("POST /mediadb/add", req)
			}
			return
		}
		if commands.JsonOut {
			commands.RenderOne(res, nil)
			return
		}
		commands.Ok(1, "added", []string{fmt.Sprintf("%s %s", res.Item.Title, dim(fmt.Sprintf("%s:%d", commands.BaseName(res.Filename), res.Line+1)))})
	case "show":
		kind, rest := libKind(words, false)
		it := self.find(self.items(core, kind), strings.Join(rest, " "))
		if self.Edit {
			core.LaunchEditor(it.Filename, it.Line+1)
			return
		}
		res, err := commands.SendReceiveGetErr[struct {
			offAnswer
			Items []libItem
		}](core, "mediadb/items", map[string]string{"hash": it.Hash})
		if err != nil {
			commands.Fail("%v", err)
		}
		failIfOff(res.offAnswer)
		if len(res.Items) > 0 {
			it = res.Items[0]
		}
		commands.RenderOne(it, func() {
			fmt.Printf("%s%s%s\n", commands.C(commands.AnsiBold), it.Title, commands.C(commands.AnsiReset))
			if a := itemAbout(it); a != "" {
				fmt.Println(dim(a))
			}
			for _, k := range mediadb.FieldNames(it.Kind) {
				if v := it.Fields[k]; v != "" && k != "STATUS" && k != "RATING" && k != "MEDIA_ID" {
					fmt.Printf("  %-16s %s\n", dim(strings.ToLower(strings.ReplaceAll(k, "_", " "))), v)
				}
			}
			if len(it.Tags) > 0 {
				fmt.Println("  " + dim(":"+strings.Join(it.Tags, ":")+":"))
			}
			if it.About != "" {
				fmt.Println()
				fmt.Println(it.About)
			}
			fmt.Println(dim(fmt.Sprintf("\n%s:%d", it.Filename, it.Line+1)))
		})
	case "set", "rate", "start", "finish", "drop":
		kind, rest := libKind(words, false)
		status := self.Status
		switch verb {
		case "start":
			status = "doing"
		case "finish":
			status = "done"
		case "drop":
			status = "dropped"
		case "rate":
			if len(rest) > 0 {
				if v, err := strconv.ParseFloat(rest[len(rest)-1], 64); err == nil {
					self.Rating, rest = v, rest[:len(rest)-1]
				}
			}
		}
		if status == "" && self.Rating < 0 {
			commands.Fail("orgs library set NAME -status read -rating 4")
		}
		it := self.find(self.items(core, kind), strings.Join(rest, " "))
		req := struct {
			Hash   string
			Status string
			Rating *float64
		}{Hash: it.Hash, Status: status}
		if self.Rating >= 0 {
			req.Rating = &self.Rating
		}
		type setAnswer struct {
			Ok   bool
			Off  bool
			Msg  string
			Item libItem
		}
		res, err := commands.SendReceivePostErr[struct {
			Hash   string
			Status string
			Rating *float64
		}, setAnswer](core, "mediadb/set", &req)
		if err == commands.ErrDryRun {
			return
		}
		if err != nil {
			commands.Fail("%v", err)
		}
		if !res.Ok {
			commands.Fail("%s", res.Msg)
		}
		commands.RenderOne(res.Item, func() {
			fmt.Printf("%s✓%s %s  %s\n", commands.C(commands.AnsiGreen), commands.C(commands.AnsiReset), res.Item.Title, dim(itemAbout(res.Item)))
		})
	case "refresh":
		kind, rest := libKind(words, false)
		it := self.find(self.items(core, kind), strings.Join(rest, " "))
		req := struct {
			Hash      string
			Overwrite bool
		}{it.Hash, self.Overwrite}
		type refreshAnswer struct {
			Ok       bool
			Off      bool
			Msg      string
			Hash     string
			Set      []string
			Warnings []string
		}
		res, err := commands.SendReceivePostErr[struct {
			Hash      string
			Overwrite bool
		}, refreshAnswer](core, "mediadb/refresh", &req)
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
		commands.RenderOne(res, func() { fmt.Println(res.Msg) })
	case "sources":
		res, err := commands.SendReceiveGetErr[struct {
			offAnswer
			Sources []struct {
				Name, Label string
				Kinds       []string
				Usable      bool
				Missing     []string
				OptionalKey bool
				Site        string
				Attribution string
			}
		}](core, "mediadb/sources", nil)
		if err != nil {
			commands.Fail("%v", err)
		}
		failIfOff(res.offAnswer)
		commands.Render(res.Sources, func() {
			for _, s := range res.Sources {
				mark := commands.C(commands.AnsiGreen) + "✓" + commands.C(commands.AnsiReset)
				note := ""
				if !s.Usable {
					mark = dim("·")
					note = "needs mediaDb.keys." + strings.Join(s.Missing, ", mediaDb.keys.")
				} else if s.OptionalKey {
					note = "a key is optional"
				}
				fmt.Printf("%s %-12s %-28s %s\n", mark, s.Name, dim(strings.Join(s.Kinds, ", ")), dim(note))
			}
		})
	default:
		commands.Fail("orgs library [ls|search|add|show|set|rate|start|finish|drop|refresh|sources] (unknown %q)", verb)
	}
}

func init() {
	commands.AddCmd("library", "books, films, shows, games and albums looked up and filed as headings, on shelves with stars",
		func() commands.Cmd { return &Library{} })
}
