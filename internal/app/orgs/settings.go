//lint:file-ignore ST1006 allow the use of self
package orgs

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	"github.com/ihdavids/orgs/internal/app/orgs/plugs"
	"github.com/ihdavids/orgs/internal/common"
	"github.com/ihdavids/orgs/internal/templates"
	"gopkg.in/op/go-logging.v1"
	"gopkg.in/yaml.v2"
)



type Config struct {
	// Core systems
	PlugManager *plugs.PluginManager
	Out         *logging.Logger
	// Configuration options
	ServePath            string                   `yaml:"servepath"`
	Port                 int                      `yaml:"port"`
	TLSPort              int                      `yaml:"tlsport"`
	ServerCrt            string                   `yaml:"servercrt"`
	ServerKey            string                   `yaml:"serverkey"`
	/* SDOC: Settings
* Org Dirs
	A list of directories containing your org files
	#+BEGIN_SRC yaml
  orgDirs: 
    - "/Users/me/dev/gtd"
	#+END_SRC
	EDOC */
	OrgDirs              []string                 `yaml:"orgDirs"`
	CanFailWatch         bool                     `yaml:"canWatchFail"`
	/* SDOC: Settings
* Use Project Tag
	How should we define a project. If this is set a project
	is defined as a heading with a :PROJECT: tag on it.

	#+BEGIN_SRC yaml
  useProjectTag: true
	#+END_SRC

	If this is false then a project is defined to be a headline
	that has a child headline that has a status on it.

	#+BEGIN_SRC org
  * Project
  ** TODO This task makes it a project
	#+END_SRC
	EDOC */
	UseTagForProjects    bool                     `yaml:"useProjectTag"`
	AllowHttp            bool                     `yaml:"allowHttp"`
	AllowHttps           bool                     `yaml:"allowHttps"`
	DefaultTodoStates    string                   `yaml:"defaultTodoStates"`
	TemplatePath         string                   `yaml:"templatePath"`
	DayPageTemplate      string                   `yaml:"dayPageTemplate"`
	/* SDOC: Settings
* Day Page
	The day page system has a number of settings that can be used to control
	its behaviour.

	The first and most important is where your daypages should be generated
	This should be a folder inside your orgDirs.
	#+BEGIN_SRC yaml
  dayPagePath: "/Users/me/dev/gtd/worklog"
	#+END_SRC
	EDOC */
	DayPagePath          string                   `yaml:"dayPagePath"`
	DayPageMode          string                   `yaml:"dayPageMode"`
	DayPageModeWeekDay   string                   `yaml:"dayPageModeWeekDay"`
	DayPageMaxSearchBack int                      `yaml:"dayPageMaxSearch"`
	Plugins              []plugs.PluginDef        `yaml:"plugins"`
	/* SDOC: Settings
* Enabled Exporters, Plugins, Updaters
	The list of enabled exporter modules
	#+BEGIN_SRC yaml
	exporters:
    - name: "gantt"
    - name: "mermaid"
    - name: "mindmap"
    - name: "html"
      props:
        fontfamily: "Underdog"
    - name: "revealjs"
    - name: "impressjs"
    - name: "latex"
	#+END_SRC

	The same is true for updaters and plugins.
	You must explicitly enable the modules you wish
	to be active in your orgs installation for them to be available.
	EDOC */
	Exporters            []plugs.ExportDef        `yaml:"exporters"`
	Updaters             []plugs.UpdaterDef       `yaml:"updaters"`
	CaptureTemplates     []common.CaptureTemplate `yaml:"captureTemplates"`
	AccessControl        string                   `yaml:"accessControl"`
	// These specify a valid set of org files that can be searched for valid
	// refile targets
	RefileTargets []string `yaml:"refileTargets"`
	/* SDOC: Settings
* Default Author
	Default author parameter to use when generating new templates
	#+BEGIN_SRC yaml
	 author: "John Smith"
	#+END_SRC
	EDOC */
	Author string `yaml:"author"`
	// What template file to render when generating a new org file
	// This is a file found in the templatePath option
	NewFileTemplate string `yaml:"newFileTemplate"`
	// This parameter is used to control the default archive file is using the archive mechanism.
	// The location where subtrees should be archived.
	//
	// The value of this variable is a string, consisting of two parts,
	// separated by a double-colon. The first part is a filename and
	// the second part is a headline.
	//
	// When the filename is omitted, archiving happens in the same file.
	// %s in the filename will be replaced by the current file
	// name (without the directory part). Archiving to a different file
	// is useful to keep archived entries from contributing to the
	// Org-mode Agenda.
	//
	// The archived entries will be filed as subtrees of the specified
	// headline. When the headline is omitted, the subtrees are simply
	// filed away at the end of the file, as top-level entries. Also in
	// the heading you can use %s to represent the file name, this can be
	// useful when using the same archive for a number of different files.
	ArchiveDefaultTarget string `yaml:"archiveDefault"`
	// file from where the entry came, its outline path the archiving time
	// org-archive-save-context-info
	// Parts of context info that should be stored as properties when archiving.
	// When a subtree is moved to an archive file, it loses information given by
	// context, like inherited tags, the category, and possibly also the TODO
	// state (depending on the variable `org-archive-mark-done').
	// This variable can be a list of any of the following symbols:
	//
	// time       The time of archiving.
	// file       The file where the entry originates.
	// ltags      The local tags, in the headline of the subtree.
	// itags      The tags the subtree inherits from further up the hierarchy.
	// todo       The pre-archive TODO state.
	// category   The category, taken from file name or #+CATEGORY lines.
	// olpath     The outline path to the item.  These are all headlines above
	//            the current item, separated by /, like a file path.
	//
	// For each symbol present in the list, a property will be created in
	// the archived entry, with a prefix \"ARCHIVE_\", to remember this
	// information."
	ArchiveSaveContextInfo []string `yaml:"archiveSaveContextInfo"`

	// If true empty properties in the ArchiveSaveContextInfo are not included
	// in the archive output
	ArchiveSkipEmptyProperties bool `yaml:"archiveSkipEmptyProperties"`

	// If true tasks are marked done when moved to archive.
	ArchiveMarkDone bool `yaml:"archiveMarkDone"`
	/* SDOC: Settings
* Date Tree Formatting
	These options control the format of your datatree

	#+BEGIN_SRC org
	 * 2006
	 ** January
	 *** 01 Monday
	#+END_SRC

	#+BEGIN_SRC org
	 * 2006
	 ** 2006-01 October
	 *** 2006-01-02 Friday
	 *** 2006-01-02 Saturday
	#+END_SRC

	#+BEGIN_SRC yaml
	dateTreeYearFormat:  "2006"
	dateTreeMonthFormat: "January"
	dateTreeDayFormat:   "01 Monday"
	#+END_SRC

	EDOC */
	DateTreeYearFormat  string `yaml:"dateTreeYearFormat"`
	DateTreeMonthFormat string `yaml:"dateTreeMonthFormat"`
	DateTreeDayFormat   string `yaml:"dateTreeDayFormat"`
	/* SDOC: Settings
* Clock Into Drawer
	When using the clocking features org can choose to dump clock data at the top of a heading
	OR produce the clock data into a drawer of your choosing. This option lets you
	choose the name of the drawer.
	#+BEGIN_SRC yaml
	 clockIntoDrawer: "LOGBOOK"
	#+END_SRC

	This defaults to LOGBOOK

	EDOC */
	ClockIntoDrawer string `yaml:"clockIntoDrawer"`
	/* SDOC: Settings
* Image Path
	Images and fonts are secondary html requests when loading an html
	document in vscode. In addition, orgs may want to provide
	quick links et al.

	#+BEGIN_SRC yaml
	 templateImagesPath: "<path>"
	 templateFontPath: "<path>"
	#+END_SRC

	This defaults to: ./templates/html_styles/images
	EDOC */
	TemplateImagesPath string `yaml:"templateImagesPath"`
	TemplateFontPath   string `yaml:"templateFontPath"`
	/* SDOC: Settings
* Tag Groups

  Tag Groups are a cheater way of helping to make your queries less verbose.
  They are defined on the orgs server and provide a grouping of tags that you
  can query against.

  Here is a quick example. The following entry in your orgs.yaml
  allows you to treat PERSONAL as any of FAMILY ME or PERSONAL tags
  and WORK as any of WORK BACKLOG or PROJECTX tags in an InTagGroup('GROUPNAME')
  query.

  #+BEGIN_SRC yaml
	tagGroups:
	  PERSONAL:
	    - FAMILY
	    - ME
	    - PERSONAL
	  WORK:
	    - WORK
	    - BACKLOG
	    - PROJECTX
  #+END_SRC

  Here is a query that might be using that in your vscode org.todoConfigs:
  I am defining a Todo view that will have filename, status, headline and a few property columns of various sizes
  and it is looking for active todo's that are either WAITING or BLOCKED that are not tagged with any tags in my PERSONAL tag group.

  #+BEGIN_SRC json
    "Waiting": {
      "query": "!IsProject() && IsTodo() && !IsArchived() && !InTagGroup('PERSONAL') && (IsStatus('WAITING') || IsStatus('BLOCKED')",
      "display": {
        "filename": 15,
        "status": 10,
        "headline": 25,
        "properties": {
          "EFFORT": 5,
          "ASSIGNED": 15
        }
      }
    }
  #+END_SRC
	EDOC */
	TagGroups map[string][]string `yaml:"tagGroups"`
}

func (self *Config) Defaults() {

	self.Out = logging.MustGetLogger("orgs")
	self.ServePath = "/org"
	self.Port = 8010
	self.TLSPort = 443
	self.ServerCrt = "server.crt"
	self.ServerKey = "server.key"
	self.AllowHttp = false
	self.AllowHttps = true
	self.Author = ""
	self.TemplatePath = "./templates"
	self.NewFileTemplate = "newfile.tpl"
	self.DayPageTemplate = "daypage.tpl"
	self.DayPagePath = "./daypages"
	self.DayPageMode = "week"
	self.DayPageModeWeekDay = "Monday"
	self.DayPageMaxSearchBack = 30 // How many weeks back should we look to pull last weeks tasks from.
	self.UseTagForProjects = true
	self.CaptureTemplates = []common.CaptureTemplate{}
	self.AccessControl = "null"
	self.RefileTargets = []string{".*\\.org"}
	self.ArchiveDefaultTarget = "%s_archive::"
	self.ArchiveSaveContextInfo = []string{"time", "file", "ltags", "itags", "todo", "category", "olpath"}
	self.ArchiveSkipEmptyProperties = true

	self.DateTreeYearFormat = "2006"
	self.DateTreeMonthFormat = "January"
	self.DateTreeDayFormat = "02 Monday"
	self.ClockIntoDrawer = "LOGBOOK"
	self.TemplateImagesPath = "./templates/html_styles/images"
	self.TemplateFontPath = "./templates/fonts"
}

func (self *Config) Validate() {
	if self.OrgDirs == nil || len(self.OrgDirs) < 1 {
		log.Fatalln("Config file must specify orgDirs parameter!", self.OrgDirs)
	}
}

func (self *Config) ParseCommandLine() {
	// NOTE: for this to work the default should always be the current
	//       value of the structure. Avoid using a default here
	//       instead specify it in Defaults up above.
	flag.StringVar(&self.ServePath, "servepath", self.ServePath, "serve path")
	flag.IntVar(&self.Port, "port", 8010, "serve port")
	flag.IntVar(&self.TLSPort, "tlsport", 443, "tls serve port")
	flag.Parse()
}

func (self *Config) ParseConfig() {
	// Setup overall defaults for all options
	self.Defaults()
	manager := new(plugs.PluginManager)

	execPath, _ := os.Executable()
	execPath = filepath.Dir(execPath)
	manager.HomeDir = execPath
	execPath = filepath.Join(execPath, "orgs.yaml")
	filename := execPath
	if _, err := os.Stat(filename); err != nil {
		filename, _ = filepath.Abs("orgc.yaml")
		if _, err = os.Stat(filename); err != nil {
			fmt.Printf("Looks like you do not have an orgs.yaml configuration file. Please add one!")
			os.Exit(-1)
		}
	}
	// Parse our config file next if present.
	self.Out.Infof("Loading: %s\n", filename)
	yamlFile, err := ioutil.ReadFile(filename)
	if err == nil {
		err = yaml.Unmarshal(yamlFile, self)
		if err != nil {
			err2 := fmt.Errorf("loading plugin: %s we experienced the following error during unmarshal: %s", filename, err)
			panic(err2)
		}
	}
	manager.Port = self.Port
	manager.TLSPort = self.TLSPort
	manager.HomeDir = filepath.Dir(execPath)
	manager.Out = GetLog()
	manager.Tempo = &templates.TemplateManager{TemplatePath: config.TemplatePath}
	manager.OrgDirs = self.OrgDirs
	manager.Tempo.Initialize()
	config.PlugManager = manager
	for _, pd := range self.Plugins {
		plugOpts := plugs.PluginOpts{}
		pd.Plugin.Startup(pd.Frequency, manager, &plugOpts)
	}
	for _, pd := range self.Exporters {
		plugOpts := plugs.PluginOpts{}
		pd.Plugin.Startup(manager, &plugOpts)
	}
	for _, pd := range self.Updaters {
		plugOpts := plugs.PluginOpts{}
		pd.Plugin.Startup(1, manager, &plugOpts)
	}
	// Command line overrides config file.
	self.ParseCommandLine()

	// Validate that all required parameters are present for
	// us to start up.
	self.Validate()
}

var config *Config

func Conf() *Config {
	if config == nil {
		config = new(Config)
		config.DefaultTodoStates = "TODO INPROGRESS IN-PROGRESS NEXT BLOCKED WAITING PHONE MEETING | DONE CANCELLED"
		config.ParseConfig()
	}
	return config
}

func Log() *logging.Logger {
	return Conf().Out
}
