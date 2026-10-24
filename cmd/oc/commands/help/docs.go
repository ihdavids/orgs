package help

// The documentation `orgs help` shows beside a command's flags, carried in
// the binary. docs.json is written by docex from the SDOC blocks and the
// guides in docs/ (internal/docex.Bundle says what is in it); go generate
// here, or tools/builddocs.sh, writes it again, and TestDocsBundleIsCurrent
// fails when it no longer matches the sources.

//go:generate go run ../../../docex -src ../../../.. -bundle docs.json

import (
	_ "embed"
	"encoding/json"
	"sort"
	"strings"
	"sync"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/docex"
)

//go:embed docs.json
var docsJSON []byte

var bundle = sync.OnceValue(func() *docex.Bundle {
	b := &docex.Bundle{}
	json.Unmarshal(docsJSON, b)
	return b
})

// Docs is the bundle, for tests and other commands that want to say what a
// command or query function does.
func Docs() *docex.Bundle { return bundle() }

// Commands that take a query, by name; any other with a -query flag (or a -q
// flag its usage says is a query) is one too.
var queryCommands = map[string]bool{"search": true, "q": true, "filter": true, "agenda": true,
	"tui": true, "kanban": true, "gantt": true, "csv": true, "triage": true, "review": true}

func takesQuery(c *commands.CmdCreatorThunk) bool {
	if queryCommands[c.Name] {
		return true
	}
	if c.Flags == nil {
		return false
	}
	if c.Flags.Lookup("query") != nil {
		return true
	}
	if f := c.Flags.Lookup("q"); f != nil && strings.Contains(strings.ToLower(f.Usage), "query") {
		return true
	}
	return false
}

type queryFunc struct {
	Name, Doc string
}

// queryFuncs is every function a query may call - from the binary's own
// function map when it has the server in it, else the ones documented - each
// with its line from the Querying reference ("" when it has none).
func queryFuncs() []queryFunc {
	b := bundle()
	names := []string{}
	if commands.QueryFunctions != nil {
		names = commands.QueryFunctions()
	}
	if len(names) == 0 {
		for n := range b.QueryFuncs {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	out := []queryFunc{}
	for _, n := range names {
		out = append(out, queryFunc{Name: n, Doc: b.QueryFuncs[n]})
	}
	return out
}
