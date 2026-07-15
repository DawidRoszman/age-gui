// Package view exposes the services to the UI.
//
// It is the only layer that knows a GUI exists. Handlers here are deliberately
// thin: they convert between DTOs and domain types, translate domain errors
// into codes the UI can branch on, and nothing else. Any logic that appears
// here belongs in a service.
//
// Nothing in this package ever puts a private key or a passphrase into a DTO.
// DTOs travel to the webview, and the webview is the one place key material
// must never reach.
package view

import (
	"time"

	"dawidroszman.eu/age-gui/internal/model"
	"dawidroszman.eu/age-gui/internal/service"
)

// Error is a machine-readable failure for the UI.
//
// Wails delivers a returned Go error to JS as a bare string, which would force
// the frontend to match on prose. Since the UI must branch on outcomes — prompt
// for a passphrase, offer to unlock, warn before overwriting — every handler
// returns a result envelope carrying a stable Code instead.
type Error struct {
	// Code is stable and safe to switch on.
	Code string `json:"code"`
	// Message is human-readable text, already suitable for display.
	Message string `json:"message"`
	// Recoverable marks errors the user can resolve by acting, as opposed to
	// bugs. The UI shows these calmly rather than as a crash.
	Recoverable bool `json:"recoverable"`
}

// KeyStatusDTO describes the identity state.
type KeyStatusDTO struct {
	Exists    bool   `json:"exists"`
	Unlocked  bool   `json:"unlocked"`
	PublicKey string `json:"publicKey"`
	// Abbrev is the display form. Post-quantum keys are ~2000 characters, so
	// the UI must never lay out the full value in a list.
	Abbrev  string `json:"abbrev"`
	KeyType string `json:"keyType"`
}

// ContactDTO is a contact as the UI sees it.
type ContactDTO struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	PublicKey string `json:"publicKey"`
	Abbrev    string `json:"abbrev"`
	KeyType   string `json:"keyType"`
	Note      string `json:"note"`
	AddedAt   string `json:"addedAt"`
}

// Result envelopes. One per return shape, so the generated TypeScript stays
// typed rather than collapsing to `any`.

// KeyStatusResult wraps a key status.
type KeyStatusResult struct {
	Status KeyStatusDTO `json:"status"`
	Error  *Error       `json:"error,omitempty"`
}

// ContactsResult wraps a contact list.
type ContactsResult struct {
	Contacts []ContactDTO `json:"contacts"`
	Error    *Error       `json:"error,omitempty"`
}

// ContactResult wraps a single contact.
type ContactResult struct {
	Contact ContactDTO `json:"contact"`
	Error   *Error     `json:"error,omitempty"`
}

// VoidResult reports success or failure with no payload.
type VoidResult struct {
	Error *Error `json:"error,omitempty"`
}

// StringResult wraps a single string.
type StringResult struct {
	Value string `json:"value"`
	Error *Error `json:"error,omitempty"`
}

// FileKindResult reports whether a file needs a passphrase or a key.
type FileKindResult struct {
	// Kind is "passphrase" or "recipients".
	Kind string `json:"kind"`
	// Path echoes the inspected file so the UI can correlate.
	Path  string `json:"path"`
	Error *Error `json:"error,omitempty"`
}

// PathsResult wraps a list of file paths.
type PathsResult struct {
	Paths []string `json:"paths"`
	Error *Error   `json:"error,omitempty"`
}

// ProgressEvent is emitted during encryption and decryption.
type ProgressEvent struct {
	// JobID correlates events with the operation that started them.
	JobID string `json:"jobId"`
	Done  int64  `json:"done"`
	Total int64  `json:"total"`
	// Percent is precomputed so the UI never divides by an unknown total.
	Percent float64 `json:"percent"`
}

// keyStatusDTO converts a service status.
func keyStatusDTO(s service.KeyStatus) KeyStatusDTO {
	dto := KeyStatusDTO{Exists: s.Exists, Unlocked: s.Unlocked}
	if !s.PublicKey.IsZero() {
		dto.PublicKey = s.PublicKey.String()
		dto.Abbrev = s.PublicKey.Abbrev()
		dto.KeyType = string(s.PublicKey.Type())
	}
	return dto
}

// contactDTO converts a domain contact.
func contactDTO(c model.Contact) ContactDTO {
	return ContactDTO{
		ID:        c.ID,
		Name:      c.Name,
		PublicKey: c.PublicKey.String(),
		Abbrev:    c.PublicKey.Abbrev(),
		KeyType:   string(c.PublicKey.Type()),
		Note:      c.Note,
		AddedAt:   c.AddedAt.Format(time.RFC3339),
	}
}

// contactDTOs converts a slice of domain contacts.
func contactDTOs(cs []model.Contact) []ContactDTO {
	// Never nil: JSON null would make the frontend guard every iteration.
	out := make([]ContactDTO, 0, len(cs))
	for _, c := range cs {
		out = append(out, contactDTO(c))
	}
	return out
}
