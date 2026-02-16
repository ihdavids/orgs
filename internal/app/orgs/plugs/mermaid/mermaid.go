//lint:file-ignore ST1006 allow the use of self
// EXPORTER: Gantt Chart
/* SDOC: Exporters

* Mermaid Gantt
  This is a mermaid JS based gantt chart exporter.
  This returns an html page with a mermaid js rendering of the
  requested query.

  Unlike the google gantt chart this has swim lane
  based sections and does not support resource based
  bar coloring.

	TODO More documentation on this module

	#+BEGIN_SRC yaml
  - name: "mermaid"
	#+END_SRC

EDOC */

package mermaid

import (
	"bufio"
	"bytes"
	"fmt"
	"html"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/app/orgs/plugs"
	"github.com/ihdavids/orgs/internal/common"
)

var docStart = `
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <link rel="stylesheet" href="https://fonts.googleapis.com/css2?family={{.fontfamily}}">
  {{.userheaderscript}}
</head>
<body>
  <div class="mermaid" style="background-color: #DCDCDC; resize: both;">
  gantt
  	dateFormat YYYY-MM-DD
  	axisFormat %y-%m-%d
	tickInterval 2week
	excludes    weekends  
  	title {{.title}}
`
var startMarkers = `
`

var endMarkers = `
`

var docEnd = `
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

type Mermaid struct {
	Props map[string]interface{}
}

func (self *Mermaid) Unmarshal(unmarshal func(interface{}) error) error {
	return unmarshal(self)
}

func agendaFilenameTag(fileName string) string {
	return plugs.FileNameWithoutExt(filepath.Base(fileName))
}

func formatDateForGantt(tm time.Time) string {
	out := "null"
	out = fmt.Sprintf("%d-%02d-%02d", tm.Year(), tm.Month(), tm.Day())
	return out
}

func CheckPushDownChild(have map[string]*common.Todo, db common.ODb, n *common.Todo) *common.Todo {
	if n != nil {
		if _, ok := have[n.Hash]; !ok {
			if _, ok2 := n.Props["ORDERED"]; ok2 {
				lastChild := db.FindLastChild(n.Hash)
				if lastChild != nil {
					// Okay is this last child in our have list?
					if _, ok := have[lastChild.Hash]; ok {
						return lastChild
					}
				}
			}
		}
	}
	return n
}

func After(have map[string]*common.Todo, db common.ODb, n *common.Todo) *common.Todo {
	var dep *common.Todo = nil
	if p, ok := n.Props["AFTER"]; ok && p != "" {
		//fmt.Printf("AFTER: %s\n", n.Props["AFTER"])
		// Search by ID
		d := db.FindByAnyId(p)
		// If we have a node, but the node is not in proper list
		// and the node is ordered then grab it's children
		return CheckPushDownChild(have, db, d)
	} else {
		curNode := n
		if curNode == nil {
			return nil
		}
		for curNode.Parent != "" {
			par := db.FindByHash(curNode.Parent)
			if par == nil {
				//fmt.Printf("Cannot find parent of %v : %v\n", curNode.Headline, curNode.Parent)
				break
			}
			if _, ok := par.Props["ORDERED"]; ok {
				prevSib := db.FindPrevSibling(curNode.Hash)
				// Chain to parent in ORDERED setup.
				if prevSib == nil {
					// if parent is Not in have then try to find what parent is after!
					if _, ok := have[par.Hash]; !ok {
						pafter := After(have, db, par)
						if pafter != nil {
							prevSib = pafter
						}
					} else {
						prevSib = par
					}
				}
				// If this node we found is not in our have but its lst child is...
				// Then we return that.
				prevSib = CheckPushDownChild(have, db, prevSib)
				dep = prevSib
				break
			}
			curNode = par
		}
	}
	return dep
}

func HeadlineAloneHasTag(name string, tags []string) bool {
	for _, t := range tags {
		t = strings.ToLower(strings.TrimSpace(t))
		if t != "" && (t == name) {
			return true
		}
	}
	return false
}

func GetResource(db common.ODb, resource string, td *common.Todo) string {
	if res, ok := td.Props["ASSIGNED"]; ok {
		resource = res
	}
	if res, ok := td.Props["RID"]; ok {
		resource = res
	}
	if res, ok := td.Props["RESOURCEID"]; ok {
		resource = res
	}
	if (resource == "" || resource == "unknown") && td.Parent != "" {
		par := db.FindByHash(td.Parent)
		if par != nil && HeadlineAloneHasTag("project", par.Tags) {
			return strings.TrimSpace(par.Headline)
		}
	}
	return resource
}

func GetSection(db common.ODb, td *common.Todo) string {
	section := ""
	if res, ok := td.Props["SECTION"]; ok {
		section = res
	}
	if (section == "" || section == "unknown") && td.Parent != "" {
		par := db.FindByHash(td.Parent)
		if par != nil && HeadlineAloneHasTag("project", par.Tags) {
			if res, ok := par.Props["SECTION"]; ok {
				section = res
			}
		}
	}
	if section == "" {
		if res, ok := td.Props["ASSIGNED"]; ok {
			section = res
		}
	}
	return section
}

func EscapeQuotes(str string) string {
	return html.EscapeString(strings.ReplaceAll(str, ",", ""))
}

func ReplaceQuotes(str string) string {
	return strings.ReplaceAll(str, "\"", "")
}

func getMermaidName(namesMap map[string]string, td *common.Todo, idx *int) string {
	if name, ok := namesMap[td.Hash]; ok {
		return name
	}
	name := fmt.Sprintf("tsk%d", *idx)
	namesMap[td.Hash] = name
	*idx += 1
	return name
}

func getSectionName(sectionMap map[string]string, section *string, db common.ODb, td *common.Todo) string {
	sec := GetSection(db, td)
	actualSection := ""
	if sec != "" && *section != sec {
		actualSection = sec
		*section = sec
	}
	if *section == "" {
		actualSection = "main"
		*section = "main"
	}
	if actualSection != "" {
		trial := actualSection
		idx := 1
		for {
			if _, ok := sectionMap[trial]; ok {
				idx += 1
				trial = fmt.Sprintf("%s %d", actualSection, idx)
				continue
			}
			actualSection = trial
			sectionMap[actualSection] = "t"
			break
		}
	}
	return actualSection
}

func haveDate(td *common.Todo) bool {
	return td.Date != nil && td.Date.TimestampType == org.Active
}

func (self *Mermaid) ExportRes(o *bytes.Buffer, db common.ODb, sectionMap map[string]string, namesMap map[string]string, have map[string]*common.Todo, idx *int, td *common.Todo, section *string) {
	resource := "unknown"
	percentDone := "0"
	duration := "1d"

	now := time.Now()
	start := formatDateForGantt(now)
	end := formatDateForGantt(now)
	if haveDate(td) {
		fmt.Printf("TIMESTAMP TYPE: %d\n", td.Date.TimestampType)
		start = formatDateForGantt(td.Date.Start)
		if td.Date.Start != td.Date.End {
			end = formatDateForGantt(td.Date.End)
		} else {
			end = "null"
		}
	}
	afterNode := After(have, db, td)
	after := "null"
	if afterNode != nil && afterNode.Headline != "" {
		if _, ok := have[afterNode.Hash]; !ok {
			have[afterNode.Hash] = afterNode
			self.ExportRes(o, db, sectionMap, namesMap, have, idx, afterNode, section)
		}
		//after = "\"" + EscapeQuotes(strings.TrimSpace(afterNode.Headline)) + "\""
		after = getMermaidName(namesMap, afterNode, idx)
	}

	resource = GetResource(db, resource, td)
	actualSection := getSectionName(sectionMap, section, db, td)
	if estimate, ok := td.Props["EFFORT"]; ok {
		if estimate != "" {
			log.Printf("EFFORT: %s\n", estimate)
			dur := common.ParseDuration(estimate)
			if dur != nil {
				duration = fmt.Sprintf("%.1fd", dur.Days())
				if td.Date != nil {
					tend := td.Date.End
					tend.Add(dur.Duration())
					end = formatDateForGantt(tend)
				} else {
					tend := time.Now()
					tend.Add(dur.Duration())
					end = formatDateForGantt(tend)
				}
				end = "null"
			}
			//end = dt + duration.timedelta()
			//duration = duration.days()
		}
	} else {
		duration = "1d"
		end = "null"
	}
	name := EscapeQuotes(strings.TrimSpace(td.Headline))
	if after != "null" && !haveDate(td) {
		start = fmt.Sprintf("after %s", after)
		end = "null"
	}

	hash := getMermaidName(namesMap, td, idx)
	prefix := ""
	if plugs.HasP(td, "CRIT") || td.Status == "BLOCKED" {
		prefix += "crit,"
	}
	if plugs.HasP(td, "ACT") || (td.Status == "IN-PROGRESS" || td.Status == "INPROGRESS") {
		prefix += "active,"
	}
	if plugs.HasP(td, "DONE") || td.Status == "DONE" || td.Status == "COMPLETED" {
		prefix += "done,"
	}
	if plugs.HasP(td, "MILESTONE") || td.Status == "MILESTONE" {
		prefix += "milestone,"
	}
	if plugs.HasP(td, "MARK") || td.Status == "MARK" {
		prefix += "vert,"
	}

	if !td.IsActive {
		percentDone = "100"
	} else {
		if res, ok := td.Props["PERCENTDONE"]; ok {
			percentDone = res
		}
	}
	// TODO AFTER should read after "dep"
	m := map[string]interface{}{"name": name, "idx": idx, "hash": hash, "start": start, "end": end, "duration": duration, "percent": percentDone, "resource": resource, "prefix": prefix, "after": after, "sectionName": actualSection}
	if actualSection != "" {
		plugs.ExpandTemplateIntoBuf(o, "\nsection {{.sectionName}}\n", m)
	}
	plugs.ExpandTemplateIntoBuf(o, "\t{{.name}}\t:{{.prefix}}{{.hash}},{{.start}},{{.duration}}\n", m)
}

func (self *Mermaid) Export(db common.ODb, query string, to string, opts string, props map[string]string) error {
	ValidateMap(self.Props)
	fmt.Printf("GANTT: Export called", query, to, opts)
	tds, err := db.QueryTodosExpr(query)
	if err != nil {
		msg := fmt.Sprintf("ERROR: gantt failed to query expression, %v [%s]\n", err, query)
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
	plugs.ExpandTemplateIntoBuf(o, docStart, self.Props)
	var section = ""
	var idx = 0
	namesMap := map[string]string{}
	sectionMap := map[string]string{}
	for _, td := range tds {
		//line = ""
		//if(dep != None && dep != "") {
		//    line += "[\"{name}\",\"{name}\",\"{resource}\", {start},{end},daysToMilliseconds({duration}),{percent},\"{after}\"]\n".format(name=n.heading,idx=idx,after=str(dep),duration=duration,start=start,end=end,percent=percentDone,resource=resource)
		//} else {
		//}
		self.ExportRes(o, db, sectionMap, namesMap, have, &idx, &td, &section)
	}
	plugs.ExpandTemplateIntoBuf(o, docEnd, self.Props)
	fo, err := os.Create(to)
	if err == nil {
		w := bufio.NewWriter(fo)
		_, err2 := w.WriteString(o.String())
		if err2 != nil {
			msg := fmt.Sprintf("Failed to write file[%v]: %v\n", err2, to)
			log.Printf(msg)
			res = fmt.Errorf(msg)
		}
		w.Flush()
		fo.Close()
	} else {
		msg := fmt.Sprintf("Failed to open file[%v]: %v\n", err, to)
		log.Printf(msg)
		log.Printf("DOC:\n\n")
		log.Printf("%v\n", o.String())
		res = fmt.Errorf(msg)
	}
	return res
}

func (self *Mermaid) ExportToString(db common.ODb, query string, opts string, props map[string]string) (error, string) {
	self.Props = ValidateMap(self.Props)
	fmt.Println("GANTT: Export string called", query, opts)
	tds, err := db.QueryTodosExpr(query)
	if err != nil {
		msg := fmt.Sprintf("ERROR: gantt failed to query expression, %v [%s]\n", err, query)
		log.Println(msg)
		return fmt.Errorf(msg), ""
	}
	have := map[string]*common.Todo{}
	for _, td := range tds {
		have[td.Hash] = &td
	}
	var res error = nil
	o := bytes.NewBufferString("")
	fmt.Println("++++++++++++++++++++++++++++++++++++++++++++++++++++++")
	fmt.Println(self.Props)
	plugs.ExpandTemplateIntoBuf(o, docStart, self.Props)

	namesMap := map[string]string{}
	sectionMap := map[string]string{}
	section := ""
	idx := 0
	for _, td := range tds {
		self.ExportRes(o, db, sectionMap, namesMap, have, &idx, &td, &section)
	}
	plugs.ExpandTemplateIntoBuf(o, startMarkers, self.Props)
	plugs.ExpandTemplateIntoBuf(o, endMarkers, self.Props)
	plugs.ExpandTemplateIntoBuf(o, docEnd, self.Props)
	txt := o.String()
	fmt.Printf("%s\n", txt)
	return res, txt
}

func (self *Mermaid) Startup(manager *common.PluginManager, opts *common.PluginOpts) {
}

func NewMermaid() *Mermaid {
	var g *Mermaid = new(Mermaid)
	return g
}

func ValidateMap(m map[string]interface{}) map[string]interface{} {
	if _, ok := m["title"]; !ok {
		m["title"] = "Schedule"
	}
	if _, ok := m["fontfamily"]; !ok {
		m["fontfamily"] = "Inconsolata"
	}
	if _, ok := m["trackheight"]; !ok {
		m["trackheight"] = 30
	}
	if _, ok := m["userbodyscript"]; !ok {
		m["userbodyscript"] = plugs.UserBodyScriptBlock()
	}
	if _, ok := m["userheaderscript"]; !ok {
		m["userheaderscript"] = plugs.UserHeaderScriptBlock()
	}

	return m
}

// init function is called at boot
func init() {
	common.AddExporter("mermaid", func() common.Exporter {
		return &Mermaid{Props: ValidateMap(map[string]interface{}{})}
	})
	common.AddExporter("mindmap", func() common.Exporter {
		return &MermaidMindMap{Props: ValidateMap(map[string]interface{}{})}
	})
}
