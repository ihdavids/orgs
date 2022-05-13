package orgc

import (
	"flag"
	"io/ioutil"
	"log"
	"path/filepath"

	"gopkg.in/yaml.v2"
)

type Config struct {
	Url string `yaml:"url"`
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
	flag.Parse()
}

func (self *Config) ParseConfig() {
	// Setup overall defaults for all options
	self.Defaults()

	// Parse our config file next if present.
	filename, _ := filepath.Abs("orgc.yaml")
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
		config.ParseConfig()
	}
	return config
}
