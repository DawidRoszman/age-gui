package model

import (
	"fmt"
	"strings"
	"time"
)

// MaxGroupNameLen bounds a group name, matching MaxContactNameLen so the two
// naming rules do not surprise a user by differing.
const MaxGroupNameLen = 128

// Group is a named set of contacts, so the user can pick "the whole team" in one
// action instead of ticking each person.
//
// Members are contact IDs, not keys. A contact's key is immutable and its ID is
// stable, so storing IDs tracks "these people" without duplicating key material
// or needing a reverse lookup. A group is only ever a selection shortcut: age
// still encrypts to individual keys, so a group never appears in an encrypted
// file.
//
// Like Contact, a Group holds no secrets, which keeps groups.json safe to store
// unencrypted and to back up.
type Group struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	MemberIDs []string  `json:"memberIds"`
	CreatedAt time.Time `json:"createdAt"`
}

// NewGroup validates and builds a Group, assigning it a fresh id.
//
// An empty group is allowed: creating one and then adding people is a normal
// flow, and an empty group simply contributes no recipients.
func NewGroup(name string, memberIDs []string) (Group, error) {
	name = strings.TrimSpace(name)
	if err := validateGroupName(name); err != nil {
		return Group{}, err
	}

	id, err := newID()
	if err != nil {
		return Group{}, err
	}
	return Group{
		ID:        id,
		Name:      name,
		MemberIDs: dedupe(memberIDs),
		CreatedAt: time.Now().UTC(),
	}, nil
}

// validateGroupName enforces the shared naming rule. Extracted so create and
// rename cannot drift apart.
func validateGroupName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: name must not be empty", ErrInvalidGroup)
	}
	if len(name) > MaxGroupNameLen {
		return fmt.Errorf("%w: name must be at most %d characters", ErrInvalidGroup, MaxGroupNameLen)
	}
	return nil
}

// dedupe removes repeated member IDs while preserving first-seen order, so a
// contact picked twice does not become two recipients or clutter the stored
// list.
func dedupe(ids []string) []string {
	seen := make(map[string]struct{}, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
