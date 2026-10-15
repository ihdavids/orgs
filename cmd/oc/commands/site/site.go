package site

// orgs site - a folder of org files as a static web site.
//
//	orgs site blog -out ./public                 the site, written here
//	orgs site blog -out ./public -theme journal  in another theme
//	orgs site . -out ~/www -base-url https://me.example -clean -open
//	orgs site blog -local -out /srv/www          written on the server's disk
//	orgs site -list-themes
//
// The server builds the site (it has the files parsed and knows every link)
// and answers with a zip, which is unpacked here: as with orgs export, -out is
// this machine's unless -local says it is the server's.

import (
	"archive/zip"
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

// Options say what to build and where it goes.
type Options struct {
	Dir     string
	Out     string
	Theme   string
	Title   string
	BaseURL string
	Drafts  bool
	Clean   bool
	Open    bool
	Local   bool
}

type SiteCmd struct {
	fset       *flag.FlagSet
	o          Options
	listThemes bool
}

func (self *SiteCmd) Unmarshal(unmarshal func(interface{}) error) error { return unmarshal(self) }
func (self *SiteCmd) StartPlugin(manager *common.PluginManager)         {}
func (self *SiteCmd) HelpGroup() string                                 { return "Files and code" }

func (self *SiteCmd) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	fset.StringVar(&self.o.Out, "out", "", "the folder to write the site into")
	fset.StringVar(&self.o.Theme, "theme", "", "paper, ink, tufte, garden, docs, mono or journal (see -list-themes)")
	fset.StringVar(&self.o.Title, "title", "", "the site's title (default: site.yaml's, else the folder's name)")
	fset.StringVar(&self.o.BaseURL, "base-url", "", "where the site will live, for the feed and sitemap (https://me.example/notes)")
	fset.BoolVar(&self.o.Drafts, "drafts", false, "publish files marked #+DRAFT: t as well")
	fset.BoolVar(&self.o.Clean, "clean", false, "empty -out first (only when it holds a site made before)")
	fset.BoolVar(&self.o.Open, "open", false, "open the home page in a browser afterwards")
	fset.BoolVar(&self.o.Local, "local", false, "let the server write the site, at -out on its own disk")
	fset.BoolVar(&self.listThemes, "list-themes", false, "the themes a site can wear")
}

func (self *SiteCmd) Exec(core *commands.Core) {
	words := commands.FreeArgs(self.fset)
	if self.listThemes {
		ListThemes(core)
		return
	}
	if len(words) != 1 {
		commands.Fail("orgs site <folder> -out <where> [-theme paper] [-drafts] [-base-url URL] [-clean] [-open] [-local]\n" +
			"The folder is relative to the first org directory; . is all of it.")
	}
	self.o.Dir = words[0]
	Build(core, self.o)
}

// ThemeInfo is a theme as GET /site?list=t answers it.
type ThemeInfo struct {
	Name        string
	Description string
}

// ListThemes prints the themes the server has.
func ListThemes(core *commands.Core) {
	res, err := commands.SendReceiveGetErr[struct {
		Ok     bool
		Themes []ThemeInfo
	}](core, "site", map[string]string{"list": "t"})
	if err != nil {
		commands.Fail("orgs site: %v", err)
	}
	commands.Render(res.Themes, func() {
		w := 0
		for _, t := range res.Themes {
			if len(t.Name) > w {
				w = len(t.Name)
			}
		}
		for _, t := range res.Themes {
			fmt.Printf("%-*s  %s%s%s\n", w, t.Name, commands.C(commands.AnsiDim), t.Description, commands.C(commands.AnsiReset))
		}
	})
}

// Built is what -json prints.
type Built struct {
	Ok       bool
	Out      string
	Pages    string
	Files    int
	Theme    string
	Warnings []string `json:",omitempty"`
}

// Build asks the server for the site and writes it. orgs export -f site is
// this too.
func Build(core *commands.Core, o Options) {
	if strings.TrimSpace(o.Out) == "" {
		commands.Fail("orgs site: -out says where the site goes")
	}
	ps := map[string]string{"dir": o.Dir, "theme": o.Theme, "title": o.Title, "base": o.BaseURL}
	if o.Drafts {
		ps["drafts"] = "t"
	}
	if o.Local {
		ps["local"] = "t"
		ps["out"] = o.Out
		if commands.Wrote(fmt.Sprintf("build the site of %s into %s on the server", o.Dir, o.Out), ps) {
			return
		}
		res, err := commands.SendReceiveGetErr[common.ResultMsg](core, "site", ps)
		if err != nil {
			commands.Fail("orgs site: %v", err)
		}
		if !res.Ok {
			commands.Fail("orgs site: %s", res.Msg)
		}
		commands.RenderOne(res, func() { fmt.Fprintln(os.Stderr, res.Msg) })
		return
	}
	body, h, status, err := core.Rest.GetRawHeader("site", ps)
	if err != nil {
		commands.Fail("orgs site: %v", err)
	}
	if status >= 400 || !strings.Contains(h.Get("Content-Type"), "zip") {
		msg := strings.TrimSpace(string(body))
		if len(msg) > 400 {
			msg = msg[:400] + "…"
		}
		commands.Fail("orgs site: %s", msg)
	}
	z, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		commands.Fail("orgs site: the answer was not a zip: %v", err)
	}
	out := Built{Ok: true, Out: o.Out, Pages: h.Get("X-Orgs-Pages"), Files: len(z.File), Theme: h.Get("X-Orgs-Theme"), Warnings: h.Values("X-Orgs-Warning")}
	if commands.Wrote(fmt.Sprintf("write a site of %s pages (%d files) into %s", out.Pages, out.Files, o.Out), nil) {
		return
	}
	if o.Clean {
		if err := clean(o.Out); err != nil {
			commands.Fail("orgs site: %v", err)
		}
	}
	if err := unzip(z, o.Out); err != nil {
		commands.Fail("orgs site: %v", err)
	}
	if o.Open {
		abs, _ := filepath.Abs(filepath.Join(o.Out, "index.html"))
		commands.OpenInBrowser("file://" + filepath.ToSlash(abs))
	}
	if commands.RenderOne(out, nil) {
		return
	}
	g, d, r := commands.C(commands.AnsiGreen), commands.C(commands.AnsiDim), commands.C(commands.AnsiReset)
	fmt.Printf("%sBuilt%s %s pages in %s %s(%d files, theme %s)%s\n", g, r, out.Pages, o.Out, d, out.Files, out.Theme, r)
	for _, w := range out.Warnings {
		fmt.Printf("  %snote:%s %s\n", commands.C(commands.AnsiGold), r, w)
	}
	if !o.Open {
		fmt.Printf("  %sOpen it: %s%s\n", d, filepath.Join(o.Out, "index.html"), r)
	}
}

// clean empties a folder that holds a site made before, and refuses one that
// does not look like one: -clean next to a mistyped -out must not empty a
// home directory.
func clean(dir string) error {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) || (err == nil && len(entries) == 0) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, mark := range []string{"index.html", filepath.Join("assets", "site.css")} {
		if _, err := os.Stat(filepath.Join(dir, mark)); err != nil {
			return fmt.Errorf("-clean: %s does not look like a site made before (no %s), so it was left alone", dir, mark)
		}
	}
	for _, e := range entries {
		if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

func unzip(z *zip.Reader, dir string) error {
	for _, f := range z.File {
		name := path.Clean(f.Name)
		if strings.HasPrefix(name, "../") || name == ".." || path.IsAbs(name) || strings.Contains(name, "\\") {
			return fmt.Errorf("the site holds a path outside itself: %s", f.Name)
		}
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return err
		}
		if err := os.WriteFile(p, data, 0644); err != nil {
			return err
		}
	}
	return nil
}

func init() {
	commands.AddCmd("site", "a folder of org files as a static web site, in a theme", func() commands.Cmd {
		return &SiteCmd{}
	})
}
