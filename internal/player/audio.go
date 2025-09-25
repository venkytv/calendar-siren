package player

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"sync"
	"text/template"
	"time"

	"github.com/meeting-siren/meeting-siren/internal/domain"
)

type Player struct {
	config *domain.Config
	logger domain.Logger
	mu     sync.Mutex
}

func NewPlayer(config *domain.Config, logger domain.Logger) *Player {
	return &Player{
		config: config,
		logger: logger,
	}
}

func (p *Player) Play(ctx context.Context, soundFiles []string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(soundFiles) == 0 {
		p.logger.Info("No sound files to play", nil)
		return nil
	}

	// Set volume before playing
	if err := p.setVolumeInternal(ctx, p.config.VolumePct); err != nil {
		p.logger.Error("Failed to set volume", err, map[string]interface{}{
			"volume_pct": p.config.VolumePct,
		})
	}

	// Play each sound file
	for _, soundFile := range soundFiles {
		if err := p.playFile(ctx, soundFile); err != nil {
			p.logger.Error("Failed to play sound file", err, map[string]interface{}{
				"file": soundFile,
			})
			continue
		}

		p.logger.Info("Played sound file", map[string]interface{}{
			"file": soundFile,
		})
	}

	return nil
}

func (p *Player) SetVolume(ctx context.Context, percent int) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.setVolumeInternal(ctx, percent)
}

func (p *Player) setVolumeInternal(ctx context.Context, percent int) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		// macOS: use osascript to set volume
		volume := float64(percent) / 100.0 * 7.0 // macOS volume scale 0-7
		cmd = exec.CommandContext(ctx, "osascript", "-e", fmt.Sprintf("set volume output volume %.0f", volume))
	case "linux":
		// Linux: use amixer to set volume
		cmd = exec.CommandContext(ctx, "amixer", "sset", "Master", fmt.Sprintf("%d%%", percent))
	default:
		return fmt.Errorf("volume control not supported on %s", runtime.GOOS)
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("setting volume to %d%%: %w", percent, err)
	}

	return nil
}

func (p *Player) playFile(ctx context.Context, soundFile string) error {
	if _, err := os.Stat(soundFile); os.IsNotExist(err) {
		return fmt.Errorf("sound file does not exist: %s", soundFile)
	}

	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		// macOS: use afplay
		cmd = exec.CommandContext(ctx, "afplay", soundFile)
	case "linux":
		// Linux: try mpg123 first, then aplay
		if _, err := exec.LookPath("mpg123"); err == nil {
			cmd = exec.CommandContext(ctx, "mpg123", "-q", soundFile)
		} else if _, err := exec.LookPath("aplay"); err == nil {
			cmd = exec.CommandContext(ctx, "aplay", soundFile)
		} else {
			return fmt.Errorf("no audio player found (mpg123 or aplay required)")
		}
	default:
		return fmt.Errorf("audio playback not supported on %s", runtime.GOOS)
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("playing audio file %s: %w", soundFile, err)
	}

	return nil
}

func (p *Player) PlayTTS(ctx context.Context, message string) error {
	if !p.config.TTSEnabled {
		return nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		// macOS: use say command
		cmd = exec.CommandContext(ctx, "say", message)
	case "linux":
		// Linux: use espeak-ng if available
		if _, err := exec.LookPath("espeak-ng"); err == nil {
			cmd = exec.CommandContext(ctx, "espeak-ng", message)
		} else if _, err := exec.LookPath("espeak"); err == nil {
			cmd = exec.CommandContext(ctx, "espeak", message)
		} else {
			p.logger.Error("TTS not available", fmt.Errorf("espeak-ng or espeak not found"), nil)
			return nil
		}
	default:
		return fmt.Errorf("TTS not supported on %s", runtime.GOOS)
	}

	p.logger.Info("Playing TTS message", map[string]interface{}{
		"message": message,
	})

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("playing TTS message: %w", err)
	}

	return nil
}

func (p *Player) RenderTTSMessage(alert *domain.MeetingAlert) (string, error) {
	if p.config.TTSTemplate == "" {
		return fmt.Sprintf("Meeting alert: %s in %d minutes", alert.Title, alert.Lead), nil
	}

	tmpl, err := template.New("tts").Parse(p.config.TTSTemplate)
	if err != nil {
		return "", fmt.Errorf("parsing TTS template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, alert); err != nil {
		return "", fmt.Errorf("executing TTS template: %w", err)
	}

	return buf.String(), nil
}

// GPIOBuzzer controls a GPIO buzzer on Raspberry Pi
func (p *Player) GPIOBuzzer(ctx context.Context) error {
	if p.config.GPIOBuzzerPin == nil || runtime.GOOS != "linux" {
		return nil
	}

	pin := *p.config.GPIOBuzzerPin
	pinStr := strconv.Itoa(pin)

	// Export GPIO pin
	if err := p.writeGPIO("/sys/class/gpio/export", pinStr); err != nil {
		return fmt.Errorf("exporting GPIO pin %d: %w", pin, err)
	}

	// Set pin direction to output
	directionPath := fmt.Sprintf("/sys/class/gpio/gpio%d/direction", pin)
	if err := p.writeGPIO(directionPath, "out"); err != nil {
		return fmt.Errorf("setting GPIO pin %d direction: %w", pin, err)
	}

	valuePath := fmt.Sprintf("/sys/class/gpio/gpio%d/value", pin)

	// Buzz for 3 seconds
	p.logger.Info("Activating GPIO buzzer", map[string]interface{}{
		"pin": pin,
	})

	// Turn on
	if err := p.writeGPIO(valuePath, "1"); err != nil {
		return fmt.Errorf("turning on GPIO pin %d: %w", pin, err)
	}

	// Wait 3 seconds
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(3 * time.Second):
	}

	// Turn off
	if err := p.writeGPIO(valuePath, "0"); err != nil {
		return fmt.Errorf("turning off GPIO pin %d: %w", pin, err)
	}

	return nil
}

func (p *Player) writeGPIO(path, value string) error {
	file, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.WriteString(value)
	return err
}
