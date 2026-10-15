package mdorg

import (
	"fmt"
	"path"
	"regexp"
	"strings"
)

var (
	codeSpanRe = regexp.MustCompile("(`+)(.+?)(`+)")
	mathRe     = regexp.MustCompile(`\$([^\s$](?:[^$]*?[^\s$])?)\$`)
	// [[target#part|alias]], ! for an embed.
	wikiRe    = regexp.MustCompile(`(!?)\[\[([^\]|#]*)(#[^\]|]*)?(?:\|([^\]]*))?\]\]`)
	mdLinkRe  = regexp.MustCompile(`(!?)\[((?:[^\[\]]|\[[^\]]*\])*)\]\(\s*<?([^)\s>]*(?:\([^)\s]*\)[^)\s>]*)*)>?(?:\s+["'][^"']*["'])?\s*\)`)
	refLinkRe = regexp.MustCompile(`(!?)\[([^\]]+)\]\[([^\]]*)\]`)
	autoRe    = regexp.MustCompile(`<((?:https?|ftp|mailto):[^>\s]+|[^@>\s]+@[^@>\s]+\.[^@>\s]+)>`)
	escRe     = regexp.MustCompile(`\\([\\` + "`" + `*_{}\[\]()#+\-.!|~=$<>])`)
	boldRe    = regexp.MustCompile(`(\*\*|__)([^\s*_](?:.*?[^\s*_])?)(\*\*|__)`)
	italStar  = regexp.MustCompile(`(^|[^\p{L}\p{N}*\\])\*([^\s*](?:[^*]*?[^\s*])?)\*($|[^\p{L}\p{N}*])`)
	italUnder = regexp.MustCompile(`(^|[^\p{L}\p{N}_\\])_([^\s_](?:[^_]*?[^\s_])?)_($|[^\p{L}\p{N}_])`)
	strikeRe  = regexp.MustCompile(`~~([^\s~](?:.*?[^\s~])?)~~`)
	markRe    = regexp.MustCompile(`==([^\s=](?:.*?[^\s=])?)==`)
	htmlTagRe = regexp.MustCompile(`(?i)<(/?)(u|mark|sup|sub|kbd|b|strong|i|em|s|del|br)\s*/?>`)
	inlineTag = regexp.MustCompile(`(^|\s)#([\p{L}_][\p{L}\p{N}_/-]*)`)
	fnRefRe   = regexp.MustCompile(`\[\^([^\]]+)\]`)
)

var imageExt = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".svg": true,
	".webp": true, ".bmp": true, ".avif": true, ".tif": true, ".tiff": true}

func isImage(p string) bool { return imageExt[strings.ToLower(path.Ext(p))] }

// protector swaps finished org for placeholders, so what is already
// converted (a link, a code span) is not converted again by the emphasis
// rules that come after it.
type protector struct{ saved []string }

func (p *protector) keep(s string) string {
	p.saved = append(p.saved, s)
	return fmt.Sprintf("\x00%d\x00", len(p.saved)-1)
}

var placeRe = regexp.MustCompile("\x00(\\d+)\x00")

func (p *protector) restore(s string) string {
	for i := 0; i < 3 && strings.Contains(s, "\x00"); i++ {
		s = placeRe.ReplaceAllStringFunc(s, func(m string) string {
			var n int
			fmt.Sscan(m[1:len(m)-1], &n)
			if n < len(p.saved) {
				return p.saved[n]
			}
			return m
		})
	}
	return s
}

// verbatim writes a code span in org's ~code~, or =code= when it holds a
// tilde; with both it is left as the text it was.
func verbatim(s string) string {
	switch {
	case !strings.Contains(s, "~"):
		return "~" + s + "~"
	case !strings.Contains(s, "="):
		return "=" + s + "="
	}
	return s
}

// inline converts a line of markdown text to org.
func (c *conv) inline(s string) string {
	p := &protector{}
	// Code first: nothing inside it is markup.
	s = codeSpanRe.ReplaceAllStringFunc(s, func(m string) string {
		sm := codeSpanRe.FindStringSubmatch(m)
		if sm[1] != sm[3] {
			return m
		}
		return p.keep(verbatim(strings.TrimSpace(sm[2])))
	})
	s = escRe.ReplaceAllStringFunc(s, func(m string) string { return p.keep(m[1:]) })
	s = mathRe.ReplaceAllStringFunc(s, func(m string) string {
		return p.keep(`\(` + m[1:len(m)-1] + `\)`)
	})
	s = wikiRe.ReplaceAllStringFunc(s, func(m string) string {
		sm := wikiRe.FindStringSubmatch(m)
		return p.keep(c.wikiLink(sm[1] == "!", sm[2], strings.TrimPrefix(sm[3], "#"), sm[4]))
	})
	s = mdLinkRe.ReplaceAllStringFunc(s, func(m string) string {
		sm := mdLinkRe.FindStringSubmatch(m)
		return p.keep(c.mdLink(sm[1] == "!", c.inline(sm[2]), sm[3]))
	})
	s = refLinkRe.ReplaceAllStringFunc(s, func(m string) string {
		sm := refLinkRe.FindStringSubmatch(m)
		ref := sm[3]
		if ref == "" {
			ref = sm[2]
		}
		if u, ok := c.refs[strings.ToLower(ref)]; ok {
			return p.keep(c.mdLink(sm[1] == "!", c.inline(sm[2]), u))
		}
		return m
	})
	s = autoRe.ReplaceAllStringFunc(s, func(m string) string {
		u := m[1 : len(m)-1]
		if !strings.Contains(u, ":") {
			u = "mailto:" + u
		}
		c.stats.Links++
		return p.keep("[[" + u + "]]")
	})
	s = fnRefRe.ReplaceAllStringFunc(s, func(m string) string {
		return p.keep("[fn:" + fnName(m[2:len(m)-1]) + "]")
	})
	for _, m := range inlineTag.FindAllStringSubmatch(s, -1) {
		if t := OrgTag(m[2]); t != "" {
			c.tags[t] = true
		}
	}
	s = htmlTagRe.ReplaceAllStringFunc(s, func(m string) string {
		sm := htmlTagRe.FindStringSubmatch(m)
		switch strings.ToLower(sm[2]) {
		case "br":
			return " "
		case "u":
			return "_"
		case "mark", "b", "strong":
			return "*"
		case "i", "em":
			return "/"
		case "s", "del":
			return "+"
		case "kbd":
			return "~"
		case "sup":
			if sm[1] == "" {
				return "^{"
			}
			return "}"
		case "sub":
			if sm[1] == "" {
				return "_{"
			}
			return "}"
		}
		return m
	})
	// Bold is held back in placeholders so the italic rules do not read its
	// stars as italics.
	s = boldRe.ReplaceAllStringFunc(s, func(m string) string {
		sm := boldRe.FindStringSubmatch(m)
		if sm[1] != sm[3] {
			return m
		}
		return p.keep("*" + p.restore(sm[2]) + "*")
	})
	s = markRe.ReplaceAllStringFunc(s, func(m string) string {
		return p.keep("*" + m[2:len(m)-2] + "*")
	})
	s = strikeRe.ReplaceAllString(s, "+$1+")
	for i := 0; i < 2; i++ {
		s = italStar.ReplaceAllString(s, "$1/$2/$3")
		s = italUnder.ReplaceAllString(s, "$1/$2/$3")
	}
	return p.restore(s)
}

// noteLink is an org file link from this note to another, with a search
// part for a heading or a block.
func (c *conv) noteLink(note, part string) string {
	rel := relLink(OrgPath(c.from), OrgPath(note))
	if note == c.from {
		rel = ""
	}
	search := ""
	if part != "" {
		if strings.HasPrefix(part, "^") {
			search = part[1:]
		} else {
			search = "*" + c.ix.heading(note, part)
		}
	}
	switch {
	case rel == "" && search == "":
		return "file:" + relLink(OrgPath(c.from), OrgPath(c.from))
	case rel == "" && strings.HasPrefix(search, "*"):
		return search
	case rel == "":
		return search
	case search == "":
		return "file:" + rel
	}
	return "file:" + rel + "::" + search
}

// fileLink is a link to one of the vault's files, which is copied beside the
// org files at the same place.
func (c *conv) fileLink(f string) string {
	c.assets[f] = true
	return "file:" + relLink(OrgPath(c.from), f)
}

func linkText(s string) string {
	return strings.NewReplacer("[", "(", "]", ")").Replace(s)
}

// wikiLink converts [[target#part|alias]] and its ![[embed]] form.
func (c *conv) wikiLink(embed bool, target, part, alias string) string {
	target = strings.TrimSpace(target)
	desc := strings.TrimSpace(alias)
	if embed {
		c.stats.Embeds++
	} else {
		c.stats.Links++
	}
	// An embedded picture's |300 is a size, not a description.
	if embed && isImage(target) {
		if f, ok := c.ix.file(target, c.from); ok {
			return "[[" + c.fileLink(f) + "]]"
		}
		c.stats.Unresolved++
		return "[[file:" + target + "]]"
	}
	if desc == "" {
		desc = target
		if part != "" {
			name := strings.TrimPrefix(part, "^")
			if desc == "" {
				desc = name
			} else {
				desc += " > " + name
			}
		}
	}
	if note, ok := c.ix.note(target, c.from); ok && (target == "" || isNote(target) || path.Ext(target) == "" || c.ix.byName[strings.ToLower(path.Base(target))] != nil) {
		return "[[" + c.noteLink(note, part) + "][" + linkText(desc) + "]]"
	}
	if f, ok := c.ix.file(target, c.from); ok {
		return "[[" + c.fileLink(f) + "][" + linkText(desc) + "]]"
	}
	// A note not written yet: where it would be, beside this one.
	c.stats.Unresolved++
	guess := path.Join(path.Dir(c.from), target)
	if path.Ext(guess) == "" {
		guess += ".md"
	}
	return "[[file:" + relLink(OrgPath(c.from), OrgPath(guess)) + "][" + linkText(desc) + "]]"
}

// embedBlock is an embed on a line of its own: a picture (with its size), or
// a note brought in with #+INCLUDE:, org's own way of showing one file in
// another.
func (c *conv) embedBlock(inner string) (string, bool) {
	target, alias := inner, ""
	if i := strings.Index(inner, "|"); i >= 0 {
		target, alias = inner[:i], inner[i+1:]
	}
	part := ""
	if i := strings.Index(target, "#"); i >= 0 {
		target, part = target[:i], target[i+1:]
	}
	target = strings.TrimSpace(target)
	if isImage(target) {
		c.stats.Embeds++
		link := ""
		if f, ok := c.ix.file(target, c.from); ok {
			link = "[[" + c.fileLink(f) + "]]"
		} else {
			c.stats.Unresolved++
			link = "[[file:" + target + "]]"
		}
		if w := strings.TrimSpace(alias); w != "" {
			if wh := strings.SplitN(w, "x", 2); isNumber(wh[0]) {
				return "#+attr_html: :width " + wh[0] + "px\n" + link, true
			}
			return "#+CAPTION: " + w + "\n" + link, true
		}
		return link, true
	}
	note, ok := c.ix.note(target, c.from)
	if !ok || note == c.from {
		return "", false
	}
	c.stats.Embeds++
	rel := relLink(OrgPath(c.from), OrgPath(note))
	if part != "" && !strings.HasPrefix(part, "^") {
		return fmt.Sprintf(`#+INCLUDE: "%s::*%s"`, rel, c.ix.heading(note, part)), true
	}
	if strings.HasPrefix(part, "^") {
		return "", false
	}
	return fmt.Sprintf(`#+INCLUDE: "%s"`, rel), true
}

func isNumber(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// mdLink converts [text](url) and ![alt](picture).
func (c *conv) mdLink(image bool, text, u string) string {
	u = strings.TrimSpace(u)
	if image {
		c.stats.Embeds++
	} else {
		c.stats.Links++
	}
	if strings.Contains(u, "://") || strings.HasPrefix(u, "mailto:") || strings.HasPrefix(u, "tel:") {
		if image {
			return "[[" + u + "]]"
		}
		if text == "" || text == u {
			return "[[" + u + "]]"
		}
		return "[[" + u + "][" + linkText(text) + "]]"
	}
	// Obsidian's own links between notes.
	if strings.HasPrefix(u, "obsidian://") {
		return "[[" + u + "][" + linkText(text) + "]]"
	}
	target, part := u, ""
	if i := strings.Index(u, "#"); i >= 0 {
		target, part = u[:i], u[i+1:]
	}
	if image || isImage(target) {
		if f, ok := c.ix.file(target, c.from); ok {
			link := c.fileLink(f)
			if image {
				return "[[" + link + "]]"
			}
			return "[[" + link + "][" + linkText(text) + "]]"
		}
		c.stats.Unresolved++
		return "[[file:" + target + "]]"
	}
	if target == "" || isNote(target) || path.Ext(target) == "" {
		if note, ok := c.ix.note(target, c.from); ok {
			return "[[" + c.noteLink(note, part) + "][" + linkText(text) + "]]"
		}
	}
	if f, ok := c.ix.file(target, c.from); ok {
		return "[[" + c.fileLink(f) + "][" + linkText(text) + "]]"
	}
	c.stats.Unresolved++
	t := target
	if isNote(t) {
		t = OrgPath(t)
	}
	return "[[file:" + t + "][" + linkText(text) + "]]"
}
