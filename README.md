# Meeting Siren 🚨

A production-ready Go daemon that plays loud audible alarms when it receives NATS messages about upcoming meetings. Perfect for ensuring you never miss important meetings when working remotely or in a busy environment.

## Features

- **Cross-platform audio support** - Works on macOS (`afplay`) and Linux (`mpg123`/`aplay`)
- **NATS messaging** - Reliable message delivery with automatic reconnection
- **Event deduplication** - Prevents duplicate alarms for the same meeting
- **Configurable scheduling** - Work hours and quiet days support
- **Text-to-speech** - Optional TTS announcements with customizable templates
- **Alarm repetition** - Configurable repeat intervals and limits
- **GPIO buzzer support** - Hardware buzzer support for Raspberry Pi
- **Snooze functionality** - Temporary alarm suppression
- **Clean Architecture** - Well-structured, testable, and maintainable code
- **Comprehensive logging** - Structured JSON logs for monitoring
- **Service integration** - Systemd (Linux) and Launchd (macOS) support

## Quick Start

### Prerequisites

- Go 1.22+ for building from source
- NATS server running and accessible
- Audio system with `afplay` (macOS) or `mpg123`/`aplay` (Linux)
- Optional: `espeak-ng` for TTS on Linux

### Installation

1. **Download and build:**
```bash
git clone https://github.com/meeting-siren/meeting-siren.git
cd meeting-siren
go build -o meeting-siren cmd/meeting-siren/main.go
```

2. **Install as system service:**
```bash
sudo ./packaging/install.sh
```

3. **Configure the daemon:**
```bash
sudo cp /etc/meeting-siren.yaml.example /etc/meeting-siren.yaml
sudo nano /etc/meeting-siren.yaml
```

4. **Start the service:**

**Linux (systemd):**
```bash
sudo systemctl start meeting-siren
sudo systemctl status meeting-siren
```

**macOS (launchd):**
```bash
sudo launchctl start com.meeting-siren
sudo launchctl list | grep meeting-siren
```

### Basic Usage

**Environment Variables:**
```bash
export NATS_URL="nats://localhost:4222"
export NATS_SUBJECT="alerts.meeting.alarm"
export SOUNDS="/path/to/alarm.wav"
export VOLUME_PCT="80"
./meeting-siren
```

**Test the daemon:**
```bash
# Publish a test alert
nats pub alerts.meeting.alarm '{"title":"Test Meeting","when":"2025-09-25T15:00:00+01:00","lead":5}'
```

## Configuration

Configuration can be provided via environment variables or YAML file. YAML file settings override environment variables.

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `NATS_URL` | NATS server URL | `nats://localhost:4222` |
| `NATS_SUBJECT` | NATS subject to subscribe | `alerts.meeting.alarm` |
| `VOLUME_PCT` | Audio volume (0-100) | `80` |
| `SOUNDS` | Comma-separated audio files | `` |
| `REPEAT_SECONDS` | Seconds between repeats | `30` |
| `MAX_REPEATS` | Maximum repeats per event | `3` |
| `TTS_ENABLED` | Enable text-to-speech | `false` |
| `TTS_TEMPLATE` | TTS message template | `Meeting alert: {{.Title}} in {{.Lead}} minutes` |
| `WORK_HOURS` | Active hours (HH:MM-HH:MM) | `08:00-19:00` |
| `QUIET_DAYS` | Days to skip (comma-separated) | `Sat,Sun` |
| `STATE_DIR` | State directory for deduplication | `/var/lib/meeting-siren` |
| `GPIO_BUZZER_PIN` | GPIO pin for buzzer (Linux only) | `` |
| `SNOOZE_FILE` | Snooze file path | `` |
| `SNOOZE_MINUTES` | Snooze duration | `0` |

### YAML Configuration

See [`configs/meeting-siren.example.yaml`](configs/meeting-siren.example.yaml) for a complete configuration example.

### Message Format

The daemon expects JSON messages with this structure:

```json
{
  "title": "Team Sync",
  "when": "2025-09-25T14:00:00+01:00",
  "lead": 10,
  "severity": "normal"
}
```

## Architecture

The application follows Clean Architecture principles:

```
cmd/meeting-siren/          # Application entry point
internal/
├── domain/                 # Domain models and interfaces
├── config/                 # Configuration loading
├── daemon/                 # Main orchestration logic
├── nats/                   # NATS client implementation
├── player/                 # Audio playback and TTS
└── state/                  # Event deduplication and scheduling
pkg/                        # Shared utilities
└── logger/                 # Structured JSON logging
test/                       # Unit and integration tests
```

## Development

### Building

```bash
# Development build
go build -o meeting-siren cmd/meeting-siren/main.go

# Production build with version info
go build -ldflags "-X main.version=v1.0.0 -X main.commit=$(git rev-parse HEAD) -X main.buildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)" -o meeting-siren cmd/meeting-siren/main.go

# Cross-compilation for Raspberry Pi
GOOS=linux GOARCH=arm64 go build -o meeting-siren-linux-arm64 cmd/meeting-siren/main.go
```

### Testing

```bash
# Run unit tests
go test ./...

# Run with coverage
go test -cover ./...

# Run integration tests (requires NATS server)
go test -tags=integration ./test/integration/

# Run specific test
go test -run TestConfigLoader ./internal/config/
```

### Code Quality

```bash
# Format code
go fmt ./...
goimports -w .

# Lint (requires golangci-lint)
golangci-lint run

# Vet
go vet ./...
```

## License

This project is licensed under the GNU Affero General Public License v3.0.

## Contributing

1. Fork the repository
2. Create a feature branch
3. Add tests for your changes
4. Ensure all tests pass
5. Submit a pull request