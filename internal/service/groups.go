package service

import (
	"cmp"
	"slices"
	"strings"

	"dawidroszman.eu/encryptor/internal/model"
)

// GroupService manages contact groups.
//
// It depends only on its store, exactly like ContactService. It does not verify
// that members reference real contacts: membership is built on the frontend
// from the live contact list, and the encrypt path expands a group against that
// same list, so a stale id simply resolves to nobody. PruneContact keeps the
// stored file tidy on top of that.
type GroupService struct {
	store GroupStore
}

// NewGroupService builds a GroupService over the given store.
func NewGroupService(store GroupStore) *GroupService {
	return &GroupService{store: store}
}

// List returns groups sorted by name, so the UI order is stable and predictable
// rather than reflecting creation order.
func (s *GroupService) List() ([]model.Group, error) {
	all, err := s.store.List()
	if err != nil {
		return nil, err
	}
	slices.SortFunc(all, func(a, b model.Group) int {
		if c := cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)); c != 0 {
			return c
		}
		// Names are not unique, so fall back to the id to keep the order total
		// and the list from shuffling between renders.
		return cmp.Compare(a.ID, b.ID)
	})
	return all, nil
}

// Get returns a single group by ID.
func (s *GroupService) Get(id string) (model.Group, error) {
	all, err := s.store.List()
	if err != nil {
		return model.Group{}, err
	}
	for _, g := range all {
		if g.ID == id {
			return g, nil
		}
	}
	return model.Group{}, model.ErrGroupNotFound
}

// Create builds and stores a new group.
func (s *GroupService) Create(name string, memberIDs []string) (model.Group, error) {
	g, err := model.NewGroup(name, memberIDs)
	if err != nil {
		return model.Group{}, err
	}
	if err := s.store.Put(g); err != nil {
		return model.Group{}, err
	}
	return g, nil
}

// Update replaces a group's name and members, keeping its id and creation time.
//
// The whole membership is replaced rather than diffed: the UI edits the set as
// a whole, and a replace cannot leave the stored group half-updated the way a
// sequence of add/remove calls could.
func (s *GroupService) Update(id, name string, memberIDs []string) (model.Group, error) {
	g, err := s.Get(id)
	if err != nil {
		return model.Group{}, err
	}

	// Reuse NewGroup's validation and dedupe, then keep the original identity.
	next, err := model.NewGroup(name, memberIDs)
	if err != nil {
		return model.Group{}, err
	}
	g.Name = next.Name
	g.MemberIDs = next.MemberIDs

	if err := s.store.Put(g); err != nil {
		return model.Group{}, err
	}
	return g, nil
}

// Delete removes a group.
func (s *GroupService) Delete(id string) error {
	return s.store.Delete(id)
}

// PruneContact removes a contact id from every group that contains it.
//
// Called when a contact is deleted, so groups never keep a dangling member.
// Best-effort tidiness: correctness does not depend on it, because group
// expansion on the frontend ignores ids that no longer resolve to a contact.
// Groups that do not contain the id are left untouched, so this writes only
// what it changes.
func (s *GroupService) PruneContact(contactID string) error {
	all, err := s.store.List()
	if err != nil {
		return err
	}
	for _, g := range all {
		idx := slices.Index(g.MemberIDs, contactID)
		if idx < 0 {
			continue
		}
		g.MemberIDs = slices.Delete(g.MemberIDs, idx, idx+1)
		if err := s.store.Put(g); err != nil {
			return err
		}
	}
	return nil
}
