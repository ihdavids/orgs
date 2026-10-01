package plugs

import (
	"bytes"
	"fmt"
	"html"
	"html/template"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/ihdavids/go-org/org"
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

// MediaURL turns the target of an org `file:` link into something a browser can
// fetch, and is the one copy of that sum.
//
// There were four of them before this: the html exporter had the careful one
// (resolve against the file that names it, fall back to the org root, work out
// the path under the file server, hand back a file:// url for anything outside
// it), and the reveal.js and impress.js exporters each had a version that
// assumed the link was relative to the *first org directory* and that the
// reader was on the same machine as the server. That is wrong for a file in a
// subdirectory - every picture in a deck under notes/talks/ pointed at the
// wrong place - and wrong for a phone looking at the server over the network.
//
// Three answers, picked by the opts string the exporter was called with, which
// is how the vscode plugin and an exported-to-disk page ask for what they need:
//
//   - `filelinks;` - an absolute `file://` url, for a page opened off disk;
//   - `httpslinks;`/`httplinks;` - this server by name, for a webview that
//     cannot do relative urls;
//   - anything else - a **path** under /images, which is the one that works
//     wherever the browser reached this server from.
func MediaURL(doc *org.Document, pm *common.PluginManager, opts, target string) string {
	if target == "" {
		return ""
	}
	target = strings.TrimPrefix(target, "file://")
	target = strings.TrimPrefix(target, "file:")
	// Anything with a protocol of its own is already fetchable.
	if strings.Contains(target, "://") || strings.HasPrefix(target, "//") || strings.HasPrefix(target, "data:") {
		return target
	}

	root := ""
	if pm != nil && len(pm.OrgDirs) > 0 {
		root, _ = filepath.Abs(pm.OrgDirs[0])
	}

	// Resolve against the file that names it, and fall back to the org root.
	abs := ""
	switch {
	case filepath.IsAbs(target):
		abs = filepath.FromSlash(target)
	case doc != nil && doc.Path != "":
		abs, _ = filepath.Abs(filepath.Join(filepath.Dir(doc.Path), filepath.FromSlash(target)))
		if _, err := os.Stat(abs); err != nil && root != "" {
			if alt, aerr := filepath.Abs(filepath.Join(root, filepath.FromSlash(target))); aerr == nil {
				if _, err := os.Stat(alt); err == nil {
					abs = alt
				}
			}
		}
	case root != "":
		abs, _ = filepath.Abs(filepath.Join(root, filepath.FromSlash(target)))
	}
	if abs == "" {
		return target
	}

	if strings.Contains(opts, "filelinks;") {
		return "file://" + filepath.ToSlash(abs)
	}

	// Under the file server, which is rooted at the first org directory. A file
	// outside it cannot be served, and is offered as a file:// link rather than
	// as a url that would answer 404.
	rel := ""
	if root != "" {
		if r, err := filepath.Rel(root, abs); err == nil && !strings.HasPrefix(r, "..") {
			rel = filepath.ToSlash(r)
		}
	}
	if rel == "" {
		return "file://" + filepath.ToSlash(abs)
	}
	if strings.Contains(opts, "httpslinks;") && pm != nil {
		return fmt.Sprintf("https://localhost:%d/images/%s", pm.TLSPort, rel)
	}
	if strings.Contains(opts, "httplinks;") && pm != nil {
		return fmt.Sprintf("http://localhost:%d/images/%s", pm.Port, rel)
	}
	return "/images/" + rel
}
