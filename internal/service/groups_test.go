package service

import (
	"errors"
	"testing"

	"dawidroszman.eu/encryptor/internal/model"
)

func TestGroupService_ListSortsByName(t *testing.T) {
	svc := NewGroupService(&fakeGroupStore{})
	for _, name := range []string{"Zebra", "apple", "Mango"} {
		if _, err := svc.Create(name, nil); err != nil {
			t.Fatal(err)
		}
	}

	all, err := svc.List()
	if err != nil {
		t.Fatal(err)
	}
	// Case-insensitive: "apple" must not sort after the capitalised names.
	want := []string{"apple", "Mango", "Zebra"}
	for i, w := range want {
		if all[i].Name != w {
			t.Errorf("List()[%d] = %q, want %q", i, all[i].Name, w)
		}
	}
}

func TestGroupService_CreateUpdateDelete(t *testing.T) {
	svc := NewGroupService(&fakeGroupStore{})

	g, err := svc.Create("Team", []string{"a", "b"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	updated, err := svc.Update(g.ID, "Team Alpha", []string{"a", "b", "c"})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.ID != g.ID {
		t.Errorf("Update changed the id: %q -> %q", g.ID, updated.ID)
	}
	if updated.Name != "Team Alpha" || len(updated.MemberIDs) != 3 {
		t.Errorf("Update = %+v, want renamed with three members", updated)
	}
	// Identity is preserved across an edit.
	if !updated.CreatedAt.Equal(g.CreatedAt) {
		t.Error("Update must keep the original CreatedAt")
	}

	if err := svc.Delete(g.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := svc.Get(g.ID); !errors.Is(err, model.ErrGroupNotFound) {
		t.Errorf("Get after Delete = %v, want ErrGroupNotFound", err)
	}
}

func TestGroupService_UpdateMissing(t *testing.T) {
	svc := NewGroupService(&fakeGroupStore{})
	if _, err := svc.Update("nope", "X", nil); !errors.Is(err, model.ErrGroupNotFound) {
		t.Errorf("Update(missing) = %v, want ErrGroupNotFound", err)
	}
}

func TestGroupService_CreateRejectsEmptyName(t *testing.T) {
	svc := NewGroupService(&fakeGroupStore{})
	if _, err := svc.Create("  ", []string{"a"}); !errors.Is(err, model.ErrInvalidGroup) {
		t.Errorf("Create(blank) = %v, want ErrInvalidGroup", err)
	}
}

// PruneContact must strip the id from every group holding it, leave the others
// untouched, and not disturb a group that never had the member.
func TestGroupService_PruneContact(t *testing.T) {
	svc := NewGroupService(&fakeGroupStore{})

	team, _ := svc.Create("Team", []string{"a", "b", "c"})
	pair, _ := svc.Create("Pair", []string{"b", "d"})
	other, _ := svc.Create("Other", []string{"x", "y"})

	if err := svc.PruneContact("b"); err != nil {
		t.Fatalf("PruneContact: %v", err)
	}

	got := func(id string) model.Group {
		g, err := svc.Get(id)
		if err != nil {
			t.Fatal(err)
		}
		return g
	}

	if members := got(team.ID).MemberIDs; !equalIDs(members, []string{"a", "c"}) {
		t.Errorf("Team members = %v, want [a c]", members)
	}
	if members := got(pair.ID).MemberIDs; !equalIDs(members, []string{"d"}) {
		t.Errorf("Pair members = %v, want [d]", members)
	}
	if members := got(other.ID).MemberIDs; !equalIDs(members, []string{"x", "y"}) {
		t.Errorf("Other members = %v, want [x y] unchanged", members)
	}
}

// Deleting a contact fires the cascade hook with that contact's id, which is
// what prunes it from groups in production.
func TestContactService_DeleteCascadesToGroups(t *testing.T) {
	contacts := NewContactService(&fakeContactStore{})
	c, err := contacts.Add("Alice", x25519Pub(t), "")
	if err != nil {
		t.Fatal(err)
	}

	var pruned string
	contacts.SetOnDelete(func(id string) { pruned = id })

	if err := contacts.Delete(c.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if pruned != c.ID {
		t.Errorf("cascade saw %q, want the deleted id %q", pruned, c.ID)
	}
}

// End-to-end: the same wiring main.go uses must remove a deleted contact from
// the groups it belonged to.
func TestContactDelete_PrunesRealGroup(t *testing.T) {
	contacts := NewContactService(&fakeContactStore{})
	groups := NewGroupService(&fakeGroupStore{})
	contacts.SetOnDelete(func(id string) { _ = groups.PruneContact(id) })

	alice, _ := contacts.Add("Alice", x25519Pub(t), "")
	bob, _ := contacts.Add("Bob", x25519Pub(t), "")
	g, _ := groups.Create("Team", []string{alice.ID, bob.ID})

	if err := contacts.Delete(alice.ID); err != nil {
		t.Fatal(err)
	}

	after, err := groups.Get(g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !equalIDs(after.MemberIDs, []string{bob.ID}) {
		t.Errorf("group members = %v, want just Bob after Alice was deleted", after.MemberIDs)
	}
}

func equalIDs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
