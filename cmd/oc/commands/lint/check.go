package lint

// Check is the lint as a function, for `orgs lsp`: the lines of one file in,
// problems with their extent out, and every failure an error rather than an
// exit.

import (
	"strconv"
	"strings"

	"github.com/ihdavids/orgs/cmd/oc/commands"
)

// Problem is one lint. Lines are 0 based, columns 0 based bytes; End is just
// past the problem.
type Problem struct {
	Line, Col       int
	EndLine, EndCol int
	Severity        string
	Rule            string
	Msg             string
	Fix             []string
	Checker         string
}

// Options are the lint command's flags.
type Options struct {
	Lang    string
	Dialect string
	Ignore  []string
}

// Check runs the named checkers (from Checkers) over the prose of lines.
func Check(core *commands.Core, lines []string, checkers []string, opt Options) ([]Problem, error) {
	if opt.Lang == "" {
		opt.Lang = "en-US"
	}
	if opt.Dialect == "" {
		opt.Dialect = "american"
	}
	ignore := map[string]bool{}
	for _, r := range opt.Ignore {
		ignore[strings.ToLower(r)] = true
	}
	paras := paragraphs(lines, 0, len(lines))
	if len(paras) == 0 {
		return nil, nil
	}
	out := []Problem{}
	seen := map[string]bool{}
	for _, name := range checkers {
		var res [][]found
		var err error
		switch name {
		case "harper":
			res, err = runHarper(core, paras, opt.Dialect, opt.Ignore)
		case "languagetool":
			res, err = runLanguageTool(core, paras, opt.Lang, opt.Ignore)
		}
		if err != nil {
			return out, err
		}
		for _, fs := range res {
			for _, f := range fs {
				if ignore[strings.ToLower(f.rule)] {
					continue
				}
				// Both checkers knowing a misspelling is one problem, as tidy
				// has it for the command.
				k := ""
				if len(f.fix) > 0 {
					k = strings.ToLower(f.fix[0])
				}
				key := strings.Join([]string{strconv.Itoa(f.at.line), strconv.Itoa(f.at.col), k}, ":")
				if k != "" && seen[key] {
					continue
				}
				seen[key] = true
				end := f.end
				if end.line < f.at.line || (end.line == f.at.line && end.col <= f.at.col) {
					end = pos{f.at.line, f.at.col + 1}
				}
				out = append(out, Problem{Line: f.at.line, Col: f.at.col, EndLine: end.line, EndCol: end.col,
					Severity: f.severity, Rule: f.rule, Msg: f.msg, Fix: f.fix, Checker: name})
			}
		}
	}
	return out, nil
}
