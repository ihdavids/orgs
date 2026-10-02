package common

// Link protocols the user teaches orgs in the yaml: `jira:ABC-123` either turns
// into a real url (urlPrefix / url) or into a command line to run (cmd).
//
// The server does the resolving, because it holds the yaml and every client
// (the cli, worg, an editor) should agree on where a link goes. It hands back
// the command rather than running it: the command has to run on the machine of
// whoever is following the link, which is not necessarily the server.

import (
	"net/url"
	"regexp"
	"strings"
)

// One protocol's definition. Exactly one of UrlPrefix, Url and Cmd is expected;
// when several are given a url wins over a command, since a url can be followed
// by every client and a command only by the ones that can run things.
type LinkProtocol struct {
	// Replaces the protocol: with urlPrefix "https://jira.me.com/browse/",
	// `jira:ABC-1` and `jira://ABC-1` both become
	// "https://jira.me.com/browse/ABC-1".
	UrlPrefix string `yaml:"urlPrefix"`
	// A url with the link spliced in, for when the link is not at the end:
	// "https://tracker/{{path}}/view". Same placeholders as Cmd.
	Url string `yaml:"url"`
	// A command line to run. {{link}} is the whole link, protocol and all;
	// {{path}} is the part after the protocol. With neither placeholder the
	// link is added as the last argument: the path, or the whole link when
	// WithProtocol is set. Words split like a shell would split them (quotes
	// group), and a placeholder never splits a word, so a link with spaces or
	// shell characters in it is one argument and is never interpreted.
	Cmd          string `yaml:"cmd"`
	WithProtocol bool   `yaml:"withProtocol"`
}

// Where a link goes, once a protocol definition has been applied.
type LinkResolution struct {
	Ok       bool
	Msg      string
	Raw      string
	Protocol string
	// Set when the protocol maps to a url.
	Url string
	// Set when the protocol maps to a command: the arguments to exec, and the
	// same thing quoted as one line for a person or a shell.
	Command     []string
	CommandLine string
}

var linkProtoRe = regexp.MustCompile(`^([a-zA-Z][a-zA-Z0-9+.-]*):(.*)$`)

// SplitLinkProtocol pulls the protocol off a link, lower cased, and the path
// after it with any leading "//" dropped, so `jira:X` and `jira://X` agree.
func SplitLinkProtocol(raw string) (string, string) {
	m := linkProtoRe.FindStringSubmatch(strings.TrimSpace(raw))
	if m == nil {
		return "", raw
	}
	return strings.ToLower(m[1]), strings.TrimPrefix(m[2], "//")
}

// ResolveLinkProtocol applies the definition for this link's protocol, if the
// map has one. ok is false when the link has no protocol or nobody defined it.
func ResolveLinkProtocol(protos map[string]LinkProtocol, raw string) (LinkResolution, bool) {
	proto, path := SplitLinkProtocol(raw)
	if proto == "" {
		return LinkResolution{}, false
	}
	def, ok := lookupProtocol(protos, proto)
	if !ok {
		return LinkResolution{}, false
	}
	raw = strings.TrimSpace(raw)
	res := LinkResolution{Ok: true, Raw: raw, Protocol: proto}
	fill := func(s string) string {
		return strings.NewReplacer("{{link}}", raw, "{{path}}", path).Replace(s)
	}
	switch {
	case def.UrlPrefix != "":
		res.Url = def.UrlPrefix + path
	case def.Url != "":
		// The link is the variable part of somebody else's url here, so it is
		// escaped as a path would be - except for the slashes it may carry.
		res.Url = strings.NewReplacer(
			"{{link}}", url.PathEscape(raw),
			"{{path}}", strings.ReplaceAll(url.PathEscape(path), "%2F", "/"),
		).Replace(def.Url)
	case def.Cmd != "":
		words := splitWords(def.Cmd)
		placed := strings.Contains(def.Cmd, "{{link}}") || strings.Contains(def.Cmd, "{{path}}")
		for _, w := range words {
			res.Command = append(res.Command, fill(w))
		}
		if !placed {
			if def.WithProtocol {
				res.Command = append(res.Command, raw)
			} else {
				res.Command = append(res.Command, path)
			}
		}
		if len(res.Command) == 0 {
			return LinkResolution{Ok: false, Raw: raw, Protocol: proto, Msg: "the " + proto + " protocol has an empty cmd"}, true
		}
		res.CommandLine = ShellJoin(res.Command)
	default:
		return LinkResolution{Ok: false, Raw: raw, Protocol: proto, Msg: "the " + proto + " protocol has neither urlPrefix, url nor cmd"}, true
	}
	return res, true
}

// Protocol names in the yaml are matched without case, and a trailing ":" or
// "://" in the name is forgiven, since that is how people write them.
func lookupProtocol(protos map[string]LinkProtocol, proto string) (LinkProtocol, bool) {
	for k, v := range protos {
		name := strings.ToLower(strings.TrimSuffix(strings.TrimSuffix(strings.TrimSpace(k), "//"), ":"))
		if name == proto {
			return v, true
		}
	}
	return LinkProtocol{}, false
}

// splitWords splits a command line into words the way a shell would for the
// simple cases: whitespace separates, single and double quotes group, a
// backslash escapes the next character outside single quotes.
func splitWords(s string) []string {
	var out []string
	var cur strings.Builder
	have := false
	var quote rune
	esc := false
	for _, r := range s {
		switch {
		case esc:
			cur.WriteRune(r)
			esc = false
		case quote != 0:
			if r == quote {
				quote = 0
			} else if r == '\\' && quote == '"' {
				esc = true
			} else {
				cur.WriteRune(r)
			}
		case r == '\\':
			esc, have = true, true
		case r == '\'' || r == '"':
			quote, have = r, true
		case r == ' ' || r == '\t' || r == '\n':
			if have {
				out = append(out, cur.String())
				cur.Reset()
				have = false
			}
		default:
			cur.WriteRune(r)
			have = true
		}
	}
	if have {
		out = append(out, cur.String())
	}
	return out
}

var shellSafeRe = regexp.MustCompile(`^[a-zA-Z0-9_@%+=:,./-]+$`)

// ShellJoin quotes each argument for a POSIX shell and joins them.
func ShellJoin(args []string) string {
	q := make([]string, len(args))
	for i, a := range args {
		if shellSafeRe.MatchString(a) {
			q[i] = a
		} else {
			q[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
		}
	}
	return strings.Join(q, " ")
}
