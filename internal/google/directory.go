package google

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"golang.org/x/oauth2/google"
	admin "google.golang.org/api/admin/directory/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"

	"github.com/misfitdev/richmond/internal/config"
)

type Client struct {
	svc    *admin.Service
	config config.GoogleConfig
}

func NewClient(ctx context.Context, cfg config.GoogleConfig) (*Client, error) {
	var opts []option.ClientOption
	if cfg.CredentialsFile != "" {
		data, err := os.ReadFile(cfg.CredentialsFile) //nolint:gosec // path from trusted config
		if err != nil {
			return nil, fmt.Errorf("read credentials file: %w", err)
		}
		jwtConfig, err := google.JWTConfigFromJSON(data,
			admin.AdminDirectoryUserReadonlyScope,
			admin.AdminDirectoryGroupReadonlyScope,
			admin.AdminDirectoryGroupMemberReadonlyScope,
		)
		if err != nil {
			return nil, fmt.Errorf("parse service account credentials: %w", err)
		}
		jwtConfig.Subject = cfg.AdminEmail
		opts = append(opts, option.WithHTTPClient(jwtConfig.Client(ctx)))
	}

	svc, err := admin.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create admin service: %w", err)
	}

	return &Client{svc: svc, config: cfg}, nil
}

// ListUsers retrieves all users from the Google Workspace directory.
// The fields parameter controls which attributes are returned (partial response).
func (c *Client) ListUsers(ctx context.Context, fields string) ([]*admin.User, error) {
	var users []*admin.User

	call := c.svc.Users.List().
		Customer(c.config.CustomerID).
		MaxResults(500).
		OrderBy("email")

	if c.config.Domain != "" {
		call = call.Domain(c.config.Domain)
	}
	if c.config.UserQuery != "" {
		call = call.Query(c.config.UserQuery)
	}
	if fields != "" {
		call = call.Fields(googleapi.Field(fields))
	}

	err := call.Pages(ctx, func(resp *admin.Users) error {
		users = append(users, resp.Users...)
		slog.Debug("fetched users page", "count", len(resp.Users), "total", len(users))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}

	slog.Info("fetched all users from Google", "count", len(users))
	return users, nil
}

// ListGroups retrieves all groups from the Google Workspace directory.
func (c *Client) ListGroups(ctx context.Context) ([]*admin.Group, error) {
	var groups []*admin.Group

	call := c.svc.Groups.List().
		Customer(c.config.CustomerID).
		MaxResults(200).
		OrderBy("email")

	if c.config.Domain != "" {
		call = call.Domain(c.config.Domain)
	}

	err := call.Pages(ctx, func(resp *admin.Groups) error {
		groups = append(groups, resp.Groups...)
		slog.Debug("fetched groups page", "count", len(resp.Groups), "total", len(groups))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list groups: %w", err)
	}

	slog.Info("fetched all groups from Google", "count", len(groups))
	return groups, nil
}

// ListGroupMembers retrieves all members of a group.
// When includeDerived is true, nested group memberships are flattened.
func (c *Client) ListGroupMembers(ctx context.Context, groupKey string, includeDerived bool) ([]*admin.Member, error) {
	var members []*admin.Member

	call := c.svc.Members.List(groupKey).MaxResults(200)
	if includeDerived {
		call = call.IncludeDerivedMembership(true)
	}

	err := call.Pages(ctx, func(resp *admin.Members) error {
		members = append(members, resp.Members...)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list members for group %s: %w", groupKey, err)
	}

	return members, nil
}
