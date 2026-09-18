package e2e

import "testing"

func TestCaptureOperatorLogsBeforeCleanup(t *testing.T) {
	var events []string

	logs := captureOperatorLogsBeforeCleanup(
		func() string {
			events = append(events, "logs")
			return "operator logs"
		},
		func() {
			events = append(events, "cleanup")
		},
	)

	if logs != "operator logs" {
		t.Fatalf("captured logs = %q, want %q", logs, "operator logs")
	}

	if got, want := len(events), 2; got != want {
		t.Fatalf("events = %v, want %d events", events, want)
	}
	if events[0] != "logs" || events[1] != "cleanup" {
		t.Fatalf("events = %v, want logs followed by cleanup", events)
	}
}
