package todoist

/* SDOC: Pollers

* Todoist

	TODO More documentation on this module

	#+BEGIN_SRC yaml
    - name: "todoist"
      token: "todoist api token"
	#+END_SRC

EDOC */
import (
	"context"
	"fmt"
	"os"

	"github.com/ihdavids/orgs/internal/common"

	"github.com/ides15/todoist"
)

type Todoist struct {
	Name   string
	Token  string
	client *todoist.Client
}

func (self *Todoist) Unmarshal(unmarshal func(interface{}) error) error {
	return unmarshal(self)
}

func (self *Todoist) Update(db common.ODb) {
	fmt.Fprintf(os.Stderr, "Todoist Update...\n")
	if self.client != nil {
		projects, _, err := self.client.Projects.List(context.Background(), "")
		if err != nil {
			panic(err)
		}
		tasks, _, err := self.client.Tasks.List(context.Background(), "")
		if err != nil {
			panic(err)
		}

		for _, p := range projects {
			if p.IsDeleted == 0 && p.IsArchived == 0 {
				fmt.Fprintf(os.Stderr, "* %-25s :Project:\n", p.Name)
				for _, t := range tasks {
					if t.IsDeleted == 0 && t.ProjectID == p.ID {
						status := "TODO"
						if t.Checked == 1 {
							status = "DONE"
						}
						fmt.Fprintf(os.Stderr, "** %s %s\n", status, t.Content)
						if t.Due != nil {
							d := (*t.Due).(map[string]interface{})
							//for key, element := range d {
							//	fmt.Fprintf(os.Stderr, "%s, %v\n", key, element)
							//}
							if dt, ok := d["date"]; ok {
								fmt.Fprintf(os.Stderr, "   DEADLINE: <%s>\n", dt)
							}
						}
						fmt.Fprintf(os.Stderr, "   :PROPERTIES:\n")
						if t.DateAdded != "" {
							fmt.Fprintf(os.Stderr, "     :Created: %s\n", t.DateAdded)
						}
						if t.DateCompleted != nil {
							fmt.Fprintf(os.Stderr, "     :Completed: %s\n", *t.DateCompleted)
						}
						fmt.Fprintf(os.Stderr, "   :END:\n")
						if t.Description != "" {
							fmt.Fprintf(os.Stderr, "   %s\n", t.Description)
						}
					}
				}
			}
		}
	}
}

func (self *Todoist) Startup(freq int, manager *common.PluginManager, opts *common.PluginOpts) {
	var err error
	self.client, err = todoist.NewClient(self.Token)
	if err != nil {
		panic(err)
	}
}

// init function is called at boot
func init() {
	common.AddPoller("todoist", func() common.Poller {
		return &Todoist{}
	})
}
