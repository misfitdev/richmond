package cmd

import (
	"testing"

	"github.com/misfitdev/richmond/internal/config"
	"github.com/misfitdev/richmond/internal/scim"
)

func TestFilterUnsupportedAttributes_AllSupported(t *testing.T) {
	cfg := &config.Config{
		SCIM: config.SCIMConfig{
			Attributes: []string{"external_id", "user_name", "active", "name", "emails", "title", "phone_numbers"},
		},
	}
	support := &scim.SchemaSupport{
		UserAttributes:       map[string]bool{"externalid": true, "username": true, "active": true, "name": true, "emails": true, "title": true, "phonenumbers": true},
		EnterpriseAttributes: map[string]bool{},
	}

	filterUnsupportedAttributes(cfg, support)

	if len(cfg.SCIM.Attributes) != 7 {
		t.Errorf("expected 7 attributes, got %d: %v", len(cfg.SCIM.Attributes), cfg.SCIM.Attributes)
	}
}

func TestFilterUnsupportedAttributes_NoEnterprise(t *testing.T) {
	cfg := &config.Config{
		SCIM: config.SCIMConfig{
			Attributes: []string{"external_id", "user_name", "active", "name", "emails", "title", "department", "phone_numbers"},
		},
	}
	support := &scim.SchemaSupport{
		UserAttributes:       map[string]bool{"externalid": true, "username": true, "active": true, "name": true, "emails": true, "title": true, "phonenumbers": true},
		EnterpriseAttributes: map[string]bool{},
	}

	filterUnsupportedAttributes(cfg, support)

	for _, a := range cfg.SCIM.Attributes {
		if a == "department" {
			t.Error("department should have been filtered out")
		}
	}
	if len(cfg.SCIM.Attributes) != 7 {
		t.Errorf("expected 7 attributes, got %d: %v", len(cfg.SCIM.Attributes), cfg.SCIM.Attributes)
	}
}

func TestFilterUnsupportedAttributes_DefaultsNeverRemoved(t *testing.T) {
	cfg := &config.Config{
		SCIM: config.SCIMConfig{
			Attributes: []string{"external_id", "user_name", "active"},
		},
	}
	// Empty schema — no attributes advertised
	support := &scim.SchemaSupport{
		UserAttributes:       map[string]bool{},
		EnterpriseAttributes: map[string]bool{},
	}

	filterUnsupportedAttributes(cfg, support)

	if len(cfg.SCIM.Attributes) != 3 {
		t.Errorf("expected 3 default attributes preserved, got %d: %v", len(cfg.SCIM.Attributes), cfg.SCIM.Attributes)
	}
}

func TestFilterUnsupportedAttributes_UnknownPassthrough(t *testing.T) {
	cfg := &config.Config{
		SCIM: config.SCIMConfig{
			Attributes: []string{"external_id", "user_name", "active", "custom_field"},
		},
	}
	support := &scim.SchemaSupport{
		UserAttributes:       map[string]bool{"externalid": true, "username": true, "active": true},
		EnterpriseAttributes: map[string]bool{},
	}

	filterUnsupportedAttributes(cfg, support)

	found := false
	for _, a := range cfg.SCIM.Attributes {
		if a == "custom_field" {
			found = true
		}
	}
	if !found {
		t.Error("unknown attributes should pass through unfiltered")
	}
}
