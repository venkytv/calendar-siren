# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Meeting-siren is a production-ready Go daemon that plays loud audible alarms when it receives NATS messages about upcoming meetings. The daemon runs on desktop machines (macOS) or Raspberry Pi (Linux) connected to USB speakers, prioritizing reliability over features. The project is licensed under GNU AGPL v3.

### Core Functionality
- Subscribes to NATS messages containing meeting alerts with JSON payloads
- Plays configurable alarm sounds with cross-platform audio support (afplay on macOS, mpg123/aplay on Linux)
- Optional TTS announcements using native platform tools
- Event deduplication using UID hashing to prevent duplicate alarms
- Configurable volume, repeat intervals, and maximum repeat counts
- Work hours and quiet days filtering
- Simple snooze mechanism via file-based controls

## Development Architecture

This project follows Clean Architecture patterns with these key principles:
- **Clean Architecture**: Structure code into handlers/controllers, services/use cases, repositories/data access, and domain models
- **Domain-driven design** principles where applicable
- **Interface-driven development** with explicit dependency injection
- **Composition over inheritance** with small, purpose-specific interfaces
- All public functions should interact with interfaces, not concrete types

### Expected Project Structure
```
cmd/meeting-siren/   # main application entry point
internal/            # core application logic (not exposed externally)
  ├── config/        # configuration loading and validation
  ├── nats/          # NATS client and message handling
  ├── player/        # audio playback and TTS functionality
  ├── state/         # event deduplication and state management
  └── daemon/        # main daemon orchestration
pkg/                 # shared utilities and packages
configs/             # configuration schemas and examples
packaging/           # systemd service and launchd plist files
test/                # test utilities, mocks, and integration tests
```

### Message Format
The daemon expects JSON messages with the following structure:
```json
{
  "title": "Team Sync",
  "when": "2025-09-25T14:00:00+01:00",
  "lead": 10,
  "severity": "normal|critical"
}
```

## Development Commands

Standard Go commands for development:
- `go run cmd/meeting-siren/main.go` - Run the daemon
- `go build -o meeting-siren cmd/meeting-siren/main.go` - Build the application
- `go test ./...` - Run all tests
- `go test -cover ./...` - Run tests with coverage
- `go fmt ./...` - Format code
- `goimports -w .` - Format imports
- `golangci-lint run` - Run linting (if configured)

### Build Targets
The application should support cross-compilation for:
- `GOOS=linux GOARCH=arm64` - Raspberry Pi
- `GOOS=darwin GOARCH=arm64` - macOS Apple Silicon

### Configuration
Configuration can be provided via:
1. Environment variables (NATS_URL, NATS_SUBJECT, etc.)
2. YAML file at `/etc/meeting-siren.yaml` (overrides env vars)

Example usage:
```bash
NATS_URL=nats://user:pass@host:4222 NATS_SUBJECT=alerts.meeting.alarm ./meeting-siren
```

Test with:
```bash
nats pub alerts.meeting.alarm '{"title":"Design Review","when":"2025-09-25T15:00:00+01:00","lead":10}'
```

## Key Development Guidelines

### Architecture Patterns
- Apply Clean Architecture by structuring code into clear layers
- Use domain-driven design principles
- Prioritize interface-driven development with dependency injection
- Group code by feature when it improves clarity and cohesion

### Code Quality
- Write short, focused functions with single responsibility
- Always check and handle errors explicitly using wrapped errors: `fmt.Errorf("context: %w", err)`
- Avoid global state; use constructor functions for dependency injection
- Leverage Go's context propagation for request-scoped values and cancellations
- Use goroutines safely with proper synchronization

### Security & Resilience
- Apply input validation and sanitization rigorously
- Use secure defaults for configuration
- Implement NATS reconnection with exponential backoff
- Handle multiple concurrent messages without overlapping audio playback
- Exit with non-zero status on fatal configuration errors, otherwise keep running
- Implement proper error handling and logging for all operations

### Testing
- Write unit tests using table-driven patterns and parallel execution
- Mock external interfaces cleanly
- Separate fast unit tests from slower integration tests
- Ensure test coverage for every exported function

### Observability
- Use structured JSON logs to stdout for all events
- Include event UID, subject, retry attempts in log messages
- Support -version flag that prints version/commit/date (via ldflags)
- Log message receipt, deduplication decisions, and playback events
- Always attach context.Context for cancellation and timeout handling

## Technical Requirements

### Dependencies
- Go 1.22+ required
- `github.com/nats-io/nats.go` for NATS client
- YAML configuration support

### Audio Platform Support
- **macOS**: Use `afplay` for audio playback, optional TTS via `say`
- **Linux**: Use `mpg123` for MP3 or `aplay` for WAV, optional TTS via `espeak-ng`

### State Management
- Event deduplication using UID hash from `{title|when}`
- State directory at `/var/lib/meeting-siren` (configurable via STATE_DIR)
- Track last-fired timestamps to prevent duplicate alarms

### Configuration Keys
- `volume_pct`: Audio volume percentage
- `sounds[]`: Array of audio file paths to play
- `repeat_seconds`: Interval between alarm repeats
- `max_repeats`: Maximum number of repeats per event
- `tts_enabled`: Enable text-to-speech announcements
- `tts_template`: Template for TTS messages
- `work_hours`: Time range (e.g., "08:00-19:00")
- `quiet_days`: Days to skip (e.g., "Sat,Sun")

### Optional Features
- GPIO buzzer support via `GPIO_BUZZER_PIN` (Linux only)
- Snooze functionality via `SNOOZE_FILE` and `SNOOZE_MINUTES`

### Packaging
- Systemd service file: `packaging/meeting-siren.service`
- Launchd plist: `packaging/com.meeting-siren.plist`
- Example config: `configs/meeting-siren.example.yaml`

## Configuration Notes

- The project uses standard Go gitignore patterns
- Code coverage profiles and test artifacts are ignored
- Environment files (.env) are excluded from version control
- State files and audio assets are excluded from version control