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
	normalized := strings.ReplaceAll(untrustedPath, `\`, `/`)
	if filepath.IsAbs(untrustedPath) || isWindowsAbs(normalized) {
		return "", fmt.Errorf("absolute path not allowed: %q", untrustedPath)
	}

	if len(normalized) > 0 && normalized[0] == '/' {
		return "", fmt.Errorf("absolute path not allowed: %q", untrustedPath)
	}

	cleaned := filepath.ToSlash(filepath.Clean(normalized))
	for _, part := range strings.Split(cleaned, "/") {
		if part == ".." {
			return "", fmt.Errorf("path traversal not allowed: %q", untrustedPath)
		}
	}

	safe, err := securejoin.SecureJoin(root, normalized)
	if err != nil {
		return "", fmt.Errorf("path validation failed for %q: %w", untrustedPath, err)
	}

	return safe, nil
}

func isWindowsAbs(p string) bool {
	if len(p) >= 3 && p[1] == ':' && (p[2] == '/' || p[2] == '\\') {
		c := p[0]
		return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
	}
	return false
}
