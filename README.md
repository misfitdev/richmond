<div align="center">

<h1>richmond</h1>

---

[![CI](https://github.com/misfitdev/richmond/actions/workflows/ci.yml/badge.svg)](https://github.com/misfitdev/richmond/actions/workflows/ci.yml)
[![Release](https://github.com/misfitdev/richmond/actions/workflows/release.yml/badge.svg)](https://github.com/misfitdev/richmond/actions/workflows/release.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/misfitdev/richmond)](https://goreportcard.com/report/github.com/misfitdev/richmond)
[![license](https://img.shields.io/github/license/misfitdev/richmond?style=flat&color=blue)](LICENSE)
[![SLSA 2](https://slsa.dev/images/gh-badge-level2.svg)](https://slsa.dev)

Sync Google Workspace directory users and groups to any SCIM v2 endpoint.
Reads from the Google Directory API, maps to SCIM resources, and pushes
creates, updates, and deactivations. Runs as a scheduled Cloud Run job or locally.

[Install](#install) · [Quick start](#quick-start) · [Commands](#commands) · [Config](#configuration) · [How it works](#how-it-works) · [Deploy](#deployment)

</div>

---

## Install

**Binary** — grab the latest from [Releases](https://github.com/misfitdev/richmond/releases):

```bash
# macOS / Linux
tar xzf richmond_*.tar.gz
sudo mv richmond /usr/local/bin/
```

**Container:**

```bash
docker pull ghcr.io/misfitdev/richmond:latest
```

**From source:**

```bash
go install github.com/misfitdev/richmond@latest
```

---

## Quick start

```bash
# Preview what would change (no writes)
richmond --log-level debug diff -c config.yaml

# Run the sync
richmond sync -c config.yaml
```

Richmond needs three things: a Google Cloud service account with domain-wide delegation, your Workspace customer ID, and a SCIM endpoint with a bearer token. See [Setup](#setup) for step-by-step instructions.

---

## Commands

| Command | Description |
|---------|-------------|
| `richmond sync` | Run a full or incremental sync |
| `richmond sync --dry-run` | Show what would change without writing |
| `richmond diff` | Preview changes in tabular format |

**Global flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `-c, --config` | | Config file path |
| `--log-level` | `info` | `debug`, `info`, `warn`, `error` |
| `-v, --version` | | Print version |

---

## Configuration

YAML file with env var overrides. See [`config.example.yaml`](config.example.yaml) for all options.

### Minimal

```yaml
google:
  credentials_file: /path/to/sa-key.json
  customer_id: C01234567

scim:
  endpoint: https://your-scim-endpoint/v2
  bearer_token: your-token-here

sync:
  state_file: /tmp/richmond-state.json
```

### Environment variables

Env vars override YAML. Useful for Cloud Run where everything is env-based.

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

**Exclude users by org unit** (hierarchical &mdash; `/Limited` also excludes `/Limited/Temps`):

```yaml
google:
  exclude_org_units:
    - /Limited
    - /Contractors
```

**Filter groups by email** (glob patterns, set include _or_ exclude, not both):

```yaml
google:
  include_groups:
    - engineering@example.com
    - team-*@example.com
```

### Attribute mapping

Core attributes (`external_id`, `user_name`, `active`) are always synced. Optional:

| Attribute | Google source | SCIM target |
|-----------|-------------|-------------|
| `name` | Name | name.givenName, name.familyName |
| `emails` | PrimaryEmail | emails[0] |
| `title` | Organizations[0].Title | title |
| `department` | Organizations[0].Department | enterprise extension |
| `phone_numbers` | Phones | phoneNumbers |

Only the Google API fields needed for configured attributes are requested (partial responses).

---

## How it works

1. Fetch users and groups from Google Workspace Directory API
2. Apply configured filters (OU exclusion, group include/exclude)
3. Load previous sync state
4. Diff: create new users, update changed users, deactivate removed/suspended/archived users
5. Same for groups (auto-detected &mdash; skipped if SCIM endpoint doesn't support them)
6. Save state for next run

State tracks SCIM-assigned IDs and content hashes for incremental sync.

---

## Setup

### 1. Google Cloud service account

```bash
gcloud iam service-accounts create richmond \
  --display-name="Richmond SCIM Sync" \
  --project=YOUR_PROJECT_ID

# Local dev only -- use Workload Identity on Cloud Run
gcloud iam service-accounts keys create richmond-sa-key.json \
  --iam-account=richmond@YOUR_PROJECT_ID.iam.gserviceaccount.com
```

**Domain-wide delegation:**

1. [admin.google.com](https://admin.google.com) > Security > Access and data control > API controls
2. Manage Domain Wide Delegation > Add new
3. Enter the service account **Client ID**
4. Add scopes:
   ```
   https://www.googleapis.com/auth/admin.directory.user.readonly
   https://www.googleapis.com/auth/admin.directory.group.readonly
   https://www.googleapis.com/auth/admin.directory.group.member.readonly
   ```

### 2. Workspace customer ID

[admin.google.com](https://admin.google.com) > Account > Account settings > copy the **Customer ID** (starts with `C`).

### 3. SCIM endpoint

**Zitadel:** `https://your-domain/scim/v2/YOUR_ORG_ID` with a service user PAT. Zitadel supports SCIM Users only; Richmond auto-detects this and skips groups.

**Generic SCIM v2:** your provider's base URL + a bearer token with user/group management permissions.

---

## Deployment

### Local

```bash
just build
./bin/richmond sync -c config.yaml
```

### Docker

```bash
docker run --rm \
  -v /path/to/config.yaml:/config.yaml \
  ghcr.io/misfitdev/richmond:latest richmond sync -c /config.yaml
```

### Cloud Run Job

```bash
gcloud run jobs create richmond-sync \
  --image=ghcr.io/misfitdev/richmond:latest \
  --set-env-vars="GOOGLE_CUSTOMER_ID=C01234567,SCIM_ENDPOINT=https://...,STATE_FILE=gs://bucket/state.json" \
  --set-secrets="SCIM_BEARER_TOKEN=richmond-scim-token:latest" \
  --service-account=richmond@YOUR_PROJECT.iam.gserviceaccount.com

gcloud scheduler jobs create http richmond-sync-schedule \
  --schedule="0 */6 * * *" \
  --uri="https://REGION-run.googleapis.com/apis/run.googleapis.com/v1/namespaces/PROJECT/jobs/richmond-sync:run" \
  --http-method=POST \
  --oauth-service-account-email=richmond@YOUR_PROJECT.iam.gserviceaccount.com
```

---

## Development

```bash
just build       # build binary
just test        # run tests
just lint        # golangci-lint
just check       # lint + vet + test + govulncheck
just fuzz        # fuzz SCIM parsing (30s)
```

---

## License

[MIT](LICENSE)
