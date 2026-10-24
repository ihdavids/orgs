package orgs

// Writing a setting back into the server's own yaml file. Only the lines a
// setting lives on change: the rest of the file - comments, order, spacing -
// stays, which re-marshalling the Config would lose.

import (
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// editConfigFile runs edit over the lines of the file the server was started
// from and writes the result whole or not at all, after keeping the file as it
// was in <file><backup>. The status is the one to answer with when err is set:
// 400 for what the writer can fix (no file, an edit that does not fit), 500
// for the disk.
func editConfigFile(backup string, edit func([]string) ([]string, error)) (string, int, error) {
	file := Conf().loadedFrom
	if file == "" {
		return "", http.StatusBadRequest, fmt.Errorf("this server was not started from a configuration file, so there is nothing to write it into")
	}
	st, err := os.Stat(file)
	if err != nil {
		return file, http.StatusInternalServerError, err
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return file, http.StatusInternalServerError, err
	}
	text := string(data)
	nl := "\n"
	if strings.Contains(text, "\r\n") {
		nl = "\r\n"
	}
	lines, err := edit(strings.Split(strings.TrimSuffix(text, nl), nl))
	if err != nil {
		return file, http.StatusBadRequest, err
	}
	if err := os.WriteFile(file+backup, data, st.Mode().Perm()); err != nil {
		return file, http.StatusInternalServerError, fmt.Errorf("keeping a copy: %s", err)
	}
	tmp := file + ".writing"
	if err := os.WriteFile(tmp, []byte(strings.Join(lines, nl)+nl), st.Mode().Perm()); err != nil {
		return file, http.StatusInternalServerError, err
	}
	if err := os.Rename(tmp, file); err != nil {
		os.Remove(tmp)
		return file, http.StatusInternalServerError, err
	}
	return file, 0, nil
}

func yamlIndent(l string) int { return len(l) - len(strings.TrimLeft(l, " ")) }

// A line that says nothing to yaml: blank or a comment.
func yamlQuiet(l string) bool {
	t := strings.TrimSpace(l)
	return t == "" || strings.HasPrefix(t, "#")
}

// setYamlPath sets `a: b: c: "value"` in block-style yaml, path {"a", "b",
// "c"}: the leaf's line is replaced, or added at the end of its section, and
// a section that is not there yet is added the same way, indented as its
// siblings are (two spaces past its parent when it has none). A section
// written inline (`assistant: {backend: claude}`) is refused rather than
// guessed at.
func setYamlPath(lines []string, path []string, value string) ([]string, error) {
	out := append([]string{}, lines...)
	start, end, indent := 0, len(out), -1 // the section's body, and its own indent (-1: the file)
	for depth, key := range path {
		// The children's indent: the first line in the section that says something.
		child := indent + 2
		if indent < 0 {
			child = 0
		}
		for i := start; i < end; i++ {
			if !yamlQuiet(out[i]) {
				child = yamlIndent(out[i])
				break
			}
		}
		keyRe := regexp.MustCompile(`^ {` + strconv.Itoa(child) + `}` + regexp.QuoteMeta(key) + `\s*:(.*)$`)
		at := -1
		rest := ""
		for i := start; i < end; i++ {
			if m := keyRe.FindStringSubmatch(out[i]); m != nil {
				at, rest = i, strings.TrimSpace(m[1])
				break
			}
		}
		pad := strings.Repeat(" ", child)
		leaf := depth == len(path)-1
		if leaf {
			line := pad + key + ": " + strconv.Quote(value)
			if at >= 0 {
				out[at] = line
				return out, nil
			}
			return insertLine(out, sectionTail(out, start, end), line), nil
		}
		if at < 0 {
			at = sectionTail(out, start, end)
			out = insertLine(out, at, pad+key+":")
			end++
		} else if rest != "" && !strings.HasPrefix(rest, "#") {
			return nil, fmt.Errorf("%s is written inline (%s: %s); change it by hand", strings.Join(path[:depth+1], "."), key, rest)
		}
		// The section's body runs to the next line that says something at
		// its own indent or less.
		start, indent = at+1, child
		for i := start; i < end; i++ {
			if !yamlQuiet(out[i]) && yamlIndent(out[i]) <= indent {
				end = i
				break
			}
		}
	}
	return out, nil
}

// Where a new line goes in a section: after its last line that says something,
// so the comments heading the next section stay with it.
func sectionTail(lines []string, start, end int) int {
	at := start
	for i := start; i < end; i++ {
		if !yamlQuiet(lines[i]) {
			at = i + 1
		}
	}
	return at
}

func insertLine(lines []string, at int, line string) []string {
	out := append([]string{}, lines[:at]...)
	out = append(out, line)
	return append(out, lines[at:]...)
}
