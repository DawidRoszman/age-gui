package model

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func testKey(t *testing.T) PublicKey {
	t.Helper()
	k, err := ParsePublicKey(x25519Key(t))
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func TestNewContact(t *testing.T) {
	key := testKey(t)
	before := time.Now().UTC()

	c, err := NewContact("  Alice  ", key, "  work laptop  ")
	if err != nil {
		t.Fatalf("NewContact: %v", err)
	}

	if c.Name != "Alice" {
		t.Errorf("Name = %q, want %q (should trim)", c.Name, "Alice")
	}
	if c.Note != "work laptop" {
		t.Errorf("Note = %q, want trimmed", c.Note)
	}
	if c.ID == "" {
		t.Error("ID is empty")
	}
	if c.AddedAt.Before(before) {
		t.Error("AddedAt is before the call")
	}
	if c.AddedAt.Location() != time.UTC {
		t.Error("AddedAt should be UTC so the file is timezone-stable")
	}
}

func TestNewContact_IDsAreUnique(t *testing.T) {
	key := testKey(t)
	seen := make(map[string]bool)

	for range 100 {
		c, err := NewContact("Alice", key, "")
		if err != nil {
			t.Fatal(err)
		}
		if seen[c.ID] {
			t.Fatalf("duplicate contact ID %q", c.ID)
		}
		seen[c.ID] = true
	}
}

func TestNewContact_Rejects(t *testing.T) {
	key := testKey(t)

	t.Run("empty name", func(t *testing.T) {
		if _, err := NewContact("", key, ""); err == nil {
			t.Error("want error for empty name")
		}
	})
	t.Run("whitespace name", func(t *testing.T) {
		if _, err := NewContact("   ", key, ""); err == nil {
			t.Error("want error for whitespace-only name")
		}
	})
	t.Run("overlong name", func(t *testing.T) {
		if _, err := NewContact(strings.Repeat("a", MaxContactNameLen+1), key, ""); err == nil {
			t.Error("want error for overlong name")
		}
	})
	t.Run("zero key", func(t *testing.T) {
		if _, err := NewContact("Alice", PublicKey{}, ""); err == nil {
			t.Error("want error for missing public key")
		}
	})
}

func TestContact_JSONRoundTrip(t *testing.T) {
	orig, err := NewContact("Alice", testKey(t), "note")
	if err != nil {
		t.Fatal(err)
	}

	b, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var back Contact
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if back.ID != orig.ID || back.Name != orig.Name || back.Note != orig.Note {
		t.Error("scalar fields did not round-trip")
	}
	if !back.PublicKey.Equal(orig.PublicKey) {
		t.Error("public key did not round-trip")
	}
	if !back.AddedAt.Equal(orig.AddedAt) {
		t.Error("AddedAt did not round-trip")
	}
}

// Contacts are stored unencrypted, so it matters that the serialised form can
// never carry a secret.
func TestContact_JSONHasNoSecrets(t *testing.T) {
	c, err := NewContact("Alice", testKey(t), "note")
	if err != nil {
		t.Fatal(err)
	}

	b, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToUpper(string(b)), "AGE-SECRET-KEY") {
		t.Fatalf("serialised contact contains a private key: %s", b)
	}
}
