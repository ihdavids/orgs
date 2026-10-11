//go:build windows

package desktop

import (
	"fmt"
	"os"

	"github.com/go-toast/toast"
)

// A toast needs an application id Windows knows, or it is dropped without a
// word. PowerShell's is registered on every Windows 10 and later machine, so
// the toast is shown as PowerShell's - the trade every toast-from-a-script
// makes without an installer.
const powershellAppID = "{1AC14E77-02E7-4E5D-B744-2EB1AE5198B7}\\WindowsPowerShell\\v1.0\\powershell.exe"

type toastNotifier struct{}

func (toastNotifier) Name() string { return "Windows (toast)" }

func (toastNotifier) Send(n Note) error {
	t := toastFor(n)
	if err := t.Push(); err != nil {
		return fmt.Errorf("toast: %w", err)
	}
	return nil
}

// The toast for a note. A link opens through protocol activation, both on a
// click of the toast and from its Open button.
func toastFor(n Note) *toast.Notification {
	t := &toast.Notification{
		AppID:   powershellAppID,
		Title:   clip(n.Title, maxTitle),
		Message: clip(bodyWithSubtitle(n), maxBody),
		Audio:   toast.Silent,
	}
	if n.Sound || n.Urgency == Critical {
		t.Audio = toast.Reminder
	}
	if n.Urgency == Critical {
		t.Duration = toast.Long
	}
	if n.Icon != "" {
		if _, err := os.Stat(n.Icon); err == nil {
			t.Icon = n.Icon
		}
	}
	if n.URL != "" {
		t.ActivationType = "protocol"
		t.ActivationArguments = n.URL
		t.Actions = []toast.Action{{Type: "protocol", Label: "Open", Arguments: n.URL}}
	}
	return t
}

func Detect() Notifier {
	if have("powershell") || have("powershell.exe") {
		return toastNotifier{}
	}
	return noNotifier{why: "PowerShell not found"}
}
