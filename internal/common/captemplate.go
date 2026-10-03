package common

// The capture template language - the whole grammar, both halves of it.
//
// A capture template carries a `template:` string which is a *form*: the shape
// of the entry, with a hole at every place somebody has to say something. The
// server does not file that string, it hands it over; what fills the holes in
// is a client with a person in front of it.
//
// The grammar is four spellings of one thing:
//
//	{{name}}                   a value to fill in, labelled from the name
//	{{name|prompt}}            the same, asked for in your own words
//	{{name|=default}}          pre-filled with a default, still editable
//	{{name|prompt|=default}}   both
//	{{CONTENT}}                the body - one big box rather than a line
//
// The `|=` half is written by the **server**, not by the person writing the
// template. A handful of names answer themselves - `{{uuid}}` is a number out
// of a hat, `{{username}}` is who asked, `{{now}}` is what time it is on the
// machine holding the org files - and those are expanded on the way out of
// `/capture/templates`, in place, so that:
//
//   - a client that does nothing about it has a sensible value already in the
//     box, which is what makes `:CUSTOM_ID: {{uuid}}` work at all; and
//   - a client that wants to do its own thing still knows the name, so it can
//     ignore the default and answer `{{uuid}}` however it likes.
//
// Which is why the expansion is a *default* rather than a substitution: a value
// written into the template as literal text is a value nobody can change, and
// a captured timestamp is very often the one thing somebody wants to change -
// a thing captured today did not necessarily happen today.
//
// It is also why the marker is `|=` rather than the prompt slot on its own.
// `{{source|Where did this come from?}}` is a question and
// `{{uuid|=f81d4fae-...}}` is an answer, and a client has to be able to tell
// them apart without keeping its own copy of the list of names - which is the
// kind of second copy that goes stale (see ClientHash, FuzzyScore, HealthLevel).
//
// Everything here is shared by both ends on purpose. The server expands, the
// client reads the fields and fills the template back in, and neither has a
// private understanding of the grammar.

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/ihdavids/orgs/internal/templates"
)

// {{name}}, {{name|prompt}}, {{name|=default}}, {{name|prompt|=default}}.
//
// The name may not contain a pipe, so everything after the first one is the
// rest and is taken apart by capSplitRest.
var capPlaceholder = regexp.MustCompile(`\{\{\s*([^}|]+?)\s*(?:\|([^}]*?))?\}\}`)

// A line that was nothing but a placeholder, now that the placeholder is empty:
// a blank line, or a property with nothing after its name.
var capEmptyProp = regexp.MustCompile(`^\s*:[^:\s]+:\s*$`)

// CapField is one thing to fill in before the capture can be made.
type CapField struct {
	// What the template wrote between the braces, exactly: this is what gets
	// substituted back.
	Key string
	// What to call it on screen.
	Prompt string
	// What it starts as. Empty for a value somebody has to give.
	Default string
	// Whether the default came from a function rather than from nothing. Only
	// used to say so on screen - a default is still editable, because a thing
	// captured today did not necessarily happen today.
	Auto bool
	// The body: one big box, and where a dictation or a pasted picture lands.
	// Everything else is a line.
	Content bool
}

// CapCanonName is the name a value is remembered under while one template is
// being expanded. Names are matched trimmed and case folded, so {{ NOW }} and
// {{now}} are the same name. The names and their aliases are shared with file
// templates, in internal/templates/values.go.
func CapCanonName(name string) string {
	return templates.CanonName(name)
}

// IsCapContentKey reports whether this is the body rather than a line.
func IsCapContentKey(key string) bool {
	return strings.EqualFold(strings.TrimSpace(key), "content")
}

// CapAutoValue is the value for one placeholder name, and whether there is one
// at all. The false return is load-bearing: it means "leave this for somebody
// to answer", not "answer it with nothing".
func CapAutoValue(name string, now time.Time, username string) (string, bool) {
	switch k := CapCanonName(name); k {
	// Where the current day page lives, relative to the first org directory.
	// Only the server knows where that is, so it is answered through a hook the
	// server fills in; with no hook it is left for somebody to answer.
	case "daypage":
		if CapDayPage == nil {
			return "", false
		}
		return CapDayPage(now)
	// Whoever asked. Empty where there is no request to read a username off,
	// and then left for somebody to answer rather than filled blank.
	case "username":
		if username == "" {
			return "", false
		}
		return username, true
	default:
		// The clock, the ids and the machine: the names every template
		// answers, capture or file.
		return templates.AutoValue(k, now)
	}
}

// IsCapAutoName reports whether this name answers itself. It asks CapAutoValue
// rather than keeping a list beside it, so the two can never disagree about
// what the names are.
func IsCapAutoName(name string) bool {
	// A username there is nobody to answer is still an auto name - the question
	// is what kind of placeholder this is, not whether today's answer exists.
	if k := CapCanonName(name); k == "username" || k == "daypage" {
		return true
	}
	_, ok := CapAutoValue(name, time.Now(), "")
	return ok
}

// CapDayPage answers {{daypage}}: the day page in force at that moment, as a
// path relative to the first org directory. It is set by the server, which is
// the end that knows dayPagePath and the day page mode.
var CapDayPage func(now time.Time) (string, bool)

// CapUsesName reports whether a template has a placeholder of that name,
// aliases counted as the name.
func CapUsesName(template, name string) bool {
	want := CapCanonName(name)
	for _, m := range capPlaceholder.FindAllStringSubmatch(template, -1) {
		if CapCanonName(m[1]) == want {
			return true
		}
	}
	return false
}

// CapExpandFilename answers every placeholder in a capture target's filename.
//
// There is nobody to ask at this point - the filename is resolved when the
// capture is made, not put up as a form - so every placeholder must answer
// itself or carry a default, and one that does neither is an error rather than
// a file called {{project}}.org.
//
// Each value is made safe for a filename on the way in: brackets go, spaces
// become _ and colons become -, so {{now}} is 2026-09-29_Tue_14-05 rather than
// [2026-09-29 Tue 14:05]. {{daypage}} is the exception, because it is a path
// and its separators are the point of it.
func CapExpandFilename(filename string, now time.Time, username string) (string, error) {
	if !strings.Contains(filename, "{{") {
		return filename, nil
	}
	var missing []string
	memo := map[string]string{}
	out := capPlaceholder.ReplaceAllStringFunc(filename, func(m string) string {
		sub := capPlaceholder.FindStringSubmatch(m)
		if sub == nil {
			return m
		}
		name := strings.TrimSpace(sub[1])
		key := CapCanonName(name)
		if v, ok := memo[key]; ok {
			return v
		}
		v, ok := CapAutoValue(name, now, username)
		if !ok {
			_, def, hasDef := capSplitRest(sub[2])
			if !hasDef || def == "" {
				missing = append(missing, name)
				return m
			}
			v = def
		}
		if key != "daypage" {
			v = CapFileSafe(v)
		}
		memo[key] = v
		return v
	})
	if len(missing) > 0 {
		return "", fmt.Errorf("capture filename %q: no value for {{%s}}", filename, strings.Join(missing, "}}, {{"))
	}
	return out, nil
}

// CapFileSafe is a value made fit to be part of a filename: org's timestamp
// brackets dropped, spaces to _, and anything a filesystem objects to to -.
func CapFileSafe(v string) string {
	v = strings.NewReplacer("[", "", "]", "", "<", "", ">", "").Replace(strings.TrimSpace(v))
	return strings.NewReplacer(" ", "_", ":", "-", "/", "-", "\\", "-", "*", "-", "?", "-", "\"", "-", "|", "-").Replace(v)
}

// capSplitRest takes everything after the name apart: the prompt somebody
// wrote, and the default value the server filled in.
func capSplitRest(rest string) (prompt string, def string, hasDef bool) {
	if i := strings.LastIndex(rest, "|="); i >= 0 {
		return strings.TrimSpace(rest[:i]), strings.TrimSpace(rest[i+2:]), true
	}
	if t := strings.TrimSpace(rest); strings.HasPrefix(t, "=") {
		return "", strings.TrimSpace(t[1:]), true
	}
	return strings.TrimSpace(rest), "", false
}

// capLabelFor turns a name into a label: "dueDate" and "due_date" both read as
// "Due date", and the first letter goes up because it starts a line on screen.
func capLabelFor(name string) string {
	words := strings.TrimSpace(capCamel.ReplaceAllString(strings.NewReplacer("_", " ", "-", " ").Replace(name), "$1 $2"))
	if words == "" {
		return name
	}
	return strings.ToUpper(words[:1]) + strings.ToLower(words[1:])
}

var capCamel = regexp.MustCompile(`([a-z0-9])([A-Z])`)

// CapExpandAutos rewrites every placeholder whose name answers itself so that
// it carries the answer as its default, and leaves everything else exactly as
// written - a name with no value keeps its braces, and so does a question.
//
// Three rules:
//
//  1. **It is idempotent.** A placeholder that already carries a `|=` default
//     is left alone, so expanding an expanded template changes nothing and a
//     template cannot collect a second answer.
//  2. **One value per name per template**, aliases counted as one name, so
//     `{{uuid}}` twice in one template is one uuid and a `:CUSTOM_ID:` plus an
//     `id:` link to it agree.
//  3. **A value that could not be written back out is not written.** A default
//     holding `}` or `|` would be a placeholder that no longer parses, so the
//     placeholder is left bare instead and the client asks.
func CapExpandAutos(template string, now time.Time, username string) string {
	if !strings.Contains(template, "{{") {
		return template
	}
	memo := map[string]string{}
	return capPlaceholder.ReplaceAllStringFunc(template, func(m string) string {
		sub := capPlaceholder.FindStringSubmatch(m)
		if sub == nil {
			return m
		}
		name := strings.TrimSpace(sub[1])
		prompt, _, hasDef := capSplitRest(sub[2])
		if hasDef {
			return m
		}
		key := CapCanonName(name)
		v, ok := memo[key]
		if !ok {
			if v, ok = CapAutoValue(name, now, username); !ok {
				return m
			}
			memo[key] = v
		}
		if strings.ContainsAny(v, "}|") {
			return m
		}
		if prompt != "" {
			return "{{" + name + "|" + prompt + "|=" + v + "}}"
		}
		return "{{" + name + "|=" + v + "}}"
	})
}

// CapSubstitute walks every placeholder and asks what to put there. A false
// second return leaves the placeholder exactly as it was written, braces and
// all - which is what an editor wants for a hole nobody has filled in yet, and
// what FillCapTemplate does not do (it writes an unanswered hole as nothing and
// drops the line).
//
// It is here rather than in the client that needs it so that the pattern itself
// is in one place: a second regular expression for the same grammar is the kind
// of copy that goes a comma out of step and takes a template apart differently
// at each end.
func CapSubstitute(template string, what func(key string) (string, bool)) string {
	if !strings.Contains(template, "{{") {
		return template
	}
	return capPlaceholder.ReplaceAllStringFunc(template, func(m string) string {
		sub := capPlaceholder.FindStringSubmatch(m)
		if sub == nil {
			return m
		}
		if v, ok := what(strings.TrimSpace(sub[1])); ok {
			return v
		}
		return m
	})
}

// CapFields is what a template asks for, in the order it asks, each thing once.
//
// A template with nothing in it still asks for a body: that is what every
// template in the shipped example config is, and "no template string" has
// always meant "a headline and whatever you type", not "capture nothing".
func CapFields(template string) []CapField {
	if strings.TrimSpace(template) == "" {
		return []CapField{{Key: "CONTENT", Prompt: "Content", Content: true}}
	}
	out := []CapField{}
	seen := map[string]bool{}
	for _, m := range capPlaceholder.FindAllStringSubmatch(template, -1) {
		name := strings.TrimSpace(m[1])
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		prompt, def, _ := capSplitRest(m[2])
		content := IsCapContentKey(name)
		if prompt == "" {
			if content {
				prompt = "Content"
			} else {
				prompt = capLabelFor(name)
			}
		}
		out = append(out, CapField{
			Key:     name,
			Prompt:  prompt,
			Default: def,
			Auto:    IsCapAutoName(name),
			Content: content,
		})
	}
	return out
}

// FillCapTemplate is the template with the answers in it.
//
// A placeholder nobody answered is written as nothing rather than left standing
// - `{{project}}` in a captured file is a capture that did not happen properly
// - and a line that was *only* that placeholder goes with it. A template
// offering three properties where somebody filled in one should write one
// property, not one property and two empty ones.
//
// A line that had no placeholder on it is never touched, however empty it
// looks, because that is a line the template meant to write.
func FillCapTemplate(template string, values map[string]string) string {
	if strings.TrimSpace(template) == "" {
		return values["CONTENT"]
	}
	valueFor := func(raw string) string {
		key := strings.TrimSpace(raw)
		if v, ok := values[key]; ok {
			return v
		}
		// A key nobody was asked for - the same name written two ways, say.
		for k, v := range values {
			if strings.EqualFold(k, key) {
				return v
			}
		}
		return ""
	}
	out := []string{}
	for _, line := range strings.Split(template, "\n") {
		had := capPlaceholder.MatchString(line)
		filled := capPlaceholder.ReplaceAllStringFunc(line, func(m string) string {
			sub := capPlaceholder.FindStringSubmatch(m)
			if sub == nil {
				return m
			}
			return valueFor(sub[1])
		})
		if had && (strings.TrimSpace(filled) == "" || capEmptyProp.MatchString(filled)) {
			continue
		}
		out = append(out, filled)
	}
	return strings.Join(out, "\n")
}
