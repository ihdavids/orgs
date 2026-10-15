// EXPORTER: todo.txt

package todotxtexp

/* SDOC: Exporters

* todo.txt
	Writes headings with a TODO keyword as a todo.txt list
	(https://github.com/todotxt/todo.txt), one task a line, for the phone
	apps and command line tools that read it.

	#+BEGIN_SRC bash
	orgs export -f todotxt inbox.org -out todo.txt
	orgs export -f todotxt 'IsTodo() && HasTag("work")' -out work.txt
	#+END_SRC

	The query is a file (every task in it) or an expression (the tasks it
	finds). It needs no settings and need not be listed in the yaml.

	| org                               | todo.txt                         |
	|-----------------------------------+----------------------------------|
	| a done keyword, =CLOSED:=         | =x 2026-10-12=                   |
	| =[#A]=                            | =(A)=, or =pri:A= once done      |
	| =:CREATED:= property              | the creation date                |
	| the parent heading, =:PROJECTS:=  | =+project= words                 |
	| tags (=@phone= or =phone=)        | =@phone=                         |
	| =DEADLINE:= / =SCHEDULED:=        | =due:= / =t:=                    |
	| a repeater =+1w= / =.+1w=         | =rec:+1w= / =rec:1w=             |
	| other one-word properties         | =key:value=                      |

	=orgs import todo.txt= reads it back by the same rules, so a list can go
	out to a phone and come home again.
EDOC */

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/app/orgs/plugs"
	"github.com/ihdavids/orgs/internal/common"
	"github.com/ihdavids/orgs/internal/common/todotxt"
)

type Exporter struct {
	// Leave out finished tasks: todo.txt keeps those in done.txt.
	OpenOnly bool `yaml:"openOnly"`
	// Properties never written as key:value; added to the built in ones.
	SkipProps []string `yaml:"skipProps"`
	pm        *common.PluginManager
}

func (self *Exporter) Unmarshal(unmarshal func(interface{}) error) error { return unmarshal(self) }
func (self *Exporter) Startup(manager *common.PluginManager, opts *common.PluginOpts) {
	self.pm = manager
}
func (self *Exporter) Standalone() bool { return true }

func (self *Exporter) Export(db common.ODb, query string, to string, opts string, props map[string]string) error {
	err, txt := self.ExportToString(db, query, opts, props)
	if err != nil {
		return err
	}
	return plugs.WriteExportFile(to, txt)
}

func (self *Exporter) ExportToString(db common.ODb, query string, opts string, props map[string]string) (error, string) {
	secs, err := plugs.Headings(db, query)
	if err != nil {
		return err, ""
	}
	tasks := []todotxt.Task{}
	for _, s := range secs {
		if s.Headline.Status == "" {
			continue
		}
		t := self.Task(s)
		if t.Done && self.OpenOnly {
			continue
		}
		tasks = append(tasks, t)
	}
	if len(tasks) == 0 {
		return fmt.Errorf("no tasks: nothing in %q has a TODO keyword", query), ""
	}
	return nil, todotxt.Format(tasks)
}

// Properties that are org's own bookkeeping, or are written as something
// else (the creation date, the projects), and so never become key:value.
var skipProps = map[string]bool{
	"CREATED": true, "PROJECTS": true, "CATEGORY": true, "LOGGING": true,
	"ORDERED": true, "STYLE": true, "COOKIE_DATA": true, "VISIBILITY": true,
	"COLUMNS": true, "LAST_REPEAT": true, "ARCHIVE": true, "ARCHIVE_TIME": true,
	"ARCHIVE_FILE": true, "ARCHIVE_OLPATH": true, "ARCHIVE_CATEGORY": true,
	"ARCHIVE_TODO": true, "ARCHIVE_ITAGS": true, "CUSTOM_ID": true,
	// Where an imported calendar entry came from, which a todo.txt app has
	// no use for.
	"ICAL_UID": true,
}

var isoDate = regexp.MustCompile(`\d{4}-\d{2}-\d{2}`)

func dateOf(s string) time.Time {
	if m := isoDate.FindString(s); m != "" {
		if t, err := time.ParseInLocation("2006-01-02", m, time.Local); err == nil {
			return t
		}
	}
	return time.Time{}
}

// Task is one heading as a todo.txt task.
func (self *Exporter) Task(s *org.Section) todotxt.Task {
	h := s.Headline
	t := todotxt.Task{
		Done:     h.IsDone(),
		Priority: strings.ToUpper(h.Priority),
		Text:     plugs.Title(h),
	}
	if h.Closed != nil && h.Closed.Date != nil {
		t.Completed = h.Closed.Date.Start
	}
	prop := func(k string) string {
		if h.Properties == nil {
			return ""
		}
		v, _ := h.Properties.Get(k)
		return strings.TrimSpace(v)
	}
	t.Created = dateOf(prop("CREATED"))

	// The project is the heading it sits under, as the importer puts it
	// there; any others were kept in :PROJECTS:.
	if p := s.Parent; p != nil && p.Headline != nil {
		if w := todotxt.Word(plugs.Title(p.Headline)); w != "" {
			t.Projects = append(t.Projects, w)
		}
	}
	for _, w := range strings.Fields(prop("PROJECTS")) {
		if w = strings.TrimPrefix(w, "+"); w != "" && !has(t.Projects, w) {
			t.Projects = append(t.Projects, w)
		}
	}
	for _, tag := range h.Tags {
		if c := strings.TrimPrefix(tag, "@"); c != "" {
			t.Contexts = append(t.Contexts, c)
		}
	}

	rec := ""
	if h.Deadline != nil && h.Deadline.Date != nil {
		t.Set("due", h.Deadline.Date.Start.Format("2006-01-02"))
		rec = todotxt.OrgToRec(h.Deadline.Date.RepeatString())
	}
	if h.Scheduled != nil && h.Scheduled.Date != nil {
		t.Set("t", h.Scheduled.Date.Start.Format("2006-01-02"))
		if rec == "" {
			rec = todotxt.OrgToRec(h.Scheduled.Date.RepeatString())
		}
	}
	if rec != "" && !t.Done {
		t.Set("rec", rec)
	}
	if h.Properties != nil {
		skip := map[string]bool{}
		for _, k := range self.SkipProps {
			skip[strings.ToUpper(k)] = true
		}
		for _, kv := range h.Properties.Properties {
			if len(kv) < 2 {
				continue
			}
			k, v := strings.ToUpper(strings.TrimSpace(kv[0])), strings.TrimSpace(kv[1])
			// A value with a space in it would end at the space when read
			// back, and the rest would join the task's text.
			if v == "" || strings.ContainsAny(v, " \t") || skipProps[k] || skip[k] ||
				strings.HasSuffix(k, "_ALL") || strings.HasSuffix(k, "+") {
				continue
			}
			t.Set(strings.ToLower(k), v)
		}
	}
	return t
}

func has(vs []string, v string) bool {
	for _, x := range vs {
		if x == v {
			return true
		}
	}
	return false
}

func init() {
	common.AddExporter("todotxt", func() common.Exporter { return &Exporter{} })
}
