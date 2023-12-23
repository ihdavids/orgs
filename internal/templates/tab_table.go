package templates

import (
	"bytes"
	"strings"

	"github.com/flosch/pongo2/v5"
	"github.com/ihdavids/tablewriter"
)

// == [TABLE] ===================================================

type tagTableNode struct {
	wrapper *pongo2.NodeWrapper
}

type tagTableInfo struct {
	Headers []string
	Cells   [][]string
}

func (node *tagTableNode) Execute(ctx *pongo2.ExecutionContext, writer pongo2.TemplateWriter) *pongo2.Error {
	ifo := &tagTableInfo{}
	ctx.Private["table"] = ifo
	node.wrapper.Execute(ctx, writer)
	if ifo != nil {
		table := tablewriter.NewWriter(writer)
		table.SetBorders(tablewriter.Border{Top: false, Left: true, Right: true, Bottom: false, CenterMarkers: false})
		table.SetAutoFormatHeaders(false)
		if len(ifo.Headers) > 0 || len(ifo.Cells) > 0 {
			table.SetHeader(ifo.Headers)
			table.AppendBulk(ifo.Cells)
			table.Render()
		}
	}
	return nil
}

func tagTableParser(doc *pongo2.Parser, start *pongo2.Token, arguments *pongo2.Parser) (pongo2.INodeTag, *pongo2.Error) {
	opNode := &tagTableNode{}

	wrapper, tagArgs, err := doc.WrapUntilTag("endtable")
	if err != nil {
		return nil, err
	}
	opNode.wrapper = wrapper
	if tagArgs.Count() > 0 {
		// table can't take any conditions
		return nil, tagArgs.Error("Table arguments not allowed here.", nil)
	}
	return opNode, nil
}

// == [ROW] ===================================================

type tagRowNode struct {
	wrapper *pongo2.NodeWrapper
}

func (node *tagRowNode) Execute(ctx *pongo2.ExecutionContext, writer pongo2.TemplateWriter) *pongo2.Error {
	ifo := ctx.Private["table"].(*tagTableInfo)
	ifo.Cells = append(ifo.Cells, []string{})
	node.wrapper.Execute(ctx, writer)
	return nil
}

func tagRowParser(doc *pongo2.Parser, start *pongo2.Token, arguments *pongo2.Parser) (pongo2.INodeTag, *pongo2.Error) {
	opNode := &tagRowNode{}
	wrapper, tagArgs, err := doc.WrapUntilTag("endrow")
	if err != nil {
		return nil, err
	}
	opNode.wrapper = wrapper
	if tagArgs.Count() > 0 {
		// table can't take any conditions
		return nil, tagArgs.Error("Row arguments not allowed here.", nil)
	}
	return opNode, nil
}

// == [CELL] ===================================================

type tagCellNode struct {
	wrapper *pongo2.NodeWrapper
}

func (node *tagCellNode) Execute(ctx *pongo2.ExecutionContext, writer pongo2.TemplateWriter) *pongo2.Error {
	ifo := ctx.Private["table"].(*tagTableInfo)
	if len(ifo.Cells) == 0 {
		return ctx.Error("cell template function called before row template function", nil)
	}

	temp := bytes.NewBuffer(make([]byte, 0, 1024)) // 1 KiB size
	err := node.wrapper.Execute(ctx, temp)
	if err != nil {
		return err
	}
	value := strings.TrimSpace(temp.String())
	ifo.Cells[len(ifo.Cells)-1] = append(ifo.Cells[len(ifo.Cells)-1], value)
	return nil
}

func tagCellParser(doc *pongo2.Parser, start *pongo2.Token, arguments *pongo2.Parser) (pongo2.INodeTag, *pongo2.Error) {
	opNode := &tagCellNode{}
	wrapper, tagArgs, err := doc.WrapUntilTag("endc")
	if err != nil {
		return nil, err
	}
	opNode.wrapper = wrapper
	if tagArgs.Count() > 0 {
		// table can't take any conditions
		return nil, tagArgs.Error("Cell arguments not allowed here.", nil)
	}
	return opNode, nil
}

// == [HEADERS] ===================================================

type tagHeadersNode struct {
	wrapper *pongo2.NodeWrapper
}

func (node *tagHeadersNode) Execute(ctx *pongo2.ExecutionContext, writer pongo2.TemplateWriter) *pongo2.Error {
	ifo := ctx.Private["table"].(*tagTableInfo)
	ifo.Headers = []string{}
	node.wrapper.Execute(ctx, writer)
	return nil
}

func tagHeadersParser(doc *pongo2.Parser, start *pongo2.Token, arguments *pongo2.Parser) (pongo2.INodeTag, *pongo2.Error) {
	opNode := &tagHeadersNode{}
	wrapper, tagArgs, err := doc.WrapUntilTag("endheaders")
	if err != nil {
		return nil, err
	}
	opNode.wrapper = wrapper
	if tagArgs.Count() > 0 {
		// table can't take any conditions
		return nil, tagArgs.Error("Headers arguments not allowed here.", nil)
	}
	return opNode, nil
}

// == [HEADER] ===================================================

type tagHeaderNode struct {
	wrapper *pongo2.NodeWrapper
}

func (node *tagHeaderNode) Execute(ctx *pongo2.ExecutionContext, writer pongo2.TemplateWriter) *pongo2.Error {
	ifo := ctx.Private["table"].(*tagTableInfo)
	if ifo.Headers == nil {
		return ctx.Error("header template function called before headers template function", nil)
	}
	temp := bytes.NewBuffer(make([]byte, 0, 1024)) // 1 KiB size
	err := node.wrapper.Execute(ctx, temp)
	if err != nil {
		return err
	}
	value := strings.TrimSpace(temp.String())
	ifo.Headers = append(ifo.Headers, value)
	return nil
}

func tagHeaderParser(doc *pongo2.Parser, start *pongo2.Token, arguments *pongo2.Parser) (pongo2.INodeTag, *pongo2.Error) {
	opNode := &tagHeaderNode{}
	wrapper, tagArgs, err := doc.WrapUntilTag("endh")
	if err != nil {
		return nil, err
	}
	opNode.wrapper = wrapper
	if tagArgs.Count() > 0 {
		// table can't take any conditions
		return nil, tagArgs.Error("Header arguments not allowed here.", nil)
	}
	return opNode, nil
}

func init() {
	pongo2.RegisterTag("table", tagTableParser)
	pongo2.RegisterTag("row", tagRowParser)
	pongo2.RegisterTag("c", tagCellParser)
	pongo2.RegisterTag("headers", tagHeadersParser)
	pongo2.RegisterTag("h", tagHeaderParser)
}
