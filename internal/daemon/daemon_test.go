package daemon

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/meeting-siren/meeting-siren/internal/domain"
	"github.com/meeting-siren/meeting-siren/test/mocks"
)

type MockSubscriber struct {
	mu       sync.RWMutex
	handler  func(*domain.MeetingAlert)
	closed   bool
	subCalls int
}

func (m *MockSubscriber) Subscribe(ctx context.Context, subject string, handler func(*domain.MeetingAlert)) error {
	m.mu.Lock()
	m.handler = handler
	m.subCalls++
	m.mu.Unlock()

	<-ctx.Done()
	return ctx.Err()
}

func (m *MockSubscriber) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

func (m *MockSubscriber) SendAlert(alert *domain.MeetingAlert) {
	m.mu.RLock()
	handler := m.handler
	m.mu.RUnlock()

	if handler != nil {
		handler(alert)
	}
}

func (m *MockSubscriber) IsClosed() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.closed
}

type MockStateManager struct {
	mu          sync.RWMutex
	shouldFire  bool
	fireError   error
	recordError error
	firedEvents []string
}

func (m *MockStateManager) ShouldFire(event *domain.AlarmEvent) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.shouldFire, m.fireError
}

func (m *MockStateManager) RecordFired(event *domain.AlarmEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.firedEvents = append(m.firedEvents, event.UID)
	return m.recordError
}

func (m *MockStateManager) Cleanup(olderThan time.Duration) error {
	return nil
}

func (m *MockStateManager) SetShouldFire(should bool, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.shouldFire = should
	m.fireError = err
}

func (m *MockStateManager) GetFiredEvents() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]string(nil), m.firedEvents...)
}

type MockScheduler struct {
	mu         sync.RWMutex
	shouldFire bool
}

func (m *MockScheduler) ShouldFire(when time.Time) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.shouldFire
}

func (m *MockScheduler) SetShouldFire(should bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.shouldFire = should
}

func TestDaemon_HandleMeetingAlert(t *testing.T) {
	config := &domain.Config{
		NATSUrl:       "nats://test:4222",
		NATSSubject:   "test.alerts",
		Sounds:        []string{"/test/sound.wav"},
		TTSEnabled:    true,
		RepeatSeconds: 0, // Disable repeats for simpler testing
	}

	logger := mocks.NewMockLogger()
	subscriber := &MockSubscriber{}
	audioPlayer := mocks.NewMockAudioPlayer()
	stateManager := &MockStateManager{shouldFire: true}
	scheduler := &MockScheduler{shouldFire: true}

	daemon := NewDaemon(config, logger, subscriber, audioPlayer, stateManager, scheduler)

	alert := &domain.MeetingAlert{
		Title:    "Test Meeting",
		When:     time.Now().Add(10 * time.Minute),
		Lead:     10,
		Severity: "normal",
	}

	t.Run("fires alarm when conditions met", func(t *testing.T) {
		// Reset mocks
		audioPlayer.Clear()
		stateManager.firedEvents = nil

		daemon.handleMeetingAlert(alert)

		// Give some time for goroutine to execute
		time.Sleep(100 * time.Millisecond)

		// Check that audio was played
		playCalls := audioPlayer.GetPlayCalls()
		if len(playCalls) != 1 {
			t.Errorf("expected 1 play call, got %d", len(playCalls))
		}

		// Check that TTS was played
		ttsCalls := audioPlayer.GetTTSCalls()
		if len(ttsCalls) != 1 {
			t.Errorf("expected 1 TTS call, got %d", len(ttsCalls))
		}

		// Check that event was recorded
		firedEvents := stateManager.GetFiredEvents()
		if len(firedEvents) != 1 {
			t.Errorf("expected 1 fired event, got %d", len(firedEvents))
		}
	})

	t.Run("skips alarm when scheduler says no", func(t *testing.T) {
		audioPlayer.Clear()
		stateManager.firedEvents = nil
		scheduler.SetShouldFire(false)

		daemon.handleMeetingAlert(alert)

		// Give some time for potential goroutine to execute
		time.Sleep(100 * time.Millisecond)

		// Check that no audio was played
		playCalls := audioPlayer.GetPlayCalls()
		if len(playCalls) != 0 {
			t.Errorf("expected 0 play calls when scheduler blocks, got %d", len(playCalls))
		}

		// Reset scheduler
		scheduler.SetShouldFire(true)
	})

	t.Run("skips alarm when state manager says no", func(t *testing.T) {
		audioPlayer.Clear()
		stateManager.firedEvents = nil
		stateManager.SetShouldFire(false, nil)

		daemon.handleMeetingAlert(alert)

		// Give some time for potential goroutine to execute
		time.Sleep(100 * time.Millisecond)

		// Check that no audio was played
		playCalls := audioPlayer.GetPlayCalls()
		if len(playCalls) != 0 {
			t.Errorf("expected 0 play calls when state manager blocks, got %d", len(playCalls))
		}

		// Reset state manager
		stateManager.SetShouldFire(true, nil)
	})
}

func TestDaemon_StartStop(t *testing.T) {
	config := &domain.Config{
		NATSUrl:     "nats://test:4222",
		NATSSubject: "test.alerts",
	}

	logger := mocks.NewMockLogger()
	subscriber := &MockSubscriber{}
	audioPlayer := mocks.NewMockAudioPlayer()
	stateManager := &MockStateManager{}
	scheduler := &MockScheduler{}

	daemon := NewDaemon(config, logger, subscriber, audioPlayer, stateManager, scheduler)

	t.Run("start and stop daemon", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		// Start daemon in goroutine
		errChan := make(chan error, 1)
		go func() {
			errChan <- daemon.Start(ctx)
		}()

		// Give daemon time to start
		time.Sleep(50 * time.Millisecond)

		// Stop daemon
		cancel()

		// Wait for daemon to finish
		select {
		case err := <-errChan:
			if err != nil && err != context.Canceled {
				t.Errorf("expected nil or context.Canceled, got %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("daemon did not stop within timeout")
		}

		// Check that subscriber was closed
		if !subscriber.IsClosed() {
			t.Error("expected subscriber to be closed")
		}
	})

	t.Run("cannot start already running daemon", func(t *testing.T) {
		// Create a new daemon for this test
		newDaemon := NewDaemon(config, logger, &MockSubscriber{}, audioPlayer, stateManager, scheduler)

		ctx1, cancel1 := context.WithCancel(context.Background())

		// Start first instance in background
		errChan1 := make(chan error, 1)
		go func() {
			errChan1 <- newDaemon.Start(ctx1)
		}()

		// Give daemon time to start
		time.Sleep(50 * time.Millisecond)

		// Try to start the same daemon again - should fail
		ctx2 := context.Background()
		err := newDaemon.Start(ctx2)
		if err == nil {
			t.Error("expected error when starting already running daemon")
		}

		// Stop the first daemon
		cancel1()

		// Wait for daemon to stop
		select {
		case <-errChan1:
		case <-time.After(2 * time.Second):
			t.Error("daemon did not stop within timeout")
		}
	})
}
