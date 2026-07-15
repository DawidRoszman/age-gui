package service

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"dawidroszman.eu/age-gui/internal/model"
)

// These tests run against the real age CLI, using it as an oracle.
//
// Round-tripping against our own library proves only that we are
// self-consistent. The actual requirement is that a non-technical user can
// exchange files with someone using stock age, so the only test that means
// anything is one where the other side really is stock age.
//
// Passphrase flows are deliberately absent: the age CLI reads passphrases from
// /dev/tty and refuses a pipe ("standard input is not a terminal"), so they
// cannot be driven here without a pty. See README for the manual check.

func requireAge(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("age"); err != nil {
		t.Skip("age CLI not found in PATH; skipping interop tests")
	}
	if _, err := exec.LookPath("age-keygen"); err != nil {
		t.Skip("age-keygen not found in PATH; skipping interop tests")
	}
}

// requirePQAge skips when the installed age predates native post-quantum
// support, which landed in v1.3.0.
func requirePQAge(t *testing.T) {
	t.Helper()
	requireAge(t)
	out, err := exec.Command("age-keygen", "-pq", "-o", filepath.Join(t.TempDir(), "probe")).CombinedOutput()
	if err != nil {
		t.Skipf("installed age lacks -pq support (needs v1.3.0+): %s", out)
	}
}

func run(t *testing.T, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("%s %s\nerror: %v\nstderr: %s", name, strings.Join(args, " "), err, stderr.String())
	}
}

// stockKeygen creates a keypair with the real age-keygen and returns the
// identity file path and the public key.
func stockKeygen(t *testing.T, dir string, pq bool) (keyFile, pub string) {
	t.Helper()
	keyFile = filepath.Join(dir, "key.txt")
	if pq {
		run(t, "age-keygen", "-pq", "-o", keyFile)
	} else {
		run(t, "age-keygen", "-o", keyFile)
	}

	// -y converts an identity file to its recipient.
	out, err := exec.Command("age-keygen", "-y", keyFile).Output()
	if err != nil {
		t.Fatalf("age-keygen -y: %v", err)
	}
	return keyFile, strings.TrimSpace(string(out))
}

// Files this app produces must open with the stock CLI. If this breaks, we have
// silently invented a private format.
func TestInterop_StockAgeDecryptsOurOutput(t *testing.T) {
	for _, tc := range []struct {
		name string
		pq   bool
	}{
		{"post-quantum hybrid", true},
		{"classic x25519", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.pq {
				requirePQAge(t)
			} else {
				requireAge(t)
			}
			dir := t.TempDir()
			keyFile, pub := stockKeygen(t, dir, tc.pq)

			key, err := model.ParsePublicKey(pub)
			if err != nil {
				t.Fatalf("our parser rejected a key from stock age-keygen: %v", err)
			}

			want := []byte("interop payload \x00\x01\x02 with binary bytes")
			in := writeTemp(t, dir, "msg.txt", want)
			enc := filepath.Join(dir, "msg.age")

			crypto, _, _ := newCryptoFixture(t)
			if err := crypto.EncryptFile(t.Context(), in, enc, []model.PublicKey{key}, Refuse, nil); err != nil {
				t.Fatalf("EncryptFile: %v", err)
			}

			out := filepath.Join(dir, "msg.out")
			run(t, "age", "-d", "-i", keyFile, "-o", out, enc)

			got, err := os.ReadFile(out)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("stock age decrypted our file to %q, want %q", got, want)
			}
		})
	}
}

// Files produced by stock age must open in this app. Without this, a user could
// not receive anything from a CLI user.
func TestInterop_WeDecryptStockAgeOutput(t *testing.T) {
	for _, tc := range []struct {
		name    string
		armored bool
	}{
		{"binary", false},
		{"armored", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requirePQAge(t)
			dir := t.TempDir()

			crypto, _, pub := newCryptoFixture(t)

			want := []byte("sent from the command line")
			in := writeTemp(t, dir, "msg.txt", want)
			enc := filepath.Join(dir, "msg.age")

			args := []string{"-r", pub.String(), "-o", enc}
			if tc.armored {
				args = append(args, "-a")
			}
			args = append(args, in)
			run(t, "age", args...)

			// Inspect must recognise a stock-produced file as key-encrypted.
			kind, err := crypto.Inspect(enc)
			if err != nil {
				t.Fatalf("Inspect on stock age output: %v", err)
			}
			if kind != FileKindRecipients {
				t.Errorf("Inspect = %q, want %q", kind, FileKindRecipients)
			}

			out := filepath.Join(dir, "msg.out")
			if err := crypto.DecryptFile(t.Context(), enc, out, Refuse, nil); err != nil {
				t.Fatalf("DecryptFile on stock age output: %v", err)
			}

			got, err := os.ReadFile(out)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("we decrypted stock age's file to %q, want %q", got, want)
			}
		})
	}
}

// The public key this app exports must be usable as-is by someone running the
// CLI. This is the "share your public key" requirement, end to end.
func TestInterop_StockAgeEncryptsToOurExportedKey(t *testing.T) {
	requirePQAge(t)
	dir := t.TempDir()

	keys, _ := newKeyService()
	if _, err := keys.Generate([]byte("passphrase")); err != nil {
		t.Fatal(err)
	}
	crypto := NewCryptoService(keys)

	// Export exactly as the Keys screen's Export button does.
	pubFile := filepath.Join(dir, "me.pub")
	if err := keys.ExportPublicKey(pubFile, Refuse); err != nil {
		t.Fatalf("ExportPublicKey: %v", err)
	}

	want := []byte("encrypted using -R against our exported file")
	in := writeTemp(t, dir, "msg.txt", want)
	enc := filepath.Join(dir, "msg.age")

	// -R reads a recipients file: the file we just exported.
	run(t, "age", "-R", pubFile, "-o", enc, in)

	out := filepath.Join(dir, "msg.out")
	if err := crypto.DecryptFile(t.Context(), enc, out, Refuse, nil); err != nil {
		t.Fatalf("DecryptFile: %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("round-trip via exported key file = %q, want %q", got, want)
	}
}

// A public key file produced by stock age-keygen must import as a contact.
// This is the "add a contact" requirement against a real-world key file.
func TestInterop_ImportStockKeygenPublicKey(t *testing.T) {
	for _, tc := range []struct {
		name string
		pq   bool
		want model.KeyType
	}{
		{"post-quantum hybrid", true, model.KeyTypeHybridPQ},
		{"classic x25519", false, model.KeyTypeX25519},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.pq {
				requirePQAge(t)
			} else {
				requireAge(t)
			}
			dir := t.TempDir()
			keyFile, _ := stockKeygen(t, dir, tc.pq)

			// age-keygen -y output is exactly what a contact would send.
			pubFile := filepath.Join(dir, "friend.pub")
			out, err := exec.Command("age-keygen", "-y", keyFile).Output()
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(pubFile, out, 0o600); err != nil {
				t.Fatal(err)
			}

			svc := newContactService()
			c, err := svc.AddFromFile("Friend", pubFile, "")
			if err != nil {
				t.Fatalf("AddFromFile on stock age-keygen output: %v", err)
			}
			if c.PublicKey.Type() != tc.want {
				t.Errorf("imported key type = %q, want %q", c.PublicKey.Type(), tc.want)
			}
		})
	}
}

// age-keygen writes its identity file with a "# public key:" comment line.
// Users hand each other these files by mistake; importing one as a contact
// must fail loudly rather than quietly grab the key from a file that also
// contains a private key.
func TestInterop_ImportRejectsIdentityFile(t *testing.T) {
	requireAge(t)
	dir := t.TempDir()
	keyFile, _ := stockKeygen(t, dir, false)

	svc := newContactService()
	if _, err := svc.AddFromFile("Oops", keyFile, ""); err == nil {
		t.Fatal("AddFromFile on an identity file = nil, want error — that file holds a private key")
	}
}
