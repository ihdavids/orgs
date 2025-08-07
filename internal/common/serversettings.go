package common

import (
	"crypto/sha1"
	b64 "encoding/base64"
	"fmt"
	"log"
	"math/rand"
	"time"
)

const KBAD_SALT = "THIS IS A DEFAULT SALT DO NOT USE THIS! SET YOUR OWN"

type ServerSettings struct {
	/* SDOC: Settings
	* Orgs Keys
		There are 2 keys that will be generated
		if not provided by the system.
		#+BEGIN_SRC yaml
	  orgJWS: "this key is used to sign the JWT"
	  orgJWE: "This key is used to encrypt the JWT"
	  orgSalt: "Appended to the per user salt to help with rainbow tables"
		#+END_SRC
		EDOC */
	OrgJWS  string `yaml:"orgJWS"`
	OrgJWE  string `yaml:"orgJWE"`
	OrgSalt string `yaml:"orgSalt"`

	/* SDOC: Settings
	* Orgs Keystore
		Orgs has various ways of storing credentials. You need something to protect your
		data. By default the yaml keystore is just a file with usernames and creds.
		Other keystores are possible.
		#+BEGIN_SRC yaml
	  keystore: "path to yaml file"
		#+END_SRC
		EDOC */
	Keystore string `yaml:"keystore"`

	// Configuration options
	ServePath string `yaml:"servepath"`
	Port      int    `yaml:"port"`
	TLSPort   int    `yaml:"tlsport"`
	ServerCrt string `yaml:"servercrt"`
	ServerKey string `yaml:"serverkey"`
	/* SDOC: Settings
	* Org Dirs
		A list of directories containing your org files
		#+BEGIN_SRC yaml
	  orgDirs:
	    - "/Users/me/dev/gtd"
		#+END_SRC
		EDOC */
	OrgDirs      []string `yaml:"orgDirs"`
	CanFailWatch bool     `yaml:"canWatchFail"`
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
	UseTagForProjects bool   `yaml:"useProjectTag"`
	AllowHttp         bool   `yaml:"allowHttp"`
	AllowHttps        bool   `yaml:"allowHttps"`
	DefaultTodoStates string `yaml:"defaultTodoStates"`
	TemplatePath      string `yaml:"templatePath"`
	DayPageTemplate   string `yaml:"dayPageTemplate"`
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
	DayPagePath          string      `yaml:"dayPagePath"`
	DayPageMode          string      `yaml:"dayPageMode"`
	DayPageModeWeekDay   string      `yaml:"dayPageModeWeekDay"`
	DayPageMaxSearchBack int         `yaml:"dayPageMaxSearch"`
	Plugins              []PluginDef `yaml:"plugins"`
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
	Exporters        []ExportDef       `yaml:"exporters"`
	Updaters         []UpdaterDef      `yaml:"updaters"`
	CaptureTemplates []CaptureTemplate `yaml:"captureTemplates"`
	AccessControl    string            `yaml:"accessControl"`
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
}

func (self *ServerSettings) Validate() {
	// You HAVE to have a orgdir
	if len(self.OrgDirs) < 1 {
		log.Default().Fatalln("Config file must specify orgDirs parameter!", self.OrgDirs)
	}
	// You have to have a salt of some kind defined
	if self.OrgSalt == KBAD_SALT {
		fmt.Printf("B")
		log.Default().Printf("BAD SALT!\n>> You NEED to set orgSalt in your config file!\n")
	}
}

const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*()"

func generateRandomString(length int) string {
	seededRand := rand.New(rand.NewSource(time.Now().UnixNano())) // Seed the random number generator
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[seededRand.Intn(len(charset))]
	}
	return string(b)
}

func (self *ServerSettings) RandomKeyVal() string {
	tHash := sha1.New()
	tHash.Write([]byte(generateRandomString(20)))
	return b64.StdEncoding.EncodeToString(tHash.Sum(nil))
}

func (self *ServerSettings) Init() {
	// Really you should not use these and should provide your own!
	// But at least these are cryptographically sound as a time based
	// randomly generated string that we sha1 hash and base 64 encode
	// to produce something that should work as our keyset
	self.OrgJWS = self.RandomKeyVal()
	self.OrgJWE = self.RandomKeyVal()
	// The default keystore is useless, we need to force the user to make one of their own
	self.Keystore = ""
	self.OrgSalt = KBAD_SALT
	self.ServePath = "/org"
	self.Port = 8010
	self.TLSPort = 443
	self.ServerCrt = "server.crt"
	self.ServerKey = "server.key"
	self.AllowHttp = false
	self.AllowHttps = true
	self.TemplatePath = "./templates"
	self.DayPageTemplate = "daypage.tpl"
	self.DayPagePath = "./daypages"
	self.DayPageMode = "week"
	self.DayPageModeWeekDay = "Monday"
	self.DayPageMaxSearchBack = 30 // How many weeks back should we look to pull last weeks tasks from.
	self.UseTagForProjects = true
	self.CaptureTemplates = []CaptureTemplate{}
	self.AccessControl = "null"
	self.RefileTargets = []string{".*\\.org"}
}
