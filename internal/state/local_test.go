package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLocalStore_SaveIsAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	store := &localStore{path: path}

	st := &SyncState{
		Users:  map[string]UserState{"g1": {SCIMID: "s1", Active: true, Email: "a@b.com"}},
		Groups: make(map[string]GroupState),
	}
	if err := store.Save(st); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// No tmp files should be left behind.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".richmond-state-") {
			t.Errorf("temporary file not cleaned up: %s", e.Name())
		}
	}

	// File permissions must be 0600.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("state file mode = %o, want 0600", info.Mode().Perm())
	}
}

func TestLocalStore_LoadMissing(t *testing.T) {
	store := &localStore{path: filepath.Join(t.TempDir(), "missing.json")}
	st, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(st.Users) != 0 {
		t.Errorf("expected empty users, got %d", len(st.Users))
	}
	if len(st.Groups) != 0 {
		t.Errorf("expected empty groups, got %d", len(st.Groups))
	}
}

func TestLocalStore_SaveAndLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	store := &localStore{path: path}

	now := time.Now().UTC().Truncate(time.Second)
	original := &SyncState{
		LastSync: now,
		Users: map[string]UserState{
			"g1": {SCIMID: "s1", Hash: "h1", Active: true, Email: "test@example.com"},
		},
		Groups: map[string]GroupState{
			"gg1": {SCIMID: "sg1", Hash: "gh1", Name: "Engineering"},
		},
	}

	if err := store.Save(original); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("state file not created: %v", err)
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if !loaded.LastSync.Equal(now) {
		t.Errorf("LastSync = %v, want %v", loaded.LastSync, now)
	}
	if u, ok := loaded.Users["g1"]; !ok {
		t.Error("expected user g1 in loaded state")
	} else {
		if u.SCIMID != "s1" {
			t.Errorf("SCIMID = %q, want s1", u.SCIMID)
		}
		if u.Email != "test@example.com" {
			t.Errorf("Email = %q, want test@example.com", u.Email)
		}
	}
	if g, ok := loaded.Groups["gg1"]; !ok {
		t.Error("expected group gg1 in loaded state")
	} else if g.Name != "Engineering" {
		t.Errorf("Group Name = %q, want Engineering", g.Name)
	}
}
