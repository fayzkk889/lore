package pathutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSafeJoin(t *testing.T) {
	root := t.TempDir()

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"simple relative", "foo.txt", false},
		{"nested relative", "src/main.go", false},
		{"dotdot escape", "../../../etc/passwd", true},
		{"absolute unix", "/etc/passwd", true},
		{"absolute windows", "C:\\Windows\\System32\\config", true},
		{"dotdot in middle", "src/../../etc/passwd", true},
		{"dot current", "./foo.txt", false},
		{"hidden file", ".config/settings.json", false},
		{"double dotdot", "foo/bar/../../baz/../../../escape", true},
		{"backslash traversal", "..\\..\\..\\etc\\passwd", true},
		{"encoded dots", "..%2f..%2f..%2fetc%2fpasswd", false}, // URL encoding is literal chars, not traversal
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := SafeJoin(root, tt.path)
			if tt.wantErr {
				if err == nil {
					t.Errorf("SafeJoin(%q, %q) = %q, want error", root, tt.path, result)
				}
				return
			}
			if err != nil {
				t.Errorf("SafeJoin(%q, %q) unexpected error: %v", root, tt.path, err)
				return
			}
			rel, err := filepath.Rel(root, result)
			if err != nil {
				t.Fatalf("filepath.Rel failed: %v", err)
			}
			if rel == ".." || (len(rel) > 2 && rel[:3] == ".."+string(os.PathSeparator)) {
				t.Errorf("result %q escapes root %q (rel=%q)", result, root, rel)
			}
		})
	}
}
