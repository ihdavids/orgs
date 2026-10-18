package orgs

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// The pictures a GM shows the table: an images folder beside the map's org
// file, its subfolders (npcs, monsters, players...) the categories. The map
// viewer lists them to search, shows one on its own screen and the players',
// and adds new ones - dropped, pasted, or fetched from a url (a page's own
// picture, as a link preview finds it, when the url is a page).

var showExts = map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".webp": true, ".gif": true, ".svg": true, ".avif": true}

// The most a picture added may weigh.
const showMaxBytes = 30 << 20

type showPicture struct {
	Path     string `json:"path"`           // relative to the org file: images/npcs/elf.png
	Name     string `json:"name"`           // elf
	Category string `json:"category"`       // npcs, or "" at the top of images
	Size     string `json:"size,omitempty"` // a token's: "large", else small
	// An animated token's still picture (a picture beside the video with its
	// name), for a thumbnail.
	Still  string `json:"still,omitempty"`
	Shared bool   `json:"shared,omitempty"` // from the shared folder (dndSharedPath)
}

// Tokens may also be videos: a looping animated token (a WebM with a clear
// background plays as one in a browser).
var videoExts = map[string]bool{".webm": true, ".mp4": true, ".m4v": true}

// The prefix a path in the shared folder is given, in place of a path
// relative to the map's org file: @shared/tokens/goblin.png.
const sharedPrefix = "@shared/"

// The shared folder (dndSharedPath), if one is set: relative to the first org
// directory, ~ for home.
func sharedDir() (string, bool) {
	if Conf().Server == nil {
		return "", false
	}
	p := strings.TrimSpace(Conf().Server.DndSharedPath)
	if p == "" {
		return "", false
	}
	if strings.HasPrefix(p, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(p, "~"), "/"))
		}
	}
	if !filepath.IsAbs(p) && len(Conf().Server.OrgDirs) > 0 {
		p = filepath.Join(Conf().Server.OrgDirs[0], p)
	}
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	return p, true
}

// walkShow lists one tokens or images folder: `top` the folder, `as` what its
// paths start with (tokens/, or @shared/tokens/). A video and a picture of the
// same name in one folder are one token: the video, with the picture its still.
func walkShow(top, as string, tokens bool, shared bool, cats map[string]bool) []showPicture {
	sizes := readTokenSizes(top)
	type found struct{ rel, name, ext string }
	byDir := map[string][]found{}
	zips := []showPicture{}
	filepath.WalkDir(top, func(p string, e os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if e.IsDir() {
			if strings.HasPrefix(e.Name(), ".") && p != top {
				return filepath.SkipDir
			}
			if rel, err := filepath.Rel(top, p); err == nil && rel != "." && !strings.Contains(rel, string(filepath.Separator)) {
				cats[rel] = true
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(p))
		if k, _ := archiveKind(e.Name()); k != "" {
			// An archive is a folder of its own (dndarchive.go).
			rel, _ := filepath.Rel(top, p)
			zips = append(zips, archivePictures(p, filepath.ToSlash(rel), as, tokens, sizes, shared, cats)...)
			return nil
		}
		if !showExts[ext] && !(tokens && videoExts[ext]) {
			return nil
		}
		rel, _ := filepath.Rel(top, p)
		rel = filepath.ToSlash(rel)
		byDir[filepath.Dir(rel)] = append(byDir[filepath.Dir(rel)], found{rel, strings.TrimSuffix(e.Name(), filepath.Ext(e.Name())), ext})
		return nil
	})
	out := []showPicture{}
	for _, list := range byDir {
		stills := map[string]string{}
		videos := map[string]bool{}
		for _, f := range list {
			if videoExts[f.ext] {
				videos[f.name] = true
			} else {
				stills[f.name] = f.rel
			}
		}
		for _, f := range list {
			if !videoExts[f.ext] && videos[f.name] {
				continue // the still of a video, listed with it
			}
			cat := ""
			if parts := strings.Split(f.rel, "/"); len(parts) > 1 {
				cat = parts[0]
			}
			pic := showPicture{Path: as + f.rel, Name: f.name, Category: cat, Size: sizes.Sizes[f.rel], Shared: shared}
			if videoExts[f.ext] && stills[f.name] != "" {
				pic.Still = as + stills[f.name]
			}
			out = append(out, pic)
		}
	}
	return append(out, zips...)
}

// resolveShow finds a picture or token by the path the map viewer has for
// it: relative to the map's org file (tokens/x.png), or in the shared folder
// (@shared/tokens/x.png). Answers the file, its folder's top, and whether the
// path is one.
func resolveShow(fname, folder, rel string) (string, string, bool) {
	if strings.Contains(rel, "..") {
		return "", "", false
	}
	if strings.HasPrefix(rel, sharedPrefix) {
		dir, ok := sharedDir()
		if !ok {
			return "", "", false
		}
		top := filepath.Join(dir, folder)
		p := filepath.Clean(filepath.Join(dir, filepath.FromSlash(strings.TrimPrefix(rel, sharedPrefix))))
		in, err := filepath.Rel(top, p)
		if err != nil || in == "." || strings.HasPrefix(in, "..") {
			return "", "", false
		}
		return p, top, true
	}
	return showPath(fname, folder, rel)
}

// Which folder beside the map: the pictures to show, or the tokens.
func showFolder(f string) (string, bool) {
	switch strings.TrimSpace(f) {
	case "", "images":
		return "images", true
	case "tokens":
		return "tokens", true
	}
	return "", false
}

// A tokens folder's sizes, by path inside it: tokens/tokens.json. A token not
// in it is small.
type tokenSizes struct {
	Sizes map[string]string `json:"sizes"`
}

func readTokenSizes(dir string) tokenSizes {
	var t tokenSizes
	if b, err := os.ReadFile(filepath.Join(dir, "tokens.json")); err == nil {
		json.Unmarshal(b, &t)
	}
	if t.Sizes == nil {
		t.Sizes = map[string]string{}
	}
	return t
}

func writeTokenSizes(dir string, t tokenSizes) error {
	b, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "tokens.json"), b, 0644)
}

/* SDOC: API
* GET /dnd/dungeon/pictures — A Map's Pictures to Show
	Every picture in the =images= folder beside a map's org file, and in its subfolders, which
	are its categories (=images/npcs/=, =images/monsters/=...). Fetch one with
	=GET /dnd/dungeon/image= and its =path=.

	*Method:* =GET=

	*Query Parameters:*
	| Parameter | Type   | Required | Description              |
	|-----------+--------+----------+--------------------------|
	| =file=    | string | yes      | The map's org file.      |
	| =folder=  | string | no       | =images= (the default) or =tokens=. |

	*Response:* ={Ok, Dir, Pictures: [{path, name, category, size, still, shared}], Categories, Shared}=;
	no folder is an empty list, not an error. A token's =size= is =large= when its folder's
	=tokens.json= says so. The shared folder's (=dndSharedPath=) come after the map's own, their
	paths starting =@shared/=; =Shared= says whether there is one. A token may be a video (=.webm=,
	=.mp4=), with =still= the picture of the same name beside it.
	EDOC */
func RequestDndDungeonPictures(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	fname, ok := insideOrgDirs(strings.TrimSpace(r.URL.Query().Get("file")))
	if !ok {
		dndError(w, http.StatusBadRequest, "the file has to be inside one of the org directories")
		return
	}
	folder, ok := showFolder(r.URL.Query().Get("folder"))
	if !ok {
		dndError(w, http.StatusBadRequest, "the folder is images or tokens")
		return
	}
	base := filepath.Dir(fname)
	dir := filepath.Join(base, folder)
	cats := map[string]bool{}
	pics := walkShow(dir, folder+"/", folder == "tokens", false, cats)
	sort.Slice(pics, func(i, j int) bool {
		if pics[i].Category != pics[j].Category {
			return pics[i].Category < pics[j].Category
		}
		return strings.ToLower(pics[i].Name) < strings.ToLower(pics[j].Name)
	})
	// The shared folder's after the map's own (unless they are the same).
	shared, hasShared := sharedDir()
	if hasShared {
		if top := filepath.Join(shared, folder); filepath.Clean(top) != filepath.Clean(dir) {
			more := walkShow(top, sharedPrefix+folder+"/", folder == "tokens", true, cats)
			sort.Slice(more, func(i, j int) bool {
				if more[i].Category != more[j].Category {
					return more[i].Category < more[j].Category
				}
				return strings.ToLower(more[i].Name) < strings.ToLower(more[j].Name)
			})
			pics = append(pics, more...)
		}
	}
	list := []string{}
	for c := range cats {
		list = append(list, c)
	}
	sort.Strings(list)
	dndJson(w, map[string]interface{}{"Ok": true, "Dir": dir, "Pictures": pics, "Categories": list, "Shared": hasShared})
}

// A name safe for a file or a folder: letters, digits, spaces and a little
// punctuation (no dots, no slashes), not too long.
var unsafeName = regexp.MustCompile(`[^\p{L}\p{N} _(),&'-]+`)

func safeShowName(s string) string {
	s = strings.TrimSpace(unsafeName.ReplaceAllString(s, " "))
	s = strings.Join(strings.Fields(s), " ")
	if len([]rune(s)) > 80 {
		s = string([]rune(s)[:80])
	}
	return strings.TrimSpace(s)
}

var showClient = &http.Client{Timeout: 25 * time.Second}

// fetchShow gets what a url points at, no more than the limit, as a browser
// would ask for it (some sites refuse anything else).
func fetchShow(u string) ([]byte, string, error) {
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36")
	req.Header.Set("Accept", "image/avif,image/webp,image/png,image/*,text/html;q=0.9,*/*;q=0.8")
	resp, err := showClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, "", fmt.Errorf("%s answered %s", req.URL.Host, resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, showMaxBytes+1))
	if err != nil {
		return nil, "", err
	}
	if len(b) > showMaxBytes {
		return nil, "", fmt.Errorf("that is more than %d MB", showMaxBytes>>20)
	}
	return b, resp.Request.URL.String(), nil
}

var (
	metaTag  = regexp.MustCompile(`(?is)<meta\s[^>]*>`)
	linkTag  = regexp.MustCompile(`(?is)<link\s[^>]*>`)
	imgTag   = regexp.MustCompile(`(?is)<img\s[^>]*>`)
	titleTag = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	attrRe   = regexp.MustCompile(`(?is)([a-z:_-]+)\s*=\s*("[^"]*"|'[^']*'|[^\s>]+)`)
)

func tagAttrs(tag string) map[string]string {
	out := map[string]string{}
	for _, m := range attrRe.FindAllStringSubmatch(tag, -1) {
		v := strings.Trim(m[2], `"'`)
		out[strings.ToLower(m[1])] = strings.ReplaceAll(v, "&amp;", "&")
	}
	return out
}

// pagePicture is the picture a page offers as its own - what a link preview
// shows: og:image, twitter:image, an image_src link, else its first picture.
// Also the page's title, for a name.
func pagePicture(page []byte, at string) (string, string) {
	html := string(page)
	title, ogTitle := "", ""
	if m := titleTag.FindStringSubmatch(html); m != nil {
		title = strings.TrimSpace(m[1])
	}
	found := ""
	for _, t := range metaTag.FindAllString(html, -1) {
		if a := tagAttrs(t); strings.EqualFold(a["property"], "og:title") && ogTitle == "" {
			ogTitle = strings.TrimSpace(a["content"])
		}
	}
	for _, want := range []string{"og:image:secure_url", "og:image", "og:image:url", "twitter:image", "twitter:image:src"} {
		for _, t := range metaTag.FindAllString(html, -1) {
			a := tagAttrs(t)
			if (strings.EqualFold(a["property"], want) || strings.EqualFold(a["name"], want)) && a["content"] != "" {
				found = a["content"]
				break
			}
		}
		if found != "" {
			break
		}
	}
	// The page's own name for itself, else its title without the site's name
	// on the end ("Owlbear - Wikipedia").
	if ogTitle != "" {
		title = ogTitle
	}
	for _, sep := range []string{" | ", " - ", " — ", " · "} {
		if i := strings.LastIndex(title, sep); i > 0 && len(title)-i-len(sep) <= 32 {
			title = strings.TrimSpace(title[:i])
			break
		}
	}
	if found == "" {
		for _, t := range linkTag.FindAllString(html, -1) {
			if a := tagAttrs(t); strings.EqualFold(a["rel"], "image_src") && a["href"] != "" {
				found = a["href"]
				break
			}
		}
	}
	if found == "" {
		for _, t := range imgTag.FindAllString(html, -1) {
			if a := tagAttrs(t); a["src"] != "" && !strings.HasPrefix(a["src"], "data:") {
				found = a["src"]
				break
			}
		}
	}
	if found == "" {
		return "", title
	}
	base, err := url.Parse(at)
	ref, err2 := url.Parse(found)
	if err != nil || err2 != nil {
		return "", title
	}
	return base.ResolveReference(ref).String(), title
}

/* SDOC: API
* POST /dnd/dungeon/pictures — Add a Picture to Show
	Writes a picture into the =images= folder beside a map's org file (made if missing), in a
	category's subfolder when one is named. The picture comes as =data= (base64, as dropped or
	pasted) or as a =url=: a picture's own url is fetched as it is, and a page's url has its
	picture found the way a link preview finds it (=og:image=, =twitter:image=, then its first
	picture). Only JPEG, PNG, WebP and GIF are kept, whatever the name says. A name already taken
	gets a number.

	*Method:* =POST=

	With =folder: tokens= it goes into the =tokens= folder instead, and =size: large= marks it a
	large token (two squares by two on the map).

	With =shared: true= it goes into the shared folder (=dndSharedPath=) instead, for every map.

	*Request Body:* ={file, folder, category, name, size, shared, data | url}=

	*Response:* ={Ok, Msg, Path, Name, Category}=
	EDOC */
func PostDndDungeonPictures(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	r.Body = http.MaxBytesReader(w, r.Body, showMaxBytes*2)
	var req struct {
		File     string `json:"file"`
		Category string `json:"category"`
		Name     string `json:"name"`
		Data     string `json:"data"`
		Url      string `json:"url"`
		Folder   string `json:"folder"`
		Size     string `json:"size"`
		Shared   bool   `json:"shared"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		dndError(w, http.StatusBadRequest, "could not read request: %s", err)
		return
	}
	// Where it goes: beside the map, or into the shared folder.
	var home, as string
	if req.Shared {
		dir, ok := sharedDir()
		if !ok {
			dndError(w, http.StatusBadRequest, "there is no shared folder: set dndSharedPath")
			return
		}
		home, as = dir, sharedPrefix
	} else {
		fname, ok := insideOrgDirs(strings.TrimSpace(req.File))
		if !ok || filepath.Ext(fname) != ".org" {
			dndError(w, http.StatusBadRequest, "the file has to be an org file inside one of the org directories")
			return
		}
		home = filepath.Dir(fname)
	}
	folder, ok := showFolder(req.Folder)
	if !ok {
		dndError(w, http.StatusBadRequest, "the folder is images or tokens")
		return
	}
	pic, name, code, err := takePicture(req.Data, req.Url, req.Name)
	if err != nil {
		dndError(w, code, "%s", err)
		return
	}
	ext := pictureExt(pic)
	if ext == "" {
		dndError(w, http.StatusUnsupportedMediaType, "that is not a JPEG, PNG, WebP or GIF picture")
		return
	}
	if len(pic) > showMaxBytes {
		dndError(w, http.StatusRequestEntityTooLarge, "that is more than %d MB", showMaxBytes>>20)
		return
	}
	top := filepath.Join(home, folder)
	dir := top
	cat := safeShowName(req.Category)
	if cat != "" {
		dir = filepath.Join(dir, cat)
	}
	if _, ok := insideOrgDirs(dir); !ok && !req.Shared {
		dndError(w, http.StatusBadRequest, "not a folder inside the org directories")
		return
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		dndError(w, http.StatusInternalServerError, "%s", err)
		return
	}
	stem := safeShowName(name)
	if stem == "" {
		stem = "picture " + time.Now().Format("2006-01-02 150405")
	}
	p := filepath.Join(dir, stem+ext)
	for i := 2; ; i++ {
		if _, err := os.Stat(p); os.IsNotExist(err) {
			break
		}
		p = filepath.Join(dir, fmt.Sprintf("%s %d%s", stem, i, ext))
	}
	if err := os.WriteFile(p, pic, 0644); err != nil {
		dndError(w, http.StatusInternalServerError, "%s", err)
		return
	}
	if folder == "tokens" && req.Size == "large" {
		inTop, _ := filepath.Rel(top, p)
		t := readTokenSizes(top)
		t.Sizes[filepath.ToSlash(inTop)] = "large"
		writeTokenSizes(top, t)
	}
	rel, _ := filepath.Rel(home, p)
	dndJson(w, map[string]interface{}{"Ok": true, "Msg": "Added " + filepath.Base(p),
		"Path": as + filepath.ToSlash(rel), "Name": strings.TrimSuffix(filepath.Base(p), ext), "Category": cat})
}

// takePicture is a picture sent as base64, or fetched from a url (a page's own
// picture, when the url is a page), and a name for it: the one asked for, else
// the page's title or the picture's file name. The status is for an error.
func takePicture(data, link, name string) ([]byte, string, int, error) {
	switch {
	case data != "":
		d := data
		if i := strings.Index(d, ","); strings.HasPrefix(d, "data:") && i > 0 {
			d = d[i+1:]
		}
		b, err := base64.StdEncoding.DecodeString(d)
		if err != nil {
			return nil, "", http.StatusBadRequest, fmt.Errorf("the picture is not base64: %s", err)
		}
		return b, name, 0, nil
	case link != "":
		u, err := url.Parse(strings.TrimSpace(link))
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return nil, "", http.StatusBadRequest, fmt.Errorf("%q is not a web address", link)
		}
		b, at, err := fetchShow(u.String())
		if err != nil {
			return nil, "", http.StatusBadGateway, fmt.Errorf("could not fetch it: %s", err)
		}
		if pictureExt(b) == "" {
			// A page: find its picture.
			src, title := pagePicture(b, at)
			if src == "" {
				return nil, "", http.StatusUnprocessableEntity, fmt.Errorf("that page has no picture to take")
			}
			if b, _, err = fetchShow(src); err != nil {
				return nil, "", http.StatusBadGateway, fmt.Errorf("could not fetch its picture: %s", err)
			}
			if name == "" {
				name = title
			}
			u, _ = url.Parse(src)
		}
		if name == "" && u != nil {
			name = strings.TrimSuffix(filepath.Base(u.Path), filepath.Ext(u.Path))
		}
		return b, name, 0, nil
	}
	return nil, "", http.StatusBadRequest, fmt.Errorf("send the picture as data or a url")
}

/* SDOC: API
* POST /dnd/picture/fetch — Fetch a Picture from the Web
	What =POST /dnd/dungeon/pictures= does with a url, without keeping it: the picture, or a
	page's own picture, handed back as a data url, so the token maker can work on it in the
	browser (a picture from another site would taint its canvas).

	*Method:* =POST=

	*Request Body:* ={url}=

	*Response:* ={Ok, Msg, Data, Name}=, =Data= a =data:= url.
	EDOC */
func PostDndPictureFetch(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	var req struct {
		Url string `json:"url"`
	}
	if !dndBody(w, r, &req) {
		return
	}
	b, name, code, err := takePicture("", req.Url, "")
	if err != nil {
		dndError(w, code, "%s", err)
		return
	}
	ext := pictureExt(b)
	if ext == "" {
		dndError(w, http.StatusUnsupportedMediaType, "that is not a JPEG, PNG, WebP or GIF picture")
		return
	}
	mime := map[string]string{".jpg": "image/jpeg", ".png": "image/png", ".webp": "image/webp", ".gif": "image/gif"}[ext]
	dndJson(w, map[string]interface{}{"Ok": true, "Name": safeShowName(name), "Data": "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(b)})
}

// A picture or token beside a map, by its path relative to the org file,
// inside the folder named.
func showPath(fname, folder, rel string) (string, string, bool) {
	top := filepath.Join(filepath.Dir(fname), folder)
	p := filepath.Clean(filepath.Join(filepath.Dir(fname), filepath.FromSlash(rel)))
	in, err := filepath.Rel(top, p)
	if err != nil || in == "." || strings.HasPrefix(in, "..") || strings.Contains(rel, "..") {
		return "", "", false
	}
	return p, top, true
}

/* SDOC: API
* POST /dnd/dungeon/pictures/edit — Size or Put Away a Token or Picture
	For a picture or token beside a map (its =path= relative to the org file): =size= (=small=
	or =large=) sets a token's size in its folder's =tokens.json=; =delete: true= moves it into
	a hidden =.deleted= folder in its folder (never listed, so it can be fished back out).

	*Method:* =POST=

	*Request Body:* ={file, folder, path, size, delete}=

	*Response:* ={Ok, Msg}=
	EDOC */
func PostDndDungeonPictureEdit(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	var req struct {
		File   string `json:"file"`
		Folder string `json:"folder"`
		Path   string `json:"path"`
		Size   string `json:"size"`
		Delete bool   `json:"delete"`
	}
	if !dndBody(w, r, &req) {
		return
	}
	fname, ok := insideOrgDirs(strings.TrimSpace(req.File))
	if (!ok || filepath.Ext(fname) != ".org") && !strings.HasPrefix(req.Path, sharedPrefix) {
		dndError(w, http.StatusBadRequest, "the file has to be an org file inside one of the org directories")
		return
	}
	folder, ok := showFolder(req.Folder)
	if !ok {
		dndError(w, http.StatusBadRequest, "the folder is images or tokens")
		return
	}
	p, top, ok := resolveShow(fname, folder, req.Path)
	if !ok {
		dndError(w, http.StatusBadRequest, "not a picture in the %s folder", folder)
		return
	}
	_, inZip, zipped := splitArchive(p)
	if _, err := os.Stat(p); err != nil && !(zipped && inZip != "") {
		dndError(w, http.StatusNotFound, "no picture at %s", req.Path)
		return
	}
	in, _ := filepath.Rel(top, p)
	key := filepath.ToSlash(in)
	if req.Delete && zipped {
		dndError(w, http.StatusBadRequest, "that one is inside an archive: take it out of the archive to put it away")
		return
	}
	if req.Delete {
		bin := filepath.Join(top, ".deleted")
		if err := os.MkdirAll(bin, 0755); err != nil {
			dndError(w, http.StatusInternalServerError, "%s", err)
			return
		}
		to := filepath.Join(bin, time.Now().Format("20060102-150405 ")+filepath.Base(p))
		if err := os.Rename(p, to); err != nil {
			dndError(w, http.StatusInternalServerError, "%s", err)
			return
		}
		t := readTokenSizes(top)
		if _, had := t.Sizes[key]; had {
			delete(t.Sizes, key)
			writeTokenSizes(top, t)
		}
		dndJson(w, map[string]interface{}{"Ok": true, "Msg": "Put away " + filepath.Base(p)})
		return
	}
	if folder != "tokens" || (req.Size != "small" && req.Size != "large") {
		dndError(w, http.StatusBadRequest, "a token's size is small or large")
		return
	}
	t := readTokenSizes(top)
	if req.Size == "large" {
		t.Sizes[key] = "large"
	} else {
		delete(t.Sizes, key)
	}
	if err := writeTokenSizes(top, t); err != nil {
		dndError(w, http.StatusInternalServerError, "%s", err)
		return
	}
	dndJson(w, map[string]interface{}{"Ok": true, "Msg": filepath.Base(p) + " is " + req.Size})
}
