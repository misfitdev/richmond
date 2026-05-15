package cmd

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/spf13/cobra"
	admin "google.golang.org/api/admin/directory/v1"

	"github.com/misfitdev/richmond/internal/config"
	"github.com/misfitdev/richmond/internal/filter"
	"github.com/misfitdev/richmond/internal/google"
	"github.com/misfitdev/richmond/internal/mapping"
	"github.com/misfitdev/richmond/internal/reconcile"
	"github.com/misfitdev/richmond/internal/scim"
	"github.com/misfitdev/richmond/internal/state"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Run a full or incremental sync",
	Long: `Fetches users and groups from Google Workspace, compares them to
the previous sync state, and pushes creates, updates, and deactivations
to the configured SCIM v2 endpoint.`,
	RunE: runSync,
}

func init() {
	syncCmd.Flags().Bool("dry-run", false, "show what would change without making changes")
	rootCmd.AddCommand(syncCmd)
}

func runSync(cmd *cobra.Command, _ []string) error {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}

	ctx := context.Background()

	return doSync(ctx, cfg)
}

func doSync(ctx context.Context, cfg *config.Config) error {
	mapper := mapping.New(cfg)

	// Google Directory client
	googleClient, err := google.NewClient(ctx, cfg.Google)
	if err != nil {
		return fmt.Errorf("init google client: %w", err)
	}

	// SCIM client
	scimClient := scim.NewClient(cfg.SCIM.Endpoint, cfg.SCIM.BearerToken)

	// State store
	store, err := state.NewStore(cfg.Sync.StateFile)
	if err != nil {
		return fmt.Errorf("init state store: %w", err)
	}

	prev, err := store.Load()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}
	slog.Info("loaded previous state", "users", len(prev.Users), "groups", len(prev.Groups))

	// Fetch and filter Google data
	fields := mapper.GoogleUserFields()
	users, err := googleClient.ListUsers(ctx, fields)
	if err != nil {
		return err
	}
	users = filter.Users(users, cfg.Google.ExcludeOrgUnits)

	// Group sync: check config and auto-detect endpoint support
	var groups []*admin.Group
	var members map[string][]*admin.Member
	if *cfg.Sync.SyncGroups {
		if !scimClient.SupportsGroups(ctx) {
			slog.Warn("SCIM endpoint does not advertise Group support, skipping group sync")
		} else {
			groups, err = googleClient.ListGroups(ctx)
			if err != nil {
				return err
			}
			groups = filter.Groups(groups, cfg.Google.IncludeGroups, cfg.Google.ExcludeGroups)

			derived := *cfg.Google.IncludeDerivedMembership
			members = make(map[string][]*admin.Member)
			for _, g := range groups {
				m, mErr := googleClient.ListGroupMembers(ctx, g.Id, derived)
				if mErr != nil {
					slog.Error("failed to list group members", "group", g.Name, "err", mErr)
					continue
				}
				members[g.Id] = m
			}
		}
	} else {
		slog.Info("group sync disabled by config")
	}

	// Reconcile
	rec := reconcile.New(scimClient, mapper, cfg.Sync.DryRun, *cfg.Sync.AdoptExisting)
	result, err := rec.Reconcile(ctx, users, groups, members, prev)
	if err != nil {
		return err
	}

	// Save state
	if !cfg.Sync.DryRun {
		if err := store.Save(result.State); err != nil {
			return fmt.Errorf("save state: %w", err)
		}
		slog.Info("saved sync state")
	}

	return logSummary(result.Stats, cfg.Sync.DryRun)
}

func loadConfig(cmd *cobra.Command) (*config.Config, error) {
	dryRun, _ := cmd.Flags().GetBool("dry-run")

	cfg, err := config.Load(configFile)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	if dryRun {
		cfg.Sync.DryRun = true
	}
	return cfg, nil
}

func logSummary(stats reconcile.Stats, dryRun bool) error {
	prefix := ""
	if dryRun {
		prefix = "[dry-run] "
	}

	logger := slog.With("dry_run", dryRun)
	logger.Info(prefix+"sync complete",
		"users_created", stats.UsersCreated,
		"users_updated", stats.UsersUpdated,
		"users_deactivated", stats.UsersDeactivated,
		"users_skipped", stats.UsersSkipped,
		"groups_created", stats.GroupsCreated,
		"groups_updated", stats.GroupsUpdated,
		"groups_deleted", stats.GroupsDeleted,
		"groups_skipped", stats.GroupsSkipped,
		"errors", stats.Errors,
	)

	if stats.Errors > 0 {
		return fmt.Errorf("sync completed with %d error(s)", stats.Errors)
	}
	return nil
}
