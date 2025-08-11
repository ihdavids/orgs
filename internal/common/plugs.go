package common

import (
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/templates"
	"github.com/tmc/keyring"
	"gopkg.in/AlecAivazis/survey.v1"
	"gopkg.in/op/go-logging.v1"
)

func keyringGet(user string) (string, error) {
	fmt.Printf("KEYRING GET CALLED: %s\n", user)
	password, err := keyring.Get("orgs", user)
	if err != nil {
		return password, err
	}
	return password, nil
}

func keyringSet(user, passwd string) error {
	return keyring.Set("orgs", user, passwd)
}

type PluginLookup interface {
	GetExporter(name string) Exporter
	GetPoller(name string) Poller
	GetUpdater(name string) Updater
}

type BlockExecMethod func(*OrgFile, *org.Section, *org.Block) *ResultMsg
type PluginManager struct {
	HomeDir        string
	Out            *logging.Logger
	Tempo          *templates.TemplateManager
	BlockExec      map[string]BlockExecMethod
	Port           int
	TLSPort        int
	OrgDirs        []string
	cachedPassword map[string]string
	Plugs          PluginLookup
}

// Updating plugins are called with a target
//
// They can be used to update a specific node
// in an org file, or do something with it (like sync it with a DB)
type Updater interface {
	UpdateTarget(db ODb, target *Target, manager *PluginManager) (ResultMsg, error)
	Startup(freq int, manager *PluginManager, opts *PluginOpts)
	Unmarshal(unmarshal func(interface{}) error) error
}

func (o *PluginManager) AddBlockMethod(name string, method BlockExecMethod) {
	if o.BlockExec == nil {
		o.BlockExec = make(map[string]BlockExecMethod)
	}
	o.BlockExec[name] = method
}

func (o *PluginManager) GetPassFromStdIn(name string) string {
	o.Out.Info("Reading password from stdin.")
	allBytes, err := ioutil.ReadAll(os.Stdin)
	if err != nil {
		panic(fmt.Sprintf("unable to read bytes from stdin: %s", err))
	}
	o.cachedPassword[name] = string(allBytes)
	return o.cachedPassword[name]
}

func (o *PluginManager) GetPassFromKeyring(name string, nonfatal bool) string {
	o.Out.Info("Querying keyring password source.")
	var err error
	o.cachedPassword[name], err = keyringGet(name)
	if err != nil {
		if nonfatal {
			return ""
		}
		panic(err)
	}
	return o.cachedPassword[name]
}

func (o *PluginManager) GetPass(name string, passSource string) string {
	if o.cachedPassword == nil {
		o.cachedPassword = map[string]string{}
	}
	if pass, ok := o.cachedPassword[name]; ok && pass != "" {
		return pass
	}
	o.Out.Debugf("Getting Password")
	if passSource == "" {
		passSource = "keyring"
	}
	if passSource != "" {
		o.Out.Debugf("password-source: %s", passSource)
		if passSource == "keyring" {
			o.GetPassFromKeyring(name, false)
		} else if passSource == "keyring-nonfatal" {
			if o.GetPassFromKeyring(name, true) == "" {
				return ""
			}
		} else if passSource == "stdin" {
			o.GetPassFromStdIn(name)
		} else {
			o.Out.Warningf("Unknown password-source: %s", passSource)
		}
	}

	if o.cachedPassword[name] != "" {
		o.Out.Info("Password cached.")
		return o.cachedPassword[name]
	}

	prompt := fmt.Sprintf("Password Prompt [%s]: ", name)
	help := ""

	pass := ""
	err := survey.AskOne(
		&survey.Password{
			Message: prompt,
			Help:    help,
		},
		&pass,
		nil,
	)
	if err != nil {
		fmt.Errorf("%s", err.Error())
		panic("Failed to aquire password... ABORT!")
	}
	o.cachedPassword[name] = pass
	if passSource == "keyring" {
		o.SetPass(name, pass)
	}
	return o.cachedPassword[name]
}

func (o *PluginManager) SetPass(name string, passwd string) error {
	// dont reset password to empty string
	if name == "" || passwd == "" {
		return nil
	}

	// save password in keychain so that it can be used for subsequent http requests
	err := keyringSet(name, passwd)
	if err != nil {
		fmt.Errorf("Failed to set password in keyring: %s", err.Error())
		return err
	}
	return nil
}

// Polling plugins are called periodically
// and allowed to scan the database.
//
// Their purpose is to do things like handle
// notifications or try to sync with external
// data sources.
type Poller interface {
	Update(db ODb)
	Startup(freq int, manager *PluginManager, opts *PluginOpts)
	Unmarshal(unmarshal func(interface{}) error) error
}

type ODb interface {
	QueryTodosExpr(query string) (Todos, error)
	FindByAnyId(id string) *Todo
	FindByHash(hash string) *Todo
	FindNextSibling(hash string) *Todo
	FindPrevSibling(hash string) *Todo
	FindLastChild(hash string) *Todo
	FindByFile(filename string) *org.Document
	GetFile(filename string) *OrgFile
	GetFromTarget(target *Target, allowCreate bool) (*OrgFile, *org.Section)
	GetFromPreciseTarget(target *PreciseTarget, typeId org.NodeType) (*OrgFile, *org.Section, org.Node)
}

type PluginOpts map[string]interface{}

//////////////////////////////// EXPORTER /////////////////////////////////////////

// Exporter plugins are called only when requested
// and allowed to scan the database.
//
// Their purpose is to export one or more org files as desired.
type Exporter interface {
	Export(db ODb, query string, to string, opts string, props map[string]string) error
	ExportToString(db ODb, query string, opts string, props map[string]string) (error, string)
	Startup(manager *PluginManager, opts *PluginOpts)
	Unmarshal(unmarshal func(interface{}) error) error
}

type ExportDef struct {
	Name   string
	Plugin Exporter
}

type ExportIdentifier struct {
	Name string `yaml:"name"`
}

func (self *ExportDef) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var id = ExportIdentifier{""}
	res := unmarshal(&id)
	if res != nil {
		return res
	}
	if creator, ok := ExporterRegistry[id.Name]; ok {
		self.Plugin = creator()
		self.Name = id.Name
		return self.Plugin.Unmarshal(unmarshal)
	}
	return fmt.Errorf("Failed to create exporter %s", id.Name)
}

// The exporter registry has the definitions of all known exporters
type ExportCreator func() Exporter

var ExporterRegistry = map[string]ExportCreator{}

func AddExporter(name string, creator ExportCreator) {
	//fmt.Printf("ADDING EXPORTER: %s\n", name)
	ExporterRegistry[name] = creator
}

func FindExporter(name string) ExportCreator {
	if v, ok := ExporterRegistry[name]; ok {
		return v
	}
	return nil
}

// The poller registry has the definitions of all known pollers
type PollerCreator func() Poller

var PollerRegistry = map[string]PollerCreator{}

func AddPoller(name string, creator PollerCreator) {
	//fmt.Printf("ADDING PLUGIN: %s\n", name)
	PollerRegistry[name] = creator
}

func Find(name string) PollerCreator {
	if v, ok := PollerRegistry[name]; ok {
		return v
	}
	return nil
}

type PluginIdentifier struct {
	Name      string `yaml:"name"`
	Frequency int    `yaml:"freq"`
}

type PluginDef struct {
	Name      string
	Frequency int
	Plugin    Poller
	quit      chan struct{}
}

func (self *PluginDef) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var id = PluginIdentifier{"", 0}
	res := unmarshal(&id)
	if res != nil {
		return res
	}
	if creator, ok := PollerRegistry[id.Name]; ok {
		self.Plugin = creator()
		self.Name = id.Name
		self.Frequency = id.Frequency
		// Default is every 60 seconds
		if self.Frequency <= 0 {
			self.Frequency = 60
		}
		return self.Plugin.Unmarshal(unmarshal)
	}
	return fmt.Errorf("Failed to create plugin %s", id.Name)
}

func (self *PluginDef) Start(db ODb) {
	if self.Plugin == nil {
		return
	}
	ticker := time.NewTicker(time.Duration(self.Frequency) * time.Second)
	self.quit = make(chan struct{})
	go func(plug PluginDef, name string, t *time.Ticker, frequency int) {
		p := plug
		fmt.Printf("PLUGIN START: %s %d %v\n", name, frequency, time.Now())
		for {
			select {
			case <-p.quit:
				fmt.Printf("PLUGIN STOP:  %s %d\n", name, frequency)
				t.Stop()
				return
			case <-t.C:
				fmt.Printf("UPDATE [%s:%d]: <%v>\n", name, frequency, time.Now())
				p.Plugin.Update(db)
			}
		}
	}(*self, self.Name, ticker, self.Frequency)
}

func (self *PluginDef) Stop() {
	if self.quit != nil {
		close(self.quit)
	}
}

/////////////////// UPDATER /////////////////////////////

type UpdaterDef struct {
	Name   string
	Plugin Updater
}

type UpdaterCreator func() Updater

var UpdaterRegistry = map[string]UpdaterCreator{}

type UpdaterIdentifier struct {
	Name string `yaml:"name"`
}

func (self *UpdaterDef) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var id = UpdaterIdentifier{""}
	res := unmarshal(&id)
	if res != nil {
		return res
	}
	if creator, ok := UpdaterRegistry[id.Name]; ok {
		self.Plugin = creator()
		self.Name = id.Name
		return self.Plugin.Unmarshal(unmarshal)
	}
	return fmt.Errorf("Failed to create updater %s", id.Name)
}

func FindUpdater(name string) UpdaterCreator {
	if v, ok := UpdaterRegistry[name]; ok {
		return v
	}
	return nil
}

func AddUpdater(name string, creator UpdaterCreator) {
	//fmt.Printf("ADDING PLUGIN: %s\n", name)
	UpdaterRegistry[name] = creator
}
