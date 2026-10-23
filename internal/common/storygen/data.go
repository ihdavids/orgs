package storygen

// The word tables live in data/*.txt, compiled into the binary, so orgs ships
// as one file and a table is changed by editing text, not Go.
//
// A file holds many tables:
//
//	## table_name
//	# used as: "Wants {}."     (the sentence its entries are dropped into)
//	== all
//	entry
//	== fantasy
//	entry
//
// "== all" entries belong to every genre; a genre's own entries are added to
// them for a result in that genre. Some tables are split by something else
// (seasons, kinds of conflict) and read by that name. Lines starting with #
// are comments; blank lines are ignored.
//
// An entry may end with genre tags, "a crewmate left outside [sf]" or
// "[fantasy, modern]": it is then used only in those genres. It is kept in its
// section under "<section>@<genre>" for each tag, so a table split by season
// or kind can still say which of its entries suit which genre.

import (
	"embed"
	"fmt"
	"path"
	"sort"
	"strings"
	"sync"
)

//go:embed data/*.txt
var dataFS embed.FS

var (
	tables    map[string]gl
	loadErr   error
	loadOnce  sync.Once
	missingMu sync.Mutex
	missing   = map[string]bool{}
)

func loadTables() {
	tables = map[string]gl{}
	files, err := dataFS.ReadDir("data")
	if err != nil {
		loadErr = err
		return
	}
	for _, f := range files {
		b, err := dataFS.ReadFile(path.Join("data", f.Name()))
		if err != nil {
			loadErr = err
			return
		}
		if err := parseTables(f.Name(), string(b), tables); err != nil {
			loadErr = err
			return
		}
	}
}

func parseTables(file, text string, into map[string]gl) error {
	table, section := "", ""
	for i, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "":
		case strings.HasPrefix(line, "## "):
			table, section = strings.TrimSpace(line[3:]), ""
			if _, dup := into[table]; dup {
				return fmt.Errorf("%s:%d: table %s defined twice", file, i+1, table)
			}
			into[table] = gl{}
		case strings.HasPrefix(line, "#"):
		case strings.HasPrefix(line, "== "):
			section = strings.TrimSpace(line[3:])
			if section == "all" {
				section = ""
			}
		case table == "":
			return fmt.Errorf("%s:%d: entry before any table", file, i+1)
		default:
			entry, genres := genreTags(line)
			if genres == nil {
				into[table][section] = append(into[table][section], entry)
				continue
			}
			for _, g := range genres {
				k := section + "@" + g
				into[table][k] = append(into[table][k], entry)
			}
		}
	}
	return nil
}

// genreTags splits "entry [sf, modern]" into the entry and its genres; an
// entry with no tags (or brackets that are not genre names) has none.
func genreTags(line string) (string, []string) {
	if !strings.HasSuffix(line, "]") {
		return line, nil
	}
	i := strings.LastIndex(line, "[")
	if i <= 0 {
		return line, nil
	}
	var gs []string
	for _, g := range strings.Split(line[i+1:len(line)-1], ",") {
		g = strings.TrimSpace(g)
		if !known(g) {
			return line, nil
		}
		gs = append(gs, g)
	}
	return strings.TrimSpace(line[:i]), gs
}

// tab is a table by name. A name no file defines gives an empty table, and
// is remembered so the tests can say which.
func tab(name string) gl {
	loadOnce.Do(loadTables)
	t, ok := tables[name]
	if !ok {
		missingMu.Lock()
		missing[name] = true
		missingMu.Unlock()
		return gl{}
	}
	return t
}

// all is every entry of a table, whatever its section or genre, once each.
func (t gl) all() []string {
	keys := make([]string, 0, len(t))
	for k := range t {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := []string{}
	seen := map[string]bool{}
	for _, k := range keys {
		for _, e := range t[k] {
			if !seen[e] {
				seen[e] = true
				out = append(out, e)
			}
		}
	}
	return out
}

// split2 reads a "value|tag" entry.
func split2(e string) (string, string) {
	a, b, _ := strings.Cut(e, "|")
	return strings.TrimSpace(a), strings.TrimSpace(b)
}

// TableStats says how many entries each table holds, for the curious.
func TableStats() (map[string]int, error) {
	loadOnce.Do(loadTables)
	out := map[string]int{}
	for k, t := range tables {
		out[k] = len(t.all())
	}
	return out, loadErr
}
