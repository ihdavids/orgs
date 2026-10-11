package importcmd

// orgs import - other formats into org files.
//
//	orgs import analysis.ipynb                     beside the others, in the first org directory
//	orgs import analysis.ipynb -o science/penguins.org
//	orgs import analysis.ipynb -overwrite -run     import again, then run every block
//	orgs import analysis.ipynb -print              the org text, and nothing written
//
// The file is read here and sent, because it is usually on this machine and
// the server may be another. The extension says what the file is; each kind
// is an Importer, so another format is one more Register call (the vCard one
// could hang off this as .vcf) rather than another command.

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

// Options are the flags every importer is handed.
type Options struct {
	Out       string
	Overwrite bool
	Print     bool
	Run       bool
	Open      bool
}

// An Importer is one kind of file orgs import reads.
type Importer struct {
	// What a person calls it: "Jupyter notebook".
	Kind string
	// The extensions that mean it, with the dot: ".ipynb".
	Exts []string
	// Import the file's bytes; name is the file's name as given.
	Import func(core *commands.Core, name string, data []byte, o *Options)
}

var importers = []Importer{}

// Register adds a kind of file to orgs import.
func Register(imp Importer) { importers = append(importers, imp) }

func importerFor(name, as string) (*Importer, bool) {
	ext := strings.ToLower(filepath.Ext(name))
	if as != "" {
		ext = "." + strings.TrimPrefix(strings.ToLower(as), ".")
	}
	for i := range importers {
		for _, e := range importers[i].Exts {
			if e == ext {
				return &importers[i], true
			}
		}
	}
	return nil, false
}

func kinds() string {
	out := []string{}
	for _, imp := range importers {
		out = append(out, fmt.Sprintf("%s (%s)", imp.Kind, strings.Join(imp.Exts, ", ")))
	}
	sort.Strings(out)
	return strings.Join(out, "; ")
}

type ImportCmd struct {
	fset *flag.FlagSet
	opts Options
	As   string
}

func (self *ImportCmd) Unmarshal(unmarshal func(interface{}) error) error { return unmarshal(self) }
func (self *ImportCmd) StartPlugin(manager *common.PluginManager)         {}
func (self *ImportCmd) HelpGroup() string                                 { return "Files and code" }

func (self *ImportCmd) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	fset.StringVar(&self.opts.Out, "o", "", "the org file to write: relative to the first org directory, or absolute inside one (default: the file's own name)")
	fset.BoolVar(&self.opts.Overwrite, "overwrite", false, "replace a file (and its pictures) that is already there")
	fset.BoolVar(&self.opts.Print, "print", false, "print the org text and write nothing")
	fset.BoolVar(&self.opts.Run, "run", false, "run every source block once it is imported (babel has to be on)")
	fset.BoolVar(&self.opts.Open, "open", false, "open the new file in your editor")
	fset.StringVar(&self.As, "as", "", "what the file is, when its extension does not say (e.g. ipynb); needed for -")
}

func (self *ImportCmd) Exec(core *commands.Core) {
	words := commands.FreeArgs(self.fset)
	if len(words) != 1 {
		commands.Fail("orgs import <file> [-o target.org] [-overwrite] [-run] [-print]\nKnows: %s", kinds())
	}
	name := words[0]
	imp, ok := importerFor(name, self.As)
	if !ok {
		commands.Fail("orgs import does not know what %s is. It knows: %s", name, kinds())
	}
	var data []byte
	var err error
	if name == "-" {
		data, err = io.ReadAll(os.Stdin)
		name = ""
	} else {
		data, err = os.ReadFile(name)
	}
	if err != nil {
		commands.Fail("could not read %s: %v", name, err)
	}
	imp.Import(core, name, data, &self.opts)
}

func init() {
	commands.AddCmd("import", "turn another kind of file into an org file: a Jupyter notebook (.ipynb)", func() commands.Cmd {
		return &ImportCmd{}
	})
	Register(Importer{Kind: "Jupyter notebook", Exts: []string{".ipynb"}, Import: importNotebook})
}

// ----------------------------------------------------------------------------
// Jupyter notebooks
// ----------------------------------------------------------------------------

func importNotebook(core *commands.Core, name string, data []byte, o *Options) {
	req := common.IpynbImport{Notebook: string(data), Name: filepath.Base(name), Filename: o.Out, Overwrite: o.Overwrite}
	if name == "" {
		req.Name = ""
	}
	// A dry run is the server converting and writing nothing, which says more
	// than the request would (it is mostly the notebook, base64 and all).
	dry := o.Print || commands.DryRun
	req.DryRun = dry
	res, err := common.RestPost[common.IpynbImported](&core.Rest, "import/ipynb", &req)
	if err != nil {
		commands.Fail("%v", err)
	}
	if !res.Ok {
		if res.Msg == "" {
			res.Msg = "the server did not import it (is it new enough to have /import/ipynb?)"
		}
		commands.Fail("%s", res.Msg)
	}
	if o.Print && !commands.Machine() {
		fmt.Print(res.Org)
		return
	}
	if dry {
		commands.Wrote("import "+res.Filename, nil)
	} else {
		if o.Run {
			run := common.BabelExec{Filename: res.Filename, All: true}
			ran, err := commands.SendReceivePostErr[common.BabelExec, common.BabelExecResult](core, "babel/exec", &run)
			switch {
			case errors.Is(err, commands.ErrDryRun):
			case err != nil:
				fmt.Fprintf(os.Stderr, "imported, but running it failed: %v\n", err)
			case !ran.Ok:
				fmt.Fprintf(os.Stderr, "imported; running it: %s\n", ran.Msg)
			}
		}
		if o.Open {
			core.LaunchEditor(res.Filename, 1)
		}
	}
	if commands.RenderOne(res, nil) {
		return
	}
	g, d, r := commands.C(commands.AnsiGreen), commands.C(commands.AnsiDim), commands.C(commands.AnsiReset)
	verb := "Imported"
	if dry {
		verb = "Would import"
	}
	from := filepath.Base(name)
	if from == "" || from == "." {
		from = "the notebook"
	}
	fmt.Printf("%s%s%s %s → %s\n", g, verb, r, from, res.Filename)
	fmt.Printf("  %q · %s, session %s\n", res.Title, res.Language, res.Session)
	fmt.Printf("  %s: %s, %s, %s", plural(res.Cells, "cell"), plural(res.Blocks, "source block"),
		plural(res.Outputs, "result"), plural(res.Images, "picture"))
	if res.Images > 0 && len(res.Files) > 0 {
		fmt.Printf(" %s(in %s/)%s", d, filepath.Base(filepath.Dir(res.Files[0])), r)
	}
	fmt.Println()
	if res.Errors > 0 && !o.Run {
		fmt.Printf("  %s ended in an error, kept as the notebook had it\n", plural(res.Errors, "cell"))
	}
	for _, w := range res.Warnings {
		fmt.Printf("  %snote:%s %s\n", commands.C(commands.AnsiGold), r, w)
	}
	if !dry && !o.Run {
		fmt.Printf("  %sRun it: orgs code all %s%s\n", d, commands.Shq(res.Filename), r)
	}
}

func plural(n int, what string) string {
	if n == 1 {
		return "1 " + what
	}
	return fmt.Sprintf("%d %ss", n, what)
}
