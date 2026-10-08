package server

import (
	"path/filepath"
	"strings"
)

// projectPathAllowed reports whether path may be served when the project
// root is root. An empty root means unrestricted — the local-development
// posture; a set root confines every request under it (AUD-005).
func projectPathAllowed(root, path string) bool {
	if root == "" {
		return true
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "..")
}
