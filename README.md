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
| MyWhoosh device identity | Spoofed to Garmin Fenix 6S Pro |

The result: your indoor rides show up on Garmin Connect just like a native Garmin recording, complete with **Training Effect**, **VO2max updates**, **Training Load**, and **Training Status**.

## Run locally

This fork is intended to be built and run on the local machine.

## Usage

Set all four credentials as environment variables, then run the program:

```bash
export MYWHOOSH_EMAIL="you@example.com"
export MYWHOOSH_PASSWORD="your-mywhoosh-password"
export GARMIN_EMAIL="you@example.com"
export GARMIN_PASSWORD="your-garmin-password"

go run .
```

The tool finds activities whose start time falls on the current local calendar day, then downloads, fixes, and uploads each unsynced activity. It exits successfully when there are no activities to sync.

Sessions and the synced-activity tracker are stored locally in `~/.mywhoosh2garmin/`. Credentials are never written there. A Garmin duplicate is treated as already synced.

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
  │  Spoof → Fenix 6S   │
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
