package orgs

// Org Plot (org manual 3.6): a #+PLOT: line above a table says how to chart
// it. worg's Tables tab opens the chart that line describes (TableData.Plot);
// an export draws it with gnuplot, as org-plot does, into the line's file:
// and puts a link to the picture under the table in the copy every export
// reads (exportLinesFor), never in the file.
//
//	#+PLOT: title:"Sales" ind:1 deps:(2 3) type:2d with:lines file:"sales.png"
//	#+PLOT: type:2d with:histograms set:"yrange [0:]" file:"plots/h.svg"
//	#+PLOT: type:grid file:"grid.png"      (3d draws a surface, grid an image)
//
// Drawing runs gnuplot, whose set: lines can do anything a shell can, so it
// happens only with babel.enable (and gnuplot allowed in babel.languages),
// the same promise running a block is. A plot with no file: is left to
// worg's chart; one whose data and options have not changed since it was last
// drawn is not drawn again.

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var plotLineRe = regexp.MustCompile(`(?i)^\s*#\+PLOT:\s*(.*)$`)

// plotLineAbove is the options of a #+PLOT: line among the keywords right
// above row (a table's first line).
func plotLineAbove(lines []string, row int) string {
	for r := row - 1; r >= 0 && r < len(lines); r-- {
		t := strings.TrimSpace(lines[r])
		if !strings.HasPrefix(t, "#+") {
			break
		}
		if m := plotLineRe.FindStringSubmatch(lines[r]); m != nil {
			return strings.TrimSpace(m[1])
		}
	}
	return ""
}

// plotOptions reads org-plot's key:value options; values may be "quoted" or
// (lists), and set: may come more than once.
func plotOptions(s string) map[string][]string {
	out := map[string][]string{}
	for i := 0; i < len(s); {
		for i < len(s) && s[i] == ' ' {
			i++
		}
		colon := strings.IndexByte(s[i:], ':')
		if colon <= 0 {
			break
		}
		key := strings.ToLower(strings.TrimSpace(s[i : i+colon]))
		i += colon + 1
		start := i
		switch {
		case i < len(s) && s[i] == '"':
			end := strings.IndexByte(s[i+1:], '"')
			if end < 0 {
				end = len(s) - i - 1
			}
			out[key] = append(out[key], s[i+1:i+1+end])
			i += end + 2
			continue
		case i < len(s) && s[i] == '(':
			depth := 0
			for i < len(s) {
				if s[i] == '(' {
					depth++
				} else if s[i] == ')' {
					depth--
					if depth == 0 {
						i++
						break
					}
				}
				i++
			}
		default:
			for i < len(s) && s[i] != ' ' {
				i++
			}
		}
		out[key] = append(out[key], s[start:min(i, len(s))])
	}
	return out
}

func plotList(v string) []string {
	v = strings.Trim(strings.TrimSpace(v), "()")
	out := []string{}
	for _, w := range strings.Fields(v) {
		out = append(out, strings.Trim(w, `"`))
	}
	return out
}

// plotTableRows are a table's data rows and its column names (the row above
// its first rule), read off its lines.
func plotTableRows(lines []string, start int) (rows [][]string, names []string, end int) {
	all := [][]string{}
	ruleAfter := -1
	end = start
	for r := start; r < len(lines); r++ {
		t := strings.TrimSpace(lines[r])
		if !strings.HasPrefix(t, "|") {
			break
		}
		end = r
		if strings.HasPrefix(t, "|-") {
			if ruleAfter < 0 {
				ruleAfter = len(all)
			}
			continue
		}
		cells := strings.Split(strings.Trim(t, "|"), "|")
		for i := range cells {
			cells[i] = strings.TrimSpace(cells[i])
		}
		all = append(all, cells)
	}
	if ruleAfter == 1 && len(all) > 1 {
		return all[1:], all[0], end
	}
	return all, nil, end
}

// plotScript is org-plot's gnuplot script for a table written to data.
func plotScript(opts map[string][]string, data, out string, names []string, rows [][]string) string {
	first := func(k, def string) string {
		if v := opts[k]; len(v) > 0 && v[len(v)-1] != "" {
			return v[len(v)-1]
		}
		return def
	}
	var b strings.Builder
	format := strings.TrimPrefix(strings.ToLower(filepath.Ext(out)), ".")
	term := map[string]string{"svg": "svg", "pdf": "pdfcairo", "eps": "postscript eps", "jpg": "jpeg", "jpeg": "jpeg", "gif": "gif"}[format]
	if term == "" {
		term = "png"
	}
	fmt.Fprintf(&b, "set terminal %s\nset output %s\nset datafile separator \"\\t\"\n", term, quoteDouble(out))
	if t := first("title", ""); t != "" {
		fmt.Fprintf(&b, "set title %s\n", quoteDouble(t))
	}
	for _, s := range opts["set"] {
		b.WriteString("set " + s + "\n")
	}
	kind := first("type", "2d")
	with := first("with", "lines")
	switch kind {
	case "3d":
		fmt.Fprintf(&b, "splot %s matrix with %s title ''\n", quoteDouble(data), first("with", "pm3d"))
		return b.String()
	case "grid":
		fmt.Fprintf(&b, "plot %s matrix with image title ''\n", quoteDouble(data))
		return b.String()
	}
	cols := 0
	if len(rows) > 0 {
		cols = len(rows[0])
	}
	ind, _ := strconv.Atoi(first("ind", "0"))
	deps := []int{}
	for _, d := range plotList(first("deps", "")) {
		if n, err := strconv.Atoi(d); err == nil {
			deps = append(deps, n)
		}
	}
	if len(deps) == 0 {
		for c := 1; c <= cols; c++ {
			if c != ind {
				deps = append(deps, c)
			}
		}
	}
	// A text column as the independent one is the axis's labels.
	textInd := false
	if ind > 0 {
		for _, r := range rows {
			if ind <= len(r) && r[ind-1] != "" {
				if _, err := strconv.ParseFloat(r[ind-1], 64); err != nil {
					textInd = true
				}
				break
			}
		}
	}
	if with == "histograms" {
		b.WriteString("set style data histograms\nset style fill solid border -1\n")
	}
	parts := []string{}
	for _, d := range deps {
		title := ""
		if d-1 < len(names) {
			title = names[d-1]
		}
		using := strconv.Itoa(d)
		switch {
		case with == "histograms" && ind > 0:
			using = fmt.Sprintf("%d:xtic(%d)", d, ind)
		case with == "histograms":
		case ind > 0 && textInd:
			using = fmt.Sprintf("%d:xticlabels(%d)", d, ind)
		case ind > 0:
			using = fmt.Sprintf("%d:%d", ind, d)
		}
		part := fmt.Sprintf("%s using %s", quoteDouble(data), using)
		if with != "histograms" {
			part += " with " + with
		}
		parts = append(parts, part+" title "+quoteDouble(title))
	}
	b.WriteString("plot " + strings.Join(parts, ", ") + "\n")
	return b.String()
}

var plotDrawn = struct {
	sync.Mutex
	m map[string]string
}{m: map[string]string{}}

// plotExportLines draws every #+PLOT: with a file: under the org directories
// and puts a link to it under its table, unless the lines after the table
// already link it. changed is false when nothing was added.
func plotExportLines(path string, lines []string) ([]string, bool) {
	if _, err := babelAllows("gnuplot"); err != nil {
		return lines, false
	}
	out := []string{}
	changed := false
	for i := 0; i < len(lines); i++ {
		out = append(out, lines[i])
		m := plotLineRe.FindStringSubmatch(lines[i])
		if m == nil {
			continue
		}
		start := i + 1
		for start < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[start]), "#+") {
			start++
		}
		if start >= len(lines) || !strings.HasPrefix(strings.TrimSpace(lines[start]), "|") {
			continue
		}
		opts := plotOptions(m[1])
		file := ""
		if v := opts["file"]; len(v) > 0 {
			file = v[len(v)-1]
		}
		if file == "" {
			continue
		}
		abs := file
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(filepath.Dir(path), file)
		}
		if _, ok := insideOrgDirs(abs); !ok {
			fmt.Fprintf(os.Stderr, "plot: %s: %s is not under the org directories; not drawn\n", filepath.Base(path), file)
			continue
		}
		rows, names, end := plotTableRows(lines, start)
		if err := drawPlot(opts, abs, names, rows); err != nil {
			fmt.Fprintf(os.Stderr, "plot: %s: %v\n", filepath.Base(path), err)
			continue
		}
		// Copy up to the table's end, then the link, unless one is there.
		for j := i + 1; j <= end; j++ {
			out = append(out, lines[j])
		}
		i = end
		linked := false
		for j := end + 1; j < len(lines) && j <= end+2; j++ {
			if strings.Contains(lines[j], file) {
				linked = true
			}
		}
		if !linked {
			out = append(out, "", "[[file:"+file+"]]")
			changed = true
		}
	}
	return out, changed
}

// drawPlot runs gnuplot for one table, unless what it would draw is what was
// drawn there last.
func drawPlot(opts map[string][]string, out string, names []string, rows [][]string) error {
	tmp, err := os.MkdirTemp("", "orgsplot")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	data := filepath.Join(tmp, "data.tsv")
	var b strings.Builder
	for _, r := range rows {
		b.WriteString(strings.Join(r, "\t") + "\n")
	}
	if err := os.WriteFile(data, []byte(b.String()), 0o600); err != nil {
		return err
	}
	script := plotScript(opts, data, out, names, rows)
	sum := sha1.Sum([]byte(strings.ReplaceAll(script, tmp, "") + b.String()))
	key := hex.EncodeToString(sum[:])
	plotDrawn.Lock()
	same := plotDrawn.m[out] == key
	plotDrawn.Unlock()
	if same {
		if _, err := os.Stat(out); err == nil {
			return nil
		}
	}
	r, err := babelAllows("gnuplot")
	if err != nil {
		return err
	}
	os.MkdirAll(filepath.Dir(out), 0o755)
	r.picture = nil
	ran, err := runProgram(r, script, tmp, 30*time.Second, "", "")
	if err != nil {
		return err
	}
	if ran.code != 0 {
		return fmt.Errorf("gnuplot: %s", strings.TrimSpace(ran.stderr))
	}
	plotDrawn.Lock()
	plotDrawn.m[out] = key
	plotDrawn.Unlock()
	return nil
}
