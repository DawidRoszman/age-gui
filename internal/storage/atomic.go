// Package storage implements the persistence ports declared by the service
// layer, backed by files under the user's OS config directory.
//
// Every write goes through writeFileAtomic. This is not incidental: the
// identity file is the only copy of the user's private key, and a crash or a
// full disk partway through a naive truncate-and-write would destroy it
// permanently, taking every file they ever encrypted with it.
package storage

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// writeFileAtomic writes data to path so that a concurrent or interrupted
// write can never leave a truncated file behind.
//
// It writes to a temporary file in the same directory (same filesystem, so the
// rename cannot fail with EXDEV), fsyncs it, then renames over the target.
// Rename is atomic on POSIX and, via MoveFileEx, on Windows.
func writeFileAtomic(path string, data []byte, perm fs.FileMode) (err error) {
	dir := filepath.Dir(path)

	// CreateTemp makes the file 0600, so the contents are never briefly
	// world-readable even before the chmod below.
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()

	// On any failure past this point the temp file must not survive; a stray
	// ".identity.age.tmp-123" containing key material would be exactly the
	// leak this package exists to prevent.
	defer func() {
		if err != nil {
			tmp.Close()
			os.Remove(tmpName)
		}
	}()

	if _, err = tmp.Write(data); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}
	// Sync before rename: rename only orders the directory entry, it does not
	// flush the data blocks. Without this a power loss can yield a
	// correctly-named, zero-length key file.
	if err = tmp.Sync(); err != nil {
		return fmt.Errorf("sync temp file: %w", err)
	}
	if err = tmp.Chmod(perm); err != nil {
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err = os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename into place: %w", err)
	}

	syncDir(dir)
	return nil
}

// syncDir flushes a directory entry so the rename survives a power loss.
//
// Best effort by design: opening a directory is not permitted on Windows, and
// the rename is already atomic everywhere. This only upgrades durability on
// Unix, so a failure here is not worth failing an otherwise complete write.
func syncDir(dir string) {
	d, err := os.Open(dir)
	if err != nil {
		return
	}
	defer d.Close()
	_ = d.Sync()
}
