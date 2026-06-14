// Package snapshot provides local point-in-time snapshots for Lore rollback.
//
// New snapshots are stored under .lore/snapshots instead of being committed
// to git, so uncommitted user files are not accidentally written to repository
// history. Older git-based lore-snapshot commits are still listed and
// restorable for compatibility.
package snapshot

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"

	"lore-cli/internal/lorefs"
)

// SnapshotPrefix is the commit-message prefix used by older git snapshots.
const SnapshotPrefix = "lore-snapshot:"

var errLocalSnapshotNotFound = errors.New("local snapshot not found")

// Snapshot represents a single rollback point.
type Snapshot struct {
	Hash      string
	ShortHash string
	Timestamp time.Time
	Message   string
}

type manifest struct {
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Message   string    `json:"message"`
	Files     []string  `json:"files"`
	Skipped   []string  `json:"skipped,omitempty"`
}

// CreateSnapshot stores a local copy of the current project files under
// .lore/snapshots. It deliberately does not stage or commit anything.
func CreateSnapshot(projectDir string) (hash string, warn string, err error) {
	now := time.Now()
	id := snapshotID(now)
	loreDir := filepath.Join(projectDir, ".lore")
	if err := lorefs.MkdirPrivate(loreDir); err != nil {
		return "", "", fmt.Errorf("creating lore directory: %w", err)
	}
	if err := lorefs.MkdirPrivate(filepath.Join(loreDir, "snapshots")); err != nil {
		return "", "", fmt.Errorf("creating snapshots directory: %w", err)
	}
	root := filepath.Join(projectDir, ".lore", "snapshots", id)
	filesRoot := filepath.Join(root, "files")
	if err := lorefs.MkdirPrivate(root); err != nil {
		return "", "", fmt.Errorf("creating snapshot directory: %w", err)
	}
	if err := lorefs.MkdirPrivate(filesRoot); err != nil {
		return "", "", fmt.Errorf("creating snapshot directory: %w", err)
	}

	m := manifest{
		ID:        id,
		Timestamp: now,
		Message:   fmt.Sprintf("local-snapshot: %s - before applying changes", now.Format("2006-01-02 15:04:05")),
	}
	err = filepath.Walk(projectDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		rel, err := filepath.Rel(projectDir, path)
		if err != nil || rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if info.IsDir() {
			if skipDir(info.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if shouldSkipFile(path, rel, info) {
			m.Skipped = append(m.Skipped, rel)
			return nil
		}
		dst := filepath.Join(filesRoot, filepath.FromSlash(rel))
		if err := lorefs.MkdirPrivate(filepath.Dir(dst)); err != nil {
			return err
		}
		if err := copyFile(path, dst, info.Mode()); err != nil {
			return err
		}
		m.Files = append(m.Files, rel)
		return nil
	})
	if err != nil {
		return "", "", fmt.Errorf("creating snapshot: %w", err)
	}
	sort.Strings(m.Files)
	sort.Strings(m.Skipped)
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return "", "", err
	}
	if err := lorefs.WritePrivate(filepath.Join(root, "manifest.json"), data); err != nil {
		return "", "", fmt.Errorf("writing snapshot manifest: %w", err)
	}
	return shortID(id), "", nil
}

// ListSnapshots returns recent local snapshots, followed by older git
// lore-snapshot commits for compatibility.
func ListSnapshots(projectDir string, n int) ([]Snapshot, error) {
	snapshots, err := listLocalSnapshots(projectDir, n)
	if err != nil {
		return nil, err
	}
	if len(snapshots) >= n {
		return snapshots[:n], nil
	}
	gitSnaps, err := listGitSnapshots(projectDir, n-len(snapshots))
	if err != nil {
		return nil, err
	}
	return append(snapshots, gitSnaps...), nil
}

// RestoreSnapshot restores either a local .lore snapshot or an older
// git-based snapshot commit.
func RestoreSnapshot(projectDir string, shortOrFullHash string) error {
	err := restoreLocalSnapshot(projectDir, shortOrFullHash)
	if err == nil {
		return nil
	}
	if !errors.Is(err, errLocalSnapshotNotFound) {
		return err
	}
	return restoreGitSnapshot(projectDir, shortOrFullHash)
}

func listLocalSnapshots(projectDir string, n int) ([]Snapshot, error) {
	root := filepath.Join(projectDir, ".lore", "snapshots")
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Snapshot
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		m, err := readManifest(filepath.Join(root, e.Name()))
		if err != nil {
			continue
		}
		out = append(out, Snapshot{
			Hash:      m.ID,
			ShortHash: shortID(m.ID),
			Timestamp: m.Timestamp,
			Message:   m.Message,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Timestamp.After(out[j].Timestamp) })
	if len(out) > n {
		out = out[:n]
	}
	return out, nil
}

func listGitSnapshots(projectDir string, n int) ([]Snapshot, error) {
	repo, err := git.PlainOpenWithOptions(projectDir, &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return nil, nil
	}
	head, err := repo.Head()
	if err != nil {
		return nil, nil
	}
	iter, err := repo.Log(&git.LogOptions{From: head.Hash()})
	if err != nil {
		return nil, fmt.Errorf("reading git log: %w", err)
	}
	defer iter.Close()

	var snapshots []Snapshot
	for {
		c, iterErr := iter.Next()
		if iterErr != nil {
			break
		}
		firstLine := strings.SplitN(c.Message, "\n", 2)[0]
		if !strings.HasPrefix(firstLine, SnapshotPrefix) {
			continue
		}
		hashStr := c.Hash.String()
		snapshots = append(snapshots, Snapshot{
			Hash:      hashStr,
			ShortHash: shortID(hashStr),
			Timestamp: c.Author.When,
			Message:   strings.TrimSpace(firstLine),
		})
		if len(snapshots) >= n {
			break
		}
	}
	return snapshots, nil
}

func restoreLocalSnapshot(projectDir string, idPrefix string) error {
	root := filepath.Join(projectDir, ".lore", "snapshots")
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return errLocalSnapshotNotFound
		}
		return err
	}
	var snapDir string
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), idPrefix) {
			if snapDir != "" {
				return fmt.Errorf("multiple local snapshots match %q", idPrefix)
			}
			snapDir = filepath.Join(root, e.Name())
		}
	}
	if snapDir == "" {
		return fmt.Errorf("%w: %q", errLocalSnapshotNotFound, idPrefix)
	}
	m, err := readManifest(snapDir)
	if err != nil {
		return err
	}
	keep := make(map[string]bool, len(m.Files)+len(m.Skipped))
	for _, f := range m.Files {
		keep[f] = true
	}
	for _, f := range m.Skipped {
		keep[f] = true
	}
	if err := removeFilesNotInSnapshot(projectDir, keep); err != nil {
		return err
	}
	for _, rel := range m.Files {
		src := filepath.Join(snapDir, "files", filepath.FromSlash(rel))
		dst := filepath.Join(projectDir, filepath.FromSlash(rel))
		info, err := os.Stat(src)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(dst), lorefs.PublicDirMode); err != nil {
			return err
		}
		if err := copyFile(src, dst, info.Mode()); err != nil {
			return err
		}
	}
	return nil
}

func restoreGitSnapshot(projectDir string, shortOrFullHash string) error {
	repo, err := git.PlainOpenWithOptions(projectDir, &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return fmt.Errorf("not a git repo: %w", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		return err
	}
	hash, err := resolveHash(repo, shortOrFullHash)
	if err != nil {
		return fmt.Errorf("resolving %q: %w", shortOrFullHash, err)
	}
	return wt.Reset(&git.ResetOptions{Commit: hash, Mode: git.HardReset})
}

func resolveHash(repo *git.Repository, s string) (plumbing.Hash, error) {
	if len(s) == 40 {
		return plumbing.NewHash(s), nil
	}
	head, err := repo.Head()
	if err != nil {
		return plumbing.ZeroHash, err
	}
	iter, err := repo.Log(&git.LogOptions{From: head.Hash()})
	if err != nil {
		return plumbing.ZeroHash, err
	}
	defer iter.Close()
	for {
		c, iterErr := iter.Next()
		if iterErr != nil {
			break
		}
		if strings.HasPrefix(c.Hash.String(), s) {
			return c.Hash, nil
		}
	}
	return plumbing.ZeroHash, fmt.Errorf("no commit with prefix %q", s)
}

func snapshotID(t time.Time) string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return t.Format("20060102150405") + "-" + hex.EncodeToString(b[:])
}

func shortID(id string) string {
	if strings.Contains(id, "-") && len(id) > 19 {
		return id[:19]
	}
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

func readManifest(dir string) (manifest, error) {
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return manifest{}, err
	}
	var m manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return manifest{}, err
	}
	return m, nil
}

func removeFilesNotInSnapshot(projectDir string, keep map[string]bool) error {
	return filepath.Walk(projectDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		rel, err := filepath.Rel(projectDir, path)
		if err != nil || rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if info.IsDir() {
			if skipDir(info.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !keep[rel] {
			return os.Remove(path)
		}
		return nil
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func skipDir(name string) bool {
	switch name {
	case ".git", ".lore", "node_modules", "vendor", "dist", "build", "__pycache__":
		return true
	}
	return false
}

func isBinaryName(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".exe", ".dll", ".so", ".dylib", ".bin", ".png", ".jpg", ".jpeg", ".gif", ".ico",
		".zip", ".tar", ".gz", ".pdf", ".db", ".sqlite", ".woff", ".woff2", ".ttf":
		return true
	}
	return false
}

func shouldSkipFile(path, rel string, info os.FileInfo) bool {
	name := strings.ToLower(info.Name())
	rel = strings.ToLower(filepath.ToSlash(rel))
	if info.Size() > 5*1024*1024 || isBinaryName(name) {
		return true
	}
	if strings.HasPrefix(name, ".env") ||
		strings.HasSuffix(name, ".pem") ||
		strings.HasSuffix(name, ".key") ||
		strings.HasSuffix(name, ".p12") ||
		strings.HasSuffix(name, ".pfx") ||
		strings.Contains(name, "id_rsa") ||
		strings.Contains(name, "id_ed25519") {
		return true
	}
	if strings.Contains(rel, "secret") ||
		strings.Contains(rel, "secrets") ||
		strings.Contains(rel, "credential") ||
		strings.Contains(rel, "credentials") ||
		strings.Contains(rel, "token") {
		return true
	}
	return hasSensitiveContent(path, info.Size())
}

func hasSensitiveContent(path string, size int64) bool {
	if size <= 0 || size > 512*1024 {
		return false
	}
	data, err := os.ReadFile(path)
	if err != nil || bytes.IndexByte(data, 0) >= 0 {
		return false
	}
	lower := strings.ToLower(string(data))
	if strings.Contains(lower, "-----begin ") && strings.Contains(lower, " private key-----") {
		return true
	}
	for _, line := range strings.Split(lower, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		if strings.Contains(line, "authorization: bearer ") {
			return true
		}
		for _, marker := range []string{"api_key", "apikey", "api-key", "password", "secret", "token"} {
			if strings.Contains(line, marker) && (strings.Contains(line, "=") || strings.Contains(line, ":")) {
				return true
			}
		}
	}
	return false
}
