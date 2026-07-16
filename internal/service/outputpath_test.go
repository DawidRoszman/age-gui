package service

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOutputPath_UsesTheSaveFolderNotTheInputFolder(t *testing.T) {
	dir := t.TempDir()

	got, err := OutputPath(dir, "/somewhere/else/report.pdf", EncryptedName)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "report.pdf.age"); got != want {
		t.Errorf("OutputPath = %q, want %q", got, want)
	}
}

// Numbering keeps the extension on the end, or the file stops opening by
// double-click -- "report (2).pdf.age" would be wrong in a way the user only
// discovers later.
func TestOutputPath_NumbersBeforeTheExtension(t *testing.T) {
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "report.pdf.age"))

	got, err := OutputPath(dir, "/in/report.pdf", EncryptedName)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "report.pdf (2).age"); got != want {
		t.Errorf("OutputPath = %q, want %q", got, want)
	}
}

func TestOutputPath_CountsUpPastEveryTakenName(t *testing.T) {
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "a.txt"))
	touch(t, filepath.Join(dir, "a (2).txt"))
	touch(t, filepath.Join(dir, "a (3).txt"))

	got, err := OutputPath(dir, "/in/a.txt.age", DecryptedName)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "a (4).txt"); got != want {
		t.Errorf("OutputPath = %q, want %q", got, want)
	}
}

// A dotfile is all extension as far as filepath.Ext is concerned. Numbering it
// naively yields " (2).bashrc", which loses the name entirely.
func TestOutputPath_DotfileKeepsItsName(t *testing.T) {
	dir := t.TempDir()
	touch(t, filepath.Join(dir, ".bashrc.age"))

	got, err := OutputPath(dir, "/in/.bashrc", EncryptedName)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, ".bashrc (2).age"); got != want {
		t.Errorf("OutputPath = %q, want %q", got, want)
	}
}

// An input with no extension at all must not grow a stray dot.
func TestOutputPath_ExtensionlessName(t *testing.T) {
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "README.age"))

	got, err := OutputPath(dir, "/in/README", EncryptedName)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "README (2).age"); got != want {
		t.Errorf("OutputPath = %q, want %q", got, want)
	}
}

// The downloads folder may simply not exist on a fresh account, and a first
// encrypt must not fail because of that.
func TestOutputPath_CreatesTheFolder(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "not", "there", "yet")

	got, err := OutputPath(dir, "/in/a.txt", EncryptedName)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("folder was not created: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("not a directory")
	}
	// A folder we create is about to hold a secret, so it must not be readable
	// by anyone else.
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Errorf("created folder mode = %o, want 700", perm)
	}
	if want := filepath.Join(dir, "a.txt.age"); got != want {
		t.Errorf("OutputPath = %q, want %q", got, want)
	}
}

// A name is only free until someone else takes it. The returned path reserves
// nothing, so callers must pass Refuse -- this pins that the collision is
// detectable rather than silently overwritten.
func TestOutputPath_DoesNotReserveTheName(t *testing.T) {
	dir := t.TempDir()

	got, err := OutputPath(dir, "/in/a.txt", EncryptedName)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(got); !os.IsNotExist(err) {
		t.Errorf("OutputPath created %s; it must only propose a name", got)
	}
}

// A symlink occupies a name even though its target may not exist. Following it
// would write through to wherever it points.
func TestOutputPath_TreatsASymlinkAsTaken(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, "a.txt.age")
	if err := os.Symlink(filepath.Join(dir, "nowhere"), link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	got, err := OutputPath(dir, "/in/a.txt", EncryptedName)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "a.txt (2).age"); got != want {
		t.Errorf("OutputPath = %q, want %q -- a dangling symlink still owns the name", got, want)
	}
}

func TestOutputPath_EmptyFolderIsRejected(t *testing.T) {
	if _, err := OutputPath("", "/in/a.txt", EncryptedName); err == nil {
		t.Error("OutputPath(\"\") = nil error, want a refusal rather than a write to the working directory")
	}
}

func touch(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
}
