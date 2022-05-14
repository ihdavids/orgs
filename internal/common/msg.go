package common

type HelloArgs struct {
	Msg string
}

type Empty struct{}

type HelloReply string

type FileList []string

type Todo struct {
	Headline string
	Tags     []string
}

type Todos []Todo

type Query struct {
	HeadlineRe string
	Status     []string
	Tags       []string
	Priorities []string
	IsProject  bool
}
