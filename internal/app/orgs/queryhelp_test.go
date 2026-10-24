package orgs

import (
	"sort"
	"testing"

	"github.com/ihdavids/orgs/cmd/oc/commands/help"
)

// Every function a query may call has a line in the Querying reference (the
// SDOC block at the top of todo.go), which `orgs help search` and the man
// pages show beside it; and the reference names no function that is gone.
// A new function needs its `- *Name* - what it does` line, then
// `go generate ./cmd/oc/commands/help/`.
func TestQueryFunctionsDocumented(t *testing.T) {
	docs := help.Docs().QueryFuncs
	have := map[string]bool{}
	missing := []string{}
	for _, name := range queryFunctionNames() {
		have[name] = true
		if docs[name] == "" {
			missing = append(missing, name)
		}
	}
	gone := []string{}
	for name := range docs {
		if !have[name] {
			gone = append(gone, name)
		}
	}
	sort.Strings(gone)
	if len(missing) > 0 {
		t.Errorf("query functions with no line in the Querying reference: %v", missing)
	}
	if len(gone) > 0 {
		t.Errorf("the Querying reference names functions a query cannot call: %v", gone)
	}
}
