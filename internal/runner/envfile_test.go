package runner

import (
	"reflect"
	"testing"
)

func TestParseEnvFile(t *testing.T) {
	tests := []struct {
		name     string
		contents string
		want     map[string]string
	}{
		{
			name:     "empty",
			contents: "",
			want:     map[string]string{},
		},
		{
			name:     "single pair",
			contents: "FOO=bar",
			want:     map[string]string{"FOO": "bar"},
		},
		{
			name:     "multiple pairs with trailing newline",
			contents: "FOO=bar\nBAZ=qux\n",
			want:     map[string]string{"FOO": "bar", "BAZ": "qux"},
		},
		{
			name:     "blank lines ignored",
			contents: "FOO=bar\n\n\nBAZ=qux\n",
			want:     map[string]string{"FOO": "bar", "BAZ": "qux"},
		},
		{
			name:     "lines without '=' ignored",
			contents: "FOO=bar\nnot an assignment\nBAZ=qux",
			want:     map[string]string{"FOO": "bar", "BAZ": "qux"},
		},
		{
			name:     "value containing '=' keeps everything after first",
			contents: "URL=https://example.com/?a=1&b=2",
			want:     map[string]string{"URL": "https://example.com/?a=1&b=2"},
		},
		{
			name:     "empty value",
			contents: "FOO=",
			want:     map[string]string{"FOO": ""},
		},
		{
			name:     "surrounding whitespace trimmed",
			contents: "  FOO=bar  \n",
			want:     map[string]string{"FOO": "bar"},
		},
		{
			name:     "later value wins (append order)",
			contents: "FOO=first\nFOO=second",
			want:     map[string]string{"FOO": "second"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseEnvFile(tt.contents)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseEnvFile(%q) = %v, want %v", tt.contents, got, tt.want)
			}
		})
	}
}

func TestEnvSlice(t *testing.T) {
	t.Run("nil for empty map", func(t *testing.T) {
		if got := envSlice(nil); got != nil {
			t.Errorf("envSlice(nil) = %v, want nil", got)
		}
		if got := envSlice(map[string]string{}); got != nil {
			t.Errorf("envSlice(empty) = %v, want nil", got)
		}
	})

	t.Run("formats as KEY=value", func(t *testing.T) {
		got := envSlice(map[string]string{"FOO": "bar"})
		want := []string{"FOO=bar"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("envSlice = %v, want %v", got, want)
		}
	})

	t.Run("one entry per key", func(t *testing.T) {
		got := envSlice(map[string]string{"A": "1", "B": "2", "C": "3"})
		if len(got) != 3 {
			t.Errorf("envSlice returned %d entries, want 3: %v", len(got), got)
		}
	})
}

func TestNewlyExportedKeys(t *testing.T) {
	tests := []struct {
		name   string
		before map[string]string
		after  map[string]string
		want   []string
	}{
		{
			name:   "no change",
			before: map[string]string{"FOO": "bar"},
			after:  map[string]string{"FOO": "bar"},
			want:   nil,
		},
		{
			name:   "new key",
			before: map[string]string{"FOO": "bar"},
			after:  map[string]string{"FOO": "bar", "BAZ": "qux"},
			want:   []string{"BAZ"},
		},
		{
			name:   "changed value",
			before: map[string]string{"FOO": "old"},
			after:  map[string]string{"FOO": "new"},
			want:   []string{"FOO"},
		},
		{
			name:   "new and changed, sorted",
			before: map[string]string{"FOO": "old"},
			after:  map[string]string{"FOO": "new", "ZED": "1", "ABC": "2"},
			want:   []string{"ABC", "FOO", "ZED"},
		},
		{
			name:   "from empty",
			before: map[string]string{},
			after:  map[string]string{"FOO": "bar"},
			want:   []string{"FOO"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := newlyExportedKeys(tt.before, tt.after)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("newlyExportedKeys() = %v, want %v", got, tt.want)
			}
		})
	}
}
