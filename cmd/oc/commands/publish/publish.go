package publish

// orgs publish - put a page on GitHub Pages.
//
//	orgs publish -list                          the targets server.publish names
//	orgs publish plan.html -to site             a page saved here (worg's gantt "Save as HTML")
//	orgs publish notes.org -to site             a file through the html exporter, then published
//	orgs publish notes.org -to site -as plans/notes.html -theme docs
//	orgs publish -f gantt 'IsProject()' -to site -as launch.html
//
// orgs export -publish site is the same as the last three. The server does
// the upload (one commit through GitHub's contents api) with the token in its
// keyring, so nothing here needs git or a token.

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

type PublishCmd struct {
	fset    *flag.FlagSet
	to      string
	as      string
	format  string
	theme   string
	message string
	list    bool
	open    bool
}

func (self *PublishCmd) Unmarshal(unmarshal func(interface{}) error) error { return unmarshal(self) }
func (self *PublishCmd) StartPlugin(manager *common.PluginManager)         {}
func (self *PublishCmd) HelpGroup() string                                 { return "Files and code" }

func (self *PublishCmd) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	fset.StringVar(&self.to, "to", "", "the target, by name (may be left out when the server has one)")
	fset.StringVar(&self.as, "as", "", "the file's path in the target's folder (default: the file's name with .html)")
	fset.StringVar(&self.format, "f", "html", "the exporter, for an org file or a query: html, gantt, revealjs ...")
	fset.StringVar(&self.theme, "theme", "", "the html theme")
	fset.StringVar(&self.message, "m", "", "the commit message")
	fset.BoolVar(&self.list, "list", false, "the targets the server can publish to")
	fset.BoolVar(&self.open, "open", false, "open the published page in a browser afterwards")
}

func (self *PublishCmd) Exec(core *commands.Core) {
	words := commands.FreeArgs(self.fset)
	if self.list {
		ListTargets(core)
		return
	}
	if len(words) == 0 {
		commands.Fail("orgs publish <page.html | file.org | query> [-to target] [-as name.html] [-f html|gantt|...]\n" +
			"orgs publish -list    where the server can publish to")
	}
	what := strings.Join(words, " ")
	req := common.PublishRequest{Target: self.to, Path: self.as, Message: self.message}
	// A page saved here is sent as it is; anything else is the server's to
	// export. A .html that is not a file here is a mistake, not a query.
	if ext := strings.ToLower(filepath.Ext(what)); ext == ".html" || ext == ".htm" {
		data, err := os.ReadFile(what)
		if err != nil {
			commands.Fail("orgs publish: %v", err)
		}
		req.Html = string(data)
		if req.Path == "" {
			req.Path = filepath.Base(what)
		}
	} else {
		req.Format, req.Query, req.Theme = self.format, what, self.theme
	}
	res := Send(core, &req)
	if self.open && res.Ok && res.Url != "" {
		commands.OpenInBrowser(res.Url)
	}
}

// ListTargets prints what GET /publish/targets answers.
func ListTargets(core *commands.Core) {
	res, err := commands.SendReceiveGetErr[common.PublishTargets](core, "publish/targets", nil)
	if err != nil {
		commands.Fail("orgs publish: %v", err)
	}
	commands.Render(res.Targets, func() {
		if len(res.Targets) == 0 {
			fmt.Fprintln(os.Stderr, "no publish targets: add one under server.publish in the server's yaml")
			return
		}
		w := 0
		for _, t := range res.Targets {
			w = max(w, len(t.Name))
		}
		for _, t := range res.Targets {
			where := t.Repo
			if t.Branch != "" {
				where += "@" + t.Branch
			}
			if t.Dir != "" {
				where += ":" + t.Dir
			}
			fmt.Printf("%-*s  %s  %s%s%s\n", w, t.Name, where, commands.C(commands.AnsiDim), t.Url, commands.C(commands.AnsiReset))
		}
	})
}

// Send asks the server to publish and says how it went. orgs export -publish
// is this too.
func Send(core *commands.Core, req *common.PublishRequest) common.PublishResult {
	// The page itself is not worth echoing on -dry-run; its size is.
	what := req.Query
	if req.Html != "" {
		what = fmt.Sprintf("a %d byte page", len(req.Html))
	}
	if commands.Wrote(fmt.Sprintf("publish %s to %s as %s", what, targetWord(req.Target), pathWord(req)), nil) {
		return common.PublishResult{}
	}
	res, err := common.RestPost[common.PublishResult](&core.Rest, "publish", req)
	if err != nil {
		commands.Fail("orgs publish: %v", err)
	}
	if !res.Ok {
		commands.Fail("orgs publish: %s", res.Msg)
	}
	commands.RenderOne(res, func() {
		for _, w := range res.Warnings {
			fmt.Fprintf(os.Stderr, "warning: %s\n", w)
		}
		if res.Unchanged {
			fmt.Fprintf(os.Stderr, "%s was already up to date\n", res.Path)
		} else {
			fmt.Fprintf(os.Stderr, "published %s (%d bytes, commit %.7s)\n", res.Path, res.Bytes, res.Commit)
		}
		// The url is the answer; GitHub Pages takes a minute to rebuild.
		fmt.Println(res.Url)
	})
	return res
}

func targetWord(t string) string {
	if t == "" {
		return "the server's publish target"
	}
	return t
}

func pathWord(req *common.PublishRequest) string {
	if req.Path != "" {
		return req.Path
	}
	return common.PublishName(req.Query)
}

func init() {
	commands.AddCmd("publish", "put an exported page (html, gantt, slides) on GitHub Pages",
		func() commands.Cmd { return &PublishCmd{format: "html"} })
}
