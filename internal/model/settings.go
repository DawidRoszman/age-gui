package model

import (
	"fmt"
	"path/filepath"
	"time"
)

// Auto-lock bounds.
const (
	// AutoLockDisabled is the sentinel for "never lock automatically".
	//
	// Disabling must be a first-class, representable choice rather than a
	// grudging one: someone encrypting a long series of files should not be
	// interrupted, and a tool that fights its user gets abandoned for one that
	// does not.
	AutoLockDisabled = 0

	// MinAutoLockMinutes keeps the feature useful. Below a minute the app would
	// lock mid-thought and teach the user to hate it.
	MinAutoLockMinutes = 1

	// MaxAutoLockMinutes caps a value that is effectively "off" anyway (24h).
	// Someone who wants that should choose Disabled and know they chose it.
	MaxAutoLockMinutes = 24 * 60

	// DefaultAutoLockMinutes is the out-of-the-box setting. On by default: the
	// private key sits decrypted in memory while unlocked, and the common case
	// is a laptop that gets walked away from.
	DefaultAutoLockMinutes = 15
)

// Theme selects the colour scheme the UI paints in.
type Theme string

// The available themes.
const (
	// ThemeSystem follows the desktop's light/dark preference. The default:
	// the app runs in a webview alongside every other window on the machine,
	// and one that ignores the desktop looks broken rather than opinionated.
	ThemeSystem Theme = "system"

	// ThemeLight and ThemeDark override the desktop. Worth having despite the
	// default being right for most people: plenty of users run a light desktop
	// and want dark tools (or the reverse), and some can only read one of them
	// comfortably. That is not a preference to make someone re-litigate with
	// their OS settings.
	ThemeLight Theme = "light"
	ThemeDark  Theme = "dark"
)

// DefaultTheme is what a new install starts with.
const DefaultTheme = ThemeSystem

// Valid reports whether t is a theme the app knows how to paint.
func (t Theme) Valid() bool {
	switch t {
	case ThemeSystem, ThemeLight, ThemeDark:
		return true
	}
	return false
}

// Settings holds user preferences.
//
// Preferences only — never secrets, so settings.json is safe to read, sync, and
// back up.
type Settings struct {
	// AutoLockMinutes is the idle period before the key is dropped from
	// memory. AutoLockDisabled means never.
	AutoLockMinutes int `json:"autoLockMinutes"`

	// EncryptDir is where encrypted output is written. Empty means "wherever
	// the OS puts downloads", resolved outside the model: this package must
	// stay free of filesystem knowledge, and an empty value also lets someone
	// who never opens Settings follow their downloads folder if it moves.
	EncryptDir string `json:"encryptDir"`

	// DecryptDir is the same for decrypted output. Kept separate from
	// EncryptDir because the two have genuinely different risk: ciphertext is
	// safe to leave in a shared folder, plaintext often is not.
	DecryptDir string `json:"decryptDir"`

	// Theme is the colour scheme, ThemeSystem to follow the desktop.
	Theme Theme `json:"theme"`
}

// DefaultSettings returns the settings a new install starts with.
//
// Both directories are empty, meaning the downloads folder. Storing the
// resolved path instead would freeze today's location into the file and leave
// a stale absolute path behind if the user ever moved it.
func DefaultSettings() Settings {
	return Settings{
		AutoLockMinutes: DefaultAutoLockMinutes,
		Theme:           DefaultTheme,
	}
}

// Validate checks the settings are usable.
func (s Settings) Validate() error {
	if err := validateSaveDir("encrypted", s.EncryptDir); err != nil {
		return err
	}
	if err := validateSaveDir("decrypted", s.DecryptDir); err != nil {
		return err
	}
	if !s.Theme.Valid() {
		return fmt.Errorf("%w: theme must be %q, %q, or %q",
			ErrInvalidSettings, ThemeSystem, ThemeLight, ThemeDark)
	}
	if s.AutoLockMinutes == AutoLockDisabled {
		return nil
	}
	if s.AutoLockMinutes < MinAutoLockMinutes || s.AutoLockMinutes > MaxAutoLockMinutes {
		return fmt.Errorf("%w: auto-lock must be off, or between %d and %d minutes",
			ErrInvalidSettings, MinAutoLockMinutes, MaxAutoLockMinutes)
	}
	return nil
}

// validateSaveDir rejects a save location that could not work.
//
// Only the shape is checked here. Whether the directory exists is a question
// about the world, not about the value, and the answer changes between the
// moment settings are saved and the moment a file is written — so the writer
// deals with that, not this.
func validateSaveDir(which, dir string) error {
	if dir == "" {
		return nil // the downloads folder
	}
	// A relative path would resolve against the process's working directory,
	// which for a double-clicked desktop app is arbitrary — "/" on some Linux
	// launchers. Files would land somewhere the user could not predict.
	if !filepath.IsAbs(dir) {
		return fmt.Errorf("%w: the folder for %s files must be a full path, got %q",
			ErrInvalidSettings, which, dir)
	}
	return nil
}

// AutoLockEnabled reports whether idle auto-lock is on.
func (s Settings) AutoLockEnabled() bool {
	return s.AutoLockMinutes != AutoLockDisabled
}

// IdleTimeout returns the auto-lock period, or zero when disabled.
func (s Settings) IdleTimeout() time.Duration {
	if !s.AutoLockEnabled() {
		return 0
	}
	return time.Duration(s.AutoLockMinutes) * time.Minute
}
