package orgs

import "github.com/ihdavids/orgs/cmd/oc/commands"

// `orgs help search` lists the query functions this binary has, from the very
// map a query is evaluated with, so the list cannot drift from the language.
// The commands cannot import this package (it imports them), so they are
// handed the function here.
func init() {
	commands.QueryFunctions = queryFunctionNames
}
