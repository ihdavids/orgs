//lint:file-ignore ST1006 allow the use of self
package capture

import (
	"encoding/json"
	"flag"
	"fmt"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

type Capture struct {
	Template string
	Head     string
	Cont     string
}

func (self *Capture) Unmarshal(unmarshal func(interface{}) error) error {
	return unmarshal(self)
}

func (self *Capture) Exec(core *commands.Core, args []string) {
	fmt.Printf("Capture called\n")

	fset := flag.NewFlagSet("capture", flag.ExitOnError)
	fset.StringVar(&self.Template, "temp", "", "template name")
	fset.StringVar(&self.Head, "head", "", "heading")
	fset.StringVar(&self.Cont, "cont", "", "content")
	fset.Parse(args)

	var query common.Capture
	query.Template = self.Template
	query.NewNode.Headline = self.Head
	query.NewNode.Content = self.Cont
	fmt.Printf("CAP: %s\n\t%s\n\t%s\n", query.Template, query.NewNode.Headline, query.NewNode.Content)
	var reply common.ResultMsg = common.ResultMsg{}
	commands.SendReceivePost(core, "capture", &query, &reply)
	//commands.SendReceiveRpc(core, "Db.Capture", &query, &reply)
	if reply.Ok {
		fmt.Printf("OK: %s\n", reply.Msg)
	} else {
		fmt.Printf("Err: %s\n", reply.Msg)
	}
}

type CaptureTemplate struct {
}

func (self *CaptureTemplate) Unmarshal(unmarshal func(interface{}) error) error {
	return unmarshal(self)
}

func (self *CaptureTemplate) Exec(core *commands.Core, args []string) {
	fmt.Printf("Capture templates\n")

	var qry map[string]string = map[string]string{}
	//var reply common.Result = common.Result{}
	var reply *[]common.CaptureTemplate = &[]common.CaptureTemplate{}
	commands.SendReceiveGet(core, "capture/templates", qry, &reply)
	//commands.SendReceiveRpc(core, "Db.QueryCaptureTemplates", &query, &reply)
	for _, x := range *reply {
		b, err := json.MarshalIndent(x, "", "  ")
		if err != nil {
			fmt.Println(err)
		}
		fmt.Print(string(b))
		//fmt.Printf("%v", x)
	}
}

// init function is called at boot
func init() {
	commands.AddCmd("cap", "quick capture idea", func() commands.Cmd {
		return &Capture{}
	})
	commands.AddCmd("listcap", "list capture templates", func() commands.Cmd {
		return &CaptureTemplate{}
	})
}
