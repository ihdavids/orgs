//lint:file-ignore ST1006 allow the use of self
package main

import (
	"fmt"

	"github.com/ihdavids/orgs/internal/app/orgs"
	"github.com/ihdavids/orgs/internal/common"
)

// OLD NUKE THIS!

type Comm struct{}

type Db struct{}

func (s *Db) GetFileList(args *common.Empty, reply *common.FileList) error {
	*reply = orgs.GetDb().GetFiles()
	return nil
}

func (s *Db) QueryTodosExp(args *common.StringQuery, reply *common.Todos) error {
	tmp, err := orgs.QueryStringTodos(args)
	if reply != nil {
		*reply = *tmp
	}
	return err
}

func (s *Db) QueryTodosExpr(args string) (common.Todos, error) {
	var query common.StringQuery
	query.Query = args
	var reply common.Todos = nil
	err := db.QueryTodosExp(&query, &reply)
	return reply, err
}

func (s *Db) QueryFullTodo(args *common.TodoHash, reply *common.FullTodo) error {
	var err error = nil
	*reply, err = orgs.QueryFullTodo(args)
	return err
}

func (s *Db) QueryByHash(args *common.TodoHash, reply *common.Todo) error {
	var err error = nil
	res := orgs.FindByHash(args)
	if res == nil {
		err = fmt.Errorf("could not find hash %s", args)
	} else {
		*reply = *res
	}
	return err
}

func (s *Db) QueryByAnyId(args *common.TodoHash, reply *common.Todo) error {
	var err error = nil
	res := orgs.FindByAnyId(args)

	if res == nil {
		err = fmt.Errorf("could not find by any id %s", args)
	} else {
		*reply = *res
	}
	return err
}

func (s *Db) QueryNextSibling(args *common.TodoHash, reply *common.Todo) error {
	var err error = nil
	res := orgs.NextSibling(args)
	if res == nil {
		err = fmt.Errorf("could not find next sibling %s", args)
	} else {
		*reply = *res
	}
	return err
}

func (s *Db) QueryPrevSibling(args *common.TodoHash, reply *common.Todo) error {
	var err error = nil
	res := orgs.PrevSibling(args)
	if res == nil {
		err = fmt.Errorf("could not find prev sibling %s", args)
	} else {
		*reply = *res
	}
	return err
}

func (s *Db) QueryLastChild(args *common.TodoHash, reply *common.Todo) error {
	var err error = nil
	res := orgs.LastChild(args)
	if res == nil {
		err = fmt.Errorf("could not find last child %s", args)
	} else {
		*reply = *res
	}
	return err
}

func (s *Db) QueryFullTodoHtml(args *common.TodoHash, reply *common.FullTodo) error {
	var err error = nil
	*reply, err = orgs.QueryFullTodoHtml(args)
	return err
}

func (s *Db) QueryValidStatus(args *common.TodoHash, reply *common.TodoStatesResult) error {
	var err error = nil
	*reply, err = orgs.ValidStatus(args)
	return err
}

func (s *Db) QueryFullFileHtml(args *common.TodoHash, reply *common.FullTodo) error {
	var err error = nil
	*reply, err = orgs.QueryFullFileHtml(args)
	return err
}

func (s *Db) GetDayPageAt(args *common.Date, reply *common.FileList) error {
	var err error = nil
	*reply, err = orgs.GetDayPageAt(args)
	return err
}

// / Run a setup export plugin if available
func (s *Db) ExportToFile(args *common.ExportToFile, reply *common.ResultMsg) error {
	var err error = nil
	*reply, err = orgs.ExportToFile(s, args)
	return err
}

func (s *Db) ExportToString(args *common.ExportToFile, reply *common.ResultMsg) error {
	var err error = nil
	*reply, err = orgs.ExportToString(s, args)
	return err
}

func (s *Db) QueryCaptureTemplates(args *string, reply *[]common.CaptureTemplate) error {
	var err error = nil
	*reply, err = orgs.QueryCaptureTemplates()
	if err != nil {
		fmt.Printf("QueryCaptureTemplates: %s", err.Error())
	}
	return err
}
