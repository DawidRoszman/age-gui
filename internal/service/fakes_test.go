package service

import (
	"errors"
	"sync"

	"dawidroszman.eu/age-gui/internal/model"
)

// The fakes below are the payoff for declaring ports in this package: every use
// case is exercised without touching a disk or a GUI.

// fakeIdentityStore is an in-memory IdentityStore.
type fakeIdentityStore struct {
	mu   sync.Mutex
	blob []byte

	// failNext, when set, is returned by the next call and then cleared. It
	// lets tests drive the error paths that a real disk almost never takes.
	failNext error
}

func (f *fakeIdentityStore) take() error {
	err := f.failNext
	f.failNext = nil
	return err
}

func (f *fakeIdentityStore) Exists() (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.take(); err != nil {
		return false, err
	}
	return f.blob != nil, nil
}

func (f *fakeIdentityStore) Load() ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.take(); err != nil {
		return nil, err
	}
	if f.blob == nil {
		return nil, errors.New("no identity stored")
	}
	return f.blob, nil
}

func (f *fakeIdentityStore) Save(ciphertext []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.take(); err != nil {
		return err
	}
	f.blob = append([]byte(nil), ciphertext...)
	return nil
}

// fakeContactStore is an in-memory ContactStore.
type fakeContactStore struct {
	mu       sync.Mutex
	contacts []model.Contact
}

func (f *fakeContactStore) List() ([]model.Contact, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]model.Contact(nil), f.contacts...), nil
}

func (f *fakeContactStore) Put(c model.Contact) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.contacts {
		if f.contacts[i].ID == c.ID {
			f.contacts[i] = c
			return nil
		}
	}
	f.contacts = append(f.contacts, c)
	return nil
}

func (f *fakeContactStore) Delete(id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.contacts {
		if f.contacts[i].ID == id {
			f.contacts = append(f.contacts[:i:i], f.contacts[i+1:]...)
			return nil
		}
	}
	return model.ErrContactNotFound
}

// Compile-time proof the fakes satisfy the ports they stand in for.
var (
	_ IdentityStore = (*fakeIdentityStore)(nil)
	_ ContactStore  = (*fakeContactStore)(nil)
)
