package common

import "testing"

// The same cases are in worg's src/medianote.test.ts and vscode's
// test/media-note-model.test.ts: change all three together.

func TestMediaStampRoundTrip(t *testing.T) {
	parse := []struct {
		in   string
		want float64
		ok   bool
	}{
		{"0:12:30", 750, true},
		{"1:02:03", 3723, true},
		{"12:30", 750, true},
		{"0:00:05.250", 5.25, true},
		{"0:00:05,5", 5.5, true},
		{"754", 754, true},
		{"754.5s", 754.5, true},
		{"0:61:00", 0, false},
		{"12:75", 0, false},
		{"soon", 0, false},
	}
	for _, c := range parse {
		got, ok := ParseMediaStamp(c.in)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("ParseMediaStamp(%q) = %v %v, want %v %v", c.in, got, ok, c.want, c.ok)
		}
	}
	write := []struct {
		in   float64
		frac bool
		want string
	}{
		{750, false, "0:12:30"},
		{3723.4, false, "1:02:03"},
		{59.6, false, "0:01:00"},
		{5.25, true, "0:00:05.250"},
		{-3, false, "0:00:00"},
	}
	for _, c := range write {
		if got := MediaStamp(c.in, c.frac); got != c.want {
			t.Errorf("MediaStamp(%v, %v) = %q, want %q", c.in, c.frac, got, c.want)
		}
	}
}

func TestFindMediaLinks(t *testing.T) {
	line := "- [[video:lectures/week3.mp4#0:12:30][0:12:30]] proof, then [[audio:talk.mp3#0:01:23-0:01:40][0:01:23-0:01:40]] and [[videocite:smith2020#0:05:00]] [[video:https://youtu.be/dQw4w9WgXcQ#t#0:00:43][x]] [[file:a.mp4]]"
	got := FindMediaLinks(line)
	if len(got) != 4 {
		t.Fatalf("want 4 links, got %d: %+v", len(got), got)
	}
	want := []MediaLink{
		{"video", "lectures/week3.mp4", 750, -1, "0:12:30"},
		{"audio", "talk.mp3", 83, 100, "0:01:23-0:01:40"},
		{"videocite", "smith2020", 300, -1, ""},
		{"video", "https://youtu.be/dQw4w9WgXcQ#t", 43, -1, "x"},
	}
	for i, w := range want {
		if got[i].MediaLink != w {
			t.Errorf("link %d = %+v, want %+v", i, got[i].MediaLink, w)
		}
	}
	if line[got[0].From:got[0].To] != "[[video:lectures/week3.mp4#0:12:30][0:12:30]]" {
		t.Errorf("span wrong: %q", line[got[0].From:got[0].To])
	}
	if p, s, e := SplitMediaTarget("https://example.com/a#section"); p != "https://example.com/a#section" || s != -1 || e != -1 {
		t.Errorf("a url fragment that is not a time was split: %q %v %v", p, s, e)
	}
}

func TestMediaLinkText(t *testing.T) {
	cases := []struct {
		typ, target string
		a, b        float64
		want        string
	}{
		{"video", "a.mp4", 750, -1, "[[video:a.mp4#0:12:30][0:12:30]]"},
		{"audio", "b.mp3", 83, 100, "[[audio:b.mp3#0:01:23-0:01:40][0:01:23-0:01:40]]"},
		{"videocite", "key", 0, -1, "[[videocite:key#0:00:00][0:00:00]]"},
		{"video", "a.mp4", -1, -1, "[[video:a.mp4]]"},
	}
	for _, c := range cases {
		if got := MediaLinkText(c.typ, c.target, c.a, c.b, false); got != c.want {
			t.Errorf("MediaLinkText = %q, want %q", got, c.want)
		}
	}
}

func TestMediaIds(t *testing.T) {
	yt := map[string]string{
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ&t=10s": "dQw4w9WgXcQ",
		"https://youtu.be/dQw4w9WgXcQ":                      "dQw4w9WgXcQ",
		"https://www.youtube.com/embed/dQw4w9WgXcQ":         "dQw4w9WgXcQ",
		"https://youtube.com/shorts/dQw4w9WgXcQ":            "dQw4w9WgXcQ",
		"https://m.youtube.com/watch?v=dQw4w9WgXcQ":         "dQw4w9WgXcQ",
		"https://example.com/watch?v=dQw4w9WgXcQ":           "",
	}
	for u, want := range yt {
		if got := YoutubeId(u); got != want {
			t.Errorf("YoutubeId(%q) = %q, want %q", u, got, want)
		}
	}
	vm := map[string]string{
		"https://vimeo.com/76979871":                  "76979871",
		"https://player.vimeo.com/video/76979871":     "76979871",
		"https://vimeo.com/channels/staffpicks/12345": "12345",
		"https://youtu.be/dQw4w9WgXcQ":                "",
	}
	for u, want := range vm {
		if got := VimeoId(u); got != want {
			t.Errorf("VimeoId(%q) = %q, want %q", u, got, want)
		}
	}
	kinds := map[string]string{"a.mp4": "video", "b.MP3": "audio", "c.webm": "video", "d.m4a": "audio", "e.pdf": ""}
	for n, want := range kinds {
		if got := MediaKindOf(n); got != want {
			t.Errorf("MediaKindOf(%q) = %q, want %q", n, got, want)
		}
	}
}
