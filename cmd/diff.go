package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
	admin "google.golang.org/api/admin/directory/v1"

	"github.com/misfitdev/richmond/internal/config"
	"github.com/misfitdev/richmond/internal/google"
	"github.com/misfitdev/richmond/internal/mapping"
	"github.com/misfitdev/richmond/internal/reconcile"
	"github.com/misfitdev/richmond/internal/scim"
	"github.com/misfitdev/richmond/internal/state"
)

var diffCmd = &cobra.Command{
	Use:   "diff",
	Short: "Show what would change without making changes",
	Long: `Fetches users and groups from Google Workspace, compares them to
the previous sync state, and displays what creates, updates, and
deactivations would occur — without actually making any changes.`,
	RunE: runDiff,
}

func init() {
	rootCmd.AddCommand(diffCmd)
}

func runDiff(cmd *cobra.Command, _ []string) error {
	cfg, err := config.Load(configFile)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	cfg.Sync.DryRun = true

	ctx := context.Background()
	mapper := mapping.New(cfg)

	googleClient, err := google.NewClient(ctx, cfg.Google)
	if err != nil {
		return fmt.Errorf("init google client: %w", err)
	}

	scimClient := scim.NewClient(cfg.SCIM.Endpoint, cfg.SCIM.BearerToken)

	store, err := state.NewStore(cfg.Sync.StateFile)
	if err != nil {
		return fmt.Errorf("init state store: %w", err)
	}

	prev, err := store.Load()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	fields := mapper.GoogleUserFields()
	users, err := googleClient.ListUsers(ctx, fields)
	if err != nil {
		return err
	}

	groups, err := googleClient.ListGroups(ctx)
	if err != nil {
		return err
	}

	members := make(map[string][]*admin.Member)
	for _, g := range groups {
		m, mErr := googleClient.ListGroupMembers(ctx, g.Id, true)
		if mErr != nil {
			slog.Error("failed to list group members", "group", g.Name, "err", mErr)
			continue
		}
		members[g.Id] = m
	}

	rec := reconcile.New(scimClient, mapper, true)
	result, err := rec.Reconcile(ctx, users, groups, members, prev)
	if err != nil {
		return err
	}

	printDiff(result)
	return nil
}

func printDiff(result *reconcile.Result) {
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "ACTION\tRESOURCE\tIDENTIFIER\tGOOGLE ID")
	_, _ = fmt.Fprintln(w, "------\t--------\t----------\t---------")

	for _, op := range result.Ops {
		if op.Type == reconcile.OpSkip {
			continue
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", op.Type, op.Resource, op.Email, op.GoogleID)
	}
	_ = w.Flush()

	s := result.Stats
	fmt.Printf("\nUsers:  %d create, %d update, %d deactivate, %d unchanged\n",
		s.UsersCreated, s.UsersUpdated, s.UsersDeactivated, s.UsersSkipped)
	fmt.Printf("Groups: %d create, %d update, %d delete, %d unchanged\n",
		s.GroupsCreated, s.GroupsUpdated, s.GroupsDeleted, s.GroupsSkipped)
}
