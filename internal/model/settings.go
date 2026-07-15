package model

import (
	"fmt"
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

// Settings holds user preferences.
//
// Preferences only — never secrets, so settings.json is safe to read, sync, and
// back up.
type Settings struct {
	// AutoLockMinutes is the idle period before the key is dropped from
	// memory. AutoLockDisabled means never.
	AutoLockMinutes int `json:"autoLockMinutes"`
}

// DefaultSettings returns the settings a new install starts with.
func DefaultSettings() Settings {
	return Settings{AutoLockMinutes: DefaultAutoLockMinutes}
}

// Validate checks the settings are usable.
func (s Settings) Validate() error {
	if s.AutoLockMinutes == AutoLockDisabled {
		return nil
	}
	if s.AutoLockMinutes < MinAutoLockMinutes || s.AutoLockMinutes > MaxAutoLockMinutes {
		return fmt.Errorf("%w: auto-lock must be off, or between %d and %d minutes",
			ErrInvalidSettings, MinAutoLockMinutes, MaxAutoLockMinutes)
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
