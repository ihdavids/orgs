package orgpkgs

// orgs media: org-media-note from the command line.
//
//	orgs media                           every heading with a recording
//	orgs media notes [heading]           its notes, by time
//	orgs media add [heading] 12:30 'text'   a note at a moment (12:30-12:45 is an A-B loop)
//	orgs media play [heading] [12:30]    open it in mpv (else vlc, else the OS player) there
//	orgs media start lecture.mp4 -file notes.org [-title T] [-under heading]
//	orgs media transcribe [heading] [-write]   whisper's words as linked notes
//
// Needs features.mediaNotes on.

import (
	"flag"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

type Media struct {
	base
	tf     commands.TargetFlags
	File   string
	Title  string
	Under  string
	Write  bool
	Player string
	Frac   bool
}

func (self *Media) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	commands.AddTargetFlags(fset, &self.tf)
	fset.StringVar(&self.File, "file", "", "start: the org file the notes go in")
	fset.StringVar(&self.Title, "title", "", "start: the heading's title")
	fset.StringVar(&self.Under, "under", "", "start: put the heading under this one (a query or hash)")
	fset.BoolVar(&self.Write, "write", false, "transcribe: write the segments under the heading")
	fset.StringVar(&self.Player, "player", "", "play: mpv, vlc, iina or open (default: the first found)")
	fset.BoolVar(&self.Frac, "ms", false, "add: write the time with milliseconds (H:MM:SS.fff)")
}

type mediaRef struct {
	Kind, Link, Target, Key, Path, Url, Youtube, Vimeo, Cite string
	Exists, Online                                         bool
}

type mediaSession struct {
	Hash, Headline, Filename, From string
	Line, Notes                    int
	Media                          mediaRef
}

type mediaNote struct {
	Seconds, End                   float64
	Stamp, Label, Text, Hash, Headline string
	Line                           int
}

type mediaGroup struct {
	Media mediaRef
	Notes []mediaNote
}

func mediaWhere(m mediaRef) string {
	w := m.Target
	if m.Link != "" {
		w = m.Link + ":" + w
	}
	if !m.Exists {
		w += commands.C(commands.AnsiRed) + " (missing)" + commands.C(commands.AnsiReset)
	}
	return w
}

func (self *Media) sessions(core *commands.Core) []mediaSession {
	res, err := commands.SendReceiveGetErr[struct {
		offAnswer
		Sessions []mediaSession
	}](core, "media/sessions", nil)
	if err != nil {
		commands.Fail("%v", err)
	}
	failIfOff(res.offAnswer)
	return res.Sessions
}

// session is the heading the words name, or the one session there is, or a
// pick among them.
func (self *Media) session(core *commands.Core, words []string) string {
	if len(words) > 0 || self.tf.Hash != "" {
		return commands.Resolve(core, &self.tf, words, commands.TargetOpts{Prompt: "Recording> ", Quiet: true})[0].Hash
	}
	ss := self.sessions(core)
	switch {
	case len(ss) == 0:
		commands.Fail("no headings with a recording (orgs media start FILE -file notes.org)")
	case len(ss) == 1:
		return ss[0].Hash
	case !commands.Interactive():
		commands.Fail("%d headings have a recording: say which (a query or -hash)", len(ss))
	}
	lines := []string{}
	for _, s := range ss {
		lines = append(lines, commands.PickLine([]string{s.Hash}, s.Headline+"  "+s.Media.Target))
	}
	got := commands.Pick(commands.PickOpts{Lines: lines, AddressFields: 1, Prompt: "Recording> "})
	if len(got) == 0 {
		os.Exit(1)
	}
	a, _ := commands.Address(got[0], 1)
	return a[0]
}

// splitAt takes the first word that reads as a time (or an A-B loop) out of
// words: what is before it names the heading, what is after is the note.
func splitAt(words []string) (before []string, start, end float64, after []string, ok bool) {
	for i, w := range words {
		a, b := w, ""
		if j := strings.Index(w, "-"); j > 0 {
			a, b = w[:j], w[j+1:]
		}
		if !strings.Contains(a, ":") {
			continue
		}
		s, ok1 := common.ParseMediaStamp(a)
		if !ok1 {
			continue
		}
		e := -1.0
		if b != "" {
			v, ok2 := common.ParseMediaStamp(b)
			if !ok2 {
				continue
			}
			e = v
		}
		return words[:i], s, e, words[i+1:], true
	}
	return words, 0, -1, nil, false
}

func (self *Media) notes(core *commands.Core, hash string) (mediaSession, []mediaGroup) {
	res, err := commands.SendReceiveGetErr[struct {
		offAnswer
		Session mediaSession
		Groups  []mediaGroup
	}](core, "media/notes", map[string]string{"hash": hash})
	if err != nil {
		commands.Fail("%v", err)
	}
	failIfOff(res.offAnswer)
	return res.Session, res.Groups
}

func (self *Media) Exec(core *commands.Core) {
	words := commands.FreeArgs(self.fset)
	verb := ""
	if len(words) > 0 {
		verb, words = words[0], words[1:]
	}
	switch verb {
	case "", "ls", "list":
		ss := self.sessions(core)
		commands.Render(ss, func() {
			for _, s := range ss {
				fmt.Printf("%s:%d %s  %s  %s\n", dim(commands.BaseName(s.Filename)), s.Line+1, s.Headline, mediaWhere(s.Media),
					dim(fmt.Sprintf("%d note%s", s.Notes, plural(s.Notes))))
			}
			if len(ss) == 0 {
				fmt.Println("no headings with a recording (orgs media start FILE -file notes.org)")
			}
		})
	case "notes", "show":
		s, groups := self.notes(core, self.session(core, words))
		commands.Render(groups, func() {
			fmt.Println(s.Headline)
			for _, g := range groups {
				fmt.Printf("  %s\n", mediaWhere(g.Media))
				for _, n := range g.Notes {
					fmt.Printf("    %s%-17s%s %s\n", commands.C(commands.AnsiGreen), n.Stamp, commands.C(commands.AnsiReset), n.Text)
				}
			}
		})
	case "add":
		before, start, end, after, ok := splitAt(words)
		if !ok {
			commands.Fail("orgs media add [heading] 12:30 'the note'  (or 0:12:30, 12:30-12:45 for a loop)")
		}
		hash := self.session(core, before)
		req := struct {
			Hash     string
			Seconds  float64
			End      *float64
			Text     string
			Fraction bool
		}{Hash: hash, Seconds: start, Text: strings.Join(after, " "), Fraction: self.Frac}
		if end > start {
			req.End = &end
		}
		var reply struct {
			Ok        bool
			Msg, Link string
		}
		commands.SendReceivePost(core, "media/note", &req, &reply)
		if commands.DryRun {
			return
		}
		if !reply.Ok {
			commands.Fail("%s", reply.Msg)
		}
		commands.Ok(1, "noted", []string{reply.Link + " " + req.Text})
	case "play", "open":
		before, start, end, _, ok := splitAt(words)
		if !ok {
			start, end = -1, -1
		}
		s, _ := self.notes(core, self.session(core, before))
		self.play(core, s.Media, start, end)
	case "start":
		if len(words) != 1 || self.File == "" && self.Under == "" {
			commands.Fail("orgs media start FILE-OR-URL -file notes.org [-title T] [-under heading]")
		}
		media := words[0]
		if !common.IsOnlineMedia(media) && !strings.HasPrefix(media, "cite:") && !strings.HasPrefix(media, "@") {
			if abs, err := filepath.Abs(media); err == nil {
				media = abs
			}
		}
		req := struct{ File, Under, Media, Title string }{Media: media, Title: self.Title}
		if self.Under != "" {
			req.Under = commands.Resolve(core, &self.tf, []string{self.Under}, commands.TargetOpts{Prompt: "Under> ", Quiet: true})[0].Hash
		} else {
			req.File = commands.ResolveOrgFile(core, self.File)
		}
		var reply common.Result
		commands.SendReceivePost(core, "media/start", &req, &reply)
		if commands.DryRun {
			return
		}
		if !reply.Ok {
			commands.Fail("%s", reply.Msg)
		}
		commands.Ok(1, "started", []string{reply.Msg})
	case "transcribe":
		hash := self.session(core, words)
		var reply struct {
			Ok      bool
			Msg     string
			Lines   []string
			Written bool
		}
		commands.SendReceivePost(core, "media/transcribe", &struct {
			Hash  string
			Write bool
		}{hash, self.Write}, &reply)
		if commands.DryRun {
			return
		}
		if !reply.Ok {
			commands.Fail("%s", reply.Msg)
		}
		commands.RenderOne(reply, func() {
			for _, l := range reply.Lines {
				fmt.Println(l)
			}
			if reply.Written {
				fmt.Fprintln(os.Stderr, "written under the heading")
			} else {
				fmt.Fprintln(os.Stderr, "not written (-write to put them under the heading)")
			}
		})
	default:
		commands.Fail("orgs media [ls | notes | add 12:30 'note' | play [12:30] | start FILE -file ORG | transcribe [-write]]")
	}
}

// play opens the recording in a player at a moment: mpv takes a start and an
// A-B loop as org-media-note gives them; vlc a start; the OS opener neither.
func (self *Media) play(core *commands.Core, m mediaRef, start, end float64) {
	src := ""
	headers := []string{}
	switch {
	case m.Online:
		src = m.Url
	case m.Path != "":
		if _, err := os.Stat(m.Path); err == nil {
			src = m.Path
		}
	}
	if src == "" && m.Url != "" {
		// Not on this machine: stream it from the server.
		src = strings.TrimRight(core.Rest.Url, "/") + m.Url
		if a := core.Rest.Header.Get("Authorization"); a != "" {
			headers = append(headers, "Authorization: "+a)
		}
	}
	if src == "" {
		commands.Fail("nothing to play: %s", mediaWhere(m))
	}
	player := self.Player
	if player == "" {
		for _, p := range []string{"mpv", "vlc", "iina"} {
			if _, err := exec.LookPath(p); err == nil {
				player = p
				break
			}
		}
	}
	args := []string{}
	ts := func(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }
	switch player {
	case "mpv":
		if start >= 0 {
			args = append(args, "--start=+"+ts(start))
			if end > start {
				args = append(args, "--ab-loop-a="+ts(start), "--ab-loop-b="+ts(end))
			}
		}
		if len(headers) > 0 {
			args = append(args, "--http-header-fields="+strings.Join(headers, ","))
		}
	case "vlc":
		if start >= 0 {
			args = append(args, "--start-time="+ts(start))
			if end > start {
				args = append(args, "--stop-time="+ts(end))
			}
		}
	case "iina":
		if start >= 0 {
			args = append(args, "--mpv-start=+"+ts(start))
		}
	default:
		player = map[string]string{"darwin": "open", "windows": "explorer"}[runtime.GOOS]
		if player == "" {
			player = "xdg-open"
		}
		if m.Youtube != "" && start >= 0 {
			src = "https://www.youtube.com/watch?v=" + url.QueryEscape(m.Youtube) + "&t=" + strconv.Itoa(int(start)) + "s"
		} else if start >= 0 {
			fmt.Fprintf(os.Stderr, "no mpv or vlc: %s opens at the start, not at %s\n", player, common.MediaStamp(start, false))
		}
	}
	args = append(args, src)
	if commands.Machine() {
		commands.RenderOne(map[string]interface{}{"Ok": true, "Player": player, "Args": args, "DryRun": commands.DryRun}, nil)
	}
	if commands.DryRun {
		if !commands.Machine() {
			fmt.Fprintln(os.Stderr, "would run:", player, strings.Join(args, " "))
		}
		return
	}
	cmd := exec.Command(player, args...)
	if err := cmd.Start(); err != nil {
		commands.Fail("could not start %s: %v", player, err)
	}
	if !commands.Machine() {
		at := ""
		if start >= 0 {
			at = " at " + common.MediaStamp(start, false)
		}
		fmt.Fprintf(os.Stderr, "%s %s%s\n", player, filepath.Base(src), at)
	}
}

func init() {
	commands.AddCmd("media", "notes at moments of a video or recording (org-media-note)",
		func() commands.Cmd { return &Media{} })
}
