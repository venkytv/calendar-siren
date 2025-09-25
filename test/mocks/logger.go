package mocks

import "sync"

type MockLogger struct {
	mu       sync.RWMutex
	InfoLog  []LogEntry
	ErrLog   []LogEntry
	DebugLog []LogEntry
}

type LogEntry struct {
	Message string
	Error   error
	Fields  map[string]interface{}
}

func NewMockLogger() *MockLogger {
	return &MockLogger{
		InfoLog:  make([]LogEntry, 0),
		ErrLog:   make([]LogEntry, 0),
		DebugLog: make([]LogEntry, 0),
	}
}

func (m *MockLogger) Info(msg string, fields map[string]interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.InfoLog = append(m.InfoLog, LogEntry{Message: msg, Fields: fields})
}

func (m *MockLogger) Error(msg string, err error, fields map[string]interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ErrLog = append(m.ErrLog, LogEntry{Message: msg, Error: err, Fields: fields})
}

func (m *MockLogger) Debug(msg string, fields map[string]interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.DebugLog = append(m.DebugLog, LogEntry{Message: msg, Fields: fields})
}

func (m *MockLogger) GetInfoLogs() []LogEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]LogEntry(nil), m.InfoLog...)
}

func (m *MockLogger) GetErrorLogs() []LogEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]LogEntry(nil), m.ErrLog...)
}

func (m *MockLogger) GetDebugLogs() []LogEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]LogEntry(nil), m.DebugLog...)
}

func (m *MockLogger) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.InfoLog = m.InfoLog[:0]
	m.ErrLog = m.ErrLog[:0]
	m.DebugLog = m.DebugLog[:0]
}
