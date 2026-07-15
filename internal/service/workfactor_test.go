package service

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"filippo.io/age"
	"filippo.io/age/armor"
)

// The tests below guard the withWorkFactor / withCryptoWorkFactor knobs.
//
// Those options make the suite ~100x faster, and they would also silently gut
// key stretching if they ever leaked into a production path. Everything else
// here would still pass if that happened: the file would encrypt, decrypt, and
// round-trip perfectly — just with a passphrase an attacker could brute-force.
// So the default must be asserted explicitly.

// ageDefaultWorkFactor is the log2 scrypt work factor age uses when
// SetWorkFactor is not called, and what the age CLI writes.
const ageDefaultWorkFactor = 18

// stanzaCapture is an age.Identity that records the header stanzas and
// declines. It reads the work factor through public API rather than by parsing
// the file format by hand.
type stanzaCapture struct{ stanzas []*age.Stanza }

func (c *stanzaCapture) Unwrap(stanzas []*age.Stanza) ([]byte, error) {
	c.stanzas = stanzas
	return nil, age.ErrIncorrectIdentity
}

// scryptWorkFactor reads the work factor out of an age file's scrypt stanza.
func scryptWorkFactor(t *testing.T, ageFile []byte) int {
	t.Helper()

	// Handles both the armored identity file and binary user files.
	src, err := ageSource(bytes.NewReader(ageFile))
	if err != nil {
		t.Fatalf("ageSource: %v", err)
	}

	capture := &stanzaCapture{}
	_, _ = age.Decrypt(src, capture)

	for _, s := range capture.stanzas {
		if s.Type != "scrypt" {
			continue
		}
		if len(s.Args) != 2 {
			t.Fatalf("scrypt stanza has %d args, want 2", len(s.Args))
		}
		n, err := strconv.Atoi(s.Args[1])
		if err != nil {
			t.Fatalf("work factor %q is not a number: %v", s.Args[1], err)
		}
		return n
	}
	t.Fatal("no scrypt stanza found")
	return 0
}

// A KeyService built the way production builds it must stretch at age's full
// default, no matter that the tests elsewhere turn it down.
func TestKeyService_ProductionUsesStrongWorkFactor(t *testing.T) {
	store := &fakeIdentityStore{}
	svc := NewKeyService(store) // no options: exactly what main.go does

	if _, err := svc.Generate([]byte("passphrase")); err != nil {
		t.Fatal(err)
	}

	got := scryptWorkFactor(t, store.blob)
	if got != ageDefaultWorkFactor {
		t.Errorf("identity file scrypt work factor = %d, want %d — key stretching has been weakened", got, ageDefaultWorkFactor)
	}
}

// Same guard for passphrase-encrypted user files.
func TestCryptoService_ProductionUsesStrongWorkFactor(t *testing.T) {
	keys := NewKeyService(&fakeIdentityStore{}, withWorkFactor(testWorkFactor))
	crypto := NewCryptoService(keys) // no options: exactly what main.go does

	dir := t.TempDir()
	in := writeTemp(t, dir, "a.txt", []byte("content"))
	enc := filepath.Join(dir, "a.age")

	if err := crypto.EncryptFilePassphrase(t.Context(), in, enc, []byte("pw"), Refuse, nil); err != nil {
		t.Fatal(err)
	}
	blob, err := os.ReadFile(enc)
	if err != nil {
		t.Fatal(err)
	}

	got := scryptWorkFactor(t, blob)
	if got != ageDefaultWorkFactor {
		t.Errorf("passphrase file scrypt work factor = %d, want %d — key stretching has been weakened", got, ageDefaultWorkFactor)
	}
}

// Proves the knob is real, so the two tests above cannot pass vacuously.
func TestWorkFactorOptionApplies(t *testing.T) {
	store := &fakeIdentityStore{}
	svc := NewKeyService(store, withWorkFactor(testWorkFactor))

	if _, err := svc.Generate([]byte("passphrase")); err != nil {
		t.Fatal(err)
	}

	if got := scryptWorkFactor(t, store.blob); got != testWorkFactor {
		t.Errorf("work factor = %d, want %d", got, testWorkFactor)
	}
}

// The identity file must stay armored: it is what makes `age -d identity.age`
// work and keeps the user's key portable out of this app.
func TestKeyService_IdentityFileIsArmored(t *testing.T) {
	store := &fakeIdentityStore{}
	svc := NewKeyService(store, withWorkFactor(testWorkFactor))
	if _, err := svc.Generate([]byte("passphrase")); err != nil {
		t.Fatal(err)
	}

	if !bytes.HasPrefix(store.blob, []byte(armor.Header)) {
		t.Errorf("identity file does not start with %q", armor.Header)
	}
}
