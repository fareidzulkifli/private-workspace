# Private Workspace

Private Workspace is a Go, SQLite, and Vite React application migrated from the
former Next.js/Supabase task manager. Production is intended to run without a
Node server: Nginx terminates HTTPS using certificates managed by Certbot, the
Go binary serves APIs and the compiled Vite app, and SQLite WAL stores
application data.

## Repository Layout

```text
cmd/
  audit-supabase-schema/        # migration-only schema audit
  backup-sqlite/                # SQLite backup command
  hash-password/                # Argon2id admin password hash helper
  migrate-supabase-to-sqlite/   # migration-only importer
  server/                       # production Go server
internal/                       # Go runtime, feature APIs, auth, migration helpers
migrations/                     # active SQLite migrations
web/                            # Vite React app
ops/                            # systemd, Nginx, and env templates
.docs/ops/                      # staging, cutover, backup, restore runbooks
db/                             # historical Supabase schema references only
```

## Local Setup

Install frontend dependencies:

```bash
npm run web:install
```

Create an admin password hash:

```bash
ADMIN_PASSWORD='replace-me' go run ./cmd/hash-password
```

Copy `.env.local.example` to `.env.local` and set the Go runtime values. Do not
commit real secrets. The local start script loads `.env.local` for you.

Build the Vite app and start the Go server:

```bash
npm run web:build
npm start
```

The Go server serves `web/dist/index.html` and `web/dist/assets/*` from disk.
Private browser routes fall back to the Vite app shell after auth checks.

## Tests and Builds

Go tests:

```bash
go test ./cmd/... ./internal/...
```

Frontend production build:

```bash
cd web
npm run build
```

Build production binaries:

```bash
go build -o dist/private-workspace ./cmd/server
go build -o dist/backup-sqlite ./cmd/backup-sqlite
```

## Migration Commands

Audit the live Supabase/Postgres schema:

```bash
SUPABASE_DB_URL='postgres://...' go run ./cmd/audit-supabase-schema
```

Run a dry-run Supabase-to-SQLite import:

```bash
SUPABASE_DB_URL='postgres://...' \
SQLITE_PATH=./data/private-workspace-shadow.db \
MIGRATION_DRY_RUN=true \
go run ./cmd/migrate-supabase-to-sqlite
```

Run a real import into a new SQLite file:

```bash
SUPABASE_DB_URL='postgres://...' \
SQLITE_PATH=./data/private-workspace.db \
MIGRATION_DRY_RUN=false \
go run ./cmd/migrate-supabase-to-sqlite
```

Create a consistent SQLite backup:

```bash
SQLITE_PATH=./data/private-workspace.db \
BACKUP_DIR=./backups \
BACKUP_TIER=hourly \
go run ./cmd/backup-sqlite
```

If complete R2 variables are present, `backup-sqlite` also uploads the
compressed backup to Cloudflare R2 under `R2_BACKUP_PREFIX`.

## Deployment

Use the deployment templates in:

```text
ops/
```

The supported VPS release model is:

```text
/opt/private-workspace/releases/<release-id>/
  private-workspace
  backup-sqlite
  migrations/
  web/dist/

/opt/private-workspace/current -> releases/<release-id>
/var/lib/private-workspace/private-workspace.db
/etc/private-workspace/env
```

On the VPS, keep the Go service bound to localhost and let Nginx be the only
public HTTP/HTTPS entry point. Certbot manages the Let's Encrypt certificate and
Nginx reloads when certificates renew.
