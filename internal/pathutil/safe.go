package pathutil

import (
	"fmt"
	"path/filepath"
	"strings"

	securejoin "github.com/cyphar/filepath-securejoin"
)

// SafeJoin resolves a potentially-untrusted relative path against a root
// directory and guarantees the result is within root. Returns an error if
// the path escapes root (via "..", absolute path, symlink, etc.).
//
// Use this for EVERY file write where the path originates from model output,
// API responses, or any external/untrusted source.
func SafeJoin(root, untrustedPath string) (string, error) {
	if filepath.IsAbs(untrustedPath) {
		return "", fmt.Errorf("absolute path not allowed: %q", untrustedPath)
	}

	// Reject Unix-style absolute paths on Windows (filepath.IsAbs misses these)
	if len(untrustedPath) > 0 && (untrustedPath[0] == '/' || untrustedPath[0] == '\\') {
		return "", fmt.Errorf("absolute path not allowed: %q", untrustedPath)
	}

	cleaned := filepath.Clean(untrustedPath)
	for _, part := range strings.Split(cleaned, string(filepath.Separator)) {
		if part == ".." {
			return "", fmt.Errorf("path traversal not allowed: %q", untrustedPath)
		}
	}

	safe, err := securejoin.SecureJoin(root, untrustedPath)
	if err != nil {
		return "", fmt.Errorf("path validation failed for %q: %w", untrustedPath, err)
	}

	return safe, nil
}