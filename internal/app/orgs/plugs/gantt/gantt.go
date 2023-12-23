//lint:file-ignore ST1006 allow the use of self
// EXPORTER: Gantt Chart
// This is a google gantt chart based exporter. It returns an
// html page structured to display a google gantt chart with the respective nodes.

package gantt

import (
	"bufio"
	"bytes"
	"fmt"
	"html"
	"log"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/ihdavids/orgs/internal/app/orgs/plugs"
	"github.com/ihdavids/orgs/internal/common"
)

var docStart = `
<!DOCTYPE html>
<html>
<head>
  <link rel="stylesheet" href="https://fonts.googleapis.com/css2?family={{.fontfamily}}">  
  <script type="text/javascript" src="https://www.gstatic.com/charts/loader.js"></script>

<style>
text {
  font-family: "{{.fontfamily}}" !important;
}
body {
  font-family: "{{.fontfamily}}", sans-serif;
}
</style>  
  <script type="text/javascript">
    google.charts.load('current', {'packages':['gantt']});
    google.charts.setOnLoadCallback(drawChart);

    function daysToMilliseconds(days) {
      return days * 24 * 60 * 60 * 1000;
    }

    function addMarker(container, markerRow, markerDate) {
      var baseline;
      var baselineBounds;
      var chartElements;
      var marker;
      var markerSpan;
      var rowLabel;
      var svg;
      var svgNS;
      var gantt;
      var ganttUnit;
      var ganttWidth;
      var ganttHeight;
      var timespan;
      var xCoord;
      var yCoord;

    // initialize chart elements
    baseline = null;
    gantt = null;
    rowLabel = null;
    svg = null;
    svgNS = null;
    chartElements = container.getElementsByTagName('svg');
    if (chartElements.length > 0) {
      svg = chartElements[0];
      svgNS = svg.namespaceURI;
    }
    chartElements = container.getElementsByTagName('rect');
    if (chartElements.length > 0) {
      gantt = chartElements[0];
    }
    chartElements = container.getElementsByTagName('path');
    if (chartElements.length > 0) {
      Array.prototype.forEach.call(chartElements, function(path) {
        if ((baseline === null) && (path.getAttribute('fill') !== 'none')) {
          baseline = path;
        }
      });
    }
    chartElements = container.getElementsByTagName('text');
    if (chartElements.length > 0) {
      Array.prototype.forEach.call(chartElements, function(label) {
        if (label.textContent === markerRow) {
          rowLabel = label;
        }
      });
    }
    if ((svg === null) || (gantt === null) || (baseline === null) || (rowLabel === null) ||
        (markerDate.getTime() < dateRangeStart.min.getTime()) ||
        (markerDate.getTime() > dateRangeEnd.max.getTime())) {
      return;
    }

    // calculate placement
    ganttWidth = parseFloat(gantt.getAttribute('width'));
    ganttHeight = parseFloat(gantt.getAttribute('height'));
    baselineBounds = baseline.getBBox();
    timespan = dateRangeEnd.max.getTime() - dateRangeStart.min.getTime();
    ganttUnit = (ganttWidth - baselineBounds.x) / timespan;
    markerSpan = markerDate.getTime() - dateRangeStart.min.getTime();

    // add marker
    marker = document.createElementNS(svgNS, 'polygon');
    marker.setAttribute('fill', 'transparent');
    marker.setAttribute('stroke', '#ffeb3b');
    marker.setAttribute('stroke-width', '3');
    xCoord = (baselineBounds.x + (ganttUnit * markerSpan) - 4);
    yCoord = parseFloat(rowLabel.getAttribute('y'));
    //marker.setAttribute('points', xCoord + ',' + (yCoord - 10) + ' ' + (xCoord - 5) + ',' + yCoord + ' ' + (xCoord + 5) + ',' + yCoord);
    marker.setAttribute('points', xCoord + ',0' + ' ' + xCoord + ',' + ganttHeight);
    svg.insertBefore(marker, rowLabel.parentNode);
  }



    function drawChart() {

      var data = new google.visualization.DataTable();
      data.addColumn('string', 'Task ID');
      data.addColumn('string', 'Task Name');
      data.addColumn('string', 'Resource');
      data.addColumn('date',   'Start Date');
      data.addColumn('date',   'End Date');
      data.addColumn('number', 'Duration');
      data.addColumn('number', 'Percent Complete');
      data.addColumn('string', 'Dependencies');
      data.addRows([
`
var startMarkers = `
	  ]);

  var container = document.getElementById('chart_div');
  var chart = new google.visualization.Gantt(container);
  google.visualization.events.addListener(chart, 'ready', function () {
    // add marker for current date
	addMarker(container, 'Rework TC Workflow', new Date(2023, 11, 3));
`

var endMarkers = `
`

//addMarker('Find sources', new Date(2019, 0, 3));
//addMarker('Outline paper', new Date(2019, 0, 5, 12));
//addMarker('Write paper', new Date(2019, 0, 8));

var docEnd = `
});
var options = {
height: 2200,
is3D: true,
title: '{{.title}}',
gantt: {
  trackHeight: {{.trackheight}},
  shadowEnabled: false,
  barHeight: 15,
  criticalPathEnabled: true,
  shadowOffset: 0,
  barCornerRadius: 2,
          innerGridHorizLine: {
            stroke: '#dddddd',
            strokeWidth: 0
          },
          innerGridTrack: {fill: '#dddddd'},
          innerGridDarkTrack: {fill: '#d7d7d7'} 
}
};

var chart = new google.visualization.Gantt(document.getElementById('chart_div'));

chart.draw(data, options);
}
</script>
</head>
<body>
<div id="chart_div" style="overflow:scroll; height:2200;"></div>
</body>
</html>
`

type Gantt struct {
	Props map[string]interface{}
}

func (self *Gantt) Unmarshal(unmarshal func(interface{}) error) error {
	return unmarshal(self)
}

func fileNameWithoutExt(fileName string) string {
	return fileName[:len(fileName)-len(filepath.Ext(fileName))]
}

func agendaFilenameTag(fileName string) string {
	return fileNameWithoutExt(filepath.Base(fileName))
}

func formatDateForGantt(tm time.Time) string {
	out := "null"
	out = fmt.Sprintf("new Date(%d,%02d,%02d)", tm.Year(), tm.Month()-1, tm.Day())
	return out
}

func CheckPushDownChild(have map[string]*common.Todo, db plugs.ODb, n *common.Todo) *common.Todo {
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

func After(have map[string]*common.Todo, db plugs.ODb, n *common.Todo) *common.Todo {
	var dep *common.Todo = nil
	if p, ok := n.Props["AFTER"]; ok && p != "" {
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

func GetResource(db plugs.ODb, resource string, td *common.Todo) string {
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

func EscapeQuotes(str string) string {
	return html.EscapeString(strings.ReplaceAll(str, ",", ""))
}

func ReplaceQuotes(str string) string {
	return strings.ReplaceAll(str, "\"", "")
}

func (self *Gantt) ExportRes(o *bytes.Buffer, db plugs.ODb, have map[string]*common.Todo, idx int, td *common.Todo) {
	resource := "unknown"
	percentDone := "0"
	duration := "1"

	now := time.Now()
	start := fmt.Sprintf("new Date(%d,%02d,%02d)", now.Year(), now.Month()-1, now.Day())
	end := fmt.Sprintf("new Date(%d,%02d,%02d)", now.Year(), now.Month()-1, now.Day())
	if td.Date != nil {
		start = formatDateForGantt(td.Date.Start)
		if td.Date.Start != td.Date.End {
			end = formatDateForGantt(td.Date.End)
		} else {
			end = "null"
		}
	}
	atd := After(have, db, td)
	after := "null"
	if atd != nil && atd.Headline != "" {
		if _, ok := have[atd.Hash]; !ok {
			have[atd.Hash] = atd
			self.ExportRes(o, db, have, idx, atd)
			idx += 1
		}
		after = "\"" + EscapeQuotes(strings.TrimSpace(atd.Headline)) + "\""
	}
	resource = GetResource(db, resource, td)
	if estimate, ok := td.Props["EFFORT"]; ok {
		if estimate != "" {
			log.Printf("EFFORT: %s\n", estimate)
			dur := common.ParseDuration(estimate)
			if dur != nil {
				duration = fmt.Sprintf("%f", dur.Days())
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
		duration = "1"
		end = "null"
	}
	if after != "null" {
		start = "null"
		end = "null"
	}
	//filename := agendaFilenameTag(td.Filename)
	//section  = entry['section'] if 'section' in entry else None
	//estimate = n.get_property("EFFORT","")

	prefix := ""
	if idx > 0 {
		prefix = ","
	}

	if !td.IsActive {
		percentDone = "100"
	} else {
		if res, ok := td.Props["PERCENTDONE"]; ok {
			percentDone = res
		}
	}
	m := map[string]interface{}{"name": EscapeQuotes(strings.TrimSpace(td.Headline)), "start": start, "end": end, "duration": duration, "percent": percentDone, "resource": resource, "prefix": prefix, "after": after}
	ExpandTemplateIntoBuf(o, "{{.prefix}}[\"{{.name}}\",\"{{.name}}\",\"{{.resource}}\", {{.start}},{{.end}},daysToMilliseconds({{.duration}}),{{.percent}},{{.after}}]\n", m)
}

func ExpandTemplateIntoBuf(o *bytes.Buffer, temp string, m map[string]interface{}) {
	t := template.Must(template.New("").Parse(temp))
	t.Execute(o, m)
}

func (self *Gantt) Export(db plugs.ODb, query string, to string, opts string) error {
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
	ExpandTemplateIntoBuf(o, docStart, self.Props)
	for idx, td := range tds {
		//line = ""
		//if(dep != None && dep != "") {
		//    line += "[\"{name}\",\"{name}\",\"{resource}\", {start},{end},daysToMilliseconds({duration}),{percent},\"{after}\"]\n".format(name=n.heading,idx=idx,after=str(dep),duration=duration,start=start,end=end,percent=percentDone,resource=resource)
		//} else {
		//}
		self.ExportRes(o, db, have, idx, &td)
	}
	ExpandTemplateIntoBuf(o, docEnd, self.Props)
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

func (self *Gantt) ExportToString(db plugs.ODb, query string, opts string) (error, string) {
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
	ExpandTemplateIntoBuf(o, docStart, self.Props)

	for idx, td := range tds {
		self.ExportRes(o, db, have, idx, &td)
	}
	ExpandTemplateIntoBuf(o, startMarkers, self.Props)
	ExpandTemplateIntoBuf(o, endMarkers, self.Props)
	ExpandTemplateIntoBuf(o, docEnd, self.Props)
	txt := o.String()
	fmt.Printf("%s\n", txt)
	return res, txt
}

func (self *Gantt) Startup(manager *plugs.PluginManager, opts *plugs.PluginOpts) {
}

func NewGantt() *Gantt {
	var g *Gantt = new(Gantt)
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

	return m
}

// init function is called at boot
func init() {
	plugs.AddExporter("gantt", func() plugs.Exporter {
		return &Gantt{Props: ValidateMap(map[string]interface{}{})}
	})
}

/*
google.charts.load('current', {
  packages:['gantt']
}).then(function () {
  var container = document.getElementById('gantt');
  var chart = new google.visualization.Gantt(container);

  var dataTable = new google.visualization.DataTable();
  dataTable.addColumn('string', 'Task ID');
  dataTable.addColumn('string', 'Task Name');
  dataTable.addColumn('string', 'Resource');
  dataTable.addColumn('date', 'Start Date');
  dataTable.addColumn('date', 'End Date');
  dataTable.addColumn('number', 'Duration');
  dataTable.addColumn('number', 'Percent Complete');
  dataTable.addColumn('string', 'Dependencies');
  dataTable.addRows([
    ['Research', 'Find sources', null, new Date(2019, 0, 1), new Date(2019, 0, 5), null,  100,  null],
    ['Write', 'Write paper', 'write', null, new Date(2019, 0, 9), daysToMilliseconds(3), 25, 'Research,Outline'],
    ['Cite', 'Create bibliography', 'write', null, new Date(2019, 0, 7), daysToMilliseconds(1), 20, 'Research'],
    ['Complete', 'Hand in paper', 'complete', null, new Date(2019, 0, 10), daysToMilliseconds(1), 0, 'Cite,Write'],
    ['Outline', 'Outline paper', 'write', null, new Date(2019, 0, 6), daysToMilliseconds(1), 100, 'Research']
  ]);

  var dateRangeStart = dataTable.getColumnRange(3);
  var dateRangeEnd = dataTable.getColumnRange(4);
  var formatDate = new google.visualization.DateFormat({
    pattern: 'MM/dd/yyyy'
  });
  var rowHeight = 45;

  var options = {
    height: ((dataTable.getNumberOfRows() * rowHeight) + rowHeight),
    gantt: {
      criticalPathEnabled: true,
      criticalPathStyle: {
        stroke: '#e64a19',
        strokeWidth: 5
      }
    }
  };

  function daysToMilliseconds(days) {
    return days * 24 * 60 * 60 * 1000;
  }

  function drawChart() {
    chart.draw(dataTable, options);
  }

  function addMarker(markerRow, markerDate) {
    var baseline;
    var baselineBounds;
    var chartElements;
    var marker;
    var markerSpan;
    var rowLabel;
    var svg;
    var svgNS;
    var gantt;
    var ganttUnit;
    var ganttWidth;
    var timespan;
    var xCoord;
    var yCoord;

    // initialize chart elements
    baseline = null;
    gantt = null;
    rowLabel = null;
    svg = null;
    svgNS = null;
    chartElements = container.getElementsByTagName('svg');
    if (chartElements.length > 0) {
      svg = chartElements[0];
      svgNS = svg.namespaceURI;
    }
    chartElements = container.getElementsByTagName('rect');
    if (chartElements.length > 0) {
      gantt = chartElements[0];
    }
    chartElements = container.getElementsByTagName('path');
    if (chartElements.length > 0) {
      Array.prototype.forEach.call(chartElements, function(path) {
        if ((baseline === null) && (path.getAttribute('fill') !== 'none')) {
          baseline = path;
        }
      });
    }
    chartElements = container.getElementsByTagName('text');
    if (chartElements.length > 0) {
      Array.prototype.forEach.call(chartElements, function(label) {
        if (label.textContent === markerRow) {
          rowLabel = label;
        }
      });
    }
    if ((svg === null) || (gantt === null) || (baseline === null) || (rowLabel === null) ||
        (markerDate.getTime() < dateRangeStart.min.getTime()) ||
        (markerDate.getTime() > dateRangeEnd.max.getTime())) {
      return;
    }

    // calculate placement
    ganttWidth = parseFloat(gantt.getAttribute('width'));
    baselineBounds = baseline.getBBox();
    timespan = dateRangeEnd.max.getTime() - dateRangeStart.min.getTime();
    ganttUnit = (ganttWidth - baselineBounds.x) / timespan;
    markerSpan = markerDate.getTime() - dateRangeStart.min.getTime();

    // add marker
    marker = document.createElementNS(svgNS, 'polygon');
    marker.setAttribute('fill', 'transparent');
    marker.setAttribute('stroke', '#ffeb3b');
    marker.setAttribute('stroke-width', '3');
    xCoord = (baselineBounds.x + (ganttUnit * markerSpan) - 4);
    yCoord = parseFloat(rowLabel.getAttribute('y'));
    marker.setAttribute('points', xCoord + ',' + (yCoord - 10) + ' ' + (xCoord - 5) + ',' + yCoord + ' ' + (xCoord + 5) + ',' + yCoord);
    svg.insertBefore(marker, rowLabel.parentNode);
  }

  google.visualization.events.addListener(chart, 'ready', function () {
    // add marker for current date
    addMarker('Find sources', new Date(2019, 0, 3));
    addMarker('Outline paper', new Date(2019, 0, 5, 12));
    addMarker('Write paper', new Date(2019, 0, 8));
  });

  window.addEventListener('resize', drawChart, false);
  drawChart();
});

*/
