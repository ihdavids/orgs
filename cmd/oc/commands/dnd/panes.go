package dnd

// Two pane rendering for the interactive character builder.
//
// The chooser on the left is still an ordinary survey prompt - arrow keys,
// type to filter and paging all come from the library - but the question
// template is swapped for one that hands the whole body to renderPanes. That
// lets us draw the full rules text of whatever option is under the cursor in a
// second pane on the right, and to keep a live count of what is selected.
//
// survey only ever gives a template the option *label* strings, so everything
// else about an option is looked up in the paneCtx that the ask helpers install
// around each prompt.

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/ihdavids/orgs/internal/common/dnd"
	"golang.org/x/term"
	survey "gopkg.in/AlecAivazis/survey.v1"
	"gopkg.in/AlecAivazis/survey.v1/core"
)

// paneCtx describes the prompt that is currently on screen. Only one prompt is
// ever up at a time, so a single package level value is enough.
type paneCtx struct {
	byLabel map[string]dnd.Option // option metadata, keyed by menu label
	extras  map[string]string     // help text for the "  ···  " menu entries
	multi   bool
	min     int
	max     int
}

var activePane *paneCtx

// menuHelp explains the entries that are not ruleset choices, so that the
// detail pane has something to say when the cursor is sitting on one of them.
var menuHelp = map[string]string{
	entryDetails: "Print the full rules text for every option in this list, then ask again.",
	entryRandom:  "Let the dice decide. The builder makes a sensible choice for your class.",
	entryBack:    "Return to the previous question. Later answers are replayed onto the change.",
	entryCustom:  "Type your own value instead of taking one from the list.",
	entrySkip:    "Leave this out for now. You can always add it to the org file by hand later.",
	entryQuit:    "Abandon this character. Nothing is written to disk.",
	entryAgain:   "Show the list again so you can select the right number of entries.",
	entryFill:    "Answer each line in turn. Every one of them can still be left blank.",
	entryBlank:   "Leave this one line out. The rest of the questions still get asked.",
}

// The chooser template renders everything after the question mark itself, so
// that every line can be clipped to the width of the terminal. survey erases
// the prompt by counting newlines, and a line that wraps would break that.
const chooserTemplate = `
{{- color "green+hb"}}{{ QuestionIcon }} {{color "reset"}}{{ DndPanes . }}`

func init() {
	core.TemplateFuncs["DndPanes"] = renderPanes
}

// paneAsk runs one list question. The caller describes it with an ordinary
// survey.Select or MultiSelect, which is then handed to the chooser in
// chooser.go - survey's own list prompts cannot filter the way we want.
func paneAsk(p survey.Prompt, response interface{}, ctx *paneCtx) error {
	activePane = ctx
	defer func() { activePane = nil }()
	var c *chooser
	switch q := p.(type) {
	case *survey.Select:
		c = newChooser(q.Message, q.Options, q.Help, q.PageSize, q.Default, nil, false, ctx)
	case *survey.MultiSelect:
		c = newChooser(q.Message, q.Options, q.Help, q.PageSize, "", q.Default, true, ctx)
	default:
		return survey.AskOne(p, response, nil)
	}
	return survey.AskOne(c, response, nil)
}

// paneInput is the flattened form of the two survey template data types.
type paneInput struct {
	message    string
	filter     string
	help       string
	showHelp   bool
	showAnswer bool
	answer     string
	entries    []string
	sel        int
	options    []string
	checked    map[string]bool
	multi      bool
	// filtering says a filter is in force, which is why the list may be short
	// or empty.
	filtering bool
}

// renderPanes is the DndPanes template function.
func renderPanes(data interface{}) string {
	if in, ok := data.(paneInput); ok {
		return panesBody(in)
	}
	return "\n"
}

// paneLine is one line of the detail pane plus the colour to draw it in.
type paneLine struct {
	text  string
	color string
}

func panesBody(in paneInput) string {
	width, height := termSize()
	var b strings.Builder

	// the question itself continues the line the template icon started
	b.WriteString(cBold + clip(in.message+in.filter, width-3) + cReset)
	if in.showAnswer {
		b.WriteString(cCyan + " " + clip(paneAnswer(in), width-6-visLen(in.message)) + cReset + "\n")
		return b.String()
	}
	b.WriteString("\n")

	// the live count of what is picked comes first: on a narrow terminal it is
	// the legend that gets clipped, not the number the player is counting on
	status := ""
	if in.multi {
		n := countChecked(in)
		status = countColor(n) + countText(n) + cReset + "  "
	}
	legend := "[↑↓ move · type to filter · ⌫ to widen · enter to accept]"
	if in.multi {
		legend = "[↑↓ move · space to select · type to filter · ⌫ to widen · enter to accept]"
	}
	if in.help != "" && !in.showHelp {
		legend = strings.TrimSuffix(legend, "]") + " · ? for help]"
	}
	if room := width - visLen(status) - 1; room > 12 {
		status += cCyan + clip(legend, room) + cReset
	}
	b.WriteString(status + "\n")

	if in.multi {
		if names := checkedNames(in); names != "" {
			b.WriteString(cDim + clip("selected: "+names, width-1) + cReset + "\n")
		}
	}
	if in.showHelp && in.help != "" {
		for _, l := range wrapLines(in.help, width-3) {
			b.WriteString(cDim + "  " + clip(l, width-3) + cReset + "\n")
		}
	}

	// the two panes themselves
	leftW, rightW := paneWidths(width)
	detail := []paneLine{}
	if rightW > 0 && in.sel >= 0 && in.sel < len(in.entries) {
		detail = detailLines(in.entries[in.sel], rightW)
	}
	rows := len(in.entries)
	if len(detail) > rows {
		maxRows := height - 10
		if maxRows < rows {
			maxRows = rows
		}
		if len(detail) < maxRows {
			rows = len(detail)
		} else {
			rows = maxRows
		}
	}
	if len(detail) > rows && rows > 0 {
		more := len(detail) - rows + 1
		detail = detail[:rows]
		detail[rows-1] = paneLine{fmt.Sprintf("… %d more lines", more), cDim}
	}

	if rows == 0 {
		msg := "  nothing matches that, backspace to widen the filter"
		if !in.filtering {
			msg = "  there is nothing to choose from here"
		}
		b.WriteString(cDim + clip(msg, width-1) + cReset + "\n")
	}
	for i := 0; i < rows; i++ {
		line := ""
		if i < len(in.entries) {
			line = entryCell(in, i, leftW, rightW > 0)
		} else if rightW > 0 {
			line = strings.Repeat(" ", leftW)
		}
		// the divider runs the whole height so the two panes stay square
		if rightW > 0 && len(detail) > 0 {
			line += cDim + "│" + cReset + " "
			if i < len(detail) && detail[i].text != "" {
				if detail[i].color == "" {
					line += clip(detail[i].text, rightW)
				} else {
					line += detail[i].color + clip(detail[i].text, rightW) + cReset
				}
			}
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}

// entryCell renders one row of the chooser, padded to the pane width.
func entryCell(in paneInput, i, leftW int, padded bool) string {
	cell := ""
	prefix := 2
	if i == in.sel {
		cell += cCyan + core.SelectFocusIcon + cReset + " "
	} else {
		cell += "  "
	}
	if in.multi {
		prefix = 4
		if in.checked[in.entries[i]] {
			cell += cGreen + core.MarkedOptionIcon + cReset + " "
		} else {
			cell += cDim + core.UnmarkedOptionIcon + cReset + " "
		}
	}
	text := in.entries[i]
	if padded {
		text = fit(text, leftW-prefix-1) + " "
	} else {
		text = clip(text, leftW-prefix)
	}
	if i == in.sel {
		return cell + cBold + text + cReset
	}
	return cell + text
}

// detailLines is the right hand pane: everything we know about one option.
func detailLines(label string, w int) []paneLine {
	out := []paneLine{}
	o, ok := paneOption(label)
	if !ok {
		if txt := paneExtra(label); txt != "" {
			out = append(out, paneLine{paneName(label), cBold}, paneLine{"", ""})
			for _, l := range wrapLines(txt, w) {
				out = append(out, paneLine{l, cDim})
			}
		}
		return out
	}
	out = append(out, paneLine{o.Name, cBold + cGold})
	if o.Recommended {
		note := "★ recommended"
		if o.Reason != "" {
			note += ": " + o.Reason
		}
		for _, l := range wrapLines(note, w) {
			out = append(out, paneLine{l, cGold})
		}
	}
	if meta := metaText(o); meta != "" {
		for _, l := range wrapLines(meta, w) {
			out = append(out, paneLine{l, cCyan})
		}
	}
	body := strings.TrimSpace(o.Detail)
	if body == "" {
		body = strings.TrimSpace(o.Summary)
	}
	out = append(out, paneLine{"", ""})
	if body == "" {
		out = append(out, paneLine{"This ruleset has no further description for this entry.", cDim})
		return out
	}
	for _, l := range wrapLines(body, w) {
		out = append(out, paneLine{l, ""})
	}
	return out
}

// metaText is the one line of stats under an option name in the detail pane.
func metaText(o dnd.Option) string {
	bits := []string{}
	keys := []string{}
	for k := range o.Meta {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if o.Meta[k] != "" {
			bits = append(bits, fmt.Sprintf("%s %s", k, o.Meta[k]))
		}
	}
	for _, t := range o.Tags {
		if t != "" && !containsFold(bits, t) {
			bits = append(bits, t)
		}
	}
	return strings.Join(bits, " · ")
}

// ----------------------------------------------------------------------------
// selection counting
// ----------------------------------------------------------------------------

func countChecked(in paneInput) int {
	n := 0
	for _, o := range in.options {
		if in.checked[o] {
			n++
		}
	}
	return n
}

// checkedNames lists what is picked so far, by name rather than by menu label.
func checkedNames(in paneInput) string {
	names := []string{}
	for _, o := range in.options {
		if in.checked[o] {
			names = append(names, paneName(o))
		}
	}
	return strings.Join(names, ", ")
}

// countText is the running "selected 1 of 2" that sits beside the legend.
func countText(n int) string {
	wantMin, wantMax := paneRange()
	switch {
	case wantMin > 0 && wantMin == wantMax:
		return fmt.Sprintf("selected %d of %d", n, wantMax)
	case wantMax > 0 && wantMin > 0:
		return fmt.Sprintf("selected %d of %d-%d", n, wantMin, wantMax)
	case wantMax > 0:
		return fmt.Sprintf("selected %d, up to %d", n, wantMax)
	case wantMin > 0:
		return fmt.Sprintf("selected %d, at least %d", n, wantMin)
	}
	return fmt.Sprintf("selected %d", n)
}

func countColor(n int) string {
	wantMin, wantMax := paneRange()
	if countOk(n, wantMin, wantMax) {
		return cGreen
	}
	return cGold
}

// countOk reports whether n picks satisfy the min/max the server asked for.
func countOk(n, wantMin, wantMax int) bool {
	if n == 0 {
		return false
	}
	if wantMin > 0 && n < wantMin {
		return false
	}
	if wantMax > 0 && n > wantMax {
		return false
	}
	return true
}

// needText describes the required number of picks for an error message.
func needText(wantMin, wantMax int) string {
	switch {
	case wantMin > 0 && wantMin == wantMax:
		return fmt.Sprintf("pick exactly %d", wantMin)
	case wantMin > 0 && wantMax > 0:
		return fmt.Sprintf("pick between %d and %d", wantMin, wantMax)
	case wantMax > 0:
		return fmt.Sprintf("pick at most %d", wantMax)
	case wantMin > 0:
		return fmt.Sprintf("pick at least %d", wantMin)
	}
	return "pick at least one"
}

func paneRange() (int, int) {
	if activePane == nil {
		return 0, 0
	}
	return activePane.min, activePane.max
}

func paneOption(label string) (dnd.Option, bool) {
	if activePane == nil {
		return dnd.Option{}, false
	}
	o, ok := activePane.byLabel[label]
	return o, ok
}

func paneExtra(label string) string {
	if activePane != nil {
		if txt, ok := activePane.extras[label]; ok {
			return txt
		}
	}
	return menuHelp[label]
}

// paneName is the short name of a label, used for the running selection list
// and for the answer echoed once the prompt is done.
func paneName(label string) string {
	if o, ok := paneOption(label); ok {
		return o.Name
	}
	name := strings.TrimSpace(label)
	name = strings.TrimSpace(strings.TrimPrefix(name, "···"))
	if i := strings.Index(name, "  —  "); i > 0 {
		name = name[:i]
	}
	return strings.TrimSpace(strings.TrimPrefix(name, "★"))
}

// paneAnswer is what survey echoes after the prompt is answered. The labels
// carry a summary each, which makes for a very noisy transcript, so echo the
// names instead.
func paneAnswer(in paneInput) string {
	if in.multi {
		names := checkedNames(in)
		if names == "" {
			return in.answer
		}
		return fmt.Sprintf("%s  (%d)", names, countChecked(in))
	}
	return paneName(in.answer)
}

// ----------------------------------------------------------------------------
// geometry and text fitting
// ----------------------------------------------------------------------------

func termSize() (int, int) {
	w, h, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || w <= 20 {
		return 100, 30
	}
	if h <= 10 {
		h = 30
	}
	return w, h
}

// paneWidths splits the terminal between the chooser and the detail pane. A
// narrow terminal gets the chooser only, with a right width of zero.
func paneWidths(width int) (int, int) {
	left := width * 45 / 100
	if left < 34 {
		left = 34
	}
	if left > 64 {
		left = 64
	}
	right := width - left - 3
	if width < 92 || right < 26 {
		return width - 1, 0
	}
	return left, right
}

// panePageSize keeps the chooser short enough that the whole prompt fits on
// screen, which matters because survey redraws it by walking back up the lines.
func panePageSize() int {
	_, h := termSize()
	n := h - 12
	if n < 5 {
		n = 5
	}
	if n > 16 {
		n = 16
	}
	return n
}

func wrapLines(text string, w int) []string {
	if w < 8 {
		w = 8
	}
	return strings.Split(wrap(text, w), "\n")
}

// visLen is the printed width of a string, ignoring the escape sequences.
func visLen(s string) int {
	n, esc := 0, false
	for _, r := range s {
		switch {
		case esc && r == 'm':
			esc = false
		case esc:
		case r == '\033':
			esc = true
		default:
			n++
		}
	}
	return n
}

// clip shortens plain text to w printed columns.
func clip(s string, w int) string {
	if w <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= w {
		return s
	}
	if w == 1 {
		return "…"
	}
	return string(runes[:w-1]) + "…"
}

// fit clips and then pads plain text so that it is exactly w columns wide.
func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	s = clip(s, w)
	if n := w - len([]rune(s)); n > 0 {
		s += strings.Repeat(" ", n)
	}
	return s
}

func containsFold(list []string, v string) bool {
	for _, l := range list {
		if strings.EqualFold(l, v) || strings.HasSuffix(strings.ToLower(l), " "+strings.ToLower(v)) {
			return true
		}
	}
	return false
}

func hasStr(list []string, v string) bool {
	for _, l := range list {
		if l == v {
			return true
		}
	}
	return false
}
