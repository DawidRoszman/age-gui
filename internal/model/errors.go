package model

import "errors"

// Sentinel errors describing domain failures. The view layer maps these to
// user-facing messages; nothing below the view layer should format prose for
// end users.
var (
	// ErrNoIdentity means no keypair has been created yet (first run).
	ErrNoIdentity = errors.New("no identity exists")

	// ErrIdentityExists means a keypair already exists. Generating over it
	// would destroy the only copy of the user's private key.
	ErrIdentityExists = errors.New("an identity already exists")

	// ErrLocked means the identity exists on disk but is not unlocked.
	ErrLocked = errors.New("identity is locked")

	// ErrWrongPassphrase means the passphrase did not decrypt the identity.
	// Distinct from ErrCorruptIdentity so the UI can say "wrong passphrase"
	// rather than alarming the user about a damaged key file.
	ErrWrongPassphrase = errors.New("wrong passphrase")

	// ErrCorruptIdentity means the identity file could not be parsed even
	// though it was decrypted.
	ErrCorruptIdentity = errors.New("identity file is damaged")

	// ErrNotForYou means a file was encrypted to recipients that do not
	// include this user's key.
	ErrNotForYou = errors.New("file was not encrypted for you")

	// ErrPassphraseRequired means the file is passphrase-encrypted (scrypt),
	// so a key cannot open it and the UI should prompt for a passphrase.
	ErrPassphraseRequired = errors.New("file is passphrase-encrypted")

	// ErrKeyRequired means a passphrase was offered for a file that was
	// encrypted to recipients, so it needs a private key instead. The mirror
	// image of ErrPassphraseRequired.
	ErrKeyRequired = errors.New("file needs a private key, not a passphrase")

	// ErrTargetExists means the output path is already taken. Encryption and
	// decryption never overwrite silently.
	ErrTargetExists = errors.New("output file already exists")

	// ErrDuplicateContact means a contact already holds that public key.
	ErrDuplicateContact = errors.New("a contact with that public key already exists")

	// ErrContactNotFound means no contact matched the given id.
	ErrContactNotFound = errors.New("contact not found")

	// ErrNoRecipients means an encrypt request named no recipients.
	ErrNoRecipients = errors.New("no recipients selected")

	// ErrSecretKeyGiven means the user supplied a private key where a public
	// key was expected. Worth its own error: it is a plausible mistake and the
	// user must be told to stop sharing it.
	ErrSecretKeyGiven = errors.New("that is a private key, not a public key")

	// ErrEmptyPassphrase means a blank passphrase was supplied.
	ErrEmptyPassphrase = errors.New("passphrase must not be empty")

	// ErrInvalidSettings means a settings value was out of range.
	ErrInvalidSettings = errors.New("invalid setting")

	// ErrNotAnIdentityFile means a file offered for restore held no age key.
	ErrNotAnIdentityFile = errors.New("that file does not contain an age key")
)
