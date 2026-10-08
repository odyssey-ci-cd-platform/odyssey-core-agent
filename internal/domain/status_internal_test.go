package domain

// Internal test for an unexported helper: worstStatus and statusRank are
// aggregation internals with no exported surface that can observe the
// latent statuses (Running, Skipped, Unknown). This file is a documented
// exception to the external-test-package convention for that reason.

import (
	"testing"
)

// TestWorstStatusRanksEveryStatus pins the precedence over every status:
// Running, Skipped, and Unknown must not rank as Passed (AUD-010).
func TestWorstStatusRanksEveryStatus(t *testing.T) {
	tests := []struct {
		name string
		a, b Status
		want Status
	}{
		{"errored beats failed", StatusErrored, StatusFailed, StatusErrored},
		{"failed beats unknown", StatusFailed, StatusUnknown, StatusFailed},
		{"unknown beats running", StatusUnknown, StatusRunning, StatusUnknown},
		{"running beats pending", StatusRunning, StatusPending, StatusRunning},
		{"pending beats skipped", StatusPending, StatusSkipped, StatusPending},
		{"skipped beats passed", StatusSkipped, StatusPassed, StatusSkipped},
		{"pending beats passed", StatusPending, StatusPassed, StatusPending},
		{"equal statuses stay", StatusFailed, StatusFailed, StatusFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := worstStatus(tt.a, tt.b); got != tt.want {
				t.Errorf("worstStatus(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
			// Aggregation is symmetric in either argument order.
			if got := worstStatus(tt.b, tt.a); got != tt.want {
				t.Errorf("worstStatus(%v, %v) = %v, want %v", tt.b, tt.a, got, tt.want)
			}
		})
	}
}
