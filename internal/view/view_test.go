package view

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"filippo.io/age"

	"dawidroszman.eu/encryptor/internal/model"
	"dawidroszman.eu/encryptor/internal/service"
)

// fakePlatform stands in for the desktop. This is what the Platform port buys:
// handler tests run with no window, no display, and no GTK build.
type fakePlatform struct {
	mu sync.Mutex

	clipboard string
	openPath  string
	openPaths []string
	savePath  string
	failWith  error
	events    []recordedEvent
}

type recordedEvent struct {
	name string
	data []any
}

func (p *fakePlatform) OpenFileDialog(string) (string, error) {
	return p.openPath, p.failWith
}

func (p *fakePlatform) OpenFilesDialog(string) ([]string, error) {
	return p.openPaths, p.failWith
}

func (p *fakePlatform) SaveFileDialog(string, string) (string, error) {
	return p.savePath, p.failWith
}

func (p *fakePlatform) SetClipboard(text string) error {
	if p.failWith != nil {
		return p.failWith
	}
	p.clipboard = text
	return nil
}

func (p *fakePlatform) EmitEvent(name string, data ...any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, recordedEvent{name, data})
}

func (p *fakePlatform) eventCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.events)
}

var _ Platform = (*fakePlatform)(nil)

// memIdentityStore is an in-memory service.IdentityStore.
type memIdentityStore struct{ blob []byte }

func (m *memIdentityStore) Exists() (bool, error) { return m.blob != nil, nil }
func (m *memIdentityStore) Load() ([]byte, error) { return m.blob, nil }
func (m *memIdentityStore) Save(b []byte) error   { m.blob = b; return nil }

// memContactStore is an in-memory service.ContactStore.
type memContactStore struct{ contacts []model.Contact }

func (m *memContactStore) List() ([]model.Contact, error) {
	return append([]model.Contact(nil), m.contacts...), nil
}

func (m *memContactStore) Put(c model.Contact) error {
	for i := range m.contacts {
		if m.contacts[i].ID == c.ID {
			m.contacts[i] = c
			return nil
		}
	}
	m.contacts = append(m.contacts, c)
	return nil
}

func (m *memContactStore) Delete(id string) error {
	for i := range m.contacts {
		if m.contacts[i].ID == id {
			m.contacts = append(m.contacts[:i:i], m.contacts[i+1:]...)
			return nil
		}
	}
	return model.ErrContactNotFound
}

type fixture struct {
	keys     *Keys
	contacts *Contacts
	crypto   *Crypto
	platform *fakePlatform
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	platform := &fakePlatform{}
	keySvc := service.NewKeyService(&memIdentityStore{})
	contactSvc := service.NewContactService(&memContactStore{})
	cryptoSvc := service.NewCryptoService(keySvc)

	return &fixture{
		keys:     NewKeys(keySvc, platform),
		contacts: NewContacts(contactSvc, platform),
		crypto:   NewCrypto(cryptoSvc, contactSvc, platform),
		platform: platform,
	}
}

func pubKey(t *testing.T) string {
	t.Helper()
	id, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	return id.Recipient().String()
}

func TestKeys_StatusOnFirstRun(t *testing.T) {
	f := newFixture(t)

	res := f.keys.Status()
	if res.Error != nil {
		t.Fatalf("Status: %+v", res.Error)
	}
	if res.Status.Exists || res.Status.Unlocked {
		t.Errorf("Status = %+v, want a blank first run", res.Status)
	}
	if res.Status.PublicKey != "" {
		t.Error("a public key is present before any key was created")
	}
}

func TestKeys_GenerateThenLockUnlock(t *testing.T) {
	f := newFixture(t)

	res := f.keys.Generate("a good passphrase")
	if res.Error != nil {
		t.Fatalf("Generate: %+v", res.Error)
	}
	if !res.Status.Exists || !res.Status.Unlocked {
		t.Fatalf("after Generate, status = %+v", res.Status)
	}
	if res.Status.KeyType != string(model.KeyTypeHybridPQ) {
		t.Errorf("KeyType = %q, want post-quantum", res.Status.KeyType)
	}
	// The abbreviation exists so a ~2000 char key never lands in a list.
	if len(res.Status.Abbrev) > 32 {
		t.Errorf("Abbrev is %d chars, too long to display", len(res.Status.Abbrev))
	}

	if res := f.keys.Lock(); res.Error != nil || res.Status.Unlocked {
		t.Fatalf("Lock: %+v %+v", res.Error, res.Status)
	}

	if res := f.keys.Unlock("wrong"); res.Error == nil || res.Error.Code != CodeWrongPassphrase {
		t.Fatalf("Unlock(wrong) error = %+v, want %s", res.Error, CodeWrongPassphrase)
	}
	if res := f.keys.Unlock("a good passphrase"); res.Error != nil || !res.Status.Unlocked {
		t.Fatalf("Unlock(right): %+v", res.Error)
	}
}

func TestKeys_GenerateTwiceIsRefused(t *testing.T) {
	f := newFixture(t)
	if res := f.keys.Generate("pass"); res.Error != nil {
		t.Fatal(res.Error)
	}

	res := f.keys.Generate("pass2")
	if res.Error == nil || res.Error.Code != CodeIdentityExists {
		t.Fatalf("second Generate error = %+v, want %s", res.Error, CodeIdentityExists)
	}
}

func TestKeys_CopyPublicKey(t *testing.T) {
	f := newFixture(t)
	gen := f.keys.Generate("pass")
	if gen.Error != nil {
		t.Fatal(gen.Error)
	}

	if res := f.keys.CopyPublicKey(); res.Error != nil {
		t.Fatalf("CopyPublicKey: %+v", res.Error)
	}
	if f.platform.clipboard != gen.Status.PublicKey {
		t.Error("clipboard does not hold the public key")
	}
	// The one thing that must never happen.
	if strings.Contains(strings.ToUpper(f.platform.clipboard), "AGE-SECRET-KEY") {
		t.Fatal("a private key was copied to the clipboard")
	}
}

func TestKeys_CopyPublicKeyWhileLocked(t *testing.T) {
	f := newFixture(t)

	res := f.keys.CopyPublicKey()
	if res.Error == nil || res.Error.Code != CodeLocked {
		t.Fatalf("CopyPublicKey while locked = %+v, want %s", res.Error, CodeLocked)
	}
	if f.platform.clipboard != "" {
		t.Error("something was copied while locked")
	}
}

func TestKeys_ExportPublicKey(t *testing.T) {
	f := newFixture(t)
	if res := f.keys.Generate("pass"); res.Error != nil {
		t.Fatal(res.Error)
	}
	f.platform.savePath = filepath.Join(t.TempDir(), "me.pub")

	res := f.keys.ExportPublicKey()
	if res.Error != nil {
		t.Fatalf("ExportPublicKey: %+v", res.Error)
	}
	if res.Value != f.platform.savePath {
		t.Errorf("Value = %q, want the chosen path", res.Value)
	}

	b, err := os.ReadFile(f.platform.savePath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToUpper(string(b)), "AGE-SECRET-KEY") {
		t.Fatal("the exported file contains a private key")
	}
}

// Cancelling a dialog is a normal thing to do, not a failure to report.
func TestKeys_ExportCancelledIsNotAnError(t *testing.T) {
	f := newFixture(t)
	if res := f.keys.Generate("pass"); res.Error != nil {
		t.Fatal(res.Error)
	}
	f.platform.savePath = "" // user pressed Cancel

	res := f.keys.ExportPublicKey()
	if res.Error != nil {
		t.Errorf("cancelling the export reported an error: %+v", res.Error)
	}
	if res.Value != "" {
		t.Errorf("Value = %q, want empty on cancel", res.Value)
	}
}

// The save dialog already asked about replacing, so the handler must not then
// refuse and contradict the user.
func TestKeys_ExportOverExistingFileSucceeds(t *testing.T) {
	f := newFixture(t)
	if res := f.keys.Generate("pass"); res.Error != nil {
		t.Fatal(res.Error)
	}

	path := filepath.Join(t.TempDir(), "me.pub")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	f.platform.savePath = path

	if res := f.keys.ExportPublicKey(); res.Error != nil {
		t.Fatalf("export over a dialog-confirmed path failed: %+v", res.Error)
	}
	b, _ := os.ReadFile(path)
	if string(b) == "old" {
		t.Error("the file was not replaced after the user confirmed")
	}
}

func TestContacts_AddListDelete(t *testing.T) {
	f := newFixture(t)

	add := f.contacts.Add("Alice", pubKey(t), "friend")
	if add.Error != nil {
		t.Fatalf("Add: %+v", add.Error)
	}

	list := f.contacts.List()
	if list.Error != nil || len(list.Contacts) != 1 {
		t.Fatalf("List = %+v", list)
	}
	if list.Contacts[0].Name != "Alice" {
		t.Errorf("Name = %q", list.Contacts[0].Name)
	}

	if res := f.contacts.Delete(add.Contact.ID); res.Error != nil {
		t.Fatalf("Delete: %+v", res.Error)
	}
	if list := f.contacts.List(); len(list.Contacts) != 0 {
		t.Errorf("contact survived delete")
	}
}

// An empty list must serialise as [] rather than null, or the frontend has to
// guard every iteration.
func TestContacts_EmptyListIsNotNull(t *testing.T) {
	f := newFixture(t)

	res := f.contacts.List()
	if res.Contacts == nil {
		t.Fatal("Contacts is nil; JSON null forces the UI to null-check")
	}
	if len(res.Contacts) != 0 {
		t.Errorf("len = %d, want 0", len(res.Contacts))
	}
}

// Pasting a private key into the contact field must be caught loudly: the user
// may be about to send it to somebody.
func TestContacts_AddPrivateKeyIsRefusedLoudly(t *testing.T) {
	f := newFixture(t)
	id, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}

	res := f.contacts.Add("Oops", id.String(), "")
	if res.Error == nil || res.Error.Code != CodeSecretKeyGiven {
		t.Fatalf("Add(private key) = %+v, want %s", res.Error, CodeSecretKeyGiven)
	}
	if !strings.Contains(res.Error.Message, "Never share") {
		t.Errorf("message %q does not warn the user", res.Error.Message)
	}
}

func TestContacts_ImportCancelledIsNotAnError(t *testing.T) {
	f := newFixture(t)
	f.platform.openPath = "" // cancelled

	res := f.contacts.ImportFromFile("Alice", "")
	if res.Error != nil {
		t.Errorf("cancelling import reported an error: %+v", res.Error)
	}
	if res.Contact.ID != "" {
		t.Error("a contact was created despite cancelling")
	}
}

func TestContacts_CopyPublicKey(t *testing.T) {
	f := newFixture(t)
	key := pubKey(t)
	add := f.contacts.Add("Alice", key, "")
	if add.Error != nil {
		t.Fatal(add.Error)
	}

	if res := f.contacts.CopyPublicKey(add.Contact.ID); res.Error != nil {
		t.Fatalf("CopyPublicKey: %+v", res.Error)
	}
	if f.platform.clipboard != key {
		t.Error("clipboard does not hold the contact's key")
	}
}

func TestCrypto_EncryptDecryptRoundTrip(t *testing.T) {
	f := newFixture(t)
	gen := f.keys.Generate("pass")
	if gen.Error != nil {
		t.Fatal(gen.Error)
	}
	add := f.contacts.Add("Me", gen.Status.PublicKey, "")
	if add.Error != nil {
		t.Fatal(add.Error)
	}

	dir := t.TempDir()
	in := filepath.Join(dir, "secret.txt")
	want := []byte("top secret")
	if err := os.WriteFile(in, want, 0o600); err != nil {
		t.Fatal(err)
	}

	enc := f.crypto.Encrypt("job1", in, "", []string{add.Contact.ID})
	if enc.Error != nil {
		t.Fatalf("Encrypt: %+v", enc.Error)
	}
	if enc.Value != in+".age" {
		t.Errorf("output = %q, want the default .age name", enc.Value)
	}

	// Inspect must route this to the key flow, not the passphrase flow.
	if kind := f.crypto.Inspect(enc.Value); kind.Error != nil || kind.Kind != "recipients" {
		t.Fatalf("Inspect = %+v", kind)
	}

	dec := f.crypto.Decrypt("job2", enc.Value, filepath.Join(dir, "out.txt"))
	if dec.Error != nil {
		t.Fatalf("Decrypt: %+v", dec.Error)
	}
	got, err := os.ReadFile(dec.Value)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Errorf("round-trip = %q, want %q", got, want)
	}

	// Progress must have reached the frontend.
	if f.platform.eventCount() == 0 {
		t.Error("no progress events were emitted")
	}
}

func TestCrypto_DecryptWhileLockedReportsLocked(t *testing.T) {
	f := newFixture(t)
	gen := f.keys.Generate("pass")
	if gen.Error != nil {
		t.Fatal(gen.Error)
	}
	add := f.contacts.Add("Me", gen.Status.PublicKey, "")

	dir := t.TempDir()
	in := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(in, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	enc := f.crypto.Encrypt("j1", in, "", []string{add.Contact.ID})
	if enc.Error != nil {
		t.Fatal(enc.Error)
	}

	f.keys.Lock()

	res := f.crypto.Decrypt("j2", enc.Value, filepath.Join(dir, "out"))
	if res.Error == nil || res.Error.Code != CodeLocked {
		t.Fatalf("Decrypt while locked = %+v, want %s", res.Error, CodeLocked)
	}
}

// The default output path is one the user never saw, so a collision must stop
// and ask rather than destroy a file.
func TestCrypto_DefaultOutputRefusesToOverwrite(t *testing.T) {
	f := newFixture(t)
	gen := f.keys.Generate("pass")
	add := f.contacts.Add("Me", gen.Status.PublicKey, "")

	dir := t.TempDir()
	in := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(in, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(in+".age", []byte("precious"), 0o600); err != nil {
		t.Fatal(err)
	}

	res := f.crypto.Encrypt("j1", in, "", []string{add.Contact.ID})
	if res.Error == nil || res.Error.Code != CodeTargetExists {
		t.Fatalf("Encrypt = %+v, want %s", res.Error, CodeTargetExists)
	}
	if b, _ := os.ReadFile(in + ".age"); string(b) != "precious" {
		t.Error("the existing file was destroyed")
	}
}

// A passphrase file must be openable with the app locked, and Inspect must say
// so, since no key is involved.
func TestCrypto_PassphraseFlowWorksWhileLocked(t *testing.T) {
	f := newFixture(t)

	dir := t.TempDir()
	in := filepath.Join(dir, "a.txt")
	want := []byte("passphrase content")
	if err := os.WriteFile(in, want, 0o600); err != nil {
		t.Fatal(err)
	}

	enc := f.crypto.EncryptWithPassphrase("j1", in, "", "hunter2")
	if enc.Error != nil {
		t.Fatalf("EncryptWithPassphrase: %+v", enc.Error)
	}

	kind := f.crypto.Inspect(enc.Value)
	if kind.Error != nil || kind.Kind != "passphrase" {
		t.Fatalf("Inspect = %+v, want passphrase", kind)
	}

	// Never unlocked at any point in this test.
	dec := f.crypto.DecryptWithPassphrase("j2", enc.Value, filepath.Join(dir, "out.txt"), "hunter2")
	if dec.Error != nil {
		t.Fatalf("DecryptWithPassphrase while locked: %+v", dec.Error)
	}
	got, _ := os.ReadFile(dec.Value)
	if string(got) != string(want) {
		t.Errorf("round-trip = %q, want %q", got, want)
	}
}

func TestCrypto_WrongPassphraseIsReportedAsSuch(t *testing.T) {
	f := newFixture(t)
	dir := t.TempDir()
	in := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(in, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	enc := f.crypto.EncryptWithPassphrase("j1", in, "", "right")
	if enc.Error != nil {
		t.Fatal(enc.Error)
	}

	res := f.crypto.DecryptWithPassphrase("j2", enc.Value, filepath.Join(dir, "out"), "wrong")
	if res.Error == nil || res.Error.Code != CodeWrongPassphrase {
		t.Fatalf("= %+v, want %s", res.Error, CodeWrongPassphrase)
	}
}

func TestCrypto_CancelUnknownJobIsHarmless(t *testing.T) {
	f := newFixture(t)
	if res := f.crypto.Cancel("never-existed"); res.Error != nil {
		t.Errorf("Cancel(unknown) = %+v, want no error", res.Error)
	}
}

func TestCrypto_SuggestedNames(t *testing.T) {
	f := newFixture(t)

	if got := f.crypto.SuggestEncryptOutput("/tmp/report.pdf").Value; got != "/tmp/report.pdf.age" {
		t.Errorf("SuggestEncryptOutput = %q", got)
	}
	if got := f.crypto.SuggestDecryptOutput("/tmp/report.pdf.age").Value; got != "/tmp/report.pdf" {
		t.Errorf("SuggestDecryptOutput = %q", got)
	}
}

func TestMapError_UnknownKeepsItsText(t *testing.T) {
	err := mapError(errors.New("something specific went wrong"))
	if err.Code != CodeInternal {
		t.Errorf("Code = %q, want %s", err.Code, CodeInternal)
	}
	// An unanticipated message is more useful than a reassuring lie.
	if !strings.Contains(err.Message, "something specific") {
		t.Errorf("Message = %q, want the original text preserved", err.Message)
	}
}

func TestMapError_Nil(t *testing.T) {
	if got := mapError(nil); got != nil {
		t.Errorf("mapError(nil) = %+v, want nil", got)
	}
}

func TestMapError_Cancelled(t *testing.T) {
	if got := mapError(context.Canceled); got.Code != CodeCancelled {
		t.Errorf("Code = %q, want %s", got.Code, CodeCancelled)
	}
}

// Every domain error needs a mapping. A new one falling through to INTERNAL
// would show a raw Go string to a non-technical user.
func TestMapError_AllDomainErrorsAreMapped(t *testing.T) {
	for _, err := range []error{
		model.ErrNoIdentity, model.ErrIdentityExists, model.ErrLocked,
		model.ErrWrongPassphrase, model.ErrCorruptIdentity, model.ErrNotForYou,
		model.ErrPassphraseRequired, model.ErrKeyRequired, model.ErrTargetExists,
		model.ErrDuplicateContact, model.ErrContactNotFound, model.ErrNoRecipients,
		model.ErrSecretKeyGiven, model.ErrEmptyPassphrase, model.ErrInvalidSettings,
		model.ErrNotAnIdentityFile,
	} {
		got := mapError(err)
		if got.Code == CodeInternal {
			t.Errorf("%v maps to INTERNAL; it needs a case in mapError", err)
		}
		if got.Message == "" {
			t.Errorf("%v maps to an empty message", err)
		}
	}
}
