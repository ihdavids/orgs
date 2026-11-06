package notionorg

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
)

// MaxUnpacked caps what ReadZip will unpack in all, nested zips included, so
// a zip bomb is refused rather than filling memory.
var MaxUnpacked int64 = 4 << 30

// How deep zips inside zips are opened. Notion's large exports are a zip of
// part zips; nothing it writes goes deeper than that.
const maxZipDepth = 3

// ReadZip unpacks a Notion export zip in memory: every file by its slash
// path. A zip inside it (Notion splits a large export into Part-1.zip,
// Part-2.zip, ... inside one download) is unpacked into the same tree, as
// the parts are pieces of one export. A name that would land outside the
// tree (an absolute path, a .. segment) is refused, and so is the whole zip:
// a file that escapes is not an export to half import.
func ReadZip(data []byte) (map[string][]byte, error) {
	files := map[string][]byte{}
	var total int64
	if err := readZip(data, files, &total, 0); err != nil {
		return nil, err
	}
	return files, nil
}

func readZip(data []byte, files map[string][]byte, total *int64, depth int) error {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if errors.Is(err, zip.ErrInsecurePath) {
		return fmt.Errorf("the zip names a file outside itself; not unpacking it")
	}
	if err != nil {
		return fmt.Errorf("not a zip: %v", err)
	}
	for _, f := range zr.File {
		name := strings.ReplaceAll(f.Name, `\`, "/")
		if strings.HasSuffix(name, "/") || f.FileInfo().IsDir() {
			continue
		}
		clean, err := safeName(name)
		if err != nil {
			return err
		}
		base := path.Base(clean)
		if strings.HasPrefix(clean, "__MACOSX/") || base == ".DS_Store" || strings.HasPrefix(base, "._") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("%s: %v", clean, err)
		}
		left := MaxUnpacked - *total
		b, err := io.ReadAll(io.LimitReader(rc, left+1))
		rc.Close()
		if err != nil {
			return fmt.Errorf("%s: %v", clean, err)
		}
		if int64(len(b)) > left {
			return fmt.Errorf("the zip unpacks to more than %d MiB; not unpacking it", MaxUnpacked>>20)
		}
		*total += int64(len(b))
		if strings.EqualFold(path.Ext(clean), ".zip") && depth < maxZipDepth {
			if err := readZip(b, files, total, depth+1); err != nil {
				return fmt.Errorf("%s: %v", clean, err)
			}
			continue
		}
		files[clean] = b
	}
	return nil
}

// safeName is a zip entry's name as a path inside the tree, or an error for
// one that would leave it.
func safeName(name string) (string, error) {
	bad := fmt.Errorf("the zip names %q, which is outside it; not unpacking it", name)
	if name == "" || strings.HasPrefix(name, "/") || strings.Contains(name, "\x00") ||
		len(name) > 1 && name[1] == ':' {
		return "", bad
	}
	for _, seg := range strings.Split(name, "/") {
		if seg == ".." {
			return "", bad
		}
	}
	clean := path.Clean(name)
	if clean == "." || strings.HasPrefix(clean, "../") {
		return "", bad
	}
	return clean, nil
}
