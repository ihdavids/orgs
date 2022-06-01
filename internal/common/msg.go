package common

import (
	"bytes"
	"encoding/json"

	"gopkg.in/yaml.v2"
)

type HelloArgs struct {
	Msg string
}

type Empty struct{}

type HelloReply string

type FileList []string

type Todo struct {
	Headline string
	Tags     []string
	Hash     string
}

type FullTodo struct {
	Headline string
	Content  string
	Tags     []string
	Hash     string
	Priority string
}

type TodoHash string

type Todos []Todo

type NodeType int16

const (
	And NodeType = iota
	Or
	Not
	Tag
	Status
	Priority
	HeadlineRe
	IsProject
)

func (s NodeType) String() string {
	return toString[s]
}

var toString = map[NodeType]string{
	And:        "and",
	Or:         "or",
	Not:        "not",
	Tag:        "tag",
	Status:     "status",
	Priority:   "priority",
	HeadlineRe: "headlinere",
	IsProject:  "isproject",
}

var toID = map[string]NodeType{
	"and":        And,
	"or":         Or,
	"not":        Not,
	"tag":        Tag,
	"status":     Status,
	"priority":   Priority,
	"headlinere": HeadlineRe,
	"isproject":  IsProject,
}

// MarshalJSON marshals the enum as a quoted json string
func (s NodeType) MarshalJSON() ([]byte, error) {
	buffer := bytes.NewBufferString(`"`)
	buffer.WriteString(toString[s])
	buffer.WriteString(`"`)
	return buffer.Bytes(), nil
}

// UnmarshalJSON unmashals a quoted json string to the enum value
func (s *NodeType) UnmarshalJSON(b []byte) error {
	var j string
	err := json.Unmarshal(b, &j)
	if err != nil {
		return err
	}
	// Note that if the string cannot be found then it will be set to the zero value, 'Created' in this case.
	*s = toID[j]
	return nil
}

// MarshalYAML marshals the enum as a quoted yaml string
func (s NodeType) MarshalYAML() ([]byte, error) {
	buffer := bytes.NewBufferString(`"`)
	buffer.WriteString(toString[s])
	buffer.WriteString(`"`)
	return buffer.Bytes(), nil
}

// UnmarshalYAML unmashals a quoted yaml string to the enum value
func (s *NodeType) UnmarshalYAML(b []byte) error {
	var j string
	err := yaml.Unmarshal(b, &j)
	if err != nil {
		return err
	}
	// Note that if the string cannot be found then it will be set to the zero value, 'Created' in this case.
	*s = toID[j]
	return nil
}

type ValueNode struct {
	String string `yaml:"str"`
	Int    int    `yaml:"int"`
	Bool   bool   `yaml:"bool"`
}
type Query struct {
	NodeType NodeType  `yaml:"type"`
	Value    ValueNode `yaml:"val"`
	Children []Query   `yaml:"dep"`
}

type StringQuery struct {
	Query string `yaml:"query"`
}
