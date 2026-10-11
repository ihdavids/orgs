//go:build darwin

package desktop

// terminal-notifier when it is installed (`brew install terminal-notifier`),
// because it is the only way on macOS to have a click open the heading and to
// replace a notification rather than stack another; osascript, which every Mac
// has, otherwise.
func Detect() Notifier {
	if have("terminal-notifier") {
		return terminalNotifier{}
	}
	if have("osascript") {
		return osascriptNotifier{}
	}
	return noNotifier{why: "neither terminal-notifier nor osascript found"}
}
