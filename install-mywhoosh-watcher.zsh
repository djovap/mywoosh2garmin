#!/bin/zsh
# Build and install the event-driven MyWhoosh watcher as a per-user LaunchAgent.

set -eu

PROJECT_DIR="$(cd -- "$(dirname -- "$0")" && pwd -P)"
LABEL="com.mywhoosh2garmin.watcher"
UID="$(id -u)"
WATCHER="$PROJECT_DIR/mywhoosh-watcher"
PLIST="$HOME/Library/LaunchAgents/$LABEL.plist"
STATE_DIR="$HOME/.mywhoosh2garmin"

mkdir -p "$HOME/Library/LaunchAgents" "$STATE_DIR"
chmod +x "$PROJECT_DIR/sync-when-mywhoosh-closes.zsh"

(
  cd "$PROJECT_DIR"
  go build -o mywhoosh2garmin .
  swiftc mywhoosh-watcher.swift -framework AppKit -o "$WATCHER"
)

cat > "$PLIST" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>$LABEL</string>
  <key>ProgramArguments</key>
  <array>
    <string>$WATCHER</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>ThrottleInterval</key>
  <integer>10</integer>
  <key>StandardOutPath</key>
  <string>$STATE_DIR/watcher.log</string>
  <key>StandardErrorPath</key>
  <string>$STATE_DIR/watcher-error.log</string>
</dict>
</plist>
EOF

# Replace an existing copy, then start the agent in this GUI session.
launchctl bootout "gui/$UID" "$PLIST" 2>/dev/null || true
launchctl bootstrap "gui/$UID" "$PLIST"
launchctl kickstart -k "gui/$UID/$LABEL"

print "Installed $LABEL. It now starts at login and reacts to MyWhoosh launch/quit events."
print "Logs: $STATE_DIR/watcher.log and $STATE_DIR/watcher-error.log"
