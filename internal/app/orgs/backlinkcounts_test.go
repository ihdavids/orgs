package orgs

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/ihdavids/orgs/internal/common"
)

const lcNotes = `#+TITLE: Notes
* Apollo
:PROPERTIES:
:ID: apollo-id
:END:
The moon programme.
* Gemini
See [[id:apollo-id][Apollo]] and [[*Apollo]] and again [[id:apollo-id]].
* Mercury
Before [[*Gemini]].
* Apollo self
Back to [[*Apollo self]].
`

const lcOther = `Preamble link to [[file:notes.org]].
* Elsewhere
Read [[id:apollo-id][the moon]] and [[file:notes.org::*Gemini]].
* Another
Also [[id:apollo-id]]. Apollo is mentioned here without a link.
`

func lcSetup(t *testing.T) map[string]string {
	t.Helper()
	paths := loadOrg(t, map[string]string{"notes.org": lcNotes, "other.org": lcOther})
	old := Conf().Server.Features.BacklinkCounts
	Conf().Server.Features.BacklinkCounts = true
	t.Cleanup(func() { Conf().Server.Features.BacklinkCounts = old })
	return paths
}

func lcHeadings(res common.LinkCounts) string {
	var out []string
	for _, h := range res.Headings {
		s := fmt.Sprintf("%d:%s=%d/%d out%d", h.Line, h.Title, h.Backlinks, h.Links, h.Out)
		if res.Mentions {
			s += fmt.Sprintf(" m%d", h.Mentions)
		}
		out = append(out, s)
	}
	return strings.Join(out, " | ")
}

func TestLinkCountsOfAFile(t *testing.T) {
	paths := lcSetup(t)
	res := QueryLinkCounts(paths["notes.org"], false)
	if !res.Ok {
		t.Fatal(res.Msg)
	}
	// Apollo: Gemini (three links, one place) + Elsewhere + Another = 3 places, 5 links.
	// Gemini: Mercury and Elsewhere. A self link does not count.
	want := "1:Apollo=3/5 out0 | 6:Gemini=2/2 out3 | 8:Mercury=0/0 out1 | 10:Apollo self=0/0 out0"
	if got := lcHeadings(res); got != want {
		t.Errorf("headings\n got  %s\n want %s", got, want)
	}
	if res.File.Backlinks != 1 || res.File.Links != 1 || res.File.Any != 4 {
		t.Errorf("file: %+v", res.File)
	}
	// The links written in the file: where they are, and how many others go there.
	var links []string
	for _, l := range res.Links {
		links = append(links, fmt.Sprintf("%d:%d-%d %s n%d p%d", l.Line, l.Col, l.End, l.Raw, l.Count, l.Places))
	}
	wantLinks := "7:4-28 id:apollo-id n4 p2 | 7:33-44 *Apollo n4 p2 | 7:55-71 id:apollo-id n4 p2 | " +
		"9:7-18 *Gemini n1 p1 | 11:8-24 *Apollo self n0 p0"
	if got := strings.Join(links, " | "); got != wantLinks {
		t.Errorf("links\n got  %s\n want %s", got, wantLinks)
	}
}

func TestLinkCountsMentionsAndTop(t *testing.T) {
	paths := lcSetup(t)
	res := QueryLinkCounts(paths["notes.org"], true)
	// "Apollo is mentioned" in Another, and the headline "Apollo self".
	if !res.Mentions || res.Headings[0].Mentions != 2 {
		t.Errorf("Apollo should have two unlinked mentions: %s", lcHeadings(res))
	}
	top := QueryLinkCountsTop(2, false)
	if top.Total != 2 || len(top.Headings) != 2 || top.Headings[0].Title != "Apollo" || top.Headings[0].Backlinks != 3 {
		t.Errorf("top: %+v", top)
	}
}

func TestLinkCountsOfABuffer(t *testing.T) {
	paths := lcSetup(t)
	// Gemini renamed, a line added above it and a new heading: the renamed one
	// is found again by its words, Apollo by its line, the new one has no hash.
	text := strings.Replace(lcNotes, "* Gemini\n", "Added.\n* TODO Gemini\n", 1) + "* Brand new\nTo [[id:apollo-id]].\n"
	res := QueryLinkCountsText(common.LinkCountsText{Filename: paths["notes.org"], Text: text})
	want := "1:Apollo=4/6 out0 | 7:Gemini=2/2 out3 | 9:Mercury=0/0 out1 | 11:Apollo self=0/0 out0 | 13:Brand new=0/0 out0"
	if got := lcHeadings(res); got != want {
		t.Errorf("buffer\n got  %s\n want %s", got, want)
	}
	if res.Headings[4].Hash != "" {
		t.Errorf("a new heading has no hash")
	}
	if n := len(res.Links); n != 6 || res.Links[5].Count != 5 {
		t.Errorf("buffer links: %+v", res.Links)
	}
}

func TestLinkCountsOffAndOverHttp(t *testing.T) {
	paths := lcSetup(t)
	_, raw := call(t, RequestLinkCounts, "GET", "/links/counts?filename="+url.QueryEscape(paths["notes.org"]), nil, nil)
	var res common.LinkCounts
	if err := json.Unmarshal(raw, &res); err != nil || !res.Ok || len(res.Headings) != 4 {
		t.Fatalf("%v %s", err, raw)
	}
	Conf().Server.Features.BacklinkCounts = false
	_, raw = call(t, RequestLinkCounts, "GET", "/links/counts?filename="+url.QueryEscape(paths["notes.org"]), nil, nil)
	if err := json.Unmarshal(raw, &res); err != nil || res.Ok || !res.Off {
		t.Fatalf("off: %s", raw)
	}
}

func TestLinkCountsInHtmlExports(t *testing.T) {
	paths := lcSetup(t)
	old := Conf().Server.BacklinkCounts
	t.Cleanup(func() { Conf().Server.BacklinkCounts = old })
	lines := strings.Split(strings.TrimSuffix(lcNotes, "\n"), "\n")
	if backlinkExportMayApply("html") {
		t.Fatal("export is off unless asked for")
	}
	Conf().Server.BacklinkCounts.Export = true
	if !backlinkExportMayApply("html") || backlinkExportMayApply("latex") {
		t.Fatal("html only")
	}
	out, changed := backlinkExportLines(paths["notes.org"], lines)
	if !changed || !strings.Contains(out[1], `<span class="org-backlinks" title="3 places link here">↩ 3</span>`) ||
		out[8] != "* Mercury" {
		t.Errorf("export: %q %q", out[1], out[8])
	}
}
