package service

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"filippo.io/age"
	"filippo.io/age/armor"

	"dawidroszman.eu/encryptor/internal/model"
)

// newCryptoFixture returns a crypto service with an unlocked identity.
func newCryptoFixture(t *testing.T) (*CryptoService, *KeyService, model.PublicKey) {
	t.Helper()
	keys := NewKeyService(&fakeIdentityStore{}, withWorkFactor(testWorkFactor))
	pub, err := keys.Generate([]byte("test passphrase"))
	if err != nil {
		t.Fatal(err)
	}
	return NewCryptoService(keys, withCryptoWorkFactor(testWorkFactor)), keys, pub
}

func writeTemp(t *testing.T, dir, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCrypto_RoundTripToRecipient(t *testing.T) {
	crypto, _, pub := newCryptoFixture(t)
	dir := t.TempDir()
	want := []byte("the launch codes are 0000")

	in := writeTemp(t, dir, "secret.txt", want)
	enc := EncryptedName(in)

	if err := crypto.EncryptFile(t.Context(), in, enc, []model.PublicKey{pub}, Refuse, nil); err != nil {
		t.Fatalf("EncryptFile: %v", err)
	}

	// The ciphertext must not contain the plaintext.
	ct, err := os.ReadFile(enc)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(ct, want) {
		t.Fatal("plaintext found inside the encrypted file")
	}

	out := filepath.Join(dir, "roundtrip.txt")
	if err := crypto.DecryptFile(t.Context(), enc, out, Refuse, nil); err != nil {
		t.Fatalf("DecryptFile: %v", err)
	}

	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("round-trip = %q, want %q", got, want)
	}
}

func TestCrypto_RoundTripPassphrase(t *testing.T) {
	crypto, _, _ := newCryptoFixture(t)
	dir := t.TempDir()
	want := []byte("passphrase protected content")
	pass := []byte("hunter2")

	in := writeTemp(t, dir, "secret.txt", want)
	enc := EncryptedName(in)

	if err := crypto.EncryptFilePassphrase(t.Context(), in, enc, pass, Refuse, nil); err != nil {
		t.Fatalf("EncryptFilePassphrase: %v", err)
	}

	out := filepath.Join(dir, "out.txt")
	if err := crypto.DecryptFilePassphrase(t.Context(), enc, out, pass, Refuse, nil); err != nil {
		t.Fatalf("DecryptFilePassphrase: %v", err)
	}

	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("round-trip = %q, want %q", got, want)
	}
}

// A file encrypted to somebody else must produce a message the user can act on,
// not a raw library error.
func TestCrypto_DecryptNotForYou(t *testing.T) {
	crypto, _, _ := newCryptoFixture(t)
	dir := t.TempDir()

	stranger, err := age.GenerateHybridIdentity()
	if err != nil {
		t.Fatal(err)
	}
	strangerKey, err := model.ParsePublicKey(stranger.Recipient().String())
	if err != nil {
		t.Fatal(err)
	}

	in := writeTemp(t, dir, "theirs.txt", []byte("not yours"))
	enc := EncryptedName(in)
	if err := crypto.EncryptFile(t.Context(), in, enc, []model.PublicKey{strangerKey}, Refuse, nil); err != nil {
		t.Fatal(err)
	}

	err = crypto.DecryptFile(t.Context(), enc, filepath.Join(dir, "out"), Refuse, nil)
	if !errors.Is(err, model.ErrNotForYou) {
		t.Fatalf("DecryptFile(someone else's file) = %v, want ErrNotForYou", err)
	}
}

// Trying to open a passphrase file with a key must say "needs a passphrase",
// not "not encrypted for you". This is the NoIdentityMatchError.StanzaTypes
// path that makes the decrypt UI able to prompt for the right thing.
func TestCrypto_DecryptPassphraseFileWithKey(t *testing.T) {
	crypto, _, _ := newCryptoFixture(t)
	dir := t.TempDir()

	in := writeTemp(t, dir, "secret.txt", []byte("content"))
	enc := EncryptedName(in)
	if err := crypto.EncryptFilePassphrase(t.Context(), in, enc, []byte("pw"), Refuse, nil); err != nil {
		t.Fatal(err)
	}

	err := crypto.DecryptFile(t.Context(), enc, filepath.Join(dir, "out"), Refuse, nil)
	if !errors.Is(err, model.ErrPassphraseRequired) {
		t.Fatalf("DecryptFile(passphrase file) = %v, want ErrPassphraseRequired", err)
	}
}

func TestCrypto_DecryptWrongPassphrase(t *testing.T) {
	crypto, _, _ := newCryptoFixture(t)
	dir := t.TempDir()

	in := writeTemp(t, dir, "secret.txt", []byte("content"))
	enc := EncryptedName(in)
	if err := crypto.EncryptFilePassphrase(t.Context(), in, enc, []byte("right"), Refuse, nil); err != nil {
		t.Fatal(err)
	}

	err := crypto.DecryptFilePassphrase(t.Context(), enc, filepath.Join(dir, "out"), []byte("wrong"), Refuse, nil)
	if !errors.Is(err, model.ErrWrongPassphrase) {
		t.Fatalf("DecryptFilePassphrase(wrong) = %v, want ErrWrongPassphrase", err)
	}
}

// The mirror of TestCrypto_DecryptPassphraseFileWithKey: offering a passphrase
// for a key-encrypted file must say so, rather than claim the passphrase was
// wrong. Both directions hinge on classifyDecryptError knowing what was tried.
func TestCrypto_DecryptKeyFileWithPassphrase(t *testing.T) {
	crypto, _, pub := newCryptoFixture(t)
	dir := t.TempDir()

	in := writeTemp(t, dir, "secret.txt", []byte("content"))
	enc := EncryptedName(in)
	if err := crypto.EncryptFile(t.Context(), in, enc, []model.PublicKey{pub}, Refuse, nil); err != nil {
		t.Fatal(err)
	}

	err := crypto.DecryptFilePassphrase(t.Context(), enc, filepath.Join(dir, "out"), []byte("pw"), Refuse, nil)
	if !errors.Is(err, model.ErrKeyRequired) {
		t.Fatalf("DecryptFilePassphrase(key file) = %v, want ErrKeyRequired", err)
	}
}

// Inspect must work while locked: a passphrase file needs no identity, so
// demanding an unlock before we can even say what the file needs is nonsense.
func TestCrypto_InspectWorksWhileLocked(t *testing.T) {
	crypto, keys, pub := newCryptoFixture(t)
	dir := t.TempDir()

	in := writeTemp(t, dir, "a.txt", []byte("content"))
	byKey := filepath.Join(dir, "bykey.age")
	byPass := filepath.Join(dir, "bypass.age")

	if err := crypto.EncryptFile(t.Context(), in, byKey, []model.PublicKey{pub}, Refuse, nil); err != nil {
		t.Fatal(err)
	}
	if err := crypto.EncryptFilePassphrase(t.Context(), in, byPass, []byte("pw"), Refuse, nil); err != nil {
		t.Fatal(err)
	}

	keys.Lock()

	kind, err := crypto.Inspect(byPass)
	if err != nil {
		t.Fatalf("Inspect(passphrase file) while locked: %v", err)
	}
	if kind != FileKindPassphrase {
		t.Errorf("Inspect(passphrase file) = %q, want %q", kind, FileKindPassphrase)
	}

	kind, err = crypto.Inspect(byKey)
	if err != nil {
		t.Fatalf("Inspect(recipient file) while locked: %v", err)
	}
	if kind != FileKindRecipients {
		t.Errorf("Inspect(recipient file) = %q, want %q", kind, FileKindRecipients)
	}
}

func TestCrypto_InspectRejectsNonAgeFile(t *testing.T) {
	crypto, _, _ := newCryptoFixture(t)
	dir := t.TempDir()
	p := writeTemp(t, dir, "notage.txt", []byte("just a text file"))

	if _, err := crypto.Inspect(p); err == nil {
		t.Error("Inspect(plain text) = nil, want error")
	}
}

// age files come armored or binary; a GUI that only read one would reject
// files users legitimately received.
func TestCrypto_DecryptArmoredInput(t *testing.T) {
	crypto, _, pub := newCryptoFixture(t)
	dir := t.TempDir()
	want := []byte("armored content")

	// Build an armored file the way the age CLI's -a flag does.
	var buf bytes.Buffer
	aw := armor.NewWriter(&buf)
	w, err := age.Encrypt(aw, pub.Recipient())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(want); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := aw.Close(); err != nil {
		t.Fatal(err)
	}

	enc := writeTemp(t, dir, "armored.age", buf.Bytes())
	out := filepath.Join(dir, "out.txt")
	if err := crypto.DecryptFile(t.Context(), enc, out, Refuse, nil); err != nil {
		t.Fatalf("DecryptFile(armored): %v", err)
	}

	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("armored round-trip = %q, want %q", got, want)
	}

	// Inspect must sniff armor too.
	kind, err := crypto.Inspect(enc)
	if err != nil {
		t.Fatalf("Inspect(armored): %v", err)
	}
	if kind != FileKindRecipients {
		t.Errorf("Inspect(armored) = %q, want %q", kind, FileKindRecipients)
	}
}

func TestCrypto_DecryptRequiresUnlock(t *testing.T) {
	crypto, keys, pub := newCryptoFixture(t)
	dir := t.TempDir()

	in := writeTemp(t, dir, "a.txt", []byte("x"))
	enc := EncryptedName(in)
	if err := crypto.EncryptFile(t.Context(), in, enc, []model.PublicKey{pub}, Refuse, nil); err != nil {
		t.Fatal(err)
	}

	keys.Lock()
	err := crypto.DecryptFile(t.Context(), enc, filepath.Join(dir, "out"), Refuse, nil)
	if !errors.Is(err, model.ErrLocked) {
		t.Errorf("DecryptFile while locked = %v, want ErrLocked", err)
	}
}

func TestCrypto_NeverOverwrites(t *testing.T) {
	crypto, _, pub := newCryptoFixture(t)
	dir := t.TempDir()

	in := writeTemp(t, dir, "a.txt", []byte("x"))
	out := writeTemp(t, dir, "taken.age", []byte("precious"))

	err := crypto.EncryptFile(t.Context(), in, out, []model.PublicKey{pub}, Refuse, nil)
	if !errors.Is(err, model.ErrTargetExists) {
		t.Fatalf("EncryptFile onto an existing file = %v, want ErrTargetExists", err)
	}

	got, _ := os.ReadFile(out)
	if string(got) != "precious" {
		t.Error("EncryptFile clobbered an existing file")
	}
}

// Replace exists for paths the user confirmed in the OS save dialog, which
// asks before it ever hands back the path. Refusing there would show an error
// immediately after the user agreed.
func TestCrypto_ReplaceOverwritesWhenAsked(t *testing.T) {
	crypto, _, pub := newCryptoFixture(t)
	dir := t.TempDir()

	want := []byte("fresh content")
	in := writeTemp(t, dir, "a.txt", want)
	out := writeTemp(t, dir, "taken.age", []byte("stale"))

	if err := crypto.EncryptFile(t.Context(), in, out, []model.PublicKey{pub}, Replace, nil); err != nil {
		t.Fatalf("EncryptFile(Replace): %v", err)
	}

	// It must be a real encryption of the new input, not the old bytes.
	back := filepath.Join(dir, "back.txt")
	if err := crypto.DecryptFile(t.Context(), out, back, Refuse, nil); err != nil {
		t.Fatalf("DecryptFile: %v", err)
	}
	got, err := os.ReadFile(back)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("replaced file decrypts to %q, want %q", got, want)
	}
}

// A failed Replace must leave the original intact rather than destroying it on
// the way to an error.
func TestCrypto_FailedReplaceKeepsOriginal(t *testing.T) {
	crypto, _, _ := newCryptoFixture(t)
	dir := t.TempDir()

	in := writeTemp(t, dir, "a.txt", []byte("x"))
	out := writeTemp(t, dir, "taken.age", []byte("precious"))

	// No recipients: fails before any writing starts.
	err := crypto.EncryptFile(t.Context(), in, out, nil, Replace, nil)
	if !errors.Is(err, model.ErrNoRecipients) {
		t.Fatalf("EncryptFile = %v, want ErrNoRecipients", err)
	}
	got, _ := os.ReadFile(out)
	if string(got) != "precious" {
		t.Error("a failed Replace destroyed the existing file")
	}
}

func TestCrypto_NoRecipients(t *testing.T) {
	crypto, _, _ := newCryptoFixture(t)
	dir := t.TempDir()
	in := writeTemp(t, dir, "a.txt", []byte("x"))

	err := crypto.EncryptFile(t.Context(), in, EncryptedName(in), nil, Refuse, nil)
	if !errors.Is(err, model.ErrNoRecipients) {
		t.Errorf("EncryptFile with no recipients = %v, want ErrNoRecipients", err)
	}
}

// A cancelled operation must not leave a partial file that looks real.
func TestCrypto_CancelLeavesNoOutput(t *testing.T) {
	crypto, _, pub := newCryptoFixture(t)
	dir := t.TempDir()

	// Large enough that the copy loop spans several Reads, so cancellation
	// lands mid-stream.
	in := writeTemp(t, dir, "big.bin", bytes.Repeat([]byte("A"), 8<<20))
	out := filepath.Join(dir, "big.age")

	ctx, cancel := context.WithCancel(t.Context())
	err := crypto.EncryptFile(ctx, in, out, []model.PublicKey{pub}, Refuse, func(done, total int64) {
		cancel() // cancel on the first progress callback
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("EncryptFile(cancelled) = %v, want context.Canceled", err)
	}

	if _, statErr := os.Stat(out); !errors.Is(statErr, os.ErrNotExist) {
		t.Error("a cancelled encryption left an output file behind")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Errorf("cancelled encryption left a temp file: %s", e.Name())
		}
	}
}

func TestCrypto_ProgressReported(t *testing.T) {
	crypto, _, pub := newCryptoFixture(t)
	dir := t.TempDir()

	const size = 4 << 20
	in := writeTemp(t, dir, "big.bin", bytes.Repeat([]byte("B"), size))

	var lastDone, lastTotal int64
	var calls int
	err := crypto.EncryptFile(t.Context(), in, EncryptedName(in), []model.PublicKey{pub}, Refuse, func(done, total int64) {
		calls++
		lastDone, lastTotal = done, total
	})
	if err != nil {
		t.Fatal(err)
	}

	if calls == 0 {
		t.Fatal("progress callback was never invoked")
	}
	if lastTotal != size {
		t.Errorf("final total = %d, want %d", lastTotal, size)
	}
	// The last callback fires at EOF, so it must show the whole file consumed
	// or the progress bar would stall short of 100%.
	if lastDone != size {
		t.Errorf("final done = %d, want %d", lastDone, size)
	}
}

func TestEncryptedName(t *testing.T) {
	if got := EncryptedName("/tmp/report.pdf"); got != "/tmp/report.pdf.age" {
		t.Errorf("EncryptedName = %q", got)
	}
}

func TestDecryptedName(t *testing.T) {
	if got := DecryptedName("/tmp/report.pdf.age"); got != "/tmp/report.pdf" {
		t.Errorf("DecryptedName = %q, want the .age stripped", got)
	}
	// Without a .age suffix we cannot know the real name, and must not risk
	// overwriting the input.
	if got := DecryptedName("/tmp/mystery"); got != "/tmp/mystery.decrypted" {
		t.Errorf("DecryptedName = %q, want a distinct output path", got)
	}
}

func TestCrypto_EncryptDirectoryRejected(t *testing.T) {
	crypto, _, pub := newCryptoFixture(t)
	dir := t.TempDir()

	err := crypto.EncryptFile(t.Context(), dir, filepath.Join(dir, "out.age"), []model.PublicKey{pub}, Refuse, nil)
	if err == nil {
		t.Fatal("EncryptFile(directory) = nil, want error")
	}
	if !strings.Contains(err.Error(), "folder") {
		t.Errorf("error %q should tell the user it is a folder", err)
	}
}
