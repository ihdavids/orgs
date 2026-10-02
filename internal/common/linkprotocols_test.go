package common

import (
	"reflect"
	"testing"
)

func TestResolveLinkProtocol(t *testing.T) {
	protos := map[string]LinkProtocol{
		"jira://": {UrlPrefix: "https://jira.me.com/browse/"},
		"wiki":    {Url: "https://wiki.me.com/p/{{path}}/view"},
		"rdar":    {Cmd: `mytool --open "{{link}}"`},
		"tkt":     {Cmd: "tkt show"},
		"tktp":    {Cmd: "tkt show", WithProtocol: true},
	}
	cases := []struct {
		raw  string
		url  string
		argv []string
	}{
		{"jira:ABC-1", "https://jira.me.com/browse/ABC-1", nil},
		{"JIRA://ABC-1", "https://jira.me.com/browse/ABC-1", nil},
		{"wiki:a b/c", "https://wiki.me.com/p/a%20b/c/view", nil},
		{"rdar://123; rm -rf ~", "", []string{"mytool", "--open", "rdar://123; rm -rf ~"}},
		{"tkt:42", "", []string{"tkt", "show", "42"}},
		{"tktp:42", "", []string{"tkt", "show", "tktp:42"}},
	}
	for _, c := range cases {
		res, ok := ResolveLinkProtocol(protos, c.raw)
		if !ok || !res.Ok {
			t.Fatalf("%s: not resolved: %+v", c.raw, res)
		}
		if res.Url != c.url || !reflect.DeepEqual(res.Command, c.argv) {
			t.Errorf("%s: got url %q cmd %q", c.raw, res.Url, res.Command)
		}
	}
	if _, ok := ResolveLinkProtocol(protos, "https://x.com"); ok {
		t.Error("an undefined protocol resolved")
	}
	if res, _ := ResolveLinkProtocol(protos, "rdar://1 2"); res.CommandLine != "mytool --open 'rdar://1 2'" {
		t.Errorf("command line %q", res.CommandLine)
	}
}
