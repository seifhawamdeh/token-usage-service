# systemd user services

The timer runs one-shot ingest every 12 hours. The dashboard service keeps the
HTTP dashboard running.

For shared prerequisites, dashboard setup, other operating systems, upgrades,
and removal, see the [cross-platform service installation guide](../../docs/service-installation.md).

## Install

```bash
go build -o ~/.local/bin/token-usage-ingest ./cmd/ingest

mkdir -p ~/.config/systemd/user
cp deploy/systemd/token-usage-ingest.service ~/.config/systemd/user/
cp deploy/systemd/token-usage-ingest.timer ~/.config/systemd/user/
cp deploy/systemd/token-usage-dashboard.service ~/.config/systemd/user/
```

Edit `WorkingDirectory` and `EnvironmentFile` in the `.service` so they point at **your** clone of this repo (the checked-in unit uses a placeholder path).

```bash
systemctl --user daemon-reload
systemctl --user enable --now token-usage-ingest.timer
systemctl --user enable --now token-usage-dashboard.service
systemctl --user list-timers | grep token-usage
```

## Logs

```bash
journalctl --user -u token-usage-ingest.service -n 50
journalctl --user -u token-usage-dashboard.service -n 50
```

## Manual run

```bash
systemctl --user start token-usage-ingest.service
# or
token-usage-ingest ingest
```
