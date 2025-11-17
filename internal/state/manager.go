package state

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/meeting-siren/meeting-siren/internal/domain"
)

type Manager struct {
	stateDir string
	logger   domain.Logger
	mu       sync.RWMutex
	events   map[string]*domain.AlarmEvent
}

type eventRecord struct {
	UID                 string    `json:"uid"`
	Title               string    `json:"title"`
	When                time.Time `json:"when"`
	LastFired           time.Time `json:"last_fired"`
	FireCount           int       `json:"fire_count"`
	IsFinalNotification bool      `json:"is_final_notification"`
}

func NewManager(stateDir string, logger domain.Logger) (*Manager, error) {
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return nil, fmt.Errorf("creating state directory: %w", err)
	}

	m := &Manager{
		stateDir: stateDir,
		logger:   logger,
		events:   make(map[string]*domain.AlarmEvent),
	}

	if err := m.loadState(); err != nil {
		logger.Error("Failed to load existing state", err, nil)
	}

	return m, nil
}

func (m *Manager) ShouldFire(event *domain.AlarmEvent) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	uid := event.UID
	stateFile := m.getStateFilePath(uid)

	// Check if we have a record of this event
	record, exists := m.loadEventRecord(stateFile)
	if !exists {
		m.logger.Debug("Event not seen before, should fire", map[string]interface{}{
			"uid": uid,
		})
		return true, nil
	}

	// Check if this is the same event (same title and time)
	if record.Title != event.Alert.Title || !record.When.Equal(event.Alert.When) {
		m.logger.Debug("Event details changed, should fire", map[string]interface{}{
			"uid":       uid,
			"old_title": record.Title,
			"new_title": event.Alert.Title,
			"old_when":  record.When,
			"new_when":  event.Alert.When,
		})
		return true, nil
	}

	// Check if previous fire was a final notification
	if record.IsFinalNotification {
		m.logger.Debug("Previous fire was final notification, no more snoozes allowed", map[string]interface{}{
			"uid":        uid,
			"last_fired": record.LastFired,
		})
		return false, nil
	}

	// Check if enough time has passed since last fire
	now := time.Now()
	timeSinceLast := now.Sub(record.LastFired)

	// If the event was fired recently (within the last minute), don't fire again
	// This prevents rapid duplicate firing
	if timeSinceLast < time.Minute {
		m.logger.Debug("Event fired recently, skipping", map[string]interface{}{
			"uid":             uid,
			"time_since_last": timeSinceLast.String(),
			"last_fired":      record.LastFired,
		})
		return false, nil
	}

	m.logger.Debug("Sufficient time passed since last fire, should fire", map[string]interface{}{
		"uid":             uid,
		"time_since_last": timeSinceLast.String(),
		"last_fired":      record.LastFired,
	})
	return true, nil
}

func (m *Manager) RecordFired(event *domain.AlarmEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	uid := event.UID
	stateFile := m.getStateFilePath(uid)

	record, _ := m.loadEventRecord(stateFile)

	record.UID = uid
	record.Title = event.Alert.Title
	record.When = event.Alert.When
	record.LastFired = time.Now()
	record.FireCount++
	record.IsFinalNotification = event.Alert.IsFinalNotification

	if err := m.saveEventRecord(stateFile, record); err != nil {
		return fmt.Errorf("saving event record: %w", err)
	}

	m.events[uid] = event

	m.logger.Info("Recorded event fire", map[string]interface{}{
		"uid":        uid,
		"fire_count": record.FireCount,
		"fired_at":   record.LastFired,
	})

	return nil
}

func (m *Manager) Cleanup(olderThan time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cutoff := time.Now().Add(-olderThan)
	cleaned := 0

	files, err := os.ReadDir(m.stateDir)
	if err != nil {
		return fmt.Errorf("reading state directory: %w", err)
	}

	for _, file := range files {
		if !file.Type().IsRegular() || filepath.Ext(file.Name()) != ".json" {
			continue
		}

		filePath := filepath.Join(m.stateDir, file.Name())

		// Check file modification time for cleanup decision
		fileInfo, err := file.Info()
		if err != nil {
			continue
		}

		if fileInfo.ModTime().Before(cutoff) {
			// Load record to get UID for cleanup
			record, exists := m.loadEventRecord(filePath)

			if err := os.Remove(filePath); err != nil {
				m.logger.Error("Failed to remove old state file", err, map[string]interface{}{
					"file": filePath,
				})
				continue
			}

			if exists {
				delete(m.events, record.UID)
			}
			cleaned++
		}
	}

	if cleaned > 0 {
		m.logger.Info("Cleaned up old state files", map[string]interface{}{
			"cleaned": cleaned,
			"cutoff":  cutoff,
		})
	}

	return nil
}

func (m *Manager) loadState() error {
	files, err := os.ReadDir(m.stateDir)
	if err != nil {
		return fmt.Errorf("reading state directory: %w", err)
	}

	loaded := 0
	for _, file := range files {
		if !file.Type().IsRegular() || filepath.Ext(file.Name()) != ".json" {
			continue
		}

		filePath := filepath.Join(m.stateDir, file.Name())
		record, exists := m.loadEventRecord(filePath)
		if exists {
			// Create a minimal AlarmEvent for the in-memory cache
			event := &domain.AlarmEvent{
				Alert: &domain.MeetingAlert{
					Title: record.Title,
					When:  record.When,
				},
				UID:       record.UID,
				Timestamp: record.LastFired,
			}
			m.events[record.UID] = event
			loaded++
		}
	}

	m.logger.Info("Loaded existing state", map[string]interface{}{
		"events_loaded": loaded,
	})

	return nil
}

func (m *Manager) getStateFilePath(uid string) string {
	// Create a safe filename from the UID hash
	hash := sha256.Sum256([]byte(uid))
	filename := hex.EncodeToString(hash[:])[:16] + ".json"
	return filepath.Join(m.stateDir, filename)
}

func (m *Manager) loadEventRecord(filePath string) (*eventRecord, bool) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return &eventRecord{}, false
	}

	var record eventRecord
	if err := json.Unmarshal(data, &record); err != nil {
		m.logger.Error("Failed to unmarshal event record", err, map[string]interface{}{
			"file": filePath,
		})
		return &eventRecord{}, false
	}

	return &record, true
}

func (m *Manager) saveEventRecord(filePath string, record *eventRecord) error {
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling event record: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("writing event record file: %w", err)
	}

	return nil
}

// GetStats returns statistics about the state manager
func (m *Manager) GetStats() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return map[string]interface{}{
		"total_events": len(m.events),
		"state_dir":    m.stateDir,
	}
}
