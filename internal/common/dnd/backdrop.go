package dnd

/* SDOC: DnD
* Character Sheet Backdrops

  The portrait is the character's face; the backdrop is the room they are
  standing in. It is a picture - or a pile of pictures - washed out behind the
  whole html sheet, and like the portrait it is a property in the drawer, so
  it round trips through the org file:

  #+BEGIN_SRC org
  ,   :DND_BACKDROP:         [[file:art/tavern.jpg]], https://example.com/woods.png
  ,   :DND_BACKDROP_CYCLE:   4m-12m
  ,   :DND_BACKDROP_OPACITY: 18%
  #+END_SRC

  =DND_BACKDROP= takes the same references =DND_IMAGE= does, one or more of
  them: a full org link, a bare relative or home relative path, a url or a
  data uri. They are separated by commas or by whitespace, and a bracket link
  may hold either inside it. A relative path resolves against the directory of
  the org file, the way emacs would resolve it, and a local file is inlined
  into the exported html so the sheet stays one self contained document.

  A reference naming a *folder* stands for every image in it, in name order,
  so a folder of scenery is one property and dropping another picture into it
  is the whole of adding it:

  #+BEGIN_SRC org
  ,   :DND_BACKDROP: ~/pics/druid
  #+END_SRC

  The same thing can be handed to an export instead of written on the
  character, which paints one sheet and changes nothing on disk:

  #+BEGIN_SRC bash
  orgs dnd sheet  -file lyra.org -backdrop ~/pics/druid -backdrop-cycle 4m-12m
  orgs dnd import -ddb 12345678  -backdrop ~/pics/druid
  #+END_SRC

  The import writes it into the sheet it creates; the export does not.

** Cycling

   With more than one backdrop the sheet changes picture on its own, picking
   the next one at random - never the one already up - and crossfading to it.
   How long a picture stays up is a random time too, so the sheet never falls
   into a visible rhythm.

   =DND_BACKDROP_CYCLE= sets that dwell time:

   | =8m=      | eight minutes every time                       |
   | =4m-12m=  | a random time between four and twelve minutes  |
   | =45s=     | forty five seconds                             |
   | =10=      | a bare number is minutes                       |
   | =off=     | no cycling, the first backdrop simply stays     |

   Left out, a backdrop stays up for a random five to twenty five minutes. No
   dwell time is ever longer than twenty five minutes - a larger one is
   clamped down to it - and none is shorter than five seconds.

** How washed out

   =DND_BACKDROP_OPACITY= is how strongly the picture shows through, written
   as =18%= or =0.18=. It defaults to =14%=, which is enough to colour the
   page without the text having to fight it. The backdrop is desaturated a
   little on top of that, so a busy photograph still reads as a background.

   The backdrop is a screen affair: printing the sheet leaves it out, as does
   the =dndlatex= / =dndpdf= sheet, because a page of toner spent on a picture
   nobody asked to print helps nobody.
EDOC */

import (
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	// BackdropMaxSeconds is the longest a backdrop may stay up. A picture the
	// reader has had in front of them for half an hour has stopped being a
	// change of scene, so every dwell time is clamped to this.
	BackdropMaxSeconds = 25 * 60
	// BackdropMinSeconds is the shortest, which keeps a typo out of strobe
	// territory.
	BackdropMinSeconds = 5

	backdropDefaultMin     = 5 * 60
	backdropDefaultMax     = BackdropMaxSeconds
	backdropDefaultOpacity = 0.14
)

// ParseImageRefs pulls a list of image references out of one property value.
// Org has more than one way to write a link and a player will paste whichever
// their editor produced, so the separators have to be picked carefully: a
// bracket or angle link is read whole (a data uri inside one carries commas
// of its own), and everything else ends at the first comma or space.
func ParseImageRefs(raw string) []string {
	out := []string{}
	s := strings.TrimSpace(raw)
	for i := 0; i < len(s); {
		switch c := s[i]; {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == ',':
			i++
			continue
		case strings.HasPrefix(s[i:], "[["):
			end := strings.Index(s[i:], "]]")
			if end < 0 {
				out = appendRef(out, s[i:])
				return out
			}
			out = appendRef(out, s[i:i+end+2])
			i += end + 2
		case c == '<':
			end := strings.IndexByte(s[i:], '>')
			if end < 0 {
				out = appendRef(out, s[i:])
				return out
			}
			out = appendRef(out, s[i:i+end+1])
			i += end + 1
		default:
			// A bare data uri holds commas, so only whitespace ends one.
			stop := func(r byte) bool { return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == ',' }
			if strings.HasPrefix(strings.ToLower(s[i:]), "data:") {
				stop = func(r byte) bool { return r == ' ' || r == '\t' || r == '\n' || r == '\r' }
			}
			j := i
			for j < len(s) && !stop(s[j]) {
				j++
			}
			out = appendRef(out, s[i:j])
			i = j
		}
	}
	return out
}

func appendRef(out []string, raw string) []string {
	if ref := ParseImageRef(raw); ref != "" {
		return append(out, ref)
	}
	return out
}

// BackdropRefs reads a backdrop property into the individual pictures it
// stands for. A reference naming a folder becomes every image inside it, in
// name order, so a folder of scenery is one property and dropping another
// picture in is the whole of adding it. Everything else - a file, a url, a
// data uri - stands for itself.
func BackdropRefs(raw string, baseDir string) []string {
	out := []string{}
	for _, ref := range ParseImageRefs(raw) {
		if dir := ImagePath(ref, baseDir); dir != "" && isDir(dir) {
			out = append(out, folderImages(dir, ref)...)
			continue
		}
		out = append(out, ref)
	}
	return out
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// folderImages is every picture in one folder, in name order. It does not
// descend: a folder of folders is a filing scheme, not a slideshow. The
// references come back written the way the folder was, so a relative folder
// yields relative files and the sheet stays portable.
func folderImages(dir string, ref string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []string{}
	}
	out := []string{}
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if _, ok := imageMimes[strings.ToLower(filepath.Ext(e.Name()))]; !ok {
			continue
		}
		out = append(out, path.Join(strings.TrimSuffix(filepath.ToSlash(ref), "/"), e.Name()))
	}
	sort.Strings(out)
	return out
}

// ResolveBackdrops turns backdrop references into things a css background can
// point at, in the order they were written, inlining local files the way the
// portrait is inlined. A picture that cannot be read is left out and
// explained in the returned warnings rather than failing the export - the
// rest of them still show.
func ResolveBackdrops(refs []string, baseDir string) ([]string, []string) {
	srcs := []string{}
	warnings := []string{}
	for _, ref := range refs {
		src, err := imageSrc(ref, baseDir, "backdrop")
		if err != nil {
			warnings = append(warnings, err.Error())
			continue
		}
		if src != "" {
			srcs = append(srcs, src)
		}
	}
	return srcs, warnings
}

// PlanBackdrop settles the backdrop on a sheet: which pictures, how washed
// out, and how long each one stays up. The property values are passed in
// rather than read off the character so that an exporter can override them -
// that is what lets an export flag paint one sheet without writing anything
// into the character's org file. Resolving the pictures into image data is a
// separate step, ResolveBackdrops, because only the html sheet needs it.
func PlanBackdrop(s *Sheet, raw string, cycle string, opacity float64, baseDir string) {
	if s == nil {
		return
	}
	s.Backdrop = BackdropRefs(raw, baseDir)
	s.BackdropOpacity = opacity
	if s.BackdropOpacity <= 0 {
		s.BackdropOpacity = backdropDefaultOpacity
	}
	// One picture has nothing to change to, so it simply stays up.
	s.BackdropMinSeconds, s.BackdropMaxSeconds = 0, 0
	if len(s.Backdrop) > 1 {
		s.BackdropMinSeconds, s.BackdropMaxSeconds = ParseCycle(cycle)
	}
}

// CharacterDir is the folder a character's sheet lives in, which is what a
// relative picture path in it is written against.
func CharacterDir(c *Character) string {
	if c == nil || c.Filename == "" {
		return ""
	}
	return filepath.Dir(c.Filename)
}

// ParseCycle reads the dwell time a backdrop gets before the next one takes
// over, as a range of seconds. Both ends come back zero when the sheet has
// been told not to cycle at all.
func ParseCycle(val string) (int, int) {
	s := strings.ToLower(strings.TrimSpace(val))
	if s == "" {
		return backdropDefaultMin, backdropDefaultMax
	}
	switch s {
	case "off", "none", "never", "no", "0":
		return 0, 0
	}
	// "4m-12m", or "4m - 12m", is a range. A lone "-" is not a separator in
	// any of the forms below, so splitting on it is safe.
	if lo, hi, ok := strings.Cut(s, "-"); ok {
		a, aok := parseDwell(lo)
		b, bok := parseDwell(hi)
		if aok && bok {
			if a > b {
				a, b = b, a
			}
			return clampDwell(a), clampDwell(b)
		}
	}
	if v, ok := parseDwell(s); ok {
		v = clampDwell(v)
		return v, v
	}
	return backdropDefaultMin, backdropDefaultMax
}

// parseDwell reads one duration: "45s", "8m", "1h", or a bare number, which
// is minutes - the unit anybody writing a backdrop timer is thinking in.
func parseDwell(s string) (int, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	mult := 60.0
	switch s[len(s)-1] {
	case 's':
		mult, s = 1, s[:len(s)-1]
	case 'm':
		mult, s = 60, s[:len(s)-1]
	case 'h':
		mult, s = 3600, s[:len(s)-1]
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || v < 0 {
		return 0, false
	}
	return int(v * mult), true
}

func clampDwell(v int) int {
	if v < BackdropMinSeconds {
		return BackdropMinSeconds
	}
	if v > BackdropMaxSeconds {
		return BackdropMaxSeconds
	}
	return v
}

// ParseOpacity reads how strongly a backdrop shows through, written either as
// a fraction ("0.18") or a percentage ("18%"). Anything unreadable falls back
// to the default wash.
func ParseOpacity(val string) float64 {
	s := strings.TrimSpace(val)
	if s == "" {
		return backdropDefaultOpacity
	}
	pct := strings.HasSuffix(s, "%")
	v, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(s, "%")), 64)
	if err != nil {
		return backdropDefaultOpacity
	}
	if pct {
		v /= 100
	}
	// A number above one was meant as a percentage whether or not it was
	// written with the sign.
	if v > 1 {
		v /= 100
	}
	if v < 0 {
		return backdropDefaultOpacity
	}
	if v > 1 {
		return 1
	}
	return v
}

// FormatOpacity writes a wash back out for the org file as a percentage,
// which is how it reads best in a drawer. The default is written as nothing,
// so an untouched sheet keeps an empty property.
func FormatOpacity(v float64) string {
	if v <= 0 || v == backdropDefaultOpacity {
		return ""
	}
	return strconv.FormatFloat(v*100, 'f', -1, 64) + "%"
}
