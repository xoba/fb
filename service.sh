#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"

# Manage local_md as a macOS LaunchAgent: a per-user service that starts at
# login, restarts if it crashes, and can be stopped/started/redeployed at
# will. Logs go to ~/Library/Logs/localmd.log.
#
# Usage:
#   ./service.sh install     build, write the LaunchAgent plist, and start
#   ./service.sh redeploy    rebuild the binary and restart the service
#   ./service.sh start       start (or load) the service
#   ./service.sh stop        stop until 'start' or next login
#   ./service.sh restart     restart the running service
#   ./service.sh status      show launchd state and probe the HTTP endpoint
#   ./service.sh logs        tail the log file
#   ./service.sh uninstall   stop and remove the LaunchAgent
#
# The serve root defaults to /; override at install time:
#   LOCALMD_ROOT=~/notes ./service.sh install    (serve ~/notes instead of /)

LABEL="com.xoba.localmd"
PLIST="$HOME/Library/LaunchAgents/$LABEL.plist"
DOMAIN="gui/$(id -u)"
REPO="$PWD"
BIN="$REPO/localmd"
LOG="$HOME/Library/Logs/localmd.log"
ROOT="${LOCALMD_ROOT:-/}"
TAGS="sqlite_dbstat sqlite_fts5"

build() {
    # Build to a temp name and rename over, so a running binary is replaced
    # atomically instead of truncated in place.
    go build -tags "$TAGS" -o "$BIN.new" .
    mv -f "$BIN.new" "$BIN"
}

write_plist() {
    mkdir -p "$(dirname "$PLIST")"
    cat >"$PLIST" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>$LABEL</string>
    <key>ProgramArguments</key>
    <array>
        <string>$BIN</string>
        <string>$ROOT</string>
    </array>
    <key>WorkingDirectory</key>
    <string>$REPO</string>
    <key>EnvironmentVariables</key>
    <dict>
        <key>PATH</key>
        <string>/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin</string>
    </dict>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>ThrottleInterval</key>
    <integer>5</integer>
    <key>StandardOutPath</key>
    <string>$LOG</string>
    <key>StandardErrorPath</key>
    <string>$LOG</string>
</dict>
</plist>
EOF
}

loaded() { launchctl print "$DOMAIN/$LABEL" >/dev/null 2>&1; }

start() {
    if loaded; then
        launchctl kickstart "$DOMAIN/$LABEL"
    else
        launchctl bootstrap "$DOMAIN" "$PLIST"
    fi
    echo "started $LABEL"
}

stop() {
    if loaded; then
        launchctl bootout "$DOMAIN/$LABEL"
        echo "stopped $LABEL (starts again at next login, or with './service.sh start')"
    else
        echo "$LABEL is not running"
    fi
}

status() {
    if loaded; then
        launchctl print "$DOMAIN/$LABEL" | grep -E '\b(state|pid|path|last exit code) = '
    else
        echo "$LABEL is not loaded"
    fi
    if curl -fsS -o /dev/null --max-time 3 http://localhost:3030/; then
        echo "http://localhost:3030/ is responding"
    else
        echo "http://localhost:3030/ is NOT responding"
    fi
}

case "${1:-}" in
install)
    build
    write_plist
    if loaded; then
        launchctl bootout "$DOMAIN/$LABEL"
    fi
    launchctl bootstrap "$DOMAIN" "$PLIST"
    echo "installed and started $LABEL (serving $ROOT, logs in $LOG)"
    ;;
redeploy)
    build
    if loaded; then
        launchctl kickstart -k "$DOMAIN/$LABEL"
        echo "rebuilt and restarted $LABEL"
    else
        start
    fi
    ;;
start) start ;;
stop) stop ;;
restart)
    if loaded; then
        launchctl kickstart -k "$DOMAIN/$LABEL"
        echo "restarted $LABEL"
    else
        start
    fi
    ;;
status) status ;;
logs) exec tail -n 50 -f "$LOG" ;;
uninstall)
    if loaded; then
        launchctl bootout "$DOMAIN/$LABEL"
    fi
    rm -f "$PLIST"
    echo "uninstalled $LABEL"
    ;;
*)
    grep '^#   ' "$0" | sed 's/^#   //'
    exit 2
    ;;
esac
