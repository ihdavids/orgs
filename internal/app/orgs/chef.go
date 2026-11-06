package orgs

// org-chef (features.chef): a recipe fetched from a web page into a heading
// in org-chef's own layout, so a cookbook file is the same whichever of
// Emacs or orgs wrote it:
//
//	* Weeknight Chicken Curry
//	:PROPERTIES:
//	:source-url: https://spiceandsimmer.example/chicken-curry/
//	:servings:   4
//	:prep-time:  15 min
//	:cook-time:  25 min
//	:ready-in:   40 min
//	:IMAGE:      [[file:images/recipes/weeknight-chicken-curry.jpg]]
//	:RECORD:     recipe
//	:END:
//	** Ingredients
//	- 2 tablespoons vegetable oil
//	** Directions
//	1. Heat the oil in a large pan over medium heat.
//
// The page is read for its schema.org Recipe (JSON-LD, else microdata) by
// internal/common/recipe, which is pure and pinned by saved pages. A heading
// is a recipe when it has org-chef's :source-url:, :RECORD: recipe (what
// orgs adds, so the records browser lists them), or both an Ingredients and
// a Directions child (a cookbook typed by hand).
//
// Scaling and the shopping list are worked out here only - clients ask - so
// the quantity rules have no TypeScript twin.

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
	"github.com/ihdavids/orgs/internal/common/recipe"
)

const (
	chefPictureDir = "images/recipes"
	chefPageCap    = 5 << 20
	chefImageCap   = 10 << 20
	chefUserAgent  = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36"
)

func registerChefRoutes(api *mux.Router) {
	api.HandleFunc("/chef/fetch", PostChefFetch).Methods("POST")
	api.HandleFunc("/chef/add", PostChefAdd).Methods("POST")
	api.HandleFunc("/chef/recipes", RequestChefRecipes).Methods("GET")
	api.HandleFunc("/chef/scale", RequestChefScale).Methods("GET")
	api.HandleFunc("/chef/shop", PostChefShop).Methods("POST")
}

func chefSettings() common.ChefSettings {
	if s := Conf().Server; s != nil {
		return s.Chef
	}
	return common.ChefSettings{}
}

var chefClient = &http.Client{
	Timeout: 15 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("too many redirects")
		}
		if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
			return fmt.Errorf("redirected to a %s url", req.URL.Scheme)
		}
		return nil
	},
}

func chefURL(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("no url")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("%q is not a url", raw)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("only http and https pages can be fetched, not %s", u.Scheme)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("%q has no host", raw)
	}
	return u, nil
}

// chefGet fetches a url with a size cap, the way a browser would ask for it.
func chefGet(u string, accept string, limit int64) ([]byte, string, string, error) {
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, "", "", err
	}
	req.Header.Set("User-Agent", chefUserAgent)
	req.Header.Set("Accept", accept)
	req.Header.Set("Accept-Language", "en;q=0.9,*;q=0.5")
	resp, err := chefClient.Do(req)
	if err != nil {
		return nil, "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, "", "", fmt.Errorf("%s answered %s", resp.Request.URL.Host, resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, "", "", err
	}
	if int64(len(b)) > limit {
		return nil, "", "", fmt.Errorf("%s is larger than %d MB", u, limit>>20)
	}
	return b, resp.Request.URL.String(), resp.Header.Get("Content-Type"), nil
}

// chefFetch reads the recipe from a url, or from a page's html the client
// already has (a site that turns servers away still shows a browser the page).
func chefFetch(rawURL, page string) (*recipe.Recipe, error) {
	if strings.TrimSpace(page) != "" {
		return recipe.Parse([]byte(page), strings.TrimSpace(rawURL))
	}
	u, err := chefURL(rawURL)
	if err != nil {
		return nil, err
	}
	b, final, _, err := chefGet(u.String(), "text/html,application/xhtml+xml;q=0.9,*/*;q=0.8", chefPageCap)
	if err != nil {
		return nil, err
	}
	r, err := recipe.Parse(b, final)
	if err != nil {
		return nil, err
	}
	// The url asked for, not where a tracking redirect ended, unless the
	// page says its own.
	if r.Url == final && final != u.String() {
		r.Url = u.String()
	}
	return r, nil
}

var chefImageExt = map[string]string{"image/jpeg": "jpg", "image/jpg": "jpg", "image/png": "png", "image/gif": "gif", "image/webp": "webp", "image/avif": "avif"}

// chefPicture fetches a recipe's picture: an http(s) url or a data: url.
func chefPicture(src string) ([]byte, string, error) {
	if strings.HasPrefix(src, "data:") {
		comma := strings.IndexByte(src, ',')
		if comma < 0 || !strings.Contains(src[:comma], ";base64") {
			return nil, "", fmt.Errorf("an unreadable data: picture")
		}
		ct := strings.TrimPrefix(strings.SplitN(src[:comma], ";", 2)[0], "data:")
		ext := chefImageExt[ct]
		if ext == "" {
			return nil, "", fmt.Errorf("a %s is not a picture", ct)
		}
		b, err := base64.StdEncoding.DecodeString(src[comma+1:])
		return b, ext, err
	}
	u, err := chefURL(src)
	if err != nil {
		return nil, "", err
	}
	b, _, ct, err := chefGet(u.String(), "image/avif,image/webp,image/png,image/jpeg,image/*;q=0.8", chefImageCap)
	if err != nil {
		return nil, "", err
	}
	mt, _, _ := mime.ParseMediaType(ct)
	ext := chefImageExt[strings.ToLower(mt)]
	if ext == "" {
		ct = http.DetectContentType(b)
		ext = chefImageExt[strings.SplitN(ct, ";", 2)[0]]
	}
	if ext == "" {
		return nil, "", fmt.Errorf("%s is not a picture (%s)", u.Host, ct)
	}
	return b, ext, nil
}

// chefSavePicture writes a picture beside the org file it is linked from, as
// vCard photos are; the same bytes already there are reused.
func chefSavePicture(orgFile, name string, data []byte, ext string) (string, error) {
	dir := filepath.Join(filepath.Dir(orgFile), filepath.FromSlash(chefPictureDir))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	slug := photoSlug(name)
	for n := 1; n < 1000; n++ {
		fname := slug + "." + ext
		if n > 1 {
			fname = fmt.Sprintf("%s-%d.%s", slug, n, ext)
		}
		path := filepath.Join(dir, fname)
		if have, err := os.ReadFile(path); err == nil {
			if samePhoto(have, data) {
				return "[[file:" + chefPictureDir + "/" + fname + "]]", nil
			}
			continue
		}
		if err := os.WriteFile(path, data, 0644); err != nil {
			return "", err
		}
		return "[[file:" + chefPictureDir + "/" + fname + "]]", nil
	}
	return "", fmt.Errorf("too many pictures called %s", slug)
}

func chefOptions(lvl int) recipe.Options {
	s := chefSettings()
	return recipe.Options{Level: lvl, Record: !s.NoRecord, Checkboxes: s.Checkboxes}
}

// ---------------------------------------------------------------------------
// Which headings are recipes
// ---------------------------------------------------------------------------

func chefProp(sec *org.Section, name string) (string, bool) {
	if sec == nil || sec.Headline == nil || sec.Headline.Properties == nil {
		return "", false
	}
	for _, p := range sec.Headline.Properties.Properties {
		if strings.EqualFold(strings.TrimSpace(p[0]), name) {
			return strings.TrimSpace(p[1]), true
		}
	}
	return "", false
}

func isRecipeSection(sec *org.Section) bool {
	if !isRealHeading(sec) {
		return false
	}
	if _, ok := chefProp(sec, "source-url"); ok {
		return true
	}
	if strings.EqualFold(RecordTypeOf(sec), "recipe") {
		return true
	}
	ing, dir := false, false
	for _, c := range sec.Children {
		t := strings.ToLower(sectionTitle(c))
		switch {
		case strings.HasPrefix(t, "ingredient"):
			ing = true
		case strings.HasPrefix(t, "direction") || strings.HasPrefix(t, "instruction") || t == "method" || t == "steps":
			dir = true
		}
	}
	return ing && dir
}

// ChefRecipe is a recipe heading: where it is, and the recipe read off it.
type ChefRecipe struct {
	Hash     string
	Filename string
	Line     int
	Tags     []string `json:",omitempty"`
	Picture  string   `json:",omitempty"` // a path or url a client can show
	recipe.Recipe
}

func chefRead(f *common.OrgFile, sec *org.Section) (*ChefRecipe, bool) {
	lines, from, to, ok := recordLines(f.Filename, sec)
	if !ok {
		return nil, false
	}
	_, props := sectionProps(sec)
	r := recipe.FromOrg(sectionTitle(sec), props, lines[from:to+1])
	out := &ChefRecipe{Hash: sec.Hash, Filename: f.Filename, Line: sec.Headline.Pos.Row, Tags: sec.Headline.Tags, Recipe: *r}
	if r.Image != "" {
		out.Picture = fieldLink("image", "IMAGE", r.Image, f.Filename)
	}
	if out.Ingredients == nil {
		out.Ingredients = []string{}
	}
	if out.Directions == nil {
		out.Directions = []recipe.Step{}
	}
	return out, true
}

var (
	chefCacheMu  sync.Mutex
	chefCacheAt  uint64
	chefCacheDb  *OrgDb
	chefCacheAll []*ChefRecipe
)

// chefAll is every recipe heading, cached on ReloadIndex (see Traps: caches).
func chefAll() []*ChefRecipe {
	chefCacheMu.Lock()
	defer chefCacheMu.Unlock()
	db := GetDb()
	if chefCacheAll != nil && chefCacheDb == db && chefCacheAt == db.ReloadIndex {
		return chefCacheAll
	}
	at := db.ReloadIndex
	out := []*ChefRecipe{}
	walkSections(func(f *common.OrgFile, sec *org.Section) {
		if isArchiveFile(f.Filename) || !isRecipeSection(sec) {
			return
		}
		if r, ok := chefRead(f, sec); ok {
			out = append(out, r)
		}
	})
	chefCacheAll, chefCacheAt, chefCacheDb = out, at, db
	return out
}

func chefMatches(r *ChefRecipe, q string) bool {
	hay := strings.ToLower(strings.Join(append([]string{r.Name, r.Category, r.Cuisine, r.Description, strings.Join(r.Tags, " "), strings.Join(r.Keywords, " ")}, r.Ingredients...), " "))
	for _, w := range strings.Fields(strings.ToLower(q)) {
		if !strings.Contains(hay, w) {
			return false
		}
	}
	return true
}

// chefFind is one recipe heading by hash, read fresh.
func chefFind(hash string) (*ChefRecipe, error) {
	sec := GetDb().FindByHash(hash)
	if sec == nil || sec.Headline == nil {
		return nil, fmt.Errorf("no heading with that hash - it may have moved; refresh and try again")
	}
	f := GetDb().FileFromSection(sec)
	if f == nil {
		return nil, fmt.Errorf("could not find the file that heading is in")
	}
	r, ok := chefRead(f, sec)
	if !ok {
		return nil, fmt.Errorf("could not read %s", f.Filename)
	}
	return r, nil
}

// ---------------------------------------------------------------------------
// Where a recipe or a list goes
// ---------------------------------------------------------------------------

// chefTarget is the file and heading to write under. under is a heading
// hash, else a title or outline path (a::b) in the file - or, with no file,
// the one heading of that title anywhere. No file at all is chef.file
// (recipes.org), made in the first org directory if missing.
func chefTarget(file, under string) (importTargetAt, error) {
	file, under = strings.TrimSpace(file), strings.TrimSpace(under)
	if under != "" {
		if sec := GetDb().FindByHash(under); sec != nil && sec.Headline != nil {
			return importTarget("", under, "")
		}
	}
	if file == "" && under != "" {
		// A heading named with no file: wherever it is, when it is one.
		hits := []*org.Section{}
		walkSections(func(f *common.OrgFile, sec *org.Section) {
			if !isArchiveFile(f.Filename) && strings.EqualFold(sectionTitle(sec), under) {
				hits = append(hits, sec)
			}
		})
		if len(hits) == 1 {
			return importTarget("", hits[0].Hash, "")
		}
		if len(hits) > 1 {
			return importTargetAt{}, fmt.Errorf("%d headings are called %q - say which file", len(hits), under)
		}
	}
	if file == "" {
		file = chefSettings().File
		if file == "" {
			file = "recipes.org"
		}
		if f := GetDb().FindByFile(file); f == nil {
			path := file
			if !filepath.IsAbs(path) && Conf().Server != nil && len(Conf().Server.OrgDirs) > 0 {
				path = filepath.Join(Conf().Server.OrgDirs[0], file)
			}
			if _, err := os.Stat(path); os.IsNotExist(err) {
				if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
					return importTargetAt{}, err
				}
				if err := os.WriteFile(path, []byte("#+TITLE: Recipes\n"), 0644); err != nil {
					return importTargetAt{}, err
				}
			}
			GetDb().LoadFile(path, true)
			file = path
		}
	}
	f := GetDb().FindByFile(file)
	if f == nil {
		return importTargetAt{}, fmt.Errorf("no file called %q", file)
	}
	if under != "" {
		_, sec := GetDb().GetFromTarget(&common.Target{Type: "file+olp", Filename: f.Filename, Id: under}, false)
		if sec == nil || sec.Headline == nil {
			_, sec = GetDb().GetFromTarget(&common.Target{Type: "file+headline", Filename: f.Filename, Id: under}, false)
		}
		if sec == nil || sec.Headline == nil {
			return importTargetAt{}, fmt.Errorf("no heading %q in %s", under, filepath.Base(f.Filename))
		}
		return importTarget("", sec.Hash, "")
	}
	return importTarget("", "", f.Filename)
}

// chefInsert splices lines in at a target and answers with the new heading.
func chefInsert(at importTargetAt, add []string) (string, int, error) {
	lines, err := ganttFileLines(at.filename)
	if err != nil {
		return "", 0, err
	}
	add = spaceBefore(lines, at.row, add)
	head := at.row + 1
	if len(add) > 0 && add[0] == "" {
		head++
	}
	if _, err := insertLinesAt(at.filename, at.row, add); err != nil {
		return "", 0, err
	}
	hash := ""
	if f := GetDb().FindByFile(at.filename); f != nil {
		if sec := sectionAtRow(f, head); sec != nil && sec.Headline != nil && sec.Headline.Pos.Row == head {
			hash = sec.Hash
		}
	}
	return hash, head, nil
}

// ---------------------------------------------------------------------------
// Endpoints
// ---------------------------------------------------------------------------

type chefAnswer struct {
	Ok       bool
	Msg      string   `json:",omitempty"`
	Warnings []string `json:",omitempty"`
}

func chefBody(w http.ResponseWriter, r *http.Request, v interface{}) bool {
	body, _ := io.ReadAll(io.LimitReader(r.Body, chefPageCap+1<<20))
	if err := json.Unmarshal(body, v); err != nil {
		dynJson(w, chefAnswer{Msg: err.Error()})
		return false
	}
	return true
}

/* SDOC: API
* POST /chef/fetch — Read A Recipe Off A Page
* POST /chef/add — Write One Into A File
* GET /chef/recipes — Every Recipe
* GET /chef/scale — One Recipe, For So Many
* POST /chef/shop — A Shopping List From Recipes

	#+BEGIN_EXAMPLE
	POST /chef/fetch   {"Url": "https://..."}  (or "Html": the page, for a site that refuses servers)
	                   {"Ok": true, "Recipe": {Name, Url, Image, Yield, Servings, PrepTime, CookTime,
	                    TotalTime, Ingredients: [...], Directions: [{Section, Text}]}, "Org": "* ..."}
	POST /chef/add     {"Url": "...", "File": "recipes.org", "Under": "Mains", "NoPicture": false,
	                    "Force": false}   (or "Recipe": {...} instead of Url)
	                   {"Ok": true, "Hash", "Filename", "Line", "Recipe", "Warnings": [...]}
	GET  /chef/recipes?q=chicken   {"Ok": true, "Recipes": [{Hash, Filename, Line, Tags, Picture, Name, ...}]}
	GET  /chef/scale?hash=...&servings=6   (or &factor=2)
	                   {"Ok": true, "Recipe": {... Ingredients scaled}, "From": 4, "Servings": 6, "Factor": 1.5}
	POST /chef/shop    {"Hashes": [...], "Servings": 6, "Each": {"<hash>": 2}, "File": "...", "Under": "...",
	                    "Preview": true}
	                   {"Ok": true, "Items": [{Text, From: [recipe names]}], "Hash", "Filename", "Line"}
	#+END_EXAMPLE

	A page is read for its schema.org =Recipe= (JSON-LD, including =@graph=,
	=HowToSection= and =HowToStep=; microdata when there is none). Fetching
	is http and https only, 15 seconds, 5 MB. =/chef/fetch= writes nothing:
	=Org= is the heading =/chef/add= would write. =/chef/add= writes it in
	org-chef's layout (=:source-url:=, =:servings:=, =:prep-time:=,
	=:cook-time:=, =:ready-in:=, =** Ingredients=, =** Directions=) plus
	=:RECORD: recipe=, saving the picture in =images/recipes/= beside the
	org file. =Under= is a heading hash, title or outline path in =File=
	(with no =File=, the one heading of that title anywhere); no =File= is
	=chef.file= (=recipes.org=, made if missing). A page already
	in a file (same =:source-url:=) is refused unless =Force=.

	A recipe is a heading with =:source-url:=, =:RECORD: recipe=, or both an
	Ingredients and a Directions child. Scaling multiplies each
	ingredient's leading amount (=1/2=, =½=, =1 1/2=, =0.5=, =2-3=), keeping
	its spelling and units; a recipe without servings scales by =factor=.
	The shopping list adds up the same ingredient in the same unit across
	recipes and is written as a heading of =- [ ]= items with a =[0/n]=
	cookie (=Preview= only answers). =Servings= scales every recipe to that
	many, =Each= one recipe by its hash. Needs =features: chef:=.
EDOC */
func PostChefFetch(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "chef") {
		return
	}
	var req struct{ Url, Html string }
	if !chefBody(w, r, &req) {
		return
	}
	rec, err := chefFetch(req.Url, req.Html)
	if err != nil {
		dynJson(w, chefAnswer{Msg: err.Error()})
		return
	}
	opts := chefOptions(1)
	opts.ImageUrl = true
	dynJson(w, map[string]interface{}{"Ok": true, "Recipe": rec, "Org": strings.Join(recipe.ToOrg(rec, opts), "\n") + "\n"})
}

func PostChefAdd(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "chef") {
		return
	}
	var req struct {
		Url, Html, File, Under string
		Recipe                 *recipe.Recipe
		NoPicture, Force       bool
	}
	if !chefBody(w, r, &req) {
		return
	}
	res, err := chefAdd(req.Url, req.Html, req.Recipe, req.File, req.Under, req.NoPicture, req.Force)
	if err != nil {
		dynJson(w, chefAnswer{Msg: err.Error()})
		return
	}
	dynJson(w, res)
}

// ChefAddResult is what /chef/add wrote.
type ChefAddResult struct {
	Ok       bool
	Msg      string `json:",omitempty"`
	Hash     string
	Filename string
	Line     int
	Existing string `json:",omitempty"` // the hash of the copy already there, when refused
	Recipe   *recipe.Recipe
	Warnings []string `json:",omitempty"`
}

func chefAdd(rawURL, page string, rec *recipe.Recipe, file, under string, noPicture, force bool) (ChefAddResult, error) {
	var err error
	if rec == nil {
		if rec, err = chefFetch(rawURL, page); err != nil {
			return ChefAddResult{}, err
		}
	}
	if strings.TrimSpace(rec.Name) == "" {
		return ChefAddResult{}, fmt.Errorf("a recipe needs a name")
	}
	if !force && rec.Url != "" {
		for _, have := range chefAll() {
			if have.Url != "" && strings.TrimRight(have.Url, "/") == strings.TrimRight(rec.Url, "/") {
				return ChefAddResult{Msg: fmt.Sprintf("%q is already in %s (line %d) - add it again with Force", have.Name, filepath.Base(have.Filename), have.Line+1),
					Existing: have.Hash, Filename: have.Filename, Line: have.Line, Recipe: rec}, nil
			}
		}
	}
	at, err := chefTarget(file, under)
	if err != nil {
		return ChefAddResult{}, err
	}
	res := ChefAddResult{Recipe: rec, Filename: at.filename}
	opts := chefOptions(at.lvl)
	if rec.Image != "" {
		switch {
		case strings.HasPrefix(rec.Image, "[[") || (!strings.Contains(rec.Image, "://") && !strings.HasPrefix(rec.Image, "data:")):
			// Already a link in some file (a recipe read back and added again).
			opts.ImageUrl = true
		case noPicture || chefSettings().NoPictures:
			opts.ImageUrl = true
		default:
			data, ext, err := chefPicture(rec.Image)
			if err == nil {
				opts.ImageLink, err = chefSavePicture(at.filename, rec.Name, data, ext)
			}
			if err != nil {
				res.Warnings = append(res.Warnings, "the picture was not saved: "+err.Error())
				opts.ImageUrl = true
			}
		}
	}
	hash, line, err := chefInsert(at, recipe.ToOrg(rec, opts))
	if err != nil {
		return ChefAddResult{}, err
	}
	res.Ok, res.Hash, res.Line = true, hash, line
	res.Msg = fmt.Sprintf("%s added to %s", rec.Name, filepath.Base(at.filename))
	return res, nil
}

func RequestChefRecipes(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "chef") {
		return
	}
	q := r.URL.Query().Get("q")
	out := []*ChefRecipe{}
	for _, rec := range chefAll() {
		if q == "" || chefMatches(rec, q) {
			out = append(out, rec)
		}
	}
	dynJson(w, map[string]interface{}{"Ok": true, "Recipes": out})
}

func RequestChefScale(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "chef") {
		return
	}
	q := r.URL.Query()
	rec, err := chefFind(q.Get("hash"))
	if err != nil {
		dynJson(w, chefAnswer{Msg: err.Error()})
		return
	}
	servings, _ := strconv.ParseFloat(q.Get("servings"), 64)
	factor, _ := strconv.ParseFloat(q.Get("factor"), 64)
	scaled, f := recipe.Scale(&rec.Recipe, servings, factor)
	out := *rec
	out.Recipe = *scaled
	msg := ""
	if servings > 0 && rec.Servings == 0 {
		msg = "this recipe does not say how many it serves, so it is shown as written (scale it by a factor instead)"
	}
	dynJson(w, map[string]interface{}{"Ok": true, "Msg": msg, "Recipe": out, "From": rec.Servings, "Servings": scaled.Servings, "Factor": f})
}

// ChefShopRequest is a shopping list asked for.
type ChefShopRequest struct {
	Hashes   []string
	Servings float64
	Each     map[string]float64
	File     string
	Under    string
	Preview  bool
}

func PostChefShop(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "chef") {
		return
	}
	var req ChefShopRequest
	if !chefBody(w, r, &req) {
		return
	}
	res, err := chefShop(&req)
	if err != nil {
		dynJson(w, chefAnswer{Msg: err.Error()})
		return
	}
	dynJson(w, res)
}

// ChefShopResult is a shopping list, and where it was written.
type ChefShopResult struct {
	Ok       bool
	Msg      string `json:",omitempty"`
	Items    []recipe.ShopItem
	Hash     string `json:",omitempty"`
	Filename string `json:",omitempty"`
	Line     int
}

func chefShop(req *ChefShopRequest) (ChefShopResult, error) {
	if len(req.Hashes) == 0 {
		return ChefShopResult{}, fmt.Errorf("no recipes to shop for")
	}
	lists := map[string][]string{}
	order := []string{}
	firstFile := ""
	for _, h := range req.Hashes {
		rec, err := chefFind(h)
		if err != nil {
			return ChefShopResult{}, err
		}
		if firstFile == "" {
			firstFile = rec.Filename
		}
		servings, factor := req.Servings, 0.0
		if v, ok := req.Each[h]; ok && v > 0 {
			servings, factor = 0, v
		}
		scaled, _ := recipe.Scale(&rec.Recipe, servings, factor)
		name := rec.Name
		for n := 2; lists[name] != nil; n++ {
			name = fmt.Sprintf("%s (%d)", rec.Name, n)
		}
		lists[name] = scaled.Ingredients
		order = append(order, name)
	}
	items := recipe.Shop(lists, order)
	res := ChefShopResult{Ok: true, Items: items}
	if req.Preview {
		return res, nil
	}
	file, under := req.File, req.Under
	if file == "" && under == "" {
		// A Shopping heading in the chef file, else in the first recipe's
		// file, else the end of the first recipe's file. Never a new file.
		file = firstFile
		for _, name := range []string{chefSettings().File, "recipes.org", firstFile} {
			if f := GetDb().FindByFile(name); name != "" && f != nil {
				if _, sec := GetDb().GetFromTarget(&common.Target{Type: "file+headline", Filename: f.Filename, Id: "Shopping"}, false); sec != nil && sec.Headline != nil {
					file, under = f.Filename, "Shopping"
					break
				}
			}
		}
	}
	at, err := chefTarget(file, under)
	if err != nil {
		return ChefShopResult{}, err
	}
	add := []string{fmt.Sprintf("%s Shopping: %s [0/%d]", strings.Repeat("*", at.lvl), strings.Join(order, ", "), len(items))}
	for _, it := range items {
		add = append(add, "- [ ] "+it.Text)
	}
	hash, line, err := chefInsert(at, add)
	if err != nil {
		return ChefShopResult{}, err
	}
	res.Hash, res.Filename, res.Line = hash, at.filename, line
	res.Msg = fmt.Sprintf("%d item%s on the list in %s", len(items), plural(len(items)), filepath.Base(at.filename))
	return res, nil
}
