package orgpkgs

// Link titles (features.autoLinkTitle, org-cliplink), from the
// command line.
//
//	orgs link-title https://youtu.be/x        [[https://youtu.be/x][Never Gonna Give You Up]]
//	orgs link-title -title URL                 the title alone
//	pbpaste | orgs link-title                  every url on stdin, a link per line
//	orgs link-title -filter < notes.org        the text back with its bare urls titled (an editor filter)
//	orgs link-title -retitle notes.org         the bare urls of a file given titles: the diff, then ask
//	orgs link-title -retitle notes.org -dry-run   the diff only
//
// The server fetches (and caches) every title. Needs `orgs features autoLinkTitle on`.

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

type LinkTitle struct {
	base
	TitleOnly bool
	Retitle   string
	Filter    bool
	Refresh   bool
	Yes       bool
}

func init() {
	commands.AddCmd("link-title", "a web page's title as an org link: link-title URL... (or urls on stdin), -retitle FILE, -filter",
		func() commands.Cmd { return &LinkTitle{} })
}

func (self *LinkTitle) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	fset.BoolVar(&self.TitleOnly, "title", false, "print the title alone, not the link")
	fset.StringVar(&self.Retitle, "retitle", "", "give the bare urls and [[url]] links of this org file their titles (shows the diff, then asks)")
	fset.BoolVar(&self.Filter, "filter", false, "read org text on stdin and write it back with its bare urls titled (writes nothing)")
	fset.BoolVar(&self.Refresh, "refresh", false, "fetch again, not from the server's cache")
	fset.BoolVar(&self.Yes, "yes", false, "with -retitle: write without asking")
}

var linkTitleStdinUrlRe = regexp.MustCompile(`https?://[^\s\[\]<>"']+`)

func (self *LinkTitle) Exec(core *commands.Core) {
	words := commands.FreeArgs(self.fset)
	switch {
	case self.Retitle != "":
		self.retitle(core, commands.ResolveOrgFile(core, self.Retitle))
		return
	case self.Filter:
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			commands.Fail("%v", err)
		}
		req := common.RetitleRequest{Text: string(b), Refresh: self.Refresh}
		res, err := commands.SendReceivePostErr[common.RetitleRequest, common.RetitleAnswer](core, "link/retitle", &req)
		if err == commands.ErrDryRun {
			return
		}
		if err != nil {
			commands.Fail("orgs link-title: %v", err)
		}
		if !res.Ok {
			commands.Fail("%s", res.Msg)
		}
		if commands.RenderOne(res, nil) {
			return
		}
		fmt.Print(res.Text)
		linkTitleMisses(res.Links)
		return
	}
	if len(words) == 0 {
		if commands.Interactive() {
			commands.Fail("orgs link-title URL... (or urls on stdin, -retitle FILE, -filter)")
		}
		sc := bufio.NewScanner(os.Stdin)
		sc.Buffer(make([]byte, 1<<20), 16<<20)
		for sc.Scan() {
			for _, u := range linkTitleStdinUrlRe.FindAllString(sc.Text(), -1) {
				words = append(words, strings.TrimRight(u, ".,;:!?)"))
			}
		}
		if len(words) == 0 {
			commands.Fail("no urls on stdin")
		}
	}
	var titles []common.LinkTitle
	if len(words) == 1 {
		params := map[string]string{"url": words[0]}
		if self.Refresh {
			params["refresh"] = "1"
		}
		t, err := commands.SendReceiveGetErr[common.LinkTitle](core, "link/title", params)
		if err != nil {
			commands.Fail("orgs link-title: %v", err)
		}
		if t.Off {
			commands.Fail("%s", t.Msg)
		}
		titles = []common.LinkTitle{t}
	} else {
		req := common.LinkTitlesRequest{Urls: words, Refresh: self.Refresh}
		// A read, so not a POST -dry-run should stop: asked for directly.
		res, err := common.RestPost[common.LinkTitlesAnswer](&core.Rest, "link/titles", &req)
		if err != nil {
			commands.Fail("orgs link-title: %v", err)
		}
		if !res.Ok {
			commands.Fail("%s", res.Msg)
		}
		titles = res.Titles
	}
	commands.Render(titles, func() {
		for _, t := range titles {
			if self.TitleOnly {
				fmt.Println(t.Title)
			} else {
				fmt.Println(t.Link)
			}
			if !t.Ok && t.Msg != "" {
				fmt.Fprintf(os.Stderr, "%s %s\n", dim(t.Url+":"), t.Msg)
			}
		}
	})
}

func linkTitleMisses(links []common.RetitleLink) {
	for _, l := range links {
		if !l.Ok && l.Msg != "" {
			fmt.Fprintf(os.Stderr, "%s %s\n", dim(fmt.Sprintf("line %d, %s:", l.Line+1, l.Url)), l.Msg)
		}
	}
}

func linkTitleColourPatch(p string) string {
	if !commands.Colour() {
		return p
	}
	var sb strings.Builder
	for _, l := range strings.SplitAfter(p, "\n") {
		if l == "" {
			continue
		}
		code := ""
		switch {
		case strings.HasPrefix(l, "+++"), strings.HasPrefix(l, "---"):
			code = commands.AnsiBold
		case strings.HasPrefix(l, "@@"):
			code = commands.AnsiCyan
		case strings.HasPrefix(l, "+"):
			code = commands.AnsiGreen
		case strings.HasPrefix(l, "-"):
			code = commands.AnsiRed
		}
		if code == "" {
			sb.WriteString(l)
			continue
		}
		sb.WriteString(commands.C(code) + strings.TrimRight(l, "\n") + commands.C(commands.AnsiReset) + "\n")
	}
	return sb.String()
}

// retitle: the preview (a POST that writes nothing, so it runs under
// -dry-run too), the diff, the question, the apply with the preview's titles.
func (self *LinkTitle) retitle(core *commands.Core, file string) {
	req := common.RetitleRequest{File: file, Refresh: self.Refresh}
	pre, err := common.RestPost[common.RetitleAnswer](&core.Rest, "link/retitle", &req)
	if err != nil {
		commands.Fail("orgs link-title: %v", err)
	}
	if !pre.Ok {
		commands.Fail("%s", pre.Msg)
	}
	if commands.JsonOut && (commands.DryRun || pre.Changed == 0) {
		commands.RenderOne(pre, nil)
		return
	}
	if !commands.Machine() {
		fmt.Print(linkTitleColourPatch(pre.Patch))
		linkTitleMisses(pre.Links)
		fmt.Fprintf(os.Stderr, "%s%d of %d links can be given a title%s\n", commands.C(commands.AnsiBold), pre.Changed, len(pre.Links), commands.C(commands.AnsiReset))
	}
	if pre.Changed == 0 || commands.DryRun {
		return
	}
	if !commands.Confirm(&commands.TargetFlags{Yes: self.Yes}, "Write this to %s?", commands.BaseName(pre.File)) {
		fmt.Fprintln(os.Stderr, "Nothing written.")
		return
	}
	titles := map[string]string{}
	for _, l := range pre.Links {
		if l.Ok {
			titles[l.Url] = l.Title
		}
	}
	app := common.RetitleRequest{File: pre.File, Apply: true, Hash: pre.Hash, Titles: titles}
	done, err := commands.SendReceivePostErr[common.RetitleRequest, common.RetitleAnswer](core, "link/retitle", &app)
	if err == commands.ErrDryRun {
		return
	}
	if err != nil {
		commands.Fail("orgs link-title: %v", err)
	}
	if !done.Ok {
		commands.Fail("%s", done.Msg)
	}
	if commands.RenderOne(done, nil) {
		return
	}
	commands.Ok(1, "", []string{fmt.Sprintf("%d links titled in %s", done.Changed, commands.BaseName(done.File))})
}
