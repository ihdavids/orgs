package paste

// A picture on the clipboard, into an org file.
//
//	orgs paste 'MatchHeadline("Q3 report")'   under that heading's body
//	orgs paste hOpOB7vIg6oiYz5sMVS=            under the heading with that hash
//	orgs paste -pick                           choose the heading
//	orgs paste -file notes.org -line 42        after line 42 (a table's last row)
//	orgs paste -file notes.org                 at the end of the file
//	orgs paste                                 keep it, print [[file:…]] to stdout
//	orgs paste -template todo                  …relative to where that capture goes
//	orgs paste -in chart.png 'IsProject()'     a file instead of the clipboard
//	some-tool | orgs paste -in - -file a.org   or stdin
//
// It is worg's paste into a card, through the same `POST /image/paste`: the
// server keeps the bytes under `images/` beside the first org directory, names
// them by when they arrived, and splices one `[[file:…]]` line into the body at
// the body's own indent - a line edit, not a rewrite of the file. So a picture
// pasted here and one pasted in worg land in the same folder, linked the same
// way.
//
// With no target the picture is stashed (the capture editor's path): kept, and
// the link handed back to be typed into something. That is the form for an
// editor binding - `:r !orgs paste` in vim - where the editor, not the server,
// owns the line the link goes on.
//
// The clipboard is read with whatever the platform has: `pngpaste` or
// AppleScript on a mac, `wl-paste` or `xclip` elsewhere. Every one of them is
// asked for PNG, which macOS converts to from the TIFF that Preview and most
// apps copy; a clipboard with text or a file reference on it is refused rather
// than uploaded as a picture of nothing.

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

type Paste struct {
	fset     *flag.FlagSet
	tf       commands.TargetFlags
	In       string
	File     string
	Line     int
	Template string
	Pick     bool
}

// pasteResult mirrors ImagePasteResult in internal/app/orgs/imagepaste.go.
type pasteResult struct {
	Ok       bool   `json:"Ok"`
	Msg      string `json:"Msg"`
	Link     string `json:"link"`
	Url      string `json:"url"`
	Filename string `json:"filename"`
	Line     int    `json:"line"`
}

func (self *Paste) Unmarshal(unmarshal func(interface{}) error) error { return unmarshal(self) }
func (self *Paste) StartPlugin(manager *common.PluginManager)         {}
func (self *Paste) HelpGroup() string                                 { return "Changing a heading" }

func (self *Paste) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	fset.StringVar(&self.In, "in", "", "read the picture from this file (- for stdin) instead of the clipboard")
	fset.StringVar(&self.File, "file", "", "put the link in this file rather than under a heading")
	fset.IntVar(&self.Line, "line", 0, "with -file: after this line (1 based); 0 means the end of the file")
	fset.StringVar(&self.Template, "template", "", "no target: make the link relative to where this capture template files")
	fset.BoolVar(&self.Pick, "pick", false, "choose the heading from a picker")
	commands.AddTargetFlags(fset, &self.tf)
}

func (self *Paste) Exec(core *commands.Core) {
	words := commands.FreeArgs(self.fset)

	data, err := self.read()
	if err != nil {
		commands.Fail("%v", err)
	}
	kind := imageKind(data, self.In)
	if kind == "" {
		commands.Fail("that is not a picture (png, jpeg, gif, webp, bmp or svg)")
	}

	fields := map[string]string{"type": kind}
	where := ""
	switch {
	case self.File != "":
		abs := self.File
		if a, err := filepath.Abs(self.File); err == nil {
			abs = a
		}
		fields["filename"] = abs
		// The server's afterLine is the zero based index of the line to go
		// after, so line N (1 based) is index N-1. No line means the end: a
		// number past the last line is clamped to it there.
		after := 1 << 30
		if self.Line > 0 {
			after = self.Line - 1
		}
		fields["afterLine"] = fmt.Sprint(after)
		where = commands.BaseName(abs)
	// Hashes on stdin need the `-` word: an editor running `orgs paste` hands
	// it a stdin that is not a terminal, and reading that as a list of
	// headings would refuse the one call that wanted only the link.
	case len(words) > 0 || self.tf.Hash != "" || self.Pick:
		if self.In == "-" && contains(words, "-") {
			commands.Fail("stdin cannot be both the picture and the headings")
		}
		hs := commands.Resolve(core, &self.tf, words, commands.TargetOpts{Prompt: "Paste under> "})
		if len(hs) != 1 {
			commands.Fail("a picture goes under one heading; %d matched", len(hs))
		}
		fields["hash"] = hs[0].Hash
		where = commands.Describe(hs[0])
	default:
		fields["stash"] = "1"
		if self.Template != "" {
			fields["template"] = self.Template
		}
	}

	name := "paste" + extOf(kind)
	if commands.Wrote("POST /image/paste", map[string]interface{}{"fields": fields, "bytes": len(data), "type": kind}) {
		return
	}
	res, err := common.RestPostFile[pasteResult](&core.Rest, "image/paste", "image", name, data, fields)
	if err != nil {
		commands.Fail("could not paste: %v", err)
	}
	if !res.Ok {
		if res.Link != "" {
			// The picture is on disk but the file was not written: still worth
			// the link, so it can be put in by hand.
			fmt.Fprintf(os.Stderr, "%s\n", res.Msg)
			fmt.Printf("[[file:%s]]\n", res.Link)
			os.Exit(1)
		}
		commands.Fail("%s", res.Msg)
	}
	commands.RenderOne(res, func() {
		if fields["stash"] != "" {
			// Stdout is the answer: only the link, so an editor reading it
			// gets exactly the text to insert.
			fmt.Printf("[[file:%s]]\n", res.Link)
			return
		}
		fmt.Printf("%spasted%s [[file:%s]] into %s, line %d\n",
			commands.C(commands.AnsiGreen), commands.C(commands.AnsiReset), res.Link, where, res.Line)
	})
}

// read gets the bytes: a named file, stdin, or the clipboard.
func (self *Paste) read() ([]byte, error) {
	switch self.In {
	case "-":
		return io.ReadAll(os.Stdin)
	case "":
		return clipboardImage()
	}
	return os.ReadFile(self.In)
}

func clipboardImage() ([]byte, error) {
	var tries [][]string
	if runtime.GOOS == "darwin" {
		if _, err := exec.LookPath("pngpaste"); err == nil {
			tries = append(tries, []string{"pngpaste", "-"})
		}
		if b, err := osascriptPNG(); err == nil {
			return b, nil
		} else if len(tries) == 0 {
			return nil, err
		}
	} else {
		if os.Getenv("WAYLAND_DISPLAY") != "" {
			tries = append(tries, []string{"wl-paste", "--no-newline", "--type", "image/png"})
		}
		tries = append(tries, []string{"xclip", "-selection", "clipboard", "-t", "image/png", "-o"})
	}
	var last error
	for _, t := range tries {
		if _, err := exec.LookPath(t[0]); err != nil {
			last = fmt.Errorf("%s is not installed", t[0])
			continue
		}
		var errb bytes.Buffer
		cmd := exec.Command(t[0], t[1:]...)
		cmd.Stderr = &errb
		out, err := cmd.Output()
		if err == nil && len(out) > 0 && imageKind(out, "") != "" {
			return out, nil
		}
		last = fmt.Errorf("no picture on the clipboard")
	}
	if last == nil {
		last = fmt.Errorf("no way to read the clipboard here; use -in file")
	}
	return nil, last
}

// AppleScript can only write the clipboard's PNG to a file, not to stdout, so
// it goes through a temporary one. It fails with "Can't make some data into
// the expected type" when there is no picture, which is the refusal to give.
func osascriptPNG() ([]byte, error) {
	f, err := os.CreateTemp("", "orgs-paste-*.png")
	if err != nil {
		return nil, err
	}
	path := f.Name()
	f.Close()
	defer os.Remove(path)
	script := fmt.Sprintf(`set f to open for access POSIX file %q with write permission
try
	set eof f to 0
	write (the clipboard as «class PNGf») to f
	close access f
on error e
	close access f
	error e
end try`, path)
	if out, err := exec.Command("osascript", "-e", script).CombinedOutput(); err != nil {
		if strings.Contains(string(out), "expected type") || strings.Contains(string(out), "-1700") {
			return nil, fmt.Errorf("no picture on the clipboard")
		}
		return nil, fmt.Errorf("could not read the clipboard: %s", strings.TrimSpace(string(out)))
	}
	b, err := os.ReadFile(path)
	if err != nil || len(b) == 0 {
		return nil, fmt.Errorf("no picture on the clipboard")
	}
	return b, nil
}

// What the bytes are, by looking at them; svg by its name or its first tag,
// since a sniffer calls it text.
func imageKind(b []byte, name string) string {
	ct := http.DetectContentType(b)
	switch ct {
	case "image/png", "image/jpeg", "image/gif", "image/webp", "image/bmp":
		return ct
	}
	head := strings.ToLower(string(b[:min(len(b), 512)]))
	if strings.EqualFold(filepath.Ext(name), ".svg") || strings.Contains(head, "<svg") {
		return "image/svg+xml"
	}
	if strings.EqualFold(filepath.Ext(name), ".avif") {
		return "image/avif"
	}
	return ""
}

func contains(words []string, w string) bool {
	for _, x := range words {
		if x == w {
			return true
		}
	}
	return false
}

func extOf(kind string) string {
	switch kind {
	case "image/jpeg":
		return ".jpg"
	case "image/svg+xml":
		return ".svg"
	}
	return "." + strings.TrimPrefix(kind, "image/")
}

func init() {
	commands.AddCmd("paste", "put the picture on the clipboard (or -in file) under a heading, or print its link",
		func() commands.Cmd { return &Paste{} })
}
