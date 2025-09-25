package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/meeting-siren/meeting-siren/internal/config"
)

func TestConfigLoader_Integration(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("env overrides and YAML file integration", func(t *testing.T) {
		// Create a YAML config file
		configFile := filepath.Join(tempDir, "integration-config.yaml")
		yamlContent := `
nats_url: "nats://yaml:4222"
nats_subject: "yaml.alerts"
volume_pct: 90
sounds:
  - "/yaml/sound1.wav"
  - "/yaml/sound2.wav"
repeat_seconds: 30
max_repeats: 3
tts_enabled: false
work_hours: "08:00-18:00"
state_dir: "/tmp/yaml-state"
`

		err := os.WriteFile(configFile, []byte(yamlContent), 0644)
		if err != nil {
			t.Fatalf("failed to write config file: %v", err)
		}

		// Set some environment variables that will be overridden by YAML
		os.Setenv("NATS_URL", "nats://env:4222")
		os.Setenv("VOLUME_PCT", "95")
		os.Setenv("TTS_ENABLED", "true")
		defer func() {
			os.Unsetenv("NATS_URL")
			os.Unsetenv("VOLUME_PCT")
			os.Unsetenv("TTS_ENABLED")
		}()

		// Load configuration
		loader := config.NewLoader(configFile)
		cfg, err := loader.Load()
		if err != nil {
			t.Fatalf("failed to load config: %v", err)
		}

		// Check that YAML overrides env vars (correct behavior)
		if cfg.NATSUrl != "nats://yaml:4222" {
			t.Errorf("expected NATS URL from YAML, got %s", cfg.NATSUrl)
		}

		if cfg.VolumePct != 90 {
			t.Errorf("expected volume from YAML (90), got %d", cfg.VolumePct)
		}

		if cfg.TTSEnabled {
			t.Errorf("expected TTS disabled from YAML, got %v", cfg.TTSEnabled)
		}

		// Check that YAML values are used where no env override exists
		if cfg.NATSSubject != "yaml.alerts" {
			t.Errorf("expected NATS subject from YAML, got %s", cfg.NATSSubject)
		}

		if cfg.RepeatSeconds != 30 {
			t.Errorf("expected repeat seconds from YAML (30), got %d", cfg.RepeatSeconds)
		}

		if len(cfg.Sounds) != 2 {
			t.Errorf("expected 2 sounds from YAML, got %d", len(cfg.Sounds))
		}
	})

	t.Run("config validation with real file system", func(t *testing.T) {
		// Create a config file with invalid settings
		configFile := filepath.Join(tempDir, "invalid-config.yaml")
		invalidYaml := `
nats_url: ""  # Invalid: empty NATS URL
nats_subject: "test.alerts"
volume_pct: 150  # Invalid: out of range
state_dir: "/tmp/test-state"
work_hours: "invalid-time-format"  # Invalid format
`

		err := os.WriteFile(configFile, []byte(invalidYaml), 0644)
		if err != nil {
			t.Fatalf("failed to write invalid config file: %v", err)
		}

		loader := config.NewLoader(configFile)
		_, err = loader.Load()

		if err == nil {
			t.Fatal("expected validation error for invalid config")
		}

		t.Logf("Validation correctly caught error: %v", err)
	})

	t.Run("config with missing YAML file uses defaults", func(t *testing.T) {
		nonexistentFile := filepath.Join(tempDir, "nonexistent.yaml")

		// Clear environment to test defaults
		os.Clearenv()
		os.Setenv("NATS_URL", "nats://localhost:4222")
		os.Setenv("NATS_SUBJECT", "test.alerts")
		os.Setenv("STATE_DIR", tempDir)
		defer func() {
			os.Unsetenv("NATS_URL")
			os.Unsetenv("NATS_SUBJECT")
			os.Unsetenv("STATE_DIR")
		}()

		loader := config.NewLoader(nonexistentFile)
		cfg, err := loader.Load()

		if err != nil {
			t.Fatalf("failed to load config with missing file: %v", err)
		}

		// Should use default values for non-overridden settings
		if cfg.VolumePct != 80 { // Default volume
			t.Errorf("expected default volume (80), got %d", cfg.VolumePct)
		}

		if cfg.RepeatSeconds != 30 { // Default repeat
			t.Errorf("expected default repeat seconds (30), got %d", cfg.RepeatSeconds)
		}

		// Should use env values for overridden settings
		if cfg.NATSUrl != "nats://localhost:4222" {
			t.Errorf("expected NATS URL from env, got %s", cfg.NATSUrl)
		}

		if cfg.NATSSubject != "test.alerts" {
			t.Errorf("expected NATS subject from env, got %s", cfg.NATSSubject)
		}
	})
}
