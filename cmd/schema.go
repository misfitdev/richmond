package cmd

import (
	"context"
	"log/slog"

	"github.com/misfitdev/richmond/internal/config"
	"github.com/misfitdev/richmond/internal/scim"
)

type attrMapping struct {
	schema string // "core" or "enterprise"
	name   string // SCIM attribute name (lowercase)
}

var richmondAttrToSCIM = map[string]attrMapping{
	"external_id":   {schema: "core", name: "externalid"},
	"user_name":     {schema: "core", name: "username"},
	"active":        {schema: "core", name: "active"},
	"name":          {schema: "core", name: "name"},
	"emails":        {schema: "core", name: "emails"},
	"title":         {schema: "core", name: "title"},
	"phone_numbers": {schema: "core", name: "phonenumbers"},
	"department":    {schema: "enterprise", name: "department"},
}

// filterUnsupportedAttributes removes SCIM attributes from cfg that the
// provider does not advertise in its schema. Default attributes (external_id,
// user_name, active) are never removed.
func filterUnsupportedAttributes(cfg *config.Config, support *scim.SchemaSupport) {
	defaults := make(map[string]bool, len(config.DefaultAttributes))
	for _, a := range config.DefaultAttributes {
		defaults[a] = true
	}

	var kept []string
	for _, attr := range cfg.SCIM.Attributes {
		m, ok := richmondAttrToSCIM[attr]
		if !ok {
			kept = append(kept, attr)
			continue
		}

		if defaults[attr] {
			kept = append(kept, attr)
			continue
		}

		supported := false
		switch m.schema {
		case "core":
			supported = support.UserAttributes[m.name]
		case "enterprise":
			supported = support.EnterpriseAttributes[m.name]
		}

		if supported {
			kept = append(kept, attr)
		} else {
			slog.Warn("SCIM provider does not support attribute, excluding from sync",
				"attribute", attr,
				"scim_name", m.name,
				"schema", m.schema,
			)
		}
	}

	cfg.SCIM.Attributes = kept
}

// supportsGroups checks group support from schema discovery, falling back
// to the /ResourceTypes endpoint if discovery returned nil.
func supportsGroups(ctx context.Context, client *scim.Client, support *scim.SchemaSupport) bool {
	if support != nil {
		return support.HasGroupSchema
	}
	return client.SupportsGroups(ctx)
}
