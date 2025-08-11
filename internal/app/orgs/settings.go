//lint:file-ignore ST1006 allow the use of self
package orgs

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/ihdavids/orgs/internal/common"
	"github.com/ihdavids/orgs/internal/templates"
	"gopkg.in/op/go-logging.v1"
	"gopkg.in/yaml.v2"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	_ "github.com/ihdavids/orgs/cmd/oc/commands/all"
)

type Config struct {
	// Core systems
	PlugManager *common.PluginManager
	Out         *logging.Logger
	Config      string
	HomeDir     string
	Server      *common.ServerSettings `yaml:"server"`
	Author      string                 `yaml:"author"`
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

	// NON SERVE CONFIGURATION OPTIONS:
	Url            string `yaml:"url"`
	EditorTemplate []string
	// When a command is called we configure it and add it
	// to the list to avoid having to redo that
	ConfigedCommands []commands.PluginDef
}

func (self *Config) Defaults() {
	self.Server.Init()
	// That said, you SHOULD NOT USE THIS!
	self.Out = logging.MustGetLogger("orgs")
	self.Author = ""
	self.NewFileTemplate = "newfile.tpl"
	self.ArchiveDefaultTarget = "%s_archive::"
	self.ArchiveSaveContextInfo = []string{"time", "file", "ltags", "itags", "todo", "category", "olpath"}
	self.ArchiveSkipEmptyProperties = true

	self.DateTreeYearFormat = "2006"
	self.DateTreeMonthFormat = "January"
	self.DateTreeDayFormat = "02 Monday"
	self.ClockIntoDrawer = "LOGBOOK"
	self.TemplateImagesPath = "./templates/html_styles/images"
	self.TemplateFontPath = "./templates/fonts"

	// Non Serve options:
	self.Url = "http://localhost:8010"
	self.EditorTemplate = []string{"code", "-g", "{filename}:{linenum}"}

	execPath, _ := os.Executable()
	execPath = filepath.Dir(execPath)
	self.HomeDir = execPath
	self.Config = filepath.Join(execPath, "orgs.yaml")
}

func (self *Config) Validate() {
}

func Usage() {
	flag.PrintDefaults()
	fmt.Printf("  Commands:\n")
	for name, val := range commands.CmdRegistry {
		fmt.Printf("   %-15s\t%s\n", name, val.Usage)
	}
}

func (self *Config) AddCommands() {
	//fmt.Printf("Add Commands\n")
	for name, val := range commands.CmdRegistry {
		//fmt.Printf("NAME: %s\n", name)
		op := flag.NewFlagSet(name, flag.ExitOnError)
		val.Flags = op
		val.Cmd.SetupParameters(op)
	}
	flag.Usage = Usage
}

func (self *Config) FindCommand(name string) commands.Cmd {
	for _, p := range self.ConfigedCommands {
		if p.Name == name {
			return p.Plugin
		}
	}
	return nil
}

func (self *Config) SetupCommandLine() {
	flag.StringVar(&self.Config, "config", self.Config, "config file")

	flag.StringVar(&self.Url, "url", self.Url, "server url for non serve mode")
	self.AddCommands()

}

func (self *Config) ParseConfig() {
	// Setup overall defaults for all options
	self.Defaults()
	manager := new(common.PluginManager)
	manager.HomeDir = self.HomeDir
	manager.Plugs = common.PluginLookup(self)

	self.SetupCommandLine()
	// Parse to pull config file from command line first
	flag.Parse()
	filename := self.Config
	if _, err := os.Stat(filename); err != nil {
		// I should really remove this crap!
		filename, _ := filepath.Abs("orgc.yaml")
		if _, err = os.Stat(filename); err != nil {
			fmt.Printf("Looks like you do not have a [%s] configuration file. Please add one!", self.Config)
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
	manager.HomeDir = filepath.Dir(self.HomeDir)
	manager.Out = GetLog()
	if self.Server != nil {
		manager.Tempo = &templates.TemplateManager{TemplatePath: config.Server.TemplatePath}
		manager.Tempo.Initialize()
	}
	if self.Server != nil {
		manager.Port = self.Server.Port
		manager.TLSPort = self.Server.TLSPort
		manager.OrgDirs = self.Server.OrgDirs
	}
	config.PlugManager = manager
	if self.Server != nil {
		for _, pd := range self.Server.Plugins {
			plugOpts := common.PluginOpts{}
			pd.Plugin.Startup(pd.Frequency, manager, &plugOpts)
		}
		for _, pd := range self.Server.Exporters {
			plugOpts := common.PluginOpts{}
			pd.Plugin.Startup(manager, &plugOpts)
		}
		for _, pd := range self.Server.Updaters {
			plugOpts := common.PluginOpts{}
			pd.Plugin.Startup(1, manager, &plugOpts)
		}
	}
	// Command line overrides config file.
	// down here. This dual parse facilitates the command line
	// override of the yaml file.
	flag.Parse()

	// Validate that all required parameters are present for
	// us to start up.
	self.Validate()
}

func (self *Config) GetExporter(name string) common.Exporter {
	if self.Server != nil {
		for _, e := range self.Server.Exporters {
			if name == e.Name {
				return e.Plugin
			}
		}
	}
	return nil
}

func (self *Config) GetPoller(name string) common.Poller {
	if self.Server != nil {
		for _, e := range self.Server.Plugins {
			if name == e.Name {
				return e.Plugin
			}
		}
	}
	return nil
}

func (self *Config) GetUpdater(name string) common.Updater {
	if self.Server != nil {
		for _, e := range self.Server.Updaters {
			if name == e.Name {
				return e.Plugin
			}
		}
	}
	return nil
}

var config *Config

func Conf() *Config {
	if config == nil {
		config = new(Config)
		config.Server = &common.ServerSettings{}
		config.Server.DefaultTodoStates = "TODO INPROGRESS IN-PROGRESS NEXT BLOCKED WAITING PHONE MEETING | DONE CANCELLED"
		config.ParseConfig()
	}
	return config
}

func Log() *logging.Logger {
	return Conf().Out
}
