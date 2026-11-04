// Package babelargs works out the header arguments a source block inherits,
// the way org does, for both halves that need them: running a block (the
// server's babel) and tangling one (plugs/tangle), which cannot import each
// other. Each caller parses the texts it is given with its own reader; what
// is shared is which texts, in which order.
//
// Org merges four layers key by key, each over the one before:
//
//  1. the defaults for every language (org-babel-default-header-args; here
//     babel.defaultHeaderArgs in the yaml);
//  2. the defaults for the block's language (babel.languageHeaderArgs);
//  3. the header-args property, then the header-args:<lang> property;
//  4. the block's #+HEADER: lines and its own line.
//
// A property is inherited as org-entry-get with inheritance has it: the
// nearest heading that sets it wins outright, and a name ending in + (on a
// heading, or a #+PROPERTY: line) adds to what is above instead. A file's
// #+PROPERTY: lines are the level above every heading, read in order, a
// #+SETUPFILE: read where it stands.
package babelargs

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Entry is one setting of header-args where it was found: the property's
// name as written (header-args, header-args:python+) and its value.
type Entry struct {
	Name  string
	Value string
}

// Lang is the language a name is for ("" for plain header-args), whether it
// adds (+), and whether it is a header-args name at all.
func Lang(name string) (lang string, plus bool, ok bool) {
	n := strings.ToLower(strings.TrimSpace(name))
	plus = strings.HasSuffix(n, "+")
	n = strings.TrimSuffix(n, "+")
	if n == "header-args" {
		return "", plus, true
	}
	if strings.HasPrefix(n, "header-args:") && len(n) > len("header-args:") {
		return n[len("header-args:"):], plus, true
	}
	return "", false, false
}

// Resolve is the value of header-args and of header-args:<lang> at a block,
// from levels ordered outermost first (the file's lines, then each heading
// down to the block's own). same says whether a language name is the
// block's language (sh and bash, say).
func Resolve(levels [][]Entry, same func(lang string) bool) (general, specific string) {
	set := func(cur *string, v string, plus bool) {
		v = strings.TrimSpace(v)
		if plus && *cur != "" {
			*cur += " " + v
		} else {
			*cur = v
		}
	}
	for _, level := range levels {
		for _, e := range level {
			l, plus, ok := Lang(e.Name)
			switch {
			case !ok:
			case l == "":
				set(&general, e.Value, plus)
			case same(l):
				set(&specific, e.Value, plus)
			}
		}
	}
	return general, specific
}

// Defaults are the yaml's two layers for a language: every language's, then
// the language's own (keys matched with same).
func Defaults(all string, byLang map[string]string, same func(lang string) bool) []string {
	out := []string{}
	if strings.TrimSpace(all) != "" {
		out = append(out, all)
	}
	for l, v := range byLang {
		if same(strings.ToLower(l)) && strings.TrimSpace(v) != "" {
			out = append(out, v)
		}
	}
	return out
}

var (
	propertyRe   = regexp.MustCompile(`(?i)^\s*#\+PROPERTY:\s+(\S+)\s*(.*)$`)
	setupRe      = regexp.MustCompile(`(?i)^\s*#\+SETUPFILE:\s*(.+?)\s*$`)
	blockBeginRe = regexp.MustCompile(`(?i)^\s*#\+begin_`)
	blockEndRe   = regexp.MustCompile(`(?i)^\s*#\+end_`)
)

// FileEntries are the header-args #+PROPERTY: lines of an org file's text, in
// order, with each #+SETUPFILE: read in where it stands (a local file,
// relative to the one naming it; a url is not fetched). Lines inside blocks
// are an example of a file, not settings.
func FileEntries(path, text string) []Entry {
	return fileEntries(path, text, map[string]bool{}, 0)
}

func fileEntries(path, text string, seen map[string]bool, depth int) []Entry {
	out := []Entry{}
	if abs, err := filepath.Abs(path); err == nil {
		seen[abs] = true
	}
	inBlock := false
	for _, l := range strings.Split(text, "\n") {
		if inBlock {
			inBlock = !blockEndRe.MatchString(l)
			continue
		}
		if blockBeginRe.MatchString(l) {
			inBlock = true
			continue
		}
		if m := propertyRe.FindStringSubmatch(l); m != nil {
			if _, _, ok := Lang(m[1]); ok {
				out = append(out, Entry{m[1], m[2]})
			}
			continue
		}
		if m := setupRe.FindStringSubmatch(l); m != nil && depth < 5 {
			name := strings.Trim(m[1], `"`)
			if strings.Contains(name, "://") {
				continue
			}
			if strings.HasPrefix(name, "~/") {
				if home, err := os.UserHomeDir(); err == nil {
					name = filepath.Join(home, name[2:])
				}
			}
			if !filepath.IsAbs(name) {
				name = filepath.Join(filepath.Dir(path), name)
			}
			abs, err := filepath.Abs(name)
			if err != nil || seen[abs] {
				continue
			}
			if b, err := os.ReadFile(abs); err == nil {
				out = append(out, fileEntries(abs, string(b), seen, depth+1)...)
			}
		}
	}
	return out
}

var octalRe = regexp.MustCompile(`#o([0-7]+)`)

// ParseMode reads a :tangle-mode or :file-mode value: org's (identity #o755),
// #o755, or plain octal (0755, 755).
func ParseMode(s string) (os.FileMode, bool) {
	s = strings.TrimSpace(s)
	if m := octalRe.FindStringSubmatch(s); m != nil {
		s = m[1]
	}
	s = strings.TrimPrefix(s, "0")
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseUint(s, 8, 32)
	if err != nil {
		return 0, false
	}
	return os.FileMode(v), true
}
