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

// OLD API DEPRECATED AND REMOVE
func (s *Db) QueryTodos(args *common.Query, reply *common.Todos) error {
	*reply = orgs.QueryTodos(args)
	return nil
}

func (s *Db) QueryTodosExp(args *common.StringQuery, reply *common.Todos) error {
	var err error = nil
	*reply, err = orgs.QueryStringTodos(args)
	return err
}

func (s *Db) QueryFullTodo(args *common.TodoHash, reply *common.FullTodo) error {
	var err error = nil
	*reply, err = orgs.QueryFullTodo(args)
	return err
}

// Change the status TODO,DONE etc in a todo head by hash
func (s *Db) ChangeStatus(args *common.TodoItemChange, reply *common.Result) error {
	var err error = nil
	*reply, err = orgs.ChangeStatus(args)
	return err
}

func (s *Db) ToggleTags(args *common.TodoItemChange, reply *common.Result) error {
	var err error = nil
	*reply, err = orgs.ToggleTag(args)
	return err
}
