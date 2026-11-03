package common

import (
	"net/url"
	"testing"
)

func TestParseOrgProtocol(t *testing.T) {
	cases := []struct {
		in   string
		want OrgProtocol
	}{
		{"org-protocol://capture?template=w&url=https%3A%2F%2Fex.com%2Fa%3Fb%3D1&title=A%20page&body=some%20text",
			OrgProtocol{Kind: "capture", Template: "w", Url: "https://ex.com/a?b=1", Title: "A page", Body: "some text"}},
		// One slash, as some systems hand the link over.
		{"org-protocol:/capture?url=https%3A%2F%2Fex.com&title=T",
			OrgProtocol{Kind: "capture", Url: "https://ex.com", Title: "T"}},
		{"org-protocol://capture/?template=Todo&url=u",
			OrgProtocol{Kind: "capture", Template: "Todo", Url: "u"}},
		// The old spelling, with a template key...
		{"org-protocol://capture:/w/https%3A%2F%2Fex.com%2Fa/A%20page/the%20body",
			OrgProtocol{Kind: "capture", Template: "w", Url: "https://ex.com/a", Title: "A page", Body: "the body"}},
		// ...and without one: a first part longer than a character is the url.
		{"org-protocol://capture:/https%3A%2F%2Fex.com/Title/Body",
			OrgProtocol{Kind: "capture", Url: "https://ex.com", Title: "Title", Body: "Body"}},
		// An unescaped slash in the last part stays in it.
		{"org-protocol://capture:/w/u/t/a/b",
			OrgProtocol{Kind: "capture", Template: "w", Url: "u", Title: "t", Body: "a/b"}},
		{"org-protocol://store-link?url=https%3A%2F%2Fex.com&title=Ex",
			OrgProtocol{Kind: "store-link", Url: "https://ex.com", Title: "Ex"}},
		{"org-protocol://store-link:/https%3A%2F%2Fex.com/Ex",
			OrgProtocol{Kind: "store-link", Url: "https://ex.com", Title: "Ex"}},
		{"ORG-PROTOCOL://Open-Source?url=https%3A%2F%2Fex.com%2Fnotes%2Fa.html",
			OrgProtocol{Kind: "open-source", Url: "https://ex.com/notes/a.html"}},
		{"org-protocol://open-source:/https%3A%2F%2Fex.com%2Fnotes%2F",
			OrgProtocol{Kind: "open-source", Url: "https://ex.com/notes/"}},
		{"org-protocol://capture", OrgProtocol{Kind: "capture"}},
	}
	for _, c := range cases {
		got, err := ParseOrgProtocol(c.in)
		if err != nil {
			t.Errorf("%s: %v", c.in, err)
			continue
		}
		if got.Kind != c.want.Kind || got.Template != c.want.Template || got.Url != c.want.Url ||
			got.Title != c.want.Title || got.Body != c.want.Body {
			t.Errorf("%s:\n got %+v\nwant %+v", c.in, got, c.want)
		}
	}
}

func TestParseOrgProtocolRefuses(t *testing.T) {
	for _, in := range []string{
		"https://ex.com",
		"org-protocol://roam-ref?url=x",
		"org-protocol://store-link?title=no+url",
		"org-protocol://open-source",
	} {
		if _, err := ParseOrgProtocol(in); err == nil {
			t.Errorf("%s: wanted a refusal", in)
		}
	}
}

func TestOrgProtocolWorgPath(t *testing.T) {
	p, err := ParseOrgProtocol("org-protocol://capture?template=w&url=u&title=a%26b&source=mail&protocol=evil")
	if err != nil {
		t.Fatal(err)
	}
	path := p.WorgPath()
	q, err := url.ParseQuery(path[2:])
	if err != nil || path[:2] != "/?" {
		t.Fatalf("%s: %v", path, err)
	}
	if q.Get("protocol") != "capture" || q.Get("template") != "w" || q.Get("url") != "u" || q.Get("title") != "a&b" ||
		q.Get("x-source") != "mail" || q.Get("x-protocol") != "evil" {
		t.Errorf("worg path %s", path)
	}
}

func TestOrgLinkText(t *testing.T) {
	cases := [][3]string{
		{"https://ex.com", "Ex", "[[https://ex.com][Ex]]"},
		{"https://ex.com", "", "[[https://ex.com]]"},
		{"https://ex.com", "https://ex.com", "[[https://ex.com]]"},
		{"https://ex.com/[a]", "A [b]\n c", "[[https://ex.com/%5Ba%5D][A {b} c]]"},
	}
	for _, c := range cases {
		if got := OrgLinkText(c[0], c[1]); got != c[2] {
			t.Errorf("OrgLinkText(%q, %q) = %q, want %q", c[0], c[1], got, c[2])
		}
	}
}

func TestProtocolSourceRel(t *testing.T) {
	pr := ProtocolProject{BaseUrl: "https://ex.com/notes/", WorkingDirectory: "/n", OnlineSuffix: ".html", WorkingSuffix: ".org"}
	cases := []struct {
		in, want string
		ok       bool
	}{
		{"https://ex.com/notes/a/b.html", "a/b.org", true},
		{"https://ex.com/notes/a/b.html?x=1#top", "a/b.org", true},
		{"https://ex.com/notes/", "index.org", true},
		{"https://ex.com/notes/sub/", "sub/index.org", true},
		{"https://ex.com/notes/my%20page.html", "my page.org", true},
		{"https://ex.com/notes/pic.png", "pic.png", true},
		{"https://ex.com/notes/../etc/passwd", "", false},
		{"https://ex.com/notes/a/%2e%2e/%2e%2e/x.html", "", false},
		{"https://other.com/notes/a.html", "", false},
	}
	for _, c := range cases {
		got, ok := ProtocolSourceRel(pr, c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("%s: got %q %v, want %q %v", c.in, got, ok, c.want, c.ok)
		}
	}
}
