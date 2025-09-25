package state

import (
	"os"
	"testing"
	"time"

	"github.com/meeting-siren/meeting-siren/internal/domain"
	"github.com/meeting-siren/meeting-siren/test/mocks"
)

func TestManager_ShouldFire(t *testing.T) {
	tempDir := t.TempDir()
	logger := mocks.NewMockLogger()

	manager, err := NewManager(tempDir, logger)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	now := time.Now()
	alert := &domain.MeetingAlert{
		Title: "Test Meeting",
		When:  now.Add(10 * time.Minute),
		Lead:  10,
	}
	event := &domain.AlarmEvent{
		Alert:     alert,
		UID:       alert.EventUID(),
		Timestamp: now,
	}

	t.Run("first time should fire", func(t *testing.T) {
		shouldFire, err := manager.ShouldFire(event)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !shouldFire {
			t.Errorf("expected to fire on first occurrence")
		}
	})

	t.Run("after recording should not fire immediately", func(t *testing.T) {
		err := manager.RecordFired(event)
		if err != nil {
			t.Fatalf("failed to record fired: %v", err)
		}

		shouldFire, err := manager.ShouldFire(event)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if shouldFire {
			t.Errorf("expected not to fire immediately after recording")
		}
	})

	t.Run("different event should fire", func(t *testing.T) {
		differentAlert := &domain.MeetingAlert{
			Title: "Different Meeting",
			When:  now.Add(15 * time.Minute),
			Lead:  15,
		}
		differentEvent := &domain.AlarmEvent{
			Alert:     differentAlert,
			UID:       differentAlert.EventUID(),
			Timestamp: now,
		}

		shouldFire, err := manager.ShouldFire(differentEvent)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !shouldFire {
			t.Errorf("expected to fire for different event")
		}
	})
}

func TestManager_Cleanup(t *testing.T) {
	tempDir := t.TempDir()
	logger := mocks.NewMockLogger()

	manager, err := NewManager(tempDir, logger)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	// Create some test events with different timestamps
	now := time.Now()

	// Create old events that should be cleaned up
	for i := 0; i < 2; i++ {
		alert := &domain.MeetingAlert{
			Title: "Old Meeting " + string(rune('A'+i)),
			When:  now.Add(-time.Duration(i+2) * time.Hour),
			Lead:  10,
		}
		event := &domain.AlarmEvent{
			Alert:     alert,
			UID:       alert.EventUID(),
			Timestamp: now.Add(-time.Duration(i+2) * time.Hour),
		}

		// Record and then manually adjust the file timestamps to simulate old events
		err := manager.RecordFired(event)
		if err != nil {
			t.Fatalf("failed to record old event: %v", err)
		}

		// Manually set file timestamp to be old
		stateFile := manager.getStateFilePath(event.UID)
		oldTime := now.Add(-2 * time.Hour)
		os.Chtimes(stateFile, oldTime, oldTime)
	}

	// Create a recent event that should not be cleaned up
	recentAlert := &domain.MeetingAlert{
		Title: "Recent Meeting",
		When:  now.Add(10 * time.Minute),
		Lead:  10,
	}
	recentEvent := &domain.AlarmEvent{
		Alert:     recentAlert,
		UID:       recentAlert.EventUID(),
		Timestamp: now,
	}
	err = manager.RecordFired(recentEvent)
	if err != nil {
		t.Fatalf("failed to record recent event: %v", err)
	}

	// Clean up events older than 30 minutes
	err = manager.Cleanup(30 * time.Minute)
	if err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}

	// Verify some events were cleaned up
	files, err := os.ReadDir(tempDir)
	if err != nil {
		t.Fatalf("failed to read temp dir: %v", err)
	}

	// Should have fewer files after cleanup
	if len(files) >= 3 {
		t.Errorf("expected cleanup to remove some files, but found %d files", len(files))
	}
}

func TestManager_EventUID(t *testing.T) {
	alert1 := &domain.MeetingAlert{
		Title: "Same Meeting",
		When:  time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
		Lead:  10,
	}

	alert2 := &domain.MeetingAlert{
		Title: "Same Meeting",
		When:  time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
		Lead:  15, // Different lead time
	}

	alert3 := &domain.MeetingAlert{
		Title: "Different Meeting",
		When:  time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
		Lead:  10,
	}

	uid1 := alert1.EventUID()
	uid2 := alert2.EventUID()
	uid3 := alert3.EventUID()

	// Same title and time should have same UID regardless of lead time
	if uid1 != uid2 {
		t.Errorf("expected same UID for same title/time, got %s != %s", uid1, uid2)
	}

	// Different title should have different UID
	if uid1 == uid3 {
		t.Errorf("expected different UID for different title, got %s == %s", uid1, uid3)
	}
}
