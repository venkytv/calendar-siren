package state

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/meeting-siren/meeting-siren/internal/domain"
)

type Scheduler struct {
	config *domain.Config
	logger domain.Logger
}

func NewScheduler(config *domain.Config, logger domain.Logger) *Scheduler {
	return &Scheduler{
		config: config,
		logger: logger,
	}
}

func (s *Scheduler) ShouldFire(when time.Time) bool {
	// Check if it's a quiet day
	if s.isQuietDay(when) {
		s.logger.Debug("Skipping alarm on quiet day", map[string]interface{}{
			"day": when.Weekday().String(),
		})
		return false
	}

	// Check work hours
	if !s.isWorkHours(when) {
		s.logger.Debug("Skipping alarm outside work hours", map[string]interface{}{
			"current_time": when.Format("15:04"),
			"work_hours":   s.config.WorkHours,
		})
		return false
	}

	// Check if snooze is active
	if s.isSnoozeActive() {
		s.logger.Debug("Skipping alarm due to active snooze", map[string]interface{}{
			"snooze_file": s.config.SnoozeFile,
		})
		return false
	}

	return true
}

func (s *Scheduler) isQuietDay(t time.Time) bool {
	if s.config.QuietDays == "" {
		return false
	}

	weekday := t.Weekday().String()
	quietDays := strings.Split(s.config.QuietDays, ",")

	for _, day := range quietDays {
		day = strings.TrimSpace(day)
		if strings.EqualFold(day, weekday) || strings.EqualFold(day, weekday[:3]) {
			return true
		}
	}

	return false
}

func (s *Scheduler) isWorkHours(t time.Time) bool {
	if s.config.WorkHours == "" {
		return true // No work hours restriction
	}

	parts := strings.Split(s.config.WorkHours, "-")
	if len(parts) != 2 {
		s.logger.Error("Invalid work hours format", nil, map[string]interface{}{
			"work_hours": s.config.WorkHours,
		})
		return true // Allow if invalid format
	}

	startTime, err := parseTime(parts[0])
	if err != nil {
		s.logger.Error("Invalid start time in work hours", err, map[string]interface{}{
			"start_time": parts[0],
		})
		return true
	}

	endTime, err := parseTime(parts[1])
	if err != nil {
		s.logger.Error("Invalid end time in work hours", err, map[string]interface{}{
			"end_time": parts[1],
		})
		return true
	}

	currentTime := t.Hour()*60 + t.Minute()

	// Handle cases where work hours span midnight
	if startTime <= endTime {
		return currentTime >= startTime && currentTime <= endTime
	} else {
		return currentTime >= startTime || currentTime <= endTime
	}
}

func (s *Scheduler) isSnoozeActive() bool {
	if s.config.SnoozeFile == "" {
		return false
	}

	// Check if snooze file exists
	if _, err := os.Stat(s.config.SnoozeFile); os.IsNotExist(err) {
		return false
	}

	// Read file modification time
	info, err := os.Stat(s.config.SnoozeFile)
	if err != nil {
		return false
	}

	// Check if snooze period has elapsed
	snoozeDuration := time.Duration(s.config.SnoozeMinutes) * time.Minute
	if time.Since(info.ModTime()) > snoozeDuration {
		// Snooze expired, remove file
		os.Remove(s.config.SnoozeFile)
		return false
	}

	return true
}

func parseTime(timeStr string) (int, error) {
	parts := strings.Split(strings.TrimSpace(timeStr), ":")
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid time format: %s", timeStr)
	}

	hour, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("invalid hour: %s", parts[0])
	}

	minute, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, fmt.Errorf("invalid minute: %s", parts[1])
	}

	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, fmt.Errorf("time out of range: %02d:%02d", hour, minute)
	}

	return hour*60 + minute, nil
}
