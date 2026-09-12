# Service installation

The repository contains two executables with different service models:

- `token-usage-ingest` performs one collection pass and exits. Run it from an
  operating-system scheduler every 12 hours.
- `token-usage-dashboard` is a long-running HTTP server. Install it as a
  startup service only when the dashboard must remain available.

Examples below assume the repository remains installed because both programs
load `.env` from their working directory. Replace every example path and user
name before enabling a service.

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

## Generic Linux installation

This procedure installs a per-user systemd timer and collects only Claude Code,
Codex, and Gemini sessions owned by the current Linux user.

### 1. Install Go

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

### 2. Install the repository

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

### 3. Configure the service

Edit `.env` and set the database connection, a stable machine identity, and the
three enabled adapters:

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

### 4. Build and initialize

```bash
go build -o ~/.local/bin/token-usage-ingest ./cmd/ingest
~/.local/bin/token-usage-ingest validate-config
~/.local/bin/token-usage-ingest migrate
~/.local/bin/token-usage-ingest ingest
```

The manual ingest should finish with `errors=0` before installing the timer.

### 5. Install and verify the timer

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

The checked-in unit already uses `%h/src/token-usage-service`, so no path edit is
needed when the clone location above is used. To run the timer while the current
user is logged out, enable lingering once:

```bash
sudo loginctl enable-linger "$USER"
```

## Linux with systemd

Use the checked-in user unit and timer. They run ingest at midnight and noon,
including one catch-up run after missed schedules.

```bash
go build -o "$HOME/.local/bin/token-usage-ingest" ./cmd/ingest
mkdir -p "$HOME/.config/systemd/user"
cp deploy/systemd/token-usage-ingest.service "$HOME/.config/systemd/user/"
cp deploy/systemd/token-usage-ingest.timer "$HOME/.config/systemd/user/"
```

Edit `~/.config/systemd/user/token-usage-ingest.service`. Set
`WorkingDirectory` and `EnvironmentFile` to the absolute repository path, then
enable the timer:

```bash
systemctl --user daemon-reload
systemctl --user enable --now token-usage-ingest.timer
systemctl --user list-timers token-usage-ingest.timer
systemctl --user start token-usage-ingest.service
journalctl --user -u token-usage-ingest.service -n 50
```

The final two commands perform a manual test and show its logs. On a headless
machine, enable user lingering if ingest must run while the user is logged out:

```bash
loginctl enable-linger "$USER"
```

### Linux dashboard

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

## macOS with launchd

Build binaries in a stable location:

```bash
mkdir -p "$HOME/.local/bin" "$HOME/Library/LaunchAgents"
go build -o "$HOME/.local/bin/token-usage-ingest" ./cmd/ingest
go build -o "$HOME/.local/bin/token-usage-dashboard" ./cmd/dashboard
```

Create `~/Library/LaunchAgents/com.example.token-usage-ingest.plist`. Replace
`USER` in all paths; launchd does not expand `$HOME` or `~` in plist values.

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>com.example.token-usage-ingest</string>
  <key>ProgramArguments</key>
  <array>
    <string>/Users/USER/.local/bin/token-usage-ingest</string>
    <string>ingest</string>
  </array>
  <key>WorkingDirectory</key>
  <string>/Users/USER/src/token-usage-service</string>
  <key>StartInterval</key>
  <integer>43200</integer>
  <key>RunAtLoad</key>
  <true/>
  <key>StandardOutPath</key>
  <string>/Users/USER/Library/Logs/token-usage-ingest.log</string>
  <key>StandardErrorPath</key>
  <string>/Users/USER/Library/Logs/token-usage-ingest.error.log</string>
</dict>
</plist>
```

Validate, enable, and test it:

```bash
plutil -lint "$HOME/Library/LaunchAgents/com.example.token-usage-ingest.plist"
launchctl bootstrap "gui/$(id -u)" "$HOME/Library/LaunchAgents/com.example.token-usage-ingest.plist"
launchctl kickstart -k "gui/$(id -u)/com.example.token-usage-ingest"
tail -n 50 "$HOME/Library/Logs/token-usage-ingest.error.log"
```

For the dashboard, copy the plist, change its label to
`com.example.token-usage-dashboard`, remove `StartInterval`, change the
executable, remove the `ingest` argument, and add `<key>KeepAlive</key><true/>`.

To remove either service:

```bash
launchctl bootout "gui/$(id -u)/com.example.token-usage-ingest"
```

## Windows with Task Scheduler

Run these commands from PowerShell. Build Windows executables and configure the
repository first:

```powershell
git clone https://github.com/seifhawamdeh/token-usage-service.git C:\Services\token-usage-service
Set-Location C:\Services\token-usage-service
Copy-Item .env.example .env
New-Item -ItemType Directory -Force .\bin | Out-Null
go build -o C:\Services\token-usage-service\bin\token-usage-ingest.exe .\cmd\ingest
go build -o C:\Services\token-usage-service\bin\token-usage-dashboard.exe .\cmd\dashboard
.\bin\token-usage-ingest.exe validate-config
.\bin\token-usage-ingest.exe migrate
.\bin\token-usage-ingest.exe ingest
```

Windows locations for provider databases vary by provider version. Set explicit
path overrides in `.env` when a Linux-style default does not exist, especially
`OPENCODE_DB_PATH` and `CURSOR_STATE_DB`.

Create the ingest task in **Task Scheduler**:

1. Select **Create Task**, name it `Token Usage Ingest`, and choose **Run only
   when user is logged on** so the task can read that user's provider data.
2. Add a daily trigger at midnight, enable **Repeat task every: 12 hours**, and
   set **for a duration of: Indefinitely**.
3. Add action **Start a program**. Program:
   `C:\Services\token-usage-service\bin\token-usage-ingest.exe`; arguments:
   `ingest`; start in: `C:\Services\token-usage-service`.
4. In **Settings**, enable **Run task as soon as possible after a scheduled
   start is missed** and disable parallel starts by choosing **Do not start a
   new instance**.
5. Run the task once. Confirm **Last Run Result** is `0x0`; inspect the Windows
   Task Scheduler operational log if it is not.

The **Start in** value is required: it lets the executable load the repository
`.env` and resolve relative paths consistently.

### Windows dashboard

Task Scheduler can also start the dashboard at logon:

1. Create task `Token Usage Dashboard` with an **At log on** trigger.
2. Set program to `token-usage-dashboard.exe` and **Start in** to the repository.
3. Disable the setting that stops the task after a time limit.
4. Run it and open `http://127.0.0.1:8080`.

For an always-on machine where no user logs in, use a dedicated Windows service
account that owns or can read the provider files. Do not run under
`LocalSystem`: its home directory does not contain the user's transcripts.

## Other Unix systems with cron

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
