package view

import (
	"dawidroszman.eu/encryptor/internal/model"
	"dawidroszman.eu/encryptor/internal/service"
)

// EventAutoLocked is emitted when the key is dropped after an idle period, so
// the UI can send the user to the unlock screen instead of leaving them
// clicking buttons that have quietly stopped working.
const EventAutoLocked = "keys:auto-locked"

// SettingsDTO is the settings as the UI sees them.
type SettingsDTO struct {
	// AutoLockMinutes is 0 when auto-lock is off.
	AutoLockMinutes int `json:"autoLockMinutes"`
	// AutoLockEnabled saves the frontend from re-deriving what 0 means.
	AutoLockEnabled bool `json:"autoLockEnabled"`
	// Bounds so the UI can build its control and validate without hardcoding
	// numbers that would drift from the Go side.
	MinMinutes int `json:"minMinutes"`
	MaxMinutes int `json:"maxMinutes"`

	// EncryptDir and DecryptDir are resolved absolute paths, never the empty
	// "use the default" sentinel: the UI's job is to show the user where files
	// actually go, and "" would tell them nothing.
	EncryptDir string `json:"encryptDir"`
	DecryptDir string `json:"decryptDir"`
	// UsingDefault flags say whether the path above is merely the default
	// rather than a choice, so the UI can label it and offer to reset only
	// when that would do something.
	EncryptDirIsDefault bool `json:"encryptDirIsDefault"`
	DecryptDirIsDefault bool `json:"decryptDirIsDefault"`
	// DefaultDir is the downloads folder, so the UI can name what "reset"
	// means without guessing.
	DefaultDir string `json:"defaultDir"`

	// Theme is "system", "light", or "dark".
	Theme string `json:"theme"`
}

// SettingsResult wraps settings.
type SettingsResult struct {
	Settings SettingsDTO `json:"settings"`
	Error    *Error      `json:"error,omitempty"`
}

// Settings is the Wails-bound handler for preferences.
type Settings struct {
	settings *service.SettingsService
	platform Platform
}

// NewSettings builds the handler.
func NewSettings(settings *service.SettingsService, platform Platform) *Settings {
	return &Settings{settings: settings, platform: platform}
}

// dto renders settings for the UI.
//
// A method rather than a function because the resolved directories come from
// the service: the stored value may be empty, and only the service knows what
// that resolves to.
func (h *Settings) dto(s model.Settings) SettingsDTO {
	return SettingsDTO{
		AutoLockMinutes:     s.AutoLockMinutes,
		AutoLockEnabled:     s.AutoLockEnabled(),
		MinMinutes:          model.MinAutoLockMinutes,
		MaxMinutes:          model.MaxAutoLockMinutes,
		EncryptDir:          h.settings.EncryptDir(),
		DecryptDir:          h.settings.DecryptDir(),
		EncryptDirIsDefault: s.EncryptDir == "",
		DecryptDirIsDefault: s.DecryptDir == "",
		DefaultDir:          h.settings.DefaultSaveDir(),
		Theme:               string(s.Theme),
	}
}

// Get returns the current settings.
func (h *Settings) Get() SettingsResult {
	return SettingsResult{Settings: h.dto(h.settings.Get())}
}

// SetAutoLock configures idle auto-lock. Pass 0 to turn it off.
func (h *Settings) SetAutoLock(minutes int) SettingsResult {
	next := h.settings.Get()
	next.AutoLockMinutes = minutes
	return h.update(next)
}

// SetEncryptDir sets where encrypted files are written. Pass "" to go back to
// the downloads folder.
func (h *Settings) SetEncryptDir(dir string) SettingsResult {
	next := h.settings.Get()
	next.EncryptDir = dir
	return h.update(next)
}

// SetDecryptDir sets where decrypted files are written. Pass "" to go back to
// the downloads folder.
func (h *Settings) SetDecryptDir(dir string) SettingsResult {
	next := h.settings.Get()
	next.DecryptDir = dir
	return h.update(next)
}

// ChooseDir opens a folder picker starting at the current setting, and returns
// the chosen path. "" means the user cancelled, which is not an error.
//
// Picking and saving are separate calls so a cancelled dialog cannot be
// mistaken for "reset to default" -- both would arrive here as "".
func (h *Settings) ChooseDir(title, startDir string) StringResult {
	dir, err := h.platform.OpenDirectoryDialog(title, startDir)
	if err != nil {
		return StringResult{Error: mapError(err)}
	}
	return StringResult{Value: dir}
}

// update persists and renders, keeping the rejected-value path identical for
// every setter: on failure the UI is handed what is still in force, not the
// value that was refused.
func (h *Settings) update(next model.Settings) SettingsResult {
	updated, err := h.settings.Update(next)
	if err != nil {
		return SettingsResult{Settings: h.dto(updated), Error: mapError(err)}
	}
	return SettingsResult{Settings: h.dto(updated)}
}

// SetTheme configures the colour scheme: "system", "light", or "dark".
//
// An unknown value is rejected rather than quietly treated as "system", so a
// typo surfaces here instead of as a theme that mysteriously will not stick.
func (h *Settings) SetTheme(theme string) SettingsResult {
	next := h.settings.Get()
	next.Theme = model.Theme(theme)
	return h.update(next)
}
