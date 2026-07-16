package service

import (
	"sync"

	"dawidroszman.eu/encryptor/internal/model"
)

// SettingsService reads and writes user preferences, and applies the ones that
// other services need to act on.
//
// It owns the KeyService reference rather than the other way round: preferences
// are a UI concern that reach into the domain, not something the key logic
// should have to know about.
type SettingsService struct {
	store SettingsStore
	keys  *KeyService

	mu      sync.RWMutex
	current model.Settings
}

// NewSettingsService builds the service and applies whatever is stored.
//
// Applying at construction matters: without it the app would run with auto-lock
// off until the user happened to open the settings screen, silently ignoring a
// preference they set weeks ago.
func NewSettingsService(store SettingsStore, keys *KeyService) (*SettingsService, error) {
	s := &SettingsService{store: store, keys: keys}

	loaded, err := store.Load()
	if err != nil {
		return nil, err
	}
	s.current = loaded
	s.apply(loaded)
	return s, nil
}

// Get returns the current settings.
func (s *SettingsService) Get() model.Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.current
}

// Update validates, persists, and applies new settings.
//
// Validation comes before persistence so a rejected value never reaches the
// disk, and persistence before application so a failed write cannot leave the
// running app disagreeing with the file it will read next launch.
func (s *SettingsService) Update(next model.Settings) (model.Settings, error) {
	if err := next.Validate(); err != nil {
		return s.Get(), err
	}
	if err := s.store.Save(next); err != nil {
		return s.Get(), err
	}

	s.mu.Lock()
	s.current = next
	s.mu.Unlock()

	s.apply(next)
	return next, nil
}

// apply pushes settings into the services that act on them.
func (s *SettingsService) apply(settings model.Settings) {
	// Zero when disabled, which KeyService reads as "never auto-lock".
	s.keys.SetIdleTimeout(settings.IdleTimeout())
}
