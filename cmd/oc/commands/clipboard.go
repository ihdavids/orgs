package commands

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// CopyText puts text on the clipboard with whichever program this system has
// for it, answering the program's name.
func CopyText(text string) (string, error) {
	var cmds [][]string
	switch runtime.GOOS {
	case "darwin":
		cmds = [][]string{{"pbcopy"}}
	case "windows":
		cmds = [][]string{{"clip"}}
	default:
		cmds = [][]string{{"wl-copy"}, {"xclip", "-selection", "clipboard"}, {"xsel", "--clipboard", "--input"}}
	}
	for _, c := range cmds {
		if _, err := exec.LookPath(c[0]); err != nil {
			continue
		}
		cmd := exec.Command(c[0], c[1:]...)
		cmd.Stdin = strings.NewReader(text)
		if err := cmd.Run(); err == nil {
			return c[0], nil
		}
	}
	return "", fmt.Errorf("no clipboard program found (pbcopy, wl-copy, xclip, xsel, clip)")
}
