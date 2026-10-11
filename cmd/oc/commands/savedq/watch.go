package savedq

// orgs q watch / unwatch / watches - the terminal half of /watches.
//
//	orgs q watch blocked                  tell me when "blocked" finds something new
//	orgs q watch blocked -left -desktop   ...and when something leaves it, with a popup
//	orgs q watch soon 'DeadlineWithin(2) && IsTodo()'
//	orgs q unwatch blocked
//	orgs q watches
//
// A name that is a saved query watches that query, read afresh at every check,
// so editing the query here or in worg edits what is watched. A name followed
// by an expression is a watch of its own with no saved query behind it.
// What the watches find is `orgs notifications`.

import (
	"fmt"
	"strings"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

func (self *Q) watch(core *commands.Core, words []string) {
	if len(words) == 0 {
		commands.Fail("orgs q watch <saved query> | orgs q watch <name> '<query>'")
	}
	w := common.QueryWatch{Name: words[0], Label: self.Label, Left: self.Left, Desktop: self.Desktop}
	if len(words) > 1 {
		w.Query = strings.Join(words[1:], " ")
	} else {
		w.StoredQuery = words[0]
	}
	reply, err := commands.SendReceivePostErr[common.QueryWatch, common.WatchList](core, "watches", &w)
	if err == commands.ErrDryRun {
		return
	}
	if err != nil {
		commands.Fail("could not watch %s: %v", w.Name, err)
	}
	if !reply.Ok {
		commands.Fail("could not watch %s: %s", w.Name, reply.Msg)
	}
	for _, st := range reply.Watches {
		if st.Name != w.Name {
			continue
		}
		if commands.RenderOne(st, nil) {
			return
		}
		fmt.Printf("%s✓%s watching %s%s%s  %s%d match%s now%s\n", commands.C(commands.AnsiGreen),
			commands.C(commands.AnsiReset), commands.C(commands.AnsiBold), st.Title(),
			commands.C(commands.AnsiReset), commands.C(commands.AnsiDim), st.Count, plural(st.Count, "es"),
			commands.C(commands.AnsiReset))
		if st.Desktop && !reply.Desktop {
			fmt.Printf("%s  -desktop asks for a popup, but the server has no notify plugin in server.plugins%s\n",
				commands.C(commands.AnsiGold), commands.C(commands.AnsiReset))
		}
		fmt.Printf("%s  new matches arrive in orgs notifications, and live in orgs notifications -follow%s\n",
			commands.C(commands.AnsiDim), commands.C(commands.AnsiReset))
	}
}

func (self *Q) unwatch(core *commands.Core, words []string) {
	if len(words) == 0 {
		commands.Fail("orgs q unwatch <name>")
	}
	for _, name := range words {
		reply, err := commands.SendReceiveDelete[common.WatchList](core, "watches",
			map[string]string{"name": name})
		if err == commands.ErrDryRun {
			continue
		}
		if err != nil || !reply.Ok {
			commands.Fail("could not stop watching %s: %v %s", name, err, reply.Msg)
		}
		fmt.Printf("%s✓%s stopped watching %s\n", commands.C(commands.AnsiGreen),
			commands.C(commands.AnsiReset), name)
	}
}

func (self *Q) watches(core *commands.Core) {
	reply, err := commands.SendReceiveGetErr[common.WatchList](core, "watches", nil)
	if err != nil {
		commands.Fail("could not read the watches: %v", err)
	}
	ws := reply.Watches
	if commands.Render(ws, nil) {
		return
	}
	if len(ws) == 0 {
		fmt.Printf("nothing is being watched.\n\n  orgs q watch <saved query>\n")
		return
	}
	width := 0
	for _, w := range ws {
		if len(w.Title()) > width {
			width = len(w.Title())
		}
	}
	for _, w := range ws {
		count := "     ?"
		if w.Count >= 0 {
			count = fmt.Sprintf("%6d", w.Count)
		}
		flags := []string{}
		if w.Left {
			flags = append(flags, "left")
		}
		if w.Desktop {
			flags = append(flags, "desktop")
		}
		if w.Paused {
			flags = append(flags, "paused")
		}
		extra := ""
		if len(flags) > 0 {
			extra = " [" + strings.Join(flags, ",") + "]"
		}
		what := w.Resolved
		if w.StoredQuery != "" {
			what = "q " + w.StoredQuery
		}
		fmt.Printf("%s%-*s%s %s %s%s%s%s\n", commands.C(commands.AnsiBold), width, w.Title(),
			commands.C(commands.AnsiReset), count, commands.C(commands.AnsiDim), what, extra,
			commands.C(commands.AnsiReset))
		if w.Error != "" {
			fmt.Printf("%s  %s%s\n", commands.C(commands.AnsiRed), w.Error, commands.C(commands.AnsiReset))
		}
	}
	if !reply.Desktop {
		fmt.Printf("%s(no notify plugin on the server: desktop popups are off)%s\n",
			commands.C(commands.AnsiDim), commands.C(commands.AnsiReset))
	}
}

func plural(n int, suffix string) string {
	if n == 1 {
		return ""
	}
	return suffix
}
