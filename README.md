# unlocked-provider-platforms

Provider Platforms as its own service (EN-206): LMS connections (Canvas,
Brightspace, Kolibri, Essential Education), the users linked to them, their
sync jobs, and the OIDC clients for LMS sign-in. Built from
unlocked-product-template; the module code is in `internal/providerplatforms`.

One binary, one image:

- `app migrate` applies the embedded migrations (ledger
  `goose_db_version_providerplatforms`). Run it after core's migrations: the
  first one moves tables core created out of `public`.
- `app serve` runs the API and the job scheduler.
- `app worker` runs the LMS jobs and serves the provider user lists on `:8081`.

Core stays the public edge. It checks the session, role, feature flag and
facility, then proxies the module's `/api` routes here with the service token
and the caller's claims. Core calls the typed interface under
`/internal/v1/provider-platforms/` (unlocked-contracts `providerplatforms`),
and this service calls core under `/internal/v1/core/` (`coreapi`). Neither
path is ever routed by ingress, and `PATH_PREFIX` doesn't apply to the
internal one: core reaches it at the service's own address.

## Run it locally

Needs Go 1.26 and Docker, and a database that already has core's schema.

```sh
export SERVICE_TOKEN=local-token CORE_URL=http://localhost:9090   # serve needs both
make run      # API on :8080
make worker   # the worker
make test     # go test -race
make test-integration CORE_MIGRATIONS=/path/to/core/backend/migrations   # Postgres in Docker
make lint     # golangci-lint, in Docker
make build    # the image, named after the repo
```

Settings, all from the environment:

| Variable | Used by | What it does |
|---|---|---|
| `ADDR` | serve | Where the API listens (default `:8080`) |
| `PATH_PREFIX` | serve | Path every `/api` route lives under (default none). `/internal/v1/` stays at the root |
| `SERVICE_TOKEN` | serve | The shared token, sent as `Authorization: Bearer <token>`. Required: an empty token rejects every call |
| `CORE_URL` | serve | Where core's `/internal/v1/core/` endpoints are, such as `http://server:8080`. Required |
| `APP_ENV` | serve | `dev` skips the run at start of the LMS jobs |
| `LOG_LEVEL` | all | `debug`, `info`, `warn`, `error` and the other logrus levels (default `info`). Logs are JSON like core's; a logging redo is tracked in EN-158 |
| `APP_DSN`, or `DB_HOST` `DB_PORT` `DB_USER` `DB_PASSWORD` `DB_NAME` | all | The Postgres database |
| `NATS_URL`, `NATS_USER`, `NATS_PASSWORD` | serve, worker | NATS, for the LMS jobs |
| `PROVIDER_SERVICE_URL` | serve | Where the worker's user lists are, such as `http://worker:8081` |
| `APP_KEY`, `APP_URL`, `HYDRA_ADMIN_URL`, `HYDRA_PUBLIC_URL`, `ORY_TOKEN`, `KOLIBRI_*`, `MIDDLEWARE_CRON_SCHEDULE`, `BRIGHTSPACE_TEMP_DIR` | serve, worker | What the module read in core |
| `APP_VERSION`, `GIT_COMMIT`, `GIT_BRANCH` | serve | What `/api/version` reports; CI bakes them into the image |

The module needs unlocked-contracts v0.4.0. Merge unlocked-contracts first and
tag it, then merge this repo. Until it is tagged, builds need an untracked
`go.work` that replaces it with a local checkout. Once it is tagged, run
`GOWORK=off go mod tidy` and commit the new `go.sum` lines, or CI and the
image build fail on a missing checksum.

## How to release

1. Merge PRs to `main` with conventional-commit titles (`feat: ...`,
   `fix: ...`). Each merge pushes `:sha-<short>` to ECR, then moves `:latest`
   to it if it's still the tip of `main`.
2. release-please keeps a `chore(main): release X.Y.Z` PR open with the
   changelog. A `feat` bumps the minor version, a `fix` the patch (before
   1.0.0, a breaking change bumps the minor version).
3. Merge that PR when you want a release. It tags `vX.Y.Z`, and CI pushes
   `:X.Y.Z` to ECR.

### Commit types

The `conventional` and `pr title` checks allow these types, and nothing else:

| Type | In the changelog? | Version bump |
|---|---|---|
| `feat` | yes | minor |
| `fix`, `perf`, `revert` | yes | patch |
| `chore`, `docs`, `style`, `refactor`, `test`, `build`, `ci` | no | none on its own |

A `!` after any type (`feat!:`, `refactor!:`) marks a breaking change: it
always shows in the changelog and bumps the major version (the minor before
1.0.0). A scope is optional: `fix(worker): ...`.

## How to deploy

In `unlocked-platform`:

1. **First time only:** add the product's templates to the chart behind
   `components.<product>`: a Deployment for `serve` with a readiness probe on
   `<PATH_PREFIX>/api/healthcheck` (and one for `worker` if it has background
   jobs), a Service, ingress paths for its `PATH_PREFIX`, and a
   per-revision migrate Job if it has tables.
2. Pin the release in each site file (`images.<product>.tag: X.Y.Z`) and
   deploy through the deploy workflow. Only staging may use `latest`.

## Vulnerability alerts in Slack

`govulncheck` checks the code, its dependencies and the Go version against the
Go vulnerability database. It's a required check on every push, it runs on
`main` every Monday at 14:00 UTC so new vulnerabilities are caught without a
push, and it can be run by hand (**Actions → govulncheck → Run workflow**).

When it fails on `main`, it posts one line to Slack: `UnlockedLabs/<repo>:
govulncheck failed on main.` and a link to the run. Failures on other branches
don't post; the PR's red check shows them. It posts only if the repo can see a
secret named `SLACK_WEBHOOK_URL`. It's a secret, not a variable, because anyone
with the URL can post to the channel. Without it, the run logs a notice and
posts nothing.

**Still to decide (EN-205):** one organization secret shared by every product
repo, or a secret per repo, and which channel the alerts go to. Until then,
repos run without it.

### Get a webhook URL

The URL comes from Slack. GitHub never shows a secret's value again, so it
can't be copied from core's `SLACK_WEBHOOK_URL`.

1. Go to [api.slack.com/apps](https://api.slack.com/apps) and sign in to the
   Unlocked workspace.
2. Open the app that already posts our alerts (core's go to
   `#unlockedv2-alerts`). If there isn't one, **Create New App → From
   scratch**, name it (such as "Unlocked CI alerts"), pick the workspace and
   **Create App**.
3. **Features → Incoming Webhooks**, and turn on **Activate Incoming
   Webhooks**.
4. **Add New Webhook to Workspace**, pick the channel and **Allow**. A
   workspace admin may have to approve a new app first.
5. Copy the URL (`https://hooks.slack.com/services/...`). Each URL posts to
   one channel; a different channel needs a new webhook.

### Give it to the repo

**As an organization secret** (set up once; an org admin does it):

1. Org **Settings → Secrets and variables → Actions → New organization
   secret**.
2. Name `SLACK_WEBHOOK_URL`, the URL as the value, **Repository access:
   Selected repositories**, pick the repos, **Add secret**.

For each new repo after that, open the secret, add the repo under **Repository
access** and **Save**. Or, with a token that has the `admin:org` scope
(`gh auth refresh -h github.com -s admin:org`):

```sh
gh api -X PUT orgs/UnlockedLabs/actions/secrets/SLACK_WEBHOOK_URL/repositories/$(gh api repos/UnlockedLabs/<repo> --jq .id)
```

Don't use `gh secret set --org ... --repos ...` to add a repo: it replaces the
secret's whole repo list.

**As a repo secret** (a repo admin does it); `gh` asks for the URL, so it stays
out of your shell history:

```sh
gh secret set SLACK_WEBHOOK_URL -R UnlockedLabs/<repo>
```

Or repo **Settings → Secrets and variables → Actions → New repository secret**.

### Check it

The repo can see the secret if its name is listed by one of these:

```sh
gh secret list -R UnlockedLabs/<repo>
gh api repos/UnlockedLabs/<repo>/actions/organization-secrets --jq '.secrets[].name'
```

The first lists repo secrets, the second organization secrets shared with the
repo. To check the URL itself, post a test message to its channel:

```sh
curl --fail -X POST -H 'Content-Type: application/json' --data '{"text":"test from <repo>"}' '<url>'
```

### Change or revoke the URL

If the URL leaks or the channel changes, remove the webhook in the Slack app's
**Incoming Webhooks** page (that kills the old URL), add a new one, and update
the secret. Updating an organization secret updates every repo that uses it.

### When the weekly run stops

GitHub turns off scheduled workflows in a public repo after 60 days with no
activity. Turn it back on at **Actions → govulncheck → Enable workflow**.

### When the check fails

- **A finding in a dependency:** `go get <module>@<fixed version>`, then
  `go mod tidy`, in a PR.
- **A finding in the standard library:** a newer Go patch release fixes it.
  The check and the image both use the newest patch of the Go version in
  `go.mod`, so rerun the check once that release is out, and merge something
  so `main` builds a new image with it.
