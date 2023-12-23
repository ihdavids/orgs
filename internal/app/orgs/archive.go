//lint:file-ignore ST1006 allow the use of self
package orgs

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/app/orgs/plugs"
	"github.com/ihdavids/orgs/internal/common"
)

var headlineRegexp = regexp.MustCompile(`^([*]+)\s+(.*)`)

func FindArchiveTarget(db plugs.ODb, tgt *common.Target) *common.Target {
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
			fname = fmt.Sprintf(fname_temp, fromFile.Filename)
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
				res := common.Target{Filename: fname, Type: "file+heading", Id: m[2], Lvl: len(m[1])}
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
	// TODO add archive properties!
	if Conf().ArchiveSaveContextInfo != nil && len(Conf().ArchiveSaveContextInfo) > 0 && c.Headline.Properties == nil {
		c.Headline.Properties = &org.PropertyDrawer{}
	}
	if slices.Contains(Conf().ArchiveSaveContextInfo, "time") {
		timestamp := org.NewOrgDateNow()
		timestamp.HaveTime = true
		setProp(c, "ARCHIVE_TIME", timestamp.ToString())
	}
	if slices.Contains(Conf().ArchiveSaveContextInfo, "todo") {
		setProp(c, "ARCHIVE_TODO", c.Headline.Status)
	}
	if slices.Contains(Conf().ArchiveSaveContextInfo, "file") {
		setProp(c, "ARCHIVE_FILE", ofile.Filename)
	}
	if slices.Contains(Conf().ArchiveSaveContextInfo, "category") {
		category := ""
		if cat := ofile.Doc.Get("CATEGORY"); cat != "" {
			category = cat
		}
		if cat, ok := c.Headline.Properties.Get("CATEGORY"); ok {
			category = cat
		}
		setProp(c, "ARCHIVE_CATEGORY", category)
	}
	if slices.Contains(Conf().ArchiveSaveContextInfo, "ltags") {
		tags := strings.TrimSpace(strings.Join(c.Headline.Tags, ":"))
		if tags != "" {
			tags = ":" + tags + ":"
		}
		setProp(c, "ARCHIVE_LOCAL_TAGS", tags)
	}
	if slices.Contains(Conf().ArchiveSaveContextInfo, "itags") {
		tags := GetParentTags(c, ofile.Doc)
		tagstr := ""
		if len(tags) > 0 {
			tagstr = ":" + strings.Join(tags, ":") + ":"
		}
		setProp(c, "ARCHIVE_INHERITED_TAGS", tagstr)
	}
	if slices.Contains(Conf().ArchiveSaveContextInfo, "olpath") {
		path := common.BuildOutlinePath(c, "/")
		setProp(c, "ARCHIVE_OUTLINE_PATH", path)
	}
	for _, p := range c.Headline.Properties.Properties {
		fmt.Printf("PROP: %v\n", p)
	}

	if Conf().ArchiveMarkDone {
		archiveMarkDone(c, ofile)
	}
	return c
}

func Archive(db plugs.ODb, tgt *common.Target) (common.ResultMsg, error) {
	var res common.ResultMsg = common.ResultMsg{}
	// Find the archive target
	// Refile to the archive target
	archiveTgt := FindArchiveTarget(db, tgt)
	if archiveTgt != nil {
		fmt.Printf("Archive target found: %s [%s]\n", archiveTgt.Filename, archiveTgt.Id)
		refile := common.Refile{FromId: *tgt, ToId: *archiveTgt}
		// This does not quite work because we need to add a bunch of properties to the
		// copied section
		return Refile(db, &refile, fixupArchiveHeading, true)
	} else {
		fmt.Printf("Could not find archive target. ABORT")
	}
	res.Ok = false
	res.Msg = "failed to find archive target"
	return res, fmt.Errorf("failed to find archive target")
}
