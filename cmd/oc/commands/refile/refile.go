//lint:file-ignore ST1006 allow the use of self
package capture

import (
	"flag"
	"fmt"
	"log"
	"strings"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
	"github.com/koki-develop/go-fzf"
)

type Refile struct {
	From *common.Target
	To   *common.Target
}

/*
type Target struct {
	Filename string
	Id       string
	// File+ types use file and id fields except for line
	// id, customid and hash all just use the id field
	Type string // file+headline, id, customid, hash, file+line
	Lvl  int    // For heading matches if this is non-zero then this fixes the level we MUST match at
}
*/

func (self *Refile) Unmarshal(unmarshal func(interface{}) error) error {
	return unmarshal(self)
}

func (self *Refile) SetupParameters(fset *flag.FlagSet) {
	//fmt.Printf("REFILE CALLED\n")
	// Need custom parser for this....
	//fset.StringVar(&(self.From), "from", "", "source id")
	//fset.StringVar(&(self.To), "head", "", "destination id")
}

func (self *Refile) Exec(core *commands.Core) {
	fmt.Printf("Refile called\n")
	var qry map[string]string = map[string]string{}
	var files []string
	commands.SendReceiveGet(core, "files", qry, &files)
	if len(files) <= 0 {
		fmt.Printf("No files found to refile")
		return
	}
	var fromFile string
	if self.From == nil {
		f, err := fzf.New(
			fzf.WithNoLimit(true),
			fzf.WithCountViewEnabled(true),
			fzf.WithCountView(func(meta fzf.CountViewMeta) string {
				return fmt.Sprintf("files: %d, selected: %d", meta.ItemsCount, meta.SelectedCount)
			}),
		)
		if err != nil {
			log.Fatal(err)
		}
		var idx []int = []int{}
		idx, err = f.Find(files, func(i int) string { return files[i] })
		if err != nil {
			log.Fatal(err)
		}
		fromFile = files[idx[0]]
		fmt.Printf("FROM FILE: %s\n", fromFile)
	}
	qry["filename"] = fromFile
	var todos common.Todos
	commands.SendReceiveGet(core, "filecontents/headings", qry, &todos)

	if len(todos) > 0 {
		f, err := fzf.New(
			fzf.WithNoLimit(true),
			fzf.WithCountViewEnabled(true),
			fzf.WithCountView(func(meta fzf.CountViewMeta) string {
				return fmt.Sprintf("headings: %d, selected: %d", meta.ItemsCount, meta.SelectedCount)
			}),
		)
		if err != nil {
			log.Fatal(err)
		}
		var idx []int = []int{}
		idx, err = f.Find(todos, func(i int) string {
			pre := ""
			if todos[i].Level > 1 {
				pre = strings.Repeat("  ", todos[i].Level-1)
			}
			return pre + "." + " " + todos[i].Headline
		})
		if err != nil {
			log.Fatal(err)
		}
		fromFile = todos[idx[0]].Hash
		fmt.Printf("NODE HASH: %s\n", fromFile)

	}

}

// init function is called at boot
func init() {
	commands.AddCmd("ref", "refile heading",
		func() commands.Cmd {
			return &Refile{}
		})
}
