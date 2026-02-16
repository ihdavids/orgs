package files

// Files lets you walk through your ORG mode repo
// using an fzf and bat like interface.
//
// Selecting a file opens it in your chosen editor

/*
import (
	"flag"
	"fmt"
	"log"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"

	"github.com/koki-develop/go-fzf"
)

type FilesQuery struct {
}

func (self *FilesQuery) Unmarshal(unmarshal func(interface{}) error) error {
	return unmarshal(self)
}

func (self *FilesQuery) SetupParameters(*flag.FlagSet) {
}

func (self *FilesQuery) Exec(core *commands.Core) {
	fmt.Printf("FilesQuery called\n")

	var qry map[string]string = map[string]string{}
	//qry["filename"] = "./out.html"
	//qry["query"] = "IsTask() && HasProperty(\"EFFORT\")"
	var reply common.FileList = common.FileList{}

	//func SendReceiveGet[RPC any, RESP any](core *Core, name string, args *RPC, resp *RESP) {
	commands.SendReceiveGet(core, "files", qry, &reply)
	//commands.SendReceiveRpc(core, "Db.ExportToFile", &query, &reply)
	if reply != nil {
		//fmt.Printf("OK")
		f, err := fzf.New(
			fzf.WithNoLimit(true),
			fzf.WithCountViewEnabled(true),
			fzf.WithCountView(func(meta fzf.CountViewMeta) string {
				return fmt.Sprintf("items: %d, selected: %d", meta.ItemsCount, meta.SelectedCount)
			}),
		)
		if err != nil {
			log.Fatal(err)
		}
		idxs, err := f.Find(reply, func(i int) string { return reply[i] })
		if err != nil {
			log.Fatal(err)
		}

		for _, i := range idxs {
			core.LaunchEditor(reply[i], 0)
			//fmt.Println(reply[i])
		}
	} else {
		fmt.Printf("Err")
	}
}

// init function is called at boot
func init() {
	commands.AddCmd("files", "query information about files in DB",
		func() commands.Cmd {
			return &FilesQuery{}
		})
}
*/

/*
func ShowFileList(c *rpc.Client) {
	var reply common.FileList
	err := c.Call("Db.GetFileList", nil, &reply)
	if err != nil {
		log.Printf("%v", err)
	} else {
		log.Printf("%v", reply)
	}
}
*/

import (
	"flag"
	"fmt"
	"runtime"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
	fzf "github.com/junegunn/fzf/src"
)

type FilesQuery struct {
}

func (self *FilesQuery) Unmarshal(unmarshal func(interface{}) error) error {
	return unmarshal(self)
}

func (self *FilesQuery) StartPlugin(manager *common.PluginManager) {
}

func (self *FilesQuery) SetupParameters(*flag.FlagSet) {
}

func (self *FilesQuery) Exec(core *commands.Core) {
	fmt.Printf("FilesQuery called\n")

	var qry map[string]string = map[string]string{}
	//qry["filename"] = "./out.html"
	//qry["query"] = "IsTask() && HasProperty(\"EFFORT\")"
	var reply common.FileList = common.FileList{}

	//func SendReceiveGet[RPC any, RESP any](core *Core, name string, args *RPC, resp *RESP) {
	commands.SendReceiveGet(core, "files", qry, &reply)
	//commands.SendReceiveRpc(core, "Db.ExportToFile", &query, &reply)
	if reply != nil {
		//fmt.Printf("OK")

		inputChan := make(chan string)
		go func() {
			for _, s := range reply {
				inputChan <- s
			}
			close(inputChan)
		}()

		output := []string{}
		outputChan := make(chan string)
		go func() {
			for s := range outputChan {
				//fmt.Println("Got: " + s)
				output = append(output, s)
			}
		}()

		/*
			exit := func(code int, err error) {
				if err != nil {
					fmt.Fprintln(os.Stderr, err.Error())
				}
				os.Exit(code)
			}
		*/
		// Build fzf.Options
		var options *fzf.Options = nil
		switch runtime.GOOS {
		case "windows":
			options, _ = fzf.ParseOptions(
				true, // whether to load defaults ($FZF_DEFAULT_OPTS_FILE and $FZF_DEFAULT_OPTS)
				// I need to make fzf editor something that is configured!
				//rg --with-filename --vimgrep --context 0 --sort path FilesQuery | fzf --border --reverse --delimiter ':' --with-nth '3..' --preview "bat --style=numbers --color=always --highlight-line {2} {1} -n -H {2}" --preview-window 'border-bottom,+{2}+3/3'
				[]string{"--multi", "--reverse", "--border", "--height=40%", "--preview", "bat --style=numbers --color=always --line-range :500 {}"},
			)
		default:
			options, _ = fzf.ParseOptions(
				true, // whether to load defaults ($FZF_DEFAULT_OPTS_FILE and $FZF_DEFAULT_OPTS)
				// I need to make fzf editor something that is configured!
				//rg --with-filename --vimgrep --context 0 --sort path FilesQuery | fzf --border --reverse --delimiter ':' --with-nth '3..' --preview "bat --style=numbers --color=always --highlight-line {2} {1} -n -H {2}" --preview-window 'border-bottom,+{2}+3/3'
				[]string{"--multi", "--reverse", "--border", "--height=40%", "--preview", "bat --style=numbers --color=always --line-range :500 {}"},
			)
		}
		/*
			if err != nil {
				exit(fzf.ExitError, err)
			}
		*/

		// Set up input and output channels
		options.Input = inputChan
		options.Output = outputChan

		// Run fzf
		code, _ := fzf.Run(options)

		fmt.Printf("OUTPUT: %v\n", code)
		for _, o := range output {
			core.LaunchEditor(o, 0)
			//fmt.Println(reply[i])
		}
	} else {
		fmt.Printf("Err")
	}
}

// init function is called at boot
func init() {
	commands.AddCmd("files", "query information about files in DB",
		func() commands.Cmd {
			return &FilesQuery{}
		})
}
