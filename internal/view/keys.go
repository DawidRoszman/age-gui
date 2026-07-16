package view

import (
	"dawidroszman.eu/encryptor/internal/service"
)

// defaultPublicKeyFilename is offered in the export dialog. The ".pub" ending
// signals to the recipient that this is the safe half of the pair.
const defaultPublicKeyFilename = "my-age-public-key.pub"

// defaultBackupFilename is offered in the backup dialog. Named so that a user
// finding it in a year knows what it is without opening it.
const defaultBackupFilename = "encryptor-key-backup.age"

// Keys is the Wails-bound handler for identity operations.
//
// Every method returns a result envelope rather than (T, error): Wails hands JS
// only an error's string, and this UI must branch on the specific outcome.
//
// Note on passphrases: they arrive here as Go strings, having crossed the JS
// bridge. Go strings are immutable, so those bytes cannot be wiped and may
// persist until GC. This is inherent to a webview app and is documented in the
// README rather than pretended away. We convert to []byte at the boundary and
// never log or return them.
type Keys struct {
	keys     *service.KeyService
	platform Platform
}

// NewKeys builds the handler.
func NewKeys(keys *service.KeyService, platform Platform) *Keys {
	return &Keys{keys: keys, platform: platform}
}

// Status reports whether a key exists and whether it is unlocked.
func (h *Keys) Status() KeyStatusResult {
	st, err := h.keys.Status()
	if err != nil {
		return KeyStatusResult{Error: mapError(err)}
	}
	return KeyStatusResult{Status: keyStatusDTO(st)}
}

// Generate creates the user's keypair. Used by the first-run screen.
func (h *Keys) Generate(passphrase string) KeyStatusResult {
	if _, err := h.keys.Generate([]byte(passphrase)); err != nil {
		return KeyStatusResult{Error: mapError(err)}
	}
	return h.Status()
}

// Unlock decrypts the stored key for this session.
func (h *Keys) Unlock(passphrase string) KeyStatusResult {
	if err := h.keys.Unlock([]byte(passphrase)); err != nil {
		return KeyStatusResult{Error: mapError(err)}
	}
	return h.Status()
}

// Lock forgets the private key until the passphrase is entered again.
func (h *Keys) Lock() KeyStatusResult {
	h.keys.Lock()
	return h.Status()
}

// CopyPublicKey puts the user's public key on the clipboard.
//
// This is the primary way to share a key: post-quantum recipients are ~2000
// characters, so retyping is not an option and the clipboard is the path of
// least resistance.
func (h *Keys) CopyPublicKey() VoidResult {
	pub, err := h.keys.PublicKey()
	if err != nil {
		return VoidResult{Error: mapError(err)}
	}
	if err := h.platform.SetClipboard(pub.String()); err != nil {
		return VoidResult{Error: mapError(err)}
	}
	return VoidResult{}
}

// ExportPublicKey asks where to save the public key and writes it there.
//
// Returns an empty Value when the user cancels, which is a normal outcome and
// deliberately not an error.
func (h *Keys) ExportPublicKey() StringResult {
	// Fail before showing a dialog if there is nothing to export.
	if _, err := h.keys.PublicKey(); err != nil {
		return StringResult{Error: mapError(err)}
	}

	path, err := h.platform.SaveFileDialog("Save your public key", defaultPublicKeyFilename)
	if err != nil {
		return StringResult{Error: mapError(err)}
	}
	if path == "" {
		return StringResult{} // cancelled
	}

	// Replace: the save dialog already asked before returning an existing path,
	// so refusing here would contradict the answer the user just gave.
	if err := h.keys.ExportPublicKey(path, service.Replace); err != nil {
		return StringResult{Error: mapError(err)}
	}
	return StringResult{Value: path}
}

// Backup writes an encrypted copy of the user's key to a chosen location.
//
// Returns an empty Value when cancelled.
func (h *Keys) Backup() StringResult {
	path, err := h.platform.SaveFileDialog("Back up your key", defaultBackupFilename)
	if err != nil {
		return StringResult{Error: mapError(err)}
	}
	if path == "" {
		return StringResult{} // cancelled
	}

	if err := h.keys.ExportIdentity(path, service.Replace); err != nil {
		return StringResult{Error: mapError(err)}
	}
	return StringResult{Value: path}
}

// Restore adopts a key from a backup file, or from a plaintext identity file
// written by age-keygen.
//
// The file is chosen here rather than passed in, so the frontend never handles
// a path it did not get from the OS.
func (h *Keys) Restore(passphrase string) KeyStatusResult {
	path, err := h.platform.OpenFileDialog("Choose your key backup")
	if err != nil {
		return KeyStatusResult{Error: mapError(err)}
	}
	if path == "" {
		return KeyStatusResult{} // cancelled; Status.Exists stays false
	}

	if _, err := h.keys.RestoreIdentity(path, []byte(passphrase)); err != nil {
		return KeyStatusResult{Error: mapError(err)}
	}
	return h.Status()
}

// Touch records user activity, resetting the idle auto-lock countdown.
//
// The frontend calls this because only it can see the user typing and clicking;
// Go owns the clock and the decision, so a wedged UI cannot keep the key
// unlocked forever by simply never reporting idleness.
func (h *Keys) Touch() {
	h.keys.Touch()
}
