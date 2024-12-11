//lint:file-ignore ST1006 allow the use of self
package capture

import (
	"flag"
	"fmt"

	"github.com/ihdavids/orgs/cmd/oc/commands"
)

type Refile struct {
	From string
	To   string
}

func (self *Refile) Unmarshal(unmarshal func(interface{}) error) error {
	return unmarshal(self)
}

func (self *Refile) SetupParameters(fset *flag.FlagSet) {
	fmt.Printf("REFILE CALLED\n")
	fset.StringVar(&(self.From), "from", "", "source id")
	fset.StringVar(&(self.To), "head", "", "destination id")
}

func (self *Refile) Exec(core *commands.Core) {
	fmt.Printf("Refile called\n")
}

// init function is called at boot
func init() {
	commands.AddCmd("ref", "refile heading",
		func() commands.Cmd {
			return &Refile{}
		})
}
