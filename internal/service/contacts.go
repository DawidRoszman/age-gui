package service

import (
	"cmp"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"filippo.io/age"

	"dawidroszman.eu/age-gui/internal/model"
)

// recipientFileSizeLimit bounds an imported public key file. Recipient files
// are a few KiB even with hybrid keys; this stops someone handing the user a
// multi-gigabyte "public key".
const recipientFileSizeLimit = 1 << 20 // 1 MiB

// ContactService manages the address book.
type ContactService struct {
	store ContactStore
}

// NewContactService builds a ContactService over the given store.
func NewContactService(store ContactStore) *ContactService {
	return &ContactService{store: store}
}

// List returns contacts sorted by name, so the UI order is stable and
// predictable rather than reflecting insertion order.
func (s *ContactService) List() ([]model.Contact, error) {
	all, err := s.store.List()
	if err != nil {
		return nil, err
	}
	slices.SortFunc(all, func(a, b model.Contact) int {
		if c := cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)); c != 0 {
			return c
		}
		// Names are not unique, so fall back to the id to keep the order total
		// and the list from shuffling between renders.
		return cmp.Compare(a.ID, b.ID)
	})
	return all, nil
}

// Get returns a single contact by ID.
func (s *ContactService) Get(id string) (model.Contact, error) {
	all, err := s.store.List()
	if err != nil {
		return model.Contact{}, err
	}
	for _, c := range all {
		if c.ID == id {
			return c, nil
		}
	}
	return model.Contact{}, model.ErrContactNotFound
}

// Add creates a contact from a pasted public key.
func (s *ContactService) Add(name, publicKey, note string) (model.Contact, error) {
	key, err := model.ParsePublicKey(publicKey)
	if err != nil {
		return model.Contact{}, err
	}
	return s.add(name, key, note)
}

// AddFromFile creates a contact from an exported public key file, of the same
// shape age's -R flag accepts.
func (s *ContactService) AddFromFile(name, path, note string) (model.Contact, error) {
	f, err := os.Open(path)
	if err != nil {
		return model.Contact{}, fmt.Errorf("open public key file: %w", err)
	}
	defer f.Close()

	// age.ParseRecipients handles comments, blank lines, and both key types.
	// Reusing it means an exported key file from any age tool just works.
	recipients, err := age.ParseRecipients(io.LimitReader(f, recipientFileSizeLimit))
	if err != nil {
		return model.Contact{}, fmt.Errorf("read public key file: %w", err)
	}
	if len(recipients) != 1 {
		return model.Contact{}, fmt.Errorf("file holds %d public keys; a contact needs exactly one", len(recipients))
	}

	// Re-encode through the model so the stored value is validated and
	// canonical, exactly as if it had been pasted.
	key, err := model.ParsePublicKey(recipientString(recipients[0]))
	if err != nil {
		return model.Contact{}, err
	}
	return s.add(name, key, note)
}

func (s *ContactService) add(name string, key model.PublicKey, note string) (model.Contact, error) {
	existing, err := s.store.List()
	if err != nil {
		return model.Contact{}, err
	}
	// Two contacts sharing a key means the encrypt screen shows the same
	// person twice with no way to tell them apart.
	for _, c := range existing {
		if c.PublicKey.Equal(key) {
			return model.Contact{}, fmt.Errorf("%w: %q already has that key", model.ErrDuplicateContact, c.Name)
		}
	}

	c, err := model.NewContact(name, key, note)
	if err != nil {
		return model.Contact{}, err
	}
	if err := s.store.Put(c); err != nil {
		return model.Contact{}, err
	}
	return c, nil
}

// Rename updates a contact's name and note. The public key is immutable: a
// different key is a different person, and editing it in place would silently
// redirect future encryptions.
func (s *ContactService) Rename(id, name, note string) (model.Contact, error) {
	c, err := s.Get(id)
	if err != nil {
		return model.Contact{}, err
	}

	name = strings.TrimSpace(name)
	if name == "" {
		return model.Contact{}, fmt.Errorf("contact name must not be empty")
	}
	if len(name) > model.MaxContactNameLen {
		return model.Contact{}, fmt.Errorf("contact name must be at most %d characters", model.MaxContactNameLen)
	}

	c.Name = name
	c.Note = strings.TrimSpace(note)
	if err := s.store.Put(c); err != nil {
		return model.Contact{}, err
	}
	return c, nil
}

// Delete removes a contact.
func (s *ContactService) Delete(id string) error {
	return s.store.Delete(id)
}

// Recipients resolves contact IDs to public keys for encryption.
//
// It fails on the first unknown ID rather than encrypting to a subset: silently
// dropping a recipient would produce a file the user believes they shared and
// which that person cannot open.
func (s *ContactService) Recipients(ids []string) ([]model.PublicKey, error) {
	if len(ids) == 0 {
		return nil, model.ErrNoRecipients
	}
	all, err := s.store.List()
	if err != nil {
		return nil, err
	}
	byID := make(map[string]model.Contact, len(all))
	for _, c := range all {
		byID[c.ID] = c
	}

	keys := make([]model.PublicKey, 0, len(ids))
	for _, id := range ids {
		c, ok := byID[id]
		if !ok {
			return nil, fmt.Errorf("%w: %s", model.ErrContactNotFound, id)
		}
		keys = append(keys, c.PublicKey)
	}
	return keys, nil
}

// recipientString renders a parsed recipient back to its encoding.
func recipientString(r age.Recipient) string {
	if s, ok := r.(fmt.Stringer); ok {
		return s.String()
	}
	return ""
}
