package orgs

// Attachments: the files a heading owns.
//
// This is org-attach, and it is deliberately org-attach rather than a third
// scheme of orgs' own. `/image/paste` and `/voice/recording` are each one
// special case of "this heading owns a file on disk" - a picture in `images/`,
// a recording in `audio/` - and neither of them is somewhere emacs would look.
// A heading's attachment folder is somewhere both ends already agree about:
//
//   - a `:DIR:` (or the older `:ATTACH_DIR:`) property names it outright,
//     relative to the org file that holds the heading; or
//   - it is worked out from the heading's `:ID:` - `data/8f/3c1a20-.../`, the
//     id split two characters deep, which is org-attach-id-uuid-folder-format.
//
// The link written into the body is `[[attachment:name]]`: org's own link type
// for this, and the one that says *which file* rather than where it currently
// is, so moving the folder does not break it. Resolving that link is in
// attachlink.go, because more than one thing has to do it.
//
// Three things about the writing side, all of them the same rules the rest of
// the server writes org by:
//
//  1. Every edit is a **line splice**. A heading shares its file with a hundred
//     others and writing the parsed document back would reformat all of them to
//     add one property.
//  2. The folder is the index. A file's name is its id, and there is no list
//     beside the folder that could fall out of step with what is in it.
//  3. Nothing is converted and nothing is renamed beyond being made safe. What
//     was uploaded is what is on disk, because the extension is the only thing
//     either end has to go on about what the file is.

import (
	b64 "encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

// attachLock serialises the read/modify/write cycle when a heading is given an
// id, a tag or a link.
var attachLock sync.Mutex

// The properties that name a heading's folder outright, in the order org looks
// at them. DIR is what org-attach writes now; ATTACH_DIR is what it wrote
// before, and files written years ago still say it.
var attachDirProps = []string{"DIR", "ATTACH_DIR"}

// A name safe to join onto a directory: no separators, nothing that climbs out,
// and not one of the names a filesystem reserves.
//
// Checked rather than trusted because the name comes off a multipart upload,
// which is to say off whatever the browser was handed, which is to say off
// whatever was on the other end of a "share" button.
var attachNameRe = regexp.MustCompile(`^[^/\\:*?"<>|\x00-\x1f]+$`)

// ---------------------------------------------------------------------------
// Settings
// ---------------------------------------------------------------------------

func attachSettings() common.AttachSettings {
	if Conf().Server == nil {
		return common.AttachSettings{}
	}
	return Conf().Server.Attach
}

// The root of the attachment store. The arithmetic is in internal/common so
// that the exporters, which cannot import this package, reach the same folder.
func attachRoot() (string, error) {
	return common.AttachRootIn(orgDirsOf(), attachSettings()), nil
}

func orgDirsOf() []string {
	if Conf().Server == nil {
		return nil
	}
	return Conf().Server.OrgDirs
}

// The tag put on a heading the first time something is attached to it. "ATTACH"
// unless the yaml says otherwise; "-" means tag nothing.
func attachTag() string {
	t := strings.TrimSpace(attachSettings().Tag)
	if t == "" {
		return "ATTACH"
	}
	if t == "-" {
		return ""
	}
	return t
}

func attachMaxBytes() int64 {
	mb := attachSettings().MaxMb
	if mb <= 0 {
		mb = 64
	}
	return int64(mb) << 20
}

// ---------------------------------------------------------------------------
// Where a heading's files live
// ---------------------------------------------------------------------------

// Where a heading's files are, without creating anything.
//
// `from` says how the answer was reached - "dir" for a property, "id" for the
// heading's id, "" for a heading that owns nothing yet - because a client
// showing an empty list wants to know which of those it is looking at.
func attachDirOf(sec *org.Section, f *common.OrgFile) (dir string, from string, id string) {
	if sec == nil || sec.Headline == nil {
		return "", "", ""
	}
	orgFile := ""
	if f != nil && f.Doc != nil {
		orgFile = f.Doc.Path
	}
	return common.AttachDirFrom(
		GetProp(sec, attachDirProps...),
		GetProp(sec, "ID", "Id", "id"),
		orgFile, orgDirsOf(), attachSettings())
}

// The folder as it is worth showing: relative to the org root when it is inside
// it, and absolute when it is not. A path is either "somewhere in my notes" or
// "somewhere else", and the shape of it should say which.
func attachDirShown(dir string) string {
	if dir == "" {
		return ""
	}
	if Conf().Server == nil || len(Conf().Server.OrgDirs) == 0 {
		return dir
	}
	root, err := filepath.Abs(Conf().Server.OrgDirs[0])
	if err != nil {
		return dir
	}
	if rel, err := filepath.Rel(root, dir); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return dir
}

// A name that can be joined onto a directory safely, and that a person would
// recognise as the file they uploaded.
//
// Everything a filesystem or a url would argue about is replaced rather than
// refused: somebody dragging "Q3 report (final?).pdf" onto a card should get a
// file, not a complaint.
func attachSafeName(name string) (string, error) {
	// Only the last element: a browser on some platforms sends a whole path.
	name = strings.TrimSpace(name)
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	name = strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|':
			return '-'
		}
		if r < 0x20 {
			return -1
		}
		return r
	}, name)
	name = strings.Trim(name, " .")
	if name == "" {
		return "", fmt.Errorf("that file has no name")
	}
	if len(name) > 180 {
		// Keep the extension: it is what says what the file is.
		ext := filepath.Ext(name)
		if len(ext) > 12 {
			ext = ""
		}
		name = name[:180-len(ext)] + ext
	}
	if !attachNameRe.MatchString(name) {
		return "", fmt.Errorf("%q is not a name a file can have", name)
	}
	return name, nil
}

// The path one attachment sits at, refusing anything that would climb out of
// the heading's own folder.
func attachPath(dir, name string) (string, error) {
	safe, err := attachSafeName(name)
	if err != nil {
		return "", err
	}
	p := filepath.Join(dir, safe)
	// Belt and braces: after the join, the answer still has to be inside.
	absDir, err1 := filepath.Abs(dir)
	absP, err2 := filepath.Abs(p)
	if err1 != nil || err2 != nil {
		return "", fmt.Errorf("could not resolve that attachment")
	}
	if rel, err := filepath.Rel(absDir, absP); err != nil || rel != safe {
		return "", fmt.Errorf("%q is not in that heading's folder", name)
	}
	return absP, nil
}

// ---------------------------------------------------------------------------
// Reading
// ---------------------------------------------------------------------------

// Everything in a heading's folder, newest first.
//
// Subdirectories are walked past rather than into: org-attach's folder is flat,
// and a folder inside it is somebody putting something there on purpose.
func attachmentsIn(dir string, hash string) []common.Attachment {
	out := []common.Attachment{}
	if dir == "" {
		return out
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return out
	}
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		kind, lang := mediaKindOf(e.Name())
		if kind == "text" && !attachLooksLikeText(filepath.Join(dir, e.Name())) {
			kind = "binary"
			lang = ""
		}
		out = append(out, common.Attachment{
			Name:     e.Name(),
			Size:     info.Size(),
			Modified: info.ModTime().Format("2006-01-02 15:04"),
			Media:    kind,
			Lang:     lang,
			Url:      attachmentUrl(hash, e.Name()),
			Link:     "[[attachment:" + e.Name() + "]]",
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Modified != out[j].Modified {
			return out[i].Modified > out[j].Modified
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// Named like text and full of bytes is binary, not text. The extension is a
// claim; the first few kilobytes are the evidence. The same rule the babel
// result files follow, and for the same reason: a megabyte of noise on the page
// is worse than saying "binary".
func attachLooksLikeText(path string) bool {
	fh, err := os.Open(path)
	if err != nil {
		return false
	}
	defer fh.Close()
	buf := make([]byte, 8192)
	n, _ := fh.Read(buf)
	if n <= 0 {
		return true
	}
	return looksLikeText(buf[:n])
}

// Where a browser fetches one from.
//
// Through this server rather than as a path under /images, because a heading's
// folder may be named by a :DIR: property pointing anywhere at all - outside
// the org root, where the static file server cannot reach - and a url that
// works for some attachments and 404s for others is worse than one that always
// works.
func attachmentUrl(hash, name string) string {
	if hash == "" {
		return ""
	}
	return "/attachment/" + hashPathSegment(hash) + "/" + urlQueryEscape(name)
}

// A heading's hash as it goes into a url path.
//
// Encoded a second time, deliberately: the hash arrives base64 already, and
// written into a path as it stands its `+` becomes a space and its `/` a path
// separator - so the handler answers "no heading with that hash", which reads
// like a stale hash rather than a mangled one. `GetHash` decodes the segment on
// the way back in, so this is the other half of that.
func hashPathSegment(hash string) string {
	return b64.URLEncoding.EncodeToString([]byte(hash))
}

// ---------------------------------------------------------------------------
// Writing: the id, the tag and the link
// ---------------------------------------------------------------------------

// The PROPERTIES drawer of the heading at `from`, made if it is not there.
//
// A cut down cousin of what UpdateRecord does for a record - the record version
// also writes a logbook line and guards the properties that are a record's
// identity, neither of which means anything here.
func ensurePropertyDrawer(lines []string, from, to int, ind string) (out []string, ps, pe, newTo int) {
	if s, e, ok := drawerAt(lines, from, to, "PROPERTIES"); ok {
		return lines, s, e, to
	}
	// Directly under the headline, past any planning line, which is where org
	// keeps it - and where go-org expects to find it.
	at := from + 1
	for at <= to && at < len(lines) && planningRe.MatchString(lines[at]) {
		at++
	}
	lines = splice(lines, at, []string{ind + ":PROPERTIES:", ind + ":END:"})
	return lines, at, at + 1, to + 2
}

// Set one property inside a drawer that is known to exist.
func setPropIn(lines []string, ps, pe int, ind, key, val string) (out []string, newPe int) {
	for i := ps + 1; i < pe && i < len(lines); i++ {
		if m := propLineRe.FindStringSubmatch(lines[i]); m != nil && strings.EqualFold(m[2], key) {
			lines[i] = propLine(m[1], m[2], val, 0)
			alignDrawer(lines, ps, pe)
			return lines, pe
		}
	}
	lines = splice(lines, pe, []string{propLine(ind, key, val, 0)})
	pe++
	alignDrawer(lines, ps, pe)
	return lines, pe
}

// Put a tag on a headline's own line, leaving everything else about it alone.
//
// A line edit rather than ToggleTag, which writes the whole document back
// through go-org and reformats every drawer and table in the file on its way
// past. Attaching a file should change one line.
func addHeadlineTag(line, tag string) (string, bool) {
	if tag == "" {
		return line, false
	}
	m := headlineRe.FindStringSubmatch(line)
	if m == nil {
		return line, false
	}
	existing := strings.Trim(strings.TrimSpace(m[5]), ":")
	tags := []string{}
	if existing != "" {
		tags = strings.Split(existing, ":")
	}
	for _, t := range tags {
		if strings.EqualFold(t, tag) {
			return line, false
		}
	}
	tags = append(tags, tag)
	return m[1] + m[2] + m[3] + strings.TrimSpace(m[4]) + "  :" + strings.Join(tags, ":") + ":", true
}

// Make sure a heading owns a folder, and answer with it.
//
// A heading that names one with :DIR: is left entirely alone - no id is minted
// for it, because it did not need one and writing an id it never asked for is
// changing the file for the server's convenience.
//
// A heading with neither is given an id, which is the only way it can own a
// folder at all. That is a real edit to somebody's file and is reported back.
func ensureAttachDir(hash string) (dir string, newId string, tagged string, filename string, err error) {
	sec := GetDb().FindByHash(hash)
	if sec == nil || sec.Headline == nil {
		return "", "", "", "", fmt.Errorf("no heading with that hash")
	}
	f := GetDb().FileFromSection(sec)
	if f == nil || f.Doc == nil {
		return "", "", "", "", fmt.Errorf("that heading has no file")
	}
	filename = f.Doc.Path

	dir, from, _ := attachDirOf(sec, f)
	wantTag := attachTag()
	haveDir := dir != "" && from != ""

	// Nothing to write: the heading already owns a folder and already carries
	// the tag.
	if haveDir && (wantTag == "" || HeadlineAloneHasTag(strings.ToLower(wantTag), sec)) {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", "", "", filename, err
		}
		return dir, "", "", filename, nil
	}

	lines, lfrom, lto, lok := recordLines(filename, sec)
	if !lok {
		return "", "", "", filename, fmt.Errorf("could not read %s", filename)
	}
	ind := indentOf(sec.Headline.Lvl)

	if !haveDir {
		newId = uuid.New().String()
		var ps, pe int
		lines, ps, pe, lto = ensurePropertyDrawer(lines, lfrom, lto, ind)
		lines, _ = setPropIn(lines, ps, pe, ind, "ID", newId)
		root, rerr := attachRoot()
		if rerr != nil {
			return "", "", "", filename, rerr
		}
		dir = filepath.Join(root, common.AttachIdPath(newId))
		lines, lfrom, lto = reread(lines, lfrom, sec.Headline.Lvl)
		_ = pe
	}

	if wantTag != "" {
		if line, added := addHeadlineTag(lines[lfrom], wantTag); added {
			lines[lfrom] = line
			tagged = wantTag
		}
	}

	if newId != "" || tagged != "" {
		if err := writeLines(filename, lines); err != nil {
			return "", "", "", filename, err
		}
		GetDb().ReloadFile(filename)
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", "", "", filename, err
	}
	return dir, newId, tagged, filename, nil
}

// Append `[[attachment:name]]` to a heading's own body.
//
// A line splice at the end of the heading's body, at the indent the body around
// it uses - the same rule a pasted picture and a voice note's text follow: a
// link at column zero under an indented heading reads as belonging to another
// heading.
func linkAttachment(hash, name string) (filename string, line int, err error) {
	sec := GetDb().FindByHash(hash)
	if sec == nil || sec.Headline == nil {
		return "", 0, fmt.Errorf("no heading with that hash")
	}
	filename, lines, from, to, err := headingBodyLines(hash)
	if err != nil {
		return "", 0, err
	}
	link := "[[attachment:" + name + "]]"
	// Already there: attaching the same file twice is one link, not two.
	for i := from; i <= to && i < len(lines); i++ {
		if strings.Contains(lines[i], link) {
			return filename, i + 1, nil
		}
	}
	at := to + 1
	if at > len(lines) {
		at = len(lines)
	}
	indent := bodyIndent(lines, from, to, sec.Headline.Lvl)
	add := []string{indent + link}
	if at > 0 && at <= len(lines) && strings.TrimSpace(lines[at-1]) != "" {
		add = append([]string{""}, add...)
	}
	// The heading's body usually ends with the blank line that separates it
	// from the next heading, and appending at the end of the body eats it - so
	// the next heading ends up hard against a link. Org does not care and a
	// person reading the file does.
	if at < len(lines) && headlineRe.MatchString(lines[at]) {
		add = append(add, "")
	}
	out := splice(lines, at, add)
	if err := writeLines(filename, out); err != nil {
		return filename, 0, err
	}
	GetDb().ReloadFile(filename)
	return filename, at + len(add), nil
}

// Take the line that links to an attachment back out, when that line is the
// link and nothing else.
//
// A link sitting in the middle of a sentence is left alone: the sentence is
// somebody's writing and a hole in it is worse than a link to a file that has
// gone. A link alone on a line is litter once the file is gone, and taking it
// out is what "delete this attachment" means.
func unlinkAttachment(hash, name string) int {
	filename, lines, from, to, err := headingBodyLines(hash)
	if err != nil {
		return 0
	}
	link := "[[attachment:" + name + "]]"
	removed := 0
	out := []string{}
	for i, l := range lines {
		if i >= from && i <= to && strings.TrimSpace(l) == link {
			removed++
			continue
		}
		out = append(out, l)
	}
	if removed == 0 {
		return 0
	}
	if err := writeLines(filename, out); err != nil {
		return 0
	}
	GetDb().ReloadFile(filename)
	return removed
}

// ---------------------------------------------------------------------------
// The endpoints
// ---------------------------------------------------------------------------

func attachJson(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

/* SDOC: API
* POST /attach — Attach a File to a Heading

	Puts a file in the heading's own folder and writes `[[attachment:name]]` into
	its body. This is org-attach: the folder is the one emacs would use, so a
	database attached to here is one emacs can read.

	*Method:* =POST= (multipart/form-data)

	*Form Fields:*
	| Field  | Type   | Required | Description                                                       |
	|--------+--------+----------+-------------------------------------------------------------------|
	| =hash= | string | yes      | The heading to attach to                                          |
	| =file= | file   | yes      | The file itself                                                   |
	| =name= | string | no       | What to call it. The upload's own name when not given.            |
	| =link= | string | no       | =f= to keep the file without writing a link into the body         |

	*Response:* An =AttachResult= JSON object carrying the attachment - its
	=Name=, =Size=, =Media=, =Url= and =Link= - plus =Filename= and =Line=.

	Two fields say what else changed about the heading, because attaching can
	edit more than the body:
	| Field    | Description                                                              |
	|----------+--------------------------------------------------------------------------|
	| =NewId=  | The =:ID:= the heading had to be given to own a folder, when it had none |
	| =Tagged= | The tag it was given, when it did not already carry one                   |

	A heading naming its own folder with =:DIR:= is never given an id: it did
	not need one, and writing one it never asked for is editing somebody's file
	for the server's convenience.

EDOC */
func PostAttach(w http.ResponseWriter, r *http.Request) {
	var res common.AttachResult
	max := attachMaxBytes()

	if err := r.ParseMultipartForm(max); err != nil {
		res.Msg = "that upload was too big to take"
		attachJson(w, res)
		return
	}
	hash := strings.TrimSpace(r.FormValue("hash"))
	if hash == "" {
		res.Msg = "no heading was named"
		attachJson(w, res)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		res.Msg = "there was no file in that"
		attachJson(w, res)
		return
	}
	defer file.Close()

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" && header != nil {
		name = header.Filename
	}
	safe, err := attachSafeName(name)
	if err != nil {
		res.Msg = err.Error()
		attachJson(w, res)
		return
	}

	data, err := io.ReadAll(io.LimitReader(file, max+1))
	if err != nil {
		res.Msg = err.Error()
		attachJson(w, res)
		return
	}
	if len(data) == 0 {
		res.Msg = "that file was empty"
		attachJson(w, res)
		return
	}
	if int64(len(data)) > max {
		res.Msg = fmt.Sprintf("that file is bigger than the %dMB an attachment may be", attachSettings().MaxMb)
		attachJson(w, res)
		return
	}

	attachLock.Lock()
	defer attachLock.Unlock()

	dir, newId, tagged, filename, err := ensureAttachDir(hash)
	if err != nil {
		res.Msg = err.Error()
		attachJson(w, res)
		return
	}
	res.NewId = newId
	res.Tagged = tagged
	res.Filename = filename

	// A name already taken gets a number, rather than overwriting what is
	// there: two files called scan.pdf are two files, and the one already
	// attached is not this upload's to replace.
	path, err := attachPath(dir, safe)
	if err != nil {
		res.Msg = err.Error()
		attachJson(w, res)
		return
	}
	if _, serr := os.Stat(path); serr == nil {
		ext := filepath.Ext(safe)
		stem := strings.TrimSuffix(safe, ext)
		for n := 2; ; n++ {
			try := fmt.Sprintf("%s-%d%s", stem, n, ext)
			p, perr := attachPath(dir, try)
			if perr != nil {
				res.Msg = perr.Error()
				attachJson(w, res)
				return
			}
			if _, serr := os.Stat(p); os.IsNotExist(serr) {
				safe, path = try, p
				break
			}
			if n > 500 {
				res.Msg = "could not find a name for that file"
				attachJson(w, res)
				return
			}
		}
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		res.Msg = err.Error()
		attachJson(w, res)
		return
	}

	kind, lang := mediaKindOf(safe)
	if kind == "text" && !attachLooksLikeText(path) {
		kind, lang = "binary", ""
	}
	res.Attachment = common.Attachment{
		Name:     safe,
		Size:     int64(len(data)),
		Modified: time.Now().Format("2006-01-02 15:04"),
		Media:    kind,
		Lang:     lang,
		Url:      attachmentUrl(hash, safe),
		Link:     "[[attachment:" + safe + "]]",
	}

	// The link goes in unless the caller says not to. Not writing one is for a
	// client that is going to put the link somewhere itself - in a table, in a
	// sentence it is composing - the same reason `/image/paste` has a stash.
	if r.FormValue("link") != "f" && r.FormValue("link") != "0" {
		fn, line, lerr := linkAttachment(hash, safe)
		if lerr != nil {
			// The file is on disk. Say that rather than pretending the whole
			// thing failed, so it can be linked by hand.
			res.Ok = true
			res.Msg = "the file was attached but the heading could not be written: " + lerr.Error()
			attachJson(w, res)
			return
		}
		res.Filename = fn
		res.Line = line
	}

	res.Ok = true
	res.Msg = "attached"
	attachJson(w, res)
}

/* SDOC: API
* GET /attachments — What a Heading Owns

	Everything in one heading's attachment folder.

	*Method:* =GET=

	*Query Parameters:*
	| Parameter | Type   | Required | Description        |
	|-----------+--------+----------+--------------------|
	| =hash=    | string | yes      | The heading to ask about |

	*Response:* An =AttachList= JSON object.
	| Field         | Description                                                          |
	|---------------+----------------------------------------------------------------------|
	| =Dir=         | The folder, relative to the org root when it is inside it            |
	| =From=        | =dir= when a =:DIR:= property named it, =id= when it came from the   |
	|               | heading's =:ID:=, empty when the heading owns nothing yet            |
	| =Id=          | The heading's id, empty when the folder came from a property         |
	| =Attachments= | The files, newest first                                              |

	A heading with no id and no =:DIR:= is not an error: it simply owns nothing
	yet, and =From= empty is how that is said. Nothing is created by asking.

EDOC */
func RequestAttachments(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	res := common.AttachList{Attachments: []common.Attachment{}}
	hash := strings.TrimSpace(r.URL.Query().Get("hash"))
	if hash == "" {
		res.Msg = "no heading was named"
		attachJson(w, res)
		return
	}
	sec := GetDb().FindByHash(hash)
	if sec == nil || sec.Headline == nil {
		res.Msg = "no heading with that hash"
		attachJson(w, res)
		return
	}
	f := GetDb().FileFromSection(sec)
	res.Ok = true
	res.Hash = hash
	res.Headline = sectionTitle(sec)
	if f != nil && f.Doc != nil {
		res.Filename = f.Doc.Path
	}
	dir, from, id := attachDirOf(sec, f)
	res.Dir = attachDirShown(dir)
	res.From = from
	res.Id = id
	res.Attachments = attachmentsIn(dir, hash)
	attachJson(w, res)
}

/* SDOC: API
* GET /attachment/{hash}/{name} — Fetch One

	The bytes of one attachment.

	Served by this server rather than as a path under =/images= because a
	heading's folder may be named by a =:DIR:= property pointing anywhere at
	all - including outside the org directory, where the static file server
	cannot reach. A url that works for some attachments and answers 404 for
	others is worse than one that always works.

	*Method:* =GET=

	*Path Parameters:*
	| Parameter | Description                                              |
	|-----------+----------------------------------------------------------|
	| ={hash}=  | The heading, base64-url encoded as every hash in a path is |
	| ={name}=  | The file's name inside that heading's folder              |

	*Response:* The file, with a content type worked out from its extension.
	=404= when the heading owns no such file.

EDOC */
func RequestAttachment(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	vars := mux.Vars(r)
	h, err := GetHash(vars, "hash")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	sec := GetDb().FindByHash(string(h))
	if sec == nil || sec.Headline == nil {
		http.Error(w, "no heading with that hash", http.StatusNotFound)
		return
	}
	dir, _, _ := attachDirOf(sec, GetDb().FileFromSection(sec))
	if dir == "" {
		http.Error(w, "that heading owns no files", http.StatusNotFound)
		return
	}
	path, err := attachPath(dir, vars["name"])
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		http.Error(w, "no such attachment", http.StatusNotFound)
		return
	}
	// ServeFile does the content type, the range requests a video needs to
	// seek, and the conditional headers a second look at the same pdf wants.
	http.ServeFile(w, r, path)
}

/* SDOC: API
* DELETE /attachment/{hash}/{name} — Throw One Away

	Removes the file from the heading's folder, and takes the line that linked
	to it out of the heading's body when that line was the link and nothing
	else.

	A link written into the middle of a sentence is left alone: the sentence is
	somebody's writing, and a hole in it is worse than a link to a file that has
	gone. A link alone on a line is litter once the file is gone.

	*Method:* =DELETE=

	*Response:* A =Result= JSON object. =Msg= says whether a link was taken out
	as well as the file.

EDOC */
func DeleteAttachment(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	vars := mux.Vars(r)
	h, err := GetHash(vars, "hash")
	if err != nil {
		attachJson(w, common.ResultMsg{Ok: false, Msg: err.Error()})
		return
	}
	hash := string(h)

	attachLock.Lock()
	defer attachLock.Unlock()

	sec := GetDb().FindByHash(hash)
	if sec == nil || sec.Headline == nil {
		attachJson(w, common.ResultMsg{Ok: false, Msg: "no heading with that hash"})
		return
	}
	dir, _, _ := attachDirOf(sec, GetDb().FileFromSection(sec))
	if dir == "" {
		attachJson(w, common.ResultMsg{Ok: false, Msg: "that heading owns no files"})
		return
	}
	path, err := attachPath(dir, vars["name"])
	if err != nil {
		attachJson(w, common.ResultMsg{Ok: false, Msg: err.Error()})
		return
	}
	if _, err := os.Stat(path); err != nil {
		attachJson(w, common.ResultMsg{Ok: false, Msg: "no such attachment"})
		return
	}
	if err := os.Remove(path); err != nil {
		attachJson(w, common.ResultMsg{Ok: false, Msg: err.Error()})
		return
	}
	msg := "deleted"
	if n := unlinkAttachment(hash, filepath.Base(path)); n > 0 {
		msg = "deleted, and its link taken out"
	}
	attachJson(w, common.ResultMsg{Ok: true, Msg: msg})
}

/* SDOC: API
* GET /attachments/all — Every Heading That Owns Files

	Walks the database for headings with an attachment folder and answers with
	what each of them owns, plus the totals - which is the question an
	attachment store eventually raises and which nothing else can answer.

	*Method:* =GET=

	*Query Parameters:*
	| Parameter | Type   | Required | Description                                               |
	|-----------+--------+----------+-----------------------------------------------------------|
	| =files=   | string | no       | =t= to include each heading's own file list. Off by       |
	|           |        |          | default: the totals are the point and the lists are long. |

	*Response:* An =AttachAll= JSON object holding =Headings=, =Count=, =Size=
	and =Orphans=.

	=Orphans= are folders under the attachment root that no heading claims - the
	heading was deleted, or its id changed, and the files were left behind. They
	are reported rather than cleaned up: they are somebody's files, and a server
	that deletes them because it cannot find a heading is a server that deletes
	them the first time a file fails to parse.

EDOC */
func RequestAllAttachments(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	res := common.AttachAll{Ok: true, Headings: []common.AttachHeading{}, Orphans: []string{}}
	withFiles := r.URL.Query().Get("files") == "t"

	db := GetDb()
	claimed := map[string]bool{}
	for _, fname := range db.GetFiles() {
		f := db.FindByFile(fname)
		if f == nil || f.Doc == nil {
			continue
		}
		for _, sec := range flattenSections(f) {
			if sec == nil || sec.Headline == nil {
				continue
			}
			db.RegisterSection(sec.Hash, sec, f)
			dir, from, _ := attachDirOf(sec, f)
			if dir == "" || from == "" {
				continue
			}
			if abs, err := filepath.Abs(dir); err == nil {
				claimed[abs] = true
			}
			files := attachmentsIn(dir, sec.Hash)
			if len(files) == 0 {
				continue
			}
			h := common.AttachHeading{
				Hash:     sec.Hash,
				Headline: sectionTitle(sec),
				Filename: fname,
				Olp:      outlinePath(sec),
				Dir:      attachDirShown(dir),
				Count:    len(files),
			}
			for _, a := range files {
				h.Size += a.Size
				if a.Modified > h.Newest {
					h.Newest = a.Modified
				}
			}
			if withFiles {
				h.Attachments = files
			}
			res.Count += h.Count
			res.Size += h.Size
			res.Headings = append(res.Headings, h)
		}
	}
	sort.Slice(res.Headings, func(i, j int) bool {
		if res.Headings[i].Newest != res.Headings[j].Newest {
			return res.Headings[i].Newest > res.Headings[j].Newest
		}
		return res.Headings[i].Headline < res.Headings[j].Headline
	})
	res.Orphans = attachOrphans(claimed)
	attachJson(w, res)
}

// Folders under the attachment root that no heading claims.
//
// Only two levels down, because that is the shape the store has: root, two
// characters, the rest of the id. Anything else in there was not put there by
// this and is not this to have an opinion about.
func attachOrphans(claimed map[string]bool) []string {
	out := []string{}
	root, err := attachRoot()
	if err != nil {
		return out
	}
	tops, err := os.ReadDir(root)
	if err != nil {
		return out
	}
	for _, top := range tops {
		if !top.IsDir() || strings.HasPrefix(top.Name(), ".") {
			continue
		}
		subs, err := os.ReadDir(filepath.Join(root, top.Name()))
		if err != nil {
			continue
		}
		for _, sub := range subs {
			if !sub.IsDir() {
				continue
			}
			p := filepath.Join(root, top.Name(), sub.Name())
			abs, err := filepath.Abs(p)
			if err != nil || claimed[abs] {
				continue
			}
			// An empty folder is not an orphan, it is a folder.
			if entries, err := os.ReadDir(p); err != nil || len(entries) == 0 {
				continue
			}
			out = append(out, attachDirShown(abs))
		}
	}
	sort.Strings(out)
	return out
}
