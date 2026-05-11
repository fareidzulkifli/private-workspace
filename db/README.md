# Historical Supabase Schema

The files in this directory are retained only as historical references for the
pre-migration Supabase/Postgres application:

- `schema.sql`
- `rls_policies.sql`

The active runtime schema is defined by SQLite migrations in `migrations/`.
Production should not apply these historical files after the Go/React/SQLite
cutover.
