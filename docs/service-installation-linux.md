# Linux installation

Prefer a per-user systemd timer. It runs ingest at midnight and noon,
including one catch-up run after missed schedules, and needs no root service
account. Fall back to cron only on systems without systemd.

## 1. Install Go

The project requires Go 1.25 or newer. First check the installed version:

```bash
go version
```

Install the download and source-control tools when needed. Use the command for
the Linux distribution:

```bash
# Debian or Ubuntu
sudo apt-get update
sudo apt-get install -y ca-certificates curl git

# Fedora, RHEL, or Rocky Linux
sudo dnf install -y ca-certificates curl git
```

If Go is missing or older, download a current Linux archive from
[go.dev/dl](https://go.dev/dl/). Select `linux-amd64` for most Intel/AMD servers
or `linux-arm64` for 64-bit ARM. The example below installs Go 1.25.0 on AMD64
when `/usr/local/go` does not already exist:

```bash
cd /tmp
curl -LO https://go.dev/dl/go1.25.0.linux-amd64.tar.gz
if [ -e /usr/local/go ]; then
  echo '/usr/local/go already exists; upgrade it before continuing'
  exit 1
fi
sudo tar -C /usr/local -xzf go1.25.0.linux-amd64.tar.gz
echo 'export PATH=/usr/local/go/bin:$PATH' >> ~/.profile
export PATH=/usr/local/go/bin:$PATH
go version
```

For ARM64, replace `linux-amd64` with `linux-arm64` in the filename and URL. If
`/usr/local/go` already exists, upgrade it using the official Go installation
instructions instead of extracting one installation over another.

## 2. Install the repository

```bash
mkdir -p ~/src ~/.local/bin
git clone https://github.com/seifhawamdeh/token-usage-service.git ~/src/token-usage-service
cd ~/src/token-usage-service
cp .env.example .env
```

If the repository already exists:

```bash
cd ~/src/token-usage-service
git pull --ff-only
```

## 3. Configure the service

Edit `.env` and set the database connection, a stable machine identity, and the
enabled adapters:

```dotenv
DATABASE_URL=postgres://USER:PASSWORD@HOST:5432/token_usage?sslmode=require
HOST_ID=HOSTNAME
ENABLED_VENDORS=claude-code,codex,gemini
SINK=postgres
```

Replace `HOSTNAME` with the output of `hostname`. Replace the database
placeholder with the real connection string, then protect the file:

```bash
chmod 600 .env
```

Default discovery checks `~/.claude/projects`, `~/.codex/sessions`, and
`~/.gemini/tmp`. A missing directory is harmless but produces no snapshots for
that provider. Set the matching path override in `.env` if data lives elsewhere.

## 4. Build and initialize

```bash
go build -o ~/.local/bin/token-usage-ingest ./cmd/ingest
~/.local/bin/token-usage-ingest validate-config
~/.local/bin/token-usage-ingest migrate
~/.local/bin/token-usage-ingest ingest
```

The manual ingest should finish with `errors=0` before installing the timer.

## 5. Install and verify the timer

Use the checked-in user unit and timer:

```bash
mkdir -p ~/.config/systemd/user
cp deploy/systemd/token-usage-ingest.service ~/.config/systemd/user/
cp deploy/systemd/token-usage-ingest.timer ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable --now token-usage-ingest.timer
systemctl --user start token-usage-ingest.service
systemctl --user list-timers token-usage-ingest.timer
journalctl --user -u token-usage-ingest.service -n 50
```

The checked-in unit already uses `%h/src/token-usage-service`, so no path edit
is needed when the clone location above is used. If the repository lives
elsewhere, edit `~/.config/systemd/user/token-usage-ingest.service` and set
`WorkingDirectory` and `EnvironmentFile` to the absolute repository path before
reloading.

The final two commands in the block above perform a manual test and show its
logs. On a headless machine, enable user lingering so the timer runs while the
user is logged out:

```bash
sudo loginctl enable-linger "$USER"
```

## Dashboard

Build the dashboard and create `~/.config/systemd/user/token-usage-dashboard.service`:

```bash
go build -o "$HOME/.local/bin/token-usage-dashboard" ./cmd/dashboard
```

```ini
[Unit]
Description=Token usage dashboard
After=network-online.target

[Service]
Type=simple
WorkingDirectory=/home/USER/src/token-usage-service
EnvironmentFile=/home/USER/src/token-usage-service/.env
ExecStart=/home/USER/.local/bin/token-usage-dashboard
Restart=on-failure

[Install]
WantedBy=default.target
```

```bash
systemctl --user daemon-reload
systemctl --user enable --now token-usage-dashboard.service
journalctl --user -u token-usage-dashboard.service -n 50
```

## Systems without systemd

Cron works on BSD, minimal Linux distributions, and Unix systems without a
supported service manager. Build the ingest binary and add this entry with
`crontab -e`, replacing both absolute paths:

```cron
0 */12 * * * cd /home/USER/src/token-usage-service && /home/USER/.local/bin/token-usage-ingest ingest >> /home/USER/.local/state/token-usage-ingest.log 2>&1
```

Create the log directory before the first run. Cron has a small environment, so
keep runtime configuration in the repository `.env` and use absolute paths.
Some cron implementations do not support `*/12`; use two entries at `0 0 * * *`
and `0 12 * * *` in that case.
