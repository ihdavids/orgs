package ics

/* SDOC: Pollers

* ICS Calendar Importer

	TODO More documentation on this module

	#+BEGIN_SRC yaml
  - name: "ics"
	timezone: "America/Los_Angeles"
	filename: "path to ics file to import"
	output: "where to output org data"
	#+END_SRC

EDOC */

import (
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"encoding/json"
	"io/ioutil"

	"github.com/apognu/gocal"
	"github.com/ihdavids/orgs/internal/app/orgs/plugs"
)

type LastChanged struct {
	Size int64
	Time time.Time
}

type Ics struct {
	Name     string
	Filename string
	History  LastChanged
	Output   string
	Timezone string
	jname    string
}

func (self *Ics) Unmarshal(unmarshal func(interface{}) error) error {
	return unmarshal(self)
}

var br = regexp.MustCompile("<\\s*br\\s*[/]?\\s*>")
var href = regexp.MustCompile("<a href\\s*=\\s*\"(?P<link>[^>]+)\">(?P<title>([^<]|[<][^/]|[<][/][^a])+)</a>")
var strong = regexp.MustCompile("<strong>(?P<str>([^<]|[<][^/]|[<][/][^s])+)</strong>")
var hr = regexp.MustCompile("<\\s*hr\\s*/?\\s*>")
var li = regexp.MustCompile("<li>(?P<str>([^<]|[<][^/]|[<][/][^l])+)</li>")
var ul = regexp.MustCompile("<\\s*/?\\s*ul\\s*/?\\s*>")
var bold = regexp.MustCompile("<\\s*b\\s*>(?P<str>([^<]|[<][^/]|[<][/][^b])+)</b\\s*>")
var underline = regexp.MustCompile("<\\s*u\\s*>(?P<str>([^<]|[<][^/]|[<][/][^u])+)</u\\s*>")
var italics = regexp.MustCompile("<\\s*i\\s*>(?P<str>([^<]|[<][^/]|[<][/][^i])+)</i\\s*>")
var emptyLink = regexp.MustCompile("(?P<pre>[^[]\\s*)(?P<link>https://[^ \t\n]+)")
var span = regexp.MustCompile("<span>(?P<str>([^<]|[<][^/]|[<][/][^s])+)</span>")
var pre = regexp.MustCompile("<pre>(?P<str>([^<]|[<][^/]|[<][/][^p])+)</pre>")
var lostStars = regexp.MustCompile("\n\\s*\\*\\s*\n")

func FormatInlineHtml(input string) string {
	out := ""
	out = bold.ReplaceAllString(input, "*${str}*")
	out = underline.ReplaceAllString(out, "_${str}_")
	out = italics.ReplaceAllString(out, "/${str}/")
	out = strings.Replace(out, "&nbsp;", " ", -1)
	out = strings.Replace(out, "&gt;", ">", -1)
	out = strings.Replace(out, "&lt;", "<", -1)
	out = br.ReplaceAllString(out, "\n  ")
	return out
}

func FormatHtml(input string) string {
	out := ""
	out = strings.Replace(input, "</html-blob>", "", -1)
	out = strings.Replace(out, "<html-blob>", "", -1)
	out = href.ReplaceAllString(out, "[[${link}][${title}]]")
	out = strong.ReplaceAllString(out, "*${str}*")
	out = hr.ReplaceAllString(out, "\n  ------\n  ")
	out = li.ReplaceAllString(out, "- ${str}\n  ")
	out = ul.ReplaceAllString(out, "\n  ")
	out = emptyLink.ReplaceAllString(out, "${pre}[[${link}]]")
	out = span.ReplaceAllString(out, "${str}")
	out = pre.ReplaceAllString(out, "${str}\n  ")
	out = lostStars.ReplaceAllString(out, "")
	return FormatInlineHtml(out)
}

func FormatPlain(input string) string {
	out := strings.Replace(input, "\\n", "\n", -1)
	return FormatInlineHtml(out)
}

var htmlIdentifier = regexp.MustCompile("<html-blob>(?P<str>([^<]|[<][^/]|[<][/][^h]|[<][/][h][^t])+)</html-blob>")

func FormatIt(input string) string {
	output := ""
	cidx := 0
	for _, match := range htmlIdentifier.FindAllSubmatchIndex([]byte(input), -1) {
		if match[0] > cidx {
			output += FormatPlain(input[cidx:match[0]])
		}
		cidx = match[1]
		output += FormatHtml(input[match[0]:match[1]])
	}
	return output
}

func (self *Ics) Update(db plugs.ODb) {
	fmt.Printf("Ics Update...%v\n", time.Now())
	loc, err := time.LoadLocation(self.Timezone)
	if err != nil {
		log.Printf("Timezone not found: %s using PDT\n", self.Timezone)
		loc, _ = time.LoadLocation("America/Los_Angeles")
	}
	doit := false
	stat, err := os.Stat(self.Filename)
	doit = err != nil
	needDateUpdate := false
	if doit == false && (stat.Size() != self.History.Size || stat.ModTime() != self.History.Time) {
		doit = true
		needDateUpdate = true
	}
	if doit {
		var calName string = ""
		fname := filepath.Base(self.Filename)
		underscore := strings.Index(fname, "_")
		if underscore > 0 {
			calName = fname[:underscore]
		}
		f, _ := os.Open(self.Filename)
		defer f.Close()
		//start, end := time.Now(), time.Now().Add(30*24*time.Hour)

		c := gocal.NewParser(f)
		//c.Start, c.End = &start, &end
		c.Parse()

		f, err := os.OpenFile(self.Output, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
		if err != nil {
			log.Fatalf("Unable to create output file [%s]: %v", self.Output, err)
		}
		defer f.Close()

		for _, e := range c.Events {
			tm := e.Start.In(loc).Format("2006-01-02 Mon 15:04")
			tags := ":CAL:"
			if calName != "" {
				tags = tags + strings.Replace(calName, " ", "_", -1) + ":"
			}
			if e.Categories != nil {
				for _, c := range e.Categories {
					tags += (c + ":")
				}
			}
			fmt.Fprintf(f, "* TODO %-60s %s\n  <%s>\n", e.Summary, tags, tm) //, e.Organizer.Cn)
			fmt.Fprintf(f, "  :PROPERTIES:\n")
			if e.URL != "" {
				fmt.Fprintf(f, "    :URL: [[%s][Link]] \n", e.URL)
			}
			if e.Status != "" {
				fmt.Fprintf(f, "    :Status: %s\n", e.Status)
			}
			if e.Uid != "" {
				fmt.Fprintf(f, "    :Id: %s\n", e.Uid)
			}
			fmt.Fprintf(f, "  :END:\n")
			if e.Description != "" {
				fmt.Fprintf(f, "  %s\n", FormatIt(e.Description))
			}

		}
		if needDateUpdate {
			// Only do this here so we KNOW we actually updated
			self.History.Size = stat.Size()
			self.History.Time = stat.ModTime()
			file, _ := json.MarshalIndent(self.History, "", " ")
			_ = ioutil.WriteFile(self.jname, file, 0644)
			// TODO: Serialize this out and back in.
		}
	}
}

func (self *Ics) Startup(freq int, manager *plugs.PluginManager, opts *plugs.PluginOpts) {

	self.jname = filepath.Base(self.Filename)
	ext := path.Ext(self.jname)
	self.jname = self.jname[0:len(self.jname)-len(ext)] + ".json"

	if _, err := os.Stat(self.jname); err == nil {
		file, _ := ioutil.ReadFile(self.jname)
		_ = json.Unmarshal([]byte(file), &self.History)
	} else {
		self.History.Size = 0
	}
}

// init function is called at boot
func init() {
	plugs.AddPoller("ics", func() plugs.Poller {
		return &Ics{Timezone: "America/Los_Angeles"}
	})
}
