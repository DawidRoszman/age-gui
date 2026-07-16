package service

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"filippo.io/age"
	"filippo.io/age/armor"

	"dawidroszman.eu/encryptor/internal/model"
)

// Extension appended to encrypted files, matching the age CLI's convention.
const Extension = ".age"

// outputPerm applies to everything we write. Decrypted output is by definition
// the plaintext of a secret, so it must not be group- or world-readable;
// ciphertext gets the same treatment because it costs nothing.
const outputPerm fs.FileMode = 0o600

// outputDirPerm applies to an output folder we have to create ourselves. Owner
// only, for the same reason as outputPerm: we are creating a folder that is
// about to hold a secret. An existing folder keeps its own permissions --
// MkdirAll does not touch those, so choosing a shared folder stays the user's
// decision to make.
const outputDirPerm fs.FileMode = 0o700

// progressInterval throttles progress callbacks. Each one crosses into the
// webview, and repainting a progress bar per 32KiB chunk would cost more than
// the encryption.
const progressInterval = 100 * time.Millisecond

// Progress reports bytes processed out of total. total is -1 when unknown.
type Progress func(done, total int64)

// Overwrite says what to do when the output path is already taken.
//
// An enum rather than a bool because these appear at call sites where a bare
// `true` would be unreadable, and the reader of a crypto call deserves to see
// "Replace" spelled out.
type Overwrite int

const (
	// Refuse returns model.ErrTargetExists rather than touching the file.
	// The default, and what every automatic output path uses.
	Refuse Overwrite = iota
	// Replace overwrites. Only legitimate once the user has explicitly agreed
	// — for example by confirming in the OS save dialog, which asks before it
	// ever returns the path. It is still atomic: the existing file is replaced
	// by a rename, never truncated in place.
	Replace
)

// FileKind describes how an age file was encrypted.
type FileKind string

const (
	// FileKindPassphrase means the file needs a passphrase (an scrypt stanza).
	FileKindPassphrase FileKind = "passphrase"
	// FileKindRecipients means the file needs a private key.
	FileKindRecipients FileKind = "recipients"
)

// CryptoService encrypts and decrypts files.
//
// Every operation streams: a file is never held in memory, so multi-gigabyte
// inputs work and plaintext never accumulates on the heap.
type CryptoService struct {
	keys *KeyService

	// workFactor is the scrypt log2 work factor for passphrase-encrypted
	// files. Zero means age's default, matching the age CLI.
	workFactor int
}

// cryptoOption configures a CryptoService. Unexported for the same reason as
// keyOption: its only use weakens key stretching, so production must not reach it.
type cryptoOption func(*CryptoService)

// withCryptoWorkFactor lowers the scrypt work factor for passphrase encryption.
// Tests only; see withWorkFactor.
func withCryptoWorkFactor(logN int) cryptoOption {
	return func(s *CryptoService) { s.workFactor = logN }
}

// NewCryptoService builds a CryptoService. It needs KeyService to reach the
// unlocked identities for key-based decryption.
func NewCryptoService(keys *KeyService, opts ...cryptoOption) *CryptoService {
	s := &CryptoService{keys: keys}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// SelfRecipient returns the user's own public key, for encrypting a file so
// they can open it themselves.
//
// It fails with ErrLocked when the key is locked, because the public key is
// only held in memory while unlocked. In practice the encrypt screen is only
// reachable unlocked, but an idle auto-lock can race an in-flight encrypt, and
// this surfaces that as a normal "unlock to continue" rather than a crash.
func (s *CryptoService) SelfRecipient() (model.PublicKey, error) {
	return s.keys.PublicKey()
}

// EncryptedName returns the conventional output path for encrypting in.
func EncryptedName(in string) string { return in + Extension }

// DecryptedName returns the conventional output path for decrypting in.
//
// When the name does not end in .age we cannot know the original extension, so
// we append .decrypted rather than silently overwrite the input.
func DecryptedName(in string) string {
	if strings.HasSuffix(in, Extension) {
		return strings.TrimSuffix(in, Extension)
	}
	return in + ".decrypted"
}

// maxNameAttempts caps the search for a free name.
//
// A thousand files of the same name is not a real workflow; reaching this means
// something is wrong (a directory we cannot stat, say), and looping forever
// would hang the operation with no way for the user to tell why.
const maxNameAttempts = 1000

// OutputPath returns where to write the result of an operation on in, inside
// dir, choosing a name that is not already taken.
//
// name maps an input file name to its output file name -- EncryptedName or
// DecryptedName. Only the base name of in is used: output goes to dir, not
// beside the input.
//
// Collisions are resolved by numbering, "report.pdf (2).age", the way a browser
// does. Now that every output lands in one shared folder, two inputs with the
// same name from different folders are ordinary rather than exceptional, and
// stopping to ask about each one would be noise. Numbering keeps the promise
// that matters -- nothing already on disk is replaced.
//
// The returned path is free at the moment it is returned and nothing reserves
// it, so a caller racing another writer can still find it taken. Callers pass
// Refuse, which turns that race into an error rather than a lost file.
func OutputPath(dir, in string, name func(string) string) (string, error) {
	if dir == "" {
		return "", fmt.Errorf("no folder given for output of %q", filepath.Base(in))
	}
	// The user picked this folder, or it is their downloads folder; either may
	// have been removed or never created. 0700 matches the rest of what we
	// write: output is ciphertext or plaintext of a secret, and neither wants
	// a group-readable parent we created ourselves.
	if err := os.MkdirAll(dir, outputDirPerm); err != nil {
		return "", fmt.Errorf("create output folder %s: %w", dir, err)
	}

	base := name(filepath.Base(in))
	candidate := filepath.Join(dir, base)

	stem, ext := splitName(base)
	for n := 2; ; n++ {
		_, err := os.Lstat(candidate)
		if errors.Is(err, fs.ErrNotExist) {
			return candidate, nil
		}
		if err != nil {
			return "", fmt.Errorf("check output path %s: %w", candidate, err)
		}
		if n > maxNameAttempts {
			return "", fmt.Errorf("no free name for %s in %s after %d tries", base, dir, maxNameAttempts)
		}
		candidate = filepath.Join(dir, fmt.Sprintf("%s (%d)%s", stem, n, ext))
	}
}

// splitName divides a file name into the part to number and the extension to
// keep on the end, so "report.pdf.age" numbers as "report.pdf (2).age" and
// stays openable by extension.
func splitName(base string) (stem, ext string) {
	ext = filepath.Ext(base)
	stem = strings.TrimSuffix(base, ext)
	// A dotfile like ".bashrc" is all extension by Ext's reckoning, which would
	// number it as " (2).bashrc" and lose the name.
	if stem == "" {
		return base, ""
	}
	return stem, ext
}

// EncryptFile encrypts in to out for the given recipients.
func (s *CryptoService) EncryptFile(ctx context.Context, in, out string, recipients []model.PublicKey, mode Overwrite, onProgress Progress) error {
	if len(recipients) == 0 {
		return model.ErrNoRecipients
	}
	rs := make([]age.Recipient, 0, len(recipients))
	for _, r := range recipients {
		rs = append(rs, r.Recipient())
	}
	return s.encrypt(ctx, in, out, rs, mode, onProgress)
}

// EncryptFilePassphrase encrypts in to out under a passphrase.
//
// Prefer EncryptFile: a passphrase must be transmitted to the recipient over
// some other channel, which is exactly the problem public keys solve.
func (s *CryptoService) EncryptFilePassphrase(ctx context.Context, in, out string, passphrase []byte, mode Overwrite, onProgress Progress) error {
	if len(passphrase) == 0 {
		return model.ErrEmptyPassphrase
	}
	r, err := age.NewScryptRecipient(string(passphrase))
	if err != nil {
		return fmt.Errorf("prepare passphrase: %w", err)
	}
	if s.workFactor > 0 {
		r.SetWorkFactor(s.workFactor)
	}
	return s.encrypt(ctx, in, out, []age.Recipient{r}, mode, onProgress)
}

// DecryptFile decrypts in to out using the unlocked identity.
func (s *CryptoService) DecryptFile(ctx context.Context, in, out string, mode Overwrite, onProgress Progress) error {
	ids, err := s.keys.Identities()
	if err != nil {
		return err // ErrLocked
	}
	return s.decrypt(ctx, in, out, ids, modeIdentity, mode, onProgress)
}

// DecryptFilePassphrase decrypts a passphrase-encrypted file.
//
// This needs no identity, so it works while the app is locked.
func (s *CryptoService) DecryptFilePassphrase(ctx context.Context, in, out string, passphrase []byte, mode Overwrite, onProgress Progress) error {
	if len(passphrase) == 0 {
		return model.ErrEmptyPassphrase
	}
	id, err := age.NewScryptIdentity(string(passphrase))
	if err != nil {
		return fmt.Errorf("prepare passphrase: %w", err)
	}
	return s.decrypt(ctx, in, out, []age.Identity{id}, modePassphrase, mode, onProgress)
}

// Inspect reports whether a file needs a passphrase or a private key.
//
// The UI calls this before prompting, so it can ask for the right thing. It
// matters that this works while locked: a passphrase-encrypted file needs no
// identity, so demanding an unlock first would be nonsense.
func (s *CryptoService) Inspect(path string) (FileKind, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", filepath.Base(path), err)
	}
	defer f.Close()

	src, err := ageSource(f)
	if err != nil {
		return "", err
	}

	// A probe identity that declines everything forces age to report the
	// stanza types it found, using only the public API and without doing any
	// scrypt work. Passing a real ScryptIdentity here would burn ~1s deriving
	// a key just to answer a question about the header.
	_, err = age.Decrypt(src, probeIdentity{})

	var noMatch *age.NoIdentityMatchError
	if errors.As(err, &noMatch) {
		if slices.Contains(noMatch.StanzaTypes, "scrypt") {
			return FileKindPassphrase, nil
		}
		return FileKindRecipients, nil
	}
	if err != nil {
		return "", fmt.Errorf("%s does not look like an age file: %w", filepath.Base(path), err)
	}
	// Unreachable: probeIdentity never returns a file key.
	return FileKindRecipients, nil
}

// probeIdentity is an age.Identity that never unwraps anything. age.Identity
// has a single method, so implementing it costs nothing and lets Inspect read
// the header through supported API instead of parsing the wire format.
type probeIdentity struct{}

func (probeIdentity) Unwrap([]*age.Stanza) ([]byte, error) {
	return nil, age.ErrIncorrectIdentity
}

func (s *CryptoService) encrypt(ctx context.Context, in, out string, recipients []age.Recipient, mode Overwrite, onProgress Progress) error {
	src, size, err := openInput(in)
	if err != nil {
		return err
	}
	defer src.Close()

	return writeStream(out, mode, func(w io.Writer) error {
		ageW, err := age.Encrypt(w, recipients...)
		if err != nil {
			return fmt.Errorf("start encryption: %w", err)
		}
		if _, err := io.Copy(ageW, newProgressReader(ctx, src, size, onProgress)); err != nil {
			return err
		}
		// Close flushes the final STREAM chunk. Without it the file is
		// silently truncated and undecryptable.
		if err := ageW.Close(); err != nil {
			return fmt.Errorf("finalise encryption: %w", err)
		}
		return nil
	})
}

func (s *CryptoService) decrypt(ctx context.Context, in, out string, ids []age.Identity, dm decryptMode, mode Overwrite, onProgress Progress) error {
	src, size, err := openInput(in)
	if err != nil {
		return err
	}
	defer src.Close()

	armored, err := ageSource(src)
	if err != nil {
		return err
	}

	r, err := age.Decrypt(armored, ids...)
	if err != nil {
		return classifyDecryptError(err, dm)
	}

	return writeStream(out, mode, func(w io.Writer) error {
		// Progress tracks the ciphertext read, which is the only size we know
		// up front; it is within a few hundred bytes of the plaintext size.
		_, err := io.Copy(w, newProgressReader(ctx, r, size, onProgress))
		return err
	})
}

// decryptMode records what the caller tried to open a file with. The same age
// error means different things depending on it, so classifying without it
// produces confidently wrong advice.
type decryptMode int

const (
	modeIdentity decryptMode = iota
	modePassphrase
)

// classifyDecryptError turns age's errors into domain errors.
//
// The NoIdentityMatchError check must come first: its Unwrap exposes the
// underlying ErrIncorrectIdentity values, so errors.Is(err, ErrIncorrectIdentity)
// is also true for it, and testing that first would collapse distinct
// situations into one wrong message.
//
// The presence of an scrypt stanza is not self-explanatory either. age forbids
// mixing scrypt with other recipients, so the stanza reliably says the file is
// passphrase-encrypted — but whether that is news to the user depends entirely
// on what they just tried:
//
//	tried a key,        found scrypt     -> it needs a passphrase, go ask for one
//	tried a passphrase, found scrypt     -> right kind of secret, wrong value
//	tried a key,        found recipients -> encrypted to someone else
//	tried a passphrase, found recipients -> it needs their key, not a passphrase
func classifyDecryptError(err error, mode decryptMode) error {
	var noMatch *age.NoIdentityMatchError
	if errors.As(err, &noMatch) {
		isScrypt := slices.Contains(noMatch.StanzaTypes, "scrypt")
		switch {
		case mode == modePassphrase && isScrypt:
			return model.ErrWrongPassphrase
		case mode == modePassphrase && !isScrypt:
			return model.ErrKeyRequired
		case isScrypt:
			return model.ErrPassphraseRequired
		default:
			return model.ErrNotForYou
		}
	}
	if errors.Is(err, age.ErrIncorrectIdentity) {
		// Defensive: age.Decrypt wraps rejections in NoIdentityMatchError, so
		// this is only reachable if that ever changes.
		if mode == modePassphrase {
			return model.ErrWrongPassphrase
		}
		return model.ErrNotForYou
	}
	return fmt.Errorf("decrypt: %w", err)
}

// openInput opens path and reports its size for progress purposes.
func openInput(path string) (*os.File, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, fmt.Errorf("open %s: %w", filepath.Base(path), err)
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, 0, fmt.Errorf("stat %s: %w", filepath.Base(path), err)
	}
	if info.IsDir() {
		f.Close()
		return nil, 0, fmt.Errorf("%s is a folder, not a file", filepath.Base(path))
	}
	return f, info.Size(), nil
}

// ageSource wraps r in an armor reader when the input is ASCII-armored.
//
// age files come in both binary and armored form and the CLI sniffs which is
// which; a GUI that only accepted one would reject files users legitimately
// received.
func ageSource(r io.Reader) (io.Reader, error) {
	br := bufio.NewReader(r)
	head, err := br.Peek(len(armor.Header))
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("read file: %w", err)
	}
	if bytes.HasPrefix(head, []byte(armor.Header)) {
		return armor.NewReader(br), nil
	}
	return br, nil
}

// writeStream runs write against a temporary file and renames it into place
// only on success.
//
// A cancelled or failed decryption must not leave a partial plaintext file
// sitting on disk looking like a real one, and a failed encryption must not
// leave a truncated archive the user might mistake for a backup.
func writeStream(out string, mode Overwrite, write func(io.Writer) error) (err error) {
	if _, statErr := os.Stat(out); statErr == nil {
		if mode != Replace {
			return model.ErrTargetExists
		}
	} else if !errors.Is(statErr, fs.ErrNotExist) {
		return fmt.Errorf("check %s: %w", filepath.Base(out), statErr)
	}

	dir := filepath.Dir(out)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(out)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		if err != nil {
			tmp.Close()
			os.Remove(tmpName)
		}
	}()

	bw := bufio.NewWriter(tmp)
	if err = write(bw); err != nil {
		return err
	}
	if err = bw.Flush(); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	if err = tmp.Sync(); err != nil {
		return fmt.Errorf("sync output: %w", err)
	}
	if err = tmp.Chmod(outputPerm); err != nil {
		return fmt.Errorf("chmod output: %w", err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("close output: %w", err)
	}
	if err = os.Rename(tmpName, out); err != nil {
		return fmt.Errorf("rename output: %w", err)
	}
	return nil
}

// progressReader reports progress and honours context cancellation.
//
// Cancellation is checked per Read rather than once up front so that
// cancelling a multi-gigabyte operation takes effect immediately.
type progressReader struct {
	ctx      context.Context
	r        io.Reader
	total    int64
	done     int64
	fn       Progress
	lastEmit time.Time
}

func newProgressReader(ctx context.Context, r io.Reader, total int64, fn Progress) io.Reader {
	return &progressReader{ctx: ctx, r: r, total: total, fn: fn}
}

func (p *progressReader) Read(b []byte) (int, error) {
	if err := p.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := p.r.Read(b)
	p.done += int64(n)

	if p.fn != nil {
		final := err != nil
		if final || time.Since(p.lastEmit) >= progressInterval {
			p.lastEmit = time.Now()
			p.fn(p.done, p.total)
		}
	}
	return n, err
}
