package help

import (
	"strconv"
	"strings"
)

// roffEscape makes text safe to put on a roff line: backslashes and dashes
// escaped, and a line that would start with a control character guarded.
func roffEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\e`)
	s = strings.ReplaceAll(s, "-", `\-`)
	return s
}

func roffLine(s string) string {
	if strings.HasPrefix(s, ".") || strings.HasPrefix(s, "'") {
		return `\&` + s
	}
	return s
}

// roffInline is a line of org in roff, code and bold in bold. upper shouts
// the plain words, for a section heading.
func roffInline(s string, upper bool) string {
	b := strings.Builder{}
	for _, sp := range inline(s) {
		t := roffEscape(sp.s)
		switch sp.kind {
		case spText:
			if upper {
				t = strings.ToUpper(t)
			}
			b.WriteString(t)
		default:
			b.WriteString(`\fB` + t + `\fR`)
		}
	}
	return b.String()
}

// renderRoff writes org text as man page sections. A heading at level top
// is a section (.SH), deeper ones subsections (.SS); a heading above top
// cannot happen, as the text is relevelled first.
func renderRoff(text string, top int) string {
	b := &strings.Builder{}
	for _, e := range parseOrg(text) {
		switch e.kind {
		case elHeading:
			if e.level <= top {
				b.WriteString(".SH \"" + strings.ReplaceAll(roffInline(e.text, true), `"`, `\(dq`) + "\"\n")
			} else {
				b.WriteString(".SS \"" + strings.ReplaceAll(roffInline(e.text, false), `"`, `\(dq`) + "\"\n")
			}
		case elPara:
			b.WriteString(".PP\n" + roffLine(roffInline(e.text, false)) + "\n")
		case elItem:
			b.WriteString(".RS " + strconv.Itoa(4*e.level) + "\n.IP \\(bu 2\n" + roffLine(roffInline(e.text, false)) + "\n.RE\n")
		case elPre:
			b.WriteString(".PP\n.RS 4\n.nf\n")
			for _, l := range e.lines {
				b.WriteString(roffLine(roffEscape(l)) + "\n")
			}
			b.WriteString(".fi\n.RE\n")
		}
	}
	return b.String()
}
