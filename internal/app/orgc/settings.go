package orgc

import (
	"flag"
	"io/ioutil"
	"net/rpc"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Url       string            `yaml:"url"`
	TodoViews map[string]string `yaml:"todo_views"`
	// Dispatch commands
	FileList    bool
	TodoList    bool
	ProjectList bool
}

func (self *Config) AddCommands() {
	for key, val := range self.TodoViews {
		NewCommandTodo(key, &val)
	}
}

func (self *Config) Dispatch(c *rpc.Client) {
	/*
		if self.FileList {
			ShowFileList(c)
		}
		if self.TodoList {
			ShowAllTodos(c)
		}
		if self.ProjectList {
			ShowAllProjects(c)
		}
	*/
}

func (self *Config) Defaults() {
	self.Url = "ws://localhost:8010/org"
}

func (self *Config) Validate() {
}

func (self *Config) ParseCommandLine() {
	// NOTE: for this to work the default should always be the current
	//       value of the structure. Avoid using a default here
	//       instead specify it in Defaults up above.
	flag.StringVar(&self.Url, "url", self.Url, "how to connect to server")
	flag.BoolVar(&self.FileList, "filelist", self.FileList, "Query for the full list of files")
	flag.BoolVar(&self.TodoList, "todolist", self.TodoList, "List all todos in all org files")
	flag.BoolVar(&self.ProjectList, "projectlist", self.ProjectList, "List all projects in all org files")
	flag.Parse()
}

func (self *Config) ParseConfig() {
	// Setup overall defaults for all options
	self.Defaults()

	// Parse our config file next if present.
	filename, _ := filepath.Abs("orgc.yaml")
	//log.Println("Loading: ", filename)
	yamlFile, err := ioutil.ReadFile(filename)
	if err == nil {
		err = yaml.Unmarshal(yamlFile, self)
		if err != nil {
			panic(err)
		}
	}
	// Command line overrides config file.
	self.ParseCommandLine()

	// Validate that all required parameters are present for
	// us to start up.
	self.Validate()
	self.AddCommands()
}

var config *Config

func Conf() *Config {
	if config == nil {
		config = new(Config)
		config.TodoViews = make(map[string]string)
		config.ParseConfig()
	}
	return config
}
