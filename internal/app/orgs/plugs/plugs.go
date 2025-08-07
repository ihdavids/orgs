package plugs

import (
	"bytes"
	"html"
	"html/template"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/ihdavids/orgs/internal/common"
)

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
