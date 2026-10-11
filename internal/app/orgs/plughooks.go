package orgs

// What the server lends a plugin that writes into org files.
//
// A poller that mirrors issues into a file has to change a heading's state
// now and then (an issue closed upstream becomes DONE), and that change has
// to mean what it means everywhere else: CLOSED stamped, the state logged per
// the file's logging settings, a repeater moved on. That is applyStatusChange,
// which lives here, and a plugin cannot import this package - this package
// imports the plugins. So the manager carries it as a function.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ihdavids/orgs/internal/common"
)

func installPluginHooks(m *common.PluginManager) {
	if m == nil {
		return
	}
	m.ChangeStatus = pluginChangeStatus
	m.WriteLines = pluginWriteLines
	m.Deliver = deliverNote
}

// The file the database holds under this name, only when it really is this
// file. FindByFile falls back to matching the base name, which is right for a
// person typing "gtd.org" and wrong here: a mirror file outside the org
// directories that shares a name with one inside must not have its edits
// measured against the other file's parse.
func loadedFileAt(filename string) *common.OrgFile {
	f := GetDb().FindByFile(filename)
	if f == nil || !samePath(f.Filename, filename) {
		return nil
	}
	return f
}

func samePath(a, b string) bool {
	norm := func(p string) string {
		if abs, err := filepath.Abs(p); err == nil {
			p = abs
		}
		if real, err := filepath.EvalSymlinks(p); err == nil {
			p = real
		}
		return p
	}
	return norm(a) == norm(b)
}

func pluginChangeStatus(filename string, row int, to string) (string, error) {
	if loadedFileAt(filename) == nil {
		return "", fmt.Errorf("%s is not a file the server has loaded (is it under an org directory?)", filename)
	}
	// Read it again first: the row comes from the file as it is on disk, and
	// the parse could be a save behind if the watcher has not caught up.
	f := GetDb().ReloadFile(filename)
	if f == nil {
		return "", fmt.Errorf("could not read %s", filename)
	}
	sec := sectionAtRow(f, row)
	if sec == nil || headlineRow(sec) != row {
		return "", fmt.Errorf("%s:%d is not a heading", filename, row+1)
	}
	res, err := applyStatusChange(f, sec, to, "", time.Now())
	return res.Status, err
}

func pluginWriteLines(filename string, lines []string) error {
	if loadedFileAt(filename) != nil {
		return writeLines(filename, lines)
	}
	// Not one of ours: write it and let the watcher pick it up if it should.
	return os.WriteFile(filename, []byte(strings.Join(lines, "\n")+"\n"), 0644)
}
