package orgpkgs

// Backlink counts (features.backlinkCounts), from the command line.
//
//	orgs linkcounts notes.org             the file's outline, ↩ 3 beside each linked-to heading
//	orgs linkcounts notes.org -mentions   ≈ 2 for unlinked mentions too
//	orgs linkcounts notes.org -links      every link in the file and how many others go where it goes
//	orgs linkcounts -top                  the most linked headings in the database (-limit 20)
//	orgs links counts notes.org           the same, as a verb of orgs links
//	orgs linkcounts notes.org -json
//
// Needs `orgs features backlinkCounts on`.

import (
	"flag"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

type LinkCounts struct {
	base
	Mentions bool
	Top      bool
	Links    bool
	All      bool
	Limit    int
}

func (self *LinkCounts) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	fset.BoolVar(&self.Mentions, "mentions", false, "count unlinked mentions too (slower)")
	fset.BoolVar(&self.Top, "top", false, "the most linked headings in the database")
	fset.BoolVar(&self.Links, "links", false, "every link written in the file, with how many other links go to the same place")
	fset.BoolVar(&self.All, "all", false, "with a file: every heading, not only the ones something links to")
	fset.IntVar(&self.Limit, "limit", 20, "with -top: at most this many (0 = all)")
}

func lcBadge(back, men int) string {
	s := ""
	if back > 0 {
		s += commands.C(commands.AnsiCyan) + fmt.Sprintf("↩ %d", back) + commands.C(commands.AnsiReset)
	}
	if men > 0 {
		if s != "" {
			s += " "
		}
		s += commands.C(commands.AnsiGold) + fmt.Sprintf("≈ %d", men) + commands.C(commands.AnsiReset)
	}
	return s
}

func (self *LinkCounts) Exec(core *commands.Core) {
	words := commands.FreeArgs(self.fset)
	if self.Top {
		self.top(core)
		return
	}
	if len(words) != 1 {
		commands.Fail("orgs linkcounts FILE [-mentions] [-links] [-all], or orgs linkcounts -top")
	}
	file := commands.ResolveOrgFile(core, words[0])
	ps := map[string]string{"filename": file}
	if self.Mentions {
		ps["mentions"] = "t"
	}
	res, err := commands.SendReceiveGetErr[common.LinkCounts](core, "links/counts", ps)
	if err != nil {
		commands.Fail("%v", err)
	}
	if !res.Ok {
		commands.Fail("%s", res.Msg)
	}
	if self.Links {
		commands.Render(res.Links, func() {
			for _, l := range res.Links {
				to := l.ToHeadline
				if to == "" && l.ToFilename != "" {
					to = filepath.Base(l.ToFilename)
				}
				if to == "" {
					to = l.Kind
				}
				n := ""
				if l.Count > 0 {
					n = "  " + lcBadge(l.Count, 0)
				}
				fmt.Printf("%s %s %s%s\n", dim(fmt.Sprintf("%4d", l.Line+1)), l.Raw, dim("→ "+to), n)
			}
		})
		return
	}
	commands.RenderOne(res, func() {
		head := filepath.Base(res.Filename)
		if b := lcBadge(res.File.Backlinks, res.File.Mentions); b != "" {
			head += "  " + b
		}
		fmt.Println(commands.C(commands.AnsiBold) + head + commands.C(commands.AnsiReset))
		shown := 0
		for _, h := range res.Headings {
			if !self.All && h.Backlinks == 0 && h.Mentions == 0 {
				continue
			}
			shown++
			line := strings.Repeat("  ", h.Level-1) + strings.Repeat("*", h.Level) + " " + h.Title
			if b := lcBadge(h.Backlinks, h.Mentions); b != "" {
				line += "  " + b
			}
			if h.Links > h.Backlinks {
				line += dim(fmt.Sprintf("  (%d links)", h.Links))
			}
			fmt.Printf("%s %s\n", dim(fmt.Sprintf("%4d", h.Line+1)), line)
		}
		if shown == 0 {
			fmt.Println(dim("  nothing links to a heading here (-all for the whole outline)"))
		}
	})
}

func (self *LinkCounts) top(core *commands.Core) {
	ps := map[string]string{"limit": fmt.Sprint(self.Limit)}
	if self.Mentions {
		ps["mentions"] = "t"
	}
	res, err := commands.SendReceiveGetErr[common.LinkCountTopResult](core, "links/counts/top", ps)
	if err != nil {
		commands.Fail("%v", err)
	}
	if !res.Ok {
		commands.Fail("%s", res.Msg)
	}
	commands.Render(res.Headings, func() {
		for _, h := range res.Headings {
			path := strings.Join(h.Olp, " / ")
			if path == "" {
				path = h.Title
			}
			// Padded by what shows, not by the colour codes around it.
			plain := strings.TrimSpace(fmt.Sprintf("↩ %d", h.Backlinks) + map[bool]string{true: fmt.Sprintf(" ≈ %d", h.Mentions), false: ""}[h.Mentions > 0])
			pad := strings.Repeat(" ", max(1, 12-len([]rune(plain))))
			fmt.Printf("%s%s%s  %s\n", lcBadge(h.Backlinks, h.Mentions), pad, path, dim(fmt.Sprintf("%s:%d", filepath.Base(h.Filename), h.Line+1)))
		}
		if res.Total > len(res.Headings) {
			fmt.Println(dim(fmt.Sprintf("%d of %d linked headings (-limit 0 for all)", len(res.Headings), res.Total)))
		}
	})
}

func init() {
	commands.AddCmd("linkcounts", "how many places link to each heading of a file, or the most linked headings",
		func() commands.Cmd { return &LinkCounts{} })
}
