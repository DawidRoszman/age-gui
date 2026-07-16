// Package service holds the application's use cases.
//
// It depends only on the model and on the ports declared below. It knows
// nothing about Wails, HTTP, or the filesystem layout, which is what lets every
// use case here be tested with in-memory fakes and no GUI.
package service

import "dawidroszman.eu/encryptor/internal/model"

// IdentityStore persists the encrypted identity blob.
//
// It deals in opaque ciphertext: encryption happens in KeyService, so a store
// implementation never sees key material and cannot leak it.
type IdentityStore interface {
	// Exists reports whether an identity has been saved.
	Exists() (bool, error)
	// Load returns the stored ciphertext.
	Load() ([]byte, error)
	// Save writes the ciphertext, replacing any previous one atomically.
	Save(ciphertext []byte) error
}

// SettingsStore persists user preferences.
type SettingsStore interface {
	// Load returns the stored settings, or the defaults when none are saved.
	Load() (model.Settings, error)
	// Save writes the settings.
	Save(s model.Settings) error
}

// ContactStore persists the address book.
type ContactStore interface {
	// List returns every contact, or an empty slice if there are none.
	List() ([]model.Contact, error)
	// Put inserts or replaces a contact by ID.
	Put(c model.Contact) error
	// Delete removes a contact by ID, returning model.ErrContactNotFound if
	// no such contact exists.
	Delete(id string) error
}
