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

func (self *GrepQuery) SetupParameters(fset *flag.FlagSet) {
	fset.StringVar(&(self.Query), "query", "", "Query")
}

func (self *GrepQuery) Exec(core *commands.Core) {
	fmt.Printf("Grep called\n")

	var qry map[string]string = map[string]string{}
	delimeter := "|"
	qry["query"] = self.Query
	qry["delimeter"] = delimeter
	var reply common.FileList = common.FileList{}

	commands.SendReceiveGet(core, "grep", qry, &reply)
	if reply != nil {
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
				output = append(output, s)
			}
		}()

		var options *fzf.Options = nil
		options, _ = fzf.ParseOptions(
			true, // whether to load defaults ($FZF_DEFAULT_OPTS_FILE and $FZF_DEFAULT_OPTS)
			// I need to make fzf editor something that is configured!
			//rg --with-filename --vimgrep --context 0 --sort path FilesQuery | fzf --border --reverse --delimiter ':' --with-nth '3..' --preview "bat --style=numbers --color=always --highlight-line {2} {1} -n -H {2}" --preview-window 'border-bottom,+{2}+3/3'
			[]string{"--border", "--reverse", "--delimiter", delimeter, "--with-nth", "3..", "--preview", "bat --style=numbers --color=always --highlight-line {2} {1} -n -H {2}", "--preview-window", "border-bottom,+{2}+3/3"},
		)

		// Set up input and output channels
		options.Input = inputChan
		options.Output = outputChan

		// Run fzf
		fzf.Run(options)

		fmt.Printf("%v\n", output)
		for _, o := range output {
			os := strings.Split(o, delimeter)
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
