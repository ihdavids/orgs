package orgpkgs

// The map view (features.mapView, org-osm) from the
// command line.
//
//	orgs map [query]                         headings with a place, with coordinates
//	orgs map -format geojson > places.json   a GeoJSON FeatureCollection
//	orgs map geocode 'Granville Island'      look an address up (best five)
//	orgs map geocode <heading> [-write]      look a heading's address up (-write: as :GEO:)
//	orgs map geocode -all                    look up every address not looked up yet
//	orgs map set <heading> '49.28,-123.12'   write :LOCATION: (an address too; -geo writes :GEO:)
//	orgs map set <heading> ''                take it off
//	orgs map open <heading>                  show it in the OS's map (or -in osm|google|apple)
//	orgs map status                          tiles, the geocoder and its cache
//
// Needs `orgs features mapView on`.

import (
	"flag"
	"fmt"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
	"github.com/ihdavids/orgs/internal/common/geo"
)

type MapView struct {
	base
	tf     commands.TargetFlags
	File   string
	Colour string
	Write  bool
	Geo    bool
	All    bool
	In     string
	Limit  int
	Placed bool
}

func (self *MapView) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	fset.StringVar(&self.File, "file", "", "only this file's headings")
	fset.StringVar(&self.Colour, "colour", "", "keyword (default) or tag: what the geojson's marker-color follows")
	fset.BoolVar(&self.Write, "write", false, "geocode <heading>: write the coordinates found as :GEO:")
	fset.BoolVar(&self.Geo, "geo", false, "set: with an address, look it up and write :GEO: too")
	fset.BoolVar(&self.All, "all", false, "geocode: every address no lookup has answered yet")
	fset.StringVar(&self.In, "in", "", "open: osm, google, apple or geo (default: Apple Maps on a mac, else openstreetmap.org)")
	fset.IntVar(&self.Limit, "n", 5, "geocode an address: how many places to show")
	fset.BoolVar(&self.Placed, "placed", false, "only places with coordinates")
	commands.AddTargetFlags(fset, &self.tf)
}

type mapPlacesAnswer = geo.PlacesAnswer

type mapGeocodeAnswer struct {
	Ok      bool
	Msg     string
	Pending int
	Place   *geo.Place
	Results []struct {
		Lat, Lon float64
		Name     string
		Kind     string
	}
}

type mapSetAnswer struct {
	Ok    bool
	Msg   string
	Hash  string
	Place *geo.Place
}

func (self *MapView) places(core *commands.Core, query string) mapPlacesAnswer {
	params := map[string]string{}
	if query != "" {
		params["query"] = query
	}
	if self.File != "" {
		params["file"] = self.File
	}
	if self.Colour != "" {
		params["colour"] = self.Colour
	}
	res, err := commands.SendReceiveGetErr[mapPlacesAnswer](core, "map/places", params)
	if err != nil {
		commands.Fail("%v", err)
	}
	failIfOff(offAnswer{Ok: res.Ok, Off: res.Off, Msg: res.Msg})
	return res
}

func mapCoords(p geo.Place) string {
	if !p.Has {
		if p.NotFound {
			return "not found"
		}
		return "not looked up"
	}
	return geo.Format(geo.LatLon{Lat: p.Lat, Lon: p.Lon})
}

func mapWhere(p geo.Place) string {
	return commands.BaseName(p.Filename) + ":" + strconv.Itoa(p.Line+1)
}

// mapQuery turns plain words into a title match, as orgs mentions does.
func mapQuery(words []string) string {
	q := strings.TrimSpace(strings.Join(words, " "))
	if q == "" || strings.ContainsAny(q, "()&|!=<>") {
		return q
	}
	return `MatchHeadline("(?i)` + strings.ReplaceAll(q, `"`, `\"`) + `")`
}

func (self *MapView) Exec(core *commands.Core) {
	words := commands.FreeArgs(self.fset)
	verb := ""
	if len(words) > 0 {
		switch words[0] {
		case "ls", "geocode", "set", "open", "status":
			verb, words = words[0], words[1:]
		}
	}
	switch verb {
	case "status":
		res, err := commands.SendReceiveGetErr[map[string]interface{}](core, "map/status", nil)
		if err != nil {
			commands.Fail("%v", err)
		}
		ok, _ := res["Ok"].(bool)
		msg, _ := res["Msg"].(string)
		failIfOff(offAnswer{Ok: ok, Msg: msg})
		commands.RenderOne(res, func() {
			fmt.Printf("tiles      %v\n", res["TileUrl"])
			if a, _ := res["Attribution"].(string); a != "" {
				fmt.Printf("           %s\n", dim(a))
			}
			fmt.Printf("geocoder   %v%s\n", res["GeocodeUrl"], map[bool]string{true: "  (looks new addresses up by itself)", false: ""}[res["AutoGeocode"] == true])
			fmt.Printf("cache      %v found, %v not found  %s\n", res["Cached"], res["Missing"], dim(fmt.Sprint(res["CacheFile"])))
			if n, _ := res["Pending"].(float64); n > 0 {
				fmt.Printf("waiting    %v to look up, one a second\n", n)
			}
			if e, _ := res["LastError"].(string); e != "" {
				fmt.Println("last error " + commands.C(commands.AnsiRed) + e + commands.C(commands.AnsiReset))
			}
			fmt.Printf("written as :%v:\n", res["GeoProperty"])
		})
		return
	case "geocode":
		self.geocode(core, words)
		return
	case "set":
		self.set(core, words)
		return
	case "open":
		self.open(core, words)
		return
	}

	res := self.places(core, mapQuery(words))
	ps := res.Places
	if self.Placed {
		kept := []geo.Place{}
		for _, p := range ps {
			if p.Has {
				kept = append(kept, p)
			}
		}
		ps = kept
	}
	if strings.EqualFold(commands.FormatOut, "geojson") {
		b, err := geo.MarshalGeoJSON(ps)
		if err != nil {
			commands.Fail("%v", err)
		}
		os.Stdout.Write(append(b, '\n'))
		return
	}
	commands.Render(ps, func() {
		if len(ps) == 0 {
			fmt.Println("no headings with a place (:LOCATION:, :LATITUDE:/:LONGITUDE: or :ADDRESS:)")
			return
		}
		wTitle := 10
		for _, p := range ps {
			w := min(48, len([]rune(mapTitle(p))))
			if p.Keyword != "" {
				w += len([]rune(p.Keyword)) + 1
			}
			wTitle = max(wTitle, w)
		}
		for i, p := range ps {
			kw := ""
			if p.Keyword != "" {
				col := commands.AnsiGold
				if p.Done {
					col = commands.AnsiDim
				}
				kw = commands.C(col) + p.Keyword + commands.C(commands.AnsiReset) + " "
			}
			title := mapTitle(p)
			if r := []rune(title); len(r) > 48 {
				title = string(r[:47]) + "…"
			}
			pad := wTitle - len([]rune(title)) - len([]rune(p.Keyword))
			if p.Keyword != "" {
				pad--
			}
			coords := mapCoords(p)
			if !p.Has {
				coords = commands.C(commands.AnsiDim) + fmt.Sprintf("%-22s", coords) + commands.C(commands.AnsiReset)
			} else {
				coords = commands.C(commands.AnsiCyan) + fmt.Sprintf("%-22s", coords) + commands.C(commands.AnsiReset)
			}
			how := p.How
			if p.Address != "" {
				how = strings.TrimSpace(how + " " + p.Address)
			}
			fmt.Printf("%3d  %s%s%s  %s  %s %s\n", i+1, kw, title, strings.Repeat(" ", max(0, pad)), coords, dim(mapWhere(p)), dim(how))
		}
		if res.Unplaced > 0 {
			fmt.Println(dim(fmt.Sprintf("%d address%s not looked up yet: orgs map geocode -all", res.Unplaced, map[bool]string{true: "", false: "es"}[res.Unplaced == 1])))
		}
	})
}

func mapTitle(p geo.Place) string {
	if p.Label != "" {
		return p.Title + " (" + p.Label + ")"
	}
	return p.Title
}

// mapHeading is the one heading words name (a hash, a query, plain title words, or a picker).
func mapHeading(core *commands.Core, tf *commands.TargetFlags, words []string, prompt string) common.Todo {
	q := words
	if len(words) > 0 && !commands.LooksLikeHash(words[0]) && words[0] != "-" {
		q = []string{mapQuery(words)}
	}
	found := commands.Resolve(core, tf, q, commands.TargetOpts{Prompt: prompt, Quiet: true})
	if len(found) != 1 {
		commands.Fail("name one heading (found %d)", len(found))
	}
	return found[0]
}

func (self *MapView) geocode(core *commands.Core, words []string) {
	if self.All {
		req := struct{ All bool }{true}
		res, err := commands.SendReceivePostErr[struct{ All bool }, mapGeocodeAnswer](core, "map/geocode", &req)
		if err == commands.ErrDryRun {
			return
		}
		if err != nil {
			commands.Fail("%v", err)
		}
		failIfOff(offAnswer{Ok: res.Ok, Msg: res.Msg})
		commands.RenderOne(res, func() { fmt.Println(res.Msg) })
		return
	}
	// A heading: -hash, a hash, or a query; anything else is an address.
	heading := self.tf.Hash != "" || (len(words) == 1 && commands.LooksLikeHash(words[0])) ||
		(len(words) > 0 && strings.ContainsAny(strings.Join(words, " "), "()"))
	if !heading && len(words) == 0 {
		commands.Fail("what to look up: an address, a heading (a hash or a query), or -all")
	}
	if heading {
		h := mapHeading(core, &self.tf, words, "Look up> ")
		req := struct {
			Hash  string
			Write bool
		}{h.Hash, self.Write}
		send := commands.SendReceivePostErr[struct {
			Hash  string
			Write bool
		}, mapGeocodeAnswer]
		if !self.Write {
			// Looking up writes nothing to the org file: not stopped by -dry-run.
			res, err := common.RestPost[mapGeocodeAnswer](&core.Rest, "map/geocode", &req)
			if err != nil {
				commands.Fail("%v", err)
			}
			self.showGeocoded(res)
			return
		}
		res, err := send(core, "map/geocode", &req)
		if err == commands.ErrDryRun {
			return
		}
		if err != nil {
			commands.Fail("%v", err)
		}
		self.showGeocoded(res)
		return
	}
	req := struct {
		Address string
		Limit   int
	}{strings.Join(words, " "), self.Limit}
	res, err := common.RestPost[mapGeocodeAnswer](&core.Rest, "map/geocode", &req)
	if err != nil {
		commands.Fail("%v", err)
	}
	failIfOff(offAnswer{Ok: res.Ok, Msg: res.Msg})
	commands.Render(res.Results, func() {
		if len(res.Results) == 0 {
			fmt.Printf("nothing found for %q\n", req.Address)
		}
		for i, r := range res.Results {
			fmt.Printf("%3d  %s%-22s%s  %s %s\n", i+1, commands.C(commands.AnsiCyan), geo.Format(geo.LatLon{Lat: r.Lat, Lon: r.Lon}), commands.C(commands.AnsiReset), r.Name, dim(r.Kind))
		}
	})
}

func (self *MapView) showGeocoded(res mapGeocodeAnswer) {
	if !res.Ok && res.Place == nil {
		commands.Fail("%s", res.Msg)
	}
	commands.RenderOne(res, func() {
		if res.Place != nil && res.Place.Has {
			fmt.Printf("%s  %s%s%s\n", res.Place.Title, commands.C(commands.AnsiCyan), mapCoords(*res.Place), commands.C(commands.AnsiReset))
		}
		fmt.Println(dim(res.Msg))
	})
	if !res.Ok {
		os.Exit(1)
	}
}

func (self *MapView) set(core *commands.Core, words []string) {
	if len(words) == 0 || (len(words) == 1 && self.tf.Hash == "") {
		commands.Fail("usage: orgs map set <heading> 'address or lat,lon'  (or '' to take it off)")
	}
	loc := words[len(words)-1]
	h := mapHeading(core, &self.tf, words[:len(words)-1], "Place> ")
	req := struct {
		Hash, Location string
		Geo            bool
	}{h.Hash, loc, self.Geo}
	res, err := commands.SendReceivePostErr[struct {
		Hash, Location string
		Geo            bool
	}, mapSetAnswer](core, "map/set", &req)
	if err == commands.ErrDryRun {
		return
	}
	if err != nil {
		commands.Fail("%v", err)
	}
	failIfOff(offAnswer{Ok: res.Ok, Msg: res.Msg})
	commands.RenderOne(res, func() {
		fmt.Printf("%s: %s\n", h.Headline, res.Msg)
		if res.Place != nil && !res.Place.Has && res.Place.Address != "" {
			fmt.Println(dim("not on the map until it is looked up: orgs map geocode -hash " + res.Hash))
		}
	})
}

// mapOpenUrl is the place in a maps application or site.
func mapOpenUrl(p geo.Place, in string) string {
	ll := geo.LatLon{Lat: p.Lat, Lon: p.Lon}
	if in == "" {
		in = "osm"
		if runtime.GOOS == "darwin" {
			in = "apple"
		}
	}
	if !p.Has {
		words := p.Address
		if words == "" {
			words = p.Value
		}
		switch in {
		case "google":
			return "https://www.google.com/maps/search/?api=1&query=" + url.QueryEscape(words)
		case "apple":
			return "https://maps.apple.com/?q=" + url.QueryEscape(words)
		}
		return geo.SearchUrl(words)
	}
	switch in {
	case "google":
		return "https://www.google.com/maps/search/?api=1&query=" + url.QueryEscape(geo.Format(ll))
	case "apple":
		return "https://maps.apple.com/?ll=" + geo.Format(ll) + "&q=" + url.QueryEscape(p.Title)
	case "geo":
		return "geo:" + geo.Format(ll)
	}
	return geo.OSMUrl(ll, 16)
}

func (self *MapView) open(core *commands.Core, words []string) {
	h := mapHeading(core, &self.tf, words, "Open on a map> ")
	res := self.places(core, "")
	for _, p := range res.Places {
		if p.Hash != h.Hash {
			continue
		}
		u := mapOpenUrl(p, self.In)
		if commands.Machine() {
			commands.RenderOne(map[string]string{"Url": u, "Title": p.Title}, nil)
			return
		}
		fmt.Println(u)
		commands.OpenInBrowser(u)
		return
	}
	commands.Fail("%s has no place (:LOCATION:, :LATITUDE:/:LONGITUDE: or :ADDRESS:)", h.Headline)
}

func init() {
	commands.AddCmd("map", "headings with a :LOCATION: on a map: list, -format geojson, geocode, set, open, status",
		func() commands.Cmd { return &MapView{} })
}
