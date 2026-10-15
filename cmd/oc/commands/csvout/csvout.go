package csvout

// orgs csv - a column view or a query's headings as a spreadsheet.
//
//	orgs csv project.org                       the file's column view, rollups and all
//	orgs csv project.org -columns '%ITEM %EFFORT{:} %CLOCKSUM'
//	orgs csv 'IsTodo() && HasTag("work")'      every heading a query finds
//	orgs csv -q stale -out stale.csv           a saved query (orgs q / worg)
//	orgs csv tasks.org -excel -out tasks.csv   with the mark Excel needs for utf-8
//
// Also `orgs cols project.org -csv` and `orgs q stale -csv`, which call Write.
//
// The rows are made by the server (GET /csv), from the same cells the
// Columns tab and `orgs cols` draw, so a summed EFFORT in the spreadsheet is
// the number on the screen.

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

// Options are the flags every way into a csv shares.
type Options struct {
	Columns  string
	Sep      string
	NoHeader bool
	Excel    bool
	Meta     bool
	Own      bool
	Out      string
	Open     bool
}

// Flags registers the options on a command's flag set; prefix keeps them
// apart from a command's own (cols has its own -columns).
func (o *Options) Flags(fset *flag.FlagSet, withColumns bool) {
	if withColumns {
		fset.StringVar(&o.Columns, "columns", "", "a #+COLUMNS: line to use instead of the file's own (or the default for a query)")
	}
	fset.StringVar(&o.Sep, "sep", "comma", "comma, semicolon or tab")
	fset.BoolVar(&o.NoHeader, "no-header", false, "leave off the header row")
	fset.BoolVar(&o.Excel, "excel", false, "start with the byte order mark Excel needs to read utf-8")
	fset.BoolVar(&o.Meta, "meta", false, "add Level, Line and Hash columns so rows can be found again")
	fset.BoolVar(&o.Own, "own", false, "each heading's own value, not a parent's rolled-up total")
	fset.StringVar(&o.Out, "out", "", "write the csv here (default: stdout)")
	fset.BoolVar(&o.Open, "open", false, "open the written file (with -out)")
}

// Write asks the server for a file's view (file) or a query's headings and
// delivers the csv.
func Write(core *commands.Core, file, query string, o Options) {
	ps := map[string]string{"columns": o.Columns, "sep": o.Sep}
	if file != "" {
		ps["file"] = file
	} else {
		ps["query"] = query
	}
	if o.NoHeader {
		ps["header"] = "f"
	}
	if o.Excel {
		ps["bom"] = "t"
	}
	if o.Meta {
		ps["meta"] = "t"
	}
	if o.Own {
		ps["own"] = "t"
	}
	body, h, status, err := core.Rest.GetRawHeader("csv", ps)
	if err != nil {
		commands.Fail("orgs csv: %v", err)
	}
	if status >= 400 {
		commands.Fail("orgs csv: %s", strings.TrimSpace(string(body)))
	}
	rows := h.Get("X-Orgs-Rows")
	if commands.JsonOut {
		commands.RenderOne(struct {
			Ok      bool
			Rows    string
			Columns string
			File    string `json:",omitempty"`
			Csv     string
		}{true, rows, h.Get("X-Orgs-Columns"), o.Out, string(body)}, nil)
		if o.Out == "" {
			return
		}
	}
	if o.Out == "" {
		os.Stdout.Write(body)
		return
	}
	if commands.Wrote(fmt.Sprintf("write %s rows to %s", rows, o.Out), nil) {
		return
	}
	if err := os.WriteFile(o.Out, body, 0o644); err != nil {
		commands.Fail("orgs csv: could not write %s: %v", o.Out, err)
	}
	if !commands.Machine() {
		fmt.Fprintf(os.Stderr, "wrote %s (%s rows; %s)\n", o.Out, rows, h.Get("X-Orgs-Columns"))
	}
	if o.Open {
		opener := "xdg-open"
		switch runtime.GOOS {
		case "darwin":
			opener = "open"
		case "windows":
			opener = "explorer"
		}
		exec.Command(opener, o.Out).Start()
	}
}

// SavedQuery is a stored query's text, by name.
func SavedQuery(core *commands.Core, name string) string {
	type stored struct {
		Name  string `json:"name"`
		Query string `json:"query"`
	}
	qs := commands.SendReceiveGetOr[[]stored](core, "ext/queries", nil)
	names := []string{}
	for _, q := range qs {
		if q.Name == name {
			return q.Query
		}
		names = append(names, q.Name)
	}
	sort.Strings(names)
	if len(names) == 0 {
		commands.Fail("no query called %s, and nothing is saved yet", name)
	}
	commands.Fail("no query called %s - there is %s", name, strings.Join(names, ", "))
	return ""
}

type Csv struct {
	fset  *flag.FlagSet
	opts  Options
	Saved string
}

func (self *Csv) Unmarshal(unmarshal func(interface{}) error) error { return unmarshal(self) }
func (self *Csv) StartPlugin(manager *common.PluginManager)         {}
func (self *Csv) HelpGroup() string                                 { return "Files and code" }

func (self *Csv) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	self.opts.Flags(fset, true)
	fset.StringVar(&self.Saved, "q", "", "a saved query, by name")
}

func (self *Csv) Exec(core *commands.Core) {
	words := commands.FreeArgs(self.fset)
	text := strings.TrimSpace(strings.Join(words, " "))
	switch {
	case self.Saved != "":
		Write(core, "", SavedQuery(core, self.Saved), self.opts)
	case text == "":
		commands.Fail("orgs csv <file.org | 'query'> [-q saved] [-columns '%%ITEM %%TODO'] [-out f.csv]")
	case len(words) == 1 && strings.HasSuffix(strings.ToLower(text), ".org"):
		Write(core, text, "", self.opts)
	default:
		Write(core, "", text, self.opts)
	}
}

func init() {
	commands.AddCmd("csv", "a file's column view, or a query's headings, as csv for a spreadsheet",
		func() commands.Cmd { return &Csv{} })
}
