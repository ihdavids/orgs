package common

import "testing"

func TestNewerVersion(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"v1.2.4", "v1.2.3", true},
		{"v1.2.3", "v1.2.3", false},
		{"v1.10.0", "v1.9.9", true},
		{"1.3.0", "v1.2.9", true},
		{"v1.2.3", "v1.2.3-rc.1", true},
		{"v1.2.3-rc.1", "v1.2.3", false},
		{"v1.2.3-rc.2", "v1.2.3-rc.1", true},
		{"v1.2.3-rc.10", "v1.2.3-rc.9", true},
		{"v1.2.3-beta", "v1.2.3-alpha", true},
		{"v1.2.3", "dev", false},
		{"dev", "v1.2.3", false},
		{"v2.0.0", "v1.99.99", true},
	}
	for _, c := range cases {
		if got := NewerVersion(c.a, c.b); got != c.want {
			t.Errorf("NewerVersion(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}
