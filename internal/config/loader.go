package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/meeting-siren/meeting-siren/internal/domain"
	"gopkg.in/yaml.v3"
)

const defaultConfigPath = "/etc/meeting-siren.yaml"

type Loader struct {
	configPath string
}

func NewLoader(configPath string) *Loader {
	if configPath == "" {
		configPath = defaultConfigPath
	}
	return &Loader{configPath: configPath}
}

func (l *Loader) Load() (*domain.Config, error) {
	config := domain.DefaultConfig

	// Load from environment variables first
	if err := l.loadFromEnv(&config); err != nil {
		return nil, fmt.Errorf("loading from environment: %w", err)
	}

	// Override with YAML file if it exists
	if _, err := os.Stat(l.configPath); err == nil {
		if err := l.loadFromYAML(&config); err != nil {
			return nil, fmt.Errorf("loading from YAML file %s: %w", l.configPath, err)
		}
	}

	// Validate configuration
	if err := l.validate(&config); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return &config, nil
}

func (l *Loader) loadFromEnv(config *domain.Config) error {
	if url := os.Getenv("NATS_URL"); url != "" {
		config.NATSUrl = url
	}
	if subject := os.Getenv("NATS_SUBJECT"); subject != "" {
		config.NATSSubject = subject
	}
	if vol := os.Getenv("VOLUME_PCT"); vol != "" {
		if v, err := strconv.Atoi(vol); err == nil && v >= 0 && v <= 100 {
			config.VolumePct = v
		}
	}
	if sounds := os.Getenv("SOUNDS"); sounds != "" {
		config.Sounds = strings.Split(sounds, ",")
	}
	if repeat := os.Getenv("REPEAT_SECONDS"); repeat != "" {
		if r, err := strconv.Atoi(repeat); err == nil && r >= 0 {
			config.RepeatSeconds = r
		}
	}
	if maxRep := os.Getenv("MAX_REPEATS"); maxRep != "" {
		if m, err := strconv.Atoi(maxRep); err == nil && m >= 0 {
			config.MaxRepeats = m
		}
	}
	if tts := os.Getenv("TTS_ENABLED"); tts != "" {
		config.TTSEnabled = strings.ToLower(tts) == "true"
	}
	if template := os.Getenv("TTS_TEMPLATE"); template != "" {
		config.TTSTemplate = template
	}
	if hours := os.Getenv("WORK_HOURS"); hours != "" {
		config.WorkHours = hours
	}
	if days := os.Getenv("QUIET_DAYS"); days != "" {
		config.QuietDays = days
	}
	if stateDir := os.Getenv("STATE_DIR"); stateDir != "" {
		config.StateDir = stateDir
	}
	if pin := os.Getenv("GPIO_BUZZER_PIN"); pin != "" {
		if p, err := strconv.Atoi(pin); err == nil {
			config.GPIOBuzzerPin = &p
		}
	}
	if snoozeFile := os.Getenv("SNOOZE_FILE"); snoozeFile != "" {
		config.SnoozeFile = snoozeFile
	}
	if snoozeMin := os.Getenv("SNOOZE_MINUTES"); snoozeMin != "" {
		if s, err := strconv.Atoi(snoozeMin); err == nil && s >= 0 {
			config.SnoozeMinutes = s
		}
	}
	return nil
}

func (l *Loader) loadFromYAML(config *domain.Config) error {
	data, err := os.ReadFile(l.configPath)
	if err != nil {
		return fmt.Errorf("reading config file: %w", err)
	}

	if err := yaml.Unmarshal(data, config); err != nil {
		return fmt.Errorf("parsing YAML config: %w", err)
	}

	return nil
}

func (l *Loader) validate(config *domain.Config) error {
	if config.NATSUrl == "" {
		return fmt.Errorf("NATS_URL is required")
	}
	if config.NATSSubject == "" {
		return fmt.Errorf("NATS_SUBJECT is required")
	}
	if config.VolumePct < 0 || config.VolumePct > 100 {
		return fmt.Errorf("volume_pct must be between 0 and 100")
	}
	if config.StateDir == "" {
		return fmt.Errorf("state_dir is required")
	}
	if config.WorkHours != "" {
		if !isValidTimeRange(config.WorkHours) {
			return fmt.Errorf("invalid work_hours format, expected HH:MM-HH:MM")
		}
	}
	return nil
}

func isValidTimeRange(timeRange string) bool {
	parts := strings.Split(timeRange, "-")
	if len(parts) != 2 {
		return false
	}
	for _, part := range parts {
		timeParts := strings.Split(part, ":")
		if len(timeParts) != 2 {
			return false
		}
		hour, err1 := strconv.Atoi(timeParts[0])
		min, err2 := strconv.Atoi(timeParts[1])
		if err1 != nil || err2 != nil || hour < 0 || hour > 23 || min < 0 || min > 59 {
			return false
		}
	}
	return true
}
