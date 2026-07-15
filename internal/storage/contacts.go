package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"dawidroszman.eu/age-gui/internal/model"
)

// contactsFileVersion is written alongside the records so a future format
// change can migrate rather than guess.
const contactsFileVersion = 1

// contactsDoc is the on-disk shape. The records live under a key rather than at
// the top level so fields can be added without breaking older files.
type contactsDoc struct {
	Version  int             `json:"version"`
	Contacts []model.Contact `json:"contacts"`
}

// Contacts persists the address book as JSON.
//
// Reads and writes are serialised by a mutex: Wails dispatches each JS call on
// its own goroutine, so two rapid UI actions could otherwise interleave a
// read-modify-write and silently drop a contact.
type Contacts struct {
	mu   sync.Mutex
	path string
}

// NewContacts returns a store rooted at dir.
func NewContacts(dir string) *Contacts {
	return &Contacts{path: filepath.Join(dir, contactsFile)}
}

// Path reports the contacts file location.
func (s *Contacts) Path() string { return s.path }

// List returns every contact. A missing file is not an error: it just means
// the user has not added anyone yet.
func (s *Contacts) List() ([]model.Contact, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.load()
}

// Put inserts or replaces a contact by id.
func (s *Contacts) Put(c model.Contact) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	all, err := s.load()
	if err != nil {
		return err
	}
	for i := range all {
		if all[i].ID == c.ID {
			all[i] = c
			return s.save(all)
		}
	}
	return s.save(append(all, c))
}

// Delete removes a contact by id, reporting ErrContactNotFound if absent.
func (s *Contacts) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	all, err := s.load()
	if err != nil {
		return err
	}
	for i := range all {
		if all[i].ID == id {
			return s.save(append(all[:i:i], all[i+1:]...))
		}
	}
	return model.ErrContactNotFound
}

// load reads the file. Callers must hold s.mu.
func (s *Contacts) load() ([]model.Contact, error) {
	b, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read contacts file: %w", err)
	}

	var doc contactsDoc
	if err := json.Unmarshal(b, &doc); err != nil {
		// Surface the path: the user's contacts are recoverable by hand, and
		// they cannot do that if we do not say which file is broken.
		return nil, fmt.Errorf("contacts file %s is not valid JSON: %w", s.path, err)
	}
	if doc.Version > contactsFileVersion {
		return nil, fmt.Errorf("contacts file %s was written by a newer version of age-gui (format %d, this build understands %d)",
			s.path, doc.Version, contactsFileVersion)
	}
	return doc.Contacts, nil
}

// save writes the file atomically. Callers must hold s.mu.
func (s *Contacts) save(all []model.Contact) error {
	if all == nil {
		all = []model.Contact{}
	}
	b, err := json.MarshalIndent(contactsDoc{
		Version:  contactsFileVersion,
		Contacts: all,
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode contacts: %w", err)
	}
	b = append(b, '\n')
	return writeFileAtomic(s.path, b, dataPerm)
}
