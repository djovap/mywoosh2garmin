#!/bin/zsh
# Invoked by mywhoosh-watcher after the final MyWhoosh process exits.
# It is deliberately not a poller; macOS lifecycle events trigger it.

SCRIPT_DIR="$(cd -- "$(dirname -- "$0")" && pwd -P)"
SYNC_BINARY="$SCRIPT_DIR/mywhoosh2garmin"

# launchd starts non-interactive shells, which do not read ~/.zshrc by default.
# Load the user's exported credentials before executing the sync binary.
if [[ -r "$HOME/.zshrc" ]]; then
  source "$HOME/.zshrc"
fi

set -u

if [[ ! -x "$SYNC_BINARY" ]]; then
  print -u2 "Sync binary is missing or not executable: $SYNC_BINARY"
  print -u2 "Build it with: (cd $SCRIPT_DIR && go build -o mywhoosh2garmin .)"
  exit 1
fi

print "MyWhoosh closed. Starting MyWhoosh sync..."
exec "$SYNC_BINARY"
