package storage

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"dawidroszman.eu/age-gui/internal/model"
	"filippo.io/age"
)

func testKey(t *testing.T) model.PublicKey {
	t.Helper()
	id, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	k, err := model.ParsePublicKey(id.Recipient().String())
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func TestIdentity_RoundTrip(t *testing.T) {
	s := NewIdentity(t.TempDir())

	exists, err := s.Exists()
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if exists {
		t.Fatal("Exists() = true on a fresh directory")
	}

	want := []byte("ciphertext-blob")
	if err := s.Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	exists, err = s.Exists()
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if !exists {
		t.Fatal("Exists() = false after Save")
	}

	got, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("Load() = %q, want %q", got, want)
	}
}

// The private key must never be readable by other users on the machine.
func TestIdentity_FilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not enforced on Windows")
	}
	s := NewIdentity(t.TempDir())
	if err := s.Save([]byte("secret")); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("identity file mode = %o, want 600", perm)
	}
}

func TestDefaultDir_Permissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not enforced on Windows")
	}
	// Redirect the config dir so the test never touches the real one.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	dir, err := DefaultDir()
	if err != nil {
		t.Fatalf("DefaultDir: %v", err)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Errorf("config dir mode = %o, want 700", perm)
	}
	if filepath.Base(dir) != appDir {
		t.Errorf("DefaultDir() = %q, want it to end in %q", dir, appDir)
	}
}

// DefaultDir must be callable repeatedly; MkdirAll on an existing dir is fine
// and must not clobber it.
func TestDefaultDir_Idempotent(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	first, err := DefaultDir()
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(first, "marker")
	if err := os.WriteFile(marker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	second, err := DefaultDir()
	if err != nil {
		t.Fatalf("second DefaultDir: %v", err)
	}
	if first != second {
		t.Errorf("DefaultDir() not stable: %q then %q", first, second)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("existing config dir contents were disturbed: %v", err)
	}
}

// The whole point of the atomic helper: an overwrite must leave either the old
// bytes or the new bytes, never a mix, and never a stray temp file holding key
// material.
func TestWriteFileAtomic_OverwriteLeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "identity.age")

	for _, data := range []string{"first", "second-much-longer-than-the-first", "third"} {
		if err := writeFileAtomic(path, []byte(data), 0o600); err != nil {
			t.Fatalf("writeFileAtomic(%q): %v", data, err)
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != data {
			t.Fatalf("read back %q, want %q", got, data)
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("directory holds %v, want only identity.age — a leftover temp file would leak key material", names)
	}
}

func TestContacts_EmptyWhenMissing(t *testing.T) {
	s := NewContacts(t.TempDir())

	all, err := s.List()
	if err != nil {
		t.Fatalf("List on fresh dir: %v", err)
	}
	if len(all) != 0 {
		t.Errorf("List() = %d contacts, want 0", len(all))
	}
}

func TestContacts_PutListDelete(t *testing.T) {
	s := NewContacts(t.TempDir())

	alice, err := model.NewContact("Alice", testKey(t), "")
	if err != nil {
		t.Fatal(err)
	}
	bob, err := model.NewContact("Bob", testKey(t), "note")
	if err != nil {
		t.Fatal(err)
	}

	if err := s.Put(alice); err != nil {
		t.Fatalf("Put(alice): %v", err)
	}
	if err := s.Put(bob); err != nil {
		t.Fatalf("Put(bob): %v", err)
	}

	all, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("List() = %d, want 2", len(all))
	}
	// The public key must survive the disk round-trip in usable form.
	for _, c := range all {
		if c.PublicKey.Recipient() == nil {
			t.Errorf("contact %q loaded without a usable recipient", c.Name)
		}
	}

	if err := s.Delete(alice.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	all, err = s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || all[0].ID != bob.ID {
		t.Errorf("after Delete(alice), List() = %v, want just Bob", all)
	}
}

func TestContacts_PutReplacesSameID(t *testing.T) {
	s := NewContacts(t.TempDir())

	c, err := model.NewContact("Alice", testKey(t), "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Put(c); err != nil {
		t.Fatal(err)
	}

	c.Name = "Alice Renamed"
	if err := s.Put(c); err != nil {
		t.Fatal(err)
	}

	all, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("List() = %d, want 1 — Put with an existing id must replace, not append", len(all))
	}
	if all[0].Name != "Alice Renamed" {
		t.Errorf("Name = %q, want the updated value", all[0].Name)
	}
}

func TestContacts_DeleteMissing(t *testing.T) {
	s := NewContacts(t.TempDir())

	if err := s.Delete("nope"); !errors.Is(err, model.ErrContactNotFound) {
		t.Errorf("Delete(missing) = %v, want ErrContactNotFound", err)
	}
}

func TestContacts_CorruptFileNamesThePath(t *testing.T) {
	dir := t.TempDir()
	s := NewContacts(dir)
	if err := os.WriteFile(s.Path(), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := s.List()
	if err == nil {
		t.Fatal("List() on a corrupt file = nil, want error")
	}
	if !strings.Contains(err.Error(), s.Path()) {
		t.Errorf("error %q does not name the offending file; the user cannot fix what we will not point at", err)
	}
}

// A file from a future version must be refused rather than silently truncated
// back to fields this build happens to know.
func TestContacts_RejectsNewerFormat(t *testing.T) {
	s := NewContacts(t.TempDir())
	if err := os.WriteFile(s.Path(), []byte(`{"version":99,"contacts":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := s.List(); err == nil {
		t.Error("List() on a newer-format file = nil, want error")
	}
}

// Wails runs each JS call on its own goroutine, so concurrent Puts are real.
// Without the mutex this drops contacts via interleaved read-modify-write.
func TestContacts_ConcurrentPut(t *testing.T) {
	s := NewContacts(t.TempDir())

	const n = 20
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, err := model.NewContact("Someone", testKey(t), "")
			if err != nil {
				errs <- err
				return
			}
			if err := s.Put(c); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent Put: %v", err)
	}

	all, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != n {
		t.Errorf("List() = %d contacts, want %d — a concurrent write was lost", len(all), n)
	}
}
