//go:build windows

package protocol

import (
	"fmt"
	"strings"
	"syscall"

	"golang.org/x/sys/windows/registry"
)

func classKey(scheme string) string { return `Software\Classes\` + scheme }

func installHandler(s handlerSpec) (string, error) {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, classKey(s.Scheme), registry.SET_VALUE)
	if err != nil {
		return "", err
	}
	defer k.Close()
	if err := k.SetStringValue("", "URL:"+s.Name); err != nil {
		return "", err
	}
	if err := k.SetStringValue("URL Protocol", ""); err != nil {
		return "", err
	}
	c, _, err := registry.CreateKey(registry.CURRENT_USER, classKey(s.Scheme)+`\shell\open\command`, registry.SET_VALUE)
	if err != nil {
		return "", err
	}
	defer c.Close()
	words := make([]string, len(s.Argv))
	for i, a := range s.Argv {
		words[i] = syscall.EscapeArg(a)
	}
	// Quoted even when it needs no quotes: Windows splits the link itself at
	// spaces otherwise.
	command := strings.Join(words, " ") + ` "%1"`
	if err := c.SetStringValue("", command); err != nil {
		return "", err
	}
	return fmt.Sprintf("Registered HKCU\\%s, which hands %s:// links to: %s\n"+
		"(orgs is a console program, so a window flashes up for a moment on each click.)", classKey(s.Scheme), s.Scheme, command), nil
}

func uninstallHandler(s handlerSpec) (string, error) {
	base := classKey(s.Scheme)
	k, err := registry.OpenKey(registry.CURRENT_USER, base+`\shell\open\command`, registry.QUERY_VALUE)
	if err != nil {
		return "Nothing to remove: HKCU\\" + base + " has no command", nil
	}
	v, _, _ := k.GetStringValue("")
	k.Close()
	if !strings.Contains(strings.ToLower(v), " protocol") {
		return "", fmt.Errorf("HKCU\\%s runs %q, which is not orgs - leaving it alone", base, v)
	}
	// A key goes only once it has no subkeys, so deepest first.
	for _, sub := range []string{`\shell\open\command`, `\shell\open`, `\shell`, ``} {
		if err := registry.DeleteKey(registry.CURRENT_USER, base+sub); err != nil && sub == "" {
			return "", err
		}
	}
	return "Removed HKCU\\" + base, nil
}

func handlerStatus(scheme string) (string, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, classKey(scheme)+`\shell\open\command`, registry.QUERY_VALUE)
	if err != nil {
		k, err = registry.OpenKey(registry.CLASSES_ROOT, scheme+`\shell\open\command`, registry.QUERY_VALUE)
	}
	if err != nil {
		return "Nothing handles " + scheme + ":// links (orgs protocol install)", nil
	}
	defer k.Close()
	v, _, err := k.GetStringValue("")
	if err != nil {
		return "", err
	}
	return scheme + ":// links run: " + v, nil
}
