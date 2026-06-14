package lorefs

import "os"

// PrivateDirMode is used for Lore-owned state that may contain project memory,
// audit records, verification output, or rollback snapshots.
const PrivateDirMode os.FileMode = 0o700

// PrivateFileMode is used for Lore-owned files that may contain sensitive
// project context or names of sensitive skipped files.
const PrivateFileMode os.FileMode = 0o600

// PublicDirMode is used for ordinary project files created by the coding agent.
const PublicDirMode os.FileMode = 0o755

// PublicFileMode is used for ordinary project files created by the coding agent.
const PublicFileMode os.FileMode = 0o644

// MkdirPrivate creates a private directory and tightens permissions if it
// already existed with broader mode bits.
func MkdirPrivate(path string) error {
	if err := os.MkdirAll(path, PrivateDirMode); err != nil {
		return err
	}
	return os.Chmod(path, PrivateDirMode)
}

// WritePrivate writes a private file and tightens permissions on platforms
// where chmod is meaningful.
func WritePrivate(path string, data []byte) error {
	if err := os.WriteFile(path, data, PrivateFileMode); err != nil {
		return err
	}
	return os.Chmod(path, PrivateFileMode)
}
