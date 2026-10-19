package orgs

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bodgit/sevenzip"
	"github.com/klauspost/compress/zstd"
	"github.com/nwaples/rardecode/v2"
	"github.com/ulikunitz/xz"
)

// An archive of tokens or pictures in a tokens or images folder - zip, 7z,
// rar, tar (plain, gz, bz2, xz, zst), comic book cbz/cb7/cbr/cbt - is read as
// a folder of its own, never unpacked where it lies: its pictures listed
// (named and sized as a token pack names them) and served out of it. A
// picture's path runs through the archive:
// tokens/FA_Tokens.zip/FA_Tokens/Creatures/Beast/Wolf_A1_01.png.
//
// Zip and 7z are read in place, a picture at a time, their index kept open
// while the file is unchanged. Tar and rar can only be read start to end, so
// their pictures are unpacked once into the user's cache (orgs/archives/,
// named for the archive's path, size and time) and served from there.

type archiveIndex struct {
	mod     time.Time
	size    int64
	entries []string // the pictures and videos, sorted
	open    func(name string) ([]byte, time.Time, error)
	close   func()
}

var archiveCache = struct {
	sync.Mutex
	m map[string]*archiveIndex
}{m: map[string]*archiveIndex{}}

// The endings an archive's name may have, longest first, and how each reads.
var archiveKinds = []struct{ suffix, kind string }{
	{".tar.gz", "tar.gz"}, {".tar.bz2", "tar.bz2"}, {".tar.xz", "tar.xz"}, {".tar.zst", "tar.zst"},
	{".tgz", "tar.gz"}, {".tbz2", "tar.bz2"}, {".tbz", "tar.bz2"}, {".txz", "tar.xz"}, {".tzst", "tar.zst"},
	{".tar", "tar"}, {".cbt", "tar"}, {".zip", "zip"}, {".cbz", "zip"}, {".7z", "7z"}, {".cb7", "7z"},
	{".rar", "rar"}, {".cbr", "rar"},
}

// archiveKind is how a file of this name reads as an archive, and the name
// without its ending; "" when it is not one.
func archiveKind(name string) (string, string) {
	low := strings.ToLower(name)
	for _, k := range archiveKinds {
		if strings.HasSuffix(low, k.suffix) {
			return k.kind, name[:len(name)-len(k.suffix)]
		}
	}
	return "", ""
}

// A picture worth listing from an archive.
func archivePicture(name string, videos bool) bool {
	base := path.Base(name)
	if strings.HasPrefix(base, ".") || strings.Contains(name, "__MACOSX") {
		return false
	}
	ext := strings.ToLower(path.Ext(name))
	return showExts[ext] || (videos && videoExts[ext])
}

// Anything an archive is kept for: pictures, videos, sounds.
func archiveMedia(name string) bool {
	base := path.Base(name)
	if strings.HasPrefix(base, ".") || strings.Contains(name, "__MACOSX") {
		return false
	}
	ext := strings.ToLower(path.Ext(name))
	return showExts[ext] || videoExts[ext] || audioExts[ext]
}

func openArchive(p string) (*archiveIndex, error) {
	st, err := os.Stat(p)
	if err != nil {
		return nil, err
	}
	archiveCache.Lock()
	defer archiveCache.Unlock()
	if a := archiveCache.m[p]; a != nil && a.mod.Equal(st.ModTime()) && a.size == st.Size() {
		return a, nil
	}
	if old := archiveCache.m[p]; old != nil && old.close != nil {
		old.close()
	}
	kind, _ := archiveKind(p)
	var a *archiveIndex
	switch kind {
	case "zip":
		a, err = openZipArchive(p)
	case "7z":
		a, err = open7z(p)
	case "":
		return nil, fmt.Errorf("%s is not an archive", filepath.Base(p))
	default:
		a, err = unpackArchive(p, kind, st)
	}
	if err != nil {
		return nil, err
	}
	a.mod, a.size = st.ModTime(), st.Size()
	sort.Strings(a.entries)
	archiveCache.m[p] = a
	return a, nil
}

func openZipArchive(p string) (*archiveIndex, error) {
	r, err := zip.OpenReader(p)
	if err != nil {
		return nil, err
	}
	byName := map[string]*zip.File{}
	a := &archiveIndex{close: func() { r.Close() }}
	for _, f := range r.File {
		if f.FileInfo().IsDir() || !archiveMedia(f.Name) {
			continue
		}
		byName[f.Name] = f
		a.entries = append(a.entries, f.Name)
	}
	a.open = func(name string) ([]byte, time.Time, error) {
		f := byName[name]
		if f == nil {
			return nil, time.Time{}, os.ErrNotExist
		}
		rc, err := f.Open()
		if err != nil {
			return nil, time.Time{}, err
		}
		defer rc.Close()
		b, err := io.ReadAll(io.LimitReader(rc, showMaxBytes*4))
		return b, f.Modified, err
	}
	return a, nil
}

func open7z(p string) (*archiveIndex, error) {
	r, err := sevenzip.OpenReader(p)
	if err != nil {
		return nil, err
	}
	byName := map[string]*sevenzip.File{}
	a := &archiveIndex{close: func() { r.Close() }}
	for _, f := range r.File {
		if f.FileInfo().IsDir() || !archiveMedia(f.Name) {
			continue
		}
		byName[f.Name] = f
		a.entries = append(a.entries, f.Name)
	}
	var mu sync.Mutex // one picture at a time out of a solid block
	a.open = func(name string) ([]byte, time.Time, error) {
		f := byName[name]
		if f == nil {
			return nil, time.Time{}, os.ErrNotExist
		}
		mu.Lock()
		defer mu.Unlock()
		rc, err := f.Open()
		if err != nil {
			return nil, time.Time{}, err
		}
		defer rc.Close()
		b, err := io.ReadAll(io.LimitReader(rc, showMaxBytes*4))
		return b, f.Modified, err
	}
	return a, nil
}

// unpackArchive reads a tar or rar start to end once, keeping its pictures in
// the cache, and serves them from there.
func unpackArchive(p, kind string, st os.FileInfo) (*archiveIndex, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}
	sum := sha1.Sum([]byte(fmt.Sprintf("%s|%d|%d", p, st.Size(), st.ModTime().UnixNano())))
	dir := filepath.Join(base, "orgs", "archives", hex.EncodeToString(sum[:10]))
	listing := filepath.Join(dir, ".entries")
	a := &archiveIndex{}
	a.open = func(name string) ([]byte, time.Time, error) {
		f := filepath.Join(dir, filepath.FromSlash(name))
		if rel, err := filepath.Rel(dir, f); err != nil || strings.HasPrefix(rel, "..") {
			return nil, time.Time{}, os.ErrNotExist
		}
		b, err := os.ReadFile(f)
		return b, st.ModTime(), err
	}
	if b, err := os.ReadFile(listing); err == nil {
		for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
			if l != "" {
				a.entries = append(a.entries, l)
			}
		}
		return a, nil
	}
	// Unpacked into a folder beside the final one, then moved into place,
	// so a half-finished unpacking is never taken for a whole one.
	tmp := dir + ".part"
	os.RemoveAll(tmp)
	if err := os.MkdirAll(tmp, 0755); err != nil {
		return nil, err
	}
	keep := func(name string, r io.Reader) error {
		name = strings.TrimPrefix(path.Clean("/"+filepath.ToSlash(name)), "/")
		if !archiveMedia(name) {
			return nil
		}
		out := filepath.Join(tmp, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(out), 0755); err != nil {
			return err
		}
		f, err := os.Create(out)
		if err != nil {
			return err
		}
		_, err = io.Copy(f, io.LimitReader(r, showMaxBytes*4))
		f.Close()
		if err == nil {
			a.entries = append(a.entries, name)
		}
		return err
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if kind == "rar" {
		rr, err := rardecode.NewReader(f)
		if err != nil {
			return nil, err
		}
		for {
			h, err := rr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, err
			}
			if !h.IsDir {
				if err := keep(h.Name, rr); err != nil {
					return nil, err
				}
			}
		}
	} else {
		var r io.Reader = f
		switch kind {
		case "tar.gz":
			gz, err := gzip.NewReader(f)
			if err != nil {
				return nil, err
			}
			defer gz.Close()
			r = gz
		case "tar.bz2":
			r = bzip2.NewReader(f)
		case "tar.xz":
			x, err := xz.NewReader(f)
			if err != nil {
				return nil, err
			}
			r = x
		case "tar.zst":
			z, err := zstd.NewReader(f)
			if err != nil {
				return nil, err
			}
			defer z.Close()
			r = z
		}
		tr := tar.NewReader(r)
		for {
			h, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, err
			}
			if h.Typeflag == tar.TypeReg {
				if err := keep(h.Name, tr); err != nil {
					return nil, err
				}
			}
		}
	}
	if err := os.WriteFile(filepath.Join(tmp, ".entries"), []byte(strings.Join(a.entries, "\n")), 0644); err != nil {
		return nil, err
	}
	os.RemoveAll(dir)
	if err := os.Rename(tmp, dir); err != nil {
		return nil, err
	}
	return a, nil
}

// splitArchive parts a path running through an archive into the archive and
// the entry in it, or says it does not.
func splitArchive(p string) (string, string, bool) {
	parts := strings.Split(filepath.ToSlash(p), "/")
	for i := 0; i < len(parts)-1; i++ {
		if k, _ := archiveKind(parts[i]); k != "" {
			return filepath.FromSlash(strings.Join(parts[:i+1], "/")), strings.Join(parts[i+1:], "/"), true
		}
	}
	return "", "", false
}

// A token pack's names: Goblin_Archer_A1_Bow_01 is a Goblin Archer, and
// Lamia_A1_Large_Monstrosity_01 a large Lamia. The name is the words before
// the variant code (A1, G2...) or the size; a size of Large or more is large.
var (
	variantCode = regexp.MustCompile(`^[A-Z]\d+$`)
	sizeWord    = map[string]string{"tiny": "small", "small": "small", "medium": "small", "large": "large", "huge": "large", "gargantuan": "large"}
)

func packName(stem string) (string, string) {
	parts := strings.FieldsFunc(strings.TrimLeft(stem, "!#@_-. "), func(r rune) bool { return r == '_' || r == ' ' || r == '-' })
	// A pack's own sorting word ahead of the name (!Core_Barbarian): not the name.
	if len(parts) > 1 && strings.EqualFold(parts[0], "core") {
		parts = parts[1:]
	}
	var name []string
	size := ""
	stop := false
	for _, p := range parts {
		if s, ok := sizeWord[strings.ToLower(p)]; ok {
			if size == "" || s == "large" {
				size = s
			}
			stop = true
			continue
		}
		if variantCode.MatchString(p) {
			stop = true
			continue
		}
		if !stop {
			name = append(name, p)
		}
	}
	if len(name) == 0 {
		return strings.ReplaceAll(stem, "_", " "), size
	}
	return strings.Join(name, " "), size
}

// archivePictures lists an archive's pictures as walkShow lists a folder's:
// `rel` the archive's path inside its folder, `as` what paths start with,
// `tokens` whether videos count (tokens) or not (pictures to show). Several
// of one name are numbered (Zombie Shambler, Zombie Shambler 2...), which a
// fight's matching reads past. Their category is the first folder that
// sorts them (Creatures, under a pack's single top folder), else the
// archive's name; an animated one's still is a picture of its name beside it.
func archivePictures(p, rel, as, folder string, sizes tokenSizes, shared bool, cats map[string]bool) []showPicture {
	tokens := folder != "images"
	a, err := openArchive(p)
	if err != nil {
		fmt.Fprintf(os.Stderr, "dnd: reading %s: %s\n", p, err)
		return nil
	}
	entries := []string{}
	for _, e := range a.entries {
		if archivePicture(e, tokens) {
			entries = append(entries, e)
		}
	}
	_, stem := archiveKind(path.Base(rel))
	tops := map[string]bool{}
	for _, e := range entries {
		tops[strings.SplitN(e, "/", 2)[0]] = true
	}
	skipTop := len(tops) == 1 && strings.Contains(entries0(entries), "/")
	stills, videos := map[string]string{}, map[string]bool{}
	for _, e := range entries {
		s := strings.TrimSuffix(e, path.Ext(e))
		if videoExts[strings.ToLower(path.Ext(e))] {
			videos[s] = true
		} else {
			stills[s] = e
		}
	}
	seen := map[string]int{}
	out := []showPicture{}
	for _, e := range entries {
		ext := strings.ToLower(path.Ext(e))
		s := strings.TrimSuffix(e, path.Ext(e))
		if !videoExts[ext] && videos[s] {
			continue
		}
		dirs := strings.Split(path.Dir(e), "/")
		if skipTop && len(dirs) > 0 {
			dirs = dirs[1:]
		}
		cat := path.Base(stem)
		if len(dirs) > 0 && dirs[0] != "." && dirs[0] != "" {
			cat = dirs[0]
		}
		cats[cat] = true
		name, size := packName(path.Base(s))
		if folder == "tiles" {
			name, size = tileName(path.Base(s)), tileSquares(path.Base(s))
		}
		seen[name]++
		if n := seen[name]; n > 1 {
			name = fmt.Sprintf("%s %d", name, n)
		}
		key := rel + "/" + e
		if sz, ok := sizes.Sizes[key]; ok {
			size = sz
		}
		if size == "small" && folder != "tiles" {
			size = ""
		}
		pic := showPicture{Path: as + key, Name: name, Category: cat, Size: size, Shared: shared, Tags: sizes.Tags[key]}
		if n := sizes.Names[key]; n != "" {
			pic.Name = n
		}
		if videoExts[ext] && stills[s] != "" {
			pic.Still = as + rel + "/" + stills[s]
		}
		out = append(out, pic)
	}
	return out
}

func entries0(e []string) string {
	if len(e) == 0 {
		return ""
	}
	return e[0]
}

// serveShow serves a picture or token by its file, or, when its path runs
// through an archive, out of the archive.
func serveShow(w http.ResponseWriter, r *http.Request, p string) bool {
	if st, err := os.Stat(p); err == nil && !st.IsDir() {
		w.Header().Set("Cache-Control", "private, max-age=600")
		if strings.EqualFold(filepath.Ext(p), ".svg") {
			w.Header().Set("Content-Security-Policy", svgPolicy)
		}
		http.ServeFile(w, r, p)
		return true
	}
	ap, entry, ok := splitArchive(p)
	if !ok {
		return false
	}
	ext := strings.ToLower(path.Ext(entry))
	if !showExts[ext] && !videoExts[ext] && !audioExts[ext] {
		return false
	}
	a, err := openArchive(ap)
	if err != nil {
		return false
	}
	b, mod, err := a.open(entry)
	if err != nil {
		return false
	}
	if t := mime.TypeByExtension(ext); t != "" {
		w.Header().Set("Content-Type", t)
	}
	if ext == ".svg" {
		w.Header().Set("Content-Security-Policy", svgPolicy)
	}
	// A picture in an archive never changes while the archive does not.
	w.Header().Set("Cache-Control", "private, max-age=86400")
	http.ServeContent(w, r, path.Base(entry), mod, bytes.NewReader(b))
	return true
}
