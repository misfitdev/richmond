package mapping

import (
	"testing"

	admin "google.golang.org/api/admin/directory/v1"

	"github.com/misfitdev/richmond/internal/config"
	"github.com/misfitdev/richmond/internal/scim"
)

func TestMapUser_AllAttributes(t *testing.T) {
	cfg := &config.Config{
		SCIM: config.SCIMConfig{
			Attributes: []string{
				"external_id", "user_name", "active",
				"name", "emails", "title", "department", "phone_numbers",
			},
		},
	}
	m := New(cfg)

	gu := &admin.User{
		Id:           "goog-123",
		PrimaryEmail: "jane@example.com",
		Name: &admin.UserName{
			GivenName:  "Jane",
			FamilyName: "Doe",
		},
		Suspended: false,
		Organizations: []interface{}{
			map[string]interface{}{
				"title":      "Staff Engineer",
				"department": "Platform",
			},
		},
		Phones: []interface{}{
			map[string]interface{}{"value": "+15551234567", "type": "work"},
		},
	}

	su := m.MapUser(gu)

	if su.ExternalID != "goog-123" {
		t.Errorf("ExternalID = %q, want goog-123", su.ExternalID)
	}
	if su.UserName != "jane@example.com" {
		t.Errorf("UserName = %q, want jane@example.com", su.UserName)
	}
	if su.Active == nil || *su.Active != true {
		t.Error("Active should be true")
	}
	if su.Name == nil || su.Name.GivenName != "Jane" || su.Name.FamilyName != "Doe" {
		t.Errorf("Name = %+v, want Jane Doe", su.Name)
	}
	if len(su.Emails) != 1 || su.Emails[0].Value != "jane@example.com" {
		t.Errorf("Emails = %+v, want jane@example.com", su.Emails)
	}
	if su.Title != "Staff Engineer" {
		t.Errorf("Title = %q, want Staff Engineer", su.Title)
	}
	if su.EnterpriseUser == nil || su.EnterpriseUser.Department != "Platform" {
		t.Errorf("Department = %+v, want Platform", su.EnterpriseUser)
	}
	if len(su.PhoneNumbers) != 1 || su.PhoneNumbers[0].Value != "+15551234567" {
		t.Errorf("PhoneNumbers = %+v, want +15551234567", su.PhoneNumbers)
	}

	// Should have enterprise schema
	hasEnterprise := false
	for _, s := range su.Schemas {
		if s == scim.EnterpriseUserSchema {
			hasEnterprise = true
		}
	}
	if !hasEnterprise {
		t.Error("expected enterprise user schema when department is set")
	}
}

func TestMapUser_MinimalAttributes(t *testing.T) {
	cfg := &config.Config{
		SCIM: config.SCIMConfig{
			Attributes: []string{"external_id", "user_name", "active"},
		},
	}
	m := New(cfg)

	gu := &admin.User{
		Id:           "goog-456",
		PrimaryEmail: "minimal@example.com",
		Suspended:    true,
	}

	su := m.MapUser(gu)

	if su.ExternalID != "goog-456" {
		t.Errorf("ExternalID = %q, want goog-456", su.ExternalID)
	}
	if su.Active == nil || *su.Active != false {
		t.Error("Active should be false for suspended user")
	}
	if su.Name != nil {
		t.Error("Name should be nil for minimal attributes")
	}
	if len(su.Emails) != 0 {
		t.Error("Emails should be empty for minimal attributes")
	}
	if su.Title != "" {
		t.Error("Title should be empty")
	}
	if su.EnterpriseUser != nil {
		t.Error("EnterpriseUser should be nil")
	}
}

func TestMapUser_ArchivedIsInactive(t *testing.T) {
	cfg := &config.Config{
		SCIM: config.SCIMConfig{
			Attributes: []string{"external_id", "user_name", "active"},
		},
	}
	m := New(cfg)

	tests := []struct {
		name       string
		suspended  bool
		archived   bool
		wantActive bool
	}{
		{"neither", false, false, true},
		{"suspended only", true, false, false},
		{"archived only", false, true, false},
		{"both", true, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gu := &admin.User{
				Id:           "g1",
				PrimaryEmail: "test@example.com",
				Suspended:    tt.suspended,
				Archived:     tt.archived,
			}
			su := m.MapUser(gu)
			if su.Active == nil {
				t.Fatal("Active is nil")
			}
			if *su.Active != tt.wantActive {
				t.Errorf("Active = %v, want %v", *su.Active, tt.wantActive)
			}
		})
	}
}

func TestGoogleUserFields(t *testing.T) {
	tests := []struct {
		name   string
		attrs  []string
		expect string
	}{
		{
			name:   "minimal",
			attrs:  []string{"external_id", "user_name", "active"},
			expect: "users(id,primaryEmail,suspended,archived,orgUnitPath),nextPageToken",
		},
		{
			name:   "with name",
			attrs:  []string{"external_id", "user_name", "active", "name"},
			expect: "users(id,primaryEmail,suspended,archived,orgUnitPath,name),nextPageToken",
		},
		{
			name:   "with org fields",
			attrs:  []string{"external_id", "user_name", "active", "title", "department"},
			expect: "users(id,primaryEmail,suspended,archived,orgUnitPath,organizations),nextPageToken",
		},
		{
			name:   "all",
			attrs:  []string{"external_id", "user_name", "active", "name", "title", "phone_numbers"},
			expect: "users(id,primaryEmail,suspended,archived,orgUnitPath,name,organizations,phones),nextPageToken",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{SCIM: config.SCIMConfig{Attributes: tt.attrs}}
			m := New(cfg)
			got := m.GoogleUserFields()
			if got != tt.expect {
				t.Errorf("GoogleUserFields() = %q, want %q", got, tt.expect)
			}
		})
	}
}

func TestHashUser_Stable(t *testing.T) {
	u := &scim.User{
		Schemas:    []string{scim.UserSchema},
		ExternalID: "123",
		UserName:   "test@example.com",
		Active:     scim.BoolPtr(true),
	}

	h1 := HashUser(u)
	h2 := HashUser(u)

	if h1 != h2 {
		t.Errorf("hash not stable: %s != %s", h1, h2)
	}
	if len(h1) != 64 {
		t.Errorf("expected SHA-256 hex string (64 chars), got %d chars", len(h1))
	}
}

func TestHashUser_DiffersOnChange(t *testing.T) {
	u1 := &scim.User{
		Schemas:    []string{scim.UserSchema},
		ExternalID: "123",
		UserName:   "test@example.com",
		Active:     scim.BoolPtr(true),
	}
	u2 := &scim.User{
		Schemas:    []string{scim.UserSchema},
		ExternalID: "123",
		UserName:   "test@example.com",
		Active:     scim.BoolPtr(false),
	}

	if HashUser(u1) == HashUser(u2) {
		t.Error("hash should differ when active changes")
	}
}

func TestHashUser_IgnoresServerID(t *testing.T) {
	u1 := &scim.User{
		Schemas:    []string{scim.UserSchema},
		ID:         "server-assigned-1",
		ExternalID: "123",
		UserName:   "test@example.com",
	}
	u2 := &scim.User{
		Schemas:    []string{scim.UserSchema},
		ID:         "server-assigned-2",
		ExternalID: "123",
		UserName:   "test@example.com",
	}

	if HashUser(u1) != HashUser(u2) {
		t.Error("hash should be the same regardless of server-assigned ID")
	}
}
