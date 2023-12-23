package main

import (
	"fmt"
	"github.com/ihdavids/orgs/internal/app/orgs"
	"github.com/ihdavids/orgs/internal/common"
)

// These are APIs that can change your actual org files
// We may want to lock these APIs to localhost, ip locked
// extra permissions, or even just lock them out.

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

func (s *Db) CreateDayPage(args *common.TodoHash, reply *common.FileList) error {
	var err error = nil
	*reply, err = orgs.CreateDayPage()
	return err
}

func (s *Db) Capture(args *common.Capture, reply *common.ResultMsg) error {
	var err error = nil
	*reply, err = orgs.Capture(s, args)
	if err != nil {
		fmt.Printf("Capture: %s", err.Error())
		fmt.Println(*args)
	}
	return err
}
