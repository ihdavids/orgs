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
	From common.Target
	To   common.Target
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

func (self *Refile) StartPlugin(manager *common.PluginManager) {
}

func (self *Refile) SetupParameters(fset *flag.FlagSet) {
	//fmt.Printf("REFILE CALLED\n")
	// Need custom parser for this....
	fset.StringVar(&(self.From.Filename), "from", "", "source file")
	//fset.StringVar(&(self.To), "head", "", "destination id")
}

func GetTarget(core *commands.Core, t *common.Target, files []string) error {
	var qry map[string]string = map[string]string{}
	if t != nil {
		if t.Filename == "" {
			f, err := fzf.New(
				fzf.WithPrompt("Source File: "),
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
			t.Filename = files[idx[0]]
		} else {
			// We have an input filename but it may not be a full path
			// resolve it against the DB just to make sure.
			var res common.ResultMsg = common.ResultMsg{}
			qry["filename"] = t.Filename
			commands.SendReceiveGet(core, "findfile", qry, &res)
			if res.Ok {
				t.Filename = res.Msg
			}
		}
		//fmt.Printf("FILE: %s\n", t.Filename)
		// GOT FILE NOW GET HEADING
		var todos common.Todos
		if t.Filename != "" {
			qry["filename"] = t.Filename
			commands.SendReceiveGet(core, "filecontents/headings", qry, &todos)
		}

		if len(todos) > 0 {
			f, err := fzf.New(
				fzf.WithPrompt("Source Heading: "),
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
			t.Id = todos[idx[0]].Hash
			t.Type = "hash"
			//fmt.Printf("NODE HASH: %s\n", t.Id)

		} else {
			return fmt.Errorf("Failed to find any nodes in file: %s", t.Filename)
		}

	}
	return nil
}

func TargetsMatch(to, from *common.Target) bool {
	if to.Type == from.Type && to.Id == from.Id {
		return true
	}
	return false
}

func (self *Refile) Exec(core *commands.Core) {
	fmt.Printf("Refile called\n")
	var files []string
	var qry map[string]string = map[string]string{}
	commands.SendReceiveGet(core, "files", qry, &files)
	if len(files) <= 0 {
		fmt.Printf("No files found to refile")
		return
	}
	//var toFile string
	GetTarget(core, &self.From, files)
	GetTarget(core, &self.To, files)
	if !TargetsMatch(&self.To, &self.From) {
		ref := common.Refile{FromId: self.From, ToId: self.To}
		//return await this.doPost(url, {FromId: {Filename: src.filename, Id: src.id, Type: src.type }, ToId: {Filename: dest.filename, Id: dest.id, Type: dest.type }});
		//fmt.Printf("Refiling!\n")
		//qry["filename"] = toFile
		var res common.ResultMsg = common.ResultMsg{}
		commands.SendReceivePost(core, "refile", &ref, &res)
		if res.Ok {
			fmt.Printf("Refile complete\n")
		} else {
			fmt.Printf("Refile failed: %v\n", res.Msg)
		}
	} else {
		fmt.Printf("Target and dest cannot be the same! ABORT")
	}

}

// init function is called at boot
func init() {
	commands.AddCmd("ref", "refile heading",
		func() commands.Cmd {
			return &Refile{}
		})
}
