//go:build !darwin && !linux && !windows

package protocol

import (
	"fmt"
	"runtime"
)

func installHandler(s handlerSpec) (string, error) {
	return "", fmt.Errorf("no way to register a url handler on %s that orgs knows; point your desktop's %s:// handler at: %v <link>",
		runtime.GOOS, s.Scheme, s.Argv)
}

func uninstallHandler(s handlerSpec) (string, error) {
	return "", fmt.Errorf("nothing orgs installs on %s", runtime.GOOS)
}

func handlerStatus(scheme string) (string, error) {
	return "", fmt.Errorf("orgs cannot ask %s which program handles %s://", runtime.GOOS, scheme)
}
