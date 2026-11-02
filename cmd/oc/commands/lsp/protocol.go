package lsp

// The part of the Language Server Protocol this speaks, as Go types. Field
// names are the protocol's; omitempty where the spec says optional, because a
// client reads a null where it wanted an absent field as a broken answer.

type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

type Location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}

type TextEdit struct {
	Range   Range  `json:"range"`
	NewText string `json:"newText"`
}

type VersionedTextDocumentIdentifier struct {
	URI     string `json:"uri"`
	Version *int   `json:"version"`
}

type TextDocumentEdit struct {
	TextDocument VersionedTextDocumentIdentifier `json:"textDocument"`
	Edits        []TextEdit                      `json:"edits"`
}

type WorkspaceEdit struct {
	Changes         map[string][]TextEdit `json:"changes,omitempty"`
	DocumentChanges []TextDocumentEdit    `json:"documentChanges,omitempty"`
}

type Command struct {
	Title     string        `json:"title"`
	Command   string        `json:"command"`
	Arguments []interface{} `json:"arguments,omitempty"`
}

type Diagnostic struct {
	Range    Range  `json:"range"`
	Severity int    `json:"severity,omitempty"`
	Code     string `json:"code,omitempty"`
	Source   string `json:"source,omitempty"`
	Message  string `json:"message"`
}

const (
	sevError   = 1
	sevWarning = 2
	sevInfo    = 3
	sevHint    = 4
)

type CodeAction struct {
	Title       string         `json:"title"`
	Kind        string         `json:"kind,omitempty"`
	Diagnostics []Diagnostic   `json:"diagnostics,omitempty"`
	IsPreferred bool           `json:"isPreferred,omitempty"`
	Disabled    *struct {
		Reason string `json:"reason"`
	} `json:"disabled,omitempty"`
	Edit    *WorkspaceEdit `json:"edit,omitempty"`
	Command *Command       `json:"command,omitempty"`
}

type CodeLens struct {
	Range   Range    `json:"range"`
	Command *Command `json:"command,omitempty"`
}

type DocumentSymbol struct {
	Name           string           `json:"name"`
	Detail         string           `json:"detail,omitempty"`
	Kind           int              `json:"kind"`
	Range          Range            `json:"range"`
	SelectionRange Range            `json:"selectionRange"`
	Children       []DocumentSymbol `json:"children,omitempty"`
}

type SymbolInformation struct {
	Name          string   `json:"name"`
	Kind          int      `json:"kind"`
	Location      Location `json:"location"`
	ContainerName string   `json:"containerName,omitempty"`
}

// Symbol kinds used: a heading is a namespace (it holds things), a heading
// with a keyword an event, a named block a function, a named table a struct.
const (
	symNamespace = 3
	symFunction  = 12
	symVariable  = 13
	symStruct    = 23
	symEvent     = 24
)

type FoldingRange struct {
	StartLine int    `json:"startLine"`
	EndLine   int    `json:"endLine"`
	Kind      string `json:"kind,omitempty"`
}

type DocumentLink struct {
	Range  Range       `json:"range"`
	Target string      `json:"target,omitempty"`
	Data   interface{} `json:"data,omitempty"`
}

type CompletionItem struct {
	Label            string    `json:"label"`
	Kind             int       `json:"kind,omitempty"`
	Detail           string    `json:"detail,omitempty"`
	SortText         string    `json:"sortText,omitempty"`
	FilterText       string    `json:"filterText,omitempty"`
	InsertTextFormat int       `json:"insertTextFormat,omitempty"`
	TextEdit         *TextEdit `json:"textEdit,omitempty"`
}

type CompletionList struct {
	IsIncomplete bool             `json:"isIncomplete"`
	Items        []CompletionItem `json:"items"`
}

// Completion item kinds.
const (
	compText     = 1
	compKeyword  = 14
	compSnippet  = 15
	compFile     = 17
	compRefer    = 18
	compProperty = 10
	compEnum     = 13
	compConstant = 21
)

type MessageActionItem struct {
	Title string `json:"title"`
}

type textDocumentId struct {
	URI string `json:"uri"`
}

type positionParams struct {
	TextDocument textDocumentId `json:"textDocument"`
	Position     Position       `json:"position"`
}
