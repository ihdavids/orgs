//lint:file-ignore ST1006 allow the use of self
package orgs

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

var headlineRegexp = regexp.MustCompile(`^([*]+)\s+(.*)`)

func FindArchiveTarget(db common.ODb, tgt *common.Target) *common.Target {
	fromFile, fromSecs := db.GetFromTarget(tgt, false)
	if fromFile != nil && fromSecs != nil {
		// Default global setting
		archiveTarget := Conf().ArchiveDefaultTarget
		// File level properties
		if at := fromFile.Doc.Get("ARCHIVE"); at != "" {
			archiveTarget = at
		}
		// Node level properties
		if at, ok := fromSecs.Headline.Properties.Get("ARCHIVE"); ok {
			archiveTarget = at
		}

		// Now find the filename
		vals := strings.Split(archiveTarget, "::")
		if len(vals) != 2 {
			return nil
		}
		fname_temp := strings.TrimSpace(vals[0])
		heading := strings.TrimSpace(vals[1])

		fname := ""
		isSameFile := false
		if fname_temp != "" {
			// As org reads it: %s is the file's name without its directory, and
			// a relative result is beside the file. Filling in the whole path
			// put a file loaded as notes/a.org into notes/notes/a.org_archive.
			src := fromFile.Filename
			if abs, err := filepath.Abs(src); err == nil {
				src = abs
			}
			if strings.Contains(fname_temp, "%s") {
				fname = fmt.Sprintf(fname_temp, filepath.Base(src))
			} else {
				fname = fname_temp
			}
			if strings.HasPrefix(fname, "~/") {
				if home, err := os.UserHomeDir(); err == nil {
					fname = filepath.Join(home, fname[2:])
				}
			}
			if !filepath.IsAbs(fname) {
				fname = filepath.Join(filepath.Dir(src), fname)
			}
			if fname == src {
				isSameFile = true
			}
		} else {
			fname = fromFile.Filename
			isSameFile = true
		}
		if heading == "" {
			// This is not allowed! We HAVE to have a new heading
			// So just abort
			if isSameFile {
				return nil
			}
			// Trim off stars to determine level
			res := common.Target{Filename: fname, Type: "file"}
			return &res
		} else {
			// We do not handle datetree yet
			if m := headlineRegexp.FindStringSubmatch(heading); m != nil {
				// "file+headline", not "file+heading". The singular spelling
				// appeared nowhere else in the codebase and `GetFromTarget`
				// does not answer to it, so it fell through to the default and
				// returned nothing - which meant archiving to a heading, the
				// form every `#+ARCHIVE:` line in the wild uses, had never
				// worked at all. It failed as "could not find destination
				// target", which reads like the heading is missing.
				res := common.Target{Filename: fname, Type: "file+headline", Id: m[2], Lvl: len(m[1])}
				return &res
			}
		}
		// TODO: Need datetree capabilities
	}

	return nil
}

func setProp(c *org.Section, name string, val string) {
	if val != "" || !Conf().ArchiveSkipEmptyProperties {
		c.Headline.Properties.Set(name, val)
	}
}

func archiveMarkDone(s *org.Section, file *common.OrgFile) {
	if s.Headline.Status != "" && IsActive(s, file) {
		s.Headline.Status = "DONE"
	}
	for _, c := range s.Children {
		archiveMarkDone(c, file)
	}
}

func fixupArchiveHeading(ofile *common.OrgFile, sec *org.Section) *org.Section {
	c := CopySection(sec)
	// file from where the entry came, its outline path the archiving time
	// org-archive-save-context-info
	// Parts of context info that should be stored as properties when archiving.
	// When a subtree is moved to an archive file, it loses information given by
	// context, like inherited tags, the category, and possibly also the TODO
	// state (depending on the variable `org-archive-mark-done').
	// This variable can be a list of any of the following symbols:
	//
	// time       The time of archiving.
	// file       The file where the entry originates.
	// ltags      The local tags, in the headline of the subtree.
	// itags      The tags the subtree inherits from further up the hierarchy.
	// todo       The pre-archive TODO state.
	// category   The category, taken from file name or #+CATEGORY lines.
	// olpath     The outline path to the item.  These are all headlines above
	//            the current item, separated by /, like a file path.
	//
	// For each symbol present in the list, a property will be created in
	// the archived entry, with a prefix \"ARCHIVE_\", to remember this
	// information."
	//
	// Written as org writes them (org-archive-subtree): ARCHIVE_ plus the
	// symbol upcased, tags space separated, the outline path of the headings
	// *above* it, the time as an active stamp without its brackets. Read off
	// sec, the original, because the copy has no parents to take a path or
	// inherited tags from.
	if len(Conf().ArchiveSaveContextInfo) > 0 && c.Headline.Properties == nil {
		c.Headline.Properties = &org.PropertyDrawer{}
	}
	for _, what := range Conf().ArchiveSaveContextInfo {
		switch what {
		case "time":
			setProp(c, "ARCHIVE_TIME", archiveTime(time.Now()))
		case "todo":
			setProp(c, "ARCHIVE_TODO", c.Headline.Status)
		case "file":
			setProp(c, "ARCHIVE_FILE", abbreviateHome(ofile.Filename))
		case "category":
			setProp(c, "ARCHIVE_CATEGORY", categoryOf(ofile, sec))
		case "ltags":
			setProp(c, "ARCHIVE_LTAGS", strings.Join(c.Headline.Tags, " "))
		case "itags":
			local := map[string]bool{}
			for _, t := range sec.Headline.Tags {
				local[t] = true
			}
			itags := []string{}
			for _, t := range GetParentTags(sec, ofile.Doc) {
				if t = strings.TrimSpace(t); t != "" && !local[t] && !slices.Contains(itags, t) {
					itags = append(itags, t)
				}
			}
			setProp(c, "ARCHIVE_ITAGS", strings.Join(itags, " "))
		case "olpath":
			olp := outlineOf(sec)
			if len(olp) > 0 {
				olp = olp[:len(olp)-1]
			}
			setProp(c, "ARCHIVE_OLPATH", strings.Join(olp, "/"))
		}
	}

	if Conf().ArchiveMarkDone {
		archiveMarkDone(c, ofile)
	}
	return c
}

func Archive(db common.ODb, tgt *common.Target) (common.ResultMsg, error) {
	var res common.ResultMsg = common.ResultMsg{}
	// Find the archive target
	// Refile to the archive target
	archiveTgt := FindArchiveTarget(db, tgt)
	if archiveTgt != nil {
		fmt.Fprintf(os.Stderr, "Archive target found: %s [%s]\n", archiveTgt.Filename, archiveTgt.Id)
		refile := common.Refile{FromId: *tgt, ToId: *archiveTgt}
		// This does not quite work because we need to add a bunch of properties to the
		// copied section
		return Refile(db, &refile, fixupArchiveHeading, true)
	} else {
		fmt.Fprintf(os.Stderr, "Could not find archive target. ABORT")
	}
	res.Ok = false
	res.Msg = "failed to find archive target"
	return res, fmt.Errorf("failed to find archive target")
}

// archiveTime is org's ARCHIVE_TIME: "2026-11-03 Tue 14:05".
func archiveTime(t time.Time) string {
	return t.Format("2006-01-02 Mon 15:04")
}

// abbreviateHome is a path as Emacs's abbreviate-file-name writes it: ~/ for
// the home directory.
func abbreviateHome(p string) string {
	if home, err := os.UserHomeDir(); err == nil && home != "" && strings.HasPrefix(p, home+string(filepath.Separator)) {
		return "~" + p[len(home):]
	}
	return p
}

// categoryOf is org-get-category: the nearest CATEGORY property at or above
// the heading, else the file's #+CATEGORY:, else the file's name without its
// extension.
func categoryOf(f *common.OrgFile, sec *org.Section) string {
	for s := sec; s != nil; s = s.Parent {
		if s.Headline != nil && s.Headline.Properties != nil {
			if v, ok := s.Headline.Properties.Get("CATEGORY"); ok && strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		}
	}
	if f != nil && f.Doc != nil {
		if v := strings.TrimSpace(f.Doc.Get("CATEGORY")); v != "" {
			return v
		}
	}
	if f == nil {
		return ""
	}
	return strings.TrimSuffix(filepath.Base(f.Filename), filepath.Ext(f.Filename))
}

// ArchiveSibling is org's org-archive-to-archive-sibling: the heading moves to
// the end of a sibling called Archive (archiveSiblingHeading), tagged
// ARCHIVE, which is made at the end of its parent's children when there is
// none. The heading gets ARCHIVE_TIME and nothing else, as in org: it has not
// left its file or its parent, so there is no context to keep.
func ArchiveSibling(db common.ODb, tgt *common.Target) (common.ResultMsg, error) {
	res := common.ResultMsg{}
	f, sec := db.GetFromTarget(tgt, false)
	if f == nil || sec == nil || sec.Headline == nil {
		res.Msg = "could not find that heading"
		return res, nil
	}
	name := strings.TrimSpace(Conf().ArchiveSiblingHeading)
	if name == "" {
		name = "Archive"
	}
	isSibling := func(s *org.Section) bool {
		return s != nil && s.Headline != nil && strings.TrimSpace(common.GetSectionTitle(s)) == name && slices.Contains(s.Headline.Tags, "ARCHIVE")
	}
	if isSibling(sec) {
		res.Msg = fmt.Sprintf("%q is the archive sibling itself", name)
		return res, nil
	}
	parent := sec.Parent
	if isSibling(parent) {
		res.Msg = fmt.Sprintf("it is already under %q", name)
		return res, nil
	}
	parentOlp := []string{}
	if parent != nil && parent.Headline != nil {
		parentOlp = outlineOf(parent)
	}
	src := common.Target{Type: "file+olp", Filename: f.Filename, Id: olpString(outlineOf(sec))}
	found := false
	if parent != nil {
		for _, c := range parent.Children {
			if isSibling(c) {
				found = true
			}
		}
	}
	if !found {
		lines, err := readFileLines(f.Filename)
		if err != nil {
			return res, err
		}
		if len(lines) > 0 && lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}
		at := len(lines)
		if row := headlineRow(parent); row >= 0 && row < len(lines) {
			at = subtreeEndRow(lines, row, parent.Headline.Lvl, row) + 1
			for at > row+1 && strings.TrimSpace(lines[at-1]) == "" {
				at--
			}
		}
		head := strings.Repeat("*", sec.Headline.Lvl) + " " + name + "  :ARCHIVE:"
		lines = append(lines[:at], append([]string{head}, lines[at:]...)...)
		if err := writeLines(f.Filename, lines); err != nil {
			return res, err
		}
	}
	dest := common.Target{Type: "file+olp", Filename: f.Filename, Id: olpString(append(parentOlp, name))}
	stamp := func(of *common.OrgFile, s *org.Section) *org.Section {
		c := CopySection(s)
		if c.Headline.Properties == nil {
			c.Headline.Properties = &org.PropertyDrawer{}
		}
		c.Headline.Properties.Set("ARCHIVE_TIME", archiveTime(time.Now()))
		return c
	}
	return Refile(db, &common.Refile{FromId: src, ToId: dest}, stamp, false)
}
