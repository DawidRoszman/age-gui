package service

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"filippo.io/age"

	"dawidroszman.eu/encryptor/internal/model"
)

func newContactService() *ContactService {
	return NewContactService(&fakeContactStore{})
}

func hybridPub(t *testing.T) string {
	t.Helper()
	id, err := age.GenerateHybridIdentity()
	if err != nil {
		t.Fatal(err)
	}
	return id.Recipient().String()
}

func x25519Pub(t *testing.T) string {
	t.Helper()
	id, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	return id.Recipient().String()
}

func TestContactService_AddAndList(t *testing.T) {
	svc := newContactService()

	c, err := svc.Add("Alice", hybridPub(t), "colleague")
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if c.Name != "Alice" {
		t.Errorf("Name = %q", c.Name)
	}

	all, err := svc.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("List() = %d, want 1", len(all))
	}
}

// The heart of "liberal on import": we generate hybrid keys, but contacts may
// use classic ones and we must be able to encrypt to them.
func TestContactService_AddAcceptsClassicKey(t *testing.T) {
	svc := newContactService()

	c, err := svc.Add("Bob", x25519Pub(t), "")
	if err != nil {
		t.Fatalf("Add(x25519) = %v, want nil — classic contacts must work", err)
	}
	if c.PublicKey.Type() != model.KeyTypeX25519 {
		t.Errorf("Type = %q, want %q", c.PublicKey.Type(), model.KeyTypeX25519)
	}
}

func TestContactService_ListSortedByName(t *testing.T) {
	svc := newContactService()

	for _, n := range []string{"zoe", "Alice", "bob"} {
		if _, err := svc.Add(n, hybridPub(t), ""); err != nil {
			t.Fatal(err)
		}
	}

	all, err := svc.List()
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, c := range all {
		got = append(got, c.Name)
	}
	want := []string{"Alice", "bob", "zoe"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("List() order = %v, want %v (case-insensitive)", got, want)
		}
	}
}

func TestContactService_AddRejectsDuplicateKey(t *testing.T) {
	svc := newContactService()
	key := hybridPub(t)

	if _, err := svc.Add("Alice", key, ""); err != nil {
		t.Fatal(err)
	}

	_, err := svc.Add("Alice Again", key, "")
	if !errors.Is(err, model.ErrDuplicateContact) {
		t.Fatalf("Add(duplicate key) = %v, want ErrDuplicateContact", err)
	}
	// The message must name who already holds it, or the user cannot resolve it.
	if !strings.Contains(err.Error(), "Alice") {
		t.Errorf("error %q should name the existing contact", err)
	}
}

func TestContactService_AddRejectsPrivateKey(t *testing.T) {
	svc := newContactService()

	id, err := age.GenerateHybridIdentity()
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.Add("Oops", id.String(), "")
	if !errors.Is(err, model.ErrSecretKeyGiven) {
		t.Errorf("Add(private key) = %v, want ErrSecretKeyGiven", err)
	}
}

func TestContactService_AddFromFile(t *testing.T) {
	svc := newContactService()
	key := hybridPub(t)

	path := filepath.Join(t.TempDir(), "alice.pub")
	if err := os.WriteFile(path, []byte(key+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	c, err := svc.AddFromFile("Alice", path, "")
	if err != nil {
		t.Fatalf("AddFromFile: %v", err)
	}
	if c.PublicKey.String() != key {
		t.Error("imported key does not match the file contents")
	}
}

// age's own recipient files carry comments; reusing age.ParseRecipients means
// a file exported by any age tool imports cleanly.
func TestContactService_AddFromFileWithComments(t *testing.T) {
	svc := newContactService()
	key := x25519Pub(t)

	body := "# created by age-keygen\n\n" + key + "\n"
	path := filepath.Join(t.TempDir(), "bob.pub")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	c, err := svc.AddFromFile("Bob", path, "")
	if err != nil {
		t.Fatalf("AddFromFile(with comments): %v", err)
	}
	if c.PublicKey.String() != key {
		t.Error("comment handling corrupted the imported key")
	}
}

// A recipients file with several keys is ambiguous: we must not silently pick
// the first and attribute it to this contact.
func TestContactService_AddFromFileRejectsMultipleKeys(t *testing.T) {
	svc := newContactService()

	body := hybridPub(t) + "\n" + x25519Pub(t) + "\n"
	path := filepath.Join(t.TempDir(), "many.pub")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.AddFromFile("Ambiguous", path, ""); err == nil {
		t.Error("AddFromFile(2 keys) = nil, want error")
	}
}

func TestContactService_Rename(t *testing.T) {
	svc := newContactService()
	c, err := svc.Add("Alice", hybridPub(t), "old note")
	if err != nil {
		t.Fatal(err)
	}

	got, err := svc.Rename(c.ID, "  Alice Smith  ", " new note ")
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if got.Name != "Alice Smith" || got.Note != "new note" {
		t.Errorf("Rename = %+v, want trimmed values", got)
	}
	// The key must be untouched: a different key is a different person.
	if !got.PublicKey.Equal(c.PublicKey) {
		t.Error("Rename changed the public key")
	}

	all, _ := svc.List()
	if len(all) != 1 {
		t.Errorf("Rename created a duplicate: %d contacts", len(all))
	}
}

func TestContactService_RenameRejectsEmptyName(t *testing.T) {
	svc := newContactService()
	c, err := svc.Add("Alice", hybridPub(t), "")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := svc.Rename(c.ID, "   ", ""); err == nil {
		t.Error("Rename to blank = nil, want error")
	}
}

func TestContactService_GetAndDeleteMissing(t *testing.T) {
	svc := newContactService()

	if _, err := svc.Get("nope"); !errors.Is(err, model.ErrContactNotFound) {
		t.Errorf("Get(missing) = %v, want ErrContactNotFound", err)
	}
	if err := svc.Delete("nope"); !errors.Is(err, model.ErrContactNotFound) {
		t.Errorf("Delete(missing) = %v, want ErrContactNotFound", err)
	}
}

func TestContactService_Delete(t *testing.T) {
	svc := newContactService()
	c, err := svc.Add("Alice", hybridPub(t), "")
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.Delete(c.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	all, _ := svc.List()
	if len(all) != 0 {
		t.Errorf("List() = %d after Delete, want 0", len(all))
	}
}

func TestContactService_Recipients(t *testing.T) {
	svc := newContactService()

	alice, err := svc.Add("Alice", hybridPub(t), "")
	if err != nil {
		t.Fatal(err)
	}
	bob, err := svc.Add("Bob", x25519Pub(t), "")
	if err != nil {
		t.Fatal(err)
	}

	keys, err := svc.Recipients([]string{alice.ID, bob.ID})
	if err != nil {
		t.Fatalf("Recipients: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("Recipients() = %d keys, want 2", len(keys))
	}
	// Order must follow the request so the caller can correlate.
	if !keys[0].Equal(alice.PublicKey) || !keys[1].Equal(bob.PublicKey) {
		t.Error("Recipients() did not preserve request order")
	}
}

// Encrypting to a subset would hand the user a file they believe they shared
// and which the missing recipient cannot open.
func TestContactService_RecipientsFailsOnUnknownID(t *testing.T) {
	svc := newContactService()
	alice, err := svc.Add("Alice", hybridPub(t), "")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := svc.Recipients([]string{alice.ID, "ghost"}); !errors.Is(err, model.ErrContactNotFound) {
		t.Errorf("Recipients(unknown id) = %v, want ErrContactNotFound", err)
	}
}

func TestContactService_RecipientsEmpty(t *testing.T) {
	svc := newContactService()

	if _, err := svc.Recipients(nil); !errors.Is(err, model.ErrNoRecipients) {
		t.Errorf("Recipients(nil) = %v, want ErrNoRecipients", err)
	}
}
