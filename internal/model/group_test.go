package model

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNewGroup(t *testing.T) {
	before := time.Now().UTC()

	g, err := NewGroup("  Team Alpha  ", []string{"a", "b"})
	if err != nil {
		t.Fatalf("NewGroup: %v", err)
	}

	if g.Name != "Team Alpha" {
		t.Errorf("Name = %q, want trimmed", g.Name)
	}
	if g.ID == "" {
		t.Error("ID is empty")
	}
	if len(g.MemberIDs) != 2 {
		t.Errorf("MemberIDs = %v, want two", g.MemberIDs)
	}
	if g.CreatedAt.Before(before) {
		t.Error("CreatedAt is before the call")
	}
	if g.CreatedAt.Location() != time.UTC {
		t.Error("CreatedAt should be UTC so the file is timezone-stable")
	}
}

// An empty group is a normal intermediate state: you create it, then add
// people. It must not be rejected.
func TestNewGroup_EmptyMembershipAllowed(t *testing.T) {
	g, err := NewGroup("Later", nil)
	if err != nil {
		t.Fatalf("NewGroup with no members: %v", err)
	}
	if len(g.MemberIDs) != 0 {
		t.Errorf("MemberIDs = %v, want empty", g.MemberIDs)
	}
}

// A contact ticked twice, or present via two paths, must not become two
// recipients or bloat the stored list.
func TestNewGroup_DedupesMembers(t *testing.T) {
	g, err := NewGroup("Dupes", []string{"a", "b", "a", "", "b", "c"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a", "b", "c"}
	if len(g.MemberIDs) != len(want) {
		t.Fatalf("MemberIDs = %v, want %v", g.MemberIDs, want)
	}
	for i, id := range want {
		if g.MemberIDs[i] != id {
			t.Errorf("MemberIDs[%d] = %q, want %q (first-seen order)", i, g.MemberIDs[i], id)
		}
	}
}

func TestNewGroup_IDsAreUnique(t *testing.T) {
	seen := make(map[string]bool)
	for range 100 {
		g, err := NewGroup("Team", nil)
		if err != nil {
			t.Fatal(err)
		}
		if seen[g.ID] {
			t.Fatalf("duplicate group ID %q", g.ID)
		}
		seen[g.ID] = true
	}
}

func TestNewGroup_Rejects(t *testing.T) {
	t.Run("empty name", func(t *testing.T) {
		_, err := NewGroup("", nil)
		if !errors.Is(err, ErrInvalidGroup) {
			t.Errorf("err = %v, want ErrInvalidGroup", err)
		}
	})
	t.Run("whitespace name", func(t *testing.T) {
		if _, err := NewGroup("   ", nil); !errors.Is(err, ErrInvalidGroup) {
			t.Errorf("err = %v, want ErrInvalidGroup", err)
		}
	})
	t.Run("overlong name", func(t *testing.T) {
		_, err := NewGroup(strings.Repeat("a", MaxGroupNameLen+1), nil)
		if !errors.Is(err, ErrInvalidGroup) {
			t.Errorf("err = %v, want ErrInvalidGroup", err)
		}
	})
}
