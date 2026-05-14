package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Google GoogleConfig `yaml:"google"`
	SCIM   SCIMConfig   `yaml:"scim"`
	Sync   SyncConfig   `yaml:"sync"`
}

type GoogleConfig struct {
	CredentialsFile string   `yaml:"credentials_file"`
	CustomerID      string   `yaml:"customer_id"`
	Domain          string   `yaml:"domain"`
	UserQuery       string   `yaml:"user_query"`
	ExcludeOrgUnits []string `yaml:"exclude_org_units"`

	// Group filtering. Set include OR exclude, not both.
	// Supports glob patterns matched against group email.
	IncludeGroups []string `yaml:"include_groups"`
	ExcludeGroups []string `yaml:"exclude_groups"`

	// Flatten nested group memberships. Defaults to true.
	IncludeDerivedMembership *bool `yaml:"include_derived_membership"`
}

type SCIMConfig struct {
	Endpoint    string   `yaml:"endpoint"`
	BearerToken string   `yaml:"bearer_token"`
	Attributes  []string `yaml:"attributes"`
}

type SyncConfig struct {
	StateFile string `yaml:"state_file"`
	DryRun    bool   `yaml:"dry_run"`
}

// DefaultAttributes are always synced regardless of config.
var DefaultAttributes = []string{"external_id", "user_name", "active"}

// OptionalAttributes can be enabled via config.
var OptionalAttributes = []string{"name", "emails", "title", "department", "phone_numbers"}

func Load(path string) (*Config, error) {
	cfg := &Config{}

	if path != "" {
		data, err := os.ReadFile(path) //nolint:gosec // config path from trusted CLI flag
		if err != nil {
			return nil, fmt.Errorf("reading config file: %w", err)
		}
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parsing config file: %w", err)
		}
	}

	applyEnvOverrides(cfg)

	if cfg.SCIM.Attributes == nil {
		all := make([]string, 0, len(DefaultAttributes)+len(OptionalAttributes))
		all = append(all, DefaultAttributes...)
		all = append(all, OptionalAttributes...)
		cfg.SCIM.Attributes = all
	}

	if cfg.Google.IncludeDerivedMembership == nil {
		t := true
		cfg.Google.IncludeDerivedMembership = &t
	}

	if err := validate(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("GOOGLE_CREDENTIALS_FILE"); v != "" {
		cfg.Google.CredentialsFile = v
	}
	if v := os.Getenv("GOOGLE_CUSTOMER_ID"); v != "" {
		cfg.Google.CustomerID = v
	}
	if v := os.Getenv("GOOGLE_DOMAIN"); v != "" {
		cfg.Google.Domain = v
	}
	if v := os.Getenv("GOOGLE_USER_QUERY"); v != "" {
		cfg.Google.UserQuery = v
	}
	if v := os.Getenv("SCIM_ENDPOINT"); v != "" {
		cfg.SCIM.Endpoint = v
	}
	if v := os.Getenv("SCIM_BEARER_TOKEN"); v != "" {
		cfg.SCIM.BearerToken = v
	}
	if v := os.Getenv("SCIM_ATTRIBUTES"); v != "" {
		cfg.SCIM.Attributes = strings.Split(v, ",")
	}
	if v := os.Getenv("GOOGLE_EXCLUDE_ORG_UNITS"); v != "" {
		cfg.Google.ExcludeOrgUnits = splitTrimmed(v)
	}
	if v := os.Getenv("GOOGLE_INCLUDE_GROUPS"); v != "" {
		cfg.Google.IncludeGroups = splitTrimmed(v)
	}
	if v := os.Getenv("GOOGLE_EXCLUDE_GROUPS"); v != "" {
		cfg.Google.ExcludeGroups = splitTrimmed(v)
	}
	if v := os.Getenv("GOOGLE_INCLUDE_DERIVED_MEMBERSHIP"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			slog.Warn("invalid GOOGLE_INCLUDE_DERIVED_MEMBERSHIP value, ignoring", "value", v)
		} else {
			cfg.Google.IncludeDerivedMembership = &b
		}
	}
	if v := os.Getenv("STATE_FILE"); v != "" {
		cfg.Sync.StateFile = v
	}
	if v := os.Getenv("DRY_RUN"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			slog.Warn("invalid DRY_RUN value, ignoring", "value", v)
		} else {
			cfg.Sync.DryRun = b
		}
	}
}

// splitTrimmed splits a comma-separated string and trims whitespace from each element.
func splitTrimmed(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func validate(cfg *Config) error {
	if cfg.Google.CustomerID == "" {
		return fmt.Errorf("google.customer_id is required")
	}
	if cfg.SCIM.Endpoint == "" {
		return fmt.Errorf("scim.endpoint is required")
	}
	if cfg.SCIM.BearerToken == "" {
		return fmt.Errorf("scim.bearer_token is required")
	}
	if len(cfg.Google.IncludeGroups) > 0 && len(cfg.Google.ExcludeGroups) > 0 {
		return fmt.Errorf("google.include_groups and google.exclude_groups are mutually exclusive")
	}
	return nil
}

// HasAttribute returns true if the given attribute is enabled in config.
func (c *Config) HasAttribute(name string) bool {
	for _, a := range c.SCIM.Attributes {
		if a == name {
			return true
		}
	}
	return false
}
