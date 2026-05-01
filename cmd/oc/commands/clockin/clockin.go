package clockin

import (
	"flag"
	"fmt"
	"log"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
	"github.com/koki-develop/go-fzf"
)

type ClockIn struct {
	Hash string
}

func (self *ClockIn) Unmarshal(unmarshal func(interface{}) error) error {
	return unmarshal(self)
}

func (self *ClockIn) StartPlugin(manager *common.PluginManager) {
}

func (self *ClockIn) SetupParameters(fset *flag.FlagSet) {
	fset.StringVar(&self.Hash, "hash", "", "hash of heading to clock into")
}

func (self *ClockIn) Exec(core *commands.Core) {
	hash := self.Hash
	if hash == "" {
		// Get all headings from the server
		var qry map[string]string = map[string]string{}
		var headings common.Todos
		commands.SendReceiveGet(core, "filecontents/headings", qry, &headings)
		if len(headings) == 0 {
			fmt.Println("No headings found")
			return
		}

		// Build display strings for fzf
		labels := make([]string, len(headings))
		for i, h := range headings {
			prefix := ""
			if h.Status != "" {
				prefix = h.Status + " "
			}
			labels[i] = fmt.Sprintf("%s%s  (%s)", prefix, h.Headline, h.Filename)
		}

		f, err := fzf.New(
			fzf.WithPrompt("Clock into> "),
			fzf.WithCountViewEnabled(true),
			fzf.WithCountView(func(meta fzf.CountViewMeta) string {
				return fmt.Sprintf("headings: %d", meta.ItemsCount)
			}),
		)
		if err != nil {
			log.Fatal(err)
		}
		idxs, err := f.Find(labels, func(i int) string { return labels[i] })
		if err != nil {
			log.Fatal(err)
		}
		if len(idxs) == 0 {
			fmt.Println("No heading selected")
			return
		}
		hash = headings[idxs[0]].Hash
	}

	// Clock in via the server (this also clocks out any active clock)
	target := common.Target{
		Id:   hash,
		Type: "hash",
	}
	var reply common.ResultMsg
	commands.SendReceivePost(core, "clockin", &target, &reply)
	if reply.Ok {
		fmt.Printf("OK: %s\n", reply.Msg)
	} else {
		fmt.Printf("Err: %s\n", reply.Msg)
	}
}

// init function is called at boot
func init() {
	commands.AddCmd("clockin", "clock into a heading selected via fzf",
		func() commands.Cmd {
			return &ClockIn{}
		})
}
