# Service installation

The repository contains two executables with different service models:

- `token-usage-ingest` performs one collection pass and exits. Run it from an
  operating-system scheduler every 12 hours.
- `token-usage-dashboard` is a long-running HTTP server. Install it as a
  startup service only when the dashboard must remain available.

Examples in the OS-specific guides assume the repository remains installed
because both programs load `.env` from their working directory. Replace every
example path and user name before enabling a service.

## Common setup

1. Install Go 1.25 or newer and provide a reachable PostgreSQL database.
2. Clone the repository and create the configuration file:

   ```bash
   git clone https://github.com/seifhawamdeh/token-usage-service.git
   cd token-usage-service
   cp .env.example .env
   ```

3. Set `DATABASE_URL`, `HOST_ID`, and `ENABLED_VENDORS` in `.env`. Use a
   different stable `HOST_ID` on each machine.
4. Verify the configuration and initialize the schema:

   ```bash
   go run ./cmd/ingest validate-config
   go run ./cmd/ingest migrate
   go run ./cmd/ingest ingest
   ```

Service accounts must have read access to provider data, the repository and
`.env`, plus network access to PostgreSQL. Prefer a per-user service: provider
transcripts normally live in that user's home directory.

## Pick your operating system

| Guide | Covers |
| --- | --- |
| [Linux](service-installation-linux.md) | systemd user timer, Go install, generic and cron-only paths |
| [macOS](service-installation-macos.md) | launchd agents for ingest and dashboard |
| [Windows](service-installation-windows.md) | Task Scheduler via the GUI or PowerShell, Go install via winget |

Each guide is self-contained: prerequisites, build steps, scheduler setup,
dashboard setup, and OS-specific troubleshooting all live there.

## Upgrades

The database migration command is idempotent. Upgrade from the repository
directory, rebuild both installed binaries, migrate, and restart only the
long-running dashboard:

```bash
git pull --ff-only
go build -o "$HOME/.local/bin/token-usage-ingest" ./cmd/ingest
go build -o "$HOME/.local/bin/token-usage-dashboard" ./cmd/dashboard
"$HOME/.local/bin/token-usage-ingest" migrate
```

On Windows, use the equivalent `.exe` paths. Stop the dashboard before replacing
its executable, then start its task again.

## Verification and removal

A healthy scheduled run reports nonzero `scanned` when source files exist and
`errors=0`. Confirm database writes by querying the rated view:

```sql
SELECT vendor, count(*), max(observed_at)
FROM v_burn_usage_rated
GROUP BY vendor
ORDER BY vendor;
```

Disable the scheduler before deleting binaries or the repository. Removing a
service does not remove PostgreSQL data; database cleanup is a separate,
destructive operation.
