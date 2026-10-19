package orgs

import (
	"strings"
	"testing"
)

func TestSetYamlKey(t *testing.T) {
	file := "server:\n  orgDirs:\n    - \"/x\"\n  dndMusicPath: \"/m\"\n  # make an app first\n  #spotifyClientId: \"\"\n  port: 8010\nurl: \"http://localhost\"\n"
	lines := strings.Split(strings.TrimSuffix(file, "\n"), "\n")
	got, err := setYamlKey(lines, "spotifyClientId", "abc", "dndMusicPath")
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(file, `  #spotifyClientId: ""`, `  spotifyClientId: "abc"`, 1)
	if strings.Join(got, "\n")+"\n" != want {
		t.Fatalf("replaced:\n%s", strings.Join(got, "\n"))
	}
	// Not there at all: added after dndMusicPath, inside server:, not after url:.
	file2 := "server:\n    dndMusicPath: \"/m\"\n    port: 8010\nurl: \"x\"\n"
	got, _ = setYamlKey(strings.Split(strings.TrimSuffix(file2, "\n"), "\n"), "spotifyClientId", "abc", "dndMusicPath")
	if strings.Join(got, "\n") != "server:\n    dndMusicPath: \"/m\"\n    spotifyClientId: \"abc\"\n    port: 8010\nurl: \"x\"" {
		t.Fatalf("added:\n%s", strings.Join(got, "\n"))
	}
	// A key of the same name outside server: is not the one.
	file3 := "spotifyClientId: \"top\"\nserver:\n  port: 8010\n"
	got, _ = setYamlKey(strings.Split(strings.TrimSuffix(file3, "\n"), "\n"), "spotifyClientId", "abc", "dndMusicPath")
	if strings.Join(got, "\n") != "spotifyClientId: \"top\"\nserver:\n  spotifyClientId: \"abc\"\n  port: 8010" {
		t.Fatalf("outside server:\n%s", strings.Join(got, "\n"))
	}
	if _, err := setYamlKey([]string{"url: x"}, "spotifyClientId", "abc", ""); err == nil {
		t.Fatal("no server: section, and no complaint")
	}
}
