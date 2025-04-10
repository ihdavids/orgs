// EXPORTER: CONFLUENCE Export

/* SDOC: Exporters

* Confluence
  The confluence exporter has the ability to take an entire file
  and export it as a confluence page in the space of your choosing.

	#+BEGIN_SRC yaml
    exporters:
      - name: "confluence"
        url: "<confluenceurl>"
        user: "<your username>"
        token: "<generated token from confluence>"
        space: "<name of space where you would like docs generated>"
	#+END_SRC

	By default your personal space's name is tilde followed by your username
	best to put it in quotes to avoid parsing issues:
	#+BEGIN_SRC yaml
    space: "~myusername"
	#+END_SRC


EDOC */

package confluence

import (
	"fmt"
	//"html"
	//"html/template"
	"log"
	"os"
	//"path/filepath"
	"regexp"
	"strings"
	"net/http"
	"encoding/json"
	"bytes"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/app/orgs/plugs"
	"gopkg.in/op/go-logging.v1"

	"github.com/ihdavids/orgs/internal/app/orgs/plugs/html"
)

type OrgConfluenceExporter struct {
	TemplatePath string
	Props        map[string]interface{}
	User         string
	Token        string
	Space        string
	Url          string	
	out          *logging.Logger
	pm           *plugs.PluginManager
	opts         *plugs.PluginOpts
}



func HeadlineAloneHasTag(name string, h *org.Headline) bool {
	if h != nil {
		for _, t := range h.Tags {
			t = strings.ToLower(strings.TrimSpace(t))
			if t != "" && (t == name) {
				return true
			}
		}
	}
	return false
}

// OVERRIDE: This overrides the core method
func WriteHeadline(w* htmlexp.OrgHtmlWriter, h org.Headline) {
	// CONFLUENCE CRAP!
	haveConf := false
	if _,ok := w.Exp.Props["skipnoconfluence"]; ok {
		haveConf = HeadlineAloneHasTag("confluence", &h)
		if ip,o2 := w.Exp.Props["inconf"]; !haveConf && (!o2 || ip == "f") 	{
			return
		} else {
			if haveConf {
				w.Exp.Props["inconf"] = "t"
			}
		}
	}
	//secProps := ""
	//secProps = GetProp("REVEAL_TRANSITION", "data-transition", h, secProps)
	//w.WriteString(fmt.Sprintf(`<section %s>`, secProps))

	// CONFLUENCE CRAP!
	confIndent := h.Lvl*5
	w.WriteString(fmt.Sprintf("<div style=\"padding-left:%dpx;\">", confIndent))
	w.WriteString(fmt.Sprintf("<h%d>", h.Lvl))

	// This is not good enough, we add a span with the status if requested, but this is
	// Kind of lame
	if w.Exp.Props["showstatus"] == true {
		statColor := ""
		if col,ok := w.Exp.StatusColors[h.Status]; ok {
			statColor = fmt.Sprintf("style=\"color:%s;\"",col)
		}
		w.WriteString(fmt.Sprintf("<span class=\"status\" %s> %s </span> ", statColor, h.Status))
	}
	org.WriteNodes(w, h.Title...)

	w.WriteString(fmt.Sprintf("</h%d>", h.Lvl))
	w.WriteString(fmt.Sprintf("<div style=\"padding-left:%dpx;\">", 3))

	if content := w.WriteNodesAsString(h.Children...); content != "" {
		w.WriteString(content)
	}
	if haveConf {
		w.Exp.Props["inconf"] = "f"
	}

	w.WriteString(fmt.Sprintf("</div>"))
	w.WriteString(fmt.Sprintf("</div>"))
	//w.WriteString("</section>\n")
}

func (self *OrgConfluenceExporter) Unmarshal(unmarshal func(interface{}) error) error {
	return unmarshal(self)
}

func (self *OrgConfluenceExporter) Export(db plugs.ODb, query string, to string, opts string, props map[string]string) error {
	fmt.Printf("CONFLUENCE: Export called", query, to, opts)
	_, err := db.QueryTodosExpr(query)
	if err != nil {
		msg := fmt.Sprintf("ERROR: html failed to query expression, %v [%s]\n", err, query)
		log.Printf(msg)
		return fmt.Errorf(msg)
	}
	return nil
}

/*
	func ExpandTemplateIntoBuf(o *bytes.Buffer, temp string, m map[string]interface{}) {
		t := template.Must(template.New("").Funcs(funcMap).Parse(temp))
		err := t.Execute(o, m)
		if err != nil {
			fmt.Printf("TEMPLATE ERROR: %s\n", err.Error())
		}
	}
*/
func (self *OrgConfluenceExporter) ExportToString(db plugs.ODb, query string, opts string, props map[string]string) (error, string) {

	exp := htmlexp.OrgHtmlExporter{Props: htmlexp.ValidateMap(map[string]interface{}{}), TemplatePath: "html_default.tpl"}
	exp.ExtendedHeadline = WriteHeadline
	// Limit our exports to only things tagged with confluence
	exp.Props["skipnoconfluence"] = "t"
	exp.Startup(self.pm, self.opts)
	err, str := exp.ExportToString(db, query, opts, props)
	if err == nil {
		self.CreateConfluencePage(str, props)
	}
	return nil, ""
}

func (self *OrgConfluenceExporter) CreateConfluencePage(res string, props map[string]string) *http.Response {
	// we will run an HTTP server locally to test the POST request
	url := self.Url + "/rest/api/content" 

	title := "My New Page"
	if t,ok := props["title"]; ok && t != "" {
		title = t
	}
	page_data := map[string]interface{} {
    	"type":  "page",
    	"title": title,
    	"space": map[string]string { "key": self.Space },
    	"body":  map[string]interface{}{ "storage": map[string]string {
            	 "value": res,
            	 "representation": "storage" } } }
  if pid,ok := props["parent"]; ok && pid != "" {
  	page_data["parentId"] = pid
  }
	body,err := json.Marshal(page_data)
	fmt.Printf("body: %s\n", body)
	if err != nil {
		fmt.Printf("ERROR could not marshal json data: ", err)
		return nil
	}
	// create post body
  client := &http.Client{}
  req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	req.Header.Add("Content-Type", "application/json")  
  req.SetBasicAuth(self.User, self.Token)
  resp, err := client.Do(req)
	//resp, err := http.Post(url, "application/json", bytes.NewBuffer(body))
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	fmt.Println("Status:", resp.Status)

	return resp
}

func (self *OrgConfluenceExporter) Startup(manager *plugs.PluginManager, opts *plugs.PluginOpts) {
	self.opts = opts
	self.out = manager.Out
	self.pm = manager
}

func NewConfluenceExp() *OrgConfluenceExporter {
	var g *OrgConfluenceExporter = new(OrgConfluenceExporter)
	return g
}

var hljsver = "11.9.0"
var hljscdn = "https://cdnjs.cloudflare.com/ajax/libs/highlight.js/" + hljsver

func GetStylesheet(name string) string {
	if data, err := os.ReadFile(plugs.PlugExpandTemplatePath("html_styles/" + name + "_style.css")); err == nil {
		// HACK: We probably do not alway want to do this. Need to think of a better way to handle this!
		re := regexp.MustCompile(`url\(([^)]+)\)`)
		return re.ReplaceAllString(string(data), "url(http://localhost:8010/${1})")
	}
	return ""
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
		m["stylesheet"] = GetStylesheet("default")
	}
	if _, ok := m["hljscdn"]; !ok {
		m["hljs_cdn"] = hljscdn
	}
	if _, ok := m["hljsstyle"]; !ok {
		m["hljs_style"] = "monokai"
	}
	if _, ok := m["wordcloud"]; !ok {
		m["wordcloud"] = false
	}
	if _, ok := m["fontfamily"]; !ok {
		m["fontfamily"] = "Inconsolata"
	}
	if _, ok := m["theme"]; !ok {
		m["theme"] = "default"
	}
	return m
}

// init function is called at boot
func init() {
	plugs.AddExporter("confluence", func() plugs.Exporter {
		return &OrgConfluenceExporter{Props: ValidateMap(map[string]interface{}{}), TemplatePath: "html_default.tpl"}
	})
}
