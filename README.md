# Richmond

A CLI tool that syncs Google Workspace directory users and groups to any
SCIM v2 endpoint. Designed to run as a scheduled Cloud Run job or locally.

## Quick start

```bash
# Preview what would change
richmond --log-level debug diff -c config.yaml

# Run the sync
richmond sync -c config.yaml

# Dry run (same as diff, but through the sync command)
richmond sync --dry-run -c config.yaml
```

## Setup

Richmond needs three things:

1. A Google Cloud service account with domain-wide delegation
2. Your Google Workspace customer ID
3. A SCIM v2 endpoint with a bearer token

### 1. Google Cloud service account

**Create the service account:**

```bash
gcloud iam service-accounts create richmond \
  --display-name="Richmond SCIM Sync" \
  --project=YOUR_PROJECT_ID
```

**Create and download a key (for local dev only -- use Workload Identity on Cloud Run):**

```bash
gcloud iam service-accounts keys create richmond-sa-key.json \
  --iam-account=richmond@YOUR_PROJECT_ID.iam.gserviceaccount.com
```

**Grant domain-wide delegation in Google Workspace Admin:**

1. Go to [admin.google.com](https://admin.google.com) > Security > Access and data control > API controls
2. Click "Manage Domain Wide Delegation"
3. Click "Add new"
4. Enter the service account's **Client ID** (find it in the GCP console under IAM > Service Accounts > your account > Details)
5. Add these OAuth scopes:
   ```
   https://www.googleapis.com/auth/admin.directory.user.readonly
   https://www.googleapis.com/auth/admin.directory.group.readonly
   https://www.googleapis.com/auth/admin.directory.group.member.readonly
   ```
6. Click "Authorize"

**Set the subject (impersonated admin):**

The service account must impersonate a Workspace admin user. Set this in the
service account key JSON or via `GOOGLE_ADMIN_EMAIL` (if using Workload Identity
with impersonation). For key-file auth, the subject is typically configured in
the JWT claims -- Richmond uses Application Default Credentials or the key file
directly, so ensure the service account has the delegated scopes above.

### 2. Google Workspace customer ID

1. Go to [admin.google.com](https://admin.google.com) > Account > Account settings
2. Copy the **Customer ID** (starts with `C`)

### 3. SCIM endpoint and token

This depends on your identity provider. Examples:

**Zitadel (self-hosted):**
- Endpoint: `https://your-zitadel-domain/scim/v2/YOUR_ORG_ID`
- Token: Create a service user in Zitadel, generate a Personal Access Token (PAT)
- Note: Zitadel currently supports SCIM Users only (not Groups). Richmond auto-detects this and skips group sync.

**Generic SCIM v2:**
- Endpoint: your provider's SCIM v2 base URL
- Token: a bearer token with permission to manage users/groups

## Configuration

Richmond reads config from a YAML file (`-c` flag) with env var overrides.
See [config.example.yaml](config.example.yaml) for all options.

### Minimal config

```yaml
google:
  credentials_file: /path/to/richmond-sa-key.json
  customer_id: C01234567

scim:
  endpoint: https://your-scim-endpoint/v2
  bearer_token: your-token-here

sync:
  state_file: /tmp/richmond-state.json
```

### Environment variables

Every config field can be set via env var. Env vars override YAML values.

| Env var | Config field |
|---------|-------------|
| `GOOGLE_CREDENTIALS_FILE` | `google.credentials_file` |
| `GOOGLE_CUSTOMER_ID` | `google.customer_id` |
| `GOOGLE_DOMAIN` | `google.domain` |
| `GOOGLE_USER_QUERY` | `google.user_query` |
| `GOOGLE_EXCLUDE_ORG_UNITS` | `google.exclude_org_units` (comma-separated) |
| `GOOGLE_INCLUDE_GROUPS` | `google.include_groups` (comma-separated) |
| `GOOGLE_EXCLUDE_GROUPS` | `google.exclude_groups` (comma-separated) |
| `GOOGLE_INCLUDE_DERIVED_MEMBERSHIP` | `google.include_derived_membership` |
| `SCIM_ENDPOINT` | `scim.endpoint` |
| `SCIM_BEARER_TOKEN` | `scim.bearer_token` |
| `SCIM_ATTRIBUTES` | `scim.attributes` (comma-separated) |
| `STATE_FILE` | `sync.state_file` |
| `DRY_RUN` | `sync.dry_run` |
| `SYNC_GROUPS` | `sync.sync_groups` |

### Filtering

**Exclude users by org unit:**

```yaml
google:
  exclude_org_units:
    - /Limited
    - /Contractors
```

Matching is hierarchical: `/Limited` also excludes `/Limited/Temps`.

**Filter groups by email pattern:**

```yaml
google:
  # Include only these groups (glob patterns):
  include_groups:
    - engineering@example.com
    - team-*@example.com

  # OR exclude these groups:
  exclude_groups:
    - noreply-*@example.com
```

Set `include_groups` or `exclude_groups`, not both.

### Attribute mapping

Choose which user attributes to sync. Core attributes (`external_id`,
`user_name`, `active`) are always included. Optional attributes:

| Attribute | Google source | SCIM target |
|-----------|-------------|-------------|
| `name` | Name.GivenName, Name.FamilyName | name.givenName, name.familyName |
| `emails` | PrimaryEmail | emails[0] |
| `title` | Organizations[0].Title | title |
| `department` | Organizations[0].Department | enterprise extension |
| `phone_numbers` | Phones | phoneNumbers |

Richmond only requests the Google API fields it needs (partial responses).

## Commands

| Command | Description |
|---------|-------------|
| `richmond sync` | Run a full or incremental sync |
| `richmond sync --dry-run` | Show what would change without making changes |
| `richmond diff` | Same as `sync --dry-run` with tabular output |

### Global flags

| Flag | Default | Description |
|------|---------|-------------|
| `-c, --config` | | Config file path |
| `--log-level` | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `-v, --version` | | Print version |

## How sync works

1. Fetch users and groups from Google Workspace Directory API
2. Apply configured filters (OU exclusion, group include/exclude)
3. Load previous sync state from state file
4. For each Google user:
   - **New** (not in state): create in SCIM
   - **Changed** (hash differs): update via SCIM PATCH
   - **Unchanged**: skip
   - **Suspended or archived**: set `active: false` in SCIM
5. For users in state but gone from Google: deactivate in SCIM
6. Same for groups (if SCIM endpoint supports them)
7. Save new state

The state file tracks SCIM-assigned IDs and content hashes for incremental sync.

## Deployment

### Local

```bash
just build
./bin/richmond sync -c config.yaml
```

### Docker

```bash
just docker-build
docker run --rm \
  -v /path/to/config.yaml:/config.yaml \
  -v /path/to/sa-key.json:/sa-key.json \
  richmond:latest richmond sync -c /config.yaml
```

### Cloud Run Job

Use env vars for config (no mounted files needed if using Workload Identity):

```bash
gcloud run jobs create richmond-sync \
  --image=gcr.io/YOUR_PROJECT/richmond:latest \
  --set-env-vars="GOOGLE_CUSTOMER_ID=C01234567,SCIM_ENDPOINT=https://...,STATE_FILE=gs://your-bucket/richmond-state.json" \
  --set-secrets="SCIM_BEARER_TOKEN=richmond-scim-token:latest" \
  --service-account=richmond@YOUR_PROJECT_ID.iam.gserviceaccount.com \
  --region=us-central1

# Schedule it
gcloud scheduler jobs create http richmond-sync-schedule \
  --schedule="0 */6 * * *" \
  --uri="https://REGION-run.googleapis.com/apis/run.googleapis.com/v1/namespaces/YOUR_PROJECT/jobs/richmond-sync:run" \
  --http-method=POST \
  --oauth-service-account-email=richmond@YOUR_PROJECT_ID.iam.gserviceaccount.com
```

## Development

```bash
just build          # build binary
just test           # run tests
just lint           # golangci-lint
just check          # lint + vet + test + govulncheck
just fuzz           # fuzz SCIM parsing (30s)
```

## License

MIT
