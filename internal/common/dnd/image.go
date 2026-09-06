package dnd

/* SDOC: DnD
* Character Portraits

  A character sheet can carry a portrait. It is a property in the drawer, so
  it round trips through the org file like everything else:

  #+BEGIN_SRC org
  ,   :DND_IMAGE:       [[file:art/lyra.png]]
  ,   :DND_IMAGE_FOCUS: 57% 29%
  ,   :DND_IMAGE_ZOOM:  3.2
  #+END_SRC

  =DND_IMAGE= accepts anything that names an image:

  | =[[file:art/lyra.png][Lyra]]=   | a full org link, description and all |
  | =[[./art/lyra.webp]]=           | a bracket link with no link type     |
  | =<file:~/pics/lyra.gif>=        | an angle link                        |
  | =file:art/lyra.png=             | a plain link                         |
  | =art/lyra.apng=                 | a bare relative path                 |
  | =~/pics/lyra.png=               | a home relative path                 |
  | =https://example.com/lyra.avif= | a url                                |
  | =data:image/png;base64,iVBO...= | an inline data uri                   |

  A relative path is resolved against the directory of the org file, the way
  emacs would resolve it. Any format a browser can show works, animated ones
  included - the bytes are handed to the browser untouched. Local files are
  inlined into the exported html so the sheet stays a single self contained
  document you can mail to somebody.

** Cropping to a face

   Portrait art is rarely framed for a circular medallion, so two more
   properties choose which part of it to show:

   - =DND_IMAGE_FOCUS= is the point of the image that lands in the middle of
     the medallion, written as =x% y%= from the top left corner. It defaults
     to =50% 50%=, the middle of the image.
   - =DND_IMAGE_ZOOM= is how far to magnify, so =3.2= shows a bit under a
     third of the picture. It defaults to =1=, which fits the image to the
     medallion the way =object-fit: cover= would.

   Together they crop to a head: point the focus at the face and zoom in
   until the shoulders sit on the rim.

** The printed sheet

   =dndlatex= and =dndpdf= include the portrait too, but pdflatex is stricter
   than a browser: it only reads png, jpeg and pdf files from the local disk.
   A url or an animated gif is left out of the printed sheet and noted in the
   sheet warnings. Focus and zoom are honoured, the crop is computed from the
   real pixel size of the file.
EDOC */

import (
	"encoding/base64"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// maxEmbeddedImage caps how much image data is inlined into an exported
// sheet. Past that the sheet points at the file instead of carrying it, since
// a hundred megabyte data uri helps nobody.
const maxEmbeddedImage = 12 << 20

// imageMimes covers the formats a browser will show. Anything not listed is
// sniffed from the bytes instead.
var imageMimes = map[string]string{
	".png":  "image/png",
	".apng": "image/apng",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".jfif": "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
	".avif": "image/avif",
	".svg":  "image/svg+xml",
	".bmp":  "image/bmp",
	".ico":  "image/x-icon",
	".tif":  "image/tiff",
	".tiff": "image/tiff",
	".jxl":  "image/jxl",
}

// latexImageExts are what pdflatex can \includegraphics.
var latexImageExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".pdf": true,
}

// linkTypes are the org link prefixes that all just mean "a file on disk".
var linkTypes = []string{"file+sys:", "file+emacs:", "file:", "attachment:", "docview:"}

// ParseImageRef pulls the target out of whatever the DND_IMAGE property
// holds. Org has more than one way to write the same link and a player will
// paste whichever their editor produced, so [[file:art/lyra.png][Lyra]],
// <file:art/lyra.png> and art/lyra.png all mean the same file. A url or a
// data uri comes back untouched.
func ParseImageRef(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	if strings.HasPrefix(s, "[[") {
		s = s[2:]
		if end := strings.Index(s, "]]"); end >= 0 {
			s = s[:end]
		}
		// [[target][description]] - the description is not part of the target
		if i := strings.Index(s, "]["); i >= 0 {
			s = s[:i]
		}
		s = strings.TrimSpace(s)
	} else if strings.HasPrefix(s, "<") && strings.HasSuffix(s, ">") {
		s = strings.TrimSpace(s[1 : len(s)-1])
	}
	for _, p := range linkTypes {
		if strings.HasPrefix(strings.ToLower(s), p) {
			s = s[len(p):]
			// A file link can carry a ::search suffix. An image never wants one.
			if i := strings.Index(s, "::"); i >= 0 {
				s = s[:i]
			}
			break
		}
	}
	return strings.TrimSpace(s)
}

// ImageIsRemote reports whether a reference is something a browser can fetch
// on its own - a url or an inline data uri - rather than a path on disk.
func ImageIsRemote(ref string) bool {
	l := strings.ToLower(ref)
	if strings.HasPrefix(l, "data:") || strings.HasPrefix(l, "//") {
		return true
	}
	i := strings.Index(l, "://")
	// A scheme is a plain word, so a directory called "a://b" is still a path.
	return i > 0 && !strings.ContainsAny(l[:i], `/\ .`)
}

// ImagePath turns a local reference into an absolute path. baseDir is the
// directory holding the org sheet, so a relative link resolves the way it
// would in emacs - next to the file it was written in.
func ImagePath(ref string, baseDir string) string {
	if ref == "" || ImageIsRemote(ref) {
		return ""
	}
	p := ref
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, p[1:])
		}
	}
	if !filepath.IsAbs(p) && baseDir != "" {
		p = filepath.Join(baseDir, p)
	}
	return filepath.Clean(p)
}

// ImageSrc resolves a reference into something that can be dropped straight
// into an <img src>. Urls and data uris pass through untouched; a file on
// disk is read and inlined as a data uri so an exported sheet still shows the
// portrait when it is mailed to somebody or opened off a thumb drive.
//
// Every web image format works, animated ones included - the bytes are copied
// through as they are and the browser does the rest.
func ImageSrc(ref string, baseDir string) (string, error) {
	ref = ParseImageRef(ref)
	if ref == "" {
		return "", nil
	}
	if ImageIsRemote(ref) {
		return ref, nil
	}
	path := ImagePath(ref, baseDir)
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("portrait %s could not be read: %s", ref, err)
	}
	if info.Size() > maxEmbeddedImage {
		// Too big to inline. A file url still shows on the machine that holds
		// the picture, which is where an oversized one usually gets viewed.
		return "file://" + filepath.ToSlash(path), nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("portrait %s could not be read: %s", ref, err)
	}
	return "data:" + imageMime(path, data) + ";base64," +
		base64.StdEncoding.EncodeToString(data), nil
}

// imageMime names the media type of an image, by extension where we know it
// and by sniffing the bytes where we do not.
func imageMime(path string, data []byte) string {
	if m, ok := imageMimes[strings.ToLower(filepath.Ext(path))]; ok {
		return m
	}
	if t := http.DetectContentType(data); strings.HasPrefix(t, "image/") {
		return t
	}
	return "application/octet-stream"
}

// PortraitCrop is the placement of a portrait inside the round medallion,
// worked out for a renderer that cannot do it itself.
//
// The web sheet gets by with focus and zoom alone because css can crop with
// object-fit. LaTeX cannot, so it is handed the finished geometry: an image
// Width wide with its top left corner at (X, Y), all three in multiples of
// the medallion diameter, with the origin at the middle of the medallion and
// y pointing up the page the way tikz likes it.
type PortraitCrop struct {
	File   string  `json:"file"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
}

// LatexPortrait works out how to place a portrait in the printed sheet, or
// explains why it cannot. pdflatex only reads png, jpeg and pdf from the
// local disk, so a url or an animated gif has to sit the printed sheet out.
func LatexPortrait(ref string, baseDir string, focusX, focusY, zoom float64) (*PortraitCrop, string) {
	ref = ParseImageRef(ref)
	if ref == "" {
		return nil, ""
	}
	if ImageIsRemote(ref) {
		return nil, fmt.Sprintf("portrait %s is a url and pdflatex cannot fetch it, "+
			"the printed sheet has no portrait", ref)
	}
	path := ImagePath(ref, baseDir)
	ext := strings.ToLower(filepath.Ext(path))
	if !latexImageExts[ext] {
		return nil, fmt.Sprintf("portrait %s is not a png, jpeg or pdf, "+
			"the printed sheet has no portrait", filepath.Base(path))
	}
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Sprintf("portrait %s could not be read: %s", ref, err)
	}
	crop := &PortraitCrop{File: path, Width: 1, Height: 1, X: -0.5, Y: 0.5}
	pw, ph := imageSize(path)
	if pw <= 0 || ph <= 0 {
		// A pdf, or something image.DecodeConfig does not know. Fall back to
		// a square fit, which is right for a portrait cropped ahead of time.
		return crop, ""
	}
	w, h := float64(pw), float64(ph)
	focusX, focusY, zoom = normalizeCrop(focusX, focusY, zoom)
	// Cover the medallion first, then magnify: exactly what the css does, so
	// the same focus and zoom frame the same face on paper and on screen.
	crop.Width, crop.Height = zoom, zoom
	if w > h {
		crop.Width = zoom * w / h
	} else {
		crop.Height = zoom * h / w
	}
	crop.X = -focusX / 100 * crop.Width
	crop.Y = focusY / 100 * crop.Height
	return crop, ""
}

// imageSize reads the pixel size out of an image file, returning zeroes when
// the format is one the standard library cannot decode.
func imageSize(path string) (int, int) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return 0, 0
	}
	return cfg.Width, cfg.Height
}

// normalizeCrop clamps the focus into the picture and keeps the zoom sane.
func normalizeCrop(focusX, focusY, zoom float64) (float64, float64, float64) {
	clamp := func(v, lo, hi, def float64) float64 {
		if v <= 0 && def > 0 {
			return def
		}
		if v < lo {
			return lo
		}
		if v > hi {
			return hi
		}
		return v
	}
	return clamp(focusX, 0, 100, 50), clamp(focusY, 0, 100, 50), clamp(zoom, 0.05, 40, 1)
}

// ParseFocus reads a "57% 29%" focus point. A single number sets both axes
// and anything unparseable falls back to the middle of the picture.
func ParseFocus(val string) (float64, float64) {
	fields := strings.FieldsFunc(strings.TrimSpace(val), func(r rune) bool {
		return r == ' ' || r == ',' || r == '\t'
	})
	num := func(s string) (float64, bool) {
		v, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(s), "%"), 64)
		return v, err == nil
	}
	switch len(fields) {
	case 0:
		return 50, 50
	case 1:
		if v, ok := num(fields[0]); ok {
			return v, v
		}
	default:
		x, okx := num(fields[0])
		y, oky := num(fields[1])
		if okx && oky {
			return x, y
		}
	}
	return 50, 50
}

// ParseZoom reads a magnification, written either as a multiplier ("3.2") or
// as a percentage ("320%").
func ParseZoom(val string) float64 {
	s := strings.TrimSpace(val)
	if s == "" {
		return 1
	}
	pct := strings.HasSuffix(s, "%")
	v, err := strconv.ParseFloat(strings.TrimSuffix(s, "%"), 64)
	if err != nil {
		return 1
	}
	if pct {
		v /= 100
	}
	if v <= 0 {
		return 1
	}
	return v
}

// FormatZoom writes a zoom back out for the org file, with no trailing ".0".
func FormatZoom(v float64) string {
	if v <= 0 || v == 1 {
		return ""
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}
