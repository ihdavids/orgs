package main

import (
	"github.com/ihdavids/orgs/internal/app/orgs"
	"github.com/ihdavids/orgs/internal/common"
)

type Comm struct{}

type Db struct{}

func (s *Db) GetFileList(args *common.Empty, reply *common.FileList) error {
	*reply = orgs.GetDb().GetFiles()
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

func (s *Db) QueryValidStatus(args *common.TodoHash, reply *common.TodoStatesResult) error {
	var err error = nil
	*reply, err = orgs.ValidStatus(args)
	return err
}

func (s *Db) CreateDayPage(args *common.TodoHash, reply *common.FileList) error {
	var err error = nil
	*reply, err = orgs.CreateDayPage()
	return err
}

func (s *Db) GetDayPageAt(args *common.Date, reply *common.FileList) error {
	var err error = nil
	*reply, err = orgs.GetDayPageAt(args)
	return err
}
