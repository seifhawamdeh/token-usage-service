#!/usr/bin/env bash
# Interactive installer for token-usage-service (macOS).
# Builds binaries, configures .env, runs migration, and optionally installs
# launchd agents for scheduled ingestion and the dashboard.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
BIN_DIR="$HOME/.local/bin"
AGENT_DIR="$HOME/Library/LaunchAgents"
LOG_DIR="$HOME/Library/Logs"
ENV_FILE="$REPO_DIR/.env"
INGEST_LABEL="com.token-usage-service.ingest"
DASHBOARD_LABEL="com.token-usage-service.dashboard"

ALL_VENDORS=(claude-code codex opencode github-copilot gemini antigravity cursor cursor-usage)

bold() { printf '\033[1m%s\033[0m\n' "$1"; }
info() { printf '  %s\n' "$1"; }
err()  { printf 'error: %s\n' "$1" >&2; }
require_cmd() { command -v "$1" >/dev/null 2>&1; }

xml_escape() {
  printf '%s' "$1" | sed -e 's/&/\&amp;/g' -e 's/</\&lt;/g' -e 's/>/\&gt;/g' -e 's/"/\&quot;/g' -e "s/'/\&apos;/g"
}

step_platform() {
  bold "1/6 Platform + Go toolchain"
  if [ "$(uname -s)" != "Darwin" ]; then
    err "this installer requires macOS"
    exit 1
  fi
  if ! require_cmd go; then
    err "go not found. Install Go 1.25+ from https://go.dev/dl/ or run: brew install go"
    exit 1
  fi
  info "found: $(go version)"
}

step_vendors() {
  bold "2/6 Choose adapters (components) to enable"
  local selected=() v reply
  for v in "${ALL_VENDORS[@]}"; do
    read -r -p "  enable ${v}? [Y/n] " reply </dev/tty
    case "$reply" in [nN]*) ;; *) selected+=("$v") ;; esac
  done
  if [ "${#selected[@]}" -eq 0 ]; then
    err "no adapters selected, enabling all"
    selected=("${ALL_VENDORS[@]}")
  fi
  ENABLED_VENDORS_CSV="$(IFS=,; echo "${selected[*]}")"
}

step_env() {
  bold "3/6 Configuration"
  local db_url host_id default_host
  default_host="$(scutil --get LocalHostName 2>/dev/null || hostname)"
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
  (cd "$REPO_DIR" && go build -o "$BIN_DIR/token-usage-loadworkstyle" ./cmd/loadworkstyle)
  (cd "$REPO_DIR" && go build -o "$BIN_DIR/token-usage-dashboard" ./cmd/dashboard)
  info "built binaries in $BIN_DIR"
}

step_migrate() {
  bold "5/6 Validate + migrate + first ingest"
  (cd "$REPO_DIR" && "$BIN_DIR/token-usage-ingest" validate-config)
  (cd "$REPO_DIR" && "$BIN_DIR/token-usage-ingest" migrate)
  (cd "$REPO_DIR" && "$BIN_DIR/token-usage-ingest" ingest)
  (cd "$REPO_DIR" && "$BIN_DIR/token-usage-loadworkstyle" \
    claude_history="$HOME/.claude/history.jsonl" \
    claude_caveman_history="$HOME/.claude/.caveman-history.jsonl" \
    codex_history="$HOME/.codex/history.jsonl" \
    codex_session_index="$HOME/.codex/session_index.jsonl")
}

unload_agent() {
  launchctl bootout "gui/$(id -u)/$1" >/dev/null 2>&1 || true
}

step_services() {
  bold "6/6 Scheduling"
  local reply repo_xml run_xml dashboard_xml run_script
  mkdir -p "$AGENT_DIR" "$LOG_DIR"
  repo_xml="$(xml_escape "$REPO_DIR")"
  dashboard_xml="$(xml_escape "$BIN_DIR/token-usage-dashboard")"

  run_script="$BIN_DIR/token-usage-run.sh"
  cat > "$run_script" <<EOF
#!/bin/sh
set -e
"$BIN_DIR/token-usage-ingest" ingest
"$BIN_DIR/token-usage-loadworkstyle" \\
  claude_history="\$HOME/.claude/history.jsonl" \\
  claude_caveman_history="\$HOME/.claude/.caveman-history.jsonl" \\
  codex_history="\$HOME/.codex/history.jsonl" \\
  codex_session_index="\$HOME/.codex/session_index.jsonl"
EOF
  chmod +x "$run_script"
  run_xml="$(xml_escape "$run_script")"

  read -r -p "  install launchd agent for ingest (every 30m)? [Y/n] " reply </dev/tty
  if [[ ! "$reply" =~ ^[nN] ]]; then
    unload_agent "$INGEST_LABEL"
    cat > "$AGENT_DIR/$INGEST_LABEL.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>Label</key><string>$INGEST_LABEL</string>
  <key>ProgramArguments</key><array><string>$run_xml</string></array>
  <key>WorkingDirectory</key><string>$repo_xml</string>
  <key>StartInterval</key><integer>1800</integer>
  <key>RunAtLoad</key><true/>
  <key>StandardOutPath</key><string>$(xml_escape "$LOG_DIR/token-usage-ingest.log")</string>
  <key>StandardErrorPath</key><string>$(xml_escape "$LOG_DIR/token-usage-ingest.error.log")</string>
</dict></plist>
EOF
    plutil -lint "$AGENT_DIR/$INGEST_LABEL.plist"
    launchctl bootstrap "gui/$(id -u)" "$AGENT_DIR/$INGEST_LABEL.plist"
    info "ingest agent installed and loaded"
  fi

  read -r -p "  install dashboard as a launchd agent? [y/N] " reply </dev/tty
  if [[ "$reply" =~ ^[yY] ]]; then
    unload_agent "$DASHBOARD_LABEL"
    cat > "$AGENT_DIR/$DASHBOARD_LABEL.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>Label</key><string>$DASHBOARD_LABEL</string>
  <key>ProgramArguments</key><array><string>$dashboard_xml</string></array>
  <key>WorkingDirectory</key><string>$repo_xml</string>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>StandardOutPath</key><string>$(xml_escape "$LOG_DIR/token-usage-dashboard.log")</string>
  <key>StandardErrorPath</key><string>$(xml_escape "$LOG_DIR/token-usage-dashboard.error.log")</string>
</dict></plist>
EOF
    plutil -lint "$AGENT_DIR/$DASHBOARD_LABEL.plist"
    launchctl bootstrap "gui/$(id -u)" "$AGENT_DIR/$DASHBOARD_LABEL.plist"
    info "dashboard running: http://127.0.0.1:8080"
  fi
}

bold "token-usage-service installer for macOS"
step_platform
step_vendors
step_env
step_build
step_migrate
step_services

bold "Done."
info "ingest:      $BIN_DIR/token-usage-ingest ingest"
info "loadworkstyle: $BIN_DIR/token-usage-loadworkstyle"
info "dashboard:   $BIN_DIR/token-usage-dashboard"
info "config:    $ENV_FILE"
