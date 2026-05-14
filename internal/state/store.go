package state

import (
	"fmt"
	"strings"
	"time"
)

// SyncState tracks the result of a previous sync for incremental mode.
type SyncState struct {
	LastSync time.Time             `json:"last_sync"`
	Users    map[string]UserState  `json:"users"`
	Groups   map[string]GroupState `json:"groups"`
}

// UserState tracks a synced user's SCIM identity and content hash.
type UserState struct {
	SCIMID string `json:"scim_id"`
	Hash   string `json:"hash"`
	Active bool   `json:"active"`
	Email  string `json:"email"`
}

// GroupState tracks a synced group's SCIM identity and content hash.
type GroupState struct {
	SCIMID string `json:"scim_id"`
	Hash   string `json:"hash"`
	Name   string `json:"name"`
}

// Store is the interface for loading and saving sync state.
type Store interface {
	Load() (*SyncState, error)
	Save(state *SyncState) error
}

// NewStore creates a Store based on the path.
// Paths starting with "gs://" use GCS; everything else is a local file.
func NewStore(path string) (Store, error) {
	if path == "" {
		return nil, fmt.Errorf("state file path is required")
	}
	if strings.HasPrefix(path, "gs://") {
		return newGCSStore(path)
	}
	return &localStore{path: path}, nil
}

// Empty returns an initialized empty state.
func Empty() *SyncState {
	return &SyncState{
		Users:  make(map[string]UserState),
		Groups: make(map[string]GroupState),
	}
}
