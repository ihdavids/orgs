package main

import (
	"log"
	"time"

	"github.com/ihdavids/orgs/internal/app/orgs"
	"github.com/ihdavids/orgs/internal/common"
)

type Comm struct{}

func (c *Comm) Hello(args *common.HelloArgs, reply *common.HelloReply) error {
	*reply = "Hello!"
	log.Println(args, *reply)
	time.Sleep(1 * time.Second)
	return nil
}

type Db struct{}

func (s *Db) GetFileList(args *common.Empty, reply *common.FileList) error {
	*reply = orgs.GetDb().GetFiles()
	return nil
}

func (s *Db) QueryTodos(args *common.Query, reply *common.Todos) error {
	*reply = orgs.QueryTodos(args)
	return nil
}
