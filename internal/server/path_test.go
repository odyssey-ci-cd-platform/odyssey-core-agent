package server

import (
	"testing"
)

// TestProjectPathAllowed asserts project paths are confined under the
// configured root; an empty root means unrestricted (local development)
// (AUD-005).
func TestProjectPathAllowed(t *testing.T) {
	tests := []struct {
		name    string
		root    string
		path    string
		allowed bool
	}{
		{name: "inside root", root: "/srv/projects", path: "/srv/projects/app", allowed: true},
		{name: "root itself", root: "/srv/projects", path: "/srv/projects", allowed: true},
		{name: "outside root", root: "/srv/projects", path: "/home/user", allowed: false},
		{name: "traversal stays outside", root: "/srv/projects", path: "/srv/projects/../secrets", allowed: false},
		{name: "empty root is unrestricted", root: "", path: "/home/user", allowed: true},
		{name: "prefix lookalike is outside", root: "/srv/projects", path: "/srv/projects-evil", allowed: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := projectPathAllowed(tt.root, tt.path); got != tt.allowed {
				t.Errorf("projectPathAllowed(%q, %q) = %v, want %v", tt.root, tt.path, got, tt.allowed)
			}
		})
	}
}
