package service

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"dawidroszman.eu/age-gui/internal/model"
)

// The whole point of a backup: a lost machine must not mean lost files.
func TestBackup_RestoreOnANewMachine(t *testing.T) {
	dir := t.TempDir()
	backup := filepath.Join(dir, "key-backup.age")
	const pass = "correct horse battery staple"

	// Machine one.
	original, _ := newKeyService()
	pub, err := original.Generate([]byte(pass))
	if err != nil {
		t.Fatal(err)
	}
	if err := original.ExportIdentity(backup, Refuse); err != nil {
		t.Fatalf("ExportIdentity: %v", err)
	}

	// Machine two: empty store, only the backup file.
	fresh := NewKeyService(&fakeIdentityStore{}, withWorkFactor(testWorkFactor))
	restored, err := fresh.RestoreIdentity(backup, []byte(pass))
	if err != nil {
		t.Fatalf("RestoreIdentity: %v", err)
	}
	if !restored.Equal(pub) {
		t.Fatal("the restored key is not the original key")
	}

	// And it must actually be able to decrypt: the point is the files, not the
	// string matching.
	crypto := NewCryptoService(fresh, withCryptoWorkFactor(testWorkFactor))
	in := writeTemp(t, dir, "secret.txt", []byte("still readable"))
	enc := filepath.Join(dir, "secret.age")
	if err := crypto.EncryptFile(t.Context(), in, enc, []model.PublicKey{pub}, Refuse, nil); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out.txt")
	if err := crypto.DecryptFile(t.Context(), enc, out, Refuse, nil); err != nil {
		t.Fatalf("restored key cannot decrypt: %v", err)
	}
	got, _ := os.ReadFile(out)
	if string(got) != "still readable" {
		t.Errorf("decrypted %q, want %q", got, "still readable")
	}
}

// A backup is only useful if it is safe to keep somewhere. It must be the same
// encrypted bytes, not a plaintext key.
func TestBackup_IsEncryptedAndVerbatim(t *testing.T) {
	dir := t.TempDir()
	backup := filepath.Join(dir, "b.age")

	svc, store := newKeyService()
	if _, err := svc.Generate([]byte("pass")); err != nil {
		t.Fatal(err)
	}
	if err := svc.ExportIdentity(backup, Refuse); err != nil {
		t.Fatal(err)
	}

	b, err := os.ReadFile(backup)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToUpper(string(b)), "AGE-SECRET-KEY") {
		t.Fatal("the backup contains the private key in plaintext")
	}
	if !strings.HasPrefix(string(b), "-----BEGIN AGE ENCRYPTED FILE-----") {
		t.Errorf("backup is not an armored age file: %.40q", b)
	}
	if string(b) != string(store.blob) {
		t.Error("the backup is not a verbatim copy of the stored identity")
	}
}

func TestBackup_RestoreWrongPassphrase(t *testing.T) {
	dir := t.TempDir()
	backup := filepath.Join(dir, "b.age")

	svc, _ := newKeyService()
	if _, err := svc.Generate([]byte("right")); err != nil {
		t.Fatal(err)
	}
	if err := svc.ExportIdentity(backup, Refuse); err != nil {
		t.Fatal(err)
	}

	fresh := NewKeyService(&fakeIdentityStore{}, withWorkFactor(testWorkFactor))
	_, err := fresh.RestoreIdentity(backup, []byte("wrong"))
	if !errors.Is(err, model.ErrWrongPassphrase) {
		t.Fatalf("RestoreIdentity(wrong) = %v, want ErrWrongPassphrase", err)
	}
	// A failed restore must leave the fresh install untouched.
	st, _ := fresh.Status()
	if st.Exists {
		t.Error("a failed restore left an identity behind")
	}
}

// Restoring over an existing key would destroy it and orphan every file
// encrypted to it — the same reasoning that makes Generate refuse.
func TestBackup_RestoreRefusesToClobber(t *testing.T) {
	dir := t.TempDir()
	backup := filepath.Join(dir, "b.age")

	donor, _ := newKeyService()
	if _, err := donor.Generate([]byte("pass")); err != nil {
		t.Fatal(err)
	}
	if err := donor.ExportIdentity(backup, Refuse); err != nil {
		t.Fatal(err)
	}

	victim, store := newKeyService()
	mine, err := victim.Generate([]byte("mine"))
	if err != nil {
		t.Fatal(err)
	}
	before := append([]byte(nil), store.blob...)

	if _, err := victim.RestoreIdentity(backup, []byte("pass")); !errors.Is(err, model.ErrIdentityExists) {
		t.Fatalf("RestoreIdentity over an existing key = %v, want ErrIdentityExists", err)
	}
	if string(store.blob) != string(before) {
		t.Fatal("a refused restore modified the stored identity")
	}
	got, _ := victim.PublicKey()
	if !got.Equal(mine) {
		t.Error("a refused restore changed the in-memory key")
	}
}

func TestBackup_RestoreLeavesKeyUnlocked(t *testing.T) {
	dir := t.TempDir()
	backup := filepath.Join(dir, "b.age")

	svc, _ := newKeyService()
	if _, err := svc.Generate([]byte("pass")); err != nil {
		t.Fatal(err)
	}
	if err := svc.ExportIdentity(backup, Refuse); err != nil {
		t.Fatal(err)
	}

	fresh := NewKeyService(&fakeIdentityStore{}, withWorkFactor(testWorkFactor))
	if _, err := fresh.RestoreIdentity(backup, []byte("pass")); err != nil {
		t.Fatal(err)
	}

	st, err := fresh.Status()
	if err != nil {
		t.Fatal(err)
	}
	// They just proved they know the passphrase; asking again would be rude.
	if !st.Exists || !st.Unlocked {
		t.Errorf("after restore, status = %+v, want existing and unlocked", st)
	}
}

func TestBackup_ExportWithoutIdentity(t *testing.T) {
	svc, _ := newKeyService()
	err := svc.ExportIdentity(filepath.Join(t.TempDir(), "b.age"), Refuse)
	if !errors.Is(err, model.ErrNoIdentity) {
		t.Errorf("ExportIdentity with no key = %v, want ErrNoIdentity", err)
	}
}

// Backing up ciphertext needs no plaintext, so requiring an unlock would be
// theatre — the user could copy the file in their file manager regardless.
func TestBackup_ExportWorksWhileLocked(t *testing.T) {
	dir := t.TempDir()
	svc, _ := newKeyService()
	if _, err := svc.Generate([]byte("pass")); err != nil {
		t.Fatal(err)
	}
	svc.Lock()

	if err := svc.ExportIdentity(filepath.Join(dir, "b.age"), Refuse); err != nil {
		t.Errorf("ExportIdentity while locked = %v, want it to work", err)
	}
}

func TestBackup_ExportRefusesOverwrite(t *testing.T) {
	dir := t.TempDir()
	taken := writeTemp(t, dir, "taken.age", []byte("precious"))

	svc, _ := newKeyService()
	if _, err := svc.Generate([]byte("pass")); err != nil {
		t.Fatal(err)
	}

	if err := svc.ExportIdentity(taken, Refuse); !errors.Is(err, model.ErrTargetExists) {
		t.Fatalf("= %v, want ErrTargetExists", err)
	}
	if b, _ := os.ReadFile(taken); string(b) != "precious" {
		t.Error("a refused export clobbered an existing file")
	}
	// Replace is for a path the save dialog already confirmed.
	if err := svc.ExportIdentity(taken, Replace); err != nil {
		t.Errorf("ExportIdentity(Replace) = %v, want success", err)
	}
}

func TestBackup_RestoreRejectsGarbage(t *testing.T) {
	dir := t.TempDir()
	junk := writeTemp(t, dir, "notes.txt", []byte("just some notes I wrote"))

	svc := NewKeyService(&fakeIdentityStore{}, withWorkFactor(testWorkFactor))
	_, err := svc.RestoreIdentity(junk, []byte("pass"))
	if !errors.Is(err, model.ErrNotAnIdentityFile) {
		t.Errorf("RestoreIdentity(junk) = %v, want ErrNotAnIdentityFile", err)
	}
}

func TestBackup_RestoreEmptyPassphrase(t *testing.T) {
	svc := NewKeyService(&fakeIdentityStore{}, withWorkFactor(testWorkFactor))
	_, err := svc.RestoreIdentity(filepath.Join(t.TempDir(), "whatever"), nil)
	if !errors.Is(err, model.ErrEmptyPassphrase) {
		t.Errorf("= %v, want ErrEmptyPassphrase", err)
	}
}

// Someone already using the age CLI must be able to bring their key across.
// age-keygen writes a plaintext file with comment lines; the passphrase they
// give becomes the new protection.
func TestBackup_RestoreFromStockAgeKeygenFile(t *testing.T) {
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
			keyFile, pub := stockKeygen(t, dir, tc.pq)

			svc := NewKeyService(&fakeIdentityStore{}, withWorkFactor(testWorkFactor))
			got, err := svc.RestoreIdentity(keyFile, []byte("a new passphrase"))
			if err != nil {
				t.Fatalf("RestoreIdentity(age-keygen file): %v", err)
			}
			if got.String() != pub {
				t.Errorf("imported key = %q, want %q", got.Abbrev(), pub[:16])
			}
			if got.Type() != tc.want {
				t.Errorf("key type = %q, want %q", got.Type(), tc.want)
			}
		})
	}
}

// A plaintext key must be encrypted before it is stored, under the passphrase
// just supplied — otherwise importing from the CLI would quietly downgrade the
// user to an unprotected key file.
func TestBackup_ImportedPlaintextKeyIsStoredEncrypted(t *testing.T) {
	requireAge(t)
	dir := t.TempDir()
	keyFile, _ := stockKeygen(t, dir, false)

	store := &fakeIdentityStore{}
	svc := NewKeyService(store, withWorkFactor(testWorkFactor))
	if _, err := svc.RestoreIdentity(keyFile, []byte("chosen passphrase")); err != nil {
		t.Fatal(err)
	}

	if strings.Contains(strings.ToUpper(string(store.blob)), "AGE-SECRET-KEY") {
		t.Fatal("an imported plaintext key was stored without being encrypted")
	}
	// And the chosen passphrase must actually open it.
	svc.Lock()
	if err := svc.Unlock([]byte("chosen passphrase")); err != nil {
		t.Errorf("cannot unlock an imported key with the passphrase given at import: %v", err)
	}
}

// The backup must open with stock age, not just with this app. If this breaks,
// the user's key is trapped here.
func TestBackup_StockAgeUnderstandsOurBackupFormat(t *testing.T) {
	requireAge(t)
	dir := t.TempDir()
	backup := filepath.Join(dir, "b.age")

	svc, _ := newKeyService()
	if _, err := svc.Generate([]byte("pass")); err != nil {
		t.Fatal(err)
	}
	if err := svc.ExportIdentity(backup, Refuse); err != nil {
		t.Fatal(err)
	}

	// age refuses a piped passphrase (it demands a TTY), so we cannot decrypt
	// here. We can still prove age recognises the file as a passphrase-
	// encrypted age file rather than something it does not understand.
	cmd := exec.Command("age", "-d", backup)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("age -d succeeded without a passphrase")
	}
	if !strings.Contains(string(out), "passphrase") {
		t.Errorf("stock age did not recognise our backup as passphrase-encrypted.\ngot: %s", out)
	}
}
