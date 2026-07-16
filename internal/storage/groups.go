package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"dawidroszman.eu/encryptor/internal/model"
)

// groupsFileVersion is written alongside the records so a future format change
// can migrate rather than guess.
const groupsFileVersion = 1

// groupsDoc is the on-disk shape. The records live under a key rather than at
// the top level so fields can be added without breaking older files.
type groupsDoc struct {
	Version int           `json:"version"`
	Groups  []model.Group `json:"groups"`
}

// Groups persists the user's contact groups as JSON.
//
// Reads and writes are serialised by a mutex for the same reason as Contacts:
// Wails dispatches each JS call on its own goroutine, so two rapid UI actions
// could otherwise interleave a read-modify-write and silently drop a group.
type Groups struct {
	mu   sync.Mutex
	path string
}

// NewGroups returns a store rooted at dir.
func NewGroups(dir string) *Groups {
	return &Groups{path: filepath.Join(dir, groupsFile)}
}

// Path reports the groups file location.
func (s *Groups) Path() string { return s.path }

// List returns every group. A missing file is not an error: it just means the
// user has not created any groups yet.
func (s *Groups) List() ([]model.Group, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.load()
}

// Put inserts or replaces a group by id.
func (s *Groups) Put(g model.Group) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	all, err := s.load()
	if err != nil {
		return err
	}
	for i := range all {
		if all[i].ID == g.ID {
			all[i] = g
			return s.save(all)
		}
	}
	return s.save(append(all, g))
}

// Delete removes a group by id, reporting ErrGroupNotFound if absent.
func (s *Groups) Delete(id string) error {
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
	return model.ErrGroupNotFound
}

// load reads the file. Callers must hold s.mu.
func (s *Groups) load() ([]model.Group, error) {
	b, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read groups file: %w", err)
	}

	var doc groupsDoc
	if err := json.Unmarshal(b, &doc); err != nil {
		// Surface the path: the user's groups are recoverable by hand, and they
		// cannot do that if we do not say which file is broken.
		return nil, fmt.Errorf("groups file %s is not valid JSON: %w", s.path, err)
	}
	if doc.Version > groupsFileVersion {
		return nil, fmt.Errorf("groups file %s was written by a newer version of Encryptor (format %d, this build understands %d)",
			s.path, doc.Version, groupsFileVersion)
	}
	return doc.Groups, nil
}

// save writes the file atomically. Callers must hold s.mu.
func (s *Groups) save(all []model.Group) error {
	if all == nil {
		all = []model.Group{}
	}
	b, err := json.MarshalIndent(groupsDoc{
		Version: groupsFileVersion,
		Groups:  all,
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode groups: %w", err)
	}
	b = append(b, '\n')
	return writeFileAtomic(s.path, b, dataPerm)
}
