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

The Nginx templates assume the matching Certbot certificate files already exist.
For a first-time host, bring DNS up, obtain the certificate with a temporary
HTTP-only Nginx block or `certbot certonly`, then install the template into
`/etc/nginx/sites-available/` and enable it from `/etc/nginx/sites-enabled/`.

Keep the Go service bound to `127.0.0.1` through `HTTP_ADDR`. Nginx should be
the only public HTTP/HTTPS entry point, and the templates overwrite forwarded IP
headers with `$remote_addr` before proxying to Go.
