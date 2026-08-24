package main

import (
	"fmt"
	"os/exec"
)

const notificationTitle = "MyWhoosh2Garmin"

// garminSyncSummary creates the user-facing outcome for a completed sync.
func garminSyncSummary(uploaded, skipped int) string {
	if uploaded > 0 {
		message := fmt.Sprintf("Garmin sync successful: %d activit%s", uploaded, plural(uploaded, "y", "ies"))
		if skipped > 0 {
			message += fmt.Sprintf("; %d skipped", skipped)
		}
		return message
	}
	return fmt.Sprintf("Garmin sync skipped: %d activit%s", skipped, plural(skipped, "y", "ies"))
}

// sendMacNotification posts a notification through Notification Center. The
// AppleScript is fixed and the message is passed as an argv value, not code.
func sendMacNotification(message string) error {
	script := `on run argv
	display notification (item 1 of argv) with title (item 2 of argv)
end run`
	if output, err := exec.Command("osascript", "-e", script, message, notificationTitle).CombinedOutput(); err != nil {
		return fmt.Errorf("post macOS notification: %w: %s", err, string(output))
	}
	return nil
}
