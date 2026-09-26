package orgs

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Putting a picture on a heading.
//
// A picture pasted onto a card has to land somewhere before it can be linked
// to, and "somewhere" is a folder in the org directory - so that the link is a
// relative one and moving the whole org directory keeps every picture, exactly
// as a voice note's audio does.
//
// The write is a line splice: the link is appended to the heading's own body
// and nothing else in the file is touched.

const imageDirName = "images"

// The biggest paste worth taking. A screenshot is a megabyte or two; anything
// past this is somebody pasting a photograph library into a kanban card, and
// the org directory is not the place for it.
const maxImageBytes = 24 << 20

var imageLock sync.Mutex

// What a browser calls the thing on the clipboard, and what to call the file.
// Nothing is converted - the bytes are written as they arrive - so the
// extension has to follow what was actually pasted.
var imageTypes = map[string]string{
	"image/png":     ".png",
	"image/jpeg":    ".jpg",
	"image/jpg":     ".jpg",
	"image/gif":     ".gif",
	"image/webp":    ".webp",
	"image/avif":    ".avif",
	"image/bmp":     ".bmp",
	"image/svg+xml": ".svg",
}

type ImagePasteResult struct {
	Ok bool   `json:"Ok"`
	Msg string `json:"Msg"`
	// The link written into the file, relative to the org file that holds it.
	Link string `json:"link"`
	// Where a browser can fetch it, for showing it straight away.
	Url      string `json:"url"`
	Filename string `json:"filename"`
	Line     int    `json:"line"`
}

// The folder pictures go in: `images` under the first org directory, made if
// it is not there. The same place the html exporter's links resolve against,
// so a pasted picture shows in an exported page as well as on the card.
func imageDir() (string, error) {
	root := "."
	if Conf().Server != nil && len(Conf().Server.OrgDirs) > 0 {
		root = Conf().Server.OrgDirs[0]
	}
	p := filepath.Join(root, imageDirName)
	if err := os.MkdirAll(p, 0755); err != nil {
		return "", err
	}
	return filepath.Abs(p)
}

// A name that says when it arrived and cannot collide with the one pasted a
// second later. The same shape a recording's name has, for the same reason:
// the file is its own index, and there is no list beside the folder to fall
// out of step with it.
func imageName(ext string, now time.Time, n int) string {
	stamp := now.Format("20060102-150405")
	if n > 0 {
		return fmt.Sprintf("paste-%s-%d%s", stamp, n, ext)
	}
	return fmt.Sprintf("paste-%s%s", stamp, ext)
}

func PostImagePaste(w http.ResponseWriter, r *http.Request) {
	var res ImagePasteResult

	if err := r.ParseMultipartForm(maxImageBytes); err != nil {
		res.Msg = "that paste was too big to take"
		imageJson(w, res)
		return
	}
	// Two ways of saying where it goes. A card names its heading; the tables
	// view names the file and the line to put it after, because a chart of a
	// table belongs under that table rather than at the end of whatever
	// heading the table happens to sit in.
	hash := r.FormValue("hash")
	intoFile := r.FormValue("filename")
	afterLine := 0
	if v := r.FormValue("afterLine"); v != "" {
		fmt.Sscanf(v, "%d", &afterLine)
	}
	if hash == "" && intoFile == "" {
		res.Msg = "no heading or file was named"
		imageJson(w, res)
		return
	}

	file, header, err := r.FormFile("image")
	if err != nil {
		res.Msg = "there was no picture in that"
		imageJson(w, res)
		return
	}
	defer file.Close()

	kind := strings.ToLower(strings.TrimSpace(r.FormValue("type")))
	if kind == "" && header != nil {
		kind = strings.ToLower(header.Header.Get("Content-Type"))
	}
	ext, ok := imageTypes[kind]
	if !ok {
		// Fall back to what the file was called, for a browser that pastes
		// without saying what it is.
		if header != nil {
			e := strings.ToLower(filepath.Ext(header.Filename))
			for _, known := range imageTypes {
				if e == known {
					ext = e
					ok = true
					break
				}
			}
		}
	}
	if !ok {
		res.Msg = fmt.Sprintf("%q is not a picture this knows how to keep", kind)
		imageJson(w, res)
		return
	}

	data, err := io.ReadAll(io.LimitReader(file, maxImageBytes+1))
	if err != nil {
		res.Msg = err.Error()
		imageJson(w, res)
		return
	}
	if len(data) == 0 {
		res.Msg = "that picture was empty"
		imageJson(w, res)
		return
	}
	if len(data) > maxImageBytes {
		res.Msg = "that picture is too big to keep in an org directory"
		imageJson(w, res)
		return
	}

	imageLock.Lock()
	defer imageLock.Unlock()

	var filename string
	var lines []string
	var from, to, at int
	var indentLvl int

	if hash != "" {
		var herr error
		filename, lines, from, to, herr = headingBodyLines(hash)
		if herr != nil {
			res.Msg = herr.Error()
			imageJson(w, res)
			return
		}
		sec, ok := GetDb().ByHash[hash]
		if !ok || sec == nil || sec.Headline == nil {
			res.Msg = "no heading with that hash"
			imageJson(w, res)
			return
		}
		indentLvl = sec.Headline.Lvl
		at = to + 1
	} else {
		filename = intoFile
		b, rerr := os.ReadFile(filename)
		if rerr != nil {
			res.Msg = rerr.Error()
			imageJson(w, res)
			return
		}
		lines = strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
		// `afterLine` is the **zero-based index of the line to go after**,
		// which is what a table's EndLine is - go-org counts rows from zero.
		// Inserting *at* that index would put the picture inside the table,
		// one row up from the bottom.
		at = afterLine + 1
		if at < 1 {
			at = 0
		}
		if at > len(lines) {
			at = len(lines)
		}
		from, to = at, at-1
		indentLvl = 1
	}

	dir, err := imageDir()
	if err != nil {
		res.Msg = err.Error()
		imageJson(w, res)
		return
	}
	now := time.Now()
	var path string
	for n := 0; ; n++ {
		path = filepath.Join(dir, imageName(ext, now, n))
		if _, err := os.Stat(path); os.IsNotExist(err) {
			break
		}
		if n > 500 {
			res.Msg = "could not find a name for that picture"
			imageJson(w, res)
			return
		}
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		res.Msg = err.Error()
		imageJson(w, res)
		return
	}

	// Relative to the org file that holds the link, so that moving the whole
	// org directory keeps every picture.
	link := filepath.ToSlash(path)
	if rel, rerr := filepath.Rel(filepath.Dir(filename), path); rerr == nil && !strings.HasPrefix(rel, "..") {
		link = filepath.ToSlash(rel)
	}

	// Written at the indent the body around it uses, the same as a voice
	// note's text: a link at column zero under an indented heading reads as
	// another heading's.
	indent := bodyIndent(lines, from, to, indentLvl)
	if hash == "" && at > 0 && at <= len(lines) {
		// Put it under the table at the table's own indent.
		indent = bodyIndent(lines, at-1, at-1, indentLvl)
	}

	add := []string{indent + "[[file:" + link + "]]"}
	// A blank line before it when the line above is not already one, so the
	// link is its own paragraph rather than being run into the table.
	if at > 0 && at <= len(lines) && strings.TrimSpace(lines[at-1]) != "" {
		add = append([]string{""}, add...)
	}
	out := append([]string{}, lines[:at]...)
	out = append(out, add...)
	out = append(out, lines[at:]...)

	if err := os.WriteFile(filename, []byte(strings.Join(out, "\n")+"\n"), 0644); err != nil {
		// The picture is already on disk; say so rather than pretending the
		// whole thing failed, so it can be linked by hand.
		res.Msg = "the picture was saved but the heading could not be written: " + err.Error()
		res.Link = link
		imageJson(w, res)
		return
	}

	res.Ok = true
	res.Link = link
	res.Url = mediaURL(link, filename)
	res.Filename = filename
	res.Line = at + 1
	res.Msg = "added"
	imageJson(w, res)
}

func imageJson(w http.ResponseWriter, res ImagePasteResult) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}
