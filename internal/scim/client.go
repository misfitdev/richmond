package scim

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ErrNotFound is returned when a SCIM resource does not exist (HTTP 404).
var ErrNotFound = errors.New("resource not found")

// ErrConflict is returned when a SCIM resource already exists (HTTP 409).
var ErrConflict = errors.New("resource already exists")

type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
	maxRetries int
}

func NewClient(endpoint, bearerToken string) *Client {
	return &Client{
		baseURL:    strings.TrimRight(endpoint, "/"),
		token:      bearerToken,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		maxRetries: 3,
	}
}

// CreateUser creates a new SCIM user. Returns the created user with server-assigned ID.
func (c *Client) CreateUser(ctx context.Context, user *User) (*User, error) {
	var result User
	if err := c.do(ctx, http.MethodPost, "/Users", user, &result); err != nil {
		return nil, fmt.Errorf("create user %s: %w", user.UserName, err)
	}
	return &result, nil
}

// GetUser fetches a single SCIM user by ID.
func (c *Client) GetUser(ctx context.Context, id string) (*User, error) {
	var result User
	if err := c.do(ctx, http.MethodGet, "/Users/"+id, nil, &result); err != nil {
		return nil, fmt.Errorf("get user %s: %w", id, err)
	}
	return &result, nil
}

// FindUserByExternalID looks up a user by externalId filter.
func (c *Client) FindUserByExternalID(ctx context.Context, externalID string) (*User, error) {
	filter := fmt.Sprintf("externalId eq %q", externalID)
	params := url.Values{"filter": {filter}, "count": {"1"}}
	var result ListResponse
	if err := c.do(ctx, http.MethodGet, "/Users?"+params.Encode(), nil, &result); err != nil {
		return nil, fmt.Errorf("find user by externalId %s: %w", externalID, err)
	}
	if result.TotalResults == 0 || len(result.Resources) == 0 {
		return nil, nil
	}
	return &result.Resources[0], nil
}

// FindUserByUserName looks up a user by userName filter.
func (c *Client) FindUserByUserName(ctx context.Context, userName string) (*User, error) {
	filter := fmt.Sprintf("userName eq %q", userName)
	params := url.Values{"filter": {filter}, "count": {"1"}}
	var result ListResponse
	if err := c.do(ctx, http.MethodGet, "/Users?"+params.Encode(), nil, &result); err != nil {
		return nil, fmt.Errorf("find user by userName %s: %w", userName, err)
	}
	if result.TotalResults == 0 || len(result.Resources) == 0 {
		return nil, nil
	}
	return &result.Resources[0], nil
}

// UpdateUser applies a PATCH operation to a user.
func (c *Client) UpdateUser(ctx context.Context, id string, patch *PatchOp) error {
	if err := c.do(ctx, http.MethodPatch, "/Users/"+id, patch, nil); err != nil {
		return fmt.Errorf("update user %s: %w", id, err)
	}
	return nil
}

// DeleteUser removes a user by ID.
func (c *Client) DeleteUser(ctx context.Context, id string) error {
	if err := c.do(ctx, http.MethodDelete, "/Users/"+id, nil, nil); err != nil {
		return fmt.Errorf("delete user %s: %w", id, err)
	}
	return nil
}

// CreateGroup creates a new SCIM group.
func (c *Client) CreateGroup(ctx context.Context, group *Group) (*Group, error) {
	var result Group
	if err := c.do(ctx, http.MethodPost, "/Groups", group, &result); err != nil {
		return nil, fmt.Errorf("create group %s: %w", group.DisplayName, err)
	}
	return &result, nil
}

// FindGroupByExternalID looks up a group by externalId filter.
func (c *Client) FindGroupByExternalID(ctx context.Context, externalID string) (*Group, error) {
	filter := fmt.Sprintf("externalId eq %q", externalID)
	params := url.Values{"filter": {filter}, "count": {"1"}}
	var result GroupListResponse
	if err := c.do(ctx, http.MethodGet, "/Groups?"+params.Encode(), nil, &result); err != nil {
		return nil, fmt.Errorf("find group by externalId %s: %w", externalID, err)
	}
	if result.TotalResults == 0 || len(result.Resources) == 0 {
		return nil, nil
	}
	return &result.Resources[0], nil
}

// UpdateGroup applies a PATCH operation to a group.
func (c *Client) UpdateGroup(ctx context.Context, id string, patch *PatchOp) error {
	if err := c.do(ctx, http.MethodPatch, "/Groups/"+id, patch, nil); err != nil {
		return fmt.Errorf("update group %s: %w", id, err)
	}
	return nil
}

// DeleteGroup removes a group by ID.
func (c *Client) DeleteGroup(ctx context.Context, id string) error {
	if err := c.do(ctx, http.MethodDelete, "/Groups/"+id, nil, nil); err != nil {
		return fmt.Errorf("delete group %s: %w", id, err)
	}
	return nil
}

// SupportsGroups queries /ResourceTypes to check if the SCIM endpoint
// advertises Group as a supported resource type.
// DiscoverSchemas queries the /Schemas endpoint and returns the schemas
// the SCIM provider advertises. Handles both bare arrays and ListResponse envelopes.
func (c *Client) DiscoverSchemas(ctx context.Context) ([]Schema, error) {
	var schemas []Schema
	if err := c.do(ctx, http.MethodGet, "/Schemas", nil, &schemas); err == nil && len(schemas) > 0 {
		return schemas, nil
	}
	var envelope SchemaListResponse
	if err := c.do(ctx, http.MethodGet, "/Schemas", nil, &envelope); err != nil {
		return nil, err
	}
	return envelope.Resources, nil
}

// DiscoverSupport queries /Schemas and returns a summary of provider capabilities.
// Returns nil on error so callers can fall back to configured attributes.
func (c *Client) DiscoverSupport(ctx context.Context) *SchemaSupport {
	schemas, err := c.DiscoverSchemas(ctx)
	if err != nil {
		slog.Warn("could not query SCIM Schemas, using configured attributes as-is", "err", err)
		return nil
	}

	support := &SchemaSupport{
		UserAttributes:       make(map[string]bool),
		EnterpriseAttributes: make(map[string]bool),
	}

	for _, s := range schemas {
		switch s.ID {
		case UserSchema:
			for _, a := range s.Attributes {
				support.UserAttributes[strings.ToLower(a.Name)] = true
			}
		case EnterpriseUserSchema:
			for _, a := range s.Attributes {
				support.EnterpriseAttributes[strings.ToLower(a.Name)] = true
			}
		case GroupSchema:
			support.HasGroupSchema = true
		}
	}

	return support
}

func (c *Client) SupportsGroups(ctx context.Context) bool {
	resourceTypes := c.discoverResourceTypes(ctx)
	for _, rt := range resourceTypes {
		if rt.Name == "Group" {
			return true
		}
	}
	return false
}

// discoverResourceTypes queries /ResourceTypes, handling both bare arrays
// and ListResponse envelopes.
func (c *Client) discoverResourceTypes(ctx context.Context) []ResourceType {
	var resourceTypes []ResourceType
	if err := c.do(ctx, http.MethodGet, "/ResourceTypes", nil, &resourceTypes); err == nil && len(resourceTypes) > 0 {
		return resourceTypes
	}
	var envelope ResourceTypeListResponse
	if err := c.do(ctx, http.MethodGet, "/ResourceTypes", nil, &envelope); err != nil {
		slog.Warn("could not query SCIM ResourceTypes", "err", err)
		return nil
	}
	return envelope.Resources
}

func (c *Client) do(ctx context.Context, method, path string, body, result interface{}) error {
	// Pre-marshal once; the reader is reset per attempt so retries send identical bytes.
	var bodyData []byte
	if body != nil {
		var err error
		bodyData, err = json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
	}

	endpoint := c.baseURL + path

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		var reqBody io.Reader
		if bodyData != nil {
			reqBody = bytes.NewReader(bodyData)
		}

		req, err := http.NewRequestWithContext(ctx, method, endpoint, reqBody)
		if err != nil {
			return fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.token)
		req.Header.Set("Content-Type", "application/scim+json")
		req.Header.Set("Accept", "application/scim+json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			if attempt < c.maxRetries {
				backoff(attempt)
				continue
			}
			return fmt.Errorf("request failed: %w", err)
		}

		respBody, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		err = readErr
		if err != nil {
			return fmt.Errorf("read response: %w", err)
		}

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			if attempt < c.maxRetries {
				wait := retryAfter(resp)
				if wait == 0 {
					wait = backoffDuration(attempt)
				}
				slog.Warn("retrying SCIM request",
					"method", method,
					"path", path,
					"status", resp.StatusCode,
					"attempt", attempt+1,
					"wait", wait,
				)
				time.Sleep(wait)
				continue
			}
		}

		if resp.StatusCode == http.StatusNotFound {
			return ErrNotFound
		}
		if resp.StatusCode == http.StatusConflict {
			return ErrConflict
		}

		if resp.StatusCode >= 400 {
			var scimErr ErrorResponse
			if json.Unmarshal(respBody, &scimErr) == nil && scimErr.Detail != "" {
				return fmt.Errorf("SCIM error %s: %s", resp.Status, scimErr.Detail)
			}
			return fmt.Errorf("SCIM error %s: %s", resp.Status, string(respBody))
		}

		// No content (204 from DELETE, etc.)
		if resp.StatusCode == http.StatusNoContent || len(respBody) == 0 {
			return nil
		}

		if result != nil {
			if err := json.Unmarshal(respBody, result); err != nil {
				return fmt.Errorf("decode response: %w", err)
			}
		}

		return nil
	}

	return fmt.Errorf("max retries exceeded for %s %s", method, path)
}

func backoff(attempt int) {
	time.Sleep(backoffDuration(attempt))
}

func backoffDuration(attempt int) time.Duration {
	return time.Duration(math.Pow(2, float64(attempt))) * time.Second
}

func retryAfter(resp *http.Response) time.Duration {
	h := resp.Header.Get("Retry-After")
	if h == "" {
		return 0
	}
	if secs, err := strconv.Atoi(h); err == nil {
		return time.Duration(secs) * time.Second
	}
	return 0
}
