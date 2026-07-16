package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"dawidroszman.eu/encryptor/internal/model"
)

// settingsFileVersion tracks the on-disk format.
const settingsFileVersion = 1

type settingsDoc struct {
	Version  int            `json:"version"`
	Settings model.Settings `json:"settings"`
}

// Settings persists user preferences as JSON.
type Settings struct {
	mu   sync.Mutex
	path string
}

// NewSettings returns a store rooted at dir.
func NewSettings(dir string) *Settings {
	return &Settings{path: filepath.Join(dir, settingsFile)}
}

// Path reports the settings file location.
func (s *Settings) Path() string { return s.path }

// Load returns the stored settings.
//
// A missing file means a fresh install, so the defaults are returned rather
// than an error.
//
// A *corrupt* file also falls back to defaults rather than failing. Settings
// are preferences, not data: refusing to start the app because a preferences
// file got mangled would be a self-inflicted outage over something the user can
// simply set again. Note the fallback is the safe direction — auto-lock ends up
// on, not off.
func (s *Settings) Load() (model.Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	b, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return model.DefaultSettings(), nil
	}
	if err != nil {
		return model.DefaultSettings(), fmt.Errorf("read settings file: %w", err)
	}

	var doc settingsDoc
	if err := json.Unmarshal(b, &doc); err != nil {
		return model.DefaultSettings(), nil
	}
	// A file written before the theme existed has no theme field, which decodes
	// to the empty string. Fill it in before validating: without this the whole
	// file would fail validation and every upgrading user would silently lose
	// the auto-lock period they had chosen.
	if doc.Settings.Theme == "" {
		doc.Settings.Theme = model.DefaultTheme
	}
	// A value from a future version, or one hand-edited out of range, must not
	// silently become an unlocked-forever session.
	if err := doc.Settings.Validate(); err != nil {
		return model.DefaultSettings(), nil
	}
	return doc.Settings, nil
}

// Save atomically writes the settings.
func (s *Settings) Save(settings model.Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	b, err := json.MarshalIndent(settingsDoc{
		Version:  settingsFileVersion,
		Settings: settings,
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode settings: %w", err)
	}
	b = append(b, '\n')
	return writeFileAtomic(s.path, b, dataPerm)
}
