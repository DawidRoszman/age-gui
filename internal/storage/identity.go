package storage

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Identity persists the encrypted identity blob.
//
// This store is deliberately dumb: it moves opaque bytes and never inspects,
// decrypts, or understands them. All crypto lives in the service layer, so the
// only thing that ever touches the disk is ciphertext.
type Identity struct {
	path string
}

// NewIdentity returns a store rooted at dir.
func NewIdentity(dir string) *Identity {
	return &Identity{path: filepath.Join(dir, identityFile)}
}

// Path reports the identity file location, for display and for the docs that
// tell users what to back up.
func (s *Identity) Path() string { return s.path }

// Exists reports whether an identity has been created yet.
func (s *Identity) Exists() (bool, error) {
	_, err := os.Stat(s.path)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	default:
		return false, fmt.Errorf("stat identity file: %w", err)
	}
}

// Load returns the encrypted identity blob.
func (s *Identity) Load() ([]byte, error) {
	b, err := os.ReadFile(s.path)
	if err != nil {
		return nil, fmt.Errorf("read identity file: %w", err)
	}
	return b, nil
}

// Save atomically writes the encrypted identity blob.
func (s *Identity) Save(ciphertext []byte) error {
	return writeFileAtomic(s.path, ciphertext, secretPerm)
}
