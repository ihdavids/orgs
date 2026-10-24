package orgs

import (
	"strings"
	"testing"
)

func TestSetYamlPath(t *testing.T) {
	cases := []struct {
		name, in, want string
		path           []string
		value          string
	}{
		{
			name: "replaces the leaf, leaves the rest",
			in: `server:
    port: 8010
    # the writing assistant
    assistant:
        backend: claude   # was
        ollama:
            enabled: true
            model: llama3
    noAuth: true
`,
			path:  []string{"server", "assistant", "ollama", "model"},
			value: "qwen3:8b",
			want: `server:
    port: 8010
    # the writing assistant
    assistant:
        backend: claude   # was
        ollama:
            enabled: true
            model: "qwen3:8b"
    noAuth: true
`,
		},
		{
			name: "adds the leaf at the end of its section, before the next section's comment",
			in: `server:
  assistant:
    ollama:
      enabled: true

  # dnd
  dndPaths: []
`,
			path:  []string{"server", "assistant", "backend"},
			value: "ollama",
			want: `server:
  assistant:
    ollama:
      enabled: true
    backend: "ollama"

  # dnd
  dndPaths: []
`,
		},
		{
			name: "adds missing sections",
			in: `url: http://localhost:8010
server:
  port: 8010
author: me
`,
			path:  []string{"server", "assistant", "local", "model"},
			value: "llamacpp:qwen",
			want: `url: http://localhost:8010
server:
  port: 8010
  assistant:
    local:
      model: "llamacpp:qwen"
author: me
`,
		},
		{
			name: "a key of the same name deeper down is not the one",
			in: `server:
  plugins:
    - name: x
      backend: y
  port: 1
`,
			path:  []string{"server", "backend"},
			value: "z",
			want: `server:
  plugins:
    - name: x
      backend: y
  port: 1
  backend: "z"
`,
		},
	}
	for _, c := range cases {
		got, err := setYamlPath(strings.Split(strings.TrimSuffix(c.in, "\n"), "\n"), c.path, c.value)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if s := strings.Join(got, "\n") + "\n"; s != c.want {
			t.Errorf("%s:\n got:\n%s\nwant:\n%s", c.name, s, c.want)
		}
	}

	if _, err := setYamlPath([]string{"server:", "  assistant: {backend: claude}"}, []string{"server", "assistant", "backend"}, "x"); err == nil {
		t.Errorf("an inline section was edited instead of refused")
	}
}
