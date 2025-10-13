package grep

// Files lets you walk through your ORG mode repo
// using an fzf and bat like interface.
//
// Selecting a file opens it in your chosen editor

import (
	"flag"
	"fmt"
	"strconv"
	"strings"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
	fzf "github.com/junegunn/fzf/src"
)

type GrepQuery struct {
	Query string
}

func (self *GrepQuery) Unmarshal(unmarshal func(interface{}) error) error {
	return unmarshal(self)
}

func (self *GrepQuery) StartPlugin(manager *common.PluginManager) {
}

func (self *GrepQuery) SetupParameters(fset *flag.FlagSet) {
	fset.StringVar(&(self.Query), "query", "", "Query")
}

func (self *GrepQuery) Exec(core *commands.Core) {
	fmt.Printf("Grep called\n")

	var qry map[string]string = map[string]string{}
	//qry["filename"] = "./out.html"
	//qry["query"] = "IsTask() && HasProperty(\"EFFORT\")"
	qry["query"] = self.Query
	var reply common.FileList = common.FileList{}

	//func SendReceiveGet[RPC any, RESP any](core *Core, name string, args *RPC, resp *RESP) {
	commands.SendReceiveGet(core, "grep", qry, &reply)
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
		options, _ := fzf.ParseOptions(
			true, // whether to load defaults ($FZF_DEFAULT_OPTS_FILE and $FZF_DEFAULT_OPTS)
			// I need to make fzf editor something that is configured!

			//rg --with-filename --vimgrep --context 0 --sort path FilesQuery | fzf --border --reverse --delimiter ':' --with-nth '3..' --preview "bat --style=numbers --color=always --highlight-line {2} {1} -n -H {2}" --preview-window 'border-bottom,+{2}+3/3'
			//[]string{"--multi", "--reverse", "--border", "--height=40%", "--preview", "bat --style=numbers --color=always --line-range :500 {}"},
			[]string{"--border", "--reverse", "--delimiter", ":", "--with-nth", "3..", "--preview", "bat --style=numbers --color=always --highlight-line {2} {1} -n -H {2}", "--preview-window", "border-bottom,+{2}+3/3"},
		)
		/*
			if err != nil {
				exit(fzf.ExitError, err)
			}
		*/

		// Set up input and output channels
		options.Input = inputChan
		options.Output = outputChan

		// Run fzf
		fzf.Run(options)

		//fmt.Printf("OUTPUT: %v:%v\n", code, reply[code])
		fmt.Printf("%v\n", output)
		for _, o := range output {
			os := strings.Split(o, ":")
			line, _ := strconv.Atoi(os[1])
			core.LaunchEditor(os[0], line)
		}
	} else {
		fmt.Printf("Err")
	}
}

// init function is called at boot
func init() {
	commands.AddCmd("grep", "query information about files in DB",
		func() commands.Cmd {
			return &GrepQuery{}
		})
}
