#!/usr/bin/env bash
# Interactive installer for token-usage-service (Linux).
# Builds binaries, lets you enable/disable vendor adapters, configures .env,
# runs migration, and optionally installs the systemd timer + dashboard service.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
BIN_DIR="$HOME/.local/bin"
UNIT_DIR="$HOME/.config/systemd/user"
ENV_FILE="$REPO_DIR/.env"

ALL_VENDORS=(claude-code codex opencode github-copilot gemini antigravity cursor cursor-usage)

bold() { printf '\033[1m%s\033[0m\n' "$1"; }
info() { printf '  %s\n' "$1"; }
err()  { printf 'error: %s\n' "$1" >&2; }

require_cmd() {
  command -v "$1" >/dev/null 2>&1
}

step_go() {
  bold "1/6 Go toolchain"
  if require_cmd go; then
    info "found: $(go version)"
    return
  fi
  err "go not found. Install Go 1.25+ first: https://go.dev/dl/"
  exit 1
}

step_vendors() {
  bold "2/6 Choose adapters (components) to enable"
  local selected=()
  for v in "${ALL_VENDORS[@]}"; do
    local reply
    read -r -p "  enable ${v}? [Y/n] " reply </dev/tty
    case "$reply" in
      [nN]*) ;;
      *) selected+=("$v") ;;
    esac
  done
  if [ "${#selected[@]}" -eq 0 ]; then
    err "no adapters selected, enabling all"
    selected=("${ALL_VENDORS[@]}")
  fi
  ENABLED_VENDORS_CSV="$(IFS=,; echo "${selected[*]}")"
}

step_env() {
  bold "3/6 Configuration"
  local db_url host_id
  local default_host
  default_host="$(hostname)"
  read -r -p "  DATABASE_URL [postgres://USER:PASSWORD@localhost:5432/token_usage?sslmode=disable]: " db_url </dev/tty
  db_url="${db_url:-postgres://USER:PASSWORD@localhost:5432/token_usage?sslmode=disable}"
  read -r -p "  HOST_ID [$default_host]: " host_id </dev/tty
  host_id="${host_id:-$default_host}"

  cat > "$ENV_FILE" <<EOF
DATABASE_URL=$db_url
HOST_ID=$host_id
ENABLED_VENDORS=$ENABLED_VENDORS_CSV
SINK=postgres
EOF
  chmod 600 "$ENV_FILE"
  info "wrote $ENV_FILE (mode 600)"
}

step_build() {
  bold "4/6 Build"
  mkdir -p "$BIN_DIR"
  (cd "$REPO_DIR" && go build -o "$BIN_DIR/token-usage-ingest" ./cmd/ingest)
  (cd "$REPO_DIR" && go build -o "$BIN_DIR/token-usage-dashboard" ./cmd/dashboard)
  info "built $BIN_DIR/token-usage-ingest"
  info "built $BIN_DIR/token-usage-dashboard"
}

step_migrate() {
  bold "5/6 Validate + migrate + first ingest"
  (cd "$REPO_DIR" && "$BIN_DIR/token-usage-ingest" validate-config)
  (cd "$REPO_DIR" && "$BIN_DIR/token-usage-ingest" migrate)
  (cd "$REPO_DIR" && "$BIN_DIR/token-usage-ingest" ingest)
}

step_services() {
  bold "6/6 Scheduling"
  if ! require_cmd systemctl; then
    info "no systemd; add this to crontab -e instead:"
    info "0 */12 * * * cd $REPO_DIR && $BIN_DIR/token-usage-ingest ingest"
    return
  fi

  local reply
  read -r -p "  install systemd user timer for ingest (every 12h)? [Y/n] " reply </dev/tty
  if [[ ! "$reply" =~ ^[nN] ]]; then
    mkdir -p "$UNIT_DIR"
    sed -e "s|%h/src/token-usage-service|$REPO_DIR|g" \
      "$REPO_DIR/deploy/systemd/token-usage-ingest.service" > "$UNIT_DIR/token-usage-ingest.service"
    cp "$REPO_DIR/deploy/systemd/token-usage-ingest.timer" "$UNIT_DIR/token-usage-ingest.timer"
    systemctl --user daemon-reload
    systemctl --user enable --now token-usage-ingest.timer
    info "timer installed and enabled"
  fi

  read -r -p "  install dashboard as a systemd user service? [y/N] " reply </dev/tty
  if [[ "$reply" =~ ^[yY] ]]; then
    mkdir -p "$UNIT_DIR"
    sed -e "s|%h/src/token-usage-service|$REPO_DIR|g" \
      "$REPO_DIR/deploy/systemd/token-usage-dashboard.service" > "$UNIT_DIR/token-usage-dashboard.service"
    systemctl --user daemon-reload
    systemctl --user enable --now token-usage-dashboard.service
    info "dashboard running: http://127.0.0.1:8080"
  fi
}

bold "token-usage-service installer"
step_go
step_vendors
step_env
step_build
step_migrate
step_services

bold "Done."
info "ingest:    $BIN_DIR/token-usage-ingest ingest"
info "dashboard: $BIN_DIR/token-usage-dashboard"
info "config:    $ENV_FILE"
