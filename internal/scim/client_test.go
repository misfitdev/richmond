package scim

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateUser(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/Users" {
			t.Errorf("expected /Users, got %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("missing or wrong authorization header")
		}
		if r.Header.Get("Content-Type") != "application/scim+json" {
			t.Errorf("wrong content type: %s", r.Header.Get("Content-Type"))
		}

		var u User
		json.NewDecoder(r.Body).Decode(&u)

		u.ID = "server-id-123"
		w.Header().Set("Content-Type", "application/scim+json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(u)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	user := &User{
		Schemas:    []string{UserSchema},
		ExternalID: "google-123",
		UserName:   "test@example.com",
		Active:     BoolPtr(true),
		Name:       &Name{GivenName: "Test", FamilyName: "User"},
	}

	created, err := client.CreateUser(context.Background(), user)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if created.ID != "server-id-123" {
		t.Errorf("expected ID server-id-123, got %s", created.ID)
	}
	if created.UserName != "test@example.com" {
		t.Errorf("expected UserName test@example.com, got %s", created.UserName)
	}
}

func TestFindUserByExternalID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		filter := r.URL.Query().Get("filter")
		if filter != `externalId eq "google-456"` {
			t.Errorf("unexpected filter: %s", filter)
		}

		resp := ListResponse{
			Schemas:      []string{ListResponseSchema},
			TotalResults: 1,
			Resources: []User{
				{ID: "scim-789", ExternalID: "google-456", UserName: "found@example.com"},
			},
		}
		w.Header().Set("Content-Type", "application/scim+json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	user, err := client.FindUserByExternalID(context.Background(), "google-456")
	if err != nil {
		t.Fatalf("FindUserByExternalID: %v", err)
	}
	if user == nil {
		t.Fatal("expected user, got nil")
	}
	if user.ID != "scim-789" {
		t.Errorf("expected ID scim-789, got %s", user.ID)
	}
}

func TestFindUserByExternalID_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ListResponse{
			Schemas:      []string{ListResponseSchema},
			TotalResults: 0,
			Resources:    []User{},
		}
		w.Header().Set("Content-Type", "application/scim+json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	user, err := client.FindUserByExternalID(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("FindUserByExternalID: %v", err)
	}
	if user != nil {
		t.Errorf("expected nil, got user %+v", user)
	}
}

func TestFindUserByUserName(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		filter := r.URL.Query().Get("filter")
		if filter != `userName eq "alice@example.com"` {
			t.Errorf("unexpected filter: %s", filter)
		}

		resp := ListResponse{
			Schemas:      []string{ListResponseSchema},
			TotalResults: 1,
			Resources: []User{
				{ID: "scim-111", UserName: "alice@example.com"},
			},
		}
		w.Header().Set("Content-Type", "application/scim+json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	user, err := client.FindUserByUserName(context.Background(), "alice@example.com")
	if err != nil {
		t.Fatalf("FindUserByUserName: %v", err)
	}
	if user == nil {
		t.Fatal("expected user, got nil")
	}
	if user.ID != "scim-111" {
		t.Errorf("expected ID scim-111, got %s", user.ID)
	}
}

func TestFindUserByUserName_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ListResponse{
			Schemas:      []string{ListResponseSchema},
			TotalResults: 0,
			Resources:    []User{},
		}
		w.Header().Set("Content-Type", "application/scim+json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	user, err := client.FindUserByUserName(context.Background(), "nobody@example.com")
	if err != nil {
		t.Fatalf("FindUserByUserName: %v", err)
	}
	if user != nil {
		t.Errorf("expected nil, got user %+v", user)
	}
}

func TestUpdateUser(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("expected PATCH, got %s", r.Method)
		}
		if r.URL.Path != "/Users/scim-123" {
			t.Errorf("expected /Users/scim-123, got %s", r.URL.Path)
		}

		var patch PatchOp
		json.NewDecoder(r.Body).Decode(&patch)

		if len(patch.Operations) == 0 {
			t.Error("expected operations in patch")
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	patch := NewPatchOp(Operation{Op: "replace", Path: "active", Value: false})
	err := client.UpdateUser(context.Background(), "scim-123", patch)
	if err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}
}

func TestDeleteUser(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	err := client.DeleteUser(context.Background(), "scim-123")
	if err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
}

func TestSCIMError_Conflict(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/scim+json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(ErrorResponse{
			Schemas: []string{ErrorSchema},
			Detail:  "Resource already exists",
			Status:  "409",
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	_, err := client.CreateUser(context.Background(), &User{Schemas: []string{UserSchema}})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrConflict) {
		t.Errorf("expected ErrConflict, got %v", err)
	}
}

func TestRetryOn429(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	err := client.DeleteUser(context.Background(), "test-id")
	if err != nil {
		t.Fatalf("expected success after retries, got: %v", err)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func FuzzUserUnmarshal(f *testing.F) {
	f.Add([]byte(`{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"id":"1","userName":"test@test.com"}`))
	f.Add([]byte(`{"totalResults":0,"Resources":[]}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"active":true}`))
	f.Add([]byte(`{"active":"not-a-bool"}`))
	f.Add([]byte(`null`))

	f.Fuzz(func(t *testing.T, data []byte) {
		var u User
		_ = json.Unmarshal(data, &u)

		var lr ListResponse
		_ = json.Unmarshal(data, &lr)

		var po PatchOp
		_ = json.Unmarshal(data, &po)
	})
}
