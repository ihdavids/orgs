package common

// Org's own capture template escapes (org manual 10.1.3.1), read into orgs's
// {{...}} grammar (captemplate.go) so a template copied out of an Emacs
// config works as it is. The server translates on the way to a client
// (FillCaptureTemplateAutos), so worg and `orgs cap` see only orgs's grammar.
//
//	%?  %i               {{CONTENT}} (where the text goes), {{initial}}
//	%^{Who}  %^{Who|Sam|Kim}   {{who|Who}}, {{who|Who|=Sam}}
//	%\1                  the first prompt's answer again
//	%t %T %u %U          {{today}} {{active_now}} {{inactive_today}} {{now}}
//	%^t %^T %^u %^U      the same, offered for changing (every default is)
//	%<%Y-%m-%d %H:%M>    the time now, in that format
//	%a %A %l %L          {{annotation}}, {{link}} (org-protocol fills them)
//	%:link %:description %:initial   {{link}} {{description}} {{initial}}
//	%c %x %^C            {{clipboard|Clipboard}}
//	%^g %^G              {{tags|Tags}}
//	%^{EFFORT}p          a property drawer under the headline: :EFFORT: {{effort|EFFORT}}
//	%n                   {{username}}
//	%%                   a %
//
// %f, %F, %k and %K (the file and clock of the Emacs buffer capture was called
// from) have nothing to name here and are left out; %(sexp) and %[file] are
// left as written.

import (
	"regexp"
	"strings"
	"time"
)

var orgCaptureEscapeRe = regexp.MustCompile(`%(\?|\^[{a-zA-Z]|:[a-z]|<|\\[0-9]|[iaAlLcxntTuUfFkK](?:[^a-zA-Z]|$))`)

// IsOrgCaptureTemplate is whether a template uses org's escapes and none of
// orgs's own placeholders.
func IsOrgCaptureTemplate(t string) bool {
	return !strings.Contains(t, "{{") && orgCaptureEscapeRe.MatchString(t)
}

var promptNameRe = regexp.MustCompile(`[^a-z0-9]+`)

func promptName(p string) string {
	n := strings.Trim(promptNameRe.ReplaceAllString(strings.ToLower(p), "_"), "_")
	if n == "" {
		n = "answer"
	}
	return n
}

// safeDefault is text a {{name|prompt|=default}} can hold.
func safeDefault(s string) string { return strings.NewReplacer("}", "", "|", "/").Replace(s) }

// OrgCaptureToOrgs is an org capture template in orgs's grammar.
func OrgCaptureToOrgs(t string, now time.Time) string {
	var b strings.Builder
	prompts := []string{}
	props := []string{}
	hasInitial := strings.Contains(t, "%i")
	for i := 0; i < len(t); i++ {
		if t[i] != '%' || i+1 >= len(t) {
			b.WriteByte(t[i])
			continue
		}
		rest := t[i+1:]
		switch {
		case rest[0] == '%':
			b.WriteByte('%')
			i++
		case rest[0] == '?':
			b.WriteString("{{CONTENT}}")
			i++
		case rest[0] == 'i':
			if hasInitial {
				b.WriteString("{{initial|Initial text}}")
			}
			i++
		case rest[0] == '<':
			end := strings.IndexByte(rest, '>')
			if end < 0 {
				b.WriteByte('%')
				continue
			}
			b.WriteString(Strftime(rest[1:end], now))
			i += end + 1
		case rest[0] == '\\' && len(rest) > 1 && rest[1] >= '1' && rest[1] <= '9':
			n := int(rest[1] - '1')
			if n < len(prompts) {
				b.WriteString("{{" + prompts[n] + "}}")
			}
			i += 2
		case rest[0] == ':':
			j := 1
			for j < len(rest) && (rest[j] >= 'a' && rest[j] <= 'z' || rest[j] == '-' || rest[j] == '_') {
				j++
			}
			b.WriteString("{{" + rest[1:j] + "}}")
			i += j
		case rest[0] == '^' && len(rest) > 1 && rest[1] == '{':
			end := strings.IndexByte(rest, '}')
			if end < 0 {
				b.WriteByte('%')
				continue
			}
			inner := rest[2:end]
			parts := strings.Split(inner, "|")
			prompt := strings.TrimSpace(parts[0])
			i += end + 1
			// A letter after the brace changes what it asks for.
			kind := byte(0)
			if i+1 < len(t) && strings.IndexByte("tTuUpgG", t[i+1]) >= 0 {
				kind = t[i+1]
				i++
			}
			name := promptName(prompt)
			switch kind {
			case 'p':
				props = append(props, ":"+prompt+": {{"+name+"|"+prompt+"}}")
				continue
			case 't':
				b.WriteString("{{today}}")
				continue
			case 'T':
				b.WriteString("{{active_now}}")
				continue
			case 'u':
				b.WriteString("{{inactive_today}}")
				continue
			case 'U':
				b.WriteString("{{now}}")
				continue
			case 'g', 'G':
				b.WriteString("{{tags|" + prompt + "}}")
				continue
			}
			prompts = append(prompts, name)
			if len(parts) > 1 && strings.TrimSpace(parts[1]) != "" {
				b.WriteString("{{" + name + "|" + prompt + "|=" + safeDefault(strings.TrimSpace(parts[1])) + "}}")
			} else {
				b.WriteString("{{" + name + "|" + prompt + "}}")
			}
		case rest[0] == '^' && len(rest) > 1:
			switch rest[1] {
			case 't':
				b.WriteString("{{today}}")
			case 'T':
				b.WriteString("{{active_now}}")
			case 'u':
				b.WriteString("{{inactive_today}}")
			case 'U':
				b.WriteString("{{now}}")
			case 'g', 'G':
				b.WriteString("{{tags|Tags}}")
			case 'C':
				b.WriteString("{{clipboard|Clipboard}}")
			case 'L':
				b.WriteString("{{link|Link}}")
			default:
				b.WriteString("%^" + string(rest[1]))
			}
			i += 2
		default:
			switch rest[0] {
			case 't':
				b.WriteString("{{today}}")
			case 'T':
				b.WriteString("{{active_now}}")
			case 'u':
				b.WriteString("{{inactive_today}}")
			case 'U':
				b.WriteString("{{now}}")
			case 'a', 'A':
				b.WriteString("{{annotation}}")
			case 'l', 'L':
				b.WriteString("{{link}}")
			case 'c', 'x':
				b.WriteString("{{clipboard|Clipboard}}")
			case 'n':
				b.WriteString("{{username}}")
			case 'f', 'F', 'k', 'K':
				// The Emacs buffer capture was called from: nothing here.
			default:
				b.WriteByte('%')
				continue
			}
			i++
		}
	}
	out := b.String()
	if len(props) > 0 {
		// Under the headline, which org puts them in.
		lines := strings.Split(out, "\n")
		at := -1
		for k, l := range lines {
			if strings.HasPrefix(strings.TrimSpace(l), "*") {
				at = k
				break
			}
		}
		drawer := append(append([]string{":PROPERTIES:"}, props...), ":END:")
		if at < 0 {
			lines = append(drawer, lines...)
		} else {
			lines = append(lines[:at+1], append(drawer, lines[at+1:]...)...)
		}
		out = strings.Join(lines, "\n")
	}
	return out
}

// Strftime is the subset of format-time-string org templates use.
func Strftime(f string, t time.Time) string {
	r := strings.NewReplacer(
		"%Y", t.Format("2006"), "%y", t.Format("06"), "%m", t.Format("01"), "%d", t.Format("02"),
		"%e", t.Format("_2"), "%H", t.Format("15"), "%I", t.Format("03"), "%M", t.Format("04"),
		"%S", t.Format("05"), "%p", t.Format("PM"), "%a", t.Format("Mon"), "%A", t.Format("Monday"),
		"%b", t.Format("Jan"), "%B", t.Format("January"), "%Z", t.Format("MST"), "%z", t.Format("-0700"),
		"%j", t.Format("002"), "%F", t.Format("2006-01-02"), "%R", t.Format("15:04"), "%T", t.Format("15:04:05"),
		"%%", "%",
	)
	return r.Replace(f)
}
