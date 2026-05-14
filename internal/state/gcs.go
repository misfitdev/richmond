package state

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"cloud.google.com/go/storage"
)

type gcsStore struct {
	bucket string
	object string
}

func newGCSStore(path string) (*gcsStore, error) {
	// path is "gs://bucket/object/path"
	trimmed := strings.TrimPrefix(path, "gs://")
	parts := strings.SplitN(trimmed, "/", 2)
	if len(parts) < 2 || parts[1] == "" {
		return nil, fmt.Errorf("invalid GCS path %q: expected gs://bucket/object", path)
	}
	return &gcsStore{bucket: parts[0], object: parts[1]}, nil
}

func (s *gcsStore) Load() (*SyncState, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("create GCS client: %w", err)
	}
	defer func() { _ = client.Close() }()

	reader, err := client.Bucket(s.bucket).Object(s.object).NewReader(ctx)
	if errors.Is(err, storage.ErrObjectNotExist) {
		return Empty(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read GCS object: %w", err)
	}
	defer func() { _ = reader.Close() }()

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read GCS data: %w", err)
	}

	var st SyncState
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, fmt.Errorf("parse state from GCS: %w", err)
	}

	if st.Users == nil {
		st.Users = make(map[string]UserState)
	}
	if st.Groups == nil {
		st.Groups = make(map[string]GroupState)
	}

	return &st, nil
}

func (s *gcsStore) Save(st *SyncState) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := storage.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("create GCS client: %w", err)
	}
	defer func() { _ = client.Close() }()

	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	writer := client.Bucket(s.bucket).Object(s.object).NewWriter(ctx)
	writer.ContentType = "application/json"
	if _, err := writer.Write(data); err != nil {
		return fmt.Errorf("write GCS object: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("close GCS writer: %w", err)
	}

	return nil
}
