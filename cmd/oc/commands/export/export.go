package export

// Run a file through an exporter.
//
//	orgs export -f html notes.org -out notes.html
//	orgs export -f html notes.org -theme docs -stdout
//	orgs export -f pdf adventure.org -out adventure.pdf
//	orgs export -f mermaid -query 'IsTask()' -out plan.mmd
//	orgs export -list                    what this server can export to
//	orgs export -list-themes             what the html exporter can wear
//
// Two things to know before changing anything here.
//
// The **query** is the exporter's argument, not a filter this command applies:
// a file exporter reads it as the file to export and a query exporter (gantt,
// mermaid) reads it as the expression to select headings with. That is why it
// can be written as a bare word after the flags - for the exporters people
// reach for most, it is a filename.
//
// Whether the **server** or **this machine** writes the answer is `-local`,
// and the default changed. It used to be the server, which is right for a
// server and a client on the same machine and surprising everywhere else:
// `orgs -url https://box:8010 export -out ./notes.html` wrote notes.html on
// box. Now the bytes come back and are written here unless -local is asked
// for, and -local says out loud that the path is the server's.

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

type Export struct {
	fset *flag.FlagSet

	Filename   string
	Query      string
	Format     string
	Theme      string
	Parent     string
	Local      bool
	Stdout     bool
	Printable  bool
	Refresh    bool
	FileLinks  bool
	HttpsLinks bool
	List       bool
	ListThemes bool
}

func (self *Export) Unmarshal(unmarshal func(interface{}) error) error { return unmarshal(self) }
func (self *Export) StartPlugin(manager *common.PluginManager)         {}

func (self *Export) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	fset.StringVar(&self.Filename, "out", "", "write the answer here (default: stdout)")
	fset.StringVar(&self.Query, "query", "", "the file to export, or the expression to select headings")
	fset.StringVar(&self.Format, "f", "html", "html, latex, pdf, revealjs, impressjs, mermaid, gantt, tangle, dndsheet …")
	fset.StringVar(&self.Theme, "theme", "", "html theme to render with (see -list-themes)")
	fset.StringVar(&self.Parent, "parent", "", "parent identifier, for the exporters that take one")
	fset.BoolVar(&self.Local, "local", false, "let the server write the file, at -out on its own disk")
	fset.BoolVar(&self.Stdout, "stdout", false, "print the answer even when -out was given")
	fset.BoolVar(&self.Printable, "printable", false, "the printable variant, where the exporter has one")
	fset.BoolVar(&self.Refresh, "refresh", false, "build again rather than answering from the pdf cache")
	fset.BoolVar(&self.FileLinks, "filelinks", false, "write links as file:// urls")
	fset.BoolVar(&self.HttpsLinks, "httpslinks", false, "write links as https urls to this server")
	fset.BoolVar(&self.List, "list", false, "what this server can export to")
	fset.BoolVar(&self.ListThemes, "list-themes", false, "what the html exporter can wear")
}

func (self *Export) Exec(core *commands.Core) {
	if free := commands.FreeText(self.fset); free != "" && self.Query == "" {
		self.Query = free
	}
	switch {
	case self.List:
		self.list(core)
		return
	case self.ListThemes:
		self.themes(core)
		return
	}
	if strings.TrimSpace(self.Query) == "" {
		commands.Fail("orgs export: nothing to export.\n\n%s", usage)
	}
	if strings.EqualFold(self.Format, "pdf") {
		self.pdf(core)
		return
	}
	self.run(core)
}

// ---------------------------------------------------------------------------

func (self *Export) list(core *commands.Core) {
	res, err := commands.SendReceiveGetErr[struct {
		Ok    bool
		Names []string
	}](core, "exporters", nil)
	if err != nil {
		commands.Fail("orgs export: %v", err)
	}
	// pdf is not an exporter plugin from a client's point of view - it is its
	// own endpoint, because the answer is bytes and an exporter answers with a
	// string. It is a format you can ask for, so it belongs in the list.
	names := append([]string{}, res.Names...)
	if !has(names, "pdf") {
		names = append(names, "pdf")
	}
	commands.Render(names, func() {
		for _, n := range names {
			fmt.Println(n)
		}
	})
}

func (self *Export) themes(core *commands.Core) {
	res, err := commands.SendReceiveGetErr[struct {
		Ok     bool
		Themes []string
	}](core, "html/themes", nil)
	if err != nil {
		commands.Fail("orgs export: %v", err)
	}
	commands.Render(res.Themes, func() {
		for _, t := range res.Themes {
			fmt.Println(t)
		}
	})
}

// ---------------------------------------------------------------------------

func (self *Export) run(core *commands.Core) {
	ps := map[string]string{
		"query":  self.Query,
		"parent": self.Parent,
		"theme":  self.Theme,
	}
	if self.Printable {
		ps["printable"] = "t"
	}
	if self.FileLinks {
		ps["filelinks"] = "t"
	}
	if self.HttpsLinks {
		ps["httpslinks"] = "t"
	}

	if self.Local {
		if self.Filename == "" {
			commands.Fail("orgs export -local: -out says where on the server to write it")
		}
		ps["local"] = "t"
		ps["filename"] = self.Filename
		if commands.Wrote(fmt.Sprintf("export %s → %s on the server", self.Query, self.Filename), ps) {
			return
		}
		res, err := commands.SendReceiveGetErr[common.ResultMsg](core, "file/"+self.Format, ps)
		if err != nil {
			commands.Fail("orgs export: %v", err)
		}
		if !res.Ok {
			commands.Fail("orgs export: %s", strings.TrimSpace(res.Msg))
		}
		commands.RenderOne(res, func() {
			fmt.Fprintf(os.Stderr, "wrote %s on the server\n", self.Filename)
		})
		return
	}

	ps["local"] = "f"
	res, err := commands.SendReceiveGetErr[common.ResultMsg](core, "file/"+self.Format, ps)
	if err != nil {
		commands.Fail("orgs export: %v", err)
	}
	if !res.Ok {
		// The exporter puts its refusal in the same field it puts the answer
		// in, so this is the only place the two are told apart.
		commands.Fail("orgs export: %s", strings.TrimSpace(res.Msg))
	}
	self.deliver([]byte(res.Msg))
}

// A pdf is the one format that is not an exporter call: the pdf exporter
// writes a file and returns nothing to a string, so /pdf is its own endpoint
// answering with the bytes. It is cached against what the file was when it was
// built - path, modification time and size - so asking twice builds once.
func (self *Export) pdf(core *commands.Core) {
	ps := map[string]string{"filename": self.Query}
	if self.Refresh {
		ps["refresh"] = "t"
	}
	body, ctype, status, err := core.Rest.GetRaw("pdf", ps)
	if err != nil {
		commands.Fail("orgs export: %v", err)
	}
	if status >= 400 || !strings.Contains(ctype, "pdf") {
		msg := strings.TrimSpace(string(body))
		if len(msg) > 400 {
			msg = msg[:400] + "…"
		}
		commands.Fail("orgs export -f pdf: %s", msg)
	}
	self.deliver(body)
}

// Where the bytes end up: a file when -out was given, stdout otherwise, and
// both when -stdout was asked for as well.
func (self *Export) deliver(body []byte) {
	if self.Filename == "" || self.Stdout {
		os.Stdout.Write(body)
		if len(body) > 0 && body[len(body)-1] != '\n' {
			fmt.Println()
		}
		if self.Filename == "" {
			return
		}
	}
	if commands.Wrote(fmt.Sprintf("write %d bytes to %s", len(body), self.Filename), nil) {
		return
	}
	if err := os.WriteFile(self.Filename, body, 0644); err != nil {
		commands.Fail("orgs export: could not write %s: %v", self.Filename, err)
	}
	fmt.Fprintf(os.Stderr, "wrote %s (%d bytes)\n", self.Filename, len(body))
}

func has(vs []string, want string) bool {
	for _, v := range vs {
		if strings.EqualFold(v, want) {
			return true
		}
	}
	return false
}

const usage = `  orgs export -f html notes.org -out notes.html
  orgs export -f html notes.org -theme docs        (to stdout)
  orgs export -f pdf  adventure.org -out book.pdf
  orgs export -f mermaid -query 'IsTask()' -out plan.mmd
  orgs export -list          what this server can export to
  orgs export -list-themes   what the html exporter can wear

-out writes here. -local writes on the server instead, at that path on its
own disk.`

func init() {
	commands.AddCmd("export", "run a file or a query through an exporter",
		func() commands.Cmd { return &Export{Format: "html"} })
}
