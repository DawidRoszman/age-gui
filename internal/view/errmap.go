package view

import (
	"context"
	"errors"

	"dawidroszman.eu/encryptor/internal/model"
)

// Error codes. These are contract with the frontend: the UI branches on them,
// so they must stay stable even if the wording changes.
const (
	CodeLocked             = "LOCKED"
	CodeNoIdentity         = "NO_IDENTITY"
	CodeIdentityExists     = "IDENTITY_EXISTS"
	CodeWrongPassphrase    = "WRONG_PASSPHRASE"
	CodeCorruptIdentity    = "CORRUPT_IDENTITY"
	CodeNotForYou          = "NOT_FOR_YOU"
	CodePassphraseRequired = "PASSPHRASE_REQUIRED"
	CodeKeyRequired        = "KEY_REQUIRED"
	CodeTargetExists       = "TARGET_EXISTS"
	CodeDuplicateContact   = "DUPLICATE_CONTACT"
	CodeContactNotFound    = "CONTACT_NOT_FOUND"
	CodeNoRecipients       = "NO_RECIPIENTS"
	CodeSecretKeyGiven     = "SECRET_KEY_GIVEN"
	CodeEmptyPassphrase    = "EMPTY_PASSPHRASE"
	CodeInvalidSettings    = "INVALID_SETTINGS"
	CodeNotAnIdentityFile  = "NOT_AN_IDENTITY_FILE"
	CodeCancelled          = "CANCELLED"
	CodeInternal           = "INTERNAL"
)

// mapError converts a domain error into a UI error.
//
// The messages are written for someone who does not know what a recipient or a
// stanza is. "no identity match" is true and useless; "this file wasn't
// encrypted for you" is what the person actually needs to hear.
//
// Anything unrecognised becomes CodeInternal and keeps its original text: a
// message we did not anticipate is more useful to a confused user, and to a bug
// report, than a reassuring lie.
func mapError(err error) *Error {
	if err == nil {
		return nil
	}

	switch {
	case errors.Is(err, model.ErrLocked):
		return &Error{CodeLocked, "Your key is locked. Unlock it to continue.", true}

	case errors.Is(err, model.ErrNoIdentity):
		return &Error{CodeNoIdentity, "You don't have a key yet. Create one to get started.", true}

	case errors.Is(err, model.ErrIdentityExists):
		return &Error{CodeIdentityExists, "You already have a key. Creating another would make every file encrypted to the old one unreadable.", true}

	case errors.Is(err, model.ErrWrongPassphrase):
		return &Error{CodeWrongPassphrase, "That passphrase isn't right. Try again.", true}

	case errors.Is(err, model.ErrCorruptIdentity):
		// Deliberately distinct from a wrong passphrase: this one warrants
		// reaching for a backup, and saying so wrongly would be alarming.
		return &Error{CodeCorruptIdentity, "Your key file appears to be damaged. If you have a backup, restore it.", false}

	case errors.Is(err, model.ErrNotForYou):
		return &Error{CodeNotForYou, "This file wasn't encrypted for you, so your key can't open it. Ask the sender to encrypt it to your public key.", true}

	case errors.Is(err, model.ErrPassphraseRequired):
		return &Error{CodePassphraseRequired, "This file is protected by a passphrase rather than a key.", true}

	case errors.Is(err, model.ErrKeyRequired):
		return &Error{CodeKeyRequired, "This file was encrypted to a key, not a passphrase.", true}

	case errors.Is(err, model.ErrTargetExists):
		return &Error{CodeTargetExists, "A file with that name already exists. Choose a different name so nothing is overwritten.", true}

	case errors.Is(err, model.ErrDuplicateContact):
		return &Error{CodeDuplicateContact, err.Error(), true}

	case errors.Is(err, model.ErrContactNotFound):
		return &Error{CodeContactNotFound, "That contact no longer exists.", true}

	case errors.Is(err, model.ErrNoRecipients):
		return &Error{CodeNoRecipients, "Choose at least one person to encrypt this for.", true}

	case errors.Is(err, model.ErrSecretKeyGiven):
		// The user is holding a private key and may be about to send it to
		// someone. This is the one error that should genuinely alarm them.
		return &Error{CodeSecretKeyGiven, "That's a private key, not a public key. Never share it with anyone — a public key starts with \"age1\".", true}

	case errors.Is(err, model.ErrEmptyPassphrase):
		return &Error{CodeEmptyPassphrase, "Please enter a passphrase.", true}

	case errors.Is(err, model.ErrInvalidSettings):
		// The service's message already names the acceptable range.
		return &Error{CodeInvalidSettings, err.Error(), true}

	case errors.Is(err, model.ErrNotAnIdentityFile):
		return &Error{CodeNotAnIdentityFile, "That file doesn't contain an age key. Choose the backup file you saved from this app, or a key file from age-keygen.", true}

	case errors.Is(err, context.Canceled):
		return &Error{CodeCancelled, "Cancelled.", true}

	default:
		return &Error{CodeInternal, err.Error(), false}
	}
}
