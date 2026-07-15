package model

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// MaxContactNameLen bounds a contact name so a pathological value cannot break
// the UI or bloat the contacts file.
const MaxContactNameLen = 128

// Contact is somebody the user can encrypt to: a human-readable name bound to
// their age public key.
//
// A Contact holds no secrets, which is what makes contacts.json safe to store
// unencrypted and safe to back up.
type Contact struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	PublicKey PublicKey `json:"publicKey"`
	Note      string    `json:"note,omitempty"`
	AddedAt   time.Time `json:"addedAt"`
}

// NewContact validates and builds a Contact, assigning it a fresh id.
func NewContact(name string, key PublicKey, note string) (Contact, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Contact{}, fmt.Errorf("contact name must not be empty")
	}
	if len(name) > MaxContactNameLen {
		return Contact{}, fmt.Errorf("contact name must be at most %d characters", MaxContactNameLen)
	}
	if key.IsZero() {
		return Contact{}, fmt.Errorf("contact must have a public key")
	}

	id, err := newID()
	if err != nil {
		return Contact{}, err
	}
	return Contact{
		ID:        id,
		Name:      name,
		PublicKey: key,
		Note:      strings.TrimSpace(note),
		AddedAt:   time.Now().UTC(),
	}, nil
}

// newID returns a random 128-bit identifier. Contacts are local-only, so a
// random hex string is sufficient and avoids a UUID dependency.
func newID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("failed to generate contact id: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}
