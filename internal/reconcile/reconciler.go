package reconcile

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	admin "google.golang.org/api/admin/directory/v1"

	"github.com/misfitdev/richmond/internal/mapping"
	"github.com/misfitdev/richmond/internal/scim"
	"github.com/misfitdev/richmond/internal/state"
)

type OpType string

const (
	OpCreate     OpType = "create"
	OpUpdate     OpType = "update"
	OpDeactivate OpType = "deactivate"
	OpDelete     OpType = "delete"
	OpSkip       OpType = "skip"
)

type Op struct {
	Type     OpType
	Resource string // "user" or "group"
	GoogleID string
	Email    string
	SCIMID   string
}

type Result struct {
	State *state.SyncState
	Ops   []Op
	Stats Stats
}

type Stats struct {
	UsersCreated     int
	UsersUpdated     int
	UsersDeactivated int
	UsersSkipped     int
	GroupsCreated    int
	GroupsUpdated    int
	GroupsDeleted    int
	GroupsSkipped    int
	Errors           int
}

type Reconciler struct {
	scimClient    *scim.Client
	mapper        *mapping.Mapper
	dryRun        bool
	adoptExisting bool
}

func New(scimClient *scim.Client, mapper *mapping.Mapper, dryRun, adoptExisting bool) *Reconciler {
	return &Reconciler{
		scimClient:    scimClient,
		mapper:        mapper,
		dryRun:        dryRun,
		adoptExisting: adoptExisting,
	}
}

func (r *Reconciler) Reconcile(ctx context.Context, users []*admin.User, groups []*admin.Group, members map[string][]*admin.Member, prev *state.SyncState) (*Result, error) {
	result := &Result{
		State: &state.SyncState{
			LastSync: time.Now().UTC(),
			Users:    make(map[string]state.UserState),
			Groups:   make(map[string]state.GroupState),
		},
	}

	r.reconcileUsers(ctx, users, prev, result)
	r.reconcileGroups(ctx, groups, members, prev, result)
	r.deactivateRemovedUsers(ctx, users, prev, result)
	r.deleteRemovedGroups(ctx, groups, prev, result)

	return result, nil
}

func (r *Reconciler) reconcileUsers(ctx context.Context, users []*admin.User, prev *state.SyncState, result *Result) {
	for _, gu := range users {
		su := r.mapper.MapUser(gu)
		hash := mapping.HashUser(su)

		prevUser, exists := prev.Users[gu.Id]

		switch {
		case !exists:
			if err := r.createUser(ctx, gu, su, hash, result); err != nil {
				slog.Error("failed to create user", "email", gu.PrimaryEmail, "err", err)
				result.Stats.Errors++
				continue
			}

		case prevUser.Hash != hash || prevUser.LastError != "":
			if prevUser.LastError != "" && prevUser.Hash == hash {
				slog.Info("retrying previously failed operation", "email", gu.PrimaryEmail, "previous_error", prevUser.LastError)
			}
			if err := r.updateUser(ctx, gu, su, hash, prevUser, result); err != nil {
				slog.Error("failed to update user", "email", gu.PrimaryEmail, "err", err)
				result.Stats.Errors++
				prevUser.LastError = err.Error()
				result.State.Users[gu.Id] = prevUser
				continue
			}

		default:
			result.State.Users[gu.Id] = prevUser
			result.Stats.UsersSkipped++
			result.Ops = append(result.Ops, Op{Type: OpSkip, Resource: "user", GoogleID: gu.Id, Email: gu.PrimaryEmail})
		}
	}
}

func (r *Reconciler) createUser(ctx context.Context, gu *admin.User, su *scim.User, hash string, result *Result) error {
	log := slog.With("email", gu.PrimaryEmail, "google_id", gu.Id)

	// Check if user already exists in SCIM (from a partial previous sync)
	existing, err := r.scimClient.FindUserByExternalID(ctx, gu.Id)
	if err != nil {
		if !r.dryRun {
			return fmt.Errorf("lookup existing user: %w", err)
		}
		log.Warn("skipping externalId lookup error in dry-run", "err", err)
	}

	if existing != nil {
		log.Info("user already exists in SCIM, updating", "scim_id", existing.ID)
		return r.updateUser(ctx, gu, su, hash, state.UserState{SCIMID: existing.ID, Active: true}, result)
	}

	// Fall back to userName lookup for users provisioned outside Richmond (e.g. JIT)
	if r.adoptExisting {
		existing, err = r.scimClient.FindUserByUserName(ctx, gu.PrimaryEmail)
		if err != nil {
			if !r.dryRun {
				return fmt.Errorf("lookup existing user by userName: %w", err)
			}
			log.Warn("skipping userName lookup error in dry-run", "err", err)
		}
		if existing != nil {
			log.Info("existing account found, patching", "scim_id", existing.ID)
			return r.updateUser(ctx, gu, su, hash, state.UserState{SCIMID: existing.ID, Active: true}, result)
		}
	}

	op := Op{Type: OpCreate, Resource: "user", GoogleID: gu.Id, Email: gu.PrimaryEmail}

	if r.dryRun {
		log.Info("would create user")
		result.Ops = append(result.Ops, op)
		result.Stats.UsersCreated++
		result.State.Users[gu.Id] = state.UserState{Hash: hash, Active: su.Active != nil && *su.Active, Email: gu.PrimaryEmail}
		return nil
	}

	created, err := r.scimClient.CreateUser(ctx, su)
	if err != nil {
		if errors.Is(err, scim.ErrConflict) {
			if !r.adoptExisting {
				log.Warn("account already exists, skipping (adopt_existing is disabled)")
				result.Stats.UsersSkipped++
				result.Ops = append(result.Ops, Op{Type: OpSkip, Resource: "user", GoogleID: gu.Id, Email: gu.PrimaryEmail})
				return nil
			}
			// Race: user created between our lookup and CreateUser call
			existing, lookupErr := r.scimClient.FindUserByUserName(ctx, gu.PrimaryEmail)
			if lookupErr == nil && existing != nil {
				log.Info("existing account found after conflict, patching", "scim_id", existing.ID)
				return r.updateUser(ctx, gu, su, hash, state.UserState{SCIMID: existing.ID, Active: true}, result)
			}
		}
		return err
	}

	log.Info("created user", "scim_id", created.ID)
	op.SCIMID = created.ID
	result.Ops = append(result.Ops, op)
	result.Stats.UsersCreated++
	result.State.Users[gu.Id] = state.UserState{
		SCIMID: created.ID,
		Hash:   hash,
		Active: su.Active != nil && *su.Active,
		Email:  gu.PrimaryEmail,
	}
	return nil
}

func (r *Reconciler) updateUser(ctx context.Context, gu *admin.User, su *scim.User, hash string, prevUser state.UserState, result *Result) error {
	log := slog.With("email", gu.PrimaryEmail, "scim_id", prevUser.SCIMID)
	op := Op{Type: OpUpdate, Resource: "user", GoogleID: gu.Id, Email: gu.PrimaryEmail, SCIMID: prevUser.SCIMID}

	patch := buildUserPatch(su)

	if r.dryRun {
		log.Info("would update user")
		result.Ops = append(result.Ops, op)
		result.Stats.UsersUpdated++
		result.State.Users[gu.Id] = state.UserState{SCIMID: prevUser.SCIMID, Hash: hash, Active: su.Active != nil && *su.Active, Email: gu.PrimaryEmail}
		return nil
	}

	if err := r.scimClient.UpdateUser(ctx, prevUser.SCIMID, patch); err != nil {
		return err
	}

	log.Info("updated user")
	result.Ops = append(result.Ops, op)
	result.Stats.UsersUpdated++
	result.State.Users[gu.Id] = state.UserState{
		SCIMID: prevUser.SCIMID,
		Hash:   hash,
		Active: su.Active != nil && *su.Active,
		Email:  gu.PrimaryEmail,
	}
	return nil
}

func (r *Reconciler) deactivateRemovedUsers(ctx context.Context, users []*admin.User, prev *state.SyncState, result *Result) {
	currentIDs := make(map[string]bool, len(users))
	for _, u := range users {
		currentIDs[u.Id] = true
	}

	for googleID, prevUser := range prev.Users {
		if currentIDs[googleID] || !prevUser.Active {
			continue
		}

		log := slog.With("email", prevUser.Email, "scim_id", prevUser.SCIMID, "google_id", googleID)
		op := Op{Type: OpDeactivate, Resource: "user", GoogleID: googleID, Email: prevUser.Email, SCIMID: prevUser.SCIMID}

		if r.dryRun {
			log.Info("would deactivate user")
			result.Ops = append(result.Ops, op)
			result.Stats.UsersDeactivated++
			continue
		}

		patch := scim.NewPatchOp(scim.Operation{
			Op:    "replace",
			Path:  "active",
			Value: false,
		})
		if err := r.scimClient.UpdateUser(ctx, prevUser.SCIMID, patch); err != nil {
			if errors.Is(err, scim.ErrNotFound) {
				log.Info("user already removed from SCIM")
				result.Ops = append(result.Ops, op)
				result.Stats.UsersDeactivated++
				continue
			}
			slog.Error("failed to deactivate user", "email", prevUser.Email, "err", err)
			result.Stats.Errors++
			prevUser.LastError = err.Error()
			result.State.Users[googleID] = prevUser
			continue
		}

		log.Info("deactivated user")
		result.Ops = append(result.Ops, op)
		result.Stats.UsersDeactivated++
	}
}

func (r *Reconciler) reconcileGroups(ctx context.Context, groups []*admin.Group, members map[string][]*admin.Member, prev *state.SyncState, result *Result) {
	// Build an email→SCIM-ID index once so resolveGroupMembers is O(1) per lookup.
	emailToSCIMID := make(map[string]string, len(result.State.Users))
	for _, us := range result.State.Users {
		if us.Email != "" && us.SCIMID != "" {
			emailToSCIMID[us.Email] = us.SCIMID
		}
	}

	for _, gg := range groups {
		groupMembers := r.resolveGroupMembers(gg.Id, members, emailToSCIMID)
		sg := r.mapper.MapGroup(gg, groupMembers)
		hash := mapping.HashGroup(sg)

		prevGroup, exists := prev.Groups[gg.Id]

		switch {
		case !exists:
			if err := r.createGroup(ctx, gg, sg, hash, result); err != nil {
				slog.Error("failed to create group", "name", gg.Name, "err", err)
				result.Stats.Errors++
				continue
			}

		case prevGroup.Hash != hash || prevGroup.LastError != "":
			if prevGroup.LastError != "" && prevGroup.Hash == hash {
				slog.Info("retrying previously failed group operation", "group", gg.Name, "previous_error", prevGroup.LastError)
			}
			if err := r.updateGroup(ctx, gg, sg, hash, prevGroup, result); err != nil {
				slog.Error("failed to update group", "name", gg.Name, "err", err)
				result.Stats.Errors++
				prevGroup.LastError = err.Error()
				result.State.Groups[gg.Id] = prevGroup
				continue
			}

		default:
			result.State.Groups[gg.Id] = prevGroup
			result.Stats.GroupsSkipped++
		}
	}
}

func (r *Reconciler) resolveGroupMembers(groupID string, allMembers map[string][]*admin.Member, emailToSCIMID map[string]string) []scim.GroupMember {
	gMembers := allMembers[groupID]
	var resolved []scim.GroupMember
	for _, m := range gMembers {
		if m.Type != "USER" {
			continue
		}
		if scimID, ok := emailToSCIMID[m.Email]; ok {
			resolved = append(resolved, scim.GroupMember{
				Value:   scimID,
				Display: m.Email,
			})
		}
	}
	return resolved
}

func (r *Reconciler) createGroup(ctx context.Context, gg *admin.Group, sg *scim.Group, hash string, result *Result) error {
	log := slog.With("group", gg.Name, "google_id", gg.Id)
	op := Op{Type: OpCreate, Resource: "group", GoogleID: gg.Id, Email: gg.Email}

	// Check if group already exists
	existing, err := r.scimClient.FindGroupByExternalID(ctx, gg.Id)
	if err != nil && !r.dryRun {
		return fmt.Errorf("lookup existing group: %w", err)
	}
	if existing != nil {
		log.Info("group already exists in SCIM, updating", "scim_id", existing.ID)
		return r.updateGroup(ctx, gg, sg, hash, state.GroupState{SCIMID: existing.ID}, result)
	}

	if r.dryRun {
		log.Info("would create group")
		result.Ops = append(result.Ops, op)
		result.Stats.GroupsCreated++
		result.State.Groups[gg.Id] = state.GroupState{Hash: hash, Name: gg.Name}
		return nil
	}

	created, err := r.scimClient.CreateGroup(ctx, sg)
	if err != nil {
		return err
	}

	log.Info("created group", "scim_id", created.ID)
	op.SCIMID = created.ID
	result.Ops = append(result.Ops, op)
	result.Stats.GroupsCreated++
	result.State.Groups[gg.Id] = state.GroupState{
		SCIMID: created.ID,
		Hash:   hash,
		Name:   gg.Name,
	}
	return nil
}

func (r *Reconciler) updateGroup(ctx context.Context, gg *admin.Group, sg *scim.Group, hash string, prevGroup state.GroupState, result *Result) error {
	log := slog.With("group", gg.Name, "scim_id", prevGroup.SCIMID)
	op := Op{Type: OpUpdate, Resource: "group", GoogleID: gg.Id, Email: gg.Email, SCIMID: prevGroup.SCIMID}

	patch := buildGroupPatch(sg)

	if r.dryRun {
		log.Info("would update group")
		result.Ops = append(result.Ops, op)
		result.Stats.GroupsUpdated++
		result.State.Groups[gg.Id] = state.GroupState{SCIMID: prevGroup.SCIMID, Hash: hash, Name: gg.Name}
		return nil
	}

	if err := r.scimClient.UpdateGroup(ctx, prevGroup.SCIMID, patch); err != nil {
		return err
	}

	log.Info("updated group")
	result.Ops = append(result.Ops, op)
	result.Stats.GroupsUpdated++
	result.State.Groups[gg.Id] = state.GroupState{
		SCIMID: prevGroup.SCIMID,
		Hash:   hash,
		Name:   gg.Name,
	}
	return nil
}

func (r *Reconciler) deleteRemovedGroups(ctx context.Context, groups []*admin.Group, prev *state.SyncState, result *Result) {
	currentIDs := make(map[string]bool, len(groups))
	for _, g := range groups {
		currentIDs[g.Id] = true
	}

	for googleID, prevGroup := range prev.Groups {
		if currentIDs[googleID] {
			continue
		}

		log := slog.With("group", prevGroup.Name, "scim_id", prevGroup.SCIMID, "google_id", googleID)
		op := Op{Type: OpDelete, Resource: "group", GoogleID: googleID, Email: "", SCIMID: prevGroup.SCIMID}

		if r.dryRun {
			log.Info("would delete group")
			result.Ops = append(result.Ops, op)
			result.Stats.GroupsDeleted++
			continue
		}

		if err := r.scimClient.DeleteGroup(ctx, prevGroup.SCIMID); err != nil {
			if errors.Is(err, scim.ErrNotFound) {
				log.Info("group already removed from SCIM")
				result.Ops = append(result.Ops, op)
				result.Stats.GroupsDeleted++
				continue
			}
			slog.Error("failed to delete group", "group", prevGroup.Name, "err", err)
			result.Stats.Errors++
			prevGroup.LastError = err.Error()
			result.State.Groups[googleID] = prevGroup
			continue
		}

		log.Info("deleted group")
		result.Ops = append(result.Ops, op)
		result.Stats.GroupsDeleted++
	}
}

func buildUserPatch(su *scim.User) *scim.PatchOp {
	var ops []scim.Operation

	ops = append(ops, scim.Operation{Op: "replace", Path: "externalId", Value: su.ExternalID})
	ops = append(ops, scim.Operation{Op: "replace", Path: "userName", Value: su.UserName})
	ops = append(ops, scim.Operation{Op: "replace", Path: "active", Value: su.Active})

	if su.Name != nil {
		ops = append(ops, scim.Operation{Op: "replace", Path: "name.givenName", Value: su.Name.GivenName})
		ops = append(ops, scim.Operation{Op: "replace", Path: "name.familyName", Value: su.Name.FamilyName})
	}
	if len(su.Emails) > 0 {
		ops = append(ops, scim.Operation{Op: "replace", Path: "emails", Value: su.Emails})
	}
	if su.Title != "" {
		ops = append(ops, scim.Operation{Op: "replace", Path: "title", Value: su.Title})
	}
	if su.EnterpriseUser != nil {
		ops = append(ops, scim.Operation{Op: "replace", Path: "urn:ietf:params:scim:schemas:extension:enterprise:2.0:User:department", Value: su.EnterpriseUser.Department})
	}
	if len(su.PhoneNumbers) > 0 {
		ops = append(ops, scim.Operation{Op: "replace", Path: "phoneNumbers", Value: su.PhoneNumbers})
	}

	return scim.NewPatchOp(ops...)
}

func buildGroupPatch(sg *scim.Group) *scim.PatchOp {
	return scim.NewPatchOp(
		scim.Operation{Op: "replace", Path: "displayName", Value: sg.DisplayName},
		scim.Operation{Op: "replace", Path: "members", Value: sg.Members},
	)
}
