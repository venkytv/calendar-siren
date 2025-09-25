package integration

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/meeting-siren/meeting-siren/internal/config"
	"github.com/meeting-siren/meeting-siren/internal/daemon"
	"github.com/meeting-siren/meeting-siren/internal/domain"
	"github.com/meeting-siren/meeting-siren/internal/nats"
	"github.com/meeting-siren/meeting-siren/internal/state"
	"github.com/meeting-siren/meeting-siren/pkg/logger"
	"github.com/meeting-siren/meeting-siren/test/mocks"
	natsio "github.com/nats-io/nats.go"
)

func TestDaemon_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// This test requires a running NATS server
	// You can start one with: nats-server -p 4222
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = "nats://localhost:4222"
	}

	// Try to connect to NATS to see if it's available
	nc, err := natsio.Connect(natsURL)
	if err != nil {
		t.Skipf("NATS server not available at %s: %v", natsURL, err)
	}
	nc.Close()

	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "test-config.yaml")

	// Create test configuration
	configContent := `
nats_url: "` + natsURL + `"
nats_subject: "test.meeting.alarm"
volume_pct: 50
sounds: []
repeat_seconds: 5
max_repeats: 2
tts_enabled: false
state_dir: "` + tempDir + `/state"
work_hours: ""
quiet_days: ""
`

	err = os.WriteFile(configFile, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	// Load configuration
	loader := config.NewLoader(configFile)
	cfg, err := loader.Load()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	// Use mock audio player for integration test
	log := logger.NewJSONLogger("test-daemon")
	mockAudioPlayer := mocks.NewMockAudioPlayer()

	// Create real components
	subscriber, err := nats.NewSubscriber(cfg.NATSUrl, log)
	if err != nil {
		t.Fatalf("failed to create subscriber: %v", err)
	}
	defer subscriber.Close()

	stateManager, err := state.NewManager(cfg.StateDir, log)
	if err != nil {
		t.Fatalf("failed to create state manager: %v", err)
	}

	scheduler := state.NewScheduler(cfg, log)

	// Create daemon with mock audio player
	d := daemon.NewDaemon(cfg, log, subscriber, mockAudioPlayer, stateManager, scheduler)

	// Start daemon in background
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	daemonDone := make(chan error, 1)
	go func() {
		daemonDone <- d.Start(ctx)
	}()

	// Give daemon time to start
	time.Sleep(100 * time.Millisecond)

	t.Run("receives and processes meeting alert", func(t *testing.T) {
		// Create test publisher
		pub, err := natsio.Connect(natsURL)
		if err != nil {
			t.Fatalf("failed to connect publisher: %v", err)
		}
		defer pub.Close()

		// Create test alert
		alert := domain.MeetingAlert{
			Title:    "Integration Test Meeting",
			When:     time.Now().Add(10 * time.Minute),
			Lead:     10,
			Severity: "normal",
		}

		alertData, err := json.Marshal(alert)
		if err != nil {
			t.Fatalf("failed to marshal alert: %v", err)
		}

		// Publish alert
		err = pub.Publish(cfg.NATSSubject, alertData)
		if err != nil {
			t.Fatalf("failed to publish alert: %v", err)
		}

		// Wait for alert to be processed
		time.Sleep(500 * time.Millisecond)

		// Check that audio was played
		playCalls := mockAudioPlayer.GetPlayCalls()
		if len(playCalls) == 0 {
			t.Error("expected audio to be played but no play calls were made")
		}
	})

	t.Run("deduplicates identical alerts", func(t *testing.T) {
		// Clear previous calls
		mockAudioPlayer.Clear()

		// Create test publisher
		pub, err := natsio.Connect(natsURL)
		if err != nil {
			t.Fatalf("failed to connect publisher: %v", err)
		}
		defer pub.Close()

		// Create identical alerts
		alert := domain.MeetingAlert{
			Title:    "Duplicate Test Meeting",
			When:     time.Now().Add(15 * time.Minute),
			Lead:     15,
			Severity: "normal",
		}

		alertData, err := json.Marshal(alert)
		if err != nil {
			t.Fatalf("failed to marshal alert: %v", err)
		}

		// Publish the same alert twice quickly
		err = pub.Publish(cfg.NATSSubject, alertData)
		if err != nil {
			t.Fatalf("failed to publish first alert: %v", err)
		}

		time.Sleep(100 * time.Millisecond)

		err = pub.Publish(cfg.NATSSubject, alertData)
		if err != nil {
			t.Fatalf("failed to publish second alert: %v", err)
		}

		// Wait for alerts to be processed
		time.Sleep(500 * time.Millisecond)

		// Should only have one play call due to deduplication
		playCalls := mockAudioPlayer.GetPlayCalls()
		if len(playCalls) != 1 {
			t.Errorf("expected 1 play call due to deduplication, got %d", len(playCalls))
		}
	})

	// Stop daemon
	cancel()

	// Wait for daemon to stop
	select {
	case err := <-daemonDone:
		if err != nil && err != context.Canceled {
			t.Errorf("daemon stopped with error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("daemon did not stop within timeout")
	}
}
