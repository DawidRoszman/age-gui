package model

import (
	"errors"
	"runtime"
	"testing"
)

func TestDefaultSettings_LeavesSaveFoldersUnset(t *testing.T) {
	s := DefaultSettings()

	// Empty means "the downloads folder", resolved at run time. Baking today's
	// path in here would strand the setting if the folder ever moved, and would
	// drag filesystem knowledge into the model.
	if s.EncryptDir != "" || s.DecryptDir != "" {
		t.Errorf("DefaultSettings save folders = %q/%q, want both empty",
			s.EncryptDir, s.DecryptDir)
	}
	if err := s.Validate(); err != nil {
		t.Errorf("the defaults must be valid: %v", err)
	}
}

func TestSettings_ValidateAcceptsAbsoluteSaveFolders(t *testing.T) {
	dir := "/srv/secrets"
	if runtime.GOOS == "windows" {
		dir = `C:\secrets`
	}
	s := Settings{AutoLockMinutes: DefaultAutoLockMinutes, EncryptDir: dir, DecryptDir: dir}

	if err := s.Validate(); err != nil {
		t.Errorf("Validate(%q) = %v, want nil", dir, err)
	}
}

// A relative folder resolves against the working directory, which for a
// double-clicked app is arbitrary. Files would land somewhere unpredictable, so
// this has to be refused at the boundary rather than discovered later.
func TestSettings_ValidateRejectsRelativeSaveFolders(t *testing.T) {
	for _, tc := range []struct {
		name string
		s    Settings
	}{
		{"encrypt", Settings{AutoLockMinutes: 15, EncryptDir: "relative/path"}},
		{"decrypt", Settings{AutoLockMinutes: 15, DecryptDir: "relative/path"}},
		{"bare name", Settings{AutoLockMinutes: 15, EncryptDir: "Downloads"}},
		{"dot", Settings{AutoLockMinutes: 15, DecryptDir: "."}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.s.Validate()
			if err == nil {
				t.Fatal("Validate = nil, want a rejection")
			}
			if !errors.Is(err, ErrInvalidSettings) {
				t.Errorf("Validate = %v, want ErrInvalidSettings", err)
			}
		})
	}
}

// Auto-lock and the save folders are independent; a bad folder must not be
// reported as an auto-lock problem, and vice versa.
func TestSettings_ValidateStillChecksAutoLock(t *testing.T) {
	s := Settings{AutoLockMinutes: MaxAutoLockMinutes + 1}

	if err := s.Validate(); !errors.Is(err, ErrInvalidSettings) {
		t.Errorf("Validate = %v, want ErrInvalidSettings", err)
	}
}
