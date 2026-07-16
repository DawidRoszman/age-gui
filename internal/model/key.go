// Package model holds the pure domain types for Encryptor.
//
// This package knows nothing about storage, the Wails runtime, or the UI.
// It imports filippo.io/age solely to parse key encodings: age recipients are
// the vocabulary of this domain, not an infrastructure detail we might swap.
//
// Types here follow "parse, don't validate" — a PublicKey can only be
// constructed through ParsePublicKey, so holding one proves it is well formed.
package model

import (
	"fmt"
	"strings"

	"filippo.io/age"
)

// KeyType identifies an age key encoding.
type KeyType string

const (
	// KeyTypeHybridPQ is the X25519 + ML-KEM-768 hybrid key, quantum-resistant
	// and what this app generates. Recipients encode as "age1pq1…" (~2000
	// chars), identities as "AGE-SECRET-KEY-PQ-1…".
	KeyTypeHybridPQ KeyType = "hybrid-pq"

	// KeyTypeX25519 is the classic key: "age1…" (62 chars) and
	// "AGE-SECRET-KEY-1…". We never generate these, but contacts may well use
	// them, so we always accept them on import.
	KeyTypeX25519 KeyType = "x25519"
)

// Key encoding prefixes, matching filippo.io/age's own parse.go routing.
const (
	prefixHybridRecipient = "age1pq1"
	prefixX25519Recipient = "age1"
	prefixX25519Secret    = "AGE-SECRET-KEY-1"
	prefixHybridSecret    = "AGE-SECRET-KEY-PQ-1"
)

// PublicKey is an age recipient proven valid at construction.
//
// The fields are unexported so that the only way to obtain one is
// ParsePublicKey; there is no such thing as an invalid non-zero PublicKey.
type PublicKey struct {
	value     string
	kind      KeyType
	recipient age.Recipient
}

// ParsePublicKey parses an age recipient encoding.
//
// It deliberately accepts BOTH hybrid and classic keys: we generate hybrid
// keys, but a contact's key belongs to someone else and may be either. Being
// strict here would make this app unable to encrypt to most of the age
// ecosystem.
//
// Surrounding whitespace is tolerated because these strings arrive by
// copy-paste, which routinely carries a trailing newline.
func ParsePublicKey(s string) (PublicKey, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return PublicKey{}, fmt.Errorf("public key is empty")
	}

	// Catch a pasted private key before anything else so we can tell the user
	// plainly to stop sharing it, rather than emitting "unknown recipient type".
	if isSecretKey(s) {
		return PublicKey{}, ErrSecretKeyGiven
	}

	// Order matters: hybrid keys also start with "age1", so the longer prefix
	// must be tested first. This mirrors parseRecipient in filippo.io/age.
	switch {
	case strings.HasPrefix(s, prefixHybridRecipient):
		r, err := age.ParseHybridRecipient(s)
		if err != nil {
			return PublicKey{}, fmt.Errorf("invalid post-quantum public key: %w", err)
		}
		return PublicKey{value: s, kind: KeyTypeHybridPQ, recipient: r}, nil

	case strings.HasPrefix(s, prefixX25519Recipient):
		r, err := age.ParseX25519Recipient(s)
		if err != nil {
			return PublicKey{}, fmt.Errorf("invalid public key: %w", err)
		}
		return PublicKey{value: s, kind: KeyTypeX25519, recipient: r}, nil

	default:
		return PublicKey{}, fmt.Errorf("not an age public key: it should start with %q", prefixX25519Recipient)
	}
}

// isSecretKey reports whether s looks like an age private key. age encodes
// these in uppercase bech32, but we fold case so a lowercased paste is still
// recognised as the mistake it is.
func isSecretKey(s string) bool {
	u := strings.ToUpper(s)
	return strings.HasPrefix(u, prefixX25519Secret) || strings.HasPrefix(u, prefixHybridSecret)
}

// String returns the canonical recipient encoding.
func (k PublicKey) String() string { return k.value }

// Type reports which key encoding this is.
func (k PublicKey) Type() KeyType { return k.kind }

// Recipient exposes the underlying age recipient for encryption.
func (k PublicKey) Recipient() age.Recipient { return k.recipient }

// IsZero reports whether this is the unset zero value.
func (k PublicKey) IsZero() bool { return k.value == "" }

// Equal compares two keys by their encoding.
func (k PublicKey) Equal(other PublicKey) bool { return k.value == other.value }

// Abbrev returns a short display form. Hybrid keys run to roughly 2000
// characters, so they must never be rendered raw in a list.
func (k PublicKey) Abbrev() string {
	const (
		head = 12
		tail = 6
	)
	if len(k.value) <= head+tail+1 {
		return k.value
	}
	return k.value[:head] + "…" + k.value[len(k.value)-tail:]
}

// MarshalText implements encoding.TextMarshaler, which also gives us JSON
// encoding for free despite the unexported fields.
func (k PublicKey) MarshalText() ([]byte, error) {
	return []byte(k.value), nil
}

// UnmarshalText implements encoding.TextUnmarshaler, routing through
// ParsePublicKey so a key loaded from disk is validated exactly like one typed
// by the user. A corrupted contacts file therefore fails loudly instead of
// yielding a Contact that cannot encrypt.
func (k *PublicKey) UnmarshalText(b []byte) error {
	parsed, err := ParsePublicKey(string(b))
	if err != nil {
		return err
	}
	*k = parsed
	return nil
}
