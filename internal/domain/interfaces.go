package domain

import (
	"context"
	"time"
)

// ConfigLoader loads configuration from various sources
type ConfigLoader interface {
	Load() (*Config, error)
}

// MessageSubscriber handles NATS message subscription
type MessageSubscriber interface {
	Subscribe(ctx context.Context, subject string, handler func(*MeetingAlert)) error
	Close() error
}

// AudioPlayer handles audio playback functionality
type AudioPlayer interface {
	Play(ctx context.Context, soundFiles []string) error
	SetVolume(ctx context.Context, percent int) error
	PlayTTS(ctx context.Context, message string) error
}

// StateManager handles event deduplication and persistence
type StateManager interface {
	ShouldFire(event *AlarmEvent) (bool, error)
	RecordFired(event *AlarmEvent) error
	Cleanup(olderThan time.Duration) error
}

// Scheduler determines if alarms should fire based on time/day rules
type Scheduler interface {
	ShouldFire(when time.Time) bool
}

// Logger provides structured logging capabilities
type Logger interface {
	Info(msg string, fields map[string]interface{})
	Error(msg string, err error, fields map[string]interface{})
	Debug(msg string, fields map[string]interface{})
}

// Daemon orchestrates all components
type Daemon interface {
	Start(ctx context.Context) error
	Stop() error
}
