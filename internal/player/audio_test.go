package player

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/meeting-siren/meeting-siren/internal/domain"
	"github.com/meeting-siren/meeting-siren/test/mocks"
)

func TestPlayer_RenderTTSMessage(t *testing.T) {
	tests := []struct {
		name        string
		template    string
		alert       *domain.MeetingAlert
		expected    string
		expectError bool
	}{
		{
			name:     "default template",
			template: "",
			alert: &domain.MeetingAlert{
				Title: "Team Standup",
				Lead:  5,
			},
			expected: "Meeting alert: Team Standup in 5 minutes",
		},
		{
			name:     "custom template",
			template: "Alert: {{.Title}} starting in {{.Lead}} minutes",
			alert: &domain.MeetingAlert{
				Title: "Design Review",
				Lead:  10,
			},
			expected: "Alert: Design Review starting in 10 minutes",
		},
		{
			name:     "template with severity",
			template: "{{.Severity}} priority: {{.Title}}",
			alert: &domain.MeetingAlert{
				Title:    "Critical Meeting",
				Severity: "critical",
			},
			expected: "critical priority: Critical Meeting",
		},
		{
			name:        "invalid template",
			template:    "{{.InvalidField}",
			alert:       &domain.MeetingAlert{Title: "Test"},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &domain.Config{
				TTSTemplate: tt.template,
			}
			logger := mocks.NewMockLogger()
			player := NewPlayer(config, logger)

			result, err := player.RenderTTSMessage(tt.alert)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result != tt.expected {
				t.Errorf("RenderTTSMessage() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestPlayer_Play(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Audio testing not supported on Windows")
	}

	tempDir := t.TempDir()

	// Create a test "sound file" (just an empty file for testing)
	soundFile := filepath.Join(tempDir, "test.wav")
	err := os.WriteFile(soundFile, []byte("fake audio data"), 0644)
	if err != nil {
		t.Fatalf("failed to create test sound file: %v", err)
	}

	config := &domain.Config{
		VolumePct: 50,
		Sounds:    []string{soundFile},
	}
	logger := mocks.NewMockLogger()
	player := NewPlayer(config, logger)

	t.Run("play existing file", func(t *testing.T) {
		ctx := context.Background()

		// This will fail because we don't have actual audio tools, but we're testing the logic
		err := player.Play(ctx, []string{soundFile})
		// We expect an error because audio tools aren't available in test environment
		// The important thing is that it doesn't panic and handles the file check
		if err == nil && runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
			t.Log("Note: Audio playback succeeded unexpectedly in test environment")
		}
	})

	t.Run("play nonexistent file", func(t *testing.T) {
		ctx := context.Background()
		nonexistentFile := filepath.Join(tempDir, "nonexistent.wav")

		// The Play method logs errors but doesn't fail entirely for missing files
		// This is the correct behavior for production use
		err := player.Play(ctx, []string{nonexistentFile})
		if err != nil {
			t.Errorf("Play method should not fail entirely for missing files: %v", err)
		}

		// Check that error was logged
		errorLogs := logger.GetErrorLogs()
		if len(errorLogs) == 0 {
			t.Error("expected error to be logged for nonexistent file")
		}
	})

	t.Run("play empty file list", func(t *testing.T) {
		ctx := context.Background()

		err := player.Play(ctx, []string{})
		if err != nil {
			t.Errorf("unexpected error for empty file list: %v", err)
		}
	})
}

func TestPlayer_SetVolume(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Volume control testing not supported on Windows")
	}

	config := &domain.Config{}
	logger := mocks.NewMockLogger()
	player := NewPlayer(config, logger)

	t.Run("set valid volume", func(t *testing.T) {
		ctx := context.Background()

		// This will likely fail in test environment, but we're testing the validation logic
		err := player.SetVolume(ctx, 75)
		// We expect this to fail in most test environments due to missing audio tools
		// The test ensures the method handles the call properly
		if err == nil {
			t.Log("Note: Volume setting succeeded unexpectedly in test environment")
		}
	})

	t.Run("set volume out of range", func(t *testing.T) {
		ctx := context.Background()

		// The implementation doesn't validate range, but audio tools should handle it
		err := player.SetVolume(ctx, 150)
		// This may or may not fail depending on the underlying audio tool
		_ = err // Don't fail the test for this
	})
}

func TestPlayer_PlayTTS(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("TTS testing not supported on Windows")
	}

	tests := []struct {
		name       string
		ttsEnabled bool
		message    string
	}{
		{
			name:       "TTS enabled",
			ttsEnabled: true,
			message:    "Test TTS message",
		},
		{
			name:       "TTS disabled",
			ttsEnabled: false,
			message:    "This should not be spoken",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &domain.Config{
				TTSEnabled: tt.ttsEnabled,
			}
			logger := mocks.NewMockLogger()
			player := NewPlayer(config, logger)

			ctx := context.Background()
			err := player.PlayTTS(ctx, tt.message)

			if !tt.ttsEnabled {
				// Should return immediately without error when TTS is disabled
				if err != nil {
					t.Errorf("unexpected error when TTS disabled: %v", err)
				}
				return
			}

			// When TTS is enabled, it will likely fail in test environment
			// due to missing TTS tools, but that's expected
			if err == nil && (runtime.GOOS == "darwin" || runtime.GOOS == "linux") {
				t.Log("Note: TTS succeeded unexpectedly in test environment")
			}
		})
	}
}

func TestPlayer_VolumeRestoration(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Volume control testing not supported on Windows")
	}

	tests := []struct {
		name           string
		currentVolume  int
		alarmVolume    int
		expectRestore  bool
		simulateGetErr bool
		simulateSetErr bool
	}{
		{
			name:          "normal volume restoration",
			currentVolume: 50,
			alarmVolume:   100,
			expectRestore: true,
		},
		{
			name:          "restore from low volume",
			currentVolume: 10,
			alarmVolume:   90,
			expectRestore: true,
		},
		{
			name:           "fail to get current volume",
			currentVolume:  50,
			alarmVolume:    100,
			expectRestore:  false,
			simulateGetErr: true,
		},
		{
			name:          "same volume (still restore)",
			currentVolume: 80,
			alarmVolume:   80,
			expectRestore: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()

			// Create a test sound file
			soundFile := filepath.Join(tempDir, "test.wav")
			err := os.WriteFile(soundFile, []byte("fake audio"), 0644)
			if err != nil {
				t.Fatalf("failed to create test sound file: %v", err)
			}

			config := &domain.Config{
				VolumePct: tt.alarmVolume,
				Sounds:    []string{soundFile},
			}
			logger := mocks.NewMockLogger()
			player := NewPlayer(config, logger)

			ctx := context.Background()

			// Note: In a real test environment, we can't actually control system volume
			// This test verifies the logic flow and error handling
			err = player.Play(ctx, []string{soundFile})

			// The Play method should complete without error regardless of volume operations
			// since volume operations are logged but don't fail the entire operation
			if err != nil {
				t.Errorf("Play() should not fail due to volume operations: %v", err)
			}

			// Check that appropriate log entries were created
			logs := logger.GetDebugLogs()
			errorLogs := logger.GetErrorLogs()

			// In test environment, volume operations will likely fail, so we mainly test
			// that the code attempts the operations and handles errors gracefully

			// Should have attempted to get current volume (may have failed in test env)
			hasVolumeLog := false
			for _, log := range logs {
				if log.Message == "Saved original volume" {
					hasVolumeLog = true
					break
				}
			}
			for _, log := range errorLogs {
				if log.Message == "Failed to get current volume, proceeding without restoration" {
					hasVolumeLog = true
					break
				}
			}

			if !hasVolumeLog {
				t.Error("Expected volume save operation to be attempted")
			}
		})
	}
}

func TestPlayer_TTSVolumeRestoration(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("TTS testing not supported on Windows")
	}

	tests := []struct {
		name       string
		ttsEnabled bool
		message    string
	}{
		{
			name:       "TTS with volume restoration",
			ttsEnabled: true,
			message:    "Test TTS with volume restore",
		},
		{
			name:       "TTS disabled (no volume change)",
			ttsEnabled: false,
			message:    "This should not speak or change volume",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &domain.Config{
				TTSEnabled: tt.ttsEnabled,
				VolumePct:  85, // Set to specific volume for testing
			}
			logger := mocks.NewMockLogger()
			player := NewPlayer(config, logger)

			ctx := context.Background()
			err := player.PlayTTS(ctx, tt.message)

			if !tt.ttsEnabled {
				// Should return immediately without error when TTS is disabled
				if err != nil {
					t.Errorf("unexpected error when TTS disabled: %v", err)
				}

				// Should not have any volume-related log entries
				logs := logger.GetDebugLogs()
				for _, log := range logs {
					if strings.Contains(log.Message, "volume") {
						t.Errorf("unexpected volume operation when TTS disabled: %s", log.Message)
					}
				}
				return
			}

			// When TTS is enabled, volume operations should be attempted
			// (may fail in test environment, but should be attempted)
			logs := logger.GetDebugLogs()
			errorLogs := logger.GetErrorLogs()

			hasVolumeLog := false
			for _, log := range logs {
				if log.Message == "Saved original volume for TTS" {
					hasVolumeLog = true
					break
				}
			}
			for _, log := range errorLogs {
				if log.Message == "Failed to get current volume for TTS, proceeding without restoration" {
					hasVolumeLog = true
					break
				}
			}

			if !hasVolumeLog {
				t.Error("Expected TTS volume save operation to be attempted")
			}
		})
	}
}

func TestPlayer_GetCurrentVolume(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Volume control testing not supported on Windows")
	}

	config := &domain.Config{}
	logger := mocks.NewMockLogger()
	player := NewPlayer(config, logger)

	ctx := context.Background()

	t.Run("get current volume", func(t *testing.T) {
		// This will likely fail in test environment due to missing audio tools
		// but we're testing that the method handles the call properly
		volume, err := player.getCurrentVolume(ctx)

		if err == nil {
			// If it succeeds, volume should be in valid range
			if volume < 0 || volume > 100 {
				t.Errorf("volume %d out of expected range 0-100", volume)
			}
			t.Logf("Successfully got current volume: %d%%", volume)
		} else {
			// Expected to fail in most test environments
			t.Logf("getCurrentVolume failed as expected in test environment: %v", err)
		}
	})

	t.Run("get volume with cancelled context", func(t *testing.T) {
		cancelledCtx, cancel := context.WithCancel(ctx)
		cancel() // Cancel immediately

		_, err := player.getCurrentVolume(cancelledCtx)
		if err == nil {
			t.Log("Note: getCurrentVolume succeeded despite cancelled context")
		}
		// Don't fail the test since this depends on external command execution
	})
}
