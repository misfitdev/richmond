package mapping

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	admin "google.golang.org/api/admin/directory/v1"

	"github.com/misfitdev/richmond/internal/config"
	"github.com/misfitdev/richmond/internal/scim"
)

// Mapper translates Google Directory resources to SCIM v2 resources.
type Mapper struct {
	attrs map[string]bool
}

func New(cfg *config.Config) *Mapper {
	attrs := make(map[string]bool, len(cfg.SCIM.Attributes))
	for _, a := range cfg.SCIM.Attributes {
		attrs[a] = true
	}
	return &Mapper{attrs: attrs}
}

// MapUser converts a Google admin.User to a SCIM User.
func (m *Mapper) MapUser(u *admin.User) *scim.User {
	schemas := []string{scim.UserSchema}

	active := !u.Suspended && !u.Archived
	su := &scim.User{
		ExternalID: u.Id,
		UserName:   u.PrimaryEmail,
		Active:     scim.BoolPtr(active),
	}

	if m.attrs["name"] && u.Name != nil {
		su.Name = &scim.Name{
			GivenName:  u.Name.GivenName,
			FamilyName: u.Name.FamilyName,
		}
	}

	if m.attrs["emails"] {
		su.Emails = []scim.Email{
			{Value: u.PrimaryEmail, Type: "work", Primary: true},
		}
	}

	if m.attrs["title"] || m.attrs["department"] {
		title, dept := extractOrgFields(u)
		if m.attrs["title"] && title != "" {
			su.Title = title
		}
		if m.attrs["department"] && dept != "" {
			schemas = append(schemas, scim.EnterpriseUserSchema)
			su.EnterpriseUser = &scim.EnterpriseUser{Department: dept}
		}
	}

	if m.attrs["phone_numbers"] {
		su.PhoneNumbers = extractPhones(u)
	}

	su.Schemas = schemas
	return su
}

// MapGroup converts a Google admin.Group plus resolved SCIM member IDs to a SCIM Group.
func (m *Mapper) MapGroup(g *admin.Group, members []scim.GroupMember) *scim.Group {
	return &scim.Group{
		Schemas:     []string{scim.GroupSchema},
		ExternalID:  g.Id,
		DisplayName: g.Name,
		Members:     members,
	}
}

// GoogleUserFields returns the fields parameter for partial responses based on
// which attributes are configured for sync.
func (m *Mapper) GoogleUserFields() string {
	// Always need these base fields
	fields := []string{"id", "primaryEmail", "suspended", "archived", "orgUnitPath"}

	if m.attrs["name"] {
		fields = append(fields, "name")
	}
	if m.attrs["title"] || m.attrs["department"] {
		fields = append(fields, "organizations")
	}
	if m.attrs["phone_numbers"] {
		fields = append(fields, "phones")
	}

	inner := strings.Join(fields, ",")
	return "users(" + inner + "),nextPageToken"
}

// HashUser computes a stable hash of the SCIM user representation for change detection.
func HashUser(u *scim.User) string {
	// Zero out server-assigned fields before hashing.
	snapshot := *u
	snapshot.ID = ""
	data, _ := json.Marshal(snapshot)
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// HashGroup computes a stable hash of the SCIM group representation.
func HashGroup(g *scim.Group) string {
	snapshot := *g
	snapshot.ID = ""
	data, _ := json.Marshal(snapshot)
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func extractOrgFields(u *admin.User) (title, department string) {
	orgs, ok := u.Organizations.([]interface{})
	if !ok || len(orgs) == 0 {
		return "", ""
	}
	org, ok := orgs[0].(map[string]interface{})
	if !ok {
		return "", ""
	}
	title, _ = org["title"].(string)
	department, _ = org["department"].(string)
	return title, department
}

func extractPhones(u *admin.User) []scim.PhoneNumber {
	phones, ok := u.Phones.([]interface{})
	if !ok {
		return nil
	}
	var result []scim.PhoneNumber
	for _, p := range phones {
		phone, ok := p.(map[string]interface{})
		if !ok {
			continue
		}
		value, _ := phone["value"].(string)
		if value == "" {
			continue
		}
		phoneType, _ := phone["type"].(string)
		result = append(result, scim.PhoneNumber{Value: value, Type: phoneType})
	}
	return result
}
