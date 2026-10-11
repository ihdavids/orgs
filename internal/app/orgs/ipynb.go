package orgs

/* SDOC: Editing
* Importing A Jupyter Notebook

  =orgs import analysis.ipynb= (or worg's new file dialog, "Import a Jupyter
  notebook") turns a notebook into an org file that runs: every code cell a
  =#+BEGIN_SRC= block, its outputs the =#+RESULTS:= under it, and the whole
  file one python session, so running the blocks top to bottom is running
  the notebook.

  #+BEGIN_EXAMPLE
  #+TITLE: Penguin measurements
  #+PROPERTY: header-args:python :session penguins :results output
  #+STARTUP: inlineimages

  * Loading the data
  #+BEGIN_SRC python
  df = pd.read_csv("penguins.csv")
  df.shape
  #+END_SRC

  #+RESULTS:
  : (344, 7)
  #+END_EXAMPLE

  - The title is the notebook's own (=metadata.title=), else its first
    heading, else its file name.
  - Markdown becomes org: headings (so code lands under the heading it sat
    under), emphasis, inline code, links, pictures, lists, quotes, fenced
    code, tables and rules. =$…$= and =$$…$$= are left alone, since org reads
    them as they are. HTML that has no org spelling goes in an export block.
  - Outputs are written the way running the block writes them, so running it
    again replaces them rather than adding to them: printed text as =: =
    lines, a simple html table (a pandas dataframe) as an org table, a
    picture saved beside the file in =<name>_files/= and linked, a traceback
    with its colours taken out. A cell with several outputs has them in a
    =:results:= drawer.
  - A cell that drew a picture says =:file= where, so running it again draws
    over the same picture.
  - Pictures pasted into markdown cells (=attachment:=) are saved too.
  - Raw cells become export blocks.

  nbformat 4 is read, and 3 as far as its cells go; anything older is
  refused with a message saying so.
EDOC */

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ihdavids/orgs/internal/common"
)

// ----------------------------------------------------------------------------
// Reading the notebook
// ----------------------------------------------------------------------------

// Text in a notebook is a string or a list of strings that are joined - the
// format allows both and both are common.
type nbText string

func (t *nbText) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		*t = nbText(s)
		return nil
	}
	var l []string
	if err := json.Unmarshal(b, &l); err != nil {
		return err
	}
	*t = nbText(strings.Join(l, ""))
	return nil
}

type nbOutput struct {
	OutputType string                     `json:"output_type"`
	Name       string                     `json:"name"`
	Text       nbText                     `json:"text"`
	Data       map[string]json.RawMessage `json:"data"`
	Ename      string                     `json:"ename"`
	Evalue     string                     `json:"evalue"`
	Traceback  []string                   `json:"traceback"`
}

type nbCell struct {
	CellType       string                                `json:"cell_type"`
	Source         nbText                                `json:"source"`
	Metadata       map[string]json.RawMessage            `json:"metadata"`
	Outputs        []nbOutput                            `json:"outputs"`
	ExecutionCount *int                                  `json:"execution_count"`
	Attachments    map[string]map[string]json.RawMessage `json:"attachments"`
}

type notebook struct {
	Nbformat      int                        `json:"nbformat"`
	NbformatMinor int                        `json:"nbformat_minor"`
	Metadata      map[string]json.RawMessage `json:"metadata"`
	Cells         []nbCell                   `json:"cells"`
	// nbformat 3 kept its cells in worksheets.
	Worksheets []struct {
		Cells []nb3Cell `json:"cells"`
	} `json:"worksheets"`
}

// An nbformat 3 cell: code in "input", headings a cell type of their own, and
// outputs keyed by short names.
type nb3Cell struct {
	CellType string                     `json:"cell_type"`
	Source   nbText                     `json:"source"`
	Input    nbText                     `json:"input"`
	Level    int                        `json:"level"`
	Language string                     `json:"language"`
	Metadata map[string]json.RawMessage `json:"metadata"`
	Outputs  []map[string]json.RawMessage `json:"outputs"`
}

// The data of an output under a mime type, as text; "" when it has none.
func nbData(o nbOutput, mime string) string {
	raw, ok := o.Data[mime]
	if !ok {
		return ""
	}
	var t nbText
	if err := json.Unmarshal(raw, &t); err == nil {
		return string(t)
	}
	return string(raw)
}

func metaString(m map[string]json.RawMessage, path ...string) string {
	cur := m
	for i, k := range path {
		raw, ok := cur[k]
		if !ok {
			return ""
		}
		if i == len(path)-1 {
			var s string
			if json.Unmarshal(raw, &s) == nil {
				return s
			}
			return ""
		}
		next := map[string]json.RawMessage{}
		if json.Unmarshal(raw, &next) != nil {
			return ""
		}
		cur = next
	}
	return ""
}

// nbformat 3's cells in 4's shape: input is source, a heading cell is a
// markdown heading, pyout/pyerr are execute_result/error and their data is
// keyed by mime type.
func upgradeNb3(nb *notebook) {
	short := map[string]string{
		"text": "text/plain", "html": "text/html", "png": "image/png", "jpeg": "image/jpeg",
		"svg": "image/svg+xml", "latex": "text/latex", "markdown": "text/markdown", "json": "application/json",
	}
	for _, ws := range nb.Worksheets {
		for _, c := range ws.Cells {
			cell := nbCell{CellType: c.CellType, Source: c.Source, Metadata: c.Metadata}
			switch c.CellType {
			case "heading":
				lvl := c.Level
				if lvl < 1 {
					lvl = 1
				}
				cell.CellType = "markdown"
				cell.Source = nbText(strings.Repeat("#", lvl) + " " + strings.TrimSpace(string(c.Source)))
			case "code":
				cell.Source = c.Input
				for _, o := range c.Outputs {
					out := nbOutput{Data: map[string]json.RawMessage{}}
					json.Unmarshal(o["output_type"], &out.OutputType)
					switch out.OutputType {
					case "pyout":
						out.OutputType = "execute_result"
					case "pyerr":
						out.OutputType = "error"
					}
					json.Unmarshal(o["stream"], &out.Name)
					json.Unmarshal(o["ename"], &out.Ename)
					json.Unmarshal(o["evalue"], &out.Evalue)
					json.Unmarshal(o["traceback"], &out.Traceback)
					if out.OutputType == "stream" {
						json.Unmarshal(o["text"], &out.Text)
					} else {
						for k, mime := range short {
							if v, ok := o[k]; ok {
								out.Data[mime] = v
							}
						}
					}
					cell.Outputs = append(cell.Outputs, out)
				}
			}
			nb.Cells = append(nb.Cells, cell)
		}
	}
}

// ----------------------------------------------------------------------------
// The conversion
// ----------------------------------------------------------------------------

// What a notebook became: the org text, the files to write beside it (keyed
// by their path relative to the org file), and the counts worth reporting.
type ipynbConversion struct {
	Org   string
	Assets map[string][]byte
	common.IpynbImported
}

// What the conversion needs to know besides the notebook.
type ipynbOptions struct {
	// The org file's name without its extension: the session's name and the
	// <base>_files folder pictures go in.
	Base string
	// The notebook's own file name, for the title of last resort and the line
	// saying where the file came from. May be empty.
	Source string
	// When it was imported, for that same line.
	Now time.Time
}

// The most lines of one output worth writing. A training loop that printed a
// line per batch is not something to read in a file, and the line saying how
// much was left out is.
const ipynbMaxOutputLines = 2000

// The biggest html output kept as html. Past this it is a plotly figure or a
// widget carrying its own javascript, and its text form is the useful one.
const ipynbMaxHtml = 100 << 10

func convertIpynb(data []byte, opt ipynbOptions) (*ipynbConversion, error) {
	var nb notebook
	if err := json.Unmarshal(data, &nb); err != nil {
		return nil, fmt.Errorf("that is not a notebook: %v", err)
	}
	switch {
	case nb.Nbformat == 4:
	case nb.Nbformat == 3:
		upgradeNb3(&nb)
	case nb.Nbformat == 0 && len(nb.Cells) > 0:
		// No version, but cells in 4's shape: read it as 4.
	default:
		return nil, fmt.Errorf("this is nbformat %d; orgs reads 4 (and 3). Open it in a current Jupyter and save it to upgrade it", nb.Nbformat)
	}
	if opt.Base == "" {
		opt.Base = "notebook"
	}
	if opt.Now.IsZero() {
		opt.Now = time.Now()
	}
	c := &ipynbConversion{Assets: map[string][]byte{}}
	c.Nbformat = nb.Nbformat
	c.Cells = len(nb.Cells)

	// The language, as org names it.
	lang := metaString(nb.Metadata, "kernelspec", "language")
	if lang == "" {
		lang = metaString(nb.Metadata, "language_info", "name")
	}
	if lang == "" {
		lang = metaString(nb.Metadata, "kernelspec", "name")
	}
	c.Language = orgLangFor(lang)
	c.Session = sessionNameFor(opt.Base)
	filesDir := opt.Base + "_files"

	// The title: the notebook's own, its first heading, or its file name. A
	// first heading used as the title is not written again, and the headings
	// under it move up a level so they are not all children of nothing.
	title := metaString(nb.Metadata, "title")
	if title == "" {
		title = metaString(nb.Metadata, "name")
	}
	skipHeading := -1
	if title == "" {
		for i, cell := range nb.Cells {
			if cell.CellType != "markdown" {
				continue
			}
			if lvl, text, ok := firstMarkdownHeading(string(cell.Source)); ok && lvl == 1 {
				title, skipHeading = text, i
			}
			break
		}
	}
	if title == "" && opt.Source != "" {
		title = titleFromFilename(opt.Source)
	}
	if title == "" {
		title = titleFromFilename(opt.Base)
	}
	c.Title = title

	// The shallowest heading left decides how far headings move up.
	least := 0
	for i, cell := range nb.Cells {
		if cell.CellType != "markdown" {
			continue
		}
		for j, l := range markdownHeadingLevels(string(cell.Source)) {
			if i == skipHeading && j == 0 {
				continue
			}
			if least == 0 || l < least {
				least = l
			}
		}
	}
	shift := 0
	if least > 1 {
		shift = least - 1
	}

	out := []string{"#+TITLE: " + oneLine(title)}
	out = append(out, fmt.Sprintf("#+PROPERTY: header-args:%s :session %s :results output", c.Language, c.Session))
	out = append(out, "#+STARTUP: inlineimages")
	from := "a notebook"
	if opt.Source != "" {
		from = filepath.Base(opt.Source)
	}
	out = append(out, fmt.Sprintf("# Imported from %s (nbformat %d.%d) on %s.", from, nb.Nbformat, nb.NbformatMinor, opt.Now.Format("2006-01-02")))

	codeNo := 0
	for i, cell := range nb.Cells {
		src := strings.ReplaceAll(string(cell.Source), "\r\n", "\n")
		var chunk []string
		switch cell.CellType {
		case "markdown":
			c.Markdown++
			if strings.TrimSpace(src) == "" {
				continue
			}
			attach := map[string]string{}
			for name, mimes := range cell.Attachments {
				if p := c.saveAttachment(filesDir, i, name, mimes); p != "" {
					attach[name] = p
				}
			}
			if i == skipHeading {
				src = dropFirstHeading(src)
			}
			chunk = markdownToOrg(src, mdContext{shift: shift, attachments: attach})
		case "code":
			c.Code++
			if strings.TrimSpace(src) == "" && len(cell.Outputs) == 0 {
				continue
			}
			codeNo++
			chunk = c.codeCell(cell, src, codeNo, filesDir)
		case "raw":
			c.Raw++
			if strings.TrimSpace(src) == "" {
				continue
			}
			kind := rawExportKind(metaString(cell.Metadata, "format"), metaString(cell.Metadata, "raw_mimetype"))
			chunk = append([]string{"#+BEGIN_EXPORT " + kind}, escapeBlockLines(strings.TrimRight(src, "\n"))...)
			chunk = append(chunk, "#+END_EXPORT")
		default:
			c.Warnings = append(c.Warnings, fmt.Sprintf("cell %d is a %q cell, which was left out", i+1, cell.CellType))
			continue
		}
		if len(chunk) == 0 {
			continue
		}
		out = append(out, "")
		out = append(out, chunk...)
	}
	c.Org = strings.Join(out, "\n") + "\n"
	return c, nil
}

// One code cell: its block, and its outputs as the result under it.
func (c *ipynbConversion) codeCell(cell nbCell, src string, n int, filesDir string) []string {
	lang := c.Language
	body := strings.TrimRight(src, "\n")
	header := ""

	// A cell that is all one other language (%%bash, %%html...) is written as
	// that language, which is what it is, and needs no python to run.
	first, rest, _ := strings.Cut(body, "\n")
	if magic := strings.TrimSpace(first); strings.HasPrefix(magic, "%%") {
		switch f := strings.Fields(magic[2:]); {
		case len(f) == 0:
		case f[0] == "bash" || f[0] == "sh":
			lang, body = "sh", rest
		case f[0] == "javascript" || f[0] == "js":
			lang, body = "js", rest
		case f[0] == "html" || f[0] == "latex" || f[0] == "markdown":
			// Not code at all: it is what the cell shows.
			kind := map[string]string{"html": "html", "latex": "latex", "markdown": "md"}[f[0]]
			lines := append([]string{"#+BEGIN_EXPORT " + kind}, escapeBlockLines(rest)...)
			return append(lines, "#+END_EXPORT")
		}
	}

	// The outputs, in the order the notebook shows them.
	pieces := []resultPiece{}
	pics := 0
	for _, o := range cell.Outputs {
		switch o.OutputType {
		case "stream":
			pieces = append(pieces, resultPiece{text: string(o.Text)})
		case "error":
			c.Errors++
			tb := stripAnsi(strings.Join(o.Traceback, "\n"))
			if strings.TrimSpace(tb) == "" {
				tb = o.Ename + ": " + o.Evalue
			}
			pieces = append(pieces, resultPiece{text: tb})
		case "execute_result", "display_data":
			p, ok := c.richOutput(o, n, &pics, filesDir)
			if ok {
				pieces = append(pieces, p)
			}
		}
	}
	if pics > 0 && lang == "python" {
		// Where the picture goes when the block runs again: the one it came
		// with. graphics and file say so to emacs too.
		header = fmt.Sprintf(" :results graphics file :file %s", picName(filesDir, n, 1, ".png"))
		if first := firstLink(pieces); first != "" {
			header = fmt.Sprintf(" :results graphics file :file %s", first)
		}
	}

	lines := []string{"#+BEGIN_SRC " + lang + header}
	lines = append(lines, escapeBlockLines(body)...)
	lines = append(lines, "#+END_SRC")
	c.Blocks++

	kind, result := composePieces(pieces)
	if result == "" {
		return lines
	}
	c.Outputs++
	res := resultBody(&common.CodeResult{Kind: kind, Result: result}, nil, lang)
	if len(res) == 0 {
		return lines
	}
	lines = append(lines, "", "#+RESULTS:")
	return append(lines, res...)
}

// The best of what an execute_result or display_data carries: a picture, a
// table, html, then text.
func (c *ipynbConversion) richOutput(o nbOutput, n int, pics *int, filesDir string) (resultPiece, bool) {
	for _, im := range []struct{ mime, ext string }{{"image/png", ".png"}, {"image/jpeg", ".jpg"}, {"image/gif", ".gif"}, {"image/svg+xml", ".svg"}} {
		d := nbData(o, im.mime)
		if d == "" {
			continue
		}
		var b []byte
		if im.mime == "image/svg+xml" {
			b = []byte(d)
		} else {
			var err error
			b, err = base64.StdEncoding.DecodeString(strings.Join(strings.Fields(d), ""))
			if err != nil {
				c.Warnings = append(c.Warnings, fmt.Sprintf("a picture in code cell %d is not base64 and was left out", n))
				continue
			}
		}
		*pics++
		name := picName(filesDir, n, *pics, im.ext)
		c.Assets[name] = b
		c.Images++
		return resultPiece{link: "[[file:" + name + "]]"}, true
	}
	plain := nbData(o, "text/plain")
	if h := nbData(o, "text/html"); h != "" {
		if rows := htmlTable(h); rows != nil {
			return resultPiece{table: rows}, true
		}
		// A plain spelling that says something beats html that says it with
		// markup; one that only names the object ("<IPython.core.display.HTML
		// object>") does not.
		if plain != "" && !reprOnlyRe.MatchString(strings.TrimSpace(plain)) {
			return resultPiece{text: plain}, true
		}
		if len(h) <= ipynbMaxHtml {
			return resultPiece{html: h}, true
		}
	}
	if md := nbData(o, "text/markdown"); md != "" && (plain == "" || reprOnlyRe.MatchString(strings.TrimSpace(plain))) {
		return resultPiece{org: strings.Join(markdownToOrg(md, mdContext{}), "\n")}, true
	}
	if lx := nbData(o, "text/latex"); lx != "" && (plain == "" || reprOnlyRe.MatchString(strings.TrimSpace(plain))) {
		return resultPiece{org: strings.TrimSpace(lx)}, true
	}
	if plain != "" {
		return resultPiece{text: plain}, true
	}
	return resultPiece{}, false
}

var reprOnlyRe = regexp.MustCompile(`^<[\w.]+ (object|at 0x[0-9a-f]+)>$|^<[\w.]+ object at 0x[0-9a-f]+>$`)

// cell-7.png, cell-7-2.png: the names a session gives the figures of a block
// whose :file is cell-7.png, so importing and running again agree.
func picName(dir string, cell, k int, ext string) string {
	if k <= 1 {
		return fmt.Sprintf("%s/cell-%d%s", dir, cell, ext)
	}
	return fmt.Sprintf("%s/cell-%d-%d%s", dir, cell, k, ext)
}

func firstLink(ps []resultPiece) string {
	for _, p := range ps {
		if p.link != "" {
			return strings.TrimSuffix(strings.TrimPrefix(p.link, "[[file:"), "]]")
		}
	}
	return ""
}

var safeNameRe = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func (c *ipynbConversion) saveAttachment(dir string, cell int, name string, mimes map[string]json.RawMessage) string {
	for mime, raw := range mimes {
		if !strings.HasPrefix(mime, "image/") {
			continue
		}
		var t nbText
		if json.Unmarshal(raw, &t) != nil {
			continue
		}
		var b []byte
		if mime == "image/svg+xml" {
			b = []byte(t)
		} else {
			var err error
			if b, err = base64.StdEncoding.DecodeString(strings.Join(strings.Fields(string(t)), "")); err != nil {
				continue
			}
		}
		clean := safeNameRe.ReplaceAllString(filepath.Base(name), "-")
		p := fmt.Sprintf("%s/md-%d-%s", dir, cell+1, clean)
		c.Assets[p] = b
		c.Images++
		return p
	}
	return ""
}

// One output of a cell, before they are put together.
type resultPiece struct {
	text  string     // printed text, shown fixed width
	table [][]string // a table, the first row its header
	link  string     // [[file:...]]
	html  string     // html with no org spelling
	org   string     // already org (converted markdown)
}

// composePieces is composeResult for outputs in an order of their own: one
// alone in its own shape, several in a :results: drawer in the order they
// came. Neighbouring text is one text.
func composePieces(ps []resultPiece) (kind, result string) {
	merged := []resultPiece{}
	for _, p := range ps {
		if p.text != "" && len(merged) > 0 && merged[len(merged)-1].text != "" {
			merged[len(merged)-1].text += p.text
			continue
		}
		if p.text == "" && p.table == nil && p.link == "" && p.html == "" && p.org == "" {
			continue
		}
		merged = append(merged, p)
	}
	for i := range merged {
		if merged[i].text != "" {
			merged[i].text = capLines(strings.TrimRight(merged[i].text, "\n"))
			if strings.TrimSpace(merged[i].text) == "" {
				merged[i].text = ""
			}
		}
	}
	keep := merged[:0]
	for _, p := range merged {
		if p.text != "" || p.table != nil || p.link != "" || p.html != "" || p.org != "" {
			keep = append(keep, p)
		}
	}
	merged = keep
	if len(merged) == 0 {
		return "text", ""
	}
	if len(merged) == 1 {
		p := merged[0]
		switch {
		case p.table != nil:
			return "table", tableResultText(p.table)
		case p.link != "":
			return "file", p.link
		case p.text != "":
			k := babelShape(nil, p.text)
			return k, babelFormat(k, p.text)
		}
	}
	body := []string{":results:"}
	for _, p := range merged {
		switch {
		case p.text != "":
			body = append(body, resultBody(&common.CodeResult{Kind: "text", Result: p.text}, nil, "")...)
		case p.table != nil:
			body = append(body, tableResultText(p.table))
		case p.link != "":
			body = append(body, p.link)
		case p.html != "":
			body = append(body, "#+begin_export html")
			body = append(body, escapeBlockLines(strings.TrimRight(p.html, "\n"))...)
			body = append(body, "#+end_export")
		case p.org != "":
			body = append(body, p.org)
		}
	}
	return "drawer", strings.Join(append(body, ":end:"), "\n")
}

func capLines(t string) string {
	lines := strings.Split(t, "\n")
	if len(lines) <= ipynbMaxOutputLines {
		return t
	}
	more := len(lines) - ipynbMaxOutputLines
	return strings.Join(lines[:ipynbMaxOutputLines], "\n") + fmt.Sprintf("\n[… %d more lines were not imported]", more)
}

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b[()][A-Za-z0-9]|\x1b[=>]`)

func stripAnsi(s string) string { return ansiRe.ReplaceAllString(s, "") }

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// The language a kernel names, as org names it.
func orgLangFor(lang string) string {
	l := strings.ToLower(strings.TrimSpace(lang))
	switch {
	case l == "" || strings.HasPrefix(l, "python") || l == "ipython" || strings.HasPrefix(l, "pyspark"):
		return "python"
	case l == "r" || l == "ir":
		return "R"
	case strings.HasPrefix(l, "julia"):
		return "julia"
	case l == "bash" || l == "sh" || l == "zsh":
		return "sh"
	case l == "javascript" || strings.HasPrefix(l, "node"):
		return "js"
	case l == "c++" || strings.HasPrefix(l, "xcpp"):
		return "C++"
	}
	return l
}

// A session's name from the file's: letters, digits, - and _.
func sessionNameFor(base string) string {
	s := strings.Trim(safeNameRe.ReplaceAllString(strings.ToLower(base), "-"), "-.")
	if s == "" {
		return "notebook"
	}
	return s
}

func rawExportKind(format, mime string) string {
	f := strings.ToLower(format + " " + mime)
	switch {
	case strings.Contains(f, "html"):
		return "html"
	case strings.Contains(f, "latex") || strings.Contains(f, "tex"):
		return "latex"
	case strings.Contains(f, "markdown"):
		return "md"
	case strings.Contains(f, "rst") || strings.Contains(f, "restructuredtext"):
		return "rst"
	case strings.Contains(f, "python"):
		return "python"
	}
	return "text"
}

// ----------------------------------------------------------------------------
// Html tables: a dataframe's output
// ----------------------------------------------------------------------------

var (
	tableRe   = regexp.MustCompile(`(?is)<table\b[^>]*>(.*?)</table>`)
	trRe      = regexp.MustCompile(`(?is)<tr\b[^>]*>(.*?)</tr>`)
	cellRe    = regexp.MustCompile(`(?is)<(t[hd])\b([^>]*)>(.*?)</t[hd]>`)
	tagRe     = regexp.MustCompile(`(?s)<[^>]+>`)
	spanRe    = regexp.MustCompile(`(?i)(row|col)span\s*=\s*"?(\d+)`)
	theadRe   = regexp.MustCompile(`(?is)<thead\b[^>]*>(.*?)</thead>`)
	styleRe   = regexp.MustCompile(`(?is)<style\b.*?</style>`)
	dfShapeRe = regexp.MustCompile(`(?is)<p>\s*\d+ rows × \d+ columns\s*</p>`)
)

// The rows of an html table simple enough to be an org table: one table,
// with no cell spanning rows. A pandas dataframe is the case that matters,
// and its two header rows (column names, then the index's name) are folded
// into one. nil when it is not that simple.
func htmlTable(h string) [][]string {
	h = styleRe.ReplaceAllString(h, "")
	h = dfShapeRe.ReplaceAllString(h, "")
	tables := tableRe.FindAllStringSubmatch(h, -1)
	if len(tables) != 1 {
		return nil
	}
	// Anything but the table and whitespace around it means the html is
	// saying more than the table does.
	rest := strings.TrimSpace(tagRe.ReplaceAllString(tableRe.ReplaceAllString(h, ""), ""))
	if rest != "" {
		return nil
	}
	inner := tables[0][1]
	headRows := 0
	if m := theadRe.FindStringSubmatch(inner); m != nil {
		headRows = len(trRe.FindAllString(m[1], -1))
	}
	rows := [][]string{}
	for _, tr := range trRe.FindAllStringSubmatch(inner, -1) {
		row := []string{}
		for _, c := range cellRe.FindAllStringSubmatch(tr[1], -1) {
			if m := spanRe.FindStringSubmatch(c[2]); m != nil {
				n, _ := strconv.Atoi(m[2])
				if strings.EqualFold(m[1], "row") && n > 1 {
					return nil
				}
				text := htmlCellText(c[3])
				row = append(row, text)
				for k := 1; k < n && k < 50; k++ {
					row = append(row, "")
				}
				continue
			}
			row = append(row, htmlCellText(c[3]))
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		return nil
	}
	// pandas: a second header row holding only the index's name, under a
	// first that leaves that column blank.
	if headRows == 2 && len(rows) > 2 && rows[0][0] == "" {
		second := rows[1]
		onlyFirst := len(second) > 0 && second[0] != ""
		for _, c := range second[1:] {
			if c != "" {
				onlyFirst = false
			}
		}
		if onlyFirst {
			rows[0][0] = second[0]
			rows = append(rows[:1], rows[2:]...)
		}
	}
	width := 0
	for _, r := range rows {
		if len(r) > width {
			width = len(r)
		}
	}
	if width == 0 || width > 60 {
		return nil
	}
	return rows
}

func htmlCellText(s string) string {
	s = tagRe.ReplaceAllString(s, "")
	return oneLine(html.UnescapeString(s))
}

// ----------------------------------------------------------------------------
// Markdown
// ----------------------------------------------------------------------------

var (
	mdHeadingRe  = regexp.MustCompile(`^ {0,3}(#{1,6})\s+(.*?)\s*#*\s*$`)
	mdSetext1Re  = regexp.MustCompile(`^ {0,3}=+\s*$`)
	mdSetext2Re  = regexp.MustCompile(`^ {0,3}-+\s*$`)
	mdFenceRe    = regexp.MustCompile("^(\\s*)(```+|~~~+)\\s*([^`\\s{]*)")
	mdBulletRe   = regexp.MustCompile(`^(\s*)([-*+])\s+(.*)$`)
	mdNumberRe   = regexp.MustCompile(`^(\s*)(\d+)[.)]\s+(.*)$`)
	mdRuleRe     = regexp.MustCompile(`^ {0,3}(?:(?:-\s*){3,}|(?:\*\s*){3,}|(?:_\s*){3,})$`)
	mdQuoteRe    = regexp.MustCompile(`^ {0,3}> ?(.*)$`)
	mdTableSepRe = regexp.MustCompile(`^\s*\|?\s*:?-+:?\s*(\|\s*:?-+:?\s*)*\|?\s*$`)
	mdHtmlRe     = regexp.MustCompile(`(?i)^\s*<(/?)([a-z][a-z0-9]*)\b[^>]*>`)
	mdImgTagRe   = regexp.MustCompile(`(?i)^\s*<img\b[^>]*\bsrc\s*=\s*["']([^"']+)["'][^>]*/?>\s*$`)
	mdMathOpenRe = regexp.MustCompile(`^\s*(\$\$|\\\[|\\begin\{([a-zA-Z*]+)\})`)
	mdAnchorRe   = regexp.MustCompile(`(?i)<a\s+(?:name|id|class)\s*=[^>]*>\s*</a>`)
)

type mdContext struct {
	// How many levels headings move up.
	shift int
	// attachment:name -> where it was saved, relative to the org file.
	attachments map[string]string
}

func firstMarkdownHeading(src string) (int, string, bool) {
	lines := strings.Split(src, "\n")
	for i, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		if m := mdHeadingRe.FindStringSubmatch(l); m != nil {
			return len(m[1]), strings.TrimSpace(stripInlineMarkdown(mdAnchorRe.ReplaceAllString(m[2], ""))), true
		}
		if i+1 < len(lines) && mdSetext1Re.MatchString(lines[i+1]) {
			return 1, strings.TrimSpace(stripInlineMarkdown(l)), true
		}
		return 0, "", false
	}
	return 0, "", false
}

// The heading text as plain words, for a title.
func stripInlineMarkdown(s string) string {
	s = mdLinkRe.ReplaceAllString(s, "$1")
	return strings.NewReplacer("**", "", "__", "", "`", "", "*", "").Replace(s)
}

func dropFirstHeading(src string) string {
	lines := strings.Split(src, "\n")
	for i, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		if mdHeadingRe.MatchString(l) {
			return strings.Join(lines[i+1:], "\n")
		}
		if i+1 < len(lines) && mdSetext1Re.MatchString(lines[i+1]) {
			return strings.Join(lines[i+2:], "\n")
		}
		break
	}
	return src
}

func markdownHeadingLevels(src string) []int {
	out := []int{}
	inFence := false
	lines := strings.Split(src, "\n")
	for i, l := range lines {
		if mdFenceRe.MatchString(l) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if m := mdHeadingRe.FindStringSubmatch(l); m != nil {
			out = append(out, len(m[1]))
		} else if strings.TrimSpace(l) != "" && i+1 < len(lines) && !mdBulletRe.MatchString(l) {
			if mdSetext1Re.MatchString(lines[i+1]) {
				out = append(out, 1)
			}
		}
	}
	return out
}

// markdownToOrg converts the markdown of a cell, block by block. It is not a
// complete markdown reader: it is right about what notebooks are written
// with, and leaves alone what it does not know rather than guessing.
func markdownToOrg(src string, ctx mdContext) []string {
	lines := strings.Split(strings.ReplaceAll(strings.TrimRight(src, "\n"), "\t", "    "), "\n")
	out := []string{}
	inList := false
	prevBlank := true
	for i := 0; i < len(lines); i++ {
		l := lines[i]
		t := strings.TrimSpace(l)
		if t == "" {
			out = append(out, "")
			prevBlank = true
			continue
		}
		// Fenced code.
		if m := mdFenceRe.FindStringSubmatch(l); m != nil {
			fence := m[2]
			lang := strings.ToLower(m[3])
			body := []string{}
			j := i + 1
			for ; j < len(lines); j++ {
				if strings.HasPrefix(strings.TrimSpace(lines[j]), fence[:3]) && strings.Trim(strings.TrimSpace(lines[j]), string(fence[0])) == "" {
					break
				}
				body = append(body, strings.TrimPrefix(lines[j], m[1]))
			}
			i = j
			text := strings.Join(body, "\n")
			if lang == "" || lang == "text" || lang == "console" || lang == "output" {
				out = append(out, "#+BEGIN_EXAMPLE")
				out = append(out, escapeBlockLines(text)...)
				out = append(out, "#+END_EXAMPLE")
			} else {
				out = append(out, "#+BEGIN_SRC "+mdLang(lang))
				out = append(out, escapeBlockLines(text)...)
				out = append(out, "#+END_SRC")
			}
			prevBlank = false
			continue
		}
		// Display maths, kept as written until it closes.
		if m := mdMathOpenRe.FindStringSubmatch(l); m != nil {
			closer := "$$"
			switch {
			case m[1] == `\[`:
				closer = `\]`
			case m[2] != "":
				closer = `\end{` + m[2] + `}`
			}
			out = append(out, l)
			after := strings.SplitN(t, m[1], 2)[1]
			for !strings.Contains(after, closer) && i+1 < len(lines) {
				i++
				out = append(out, lines[i])
				if strings.Contains(lines[i], closer) {
					break
				}
			}
			prevBlank = false
			continue
		}
		// Headings.
		if m := mdHeadingRe.FindStringSubmatch(l); m != nil {
			lvl := len(m[1]) - ctx.shift
			if lvl < 1 {
				lvl = 1
			}
			out = append(out, strings.Repeat("*", lvl)+" "+mdInline(mdAnchorRe.ReplaceAllString(m[2], ""), ctx))
			inList, prevBlank = false, false
			continue
		}
		if !inList && i+1 < len(lines) && (mdSetext1Re.MatchString(lines[i+1]) || (mdSetext2Re.MatchString(lines[i+1]) && prevBlank)) && !mdTableRow(l) {
			lvl := 1
			if mdSetext2Re.MatchString(lines[i+1]) {
				lvl = 2
			}
			lvl -= ctx.shift
			if lvl < 1 {
				lvl = 1
			}
			out = append(out, strings.Repeat("*", lvl)+" "+mdInline(t, ctx))
			i++
			prevBlank = false
			continue
		}
		if mdRuleRe.MatchString(l) {
			out = append(out, "-----")
			inList, prevBlank = false, false
			continue
		}
		// Block quotes, converted inside.
		if mdQuoteRe.MatchString(l) {
			inner := []string{}
			for ; i < len(lines); i++ {
				m := mdQuoteRe.FindStringSubmatch(lines[i])
				if m == nil {
					if strings.TrimSpace(lines[i]) == "" {
						break
					}
					inner = append(inner, lines[i]) // a lazy continuation
					continue
				}
				inner = append(inner, m[1])
			}
			i--
			out = append(out, "#+BEGIN_QUOTE")
			out = append(out, markdownToOrg(strings.Join(inner, "\n"), ctx)...)
			out = append(out, "#+END_QUOTE")
			prevBlank = false
			continue
		}
		// Tables: a header row with a rule under it.
		if mdTableRow(l) && i+1 < len(lines) && mdTableSepRe.MatchString(lines[i+1]) && strings.Contains(lines[i+1], "-") {
			rows := [][]string{mdTableCells(l, ctx), nil}
			i += 2
			for ; i < len(lines) && mdTableRow(lines[i]); i++ {
				rows = append(rows, mdTableCells(lines[i], ctx))
			}
			i--
			out = append(out, strings.Split(orgTableText(rows), "\n")...)
			prevBlank = false
			continue
		}
		// Lists.
		if m := mdBulletRe.FindStringSubmatch(l); m != nil && !mdRuleRe.MatchString(l) {
			out = append(out, m[1]+"- "+mdInline(m[3], ctx))
			inList, prevBlank = true, false
			continue
		}
		if m := mdNumberRe.FindStringSubmatch(l); m != nil {
			out = append(out, m[1]+m[2]+". "+mdInline(m[3], ctx))
			inList, prevBlank = true, false
			continue
		}
		// Code indented four spaces, outside a list.
		if prevBlank && !inList && strings.HasPrefix(l, "    ") {
			body := []string{}
			for ; i < len(lines) && (strings.HasPrefix(lines[i], "    ") || strings.TrimSpace(lines[i]) == ""); i++ {
				body = append(body, strings.TrimPrefix(lines[i], "    "))
			}
			i--
			for len(body) > 0 && strings.TrimSpace(body[len(body)-1]) == "" {
				body = body[:len(body)-1]
			}
			out = append(out, "#+BEGIN_EXAMPLE")
			out = append(out, escapeBlockLines(strings.Join(body, "\n"))...)
			out = append(out, "#+END_EXAMPLE", "")
			prevBlank = true
			continue
		}
		// A picture on its own in an <img> tag.
		if m := mdImgTagRe.FindStringSubmatch(l); m != nil {
			out = append(out, mdImageLink(html.UnescapeString(m[1]), ctx))
			prevBlank = false
			continue
		}
		// Html that is not a picture: as it is, for the html exporter, up to
		// the next blank line.
		if m := mdHtmlRe.FindStringSubmatch(l); m != nil && blockTag[strings.ToLower(m[2])] {
			block := []string{}
			for ; i < len(lines) && strings.TrimSpace(lines[i]) != ""; i++ {
				block = append(block, lines[i])
			}
			i--
			text := mdAnchorRe.ReplaceAllString(strings.Join(block, "\n"), "")
			if plain, ok := simpleHtml(text); ok {
				for _, pl := range strings.Split(plain, "\n") {
					if strings.TrimSpace(pl) != "" {
						out = append(out, mdInline(strings.TrimSpace(pl), ctx))
					}
				}
			} else if strings.TrimSpace(text) != "" {
				out = append(out, "#+BEGIN_EXPORT html")
				out = append(out, escapeBlockLines(text)...)
				out = append(out, "#+END_EXPORT")
			}
			prevBlank = false
			continue
		}
		// A paragraph line (or a list item's continuation).
		if prevBlank && !strings.HasPrefix(l, " ") {
			inList = false
		}
		line := mdInline(l, ctx)
		// A line that would start something else in org is pushed off it.
		if lt := strings.TrimLeft(line, " "); strings.HasPrefix(lt, "* ") || strings.HasPrefix(lt, "#+") || strings.HasPrefix(lt, ": ") {
			line = strings.Repeat(" ", len(line)-len(lt)) + "​" + lt
		}
		out = append(out, line)
		prevBlank = false
	}
	// Blank lines at either end are the cell's edges, which the file supplies.
	for len(out) > 0 && out[0] == "" {
		out = out[1:]
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	// Two blank lines in a row end a list in org; one is enough between
	// paragraphs.
	squeezed := []string{}
	for _, l := range out {
		if l == "" && len(squeezed) > 0 && squeezed[len(squeezed)-1] == "" {
			continue
		}
		squeezed = append(squeezed, l)
	}
	return squeezed
}

// Html made only of the tags that dress text up - <b>, <font color=...>,
// <center>, <br> - as the text, in org's emphasis where it has one. Not ok
// when anything else is in it.
var dressTagRe = regexp.MustCompile(`(?i)</?(b|strong|i|em|u|font|span|center|p|br|small|big|sup|sub)\b[^>]*>`)

func simpleHtml(h string) (string, bool) {
	rest := dressTagRe.ReplaceAllString(h, "")
	if strings.Contains(rest, "<") {
		return "", false
	}
	out := dressTagRe.ReplaceAllStringFunc(h, func(tag string) string {
		name := strings.ToLower(strings.Trim(strings.Fields(strings.Trim(tag, "</>"))[0], "/"))
		switch name {
		case "b", "strong":
			return "**"
		case "i", "em":
			return "*"
		case "br", "p":
			return "\n"
		}
		return ""
	})
	return html.UnescapeString(out), true
}

var blockTag = map[string]bool{
	"div": true, "table": true, "p": true, "center": true, "details": true, "summary": true,
	"iframe": true, "video": true, "audio": true, "figure": true, "font": true, "span": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true, "ul": true, "ol": true,
	"pre": true, "style": true, "script": true, "section": true, "blockquote": true, "hr": true, "br": true,
}

func mdLang(l string) string {
	switch l {
	case "py", "python3", "ipython", "ipython3":
		return "python"
	case "bash", "shell", "console", "zsh":
		return "sh"
	case "r":
		return "R"
	}
	return l
}

func mdTableRow(l string) bool {
	t := strings.TrimSpace(l)
	return strings.HasPrefix(t, "|") || (strings.Count(t, "|") >= 1 && !strings.HasPrefix(t, "-"))
}

func mdTableCells(l string, ctx mdContext) []string {
	t := strings.TrimSpace(l)
	t = strings.TrimPrefix(t, "|")
	if strings.HasSuffix(t, "|") && !strings.HasSuffix(t, `\|`) {
		t = t[:len(t)-1]
	}
	cells := []string{}
	cur := strings.Builder{}
	inCode := false
	for i := 0; i < len(t); i++ {
		ch := t[i]
		switch {
		case ch == '\\' && i+1 < len(t) && t[i+1] == '|':
			cur.WriteString(`\vert{}`)
			i++
		case ch == '`':
			inCode = !inCode
			cur.WriteByte(ch)
		case ch == '|' && !inCode:
			cells = append(cells, mdInline(strings.TrimSpace(cur.String()), ctx))
			cur.Reset()
		default:
			cur.WriteByte(ch)
		}
	}
	cells = append(cells, mdInline(strings.TrimSpace(cur.String()), ctx))
	for i, c := range cells {
		cells[i] = strings.ReplaceAll(c, "|", `\vert{}`)
	}
	return cells
}

var (
	mdCodeRe     = regexp.MustCompile("(`+)(.+?)(`+)")
	mdMathRe     = regexp.MustCompile(`\$\$[^$]+\$\$|\$[^$\s][^$]*?\$|\\\(.+?\\\)`)
	mdImageRe    = regexp.MustCompile(`!\[([^\]]*)\]\(\s*<?([^)\s>]+)>?(?:\s+"[^"]*")?\s*\)`)
	mdLinkRe     = regexp.MustCompile(`\[([^\]]+)\]\(\s*<?([^)\s>]+)>?(?:\s+"[^"]*")?\s*\)`)
	mdAutoLinkRe = regexp.MustCompile(`<(https?://[^>\s]+)>`)
	mdBoldRe     = regexp.MustCompile(`\*\*([^*\s](?:[^*]*[^*\s])?)\*\*|__([^_\s](?:[^_]*[^_\s])?)__`)
	mdItalicRe   = regexp.MustCompile(`(^|[^\w*])\*([^*\s](?:[^*]*[^*\s])?)\*([^\w*]|$)|(^|[^\w])_([^_\s](?:[^_]*[^_\s])?)_([^\w]|$)`)
	mdStrikeRe   = regexp.MustCompile(`~~([^~]+)~~`)
	mdBrRe       = regexp.MustCompile(`(?i)\s*<br\s*/?>\s*$`)
	mdImgInRe    = regexp.MustCompile(`(?i)<img\b[^>]*\bsrc\s*=\s*["']([^"']+)["'][^>]*/?>`)
)

// The inline markdown of one line as org: code and maths are kept away from
// everything else first, so a * in either is not emphasis.
func mdInline(s string, ctx mdContext) string {
	held := []string{}
	hold := func(v string) string {
		held = append(held, v)
		return fmt.Sprintf("\x00%d\x00", len(held)-1)
	}
	s = mdAnchorRe.ReplaceAllString(s, "")
	s = mdCodeRe.ReplaceAllStringFunc(s, func(m string) string {
		p := mdCodeRe.FindStringSubmatch(m)
		if len(p[1]) != len(p[3]) {
			return m
		}
		code := strings.TrimSpace(p[2])
		mark := "~"
		if strings.Contains(code, "~") {
			mark = "="
		}
		return hold(mark + code + mark)
	})
	s = mdMathRe.ReplaceAllStringFunc(s, hold)
	s = mdImageRe.ReplaceAllStringFunc(s, func(m string) string {
		p := mdImageRe.FindStringSubmatch(m)
		return hold(mdImageLink(p[2], ctx))
	})
	s = mdImgInRe.ReplaceAllStringFunc(s, func(m string) string {
		p := mdImgInRe.FindStringSubmatch(m)
		return hold(mdImageLink(html.UnescapeString(p[1]), ctx))
	})
	s = mdLinkRe.ReplaceAllStringFunc(s, func(m string) string {
		p := mdLinkRe.FindStringSubmatch(m)
		desc := mdInline(p[1], ctx)
		return hold("[[" + mdTarget(p[2]) + "][" + desc + "]]")
	})
	s = mdAutoLinkRe.ReplaceAllStringFunc(s, func(m string) string {
		return hold("[[" + strings.Trim(m, "<>") + "]]")
	})
	s = mdBoldRe.ReplaceAllStringFunc(s, func(m string) string {
		p := mdBoldRe.FindStringSubmatch(m)
		inner := p[1] + p[2]
		return "\x01" + inner + "\x01"
	})
	// Twice, because neighbouring emphasis shares the character between them.
	for k := 0; k < 2; k++ {
		s = mdItalicRe.ReplaceAllStringFunc(s, func(m string) string {
			p := mdItalicRe.FindStringSubmatch(m)
			if p[2] != "" {
				return p[1] + "/" + p[2] + "/" + p[3]
			}
			return p[4] + "/" + p[5] + "/" + p[6]
		})
	}
	s = strings.ReplaceAll(s, "\x01", "*")
	s = mdStrikeRe.ReplaceAllString(s, "+$1+")
	s = mdBrRe.ReplaceAllString(s, ` \\`)
	for i := len(held) - 1; i >= 0; i-- {
		s = strings.ReplaceAll(s, fmt.Sprintf("\x00%d\x00", i), held[i])
	}
	return s
}

// A link target as org writes it: a url as it is, a file path with file:, a
// notebook beside this one as the org file it will become.
func mdTarget(u string) string {
	switch {
	case strings.Contains(u, "://") || strings.HasPrefix(u, "mailto:"):
		return u
	case strings.HasPrefix(u, "#"):
		return u
	}
	path, frag, _ := strings.Cut(u, "#")
	if strings.HasSuffix(strings.ToLower(path), ".ipynb") {
		path = path[:len(path)-len(".ipynb")] + ".org"
	}
	if frag != "" {
		return "file:" + path + "::" + frag
	}
	return "file:" + path
}

func mdImageLink(src string, ctx mdContext) string {
	if strings.HasPrefix(src, "attachment:") {
		if p, ok := ctx.attachments[strings.TrimPrefix(src, "attachment:")]; ok {
			return "[[file:" + p + "]]"
		}
	}
	if strings.Contains(src, "://") || strings.HasPrefix(src, "data:") {
		return "[[" + src + "]]"
	}
	return "[[file:" + src + "]]"
}
