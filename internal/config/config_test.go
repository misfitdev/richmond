package config

import (
	"os"
	"testing"
)

func TestLoad_Defaults(t *testing.T) {
	t.Setenv("GOOGLE_CUSTOMER_ID", "C01234567")
	t.Setenv("SCIM_ENDPOINT", "https://scim.example.com/v2")
	t.Setenv("SCIM_BEARER_TOKEN", "tok")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	if cfg.Google.IncludeDerivedMembership == nil {
		t.Fatal("IncludeDerivedMembership should default to non-nil")
	}
	if !*cfg.Google.IncludeDerivedMembership {
		t.Error("IncludeDerivedMembership should default to true")
	}

	if cfg.SCIM.Attributes == nil {
		t.Fatal("Attributes should default to non-nil")
	}
}

func TestValidate_MutuallyExclusiveGroups(t *testing.T) {
	t.Setenv("GOOGLE_CUSTOMER_ID", "C01234567")
	t.Setenv("SCIM_ENDPOINT", "https://scim.example.com/v2")
	t.Setenv("SCIM_BEARER_TOKEN", "tok")
	t.Setenv("GOOGLE_INCLUDE_GROUPS", "eng@example.com")
	t.Setenv("GOOGLE_EXCLUDE_GROUPS", "noreply-*@example.com")

	_, err := Load("")
	if err == nil {
		t.Fatal("expected error when include_groups and exclude_groups are both set")
	}
}

func TestValidate_MissingCustomerID(t *testing.T) {
	os.Unsetenv("GOOGLE_CUSTOMER_ID")
	t.Setenv("SCIM_ENDPOINT", "https://scim.example.com/v2")
	t.Setenv("SCIM_BEARER_TOKEN", "tok")

	_, err := Load("")
	if err == nil {
		t.Fatal("expected error for missing customer_id")
	}
}

func TestApplyEnvOverrides_TrimmedSplit(t *testing.T) {
	t.Setenv("GOOGLE_CUSTOMER_ID", "C01234567")
	t.Setenv("SCIM_ENDPOINT", "https://scim.example.com/v2")
	t.Setenv("SCIM_BEARER_TOKEN", "tok")
	t.Setenv("GOOGLE_EXCLUDE_ORG_UNITS", "/Limited, /Contractors , /Temp")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	want := []string{"/Limited", "/Contractors", "/Temp"}
	if len(cfg.Google.ExcludeOrgUnits) != len(want) {
		t.Fatalf("ExcludeOrgUnits = %v, want %v", cfg.Google.ExcludeOrgUnits, want)
	}
	for i, v := range want {
		if cfg.Google.ExcludeOrgUnits[i] != v {
			t.Errorf("ExcludeOrgUnits[%d] = %q, want %q", i, cfg.Google.ExcludeOrgUnits[i], v)
		}
	}
}

func TestApplyEnvOverrides_InvalidBoolIgnored(t *testing.T) {
	t.Setenv("GOOGLE_CUSTOMER_ID", "C01234567")
	t.Setenv("SCIM_ENDPOINT", "https://scim.example.com/v2")
	t.Setenv("SCIM_BEARER_TOKEN", "tok")
	t.Setenv("DRY_RUN", "yes") // not a valid strconv.ParseBool value

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	// Invalid value is ignored; DryRun stays at YAML/default (false).
	if cfg.Sync.DryRun {
		t.Error("DryRun should remain false when env var value is invalid")
	}
}

func TestApplyEnvOverrides_IncludeDerivedMembership(t *testing.T) {
	t.Setenv("GOOGLE_CUSTOMER_ID", "C01234567")
	t.Setenv("SCIM_ENDPOINT", "https://scim.example.com/v2")
	t.Setenv("SCIM_BEARER_TOKEN", "tok")
	t.Setenv("GOOGLE_INCLUDE_DERIVED_MEMBERSHIP", "false")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	if cfg.Google.IncludeDerivedMembership == nil {
		t.Fatal("IncludeDerivedMembership should not be nil")
	}
	if *cfg.Google.IncludeDerivedMembership {
		t.Error("IncludeDerivedMembership should be false when env var is false")
	}
}

func TestSplitTrimmed(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"/A,/B,/C", []string{"/A", "/B", "/C"}},
		{"/A, /B , /C", []string{"/A", "/B", "/C"}},
		{"/A,,/C", []string{"/A", "/C"}}, // empty element dropped
		{"  /A  ", []string{"/A"}},
		{"", []string{}},
	}

	for _, tt := range tests {
		got := splitTrimmed(tt.input)
		if len(got) != len(tt.want) {
			t.Errorf("splitTrimmed(%q) = %v, want %v", tt.input, got, tt.want)
			continue
		}
		for i := range tt.want {
			if got[i] != tt.want[i] {
				t.Errorf("splitTrimmed(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
			}
		}
	}
}
