package domain

import (
	"time"
)

// MeetingAlert represents an incoming meeting alert message
type MeetingAlert struct {
	Title               string    `json:"title"`
	When                time.Time `json:"when"`
	Lead                int       `json:"lead"`
	Severity            string    `json:"severity"`
	IsFinalNotification bool      `json:"is_final_notification"`
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
	VolumePct               int      `yaml:"volume_pct"`
	Sounds                  []string `yaml:"sounds"`
	FinalNotificationSounds []string `yaml:"final_notification_sounds,omitempty"` // Sounds to use when is_final_notification is true
	AudioOutputDriver       string   `yaml:"audio_output_driver,omitempty"`       // mpg123 output driver (e.g., "alsa", "pulse")
	AudioDevice             string   `yaml:"audio_device,omitempty"`              // ALSA device for mpg123 (e.g., "hw:2,0", "plughw:2,0", "default")
	AmixerCard              string   `yaml:"amixer_card,omitempty"`               // ALSA card for amixer (e.g., "0", "2", "default")

	// Repeat configuration
	RepeatSeconds int `yaml:"repeat_seconds"`
	MaxRepeats    int `yaml:"max_repeats"`

	// TTS configuration
	TTSEnabled                   bool    `yaml:"tts_enabled"`
	TTSCommand                   string  `yaml:"tts_command,omitempty"`                     // Custom TTS command (accepts input on stdin). Falls back to say/espeak-ng/espeak if not provided
	TTSTemplate                  string  `yaml:"tts_template"`
	FinalNotificationTTSTemplate *string `yaml:"final_notification_tts_template,omitempty"` // TTS template for final notifications (nil=use default, ""=skip TTS, "template"=use template)

	// Schedule configuration
	WorkHours string `yaml:"work_hours"`
	QuietDays string `yaml:"quiet_days"`

	// State management
	StateDir string `yaml:"state_dir"`

	// Optional features
	GPIOBuzzerPin *int   `yaml:"gpio_buzzer_pin,omitempty"`
	SnoozeFile    string `yaml:"snooze_file,omitempty"`
	SnoozeMinutes int    `yaml:"snooze_minutes,omitempty"`

	// Heartbeat configuration
	HeartbeatEnabled     bool   `yaml:"heartbeat_enabled"`
	HeartbeatSubject     string `yaml:"heartbeat_subject,omitempty"`
	HeartbeatInterval    int    `yaml:"heartbeat_interval,omitempty"`    // Interval in seconds
	HeartbeatDescription string `yaml:"heartbeat_description,omitempty"`
	HeartbeatGracePeriod int    `yaml:"heartbeat_grace_period,omitempty"` // Grace period in seconds
}

// Default configuration values
var DefaultConfig = Config{
	NATSUrl:              "nats://localhost:4222",
	NATSSubject:          "alerts.meeting.alarm",
	VolumePct:            80,
	Sounds:               []string{},
	AudioOutputDriver:    "alsa",     // Default to ALSA on Linux for systemd compatibility
	AudioDevice:          "default",  // Default ALSA device
	AmixerCard:           "0",        // Default to card 0
	RepeatSeconds:        30,
	MaxRepeats:           3,
	TTSEnabled:           false,
	TTSTemplate:          "Meeting alert: {{.Title}} in {{.Lead}} minutes",
	WorkHours:            "08:00-19:00",
	QuietDays:            "Sat,Sun",
	StateDir:             "/var/lib/meeting-siren",
	HeartbeatEnabled:     false,
	HeartbeatSubject:     "heartbeat.meeting-siren",
	HeartbeatInterval:    60,  // 60 seconds (1 minute)
	HeartbeatDescription: "Meeting Siren",
	HeartbeatGracePeriod: 180, // 180 seconds (3 minutes)
}
