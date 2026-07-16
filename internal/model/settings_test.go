package model

import (
	"errors"
	"runtime"
	"strings"
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

// valid returns settings that pass Validate, for a test to spoil one field of.
//
// Built from the defaults rather than a literal: a literal silently acquires a
// zero value for every field added later, so these tests would start failing --
// or worse, passing for the wrong reason -- each time a setting appears.
func valid() Settings { return DefaultSettings() }

func TestSettings_ValidateAcceptsAbsoluteSaveFolders(t *testing.T) {
	dir := "/srv/secrets"
	if runtime.GOOS == "windows" {
		dir = `C:\secrets`
	}
	s := valid()
	s.EncryptDir, s.DecryptDir = dir, dir

	if err := s.Validate(); err != nil {
		t.Errorf("Validate(%q) = %v, want nil", dir, err)
	}
}

// A relative folder resolves against the working directory, which for a
// double-clicked app is arbitrary. Files would land somewhere unpredictable, so
// this has to be refused at the boundary rather than discovered later.
func TestSettings_ValidateRejectsRelativeSaveFolders(t *testing.T) {
	for _, tc := range []struct {
		name    string
		spoil   func(*Settings)
		wantDir string
	}{
		{"encrypt", func(s *Settings) { s.EncryptDir = "relative/path" }, "encrypted"},
		{"decrypt", func(s *Settings) { s.DecryptDir = "relative/path" }, "decrypted"},
		{"bare name", func(s *Settings) { s.EncryptDir = "Downloads" }, "encrypted"},
		{"dot", func(s *Settings) { s.DecryptDir = "." }, "decrypted"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := valid()
			tc.spoil(&s)

			err := s.Validate()
			if err == nil {
				t.Fatal("Validate = nil, want a rejection")
			}
			if !errors.Is(err, ErrInvalidSettings) {
				t.Errorf("Validate = %v, want ErrInvalidSettings", err)
			}
			// Everything else about these settings is valid, so the complaint
			// must name the folder. Without this the test would pass on any
			// rejection at all, including one about an unrelated field.
			if !strings.Contains(err.Error(), tc.wantDir) {
				t.Errorf("Validate = %v, want it to name the %s folder", err, tc.wantDir)
			}
		})
	}
}

// Auto-lock and the save folders are independent; a bad folder must not be
// reported as an auto-lock problem, and vice versa.
func TestSettings_ValidateStillChecksAutoLock(t *testing.T) {
	s := valid()
	s.AutoLockMinutes = MaxAutoLockMinutes + 1

	err := s.Validate()
	if !errors.Is(err, ErrInvalidSettings) {
		t.Fatalf("Validate = %v, want ErrInvalidSettings", err)
	}
	if !strings.Contains(err.Error(), "auto-lock") {
		t.Errorf("Validate = %v, want it to name auto-lock", err)
	}
}
