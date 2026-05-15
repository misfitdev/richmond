# Richmond

Go CLI that syncs Google Workspace users/groups to any SCIM v2 endpoint.

## Build & Test

```
just build          # build binary to bin/richmond
just test           # run unit tests
just lint           # golangci-lint
just check          # lint + vet + test + govulncheck
just fuzz           # fuzz SCIM client/mapping (default 30s)
```

## Architecture

- `cmd/` -- Cobra CLI: `sync` runs the pipeline, `diff` previews changes
- `internal/config/` -- YAML file + env var overrides, validation
- `internal/google/` -- Google Directory API (partial responses via fields param)
- `internal/scim/` -- Generic SCIM v2 HTTP client with retry on 429/5xx
- `internal/mapping/` -- Configurable Google-to-SCIM attribute mapping + hashing
- `internal/reconcile/` -- Diffs Google state against previous sync, emits operations
- `internal/state/` -- Sync state persistence (local JSON or GCS)

## Conventions

- slog for all logging; never log bearer tokens
- SCIM client is provider-agnostic
- Google API calls always use partial responses (fields parameter)
- State file tracks content hashes for incremental sync
- Env vars override YAML config (GOOGLE_CUSTOMER_ID, GOOGLE_ADMIN_EMAIL, SCIM_ENDPOINT, etc.)
- Google auth uses domain-wide delegation with admin email impersonation (JWT subject)
- `adopt_existing` (default true) finds pre-existing SCIM users by userName and patches them with externalId instead of creating duplicates (handles JIT-provisioned accounts)
