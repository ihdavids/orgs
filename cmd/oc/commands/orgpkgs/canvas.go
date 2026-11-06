package orgpkgs

// Canvases (the JSON Canvas format), from the command line.
//
//	orgs canvas                          every .canvas under the org directories
//	orgs canvas show plan.canvas         groups, cards and edges as an outline
//	orgs canvas add plan TODO report     put a heading on it (a title, query or -hash)
//	orgs canvas add plan pics/map.png    a file (a picture, a pdf, an org file)
//	orgs canvas add plan https://x.org   a link
//	orgs canvas add plan -text 'An idea' a text card (- reads stdin)
//	orgs canvas add plan -group Later    a group   (-in GROUP -color green -x 0 -y 0 for any of them)
//	orgs canvas link plan Plans 'Book flights' -label then [-color 2] [-both|-none]
//	orgs canvas svg plan -out plan.svg   the board drawn as a picture
//	orgs canvas new boards/plan          an empty one
//
// Needs `orgs features canvas on`.

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common/jcanvas"
)

type Canvas struct {
	base
	tf    commands.TargetFlags
	Text  bool
	Group string
	In    string
	Color string
	Label string
	Both  bool
	None  bool
	Out   string
	Dark  bool
	X, Y  float64
	hasXY map[string]bool
}

func (self *Canvas) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	fset.BoolVar(&self.Text, "text", false, "add: the words are a text card (- reads stdin)")
	fset.StringVar(&self.Group, "group", "", "add: a group with this label")
	fset.StringVar(&self.In, "in", "", "add: inside this group (its label or id)")
	fset.StringVar(&self.Color, "color", "", "add, link: 1-6, red orange yellow green cyan purple, or #hex")
	fset.StringVar(&self.Label, "label", "", "link: the edge's label")
	fset.BoolVar(&self.Both, "both", false, "link: arrows at both ends")
	fset.BoolVar(&self.None, "none", false, "link: no arrows")
	fset.StringVar(&self.Out, "out", "", "svg: write here (default stdout)")
	fset.BoolVar(&self.Dark, "dark", false, "svg: dark cards")
	fset.Float64Var(&self.X, "x", 0, "add: where (left), instead of the first free spot")
	fset.Float64Var(&self.Y, "y", 0, "add: where (top)")
	commands.AddTargetFlags(fset, &self.tf)
}

type canvasInfo struct {
	File, Rel, Name, Mod, Msg string
	Nodes, Edges              int
}

type canvasRes struct {
	Ok       bool
	Off      bool
	Msg      string
	File     string
	Rel      string
	Version  string
	Conflict bool
	Canvas   json.RawMessage
	Cards    map[string]jcanvas.Card
	Problems []string
	Warnings []string
	Node     json.RawMessage
	Edge     json.RawMessage
	Canvases []canvasInfo
}

func canvasOk(res canvasRes, err error) canvasRes {
	if err == commands.ErrDryRun {
		os.Exit(0)
	}
	if err != nil {
		commands.Fail("%v", err)
	}
	if !res.Ok {
		commands.Fail("%s", res.Msg)
	}
	return res
}

func (self *Canvas) Exec(core *commands.Core) {
	self.hasXY = map[string]bool{}
	self.fset.Visit(func(f *flag.Flag) { self.hasXY[f.Name] = true })
	words := commands.FreeArgs(self.fset)
	verb := "ls"
	if len(words) > 0 {
		verb, words = words[0], words[1:]
	}
	switch verb {
	case "ls", "list":
		self.ls(core)
	case "show", "cat":
		self.show(core, words)
	case "add":
		self.add(core, words)
	case "link":
		self.link(core, words)
	case "svg":
		self.svg(core, words)
	case "new":
		self.newCanvas(core, words)
	default:
		// `orgs canvas plan.canvas` shows it.
		self.show(core, append([]string{verb}, words...))
	}
}

func (self *Canvas) ls(core *commands.Core) {
	res := canvasOk(commands.SendReceiveGetErr[canvasRes](core, "canvas/list", nil))
	commands.Render(res.Canvases, func() {
		if len(res.Canvases) == 0 {
			fmt.Println(dim("no canvases: orgs canvas new NAME"))
		}
		for _, c := range res.Canvases {
			if c.Msg != "" {
				fmt.Printf("%-30s %s\n", c.Rel, commands.C(commands.AnsiRed)+c.Msg+commands.C(commands.AnsiReset))
				continue
			}
			fmt.Printf("%-30s %s\n", c.Rel, dim(fmt.Sprintf("%d cards, %d edges", c.Nodes, c.Edges)))
		}
	})
}

func (self *Canvas) read(core *commands.Core, name string) (canvasRes, jcanvas.Canvas) {
	res := canvasOk(commands.SendReceiveGetErr[canvasRes](core, "canvas", map[string]string{"file": name, "html": "f"}))
	c, err := jcanvas.Parse(res.Canvas)
	if err != nil {
		commands.Fail("%v", err)
	}
	return res, c
}

func (self *Canvas) show(core *commands.Core, words []string) {
	if len(words) != 1 {
		commands.Fail("orgs canvas show CANVAS")
	}
	res, c := self.read(core, words[0])
	commands.RenderOne(res, func() {
		fmt.Println(commands.C(commands.AnsiBold) + res.Rel + commands.C(commands.AnsiReset) + dim(fmt.Sprintf("  %d cards, %d edges", len(c.Nodes), len(c.Edges))))
		fmt.Print(jcanvas.Outline(&c, res.Cards))
		for _, w := range res.Warnings {
			fmt.Fprintln(os.Stderr, w)
		}
	})
}

func looksLikeURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "mailto:")
}

type canvasAddReq struct {
	File, Kind, Hash, Path, Url, Text, Label, In, Color string
	X, Y                                                *float64
}

func (self *Canvas) add(core *commands.Core, words []string) {
	if len(words) < 1 {
		commands.Fail("orgs canvas add CANVAS (HEADING | FILE | URL | -text WORDS | -group LABEL)")
	}
	req := canvasAddReq{File: words[0], In: self.In, Color: self.Color}
	rest := words[1:]
	if self.hasXY["x"] {
		req.X = &self.X
	}
	if self.hasXY["y"] {
		req.Y = &self.Y
	}
	switch {
	case self.Group != "":
		req.Kind, req.Label = "group", self.Group
	case self.Text:
		req.Kind = "text"
		req.Text = strings.Join(rest, " ")
		if req.Text == "-" || (len(rest) == 0 && commands.StdinIsPipe()) {
			b, _ := io.ReadAll(os.Stdin)
			req.Text = strings.TrimRight(string(b), "\n")
		}
	case len(rest) == 1 && looksLikeURL(rest[0]):
		req.Kind, req.Url = "link", rest[0]
	case len(rest) == 1 && fileish(rest[0]):
		req.Kind, req.Path = "file", rest[0]
		if abs, err := filepath.Abs(rest[0]); err == nil {
			if _, err := os.Stat(abs); err == nil {
				req.Path = abs
			}
		}
	default:
		hs := commands.Resolve(core, &self.tf, rest, commands.TargetOpts{Prompt: "Put on " + filepath.Base(words[0]) + "> ", Quiet: commands.Machine()})
		if len(hs) == 0 {
			commands.Fail("no heading")
		}
		req.Kind = "heading"
		for i, h := range hs {
			req.Hash = h.Hash
			res := canvasOk(commands.SendReceivePostErr[canvasAddReq, canvasRes](core, "canvas/add", &req))
			self.added(res, i == len(hs)-1)
		}
		return
	}
	res := canvasOk(commands.SendReceivePostErr[canvasAddReq, canvasRes](core, "canvas/add", &req))
	self.added(res, true)
}

// A word that names a file rather than a heading: an existing path, or a
// name with an extension and no spaces.
func fileish(s string) bool {
	if _, err := os.Stat(s); err == nil {
		return true
	}
	return !strings.ContainsAny(s, " \t") && filepath.Ext(s) != "" && !strings.HasPrefix(s, ".") && !strings.ContainsAny(s, "()")
}

func (self *Canvas) added(res canvasRes, last bool) {
	commands.RenderOne(res, func() {
		var n map[string]interface{}
		json.Unmarshal(res.Node, &n)
		label := ""
		for _, c := range res.Cards {
			if c.Title != "" {
				label = c.Title
			}
		}
		if label == "" {
			for _, k := range []string{"label", "url", "file", "text"} {
				if s, ok := n[k].(string); ok && s != "" {
					label = s
					break
				}
			}
		}
		fmt.Printf("%s: added %s %s %s\n", filepath.Base(res.File), n["type"], label, dim(fmt.Sprintf("(%v, at %v,%v)", n["id"], n["x"], n["y"])))
	})
}

func (self *Canvas) link(core *commands.Core, words []string) {
	if len(words) != 3 {
		commands.Fail("orgs canvas link CANVAS FROM TO [-label TEXT] [-color C] [-both|-none]")
	}
	req := struct{ File, From, To, Label, Color, Ends string }{words[0], words[1], words[2], self.Label, self.Color, "->"}
	if self.Both {
		req.Ends = "<->"
	}
	if self.None {
		req.Ends = "--"
	}
	res := canvasOk(commands.SendReceivePostErr[struct{ File, From, To, Label, Color, Ends string }, canvasRes](core, "canvas/link", &req))
	commands.RenderOne(res, func() {
		var e map[string]interface{}
		json.Unmarshal(res.Edge, &e)
		fmt.Printf("%s: %s %s %s %s\n", filepath.Base(res.File), words[1], map[string]string{"->": "-->", "<->": "<-->", "--": "---"}[req.Ends], words[2], dim(fmt.Sprintf("(%v)", e["id"])))
	})
}

func (self *Canvas) svg(core *commands.Core, words []string) {
	if len(words) != 1 {
		commands.Fail("orgs canvas svg CANVAS [-out FILE]")
	}
	res, c := self.read(core, words[0])
	// Pictures are served by the server: a drawing read off disk needs its url.
	base := strings.TrimRight(core.Rest.Url, "/")
	for k, card := range res.Cards {
		if strings.HasPrefix(card.Src, "/") {
			card.Src = base + card.Src
			res.Cards[k] = card
		}
	}
	svg := jcanvas.SVG(&c, res.Cards, jcanvas.SvgOpts{IDPrefix: "cv", Dark: self.Dark})
	if self.Out == "" {
		fmt.Println(svg)
		return
	}
	if commands.Wrote("write "+self.Out, nil) {
		return
	}
	if err := os.WriteFile(self.Out, []byte(svg+"\n"), 0644); err != nil {
		commands.Fail("%v", err)
	}
	fmt.Fprintf(os.Stderr, "wrote %s\n", self.Out)
}

func (self *Canvas) newCanvas(core *commands.Core, words []string) {
	if len(words) != 1 {
		commands.Fail("orgs canvas new NAME   (relative to the first org directory)")
	}
	req := struct{ File string }{words[0]}
	res := canvasOk(commands.SendReceivePostErr[struct{ File string }, canvasRes](core, "canvas/new", &req))
	commands.RenderOne(res, func() { fmt.Println(res.File) })
}

func init() {
	commands.AddCmd("canvas", "boards of cards and arrows in .canvas files (JSON Canvas): ls, show, add, link, svg, new",
		func() commands.Cmd { return &Canvas{} })
}
