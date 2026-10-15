package typst

import (
	"regexp"
	"strings"
	"unicode"
)

// Escaping is where a typst exporter breaks, so it is all in one place.
//
// Typst has three kinds of text this writer produces, each with its own rules:
//
//   - markup, the body of the document: a handful of characters start
//     something (`#` code, `*` strong, `_` emphasis, `` ` `` raw, `$` math, `<`
//     a label, `@` a reference, `\` an escape or a line break, `~` a
//     non-breaking space, `[` `]` content, `//` and `/*` comments), and a few
//     more do so only at the start of a line (`=` a heading, `-` a list, `+`
//     an enumeration, `/ ` a term, `1.` a numbered item). Every one of them
//     takes a backslash, and a backslash is all it needs.
//   - strings in code (`"..."`), where only `\` and `"` matter.
//   - raw text, which needs a fence longer than any run of backticks in it.

// The characters that start something wherever they are in markup.
var markupSpecial = map[rune]bool{
	'\\': true, '#': true, '*': true, '_': true, '`': true, '$': true,
	'<': true, '>': true, '@': true, '[': true, ']': true, '~': true,
}

// A line that typst would read as a heading, list, enumeration or term.
var lineStartRe = regexp.MustCompile(`^([ \t]*)([=+\-]|/ |\d+\.)`)

// escapeMarkup makes text safe to put in markup. bol says the text starts a
// line (or a content block, which is the same thing to typst's parser).
func escapeMarkup(s string, bol bool) string {
	if s == "" {
		return s
	}
	b := strings.Builder{}
	runes := []rune(s)
	for i, r := range runes {
		switch {
		case markupSpecial[r]:
			b.WriteRune('\\')
			b.WriteRune(r)
		case r == '/' && i+1 < len(runes) && (runes[i+1] == '/' || runes[i+1] == '*'):
			// The start of a comment; a / anywhere else is a slash.
			b.WriteString(`\/`)
		default:
			b.WriteRune(r)
		}
	}
	out := b.String()
	lines := strings.Split(out, "\n")
	for i, l := range lines {
		if i == 0 && !bol {
			continue
		}
		if m := lineStartRe.FindStringSubmatchIndex(l); m != nil {
			// Before the marker's first character: `\=`, `\-`, `1\.`.
			at := m[4]
			if l[at] >= '0' && l[at] <= '9' {
				at = m[5] - 1
			}
			lines[i] = l[:at] + `\` + l[at:]
		}
	}
	return strings.Join(lines, "\n")
}

// str is s as a typst string literal, quotes included.
func str(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", ``, "\t", `\t`).Replace(s) + `"`
}

// fence is the backticks that can open and close raw text holding s: three,
// or one more than the longest run of them inside it.
func fence(s string) string {
	longest, run := 0, 0
	for _, r := range s {
		if r == '`' {
			run++
			if run > longest {
				longest = run
			}
		} else {
			run = 0
		}
	}
	n := 3
	if longest >= n {
		n = longest + 1
	}
	return strings.Repeat("`", n)
}

// A typst label may hold letters, digits, _ - : and . - and two labels the
// same in one document make every link to either an error, which is why
// labelFor hands them out.
func labelText(s string) string {
	b := strings.Builder{}
	dash := false
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '.' || r == ':' {
			b.WriteRune(r)
			dash = false
		} else if !dash {
			b.WriteRune('-')
			dash = true
		}
	}
	return strings.Trim(b.String(), "-.:")
}

// A language name a raw fence can carry: one word, nothing that would end it.
func langWord(s string) string {
	s = strings.TrimSpace(s)
	for _, r := range s {
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '+' || r == '#' || r == '.') {
			return ""
		}
	}
	return s
}
