package links

// `orgs links` with the far end of the link drawn beside it.
//
// The list on its own answers "where do my links go". The pane answers the
// question you actually opened the list to ask, which is "is this the one I
// meant" - and for a link that is mostly about **where it was written**. A
// ticket pasted into a heading eighteen months ago is findable by grep and by
// nothing else; what tells you it is the right one is the paragraph around it,
// not the url.
//
// So the pane is: what the link says, where it goes, and the lines of the org
// file it sits in with its own line marked. For a link that stays inside the
// org files there is a second half - the heading it lands on - because that is
// the far end, and for a link naming a file on disk it is what kind of thing
// that file is and how big.
//
// The machinery (running the picker, finding this binary again, the boxes) is
// `cmd/oc/commands/picker.go`, shared with the code and tables pickers.

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

// How many lines of the org file to show around the link.
const contextLines = 6

// ---------------------------------------------------------------------------
// The picker
// ---------------------------------------------------------------------------

func (self *Links) pick(core *commands.Core) {
	// Nothing is reading a chooser: a pipe, a -json run or a cron job gets the
	// listing instead, which is the same question in a form it can use.
	if !commands.Interactive() {
		self.all(core)
		return
	}
	rows := self.rows(core)
	if len(rows) == 0 {
		fmt.Fprintln(os.Stderr, "no links matched")
		return
	}

	self2, err := commands.SelfCommand(core)
	if err != nil {
		fmt.Fprintf(os.Stderr, "orgs links: no preview (%v)\n", err)
	}

	lines := make([]string, 0, len(rows))
	for i, l := range rows {
		// The address is the position in *this* list rather than anything about
		// the link. A link has no id of its own - it is named by the file, the
		// line and what it said - and threading three fields through a shell
		// command to identify it is three chances to quote something wrong.
		// The pane re-runs the same query and indexes into the same answer.
		lines = append(lines, commands.PickLine([]string{strconv.Itoa(i)}, pickLine(l)))
	}

	opts := commands.PickOpts{
		Lines:         lines,
		AddressFields: 1,
		Prompt:        "link> ",
		Header:        "enter: open where it was written · ctrl-o: open the target · ctrl-/: hide pane",
	}
	if self2 != "" {
		q := self.previewArgs()
		opts.Preview = self2 + " links preview -at {1}" + q
		// Opening the far end in a browser, without leaving the list. The
		// terminal's own opener, because there is no knowing what a doi: or a
		// mailto: should do and the desktop already has an opinion.
		opts.Extra = []string{
			"--bind", "ctrl-o:execute-silent(" + self2 + " links open -at {1}" + q + ")",
		}
	}

	for _, chosen := range commands.Pick(opts) {
		addr, ok := commands.Address(chosen, 1)
		if !ok {
			continue
		}
		i, cerr := strconv.Atoi(strings.TrimSpace(addr[0]))
		if cerr != nil || i < 0 || i >= len(rows) {
			continue
		}
		// Enter goes to where the link was *written*, not where it points.
		// Following it is the browser's job; getting back to the heading you
		// wrote it in is the thing the terminal can do that the browser cannot.
		core.LaunchEditor(rows[i].Filename, rows[i].Line+1)
	}
}

// The flags the pane has to be given so that it indexes into the same list this
// picker is showing. Anything that changes which links are in the answer has to
// be here, or `-at 4` means a different link in the child than in the parent.
func (self *Links) previewArgs() string {
	out := ""
	if self.Query != "" {
		out += " -q " + commands.Shq(self.Query)
	}
	if self.Service != "" {
		out += " -service " + commands.Shq(self.Service)
	}
	if self.File != "" {
		out += " -file " + commands.Shq(self.File)
	}
	if self.Broken {
		out += " -broken"
	}
	return out
}

// One line of the picker: what the link says, where it goes, and where it was
// written - the last of those in the display rather than only in the data, so
// that "that link in the meeting notes" is something you can type.
func pickLine(l common.LinkEntry) string {
	var b strings.Builder
	if l.Broken {
		fmt.Fprintf(&b, "%s✗%s ", commands.C(commands.AnsiRed), commands.C(commands.AnsiReset))
	} else {
		b.WriteString("  ")
	}
	desc := l.Desc
	if desc == "" {
		desc = l.Raw
	}
	fmt.Fprintf(&b, "%s%s%s", commands.C(commands.AnsiBold), commands.Ellipsis(desc, 48), commands.C(commands.AnsiReset))
	if g := groupOf(l); g != "" {
		fmt.Fprintf(&b, "  %s[%s]%s", commands.C(commands.AnsiCyan), g, commands.C(commands.AnsiReset))
	}
	where := l.Heading
	if where == "" {
		where = "(preamble)"
	}
	fmt.Fprintf(&b, "  %s%s · %s:%d%s",
		commands.C(commands.AnsiDim), where, l.Filename, l.Line+1, commands.C(commands.AnsiReset))
	return b.String()
}

// ---------------------------------------------------------------------------
// The pane
// ---------------------------------------------------------------------------

func (self *Links) preview(core *commands.Core) {
	// Drawn for fzf rather than for a terminal, so the colour has to be said
	// explicitly - stdout here is a pipe.
	commands.PickerOutput()
	rows := self.rows(core)
	if self.At < 0 || self.At >= len(rows) {
		fmt.Printf("%sthat link is not in the list any more%s\n",
			commands.C(commands.AnsiDim), commands.C(commands.AnsiReset))
		return
	}
	RenderPane(core, rows[self.At], commands.PaneWidth())
}

// RenderPane draws one link the way the `orgs links` pane does: what it says,
// the lines it was written among, and the far end. Exported for `orgs go`,
// whose pane is the same question.
func RenderPane(core *commands.Core, l common.LinkEntry, width int) {
	// ---- what it says, and what it is
	desc := l.Desc
	if desc == "" {
		desc = l.Raw
	}
	fmt.Printf("%s%s%s\n", commands.C(commands.AnsiBold), desc, commands.C(commands.AnsiReset))
	fmt.Printf("%s%s%s\n", commands.C(commands.AnsiCyan), l.Raw, commands.C(commands.AnsiReset))

	bits := []string{l.Kind}
	if g := groupOf(l); g != "" && g != l.Kind {
		bits = append(bits, g)
	}
	if l.Host != "" && l.Host != l.Service {
		bits = append(bits, l.Host)
	}
	if l.Broken {
		bits = append(bits, commands.C(commands.AnsiRed)+"broken"+commands.C(commands.AnsiReset))
	}
	fmt.Printf("%s%s%s\n\n", commands.C(commands.AnsiDim), strings.Join(bits, " · "), commands.C(commands.AnsiReset))

	// ---- where it was written, which is what says whether this is the one
	where := l.Heading
	if where == "" {
		where = "(preamble)"
	}
	commands.OpenBox(fmt.Sprintf("written in %s — %s:%d",
		where, commands.BaseName(l.Filename), l.Line+1), width)
	for _, line := range contextAround(l.Filename, l.Line, contextLines) {
		commands.BoxLine(line)
	}
	commands.CloseBox(width)

	// ---- the far end, when there is one this side can show
	switch {
	case l.ToHash != "":
		fmt.Println()
		commands.OpenBox("lands on "+l.ToHeadline, width)
		body, err := commands.SendReceiveGetErr[bodyResult](core, "body/"+commands.HashPath(l.ToHash), nil)
		switch {
		case err != nil:
			commands.BoxLine(commands.C(commands.AnsiRed) + err.Error() + commands.C(commands.AnsiReset))
		case strings.TrimSpace(body.Text) == "":
			commands.BoxLine(commands.C(commands.AnsiDim) + "(nothing written under that heading)" + commands.C(commands.AnsiReset))
		default:
			commands.BoxText(commands.Ellipsis(body.Text, 1200))
		}
		commands.CloseBox(width)

	case l.ToFilename != "":
		fmt.Println()
		commands.OpenBox("lands on "+commands.BaseName(l.ToFilename), width)
		commands.BoxLine(commands.C(commands.AnsiDim) + l.ToFilename + commands.C(commands.AnsiReset))
		commands.CloseBox(width)

	case l.Media != "":
		// A link at a file beside the notes - a picture, a recording, a pdf.
		// The server resolved it, because a link is written relative to the org
		// file that holds it and only that side knows the org roots.
		fmt.Println()
		commands.OpenBox("a "+l.Media+" beside the notes", width)
		commands.BoxLine(l.Url)
		commands.CloseBox(width)

	case l.Broken:
		fmt.Println()
		commands.OpenBox("the far end", width)
		commands.BoxLine(commands.C(commands.AnsiRed) +
			"This names an org target nothing answers to." + commands.C(commands.AnsiReset))
		commands.CloseBox(width)
	}
}

// The heading body, as /body/{hash} answers it.
type bodyResult struct {
	Ok   bool
	Msg  string
	Text string
}

// The lines of the file around the one the link is on, with that line marked.
//
// Read here rather than asked for: the file is on this machine as often as not,
// and a whole endpoint for "six lines of a file" is a lot of server for a
// preview pane. When it is *not* on this machine the pane simply says so, which
// is honest and costs nothing - the rest of the pane is still worth reading.
func contextAround(filename string, line int, n int) []string {
	data, err := os.ReadFile(filename)
	if err != nil {
		return []string{commands.C(commands.AnsiDim) +
			"(not readable from here — the server has it)" + commands.C(commands.AnsiReset)}
	}
	all := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	from := line - n/2
	if from < 0 {
		from = 0
	}
	to := from + n
	if to > len(all) {
		to = len(all)
		from = to - n
		if from < 0 {
			from = 0
		}
	}
	out := make([]string, 0, to-from)
	for i := from; i < to; i++ {
		mark := "  "
		body := all[i]
		if i == line {
			mark = commands.C(commands.AnsiGold) + "▸ " + commands.C(commands.AnsiReset)
			body = commands.C(commands.AnsiBold) + body + commands.C(commands.AnsiReset)
		}
		out = append(out, fmt.Sprintf("%s%s%4d%s %s",
			mark, commands.C(commands.AnsiDim), i+1, commands.C(commands.AnsiReset), body))
	}
	return out
}

// ---------------------------------------------------------------------------
// Opening the far end
// ---------------------------------------------------------------------------

// `orgs links open -at N`, which the picker binds to ctrl-o.
//
// It hands the url to whatever the desktop uses, rather than deciding anything
// itself: there is no knowing here what a doi: or a mailto: ought to do, and
// the desktop already has an opinion about both.
func (self *Links) openTarget(core *commands.Core) {
	rows := self.rows(core)
	if self.At < 0 || self.At >= len(rows) {
		return
	}
	l := rows[self.At]
	if l.Kind != "external" {
		// Not something out there. Open where it was written instead, which is
		// the only thing "open" can mean for a link between two org files.
		core.LaunchEditor(l.Filename, l.Line+1)
		return
	}
	commands.OpenInBrowser(l.Raw)
}
