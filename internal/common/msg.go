package common

import (
	"time"

	"github.com/ihdavids/go-org/org"
)

type HelloArgs struct {
	Msg string
}

type Empty struct{}

type HelloReply string

type FileList []string

type DateTime time.Time

type Todo struct {
	Headline string
	Tags     []string
	Hash     string
	Date     org.OrgDate
	Status   string
	Filename string
	LineNum  int
	IsActive bool
}

type FullTodo struct {
	Headline string
	Content  string
	Tags     []string
	Hash     string
	Priority string
}

type TodoHash string
type TodoItemChange struct {
	Hash  string
	Value string
}

type Todos []Todo

type StringQuery struct {
	Query string `yaml:"query"`
}
type Result struct {
	Ok bool `yaml: "status"`
}

type ListResult struct {
	Vals []string
}

type TodoStatesResult struct {
	Active []string
	Done   []string
}
