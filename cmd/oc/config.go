package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	_ "github.com/ihdavids/orgs/cmd/oc/commands/all"
)

type Config struct {
	Url             string               `yaml:"url"`
	ConfigedPlugins []commands.PluginDef `yaml:"plugins"`
}

func (self *Config) AddCommands() {
}

func (self *Config) FindCommand(name string) commands.Cmd {
	for _, p := range self.ConfigedPlugins {
		if p.Name == name {
			return p.Plugin
		}
	}
	return nil
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
	execPath, _ := os.Executable()
	execPath = filepath.Dir(execPath)
	execPath = filepath.Join(execPath, "oc.yaml")
	filename := execPath
	haveConfig := true
	if _, err := os.Stat(filename); err != nil {
		filename, _ = filepath.Abs("oc.yaml")
		if _, err = os.Stat(filename); err != nil {
			fmt.Printf("No oc.yaml config file found...\n")
			haveConfig = false
		}
	}
	if haveConfig {
		yamlFile, err := ioutil.ReadFile(filename)
		if err == nil {
			err = yaml.Unmarshal(yamlFile, self)
			if err != nil {
				panic(err)
			}
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
		config.ParseConfig()
	}
	return config
}
