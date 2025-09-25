package domain

import (
	"time"
)

// MeetingAlert represents an incoming meeting alert message
type MeetingAlert struct {
	Title    string    `json:"title"`
	When     time.Time `json:"when"`
	Lead     int       `json:"lead"`
	Severity string    `json:"severity"`
}

// EventUID generates a unique identifier for deduplication
func (m *MeetingAlert) EventUID() string {
	return m.Title + "|" + m.When.Format(time.RFC3339)
}

// AlarmEvent represents a processed alarm event with metadata
type AlarmEvent struct {
	Alert     *MeetingAlert
	UID       string
	Timestamp time.Time
}

// Config represents the application configuration
type Config struct {
	// NATS configuration
	NATSUrl     string `yaml:"nats_url"`
	NATSSubject string `yaml:"nats_subject"`

	// Audio configuration
	VolumePct int      `yaml:"volume_pct"`
	Sounds    []string `yaml:"sounds"`

	// Repeat configuration
	RepeatSeconds int `yaml:"repeat_seconds"`
	MaxRepeats    int `yaml:"max_repeats"`

	// TTS configuration
	TTSEnabled  bool   `yaml:"tts_enabled"`
	TTSTemplate string `yaml:"tts_template"`

	// Schedule configuration
	WorkHours string `yaml:"work_hours"`
	QuietDays string `yaml:"quiet_days"`

	// State management
	StateDir string `yaml:"state_dir"`

	// Optional features
	GPIOBuzzerPin *int   `yaml:"gpio_buzzer_pin,omitempty"`
	SnoozeFile    string `yaml:"snooze_file,omitempty"`
	SnoozeMinutes int    `yaml:"snooze_minutes,omitempty"`
}

// Default configuration values
var DefaultConfig = Config{
	NATSUrl:       "nats://localhost:4222",
	NATSSubject:   "alerts.meeting.alarm",
	VolumePct:     80,
	Sounds:        []string{},
	RepeatSeconds: 30,
	MaxRepeats:    3,
	TTSEnabled:    false,
	TTSTemplate:   "Meeting alert: {{.Title}} in {{.Lead}} minutes",
	WorkHours:     "08:00-19:00",
	QuietDays:     "Sat,Sun",
	StateDir:      "/var/lib/meeting-siren",
}
