# Provider Platforms

LMS connections (Canvas, Brightspace, Kolibri, Essential Education), the users
linked to them, their courses, outcomes and activity, and the OIDC clients that
let residents log in to them. It moved here from unlocked-core in EN-206.

## Boundaries

- **Nothing here imports core.** The package reaches core through
  `internal/module`: `Users` and `LiveProviders` are the `coreapi` clients from
  unlocked-contracts, which call core's `/internal/v1/core/` endpoints.
- **Core asks it questions through the typed interface** in unlocked-contracts
  (`providerplatforms.ProviderPlatforms`). `api.go` answers it, and the server
  mounts the contracts handler at `/internal/v1/provider-platforms/`.
- **Core is the public edge.** It checks the session, role, feature flag and
  facility, then proxies the `/api` routes in `handlers.go` here with the
  service token and the caller's claims. The routes themselves enforce none of
  that.
- **It reads core's `users` table** to match and list the users linked to a
  provider (the `user` type in `models.go`). It's the only core table its
  Postgres role can read.

## Database

- Its tables live in the `provider` schema; migration `00001` moved them there
  from `public`, so it has to run after core's migrations. `app migrate`
  applies it and records it in `goose_db_version_providerplatforms`, the table
  core's runner used before the move.
- `serve` and `worker` connect as the app's database user and `SET ROLE
  provider` with `search_path = provider, public`. The role can't log in and
  can write only the `provider` schema.
- Later migrations must create tables as `provider.<name>`.
- On SQLite (the tests) there are no schemas or roles.

## Files

| File | What it holds |
|---|---|
| `module.go` | Registers the routes and the LMS jobs, starts the worker, sets up the `oauth_state` bucket, lists the test tables |
| `handlers.go` | The routes and the provider, OAuth, OIDC and learning-data handlers |
| `users.go`, `matching.go`, `accounts.go` | Logins and mappings, importing and matching users, creating Canvas or Kolibri accounts |
| `jobs.go`, `worker.go` | Schedules and publishes each provider's LMS jobs, and the worker that runs them and serves the provider user lists |
| `canvas.go`, `brightspace*.go`, `kolibri*.go` | Each LMS's sync |
| `providerservice.go` | The client for the worker's user lists |
| `store.go`, `models.go`, `http.go` | Queries, tables as Go types, response and error helpers |
| `api.go` | The answers to core's questions |
| `migrations/` | This module's migrations |

## Tests

They run on SQLite without NATS, Canvas, Brightspace or Essential Education, so
they can't run the Brightspace OAuth flow, creating Canvas accounts and logins,
seeding demo data (which calls Postgres procedures), or the schema move and the
role. Run `app migrate` against a database that has core's schema to check
those.
