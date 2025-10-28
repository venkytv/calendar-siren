package player

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
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

	// Skip volume control if disabled or on systems without audio mixer
	var originalVolume int = -1

	// Only attempt volume control if we can detect audio system
	if p.hasAudioMixer() {
		// Save current volume before changing it
		vol, err := p.getCurrentVolume(ctx)
		if err != nil {
			p.logger.Debug("Cannot get current volume, skipping volume control", map[string]interface{}{
				"error": err.Error(),
			})
		} else {
			originalVolume = vol
			p.logger.Debug("Saved original volume", map[string]interface{}{
				"original_volume": originalVolume,
			})

			// Set volume for alarm
			if err := p.setVolumeInternal(ctx, p.config.VolumePct); err != nil {
				p.logger.Debug("Cannot set alarm volume, continuing without volume control", map[string]interface{}{
					"alarm_volume": p.config.VolumePct,
					"error": err.Error(),
				})
				originalVolume = -1 // Mark as unavailable since setting failed
			}
		}
	} else {
		p.logger.Debug("No audio mixer detected, skipping volume control", nil)
	}

	// Ensure volume is restored even if playback fails
	defer func() {
		if originalVolume >= 0 {
			if err := p.setVolumeInternal(ctx, originalVolume); err != nil {
				p.logger.Error("Failed to restore original volume", err, map[string]interface{}{
					"original_volume": originalVolume,
				})
			} else {
				p.logger.Debug("Restored original volume", map[string]interface{}{
					"restored_volume": originalVolume,
				})
			}
		}
	}()

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
		// macOS: use osascript to set volume (scale 0-100)
		cmd = exec.CommandContext(ctx, "osascript", "-e", fmt.Sprintf("set volume output volume %d", percent))
	case "linux":
		// Linux: use amixer to set volume, try different control names
		// First try to find the actual control name
		controlName, err := p.findVolumeControl(ctx)
		if err != nil {
			p.logger.Debug("Could not find volume control, trying Master", map[string]interface{}{
				"error": err.Error(),
			})
			controlName = "Master"
		}

		// Build amixer command with explicit card if configured
		args := []string{"sset", controlName, fmt.Sprintf("%d%%", percent)}
		if p.config.AmixerCard != "" {
			args = append([]string{"-c", p.config.AmixerCard}, args...)
		}
		cmd = exec.CommandContext(ctx, "amixer", args...)
	default:
		return fmt.Errorf("volume control not supported on %s", runtime.GOOS)
	}

	if err := cmd.Run(); err != nil {
		// On Linux, if Master fails, try other common control names
		if runtime.GOOS == "linux" {
			alternativeControls := []string{"PCM", "Speaker", "Headphone", "Digital"}
			for _, alt := range alternativeControls {
				altArgs := []string{"sset", alt, fmt.Sprintf("%d%%", percent)}
				if p.config.AmixerCard != "" {
					altArgs = append([]string{"-c", p.config.AmixerCard}, altArgs...)
				}
				altCmd := exec.CommandContext(ctx, "amixer", altArgs...)
				if altErr := altCmd.Run(); altErr == nil {
					p.logger.Info("Successfully set volume using alternative control", map[string]interface{}{
						"control": alt,
						"volume":  percent,
					})
					return nil
				}
			}
		}
		return fmt.Errorf("setting volume to %d%%: %w", percent, err)
	}

	return nil
}

func (p *Player) findVolumeControl(ctx context.Context) (string, error) {
	// First try to get list of available controls with amixer
	args := []string{"scontrols"}
	if p.config.AmixerCard != "" {
		args = append([]string{"-c", p.config.AmixerCard}, args...)
	}
	cmd := exec.CommandContext(ctx, "amixer", args...)
	output, err := cmd.Output()
	if err != nil {
		// If amixer fails, try other approaches
		return p.fallbackVolumeControl(ctx)
	}

	controls := strings.Split(string(output), "\n")
	if len(controls) == 0 {
		return p.fallbackVolumeControl(ctx)
	}

	// Priority order for control names
	priorityControls := []string{"Master", "PCM", "Speaker", "Headphone", "Digital", "Capture"}

	for _, priority := range priorityControls {
		for _, line := range controls {
			if strings.Contains(line, fmt.Sprintf("'%s'", priority)) {
				return priority, nil
			}
		}
	}

	// If no priority control found, try to extract the first available control
	for _, line := range controls {
		if strings.Contains(line, "Simple mixer control") {
			start := strings.Index(line, "'")
			end := strings.LastIndex(line, "'")
			if start != -1 && end != -1 && start != end {
				control := line[start+1 : end]
				return control, nil
			}
		}
	}

	return p.fallbackVolumeControl(ctx)
}

func (p *Player) fallbackVolumeControl(ctx context.Context) (string, error) {
	// Try alternative volume control methods for Raspberry Pi

	// Try checking /proc/asound/cards for available audio devices
	if cards, err := p.getAudioCards(); err == nil && len(cards) > 0 {
		p.logger.Debug("Found audio cards", map[string]interface{}{
			"cards": cards,
		})
	}

	// Try common Raspberry Pi volume controls
	commonControls := []string{"PCM", "Master", "Headphone", "Speaker", "Digital"}

	for _, control := range commonControls {
		// Test if this control exists by trying to get its value
		testArgs := []string{"get", control}
		if p.config.AmixerCard != "" {
			testArgs = append([]string{"-c", p.config.AmixerCard}, testArgs...)
		}
		testCmd := exec.CommandContext(ctx, "amixer", testArgs...)
		if err := testCmd.Run(); err == nil {
			return control, nil
		}
	}

	return "", fmt.Errorf("no working audio controls found")
}

func (p *Player) getAudioCards() ([]string, error) {
	data, err := os.ReadFile("/proc/asound/cards")
	if err != nil {
		return nil, err
	}

	var cards []string
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.Contains(line, ":") && len(line) > 10 {
			parts := strings.Split(line, ":")
			if len(parts) > 1 {
				cards = append(cards, strings.TrimSpace(parts[1]))
			}
		}
	}
	return cards, nil
}

func (p *Player) hasAudioMixer() bool {
	// Quick check if amixer is available and working
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "amixer", "--version")
	if err := cmd.Run(); err != nil {
		return false
	}

	// Try to list controls to see if any audio hardware is available
	args := []string{"scontrols"}
	if p.config.AmixerCard != "" {
		args = append([]string{"-c", p.config.AmixerCard}, args...)
	}
	cmd = exec.CommandContext(ctx, "amixer", args...)
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	// If we get some output, assume we have audio controls
	return len(strings.TrimSpace(string(output))) > 0
}

func (p *Player) getCurrentVolume(ctx context.Context) (int, error) {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		// macOS: get current volume using osascript
		cmd = exec.CommandContext(ctx, "osascript", "-e", "output volume of (get volume settings)")
	case "linux":
		// Linux: find the actual control name and get volume
		controlName, err := p.findVolumeControl(ctx)
		if err != nil {
			controlName = "Master" // fallback
		}

		// Build amixer command with optional card specification
		amixerCmd := "amixer"
		if p.config.AmixerCard != "" {
			amixerCmd = fmt.Sprintf("amixer -c %s", p.config.AmixerCard)
		}
		cmd = exec.CommandContext(ctx, "sh", "-c", fmt.Sprintf("%s get %s | grep -o '[0-9]*%%' | head -1 | sed 's/%%//'", amixerCmd, controlName))
	default:
		return 0, fmt.Errorf("volume control not supported on %s", runtime.GOOS)
	}

	output, err := cmd.Output()
	if err != nil {
		// On Linux, try alternative controls if the primary one fails
		if runtime.GOOS == "linux" {
			alternativeControls := []string{"PCM", "Speaker", "Headphone", "Digital"}
			amixerCmd := "amixer"
			if p.config.AmixerCard != "" {
				amixerCmd = fmt.Sprintf("amixer -c %s", p.config.AmixerCard)
			}
			for _, alt := range alternativeControls {
				altCmd := exec.CommandContext(ctx, "sh", "-c", fmt.Sprintf("%s get %s | grep -o '[0-9]*%%' | head -1 | sed 's/%%//'", amixerCmd, alt))
				if altOutput, altErr := altCmd.Output(); altErr == nil {
					output = altOutput
					err = nil
					break
				}
			}
		}

		if err != nil {
			return 0, fmt.Errorf("getting current volume: %w", err)
		}
	}

	volumeStr := strings.TrimSpace(string(output))
	if volumeStr == "" {
		return 0, fmt.Errorf("empty volume output - amixer may not be available or configured")
	}

	volume, err := strconv.Atoi(volumeStr)
	if err != nil {
		return 0, fmt.Errorf("parsing volume output '%s': %w", volumeStr, err)
	}

	return volume, nil
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
			// Use a longer timeout context and ignore SIGINT/SIGTERM during playback
			playCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			// Build mpg123 command with explicit output driver and device if configured
			args := []string{"-q"}
			if p.config.AudioOutputDriver != "" {
				args = append(args, "-o", p.config.AudioOutputDriver)
			}
			if p.config.AudioDevice != "" {
				args = append(args, "-a", p.config.AudioDevice)
			}
			args = append(args, soundFile)
			cmd = exec.CommandContext(playCtx, "mpg123", args...)
		} else if _, err := exec.LookPath("aplay"); err == nil {
			playCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			// aplay can use -D to specify device
			args := []string{}
			if p.config.AudioDevice != "" {
				args = append(args, "-D", p.config.AudioDevice)
			}
			args = append(args, soundFile)
			cmd = exec.CommandContext(playCtx, "aplay", args...)
		} else {
			return fmt.Errorf("no audio player found (mpg123 or aplay required)")
		}
	default:
		return fmt.Errorf("audio playback not supported on %s", runtime.GOOS)
	}

	// Redirect stdin to prevent blocking
	cmd.Stdin = nil

	// Capture stderr to log actual error messages
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	p.logger.Debug("Executing audio command", map[string]interface{}{
		"command": cmd.Path,
		"args":    cmd.Args,
	})

	if err := cmd.Run(); err != nil {
		stderrOutput := strings.TrimSpace(stderrBuf.String())
		if stderrOutput != "" {
			p.logger.Error("Audio command stderr", fmt.Errorf("%s", stderrOutput), nil)
			return fmt.Errorf("playing audio file %s: %w (stderr: %s)", soundFile, err, stderrOutput)
		}
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

	// Skip volume control if disabled or on systems without audio mixer
	var originalVolume int = -1

	// Only attempt volume control if we can detect audio system
	if p.hasAudioMixer() {
		// Save current volume before changing it
		vol, err := p.getCurrentVolume(ctx)
		if err != nil {
			p.logger.Debug("Cannot get current volume for TTS, skipping volume control", map[string]interface{}{
				"error": err.Error(),
			})
		} else {
			originalVolume = vol
			p.logger.Debug("Saved original volume for TTS", map[string]interface{}{
				"original_volume": originalVolume,
			})

			// Set volume for TTS
			if err := p.setVolumeInternal(ctx, p.config.VolumePct); err != nil {
				p.logger.Debug("Cannot set TTS volume, continuing without volume control", map[string]interface{}{
					"tts_volume": p.config.VolumePct,
					"error": err.Error(),
				})
				originalVolume = -1 // Mark as unavailable since setting failed
			}
		}
	} else {
		p.logger.Debug("No audio mixer detected for TTS, skipping volume control", nil)
	}

	// Ensure volume is restored even if TTS fails
	defer func() {
		if originalVolume >= 0 {
			if err := p.setVolumeInternal(ctx, originalVolume); err != nil {
				p.logger.Error("Failed to restore original volume after TTS", err, map[string]interface{}{
					"original_volume": originalVolume,
				})
			} else {
				p.logger.Debug("Restored original volume after TTS", map[string]interface{}{
					"restored_volume": originalVolume,
				})
			}
		}
	}()

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

	// Redirect stdin to prevent blocking
	cmd.Stdin = nil

	// Capture stderr to log actual error messages
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	if err := cmd.Run(); err != nil {
		stderrOutput := strings.TrimSpace(stderrBuf.String())
		if stderrOutput != "" {
			p.logger.Error("TTS command stderr", fmt.Errorf("%s", stderrOutput), nil)
			return fmt.Errorf("playing TTS message: %w (stderr: %s)", err, stderrOutput)
		}
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
