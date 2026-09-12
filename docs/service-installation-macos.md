# macOS installation

macOS uses `launchd` instead of systemd or Task Scheduler. Ingest runs as a
per-user `LaunchAgent` on an interval; the dashboard runs as a `LaunchAgent`
kept alive in the background.

## 1. Install Go and the repository

```bash
mkdir -p "$HOME/.local/bin" "$HOME/Library/LaunchAgents"
git clone https://github.com/seifhawamdeh/token-usage-service.git ~/src/token-usage-service
cd ~/src/token-usage-service
cp .env.example .env
```

Install Go 1.25 or newer from [go.dev/dl](https://go.dev/dl/) or `brew install go`
if it is not already present. Confirm with `go version`.

## 2. Configure the service

Edit `.env` and set the database connection, a stable machine identity, and the
enabled adapters, matching the [Linux configuration example](service-installation-linux.md#3-configure-the-service).
Default discovery on macOS checks the same `~/.claude`, `~/.codex`, and
`~/.gemini` locations as Linux.

## 3. Build binaries in a stable location

```bash
go build -o "$HOME/.local/bin/token-usage-ingest" ./cmd/ingest
go build -o "$HOME/.local/bin/token-usage-dashboard" ./cmd/dashboard
~/.local/bin/token-usage-ingest validate-config
~/.local/bin/token-usage-ingest migrate
~/.local/bin/token-usage-ingest ingest
```

The manual ingest should finish with `errors=0` before installing the agent.

## 4. Install the ingest LaunchAgent

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

`StartInterval` is in seconds; `43200` is 12 hours.

Validate, enable, and test it:

```bash
plutil -lint "$HOME/Library/LaunchAgents/com.example.token-usage-ingest.plist"
launchctl bootstrap "gui/$(id -u)" "$HOME/Library/LaunchAgents/com.example.token-usage-ingest.plist"
launchctl kickstart -k "gui/$(id -u)/com.example.token-usage-ingest"
tail -n 50 "$HOME/Library/Logs/token-usage-ingest.error.log"
```

## Dashboard

Copy the plist above to `com.example.token-usage-dashboard.plist` and make
these changes:

- Change `Label` to `com.example.token-usage-dashboard`.
- Remove the `StartInterval` key.
- Change the executable to `token-usage-dashboard` and remove the `ingest`
  argument.
- Add `<key>KeepAlive</key><true/>` so launchd restarts it if it exits.

Bootstrap it the same way as the ingest agent, then open
`http://127.0.0.1:8080`.

## Removal

```bash
launchctl bootout "gui/$(id -u)/com.example.token-usage-ingest"
launchctl bootout "gui/$(id -u)/com.example.token-usage-dashboard"
```

Removing the agent does not remove PostgreSQL data or the built binaries;
delete those separately if needed.
