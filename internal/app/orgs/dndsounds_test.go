package orgs

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestSoundTranscode(t *testing.T) {
	ff, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("no ffmpeg")
	}
	dir := t.TempDir()
	in := filepath.Join(dir, "beep.aiff")
	if out, err := exec.Command(ff, "-v", "error", "-f", "lavfi", "-i", "sine=frequency=440:duration=0.3", in).CombinedOutput(); err != nil {
		t.Fatalf("making an aiff: %s %s", err, out)
	}
	out, err := soundTranscode(in)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(out)
	if len(b) < 4 || string(b[:4]) != "OggS" {
		t.Fatalf("not an ogg: %q", b[:8])
	}
	// Again: from the cache, the same file.
	if again, err := soundTranscode(in); err != nil || again != out {
		t.Fatalf("not kept: %s %v", again, err)
	}
}

func TestCleanTitle(t *testing.T) {
	for in, want := range map[string]string{"Dragon roar 7.wav": "Dragon roar 7", "sword 02.aif": "sword 02", "Thunder": "Thunder"} {
		if got := cleanTitle(in); got != want {
			t.Errorf("%q: got %q", in, got)
		}
	}
}
