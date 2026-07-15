package storage

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

const (
	// appDir is our subdirectory inside the OS config directory.
	appDir = "age-gui"

	// identityFile holds the scrypt-encrypted, armored age identity.
	identityFile = "identity.age"

	// contactsFile holds public keys only.
	contactsFile = "contacts.json"

	// settingsFile holds user preferences. No secrets.
	settingsFile = "settings.json"

	// dirPerm keeps the config directory owner-only.
	dirPerm fs.FileMode = 0o700

	// secretPerm is for anything key-related.
	secretPerm fs.FileMode = 0o600

	// dataPerm is for non-secret data. Contacts hold no secrets, but they are
	// still private metadata: who the user talks to is worth protecting.
	dataPerm fs.FileMode = 0o600
)

// DefaultDir returns the per-user config directory for the app, creating it if
// needed.
//
// os.UserConfigDir is what makes this multiplatform without any build tags:
// ~/.config on Linux, %AppData% on Windows, ~/Library/Application Support on
// macOS.
//
// Note the permission bits are POSIX; on Windows they are approximated by the
// runtime and inherited ACLs govern real access. We set them regardless because
// they are correct where they are enforced, but we do not rely on them as the
// only protection — the identity file is encrypted at rest either way.
func DefaultDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate user config directory: %w", err)
	}
	dir := filepath.Join(base, appDir)
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return "", fmt.Errorf("create config directory %s: %w", dir, err)
	}
	return dir, nil
}
