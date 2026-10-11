//go:build !darwin && !linux && !windows

package desktop

import "runtime"

func Detect() Notifier {
	return noNotifier{why: "no desktop notifications on " + runtime.GOOS}
}
