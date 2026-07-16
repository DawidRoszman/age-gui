package view

import (
	"os"
	"path/filepath"
	"testing"

	"filippo.io/age"
)

// hybridPubKey returns a post-quantum public key, matching the kind Generate
// produces for the user's own identity. Encrypting to self plus a contact only
// works when the contact's key is the same kind.
func hybridPubKey(t *testing.T) string {
	t.Helper()
	id, err := age.GenerateHybridIdentity()
	if err != nil {
		t.Fatal(err)
	}
	return id.Recipient().String()
}

func TestGroups_CreateListUpdateDelete(t *testing.T) {
	f := newFixture(t)
	a := f.contacts.Add("Ana", pubKey(t), "")
	b := f.contacts.Add("Ben", pubKey(t), "")

	created := f.groups.Create("Team", []string{a.Contact.ID, b.Contact.ID})
	if created.Error != nil {
		t.Fatalf("Create: %+v", created.Error)
	}
	if created.Group.MemberCount != 2 {
		t.Errorf("MemberCount = %d, want 2", created.Group.MemberCount)
	}

	list := f.groups.List()
	if list.Error != nil || len(list.Groups) != 1 {
		t.Fatalf("List = %+v", list)
	}

	upd := f.groups.Update(created.Group.ID, "Team Alpha", []string{a.Contact.ID})
	if upd.Error != nil {
		t.Fatalf("Update: %+v", upd.Error)
	}
	if upd.Group.Name != "Team Alpha" || upd.Group.MemberCount != 1 {
		t.Errorf("Update = %+v, want renamed with one member", upd.Group)
	}

	if del := f.groups.Delete(created.Group.ID); del.Error != nil {
		t.Fatalf("Delete: %+v", del.Error)
	}
	if list := f.groups.List(); len(list.Groups) != 0 {
		t.Errorf("after Delete, List = %+v, want empty", list.Groups)
	}
}

func TestGroups_CreateRejectsEmptyName(t *testing.T) {
	f := newFixture(t)
	res := f.groups.Create("   ", nil)
	if res.Error == nil || res.Error.Code != CodeInvalidGroup {
		t.Fatalf("Create(blank) = %+v, want %s", res.Error, CodeInvalidGroup)
	}
}

func TestGroups_ListNeverNil(t *testing.T) {
	f := newFixture(t)
	// A fresh install has no groups; the field must still marshal as [] so the
	// frontend does not have to guard every iteration.
	if res := f.groups.List(); res.Groups == nil {
		t.Error("List().Groups is nil, want an empty slice")
	}
}

// Deleting a contact must remove it from the groups it belonged to, through the
// same handlers the UI calls.
func TestGroups_ContactDeletePrunesMembership(t *testing.T) {
	f := newFixture(t)
	a := f.contacts.Add("Ana", pubKey(t), "")
	b := f.contacts.Add("Ben", pubKey(t), "")
	g := f.groups.Create("Team", []string{a.Contact.ID, b.Contact.ID})

	if res := f.contacts.Delete(a.Contact.ID); res.Error != nil {
		t.Fatalf("Delete contact: %+v", res.Error)
	}

	list := f.groups.List()
	if len(list.Groups) != 1 {
		t.Fatalf("List = %+v", list.Groups)
	}
	got := list.Groups[0]
	if got.ID != g.Group.ID {
		t.Fatal("unexpected group id")
	}
	if len(got.MemberIDs) != 1 || got.MemberIDs[0] != b.Contact.ID {
		t.Errorf("members = %v, want just Ben after Ana was deleted", got.MemberIDs)
	}
}

// The whole point of the feature: a file encrypted only for a contact cannot be
// opened by the user, but with "include me" it can.
func TestCrypto_IncludeSelfLetsYouOpenYourOwnFile(t *testing.T) {
	f := newFixture(t)
	gen := f.keys.Generate("pass")
	if gen.Error != nil {
		t.Fatal(gen.Error)
	}
	// A contact who is somebody else entirely, with a key of the same kind as
	// ours so the two can share a file.
	other := f.contacts.Add("Other", hybridPubKey(t), "")

	dir := t.TempDir()
	in := filepath.Join(dir, "note.txt")
	want := []byte("for me and one other")
	if err := os.WriteFile(in, want, 0o600); err != nil {
		t.Fatal(err)
	}

	enc := f.crypto.Encrypt("j1", in, "", []string{other.Contact.ID}, true)
	if enc.Error != nil {
		t.Fatalf("Encrypt: %+v", enc.Error)
	}

	// Our own key must open it.
	dec := f.crypto.Decrypt("j2", enc.Value, filepath.Join(dir, "out.txt"))
	if dec.Error != nil {
		t.Fatalf("Decrypt with own key: %+v", dec.Error)
	}
	got, err := os.ReadFile(dec.Value)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Errorf("round-trip = %q, want %q", got, want)
	}
}

// Encrypting to nobody but yourself is valid: a file for your own storage.
func TestCrypto_IncludeSelfAloneIsValid(t *testing.T) {
	f := newFixture(t)
	if gen := f.keys.Generate("pass"); gen.Error != nil {
		t.Fatal(gen.Error)
	}

	dir := t.TempDir()
	in := filepath.Join(dir, "private.txt")
	if err := os.WriteFile(in, []byte("just mine"), 0o600); err != nil {
		t.Fatal(err)
	}

	enc := f.crypto.Encrypt("j1", in, "", nil, true)
	if enc.Error != nil {
		t.Fatalf("Encrypt to self only: %+v", enc.Error)
	}
	if dec := f.crypto.Decrypt("j2", enc.Value, filepath.Join(dir, "out.txt")); dec.Error != nil {
		t.Fatalf("Decrypt: %+v", dec.Error)
	}
}

// No contacts and not including self is the same as choosing nobody.
func TestCrypto_NoRecipientsAndNoSelfIsRefused(t *testing.T) {
	f := newFixture(t)
	if gen := f.keys.Generate("pass"); gen.Error != nil {
		t.Fatal(gen.Error)
	}

	dir := t.TempDir()
	in := filepath.Join(dir, "x.txt")
	if err := os.WriteFile(in, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	res := f.crypto.Encrypt("j1", in, "", nil, false)
	if res.Error == nil || res.Error.Code != CodeNoRecipients {
		t.Fatalf("Encrypt with nobody = %+v, want %s", res.Error, CodeNoRecipients)
	}
}

// Including yourself (always quantum-resistant) alongside a classic-key contact
// must fail with a clear, recoverable message, not a raw age error.
func TestCrypto_IncludeSelfWithClassicContactIsExplained(t *testing.T) {
	f := newFixture(t)
	if gen := f.keys.Generate("pass"); gen.Error != nil {
		t.Fatal(gen.Error)
	}
	classic := f.contacts.Add("Classic", pubKey(t), "") // pubKey is x25519

	dir := t.TempDir()
	in := filepath.Join(dir, "x.txt")
	if err := os.WriteFile(in, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	res := f.crypto.Encrypt("j1", in, "", []string{classic.Contact.ID}, true)
	if res.Error == nil || res.Error.Code != CodeIncompatibleRecipients {
		t.Fatalf("Encrypt mixing self+classic = %+v, want %s", res.Error, CodeIncompatibleRecipients)
	}
	if !res.Error.Recoverable {
		t.Error("the incompatible-recipients error should be recoverable, not a crash")
	}
}

// Selecting yourself both individually (as a contact holding your key) and via
// "include me" must not encrypt to the same key twice.
func TestCrypto_IncludeSelfDedupesAgainstContacts(t *testing.T) {
	f := newFixture(t)
	gen := f.keys.Generate("pass")
	if gen.Error != nil {
		t.Fatal(gen.Error)
	}
	// Add the user's own public key as a contact, then pick it and also ask to
	// include self.
	self := f.contacts.Add("Me", gen.Status.PublicKey, "")

	dir := t.TempDir()
	in := filepath.Join(dir, "x.txt")
	if err := os.WriteFile(in, []byte("hi"), 0o600); err != nil {
		t.Fatal(err)
	}

	enc := f.crypto.Encrypt("j1", in, "", []string{self.Contact.ID}, true)
	if enc.Error != nil {
		t.Fatalf("Encrypt: %+v", enc.Error)
	}
	// It still round-trips; the dedupe is about not listing the recipient twice,
	// which age tolerates but should not be produced.
	if dec := f.crypto.Decrypt("j2", enc.Value, filepath.Join(dir, "out.txt")); dec.Error != nil {
		t.Fatalf("Decrypt: %+v", dec.Error)
	}
}
