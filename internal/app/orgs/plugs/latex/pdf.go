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
	"os"
	"os/exec"
)

type OrgPdfExporter struct {
	TemplatePath string
	Props        map[string]any
	PdfLatex     string
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
	fmt.Printf("PDF: Export called: [%s] [%s]\n%v", query, to, opts)
	exp := self.pm.Plugs.GetExporter("latex")
	if exp == nil {
		self.pm.Out.Error("Failed to find latex exporter for pdf conversion!")
		return fmt.Errorf("Failed to find latex exporter, has it been initialized?")
	}
	if err, res := exp.ExportToString(db, query, opts, props); err == nil {
		if tmp, e2 := os.CreateTemp("", "latexcache-*.tex"); e2 == nil {
			defer os.Remove(tmp.Name())
			{
				defer tmp.Close()
				tmp.Write([]byte(res))
			}
			tmp.Name()
			pdflatex := self.PdfLatex
			if pdflatex == "" {
				pdflatex = "/Library/TeX/texbin/pdflatex"
			}
			args := []string{"--shell-escape", tmp.Name(), "-output-format=pdf", fmt.Sprintf("-o=%s", to)}
			cmd := exec.Command(pdflatex, args...)
			//cmd := exec.Command(pdflatex, fmt.Sprintf("--shell-escape %s -output-format=pdf -o=%s", tmp.Name(), to))

			if out, e3 := cmd.Output(); e3 == nil {
				self.pm.Out.Info("%s\n", string(out))
				self.pm.Out.Info("CONVERSION FINISHED\n")
			} else {
				fmt.Printf("ERROR PDF Export: %v\n%v\n%v\n", cmd.Args, e3, string(out))
			}
		} else {
			fmt.Printf("Temp file not created... %v\n", e2)
		}

		//fmt.Printf("RES: %s\n", res)
	} else {
		self.pm.Out.Error("Latex Conversion Error: %v", err)
	}
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
	self.pm = manager
}

// init function is called at boot
func init() {
	plugs.AddExporter("pdf", func() plugs.Exporter {
		return &OrgPdfExporter{Props: map[string]interface{}{}, TemplatePath: "latex_default.tpl"}
	})
}
