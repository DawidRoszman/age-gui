package model

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"filippo.io/age"
)

// generated once so the tests use real, well-formed keys rather than fixtures
// that could drift from what the age library actually emits.
func hybridKey(t *testing.T) string {
	t.Helper()
	id, err := age.GenerateHybridIdentity()
	if err != nil {
		t.Fatalf("GenerateHybridIdentity: %v", err)
	}
	return id.Recipient().String()
}

func x25519Key(t *testing.T) string {
	t.Helper()
	id, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatalf("GenerateX25519Identity: %v", err)
	}
	return id.Recipient().String()
}

func TestParsePublicKey_AcceptsHybrid(t *testing.T) {
	s := hybridKey(t)

	k, err := ParsePublicKey(s)
	if err != nil {
		t.Fatalf("ParsePublicKey(hybrid) = %v, want nil", err)
	}
	if k.Type() != KeyTypeHybridPQ {
		t.Errorf("Type() = %q, want %q", k.Type(), KeyTypeHybridPQ)
	}
	if k.String() != s {
		t.Errorf("String() did not round-trip")
	}
	if k.Recipient() == nil {
		t.Error("Recipient() = nil, want a usable age.Recipient")
	}
}

// The core of "strict on generation, liberal on import": we only ever generate
// hybrid keys, but a contact's key is not ours to choose. Rejecting classic
// keys would cut us off from most of the age ecosystem.
func TestParsePublicKey_AcceptsX25519(t *testing.T) {
	s := x25519Key(t)

	k, err := ParsePublicKey(s)
	if err != nil {
		t.Fatalf("ParsePublicKey(x25519) = %v, want nil", err)
	}
	if k.Type() != KeyTypeX25519 {
		t.Errorf("Type() = %q, want %q", k.Type(), KeyTypeX25519)
	}
}

// Hybrid recipients also begin with "age1", so a naive prefix check in the
// wrong order would misroute them to the X25519 parser and reject valid keys.
func TestParsePublicKey_HybridIsNotMisreadAsX25519(t *testing.T) {
	s := hybridKey(t)
	if !strings.HasPrefix(s, "age1") {
		t.Fatalf("precondition failed: hybrid key %q does not start with age1", s[:8])
	}

	k, err := ParsePublicKey(s)
	if err != nil {
		t.Fatalf("ParsePublicKey: %v", err)
	}
	if k.Type() != KeyTypeHybridPQ {
		t.Fatalf("Type() = %q, want %q — prefix routing order is wrong", k.Type(), KeyTypeHybridPQ)
	}
}

func TestParsePublicKey_TrimsWhitespace(t *testing.T) {
	s := x25519Key(t)

	for _, in := range []string{" " + s, s + "\n", "\t" + s + "  \r\n"} {
		k, err := ParsePublicKey(in)
		if err != nil {
			t.Fatalf("ParsePublicKey(%q) = %v, want nil", in, err)
		}
		if k.String() != s {
			t.Errorf("ParsePublicKey(%q) did not trim to canonical form", in)
		}
	}
}

// Pasting a private key where a public key belongs is a plausible and
// dangerous mistake, so it gets a distinct error the UI can shout about.
func TestParsePublicKey_RejectsPrivateKey(t *testing.T) {
	x, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	h, err := age.GenerateHybridIdentity()
	if err != nil {
		t.Fatal(err)
	}

	for name, secret := range map[string]string{
		"x25519": x.String(),
		"hybrid": h.String(),
		"lower":  strings.ToLower(x.String()),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParsePublicKey(secret); !errors.Is(err, ErrSecretKeyGiven) {
				t.Errorf("ParsePublicKey(private) = %v, want ErrSecretKeyGiven", err)
			}
		})
	}
}

func TestParsePublicKey_Rejects(t *testing.T) {
	for name, in := range map[string]string{
		"empty":     "",
		"blank":     "   ",
		"garbage":   "hello world",
		"truncated": "age1qqqq",
		"ssh":       "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParsePublicKey(in); err == nil {
				t.Errorf("ParsePublicKey(%q) = nil, want error", in)
			}
		})
	}
}

func TestPublicKey_ZeroValue(t *testing.T) {
	var k PublicKey
	if !k.IsZero() {
		t.Error("zero PublicKey.IsZero() = false, want true")
	}

	parsed, err := ParsePublicKey(x25519Key(t))
	if err != nil {
		t.Fatal(err)
	}
	if parsed.IsZero() {
		t.Error("parsed PublicKey.IsZero() = true, want false")
	}
}

// A ~2000 character hybrid key must never reach the UI in full.
func TestPublicKey_Abbrev(t *testing.T) {
	k, err := ParsePublicKey(hybridKey(t))
	if err != nil {
		t.Fatal(err)
	}

	got := k.Abbrev()
	if len(got) > 32 {
		t.Errorf("Abbrev() = %d chars, want short enough to render in a list", len(got))
	}
	if !strings.HasPrefix(got, "age1pq1") {
		t.Errorf("Abbrev() = %q, want it to keep the recognisable prefix", got)
	}
}

func TestPublicKey_JSONRoundTrip(t *testing.T) {
	orig, err := ParsePublicKey(hybridKey(t))
	if err != nil {
		t.Fatal(err)
	}

	b, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var back PublicKey
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !back.Equal(orig) {
		t.Error("JSON round-trip changed the key")
	}
	// The recipient must survive too, or a loaded contact could not encrypt.
	if back.Recipient() == nil {
		t.Error("Recipient() = nil after round-trip; loaded contacts would be unusable")
	}
}

// A damaged contacts file must fail loudly rather than produce a Contact whose
// key silently cannot encrypt.
func TestPublicKey_UnmarshalRejectsGarbage(t *testing.T) {
	var k PublicKey
	if err := json.Unmarshal([]byte(`"not-a-key"`), &k); err == nil {
		t.Error("Unmarshal(garbage) = nil, want error")
	}
}
