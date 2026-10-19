package orgs

import (
	"bytes"
	"encoding/json"
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

func campaignPost(t *testing.T, body map[string]interface{}) (map[string]interface{}, int) {
	t.Helper()
	b, _ := json.Marshal(body)
	r := httptest.NewRequest("POST", "/dnd/campaign", bytes.NewReader(b))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	PostDndCampaign(w, r)
	var out map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &out)
	return out, w.Code
}

// A map: an org file with a .dungeon.json beside it, and a picture.
func writeMap(t *testing.T, dir, stem, name string) string {
	t.Helper()
	os.MkdirAll(filepath.Join(dir, "maps"), 0755)
	org := filepath.Join(dir, stem+".org")
	os.WriteFile(org, []byte("#+TITLE: "+name+"\n* Map\n"), 0644)
	os.WriteFile(filepath.Join(dir, stem+".dungeon.json"), []byte(`{"name":"`+name+`","backdrop":{"src":"maps/`+stem+`-map.png"}}`), 0644)
	img := image.NewRGBA(image.Rect(0, 0, 1200, 800))
	for y := 0; y < 800; y++ {
		for x := 0; x < 1200; x++ {
			img.Set(x, y, color.RGBA{uint8(x % 256), uint8(y % 256), 90, 255})
		}
	}
	f, _ := os.Create(filepath.Join(dir, "maps", stem+"-map.png"))
	png.Encode(f, img)
	f.Close()
	return org
}

func TestCampaignFromMakingToAreasOfMaps(t *testing.T) {
	dir := t.TempDir()
	config.Server = &common.ServerSettings{OrgDirs: []string{dir}}
	odb = nil
	t.Cleanup(func() { odb = nil })
	grotto := writeMap(t, filepath.Join(dir, "dungeons"), "grotto", "The Drowned Grotto")
	land := writeMap(t, filepath.Join(dir, "maps"), "a-new-land", "A New Land")

	out, code := campaignPost(t, map[string]interface{}{"op": "new", "name": "The Shattered Coast", "icon": "🐉", "description": "Pirates and worse."})
	if code != 200 || out["Ok"] != true {
		t.Fatalf("new: %d %v", code, out)
	}
	file := out["File"].(string)
	if file != filepath.Join(dir, "campaigns", "the-shattered-coast.org") {
		t.Fatalf("file %s", file)
	}
	if _, code := campaignPost(t, map[string]interface{}{"op": "new", "name": "The Shattered Coast"}); code != http.StatusConflict {
		t.Errorf("a second of the same name: %d", code)
	}
	for _, a := range []string{"The North", "The Sunken City"} {
		if out, _ := campaignPost(t, map[string]interface{}{"op": "area", "file": file, "name": a}); out["Ok"] != true {
			t.Fatalf("area %s: %v", a, out)
		}
	}
	// Some notes of the GM's own in the area, kept through edits.
	lines := fileLinesFresh(file)
	at, own, _, _ := findArea(lines, "The North", 0)
	lines = append(lines[:own:own], append([]string{"Cold, and full of wolves."}, lines[own:]...)...)
	_ = at
	writeLines(file, lines)

	if out, _ := campaignPost(t, map[string]interface{}{"op": "maps", "file": file, "area": "The North", "maps": []string{grotto, land, grotto}}); out["Ok"] != true {
		t.Fatalf("maps: %v", out)
	}
	c, ok := readCampaign(file, fileLinesFresh(file))
	if !ok || c.Title != "The Shattered Coast" || c.Icon != "🐉" || c.Description != "Pirates and worse." {
		t.Fatalf("campaign %+v", c)
	}
	if len(c.Areas) != 2 || len(c.Areas[0].Maps) != 2 || c.Areas[0].Maps[0].File != grotto || c.Areas[0].Maps[1].Title != "A New Land" || c.Areas[0].Maps[0].Missing {
		t.Fatalf("areas %+v", c.Areas)
	}
	text, _ := os.ReadFile(file)
	if !strings.Contains(string(text), "- [[file:../dungeons/grotto.org][The Drowned Grotto]]") || !strings.Contains(string(text), "Cold, and full of wolves.") {
		t.Fatalf("file:\n%s", text)
	}
	// Setting the list again replaces it, the notes kept.
	campaignPost(t, map[string]interface{}{"op": "maps", "file": file, "area": "The North", "maps": []string{land}})
	c, _ = readCampaign(file, fileLinesFresh(file))
	if len(c.Areas[0].Maps) != 1 || c.Areas[0].Maps[0].File != land {
		t.Fatalf("replaced: %+v", c.Areas[0])
	}
	if text, _ := os.ReadFile(file); strings.Count(string(text), "grotto.org") != 0 || !strings.Contains(string(text), "wolves") {
		t.Fatalf("file:\n%s", text)
	}
	// Order, rename, delete.
	campaignPost(t, map[string]interface{}{"op": "order", "file": file, "names": []string{"The Sunken City", "The North"}})
	c, _ = readCampaign(file, fileLinesFresh(file))
	if c.Areas[0].Name != "The Sunken City" || c.Areas[1].Name != "The North" || len(c.Areas[1].Maps) != 1 {
		t.Fatalf("order: %+v", c.Areas)
	}
	campaignPost(t, map[string]interface{}{"op": "rename", "file": file, "area": "The North", "line": c.Areas[1].Line, "name": "The Frozen North"})
	campaignPost(t, map[string]interface{}{"op": "delete", "file": file, "area": "The Sunken City"})
	c, _ = readCampaign(file, fileLinesFresh(file))
	if len(c.Areas) != 1 || c.Areas[0].Name != "The Frozen North" || len(c.Areas[0].Maps) != 1 {
		t.Fatalf("rename/delete: %+v", c.Areas)
	}
	// Edit: a new title and description, the icon cleared.
	campaignPost(t, map[string]interface{}{"op": "edit", "file": file, "title": "The Broken Coast", "description": "Line one\nLine two", "icon": ""})
	c, _ = readCampaign(file, fileLinesFresh(file))
	if c.Title != "The Broken Coast" || c.Description != "Line one\nLine two" || c.Icon != "" || len(c.Areas) != 1 {
		t.Fatalf("edit: %+v", c)
	}
	// Not a campaign: refused.
	if _, code := campaignPost(t, map[string]interface{}{"op": "area", "file": grotto, "name": "x"}); code != http.StatusBadRequest {
		t.Errorf("a map is not a campaign: %d", code)
	}

	// Listing.
	w := httptest.NewRecorder()
	RequestDndCampaigns(w, httptest.NewRequest("GET", "/dnd/campaigns", nil))
	var list struct{ Campaigns []campaign }
	json.Unmarshal(w.Body.Bytes(), &list)
	if len(list.Campaigns) != 1 || list.Campaigns[0].Title != "The Broken Coast" {
		t.Fatalf("list %s", w.Body.String())
	}
	w = httptest.NewRecorder()
	RequestDndMaps(w, httptest.NewRequest("GET", "/dnd/maps", nil))
	var maps struct{ Maps []mapEntry }
	json.Unmarshal(w.Body.Bytes(), &maps)
	if len(maps.Maps) != 2 {
		t.Fatalf("maps %s", w.Body.String())
	}

	// A thumbnail: the picture, shrunk to a JPEG.
	w = httptest.NewRecorder()
	RequestDndMapThumb(w, httptest.NewRequest("GET", "/dnd/map/thumb?file="+grotto+"&w=300", nil))
	if w.Code != 200 || w.Header().Get("Content-Type") != "image/jpeg" {
		t.Fatalf("thumb %d %s", w.Code, w.Header().Get("Content-Type"))
	}
	img, _, err := image.Decode(bytes.NewReader(w.Body.Bytes()))
	if err != nil || img.Bounds().Dx() != 300 || img.Bounds().Dy() != 200 {
		t.Fatalf("thumb %v %v", err, img)
	}
}

func TestACampaignByItsTags(t *testing.T) {
	lines := []string{"#+TITLE: Old One", "#+FILETAGS: :rpg:campaign:", "", "* Notes", "* Wilds  :outdoors:area:", "- [[file:wild.org][The Wild]] and [[file:../x.org]]"}
	c, ok := readCampaign("/n/c/old.org", lines)
	if !ok || len(c.Areas) != 1 || c.Areas[0].Name != "Wilds" || len(c.Areas[0].Maps) != 2 || c.Areas[0].Maps[1].File != "/n/x.org" {
		t.Fatalf("%+v", c)
	}
	if _, ok := readCampaign("/n/c/x.org", []string{"#+TITLE: x", "* a :area:"}); ok {
		t.Error("not a campaign")
	}
}
