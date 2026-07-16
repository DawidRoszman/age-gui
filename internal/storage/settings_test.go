package storage

import (
	"os"
	"strings"
	"testing"

	"dawidroszman.eu/encryptor/internal/model"
)

func TestSettings_DefaultsWhenMissing(t *testing.T) {
	s := NewSettings(t.TempDir())

	got, err := s.Load()
	if err != nil {
		t.Fatalf("Load on a fresh dir: %v", err)
	}
	if got != model.DefaultSettings() {
		t.Errorf("Load() = %+v, want the defaults", got)
	}
}

func TestSettings_RoundTrip(t *testing.T) {
	s := NewSettings(t.TempDir())
	want := model.Settings{AutoLockMinutes: 42, Theme: model.ThemeDark}

	if err := s.Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got != want {
		t.Errorf("Load() = %+v, want %+v", got, want)
	}
}

// Disabled must survive the round trip. It is the one value that a naive
// "zero means unset, use the default" would silently discard.
func TestSettings_DisabledSurvivesRoundTrip(t *testing.T) {
	s := NewSettings(t.TempDir())

	if err := s.Save(model.Settings{AutoLockMinutes: model.AutoLockDisabled}); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.AutoLockEnabled() {
		t.Fatal("the user disabled auto-lock and it came back enabled after a reload")
	}
}

// The theme was added after the first release, so files in the wild have no
// theme field. Reading one must leave the user's other preferences alone: the
// whole file failing validation over the missing field would reset the
// auto-lock period they deliberately chose, which they would discover only by
// noticing the app locking at the wrong time.
func TestSettings_FileWithoutThemeKeepsOtherPreferences(t *testing.T) {
	dir := t.TempDir()
	s := NewSettings(dir)
	if err := os.WriteFile(s.Path(), []byte(`{"version":1,"settings":{"autoLockMinutes":45}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.AutoLockMinutes != 45 {
		t.Errorf("AutoLockMinutes = %d, want the stored 45", got.AutoLockMinutes)
	}
	if got.Theme != model.DefaultTheme {
		t.Errorf("Theme = %q, want the default %q", got.Theme, model.DefaultTheme)
	}
}

// A theme this build does not know how to paint is not worth resetting the
// whole file over, but it must not be honoured either.
func TestSettings_UnknownThemeFallsBackToDefaults(t *testing.T) {
	dir := t.TempDir()
	s := NewSettings(dir)
	body := `{"version":1,"settings":{"autoLockMinutes":45,"theme":"solarized"}}`
	if err := os.WriteFile(s.Path(), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !got.Theme.Valid() {
		t.Errorf("Load() = %q, want a theme the app can paint", got.Theme)
	}
}

// Preferences are not data. Refusing to start over a mangled preferences file
// would be a self-inflicted outage, so a corrupt file falls back to defaults —
// and note the fallback direction is the safe one: auto-lock ends up ON.
func TestSettings_CorruptFileFallsBackToSafeDefaults(t *testing.T) {
	for name, body := range map[string]string{
		"not json":       "{not json at all",
		"wrong types":    `{"version":1,"settings":{"autoLockMinutes":"fifteen"}}`,
		"out of range":   `{"version":1,"settings":{"autoLockMinutes":-99}}`,
		"future version": `{"version":99,"settings":{"autoLockMinutes":-5}}`,
		"empty":          ``,
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			s := NewSettings(dir)
			if err := os.WriteFile(s.Path(), []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}

			got, err := s.Load()
			if err != nil {
				t.Fatalf("Load(corrupt) = %v, want a graceful fallback", err)
			}
			if got != model.DefaultSettings() {
				t.Errorf("Load(corrupt) = %+v, want the defaults", got)
			}
			if !got.AutoLockEnabled() {
				t.Error("a corrupt settings file left auto-lock off; the fallback must be the safe direction")
			}
		})
	}
}

func TestSettings_FileHasNoSecrets(t *testing.T) {
	dir := t.TempDir()
	s := NewSettings(dir)
	if err := s.Save(model.Settings{AutoLockMinutes: 10}); err != nil {
		t.Fatal(err)
	}

	b, err := os.ReadFile(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	if len(b) == 0 {
		t.Fatal("settings file is empty")
	}
	// Settings are preferences only; nothing here should ever be sensitive.
	if strings.Contains(string(b), "AGE-SECRET-KEY") || strings.Contains(string(b), "passphrase") {
		t.Errorf("settings file contains something sensitive: %s", b)
	}
}
