package runner

import (
	"sort"
	"strings"
)

// Steps run as separate `docker exec` calls, so shell state (e.g. `export`)
// does not survive from one step to the next — only the container filesystem
// does. To let a step deliberately pass env vars to later steps, its path is
// exposed via the ODYSSEY_ENV variable: a step appends `KEY=value` lines to
// that file (e.g. `echo "VERSION=1.2.3" >> "$ODYSSEY_ENV"`) and subsequent
// steps receive those vars. Modeled on GitHub Actions' $GITHUB_ENV.
const (
	envFilePath = "/tmp/.odyssey_env"
	envFileVar  = "ODYSSEY_ENV"
)

// parseEnvFile parses the contents of the env file into a map. Blank lines and
// lines without '=' are ignored. When a key appears more than once the later
// value wins, matching shell append (>>) order.
func parseEnvFile(contents string) map[string]string {
	env := make(map[string]string)
	for line := range strings.SplitSeq(contents, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		env[key] = val
	}
	return env
}

// envSlice converts an env map into the KEY=value slice Docker expects.
func envSlice(env map[string]string) []string {
	if len(env) == 0 {
		return nil
	}
	out := make([]string, 0, len(env))
	for k, v := range env {
		out = append(out, k+"="+v)
	}
	return out
}

// newlyExportedKeys returns the sorted keys present in after but absent or
// changed in before — i.e. the vars the most recent step exported.
func newlyExportedKeys(before, after map[string]string) []string {
	var keys []string
	for k, v := range after {
		if old, ok := before[k]; !ok || old != v {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}
