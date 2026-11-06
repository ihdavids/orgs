package orgpkgs

// Keeping web pictures here (features.localImages, org-download), from
// the command line.
//
//	orgs images scan [files]            the web pictures each file links (every file when none named)
//	orgs images localize [files]        download them beside the notes and point the links at the copies
//	orgs images localize trip.org -dry-run   what would be downloaded, and where
//	orgs images orphans                 pictures in the pictures folders nothing links any more
//	orgs images orphans -delete         ...and delete them (asks; -yes to not)
//
// Needs `orgs features localImages on`.

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ihdavids/orgs/cmd/oc/commands"
)

type Images struct {
	base
	Delete bool
	Yes    bool
	Probe  bool
}

func (self *Images) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	fset.BoolVar(&self.Delete, "delete", false, "orphans: delete them (asks first)")
	fset.BoolVar(&self.Yes, "yes", false, "orphans -delete: do not ask")
	fset.BoolVar(&self.Probe, "probe", false, "scan with no file named: ask servers about urls with no extension too")
}

type imgRemote struct {
	Url   string
	Line  int
	Desc  string
	Bare  bool
	Alone bool
	Sure  bool
	Type  string
	Size  int64
	Skip  string
}

type imgFetched struct {
	Url, Link, Src string
	Line           int
	Size           int64
	Reused         bool
}

type imgFailed struct {
	Url  string
	Line int
	Why  string
}

type imgFile struct {
	File    string
	Ok      bool
	Msg     string
	Dir     string
	Images  []imgRemote
	Fetched []imgFetched
	Failed  []imgFailed
	Bytes   int64
	Wrote   bool
}

type imgScan struct {
	Ok    bool
	Off   bool
	Msg   string
	Count int
	Bytes int64
	Files []imgFile
}

type imgFetch struct {
	Ok      bool
	Off     bool
	Msg     string
	Fetched int
	Failed  int
	Bytes   int64
	Wrote   int
	Files   []imgFile
}

type imgOrphan struct {
	Path, Rel, Src string
	Size           int64
}

type imgOrphans struct {
	Ok      bool
	Off     bool
	Msg     string
	Dirs    []string
	Orphans []imgOrphan
	Bytes   int64
	Deleted []string
	Failed  map[string]string
}

func imgSize(n int64) string {
	switch {
	case n <= 0:
		return ""
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/float64(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%d KB", n>>10)
	}
	return fmt.Sprintf("%d B", n)
}

func (self *Images) Exec(core *commands.Core) {
	words := commands.FreeArgs(self.fset)
	if len(words) == 0 {
		commands.Fail("orgs images scan|localize|orphans [files]")
	}
	files := []string{}
	for _, w := range words[1:] {
		files = append(files, commands.ResolveOrgFile(core, w))
	}
	switch words[0] {
	case "scan", "ls":
		self.scan(core, files, false)
	case "localize", "localise", "fetch", "get":
		if commands.DryRun {
			self.scan(core, files, true)
			return
		}
		self.localize(core, files)
	case "orphans":
		self.orphans(core)
	default:
		commands.Fail("orgs images scan|localize|orphans [files]")
	}
}

func (self *Images) scanFiles(core *commands.Core, files []string) []imgFile {
	if len(files) == 0 {
		ps := map[string]string{}
		if self.Probe {
			ps["probe"] = "1"
		}
		res, err := commands.SendReceiveGetErr[imgScan](core, "localimages/scan", ps)
		if err != nil {
			commands.Fail("%v", err)
		}
		if !res.Ok {
			commands.Fail("%s", res.Msg)
		}
		return res.Files
	}
	out := []imgFile{}
	for _, f := range files {
		res, err := commands.SendReceiveGetErr[imgScan](core, "localimages/scan", map[string]string{"file": f})
		if err != nil {
			commands.Fail("%v", err)
		}
		if res.Off {
			commands.Fail("%s", res.Msg)
		}
		if !res.Ok {
			fmt.Fprintf(os.Stderr, "%s: %s\n", f, res.Msg)
			continue
		}
		out = append(out, res.Files...)
	}
	return out
}

// scan lists what localize would download: per file, each url with its
// line, size when the server said, and why one will be left alone.
func (self *Images) scan(core *commands.Core, files []string, dry bool) {
	got := self.scanFiles(core, files)
	commands.Render(got, func() {
		n := 0
		var total int64
		for _, f := range got {
			if len(f.Images) == 0 {
				if len(files) > 0 {
					fmt.Printf("%s: %s\n", filepath.Base(f.File), dim("no web pictures"))
				}
				continue
			}
			fmt.Printf("%s %s\n", filepath.Base(f.File), dim("→ "+f.Dir+"/"))
			for _, im := range f.Images {
				note := imgSize(im.Size)
				if !im.Sure && im.Type != "" {
					note = im.Type + " " + note
				}
				if im.Skip != "" {
					fmt.Printf("  %s %s  %s\n", dim(fmt.Sprintf("%4d", im.Line)), im.Url, commands.C(commands.AnsiGold)+"skip: "+im.Skip+commands.C(commands.AnsiReset))
					continue
				}
				n++
				total += im.Size
				fmt.Printf("  %s %s  %s\n", dim(fmt.Sprintf("%4d", im.Line)), im.Url, dim(note))
			}
		}
		if n == 0 {
			fmt.Fprintln(os.Stderr, "no web pictures to download")
			return
		}
		verb := "to download"
		if dry {
			verb = "would be downloaded (dry run: nothing written)"
		}
		sz := ""
		if total > 0 {
			sz = ", " + imgSize(total) + " known"
		}
		fmt.Fprintf(os.Stderr, "%d picture(s) %s%s\n", n, verb, sz)
	})
}

// localize lists, then downloads file by file, saying how each went on
// stderr as it goes.
func (self *Images) localize(core *commands.Core, files []string) {
	targets := self.scanFiles(core, files)
	type req struct{ File string }
	all := imgFetch{Ok: true, Files: []imgFile{}}
	quiet := commands.Machine()
	for _, t := range targets {
		want := 0
		for _, im := range t.Images {
			if im.Skip == "" {
				want++
			}
		}
		if want == 0 {
			continue
		}
		name := filepath.Base(t.File)
		if !quiet {
			fmt.Fprintf(os.Stderr, "%s: downloading %d picture(s)…\n", name, want)
		}
		res, err := commands.SendReceivePostErr[req, imgFetch](core, "localimages/fetch", &req{t.File})
		if err == commands.ErrDryRun {
			continue
		}
		if err != nil {
			commands.Fail("%v", err)
		}
		if res.Off {
			commands.Fail("%s", res.Msg)
		}
		for _, f := range res.Files {
			if !quiet {
				for _, x := range f.Fetched {
					what := "kept"
					if x.Reused {
						what = "same as"
					}
					fmt.Fprintf(os.Stderr, "  %s %s %s %s\n", commands.C(commands.AnsiGreen)+"✓"+commands.C(commands.AnsiReset), x.Url, dim(what), x.Link)
				}
				for _, x := range f.Failed {
					fmt.Fprintf(os.Stderr, "  %s %s %s\n", commands.C(commands.AnsiRed)+"✗"+commands.C(commands.AnsiReset), x.Url, dim(x.Why))
				}
				if !f.Ok && f.Msg != "" {
					fmt.Fprintf(os.Stderr, "  %s\n", f.Msg)
				}
			}
			all.Files = append(all.Files, f)
		}
		all.Fetched += res.Fetched
		all.Failed += res.Failed
		all.Bytes += res.Bytes
		all.Wrote += res.Wrote
	}
	commands.RenderOne(all, func() {
		if all.Fetched == 0 && all.Failed == 0 {
			fmt.Println("no web pictures to download")
			return
		}
		fmt.Printf("%d picture(s) kept here (%s downloaded) in %d file(s)", all.Fetched, imgSize(all.Bytes), all.Wrote)
		if all.Failed > 0 {
			fmt.Printf(", %d failed", all.Failed)
		}
		fmt.Println()
	})
	if all.Failed > 0 && all.Fetched == 0 {
		os.Exit(1)
	}
}

func (self *Images) orphans(core *commands.Core) {
	type req struct {
		Delete  []string
		Confirm bool
	}
	// The list is a read; the POST carries no change without Delete, so it
	// is asked even with -dry-run.
	was := commands.DryRun
	commands.DryRun = false
	res, err := commands.SendReceivePostErr[req, imgOrphans](core, "localimages/orphans", &req{})
	commands.DryRun = was
	if err != nil {
		commands.Fail("%v", err)
	}
	if !res.Ok {
		commands.Fail("%s", res.Msg)
	}
	if !self.Delete || len(res.Orphans) == 0 {
		commands.Render(res.Orphans, func() {
			for _, o := range res.Orphans {
				fmt.Printf("%s  %s\n", o.Rel, dim(imgSize(o.Size)))
			}
			if len(res.Orphans) == 0 {
				fmt.Fprintln(os.Stderr, "every picture is linked from somewhere")
			} else {
				fmt.Fprintf(os.Stderr, "%d picture(s), %s, that nothing links (orgs images orphans -delete)\n", len(res.Orphans), imgSize(res.Bytes))
			}
		})
		return
	}
	if !commands.Machine() {
		for _, o := range res.Orphans {
			fmt.Fprintf(os.Stderr, "  %s  %s\n", o.Rel, dim(imgSize(o.Size)))
		}
	}
	if !commands.Confirm(&commands.TargetFlags{Yes: self.Yes}, "delete these %d picture(s), %s?", len(res.Orphans), imgSize(res.Bytes)) {
		fmt.Fprintln(os.Stderr, "nothing deleted")
		return
	}
	paths := []string{}
	for _, o := range res.Orphans {
		paths = append(paths, o.Path)
	}
	del, err := commands.SendReceivePostErr[req, imgOrphans](core, "localimages/orphans", &req{Delete: paths, Confirm: true})
	if err == commands.ErrDryRun {
		return
	}
	if err != nil {
		commands.Fail("%v", err)
	}
	commands.RenderOne(del, func() {
		fmt.Printf("deleted %d picture(s)\n", len(del.Deleted))
		for p, why := range del.Failed {
			fmt.Fprintf(os.Stderr, "  %s: %s\n", p, why)
		}
	})
	if !del.Ok {
		if del.Msg != "" {
			fmt.Fprintln(os.Stderr, del.Msg)
		}
		os.Exit(1)
	}
}

func init() {
	commands.AddCmd("images", "keep pictures linked from the web beside the notes; orphaned pictures (org-download)",
		func() commands.Cmd { return &Images{} })
}
