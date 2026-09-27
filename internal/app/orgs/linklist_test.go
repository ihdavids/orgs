package orgs

import "testing"

// A service is the name a person would use for where a link goes. The list
// cannot be complete, so what matters most is the fallback: a host nobody has
// heard of still has to land in a group of its own rather than in no group.
func TestServiceOf(t *testing.T) {
	cases := []struct{ host, want string }{
		{"github.com", "GitHub"},
		{"www.github.com", "GitHub"},
		{"gist.github.com", "GitHub"},
		{"raw.githubusercontent.com", "GitHub"},
		{"mycompany.atlassian.net", "Jira"},
		{"docs.google.com", "Google Docs"},
		{"mail.google.com", "Gmail"},
		// A google host with no entry of its own still says Google rather than
		// falling through to the domain rule and saying the same thing by luck.
		{"translate.google.com", "Google"},
		{"youtu.be", "YouTube"},
		{"news.ycombinator.com", "Hacker News"},
		// Nobody's list has these on it.
		{"example.com", "Example"},
		{"wiki.internal.corp", "Internal"},
		{"localhost", "Localhost"},
		{"", ""},
	}
	for _, c := range cases {
		if got := serviceOf(c.host); got != c.want {
			t.Errorf("serviceOf(%q) = %q, want %q", c.host, got, c.want)
		}
	}
}

func TestLinkSchemeHost(t *testing.T) {
	cases := []struct{ raw, scheme, host string }{
		{"https://github.com/ihdavids/orgs/issues/4", "https", "github.com"},
		{"http://Example.COM/a?b=c#d", "http", "example.com"},
		{"https://user:pw@example.com:8443/x", "https", "example.com"},
		{"mailto:jane.roe@example.org", "mailto", "example.org"},
		{"doi:10.1000/182", "doi", ""},
		// Not a link out of the org files: no scheme, no host, and it must not
		// invent one. `notes.org` reads exactly like a protocol to the parser,
		// which is the reason the check is against the resolver's own list
		// rather than against the shape of the text.
		{"notes.org::*Plans", "", ""},
		{"id:07e67723-5c8b-444d", "", ""},
	}
	for _, c := range cases {
		s, h := linkSchemeHost(c.raw)
		if s != c.scheme || h != c.host {
			t.Errorf("linkSchemeHost(%q) = %q/%q, want %q/%q", c.raw, s, h, c.scheme, c.host)
		}
	}
}
