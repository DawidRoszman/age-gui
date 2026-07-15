package service

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dawidroszman.eu/age-gui/internal/model"
)

// testWorkFactor keeps scrypt honest but fast. Production uses age's default
// (18); the algorithm and code path are identical either way, so nothing about
// correctness goes untested — only the deliberate slowness is skipped.
const testWorkFactor = 10

func newKeyService() (*KeyService, *fakeIdentityStore) {
	store := &fakeIdentityStore{}
	return NewKeyService(store, withWorkFactor(testWorkFactor)), store
}

func TestKeyService_GenerateProducesHybridKey(t *testing.T) {
	svc, _ := newKeyService()

	pub, err := svc.Generate([]byte("correct horse battery staple"))
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	// "Strict on generation": we only ever mint post-quantum keys.
	if pub.Type() != model.KeyTypeHybridPQ {
		t.Errorf("generated key type = %q, want %q", pub.Type(), model.KeyTypeHybridPQ)
	}
	if !strings.HasPrefix(pub.String(), "age1pq1") {
		t.Errorf("generated key %q does not look post-quantum", pub.Abbrev())
	}
}

// Generating leaves the key unlocked: the user just chose the passphrase, so
// making them retype it immediately would be pointless friction.
func TestKeyService_GenerateLeavesUnlocked(t *testing.T) {
	svc, _ := newKeyService()

	pub, err := svc.Generate([]byte("pass"))
	if err != nil {
		t.Fatal(err)
	}

	st, err := svc.Status()
	if err != nil {
		t.Fatal(err)
	}
	if !st.Exists || !st.Unlocked {
		t.Errorf("Status() = %+v, want Exists and Unlocked", st)
	}
	if !st.PublicKey.Equal(pub) {
		t.Error("Status().PublicKey does not match the generated key")
	}
}

// The single most destructive bug this app could have: silently replacing the
// user's key would orphan every file ever encrypted to them.
func TestKeyService_GenerateRefusesToOverwrite(t *testing.T) {
	svc, store := newKeyService()

	first, err := svc.Generate([]byte("pass"))
	if err != nil {
		t.Fatal(err)
	}
	blob := append([]byte(nil), store.blob...)

	_, err = svc.Generate([]byte("other pass"))
	if !errors.Is(err, model.ErrIdentityExists) {
		t.Fatalf("second Generate = %v, want ErrIdentityExists", err)
	}
	if string(store.blob) != string(blob) {
		t.Fatal("the stored identity was modified by a refused Generate")
	}

	// And the original key must still be the live one.
	pub, err := svc.PublicKey()
	if err != nil {
		t.Fatal(err)
	}
	if !pub.Equal(first) {
		t.Error("in-memory key changed after a refused Generate")
	}
}

func TestKeyService_UnlockRoundTrip(t *testing.T) {
	svc, store := newKeyService()
	pass := []byte("correct horse battery staple")

	pub, err := svc.Generate(pass)
	if err != nil {
		t.Fatal(err)
	}

	// A fresh service over the same bytes stands in for an app restart.
	restarted := NewKeyService(store)

	if _, err := restarted.PublicKey(); !errors.Is(err, model.ErrLocked) {
		t.Errorf("PublicKey() before unlock = %v, want ErrLocked", err)
	}
	if err := restarted.Unlock(pass); err != nil {
		t.Fatalf("Unlock: %v", err)
	}

	got, err := restarted.PublicKey()
	if err != nil {
		t.Fatal(err)
	}
	if !got.Equal(pub) {
		t.Error("key recovered after restart does not match the generated one")
	}
}

func TestKeyService_UnlockWrongPassphrase(t *testing.T) {
	svc, store := newKeyService()
	if _, err := svc.Generate([]byte("right")); err != nil {
		t.Fatal(err)
	}

	restarted := NewKeyService(store)
	err := restarted.Unlock([]byte("wrong"))
	if !errors.Is(err, model.ErrWrongPassphrase) {
		t.Fatalf("Unlock(wrong) = %v, want ErrWrongPassphrase", err)
	}
	// Must be distinguishable from a damaged file, or we would tell users
	// their key is broken when they merely typed the wrong passphrase.
	if errors.Is(err, model.ErrCorruptIdentity) {
		t.Error("a wrong passphrase must not be reported as a damaged key file")
	}
}

func TestKeyService_UnlockCorruptFile(t *testing.T) {
	store := &fakeIdentityStore{blob: []byte("this is not an age file")}
	svc := NewKeyService(store)

	err := svc.Unlock([]byte("pass"))
	if !errors.Is(err, model.ErrCorruptIdentity) {
		t.Errorf("Unlock(corrupt) = %v, want ErrCorruptIdentity", err)
	}
}

func TestKeyService_UnlockWithoutIdentity(t *testing.T) {
	svc, _ := newKeyService()

	if err := svc.Unlock([]byte("pass")); !errors.Is(err, model.ErrNoIdentity) {
		t.Errorf("Unlock with no identity = %v, want ErrNoIdentity", err)
	}
}

func TestKeyService_EmptyPassphraseRejected(t *testing.T) {
	svc, _ := newKeyService()

	if _, err := svc.Generate(nil); !errors.Is(err, model.ErrEmptyPassphrase) {
		t.Errorf("Generate(nil) = %v, want ErrEmptyPassphrase", err)
	}
	if err := svc.Unlock([]byte("")); !errors.Is(err, model.ErrEmptyPassphrase) {
		t.Errorf("Unlock(empty) = %v, want ErrEmptyPassphrase", err)
	}
}

func TestKeyService_Lock(t *testing.T) {
	svc, _ := newKeyService()
	if _, err := svc.Generate([]byte("pass")); err != nil {
		t.Fatal(err)
	}

	svc.Lock()

	if _, err := svc.PublicKey(); !errors.Is(err, model.ErrLocked) {
		t.Errorf("PublicKey() after Lock = %v, want ErrLocked", err)
	}
	if _, err := svc.Identities(); !errors.Is(err, model.ErrLocked) {
		t.Errorf("Identities() after Lock = %v, want ErrLocked", err)
	}

	st, err := svc.Status()
	if err != nil {
		t.Fatal(err)
	}
	// Locking discards the key but must not suggest it is gone from disk.
	if !st.Exists {
		t.Error("Status().Exists = false after Lock; the key is still on disk")
	}
	if st.Unlocked {
		t.Error("Status().Unlocked = true after Lock")
	}
}

// The identity file must be a normal age file, not a private format. Users must
// be able to recover their key with the stock CLI if this app disappears.
func TestKeyService_StoredIdentityIsArmoredAgeFile(t *testing.T) {
	svc, store := newKeyService()
	if _, err := svc.Generate([]byte("pass")); err != nil {
		t.Fatal(err)
	}

	blob := string(store.blob)
	if !strings.HasPrefix(blob, "-----BEGIN AGE ENCRYPTED FILE-----") {
		t.Errorf("stored identity is not an armored age file, starts with: %.40q", blob)
	}
	if strings.Contains(strings.ToUpper(blob), "AGE-SECRET-KEY") {
		t.Fatal("the private key appears in plaintext in the stored blob")
	}
}

func TestKeyService_ExportPublicKey(t *testing.T) {
	svc, _ := newKeyService()
	pub, err := svc.Generate([]byte("pass"))
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "alice.pub")
	if err := svc.ExportPublicKey(path, Refuse); err != nil {
		t.Fatalf("ExportPublicKey: %v", err)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(b)) != pub.String() {
		t.Error("exported file does not contain the public key")
	}
	if strings.Contains(strings.ToUpper(string(b)), "AGE-SECRET-KEY") {
		t.Fatal("exported public key file contains a private key")
	}
}

func TestKeyService_ExportRequiresUnlock(t *testing.T) {
	svc, _ := newKeyService()
	path := filepath.Join(t.TempDir(), "alice.pub")

	if err := svc.ExportPublicKey(path, Refuse); !errors.Is(err, model.ErrLocked) {
		t.Errorf("ExportPublicKey while locked = %v, want ErrLocked", err)
	}
}

func TestKeyService_ExportRefusesOverwrite(t *testing.T) {
	svc, _ := newKeyService()
	if _, err := svc.Generate([]byte("pass")); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "taken")
	if err := os.WriteFile(path, []byte("important"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := svc.ExportPublicKey(path, Refuse); !errors.Is(err, model.ErrTargetExists) {
		t.Errorf("ExportPublicKey over an existing file = %v, want ErrTargetExists", err)
	}
	b, _ := os.ReadFile(path)
	if string(b) != "important" {
		t.Error("ExportPublicKey clobbered an existing file")
	}
}
