package new

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
	"github.com/koki-develop/go-fzf"
)

type NewFile struct {
	Name     string
	Template string
}

func (self *NewFile) Unmarshal(unmarshal func(interface{}) error) error {
	return unmarshal(self)
}

func (self *NewFile) StartPlugin(manager *common.PluginManager) {
}

func (self *NewFile) SetupParameters(fset *flag.FlagSet) {
	fset.StringVar(&self.Name, "name", "", "filename for the new org file (without .org extension)")
	fset.StringVar(&self.Template, "temp", "", "template name to use")
}

func (self *NewFile) Exec(core *commands.Core) {
	// Get available templates from the server
	var qry map[string]string = map[string]string{}
	var templates common.FileList = common.FileList{}
	commands.SendReceiveGet(core, "newtemplates", qry, &templates)
	if len(templates) == 0 {
		fmt.Println("No new-file templates found in templates/new/")
		return
	}
	sort.Strings(templates)

	// Use fzf to pick a template
	selectedTemplate := self.Template
	if selectedTemplate == "" {
		f, err := fzf.New(
			fzf.WithPrompt("Select template> "),
			fzf.WithCountViewEnabled(true),
			fzf.WithCountView(func(meta fzf.CountViewMeta) string {
				return fmt.Sprintf("templates: %d", meta.ItemsCount)
			}),
		)
		if err != nil {
			log.Fatal(err)
		}
		idxs, err := f.Find(templates, func(i int) string { return templates[i] })
		if err != nil {
			log.Fatal(err)
		}
		if len(idxs) == 0 {
			fmt.Println("No template selected")
			return
		}
		selectedTemplate = templates[idxs[0]]
	}

	// Get available directories from the server
	var dirs common.FileList = common.FileList{}
	commands.SendReceiveGet(core, "dirs", qry, &dirs)
	if len(dirs) == 0 {
		fmt.Println("No directories available from server")
		return
	}
	sort.Strings(dirs)

	// Use fzf to pick a directory
	f, err := fzf.New(
		fzf.WithPrompt("Select directory> "),
		fzf.WithCountViewEnabled(true),
		fzf.WithCountView(func(meta fzf.CountViewMeta) string {
			return fmt.Sprintf("dirs: %d", meta.ItemsCount)
		}),
	)
	if err != nil {
		log.Fatal(err)
	}
	idxs, err := f.Find(dirs, func(i int) string { return dirs[i] })
	if err != nil {
		log.Fatal(err)
	}
	if len(idxs) == 0 {
		fmt.Println("No directory selected")
		return
	}
	dir := dirs[idxs[0]]

	// Prompt for filename
	filename := self.Name
	if filename == "" {
		reader := bufio.NewReader(os.Stdin)
		fmt.Print("Enter filename (without .org extension): ")
		filename, err = reader.ReadString('\n')
		if err != nil {
			log.Fatal(err)
		}
		filename = strings.TrimSpace(filename)
	}
	if filename == "" {
		fmt.Println("No filename provided")
		return
	}

	// Ensure .org extension
	if !strings.HasSuffix(filename, ".org") {
		filename = filename + ".org"
	}

	fullPath := filepath.Join(dir, filename)
	title := strings.TrimSuffix(filename, ".org")

	// Create the file on the server
	req := common.NewFileRequest{
		Filename: fullPath,
		Title:    title,
		Template: selectedTemplate,
	}
	var reply common.FileList
	commands.SendReceivePost(core, "file", &req, &reply)

	if len(reply) > 0 {
		fmt.Printf("Created: %s\n", reply[0])
		core.LaunchEditor(reply[0], 0)
	} else {
		fmt.Println("Failed to create file")
	}
}

// init function is called at boot
func init() {
	commands.AddCmd("new", "create a new org file from template",
		func() commands.Cmd {
			return &NewFile{}
		})
}
