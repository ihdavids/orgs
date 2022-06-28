package orgs

import (
	"flag"
	"io/ioutil"
	"log"
	"path/filepath"

	"gopkg.in/yaml.v2"
)

type Config struct {
	ServePath          string   `yaml:"servepath"`
	Port               int      `yaml:"port"`
	OrgDirs            []string `yaml:"orgDirs"`
	UseTagForProjects  bool     `yaml:"useProjectTag"`
	DefaultTodoStates  string   `yaml:"defaultTodoStates"`
	TemplatePath       string   `yaml:"templatePath"`
	DayPageTemplate    string   `yaml:"dayPageTemplate"`
	DayPagePath        string   `yaml:"dayPagePath"`
	DayPageMode        string   `yaml:"dayPageMode"`
	DayPageModeWeekDay string   `yaml:"dayPageModeWeekDay"`
}

func (self *Config) Defaults() {
	self.ServePath = "/org"
	self.Port = 8010
	self.TemplatePath = "./templates"
	self.DayPageTemplate = "daypage.tpl"
	self.DayPagePath = "./daypages"
	self.DayPageMode = "week"
	self.DayPageModeWeekDay = "Monday"
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
	flag.Parse()
}

func (self *Config) ParseConfig() {
	// Setup overall defaults for all options
	self.Defaults()

	// Parse our config file next if present.
	filename, _ := filepath.Abs("orgs.yaml")
	log.Println("Loading: ", filename)
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
}

var config *Config

func Conf() *Config {
	if config == nil {
		config = new(Config)
		config.DefaultTodoStates = "TODO | DONE"
		config.ParseConfig()
	}
	return config
}
