package orgs

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ihdavids/orgs/internal/common"
)

func chefSite(t *testing.T) *httptest.Server {
	t.Helper()
	page, err := os.ReadFile("../../common/recipe/testdata/bigsite.html")
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{255, 0, 0, 255})
	var pic bytes.Buffer
	png.Encode(&pic, img)
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/burgers":
			// The picture on the page points at this server.
			w.Header().Set("Content-Type", "text/html")
			w.Write(bytes.ReplaceAll(page, []byte("https://www.bigsite.example/thmb/burgers.jpg"), []byte(srv.URL+"/burgers.png")))
		case "/burgers.png":
			w.Header().Set("Content-Type", "image/png")
			w.Write(pic.Bytes())
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestChefAddScaleShop(t *testing.T) {
	withFeatures(t, func(f *common.FeatureSettings) { f.Chef = true })
	srv := chefSite(t)
	paths := loadOrg(t, map[string]string{"cook.org": `#+TITLE: Cookbook
* Mains
** Pancakes
:PROPERTIES:
:source-url: https://pancakes.example/
:servings: 2
:END:
*** Ingredients
- 1 cup flour
- 1 egg
*** Directions
1. Mix.
* Shopping
`})
	p := paths["cook.org"]
	res, err := chefAdd(srv.URL+"/burgers", "", nil, p, "Mains", false, false)
	if err != nil || !res.Ok {
		t.Fatalf("%v %+v", err, res)
	}
	got := readOrg(t, p)
	for _, want := range []string{
		"** Homemade Black Bean Veggie Burgers\n:PROPERTIES:\n:source-url: https://www.bigsite.example/recipe/85452/homemade-black-bean-veggie-burgers/",
		":IMAGE:      [[file:images/recipes/homemade-black-bean-veggie-burgers.png]]",
		":RECORD:     recipe\n:END:\n\n*** Ingredients\n- 1 (16 ounce) can black beans",
		"*** Directions\n1. If grilling",
		"\n* Shopping",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in\n%s", want, got)
		}
	}
	if strings.Index(got, "Veggie Burgers") > strings.Index(got, "* Shopping") {
		t.Fatalf("not under Mains:\n%s", got)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(p), "images/recipes/homemade-black-bean-veggie-burgers.png")); err != nil {
		t.Fatalf("picture: %v", err)
	}
	if res.Hash == "" {
		t.Error("no hash for the new heading")
	}

	// The same page again is refused.
	again, err := chefAdd(srv.URL+"/burgers", "", nil, p, "Mains", false, false)
	if err != nil || again.Ok || again.Existing == "" {
		t.Fatalf("second add: %v %+v", err, again)
	}

	all := chefAll()
	names := []string{}
	for _, r := range all {
		names = append(names, r.Name)
	}
	if len(all) != 2 {
		t.Fatalf("recipes %q", names)
	}
	var burger *ChefRecipe
	for _, r := range all {
		if strings.HasPrefix(r.Name, "Homemade") {
			burger = r
		}
	}
	if burger == nil || burger.Image == "" || len(burger.Ingredients) != 9 || burger.Servings != 4 {
		t.Fatalf("%+v", burger)
	}

	sec := GetDb().FindByHash(burger.Hash)
	if sec == nil {
		t.Fatal("hash does not find it")
	}
	shop, err := chefShop(&ChefShopRequest{Hashes: []string{burger.Hash}, Servings: 8})
	if err != nil || !shop.Ok {
		t.Fatalf("%v %+v", err, shop)
	}
	got = readOrg(t, p)
	if !strings.Contains(got, "* Shopping\n\n** Shopping: Homemade Black Bean Veggie Burgers [0/9]\n- [ ] 2 (16 ounce) cans black beans, drained and rinsed\n- [ ] 1 green bell pepper") {
		t.Fatalf("shopping list:\n%s", got)
	}
}

func TestChefOffDoesNothing(t *testing.T) {
	withFeatures(t, func(f *common.FeatureSettings) { f.Chef = false })
	w := httptest.NewRecorder()
	RequestChefRecipes(w, httptest.NewRequest("GET", "/chef/recipes", nil))
	if !strings.Contains(w.Body.String(), `"Off":true`) {
		t.Fatalf("%s", w.Body.String())
	}
}
