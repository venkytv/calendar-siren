package daemon

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/meeting-siren/meeting-siren/internal/domain"
)

type Daemon struct {
	config             *domain.Config
	logger             domain.Logger
	subscriber         domain.MessageSubscriber
	player             domain.AudioPlayer
	stateManager       domain.StateManager
	scheduler          domain.Scheduler
	heartbeatPublisher domain.HeartbeatPublisher

	// Internal state
	mu       sync.RWMutex
	running  bool
	stopChan chan struct{}
	wg       sync.WaitGroup
}

func NewDaemon(
	config *domain.Config,
	logger domain.Logger,
	subscriber domain.MessageSubscriber,
	player domain.AudioPlayer,
	stateManager domain.StateManager,
	scheduler domain.Scheduler,
	heartbeatPublisher domain.HeartbeatPublisher,
) *Daemon {
	return &Daemon{
		config:             config,
		logger:             logger,
		subscriber:         subscriber,
		player:             player,
		stateManager:       stateManager,
		scheduler:          scheduler,
		heartbeatPublisher: heartbeatPublisher,
		stopChan:           make(chan struct{}),
	}
}

func (d *Daemon) Start(ctx context.Context) error {
	d.mu.Lock()
	if d.running {
		d.mu.Unlock()
		return fmt.Errorf("daemon already running")
	}
	d.running = true
	d.mu.Unlock()

	d.logger.Info("Starting meeting-siren daemon", map[string]interface{}{
		"nats_url":          d.config.NATSUrl,
		"nats_subject":      d.config.NATSSubject,
		"state_dir":         d.config.StateDir,
		"heartbeat_enabled": d.config.HeartbeatEnabled,
	})

	// Start heartbeat publisher if configured
	if d.heartbeatPublisher != nil {
		if err := d.heartbeatPublisher.Start(ctx); err != nil {
			return fmt.Errorf("failed to start heartbeat publisher: %w", err)
		}
	}

	// Start cleanup routine
	d.wg.Add(1)
	go d.cleanupRoutine(ctx)

	// Start message subscription
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		if err := d.subscriber.Subscribe(ctx, d.config.NATSSubject, d.handleMeetingAlert); err != nil && err != context.Canceled {
			d.logger.Error("NATS subscription failed", err, nil)
		}
	}()

	// Wait for shutdown signal
	select {
	case <-ctx.Done():
		d.logger.Info("Received shutdown signal", nil)
	case <-d.stopChan:
		d.logger.Info("Received stop signal", nil)
	}

	return d.shutdown()
}

func (d *Daemon) Stop() error {
	d.mu.Lock()
	if !d.running {
		d.mu.Unlock()
		return fmt.Errorf("daemon not running")
	}
	d.mu.Unlock()

	close(d.stopChan)
	return nil
}

func (d *Daemon) shutdown() error {
	d.logger.Info("Shutting down daemon", nil)

	// Stop heartbeat publisher
	if d.heartbeatPublisher != nil {
		if err := d.heartbeatPublisher.Stop(); err != nil {
			d.logger.Error("Error stopping heartbeat publisher", err, nil)
		}
	}

	// Close NATS subscriber
	if err := d.subscriber.Close(); err != nil {
		d.logger.Error("Error closing NATS subscriber", err, nil)
	}

	// Wait for all goroutines to finish
	d.wg.Wait()

	d.mu.Lock()
	d.running = false
	d.mu.Unlock()

	d.logger.Info("Daemon shutdown complete", nil)
	return nil
}

func (d *Daemon) handleMeetingAlert(alert *domain.MeetingAlert) {
	event := &domain.AlarmEvent{
		Alert:     alert,
		UID:       alert.EventUID(),
		Timestamp: time.Now(),
	}

	d.logger.Info("Processing meeting alert", map[string]interface{}{
		"uid":      event.UID,
		"title":    alert.Title,
		"when":     alert.When,
		"lead":     alert.Lead,
		"severity": alert.Severity,
	})

	// Check if we should fire this alarm based on schedule
	if !d.scheduler.ShouldFire(time.Now()) {
		d.logger.Info("Alarm skipped due to schedule restrictions", map[string]interface{}{
			"uid": event.UID,
		})
		return
	}

	// Check if we should fire this alarm based on deduplication
	shouldFire, err := d.stateManager.ShouldFire(event)
	if err != nil {
		d.logger.Error("Error checking if alarm should fire", err, map[string]interface{}{
			"uid": event.UID,
		})
		return
	}

	if !shouldFire {
		d.logger.Info("Alarm skipped due to deduplication", map[string]interface{}{
			"uid": event.UID,
		})
		return
	}

	// Fire the alarm
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		d.fireAlarm(event)
	}()
}

func (d *Daemon) fireAlarm(event *domain.AlarmEvent) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	d.logger.Info("Firing alarm", map[string]interface{}{
		"uid":   event.UID,
		"title": event.Alert.Title,
	})

	// Record that we're firing this alarm
	if err := d.stateManager.RecordFired(event); err != nil {
		d.logger.Error("Failed to record alarm fire", err, map[string]interface{}{
			"uid": event.UID,
		})
	}

	// Play alarm sounds
	sounds := d.selectSounds(event.Alert)
	if len(sounds) > 0 {
		if err := d.player.Play(ctx, sounds); err != nil {
			d.logger.Error("Failed to play alarm sounds", err, map[string]interface{}{
				"uid":    event.UID,
				"sounds": sounds,
			})
		}
	}

	// Play TTS if enabled
	if d.config.TTSEnabled {
		message, err := d.renderTTSMessage(event.Alert)
		if err != nil {
			d.logger.Error("Failed to render TTS message", err, map[string]interface{}{
				"uid": event.UID,
			})
		} else if message != "" { // Only play if message is not empty (empty means skip TTS)
			if err := d.player.PlayTTS(ctx, message); err != nil {
				d.logger.Error("Failed to play TTS", err, map[string]interface{}{
					"uid":     event.UID,
					"message": message,
				})
			}
		}
	}

	// Activate GPIO buzzer if configured
	if gpioPlayer, ok := d.player.(interface{ GPIOBuzzer(context.Context) error }); ok {
		if err := gpioPlayer.GPIOBuzzer(ctx); err != nil {
			d.logger.Error("Failed to activate GPIO buzzer", err, map[string]interface{}{
				"uid": event.UID,
			})
		}
	}

	// Handle repeats if configured
	if d.config.RepeatSeconds > 0 && d.config.MaxRepeats > 0 {
		d.handleRepeats(ctx, event)
	}
}

func (d *Daemon) handleRepeats(ctx context.Context, event *domain.AlarmEvent) {
	repeatInterval := time.Duration(d.config.RepeatSeconds) * time.Second

	for i := 1; i < d.config.MaxRepeats; i++ {
		select {
		case <-ctx.Done():
			return
		case <-time.After(repeatInterval):
			// Stop repeating if the meeting has already started
			if time.Now().After(event.Alert.When) {
				d.logger.Info("Stopping alarm repeats: meeting has started", map[string]interface{}{
					"uid":            event.UID,
					"meeting_time":   event.Alert.When,
					"completed_reps": i,
				})
				return
			}

			d.logger.Info("Playing alarm repeat", map[string]interface{}{
				"uid":         event.UID,
				"repeat_num":  i + 1,
				"max_repeats": d.config.MaxRepeats,
			})

			// Play sounds again
			sounds := d.selectSounds(event.Alert)
			if len(sounds) > 0 {
				if err := d.player.Play(ctx, sounds); err != nil {
					d.logger.Error("Failed to play repeat alarm", err, map[string]interface{}{
						"uid":        event.UID,
						"repeat_num": i + 1,
					})
				}
			}

			// Play TTS again if enabled
			if d.config.TTSEnabled {
				message, err := d.renderTTSMessage(event.Alert)
				if err == nil && message != "" { // Only play if message is not empty
					d.player.PlayTTS(ctx, message)
				}
			}
		}
	}
}

// selectSounds returns the appropriate sound files based on whether this is a final notification
func (d *Daemon) selectSounds(alert *domain.MeetingAlert) []string {
	// If this is a final notification and final notification sounds are configured, use those
	if alert.IsFinalNotification && len(d.config.FinalNotificationSounds) > 0 {
		return d.config.FinalNotificationSounds
	}
	// Otherwise, use the regular sounds
	return d.config.Sounds
}

// selectTTSTemplate returns the appropriate TTS template based on whether this is a final notification
// Returns the template string and a boolean indicating whether TTS should be skipped
func (d *Daemon) selectTTSTemplate(alert *domain.MeetingAlert) (string, bool) {
	// If this is a final notification and final notification TTS template is configured
	if alert.IsFinalNotification && d.config.FinalNotificationTTSTemplate != nil {
		template := *d.config.FinalNotificationTTSTemplate
		// If explicitly set to empty string, skip TTS
		if template == "" {
			return "", true // skip TTS
		}
		// Use the configured final notification template
		return template, false
	}
	// Otherwise, use the regular template
	return d.config.TTSTemplate, false
}

// renderTTSMessage renders a TTS message using the appropriate template
func (d *Daemon) renderTTSMessage(alert *domain.MeetingAlert) (string, error) {
	template, skipTTS := d.selectTTSTemplate(alert)

	// If TTS should be skipped for this notification, return empty string
	if skipTTS {
		return "", nil
	}

	// Use the RenderTTSMessageWithTemplate method if available
	if ttsPlayer, ok := d.player.(interface {
		RenderTTSMessageWithTemplate(*domain.MeetingAlert, string) (string, error)
	}); ok {
		return ttsPlayer.RenderTTSMessageWithTemplate(alert, template)
	}

	// Fallback to RenderTTSMessage if RenderTTSMessageWithTemplate is not available
	if ttsPlayer, ok := d.player.(interface {
		RenderTTSMessage(*domain.MeetingAlert) (string, error)
	}); ok {
		return ttsPlayer.RenderTTSMessage(alert)
	}

	return "", fmt.Errorf("player does not support TTS rendering")
}

func (d *Daemon) cleanupRoutine(ctx context.Context) {
	defer d.wg.Done()

	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Clean up state files older than 24 hours
			if err := d.stateManager.Cleanup(24 * time.Hour); err != nil {
				d.logger.Error("Failed to cleanup old state files", err, nil)
			}
		}
	}
}

// Health returns the daemon health status
func (d *Daemon) Health() map[string]interface{} {
	d.mu.RLock()
	defer d.mu.RUnlock()

	health := map[string]interface{}{
		"running":   d.running,
		"service":   "meeting-siren",
		"timestamp": time.Now(),
	}

	// Add NATS health if available
	if natsHealth, ok := d.subscriber.(interface{ Health() map[string]interface{} }); ok {
		health["nats"] = natsHealth.Health()
	}

	// Add state manager stats if available
	if stateStats, ok := d.stateManager.(interface{ GetStats() map[string]interface{} }); ok {
		health["state"] = stateStats.GetStats()
	}

	return health
}
