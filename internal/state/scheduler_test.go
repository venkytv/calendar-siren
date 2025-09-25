package state

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/meeting-siren/meeting-siren/internal/domain"
	"github.com/meeting-siren/meeting-siren/test/mocks"
)

func TestScheduler_ShouldFire(t *testing.T) {
	tests := []struct {
		name       string
		config     *domain.Config
		testTime   time.Time
		shouldFire bool
	}{
		{
			name: "during work hours on weekday",
			config: &domain.Config{
				WorkHours: "09:00-17:00",
				QuietDays: "Sat,Sun",
			},
			testTime:   time.Date(2025, 1, 6, 14, 0, 0, 0, time.UTC), // Monday 2PM
			shouldFire: true,
		},
		{
			name: "outside work hours",
			config: &domain.Config{
				WorkHours: "09:00-17:00",
				QuietDays: "Sat,Sun",
			},
			testTime:   time.Date(2025, 1, 6, 19, 0, 0, 0, time.UTC), // Monday 7PM
			shouldFire: false,
		},
		{
			name: "on quiet day",
			config: &domain.Config{
				WorkHours: "09:00-17:00",
				QuietDays: "Sat,Sun",
			},
			testTime:   time.Date(2025, 1, 5, 14, 0, 0, 0, time.UTC), // Sunday 2PM
			shouldFire: false,
		},
		{
			name: "no restrictions",
			config: &domain.Config{
				WorkHours: "",
				QuietDays: "",
			},
			testTime:   time.Date(2025, 1, 5, 2, 0, 0, 0, time.UTC), // Sunday 2AM
			shouldFire: true,
		},
		{
			name: "work hours spanning midnight",
			config: &domain.Config{
				WorkHours: "22:00-06:00",
				QuietDays: "",
			},
			testTime:   time.Date(2025, 1, 6, 2, 0, 0, 0, time.UTC), // Monday 2AM
			shouldFire: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := mocks.NewMockLogger()
			scheduler := NewScheduler(tt.config, logger)

			result := scheduler.ShouldFire(tt.testTime)
			if result != tt.shouldFire {
				t.Errorf("ShouldFire() = %v, want %v", result, tt.shouldFire)
			}
		})
	}
}

func TestScheduler_IsQuietDay(t *testing.T) {
	tests := []struct {
		name      string
		quietDays string
		testTime  time.Time
		isQuiet   bool
	}{
		{
			name:      "saturday in Sat,Sun",
			quietDays: "Sat,Sun",
			testTime:  time.Date(2025, 1, 4, 10, 0, 0, 0, time.UTC), // Saturday
			isQuiet:   true,
		},
		{
			name:      "monday not in Sat,Sun",
			quietDays: "Sat,Sun",
			testTime:  time.Date(2025, 1, 6, 10, 0, 0, 0, time.UTC), // Monday
			isQuiet:   false,
		},
		{
			name:      "full day names",
			quietDays: "Saturday,Sunday",
			testTime:  time.Date(2025, 1, 4, 10, 0, 0, 0, time.UTC), // Saturday
			isQuiet:   true,
		},
		{
			name:      "custom quiet days",
			quietDays: "Mon,Wed,Fri",
			testTime:  time.Date(2025, 1, 6, 10, 0, 0, 0, time.UTC), // Monday
			isQuiet:   true,
		},
		{
			name:      "no quiet days",
			quietDays: "",
			testTime:  time.Date(2025, 1, 4, 10, 0, 0, 0, time.UTC), // Saturday
			isQuiet:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &domain.Config{QuietDays: tt.quietDays}
			logger := mocks.NewMockLogger()
			scheduler := NewScheduler(config, logger)

			result := scheduler.isQuietDay(tt.testTime)
			if result != tt.isQuiet {
				t.Errorf("isQuietDay() = %v, want %v", result, tt.isQuiet)
			}
		})
	}
}

func TestScheduler_IsWorkHours(t *testing.T) {
	tests := []struct {
		name       string
		workHours  string
		testTime   time.Time
		isWorkTime bool
	}{
		{
			name:       "during work hours",
			workHours:  "09:00-17:00",
			testTime:   time.Date(2025, 1, 6, 14, 0, 0, 0, time.UTC),
			isWorkTime: true,
		},
		{
			name:       "before work hours",
			workHours:  "09:00-17:00",
			testTime:   time.Date(2025, 1, 6, 8, 0, 0, 0, time.UTC),
			isWorkTime: false,
		},
		{
			name:       "after work hours",
			workHours:  "09:00-17:00",
			testTime:   time.Date(2025, 1, 6, 18, 0, 0, 0, time.UTC),
			isWorkTime: false,
		},
		{
			name:       "spanning midnight - during night hours",
			workHours:  "22:00-06:00",
			testTime:   time.Date(2025, 1, 6, 2, 0, 0, 0, time.UTC),
			isWorkTime: true,
		},
		{
			name:       "spanning midnight - during day hours",
			workHours:  "22:00-06:00",
			testTime:   time.Date(2025, 1, 6, 14, 0, 0, 0, time.UTC),
			isWorkTime: false,
		},
		{
			name:       "no work hours restriction",
			workHours:  "",
			testTime:   time.Date(2025, 1, 6, 2, 0, 0, 0, time.UTC),
			isWorkTime: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &domain.Config{WorkHours: tt.workHours}
			logger := mocks.NewMockLogger()
			scheduler := NewScheduler(config, logger)

			result := scheduler.isWorkHours(tt.testTime)
			if result != tt.isWorkTime {
				t.Errorf("isWorkHours() = %v, want %v", result, tt.isWorkTime)
			}
		})
	}
}

func TestScheduler_IsSnoozeActive(t *testing.T) {
	tempDir := t.TempDir()
	snoozeFile := filepath.Join(tempDir, "snooze")

	tests := []struct {
		name         string
		config       *domain.Config
		setupSnooze  bool
		snoozeAge    time.Duration
		expectActive bool
	}{
		{
			name: "no snooze file configured",
			config: &domain.Config{
				SnoozeFile: "",
			},
			expectActive: false,
		},
		{
			name: "snooze file doesn't exist",
			config: &domain.Config{
				SnoozeFile:    snoozeFile,
				SnoozeMinutes: 5,
			},
			setupSnooze:  false,
			expectActive: false,
		},
		{
			name: "recent snooze file",
			config: &domain.Config{
				SnoozeFile:    snoozeFile,
				SnoozeMinutes: 5,
			},
			setupSnooze:  true,
			snoozeAge:    2 * time.Minute,
			expectActive: true,
		},
		{
			name: "expired snooze file",
			config: &domain.Config{
				SnoozeFile:    snoozeFile,
				SnoozeMinutes: 5,
			},
			setupSnooze:  true,
			snoozeAge:    10 * time.Minute,
			expectActive: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clean up any existing snooze file
			os.Remove(snoozeFile)

			if tt.setupSnooze {
				// Create snooze file with specific age
				if err := os.WriteFile(snoozeFile, []byte("snooze"), 0644); err != nil {
					t.Fatalf("failed to create snooze file: %v", err)
				}

				// Set the modification time to simulate age
				pastTime := time.Now().Add(-tt.snoozeAge)
				if err := os.Chtimes(snoozeFile, pastTime, pastTime); err != nil {
					t.Fatalf("failed to set snooze file time: %v", err)
				}
			}

			logger := mocks.NewMockLogger()
			scheduler := NewScheduler(tt.config, logger)

			result := scheduler.isSnoozeActive()
			if result != tt.expectActive {
				t.Errorf("isSnoozeActive() = %v, want %v", result, tt.expectActive)
			}

			// If snooze was expired, file should be removed
			if tt.setupSnooze && !tt.expectActive && tt.config.SnoozeFile != "" {
				if _, err := os.Stat(snoozeFile); !os.IsNotExist(err) {
					t.Errorf("expected expired snooze file to be removed")
				}
			}
		})
	}
}
