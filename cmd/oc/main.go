package main

import (
	"flag"
	"fmt"
	"log"

	//"net/rpc"
	//"io"
	"os"

	//"net/rpc/jsonrpc"

	"github.com/gorilla/websocket"
	"github.com/ihdavids/orgs/internal/common"

	"github.com/ihdavids/orgs/cmd/oc/commands"
)

func logToFile() *os.File {
	f, err := os.OpenFile("oc.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("error opening file: %v", err)
	}
	//defer f.Close()
	//wrt := io.MultiWriter(os.Stdout, f)
	//log.SetOutput(wrt)
	log.SetOutput(f)
	log.Println("--- [OrgC] ----------------------------------")
	return f
}

func main() {
	f := logToFile()
	defer f.Close()
	Conf()
	core := commands.NewCore(Conf().Url)
	core.EditorTemplate = Conf().EditorTemplate
	core.Start()

	args := flag.Args()

	// Execute command line options
	for k, _ := range commands.CmdRegistry {
		if len(args) > 0 && k == args[0] {
			v := commands.CmdRegistry[k]
			//fmt.Printf("KEY: %v -> %v\n", k, args[1:])
			mod := Conf().FindCommand(k)
			if mod == nil {
				mod = v.Cmd
				oldArgs := args
				args = []string{}
				if len(oldArgs) > 1 {
					args = oldArgs[1:]
				}
				if v.Flags != nil && nil != v.Flags.Parse(args) {
					fmt.Printf("HERE\n")
					panic(fmt.Sprintf("failed to parse arguments for: %s\n", k))
				}
			}
			mod.Exec(core)
		}
	}

	handle()
}

var dialer = websocket.Dialer{
	ReadBufferSize:  common.MaxMessageSize,
	WriteBufferSize: common.MaxMessageSize,
}

func handle() {
}
