//lint:file-ignore ST1006 allow the use of self
package capture

// The capture form: the template on screen, with the cursor in it.
//
// What this replaces is two boxes asked one after the other - a headline, then
// a body - which is a fair description of what a capture *sends* and no
// description at all of what a template *is*. A template is the shape of an
// entry: a drawer with three properties in it, two of which the server has
// already answered, one of which is a question, and a body. Asking for that
// through two unlabelled text areas means the person typing has to hold the
// shape in their head and never gets to see the thing they are making.
//
// So the form is the entry itself, drawn as org and coloured as org, with the
// cursor sitting in whichever hole is being filled in. Tab moves to the next
// hole. That is org-capture's own idea and it is the right one: what you are
// editing is a document, not a dialog.
//
// Four things about it:
//
//  1. **A hole nobody has filled in is still drawn as a hole** - `{{project}}`,
//     in the colour orghl gives a placeholder - rather than as an empty space.
//     An empty space says "this line is finished"; the braces say "this line is
//     waiting for you", which is true, and it is also exactly what the line
//     will look like in an org-capture buffer in Emacs.
//  2. **Nothing is dropped while it is being edited.** `common.FillCapTemplate`
//     drops a line whose only content was an unanswered placeholder, which is
//     right when filing and disastrous while typing: the line you are editing
//     would vanish under you the moment you cleared it. The editor substitutes
//     everything and keeps every line; the dropping happens once, on the way
//     out.
//  3. **The preview is the entry as it will be filed**, down to the stars and
//     the indent: `InsertEntryUsingTemplate` writes the headline at the target's
//     level plus one and indents every line of the content under it. Getting
//     this wrong would make the form a drawing of something else.
//  4. **The caret is a span, not a character.** It is pushed into the text as a
//     sentinel, the text is coloured, and the span holding the sentinel is split
//     and drawn with the character under the cursor in reverse video - so the
//     cursor is inside the coloured org rather than beside it, and no colour is
//     lost at the join.

import (
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/ihdavids/orgs/cmd/oc/commands/orghl"
	"github.com/ihdavids/orgs/internal/common"
	"github.com/rivo/tview"
)

// Where the caret is while the text is being coloured. A rune no org file has
// any business containing, so finding it again is unambiguous.
const caretMark = "\x00"

// What kind of hole a field is. The template's own fields are the common case;
// a headline and a set of tags belong to the *entry* rather than to the
// template, and are asked for in the same list because that is what somebody
// filling this in is doing - making one entry.
type fieldKind int

const (
	fieldTemplate fieldKind = iota
	fieldHeadline
	fieldTags
)

type field struct {
	Kind    fieldKind
	Key     string
	Prompt  string
	Default string
	Auto    bool
	Multi   bool
	val     []rune
}

func (f *field) String() string { return string(f.val) }
func (f *field) set(s string)   { f.val = []rune(s) }
func (f *field) empty() bool    { return len(f.val) == 0 }

type form struct {
	app  *tview.Application
	body *tview.TextView
	hint *tview.TextView
	keys *tview.TextView

	tpl    common.CaptureTemplate
	fields []*field
	at     int
	cur    int // the caret, as a rune index into the active field
	st     orghl.State
	level  int // the level the headline will be written at

	saved bool
}

// buildFields is the list of holes, in the order they are asked: the headline
// and its tags first for an entry (they are the top line of the thing), then
// the template's own in the order the template wrote them.
func buildFields(tpl common.CaptureTemplate, wantsHead bool) []*field {
	out := []*field{}
	if wantsHead {
		out = append(out, &field{Kind: fieldHeadline, Key: "HEADLINE", Prompt: "Headline"})
		out = append(out, &field{Kind: fieldTags, Key: "TAGS", Prompt: "Tags"})
	}
	for _, f := range common.CapFields(tpl.Template) {
		out = append(out, &field{
			Kind:    fieldTemplate,
			Key:     f.Key,
			Prompt:  f.Prompt,
			Default: f.Default,
			Auto:    f.Auto,
			Multi:   f.Content,
		})
	}
	for _, f := range out {
		f.set(f.Default)
	}
	return out
}

func (self *form) find(key string) *field {
	for _, f := range self.fields {
		if strings.EqualFold(f.Key, key) {
			return f
		}
	}
	return nil
}

func (self *form) active() *field {
	if self.at < 0 || self.at >= len(self.fields) {
		return nil
	}
	return self.fields[self.at]
}

// entry is the org text of what is being captured, with the caret pushed into
// whichever field is active.
func (self *form) entry() string {
	cur := self.active()
	mark := func(f *field) string {
		s := f.String()
		if f != cur {
			return s
		}
		at := self.cur
		if at > len([]rune(s)) {
			at = len([]rune(s))
		}
		r := []rune(s)
		return string(r[:at]) + caretMark + string(r[at:])
	}

	var out []string
	if h := self.find("HEADLINE"); h != nil {
		line := strings.Repeat("*", self.level) + " " + mark(h)
		// The tags go where org puts them, after the headline, so the heading
		// reads as the heading it is going to be. While they are being typed
		// they are drawn *as typed* rather than as `:a:b:` - the caret counts
		// characters in what somebody is typing, and a colon they did not type
		// is a column the caret would be wrong by.
		if t := self.find("TAGS"); t != nil {
			if t == cur {
				line += "  " + mark(t)
			} else if tags := tagString(t.String()); tags != "" {
				line += "  " + tags
			}
		}
		out = append(out, line)
	}

	body := common.CapSubstitute(self.tpl.Template, func(key string) (string, bool) {
		f := self.find(key)
		if f == nil {
			return "", false
		}
		// A hole nobody has filled in keeps its braces - unless it is the one
		// being filled in now, which gets the caret instead.
		if f.empty() && f != cur {
			return "", false
		}
		return mark(f), true
	})
	if self.tpl.Template == "" {
		if c := self.find("CONTENT"); c != nil {
			body = mark(c)
		}
	}
	if self.find("HEADLINE") != nil {
		body = indentEach(body, strings.Repeat(" ", self.level+1))
	}
	if body != "" {
		out = append(out, strings.Split(body, "\n")...)
	}
	return strings.Join(out, "\n")
}

func tagString(s string) string {
	words := strings.FieldsFunc(s, func(r rune) bool { return r == ' ' || r == ',' || r == '\t' })
	if len(words) == 0 {
		return ""
	}
	return ":" + strings.Join(words, ":") + ":"
}

func indentEach(s, indent string) string {
	if s == "" {
		return s
	}
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		lines[i] = indent + l
	}
	return strings.Join(lines, "\n")
}

// draw colours the entry and puts the caret in it.
func (self *form) draw() {
	st := self.st
	st.InBlock = ""
	st.InDrawer = false
	var b strings.Builder
	for i, spans := range orghl.Lines(self.entry(), &st) {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(withCaret(spans))
	}
	self.body.SetText(b.String())
	self.body.SetTitle(self.title())
	self.hint.SetText(self.hintText())
}

// withCaret draws one line, splitting the span that holds the sentinel so the
// character under the cursor comes out in reverse video.
func withCaret(spans []orghl.Span) string {
	for i, s := range spans {
		at := strings.Index(s.Text, caretMark)
		if at < 0 {
			continue
		}
		before := append([]orghl.Span{}, spans[:i]...)
		if at > 0 {
			before = append(before, orghl.Span{Text: s.Text[:at], Role: s.Role})
		}
		rest := s.Text[at+len(caretMark):]
		under := " "
		if rest != "" {
			r := []rune(rest)
			under = string(r[0])
			rest = string(r[1:])
		}
		after := []orghl.Span{}
		if rest != "" {
			after = append(after, orghl.Span{Text: rest, Role: s.Role})
		}
		after = append(after, spans[i+1:]...)
		return orghl.Tview(before) + "[::r]" + tview.Escape(under) + "[::-]" + orghl.Tview(after)
	}
	return orghl.Tview(spans)
}

func (self *form) title() string {
	t := " " + self.tpl.Name
	if self.tpl.CapTarget.Filename != "" {
		t += " → " + self.tpl.CapTarget.Filename
		if self.tpl.CapTarget.Id != "" {
			t += " › " + strings.ReplaceAll(self.tpl.CapTarget.Id, "::", " › ")
		}
	}
	return t + " "
}

// The hint line says which hole the cursor is in, how far along the form it is,
// and the one thing worth knowing about this particular hole.
func (self *form) hintText() string {
	f := self.active()
	if f == nil {
		return ""
	}
	s := "[#268bd2::b]" + tview.Escape(f.Prompt) + "[-:-:-]"
	s += "[#5f5f5f] · " + itoa(self.at+1) + " of " + itoa(len(self.fields)) + "[-:-:-]"
	switch {
	case f.Kind == fieldTags:
		s += "[#5f5f5f] · words, spaces or commas between them[-:-:-]"
	case f.Multi:
		s += "[#5f5f5f] · the body · ↵ for a new line[-:-:-]"
	case f.Auto && f.Default != "":
		s += "[#859900] · answered by the server[-:-:-][#5f5f5f] · ^R puts it back[-:-:-]"
	case f.Default != "":
		s += "[#5f5f5f] · ^R puts the default back[-:-:-]"
	default:
		s += "[#5f5f5f] · ↵ for the next one[-:-:-]"
	}
	return s
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

const legend = "[#5f5f5f]tab[-:-:-] next  [#5f5f5f]⇧tab[-:-:-] back  " +
	"[#5f5f5f]^R[-:-:-] default  [#5f5f5f]^U[-:-:-] clear  " +
	"[#859900]^S[-:-:-] capture  [#cb4b16]esc[-:-:-] cancel"

// runForm puts the template up and answers with the values, or false if the
// person changed their mind.
func runForm(tpl common.CaptureTemplate, fields []*field, level int, st orghl.State) bool {
	f := &form{
		app:    tview.NewApplication(),
		tpl:    tpl,
		fields: fields,
		level:  level,
		st:     st,
	}
	if f.level < 1 {
		f.level = 1
	}
	// Open on the first thing that is actually a question: the holes the server
	// has already answered are usually right, and starting on one of them means
	// everybody tabs past it every time.
	f.at = 0
	for i, fl := range fields {
		if fl.empty() {
			f.at = i
			break
		}
	}
	f.cur = len(fields[f.at].val)

	f.body = tview.NewTextView().SetDynamicColors(true).SetWrap(true)
	f.body.SetBorder(true).SetTitleAlign(tview.AlignLeft).SetBorderPadding(0, 0, 1, 1)
	f.body.SetTitleColor(tcell.ColorMediumPurple)
	f.hint = tview.NewTextView().SetDynamicColors(true)
	f.keys = tview.NewTextView().SetDynamicColors(true).SetText(legend)

	flex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(f.body, 0, 1, true).
		AddItem(f.hint, 1, 0, false).
		AddItem(f.keys, 1, 0, false)

	f.app.SetInputCapture(f.key)
	f.draw()
	if err := f.app.SetRoot(flex, true).EnableMouse(false).Run(); err != nil {
		return false
	}
	return f.saved
}

// Moving between holes. The caret lands at the end of the next one, which is
// where somebody about to add to it wants it, and is what tabbing through a
// form does everywhere else.
func (self *form) move(d int) {
	n := len(self.fields)
	self.at = ((self.at+d)%n + n) % n
	self.cur = len(self.active().val)
}

func (self *form) key(ev *tcell.EventKey) *tcell.EventKey {
	f := self.active()
	if f == nil {
		return ev
	}
	defer self.draw()

	switch ev.Key() {
	case tcell.KeyEscape, tcell.KeyCtrlC:
		self.saved = false
		self.app.Stop()
		return nil
	case tcell.KeyCtrlS, tcell.KeyCtrlG:
		self.saved = true
		self.app.Stop()
		return nil
	case tcell.KeyTab, tcell.KeyCtrlN:
		self.move(1)
		return nil
	case tcell.KeyBacktab, tcell.KeyCtrlP:
		self.move(-1)
		return nil
	case tcell.KeyEnter:
		if f.Multi {
			self.insert('\n')
		} else {
			self.move(1)
		}
		return nil
	case tcell.KeyUp:
		if !self.lineMove(-1) {
			self.move(-1)
		}
		return nil
	case tcell.KeyDown:
		if !self.lineMove(1) {
			self.move(1)
		}
		return nil
	case tcell.KeyLeft:
		if self.cur > 0 {
			self.cur--
		}
		return nil
	case tcell.KeyRight:
		if self.cur < len(f.val) {
			self.cur++
		}
		return nil
	case tcell.KeyHome, tcell.KeyCtrlA:
		self.cur = 0
		return nil
	case tcell.KeyEnd, tcell.KeyCtrlE:
		self.cur = len(f.val)
		return nil
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		if self.cur > 0 {
			f.val = append(f.val[:self.cur-1], f.val[self.cur:]...)
			self.cur--
		}
		return nil
	case tcell.KeyDelete:
		if self.cur < len(f.val) {
			f.val = append(f.val[:self.cur], f.val[self.cur+1:]...)
		}
		return nil
	case tcell.KeyCtrlU:
		f.val = nil
		self.cur = 0
		return nil
	case tcell.KeyCtrlK:
		f.val = f.val[:self.cur]
		return nil
	// Putting the default back is worth a key of its own: the thing most likely
	// to be cleared by accident is the one value nobody can type again - a uuid.
	case tcell.KeyCtrlR:
		f.set(f.Default)
		self.cur = len(f.val)
		return nil
	case tcell.KeyRune:
		self.insert(ev.Rune())
		return nil
	}
	return nil
}

func (self *form) insert(r rune) {
	f := self.active()
	f.val = append(f.val[:self.cur], append([]rune{r}, f.val[self.cur:]...)...)
	self.cur++
}

// lineMove walks up or down inside a body that has more than one line, and
// answers false when there is nowhere to go - which is when up and down mean
// the previous and the next hole instead.
func (self *form) lineMove(d int) bool {
	f := self.active()
	if !f.Multi {
		return false
	}
	s := f.val
	start := self.cur
	for start > 0 && s[start-1] != '\n' {
		start--
	}
	col := self.cur - start
	if d < 0 {
		if start == 0 {
			return false
		}
		prev := start - 1
		for prev > 0 && s[prev-1] != '\n' {
			prev--
		}
		self.cur = min(prev+col, start-1)
		return true
	}
	end := self.cur
	for end < len(s) && s[end] != '\n' {
		end++
	}
	if end >= len(s) {
		return false
	}
	nextEnd := end + 1
	for nextEnd < len(s) && s[nextEnd] != '\n' {
		nextEnd++
	}
	self.cur = min(end+1+col, nextEnd)
	return true
}
