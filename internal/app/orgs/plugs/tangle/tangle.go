//lint:file-ignore ST1006 allow the use of self
package tangle

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

// tangleBlock holds a single source block's content and its tangle parameters.
type tangleBlock struct {
	lang       string
	content    string
	tangle     string // output file path
	mkdirp     bool
	padline    bool
	shebang    string
	tangleMode os.FileMode
	noweb      bool
	nowebRef   string
	nowebSep   string
	comments   string // none, link, both, org
}

// OrgTangleExporter implements the Exporter interface for tangling org files.
type OrgTangleExporter struct {
	pm *common.PluginManager
}

func (self *OrgTangleExporter) Startup(manager *common.PluginManager, opts *common.PluginOpts) {
	self.pm = manager
}

func (self *OrgTangleExporter) Unmarshal(unmarshal func(interface{}) error) error {
	return nil
}

// blockContent extracts the raw text content from a Block's children.
func blockContent(b *org.Block) string {
	var sb strings.Builder
	for _, child := range b.Children {
		if t, ok := child.(org.Text); ok {
			sb.WriteString(t.Content)
			if !strings.HasSuffix(t.Content, "\n") {
				sb.WriteString("\n")
			}
		}
	}
	return sb.String()
}

// commentPrefix returns the line comment prefix for a given language.
func commentPrefix(lang string) string {
	switch strings.ToLower(lang) {
	case "python", "ruby", "perl", "bash", "sh", "zsh", "r", "julia", "yaml", "toml", "conf", "makefile":
		return "# "
	case "c", "cpp", "java", "javascript", "js", "typescript", "ts", "go", "rust", "swift", "kotlin", "scala", "css", "dart", "groovy", "php":
		return "// "
	case "lisp", "elisp", "emacs-lisp", "scheme", "clojure", "racket":
		return ";; "
	case "haskell", "lua", "sql":
		return "-- "
	case "html", "xml":
		return "" // no single-line comment in markup
	default:
		return "# "
	}
}

// parseOctalMode parses a tangle-mode value like "(identity #o755)" or "0644" into an os.FileMode.
func parseOctalMode(s string) (os.FileMode, bool) {
	s = strings.TrimSpace(s)
	// Handle Emacs Lisp style: (identity #o755)
	re := regexp.MustCompile(`#o(\d+)`)
	if m := re.FindStringSubmatch(s); m != nil {
		s = m[1]
	}
	// Handle plain octal: 0755 or 755
	s = strings.TrimPrefix(s, "0")
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseUint(s, 8, 32)
	if err != nil {
		return 0, false
	}
	return os.FileMode(v), true
}

// parseParams extracts tangle-related parameters from a block's ParameterMap,
// falling back to document-level defaults.
func parseParams(params map[string]string, docDefaults map[string]string) tangleBlock {
	tb := tangleBlock{
		padline:    true,
		tangleMode: 0644,
		nowebSep:   "\n",
		comments:   "none",
	}

	// Merge document defaults first, then block-level overrides
	merged := map[string]string{}
	for k, v := range docDefaults {
		merged[k] = v
	}
	for k, v := range params {
		merged[k] = v
	}

	if v, ok := merged[":lang"]; ok {
		tb.lang = v
	}
	if v, ok := merged[":tangle"]; ok {
		tb.tangle = v
	}
	if v, ok := merged[":mkdirp"]; ok {
		tb.mkdirp = strings.ToLower(v) == "yes"
	}
	if v, ok := merged[":padline"]; ok {
		tb.padline = strings.ToLower(v) != "no"
	}
	if v, ok := merged[":shebang"]; ok {
		tb.shebang = v
	}
	if v, ok := merged[":tangle-mode"]; ok {
		if mode, valid := parseOctalMode(v); valid {
			tb.tangleMode = mode
		}
	}
	if v, ok := merged[":noweb"]; ok {
		tb.noweb = strings.ToLower(v) == "yes"
	}
	if v, ok := merged[":noweb-ref"]; ok {
		tb.nowebRef = v
	}
	if v, ok := merged[":noweb-sep"]; ok {
		tb.nowebSep = v
	}
	if v, ok := merged[":comments"]; ok {
		tb.comments = strings.ToLower(v)
	}
	return tb
}

var nowebRefRegexp = regexp.MustCompile(`<<([^>]+)>>`)

// expandNoweb replaces <<name>> references in content with the corresponding named block content.
func expandNoweb(content string, nowebRefs map[string][]string, sep string, depth int) string {
	if depth > 20 {
		return content
	}
	return nowebRefRegexp.ReplaceAllStringFunc(content, func(match string) string {
		name := nowebRefRegexp.FindStringSubmatch(match)[1]
		if parts, ok := nowebRefs[name]; ok {
			joined := strings.Join(parts, sep)
			// Recursively expand in case referenced blocks also contain <<refs>>
			return expandNoweb(joined, nowebRefs, sep, depth+1)
		}
		return match // leave unresolved references as-is
	})
}

// collectBlocks walks the document nodes and collects all SRC blocks.
func collectBlocks(nodes []org.Node) []*org.Block {
	var blocks []*org.Block
	for _, node := range nodes {
		switch n := node.(type) {
		case org.Block:
			if strings.ToUpper(n.Name) == "SRC" {
				blocks = append(blocks, &n)
			}
		case *org.Headline:
			// Blocks are indexed on the headline
			blocks = append(blocks, n.Blocks...)
			// Also walk headline children for any top-level blocks
			blocks = append(blocks, collectBlocks(n.Children)...)
		}
	}
	return blocks
}

// getDocHeaderArgs parses document-level #+PROPERTY: header-args settings.
func getDocHeaderArgs(doc *org.Document) map[string]string {
	defaults := map[string]string{}
	if v := doc.Get("header-args"); v != "" {
		// Parse the same way block parameters are parsed: "key value :key value ..."
		parts := strings.Split(v, " :")
		for _, p := range parts {
			kv := strings.SplitN(strings.TrimSpace(p), " ", 2)
			if len(kv) == 2 {
				key := kv[0]
				if !strings.HasPrefix(key, ":") {
					key = ":" + key
				}
				defaults[key] = strings.TrimSpace(kv[1])
			}
		}
	}
	return defaults
}

// tangleResult holds the result of tangling a single file.
type tangleResult struct {
	Filename string
	Lines    int
}

// doTangle processes a parsed org document and tangles its source blocks.
// baseDir is the directory of the org file, used to resolve relative tangle paths.
func doTangle(doc *org.Document, baseDir string) ([]tangleResult, error) {
	docDefaults := getDocHeaderArgs(doc)
	allBlocks := collectBlocks(doc.Nodes)

	// First pass: collect noweb-ref blocks
	nowebRefs := map[string][]string{}
	for _, b := range allBlocks {
		params := b.ParameterMap()
		tb := parseParams(params, docDefaults)
		if tb.nowebRef != "" {
			nowebRefs[tb.nowebRef] = append(nowebRefs[tb.nowebRef], blockContent(b))
		}
	}
	// Also collect #+NAME:'d blocks as noweb targets
	for name, node := range doc.NamedNodes {
		if b, ok := node.(org.Block); ok && strings.ToUpper(b.Name) == "SRC" {
			if _, exists := nowebRefs[name]; !exists {
				nowebRefs[name] = []string{blockContent(&b)}
			}
		}
	}

	// Second pass: group blocks by tangle target file
	type fileEntry struct {
		blocks []tangleBlock
	}
	files := map[string]*fileEntry{}
	fileOrder := []string{}

	for _, b := range allBlocks {
		params := b.ParameterMap()
		tb := parseParams(params, docDefaults)

		// Skip blocks with no tangle target or :tangle no
		if tb.tangle == "" || strings.ToLower(tb.tangle) == "no" {
			continue
		}
		// Skip blocks that are only noweb-ref definitions (no tangle target of their own)
		// unless they also explicitly set :tangle
		if _, hasTangle := params[":tangle"]; !hasTangle {
			if _, hasDocTangle := docDefaults[":tangle"]; !hasDocTangle {
				continue
			}
		}

		content := blockContent(b)
		if tb.noweb {
			content = expandNoweb(content, nowebRefs, tb.nowebSep, 0)
		}
		tb.content = content

		// Resolve tangle path relative to org file directory
		tanglePath := tb.tangle
		if !filepath.IsAbs(tanglePath) {
			tanglePath = filepath.Join(baseDir, tanglePath)
		}
		tb.tangle = tanglePath

		if _, ok := files[tanglePath]; !ok {
			files[tanglePath] = &fileEntry{}
			fileOrder = append(fileOrder, tanglePath)
		}
		files[tanglePath].blocks = append(files[tanglePath].blocks, tb)
	}

	// Third pass: write files
	var results []tangleResult
	for _, path := range fileOrder {
		entry := files[path]
		if len(entry.blocks) == 0 {
			continue
		}

		// Use settings from the first block for file-level options
		first := entry.blocks[0]

		if first.mkdirp {
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				return nil, fmt.Errorf("failed to create directory for %s: %v", path, err)
			}
		}

		var sb strings.Builder
		if first.shebang != "" {
			sb.WriteString(first.shebang)
			sb.WriteString("\n")
		}

		for i, tb := range entry.blocks {
			if i > 0 && tb.padline {
				sb.WriteString("\n")
			}

			prefix := commentPrefix(tb.lang)
			if tb.comments == "link" || tb.comments == "both" {
				if prefix != "" {
					sb.WriteString(fmt.Sprintf("%s[[file:%s]]\n", prefix, filepath.Base(path)))
				}
			}

			sb.WriteString(tb.content)
		}

		content := sb.String()
		// Ensure file ends with a newline
		if !strings.HasSuffix(content, "\n") {
			content += "\n"
		}

		if err := os.WriteFile(path, []byte(content), first.tangleMode); err != nil {
			return nil, fmt.Errorf("failed to write %s: %v", path, err)
		}

		lineCount := strings.Count(content, "\n")
		results = append(results, tangleResult{Filename: path, Lines: lineCount})
		log.Printf("TANGLE: wrote %s (%d lines)\n", path, lineCount)
	}

	return results, nil
}

// Export tangles the org file specified by query and writes the output files to disk.
func (self *OrgTangleExporter) Export(db common.ODb, query string, to string, opts string, props map[string]string) error {
	f := db.GetFile(query)
	if f == nil {
		return fmt.Errorf("tangle: file not found: %s", query)
	}
	baseDir := filepath.Dir(f.Filename)
	_, err := doTangle(f.Doc, baseDir)
	return err
}

// ExportToString tangles the org file and returns a summary of what was written.
func (self *OrgTangleExporter) ExportToString(db common.ODb, query string, opts string, props map[string]string) (error, string) {
	f := db.GetFile(query)
	if f == nil {
		return fmt.Errorf("tangle: file not found: %s", query), ""
	}
	baseDir := filepath.Dir(f.Filename)
	results, err := doTangle(f.Doc, baseDir)
	if err != nil {
		return err, ""
	}
	if len(results) == 0 {
		return nil, "No blocks to tangle"
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Tangled %d file(s):\n", len(results)))
	for _, r := range results {
		sb.WriteString(fmt.Sprintf("  %s (%d lines)\n", r.Filename, r.Lines))
	}
	return nil, sb.String()
}

func init() {
	common.AddExporter("tangle", func() common.Exporter {
		return &OrgTangleExporter{}
	})
}
