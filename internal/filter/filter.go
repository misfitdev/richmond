package filter

import (
	"log/slog"
	"path/filepath"
	"strings"

	admin "google.golang.org/api/admin/directory/v1"
)

// Users returns users not in any of the excluded org unit paths.
// Matching is hierarchical: excluding "/Limited" also excludes "/Limited/Temps".
func Users(users []*admin.User, excludeOrgUnits []string) []*admin.User {
	if len(excludeOrgUnits) == 0 {
		return users
	}

	var kept []*admin.User
	for _, u := range users {
		if isExcludedOU(u.OrgUnitPath, excludeOrgUnits) {
			slog.Debug("excluding user by org unit", "email", u.PrimaryEmail, "ou", u.OrgUnitPath)
			continue
		}
		kept = append(kept, u)
	}

	slog.Info("filtered users by org unit", "before", len(users), "after", len(kept), "excluded", len(users)-len(kept))
	return kept
}

// Groups returns groups matching the include/exclude rules.
// Patterns are matched against group email using filepath.Match glob syntax.
func Groups(groups []*admin.Group, include, exclude []string) []*admin.Group {
	if len(include) == 0 && len(exclude) == 0 {
		return groups
	}

	var kept []*admin.Group
	for _, g := range groups {
		if len(include) > 0 && !matchesAny(g.Email, include) {
			slog.Debug("excluding group not in include list", "email", g.Email)
			continue
		}
		if len(exclude) > 0 && matchesAny(g.Email, exclude) {
			slog.Debug("excluding group by exclude list", "email", g.Email)
			continue
		}
		kept = append(kept, g)
	}

	slog.Info("filtered groups", "before", len(groups), "after", len(kept), "excluded", len(groups)-len(kept))
	return kept
}

func isExcludedOU(ouPath string, excludes []string) bool {
	for _, ex := range excludes {
		// Exact match or hierarchical child
		if ouPath == ex || strings.HasPrefix(ouPath, ex+"/") {
			return true
		}
	}
	return false
}

func matchesAny(email string, patterns []string) bool {
	for _, p := range patterns {
		if matched, _ := filepath.Match(p, email); matched {
			return true
		}
	}
	return false
}
