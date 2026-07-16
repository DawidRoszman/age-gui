package service

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"dawidroszman.eu/encryptor/internal/model"
)

// fakeClock drives the idle logic through hours without sleeping.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{t: time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// newLockable returns an unlocked KeyService driven by a fake clock.
func newLockable(t *testing.T) (*KeyService, *fakeClock) {
	t.Helper()
	clock := newFakeClock()
	svc := NewKeyService(&fakeIdentityStore{}, withWorkFactor(testWorkFactor), withClock(clock.Now))
	if _, err := svc.Generate([]byte("pass")); err != nil {
		t.Fatal(err)
	}
	return svc, clock
}

func TestAutoLock_LocksAfterIdle(t *testing.T) {
	svc, clock := newLockable(t)
	svc.SetIdleTimeout(15 * time.Minute)

	clock.Advance(14 * time.Minute)
	if svc.checkIdle() {
		t.Fatal("locked at 14 minutes with a 15 minute timeout")
	}

	clock.Advance(2 * time.Minute) // now 16 minutes idle
	if !svc.checkIdle() {
		t.Fatal("did not lock at 16 minutes with a 15 minute timeout")
	}
	if _, err := svc.Identities(); !errors.Is(err, model.ErrLocked) {
		t.Errorf("Identities() after auto-lock = %v, want ErrLocked", err)
	}
}

// The user's explicit "off" must be honoured no matter how long they idle.
func TestAutoLock_DisabledNeverLocks(t *testing.T) {
	svc, clock := newLockable(t)
	svc.SetIdleTimeout(0) // model.AutoLockDisabled

	clock.Advance(30 * 24 * time.Hour)
	if svc.checkIdle() {
		t.Fatal("auto-locked despite the user disabling auto-lock")
	}
	if _, err := svc.PublicKey(); err != nil {
		t.Errorf("PublicKey() = %v, want it still unlocked", err)
	}
}

func TestAutoLock_TouchResetsCountdown(t *testing.T) {
	svc, clock := newLockable(t)
	svc.SetIdleTimeout(10 * time.Minute)

	// Work for an hour, touching every 9 minutes: never idle long enough.
	for range 6 {
		clock.Advance(9 * time.Minute)
		svc.Touch()
		if svc.checkIdle() {
			t.Fatal("locked while the user was actively working")
		}
	}

	// Then walk away.
	clock.Advance(11 * time.Minute)
	if !svc.checkIdle() {
		t.Fatal("did not lock after the user stopped")
	}
}

// Changing the setting restarts the countdown: choosing "5 minutes" means five
// minutes from now, not five minutes from whenever they last did something.
func TestAutoLock_SettingTimeoutCountsAsActivity(t *testing.T) {
	svc, clock := newLockable(t)

	clock.Advance(20 * time.Minute) // idle for a while first
	svc.SetIdleTimeout(10 * time.Minute)

	if svc.checkIdle() {
		t.Fatal("locked immediately after the timeout was set; the countdown did not restart")
	}
	clock.Advance(11 * time.Minute)
	if !svc.checkIdle() {
		t.Fatal("did not lock 11 minutes after the timeout was set")
	}
}

func TestAutoLock_HandlerFiresOnce(t *testing.T) {
	svc, clock := newLockable(t)

	var calls atomic.Int32
	svc.SetAutoLockHandler(func() { calls.Add(1) })
	svc.SetIdleTimeout(5 * time.Minute)

	clock.Advance(6 * time.Minute)
	if !svc.checkIdle() {
		t.Fatal("did not lock")
	}
	// Still idle, but already locked: the UI must not be told twice.
	clock.Advance(60 * time.Minute)
	if svc.checkIdle() {
		t.Fatal("checkIdle reported a second lock on an already-locked key")
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("auto-lock handler fired %d times, want 1", got)
	}
}

// A handler that reaches back into the service must not deadlock it. Calling
// the callback while holding the mutex would hang here.
func TestAutoLock_HandlerMayCallBackIn(t *testing.T) {
	svc, clock := newLockable(t)

	done := make(chan struct{})
	svc.SetAutoLockHandler(func() {
		// Any of these would deadlock if the callback ran under the lock.
		_, _ = svc.Status()
		_, _ = svc.PublicKey()
		svc.Touch()
		close(done)
	})
	svc.SetIdleTimeout(time.Minute)
	clock.Advance(2 * time.Minute)

	go svc.checkIdle()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("auto-lock handler deadlocked calling back into KeyService")
	}
}

func TestAutoLock_NotLockedWhenAlreadyLocked(t *testing.T) {
	svc, clock := newLockable(t)
	svc.SetIdleTimeout(time.Minute)
	svc.Lock()

	clock.Advance(time.Hour)
	if svc.checkIdle() {
		t.Error("checkIdle reported locking an already-locked key")
	}
}

// A freshly generated key must not be treated as idle since the zero time.
func TestAutoLock_FreshKeyIsNotImmediatelyIdle(t *testing.T) {
	svc, _ := newLockable(t)
	svc.SetIdleTimeout(15 * time.Minute)

	if svc.checkIdle() {
		t.Fatal("locked a key the instant it was created")
	}
	if got := svc.IdleFor(); got != 0 {
		t.Errorf("IdleFor() = %v on a fresh key, want 0", got)
	}
}

func TestAutoLock_IdleFor(t *testing.T) {
	svc, clock := newLockable(t)

	clock.Advance(3 * time.Minute)
	if got := svc.IdleFor(); got != 3*time.Minute {
		t.Errorf("IdleFor() = %v, want 3m", got)
	}
	svc.Touch()
	if got := svc.IdleFor(); got != 0 {
		t.Errorf("IdleFor() after Touch = %v, want 0", got)
	}
}

// StartAutoLock must stop when its context is cancelled, or the goroutine would
// outlive the app.
func TestAutoLock_StartStopsOnContextCancel(t *testing.T) {
	svc, _ := newLockable(t)
	svc.SetIdleTimeout(time.Hour)

	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() {
		svc.StartAutoLock(ctx, time.Millisecond)
		close(stopped)
	}()

	cancel()
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("StartAutoLock did not return after its context was cancelled")
	}
}

// End-to-end through the real ticker, with a real (short) timeout, to prove the
// loop actually wires checkIdle up rather than only the unit-tested logic.
func TestAutoLock_LoopReallyLocks(t *testing.T) {
	clock := newFakeClock()
	svc := NewKeyService(&fakeIdentityStore{}, withWorkFactor(testWorkFactor), withClock(clock.Now))
	if _, err := svc.Generate([]byte("pass")); err != nil {
		t.Fatal(err)
	}

	locked := make(chan struct{})
	svc.SetAutoLockHandler(func() { close(locked) })
	svc.SetIdleTimeout(time.Minute)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go svc.StartAutoLock(ctx, time.Millisecond)

	clock.Advance(2 * time.Minute)

	select {
	case <-locked:
	case <-time.After(5 * time.Second):
		t.Fatal("the auto-lock loop never locked an idle key")
	}
}
