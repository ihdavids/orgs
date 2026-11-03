//go:build linux

package protocol

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func applicationsDir() (string, error) {
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return filepath.Join(d, "applications"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "applications"), nil
}

func desktopName(s handlerSpec) string { return s.Id + ".desktop" }

// desktopArg quotes one argument of an Exec line as the desktop entry spec
// wants: in double quotes when it has a reserved character, with ", `, $
// and \ escaped, and then every backslash doubled again, because the value's
// own string escaping comes first. A % is a field code unless doubled.
func desktopArg(a string) string {
	a = strings.ReplaceAll(a, "%", "%%")
	if a != "" && !strings.ContainsAny(a, " \t\n\"'\\><~|&;$*?#()`") {
		return a
	}
	q := `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "`", "\\`", `$`, `\$`).Replace(a) + `"`
	return strings.ReplaceAll(q, `\`, `\\`)
}

func installHandler(s handlerSpec) (string, error) {
	dir, err := applicationsDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	words := make([]string, 0, len(s.Argv)+1)
	for _, a := range s.Argv {
		words = append(words, desktopArg(a))
	}
	words = append(words, "%u")
	entry := "[Desktop Entry]\n" +
		"Type=Application\n" +
		"Name=" + s.Name + "\n" +
		"Comment=Hands " + s.Scheme + ":// links to orgs\n" +
		"Exec=" + strings.Join(words, " ") + "\n" +
		"Terminal=false\n" +
		"NoDisplay=true\n" +
		"MimeType=x-scheme-handler/" + s.Scheme + ";\n"
	file := filepath.Join(dir, desktopName(s))
	if err := os.WriteFile(file, []byte(entry), 0o644); err != nil {
		return "", err
	}
	notes := []string{}
	if _, err := exec.LookPath("update-desktop-database"); err == nil {
		exec.Command("update-desktop-database", dir).Run()
	}
	if out, err := exec.Command("xdg-mime", "default", desktopName(s), "x-scheme-handler/"+s.Scheme).CombinedOutput(); err != nil {
		notes = append(notes, fmt.Sprintf("xdg-mime could not make it the default (%v %s); your desktop's settings can", err, strings.TrimSpace(string(out))))
	}
	msg := fmt.Sprintf("Installed %s, which hands %s:// links to: %s <link>", file, s.Scheme, strings.Join(s.Argv, " "))
	if now, err := handlerStatus(s.Scheme); err == nil {
		msg += "\n" + now
	}
	for _, n := range notes {
		msg += "\nNote: " + n
	}
	return msg, nil
}

func uninstallHandler(s handlerSpec) (string, error) {
	dir, err := applicationsDir()
	if err != nil {
		return "", err
	}
	file := filepath.Join(dir, desktopName(s))
	if _, err := os.Stat(file); err != nil {
		return "Nothing to remove: there is no " + file, nil
	}
	if err := os.Remove(file); err != nil {
		return "", err
	}
	if _, err := exec.LookPath("update-desktop-database"); err == nil {
		exec.Command("update-desktop-database", dir).Run()
	}
	return "Removed " + file + " (a default left naming it in mimeapps.list is ignored once the file is gone)", nil
}

func handlerStatus(scheme string) (string, error) {
	out, err := exec.Command("xdg-mime", "query", "default", "x-scheme-handler/"+scheme).Output()
	if err != nil {
		return "", fmt.Errorf("xdg-mime: %v", err)
	}
	if d := strings.TrimSpace(string(out)); d != "" {
		return scheme + ":// links open with " + d, nil
	}
	return "Nothing handles " + scheme + ":// links (orgs protocol install)", nil
}
