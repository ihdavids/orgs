package main

import (
	"log"
	"net/http"
	"net/rpc"

	"net/rpc/jsonrpc"

	"github.com/gorilla/websocket"
	"github.com/ihdavids/orgs/internal/app/orgc"
	"github.com/ihdavids/orgs/internal/common"
)

func main() {
	orgc.Conf()
	ws, res, err := dialer.Dial(orgc.Conf().Url, http.Header{})
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
	defer ws.Close()

	log.Println(res)

	handle(ws)
}

var dialer = websocket.Dialer{
	ReadBufferSize:  common.MaxMessageSize,
	WriteBufferSize: common.MaxMessageSize,
}

func handle(ws *websocket.Conn) {
	defer func() {
		ws.Close()
	}()

	rwc := &common.ReadWriteCloser{WS: ws}
	codec := jsonrpc.NewClientCodec(rwc)
	c := rpc.NewClientWithCodec(codec)

	orgc.Conf().Dispatch(c)
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
