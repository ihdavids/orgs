package plugs

import (
	"bytes"
	"fmt"
	"html"
	"html/template"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
	"github.com/ihdavids/orgs/internal/templates"
	"github.com/labstack/gommon/log"
	"github.com/zalando/go-keyring"
	"gopkg.in/AlecAivazis/survey.v1"
	"gopkg.in/op/go-logging.v1"
)

type BlockExecMethod func(*common.OrgFile, *org.Section, *org.Block) *common.ResultMsg
type PluginManager struct {
	HomeDir        string
	Out            *logging.Logger
	Tempo          *templates.TemplateManager
	BlockExec      map[string]BlockExecMethod
	Port           int
	TLSPort        int
	OrgDirs        []string
	cachedPassword map[string]string
}

func keyringGet(user string) (string, error) {
	password, err := keyring.Get("orgs", user)
	if err != nil {
		return password, err
	}
	return password, nil
}

func keyringSet(user, passwd string) error {
	return keyring.Set("orgs", user, passwd)
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

func (o *PluginManager) GetPassFromKeyring(name string) string {
	o.Out.Info("Querying keyring password source.")
	var err error
	o.cachedPassword[name], err = keyringGet(name)
	if err != nil {
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
			o.GetPassFromKeyring(name)
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
		log.Errorf("%s", err)
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
		log.Errorf("Failed to set password in keyring: %s", err)
		return err
	}
	return nil
}

type PluginOpts map[string]interface{}

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

func PlugExpandTemplatePath(name string) string {
	tempName, _ := filepath.Abs(name)
	tempFolderName, _ := filepath.Abs(path.Join("./templates", name))
	if _, err := os.Stat(name); err == nil {
		// Use name
	}
	if _, err := os.Stat(tempName); err == nil {
		// Try abs name of name
		name = tempName
	} else if _, err := os.Stat(tempFolderName); err == nil {
		// Try in the template folder for name
		name = tempFolderName
	}
	return name
}

type ODb interface {
	QueryTodosExpr(query string) (common.Todos, error)
	FindByAnyId(id string) *common.Todo
	FindByHash(hash string) *common.Todo
	FindNextSibling(hash string) *common.Todo
	FindPrevSibling(hash string) *common.Todo
	FindLastChild(hash string) *common.Todo
	FindByFile(filename string) *org.Document
	GetFile(filename string) *common.OrgFile
	GetFromTarget(target *common.Target, allowCreate bool) (*common.OrgFile, *org.Section)
	GetFromPreciseTarget(target *common.PreciseTarget, typeId org.NodeType) (*common.OrgFile, *org.Section, org.Node)
}

type PluginDef struct {
	Name      string
	Frequency int
	Plugin    Poller
	quit      chan struct{}
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

type PluginIdentifier struct {
	Name      string `yaml:"name"`
	Frequency int    `yaml:"freq"`
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

//////////////////////////////// EXPORTER /////////////////////////////////////////

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

// Exporter plugins are called only when requested
// and allowed to scan the database.
//
// Their purpose is to export one or more org files as desired.
type Exporter interface {
	Export(db ODb, query string, to string, opts string) error
	ExportToString(db ODb, query string, opts string) (error, string)
	Startup(manager *PluginManager, opts *PluginOpts)
	Unmarshal(unmarshal func(interface{}) error) error
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

func UserBodyScriptBlock() string {
	return "<!--USERBODYSCRIPT-->"
}
func UserHeaderScriptBlock() string {
	return "<!--USERHEADERSCRIPT-->"
}

func FileNameWithoutExt(fileName string) string {
	return fileName[:len(fileName)-len(filepath.Ext(fileName))]
}

func EscapeQuotes(str string) string {
	return html.EscapeString(strings.ReplaceAll(str, ",", ""))
}

func ReplaceQuotes(str string) string {
	return strings.ReplaceAll(str, "\"", "")
}

func HasP(td *common.Todo, name string) bool {
	if _, ok := td.Props[name]; ok {
		return true
	}
	return false
}

func ExpandTemplateIntoBuf(o *bytes.Buffer, temp string, m map[string]interface{}) {
	t := template.Must(template.New("").Parse(temp))
	t.Execute(o, m)
}

/////////////////// UPDATER /////////////////////////////

// Updating plugins are called with a target
//
// They can be used to update a specific node
// in an org file, or do something with it (like sync it with a DB)
type Updater interface {
	UpdateTarget(db ODb, target *common.Target, manager *PluginManager) (common.ResultMsg, error)
	Startup(freq int, manager *PluginManager, opts *PluginOpts)
	Unmarshal(unmarshal func(interface{}) error) error
}

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
