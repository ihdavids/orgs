package orgs

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func gzipped(t *testing.T, s string) []byte {
	var b bytes.Buffer
	zw := gzip.NewWriter(&b)
	zw.Write([]byte(s))
	zw.Close()
	return b.Bytes()
}

func TestWorgHandler(t *testing.T) {
	body := "console.log('worg')"
	h := newWorgHandler(fstest.MapFS{
		"index.html":              {Data: []byte("<html></html>")},
		"static/js/main.1a.js.gz": {Data: gzipped(t, body)},
		"dicefx.js.gz":            {Data: gzipped(t, body)},
	})
	get := func(path string, hdr map[string]string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		for k, v := range hdr {
			r.Header.Set(k, v)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}

	w := get("/", nil)
	if w.Code != 200 || w.Body.String() != "<html></html>" || w.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("index: %d %q %v", w.Code, w.Body.String(), w.Header())
	}
	tag := w.Header().Get("ETag")
	if tag == "" {
		t.Fatal("index has no etag")
	}
	if w := get("/", map[string]string{"If-None-Match": tag}); w.Code != http.StatusNotModified {
		t.Fatalf("revalidating index: %d", w.Code)
	}

	w = get("/static/js/main.1a.js", map[string]string{"Accept-Encoding": "gzip, br"})
	if w.Code != 200 || w.Header().Get("Content-Encoding") != "gzip" ||
		w.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" ||
		w.Header().Get("Content-Type") != "text/javascript; charset=utf-8" {
		t.Fatalf("static: %d %v", w.Code, w.Header())
	}
	zr, err := gzip.NewReader(w.Body)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := io.ReadAll(zr); string(got) != body {
		t.Fatalf("static body %q", got)
	}

	// No gzip: unzipped on the way out.
	w = get("/dicefx.js", map[string]string{"Accept-Encoding": "gzip;q=0"})
	if w.Code != 200 || w.Header().Get("Content-Encoding") != "" || w.Body.String() != body {
		t.Fatalf("plain: %d %q %v", w.Code, w.Body.String(), w.Header())
	}
	if w := get("/dicefx.js", map[string]string{"If-None-Match": w.Header().Get("ETag")}); w.Code != http.StatusNotModified {
		t.Fatalf("revalidating dicefx: %d", w.Code)
	}

	for _, p := range []string{"/missing.js", "/static", "/../index.html.gz"} {
		if w := get(p, nil); w.Code != 404 {
			t.Fatalf("%s: %d", p, w.Code)
		}
	}
}
