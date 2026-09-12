# systemd user timer

Hourly one-shot ingest without a resident daemon.

## Install

```bash
go build -o ~/.local/bin/token-usage-ingest ./cmd/ingest

mkdir -p ~/.config/systemd/user
cp deploy/systemd/token-usage-ingest.service ~/.config/systemd/user/
cp deploy/systemd/token-usage-ingest.timer ~/.config/systemd/user/
```

Edit `WorkingDirectory` and `EnvironmentFile` in the `.service` if the repo path differs from `~/github-repos/token-usage-service`.

```bash
systemctl --user daemon-reload
systemctl --user enable --now token-usage-ingest.timer
systemctl --user list-timers | grep token-usage
```

## Logs

```bash
journalctl --user -u token-usage-ingest.service -n 50
```

## Manual run

```bash
systemctl --user start token-usage-ingest.service
# or
token-usage-ingest ingest
```
