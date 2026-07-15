package view

import (
	"context"
	"path/filepath"
	"sync"

	"dawidroszman.eu/age-gui/internal/service"
)

// EventProgress is emitted as an operation runs. The frontend subscribes to it
// and filters by JobID.
const EventProgress = "crypto:progress"

// Crypto is the Wails-bound handler for encryption and decryption.
//
// File contents never appear in this API: every method takes and returns
// *paths*. Wails hands us absolute paths from both drag-drop and the file
// dialogs, so bytes stream inside Go and never cross the JS bridge — which is
// what lets this handle multi-gigabyte files and keeps plaintext out of the
// webview.
type Crypto struct {
	crypto   *service.CryptoService
	contacts *service.ContactService
	platform Platform

	// mu guards jobs. Wails dispatches each JS call on its own goroutine, so a
	// Cancel can land while an Encrypt is still registering.
	mu   sync.Mutex
	jobs map[string]context.CancelFunc
}

// NewCrypto builds the handler.
func NewCrypto(crypto *service.CryptoService, contacts *service.ContactService, platform Platform) *Crypto {
	return &Crypto{
		crypto:   crypto,
		contacts: contacts,
		platform: platform,
		jobs:     make(map[string]context.CancelFunc),
	}
}

// PickFiles opens a file chooser. The keyboard-only path to the same place
// drag-and-drop reaches with a mouse.
func (h *Crypto) PickFiles(title string) PathsResult {
	paths, err := h.platform.OpenFilesDialog(title)
	if err != nil {
		return PathsResult{Paths: []string{}, Error: mapError(err)}
	}
	if paths == nil {
		paths = []string{}
	}
	return PathsResult{Paths: paths}
}

// ChooseSavePath asks the user where to write output.
//
// The OS dialog asks about replacing an existing file itself, so a path
// returned from here has already been confirmed and callers pass it straight
// back to Encrypt or Decrypt.
func (h *Crypto) ChooseSavePath(title, defaultName string) StringResult {
	path, err := h.platform.SaveFileDialog(title, defaultName)
	if err != nil {
		return StringResult{Error: mapError(err)}
	}
	return StringResult{Value: path} // "" when cancelled
}

// SuggestEncryptOutput returns the default output path for encrypting in.
func (h *Crypto) SuggestEncryptOutput(in string) StringResult {
	return StringResult{Value: service.EncryptedName(in)}
}

// SuggestDecryptOutput returns the default output path for decrypting in.
func (h *Crypto) SuggestDecryptOutput(in string) StringResult {
	return StringResult{Value: service.DecryptedName(in)}
}

// Inspect reports whether a file needs a passphrase or a key, so the UI can
// prompt for the right thing. Safe to call while locked.
func (h *Crypto) Inspect(path string) FileKindResult {
	kind, err := h.crypto.Inspect(path)
	if err != nil {
		return FileKindResult{Path: path, Error: mapError(err)}
	}
	return FileKindResult{Kind: string(kind), Path: path}
}

// Encrypt encrypts in for the given contacts and returns the output path.
//
// An empty out means "use the default name beside the input", which refuses to
// overwrite. A non-empty out comes from ChooseSavePath, where the user already
// confirmed any replacement.
func (h *Crypto) Encrypt(jobID, in, out string, contactIDs []string) StringResult {
	keys, err := h.contacts.Recipients(contactIDs)
	if err != nil {
		return StringResult{Error: mapError(err)}
	}

	out, mode := h.resolveOutput(in, out, service.EncryptedName)
	ctx, done := h.begin(jobID)
	defer done()

	if err := h.crypto.EncryptFile(ctx, in, out, keys, mode, h.progress(jobID)); err != nil {
		return StringResult{Error: mapError(err)}
	}
	return StringResult{Value: out}
}

// EncryptWithPassphrase encrypts in under a passphrase.
func (h *Crypto) EncryptWithPassphrase(jobID, in, out, passphrase string) StringResult {
	out, mode := h.resolveOutput(in, out, service.EncryptedName)
	ctx, done := h.begin(jobID)
	defer done()

	if err := h.crypto.EncryptFilePassphrase(ctx, in, out, []byte(passphrase), mode, h.progress(jobID)); err != nil {
		return StringResult{Error: mapError(err)}
	}
	return StringResult{Value: out}
}

// Decrypt decrypts in with the unlocked key.
func (h *Crypto) Decrypt(jobID, in, out string) StringResult {
	out, mode := h.resolveOutput(in, out, service.DecryptedName)
	ctx, done := h.begin(jobID)
	defer done()

	if err := h.crypto.DecryptFile(ctx, in, out, mode, h.progress(jobID)); err != nil {
		return StringResult{Error: mapError(err)}
	}
	return StringResult{Value: out}
}

// DecryptWithPassphrase decrypts a passphrase-protected file. Works while the
// app is locked, since no identity is involved.
func (h *Crypto) DecryptWithPassphrase(jobID, in, out, passphrase string) StringResult {
	out, mode := h.resolveOutput(in, out, service.DecryptedName)
	ctx, done := h.begin(jobID)
	defer done()

	if err := h.crypto.DecryptFilePassphrase(ctx, in, out, []byte(passphrase), mode, h.progress(jobID)); err != nil {
		return StringResult{Error: mapError(err)}
	}
	return StringResult{Value: out}
}

// Cancel stops a running operation. Unknown IDs are ignored: the job may have
// finished between the user's click and this call, which is not an error.
func (h *Crypto) Cancel(jobID string) VoidResult {
	h.mu.Lock()
	cancel, ok := h.jobs[jobID]
	h.mu.Unlock()
	if ok {
		cancel()
	}
	return VoidResult{}
}

// BaseName returns the file name for display, so the UI never has to parse
// paths itself and get separators wrong across platforms.
func (h *Crypto) BaseName(path string) StringResult {
	return StringResult{Value: filepath.Base(path)}
}

// resolveOutput picks the output path and the overwrite policy.
func (h *Crypto) resolveOutput(in, out string, suggest func(string) string) (string, service.Overwrite) {
	if out == "" {
		// Automatic path: the user has not seen it, so never overwrite.
		return suggest(in), service.Refuse
	}
	// Came from the save dialog, which already asked.
	return out, service.Replace
}

// begin registers a cancellable job and returns a cleanup func.
func (h *Crypto) begin(jobID string) (context.Context, func()) {
	ctx, cancel := context.WithCancel(context.Background())

	h.mu.Lock()
	h.jobs[jobID] = cancel
	h.mu.Unlock()

	return ctx, func() {
		h.mu.Lock()
		delete(h.jobs, jobID)
		h.mu.Unlock()
		// Always cancel: releases the context's resources whether the
		// operation finished or failed.
		cancel()
	}
}

// progress returns a callback that forwards progress to the frontend.
func (h *Crypto) progress(jobID string) service.Progress {
	return func(done, total int64) {
		var pct float64
		if total > 0 {
			pct = float64(done) / float64(total) * 100
		}
		h.platform.EmitEvent(EventProgress, ProgressEvent{
			JobID:   jobID,
			Done:    done,
			Total:   total,
			Percent: pct,
		})
	}
}
