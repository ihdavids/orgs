package orgs

import "testing"

func TestPagePicture(t *testing.T) {
	page := `<html><head><title>Old Title</title>
<meta content="/img/elf.png?a=1&amp;b=2" property="og:image">
<meta name="twitter:image" content="https://cdn.example.com/t.jpg"></head>
<body><img src="first.png"></body></html>`
	got, title := pagePicture([]byte(page), "https://wiki.example.com/npcs/elf")
	if got != "https://wiki.example.com/img/elf.png?a=1&b=2" || title != "Old Title" {
		t.Fatalf("got %q, %q", got, title)
	}
	_, title = pagePicture([]byte(`<title>Owlbear - Wikipedia</title><meta property="og:image" content="x.jpg">`), "https://w.org/")
	if title != "Owlbear" {
		t.Fatalf("suffix: got %q", title)
	}
	_, title = pagePicture([]byte(`<title>Owlbear - Wikipedia</title><meta property="og:title" content="The Owlbear - Wikipedia"><meta property="og:image" content="x.jpg">`), "https://w.org/")
	if title != "The Owlbear" {
		t.Fatalf("got %q, %q", got, title)
	}
	got, _ = pagePicture([]byte(`<p><img src="data:x"><img alt="x" src='pics/a.webp'></p>`), "https://x.org/a/b.html")
	if got != "https://x.org/a/pics/a.webp" {
		t.Fatalf("fallback: got %q", got)
	}
	if got, _ := pagePicture([]byte(`<p>nothing</p>`), "https://x.org/"); got != "" {
		t.Fatalf("none: got %q", got)
	}
}

func TestSafeShowName(t *testing.T) {
	for in, want := range map[string]string{
		"../../etc/passwd": "etc passwd",
		"  Lord   Vex!  ":  "Lord Vex",
		".hidden":          "hidden",
		"npcs/elves":       "npcs elves",
		"Beholder (D&D)":   "Beholder (D&D)",
		"a..b":             "a b",
	} {
		if got := safeShowName(in); got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
}
