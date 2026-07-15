package view

import (
	"dawidroszman.eu/age-gui/internal/model"
	"dawidroszman.eu/age-gui/internal/service"
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
}

// SettingsResult wraps settings.
type SettingsResult struct {
	Settings SettingsDTO `json:"settings"`
	Error    *Error      `json:"error,omitempty"`
}

func settingsDTO(s model.Settings) SettingsDTO {
	return SettingsDTO{
		AutoLockMinutes: s.AutoLockMinutes,
		AutoLockEnabled: s.AutoLockEnabled(),
		MinMinutes:      model.MinAutoLockMinutes,
		MaxMinutes:      model.MaxAutoLockMinutes,
	}
}

// Settings is the Wails-bound handler for preferences.
type Settings struct {
	settings *service.SettingsService
}

// NewSettings builds the handler.
func NewSettings(settings *service.SettingsService) *Settings {
	return &Settings{settings: settings}
}

// Get returns the current settings.
func (h *Settings) Get() SettingsResult {
	return SettingsResult{Settings: settingsDTO(h.settings.Get())}
}

// SetAutoLock configures idle auto-lock. Pass 0 to turn it off.
func (h *Settings) SetAutoLock(minutes int) SettingsResult {
	next := h.settings.Get()
	next.AutoLockMinutes = minutes

	updated, err := h.settings.Update(next)
	if err != nil {
		return SettingsResult{Settings: settingsDTO(updated), Error: mapError(err)}
	}
	return SettingsResult{Settings: settingsDTO(updated)}
}
