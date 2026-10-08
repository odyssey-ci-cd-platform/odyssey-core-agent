package main

import (
	"testing"
)

// TestListenAddr asserts the server binds localhost by default and honors
// a verbatim ODYSSEY_ADDR — the old ":" prefix made localhost binding
// impossible (AUD-005).
func TestListenAddr(t *testing.T) {
	tests := []struct {
		name string
		env  string
		want string
	}{
		{name: "unset binds localhost", env: "", want: "localhost:50051"},
		{name: "port only is honored verbatim", env: ":6000", want: ":6000"},
		{name: "host and port is honored verbatim", env: "127.0.0.1:6000", want: "127.0.0.1:6000"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := listenAddr(tt.env); got != tt.want {
				t.Errorf("listenAddr(%q) = %q, want %q", tt.env, got, tt.want)
			}
		})
	}
}
