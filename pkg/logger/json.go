package logger

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

type JSONLogger struct {
	serviceName string
}

func NewJSONLogger(serviceName string) *JSONLogger {
	return &JSONLogger{
		serviceName: serviceName,
	}
}

type logEntry struct {
	Timestamp time.Time              `json:"timestamp"`
	Level     string                 `json:"level"`
	Service   string                 `json:"service"`
	Message   string                 `json:"message"`
	Error     string                 `json:"error,omitempty"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
}

func (l *JSONLogger) Info(msg string, fields map[string]interface{}) {
	l.log("INFO", msg, "", fields)
}

func (l *JSONLogger) Error(msg string, err error, fields map[string]interface{}) {
	var errStr string
	if err != nil {
		errStr = err.Error()
	}
	l.log("ERROR", msg, errStr, fields)
}

func (l *JSONLogger) Debug(msg string, fields map[string]interface{}) {
	l.log("DEBUG", msg, "", fields)
}

func (l *JSONLogger) log(level, msg, errStr string, fields map[string]interface{}) {
	entry := logEntry{
		Timestamp: time.Now().UTC(),
		Level:     level,
		Service:   l.serviceName,
		Message:   msg,
		Fields:    fields,
	}

	if errStr != "" {
		entry.Error = errStr
	}

	data, err := json.Marshal(entry)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to marshal log entry: %v\n", err)
		return
	}

	fmt.Println(string(data))
}
