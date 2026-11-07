package orgs

import (
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"
)

// worgHandler serves the embedded worg build. A plain http.FileServer over an
// embed.FS sent the whole bundle on every load: embedded files have no
// modification time, so there was no Last-Modified to revalidate against, no
// ETag and no compression - 6.7 MB of main.js each time the page was opened.
//
// tools/buildworg.sh stores the compressible files gzipped (name.js.gz, no
// name.js), which also keeps them small in the binary. A browser that takes
// gzip is handed those bytes as they are; anything else gets them unzipped on
// the way out.
//
// static/ names carry a content hash, so they never change and are cached for
// good. Everything else (index.html, dicefx.js, the dictionary, harper's wasm)
// keeps its name across builds and is revalidated with an ETag on each use.
type worgHandler struct {
	root  fs.FS
	etags sync.Map // stored name -> quoted etag
}

func newWorgHandler(root fs.FS) *worgHandler {
	return &worgHandler{root: root}
}

func (h *worgHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if name == "" {
		name = "index.html"
	}
	stored, gz := name, false
	f, err := h.root.Open(stored)
	if errors.Is(err, fs.ErrNotExist) {
		stored, gz = name+".gz", true
		f, err = h.root.Open(stored)
	}
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	if st, err := f.Stat(); err != nil || st.IsDir() {
		http.NotFound(w, r)
		return
	}
	rs, ok := f.(io.ReadSeeker)
	if !ok {
		http.Error(w, "unseekable file", http.StatusInternalServerError)
		return
	}

	hdr := w.Header()
	ctype := mime.TypeByExtension(path.Ext(name))
	if ctype == "" {
		// The dictionary's .aff/.dic: what FileServer's sniffing called them.
		ctype = "text/plain; charset=utf-8"
	}
	hdr.Set("Content-Type", ctype)
	if strings.HasPrefix(name, "static/") {
		hdr.Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		hdr.Set("Cache-Control", "no-cache")
		if tag := h.etag(stored, rs); tag != "" {
			hdr.Set("ETag", tag)
		}
	}
	if !gz {
		http.ServeContent(w, r, name, time.Time{}, rs)
		return
	}
	hdr.Add("Vary", "Accept-Encoding")
	if acceptsGzip(r) {
		hdr.Set("Content-Encoding", "gzip")
		// A range of the compressed bytes is not what a client asking for a
		// range means; send the whole thing.
		r.Header.Del("Range")
		http.ServeContent(w, r, name, time.Time{}, rs)
		return
	}
	if tag := hdr.Get("ETag"); tag != "" && r.Header.Get("If-None-Match") == tag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	if r.Method == http.MethodHead {
		return
	}
	zr, err := gzip.NewReader(rs)
	if err != nil {
		http.Error(w, "bad embedded file", http.StatusInternalServerError)
		return
	}
	io.Copy(w, zr)
}

// etag hashes a file once, on its first request rather than at startup, so a
// server nobody opens worg on never reads the 40 MB of it.
func (h *worgHandler) etag(stored string, rs io.ReadSeeker) string {
	if v, ok := h.etags.Load(stored); ok {
		return v.(string)
	}
	sum := sha256.New()
	if _, err := io.Copy(sum, rs); err != nil {
		return ""
	}
	if _, err := rs.Seek(0, io.SeekStart); err != nil {
		return ""
	}
	tag := `"` + hex.EncodeToString(sum.Sum(nil)[:12]) + `"`
	h.etags.Store(stored, tag)
	return tag
}

func acceptsGzip(r *http.Request) bool {
	for _, part := range strings.Split(r.Header.Get("Accept-Encoding"), ",") {
		coding, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		coding = strings.ToLower(strings.TrimSpace(coding))
		if coding != "gzip" && coding != "*" {
			continue
		}
		if q, ok := strings.CutPrefix(strings.ReplaceAll(strings.ToLower(params), " ", ""), "q="); ok {
			if v, err := strconv.ParseFloat(q, 64); err == nil && v == 0 {
				continue
			}
		}
		return true
	}
	return false
}
