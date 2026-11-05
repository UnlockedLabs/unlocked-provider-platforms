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
- `app seed-dev` (local only, `APP_ENV=dev`) adds the local Canvas from core's
  `make canvas`, its courses, and a year of activity, outcomes and milestones
  for core's residents. Core's `make seed` runs it.
- `app migrate-reset` (local only, `APP_ENV=dev`) rolls back the migrations.
  Core's `make migrate-fresh` runs it before resetting core's own.

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
export PROVIDER_PLATFORMS_TOKEN=local-token CORE_URL=http://127.0.0.1:9090   # serve needs both
make run      # API on :8080
make worker   # the worker
make test     # go test -race
make test-integration CORE_MIGRATIONS=/path/to/core/backend/migrations   # Postgres in Docker
make test-nats   # the LMS job flow over NATS in Docker
make lint     # golangci-lint, in Docker
make build    # the image, named after the repo
```

Settings, all from the environment:

| Variable | Used by | What it does |
|---|---|---|
| `ADDR` | serve | Where the API listens (default `:8080`) |
| `PATH_PREFIX` | serve | Path every `/api` route lives under (default none). `/internal/v1/` stays at the root |
| `PROVIDER_PLATFORMS_TOKEN` | serve | The site's secret core and this service call each other with, sent as `Authorization: Bearer <token>`. Required: an empty token rejects every call |
| `CORE_URL` | serve | Where core's `/internal/v1/core/` endpoints are, such as `http://server:8080`. Required |
| `APP_ENV` | serve, seed-dev, migrate-reset | `dev` skips the run at start of the LMS jobs, and is required by seed-dev and migrate-reset |
| `LOG_LEVEL` | all | `debug`, `info`, `warn`, `error` and the other logrus levels (default `info`). Logs are JSON like core's; a logging redo is tracked in EN-158 |
| `APP_DSN`, or `DB_HOST` `DB_PORT` `DB_USER` `DB_PASSWORD` `DB_NAME` | all | The Postgres database |
| `DB_SSLMODE` | all | TLS mode when the DSN is built from `DB_*` (default `prefer`; core uses `allow`) |
| `NATS_URL`, `NATS_USER`, `NATS_PASSWORD` | serve, worker | NATS, for the LMS jobs |
| `PROVIDER_SERVICE_URL` | serve | Where the worker's user lists are, such as `http://worker:8081` |
| `APP_KEY`, `APP_URL`, `HYDRA_ADMIN_URL`, `HYDRA_PUBLIC_URL`, `ORY_TOKEN`, `KOLIBRI_*`, `MIDDLEWARE_CRON_SCHEDULE`, `BRIGHTSPACE_TEMP_DIR` | serve, worker | What the module read in core |
| `APP_VERSION`, `GIT_COMMIT`, `GIT_BRANCH` | serve | What `/api/version` reports; CI bakes them into the image |

go.mod pins unlocked-contracts to unlocked-contracts#7's head commit until
v0.4.0 is tagged. Once it is, switch to the tag and commit the result:

```sh
GOWORK=off go mod edit -require=github.com/UnlockedLabs/unlocked-contracts@v0.4.0
GOWORK=off go mod tidy
```

## How to release

1. Merge PRs to `main` with conventional-commit titles (`feat: ...`,
   `fix: ...`). Each merge pushes `:sha-<short>` to ECR, then moves `:latest`
   to it if it's still the tip of `main`.
2. release-please keeps a `chore(main): release X.Y.Z` PR open with the
   changelog. A `feat` bumps the minor version, a `fix` the patch (before
   1.0.0, a breaking change bumps the minor version).
3. Merge that PR when you want a release. It tags `vX.Y.Z`, and CI pushes
   `:X.Y.Z` to ECR.

The `conventional` and `pr title` checks allow `feat`, `fix`, `perf`,
`revert`, `chore`, `docs`, `style`, `refactor`, `test`, `build` and `ci`. A
`!` after the type marks a breaking change.

## How to deploy

The templates live in `unlocked-platform`, behind `components.providerPlatforms`
(EN-206 part two): the `serve` Deployment, with its readiness probe on
`/api/healthcheck`, and Service; the `worker` Deployment; and a migrate Job
that runs after core's. There are no ingress paths: core proxies the routes.
Pin the release in each site file and deploy through the deploy workflow. Only
staging may use `latest`.

## Vulnerability alerts

`govulncheck` runs on every PR and on `main` every Monday, against the Go
version in `go.mod`. Vulnerabilities block the PR. A failure on `main` also
posts to Slack if the repo can see a `SLACK_WEBHOOK_URL` secret (not set up
yet; see EN-205); otherwise it shows only as a red check. To fix
one, bump the module (`go get <module>@<fixed version>`, `go mod tidy`) or the
`go` line in `go.mod` to the patch release that fixes it.
