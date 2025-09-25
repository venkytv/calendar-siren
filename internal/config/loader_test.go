package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/meeting-siren/meeting-siren/internal/domain"
)

func TestLoader_LoadFromEnv(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		expected domain.Config
	}{
		{
			name:     "default config",
			envVars:  map[string]string{},
			expected: domain.DefaultConfig,
		},
		{
			name: "custom NATS config",
			envVars: map[string]string{
				"NATS_URL":     "nats://test:4222",
				"NATS_SUBJECT": "test.subject",
			},
			expected: func() domain.Config {
				cfg := domain.DefaultConfig
				cfg.NATSUrl = "nats://test:4222"
				cfg.NATSSubject = "test.subject"
				return cfg
			}(),
		},
		{
			name: "audio config",
			envVars: map[string]string{
				"VOLUME_PCT": "95",
				"SOUNDS":     "/path/to/sound1.wav,/path/to/sound2.mp3",
			},
			expected: func() domain.Config {
				cfg := domain.DefaultConfig
				cfg.VolumePct = 95
				cfg.Sounds = []string{"/path/to/sound1.wav", "/path/to/sound2.mp3"}
				return cfg
			}(),
		},
		{
			name: "repeat config",
			envVars: map[string]string{
				"REPEAT_SECONDS": "45",
				"MAX_REPEATS":    "5",
			},
			expected: func() domain.Config {
				cfg := domain.DefaultConfig
				cfg.RepeatSeconds = 45
				cfg.MaxRepeats = 5
				return cfg
			}(),
		},
		{
			name: "TTS config",
			envVars: map[string]string{
				"TTS_ENABLED":  "true",
				"TTS_TEMPLATE": "Alert: {{.Title}} starting in {{.Lead}} minutes",
			},
			expected: func() domain.Config {
				cfg := domain.DefaultConfig
				cfg.TTSEnabled = true
				cfg.TTSTemplate = "Alert: {{.Title}} starting in {{.Lead}} minutes"
				return cfg
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear environment
			os.Clearenv()

			// Set test environment variables
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}

			loader := NewLoader("/nonexistent/config.yaml")
			cfg, err := loader.Load()

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Check key fields
			if cfg.NATSUrl != tt.expected.NATSUrl {
				t.Errorf("NATSUrl = %v, want %v", cfg.NATSUrl, tt.expected.NATSUrl)
			}
			if cfg.NATSSubject != tt.expected.NATSSubject {
				t.Errorf("NATSSubject = %v, want %v", cfg.NATSSubject, tt.expected.NATSSubject)
			}
			if cfg.VolumePct != tt.expected.VolumePct {
				t.Errorf("VolumePct = %v, want %v", cfg.VolumePct, tt.expected.VolumePct)
			}
			if len(cfg.Sounds) != len(tt.expected.Sounds) {
				t.Errorf("Sounds length = %v, want %v", len(cfg.Sounds), len(tt.expected.Sounds))
			}
		})
	}
}

func TestLoader_LoadFromYAML(t *testing.T) {
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "test-config.yaml")

	yamlContent := `
nats_url: "nats://yaml:4222"
nats_subject: "yaml.subject"
volume_pct: 85
sounds:
  - "/yaml/sound1.wav"
  - "/yaml/sound2.mp3"
repeat_seconds: 60
max_repeats: 2
tts_enabled: true
tts_template: "YAML: {{.Title}}"
work_hours: "09:00-18:00"
quiet_days: "Sat,Sun"
state_dir: "/tmp/yaml-state"
`

	err := os.WriteFile(configFile, []byte(yamlContent), 0644)
	if err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	loader := NewLoader(configFile)
	cfg, err := loader.Load()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.NATSUrl != "nats://yaml:4222" {
		t.Errorf("NATSUrl = %v, want %v", cfg.NATSUrl, "nats://yaml:4222")
	}
	if cfg.VolumePct != 85 {
		t.Errorf("VolumePct = %v, want %v", cfg.VolumePct, 85)
	}
	if !cfg.TTSEnabled {
		t.Errorf("TTSEnabled = %v, want %v", cfg.TTSEnabled, true)
	}
	if cfg.WorkHours != "09:00-18:00" {
		t.Errorf("WorkHours = %v, want %v", cfg.WorkHours, "09:00-18:00")
	}
}

func TestLoader_Validation(t *testing.T) {
	tests := []struct {
		name        string
		config      domain.Config
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid config",
			config:      domain.DefaultConfig,
			expectError: false,
		},
		{
			name: "missing NATS URL",
			config: func() domain.Config {
				cfg := domain.DefaultConfig
				cfg.NATSUrl = ""
				return cfg
			}(),
			expectError: true,
			errorMsg:    "NATS_URL is required",
		},
		{
			name: "invalid volume",
			config: func() domain.Config {
				cfg := domain.DefaultConfig
				cfg.VolumePct = 150
				return cfg
			}(),
			expectError: true,
			errorMsg:    "volume_pct must be between 0 and 100",
		},
		{
			name: "invalid work hours",
			config: func() domain.Config {
				cfg := domain.DefaultConfig
				cfg.WorkHours = "invalid-format"
				return cfg
			}(),
			expectError: true,
			errorMsg:    "invalid work_hours format, expected HH:MM-HH:MM",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loader := NewLoader("/nonexistent/config.yaml")
			err := loader.validate(&tt.config)

			if tt.expectError {
				if err == nil {
					t.Fatalf("expected error but got none")
				}
				if tt.errorMsg != "" && err.Error() != tt.errorMsg {
					t.Errorf("error message = %v, want %v", err.Error(), tt.errorMsg)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestIsValidTimeRange(t *testing.T) {
	tests := []struct {
		timeRange string
		expected  bool
	}{
		{"08:00-17:00", true},
		{"00:00-23:59", true},
		{"23:00-07:00", true},  // Spans midnight
		{"08:00", false},       // Missing end time
		{"08:00-25:00", false}, // Invalid hour
		{"08:60-17:00", false}, // Invalid minute
		{"invalid", false},     // Invalid format
	}

	for _, tt := range tests {
		t.Run(tt.timeRange, func(t *testing.T) {
			result := isValidTimeRange(tt.timeRange)
			if result != tt.expected {
				t.Errorf("isValidTimeRange(%s) = %v, want %v", tt.timeRange, result, tt.expected)
			}
		})
	}
}
