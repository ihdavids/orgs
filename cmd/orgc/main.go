package main

import (
	"log"
	"path"
	"path/filepath"

	//"net/rpc"
	//"io"
	"os"

	//"net/rpc/jsonrpc"

	"github.com/gorilla/websocket"
	"github.com/ihdavids/orgs/internal/app/orgc"
	"github.com/ihdavids/orgs/internal/common"
)

func logToFile() *os.File {

	ex, err := os.Executable()
	if err != nil {
		panic(err)
	}

	exPath := filepath.Dir(ex)
	logFile := path.Join(exPath, "orgc.log")

	f, err := os.OpenFile(logFile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
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
	orgc.Conf()
	//ws, res, err := dialer.Dial(orgc.Conf().Url, http.Header{})
	//if err != nil {
	//	log.Fatal("ListenAndServe: ", err)
	//}
	//defer ws.Close()
	//log.Println(res)

	handle()
}

var dialer = websocket.Dialer{
	ReadBufferSize:  common.MaxMessageSize,
	WriteBufferSize: common.MaxMessageSize,
}

func handle() {
	//rwc := &common.ReadWriteCloser{WS: ws}
	//codec := jsonrpc.NewClientCodec(rwc)
	//c := rpc.NewClientWithCodec(codec)

	core := orgc.NewCore(orgc.Conf().Url)
	core.Start()

	orgc.Conf().Dispatch(core, nil)
	/*
		args := &common.HelloArgs{Msg: "Hello, World"}
		var reply common.HelloReply
		err := c.Call("Comm.Hello", args, &reply)
		if err != nil {
			log.Printf("%v", err)
			break
		}
		log.Printf("%v", reply)
	*/
	/*
			var reply common.FileList
			err := c.Call("Db.GetFileList", nil, &reply)
			if err != nil {
				log.Printf("%v", err)
				break
			} else {
				log.Printf("%v", reply)
				break
			}
		}
	*/
}
