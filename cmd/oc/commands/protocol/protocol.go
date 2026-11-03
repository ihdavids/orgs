//lint:file-ignore ST1006 allow the use of self
package protocol

// `orgs protocol` - the handler for org-protocol:// links, Emacs's way for a
// browser to hand a page to org.
//
//	orgs protocol 'org-protocol://capture?template=w&url=…'   what the system runs on a click
//	orgs protocol install          make orgs the handler for org-protocol:// links
//	orgs protocol uninstall        stop being it
//	orgs protocol status           which program handles them now
//	orgs protocol bookmarklets     bookmarklets for this server (-temp NAME)
//	orgs protocol links            the links store-link has kept, newest first
//
// The parsing is common.ParseOrgProtocol, which reads both spellings org has
// used (`?template=w&url=…` and the old `capture:/w/URL/TITLE/BODY`).
//
//   - capture opens worg's capture, filled in from the page, through a one-use
//     login link (POST /login/ticket), so the browser is logged in without
//     anybody typing a password. A capture always goes through a form a person
//     sees: any web page can open an org-protocol link, so nothing is filed
//     from one unseen.
//   - store-link keeps the link on the server (POST /protocol/links), where
//     worg, VS Code's Insert Link and `orgs protocol links` find it, and puts
//     `[[url][title]]` on the clipboard.
//   - open-source asks the server which file a published page came from
//     (GET /protocol/source, the protocolProjects: setting) and opens it in
//     the editor when it is on this disk, in worg when the server holds it.
//
// When it runs from a click nobody is watching a terminal, so a failure is a
// desktop notification as well as a line on stderr.
//
// The same links work without anything installed by swapping
// `org-protocol://` for `https://<server>/org-protocol/`: the server parses
// them the same way and sends the browser to worg. `bookmarklets` prints both.

import (
	"flag"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
	"github.com/ihdavids/orgs/internal/common/desktop"
	"golang.org/x/term"
)

const defaultScheme = "org-protocol"

type Protocol struct {
	Print    bool
	Template string
	Scheme   string
	fset     *flag.FlagSet
}

func (self *Protocol) Unmarshal(unmarshal func(interface{}) error) error { return unmarshal(self) }
func (self *Protocol) StartPlugin(manager *common.PluginManager)         {}
func (self *Protocol) HelpGroup() string                                 { return "Changing a heading" }

// Not offline, but it must get past the token check on the way in: run from a
// click, an expired token would otherwise end it with a line on a stderr
// nobody sees. A refused login ticket falls back to worg's login page instead.
func (self *Protocol) NeedsNoServer() bool { return true }

func (self *Protocol) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	fset.BoolVar(&self.Print, "print", false, "print where the link leads instead of opening it")
	fset.StringVar(&self.Template, "temp", "", "bookmarklets: the capture template they name (default: ask in worg)")
	fset.StringVar(&self.Scheme, "scheme", defaultScheme, "the scheme to install for or read links in (for a second handler, or trying it out)")
}

func (self *Protocol) Exec(core *commands.Core) {
	args := commands.FreeArgs(self.fset)
	if len(args) == 0 {
		commands.Fail("orgs protocol: give it an org-protocol:// link, or one of install, uninstall, status, bookmarklets, links")
	}
	switch args[0] {
	case "install":
		self.install(core)
	case "uninstall":
		self.uninstall()
	case "status":
		self.status()
	case "bookmarklets":
		self.bookmarklets(core)
	case "links":
		self.links(core)
	default:
		self.handle(core, args[0])
	}
}

// A person is watching when there is a terminal; a click has none.
func watched() bool {
	return term.IsTerminal(int(os.Stderr.Fd()))
}

// fail tells whoever ran it: stderr always, and a desktop notification when
// that was a click.
func fail(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	if !watched() && !commands.Machine() {
		desktop.Send(desktop.Note{Title: "orgs: org-protocol", Body: msg})
	}
	commands.Fail("%s", msg)
}

func tell(title, msg string) {
	fmt.Fprintln(os.Stderr, msg)
	if !watched() && !commands.Machine() {
		desktop.Send(desktop.Note{Title: title, Body: msg})
	}
}

func (self *Protocol) handle(core *commands.Core, link string) {
	// A second handler installed under another scheme reads its own links
	// as org-protocol ones.
	if self.Scheme != defaultScheme && strings.HasPrefix(strings.ToLower(link), strings.ToLower(self.Scheme)+":") {
		link = defaultScheme + link[len(self.Scheme):]
	}
	p, err := common.ParseOrgProtocol(link)
	if err != nil {
		fail("%v", err)
	}
	switch p.Kind {
	case common.ProtoCapture:
		self.open(core, p.WorgPath())
	case common.ProtoStoreLink:
		self.storeLink(core, p)
	case common.ProtoOpenSource:
		self.openSource(core, p)
	}
}

// open puts a worg page in the browser, logged in when the server will give a
// login link and at worg's own login otherwise (which keeps the page's
// parameters, so the capture is still there afterwards).
func (self *Protocol) open(core *commands.Core, path string) {
	base := strings.TrimRight(core.Rest.Url, "/")
	if base == "" {
		fail("no server url in the config (url:), so there is no worg to open")
	}
	link := base + path
	if !self.Print {
		type ticketResponse struct {
			Ticket string `json:"ticket"`
			Url    string `json:"url"`
		}
		resp, err := commands.SendReceivePostErr[struct{}, ticketResponse](core, "login/ticket", &struct{}{})
		if err == commands.ErrDryRun {
			return
		}
		if err == nil && resp.Url != "" {
			link = base + resp.Url + "?next=" + url.QueryEscape(path)
		}
	}
	if self.Print || commands.Machine() {
		fmt.Println(link)
		return
	}
	commands.OpenInBrowser(link)
}

func (self *Protocol) storeLink(core *commands.Core, p common.OrgProtocol) {
	text := common.OrgLinkText(p.Url, p.Title)
	res, err := commands.SendReceivePostErr[common.StoredLink, common.StoredLinkResult](core, "protocol/links",
		&common.StoredLink{Url: p.Url, Title: p.Title})
	if err == commands.ErrDryRun {
		return
	}
	stored := err == nil && res.Ok
	if stored {
		text = res.Link
	}
	_, cerr := commands.CopyText(text)
	if commands.Machine() {
		commands.RenderOne(res, func() {})
		return
	}
	name := p.Title
	if name == "" {
		name = p.Url
	}
	switch {
	case stored && cerr == nil:
		tell("orgs: link stored", "Stored "+name+" - and "+text+" is on the clipboard")
	case stored:
		tell("orgs: link stored", "Stored "+name+" (the clipboard: "+cerr.Error()+")")
	case cerr == nil:
		tell("orgs: link copied", "The server did not keep it ("+reason(err, res.Msg)+"), but "+text+" is on the clipboard")
	default:
		fail("could not store %s: %s; and the clipboard: %v", name, reason(err, res.Msg), cerr)
	}
}

func reason(err error, msg string) string {
	if err != nil {
		return err.Error()
	}
	if msg != "" {
		return msg
	}
	return "no answer"
}

func (self *Protocol) openSource(core *commands.Core, p common.OrgProtocol) {
	res, err := commands.SendReceiveGetErr[common.ProtocolSourceResult](core, "protocol/source", map[string]string{"url": p.Url})
	if err != nil {
		fail("open-source: %v", err)
	}
	if !res.Ok {
		fail("open-source: %s", res.Msg)
	}
	if commands.Machine() {
		commands.RenderOne(res, func() {})
		return
	}
	if self.Print {
		fmt.Println(res.File)
		return
	}
	// The server's path is this machine's when the file is here: the editor.
	if _, err := os.Stat(res.File); err == nil && len(core.EditorTemplate) > 0 {
		core.LaunchEditor(res.File, 1)
		return
	}
	if res.Known {
		self.open(core, "/?app=Files&open="+url.QueryEscape(res.File))
		return
	}
	fail("open-source: the page comes from %s on the server, which is not on this machine and not an org file worg can open", res.File)
}

func (self *Protocol) links(core *commands.Core) {
	var list []common.StoredLink
	commands.SendReceiveGet(core, "protocol/links", map[string]string{}, &list)
	commands.Render(list, func() {
		if len(list) == 0 {
			fmt.Fprintln(os.Stderr, "no stored links - a store-link bookmarklet keeps them (orgs protocol bookmarklets)")
			return
		}
		for _, l := range list {
			fmt.Printf("%s  %s%s%s\n", l.Link, commands.C(commands.AnsiDim), l.At.Local().Format("Jan 2 15:04"), commands.C(commands.AnsiReset))
		}
	})
}

func (self *Protocol) bookmarklets(core *commands.Core) {
	base := strings.TrimRight(core.Rest.Url, "/")
	tq := ""
	if self.Template != "" {
		tq = "template=" + url.QueryEscape(self.Template) + "&"
	}
	page := "encodeURIComponent(location.href)"
	title := "encodeURIComponent(document.title)"
	sel := "encodeURIComponent(String(window.getSelection()))"
	capQ := "'capture?" + tq + "url='+" + page + "+'&title='+" + title + "+'&body='+" + sel
	linkQ := "'store-link?url='+" + page + "+'&title='+" + title
	srcQ := "'open-source?url='+" + page
	type mark struct{ Name, Scheme, Server string }
	// The scheme's links leave the page where it is (the system hands them
	// over); the server's open worg in a small window of its own, which closes
	// itself once the capture is filed.
	win := func(q string) string {
		return "javascript:void(window.open('" + base + "/org-protocol/'+" + q + ",'orgs-protocol','width=720,height=680'))"
	}
	loc := func(q string) string { return "javascript:location.href='org-protocol://'+" + q }
	marks := []mark{
		{"Capture", loc(capQ), win(capQ)},
		{"Store link", loc(linkQ), win(linkQ)},
		{"Open source", loc(srcQ), win(srcQ)},
	}
	commands.Render(marks, func() {
		fmt.Printf("%sThrough the org-protocol:// handler (orgs protocol install, or Emacs):%s\n\n",
			commands.C(commands.AnsiBold), commands.C(commands.AnsiReset))
		for _, m := range marks {
			fmt.Printf("%s:\n%s\n\n", m.Name, m.Scheme)
		}
		fmt.Printf("%sStraight to this server, nothing installed (phones too):%s\n\n",
			commands.C(commands.AnsiBold), commands.C(commands.AnsiReset))
		for _, m := range marks {
			fmt.Printf("%s:\n%s\n\n", m.Name, m.Server)
		}
		fmt.Fprintln(os.Stderr, "Make a bookmark and paste one of these as its address.")
	})
}

func init() {
	commands.AddCmd("protocol", "org-protocol:// links: orgs protocol LINK | install | uninstall | status | bookmarklets | links", func() commands.Cmd {
		return &Protocol{}
	})
}
