# MyWhoosh2Garmin

A command-line tool that syncs today's [MyWhoosh](https://www.mywhoosh.com/) indoor cycling activities to [Garmin Connect](https://connect.garmin.com/) — with full training effect, VO2max, and performance stats support.

It downloads activities directly from your MyWhoosh account, so it does not need to run on the same machine as MyWhoosh.


## Why?

MyWhoosh exports FIT files, but they have issues that prevent Garmin from fully processing them:

- **Missing averages** — average power, heart rate, and cadence are not set in the session data
- **Bogus temperature** — every record contains a fake temperature reading
- **Unknown device** — Garmin ignores training effect and VO2max from unknown manufacturers

MyWhoosh2Garmin fixes all of this automatically:

| Problem | Fix |
|---|---|
| Missing avg power / HR / cadence | Calculated from ride records |
| Fake temperature data | Stripped from all records |
| Device identity | Set to your registered Forerunner 265 |

The result: your indoor rides show up on Garmin Connect just like a native Garmin recording, complete with **Training Effect**, **VO2max updates**, **Training Load**, and **Training Status**.

## Run locally

This fork is intended to be built and run on the local machine.

## Usage

Create a `.env` file in the project directory with the required settings:

```dotenv
MYWHOOSH_EMAIL="you@example.com"
MYWHOOSH_PASSWORD="your-mywhoosh-password"
GARMIN_EMAIL="you@example.com"
GARMIN_PASSWORD="your-garmin-password"
```

Then run the program:

```bash
go run .
```

The `.env` file is ignored by Git. Explicitly exported environment variables override values in `.env`.

This app is configured for a **Forerunner 265** (FIT product ID `4257`). After signing in, it finds the matching registered watch by FIT product ID or Garmin Connect's exact model name, then writes that account-specific FIT unit ID into the formatted activity. It stops if the watch cannot be found—there is no configurable product or fallback serial number to spoof.

The tool finds activities whose start time falls on the current local calendar day, then downloads, fixes, and uploads each unsynced activity. It exits successfully when there are no activities to sync.

Sessions and the synced-activity tracker are stored locally in `~/.mywhoosh2garmin/`. Credentials are never written there. A Garmin duplicate is treated as already synced. The tracker is `~/.mywhoosh2garmin/synced.json`; Garmin session tokens are `oauth1_token.json` and `oauth2_token.json` in that same directory. The formatted FIT file is written only to a temporary `mywhoosh2garmin-*` directory under the system temp directory during upload, then deleted.

## Automatically sync after MyWhoosh closes (macOS)

Install the event-driven watcher once:

```bash
./install-mywhoosh-watcher.zsh
```

It builds the Go sync binary and a small macOS `NSWorkspace` watcher, then installs the watcher as a per-user LaunchAgent. It starts at login and receives app launch/termination events—there is no polling.

- When MyWhoosh (`com.whoosh.whooshgame`) opens, it launches `BikeControl`.
- When the final MyWhoosh process exits, it closes BikeControl, removes both its pinned and recent-app Dock entries, then immediately runs `sync-when-mywhoosh-closes.zsh`, which executes `./mywhoosh2garmin`.

The sync hook sources `~/.zshrc` before running, so credentials exported there are available in the LaunchAgent environment. Each sync posts a macOS notification with its Garmin outcome (successful, skipped, or failed). Logs are written to `~/.mywhoosh2garmin/watcher.log` and `~/.mywhoosh2garmin/watcher-error.log`.

## Building from Source

### Prerequisites

- Go 1.24+

### Build

```bash
# Build a native executable
go build -o mywhoosh2garmin .

# Or build and start it directly
go run .
```

### Run tests

```bash
go test ./...
```

## How It Works

```
  ┌─────────────────────┐
  │  MyWhoosh Web API   │
  │  Login + List       │
  │  Download FIT file  │
  └─────────┬───────────┘
            │
            ▼
  ┌─────────────────────┐
  │  Decode FIT (V2)    │
  │  Fix session avgs   │
  │  Strip temperature  │
  │  Set Garmin gear    │
  │  Encode FIT (V2)    │
  └─────────┬───────────┘
            │
            ▼
  ┌─────────────────────┐
  │  Garmin SSO Login   │
  │  OAuth1 → OAuth2    │
  │  Upload FIT file    │
  └─────────────────────┘
            │
            ▼
     Garmin Connect
  (Training Effect ✓)
  (VO2max ✓)
  (Training Load ✓)
```

## Credits

- [garth](https://github.com/matin/garth) by matin — Garmin SSO authentication reference
- [muktihari/fit](https://github.com/muktihari/fit) — FIT SDK for Go

## License

GPLv3 — see [LICENSE](LICENSE).
