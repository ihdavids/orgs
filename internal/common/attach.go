package common

import (
	"os"
	"path/filepath"
	"strings"
)

// Wire types for attachments: the files a heading owns.
//
// This is org-attach. A heading with an :ID: owns a folder worked out from that
// id, or names one outright with :DIR:, and the files in it are that heading's.
// The link written into the body is `[[attachment:name]]`, which is org's own
// way of pointing at one - a link that survives the folder moving, because it
// says which file rather than where it is.

// One file a heading owns.
type Attachment struct {
	// The file's name inside the heading's folder. Also its id: the folder is
	// the index, so there is no list beside it to fall out of step with.
	Name string
	Size int64
	// When it was last written, as YYYY-MM-DD HH:MM.
	Modified string

	// What kind of thing it is - image, audio, video, pdf, text or binary -
	// worked out from the extension, the same way a source block's result file
	// and a kanban card's picture are. Decided here so that one list says what
	// a picture is.
	Media string
	// The language to colour it as, when it is text. Empty when there is
	// nothing sensible to say, which leaves it plain rather than half coloured.
	Lang string

	// Where a browser can fetch it.
	Url string
	// The org link that reaches it, ready to paste.
	Link string
}

// Everything a heading owns, and where.
type AttachList struct {
	Ok       bool
	Msg      string
	Hash     string
	Filename string
	Headline string

	// The folder, relative to the org root where it is inside it and absolute
	// where it is not.
	Dir string
	// How that folder was decided: "dir" when the heading names one with a
	// :DIR: or :ATTACH_DIR: property, "id" when it was worked out from the
	// heading's :ID:, and "" when the heading has neither and so owns nothing
	// yet. A client showing an empty list wants to know which of those it is.
	From string
	// The heading's id, empty when the folder came from a property.
	Id string

	Attachments []Attachment
}

// One upload.
type AttachResult struct {
	Ok  bool
	Msg string

	Attachment

	Filename string
	// The line the link was written on, 1 based. Zero when nothing was written.
	Line int
	// Set when the heading had to be given an :ID: to own a folder at all, so a
	// client can say that the file was changed by more than the attachment.
	NewId string
	// Set when the heading was tagged for the first time.
	Tagged string
}

// One heading that owns files, for the list of everything.
type AttachHeading struct {
	Hash     string
	Headline string
	Filename string
	Olp      []string
	Dir      string
	Count    int
	// The total size of everything in the folder.
	Size int64
	// The newest thing in it, as YYYY-MM-DD HH:MM.
	Newest      string
	Attachments []Attachment
}

// Every heading in the database that owns files.
type AttachAll struct {
	Ok       bool
	Msg      string
	Headings []AttachHeading
	// How many files there are altogether, and how much disk they take, which
	// is the question an attachment store eventually raises.
	Count int
	Size  int64
	// Folders that exist under the attachment root but that no heading claims -
	// the heading was deleted, or its id changed, and the files were left
	// behind. Reported rather than cleaned up: they are somebody's files.
	Orphans []string
}

// ---------------------------------------------------------------------------
// Where a heading's folder is
// ---------------------------------------------------------------------------
//
// The path arithmetic lives here rather than in the server because more than
// the server needs it: an exporter writing a page has to turn
// `[[attachment:report.pdf]]` into something a browser can fetch, and an
// exporter cannot import the server - the server imports the exporters.
//
// Pure functions of their arguments, so both ends reach the same folder.

// The attachment root: `attach.dir` under the first org directory, `data` when
// it is not set - which is what org-attach-id-dir is.
func AttachRootIn(orgDirs []string, set AttachSettings) string {
	p := strings.TrimSpace(set.Dir)
	if p == "" {
		p = "data"
	}
	if strings.HasPrefix(p, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(p, "~"), "/"))
		}
	}
	if !filepath.IsAbs(p) {
		root := "."
		if len(orgDirs) > 0 {
			root = orgDirs[0]
		}
		p = filepath.Join(root, p)
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return abs
}

// An id's folder under the attachment root: the first two characters, then the
// rest - `8f/3c1a20-...`. That is org-attach-id-uuid-folder-format, so a store
// written by orgs is one emacs finds and the other way round.
//
// Two levels rather than one because a flat folder of ten thousand entries is
// slow to list on most filesystems and unreadable on any.
func AttachIdPath(id string) string {
	id = strings.TrimSpace(id)
	if len(id) < 3 {
		// Too short to split. Anything this odd was not minted here, and one
		// folder is better than a folder named "" with everything in it.
		return id
	}
	return filepath.Join(id[:2], id[2:])
}

// Where a heading's files are, from the two things about the heading that
// decide it.
//
// `dirProp` is its :DIR: or :ATTACH_DIR: property and wins outright, read
// relative to the org file that holds the heading - which is what org does,
// because that property is written by a person, beside the file. `idProp` is
// its :ID:, from which the folder under the attachment root is worked out.
//
// `from` says which of those answered: "dir", "id", or "" for a heading that
// owns nothing yet. Creates nothing.
func AttachDirFrom(dirProp, idProp, orgFile string, orgDirs []string, set AttachSettings) (dir string, from string, id string) {
	if v := strings.TrimSpace(dirProp); v != "" {
		p := filepath.FromSlash(v)
		if strings.HasPrefix(p, "~") {
			if home, err := os.UserHomeDir(); err == nil {
				p = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(p, "~"), "/"))
			}
		}
		if !filepath.IsAbs(p) && orgFile != "" {
			p = filepath.Join(filepath.Dir(orgFile), p)
		}
		if abs, err := filepath.Abs(p); err == nil {
			return abs, "dir", ""
		}
		return p, "dir", ""
	}
	id = strings.TrimSpace(idProp)
	if id == "" {
		return "", "", ""
	}
	return filepath.Join(AttachRootIn(orgDirs, set), AttachIdPath(id)), "id", id
}

// The `attachment:` link protocol, and what one points at.
const AttachProtocol = "attachment"

// Is this an attachment link, and if so which file does it name?
//
// `attachment:report.pdf` and `attachment:sub/report.pdf` both work - org
// allows a path inside the folder - and anything climbing out of it does not.
func AttachLinkName(target string) (string, bool) {
	rest, ok := strings.CutPrefix(target, AttachProtocol+":")
	if !ok {
		return "", false
	}
	rest = strings.TrimSpace(strings.TrimPrefix(rest, "//"))
	// A search part - `attachment:notes.org::*Heading` - names a place inside
	// the file rather than a different file.
	if i := strings.Index(rest, "::"); i >= 0 {
		rest = rest[:i]
	}
	if rest == "" {
		return "", false
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(rest)))
	if clean == "." || strings.HasPrefix(clean, "..") || filepath.IsAbs(filepath.FromSlash(clean)) {
		return "", false
	}
	return clean, true
}
