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
	"github.com/ihdavids/orgs/internal/common/noweb"
)

// tangleBlock holds a single source block's content and its tangle parameters.
type tangleBlock struct {
	src        tangleSrc
	lang       string
	content    string
	tangle     string // output file path
	mkdirp     bool
	padline    bool
	shebang    string
	tangleMode os.FileMode
	noweb      bool
	stripNoweb bool
	nowebRef   string
	nowebSep   string
	// org's :noweb-prefix: the text before a reference repeated on every
	// line of its expansion. Default yes.
	nowebPrefix bool
	comments    string // none, link, both, org
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

// blockContent is a block's code as org tangles it: the indentation every
// line shares taken off (unless the block says -i, org's "preserve
// indentation"), and org's comma escape undone - ",*" and ",#+" at the start
// of a line are how a block holds a line that would otherwise read as a
// heading or a keyword.
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
	lines := strings.Split(strings.TrimSuffix(sb.String(), "\n"), "\n")
	if !hasSwitch(b, "-i") {
		lines = dedent(lines)
	}
	for i, l := range lines {
		lines[i] = unescapeLine(l)
	}
	if len(lines) == 1 && lines[0] == "" {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

func hasSwitch(b *org.Block, sw string) bool {
	for _, s := range b.Switches {
		if s == sw {
			return true
		}
	}
	for _, p := range b.Parameters {
		if p == sw {
			return true
		}
	}
	return false
}

// dedent takes off the indentation every non-blank line shares.
func dedent(lines []string) []string {
	min := -1
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		n := len(l) - len(strings.TrimLeft(l, " \t"))
		if min < 0 || n < min {
			min = n
		}
	}
	out := make([]string, len(lines))
	for i, l := range lines {
		if len(l) >= min && min > 0 {
			out[i] = l[min:]
		} else if strings.TrimSpace(l) == "" {
			out[i] = ""
		} else {
			out[i] = l
		}
	}
	return out
}

var escapedRe = regexp.MustCompile(`^(\s*),(,*(\*|#\+))`)
var needsEscapeRe = regexp.MustCompile(`^(\s*)(,*(\*|#\+))`)

// unescapeLine undoes org's comma escape: ",*" is "*", ",,*" is ",*".
func unescapeLine(l string) string { return escapedRe.ReplaceAllString(l, "$1$2") }

// EscapeLine is unescapeLine the other way about, for writing code back into
// a block (detangle): a line that would read as a heading or a keyword gets
// a comma, and one that already starts with commas before those gets one more.
func EscapeLine(l string) string { return needsEscapeRe.ReplaceAllString(l, "$1,$2") }

// commentDelims is how a language writes a one-line comment: what goes
// before it and, for the languages without line comments, after it. ok is
// false for a language with no comment syntax at all, which gets no markers.
func commentDelims(lang string) (string, string, bool) {
	switch strings.ToLower(lang) {
	case "python", "ruby", "perl", "bash", "sh", "zsh", "shell", "fish", "r", "julia", "yaml", "toml", "conf",
		"makefile", "nix", "elixir", "crystal", "tcl", "awk", "powershell", "dockerfile", "cmake", "nim":
		return "# ", "", true
	case "c", "cpp", "c++", "java", "javascript", "js", "typescript", "ts", "go", "rust", "swift", "kotlin", "scala",
		"dart", "groovy", "php", "csharp", "c#", "zig", "jsonc", "glsl", "proto", "d":
		return "// ", "", true
	case "lisp", "elisp", "emacs-lisp", "scheme", "clojure", "racket", "fennel", "hy":
		return ";; ", "", true
	case "haskell", "lua", "sql", "ada", "elm", "sqlite", "psql":
		return "-- ", "", true
	case "latex", "tex", "matlab", "octave", "erlang", "prolog":
		return "% ", "", true
	case "vim", "vimscript":
		return "\" ", "", true
	case "css", "scss", "less":
		return "/* ", " */", true
	case "html", "xml", "svg", "markdown", "md":
		return "<!-- ", " -->", true
	case "ocaml", "fsharp", "pascal":
		return "(* ", " *)", true
	case "json":
		return "", "", false
	}
	return "# ", "", true
}

// The comments org writes around a tangled block so it can be found again
// (org-babel-tangle-comment-links): a link to the heading the block is under,
// described by the block's name or "Heading:N", and a line saying where it
// ends. Detangle reads these back.
func beginMarker(pre, post, link, label string) string {
	return pre + "[[" + link + "][" + label + "]]" + post
}

func endMarker(pre, post, label string) string {
	return pre + label + " ends here" + post
}

// Label is what a block is called in its markers: its #+NAME:, else the
// title of the heading it is in and its place there ("Setup:2"), else - above
// the first heading - the org file's own name and its place.
func Label(src tangleSrc, orgPath string) string {
	if src.name != "" {
		return src.name
	}
	return headingOf(src, orgPath) + ":" + strconv.Itoa(src.index)
}

func headingOf(src tangleSrc, orgPath string) string {
	if n := len(src.chain); n > 0 && src.chain[n-1] != nil {
		return strings.TrimSpace(org.String(src.chain[n-1].Title...))
	}
	return strings.TrimSuffix(filepath.Base(orgPath), filepath.Ext(orgPath))
}

// The link in a begin marker: the org file relative to the tangled one, and
// the heading as an org search.
func markerLink(src tangleSrc, orgPath, target string) string {
	rel, err := filepath.Rel(filepath.Dir(target), orgPath)
	if err != nil {
		rel = orgPath
	}
	link := "file:" + filepath.ToSlash(rel)
	if n := len(src.chain); n > 0 && src.chain[n-1] != nil {
		link += "::*" + strings.TrimSpace(org.String(src.chain[n-1].Title...))
	}
	return link
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

// parseParams reads the tangle-related header arguments out of a block's
// arguments, already merged with what it inherits (argsFor).
func parseParams(merged map[string]string) tangleBlock {
	tb := tangleBlock{
		padline:    true,
		tangleMode: 0644,
		nowebSep:    "\n",
		nowebPrefix: true,
		comments:    "none",
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
		// The values that expand references when tangling; strip-tangle
		// takes them out instead (org-babel-noweb-p).
		switch strings.ToLower(v) {
		case "yes", "tangle", "no-export", "strip-export":
			tb.noweb = true
		case "strip-tangle":
			tb.stripNoweb = true
		}
	}
	if v, ok := merged[":noweb-ref"]; ok {
		tb.nowebRef = v
	}
	if v, ok := merged[":noweb-sep"]; ok {
		tb.nowebSep = v
	}
	if v, ok := merged[":noweb-prefix"]; ok {
		tb.nowebPrefix = strings.ToLower(strings.TrimSpace(v)) != "no"
	}
	if v, ok := merged[":comments"]; ok {
		tb.comments = strings.ToLower(v)
	}
	return tb
}

// Evaluate, when set, runs the block called name in the org file at orgPath
// with args (as a #+CALL: would) and answers its result: what a
// <<name(args)>> reference tangles to. The server sets it; it lives there,
// with babel. Nil leaves such a reference as written, with a warning.
var Evaluate func(orgPath, name, args string) (string, error)

// A source block and the headings it sits under, outermost first: what it
// inherits header arguments from. Name is its #+NAME:, Index its place among
// the source blocks in its heading's own text (1-based, org's "Heading:N"),
// and Prose the text written between the block before it and it, for
// :comments org and both.
type tangleSrc struct {
	block *org.Block
	chain []*org.Headline
	name  string
	index int
	prose string
}

// collectBlocks walks every source block out of a node tree.
//
// Three things about go-org make this harder than it reads, and all were silent:
//
//  1. **A block is a `*org.Block`**, not an `org.Block`. This used to match only
//     the value form, which the parser never produces - so no block was ever
//     collected, and tangling any file answered "nothing to tangle" however many
//     :tangle headers it had. A nested heading is a `*org.Headline` for the same
//     reason and was already handled.
//  2. **`Headline.Blocks` is not the blocks under a heading.** The loop that
//     fills it stops after the first node it looks at, so a heading with a
//     paragraph and then a block has an empty list. Walking the children is what
//     actually finds them, so the two together would double-count - the blocks
//     are taken from the walk alone.
//  3. **A block with a `#+NAME:` is an `org.NodeWithName`** around it (and one
//     with `#+CAPTION:`/`#+ATTR_...` an `org.NodeWithMeta`), so every named
//     block was skipped: the blocks a literate file names are most of them.
func collectBlocks(nodes []org.Node, chain []*org.Headline) []tangleSrc {
	var blocks []tangleSrc
	index := 0
	prose := []string{}
	var visit func(n org.Node, name string)
	visit = func(node org.Node, name string) {
		switch n := node.(type) {
		case *org.Block:
			if strings.ToUpper(n.Name) == "SRC" {
				index++
				blocks = append(blocks, tangleSrc{n, chain, name, index, strings.TrimSpace(strings.Join(prose, "\n"))})
				prose = prose[:0]
			}
		case org.Block:
			b := n
			visit(&b, name)
		case org.NodeWithName:
			visit(n.Node, n.Name)
		case *org.NodeWithName:
			visit(n.Node, n.Name)
		case org.NodeWithMeta:
			visit(n.Node, name)
		case *org.NodeWithMeta:
			visit(n.Node, name)
		case org.Paragraph:
			prose = append(prose, strings.TrimSpace(org.String(n.Children...)))
		case *org.Headline:
			blocks = append(blocks, collectBlocks(n.Children, append(append([]*org.Headline{}, chain...), n))...)
		case org.Headline:
			h := n
			blocks = append(blocks, collectBlocks(n.Children, append(append([]*org.Headline{}, chain...), &h))...)
		}
	}
	for _, node := range nodes {
		visit(node, "")
	}
	return blocks
}

// Names a language goes by, so header-args:sh reaches a bash block - the same
// families babel's runners treat as one.
var langFamily = map[string]string{
	"sh": "sh", "bash": "sh", "shell": "sh", "zsh": "sh",
	"js": "js", "javascript": "js", "node": "js",
	"elisp": "elisp", "emacs-lisp": "elisp",
	"py": "python", "python": "python", "python3": "python",
	"ts": "ts", "typescript": "ts",
	"c++": "cpp", "cpp": "cpp",
}

func sameLang(a, b string) bool {
	a, b = strings.ToLower(a), strings.ToLower(b)
	if a == b {
		return true
	}
	fa, fb := langFamily[a], langFamily[b]
	return fa != "" && fa == fb
}

// headerArgs reads ":key value :key2 value two" into a map, keeping quoted
// values whole. A value runs to the next word that starts with a colon.
func headerArgs(text string, into map[string]string) {
	words := []string{}
	cur := strings.Builder{}
	quoted := false
	for _, r := range strings.TrimSpace(text) {
		switch {
		case r == '"':
			quoted = !quoted
			cur.WriteRune(r)
		case (r == ' ' || r == '\t') && !quoted:
			if cur.Len() > 0 {
				words = append(words, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		words = append(words, cur.String())
	}
	key := ""
	val := []string{}
	flush := func() {
		if key != "" {
			into[key] = strings.Trim(strings.Join(val, " "), `"`)
		}
	}
	for _, w := range words {
		if strings.HasPrefix(w, ":") && len(w) > 1 {
			flush()
			key, val = strings.ToLower(w), nil
			continue
		}
		val = append(val, w)
	}
	flush()
}

// The language a #+PROPERTY: or property name is for: "" for plain
// header-args, "python" for header-args:python; ok is false for any other
// property. A trailing + (org's "add to") reads the same.
func headerArgsName(name string) (string, bool) {
	n := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(name)), "+")
	if n == "header-args" {
		return "", true
	}
	if strings.HasPrefix(n, "header-args:") {
		return n[len("header-args:"):], true
	}
	return "", false
}

// argsFor is every header argument a block has, the way org gives them: the
// file's #+PROPERTY: header-args, then header-args:<lang>, then each heading
// above it from the outermost in (its header-args, then its
// header-args:<lang>), then the block's own - so the nearest says last.
//
// go-org keeps every #+PROPERTY: line under the one key PROPERTY, so
// doc.Get("header-args") is always empty: read that way, a file's
// #+PROPERTY: header-args :tangle ... was never seen at all.
func argsFor(doc *org.Document, src tangleSrc) map[string]string {
	lang := ""
	if len(src.block.Parameters) > 0 {
		lang = src.block.Parameters[0]
	}
	out := map[string]string{}
	apply := func(pairs [][2]string) {
		for _, generalFirst := range []bool{true, false} {
			for _, kv := range pairs {
				l, ok := headerArgsName(kv[0])
				if !ok || (l == "") != generalFirst || (l != "" && !sameLang(l, lang)) {
					continue
				}
				headerArgs(kv[1], out)
			}
		}
	}
	if doc != nil {
		pairs := [][2]string{}
		for _, line := range strings.Split(doc.BufferSettings["PROPERTY"], "\n") {
			if name, rest, ok := strings.Cut(strings.TrimSpace(line), " "); ok {
				pairs = append(pairs, [2]string{name, rest})
			}
		}
		apply(pairs)
	}
	for _, h := range src.chain {
		if h == nil || h.Properties == nil {
			continue
		}
		pairs := [][2]string{}
		for _, kv := range h.Properties.Properties {
			if len(kv) >= 2 {
				pairs = append(pairs, [2]string{kv[0], kv[1]})
			}
		}
		apply(pairs)
	}
	for k, v := range src.block.ParameterMap() {
		out[strings.ToLower(k)] = v
	}
	return out
}

// The extension org gives a language's file for :tangle yes
// (org-babel-tangle-lang-exts); a language it does not list is its own name.
var tangleExt = map[string]string{
	"python": "py", "py": "py", "python3": "py",
	"emacs-lisp": "el", "elisp": "el",
	"sh": "sh", "bash": "sh", "shell": "sh", "zsh": "zsh",
	"javascript": "js", "js": "js", "typescript": "ts", "ts": "ts",
	"ruby": "rb", "perl": "pl", "rust": "rs", "haskell": "hs", "julia": "jl",
	"clojure": "clj", "scheme": "scm", "ocaml": "ml", "kotlin": "kt",
	"c++": "cpp", "cpp": "cpp", "latex": "tex", "markdown": "md",
	"r": "R", "lisp": "lisp", "lua": "lua", "go": "go", "java": "java",
	"c": "c", "css": "css", "html": "html", "sql": "sql", "yaml": "yaml",
	"toml": "toml", "json": "json", "dot": "dot", "awk": "awk",
}

// yesTarget is the file :tangle yes writes: the org file's own name with the
// language's extension, beside it.
func yesTarget(orgPath, lang string) string {
	ext, ok := tangleExt[strings.ToLower(lang)]
	if !ok {
		ext = strings.ToLower(lang)
	}
	base := strings.TrimSuffix(filepath.Base(orgPath), filepath.Ext(orgPath))
	return base + "." + ext
}

// TangleFile holds the assembled content for a single tangle output file.
type TangleFile struct {
	Filename   string      `json:"filename"`
	Content    string      `json:"content"`
	Lang       string      `json:"lang"`
	Lines      int         `json:"lines"`
	TangleMode os.FileMode `json:"-"`
	Mkdirp     bool        `json:"-"`
}

// TangleResult is the structured response from a tangle operation.
type TangleResult struct {
	Files []TangleFile `json:"files"`
	// References nothing answered to, left as written.
	Warnings []string `json:"warnings,omitempty"`
}

// assembleTangle processes a parsed org document and assembles the tangled content
// without writing to disk. baseDir is used to resolve relative tangle paths.
func assembleTangle(doc *org.Document, orgPath string) (*TangleResult, error) {
	baseDir := filepath.Dir(orgPath)
	allBlocks := collectBlocks(doc.Nodes, nil)

	// What a <<name>> can stand for: the blocks whose :noweb-ref is name
	// (joined with the first one's :noweb-sep), else the block #+NAME:d name.
	type refParts struct {
		parts []string
		sep   string
	}
	nowebRefs := map[string]*refParts{}
	named := map[string]string{}
	for _, src := range allBlocks {
		tb := parseParams(argsFor(doc, src))
		if tb.nowebRef != "" {
			rp, ok := nowebRefs[tb.nowebRef]
			if !ok {
				rp = &refParts{sep: tb.nowebSep}
				nowebRefs[tb.nowebRef] = rp
			}
			rp.parts = append(rp.parts, strings.TrimSuffix(blockContent(src.block), "\n"))
		}
		if src.name != "" {
			if _, ok := named[src.name]; !ok {
				named[src.name] = blockContent(src.block)
			}
		}
	}
	body := func(name string) (string, bool) {
		if rp, ok := nowebRefs[name]; ok {
			return strings.Join(rp.parts, rp.sep), true
		}
		b, ok := named[name]
		return b, ok
	}
	var call func(name, args string) (string, error)
	if Evaluate != nil {
		call = func(name, args string) (string, error) { return Evaluate(orgPath, name, args) }
	}
	result := &TangleResult{}

	// Second pass: group blocks by tangle target file
	type fileEntry struct {
		blocks []tangleBlock
	}
	files := map[string]*fileEntry{}
	fileOrder := []string{}

	for _, src := range allBlocks {
		b := src.block
		tb := parseParams(argsFor(doc, src))
		tb.src = src
		if tb.lang == "" && len(b.Parameters) > 0 {
			tb.lang = b.Parameters[0]
		}

		// Skip blocks with no tangle target or :tangle no
		if tb.tangle == "" || strings.ToLower(tb.tangle) == "no" {
			continue
		}
		// :tangle yes is a file named after this one, for the language.
		if strings.ToLower(tb.tangle) == "yes" {
			tb.tangle = yesTarget(orgPath, tb.lang)
		}

		content := blockContent(b)
		if tb.noweb {
			out, warns, err := noweb.Expand(content, noweb.Resolver{Body: body, Call: call, Prefix: tb.nowebPrefix})
			if err != nil {
				return nil, fmt.Errorf("%s: %v", Label(src, orgPath), err)
			}
			for _, w := range warns {
				result.Warnings = append(result.Warnings, Label(src, orgPath)+": "+w)
			}
			content = out
		} else if tb.stripNoweb {
			content = noweb.Strip(content)
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

	// Third pass: assemble content for each file
	for _, path := range fileOrder {
		entry := files[path]
		if len(entry.blocks) == 0 {
			continue
		}

		first := entry.blocks[0]

		var sb strings.Builder
		if first.shebang != "" {
			sb.WriteString(first.shebang)
			sb.WriteString("\n")
		}

		for i, tb := range entry.blocks {
			if i > 0 && tb.padline {
				sb.WriteString("\n")
			}
			pre, post, ok := commentDelims(tb.lang)
			if ok && (tb.comments == "org" || tb.comments == "both") && tb.src.prose != "" {
				for _, l := range strings.Split(tb.src.prose, "\n") {
					sb.WriteString(strings.TrimRight(pre+l+post, " ") + "\n")
				}
			}
			links := ok && (tb.comments == "link" || tb.comments == "both" || tb.comments == "noweb" || tb.comments == "yes")
			label := Label(tb.src, orgPath)
			if links {
				sb.WriteString(beginMarker(pre, post, markerLink(tb.src, orgPath, path), label) + "\n")
			}
			sb.WriteString(tb.content)
			if links {
				sb.WriteString(endMarker(pre, post, label) + "\n")
			}
		}

		content := sb.String()
		if !strings.HasSuffix(content, "\n") {
			content += "\n"
		}

		result.Files = append(result.Files, TangleFile{
			Filename:   path,
			Content:    content,
			Lang:       first.lang,
			Lines:      strings.Count(content, "\n"),
			TangleMode: first.tangleMode,
			Mkdirp:     first.mkdirp,
		})
	}

	return result, nil
}

// OnWrite, when set, is told of every file a tangle writes (the server uses
// it to remember what it wrote, so auto-tangle can tell a tangled file somebody
// edited by hand from one it wrote itself).
var OnWrite func(path string, content []byte)

// WriteFile writes one tangled file, unless it already says exactly that. It
// says whether it wrote: a file rewritten with what it had makes every
// editor holding it reload, and every watcher of it fire.
func WriteFile(tf TangleFile) (bool, error) {
	if old, err := os.ReadFile(tf.Filename); err == nil && string(old) == tf.Content {
		if OnWrite != nil {
			OnWrite(tf.Filename, old)
		}
		return false, nil
	}
	if tf.Mkdirp {
		if err := os.MkdirAll(filepath.Dir(tf.Filename), 0755); err != nil {
			return false, fmt.Errorf("failed to create directory for %s: %v", tf.Filename, err)
		}
	}
	if err := os.WriteFile(tf.Filename, []byte(tf.Content), tf.TangleMode); err != nil {
		return false, fmt.Errorf("failed to write %s: %v", tf.Filename, err)
	}
	// WriteFile keeps an existing file's mode; :tangle-mode is meant.
	os.Chmod(tf.Filename, tf.TangleMode)
	if OnWrite != nil {
		OnWrite(tf.Filename, []byte(tf.Content))
	}
	log.Printf("TANGLE: wrote %s (%d lines)\n", tf.Filename, tf.Lines)
	return true, nil
}

// writeTangleFiles writes the assembled tangle results to disk.
func writeTangleFiles(result *TangleResult) error {
	for _, tf := range result.Files {
		if _, err := WriteFile(tf); err != nil {
			return err
		}
	}
	return nil
}

// Assemble is what tangling a file would write, writing nothing.
func Assemble(doc *org.Document, orgPath string) (*TangleResult, error) {
	return assembleTangle(doc, orgPath)
}

// Tangle assembles tangle output for an org file from the database.
// If write is true, it also writes the files to disk.
func Tangle(db common.ODb, query string, write bool) (*TangleResult, error) {
	f := db.GetFile(query)
	if f == nil {
		return nil, fmt.Errorf("tangle: file not found: %s", query)
	}
	result, err := assembleTangle(f.Doc, f.Filename)
	if err != nil {
		return nil, err
	}
	if write {
		if err := writeTangleFiles(result); err != nil {
			return nil, err
		}
	}
	return result, nil
}

// Export tangles the org file specified by query and writes the output files to disk.
func (self *OrgTangleExporter) Export(db common.ODb, query string, to string, opts string, props map[string]string) error {
	_, err := Tangle(db, query, true)
	return err
}

// ExportToString tangles the org file and returns a summary of what was written.
func (self *OrgTangleExporter) ExportToString(db common.ODb, query string, opts string, props map[string]string) (error, string) {
	result, err := Tangle(db, query, true)
	if err != nil {
		return err, ""
	}
	if len(result.Files) == 0 {
		return nil, "No blocks to tangle"
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Tangled %d file(s):\n", len(result.Files)))
	for _, tf := range result.Files {
		sb.WriteString(fmt.Sprintf("  %s (%d lines)\n", tf.Filename, tf.Lines))
	}
	return nil, sb.String()
}

func init() {
	common.AddExporter("tangle", func() common.Exporter {
		return &OrgTangleExporter{}
	})
}

// ----------------------------------------------------------------------------
// Going back: what detangle needs
// ----------------------------------------------------------------------------

// BlockRef is one source block of an org file as its markers name it.
type BlockRef struct {
	Label string
	Lang  string
	// The #+BEGIN_SRC line, zero based.
	Row int
	// The block keeps its indentation (-i), so code goes back as written.
	Preserve bool
	// The block's code has <<references>> that a tangle expands: what comes
	// back from the tangled file is the expansion, and cannot go back.
	Noweb bool
	Code  string
}

// Blocks lists a file's source blocks by their marker labels, in file order.
func Blocks(doc *org.Document, orgPath string) []BlockRef {
	out := []BlockRef{}
	for _, src := range collectBlocks(doc.Nodes, nil) {
		tb := parseParams(argsFor(doc, src))
		lang := ""
		if len(src.block.Parameters) > 0 {
			lang = src.block.Parameters[0]
		}
		code := blockContent(src.block)
		out = append(out, BlockRef{Label: Label(src, orgPath), Lang: lang, Row: src.block.Pos.Row,
			Preserve: hasSwitch(src.block, "-i"), Noweb: tb.noweb && noweb.Has(code), Code: code})
	}
	return out
}

// Section is one block's code found between its markers in a tangled file.
type Section struct {
	// The org file, as the link has it (relative to the tangled file), and
	// the heading it searches for ("" above the first heading).
	File    string
	Heading string
	Label   string
	Body    []string
	// The begin marker's line, zero based.
	Line int
	// Markers inside it: a noweb expansion, which cannot be put back as one
	// block's code.
	Nested bool
}

var (
	markerBeginRe = regexp.MustCompile(`^\s*(?:#|//|;;|--|%|"|/\*|<!--|\(\*)?\s*\[\[file:([^\]]+?)\]\[([^\]]+)\]\]\s*(?:\*/|-->|\*\))?\s*$`)
	markerEndRe   = regexp.MustCompile(`^\s*(?:#|//|;;|--|%|"|/\*|<!--|\(\*)?\s*(.+?) ends here\s*(?:\*/|-->|\*\))?\s*$`)
)

// ParseMarkers finds the blocks in a tangled file's text.
func ParseMarkers(text string) []Section {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	out := []Section{}
	type open struct {
		sec    Section
		start  int
		nested bool
	}
	stack := []*open{}
	for i, l := range lines {
		if m := markerBeginRe.FindStringSubmatch(l); m != nil {
			file, heading := m[1], ""
			if j := strings.Index(file, "::"); j >= 0 {
				file, heading = file[:j], strings.TrimPrefix(file[j+2:], "*")
			}
			for _, o := range stack {
				o.nested = true
			}
			stack = append(stack, &open{sec: Section{File: file, Heading: heading, Label: m[2], Line: i}, start: i + 1})
			continue
		}
		if len(stack) == 0 {
			continue
		}
		top := stack[len(stack)-1]
		if m := markerEndRe.FindStringSubmatch(l); m != nil && m[1] == top.sec.Label {
			top.sec.Body = append([]string{}, lines[top.start:i]...)
			top.sec.Nested = top.nested
			out = append(out, top.sec)
			stack = stack[:len(stack)-1]
		}
	}
	return out
}
