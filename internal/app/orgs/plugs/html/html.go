// EXPORTER: HTML Export

package gantt

import (
	"bytes"
	"fmt"
	"html/template"
	"log"
	"os"
	"path/filepath"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/app/orgs/plugs"
)

var docStart = `
<!DOCTYPE html>
<html>
<head>
  <link rel="stylesheet" href="https://fonts.googleapis.com/css2?family={{.fontfamily}}">  
<style>
{{.stylesheet | css}}
</style>
</head>
<body>
`
var docEnd = `
</body>
</html>
`

var funcMap template.FuncMap = template.FuncMap{
	"attr": func(s string) template.HTMLAttr {
		return template.HTMLAttr(s)
	},
	"safe": func(s string) template.HTML {
		return template.HTML(s)
	},
	"css": func(s string) template.CSS {
		return template.CSS(s)
	},
	"jsstr": func(s string) template.JSStr {
		return template.JSStr(s)
	},
	"js": func(s string) template.JS {
		return template.JS(s)
	},
	"url": func(s string) template.URL {
		return template.URL(s)
	},
}

type HtmlExp struct {
	Props map[string]interface{}
}

func (self *HtmlExp) Unmarshal(unmarshal func(interface{}) error) error {
	return unmarshal(self)
}

func fileNameWithoutExt(fileName string) string {
	return fileName[:len(fileName)-len(filepath.Ext(fileName))]
}

func agendaFilenameTag(fileName string) string {
	return fileNameWithoutExt(filepath.Base(fileName))
}

func (self *HtmlExp) Export(db plugs.ODb, query string, to string, opts string) error {
	fmt.Printf("HTML: Export called", query, to, opts)
	_, err := db.QueryTodosExpr(query)
	if err != nil {
		msg := fmt.Sprintf("ERROR: html failed to query expression, %v [%s]\n", err, query)
		log.Printf(msg)
		return fmt.Errorf(msg)
	}
	return nil
}

func ExpandTemplateIntoBuf(o *bytes.Buffer, temp string, m map[string]interface{}) {
	t := template.Must(template.New("").Funcs(funcMap).Parse(temp))
	err := t.Execute(o, m)
	if err != nil {
		fmt.Printf("TEMPLATE ERROR: %s\n", err.Error())
	}
}

func (self *HtmlExp) ExportToString(db plugs.ODb, query string, opts string) (error, string) {
	self.Props = ValidateMap(self.Props)
	fmt.Printf("HTML: Export string called [%s]:[%s]\n", query, opts)
	/*
		_, err := db.QueryTodosExpr(query)
		if err != nil {
			msg := fmt.Sprintf("ERROR: html failed to query expression, %v [%s]\n", err, query)
			log.Printf(msg)
			return fmt.Errorf(msg), ""
		}
	*/

	if f := db.FindByFile(query); f != nil {
		w := org.NewHTMLWriter()
		org.WriteNodes(w, f.Nodes...)
		res := w.String()
		//fmt.Printf("X: [%s]", res)

		o := bytes.NewBufferString("")
		//fmt.Printf("DOC START: %s\n", docStart)
		fmt.Printf("DOC START: ========================================\n")
		ExpandTemplateIntoBuf(o, docStart, self.Props)
		res = o.String() + res + docEnd
		//fmt.Printf("XXX: %s\n", res)
		return nil, res
	} else {
		fmt.Printf("Failed to find file in database: [%s]", query)
		return fmt.Errorf("Failed to find file in database: [%s]", query), ""
	}
	return nil, ""
}

func (self *HtmlExp) Startup(manager *plugs.PluginManager, opts *plugs.PluginOpts) {
}

func NewHtmlExp() *HtmlExp {
	var g *HtmlExp = new(HtmlExp)
	return g
}

func ValidateMap(m map[string]interface{}) map[string]interface{} {
	force_reload_style := false
	if _, ok := m["title"]; !ok {
		m["title"] = "Schedule"
	}
	if _, ok := m["fontfamily"]; !ok {
		m["fontfamily"] = "Inconsolata"
	}
	if _, ok := m["trackheight"]; !ok {
		m["trackheight"] = 30
	}
	if _, ok := m["stylesheet"]; !ok || force_reload_style {
		if data, err := os.ReadFile(plugs.PlugExpandTemplatePath("html_style.css")); err == nil {
			m["stylesheet"] = (string)(data)
		}
	}

	return m
}

// init function is called at boot
func init() {
	plugs.AddExporter("html", func() plugs.Exporter {
		return &HtmlExp{Props: ValidateMap(map[string]interface{}{})}
	})
}
