package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"filippo.io/age"
	"filippo.io/age/armor"

	"dawidroszman.eu/age-gui/internal/model"
)

// identityReadLimit bounds how much plaintext we accept out of the identity
// file. Real identity files are a few hundred bytes; this only stops a
// pathological file from being read into memory wholesale.
const identityReadLimit = 1 << 20 // 1 MiB

// KeyStatus describes the identity state for the UI.
type KeyStatus struct {
	// Exists reports whether a keypair has been created.
	Exists bool
	// Unlocked reports whether the private key is available this session.
	Unlocked bool
	// PublicKey is the user's recipient. Zero unless unlocked.
	PublicKey model.PublicKey
}

// KeyService owns the user's identity: creating it, unlocking it, and holding
// it in memory for the session.
//
// The private key exists in plaintext only inside this struct, only while
// unlocked, and never crosses the view boundary.
type KeyService struct {
	store IdentityStore

	// workFactor is the scrypt log2 work factor for the identity file. Zero
	// means age's default (currently 18), which is what the age CLI uses.
	workFactor int

	// now is the clock. Injectable so the idle logic is testable without
	// sleeping through real minutes.
	now func() time.Time

	// mu guards the fields below. Wails calls in on arbitrary goroutines, so
	// unlocking while an encrypt is reading the identity is a real race.
	mu sync.RWMutex
	// identities is nil when locked. It is a slice because an identity file
	// may hold more than one key, and all of them should be tried on decrypt.
	identities []age.Identity
	public     model.PublicKey

	// idleTimeout is the auto-lock period. Zero means auto-lock is off, which
	// is a setting the user is entitled to choose.
	idleTimeout time.Duration
	// lastActivity is when the user was last seen doing something.
	lastActivity time.Time
	// onAutoLock is notified after an idle lock, so the UI can react. Held here
	// rather than calling out to the view directly: this layer must not know a
	// GUI exists.
	onAutoLock func()
}

// keyOption configures a KeyService.
//
// The type is unexported, so callers outside this package can only ever get a
// KeyService with production defaults. That is deliberate: the sole option
// weakens key stretching, and it must be unreachable from real code.
type keyOption func(*KeyService)

// withWorkFactor lowers the scrypt work factor.
//
// Tests only. Real key stretching costs ~0.5s per call by design, which is
// correct for a user typing a passphrase once and ruinous for a suite that
// creates hundreds of identities. Production never calls this and gets age's
// default.
func withWorkFactor(logN int) keyOption {
	return func(s *KeyService) { s.workFactor = logN }
}

// withClock replaces the clock. Tests only: it lets the idle logic be driven
// through hours in microseconds.
func withClock(now func() time.Time) keyOption {
	return func(s *KeyService) { s.now = now }
}

// NewKeyService builds a KeyService over the given store.
func NewKeyService(store IdentityStore, opts ...keyOption) *KeyService {
	s := &KeyService{store: store, now: time.Now}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// SetAutoLockHandler registers a callback fired after an idle auto-lock.
// Call before StartAutoLock.
func (s *KeyService) SetAutoLockHandler(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onAutoLock = fn
}

// SetIdleTimeout configures the auto-lock period. Zero disables auto-lock.
//
// Changing it also counts as activity: a user who just chose "lock after 5
// minutes" means five minutes from now, not five minutes from whenever they
// last touched a file.
func (s *KeyService) SetIdleTimeout(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.idleTimeout = d
	s.lastActivity = s.now()
}

// Touch records user activity, resetting the idle countdown.
func (s *KeyService) Touch() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastActivity = s.now()
}

// IdleFor reports how long the user has been inactive.
func (s *KeyService) IdleFor() time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.lastActivity.IsZero() {
		return 0
	}
	return s.now().Sub(s.lastActivity)
}

// checkIdle locks the key if it has been idle past the timeout, reporting
// whether it did.
//
// Separated from the ticker loop so tests can drive it directly with a fake
// clock rather than sleeping.
func (s *KeyService) checkIdle() bool {
	s.mu.Lock()
	switch {
	case s.identities == nil: // already locked
		s.mu.Unlock()
		return false
	case s.idleTimeout <= 0: // auto-lock disabled by the user
		s.mu.Unlock()
		return false
	case s.now().Sub(s.lastActivity) < s.idleTimeout:
		s.mu.Unlock()
		return false
	}

	s.identities = nil
	s.public = model.PublicKey{}
	fn := s.onAutoLock
	s.mu.Unlock()

	// Called outside the lock: the handler emits a UI event and must not be
	// able to deadlock the service by calling back into it.
	if fn != nil {
		fn()
	}
	return true
}

// StartAutoLock runs the idle check until ctx is cancelled.
//
// The check is a poll rather than a reset-on-activity timer because Touch is
// called on every keystroke; rescheduling a timer that often would be far more
// machinery for the same outcome. interval only bounds how late a lock can be.
func (s *KeyService) StartAutoLock(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.checkIdle()
		}
	}
}

// Status reports the current identity state.
func (s *KeyService) Status() (KeyStatus, error) {
	exists, err := s.store.Exists()
	if err != nil {
		return KeyStatus{}, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	return KeyStatus{
		Exists:    exists,
		Unlocked:  s.identities != nil,
		PublicKey: s.public,
	}, nil
}

// Generate creates a new post-quantum hybrid keypair, encrypts it under
// passphrase, stores it, and leaves it unlocked.
//
// It refuses when an identity already exists: overwriting would destroy the
// only copy of the user's private key and with it every file ever encrypted to
// them. Replacing a key is a deliberate act that must go through an explicit
// delete, not a second click on "Generate".
func (s *KeyService) Generate(passphrase []byte) (model.PublicKey, error) {
	if len(passphrase) == 0 {
		return model.PublicKey{}, model.ErrEmptyPassphrase
	}

	exists, err := s.store.Exists()
	if err != nil {
		return model.PublicKey{}, err
	}
	if exists {
		return model.PublicKey{}, model.ErrIdentityExists
	}

	// Hybrid: X25519 + ML-KEM-768. This is what age itself now calls the
	// standard key type, and it is what protects against a "harvest now,
	// decrypt later" adversary.
	id, err := age.GenerateHybridIdentity()
	if err != nil {
		return model.PublicKey{}, fmt.Errorf("generate keypair: %w", err)
	}

	pub, err := model.ParsePublicKey(id.Recipient().String())
	if err != nil {
		return model.PublicKey{}, fmt.Errorf("generated key is unparseable: %w", err)
	}

	ciphertext, err := encryptIdentity(id.String(), passphrase, s.workFactor)
	if err != nil {
		return model.PublicKey{}, err
	}
	if err := s.store.Save(ciphertext); err != nil {
		return model.PublicKey{}, fmt.Errorf("save identity: %w", err)
	}

	s.mu.Lock()
	s.identities = []age.Identity{id}
	s.public = pub
	// Start the idle countdown from now; otherwise a zero lastActivity would
	// read as "idle since the epoch" and lock the key immediately.
	s.lastActivity = s.now()
	s.mu.Unlock()

	return pub, nil
}

// Unlock decrypts the stored identity with passphrase and holds it for the
// session.
func (s *KeyService) Unlock(passphrase []byte) error {
	if len(passphrase) == 0 {
		return model.ErrEmptyPassphrase
	}

	exists, err := s.store.Exists()
	if err != nil {
		return err
	}
	if !exists {
		return model.ErrNoIdentity
	}

	ciphertext, err := s.store.Load()
	if err != nil {
		return fmt.Errorf("load identity: %w", err)
	}

	ids, err := decryptIdentity(ciphertext, passphrase)
	if err != nil {
		return err
	}

	pub, err := recipientOf(ids[0])
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.identities = ids
	s.public = pub
	s.lastActivity = s.now()
	s.mu.Unlock()

	return nil
}

// Lock discards the in-memory private key.
func (s *KeyService) Lock() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.identities = nil
	s.public = model.PublicKey{}
}

// PublicKey returns the user's recipient, or ErrLocked.
func (s *KeyService) PublicKey() (model.PublicKey, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.identities == nil {
		return model.PublicKey{}, model.ErrLocked
	}
	return s.public, nil
}

// Identities returns the unlocked identities for decryption, or ErrLocked.
//
// This is the one place private keys leave the service, and it is consumed only
// by CryptoService inside this package's layer — never by the view.
func (s *KeyService) Identities() ([]age.Identity, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.identities == nil {
		return nil, model.ErrLocked
	}
	return s.identities, nil
}

// ExportPublicKey writes the user's recipient to path, in the one-per-line
// format that age's own -R flag reads.
func (s *KeyService) ExportPublicKey(path string, mode Overwrite) error {
	pub, err := s.PublicKey()
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil && mode != Replace {
		return model.ErrTargetExists
	}
	// Public keys are not secret, but 0644 is still narrower than most umasks
	// would give and costs nothing.
	return os.WriteFile(path, []byte(pub.String()+"\n"), 0o644)
}

// ExportIdentity writes a backup of the user's key to path.
//
// The bytes are copied verbatim: still the armored, scrypt-encrypted age file,
// so the backup is exactly as safe as the original and can go on a USB stick or
// into cloud storage without further thought. Anyone who finds it still needs
// the passphrase.
//
// Note this does not require the key to be unlocked. Backing up ciphertext
// needs no plaintext, and demanding a passphrase to copy a file the user could
// copy in their file manager would be security theatre.
func (s *KeyService) ExportIdentity(path string, mode Overwrite) error {
	exists, err := s.store.Exists()
	if err != nil {
		return err
	}
	if !exists {
		return model.ErrNoIdentity
	}

	blob, err := s.store.Load()
	if err != nil {
		return fmt.Errorf("read identity: %w", err)
	}
	if _, err := os.Stat(path); err == nil && mode != Replace {
		return model.ErrTargetExists
	}
	if err := os.WriteFile(path, blob, 0o600); err != nil {
		return fmt.Errorf("write backup: %w", err)
	}
	return nil
}

// RestoreIdentity adopts a key from a file and unlocks it.
//
// It accepts either shape a user might plausibly have:
//
//   - a backup this app produced (encrypted): passphrase decrypts it, and the
//     key is stored unchanged.
//   - a plaintext identity file, as `age-keygen -o key.txt` writes: there is
//     nothing to decrypt, so passphrase becomes the new protection and the key
//     is encrypted with it before being stored.
//
// One passphrase field covers both because from the user's side the question is
// the same — "the passphrase for this key" — and making them care which kind of
// file they have would be our problem leaking into their day.
//
// It refuses when a key already exists: restoring over one would destroy it and
// orphan every file encrypted to it. That is the same reasoning as Generate.
func (s *KeyService) RestoreIdentity(path string, passphrase []byte) (model.PublicKey, error) {
	if len(passphrase) == 0 {
		return model.PublicKey{}, model.ErrEmptyPassphrase
	}

	exists, err := s.store.Exists()
	if err != nil {
		return model.PublicKey{}, err
	}
	if exists {
		return model.PublicKey{}, model.ErrIdentityExists
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return model.PublicKey{}, fmt.Errorf("read %s: %w", filepath.Base(path), err)
	}

	ids, blob, err := adoptIdentity(raw, passphrase, s.workFactor)
	if err != nil {
		return model.PublicKey{}, err
	}

	pub, err := recipientOf(ids[0])
	if err != nil {
		return model.PublicKey{}, err
	}

	if err := s.store.Save(blob); err != nil {
		return model.PublicKey{}, fmt.Errorf("save identity: %w", err)
	}

	s.mu.Lock()
	s.identities = ids
	s.public = pub
	s.lastActivity = s.now()
	s.mu.Unlock()

	return pub, nil
}

// adoptIdentity works out what kind of identity file raw is and returns the
// parsed identities plus the ciphertext to store.
func adoptIdentity(raw, passphrase []byte, workFactor int) ([]age.Identity, []byte, error) {
	// An age file starts with the armor header (our backups) or the binary
	// format's version line. Anything else is a plaintext identity file.
	if isAgeFile(raw) {
		ids, err := decryptIdentity(raw, passphrase)
		if err != nil {
			return nil, nil, err
		}
		// Already encrypted under this passphrase: store it byte for byte
		// rather than re-encrypting, so a restore is a faithful restore.
		return ids, raw, nil
	}

	// Plaintext: parse it, then protect it with the passphrase just supplied.
	ids, err := age.ParseIdentities(bytes.NewReader(raw))
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", model.ErrNotAnIdentityFile, err)
	}
	if len(ids) == 0 {
		return nil, nil, model.ErrNotAnIdentityFile
	}

	// Re-encode from the parsed identities rather than storing the file as-is:
	// age-keygen files carry comment lines, and this keeps what we store to
	// exactly the keys we understood.
	var sb strings.Builder
	for _, id := range ids {
		str, ok := id.(fmt.Stringer)
		if !ok {
			return nil, nil, fmt.Errorf("%w: unsupported key type %T", model.ErrNotAnIdentityFile, id)
		}
		sb.WriteString(str.String())
		sb.WriteString("\n")
	}

	blob, err := encryptIdentity(strings.TrimSuffix(sb.String(), "\n"), passphrase, workFactor)
	if err != nil {
		return nil, nil, err
	}
	return ids, blob, nil
}

// isAgeFile reports whether raw looks like an age file rather than a plaintext
// identity file.
func isAgeFile(raw []byte) bool {
	return bytes.HasPrefix(raw, []byte(armor.Header)) ||
		bytes.HasPrefix(raw, []byte(ageFileHeader))
}

// ageFileHeader is the first line of a binary age file, per the format spec.
const ageFileHeader = "age-encryption.org/v1"

// encryptIdentity wraps an identity encoding in an armored, scrypt-encrypted
// age file.
//
// The result is a plain age file, so `age -d identity.age` opens it with the
// stock CLI. That is deliberate: the user's key must never be trapped inside
// this application.
func encryptIdentity(identity string, passphrase []byte, workFactor int) ([]byte, error) {
	// age's API takes the passphrase as a string, which copies it into
	// immutable memory we cannot wipe. Unavoidable without forking age; the
	// window is the process lifetime and the caller still wipes its own copy.
	r, err := age.NewScryptRecipient(string(passphrase))
	if err != nil {
		return nil, fmt.Errorf("prepare passphrase: %w", err)
	}
	if workFactor > 0 {
		r.SetWorkFactor(workFactor)
	}

	var buf bytes.Buffer
	armorW := armor.NewWriter(&buf)
	ageW, err := age.Encrypt(armorW, r)
	if err != nil {
		return nil, fmt.Errorf("encrypt identity: %w", err)
	}
	if _, err := io.WriteString(ageW, identity+"\n"); err != nil {
		return nil, fmt.Errorf("encrypt identity: %w", err)
	}
	// Both writers must close, innermost first: age's Close writes the final
	// STREAM chunk, armor's writes the footer. Skipping either yields a file
	// that looks fine and cannot be decrypted.
	if err := ageW.Close(); err != nil {
		return nil, fmt.Errorf("finalise identity: %w", err)
	}
	if err := armorW.Close(); err != nil {
		return nil, fmt.Errorf("finalise identity: %w", err)
	}
	return buf.Bytes(), nil
}

// decryptIdentity reverses encryptIdentity, translating age's errors into
// domain errors the UI can act on.
func decryptIdentity(ciphertext, passphrase []byte) ([]age.Identity, error) {
	scrypt, err := age.NewScryptIdentity(string(passphrase))
	if err != nil {
		return nil, fmt.Errorf("prepare passphrase: %w", err)
	}

	r, err := age.Decrypt(armor.NewReader(bytes.NewReader(ciphertext)), scrypt)
	if err != nil {
		// A wrong passphrase surfaces as ErrIncorrectIdentity (wrapped inside
		// NoIdentityMatchError). Anything else means the file itself is
		// damaged, which is a very different message for the user.
		if errors.Is(err, age.ErrIncorrectIdentity) {
			return nil, model.ErrWrongPassphrase
		}
		return nil, fmt.Errorf("%w: %v", model.ErrCorruptIdentity, err)
	}

	plaintext, err := io.ReadAll(io.LimitReader(r, identityReadLimit))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", model.ErrCorruptIdentity, err)
	}

	ids, err := age.ParseIdentities(bytes.NewReader(plaintext))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", model.ErrCorruptIdentity, err)
	}
	if len(ids) == 0 {
		return nil, model.ErrCorruptIdentity
	}
	return ids, nil
}

// recipientOf derives the public key from an identity.
//
// age.Identity is an interface with only Unwrap, so recovering the recipient
// needs a type switch. Both concrete types are handled: we generate hybrid
// keys, but a user may import a classic identity created by age-keygen.
func recipientOf(id age.Identity) (model.PublicKey, error) {
	switch v := id.(type) {
	case *age.HybridIdentity:
		return model.ParsePublicKey(v.Recipient().String())
	case *age.X25519Identity:
		return model.ParsePublicKey(v.Recipient().String())
	default:
		return model.PublicKey{}, fmt.Errorf("%w: unsupported key type %T", model.ErrCorruptIdentity, id)
	}
}
