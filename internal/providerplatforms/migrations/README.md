# Provider Platforms migrations

Goose `.sql` migrations for this module, embedded in the `app` binary and applied
by `app migrate`. They run after core's, and their versions are tracked in
`public.goose_db_version_providerplatforms`.

Never edit a migration that has been merged; add a new one.

Create new tables as `provider.<name>`. `app migrate` connects as the database
user, so an unqualified table lands in `public`, where the `provider` role
can't use it.
