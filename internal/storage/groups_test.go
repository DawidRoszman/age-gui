package storage

import (
	"errors"
	"os"
	"strings"
	"sync"
	"testing"

	"dawidroszman.eu/encryptor/internal/model"
)

func TestGroups_EmptyWhenMissing(t *testing.T) {
	s := NewGroups(t.TempDir())

	all, err := s.List()
	if err != nil {
		t.Fatalf("List on fresh dir: %v", err)
	}
	if len(all) != 0 {
		t.Errorf("List() = %d groups, want 0", len(all))
	}
}

func TestGroups_PutListDelete(t *testing.T) {
	s := NewGroups(t.TempDir())

	team, err := model.NewGroup("Team", []string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	family, err := model.NewGroup("Family", []string{"c"})
	if err != nil {
		t.Fatal(err)
	}

	if err := s.Put(team); err != nil {
		t.Fatalf("Put(team): %v", err)
	}
	if err := s.Put(family); err != nil {
		t.Fatalf("Put(family): %v", err)
	}

	all, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("List() = %d, want 2", len(all))
	}
	// Membership must survive the disk round-trip.
	for _, g := range all {
		if g.ID == team.ID && len(g.MemberIDs) != 2 {
			t.Errorf("team loaded with members %v, want two", g.MemberIDs)
		}
	}

	if err := s.Delete(team.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	all, err = s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || all[0].ID != family.ID {
		t.Errorf("after Delete(team), List() = %v, want just Family", all)
	}
}

func TestGroups_PutReplacesSameID(t *testing.T) {
	s := NewGroups(t.TempDir())

	g, err := model.NewGroup("Team", []string{"a"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Put(g); err != nil {
		t.Fatal(err)
	}

	g.Name = "Team Renamed"
	g.MemberIDs = []string{"a", "b", "c"}
	if err := s.Put(g); err != nil {
		t.Fatal(err)
	}

	all, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("List() = %d, want 1 — Put with an existing id must replace, not append", len(all))
	}
	if all[0].Name != "Team Renamed" || len(all[0].MemberIDs) != 3 {
		t.Errorf("group = %+v, want the updated name and members", all[0])
	}
}

func TestGroups_DeleteMissing(t *testing.T) {
	s := NewGroups(t.TempDir())

	if err := s.Delete("nope"); !errors.Is(err, model.ErrGroupNotFound) {
		t.Errorf("Delete(missing) = %v, want ErrGroupNotFound", err)
	}
}

func TestGroups_CorruptFileNamesThePath(t *testing.T) {
	dir := t.TempDir()
	s := NewGroups(dir)
	if err := os.WriteFile(s.Path(), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := s.List()
	if err == nil {
		t.Fatal("List() on a corrupt file = nil, want error")
	}
	if !strings.Contains(err.Error(), s.Path()) {
		t.Errorf("error %q does not name the offending file", err)
	}
}

func TestGroups_RejectsNewerFormat(t *testing.T) {
	s := NewGroups(t.TempDir())
	if err := os.WriteFile(s.Path(), []byte(`{"version":99,"groups":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := s.List(); err == nil {
		t.Error("List() on a newer-format file = nil, want error")
	}
}

// Wails runs each JS call on its own goroutine, so concurrent Puts are real.
// Without the mutex this drops groups via interleaved read-modify-write.
func TestGroups_ConcurrentPut(t *testing.T) {
	s := NewGroups(t.TempDir())

	const n = 20
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			g, err := model.NewGroup("Group", []string{"a"})
			if err != nil {
				errs <- err
				return
			}
			if err := s.Put(g); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent Put: %v", err)
	}

	all, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != n {
		t.Errorf("List() = %d groups, want %d — a concurrent write was lost", len(all), n)
	}
}
