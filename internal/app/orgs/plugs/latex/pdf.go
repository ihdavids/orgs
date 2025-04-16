package latex

import (
	"fmt"
	//"html"
	//"io/ioutil"
	"log"
	//"os"
	//"reflect"
	//"regexp"
	//"strconv"
	"strings"
	//"unicode"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/app/orgs/plugs"
	"gopkg.in/op/go-logging.v1"
	//"gopkg.in/yaml.v3"
	//"github.com/flosch/pongo2/v5"
)

type OrgPdfExporter struct {
	TemplatePath string
	Props        map[string]any
	out          *logging.Logger
	pm           *plugs.PluginManager
}

type OrgPdfWriter struct {
	ExtendingWriter org.Writer
	strings.Builder
	Document            *org.Document
	log                 *log.Logger
	footnotes           *footnotes
	PrettyRelativeLinks bool
	envs                EnvironmentStack
	docclass            string
	templateRegistry    *SubTemplates
	exporter            *OrgLatexExporter
}

func NewOrgPdfWriter(exp *OrgPdfExporter) *OrgPdfWriter {
	/*
	defaultConfig := org.New()
	return &OrgLatexWriter{
		Document: &org.Document{Configuration: defaultConfig},
		footnotes: &footnotes{
			mapping: map[string]int{},
		},
		exporter: exp,
	}
	*/
	return nil
}

func (self *OrgPdfExporter) Unmarshal(unmarshal func(interface{}) error) error {
	return unmarshal(self)
}

func (self *OrgPdfExporter) Export(db plugs.ODb, query string, to string, opts string, props map[string]string) error {
	fmt.Printf("PDF: Export called", query, to, opts)
	/*
	for _, exp := range Conf().Exporters {
		if exp.Name == "latex" {
			err := exp.Plugin.Export(db, query, to, opts, props)
			break
		}
	}*/
	//err, str := self.ExportToString(db, query, opts, props)
	//if err != nil {
	//	return err
	//}
	//return os.WriteFile(to, []byte(str), 0644)

	/*
		_, err := db.QueryTodosExpr(query)
		if err != nil {
			msg := fmt.Sprintf("ERROR: latex failed to query expression, %v [%s]\n", err, query)
			log.Printf(msg)
			return fmt.Errorf(msg)
		}
	*/
	return nil
}
// ----------- [ Exporter System ] -----------------------

func (self *OrgPdfExporter) ExportToString(db plugs.ODb, query string, opts string, props map[string]string) (error, string) {
	fmt.Printf("PDF: Export string called [%s]:[%s]\n", query, opts)
	return nil, ""
}

func (self *OrgPdfExporter) Startup(manager *plugs.PluginManager, opts *plugs.PluginOpts) {
	self.out = manager.Out
	self.pm  = manager
}

// init function is called at boot
func init() {
	plugs.AddExporter("pdf", func() plugs.Exporter {
		return &OrgPdfExporter{Props: map[string]interface{}{}, TemplatePath: "latex_default.tpl"}
	})
}
