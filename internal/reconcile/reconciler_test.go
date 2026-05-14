package reconcile

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	admin "google.golang.org/api/admin/directory/v1"

	"github.com/misfitdev/richmond/internal/config"
	"github.com/misfitdev/richmond/internal/mapping"
	"github.com/misfitdev/richmond/internal/scim"
	"github.com/misfitdev/richmond/internal/state"
)

func newTestMapper() *mapping.Mapper {
	return mapping.New(&config.Config{
		SCIM: config.SCIMConfig{
			Attributes: []string{"external_id", "user_name", "active", "name", "emails"},
		},
	})
}

func TestReconcile_CreateNewUsers(t *testing.T) {
	var idCounter atomic.Int64

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/Users":
			// FindByExternalID returns empty
			json.NewEncoder(w).Encode(scim.ListResponse{TotalResults: 0})
		case r.Method == http.MethodPost && r.URL.Path == "/Users":
			var u scim.User
			json.NewDecoder(r.Body).Decode(&u)
			u.ID = fmt.Sprintf("scim-%d", idCounter.Add(1))
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(u)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := scim.NewClient(server.URL, "token")
	mapper := newTestMapper()
	rec := New(client, mapper, false)

	users := []*admin.User{
		{Id: "g1", PrimaryEmail: "alice@example.com", Name: &admin.UserName{GivenName: "Alice", FamilyName: "A"}},
		{Id: "g2", PrimaryEmail: "bob@example.com", Name: &admin.UserName{GivenName: "Bob", FamilyName: "B"}},
	}

	result, err := rec.Reconcile(context.Background(), users, nil, nil, state.Empty())
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if result.Stats.UsersCreated != 2 {
		t.Errorf("UsersCreated = %d, want 2", result.Stats.UsersCreated)
	}
	if len(result.State.Users) != 2 {
		t.Errorf("state users = %d, want 2", len(result.State.Users))
	}
	for _, us := range result.State.Users {
		if us.SCIMID == "" {
			t.Error("expected SCIM ID to be set in state")
		}
	}
}

func TestReconcile_SkipUnchanged(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("no SCIM calls expected for unchanged users, got %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := scim.NewClient(server.URL, "token")
	mapper := newTestMapper()

	user := &admin.User{
		Id:           "g1",
		PrimaryEmail: "alice@example.com",
		Name:         &admin.UserName{GivenName: "Alice", FamilyName: "A"},
	}
	su := mapper.MapUser(user)
	hash := mapping.HashUser(su)

	prev := &state.SyncState{
		Users: map[string]state.UserState{
			"g1": {SCIMID: "scim-1", Hash: hash, Active: true, Email: "alice@example.com"},
		},
		Groups: make(map[string]state.GroupState),
	}

	rec := New(client, mapper, false)
	result, err := rec.Reconcile(context.Background(), []*admin.User{user}, nil, nil, prev)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if result.Stats.UsersSkipped != 1 {
		t.Errorf("UsersSkipped = %d, want 1", result.Stats.UsersSkipped)
	}
	if result.Stats.UsersCreated != 0 || result.Stats.UsersUpdated != 0 {
		t.Error("expected no creates or updates for unchanged user")
	}
}

func TestReconcile_UpdateChangedUser(t *testing.T) {
	patchCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPatch && r.URL.Path == "/Users/scim-1":
			patchCalled = true
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := scim.NewClient(server.URL, "token")
	mapper := newTestMapper()

	prev := &state.SyncState{
		Users: map[string]state.UserState{
			"g1": {SCIMID: "scim-1", Hash: "old-hash", Active: true, Email: "alice@example.com"},
		},
		Groups: make(map[string]state.GroupState),
	}

	users := []*admin.User{
		{Id: "g1", PrimaryEmail: "alice@example.com", Name: &admin.UserName{GivenName: "Alice", FamilyName: "Updated"}},
	}

	rec := New(client, mapper, false)
	result, err := rec.Reconcile(context.Background(), users, nil, nil, prev)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if !patchCalled {
		t.Error("expected PATCH call for changed user")
	}
	if result.Stats.UsersUpdated != 1 {
		t.Errorf("UsersUpdated = %d, want 1", result.Stats.UsersUpdated)
	}
}

func TestReconcile_DeactivateRemovedUser(t *testing.T) {
	patchCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPatch && r.URL.Path == "/Users/scim-1":
			patchCalled = true
			var patch scim.PatchOp
			json.NewDecoder(r.Body).Decode(&patch)
			for _, op := range patch.Operations {
				if op.Path == "active" && op.Value == false {
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(`{}`))
					return
				}
			}
			t.Error("expected active=false in patch")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := scim.NewClient(server.URL, "token")
	mapper := newTestMapper()

	prev := &state.SyncState{
		Users: map[string]state.UserState{
			"g-removed": {SCIMID: "scim-1", Hash: "some-hash", Active: true, Email: "removed@example.com"},
		},
		Groups: make(map[string]state.GroupState),
	}

	// Empty user list = user was removed from Google
	rec := New(client, mapper, false)
	result, err := rec.Reconcile(context.Background(), nil, nil, nil, prev)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if !patchCalled {
		t.Error("expected PATCH call to deactivate removed user")
	}
	if result.Stats.UsersDeactivated != 1 {
		t.Errorf("UsersDeactivated = %d, want 1", result.Stats.UsersDeactivated)
	}
}

func TestReconcile_DeactivationFailureRetainedInState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// SCIM returns 500 for all PATCH calls, simulating a transient failure.
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"detail":"internal error"}`))
	}))
	defer server.Close()

	client := scim.NewClient(server.URL, "token")
	mapper := newTestMapper()

	prev := &state.SyncState{
		Users: map[string]state.UserState{
			"g-removed": {SCIMID: "scim-1", Hash: "some-hash", Active: true, Email: "removed@example.com"},
		},
		Groups: make(map[string]state.GroupState),
	}

	rec := New(client, mapper, false)
	result, err := rec.Reconcile(context.Background(), nil, nil, nil, prev)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if result.Stats.Errors != 1 {
		t.Errorf("Errors = %d, want 1", result.Stats.Errors)
	}

	// The user must be retained in state so the next run re-attempts deactivation.
	us, ok := result.State.Users["g-removed"]
	if !ok {
		t.Fatal("expected failed-to-deactivate user to be retained in state for retry")
	}
	if us.SCIMID != "scim-1" {
		t.Errorf("SCIMID = %q, want scim-1", us.SCIMID)
	}
	if !us.Active {
		t.Error("retained user state should still be Active=true so next run retries deactivation")
	}
}

func TestReconcile_GroupDeleteFailureRetainedInState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"detail":"internal error"}`))
	}))
	defer server.Close()

	client := scim.NewClient(server.URL, "token")
	mapper := newTestMapper()

	prev := &state.SyncState{
		Users: make(map[string]state.UserState),
		Groups: map[string]state.GroupState{
			"g-removed": {SCIMID: "scim-g1", Hash: "some-hash", Name: "OldGroup"},
		},
	}

	rec := New(client, mapper, false)
	result, err := rec.Reconcile(context.Background(), nil, nil, nil, prev)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if result.Stats.Errors != 1 {
		t.Errorf("Errors = %d, want 1", result.Stats.Errors)
	}

	gs, ok := result.State.Groups["g-removed"]
	if !ok {
		t.Fatal("expected failed-to-delete group to be retained in state for retry")
	}
	if gs.SCIMID != "scim-g1" {
		t.Errorf("SCIMID = %q, want scim-g1", gs.SCIMID)
	}
}

func TestReconcile_DeactivationGone404DropsFromState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := scim.NewClient(server.URL, "token")
	mapper := newTestMapper()

	prev := &state.SyncState{
		Users: map[string]state.UserState{
			"g-gone": {SCIMID: "scim-1", Hash: "h", Active: true, Email: "gone@example.com"},
		},
		Groups: make(map[string]state.GroupState),
	}

	rec := New(client, mapper, false)
	result, err := rec.Reconcile(context.Background(), nil, nil, nil, prev)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if result.Stats.Errors != 0 {
		t.Errorf("Errors = %d, want 0 (404 is not an error)", result.Stats.Errors)
	}
	if result.Stats.UsersDeactivated != 1 {
		t.Errorf("UsersDeactivated = %d, want 1", result.Stats.UsersDeactivated)
	}
	if _, ok := result.State.Users["g-gone"]; ok {
		t.Error("user should be dropped from state after 404 (already gone)")
	}
}

func TestReconcile_GroupDelete404DropsFromState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := scim.NewClient(server.URL, "token")
	mapper := newTestMapper()

	prev := &state.SyncState{
		Users: make(map[string]state.UserState),
		Groups: map[string]state.GroupState{
			"g-gone": {SCIMID: "scim-g1", Hash: "h", Name: "GoneGroup"},
		},
	}

	rec := New(client, mapper, false)
	result, err := rec.Reconcile(context.Background(), nil, nil, nil, prev)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if result.Stats.Errors != 0 {
		t.Errorf("Errors = %d, want 0 (404 is not an error)", result.Stats.Errors)
	}
	if result.Stats.GroupsDeleted != 1 {
		t.Errorf("GroupsDeleted = %d, want 1", result.Stats.GroupsDeleted)
	}
	if _, ok := result.State.Groups["g-gone"]; ok {
		t.Error("group should be dropped from state after 404 (already gone)")
	}
}

func TestReconcile_DryRun(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/Users":
			json.NewEncoder(w).Encode(scim.ListResponse{TotalResults: 0})
		default:
			t.Errorf("unexpected non-GET request in dry-run: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	client := scim.NewClient(server.URL, "token")
	mapper := newTestMapper()
	rec := New(client, mapper, true)

	users := []*admin.User{
		{Id: "g1", PrimaryEmail: "alice@example.com", Name: &admin.UserName{GivenName: "Alice", FamilyName: "A"}},
	}

	result, err := rec.Reconcile(context.Background(), users, nil, nil, state.Empty())
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if result.Stats.UsersCreated != 1 {
		t.Errorf("UsersCreated = %d, want 1 (dry-run should still count)", result.Stats.UsersCreated)
	}
}
