package mocks

import (
	"context"
	"sync"

	"github.com/meeting-siren/meeting-siren/internal/domain"
)

type MockAudioPlayer struct {
	mu           sync.RWMutex
	PlayCalls    []PlayCall
	VolumeCalls  []VolumeCall
	TTSCalls     []TTSCall
	ShouldFail   bool
	FailureError error
}

type PlayCall struct {
	SoundFiles []string
	Context    context.Context
}

type VolumeCall struct {
	Percent int
	Context context.Context
}

type TTSCall struct {
	Message string
	Context context.Context
}

func NewMockAudioPlayer() *MockAudioPlayer {
	return &MockAudioPlayer{
		PlayCalls:   make([]PlayCall, 0),
		VolumeCalls: make([]VolumeCall, 0),
		TTSCalls:    make([]TTSCall, 0),
	}
}

func (m *MockAudioPlayer) Play(ctx context.Context, soundFiles []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.PlayCalls = append(m.PlayCalls, PlayCall{
		SoundFiles: append([]string(nil), soundFiles...),
		Context:    ctx,
	})

	if m.ShouldFail {
		return m.FailureError
	}
	return nil
}

func (m *MockAudioPlayer) SetVolume(ctx context.Context, percent int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.VolumeCalls = append(m.VolumeCalls, VolumeCall{
		Percent: percent,
		Context: ctx,
	})

	if m.ShouldFail {
		return m.FailureError
	}
	return nil
}

func (m *MockAudioPlayer) PlayTTS(ctx context.Context, message string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.TTSCalls = append(m.TTSCalls, TTSCall{
		Message: message,
		Context: ctx,
	})

	if m.ShouldFail {
		return m.FailureError
	}
	return nil
}

func (m *MockAudioPlayer) RenderTTSMessage(alert *domain.MeetingAlert) (string, error) {
	if m.ShouldFail {
		return "", m.FailureError
	}
	return "Test TTS message for " + alert.Title, nil
}

func (m *MockAudioPlayer) RenderTTSMessageWithTemplate(alert *domain.MeetingAlert, template string) (string, error) {
	if m.ShouldFail {
		return "", m.FailureError
	}
	return "Test TTS [" + template + "] for " + alert.Title, nil
}

func (m *MockAudioPlayer) GPIOBuzzer(ctx context.Context) error {
	if m.ShouldFail {
		return m.FailureError
	}
	return nil
}

func (m *MockAudioPlayer) GetPlayCalls() []PlayCall {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]PlayCall(nil), m.PlayCalls...)
}

func (m *MockAudioPlayer) GetVolumeCalls() []VolumeCall {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]VolumeCall(nil), m.VolumeCalls...)
}

func (m *MockAudioPlayer) GetTTSCalls() []TTSCall {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]TTSCall(nil), m.TTSCalls...)
}

func (m *MockAudioPlayer) SetFailure(shouldFail bool, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ShouldFail = shouldFail
	m.FailureError = err
}

func (m *MockAudioPlayer) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.PlayCalls = m.PlayCalls[:0]
	m.VolumeCalls = m.VolumeCalls[:0]
	m.TTSCalls = m.TTSCalls[:0]
}
