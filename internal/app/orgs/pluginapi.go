//lint:file-ignore ST1006 allow the use of self
package orgs

import (
	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

// The Plugin API are methods added to allow plugins to query data
// without the bulk of the query API. These methods are easier to use
// from within a library and may not serialize over the standard
// network. Eventually they will not be accessible from the network
// a all!

func (s *Db) FindPrevSibling(hash string) *common.Todo {
	var query common.TodoHash = (common.TodoHash)(hash)
	var reply *common.Todo = new(common.Todo)
	err := db.QueryPrevSibling(&query, reply)
	if err != nil {
		return nil
	}
	return reply
}

func (s *Db) FindByHash(hash string) *common.Todo {
	var query common.TodoHash = (common.TodoHash)(hash)
	var reply *common.Todo = new(common.Todo)
	err := db.QueryByHash(&query, reply)
	if err != nil {
		return nil
	}
	return reply
}

func (s *Db) FindByAnyId(hash string) *common.Todo {
	var query common.TodoHash = (common.TodoHash)(hash)
	var reply *common.Todo = new(common.Todo)
	err := db.QueryByAnyId(&query, reply)
	if err != nil {
		return nil
	}
	return reply
}

func (s *Db) FindNextSibling(hash string) *common.Todo {
	var query common.TodoHash = (common.TodoHash)(hash)
	var reply *common.Todo = new(common.Todo)
	err := db.QueryNextSibling(&query, reply)
	if err != nil {
		return nil
	}
	return reply
}

func (s *Db) FindLastChild(hash string) *common.Todo {
	var query common.TodoHash = (common.TodoHash)(hash)
	var reply *common.Todo = new(common.Todo)
	err := db.QueryLastChild(&query, reply)
	if err != nil {
		return nil
	}
	return reply
}

func (self *Db) FindByFile(filename string) *org.Document {
	f := GetDb().FindByFile(filename)
	if f != nil {
		return f.Doc
	}
	return nil
}

func (self *Db) GetFile(filename string) *common.OrgFile {
	return GetDb().FindByFile(filename)
}

func (self *Db) GetFromTarget(target *common.Target, allowCreate bool) (*common.OrgFile, *org.Section) {
	return GetDb().GetFromTarget(target, allowCreate)
}

func (self *Db) GetFromPreciseTarget(target *common.PreciseTarget, typeId org.NodeType) (*common.OrgFile, *org.Section, org.Node) {
	return GetDb().GetFromPreciseTarget(target, typeId)
}
