package view

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dawidroszman.eu/encryptor/internal/model"
	"dawidroszman.eu/encryptor/internal/service"
)

// memSettingsStore is an in-memory service.SettingsStore.
type memSettingsStore struct {
	settings model.Settings
	saved    bool
}

func (m *memSettingsStore) Load() (model.Settings, error) {
	if !m.saved {
		return model.DefaultSettings(), nil
	}
	return m.settings, nil
}

func (m *memSettingsStore) Save(s model.Settings) error {
	m.settings = s
	m.saved = true
	return nil
}

func newSettingsHandler(t *testing.T) (*Settings, *service.KeyService) {
	t.Helper()
	keys := service.NewKeyService(&memIdentityStore{})
	svc, err := service.NewSettingsService(&memSettingsStore{}, keys, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return NewSettings(svc, &fakePlatform{}), keys
}

func TestSettingsHandler_GetDefaults(t *testing.T) {
	h, _ := newSettingsHandler(t)

	res := h.Get()
	if res.Error != nil {
		t.Fatalf("Get: %+v", res.Error)
	}
	if !res.Settings.AutoLockEnabled {
		t.Error("auto-lock is off by default")
	}
	if res.Settings.AutoLockMinutes != model.DefaultAutoLockMinutes {
		t.Errorf("AutoLockMinutes = %d, want %d", res.Settings.AutoLockMinutes, model.DefaultAutoLockMinutes)
	}
	// Bounds ship to the UI so it need not hardcode numbers that would drift.
	if res.Settings.MinMinutes != model.MinAutoLockMinutes || res.Settings.MaxMinutes != model.MaxAutoLockMinutes {
		t.Errorf("bounds = %d..%d, want %d..%d",
			res.Settings.MinMinutes, res.Settings.MaxMinutes,
			model.MinAutoLockMinutes, model.MaxAutoLockMinutes)
	}
}

func TestSettingsHandler_SetAutoLock(t *testing.T) {
	h, _ := newSettingsHandler(t)

	res := h.SetAutoLock(30)
	if res.Error != nil {
		t.Fatalf("SetAutoLock(30): %+v", res.Error)
	}
	if res.Settings.AutoLockMinutes != 30 || !res.Settings.AutoLockEnabled {
		t.Errorf("= %+v, want 30 minutes and enabled", res.Settings)
	}
	// And it must survive a re-read.
	if got := h.Get().Settings.AutoLockMinutes; got != 30 {
		t.Errorf("Get() = %d after set, want 30", got)
	}
}

// The user must be able to turn it off, and it must stay off.
func TestSettingsHandler_DisableAutoLock(t *testing.T) {
	h, _ := newSettingsHandler(t)

	res := h.SetAutoLock(model.AutoLockDisabled)
	if res.Error != nil {
		t.Fatalf("SetAutoLock(0): %+v", res.Error)
	}
	if res.Settings.AutoLockEnabled {
		t.Error("AutoLockEnabled = true after disabling")
	}
	if got := h.Get(); got.Settings.AutoLockEnabled {
		t.Error("auto-lock came back on after a re-read")
	}
}

func TestSettingsHandler_RejectsOutOfRange(t *testing.T) {
	h, _ := newSettingsHandler(t)

	res := h.SetAutoLock(-5)
	if res.Error == nil {
		t.Fatal("SetAutoLock(-5) = nil, want an error")
	}
	if res.Error.Code != CodeInvalidSettings {
		t.Errorf("Code = %q, want %s", res.Error.Code, CodeInvalidSettings)
	}
	// A rejected value must still report the settings actually in force.
	if res.Settings.AutoLockMinutes != model.DefaultAutoLockMinutes {
		t.Errorf("= %d after a rejected update, want the unchanged default", res.Settings.AutoLockMinutes)
	}
}

func TestKeys_BackupAndRestore(t *testing.T) {
	f := newFixture(t)
	gen := f.keys.Generate("a passphrase")
	if gen.Error != nil {
		t.Fatal(gen.Error)
	}

	backup := filepath.Join(t.TempDir(), "backup.age")
	f.platform.savePath = backup

	res := f.keys.Backup()
	if res.Error != nil {
		t.Fatalf("Backup: %+v", res.Error)
	}
	if res.Value != backup {
		t.Errorf("Value = %q, want the chosen path", res.Value)
	}

	b, err := os.ReadFile(backup)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToUpper(string(b)), "AGE-SECRET-KEY") {
		t.Fatal("the backup file contains a plaintext private key")
	}

	// A fresh install restoring from that backup.
	fresh := newFixture(t)
	fresh.platform.openPath = backup

	restored := fresh.keys.Restore("a passphrase")
	if restored.Error != nil {
		t.Fatalf("Restore: %+v", restored.Error)
	}
	if !restored.Status.Exists || !restored.Status.Unlocked {
		t.Errorf("after restore, status = %+v, want existing and unlocked", restored.Status)
	}
	if restored.Status.PublicKey != gen.Status.PublicKey {
		t.Error("the restored key is not the original key")
	}
}

func TestKeys_BackupCancelledIsNotAnError(t *testing.T) {
	f := newFixture(t)
	if res := f.keys.Generate("pass"); res.Error != nil {
		t.Fatal(res.Error)
	}
	f.platform.savePath = "" // cancelled

	res := f.keys.Backup()
	if res.Error != nil {
		t.Errorf("cancelling a backup reported an error: %+v", res.Error)
	}
	if res.Value != "" {
		t.Errorf("Value = %q, want empty on cancel", res.Value)
	}
}

func TestKeys_RestoreCancelledIsNotAnError(t *testing.T) {
	f := newFixture(t)
	f.platform.openPath = "" // cancelled

	res := f.keys.Restore("pass")
	if res.Error != nil {
		t.Errorf("cancelling a restore reported an error: %+v", res.Error)
	}
	if res.Status.Exists {
		t.Error("a cancelled restore reported an existing key")
	}
}

func TestKeys_RestoreWrongPassphrase(t *testing.T) {
	f := newFixture(t)
	if res := f.keys.Generate("right"); res.Error != nil {
		t.Fatal(res.Error)
	}
	backup := filepath.Join(t.TempDir(), "b.age")
	f.platform.savePath = backup
	if res := f.keys.Backup(); res.Error != nil {
		t.Fatal(res.Error)
	}

	fresh := newFixture(t)
	fresh.platform.openPath = backup

	res := fresh.keys.Restore("wrong")
	if res.Error == nil || res.Error.Code != CodeWrongPassphrase {
		t.Fatalf("Restore(wrong) = %+v, want %s", res.Error, CodeWrongPassphrase)
	}
}

// Restoring over an existing key would orphan every file encrypted to it.
func TestKeys_RestoreOverExistingKeyIsRefused(t *testing.T) {
	f := newFixture(t)
	if res := f.keys.Generate("mine"); res.Error != nil {
		t.Fatal(res.Error)
	}
	backup := filepath.Join(t.TempDir(), "b.age")
	f.platform.savePath = backup
	if res := f.keys.Backup(); res.Error != nil {
		t.Fatal(res.Error)
	}

	f.platform.openPath = backup
	res := f.keys.Restore("mine")
	if res.Error == nil || res.Error.Code != CodeIdentityExists {
		t.Fatalf("Restore over an existing key = %+v, want %s", res.Error, CodeIdentityExists)
	}
}

func TestKeys_RestoreRejectsJunkFile(t *testing.T) {
	f := newFixture(t)
	junk := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(junk, []byte("shopping list"), 0o600); err != nil {
		t.Fatal(err)
	}
	f.platform.openPath = junk

	res := f.keys.Restore("pass")
	if res.Error == nil || res.Error.Code != CodeNotAnIdentityFile {
		t.Fatalf("Restore(junk) = %+v, want %s", res.Error, CodeNotAnIdentityFile)
	}
}

// Touch is called constantly by the UI; it must never panic or error, including
// while locked.
func TestKeys_TouchIsAlwaysSafe(t *testing.T) {
	f := newFixture(t)
	f.keys.Touch() // no key at all

	if res := f.keys.Generate("pass"); res.Error != nil {
		t.Fatal(res.Error)
	}
	f.keys.Touch()
	f.keys.Lock()
	f.keys.Touch() // locked
}
