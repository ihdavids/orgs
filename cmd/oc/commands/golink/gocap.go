package golink

// `orgs gocap`: save a link for `orgs go` to find.
//
//	orgs gocap                                  asks for the link, then the description
//	orgs gocap https://go.dev/ref/spec          asks for the description
//	orgs gocap https://go.dev/ref/spec Go spec  asks nothing
//
// It files through the server's capture, using the built-in GoLink template,
// which by default goes under * Links in links.org (goLinksFile and
// goLinksHeading under server: in orgs.yaml). Each link becomes a heading
// named by its description, with the time it was added:
//
//	** Go spec
//	   :PROPERTIES:
//	   :ADDED:    [2026-10-02 Fri 11:40]
//	   :END:
//	   [[https://go.dev/ref/spec][Go spec]]

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

// The server's name for the template; see GoLinkTemplate in capture.go.
const goLinkTemplate = "GoLink"

type GoCap struct {
	fset *flag.FlagSet
}

func (self *GoCap) Unmarshal(unmarshal func(interface{}) error) error { return unmarshal(self) }
func (self *GoCap) StartPlugin(manager *common.PluginManager)         {}
func (self *GoCap) SetupParameters(fset *flag.FlagSet)                { self.fset = fset }

func (self *GoCap) Exec(core *commands.Core) {
	words := commands.FreeArgs(self.fset)
	link, desc := "", ""
	if len(words) > 0 {
		link = words[0]
		desc = strings.Join(words[1:], " ")
	}

	in := bufio.NewReader(os.Stdin)
	ask := func(prompt string) string {
		if !commands.Interactive() {
			commands.Fail("orgs gocap: no %s given: orgs gocap <link> <description>", strings.ToLower(prompt))
		}
		fmt.Fprintf(os.Stderr, "%s%s:%s ", commands.C(commands.AnsiBold), prompt, commands.C(commands.AnsiReset))
		line, err := in.ReadString('\n')
		if err != nil && line == "" {
			fmt.Fprintln(os.Stderr)
			os.Exit(1)
		}
		return strings.TrimSpace(line)
	}
	if link = strings.TrimSpace(link); link == "" {
		link = ask("Link")
	}
	if link == "" {
		commands.Fail("orgs gocap: nothing saved, no link")
	}
	if strings.ContainsAny(link, "[]") {
		commands.Fail("orgs gocap: a link cannot hold [ or ] in org; percent-encode them (%%5B, %%5D)")
	}
	if desc = strings.TrimSpace(desc); desc == "" {
		desc = ask("Description")
	}
	if desc == "" {
		commands.Fail("orgs gocap: nothing saved, no description - it is the name orgs go finds the link by")
	}
	// A ] in the description would end the link early.
	desc = strings.NewReplacer("[", "(", "]", ")").Replace(desc)

	alreadySaved(core, link)

	var temps []common.CaptureTemplate
	commands.SendReceiveGet(core, "capture/templates", map[string]string{}, &temps)
	var tpl *common.CaptureTemplate
	for i := range temps {
		if temps[i].Name == goLinkTemplate {
			tpl = &temps[i]
		}
	}
	if tpl == nil {
		commands.Fail("orgs gocap: the server has no %s capture template; it needs restarting on a newer orgs", goLinkTemplate)
	}

	values := map[string]string{}
	for _, f := range common.CapFields(tpl.Template) {
		values[f.Key] = f.Default
	}
	values["url"] = link
	values["name"] = desc

	var query common.Capture
	query.Template = tpl.Name
	query.NewNode.Headline = desc
	query.NewNode.Content = common.FillCapTemplate(tpl.Template, values)

	reply, err := commands.SendReceivePostErr[common.Capture, common.ResultMsg](core, "capture", &query)
	if err == commands.ErrDryRun {
		return
	}
	if err != nil {
		commands.Fail("orgs gocap: %v", err)
	}
	if !reply.Ok {
		commands.Fail("orgs gocap: %s", reply.Msg)
	}
	commands.RenderOne(reply, func() {
		fmt.Printf("%sSaved%s %s → %s / %s\n", commands.C(commands.AnsiGreen), commands.C(commands.AnsiReset),
			desc, tpl.CapTarget.Filename, tpl.CapTarget.Id)
	})
}

// alreadySaved says, without stopping anything, when this link is already
// written somewhere under a name.
func alreadySaved(core *commands.Core, link string) {
	list, err := commands.SendReceiveGetErr[common.LinkList](core, "links/all", nil)
	if err != nil {
		return
	}
	for _, l := range list.Links {
		if l.Raw == link && strings.TrimSpace(l.Desc) != "" && l.Desc != l.Raw {
			fmt.Fprintf(os.Stderr, "%snote:%s already saved as %q in %s:%d\n",
				commands.C(commands.AnsiGold), commands.C(commands.AnsiReset), l.Desc, commands.BaseName(l.Filename), l.Line+1)
			return
		}
	}
}

func init() {
	commands.AddCmd("gocap", "save a link for orgs go: orgs gocap [link] [description]",
		func() commands.Cmd { return &GoCap{} })
}
