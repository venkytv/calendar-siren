package heartbeat

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/meeting-siren/meeting-siren/internal/domain"
	"github.com/nats-io/nats.go"
)

type mockLogger struct {
	mu      sync.RWMutex
	infos   []string
	errors  []string
	debugs  []string
}

func (m *mockLogger) Info(msg string, fields map[string]interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.infos = append(m.infos, msg)
}

func (m *mockLogger) Error(msg string, err error, fields map[string]interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.errors = append(m.errors, msg)
}

func (m *mockLogger) Debug(msg string, fields map[string]interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.debugs = append(m.debugs, msg)
}

func (m *mockLogger) getInfos() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]string{}, m.infos...)
}

func (m *mockLogger) getErrors() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]string{}, m.errors...)
}

func setupTestNATS(t *testing.T) (*nats.Conn, func()) {
	// Try to connect to a local NATS server for testing
	nc, err := nats.Connect(nats.DefaultURL, nats.Timeout(2*time.Second))
	if err != nil {
		t.Skip("NATS server not available for testing")
	}

	cleanup := func() {
		nc.Close()
	}

	return nc, cleanup
}

func TestNewPublisher(t *testing.T) {
	nc, cleanup := setupTestNATS(t)
	defer cleanup()

	logger := &mockLogger{}
	config := &domain.Config{
		HeartbeatEnabled:     true,
		HeartbeatSubject:     "test.heartbeat",
		HeartbeatInterval:    1,
		HeartbeatDescription: "Test Service",
		HeartbeatGracePeriod: 3,
	}

	publisher := NewPublisher(nc, config, logger)
	if publisher == nil {
		t.Fatal("NewPublisher returned nil")
	}

	if publisher.nc != nc {
		t.Error("Publisher NATS connection mismatch")
	}

	if publisher.config != config {
		t.Error("Publisher config mismatch")
	}

	if publisher.logger != logger {
		t.Error("Publisher logger mismatch")
	}
}

func TestPublisher_StartStop_Disabled(t *testing.T) {
	nc, cleanup := setupTestNATS(t)
	defer cleanup()

	logger := &mockLogger{}
	config := &domain.Config{
		HeartbeatEnabled: false,
	}

	publisher := NewPublisher(nc, config, logger)
	ctx := context.Background()

	err := publisher.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Should log that heartbeat is disabled
	time.Sleep(100 * time.Millisecond)
	infos := logger.getInfos()
	found := false
	for _, info := range infos {
		if info == "heartbeat disabled, skipping" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected 'heartbeat disabled' log message")
	}

	err = publisher.Stop()
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

func TestPublisher_StartStop_Enabled(t *testing.T) {
	nc, cleanup := setupTestNATS(t)
	defer cleanup()

	logger := &mockLogger{}
	config := &domain.Config{
		HeartbeatEnabled:     true,
		HeartbeatSubject:     "test.heartbeat",
		HeartbeatInterval:    1, // 1 second for fast testing
		HeartbeatDescription: "Test Service",
		HeartbeatGracePeriod: 3,
	}

	publisher := NewPublisher(nc, config, logger)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Subscribe to heartbeat messages to verify they're being published
	receivedMessages := 0
	var msgMu sync.Mutex
	sub, err := nc.Subscribe(config.HeartbeatSubject, func(msg *nats.Msg) {
		msgMu.Lock()
		receivedMessages++
		msgMu.Unlock()
	})
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}
	defer sub.Unsubscribe()

	err = publisher.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Wait for at least 2 heartbeats (initial + 1 interval)
	time.Sleep(2500 * time.Millisecond)

	msgMu.Lock()
	count := receivedMessages
	msgMu.Unlock()

	if count < 2 {
		t.Errorf("Expected at least 2 heartbeat messages, got %d", count)
	}

	err = publisher.Stop()
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Verify stop was logged
	time.Sleep(100 * time.Millisecond)
	infos := logger.getInfos()
	found := false
	for _, info := range infos {
		if info == "heartbeat publisher stopped" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected 'heartbeat publisher stopped' log message")
	}
}

func TestPublisher_StartValidation(t *testing.T) {
	nc, cleanup := setupTestNATS(t)
	defer cleanup()

	tests := []struct {
		name        string
		config      *domain.Config
		expectError bool
		errorMsg    string
	}{
		{
			name: "missing subject",
			config: &domain.Config{
				HeartbeatEnabled:  true,
				HeartbeatSubject:  "",
				HeartbeatInterval: 60,
			},
			expectError: true,
			errorMsg:    "heartbeat subject is required",
		},
		{
			name: "invalid interval",
			config: &domain.Config{
				HeartbeatEnabled:  true,
				HeartbeatSubject:  "test.heartbeat",
				HeartbeatInterval: 0,
			},
			expectError: true,
			errorMsg:    "heartbeat interval must be positive",
		},
		{
			name: "negative interval",
			config: &domain.Config{
				HeartbeatEnabled:  true,
				HeartbeatSubject:  "test.heartbeat",
				HeartbeatInterval: -1,
			},
			expectError: true,
			errorMsg:    "heartbeat interval must be positive",
		},
		{
			name: "valid configuration",
			config: &domain.Config{
				HeartbeatEnabled:     true,
				HeartbeatSubject:     "test.heartbeat",
				HeartbeatInterval:    60,
				HeartbeatDescription: "Test",
				HeartbeatGracePeriod: 180,
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := &mockLogger{}
			publisher := NewPublisher(nc, tt.config, logger)
			ctx := context.Background()

			err := publisher.Start(ctx)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error containing '%s', got nil", tt.errorMsg)
				} else if err.Error() != tt.errorMsg {
					t.Errorf("Expected error '%s', got '%s'", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				publisher.Stop()
			}
		})
	}
}

func TestPublisher_MultipleStartStop(t *testing.T) {
	nc, cleanup := setupTestNATS(t)
	defer cleanup()

	logger := &mockLogger{}
	config := &domain.Config{
		HeartbeatEnabled:     true,
		HeartbeatSubject:     "test.heartbeat",
		HeartbeatInterval:    1,
		HeartbeatDescription: "Test Service",
	}

	publisher := NewPublisher(nc, config, logger)
	ctx := context.Background()

	// Start multiple times should be idempotent
	err := publisher.Start(ctx)
	if err != nil {
		t.Fatalf("First Start failed: %v", err)
	}

	err = publisher.Start(ctx)
	if err != nil {
		t.Error("Second Start should not error (idempotent)")
	}

	// Stop multiple times should be idempotent
	err = publisher.Stop()
	if err != nil {
		t.Fatalf("First Stop failed: %v", err)
	}

	err = publisher.Stop()
	if err != nil {
		t.Error("Second Stop should not error (idempotent)")
	}
}
