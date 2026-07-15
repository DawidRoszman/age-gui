package service

import (
	"errors"
	"testing"
	"time"

	"dawidroszman.eu/age-gui/internal/model"
)

// fakeSettingsStore is an in-memory SettingsStore.
type fakeSettingsStore struct {
	settings model.Settings
	saved    bool
	failSave error
}

func (f *fakeSettingsStore) Load() (model.Settings, error) {
	if !f.saved {
		return model.DefaultSettings(), nil
	}
	return f.settings, nil
}

func (f *fakeSettingsStore) Save(s model.Settings) error {
	if f.failSave != nil {
		return f.failSave
	}
	f.settings = s
	f.saved = true
	return nil
}

var _ SettingsStore = (*fakeSettingsStore)(nil)

func newSettingsFixture(t *testing.T) (*SettingsService, *KeyService, *fakeSettingsStore, *fakeClock) {
	t.Helper()
	clock := newFakeClock()
	keys := NewKeyService(&fakeIdentityStore{}, withWorkFactor(testWorkFactor), withClock(clock.Now))
	store := &fakeSettingsStore{}
	svc, err := NewSettingsService(store, keys)
	if err != nil {
		t.Fatal(err)
	}
	return svc, keys, store, clock
}

func TestSettings_DefaultsHaveAutoLockOn(t *testing.T) {
	svc, _, _, _ := newSettingsFixture(t)

	got := svc.Get()
	if !got.AutoLockEnabled() {
		t.Error("auto-lock is off by default; the key would sit in memory indefinitely")
	}
	if got.AutoLockMinutes != model.DefaultAutoLockMinutes {
		t.Errorf("AutoLockMinutes = %d, want %d", got.AutoLockMinutes, model.DefaultAutoLockMinutes)
	}
}

// Stored settings must take effect at startup, not only once the user happens
// to open the settings screen.
func TestSettings_StoredValueAppliedAtStartup(t *testing.T) {
	clock := newFakeClock()
	keys := NewKeyService(&fakeIdentityStore{}, withWorkFactor(testWorkFactor), withClock(clock.Now))
	store := &fakeSettingsStore{settings: model.Settings{AutoLockMinutes: 3}, saved: true}

	if _, err := NewSettingsService(store, keys); err != nil {
		t.Fatal(err)
	}
	if _, err := keys.Generate([]byte("pass")); err != nil {
		t.Fatal(err)
	}

	clock.Advance(4 * time.Minute)
	if !keys.checkIdle() {
		t.Error("the stored 3 minute timeout was not applied at startup")
	}
}

// Same, for the user who turned it off: their choice must survive a restart.
func TestSettings_StoredDisabledAppliedAtStartup(t *testing.T) {
	clock := newFakeClock()
	keys := NewKeyService(&fakeIdentityStore{}, withWorkFactor(testWorkFactor), withClock(clock.Now))
	store := &fakeSettingsStore{
		settings: model.Settings{AutoLockMinutes: model.AutoLockDisabled},
		saved:    true,
	}

	if _, err := NewSettingsService(store, keys); err != nil {
		t.Fatal(err)
	}
	if _, err := keys.Generate([]byte("pass")); err != nil {
		t.Fatal(err)
	}

	clock.Advance(7 * 24 * time.Hour)
	if keys.checkIdle() {
		t.Error("auto-locked despite the stored setting being disabled")
	}
}

func TestSettings_UpdateAppliesImmediately(t *testing.T) {
	svc, keys, _, clock := newSettingsFixture(t)
	if _, err := keys.Generate([]byte("pass")); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.Update(model.Settings{AutoLockMinutes: 2}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	clock.Advance(90 * time.Second)
	if keys.checkIdle() {
		t.Error("locked before the new 2 minute timeout elapsed")
	}
	clock.Advance(60 * time.Second)
	if !keys.checkIdle() {
		t.Error("the new 2 minute timeout was not applied")
	}
}

func TestSettings_DisableStopsAutoLock(t *testing.T) {
	svc, keys, _, clock := newSettingsFixture(t)
	if _, err := keys.Generate([]byte("pass")); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.Update(model.Settings{AutoLockMinutes: model.AutoLockDisabled}); err != nil {
		t.Fatalf("Update(disabled): %v", err)
	}

	clock.Advance(30 * 24 * time.Hour)
	if keys.checkIdle() {
		t.Fatal("auto-locked after the user turned auto-lock off")
	}
}

func TestSettings_UpdatePersists(t *testing.T) {
	svc, _, store, _ := newSettingsFixture(t)

	if _, err := svc.Update(model.Settings{AutoLockMinutes: 45}); err != nil {
		t.Fatal(err)
	}
	if store.settings.AutoLockMinutes != 45 {
		t.Errorf("stored AutoLockMinutes = %d, want 45", store.settings.AutoLockMinutes)
	}
}

func TestSettings_UpdateRejectsOutOfRange(t *testing.T) {
	svc, _, store, _ := newSettingsFixture(t)

	for name, mins := range map[string]int{
		"negative":  -5,
		"too large": model.MaxAutoLockMinutes + 1,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := svc.Update(model.Settings{AutoLockMinutes: mins})
			if !errors.Is(err, model.ErrInvalidSettings) {
				t.Fatalf("Update(%d) = %v, want ErrInvalidSettings", mins, err)
			}
			if store.saved {
				t.Error("an invalid value reached the disk")
			}
		})
	}
}

// A rejected update must leave the live settings alone.
func TestSettings_RejectedUpdateKeepsCurrent(t *testing.T) {
	svc, _, _, _ := newSettingsFixture(t)

	if _, err := svc.Update(model.Settings{AutoLockMinutes: 20}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Update(model.Settings{AutoLockMinutes: -1}); err == nil {
		t.Fatal("want error")
	}
	if got := svc.Get().AutoLockMinutes; got != 20 {
		t.Errorf("AutoLockMinutes = %d after a rejected update, want the previous 20", got)
	}
}

// If the write fails the running app must not diverge from the file it will
// read next launch.
func TestSettings_FailedSaveDoesNotApply(t *testing.T) {
	svc, keys, store, clock := newSettingsFixture(t)
	if _, err := keys.Generate([]byte("pass")); err != nil {
		t.Fatal(err)
	}
	store.failSave = errors.New("disk full")

	if _, err := svc.Update(model.Settings{AutoLockMinutes: model.AutoLockDisabled}); err == nil {
		t.Fatal("Update = nil despite the store failing")
	}
	if !svc.Get().AutoLockEnabled() {
		t.Error("a failed save still changed the live settings")
	}
	// The default 15 minutes must still be in force.
	clock.Advance(16 * time.Minute)
	if !keys.checkIdle() {
		t.Error("a failed save silently disabled auto-lock")
	}
}

func TestSettings_Validate(t *testing.T) {
	for name, tc := range map[string]struct {
		mins int
		ok   bool
	}{
		"disabled":  {model.AutoLockDisabled, true},
		"minimum":   {model.MinAutoLockMinutes, true},
		"typical":   {15, true},
		"maximum":   {model.MaxAutoLockMinutes, true},
		"negative":  {-1, false},
		"too large": {model.MaxAutoLockMinutes + 1, false},
	} {
		t.Run(name, func(t *testing.T) {
			err := model.Settings{AutoLockMinutes: tc.mins}.Validate()
			if tc.ok && err != nil {
				t.Errorf("Validate(%d) = %v, want nil", tc.mins, err)
			}
			if !tc.ok && err == nil {
				t.Errorf("Validate(%d) = nil, want error", tc.mins)
			}
		})
	}
}

func TestSettings_IdleTimeout(t *testing.T) {
	if got := (model.Settings{AutoLockMinutes: 5}).IdleTimeout(); got != 5*time.Minute {
		t.Errorf("IdleTimeout() = %v, want 5m", got)
	}
	// Zero is what KeyService reads as "never".
	if got := (model.Settings{AutoLockMinutes: model.AutoLockDisabled}).IdleTimeout(); got != 0 {
		t.Errorf("IdleTimeout() when disabled = %v, want 0", got)
	}
}
