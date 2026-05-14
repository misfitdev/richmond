package filter

import (
	"testing"

	admin "google.golang.org/api/admin/directory/v1"
)

func TestUsers_ExcludeOrgUnits(t *testing.T) {
	users := []*admin.User{
		{PrimaryEmail: "a@test.com", OrgUnitPath: "/Engineering"},
		{PrimaryEmail: "b@test.com", OrgUnitPath: "/Limited"},
		{PrimaryEmail: "c@test.com", OrgUnitPath: "/Limited/Temps"},
		{PrimaryEmail: "d@test.com", OrgUnitPath: "/Sales"},
		{PrimaryEmail: "e@test.com", OrgUnitPath: "/LimitedEdition"},
	}

	result := Users(users, []string{"/Limited"})

	emails := make(map[string]bool)
	for _, u := range result {
		emails[u.PrimaryEmail] = true
	}

	if len(result) != 3 {
		t.Errorf("expected 3 users, got %d", len(result))
	}
	if !emails["a@test.com"] {
		t.Error("Engineering user should be kept")
	}
	if emails["b@test.com"] {
		t.Error("/Limited user should be excluded")
	}
	if emails["c@test.com"] {
		t.Error("/Limited/Temps user should be excluded (hierarchical)")
	}
	if !emails["d@test.com"] {
		t.Error("Sales user should be kept")
	}
	if !emails["e@test.com"] {
		t.Error("/LimitedEdition should NOT be excluded (not a child of /Limited)")
	}
}

func TestUsers_NoExclusions(t *testing.T) {
	users := []*admin.User{
		{PrimaryEmail: "a@test.com"},
		{PrimaryEmail: "b@test.com"},
	}

	result := Users(users, nil)
	if len(result) != 2 {
		t.Errorf("expected 2 users, got %d", len(result))
	}
}

func TestGroups_IncludeOnly(t *testing.T) {
	groups := []*admin.Group{
		{Email: "engineering@test.com"},
		{Email: "platform-team@test.com"},
		{Email: "hr@test.com"},
	}

	result := Groups(groups, []string{"engineering@test.com", "platform-*@test.com"}, nil)

	if len(result) != 2 {
		t.Errorf("expected 2 groups, got %d", len(result))
	}
	for _, g := range result {
		if g.Email == "hr@test.com" {
			t.Error("hr group should not be included")
		}
	}
}

func TestGroups_ExcludeOnly(t *testing.T) {
	groups := []*admin.Group{
		{Email: "engineering@test.com"},
		{Email: "noreply-alerts@test.com"},
		{Email: "noreply-billing@test.com"},
	}

	result := Groups(groups, nil, []string{"noreply-*@test.com"})

	if len(result) != 1 {
		t.Errorf("expected 1 group, got %d", len(result))
	}
	if result[0].Email != "engineering@test.com" {
		t.Errorf("expected engineering, got %s", result[0].Email)
	}
}

func TestGroups_NoFilters(t *testing.T) {
	groups := []*admin.Group{
		{Email: "a@test.com"},
		{Email: "b@test.com"},
	}

	result := Groups(groups, nil, nil)
	if len(result) != 2 {
		t.Errorf("expected 2 groups, got %d", len(result))
	}
}

func TestUsers_EmptyStringInExcludes(t *testing.T) {
	users := []*admin.User{
		{PrimaryEmail: "a@test.com", OrgUnitPath: "/Engineering"},
		{PrimaryEmail: "b@test.com", OrgUnitPath: "/Sales"},
	}

	// An empty string must not cause all users to be excluded.
	result := Users(users, []string{""})
	if len(result) != 2 {
		t.Errorf("empty exclude entry should not exclude any users, got %d users", len(result))
	}
}

func TestGroups_MalformedPattern(t *testing.T) {
	groups := []*admin.Group{
		{Email: "engineering@test.com"},
		{Email: "platform@test.com"},
	}

	// A malformed pattern must be skipped (not panic, not exclude everything).
	result := Groups(groups, nil, []string{"[bad-pattern"})
	if len(result) != 2 {
		t.Errorf("malformed exclude pattern should not exclude any groups, got %d", len(result))
	}
}

func TestGroups_MalformedIncludePattern(t *testing.T) {
	groups := []*admin.Group{
		{Email: "engineering@test.com"},
	}

	// A malformed include pattern must not match anything (so no groups pass through).
	result := Groups(groups, []string{"[bad-pattern"}, nil)
	if len(result) != 0 {
		t.Errorf("malformed include pattern should match nothing, got %d groups", len(result))
	}
}
