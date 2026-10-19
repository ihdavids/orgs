package orgs

import (
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
)

// Every picture on a page, for taking tiles (or tokens, or pictures) off a
// site in one go: the GM picks which, and each is then added by its url
// (POST /dnd/dungeon/pictures), which fetches it on the server.

type scrapedPicture struct {
	Url  string `json:"url"`
	Name string `json:"name"`
}

var (
	anchorRe   = regexp.MustCompile(`(?is)<a\s([^>]*)>(.*?)</a>`)
	sourceTag  = regexp.MustCompile(`(?is)<source\s[^>]*>`)
	bgImageRe  = regexp.MustCompile(`(?i)background(?:-image)?\s*:\s*url\(\s*['"]?([^'")]+)['"]?\s*\)`)
	pictureURL = regexp.MustCompile(`(?i)\.(jpe?g|png|webp|gif|avif)(\?[^#]*)?(#.*)?$`)
	// Pictures on every page that are not what the page is about.
	chromeRe = regexp.MustCompile(`(?i)(logo|favicon|icon|avatar|sprite|emoji|spacer|pixel|badge|button|banner-ad|gravatar|1x1)`)
)

// bestSrcset is the widest picture a srcset offers.
func bestSrcset(set string) string {
	best, bw := "", -1.0
	for _, part := range strings.Split(set, ",") {
		f := strings.Fields(strings.TrimSpace(part))
		if len(f) == 0 {
			continue
		}
		w := 0.0
		if len(f) > 1 {
			d := strings.TrimRight(strings.TrimRight(f[1], "w"), "x")
			w, _ = strconv.ParseFloat(d, 64)
			if strings.HasSuffix(f[1], "x") {
				w *= 1000
			}
		}
		if w > bw {
			best, bw = f[0], w
		}
	}
	return best
}

// scrapePictures lists a page's pictures, resolved against `at`, each once,
// in the order the page has them: a link straight to a picture is taken in
// place of the thumbnail inside it (that is usually the full size one).
func scrapePictures(page []byte, at string) []scrapedPicture {
	base, err := url.Parse(at)
	if err != nil {
		return nil
	}
	html := string(page)
	out := []scrapedPicture{}
	seen := map[string]bool{}
	skip := map[string]bool{}
	add := func(raw, name string) {
		raw = strings.TrimSpace(strings.ReplaceAll(raw, "&amp;", "&"))
		if raw == "" || strings.HasPrefix(raw, "data:") || strings.HasPrefix(raw, "javascript:") {
			return
		}
		ref, err := url.Parse(raw)
		if err != nil {
			return
		}
		u := base.ResolveReference(ref)
		if u.Scheme != "http" && u.Scheme != "https" {
			return
		}
		s := u.String()
		file := path.Base(u.Path)
		if seen[s] || skip[s] || chromeRe.MatchString(file) || strings.HasSuffix(strings.ToLower(u.Path), ".svg") {
			return
		}
		seen[s] = true
		name = strings.TrimSpace(name)
		if name == "" || len(name) > 80 {
			name = strings.TrimSuffix(file, path.Ext(file))
			if un, err := url.PathUnescape(name); err == nil {
				name = un
			}
		}
		out = append(out, scrapedPicture{Url: s, Name: safeShowName(name)})
	}
	resolve := func(raw string) string {
		ref, err := url.Parse(strings.TrimSpace(strings.ReplaceAll(raw, "&amp;", "&")))
		if err != nil {
			return ""
		}
		return base.ResolveReference(ref).String()
	}
	// Links to pictures first, the thumbnails inside them set aside.
	for _, m := range anchorRe.FindAllStringSubmatch(html, -1) {
		href := tagAttrs("<a " + m[1] + ">")["href"]
		if href == "" || !pictureURL.MatchString(href) {
			continue
		}
		alt := ""
		thumbs := []string{}
		for _, t := range imgTag.FindAllString(m[2], -1) {
			a := tagAttrs(t)
			if alt == "" {
				alt = a["alt"]
			}
			for _, k := range []string{"src", "data-src", "data-lazy-src", "data-original"} {
				if a[k] != "" {
					thumbs = append(thumbs, resolve(a[k]))
				}
			}
			if a["srcset"] != "" {
				thumbs = append(thumbs, resolve(bestSrcset(a["srcset"])))
			}
		}
		add(href, alt)
		for _, th := range thumbs {
			if !seen[th] {
				skip[th] = true
			}
		}
	}
	for _, t := range imgTag.FindAllString(html, -1) {
		a := tagAttrs(t)
		src := ""
		for _, k := range []string{"data-src", "data-lazy-src", "data-original", "data-full", "src"} {
			if a[k] != "" && !strings.HasPrefix(a[k], "data:") {
				src = a[k]
				break
			}
		}
		if set := a["srcset"]; set != "" {
			src = bestSrcset(set)
		} else if set := a["data-srcset"]; set != "" {
			src = bestSrcset(set)
		}
		add(src, a["alt"])
	}
	for _, t := range sourceTag.FindAllString(html, -1) {
		if set := tagAttrs(t)["srcset"]; set != "" {
			add(bestSrcset(set), "")
		}
	}
	for _, m := range bgImageRe.FindAllStringSubmatch(html, -1) {
		add(m[1], "")
	}
	for _, want := range []string{"og:image", "twitter:image"} {
		for _, t := range metaTag.FindAllString(html, -1) {
			a := tagAttrs(t)
			if strings.EqualFold(a["property"], want) || strings.EqualFold(a["name"], want) {
				add(a["content"], "")
			}
		}
	}
	if len(out) > 500 {
		out = out[:500]
	}
	return out
}

/* SDOC: API
* POST /dnd/picture/scrape — Every Picture on a Page
	The pictures a web page has, for taking many at once (a page of tiles or tokens): links
	straight to pictures (in place of the thumbnails inside them), pictures, =srcset= at its
	widest, lazy-loaded ones (=data-src=), css backgrounds, and the page's own (=og:image=).
	Logos, icons, avatars and the like are left out, and so are svg pictures. A url that is
	itself a picture answers with just it. Nothing is kept: add the ones wanted with
	=POST /dnd/dungeon/pictures= and their =url=.

	*Method:* =POST=

	*Request Body:* ={url}=

	*Response:* ={Ok, Msg, Title, Pictures: [{url, name}]}=, at most 500.
	EDOC */
func PostDndPictureScrape(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	var req struct {
		Url string `json:"url"`
	}
	if !dndBody(w, r, &req) {
		return
	}
	u, err := url.Parse(strings.TrimSpace(req.Url))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		dndError(w, http.StatusBadRequest, "%q is not a web address", req.Url)
		return
	}
	b, at, err := fetchShow(u.String())
	if err != nil {
		dndError(w, http.StatusBadGateway, "could not fetch it: %s", err)
		return
	}
	if pictureExt(b) != "" {
		name := strings.TrimSuffix(path.Base(u.Path), path.Ext(u.Path))
		dndJson(w, map[string]interface{}{"Ok": true, "Title": name, "Pictures": []scrapedPicture{{Url: at, Name: safeShowName(name)}}})
		return
	}
	_, title := pagePicture(b, at)
	pics := scrapePictures(b, at)
	msg := ""
	if len(pics) == 0 {
		msg = "no pictures on that page (one built by its scripts shows none to a fetch)"
	}
	dndJson(w, map[string]interface{}{"Ok": true, "Msg": msg, "Title": title, "Pictures": pics})
}
