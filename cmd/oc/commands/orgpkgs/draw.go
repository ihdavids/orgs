package orgpkgs

// Drawings (Excalidraw, org-excalidraw), from the command line.
//
//	orgs draw ls                                 every drawing, newest first, and what links to it
//	orgs draw new flow                           an empty drawings/…/flow.excalidraw (from the first org dir)
//	orgs draw new flow -link-from 'Design'       and [[file:flow.excalidraw]] under that heading (picker without words)
//	orgs draw new flow -caption 'The flow' -link-from 'Design'
//	orgs draw svg flow.excalidraw -out flow.svg  the server's own drawing of it (no Excalidraw needed)
//	orgs draw open flow.excalidraw               worg's editor in the browser
//
// A drawing is named by its path from the first org directory, an absolute
// path, or by name when only one drawing has it. Needs `orgs features
// excalidraw on`.

import (
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/ihdavids/orgs/cmd/oc/commands"
)

type Draw struct {
	base
	tf       commands.TargetFlags
	Out      string
	LinkFrom string
	Caption  string
	Saved    bool
}

func (self *Draw) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	fset.StringVar(&self.Out, "out", "", "svg: write the picture here (default: stdout)")
	fset.StringVar(&self.LinkFrom, "link-from", "", "new: put a link to the drawing under this heading (a title, query or hash; - for the picker)")
	fset.StringVar(&self.Caption, "caption", "", "new: a #+CAPTION: above the link")
	fset.BoolVar(&self.Saved, "saved", false, "svg: the picture saved beside the drawing when it is fresh, else the server's (default: always the server's)")
	commands.AddTargetFlags(fset, &self.tf)
}

type drawLink struct {
	File string
	Line int
}

type drawInfo struct {
	File, Rel, Name, Format string
	Modified, Size          int64
	Svg                     string
	Fresh                   bool
	LinkedFrom              []drawLink
}

type drawList struct {
	Ok       bool
	Off      bool
	Msg      string
	Drawings []drawInfo
}

type drawNewReq struct {
	File, Hash, Caption string
}

type drawNewRes struct {
	Ok       bool
	Off      bool
	Msg      string
	File     string
	Link     string
	Filename string
	Line     int
}

func (self *Draw) Exec(core *commands.Core) {
	words := commands.FreeArgs(self.fset)
	verb := "ls"
	if len(words) > 0 {
		verb, words = words[0], words[1:]
	}
	switch verb {
	case "ls", "list":
		self.list(core)
	case "new":
		self.new(core, words)
	case "svg":
		self.svg(core, words)
	case "open":
		self.open(core, words)
	default:
		commands.Fail("orgs draw ls | new NAME [-link-from HEADING] | svg FILE [-out x.svg] | open FILE")
	}
}

func (self *Draw) drawings(core *commands.Core) []drawInfo {
	res, err := commands.SendReceiveGetErr[drawList](core, "excalidraw/list", nil)
	if err != nil {
		commands.Fail("%v", err)
	}
	failIfOff(offAnswer{res.Ok, res.Off, res.Msg})
	return res.Drawings
}

// which is the drawing a word names: a path the server knows, else the one
// drawing with that name or relative path.
func (self *Draw) which(core *commands.Core, words []string) string {
	if len(words) != 1 {
		commands.Fail("which drawing? orgs draw ls lists them")
	}
	w := words[0]
	if abs, err := filepath.Abs(w); err == nil {
		if _, err := os.Stat(abs); err == nil {
			return abs
		}
	}
	hits := []string{}
	for _, d := range self.drawings(core) {
		name := strings.TrimSuffix(strings.TrimSuffix(filepath.Base(d.File), ".md"), ".excalidraw")
		if d.File == w || d.Rel == w || filepath.Base(d.File) == w || name == w || strings.HasSuffix(d.Rel, "/"+w) {
			hits = append(hits, d.File)
		}
	}
	switch len(hits) {
	case 0:
		// Let the server say what is wrong with it.
		return w
	case 1:
		return hits[0]
	}
	commands.Fail("%s names %d drawings: %s", w, len(hits), strings.Join(hits, ", "))
	return ""
}

func (self *Draw) list(core *commands.Core) {
	ds := self.drawings(core)
	commands.Render(ds, func() {
		if len(ds) == 0 {
			fmt.Fprintln(os.Stderr, "no drawings yet: orgs draw new NAME")
			return
		}
		for _, d := range ds {
			pic := commands.C(commands.AnsiGreen) + "svg" + commands.C(commands.AnsiReset)
			if d.Svg == "" {
				pic = dim("---")
			} else if !d.Fresh {
				pic = commands.C(commands.AnsiGold) + "old" + commands.C(commands.AnsiReset)
			}
			kind := ""
			if d.Format == "obsidian" {
				kind = dim(" (markdown)")
			}
			fmt.Printf("%s  %s%s\n", pic, d.Rel, kind)
			for _, l := range d.LinkedFrom {
				fmt.Printf("     %s\n", dim(fmt.Sprintf("%s:%d", l.File, l.Line+1)))
			}
		}
	})
}

func (self *Draw) new(core *commands.Core, words []string) {
	if len(words) > 1 {
		commands.Fail("orgs draw new NAME (one name; quote one with spaces)")
	}
	req := drawNewReq{Caption: self.Caption}
	if len(words) == 1 {
		req.File = words[0]
	}
	if self.LinkFrom != "" || self.tf.Hash != "" {
		pick := []string{}
		if self.LinkFrom != "" && self.LinkFrom != "-" {
			pick = strings.Fields(self.LinkFrom)
			if len(pick) > 1 {
				pick = []string{self.LinkFrom}
			}
		}
		if len(pick) == 0 && self.tf.Hash == "" && commands.Machine() {
			commands.Fail("-link-from needs a heading when nobody is there to pick one")
		}
		hs := commands.Resolve(core, &self.tf, pick, commands.TargetOpts{Prompt: "Link the drawing from> "})
		req.Hash = hs[0].Hash
	}
	res, err := commands.SendReceivePostErr[drawNewReq, drawNewRes](core, "excalidraw/new", &req)
	if err == commands.ErrDryRun {
		return
	}
	if err != nil {
		commands.Fail("%v", err)
	}
	failIfOff(offAnswer{res.Ok, res.Off, res.Msg})
	commands.RenderOne(res, func() {
		fmt.Println(res.File)
		if res.Line >= 0 && res.Filename != "" {
			fmt.Printf("%s %s\n", dim(fmt.Sprintf("linked at %s:%d:", res.Filename, res.Line+1)), res.Link)
		} else {
			fmt.Printf("%s %s\n", dim("link it with"), res.Link)
		}
		if res.Msg != "" {
			fmt.Fprintln(os.Stderr, res.Msg)
		}
	})
}

func (self *Draw) svg(core *commands.Core, words []string) {
	file := self.which(core, words)
	ps := map[string]string{"file": file}
	if !self.Saved {
		ps["render"] = "server"
	}
	b, _, status, err := core.Rest.GetRaw("excalidraw/svg", ps)
	if err != nil {
		commands.Fail("%v", err)
	}
	if status != 200 || !strings.Contains(string(b), "<svg") {
		// A feature that is off answers json.
		msg := strings.TrimSpace(string(b))
		if strings.Contains(msg, `"Off":true`) {
			msg = "drawings are turned off on this server: orgs features excalidraw on"
		}
		commands.Fail("%s", msg)
	}
	if self.Out == "" || self.Out == "-" {
		os.Stdout.Write(b)
		return
	}
	if err := os.WriteFile(self.Out, b, 0644); err != nil {
		commands.Fail("%v", err)
	}
	commands.Wrote(self.Out, fmt.Sprintf("%d bytes", len(b)))
}

func (self *Draw) open(core *commands.Core, words []string) {
	file := self.which(core, words)
	// The drawing must exist and the feature be on before a browser opens on
	// a page that would only say so.
	type answer struct {
		Ok   bool
		Off  bool
		Msg  string
		File string
	}
	probe, err := commands.SendReceiveGetErr[answer](core, "excalidraw", map[string]string{"file": file})
	if err != nil {
		commands.Fail("%v", err)
	}
	failIfOff(offAnswer{probe.Ok, probe.Off, probe.Msg})
	u := strings.TrimRight(core.Rest.Url, "/") + "/?app=Drawings&drawing=" + url.QueryEscape(probe.File)
	if commands.Machine() {
		commands.RenderOne(map[string]string{"Url": u, "File": probe.File}, func() { fmt.Println(u) })
		return
	}
	fmt.Println(u)
	commands.OpenInBrowser(u)
}

func init() {
	commands.AddCmd("draw", "Excalidraw drawings linked from org: ls, new NAME [-link-from HEADING], svg FILE, open FILE (org-excalidraw)",
		func() commands.Cmd { return &Draw{} })
}
