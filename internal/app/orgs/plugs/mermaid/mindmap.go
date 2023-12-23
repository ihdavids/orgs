// EXPORTER: MindMap
// This returns an html document containing a mind map in mermaid js format
// This can be used in a web framework or in VsCode as a means of visualizing
// a set of org nodes.
// https://mermaid.js.org/syntax/mindmap.html

package mermaid

import (
	"bufio"
	"bytes"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/ihdavids/orgs/internal/app/orgs/plugs"
	"github.com/ihdavids/orgs/internal/common"
)

var mindMapDocStart = `
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <link rel="stylesheet" href="https://fonts.googleapis.com/css2?family={{.fontfamily}}">
  {{.userheaderscript}}
</head>
<body>
  <div class="mermaid" style="resize: both;">
  mindmap
`
var mindMapStartMarkers = `
`

var mindMapEndMarkers = `
`

var mindMapDocEnd = `
  </div>
  <script type="module">
     function daysToMilliseconds(days) {
       return days * 24 * 60 * 60 * 1000;
     }
     import mermaid from 'https://cdn.jsdelivr.net/npm/mermaid@10/dist/mermaid.esm.min.mjs';
  </script>
  <script>
    $(document).ready(function () {
       mermaid.initialize({startOnLoad:true, securityLevel: 'loose'});
    });
  </script>
  <script>
  function renderMermaid(){
	  mermaid.init(undefined,document.querySelectorAll(".mermaid"));
  }
  $(function() {
	  $(document).on('previewUpdated', function() {        
		  renderMermaid();
	  });
	  renderMermaid();
  });
  </script>
  {{.userbodyscript}}
</body>
</html>
`

type MermaidMindMap struct {
	Props map[string]interface{}
}

func (self *MermaidMindMap) Unmarshal(unmarshal func(interface{}) error) error {
	return unmarshal(self)
}

func getMindMapName(namesMap map[string]string, td *common.Todo, idx *int) string {
	if name, ok := namesMap[td.Hash]; ok {
		return name
	}
	name := fmt.Sprintf("tsk%d", *idx)
	if *idx == -1 {
		name = "root"
	}
	namesMap[td.Hash] = name
	*idx += 1
	return name
}

func getMindMapStyle(td *common.Todo) (string, string) {
	formatstart, formatend := "[", "]"
	if plugs.HasP(td, "SQUARE") {
		formatstart, formatend = "[", "]"
	}
	if plugs.HasP(td, "ROUND") {
		formatstart, formatend = "(", ")"
	}
	if plugs.HasP(td, "CIRCLE") {
		formatstart, formatend = "((", "))"
	}
	if plugs.HasP(td, "BANG") {
		formatstart, formatend = "))", "(("
	}
	if plugs.HasP(td, "CLOUD") {
		formatstart, formatend = ")", "("
	}
	if plugs.HasP(td, "HEXAGON") {
		formatstart, formatend = "{{", "}}"
	}
	return formatstart, formatend
}

func (self *MermaidMindMap) MindMapExportRes(o *bytes.Buffer, db plugs.ODb, namesMap map[string]string, have map[string]*common.Todo, idx *int, td *common.Todo) {
	name := EscapeQuotes(strings.TrimSpace(td.Headline))
	hash := getMindMapName(namesMap, td, idx)
	indentStart := "      "
	prefix := ""
	if *idx == 1 && hash != "root" {
		prefix = "    root\n"
	} else if *idx == 1 && hash == "root" {
		indentStart = "  "
	}
	indent := fmt.Sprintf("%*s", td.Level*2+6, indentStart)

	formatstart, formatend := getMindMapStyle(td)
	m := map[string]interface{}{"prefix": prefix, "name": name, "idx": idx, "hash": hash, "indent": indent, "formatstart": formatstart, "formatend": formatend}
	plugs.ExpandTemplateIntoBuf(o, "{{.prefix}}{{.indent}}{{.hash}}{{.formatstart}}\"`{{.name}}`\"{{.formatend}}\n", m)
}

func (self *MermaidMindMap) Export(db plugs.ODb, query string, to string, opts string) error {
	ValidateMap(self.Props)
	fmt.Printf("MindMap: Export called", query, to, opts)
	tds, err := db.QueryTodosExpr(query)
	if err != nil {
		msg := fmt.Sprintf("ERROR: mindmap failed to query expression, %v [%s]\n", err, query)
		log.Printf(msg)
		return fmt.Errorf(msg)
	}
	have := map[string]*common.Todo{}
	for _, td := range tds {
		have[td.Hash] = &td
	}
	var res error = nil
	o := bytes.NewBufferString("")
	fmt.Println(self.Props)
	plugs.ExpandTemplateIntoBuf(o, mindMapDocStart, self.Props)

	minTd, cntAtMin := lookForRootNode(tds)
	var idx = 0
	namesMap := map[string]string{}

	haveSingleRoot := cntAtMin == 1
	if haveSingleRoot {
		temp := -1
		getMindMapName(namesMap, &minTd, &temp)
		idx = 1
	}

	for _, td := range tds {
		self.MindMapExportRes(o, db, namesMap, have, &idx, &td)
	}
	plugs.ExpandTemplateIntoBuf(o, mindMapDocEnd, self.Props)
	fo, err := os.Create(to)
	if err == nil {
		w := bufio.NewWriter(fo)
		_, err2 := w.WriteString(o.String())
		if err2 != nil {
			msg := fmt.Sprintf("Failed to write file[%v]: %v\n", err2, to)
			log.Printf("%s", msg)
			res = fmt.Errorf(msg)
		}
		w.Flush()
		fo.Close()
	} else {
		msg := fmt.Sprintf("Failed to open file[%v]: %v\n", err, to)
		log.Printf("%s", msg)
		log.Printf("DOC:\n\n")
		log.Printf("%v\n", o.String())
		res = fmt.Errorf(msg)
	}
	return res
}

func lookForRootNode(tds common.Todos) (common.Todo, int) {
	var minTd common.Todo
	var minLvl = 99999
	cntAtMin := 0
	for _, td := range tds {
		if td.Level == minLvl {
			cntAtMin += 1
		}
		if td.Level < minLvl {
			cntAtMin = 1
			minLvl = td.Level
			minTd = td
		}
	}
	return minTd, cntAtMin
}

func (self *MermaidMindMap) ExportToString(db plugs.ODb, query string, opts string) (error, string) {
	self.Props = ValidateMap(self.Props)
	fmt.Println("MindMap: Export string called", query, opts)
	tds, err := db.QueryTodosExpr(query)
	if err != nil {
		msg := fmt.Sprintf("ERROR: mipmap failed to query expression, %v [%s]\n", err, query)
		log.Println(msg)
		return fmt.Errorf(msg), ""
	}
	have := map[string]*common.Todo{}
	for _, td := range tds {
		have[td.Hash] = &td
	}
	var res error = nil
	o := bytes.NewBufferString("")
	//fmt.Println("++++++++++++++++++++++++++++++++++++++++++++++++++++++")
	//fmt.Println(self.Props)
	plugs.ExpandTemplateIntoBuf(o, mindMapDocStart, self.Props)

	minTd, cntAtMin := lookForRootNode(tds)

	namesMap := map[string]string{}
	idx := 0
	haveSingleRoot := cntAtMin == 1
	if haveSingleRoot {
		temp := -1
		getMindMapName(namesMap, &minTd, &temp)
		idx = 1
	}
	for _, td := range tds {
		self.MindMapExportRes(o, db, namesMap, have, &idx, &td)
	}
	plugs.ExpandTemplateIntoBuf(o, mindMapStartMarkers, self.Props)
	plugs.ExpandTemplateIntoBuf(o, mindMapEndMarkers, self.Props)
	plugs.ExpandTemplateIntoBuf(o, mindMapDocEnd, self.Props)
	txt := o.String()
	fmt.Printf("%s\n", txt)
	return res, txt
}

func (self *MermaidMindMap) Startup(manager *plugs.PluginManager, opts *plugs.PluginOpts) {
}
