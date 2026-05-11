# Operations

This directory contains deployment templates for the Go/React/SQLite runtime.

Use these files as templates, not as committed secrets:

- `systemd/private-workspace.service`: Go web server service.
- `systemd/private-workspace-backup.service`: one-shot SQLite backup job.
- `systemd/private-workspace-backup.timer`: daily backup schedule.
- `nginx/private-workspace.conf`: production Nginx reverse proxy.
- `nginx/staging-private-workspace.conf`: staging Nginx reverse proxy.
- `env/staging.env.example`: staging environment file shape.
- `env/production.env.example`: production environment file shape.

The current release model copies the Go binaries, `migrations/`, and `web/dist/`
into `/opt/private-workspace/releases/<release-id>/`, then updates
`/opt/private-workspace/current`.

Install an Nginx template into `/etc/nginx/sites-available/`, enable it from
`/etc/nginx/sites-enabled/`, then run Certbot for the matching domain after DNS
points to the VPS.
